package tunnels

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mcnull/sshex/internal/ui/keys"
	"github.com/mcnull/sshex/internal/ui/screen"
	"github.com/mcnull/sshex/internal/ui/theme"
)

// AddFormOptions configures the add tunnel screen.
type AddFormOptions struct {
	Styles     *theme.Styles
	Controller Controller
	SessionID  string
	Target     string
}

// AddForm is the screen that creates one tunnel for a session.
type AddForm struct {
	styles     *theme.Styles
	controller Controller
	sessionID  string
	target     string
	keys       keys.Tunnels
	input      textinput.Model

	direction  Direction
	err        string
	submitting bool

	width  int
	height int
}

// addedMsg reports the result of adding a tunnel.
type addedMsg struct {
	tunnel Tunnel
	err    error
}

// NewAddForm returns the add tunnel screen.
func NewAddForm(opts AddFormOptions) *AddForm {
	input := textinput.New()
	input.Prompt = "spec › "
	input.Placeholder = "8080:db:5432"
	input.SetStyles(inputStyles(opts.Styles))
	input.Focus()

	return &AddForm{
		styles:     opts.Styles,
		controller: opts.Controller,
		sessionID:  opts.SessionID,
		target:     opts.Target,
		keys:       keys.DefaultTunnels(),
		input:      input,
		direction:  DirectionLocal,
	}
}

// Init implements screen.Screen.
func (f *AddForm) Init() tea.Cmd { return f.input.Focus() }

// Title implements screen.Screen.
func (f *AddForm) Title() string { return "Add tunnel" }

// Keys implements screen.Screen.
func (f *AddForm) Keys() []key.Binding {
	return []key.Binding{f.keys.Submit, f.keys.Direction, keys.Cancel()}
}

// Resize implements screen.Screen.
func (f *AddForm) Resize(width, height int) {
	f.width = width
	f.height = height
	f.input.SetWidth(max(width-lipgloss.Width(f.input.Prompt)-1, 1))
}

// Update implements screen.Screen.
func (f *AddForm) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case addedMsg:
		return f.handleAdded(msg)
	case tea.KeyPressMsg:
		return f.handleKey(msg)
	}
	return f, nil
}

func (f *AddForm) handleKey(press tea.KeyPressMsg) (screen.Screen, tea.Cmd) {
	if f.submitting {
		return f, nil
	}
	switch {
	case key.Matches(press, keys.Cancel()):
		return f, screen.Pop()
	case key.Matches(press, f.keys.Submit):
		return f, f.submit()
	case key.Matches(press, f.keys.Direction):
		f.ToggleDirection()
		return f, nil
	}
	return f, f.update(press)
}

func (f *AddForm) submit() tea.Cmd {
	req, err := f.Request()
	if err != nil {
		return nil
	}
	controller := f.controller
	if controller == nil {
		f.err = "adding tunnels is not available."
		return nil
	}
	sessionID := f.sessionID
	f.submitting = true
	return func() tea.Msg {
		tunnel, err := controller.Add(sessionID, req)
		return addedMsg{tunnel: tunnel, err: err}
	}
}

func (f *AddForm) handleAdded(msg addedMsg) (screen.Screen, tea.Cmd) {
	if msg.err != nil {
		f.submitting = false
		f.err = "could not add tunnel: " + msg.err.Error()
		return f, nil
	}
	return f, screen.Pop()
}

// Request validates the spec and returns the tunnel to create.
func (f *AddForm) Request() (AddRequest, error) {
	req, err := ParseAddRequest(f.input.Value(), f.direction)
	if err != nil {
		f.err = err.Error()
		return AddRequest{}, err
	}
	f.err = ""
	return req, nil
}

func (f *AddForm) ToggleDirection() {
	if f.direction == DirectionRemote {
		f.direction = DirectionLocal
		return
	}
	f.direction = DirectionRemote
}

func (f *AddForm) directionLabel() string {
	if f.direction == DirectionRemote {
		return "remote"
	}
	return "local"
}

func (f *AddForm) sessionLabel() string {
	if f.target != "" {
		return f.target
	}
	if f.sessionID != "" {
		return f.sessionID
	}
	return "session"
}

func (f *AddForm) update(press tea.KeyPressMsg) tea.Cmd {
	before := f.input.Value()
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(press)
	if f.input.Value() != before {
		f.err = ""
	}
	return cmd
}

// View implements screen.Screen.
func (f *AddForm) View() string {
	if f.width <= 0 || f.height <= 0 {
		return ""
	}

	title := truncate("Add tunnel · "+f.sessionLabel()+" · "+f.directionLabel(), f.width)
	lines := []string{
		f.styles.ScreenTitle.Render(title),
		"",
		f.input.View(),
	}

	switch {
	case f.submitting:
		lines = append(lines, f.styles.ScreenHint.Render("adding tunnel…"))
	case f.err != "":
		lines = append(lines, f.styles.Error.Render(truncate(f.err, f.width)))
	default:
		lines = append(lines, f.styles.ScreenHint.Render("port[:target[:remote-port]]"))
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
