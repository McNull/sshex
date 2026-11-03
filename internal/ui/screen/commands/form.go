package commands

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	lgtable "charm.land/lipgloss/v2/table"

	"github.com/mcnull/sshex/internal/ui/keys"
	"github.com/mcnull/sshex/internal/ui/screen"
	"github.com/mcnull/sshex/internal/ui/theme"
)

const (
	fieldName = iota
	fieldCommand
	fieldAlias
	fieldDisabled
	fieldSave
	fieldCancel
	fieldCount
)

const (
	formLabelWidth  = 10
	formMarkerWidth = 2
)

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
	nameInput  textinput.Model
	command    textarea.Model
	aliasInput textinput.Model

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

	form.nameInput = newInput(opts.Styles, "code")
	form.aliasInput = newInput(opts.Styles, "optional")
	form.command = newCommandInput(opts.Styles)

	if opts.Original != nil {
		form.nameInput.SetValue(opts.Original.Name)
		form.command.SetValue(opts.Original.Template)
		form.aliasInput.SetValue(opts.Original.Alias)
		form.disabled = opts.Original.Disabled
	}
	form.nameInput.CursorStart()
	form.command.MoveToBegin()
	form.aliasInput.CursorStart()
	form.nameInput.Focus()
	return form
}

func newCommandInput(styles *theme.Styles) textarea.Model {
	input := textarea.New()
	input.Prompt = ""
	input.Placeholder = `code --${REMOTE_TARGET_KIND}-uri "..."`
	input.ShowLineNumbers = false
	input.MaxWidth = 0
	input.MaxContentHeight = 10000
	input.DynamicHeight = true
	input.MinHeight = 1
	input.MaxHeight = 3
	input.SetStyles(textareaStyles(styles))
	input.SetHeight(1)
	return input
}

func newInput(styles *theme.Styles, placeholder string) textinput.Model {
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = placeholder
	input.SetStyles(inputStyles(styles))
	return input
}

// Init implements screen.Screen.
func (f *Form) Init() tea.Cmd {
	return f.focusInput(f.focus)
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
func (f *Form) CanOpenHelp() bool {
	switch f.focus {
	case fieldName, fieldCommand, fieldAlias:
		return false
	default:
		return true
	}
}

// Keys implements screen.Screen.
func (f *Form) Keys() []keys.Binding {
	bindings := []keys.Binding{f.keys.NextField}
	switch f.focus {
	case fieldCommand:
		bindings = append(bindings, keys.NewBinding(
			keys.WithKeys("enter"),
			keys.WithHelp("enter", "newline", "Insert a newline in the command."),
		))
	case fieldDisabled:
		bindings = append(bindings, f.keys.Toggle, f.keys.Save)
	case fieldSave:
		bindings = append(bindings, f.keys.Save)
	case fieldCancel:
		bindings = append(bindings, keys.NewBinding(
			keys.WithKeys("enter", "space"),
			keys.WithHelp("enter", "cancel", "Discard the changes and return to the command list."),
		))
	}
	return append(bindings, keys.Cancel())
}

// Resize implements screen.Screen.
func (f *Form) Resize(width, height int) {
	f.width = width
	f.height = height

	f.nameInput.SetWidth(f.textInputWidth(f.nameInput))
	f.aliasInput.SetWidth(f.textInputWidth(f.aliasInput))
	f.command.MaxHeight = min(3, max(height-9, 1))
	f.command.SetWidth(max(width-formMarkerWidth-formLabelWidth-1, 1))
}

func (f *Form) textInputWidth(input textinput.Model) int {
	return max(f.width-formMarkerWidth-formLabelWidth-lipgloss.Width(input.Prompt)-1, 1)
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
	case keys.Matches(press, f.keys.NextField):
		return f, f.setFocus((f.focus + 1) % fieldCount)
	case keys.Matches(press, f.keys.PreviousField):
		return f, f.setFocus((f.focus + fieldCount - 1) % fieldCount)
	}

	switch f.focus {
	case fieldCommand:
		return f.updateCommand(press)
	case fieldSave:
		if f.isActivate(press) {
			return f, f.submit()
		}
	case fieldCancel:
		if f.isActivate(press) {
			return f, screen.Pop()
		}
	case fieldDisabled:
		switch {
		case keys.Matches(press, f.keys.Toggle):
			f.disabled = !f.disabled
			f.err = ""
		case keys.Matches(press, f.keys.Save):
			return f, f.submit()
		}
	default:
		if keys.Matches(press, f.keys.Save) {
			return f, f.setFocus((f.focus + 1) % fieldCount)
		}
		if cmd := f.updateTextField(press); cmd != nil {
			return f, cmd
		}
	}
	return f, nil
}

