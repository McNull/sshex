package ui

import (
	"slices"
	"sync"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"

	"github.com/mcnull/sshex/internal/ui/screen"
	"github.com/mcnull/sshex/internal/ui/screen/menu"
	"github.com/mcnull/sshex/internal/ui/theme"
)

// themeSwitcher is what the Themes menu needs from the root model.
type themeSwitcher interface {
	currentTheme() string
	themeSaveErr() error
	previewTheme(name string)
	setTheme(name string) tea.Cmd
	revertTheme()
}

// themeSaver persists theme choices off the update loop. Saves always write
// the latest choice, so quick successive picks cannot land out of order.
type themeSaver struct {
	save    func(name string) error
	mu      *sync.Mutex
	pending *atomic.Pointer[string]
	err     error
}

// themeSavedMsg reports the outcome of a background theme save.
type themeSavedMsg struct{ err error }

func (m *Model) currentTheme() string { return m.savedTheme }

func (m *Model) themeSaveErr() error { return m.themeSave.err }

// previewTheme applies name in place without persisting it. The Themes menu
// calls it as the user moves through the list.
func (m *Model) previewTheme(name string) {
	if name == m.themeName {
		return
	}
	m.themeName = name
	m.applyTheme()
}

// revertTheme restores the saved theme after a preview was abandoned.
func (m *Model) revertTheme() {
	if m.themeName == m.savedTheme {
		return
	}
	m.themeName = m.savedTheme
	m.applyTheme()
}

// setTheme switches the color theme now, marks it current and saves the choice
// in the background. The theme may already be applied from a preview.
func (m *Model) setTheme(name string) tea.Cmd {
	m.previewTheme(name)
	m.savedTheme = m.themeName
	s := &m.themeSave
	if s.save == nil {
		return nil
	}
	if s.mu == nil {
		s.mu, s.pending = &sync.Mutex{}, &atomic.Pointer[string]{}
	}
	resolved := m.themeName
	s.pending.Store(&resolved)
	save, mu, pending := s.save, s.mu, s.pending
	return func() tea.Msg {
		mu.Lock()
		defer mu.Unlock()
		return themeSavedMsg{err: save(*pending.Load())}
	}
}

// newThemesMenu lists the color themes; selecting one applies it in place and
// marks it current.
func newThemesMenu(styles *theme.Styles, themes themeSwitcher, version, updateAvailable string) screen.Screen {
	names := theme.Names()
	items := make([]menu.Item, 0, len(names))
	for _, name := range names {
		items = append(items, menu.Item{
			Kind:  "theme",
			Title: name,
			Describe: func() string {
				if name != themes.currentTheme() {
					return ""
				}
				if err := themes.themeSaveErr(); err != nil {
					return "current, not saved: " + err.Error()
				}
				return "current"
			},
			Run:     func() tea.Cmd { return themes.setTheme(name) },
			Preview: func() { themes.previewTheme(name) },
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
		Selected:        max(slices.Index(names, themes.currentTheme()), 0),
		OnBack:          themes.revertTheme,
	})
}
