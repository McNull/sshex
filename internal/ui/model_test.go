package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mcnull/sshex/internal/ui/screen"
	"github.com/mcnull/sshex/internal/ui/screen/sessions"
	"github.com/mcnull/sshex/internal/ui/screen/tunnels"
	updatescreen "github.com/mcnull/sshex/internal/ui/screen/update"
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

func TestModelOpensCommandsScreen(t *testing.T) {
	m := newTestModel()
	resize(t, m, 80, 24)

	for range 3 {
		m.Update(press(tea.KeyDown, ""))
	}
	_, cmd := m.Update(press(tea.KeyEnter, ""))
	action := run(t, cmd).(screen.ActionMsg)
	m.Update(action)

	if title := m.stack.Top().Title(); title != "Commands" {
		t.Fatalf("screen title = %q, want Commands", title)
	}
	if !strings.Contains(m.View().Content, "loading commands…") {
		t.Fatal("commands screen content is missing")
	}
}

type fakeUpdateScreenSource struct {
	snapshot updatescreen.Snapshot
}

func (s *fakeUpdateScreenSource) Load() (updatescreen.Snapshot, error) {
	return s.snapshot, nil
}

type fakeUpdateScreenChecker struct{}

func (fakeUpdateScreenChecker) Check() (updatescreen.CheckResult, error) {
	return updatescreen.CheckResult{LatestVersion: "1.2.3", CheckedAt: time.Now()}, nil
}

type fakeUpdateScreenInstaller struct{}

func (fakeUpdateScreenInstaller) Install() (updatescreen.Result, error) {
	return updatescreen.Result{}, nil
}

func TestModelOpensUpdateScreen(t *testing.T) {
	m := New(ModelOptions{
		Version:           "1.2.3",
		HasDarkBackground: true,
		NoColor:           true,
		UpdateSource: &fakeUpdateScreenSource{snapshot: updatescreen.Snapshot{
			CurrentVersion: "1.2.3",
		}},
		UpdateChecker:   fakeUpdateScreenChecker{},
		UpdateInstaller: fakeUpdateScreenInstaller{},
	})
	resize(t, m, 80, 24)

	for range 4 {
		m.Update(press(tea.KeyDown, ""))
	}
	_, cmd := m.Update(press(tea.KeyEnter, ""))
	action := run(t, cmd).(screen.ActionMsg)
	m.Update(action)

	if title := m.stack.Top().Title(); title != "Update" {
		t.Fatalf("screen title = %q, want Update", title)
	}
	content := ansi.Strip(m.View().Content)
	if !strings.Contains(content, "reading cached update state") {
		t.Fatalf("update screen content is missing:\n%s", content)
	}
	if strings.Contains(content, "not implemented") {
		t.Fatal("update screen still uses the placeholder")
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

type fakeTunnelSource struct {
	items     []tunnels.Tunnel
	sessions  int
	requested []string
}

func (s *fakeTunnelSource) List(sessionID string) (tunnels.Snapshot, error) {
	s.requested = append(s.requested, sessionID)
	sessions := s.sessions
	if sessions == 0 {
		sessions = 1
	}
	return tunnels.Snapshot{Tunnels: s.items, Sessions: sessions}, nil
}

type fakeTunnelController struct{}

func (fakeTunnelController) Add(string, tunnels.AddRequest) (tunnels.Tunnel, error) {
	return tunnels.Tunnel{}, nil
}

func (fakeTunnelController) Remove(string, string) error { return nil }

type fakeSessionSource struct {
	items []sessions.Session
}

func (s *fakeSessionSource) List() ([]sessions.Session, error) { return s.items, nil }

type fakeSessionCloser struct{}

func (fakeSessionCloser) Close(string) error { return nil }

func firstBatch(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return msg
	}
	if len(batch) == 0 {
		t.Fatal("empty batch")
	}
	return batch[0]()
}

func TestModelOpensAllSessionTunnels(t *testing.T) {
	source := &fakeTunnelSource{items: []tunnels.Tunnel{
		{
			ID: "t1", SessionID: "s1", Target: "bob@ssh01",
			LocalPort: 8080, TargetHost: "db", TargetPort: 5432,
			Direction: tunnels.DirectionLocal, State: "active",
		},
		{
			ID: "t2", SessionID: "s2", Target: "alice@ssh02",
			LocalPort: 9000, TargetHost: "cache", TargetPort: 6379,
			Direction: tunnels.DirectionRemote, State: "active",
		},
	}}
	m := New(ModelOptions{
		Version:           "1.2.3",
		HasDarkBackground: true,
		NoColor:           true,
		TunnelSource:      source,
		TunnelController:  fakeTunnelController{},
	})
	resize(t, m, 80, 24)

	m.Update(press(tea.KeyDown, ""))
	m.Update(press(tea.KeyDown, ""))
	_, cmd := m.Update(press(tea.KeyEnter, ""))
	action := run(t, cmd).(screen.ActionMsg)
	_, pushCmd := m.Update(action)

	if title := m.stack.Top().Title(); title != "Tunnels" {
		t.Fatalf("screen title = %q, want Tunnels", title)
	}
	m.Update(firstBatch(t, pushCmd))

	view := ansi.Strip(m.View().Content)
	for _, want := range []string{
		"localhost:8080 > db:5432", "localhost:9000 < cache:6379",
		"bob@ssh01", "alice@ssh02",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("view is missing %q:\n%s", want, view)
		}
	}
	if got := fmt.Sprint(source.requested); got != "[]" {
		t.Fatalf("requested sessions = %v, want one all-sessions request", got)
	}
}

