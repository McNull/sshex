package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mcnull/sshex/internal/assets"
	"github.com/mcnull/sshex/internal/versioncheck"
)

func TestRootCommandTreeHasNoDuplicates(t *testing.T) {
	rootCmd.InitDefaultHelpCmd()
	rootCmd.InitDefaultCompletionCmd("")

	counts := make(map[string]int)
	for _, c := range rootCmd.Commands() {
		counts[c.Name()]++
	}

	for name, count := range counts {
		if count != 1 {
			t.Errorf("command %q appears %d times, want 1", name, count)
		}
	}
	if counts["help"] != 1 {
		t.Errorf("help command count = %d, want 1", counts["help"])
	}
	if _, ok := counts["completion"]; ok {
		t.Error("built-in completion command should be disabled")
	}
}

func TestRootConfigFlag(t *testing.T) {
	flag := rootCmd.PersistentFlags().Lookup("config")
	if flag == nil {
		t.Fatal("root --config flag is not registered")
	}
	if flag.Shorthand != "c" {
		t.Fatalf("--config shorthand = %q, want c", flag.Shorthand)
	}
}

func TestRootVersionCheckFlag(t *testing.T) {
	flag := rootCmd.PersistentFlags().Lookup("version-check")
	if flag == nil {
		t.Fatal("root --version-check flag is not registered")
	}
	if flag.Usage != "Perform online version check and exit" {
		t.Fatalf("--version-check usage = %q", flag.Usage)
	}
}

type latestVersionFunc func(context.Context) (string, error)

func (f latestVersionFunc) LatestVersion(ctx context.Context) (string, error) { return f(ctx) }

func TestCheckVersionSynchronouslyReportsLatestVersion(t *testing.T) {
	cmd := rootCmd
	var output strings.Builder
	cmd.SetOut(&output)
	cmd.SetContext(context.Background())
	defer cmd.SetOut(nil)
	path := t.TempDir() + "/version.yml"
	if err := checkVersionSynchronously(cmd, path, latestVersionFunc(func(context.Context) (string, error) {
		return assets.Version(), nil
	})); err != nil {
		t.Fatalf("checkVersionSynchronously() error: %v", err)
	}
	if !strings.Contains(output.String(), "Checking for sshex updates...") {
		t.Fatalf("output missing progress status: %q", output.String())
	}
	if !strings.Contains(output.String(), "sshex is up to date") {
		t.Fatalf("output missing up-to-date status: %q", output.String())
	}
	state, err := versioncheck.NewStore(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.AvailableVersion != assets.Version() || state.CheckedAt.IsZero() || time.Since(state.CheckedAt) > time.Minute {
		t.Fatalf("saved state = %+v", state)
	}
}

func TestCheckVersionSynchronouslyReportsAvailableVersion(t *testing.T) {
	cmd := rootCmd
	var output strings.Builder
	cmd.SetOut(&output)
	cmd.SetContext(context.Background())
	defer cmd.SetOut(nil)
	path := t.TempDir() + "/version.yml"
	const latestVersion = "999.0.0"
	if err := checkVersionSynchronously(cmd, path, latestVersionFunc(func(context.Context) (string, error) {
		return latestVersion, nil
	})); err != nil {
		t.Fatalf("checkVersionSynchronously() error: %v", err)
	}
	want := "Update available: " + latestVersion + "\nRun sshex update to install.\n\n"
	if !strings.Contains(output.String(), want) {
		t.Fatalf("output missing update notice %q: %q", want, output.String())
	}
}

func TestPrintUpdateAvailableUsesTerminalColors(t *testing.T) {
	forceTerminal(t, true)
	t.Setenv("NO_COLOR", "")

	var output strings.Builder
	printUpdateAvailable(&output, "1.2.3")
	want := "Update available: \x1b[33m1.2.3\x1b[0m\nRun \x1b[34msshex update\x1b[0m to install.\n\n"
	if got := output.String(); got != want {
		t.Fatalf("printUpdateAvailable() = %q, want %q", got, want)
	}
}

func TestCheckVersionSynchronouslyReportsFailure(t *testing.T) {
	cmd := rootCmd
	var output strings.Builder
	cmd.SetOut(&output)
	cmd.SetContext(context.Background())
	defer cmd.SetOut(nil)
	path := t.TempDir() + "/version.yml"
	err := checkVersionSynchronously(cmd, path, latestVersionFunc(func(context.Context) (string, error) {
		return "", context.DeadlineExceeded
	}))
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code == 0 {
		t.Fatalf("checkVersionSynchronously() error = %v, want non-zero ExitError", err)
	}
	if !strings.Contains(output.String(), "Update check failed:") || !strings.Contains(output.String(), context.DeadlineExceeded.Error()) {
		t.Fatalf("output missing failure details: %q", output.String())
	}
	state, err := versioncheck.NewStore(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	if state.Error != context.DeadlineExceeded.Error() {
		t.Fatalf("saved error = %q", state.Error)
	}
}

func TestRootHasExecAndCommand(t *testing.T) {
	names := make(map[string]bool)
	for _, c := range rootCmd.Commands() {
		names[c.Name()] = true
	}
	for _, want := range []string{"exec", "command", "update"} {
		if !names[want] {
			t.Errorf("root command %q is not registered", want)
		}
	}
}
