package tunnels

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mcnull/sshex/internal/ui/screen"
	"github.com/mcnull/sshex/internal/ui/theme"
)

func newFormScreen(controller Controller) *AddForm {
	styles := theme.NewStyles(theme.Load(theme.Options{NoColor: true}))
	form := NewAddForm(AddFormOptions{
		Styles:     &styles,
		Controller: controller,
		SessionID:  "s1",
		Target:     "bob@ssh01",
	})
	form.Resize(60, 12)
	return form
}

func typeFormText(form *AddForm, text string) {
	for _, r := range text {
		form.Update(press(r, string(r)))
	}
}

func TestAddFormRejectsInvalidPortsInline(t *testing.T) {
	controller := &fakeController{}
	form := newFormScreen(controller)

	typeFormText(form, "0")
	_, cmd := form.Update(press(tea.KeyEnter, ""))
	if cmd != nil {
		t.Fatalf("invalid spec returned a command (%T), want nil", cmd)
	}
	if len(controller.addCalls) != 0 {
		t.Fatalf("add called with an invalid spec: %v", controller.addCalls)
	}
	if !strings.Contains(form.err, "invalid port") {
		t.Fatalf("inline validation error is missing: %+v", form)
	}
	if view := ansi.Strip(form.View()); !strings.Contains(view, "invalid port") {
		t.Fatalf("validation error is not rendered:\n%s", view)
	}
}

func TestAddFormRejectsEmptySpec(t *testing.T) {
	form := newFormScreen(&fakeController{})

	_, cmd := form.Update(press(tea.KeyEnter, ""))
	if cmd != nil {
		t.Fatalf("empty spec returned a command (%T), want nil", cmd)
	}
	if !strings.Contains(form.err, "enter a port") {
		t.Fatalf("empty spec error is missing: %+v", form)
	}
}

func TestAddFormSuccessPops(t *testing.T) {
	added := Tunnel{
		ID: "new", SessionID: "s1", LocalPort: 8080,
		TargetHost: "db", TargetPort: 5432, Direction: DirectionLocal, State: "active",
	}
	controller := &fakeController{added: added}
	form := newFormScreen(controller)

	typeFormText(form, "8080:db:5432")
	_, cmd := form.Update(press(tea.KeyEnter, ""))
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

	_, popCmd := form.Update(msg)
	if action := action(t, popCmd); action.Action != screen.ActionPop {
		t.Fatalf("success action = %v, want pop", action.Action)
	}
}

func TestAddFormShowsSessionAndDirection(t *testing.T) {
	form := newFormScreen(&fakeController{})

	view := ansi.Strip(form.View())
	if !strings.Contains(view, "Add tunnel · bob@ssh01 · local") {
		t.Fatalf("form header is missing the session or direction:\n%s", view)
	}

	form.Update(press(tea.KeyRight, ""))
	if form.direction != DirectionRemote {
		t.Fatalf("direction = %q, want remote", form.direction)
	}
	if view := ansi.Strip(form.View()); !strings.Contains(view, "Add tunnel · bob@ssh01 · remote") {
		t.Fatalf("remote direction is not shown:\n%s", view)
	}

	form.Update(press(tea.KeyLeft, ""))
	if form.direction != DirectionLocal {
		t.Fatalf("direction = %q, want local", form.direction)
	}
}

func TestAddFormUsesRemoteDirection(t *testing.T) {
	controller := &fakeController{added: Tunnel{ID: "new", SessionID: "s1", State: "active"}}
	form := newFormScreen(controller)

	typeFormText(form, "9000")
	form.Update(press(tea.KeyRight, ""))
	_, cmd := form.Update(press(tea.KeyEnter, ""))
	if cmd == nil {
		t.Fatal("valid spec returned no command")
	}
	form.Update(cmd())

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

func TestAddFormCancelPops(t *testing.T) {
	form := newFormScreen(&fakeController{})
	typeFormText(form, "8080")

	_, cmd := form.Update(press(tea.KeyEsc, ""))
	if action := action(t, cmd); action.Action != screen.ActionPop {
		t.Fatalf("esc action = %v, want pop", action.Action)
	}
}

func TestAddFormErrorIsShownInline(t *testing.T) {
	controller := &fakeController{addErr: errors.New("boom")}
	form := newFormScreen(controller)

	typeFormText(form, "8080")
	_, cmd := form.Update(press(tea.KeyEnter, ""))
	if cmd == nil {
		t.Fatal("valid spec returned no command")
	}
	form.Update(cmd())

	if !strings.Contains(form.err, "could not add tunnel: boom") {
		t.Fatalf("form error = %q, want the controller error", form.err)
	}
	if view := ansi.Strip(form.View()); !strings.Contains(view, "could not add tunnel: boom") {
		t.Fatalf("add error is not rendered:\n%s", view)
	}
	if form.submitting {
		t.Fatal("form still considers the failed add pending")
	}
}

func TestAddFormViewFitsContentSize(t *testing.T) {
	form := newFormScreen(&fakeController{})
	form.Resize(48, 12)

	lines := strings.Split(form.View(), "\n")
	if len(lines) != 12 {
		t.Fatalf("rendered %d lines, want 12", len(lines))
	}
	for i, line := range lines {
		if width := lipgloss.Width(line); width > 48 {
			t.Fatalf("line %d width = %d, want <= 48", i, width)
		}
	}
}

func TestAddFormTitle(t *testing.T) {
	form := newFormScreen(&fakeController{})
	if got := form.Title(); got != "Add tunnel" {
		t.Fatalf("title = %q, want Add tunnel", got)
	}
}
