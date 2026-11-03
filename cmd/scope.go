package cmd

import (
	"errors"
	"fmt"

	"github.com/mcnull/sshex/internal/client"
	"github.com/mcnull/sshex/internal/sessionfile"
	"github.com/spf13/cobra"
)

// controlScope selects the origin a command manages. Origin is this machine;
// Client is the origin of the current outer session.
type controlScope int

const (
	scopeOrigin controlScope = iota
	scopeClient
)

// scopeFromFlags reads the persistent --client flag.
func scopeFromFlags(cmd *cobra.Command) controlScope {
	client, _ := cmd.Flags().GetBool("client")
	if client {
		return scopeClient
	}
	return scopeOrigin
}

// sessionFile returns a live broker session file for the scope. Client scope is
// strictly the current SSHEX_SESSION. Origin scope scans only this machine's
// runtime directory and falls back to the first active local session when more
// than one is reachable.
func (s controlScope) sessionFile() (sessionfile.File, error) {
	if s == scopeClient {
		file, err := sessionfile.Current()
		if errors.Is(err, sessionfile.ErrNone) {
			return sessionfile.File{}, fmt.Errorf("--client requires an active sshex session")
		}
		return file, err
	}

	file, err := sessionfile.DiscoverLocal()
	if err == nil {
		return file, nil
	}
	if errors.Is(err, sessionfile.ErrAmbiguous) {
		active, listErr := sessionfile.ListActiveLocal()
		if listErr == nil && len(active) > 0 {
			return active[0], nil
		}
	}
	return file, err
}

func (s controlScope) client() (*client.Client, error) {
	file, err := s.sessionFile()
	if err != nil {
		return nil, err
	}
	return client.New(file), nil
}

func (s controlScope) sessionClient(sessionID string) (*client.Client, error) {
	c, err := s.client()
	if err != nil {
		return nil, err
	}
	if sessionID == "" {
		return c, nil
	}
	return c.ForSession(sessionID), nil
}

// mutationClient is sessionClient with explicit-session guidance: a tunnel
// mutation must not silently pick one of several local origin sessions.
func (s controlScope) mutationClient(sessionID string) (*client.Client, error) {
	if s == scopeOrigin && sessionID == "" {
		if _, err := sessionfile.DiscoverLocal(); errors.Is(err, sessionfile.ErrAmbiguous) {
			return nil, fmt.Errorf("multiple active sshex sessions; specify --session <id> (see `sshex session list`)")
		}
	}
	return s.sessionClient(sessionID)
}

// autoSessionFile keeps the historical exec behavior: prefer SSHEX_SESSION,
// then fall back to any active session on this machine.
func autoSessionFile() (sessionfile.File, error) {
	file, err := sessionfile.Discover()
	if err == nil {
		return file, nil
	}
	if errors.Is(err, sessionfile.ErrAmbiguous) {
		active, listErr := sessionfile.ListActive()
		if listErr == nil && len(active) > 0 {
			return active[0], nil
		}
	}
	return sessionfile.File{}, err
}

func autoClient() (*client.Client, error) {
	file, err := autoSessionFile()
	if err != nil {
		return nil, err
	}
	return client.New(file), nil
}

func autoSessionClient(sessionID string) (*client.Client, error) {
	c, err := autoClient()
	if err != nil {
		return nil, err
	}
	if sessionID == "" {
		return c, nil
	}
	return c.ForSession(sessionID), nil
}
