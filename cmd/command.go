package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/mcnull/sshex/internal/api"
	"github.com/mcnull/sshex/internal/client"
	"github.com/mcnull/sshex/internal/model"
	"github.com/mcnull/sshex/internal/repository"
	"github.com/mcnull/sshex/internal/service"
	"github.com/spf13/cobra"
)

var commandCmd = &cobra.Command{
	Use:   "command",
	Short: "Manage predefined commands",
	RunE: func(cmd *cobra.Command, _ []string) error {
		printHelp(cmd.OutOrStdout(), "sshex command")
		return nil
	},
}

// aliasFromName is the sentinel NoOptDefVal for --alias. It marks a bare
// `--alias` (no value), meaning "use the command name".
const aliasFromName = "__sshex_alias_from_name__"

var commandAddCmd = &cobra.Command{
	Use:               "add [--disabled] [--alias [<alias>]] <name> <command>...",
	Short:             "Add a predefined command",
	Args:              cobra.MinimumNArgs(2),
	ValidArgsFunction: completeCommandNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		disabled, _ := cmd.Flags().GetBool("disabled")
		alias, args := resolveAddAlias(cmd, args)
		manager, err := resolveCommandManager()
		if err != nil {
			return err
		}
		name := args[0]
		command := strings.Join(args[1:], " ")
		if err := manager.Add(cmd.Context(), name, command, alias, disabled); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "command %q added\n", name)
		return nil
	},
}

// resolveAddAlias interprets the --alias flag. A bare `--alias` uses the command
// name; an explicit `--alias <alias>` or `--alias=<alias>` uses the given value.
// Cobra cannot bind an optional value to a flag, so a bare --alias is detected
// through its sentinel value. When the flag is bare and a leading token remains
// beyond <name> <command>, that token is taken as the alias.
func resolveAddAlias(cmd *cobra.Command, args []string) (string, []string) {
	alias, _ := cmd.Flags().GetString("alias")
	if alias != aliasFromName {
		return alias, args
	}
	if len(args) >= 3 {
		return args[0], args[1:]
	}
	return args[0], args
}

var commandEditCmd = &cobra.Command{
	Use:               "edit <name>",
	Short:             "Edit a predefined command field by field",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeCommandNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		manager, err := resolveCommandManager()
		if err != nil {
			return err
		}
		current, err := manager.Get(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		in := bufio.NewReader(cmd.InOrStdin())
		out := cmd.OutOrStdout()

		name, err := promptField(in, out, "name", current.Name)
		if err != nil {
			return err
		}
		command, err := promptField(in, out, "command", current.Command)
		if err != nil {
			return err
		}
		alias, err := promptField(in, out, "alias", current.Alias)
		if err != nil {
			return err
		}
		disabled, err := promptBool(in, out, "disabled", current.Disabled)
		if err != nil {
			return err
		}

		updated := model.Command{Name: name, Command: command, Alias: alias, Disabled: disabled}
		if err := manager.Update(cmd.Context(), current.Name, updated); err != nil {
			return err
		}
		fmt.Fprintf(out, "command %q updated\n", updated.Name)
		return nil
	},
}

// promptField prints the current value and reads a replacement from in. An empty
// line keeps the current value.
func promptField(in *bufio.Reader, out io.Writer, label, current string) (string, error) {
	fmt.Fprintf(out, "%s [%s]: ", label, current)
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	value := strings.TrimSpace(line)
	if value == "" {
		return current, nil
	}
	return value, nil
}

// promptBool prompts for a yes/no value, keeping the current value on an empty
// line or an unrecognized answer.
func promptBool(in *bufio.Reader, out io.Writer, label string, current bool) (bool, error) {
	shown := "no"
	if current {
		shown = "yes"
	}
	fmt.Fprintf(out, "%s (yes/no) [%s]: ", label, shown)
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return current, err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "":
		return current, nil
	case "y", "yes", "true":
		return true, nil
	case "n", "no", "false":
		return false, nil
	default:
		return current, nil
	}
}

var commandRmCmd = &cobra.Command{
	Use:               "rm <name>",
	Short:             "Remove a predefined command",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeCommandNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		manager, err := resolveCommandManager()
		if err != nil {
			return err
		}
		if err := manager.Remove(cmd.Context(), args[0]); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "command removed")
		return nil
	},
}

var commandListCmd = &cobra.Command{
	Use:   "list",
	Short: "List predefined commands",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		manager, err := resolveCommandManager()
		if err != nil {
			return err
		}
		commands, err := manager.List(cmd.Context())
		if err != nil {
			return err
		}
		if len(commands) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "no commands.")
			return nil
		}
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
		defer w.Flush()
		fmt.Fprintln(w, "NAME\tCOMMAND\tALIAS\tDISABLED")
		for _, command := range commands {
			disabled := "no"
			if command.Disabled {
				disabled = "yes"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", command.Name, command.Command, command.Alias, disabled)
		}
		return nil
	},
}

var commandEnableCmd = &cobra.Command{
	Use:               "enable <name>",
	Short:             "Enable a predefined command",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeCommandNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		return setCommandDisabled(cmd, args[0], false)
	},
}

var commandDisableCmd = &cobra.Command{
	Use:               "disable <name>",
	Short:             "Disable a predefined command",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeCommandNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		return setCommandDisabled(cmd, args[0], true)
	},
}

