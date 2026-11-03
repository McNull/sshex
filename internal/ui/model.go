package ui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mcnull/sshex/internal/ui/component/footer"
	"github.com/mcnull/sshex/internal/ui/component/header"
	"github.com/mcnull/sshex/internal/ui/keys"
	"github.com/mcnull/sshex/internal/ui/screen"
	commandscreen "github.com/mcnull/sshex/internal/ui/screen/commands"
	"github.com/mcnull/sshex/internal/ui/screen/connect"
	"github.com/mcnull/sshex/internal/ui/screen/menu"
	"github.com/mcnull/sshex/internal/ui/screen/sessions"
	tunnelscreen "github.com/mcnull/sshex/internal/ui/screen/tunnels"
	updatescreen "github.com/mcnull/sshex/internal/ui/screen/update"
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
	// HostSource supplies the Connect screen. Nil uses the local SSH files.
	HostSource connect.HostSource
	// Runner starts sessions from the Connect screen. Nil runs this binary.
	Runner connect.Runner
	// SessionSource supplies the Sessions screen. Nil uses the live broker.
	SessionSource sessions.Source
	// SessionCloser closes sessions from the Sessions screen. Nil uses the
	// live broker.
	SessionCloser sessions.Closer
	// TunnelSource supplies the Tunnels screens. Nil uses the live broker.
	TunnelSource tunnelscreen.Source
	// TunnelController adds and removes tunnels. Nil uses the live broker.
	TunnelController tunnelscreen.Controller
	// PortSource supplies the listening ports shown by the add form. Nil uses
	// the live broker.
	PortSource tunnelscreen.PortSource
	// CommandSource supplies the Commands screen. Nil uses the live broker or
	// the local config.
	CommandSource commandscreen.Source
	// CommandController mutates commands. Nil uses the live broker or the local
	// config.
	CommandController commandscreen.Controller
	// UpdateSource supplies the cached update state. Nil reads version.yml.
	UpdateSource updatescreen.Source
	// UpdateChecker performs online version checks. Nil uses the release
	// updater.
	UpdateChecker updatescreen.Checker
	// UpdateInstaller installs the latest release. Nil uses the release updater
	// and this executable.
	UpdateInstaller updatescreen.Installer
}

// screenDeps carries the injectable sources and controllers used by the menu
// screens. Keeping them in one value avoids a long positional parameter list.
type screenDeps struct {
	connectSource     connect.HostSource
	connectRunner     connect.Runner
	sessionSource     sessions.Source
	sessionCloser     sessions.Closer
	tunnelSource      tunnelscreen.Source
	tunnelController  tunnelscreen.Controller
	portSource        tunnelscreen.PortSource
	commandSource     commandscreen.Source
	commandController commandscreen.Controller
	updateSource      updatescreen.Source
	updateChecker     updatescreen.Checker
	updateInstaller   updatescreen.Installer
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
	// quitErr is the failure reported by the screen that asked the UI to quit.
	quitErr error
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

	source := opts.HostSource
	if source == nil {
		source = sshHostSource{}
	}
	runner := opts.Runner
	if runner == nil {
		runner = connect.NewExecRunner()
	}

	sessionSource := opts.SessionSource
	sessionCloser := opts.SessionCloser
	if sessionSource == nil || sessionCloser == nil {
		control := sessionControl{}
		if sessionSource == nil {
			sessionSource = control
		}
		if sessionCloser == nil {
			sessionCloser = control
		}
	}

	tunnelSource := opts.TunnelSource
	tunnelController := opts.TunnelController
	if tunnelSource == nil || tunnelController == nil {
		control := tunnelControl{}
		if tunnelSource == nil {
			tunnelSource = control
		}
		if tunnelController == nil {
			tunnelController = control
		}
	}

	portSource := opts.PortSource
	if portSource == nil {
		portSource = portControl{}
	}

	commandSource := opts.CommandSource
	commandController := opts.CommandController
	if commandSource == nil || commandController == nil {
		control := commandControl{}
		if commandSource == nil {
			commandSource = control
		}
		if commandController == nil {
			commandController = control
		}
	}

	updateSource := opts.UpdateSource
	updateChecker := opts.UpdateChecker
	updateInstaller := opts.UpdateInstaller
	if updateSource == nil || updateChecker == nil || updateInstaller == nil {
		control := newUpdateControl(opts.Version)
		if updateSource == nil {
			updateSource = control
		}
		if updateChecker == nil {
			updateChecker = control
		}
		if updateInstaller == nil {
			updateInstaller = control
		}
	}

	m := &Model{
		styles:    &styles,
		themeName: resolved.Name,
		noColor:   resolved.NoColor,
		global:    keys.DefaultGlobal(),
		header:    header.New(&styles),
		footer:    footer.New(&styles),
		stack: screen.NewStack(menu.New(
			defaultMenuItems(&styles, screenDeps{
				connectSource:     source,
				connectRunner:     runner,
				sessionSource:     sessionSource,
				sessionCloser:     sessionCloser,
				tunnelSource:      tunnelSource,
				tunnelController:  tunnelController,
				portSource:        portSource,
				commandSource:     commandSource,
				commandController: commandController,
				updateSource:      updateSource,
				updateChecker:     updateChecker,
				updateInstaller:   updateInstaller,
			}),
			opts.Version,
			opts.UpdateAvailable,
			&styles,
		)),
	}
	return m
}

