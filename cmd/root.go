package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/mcnull/sshex/internal/app"
	"github.com/mcnull/sshex/internal/assets"
	"github.com/mcnull/sshex/internal/config"
	"github.com/spf13/cobra"
)

var version = ""

var application *app.App

var configFlag string

// ExitError carries a command's exit code so the process can exit with it.
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("exit status %d", e.Code)
}

var rootCmd = &cobra.Command{
	Use:           "sshex",
	Short:         "Spawn ssh tunnels and execute commands from a remote shell",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		printHelp(cmd.OutOrStdout(), "sshex")
		return nil
	},
}

func Execute() {
	rootCmd.SetHelpFunc(helpFunc)
	rootCmd.SetUsageFunc(usageFunc)

	if err := rootCmd.Execute(); err != nil {
		var exitErr *ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		fmt.Fprintf(os.Stderr, "sshex: %v\n", err)
		os.Exit(1)
	}
}

func notImplemented(cmd *cobra.Command) {
	fmt.Fprintf(cmd.OutOrStdout(), "sshex: %s is not implemented yet\n", cmd.CommandPath())
}

// setupApplication loads the selected config file and builds the application.
// It runs after flag parsing so --config is available, and it exports the
// resolved path so child processes and the runtime directory agree on it.
func setupApplication(cmd *cobra.Command) error {
	if showBanner(cmd) {
		printBanner(cmd.OutOrStdout())
	}
	path, err := config.Resolve(configFlag)
	if err != nil {
		return err
	}
	if err := os.Setenv("SSHEX_CONFIG", path); err != nil {
		return err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	application, err = app.New(cfg, path)
	return err
}

func init() {
	if version == "" {
		version = assets.Version()
	}
	rootCmd.Version = version
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.PersistentFlags().StringVarP(&configFlag, "config", "c", "", "Config file to use (default ~/.config/sshex/config.yml)")
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		return setupApplication(cmd)
	}
	rootCmd.SetHelpCommand(helpCmd)
	rootCmd.AddCommand(connectCmd, tunnelCmd, execCmd, commandCmd, sessionCmd, serveCmd, uninstallCmd, completionsCmd, helpCmd)
}
