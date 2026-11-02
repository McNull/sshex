package tunnels

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mcnull/sshex/internal/ui/theme"
)

type fakePortSource struct {
	results  [][]Port
	byHost   map[PortHost][]Port
	err      error
	hosts    []PortHost
	sessions []string
	calls    int
}

func (s *fakePortSource) List(sessionID string, host PortHost) ([]Port, error) {
	s.calls++
	s.hosts = append(s.hosts, host)
	s.sessions = append(s.sessions, sessionID)
	if s.byHost != nil {
		return s.byHost[host], s.err
	}
	if len(s.results) > 0 {
		result := s.results[0]
		s.results = s.results[1:]
		return result, s.err
	}
	return nil, s.err
}

func newPortForm(controller Controller, source PortSource) *AddForm {
	styles := theme.NewStyles(theme.Load(theme.Options{NoColor: true}))
	form := NewAddForm(AddFormOptions{
		Styles:     &styles,
		Controller: controller,
		PortSource: source,
		SessionID:  "s1",
		Target:     "bob@ssh01",
	})
	form.Resize(60, 12)
	return form
}

// runFormInit executes every command returned by Init and applies the resulting
// messages to the form. It ignores cursor blink commands.
func runFormInit(t *testing.T, form *AddForm) {
	t.Helper()
	cmd := form.Init()
	if cmd == nil {
		t.Fatal("Init returned no command")
	}
	applyFormCmd(t, form, cmd)
}

func applyFormCmd(t *testing.T, form *AddForm, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, item := range batch {
			if item == nil {
				continue
			}
			form.Update(item())
		}
		return
	}
	if msg != nil {
		form.Update(msg)
	}
}

func testPorts() []Port {
	return []Port{
		{Port: 22, Address: "0.0.0.0", Process: "sshd", PID: 1},
		{Port: 3000, Address: "127.0.0.1", Process: "node", PID: 24112},
		{Port: 8080, Address: "127.0.0.1", Process: "python", PID: 4310},
	}
}

func TestPortHostForDirection(t *testing.T) {
	if got := portHostForDirection(DirectionLocal); got != PortHostRemote {
		t.Fatalf("local direction host = %q, want remote", got)
	}
	if got := portHostForDirection(DirectionRemote); got != PortHostLocal {
		t.Fatalf("remote direction host = %q, want local", got)
	}
}

func TestPortFormLoadsTargetPorts(t *testing.T) {
	source := &fakePortSource{results: [][]Port{testPorts()}}
	form := newPortForm(&fakeController{}, source)

	runFormInit(t, form)

	if source.calls != 1 {
		t.Fatalf("port calls = %d, want 1", source.calls)
	}
	if len(source.hosts) != 1 || source.hosts[0] != PortHostRemote {
		t.Fatalf("port hosts = %v, want remote", source.hosts)
	}
	if len(source.sessions) != 1 || source.sessions[0] != "s1" {
		t.Fatalf("port sessions = %v, want s1", source.sessions)
	}
	if len(form.ports) != 3 {
		t.Fatalf("ports = %+v, want three", form.ports)
	}

	view := ansi.Strip(form.View())
	for _, want := range []string{"PORTS", "bob@ssh01", "sshd", "24112", "8080"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view is missing %q:\n%s", want, view)
		}
	}
}

