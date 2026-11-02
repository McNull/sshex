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
	Up      key.Binding
	Down    key.Binding
	Open    key.Binding
	Close   key.Binding
	Refresh key.Binding
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
		Open: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "tunnels"),
		),
		Close: key.NewBinding(
			key.WithKeys("x"),
			key.WithHelp("x", "close session"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
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
