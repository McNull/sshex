package control

import (
	"context"
	"net/http"

	"github.com/mcnull/sshex/internal/model"
)

type Listener interface {
	Endpoint() model.Endpoint
	Serve(ctx context.Context, handler http.Handler) error
	Close() error
}

type Provider interface {
	Listen(ctx context.Context, id string) (Listener, error)
}
