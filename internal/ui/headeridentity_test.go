package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcnull/sshex/internal/sessionfile"
)

func TestHeaderIdentityUsesCurrentSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	data, err := json.Marshal(sessionfile.File{
		ID:     "session-id",
		Origin: "broker-host",
		Target: "alice@ssh01",
	})
	if err != nil {
		t.Fatalf("marshal session file: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write session file: %v", err)
	}
	t.Setenv("SSHEX_SESSION", path)

	origin, client := headerIdentity("", "")
	if origin != "broker-host" || client != "alice@ssh01" {
		t.Fatalf("headerIdentity() = (%q, %q), want (%q, %q)", origin, client, "broker-host", "alice@ssh01")
	}
}

func TestHeaderIdentityAllowsOverrides(t *testing.T) {
	t.Setenv("SSHEX_SESSION", "")
	origin, client := headerIdentity("configured-origin", "configured-client")
	if origin != "configured-origin" || client != "configured-client" {
		t.Fatalf("headerIdentity() = (%q, %q), want configured values", origin, client)
	}
}
