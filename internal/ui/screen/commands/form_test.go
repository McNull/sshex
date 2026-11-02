package commands

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

func newFormScreen(controller Controller, existing []Command, original *Command) *Form {
	styles := theme.NewStyles(theme.Load(theme.Options{NoColor: true}))
	form := NewForm(FormOptions{
		Styles:     &styles,
		Controller: controller,
		Existing:   existing,
		Original:   original,
	})
	form.Resize(64, 12)
	return form
}

func typeFormText(form *Form, text string) {
	for _, r := range text {
		form.Update(press(r, string(r)))
	}
}

func tab(form *Form) {
	form.Update(tea.KeyPressMsg{Code: tea.KeyTab})
}

func TestFormAddSuccessPops(t *testing.T) {
	controller := &fakeController{}
	form := newFormScreen(controller, nil, nil)

	typeFormText(form, "hello")
	tab(form)
	typeFormText(form, "echo hi")
	_, cmd := form.Update(press(tea.KeyEnter, ""))
	if cmd == nil {
		t.Fatal("valid form returned no command")
	}
	msg, ok := cmd().(savedMsg)
	if !ok {
		t.Fatalf("message = %T, want savedMsg", cmd())
	}
	if len(controller.addCalls) != 1 {
		t.Fatalf("add calls = %v, want one", controller.addCalls)
	}
	want := Command{Name: "hello", Template: "echo hi"}
	if controller.addCalls[0] != want {
		t.Fatalf("add call = %+v, want %+v", controller.addCalls[0], want)
	}

	_, popCmd := form.Update(msg)
	if action := action(t, popCmd); action.Action != screen.ActionPop {
		t.Fatalf("success action = %v, want pop", action.Action)
	}
}

func TestFormEditPrefillsAndRenames(t *testing.T) {
	original := Command{Name: "code", Template: "code", Alias: "ed"}
	controller := &fakeController{}
	form := newFormScreen(controller, []Command{original}, &original)

	if got := form.inputs[fieldName].Value(); got != "code" {
		t.Fatalf("name = %q, want code", got)
	}
	if got := form.inputs[fieldCommand].Value(); got != "code" {
		t.Fatalf("command = %q, want code", got)
	}
	if got := form.inputs[fieldAlias].Value(); got != "ed" {
		t.Fatalf("alias = %q, want ed", got)
	}

	form.inputs[fieldName].SetValue("code2")
	_, cmd := form.Update(press(tea.KeyEnter, ""))
	if cmd == nil {
		t.Fatal("valid edit returned no command")
	}
	msg := cmd().(savedMsg)
	form.Update(msg)

	if len(controller.updateCalls) != 1 {
		t.Fatalf("update calls = %v, want one", controller.updateCalls)
	}
	call := controller.updateCalls[0]
	if call.name != "code" {
		t.Fatalf("update name = %q, want code", call.name)
	}
	if call.command.Name != "code2" {
		t.Fatalf("updated name = %q, want code2", call.command.Name)
	}
}

func TestFormRejectsEmptyName(t *testing.T) {
	controller := &fakeController{}
	form := newFormScreen(controller, nil, nil)

	_, cmd := form.Update(press(tea.KeyEnter, ""))
	if cmd != nil {
		t.Fatalf("invalid form returned a command (%T), want nil", cmd)
	}
	if len(controller.addCalls) != 0 {
		t.Fatalf("add called with an invalid form: %v", controller.addCalls)
	}
	if !strings.Contains(form.err, "name is required") {
		t.Fatalf("validation error = %q, want a required name", form.err)
	}
	if form.focus != fieldName {
		t.Fatalf("focus = %d, want the name field", form.focus)
	}
	if view := ansi.Strip(form.View()); !strings.Contains(view, "name is required") {
		t.Fatalf("validation error is not rendered:\n%s", view)
	}
}

func TestFormRejectsEmptyCommand(t *testing.T) {
	form := newFormScreen(&fakeController{}, nil, nil)

	typeFormText(form, "hello")
	tab(form)
	form.Update(press(tea.KeyEnter, ""))

	if !strings.Contains(form.err, "command is required") {
		t.Fatalf("validation error = %q, want a required command", form.err)
	}
	if form.focus != fieldCommand {
		t.Fatalf("focus = %d, want the command field", form.focus)
	}
}

func TestFormRejectsInvalidName(t *testing.T) {
	form := newFormScreen(&fakeController{}, nil, nil)

	typeFormText(form, "bad name")
	tab(form)
	typeFormText(form, "echo")
	form.Update(press(tea.KeyEnter, ""))

	if !strings.Contains(form.err, "invalid command name") {
		t.Fatalf("validation error = %q, want an invalid name", form.err)
	}
}

