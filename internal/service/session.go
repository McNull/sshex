package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/mcnull/sshex/internal/config"
	"github.com/mcnull/sshex/internal/control"
	"github.com/mcnull/sshex/internal/errs"
	"github.com/mcnull/sshex/internal/model"
	"github.com/mcnull/sshex/internal/remoteinstall"
	"github.com/mcnull/sshex/internal/repository"
	"github.com/mcnull/sshex/internal/sessionfile"
	"github.com/mcnull/sshex/internal/sshx"
)

type ConnectRequest struct {
	User            string
	Host            string
	Port            int
	IdentityFile    string
	JumpHosts       string
	Options         []string
	SkipCompletions bool
	// ControllerControlPath and ConnectionControlPath are SSH control masters
	// the interactive client already established in the foreground. When set,
	// the service adopts them instead of creating a master it cannot prompt on.
	ControllerControlPath string
	ConnectionControlPath string
}

type SessionServiceConfig struct {
	Sessions          SessionStore
	Connections       ConnectionStore
	Tunnels           TunnelStore
	Commands          CommandStore
	Transport         Transport
	Control           ControlProvider
	Installer         Installer
	Completions       Completer
	Aliases           AliasInstaller
	Auth              Auth
	IDs               IDGenerator
	Pool              *sshx.Pool
	SessionDir        string
	ControlDir        string
	HeartbeatInterval time.Duration
	HeartbeatTimeout  time.Duration
}

type SessionService struct {
	sessions    SessionStore
	connections ConnectionStore
	tunnels     TunnelStore
	commands    CommandStore
	transport   Transport
	control     ControlProvider
	installer   Installer
	completions Completer
	aliases     AliasInstaller
	auth        Auth
	ids         IDGenerator
	pool        *sshx.Pool
	sessionDir  string
	controlDir  string
	heartbeat   *HeartbeatMonitor

	createMu  sync.Mutex
	mu        sync.Mutex
	handler   http.Handler
	baseCtx   context.Context
	listeners map[string]control.Listener
	teardown  func()
}

func NewSessionService(cfg SessionServiceConfig) *SessionService {
	s := &SessionService{
		sessions:    cfg.Sessions,
		connections: cfg.Connections,
		tunnels:     cfg.Tunnels,
		commands:    cfg.Commands,
		transport:   cfg.Transport,
		control:     cfg.Control,
		installer:   cfg.Installer,
		completions: cfg.Completions,
		aliases:     cfg.Aliases,
		auth:        cfg.Auth,
		ids:         cfg.IDs,
		pool:        cfg.Pool,
		sessionDir:  cfg.SessionDir,
		controlDir:  cfg.ControlDir,
		listeners:   make(map[string]control.Listener),
	}
	s.heartbeat = NewHeartbeatMonitor(cfg.HeartbeatInterval, cfg.HeartbeatTimeout, time.Now, func(connectionID string) {
		s.expireConnection(context.Background(), connectionID)
	})
	return s
}

func (s *SessionService) SetHandler(handler http.Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handler = handler
}

// SetBaseContext sets the lifetime context used to serve control listeners. It
// must outlive the request that creates a session, so the session's listener
// stays up after the create request returns.
func (s *SessionService) SetBaseContext(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.baseCtx = ctx
}

func (s *SessionService) serveContext() context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.baseCtx != nil {
		return s.baseCtx
	}
	return context.Background()
}

// SetTeardownHook registers a callback invoked after the last session has been
// destroyed and no connections remain.
func (s *SessionService) SetTeardownHook(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.teardown = fn
}

