package remoteinstall

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"strings"
)

const (
	completionsMarker = "# >>> sshex completions >>>"
	dataExpr          = `${SSHEX_HOME:-${XDG_DATA_HOME:-$HOME/.local/share}/sshex}`
	shellDetectScript = `s=${SHELL:-}; [ -n "$s" ] || s=$(getent passwd "$(id -un)" 2>/dev/null | cut -d: -f7); basename "$s"`
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

type LinuxCompletionsInstaller struct{}

func NewLinuxCompletionsInstaller() *LinuxCompletionsInstaller {
	return &LinuxCompletionsInstaller{}
}

func (i *LinuxCompletionsInstaller) DetectShell(ctx context.Context, conn Runner) (Shell, bool, error) {
	var buf bytes.Buffer
	script := shellDetectScript
	if err := conn.Run(ctx, script, nil, &buf, nil); err != nil {
		return "", false, err
	}
	shell, ok := parseShell(buf.String())
	return shell, ok, nil
}

func (i *LinuxCompletionsInstaller) Install(ctx context.Context, conn Runner, paths RemotePaths, shell Shell) error {
	script, err := completionInstallScript(paths, shell)
	if err != nil {
		return err
	}
	if err := conn.Run(ctx, script, nil, nil, nil); err != nil {
		return fmt.Errorf("install remote completions: %w", err)
	}
	return nil
}

func (i *LinuxCompletionsInstaller) Uninstall(ctx context.Context, conn Runner, paths RemotePaths) error {
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

	switch shell {
	case ShellBash:
		return generate + `; ` + rcSourceScript(`"$HOME/.bashrc"`, `sshex.bash`), nil
	case ShellZsh:
		return generate + `; ` + rcSourceScript(`"${ZDOTDIR:-$HOME}/.zshrc"`, `sshex.zsh`), nil
	case ShellFish:
		return generate + `; ` + fishWiringScript(paths), nil
	default:
		return "", fmt.Errorf("unsupported shell: %q", shell)
	}
}

func rcSourceScript(rc, completion string) string {
	return `rc=` + rc + `; touch "$rc"; ` +
		`if ! grep -q 'sshex completions' "$rc" 2>/dev/null; then ` +
		`printf '%s\n' '` + completionsMarker + `' ` +
		`'export PATH="` + dataExpr + `/bin:$PATH"' ` +
		`'[ -f "` + dataExpr + `/completions/` + completion + `" ] && . "` + dataExpr + `/completions/` + completion + `"' ` +
		`'# <<< sshex completions <<<' >> "$rc"; fi`
}

func fishWiringScript(paths RemotePaths) string {
	configDir := `"${XDG_CONFIG_HOME:-$HOME/.config}/fish"`
	binDir := paths.BinDir()
	return `mkdir -p ` + configDir + `/completions ` + configDir + `/conf.d; ` +
		`cp ` + shellQuote(path.Join(paths.CompletionsDir(), "sshex.fish")) + ` ` + configDir + `/completions/sshex.fish; ` +
		`printf '%s\n' '` + completionsMarker + `' ` +
		`'if not contains "` + binDir + `" $PATH' ` +
		`'    set -gx PATH "` + binDir + `" $PATH' ` +
		`'end' ` +
		`'# <<< sshex completions <<<' > ` + configDir + `/conf.d/sshex.fish`
}

// completionUninstallScript removes the shell startup wiring added by
// completionInstallScript and the generated fish completion files. It is safe
// to run more than once and against files that were never wired up.
func completionUninstallScript() string {
	fishConfigDir := `"${XDG_CONFIG_HOME:-$HOME/.config}/fish"`
	removeBlock := `for rc in "$HOME/.bashrc" "${ZDOTDIR:-$HOME}/.zshrc"; do ` +
		`[ -f "$rc" ] || continue; ` +
		`grep -q '` + completionsMarker + `' "$rc" 2>/dev/null || continue; ` +
		`tmp="$rc.sshex.tmp"; ` +
		`sed '/` + completionsMarker + `/,/# <<< sshex completions <<</d' "$rc" > "$tmp" && mv "$tmp" "$rc"; ` +
		`done`
	removeFish := `rm -f ` + fishConfigDir + `/conf.d/sshex.fish ` + fishConfigDir + `/completions/sshex.fish`
	return removeBlock + `; ` + removeFish
}
