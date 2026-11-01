package remoteinstall

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"strings"
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

// platformScript prints uname -s and uname -m in two lines.
const platformScript = `uname -s; uname -m`

// DetectPlatform identifies the operating system and architecture of the
// machine a runner is connected to.
func DetectPlatform(ctx context.Context, run Runner) (Platform, error) {
	var buf bytes.Buffer
	if err := run.Run(ctx, platformScript, nil, &buf, nil); err != nil {
		return Platform{}, err
	}
	return ParsePlatform(buf.String())
}

// ParsePlatform parses the output of DetectPlatform's script.
func ParsePlatform(output string) (Platform, error) {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) != 2 {
		return Platform{}, fmt.Errorf("could not detect remote platform")
	}
	os, osOK := unameOS[strings.ToLower(lines[0])]
	arch, archOK := unameArch[strings.ToLower(lines[1])]
	if !osOK || !archOK {
		return Platform{}, fmt.Errorf("unsupported remote platform: %s/%s (supported: %s)", lines[0], lines[1], supportedList())
	}
	return Platform{OS: os, Arch: arch}, nil
}

func supportedList() string {
	names := make([]string, 0, len(SupportedPlatforms))
	for _, p := range SupportedPlatforms {
		names = append(names, p.String())
	}
	return strings.Join(names, ", ")
}
