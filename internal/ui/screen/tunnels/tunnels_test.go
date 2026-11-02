package tunnels

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mcnull/sshex/internal/ui/screen"
	"github.com/mcnull/sshex/internal/ui/theme"
)

type fakeSource struct {
	items      []Tunnel
	sessions   int
	noSessions bool
	err        error
	calls      int
	requested  []string
}

func (s *fakeSource) List(sessionID string) (Snapshot, error) {
	s.calls++
	s.requested = append(s.requested, sessionID)
	sessions := s.sessions
	if sessions == 0 && !s.noSessions {
		sessions = 1
	}
	return Snapshot{Tunnels: s.items, Sessions: sessions}, s.err
}

type addCall struct {
	sessionID string
	request   AddRequest
}

type removeCall struct {
	sessionID string
	tunnelID  string
}

type fakeController struct {
	addCalls    []addCall
	removeCalls []removeCall
	added       Tunnel
	addErr      error
	removeErr   error
}

func (c *fakeController) Add(sessionID string, request AddRequest) (Tunnel, error) {
	c.addCalls = append(c.addCalls, addCall{sessionID: sessionID, request: request})
	return c.added, c.addErr
}

func (c *fakeController) Remove(sessionID, tunnelID string) error {
	c.removeCalls = append(c.removeCalls, removeCall{sessionID: sessionID, tunnelID: tunnelID})
	return c.removeErr
}

func newScreen(source Source, controller Controller) *Model {
	styles := theme.NewStyles(theme.Load(theme.Options{NoColor: true}))
	m := New(Options{
		Styles:     &styles,
		Source:     source,
		Controller: controller,
		SessionID:  "s1",
		Target:     "bob@ssh01",
	})
	m.Resize(60, 12)
	return m
}

func newAllScreen(source Source, controller Controller) *Model {
	styles := theme.NewStyles(theme.Load(theme.Options{NoColor: true}))
	m := New(Options{
		Styles:     &styles,
		Source:     source,
		Controller: controller,
	})
	m.Resize(60, 12)
	return m
}

func testTunnels() []Tunnel {
	return []Tunnel{
		{
			ID: "t1", SessionID: "s1", Target: "bob@ssh01",
			LocalPort: 8080, TargetHost: "db", TargetPort: 5432,
			Direction: DirectionLocal, State: "active",
		},
		{
			ID: "t2", SessionID: "s1", Target: "bob@ssh01",
			LocalPort: 9000, TargetHost: "localhost", TargetPort: 80,
			Direction: DirectionRemote, State: "failed", Error: "boom", Stale: true,
		},
	}
}

func load(t *testing.T, m *Model) {
	t.Helper()
	cmd := m.load(false)
	if cmd == nil {
		t.Fatal("load returned no command")
	}
	m.Update(cmd())
}

func press(code rune, text string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Text: text}
}

func typeText(m *Model, text string) {
	for _, r := range text {
		m.Update(press(r, string(r)))
	}
}

func action(t *testing.T, cmd tea.Cmd) screen.ActionMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	msg, ok := cmd().(screen.ActionMsg)
	if !ok {
		t.Fatalf("message = %T, want screen.ActionMsg", cmd())
	}
	return msg
}

// firstBatch runs the first command of a tea.Batch without waiting for the
// polling tick.
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

func tunnelIDs(items []Tunnel) []string {
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	return ids
}

func TestLoadingAndEmptyStates(t *testing.T) {
	m := newScreen(&fakeSource{}, &fakeController{})
	if !strings.Contains(ansi.Strip(m.View()), "loading tunnels…") {
		t.Fatalf("loading state is missing:\n%s", m.View())
	}

	load(t, m)
	if !strings.Contains(ansi.Strip(m.View()), "no active tunnels") {
		t.Fatalf("empty state is missing:\n%s", m.View())
	}
}

