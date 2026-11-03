package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mcnull/sshex/internal/api"
	"github.com/mcnull/sshex/internal/client"
	"github.com/mcnull/sshex/internal/sessionfile"
	"github.com/mcnull/sshex/internal/sshx"
	"github.com/mcnull/sshex/internal/sshx/openssh"
	"github.com/mcnull/sshex/internal/ui/screen/sessions"
)

const sessionRequestTimeout = 5 * time.Second

// controlScope chooses which origin a screen talks to. Origin is the machine
// running the UI; Client is the origin of the current outer session.
type controlScope int

const (
	scopeClient controlScope = iota
	scopeOrigin
)

// sessionControl adapts the live broker client for the Sessions screen. The
// screen only sees its own narrow Source and Closer interfaces, so tests never
// touch the daemon or the session files.
type sessionControl struct {
	scope controlScope
}

// List implements sessions.Source.
func (c sessionControl) List() ([]sessions.Session, error) {
	ctx, cancel := context.WithTimeout(context.Background(), sessionRequestTimeout)
	defer cancel()

	broker, err := controlSessionClient(c.scope)
	if err != nil {
		if errors.Is(err, sessionfile.ErrNone) {
			return nil, nil
		}
		return nil, err
	}

	list, err := broker.ListSessions(ctx)
	if err != nil {
		return nil, err
	}

	current := currentRemoteSessionID()
	out := make([]sessions.Session, 0, len(list))
	for _, session := range list {
		target := session.Host
		if session.User != "" {
			target = session.User + "@" + target
		}
		out = append(out, sessions.Session{
			ID:          session.ID,
			Host:        session.Host,
			Target:      target,
			Origin:      session.Origin,
			State:       string(session.State),
			Connections: session.Connections,
			Hops:        sessionHopsLabel(ctx, session),
			Stale:       session.Stale,
			Current:     current != "" && session.ID == current,
		})
	}
	return out, nil
}

// currentRemoteSessionID reports the session of the remote shell that started
// the UI, if any. SSHEX_SESSION may hold an id or a path to a session file.
func currentRemoteSessionID() string {
	value := strings.TrimSpace(os.Getenv("SSHEX_SESSION"))
	if value == "" {
		return ""
	}
	if strings.ContainsRune(value, os.PathSeparator) {
		return strings.TrimSuffix(filepath.Base(value), filepath.Ext(value))
	}
	return value
}

// Close implements sessions.Closer.
func (c sessionControl) Close(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), sessionRequestTimeout)
	defer cancel()

	broker, err := controlSessionClient(c.scope)
	if err != nil {
		return err
	}
	_, err = broker.CloseSession(ctx, id)
	return err
}

// controlSessionFile returns a live session file for the scope. Client scope is
// strictly the current SSHEX_SESSION. Origin scope scans only this machine's
// runtime directory and falls back to the first active local session when more
// than one is reachable, because any of them can address the local broker.
func controlSessionFile(scope controlScope) (sessionfile.File, error) {
	if scope == scopeOrigin {
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
		return sessionfile.File{}, err
	}
	return sessionfile.Current()
}

// controlSessionClient returns a client for the scope's live broker.
func controlSessionClient(scope controlScope) (*client.Client, error) {
	file, err := controlSessionFile(scope)
	if err != nil {
		return nil, err
	}
	return client.New(file), nil
}

// sessionHopsLabel reports the hop count of a session, mirroring the CLI's
// HOPS column. A ProxyCommand makes the count unknowable and is shown as "?".
func sessionHopsLabel(ctx context.Context, session api.SessionResponse) string {
	if session.JumpHosts != "" {
		return strconv.Itoa(hopCount(session.JumpHosts))
	}
	info, err := openssh.ResolveProxy(ctx, sshx.Target{User: session.User, Host: session.Host, Port: session.Port})
	if err != nil {
		return "?"
	}
	if info.JumpHosts != "" {
		return strconv.Itoa(hopCount(info.JumpHosts))
	}
	if info.ProxyCommand {
		return "?"
	}
	return "0"
}

func hopCount(jumpHosts string) int {
	count := 0
	for _, hop := range strings.Split(jumpHosts, ",") {
		if strings.TrimSpace(hop) != "" {
			count++
		}
	}
	return count
}
