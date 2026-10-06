package service

import (
	"context"
	"io"

	"github.com/mcnull/sshex/internal/control"
	"github.com/mcnull/sshex/internal/errs"
	"github.com/mcnull/sshex/internal/model"
	"github.com/mcnull/sshex/internal/remoteinstall"
	"github.com/mcnull/sshex/internal/repository"
	"github.com/mcnull/sshex/internal/sshx"
)

var ErrNotImplemented = errs.ErrNotImplemented

type (
	SessionStore    = repository.SessionRepository
	ConnectionStore = repository.ConnectionRepository
	TunnelStore     = repository.TunnelRepository
	CommandStore    = repository.CommandRepository
	Transport       = sshx.Transport
	ControlProvider = control.Provider
	Installer       = remoteinstall.Installer
	Completer       = remoteinstall.CompletionsInstaller
	AliasInstaller  = remoteinstall.AliasInstaller
	Capability      = model.Capability
)

type Auth interface {
	Register(token string, capabilities model.Capabilities, sessionID string)
	Revoke(token string)
}

// CommandExecutor runs a predefined command on the origin machine. The script
// is interpreted by the origin's shell with name as $0, args as the positional
// parameters and env appended to the process environment.
type CommandExecutor interface {
	Run(ctx context.Context, script, name string, args, env []string, stdout, stderr io.Writer) (int, error)
}

type IDGenerator func() string

type PortChecker func(port int) (bool, error)
