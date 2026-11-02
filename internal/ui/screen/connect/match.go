package connect

import (
	"sort"
	"strings"

	"github.com/mcnull/sshex/internal/sshhosts"
)

// Match ranks, best first. Ties keep the alphabetical host order.
const (
	rankExact = iota
	rankPrefix
	rankBoundary
	rankSubstring
)

type hostMatch struct {
	host sshhosts.Host
	rank int
}

// filterHosts returns the hosts matching query in best-match order. Matching is
// case-insensitive and applies to the host name only.
func filterHosts(hosts []sshhosts.Host, query string) []sshhosts.Host {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return append([]sshhosts.Host(nil), hosts...)
	}

	matches := make([]hostMatch, 0, len(hosts))
	for _, host := range hosts {
		rank, ok := matchRank(host.Name, query)
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

	filtered := make([]sshhosts.Host, len(matches))
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
