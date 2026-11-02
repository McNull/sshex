// Package logo renders the sshex ASCII wordmark used by the terminal UI.
package logo

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mcnull/sshex/internal/assets"
	"github.com/mcnull/sshex/internal/ui/theme"
)

// Height is the number of terminal rows the logo occupies.
const Height = 5

var art = plainArt()

// Width is the width of the widest logo line.
var Width = maxWidth(art)

// View renders the logo centered within width. The art is always drawn in the
// theme's logo style, so it follows light/dark mode and NO_COLOR instead of
// using the banner's fixed colors.
func View(styles *theme.Styles, width int) string {
	lines := make([]string, len(art))
	for i, line := range art {
		styled := styles.Logo.Render(line)
		if width > 0 {
			lines[i] = lipgloss.PlaceHorizontal(width, lipgloss.Center, styled)
			continue
		}
		lines[i] = styled
	}
	return strings.Join(lines, "\n")
}

func maxWidth(lines []string) int {
	width := 0
	for _, line := range lines {
		width = max(width, lipgloss.Width(line))
	}
	return width
}

func plainArt() []string {
	plain := ansi.Strip(assets.Logo())
	lines := strings.Split(strings.TrimRight(plain, "\r\n"), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	return lines
}
