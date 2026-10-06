package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/mcnull/sshex/internal/errs"
	"github.com/mcnull/sshex/internal/model"
	"github.com/mcnull/sshex/internal/repository"
	"github.com/mcnull/sshex/internal/repository/memory"
)

type fakeExecutor struct {
	script string
	name   string
	args   []string
	env    []string
	code   int
	err    error
}

func (f *fakeExecutor) Run(_ context.Context, script, name string, args, env []string, stdout, stderr io.Writer) (int, error) {
	f.script = script
	f.name = name
	f.args = args
	f.env = env
	if stdout != nil {
		_, _ = io.WriteString(stdout, "stdout-data")
	}
	if stderr != nil {
		_, _ = io.WriteString(stderr, "stderr-data")
	}
	return f.code, f.err
}

func newCommandService(t *testing.T) (*CommandService, *fakeExecutor, *memory.CommandRepository) {
	t.Helper()
	repo := memory.NewCommandRepository()
	executor := &fakeExecutor{}
	return NewCommandService(CommandServiceConfig{Commands: repo, Executor: executor}), executor, repo
}

func TestCommandServiceExecute(t *testing.T) {
	ctx := context.Background()
	svc, executor, repo := newCommandService(t)
	executor.code = 3
	if err := repo.Create(ctx, model.Command{Name: "code", Command: `code --folder-uri "vscode-remote://ssh-remote+${REMOTE_HOST}"`}); err != nil {
		t.Fatalf("seed command: %v", err)
	}

	var stdout, stderr, notice bytes.Buffer
	session := model.Session{ID: "s1", User: "bob", Host: "ssh01", Port: 2222, Origin: "laptop"}
	code, err := svc.Execute(ctx, session, ExecRequest{
		SessionID: "s1", Name: "code", Args: []string{"/path"}, Cwd: "/home/bob", User: "fallback",
	}, ExecOutput{Command: &notice, Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if code != 3 {
		t.Fatalf("Execute() code = %d, want 3", code)
	}
	if got := notice.String(); got != `code --folder-uri "vscode-remote://ssh-remote+ssh01"` {
		t.Fatalf("Execute() notice = %q, want rendered command", got)
	}
	if executor.name != "code" || executor.script != `code --folder-uri "vscode-remote://ssh-remote+${REMOTE_HOST}"` {
		t.Fatalf("executor got name=%q script=%q", executor.name, executor.script)
	}
	if len(executor.args) != 1 || executor.args[0] != "/path" {
		t.Fatalf("executor args = %v, want [/path]", executor.args)
	}
	env := strings.Join(executor.env, "\n")
	for _, want := range []string{"REMOTE_CWD=/home/bob", "REMOTE_USER=bob", "REMOTE_HOST=ssh01", "REMOTE_PORT=2222", "REMOTE_SESSION=s1", "REMOTE_ORIGIN=laptop"} {
		if !strings.Contains(env, want) {
			t.Fatalf("executor env missing %q:\n%s", want, env)
		}
	}
	if stdout.String() != "stdout-data" || stderr.String() != "stderr-data" {
		t.Fatalf("output = %q / %q", stdout.String(), stderr.String())
	}
}

func TestCommandServiceExecuteDisabled(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := newCommandService(t)
	if err := repo.Create(ctx, model.Command{Name: "code", Command: "code", Disabled: true}); err != nil {
		t.Fatalf("seed command: %v", err)
	}
	if _, err := svc.Execute(ctx, model.Session{}, ExecRequest{Name: "code"}, ExecOutput{Stdout: io.Discard, Stderr: io.Discard}); !errors.Is(err, errs.ErrDisabled) {
		t.Fatalf("Execute() error = %v, want ErrDisabled", err)
	}
}

func TestCommandServiceExecuteMissing(t *testing.T) {
	svc, _, _ := newCommandService(t)
	if _, err := svc.Execute(context.Background(), model.Session{}, ExecRequest{Name: "nope"}, ExecOutput{Stdout: io.Discard, Stderr: io.Discard}); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Execute() error = %v, want ErrNotFound", err)
	}
}

func TestCommandServiceAddValidation(t *testing.T) {
	svc, _, _ := newCommandService(t)
	ctx := context.Background()
	if err := svc.Add(ctx, "", "cmd", "", false); !errors.Is(err, errs.ErrInvalidInput) {
		t.Fatalf("Add(empty name) error = %v, want ErrInvalidInput", err)
	}
	if err := svc.Add(ctx, "bad name", "cmd", "", false); !errors.Is(err, errs.ErrInvalidInput) {
		t.Fatalf("Add(bad name) error = %v, want ErrInvalidInput", err)
	}
	if err := svc.Add(ctx, "code", "  ", "", false); !errors.Is(err, errs.ErrInvalidInput) {
		t.Fatalf("Add(empty command) error = %v, want ErrInvalidInput", err)
	}
	if err := svc.Add(ctx, "code", "cmd", "bad alias", false); !errors.Is(err, errs.ErrInvalidInput) {
		t.Fatalf("Add(bad alias) error = %v, want ErrInvalidInput", err)
	}
	if err := svc.Add(ctx, "code", `code --folder-uri "vscode-remote://ssh-remote+${REMOTE_HOST}"`, "code", false); err != nil {
		t.Fatalf("Add() error: %v", err)
	}
	if err := svc.Add(ctx, "code", `code --folder-uri "vscode-remote://ssh-remote+${REMOTE_HOST}"`, "code", false); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("Add(duplicate) error = %v, want ErrConflict", err)
	}
}

