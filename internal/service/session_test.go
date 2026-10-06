package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/mcnull/sshex/internal/control"
	"github.com/mcnull/sshex/internal/control/unix"
	"github.com/mcnull/sshex/internal/errs"
	"github.com/mcnull/sshex/internal/model"
	"github.com/mcnull/sshex/internal/repository"
	"github.com/mcnull/sshex/internal/repository/memory"
	"github.com/mcnull/sshex/internal/sshx"
)

func TestSessionServiceConnectRequiresHost(t *testing.T) {
	svc := newTestServices()
	if _, _, err := svc.sessions.Connect(context.Background(), ConnectRequest{}); !errors.Is(err, errs.ErrInvalidInput) {
		t.Fatalf("Connect() error = %v, want ErrInvalidInput", err)
	}
}

func TestSessionServiceCloseMissing(t *testing.T) {
	svc := newTestServices()
	if _, _, err := svc.sessions.Close(context.Background(), "s1"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Close() error = %v, want ErrNotFound", err)
	}
}

func TestSessionServiceListAndGet(t *testing.T) {
	ctx := context.Background()
	svc := newTestServices()
	session := model.Session{ID: "s1", Host: "h"}
	if err := svc.sessionRepo.Create(ctx, session); err != nil {
		t.Fatalf("seed error: %v", err)
	}

	list, err := svc.sessions.List(ctx)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(list) != 1 || list[0].ID != "s1" {
		t.Fatalf("List() = %+v, want [s1]", list)
	}

	got, err := svc.sessions.Get(ctx, "s1")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got.ID != "s1" {
		t.Fatalf("Get() = %+v, want s1", got)
	}
}

