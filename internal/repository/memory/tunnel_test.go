package memory

import (
	"context"
	"errors"
	"testing"

	"github.com/mcnull/sshex/internal/model"
	"github.com/mcnull/sshex/internal/repository"
)

func TestTunnelRepositoryRejectsDuplicatePort(t *testing.T) {
	ctx := context.Background()
	repo := NewTunnelRepository()

	if err := repo.Create(ctx, model.Tunnel{ID: "t1", SessionID: "s1", LocalPort: 8080, Direction: model.ForwardLocal}); err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := repo.Create(ctx, model.Tunnel{ID: "t2", SessionID: "s1", LocalPort: 8080, Direction: model.ForwardLocal}); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("Create() duplicate port error = %v, want ErrConflict", err)
	}
	if err := repo.Create(ctx, model.Tunnel{ID: "t3", SessionID: "s2", LocalPort: 8080, Direction: model.ForwardLocal}); err != nil {
		t.Fatalf("Create() other session error = %v, want nil", err)
	}
	if err := repo.Create(ctx, model.Tunnel{ID: "t4", SessionID: "s1", LocalPort: 8081, Direction: model.ForwardLocal}); err != nil {
		t.Fatalf("Create() other port error = %v, want nil", err)
	}
}

func TestTunnelRepositoryLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := NewTunnelRepository()
	tunnel := model.Tunnel{ID: "t1", SessionID: "s1", LocalPort: 8080}

	if err := repo.Create(ctx, tunnel); err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := repo.Create(ctx, tunnel); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("Create() duplicate error = %v, want ErrConflict", err)
	}

	got, err := repo.Get(ctx, "t1")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got.LocalPort != 8080 {
		t.Fatalf("Get() = %+v, want port 8080", got)
	}

	if err := repo.Create(ctx, model.Tunnel{ID: "t2", SessionID: "s2"}); err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	list, err := repo.ListBySession(ctx, "s1")
	if err != nil {
		t.Fatalf("ListBySession() error: %v", err)
	}
	if len(list) != 1 || list[0].ID != "t1" {
		t.Fatalf("ListBySession(s1) = %+v, want [t1]", list)
	}

	if err := repo.Delete(ctx, "t1"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
	if err := repo.Delete(ctx, "t1"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Delete() missing error = %v, want ErrNotFound", err)
	}

	if err := repo.DeleteBySession(ctx, "s2"); err != nil {
		t.Fatalf("DeleteBySession() error: %v", err)
	}
	if _, err := repo.Get(ctx, "t2"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Get() after DeleteBySession error = %v, want ErrNotFound", err)
	}
}
