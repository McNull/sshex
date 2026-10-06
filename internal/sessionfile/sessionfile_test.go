package sessionfile

import (
	"net"
	"path/filepath"
	"testing"

	"github.com/mcnull/sshex/internal/model"
)

func TestListActive(t *testing.T) {
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	t.Setenv("SSHEX_HOME", t.TempDir())

	sock := filepath.Join(runtimeDir, "live.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	dir, err := LocalDir()
	if err != nil {
		t.Fatalf("LocalDir() error: %v", err)
	}
	live := File{ID: "live", Endpoint: model.Endpoint{Kind: model.EndpointUnix, Address: sock}, Token: "t", Capabilities: model.Capabilities{model.CapabilityTunnels}}
	dead := File{ID: "dead", Endpoint: model.Endpoint{Kind: model.EndpointUnix, Address: filepath.Join(runtimeDir, "missing.sock")}, Token: "u", Capabilities: model.Capabilities{model.CapabilityTunnels}}
	if err := WriteLocal(dir, live); err != nil {
		t.Fatalf("WriteLocal(live) error: %v", err)
	}
	if err := WriteLocal(dir, dead); err != nil {
		t.Fatalf("WriteLocal(dead) error: %v", err)
	}

	active, err := ListActive()
	if err != nil {
		t.Fatalf("ListActive() error: %v", err)
	}
	if len(active) != 1 || active[0].ID != "live" {
		t.Fatalf("ListActive() = %+v, want [live]", active)
	}
}
