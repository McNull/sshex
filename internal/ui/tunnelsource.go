package ui

import (
	"context"
	"errors"

	"github.com/mcnull/sshex/internal/api"
	"github.com/mcnull/sshex/internal/client"
	"github.com/mcnull/sshex/internal/model"
	"github.com/mcnull/sshex/internal/sessionfile"
	"github.com/mcnull/sshex/internal/ui/screen/tunnels"
)

// tunnelControl adapts the live broker client for the Tunnels screen. The
// screen only sees its own narrow Source and Controller interfaces, so tests
// never touch the daemon or the session files.
type tunnelControl struct{}

// List implements tunnels.Source. An empty session ID lists every session.
func (tunnelControl) List(sessionID string) ([]tunnels.Tunnel, error) {
	ctx, cancel := context.WithTimeout(context.Background(), sessionRequestTimeout)
	defer cancel()

	c, err := controlSessionClient()
	if err != nil {
		if errors.Is(err, sessionfile.ErrNone) {
			return nil, nil
		}
		return nil, err
	}

	if sessionID != "" {
		return listSessionTunnels(ctx, c, sessionID)
	}
	return listAllSessionTunnels(ctx, c)
}

// Add implements tunnels.Controller.
func (tunnelControl) Add(sessionID string, req tunnels.AddRequest) (tunnels.Tunnel, error) {
	ctx, cancel := context.WithTimeout(context.Background(), sessionRequestTimeout)
	defer cancel()

	c, err := controlSessionClient()
	if err != nil {
		return tunnels.Tunnel{}, err
	}
	added, err := c.ForSession(sessionID).AddTunnel(
		ctx,
		req.LocalPort,
		req.TargetHost,
		req.TargetPort,
		model.ForwardDirection(req.Direction),
	)
	if err != nil {
		return tunnels.Tunnel{}, err
	}
	return tunnelFromResponse(added, "", false), nil
}

// Remove implements tunnels.Controller.
func (tunnelControl) Remove(sessionID, tunnelID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), sessionRequestTimeout)
	defer cancel()

	c, err := controlSessionClient()
	if err != nil {
		return err
	}
	return c.ForSession(sessionID).RemoveTunnelByID(ctx, tunnelID)
}

func listSessionTunnels(ctx context.Context, c *client.Client, sessionID string) ([]tunnels.Tunnel, error) {
	scoped := c.ForSession(sessionID)
	target := sessionID
	stale := false
	if session, err := scoped.GetSession(ctx); err == nil {
		target = sessionLabel(session)
		stale = session.Stale
	}

	list, err := scoped.ListTunnels(ctx)
	if err != nil {
		return nil, err
	}
	return convertTunnels(list, target, stale), nil
}

func listAllSessionTunnels(ctx context.Context, c *client.Client) ([]tunnels.Tunnel, error) {
	sessions, err := c.ListSessions(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]tunnels.Tunnel, 0)
	seen := make(map[string]struct{})
	for _, session := range sessions {
		list, err := c.ForSession(session.ID).ListTunnels(ctx)
		if err != nil {
			return nil, err
		}
		for _, item := range list {
			if _, ok := seen[item.ID]; ok {
				continue
			}
			seen[item.ID] = struct{}{}
			out = append(out, tunnelFromResponse(item, sessionLabel(session), session.Stale))
		}
	}
	return out, nil
}

func convertTunnels(list []api.TunnelResponse, target string, stale bool) []tunnels.Tunnel {
	out := make([]tunnels.Tunnel, 0, len(list))
	for _, item := range list {
		out = append(out, tunnelFromResponse(item, target, stale))
	}
	return out
}

func tunnelFromResponse(item api.TunnelResponse, target string, stale bool) tunnels.Tunnel {
	direction := tunnels.Direction(item.Direction)
	if direction != tunnels.DirectionRemote {
		direction = tunnels.DirectionLocal
	}
	return tunnels.Tunnel{
		ID:         item.ID,
		SessionID:  item.SessionID,
		Target:     target,
		LocalPort:  item.LocalPort,
		TargetHost: item.TargetHost,
		TargetPort: item.TargetPort,
		Direction:  direction,
		State:      string(item.State),
		Error:      item.Error,
		Stale:      stale,
	}
}

func sessionLabel(session api.SessionResponse) string {
	target := session.Host
	if session.User != "" {
		target = session.User + "@" + target
	}
	if target == "" {
		return session.ID
	}
	return target
}
