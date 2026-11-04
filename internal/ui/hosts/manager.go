// Package hosts combines local hostname sources for the Connect screen.
package hosts

import (
	"sort"
	"strconv"
	"strings"

	"github.com/mcnull/sshex/internal/hostfilemgr"
	"github.com/mcnull/sshex/internal/sshconfigmgr"
	"github.com/mcnull/sshex/internal/sshknownhostsmgr"
)

// Candidate is one connectable entry and its source metadata.
type Candidate struct {
	Name        string
	Description string
	HostName    string
	Port        int
	Source      string
	Sources     []string
}

type HostFileSource interface {
	List() ([]hostfilemgr.Host, error)
}
type KnownHostsSource interface {
	List() ([]sshknownhostsmgr.Host, error)
}
type ConfigSource interface {
	List() ([]sshconfigmgr.Host, error)
}

// HostMgr unifies hostname sources for UI queries.
type HostMgr struct {
	HostFile   HostFileSource
	KnownHosts KnownHostsSource
	SSHConfig  ConfigSource
}

// List returns all candidates. It adapts the unified query service to screens
// that perform incremental local filtering.
func (m *HostMgr) List() ([]Candidate, error) { return m.Search("") }

// Search returns all candidates matching query, ordered by match quality.
func (m *HostMgr) Search(query string) ([]Candidate, error) {
	var candidates []Candidate
	if m.KnownHosts != nil {
		entries, err := m.KnownHosts.List()
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			candidates = append(candidates, Candidate{Name: entry.Name, Port: effectivePort(entry.Port), Source: "known_hosts", Sources: []string{"known_hosts"}})
		}
	}
	if m.SSHConfig != nil {
		entries, err := m.SSHConfig.List()
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			port := entry.Port
			if port == 0 {
				port = 22
			}
			candidates = append(candidates, Candidate{Name: entry.Alias, Description: entry.HostName, HostName: entry.HostName, Port: port, Source: "ssh_config", Sources: []string{"ssh_config"}})
		}
	}
	if m.HostFile != nil {
		entries, err := m.HostFile.List()
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			candidates = append(candidates, Candidate{Name: entry.Name, Description: entry.Address, Port: 22, Source: "hosts", Sources: []string{"hosts"}})
		}
	}
	candidates = deduplicate(candidates)
	query = strings.TrimSpace(query)
	if query == "" {
		return candidates, nil
	}
	queryLower := strings.ToLower(query)
	type ranked struct {
		candidate Candidate
		rank      int
	}
	var matches []ranked
	for _, candidate := range candidates {
		nameRank, nameMatch := matchRank(candidate.Name, queryLower)
		hostRank, hostMatch := matchRank(candidate.HostName, queryLower)
		if !nameMatch && !hostMatch {
			continue
		}
		rank := 4
		if nameMatch {
			rank = nameRank
		}
		if hostMatch && hostRank+1 < rank {
			rank = hostRank + 1
		}
		if candidate.hasSource("known_hosts") && nameMatch && nameRank == 0 {
			rank = -1
		}
		if candidate.hasSource("hosts") && nameMatch && !candidate.hasSource("known_hosts") && !candidate.hasSource("ssh_config") {
			rank = nameRank + 3
		}
		matches = append(matches, ranked{candidate, rank})
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].rank != matches[j].rank {
			return matches[i].rank < matches[j].rank
		}
		li, lj := strings.ToLower(matches[i].candidate.Name), strings.ToLower(matches[j].candidate.Name)
		if li != lj {
			return li < lj
		}
		if matches[i].candidate.Port != matches[j].candidate.Port {
			return matches[i].candidate.Port < matches[j].candidate.Port
		}
		return matches[i].candidate.Source < matches[j].candidate.Source
	})
	out := make([]Candidate, len(matches))
	for i := range matches {
		out[i] = matches[i].candidate
	}
	return out, nil
}

func effectivePort(port int) int {
	if port == 0 {
		return 22
	}
	return port
}

func deduplicate(candidates []Candidate) []Candidate {
	indexes := make(map[string]int, len(candidates))
	unique := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		candidate.Port = effectivePort(candidate.Port)
		key := strings.ToLower(candidate.Name) + "\x00" + strconv.Itoa(candidate.Port)
		if index, ok := indexes[key]; ok {
			current := &unique[index]
			current.Sources = mergeSources(current.Sources, current.Source, candidate.Sources, candidate.Source)
			if current.HostName == "" {
				current.HostName = candidate.HostName
			}
			if candidate.Description != "" && (current.Description == "" || candidate.hasSource("ssh_config")) {
				current.Description = candidate.Description
			}
			if current.Source != "ssh_config" && candidate.Source == "ssh_config" {
				// Keep the primary source useful for callers that don't inspect Sources.
				current.Source = candidate.Source
			}
			continue
		}
		if len(candidate.Sources) == 0 && candidate.Source != "" {
			candidate.Sources = []string{candidate.Source}
		}
		indexes[key] = len(unique)
		unique = append(unique, candidate)
	}
	return unique
}

func mergeSources(existing []string, source string, incoming []string, incomingSource string) []string {
	seen := make(map[string]bool, len(existing)+len(incoming)+2)
	out := make([]string, 0, len(existing)+len(incoming)+2)
	add := func(value string) {
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	add(source)
	for _, value := range existing {
		add(value)
	}
	add(incomingSource)
	for _, value := range incoming {
		add(value)
	}
	return out
}

func (candidate Candidate) hasSource(source string) bool {
	if candidate.Source == source {
		return true
	}
	for _, value := range candidate.Sources {
		if value == source {
			return true
		}
	}
	return false
}

// HasSource reports whether this candidate was contributed by source.
func (candidate Candidate) HasSource(source string) bool { return candidate.hasSource(source) }

func matchRank(name, query string) (int, bool) {
	if name == "" {
		return 0, false
	}
	name = strings.ToLower(name)
	i := strings.Index(name, query)
	if i < 0 {
		return 0, false
	}
	switch {
	case i == 0 && len(name) == len(query):
		return 0, true
	case i == 0:
		return 1, true
	case (i == 0 || name[i-1] == '.') && (i+len(query) == len(name) || name[i+len(query)] == '.'):
		return 2, true
	default:
		return 3, true
	}
}
