package commands

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mcnull/sshex/internal/ui/keys"
	"github.com/mcnull/sshex/internal/ui/screen"
	"github.com/mcnull/sshex/internal/ui/theme"
)

const (
	fieldName = iota
	fieldCommand
	fieldAlias
	fieldDisabled
	fieldCount
)

const formLabelWidth = 10

// FormOptions configures the add/edit command screen. Original is nil when
// adding; Existing is the current command set used for uniqueness checks.
type FormOptions struct {
	Styles     *theme.Styles
	Controller Controller
	Existing   []Command
	Original   *Command
}

// Form is the screen that creates or edits one command.
type Form struct {
	styles     *theme.Styles
	controller Controller
	existing   []Command
	original   *Command
	keys       keys.Commands
	inputs     [fieldCount - 1]textinput.Model

	focus      int
	disabled   bool
	err        string
	submitting bool

	width  int
	height int
}

// savedMsg reports the result of saving a command.
type savedMsg struct {
	name string
	err  error
}

// NewForm returns the add/edit command screen.
func NewForm(opts FormOptions) *Form {
	form := &Form{
		styles:     opts.Styles,
		controller: opts.Controller,
		existing:   append([]Command(nil), opts.Existing...),
		original:   opts.Original,
		keys:       keys.DefaultCommands(),
	}

	form.inputs[fieldName] = newInput(opts.Styles, "name › ", "code")
	form.inputs[fieldCommand] = newInput(opts.Styles, "command › ", "code --folder-uri \"...\"")
	form.inputs[fieldAlias] = newInput(opts.Styles, "alias › ", "optional")

	if opts.Original != nil {
		form.inputs[fieldName].SetValue(opts.Original.Name)
		form.inputs[fieldCommand].SetValue(opts.Original.Template)
		form.inputs[fieldAlias].SetValue(opts.Original.Alias)
		form.disabled = opts.Original.Disabled
	}
	form.inputs[fieldName].Focus()
	return form
}

func newInput(styles *theme.Styles, prompt, placeholder string) textinput.Model {
	input := textinput.New()
	input.Prompt = prompt
	input.Placeholder = placeholder
	input.SetStyles(inputStyles(styles))
	return input
}

// Init implements screen.Screen.
func (f *Form) Init() tea.Cmd {
	if f.focus >= len(f.inputs) {
		return nil
	}
	return f.inputs[f.focus].Focus()
}

// Title implements screen.Screen.
func (f *Form) Title() string {
	if f.original == nil {
		return "Add command"
	}
	return "Edit command"
}

// HelpText implements screen.HelpProvider.
func (f *Form) HelpText() string { return "Edit the fields, then save the command." }

// CanOpenHelp implements screen.HelpKeyAvailability.
func (f *Form) CanOpenHelp() bool { return f.focus == fieldDisabled }

// Keys implements screen.Screen.
func (f *Form) Keys() []keys.Binding {
	bindings := []keys.Binding{f.keys.NextField}
	if f.focus == fieldDisabled {
		bindings = append(bindings, f.keys.Toggle)
	}
	return append(bindings, f.keys.Save, keys.Cancel())
}

// Resize implements screen.Screen.
func (f *Form) Resize(width, height int) {
	f.width = width
	f.height = height
	for i := range f.inputs {
		promptWidth := lipgloss.Width(f.inputs[i].Prompt)
		f.inputs[i].SetWidth(max(width-formLabelWidth-promptWidth-1, 1))
	}
}

// Update implements screen.Screen.
func (f *Form) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case savedMsg:
		return f.handleSaved(msg)
	case tea.KeyPressMsg:
		return f.handleKey(msg)
	}
	return f, nil
}

func (f *Form) handleKey(press tea.KeyPressMsg) (screen.Screen, tea.Cmd) {
	if f.submitting {
		return f, nil
	}
	switch {
	case keys.Matches(press, keys.Cancel()):
		return f, screen.Pop()
	case keys.Matches(press, f.keys.Save):
		return f, f.submit()
	case keys.Matches(press, f.keys.NextField):
		return f, f.setFocus((f.focus + 1) % fieldCount)
	case keys.Matches(press, f.keys.PreviousField):
		return f, f.setFocus((f.focus + fieldCount - 1) % fieldCount)
	case f.focus == fieldDisabled && keys.Matches(press, f.keys.Toggle):
		f.disabled = !f.disabled
		f.err = ""
		return f, nil
	}

	if f.focus == fieldDisabled {
		return f, nil
	}
	before := f.inputs[f.focus].Value()
	var cmd tea.Cmd
	f.inputs[f.focus], cmd = f.inputs[f.focus].Update(press)
	if f.inputs[f.focus].Value() != before {
		f.err = ""
	}
	return f, cmd
}

