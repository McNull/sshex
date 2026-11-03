// Package update implements the Update screen of the sshex terminal UI. It
// shows the current version and the cached check state, checks the GitHub
// Releases for a newer version on every visit, and installs it through narrow
// source, checker and installer interfaces.
package update

import (
	"errors"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/mcnull/sshex/internal/ui/keys"
	"github.com/mcnull/sshex/internal/ui/screen"
	"github.com/mcnull/sshex/internal/ui/theme"
)

// Snapshot is the cached update state shown when the screen opens. It is
// supplied by the adapter so the screen does not read version.yml itself.
type Snapshot struct {
	CurrentVersion   string
	AvailableVersion string
	CheckedAt        time.Time
	CheckError       string
}

// CheckResult is the outcome of an online version check. The adapter performs
// the comparison so the screen works with plain values in tests.
type CheckResult struct {
	LatestVersion   string
	UpdateAvailable bool
	CheckedAt       time.Time
}

// Result describes the outcome of an install attempt.
type Result struct {
	CurrentVersion string
	LatestVersion  string
	Updated        bool
}

// Source loads the cached version state.
type Source interface {
	Load() (Snapshot, error)
}

// Checker performs an explicit online version check.
type Checker interface {
	Check() (CheckResult, error)
}

// Installer downloads and replaces the running executable.
type Installer interface {
	Install() (Result, error)
}

// Options configures the Update screen.
type Options struct {
	Styles    *theme.Styles
	Source    Source
	Checker   Checker
	Installer Installer
}

type mode int

const (
	modeLoading mode = iota
	modeIdle
	modeChecking
	modePrompt
	modeInstalling
)

// Model is the Update screen.
type Model struct {
	styles    *theme.Styles
	source    Source
	checker   Checker
	installer Installer
	keys      keys.Update
	spinner   spinner.Model

	mode             mode
	currentVersion   string
	availableVersion string
	updateAvailable  bool
	checkedAt        time.Time
	checkErr         string
	loadErr          error

	status    string
	statusErr bool

	epoch  uint64
	width  int
	height int
}

// loadedMsg carries the cached state. The epoch discards a result requested
// during an earlier visit.
type loadedMsg struct {
	snapshot Snapshot
	err      error
	epoch    uint64
}

// checkedMsg reports the result of an online check.
type checkedMsg struct {
	result CheckResult
	err    error
	epoch  uint64
}

// installedMsg reports the result of an install attempt.
type installedMsg struct {
	result Result
	err    error
	epoch  uint64
}

// New returns an Update screen.
func New(opts Options) *Model {
	m := &Model{
		styles:    opts.Styles,
		source:    opts.Source,
		checker:   opts.Checker,
		installer: opts.Installer,
		keys:      keys.DefaultUpdate(),
		mode:      modeLoading,
	}
	m.resetSpinner()
	return m
}

func (m *Model) resetSpinner() {
	m.spinner = spinner.New(
		spinner.WithSpinner(spinner.Line),
		spinner.WithStyle(m.styles.ScreenHint),
	)
}

// Init implements screen.Screen. It reloads the cached state; the online check
// starts as soon as the cached state has been read.
func (m *Model) Init() tea.Cmd {
	m.status = ""
	m.statusErr = false
	return m.load()
}

// Resume implements screen.Resumable. Every visit checks online again.
func (m *Model) Resume() tea.Cmd {
	m.status = ""
	m.statusErr = false
	return m.load()
}

// Title implements screen.Screen.
func (m *Model) Title() string { return "Update" }

// HelpText implements screen.HelpProvider.
func (m *Model) HelpText() string {
	return "Check for a newer release. Installing replaces the current executable."
}

// Keys implements screen.Screen.
func (m *Model) Keys() []keys.Binding {
	switch m.mode {
	case modePrompt:
		return []keys.Binding{m.keys.Install, m.keys.Dismiss}
	case modeInstalling:
		return nil
	case modeLoading, modeChecking:
		return []keys.Binding{keys.Back()}
	default:
		return []keys.Binding{m.keys.Check, keys.Back()}
	}
}

// Resize implements screen.Screen.
func (m *Model) Resize(width, height int) {
	m.width = max(width, 0)
	m.height = max(height, 0)
}

