package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mcnull/sshex/internal/api"
	"github.com/mcnull/sshex/internal/model"
	"github.com/spf13/cobra"
)

func newDirectionCmd(args ...string) *cobra.Command {
	cmd := &cobra.Command{Use: "add"}
	cmd.Flags().BoolP("local", "L", false, "")
	cmd.Flags().BoolP("remote", "R", false, "")
	_ = cmd.ParseFlags(args)
	return cmd
}

func TestShouldListAllTunnels(t *testing.T) {
	tests := []struct {
		name            string
		all             bool
		sessionID       string
		remoteSessionID string
		want            bool
	}{
		{name: "origin defaults to all", want: true},
		{name: "remote defaults to current session", remoteSessionID: "s1", want: false},
		{name: "explicit session on origin", sessionID: "s2", want: false},
		{name: "explicit session remotely", sessionID: "s2", remoteSessionID: "s1", want: false},
		{name: "all overrides explicit session", all: true, sessionID: "s2", want: true},
		{name: "all overrides remote default", all: true, remoteSessionID: "s1", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldListAllTunnels(tt.all, tt.sessionID, tt.remoteSessionID); got != tt.want {
				t.Fatalf("shouldListAllTunnels(%t, %q, %q) = %t, want %t", tt.all, tt.sessionID, tt.remoteSessionID, got, tt.want)
			}
		})
	}
}

func TestRenderTunnelList(t *testing.T) {
	var out bytes.Buffer
	renderTunnelList(&out, []tunnelListRow{
		{id: "t1", sessionID: "c2f25eb85dba1514", target: "bob@ssh01", tunnel: "localhost:12345 > localhost:12345"},
		{id: "t2", sessionID: "5f5c5edfff8c814f", target: "ssh02", tunnel: "localhost:8888 > localhost:8888"},
	})

	got := out.String()
	for _, want := range []string{
		"ID  SESSION           TARGET     TUNNEL",
		"t1  c2f25eb85dba1514  bob@ssh01  localhost:12345 > localhost:12345",
		"t2  5f5c5edfff8c814f  ssh02      localhost:8888 > localhost:8888",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderTunnelList() output missing %q:\n%s", want, got)
		}
	}
}

func TestTunnelIDCompletions(t *testing.T) {
	got := tunnelIDCompletions(
		[]api.TunnelResponse{{
			ID: "tunnel-1", SessionID: "session-1", LocalPort: 8080,
			TargetHost: "localhost", TargetPort: 80, Direction: model.ForwardLocal,
		}},
		[]api.SessionResponse{{ID: "session-1", User: "bob", Host: "ssh01"}},
	)
	if len(got) != 1 {
		t.Fatalf("tunnelIDCompletions() returned %d results, want 1", len(got))
	}
	if string(got[0]) != "tunnel-1\tsession-1 (bob@ssh01) localhost:8080 > localhost:80" {
		t.Fatalf("tunnelIDCompletions() = %q", got[0])
	}
}

func TestTunnelDirection(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		fallback model.ForwardDirection
		want     model.ForwardDirection
		wantErr  bool
	}{
		{name: "default", fallback: model.ForwardLocal, want: model.ForwardLocal},
		{name: "local flag", args: []string{"-L"}, fallback: model.ForwardLocal, want: model.ForwardLocal},
		{name: "remote flag", args: []string{"-R"}, fallback: model.ForwardLocal, want: model.ForwardRemote},
		{name: "rm default", fallback: "", want: ""},
		{name: "both flags", args: []string{"-L", "-R"}, fallback: model.ForwardLocal, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tunnelDirection(newDirectionCmd(tt.args...), tt.fallback)
			if tt.wantErr {
				if err == nil {
					t.Fatal("tunnelDirection() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("tunnelDirection() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("tunnelDirection() = %q, want %q", got, tt.want)
			}
		})
	}
}