// Err reports the failure that ended the UI, if any. It is only meaningful
// after the program has exited.
func (m *Model) Err() error {
	return m.quitErr
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
		top := m.stack.Top()
		top.Resize(width, height)
		if resumable, ok := top.(screen.Resumable); ok {
			return resumable.Resume()
		}
		return nil
	case screen.ActionReplace:
		if action.Screen == nil {
			return nil
		}
		width, height := m.contentSize()
		action.Screen.Resize(width, height)
		m.stack.ReplaceTop(action.Screen)
		return action.Screen.Init()
	case screen.ActionQuit:
		m.quitErr = action.Err
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

func defaultMenuItems(styles *theme.Styles, deps screenDeps) []menu.Item {
	// Keep one Connect screen for the lifetime of the UI so its query,
	// selection and scroll position survive a trip back to the menu.
	connectScreen := connect.New(connect.Options{
		Styles: styles,
		Source: deps.connectSource,
		Runner: deps.connectRunner,
	})

	// The add form is the same screen from both tunnel scopes. The menu flow
	// chooses the session first; the session-scoped flow already knows it.
	newAddForm := func(sessionID, target string) screen.Screen {
		return tunnelscreen.NewAddForm(tunnelscreen.AddFormOptions{
			Styles:     styles,
			Controller: deps.tunnelController,
			PortSource: deps.portSource,
			SessionID:  sessionID,
			Target:     target,
		})
	}

	// The picker reuses the Sessions screen, whose current-session selection
	// makes it a single enter away from the add form inside a remote shell.
	sessionPicker := sessions.New(sessions.Options{
		Styles: styles,
		Source: deps.sessionSource,
		Closer: deps.sessionCloser,
		OnSelect: func(session sessions.Session) tea.Cmd {
			return screen.Replace(newAddForm(session.ID, session.Target))
		},
	})

	// One cached tunnel screen per session keeps its selection across a trip
	// back to Sessions.
	sessionTunnels := make(map[string]screen.Screen)
	openSessionTunnels := func(sessionID, target string) screen.Screen {
		if cached, ok := sessionTunnels[sessionID]; ok {
			return cached
		}
		opened := tunnelscreen.New(tunnelscreen.Options{
			Styles:     styles,
			Source:     deps.tunnelSource,
			Controller: deps.tunnelController,
			SessionID:  sessionID,
			Target:     target,
			AddOpener:  func() screen.Screen { return newAddForm(sessionID, target) },
		})
		sessionTunnels[sessionID] = opened
		return opened
	}
	allTunnelsScreen := tunnelscreen.New(tunnelscreen.Options{
		Styles:     styles,
		Source:     deps.tunnelSource,
		Controller: deps.tunnelController,
		AddOpener:  func() screen.Screen { return sessionPicker },
	})

	// Likewise keep the Sessions screen so its selection survives a menu
	// round-trip; it reloads its data whenever it is opened.
	sessionsScreen := sessions.New(sessions.Options{
		Styles:       styles,
		Source:       deps.sessionSource,
		Closer:       deps.sessionCloser,
		TunnelOpener: openSessionTunnels,
	})

	// The list owns the current command set; the composition root builds the
	// add/edit form and receives that set for uniqueness checks.
	newCommandForm := func(opts commandscreen.FormOptions) screen.Screen {
		opts.Styles = styles
		opts.Controller = deps.commandController
		return commandscreen.NewForm(opts)
	}
	commandsScreen := commandscreen.New(commandscreen.Options{
		Styles:     styles,
		Source:     deps.commandSource,
		Controller: deps.commandController,
		AddOpener: func(existing []commandscreen.Command) screen.Screen {
			return newCommandForm(commandscreen.FormOptions{Existing: existing})
		},
		EditOpener: func(existing []commandscreen.Command, original commandscreen.Command) screen.Screen {
			return newCommandForm(commandscreen.FormOptions{Existing: existing, Original: &original})
		},
	})

	// Keep one Update screen so a dismissed prompt is not recreated by a trip
	// back to the menu. It still reloads and checks on every visit.
	updateScreen := updatescreen.New(updatescreen.Options{
		Styles:    styles,
		Source:    deps.updateSource,
		Checker:   deps.updateChecker,
		Installer: deps.updateInstaller,
	})

	return []menu.Item{
		{
			Kind:        "connect",
			Title:       "Connect",
			Description: "Open an SSH session",
			Open:        func() screen.Screen { return connectScreen },
		},
		{
			Kind:        "sessions",
			Title:       "Sessions",
			Description: "Manage active sessions",
			Open:        func() screen.Screen { return sessionsScreen },
		},
		{
			Kind:        "tunnels",
			Title:       "Tunnels",
			Description: "Manage port forwards",
			Open:        func() screen.Screen { return allTunnelsScreen },
		},
		{
			Kind:        "commands",
			Title:       "Commands",
			Description: "Manage predefined commands",
			Open:        func() screen.Screen { return commandsScreen },
		},
		{
			Kind:        "update",
			Title:       "Update",
			Description: "Check for updates",
			Open:        func() screen.Screen { return updateScreen },
		},
	}
}
