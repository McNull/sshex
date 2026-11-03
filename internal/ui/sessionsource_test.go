package ui

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcnull/sshex/internal/model"
	"github.com/mcnull/sshex/internal/sessionfile"
)

func TestControlSessionFileScopes(t *testing.T) {
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	t.Setenv("SSHEX_HOME", t.TempDir())

	sockDir, err := os.MkdirTemp("/tmp", "sshex-")
	if err != nil {
		t.Fatalf("create socket dir: %v", err)
	}
	defer os.RemoveAll(sockDir)

	localSock := filepath.Join(sockDir, "local.sock")
	localListener, err := net.Listen("unix", localSock)
	if err != nil {
		t.Fatalf("listen local: %v", err)
	}
	defer localListener.Close()
	remoteSock := filepath.Join(sockDir, "remote.sock")
	remoteListener, err := net.Listen("unix", remoteSock)
	if err != nil {
		t.Fatalf("listen remote: %v", err)
	}
	defer remoteListener.Close()

	localDir, err := sessionfile.LocalDir()
	if err != nil {
		t.Fatalf("LocalDir() error: %v", err)
	}
	remoteDir, err := sessionfile.RemoteDir()
	if err != nil {
		t.Fatalf("RemoteDir() error: %v", err)
	}
	if err := sessionfile.WriteLocal(localDir, sessionfile.File{ID: "local", Endpoint: model.Endpoint{Kind: model.EndpointUnix, Address: localSock}}); err != nil {
		t.Fatalf("write local session: %v", err)
	}
	if err := sessionfile.WriteLocal(remoteDir, sessionfile.File{ID: "remote", Endpoint: model.Endpoint{Kind: model.EndpointUnix, Address: remoteSock}}); err != nil {
		t.Fatalf("write remote session: %v", err)
	}
	t.Setenv("SSHEX_SESSION", "remote")

	origin, err := controlSessionFile(scopeOrigin)
	if err != nil {
		t.Fatalf("controlSessionFile(origin) error: %v", err)
	}
	if origin.ID != "local" {
		t.Fatalf("origin session = %q, want local", origin.ID)
	}

	client, err := controlSessionFile(scopeClient)
	if err != nil {
		t.Fatalf("controlSessionFile(client) error: %v", err)
	}
	if client.ID != "remote" {
		t.Fatalf("client session = %q, want remote", client.ID)
	}
}

func TestCurrentRemoteSessionID(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{"unset", "", ""},
		{"id", "3a2a58e442b070f8", "3a2a58e442b070f8"},
		{"path", "/run/user/1000/sshex/sessions/3a2a58e442b070f8.json", "3a2a58e442b070f8"},
		{"spaces", "  abc  ", "abc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SSHEX_SESSION", tc.value)
			if got := currentRemoteSessionID(); got != tc.want {
				t.Fatalf("currentRemoteSessionID() = %q, want %q", got, tc.want)
			}
		})
	}
}
