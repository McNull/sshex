package openssh

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcnull/sshex/internal/sshx"
)

func TestParseProxyOutput(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		wantJump string
		wantCmd  bool
	}{
		{
			name:   "no proxy",
			output: "user null\nhostname ssh02\nproxyusefdpass no\n",
		},
		{
			name:     "single jump",
			output:   "hostname ssh02\nproxyjump ssh01\n",
			wantJump: "ssh01",
		},
		{
			name:     "multiple jumps",
			output:   "hostname ssh03\nproxyjump ssh01,ssh02\n",
			wantJump: "ssh01,ssh02",
		},
		{
			name:   "jump none",
			output: "hostname ssh02\nproxyjump none\n",
		},
		{
			name:    "proxy command",
			output:  "hostname ssh02\nproxycommand ssh -W %h:%p ssh01\n",
			wantCmd: true,
		},
		{
			name:     "jump wins over command",
			output:   "proxyjump ssh01\nproxycommand ssh -W %h:%p ssh02\n",
			wantJump: "ssh01",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseProxyOutput(tc.output)
			if got.JumpHosts != tc.wantJump {
				t.Fatalf("parseProxyOutput() jump = %q, want %q", got.JumpHosts, tc.wantJump)
			}
			if got.ProxyCommand != tc.wantCmd {
				t.Fatalf("parseProxyOutput() proxyCommand = %v, want %v", got.ProxyCommand, tc.wantCmd)
			}
		})
	}
}

func TestResolveProxy(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ssh")
	contents := `#!/bin/sh
cat <<'EOF'
user null
hostname ssh03
proxyusefdpass no
proxyjump ssh01,ssh02
EOF
`
	if err := os.WriteFile(script, []byte(contents), 0o755); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}

	info, err := resolveProxy(context.Background(), script, sshx.Target{Host: "ssh03"})
	if err != nil {
		t.Fatalf("resolveProxy() error: %v", err)
	}
	if info.JumpHosts != "ssh01,ssh02" {
		t.Fatalf("resolveProxy() jump = %q, want ssh01,ssh02", info.JumpHosts)
	}
	if info.ProxyCommand {
		t.Fatal("resolveProxy() proxyCommand = true, want false")
	}
}

func TestResolveProxyPassesOptions(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	script := filepath.Join(dir, "ssh")
	contents := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argsFile + "\n"
	if err := os.WriteFile(script, []byte(contents), 0o755); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}

	target := sshx.Target{
		Host:         "ssh03",
		Port:         2222,
		IdentityFile: "/key",
		Options:      []string{"StrictHostKeyChecking=no", "-J bastion"},
	}
	if _, err := resolveProxy(context.Background(), script, target); err != nil {
		t.Fatalf("resolveProxy() error: %v", err)
	}
	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	want := "-G\n-p\n2222\n-i\n/key\n-o\nStrictHostKeyChecking=no\n-J\nbastion\nssh03\n"
	if string(data) != want {
		t.Fatalf("ssh args = %q, want %q", data, want)
	}
}
