package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mcnull/sshex/internal/api"
	"github.com/mcnull/sshex/internal/client"
	"github.com/mcnull/sshex/internal/config"
	"github.com/mcnull/sshex/internal/daemon"
	"github.com/mcnull/sshex/internal/sshhosts"
	"github.com/mcnull/sshex/internal/sshx"
	"github.com/spf13/cobra"
)

var connectCmd = &cobra.Command{
	Use:               "connect [<user>@]<host>",
	Short:             "Connect to a remote machine via ssh",
	Args:              cobra.ExactArgs(1),
	Annotations:       map[string]string{bannerAnnotation: "true"},
	ValidArgsFunction: completeConnectTarget,
	RunE: func(cmd *cobra.Command, args []string) error {
		user, host := splitTarget(args[0])
		port, _ := cmd.Flags().GetInt("port")
		identity, _ := cmd.Flags().GetString("identity")
		jump, _ := cmd.Flags().GetString("jump")
		options, _ := cmd.Flags().GetStringArray("option")
		noCompletions, _ := cmd.Flags().GetBool("no-completions")

		out := cmd.OutOrStdout()
		fmt.Fprintln(out, "installing remote sshex command...")

		broker, err := daemon.Ensure(cmd.Context(), os.Getenv("SSHEX_CONFIG"))
		if err != nil {
			return err
		}
		c := client.NewForEndpoint(broker.Endpoint, broker.Token)

		if port == 0 {
			port = 22
		}
		target := sshx.Target{
			User:         user,
			Host:         host,
			Port:         port,
			IdentityFile: identity,
			JumpHosts:    jump,
			Options:      options,
		}

		// Establish the SSH master here, in the foreground, so OpenSSH can
		// prompt on the terminal for host-key confirmation and authentication.
		// The broker is a detached process with no TTY and cannot prompt.
		controlPath, err := application.NewControlPath()
		if err != nil {
			return err
		}
		master, err := application.Transport.Connect(cmd.Context(), target, sshx.ConnectOptions{
			ControlPath: controlPath,
			Stderr:      os.Stderr,
		})
		if err != nil {
			return err
		}
		adopted := false
		defer func() {
			if !adopted {
				_ = master.Close(context.Background())
			}
		}()

		result, err := c.CreateSession(cmd.Context(), api.CreateSessionRequest{
			User:                  user,
			Host:                  host,
			Port:                  port,
			IdentityFile:          identity,
			JumpHosts:             jump,
			Options:               options,
			SkipCompletions:       noCompletions,
			ControllerControlPath: controlPath,
			ConnectionControlPath: controlPath,
		})
		if err != nil {
			return err
		}
		adopted = true
		fmt.Fprintln(out, "done.")
		fmt.Fprintln(out)

		session := result.Session
		connection := result.Connection
		conn := application.Transport.Attach(target, sshx.ConnectOptions{
			ControlPath: connection.ControlPath,
			SessionID:   session.ID,
		})

		heartbeatCtx, stopHeartbeat := context.WithCancel(context.Background())
		var heartbeatWG sync.WaitGroup
		heartbeatWG.Add(1)
		go func() {
			defer heartbeatWG.Done()
			heartbeatLoop(heartbeatCtx, c, session.ID, connection.ID, time.Duration(result.HeartbeatIntervalMS)*time.Millisecond)
		}()

		shellErr := conn.Shell(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())

		stopHeartbeat()
		heartbeatWG.Wait()

		dropped, detachErr := c.DetachConnection(context.Background(), session.ID, connection.ID)
		if detachErr != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "sshex: cleanup: %v\n", detachErr)
		}
		if len(dropped) > 0 {
			fmt.Fprintln(out, "dropped tunnels:")
			for _, tunnel := range dropped {
				fmt.Fprintf(out, "  %s\n", formatTunnel(tunnel.Direction, tunnel.LocalPort, tunnel.TargetHost, tunnel.TargetPort, ""))
			}
		}
		return shellErr
	},
}

func splitTarget(target string) (string, string) {
	if idx := strings.LastIndex(target, "@"); idx >= 0 {
		return target[:idx], target[idx+1:]
	}
	return "", target
}

// heartbeatLoop beats for a connection until the context is cancelled or the
// broker stops recognizing it. A failed beat ends the loop; the shell exit
// path still sends an explicit detach.
func heartbeatLoop(ctx context.Context, c *client.Client, sessionID, connectionID string, interval time.Duration) {
	if interval <= 0 {
		interval = config.DefaultHeartbeatInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			beatCtx, cancel := context.WithTimeout(ctx, interval)
			err := c.Heartbeat(beatCtx, sessionID, connectionID)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

var resolveConnectHosts = func() ([]sshhosts.Host, error) {
	return sshhosts.Resolve(sshhosts.Options{})
}

func completeConnectTarget(_ *cobra.Command, _ []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if strings.HasPrefix(toComplete, "-") {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	hosts, err := resolveConnectHosts()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	prefix, hostPart := "", toComplete
	if idx := strings.LastIndex(toComplete, "@"); idx >= 0 {
		prefix, hostPart = toComplete[:idx+1], toComplete[idx+1:]
	}

	completions := make([]cobra.Completion, 0, len(hosts))
	for _, host := range hosts {
		if hostPart != "" && !strings.HasPrefix(host.Name, hostPart) {
			continue
		}
		name := prefix + host.Name
		if host.Description != "" {
			completions = append(completions, cobra.CompletionWithDesc(name, host.Description))
			continue
		}
		completions = append(completions, name)
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}

func init() {
	connectCmd.Flags().IntP("port", "p", 22, "Specify the port to connect to")
	connectCmd.Flags().StringP("identity", "i", "", "Specify the identity file (private key) to use")
	connectCmd.Flags().StringP("jump", "J", "", "Jump host(s) to connect through, comma separated")
	connectCmd.Flags().StringArrayP("option", "o", nil, "Pass an ssh option (key=value); a value starting with '-' is passed as raw ssh args")
	connectCmd.Flags().Bool("no-completions", false, "Do not install shell completions on the remote")
}