// Connect joins the session for the remote target, creating it (and its
// controller connection) when this is the first connect, then adds an
// interactive connection for the caller's shell.
func (s *SessionService) Connect(ctx context.Context, req ConnectRequest) (model.Session, model.Connection, error) {
	if req.Host == "" {
		return model.Session{}, model.Connection{}, fmt.Errorf("%w: host is required", errs.ErrInvalidInput)
	}
	port := req.Port
	if port == 0 {
		port = 22
	}
	target := sshx.Target{
		User:         req.User,
		Host:         req.Host,
		Port:         port,
		IdentityFile: req.IdentityFile,
		JumpHosts:    req.JumpHosts,
		Options:      req.Options,
	}

	s.createMu.Lock()
	defer s.createMu.Unlock()

	session, err := s.findSession(ctx, target)
	if err != nil {
		return model.Session{}, model.Connection{}, err
	}

	created := false
	if session.ID == "" {
		session, err = s.createSession(ctx, target, req)
		if err != nil {
			return model.Session{}, model.Connection{}, err
		}
		created = true
	}

	connection, err := s.addConnection(ctx, session, target, req.ConnectionControlPath)
	if err != nil {
		if created {
			_, _ = s.closeSession(ctx, session)
		}
		return model.Session{}, model.Connection{}, err
	}
	return session, connection, nil
}

// findSession returns the active session for the target, or a zero session when
// none exists yet.
func (s *SessionService) findSession(ctx context.Context, target sshx.Target) (model.Session, error) {
	sessions, err := s.sessions.List(ctx)
	if err != nil {
		return model.Session{}, err
	}
	for _, session := range sessions {
		if session.State != model.SessionActive {
			continue
		}
		if session.User == target.User && session.Host == target.Host && session.Port == target.Port && session.JumpHosts == target.JumpHosts {
			return session, nil
		}
	}
	return model.Session{}, nil
}

// createSession establishes the controller connection that owns the control
// channel and every tunnel of the session.
func (s *SessionService) createSession(ctx context.Context, target sshx.Target, req ConnectRequest) (model.Session, error) {
	id := s.ids()
	token, err := control.GenerateToken()
	if err != nil {
		return model.Session{}, err
	}

	listener, err := s.control.Listen(ctx, id)
	if err != nil {
		return model.Session{}, fmt.Errorf("create control channel: %w", err)
	}

	controlPath := req.ControllerControlPath
	adopt := controlPath != ""
	if !adopt {
		controlPath = filepath.Join(s.controlDir, id+".ctl")
	}
	conn, err := s.dialMaster(ctx, target, controlPath, id, adopt)
	if err != nil {
		_ = listener.Close()
		return model.Session{}, err
	}

	fail := func(err error) (model.Session, error) {
		_ = conn.Close(ctx)
		_ = listener.Close()
		return model.Session{}, err
	}

	paths, err := s.installer.RemotePaths(ctx, conn)
	if err != nil {
		return fail(err)
	}
	if err := s.installer.Install(ctx, conn, paths); err != nil {
		return fail(err)
	}

	var shell remoteinstall.Shell
	if !req.SkipCompletions && s.completions != nil {
		if detected, ok, err := s.completions.DetectShell(ctx, conn); err == nil && ok {
			shell = detected
			_ = s.completions.Install(ctx, conn, paths, shell)
			s.installAliases(ctx, conn, paths, shell)
		}
	}

	remoteSocket := remoteinstall.RemoteSocketPath(paths.RunDir, id)
	if err := conn.AddRemoteSocketForward(ctx, sshx.RemoteSocketForwardSpec{
		RemotePath: remoteSocket,
		LocalPath:  listener.Endpoint().Address,
	}); err != nil {
		return fail(fmt.Errorf("establish control channel: %w", err))
	}

	origin := config.Origin()
	remoteFile := sessionfile.File{
		ID:           id,
		Origin:       origin,
		Endpoint:     model.Endpoint{Kind: model.EndpointUnix, Address: remoteSocket},
		Token:        token,
		Capabilities: model.Capabilities{model.CapabilityTunnels, model.CapabilityExec},
		Target:       target.String(),
	}
	if err := s.installer.WriteSession(ctx, conn, paths, remoteFile); err != nil {
		return fail(err)
	}

	session := model.Session{
		ID:           id,
		Origin:       origin,
		User:         target.User,
		Host:         target.Host,
		Port:         target.Port,
		JumpHosts:    target.JumpHosts,
		State:        model.SessionActive,
		Endpoint:     listener.Endpoint(),
		Token:        token,
		Capabilities: model.Capabilities{model.CapabilityTunnels, model.CapabilityExec},
		ControlPath:  controlPath,
		RemoteSocket: remoteSocket,
		Shell:        string(shell),
		CreatedAt:    time.Now(),
	}
	if err := s.sessions.Create(ctx, session); err != nil {
		return fail(err)
	}

	if s.sessionDir != "" {
		localFile := remoteFile
		localFile.Endpoint = listener.Endpoint()
		if err := sessionfile.WriteLocal(s.sessionDir, localFile); err != nil {
			_ = s.sessions.Delete(ctx, id)
			return fail(err)
		}
	}

	s.auth.Register(token, model.Capabilities{model.CapabilityTunnels, model.CapabilityExec}, "")
	s.pool.SetController(id, conn)
	s.setListener(id, listener)

	s.mu.Lock()
	handler := s.handler
	s.mu.Unlock()
	if handler == nil {
		handler = http.NotFoundHandler()
	}
	go func() { _ = listener.Serve(s.serveContext(), handler) }()

	return session, nil
}

