package tunnels

import (
	"fmt"
	"strconv"
	"strings"

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
	PortSource PortSource
	SessionID  string
	Target     string
}

type formFocus int

const (
	focusSpec formFocus = iota
	focusPorts
)

// AddForm is the screen that creates one tunnel for a session.
type AddForm struct {
	styles     *theme.Styles
	controller Controller
	portSource PortSource
	sessionID  string
	target     string
	keys       keys.Tunnels
	input      textinput.Model

	direction  Direction
	focus      formFocus
	err        string
	submitting bool

	ports        []Port
	portSelected int
	portOffset   int
	portsLoading bool
	portsLoaded  bool
	portsErr     string
	portEpoch    uint64

	width  int
	height int
}

// addedMsg reports the result of adding a tunnel.
type addedMsg struct {
	tunnel Tunnel
	err    error
}

// portsLoadedMsg carries one port-list result. The epoch discards a result
// requested before the direction changed.
type portsLoadedMsg struct {
	ports []Port
	host  PortHost
	epoch uint64
	err   error
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
		portSource: opts.PortSource,
		sessionID:  opts.SessionID,
		target:     opts.Target,
		keys:       keys.DefaultTunnels(),
		input:      input,
		direction:  DirectionLocal,
		focus:      focusSpec,
	}
}

// Init implements screen.Screen. It loads the ports of the target side.
func (f *AddForm) Init() tea.Cmd {
	focus := f.input.Focus()
	load := f.loadPorts(true)
	if load == nil {
		return focus
	}
	return tea.Batch(load, focus)
}

// Title implements screen.Screen.
func (f *AddForm) Title() string { return "Add tunnel" }

// HelpText implements screen.HelpProvider.
func (f *AddForm) HelpText() string {
	return "Enter port[:target[:remote-port]] and choose the forwarding direction."
}

// CanOpenHelp implements screen.HelpKeyAvailability.
func (f *AddForm) CanOpenHelp() bool { return f.focus == focusPorts }

// Keys implements screen.Screen.
func (f *AddForm) Keys() []keys.Binding {
	if f.focus == focusPorts {
		return []keys.Binding{
			f.keys.Up, f.keys.Down, f.keys.SelectPort, f.keys.Direction,
			f.keys.FocusSpec, f.keys.RefreshPorts, keys.Cancel(),
		}
	}
	bindings := []keys.Binding{f.keys.Submit, f.keys.Direction}
	if f.portSource != nil {
		bindings = append(bindings, f.keys.FocusPorts)
	}
	return append(bindings, keys.Cancel())
}

// Resize implements screen.Screen.
func (f *AddForm) Resize(width, height int) {
	f.width = width
	f.height = height
	f.input.SetWidth(max(width-lipgloss.Width(f.input.Prompt)-1, 1))
	f.clampPortOffset()
}

// Update implements screen.Screen.
func (f *AddForm) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case addedMsg:
		return f.handleAdded(msg)
	case portsLoadedMsg:
		return f.handlePortsLoaded(msg)
	case tea.KeyPressMsg:
		return f.handleKey(msg)
	default:
		var cmd tea.Cmd
		f.input, cmd = f.input.Update(msg)
		return f, cmd
	}
}

func (f *AddForm) handleKey(press tea.KeyPressMsg) (screen.Screen, tea.Cmd) {
	if f.submitting {
		return f, nil
	}
	if keys.Matches(press, keys.Cancel()) {
		return f, screen.Pop()
	}
	if f.portSource != nil && (keys.Matches(press, f.keys.FocusPorts) || keys.Matches(press, f.keys.FocusSpec)) {
		return f, f.toggleFocus()
	}
	if keys.Matches(press, f.keys.Direction) {
		f.ToggleDirection()
		return f, f.loadPorts(true)
	}
	if f.focus == focusPorts {
		switch {
		case keys.Matches(press, f.keys.Up):
			f.movePort(-1)
		case keys.Matches(press, f.keys.Down):
			f.movePort(1)
		case keys.Matches(press, f.keys.SelectPort):
			return f, f.selectPort()
		case keys.Matches(press, f.keys.RefreshPorts):
			return f, f.loadPorts(false)
		case keys.Matches(press, f.keys.Backspace):
			f.focus = focusSpec
			return f, f.input.Focus()
		}
		return f, nil
	}
	if keys.Matches(press, f.keys.Submit) {
		return f, f.submit()
	}
	return f, f.update(press)
}

func (f *AddForm) toggleFocus() tea.Cmd {
	if f.focus == focusPorts {
		f.focus = focusSpec
		return f.input.Focus()
	}
	f.focus = focusPorts
	f.input.Blur()
	return nil
}

