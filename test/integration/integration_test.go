//go:build integration

package integration

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

const defaultHost = "ssh01"

func testHost(t *testing.T) string {
	t.Helper()
	host := os.Getenv("SSHEX_TEST_HOST")
	if host == "" {
		host = defaultHost
	}
	return host
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}

func buildBinary(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sshex")
	cmd := exec.Command("go", "build", "-o", path, ".")
	cmd.Dir = repoRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build sshex: %v\n%s", err, out)
	}
	return path
}

func runSSHEX(t *testing.T, binary, runtimeDir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(),
		"XDG_RUNTIME_DIR="+runtimeDir,
		"SSHEX_HOME="+filepath.Join(runtimeDir, "data"),
	)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestConnectAndTunnel(t *testing.T) {
	if os.Getenv("SSHEX_TEST_HOST") == "" {
		t.Skip("set SSHEX_TEST_HOST to run integration tests")
	}
	if _, err := exec.LookPath("script"); err != nil {
		t.Skip("`script` (util-linux) is required for the interactive flow")
	}

	host := testHost(t)
	binary := buildBinary(t)
	runtimeDir := t.TempDir()

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		time.Sleep(5 * time.Second)
		fmt.Fprintln(pw, "sshex tunnel add 2222:localhost:22")
		time.Sleep(2 * time.Second)
		fmt.Fprintln(pw, "sshex tunnel add 2222:localhost:22")
		time.Sleep(2 * time.Second)
		fmt.Fprintln(pw, "sshex tunnel list")
		time.Sleep(2 * time.Second)
		fmt.Fprintln(pw, "exit")
	}()

	cmd := exec.Command("script", "-qec", fmt.Sprintf("%s connect %s", binary, host), "/dev/null")
	cmd.Env = append(os.Environ(), "XDG_RUNTIME_DIR="+runtimeDir)
	cmd.Stdin = pr
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		t.Fatalf("connect flow failed: %v\n%s", err, out.String())
	}

	log := out.String()
	for _, want := range []string{
		"installing remote sshex command...",
		"tunnel created",
		"already in use",
		"localhost:2222 > localhost:22",
		"dropped tunnels:",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("connect log missing %q:\n%s", want, log)
		}
	}

	listOut, err := runSSHEX(t, binary, runtimeDir, "tunnel", "list")
	if err == nil || !strings.Contains(listOut, "no active sshex session") {
		t.Fatalf("expected no active session after exit, got err=%v out=%q", err, listOut)
	}

	if _, err := exec.LookPath("ssh"); err == nil {
		lsOut, lsErr := exec.Command("ssh", host, `ls -1 "${SSHEX_HOME:-${XDG_DATA_HOME:-$HOME/.local/share}/sshex}/completions" 2>/dev/null`).CombinedOutput()
		if lsErr != nil {
			t.Fatalf("list remote completions: %v\n%s", lsErr, lsOut)
		}
		found := false
		for _, name := range []string{"sshex.bash", "sshex.zsh", "sshex.fish"} {
			if strings.Contains(string(lsOut), name) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("no remote completion file installed:\n%s", lsOut)
		}
	}
}

type shell struct {
	cmd *exec.Cmd
	in  io.WriteCloser
	out *bytes.Buffer
}

func startShell(t *testing.T, binary, runtimeDir, host, command string, delay time.Duration) *shell {
	t.Helper()
	pr, pw := io.Pipe()
	cmd := exec.Command("script", "-qec", fmt.Sprintf("%s connect %s", binary, host), "/dev/null")
	cmd.Env = append(os.Environ(),
		"XDG_RUNTIME_DIR="+runtimeDir,
		"SSHEX_HOME="+filepath.Join(runtimeDir, "data"),
	)
	cmd.Stdin = pr
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start shell: %v", err)
	}
	if command != "" {
		go func() {
			time.Sleep(delay)
			fmt.Fprintln(pw, command)
		}()
	}
	return &shell{cmd: cmd, in: pw, out: &out}
}

func (s *shell) send(command string) {
	fmt.Fprintln(s.in, command)
}

func (s *shell) stop(t *testing.T) {
	t.Helper()
	fmt.Fprintln(s.in, "exit")
	_ = s.in.Close()
	if err := s.cmd.Wait(); err != nil {
		t.Fatalf("shell exited with error: %v\n%s", err, s.out.String())
	}
}

// kill abruptly terminates the whole shell process group, including the sshex
// connect process, so no detach is sent to the broker.
func (s *shell) kill(t *testing.T) {
	t.Helper()
	if s.cmd.Process == nil {
		return
	}
	pgid, err := syscall.Getpgid(s.cmd.Process.Pid)
	if err == nil {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	} else {
		_ = s.cmd.Process.Kill()
	}
	_ = s.in.Close()
	_ = s.cmd.Wait()
}

