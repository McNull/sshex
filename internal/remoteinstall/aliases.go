package remoteinstall

import (
	"context"
	"fmt"
	"path"
	"strings"
)

const aliasesMarker = "# >>> sshex aliases >>>"

// Alias maps a shell alias to a predefined sshex command.
type Alias struct {
	// Name is the alias name defined in the remote shell.
	Name string
	// Command is the predefined command the alias runs via `sshex exec`.
	Command string
}

type AliasInstaller interface {
	Install(ctx context.Context, conn Runner, paths RemotePaths, shell Shell, aliases []Alias) error
	Uninstall(ctx context.Context, conn Runner, paths RemotePaths) error
}

type LinuxAliasInstaller struct{}

func NewLinuxAliasInstaller() *LinuxAliasInstaller {
	return &LinuxAliasInstaller{}
}

func (i *LinuxAliasInstaller) Install(ctx context.Context, conn Runner, paths RemotePaths, shell Shell, aliases []Alias) error {
	script, err := aliasInstallScript(paths, shell, aliases)
	if err != nil {
		return err
	}
	if err := conn.Run(ctx, script, nil, nil, nil); err != nil {
		return fmt.Errorf("install remote aliases: %w", err)
	}
	return nil
}

func (i *LinuxAliasInstaller) Uninstall(ctx context.Context, conn Runner, paths RemotePaths) error {
	if err := conn.Run(ctx, aliasUninstallScript(), nil, nil, nil); err != nil {
		return fmt.Errorf("uninstall remote aliases: %w", err)
	}
	return nil
}

// aliasInstallScript writes the generated alias file and shared session init.
// Repeated installs replace only complete managed blocks and the managed hook line.
func aliasInstallScript(paths RemotePaths, shell Shell, aliases []Alias) (string, error) {
	if _, ok := parseShell(string(shell)); !ok {
		return "", fmt.Errorf("unsupported shell: %q", shell)
	}
	dest := paths.AliasPath(string(shell))
	content := aliasContent(shell, aliases)
	write := `umask 077; mkdir -p ` + shellQuote(paths.DataDir) + `; ` +
		`cat > ` + shellQuote(dest) + "<<'SSHEX_ALIASES'\n" + content + "SSHEX_ALIASES\n"
	if shell == ShellFish {
		return write + aliasWiringScript(paths, shell), nil
	}
	rc := `"$HOME/.bashrc"`
	if shell == ShellZsh {
		rc = `"${ZDOTDIR:-$HOME}/.zshrc"`
	}
	return write + initWriteScript(paths) + initWiringScript(rc), nil
}

func aliasContent(shell Shell, aliases []Alias) string {
	var b strings.Builder
	for _, alias := range aliases {
		if alias.Name == "" || alias.Command == "" {
			continue
		}
		target := "sshex exec " + shellQuote(alias.Command)
		if shell == ShellFish {
			b.WriteString("functions -q " + alias.Name + "; or alias " + alias.Name + " " + shellQuote(target) + "\n")
			continue
		}
		b.WriteString("if ! alias " + alias.Name + " >/dev/null 2>&1 && ! typeset -f " + alias.Name + " >/dev/null 2>&1; then alias " + alias.Name + "=" + shellQuote(target) + "; fi\n")
	}
	return b.String()
}

// aliasWiringScript installs fish's native startup hook.
func aliasWiringScript(paths RemotePaths, shell Shell) string {
	if shell != ShellFish {
		return ""
	}
	configDir := `"${XDG_CONFIG_HOME:-$HOME/.config}/fish"`
	fishSource := paths.AliasPath(string(shell))
	block := aliasesMarker + `
if test -n "$SSHEX_SESSION"
    set alias_file "` + fishSource + `"
    test -f "$alias_file"; and source "$alias_file"
end
# <<< sshex aliases <<<`
	return `mkdir -p ` + configDir + `/conf.d; rc=` + configDir + `/conf.d/sshex.fish; touch "$rc"; ` +
		replaceManagedBlockScript(`"$rc"`, aliasesMarker, "# <<< sshex aliases <<<", block)
}

// aliasUninstallScript removes managed startup hooks and generated aliases.
// It is safe to run repeatedly and with missing files.
func aliasUninstallScript() string {
	fishConfigDir := `"${XDG_CONFIG_HOME:-$HOME/.config}/fish"`
	removeBlocks := `fish_rc=` + fishConfigDir + `/conf.d/sshex.fish; for rc in "$HOME/.bashrc" "${ZDOTDIR:-$HOME}/.zshrc" "$fish_rc"; do ` +
		`[ -f "$rc" ] || continue; ` +
		removeManagedBlockScript(`"$rc"`, aliasesMarker, "# <<< sshex aliases <<<") + `; ` +
		`[ -s "$rc" ] || case "$rc" in */conf.d/sshex.fish) rm -f "$rc";; esac; ` +
		`done; rm -f "` + dataExpr + `/aliases.bash" "` + dataExpr + `/aliases.zsh" "` + dataExpr + `/aliases.fish"`
	return removeBlocks + `; ` + initUninstallScript()
}

// AliasPath returns the on-disk location of the alias file for shell.
func (p RemotePaths) AliasPath(shell string) string {
	return path.Join(p.DataDir, "aliases."+shell)
}
