package remoteinstall

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/mcnull/sshex/internal/verbose"
)

// Platform identifies a remote operating system and CPU architecture. OS and
// Arch use the Go GOOS/GOARCH vocabulary so they map directly onto
// cross-compilation and release artifact names.
type Platform struct {
	OS   string
	Arch string
}

func (p Platform) String() string { return p.OS + "/" + p.Arch }

// IsLocal reports whether the platform matches the machine running sshex.
func (p Platform) IsLocal() bool {
	return p.OS == runtime.GOOS && p.Arch == runtime.GOARCH
}

// SupportedPlatforms lists the remote platforms sshex can install itself on.
var SupportedPlatforms = []Platform{
	{OS: "linux", Arch: "amd64"},
	{OS: "linux", Arch: "arm64"},
	{OS: "darwin", Arch: "amd64"},
	{OS: "darwin", Arch: "arm64"},
}

var (
	unameOS = map[string]string{
		"linux":  "linux",
		"darwin": "darwin",
	}
	unameArch = map[string]string{
		"x86_64":  "amd64",
		"amd64":   "amd64",
		"aarch64": "arm64",
		"arm64":   "arm64",
	}
)

const (
	platformBeginMarker = "SSHEX_REMOTE_PLATFORM_BEGIN_7F3A9C"
	platformEndMarker   = "SSHEX_REMOTE_PLATFORM_END_7F3A9C"
)

// platformScript frames uname output so shell startup messages cannot be
// mistaken for platform data.
const platformScript = `printf '%s\n' '` + platformBeginMarker + `'; uname -s; uname -m; printf '%s\n' '` + platformEndMarker + `'`

// DetectPlatform identifies the operating system and architecture of the
// machine a runner is connected to.
func DetectPlatform(ctx context.Context, run Runner) (Platform, error) {
	var buf bytes.Buffer
	if err := run.Run(ctx, platformScript, nil, &buf, nil); err != nil {
		return Platform{}, err
	}
	output := buf.String()
	verbosef("remote platform probe output: %q", diagnosticOutput(output))
	return parsePlatform(output)
}

// ParsePlatform parses the output of DetectPlatform's script.
func ParsePlatform(output string) (Platform, error) {
	return parsePlatform(output)
}

func parsePlatform(output string) (Platform, error) {
	allLines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	begin, end := -1, -1
	for i, line := range allLines {
		switch strings.TrimSpace(line) {
		case platformBeginMarker:
			begin = i
		case platformEndMarker:
			if begin >= 0 && i > begin {
				end = i
			}
		}
	}
	var data []string
	if begin >= 0 || end >= 0 {
		if begin < 0 || end != begin+3 {
			return Platform{}, fmt.Errorf("could not detect remote platform from framed probe output: %q", output)
		}
		data = allLines[begin+1 : end]
	} else {
		data = allLines
	}
	var lines []string
	for _, line := range data {
		if line = strings.TrimSpace(strings.TrimSuffix(line, "\r")); line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) != 2 {
		return Platform{}, fmt.Errorf("could not detect remote platform from probe output: %q", diagnosticOutput(output))
	}
	os, osOK := unameOS[strings.ToLower(lines[0])]
	arch, archOK := unameArch[strings.ToLower(lines[1])]
	if !osOK || !archOK {
		return Platform{}, fmt.Errorf("unsupported remote platform: %s/%s (supported: %s)", lines[0], lines[1], supportedList())
	}
	return Platform{OS: os, Arch: arch}, nil
}

func diagnosticOutput(output string) string {
	const limit = 512
	if len(output) <= limit {
		return output
	}
	return output[:limit] + "…"
}

func verbosef(format string, args ...any) {
	if verbose.ShouldLog() {
		_, _ = fmt.Fprintf(os.Stderr, "sshex: verbose: "+format+"\n", args...)
	}
}

func supportedList() string {
	names := make([]string, 0, len(SupportedPlatforms))
	for _, p := range SupportedPlatforms {
		names = append(names, p.String())
	}
	return strings.Join(names, ", ")
}
