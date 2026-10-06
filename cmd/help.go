package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

const rootHelp = `  Spawn ssh tunnels and execute commands from a remote shell.

  sshex [options] [<command>] [<args>]

  Options:
    -h, --help            Show this help message and exit
    -v, --version         Show version information and exit

  Commands:
    connect               Connect to a remote machine via ssh
    tunnel                Manage ssh tunnels
    exec                  Execute a predefined command on the origin
    command               Manage predefined commands
    session               Manage sshex sessions
    uninstall             Remove the sshex installation from a machine
    completions           Generate shell completions for sshex
    help                  Show this help message and exit

  Type sshex <command> [help|-h|--help] for more information on a specific command.
`

const connectHelp = `  Connect to a remote machine via ssh.

  Usage:
    sshex connect [options] [<user>@]<host>

  Options:
    -p, --port <port>     Specify the port to connect to (default: 22)
    -i, --identity <file> Specify the identity file (private key) to use
    -J, --jump <hosts>    Jump host(s) to connect through (comma separated)
    -o, --option <option> Pass an ssh option (key=value), or raw ssh args when
                          the value starts with '-'
        --no-completions  Do not install shell completions on the remote
    -h, --help            Show this help message and exit
`

const uninstallHelp = `  Remove the sshex installation from a machine.

  Usage:
    sshex uninstall [options] [<user>@]<host>

  Arguments:
    [<user>@]<host>       The machine to uninstall from. Defaults to this machine.

  Options:
    -p, --port <port>     Specify the port to connect to (default: 22)
    -i, --identity <file> Specify the identity file (private key) to use
    -J, --jump <hosts>    Jump host(s) to connect through (comma separated)
    -o, --option <option> Pass an ssh option (key=value), or raw ssh args when
                          the value starts with '-'
    -y, --yes             Do not prompt for confirmation
        --force           Uninstall even if other sshex sessions are active
    -h, --help            Show this help message and exit
`

const tunnelHelp = `  Manage ssh tunnels.

  Usage:
    sshex tunnel [options] <command> [<args>]

  Commands:
    add                   Add a new ssh tunnel
    rm                    Remove an existing ssh tunnel
    list                  List all active ssh tunnels
    help                  Show this help message and exit

  Options:
    -h, --help            Show this help message and exit
`

const tunnelAddHelp = `  Add a new ssh tunnel.

  Usage:
    sshex tunnel add [-L|-R] <port>[:<target>[:<remote-port>]]

  Options:
    -L, --local           Create a local forward (default). The local machine
                          listens on <port> and forwards to <target>:<remote-port>
                          via the remote machine.
    -R, --remote          Create a remote forward. The remote machine listens on
                          <port> and forwards to <target>:<remote-port> on the
                          local machine.
    -s, --session <id>    Target a specific session id instead of this shell's
                          session.
    -h, --help            Show this help message and exit

  Examples:
    # local forward from localhost:8080 to localhost:8080 via remote-machine:
    $ sshex tunnel add 8080

    # local forward from localhost:8080 to localhost:80 via remote-machine:
    $ sshex tunnel add 8080:localhost:80

    # local forward from localhost:8080 to server01:80 via remote-machine:
    $ sshex tunnel add 8080:server01:80

    # remote forward from remote-machine:8080 to localhost:80:
    $ sshex tunnel add -R 8080:localhost:80
`

const tunnelRmHelp = `  Remove an existing ssh tunnel.

  Usage:
    sshex tunnel rm [-L|-R] <port>

  Arguments:
    <port>                The port of the tunnel to remove

  Options:
    -L, --local           Remove a local forward
    -R, --remote          Remove a remote forward
    -s, --session <id>    Target a specific session id instead of this shell's
                          session.
    -h, --help            Show this help message and exit
`

const tunnelListHelp = `  List all active ssh tunnels.

  Usage:
    sshex tunnel list [options]

  Options:
    -s, --session <id>    List tunnels for a specific session id instead of this
                          shell's session.
        --all             List tunnels across all sessions of this machine.
    -h, --help            Show this help message and exit
`

const sessionHelp = `  Manage sshex sessions.

  Usage:
    sshex session <command> [<args>]

  Commands:
    list                  List active sshex sessions
    close                 Close a session and drop its tunnels
    help                  Show this help message and exit

  Options:
    -h, --help            Show this help message and exit
`

const sessionListHelp = `  List active sshex sessions.

  Usage:
    sshex session list

  Options:
    -h, --help            Show this help message and exit

  The session running this command is marked with *.
`

const sessionCloseHelp = `  Close a session and drop its tunnels.

  Usage:
    sshex session close <id>

  Arguments:
    <id>                  The session id to close

  Options:
    -h, --help            Show this help message and exit
`

const completionsHelp = `  Generate shell completions for sshex.

  Usage:
    sshex completions <shell>

  Arguments:
    <shell>               The shell to generate completions for
                          (bash, zsh, or fish)

  Options:
    -h, --help            Show this help message and exit
`

const helpHelp = `  Show help for sshex or one of its commands.

  Usage:
    sshex help [<command>]

  Arguments:
    <command>             Command to show help for

  Options:
    -h, --help            Show this help message and exit
`

const helpHelpHelp = `  You really need help.
`

const serveHelp = `  Run the sshex service.

  Usage:
    sshex serve

  Options:
    -h, --help            Show this help message and exit
`

