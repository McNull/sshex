package connect

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mcnull/sshex/internal/sshhosts"
	"github.com/mcnull/sshex/internal/ui/screen"
	"github.com/mcnull/sshex/internal/ui/theme"
)

type fakeSource struct {
	hosts []sshhosts.Host
	err   error
}

func (s fakeSource) Resolve() ([]sshhosts.Host, error) {
	return s.hosts, s.err
}

type countingSource struct {
	hosts []sshhosts.Host
	calls int
}

func (s *countingSource) Resolve() ([]sshhosts.Host, error) {
	s.calls++
	return s.hosts, nil
}

type fakeRunner struct {
	targets []string
	err     error
}

func (r *fakeRunner) Run(target string) tea.Cmd {
	r.targets = append(r.targets, target)
	err := r.err
	return func() tea.Msg { return FinishedMsg{Err: err} }
}

func newScreen(source HostSource, runner Runner) *Model {
	styles := theme.NewStyles(theme.Load(theme.Options{NoColor: true}))
	m := New(Options{Styles: &styles, Source: source, Runner: runner})
	m.Resize(60, 12)
	return m
}

func testHosts() []sshhosts.Host {
	return []sshhosts.Host{
		{Name: "db", Description: "db.example.com"},
		{Name: "host1"},
		{Name: "web", Description: "web.example.com"},
	}
}

