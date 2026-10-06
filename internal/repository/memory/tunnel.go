package memory

import (
	"context"
	"sync"

	"github.com/mcnull/sshex/internal/model"
	"github.com/mcnull/sshex/internal/repository"
)

type TunnelRepository struct {
	mu      sync.RWMutex
	tunnels map[string]model.Tunnel
}

func NewTunnelRepository() *TunnelRepository {
	return &TunnelRepository{tunnels: make(map[string]model.Tunnel)}
}

func (r *TunnelRepository) Create(_ context.Context, tunnel model.Tunnel) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tunnels[tunnel.ID]; ok {
		return repository.ErrConflict
	}
	for _, existing := range r.tunnels {
		if existing.SessionID == tunnel.SessionID &&
			existing.LocalPort == tunnel.LocalPort &&
			existing.Direction == tunnel.Direction {
			return repository.ErrConflict
		}
	}
	r.tunnels[tunnel.ID] = tunnel
	return nil
}

func (r *TunnelRepository) Get(_ context.Context, id string) (model.Tunnel, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tunnel, ok := r.tunnels[id]
	if !ok {
		return model.Tunnel{}, repository.ErrNotFound
	}
	return tunnel, nil
}

func (r *TunnelRepository) ListBySession(_ context.Context, sessionID string) ([]model.Tunnel, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tunnels := make([]model.Tunnel, 0)
	for _, tunnel := range r.tunnels {
		if tunnel.SessionID == sessionID {
			tunnels = append(tunnels, tunnel)
		}
	}
	return tunnels, nil
}

func (r *TunnelRepository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tunnels[id]; !ok {
		return repository.ErrNotFound
	}
	delete(r.tunnels, id)
	return nil
}

func (r *TunnelRepository) DeleteBySession(_ context.Context, sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, tunnel := range r.tunnels {
		if tunnel.SessionID == sessionID {
			delete(r.tunnels, id)
		}
	}
	return nil
}
