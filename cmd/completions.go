package cmd

import (
	"fmt"
	"path"

	"github.com/spf13/cobra"
)

var completionsCmd = &cobra.Command{
	Use:   "completions <shell>",
	Short: "Generate shell completions for sshex",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		shell := path.Base(args[0])
		switch shell {
		case "bash":
			return rootCmd.GenBashCompletionV2(out, true)
		case "zsh":
			return rootCmd.GenZshCompletion(out)
		case "fish":
			return rootCmd.GenFishCompletion(out, true)
		default:
			return fmt.Errorf("unsupported shell: %q (expected bash, zsh, or fish)", args[0])
		}
	},
}
