package tunnels

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mcnull/sshex/internal/ui/screen"
	"github.com/mcnull/sshex/internal/ui/theme"
)

func newScreen() *Model {
	styles := theme.NewStyles(theme.Load(theme.Options{NoColor: true}))
	m := New(Options{Styles: &styles, SessionID: "s1", Target: "bob@ssh01"})
	m.Resize(60, 12)
	return m
}

func TestViewShowsSession(t *testing.T) {
	m := newScreen()
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Tunnels") || !strings.Contains(view, "bob@ssh01") {
		t.Fatalf("view is missing the session context:\n%s", view)
	}
	if !strings.Contains(view, "not implemented") {
		t.Fatalf("placeholder text is missing:\n%s", view)
	}
}

func TestEscPopsScreen(t *testing.T) {
	m := newScreen()
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esc returned no command")
	}
	msg, ok := cmd().(screen.ActionMsg)
	if !ok || msg.Action != screen.ActionPop {
		t.Fatalf("action = %+v, want pop", msg)
	}
}
