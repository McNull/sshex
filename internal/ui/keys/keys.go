// Package keys holds the shared key bindings of the sshex terminal UI.
// Screens compose these bindings instead of spelling key strings themselves,
// which keeps the footer help and the actual handling in sync.
package keys

import "charm.land/bubbles/v2/key"

// Global contains bindings that work on every screen.
type Global struct {
	Quit key.Binding
}

// DefaultGlobal returns the bindings the root model always handles.
func DefaultGlobal() Global {
	return Global{
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
		),
	}
}

// Menu contains the bindings of the main menu.
type Menu struct {
	Up     key.Binding
	Down   key.Binding
	Select key.Binding
	Quit   key.Binding
}

// DefaultMenu returns the main menu bindings.
func DefaultMenu() Menu {
	return Menu{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Select: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "esc"),
			key.WithHelp("q/esc", "quit"),
		),
	}
}

// Connect contains the bindings of the Connect screen.
type Connect struct {
	Up          key.Binding
	Down        key.Binding
	Select      key.Binding
	FocusList   key.Binding
	FocusSearch key.Binding
	Backspace   key.Binding
}

// DefaultConnect returns the Connect screen bindings.
func DefaultConnect() Connect {
	return Connect{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Select: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "connect"),
		),
		FocusList: key.NewBinding(
			key.WithKeys("tab", "shift+tab"),
			key.WithHelp("tab", "hosts"),
		),
		FocusSearch: key.NewBinding(
			key.WithKeys("tab", "shift+tab"),
			key.WithHelp("tab", "search"),
		),
		Backspace: key.NewBinding(
			key.WithKeys("backspace"),
			key.WithHelp("⌫", "edit"),
		),
	}
}

// Sessions contains the bindings of the Sessions screen.
type Sessions struct {
	Up          key.Binding
	Down        key.Binding
	SearchUp    key.Binding
	SearchDown  key.Binding
	Open        key.Binding
	Select      key.Binding
	Close       key.Binding
	Refresh     key.Binding
	FocusList   key.Binding
	FocusSearch key.Binding
	Backspace   key.Binding
}

// DefaultSessions returns the Sessions screen bindings.
func DefaultSessions() Sessions {
	return Sessions{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		// The search field needs j and k as text, so it only treats the arrow
		// keys as movement.
		SearchUp: key.NewBinding(
			key.WithKeys("up"),
			key.WithHelp("↑", "up"),
		),
		SearchDown: key.NewBinding(
			key.WithKeys("down"),
			key.WithHelp("↓", "down"),
		),
		Open: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "tunnels"),
		),
		// Select is enter with picker wording, used when the same screen
		// chooses a session for another screen.
		Select: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Close: key.NewBinding(
			key.WithKeys("x"),
			key.WithHelp("x", "close session"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		FocusList: key.NewBinding(
			key.WithKeys("tab", "shift+tab"),
			key.WithHelp("tab", "sessions"),
		),
		FocusSearch: key.NewBinding(
			key.WithKeys("tab", "shift+tab"),
			key.WithHelp("tab", "search"),
		),
		Backspace: key.NewBinding(
			key.WithKeys("backspace"),
			key.WithHelp("⌫", "edit"),
		),
	}
}

// Tunnels contains the bindings of the Tunnels screen.
type Tunnels struct {
	Up        key.Binding
	Down      key.Binding
	Add       key.Binding
	Remove    key.Binding
	Refresh   key.Binding
	Direction key.Binding
	Submit    key.Binding
}

// DefaultTunnels returns the Tunnels screen bindings.
func DefaultTunnels() Tunnels {
	return Tunnels{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Add: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "add tunnel"),
		),
		Remove: key.NewBinding(
			key.WithKeys("x"),
			key.WithHelp("x", "remove tunnel"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		Direction: key.NewBinding(
			key.WithKeys("left", "right"),
			key.WithHelp("←/→", "direction"),
		),
		Submit: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "add"),
		),
	}
}

// Commands contains the bindings of the Commands list and its add/edit form.
type Commands struct {
	Up            key.Binding
	Down          key.Binding
	Navigate      key.Binding
	SearchUp      key.Binding
	SearchDown    key.Binding
	Add           key.Binding
	Edit          key.Binding
	Remove        key.Binding
	AddRemove     key.Binding
	Toggle        key.Binding
	Refresh       key.Binding
	FocusList     key.Binding
	FocusSearch   key.Binding
	Search        key.Binding
	NextField     key.Binding
	PreviousField key.Binding
	Save          key.Binding
}

// DefaultCommands returns the Commands screen bindings.
func DefaultCommands() Commands {
	return Commands{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		// Navigate and AddRemove are the compact footer forms of the list
		// bindings; the actual handling still uses Up/Down and Add/Remove.
		Navigate: key.NewBinding(
			key.WithKeys("up", "k", "down", "j"),
			key.WithHelp("↑/k/↓/j", "navigate"),
		),
		// The search field needs j and k as text, so it only treats the arrow
		// keys as movement.
		SearchUp: key.NewBinding(
			key.WithKeys("up"),
			key.WithHelp("↑", "up"),
		),
		SearchDown: key.NewBinding(
			key.WithKeys("down"),
			key.WithHelp("↓", "down"),
		),
		Add: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "add command"),
		),
		Edit: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "edit"),
		),
		Remove: key.NewBinding(
			key.WithKeys("x"),
			key.WithHelp("x", "remove command"),
		),
		AddRemove: key.NewBinding(
			key.WithKeys("a", "x"),
			key.WithHelp("a/x", "add/remove command"),
		),
		Toggle: key.NewBinding(
			key.WithKeys("space"),
			key.WithHelp("space", "enable/disable"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		FocusList: key.NewBinding(
			key.WithKeys("tab", "shift+tab"),
			key.WithHelp("tab", "commands"),
		),
		FocusSearch: key.NewBinding(
			key.WithKeys("tab", "shift+tab"),
			key.WithHelp("tab", "search"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		NextField: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next field"),
		),
		PreviousField: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "previous field"),
		),
		Save: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "save"),
		),
	}
}

// Update contains the bindings of the Update screen.
type Update struct {
	Check   key.Binding
	Install key.Binding
	Dismiss key.Binding
}

// DefaultUpdate returns the Update screen bindings.
func DefaultUpdate() Update {
	return Update{
		Check: key.NewBinding(
			key.WithKeys("enter", "r"),
			key.WithHelp("enter/r", "check"),
		),
		Install: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "confirm"),
		),
		Dismiss: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "dismiss"),
		),
	}
}

// Confirm and Cancel are used by confirmation prompts.
func Confirm() key.Binding {
	return key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "confirm"),
	)
}

func Cancel() key.Binding {
	return key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "cancel"),
	)
}

// Back returns the binding used by child screens to return to the previous
// screen.
func Back() key.Binding {
	return key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "back"),
	)
}
