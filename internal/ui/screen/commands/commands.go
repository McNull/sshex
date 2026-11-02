package commands

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mcnull/sshex/internal/ui/keys"
	"github.com/mcnull/sshex/internal/ui/screen"
	"github.com/mcnull/sshex/internal/ui/theme"
)

const refreshInterval = 5 * time.Second

const (
	focusSearch = iota
	focusList
)

// Options configures the Commands screen. AddOpener and EditOpener supply the
// form screens, so the list never constructs them or reaches the controller's
// storage. Existing commands are passed to the form for uniqueness checks.
type Options struct {
	Styles     *theme.Styles
	Source     Source
	Controller Controller
	AddOpener  func(existing []Command) screen.Screen
	EditOpener func(existing []Command, original Command) screen.Screen
}

// Model is the Commands screen.
type Model struct {
	styles     *theme.Styles
	source     Source
	controller Controller
	addOpener  func([]Command) screen.Screen
	editOpener func([]Command, Command) screen.Screen
	keys       keys.Commands
	input      textinput.Model

	focus    int
	commands []Command
	filtered []Command
	selected int
	offset   int
	query    string
	editable bool

	loaded    bool
	loading   bool
	inflight  bool
	loadErr   error
	epoch     uint64
	loadEpoch uint64

	status    string
	statusErr bool

	confirming  bool
	confirmName string

	width  int
	height int
}

// loadedMsg carries the result of an asynchronous command lookup. The epoch
// discards a result requested during an earlier visit.
type loadedMsg struct {
	snapshot Snapshot
	err      error
	epoch    uint64
}

// tickMsg asks the screen to refresh its list. The epoch ignores ticks that
// were scheduled during an earlier visit and would otherwise start a second
// polling chain.
type tickMsg struct {
	epoch uint64
}

// removedMsg reports the result of removing a command.
type removedMsg struct {
	name string
	err  error
}

// toggledMsg reports the result of enabling or disabling a command.
type toggledMsg struct {
	name     string
	disabled bool
	err      error
}

// New returns a Commands screen.
func New(opts Options) *Model {
	input := textinput.New()
	input.Prompt = "search › "
	input.Placeholder = "name or alias"
	input.SetStyles(inputStyles(opts.Styles))

	return &Model{
		styles:     opts.Styles,
		source:     opts.Source,
		controller: opts.Controller,
		addOpener:  opts.AddOpener,
		editOpener: opts.EditOpener,
		keys:       keys.DefaultCommands(),
		input:      input,
		// The list owns the focus on entry so the mutation keys work without
		// tabbing first. Tab moves to the search field.
		focus:   focusList,
		loading: true,
	}
}

// Init implements screen.Screen. It reloads the list on every visit so the
// screen reflects commands that were added or removed elsewhere.
func (m *Model) Init() tea.Cmd {
	// A request from a previous visit may have completed while another screen
	// was active, so its result never reached this model.
	m.inflight = false
	m.status = ""
	m.statusErr = false
	m.clearConfirmation()
	if m.focus == focusSearch {
		return tea.Batch(m.load(!m.loaded), m.startPolling(), m.input.Focus())
	}
	return tea.Batch(m.load(!m.loaded), m.startPolling())
}

// Resume implements screen.Resumable. It refreshes the list and restarts the
// poll loop after a child screen is popped.
func (m *Model) Resume() tea.Cmd {
	m.inflight = false
	return tea.Batch(m.load(false), m.startPolling())
}

// Title implements screen.Screen.
func (m *Model) Title() string { return "Commands" }

// Keys implements screen.Screen.
func (m *Model) Keys() []key.Binding {
	if m.confirming {
		return []key.Binding{keys.Confirm(), keys.Cancel()}
	}
	if m.focus == focusList {
		screenKeys := []key.Binding{m.keys.Navigate, m.keys.Search}
		if m.mutationsAvailable() {
			// Keep the stateful toggle help visible at narrow widths; edit and
			// refresh follow it.
			screenKeys = append(screenKeys, m.keys.AddRemove, m.toggleBinding(), m.keys.Edit)
		}
		return append(screenKeys, m.keys.Refresh, keys.Back())
	}
	screenKeys := []key.Binding{m.keys.SearchUp, m.keys.SearchDown}
	if m.mutationsAvailable() {
		screenKeys = append(screenKeys, m.keys.Edit)
	}
	return append(screenKeys, m.keys.FocusList, keys.Back())
}

