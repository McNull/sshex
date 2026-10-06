package cmd

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/mcnull/sshex/internal/assets"
	"github.com/spf13/cobra"
)

const bannerAnnotation = "sshex.banner"

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// showBanner reports whether the banner should be printed for the command
// being executed. It shows for a bare `sshex` invocation and for commands
// explicitly marked as interactive via bannerAnnotation.
func showBanner(cmd *cobra.Command) bool {
	if cmd == rootCmd {
		return true
	}
	return cmd.Annotations[bannerAnnotation] == "true"
}

// isColorTerminal reports whether colors should be emitted to w. It is false
// when NO_COLOR is set or w is not a character device.
func isColorTerminal(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

// printBanner writes the logo, which already carries the version number.
func printBanner(w io.Writer) {
	logo := assets.Logo()
	if !isColorTerminal(w) {
		logo = stripANSI(logo)
	}
	fmt.Fprint(w, logo)
	if !strings.HasSuffix(logo, "\n") {
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w)
}
