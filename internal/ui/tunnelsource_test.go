package ui

import (
	"testing"

	"github.com/mcnull/sshex/internal/api"
	"github.com/mcnull/sshex/internal/model"
	"github.com/mcnull/sshex/internal/ui/screen/tunnels"
)

func TestTunnelFromResponse(t *testing.T) {
	response := api.TunnelResponse{
		ID:         "t1",
		SessionID:  "s1",
		LocalPort:  8080,
		TargetHost: "db",
		TargetPort: 5432,
		Direction:  model.ForwardRemote,
		State:      model.ForwardFailed,
		Error:      "boom",
	}

	got := tunnelFromResponse(response, "bob@ssh01", true)
	want := tunnels.Tunnel{
		ID:         "t1",
		SessionID:  "s1",
		Target:     "bob@ssh01",
		LocalPort:  8080,
		TargetHost: "db",
		TargetPort: 5432,
		Direction:  tunnels.DirectionRemote,
		State:      "failed",
		Error:      "boom",
		Stale:      true,
	}
	if got != want {
		t.Fatalf("tunnelFromResponse() = %+v, want %+v", got, want)
	}
}

func TestTunnelFromResponseDefaultsToLocal(t *testing.T) {
	got := tunnelFromResponse(api.TunnelResponse{ID: "t1"}, "", false)
	if got.Direction != tunnels.DirectionLocal {
		t.Fatalf("direction = %q, want local", got.Direction)
	}
}

func TestSessionLabel(t *testing.T) {
	cases := []struct {
		name    string
		session api.SessionResponse
		want    string
	}{
		{"user and host", api.SessionResponse{User: "bob", Host: "ssh01"}, "bob@ssh01"},
		{"host", api.SessionResponse{Host: "ssh01"}, "ssh01"},
		{"id fallback", api.SessionResponse{ID: "s1"}, "s1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sessionLabel(tc.session); got != tc.want {
				t.Fatalf("sessionLabel() = %q, want %q", got, tc.want)
			}
		})
	}
}
