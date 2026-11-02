// Package sessions implements the Sessions screen of the sshex terminal UI. It
// lists the sessions of a live broker and opens the session-scoped tunnel
// screen for the selected session.
package sessions

import (
	"fmt"
	"sort"
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

// Session is one row of the session list. It is deliberately independent of
// the broker DTO so the screen can be tested without a daemon.
type Session struct {
	ID          string
	Host        string
	Target      string
	Origin      string
	State       string
	Connections int
	Hops        string
	Stale       bool
	// Current marks the session of the remote shell that started the UI.
	Current bool
}

// Source lists the active sessions.
type Source interface {
	List() ([]Session, error)
}

// Closer closes a session and drops its connections and tunnels.
type Closer interface {
	Close(id string) error
}

// TunnelOpener returns the tunnel screen for a session. The Sessions screen
// never reaches the broker itself; the caller supplies the opener.
type TunnelOpener func(sessionID, target string) screen.Screen

// Options configures the Sessions screen. OnSelect turns the screen into a
// picker: enter chooses the session instead of opening its tunnels, closing is
// disabled, and the callback owns the navigation.
type Options struct {
	Styles       *theme.Styles
	Source       Source
	Closer       Closer
	TunnelOpener TunnelOpener
	OnSelect     func(Session) tea.Cmd
}

// Model is the Sessions screen.
type Model struct {
	styles       *theme.Styles
	source       Source
	closer       Closer
	tunnelOpener TunnelOpener
	onSelect     func(Session) tea.Cmd
	keys         keys.Sessions
	input        textinput.Model

	focus    int
	sessions []Session
	filtered []Session
	selected int
	offset   int
	query    string

	loaded    bool
	loading   bool
	inflight  bool
	loadErr   error
	epoch     uint64
	loadEpoch uint64

	status    string
	statusErr bool

	confirming bool
	confirmID  string
	confirmFor string

	width  int
	height int
}

// loadedMsg carries the result of an asynchronous session lookup. The epoch
// discards a result requested during an earlier visit.
type loadedMsg struct {
	sessions []Session
	err      error
	epoch    uint64
}

// tickMsg asks the screen to refresh its list. The epoch ignores ticks that
// were scheduled during an earlier visit and would otherwise start a second
// polling chain.
type tickMsg struct {
	epoch uint64
}

// closedMsg reports the result of closing a session.
type closedMsg struct {
	id  string
	err error
}

// New returns a Sessions screen.
func New(opts Options) *Model {
	input := textinput.New()
	input.Prompt = "search › "
	input.Placeholder = "host name or part of it"
	input.SetStyles(inputStyles(opts.Styles))
	input.Focus()

	return &Model{
		styles:       opts.Styles,
		source:       opts.Source,
		closer:       opts.Closer,
		tunnelOpener: opts.TunnelOpener,
		onSelect:     opts.OnSelect,
		keys:         keys.DefaultSessions(),
		input:        input,
		focus:        focusSearch,
		loading:      true,
	}
}

// Init implements screen.Screen. It reloads the list on every visit so the
// screen reflects sessions that were opened or closed elsewhere.
func (m *Model) Init() tea.Cmd {
	// A request from a previous visit may have completed while another screen
	// was active, so its result never reached this model.
	m.inflight = false
	m.status = ""
	m.statusErr = false
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
func (m *Model) Title() string {
	if m.picker() {
		return "Select session"
	}
	return "Sessions"
}

func (m *Model) picker() bool { return m.onSelect != nil }

func (m *Model) openBinding() key.Binding {
	if m.picker() {
		return m.keys.Select
	}
	return m.keys.Open
}

// Keys implements screen.Screen.
func (m *Model) Keys() []key.Binding {
	if m.confirming {
		return []key.Binding{keys.Confirm(), keys.Cancel()}
	}
	if m.focus == focusList {
		screenKeys := []key.Binding{m.keys.Up, m.keys.Down, m.openBinding()}
		if !m.picker() {
			screenKeys = append(screenKeys, m.keys.Close)
		}
		return append(screenKeys, m.keys.Refresh, m.keys.FocusSearch, m.keys.Backspace, keys.Back())
	}
	return []key.Binding{m.keys.SearchUp, m.keys.SearchDown, m.openBinding(), m.keys.FocusList, keys.Back()}
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
	case closedMsg:
		return m.handleClosed(msg)
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
			return m, m.closeSelected()
		case key.Matches(press, keys.Cancel()):
			m.confirming = false
			m.confirmID = ""
			m.confirmFor = ""
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
		case key.Matches(press, m.openBinding()):
			return m.openSelected()
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
	case key.Matches(press, m.keys.FocusSearch):
		return m, m.setFocus(focusSearch)
	case key.Matches(press, m.keys.Backspace):
		return m, m.backspaceSearch()
	case key.Matches(press, m.keys.Up):
		m.move(-1)
	case key.Matches(press, m.keys.Down):
		m.move(1)
	case key.Matches(press, m.openBinding()):
		return m.openSelected()
	case key.Matches(press, m.keys.Close):
		if !m.picker() {
			m.startClose()
		}
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

// syncInputToSelection shows the highlighted host in the search field while
// the list owns the focus. The real filter stays in m.query, so a background
// refresh cannot narrow the list to the selected host.
func (m *Model) syncInputToSelection() {
	if m.focus != focusList {
		return
	}
	m.input.SetValue(m.selectedHost())
	m.input.CursorEnd()
}

// backspaceSearch returns to the search field and removes the last character
// from the query.
func (m *Model) backspaceSearch() tea.Cmd {
	focusCmd := m.setFocus(focusSearch)
	var inputCmd tea.Cmd
	m.input, inputCmd = m.input.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m.query = m.input.Value()
	m.applyFilter()
	return tea.Batch(focusCmd, inputCmd)
}

func (m *Model) handleLoaded(msg loadedMsg) (screen.Screen, tea.Cmd) {
	if msg.epoch != m.loadEpoch {
		return m, nil
	}
	m.inflight = false
	m.loading = false

	selectedID := m.selectedID()
	if msg.err != nil {
		if len(m.sessions) == 0 {
			m.loadErr = msg.err
		} else {
			m.statusErr = true
			m.status = "could not refresh sessions: " + msg.err.Error()
		}
		return m, nil
	}

	m.loadErr = nil
	m.sessions = append([]Session(nil), msg.sessions...)
	sortSessions(m.sessions)
	m.filtered = filterSessions(m.sessions, m.query)
	m.loaded = true
	m.restoreSelection(selectedID)
	m.clampOffset()
	if m.focus == focusList {
		m.syncInputToSelection()
	}

	if m.confirming && !m.hasSession(m.confirmID) {
		m.confirming = false
		m.confirmID = ""
		m.confirmFor = ""
		m.status = ""
		m.statusErr = false
	}
	return m, nil
}

func (m *Model) handleClosed(msg closedMsg) (screen.Screen, tea.Cmd) {
	if msg.err != nil {
		m.statusErr = true
		m.status = "could not close session: " + msg.err.Error()
		return m, nil
	}
	m.statusErr = false
	m.status = "session closed."
	m.removeSession(msg.id)
	return m, m.load(false)
}

func (m *Model) openSelected() (screen.Screen, tea.Cmd) {
	session, ok := m.selectedSession()
	if !ok {
		return m, nil
	}
	if m.onSelect != nil {
		return m, m.onSelect(session)
	}
	if m.tunnelOpener == nil {
		m.statusErr = true
		m.status = "tunnels are not available."
		return m, nil
	}
	return m, screen.Push(m.tunnelOpener(session.ID, session.Target))
}

func (m *Model) startClose() {
	session, ok := m.selectedSession()
	if !ok {
		return
	}
	if m.closer == nil {
		m.statusErr = true
		m.status = "closing sessions is not available."
		return
	}
	m.confirming = true
	m.confirmID = session.ID
	m.confirmFor = session.Target
	m.status = ""
	m.statusErr = false
}

func (m *Model) closeSelected() tea.Cmd {
	id := m.confirmID
	m.confirming = false
	m.confirmID = ""
	m.confirmFor = ""
	m.status = ""
	m.statusErr = false

	closer := m.closer
	if closer == nil {
		return nil
	}
	return func() tea.Msg {
		return closedMsg{id: id, err: closer.Close(id)}
	}
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
			return loadedMsg{err: fmt.Errorf("no session source configured"), epoch: epoch}
		}
	}
	return func() tea.Msg {
		list, err := source.List()
		return loadedMsg{sessions: list, err: err, epoch: epoch}
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

func (m *Model) selectedSession() (Session, bool) {
	if m.selected < 0 || m.selected >= len(m.filtered) {
		return Session{}, false
	}
	return m.filtered[m.selected], true
}

func (m *Model) selectedID() string {
	session, ok := m.selectedSession()
	if !ok {
		return ""
	}
	return session.ID
}

func (m *Model) selectedHost() string {
	session, ok := m.selectedSession()
	if !ok {
		return ""
	}
	return sessionHost(session)
}

func (m *Model) hasSession(id string) bool {
	for _, session := range m.sessions {
		if session.ID == id {
			return true
		}
	}
	return false
}

// applyFilter rebuilds the visible rows after the query changes. A new query
// starts from the top, except that an empty query returns to the current
// session when there is one.
func (m *Model) applyFilter() {
	m.filtered = filterSessions(m.sessions, m.query)
	m.selected = 0
	m.offset = 0
	if strings.TrimSpace(m.query) == "" {
		if index := m.currentIndex(); index >= 0 {
			m.selected = index
		}
	}
	m.clampOffset()
}

func filterSessions(sessions []Session, query string) []Session {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return append([]Session(nil), sessions...)
	}
	filtered := make([]Session, 0, len(sessions))
	for _, session := range sessions {
		if strings.Contains(strings.ToLower(sessionHost(session)), query) {
			filtered = append(filtered, session)
		}
	}
	return filtered
}

func sessionHost(session Session) string {
	if session.Host != "" {
		return session.Host
	}
	return session.Target
}

// sortSessions orders rows by hostname, then by the full target and ID so a
// broker that returns a different order cannot make the list jump around.
func sortSessions(sessions []Session) {
	sort.SliceStable(sessions, func(i, j int) bool {
		left, right := strings.ToLower(sessionHost(sessions[i])), strings.ToLower(sessionHost(sessions[j]))
		if left != right {
			return left < right
		}
		left, right = strings.ToLower(sessions[i].Target), strings.ToLower(sessions[j].Target)
		if left != right {
			return left < right
		}
		return sessions[i].ID < sessions[j].ID
	})
}

func (m *Model) restoreSelection(id string) {
	if len(m.filtered) == 0 {
		m.selected = 0
		return
	}
	if id != "" {
		for i, session := range m.filtered {
			if session.ID == id {
				m.selected = i
				return
			}
		}
	}
	// Without a previous selection, or when it disappeared, prefer the
	// session of the remote shell that started the UI.
	if index := m.currentIndex(); index >= 0 {
		m.selected = index
		return
	}
	m.selected = min(max(m.selected, 0), len(m.filtered)-1)
}

func (m *Model) currentIndex() int {
	for i, session := range m.filtered {
		if session.Current {
			return i
		}
	}
	return -1
}

func (m *Model) removeSession(id string) {
	for i, session := range m.sessions {
		if session.ID != id {
			continue
		}
		m.sessions = append(m.sessions[:i], m.sessions[i+1:]...)
		m.filtered = filterSessions(m.sessions, m.query)
		m.selected = min(m.selected, max(len(m.filtered)-1, 0))
		m.clampOffset()
		if m.focus == focusList {
			m.syncInputToSelection()
		}
		return
	}
}

// sessionRows returns the number of sessions that fit in the content area.
// The search field, header and status line each take one row.
func (m *Model) sessionRows() int {
	return max((m.height-3)/2, 0)
}

func (m *Model) clampOffset() {
	rows := m.sessionRows()
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
	lines = append(lines, m.input.View(), m.headerView())
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

func (m *Model) headerView() string {
	inner := max(m.width-2, 0)
	left := "  SESSION"
	right := "STATE"
	gap := max(inner-lipgloss.Width(left)-lipgloss.Width(right), 1)
	line := left + strings.Repeat(" ", gap) + right
	return m.styles.ListHeader.Padding(0, 1).Width(m.width).Render(padRight(line, inner))
}

func (m *Model) listView() []string {
	rows := m.sessionRows()
	if rows <= 0 {
		return nil
	}

	switch {
	case m.loading:
		return []string{m.styles.ScreenHint.Render("loading sessions…")}
	case m.loadErr != nil:
		message := truncate("could not load sessions: "+m.loadErr.Error(), m.width)
		return []string{m.styles.Error.Render(message)}
	case len(m.sessions) == 0:
		return []string{m.styles.ScreenHint.Render("no active sessions")}
	case len(m.filtered) == 0:
		return []string{m.styles.ScreenHint.Render("no matching sessions")}
	}

	lines := make([]string, 0, rows*2)
	for i := 0; i < rows; i++ {
		index := m.offset + i
		if index >= len(m.filtered) {
			break
		}
		first, second := m.row(m.filtered[index], index == m.selected)
		lines = append(lines, first, second)
	}
	return lines
}

func (m *Model) row(session Session, selected bool) (string, string) {
	inner := max(m.width-2, 0)
	marker := " "
	if selected {
		marker = "›"
	}

	state := session.State
	if state == "" {
		state = "unknown"
	}
	targetWidth := max(inner-lipgloss.Width(state)-3, 1)
	target := truncate(session.Target, targetWidth)
	left := marker + " " + target
	gap := max(inner-lipgloss.Width(left)-lipgloss.Width(state), 1)
	first := left + strings.Repeat(" ", gap) + state

	second := m.detail(session, selected, inner)

	if selected {
		style := m.styles.ListRowSelected.Padding(0, 1).Width(m.width)
		return style.Render(padRight(first, inner)), style.Render(padRight(second, inner))
	}

	stateStyle := m.stateStyle(session.State)
	first = m.styles.ListMarker.Render(marker) + " " + m.styles.ListRow.Render(target) +
		strings.Repeat(" ", gap) + stateStyle.Render(state)

	rowStyle := m.styles.ListRow.Padding(0, 1).Width(m.width)
	return rowStyle.Render(padRight(first, inner)), rowStyle.Render(padRight(second, inner))
}

func (m *Model) detail(session Session, selected bool, inner int) string {
	parts := []string{session.ID}
	if session.Origin != "" {
		parts = append(parts, session.Origin)
	}
	parts = append(parts, fmt.Sprintf("%d %s", session.Connections, plural(session.Connections, "connection", "connections")))
	hops := session.Hops
	if hops == "" {
		hops = "0"
	}
	parts = append(parts, hops+" hops")
	base := "  " + strings.Join(parts, " · ")

	type suffix struct {
		text  string
		style lipgloss.Style
	}
	var suffixes []suffix
	if session.Current {
		suffixes = append(suffixes, suffix{" · current", m.styles.Accent})
	}
	if session.Stale {
		suffixes = append(suffixes, suffix{" · stale", m.styles.Warning})
	}
	if len(suffixes) == 0 {
		if selected {
			return truncate(base, inner)
		}
		return m.styles.ListDescription.Render(truncate(base, inner))
	}

	plain := base
	suffixWidth := 0
	for _, s := range suffixes {
		plain += s.text
		suffixWidth += lipgloss.Width(s.text)
	}
	if selected {
		return truncate(plain, inner)
	}

	out := m.styles.ListDescription.Render(truncate(base, max(inner-suffixWidth, 0)))
	for _, s := range suffixes {
		out += s.style.Render(s.text)
	}
	return out
}

func (m *Model) stateStyle(state string) lipgloss.Style {
	switch state {
	case "active":
		return m.styles.Success
	case "failed":
		return m.styles.Error
	case "pending", "closing":
		return m.styles.Warning
	default:
		return m.styles.Muted
	}
}

func (m *Model) statusView() string {
	if m.confirming {
		message := fmt.Sprintf("close %s? enter confirm · esc cancel — drops connections and tunnels.", m.confirmFor)
		return m.styles.Warning.Render(truncate(message, m.width))
	}
	if m.status != "" {
		if m.statusErr {
			return m.styles.Error.Render(truncate(m.status, m.width))
		}
		return m.styles.ScreenHint.Render(truncate(m.status, m.width))
	}
	if m.loading || m.loadErr != nil || len(m.sessions) == 0 {
		return ""
	}
	var message string
	if strings.TrimSpace(m.query) == "" {
		message = fmt.Sprintf("%d %s", len(m.sessions), plural(len(m.sessions), "session", "sessions"))
	} else {
		message = fmt.Sprintf("%d of %d sessions", len(m.filtered), len(m.sessions))
	}
	if m.picker() {
		message = "select a session for the new tunnel · " + message
	}
	return m.styles.ScreenHint.Render(truncate(message, m.width))
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