func waitForSessionCount(t *testing.T, runtimeDir string, want int) {
	t.Helper()
	dir := filepath.Join(runtimeDir, "sshex", "sessions")
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		entries, _ := os.ReadDir(dir)
		n := 0
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".json") {
				n++
			}
		}
		if n == want {
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d session file(s) in %s", want, dir)
}

func waitForTunnel(t *testing.T, binary, runtimeDir, want string) string {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		out, err := runSSHEX(t, binary, runtimeDir, "tunnel", "list", "--all")
		if err == nil && strings.Contains(out, want) {
			return out
		}
		last = out
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for tunnel %q; last output:\n%s", want, last)
	return ""
}

func firstSessionID(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, " ") || strings.TrimSpace(line) == "" {
			continue
		}
		if fields := strings.Fields(line); len(fields) > 0 {
			return fields[0]
		}
	}
	return ""
}

func TestMultipleSessionsSharing(t *testing.T) {
	if os.Getenv("SSHEX_TEST_HOST") == "" {
		t.Skip("set SSHEX_TEST_HOST to run integration tests")
	}
	if _, err := exec.LookPath("script"); err != nil {
		t.Skip("`script` (util-linux) is required for the interactive flow")
	}

	host := testHost(t)
	binary := buildBinary(t)
	runtimeDir := t.TempDir()

	a := startShell(t, binary, runtimeDir, host, "sshex tunnel add 3333:localhost:22", 8*time.Second)
	waitForSessionCount(t, runtimeDir, 1)

	b := startShell(t, binary, runtimeDir, host, "", 0)
	waitForSessionCount(t, runtimeDir, 1)

	all := waitForTunnel(t, binary, runtimeDir, "localhost:3333 > localhost:22")
	sessionA := firstSessionID(all)
	if sessionA == "" {
		t.Fatalf("could not determine session id from:\n%s", all)
	}

	// Both shells share one session, so a plain `tunnel list` resolves it too.
	plain, err := runSSHEX(t, binary, runtimeDir, "tunnel", "list")
	if err != nil || !strings.Contains(plain, "localhost:3333 > localhost:22") {
		t.Fatalf("plain tunnel list did not show the shared tunnel: err=%v out=%q", err, plain)
	}

	// A sibling connection can see and remove the tunnel.
	sessionTunnels, err := runSSHEX(t, binary, runtimeDir, "tunnel", "list", "--session", sessionA)
	if err != nil || !strings.Contains(sessionTunnels, "localhost:3333 > localhost:22") {
		t.Fatalf("tunnel list --session %s: err=%v out=%q", sessionA, err, sessionTunnels)
	}
	if out, err := runSSHEX(t, binary, runtimeDir, "tunnel", "rm", "--session", sessionA, "3333"); err != nil || !strings.Contains(out, "tunnel removed") {
		t.Fatalf("tunnel rm --session %s: err=%v out=%q", sessionA, err, out)
	}
	if out, err := runSSHEX(t, binary, runtimeDir, "tunnel", "list", "--all"); err != nil || strings.Contains(out, "localhost:3333") {
		t.Fatalf("tunnel still listed after cross-session rm: err=%v out=%q", err, out)
	}

	// Recreate it and close A while B is still connected: the tunnel must survive.
	a.send("sshex tunnel add 3333:localhost:22")
	waitForTunnel(t, binary, runtimeDir, "localhost:3333 > localhost:22")
	a.stop(t)

	survived, err := runSSHEX(t, binary, runtimeDir, "tunnel", "list", "--all")
	if err != nil || !strings.Contains(survived, "localhost:3333 > localhost:22") {
		t.Fatalf("tunnel did not survive session A exit while B was attached: err=%v out=%q", err, survived)
	}

	// Closing the last shell tears everything down.
	b.stop(t)
	waitForSessionCount(t, runtimeDir, 0)
	if out, err := runSSHEX(t, binary, runtimeDir, "tunnel", "list"); err == nil || !strings.Contains(out, "no active sshex session") {
		t.Fatalf("expected no active session after last exit: err=%v out=%q", err, out)
	}
}

func TestAbruptDisconnectReaped(t *testing.T) {
	if os.Getenv("SSHEX_TEST_HOST") == "" {
		t.Skip("set SSHEX_TEST_HOST to run integration tests")
	}
	if _, err := exec.LookPath("script"); err != nil {
		t.Skip("`script` (util-linux) is required for the interactive flow")
	}

	host := testHost(t)
	binary := buildBinary(t)
	runtimeDir := t.TempDir()

	configDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(configDir, "sshex"), 0o700); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	data := "server:\n  heartbeat_interval: 1s\n  heartbeat_timeout: 3s\n"
	if err := os.WriteFile(filepath.Join(configDir, "sshex", "config.yml"), []byte(data), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", configDir)

	sh := startShell(t, binary, runtimeDir, host, "", 0)
	waitForSessionCount(t, runtimeDir, 1)

	// Kill the shell without a clean exit so no detach reaches the broker. The
	// heartbeat monitor must reap the stale connection and destroy the session.
	sh.kill(t)
	waitForSessionCount(t, runtimeDir, 0)
}
