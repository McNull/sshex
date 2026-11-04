package sshknownhostsmgr

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Host is a concrete hostname from a known_hosts entry.
type Host struct {
	Name   string
	Port   int
	Marker string
	File   string
	Line   int
}

// SshKnownHostsMgr reads OpenSSH known_hosts files.
type SshKnownHostsMgr struct{ files []string }

func NewSshKnownHostsMgr(files ...string) *SshKnownHostsMgr {
	return &SshKnownHostsMgr{files: append([]string(nil), files...)}
}

func (m *SshKnownHostsMgr) List() ([]Host, error) {
	files := m.files
	if files == nil {
		home, _ := os.UserHomeDir()
		files = []string{"/etc/ssh/ssh_known_hosts", "/etc/ssh/ssh_known_hosts2", "/etc/known_hosts", "/etc/known_hosts2", home + "/.ssh/known_hosts", home + "/.ssh/known_hosts2"}
	}
	var hosts []Host
	for _, file := range files {
		file = expandTilde(file)
		f, err := os.Open(file)
		if os.IsNotExist(err) && m.files == nil {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read known_hosts file %q: %w", file, err)
		}
		s := bufio.NewScanner(f)
		for line := 1; s.Scan(); line++ {
			fields := strings.Fields(s.Text())
			if len(fields) < 3 {
				continue
			}
			i := 0
			marker := ""
			if strings.HasPrefix(fields[0], "@") {
				marker, i = fields[0], 1
			}
			if i >= len(fields) {
				continue
			}
			for _, token := range strings.Split(fields[i], ",") {
				hostname, port := splitHost(token)
				if hostname == "" || strings.HasPrefix(hostname, "|") || strings.ContainsAny(hostname, "*?") {
					continue
				}
				hosts = append(hosts, Host{Name: hostname, Port: port, Marker: marker, File: file, Line: line})
			}
		}
		if err := s.Err(); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("read known_hosts file %q: %w", file, err)
		}
		if err := f.Close(); err != nil {
			return nil, err
		}
	}
	return hosts, nil
}

func expandTilde(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		if path == "~" {
			return home
		}
		return home + path[1:]
	}
	return path
}

func splitHost(token string) (string, int) {
	if !strings.HasPrefix(token, "[") {
		return token, 22
	}
	end := strings.IndexByte(token, ']')
	if end < 0 {
		return "", 0
	}
	port, err := strconv.Atoi(strings.TrimPrefix(token[end+1:], ":"))
	if err != nil || port < 1 || port > 65535 {
		return "", 0
	}
	return token[1:end], port
}
