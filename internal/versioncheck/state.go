// Package versioncheck checks for newer sshex releases and caches the result.
package versioncheck

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/mcnull/sshex/internal/config"
	"gopkg.in/yaml.v3"
)

const (
	StateFileName = "version.yml"
	CheckInterval = 24 * time.Hour
	fileMode      = 0o600
	dirMode       = 0o700
)

// State records the most recent check and the latest version found, if any.
type State struct {
	CheckedAt        time.Time `yaml:"checked_at"`
	AvailableVersion string    `yaml:"available_version,omitempty"`
	Error            string    `yaml:"error,omitempty"`
}

// Store reads and writes version state under an adjacent stable lock file.
// Locking a separate file is necessary because saves atomically replace the
// YAML file itself.
type Store struct {
	path string
}

func NewStore(path string) *Store { return &Store{path: path} }

// DefaultPath returns the version state path under the user's config home,
// independent of any selected sshex config file.
func DefaultPath() (string, error) {
	if override := os.Getenv("SSHEX_VERSION_STATE"); override != "" {
		return override, nil
	}
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, StateFileName), nil
}

// Load returns an empty state when the state file does not exist.
func (s *Store) Load() (State, error) {
	unlock, err := s.lock(syscall.LOCK_SH)
	if err != nil {
		return State{}, err
	}
	defer unlock()
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return State{}, nil
	}
	if err != nil {
		return State{}, err
	}
	var state State
	if err := yaml.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("parse version state %s: %w", s.path, err)
	}
	return state, nil
}

// Save writes state atomically while holding the state lock.
func (s *Store) Save(state State) error {
	unlock, err := s.lock(syscall.LOCK_EX)
	if err != nil {
		return err
	}
	defer unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), dirMode); err != nil {
		return err
	}
	data, err := yaml.Marshal(state)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".version-*.yml")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(fileMode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.path)
}

// lock acquires a lock on a stable sibling file, not the replaceable YAML.
func (s *Store) lock(how int) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(s.path), dirMode); err != nil {
		return nil, err
	}
	lockFile, err := os.OpenFile(s.path+".lock", os.O_CREATE|os.O_RDWR, fileMode)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lockFile.Fd()), how); err != nil {
		_ = lockFile.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		_ = lockFile.Close()
	}, nil
}
