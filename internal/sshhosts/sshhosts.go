package sshhosts

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxIncludeDepth = 16

type Host struct {
	Name        string
	Description string
}

type Options struct {
	Home string
	// ConfigFiles, KnownHostsFiles and HostsFiles override the defaults when
	// non-nil. An empty (non-nil) slice disables that source.
	ConfigFiles     []string
	KnownHostsFiles []string
	HostsFiles      []string
}

func Resolve(opts Options) ([]Host, error) {
	home := opts.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}

	configFiles := opts.ConfigFiles
	if configFiles == nil {
		configFiles = defaultConfigFiles(home)
	}
	configs := loadConfigs(configFiles, home)

	knownHostsFiles := opts.KnownHostsFiles
	if knownHostsFiles == nil {
		knownHostsFiles = knownHostsFromConfigs(configs, home)
	}

	hostsFiles := opts.HostsFiles
	if hostsFiles == nil {
		hostsFiles = []string{"/etc/hosts"}
	}

	var hosts []Host
	for _, cfg := range configs {
		hosts = append(hosts, cfg.Aliases...)
	}
	hosts = append(hosts, parseKnownHosts(knownHostsFiles, home)...)
	hosts = append(hosts, parseHostsFiles(hostsFiles, home)...)

	return dedupSort(hosts), nil
}

func defaultConfigFiles(home string) []string {
	return []string{
		"/etc/ssh/ssh_config",
		filepath.Join(home, ".ssh", "config"),
		filepath.Join(home, ".ssh2", "config"),
	}
}

func knownHostsFromConfigs(configs []parsedConfig, home string) []string {
	files := []string{
		"/etc/ssh/ssh_known_hosts",
		"/etc/ssh/ssh_known_hosts2",
		"/etc/known_hosts",
		"/etc/known_hosts2",
		filepath.Join(home, ".ssh", "known_hosts"),
		filepath.Join(home, ".ssh", "known_hosts2"),
	}
	for _, cfg := range configs {
		files = append(files, cfg.GlobalKnownHostsFile...)
		files = append(files, cfg.UserKnownHostsFile...)
	}
	return files
}

type parsedConfig struct {
	Aliases              []Host
	Includes             []string
	GlobalKnownHostsFile []string
	UserKnownHostsFile   []string
}

func loadConfigs(initial []string, home string) []parsedConfig {
	var out []parsedConfig
	seen := make(map[string]bool)
	queue := append([]string(nil), initial...)

	for depth := 0; len(queue) > 0 && depth < maxIncludeDepth; depth++ {
		var next []string
		for _, name := range queue {
			name = expandTilde(name, home)
			if seen[name] {
				continue
			}
			seen[name] = true

			data, err := os.ReadFile(name)
			if err != nil {
				continue
			}
			cfg := parseConfig(string(data))
			out = append(out, cfg)

			base := includeBase(name, home)
			for _, pattern := range cfg.Includes {
				next = append(next, expandInclude(pattern, base, home)...)
			}
		}
		queue = next
	}
	return out
}

func includeBase(configPath, home string) string {
	if strings.HasPrefix(configPath, "/etc/ssh") {
		return "/etc/ssh"
	}
	if home != "" && strings.HasPrefix(configPath, filepath.Join(home, ".ssh2")) {
		return filepath.Join(home, ".ssh2")
	}
	if home != "" {
		return filepath.Join(home, ".ssh")
	}
	return filepath.Dir(configPath)
}

