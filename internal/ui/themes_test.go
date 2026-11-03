package ui

import (
	"errors"
	"strings"
	"testing"
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
