package versioncheck

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var ErrCheckInProgress = errors.New("version check is already in progress")

// LatestSource supplies the currently published version.
type LatestSource interface {
	LatestVersion(context.Context) (string, error)
}

var versionPattern = regexp.MustCompile(`^v?([0-9]+(\.[0-9]+)*)(-([0-9A-Za-z.-]+))?(\+[0-9A-Za-z.-]+)?$`)

// StartIfDue launches a detached copy of the current executable when the last
// check is at least 24 hours old. It returns immediately; state errors are
// intentionally non-fatal to normal CLI startup.
func StartIfDue(path string, launch func(string) error, now time.Time) {
	store := NewStore(path)
	state, err := store.Load()
	if err != nil || (!state.CheckedAt.IsZero() && now.Sub(state.CheckedAt) < CheckInterval) {
		return
	}
	_ = launch(path)
}

// RunWorker performs a due check, allowing only one worker at a time. The
// network request runs without the state lock; only the final save locks it.
func RunWorker(ctx context.Context, path string, source LatestSource, now func() time.Time) error {
	lockPath := path + ".worker.lock"
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return err
	}
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, fileMode)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return ErrCheckInProgress
		}
		return err
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)

	store := NewStore(path)
	state, err := store.Load()
	if err != nil {
		return err
	}
	startedAt := now()
	if !state.CheckedAt.IsZero() && startedAt.Sub(state.CheckedAt) < CheckInterval {
		return nil
	}
	latest, checkErr := source.LatestVersion(ctx)
	if checkErr != nil {
		state.Error = checkErr.Error()
	} else {
		state.AvailableVersion = latest
		state.Error = ""
	}
	state.CheckedAt = now()
	if err := store.Save(state); err != nil {
		return err
	}
	return checkErr
}

// IsNewer reports whether candidate is a valid semantic version newer than
// current. Numeric components are compared numerically; prereleases sort before
// their corresponding final release.
func IsNewer(candidate, current string) bool {
	candidateParts, ok := parseVersion(candidate)
	if !ok {
		return false
	}
	currentParts, ok := parseVersion(current)
	if !ok {
		return false
	}
	length := len(candidateParts.numbers)
	if len(currentParts.numbers) > length {
		length = len(currentParts.numbers)
	}
	for i := 0; i < length; i++ {
		var a, b int64
		if i < len(candidateParts.numbers) {
			a = candidateParts.numbers[i]
		}
		if i < len(currentParts.numbers) {
			b = currentParts.numbers[i]
		}
		if a != b {
			return a > b
		}
	}
	if candidateParts.pre == currentParts.pre {
		return false
	}
	if candidateParts.pre == "" {
		return true
	}
	if currentParts.pre == "" {
		return false
	}
	return comparePrerelease(candidateParts.pre, currentParts.pre) > 0
}

type parsedVersion struct {
	numbers []int64
	pre     string
}

func parseVersion(version string) (parsedVersion, bool) {
	match := versionPattern.FindStringSubmatch(strings.TrimSpace(version))
	if match == nil {
		return parsedVersion{}, false
	}
	parts := strings.Split(match[1], ".")
	parsed := parsedVersion{pre: match[4]}
	for _, part := range parts {
		n, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return parsedVersion{}, false
		}
		parsed.numbers = append(parsed.numbers, n)
	}
	return parsed, true
}

func comparePrerelease(a, b string) int {
	left, right := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(left) && i < len(right); i++ {
		if left[i] == right[i] {
			continue
		}
		li, le := strconv.ParseUint(left[i], 10, 64)
		ri, re := strconv.ParseUint(right[i], 10, 64)
		switch {
		case le == nil && re == nil:
			if li > ri {
				return 1
			}
			return -1
		case le == nil:
			return -1
		case re == nil:
			return 1
		case left[i] > right[i]:
			return 1
		default:
			return -1
		}
	}
	if len(left) > len(right) {
		return 1
	}
	if len(left) < len(right) {
		return -1
	}
	return 0
}