// installAliases writes the shell alias file for a freshly created session.
func (s *SessionService) installAliases(ctx context.Context, conn sshx.Connection, paths remoteinstall.RemotePaths, shell remoteinstall.Shell) {
	if s.aliases == nil || s.commands == nil {
		return
	}
	commands, err := s.commands.List(ctx)
	if err != nil {
		return
	}
	_ = s.aliases.Install(ctx, conn, paths, shell, activeAliases(commands))
}

// PublishAliases rewrites the shell alias file for every active session so that
// a newly opened shell sees the current set of enabled commands.
func (s *SessionService) PublishAliases(ctx context.Context) {
	if s.aliases == nil || s.commands == nil {
		return
	}
	commands, err := s.commands.List(ctx)
	if err != nil {
		return
	}
	aliases := activeAliases(commands)
	sessions, err := s.sessions.List(ctx)
	if err != nil {
		return
	}
	for _, session := range sessions {
		if session.State != model.SessionActive || session.Shell == "" {
			continue
		}
		conn, ok := s.pool.Controller(session.ID)
		if !ok {
			continue
		}
		paths, err := s.installer.RemotePaths(ctx, conn)
		if err != nil {
			continue
		}
		_ = s.aliases.Install(ctx, conn, paths, remoteinstall.Shell(session.Shell), aliases)
	}
}

// activeAliases selects the aliases of enabled commands.
func activeAliases(commands []model.Command) []remoteinstall.Alias {
	aliases := make([]remoteinstall.Alias, 0, len(commands))
	for _, command := range commands {
		if command.Disabled || command.Alias == "" {
			continue
		}
		aliases = append(aliases, remoteinstall.Alias{Name: command.Alias, Command: command.Name})
	}
	return aliases
}

// and records it as a member of the session. When controlPath is non-empty it
// adopts a master the interactive client already established.
func (s *SessionService) addConnection(ctx context.Context, session model.Session, target sshx.Target, controlPath string) (model.Connection, error) {
	id := s.ids()
	adopt := controlPath != ""
	if !adopt {
		controlPath = filepath.Join(s.controlDir, id+".ctl")
	}
	conn, err := s.dialMaster(ctx, target, controlPath, session.ID, adopt)
	if err != nil {
		return model.Connection{}, err
	}

	connection := model.Connection{
		ID:          id,
		SessionID:   session.ID,
		ControlPath: controlPath,
		State:       model.ConnectionActive,
		CreatedAt:   time.Now(),
	}
	if err := s.connections.Create(ctx, connection); err != nil {
		_ = conn.Close(ctx)
		return model.Connection{}, err
	}
	s.pool.SetConnection(id, conn)
	s.heartbeat.Beat(id)
	return connection, nil
}

