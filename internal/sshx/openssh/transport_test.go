package openssh

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mcnull/sshex/internal/sshx"
)

func TestTargetArgs(t *testing.T) {
	tests := []struct {
		name   string
		target sshx.Target
		want   []string
	}{
		{
			name:   "empty",
			target: sshx.Target{Host: "ssh02"},
			want:   nil,
		},
		{
			name:   "jump hosts",
			target: sshx.Target{Host: "ssh03", JumpHosts: "ssh01,ssh02"},
			want:   []string{"-J", "ssh01,ssh02"},
		},
		{
			name:   "full target",
			target: sshx.Target{Host: "ssh02", Port: 2222, IdentityFile: "/key", JumpHosts: "ssh01", Options: []string{"StrictHostKeyChecking=no"}},
			want:   []string{"-p", "2222", "-i", "/key", "-J", "ssh01", "-o", "StrictHostKeyChecking=no"},
		},
		{
			name:   "raw args",
			target: sshx.Target{Host: "ssh02", Options: []string{"-J nulls-probe"}},
			want:   []string{"-J", "nulls-probe"},
		},
		{
			name:   "options and raw args",
			target: sshx.Target{Host: "ssh02", Options: []string{"StrictHostKeyChecking=no", "-o ConnectTimeout=5 -J bastion"}},
			want:   []string{"-o", "StrictHostKeyChecking=no", "-o", "ConnectTimeout=5", "-J", "bastion"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := targetArgs(tc.target)
			if err != nil {
				t.Fatalf("targetArgs(%+v) error: %v", tc.target, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("targetArgs(%+v) = %v; want %v", tc.target, got, tc.want)
			}
		})
	}
}

func TestTargetArgsRejectsUnterminatedQuote(t *testing.T) {
	if _, err := targetArgs(sshx.Target{Host: "ssh02", Options: []string{"-J 'nulls-probe"}}); err == nil {
		t.Fatal("targetArgs() error = nil, want invalid -o value")
	}
}

func TestShellCommand(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		want      string
	}{
		{
			name:      "empty session id",
			sessionID: "",
			want:      "",
		},
		{
			name:      "hex session id",
			sessionID: "0123456789abcdef",
			want:      `exec env SSHEX_SESSION='0123456789abcdef' "${SHELL:-/bin/sh}" -l`,
		},
		{
			name:      "session id is shell quoted",
			sessionID: "a'b",
			want:      `exec env SSHEX_SESSION='a'\''b' "${SHELL:-/bin/sh}" -l`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shellCommand(tc.sessionID); got != tc.want {
				t.Fatalf("shellCommand(%q) = %q; want %q", tc.sessionID, got, tc.want)
			}
		})
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "", want: "''"},
		{in: "plain", want: "'plain'"},
		{in: "a'b", want: `'a'\''b'`},
	}
	for _, tc := range tests {
		if got := shellQuote(tc.in); got != tc.want {
			t.Fatalf("shellQuote(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

// TestConnectReturnsWhenHelperHoldsStderr reproduces the `-f` + `-J` hang: the
// fake ssh daemonizes a helper that inherits stderr and outlives the parent, as
// OpenSSH's ProxyJump helper does. Capturing output through a pipe would block
// until the helper exits; Connect must return promptly.
func TestConnectReturnsWhenHelperHoldsStderr(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "helper.pid")
	script := filepath.Join(dir, "ssh")
	contents := "#!/bin/sh\nsleep 30 &\necho $! > " + pidFile + "\nexit 0\n"
	if err := os.WriteFile(script, []byte(contents), 0o755); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}
	t.Cleanup(func() {
		data, err := os.ReadFile(pidFile)
		if err != nil {
			return
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			return
		}
		if proc, err := os.FindProcess(pid); err == nil {
			_ = proc.Kill()
		}
	})

	transport := &Transport{sshPath: script}
	done := make(chan error, 1)
	go func() {
		_, err := transport.Connect(context.Background(), sshx.Target{Host: "ssh02", JumpHosts: "ssh01"}, sshx.ConnectOptions{
			ControlPath: filepath.Join(dir, "test.ctl"),
		})
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Connect() error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Connect() blocked while a daemonized helper held stderr")
	}
}

func TestAdoptChecksExistingMaster(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ssh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}

	controlPath := filepath.Join(dir, "master.ctl")
	transport := &Transport{sshPath: script}
	conn, err := transport.Adopt(context.Background(), sshx.Target{Host: "ssh02"}, sshx.ConnectOptions{ControlPath: controlPath})
	if err != nil {
		t.Fatalf("Adopt() error: %v", err)
	}
	if conn.ControlPath() != controlPath {
		t.Fatalf("Adopt() control path = %q, want %q", conn.ControlPath(), controlPath)
	}
}

func TestAdoptRejectsDeadMaster(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ssh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho 'Control socket connect: No such file or directory' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}

	transport := &Transport{sshPath: script}
	_, err := transport.Adopt(context.Background(), sshx.Target{Host: "ssh02"}, sshx.ConnectOptions{ControlPath: filepath.Join(dir, "master.ctl")})
	if err == nil {
		t.Fatal("Adopt() error = nil, want failure for a dead master")
	}
	if !strings.Contains(err.Error(), "adopt ssh master") {
		t.Fatalf("Adopt() error = %v, want adopt ssh master", err)
	}
}

// TestConnectInteractiveWiresTerminal verifies that an interactive caller's
// terminal file receives the master's stdio, so OpenSSH prompts are visible.
func TestConnectInteractiveWiresTerminal(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ssh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho 'Are you sure you want to continue connecting?' >&2\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}

	term, err := os.CreateTemp(dir, "term")
	if err != nil {
		t.Fatalf("create terminal file: %v", err)
	}
	defer term.Close()

	transport := &Transport{sshPath: script}
	if _, err := transport.Connect(context.Background(), sshx.Target{Host: "ssh02"}, sshx.ConnectOptions{
		ControlPath: filepath.Join(dir, "master.ctl"),
		Stderr:      term,
	}); err != nil {
		t.Fatalf("Connect() error: %v", err)
	}

	if _, err := term.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("seek terminal file: %v", err)
	}
	data, err := io.ReadAll(term)
	if err != nil {
		t.Fatalf("read terminal file: %v", err)
	}
	if !strings.Contains(string(data), "continue connecting") {
		t.Fatalf("terminal did not receive ssh output: %q", data)
	}
}
