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

// aliasInstallScript writes the alias file for shell and wires it into the
// shell startup files. It is idempotent: rerunning rewrites the alias file and
// leaves the startup wiring untouched.
func aliasInstallScript(paths RemotePaths, shell Shell, aliases []Alias) (string, error) {
	if _, ok := parseShell(string(shell)); !ok {
		return "", fmt.Errorf("unsupported shell: %q", shell)
	}
	dest := paths.AliasPath(string(shell))
	content := aliasContent(shell, aliases)
	write := `umask 077; mkdir -p ` + shellQuote(paths.DataDir) + `; ` +
		`cat > ` + shellQuote(dest) + "<<'SSHEX_ALIASES'\n" + content + "SSHEX_ALIASES\n"
	return write + aliasWiringScript(paths, shell), nil
}

func aliasContent(shell Shell, aliases []Alias) string {
	var b strings.Builder
	for _, alias := range aliases {
		if alias.Name == "" || alias.Command == "" {
			continue
		}
		target := "sshex exec " + shellQuote(alias.Command)
		if shell == ShellFish {
			b.WriteString("alias " + alias.Name + " " + shellQuote(target) + "\n")
			continue
		}
		b.WriteString("alias " + alias.Name + "=" + shellQuote(target) + "\n")
	}
	return b.String()
}

// aliasWiringScript appends a marker block that sources the alias file to the
// appropriate shell startup file. The block is only added once.
func aliasWiringScript(paths RemotePaths, shell Shell) string {
	source := dataExpr + `/aliases.` + string(shell)
	block := `'` + aliasesMarker + `' ` +
		`'[ -f "` + source + `" ] && . "` + source + `"' ` +
		`'# <<< sshex aliases <<<'`

	switch shell {
	case ShellBash:
		return `rc="$HOME/.bashrc"; touch "$rc"; ` +
			`if ! grep -q '` + aliasesMarker + `' "$rc" 2>/dev/null; then ` +
			`printf '%s\n' ` + block + ` >> "$rc"; fi`
	case ShellZsh:
		return `rc="${ZDOTDIR:-$HOME}/.zshrc"; touch "$rc"; ` +
			`if ! grep -q '` + aliasesMarker + `' "$rc" 2>/dev/null; then ` +
			`printf '%s\n' ` + block + ` >> "$rc"; fi`
	case ShellFish:
		configDir := `"${XDG_CONFIG_HOME:-$HOME/.config}/fish"`
		fishBlock := `'` + aliasesMarker + `' ` +
			`'test -f ` + source + `; and source ` + source + `' ` +
			`'# <<< sshex aliases <<<'`
		return `mkdir -p ` + configDir + `/conf.d; ` +
			`rc=` + configDir + `/conf.d/sshex.fish; touch "$rc"; ` +
			`if ! grep -q '` + aliasesMarker + `' "$rc" 2>/dev/null; then ` +
			`printf '%s\n' ` + fishBlock + ` >> "$rc"; fi`
	default:
		return ""
	}
}

// aliasUninstallScript removes the startup wiring added by aliasWiringScript
// and the generated alias files. It is safe to run more than once.
func aliasUninstallScript() string {
	fishConfigDir := `"${XDG_CONFIG_HOME:-$HOME/.config}/fish"`
	removeBlock := `for rc in "$HOME/.bashrc" "${ZDOTDIR:-$HOME}/.zshrc" ` + fishConfigDir + `/conf.d/sshex.fish; do ` +
		`[ -f "$rc" ] || continue; ` +
		`grep -q '` + aliasesMarker + `' "$rc" 2>/dev/null || continue; ` +
		`tmp="$rc.sshex-alias.tmp"; ` +
		`sed '/` + aliasesMarker + `/,/# <<< sshex aliases <<</d' "$rc" > "$tmp" && mv "$tmp" "$rc"; ` +
		`done`
	removeFiles := `rm -f ` + dataExpr + `/aliases.bash ` + dataExpr + `/aliases.zsh ` + dataExpr + `/aliases.fish`
	return removeBlock + `; ` + removeFiles
}

// AliasPath returns the on-disk location of the alias file for shell.
func (p RemotePaths) AliasPath(shell string) string {
	return path.Join(p.DataDir, "aliases."+shell)
}
