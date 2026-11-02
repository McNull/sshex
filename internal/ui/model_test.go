package ui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mcnull/sshex/internal/ui/screen"
)

func newTestModel() *Model {
	return New(ModelOptions{
		Version:           "1.2.3",
		HasDarkBackground: true,
		NoColor:           true,
	})
}

func press(code rune, text string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Text: text}
}

func resize(t *testing.T, m *Model, width, height int) {
	t.Helper()
	m.Update(tea.WindowSizeMsg{Width: width, Height: height})
}

func run(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	return cmd()
}

func TestModelRendersFullWindow(t *testing.T) {
	m := newTestModel()
	resize(t, m, 80, 24)

	content := m.View().Content
	lines := strings.Split(content, "\n")
	if len(lines) != 24 {
		t.Fatalf("rendered %d lines, want 24", len(lines))
	}
	for i, line := range lines {
		if width := lipgloss.Width(line); width > 80 {
			t.Fatalf("line %d width = %d, want <= 80", i, width)
		}
	}
	if header := ansi.Strip(lines[0]); !strings.Contains(header, "sshex") || strings.Contains(header, "v1.2.3") {
		t.Fatalf("header = %q, want wordmark without the version", header)
	}
	if !strings.Contains(content, "┌──────┐") {
		t.Fatal("menu logo is missing")
	}
	if strings.Contains(content, "What would you like to do?") {
		t.Fatal("menu heading should have been removed")
	}
	if !strings.Contains(content, "Connect") || !strings.Contains(content, "Open an SSH session") {
		t.Fatal("menu items are missing")
	}
	if !strings.Contains(lines[len(lines)-1], "select") {
		t.Fatalf("footer = %q, want key help", lines[len(lines)-1])
	}
}

func TestModelPushesAndPopsScreen(t *testing.T) {
	m := newTestModel()
	resize(t, m, 80, 24)

	_, cmd := m.Update(press(tea.KeyEnter, ""))
	action, ok := run(t, cmd).(screen.ActionMsg)
	if !ok || action.Action != screen.ActionPush {
		t.Fatalf("enter message = %+v, want push", action)
	}
	m.Update(action)

	if m.stack.Len() != 2 {
		t.Fatalf("stack length = %d, want 2", m.stack.Len())
	}
	if !strings.Contains(m.View().Content, "loading hosts…") {
		t.Fatal("connect screen content is missing")
	}

	_, cmd = m.Update(press(tea.KeyEsc, ""))
	action, ok = run(t, cmd).(screen.ActionMsg)
	if !ok || action.Action != screen.ActionPop {
		t.Fatalf("esc message = %+v, want pop", action)
	}
	m.Update(action)

	if m.stack.Len() != 1 {
		t.Fatalf("stack length = %d, want 1", m.stack.Len())
	}
	if !strings.Contains(m.View().Content, "┌──────┐") {
		t.Fatal("menu did not return after pop")
	}
}

func TestModelOpensSessionsScreen(t *testing.T) {
	m := newTestModel()
	resize(t, m, 80, 24)

	m.Update(press(tea.KeyDown, ""))
	_, cmd := m.Update(press(tea.KeyEnter, ""))
	action := run(t, cmd).(screen.ActionMsg)
	m.Update(action)

	if title := m.stack.Top().Title(); title != "Sessions" {
		t.Fatalf("screen title = %q, want Sessions", title)
	}
	if !strings.Contains(m.View().Content, "loading sessions…") {
		t.Fatal("sessions screen content is missing")
	}
}

type resumableStub struct {
	resumed bool
}

func (s *resumableStub) Init() tea.Cmd                           { return nil }
func (s *resumableStub) Update(tea.Msg) (screen.Screen, tea.Cmd) { return s, nil }
func (s *resumableStub) View() string                            { return "child" }
func (s *resumableStub) Title() string                           { return "child" }
func (s *resumableStub) Keys() []key.Binding                     { return nil }
func (s *resumableStub) Resize(int, int)                         {}
func (s *resumableStub) Resume() tea.Cmd {
	s.resumed = true
	return nil
}

