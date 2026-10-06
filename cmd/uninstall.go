package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mcnull/sshex/internal/service"
	"github.com/mcnull/sshex/internal/sessionfile"
	"github.com/spf13/cobra"
)

const uninstallDetails = `- the sshex command and the ~/.local/bin/sshex symlink
- shell completions, shell aliases and their shell startup wiring (bash, zsh, fish)
- session state and runtime sockets

`

var uninstallCmd = &cobra.Command{
	Use:               "uninstall [<user>@]<host>",
	Short:             "Remove the sshex installation from a machine",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeConnectTarget,
	RunE: func(cmd *cobra.Command, args []string) error {
		var user, host string
		if len(args) == 1 {
			user, host = splitTarget(args[0])
		}
		port, _ := cmd.Flags().GetInt("port")
		identity, _ := cmd.Flags().GetString("identity")
		jump, _ := cmd.Flags().GetString("jump")
		options, _ := cmd.Flags().GetStringArray("option")
		yes, _ := cmd.Flags().GetBool("yes")
		force, _ := cmd.Flags().GetBool("force")

		target := "this machine"
		if host != "" {
			target = host
			if user != "" {
				target = user + "@" + host
			}
		}

		out := cmd.OutOrStdout()
		if !yes {
			ok, err := confirmUninstall(cmd.InOrStdin(), out, target)
			if err != nil {
				return err
			}
			if !ok {
				fmt.Fprintln(out, "aborted.")
				return nil
			}
		}

		fmt.Fprintf(out, "uninstalling sshex from %s...\n", target)
		err := application.Installs.Uninstall(cmd.Context(), service.UninstallRequest{
			User:         user,
			Host:         host,
			Port:         port,
			IdentityFile: identity,
			JumpHosts:    jump,
			Options:      options,
			Force:        force,
		})
		if err != nil {
			var active *service.ActiveSessionsError
			if errors.As(err, &active) {
				printActiveSessions(out, active.Sessions)
				return fmt.Errorf("aborting: sessions are still active; close them or re-run with --force")
			}
			return err
		}
		fmt.Fprintln(out, "done.")
		return nil
	},
}

func printActiveSessions(out io.Writer, sessions []sessionfile.File) {
	fmt.Fprintln(out, "active sshex sessions:")
	for _, s := range sessions {
		line := "  " + s.ID
		if s.Target != "" {
			line += "  " + s.Target
		}
		if s.Origin != "" {
			line += "  from " + s.Origin
		}
		fmt.Fprintln(out, line)
	}
}

func confirmUninstall(in io.Reader, out io.Writer, target string) (bool, error) {
	fmt.Fprintf(out, "This will remove the following from %s:\n", target)
	fmt.Fprint(out, uninstallDetails)
	fmt.Fprint(out, "Continue? (y/n): ")
	line, _ := bufio.NewReader(in).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

func init() {
	uninstallCmd.Flags().IntP("port", "p", 22, "Specify the port to connect to")
	uninstallCmd.Flags().StringP("identity", "i", "", "Specify the identity file (private key) to use")
	uninstallCmd.Flags().StringP("jump", "J", "", "Jump host(s) to connect through, comma separated")
	uninstallCmd.Flags().StringArrayP("option", "o", nil, "Pass an ssh option (key=value); a value starting with '-' is passed as raw ssh args")
	uninstallCmd.Flags().BoolP("yes", "y", false, "Do not prompt for confirmation")
	uninstallCmd.Flags().Bool("force", false, "Uninstall even if other sshex sessions are active")
}
