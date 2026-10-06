package cmd

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/mcnull/sshex/internal/client"
	"github.com/mcnull/sshex/internal/model"
	"github.com/mcnull/sshex/internal/service"
	"github.com/mcnull/sshex/internal/sessionfile"
	"github.com/spf13/cobra"
)

var tunnelCmd = &cobra.Command{
	Use:   "tunnel",
	Short: "Manage ssh tunnels",
	RunE: func(cmd *cobra.Command, _ []string) error {
		printHelp(cmd.OutOrStdout(), "sshex tunnel")
		return nil
	},
}

var tunnelAddCmd = &cobra.Command{
	Use:   "add [-L|-R] <port>[:<target>[:<remote-port>]]",
	Short: "Add a new ssh tunnel",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		spec, err := service.ParsePortSpec(args[0])
		if err != nil {
			return err
		}
		direction, err := tunnelDirection(cmd, model.ForwardLocal)
		if err != nil {
			return err
		}
		sessionID, _ := cmd.Flags().GetString("session")
		c, err := selectClient(sessionID)
		if err != nil {
			return err
		}
		tunnel, err := c.AddTunnel(cmd.Context(), spec.LocalPort, spec.TargetHost, spec.TargetPort, direction)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), formatTunnel(tunnel.Direction, tunnel.LocalPort, tunnel.TargetHost, tunnel.TargetPort, sessionTarget(cmd, c, sessionID)))
		fmt.Fprintln(cmd.OutOrStdout(), "tunnel created")
		return nil
	},
}

var tunnelRmCmd = &cobra.Command{
	Use:   "rm [-L|-R] <port>",
	Short: "Remove an existing ssh tunnel",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		port, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid port: %q", args[0])
		}
		direction, err := tunnelDirection(cmd, "")
		if err != nil {
			return err
		}
		sessionID, _ := cmd.Flags().GetString("session")
		c, err := selectClient(sessionID)
		if err != nil {
			return err
		}
		if err := c.RemoveTunnel(cmd.Context(), port, direction); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "tunnel removed")
		return nil
	},
}

var tunnelListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all active ssh tunnels",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		all, _ := cmd.Flags().GetBool("all")
		if all {
			return listAllTunnels(cmd)
		}
		sessionID, _ := cmd.Flags().GetString("session")
		c, err := selectClient(sessionID)
		if err != nil {
			return err
		}
		tunnels, err := c.ListTunnels(cmd.Context())
		if err != nil {
			return err
		}
		if len(tunnels) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "no active tunnels.")
			return nil
		}
		target := sessionTarget(cmd, c, sessionID)
		for _, tunnel := range tunnels {
			fmt.Fprintln(cmd.OutOrStdout(), formatTunnel(tunnel.Direction, tunnel.LocalPort, tunnel.TargetHost, tunnel.TargetPort, target))
		}
		return nil
	},
}

var tunnelHelpCmd = &cobra.Command{
	Use:   "help [command]",
	Short: "Show help for tunnel or one of its commands",
	RunE: func(cmd *cobra.Command, args []string) error {
		target, _, err := tunnelCmd.Find(args)
		if err != nil || target == nil {
			printHelp(cmd.OutOrStdout(), "sshex tunnel")
			return nil
		}
		printHelp(cmd.OutOrStdout(), target.CommandPath())
		return nil
	},
}

func discoverClient() (*client.Client, error) {
	file, err := sessionfile.Discover()
	if err != nil {
		return nil, err
	}
	return client.New(file), nil
}

// controlClient returns a client for some live session, preferring the default
// one. The broker's token authorizes every session of the origin, so any live
// session file is enough to reach it.
func controlClient() (*client.Client, error) {
	file, err := sessionfile.Discover()
	if err == nil {
		return client.New(file), nil
	}
	if errors.Is(err, sessionfile.ErrAmbiguous) {
		active, listErr := sessionfile.ListActive()
		if listErr == nil && len(active) > 0 {
			return client.New(active[0]), nil
		}
	}
	return nil, err
}