// selectPort writes the highlighted port into the spec field and returns to
// it with the cursor at the end.
func (f *AddForm) selectPort() tea.Cmd {
	if f.portSelected < 0 || f.portSelected >= len(f.ports) {
		return nil
	}
	f.input.SetValue(strconv.Itoa(f.ports[f.portSelected].Port))
	f.input.CursorEnd()
	f.err = ""
	f.focus = focusSpec
	return f.input.Focus()
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

func (f *AddForm) loadPorts(clear bool) tea.Cmd {
	source := f.portSource
	if source == nil {
		return nil
	}
	if clear {
		f.ports = nil
		f.portSelected = 0
		f.portOffset = 0
		f.portsLoaded = false
	}
	f.portEpoch++
	epoch := f.portEpoch
	host := f.portHost()
	sessionID := f.sessionID
	f.portsLoading = true
	f.portsErr = ""
	return func() tea.Msg {
		ports, err := source.List(sessionID, host)
		return portsLoadedMsg{ports: ports, host: host, epoch: epoch, err: err}
	}
}

func (f *AddForm) handlePortsLoaded(msg portsLoadedMsg) (screen.Screen, tea.Cmd) {
	if msg.epoch != f.portEpoch {
		return f, nil
	}
	currentPort := 0
	if f.portSelected >= 0 && f.portSelected < len(f.ports) {
		currentPort = f.ports[f.portSelected].Port
	}
	f.portsLoading = false
	f.portsLoaded = true
	if msg.err != nil {
		f.portsErr = "could not list ports: " + msg.err.Error()
		return f, nil
	}
	f.portsErr = ""
	f.ports = msg.ports
	f.portSelected = 0
	if currentPort != 0 {
		for i, port := range f.ports {
			if port.Port == currentPort {
				f.portSelected = i
				break
			}
		}
	}
	f.clampPortOffset()
	return f, nil
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

func (f *AddForm) portHost() PortHost {
	return portHostForDirection(f.direction)
}

func (f *AddForm) portHostLabel() string {
	if f.portHost() == PortHostLocal {
		return "local"
	}
	return f.sessionLabel()
}

func (f *AddForm) movePort(delta int) {
	if len(f.ports) == 0 {
		return
	}
	next := f.portSelected + delta
	if next < 0 {
		next = 0
	}
	if next >= len(f.ports) {
		next = len(f.ports) - 1
	}
	f.portSelected = next
	f.clampPortOffset()
}

// portRowCount is the number of port lines that fit below the spec.
func (f *AddForm) portRowCount() int {
	return max(f.height-6, 0)
}

func (f *AddForm) clampPortOffset() {
	rows := f.portRowCount()
	if rows <= 0 {
		f.portOffset = 0
		return
	}
	if f.portSelected < f.portOffset {
		f.portOffset = f.portSelected
	}
	if f.portSelected >= f.portOffset+rows {
		f.portOffset = f.portSelected - rows + 1
	}
	maxOffset := max(len(f.ports)-rows, 0)
	if f.portOffset > maxOffset {
		f.portOffset = maxOffset
	}
	if f.portOffset < 0 {
		f.portOffset = 0
	}
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

	context := truncate(f.sessionLabel()+" · "+f.directionLabel(), f.width)
	lines := []string{
		f.styles.ScreenHint.Render(context),
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

	if f.portSource != nil {
		lines = append(lines, f.portSection(f.height-4)...)
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

func (f *AddForm) portSection(limit int) []string {
	if limit < 3 {
		return nil
	}
	section := []string{"", f.portHeaderView()}
	return append(section, f.portBodyView(limit-2)...)
}

func (f *AddForm) portHeaderView() string {
	inner := max(f.width-2, 0)
	left := "  PORTS"
	right := f.portHostLabel()
	gap := max(inner-lipgloss.Width(left)-lipgloss.Width(right), 1)
	line := truncate(left+strings.Repeat(" ", gap)+right, inner)
	return f.styles.ListHeader.Padding(0, 1).Width(f.width).Render(padRight(line, inner))
}

func (f *AddForm) portBodyView(rows int) []string {
	if rows <= 0 {
		return nil
	}
	switch {
	case f.portsErr != "":
		return []string{f.styles.Error.Render(truncate(f.portsErr, f.width))}
	case f.portsLoading && len(f.ports) == 0:
		return []string{f.styles.ScreenHint.Render("loading ports…")}
	case !f.portsLoaded && len(f.ports) == 0:
		return nil
	case len(f.ports) == 0:
		return []string{f.styles.ScreenHint.Render("no listening ports")}
	}

	lines := make([]string, 0, rows)
	for i := 0; i < rows; i++ {
		index := f.portOffset + i
		if index >= len(f.ports) {
			break
		}
		lines = append(lines, f.portRow(f.ports[index], index == f.portSelected))
	}
	return lines
}

func (f *AddForm) portRow(port Port, selected bool) string {
	inner := max(f.width-2, 0)
	marker := " "
	if selected {
		marker = "›"
	}

	portText := fmt.Sprintf("%5d", port.Port)
	process := port.Process
	if process == "" {
		process = "-"
	}
	detail := process
	if port.PID > 0 {
		detail += " · " + strconv.Itoa(port.PID)
	}

	left := marker + " " + portText
	detailWidth := max(inner-lipgloss.Width(left)-2, 1)
	line := left + "  " + truncate(detail, detailWidth)
	if selected {
		style := f.styles.ListRowSelected
		if f.focus != focusPorts {
			style = f.styles.ListRowSelectedBlurred
		}
		return style.Padding(0, 1).Width(f.width).Render(padRight(line, inner))
	}

	row := f.styles.ListMarker.Render(marker) + " " + f.styles.ListRow.Render(portText) + "  " +
		f.styles.ListDescription.Render(truncate(detail, detailWidth))
	return f.styles.ListRow.Padding(0, 1).Width(f.width).Render(padRight(row, inner))
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
