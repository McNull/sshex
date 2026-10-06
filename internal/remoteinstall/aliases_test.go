package remoteinstall

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAliasContent(t *testing.T) {
	aliases := []Alias{{Name: "code", Command: "code"}, {Name: "gs", Command: "git-status"}}
	bash := aliasContent(ShellBash, aliases)
	for _, want := range []string{
		`alias code='sshex exec '\''code'\'''`,
		`alias gs='sshex exec '\''git-status'\'''`,
	} {
		if !strings.Contains(bash, want) {
			t.Fatalf("bash alias content %q missing %q", bash, want)
		}
	}
	fish := aliasContent(ShellFish, aliases)
	if !strings.Contains(fish, `alias code 'sshex exec '\''code'\'''`) {
		t.Fatalf("fish alias content = %q", fish)
	}
}

func TestAliasContentSkipsIncomplete(t *testing.T) {
	if got := aliasContent(ShellBash, []Alias{{Name: "", Command: "x"}, {Name: "x", Command: ""}}); got != "" {
		t.Fatalf("aliasContent() = %q, want empty", got)
	}
}

func TestInstallAliases(t *testing.T) {
	paths := RemotePaths{DataDir: "/home/tester/.local/share/sshex", RunDir: "/run/user/1000/sshex"}
	for _, shell := range []Shell{ShellBash, ShellZsh, ShellFish} {
		conn := &fakeConn{}
		if err := NewLinuxAliasInstaller().Install(context.Background(), conn, paths, shell, []Alias{{Name: "code", Command: "code"}}); err != nil {
			t.Fatalf("Install(%s) error: %v", shell, err)
		}
		if len(conn.commands) != 1 {
			t.Fatalf("Install(%s) ran %d commands, want 1", shell, len(conn.commands))
		}
		cmd := conn.commands[0]
		for _, want := range []string{
			paths.AliasPath(string(shell)),
			aliasesMarker,
			"# <<< sshex aliases <<<",
		} {
			if !strings.Contains(cmd, want) {
				t.Fatalf("Install(%s) command missing %q:\n%s", shell, want, cmd)
			}
		}
	}
}

func TestInstallAliasesUnsupported(t *testing.T) {
	paths := RemotePaths{DataDir: "/home/tester/.local/share/sshex", RunDir: "/run/user/1000/sshex"}
	if _, err := aliasInstallScript(paths, Shell("powershell"), nil); err == nil {
		t.Fatal("aliasInstallScript(powershell) error = nil, want error")
	}
}

func TestAliasUninstallScript(t *testing.T) {
	script := aliasUninstallScript()
	for _, want := range []string{
		"$HOME/.bashrc",
		"${ZDOTDIR:-$HOME}/.zshrc",
		aliasesMarker,
		"# <<< sshex aliases <<<",
		"${XDG_CONFIG_HOME:-$HOME/.config}/fish",
		"/conf.d/sshex.fish",
		"/aliases.bash",
		"/aliases.fish",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("aliasUninstallScript() missing %q:\n%s", want, script)
		}
	}
}

func TestUninstallAliasesRunsScript(t *testing.T) {
	conn := &fakeConn{}
	paths := RemotePaths{DataDir: "/home/tester/.local/share/sshex", RunDir: "/run/user/1000/sshex"}
	if err := NewLinuxAliasInstaller().Uninstall(context.Background(), conn, paths); err != nil {
		t.Fatalf("Uninstall() error: %v", err)
	}
	if len(conn.commands) != 1 || !strings.Contains(conn.commands[0], aliasesMarker) {
		t.Fatalf("Uninstall() commands = %v", conn.commands)
	}
}

// TestAliasScriptBash sources the generated wiring in a real bash to prove the
// alias file is written and picked up by the shell startup file.
func TestAliasScriptBash(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	home := t.TempDir()
	dataDir := filepath.Join(home, ".local", "share", "sshex")
	paths := RemotePaths{DataDir: dataDir, RunDir: filepath.Join(home, "run")}

	script, err := aliasInstallScript(paths, ShellBash, []Alias{{Name: "code", Command: "code"}})
	if err != nil {
		t.Fatalf("aliasInstallScript() error: %v", err)
	}
	cmd := exec.Command(bash, "-c", script)
	cmd.Env = append(os.Environ(), "HOME="+home, "SSHEX_HOME=", "XDG_DATA_HOME="+filepath.Join(home, ".local", "share"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run alias install: %v: %s", err, out)
	}

	aliasFile := paths.AliasPath(string(ShellBash))
	content, err := os.ReadFile(aliasFile)
	if err != nil {
		t.Fatalf("read alias file: %v", err)
	}
	if !strings.Contains(string(content), "alias code=") {
		t.Fatalf("alias file = %q", content)
	}

	// A login shell sourcing the rc must expose the alias.
	source := exec.Command(bash, "-ic", `. "$HOME/.bashrc"; alias code`)
	source.Env = cmd.Env
	out, err := source.CombinedOutput()
	if err != nil {
		t.Fatalf("source rc: %v: %s", err, out)
	}
	if !strings.Contains(string(out), "sshex exec") {
		t.Fatalf("alias not defined after sourcing rc: %s", out)
	}
}
