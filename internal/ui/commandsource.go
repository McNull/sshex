package ui

import (
	"context"

	"github.com/mcnull/sshex/internal/api"
	"github.com/mcnull/sshex/internal/client"
	"github.com/mcnull/sshex/internal/config"
	"github.com/mcnull/sshex/internal/errs"
	"github.com/mcnull/sshex/internal/model"
	configrepo "github.com/mcnull/sshex/internal/repository/config"
	"github.com/mcnull/sshex/internal/service"
	"github.com/mcnull/sshex/internal/ui/screen/commands"
)

// commandControl adapts the live broker or the local config for the Commands
// screen. The screen only sees its own narrow Source and Controller interfaces,
// so tests never touch the daemon or the config file.
type commandControl struct {
	scope controlScope
}

// List implements commands.Source. Commands are owned by the origin: origin
// scope manages this machine's config, while client scope reads the current
// session's outer origin through its broker. The returned Editable flag mirrors
// the CLI's capability check.
func (c commandControl) List() (commands.Snapshot, error) {
	ctx, cancel := context.WithTimeout(context.Background(), sessionRequestTimeout)
	defer cancel()

	store, err := resolveCommandStore(c.scope)
	if err != nil {
		return commands.Snapshot{}, err
	}
	list, err := store.List(ctx)
	if err != nil {
		return commands.Snapshot{}, err
	}
	return commands.Snapshot{Commands: commandRows(list), Editable: store.CanManage()}, nil
}

// Add implements commands.Controller.
func (c commandControl) Add(command commands.Command) error {
	ctx, cancel := context.WithTimeout(context.Background(), sessionRequestTimeout)
	defer cancel()

	store, err := resolveCommandStore(c.scope)
	if err != nil {
		return err
	}
	if !store.CanManage() {
		return errs.ErrCommandReadOnly
	}
	return store.Add(ctx, command.Name, command.Template, command.Alias, command.Disabled)
}

// Update implements commands.Controller. name is the current name, so a rename
// is expressed by command.Name differing from it.
func (c commandControl) Update(name string, command commands.Command) error {
	ctx, cancel := context.WithTimeout(context.Background(), sessionRequestTimeout)
	defer cancel()

	store, err := resolveCommandStore(c.scope)
	if err != nil {
		return err
	}
	if !store.CanManage() {
		return errs.ErrCommandReadOnly
	}
	return store.Update(ctx, name, model.Command{
		Name:     command.Name,
		Command:  command.Template,
		Alias:    command.Alias,
		Disabled: command.Disabled,
	})
}

// Remove implements commands.Controller.
func (c commandControl) Remove(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), sessionRequestTimeout)
	defer cancel()

	store, err := resolveCommandStore(c.scope)
	if err != nil {
		return err
	}
	if !store.CanManage() {
		return errs.ErrCommandReadOnly
	}
	return store.Remove(ctx, name)
}

// SetDisabled implements commands.Controller.
func (c commandControl) SetDisabled(name string, disabled bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), sessionRequestTimeout)
	defer cancel()

	store, err := resolveCommandStore(c.scope)
	if err != nil {
		return err
	}
	if !store.CanManage() {
		return errs.ErrCommandReadOnly
	}
	return store.SetDisabled(ctx, name, disabled)
}

// commandStore is the storage adapter shared by the list and its mutations. It
// mirrors the CLI's commandManager: a discovered session is reached through the
// broker and may be read-only, while the local config is always manageable.
type commandStore interface {
	List(ctx context.Context) ([]model.Command, error)
	Add(ctx context.Context, name, command, alias string, disabled bool) error
	Update(ctx context.Context, name string, command model.Command) error
	Remove(ctx context.Context, name string) error
	SetDisabled(ctx context.Context, name string, disabled bool) error
	CanManage() bool
}

