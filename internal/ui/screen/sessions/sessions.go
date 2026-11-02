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
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mcnull/sshex/internal/ui/keys"
	"github.com/mcnull/sshex/internal/ui/screen"
	"github.com/mcnull/sshex/internal/ui/screen/tunnels"
	"github.com/mcnull/sshex/internal/ui/theme"
)

const refreshInterval = 5 * time.Second

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

// Options configures the Sessions screen.
type Options struct {
	Styles *theme.Styles
	Source Source
	Closer Closer
}

// Model is the Sessions screen.
type Model struct {
	styles *theme.Styles
	source Source
	closer Closer
	keys   keys.Sessions

	sessions []Session
	selected int
	offset   int

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
	return &Model{
		styles:  opts.Styles,
		source:  opts.Source,
		closer:  opts.Closer,
		keys:    keys.DefaultSessions(),
		loading: true,
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
	return tea.Batch(m.load(!m.loaded), m.startPolling())
}

// Resume implements screen.Resumable. It refreshes the list and restarts the
// poll loop after the tunnel screen is popped.
func (m *Model) Resume() tea.Cmd {
	m.inflight = false
	return tea.Batch(m.load(false), m.startPolling())
}

// Title implements screen.Screen.
func (m *Model) Title() string { return "Sessions" }

// Keys implements screen.Screen.
func (m *Model) Keys() []key.Binding {
	if m.confirming {
		return []key.Binding{keys.Confirm(), keys.Cancel()}
	}
	return []key.Binding{m.keys.Up, m.keys.Down, m.keys.Open, m.keys.Close, m.keys.Refresh, keys.Back()}
}

// Resize implements screen.Screen.
func (m *Model) Resize(width, height int) {
	m.width = width
	m.height = height
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

	switch {
	case key.Matches(press, m.keys.Up):
		m.move(-1)
	case key.Matches(press, m.keys.Down):
		m.move(1)
	case key.Matches(press, m.keys.Open):
		return m.openTunnels()
	case key.Matches(press, m.keys.Close):
		m.startClose()
	case key.Matches(press, m.keys.Refresh):
		return m, m.load(false)
	}
	return m, nil
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
	m.loaded = true
	m.restoreSelection(selectedID)
	m.clampOffset()

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

func (m *Model) openTunnels() (screen.Screen, tea.Cmd) {
	session, ok := m.selectedSession()
	if !ok {
		return m, nil
	}
	return m, screen.Push(tunnels.New(tunnels.Options{
		Styles:    m.styles,
		SessionID: session.ID,
		Target:    session.Target,
	}))
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
	if len(m.sessions) == 0 {
		return
	}
	m.selected = (m.selected + delta + len(m.sessions)) % len(m.sessions)
	m.clampOffset()
}

func (m *Model) selectedSession() (Session, bool) {
	if m.selected < 0 || m.selected >= len(m.sessions) {
		return Session{}, false
	}
	return m.sessions[m.selected], true
}

func (m *Model) selectedID() string {
	session, ok := m.selectedSession()
	if !ok {
		return ""
	}
	return session.ID
}

func (m *Model) hasSession(id string) bool {
	for _, session := range m.sessions {
		if session.ID == id {
			return true
		}
	}
	return false
}

// sortSessions orders rows by hostname, then by the full target and ID so a
// broker that returns a different order cannot make the list jump around.
func sortSessions(sessions []Session) {
	sort.SliceStable(sessions, func(i, j int) bool {
		left, right := strings.ToLower(sessions[i].Host), strings.ToLower(sessions[j].Host)
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
	if len(m.sessions) == 0 {
		m.selected = 0
		return
	}
	if id != "" {
		for i, session := range m.sessions {
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
	m.selected = min(max(m.selected, 0), len(m.sessions)-1)
}

func (m *Model) currentIndex() int {
	for i, session := range m.sessions {
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
		m.selected = min(m.selected, max(len(m.sessions)-1, 0))
		m.clampOffset()
		return
	}
}

// sessionRows returns the number of sessions that fit in the content area.
func (m *Model) sessionRows() int {
	return max((m.height-2)/2, 0)
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
	maxOffset := max(len(m.sessions)-rows, 0)
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
	}

	lines := make([]string, 0, rows*2)
	for i := 0; i < rows; i++ {
		index := m.offset + i
		if index >= len(m.sessions) {
			break
		}
		first, second := m.row(m.sessions[index], index == m.selected)
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
	message := fmt.Sprintf("%d %s", len(m.sessions), plural(len(m.sessions), "session", "sessions"))
	return m.styles.ScreenHint.Render(truncate(message, m.width))
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