func TestPortFormTabFocusAndSelect(t *testing.T) {
	source := &fakePortSource{results: [][]Port{testPorts()}}
	controller := &fakeController{added: Tunnel{ID: "new", SessionID: "s1", State: "active"}}
	form := newPortForm(controller, source)
	runFormInit(t, form)

	form.Update(press(tea.KeyTab, ""))
	if form.focus != focusPorts {
		t.Fatalf("focus = %v, want ports", form.focus)
	}
	if form.input.Focused() {
		t.Fatal("input should be blurred while the port list has focus")
	}

	form.Update(press(tea.KeyDown, ""))
	_, cmd := form.Update(press(tea.KeyEnter, ""))
	if cmd == nil {
		t.Fatal("selecting a port returned no focus command")
	}
	if form.focus != focusSpec {
		t.Fatalf("focus after select = %v, want spec", form.focus)
	}
	if got := form.input.Value(); got != "3000" {
		t.Fatalf("spec after select = %q, want 3000", got)
	}
	if got := form.input.Position(); got != len("3000") {
		t.Fatalf("cursor position = %d, want %d", got, len("3000"))
	}

	_, cmd = form.Update(press(tea.KeyEnter, ""))
	if cmd == nil {
		t.Fatal("valid spec after selecting a port returned no command")
	}
	msg := cmd().(addedMsg)
	if msg.err != nil {
		t.Fatalf("add error = %v", msg.err)
	}
	if len(controller.addCalls) != 1 {
		t.Fatalf("add calls = %v, want one", controller.addCalls)
	}
	if got := controller.addCalls[0].request; got.LocalPort != 3000 || got.Direction != DirectionLocal {
		t.Fatalf("add request = %+v, want port 3000 local", got)
	}
}

func TestPortFormDirectionReloadsHost(t *testing.T) {
	local := []Port{{Port: 5000, Process: "localapp", PID: 7}}
	source := &fakePortSource{results: [][]Port{testPorts(), local}}
	form := newPortForm(&fakeController{}, source)
	runFormInit(t, form)

	_, cmd := form.Update(press(tea.KeyRight, ""))
	if form.direction != DirectionRemote {
		t.Fatalf("direction = %v, want remote", form.direction)
	}
	applyFormCmd(t, form, cmd)

	wantHosts := []PortHost{PortHostRemote, PortHostLocal}
	if len(source.hosts) != len(wantHosts) {
		t.Fatalf("port hosts = %v, want %v", source.hosts, wantHosts)
	}
	for i := range wantHosts {
		if source.hosts[i] != wantHosts[i] {
			t.Fatalf("port hosts = %v, want %v", source.hosts, wantHosts)
		}
	}
	if len(form.ports) != 1 || form.ports[0].Port != 5000 {
		t.Fatalf("ports = %+v, want the local list", form.ports)
	}
	view := ansi.Strip(form.View())
	if !strings.Contains(view, "PORTS") || !strings.Contains(view, "local") {
		t.Fatalf("view does not show the local host:\n%s", view)
	}
	if !strings.Contains(view, "remote") {
		t.Fatalf("title does not show the remote direction:\n%s", view)
	}
}

func TestPortFormIgnoresStaleLoads(t *testing.T) {
	remote := []Port{{Port: 9000, Process: "remoteapp", PID: 1}}
	local := []Port{{Port: 5000, Process: "localapp", PID: 2}}
	source := &fakePortSource{byHost: map[PortHost][]Port{
		PortHostRemote: remote,
		PortHostLocal:  local,
	}}
	form := newPortForm(&fakeController{}, source)

	first := form.loadPorts(true)
	_, second := form.Update(press(tea.KeyRight, ""))

	applyFormCmd(t, form, second)
	applyFormCmd(t, form, first)

	if len(form.ports) != 1 || form.ports[0].Port != 5000 {
		t.Fatalf("ports = %+v, want the newest local list", form.ports)
	}
}

func TestPortFormRefreshKeepsSelection(t *testing.T) {
	first := []Port{
		{Port: 22, Process: "sshd", PID: 1},
		{Port: 3000, Process: "node", PID: 2},
		{Port: 8080, Process: "python", PID: 3},
	}
	second := []Port{
		{Port: 8080, Process: "python", PID: 3},
		{Port: 22, Process: "sshd", PID: 1},
		{Port: 3000, Process: "node", PID: 2},
	}
	source := &fakePortSource{results: [][]Port{first, second}}
	form := newPortForm(&fakeController{}, source)
	runFormInit(t, form)

	form.Update(press(tea.KeyTab, ""))
	form.Update(press(tea.KeyDown, ""))
	form.Update(press(tea.KeyDown, ""))
	if form.portSelected != 2 {
		t.Fatalf("selected = %d, want 2", form.portSelected)
	}

	_, cmd := form.Update(press('r', "r"))
	if cmd == nil {
		t.Fatal("refresh returned no command")
	}
	applyFormCmd(t, form, cmd)

	if source.calls != 2 {
		t.Fatalf("port calls = %d, want 2", source.calls)
	}
	if form.portSelected != 0 || form.ports[form.portSelected].Port != 8080 {
		t.Fatalf("selected = %d (%+v), want the refreshed 8080 row", form.portSelected, form.ports)
	}
}