// dialMaster adopts a control master established by the interactive client when
// adopt is true; otherwise it creates a new master. The broker has no terminal,
// so masters it creates itself cannot answer auth prompts.
func (s *SessionService) dialMaster(ctx context.Context, target sshx.Target, controlPath, sessionID string, adopt bool) (sshx.Connection, error) {
	opts := sshx.ConnectOptions{ControlPath: controlPath, SessionID: sessionID}
	if adopt {
		return s.transport.Adopt(ctx, target, opts)
	}
	return s.transport.Connect(ctx, target, opts)
}

func (s *SessionService) List(ctx context.Context) ([]model.Session, error) {
	sessions, err := s.sessions.List(ctx)
	if err != nil {
		return nil, err
	}
	live := sessions[:0]
	for _, session := range sessions {
		if session.State == model.SessionClosed {
			continue
		}
		live = append(live, session)
	}
	return live, nil
}

func (s *SessionService) Get(ctx context.Context, id string) (model.Session, error) {
	return s.sessions.Get(ctx, id)
}

// Connections lists the interactive connections of a session.
func (s *SessionService) Connections(ctx context.Context, id string) ([]model.Connection, error) {
	return s.connections.ListBySession(ctx, id)
}

// ConnectionCount reports how many interactive connections a session has.
func (s *SessionService) ConnectionCount(ctx context.Context, id string) int {
	count, err := s.connections.CountBySession(ctx, id)
	if err != nil {
		return 0
	}
	return count
}

// HeartbeatInterval is the cadence clients should use when beating.
func (s *SessionService) HeartbeatInterval() time.Duration {
	return s.heartbeat.Interval()
}

// IsStale reports whether a connection has missed its heartbeat timeout.
func (s *SessionService) IsStale(connectionID string) bool {
	return s.heartbeat.IsStale(connectionID)
}

// Heartbeat records a beat for a connection of a session.
func (s *SessionService) Heartbeat(ctx context.Context, sessionID, connectionID string) error {
	connection, err := s.connections.Get(ctx, connectionID)
	if err != nil {
		return err
	}
	if connection.SessionID != sessionID {
		return fmt.Errorf("%w: connection does not belong to session", errs.ErrInvalidInput)
	}
	s.heartbeat.Beat(connectionID)
	return nil
}

// RunMonitor reaps connections that stop beating until the context is cancelled.
func (s *SessionService) RunMonitor(ctx context.Context) {
	s.heartbeat.Run(ctx)
}

// Close destroys a session and every connection and tunnel it owns. It reports
// whether this was the last session, so the caller can run the teardown hook
// after responding.
func (s *SessionService) Close(ctx context.Context, id string) ([]model.Tunnel, bool, error) {
	session, err := s.sessions.Get(ctx, id)
	if err != nil {
		return nil, false, err
	}
	dropped, err := s.closeSession(ctx, session)
	if err != nil {
		return dropped, false, err
	}
	return dropped, !s.hasLiveSessions(ctx), nil
}

// Detach removes one connection from its session. When no connections remain
// the session is destroyed and its tunnels are closed.
func (s *SessionService) Detach(ctx context.Context, sessionID, connectionID string) ([]model.Tunnel, bool, error) {
	connection, err := s.connections.Get(ctx, connectionID)
	if err != nil {
		return nil, false, err
	}
	if connection.SessionID != sessionID {
		return nil, false, fmt.Errorf("%w: connection does not belong to session", errs.ErrInvalidInput)
	}
	return s.removeConnection(ctx, connection)
}

// expireConnection removes a connection that stopped heartbeating. It is safe
// to race with Detach: whichever deletes the record first wins and the other is
// a no-op.
func (s *SessionService) expireConnection(ctx context.Context, connectionID string) {
	connection, err := s.connections.Get(ctx, connectionID)
	if err != nil {
		return
	}
	_, _, _ = s.removeConnection(ctx, connection)
}

