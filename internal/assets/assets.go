package assets

import (
	_ "embed"
	"strings"
)

//go:embed LOGO.txt
var logoRaw string

//go:embed VERSION.txt
var versionRaw string

// Logo returns the banner with the literal `\033` escape text used in the
// source file converted to real ESC bytes and the {VERSION} placeholder
// replaced with the current version.
func Logo() string {
	logo := strings.ReplaceAll(logoRaw, `\033`, "\x1b")
	return strings.ReplaceAll(logo, "{VERSION}", "v"+Version())
}

// Version returns the trimmed contents of VERSION.txt.
func Version() string {
	return strings.TrimSpace(versionRaw)
}
