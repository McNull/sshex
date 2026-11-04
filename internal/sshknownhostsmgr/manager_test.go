package sshknownhostsmgr

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCustomKnownHosts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	data := "plain ssh-ed25519 AAAA\n[custom]:222 ssh-ed25519 AAAA\nh1,h2 ssh-rsa AAAA\n|1|hash ssh-rsa AAAA\n*.wild ssh-rsa AAAA\n@cert-authority ca.example ssh-rsa AAAA\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	hosts, err := NewSshKnownHostsMgr(path).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 5 {
		t.Fatalf("got %d entries: %+v", len(hosts), hosts)
	}
	if hosts[0].Name != "plain" || hosts[0].Port != 22 || hosts[1].Port != 222 || hosts[4].Marker != "@cert-authority" {
		t.Fatalf("hosts = %+v", hosts)
	}
}

func TestRepositoryFixture(t *testing.T) {
	hosts, err := NewSshKnownHostsMgr("../../test-data/sshknownhosts/known_hosts").List()
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 2 || hosts[0].Name != "node-a.example.test" || hosts[0].Port != 22 {
		t.Fatalf("fixture hosts = %+v", hosts)
	}
}