// resolveCommandStore uses the broker of the selected scope when one is
// discoverable. Origin scope falls back to the local config directly; client
// scope must have a current session and stays read-only without the commands
// capability.
func resolveCommandStore(scope controlScope) (commandStore, error) {
	file, err := controlSessionFile(scope)
	if err == nil {
		return remoteCommandStore{
			broker:    client.New(file),
			canManage: file.Capabilities.Contains(model.CapabilityCommands),
		}, nil
	}
	if scope == scopeClient {
		return nil, err
	}
	path, err := config.Resolve("")
	if err != nil {
		return nil, err
	}
	return localCommandStore{commands: service.NewCommandService(service.CommandServiceConfig{
		Commands: configrepo.NewCommandRepository(path),
	})}, nil
}

// brokerCommands is the subset of the broker client used by the Commands
// screen. Keeping it narrow makes the read-only guard testable with a fake.
type brokerCommands interface {
	ListCommands(ctx context.Context) ([]api.CommandResponse, error)
	AddCommand(ctx context.Context, name, command, alias string, disabled bool) (api.CommandResponse, error)
	UpdateCommand(ctx context.Context, name string, req api.UpdateCommandRequest) (api.CommandResponse, error)
	RemoveCommand(ctx context.Context, name string) error
	SetCommandDisabled(ctx context.Context, name string, disabled bool) error
}

type remoteCommandStore struct {
	broker    brokerCommands
	canManage bool
}

func (r remoteCommandStore) List(ctx context.Context) ([]model.Command, error) {
	responses, err := r.broker.ListCommands(ctx)
	if err != nil {
		return nil, err
	}
	commands := make([]model.Command, 0, len(responses))
	for _, response := range responses {
		commands = append(commands, model.Command{
			Name:     response.Name,
			Command:  response.Command,
			Alias:    response.Alias,
			Disabled: response.Disabled,
		})
	}
	return commands, nil
}

func (r remoteCommandStore) Add(ctx context.Context, name, command, alias string, disabled bool) error {
	if !r.canManage {
		return errs.ErrCommandReadOnly
	}
	_, err := r.broker.AddCommand(ctx, name, command, alias, disabled)
	return err
}

func (r remoteCommandStore) Update(ctx context.Context, name string, command model.Command) error {
	if !r.canManage {
		return errs.ErrCommandReadOnly
	}
	_, err := r.broker.UpdateCommand(ctx, name, api.UpdateCommandRequest{
		Name:     command.Name,
		Command:  command.Command,
		Alias:    command.Alias,
		Disabled: command.Disabled,
	})
	return err
}

func (r remoteCommandStore) Remove(ctx context.Context, name string) error {
	if !r.canManage {
		return errs.ErrCommandReadOnly
	}
	return r.broker.RemoveCommand(ctx, name)
}

func (r remoteCommandStore) SetDisabled(ctx context.Context, name string, disabled bool) error {
	if !r.canManage {
		return errs.ErrCommandReadOnly
	}
	return r.broker.SetCommandDisabled(ctx, name, disabled)
}

func (r remoteCommandStore) CanManage() bool { return r.canManage }

type localCommandStore struct {
	commands *service.CommandService
}

func (l localCommandStore) List(ctx context.Context) ([]model.Command, error) {
	return l.commands.List(ctx)
}

func (l localCommandStore) Add(ctx context.Context, name, command, alias string, disabled bool) error {
	return l.commands.Add(ctx, name, command, alias, disabled)
}

func (l localCommandStore) Update(ctx context.Context, name string, command model.Command) error {
	return l.commands.Update(ctx, name, command)
}

func (l localCommandStore) Remove(ctx context.Context, name string) error {
	return l.commands.Remove(ctx, name)
}

func (l localCommandStore) SetDisabled(ctx context.Context, name string, disabled bool) error {
	return l.commands.SetDisabled(ctx, name, disabled)
}

func (l localCommandStore) CanManage() bool { return true }

func commandRows(list []model.Command) []commands.Command {
	out := make([]commands.Command, 0, len(list))
	for _, command := range list {
		out = append(out, commandFromModel(command))
	}
	return out
}

func commandFromModel(command model.Command) commands.Command {
	return commands.Command{
		Name:     command.Name,
		Template: command.Command,
		Alias:    command.Alias,
		Disabled: command.Disabled,
	}
}
