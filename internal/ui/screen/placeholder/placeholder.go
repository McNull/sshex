// Package placeholder provides a temporary screen for menu entries whose real
// screen has not been implemented yet.
package placeholder

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mcnull/sshex/internal/ui/keys"
	"github.com/mcnull/sshex/internal/ui/screen"
	"github.com/mcnull/sshex/internal/ui/theme"
)

// Model is a simple titled screen with a "coming soon" message.
type Model struct {
	title       string
	description string
	styles      *theme.Styles
	back        keys.Binding
	width       int
	height      int
}

// New returns a placeholder screen.
func New(title, description string, styles *theme.Styles) *Model {
	return &Model{
		title:       title,
		description: description,
		styles:      styles,
		back:        keys.Back(),
	}
}

// Init implements screen.Screen.
func (m *Model) Init() tea.Cmd { return nil }

// Title implements screen.Screen.
func (m *Model) Title() string { return m.title }

// Keys implements screen.Screen.
func (m *Model) Keys() []keys.Binding { return []keys.Binding{m.back} }

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
	if keys.Matches(press, m.back) {
		return m, screen.Pop()
	}
	return m, nil
}

// View implements screen.Screen.
func (m *Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	block := m.styles.ScreenTitle.Render(m.title) + "\n" +
		m.styles.ScreenSubtitle.Render(m.description) + "\n\n" +
		m.styles.ScreenBody.Render("This screen is not implemented yet.") + "\n\n" +
		m.styles.ScreenHint.Render("Press esc to go back.")

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, block)
}
