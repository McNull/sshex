package configrepo

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/mcnull/sshex/internal/config"
	"github.com/mcnull/sshex/internal/model"
	"github.com/mcnull/sshex/internal/repository"
)

func newRepo(t *testing.T) (*CommandRepository, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := config.Save(path, config.Default()); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	return NewCommandRepository(path), path
}

func TestCommandRepositoryCRUD(t *testing.T) {
	ctx := context.Background()
	repo, path := newRepo(t)

	if _, err := repo.Get(ctx, "code"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Get(missing) error = %v, want ErrNotFound", err)
	}

	if err := repo.Create(ctx, model.Command{Name: "code", Command: `code --folder-uri "vscode-remote://ssh-remote+${REMOTE_HOST}"`}); err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := repo.Create(ctx, model.Command{Name: "code", Command: "other"}); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("Create(duplicate) error = %v, want ErrConflict", err)
	}

	command, err := repo.Get(ctx, "code")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if command.Command != `code --folder-uri "vscode-remote://ssh-remote+${REMOTE_HOST}"` || command.Disabled {
		t.Fatalf("Get() = %+v", command)
	}

	if err := repo.Update(ctx, "code", model.Command{Name: "code", Command: `code --folder-uri "vscode-remote://ssh-remote+${REMOTE_HOST}"`, Disabled: true}); err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	command, _ = repo.Get(ctx, "code")
	if !command.Disabled {
		t.Fatal("Update() did not persist disabled")
	}
	if err := repo.Update(ctx, "missing", model.Command{Name: "missing"}); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Update(missing) error = %v, want ErrNotFound", err)
	}

	commands, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(commands) != 1 || commands[0].Name != "code" {
		t.Fatalf("List() = %+v", commands)
	}

	if err := repo.Delete(ctx, "code"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
	if err := repo.Delete(ctx, "code"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Delete(missing) error = %v, want ErrNotFound", err)
	}

	// The repository must preserve the other config sections on every save.
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Server.Transport != "unix" {
		t.Fatalf("server transport = %q, want unix", cfg.Server.Transport)
	}
}

func TestCommandRepositoryAliasAndRename(t *testing.T) {
	ctx := context.Background()
	repo, _ := newRepo(t)

	if err := repo.Create(ctx, model.Command{Name: "code", Command: "code", Alias: "ed"}); err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := repo.Create(ctx, model.Command{Name: "zed", Command: "zed", Alias: "ed"}); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("Create(duplicate alias) error = %v, want ErrConflict", err)
	}

	// Rename code -> editor, keeping its alias.
	if err := repo.Update(ctx, "code", model.Command{Name: "editor", Command: "code", Alias: "ed"}); err != nil {
		t.Fatalf("Update(rename) error: %v", err)
	}
	if _, err := repo.Get(ctx, "code"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Get(old) error = %v, want ErrNotFound", err)
	}
	updated, err := repo.Get(ctx, "editor")
	if err != nil {
		t.Fatalf("Get(renamed) error: %v", err)
	}
	if updated.Alias != "ed" {
		t.Fatalf("renamed alias = %q, want ed", updated.Alias)
	}

	// Another command may not take a used alias.
	if err := repo.Create(ctx, model.Command{Name: "zed", Command: "zed"}); err != nil {
		t.Fatalf("Create(zed) error: %v", err)
	}
	if err := repo.Update(ctx, "zed", model.Command{Name: "zed", Command: "zed", Alias: "ed"}); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("Update(alias in use) error = %v, want ErrConflict", err)
	}

	// A rename onto an existing name conflicts.
	if err := repo.Update(ctx, "zed", model.Command{Name: "editor", Command: "zed"}); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("Update(rename conflict) error = %v, want ErrConflict", err)
	}
}

func TestCommandRepositoryReloadsFile(t *testing.T) {
	ctx := context.Background()
	repo, path := newRepo(t)

	if err := repo.Create(ctx, model.Command{Name: "code", Command: "code"}); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	// A second repository instance for the same file sees the first's write.
	other := NewCommandRepository(path)
	if _, err := other.Get(ctx, "code"); err != nil {
		t.Fatalf("second repo Get() error: %v", err)
	}
}
