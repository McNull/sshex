package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/mcnull/sshex/internal/errs"
	"github.com/mcnull/sshex/internal/remoteinstall"
	"github.com/mcnull/sshex/internal/sessionfile"
	"github.com/mcnull/sshex/internal/sshx"
)

type fakeInstaller struct {
	paths       remoteinstall.RemotePaths
	uninstalled bool
}

func (f *fakeInstaller) RemotePaths(context.Context, remoteinstall.Runner) (remoteinstall.RemotePaths, error) {
	return f.paths, nil
}

func (f *fakeInstaller) Install(context.Context, remoteinstall.Runner, remoteinstall.RemotePaths) error {
	return nil
}

func (f *fakeInstaller) WriteSession(context.Context, remoteinstall.Runner, remoteinstall.RemotePaths, sessionfile.File) error {
	return nil
}

func (f *fakeInstaller) Remove(context.Context, remoteinstall.Runner, remoteinstall.RemotePaths, sessionfile.File) error {
	return nil
}

func (f *fakeInstaller) Uninstall(context.Context, remoteinstall.Runner, remoteinstall.RemotePaths) error {
	f.uninstalled = true
	return nil
}

type fakeCompletions struct {
	uninstalled bool
}

func (f *fakeCompletions) DetectShell(context.Context, remoteinstall.Runner) (remoteinstall.Shell, bool, error) {
	return "", false, nil
}

func (f *fakeCompletions) Install(context.Context, remoteinstall.Runner, remoteinstall.RemotePaths, remoteinstall.Shell) error {
	return nil
}

func (f *fakeCompletions) Uninstall(context.Context, remoteinstall.Runner, remoteinstall.RemotePaths) error {
	f.uninstalled = true
	return nil
}

type fakeTransport struct {
	called     bool
	adopted    bool
	target     sshx.Target
	controlOpt sshx.ConnectOptions
	conn       sshx.Connection
}

func (f *fakeTransport) Connect(_ context.Context, target sshx.Target, opts sshx.ConnectOptions) (sshx.Connection, error) {
	f.called = true
	f.target = target
	f.controlOpt = opts
	return f.conn, nil
}

func (f *fakeTransport) Adopt(_ context.Context, target sshx.Target, opts sshx.ConnectOptions) (sshx.Connection, error) {
	f.adopted = true
	f.target = target
	f.controlOpt = opts
	return f.conn, nil
}

func (f *fakeTransport) Attach(sshx.Target, sshx.ConnectOptions) sshx.Connection {
	return f.conn
}

type fakeProbe struct {
	sessions []sessionfile.File
	err      error
}

func (f *fakeProbe) Active(context.Context, remoteinstall.Runner, remoteinstall.RemotePaths) ([]sessionfile.File, error) {
	return f.sessions, f.err
}

func newInstallService(t *testing.T, installer Installer, completions Completer, transport Transport, local remoteinstall.Runner) *InstallService {
	t.Helper()
	return newInstallServiceWithProbe(t, installer, completions, transport, local, nil)
}

func newInstallServiceWithProbe(t *testing.T, installer Installer, completions Completer, transport Transport, local remoteinstall.Runner, probe SessionProbe) *InstallService {
	t.Helper()
	return NewInstallService(InstallServiceConfig{
		Transport:   transport,
		Installer:   installer,
		Completions: completions,
		Probe:       probe,
		Local:       local,
		IDs:         func() string { return "test-id" },
		ControlDir:  "/tmp/control",
	})
}

func TestInstallServiceUninstallLocal(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Setenv("SSHEX_HOME", t.TempDir())

	installer := &fakeInstaller{paths: remoteinstall.RemotePaths{DataDir: "/data", RunDir: "/run"}}
	completions := &fakeCompletions{}
	transport := &fakeTransport{}
	svc := newInstallService(t, installer, completions, transport, &fakeConn{})

	if err := svc.Uninstall(context.Background(), UninstallRequest{}); err != nil {
		t.Fatalf("Uninstall() error: %v", err)
	}
	if transport.called {
		t.Fatal("Uninstall() used the transport for a local uninstall")
	}
	if !installer.uninstalled || !completions.uninstalled {
		t.Fatalf("Uninstall() installer=%v completions=%v, want both true", installer.uninstalled, completions.uninstalled)
	}
}