// removeConnection deletes a connection and, when it was the session's last,
// destroys the session and reports whether the broker can shut down.
func (s *SessionService) removeConnection(ctx context.Context, connection model.Connection) ([]model.Tunnel, bool, error) {
	if err := s.connections.Delete(ctx, connection.ID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	s.heartbeat.Forget(connection.ID)
	if conn, ok := s.pool.Connection(connection.ID); ok {
		// The first connection of a session may share the controller's control
		// master (the interactive client established one master for both).
		// Closing it here would tear down the session's controller and tunnels;
		// closeSession closes it when the last connection goes away.
		if !s.sharesControllerMaster(connection.SessionID, connection.ControlPath) {
			_ = conn.Close(ctx)
		}
		s.pool.RemoveConnection(connection.ID)
	}

	if count, _ := s.connections.CountBySession(ctx, connection.SessionID); count > 0 {
		return nil, false, nil
	}

	session, err := s.sessions.Get(ctx, connection.SessionID)
	if err != nil {
		return nil, true, err
	}
	dropped, err := s.closeSession(ctx, session)
	if err != nil {
		return dropped, true, err
	}
	return dropped, !s.hasLiveSessions(ctx), nil
}

// sharesControllerMaster reports whether a connection's control path is the
// session controller's master. When it is, closing the connection must not
// close the master; session teardown owns it.
func (s *SessionService) sharesControllerMaster(sessionID, controlPath string) bool {
	if controlPath == "" {
		return false
	}
	ctrl, ok := s.pool.Controller(sessionID)
	if !ok {
		return false
	}
	return ctrl.ControlPath() == controlPath
}

// NotifyTeardown runs the teardown hook registered with SetTeardownHook. It is
// called by the API after the detach response has been written.
func (s *SessionService) NotifyTeardown() {
	s.mu.Lock()
	hook := s.teardown
	s.mu.Unlock()
	if hook != nil {
		hook()
	}
}

// hasLiveSessions reports whether any session is still active.
func (s *SessionService) hasLiveSessions(ctx context.Context) bool {
	sessions, err := s.sessions.List(ctx)
	if err != nil {
		return true
	}
	for _, session := range sessions {
		if session.State != model.SessionClosed {
			return true
		}
	}
	return false
}

func (s *SessionService) closeSession(ctx context.Context, session model.Session) ([]model.Tunnel, error) {
	dropped, err := s.tunnels.ListBySession(ctx, session.ID)
	if err != nil {
		return nil, err
	}

	if conn, ok := s.pool.Controller(session.ID); ok {
		if paths, perr := s.installer.RemotePaths(ctx, conn); perr == nil {
			_ = s.installer.Remove(ctx, conn, paths, sessionfile.File{ID: session.ID})
		}
		_ = conn.Close(ctx)
		s.pool.RemoveController(session.ID)
	}

	if connections, cerr := s.connections.ListBySession(ctx, session.ID); cerr == nil {
		for _, connection := range connections {
			s.heartbeat.Forget(connection.ID)
			if conn, ok := s.pool.Connection(connection.ID); ok {
				_ = conn.Close(ctx)
				s.pool.RemoveConnection(connection.ID)
			}
		}
	}
	_ = s.connections.DeleteBySession(ctx, session.ID)

	if listener, ok := s.listener(session.ID); ok {
		_ = listener.Close()
		s.removeListener(session.ID)
	}

	s.auth.Revoke(session.Token)
	if s.sessionDir != "" {
		_ = sessionfile.RemoveLocal(s.sessionDir, session.ID)
	}
	_ = s.tunnels.DeleteBySession(ctx, session.ID)

	session.State = model.SessionClosed
	session.ClosedAt = time.Now()
	_ = s.sessions.Update(ctx, session)

	return dropped, nil
}

func (s *SessionService) setListener(id string, listener control.Listener) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners[id] = listener
}

func (s *SessionService) listener(id string) (control.Listener, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	listener, ok := s.listeners[id]
	return listener, ok
}

func (s *SessionService) removeListener(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.listeners, id)
}