func parseConfig(content string) parsedConfig {
	var cfg parsedConfig
	start, end := -1, -1

	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(stripComment(raw))
		if line == "" {
			continue
		}
		key, value := splitKeyValue(line)
		switch strings.ToLower(key) {
		case "host":
			start = len(cfg.Aliases)
			for _, pattern := range strings.Fields(value) {
				if validHostPattern(pattern) {
					cfg.Aliases = append(cfg.Aliases, Host{Name: pattern})
				}
			}
			end = len(cfg.Aliases)
			if start == end {
				start = -1
			}
		case "match":
			start, end = -1, -1
		case "hostname":
			if start >= 0 && value != "" {
				for i := start; i < end; i++ {
					if cfg.Aliases[i].Description == "" {
						cfg.Aliases[i].Description = value
					}
				}
			}
		case "include":
			cfg.Includes = append(cfg.Includes, splitFields(value)...)
		case "globalknownhostsfile":
			cfg.GlobalKnownHostsFile = append(cfg.GlobalKnownHostsFile, splitFields(value)...)
		case "userknownhostsfile":
			cfg.UserKnownHostsFile = append(cfg.UserKnownHostsFile, splitFields(value)...)
		}
	}
	return cfg
}

func validHostPattern(pattern string) bool {
	if pattern == "" || strings.HasPrefix(pattern, "!") {
		return false
	}
	return !strings.ContainsAny(pattern, "*?%")
}

func parseKnownHosts(files []string, home string) []Host {
	var hosts []Host
	for _, name := range files {
		name = expandTilde(name, home)
		data, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		for _, raw := range strings.Split(string(data), "\n") {
			fields := strings.Fields(raw)
			if len(fields) == 0 {
				continue
			}
			i := 0
			if strings.HasPrefix(fields[0], "@") {
				i = 1
			}
			if i >= len(fields) {
				continue
			}
			hostList := fields[i]
			if strings.HasPrefix(hostList, "|") || strings.HasPrefix(hostList, "#") {
				continue
			}
			for _, host := range strings.Split(hostList, ",") {
				host = normalizeKnownHost(host)
				if host == "" || strings.ContainsAny(host, "*?") {
					continue
				}
				hosts = append(hosts, Host{Name: host})
			}
		}
	}
	return hosts
}

func normalizeKnownHost(host string) string {
	if !strings.HasPrefix(host, "[") {
		return host
	}
	end := strings.Index(host, "]")
	if end < 0 {
		return host
	}
	return host[1:end]
}

func parseHostsFiles(files []string, home string) []Host {
	var hosts []Host
	for _, name := range files {
		name = expandTilde(name, home)
		data, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		for _, raw := range strings.Split(string(data), "\n") {
			line := strings.TrimSpace(stripComment(raw))
			if line == "" {
				continue
			}
			fields := strings.Fields(line)
			for _, host := range fields[1:] {
				hosts = append(hosts, Host{Name: host})
			}
		}
	}
	return hosts
}

func dedupSort(hosts []Host) []Host {
	index := make(map[string]int)
	var out []Host
	for _, host := range hosts {
		if host.Name == "" {
			continue
		}
		if i, ok := index[host.Name]; ok {
			if out[i].Description == "" {
				out[i].Description = host.Description
			}
			continue
		}
		index[host.Name] = len(out)
		out = append(out, host)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func expandInclude(pattern, base, home string) []string {
	pattern = strings.TrimSpace(pattern)
	pattern = strings.Trim(pattern, `"`)
	if pattern == "" {
		return nil
	}
	if !strings.HasPrefix(pattern, "/") && !strings.HasPrefix(pattern, "~") {
		pattern = filepath.Join(base, pattern)
	}
	pattern = expandTilde(pattern, home)

	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return []string{pattern}
	}
	return matches
}

func expandTilde(path, home string) string {
	if home == "" {
		return path
	}
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

func splitKeyValue(line string) (string, string) {
	if i := strings.IndexAny(line, " \t="); i >= 0 {
		return line[:i], strings.TrimSpace(line[i+1:])
	}
	return line, ""
}

func splitFields(value string) []string {
	value = strings.NewReplacer(`"`, " ", `'`, " ").Replace(value)
	return strings.Fields(value)
}

func stripComment(line string) string {
	inQuote := false
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '"':
			inQuote = !inQuote
		case '#':
			if !inQuote && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t') {
				return line[:i]
			}
		}
	}
	return line
}
