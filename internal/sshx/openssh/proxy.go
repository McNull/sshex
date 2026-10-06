package openssh

import (
	"context"
	"os/exec"
	"strings"

	"github.com/mcnull/sshex/internal/sshx"
)

// ProxyInfo describes how OpenSSH would reach a target, as resolved from ssh
// configuration and command-line options.
type ProxyInfo struct {
	// JumpHosts is the effective ProxyJump chain (e.g. "ssh01,ssh02"), empty
	// when no ProxyJump applies.
	JumpHosts string
	// ProxyCommand reports that a non-"none" ProxyCommand applies while no
	// ProxyJump does, so the number of hops cannot be determined.
	ProxyCommand bool
}

// ResolveProxy probes OpenSSH with `ssh -G` to report the proxy path it would
// use for the target. It reads the same configuration as the connection.
func ResolveProxy(ctx context.Context, target sshx.Target) (ProxyInfo, error) {
	return resolveProxy(ctx, "ssh", target)
}

func resolveProxy(ctx context.Context, sshPath string, target sshx.Target) (ProxyInfo, error) {
	args := []string{"-G"}
	extra, err := targetArgs(target)
	if err != nil {
		return ProxyInfo{}, err
	}
	args = append(args, extra...)
	args = append(args, target.String())

	out, err := exec.CommandContext(ctx, sshPath, args...).Output()
	if err != nil {
		return ProxyInfo{}, err
	}
	return parseProxyOutput(string(out)), nil
}

// parseProxyOutput extracts the effective proxy directives from `ssh -G` output.
// A ProxyJump takes precedence; a ProxyCommand only signals an unknown hop count
// when no ProxyJump is set.
func parseProxyOutput(output string) ProxyInfo {
	var info ProxyInfo
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.ToLower(key) {
		case "proxyjump":
			if value != "" && !strings.EqualFold(value, "none") {
				info.JumpHosts = value
			}
		case "proxycommand":
			if value != "" && !strings.EqualFold(value, "none") {
				info.ProxyCommand = true
			}
		}
	}
	if info.JumpHosts != "" {
		info.ProxyCommand = false
	}
	return info
}