func TestPortFormEmptyState(t *testing.T) {
	source := &fakePortSource{results: [][]Port{{}}}
	form := newPortForm(&fakeController{}, source)
	runFormInit(t, form)

	view := ansi.Strip(form.View())
	if !strings.Contains(view, "no listening ports") {
		t.Fatalf("empty port state is missing:\n%s", view)
	}

	form.Update(press(tea.KeyTab, ""))
	_, cmd := form.Update(press(tea.KeyEnter, ""))
	if cmd != nil {
		t.Fatalf("enter on an empty port list returned a command (%T)", cmd)
	}
	if form.focus != focusPorts {
		t.Fatalf("focus = %v, want ports", form.focus)
	}
}

func TestPortFormErrorDoesNotBlockSubmit(t *testing.T) {
	source := &fakePortSource{err: errors.New("boom")}
	controller := &fakeController{added: Tunnel{ID: "new", SessionID: "s1", State: "active"}}
	form := newPortForm(controller, source)
	runFormInit(t, form)

	if form.portsErr == "" {
		t.Fatal("port load error is missing")
	}
	if view := ansi.Strip(form.View()); !strings.Contains(view, "could not list ports: boom") {
		t.Fatalf("port error is not rendered:\n%s", view)
	}

	typeFormText(form, "8080")
	_, cmd := form.Update(press(tea.KeyEnter, ""))
	if cmd == nil {
		t.Fatal("submit returned no command despite the port error")
	}
	form.Update(cmd())
	if len(controller.addCalls) != 1 {
		t.Fatalf("add calls = %v, want one", controller.addCalls)
	}
}

func TestPortFormScrollsLongList(t *testing.T) {
	ports := make([]Port, 10)
	for i := range ports {
		ports[i] = Port{Port: 1001 + i, Process: "app", PID: i + 1}
	}
	source := &fakePortSource{results: [][]Port{ports}}
	form := newPortForm(&fakeController{}, source)
	runFormInit(t, form)

	form.Update(press(tea.KeyTab, ""))
	for i := 0; i < 8; i++ {
		form.Update(press(tea.KeyDown, ""))
	}
	if form.portSelected != 8 {
		t.Fatalf("selected = %d, want 8", form.portSelected)
	}
	if form.portOffset != 3 {
		t.Fatalf("offset = %d, want 3", form.portOffset)
	}

	view := ansi.Strip(form.View())
	if strings.Contains(view, "1001") {
		t.Fatalf("scrolled view still shows the first port:\n%s", view)
	}
	if !strings.Contains(view, "1009") {
		t.Fatalf("scrolled view does not show the selected port:\n%s", view)
	}
}

func TestPortFormBackspaceReturnsToSpec(t *testing.T) {
	source := &fakePortSource{results: [][]Port{testPorts()}}
	form := newPortForm(&fakeController{}, source)
	runFormInit(t, form)

	form.Update(press(tea.KeyTab, ""))
	_, cmd := form.Update(press(tea.KeyBackspace, ""))
	if cmd == nil {
		t.Fatal("backspace returned no focus command")
	}
	if form.focus != focusSpec {
		t.Fatalf("focus = %v, want spec", form.focus)
	}
	if !form.input.Focused() {
		t.Fatal("input is not focused after backspace")
	}
}

func TestPortFormWithoutSourceKeepsTabInSpec(t *testing.T) {
	form := newFormScreen(&fakeController{})
	_, cmd := form.Update(press(tea.KeyTab, ""))
	if cmd != nil {
		t.Fatalf("tab returned a command (%T) without a port source", cmd)
	}
	if form.focus != focusSpec {
		t.Fatalf("focus = %v, want spec", form.focus)
	}
	if view := ansi.Strip(form.View()); strings.Contains(view, "PORTS") {
		t.Fatalf("port section is shown without a source:\n%s", view)
	}
}
