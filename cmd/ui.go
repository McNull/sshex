package cmd

import (
	"github.com/mcnull/sshex/internal/assets"
	"github.com/mcnull/sshex/internal/ui"
	"github.com/spf13/cobra"
)

var uiTheme string

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Open the interactive terminal interface",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runUI(cmd)
	},
}

// runUI starts the interactive interface for a command that has already
// validated its arguments and flags.
func runUI(cmd *cobra.Command) error {
	return ui.Run(ui.Options{
		Version:         assets.Version(),
		UpdateAvailable: updateAvailableVersion(),
		Theme:           uiTheme,
		In:              cmd.InOrStdin(),
		Out:             cmd.OutOrStdout(),
	})
}

// launchesUI reports whether the root command, invoked without arguments on an
// interactive terminal, should open the UI instead of printing help.
func launchesUI(cmd *cobra.Command) bool {
	return len(cmd.Flags().Args()) == 0 &&
		isTerminal(cmd.InOrStdin()) &&
		isTerminalWriter(cmd.OutOrStdout())
}

// runsUI reports whether cmd will open the interactive interface. The UI draws
// its own logo and update notice, so callers suppress the CLI versions.
func runsUI(cmd *cobra.Command) bool {
	if cmd == uiCmd {
		return true
	}
	return cmd == rootCmd && launchesUI(cmd)
}

func init() {
	uiCmd.Flags().StringVar(&uiTheme, "theme", "", "Color theme (default: mono)")
	_ = uiCmd.RegisterFlagCompletionFunc("theme", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return ui.Themes(), cobra.ShellCompDirectiveNoFileComp
	})
	rootCmd.Flags().StringVar(&uiTheme, "theme", "", "Color theme for the interactive interface (default: mono)")
	_ = rootCmd.RegisterFlagCompletionFunc("theme", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return ui.Themes(), cobra.ShellCompDirectiveNoFileComp
	})
}
