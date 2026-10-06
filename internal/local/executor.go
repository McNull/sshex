package local

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
)

// Executor runs a predefined command on the local machine through the shell.
// The script is interpreted by the shell, name becomes $0, args become the
// positional parameters and env is appended to the process environment.
type Executor struct {
	shell string
}

func NewExecutor() *Executor {
	return &Executor{shell: "sh"}
}

func (e *Executor) Run(ctx context.Context, script, name string, args, env []string, stdout, stderr io.Writer) (int, error) {
	shellArgs := make([]string, 0, len(args)+3)
	shellArgs = append(shellArgs, "-c", script, name)
	shellArgs = append(shellArgs, args...)

	cmd := exec.CommandContext(ctx, e.shell, shellArgs...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return -1, err
	}
	return 0, nil
}
