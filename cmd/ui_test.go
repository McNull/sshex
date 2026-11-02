package cmd

import (
	"strings"
	"testing"

	"github.com/mcnull/sshex/internal/versioncheck"
)

func TestUICommand(t *testing.T) {
	found := false
	for _, c := range rootCmd.Commands() {
		if c.Name() == "ui" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("ui command is not registered")
	}

	flag := uiCmd.Flags().Lookup("theme")
	if flag == nil {
		t.Fatal("ui --theme flag is not registered")
	}
	if flag.DefValue != "" {
		t.Fatalf("ui --theme default = %q, want empty", flag.DefValue)
	}
}

func TestUpdateAvailableVersion(t *testing.T) {
	path := t.TempDir() + "/version.yml"
	t.Setenv("SSHEX_VERSION_STATE", path)

	if got := updateAvailableVersion(); got != "" {
		t.Fatalf("empty state = %q, want no update", got)
	}

	store := versioncheck.NewStore(path)
	if err := store.Save(versioncheck.State{AvailableVersion: "v999.0.0"}); err != nil {
		t.Fatal(err)
	}
	if got := updateAvailableVersion(); got != "v999.0.0" {
		t.Fatalf("update version = %q, want v999.0.0", got)
	}
}

func TestUIHelp(t *testing.T) {
	var out strings.Builder
	printHelp(&out, "sshex ui")
	if !strings.Contains(out.String(), "sshex ui [options]") {
		t.Fatalf("ui help = %q", out.String())
	}
	if !strings.Contains(out.String(), "--theme") {
		t.Fatalf("ui help is missing --theme: %q", out.String())
	}

	out.Reset()
	printHelp(&out, "sshex")
	if !strings.Contains(out.String(), "ui                    Open the interactive terminal interface") {
		t.Fatalf("root help is missing ui: %q", out.String())
	}
}