func load(t *testing.T, m *Model) {
	t.Helper()
	m.Update(m.loadHosts()())
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

func TestLoadsAndFiltersHosts(t *testing.T) {
	m := newScreen(fakeSource{hosts: testHosts()}, &fakeRunner{})
	load(t, m)

	if len(m.filtered) != 3 {
		t.Fatalf("filtered = %d, want 3", len(m.filtered))
	}
	if !strings.Contains(m.View(), "3 of 3 hosts") {
		t.Fatalf("view is missing the host count:\n%s", m.View())
	}

	typeText(m, "WE")

	if len(m.filtered) != 1 || m.filtered[0].Name != "web" {
		t.Fatalf("filtered = %v, want web", m.filtered)
	}
	view := m.View()
	if !strings.Contains(view, "web") || strings.Contains(view, "host1") {
		t.Fatalf("filtered view is wrong:\n%s", view)
	}
	if !strings.Contains(view, "1 of 3 hosts") {
		t.Fatalf("view is missing the filtered count:\n%s", view)
	}
}

func TestTypingExactMatchSelectsItFirst(t *testing.T) {
	m := newScreen(fakeSource{hosts: []sshhosts.Host{
		{Name: "alpha.nullest.com"},
		{Name: "nullest.com"},
		{Name: "zeta.nullest.com"},
	}}, &fakeRunner{})
	load(t, m)

	typeText(m, "nullest.com")

	if m.selected != 0 || m.selectedName() != "nullest.com" {
		t.Fatalf("selected = %q, want nullest.com first", m.selectedName())
	}
	if got := m.filtered[0].Name; got != "nullest.com" {
		t.Fatalf("first host = %q, want nullest.com", got)
	}
}

func TestEnterUsesFreeFormTarget(t *testing.T) {
	runner := &fakeRunner{}
	m := newScreen(fakeSource{hosts: testHosts()}, runner)
	load(t, m)

	typeText(m, "user@example.com")
	_, cmd := m.Update(press(tea.KeyEnter, ""))

	if len(runner.targets) != 1 || runner.targets[0] != "user@example.com" {
		t.Fatalf("targets = %v, want the typed target", runner.targets)
	}
	m.Update(cmd())
	if !strings.Contains(m.View(), "session ended.") {
		t.Fatalf("session result is missing:\n%s", m.View())
	}
}

func TestEmptyEnterConnectsSelectedHost(t *testing.T) {
	runner := &fakeRunner{}
	m := newScreen(fakeSource{hosts: testHosts()}, runner)
	load(t, m)

	m.Update(press(tea.KeyDown, ""))
	if m.selected != 1 {
		t.Fatalf("selected = %d, want 1 after moving in the search field", m.selected)
	}
	m.Update(press(tea.KeyEnter, ""))

	if len(runner.targets) != 1 || runner.targets[0] != "host1" {
		t.Fatalf("targets = %v, want host1", runner.targets)
	}
}

func TestSearchFocusArrowsMoveSelection(t *testing.T) {
	m := newScreen(fakeSource{hosts: testHosts()}, &fakeRunner{})
	load(t, m)

	m.Update(press(tea.KeyDown, ""))
	m.Update(press(tea.KeyDown, ""))
	if m.selected != 2 {
		t.Fatalf("selected = %d, want 2", m.selected)
	}

	m.Update(press(tea.KeyUp, ""))
	if m.selected != 1 {
		t.Fatalf("selected = %d, want 1", m.selected)
	}
	if m.focus != focusSearch {
		t.Fatal("arrow keys should not change focus")
	}
}

func TestEmptyEnterWithNoHostsDoesNothing(t *testing.T) {
	runner := &fakeRunner{}
	m := newScreen(fakeSource{}, runner)
	load(t, m)

	_, cmd := m.Update(press(tea.KeyEnter, ""))
	if cmd != nil {
		t.Fatalf("empty enter returned a command (%T), want nil", cmd)
	}
	if len(runner.targets) != 0 {
		t.Fatalf("targets = %v, want none", runner.targets)
	}
}

func TestTabTogglesFocusAndListConnect(t *testing.T) {
	runner := &fakeRunner{}
	m := newScreen(fakeSource{hosts: testHosts()}, runner)
	load(t, m)

	m.Update(press(tea.KeyTab, ""))
	if m.focus != focusList {
		t.Fatal("tab did not focus the host list")
	}
	if m.input.Focused() {
		t.Fatal("search input should be blurred while the list has focus")
	}

	m.Update(press(tea.KeyDown, ""))
	m.Update(press(tea.KeyEnter, ""))
	if len(runner.targets) != 1 || runner.targets[0] != "host1" {
		t.Fatalf("targets = %v, want host1", runner.targets)
	}

	m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if m.focus != focusSearch || !m.input.Focused() {
		t.Fatal("shift+tab did not return focus to the search input")
	}
}

func TestEscPopsScreen(t *testing.T) {
	m := newScreen(fakeSource{}, &fakeRunner{})

	_, cmd := m.Update(press(tea.KeyEsc, ""))
	msg := action(t, cmd)
	if msg.Action != screen.ActionPop {
		t.Fatalf("action = %v, want pop", msg.Action)
	}
}

func TestListScrollsWithSelection(t *testing.T) {
	hosts := make([]sshhosts.Host, 30)
	for i := range hosts {
		hosts[i] = sshhosts.Host{Name: fmt.Sprintf("h%02d", i)}
	}
	m := newScreen(fakeSource{hosts: hosts}, &fakeRunner{})
	load(t, m)
	m.Update(press(tea.KeyTab, ""))

	for range 10 {
		m.Update(press(tea.KeyDown, ""))
	}

	if m.selected != 10 {
		t.Fatalf("selected = %d, want 10", m.selected)
	}
	if m.offset != 2 {
		t.Fatalf("offset = %d, want 2", m.offset)
	}
	view := m.View()
	if strings.Contains(view, "h00") || !strings.Contains(view, "h09") {
		t.Fatalf("view does not show the scrolled window:\n%s", view)
	}
}

func TestListHeaderIsStickyAndAligned(t *testing.T) {
	m := newScreen(fakeSource{hosts: testHosts()}, &fakeRunner{})
	load(t, m)

	lines := strings.Split(ansi.Strip(m.View()), "\n")
	header := lines[1]
	nameColumn := strings.Index(header, "HOST")
	descColumn := strings.Index(header, "HOSTNAME")
	if nameColumn < 0 || descColumn < 0 {
		t.Fatalf("header = %q, want HOST and HOSTNAME columns", header)
	}

	firstRow := lines[2]
	if got := lipgloss.Width(firstRow[:strings.Index(firstRow, "db")]); got != nameColumn {
		t.Fatalf("host name starts at %d, header at %d", got, nameColumn)
	}
	if got := lipgloss.Width(firstRow[:strings.Index(firstRow, "db.example.com")]); got != descColumn {
		t.Fatalf("description starts at %d, header at %d", got, descColumn)
	}
}

func TestDisplayDescriptionHidesRepeatedName(t *testing.T) {
	cases := []struct {
		name string
		host sshhosts.Host
		want string
	}{
		{"same", sshhosts.Host{Name: "web", Description: "web"}, ""},
		{"same ignoring case", sshhosts.Host{Name: "web", Description: "WEB"}, ""},
		{"different", sshhosts.Host{Name: "web", Description: "web.example.com"}, "web.example.com"},
		{"empty", sshhosts.Host{Name: "web"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := displayDescription(tc.host); got != tc.want {
				t.Fatalf("displayDescription() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestViewFitsContentSize(t *testing.T) {
	m := newScreen(fakeSource{hosts: testHosts()}, &fakeRunner{})
	load(t, m)

	lines := strings.Split(m.View(), "\n")
	if len(lines) != 12 {
		t.Fatalf("rendered %d lines, want 12", len(lines))
	}
	for i, line := range lines {
		if width := lipgloss.Width(line); width > 60 {
			t.Fatalf("line %d width = %d, want <= 60", i, width)
		}
	}
}

func TestHostLoadErrorIsShown(t *testing.T) {
	m := newScreen(fakeSource{err: errors.New("boom")}, &fakeRunner{})
	load(t, m)

	if !strings.Contains(m.View(), "could not load hosts: boom") {
		t.Fatalf("load error is missing:\n%s", m.View())
	}
}

func TestRunnerErrorIsShown(t *testing.T) {
	m := newScreen(fakeSource{hosts: testHosts()}, &fakeRunner{err: errors.New("exit status 255")})
	load(t, m)

	typeText(m, "web")
	_, cmd := m.Update(press(tea.KeyEnter, ""))
	m.Update(cmd())

	if !strings.Contains(m.View(), "session failed: exit status 255") {
		t.Fatalf("session error is missing:\n%s", m.View())
	}
}

func TestListFocusShowsSelectionInSearchField(t *testing.T) {
	m := newScreen(fakeSource{hosts: testHosts()}, &fakeRunner{})
	load(t, m)

	m.Update(press(tea.KeyTab, ""))
	if got := m.input.Value(); got != "db" {
		t.Fatalf("search value = %q, want the selected host", got)
	}
	if got := m.input.Position(); got != len("db") {
		t.Fatalf("cursor = %d, want it at the end", got)
	}

	m.Update(press(tea.KeyDown, ""))
	if got := m.input.Value(); got != "host1" {
		t.Fatalf("search value = %q, want host1", got)
	}
	if got := m.input.Position(); got != len("host1") {
		t.Fatalf("cursor = %d, want it at the end", got)
	}

	m.Update(press(tea.KeyUp, ""))
	if got := m.input.Value(); got != "db" {
		t.Fatalf("search value = %q, want db", got)
	}
}

func TestSearchFocusKeepsTypedValue(t *testing.T) {
	m := newScreen(fakeSource{hosts: testHosts()}, &fakeRunner{})
	load(t, m)

	typeText(m, "ho")
	m.Update(press(tea.KeyDown, ""))
	if got := m.input.Value(); got != "ho" {
		t.Fatalf("search value = %q, want the typed query", got)
	}
}

func TestInitKeepsLoadedState(t *testing.T) {
	source := &countingSource{hosts: testHosts()}
	m := newScreen(source, &fakeRunner{})
	load(t, m)

	typeText(m, "ho")
	if m.selectedName() != "host1" {
		t.Fatalf("selected = %q, want host1", m.selectedName())
	}

	m.Init()

	if source.calls != 1 {
		t.Fatalf("source resolved %d times, want 1", source.calls)
	}
	if got := m.input.Value(); got != "ho" {
		t.Fatalf("search value = %q, want ho", got)
	}
	if m.selectedName() != "host1" {
		t.Fatalf("selected = %q, want host1", m.selectedName())
	}
}

func TestBackspaceInListFocusesSearchAndEdits(t *testing.T) {
	m := newScreen(fakeSource{hosts: testHosts()}, &fakeRunner{})
	load(t, m)

	m.Update(press(tea.KeyTab, ""))
	m.Update(press(tea.KeyDown, ""))
	if got := m.input.Value(); got != "host1" {
		t.Fatalf("search value = %q, want host1 before backspace", got)
	}

	m.Update(press(tea.KeyBackspace, ""))

	if m.focus != focusSearch || !m.input.Focused() {
		t.Fatal("backspace did not focus the search input")
	}
	if got := m.input.Value(); got != "host" {
		t.Fatalf("search value = %q, want host", got)
	}
	if got := m.input.Position(); got != len("host") {
		t.Fatalf("cursor = %d, want it at the end", got)
	}
	if len(m.filtered) != 1 || m.filtered[0].Name != "host1" {
		t.Fatalf("filtered = %v, want host1", m.filtered)
	}
}

func TestSelectedRowStyleFollowsFocus(t *testing.T) {
	m := newScreen(fakeSource{hosts: testHosts()}, &fakeRunner{})
	load(t, m)

	if m.selectedRowStyle().GetBold() {
		t.Fatal("selection should be muted while the search field has focus")
	}
	m.Update(press(tea.KeyTab, ""))
	if !m.selectedRowStyle().GetBold() {
		t.Fatal("selection should be stronger while the list has focus")
	}
}

func TestKeysFollowFocus(t *testing.T) {
	m := newScreen(fakeSource{}, &fakeRunner{})
	if len(m.Keys()) != 5 {
		t.Fatalf("search keys = %d, want 5", len(m.Keys()))
	}
	m.Update(press(tea.KeyTab, ""))
	if len(m.Keys()) != 6 {
		t.Fatalf("list keys = %d, want 6", len(m.Keys()))
	}
}
