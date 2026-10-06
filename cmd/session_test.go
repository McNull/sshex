package cmd

import (
	"bytes"
	"context"
	"net"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcnull/sshex/internal/api"
	"github.com/mcnull/sshex/internal/model"
	"github.com/mcnull/sshex/internal/sessionfile"
	"github.com/mcnull/sshex/internal/sshx"
	"github.com/mcnull/sshex/internal/sshx/openssh"
)

func liveSession(t *testing.T, runtimeDir, id, target string) {
	t.Helper()
	sock := filepath.Join(runtimeDir, id+".sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	dir, err := sessionfile.LocalDir()
	if err != nil {
		t.Fatalf("LocalDir() error: %v", err)
	}
	f := sessionfile.File{
		ID:           id,
		Endpoint:     model.Endpoint{Kind: model.EndpointUnix, Address: sock},
		Token:        "t",
		Capabilities: model.Capabilities{model.CapabilityTunnels},
		Target:       target,
	}
	if err := sessionfile.WriteLocal(dir, f); err != nil {
		t.Fatalf("WriteLocal() error: %v", err)
	}
}

func TestSelectClientResolution(t *testing.T) {
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	t.Setenv("SSHEX_HOME", t.TempDir())
	t.Setenv("SSHEX_SESSION", "")
	liveSession(t, runtimeDir, "s1", "bob@ssh01")

	c, err := selectClient("")
	if err != nil {
		t.Fatalf("selectClient(\"\") error: %v", err)
	}
	if c.SessionID() != "s1" {
		t.Fatalf("selectClient(\"\") id = %q, want s1", c.SessionID())
	}

	c, err = selectClient("other")
	if err != nil {
		t.Fatalf("selectClient(other) error: %v", err)
	}
	if c.SessionID() != "other" {
		t.Fatalf("selectClient(other) id = %q, want other", c.SessionID())
	}

	liveSession(t, runtimeDir, "s2", "bob@ssh01")
	if _, err := selectClient(""); err == nil {
		t.Fatal("selectClient(\"\") with two sessions error = nil, want ambiguity error")
	}
	c, err = selectClient("s2")
	if err != nil {
		t.Fatalf("selectClient(s2) error: %v", err)
	}
	if c.SessionID() != "s2" {
		t.Fatalf("selectClient(s2) id = %q, want s2", c.SessionID())
	}
}

func TestRenderSessions(t *testing.T) {
	var out bytes.Buffer
	renderSessions(&out, []api.SessionResponse{
		{ID: "s1", User: "bob", Host: "ssh01", Origin: "laptop", State: model.SessionActive, Connections: 1},
		{ID: "s2", Host: "ssh02", State: model.SessionActive},
		{ID: "s3", Host: "ssh03", State: model.SessionActive, JumpHosts: "ssh01,ssh02"},
		{ID: "s4", Host: "ssh04", State: model.SessionActive},
	}, "s1", []string{"0", "?", "2", "0"})

	got := out.String()
	for _, want := range []string{"ID", "TARGET", "ORIGIN", "STATE", "CONNECTIONS", "HOPS", "STALE", "bob@ssh01", "laptop", "s1 *", "ssh02"} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderSessions() output missing %q:\n%s", want, got)
		}
	}
	// "?" only ever comes from a session's HOPS value.
	if !strings.Contains(got, "?") {
		t.Fatalf("renderSessions() output missing unknown hop marker:\n%s", got)
	}
}

func TestHopCount(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"ssh01", 1},
		{"ssh01,ssh02", 2},
		{"ssh01, ssh02", 2},
		{",,", 0},
	}
	for _, tc := range tests {
		if got := hopCount(tc.in); got != tc.want {
			t.Fatalf("hopCount(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestSessionHopsLabelUsesJumpHosts(t *testing.T) {
	cases := []struct {
		jumpHosts string
		want      string
	}{
		{"ssh01", "1"},
		{"ssh01,ssh02", "2"},
	}
	for _, tc := range cases {
		s := api.SessionResponse{Host: "ssh03", JumpHosts: tc.jumpHosts}
		if got := sessionHopsLabel(context.Background(), s); got != tc.want {
			t.Fatalf("sessionHopsLabel(%q) = %q, want %q", tc.jumpHosts, got, tc.want)
		}
	}
}

func TestSessionHopsLabelResolvesFromConfig(t *testing.T) {
	orig := resolveProxy
	t.Cleanup(func() { resolveProxy = orig })

	cases := []struct {
		name string
		info openssh.ProxyInfo
		err  error
		want string
	}{
		{"direct", openssh.ProxyInfo{}, nil, "0"},
		{"config jump", openssh.ProxyInfo{JumpHosts: "ssh01,ssh02"}, nil, "2"},
		{"proxy command", openssh.ProxyInfo{ProxyCommand: true}, nil, "?"},
		{"resolve error", openssh.ProxyInfo{}, context.DeadlineExceeded, "?"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolveProxy = func(context.Context, sshx.Target) (openssh.ProxyInfo, error) {
				return tc.info, tc.err
			}
			if got := sessionHopsLabel(context.Background(), api.SessionResponse{Host: "ssh03"}); got != tc.want {
				t.Fatalf("sessionHopsLabel() = %q, want %q", got, tc.want)
			}
		})
	}
}
