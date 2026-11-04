package hostfilemgr

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCustomFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts")
	if err := os.WriteFile(path, []byte("127.0.0.1 local alias # comment\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hosts, err := NewHostFileMgr(path).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 2 || hosts[1].Name != "alias" || hosts[0].Address != "127.0.0.1" {
		t.Fatalf("hosts = %+v", hosts)
	}
}

func TestRepositoryFixture(t *testing.T) {
	hosts, err := NewHostFileMgr("../../test-data/hostfile/hosts").List()
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) == 0 || hosts[0].Name != "localhost" {
		t.Fatalf("fixture hosts = %+v", hosts)
	}
}

func TestCustomMissingFileIsError(t *testing.T) {
	if _, err := NewHostFileMgr(filepath.Join(t.TempDir(), "missing")).List(); err == nil {
		t.Fatal("expected error")
	}
}
