package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mcnull/sshex/internal/ui/theme"
)

func TestSetThemeSavesLatestAndReportsErrors(t *testing.T) {
	t.Setenv("SSHEX_SESSION", "")
	var saved []string
	m := New(ModelOptions{
		HasDarkBackground: true,
		SaveTheme: func(name string) error {
			saved = append(saved, name)
			return errors.New("disk full")
		},
	})

	first := m.setTheme("ocean")
	second := m.setTheme("rose")
	// The older save runs last but must still write the newest choice.
	m.Update(run(t, second))
	m.Update(run(t, first))
	if len(saved) != 2 || saved[0] != "rose" || saved[1] != "rose" {
		t.Fatalf("saved = %v, want [rose rose]", saved)
	}

	menu := newThemesMenu(m.styles, m, "", "")
	m.stack.Push(menu)
	resize(t, m, 100, 30)
	if view := m.View().Content; !strings.Contains(view, "current, not saved: disk full") {
		t.Fatalf("save error not shown:\n%s", view)
	}
}

func TestThemesMenuPreviewsAndRevertsWithoutSaving(t *testing.T) {
	t.Setenv("SSHEX_SESSION", "")
	var saved []string
	m := New(ModelOptions{
		HasDarkBackground: true,
		SaveTheme:         func(name string) error { saved = append(saved, name); return nil },
	})
	menu := newThemesMenu(m.styles, m, "", "")
	m.stack.Push(menu)
	resize(t, m, 100, 30)

	if m.currentTheme() != theme.DefaultName {
		t.Fatalf("initial current = %q, want %q", m.currentTheme(), theme.DefaultName)
	}

	// Moving the selection previews the highlighted theme without saving it.
	m.Update(press(tea.KeyDown, ""))
	if m.themeName == m.savedTheme {
		t.Fatalf("navigation did not preview: theme=%q saved=%q", m.themeName, m.savedTheme)
	}
	if len(saved) != 0 {
		t.Fatalf("navigation saved %v, want nothing", saved)
	}

	// Leaving without enter restores the saved theme.
	m.Update(press(tea.KeyEsc, ""))
	if m.themeName != m.savedTheme {
		t.Fatalf("escape = theme %q, saved %q; want the saved theme", m.themeName, m.savedTheme)
	}
}

func TestThemesMenuEnterSavesPreview(t *testing.T) {
	t.Setenv("SSHEX_SESSION", "")
	var saved []string
	m := New(ModelOptions{
		HasDarkBackground: true,
		SaveTheme:         func(name string) error { saved = append(saved, name); return nil },
	})
	menu := newThemesMenu(m.styles, m, "", "")
	m.stack.Push(menu)
	resize(t, m, 100, 30)

	// Preview the highlighted theme, then commit it with enter.
	m.Update(press(tea.KeyDown, ""))
	previewed := m.themeName
	_, cmd := m.Update(press(tea.KeyEnter, ""))
	if cmd == nil {
		t.Fatal("enter returned no command")
	}
	m.Update(run(t, cmd))

	if m.currentTheme() != previewed {
		t.Fatalf("current after enter = %q, want %q", m.currentTheme(), previewed)
	}
	if m.themeName != m.savedTheme {
		t.Fatalf("theme = %q, saved = %q; want them equal after enter", m.themeName, m.savedTheme)
	}
	if len(saved) != 1 || saved[0] != previewed {
		t.Fatalf("saved = %v, want [%s]", saved, previewed)
	}

	// A later preview reverts to the committed theme, not the default.
	m.Update(press(tea.KeyDown, ""))
	if m.themeName == m.savedTheme {
		t.Fatal("second navigation did not preview a different theme")
	}
	m.Update(press(tea.KeyEsc, ""))
	if m.themeName != previewed {
		t.Fatalf("revert = %q, want the committed %q", m.themeName, previewed)
	}
}
