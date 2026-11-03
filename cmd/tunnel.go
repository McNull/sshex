package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"text/tabwriter"

	"github.com/mcnull/sshex/internal/api"
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
		c, err := scopeFromFlags(cmd).mutationClient(sessionID)
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
	Use:   "rm [-L|-R] <port> | --id <tunnel-id>",
	Short: "Remove an existing ssh tunnel",
	Args: func(cmd *cobra.Command, args []string) error {
		idSet := cmd.Flags().Changed("id")
		if idSet {
			id, _ := cmd.Flags().GetString("id")
			if id == "" {
				return fmt.Errorf("tunnel id must not be empty")
			}
			if len(args) != 0 {
				return fmt.Errorf("tunnel port cannot be combined with --id")
			}
			return nil
		}
		return cobra.ExactArgs(1)(cmd, args)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		direction, err := tunnelDirection(cmd, "")
		if err != nil {
			return err
		}
		sessionID, _ := cmd.Flags().GetString("session")
		c, err := scopeFromFlags(cmd).mutationClient(sessionID)
		if err != nil {
			return err
		}
		idSet := cmd.Flags().Changed("id")
		id, _ := cmd.Flags().GetString("id")
		if idSet {
			if direction != "" {
				return fmt.Errorf("cannot combine --id with -L or -R")
			}
			err = c.RemoveTunnelByID(cmd.Context(), id)
		} else {
			var port int
			port, err = strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid port: %q", args[0])
			}
			err = c.RemoveTunnel(cmd.Context(), port, direction)
		}
		if err != nil {
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
		sessionID, _ := cmd.Flags().GetString("session")
		scope := scopeFromFlags(cmd)
		if shouldListAllTunnels(all, sessionID, scope, currentSessionID()) {
			return listAllTunnels(cmd, scope)
		}
		c, err := scope.sessionClient(sessionID)
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
		rows := make([]tunnelListRow, 0, len(tunnels))
		for _, tunnel := range tunnels {
			rows = append(rows, tunnelListRow{
				id:        tunnel.ID,
				sessionID: c.SessionID(),
				target:    target,
				tunnel:    formatTunnel(tunnel.Direction, tunnel.LocalPort, tunnel.TargetHost, tunnel.TargetPort, ""),
			})
		}
		renderTunnelList(cmd.OutOrStdout(), rows)
		return nil
	},
}

// shouldListAllTunnels defaults origin scope to every local session and client
// scope to the current session, unless a session is named explicitly.
func shouldListAllTunnels(all bool, sessionID string, scope controlScope, remoteSessionID string) bool {
	return all || (sessionID == "" && (scope == scopeOrigin || remoteSessionID == ""))
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

func listAllTunnels(cmd *cobra.Command, scope controlScope) error {
	c, err := scope.client()
	if err != nil {
		if errors.Is(err, sessionfile.ErrNone) && scope == scopeOrigin {
			fmt.Fprintln(cmd.OutOrStdout(), "No active sessions.")
			return nil
		}
		return err
	}
	sessions, err := c.ListSessions(cmd.Context())
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	rows := make([]tunnelListRow, 0)
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
		for _, tunnel := range tunnels {
			rows = append(rows, tunnelListRow{
				id:        tunnel.ID,
				sessionID: session.ID,
				target:    target,
				tunnel:    formatTunnel(tunnel.Direction, tunnel.LocalPort, tunnel.TargetHost, tunnel.TargetPort, ""),
			})
		}
	}
	if len(rows) == 0 {
		fmt.Fprintln(out, "no active tunnels.")
		return nil
	}
	renderTunnelList(out, rows)
	return nil
}

type tunnelListRow struct {
	id        string
	sessionID string
	target    string
	tunnel    string
}

func renderTunnelList(out io.Writer, rows []tunnelListRow) {
	w := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	defer w.Flush()
	fmt.Fprintln(w, "ID\tSESSION\tTARGET\tTUNNEL")
	for _, row := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", row.id, row.sessionID, row.target, row.tunnel)
	}
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

func completeSessionIDs(cmd *cobra.Command, _ []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
	return completeSessionIDsForScope(scopeFromFlags(cmd))
}

func completeAutoSessionIDs(_ *cobra.Command, _ []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
	c, err := autoClient()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return sessionIDCompletions(c), cobra.ShellCompDirectiveNoFileComp
}

