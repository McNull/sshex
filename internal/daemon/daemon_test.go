package daemon

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcnull/sshex/internal/config"
	"github.com/mcnull/sshex/internal/errs"
	"github.com/mcnull/sshex/internal/model"
)

func TestRuntimeLifecycle(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", base)

	info := RuntimeInfo{PID: 1234, Endpoint: model.Endpoint{Kind: model.EndpointUnix, Address: "/tmp/x.sock"}}
	if err := WriteRuntime(info); err != nil {
		t.Fatalf("WriteRuntime() error: %v", err)
	}

	dir, _ := RuntimeDir()
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir error: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != RuntimeDirMode {
		t.Fatalf("dir perm = %o, want %o", perm, RuntimeDirMode)
	}

	path, _ := RuntimeFilePath()
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat file error: %v", err)
	}
	if perm := fileInfo.Mode().Perm(); perm != RuntimeFileMode {
		t.Fatalf("file perm = %o, want %o", perm, RuntimeFileMode)
	}

	got, err := ReadRuntime()
	if err != nil {
		t.Fatalf("ReadRuntime() error: %v", err)
	}
	if got.PID != info.PID || got.Endpoint != info.Endpoint {
		t.Fatalf("ReadRuntime() = %+v, want %+v", got, info)
	}

	if err := RemoveRuntime(); err != nil {
		t.Fatalf("RemoveRuntime() error: %v", err)
	}
	if _, err := ReadRuntime(); err == nil {
		t.Fatal("ReadRuntime() expected error after remove, got nil")
	}
	if err := RemoveRuntime(); err != nil {
		t.Fatalf("RemoveRuntime() on missing file error: %v", err)
	}
}

func TestDaemonRunNotImplemented(t *testing.T) {
	d := New(config.Default())
	if err := d.Run(context.Background()); !errors.Is(err, errs.ErrNotImplemented) {
		t.Fatalf("Run() error = %v, want ErrNotImplemented", err)
	}
}

func TestEnsureReusesRunningService(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", base)

	sock := filepath.Join(base, "daemon.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	info := RuntimeInfo{PID: os.Getpid(), Endpoint: model.Endpoint{Kind: model.EndpointUnix, Address: sock}, Token: "tok"}
	if err := WriteRuntime(info); err != nil {
		t.Fatalf("WriteRuntime() error: %v", err)
	}
	defer RemoveRuntime()

	old := spawn
	called := false
	spawn = func(context.Context, string) error { called = true; return nil }
	defer func() { spawn = old }()

	got, err := Ensure(context.Background(), "")
	if err != nil {
		t.Fatalf("Ensure() error: %v", err)
	}
	if called {
		t.Fatal("Ensure() spawned a service despite a live endpoint")
	}
	if got.Token != "tok" {
		t.Fatalf("Ensure() token = %q, want %q", got.Token, "tok")
	}
}

func TestEnsureSpawnsWhenStale(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", base)

	if err := WriteRuntime(RuntimeInfo{PID: 1, Endpoint: model.Endpoint{Kind: model.EndpointUnix, Address: filepath.Join(base, "missing.sock")}}); err != nil {
		t.Fatalf("WriteRuntime() error: %v", err)
	}
	defer RemoveRuntime()

	sock := filepath.Join(base, "daemon.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	old := spawn
	spawn = func(context.Context, string) error {
		return WriteRuntime(RuntimeInfo{PID: os.Getpid(), Endpoint: model.Endpoint{Kind: model.EndpointUnix, Address: sock}, Token: "fresh"})
	}
	defer func() { spawn = old }()

	got, err := Ensure(context.Background(), "")
	if err != nil {
		t.Fatalf("Ensure() error: %v", err)
	}
	if got.Token != "fresh" {
		t.Fatalf("Ensure() token = %q, want %q", got.Token, "fresh")
	}
}
