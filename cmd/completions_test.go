package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCompletionsAcceptsShellPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"zsh", "#compdef sshex"},
		{"/bin/zsh", "#compdef sshex"},
		{"bash", "bash completion V2 for sshex"},
		{"/usr/bin/bash", "bash completion V2 for sshex"},
		{"fish", "fish completion for sshex"},
		{"/usr/local/bin/fish", "fish completion for sshex"},
	}
	for _, tt := range tests {
		var out bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetOut(&out)
		if err := completionsCmd.RunE(cmd, []string{tt.input}); err != nil {
			t.Fatalf("completions(%q) error: %v", tt.input, err)
		}
		if !strings.Contains(out.String(), tt.want) {
			t.Fatalf("completions(%q) missing %q:\n%s", tt.input, tt.want, out.String())
		}
	}
}

func TestCompletionsRejectsUnsupportedShell(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	if err := completionsCmd.RunE(cmd, []string{"/bin/pwsh"}); err == nil {
		t.Fatal("completions(/bin/pwsh) error = nil, want error")
	}
}
