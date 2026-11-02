package update

import (
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mcnull/sshex/internal/ui/screen"
	"github.com/mcnull/sshex/internal/ui/theme"
)

type fakeSource struct {
	snapshot Snapshot
	err      error
	calls    int
}

func (s *fakeSource) Load() (Snapshot, error) {
	s.calls++
	return s.snapshot, s.err
}

type fakeChecker struct {
	result CheckResult
	err    error
	calls  int
}

func (c *fakeChecker) Check() (CheckResult, error) {
	c.calls++
	return c.result, c.err
}

type fakeInstaller struct {
	result Result
	err    error
	calls  int
}

func (i *fakeInstaller) Install() (Result, error) {
	i.calls++
	return i.result, i.err
}

func checkedAt(hour int) time.Time {
	return time.Date(2026, 11, 2, hour, 0, 0, 0, time.UTC)
}

func newTestScreen(t *testing.T) (*Model, *fakeSource, *fakeChecker, *fakeInstaller) {
	t.Helper()
	styles := theme.NewStyles(theme.Load(theme.Options{NoColor: true}))
	source := &fakeSource{snapshot: Snapshot{
		CurrentVersion: "1.2.3",
		CheckedAt:      checkedAt(10),
	}}
	checker := &fakeChecker{result: CheckResult{
		LatestVersion: "1.2.3",
		CheckedAt:     checkedAt(12),
	}}
	installer := &fakeInstaller{}
	m := New(Options{
		Styles:    &styles,
		Source:    source,
		Checker:   checker,
		Installer: installer,
	})
	m.Resize(80, 14)
	return m, source, checker, installer
}

// deliver runs a command tree and feeds every resulting message back into the
// screen. Spinner tick chains are stopped after one frame so tests stay fast.
func deliver(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, sub := range batch {
			deliver(t, m, sub)
		}
		return
	}
	if _, ok := msg.(spinner.TickMsg); ok {
		m.Update(msg)
		return
	}
	_, next := m.Update(msg)
	deliver(t, m, next)
}

func openAndCheck(t *testing.T, m *Model) {
	t.Helper()
	deliver(t, m, m.Init())
}

func press(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func prompt(t *testing.T, m *Model, checker *fakeChecker) {
	t.Helper()
	checker.result = CheckResult{
		LatestVersion:   "2.0.0",
		UpdateAvailable: true,
		CheckedAt:       checkedAt(12),
	}
	openAndCheck(t, m)
	if m.mode != modePrompt {
		t.Fatalf("mode = %d, want prompt", m.mode)
	}
}

func TestInitLoadsCacheThenChecks(t *testing.T) {
	m, source, checker, _ := newTestScreen(t)
	openAndCheck(t, m)

	if source.calls != 1 {
		t.Fatalf("cache loads = %d, want 1", source.calls)
	}
	if checker.calls != 1 {
		t.Fatalf("online checks = %d, want 1", checker.calls)
	}
	if m.mode != modeIdle {
		t.Fatalf("mode = %d, want idle", m.mode)
	}
}

func TestCachedStateIsShownWhileChecking(t *testing.T) {
	m, source, _, _ := newTestScreen(t)
	source.snapshot = Snapshot{
		CurrentVersion:   "1.2.3",
		AvailableVersion: "2.0.0",
		CheckedAt:        checkedAt(10),
		CheckError:       "previous failure",
	}

	loaded := m.Init()()
	_, cmd := m.Update(loaded)

	if m.mode != modeChecking {
		t.Fatalf("mode = %d, want checking", m.mode)
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{
		"v1.2.3", "v2.0.0", "2026-11-02 10:00", "previous failure",
		"checking for updates",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("checking view is missing %q:\n%s", want, view)
		}
	}
	deliver(t, m, cmd)
}

func TestUpToDateShowsStatusAndCheckKeys(t *testing.T) {
	m, _, _, _ := newTestScreen(t)
	openAndCheck(t, m)

	view := ansi.Strip(m.View())
	if !strings.Contains(view, "sshex is up to date (v1.2.3).") {
		t.Fatalf("up-to-date status is missing:\n%s", view)
	}
	if !hasHelp(m.Keys(), "enter/r", "check") {
		t.Fatalf("footer is missing the check binding: %v", helpKeys(m.Keys()))
	}
}

func TestPromptAndDismiss(t *testing.T) {
	m, _, checker, _ := newTestScreen(t)
	prompt(t, m, checker)

	view := ansi.Strip(m.View())
	for _, want := range []string{
		"Update available: v1.2.3 → v2.0.0",
		"The sshex executable will be replaced. Restart sshex to use the new version.",
		"enter confirm · esc dismiss",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("prompt view is missing %q:\n%s", want, view)
		}
	}
	if !hasHelp(m.Keys(), "enter", "confirm") || !hasHelp(m.Keys(), "esc", "dismiss") {
		t.Fatalf("prompt footer is wrong: %v", helpKeys(m.Keys()))
	}

	_, cmd := m.Update(press(tea.KeyEsc))
	if cmd != nil {
		t.Fatal("dismissing the prompt must not pop the screen")
	}
	if m.mode != modeIdle {
		t.Fatalf("mode = %d, want idle after dismiss", m.mode)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "update dismissed.") {
		t.Fatalf("dismissed status is missing:\n%s", view)
	}
}