// toggleBinding names the action space performs for the selected command so
// the footer reads enable or disable instead of a generic toggle.
func (m *Model) toggleBinding() key.Binding {
	action := "toggle"
	if command, ok := m.selectedCommand(); ok {
		action = "disable"
		if command.Disabled {
			action = "enable"
		}
	}
	return key.NewBinding(
		key.WithKeys("space"),
		key.WithHelp("space", action),
	)
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
	case loadedMsg:
		return m.handleLoaded(msg)
	case removedMsg:
		return m.handleRemoved(msg)
	case toggledMsg:
		return m.handleToggled(msg)
	case tickMsg:
		if msg.epoch != m.epoch {
			return m, nil
		}
		return m, tea.Batch(m.load(false), m.tick())
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleKey(press tea.KeyPressMsg) (screen.Screen, tea.Cmd) {
	if m.confirming {
		switch {
		case key.Matches(press, keys.Confirm()):
			return m, m.removeSelected()
		case key.Matches(press, keys.Cancel()):
			m.clearConfirmation()
			m.status = ""
			m.statusErr = false
		}
		return m, nil
	}

	if key.Matches(press, keys.Back()) {
		return m, screen.Pop()
	}

	if m.focus == focusSearch {
		switch {
		case key.Matches(press, m.keys.FocusList):
			return m, m.setFocus(focusList)
		case key.Matches(press, m.keys.SearchUp):
			m.move(-1)
		case key.Matches(press, m.keys.SearchDown):
			m.move(1)
		case key.Matches(press, m.keys.Edit):
			return m.openEdit()
		}

		before := m.input.Value()
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(press)
		if m.input.Value() != before {
			m.query = m.input.Value()
			m.applyFilter()
		}
		return m, cmd
	}

	switch {
	case key.Matches(press, m.keys.FocusSearch), key.Matches(press, m.keys.Search):
		return m, m.setFocus(focusSearch)
	case key.Matches(press, m.keys.Up):
		m.move(-1)
	case key.Matches(press, m.keys.Down):
		m.move(1)
	case key.Matches(press, m.keys.Edit):
		return m.openEdit()
	case key.Matches(press, m.keys.Add):
		return m.openAdd()
	case key.Matches(press, m.keys.Remove):
		m.startRemove()
	case key.Matches(press, m.keys.Toggle):
		return m, m.toggleSelected()
	case key.Matches(press, m.keys.Refresh):
		return m, m.load(false)
	}
	return m, nil
}

func (m *Model) setFocus(focus int) tea.Cmd {
	if m.focus == focus {
		return nil
	}
	m.focus = focus
	if focus == focusSearch {
		m.input.SetValue(m.query)
		m.input.CursorEnd()
		return m.input.Focus()
	}
	m.syncInputToSelection()
	m.input.Blur()
	return nil
}

// syncInputToSelection shows the highlighted command in the search field while
// the list owns the focus. The real filter stays in m.query, so a background
// refresh cannot narrow the list to the selected command.
func (m *Model) syncInputToSelection() {
	if m.focus != focusList {
		return
	}
	m.input.SetValue(m.selectedName())
	m.input.CursorEnd()
}

func (m *Model) handleLoaded(msg loadedMsg) (screen.Screen, tea.Cmd) {
	if msg.epoch != m.loadEpoch {
		return m, nil
	}
	m.inflight = false
	m.loading = false

	selectedName := m.selectedName()
	if msg.err != nil {
		if len(m.commands) == 0 {
			m.loadErr = msg.err
		} else {
			m.statusErr = true
			m.status = "could not refresh commands: " + msg.err.Error()
		}
		return m, nil
	}

	m.loadErr = nil
	m.commands = append([]Command(nil), msg.snapshot.Commands...)
	m.editable = msg.snapshot.Editable
	sortCommands(m.commands)
	m.filtered = filterCommands(m.commands, m.query)
	m.loaded = true
	m.restoreSelection(selectedName)
	m.clampOffset()
	if m.focus == focusList {
		m.syncInputToSelection()
	}

	if m.confirming && !m.hasCommand(m.confirmName) {
		m.clearConfirmation()
		m.status = ""
		m.statusErr = false
	}
	return m, nil
}

func (m *Model) handleRemoved(msg removedMsg) (screen.Screen, tea.Cmd) {
	if msg.err != nil {
		m.statusErr = true
		m.status = "could not remove command: " + msg.err.Error()
		return m, nil
	}
	m.statusErr = false
	m.status = "command removed."
	m.removeCommand(msg.name)
	return m, m.load(false)
}

func (m *Model) handleToggled(msg toggledMsg) (screen.Screen, tea.Cmd) {
	if msg.err != nil {
		action := "enable"
		if msg.disabled {
			action = "disable"
		}
		m.statusErr = true
		m.status = "could not " + action + " command: " + msg.err.Error()
		return m, nil
	}
	state := "enabled"
	if msg.disabled {
		state = "disabled"
	}
	m.statusErr = false
	m.status = fmt.Sprintf("command %q %s.", msg.name, state)
	m.setCommandDisabled(msg.name, msg.disabled)
	return m, m.load(false)
}

func (m *Model) openAdd() (screen.Screen, tea.Cmd) {
	if !m.mutationsAvailable() {
		return m, nil
	}
	if m.addOpener == nil {
		m.statusErr = true
		m.status = "adding commands is not available."
		return m, nil
	}
	return m, screen.Push(m.addOpener(m.commandsCopy()))
}

func (m *Model) openEdit() (screen.Screen, tea.Cmd) {
	if !m.mutationsAvailable() {
		return m, nil
	}
	command, ok := m.selectedCommand()
	if !ok {
		return m, nil
	}
	if m.editOpener == nil {
		m.statusErr = true
		m.status = "editing commands is not available."
		return m, nil
	}
	return m, screen.Push(m.editOpener(m.commandsCopy(), command))
}

func (m *Model) startRemove() {
	if !m.mutationsAvailable() {
		return
	}
	command, ok := m.selectedCommand()
	if !ok {
		return
	}
	m.confirming = true
	m.confirmName = command.Name
	m.status = ""
	m.statusErr = false
}

func (m *Model) removeSelected() tea.Cmd {
	name := m.confirmName
	m.clearConfirmation()
	m.status = ""
	m.statusErr = false

	controller := m.controller
	if controller == nil {
		return nil
	}
	return func() tea.Msg {
		return removedMsg{name: name, err: controller.Remove(name)}
	}
}

func (m *Model) toggleSelected() tea.Cmd {
	if !m.mutationsAvailable() {
		return nil
	}
	command, ok := m.selectedCommand()
	if !ok {
		return nil
	}
	controller := m.controller
	if controller == nil {
		return nil
	}
	disabled := !command.Disabled
	return func() tea.Msg {
		return toggledMsg{name: command.Name, disabled: disabled, err: controller.SetDisabled(command.Name, disabled)}
	}
}

func (m *Model) clearConfirmation() {
	m.confirming = false
	m.confirmName = ""
}

func (m *Model) load(showLoading bool) tea.Cmd {
	if m.inflight {
		return nil
	}
	m.inflight = true
	m.loadEpoch++
	epoch := m.loadEpoch
	if showLoading {
		m.loading = true
		m.loadErr = nil
	}

	source := m.source
	if source == nil {
		return func() tea.Msg {
			return loadedMsg{err: fmt.Errorf("no command source configured"), epoch: epoch}
		}
	}
	return func() tea.Msg {
		snapshot, err := source.List()
		return loadedMsg{snapshot: snapshot, err: err, epoch: epoch}
	}
}

// startPolling invalidates any tick from a previous visit and starts one new
// polling chain.
func (m *Model) startPolling() tea.Cmd {
	m.epoch++
	return m.tick()
}

func (m *Model) tick() tea.Cmd {
	epoch := m.epoch
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg {
		return tickMsg{epoch: epoch}
	})
}

