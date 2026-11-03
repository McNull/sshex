// Package tunnels implements the Tunnels screen of the sshex terminal UI. It
// lists the active port forwards of one session, or of every session when no
// session is given, and manages them through narrow source and controller
// interfaces.
package tunnels

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/mcnull/sshex/internal/ui/keys"
	"github.com/mcnull/sshex/internal/ui/screen"
	"github.com/mcnull/sshex/internal/ui/theme"
)

const refreshInterval = 5 * time.Second

// Options configures the Tunnels screen. An empty SessionID lists every
// session; Target labels the session-scoped rows. AddOpener supplies the screen
// pushed by `a`, so the Tunnels screen never reaches the broker or Sessions.
type Options struct {
	Styles     *theme.Styles
	Source     Source
	Controller Controller
	SessionID  string
	Target     string
	AddOpener  func() screen.Screen
}

// Model is the Tunnels screen.
type Model struct {
	styles     *theme.Styles
	source     Source
	controller Controller
	sessionID  string
	target     string
	addOpener  func() screen.Screen
	keys       keys.Tunnels

	tunnels  []Tunnel
	sessions int
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

	confirming     bool
	confirmID      string
	confirmSession string
	confirmFor     string

	width  int
	height int
}

// loadedMsg carries the result of an asynchronous tunnel lookup. The epoch
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

// removedMsg reports the result of removing a tunnel.
type removedMsg struct {
	id  string
	err error
}

// New returns a Tunnels screen.
func New(opts Options) *Model {
	return &Model{
		styles:     opts.Styles,
		source:     opts.Source,
		controller: opts.Controller,
		sessionID:  opts.SessionID,
		target:     opts.Target,
		addOpener:  opts.AddOpener,
		keys:       keys.DefaultTunnels(),
		loading:    true,
	}
}

// Init implements screen.Screen. It reloads the list on every visit so the
// screen reflects tunnels that were added or removed elsewhere.
func (m *Model) Init() tea.Cmd {
	// A request from a previous visit may have completed while another screen
	// was active, so its result never reached this model.
	m.inflight = false
	m.status = ""
	m.statusErr = false
	m.clearConfirmation()
	return tea.Batch(m.load(!m.loaded), m.startPolling())
}

// Resume implements screen.Resumable. It refreshes the list and restarts the
// poll loop when the screen becomes active again.
func (m *Model) Resume() tea.Cmd {
	m.inflight = false
	return tea.Batch(m.load(false), m.startPolling())
}

// Title implements screen.Screen.
func (m *Model) Title() string { return "Tunnels" }

// HelpText implements screen.HelpProvider.
func (m *Model) HelpText() string { return "Manage tunnels between the origin and clients." }

// Keys implements screen.Screen.
func (m *Model) Keys() []keys.Binding {
	if m.confirming {
		return []keys.Binding{keys.Confirm(), keys.Cancel()}
	}

	screenKeys := []keys.Binding{m.keys.Up, m.keys.Down}
	if m.mutationsAvailable() {
		if m.addOpener != nil {
			screenKeys = append(screenKeys, m.keys.Add)
		}
		screenKeys = append(screenKeys, m.keys.Remove)
	}
	return append(screenKeys, m.keys.Refresh, keys.Back())
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
	case removedMsg:
		return m.handleRemoved(msg)
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
		case keys.Matches(press, keys.Confirm()):
			return m, m.removeSelected()
		case keys.Matches(press, keys.Cancel()):
			m.clearConfirmation()
			m.status = ""
			m.statusErr = false
		}
		return m, nil
	}

	if keys.Matches(press, keys.Back()) {
		return m, screen.Pop()
	}

	switch {
	case keys.Matches(press, m.keys.Up):
		m.move(-1)
	case keys.Matches(press, m.keys.Down):
		m.move(1)
	case keys.Matches(press, m.keys.Add):
		if m.mutationsAvailable() {
			return m, m.openAdd()
		}
	case keys.Matches(press, m.keys.Remove):
		if m.mutationsAvailable() {
			m.startRemove()
		}
	case keys.Matches(press, m.keys.Refresh):
		return m, m.load(false)
	}
	return m, nil
}

// openAdd pushes the screen supplied by the caller. Session-scoped views open
// the add form directly; the all-sessions view opens a session picker.
func (m *Model) openAdd() tea.Cmd {
	if m.addOpener == nil {
		return nil
	}
	return screen.Push(m.addOpener())
}

func (m *Model) startRemove() {
	tunnel, ok := m.selectedTunnel()
	if !ok {
		return
	}
	if m.controller == nil {
		m.statusErr = true
		m.status = "removing tunnels is not available."
		return
	}
	m.confirming = true
	m.confirmID = tunnel.ID
	m.confirmSession = tunnel.SessionID
	m.confirmFor = tunnel.ID
	m.status = ""
	m.statusErr = false
}

