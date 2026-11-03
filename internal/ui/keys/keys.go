// Package keys holds the shared key bindings of the sshex terminal UI.
// Screens compose these bindings instead of spelling key strings themselves,
// which keeps the footer help and the actual handling in sync.
package keys

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// Binding combines an active key binding with optional expanded help text.
type Binding struct {
	key.Binding
	longHelp string
}

type bindingConfig struct {
	keys           []string
	helpKey, short string
	longHelp       string
}

// BindingOption configures an application key binding. Use keys.WithKeys to
// supply accepted keystrokes and keys.WithHelp to provide help labels.
type BindingOption interface {
	apply(*bindingConfig)
}

type bindingOptionFunc func(*bindingConfig)

func (f bindingOptionFunc) apply(config *bindingConfig) { f(config) }

// WithHelp supplies the compact footer label and optional expanded help-panel
// description for a key binding.
func WithHelp(helpKey, short string, long ...string) BindingOption {
	return bindingOptionFunc(func(config *bindingConfig) {
		config.helpKey = helpKey
		config.short = short
		if len(long) > 0 {
			config.longHelp = strings.TrimSpace(long[0])
		}
	})
}

// WithKeys sets the keys that activate a binding.
func WithKeys(names ...string) BindingOption {
	return bindingOptionFunc(func(config *bindingConfig) { config.keys = names })
}

// NewBinding creates a UI key binding from WithKeys and WithHelp options.
func NewBinding(options ...BindingOption) Binding {
	config := bindingConfig{}
	for _, option := range options {
		option.apply(&config)
	}
	binding := key.NewBinding(key.WithKeys(config.keys...), key.WithHelp(config.helpKey, config.short))
	return Binding{Binding: binding, longHelp: config.longHelp}
}

// LongHelp returns the expanded description, falling back to the compact
// footer description when no long description was supplied.
func (b Binding) LongHelp() string {
	if b.longHelp != "" {
		return b.longHelp
	}
	return b.Help().Desc
}

// Matches reports whether a keypress matches an application binding.
func Matches(press tea.KeyPressMsg, binding Binding) bool {
	return key.Matches(press, binding.Binding)
}

// Global contains bindings that work on every screen.
type Global struct {
	Quit Binding
}

// DefaultGlobal returns the bindings the root model always handles.
func DefaultGlobal() Global {
	return Global{
		Quit: NewBinding(
			WithKeys("ctrl+c"),
			WithHelp("ctrl+c", "quit", "Exit the terminal interface."),
		),
	}
}

// Help returns the binding that opens the contextual help overlay.
func Help() Binding {
	return NewBinding(
		WithKeys("h"),
		WithHelp("h", "help", "Show the available keys and actions for this screen."),
	)
}

// Menu contains the bindings of the main menu.
type Menu struct {
	Up     Binding
	Down   Binding
	Select Binding
	Quit   Binding
}

// DefaultMenu returns the main menu bindings.
func DefaultMenu() Menu {
	return Menu{
		Up: NewBinding(
			WithKeys("up", "k"),
			WithHelp("↑/k", "up", "Move the selection to the previous menu item."),
		),
		Down: NewBinding(
			WithKeys("down", "j"),
			WithHelp("↓/j", "down", "Move the selection to the next menu item."),
		),
		Select: NewBinding(
			WithKeys("enter"),
			WithHelp("enter", "select", "Open the selected section."),
		),
		Quit: NewBinding(
			WithKeys("q", "esc"),
			WithHelp("q/esc", "quit", "Exit the terminal interface."),
		),
	}
}

// Connect contains the bindings of the Connect screen.
type Connect struct {
	Up          Binding
	Down        Binding
	Select      Binding
	FocusList   Binding
	FocusSearch Binding
	Backspace   Binding
}

// DefaultConnect returns the Connect screen bindings.
func DefaultConnect() Connect {
	return Connect{
		Up: NewBinding(
			WithKeys("up", "k"),
			WithHelp("↑/k", "up", "Move the selection to the previous host."),
		),
		Down: NewBinding(
			WithKeys("down", "j"),
			WithHelp("↓/j", "down", "Move the selection to the next host."),
		),
		Select: NewBinding(
			WithKeys("enter"),
			WithHelp("enter", "connect", "Connect to the host."),
		),
		FocusList: NewBinding(
			WithKeys("tab", "shift+tab"),
			WithHelp("tab", "hosts", "Move focus to the host list."),
		),
		FocusSearch: NewBinding(
			WithKeys("tab", "shift+tab"),
			WithHelp("tab", "search", "Move focus to the host search field."),
		),
		Backspace: NewBinding(
			WithKeys("backspace"),
			WithHelp("⌫", "edit", "Return focus to search and remove the last character."),
		),
	}
}