const execHelpPrefix = `  Execute a predefined command on the origin machine.

  Usage:
    sshex exec [options] <command> [<args>...]

  Arguments:
    <command>             The predefined command to execute
    <args>                Arguments passed to the command

  Options:
    -s, --session <id>    Target a specific session id instead of this shell's
                          session.
    -h, --help            Show this help message and exit

`

const execHelpSuffix = `
  Arguments that are explicit relative paths (., .., ./x, ../x, ~/x, ~) are
  expanded to absolute paths before they are bound to the template.
`

var execHelp = execHelpPrefix + formatCommandVariablesHelp("  ") + execHelpSuffix

const commandHelp = `  Manage predefined commands.

  Usage:
    sshex command <command> [<args>]

  Commands:
    add                   Add a predefined command
    edit                  Edit a predefined command field by field
    rm                    Remove a predefined command
    list                  List predefined commands
    enable                Enable a predefined command
    disable               Disable a predefined command
    help                  Show this help message and exit

  Options:
    -h, --help            Show this help message and exit
`

const commandAddHelpPrefix = `  Add a predefined command.

  Usage:
    sshex command add [options] [<name> <command>...]

  Arguments:
    <name>                The name used by sshex exec
    <command>             The command line executed on the origin. Quote it
                          with single quotes so the shell does not expand the
                          template variables before sshex stores them.

  With no arguments, sshex opens the same field-by-field editor as
  ` + "`sshex command edit`" + `, starting from empty values. The --alias and
  --disabled options pre-fill the editor.

  Options:
        --alias [<alias>] Install a shell alias for the command on the remote.
                          Omit the value to use the command name as the alias.
        --disabled        Add the command in a disabled state
    -h, --help            Show this help message and exit

`

var commandAddHelp = commandAddHelpPrefix + formatCommandVariablesHelp("  ") + formatCommandExamplesAdd()

const commandEditHelp = `  Edit a predefined command field by field.

  Usage:
    sshex command edit <name>

  Arguments:
    <name>                The name of the command to edit

  Each field is preceded by a short description and shown with its current
  value; press Enter to keep it. The name field can be changed to rename the
  command. The command field lists the available template variables. The name
  and command are required; the name and alias must be unique among commands.
  Leaving the alias empty clears it rather than keeping it. An invalid answer
  is reported and the field is asked again.

  Options:
    -h, --help            Show this help message and exit
`

const commandRmHelp = `  Remove a predefined command.

  Usage:
    sshex command rm <name>

  Arguments:
    <name>                The name of the command to remove

  Options:
    -h, --help            Show this help message and exit
`

const commandListHelp = `  List predefined commands.

  Usage:
    sshex command list

  Options:
    -h, --help            Show this help message and exit
`

const commandEnableHelp = `  Enable a predefined command.

  Usage:
    sshex command enable <name>

  Arguments:
    <name>                The name of the command to enable

  Options:
    -h, --help            Show this help message and exit
`

const commandDisableHelp = `  Disable a predefined command.

  Usage:
    sshex command disable <name>

  Arguments:
    <name>                The name of the command to disable

  Options:
    -h, --help            Show this help message and exit
`

var helpTexts = map[string]string{
	"sshex":                 rootHelp,
	"sshex connect":         connectHelp,
	"sshex uninstall":       uninstallHelp,
	"sshex tunnel":          tunnelHelp,
	"sshex tunnel add":      tunnelAddHelp,
	"sshex tunnel rm":       tunnelRmHelp,
	"sshex tunnel list":     tunnelListHelp,
	"sshex session":         sessionHelp,
	"sshex session list":    sessionListHelp,
	"sshex session close":   sessionCloseHelp,
	"sshex exec":            execHelp,
	"sshex command":         commandHelp,
	"sshex command add":     commandAddHelp,
	"sshex command edit":    commandEditHelp,
	"sshex command rm":      commandRmHelp,
	"sshex command list":    commandListHelp,
	"sshex command enable":  commandEnableHelp,
	"sshex command disable": commandDisableHelp,
	"sshex completions":     completionsHelp,
	"sshex serve":           serveHelp,
	"sshex help":            helpHelp,
}

var helpCmd = &cobra.Command{
	Use:   "help [command]",
	Short: "Show help for sshex or one of its commands",
	RunE:  runHelp,
}

func runHelp(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	if len(args) == 0 {
		printHelp(out, "sshex")
		return nil
	}
	target, _, err := rootCmd.Find(args)
	if err != nil || target == nil {
		printHelp(out, "sshex")
		return nil
	}
	if target.CommandPath() == "sshex help" {
		fmt.Fprint(out, helpHelpHelp)
		return nil
	}
	printHelp(out, target.CommandPath())
	return nil
}

func printHelp(w io.Writer, path string) {
	for path != "" {
		if text, ok := helpTexts[path]; ok {
			fmt.Fprint(w, text)
			return
		}
		idx := strings.LastIndex(path, " ")
		if idx < 0 {
			break
		}
		path = path[:idx]
	}
	fmt.Fprint(w, rootHelp)
}

func helpFunc(c *cobra.Command, _ []string) {
	printHelp(c.OutOrStdout(), c.CommandPath())
}

func usageFunc(c *cobra.Command) error {
	printHelp(c.OutOrStderr(), c.CommandPath())
	return nil
}
