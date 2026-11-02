// Package screen defines the screen contract and the navigation stack of the
// sshex terminal UI.
package screen

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
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
	// Keys lists the bindings shown in the footer help.
	Keys() []key.Binding
	// Resize tells the screen how much space it has.
	Resize(width, height int)
}

// Resumable is implemented by screens that need to refresh when they become
// the active screen again after a child screen is popped.
type Resumable interface {
	Resume() tea.Cmd
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