// Sessions contains the bindings of the Sessions screen.
type Sessions struct {
	Up          Binding
	Down        Binding
	SearchUp    Binding
	SearchDown  Binding
	Open        Binding
	Select      Binding
	Close       Binding
	Refresh     Binding
	FocusList   Binding
	FocusSearch Binding
	Backspace   Binding
}

// DefaultSessions returns the Sessions screen bindings.
func DefaultSessions() Sessions {
	return Sessions{
		Up: NewBinding(
			WithKeys("up", "k"),
			WithHelp("↑/k", "up", "Move the selection to the previous active session."),
		),
		Down: NewBinding(
			WithKeys("down", "j"),
			WithHelp("↓/j", "down", "Move the selection to the next active session."),
		),
		// The search field needs j and k as text, so it only treats the arrow
		// keys as movement.
		SearchUp: NewBinding(
			WithKeys("up"),
			WithHelp("↑", "up", "Move the selection to the previous session."),
		),
		SearchDown: NewBinding(
			WithKeys("down"),
			WithHelp("↓", "down", "Move the selection to the next session."),
		),
		Open: NewBinding(
			WithKeys("enter"),
			WithHelp("enter", "tunnels", "Open the tunnels associated with the selected session."),
		),
		// Select is enter with picker wording, used when the same screen
		// chooses a session for another screen.
		Select: NewBinding(
			WithKeys("enter"),
			WithHelp("enter", "select", "Choose this session for the requested action."),
		),
		Close: NewBinding(
			WithKeys("x"),
			WithHelp("x", "close session", "Close the selected session."),
		),
		Refresh: NewBinding(
			WithKeys("r"),
			WithHelp("r", "refresh", "Reload the active session list now."),
		),
		FocusList: NewBinding(
			WithKeys("tab", "shift+tab"),
			WithHelp("tab", "sessions", "Move focus to the session list."),
		),
		FocusSearch: NewBinding(
			WithKeys("tab", "shift+tab"),
			WithHelp("tab", "search", "Move focus to the session search field."),
		),
		Backspace: NewBinding(
			WithKeys("backspace"),
			WithHelp("⌫", "edit", "Return focus to search and remove the last character."),
		),
	}
}

// Tunnels contains the bindings of the Tunnels screen.
type Tunnels struct {
	Up           Binding
	Down         Binding
	Add          Binding
	Remove       Binding
	Refresh      Binding
	Direction    Binding
	Submit       Binding
	FocusPorts   Binding
	FocusSpec    Binding
	SelectPort   Binding
	RefreshPorts Binding
	Backspace    Binding
}

// DefaultTunnels returns the Tunnels screen bindings.
func DefaultTunnels() Tunnels {
	return Tunnels{
		Up: NewBinding(
			WithKeys("up", "k"),
			WithHelp("↑/k", "up", "Move the selection to the previous tunnel."),
		),
		Down: NewBinding(
			WithKeys("down", "j"),
			WithHelp("↓/j", "down", "Move the selection to the next tunnel."),
		),
		Add: NewBinding(
			WithKeys("a"),
			WithHelp("a", "add", "Start adding a tunnel. In the all-sessions view, first choose the session."),
		),
		Remove: NewBinding(
			WithKeys("x"),
			WithHelp("x", "remove", "Remove the selected tunnel. This stops the forward immediately."),
		),
		Refresh: NewBinding(
			WithKeys("r"),
			WithHelp("r", "refresh", "Reload the tunnel list now."),
		),
		Direction: NewBinding(
			WithKeys("left", "right"),
			WithHelp("←/→", "direction", "Switch between local and remote forwarding."),
		),
		Submit: NewBinding(
			WithKeys("enter"),
			WithHelp("enter", "add", "Create the tunnel using the entered specification and selected direction."),
		),
		FocusPorts: NewBinding(
			WithKeys("tab", "shift+tab"),
			WithHelp("tab", "ports", "Move focus to the available listening ports."),
		),
		FocusSpec: NewBinding(
			WithKeys("tab", "shift+tab"),
			WithHelp("tab", "spec", "Move focus back to the tunnel specification."),
		),
		SelectPort: NewBinding(
			WithKeys("enter"),
			WithHelp("enter", "use", "Use the selected listening port in the tunnel specification."),
		),
		RefreshPorts: NewBinding(
			WithKeys("r"),
			WithHelp("r", "refresh ports", "Reload the listening ports available on the target side."),
		),
		Backspace: NewBinding(
			WithKeys("backspace"),
			WithHelp("⌫", "spec", "Return focus to the tunnel specification."),
		),
	}
}

