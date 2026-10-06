package remoteinstall

import (
	"context"
	"strings"
	"testing"
)

func TestParseShell(t *testing.T) {
	cases := []struct {
		in     string
		shell  Shell
		wantOK bool
	}{
		{"bash\n", ShellBash, true},
		{"zsh\n", ShellZsh, true},
		{"fish\n", ShellFish, true},
		{"  bash  ", ShellBash, true},
		{"dash\n", "", false},
		{"\n", "", false},
	}
	for _, tc := range cases {
		shell, ok := parseShell(tc.in)
		if shell != tc.shell || ok != tc.wantOK {
			t.Fatalf("parseShell(%q) = %q, %v; want %q, %v", tc.in, shell, ok, tc.shell, tc.wantOK)
		}
	}
}

func TestDetectShell(t *testing.T) {
	for _, tc := range []struct {
		out   string
		shell Shell
		ok    bool
	}{
		{"bash\n", ShellBash, true},
		{"zsh\n", ShellZsh, true},
		{"fish\n", ShellFish, true},
		{"tcsh\n", "", false},
	} {
		conn := &fakeConn{stdout: map[string]string{shellDetectScript: tc.out}}
		shell, ok, err := NewLinuxCompletionsInstaller().DetectShell(context.Background(), conn)
		if err != nil {
			t.Fatalf("DetectShell() error: %v", err)
		}
		if shell != tc.shell || ok != tc.ok {
			t.Fatalf("DetectShell() = %q, %v; want %q, %v", shell, ok, tc.shell, tc.ok)
		}
	}
}

func TestInstallCompletions(t *testing.T) {
	paths := RemotePaths{DataDir: "/home/tester/.local/share/sshex", RunDir: "/run/user/1000/sshex"}
	for _, shell := range []Shell{ShellBash, ShellZsh, ShellFish} {
		conn := &fakeConn{}
		if err := NewLinuxCompletionsInstaller().Install(context.Background(), conn, paths, shell); err != nil {
			t.Fatalf("Install(%s) error: %v", shell, err)
		}
		if len(conn.commands) != 1 {
			t.Fatalf("Install(%s) ran %d commands, want 1", shell, len(conn.commands))
		}
		cmd := conn.commands[0]
		for _, want := range []string{
			`"` + paths.BinPath() + `" completions ` + string(shell),
			paths.CompletionsDir() + `/sshex.` + string(shell),
			completionsMarker,
			paths.BinDir(),
		} {
			if !strings.Contains(cmd, want) {
				t.Fatalf("Install(%s) command missing %q:\n%s", shell, want, cmd)
			}
		}
	}
}

func TestInstallCompletionsWiring(t *testing.T) {
	paths := RemotePaths{DataDir: "/home/tester/.local/share/sshex", RunDir: "/run/user/1000/sshex"}
	cases := map[Shell]string{
		ShellBash: ".bashrc",
		ShellZsh:  ".zshrc",
		ShellFish: `}/fish"/completions`,
	}
	for shell, wantPath := range cases {
		script, err := completionInstallScript(paths, shell)
		if err != nil {
			t.Fatalf("completionInstallScript(%s) error: %v", shell, err)
		}
		if !strings.Contains(script, wantPath) {
			t.Fatalf("completionInstallScript(%s) missing %q:\n%s", shell, wantPath, script)
		}
		if strings.Count(script, completionsMarker) != 1 {
			t.Fatalf("completionInstallScript(%s) marker count = %d, want 1", shell, strings.Count(script, completionsMarker))
		}
	}
}

func TestCompletionInstallScriptUnsupported(t *testing.T) {
	paths := RemotePaths{DataDir: "/home/tester/.local/share/sshex", RunDir: "/run/user/1000/sshex"}
	if _, err := completionInstallScript(paths, Shell("powershell")); err == nil {
		t.Fatal("completionInstallScript(powershell) error = nil, want error")
	}
}

func TestCompletionUninstallScript(t *testing.T) {
	script := completionUninstallScript()
	for _, want := range []string{
		"$HOME/.bashrc",
		"${ZDOTDIR:-$HOME}/.zshrc",
		completionsMarker,
		"# <<< sshex completions <<<",
		`${XDG_CONFIG_HOME:-$HOME/.config}/fish`,
		"conf.d/sshex.fish",
		"completions/sshex.fish",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("completionUninstallScript() missing %q:\n%s", want, script)
		}
	}
}

func TestUninstallCompletionsRunsScript(t *testing.T) {
	conn := &fakeConn{}
	paths := RemotePaths{DataDir: "/home/tester/.local/share/sshex", RunDir: "/run/user/1000/sshex"}
	if err := NewLinuxCompletionsInstaller().Uninstall(context.Background(), conn, paths); err != nil {
		t.Fatalf("Uninstall() error: %v", err)
	}
	if len(conn.commands) != 1 || !strings.Contains(conn.commands[0], completionsMarker) {
		t.Fatalf("Uninstall() commands = %v", conn.commands)
	}
}
