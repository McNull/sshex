package configrepo

import (
	"context"
	"sync"

	"github.com/mcnull/sshex/internal/config"
	"github.com/mcnull/sshex/internal/model"
	"github.com/mcnull/sshex/internal/repository"
)

// CommandRepository is a CommandRepository backed by the YAML config file. It
// reloads the file on every operation so a running broker observes edits made
// by another process, and it preserves the non-command sections on save.
type CommandRepository struct {
	mu   sync.Mutex
	path string
}

func NewCommandRepository(path string) *CommandRepository {
	return &CommandRepository{path: path}
}

func (r *CommandRepository) Create(_ context.Context, command model.Command) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cfg, err := config.Load(r.path)
	if err != nil {
		return err
	}
	if indexOf(cfg.Commands, command.Name) >= 0 {
		return repository.ErrConflict
	}
	if command.Alias != "" && aliasIndexOf(cfg.Commands, command.Alias) >= 0 {
		return repository.ErrConflict
	}
	cfg.Commands = append(cfg.Commands, toConfig(command))
	return config.Save(r.path, cfg)
}

func (r *CommandRepository) Get(_ context.Context, name string) (model.Command, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cfg, err := config.Load(r.path)
	if err != nil {
		return model.Command{}, err
	}
	index := indexOf(cfg.Commands, name)
	if index < 0 {
		return model.Command{}, repository.ErrNotFound
	}
	return toModel(cfg.Commands[index]), nil
}

func (r *CommandRepository) List(_ context.Context) ([]model.Command, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cfg, err := config.Load(r.path)
	if err != nil {
		return nil, err
	}
	commands := make([]model.Command, 0, len(cfg.Commands))
	for _, command := range cfg.Commands {
		commands = append(commands, toModel(command))
	}
	return commands, nil
}

func (r *CommandRepository) Update(_ context.Context, name string, command model.Command) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cfg, err := config.Load(r.path)
	if err != nil {
		return err
	}
	index := indexOf(cfg.Commands, name)
	if index < 0 {
		return repository.ErrNotFound
	}
	if command.Name != name {
		if other := indexOf(cfg.Commands, command.Name); other >= 0 && other != index {
			return repository.ErrConflict
		}
	}
	if command.Alias != "" {
		if other := aliasIndexOf(cfg.Commands, command.Alias); other >= 0 && other != index {
			return repository.ErrConflict
		}
	}
	cfg.Commands[index] = toConfig(command)
	return config.Save(r.path, cfg)
}

func (r *CommandRepository) Delete(_ context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cfg, err := config.Load(r.path)
	if err != nil {
		return err
	}
	index := indexOf(cfg.Commands, name)
	if index < 0 {
		return repository.ErrNotFound
	}
	cfg.Commands = append(cfg.Commands[:index], cfg.Commands[index+1:]...)
	return config.Save(r.path, cfg)
}

func indexOf(commands []config.CommandConfig, name string) int {
	for i, command := range commands {
		if command.Name == name {
			return i
		}
	}
	return -1
}

func aliasIndexOf(commands []config.CommandConfig, alias string) int {
	for i, command := range commands {
		if command.Alias != "" && command.Alias == alias {
			return i
		}
	}
	return -1
}

func toModel(command config.CommandConfig) model.Command {
	return model.Command{Name: command.Name, Command: command.Command, Alias: command.Alias, Disabled: command.Disabled}
}

func toConfig(command model.Command) config.CommandConfig {
	return config.CommandConfig{Name: command.Name, Command: command.Command, Alias: command.Alias, Disabled: command.Disabled}
}