func TestInstallSuccess(t *testing.T) {
	m, _, checker, installer := newTestScreen(t)
	prompt(t, m, checker)
	installer.result = Result{
		CurrentVersion: "1.2.3",
		LatestVersion:  "2.0.0",
		Updated:        true,
	}

	_, cmd := m.Update(press(tea.KeyEnter))
	if m.mode != modeInstalling {
		t.Fatalf("mode = %d, want installing", m.mode)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "updating to v2.0.0") {
		t.Fatalf("install spinner is missing:\n%s", view)
	}
	deliver(t, m, cmd)

	if installer.calls != 1 {
		t.Fatalf("installs = %d, want 1", installer.calls)
	}
	if m.mode != modeIdle {
		t.Fatalf("mode = %d, want idle", m.mode)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Updated sshex from v1.2.3 to v2.0.0. Restart sshex to use the new version.") {
		t.Fatalf("install status is missing:\n%s", view)
	}
}

func TestInstallNotUpdated(t *testing.T) {
	m, _, checker, installer := newTestScreen(t)
	prompt(t, m, checker)
	installer.result = Result{CurrentVersion: "2.0.0", LatestVersion: "2.0.0"}

	_, cmd := m.Update(press(tea.KeyEnter))
	deliver(t, m, cmd)

	if view := ansi.Strip(m.View()); !strings.Contains(view, "sshex is already up to date (v2.0.0).") {
		t.Fatalf("already-up-to-date status is missing:\n%s", view)
	}
	if m.updateAvailable {
		t.Fatal("UpdateAvailable = true after the updater reported no change")
	}
}

func TestInstallError(t *testing.T) {
	m, _, checker, installer := newTestScreen(t)
	prompt(t, m, checker)
	installer.err = errors.New("permission denied")

	_, cmd := m.Update(press(tea.KeyEnter))
	deliver(t, m, cmd)

	if view := ansi.Strip(m.View()); !strings.Contains(view, "update failed: permission denied") {
		t.Fatalf("install error is missing:\n%s", view)
	}
	if m.statusErr != true {
		t.Fatal("install error should use the error style")
	}
}

func TestCheckErrorAllowsRetry(t *testing.T) {
	m, _, checker, _ := newTestScreen(t)
	checker.err = errors.New("offline")
	openAndCheck(t, m)

	if m.mode != modeIdle {
		t.Fatalf("mode = %d, want idle", m.mode)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "update check failed: offline") {
		t.Fatalf("check error is missing:\n%s", view)
	}
	if !hasHelp(m.Keys(), "enter/r", "check") {
		t.Fatalf("retry binding is missing: %v", helpKeys(m.Keys()))
	}
}

func TestCheckIsIgnoredWhileInFlight(t *testing.T) {
	m, _, checker, _ := newTestScreen(t)
	loaded := m.Init()()
	_, cmd := m.Update(loaded)
	if m.mode != modeChecking {
		t.Fatalf("mode = %d, want checking", m.mode)
	}

	_, again := m.Update(press(tea.KeyEnter))
	if again != nil {
		t.Fatal("a second check started while the first was in flight")
	}
	if checker.calls != 0 {
		t.Fatalf("checker calls = %d, want 0 before the command runs", checker.calls)
	}
	deliver(t, m, cmd)
	if checker.calls != 1 {
		t.Fatalf("checker calls = %d, want 1", checker.calls)
	}
}

func TestInstallIgnoresKeys(t *testing.T) {
	m, _, checker, _ := newTestScreen(t)
	prompt(t, m, checker)

	_, installCmd := m.Update(press(tea.KeyEnter))
	if installCmd == nil {
		t.Fatal("install returned no command")
	}
	for _, code := range []rune{tea.KeyEnter, 'r', tea.KeyEsc} {
		if _, cmd := m.Update(press(code)); cmd != nil {
			t.Fatalf("key %q returned a command while installing", code)
		}
	}
	if m.mode != modeInstalling {
		t.Fatalf("mode = %d, want installing", m.mode)
	}
}

