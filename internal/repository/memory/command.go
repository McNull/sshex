package memory

import (
	"context"
	"sync"

	"github.com/mcnull/sshex/internal/model"
	"github.com/mcnull/sshex/internal/repository"
)

type CommandRepository struct {
	mu       sync.RWMutex
	commands map[string]model.Command
}

func NewCommandRepository() *CommandRepository {
	return &CommandRepository{commands: make(map[string]model.Command)}
}

func (r *CommandRepository) Create(_ context.Context, command model.Command) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.commands[command.Name]; ok {
		return repository.ErrConflict
	}
	if command.Alias != "" {
		if _, ok := r.byAlias(command.Alias); ok {
			return repository.ErrConflict
		}
	}
	r.commands[command.Name] = command
	return nil
}

func (r *CommandRepository) Get(_ context.Context, name string) (model.Command, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	command, ok := r.commands[name]
	if !ok {
		return model.Command{}, repository.ErrNotFound
	}
	return command, nil
}

func (r *CommandRepository) List(_ context.Context) ([]model.Command, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	commands := make([]model.Command, 0, len(r.commands))
	for _, command := range r.commands {
		commands = append(commands, command)
	}
	return commands, nil
}

func (r *CommandRepository) Update(_ context.Context, name string, command model.Command) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.commands[name]; !ok {
		return repository.ErrNotFound
	}
	if command.Name != name {
		if _, ok := r.commands[command.Name]; ok {
			return repository.ErrConflict
		}
	}
	if command.Alias != "" {
		if owner, ok := r.byAlias(command.Alias); ok && owner != name {
			return repository.ErrConflict
		}
	}
	delete(r.commands, name)
	r.commands[command.Name] = command
	return nil
}

// byAlias returns the name of the command currently owning alias.
func (r *CommandRepository) byAlias(alias string) (string, bool) {
	for name, command := range r.commands {
		if command.Alias != "" && command.Alias == alias {
			return name, true
		}
	}
	return "", false
}

func (r *CommandRepository) Delete(_ context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.commands[name]; !ok {
		return repository.ErrNotFound
	}
	delete(r.commands, name)
	return nil
}
