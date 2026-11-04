package connect

import (
	"slices"
	"strings"
	"testing"
)

func TestConnectCommand(t *testing.T) {
	t.Setenv("SSHEX_SESSION", "client-session")
	t.Setenv("SSHEX_VERBOSE", "")
	cmd, err := connectCommand("user@example.com", 22)
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
	for _, entry := range cmd.Env {
		if strings.HasPrefix(entry, "SSHEX_SESSION=") {
			t.Fatalf("env leaked the outer session: %q", entry)
		}
	}
}

func TestConnectCommandPropagatesGlobalVerbose(t *testing.T) {
	t.Setenv("SSHEX_VERBOSE", "1")
	cmd, err := connectCommand("host.example.test", 22)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--verbose", "connect", "host.example.test"}
	if got := cmd.Args[len(cmd.Args)-len(want):]; !slices.Equal(got, want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
	if !slices.Contains(cmd.Env, "SSHEX_VERBOSE=1") {
		t.Fatalf("verbose environment not propagated: %v", cmd.Env)
	}
}

func TestConnectCommandCarriesNonstandardPort(t *testing.T) {
	cmd, err := connectCommand("mitm.example.test", 222)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"connect", "-p", "222", "mitm.example.test"}
	if got := cmd.Args[len(cmd.Args)-4:]; !slices.Equal(got, want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
}

func TestConnectCommandKeepsArgumentSeparateFromPortDisplay(t *testing.T) {
	cmd, err := connectCommand("host.example.test:222", 22)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"connect", "host.example.test:222"}
	if got := cmd.Args[len(cmd.Args)-2:]; !slices.Equal(got, want) {
		t.Fatalf("args = %v, want typed target unchanged", got)
	}
}