// Commands contains the bindings of the Commands list and its add/edit form.
type Commands struct {
	Up            Binding
	Down          Binding
	Navigate      Binding
	SearchUp      Binding
	SearchDown    Binding
	Add           Binding
	Edit          Binding
	Remove        Binding
	Toggle        Binding
	Refresh       Binding
	FocusList     Binding
	FocusSearch   Binding
	Search        Binding
	NextField     Binding
	PreviousField Binding
	Save          Binding
}

// DefaultCommands returns the Commands screen bindings.
func DefaultCommands() Commands {
	return Commands{
		Up: NewBinding(
			WithKeys("up", "k"),
			WithHelp("↑/k", "up", "Move the selection to the previous command."),
		),
		Down: NewBinding(
			WithKeys("down", "j"),
			WithHelp("↓/j", "down", "Move the selection to the next command."),
		),
		// Navigate is the compact footer form of the list bindings; the actual
		// handling still uses Up/Down.
		Navigate: NewBinding(
			WithKeys("up", "k", "down", "j"),
			WithHelp("↑/k/↓/j", "navigate", "Move through the command list."),
		),
		// The search field needs j and k as text, so it only treats the arrow
		// keys as movement.
		SearchUp: NewBinding(
			WithKeys("up"),
			WithHelp("↑", "up", "Move the selection to the previous command."),
		),
		SearchDown: NewBinding(
			WithKeys("down"),
			WithHelp("↓", "down", "Move the selection to the next command."),
		),
		Add: NewBinding(
			WithKeys("a"),
			WithHelp("a", "add", "Open a form to create a command on the origin."),
		),
		Edit: NewBinding(
			WithKeys("enter"),
			WithHelp("enter", "edit", "Edit the selected command."),
		),
		Remove: NewBinding(
			WithKeys("x"),
			WithHelp("x", "remove", "Remove the selected command."),
		),
		Toggle: NewBinding(
			WithKeys("space"),
			WithHelp("space", "enable/disable", "Toggle whether the selected command can be run."),
		),
		Refresh: NewBinding(
			WithKeys("r"),
			WithHelp("r", "refresh", "Reload the command list now."),
		),
		FocusList: NewBinding(
			WithKeys("tab", "shift+tab"),
			WithHelp("tab", "commands", "Move focus to the command list."),
		),
		FocusSearch: NewBinding(
			WithKeys("tab", "shift+tab"),
			WithHelp("tab", "search", "Move focus to the command search field."),
		),
		Search: NewBinding(
			WithKeys("/"),
			WithHelp("/", "search", "Focus the search field."),
		),
		NextField: NewBinding(
			WithKeys("tab"),
			WithHelp("tab", "next field", "Move focus to the next form field."),
		),
		PreviousField: NewBinding(
			WithKeys("shift+tab"),
			WithHelp("shift+tab", "previous field", "Move focus to the previous form field."),
		),
		Save: NewBinding(
			WithKeys("enter"),
			WithHelp("enter", "save", "Validate and save the command."),
		),
	}
}

// Update contains the bindings of the Update screen.
type Update struct {
	Check   Binding
	Install Binding
	Dismiss Binding
}

// DefaultUpdate returns the Update screen bindings.
func DefaultUpdate() Update {
	return Update{
		Check: NewBinding(
			WithKeys("enter", "r"),
			WithHelp("enter/r", "check", "Check online for a newer release."),
		),
		Install: NewBinding(
			WithKeys("enter"),
			WithHelp("enter", "confirm", "Install the available update."),
		),
		Dismiss: NewBinding(
			WithKeys("esc"),
			WithHelp("esc", "dismiss", "Dismiss the update prompt."),
		),
	}
}

// Confirm and Cancel are used by confirmation prompts.
func Confirm() Binding {
	return NewBinding(
		WithKeys("enter"),
		WithHelp("enter", "confirm", "Confirm the pending action."),
	)
}

func Cancel() Binding {
	return NewBinding(
		WithKeys("esc"),
		WithHelp("esc", "cancel", "Cancel the current action and return to the previous screen."),
	)
}

// Back returns the binding used by child screens to return to the previous
// screen.
func Back() Binding {
	return NewBinding(
		WithKeys("esc"),
		WithHelp("esc", "back", "Return to the previous screen."),
	)
}
