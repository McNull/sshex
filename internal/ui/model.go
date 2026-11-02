package ui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mcnull/sshex/internal/ui/component/footer"
	"github.com/mcnull/sshex/internal/ui/component/header"
	"github.com/mcnull/sshex/internal/ui/keys"
	"github.com/mcnull/sshex/internal/ui/screen"
	"github.com/mcnull/sshex/internal/ui/screen/menu"
	"github.com/mcnull/sshex/internal/ui/screen/placeholder"
	"github.com/mcnull/sshex/internal/ui/theme"
)

const (
	minWidth  = 48
	minHeight = 16
)

// ModelOptions configures the root model.
type ModelOptions struct {
	Version           string
	UpdateAvailable   string
	Theme             string
	HasDarkBackground bool
	NoColor           bool
}

// Model is the root Bubble Tea model. It owns the chrome, the theme and the
// screen stack; screens are responsible only for their own content.
type Model struct {
	styles    *theme.Styles
	themeName string
	noColor   bool
	global    keys.Global
	header    header.Model
	footer    footer.Model
	stack     *screen.Stack
	width     int
	height    int
}

var _ tea.Model = (*Model)(nil)

// New builds the root model with the main menu as its root screen.
func New(opts ModelOptions) *Model {
	resolved := theme.Load(theme.Options{
		Name:              opts.Theme,
		HasDarkBackground: opts.HasDarkBackground,
		NoColor:           opts.NoColor,
	})
	styles := theme.NewStyles(resolved)

	m := &Model{
		styles:    &styles,
		themeName: resolved.Name,
		noColor:   resolved.NoColor,
		global:    keys.DefaultGlobal(),
		header:    header.New(&styles),
		footer:    footer.New(&styles),
		stack:     screen.NewStack(menu.New(defaultMenuItems(&styles), opts.Version, opts.UpdateAvailable, &styles)),
	}
	return m
}

// Init implements tea.Model. It asks the terminal for its background color so
// the theme can adapt once the program owns the terminal.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.stack.Top().Init(),
		func() tea.Msg { return tea.RequestBackgroundColor() },
	)
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil
	case screen.ActionMsg:
		return m, m.apply(msg)
	case tea.BackgroundColorMsg:
		m.setDarkBackground(msg.IsDark())
		return m, nil
	case tea.KeyPressMsg:
		if key.Matches(msg, m.global.Quit) {
			return m, tea.Quit
		}
	}

	top := m.stack.Top()
	updated, cmd := top.Update(msg)
	m.stack.ReplaceTop(updated)
	return m, cmd
}

// View implements tea.Model.
func (m *Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "sshex"
	return v
}

func (m *Model) render() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	if m.width < minWidth || m.height < minHeight {
		message := m.styles.ScreenTitle.Render("Terminal too small") + "\n\n" +
			m.styles.ScreenBody.Render("Resize the window to at least 48x16.")
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, message)
	}

	top := m.stack.Top()
	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.header.View(),
		top.View(),
		m.footer.View(top.Keys()),
	)
}

func (m *Model) apply(action screen.ActionMsg) tea.Cmd {
	switch action.Action {
	case screen.ActionPush:
		if action.Screen == nil {
			return nil
		}
		width, height := m.contentSize()
		action.Screen.Resize(width, height)
		m.stack.Push(action.Screen)
		return action.Screen.Init()
	case screen.ActionPop:
		if _, ok := m.stack.Pop(); !ok {
			return nil
		}
		width, height := m.contentSize()
		m.stack.Top().Resize(width, height)
		return nil
	case screen.ActionQuit:
		return tea.Quit
	default:
		return nil
	}
}

func (m *Model) resize(width, height int) {
	m.width = width
	m.height = height
	m.header.Resize(width)
	m.footer.Resize(width)

	contentWidth, contentHeight := m.contentSize()
	m.stack.Top().Resize(contentWidth, contentHeight)
}

func (m *Model) contentSize() (int, int) {
	width := m.width
	height := m.height - m.header.Height() - m.footer.Height()
	return max(width, 0), max(height, 0)
}

// setDarkBackground re-resolves the theme. Every screen holds a pointer to the
// same Styles value, so changing it here updates the whole UI.
func (m *Model) setDarkBackground(dark bool) {
	resolved := theme.Load(theme.Options{
		Name:              m.themeName,
		HasDarkBackground: dark,
		NoColor:           m.noColor,
	})
	*m.styles = theme.NewStyles(resolved)
}

func defaultMenuItems(styles *theme.Styles) []menu.Item {
	return []menu.Item{
		{
			Kind:        "connect",
			Title:       "Connect",
			Description: "Open an SSH session",
			Open: func() screen.Screen {
				return placeholder.New("Connect", "Open an SSH session", styles)
			},
		},
		{
			Kind:        "sessions",
			Title:       "Sessions",
			Description: "Manage active sessions",
			Open: func() screen.Screen {
				return placeholder.New("Sessions", "Manage active sessions", styles)
			},
		},
		{
			Kind:        "tunnels",
			Title:       "Tunnels",
			Description: "Manage port forwards",
			Open: func() screen.Screen {
				return placeholder.New("Tunnels", "Manage port forwards", styles)
			},
		},
		{
			Kind:        "commands",
			Title:       "Commands",
			Description: "Manage predefined commands",
			Open: func() screen.Screen {
				return placeholder.New("Commands", "Manage predefined commands", styles)
			},
		},
		{
			Kind:        "update",
			Title:       "Update",
			Description: "Check for updates",
			Open: func() screen.Screen {
				return placeholder.New("Update", "Check for updates", styles)
			},
		},
	}
}
