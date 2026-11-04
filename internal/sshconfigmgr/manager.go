package sshconfigmgr

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ssh_config "github.com/kevinburke/ssh_config"
	"github.com/mcnull/sshex/internal/sshx"
)

const maxIncludeDepth = 16

// Host is a concrete Host pattern and its effective connection settings.
type Host struct {
	Alias    string
	HostName string
	Port     int
	File     string
	Line     int
}

// SshConfigMgr reads OpenSSH client configuration files.
type SshConfigMgr struct {
	files []string
	home  string
}

// New uses the user's normal config locations when no files are supplied.
// Supplying files disables implicit system and user config loading.
func NewSshConfigMgr(files ...string) *SshConfigMgr {
	return &SshConfigMgr{files: append([]string(nil), files...)}
}

// WithHome sets the home directory used to expand ~/ paths and defaults.
func (m *SshConfigMgr) WithHome(home string) *SshConfigMgr { m.home = home; return m }

func (m *SshConfigMgr) List() ([]Host, error) {
	files := m.files
	home := m.home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if files == nil {
		files = []string{filepath.Join(home, ".ssh", "config"), "/etc/ssh/ssh_config"}
	}
	var configs []*ssh_config.Config
	var configFiles []string
	for _, path := range files {
		path = expandTilde(path, home)
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) && m.files == nil {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read ssh config %q: %w", path, err)
		}
		flat, err := expandIncludes(data, path, home, map[string]bool{}, 0)
		if err != nil {
			return nil, err
		}
		flat = normalizeMatchDirectives(flat)
		cfg, err := ssh_config.Decode(strings.NewReader(string(flat)))
		if err != nil {
			return nil, fmt.Errorf("parse ssh config %q: %w", path, err)
		}
		configs = append(configs, cfg)
		configFiles = append(configFiles, path)
	}
	var out []Host
	seen := make(map[string]bool)
	for ci, cfg := range configs {
		for _, stanza := range cfg.Hosts {
			for _, pattern := range stanza.Patterns {
				alias := pattern.String()
				if alias == "" || strings.HasPrefix(alias, "!") || strings.ContainsAny(alias, "*?") || seen[strings.ToLower(alias)] {
					continue
				}
				seen[strings.ToLower(alias)] = true
				host := Host{Alias: alias, File: configFiles[ci]}
				for _, candidate := range configs {
					if host.HostName == "" {
						host.HostName, _ = candidate.Get(alias, "HostName")
					}
					if host.Port == 0 {
						value, _ := candidate.Get(alias, "Port")
						if value != "" {
							_, _ = fmt.Sscanf(value, "%d", &host.Port)
						}
					}
				}
				out = append(out, host)
			}
		}
	}
	return out, nil
}

func expandIncludes(data []byte, file, home string, seen map[string]bool, depth int) ([]byte, error) {
	if depth >= maxIncludeDepth {
		return nil, fmt.Errorf("ssh config include depth exceeded at %q", file)
	}
	abs, err := filepath.Abs(file)
	if err != nil {
		return nil, err
	}
	if seen[abs] {
		return nil, fmt.Errorf("ssh config include cycle at %q", abs)
	}
	seen[abs] = true
	defer delete(seen, abs)
	var out strings.Builder
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(stripComment(line))
		key, value, ok := cutDirective(trimmed)
		if ok && strings.EqualFold(key, "Include") {
			patterns, splitErr := sshx.SplitArgs(value)
			if splitErr != nil {
				return nil, fmt.Errorf("parse Include in %q: %w", file, splitErr)
			}
			for _, pattern := range patterns {
				pattern = expandTilde(pattern, home)
				if !filepath.IsAbs(pattern) {
					pattern = filepath.Join(filepath.Dir(file), pattern)
				}
				matches, globErr := filepath.Glob(pattern)
				if globErr != nil {
					return nil, fmt.Errorf("expand Include %q in %q: %w", pattern, file, globErr)
				}
				for _, match := range matches {
					included, readErr := os.ReadFile(match)
					if readErr != nil {
						return nil, fmt.Errorf("read included ssh config %q: %w", match, readErr)
					}
					text, includeErr := expandIncludes(included, match, home, seen, depth+1)
					if includeErr != nil {
						return nil, includeErr
					}
					out.Write(text)
				}
			}
			continue
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return []byte(out.String()), nil
}

func expandTilde(path, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

func stripComment(line string) string {
	quoted := false
	for i, r := range line {
		if r == '"' {
			quoted = !quoted
		}
		if r == '#' && !quoted && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t') {
			return line[:i]
		}
	}
	return line
}
