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
	for _, want := range []string{"! alias code", "! typeset -f code", "alias code=", "alias gs="} {
		if !strings.Contains(bash, want) {
			t.Fatalf("bash alias content %q missing %q", bash, want)
		}
	}
	fish := aliasContent(ShellFish, aliases)
	if !strings.Contains(fish, `functions -q code; or alias code 'sshex exec '\''code'\'''`) {
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
		if err := NewPosixAliasInstaller().Install(context.Background(), conn, paths, shell, []Alias{{Name: "code", Command: "code"}}); err != nil {
			t.Fatalf("Install(%s) error: %v", shell, err)
		}
		if len(conn.commands) != 1 {
			t.Fatalf("Install(%s) ran %d commands, want 1", shell, len(conn.commands))
		}
		cmd := conn.commands[0]
		if shell == ShellFish {
			for _, want := range []string{paths.AliasPath(string(shell)), aliasesMarker, "# <<< sshex aliases <<<"} {
				if !strings.Contains(cmd, want) {
					t.Fatalf("Install(%s) command missing %q:\n%s", shell, want, cmd)
				}
			}
		} else if !strings.Contains(cmd, initPath(paths)) || !strings.Contains(cmd, initHookLine()) {
			t.Fatalf("Install(%s) command missing shared init wiring:\n%s", shell, cmd)
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
	if err := NewPosixAliasInstaller().Uninstall(context.Background(), conn, paths); err != nil {
		t.Fatalf("Uninstall() error: %v", err)
	}
	if len(conn.commands) != 1 || !strings.Contains(conn.commands[0], aliasesMarker) {
		t.Fatalf("Uninstall() commands = %v", conn.commands)
	}
}

// TestSessionGatedBashStartup exercises installed completion and alias hooks.
func TestSessionGatedBashStartup(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	home := t.TempDir()
	dataDir := filepath.Join(home, ".local", "share", "sshex")
	paths := RemotePaths{DataDir: dataDir, RunDir: filepath.Join(home, "run")}
	if err := os.MkdirAll(paths.BinDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.BinPath(), []byte("#!/bin/sh\nif [ \"$1\" = completions ]; then printf '%s\\n' '_sshex_complete() { :; }'; fi\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.CompletionsDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	completion := filepath.Join(paths.CompletionsDir(), "sshex.bash")
	if err := os.WriteFile(completion, []byte("_sshex_complete() { :; }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	rcPath := filepath.Join(home, ".bashrc")
	if err := os.WriteFile(rcPath, []byte("export PATH=\"$HOME/user-bin:/usr/bin\"\nalias code='user-code'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "HOME="+home, "SSHEX_HOME=", "XDG_DATA_HOME="+filepath.Join(home, ".local", "share"))
	run := func(script string) {
		t.Helper()
		cmd := exec.Command(bash, "-c", script)
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("run install script: %v: %s", err, out)
		}
	}
	aliasScript, err := aliasInstallScript(paths, ShellBash, []Alias{{Name: "code", Command: "code"}, {Name: "gs", Command: "git-status"}})
	if err != nil {
		t.Fatal(err)
	}
	completionScript, err := completionInstallScript(paths, ShellBash)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		run(aliasScript)
		run(completionScript)
	}
	rc, err := os.ReadFile(rcPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(rc), initMarker) != 1 || strings.Contains(string(rc), completionsMarker) || strings.Contains(string(rc), aliasesMarker) {
		t.Fatalf("startup file should contain one shared init hook:\n%s", rc)
	}
	if !strings.Contains(string(rc), "user-bin") || strings.Count(string(rc), "alias code='user-code'") != 1 {
		t.Fatalf("install changed unrelated rc content:\n%s", rc)
	}
	probe := `printf 'PATH=%s\n' "$PATH"; printf 'COMP=%s\n' "$(type -t _sshex_complete 2>/dev/null || true)"; printf 'CODE=%s\n' "$(alias code 2>/dev/null || true)"; printf 'GS=%s\n' "$(alias gs 2>/dev/null || true)"`
	checkShell := func(session string, active bool) string {
		t.Helper()
		cmd := exec.Command(bash, "--noprofile", "--rcfile", rcPath, "-ic", probe)
		cmd.Env = append(append([]string{}, env...), "SSHEX_SESSION="+session)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("bash startup (session %q): %v: %s", session, err, out)
		}
		got := string(out)
		if strings.Contains(got, paths.BinDir()) != active {
			t.Fatalf("PATH activation mismatch (session %q): %s", session, got)
		}
		if strings.Contains(got, "COMP=function") != active {
			t.Fatalf("completion activation mismatch (session %q): %s", session, got)
		}
		if !strings.Contains(got, "CODE=alias code='user-code'") || strings.Contains(got, "GS=alias gs=") != active {
			t.Fatalf("alias precedence/activation mismatch (session %q): %s", session, got)
		}
		if !strings.Contains(got, "user-bin") {
			t.Fatalf("user PATH entry lost (session %q): %s", session, got)
		}
		return got
	}
	checkShell("", false)
	checkShell("test-session", true)
	for i := 0; i < 2; i++ {
		cmd := exec.Command(bash, "-c", aliasUninstallScript()+"; "+completionUninstallScript())
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("uninstall: %v: %s", err, out)
		}
	}
	rc, err = os.ReadFile(rcPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(rc) != "export PATH=\"$HOME/user-bin:/usr/bin\"\nalias code='user-code'\n" {
		t.Fatalf("uninstall changed unrelated rc content: %q", rc)
	}
	for _, file := range []string{initPath(paths), paths.AliasPath("bash"), completion} {
		if _, err := os.Stat(file); !os.IsNotExist(err) {
			t.Fatalf("generated file remains after uninstall: %s (%v)", file, err)
		}
	}
}

func TestManagedBlockMigrationAndPreservation(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	home := t.TempDir()
	paths := RemotePaths{DataDir: filepath.Join(home, ".local", "share", "sshex"), RunDir: filepath.Join(home, "run")}
	if err := os.MkdirAll(paths.BinDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.BinPath(), []byte("#!/bin/sh\nif [ \"$1\" = completions ]; then printf '%s\\n' '_sshex_complete() { :; }'; fi\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.CompletionsDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.CompletionsDir(), "sshex.bash"), []byte("generated = `_sshex_complete() { :; }`\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	legacy := "before\n" + completionsMarker + "\nold completion hook\n# <<< sshex completions <<<\n" + aliasesMarker + "\nold alias hook\n# <<< sshex aliases <<<\nafter\n"
	rc := filepath.Join(home, ".bashrc")
	if err := os.WriteFile(rc, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	completionScript, err := completionInstallScript(paths, ShellBash)
	if err != nil {
		t.Fatal(err)
	}
	aliasScript, err := aliasInstallScript(paths, ShellBash, []Alias{{Name: "gs", Command: "git-status"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, script := range []string{completionScript, aliasScript} {
		cmd := exec.Command(bash, "-c", script)
		cmd.Env = append(os.Environ(), "HOME="+home)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("install over legacy block: %v: %s", err, out)
		}
	}
	content, err := os.ReadFile(rc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(content), "before\n") || !strings.Contains(string(content), "\nafter\n") || strings.Contains(string(content), "old completion hook") || strings.Contains(string(content), "old alias hook") {
		t.Fatalf("legacy blocks not migrated preserving outside text:\n%s", content)
	}
	if strings.Count(string(content), initMarker) != 1 || strings.Contains(string(content), completionsMarker) || strings.Contains(string(content), aliasesMarker) {
		t.Fatalf("legacy blocks not consolidated to one init hook:\n%s", content)
	}
	partial := "before\n" + completionsMarker + "\nuser unfinished content\n"
	if err := os.WriteFile(rc, []byte(partial), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bash, "-c", completionScript)
	cmd.Env = append(os.Environ(), "HOME="+home)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("install with incomplete block: %v: %s", err, out)
	}
	content, err = os.ReadFile(rc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), partial) || strings.Count(string(content), initMarker) != 1 {
		t.Fatalf("incomplete block changed or init hook missing:\n%s", content)
	}
}

func TestFishInstallOrderAndUninstall(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	for _, aliasFirst := range []bool{true, false} {
		home := t.TempDir()
		paths := RemotePaths{DataDir: filepath.Join(home, ".local", "share", "sshex"), RunDir: filepath.Join(home, "run")}
		config := filepath.Join(home, ".config")
		fishDir := filepath.Join(config, "fish")
		if err := os.MkdirAll(filepath.Join(fishDir, "conf.d"), 0o700); err != nil {
			t.Fatal(err)
		}
		fishRC := filepath.Join(fishDir, "conf.d", "sshex.fish")
		unrelated := "# user fish startup\n"
		if err := os.WriteFile(fishRC, []byte(unrelated), 0o600); err != nil {
			t.Fatal(err)
		}
		aliasScript, err := aliasInstallScript(paths, ShellFish, []Alias{{Name: "gs", Command: "git-status"}})
		if err != nil {
			t.Fatal(err)
		}
		completionScript, err := completionInstallScript(paths, ShellFish)
		if err != nil {
			t.Fatal(err)
		}
		scripts := []string{completionScript, aliasScript}
		if aliasFirst {
			scripts = []string{aliasScript, completionScript}
		}
		for _, script := range scripts {
			cmd := exec.Command(bash, "-c", script)
			cmd.Env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+config, "SSHEX_HOME="+paths.DataDir)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("fish installer: %v: %s", err, out)
			}
		}
		content, err := os.ReadFile(fishRC)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range []string{aliasesMarker, completionsMarker} {
			if strings.Count(string(content), marker) != 1 {
				t.Fatalf("fish hook missing one %s block:\n%s", marker, content)
			}
		}
		if !strings.HasPrefix(string(content), unrelated) || !strings.Contains(string(content), "SSHEX_SESSION") || strings.Contains(string(content), "cp ") {
			t.Fatalf("fish hook changed user content or copies global completion:\n%s", content)
		}
		cmd := exec.Command(bash, "-c", aliasUninstallScript()+"; "+completionUninstallScript()+"; "+aliasUninstallScript()+"; "+completionUninstallScript())
		cmd.Env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+config, "SSHEX_HOME="+paths.DataDir)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("fish uninstall: %v: %s", err, out)
		}
		content, err = os.ReadFile(fishRC)
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != unrelated {
			t.Fatalf("fish uninstall changed unrelated content: %q", content)
		}
		for _, file := range []string{filepath.Join(paths.CompletionsDir(), "sshex.fish"), filepath.Join(fishDir, "completions", "sshex.fish")} {
			if _, err := os.Stat(file); !os.IsNotExist(err) {
				t.Fatalf("fish completion file remains: %s (%v)", file, err)
			}
		}
	}
}
