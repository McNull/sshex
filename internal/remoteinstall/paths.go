package remoteinstall

import (
	"path"
	"strings"
)

// pathsScript resolves the persistent state root and runtime root on the remote
// host. It mirrors config.RuntimeDir and sessionfile.RemoteDir on the client.
const pathsScript = `data="${SSHEX_HOME:-${XDG_DATA_HOME:-$HOME/.local/share}/sshex}"; ` +
	`if [ -n "$XDG_RUNTIME_DIR" ]; then runtime="$XDG_RUNTIME_DIR/sshex"; else runtime="/tmp/sshex-$(id -u)"; fi; ` +
	`printf '%s\n%s\n' "$data" "$runtime"`

// RemotePaths holds the resolved remote filesystem locations for a session.
type RemotePaths struct {
	DataDir string
	RunDir  string
}

func (p RemotePaths) BinDir() string         { return path.Join(p.DataDir, "bin") }
func (p RemotePaths) BinPath() string        { return path.Join(p.DataDir, "bin", "sshex") }
func (p RemotePaths) SessionsDir() string    { return path.Join(p.DataDir, "sessions") }
func (p RemotePaths) CompletionsDir() string { return path.Join(p.DataDir, "completions") }

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
