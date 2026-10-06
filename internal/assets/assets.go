package assets

import (
	_ "embed"
	"strings"
)

//go:embed LOGO.txt
var logoRaw string

//go:embed VERSION.txt
var versionRaw string

// version is overridden at build time with:
//
//	-ldflags "-X github.com/mcnull/sshex/internal/assets.version=vX.Y.Z"
//
// When empty the embedded VERSION.txt is used instead.
var version = ""

// Logo returns the banner with the literal `\033` escape text used in the
// source file converted to real ESC bytes and the {VERSION} placeholder
// replaced with the current version.
func Logo() string {
	logo := strings.ReplaceAll(logoRaw, `\033`, "\x1b")
	return strings.ReplaceAll(logo, "{VERSION}", "v"+Version())
}

// Version returns the build-time injected version when present, otherwise the
// trimmed contents of VERSION.txt. A leading "v" is stripped so callers can
// format it consistently.
func Version() string {
	v := strings.TrimSpace(version)
	if v == "" {
		v = strings.TrimSpace(versionRaw)
	}
	return strings.TrimPrefix(v, "v")
}