// Update implements screen.Screen.
func (m *Model) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case loadedMsg:
		return m.handleLoaded(msg)
	case checkedMsg:
		return m.handleChecked(msg)
	case installedMsg:
		return m.handleInstalled(msg)
	case spinner.TickMsg:
		if m.mode != modeChecking && m.mode != modeInstalling {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleKey(press tea.KeyPressMsg) (screen.Screen, tea.Cmd) {
	// The executable is being replaced; leave the screen and its worker alone.
	if m.mode == modeInstalling {
		return m, nil
	}

	if m.mode == modePrompt {
		switch {
		case keys.Matches(press, m.keys.Install):
			return m, m.startInstall()
		case keys.Matches(press, m.keys.Dismiss):
			m.mode = modeIdle
			m.statusErr = false
			m.status = "update dismissed."
		}
		return m, nil
	}

	if keys.Matches(press, keys.Back()) {
		return m, screen.Pop()
	}
	// A manual check cannot overtake the load that precedes it.
	if m.mode == modeLoading {
		return m, nil
	}
	if keys.Matches(press, m.keys.Check) {
		return m, m.startCheck()
	}
	return m, nil
}

func (m *Model) handleLoaded(msg loadedMsg) (screen.Screen, tea.Cmd) {
	if msg.epoch != m.epoch {
		return m, nil
	}
	m.currentVersion = msg.snapshot.CurrentVersion
	m.availableVersion = msg.snapshot.AvailableVersion
	m.checkedAt = msg.snapshot.CheckedAt
	m.checkErr = msg.snapshot.CheckError
	m.loadErr = msg.err

	// The cache is context only: every visit checks online, so the prompt is
	// shown from a fresh result rather than from stale cached data.
	return m, m.startCheck()
}

func (m *Model) handleChecked(msg checkedMsg) (screen.Screen, tea.Cmd) {
	if msg.epoch != m.epoch {
		return m, nil
	}
	m.mode = modeIdle
	if !msg.result.CheckedAt.IsZero() {
		m.checkedAt = msg.result.CheckedAt
	}
	if msg.err != nil {
		// The fresh failure supersedes the cached error row.
		m.checkErr = ""
		m.statusErr = true
		m.status = "update check failed: " + msg.err.Error()
		return m, nil
	}

	m.loadErr = nil
	m.checkErr = ""
	m.availableVersion = msg.result.LatestVersion
	m.updateAvailable = msg.result.UpdateAvailable
	if m.updateAvailable {
		m.mode = modePrompt
		m.status = ""
		m.statusErr = false
		return m, nil
	}
	m.statusErr = false
	m.status = "sshex is up to date (" + versionLabel(m.currentVersion) + ")."
	return m, nil
}

func (m *Model) handleInstalled(msg installedMsg) (screen.Screen, tea.Cmd) {
	if msg.epoch != m.epoch {
		return m, nil
	}
	m.mode = modeIdle
	if msg.err != nil {
		m.statusErr = true
		m.status = "update failed: " + msg.err.Error()
		return m, nil
	}
	if !msg.result.Updated {
		m.updateAvailable = false
		m.availableVersion = msg.result.LatestVersion
		m.statusErr = false
		m.status = "sshex is already up to date (" + versionLabel(msg.result.CurrentVersion) + ")."
		return m, nil
	}

	m.updateAvailable = false
	m.availableVersion = msg.result.LatestVersion
	m.statusErr = false
	m.status = "Updated sshex from " + versionLabel(msg.result.CurrentVersion) +
		" to " + versionLabel(msg.result.LatestVersion) +
		". Restart sshex to use the new version."
	return m, nil
}

// load reads the cached state, then starts the online check. The two steps are
// sequential so a slow check cannot overwrite fresher values with the cache.
func (m *Model) load() tea.Cmd {
	m.mode = modeLoading
	m.updateAvailable = false
	m.epoch++
	epoch := m.epoch

	source := m.source
	if source == nil {
		return func() tea.Msg {
			return loadedMsg{err: errors.New("no update source configured"), epoch: epoch}
		}
	}
	return func() tea.Msg {
		snapshot, err := source.Load()
		return loadedMsg{snapshot: snapshot, err: err, epoch: epoch}
	}
}

func (m *Model) startCheck() tea.Cmd {
	if m.mode == modeChecking || m.mode == modeInstalling {
		return nil
	}
	m.mode = modeChecking
	m.status = ""
	m.statusErr = false
	m.epoch++
	epoch := m.epoch

	checker := m.checker
	if checker == nil {
		m.mode = modeIdle
		m.statusErr = true
		m.status = "checking for updates is not available."
		return nil
	}
	// A fresh spinner has a new ID, so ticks scheduled by an earlier
	// operation are rejected instead of starting a second frame chain.
	m.resetSpinner()
	return tea.Batch(m.spinner.Tick, func() tea.Msg {
		result, err := checker.Check()
		return checkedMsg{result: result, err: err, epoch: epoch}
	})
}

func (m *Model) startInstall() tea.Cmd {
	if m.mode != modePrompt {
		return nil
	}
	installer := m.installer
	if installer == nil {
		m.mode = modeIdle
		m.statusErr = true
		m.status = "installing updates is not available."
		return nil
	}

	m.mode = modeInstalling
	m.status = ""
	m.statusErr = false
	m.epoch++
	epoch := m.epoch
	m.resetSpinner()
	return tea.Batch(m.spinner.Tick, func() tea.Msg {
		result, err := installer.Install()
		return installedMsg{result: result, err: err, epoch: epoch}
	})
}