func seedSessionWithConnections(t *testing.T, svc testServices, sessionID string, connectionIDs ...string) {
	t.Helper()
	ctx := context.Background()
	if err := svc.sessionRepo.Create(ctx, model.Session{ID: sessionID, Host: "h", State: model.SessionActive}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	for _, id := range connectionIDs {
		if err := svc.connectionRepo.Create(ctx, model.Connection{
			ID: id, SessionID: sessionID, State: model.ConnectionActive,
		}); err != nil {
			t.Fatalf("seed connection: %v", err)
		}
		svc.pool.SetConnection(id, &fakeConn{})
	}
}

func TestSessionServiceDetachKeepsSessionWithSiblings(t *testing.T) {
	ctx := context.Background()
	svc := newTestServices()
	seedSessionWithConnections(t, svc, "s1", "c1", "c2")

	dropped, teardown, err := svc.sessions.Detach(ctx, "s1", "c1")
	if err != nil {
		t.Fatalf("Detach() error: %v", err)
	}
	if teardown {
		t.Fatal("Detach(c1) tore down while c2 was still connected")
	}
	if len(dropped) != 0 {
		t.Fatalf("Detach(c1) dropped = %+v, want none", dropped)
	}
	if s, _ := svc.sessions.Get(ctx, "s1"); s.State == model.SessionClosed {
		t.Fatal("Detach(c1) closed a session kept alive for its tunnels")
	}

	_, teardown, err = svc.sessions.Detach(ctx, "s1", "c2")
	if err != nil {
		t.Fatalf("Detach(c2) error: %v", err)
	}
	if !teardown {
		t.Fatal("Detach(c2) did not tear down the last connection")
	}
	if s, _ := svc.sessions.Get(ctx, "s1"); s.State != model.SessionClosed {
		t.Fatalf("session state = %q, want closed", s.State)
	}
}

func TestSessionServiceDetachScopesTeardownToRemote(t *testing.T) {
	ctx := context.Background()
	svc := newTestServices()
	seedSessionWithConnections(t, svc, "s1", "c1")
	seedSessionWithConnections(t, svc, "s2", "c2")

	_, teardown, err := svc.sessions.Detach(ctx, "s1", "c1")
	if err != nil {
		t.Fatalf("Detach() error: %v", err)
	}
	if teardown {
		t.Fatal("Detach() tore down the broker while another remote's session was live")
	}
	if s, _ := svc.sessions.Get(ctx, "s1"); s.State != model.SessionClosed {
		t.Fatalf("session s1 state = %q, want closed", s.State)
	}
	if s, _ := svc.sessions.Get(ctx, "s2"); s.State != model.SessionActive {
		t.Fatalf("session s2 state = %q, want active", s.State)
	}
}

func TestSessionServiceDetachRunsTeardownHookOnLast(t *testing.T) {
	ctx := context.Background()
	svc := newTestServices()
	seedSessionWithConnections(t, svc, "s1", "c1")

	hookCalls := 0
	svc.sessions.SetTeardownHook(func() { hookCalls++ })

	dropped, teardown, err := svc.sessions.Detach(ctx, "s1", "c1")
	if err != nil || !teardown {
		t.Fatalf("Detach() = teardown %v, err %v; want true, nil", teardown, err)
	}
	if len(dropped) != 0 {
		t.Fatalf("Detach() dropped = %+v, want none", dropped)
	}
	if hookCalls != 0 {
		t.Fatalf("hook called %d times before NotifyTeardown", hookCalls)
	}
	svc.sessions.NotifyTeardown()
	if hookCalls != 1 {
		t.Fatalf("hook called %d times, want 1", hookCalls)
	}
}

func TestSessionServiceConnectionCount(t *testing.T) {
	ctx := context.Background()
	svc := newTestServices()
	seedSessionWithConnections(t, svc, "s1", "c1", "c2")
	if got := svc.sessions.ConnectionCount(ctx, "s1"); got != 2 {
		t.Fatalf("ConnectionCount() = %d, want 2", got)
	}
	if _, _, err := svc.sessions.Detach(ctx, "s1", "c1"); err != nil {
		t.Fatalf("Detach() error: %v", err)
	}
	if got := svc.sessions.ConnectionCount(ctx, "s1"); got != 1 {
		t.Fatalf("ConnectionCount() = %d, want 1", got)
	}
}

func TestSessionServiceHeartbeatExpiryDestroysSession(t *testing.T) {
	ctx := context.Background()
	svc := newTestServices()
	seedSessionWithConnections(t, svc, "s1", "c1")

	now := time.Unix(0, 0)
	svc.sessions.heartbeat = NewHeartbeatMonitor(10*time.Second, 30*time.Second, func() time.Time { return now }, func(id string) {
		svc.sessions.expireConnection(ctx, id)
	})
	if err := svc.sessions.Heartbeat(ctx, "s1", "c1"); err != nil {
		t.Fatalf("Heartbeat() error: %v", err)
	}

	svc.sessions.heartbeat.reap(now.Add(29 * time.Second))
	if got := svc.sessions.ConnectionCount(ctx, "s1"); got != 1 {
		t.Fatalf("ConnectionCount() = %d before timeout, want 1", got)
	}

	svc.sessions.heartbeat.reap(now.Add(31 * time.Second))
	if got := svc.sessions.ConnectionCount(ctx, "s1"); got != 0 {
		t.Fatalf("ConnectionCount() = %d after timeout, want 0", got)
	}
	if s, _ := svc.sessions.Get(ctx, "s1"); s.State != model.SessionClosed {
		t.Fatalf("session state = %q, want closed", s.State)
	}
}

func TestSessionServiceHeartbeatRejectsForeignConnection(t *testing.T) {
	ctx := context.Background()
	svc := newTestServices()
	seedSessionWithConnections(t, svc, "s1", "c1")
	if err := svc.sessions.Heartbeat(ctx, "other", "c1"); !errors.Is(err, errs.ErrInvalidInput) {
		t.Fatalf("Heartbeat() error = %v, want ErrInvalidInput", err)
	}
	if err := svc.sessions.Heartbeat(ctx, "s1", "missing"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Heartbeat() error = %v, want ErrNotFound", err)
	}
}

func TestSessionServiceFindSessionMatchesJumpHosts(t *testing.T) {
	ctx := context.Background()
	svc := newTestServices()
	seed := func(id, jump string) {
		t.Helper()
		if err := svc.sessionRepo.Create(ctx, model.Session{
			ID: id, User: "bob", Host: "h1", Port: 22, JumpHosts: jump, State: model.SessionActive,
		}); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	seed("direct", "")
	seed("jumped", "ssh01")

	cases := []struct {
		name   string
		target sshx.Target
		wantID string
	}{
		{"direct matches direct", sshx.Target{User: "bob", Host: "h1", Port: 22}, "direct"},
		{"jump distinguishes", sshx.Target{User: "bob", Host: "h1", Port: 22, JumpHosts: "ssh01"}, "jumped"},
		{"different jump no match", sshx.Target{User: "bob", Host: "h1", Port: 22, JumpHosts: "ssh01,ssh02"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := svc.sessions.findSession(ctx, tc.target)
			if err != nil {
				t.Fatalf("findSession() error: %v", err)
			}
			if got.ID != tc.wantID {
				t.Fatalf("findSession() = %q, want %q", got.ID, tc.wantID)
			}
		})
	}
}

func TestSessionServiceFindSessionMatchesRemote(t *testing.T) {
	ctx := context.Background()
	svc := newTestServices()
	seed := func(id, host string, port int, state model.SessionState) {
		t.Helper()
		if err := svc.sessionRepo.Create(ctx, model.Session{
			ID: id, User: "bob", Host: host, Port: port, State: state,
		}); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	seed("s1", "h1", 22, model.SessionActive)
	seed("s2", "h1", 2222, model.SessionActive)
	seed("s3", "h2", 22, model.SessionClosed)

	cases := []struct {
		name   string
		target sshx.Target
		wantID string
	}{
		{"same host and port", sshx.Target{User: "bob", Host: "h1", Port: 22}, "s1"},
		{"port distinguishes", sshx.Target{User: "bob", Host: "h1", Port: 2222}, "s2"},
		{"closed ignored", sshx.Target{User: "bob", Host: "h2", Port: 22}, ""},
		{"no match", sshx.Target{User: "bob", Host: "h3", Port: 22}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := svc.sessions.findSession(ctx, tc.target)
			if err != nil {
				t.Fatalf("findSession() error: %v", err)
			}
			if got.ID != tc.wantID {
				t.Fatalf("findSession() = %q, want %q", got.ID, tc.wantID)
			}
		})
	}
}

// pathConn is a fake connection that reports a control path and records Close.
type pathConn struct {
	*fakeConn
	path   string
	closed bool
}

func (c *pathConn) ControlPath() string { return c.path }

func (c *pathConn) Close(context.Context) error {
	c.closed = true
	return nil
}

func newAdoptingService(t *testing.T, transport Transport) (testServices, *SessionService) {
	t.Helper()
	sessionRepo := memory.NewSessionRepository()
	connectionRepo := memory.NewConnectionRepository()
	tunnelRepo := memory.NewTunnelRepository()
	pool := sshx.NewPool()
	n := 0
	ids := func() string {
		n++
		return fmt.Sprintf("id-%d", n)
	}
	svc := NewSessionService(SessionServiceConfig{
		Sessions:    sessionRepo,
		Connections: connectionRepo,
		Tunnels:     tunnelRepo,
		Transport:   transport,
		Control:     unix.NewProvider(t.TempDir()),
		Installer:   &fakeInstaller{},
		Completions: &fakeCompletions{},
		Auth:        control.NewAuthenticator(),
		IDs:         ids,
		Pool:        pool,
		ControlDir:  t.TempDir(),
	})
	return testServices{
		sessions:       svc,
		pool:           pool,
		sessionRepo:    sessionRepo,
		connectionRepo: connectionRepo,
		tunnelRepo:     tunnelRepo,
	}, svc
}

func TestConnectAdoptsProvidedControlPaths(t *testing.T) {
	ft := &fakeTransport{conn: &fakeConn{}}
	_, svc := newAdoptingService(t, ft)

	session, connection, err := svc.Connect(context.Background(), ConnectRequest{
		Host:                  "ssh02",
		ControllerControlPath: "/tmp/ctrl.ctl",
		ConnectionControlPath: "/tmp/conn.ctl",
	})
	if err != nil {
		t.Fatalf("Connect() error: %v", err)
	}
	if !ft.adopted {
		t.Fatal("Connect() did not adopt the provided masters")
	}
	if ft.called {
		t.Fatal("Connect() created its own master despite provided paths")
	}
	if session.ControlPath != "/tmp/ctrl.ctl" {
		t.Fatalf("session control path = %q, want /tmp/ctrl.ctl", session.ControlPath)
	}
	if connection.ControlPath != "/tmp/conn.ctl" {
		t.Fatalf("connection control path = %q, want /tmp/conn.ctl", connection.ControlPath)
	}
}

func TestConnectCreatesMasterWithoutProvidedPath(t *testing.T) {
	ft := &fakeTransport{conn: &fakeConn{}}
	_, svc := newAdoptingService(t, ft)

	if _, _, err := svc.Connect(context.Background(), ConnectRequest{Host: "ssh02"}); err != nil {
		t.Fatalf("Connect() error: %v", err)
	}
	if ft.adopted {
		t.Fatal("Connect() adopted a master with no provided path")
	}
	if !ft.called {
		t.Fatal("Connect() did not create a master")
	}
}

func TestRemoveConnectionKeepsSharedControllerMaster(t *testing.T) {
	ctx := context.Background()
	ft := &fakeTransport{conn: &fakeConn{}}
	svc, sessions := newAdoptingService(t, ft)

	shared := "/tmp/shared.ctl"
	if err := svc.sessionRepo.Create(ctx, model.Session{ID: "s1", Host: "h", State: model.SessionActive, ControlPath: shared}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if err := svc.connectionRepo.Create(ctx, model.Connection{ID: "c1", SessionID: "s1", State: model.ConnectionActive, ControlPath: shared}); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	ctrl := &pathConn{fakeConn: &fakeConn{}, path: shared}
	conn := &pathConn{fakeConn: &fakeConn{}, path: shared}
	svc.pool.SetController("s1", ctrl)
	svc.pool.SetConnection("c1", conn)

	if _, _, err := sessions.removeConnection(ctx, model.Connection{ID: "c1", SessionID: "s1", ControlPath: shared}); err != nil {
		t.Fatalf("removeConnection() error: %v", err)
	}
	if conn.closed {
		t.Fatal("removeConnection() closed the shared controller master")
	}
	if !ctrl.closed {
		t.Fatal("session teardown did not close the controller master")
	}
}
