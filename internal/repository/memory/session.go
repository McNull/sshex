package memory

import (
	"context"
	"sync"

	"github.com/mcnull/sshex/internal/model"
	"github.com/mcnull/sshex/internal/repository"
)

type SessionRepository struct {
	mu       sync.RWMutex
	sessions map[string]model.Session
}

func NewSessionRepository() *SessionRepository {
	return &SessionRepository{sessions: make(map[string]model.Session)}
}

func (r *SessionRepository) Create(_ context.Context, session model.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sessions[session.ID]; ok {
		return repository.ErrConflict
	}
	r.sessions[session.ID] = session
	return nil
}

func (r *SessionRepository) Get(_ context.Context, id string) (model.Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	session, ok := r.sessions[id]
	if !ok {
		return model.Session{}, repository.ErrNotFound
	}
	return session, nil
}

func (r *SessionRepository) List(_ context.Context) ([]model.Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	sessions := make([]model.Session, 0, len(r.sessions))
	for _, session := range r.sessions {
		sessions = append(sessions, session)
	}
	return sessions, nil
}

func (r *SessionRepository) Update(_ context.Context, session model.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sessions[session.ID]; !ok {
		return repository.ErrNotFound
	}
	r.sessions[session.ID] = session
	return nil
}

func (r *SessionRepository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sessions[id]; !ok {
		return repository.ErrNotFound
	}
	delete(r.sessions, id)
	return nil
}
