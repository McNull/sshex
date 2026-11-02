// Package connect implements the Connect screen of the sshex terminal UI. It
// lists the hosts from the local SSH configuration and starts a session with
// the selected or typed target.
package connect

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mcnull/sshex/internal/sshhosts"
	"github.com/mcnull/sshex/internal/ui/keys"
	"github.com/mcnull/sshex/internal/ui/screen"
	"github.com/mcnull/sshex/internal/ui/theme"
)

const (
	focusSearch = iota
	focusList
)

// HostSource resolves the hosts shown by the screen.
type HostSource interface {
	Resolve() ([]sshhosts.Host, error)
}

// Options configures the Connect screen.
type Options struct {
	Styles *theme.Styles
	Source HostSource
	Runner Runner
}

// Model is the Connect screen.
type Model struct {
	styles *theme.Styles
	source HostSource
	runner Runner
	keys   keys.Connect
	input  textinput.Model

	focus    int
	hosts    []sshhosts.Host
	filtered []sshhosts.Host
	selected int
	offset   int

	loaded    bool
	loading   bool
	loadErr   error
	status    string
	statusErr bool

	width  int
	height int
}

// hostsLoadedMsg carries the result of the asynchronous host lookup.
type hostsLoadedMsg struct {
	hosts []sshhosts.Host
	err   error
}

// New returns a Connect screen.
func New(opts Options) *Model {
	input := textinput.New()
	input.Prompt = "search › "
	input.Placeholder = "host, user@host or part of a name"
	input.SetStyles(inputStyles(opts.Styles))
	input.Focus()

	return &Model{
		styles:  opts.Styles,
		source:  opts.Source,
		runner:  opts.Runner,
		keys:    keys.DefaultConnect(),
		input:   input,
		focus:   focusSearch,
		loading: true,
	}
}

// Init implements screen.Screen. When the screen already loaded its hosts it
// keeps the query and selection from the previous visit and only restarts the
// search cursor.
func (m *Model) Init() tea.Cmd {
	if m.loaded {
		if m.focus == focusSearch {
			return m.input.Focus()
		}
		return nil
	}
	m.loading = true
	m.loadErr = nil
	return tea.Batch(m.loadHosts(), m.input.Focus())
}

// Title implements screen.Screen.
func (m *Model) Title() string { return "Connect" }

// Keys implements screen.Screen.
func (m *Model) Keys() []key.Binding {
	if m.focus == focusList {
		return []key.Binding{m.keys.Up, m.keys.Down, m.keys.Select, m.keys.FocusSearch, keys.Back()}
	}
	return []key.Binding{m.keys.Up, m.keys.Down, m.keys.Select, m.keys.FocusList, keys.Back()}
}

// Resize implements screen.Screen.
func (m *Model) Resize(width, height int) {
	m.width = width
	m.height = height
	m.input.SetWidth(max(width-lipgloss.Width(m.input.Prompt)-1, 1))
	m.clampOffset()
}

// Update implements screen.Screen.
func (m *Model) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case hostsLoadedMsg:
		m.loading = false
		m.loadErr = msg.err
		m.hosts = msg.hosts
		m.loaded = msg.err == nil
		m.refilter()
		return m, nil
	case FinishedMsg:
		m.statusErr = msg.Err != nil
		if msg.Err != nil {
			m.status = "session failed: " + msg.Err.Error()
		} else {
			m.status = "session ended."
		}
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	if m.focus == focusSearch {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) handleKey(press tea.KeyPressMsg) (screen.Screen, tea.Cmd) {
	if key.Matches(press, keys.Back()) {
		return m, screen.Pop()
	}

	if m.focus == focusSearch {
		switch {
		case key.Matches(press, m.keys.FocusList):
			return m, m.setFocus(focusList)
		case key.Matches(press, m.keys.Up):
			m.move(-1)
		case key.Matches(press, m.keys.Down):
			m.move(1)
		case key.Matches(press, m.keys.Select):
			target := strings.TrimSpace(m.input.Value())
			if target == "" {
				target = m.selectedName()
			}
			if target == "" {
				return m, nil
			}
			return m, m.connect(target)
		}

		before := m.input.Value()
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(press)
		if m.input.Value() != before {
			m.refilter()
		}
		return m, cmd
	}

	switch {
	case key.Matches(press, m.keys.FocusSearch):
		return m, m.setFocus(focusSearch)
	case key.Matches(press, m.keys.Backspace):
		return m, m.backspaceSearch()
	case key.Matches(press, m.keys.Up):
		m.move(-1)
	case key.Matches(press, m.keys.Down):
		m.move(1)
	case key.Matches(press, m.keys.Select):
		if target := m.selectedName(); target != "" {
			return m, m.connect(target)
		}
	}
	return m, nil
}

func (m *Model) setFocus(focus int) tea.Cmd {
	if m.focus == focus {
		return nil
	}
	m.focus = focus
	if focus == focusSearch {
		return m.input.Focus()
	}
	m.syncInputToSelection()
	m.input.Blur()
	return nil
}

// syncInputToSelection shows the highlighted host in the search field while
// the list owns the focus. It must not refilter, or the list would collapse to
// the selected host.
func (m *Model) syncInputToSelection() {
	if m.focus != focusList {
		return
	}
	m.input.SetValue(m.selectedName())
	m.input.CursorEnd()
}

// backspaceSearch returns to the search field and removes the last character,
// so editing can start directly from the value shown while browsing the list.
func (m *Model) backspaceSearch() tea.Cmd {
	focusCmd := m.setFocus(focusSearch)
	var inputCmd tea.Cmd
	m.input, inputCmd = m.input.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m.refilter()
	return tea.Batch(focusCmd, inputCmd)
}

