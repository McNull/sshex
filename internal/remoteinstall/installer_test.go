package remoteinstall

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/mcnull/sshex/internal/model"
	"github.com/mcnull/sshex/internal/sessionfile"
	"github.com/mcnull/sshex/internal/sshx"
)

type fakeConn struct {
	commands []string
	stdin    [][]byte
	stdout   map[string]string
}

func (f *fakeConn) ControlPath() string { return "" }
func (f *fakeConn) Target() sshx.Target { return sshx.Target{} }

func (f *fakeConn) Run(_ context.Context, command string, stdin io.Reader, stdout, _ io.Writer) error {
	f.commands = append(f.commands, command)
	if stdin != nil {
		data, _ := io.ReadAll(stdin)
		f.stdin = append(f.stdin, data)
	}
	if stdout != nil && f.stdout != nil {
		io.WriteString(stdout, f.stdout[command])
	}
	return nil
}

func (f *fakeConn) AddLocalForward(context.Context, sshx.ForwardSpec) error    { return nil }
func (f *fakeConn) RemoveLocalForward(context.Context, sshx.ForwardSpec) error { return nil }
func (f *fakeConn) AddRemoteForward(context.Context, sshx.ForwardSpec) error   { return nil }
func (f *fakeConn) RemoveRemoteForward(context.Context, sshx.ForwardSpec) error {
	return nil
}
func (f *fakeConn) AddRemoteSocketForward(context.Context, sshx.RemoteSocketForwardSpec) error {
	return nil
}
func (f *fakeConn) RemoveRemoteSocketForward(context.Context, sshx.RemoteSocketForwardSpec) error {
	return nil
}
func (f *fakeConn) Shell(context.Context, io.Reader, io.Writer, io.Writer) error { return nil }
func (f *fakeConn) Close(context.Context) error                                  { return nil }

func TestRemotePaths(t *testing.T) {
	conn := &fakeConn{stdout: map[string]string{pathsScript: "/home/tester/.local/share/sshex\n/run/user/1000/sshex\n"}}
	paths, err := NewLinuxInstaller().RemotePaths(context.Background(), conn)
	if err != nil {
		t.Fatalf("RemotePaths() error: %v", err)
	}
	if paths.DataDir != "/home/tester/.local/share/sshex" {
		t.Fatalf("RemotePaths().DataDir = %q, want /home/tester/.local/share/sshex", paths.DataDir)
	}
	if paths.RunDir != "/run/user/1000/sshex" {
		t.Fatalf("RemotePaths().RunDir = %q, want /run/user/1000/sshex", paths.RunDir)
	}
}

func TestInstallCopiesBinary(t *testing.T) {
	conn := &fakeConn{}
	paths := RemotePaths{DataDir: "/home/tester/.local/share/sshex", RunDir: "/run/user/1000/sshex"}
	if err := NewLinuxInstaller().Install(context.Background(), conn, paths); err != nil {
		t.Fatalf("Install() error: %v", err)
	}
	if len(conn.commands) != 1 || !strings.Contains(conn.commands[0], paths.BinPath()) {
		t.Fatalf("Install() command = %v", conn.commands)
	}
	if len(conn.stdin) != 1 || len(conn.stdin[0]) == 0 {
		t.Fatalf("Install() did not stream the binary")
	}
}

func TestWriteSession(t *testing.T) {
	conn := &fakeConn{}
	paths := RemotePaths{DataDir: "/home/tester/.local/share/sshex", RunDir: "/run/user/1000/sshex"}
	file := sessionfile.File{
		ID:       "abc",
		Endpoint: model.Endpoint{Kind: model.EndpointUnix, Address: "/run/user/1000/sshex/abc.sock"},
		Token:    "secret",
	}
	if err := NewLinuxInstaller().WriteSession(context.Background(), conn, paths, file); err != nil {
		t.Fatalf("WriteSession() error: %v", err)
	}
	if len(conn.stdin) != 1 {
		t.Fatalf("WriteSession() stdin calls = %d, want 1", len(conn.stdin))
	}
	body := string(conn.stdin[0])
	for _, want := range []string{`"id": "abc"`, `"token": "secret"`, "abc.sock"} {
		if !strings.Contains(body, want) {
			t.Fatalf("session body %q missing %q", body, want)
		}
	}
}

func TestRemove(t *testing.T) {
	conn := &fakeConn{}
	paths := RemotePaths{DataDir: "/home/tester/.local/share/sshex", RunDir: "/run/user/1000/sshex"}
	if err := NewLinuxInstaller().Remove(context.Background(), conn, paths, sessionfile.File{ID: "abc"}); err != nil {
		t.Fatalf("Remove() error: %v", err)
	}
	if len(conn.commands) != 1 || !strings.Contains(conn.commands[0], "abc.json") || !strings.Contains(conn.commands[0], "abc.sock") {
		t.Fatalf("Remove() command = %v", conn.commands)
	}
}

func TestUninstall(t *testing.T) {
	conn := &fakeConn{}
	paths := RemotePaths{DataDir: "/home/tester/.local/share/sshex", RunDir: "/run/user/1000/sshex"}
	if err := NewLinuxInstaller().Uninstall(context.Background(), conn, paths); err != nil {
		t.Fatalf("Uninstall() error: %v", err)
	}
	if len(conn.commands) != 1 {
		t.Fatalf("Uninstall() ran %d commands, want 1", len(conn.commands))
	}
	cmd := conn.commands[0]
	for _, want := range []string{
		`$HOME/.local/bin/sshex`,
		paths.BinDir(),
		paths.SessionsDir(),
		paths.CompletionsDir(),
		paths.RunDir,
		paths.DataDir,
	} {
		if !strings.Contains(cmd, want) {
			t.Fatalf("Uninstall() command missing %q:\n%s", want, cmd)
		}
	}
}

func TestRemoteSocketPath(t *testing.T) {
	got := RemoteSocketPath("/run/user/1000/sshex", "abc")
	if got != "/run/user/1000/sshex/abc.sock" {
		t.Fatalf("RemoteSocketPath() = %q", got)
	}
}

func TestExecutable(t *testing.T) {
	path, err := Executable()
	if err != nil {
		t.Fatalf("Executable() error: %v", err)
	}
	if path == "" {
		t.Fatal("Executable() returned empty path")
	}
}
