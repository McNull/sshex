package connect

import (
	"testing"

	"github.com/mcnull/sshex/internal/sshhosts"
)

func hostNames(hosts []sshhosts.Host) []string {
	names := make([]string, len(hosts))
	for i, host := range hosts {
		names[i] = host.Name
	}
	return names
}

func assertHostNames(t *testing.T, got []sshhosts.Host, want ...string) {
	t.Helper()
	names := hostNames(got)
	if len(names) != len(want) {
		t.Fatalf("hosts = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("hosts = %v, want %v", names, want)
		}
	}
}

func TestFilterHostsRanksMatches(t *testing.T) {
	hosts := []sshhosts.Host{
		{Name: "mynullest.com"},
		{Name: "gate.nullest.com"},
		{Name: "nullest.com.evil"},
		{Name: "nullest.com"},
		{Name: "nullestation.com"},
	}

	got := filterHosts(hosts, "nullest.com")
	assertHostNames(t, got,
		"nullest.com",
		"nullest.com.evil",
		"gate.nullest.com",
		"mynullest.com",
	)
}

func TestFilterHostsPrefixBeforeDomainMatch(t *testing.T) {
	hosts := []sshhosts.Host{
		{Name: "gate.nullest.com"},
		{Name: "nullest.com.au"},
	}

	got := filterHosts(hosts, "nullest.com")
	assertHostNames(t, got, "nullest.com.au", "gate.nullest.com")
}

func TestFilterHostsIsCaseInsensitive(t *testing.T) {
	hosts := []sshhosts.Host{
		{Name: "GATE.NULLEST.COM"},
		{Name: "Nullest.COM"},
	}

	got := filterHosts(hosts, "nullest.com")
	assertHostNames(t, got, "Nullest.COM", "GATE.NULLEST.COM")
}

func TestFilterHostsSortsTiesAlphabetically(t *testing.T) {
	hosts := []sshhosts.Host{
		{Name: "zeta.nullest.com"},
		{Name: "alpha.nullest.com"},
		{Name: "beta.nullest.com"},
	}

	got := filterHosts(hosts, "nullest.com")
	assertHostNames(t, got,
		"alpha.nullest.com",
		"beta.nullest.com",
		"zeta.nullest.com",
	)
}

func TestFilterHostsKeepsOrderWithoutQuery(t *testing.T) {
	hosts := []sshhosts.Host{
		{Name: "zeta"},
		{Name: "alpha"},
	}

	got := filterHosts(hosts, "  ")
	assertHostNames(t, got, "zeta", "alpha")
}
