package ui

import (
	"context"
	"errors"
	"time"

	"github.com/mcnull/sshex/internal/api"
	"github.com/mcnull/sshex/internal/sessionfile"
	"github.com/mcnull/sshex/internal/ui/screen/tunnels"
)

// portRequestTimeout is slightly longer than the broker's probe timeout so the
// inline error comes from the probe rather than from a canceled HTTP request.
const portRequestTimeout = 8 * time.Second

// portControl adapts the live broker client for the add-tunnel port list. The
// screen only sees the narrow tunnels.PortSource interface.
type portControl struct{}

// List implements tunnels.PortSource.
func (portControl) List(sessionID string, host tunnels.PortHost) ([]tunnels.Port, error) {
	ctx, cancel := context.WithTimeout(context.Background(), portRequestTimeout)
	defer cancel()

	c, err := controlSessionClient()
	if err != nil {
		if errors.Is(err, sessionfile.ErrNone) {
			return nil, nil
		}
		return nil, err
	}

	list, err := c.ForSession(sessionID).ListPorts(ctx, string(host))
	if err != nil {
		return nil, err
	}
	out := make([]tunnels.Port, 0, len(list))
	for _, item := range list {
		out = append(out, portFromResponse(item))
	}
	return out, nil
}

func portFromResponse(item api.ListeningPortResponse) tunnels.Port {
	return tunnels.Port{
		Port:    item.Port,
		Address: item.Address,
		Process: item.Process,
		PID:     item.PID,
	}
}
