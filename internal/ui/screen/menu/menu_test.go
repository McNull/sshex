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
	return New(Options{
		Items: []Item{
			{Kind: "one", Title: "One", Description: "First", Open: func() screen.Screen {
				return placeholder.New("One", "First", stylesPtr)
			}},
			{Kind: "two", Title: "Two", Description: "Second", Open: func() screen.Screen {
				return placeholder.New("Two", "Second", stylesPtr)
			}},
			{Kind: "three", Title: "Three", Description: "Third", Open: func() screen.Screen {
				return placeholder.New("Three", "Third", stylesPtr)
			}},
		},
		Version: "1.2.3",
		Styles:  stylesPtr,
		Title:   "Test menu",
		Help:    "Test help",
	})
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

func TestMenuTitleAndHelpAreCustomizable(t *testing.T) {
	m := newMenu()
	if got := m.Title(); got != "Test menu" {
		t.Fatalf("Title() = %q, want Test menu", got)
	}
	if got := m.HelpText(); got != "Test help" {
		t.Fatalf("HelpText() = %q, want Test help", got)
	}
}

func TestNestedMenuEscapesInsteadOfQuitting(t *testing.T) {
	m := newMenu()
	m.nested = true

	_, cmd := m.Update(press(tea.KeyEsc, ""))
	if cmd == nil {
		t.Fatal("esc returned no command")
	}
	msg, ok := cmd().(screen.ActionMsg)
	if !ok || msg.Action != screen.ActionPop {
		t.Fatalf("esc action = %+v, want pop", msg)
	}
	if len(m.Keys()) != 4 || m.Keys()[3].Help().Key != "esc" {
		t.Fatalf("nested menu keys = %+v, want esc back", m.Keys())
	}

	updated, cmd := m.Update(press('q', "q"))
	if cmd != nil {
		t.Fatalf("q returned %v, want no command in a nested menu", cmd)
	}
	if updated != m {
		t.Fatal("q changed the nested menu")
	}
}

func TestRunItemActsInPlace(t *testing.T) {
	ran := false
	m := New(Options{Items: []Item{{Title: "Act", Run: func() tea.Cmd {
		ran = true
		return nil
	}}}})
	if _, cmd := m.Update(press(tea.KeyEnter, "")); cmd != nil || !ran {
		t.Fatalf("ran = %v, cmd = %v; want action run and no push", ran, cmd)
	}
}

func TestMenuNavigationPreviewsHighlightedItem(t *testing.T) {
	var previewed []string
	m := New(Options{Items: []Item{
		{Title: "One", Preview: func() { previewed = append(previewed, "one") }},
		{Title: "Two", Preview: func() { previewed = append(previewed, "two") }},
		{Title: "Three", Preview: func() { previewed = append(previewed, "three") }},
	}})

	if len(previewed) != 0 {
		t.Fatalf("previews before navigation = %v, want none", previewed)
	}

	m.Update(press(tea.KeyDown, ""))
	m.Update(press(tea.KeyDown, ""))
	m.Update(press(tea.KeyUp, ""))

	want := []string{"two", "three", "two"}
	if len(previewed) != len(want) {
		t.Fatalf("previews = %v, want %v", previewed, want)
	}
	for i := range want {
		if previewed[i] != want[i] {
			t.Fatalf("previews = %v, want %v", previewed, want)
		}
	}
}

func TestMenuPreviewDoesNotRunOnSelect(t *testing.T) {
	previewed, ran := false, false
	m := New(Options{Items: []Item{{
		Title:   "Act",
		Preview: func() { previewed = true },
		Run:     func() tea.Cmd { ran = true; return nil },
	}}})
	if _, _ = m.Update(press(tea.KeyEnter, "")); !ran || previewed {
		t.Fatalf("ran = %v, previewed = %v; want run without preview", ran, previewed)
	}
}

func TestNestedMenuCallsOnBack(t *testing.T) {
	backed := false
	m := New(Options{
		Nested: true,
		OnBack: func() { backed = true },
		Items:  []Item{{Title: "One"}},
	})
	_, cmd := m.Update(press(tea.KeyEsc, ""))
	if !backed {
		t.Fatal("onBack was not called before leaving the menu")
	}
	msg, ok := cmd().(screen.ActionMsg)
	if !ok || msg.Action != screen.ActionPop {
		t.Fatalf("esc action = %+v, want pop", msg)
	}
}
