package sshconfigmgr

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, name, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCustomConfigAndIncludedFiles(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "config")
	write(t, root, "Include included/*.conf\nHost direct\n  HostName direct.example\n")
	write(t, filepath.Join(dir, "included", "alias.conf"), "Host mitm.example.test\n  HostName node-a.example.test\n  Port 222\n")
	hosts, err := NewSshConfigMgr(root).WithHome(dir).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 2 || hosts[0].Alias != "mitm.example.test" || hosts[0].HostName != "node-a.example.test" || hosts[0].Port != 222 {
		t.Fatalf("hosts = %+v", hosts)
	}
}

func TestRepositoryFixture(t *testing.T) {
	hosts, err := NewSshConfigMgr("../../test-data/sshconfig/config").List()
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range hosts {
		if host.Alias == "mitm.example.test" {
			if host.HostName != "node-a.example.test" || host.Port != 222 {
				t.Fatalf("fixture alias = %+v", host)
			}
			return
		}
	}
	t.Fatalf("fixture alias missing from %+v", hosts)
}

func TestFedoraMatchFinalFixture(t *testing.T) {
	hosts, err := NewSshConfigMgr("../../test-data/sshconfig-fedora/config").List()
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range hosts {
		if host.Alias == "fixture.example.test" {
			if host.HostName != "fixture-host.example.test" || host.Port != 2222 {
				t.Fatalf("fixture alias = %+v", host)
			}
			return
		}
	}
	t.Fatalf("fixture alias missing from %+v", hosts)
}

func TestNormalizeMatchDirectives(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"final all", "Match final all\n", "Match all\n"},
		{"final only", "Match final\n", "Match all\n"},
		{"final host", "Match final host a.example\n", "Match host a.example\n"},
		{"all", "Match all\n", "Match all\n"},
		{"host", "Match host a.example\n", "Match host a.example\n"},
		{"host negated", "Match host !*.example\n", "Match host !*.example\n"},
		{"user", "Match user bob\n", "Match host !*\n"},
		{"exec", "Match exec \"test -f /x\"\n", "Match host !*\n"},
		{"combined", "Match host a.example user bob\n", "Match host !*\n"},
		{"indented", "\tMatch final all\n", "\tMatch all\n"},
		{"non match untouched", "Host a\n", "Host a\n"},
		{"commented out", "# Match final all\n", "# Match final all\n"},
	}
	for _, tt := range tests {
		if got := string(normalizeMatchDirectives([]byte(tt.in))); got != tt.want {
			t.Errorf("%s: got %q want %q", tt.name, got, tt.want)
		}
	}
}

func TestUnsupportedMatchCriterionDoesNotFail(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "config")
	write(t, root, "Match user bob\n  Ciphers aes256-ctr\nHost tolerated\n  HostName tolerated.example\n")
	hosts, err := NewSshConfigMgr(root).WithHome(dir).List()
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range hosts {
		if host.Alias == "tolerated" {
			if host.HostName != "tolerated.example" {
				t.Fatalf("tolerated alias = %+v", host)
			}
			return
		}
	}
	t.Fatalf("tolerated alias missing from %+v", hosts)
}

func TestCustomConfigDoesNotFallBack(t *testing.T) {
	if _, err := NewSshConfigMgr(filepath.Join(t.TempDir(), "missing")).List(); err == nil {
		t.Fatal("expected custom config error")
	}
}

func TestIncludeCycle(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	write(t, a, "Include b\n")
	write(t, b, "Include a\n")
	if _, err := NewSshConfigMgr(a).List(); err == nil {
		t.Fatal("expected cycle error")
	}
}
