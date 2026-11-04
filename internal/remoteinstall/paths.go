package remoteinstall

import (
	"path"
	"strings"
)

const (
	pathsBeginMarker = "SSHEX_REMOTE_PATHS_BEGIN_7F3A9C"
	pathsEndMarker   = "SSHEX_REMOTE_PATHS_END_7F3A9C"
)

// pathsScript resolves the persistent state root and runtime root on the remote
// host. It mirrors config.RuntimeDir and sessionfile.RemoteDir on the client.
const pathsScript = `data="${SSHEX_HOME:-${XDG_DATA_HOME:-$HOME/.local/share}/sshex}"; ` +
	`if [ -n "$XDG_RUNTIME_DIR" ]; then runtime="$XDG_RUNTIME_DIR/sshex"; else runtime="/tmp/sshex-$(id -u)"; fi; ` +
	`printf '\n%s\n%s\n%s\n%s\n' '` + pathsBeginMarker + `' "$data" "$runtime" '` + pathsEndMarker + `'`

// RemotePaths holds the resolved remote filesystem locations for a session.
type RemotePaths struct {
	DataDir string
	RunDir  string
}

func (p RemotePaths) BinDir() string         { return path.Join(p.DataDir, "bin") }
func (p RemotePaths) BinPath() string        { return path.Join(p.DataDir, "bin", "sshex") }
func (p RemotePaths) SessionsDir() string    { return path.Join(p.DataDir, "sessions") }
func (p RemotePaths) CompletionsDir() string { return path.Join(p.DataDir, "completions") }
func (p RemotePaths) DistDir() string        { return path.Join(p.DataDir, "dist") }

// InstallDirRecordPath stores the directory install.sh placed the binary in.
// Remote installs keep only the ~/.local/bin symlink, so the record is optional.
func (p RemotePaths) InstallDirRecordPath() string { return path.Join(p.DataDir, "install-dir") }

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
