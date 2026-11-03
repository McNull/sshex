// Package footer renders the bottom bar of the sshex terminal UI: a subtle
// rule followed by the key help of the active screen.
package footer

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/mcnull/sshex/internal/ui/keys"
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
func (m Model) View(bindings []keys.Binding) string {
	return m.view(bindings, true)
}

// ViewWithoutHelp renders the footer without the global help hint, for screens
// where h is currently accepted as text input.
func (m Model) ViewWithoutHelp(bindings []keys.Binding) string {
	return m.view(bindings, false)
}

// ViewHelpClose renders the footer while the help overlay is open.
func (m Model) ViewHelpClose() string {
	return m.view([]keys.Binding{keys.NewBinding(
		keys.WithKeys("esc"),
		keys.WithHelp("esc", "close help"),
	)}, false)
}

func (m Model) view(bindings []keys.Binding, showHelp bool) string {
	if m.width <= 0 {
		return ""
	}

	rule := m.styles.Rule.Render(strings.Repeat("─", m.width))
	help := m.help(bindings, showHelp)
	return rule + "\n" + ansi.Truncate(help, m.width, "…")
}

func (m Model) help(bindings []keys.Binding, showHelp bool) string {
	parts := make([]string, 0, len(bindings)+3)
	back := ""
	navigation := false
	other := make([]keys.Binding, 0, len(bindings))
	seen := make(map[string]bool, len(bindings))
	for _, binding := range bindings {
		if !binding.Enabled() {
			continue
		}
		info := binding.Help()
		if info.Key == "" {
			continue
		}
		if seen[info.Key+"\x00"+info.Desc] {
			continue
		}
		seen[info.Key+"\x00"+info.Desc] = true
		keys := binding.Keys()
		if hasKey(keys, "esc") {
			label := info.Key
			if hasKey(keys, "q") {
				label = "esc"
			}
			back = m.renderBinding(label, info.Desc)
			continue
		}
		if hasKey(keys, "up") || hasKey(keys, "down") {
			navigation = true
			continue
		}
		if hasKey(keys, "r") && isRefresh(info.Desc) {
			continue
		}
		if hasKey(keys, "/") && info.Desc == "search" {
			continue
		}
		other = append(other, binding)
	}
	if back != "" {
		parts = append(parts, back)
	}
	if showHelp {
		parts = append(parts, m.renderBinding("h", "help"))
	}
	if navigation {
		parts = append(parts, m.renderBinding("↑/↓", "navigate"))
	}
	for _, binding := range other {
		info := binding.Help()
		keyLabel := info.Key
		parts = append(parts, m.renderBinding(keyLabel, info.Desc))
	}
	return strings.Join(parts, m.styles.FooterSeparator.Render(separator))
}

func isRefresh(description string) bool {
	description = strings.ToLower(description)
	return strings.Contains(description, "refresh")
}

func (m Model) renderBinding(key, desc string) string {
	return m.styles.FooterKey.Render(key) + " " + m.styles.FooterDesc.Render(desc)
}

func hasKey(keys []string, want string) bool {
	for _, key := range keys {
		if key == want {
			return true
		}
	}
	return false
}
