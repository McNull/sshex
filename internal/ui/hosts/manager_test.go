package hosts

import (
	"path/filepath"
	"testing"

	"github.com/mcnull/sshex/internal/hostfilemgr"
	"github.com/mcnull/sshex/internal/sshconfigmgr"
	"github.com/mcnull/sshex/internal/sshknownhostsmgr"
)

type hostFileFake []hostfilemgr.Host

func (f hostFileFake) List() ([]hostfilemgr.Host, error) { return f, nil }

type knownFake []sshknownhostsmgr.Host

func (f knownFake) List() ([]sshknownhostsmgr.Host, error) { return f, nil }

type configFake []sshconfigmgr.Host

func (f configFake) List() ([]sshconfigmgr.Host, error) { return f, nil }

func TestSearchKeepsSourcesAndRanksExactKnownHostFirst(t *testing.T) {
	mgr := HostMgr{
		KnownHosts: knownFake{{Name: "node-a.example.test", Port: 22}},
		SSHConfig:  configFake{{Alias: "mitm.example.test", HostName: "node-a.example.test", Port: 222}},
	}
	got, err := mgr.Search("node-a.example.test")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "node-a.example.test" || got[0].Port != 22 || got[1].Name != "mitm.example.test" || got[1].Port != 222 {
		t.Fatalf("got %+v", got)
	}
}

func TestRepositoryFixturesResolveExpectedCandidates(t *testing.T) {
	base := filepath.Join("..", "..", "..", "test-data")
	mgr := HostMgr{
		HostFile:   hostfilemgr.NewHostFileMgr(filepath.Join(base, "hostfile", "hosts")),
		KnownHosts: sshknownhostsmgr.NewSshKnownHostsMgr(filepath.Join(base, "sshknownhosts", "known_hosts")),
		SSHConfig:  sshconfigmgr.NewSshConfigMgr(filepath.Join(base, "sshconfig", "config")),
	}
	got, err := mgr.Search("node-a.example.test")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "node-a.example.test" || got[0].Port != 22 || got[1].Name != "mitm.example.test" || got[1].Port != 222 {
		t.Fatalf("fixture candidates = %+v", got)
	}
}

func TestListDeduplicatesByNameAndPortButKeepsOtherPorts(t *testing.T) {
	mgr := HostMgr{
		HostFile: hostFileFake{{Name: "same.example.test", Address: "192.0.2.1"}},
		KnownHosts: knownFake{
			{Name: "same.example.test", Port: 22},
			{Name: "same.example.test", Port: 222},
		},
		SSHConfig: configFake{{Alias: "same.example.test", HostName: "target.example.test", Port: 222}},
	}
	got, err := mgr.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want two distinct ports: %+v", len(got), got)
	}
	if got[0].Name != "same.example.test" || got[0].Port != 22 || !got[0].hasSource("known_hosts") || !got[0].hasSource("hosts") {
		t.Fatalf("port 22 candidate = %+v", got[0])
	}
	if got[1].Name != "same.example.test" || got[1].Port != 222 || !got[1].hasSource("known_hosts") || !got[1].hasSource("ssh_config") {
		t.Fatalf("port 222 candidate = %+v", got[1])
	}
}

func TestSearchKeepsConfigHostnameMatchAheadOfHostFileDuplicate(t *testing.T) {
	mgr := HostMgr{
		HostFile:   hostFileFake{{Name: "node-a.example.test", Address: "192.0.2.1"}},
		KnownHosts: knownFake{{Name: "node-a.example.test", Port: 22}},
		SSHConfig:  configFake{{Alias: "mitm.example.test", HostName: "node-a.example.test", Port: 222}},
	}
	got, err := mgr.Search("node-a.example.test")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "node-a.example.test" || got[0].Port != 22 || got[1].Name != "mitm.example.test" || got[1].Port != 222 {
		t.Fatalf("search candidates = %+v", got)
	}
}