func TestCommandServiceAliasConflict(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := newCommandService(t)
	if err := svc.Add(ctx, "code", "code", "ed", false); err != nil {
		t.Fatalf("Add(code) error: %v", err)
	}
	if err := svc.Add(ctx, "zed", "zed", "ed", false); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("Add(duplicate alias) error = %v, want ErrConflict", err)
	}
}

func TestCommandServiceUpdate(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := newCommandService(t)
	if err := svc.Add(ctx, "code", "code", "", false); err != nil {
		t.Fatalf("Add() error: %v", err)
	}
	if err := svc.Update(ctx, "code", model.Command{Name: "editor", Command: "editor", Alias: "ed"}); err != nil {
		t.Fatalf("Update(rename) error: %v", err)
	}
	if _, err := repo.Get(ctx, "code"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Get(old) error = %v, want ErrNotFound", err)
	}
	updated, err := repo.Get(ctx, "editor")
	if err != nil {
		t.Fatalf("Get(new) error: %v", err)
	}
	if updated.Alias != "ed" || updated.Command != "editor" {
		t.Fatalf("updated = %+v", updated)
	}
	if err := svc.Update(ctx, "editor", model.Command{Name: "editor", Command: "  "}); !errors.Is(err, errs.ErrInvalidInput) {
		t.Fatalf("Update(empty command) error = %v, want ErrInvalidInput", err)
	}
}

func TestCommandServiceUpdateClearsAlias(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := newCommandService(t)
	if err := svc.Add(ctx, "code", "code", "ed", false); err != nil {
		t.Fatalf("Add() error: %v", err)
	}
	if err := svc.Update(ctx, "code", model.Command{Name: "code", Command: "code"}); err != nil {
		t.Fatalf("Update(clear alias) error: %v", err)
	}
	updated, err := repo.Get(ctx, "code")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if updated.Alias != "" {
		t.Fatalf("alias = %q, want empty", updated.Alias)
	}
	if err := svc.Update(ctx, "code", model.Command{Name: "", Command: "code"}); !errors.Is(err, errs.ErrInvalidInput) {
		t.Fatalf("Update(empty name) error = %v, want ErrInvalidInput", err)
	}
}

func TestCommandServiceOnChange(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewCommandRepository()
	calls := 0
	svc := NewCommandService(CommandServiceConfig{Commands: repo, Executor: &fakeExecutor{}, OnChange: func(context.Context) { calls++ }})
	if err := svc.Add(ctx, "code", "code", "", false); err != nil {
		t.Fatalf("Add() error: %v", err)
	}
	if err := svc.SetDisabled(ctx, "code", true); err != nil {
		t.Fatalf("SetDisabled() error: %v", err)
	}
	if err := svc.Remove(ctx, "code"); err != nil {
		t.Fatalf("Remove() error: %v", err)
	}
	if calls != 3 {
		t.Fatalf("OnChange calls = %d, want 3", calls)
	}
}

func TestCommandServiceSetDisabled(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := newCommandService(t)
	if err := repo.Create(ctx, model.Command{Name: "code", Command: "code"}); err != nil {
		t.Fatalf("seed command: %v", err)
	}
	if err := svc.SetDisabled(ctx, "code", true); err != nil {
		t.Fatalf("SetDisabled() error: %v", err)
	}
	command, _ := repo.Get(ctx, "code")
	if !command.Disabled {
		t.Fatal("SetDisabled(true) left command enabled")
	}
	if err := svc.SetDisabled(ctx, "code", false); err != nil {
		t.Fatalf("SetDisabled() error: %v", err)
	}
	command, _ = repo.Get(ctx, "code")
	if command.Disabled {
		t.Fatal("SetDisabled(false) left command disabled")
	}
	if err := svc.SetDisabled(ctx, "missing", true); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("SetDisabled(missing) error = %v, want ErrNotFound", err)
	}
}
