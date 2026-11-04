package remoteinstall

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mcnull/sshex/internal/model"
	"github.com/mcnull/sshex/internal/sessionfile"
	"github.com/mcnull/sshex/internal/sshx"
)

type staticResolver string

func (s staticResolver) Resolve(context.Context, Platform) (string, error) { return string(s), nil }

func newTestInstaller(t *testing.T) *PosixInstaller {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "sshex")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return NewPosixInstaller(staticResolver(exe))
}

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
	conn := &fakeConn{stdout: map[string]string{pathsScript: "shell startup notice\n" + pathsBeginMarker + "\n/home/tester/.local/share/sshex\n/run/user/1000/sshex\n" + pathsEndMarker + "\nmore startup output\n"}}
	paths, err := newTestInstaller(t).RemotePaths(context.Background(), conn)
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

func TestRemotePathsReportsUnexpectedOutput(t *testing.T) {
	conn := &fakeConn{stdout: map[string]string{pathsScript: "warning: unexpected\n"}}
	_, err := newTestInstaller(t).RemotePaths(context.Background(), conn)
	if err == nil || !strings.Contains(err.Error(), `"warning: unexpected\n"`) {
		t.Fatalf("RemotePaths() error = %v, want diagnostic output", err)
	}
}

func TestInstallCopiesBinary(t *testing.T) {
	conn := &fakeConn{}
	paths := RemotePaths{DataDir: "/home/tester/.local/share/sshex", RunDir: "/run/user/1000/sshex"}
	if err := newTestInstaller(t).Install(context.Background(), conn, paths, Platform{OS: "linux", Arch: "amd64"}); err != nil {
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
	if err := newTestInstaller(t).WriteSession(context.Background(), conn, paths, file); err != nil {
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
	if err := newTestInstaller(t).Remove(context.Background(), conn, paths, sessionfile.File{ID: "abc"}); err != nil {
		t.Fatalf("Remove() error: %v", err)
	}
	if len(conn.commands) != 1 || !strings.Contains(conn.commands[0], "abc.json") || !strings.Contains(conn.commands[0], "abc.sock") {
		t.Fatalf("Remove() command = %v", conn.commands)
	}
}

func TestUninstall(t *testing.T) {
	conn := &fakeConn{}
	paths := RemotePaths{DataDir: "/home/tester/.local/share/sshex", RunDir: "/run/user/1000/sshex"}
	if err := newTestInstaller(t).Uninstall(context.Background(), conn, paths, UninstallOptions{}); err != nil {
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
		paths.DistDir(),
		paths.RunDir,
		paths.DataDir,
		paths.InstallDirRecordPath(),
	} {
		if !strings.Contains(cmd, want) {
			t.Fatalf("Uninstall() command missing %q:\n%s", want, cmd)
		}
	}
}

func TestUninstallRemoveConfig(t *testing.T) {
	conn := &fakeConn{}
	paths := RemotePaths{DataDir: "/home/tester/.local/share/sshex", RunDir: "/run/user/1000/sshex"}
	if err := newTestInstaller(t).Uninstall(context.Background(), conn, paths, UninstallOptions{RemoveConfig: true}); err != nil {
		t.Fatalf("Uninstall() error: %v", err)
	}
	if len(conn.commands) != 1 || !strings.Contains(conn.commands[0], `config_file="${SSHEX_CONFIG:-${XDG_CONFIG_HOME:-$HOME/.config}/sshex/config.yml}"`) {
		t.Fatalf("Uninstall() command missing config removal: %v", conn.commands)
	}
}

func TestUninstallRemovesRecordedInstallDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uninstallScript requires a POSIX shell")
	}

	home := t.TempDir()
	data := filepath.Join(home, ".local", "share", "sshex")
	localBin := filepath.Join(home, ".local", "bin")
	customBin := filepath.Join(home, "custom", "bin")
	runDir := filepath.Join(home, "run")
	for _, dir := range []string{filepath.Join(data, "bin"), localBin, customBin, runDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := []string{
		filepath.Join(customBin, "sshex"),
		filepath.Join(localBin, "sshex"),
		filepath.Join(data, "bin", "sshex"),
	}
	for _, file := range files {
		if err := os.WriteFile(file, []byte("sshex"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	paths := RemotePaths{DataDir: data, RunDir: runDir}
	if err := os.WriteFile(paths.InstallDirRecordPath(), []byte(customBin+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("sh", "-c", uninstallScript(paths, UninstallOptions{}))
	cmd.Env = environmentWithoutHome(home)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("uninstallScript() error: %v\n%s", err, out)
	}

	for _, file := range files {
		if _, err := os.Stat(file); !os.IsNotExist(err) {
			t.Fatalf("%s still exists after uninstall (err=%v)", file, err)
		}
	}
	if _, err := os.Stat(data); !os.IsNotExist(err) {
		t.Fatalf("%s still exists after uninstall (err=%v)", data, err)
	}
}

func TestUninstallScriptRemovesConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uninstallScript requires a POSIX shell")
	}

	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "sshex")
	configFile := filepath.Join(configDir, "config.yml")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configFile, []byte("server: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configFile+".lock", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	paths := RemotePaths{
		DataDir: filepath.Join(home, ".local", "share", "sshex"),
		RunDir:  filepath.Join(home, "run"),
	}

	cmd := exec.Command("sh", "-c", uninstallScript(paths, UninstallOptions{RemoveConfig: true}))
	cmd.Env = append(environmentWithoutHome(home), "SSHEX_CONFIG=", "XDG_CONFIG_HOME=")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("uninstallScript() error: %v\n%s", err, out)
	}
	if _, err := os.Stat(configFile); !os.IsNotExist(err) {
		t.Fatalf("%s still exists after uninstall (err=%v)", configFile, err)
	}
	if _, err := os.Stat(configDir); !os.IsNotExist(err) {
		t.Fatalf("%s still exists after uninstall (err=%v)", configDir, err)
	}
}

func TestUninstallScriptRemovesExplicitConfigOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uninstallScript requires a POSIX shell")
	}

	home := t.TempDir()
	configDir := filepath.Join(home, "custom")
	configFile := filepath.Join(configDir, "test.yml")
	sibling := filepath.Join(configDir, "keep.txt")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configFile, []byte("server: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sibling, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	paths := RemotePaths{
		DataDir: filepath.Join(home, ".local", "share", "sshex"),
		RunDir:  filepath.Join(home, "run"),
	}

	cmd := exec.Command("sh", "-c", uninstallScript(paths, UninstallOptions{RemoveConfig: true}))
	cmd.Env = append(environmentWithoutHome(home), "SSHEX_CONFIG="+configFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("uninstallScript() error: %v\n%s", err, out)
	}
	if _, err := os.Stat(configFile); !os.IsNotExist(err) {
		t.Fatalf("%s still exists after uninstall (err=%v)", configFile, err)
	}
	if _, err := os.Stat(sibling); err != nil {
		t.Fatalf("%s was removed: %v", sibling, err)
	}
}

func environmentWithoutHome(home string) []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "HOME=") {
			env = append(env, entry)
		}
	}
	return append(env, "HOME="+home)
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
