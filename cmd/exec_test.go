package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestCompleteExecCommandNamesFallsBackAfterCommand(t *testing.T) {
	comps, directive := completeExecCommandNames(nil, []string{"code"}, "")
	if comps != nil {
		t.Fatalf("expected no completions, got %v", comps)
	}
	if directive != cobra.ShellCompDirectiveDefault {
		t.Fatalf("directive = %d, want ShellCompDirectiveDefault", directive)
	}
}