func TestNoActiveSessionsShowsMessageAndHidesMutations(t *testing.T) {
	controller := &fakeController{}
	m := newAllScreen(&fakeSource{noSessions: true}, controller)
	load(t, m)

	view := ansi.Strip(m.View())
	if !strings.Contains(view, "no active sessions") {
		t.Fatalf("no-session state is missing:\n%s", view)
	}
	if strings.Contains(view, "no active tunnels") {
		t.Fatalf("no-session state is shown as an empty tunnel list:\n%s", view)
	}
	for _, binding := range m.Keys() {
		switch binding.Help().Key {
		case "a", "x":
			t.Fatalf("mutation key %q is still shown with no sessions", binding.Help().Key)
		}
	}

	opened := false
	m.addOpener = func() screen.Screen {
		opened = true
		return NewAddForm(AddFormOptions{Styles: m.styles, Controller: controller, SessionID: "s1"})
	}
	if _, cmd := m.Update(press('a', "a")); cmd != nil {
		t.Fatalf("a returned a command with no sessions: %T", cmd)
	}
	if opened {
		t.Fatal("add opener was called with no sessions")
	}
	m.Update(press('x', "x"))
	if m.confirming {
		t.Fatal("remove confirmation started with no sessions")
	}
}

func TestEmptyTunnelListWithSessionsKeepsAdd(t *testing.T) {
	m := newAllScreen(&fakeSource{sessions: 1}, &fakeController{})
	m.addOpener = func() screen.Screen {
		return NewAddForm(AddFormOptions{Styles: m.styles, Controller: &fakeController{}, SessionID: "s1"})
	}
	load(t, m)

	if view := ansi.Strip(m.View()); !strings.Contains(view, "no active tunnels") {
		t.Fatalf("empty tunnel state is missing:\n%s", view)
	}
	hasAdd := false
	for _, binding := range m.Keys() {
		if binding.Help().Key == "a" {
			hasAdd = true
		}
	}
	if !hasAdd {
		t.Fatal("add is hidden while sessions are active")
	}
}

func TestLoadsAndRendersTunnels(t *testing.T) {
	m := newScreen(&fakeSource{items: testTunnels()}, &fakeController{})
	load(t, m)

	view := ansi.Strip(m.View())
	for _, want := range []string{
		"TUNNEL", "STATE",
		"localhost:8080 > db:5432", "localhost:9000 < localhost:80",
		"active", "failed",
		"t1", "t2", "bob@ssh01", "local", "remote", "stale", "boom",
		"2 tunnels",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("view is missing %q:\n%s", want, view)
		}
	}
}

func TestSessionTargetIsFallback(t *testing.T) {
	items := testTunnels()
	items[0].Target = ""
	m := newScreen(&fakeSource{items: items}, &fakeController{})
	load(t, m)

	if view := ansi.Strip(m.View()); !strings.Contains(view, "bob@ssh01") {
		t.Fatalf("session target fallback is missing:\n%s", view)
	}
}

func TestSortsByTargetDirectionPortAndID(t *testing.T) {
	source := &fakeSource{items: []Tunnel{
		{ID: "d", Target: "b", Direction: DirectionLocal, LocalPort: 80, State: "active"},
		{ID: "b", Target: "a", Direction: DirectionRemote, LocalPort: 81, State: "active"},
		{ID: "a", Target: "a", Direction: DirectionLocal, LocalPort: 8080, State: "active"},
		{ID: "c", Target: "a", Direction: DirectionLocal, LocalPort: 80, State: "active"},
	}}
	m := newScreen(source, &fakeController{})
	load(t, m)

	if got := fmt.Sprint(tunnelIDs(m.tunnels)); got != "[c a b d]" {
		t.Fatalf("order = %v, want [c a b d]", got)
	}

	// A broker returning a different order must not reorder the rows.
	source.items = []Tunnel{
		{ID: "b", Target: "a", Direction: DirectionRemote, LocalPort: 81, State: "active"},
		{ID: "d", Target: "b", Direction: DirectionLocal, LocalPort: 80, State: "active"},
		{ID: "a", Target: "a", Direction: DirectionLocal, LocalPort: 8080, State: "active"},
		{ID: "c", Target: "a", Direction: DirectionLocal, LocalPort: 80, State: "active"},
	}
	load(t, m)

	if got := fmt.Sprint(tunnelIDs(m.tunnels)); got != "[c a b d]" {
		t.Fatalf("order after refresh = %v, want [c a b d]", got)
	}
}

