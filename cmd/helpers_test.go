package cmd

import (
	"testing"

	"github.com/mcnull/sshex/internal/model"
)

func TestSplitTarget(t *testing.T) {
	tests := []struct {
		target   string
		wantUser string
		wantHost string
	}{
		{target: "user@host", wantUser: "user", wantHost: "host"},
		{target: "host", wantUser: "", wantHost: "host"},
		{target: "a@b@c", wantUser: "a@b", wantHost: "c"},
	}

	for _, tt := range tests {
		user, host := splitTarget(tt.target)
		if user != tt.wantUser || host != tt.wantHost {
			t.Fatalf("splitTarget(%q) = (%q, %q), want (%q, %q)", tt.target, user, host, tt.wantUser, tt.wantHost)
		}
	}
}

func TestFormatTunnel(t *testing.T) {
	got := formatTunnel(model.ForwardLocal, 8192, "localhost", 8192, "user@remote")
	if got != "localhost:8192 > localhost:8192 user@remote" {
		t.Fatalf("formatTunnel() = %q", got)
	}
	if got := formatTunnel(model.ForwardLocal, 8192, "localhost", 8192, ""); got != "localhost:8192 > localhost:8192" {
		t.Fatalf("formatTunnel() without target = %q", got)
	}
	if got := formatTunnel(model.ForwardRemote, 8192, "localhost", 22, ""); got != "localhost:8192 < localhost:22" {
		t.Fatalf("formatTunnel() remote = %q", got)
	}
}
