package memory

import (
	"context"
	"sync"

	"github.com/mcnull/sshex/internal/model"
	"github.com/mcnull/sshex/internal/repository"
)

type ConnectionRepository struct {
	mu          sync.RWMutex
	connections map[string]model.Connection
}

func NewConnectionRepository() *ConnectionRepository {
	return &ConnectionRepository{connections: make(map[string]model.Connection)}
}

func (r *ConnectionRepository) Create(_ context.Context, connection model.Connection) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.connections[connection.ID]; ok {
		return repository.ErrConflict
	}
	r.connections[connection.ID] = connection
	return nil
}

func (r *ConnectionRepository) Get(_ context.Context, id string) (model.Connection, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	connection, ok := r.connections[id]
	if !ok {
		return model.Connection{}, repository.ErrNotFound
	}
	return connection, nil
}

func (r *ConnectionRepository) ListBySession(_ context.Context, sessionID string) ([]model.Connection, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	connections := make([]model.Connection, 0)
	for _, connection := range r.connections {
		if connection.SessionID == sessionID {
			connections = append(connections, connection)
		}
	}
	return connections, nil
}

func (r *ConnectionRepository) CountBySession(_ context.Context, sessionID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	count := 0
	for _, connection := range r.connections {
		if connection.SessionID == sessionID {
			count++
		}
	}
	return count, nil
}

func (r *ConnectionRepository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.connections[id]; !ok {
		return repository.ErrNotFound
	}
	delete(r.connections, id)
	return nil
}

func (r *ConnectionRepository) DeleteBySession(_ context.Context, sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, connection := range r.connections {
		if connection.SessionID == sessionID {
			delete(r.connections, id)
		}
	}
	return nil
}
