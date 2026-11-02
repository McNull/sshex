package sessions

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mcnull/sshex/internal/ui/screen"
	"github.com/mcnull/sshex/internal/ui/screen/placeholder"
	"github.com/mcnull/sshex/internal/ui/theme"
)

type fakeSource struct {
	items []Session
	err   error
	calls int
}

func (s *fakeSource) List() ([]Session, error) {
	s.calls++
	return s.items, s.err
}

type fakeCloser struct {
	ids []string
	err error
}

func (c *fakeCloser) Close(id string) error {
	c.ids = append(c.ids, id)
	return c.err
}

func newScreen(source Source, closer Closer) *Model {
	styles := theme.NewStyles(theme.Load(theme.Options{NoColor: true}))
	m := New(Options{Styles: &styles, Source: source, Closer: closer})
	m.Resize(60, 12)
	return m
}

func testSessions() []Session {
	return []Session{
		{ID: "a1", Host: "ssh01", Target: "bob@ssh01", Origin: "laptop", State: "active", Connections: 2, Hops: "0"},
		{ID: "b2", Host: "ssh02", Target: "alice@ssh02", Origin: "laptop", State: "active", Connections: 1, Hops: "1", Stale: true},
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

func focusSessionList(m *Model) {
	m.Update(press(tea.KeyTab, ""))
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

func TestLoadingAndEmptyStates(t *testing.T) {
	m := newScreen(&fakeSource{}, &fakeCloser{})
	if !strings.Contains(ansi.Strip(m.View()), "loading sessions…") {
		t.Fatalf("loading state is missing:\n%s", m.View())
	}

	load(t, m)
	if !strings.Contains(ansi.Strip(m.View()), "no active sessions") {
		t.Fatalf("empty state is missing:\n%s", m.View())
	}
}

func TestLoadsAndRendersSessions(t *testing.T) {
	source := &fakeSource{items: testSessions()}
	m := newScreen(source, &fakeCloser{})
	load(t, m)

	view := ansi.Strip(m.View())
	for _, want := range []string{
		"SESSION", "STATE",
		"bob@ssh01", "alice@ssh02",
		"a1", "b2", "laptop", "2 connections", "1 connection", "0 hops", "1 hops", "stale",
		"2 sessions",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("view is missing %q:\n%s", want, view)
		}
	}
}

func TestSortsSessionsByHostname(t *testing.T) {
	source := &fakeSource{items: []Session{
		{ID: "c", Host: "ssh10", Target: "carol@ssh10", State: "active"},
		{ID: "a", Host: "ssh01", Target: "alice@ssh01", State: "active"},
		{ID: "b", Host: "SSH02", Target: "bob@ssh02", State: "active"},
	}}
	m := newScreen(source, &fakeCloser{})
	load(t, m)

	if got := sessionIDs(m.sessions); fmt.Sprint(got) != "[a b c]" {
		t.Fatalf("order = %v, want [a b c]", got)
	}

	// A broker returning a different order must not reorder the rows.
	source.items = []Session{
		{ID: "b", Host: "SSH02", Target: "bob@ssh02", State: "active"},
		{ID: "c", Host: "ssh10", Target: "carol@ssh10", State: "active"},
		{ID: "a", Host: "ssh01", Target: "alice@ssh01", State: "active"},
	}
	load(t, m)

	if got := sessionIDs(m.sessions); fmt.Sprint(got) != "[a b c]" {
		t.Fatalf("order after refresh = %v, want [a b c]", got)
	}
}

func sessionIDs(items []Session) []string {
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	return ids
}

func TestCurrentSessionIsMarked(t *testing.T) {
	items := testSessions()
	items[0].Current = true
	m := newScreen(&fakeSource{items: items}, &fakeCloser{})
	load(t, m)

	view := ansi.Strip(m.View())
	if count := strings.Count(view, "current"); count != 1 {
		t.Fatalf("view has %d current markers, want 1:\n%s", count, view)
	}
}

func TestCurrentSessionIsDefaultSelection(t *testing.T) {
	items := testSessions()
	items[1].Current = true
	m := newScreen(&fakeSource{items: items}, &fakeCloser{})
	load(t, m)

	if got := m.selectedID(); got != "b2" {
		t.Fatalf("selected = %q, want the current session b2", got)
	}
}

func TestSelectionSurvivesWhenCurrentChanges(t *testing.T) {
	source := &fakeSource{items: testSessions()}
	m := newScreen(source, &fakeCloser{})
	load(t, m)

	// The user picked the first session, then the UI learns that the second
	// one is the current session.
	source.items = testSessions()
	source.items[1].Current = true
	load(t, m)

	if got := m.selectedID(); got != "a1" {
		t.Fatalf("selected = %q, want the user's selection a1", got)
	}
}

func TestFallbackSelectionUsesCurrentSession(t *testing.T) {
	items := testSessions()
	items[0].Current = true
	source := &fakeSource{items: items}
	m := newScreen(source, &fakeCloser{})
	load(t, m)

	// The selected session disappears from the broker; the current session
	// becomes the fallback.
	source.items = []Session{items[0]}
	m.selected = 1
	load(t, m)

	if got := m.selectedID(); got != "a1" {
		t.Fatalf("selected = %q, want the current session a1", got)
	}
}

func TestSearchFiltersOnHostname(t *testing.T) {
	m := newScreen(&fakeSource{items: testSessions()}, &fakeCloser{})
	load(t, m)

	typeText(m, "SSH02")
	if len(m.filtered) != 1 || m.filtered[0].ID != "b2" {
		t.Fatalf("filtered = %+v, want only b2", m.filtered)
	}
	view := ansi.Strip(m.View())
	if strings.Contains(view, "bob@ssh01") || !strings.Contains(view, "alice@ssh02") {
		t.Fatalf("filtered view is wrong:\n%s", view)
	}
	if !strings.Contains(view, "1 of 2 sessions") {
		t.Fatalf("filtered count is missing:\n%s", view)
	}

	for range len("SSH02") {
		m.Update(press(tea.KeyBackspace, ""))
	}
	typeText(m, "zzz")
	if !strings.Contains(ansi.Strip(m.View()), "no matching sessions") {
		t.Fatalf("empty filter state is missing:\n%s", m.View())
	}
}

func TestSearchTypingDoesNotTriggerActions(t *testing.T) {
	closer := &fakeCloser{}
	m := newScreen(&fakeSource{items: testSessions()}, closer)
	load(t, m)

	typeText(m, "x")
	if m.confirming || len(closer.ids) != 0 {
		t.Fatal("typing x in the search field closed a session")
	}
	if got := m.input.Value(); got != "x" {
		t.Fatalf("search value = %q, want x", got)
	}
}

func TestTabFocusEnablesCloseAndBackspace(t *testing.T) {
	closer := &fakeCloser{}
	m := newScreen(&fakeSource{items: testSessions()}, closer)
	load(t, m)

	focusSessionList(m)
	if m.focus != focusList || m.input.Focused() {
		t.Fatal("tab did not focus the session list")
	}
	m.Update(press('x', "x"))
	if !m.confirming {
		t.Fatal("x did not start a confirmation in list focus")
	}
	m.Update(press(tea.KeyEsc, ""))

	m.Update(press(tea.KeyBackspace, ""))
	if m.focus != focusSearch || !m.input.Focused() {
		t.Fatal("backspace did not return focus to the search field")
	}
}

func TestSearchQuerySurvivesRefresh(t *testing.T) {
	source := &fakeSource{items: testSessions()}
	m := newScreen(source, &fakeCloser{})
	load(t, m)
	typeText(m, "ssh02")

	load(t, m)

	if got := m.input.Value(); got != "ssh02" {
		t.Fatalf("search value = %q, want ssh02", got)
	}
	if len(m.filtered) != 1 || m.filtered[0].ID != "b2" {
		t.Fatalf("filtered = %+v, want only b2", m.filtered)
	}
}

func TestJKAreTypedInSearch(t *testing.T) {
	m := newScreen(&fakeSource{items: testSessions()}, &fakeCloser{})
	load(t, m)

	typeText(m, "jk")
	if got := m.input.Value(); got != "jk" {
		t.Fatalf("search value = %q, want jk", got)
	}
	if m.selected != 0 {
		t.Fatalf("selected = %d, want 0", m.selected)
	}
}

func TestArrowKeysMoveSelectionWhileTyping(t *testing.T) {
	m := newScreen(&fakeSource{items: testSessions()}, &fakeCloser{})
	load(t, m)
	typeText(m, "ssh")

	m.Update(press(tea.KeyDown, ""))
	if m.selected != 1 {
		t.Fatalf("selected = %d, want 1", m.selected)
	}
	if m.focus != focusSearch {
		t.Fatal("arrow keys should not change focus")
	}
	if got := m.input.Value(); got != "ssh" {
		t.Fatalf("search value = %q, want ssh", got)
	}
}

func TestLoadErrorIsShown(t *testing.T) {
	m := newScreen(&fakeSource{err: errors.New("boom")}, &fakeCloser{})
	load(t, m)

	if !strings.Contains(ansi.Strip(m.View()), "could not load sessions: boom") {
		t.Fatalf("load error is missing:\n%s", m.View())
	}
}

func TestMoveAndScrollTracksSelection(t *testing.T) {
	items := make([]Session, 10)
	for i := range items {
		items[i] = Session{ID: fmt.Sprintf("s%02d", i), Target: fmt.Sprintf("h%02d", i), State: "active"}
	}
	m := newScreen(&fakeSource{items: items}, &fakeCloser{})
	m.Resize(60, 8)
	load(t, m)

	for range 4 {
		m.Update(press(tea.KeyDown, ""))
	}

	if m.selected != 4 {
		t.Fatalf("selected = %d, want 4", m.selected)
	}
	if m.offset != 3 {
		t.Fatalf("offset = %d, want 3", m.offset)
	}
	view := ansi.Strip(m.View())
	if strings.Contains(view, "h00") || !strings.Contains(view, "h03") || !strings.Contains(view, "h04") {
		t.Fatalf("view does not show the scrolled window:\n%s", view)
	}

	m.Update(press(tea.KeyUp, ""))
	if m.selected != 3 {
		t.Fatalf("selected = %d, want 3", m.selected)
	}
}

func TestEnterPushesSessionTunnels(t *testing.T) {
	m := newScreen(&fakeSource{items: testSessions()}, &fakeCloser{})
	var openedID, openedTarget string
	m.tunnelOpener = func(sessionID, target string) screen.Screen {
		openedID, openedTarget = sessionID, target
		return placeholder.New("Tunnels", target, m.styles)
	}
	load(t, m)
	m.Update(press(tea.KeyDown, ""))

	_, cmd := m.Update(press(tea.KeyEnter, ""))
	msg := action(t, cmd)
	if msg.Action != screen.ActionPush || msg.Screen == nil {
		t.Fatalf("action = %+v, want push", msg)
	}
	if openedID != "b2" || openedTarget != "alice@ssh02" {
		t.Fatalf("opener called with (%q, %q), want (b2, alice@ssh02)", openedID, openedTarget)
	}
	if msg.Screen.Title() != "Tunnels" {
		t.Fatalf("screen title = %q, want Tunnels", msg.Screen.Title())
	}

	msg.Screen.Resize(60, 12)
	view := ansi.Strip(msg.Screen.View())
	if !strings.Contains(view, "alice@ssh02") {
		t.Fatalf("tunnel screen does not show the selected session:\n%s", view)
	}
}

func TestPickerSelectsSession(t *testing.T) {
	m := newScreen(&fakeSource{items: testSessions()}, &fakeCloser{})
	var picked Session
	m.onSelect = func(session Session) tea.Cmd {
		picked = session
		return screen.Replace(placeholder.New("Add tunnel", session.Target, m.styles))
	}
	if got := m.Title(); got != "Select session" {
		t.Fatalf("title = %q, want Select session", got)
	}
	load(t, m)
	m.Update(press(tea.KeyDown, ""))

	_, cmd := m.Update(press(tea.KeyEnter, ""))
	msg := action(t, cmd)
	if msg.Action != screen.ActionReplace || msg.Screen == nil {
		t.Fatalf("action = %+v, want replace", msg)
	}
	if picked.ID != "b2" || picked.Target != "alice@ssh02" {
		t.Fatalf("picked = %+v, want alice@ssh02/b2", picked)
	}
	if msg.Screen.Title() != "Add tunnel" {
		t.Fatalf("screen title = %q, want Add tunnel", msg.Screen.Title())
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "select a session for the new tunnel") {
		t.Fatalf("picker hint is missing:\n%s", view)
	}
}

func TestPickerIgnoresClose(t *testing.T) {
	m := newScreen(&fakeSource{items: testSessions()}, &fakeCloser{})
	m.onSelect = func(Session) tea.Cmd { return nil }
	load(t, m)
	focusSessionList(m)

	m.Update(press('x', "x"))
	if m.confirming {
		t.Fatal("x started a close confirmation in the picker")
	}
}

func TestEnterWithoutTunnelOpenerShowsStatus(t *testing.T) {
	m := newScreen(&fakeSource{items: testSessions()}, &fakeCloser{})
	load(t, m)

	_, cmd := m.Update(press(tea.KeyEnter, ""))
	if cmd != nil {
		t.Fatalf("enter without an opener returned a command (%T), want nil", cmd)
	}
	if !strings.Contains(ansi.Strip(m.View()), "tunnels are not available.") {
		t.Fatalf("missing opener status is not shown:\n%s", m.View())
	}
}

func TestCloseRequiresConfirmation(t *testing.T) {
	closer := &fakeCloser{}
	m := newScreen(&fakeSource{items: testSessions()}, closer)
	load(t, m)
	focusSessionList(m)

	m.Update(press('x', "x"))
	if !m.confirming {
		t.Fatal("x did not start a confirmation")
	}
	if len(closer.ids) != 0 {
		t.Fatalf("close called before confirmation: %v", closer.ids)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "close bob@ssh01?") {
		t.Fatalf("confirmation prompt is missing:\n%s", view)
	}

	_, cmd := m.Update(press(tea.KeyEnter, ""))
	if cmd == nil {
		t.Fatal("confirm returned no command")
	}
	msg, ok := cmd().(closedMsg)
	if !ok {
		t.Fatalf("message = %T, want closedMsg", cmd())
	}
	m.Update(msg)

	if len(closer.ids) != 1 || closer.ids[0] != "a1" {
		t.Fatalf("closed = %v, want [a1]", closer.ids)
	}
	if m.hasSession("a1") {
		t.Fatal("closed session is still in the list")
	}
	if !strings.Contains(ansi.Strip(m.View()), "session closed.") {
		t.Fatalf("close status is missing:\n%s", m.View())
	}
}

func TestCancelCloseKeepsSession(t *testing.T) {
	closer := &fakeCloser{}
	m := newScreen(&fakeSource{items: testSessions()}, closer)
	load(t, m)
	focusSessionList(m)

	m.Update(press('x', "x"))
	m.Update(press(tea.KeyEsc, ""))

	if m.confirming || m.confirmID != "" {
		t.Fatal("esc did not cancel the confirmation")
	}
	if len(closer.ids) != 0 {
		t.Fatalf("close called after cancel: %v", closer.ids)
	}
}

func TestCloseErrorIsShown(t *testing.T) {
	closer := &fakeCloser{err: errors.New("boom")}
	m := newScreen(&fakeSource{items: testSessions()}, closer)
	load(t, m)
	focusSessionList(m)

	m.Update(press('x', "x"))
	_, cmd := m.Update(press(tea.KeyEnter, ""))
	m.Update(cmd())

	if !strings.Contains(ansi.Strip(m.View()), "could not close session: boom") {
		t.Fatalf("close error is missing:\n%s", m.View())
	}
}

func TestRefreshPreservesSelection(t *testing.T) {
	source := &fakeSource{items: testSessions()}
	m := newScreen(source, &fakeCloser{})
	load(t, m)
	m.Update(press(tea.KeyDown, ""))

	source.items = []Session{testSessions()[1], testSessions()[0]}
	load(t, m)

	if got := m.selectedID(); got != "b2" {
		t.Fatalf("selected = %q, want b2", got)
	}
}

func TestTickRefreshesSilently(t *testing.T) {
	source := &fakeSource{items: testSessions()}
	m := newScreen(source, &fakeCloser{})
	load(t, m)
	source.calls = 0

	_, cmd := m.Update(tickMsg{epoch: m.epoch})
	msg := firstBatch(t, cmd)
	m.Update(msg)

	if source.calls != 1 {
		t.Fatalf("source listed %d times, want 1", source.calls)
	}
	if !strings.Contains(ansi.Strip(m.View()), "bob@ssh01") {
		t.Fatal("list disappeared during a background refresh")
	}
}

func TestInitRecoversFromLostLoad(t *testing.T) {
	source := &fakeSource{items: testSessions()}
	m := newScreen(source, &fakeCloser{})

	// Simulate a request whose result was delivered while another screen was
	// active and therefore never reached this model.
	m.load(false)

	m.Update(firstBatch(t, m.Init()))
	if !strings.Contains(ansi.Strip(m.View()), "bob@ssh01") {
		t.Fatalf("re-entering the screen did not reload the list:\n%s", m.View())
	}
}

func TestStaleLoadResultIsIgnored(t *testing.T) {
	source := &fakeSource{items: testSessions()}
	m := newScreen(source, &fakeCloser{})
	m.inflight = false
	cmd := m.load(false)
	staleEpoch := m.loadEpoch - 1

	m.Update(loadedMsg{sessions: []Session{{ID: "stale"}}, epoch: staleEpoch})
	if len(m.sessions) != 0 {
		t.Fatalf("stale result was applied: %+v", m.sessions)
	}

	m.Update(cmd())
	if len(m.sessions) != 2 {
		t.Fatalf("current result was discarded: %+v", m.sessions)
	}
}

func TestStalePollTickIsIgnored(t *testing.T) {
	source := &fakeSource{items: testSessions()}
	m := newScreen(source, &fakeCloser{})
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

func TestInitReloadsAndResumeRefreshes(t *testing.T) {
	source := &fakeSource{items: testSessions()}
	m := newScreen(source, &fakeCloser{})

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
	m := newScreen(&fakeSource{}, &fakeCloser{})

	_, cmd := m.Update(press(tea.KeyEsc, ""))
	msg := action(t, cmd)
	if msg.Action != screen.ActionPop {
		t.Fatalf("action = %v, want pop", msg.Action)
	}
}

func TestListRefreshErrorKeepsExistingRows(t *testing.T) {
	source := &fakeSource{items: testSessions()}
	m := newScreen(source, &fakeCloser{})
	load(t, m)

	source.err = errors.New("boom")
	load(t, m)

	view := ansi.Strip(m.View())
	if !strings.Contains(view, "bob@ssh01") {
		t.Fatal("existing sessions disappeared after a refresh error")
	}
	if !strings.Contains(view, "could not refresh sessions: boom") {
		t.Fatalf("refresh error is missing:\n%s", view)
	}
}

func TestViewFitsContentSize(t *testing.T) {
	m := newScreen(&fakeSource{items: testSessions()}, &fakeCloser{})
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
