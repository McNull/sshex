package unix

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/mcnull/sshex/internal/control"
	"github.com/mcnull/sshex/internal/model"
)

const (
	DirMode    = 0o700
	SocketMode = 0o600

	// shutdownTimeout bounds how long a cancelled listener waits for in-flight
	// requests to finish before their connections are closed.
	shutdownTimeout = 5 * time.Second
)

type Provider struct {
	dir string
}

func NewProvider(dir string) *Provider {
	return &Provider{dir: dir}
}

func (p *Provider) Dir() string {
	return p.dir
}

func (p *Provider) Listen(_ context.Context, id string) (control.Listener, error) {
	if err := os.MkdirAll(p.dir, DirMode); err != nil {
		return nil, err
	}
	path := filepath.Join(p.dir, id+".sock")
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, SocketMode); err != nil {
		_ = ln.Close()
		return nil, err
	}
	return &Listener{
		endpoint: model.Endpoint{Kind: model.EndpointUnix, Address: path},
		path:     path,
		ln:       ln,
	}, nil
}

type Listener struct {
	endpoint model.Endpoint
	path     string
	ln       net.Listener
	server   *http.Server
}

func (l *Listener) Endpoint() model.Endpoint {
	return l.endpoint
}

func (l *Listener) Serve(ctx context.Context, handler http.Handler) error {
	l.server = &http.Server{Handler: handler}
	served := make(chan struct{})
	shutdown := make(chan struct{})
	go func() {
		defer close(shutdown)
		select {
		case <-ctx.Done():
			l.shutdown()
		case <-served:
		}
	}()
	err := l.server.Serve(l.ln)
	close(served)
	if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
		// When the context cancelled the server, wait until the graceful
		// shutdown has finished so that in-flight responses are written before
		// Serve returns and the caller removes the socket.
		<-shutdown
		return nil
	}
	return err
}

// shutdown stops accepting connections and waits for in-flight requests to
// finish, so a handler that triggers teardown still delivers its response.
func (l *Listener) shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := l.server.Shutdown(ctx); err != nil {
		_ = l.server.Close()
	}
	_ = l.ln.Close()
	_ = os.Remove(l.path)
}

func (l *Listener) Close() error {
	if l.server != nil {
		_ = l.server.Close()
	}
	err := l.ln.Close()
	_ = os.Remove(l.path)
	if errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}
