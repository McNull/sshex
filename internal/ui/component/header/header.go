// Package header renders the top bar of the sshex terminal UI: the wordmark on
// the left and a subtle rule below.
package header

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mcnull/sshex/internal/ui/theme"
)

// Model is the header component.
type Model struct {
	styles  *theme.Styles
	width   int
	origin  string
	client  string
	version string
}

// New returns a header showing the app version and origin and, when present,
// the client.
func New(styles *theme.Styles, version, origin, client string) Model {
	return Model{styles: styles, version: versionLabel(version), origin: origin, client: client}
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
	brand += m.styles.HeaderLabel.Render(": ") + m.styles.Version.Render(m.version)
	var identity strings.Builder
	if m.client != "" {
		identity.WriteString(m.styles.HeaderLabel.Render("client: "))
		identity.WriteString(m.styles.HeaderIdentity.Render(m.client))
		identity.WriteString(m.styles.HeaderLabel.Render(" "))
	}
	if m.origin != "" {
		identity.WriteString(m.styles.HeaderLabel.Render("origin: "))
		identity.WriteString(m.styles.HeaderIdentity.Render(m.origin))
	}
	if identity.Len() > 0 {
		available := max(m.width-lipgloss.Width(brand)-1, 0)
		renderedIdentity := ansi.Truncate(identity.String(), available, "…")
		padding := max(m.width-lipgloss.Width(brand)-lipgloss.Width(renderedIdentity), 1)
		brand += strings.Repeat(" ", padding) + renderedIdentity
	}
	rule := m.styles.Rule.Render(strings.Repeat("─", m.width))
	return brand + "\n" + rule
}

func versionLabel(version string) string {
	version = strings.TrimSpace(version)
	version = strings.SplitN(version, "-", 2)[0]
	version = strings.TrimPrefix(version, "v")
	if version == "" {
		return "dev"
	}
	return "v" + version
}