func completeSessionIDsForScope(scope controlScope) ([]cobra.Completion, cobra.ShellCompDirective) {
	c, err := scope.client()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return sessionIDCompletions(c), cobra.ShellCompDirectiveNoFileComp
}

func sessionIDCompletions(c *client.Client) []cobra.Completion {
	sessions, err := c.ListSessions(context.Background())
	if err != nil {
		return nil
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
	return comps
}

func completeTunnelIDs(cmd *cobra.Command, _ []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
	scope := scopeFromFlags(cmd)
	sessionID, _ := cmd.Flags().GetString("session")
	if shouldListAllTunnels(false, sessionID, scope, currentSessionID()) {
		c, err := scope.client()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		sessions, err := c.ListSessions(context.Background())
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		var tunnels []api.TunnelResponse
		for _, session := range sessions {
			items, err := c.ForSession(session.ID).ListTunnels(context.Background())
			if err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			tunnels = append(tunnels, items...)
		}
		return tunnelIDCompletions(tunnels, sessions), cobra.ShellCompDirectiveNoFileComp
	}

	c, err := scope.sessionClient(sessionID)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	tunnels, err := c.ListTunnels(context.Background())
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	session, err := c.GetSession(context.Background())
	if err != nil {
		return tunnelIDCompletions(tunnels, nil), cobra.ShellCompDirectiveNoFileComp
	}
	return tunnelIDCompletions(tunnels, []api.SessionResponse{session}), cobra.ShellCompDirectiveNoFileComp
}

func tunnelIDCompletions(tunnels []api.TunnelResponse, sessions []api.SessionResponse) []cobra.Completion {
	sessionByID := make(map[string]api.SessionResponse, len(sessions))
	for _, session := range sessions {
		sessionByID[session.ID] = session
	}
	completions := make([]cobra.Completion, 0, len(tunnels))
	for _, tunnel := range tunnels {
		desc := fmt.Sprintf("%s localhost:%d %s %s:%d", tunnel.SessionID, tunnel.LocalPort, tunnelArrow(tunnel.Direction), tunnel.TargetHost, tunnel.TargetPort)
		if session, ok := sessionByID[tunnel.SessionID]; ok {
			target := session.Host
			if session.User != "" {
				target = session.User + "@" + target
			}
			desc = fmt.Sprintf("%s (%s) localhost:%d %s %s:%d", tunnel.SessionID, target, tunnel.LocalPort, tunnelArrow(tunnel.Direction), tunnel.TargetHost, tunnel.TargetPort)
		}
		completions = append(completions, cobra.CompletionWithDesc(tunnel.ID, desc))
	}
	return completions
}

func tunnelArrow(direction model.ForwardDirection) string {
	if direction == model.ForwardRemote {
		return "<"
	}
	return ">"
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
	tunnelCmd.PersistentFlags().Bool("client", false, "Use the current sshex session's origin instead of the local machine")
	tunnelAddCmd.Flags().BoolP("local", "L", false, "Create a local forward (default)")
	tunnelAddCmd.Flags().BoolP("remote", "R", false, "Create a remote forward")
	tunnelAddCmd.Flags().StringP("session", "s", "", "Target a specific session id (required when multiple sessions are active)")
	tunnelRmCmd.Flags().BoolP("local", "L", false, "Remove a local forward")
	tunnelRmCmd.Flags().BoolP("remote", "R", false, "Remove a remote forward")
	tunnelRmCmd.Flags().String("id", "", "Remove a tunnel by its id instead of port")
	tunnelRmCmd.Flags().StringP("session", "s", "", "Target a specific session id (required when multiple sessions are active)")
	tunnelListCmd.Flags().StringP("session", "s", "", "Target a specific session id")
	tunnelListCmd.Flags().Bool("all", false, "List tunnels across all sessions (the default on the origin)")

	for _, c := range []*cobra.Command{tunnelAddCmd, tunnelRmCmd, tunnelListCmd} {
		_ = c.RegisterFlagCompletionFunc("session", completeSessionIDs)
	}
	_ = tunnelRmCmd.RegisterFlagCompletionFunc("id", completeTunnelIDs)

	tunnelCmd.AddCommand(tunnelAddCmd, tunnelRmCmd, tunnelListCmd, tunnelHelpCmd)
}
