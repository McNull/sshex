package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mcnull/sshex/internal/errs"
	"github.com/mcnull/sshex/internal/model"
	"github.com/mcnull/sshex/internal/repository"
)

func TestTunnelServiceAddInvalidPort(t *testing.T) {
	svc := newTestServices()
	if _, err := svc.tunnels.Add(context.Background(), AddTunnelRequest{SessionID: "s1"}); !errors.Is(err, errs.ErrInvalidInput) {
		t.Fatalf("Add() error = %v, want ErrInvalidInput", err)
	}
}

func TestTunnelServiceAddInvalidDirection(t *testing.T) {
	svc := newTestServices()
	_, err := svc.tunnels.Add(context.Background(), AddTunnelRequest{SessionID: "s1", LocalPort: 8080, Direction: "sideways"})
	if !errors.Is(err, errs.ErrInvalidInput) {
		t.Fatalf("Add() error = %v, want ErrInvalidInput", err)
	}
}

func TestTunnelServiceAddMissingSession(t *testing.T) {
	svc := newTestServices()
	if _, err := svc.tunnels.Add(context.Background(), AddTunnelRequest{SessionID: "s1", LocalPort: 8080}); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Add() error = %v, want ErrNotFound", err)
	}
}

func TestTunnelServiceAddPortInUse(t *testing.T) {
	ctx := context.Background()
	svc := newTestServicesWithPorts(func(port int) (bool, error) {
		return port == 8080, nil
	})
	conn := seedActiveSession(t, svc, "s1")

	_, err := svc.tunnels.Add(ctx, AddTunnelRequest{SessionID: "s1", LocalPort: 8080})
	if !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("Add() error = %v, want ErrConflict", err)
	}
	if !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("Add() error = %q, want message containing %q", err, "already in use")
	}
	if conn.addLocalForwards != 0 {
		t.Fatalf("Add() attempted %d forwards, want 0", conn.addLocalForwards)
	}
}

func TestTunnelServiceAddRepositoryConflict(t *testing.T) {
	ctx := context.Background()
	svc := newTestServicesWithPorts(func(int) (bool, error) { return false, nil })
	seedActiveSession(t, svc, "s1")
	if err := svc.tunnelRepo.Create(ctx, model.Tunnel{
		ID: "t1", SessionID: "s1", LocalPort: 8080, Direction: model.ForwardLocal,
	}); err != nil {
		t.Fatalf("seed tunnel: %v", err)
	}

	_, err := svc.tunnels.Add(ctx, AddTunnelRequest{SessionID: "s1", LocalPort: 8080})
	if !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("Add() error = %v, want ErrConflict", err)
	}
}

func TestTunnelServiceAddRemoteSkipsLocalPortCheck(t *testing.T) {
	ctx := context.Background()
	svc := newTestServicesWithPorts(func(port int) (bool, error) {
		return port == 8080, nil
	})
	conn := seedActiveSession(t, svc, "s1")

	tunnel, err := svc.tunnels.Add(ctx, AddTunnelRequest{SessionID: "s1", LocalPort: 8080, Direction: model.ForwardRemote})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if tunnel.Direction != model.ForwardRemote {
		t.Fatalf("Add() direction = %q, want %q", tunnel.Direction, model.ForwardRemote)
	}
	if conn.addRemoteForwards != 1 || conn.addLocalForwards != 0 {
		t.Fatalf("Add() local=%d remote=%d, want local=0 remote=1", conn.addLocalForwards, conn.addRemoteForwards)
	}
}

func TestTunnelServiceAddLocalDefaultsDirection(t *testing.T) {
	ctx := context.Background()
	svc := newTestServicesWithPorts(func(int) (bool, error) { return false, nil })
	conn := seedActiveSession(t, svc, "s1")

	tunnel, err := svc.tunnels.Add(ctx, AddTunnelRequest{SessionID: "s1", LocalPort: 8080})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if tunnel.Direction != model.ForwardLocal {
		t.Fatalf("Add() direction = %q, want %q", tunnel.Direction, model.ForwardLocal)
	}
	if conn.addLocalForwards != 1 || conn.addRemoteForwards != 0 {
		t.Fatalf("Add() local=%d remote=%d, want local=1 remote=0", conn.addLocalForwards, conn.addRemoteForwards)
	}
}

func seedActiveSession(t *testing.T, svc testServices, id string) *fakeConn {
	t.Helper()
	if err := svc.sessionRepo.Create(context.Background(), model.Session{ID: id, State: model.SessionActive}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	conn := &fakeConn{}
	svc.pool.SetController(id, conn)
	return conn
}

func TestTunnelServiceRemoveMissing(t *testing.T) {
	svc := newTestServices()
	if err := svc.tunnels.Remove(context.Background(), "s1", 8080, ""); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Remove() error = %v, want ErrNotFound", err)
	}
}

func TestTunnelServiceRemoveRemote(t *testing.T) {
	ctx := context.Background()
	svc := newTestServices()
	conn := seedActiveSession(t, svc, "s1")
	if err := svc.tunnelRepo.Create(ctx, model.Tunnel{
		ID: "t1", SessionID: "s1", LocalPort: 8080, Direction: model.ForwardRemote,
	}); err != nil {
		t.Fatalf("seed tunnel: %v", err)
	}

	if err := svc.tunnels.Remove(ctx, "s1", 8080, model.ForwardRemote); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if conn.removeRemoteForwards != 1 || conn.removeLocalForwards != 0 {
		t.Fatalf("Remove() local=%d remote=%d, want local=0 remote=1", conn.removeLocalForwards, conn.removeRemoteForwards)
	}
}

func TestTunnelServiceRemoveDirectionFilter(t *testing.T) {
	ctx := context.Background()
	svc := newTestServices()
	seedActiveSession(t, svc, "s1")
	if err := svc.tunnelRepo.Create(ctx, model.Tunnel{
		ID: "t1", SessionID: "s1", LocalPort: 8080, Direction: model.ForwardRemote,
	}); err != nil {
		t.Fatalf("seed tunnel: %v", err)
	}

	if err := svc.tunnels.Remove(ctx, "s1", 8080, model.ForwardLocal); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Remove() error = %v, want ErrNotFound", err)
	}
}

func TestTunnelServiceList(t *testing.T) {
	ctx := context.Background()
	svc := newTestServices()
	if err := svc.tunnelRepo.Create(ctx, model.Tunnel{ID: "t1", SessionID: "s1"}); err != nil {
		t.Fatalf("seed error: %v", err)
	}
	if err := svc.tunnelRepo.Create(ctx, model.Tunnel{ID: "t2", SessionID: "s2"}); err != nil {
		t.Fatalf("seed error: %v", err)
	}

	list, err := svc.tunnels.List(ctx, "s1")
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(list) != 1 || list[0].ID != "t1" {
		t.Fatalf("List(s1) = %+v, want [t1]", list)
	}
}
