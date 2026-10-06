package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/mcnull/sshex/internal/api"
	"github.com/mcnull/sshex/internal/config"
	"github.com/mcnull/sshex/internal/control"
	"github.com/mcnull/sshex/internal/control/unix"
	"github.com/mcnull/sshex/internal/daemon"
	"github.com/mcnull/sshex/internal/local"
	"github.com/mcnull/sshex/internal/model"
	"github.com/mcnull/sshex/internal/remoteinstall"
	configrepo "github.com/mcnull/sshex/internal/repository/config"
	"github.com/mcnull/sshex/internal/repository/memory"
	"github.com/mcnull/sshex/internal/service"
	"github.com/mcnull/sshex/internal/sshx"
	"github.com/mcnull/sshex/internal/sshx/openssh"
)

type App struct {
	Config    config.Config
	Sessions  *service.SessionService
	Tunnels   *service.TunnelService
	Commands  *service.CommandService
	Installs  *service.InstallService
	Auth      *control.Authenticator
	Transport sshx.Transport

	control *unix.Provider
	handler http.Handler
}

func New(cfg config.Config, configPath string) (*App, error) {
	sessionRepo := memory.NewSessionRepository()
	connectionRepo := memory.NewConnectionRepository()
	tunnelRepo := memory.NewTunnelRepository()
	commandRepo := configrepo.NewCommandRepository(configPath)
	transport := openssh.NewTransport()
	installer := remoteinstall.NewLinuxInstaller()
	completions := remoteinstall.NewLinuxCompletionsInstaller()
	aliases := remoteinstall.NewLinuxAliasInstaller()
	auth := control.NewAuthenticator()
	pool := sshx.NewPool()

	runtimeDir, err := config.RuntimeDir()
	if err != nil {
		return nil, err
	}
	controlDir := filepath.Join(runtimeDir, "control")
	sessionDir := filepath.Join(runtimeDir, "sessions")
	controlProvider := unix.NewProvider(controlDir)

	sessions := service.NewSessionService(service.SessionServiceConfig{
		Sessions:          sessionRepo,
		Connections:       connectionRepo,
		Tunnels:           tunnelRepo,
		Commands:          commandRepo,
		Transport:         transport,
		Control:           controlProvider,
		Installer:         installer,
		Completions:       completions,
		Aliases:           aliases,
		Auth:              auth,
		IDs:               newID,
		Pool:              pool,
		SessionDir:        sessionDir,
		ControlDir:        controlDir,
		HeartbeatInterval: cfg.Server.HeartbeatInterval.Duration(),
		HeartbeatTimeout:  cfg.Server.HeartbeatTimeout.Duration(),
	})
	tunnels := service.NewTunnelService(service.TunnelServiceConfig{
		Sessions:  sessionRepo,
		Tunnels:   tunnelRepo,
		Transport: transport,
		IDs:       newID,
		Pool:      pool,
	})
	commands := service.NewCommandService(service.CommandServiceConfig{
		Commands: commandRepo,
		Executor: local.NewExecutor(),
	})
	commands.SetOnChange(sessions.PublishAliases)

	installs := service.NewInstallService(service.InstallServiceConfig{
		Transport:   transport,
		Installer:   installer,
		Completions: completions,
		Aliases:     aliases,
		Probe:       remoteinstall.NewLinuxSessionProbe(),
		Local:       local.NewRunner(),
		StopBroker:  stopBroker,
		IDs:         newID,
		ControlDir:  controlDir,
	})

	handlers := api.NewHandlers(api.Services{
		Sessions: sessions,
		Tunnels:  tunnels,
		Commands: commands,
	}, auth)
	handler := api.NewRouter(handlers)
	sessions.SetHandler(handler)

	return &App{
		Config:    cfg,
		Sessions:  sessions,
		Tunnels:   tunnels,
		Commands:  commands,
		Installs:  installs,
		Auth:      auth,
		Transport: transport,
		control:   controlProvider,
		handler:   handler,
	}, nil
}

func (a *App) Serve(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	a.Sessions.SetTeardownHook(cancel)
	a.Sessions.SetBaseContext(ctx)
	go a.Sessions.RunMonitor(ctx)

	listener, err := a.control.Listen(ctx, "daemon")
	if err != nil {
		return err
	}
	token, err := control.GenerateToken()
	if err != nil {
		_ = listener.Close()
		return err
	}
	a.Auth.Register(token, model.Capabilities{model.CapabilityTunnels}, "")

	if err := daemon.WriteRuntime(daemon.RuntimeInfo{
		PID:      os.Getpid(),
		Endpoint: listener.Endpoint(),
		Token:    token,
	}); err != nil {
		_ = listener.Close()
		return err
	}
	defer func() { _ = daemon.RemoveRuntime() }()

	fmt.Fprintf(os.Stdout, "sshex service listening on %s\n", listener.Endpoint().Address)
	return listener.Serve(ctx, a.handler)
}

func newID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(buf)
}

// NewControlPath reserves a fresh SSH control-master path inside the broker's
// control directory. The interactive client uses it to establish a master in
// the foreground, where authentication and host-key prompts can reach the
// terminal, before handing the path to the broker to adopt.
func (a *App) NewControlPath() (string, error) {
	dir := a.control.Dir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, newID()+".ctl"), nil
}

func stopBroker() {
	if info, err := daemon.ReadRuntime(); err == nil {
		_ = daemon.Stop(info)
	}
}