// selectClient resolves a client for an explicit session id, or the default
// session when the id is empty.
func selectClient(sessionID string) (*client.Client, error) {
	if sessionID == "" {
		return discoverClient()
	}
	c, err := controlClient()
	if err != nil {
		return nil, err
	}
	return c.ForSession(sessionID), nil
}

// sessionTarget returns the display target for the selected session.
func sessionTarget(cmd *cobra.Command, c *client.Client, sessionID string) string {
	if sessionID == "" {
		return c.Target()
	}
	session, err := c.GetSession(cmd.Context())
	if err != nil {
		return sessionID
	}
	if session.User != "" {
		return session.User + "@" + session.Host
	}
	return session.Host
}

func listAllTunnels(cmd *cobra.Command) error {
	c, err := controlClient()
	if err != nil {
		return err
	}
	sessions, err := c.ListSessions(cmd.Context())
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	total := 0
	for _, session := range sessions {
		tunnels, err := c.ForSession(session.ID).ListTunnels(cmd.Context())
		if err != nil {
			return err
		}
		if len(tunnels) == 0 {
			continue
		}
		target := session.Host
		if session.User != "" {
			target = session.User + "@" + target
		}
		fmt.Fprintf(out, "%s  %s\n", session.ID, target)
		for _, tunnel := range tunnels {
			fmt.Fprintf(out, "  %s\n", formatTunnel(tunnel.Direction, tunnel.LocalPort, tunnel.TargetHost, tunnel.TargetPort, ""))
		}
		total += len(tunnels)
	}
	if total == 0 {
		fmt.Fprintln(out, "no active tunnels.")
	}
	return nil
}

func formatTunnel(direction model.ForwardDirection, localPort int, targetHost string, targetPort int, target string) string {
	arrow := ">"
	if direction == model.ForwardRemote {
		arrow = "<"
	}
	line := fmt.Sprintf("localhost:%d %s %s:%d", localPort, arrow, targetHost, targetPort)
	if target != "" {
		line += " " + target
	}
	return line
}

func completeSessionIDs(_ *cobra.Command, _ []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
	c, err := controlClient()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	sessions, err := c.ListSessions(context.Background())
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	comps := make([]cobra.Completion, 0, len(sessions))
	for _, s := range sessions {
		desc := s.Host
		if s.User != "" {
			desc = s.User + "@" + s.Host
		}
		if s.Origin != "" {
			desc += " (" + s.Origin + ")"
		}
		comps = append(comps, cobra.CompletionWithDesc(s.ID, desc))
	}
	return comps, cobra.ShellCompDirectiveNoFileComp
}

func tunnelDirection(cmd *cobra.Command, fallback model.ForwardDirection) (model.ForwardDirection, error) {
	local, _ := cmd.Flags().GetBool("local")
	remote, _ := cmd.Flags().GetBool("remote")
	if local && remote {
		return "", fmt.Errorf("cannot combine -L and -R")
	}
	if remote {
		return model.ForwardRemote, nil
	}
	if local {
		return model.ForwardLocal, nil
	}
	return fallback, nil
}

func init() {
	tunnelAddCmd.Flags().BoolP("local", "L", false, "Create a local forward (default)")
	tunnelAddCmd.Flags().BoolP("remote", "R", false, "Create a remote forward")
	tunnelAddCmd.Flags().StringP("session", "s", "", "Target a specific session id")
	tunnelRmCmd.Flags().BoolP("local", "L", false, "Remove a local forward")
	tunnelRmCmd.Flags().BoolP("remote", "R", false, "Remove a remote forward")
	tunnelRmCmd.Flags().StringP("session", "s", "", "Target a specific session id")
	tunnelListCmd.Flags().StringP("session", "s", "", "Target a specific session id")
	tunnelListCmd.Flags().Bool("all", false, "List tunnels across all sessions")

	for _, c := range []*cobra.Command{tunnelAddCmd, tunnelRmCmd, tunnelListCmd} {
		_ = c.RegisterFlagCompletionFunc("session", completeSessionIDs)
	}

	tunnelCmd.AddCommand(tunnelAddCmd, tunnelRmCmd, tunnelListCmd, tunnelHelpCmd)
}
