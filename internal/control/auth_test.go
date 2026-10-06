package control

import (
	"encoding/base64"
	"net/http"
	"sync"
	"testing"

	"github.com/mcnull/sshex/internal/model"
)

func TestGenerateToken(t *testing.T) {
	token, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("token is not valid base64url: %v", err)
	}
	if len(raw) != tokenBytes {
		t.Fatalf("token decodes to %d bytes, want %d", len(raw), tokenBytes)
	}

	other, _ := GenerateToken()
	if token == other {
		t.Fatal("GenerateToken() returned duplicate tokens")
	}
}

func TestAuthorize(t *testing.T) {
	auth := NewAuthenticator()
	auth.Register("good-token", model.Capabilities{model.CapabilityTunnels}, "")

	tests := []struct {
		name       string
		header     string
		capability model.Capability
		wantErr    bool
	}{
		{name: "valid", header: "Bearer good-token", capability: model.CapabilityTunnels},
		{name: "case insensitive scheme", header: "bearer good-token", capability: model.CapabilityTunnels},
		{name: "missing", header: "", capability: model.CapabilityTunnels, wantErr: true},
		{name: "malformed scheme", header: "Token good-token", capability: model.CapabilityTunnels, wantErr: true},
		{name: "unknown token", header: "Bearer nope", capability: model.CapabilityTunnels, wantErr: true},
		{name: "wrong capability", header: "Bearer good-token", capability: model.CapabilityExec, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := http.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				r.Header.Set("Authorization", tt.header)
			}
			err := auth.Authorize(r, tt.capability)
			if tt.wantErr && err == nil {
				t.Fatal("Authorize() expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Authorize() unexpected error: %v", err)
			}
		})
	}
}

func TestAuthorizeAfterRevoke(t *testing.T) {
	auth := NewAuthenticator()
	auth.Register("token", model.Capabilities{model.CapabilityTunnels}, "")
	auth.Revoke("token")

	r, _ := http.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer token")
	if err := auth.Authorize(r, model.CapabilityTunnels); err == nil {
		t.Fatal("Authorize() expected error after revoke, got nil")
	}
}

func TestAuthorizeForSession(t *testing.T) {
	auth := NewAuthenticator()
	auth.Register("session-token", model.Capabilities{model.CapabilityTunnels}, "s1")
	auth.Register("admin-token", model.Capabilities{model.CapabilityTunnels}, "")

	newReq := func(token string) *http.Request {
		r, _ := http.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		return r
	}

	if err := auth.AuthorizeForSession(newReq("session-token"), model.CapabilityTunnels, "s1"); err != nil {
		t.Fatalf("AuthorizeForSession() own session error: %v", err)
	}
	if err := auth.AuthorizeForSession(newReq("session-token"), model.CapabilityTunnels, "s2"); err == nil {
		t.Fatal("AuthorizeForSession() expected error for other session")
	}
	if err := auth.AuthorizeForSession(newReq("admin-token"), model.CapabilityTunnels, "s2"); err != nil {
		t.Fatalf("AuthorizeForSession() admin token error: %v", err)
	}
}

func TestAuthenticatorConcurrent(t *testing.T) {
	auth := NewAuthenticator()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			auth.Register("shared", model.Capabilities{model.CapabilityTunnels}, "")
			r, _ := http.NewRequest(http.MethodGet, "/", nil)
			r.Header.Set("Authorization", "Bearer shared")
			_ = auth.Authorize(r, model.CapabilityTunnels)
			auth.Revoke("shared")
		}()
	}
	wg.Wait()
}