func (f *Form) isActivate(press tea.KeyPressMsg) bool {
	return keys.Matches(press, f.keys.Save) || keys.Matches(press, f.keys.Toggle)
}

func (f *Form) updateTextField(press tea.KeyPressMsg) tea.Cmd {
	switch f.focus {
	case fieldName:
		before := f.nameInput.Value()
		var cmd tea.Cmd
		f.nameInput, cmd = f.nameInput.Update(press)
		if f.nameInput.Value() != before {
			f.err = ""
		}
		return cmd
	case fieldAlias:
		before := f.aliasInput.Value()
		var cmd tea.Cmd
		f.aliasInput, cmd = f.aliasInput.Update(press)
		if f.aliasInput.Value() != before {
			f.err = ""
		}
		return cmd
	default:
		return nil
	}
}

func (f *Form) updateCommand(press tea.KeyPressMsg) (screen.Screen, tea.Cmd) {
	before := f.command.Value()
	var cmd tea.Cmd
	f.command, cmd = f.command.Update(press)
	if f.command.Value() != before {
		f.err = ""
	}
	return f, cmd
}

func (f *Form) setFocus(focus int) tea.Cmd {
	if f.focus == focus {
		return nil
	}
	f.blurInput(f.focus)
	f.focus = focus
	return f.focusInput(focus)
}

func (f *Form) focusInput(field int) tea.Cmd {
	switch field {
	case fieldName:
		f.nameInput.CursorStart()
		return f.nameInput.Focus()
	case fieldCommand:
		f.command.MoveToBegin()
		return f.command.Focus()
	case fieldAlias:
		f.aliasInput.CursorStart()
		return f.aliasInput.Focus()
	default:
		return nil
	}
}

func (f *Form) blurInput(field int) {
	switch field {
	case fieldName:
		f.nameInput.Blur()
	case fieldCommand:
		f.command.Blur()
	case fieldAlias:
		f.aliasInput.Blur()
	}
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
		Name:     f.nameInput.Value(),
		Template: f.command.Value(),
		Alias:    f.aliasInput.Value(),
		Disabled: f.disabled,
	}
}

