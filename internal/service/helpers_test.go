package service

import (
	"context"
	"io"

	"github.com/mcnull/sshex/internal/control"
	"github.com/mcnull/sshex/internal/control/unix"
	"github.com/mcnull/sshex/internal/local"
	"github.com/mcnull/sshex/internal/remoteinstall"
	"github.com/mcnull/sshex/internal/repository/memory"
	"github.com/mcnull/sshex/internal/sshx"
	"github.com/mcnull/sshex/internal/sshx/openssh"
)

type fakeConn struct {
	addLocalForwards     int
	removeLocalForwards  int
	addRemoteForwards    int
	removeRemoteForwards int
}

func (f *fakeConn) ControlPath() string { return "" }
func (f *fakeConn) Target() sshx.Target { return sshx.Target{} }
func (f *fakeConn) Run(context.Context, string, io.Reader, io.Writer, io.Writer) error {
	return nil
}
func (f *fakeConn) AddLocalForward(context.Context, sshx.ForwardSpec) error {
	f.addLocalForwards++
	return nil
}
func (f *fakeConn) RemoveLocalForward(context.Context, sshx.ForwardSpec) error {
	f.removeLocalForwards++
	return nil
}
func (f *fakeConn) AddRemoteForward(context.Context, sshx.ForwardSpec) error {
	f.addRemoteForwards++
	return nil
}
func (f *fakeConn) RemoveRemoteForward(context.Context, sshx.ForwardSpec) error {
	f.removeRemoteForwards++
	return nil
}
func (f *fakeConn) AddRemoteSocketForward(context.Context, sshx.RemoteSocketForwardSpec) error {
	return nil
}
func (f *fakeConn) RemoveRemoteSocketForward(context.Context, sshx.RemoteSocketForwardSpec) error {
	return nil
}
func (f *fakeConn) Shell(context.Context, io.Reader, io.Writer, io.Writer) error { return nil }
func (f *fakeConn) Close(context.Context) error                                  { return nil }

type testServices struct {
	sessions       *SessionService
	tunnels        *TunnelService
	commands       *CommandService
	auth           *control.Authenticator
	pool           *sshx.Pool
	sessionRepo    *memory.SessionRepository
	connectionRepo *memory.ConnectionRepository
	tunnelRepo     *memory.TunnelRepository
}

func newTestServices() testServices {
	return newTestServicesWithPorts(nil)
}

func newTestServicesWithPorts(ports PortChecker) testServices {
	sessionRepo := memory.NewSessionRepository()
	connectionRepo := memory.NewConnectionRepository()
	tunnelRepo := memory.NewTunnelRepository()
	transport := openssh.NewTransport()
	installer := remoteinstall.NewLinuxInstaller()
	auth := control.NewAuthenticator()
	provider := unix.NewProvider("")
	pool := sshx.NewPool()
	ids := func() string { return "test-id" }

	return testServices{
		sessions: NewSessionService(SessionServiceConfig{
			Sessions:    sessionRepo,
			Connections: connectionRepo,
			Tunnels:     tunnelRepo,
			Transport:   transport,
			Control:     provider,
			Installer:   installer,
			Auth:        auth,
			IDs:         ids,
			Pool:        pool,
		}),
		tunnels: NewTunnelService(TunnelServiceConfig{
			Sessions:  sessionRepo,
			Tunnels:   tunnelRepo,
			Transport: transport,
			IDs:       ids,
			Ports:     ports,
			Pool:      pool,
		}),
		commands: NewCommandService(CommandServiceConfig{
			Commands: memory.NewCommandRepository(),
			Executor: local.NewExecutor(),
		}),
		auth:           auth,
		pool:           pool,
		sessionRepo:    sessionRepo,
		connectionRepo: connectionRepo,
		tunnelRepo:     tunnelRepo,
	}
}
