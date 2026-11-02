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

// Back returns the binding used by child screens to return to the previous
// screen.
func Back() key.Binding {
	return key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "back"),
	)
}