// View implements screen.Screen.
func (f *Form) View() string {
	if f.width <= 0 || f.height <= 0 {
		return ""
	}

	f.refreshInputStyles()

	lines := []string{
		f.inputLine("name", fieldName, f.nameInput),
	}
	lines = append(lines, f.commandLines()...)
	lines = append(lines,
		f.inputLine("alias", fieldAlias, f.aliasInput),
		f.disabledLine(),
		"",
		f.buttonsLine(),
		"",
	)

	switch {
	case f.submitting:
		lines = append(lines, f.styles.ScreenHint.Render("saving command…"))
	case f.err != "":
		lines = append(lines, f.styles.Error.Render(truncate(f.err, f.width)))
	default:
		lines = append(lines, f.styles.ScreenHint.Render(truncate(f.fieldHint(), f.width)))
		lines = append(lines, f.fieldHelpLines()...)
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

func (f *Form) fieldLabel(label string, field int) string {
	marker := "  "
	style := f.styles.ScreenSubtitle
	if f.focus == field {
		marker = "› "
		style = f.styles.MenuRowSelected
	}
	return style.Render(marker + padRight(label, formLabelWidth))
}

func (f *Form) refreshInputStyles() {
	f.nameInput.SetStyles(inputStyles(f.styles))
	f.aliasInput.SetStyles(inputStyles(f.styles))
	f.command.SetStyles(textareaStyles(f.styles))
}

func (f *Form) inputLine(label string, field int, input textinput.Model) string {
	return f.fieldLabel(label, field) + input.View()
}

func (f *Form) commandLines() []string {
	view := strings.TrimRight(f.command.View(), "\n")
	lines := strings.Split(view, "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}

	result := make([]string, 0, len(lines))
	result = append(result, f.fieldLabel("command", fieldCommand)+lines[0])
	indent := strings.Repeat(" ", formMarkerWidth+formLabelWidth)
	for _, line := range lines[1:] {
		result = append(result, indent+line)
	}
	return result
}

func (f *Form) buttonsLine() string {
	ok := f.button(" OK ", f.focus == fieldSave)
	cancel := f.button(" Cancel ", f.focus == fieldCancel)
	return strings.Repeat(" ", formMarkerWidth+formLabelWidth) + "[" + ok + "]  [" + cancel + "]"
}

func (f *Form) button(label string, focused bool) string {
	style := f.styles.ScreenSubtitle
	if focused {
		style = f.styles.MenuRowSelected
	}
	return style.Render(label)
}

func (f *Form) disabledLine() string {
	mark := " "
	if f.disabled {
		mark = "x"
	}
	value := fmt.Sprintf("[%s] %s", mark, yesNo(f.disabled))
	if f.focus == fieldDisabled {
		line := "› " + padRight("disabled", formLabelWidth) + value
		return f.styles.MenuRowSelected.Render(padRight(line, f.width))
	}
	return f.fieldLabel("disabled", fieldDisabled) + value
}

func textareaStyles(styles *theme.Styles) textarea.Styles {
	normal := textarea.StyleState{
		Base:        lipgloss.NewStyle(),
		Text:        styles.InputText,
		Placeholder: styles.InputPlaceholder,
		Prompt:      styles.InputPrompt,
		CursorLine:  lipgloss.NewStyle(),
		EndOfBuffer: lipgloss.NewStyle(),
		Selection:   styles.MenuRowSelected,
	}
	result := textarea.Styles{Focused: normal, Blurred: normal}
	result.Cursor = textarea.CursorStyle{
		Color: styles.Cursor.GetForeground(),
		Shape: tea.CursorBlock,
		Blink: true,
	}
	return result
}

func (f *Form) fieldHelpLines() []string {
	switch f.focus {
	case fieldCommand:
		return f.commandHelpLines()
	case fieldAlias:
		return f.aliasHelpLines()
	default:
		return nil
	}
}

func (f *Form) commandHelpLines() []string {
	rows := [][]string{
		{"${REMOTE_CWD}", "Remote working directory."},
		{"${REMOTE_USER}", "Remote user."},
		{"${REMOTE_HOST}", "Remote host (the SSH alias)."},
		{"${REMOTE_PORT}", "Remote port."},
		{"${REMOTE_SESSION}", "Session id."},
		{"${REMOTE_ORIGIN}", "Origin machine."},
		{"${REMOTE_TARGET}", "Resolved target path, or remote working directory."},
		{"${REMOTE_TARGET_KIND}", "folder when the target is a directory, otherwise file."},
		{"${0}", "Command name."},
		{"${1}..${10}", "Positional arguments."},
		{"${@}", "All positional arguments."},
	}
	table := lgtable.New().
		Headers("VARIABLE", "DESCRIPTION").
		Rows(rows...).
		Border(lipgloss.HiddenBorder()).
		BorderStyle(f.styles.ScreenHint).
		StyleFunc(func(row, col int) lipgloss.Style {
			switch {
			case row == lgtable.HeaderRow:
				return f.styles.ListHeader
			case col == 0:
				return f.styles.Accent
			default:
				return f.styles.ScreenHint
			}
		}).
		Width(max(f.width, 1))
	return strings.Split(strings.TrimRight(table.Render(), "\n"), "\n")
}

func (f *Form) aliasHelpLines() []string {
	lines := []string{
		"Adds a shortcut to the remote shell.",
		"",
		"Example:",
		"  alias ed='sshex exec code'",
		"Run ed in a new remote shell to execute the code command.",
	}
	for i, line := range lines {
		lines[i] = f.styles.ScreenHint.Render(truncate(line, f.width))
	}
	return lines
}

func (f *Form) fieldHint() string {
	switch f.focus {
	case fieldName:
		return "the name used by sshex exec"
	case fieldCommand:
		return "runs on the origin via sh -c · ${REMOTE_*}, ${@} expanded"
	case fieldAlias:
		return "optional shell alias in the remote shell"
	case fieldSave:
		return "save the command and return to the command list"
	case fieldCancel:
		return "discard changes and return to the command list"
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
