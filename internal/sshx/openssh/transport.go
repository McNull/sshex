package openssh

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mcnull/sshex/internal/sshx"
	"github.com/mcnull/sshex/internal/verbose"
)

const controlDirMode = 0o700

const defaultConnectTimeout = "10"

type Transport struct {
	sshPath string
	verbose bool
}

func NewTransport() *Transport {
	return &Transport{sshPath: "ssh", verbose: verbose.ShouldLog()}
}

func (t *Transport) Connect(ctx context.Context, target sshx.Target, opts sshx.ConnectOptions) (sshx.Connection, error) {
	if opts.ControlPath == "" {
		return nil, fmt.Errorf("control path is required")
	}
	if err := os.MkdirAll(filepath.Dir(opts.ControlPath), controlDirMode); err != nil {
		return nil, err
	}
	_ = os.Remove(opts.ControlPath)

	args, err := connectArgs(target, opts.ControlPath)
	if err != nil {
		return nil, err
	}
	t.logCommand(args)

	cmd := exec.CommandContext(ctx, t.sshPath, args...)

	// An interactive caller supplies a terminal file so OpenSSH can prompt for
	// host-key confirmation and authentication. Only an *os.File is used
	// directly: a pipe-based capture would block Wait while the `-f` master or a
	// `-J` ProxyJump helper holds the write end. Non-interactive callers capture
	// to a temp file instead, which never blocks Wait for the same reason.
	if term, ok := opts.Stderr.(*os.File); ok && term != nil {
		cmd.Stdout = term
		cmd.Stderr = term
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("establish ssh master: %w", err)
		}
		return &connection{sshPath: t.sshPath, controlPath: opts.ControlPath, target: target, sessionID: opts.SessionID}, nil
	}

	logFile, err := os.CreateTemp("", "sshex-master-*.log")
	if err != nil {
		return nil, err
	}
	defer os.Remove(logFile.Name())
	defer logFile.Close()

	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Run(); err != nil {
		data, _ := os.ReadFile(logFile.Name())
		return nil, fmt.Errorf("establish ssh master: %w: %s", err, strings.TrimSpace(string(data)))
	}

	return &connection{sshPath: t.sshPath, controlPath: opts.ControlPath, target: target, sessionID: opts.SessionID}, nil
}

func connectArgs(target sshx.Target, controlPath string) ([]string, error) {
	args := []string{
		"-M",
		"-S", controlPath,
		"-N",
		"-f",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "ControlMaster=yes",
	}
	extra, err := targetArgs(target)
	if err != nil {
		return nil, err
	}
	args = append(args, extra...)
	if !hasConnectTimeoutOption(target.Options) {
		args = append(args, "-o", "ConnectTimeout="+defaultConnectTimeout)
	}
	args = append(args, target.String())
	return args, nil
}

// hasConnectTimeoutOption reports whether the caller supplied an explicit
// ConnectTimeout through either sshex's key=value option or raw ssh arguments.
func hasConnectTimeoutOption(options []string) bool {
	for _, option := range options {
		if !strings.HasPrefix(option, "-") {
			if isConnectTimeout(option) {
				return true
			}
			continue
		}
		tokens, err := sshx.SplitArgs(option)
		if err != nil {
			continue
		}
		for i := 0; i < len(tokens); i++ {
			token := tokens[i]
			switch {
			case token == "-o" && i+1 < len(tokens):
				i++
				if isConnectTimeout(tokens[i]) {
					return true
				}
			case strings.HasPrefix(token, "-o") && len(token) > 2:
				if isConnectTimeout(token[2:]) {
					return true
				}
			}
		}
	}
	return false
}

func isConnectTimeout(option string) bool {
	key, _, ok := strings.Cut(option, "=")
	return ok && strings.EqualFold(strings.TrimSpace(key), "ConnectTimeout")
}

