package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/mcnull/sshex/internal/errs"
	"github.com/mcnull/sshex/internal/model"
)

// ExecRequest describes a request to run a predefined command on the origin.
type ExecRequest struct {
	SessionID string
	Name      string
	Args      []string
	Cwd       string
	User      string
}

type CommandServiceConfig struct {
	Commands CommandStore
	Executor CommandExecutor
	// OnChange, when set, is invoked after every mutation so the broker can
	// republish shell aliases to live sessions.
	OnChange func(context.Context)
}

type CommandService struct {
	commands CommandStore
	executor CommandExecutor
	onChange func(context.Context)
}

func NewCommandService(cfg CommandServiceConfig) *CommandService {
	return &CommandService{commands: cfg.Commands, executor: cfg.Executor, onChange: cfg.OnChange}
}

// SetOnChange registers a callback invoked after every mutation.
func (s *CommandService) SetOnChange(fn func(context.Context)) {
	s.onChange = fn
}

func (s *CommandService) changed(ctx context.Context) {
	if s.onChange != nil {
		s.onChange(ctx)
	}
}

// ExecOutput receives the rendered command line and the command's output.
type ExecOutput struct {
	Command io.Writer
	Stdout  io.Writer
	Stderr  io.Writer
}

// Execute looks up the predefined command, checks it is enabled, expands the
// remote context into the environment and runs it on the origin. It returns the
// command's exit code. A non-zero exit code is not an error.
func (s *CommandService) Execute(ctx context.Context, session model.Session, req ExecRequest, out ExecOutput) (int, error) {
	command, err := s.commands.Get(ctx, req.Name)
	if err != nil {
		return -1, err
	}
	if command.Disabled {
		return -1, fmt.Errorf("%w: %q", errs.ErrDisabled, command.Name)
	}
	env := remoteEnv(session, req)
	if out.Command != nil {
		_, _ = io.WriteString(out.Command, RenderCommand(command.Command, env, command.Name, req.Args))
	}
	return s.executor.Run(ctx, command.Command, command.Name, req.Args, env, out.Stdout, out.Stderr)
}

// remoteEnv exposes the remote context to the command template. The shell
// expands ${REMOTE_CWD} and friends from the environment, and ${@} and ${n}
// from the positional parameters.
func remoteEnv(session model.Session, req ExecRequest) []string {
	user := session.User
	if user == "" {
		user = req.User
	}
	port := session.Port
	if port == 0 {
		port = 22
	}
	return []string{
		"REMOTE_CWD=" + req.Cwd,
		"REMOTE_USER=" + user,
		"REMOTE_HOST=" + session.Host,
		"REMOTE_PORT=" + strconv.Itoa(port),
		"REMOTE_SESSION=" + session.ID,
		"REMOTE_ORIGIN=" + session.Origin,
	}
}

func (s *CommandService) List(ctx context.Context) ([]model.Command, error) {
	return s.commands.List(ctx)
}

func (s *CommandService) Get(ctx context.Context, name string) (model.Command, error) {
	return s.commands.Get(ctx, name)
}

func (s *CommandService) Add(ctx context.Context, name, command, alias string, disabled bool) error {
	if err := wrapInvalid(ValidateName(name)); err != nil {
		return err
	}
	if err := wrapInvalid(ValidateAlias(alias)); err != nil {
		return err
	}
	if strings.TrimSpace(command) == "" {
		return fmt.Errorf("%w: command is required", errs.ErrInvalidInput)
	}
	if err := s.commands.Create(ctx, model.Command{Name: name, Command: command, Alias: alias, Disabled: disabled}); err != nil {
		return err
	}
	s.changed(ctx)
	return nil
}

// Update replaces the command stored under name. The replacement may rename the
// command when its Name differs from name.
func (s *CommandService) Update(ctx context.Context, name string, command model.Command) error {
	if err := wrapInvalid(ValidateName(command.Name)); err != nil {
		return err
	}
	if err := wrapInvalid(ValidateAlias(command.Alias)); err != nil {
		return err
	}
	if strings.TrimSpace(command.Command) == "" {
		return fmt.Errorf("%w: command is required", errs.ErrInvalidInput)
	}
	if err := s.commands.Update(ctx, name, command); err != nil {
		return err
	}
	s.changed(ctx)
	return nil
}

func (s *CommandService) Remove(ctx context.Context, name string) error {
	if err := s.commands.Delete(ctx, name); err != nil {
		return err
	}
	s.changed(ctx)
	return nil
}

func (s *CommandService) SetDisabled(ctx context.Context, name string, disabled bool) error {
	command, err := s.commands.Get(ctx, name)
	if err != nil {
		return err
	}
	command.Disabled = disabled
	return s.Update(ctx, name, command)
}

// wrapInvalid marks a validation failure as invalid input so callers and the
// API can classify it. A nil error passes through unchanged.
func wrapInvalid(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", errs.ErrInvalidInput, err)
}

// ValidateName reports whether name is a valid command name. It returns a
// user-facing error so interactive callers can show it directly.
func ValidateName(name string) error {
	if name == "" {
		return errors.New("name is required")
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
		default:
			return fmt.Errorf("invalid command name %q", name)
		}
	}
	return nil
}

// ValidateAlias accepts an empty alias (no alias) or a shell-safe name. It
// deliberately allows the reserved characters a shell alias may use.
func ValidateAlias(alias string) error {
	if alias == "" {
		return nil
	}
	for _, r := range alias {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
		default:
			return fmt.Errorf("invalid alias %q", alias)
		}
	}
	return nil
}
