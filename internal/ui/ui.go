// Package ui implements the interactive terminal interface of sshex.
//
// All rendering, navigation, layout and theming live here. The cmd package only
// provides the small Cobra adapter that calls Run.
package ui

import (
	"errors"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/mcnull/sshex/internal/ui/theme"
)

// ErrNotTerminal is returned when the UI is started without an interactive
// terminal.
var ErrNotTerminal = errors.New("the interactive UI requires a terminal")

// Themes lists the available theme names.
func Themes() []string {
	return theme.Names()
}

// Options configures one UI run.
type Options struct {
	// Version is shown below the logo.
	Version string
	// UpdateAvailable is shown next to the version when a newer release is
	// cached. Empty hides it.
	UpdateAvailable string
	// Theme selects the color theme. Empty uses the default theme.
	Theme string
	// In and Out default to os.Stdin and os.Stdout.
	In  io.Reader
	Out io.Writer
	// NoColor overrides NO_COLOR terminal detection. It exists for tests;
	// production callers leave it nil.
	NoColor *bool
}

// Run starts the UI and blocks until it exits.
func Run(opts Options) error {
	in := opts.In
	if in == nil {
		in = os.Stdin
	}
	out := opts.Out
	if out == nil {
		out = os.Stdout
	}

	if !isTerminal(in) || !isTerminal(out) {
		return ErrNotTerminal
	}

	model := New(ModelOptions{
		Version:           opts.Version,
		UpdateAvailable:   opts.UpdateAvailable,
		Theme:             opts.Theme,
		HasDarkBackground: true,
		NoColor:           noColor(opts),
	})

	program := tea.NewProgram(
		model,
		tea.WithInput(in),
		tea.WithOutput(out),
	)
	final, err := program.Run()
	if err != nil {
		return err
	}
	// A screen can quit with a failure, for example when an SSH session ends
	// with an error. Report it once the terminal is restored.
	if m, ok := final.(*Model); ok {
		return m.Err()
	}
	return nil
}

func noColor(opts Options) bool {
	if opts.NoColor != nil {
		return *opts.NoColor
	}
	return os.Getenv("NO_COLOR") != ""
}

// isTerminal reports whether v is backed by a character device.
func isTerminal(v any) bool {
	file, ok := v.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
