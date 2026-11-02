package tunnels

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mcnull/sshex/internal/ui/screen"
)

func TestAddFormRejectsInvalidPortsInline(t *testing.T) {
	controller := &fakeController{}
	m := newScreen(&fakeSource{items: testTunnels()}, controller)
	load(t, m)

	m.Update(press('a', "a"))
	if m.form == nil {
		t.Fatal("a did not open the add form")
	}
	typeText(m, "0")
	_, cmd := m.Update(press(tea.KeyEnter, ""))
	if cmd != nil {
		t.Fatalf("invalid spec returned a command (%T), want nil", cmd)
	}
	if len(controller.addCalls) != 0 {
		t.Fatalf("add called with an invalid spec: %v", controller.addCalls)
	}
	if m.form == nil || !strings.Contains(m.form.err, "invalid port") {
		t.Fatalf("inline validation error is missing: %+v", m.form)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "invalid port") {
		t.Fatalf("validation error is not rendered:\n%s", view)
	}
}

func TestAddFormRejectsEmptySpec(t *testing.T) {
	m := newScreen(&fakeSource{}, &fakeController{})
	m.Update(press('a', "a"))

	_, cmd := m.Update(press(tea.KeyEnter, ""))
	if cmd != nil {
		t.Fatalf("empty spec returned a command (%T), want nil", cmd)
	}
	if m.form == nil || !strings.Contains(m.form.err, "enter a port") {
		t.Fatalf("empty spec error is missing: %+v", m.form)
	}
}

func TestAddTunnelSuccess(t *testing.T) {
	added := Tunnel{
		ID: "new", SessionID: "s1", LocalPort: 8080,
		TargetHost: "db", TargetPort: 5432, Direction: DirectionLocal, State: "active",
	}
	source := &fakeSource{items: []Tunnel{added}}
	controller := &fakeController{added: added}
	m := newScreen(source, controller)
	load(t, m)

	m.Update(press('a', "a"))
	typeText(m, "8080:db:5432")
	_, cmd := m.Update(press(tea.KeyEnter, ""))
	if cmd == nil {
		t.Fatal("valid spec returned no command")
	}
	msg, ok := cmd().(addedMsg)
	if !ok {
		t.Fatalf("message = %T, want addedMsg", cmd())
	}
	if len(controller.addCalls) != 1 {
		t.Fatalf("add calls = %v, want one", controller.addCalls)
	}
	call := controller.addCalls[0]
	if call.sessionID != "s1" {
		t.Fatalf("add session = %q, want s1", call.sessionID)
	}
	want := AddRequest{LocalPort: 8080, TargetHost: "db", TargetPort: 5432, Direction: DirectionLocal}
	if call.request != want {
		t.Fatalf("add request = %+v, want %+v", call.request, want)
	}

	_, loadCmd := m.Update(msg)
	if m.form != nil {
		t.Fatal("form stayed open after a successful add")
	}
	if !strings.Contains(ansi.Strip(m.View()), "tunnel created.") {
		t.Fatalf("add status is missing:\n%s", m.View())
	}
	if loadCmd != nil {
		m.Update(loadCmd())
	}
	if got := m.selectedID(); got != "new" {
		t.Fatalf("selected = %q, want the added tunnel", got)
	}
	if !strings.Contains(ansi.Strip(m.View()), "localhost:8080 > db:5432") {
		t.Fatalf("added tunnel is not rendered:\n%s", m.View())
	}
}

func TestAddFormDirectionToggle(t *testing.T) {
	controller := &fakeController{added: Tunnel{ID: "new", SessionID: "s1", State: "active"}}
	m := newScreen(&fakeSource{}, controller)
	m.Update(press('a', "a"))

	typeText(m, "9000")
	m.Update(press(tea.KeyRight, ""))
	if m.form.direction != DirectionRemote {
		t.Fatalf("direction = %q, want remote", m.form.direction)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "Add tunnel · remote") {
		t.Fatalf("remote direction is not shown:\n%s", view)
	}

	m.Update(press(tea.KeyLeft, ""))
	if m.form.direction != DirectionLocal {
		t.Fatalf("direction = %q, want local", m.form.direction)
	}
	m.Update(press(tea.KeyRight, ""))

	_, cmd := m.Update(press(tea.KeyEnter, ""))
	if cmd == nil {
		t.Fatal("valid spec returned no command")
	}
	m.Update(cmd())

	if len(controller.addCalls) != 1 {
		t.Fatalf("add calls = %v, want one", controller.addCalls)
	}
	call := controller.addCalls[0]
	if call.sessionID != "s1" {
		t.Fatalf("add session = %q, want s1", call.sessionID)
	}
	want := AddRequest{LocalPort: 9000, TargetHost: "localhost", TargetPort: 9000, Direction: DirectionRemote}
	if call.request != want {
		t.Fatalf("add request = %+v, want %+v", call.request, want)
	}
}

func TestAddFormCancel(t *testing.T) {
	m := newScreen(&fakeSource{items: testTunnels()}, &fakeController{})
	load(t, m)

	m.Update(press('a', "a"))
	typeText(m, "8080")
	_, cmd := m.Update(press(tea.KeyEsc, ""))
	if cmd != nil {
		t.Fatalf("esc in the form returned a command (%T), want nil", cmd)
	}
	if m.form != nil {
		t.Fatal("esc did not close the form")
	}

	_, cmd = m.Update(press(tea.KeyEsc, ""))
	msg := action(t, cmd)
	if msg.Action != screen.ActionPop {
		t.Fatalf("second esc action = %v, want pop", msg.Action)
	}
}

func TestAddErrorIsShownInline(t *testing.T) {
	controller := &fakeController{addErr: errors.New("boom")}
	m := newScreen(&fakeSource{}, controller)
	m.Update(press('a', "a"))

	typeText(m, "8080")
	_, cmd := m.Update(press(tea.KeyEnter, ""))
	if cmd == nil {
		t.Fatal("valid spec returned no command")
	}
	m.Update(cmd())

	if m.form == nil {
		t.Fatal("form closed after an add error")
	}
	if !strings.Contains(m.form.err, "could not add tunnel: boom") {
		t.Fatalf("form error = %q, want the controller error", m.form.err)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "could not add tunnel: boom") {
		t.Fatalf("add error is not rendered:\n%s", view)
	}
}
