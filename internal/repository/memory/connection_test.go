package memory

import (
	"context"
	"errors"
	"testing"

	"github.com/mcnull/sshex/internal/model"
	"github.com/mcnull/sshex/internal/repository"
)

func TestConnectionRepositoryLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := NewConnectionRepository()
	connection := model.Connection{ID: "c1", SessionID: "s1", ControlPath: "/tmp/c1.ctl"}

	if err := repo.Create(ctx, connection); err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := repo.Create(ctx, connection); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("Create() duplicate error = %v, want ErrConflict", err)
	}

	got, err := repo.Get(ctx, "c1")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got.SessionID != "s1" {
		t.Fatalf("Get() = %+v, want session s1", got)
	}

	if err := repo.Create(ctx, model.Connection{ID: "c2", SessionID: "s1"}); err != nil {
		t.Fatalf("Create() second error: %v", err)
	}
	if err := repo.Create(ctx, model.Connection{ID: "c3", SessionID: "s2"}); err != nil {
		t.Fatalf("Create() third error: %v", err)
	}

	list, err := repo.ListBySession(ctx, "s1")
	if err != nil {
		t.Fatalf("ListBySession() error: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("ListBySession(s1) len = %d, want 2", len(list))
	}

	count, err := repo.CountBySession(ctx, "s1")
	if err != nil {
		t.Fatalf("CountBySession() error: %v", err)
	}
	if count != 2 {
		t.Fatalf("CountBySession(s1) = %d, want 2", count)
	}

	if err := repo.Delete(ctx, "c1"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
	if err := repo.Delete(ctx, "c1"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Delete() missing error = %v, want ErrNotFound", err)
	}

	if err := repo.DeleteBySession(ctx, "s1"); err != nil {
		t.Fatalf("DeleteBySession() error: %v", err)
	}
	if count, _ := repo.CountBySession(ctx, "s1"); count != 0 {
		t.Fatalf("CountBySession(s1) after delete = %d, want 0", count)
	}
	if count, _ := repo.CountBySession(ctx, "s2"); count != 1 {
		t.Fatalf("CountBySession(s2) = %d, want 1", count)
	}
}