func (m *Model) connect(target string) tea.Cmd {
	m.status = ""
	m.statusErr = false
	if m.runner == nil {
		m.status = "connecting is not available."
		m.statusErr = true
		return nil
	}
	return m.runner.Run(target)
}

func (m *Model) loadHosts() tea.Cmd {
	source := m.source
	if source == nil {
		return func() tea.Msg {
			return hostsLoadedMsg{err: fmt.Errorf("no host source configured")}
		}
	}
	return func() tea.Msg {
		hosts, err := source.Resolve()
		return hostsLoadedMsg{hosts: hosts, err: err}
	}
}

func (m *Model) refilter() {
	m.filtered = filterHosts(m.hosts, m.input.Value())
	m.selected = 0
	m.offset = 0
	m.clampOffset()
}

func (m *Model) move(delta int) {
	if len(m.filtered) == 0 {
		return
	}
	m.selected = (m.selected + delta + len(m.filtered)) % len(m.filtered)
	m.clampOffset()
	m.syncInputToSelection()
}

func (m *Model) selectedName() string {
	if m.selected < 0 || m.selected >= len(m.filtered) {
		return ""
	}
	return m.filtered[m.selected].Name
}

func (m *Model) listRows() int {
	// One row for the search field, one for the sticky header and one for the
	// status line.
	return max(m.height-3, 0)
}

func (m *Model) clampOffset() {
	rows := m.listRows()
	if rows <= 0 {
		m.offset = 0
		return
	}
	if m.selected < m.offset {
		m.offset = m.selected
	}
	if m.selected >= m.offset+rows {
		m.offset = m.selected - rows + 1
	}
	maxOffset := max(len(m.filtered)-rows, 0)
	if m.offset > maxOffset {
		m.offset = maxOffset
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

// View implements screen.Screen.
func (m *Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	lines := make([]string, 0, m.height)
	lines = append(lines, m.searchView())
	lines = append(lines, m.headerView())
	lines = append(lines, m.listView()...)

	for len(lines) < m.height-1 {
		lines = append(lines, "")
	}
	lines = append(lines, m.statusView())

	for i := range lines {
		lines[i] = padRight(lines[i], m.width)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) searchView() string {
	return m.input.View()
}

func (m *Model) headerView() string {
	nameWidth := m.nameWidth()
	descWidth := max(m.width-nameWidth-6, 0)
	line := "  " + padRight(truncate("HOST", nameWidth), nameWidth) +
		"  " + padRight(truncate("HOSTNAME", descWidth), descWidth)
	return m.styles.ListHeader.Padding(0, 1).Width(m.width).Render(padRight(line, m.width-2))
}

func (m *Model) listView() []string {
	rows := m.listRows()
	if rows <= 0 {
		return nil
	}

	switch {
	case m.loading:
		return []string{m.styles.ScreenHint.Render("loading hosts…")}
	case m.loadErr != nil:
		return []string{m.styles.Error.Render("could not load hosts: " + m.loadErr.Error())}
	case len(m.filtered) == 0:
		return []string{m.styles.ScreenHint.Render("no matching hosts")}
	}

	nameWidth := m.nameWidth()
	out := make([]string, 0, rows)
	for i := 0; i < rows; i++ {
		index := m.offset + i
		if index >= len(m.filtered) {
			break
		}
		out = append(out, m.hostRow(m.filtered[index], index == m.selected, nameWidth))
	}
	return out
}

func (m *Model) nameWidth() int {
	longest := 0
	for _, host := range m.filtered {
		longest = max(longest, lipgloss.Width(host.Name))
	}
	return min(max(longest+2, 8), max(m.width/2, 8))
}

func (m *Model) hostRow(host sshhosts.Host, selected bool, nameWidth int) string {
	marker := " "
	if selected {
		marker = "›"
	}
	name := padRight(truncate(host.Name, nameWidth), nameWidth)
	descWidth := max(m.width-nameWidth-6, 0)
	desc := padRight(truncate(displayDescription(host), descWidth), descWidth)

	if selected {
		line := padRight(marker+" "+name+"  "+desc, m.width-2)
		return m.selectedRowStyle().Padding(0, 1).Width(m.width).Render(line)
	}

	line := m.styles.ListMarker.Render(marker) + " " +
		m.styles.ListRow.Render(name) + "  " +
		m.styles.ListDescription.Render(desc)
	return m.styles.ListRow.Padding(0, 1).Width(m.width).Render(padRight(line, m.width-2))
}

// displayDescription hides the HostName column when it repeats the host name.
func displayDescription(host sshhosts.Host) string {
	description := strings.TrimSpace(host.Description)
	if strings.EqualFold(description, host.Name) {
		return ""
	}
	return description
}

// selectedRowStyle uses a brighter selection while the list owns the focus and
// a muted one while the search field does.
func (m *Model) selectedRowStyle() lipgloss.Style {
	if m.focus == focusList {
		return m.styles.ListRowSelected
	}
	return m.styles.ListRowSelectedBlurred
}

func (m *Model) statusView() string {
	if m.status != "" {
		if m.statusErr {
			return m.styles.Error.Render(m.status)
		}
		return m.styles.ScreenHint.Render(m.status)
	}
	if m.loading {
		return ""
	}
	if m.loadErr != nil {
		return ""
	}
	count := fmt.Sprintf("%d of %d hosts", len(m.filtered), len(m.hosts))
	return m.styles.ScreenHint.Render(count)
}

// FinishedMsg reports that a session started from the screen has ended.
type FinishedMsg struct {
	Err error
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

func padRight(s string, width int) string {
	if lipgloss.Width(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-lipgloss.Width(s))
}

func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	return string(runes[:width-1]) + "…"
}
