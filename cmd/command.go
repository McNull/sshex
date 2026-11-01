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
	Use:               "add [--disabled] [--alias [<alias>]] [<name> <command>...]",
	Short:             "Add a predefined command",
	Args:              addCommandArgs,
	ValidArgsFunction: completeCommandNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		manager, err := resolveCommandManager()
		if err != nil {
			return err
		}
		if err := requireCommandEditable(manager); err != nil {
			return err
		}
		if len(args) == 0 {
			return addCommandInteractive(cmd, manager)
		}
		disabled, _ := cmd.Flags().GetBool("disabled")
		alias, args := resolveAddAlias(cmd, args)
		name := args[0]
		command := strings.Join(args[1:], " ")
		if err := manager.Add(cmd.Context(), name, command, alias, disabled); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "command %q added\n", name)
		printCommandHints(cmd.OutOrStdout(), name, alias != "", recordDemoMode())
		return nil
	},
}

// addCommandArgs accepts either no positional arguments, which opens the
// interactive editor, or a name and a command (two or more), which adds the
// command directly. A lone name has no command to store, so it is rejected.
func addCommandArgs(_ *cobra.Command, args []string) error {
	if len(args) == 0 || len(args) >= 2 {
		return nil
	}
	return fmt.Errorf("requires at least 2 args when adding directly, or none to use the editor; received 1")
}

// addCommandInteractive opens the field-by-field editor for a new command. The
// --alias and --disabled flags seed the editor's initial values; a bare --alias
// seeds the alias with the name entered in the editor.
func addCommandInteractive(cmd *cobra.Command, manager commandManager) error {
	alias, _ := cmd.Flags().GetString("alias")
	disabled, _ := cmd.Flags().GetBool("disabled")
	commands, err := manager.List(cmd.Context())
	if err != nil {
		return err
	}
	opts := commandEditorOptions{
		command:       model.Command{Disabled: disabled},
		aliasFromName: alias == aliasFromName,
	}
	if alias != aliasFromName {
		opts.command.Alias = alias
	}
	created, err := promptCommandFields(cmd, opts, commands)
	if err != nil {
		return promptCancelled(cmd, err, "add")
	}
	if err := manager.Add(cmd.Context(), created.Name, created.Command, created.Alias, created.Disabled); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "command %q added\n", created.Name)
	printCommandHints(cmd.OutOrStdout(), created.Name, created.Alias != "", recordDemoMode())
	return nil
}

// printCommandHints reports how a command can be executed and, when its shell
// alias was added or changed, that the sshex sessions must be restarted for the
// alias to take effect. Nothing is printed in demo mode.
func printCommandHints(out io.Writer, name string, aliasChanged, quiet bool) {
	if quiet {
		return
	}
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
		if err := requireCommandEditable(manager); err != nil {
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
		updated, err := promptCommandFields(cmd, commandEditorOptions{command: current}, commands)
		if err != nil {
			return promptCancelled(cmd, err, "edit")
		}
		if err := manager.Update(cmd.Context(), current.Name, updated); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "command %q updated\n", updated.Name)
		printCommandHints(cmd.OutOrStdout(), updated.Name, updated.Alias != current.Alias, recordDemoMode())
		return nil
	},
}

// commandEditorOptions configures the interactive command editor. command holds
// the initial value of each field; aliasFromName, when set, makes the alias
// default to the name entered in the editor (used for a bare --alias).
type commandEditorOptions struct {
	command       model.Command
	aliasFromName bool
}