// Adopt reuses an existing control master established by another process (for
// example the interactive `sshex connect` client). It verifies the master is
// alive before returning a connection bound to it.
func (t *Transport) Adopt(ctx context.Context, target sshx.Target, opts sshx.ConnectOptions) (sshx.Connection, error) {
	if opts.ControlPath == "" {
		return nil, fmt.Errorf("control path is required")
	}
	args := []string{"-S", opts.ControlPath, "-O", "check", target.String()}
	t.logCommand(args)
	out, err := exec.CommandContext(ctx, t.sshPath, args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("adopt ssh master: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return &connection{sshPath: t.sshPath, controlPath: opts.ControlPath, target: target, sessionID: opts.SessionID}, nil
}

type connection struct {
	sshPath     string
	controlPath string
	target      sshx.Target
	sessionID   string
}

// Attach returns a connection that reuses an existing SSH control master. It
// does not establish a new master.
func (t *Transport) Attach(target sshx.Target, opts sshx.ConnectOptions) sshx.Connection {
	return &connection{sshPath: t.sshPath, controlPath: opts.ControlPath, target: target, sessionID: opts.SessionID}
}

func (t *Transport) logCommand(args []string) {
	if !t.verbose {
		return
	}
	logSSHCommand(t.sshPath, args)
}

func (c *connection) ControlPath() string { return c.controlPath }

func (c *connection) Target() sshx.Target { return c.target }

func (c *connection) Run(ctx context.Context, command string, stdin io.Reader, stdout, stderr io.Writer) error {
	args := append(c.baseArgs(), command)
	// The transport owns the verbosity setting; the connection stores the
	// executable and can be created by Attach or Adopt, so use the same env gate.
	if verbose.ShouldLog() {
		logSSHCommand(c.sshPath, args)
	}
	cmd := exec.CommandContext(ctx, c.sshPath, args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout

	var errBuf bytes.Buffer
	if stderr != nil {
		cmd.Stderr = stderr
	} else {
		cmd.Stderr = &errBuf
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("remote command failed: %w: %s", err, strings.TrimSpace(errBuf.String()))
	}
	return nil
}

func (c *connection) AddLocalForward(ctx context.Context, spec sshx.ForwardSpec) error {
	return c.forwardOp(ctx, "forward", "-L", spec.LocalArg())
}

func (c *connection) RemoveLocalForward(ctx context.Context, spec sshx.ForwardSpec) error {
	return c.forwardOp(ctx, "cancel", "-L", spec.LocalArg())
}

func (c *connection) AddRemoteForward(ctx context.Context, spec sshx.ForwardSpec) error {
	return c.forwardOp(ctx, "forward", "-R", spec.LocalArg())
}

func (c *connection) RemoveRemoteForward(ctx context.Context, spec sshx.ForwardSpec) error {
	return c.forwardOp(ctx, "cancel", "-R", spec.LocalArg())
}

func (c *connection) AddRemoteSocketForward(ctx context.Context, spec sshx.RemoteSocketForwardSpec) error {
	return c.forwardOp(ctx, "forward", "-R", spec.Arg())
}

func (c *connection) RemoveRemoteSocketForward(ctx context.Context, spec sshx.RemoteSocketForwardSpec) error {
	return c.forwardOp(ctx, "cancel", "-R", spec.Arg())
}

func (c *connection) Shell(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer) error {
	args := append(c.baseArgs(), "-t")
	if command := shellCommand(c.sessionID); command != "" {
		args = append(args, command)
	}
	if verbose.ShouldLog() {
		logSSHCommand(c.sshPath, args)
	}
	cmd := exec.CommandContext(ctx, c.sshPath, args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	stderrFilter := newSharedConnectionWriter(stderr, c.target.String())
	cmd.Stderr = stderrFilter

	err := cmd.Run()
	flushErr := stderrFilter.Flush()
	if err != nil {
		return fmt.Errorf("ssh session: %w", err)
	}
	return flushErr
}

// shellCommand returns a remote command that exports SSHEX_SESSION for the
// interactive login shell, so remote `sshex` commands can identify which
// session they belong to. It is empty when no session id is known.
func shellCommand(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	return `exec env SSHEX_SESSION=` + shellQuote(sessionID) + ` "${SHELL:-/bin/sh}" -l`
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (c *connection) Close(_ context.Context) error {
	_ = exec.Command(c.sshPath, "-S", c.controlPath, "-O", "exit", c.target.String()).Run()
	_ = os.Remove(c.controlPath)
	return nil
}

func (c *connection) baseArgs() []string {
	return []string{"-S", c.controlPath, c.target.String()}
}

func (c *connection) forwardOp(ctx context.Context, op, flag, spec string) error {
	args := []string{"-S", c.controlPath, "-O", op, flag, spec, c.target.String()}
	if verbose.ShouldLog() {
		logSSHCommand(c.sshPath, args)
	}
	out, err := exec.CommandContext(ctx, c.sshPath, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ssh %s %s: %w: %s", op, flag, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func logSSHCommand(path string, args []string) {
	fmt.Fprintf(os.Stderr, "sshex: verbose: SSH command: %s\n", formatSSHCommand(path, args))
}

func formatSSHCommand(path string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, strconv.Quote(path))
	for _, arg := range args {
		parts = append(parts, strconv.Quote(arg))
	}
	return strings.Join(parts, " ")
}

func targetArgs(target sshx.Target) ([]string, error) {
	var args []string
	if target.Port > 0 && target.Port != 22 {
		args = append(args, "-p", strconv.Itoa(target.Port))
	}
	if target.IdentityFile != "" {
		args = append(args, "-i", target.IdentityFile)
	}
	if target.JumpHosts != "" {
		args = append(args, "-J", target.JumpHosts)
	}
	for _, option := range target.Options {
		if strings.HasPrefix(option, "-") {
			tokens, err := sshx.SplitArgs(option)
			if err != nil {
				return nil, fmt.Errorf("invalid -o value: %w", err)
			}
			args = append(args, tokens...)
			continue
		}
		args = append(args, "-o", option)
	}
	return args, nil
}
