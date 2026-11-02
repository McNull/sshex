// Package updater downloads and installs the latest sshex release.
package updater

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const (
	maxDownloadSize = 256 << 20
	maxBinarySize   = 128 << 20
)

var releaseTagPattern = regexp.MustCompile(`^v?[0-9][A-Za-z0-9.+-]*$`)

// Config contains the release endpoints and HTTP client used by an Updater.
// Empty endpoints use the official sshex GitHub repository.
type Config struct {
	Client      *http.Client
	APIBase     string
	ReleaseBase string
	OS          string
	Arch        string
}

// Result describes the outcome of an update check.
type Result struct {
	CurrentVersion string
	LatestVersion  string
	Updated        bool
}

// Updater checks GitHub Releases and atomically replaces the running binary.
type Updater struct {
	client      *http.Client
	apiBase     string
	releaseBase string
	os          string
	arch        string
}

// New constructs an updater configured for the current host.
func New(cfg Config) *Updater {
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	apiBase := cfg.APIBase
	if apiBase == "" {
		apiBase = "https://api.github.com/repos/McNull/sshex/releases/latest"
	}
	releaseBase := cfg.ReleaseBase
	if releaseBase == "" {
		releaseBase = "https://github.com/McNull/sshex/releases/download"
	}
	goos, goarch := cfg.OS, cfg.Arch
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	return &Updater{client: client, apiBase: apiBase, releaseBase: releaseBase, os: goos, arch: goarch}
}

// Update downloads the latest supported release and replaces executablePath
// when its version differs from currentVersion. The replacement is verified
// against the release checksums before the current executable is touched.
func (u *Updater) Update(ctx context.Context, currentVersion, executablePath string) (Result, error) {
	result := Result{CurrentVersion: normalizeVersion(currentVersion)}
	tag, err := u.latestTag(ctx)
	if err != nil {
		return result, err
	}
	result.LatestVersion = normalizeVersion(tag)
	if result.CurrentVersion == result.LatestVersion {
		return result, nil
	}
	if u.os != "linux" && u.os != "darwin" {
		return result, fmt.Errorf("self-update is not supported on %s", u.os)
	}
	if u.arch != "amd64" && u.arch != "arm64" {
		return result, fmt.Errorf("self-update is not supported on %s/%s", u.os, u.arch)
	}
	if !releaseTagPattern.MatchString(tag) {
		return result, fmt.Errorf("invalid latest release tag %q", tag)
	}

	asset := "sshex-" + u.os + "-" + u.arch + ".zip"
	base := strings.TrimRight(u.releaseBase, "/") + "/" + tag + "/"
	checksums, err := u.get(ctx, base+"checksums.txt")
	if err != nil {
		return result, fmt.Errorf("download release checksums: %w", err)
	}
	want, err := checksumFor(string(checksums), asset)
	if err != nil {
		return result, err
	}
	archive, err := u.get(ctx, base+asset)
	if err != nil {
		return result, fmt.Errorf("download %s: %w", asset, err)
	}
	actual := sha256.Sum256(archive)
	if !strings.EqualFold(hex.EncodeToString(actual[:]), want) {
		return result, fmt.Errorf("checksum mismatch for %s", asset)
	}
	binary, err := unpackBinary(archive)
	if err != nil {
		return result, err
	}
	if err := replaceExecutable(executablePath, binary); err != nil {
		return result, fmt.Errorf("replace executable: %w", err)
	}
	result.Updated = true
	return result, nil
}

// LatestVersion returns the latest published release version without
// downloading or installing its assets.
func (u *Updater) LatestVersion(ctx context.Context) (string, error) {
	tag, err := u.latestTag(ctx)
	if err != nil {
		return "", err
	}
	if !releaseTagPattern.MatchString(tag) {
		return "", fmt.Errorf("invalid latest release tag %q", tag)
	}
	return normalizeVersion(tag), nil
}

func (u *Updater) latestTag(ctx context.Context) (string, error) {
	data, err := u.get(ctx, u.apiBase)
	if err != nil {
		return "", fmt.Errorf("check latest release: %w", err)
	}
	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(data, &release); err != nil {
		return "", fmt.Errorf("decode latest release: %w", err)
	}
	if release.TagName == "" {
		return "", errors.New("latest release has no tag")
	}
	return release.TagName, nil
}

func (u *Updater) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "sshex-updater")
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := u.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDownloadSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxDownloadSize {
		return nil, errors.New("download exceeds size limit")
	}
	return data, nil
}

func checksumFor(checksums, asset string) (string, error) {
	var found string
	for _, line := range strings.Split(checksums, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimLeft(fields[1], "*") == asset {
			if len(fields[0]) != sha256.Size*2 {
				return "", fmt.Errorf("invalid SHA-256 checksum for %s", asset)
			}
			if _, err := hex.DecodeString(fields[0]); err != nil {
				return "", fmt.Errorf("invalid SHA-256 checksum for %s", asset)
			}
			if found != "" {
				return "", fmt.Errorf("duplicate checksum for %s", asset)
			}
			found = fields[0]
		}
	}
	if found == "" {
		return "", fmt.Errorf("checksum for %s is missing from checksums.txt", asset)
	}
	return found, nil
}

func unpackBinary(data []byte) ([]byte, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open release archive: %w", err)
	}
	for _, file := range archive.File {
		if file.Name != "sshex" || file.FileInfo().IsDir() {
			continue
		}
		if file.UncompressedSize64 > maxBinarySize {
			return nil, errors.New("sshex binary exceeds size limit")
		}
		r, err := file.Open()
		if err != nil {
			return nil, err
		}
		binary, readErr := io.ReadAll(io.LimitReader(r, maxBinarySize+1))
		closeErr := r.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if len(binary) == 0 || len(binary) > maxBinarySize {
			return nil, errors.New("release contains an invalid sshex binary")
		}
		return binary, nil
	}
	return nil, errors.New("release archive did not contain an sshex binary")
}

func replaceExecutable(path string, binary []byte) error {
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() || info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("%s is not an executable file", path)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".sshex-update-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(binary); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func normalizeVersion(version string) string {
	return strings.TrimPrefix(strings.TrimSpace(version), "v")
}
