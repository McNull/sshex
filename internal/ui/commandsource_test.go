package ui

import (
	"context"
	"errors"
	"testing"

	"github.com/mcnull/sshex/internal/api"
	"github.com/mcnull/sshex/internal/errs"
	"github.com/mcnull/sshex/internal/model"
	"github.com/mcnull/sshex/internal/ui/screen/commands"
)

func TestCommandFromModel(t *testing.T) {
	got := commandFromModel(model.Command{
		Name:     "code",
		Command:  "code --new",
		Alias:    "ed",
		Disabled: true,
	})
	want := commands.Command{
		Name:     "code",
		Template: "code --new",
		Alias:    "ed",
		Disabled: true,
	}
	if got != want {
		t.Fatalf("commandFromModel() = %+v, want %+v", got, want)
	}
}

type brokerAddCall struct {
	name     string
	command  string
	alias    string
	disabled bool
}

type fakeBroker struct {
	responses []api.CommandResponse
	listErr   error
	addCalls  []brokerAddCall
	update    []api.UpdateCommandRequest
	removed   []string
	disabled  []bool
	calls     int
}

func (b *fakeBroker) ListCommands(context.Context) ([]api.CommandResponse, error) {
	b.calls++
	return b.responses, b.listErr
}

func (b *fakeBroker) AddCommand(_ context.Context, name, command, alias string, disabled bool) (api.CommandResponse, error) {
	b.calls++
	b.addCalls = append(b.addCalls, brokerAddCall{name: name, command: command, alias: alias, disabled: disabled})
	return api.CommandResponse{Name: name, Command: command, Alias: alias, Disabled: disabled}, nil
}

func (b *fakeBroker) UpdateCommand(_ context.Context, name string, req api.UpdateCommandRequest) (api.CommandResponse, error) {
	b.calls++
	b.update = append(b.update, req)
	return api.CommandResponse{Name: name}, nil
}

func (b *fakeBroker) RemoveCommand(_ context.Context, name string) error {
	b.calls++
	b.removed = append(b.removed, name)
	return nil
}

func (b *fakeBroker) SetCommandDisabled(_ context.Context, name string, disabled bool) error {
	b.calls++
	b.disabled = append(b.disabled, disabled)
	return nil
}

func TestRemoteCommandStoreLists(t *testing.T) {
	broker := &fakeBroker{responses: []api.CommandResponse{
		{Name: "code", Command: "code", Alias: "ed"},
	}}
	store := remoteCommandStore{broker: broker, canManage: true}

	list, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(list) != 1 || list[0].Name != "code" || list[0].Command != "code" || list[0].Alias != "ed" {
		t.Fatalf("List() = %+v", list)
	}
	if !store.CanManage() {
		t.Fatal("CanManage() = false, want true")
	}
}

func TestRemoteCommandStoreListError(t *testing.T) {
	store := remoteCommandStore{broker: &fakeBroker{listErr: errors.New("boom")}, canManage: true}

	if _, err := store.List(context.Background()); err == nil {
		t.Fatal("List() error = nil, want the broker error")
	}
}

func TestRemoteCommandStoreReadOnlyRejectsMutations(t *testing.T) {
	broker := &fakeBroker{}
	store := remoteCommandStore{broker: broker, canManage: false}
	ctx := context.Background()

	calls := []struct {
		name string
		call func() error
	}{
		{"Add", func() error { return store.Add(ctx, "code", "code", "", false) }},
		{"Update", func() error { return store.Update(ctx, "code", model.Command{Name: "code"}) }},
		{"Remove", func() error { return store.Remove(ctx, "code") }},
		{"SetDisabled", func() error { return store.SetDisabled(ctx, "code", true) }},
	}
	for _, call := range calls {
		if err := call.call(); !errors.Is(err, errs.ErrCommandReadOnly) {
			t.Fatalf("%s() error = %v, want ErrCommandReadOnly", call.name, err)
		}
	}
	if broker.calls != 0 {
		t.Fatalf("broker calls = %d, want 0", broker.calls)
	}
}

func TestRemoteCommandStoreManages(t *testing.T) {
	broker := &fakeBroker{}
	store := remoteCommandStore{broker: broker, canManage: true}
	ctx := context.Background()

	if err := store.Add(ctx, "code", "code --new", "ed", false); err != nil {
		t.Fatalf("Add() error: %v", err)
	}
	if err := store.Update(ctx, "code", model.Command{Name: "code2", Command: "code2", Alias: "ed"}); err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	if err := store.Remove(ctx, "code2"); err != nil {
		t.Fatalf("Remove() error: %v", err)
	}
	if err := store.SetDisabled(ctx, "code", true); err != nil {
		t.Fatalf("SetDisabled() error: %v", err)
	}

	if len(broker.addCalls) != 1 {
		t.Fatalf("add calls = %+v, want one", broker.addCalls)
	}
	if got := broker.addCalls[0]; got.name != "code" || got.command != "code --new" || got.alias != "ed" || got.disabled {
		t.Fatalf("add call = %+v", got)
	}
	if len(broker.update) != 1 || broker.update[0].Name != "code2" || broker.update[0].Command != "code2" {
		t.Fatalf("update calls = %+v", broker.update)
	}
	if len(broker.removed) != 1 || broker.removed[0] != "code2" {
		t.Fatalf("remove calls = %+v", broker.removed)
	}
	if len(broker.disabled) != 1 || !broker.disabled[0] {
		t.Fatalf("disabled calls = %+v", broker.disabled)
	}
}
