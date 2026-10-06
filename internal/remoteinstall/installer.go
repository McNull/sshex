package remoteinstall

import (
	"context"
	"io"

	"github.com/mcnull/sshex/internal/sessionfile"
)

// Runner executes a shell command either on a remote host (over SSH) or on the
// local machine. Both sshx.Connection and the local runner satisfy it.
type Runner interface {
	Run(ctx context.Context, command string, stdin io.Reader, stdout, stderr io.Writer) error
}

type Installer interface {
	RemotePaths(ctx context.Context, run Runner) (RemotePaths, error)
	Install(ctx context.Context, run Runner, paths RemotePaths) error
	WriteSession(ctx context.Context, run Runner, paths RemotePaths, file sessionfile.File) error
	Remove(ctx context.Context, run Runner, paths RemotePaths, file sessionfile.File) error
	Uninstall(ctx context.Context, run Runner, paths RemotePaths) error
}
