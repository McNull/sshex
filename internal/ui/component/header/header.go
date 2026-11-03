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
	title   string
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

// SetTitle replaces the screen title shown in the middle.
func (m *Model) SetTitle(title string) {
	m.title = title
}

// SetIdentity replaces the origin and client labels shown on the right.
func (m *Model) SetIdentity(origin, client string) {
	m.origin = origin
	m.client = client
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

	line := m.renderLine(brand, identity.String())
	rule := m.styles.Rule.Render(strings.Repeat("─", m.width))
	return line + "\n" + rule
}

// renderLine places the title in the middle, the brand on the left and the
// identity on the right. When space is tight the title and identity are
// truncated rather than allowed to overlap.
func (m Model) renderLine(brand, identity string) string {
	title := strings.TrimSpace(m.title)
	if title == "" {
		return m.renderBrandIdentity(brand, identity)
	}

	renderedTitle := m.styles.HeaderTitle.Render(title)
	brandWidth := lipgloss.Width(brand)
	titleWidth := lipgloss.Width(renderedTitle)
	identityWidth := lipgloss.Width(identity)

	maxTitleWidth := max(m.width-brandWidth-1, 0)
	if titleWidth > maxTitleWidth {
		renderedTitle = ansi.Truncate(renderedTitle, maxTitleWidth, "…")
		titleWidth = lipgloss.Width(renderedTitle)
	}
	if titleWidth == 0 {
		return m.renderBrandIdentity(brand, identity)
	}

	start := max((m.width-titleWidth)/2, brandWidth+1)
	if identityWidth > 0 {
		// Shift the title left just enough to keep one cell before the identity.
		start = min(start, max(m.width-identityWidth-1-titleWidth, brandWidth+1))
		available := max(m.width-start-titleWidth-1, 0)
		if identityWidth > available {
			identity = ansi.Truncate(identity, available, "…")
			identityWidth = lipgloss.Width(identity)
		}
	}

	padding := max(m.width-start-titleWidth-identityWidth, 0)
	return brand + strings.Repeat(" ", max(start-brandWidth, 0)) +
		renderedTitle + strings.Repeat(" ", padding) + identity
}

// renderBrandIdentity keeps the original header layout when a screen has no
// title.
func (m Model) renderBrandIdentity(brand, identity string) string {
	if identity == "" {
		return brand
	}
	available := max(m.width-lipgloss.Width(brand)-1, 0)
	renderedIdentity := ansi.Truncate(identity, available, "…")
	padding := max(m.width-lipgloss.Width(brand)-lipgloss.Width(renderedIdentity), 1)
	return brand + strings.Repeat(" ", padding) + renderedIdentity
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
