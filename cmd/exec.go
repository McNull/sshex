package cmd

import (
	"context"
	"os"
	"strings"

	"github.com/mcnull/sshex/internal/api"
	"github.com/mcnull/sshex/internal/pathx"
	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:               "exec <command> [args...]",
	Short:             "Execute a predefined command on the origin",
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: completeExecCommandNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionID, _ := cmd.Flags().GetString("session")
		c, err := selectClient(sessionID)
		if err != nil {
			return err
		}
		cwd, err := os.Getwd()
		if err != nil {
			cwd = ""
		}
		home, _ := os.UserHomeDir()
		execArgs := pathx.ExpandArgs(cwd, home, args[1:])
		code, err := c.Exec(cmd.Context(), api.ExecRequest{
			Name: args[0],
			Args: execArgs,
			Cwd:  cwd,
			User: os.Getenv("USER"),
		}, cmd.OutOrStdout(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		if err != nil {
			return err
		}
		if code != 0 {
			return &ExitError{Code: code}
		}
		return nil
	},
}

func init() {
	execCmd.Flags().StringP("session", "s", "", "Target a specific session id")
	execCmd.Flags().SetInterspersed(false)
	_ = execCmd.RegisterFlagCompletionFunc("session", completeSessionIDs)
}

// completeExecCommandNames completes the first positional argument with the
// available command names. Once a command has been chosen, completion falls
// back to the shell's default (file) behavior so arguments can be completed
// from the filesystem.
func completeExecCommandNames(_ *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveDefault
	}
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
		if command.Disabled {
			continue
		}
		if !strings.HasPrefix(command.Name, toComplete) {
			continue
		}
		comps = append(comps, command.Name)
	}
	return comps, cobra.ShellCompDirectiveNoFileComp
}
