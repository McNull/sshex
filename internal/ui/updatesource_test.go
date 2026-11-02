package ui

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	updatescreen "github.com/mcnull/sshex/internal/ui/screen/update"
	"github.com/mcnull/sshex/internal/updater"
	"github.com/mcnull/sshex/internal/versioncheck"
)

type fakeVersionStore struct {
	state versioncheck.State
	err   error
	calls int
}

func (s *fakeVersionStore) Load() (versioncheck.State, error) {
	s.calls++
	return s.state, s.err
}

type fakeLatestChecker struct {
	latest string
	err    error
	calls  int
}

func (c *fakeLatestChecker) LatestVersion(context.Context) (string, error) {
	c.calls++
	return c.latest, c.err
}

type fakeReleaseInstaller struct {
	result  updater.Result
	err     error
	current string
	path    string
	calls   int
}

func (i *fakeReleaseInstaller) Update(_ context.Context, current, path string) (updater.Result, error) {
	i.calls++
	i.current = current
	i.path = path
	return i.result, i.err
}

func testUpdateControl(store updateStateReader, checker updateChecker, installer updateInstaller) updateControl {
	return updateControl{
		currentVersion: "1.2.3",
		store:          store,
		checker:        checker,
		installer:      installer,
		executable:     func() (string, error) { return "/usr/local/bin/sshex", nil },
		now:            func() time.Time { return time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC) },
	}
}

func TestUpdateControlLoadMapsCachedState(t *testing.T) {
	checkedAt := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	store := &fakeVersionStore{state: versioncheck.State{
		CheckedAt:        checkedAt,
		AvailableVersion: "2.0.0",
		Error:            "previous failure",
	}}
	control := testUpdateControl(store, &fakeLatestChecker{}, &fakeReleaseInstaller{})

	snapshot, err := control.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if snapshot.CurrentVersion != "1.2.3" {
		t.Fatalf("current version = %q, want 1.2.3", snapshot.CurrentVersion)
	}
	if snapshot.AvailableVersion != "2.0.0" {
		t.Fatalf("available version = %q, want 2.0.0", snapshot.AvailableVersion)
	}
	if !snapshot.CheckedAt.Equal(checkedAt) {
		t.Fatalf("checked at = %v, want %v", snapshot.CheckedAt, checkedAt)
	}
	if snapshot.CheckError != "previous failure" {
		t.Fatalf("check error = %q, want previous failure", snapshot.CheckError)
	}
}

func TestUpdateControlLoadReturnsCurrentVersionOnError(t *testing.T) {
	store := &fakeVersionStore{err: errors.New("parse version state")}
	control := testUpdateControl(store, &fakeLatestChecker{}, &fakeReleaseInstaller{})

	snapshot, err := control.Load()
	if err == nil {
		t.Fatal("Load() error = nil, want the store error")
	}
	if snapshot.CurrentVersion != "1.2.3" {
		t.Fatalf("current version = %q, want 1.2.3", snapshot.CurrentVersion)
	}
}

func TestUpdateControlCheckFindsNewerRelease(t *testing.T) {
	store := &fakeVersionStore{}
	checker := &fakeLatestChecker{latest: "2.0.0"}
	control := testUpdateControl(store, checker, &fakeReleaseInstaller{})

	result, err := control.Check()
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if result.LatestVersion != "2.0.0" {
		t.Fatalf("latest version = %q, want 2.0.0", result.LatestVersion)
	}
	if !result.UpdateAvailable {
		t.Fatal("UpdateAvailable = false, want true")
	}
	want := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	if !result.CheckedAt.Equal(want) {
		t.Fatalf("checked at = %v, want %v", result.CheckedAt, want)
	}
	if checker.calls != 1 {
		t.Fatalf("checker calls = %d, want 1", checker.calls)
	}
}

func TestUpdateControlCheckLeavesStoreAlone(t *testing.T) {
	store := &fakeVersionStore{}
	control := testUpdateControl(store, &fakeLatestChecker{latest: "2.0.0"}, &fakeReleaseInstaller{})

	if _, err := control.Check(); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	// The store interface is read-only by design; this guards against an
	// accidental Save being added to Check.
	if store.calls != 0 {
		t.Fatalf("store calls = %d, want 0 for a check", store.calls)
	}
}

func TestUpdateControlCheckReportsFailure(t *testing.T) {
	checker := &fakeLatestChecker{err: errors.New("offline")}
	control := testUpdateControl(&fakeVersionStore{}, checker, &fakeReleaseInstaller{})

	result, err := control.Check()
	if err == nil {
		t.Fatal("Check() error = nil, want the checker error")
	}
	if result.LatestVersion != "" {
		t.Fatalf("latest version = %q, want empty", result.LatestVersion)
	}
	if result.CheckedAt.IsZero() {
		t.Fatal("CheckedAt is zero, want the attempt time")
	}
}

func TestUpdateControlInstallPassesExecutableAndMapsResult(t *testing.T) {
	installer := &fakeReleaseInstaller{result: updater.Result{
		CurrentVersion: "1.2.3",
		LatestVersion:  "2.0.0",
		Updated:        true,
	}}
	control := testUpdateControl(&fakeVersionStore{}, &fakeLatestChecker{}, installer)

	result, err := control.Install()
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if installer.calls != 1 {
		t.Fatalf("installer calls = %d, want 1", installer.calls)
	}
	if installer.current != "1.2.3" {
		t.Fatalf("current version = %q, want 1.2.3", installer.current)
	}
	if installer.path != "/usr/local/bin/sshex" {
		t.Fatalf("executable = %q, want /usr/local/bin/sshex", installer.path)
	}
	want := updatescreen.Result{CurrentVersion: "1.2.3", LatestVersion: "2.0.0", Updated: true}
	if result != want {
		t.Fatalf("result = %+v, want %+v", result, want)
	}
}

func TestUpdateControlInstallExecutableError(t *testing.T) {
	installer := &fakeReleaseInstaller{}
	control := testUpdateControl(&fakeVersionStore{}, &fakeLatestChecker{}, installer)
	control.executable = func() (string, error) { return "", errors.New("no executable") }

	_, err := control.Install()
	if err == nil {
		t.Fatal("Install() error = nil, want the executable error")
	}
	if installer.calls != 0 {
		t.Fatalf("installer calls = %d, want 0", installer.calls)
	}
}

func TestUpdateControlInstallPropagatesError(t *testing.T) {
	installer := &fakeReleaseInstaller{err: errors.New("checksum mismatch")}
	control := testUpdateControl(&fakeVersionStore{}, &fakeLatestChecker{}, installer)

	if _, err := control.Install(); err == nil {
		t.Fatal("Install() error = nil, want the installer error")
	}
}

func TestVersionStateReaderUsesOverridePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "version.yml")
	t.Setenv("SSHEX_VERSION_STATE", path)
	checkedAt := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	if err := versioncheck.NewStore(path).Save(versioncheck.State{
		CheckedAt:        checkedAt,
		AvailableVersion: "2.0.0",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	state, err := (versionStateReader{}).Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if state.AvailableVersion != "2.0.0" {
		t.Fatalf("available version = %q, want 2.0.0", state.AvailableVersion)
	}
	if !state.CheckedAt.Equal(checkedAt) {
		t.Fatalf("checked at = %v, want %v", state.CheckedAt, checkedAt)
	}
}
