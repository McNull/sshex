package ui

import (
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mcnull/sshex/internal/config"
	"github.com/mcnull/sshex/internal/sessionfile"
	"github.com/mcnull/sshex/internal/ui/component/footer"
	"github.com/mcnull/sshex/internal/ui/component/header"
	"github.com/mcnull/sshex/internal/ui/component/help"
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
	Version         string
	UpdateAvailable string
	// Origin and Client override the header identity. Empty values are resolved
	// from the local host and the current SSHEX_SESSION, respectively.
	Origin            string
	Client            string
	Theme             string
	SaveTheme         func(name string) error
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
	dark      bool
	saveTheme func(name string) error
	global    keys.Global
	header    header.Model
	footer    footer.Model
	help      help.Model
	helpOpen  bool
	stack     *screen.Stack
	width     int
	height    int
	inSession bool
	// clientOrigin and clientTarget identify the current outer session. The
	// local origin identifies the machine running the UI.
	clientOrigin string
	clientTarget string
	localOrigin  string
	// quitErr is the failure reported by the screen that asked the UI to quit.
	quitErr error
}

var _ tea.Model = (*Model)(nil)

// New builds the root model with the Client menu as its root inside a session
// and the Server menu as its root everywhere else.
func New(opts ModelOptions) *Model {
	origin, client := headerIdentity(opts.Origin, opts.Client)
	resolved := theme.Load(theme.Options{
		Name:              opts.Theme,
		HasDarkBackground: opts.HasDarkBackground,
		NoColor:           opts.NoColor,
	})
	styles := theme.NewStyles(resolved)

	inSession := insideSession()
	clientDeps := resolveScreenDeps(opts, scopeClient)
	originDeps := resolveScreenDeps(opts, scopeOrigin)

	m := &Model{
		styles:       &styles,
		themeName:    resolved.Name,
		noColor:      resolved.NoColor,
		global:       keys.DefaultGlobal(),
		header:       header.New(&styles, opts.Version, origin, client),
		footer:       footer.New(&styles),
		help:         help.New(&styles),
		inSession:    inSession,
		clientOrigin: origin,
		clientTarget: client,
		localOrigin:  config.Origin(),
		dark:         opts.HasDarkBackground,
		saveTheme:    opts.SaveTheme,
	}
	m.stack = screen.NewStack(newRootMenu(
		&styles,
		clientDeps,
		originDeps,
		opts.Version,
		opts.UpdateAvailable,
		inSession,
		m.setTheme,
	))
	m.refreshHeader()
	return m
}

// resolveScreenDeps builds one set of screens for a scope. Injected sources
// override both scopes; production defaults are scoped adapters.
func resolveScreenDeps(opts ModelOptions, scope controlScope) screenDeps {
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
		control := sessionControl{scope: scope}
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
		control := tunnelControl{scope: scope}
		if tunnelSource == nil {
			tunnelSource = control
		}
		if tunnelController == nil {
			tunnelController = control
		}
	}

	portSource := opts.PortSource
	if portSource == nil {
		portSource = portControl{scope: scope}
	}

	commandSource := opts.CommandSource
	commandController := opts.CommandController
	if commandSource == nil || commandController == nil {
		control := commandControl{scope: scope}
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

	return screenDeps{
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
	}
}

// refreshHeader matches the header title and identity to the active screen.
func (m *Model) refreshHeader() {
	m.header.SetTitle(m.stack.Top().Title())
	if !m.inSession {
		return
	}
	if m.stack.Scope() == screen.ScopeClient {
		m.header.SetIdentity(m.clientOrigin, m.clientTarget)
		return
	}
	m.header.SetIdentity(m.localOrigin, "")
}

func headerIdentity(origin, client string) (string, string) {
	if strings.TrimSpace(os.Getenv("SSHEX_SESSION")) != "" {
		if file, err := sessionfile.Discover(); err == nil {
			if origin == "" {
				origin = file.Origin
			}
			if client == "" {
				client = file.Target
			}
		}
	}
	if origin == "" {
		origin = config.Origin()
	}
	return origin, client
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
		if keys.Matches(msg, m.global.Quit) {
			return m, tea.Quit
		}
		if m.helpOpen {
			if msg.Code == tea.KeyEsc {
				m.helpOpen = false
			}
			return m, nil
		}
		canOpenHelp := true
		if availability, ok := m.stack.Top().(screen.HelpKeyAvailability); ok {
			canOpenHelp = availability.CanOpenHelp()
		}
		if canOpenHelp && keys.Matches(msg, keys.Help()) {
			m.helpOpen = true
			return m, nil
		}
	}

	top := m.stack.Top()
	updated, cmd := top.Update(msg)
	m.stack.ReplaceTop(updated)
	m.refreshHeader()
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
	if m.helpOpen {
		text := ""
		if provider, ok := top.(screen.HelpProvider); ok {
			text = provider.HelpText()
		}
		return lipgloss.JoinVertical(
			lipgloss.Left,
			m.header.View(),
			m.help.View(top.Title(), text, append(top.Keys(), m.global.Quit)),
			m.footer.ViewHelpClose(),
		)
	}
	footer := m.footer.View(top.Keys())
	if availability, ok := top.(screen.HelpKeyAvailability); ok && !availability.CanOpenHelp() {
		footer = m.footer.ViewWithoutHelp(top.Keys())
	}
	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.header.View(),
		top.View(),
		footer,
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
		m.refreshHeader()
		return action.Screen.Init()
	case screen.ActionPop:
		if _, ok := m.stack.Pop(); !ok {
			return nil
		}
		m.refreshHeader()
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
		m.refreshHeader()
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
	m.help.Resize(width, max(height-m.header.Height()-m.footer.Height(), 0))

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
	m.dark = dark
	m.applyTheme()
}

