// Package screen defines the screen contract and the navigation stack of the
// sshex terminal UI.
package screen

import (
	tea "charm.land/bubbletea/v2"

	"github.com/mcnull/sshex/internal/ui/keys"
)

// Screen is one page in the UI. The root model composes the chrome (header and
// footer) around it and owns the stack; a screen only renders its own content
// and decides which actions to request.
type Screen interface {
	// Init returns the initial command of the screen.
	Init() tea.Cmd
	// Update handles a message and returns the possibly updated screen.
	Update(msg tea.Msg) (Screen, tea.Cmd)
	// View renders the screen content. The content must fit the size passed to
	// Resize.
	View() string
	// Title is the human-readable screen name, used for diagnostics and future
	// breadcrumbs.
	Title() string
	// Keys lists the active bindings used by the footer and contextual help.
	Keys() []keys.Binding
	// Resize tells the screen how much space it has.
	Resize(width, height int)
}

// Resumable is implemented by screens that need to refresh when they become
// the active screen again after a child screen is popped.
type Resumable interface {
	Resume() tea.Cmd
}

// HelpProvider optionally supplies explanatory text for the contextual help
// overlay. The active screen's Keys remain the source of its hotkey list.
type HelpProvider interface {
	HelpText() string
}

// HelpKeyAvailability lets text-entry screens reserve h for literal input.
type HelpKeyAvailability interface {
	CanOpenHelp() bool
}

// Action is a navigation request emitted by a screen.
type Action int

const (
	// ActionNone does nothing.
	ActionNone Action = iota
	// ActionPush pushes a new screen onto the stack.
	ActionPush
	// ActionPop removes the top screen, revealing the previous one.
	ActionPop
	// ActionReplace swaps the top screen for another one.
	ActionReplace
	// ActionQuit exits the UI.
	ActionQuit
)

// ActionMsg is the message that carries a navigation request to the root model.
type ActionMsg struct {
	Action Action
	Screen Screen
	// Err carries a failure to report after the UI exits. It is only set for
	// ActionQuit.
	Err error
}

// Push requests that s is pushed onto the stack.
func Push(s Screen) tea.Cmd {
	return func() tea.Msg {
		return ActionMsg{Action: ActionPush, Screen: s}
	}
}

// Pop requests that the current screen is removed from the stack.
func Pop() tea.Cmd {
	return func() tea.Msg {
		return ActionMsg{Action: ActionPop}
	}
}

// Replace requests that the current screen is swapped for s.
func Replace(s Screen) tea.Cmd {
	return func() tea.Msg {
		return ActionMsg{Action: ActionReplace, Screen: s}
	}
}

// Quit requests that the UI exits.
func Quit() tea.Cmd {
	return func() tea.Msg {
		return ActionMsg{Action: ActionQuit}
	}
}

// QuitWithError requests that the UI exits and reports err once the terminal
// is restored.
func QuitWithError(err error) tea.Cmd {
	return func() tea.Msg {
		return ActionMsg{Action: ActionQuit, Err: err}
	}
}
