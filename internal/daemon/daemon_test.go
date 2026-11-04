package daemon

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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

func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "sshex-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestEnsureReusesRunningService(t *testing.T) {
	base := shortTempDir(t)
	t.Setenv("XDG_RUNTIME_DIR", base)

	sock := filepath.Join(base, "daemon.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	info := RuntimeInfo{PID: os.Getpid(), Endpoint: model.Endpoint{Kind: model.EndpointUnix, Address: sock}, Token: "tok", Protocol: ProtocolVersion}
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

func TestEnsureVerboseReportsRunningDaemonVersion(t *testing.T) {
	base := shortTempDir(t)
	t.Setenv("XDG_RUNTIME_DIR", base)
	sock := filepath.Join(base, "daemon.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if err := WriteRuntime(RuntimeInfo{
		PID: os.Getpid(), Endpoint: model.Endpoint{Kind: model.EndpointUnix, Address: sock},
		Protocol: ProtocolVersion, Version: "9.8.7",
	}); err != nil {
		t.Fatal(err)
	}
	defer RemoveRuntime()
	var log bytes.Buffer
	if _, err := EnsureVerbose(context.Background(), "", &log); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"found running daemon", "version=9.8.7", "reusing compatible daemon"} {
		if !strings.Contains(log.String(), text) {
			t.Fatalf("log %q does not contain %q", log.String(), text)
		}
	}
}

func TestEnsureVerboseReportsIncompatibleDaemonAndRestart(t *testing.T) {
	base := shortTempDir(t)
	t.Setenv("XDG_RUNTIME_DIR", base)
	sock := filepath.Join(base, "daemon.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	oldSignal, oldSpawn := signalProcess, spawn
	t.Cleanup(func() { signalProcess, spawn = oldSignal, oldSpawn })
	signalProcess = func(_ int, _ syscall.Signal) error { return ln.Close() }
	if err := WriteRuntime(RuntimeInfo{
		PID: os.Getpid() + 1, Endpoint: model.Endpoint{Kind: model.EndpointUnix, Address: sock}, Version: "old-version", Protocol: ProtocolVersion - 1,
	}); err != nil {
		t.Fatal(err)
	}
	defer RemoveRuntime()
	spawn = func(_ context.Context, _ string) error {
		fresh, err := net.Listen("unix", sock)
		if err != nil {
			return err
		}
		t.Cleanup(func() { _ = fresh.Close() })
		return WriteRuntime(RuntimeInfo{PID: os.Getpid(), Endpoint: model.Endpoint{Kind: model.EndpointUnix, Address: sock}, Version: "new-version", Protocol: ProtocolVersion})
	}
	var log bytes.Buffer
	if _, err := EnsureVerbose(context.Background(), "", &log); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"version=old-version", "protocol is incompatible", "starting daemon", "version=new-version"} {
		if !strings.Contains(log.String(), text) {
			t.Fatalf("log %q does not contain %q", log.String(), text)
		}
	}
}

func TestEnsureRestartsIncompatibleBroker(t *testing.T) {
	base := shortTempDir(t)
	t.Setenv("XDG_RUNTIME_DIR", base)
	sock := filepath.Join(base, "daemon.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	oldSignal, oldSpawn := signalProcess, spawn
	t.Cleanup(func() { signalProcess, spawn = oldSignal, oldSpawn })
	signalProcess = func(_ int, _ syscall.Signal) error { return ln.Close() }
	if err := WriteRuntime(RuntimeInfo{
		PID:      os.Getpid() + 1,
		Endpoint: model.Endpoint{Kind: model.EndpointUnix, Address: sock},
		Token:    "stale",
		Protocol: 0,
	}); err != nil {
		t.Fatal(err)
	}
	defer RemoveRuntime()

	spawn = func(_ context.Context, _ string) error {
		fresh, err := net.Listen("unix", sock)
		if err != nil {
			return err
		}
		// Keep the replacement endpoint alive until the test finishes.
		t.Cleanup(func() { _ = fresh.Close() })
		return WriteRuntime(RuntimeInfo{PID: os.Getpid(), Endpoint: model.Endpoint{Kind: model.EndpointUnix, Address: sock}, Token: "fresh", Protocol: ProtocolVersion})
	}

	got, err := Ensure(context.Background(), "")
	if err != nil {
		t.Fatalf("Ensure() error: %v", err)
	}
	if got.Token != "fresh" || got.Protocol != ProtocolVersion {
		t.Fatalf("Ensure() = %+v, want fresh compatible broker", got)
	}
}

func TestEnsureSpawnsWhenStale(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", base)

	if err := WriteRuntime(RuntimeInfo{PID: 1, Endpoint: model.Endpoint{Kind: model.EndpointUnix, Address: filepath.Join(base, "missing.sock")}, Protocol: ProtocolVersion}); err != nil {
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
		return WriteRuntime(RuntimeInfo{PID: os.Getpid(), Endpoint: model.Endpoint{Kind: model.EndpointUnix, Address: sock}, Token: "fresh", Protocol: ProtocolVersion})
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
