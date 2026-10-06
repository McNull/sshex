package cmd

import "testing"

func TestRootCommandTreeHasNoDuplicates(t *testing.T) {
	rootCmd.InitDefaultHelpCmd()
	rootCmd.InitDefaultCompletionCmd("")

	counts := make(map[string]int)
	for _, c := range rootCmd.Commands() {
		counts[c.Name()]++
	}

	for name, count := range counts {
		if count != 1 {
			t.Errorf("command %q appears %d times, want 1", name, count)
		}
	}
	if counts["help"] != 1 {
		t.Errorf("help command count = %d, want 1", counts["help"])
	}
	if _, ok := counts["completion"]; ok {
		t.Error("built-in completion command should be disabled")
	}
}

func TestRootConfigFlag(t *testing.T) {
	flag := rootCmd.PersistentFlags().Lookup("config")
	if flag == nil {
		t.Fatal("root --config flag is not registered")
	}
	if flag.Shorthand != "c" {
		t.Fatalf("--config shorthand = %q, want c", flag.Shorthand)
	}
}

func TestRootHasExecAndCommand(t *testing.T) {
	names := make(map[string]bool)
	for _, c := range rootCmd.Commands() {
		names[c.Name()] = true
	}
	for _, want := range []string{"exec", "command"} {
		if !names[want] {
			t.Errorf("root command %q is not registered", want)
		}
	}
}
