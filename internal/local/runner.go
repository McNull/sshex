package local

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Runner executes shell commands on the local machine, mirroring the command
// execution surface of a remote SSH connection.
type Runner struct {
	shell string
}

func NewRunner() *Runner {
	return &Runner{shell: "sh"}
}

func (r *Runner) Run(ctx context.Context, command string, stdin io.Reader, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, r.shell, "-c", command)
	cmd.Stdin = stdin
	cmd.Stdout = stdout

	var errBuf bytes.Buffer
	if stderr != nil {
		cmd.Stderr = stderr
	} else {
		cmd.Stderr = &errBuf
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("local command failed: %w: %s", err, strings.TrimSpace(errBuf.String()))
	}
	return nil
}
