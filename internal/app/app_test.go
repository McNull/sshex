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
	base := shortTempDir(t)
	t.Setenv("SSHEX_CONTROL_BASE", base)

	app, err := New(config.Default(), filepath.Join(t.TempDir(), "config.yml"))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	path, err := app.NewControlPath()
	if err != nil {
		t.Fatalf("NewControlPath() error: %v", err)
	}
	if !strings.HasPrefix(path, filepath.Join(base, "sshex-")) {
		t.Fatalf("NewControlPath() = %q, want a uid-scoped dir under %q", path, base)
	}
	if filepath.Base(filepath.Dir(path)) != "control" {
		t.Fatalf("NewControlPath() dir = %q, want a control dir", filepath.Dir(path))
	}
	if !strings.HasSuffix(path, ".ctl") {
		t.Fatalf("NewControlPath() = %q, want .ctl suffix", path)
	}
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatalf("control dir not created: %v", err)
	}
}

// TestNewControlPathIsShort guards the macOS unix-socket limit: OpenSSH adds a
// random suffix while creating the master socket.
func TestNewControlPathIsShort(t *testing.T) {
	t.Setenv("SSHEX_CONTROL_BASE", "/tmp")

	app, err := New(config.Default(), filepath.Join(t.TempDir(), "config.yml"))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	path, err := app.NewControlPath()
	if err != nil {
		t.Fatalf("NewControlPath() error: %v", err)
	}
	if len(path)+20 > 100 {
		t.Fatalf("NewControlPath() = %q (%d bytes); too long for a unix socket", path, len(path))
	}
}

func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "sshex-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
