package remoteinstall

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"strings"
)

const (
	completionsMarker     = "# >>> sshex completions >>>"
	completionsLineMarker = "sshex-completions-managed"
	dataExpr              = `${SSHEX_HOME:-${XDG_DATA_HOME:-$HOME/.local/share}/sshex}`
	// getent exists on Linux; macOS uses dscl to read the login shell when
	// SHELL is unset (for example over a non-login SSH session).
	shellDetectScript = `s=${SHELL:-}; ` +
		`[ -n "$s" ] || s=$(getent passwd "$(id -un)" 2>/dev/null | cut -d: -f7); ` +
		`[ -n "$s" ] || s=$(dscl . -read "$HOME" UserShell 2>/dev/null | awk '{print $2}'); ` +
		`basename "$s"`
)

type Shell string

const (
	ShellBash Shell = "bash"
	ShellZsh  Shell = "zsh"
	ShellFish Shell = "fish"
)

type CompletionsInstaller interface {
	DetectShell(ctx context.Context, conn Runner) (Shell, bool, error)
	Install(ctx context.Context, conn Runner, paths RemotePaths, shell Shell) error
	Uninstall(ctx context.Context, conn Runner, paths RemotePaths) error
}

type PosixCompletionsInstaller struct{}

func NewPosixCompletionsInstaller() *PosixCompletionsInstaller {
	return &PosixCompletionsInstaller{}
}

func (i *PosixCompletionsInstaller) DetectShell(ctx context.Context, conn Runner) (Shell, bool, error) {
	var buf bytes.Buffer
	script := shellDetectScript
	if err := conn.Run(ctx, script, nil, &buf, nil); err != nil {
		return "", false, err
	}
	shell, ok := parseShell(buf.String())
	return shell, ok, nil
}

func (i *PosixCompletionsInstaller) Install(ctx context.Context, conn Runner, paths RemotePaths, shell Shell) error {
	script, err := completionInstallScript(paths, shell)
	if err != nil {
		return err
	}
	if err := conn.Run(ctx, script, nil, nil, nil); err != nil {
		return fmt.Errorf("install remote completions: %w", err)
	}
	return nil
}

func (i *PosixCompletionsInstaller) Uninstall(ctx context.Context, conn Runner, paths RemotePaths) error {
	if err := conn.Run(ctx, completionUninstallScript(), nil, nil, nil); err != nil {
		return fmt.Errorf("uninstall remote completions: %w", err)
	}
	return nil
}

func parseShell(name string) (Shell, bool) {
	switch strings.TrimSpace(name) {
	case string(ShellBash):
		return ShellBash, true
	case string(ShellZsh):
		return ShellZsh, true
	case string(ShellFish):
		return ShellFish, true
	default:
		return "", false
	}
}

func completionInstallScript(paths RemotePaths, shell Shell) (string, error) {
	if _, ok := parseShell(string(shell)); !ok {
		return "", fmt.Errorf("unsupported shell: %q", shell)
	}

	dest := path.Join(paths.CompletionsDir(), "sshex."+string(shell))
	generate := `umask 077; mkdir -p ` + shellQuote(paths.CompletionsDir()) + `; ` +
		`"` + paths.BinPath() + `" completions ` + string(shell) + ` > ` + shellQuote(dest)
	if shell == ShellFish {
		return generate + `; ` + fishWiringScript(paths), nil
	}
	rc := `"$HOME/.bashrc"`
	if shell == ShellZsh {
		rc = `"${ZDOTDIR:-$HOME}/.zshrc"`
	}
	return generate + `; ` + initWriteScript(paths) + initWiringScript(rc), nil
}