// promptCommandFields asks for each command field in turn and returns the
// edited command. The description of each field is printed once and the current
// value is pre-filled. The command being edited (opts.command.Name) is allowed
// to keep its own name and alias. It returns errEditCancelled when the user
// aborts.
func promptCommandFields(cmd *cobra.Command, opts commandEditorOptions, commands []model.Command) (model.Command, error) {
	nameValidator := commandNameValidator(commands, opts.command.Name)
	aliasValidator := commandAliasValidator(commands, opts.command.Name)
	p := newPrompter(cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
	p.quiet = recordDemoMode()
	defer p.close()

	name, err := p.field("name", nameFieldHelp, opts.command.Name, true, nameValidator)
	if err != nil {
		return model.Command{}, err
	}
	p.separate()

	command, err := p.field("command", commandFieldHelp, opts.command.Command, true, requireCommand)
	if err != nil {
		return model.Command{}, err
	}
	p.separate()

	aliasDefault := opts.command.Alias
	if opts.aliasFromName {
		aliasDefault = name
	}
	alias, err := p.field("alias", aliasFieldHelp, aliasDefault, false, aliasValidator)
	if err != nil {
		return model.Command{}, err
	}
	p.separate()

	disabled, err := p.boolean("disabled", disabledFieldHelp, opts.command.Disabled)
	if err != nil {
		return model.Command{}, err
	}
	p.separate()

	return model.Command{Name: name, Command: command, Alias: alias, Disabled: disabled}, nil
}

// promptCancelled turns a cancelled interactive edit into a clean, non-error
// exit and lets any other prompt failure propagate.
func promptCancelled(cmd *cobra.Command, err error, action string) error {
	if errors.Is(err, errEditCancelled) {
		fmt.Fprintf(cmd.OutOrStdout(), "%s cancelled\n", action)
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
		if err := requireCommandEditable(manager); err != nil {
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
		short, err := cmd.Flags().GetBool("short")
		if err != nil {
			return err
		}
		return writeCommandTable(cmd.OutOrStdout(), commands, short)
	},
}

// writeCommandTable renders the predefined command list. When short is set the
// wide COMMAND column is omitted so the table fits on screen.
func writeCommandTable(w io.Writer, commands []model.Command, short bool) error {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	if short {
		fmt.Fprintln(tw, "NAME\tALIAS\tDISABLED")
	} else {
		fmt.Fprintln(tw, "NAME\tCOMMAND\tALIAS\tDISABLED")
	}
	for _, command := range commands {
		disabled := "no"
		if command.Disabled {
			disabled = "yes"
		}
		if short {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", command.Name, command.Alias, disabled)
		} else {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", command.Name, command.Command, command.Alias, disabled)
		}
	}
	return tw.Flush()
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
	if err := requireCommandEditable(manager); err != nil {
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

// errCommandReadOnly is returned when a command mutation is attempted from a
// remote sshex session, where command definitions are managed by the origin.
var errCommandReadOnly = errors.New("commands cannot be edited from a remote sshex session")

// commandManager abstracts command storage so the CLI can talk to the broker
// when a session is live, or edit the local config directly otherwise.
type commandManager interface {
	List(ctx context.Context) ([]model.Command, error)
	Get(ctx context.Context, name string) (model.Command, error)
	Add(ctx context.Context, name, command, alias string, disabled bool) error
	Update(ctx context.Context, name string, command model.Command) error
	Remove(ctx context.Context, name string) error
	SetDisabled(ctx context.Context, name string, disabled bool) error
	// CanManage reports whether command definitions may be edited. It is false
	// for a remote session, whose token lacks the commands capability.
	CanManage() bool
}

type remoteCommands struct {
	client    *client.Client
	canManage bool
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
	if !r.canManage {
		return errCommandReadOnly
	}
	_, err := r.client.AddCommand(ctx, name, command, alias, disabled)
	return err
}

func (r remoteCommands) Update(ctx context.Context, name string, command model.Command) error {
	if !r.canManage {
		return errCommandReadOnly
	}
	_, err := r.client.UpdateCommand(ctx, name, api.UpdateCommandRequest{
		Name:     command.Name,
		Command:  command.Command,
		Alias:    command.Alias,
		Disabled: command.Disabled,
	})
	return err
}

func (r remoteCommands) Remove(ctx context.Context, name string) error {
	if !r.canManage {
		return errCommandReadOnly
	}
	return r.client.RemoveCommand(ctx, name)
}

func (r remoteCommands) SetDisabled(ctx context.Context, name string, disabled bool) error {
	if !r.canManage {
		return errCommandReadOnly
	}
	return r.client.SetCommandDisabled(ctx, name, disabled)
}

func (r remoteCommands) CanManage() bool { return r.canManage }

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

func (l localCommands) CanManage() bool { return true }

// resolveCommandManager prefers the broker when a live session is discoverable
// so commands can be managed from a remote shell; otherwise it edits the local
// config directly. A discovered session that lacks the commands capability is a
// remote session and is returned read-only.
func resolveCommandManager() (commandManager, error) {
	if file, err := controlFile(); err == nil {
		return remoteCommands{
			client:    client.New(file),
			canManage: file.Capabilities.Contains(model.CapabilityCommands),
		}, nil
	}
	if application == nil || application.Commands == nil {
		return nil, errors.New("command manager unavailable")
	}
	return localCommands{commands: application.Commands}, nil
}

// requireCommandEditable fails fast when command definitions may not be edited,
// before any interactive prompt is shown.
func requireCommandEditable(manager commandManager) error {
	if !manager.CanManage() {
		return errCommandReadOnly
	}
	return nil
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

	commandListCmd.Flags().BoolP("short", "s", false, "Hide the command column")

	commandCmd.AddCommand(commandAddCmd, commandEditCmd, commandRmCmd, commandListCmd, commandEnableCmd, commandDisableCmd, commandHelpCmd)
}
