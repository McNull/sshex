package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcnull/sshex/internal/config"
)

func TestNew(t *testing.T) {
	app, err := New(config.Default(), filepath.Join(t.TempDir(), "config.yml"))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if app.Sessions == nil || app.Tunnels == nil || app.Commands == nil || app.Auth == nil {
		t.Fatalf("New() returned incomplete app: %+v", app)
	}
}

func TestNewControlPath(t *testing.T) {
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	app, err := New(config.Default(), filepath.Join(t.TempDir(), "config.yml"))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	path, err := app.NewControlPath()
	if err != nil {
		t.Fatalf("NewControlPath() error: %v", err)
	}
	wantDir := filepath.Join(runtimeDir, "sshex", "control")
	if filepath.Dir(path) != wantDir {
		t.Fatalf("NewControlPath() dir = %q, want %q", filepath.Dir(path), wantDir)
	}
	if !strings.HasSuffix(path, ".ctl") {
		t.Fatalf("NewControlPath() = %q, want .ctl suffix", path)
	}
	if _, err := os.Stat(wantDir); err != nil {
		t.Fatalf("control dir not created: %v", err)
	}
}
