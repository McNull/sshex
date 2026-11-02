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

// sessionControl adapts the live broker client for the Sessions screen. The
// screen only sees its own narrow Source and Closer interfaces, so tests never
// touch the daemon or the session files.
type sessionControl struct{}

// List implements sessions.Source.
func (sessionControl) List() ([]sessions.Session, error) {
	ctx, cancel := context.WithTimeout(context.Background(), sessionRequestTimeout)
	defer cancel()

	c, err := controlSessionClient()
	if err != nil {
		if errors.Is(err, sessionfile.ErrNone) {
			return nil, nil
		}
		return nil, err
	}

	list, err := c.ListSessions(ctx)
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
func (sessionControl) Close(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), sessionRequestTimeout)
	defer cancel()

	c, err := controlSessionClient()
	if err != nil {
		return err
	}
	_, err = c.CloseSession(ctx, id)
	return err
}

// controlSessionClient returns a client for some live broker, preferring the
// default session and falling back to any active one when the origin is
// ambiguous. It mirrors the CLI's controlClient.
func controlSessionClient() (*client.Client, error) {
	file, err := sessionfile.Discover()
	if err == nil {
		return client.New(file), nil
	}
	if errors.Is(err, sessionfile.ErrAmbiguous) {
		active, listErr := sessionfile.ListActive()
		if listErr == nil && len(active) > 0 {
			return client.New(active[0]), nil
		}
	}
	return nil, err
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
