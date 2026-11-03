// Package help renders the contextual hotkey overlay for the terminal UI.
package help

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mcnull/sshex/internal/ui/keys"
	"github.com/mcnull/sshex/internal/ui/theme"
)

// Model renders help for one active screen.
type Model struct {
	styles *theme.Styles
	width  int
	height int
}

// New returns a help overlay using the shared styles.
func New(styles *theme.Styles) Model { return Model{styles: styles} }

// Resize sets the available content area.
func (m *Model) Resize(width, height int) {
	m.width = width
	m.height = height
}

// View renders optional screen guidance and the active hotkeys.
func (m Model) View(title, text string, bindings []keys.Binding) string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	panelWidth := min(m.width, 72)
	innerWidth := max(panelWidth-6, 1)
	lines := []string{m.styles.ScreenTitle.Render(title + " help"), ""}
	if strings.TrimSpace(text) != "" {
		wrapped := lipgloss.NewStyle().Width(innerWidth).Render(strings.TrimSpace(text))
		lines = append(lines, m.styles.ScreenBody.Render(wrapped), "")
	}
	for _, binding := range bindings {
		if !binding.Enabled() {
			continue
		}
		info := binding.Help()
		if info.Key == "" {
			continue
		}
		lines = append(lines, wrapHelpDescription(info.Key, binding.LongHelp(), m.styles, innerWidth)...)
	}
	lines = append(lines, "", m.styles.ScreenHint.Render("Press esc to close help"))

	content := strings.Join(lines, "\n")
	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.styles.Rule.GetForeground()).
		Padding(1, 2).
		Width(innerWidth).
		MaxHeight(m.height).
		Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel)
}

func wrapHelpDescription(keyLabel, description string, styles *theme.Styles, width int) []string {
	keyWidth := ansi.StringWidth(keyLabel)
	indent := strings.Repeat(" ", keyWidth+2)
	available := width - keyWidth - 2
	if available <= 0 {
		return []string{ansi.Truncate(styles.FooterKey.Render(keyLabel)+"  "+styles.FooterDesc.Render(description), width, "…")}
	}
	lines := make([]string, 0, 2)
	prefix := styles.FooterKey.Render(keyLabel) + "  "
	content := ""
	for _, word := range strings.Fields(description) {
		if content != "" && ansi.StringWidth(content)+1+ansi.StringWidth(word) > available {
			lines = append(lines, prefix+styles.FooterDesc.Render(content))
			prefix = styles.FooterDesc.Render(indent)
			content = ""
		}
		if content != "" {
			content += " "
		}
		if ansi.StringWidth(word) > available {
			word = ansi.Truncate(word, available, "…")
		}
		content += word
	}
	if content != "" {
		lines = append(lines, prefix+styles.FooterDesc.Render(content))
	}
	return lines
}
