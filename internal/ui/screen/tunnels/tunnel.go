package tunnels

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mcnull/sshex/internal/service"
)

// Direction is the direction of a port forward.
type Direction string

const (
	DirectionLocal  Direction = "local"
	DirectionRemote Direction = "remote"
)

// Tunnel is one row of the tunnel list. It is deliberately independent of the
// broker DTO so the screen can be tested without a daemon.
type Tunnel struct {
	ID         string
	SessionID  string
	Target     string
	LocalPort  int
	TargetHost string
	TargetPort int
	Direction  Direction
	State      string
	Error      string
	Stale      bool
}

// AddRequest describes a tunnel to create on a session.
type AddRequest struct {
	LocalPort  int
	TargetHost string
	TargetPort int
	Direction  Direction
}

// Snapshot is one tunnel listing plus the number of sessions it covers.
type Snapshot struct {
	Tunnels  []Tunnel
	Sessions int
}

// Source lists the active tunnels. An empty session ID lists every session.
type Source interface {
	List(sessionID string) (Snapshot, error)
}

// Controller adds and removes tunnels.
type Controller interface {
	Add(sessionID string, req AddRequest) (Tunnel, error)
	Remove(sessionID, tunnelID string) error
}

// Forward renders the forward of a tunnel, mirroring the CLI's formatTunnel.
func (t Tunnel) Forward() string {
	arrow := ">"
	if t.Direction == DirectionRemote {
		arrow = "<"
	}
	return fmt.Sprintf("localhost:%d %s %s:%d", t.LocalPort, arrow, t.TargetHost, t.TargetPort)
}

// ParseAddRequest validates port[:target[:remote-port]] and builds a request.
func ParseAddRequest(spec string, direction Direction) (AddRequest, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return AddRequest{}, fmt.Errorf("enter a port, for example 8080")
	}
	parsed, err := service.ParsePortSpec(spec)
	if err != nil {
		return AddRequest{}, err
	}
	if direction != DirectionLocal && direction != DirectionRemote {
		direction = DirectionLocal
	}
	return AddRequest{
		LocalPort:  parsed.LocalPort,
		TargetHost: parsed.TargetHost,
		TargetPort: parsed.TargetPort,
		Direction:  direction,
	}, nil
}

// sortTunnels orders rows by session target, direction, local port and ID so a
// broker that returns a different order cannot make the list jump around.
func sortTunnels(items []Tunnel) {
	sort.SliceStable(items, func(i, j int) bool {
		left, right := strings.ToLower(tunnelTarget(items[i])), strings.ToLower(tunnelTarget(items[j]))
		if left != right {
			return left < right
		}
		left, right = string(items[i].Direction), string(items[j].Direction)
		if left != right {
			return left < right
		}
		if items[i].LocalPort != items[j].LocalPort {
			return items[i].LocalPort < items[j].LocalPort
		}
		return items[i].ID < items[j].ID
	})
}

func tunnelTarget(t Tunnel) string {
	if t.Target != "" {
		return t.Target
	}
	return t.SessionID
}
