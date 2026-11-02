package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/mcnull/sshex/internal/app"
	"github.com/mcnull/sshex/internal/assets"
	"github.com/mcnull/sshex/internal/config"
	"github.com/mcnull/sshex/internal/updater"
	"github.com/mcnull/sshex/internal/versioncheck"
	"github.com/spf13/cobra"
)

var version = ""

var application *app.App

var configFlag string

var versionCheck bool

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
	checkVersionState(cmd)
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
	rootCmd.PersistentFlags().BoolVar(&versionCheck, "version-check", false, "Perform online version check and exit")
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		if cmd == updateCmd || cmd == versionCheckCmd {
			return nil
		}
		if versionCheck {
			path, err := versioncheck.DefaultPath()
			if err != nil {
				return err
			}
			if err := checkVersionSynchronously(cmd, path, updater.New(updater.Config{})); err != nil {
				return err
			}
			return &ExitError{Code: 0}
		}
		return setupApplication(cmd)
	}
	rootCmd.SetHelpCommand(helpCmd)
	rootCmd.AddCommand(connectCmd, tunnelCmd, execCmd, commandCmd, sessionCmd, uiCmd, serveCmd, uninstallCmd, updateCmd, completionsCmd, helpCmd, versionCheckCmd)
}

var versionCheckCmd = &cobra.Command{
	Use:    "__version-check <state-file>",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
		defer cancel()
		return versioncheck.RunWorker(ctx, args[0], updater.New(updater.Config{}), time.Now)
	},
}

const versionCheckCommandName = "__version-check"

// checkVersionState reports cached update information, then starts a detached
// worker if the last check is due. Neither operation performs network I/O on
// the CLI's startup path.
func checkVersionState(cmd *cobra.Command) {
	path, err := versioncheck.DefaultPath()
	if err != nil {
		return
	}
	if cmd != uiCmd && !bannerSuppressed() {
		if available := cachedUpdateVersion(path); available != "" {
			printUpdateAvailable(cmd.ErrOrStderr(), available)
		}
	}
	versioncheck.StartIfDue(path, launchVersionCheck, time.Now())
}

// updateAvailableVersion reports the cached newer version, if any. The UI
// displays it below the logo instead of printing the CLI notice.
func updateAvailableVersion() string {
	path, err := versioncheck.DefaultPath()
	if err != nil {
		return ""
	}
	return cachedUpdateVersion(path)
}

func cachedUpdateVersion(path string) string {
	state, err := versioncheck.NewStore(path).Load()
	if err != nil || !versioncheck.IsNewer(state.AvailableVersion, assets.Version()) {
		return ""
	}
	return state.AvailableVersion
}

func checkVersionSynchronously(cmd *cobra.Command, path string, source versioncheck.LatestSource) error {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "Checking for sshex updates...")
	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()
	if err := versioncheck.RunWorker(ctx, path, source, time.Now); err != nil {
		if errors.Is(err, versioncheck.ErrCheckInProgress) {
			fmt.Fprintln(out, "An update check is already in progress.")
			return &ExitError{Code: 1}
		}
		fmt.Fprintf(out, "Update check failed: %v\n", err)
		return &ExitError{Code: 1}
	}
	state, err := versioncheck.NewStore(path).Load()
	if err != nil {
		fmt.Fprintf(out, "Update check completed, but reading its result failed: %v\n", err)
		return &ExitError{Code: 1}
	}
	if state.AvailableVersion == "" {
		fmt.Fprintln(out, "Update check completed, but no online version was available.")
		return &ExitError{Code: 1}
	}
	if versioncheck.IsNewer(state.AvailableVersion, assets.Version()) {
		printUpdateAvailable(out, state.AvailableVersion)
		return nil
	}
	fmt.Fprintf(out, "sshex is up to date (v%s).\n", assets.Version())
	return nil
}

func launchVersionCheck(path string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(executable, versionCheckCommandName, path)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer devNull.Close()
	cmd.Stdout = devNull
	cmd.Stderr = devNull
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