func (m *Model) move(delta int) {
	if len(m.filtered) == 0 {
		return
	}
	m.selected = (m.selected + delta + len(m.filtered)) % len(m.filtered)
	m.clampOffset()
	if m.focus == focusList {
		m.syncInputToSelection()
	}
}

func (m *Model) selectedCommand() (Command, bool) {
	if m.selected < 0 || m.selected >= len(m.filtered) {
		return Command{}, false
	}
	return m.filtered[m.selected], true
}

func (m *Model) selectedName() string {
	command, ok := m.selectedCommand()
	if !ok {
		return ""
	}
	return command.Name
}

func (m *Model) commandByName(name string) (Command, bool) {
	for _, command := range m.commands {
		if command.Name == name {
			return command, true
		}
	}
	return Command{}, false
}

func (m *Model) hasCommand(name string) bool {
	_, ok := m.commandByName(name)
	return ok
}

// applyFilter rebuilds the visible rows after the query changes. A new query
// starts from the top.
func (m *Model) applyFilter() {
	m.filtered = filterCommands(m.commands, m.query)
	m.selected = 0
	m.offset = 0
	m.clampOffset()
}

func (m *Model) restoreSelection(name string) {
	if len(m.filtered) == 0 {
		m.selected = 0
		return
	}
	if name != "" {
		for i, command := range m.filtered {
			if command.Name == name {
				m.selected = i
				return
			}
		}
	}
	m.selected = min(max(m.selected, 0), len(m.filtered)-1)
}

func (m *Model) removeCommand(name string) {
	for i, command := range m.commands {
		if command.Name != name {
			continue
		}
		m.commands = append(m.commands[:i], m.commands[i+1:]...)
		m.filtered = filterCommands(m.commands, m.query)
		m.selected = min(m.selected, max(len(m.filtered)-1, 0))
		m.clampOffset()
		if m.focus == focusList {
			m.syncInputToSelection()
		}
		return
	}
}

func (m *Model) setCommandDisabled(name string, disabled bool) {
	for i, command := range m.commands {
		if command.Name != name {
			continue
		}
		m.commands[i].Disabled = disabled
		m.filtered = filterCommands(m.commands, m.query)
		return
	}
}

func (m *Model) commandsCopy() []Command {
	return append([]Command(nil), m.commands...)
}

// commandRows returns the number of two-line rows that fit in the content
// area. The search field, header and status line each take one row.
func (m *Model) commandRows() int {
	return max((m.height-3)/2, 0)
}

func (m *Model) clampOffset() {
	rows := m.commandRows()
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

// mutationsAvailable hides add/edit/remove/toggle until a successful load has
// shown that the commands are editable. The zero value is read-only.
func (m *Model) mutationsAvailable() bool {
	return m.loaded && m.editable && m.controller != nil
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

func plural(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
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
