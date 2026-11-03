package sessionfile

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcnull/sshex/internal/model"
)

func TestDiscoverLocalIgnoresClientSession(t *testing.T) {
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	t.Setenv("SSHEX_HOME", t.TempDir())

	sockDir, err := os.MkdirTemp("/tmp", "sshex-")
	if err != nil {
		t.Fatalf("create socket dir: %v", err)
	}
	defer os.RemoveAll(sockDir)

	localSock := filepath.Join(sockDir, "local.sock")
	localListener, err := net.Listen("unix", localSock)
	if err != nil {
		t.Fatalf("listen local: %v", err)
	}
	defer localListener.Close()
	remoteSock := filepath.Join(sockDir, "remote.sock")
	remoteListener, err := net.Listen("unix", remoteSock)
	if err != nil {
		t.Fatalf("listen remote: %v", err)
	}
	defer remoteListener.Close()

	localDir, err := LocalDir()
	if err != nil {
		t.Fatalf("LocalDir() error: %v", err)
	}
	remoteDir, err := RemoteDir()
	if err != nil {
		t.Fatalf("RemoteDir() error: %v", err)
	}
	if err := WriteLocal(localDir, File{ID: "local", Endpoint: model.Endpoint{Kind: model.EndpointUnix, Address: localSock}}); err != nil {
		t.Fatalf("write local session: %v", err)
	}
	if err := WriteLocal(remoteDir, File{ID: "remote", Endpoint: model.Endpoint{Kind: model.EndpointUnix, Address: remoteSock}}); err != nil {
		t.Fatalf("write remote session: %v", err)
	}
	t.Setenv("SSHEX_SESSION", "remote")

	current, err := Current()
	if err != nil {
		t.Fatalf("Current() error: %v", err)
	}
	if current.ID != "remote" {
		t.Fatalf("Current() = %q, want remote", current.ID)
	}

	local, err := DiscoverLocal()
	if err != nil {
		t.Fatalf("DiscoverLocal() error: %v", err)
	}
	if local.ID != "local" {
		t.Fatalf("DiscoverLocal() = %q, want local", local.ID)
	}

	active, err := ListActiveLocal()
	if err != nil {
		t.Fatalf("ListActiveLocal() error: %v", err)
	}
	if len(active) != 1 || active[0].ID != "local" {
		t.Fatalf("ListActiveLocal() = %+v, want [local]", active)
	}
}

func TestCurrentWithoutSession(t *testing.T) {
	t.Setenv("SSHEX_SESSION", "")
	if _, err := Current(); !errors.Is(err, ErrNone) {
		t.Fatalf("Current() error = %v, want ErrNone", err)
	}
}

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
