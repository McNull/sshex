package memory

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/mcnull/sshex/internal/model"
	"github.com/mcnull/sshex/internal/repository"
)

func TestSessionRepositoryLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := NewSessionRepository()
	session := model.Session{ID: "s1", User: "u", Host: "h"}

	if err := repo.Create(ctx, session); err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := repo.Create(ctx, session); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("Create() duplicate error = %v, want ErrConflict", err)
	}

	got, err := repo.Get(ctx, "s1")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got.ID != "s1" {
		t.Fatalf("Get() = %+v, want id s1", got)
	}

	session.Host = "h2"
	if err := repo.Update(ctx, session); err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	got, _ = repo.Get(ctx, "s1")
	if got.Host != "h2" {
		t.Fatalf("Update() host = %q, want h2", got.Host)
	}

	if err := repo.Update(ctx, model.Session{ID: "missing"}); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Update() missing error = %v, want ErrNotFound", err)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List() len = %d, want 1", len(list))
	}

	if err := repo.Delete(ctx, "s1"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
	if _, err := repo.Get(ctx, "s1"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Get() after delete error = %v, want ErrNotFound", err)
	}
	if err := repo.Delete(ctx, "s1"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Delete() missing error = %v, want ErrNotFound", err)
	}
}

func TestSessionRepositoryConcurrent(t *testing.T) {
	ctx := context.Background()
	repo := NewSessionRepository()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			session := model.Session{ID: string(rune('a' + id%26))}
			_ = repo.Create(ctx, session)
			_, _ = repo.Get(ctx, session.ID)
			_, _ = repo.List(ctx)
		}(i)
	}
	wg.Wait()
}
