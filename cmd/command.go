package cmd

import (
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
		printCommandHints(cmd.OutOrStdout(), name, alias != "")
		return nil
	},
}

// printCommandHints reports how a command can be executed and, when its shell
// alias was added or changed, that the sshex sessions must be restarted for the
// alias to take effect.
func printCommandHints(out io.Writer, name string, aliasChanged bool) {
	fmt.Fprintf(out, "Command can be executed with `sshex exec %s`\n", name)
	if aliasChanged {
		fmt.Fprintln(out, "Restart sshex sessions for the alias to take effect")
	}
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

// commandVariable documents one template variable that can be used in a
// predefined command and what it expands to.
type commandVariable struct {
	name string
	desc string
}

// commandVariables is the single source of truth for the template variable
// legend shared by the help texts and the interactive command editor.
var commandVariables = []commandVariable{
	{"${REMOTE_CWD}", "The remote working directory"},
	{"${REMOTE_USER}", "The remote user"},
	{"${REMOTE_HOST}", "The remote host (ssh alias)"},
	{"${REMOTE_PORT}", "The remote port"},
	{"${REMOTE_SESSION}", "The session id"},
	{"${REMOTE_ORIGIN}", "The origin machine"},
	{"${0}, ${1}...${10}", "Positional arguments (${0} is the command name)"},
	{"${@}", "All positional arguments"},
}

// commandExample documents one example command shared by the help texts and the
// interactive command editor.
type commandExample struct {
	name    string // name used by sshex exec
	alias   string // optional shell alias, empty when none
	desc    string // short description
	command string // the command template itself
}

// commandExamples is the single source of truth for the example commands shown
// in the `command add` help text and the interactive command editor.
var commandExamples = []commandExample{
	{
		name:    "ping",
		desc:    "Ping from origin to the remote host",
		command: "ping ${REMOTE_HOST}",
	},
	{
		name:    "portscan",
		desc:    "Portscan the remote host on a range of ports",
		command: "nmap -p ${1} ${REMOTE_HOST}",
	},
	{
		name:    "code",
		alias:   "code",
		desc:    "Start vscode using the optional directory argument as project",
		command: `code --folder-uri "vscode-remote://ssh-remote+${REMOTE_HOST}${@:-${REMOTE_CWD}}"`,
	},
	{
		name:    "zed",
		alias:   "zed",
		desc:    "Start zed using the optional directory argument as project",
		command: `zed "ssh://${REMOTE_USER}@${REMOTE_HOST}${@:-${REMOTE_CWD}}"`,
	},
}

// invocation renders the example as the arguments to `sshex command add`.
// Because --alias has an optional value, a bare `--alias` leaves the alias and
// name as positional tokens: when the alias equals the name one token serves
// both, otherwise the alias is followed by the name.
func (e commandExample) invocation() string {
	switch {
	case e.alias == "":
		return fmt.Sprintf("%s '%s'", e.name, e.command)
	case e.alias == e.name:
		return fmt.Sprintf("--alias %s '%s'", e.name, e.command)
	default:
		return fmt.Sprintf("--alias %s %s '%s'", e.alias, e.name, e.command)
	}
}

// formatCommandExamplesAdd renders the examples as full `sshex command add`
// invocations for the add help text.
func formatCommandExamplesAdd() string {
	var b strings.Builder
	b.WriteString("\n  Examples:\n")
	for _, e := range commandExamples {
		fmt.Fprintf(&b, "\n    # %s\n    $ sshex command add %s\n", e.desc, e.invocation())
	}
	return b.String()
}

// formatCommandExamplesEdit renders just the command template of each example
// for the interactive editor, without the `sshex command add` wrapper.
func formatCommandExamplesEdit() string {
	var b strings.Builder
	b.WriteString("Examples:\n")
	for _, e := range commandExamples {
		fmt.Fprintf(&b, "\n  %s\n    %s\n", e.desc, e.command)
	}
	return b.String()
}

// formatCommandVariablesHelp renders the template variable legend in two
// aligned columns under the shared header. The header is indented by indent and
// each variable line by indent plus two spaces.
func formatCommandVariablesHelp(indent string) string {
	width := 0
	for _, v := range commandVariables {
		if len(v.name) > width {
			width = len(v.name)
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%sVariables available in a command template:\n", indent)
	for _, v := range commandVariables {
		fmt.Fprintf(&b, "%s  %-*s    %s\n", indent, width, v.name, v.desc)
	}
	return b.String()
}

const (
	nameFieldHelp     = "The name used by sshex exec."
	aliasFieldHelp    = "Optional shell alias installed in the remote shell for this command."
	disabledFieldHelp = "A disabled command is listed but cannot be executed."
)

// commandFieldHelp describes the command field. It reuses the shared variable
// legend so the editor and the help texts stay in sync.
var commandFieldHelp = "The command line executed on the origin, interpreted by " +
	"its shell.\nQuote it so the shell does not expand the template variables " +
	"before sshex stores them.\n\n" +
	formatCommandVariablesHelp("") + "\n" +
	formatCommandExamplesEdit()

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
		commands, err := manager.List(cmd.Context())
		if err != nil {
			return err
		}
		nameValidator := commandNameValidator(commands, current.Name)
		aliasValidator := commandAliasValidator(commands, current.Name)
		p := newPrompter(cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		defer p.close()

		name, err := p.field("name", nameFieldHelp, current.Name, true, nameValidator)
		if err != nil {
			return editPromptError(cmd, err)
		}
		p.separate()

		command, err := p.field("command", commandFieldHelp, current.Command, true, requireCommand)
		if err != nil {
			return editPromptError(cmd, err)
		}
		p.separate()

		alias, err := p.field("alias", aliasFieldHelp, current.Alias, false, aliasValidator)
		if err != nil {
			return editPromptError(cmd, err)
		}
		p.separate()

		disabled, err := p.boolean("disabled", disabledFieldHelp, current.Disabled)
		if err != nil {
			return editPromptError(cmd, err)
		}
		p.separate()

		updated := model.Command{Name: name, Command: command, Alias: alias, Disabled: disabled}
		if err := manager.Update(cmd.Context(), current.Name, updated); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "command %q updated\n", updated.Name)
		printCommandHints(cmd.OutOrStdout(), updated.Name, updated.Alias != current.Alias)
		return nil
	},
}

// editPromptError turns a cancelled edit into a clean, non-error exit and lets
// any other prompt failure propagate.
func editPromptError(cmd *cobra.Command, err error) error {
	if errors.Is(err, errEditCancelled) {
		fmt.Fprintln(cmd.OutOrStdout(), "edit cancelled")
		return nil
	}
	return err
}

// requireCommand rejects an empty command line.
func requireCommand(value string) error {
	if value == "" {
		return errors.New("command is required")
	}
	return nil
}

// commandNameValidator rejects a name that is invalid or already used by another
// command. The command being edited (self) is allowed to keep its own name.
func commandNameValidator(commands []model.Command, self string) func(string) error {
	return func(value string) error {
		if err := service.ValidateName(value); err != nil {
			return err
		}
		for _, command := range commands {
			if command.Name == value && command.Name != self {
				return fmt.Errorf("name %q is already in use", value)
			}
		}
		return nil
	}
}

// commandAliasValidator rejects an invalid alias or one already used by another
// command. An alias may match its own command's name.
func commandAliasValidator(commands []model.Command, self string) func(string) error {
	return func(value string) error {
		if err := service.ValidateAlias(value); err != nil {
			return err
		}
		for _, command := range commands {
			if command.Alias != "" && command.Alias == value && command.Name != self {
				return fmt.Errorf("alias %q is already in use", value)
			}
		}
		return nil
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
