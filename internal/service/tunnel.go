package service

import (
	"context"
	"fmt"
	"time"

	"github.com/mcnull/sshex/internal/errs"
	"github.com/mcnull/sshex/internal/model"
	"github.com/mcnull/sshex/internal/repository"
	"github.com/mcnull/sshex/internal/sshx"
)

type AddTunnelRequest struct {
	SessionID  string
	LocalPort  int
	TargetHost string
	TargetPort int
	Direction  model.ForwardDirection
}

type TunnelServiceConfig struct {
	Sessions  SessionStore
	Tunnels   TunnelStore
	Transport Transport
	IDs       IDGenerator
	Ports     PortChecker
	Pool      *sshx.Pool
}

type TunnelService struct {
	sessions  SessionStore
	tunnels   TunnelStore
	transport Transport
	ids       IDGenerator
	ports     PortChecker
	pool      *sshx.Pool
}

func NewTunnelService(cfg TunnelServiceConfig) *TunnelService {
	ports := cfg.Ports
	if ports == nil {
		ports = localPortInUse
	}
	return &TunnelService{
		sessions:  cfg.Sessions,
		tunnels:   cfg.Tunnels,
		transport: cfg.Transport,
		ids:       cfg.IDs,
		ports:     ports,
		pool:      cfg.Pool,
	}
}

func (s *TunnelService) Add(ctx context.Context, req AddTunnelRequest) (model.Tunnel, error) {
	direction := req.Direction
	if direction == "" {
		direction = model.ForwardLocal
	}
	if direction != model.ForwardLocal && direction != model.ForwardRemote {
		return model.Tunnel{}, errs.ErrInvalidInput
	}
	if req.LocalPort < 1 || req.LocalPort > 65535 {
		return model.Tunnel{}, errs.ErrInvalidInput
	}
	session, err := s.sessions.Get(ctx, req.SessionID)
	if err != nil {
		return model.Tunnel{}, err
	}
	if session.State != model.SessionActive {
		return model.Tunnel{}, errs.ErrNoSession
	}
	conn, ok := s.pool.Controller(req.SessionID)
	if !ok {
		return model.Tunnel{}, errs.ErrNoSession
	}

	if direction == model.ForwardLocal {
		inUse, err := s.ports(req.LocalPort)
		if err != nil {
			return model.Tunnel{}, err
		}
		if inUse {
			return model.Tunnel{}, fmt.Errorf("%w: local port %d is already in use", repository.ErrConflict, req.LocalPort)
		}
	}

	targetHost := req.TargetHost
	if targetHost == "" {
		targetHost = "localhost"
	}
	targetPort := req.TargetPort
	if targetPort == 0 {
		targetPort = req.LocalPort
	}

	spec := sshx.ForwardSpec{
		BindAddress: "127.0.0.1",
		LocalPort:   req.LocalPort,
		TargetHost:  targetHost,
		TargetPort:  targetPort,
	}
	if err := addForward(ctx, conn, direction, spec); err != nil {
		return model.Tunnel{}, err
	}

	tunnel := model.Tunnel{
		ID:         s.ids(),
		SessionID:  req.SessionID,
		LocalPort:  spec.LocalPort,
		TargetHost: spec.TargetHost,
		TargetPort: spec.TargetPort,
		Direction:  direction,
		Source:     model.SourceManual,
		State:      model.ForwardActive,
		CreatedAt:  time.Now(),
	}
	if err := s.tunnels.Create(ctx, tunnel); err != nil {
		_ = removeForward(ctx, conn, direction, spec)
		return model.Tunnel{}, err
	}
	return tunnel, nil
}

func (s *TunnelService) Remove(ctx context.Context, sessionID string, localPort int, direction model.ForwardDirection) error {
	tunnels, err := s.tunnels.ListBySession(ctx, sessionID)
	if err != nil {
		return err
	}
	for _, tunnel := range tunnels {
		if tunnel.LocalPort != localPort {
			continue
		}
		if direction != "" && tunnel.Direction != direction {
			continue
		}
		if conn, ok := s.pool.Controller(sessionID); ok {
			spec := sshx.ForwardSpec{
				BindAddress: "127.0.0.1",
				LocalPort:   tunnel.LocalPort,
				TargetHost:  tunnel.TargetHost,
				TargetPort:  tunnel.TargetPort,
			}
			if err := removeForward(ctx, conn, tunnel.Direction, spec); err != nil {
				return err
			}
		}
		return s.tunnels.Delete(ctx, tunnel.ID)
	}
	return repository.ErrNotFound
}

func addForward(ctx context.Context, conn sshx.Connection, direction model.ForwardDirection, spec sshx.ForwardSpec) error {
	if direction == model.ForwardRemote {
		return conn.AddRemoteForward(ctx, spec)
	}
	return conn.AddLocalForward(ctx, spec)
}

func removeForward(ctx context.Context, conn sshx.Connection, direction model.ForwardDirection, spec sshx.ForwardSpec) error {
	if direction == model.ForwardRemote {
		return conn.RemoveRemoteForward(ctx, spec)
	}
	return conn.RemoveLocalForward(ctx, spec)
}

func (s *TunnelService) List(ctx context.Context, sessionID string) ([]model.Tunnel, error) {
	return s.tunnels.ListBySession(ctx, sessionID)
}
