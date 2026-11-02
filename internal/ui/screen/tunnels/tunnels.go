// Package tunnels provides the session-scoped tunnel screen. It is a placeholder
// until the tunnel management UI is implemented; the Sessions screen already
// opens it with the selected session, so the future implementation can replace
// this model without changing the navigation.
package tunnels

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mcnull/sshex/internal/ui/keys"
	"github.com/mcnull/sshex/internal/ui/screen"
	"github.com/mcnull/sshex/internal/ui/theme"
)

// Options configures the tunnel screen.
type Options struct {
	Styles    *theme.Styles
	SessionID string
	Target    string
}

// Model is the placeholder tunnel screen for one session.
type Model struct {
	styles    *theme.Styles
	sessionID string
	target    string
	back      key.Binding
	width     int
	height    int
}

// New returns the tunnel screen for a session.
func New(opts Options) *Model {
	return &Model{
		styles:    opts.Styles,
		sessionID: opts.SessionID,
		target:    opts.Target,
		back:      keys.Back(),
	}
}

// Init implements screen.Screen.
func (m *Model) Init() tea.Cmd { return nil }

// Title implements screen.Screen.
func (m *Model) Title() string { return "Tunnels" }

// Keys implements screen.Screen.
func (m *Model) Keys() []key.Binding { return []key.Binding{m.back} }

// Resize implements screen.Screen.
func (m *Model) Resize(width, height int) {
	m.width = width
	m.height = height
}

// Update implements screen.Screen.
func (m *Model) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	press, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	if key.Matches(press, m.back) {
		return m, screen.Pop()
	}
	return m, nil
}

// View implements screen.Screen.
func (m *Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	block := m.styles.ScreenTitle.Render("Tunnels") + "\n" +
		m.styles.ScreenSubtitle.Render(m.target) + "\n\n" +
		m.styles.ScreenBody.Render("This screen is not implemented yet.") + "\n\n" +
		m.styles.ScreenHint.Render("Session "+m.sessionID+" will list its tunnels here.") + "\n" +
		m.styles.ScreenHint.Render("Press esc to go back.")

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, block)
}
