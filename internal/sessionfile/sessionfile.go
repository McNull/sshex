package sessionfile

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcnull/sshex/internal/config"
	"github.com/mcnull/sshex/internal/model"
)

const FileMode = 0o600

var (
	ErrNone      = errors.New("no active sshex session")
	ErrAmbiguous = errors.New("multiple active sshex sessions; set SSHEX_SESSION")
)

type File struct {
	ID           string             `json:"id"`
	Origin       string             `json:"origin,omitempty"`
	Endpoint     model.Endpoint     `json:"endpoint"`
	Token        string             `json:"token"`
	Capabilities model.Capabilities `json:"capabilities"`
	Target       string             `json:"target,omitempty"`
}

func Marshal(f File) ([]byte, error) {
	return json.MarshalIndent(f, "", "  ")
}

func Read(path string) (File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return File{}, err
	}
	return f, nil
}

func WriteLocal(dir string, f File) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := Marshal(f)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, f.ID+".json"), data, FileMode)
}

func RemoveLocal(dir, id string) error {
	err := os.Remove(filepath.Join(dir, id+".json"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func ListDir(dir string) ([]File, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var files []File
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		f, err := Read(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		files = append(files, f)
	}
	return files, nil
}

func LocalDir() (string, error) {
	runtimeDir, err := config.RuntimeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(runtimeDir, "sessions"), nil
}

func RemoteDir() (string, error) {
	if root := os.Getenv("SSHEX_HOME"); root != "" {
		return filepath.Join(root, "sessions"), nil
	}
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "sshex", "sessions"), nil
}

func Discover() (File, error) {
	if value := os.Getenv("SSHEX_SESSION"); value != "" {
		if strings.ContainsRune(value, os.PathSeparator) {
			return Read(value)
		}
		for _, dir := range discoverDirs() {
			if f, err := Read(filepath.Join(dir, value+".json")); err == nil {
				return f, nil
			}
		}
		return File{}, ErrNone
	}

	var candidates []File
	for _, dir := range discoverDirs() {
		files, err := ListDir(dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if alive(f) {
				candidates = append(candidates, f)
			}
		}
	}
	switch len(candidates) {
	case 0:
		return File{}, ErrNone
	case 1:
		return candidates[0], nil
	default:
		return File{}, ErrAmbiguous
	}
}

// ListActive returns every session file whose endpoint is reachable from this
// machine, without treating more than one as an error.
func ListActive() ([]File, error) {
	var active []File
	for _, dir := range discoverDirs() {
		files, err := ListDir(dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if alive(f) {
				active = append(active, f)
			}
		}
	}
	return active, nil
}

func alive(f File) bool {
	var network, address string
	switch f.Endpoint.Kind {
	case model.EndpointUnix:
		network, address = "unix", f.Endpoint.Address
	case model.EndpointTCP:
		network, address = "tcp", f.Endpoint.Address
	default:
		return false
	}
	conn, err := net.DialTimeout(network, address, 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func discoverDirs() []string {
	var dirs []string
	if dir, err := LocalDir(); err == nil {
		dirs = append(dirs, dir)
	}
	if dir, err := RemoteDir(); err == nil {
		dirs = append(dirs, dir)
	}
	return dirs
}
