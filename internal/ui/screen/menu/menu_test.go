package menu

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mcnull/sshex/internal/ui/component/logo"
	"github.com/mcnull/sshex/internal/ui/screen"
	"github.com/mcnull/sshex/internal/ui/screen/placeholder"
	"github.com/mcnull/sshex/internal/ui/theme"
)

func newMenu() *Model {
	styles := theme.NewStyles(theme.Load(theme.Options{NoColor: true}))
	stylesPtr := &styles
	return New([]Item{
		{Kind: "one", Title: "One", Description: "First", Open: func() screen.Screen {
			return placeholder.New("One", "First", stylesPtr)
		}},
		{Kind: "two", Title: "Two", Description: "Second", Open: func() screen.Screen {
			return placeholder.New("Two", "Second", stylesPtr)
		}},
		{Kind: "three", Title: "Three", Description: "Third", Open: func() screen.Screen {
			return placeholder.New("Three", "Third", stylesPtr)
		}},
	}, "1.2.3", "", stylesPtr)
}

func press(code rune, text string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Text: text}
}

func TestMenuVersionIsBelowLogo(t *testing.T) {
	m := newMenu()
	m.Resize(80, 24)
	lines := strings.Split(ansi.Strip(m.View()), "\n")

	logoLine := -1
	for i, line := range lines {
		if strings.Contains(line, "┌──────┐") {
			logoLine = i
			break
		}
	}
	if logoLine < 0 {
		t.Fatal("logo is missing")
	}
	logoLeft := strings.Index(lines[logoLine], "┌")

	versionLine := logoLine + 5
	if versionLine >= len(lines) || !strings.Contains(lines[versionLine], "v1.2.3") {
		t.Fatalf("version is not directly below the logo: %q", lines)
	}
	if got := strings.Index(lines[versionLine], "v1.2.3"); got != logoLeft {
		t.Fatalf("version starts at column %d, logo at %d", got, logoLeft)
	}
}

func TestMenuUpdateAvailableAlignsWithLogo(t *testing.T) {
	m := newMenu()
	m.updateAvailable = "1.2.3"
	m.Resize(80, 24)
	lines := strings.Split(ansi.Strip(m.View()), "\n")

	logoLine := -1
	for i, line := range lines {
		if strings.Contains(line, "┌──────┐") {
			logoLine = i
			break
		}
	}
	if logoLine < 0 {
		t.Fatal("logo is missing")
	}
	logoLeft := strings.Index(lines[logoLine], "┌")
	logoRight := logoLeft + logo.Width - 1

	versionLine := lines[logoLine+5]
	if !strings.Contains(versionLine, "v1.2.3") || !strings.Contains(versionLine, "(update available: v1.2.3)") {
		t.Fatalf("version line = %q, want version and update indicator", versionLine)
	}
	if got := strings.Index(versionLine, ")"); got != logoRight {
		t.Fatalf("update indicator ends at column %d, logo ends at %d", got, logoRight)
	}
}

func TestMenuNavigationWraps(t *testing.T) {
	m := newMenu()
	if m.Selected() != 0 {
		t.Fatalf("initial selection = %d, want 0", m.Selected())
	}

	updated, _ := m.Update(press(tea.KeyUp, ""))
	m = updated.(*Model)
	if m.Selected() != 2 {
		t.Fatalf("after up selection = %d, want 2", m.Selected())
	}

	updated, _ = m.Update(press(tea.KeyDown, ""))
	m = updated.(*Model)
	if m.Selected() != 0 {
		t.Fatalf("after down selection = %d, want 0", m.Selected())
	}

	updated, _ = m.Update(press('j', "j"))
	m = updated.(*Model)
	if m.Selected() != 1 {
		t.Fatalf("after j selection = %d, want 1", m.Selected())
	}
}

func TestMenuSelectPushesScreen(t *testing.T) {
	m := newMenu()
	_, cmd := m.Update(press(tea.KeyEnter, ""))
	if cmd == nil {
		t.Fatal("enter returned no command")
	}
	msg, ok := cmd().(screen.ActionMsg)
	if !ok {
		t.Fatalf("enter message = %T, want screen.ActionMsg", cmd())
	}
	if msg.Action != screen.ActionPush || msg.Screen == nil {
		t.Fatalf("enter action = %+v, want push with a screen", msg)
	}
	if msg.Screen.Title() != "One" {
		t.Fatalf("pushed screen title = %q, want One", msg.Screen.Title())
	}
}

func TestMenuQuitKeys(t *testing.T) {
	for _, keyMsg := range []tea.KeyPressMsg{
		press('q', "q"),
		press(tea.KeyEsc, ""),
	} {
		m := newMenu()
		_, cmd := m.Update(keyMsg)
		if cmd == nil {
			t.Fatalf("%q returned no command", keyMsg.String())
		}
		msg, ok := cmd().(screen.ActionMsg)
		if !ok || msg.Action != screen.ActionQuit {
			t.Fatalf("%q action = %+v, want quit", keyMsg.String(), msg)
		}
	}
}

func TestMenuKeys(t *testing.T) {
	m := newMenu()
	if len(m.Keys()) != 4 {
		t.Fatalf("menu keys = %d, want 4", len(m.Keys()))
	}
}
