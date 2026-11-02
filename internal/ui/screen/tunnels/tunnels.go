// Package tunnels implements the Tunnels screen of the sshex terminal UI. It
// lists the active port forwards of one session, or of every session when no
// session is given, and manages them through narrow source and controller
// interfaces.
package tunnels

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/mcnull/sshex/internal/ui/keys"
	"github.com/mcnull/sshex/internal/ui/screen"
	"github.com/mcnull/sshex/internal/ui/theme"
)

const refreshInterval = 5 * time.Second

// Options configures the Tunnels screen. An empty SessionID lists every
// session and hides the add action; Target labels the session-scoped rows.
type Options struct {
	Styles     *theme.Styles
	Source     Source
	Controller Controller
	SessionID  string
	Target     string
}

// Model is the Tunnels screen.
type Model struct {
	styles     *theme.Styles
	source     Source
	controller Controller
	sessionID  string
	target     string
	keys       keys.Tunnels
	form       *addForm

	tunnels  []Tunnel
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

	confirming      bool
	confirmID       string
	confirmSession  string
	confirmFor      string
	selectAfterLoad string

	width  int
	height int
}

// loadedMsg carries the result of an asynchronous tunnel lookup. The epoch
// discards a result requested during an earlier visit.
type loadedMsg struct {
	tunnels []Tunnel
	err     error
	epoch   uint64
}

// tickMsg asks the screen to refresh its list. The epoch ignores ticks that
// were scheduled during an earlier visit and would otherwise start a second
// polling chain.
type tickMsg struct {
	epoch uint64
}

// addedMsg reports the result of adding a tunnel.
type addedMsg struct {
	tunnel Tunnel
	err    error
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
	m.form = nil
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

// Keys implements screen.Screen.
func (m *Model) Keys() []key.Binding {
	if m.form != nil {
		return []key.Binding{m.keys.Submit, m.keys.Direction, keys.Cancel()}
	}
	if m.confirming {
		return []key.Binding{keys.Confirm(), keys.Cancel()}
	}

	screenKeys := []key.Binding{m.keys.Up, m.keys.Down}
	if m.sessionID != "" {
		screenKeys = append(screenKeys, m.keys.Add)
	}
	return append(screenKeys, m.keys.Remove, m.keys.Refresh, keys.Back())
}

// Resize implements screen.Screen.
func (m *Model) Resize(width, height int) {
	m.width = width
	m.height = height
	if m.form != nil {
		m.form.SetWidth(width)
	}
	m.clampOffset()
}

// Update implements screen.Screen.
func (m *Model) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case loadedMsg:
		return m.handleLoaded(msg)
	case addedMsg:
		return m.handleAdded(msg)
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
	if m.form != nil {
		return m.handleFormKey(press)
	}
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

	switch {
	case key.Matches(press, m.keys.Up):
		m.move(-1)
	case key.Matches(press, m.keys.Down):
		m.move(1)
	case key.Matches(press, m.keys.Add):
		if m.sessionID != "" {
			m.openForm()
		}
	case key.Matches(press, m.keys.Remove):
		m.startRemove()
	case key.Matches(press, m.keys.Refresh):
		return m, m.load(false)
	}
	return m, nil
}

func (m *Model) handleFormKey(press tea.KeyPressMsg) (screen.Screen, tea.Cmd) {
	switch {
	case key.Matches(press, keys.Cancel()):
		m.form = nil
		return m, nil
	case key.Matches(press, m.keys.Submit):
		return m, m.submitForm()
	case key.Matches(press, m.keys.Direction):
		m.form.ToggleDirection()
		return m, nil
	}
	return m, m.form.update(press)
}

func (m *Model) openForm() {
	if m.controller == nil {
		m.statusErr = true
		m.status = "adding tunnels is not available."
		return
	}
	m.form = newAddForm(m.styles)
	m.form.SetWidth(m.width)
	m.status = ""
	m.statusErr = false
}

func (m *Model) submitForm() tea.Cmd {
	if m.form == nil {
		return nil
	}
	req, err := m.form.Request()
	if err != nil {
		return nil
	}
	controller := m.controller
	sessionID := m.sessionID
	if controller == nil {
		m.form.err = "adding tunnels is not available."
		return nil
	}
	return func() tea.Msg {
		tunnel, err := controller.Add(sessionID, req)
		return addedMsg{tunnel: tunnel, err: err}
	}
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
	if m.selectAfterLoad != "" {
		selectedID = m.selectAfterLoad
		m.selectAfterLoad = ""
	}
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
	m.tunnels = append([]Tunnel(nil), msg.tunnels...)
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

func (m *Model) handleAdded(msg addedMsg) (screen.Screen, tea.Cmd) {
	if msg.err != nil {
		if m.form != nil {
			m.form.err = "could not add tunnel: " + msg.err.Error()
		}
		return m, nil
	}
	m.form = nil
	m.statusErr = false
	m.status = "tunnel created."
	m.selectAfterLoad = msg.tunnel.ID
	return m, m.load(false)
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
		list, err := source.List(sessionID)
		return loadedMsg{tunnels: list, err: err, epoch: epoch}
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
// area. The list header and status line each take one row.
func (m *Model) tunnelRows() int {
	return max((m.height-2)/2, 0)
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
