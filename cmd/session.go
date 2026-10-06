package cmd

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/mcnull/sshex/internal/api"
	"github.com/mcnull/sshex/internal/sshx"
	"github.com/mcnull/sshex/internal/sshx/openssh"
	"github.com/spf13/cobra"
)

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage sshex sessions",
	RunE: func(cmd *cobra.Command, _ []string) error {
		printHelp(cmd.OutOrStdout(), "sshex session")
		return nil
	},
}

var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List active sshex sessions",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		c, err := controlClient()
		if err != nil {
			return err
		}
		sessions, err := c.ListSessions(cmd.Context())
		if err != nil {
			return err
		}
		if len(sessions) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "no active sessions.")
			return nil
		}
		hops := make([]string, len(sessions))
		for i, s := range sessions {
			hops[i] = sessionHopsLabel(cmd.Context(), s)
		}
		renderSessions(cmd.OutOrStdout(), sessions, c.SessionID(), hops)
		return nil
	},
}

var sessionCloseCmd = &cobra.Command{
	Use:               "close <id>",
	Short:             "Close a session and drop its connections and tunnels",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeSessionIDs,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := controlClient()
		if err != nil {
			return err
		}
		dropped, err := c.CloseSession(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "session closed")
		if len(dropped) > 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "dropped tunnels:")
			for _, tunnel := range dropped {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", formatTunnel(tunnel.Direction, tunnel.LocalPort, tunnel.TargetHost, tunnel.TargetPort, ""))
			}
		}
		return nil
	},
}

func renderSessions(out io.Writer, sessions []api.SessionResponse, current string, hops []string) {
	w := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	defer w.Flush()
	fmt.Fprintln(w, "ID\tTARGET\tORIGIN\tSTATE\tCONNECTIONS\tHOPS\tSTALE")
	for i, s := range sessions {
		id := s.ID
		if id == current {
			id += " *"
		}
		target := s.Host
		if s.User != "" {
			target = s.User + "@" + target
		}
		stale := "no"
		if s.Stale {
			stale = "yes"
		}
		hop := "0"
		if i < len(hops) {
			hop = hops[i]
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\t%s\n", id, target, s.Origin, s.State, s.Connections, hop, stale)
	}
}

// resolveProxy probes ssh configuration for a session's effective proxy path.
// It is a variable so tests can substitute a hermetic implementation.
var resolveProxy = openssh.ResolveProxy

// hopCount returns the number of ProxyJump hops in a comma-separated chain.
func hopCount(jumpHosts string) int {
	n := 0
	for _, hop := range strings.Split(jumpHosts, ",") {
		if strings.TrimSpace(hop) != "" {
			n++
		}
	}
	return n
}

// sessionHopsLabel reports the hop count for the HOPS column. Sessions created
// with -J carry JumpHosts directly; otherwise the count is resolved from ssh
// configuration. A ProxyCommand makes the count unknowable and is shown as "?".
func sessionHopsLabel(ctx context.Context, s api.SessionResponse) string {
	if s.JumpHosts != "" {
		return strconv.Itoa(hopCount(s.JumpHosts))
	}
	info, err := resolveProxy(ctx, sshx.Target{User: s.User, Host: s.Host, Port: s.Port})
	if err != nil {
		return "?"
	}
	if info.JumpHosts != "" {
		return strconv.Itoa(hopCount(info.JumpHosts))
	}
	if info.ProxyCommand {
		return "?"
	}
	return "0"
}

var sessionHelpCmd = &cobra.Command{
	Use:   "help [command]",
	Short: "Show help for session or one of its commands",
	RunE: func(cmd *cobra.Command, args []string) error {
		target, _, err := sessionCmd.Find(args)
		if err != nil || target == nil {
			printHelp(cmd.OutOrStdout(), "sshex session")
			return nil
		}
		printHelp(cmd.OutOrStdout(), target.CommandPath())
		return nil
	},
}

func init() {
	sessionCmd.AddCommand(sessionListCmd, sessionCloseCmd, sessionHelpCmd)
}
