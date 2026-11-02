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
		return ui.Run(ui.Options{
			Version:         assets.Version(),
			UpdateAvailable: updateAvailableVersion(),
			Theme:           uiTheme,
			In:              cmd.InOrStdin(),
			Out:             cmd.OutOrStdout(),
		})
	},
}

func init() {
	uiCmd.Flags().StringVar(&uiTheme, "theme", "", "Color theme (default: mono)")
	_ = uiCmd.RegisterFlagCompletionFunc("theme", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return ui.Themes(), cobra.ShellCompDirectiveNoFileComp
	})
}
