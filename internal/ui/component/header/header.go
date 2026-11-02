// Package header renders the top bar of the sshex terminal UI: the wordmark on
// the left and a subtle rule below.
package header

import (
	"strings"

	"github.com/mcnull/sshex/internal/ui/theme"
)

// Model is the header component.
type Model struct {
	styles *theme.Styles
	width  int
}

// New returns a header.
func New(styles *theme.Styles) Model {
	return Model{styles: styles}
}

// Resize sets the available width.
func (m *Model) Resize(width int) {
	m.width = width
}

// Height is the number of terminal rows the header occupies.
func (m Model) Height() int {
	return 2
}

// View renders the header.
func (m Model) View() string {
	if m.width <= 0 {
		return ""
	}

	brand := m.styles.Brand.Render("sshe") + m.styles.BrandAccent.Render("x")
	rule := m.styles.Rule.Render(strings.Repeat("─", m.width))
	return brand + "\n" + rule
}