func TestSelectionSurvivesRefresh(t *testing.T) {
	source := &fakeSource{items: testTunnels()}
	m := newScreen(source, &fakeController{})
	load(t, m)
	m.Update(press(tea.KeyDown, ""))

	source.items = []Tunnel{testTunnels()[1], testTunnels()[0]}
	load(t, m)

	if got := m.selectedID(); got != "t2" {
		t.Fatalf("selected = %q, want t2", got)
	}
}

func TestLoadErrorIsShown(t *testing.T) {
	m := newScreen(&fakeSource{err: errors.New("boom")}, &fakeController{})
	load(t, m)

	if !strings.Contains(ansi.Strip(m.View()), "could not load tunnels: boom") {
		t.Fatalf("load error is missing:\n%s", m.View())
	}
}

func TestRefreshErrorKeepsExistingRows(t *testing.T) {
	source := &fakeSource{items: testTunnels()}
	m := newScreen(source, &fakeController{})
	load(t, m)

	source.err = errors.New("boom")
	load(t, m)

	view := ansi.Strip(m.View())
	if !strings.Contains(view, "localhost:8080 > db:5432") {
		t.Fatal("existing tunnels disappeared after a refresh error")
	}
	if !strings.Contains(view, "could not refresh tunnels: boom") {
		t.Fatalf("refresh error is missing:\n%s", view)
	}
}

func TestTickRefreshesSilently(t *testing.T) {
	source := &fakeSource{items: testTunnels()}
	m := newScreen(source, &fakeController{})
	load(t, m)
	source.calls = 0

	_, cmd := m.Update(tickMsg{epoch: m.epoch})
	msg := firstBatch(t, cmd)
	m.Update(msg)

	if source.calls != 1 {
		t.Fatalf("source listed %d times, want 1", source.calls)
	}
	if !strings.Contains(ansi.Strip(m.View()), "localhost:8080 > db:5432") {
		t.Fatal("list disappeared during a background refresh")
	}
}

func TestStalePollTickIsIgnored(t *testing.T) {
	source := &fakeSource{items: testTunnels()}
	m := newScreen(source, &fakeController{})
	m.Update(firstBatch(t, m.Init()))
	source.calls = 0

	_, cmd := m.Update(tickMsg{epoch: m.epoch - 1})
	if cmd != nil {
		t.Fatalf("stale tick returned a command (%T), want nil", cmd)
	}
	if source.calls != 0 {
		t.Fatalf("stale tick listed %d times, want 0", source.calls)
	}
}

func TestInitRecoversFromLostLoad(t *testing.T) {
	source := &fakeSource{items: testTunnels()}
	m := newScreen(source, &fakeController{})

	// Simulate a request whose result was delivered while another screen was
	// active and therefore never reached this model.
	m.load(false)

	m.Update(firstBatch(t, m.Init()))
	if !strings.Contains(ansi.Strip(m.View()), "localhost:8080 > db:5432") {
		t.Fatalf("re-entering the screen did not reload the list:\n%s", m.View())
	}
}

func TestStaleLoadResultIsIgnored(t *testing.T) {
	source := &fakeSource{items: testTunnels()}
	m := newScreen(source, &fakeController{})
	m.inflight = false
	cmd := m.load(false)
	staleEpoch := m.loadEpoch - 1

	m.Update(loadedMsg{snapshot: Snapshot{Tunnels: []Tunnel{{ID: "stale"}}}, epoch: staleEpoch})
	if len(m.tunnels) != 0 {
		t.Fatalf("stale result was applied: %+v", m.tunnels)
	}

	m.Update(cmd())
	if len(m.tunnels) != 2 {
		t.Fatalf("current result was discarded: %+v", m.tunnels)
	}
}