func (f *Form) setFocus(focus int) tea.Cmd {
	if f.focus == focus {
		return nil
	}
	if f.focus < len(f.inputs) {
		f.inputs[f.focus].Blur()
	}
	f.focus = focus
	if focus >= len(f.inputs) {
		return nil
	}
	return f.inputs[focus].Focus()
}

func (f *Form) submit() tea.Cmd {
	command := f.Command()
	self := ""
	if f.original != nil {
		self = f.original.Name
	}
	if invalid := validateCommand(f.existing, self, command); invalid != nil {
		f.err = invalid.Error()
		return f.setFocus(invalid.field)
	}
	f.err = ""

	controller := f.controller
	if controller == nil {
		f.err = "editing commands is not available."
		return nil
	}

	original := ""
	editing := f.original != nil
	if editing {
		original = f.original.Name
	}
	f.submitting = true
	return func() tea.Msg {
		var err error
		if editing {
			err = controller.Update(original, command)
		} else {
			err = controller.Add(command)
		}
		return savedMsg{name: command.Name, err: err}
	}
}

func (f *Form) handleSaved(msg savedMsg) (screen.Screen, tea.Cmd) {
	if msg.err != nil {
		f.submitting = false
		f.err = "could not save command: " + msg.err.Error()
		return f, nil
	}
	return f, screen.Pop()
}

// Command returns the value currently held by the form.
func (f *Form) Command() Command {
	return Command{
		Name:     f.inputs[fieldName].Value(),
		Template: f.inputs[fieldCommand].Value(),
		Alias:    f.inputs[fieldAlias].Value(),
		Disabled: f.disabled,
	}
}

// View implements screen.Screen.
func (f *Form) View() string {
	if f.width <= 0 || f.height <= 0 {
		return ""
	}

	title := "Add command"
	if f.original != nil {
		title = "Edit command · " + f.original.Name
	}

	lines := []string{
		f.styles.ScreenTitle.Render(truncate(title, f.width)),
		"",
		f.inputLine("name", fieldName),
		f.inputLine("command", fieldCommand),
		f.inputLine("alias", fieldAlias),
		f.disabledLine(),
		"",
	}

	switch {
	case f.submitting:
		lines = append(lines, f.styles.ScreenHint.Render("saving command…"))
	case f.err != "":
		lines = append(lines, f.styles.Error.Render(truncate(f.err, f.width)))
	default:
		lines = append(lines, f.styles.ScreenHint.Render(truncate(f.fieldHint(), f.width)))
	}

	for len(lines) < f.height {
		lines = append(lines, "")
	}
	if len(lines) > f.height {
		lines = lines[:f.height]
	}
	for i := range lines {
		lines[i] = padRight(lines[i], f.width)
	}
	return strings.Join(lines, "\n")
}

func (f *Form) inputLine(label string, field int) string {
	return f.styles.ScreenSubtitle.Render(padRight(label, formLabelWidth)) + f.inputs[field].View()
}

func (f *Form) disabledLine() string {
	mark := " "
	if f.disabled {
		mark = "x"
	}
	value := fmt.Sprintf("[%s] %s", mark, yesNo(f.disabled))
	line := f.styles.ScreenSubtitle.Render(padRight("disabled", formLabelWidth)) + value
	if f.focus == fieldDisabled {
		return f.styles.Accent.Render(line)
	}
	return line
}

func (f *Form) fieldHint() string {
	switch f.focus {
	case fieldName:
		return "the name used by sshex exec"
	case fieldCommand:
		return "runs on the origin via sh -c · ${REMOTE_*}, ${@} expanded"
	case fieldAlias:
		return "optional shell alias in the remote shell"
	default:
		return "disabled commands are listed but cannot be executed"
	}
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
