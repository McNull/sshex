package tunnels

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mcnull/sshex/internal/ui/theme"
)

// addForm holds the single spec field and direction of the add form.
type addForm struct {
	input     textinput.Model
	direction Direction
	err       string
}

func newAddForm(styles *theme.Styles) *addForm {
	input := textinput.New()
	input.Prompt = "spec › "
	input.Placeholder = "8080:db:5432"
	input.SetStyles(inputStyles(styles))
	input.Focus()

	return &addForm{
		input:     input,
		direction: DirectionLocal,
	}
}

func (f *addForm) SetWidth(width int) {
	f.input.SetWidth(max(width-lipgloss.Width(f.input.Prompt)-1, 1))
}

func (f *addForm) ToggleDirection() {
	if f.direction == DirectionRemote {
		f.direction = DirectionLocal
		return
	}
	f.direction = DirectionRemote
}

func (f *addForm) directionLabel() string {
	if f.direction == DirectionRemote {
		return "remote"
	}
	return "local"
}

func (f *addForm) update(press tea.KeyPressMsg) tea.Cmd {
	before := f.input.Value()
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(press)
	if f.input.Value() != before {
		f.err = ""
	}
	return cmd
}

// Request validates the spec and returns the tunnel to create.
func (f *addForm) Request() (AddRequest, error) {
	req, err := ParseAddRequest(f.input.Value(), f.direction)
	if err != nil {
		f.err = err.Error()
		return AddRequest{}, err
	}
	f.err = ""
	return req, nil
}

func inputStyles(styles *theme.Styles) textinput.Styles {
	var s textinput.Styles
	s.Focused = textinput.StyleState{
		Text:        styles.InputText,
		Placeholder: styles.InputPlaceholder,
		Suggestion:  styles.InputPlaceholder,
		Prompt:      styles.InputPrompt,
	}
	s.Blurred = s.Focused
	s.Cursor = textinput.CursorStyle{
		Color: styles.Cursor.GetForeground(),
		Shape: tea.CursorBlock,
		Blink: true,
	}
	return s
}
