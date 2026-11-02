// Package commands implements the Commands screen of the sshex terminal UI. It
// lists the predefined commands managed by the origin and, when the origin is
// reachable for management, adds, edits, removes and toggles them through
// narrow source and controller interfaces.
package commands

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mcnull/sshex/internal/service"
)

// Command is one row of the command list. Template holds the command line
// executed by the origin. It is deliberately independent of the config and
// broker DTOs so the screen can be tested without a daemon or config file.
type Command struct {
	Name     string
	Template string
	Alias    string
	Disabled bool
}

// Snapshot is one command listing plus whether the commands may be edited.
// Editing is false inside a remote sshex session, where commands are owned by
// the origin.
type Snapshot struct {
	Commands []Command
	Editable bool
}

// Source lists the predefined commands.
type Source interface {
	List() (Snapshot, error)
}

// Controller mutates predefined commands. It is only called when the last
// snapshot reported the commands as editable.
type Controller interface {
	Add(command Command) error
	Update(name string, command Command) error
	Remove(name string) error
	SetDisabled(name string, disabled bool) error
}

// sortCommands orders rows by name so a config or broker that returns a
// different order cannot make the list jump around.
func sortCommands(items []Command) {
	sort.SliceStable(items, func(i, j int) bool {
		left, right := strings.ToLower(items[i].Name), strings.ToLower(items[j].Name)
		if left != right {
			return left < right
		}
		return items[i].Name < items[j].Name
	})
}

// filterCommands keeps the commands whose name or alias contains query. The
// query is trimmed and matched case-insensitively.
func filterCommands(items []Command, query string) []Command {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return append([]Command(nil), items...)
	}
	filtered := make([]Command, 0, len(items))
	for _, command := range items {
		if strings.Contains(strings.ToLower(command.Name), query) ||
			strings.Contains(strings.ToLower(command.Alias), query) {
			filtered = append(filtered, command)
		}
	}
	return filtered
}

// validationError reports which form field is invalid and why.
type validationError struct {
	field int
	err   error
}

func (e *validationError) Error() string { return e.err.Error() }

// validateCommand checks the required fields, their syntax and the uniqueness
// of the name and alias. self is the name of the command being edited; it is
// allowed to keep its own name and alias. The returned field identifies the
// first offending form field.
func validateCommand(existing []Command, self string, command Command) *validationError {
	if err := service.ValidateName(command.Name); err != nil {
		return &validationError{field: fieldName, err: err}
	}
	for _, other := range existing {
		if other.Name == command.Name && other.Name != self {
			return &validationError{
				field: fieldName,
				err:   fmt.Errorf("name %q is already in use", command.Name),
			}
		}
	}
	if strings.TrimSpace(command.Template) == "" {
		return &validationError{field: fieldCommand, err: errors.New("command is required")}
	}
	if err := service.ValidateAlias(command.Alias); err != nil {
		return &validationError{field: fieldAlias, err: err}
	}
	for _, other := range existing {
		if other.Alias != "" && other.Alias == command.Alias && other.Name != self {
			return &validationError{
				field: fieldAlias,
				err:   fmt.Errorf("alias %q is already in use", command.Alias),
			}
		}
	}
	return nil
}