func TestInitReloadsAndResumeRefreshes(t *testing.T) {
	source := &fakeSource{items: testTunnels()}
	m := newScreen(source, &fakeController{})

	m.Update(firstBatch(t, m.Init()))
	if source.calls != 1 {
		t.Fatalf("source listed %d times, want 1", source.calls)
	}

	m.Update(firstBatch(t, m.Resume()))
	if source.calls != 2 {
		t.Fatalf("source listed %d times, want 2", source.calls)
	}
}

func TestEscPopsScreen(t *testing.T) {
	m := newScreen(&fakeSource{}, &fakeController{})

	_, cmd := m.Update(press(tea.KeyEsc, ""))
	msg := action(t, cmd)
	if msg.Action != screen.ActionPop {
		t.Fatalf("action = %v, want pop", msg.Action)
	}
}

func TestAllSessionsListsEverySessionAndOpensAdd(t *testing.T) {
	source := &fakeSource{items: testTunnels()}
	m := newAllScreen(source, &fakeController{})
	load(t, m)

	if got := fmt.Sprint(source.requested); got != "[]" {
		t.Fatalf("requested sessions = %v, want one all-sessions request", got)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "localhost:8080 > db:5432") || !strings.Contains(view, "localhost:9000 < localhost:80") {
		t.Fatalf("all-sessions view is missing tunnels:\n%s", view)
	}

	m.addOpener = func() screen.Screen {
		return NewAddForm(AddFormOptions{
			Styles:     m.styles,
			Controller: &fakeController{},
			SessionID:  "s1",
			Target:     "bob@ssh01",
		})
	}
	_, cmd := m.Update(press('a', "a"))
	msg := action(t, cmd)
	if msg.Action != screen.ActionPush || msg.Screen == nil {
		t.Fatalf("action = %+v, want push", msg)
	}
	if got := msg.Screen.Title(); got != "Add tunnel" {
		t.Fatalf("screen title = %q, want Add tunnel", got)
	}
}

func TestSessionScopeAddPushesOpener(t *testing.T) {
	m := newScreen(&fakeSource{items: testTunnels()}, &fakeController{})
	load(t, m)

	opened := false
	m.addOpener = func() screen.Screen {
		opened = true
		return NewAddForm(AddFormOptions{Styles: m.styles, Controller: &fakeController{}, SessionID: "s1"})
	}
	_, cmd := m.Update(press('a', "a"))
	msg := action(t, cmd)
	if msg.Action != screen.ActionPush || msg.Screen == nil {
		t.Fatalf("action = %+v, want push", msg)
	}
	if !opened {
		t.Fatal("add opener was not called")
	}
}

func TestMoveAndScrollTracksSelection(t *testing.T) {
	items := make([]Tunnel, 10)
	for i := range items {
		items[i] = Tunnel{
			ID: fmt.Sprintf("t%02d", i), SessionID: "s1",
			LocalPort: 8000 + i, TargetHost: "localhost", TargetPort: 80,
			Direction: DirectionLocal, State: "active",
		}
	}
	m := newScreen(&fakeSource{items: items}, &fakeController{})
	m.Resize(60, 8)
	load(t, m)

	for range 4 {
		m.Update(press(tea.KeyDown, ""))
	}

	if m.selected != 4 {
		t.Fatalf("selected = %d, want 4", m.selected)
	}
	if m.offset != 2 {
		t.Fatalf("offset = %d, want 2", m.offset)
	}
	view := ansi.Strip(m.View())
	if strings.Contains(view, "t00") || !strings.Contains(view, "t02") || !strings.Contains(view, "t04") {
		t.Fatalf("view does not show the scrolled window:\n%s", view)
	}

	m.Update(press(tea.KeyUp, ""))
	if m.selected != 3 {
		t.Fatalf("selected = %d, want 3", m.selected)
	}
}