func TestFormRejectsDuplicateName(t *testing.T) {
	existing := []Command{{Name: "code", Template: "code"}}
	form := newFormScreen(&fakeController{}, existing, nil)

	typeFormText(form, "code")
	tab(form)
	typeFormText(form, "other")
	form.Update(press(tea.KeyEnter, ""))

	if !strings.Contains(form.err, `name "code" is already in use`) {
		t.Fatalf("validation error = %q, want a duplicate name", form.err)
	}
	if form.focus != fieldName {
		t.Fatalf("focus = %d, want the name field", form.focus)
	}
}

func TestFormRejectsDuplicateAlias(t *testing.T) {
	existing := []Command{{Name: "code", Template: "code", Alias: "ed"}}
	form := newFormScreen(&fakeController{}, existing, nil)

	typeFormText(form, "zed")
	tab(form)
	typeFormText(form, "zed")
	tab(form)
	typeFormText(form, "ed")
	form.Update(press(tea.KeyEnter, ""))

	if !strings.Contains(form.err, `alias "ed" is already in use`) {
		t.Fatalf("validation error = %q, want a duplicate alias", form.err)
	}
	if form.focus != fieldAlias {
		t.Fatalf("focus = %d, want the alias field", form.focus)
	}
}

func TestFormAllowsOwnNameAndAliasWhenEditing(t *testing.T) {
	original := Command{Name: "code", Template: "code", Alias: "code"}
	controller := &fakeController{}
	form := newFormScreen(controller, []Command{original}, &original)

	form.inputs[fieldCommand].SetValue("code --new")
	_, cmd := form.Update(press(tea.KeyEnter, ""))
	if cmd == nil {
		t.Fatalf("valid edit returned no command; err = %q", form.err)
	}
	form.Update(cmd())

	if len(controller.updateCalls) != 1 {
		t.Fatalf("update calls = %v, want one", controller.updateCalls)
	}
}

func TestFormToggleDisabled(t *testing.T) {
	form := newFormScreen(&fakeController{}, nil, nil)

	tab(form)
	tab(form)
	tab(form)
	if form.focus != fieldDisabled {
		t.Fatalf("focus = %d, want the disabled field", form.focus)
	}
	form.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})

	if !form.disabled {
		t.Fatal("space did not disable the command")
	}
	if view := ansi.Strip(form.View()); !strings.Contains(view, "[x] yes") {
		t.Fatalf("disabled value is not rendered:\n%s", view)
	}

	form.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	if form.disabled {
		t.Fatal("space did not enable the command again")
	}
}

func TestFormShiftTabMovesBackwards(t *testing.T) {
	form := newFormScreen(&fakeController{}, nil, nil)

	form.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if form.focus != fieldDisabled {
		t.Fatalf("focus = %d, want the disabled field", form.focus)
	}
}

func TestFormCancelPops(t *testing.T) {
	form := newFormScreen(&fakeController{}, nil, nil)
	typeFormText(form, "hello")

	_, cmd := form.Update(press(tea.KeyEsc, ""))
	if action := action(t, cmd); action.Action != screen.ActionPop {
		t.Fatalf("esc action = %v, want pop", action.Action)
	}
}

func TestFormErrorIsShownInline(t *testing.T) {
	controller := &fakeController{addErr: errors.New("boom")}
	form := newFormScreen(controller, nil, nil)

	typeFormText(form, "hello")
	tab(form)
	typeFormText(form, "echo")
	_, cmd := form.Update(press(tea.KeyEnter, ""))
	if cmd == nil {
		t.Fatal("valid form returned no command")
	}
	form.Update(cmd())

	if !strings.Contains(form.err, "could not save command: boom") {
		t.Fatalf("form error = %q, want the controller error", form.err)
	}
	if view := ansi.Strip(form.View()); !strings.Contains(view, "could not save command: boom") {
		t.Fatalf("save error is not rendered:\n%s", view)
	}
	if form.submitting {
		t.Fatal("form still considers the failed save pending")
	}
}

func TestFormViewFitsContentSize(t *testing.T) {
	form := newFormScreen(&fakeController{}, nil, nil)
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

func TestFormTitle(t *testing.T) {
	if got := newFormScreen(&fakeController{}, nil, nil).Title(); got != "Add command" {
		t.Fatalf("add title = %q, want Add command", got)
	}
	original := Command{Name: "code"}
	if got := newFormScreen(&fakeController{}, nil, &original).Title(); got != "Edit command" {
		t.Fatalf("edit title = %q, want Edit command", got)
	}
}