func TestInstallServiceUninstallRemote(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Setenv("SSHEX_HOME", t.TempDir())

	installer := &fakeInstaller{paths: remoteinstall.RemotePaths{DataDir: "/data", RunDir: "/run"}}
	completions := &fakeCompletions{}
	conn := &fakeConn{}
	transport := &fakeTransport{conn: conn}
	svc := newInstallService(t, installer, completions, transport, nil)

	req := UninstallRequest{User: "bob", Host: "example.com", Port: 2222, IdentityFile: "/key", JumpHosts: "ssh01", Options: []string{"StrictHostKeyChecking=no"}}
	if err := svc.Uninstall(context.Background(), req); err != nil {
		t.Fatalf("Uninstall() error: %v", err)
	}
	if !transport.called {
		t.Fatal("Uninstall() did not use the transport for a remote uninstall")
	}
	if transport.target.User != "bob" || transport.target.Host != "example.com" || transport.target.Port != 2222 {
		t.Fatalf("Uninstall() target = %+v", transport.target)
	}
	if transport.target.JumpHosts != "ssh01" {
		t.Fatalf("Uninstall() jump hosts = %q, want ssh01", transport.target.JumpHosts)
	}
	wantControl := filepath.Join("/tmp/control", "test-id.ctl")
	if transport.controlOpt.ControlPath != wantControl {
		t.Fatalf("Uninstall() control path = %q, want %q", transport.controlOpt.ControlPath, wantControl)
	}
	if !installer.uninstalled || !completions.uninstalled {
		t.Fatalf("Uninstall() installer=%v completions=%v, want both true", installer.uninstalled, completions.uninstalled)
	}
}

func TestInstallServiceUninstallLocalUnavailable(t *testing.T) {
	svc := newInstallService(t, &fakeInstaller{}, &fakeCompletions{}, &fakeTransport{}, nil)
	if err := svc.Uninstall(context.Background(), UninstallRequest{}); err == nil {
		t.Fatal("Uninstall() with no local runner error = nil, want error")
	}
}

func TestInstallServiceUninstallJumpRequiresHost(t *testing.T) {
	svc := newInstallService(t, &fakeInstaller{}, &fakeCompletions{}, &fakeTransport{}, &fakeConn{})
	err := svc.Uninstall(context.Background(), UninstallRequest{JumpHosts: "ssh01"})
	if !errors.Is(err, errs.ErrInvalidInput) {
		t.Fatalf("Uninstall() jump without host error = %v, want ErrInvalidInput", err)
	}
}

func TestInstallServiceUninstallRefusesActiveSessions(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Setenv("SSHEX_HOME", t.TempDir())

	installer := &fakeInstaller{paths: remoteinstall.RemotePaths{DataDir: "/data", RunDir: "/run"}}
	probe := &fakeProbe{sessions: []sessionfile.File{{ID: "s1", Target: "bob@example.com"}}}
	transport := &fakeTransport{conn: &fakeConn{}}
	svc := newInstallServiceWithProbe(t, installer, &fakeCompletions{}, transport, nil, probe)

	err := svc.Uninstall(context.Background(), UninstallRequest{User: "bob", Host: "example.com"})
	var active *ActiveSessionsError
	if !errors.As(err, &active) {
		t.Fatalf("Uninstall() error = %v, want ActiveSessionsError", err)
	}
	if len(active.Sessions) != 1 || active.Sessions[0].ID != "s1" {
		t.Fatalf("ActiveSessionsError sessions = %+v", active.Sessions)
	}
	if installer.uninstalled {
		t.Fatal("Uninstall() removed the install despite active sessions")
	}
}

func TestInstallServiceUninstallForceProceeds(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Setenv("SSHEX_HOME", t.TempDir())

	installer := &fakeInstaller{paths: remoteinstall.RemotePaths{DataDir: "/data", RunDir: "/run"}}
	probe := &fakeProbe{sessions: []sessionfile.File{{ID: "s1", Target: "bob@example.com"}}}
	transport := &fakeTransport{conn: &fakeConn{}}
	svc := newInstallServiceWithProbe(t, installer, &fakeCompletions{}, transport, nil, probe)

	if err := svc.Uninstall(context.Background(), UninstallRequest{User: "bob", Host: "example.com", Force: true}); err != nil {
		t.Fatalf("Uninstall() error: %v", err)
	}
	if !installer.uninstalled {
		t.Fatal("Uninstall() did not remove the install with Force")
	}
}

func TestInstallServiceUninstallStopsBroker(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Setenv("SSHEX_HOME", t.TempDir())

	installer := &fakeInstaller{paths: remoteinstall.RemotePaths{DataDir: "/data", RunDir: "/run"}}
	stopped := false
	svc := NewInstallService(InstallServiceConfig{
		Transport:   &fakeTransport{conn: &fakeConn{}},
		Installer:   installer,
		Completions: &fakeCompletions{},
		Local:       &fakeConn{},
		StopBroker:  func() { stopped = true },
		IDs:         func() string { return "test-id" },
		ControlDir:  "/tmp/control",
	})

	if err := svc.Uninstall(context.Background(), UninstallRequest{}); err != nil {
		t.Fatalf("Uninstall() error: %v", err)
	}
	if !stopped {
		t.Fatal("Uninstall() did not stop the local broker")
	}
}

func TestSameHost(t *testing.T) {
	cases := []struct {
		target string
		host   string
		want   bool
	}{
		{"bob@example.com", "example.com", true},
		{"example.com", "example.com", true},
		{"bob@other.com", "example.com", false},
		{"", "example.com", false},
	}
	for _, tc := range cases {
		if got := sameHost(tc.target, tc.host); got != tc.want {
			t.Fatalf("sameHost(%q, %q) = %v, want %v", tc.target, tc.host, got, tc.want)
		}
	}
}
