package control

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/mcnull/sshex/internal/model"
)

const tokenBytes = 32

func GenerateToken() (string, error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

type Credential struct {
	Token        string
	Capabilities model.Capabilities
	SessionID    string
}

type Authenticator struct {
	mu          sync.RWMutex
	credentials map[string]Credential
}

func NewAuthenticator() *Authenticator {
	return &Authenticator{credentials: make(map[string]Credential)}
}

func (a *Authenticator) Register(token string, capabilities model.Capabilities, sessionID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.credentials[token] = Credential{Token: token, Capabilities: capabilities, SessionID: sessionID}
}

func (a *Authenticator) Revoke(token string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.credentials, token)
}

func (a *Authenticator) Authorize(r *http.Request, required model.Capability) error {
	_, err := a.credential(r, required, "")
	return err
}

func (a *Authenticator) AuthorizeForSession(r *http.Request, required model.Capability, sessionID string) error {
	_, err := a.credential(r, required, sessionID)
	return err
}

func (a *Authenticator) credential(r *http.Request, required model.Capability, sessionID string) (Credential, error) {
	token := bearerToken(r)
	if token == "" {
		return Credential{}, errors.New("missing bearer token")
	}
	a.mu.RLock()
	credential, ok := a.credentials[token]
	a.mu.RUnlock()
	if !ok {
		return Credential{}, errors.New("unknown token")
	}
	if subtle.ConstantTimeCompare([]byte(credential.Token), []byte(token)) != 1 {
		return Credential{}, errors.New("invalid token")
	}
	if !credential.Capabilities.Contains(required) {
		return Credential{}, errors.New("insufficient capability")
	}
	if sessionID != "" && credential.SessionID != "" && credential.SessionID != sessionID {
		return Credential{}, errors.New("token not valid for session")
	}
	return credential, nil
}

func bearerToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}
