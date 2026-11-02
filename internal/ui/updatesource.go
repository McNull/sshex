package ui

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/mcnull/sshex/internal/ui/screen/update"
	"github.com/mcnull/sshex/internal/updater"
	"github.com/mcnull/sshex/internal/versioncheck"
)

const (
	updateCheckTimeout   = 30 * time.Second
	updateInstallTimeout = 5 * time.Minute
)

// updateControl adapts versioncheck and updater for the Update screen. The
// screen only sees its own narrow Source, Checker and Installer interfaces, so
// tests never read version.yml, reach the network or replace the executable.
//
// The check is intentionally screen-only: it does not save versioncheck.State,
// so the CLI notice, the menu indicator and the next background check keep
// their own lifecycle.
type updateControl struct {
	currentVersion string
	store          updateStateReader
	checker        updateChecker
	installer      updateInstaller
	executable     func() (string, error)
	now            func() time.Time
}

// updateStateReader reads the cached version state. It is read-only because the
// UI check must not write version.yml.
type updateStateReader interface {
	Load() (versioncheck.State, error)
}

// updateChecker is the subset of updater.Updater used for online checks.
type updateChecker interface {
	LatestVersion(context.Context) (string, error)
}

// updateInstaller is the subset of updater.Updater used for installs.
type updateInstaller interface {
	Update(context.Context, string, string) (updater.Result, error)
}

func newUpdateControl(version string) updateControl {
	release := updater.New(updater.Config{})
	return updateControl{
		currentVersion: version,
		store:          versionStateReader{},
		checker:        release,
		installer:      release,
		executable:     os.Executable,
		now:            time.Now,
	}
}

// Load implements update.Source. The current version is returned even when the
// cache cannot be read, so the screen can still show it.
func (c updateControl) Load() (update.Snapshot, error) {
	snapshot := update.Snapshot{CurrentVersion: c.currentVersion}
	state, err := c.store.Load()
	if err != nil {
		return snapshot, err
	}
	snapshot.AvailableVersion = state.AvailableVersion
	snapshot.CheckedAt = state.CheckedAt
	snapshot.CheckError = state.Error
	return snapshot, nil
}

// Check implements update.Checker. It performs the online check without
// writing the result to version.yml.
func (c updateControl) Check() (update.CheckResult, error) {
	checkedAt := c.now()
	ctx, cancel := context.WithTimeout(context.Background(), updateCheckTimeout)
	defer cancel()

	latest, err := c.checker.LatestVersion(ctx)
	if err != nil {
		return update.CheckResult{CheckedAt: checkedAt}, err
	}
	return update.CheckResult{
		LatestVersion:   latest,
		UpdateAvailable: versioncheck.IsNewer(latest, c.currentVersion),
		CheckedAt:       checkedAt,
	}, nil
}

// Install implements update.Installer. It mirrors `sshex update`: the updater
// locates the platform archive, verifies its checksum and replaces the running
// executable.
func (c updateControl) Install() (update.Result, error) {
	executable, err := c.executable()
	if err != nil {
		return update.Result{}, fmt.Errorf("locate sshex executable: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), updateInstallTimeout)
	defer cancel()
	result, err := c.installer.Update(ctx, c.currentVersion, executable)
	if err != nil {
		return update.Result{}, err
	}
	return update.Result{
		CurrentVersion: result.CurrentVersion,
		LatestVersion:  result.LatestVersion,
		Updated:        result.Updated,
	}, nil
}

// versionStateReader reads the cached version.yml through versioncheck. The
// path is resolved per call so SSHEX_VERSION_STATE is honored.
type versionStateReader struct{}

func (versionStateReader) Load() (versioncheck.State, error) {
	path, err := versioncheck.DefaultPath()
	if err != nil {
		return versioncheck.State{}, err
	}
	return versioncheck.NewStore(path).Load()
}
