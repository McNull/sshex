package hostfilemgr

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Host is an entry from a hosts(5) file.
type Host struct {
	Address string
	Name    string
	File    string
	Line    int
}

// HostFileMgr reads /etc/hosts or explicitly supplied files.
type HostFileMgr struct{ files []string }

func NewHostFileMgr(files ...string) *HostFileMgr {
	return &HostFileMgr{files: append([]string(nil), files...)}
}

func (m *HostFileMgr) List() ([]Host, error) {
	files := m.files
	if files == nil {
		files = []string{"/etc/hosts"}
	}
	var hosts []Host
	for _, name := range files {
		f, err := os.Open(name)
		if err != nil {
			return nil, fmt.Errorf("read hosts file %q: %w", name, err)
		}
		s := bufio.NewScanner(f)
		for line := 1; s.Scan(); line++ {
			text := strings.TrimSpace(strings.SplitN(s.Text(), "#", 2)[0])
			fields := strings.Fields(text)
			if len(fields) < 2 {
				continue
			}
			for _, host := range fields[1:] {
				hosts = append(hosts, Host{Address: fields[0], Name: host, File: name, Line: line})
			}
		}
		if err := s.Err(); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("read hosts file %q: %w", name, err)
		}
		if err := f.Close(); err != nil {
			return nil, err
		}
	}
	return hosts, nil
}
