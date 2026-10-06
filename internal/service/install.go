package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcnull/sshex/internal/errs"
	"github.com/mcnull/sshex/internal/remoteinstall"
	"github.com/mcnull/sshex/internal/sessionfile"
	"github.com/mcnull/sshex/internal/sshx"
)

type UninstallRequest struct {
	User         string
	Host         string
	Port         int
	IdentityFile string
	JumpHosts    string
	Options      []string
	Force        bool
}

// SessionProbe lists the sessions that are still active on a target machine.
type SessionProbe interface {
	Active(ctx context.Context, run remoteinstall.Runner, paths remoteinstall.RemotePaths) ([]sessionfile.File, error)
}

// ActiveSessionsError reports that sessions are still running, so uninstall was
// refused unless forced.
type ActiveSessionsError struct {
	Sessions []sessionfile.File
}

func (e *ActiveSessionsError) Error() string {
	return fmt.Sprintf("%d active sshex session(s)", len(e.Sessions))
}

func (e *ActiveSessionsError) Unwrap() error { return errs.ErrActiveSessions }

type InstallServiceConfig struct {
	Transport   Transport
	Installer   Installer
	Completions Completer
	Aliases     AliasInstaller
	Probe       SessionProbe
	Local       remoteinstall.Runner
	StopBroker  func()
	IDs         IDGenerator
	ControlDir  string
}

type InstallService struct {
	transport   Transport
	installer   Installer
	completions Completer
	aliases     AliasInstaller
	probe       SessionProbe
	local       remoteinstall.Runner
	stopBroker  func()
	ids         IDGenerator
	controlDir  string
}

func NewInstallService(cfg InstallServiceConfig) *InstallService {
	return &InstallService{
		transport:   cfg.Transport,
		installer:   cfg.Installer,
		completions: cfg.Completions,
		aliases:     cfg.Aliases,
		probe:       cfg.Probe,
		local:       cfg.Local,
		stopBroker:  cfg.StopBroker,
		ids:         cfg.IDs,
		controlDir:  cfg.ControlDir,
	}
}

// Uninstall removes the sshex installation from the requested host. An empty
// host targets the local machine.
func (s *InstallService) Uninstall(ctx context.Context, req UninstallRequest) error {
	if req.Host == "" && req.JumpHosts != "" {
		return fmt.Errorf("%w: jump hosts require a host", errs.ErrInvalidInput)
	}
	run, closeFn, err := s.runner(ctx, req)
	if err != nil {
		return err
	}
	defer closeFn()

	paths, err := s.installer.RemotePaths(ctx, run)
	if err != nil {
		return err
	}

	if !req.Force {
		active, err := s.activeSessions(ctx, req, run, paths)
		if err != nil {
			return err
		}
		if len(active) > 0 {
			return &ActiveSessionsError{Sessions: active}
		}
	}

	if s.completions != nil {
		if err := s.completions.Uninstall(ctx, run, paths); err != nil {
			return err
		}
	}
	if s.aliases != nil {
		if err := s.aliases.Uninstall(ctx, run, paths); err != nil {
			return err
		}
	}
	if err := s.installer.Uninstall(ctx, run, paths); err != nil {
		return err
	}
	if req.Host == "" && s.stopBroker != nil {
		s.stopBroker()
	}
	return nil
}

// activeSessions returns the sessions that are still running on the machine
// being uninstalled, excluding the session that invoked the command.
func (s *InstallService) activeSessions(ctx context.Context, req UninstallRequest, run remoteinstall.Runner, paths remoteinstall.RemotePaths) ([]sessionfile.File, error) {
	exclude := os.Getenv("SSHEX_SESSION")
	seen := make(map[string]bool)
	var active []sessionfile.File
	add := func(f sessionfile.File) {
		if f.ID == exclude || seen[f.ID] {
			return
		}
		if req.Host != "" && !sameHost(f.Target, req.Host) {
			return
		}
		seen[f.ID] = true
		active = append(active, f)
	}

	local, err := sessionfile.ListActive()
	if err != nil {
		return nil, err
	}
	for _, f := range local {
		add(f)
	}

	if req.Host != "" && s.probe != nil {
		remote, err := s.probe.Active(ctx, run, paths)
		if err != nil {
			return nil, err
		}
		for _, f := range remote {
			add(f)
		}
	}
	return active, nil
}

func sameHost(target, host string) bool {
	if target == "" {
		return false
	}
	if idx := strings.LastIndex(target, "@"); idx >= 0 {
		target = target[idx+1:]
	}
	return target == host
}

func (s *InstallService) runner(ctx context.Context, req UninstallRequest) (remoteinstall.Runner, func(), error) {
	if req.Host == "" {
		if s.local == nil {
			return nil, nil, fmt.Errorf("local uninstall is not available")
		}
		return s.local, func() {}, nil
	}
	target := sshx.Target{
		User:         req.User,
		Host:         req.Host,
		Port:         req.Port,
		IdentityFile: req.IdentityFile,
		JumpHosts:    req.JumpHosts,
		Options:      req.Options,
	}
	conn, err := s.transport.Connect(ctx, target, sshx.ConnectOptions{
		ControlPath: filepath.Join(s.controlDir, s.ids()+".ctl"),
	})
	if err != nil {
		return nil, nil, err
	}
	return conn, func() { _ = conn.Close(context.Background()) }, nil
}