func TestModelResumesScreenAfterPop(t *testing.T) {
	m := newTestModel()
	resize(t, m, 80, 24)

	revealed := &resumableStub{}
	m.stack.Push(revealed)
	m.stack.Push(&resumableStub{})
	m.Update(screen.ActionMsg{Action: screen.ActionPop})

	if !revealed.resumed {
		t.Fatal("popping did not resume the revealed screen")
	}
}

func TestConnectStateSurvivesMenuRoundTrip(t *testing.T) {
	m := newTestModel()
	resize(t, m, 80, 24)

	_, cmd := m.Update(press(tea.KeyEnter, ""))
	action := run(t, cmd).(screen.ActionMsg)
	m.Update(action)
	first := m.stack.Top()

	m.Update(press('w', "w"))
	m.Update(press('e', "e"))

	_, cmd = m.Update(press(tea.KeyEsc, ""))
	action = run(t, cmd).(screen.ActionMsg)
	m.Update(action)
	if m.stack.Len() != 1 {
		t.Fatalf("stack length = %d, want 1", m.stack.Len())
	}

	_, cmd = m.Update(press(tea.KeyEnter, ""))
	action = run(t, cmd).(screen.ActionMsg)
	m.Update(action)

	if m.stack.Top() != first {
		t.Fatal("connect screen was recreated instead of restored")
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "search › we") {
		t.Fatal("connect search input was not restored")
	}
}

func TestModelQuitKeys(t *testing.T) {
	m := newTestModel()
	resize(t, m, 80, 24)

	_, cmd := m.Update(press('q', "q"))
	action, ok := run(t, cmd).(screen.ActionMsg)
	if !ok || action.Action != screen.ActionQuit {
		t.Fatalf("q message = %+v, want quit action", action)
	}
	if _, cmd = m.Update(action); cmd == nil {
		t.Fatal("quit action returned no command")
	}
	if _, ok := run(t, cmd).(tea.QuitMsg); !ok {
		t.Fatalf("quit action message = %T, want tea.QuitMsg", run(t, cmd))
	}

	_, cmd = m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if _, ok := run(t, cmd).(tea.QuitMsg); !ok {
		t.Fatalf("ctrl+c message = %T, want tea.QuitMsg", run(t, cmd))
	}
}

func TestModelShowsUpdateAvailable(t *testing.T) {
	m := New(ModelOptions{
		Version:           "1.2.3",
		UpdateAvailable:   "2.0.0",
		HasDarkBackground: true,
		NoColor:           true,
	})
	resize(t, m, 80, 24)
	if !strings.Contains(m.View().Content, "(update available: v2.0.0)") {
		t.Fatal("update indicator is missing")
	}
}

func TestModelShowsMinimumSizeMessage(t *testing.T) {
	for _, size := range [][2]int{{40, 10}, {48, 15}} {
		m := newTestModel()
		resize(t, m, size[0], size[1])
		if !strings.Contains(m.View().Content, "Terminal too small") {
			t.Fatalf("%dx%d small terminal message is missing", size[0], size[1])
		}
	}
}

func TestModelNoColorStyles(t *testing.T) {
	m := newTestModel()
	resize(t, m, 80, 24)
	if !m.styles.MenuRowSelected.GetReverse() {
		t.Fatal("no-color selection should use reverse video")
	}
}

func TestModelAdaptsToBackgroundColor(t *testing.T) {
	m := New(ModelOptions{Version: "1.2.3", HasDarkBackground: true})
	dark := m.styles.ScreenTitle.GetForeground()

	m.Update(tea.BackgroundColorMsg{Color: lipgloss.Color("#ffffff")})
	light := m.styles.ScreenTitle.GetForeground()
	if fmt.Sprint(dark) == fmt.Sprint(light) {
		t.Fatalf("foreground did not change after background update: %v", dark)
	}
}