func fishWiringScript(paths RemotePaths) string {
	configDir := `"${XDG_CONFIG_HOME:-$HOME/.config}/fish"`
	block := completionsMarker + `
if test -n "$SSHEX_SESSION"
    if not contains "` + paths.BinDir() + `" $PATH
        set -gx PATH "` + paths.BinDir() + `" $PATH
    end
    set completion_file "` + path.Join(paths.CompletionsDir(), "sshex.fish") + `"
    test -f "$completion_file"; and source "$completion_file"
end
# <<< sshex completions <<<`
	return `mkdir -p ` + configDir + `/conf.d; ` +
		`rc=` + configDir + `/conf.d/sshex.fish; touch "$rc"; ` +
		replaceManagedBlockScript(`"$rc"`, completionsMarker, "# <<< sshex completions <<<", block)
}

// replaceManagedBlockScript swaps a marker-delimited block in a startup file.
// The replacement is passed through the environment because BSD awk (macOS)
// rejects literal newlines in -v assignments.
func replaceManagedBlockScript(rc, start, end, block string) string {
	return `tmp="$rc.sshex-install-` + strings.ReplaceAll(strings.Trim(start, "# >"), " ", "-") + `"; ` +
		`SSHEX_BLOCK=` + shellQuote(block) + ` awk -v start=` + shellQuote(start) + ` -v end=` + shellQuote(end) + ` '` +
		`$0 == start && !inside { inside = 1; buffer = $0 ORS; next } ` +
		`inside { buffer = buffer $0 ORS; if ($0 == end) { inside = 0; buffer = ""; if (!replaced) { printf "%s\n", ENVIRON["SSHEX_BLOCK"]; replaced = 1 } }; next } ` +
		`{ print } END { if (inside) printf "%s", buffer; if (!replaced) printf "%s\n", ENVIRON["SSHEX_BLOCK"] }' "$rc" > "$tmp" && mv "$tmp" "$rc"`
}

func removeManagedBlockScript(rc, start, end string) string {
	tmpSuffix := strings.ReplaceAll(strings.Trim(start, "# >"), " ", "-")
	return `tmp="$rc.sshex-remove-` + tmpSuffix + `"; ` +
		`awk -v start=` + shellQuote(start) + ` -v end=` + shellQuote(end) + ` '` +
		`$0 == start { if (inside) printf "%s", buffer; inside = 1; buffer = $0 ORS; next } ` +
		`inside { buffer = buffer $0 ORS; if ($0 == end) { inside = 0; buffer = "" }; next } ` +
		`{ print } END { if (inside) printf "%s", buffer }' "$rc" > "$tmp" && mv "$tmp" "$rc"`
}

// completionUninstallScript removes managed startup hooks and generated fish
// completion files. It is safe to run repeatedly and with missing files.
func completionUninstallScript() string {
	fishConfigDir := `"${XDG_CONFIG_HOME:-$HOME/.config}/fish"`
	fishRC := fishConfigDir + `/conf.d/sshex.fish`
	var b strings.Builder
	b.WriteString(`for rc in "$HOME/.bashrc" "${ZDOTDIR:-$HOME}/.zshrc"; do [ -f "$rc" ] || continue; `)
	b.WriteString(removeManagedLineScript(`"$rc"`, completionsLineMarker) + `; `)
	b.WriteString(removeManagedBlockScript(`"$rc"`, completionsMarker, "# <<< sshex completions <<<") + `; done; `)
	b.WriteString(`fish_rc=` + fishRC + `; if [ -f "$fish_rc" ]; then rc="$fish_rc"; `)
	b.WriteString(removeManagedBlockScript(`"$rc"`, completionsMarker, "# <<< sshex completions <<<") + `; `)
	b.WriteString(`[ -s "$fish_rc" ] || rm -f "$fish_rc"; fi; `)
	b.WriteString(`rm -f "` + dataExpr + `/completions/sshex.bash" "` + dataExpr + `/completions/sshex.zsh" "` + dataExpr + `/completions/sshex.fish" ` +
		`"` + dataExpr + `/sshexrc.bash" "` + dataExpr + `/sshexrc.zsh" ` + fishConfigDir + `/completions/sshex.fish`)
	return b.String() + `; ` + initUninstallScript()
}
