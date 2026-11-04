package connect

import (
	"sort"
	"strings"

	"github.com/mcnull/sshex/internal/ui/hosts"
)

// Match ranks, best first. Ties keep the alphabetical host order.
const (
	rankExact = iota
	rankPrefix
	rankBoundary
	rankSubstring
)

type hostMatch struct {
	host hosts.Candidate
	rank int
}

// filterHosts returns the hosts matching query in best-match order. Matching is
// case-insensitive and applies to the host name only.
func filterHosts(items []hosts.Candidate, query string) []hosts.Candidate {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return append([]hosts.Candidate(nil), items...)
	}

	matches := make([]hostMatch, 0, len(items))
	for _, host := range items {
		rank, ok := matchRank(host.Name, query)
		if host.HasSource("known_hosts") && ok && rank == rankExact {
			rank = -1
		}
		if host.HasSource("hosts") && ok && !host.HasSource("known_hosts") && !host.HasSource("ssh_config") {
			rank += 3
		}
		if hostRank, hostOK := matchRank(host.HostName, query); hostOK && (!ok || hostRank+1 < rank) {
			rank, ok = hostRank+1, true
		}
		if !ok {
			continue
		}
		matches = append(matches, hostMatch{host: host, rank: rank})
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].rank != matches[j].rank {
			return matches[i].rank < matches[j].rank
		}
		left := strings.ToLower(matches[i].host.Name)
		right := strings.ToLower(matches[j].host.Name)
		if left != right {
			return left < right
		}
		return matches[i].host.Name < matches[j].host.Name
	})

	filtered := make([]hosts.Candidate, len(matches))
	for i, match := range matches {
		filtered[i] = match.host
	}
	return filtered
}

func matchRank(name, query string) (int, bool) {
	name = strings.ToLower(name)
	index := strings.Index(name, query)
	if index < 0 {
		return 0, false
	}

	switch {
	case index == 0 && len(name) == len(query):
		return rankExact, true
	case index == 0:
		return rankPrefix, true
	case atDomainBoundary(name, index, len(query)):
		return rankBoundary, true
	default:
		return rankSubstring, true
	}
}

// atDomainBoundary reports whether the match covers one or more complete
// dot-separated labels, such as nullest.com in gate.nullest.com.
func atDomainBoundary(name string, index, length int) bool {
	start := index == 0 || name[index-1] == '.'
	end := index+length == len(name) || name[index+length] == '.'
	return start && end
}