func TestMenuTunnelAddUsesPickerAndForm(t *testing.T) {
	sessionSource := &fakeSessionSource{items: []sessions.Session{
		{ID: "s1", Target: "bob@ssh01", Host: "ssh01", State: "active"},
	}}
	m := New(ModelOptions{
		Version:           "1.2.3",
		HasDarkBackground: true,
		NoColor:           true,
		SessionSource:     sessionSource,
		SessionCloser:     fakeSessionCloser{},
		TunnelSource:      &fakeTunnelSource{},
		TunnelController:  fakeTunnelController{},
	})
	resize(t, m, 80, 24)

	// Open the all-sessions tunnel list.
	m.Update(press(tea.KeyDown, ""))
	m.Update(press(tea.KeyDown, ""))
	_, cmd := m.Update(press(tea.KeyEnter, ""))
	_, tunnelCmd := m.Update(run(t, cmd).(screen.ActionMsg))
	m.Update(firstBatch(t, tunnelCmd))

	// `a` pushes the session picker.
	_, cmd = m.Update(press('a', "a"))
	action := run(t, cmd).(screen.ActionMsg)
	if action.Action != screen.ActionPush {
		t.Fatalf("a action = %v, want push", action.Action)
	}
	_, pickerCmd := m.Update(action)
	if title := m.stack.Top().Title(); title != "Select session" {
		t.Fatalf("picker title = %q, want Select session", title)
	}
	m.Update(firstBatch(t, pickerCmd))
	if m.stack.Len() != 3 {
		t.Fatalf("stack length = %d, want menu, tunnels and picker", m.stack.Len())
	}

	// Enter replaces the picker with the add form.
	_, cmd = m.Update(press(tea.KeyEnter, ""))
	action = run(t, cmd).(screen.ActionMsg)
	if action.Action != screen.ActionReplace {
		t.Fatalf("enter action = %v, want replace", action.Action)
	}
	_, formCmd := m.Update(action)
	if title := m.stack.Top().Title(); title != "Add tunnel" {
		t.Fatalf("form title = %q, want Add tunnel", title)
	}
	if m.stack.Len() != 3 {
		t.Fatalf("stack length after replace = %d, want 3", m.stack.Len())
	}
	if formCmd != nil {
		formCmd()
	}

	// Cancelling the form returns straight to the tunnel list.
	_, cmd = m.Update(press(tea.KeyEsc, ""))
	action = run(t, cmd).(screen.ActionMsg)
	if action.Action != screen.ActionPop {
		t.Fatalf("esc action = %v, want pop", action.Action)
	}
	m.Update(action)
	if title := m.stack.Top().Title(); title != "Tunnels" {
		t.Fatalf("screen after cancel = %q, want Tunnels", title)
	}
	if m.stack.Len() != 2 {
		t.Fatalf("stack length after cancel = %d, want 2", m.stack.Len())
	}
}

func TestSessionTunnelScreenIsCached(t *testing.T) {
	sessionSource := &fakeSessionSource{items: []sessions.Session{
		{ID: "s1", Target: "bob@ssh01", Host: "ssh01", State: "active"},
	}}
	tunnelSource := &fakeTunnelSource{items: []tunnels.Tunnel{
		{
			ID: "t1", SessionID: "s1", Target: "bob@ssh01",
			LocalPort: 8080, TargetHost: "db", TargetPort: 5432,
			Direction: tunnels.DirectionLocal, State: "active",
		},
	}}
	m := New(ModelOptions{
		Version:           "1.2.3",
		HasDarkBackground: true,
		NoColor:           true,
		SessionSource:     sessionSource,
		SessionCloser:     fakeSessionCloser{},
		TunnelSource:      tunnelSource,
		TunnelController:  fakeTunnelController{},
	})
	resize(t, m, 80, 24)

	// Open Sessions and load its row.
	m.Update(press(tea.KeyDown, ""))
	_, cmd := m.Update(press(tea.KeyEnter, ""))
	m.Update(run(t, cmd).(screen.ActionMsg))
	m.Update(firstBatch(t, m.stack.Top().Init()))

	// Open the session's tunnel screen.
	_, cmd = m.Update(press(tea.KeyEnter, ""))
	_, pushCmd := m.Update(run(t, cmd).(screen.ActionMsg))
	first := m.stack.Top()
	m.Update(firstBatch(t, pushCmd))
	if first.Title() != "Tunnels" {
		t.Fatalf("screen title = %q, want Tunnels", first.Title())
	}
	if !strings.Contains(ansi.Strip(first.View()), "localhost:8080 > db:5432") {
		t.Fatalf("tunnel screen did not load its session:\n%s", first.View())
	}

	// Back to Sessions, then open the same session again.
	_, cmd = m.Update(press(tea.KeyEsc, ""))
	m.Update(run(t, cmd).(screen.ActionMsg))
	_, cmd = m.Update(press(tea.KeyEnter, ""))
	m.Update(run(t, cmd).(screen.ActionMsg))

	if m.stack.Top() != first {
		t.Fatal("session tunnel screen was recreated instead of restored")
	}
	if got := fmt.Sprint(tunnelSource.requested); got != "[s1]" {
		t.Fatalf("requested sessions = %v, want [s1]", got)
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