func TestRemoveRequiresConfirmation(t *testing.T) {
	controller := &fakeController{}
	m := newScreen(&fakeSource{items: testTunnels()}, controller)
	load(t, m)

	m.Update(press('x', "x"))
	if !m.confirming {
		t.Fatal("x did not start a confirmation")
	}
	if len(controller.removeCalls) != 0 {
		t.Fatalf("remove called before confirmation: %v", controller.removeCalls)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "remove t1?") || !strings.Contains(view, "stops immediately") {
		t.Fatalf("confirmation prompt is missing:\n%s", view)
	}

	_, cmd := m.Update(press(tea.KeyEnter, ""))
	if cmd == nil {
		t.Fatal("confirm returned no command")
	}
	msg, ok := cmd().(removedMsg)
	if !ok {
		t.Fatalf("message = %T, want removedMsg", cmd())
	}
	m.Update(msg)

	if len(controller.removeCalls) != 1 {
		t.Fatalf("remove calls = %v, want one", controller.removeCalls)
	}
	if got := controller.removeCalls[0]; got.sessionID != "s1" || got.tunnelID != "t1" {
		t.Fatalf("remove call = %+v, want s1/t1", got)
	}
	if m.hasTunnel("t1") {
		t.Fatal("removed tunnel is still in the list")
	}
	if !strings.Contains(ansi.Strip(m.View()), "tunnel removed.") {
		t.Fatalf("remove status is missing:\n%s", m.View())
	}
}

func TestCancelRemoveKeepsTunnel(t *testing.T) {
	controller := &fakeController{}
	m := newScreen(&fakeSource{items: testTunnels()}, controller)
	load(t, m)

	m.Update(press('x', "x"))
	m.Update(press(tea.KeyEsc, ""))

	if m.confirming || m.confirmID != "" {
		t.Fatal("esc did not cancel the confirmation")
	}
	if len(controller.removeCalls) != 0 {
		t.Fatalf("remove called after cancel: %v", controller.removeCalls)
	}
}

func TestRemoveErrorIsShown(t *testing.T) {
	controller := &fakeController{removeErr: errors.New("boom")}
	m := newScreen(&fakeSource{items: testTunnels()}, controller)
	load(t, m)

	m.Update(press('x', "x"))
	_, cmd := m.Update(press(tea.KeyEnter, ""))
	m.Update(cmd())

	if !strings.Contains(ansi.Strip(m.View()), "could not remove tunnel: boom") {
		t.Fatalf("remove error is missing:\n%s", m.View())
	}
}

func TestAllSessionsRemoveUsesTunnelSession(t *testing.T) {
	controller := &fakeController{}
	m := newAllScreen(&fakeSource{items: testTunnels()}, controller)
	load(t, m)

	m.Update(press('x', "x"))
	_, cmd := m.Update(press(tea.KeyEnter, ""))
	m.Update(cmd())

	if len(controller.removeCalls) != 1 {
		t.Fatalf("remove calls = %v, want one", controller.removeCalls)
	}
	if got := controller.removeCalls[0]; got.sessionID != "s1" || got.tunnelID != "t1" {
		t.Fatalf("remove call = %+v, want the tunnel's session s1", got)
	}
}

func TestRemoveWithoutControllerShowsStatus(t *testing.T) {
	m := newScreen(&fakeSource{items: testTunnels()}, nil)
	load(t, m)

	m.Update(press('x', "x"))
	if m.confirming {
		t.Fatal("remove started without a controller")
	}
	if !strings.Contains(ansi.Strip(m.View()), "removing tunnels is not available.") {
		t.Fatalf("missing controller status is not shown:\n%s", m.View())
	}
}

func TestViewFitsContentSize(t *testing.T) {
	m := newScreen(&fakeSource{items: testTunnels()}, &fakeController{})
	m.Resize(48, 12)
	load(t, m)

	lines := strings.Split(m.View(), "\n")
	if len(lines) != 12 {
		t.Fatalf("rendered %d lines, want 12", len(lines))
	}
	for i, line := range lines {
		if width := lipgloss.Width(line); width > 48 {
			t.Fatalf("line %d width = %d, want <= 48", i, width)
		}
	}
}
