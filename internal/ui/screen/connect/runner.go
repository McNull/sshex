package connect

import (
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Runner starts a session for a target. It returns a command that suspends the
// UI while the session runs and reports the result as FinishedMsg.
type Runner interface {
	Run(target string) tea.Cmd
}

// ExecRunner starts a session by running this sshex binary as
// `sshex connect <target>` through tea.ExecProcess.
type ExecRunner struct{}

// NewExecRunner returns the production runner.
func NewExecRunner() Runner { return ExecRunner{} }

const noBannerEnv = "SSHEX_NO_BANNER=1"

// Run implements Runner.
func (ExecRunner) Run(target string) tea.Cmd {
	cmd, err := connectCommand(target)
	if err != nil {
		return func() tea.Msg { return FinishedMsg{Err: err} }
	}

	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return FinishedMsg{Err: err}
	})
}

func connectCommand(target string) (*exec.Cmd, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(executable, "connect", target)
	// The UI already brands the session; a nested banner would clutter the
	// terminal before the shell starts. The Server menu always starts a local
	// origin connection, so the outer client session must not leak into it.
	cmd.Env = append(withoutSession(os.Environ()), noBannerEnv)
	return cmd, nil
}

func withoutSession(env []string) []string {
	out := make([]string, 0, len(env))
	for _, entry := range env {
		if strings.HasPrefix(entry, "SSHEX_SESSION=") {
			continue
		}
		out = append(out, entry)
	}
	return out
}
