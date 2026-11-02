// Package footer renders the bottom bar of the sshex terminal UI: a subtle
// rule followed by the key help of the active screen.
package footer

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"

	"github.com/mcnull/sshex/internal/ui/theme"
)

const separator = "  ·  "

// Model is the footer component.
type Model struct {
	styles *theme.Styles
	width  int
}

// New returns a footer using the given styles.
func New(styles *theme.Styles) Model {
	return Model{styles: styles}
}

// Resize sets the available width.
func (m *Model) Resize(width int) {
	m.width = width
}

// Height is the number of terminal rows the footer occupies.
func (m Model) Height() int {
	return 2
}

// View renders the footer help for the supplied bindings.
func (m Model) View(bindings []key.Binding) string {
	if m.width <= 0 {
		return ""
	}

	rule := m.styles.Rule.Render(strings.Repeat("─", m.width))
	help := m.help(bindings)
	return rule + "\n" + ansi.Truncate(help, m.width, "…")
}

func (m Model) help(bindings []key.Binding) string {
	parts := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		if !binding.Enabled() {
			continue
		}
		info := binding.Help()
		if info.Key == "" {
			continue
		}
		parts = append(parts, m.styles.FooterKey.Render(info.Key)+" "+m.styles.FooterDesc.Render(info.Desc))
	}
	return strings.Join(parts, m.styles.FooterSeparator.Render(separator))
}