var commandHelpCmd = &cobra.Command{
	Use:   "help [command]",
	Short: "Show help for command or one of its commands",
	RunE: func(cmd *cobra.Command, args []string) error {
		target, _, err := commandCmd.Find(args)
		if err != nil || target == nil {
			printHelp(cmd.OutOrStdout(), "sshex command")
			return nil
		}
		printHelp(cmd.OutOrStdout(), target.CommandPath())
		return nil
	},
}

func setCommandDisabled(cmd *cobra.Command, name string, disabled bool) error {
	manager, err := resolveCommandManager()
	if err != nil {
		return err
	}
	if err := manager.SetDisabled(cmd.Context(), name, disabled); err != nil {
		return err
	}
	state := "enabled"
	if disabled {
		state = "disabled"
	}
	fmt.Fprintf(cmd.OutOrStdout(), "command %q %s\n", name, state)
	return nil
}

// commandManager abstracts command storage so the CLI can talk to the broker
// when a session is live, or edit the local config directly otherwise.
type commandManager interface {
	List(ctx context.Context) ([]model.Command, error)
	Get(ctx context.Context, name string) (model.Command, error)
	Add(ctx context.Context, name, command, alias string, disabled bool) error
	Update(ctx context.Context, name string, command model.Command) error
	Remove(ctx context.Context, name string) error
	SetDisabled(ctx context.Context, name string, disabled bool) error
}

type remoteCommands struct {
	client *client.Client
}

func (r remoteCommands) List(ctx context.Context) ([]model.Command, error) {
	responses, err := r.client.ListCommands(ctx)
	if err != nil {
		return nil, err
	}
	commands := make([]model.Command, 0, len(responses))
	for _, response := range responses {
		commands = append(commands, model.Command{Name: response.Name, Command: response.Command, Alias: response.Alias, Disabled: response.Disabled})
	}
	return commands, nil
}

func (r remoteCommands) Get(ctx context.Context, name string) (model.Command, error) {
	commands, err := r.List(ctx)
	if err != nil {
		return model.Command{}, err
	}
	for _, command := range commands {
		if command.Name == name {
			return command, nil
		}
	}
	return model.Command{}, repository.ErrNotFound
}

func (r remoteCommands) Add(ctx context.Context, name, command, alias string, disabled bool) error {
	_, err := r.client.AddCommand(ctx, name, command, alias, disabled)
	return err
}

func (r remoteCommands) Update(ctx context.Context, name string, command model.Command) error {
	_, err := r.client.UpdateCommand(ctx, name, api.UpdateCommandRequest{
		Name:     command.Name,
		Command:  command.Command,
		Alias:    command.Alias,
		Disabled: command.Disabled,
	})
	return err
}

func (r remoteCommands) Remove(ctx context.Context, name string) error {
	return r.client.RemoveCommand(ctx, name)
}

func (r remoteCommands) SetDisabled(ctx context.Context, name string, disabled bool) error {
	return r.client.SetCommandDisabled(ctx, name, disabled)
}

type localCommands struct {
	commands *service.CommandService
}

func (l localCommands) List(ctx context.Context) ([]model.Command, error) {
	return l.commands.List(ctx)
}

func (l localCommands) Get(ctx context.Context, name string) (model.Command, error) {
	return l.commands.Get(ctx, name)
}

func (l localCommands) Add(ctx context.Context, name, command, alias string, disabled bool) error {
	return l.commands.Add(ctx, name, command, alias, disabled)
}

func (l localCommands) Update(ctx context.Context, name string, command model.Command) error {
	return l.commands.Update(ctx, name, command)
}

func (l localCommands) Remove(ctx context.Context, name string) error {
	return l.commands.Remove(ctx, name)
}

func (l localCommands) SetDisabled(ctx context.Context, name string, disabled bool) error {
	return l.commands.SetDisabled(ctx, name, disabled)
}

// resolveCommandManager prefers the broker when a live session is discoverable
// so commands can be managed from a remote shell; otherwise it edits the local
// config directly.
func resolveCommandManager() (commandManager, error) {
	if c, err := controlClient(); err == nil {
		return remoteCommands{client: c}, nil
	}
	if application == nil || application.Commands == nil {
		return nil, errors.New("command manager unavailable")
	}
	return localCommands{commands: application.Commands}, nil
}

func completeCommandNames(_ *cobra.Command, _ []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
	manager, err := resolveCommandManager()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	commands, err := manager.List(context.Background())
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	comps := make([]cobra.Completion, 0, len(commands))
	for _, command := range commands {
		desc := command.Command
		if command.Disabled {
			desc += " (disabled)"
		}
		comps = append(comps, cobra.CompletionWithDesc(command.Name, desc))
	}
	return comps, cobra.ShellCompDirectiveNoFileComp
}

func init() {
	commandAddCmd.Flags().Bool("disabled", false, "Add the command in a disabled state")
	commandAddCmd.Flags().String("alias", "", "Add a shell alias for the command on the remote (defaults to the command name)")
	commandAddCmd.Flags().Lookup("alias").NoOptDefVal = aliasFromName
	commandAddCmd.Flags().SetInterspersed(false)

	commandCmd.AddCommand(commandAddCmd, commandEditCmd, commandRmCmd, commandListCmd, commandEnableCmd, commandDisableCmd, commandHelpCmd)
}
