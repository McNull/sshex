// Package remoteinstall installs sshex commands and session-specific shell setup.
package remoteinstall

import "strings"

const initMarker = "sshex-init-managed"

func initPath(paths RemotePaths) string {
	return paths.DataDir + "/init.bash"
}

func initContent() string {
	return `if [ -n "$SSHEX_SESSION" ]; then
    export PATH="${SSHEX_HOME:-${XDG_DATA_HOME:-$HOME/.local/share}/sshex}/bin:$PATH"
    if [ -n "$ZSH_VERSION" ]; then
        sshex_shell=zsh
    else
        sshex_shell=bash
    fi
    sshex_data="${SSHEX_HOME:-${XDG_DATA_HOME:-$HOME/.local/share}/sshex}"
    [ -f "$sshex_data/aliases.$sshex_shell" ] && . "$sshex_data/aliases.$sshex_shell"
    [ -f "$sshex_data/completions/sshex.$sshex_shell" ] && . "$sshex_data/completions/sshex.$sshex_shell"
    unset sshex_shell sshex_data
fi
`
}

func initWriteScript(paths RemotePaths) string {
	return `umask 077; mkdir -p ` + shellQuote(paths.DataDir) + `; cat > ` + shellQuote(initPath(paths)) + ` <<'SSHEX_INIT'
` + initContent() + `SSHEX_INIT
`
}

func initHookLine() string {
	return `. "${SSHEX_HOME:-${XDG_DATA_HOME:-$HOME/.local/share}/sshex}/init.bash" # ` + initMarker
}

func initWiringScript(rc string) string {
	return `rc=` + rc + `; touch "$rc"; ` +
		removeManagedBlockScript(`"$rc"`, completionsMarker, "# <<< sshex completions <<<") + `; ` +
		removeManagedBlockScript(`"$rc"`, aliasesMarker, "# <<< sshex aliases <<<") + `; ` +
		replaceManagedLineScript(`"$rc"`, initMarker, initHookLine())
}

func replaceManagedLineScript(rc, marker, replacement string) string {
	return `tmp="$rc.sshex-init"; awk -v marker=` + shellQuote(marker) + ` -v replacement=` + shellQuote(replacement) +
		` 'index($0, marker) { if (!replaced) { print replacement; replaced = 1 }; next } { print } END { if (!replaced) print replacement }' "$rc" > "$tmp" && mv "$tmp" "$rc"`
}

func removeManagedLineScript(rc, marker string) string {
	return `tmp="$rc.sshex-remove-init"; awk -v marker=` + shellQuote(marker) + ` 'index($0, marker) { next } { print }' "$rc" > "$tmp" && mv "$tmp" "$rc"`
}

func initUninstallScript() string {
	var b strings.Builder
	b.WriteString(`for rc in "$HOME/.bashrc" "${ZDOTDIR:-$HOME}/.zshrc"; do [ -f "$rc" ] || continue; `)
	b.WriteString(removeManagedLineScript(`"$rc"`, initMarker))
	b.WriteString(`; done; rm -f "` + dataExpr + `/init.bash"`)
	return b.String()
}
