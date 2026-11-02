package connect

import (
	"slices"
	"testing"
)

func TestConnectCommand(t *testing.T) {
	cmd, err := connectCommand("user@example.com")
	if err != nil {
		t.Fatalf("connectCommand() error: %v", err)
	}

	want := []string{"connect", "user@example.com"}
	if got := cmd.Args[len(cmd.Args)-2:]; !slices.Equal(got, want) {
		t.Fatalf("args = %v, want %v", cmd.Args, want)
	}
	if !slices.Contains(cmd.Env, noBannerEnv) {
		t.Fatalf("env does not contain %q", noBannerEnv)
	}
}
