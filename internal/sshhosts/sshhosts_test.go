package sshhosts

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func names(hosts []Host) []string {
	out := make([]string, len(hosts))
	for i, host := range hosts {
		out[i] = host.Name
	}
	return out
}

func assertNames(t *testing.T, got []Host, want ...string) {
	t.Helper()
	gotNames := names(got)
	if len(gotNames) != len(want) {
		t.Fatalf("hosts = %v, want %v", gotNames, want)
	}
	for i := range want {
		if gotNames[i] != want[i] {
			t.Fatalf("hosts = %v, want %v", gotNames, want)
		}
	}
}

func TestResolveConfigAliases(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, ".ssh", "config")
	writeFile(t, config, `
# comment
Host web
    HostName web.example.com
Host db db2
    HostName db.example.com
Host *.internal !bad
    HostName wild.example.com
`)

	hosts, err := Resolve(Options{
		Home:            dir,
		ConfigFiles:     []string{config},
		KnownHostsFiles: []string{},
		HostsFiles:      []string{},
	})
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	assertNames(t, hosts, "db", "db2", "web")
	if hosts[0].Description != "db.example.com" || hosts[2].Description != "web.example.com" {
		t.Fatalf("descriptions = %+v", hosts)
	}
}

func TestResolveIncludes(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, ".ssh", "config")
	writeFile(t, config, "Include extra/*.conf\nHost main\n")
	writeFile(t, filepath.Join(dir, ".ssh", "extra", "a.conf"), "Host alpha\n")
	writeFile(t, filepath.Join(dir, ".ssh", "extra", "b.conf"), "Host beta\n")

	hosts, err := Resolve(Options{
		Home:            dir,
		ConfigFiles:     []string{config},
		KnownHostsFiles: []string{},
		HostsFiles:      []string{},
	})
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	assertNames(t, hosts, "alpha", "beta", "main")
}

func TestResolveKnownHosts(t *testing.T) {
	dir := t.TempDir()
	known := filepath.Join(dir, "known_hosts")
	writeFile(t, known, "|1|hashed|entry ssh-rsa AAAA\n"+
		"host1,host2 ssh-ed25519 AAAA\n"+
		"[host3]:2222 ssh-rsa AAAA\n"+
		"*.wild ssh-rsa AAAA\n"+
		"@cert-authority ca.example ssh-rsa AAAA\n")

	hosts, err := Resolve(Options{
		Home:            dir,
		ConfigFiles:     []string{},
		KnownHostsFiles: []string{known},
		HostsFiles:      []string{},
	})
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	assertNames(t, hosts, "ca.example", "host1", "host2", "host3")
}

func TestResolveHostsFile(t *testing.T) {
	dir := t.TempDir()
	hostsFile := filepath.Join(dir, "hosts")
	writeFile(t, hostsFile, "127.0.0.1 localhost\n"+
		"10.0.0.1 srv1 srv1.local\n"+
		"# comment\n")

	hosts, err := Resolve(Options{
		Home:            dir,
		ConfigFiles:     []string{},
		KnownHostsFiles: []string{},
		HostsFiles:      []string{hostsFile},
	})
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	assertNames(t, hosts, "localhost", "srv1", "srv1.local")
}

func TestResolveUserKnownHostsFromConfig(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, ".ssh", "config")
	writeFile(t, config, "UserKnownHostsFile ~/.ssh/custom_known_hosts\n")
	writeFile(t, filepath.Join(dir, ".ssh", "custom_known_hosts"), "custom.example ssh-ed25519 AAAA\n")

	hosts, err := Resolve(Options{
		Home:        dir,
		ConfigFiles: []string{config},
		HostsFiles:  []string{},
	})
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if !slices.Contains(names(hosts), "custom.example") {
		t.Fatalf("hosts = %v, want it to contain custom.example", names(hosts))
	}
}

func TestResolveDedupKeepsDescription(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, ".ssh", "config")
	writeFile(t, config, "Host srv\n    HostName srv.example.com\n")
	hostsFile := filepath.Join(dir, "hosts")
	writeFile(t, hostsFile, "10.0.0.1 srv\n")

	hosts, err := Resolve(Options{
		Home:            dir,
		ConfigFiles:     []string{config},
		KnownHostsFiles: []string{},
		HostsFiles:      []string{hostsFile},
	})
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	assertNames(t, hosts, "srv")
	if hosts[0].Description != "srv.example.com" {
		t.Fatalf("description = %q, want srv.example.com", hosts[0].Description)
	}
}

func TestStripComment(t *testing.T) {
	cases := map[string]string{
		"Host web # trailing": "Host web ",
		"Host web#notcomment": "Host web#notcomment",
		`HostName "a#b"`:      `HostName "a#b"`,
		"# full line":         "",
	}
	for in, want := range cases {
		if got := stripComment(in); got != want {
			t.Errorf("stripComment(%q) = %q, want %q", in, got, want)
		}
	}
}
