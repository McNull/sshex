package unix

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/mcnull/sshex/internal/control"
	"github.com/mcnull/sshex/internal/model"
)

const (
	DirMode    = 0o700
	SocketMode = 0o600
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
	go func() {
		<-ctx.Done()
		_ = l.Close()
	}()
	err := l.server.Serve(l.ln)
	if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
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