// setTheme switches the color theme and persists the choice.
func (m *Model) setTheme(name string) {
	m.themeName = name
	m.applyTheme()
	if m.saveTheme != nil {
		// ponytail: a failed save only loses the preference; surface it if
		// users report themes not sticking.
		_ = m.saveTheme(m.themeName)
	}
}

func (m *Model) applyTheme() {
	resolved := theme.Load(theme.Options{
		Name:              m.themeName,
		HasDarkBackground: m.dark,
		NoColor:           m.noColor,
	})
	m.themeName = resolved.Name
	*m.styles = theme.NewStyles(resolved)
}

// insideSession reports whether the UI was started from inside an sshex
// session. The session launcher exports SSHEX_SESSION for the remote shell, so
// its presence distinguishes a client menu from the server menu.
func insideSession() bool {
	return strings.TrimSpace(os.Getenv("SSHEX_SESSION")) != ""
}

// currentSessionTarget labels the current session's screens with the target
// recorded in its session file. An unreadable session file only costs the
// label; the session id still scopes the screens.
func currentSessionTarget() string {
	file, err := sessionfile.Discover()
	if err != nil {
		return ""
	}
	return file.Target
}

// newRootMenu builds the menus and the screens they share. Outside a session
// the Server menu is the root. Inside a session the Client menu is the root and
// its Server entry pushes a Server menu scoped to the local origin.
func newRootMenu(styles *theme.Styles, clientDeps, originDeps screenDeps, version, updateAvailable string, inSession bool, setTheme func(string)) screen.Screen {
	serverMenu := newServerMenu(styles, originDeps, version, updateAvailable, inSession, setTheme)
	if !inSession {
		return serverMenu
	}
	return newClientMenu(styles, clientDeps, serverMenu, version, updateAvailable)
}

// newServerMenu builds the screens that manage the machine running the UI.
func newServerMenu(styles *theme.Styles, deps screenDeps, version, updateAvailable string, nested bool, setTheme func(string)) screen.Screen {
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

	commandsScreen := newCommandsScreen(styles, deps)

	// Keep one Update screen so a dismissed prompt is not recreated by a trip
	// back to the menu. It still reloads and checks on every visit.
	updateScreen := updatescreen.New(updatescreen.Options{
		Styles:    styles,
		Source:    deps.updateSource,
		Checker:   deps.updateChecker,
		Installer: deps.updateInstaller,
	})

	themesMenu := newThemesMenu(styles, setTheme, version, updateAvailable)

	return menu.New(menu.Options{
		Items: []menu.Item{
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
			{
				Kind:        "themes",
				Title:       "Themes",
				Description: "Choose a color theme",
				Open:        func() screen.Screen { return themesMenu },
			},
		},
		Version:         version,
		UpdateAvailable: updateAvailable,
		Styles:          styles,
		Title:           "Server menu",
		Help:            "Choose a section to manage SSH connections and settings.",
		Nested:          nested,
		Scope:           screen.ScopeOrigin,
	})
}

// newClientMenu builds the screens that manage the current outer session.
func newClientMenu(styles *theme.Styles, deps screenDeps, serverMenu screen.Screen, version, updateAvailable string) screen.Screen {
	sessionID := currentRemoteSessionID()
	target := currentSessionTarget()

	newAddForm := func() screen.Screen {
		return tunnelscreen.NewAddForm(tunnelscreen.AddFormOptions{
			Styles:     styles,
			Controller: deps.tunnelController,
			PortSource: deps.portSource,
			SessionID:  sessionID,
			Target:     target,
		})
	}
	tunnelsScreen := tunnelscreen.New(tunnelscreen.Options{
		Styles:     styles,
		Source:     deps.tunnelSource,
		Controller: deps.tunnelController,
		SessionID:  sessionID,
		Target:     target,
		AddOpener:  newAddForm,
	})
	commandsScreen := newCommandsScreen(styles, deps)

	return menu.New(menu.Options{
		Items: []menu.Item{
			{
				Kind:        "tunnels",
				Title:       "Tunnels",
				Description: "Manage this session's port forwards",
				Open:        func() screen.Screen { return tunnelsScreen },
			},
			{
				Kind:        "commands",
				Title:       "Commands",
				Description: "Manage predefined commands",
				Open:        func() screen.Screen { return commandsScreen },
			},
			{
				Kind:        "server",
				Title:       "Server",
				Description: "Open the server menu",
				Open:        func() screen.Screen { return serverMenu },
			},
		},
		Version:         version,
		UpdateAvailable: updateAvailable,
		Styles:          styles,
		Title:           "Client menu",
		Help:            "Manage the current session or open the server menu.",
		Scope:           screen.ScopeClient,
	})
}

// newThemesMenu lists the color themes; selecting one applies it in place.
func newThemesMenu(styles *theme.Styles, setTheme func(string), version, updateAvailable string) screen.Screen {
	var items []menu.Item
	for _, name := range theme.Names() {
		items = append(items, menu.Item{
			Kind:  "theme",
			Title: name,
			Open: func() screen.Screen {
				setTheme(name)
				return nil
			},
		})
	}
	return menu.New(menu.Options{
		Items:           items,
		Version:         version,
		UpdateAvailable: updateAvailable,
		Styles:          styles,
		Title:           "Themes",
		Help:            "Choose a color theme for the interface.",
		Nested:          true,
		Scope:           screen.ScopeOrigin,
	})
}

func newCommandsScreen(styles *theme.Styles, deps screenDeps) screen.Screen {
	newCommandForm := func(opts commandscreen.FormOptions) screen.Screen {
		opts.Styles = styles
		opts.Controller = deps.commandController
		return commandscreen.NewForm(opts)
	}
	return commandscreen.New(commandscreen.Options{
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
}