func (m *Model) removeSelected() tea.Cmd {
	id := m.confirmID
	sessionID := m.confirmSession
	m.clearConfirmation()
	m.status = ""
	m.statusErr = false

	controller := m.controller
	if controller == nil {
		return nil
	}
	return func() tea.Msg {
		return removedMsg{id: id, err: controller.Remove(sessionID, id)}
	}
}

func (m *Model) clearConfirmation() {
	m.confirming = false
	m.confirmID = ""
	m.confirmSession = ""
	m.confirmFor = ""
}

func (m *Model) handleLoaded(msg loadedMsg) (screen.Screen, tea.Cmd) {
	if msg.epoch != m.loadEpoch {
		return m, nil
	}
	m.inflight = false
	m.loading = false

	selectedID := m.selectedID()
	if msg.err != nil {
		if len(m.tunnels) == 0 {
			m.loadErr = msg.err
		} else {
			m.statusErr = true
			m.status = "could not refresh tunnels: " + msg.err.Error()
		}
		return m, nil
	}

	m.loadErr = nil
	m.tunnels = append([]Tunnel(nil), msg.snapshot.Tunnels...)
	m.sessions = msg.snapshot.Sessions
	sortTunnels(m.tunnels)
	m.loaded = true
	m.restoreSelection(selectedID)
	m.clampOffset()

	if m.confirming && !m.hasTunnel(m.confirmID) {
		m.clearConfirmation()
		m.status = ""
		m.statusErr = false
	}
	return m, nil
}

func (m *Model) handleRemoved(msg removedMsg) (screen.Screen, tea.Cmd) {
	if msg.err != nil {
		m.statusErr = true
		m.status = "could not remove tunnel: " + msg.err.Error()
		return m, nil
	}
	m.statusErr = false
	m.status = "tunnel removed."
	m.removeTunnel(msg.id)
	return m, m.load(false)
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
			return loadedMsg{err: fmt.Errorf("no tunnel source configured"), epoch: epoch}
		}
	}
	sessionID := m.sessionID
	return func() tea.Msg {
		snapshot, err := source.List(sessionID)
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
	if len(m.tunnels) == 0 {
		return
	}
	m.selected = (m.selected + delta + len(m.tunnels)) % len(m.tunnels)
	m.clampOffset()
}

func (m *Model) selectedTunnel() (Tunnel, bool) {
	if m.selected < 0 || m.selected >= len(m.tunnels) {
		return Tunnel{}, false
	}
	return m.tunnels[m.selected], true
}

func (m *Model) selectedID() string {
	tunnel, ok := m.selectedTunnel()
	if !ok {
		return ""
	}
	return tunnel.ID
}

func (m *Model) hasTunnel(id string) bool {
	for _, tunnel := range m.tunnels {
		if tunnel.ID == id {
			return true
		}
	}
	return false
}

func (m *Model) restoreSelection(id string) {
	if len(m.tunnels) == 0 {
		m.selected = 0
		return
	}
	if id != "" {
		for i, tunnel := range m.tunnels {
			if tunnel.ID == id {
				m.selected = i
				return
			}
		}
	}
	m.selected = min(max(m.selected, 0), len(m.tunnels)-1)
}

func (m *Model) removeTunnel(id string) {
	for i, tunnel := range m.tunnels {
		if tunnel.ID != id {
			continue
		}
		m.tunnels = append(m.tunnels[:i], m.tunnels[i+1:]...)
		m.selected = min(m.selected, max(len(m.tunnels)-1, 0))
		m.clampOffset()
		return
	}
}

// tunnelRows returns the number of two-line rows that fit in the content
// area. The blank line above the list header, the header itself and the
// status line each take one row.
func (m *Model) tunnelRows() int {
	return max((m.height-3)/2, 0)
}

func (m *Model) clampOffset() {
	rows := m.tunnelRows()
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
	maxOffset := max(len(m.tunnels)-rows, 0)
	if m.offset > maxOffset {
		m.offset = maxOffset
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

// noSessions reports a successfully loaded empty session set. Mutation
// actions are hidden in that state because a tunnel needs a session.
func (m *Model) noSessions() bool {
	return m.loaded && !m.loading && m.loadErr == nil && m.sessions == 0
}

// mutationsAvailable hides add/remove until the all-sessions view knows that a
// session exists. The session-scoped view already knows its session.
func (m *Model) mutationsAvailable() bool {
	if m.sessionID == "" && !m.loaded {
		return false
	}
	return !m.noSessions()
}

// targetFor returns the session label shown on the detail line.
func (m *Model) targetFor(tunnel Tunnel) string {
	if tunnel.Target != "" {
		return tunnel.Target
	}
	if m.target != "" {
		return m.target
	}
	if tunnel.SessionID != "" {
		return tunnel.SessionID
	}
	return m.sessionID
}