func TestEscPopsWhenIdle(t *testing.T) {
	m, _, _, _ := newTestScreen(t)
	openAndCheck(t, m)

	_, cmd := m.Update(press(tea.KeyEsc))
	if cmd == nil {
		t.Fatal("esc returned no command")
	}
	action, ok := cmd().(screen.ActionMsg)
	if !ok || action.Action != screen.ActionPop {
		t.Fatalf("esc action = %+v, want pop", action)
	}
}

func TestResumeReloadsAndChecksAgain(t *testing.T) {
	m, source, checker, _ := newTestScreen(t)
	openAndCheck(t, m)

	source.snapshot = Snapshot{CurrentVersion: "1.2.4", CheckedAt: checkedAt(11)}
	checker.result = CheckResult{LatestVersion: "1.2.4", CheckedAt: checkedAt(13)}
	deliver(t, m, m.Resume())

	if source.calls != 2 {
		t.Fatalf("cache loads = %d, want 2", source.calls)
	}
	if checker.calls != 2 {
		t.Fatalf("online checks = %d, want 2", checker.calls)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "v1.2.4") {
		t.Fatalf("resumed view is missing the new version:\n%s", view)
	}
}

func TestStaleResultIsDiscarded(t *testing.T) {
	m, _, _, _ := newTestScreen(t)
	loaded := m.Init()()
	_, cmd := m.Update(loaded)
	_ = cmd
	staleEpoch := m.epoch

	// A new visit invalidates the in-flight result.
	openAndCheck(t, m)
	if m.mode != modeIdle {
		t.Fatalf("mode = %d, want idle", m.mode)
	}

	m.Update(checkedMsg{
		result: CheckResult{
			LatestVersion:   "9.9.9",
			UpdateAvailable: true,
			CheckedAt:       checkedAt(14),
		},
		epoch: staleEpoch,
	})

	if m.mode != modeIdle {
		t.Fatalf("stale result changed the mode to %d", m.mode)
	}
	if m.updateAvailable {
		t.Fatal("stale result changed the availability")
	}
}

func TestFirstFrameShowsLoading(t *testing.T) {
	m, _, _, _ := newTestScreen(t)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Update") || !strings.Contains(view, "reading cached update state") {
		t.Fatalf("loading frame is missing:\n%s", view)
	}
}

func TestViewFitsContentArea(t *testing.T) {
	m, _, checker, _ := newTestScreen(t)
	prompt(t, m, checker)
	m.Resize(40, 7)

	lines := strings.Split(m.View(), "\n")
	if len(lines) != 7 {
		t.Fatalf("rendered %d lines, want 7", len(lines))
	}
	for i, line := range lines {
		if width := lipgloss.Width(line); width > 40 {
			t.Fatalf("line %d width = %d, want <= 40", i, width)
		}
	}
}

func TestLongErrorIsTruncated(t *testing.T) {
	m, _, checker, _ := newTestScreen(t)
	m.Resize(32, 10)
	checker.result = CheckResult{LatestVersion: "1.2.3", CheckedAt: checkedAt(12)}
	checker.err = errors.New(strings.Repeat("very long failure ", 20))
	openAndCheck(t, m)

	for i, line := range strings.Split(m.View(), "\n") {
		if width := lipgloss.Width(line); width > 32 {
			t.Fatalf("line %d width = %d, want <= 32", i, width)
		}
	}
}

func TestVersionLabels(t *testing.T) {
	for _, test := range []struct {
		input string
		label string
	}{
		{"", "unknown"},
		{"dev", "dev"},
		{"1.2.3", "v1.2.3"},
		{"v1.2.3", "v1.2.3"},
		{"  v2.0.0  ", "v2.0.0"},
	} {
		if got := versionLabel(test.input); got != test.label {
			t.Fatalf("versionLabel(%q) = %q, want %q", test.input, got, test.label)
		}
	}
	if got := versionOrDev(""); got != "dev" {
		t.Fatalf("versionOrDev(\"\") = %q, want dev", got)
	}
	if got := versionOrDev("1.2.3"); got != "v1.2.3" {
		t.Fatalf("versionOrDev(1.2.3) = %q, want v1.2.3", got)
	}
	if got := checkedLabel(time.Time{}); got != "never" {
		t.Fatalf("checkedLabel(zero) = %q, want never", got)
	}
}

func hasHelp(bindings []key.Binding, keys, desc string) bool {
	for _, binding := range bindings {
		if binding.Help().Key == keys && binding.Help().Desc == desc {
			return true
		}
	}
	return false
}

func helpKeys(bindings []key.Binding) []string {
	out := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		out = append(out, binding.Help().Key+" "+binding.Help().Desc)
	}
	return out
}
