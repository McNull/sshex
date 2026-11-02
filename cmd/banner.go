package cmd

import (
	"fmt"
	"io"
	"os"
	"regexp"

	"github.com/mcnull/sshex/internal/assets"
	"github.com/spf13/cobra"
)

const bannerAnnotation = "sshex.banner"

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// showBanner reports whether the banner should be printed for the command
// being executed. It shows for a bare `sshex` invocation and for commands
// explicitly marked as interactive via bannerAnnotation. The banner is hidden
// entirely in demo mode so recordings stay clean.
func showBanner(cmd *cobra.Command) bool {
	if recordDemoMode() || bannerSuppressed() {
		return false
	}
	if cmd == rootCmd {
		return true
	}
	return cmd.Annotations[bannerAnnotation] == "true"
}

// bannerSuppressed reports whether the caller asked to keep the terminal clean
// for a session it launched itself, such as one started from the TUI.
func bannerSuppressed() bool {
	return os.Getenv("SSHEX_NO_BANNER") != ""
}

// isColorTerminal reports whether colors should be emitted to w. It is false
// when NO_COLOR is set or w is not a character device.
func isColorTerminal(w io.Writer) bool {
	return os.Getenv("NO_COLOR") == "" && isTerminalWriter(w)
}

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

// printUpdateAvailable reports an available release, using color only when the
// destination is a color terminal.
func printUpdateAvailable(w io.Writer, version string) {
	if isColorTerminal(w) {
		fmt.Fprintf(w, "Update available: \x1b[33m%s\x1b[0m\nRun \x1b[34msshex update\x1b[0m to install.\n\n", version)
		return
	}
	fmt.Fprintf(w, "Update available: %s\nRun sshex update to install.\n\n", version)
}

func printBanner(w io.Writer) {
	logo := assets.Logo()
	version := assets.Version()
	if !isColorTerminal(w) {
		logo = stripANSI(logo)
		version = stripANSI(version)
	} else {
		// question: if 33m is yellow, what is bright white? It's 97m.
		version = fmt.Sprintf("\x1b[97m%s\x1b[0m", version)
	}
	fmt.Fprintln(w, logo)
	fmt.Fprintf(w, "Version: %s\n\n", version)
}
