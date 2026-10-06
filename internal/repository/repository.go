package repository

import (
	"context"

	"github.com/mcnull/sshex/internal/model"
)

type SessionRepository interface {
	Create(ctx context.Context, session model.Session) error
	Get(ctx context.Context, id string) (model.Session, error)
	List(ctx context.Context) ([]model.Session, error)
	Update(ctx context.Context, session model.Session) error
	Delete(ctx context.Context, id string) error
}

type ConnectionRepository interface {
	Create(ctx context.Context, connection model.Connection) error
	Get(ctx context.Context, id string) (model.Connection, error)
	ListBySession(ctx context.Context, sessionID string) ([]model.Connection, error)
	CountBySession(ctx context.Context, sessionID string) (int, error)
	Delete(ctx context.Context, id string) error
	DeleteBySession(ctx context.Context, sessionID string) error
}

type TunnelRepository interface {
	Create(ctx context.Context, tunnel model.Tunnel) error
	Get(ctx context.Context, id string) (model.Tunnel, error)
	ListBySession(ctx context.Context, sessionID string) ([]model.Tunnel, error)
	Delete(ctx context.Context, id string) error
	DeleteBySession(ctx context.Context, sessionID string) error
}

// CommandRepository stores predefined commands, keyed by name.
type CommandRepository interface {
	Create(ctx context.Context, command model.Command) error
	Get(ctx context.Context, name string) (model.Command, error)
	List(ctx context.Context) ([]model.Command, error)
	// Update replaces the command currently stored under name. The replacement
	// may carry a different Name to rename the command.
	Update(ctx context.Context, name string, command model.Command) error
	Delete(ctx context.Context, name string) error
}
