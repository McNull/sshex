package commands

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

func save(form *Form) (screen.Screen, tea.Cmd) {
	for form.focus != fieldSave {
		tab(form)
	}
	return form.Update(press(tea.KeyEnter, ""))
}

func cancel(form *Form) (screen.Screen, tea.Cmd) {
	for form.focus != fieldCancel {
		tab(form)
	}
	return form.Update(press(tea.KeyEnter, ""))
}

func TestFormAddSuccessPops(t *testing.T) {
	controller := &fakeController{}
	form := newFormScreen(controller, nil, nil)

	typeFormText(form, "hello")
	tab(form)
	typeFormText(form, "echo hi")
	_, cmd := save(form)
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

func TestFormCommandEnterInsertsNewline(t *testing.T) {
	controller := &fakeController{}
	form := newFormScreen(controller, nil, nil)

	typeFormText(form, "hello")
	tab(form)
	typeFormText(form, "echo one")
	form.Update(press(tea.KeyEnter, ""))
	typeFormText(form, "echo two")

	if got := form.command.Value(); got != "echo one\necho two" {
		t.Fatalf("command = %q, want a newline-separated command", got)
	}
	if form.focus != fieldCommand {
		t.Fatalf("focus = %d, want the command field", form.focus)
	}
	if len(controller.addCalls) != 0 {
		t.Fatal("enter in the command editor saved the form")
	}
}

func TestFormEnterMovesToNextField(t *testing.T) {
	controller := &fakeController{}
	form := newFormScreen(controller, nil, nil)

	typeFormText(form, "hello")
	form.Update(press(tea.KeyEnter, ""))
	if form.focus != fieldCommand {
		t.Fatalf("focus after enter on name = %d, want the command field", form.focus)
	}

	tab(form)
	typeFormText(form, "hi")
	form.Update(press(tea.KeyEnter, ""))
	if form.focus != fieldDisabled {
		t.Fatalf("focus after enter on alias = %d, want the disabled field", form.focus)
	}
	if len(controller.addCalls) != 0 {
		t.Fatal("enter on name or alias saved the form")
	}
}

func TestFormNameAndAliasFooterOmitsEnter(t *testing.T) {
	form := newFormScreen(&fakeController{}, nil, nil)

	for _, field := range []int{fieldName, fieldAlias} {
		form.focus = field
		for _, binding := range form.Keys() {
			if binding.Help().Key == "enter" {
				t.Fatalf("field %d still shows the enter footer tip", field)
			}
		}
	}
}

func TestFormValueFieldsDoNotUseSelectionBackground(t *testing.T) {
	styles := theme.NewStyles(theme.Load(theme.Options{HasDarkBackground: true}))
	form := NewForm(FormOptions{Styles: &styles, Controller: &fakeController{}})
	form.Resize(60, 12)

	form.focus = fieldName
	form.refreshInputStyles()
	if got, unwanted := form.nameInput.Styles().Focused.Text.GetBackground(), styles.MenuRowSelected.GetBackground(); fmt.Sprint(got) == fmt.Sprint(unwanted) {
		t.Fatalf("name field uses the selection background %v", got)
	}

	form.focus = fieldCommand
	form.refreshInputStyles()
	if got, unwanted := form.command.Styles().Focused.Text.GetBackground(), styles.MenuRowSelected.GetBackground(); fmt.Sprint(got) == fmt.Sprint(unwanted) {
		t.Fatalf("command field uses the selection background %v", got)
	}
}

func TestFormFocusStartsAtFieldBeginning(t *testing.T) {
	original := Command{Name: "code", Template: "echo one\necho two", Alias: "ed"}
	form := newFormScreen(&fakeController{}, []Command{original}, &original)

	if got := form.nameInput.Position(); got != 0 {
		t.Fatalf("name cursor = %d, want 0", got)
	}
	if got := form.aliasInput.Position(); got != 0 {
		t.Fatalf("alias cursor = %d, want 0", got)
	}
	if line, column := form.command.Line(), form.command.Column(); line != 0 || column != 0 {
		t.Fatalf("command cursor = %d:%d, want 0:0", line, column)
	}

	form.nameInput.SetValue("code")
	form.aliasInput.SetValue("ed")
	form.command.SetValue("echo one\necho two")
	form.setFocus(fieldCommand)
	if line, column := form.command.Line(), form.command.Column(); line != 0 || column != 0 {
		t.Fatalf("command cursor after focus = %d:%d, want 0:0", line, column)
	}

	form.setFocus(fieldAlias)
	if got := form.aliasInput.Position(); got != 0 {
		t.Fatalf("alias cursor after focus = %d, want 0", got)
	}
}

func TestFormFooterOmitsShiftTab(t *testing.T) {
	form := newFormScreen(&fakeController{}, nil, nil)

	for _, binding := range form.Keys() {
		if binding.Help().Key == "shift+tab" {
			t.Fatal("form footer still shows the shift+tab tip")
		}
	}
}

func TestFormCancelButtonPops(t *testing.T) {
	controller := &fakeController{}
	form := newFormScreen(controller, nil, nil)
	typeFormText(form, "hello")

	_, cmd := cancel(form)
	if action := action(t, cmd); action.Action != screen.ActionPop {
		t.Fatalf("cancel action = %v, want pop", action.Action)
	}
	if len(controller.addCalls) != 0 {
		t.Fatal("cancel saved the command")
	}
}

func TestFormCommandWrapsLongInput(t *testing.T) {
	form := newFormScreen(&fakeController{}, nil, nil)
	form.Resize(40, 12)
	form.command.SetValue(strings.Repeat("word ", 30))

	if height := form.command.Height(); height < 2 {
		t.Fatalf("command height = %d, want wrapped content", height)
	}
	lines := strings.Split(form.View(), "\n")
	if len(lines) != 12 {
		t.Fatalf("rendered %d lines, want 12", len(lines))
	}
	for i, line := range lines {
		if width := lipgloss.Width(line); width > 40 {
			t.Fatalf("line %d width = %d, want <= 40", i, width)
		}
	}
}

func TestFormEditPrefillsAndRenames(t *testing.T) {
	original := Command{Name: "code", Template: "code", Alias: "ed"}
	controller := &fakeController{}
	form := newFormScreen(controller, []Command{original}, &original)

	if got := form.nameInput.Value(); got != "code" {
		t.Fatalf("name = %q, want code", got)
	}
	if got := form.command.Value(); got != "code" {
		t.Fatalf("command = %q, want code", got)
	}
	if got := form.aliasInput.Value(); got != "ed" {
		t.Fatalf("alias = %q, want ed", got)
	}

	form.nameInput.SetValue("code2")
	_, cmd := save(form)
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

	save(form)
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
	save(form)

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
	save(form)

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
	save(form)

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
	save(form)

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

	form.command.SetValue("code --new")
	_, cmd := save(form)
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
	if form.focus != fieldCancel {
		t.Fatalf("focus = %d, want the cancel button", form.focus)
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
	_, cmd := save(form)
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

	for _, field := range []int{fieldName, fieldCommand, fieldAlias, fieldDisabled, fieldSave, fieldCancel} {
		form.focus = field
		lines := strings.Split(form.View(), "\n")
		if len(lines) != 12 {
			t.Fatalf("field %d rendered %d lines, want 12", field, len(lines))
		}
		for i, line := range lines {
			if width := lipgloss.Width(line); width > 48 {
				t.Fatalf("field %d line %d width = %d, want <= 48", field, i, width)
			}
		}
	}
}

func TestFormShowsOKAndCancelButtons(t *testing.T) {
	form := newFormScreen(&fakeController{}, nil, nil)
	view := ansi.Strip(form.View())

	if !strings.Contains(view, "[ OK ]") || !strings.Contains(view, "[ Cancel ]") {
		t.Fatalf("form buttons are missing:\n%s", view)
	}
}

func TestFormHintChangesForButtons(t *testing.T) {
	form := newFormScreen(&fakeController{}, nil, nil)

	form.focus = fieldSave
	lines := strings.Split(ansi.Strip(form.View()), "\n")
	found := false
	for i, line := range lines {
		if !strings.Contains(line, "[ OK ]") {
			continue
		}
		found = true
		if i+2 >= len(lines) {
			t.Fatalf("form ends after the buttons:\n%s", strings.Join(lines, "\n"))
		}
		if strings.TrimSpace(lines[i+1]) != "" {
			t.Fatalf("buttons are not followed by an empty row:\n%s", strings.Join(lines, "\n"))
		}
		if !strings.Contains(lines[i+2], "save the command and return to the command list") {
			t.Fatalf("save hint is missing:\n%s", strings.Join(lines, "\n"))
		}
		break
	}
	if !found {
		t.Fatalf("form buttons are missing:\n%s", strings.Join(lines, "\n"))
	}

	form.focus = fieldCancel
	if view := ansi.Strip(form.View()); !strings.Contains(view, "discard changes and return to the command list") {
		t.Fatalf("cancel hint is missing:\n%s", view)
	}
}

func TestFormCommandHelpShowsTemplateVariables(t *testing.T) {
	form := newFormScreen(&fakeController{}, nil, nil)
	form.Resize(100, 30)
	form.focus = fieldCommand

	view := ansi.Strip(form.View())
	for _, want := range []string{
		"VARIABLE", "DESCRIPTION",
		"${REMOTE_HOST}", "Remote host (the SSH alias).",
		"${REMOTE_TARGET_KIND}", "folder when the target is a directory, otherwise file.",
		"${@}", "All positional arguments.",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("command help is missing %q:\n%s", want, view)
		}
	}
}

func TestFormAliasHelpShowsExample(t *testing.T) {
	form := newFormScreen(&fakeController{}, nil, nil)
	form.Resize(64, 24)
	form.focus = fieldAlias

	view := ansi.Strip(form.View())
	for _, want := range []string{
		"Adds a shortcut to the remote shell.",
		"alias ed='sshex exec code'",
		"Run ed in a new remote shell to execute the code command.",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("alias help is missing %q:\n%s", want, view)
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

func TestFormDoesNotRepeatFieldLabels(t *testing.T) {
	original := Command{Name: "hello", Template: "echo hi", Alias: "hi"}
	form := newFormScreen(&fakeController{}, []Command{original}, &original)

	view := ansi.Strip(form.View())
	for _, repeated := range []string{"name ›", "command ›", "alias ›"} {
		if strings.Contains(view, repeated) {
			t.Fatalf("field label is repeated as %q:\n%s", repeated, view)
		}
	}
	for _, value := range []string{"name      hello", "command   echo hi", "alias     hi"} {
		if !strings.Contains(view, value) {
			t.Fatalf("field value %q is missing:\n%s", value, view)
		}
	}
}

func TestFormFocusMarkerShowsOnlyFocusedField(t *testing.T) {
	form := newFormScreen(&fakeController{}, nil, nil)

	assertMarker := func(label string) {
		t.Helper()
		view := ansi.Strip(form.View())
		if count := strings.Count(view, "›"); count != 1 {
			t.Fatalf("view has %d focus markers, want 1:\n%s", count, view)
		}
		if !strings.Contains(view, "› "+label) {
			t.Fatalf("focus marker is not on %q:\n%s", label, view)
		}
	}

	assertMarker("name")
	tab(form)
	assertMarker("command")
	tab(form)
	tab(form)
	assertMarker("disabled")
}
