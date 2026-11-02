package cmd

import (
	"fmt"
	"os"

	"github.com/mcnull/sshex/internal/assets"
	"github.com/mcnull/sshex/internal/updater"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update sshex to the latest release",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		executable, err := os.Executable()
		if err != nil {
			return fmt.Errorf("locate sshex executable: %w", err)
		}
		result, err := updater.New(updater.Config{}).Update(cmd.Context(), assets.Version(), executable)
		if err != nil {
			return err
		}
		if !result.Updated {
			fmt.Fprintf(cmd.OutOrStdout(), "sshex is already up to date (v%s).\n", result.CurrentVersion)
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Updated sshex from v%s to v%s.\n", result.CurrentVersion, result.LatestVersion)
		return nil
	},
}
