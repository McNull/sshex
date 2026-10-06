package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig    `yaml:"server"`
	Hosts    []HostConfig    `yaml:"hosts,omitempty"`
	Commands []CommandConfig `yaml:"commands,omitempty"`
}

// CommandConfig is a predefined command executed on the origin machine.
type CommandConfig struct {
	Name     string `yaml:"name"`
	Command  string `yaml:"command"`
	Alias    string `yaml:"alias,omitempty"`
	Disabled bool   `yaml:"disabled,omitempty"`
}

type ServerConfig struct {
	Transport         string   `yaml:"transport"`
	SocketDir         string   `yaml:"socket_dir,omitempty"`
	HeartbeatInterval Duration `yaml:"heartbeat_interval,omitempty"`
	HeartbeatTimeout  Duration `yaml:"heartbeat_timeout,omitempty"`
}

// Duration is a time.Duration that accepts either a Go duration string
// ("10s", "1m") or a bare integer number of seconds in YAML.
type Duration time.Duration

func (d Duration) Duration() time.Duration { return time.Duration(d) }

func (d Duration) MarshalYAML() (any, error) {
	return time.Duration(d).String(), nil
}

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var n int
	if err := value.Decode(&n); err == nil {
		*d = Duration(time.Duration(n) * time.Second)
		return nil
	}
	var s string
	if err := value.Decode(&s); err != nil {
		return fmt.Errorf("invalid duration: %s", value.Value)
	}
	parsed, perr := time.ParseDuration(s)
	if perr != nil {
		return perr
	}
	*d = Duration(parsed)
	return nil
}

type HostConfig struct {
	Name         string          `yaml:"name"`
	Host         string          `yaml:"host"`
	User         string          `yaml:"user,omitempty"`
	Port         int             `yaml:"port,omitempty"`
	IdentityFile string          `yaml:"identity_file,omitempty"`
	Options      []string        `yaml:"options,omitempty"`
	Forwards     []ForwardConfig `yaml:"forwards,omitempty"`
}

type ForwardConfig struct {
	LocalPort  int    `yaml:"local_port"`
	TargetHost string `yaml:"target_host,omitempty"`
	TargetPort int    `yaml:"target_port,omitempty"`
}

const (
	DirName  = "sshex"
	FileName = "config.yml"

	DefaultHeartbeatInterval = 10 * time.Second
	DefaultHeartbeatTimeout  = 30 * time.Second
)

func Default() Config {
	return Config{
		Server: ServerConfig{
			Transport:         "unix",
			HeartbeatInterval: Duration(DefaultHeartbeatInterval),
			HeartbeatTimeout:  Duration(DefaultHeartbeatTimeout),
		},
	}
}

func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, DirName), nil
}

func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, FileName), nil
}

// Resolve returns the config file path to use: an explicit path wins, then the
// SSHEX_CONFIG environment variable, then the default location.
func Resolve(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if env := os.Getenv("SSHEX_CONFIG"); env != "" {
		return env, nil
	}
	return Path()
}

func RuntimeDir() (string, error) {
	if dir := os.Getenv("SSHEX_RUNTIME_DIR"); dir != "" {
		return dir, nil
	}
	return runtimeDirFor(os.Getenv("SSHEX_CONFIG"))
}

// runtimeDirFor derives the runtime directory. A non-default config file gets
// its own namespace so a test config has an isolated broker, sockets, runtime
// file and local session files.
func runtimeDirFor(configPath string) (string, error) {
	base := os.TempDir()
	hasXDG := false
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		base = dir
		hasXDG = true
	}
	name := DirName
	if configPath != "" && !isDefaultConfig(configPath) {
		name = fmt.Sprintf("%s-%s", DirName, shortHash(configPath))
	}
	if hasXDG {
		return filepath.Join(base, name), nil
	}
	return filepath.Join(base, fmt.Sprintf("%s-%d", name, os.Getuid())), nil
}

func isDefaultConfig(path string) bool {
	def, err := Path()
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return path == def
	}
	absDef, err := filepath.Abs(def)
	if err != nil {
		return path == def
	}
	return absPath == absDef
}

func shortHash(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	sum := sha256.Sum256([]byte(abs))
	return hex.EncodeToString(sum[:])[:12]
}

// Origin returns a stable, filesystem-safe identifier for this machine. It is
// used as the broker identity and is recorded on session files.
func Origin() string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return Config{}, err
	}
	cfg := Default()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

func LoadDefault() (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, err
	}
	return Load(path)
}

// Save writes the config to path atomically, creating the parent directory if
// needed. The write goes to a temporary file in the same directory and is
// renamed into place so a concurrent reader never sees a partial file.
func Save(path string, cfg Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".config-*.yml")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(0o600); err != nil {
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
	return os.Rename(tmpName, path)
}
