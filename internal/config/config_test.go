package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.Server.Transport != "unix" {
		t.Fatalf("Default().Server.Transport = %q, want unix", cfg.Server.Transport)
	}
	if cfg.Server.HeartbeatInterval.Duration() != 10*time.Second {
		t.Fatalf("Default().Server.HeartbeatInterval = %v, want 10s", cfg.Server.HeartbeatInterval.Duration())
	}
	if cfg.Server.HeartbeatTimeout.Duration() != 30*time.Second {
		t.Fatalf("Default().Server.HeartbeatTimeout = %v, want 30s", cfg.Server.HeartbeatTimeout.Duration())
	}
}

func TestLoadHeartbeatOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	data := "server:\n  heartbeat_interval: 5s\n  heartbeat_timeout: 15s\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Server.HeartbeatInterval.Duration() != 5*time.Second {
		t.Fatalf("interval = %v, want 5s", cfg.Server.HeartbeatInterval.Duration())
	}
	if cfg.Server.HeartbeatTimeout.Duration() != 15*time.Second {
		t.Fatalf("timeout = %v, want 15s", cfg.Server.HeartbeatTimeout.Duration())
	}
}

func TestLoadHeartbeatBareSeconds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	data := "server:\n  heartbeat_interval: 4\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Server.HeartbeatInterval.Duration() != 4*time.Second {
		t.Fatalf("interval = %v, want 4s", cfg.Server.HeartbeatInterval.Duration())
	}
	// Unset timeout keeps the default.
	if cfg.Server.HeartbeatTimeout.Duration() != 30*time.Second {
		t.Fatalf("timeout = %v, want default 30s", cfg.Server.HeartbeatTimeout.Duration())
	}
}

func TestDirAndPath(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir() error: %v", err)
	}
	if dir != filepath.Join(base, DirName) {
		t.Fatalf("Dir() = %q, want %q", dir, filepath.Join(base, DirName))
	}

	path, err := Path()
	if err != nil {
		t.Fatalf("Path() error: %v", err)
	}
	if path != filepath.Join(dir, FileName) {
		t.Fatalf("Path() = %q, want %q", path, filepath.Join(dir, FileName))
	}
}

func TestLoadMissingReturnsDefault(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault() error: %v", err)
	}
	if cfg.Server.Transport != "unix" {
		t.Fatalf("LoadDefault() transport = %q, want unix", cfg.Server.Transport)
	}
}

func TestEnsureCreatesDefaultCommands(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", FileName)

	cfg, err := Ensure(path)
	if err != nil {
		t.Fatalf("Ensure() error: %v", err)
	}
	if len(cfg.Commands) != 2 {
		t.Fatalf("Ensure() commands = %+v, want code and zed", cfg.Commands)
	}

	want := DefaultCommands()
	for i, command := range cfg.Commands {
		if command != want[i] {
			t.Fatalf("command %d = %+v, want %+v", i, command, want[i])
		}
		if !command.Disabled {
			t.Fatalf("default command %q is not disabled", command.Name)
		}
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(loaded.Commands) != 2 || loaded.Commands[0].Name != "code" || loaded.Commands[1].Name != "zed" {
		t.Fatalf("saved commands = %+v, want code and zed", loaded.Commands)
	}
}

func TestEnsureKeepsExistingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	data := "server:\n  transport: tcp\ncommands:\n  - name: custom\n    command: custom\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}

	cfg, err := Ensure(path)
	if err != nil {
		t.Fatalf("Ensure() error: %v", err)
	}
	if cfg.Server.Transport != "tcp" {
		t.Fatalf("transport = %q, want tcp", cfg.Server.Transport)
	}
	if len(cfg.Commands) != 1 || cfg.Commands[0].Name != "custom" {
		t.Fatalf("commands = %+v, want only custom", cfg.Commands)
	}
}

func TestLoadValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	data := "server:\n  transport: tcp\nhosts:\n  - name: ssh01\n    host: ssh01\n    user: tester\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Server.Transport != "tcp" {
		t.Fatalf("transport = %q, want tcp", cfg.Server.Transport)
	}
	if len(cfg.Hosts) != 1 || cfg.Hosts[0].Name != "ssh01" {
		t.Fatalf("hosts = %+v, want one ssh01", cfg.Hosts)
	}
}

func TestLoadInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte("server: [unterminated"), 0o600); err != nil {
		t.Fatalf("write error: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load() expected error for invalid YAML, got nil")
	}
}

func TestSaveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", FileName)
	cfg := Default()
	cfg.Server.Transport = "tcp"
	cfg.Commands = []CommandConfig{{Name: "code", Command: `code --folder-uri "vscode-remote://ssh-remote+${REMOTE_HOST}"`}}
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if loaded.Server.Transport != "tcp" {
		t.Fatalf("transport = %q, want tcp", loaded.Server.Transport)
	}
	if loaded.Server.HeartbeatInterval.Duration() != DefaultHeartbeatInterval {
		t.Fatalf("heartbeat_interval = %v, want %v", loaded.Server.HeartbeatInterval.Duration(), DefaultHeartbeatInterval)
	}
	if loaded.Server.HeartbeatTimeout.Duration() != DefaultHeartbeatTimeout {
		t.Fatalf("heartbeat_timeout = %v, want %v", loaded.Server.HeartbeatTimeout.Duration(), DefaultHeartbeatTimeout)
	}
	if len(loaded.Commands) != 1 || loaded.Commands[0].Name != "code" {
		t.Fatalf("commands = %+v, want one code", loaded.Commands)
	}
	if loaded.Commands[0].Disabled {
		t.Fatal("command unexpectedly disabled")
	}
}

func TestResolvePrecedence(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("SSHEX_CONFIG", "")

	explicit, err := Resolve("/tmp/explicit.yml")
	if err != nil {
		t.Fatalf("Resolve(explicit) error: %v", err)
	}
	if explicit != "/tmp/explicit.yml" {
		t.Fatalf("Resolve(explicit) = %q", explicit)
	}

	t.Setenv("SSHEX_CONFIG", "/tmp/env.yml")
	fromEnv, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve(env) error: %v", err)
	}
	if fromEnv != "/tmp/env.yml" {
		t.Fatalf("Resolve(env) = %q, want /tmp/env.yml", fromEnv)
	}

	t.Setenv("SSHEX_CONFIG", "")
	def, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve(default) error: %v", err)
	}
	if filepath.Base(def) != FileName {
		t.Fatalf("Resolve(default) = %q, want a %s path", def, FileName)
	}
}

func TestRuntimeDirIsolated(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", base)
	t.Setenv("SSHEX_RUNTIME_DIR", "")
	t.Setenv("SSHEX_CONFIG", "")

	def, err := RuntimeDir()
	if err != nil {
		t.Fatalf("RuntimeDir() error: %v", err)
	}
	if def != filepath.Join(base, DirName) {
		t.Fatalf("RuntimeDir() = %q, want %q", def, filepath.Join(base, DirName))
	}

	t.Setenv("SSHEX_CONFIG", filepath.Join(base, "test.yml"))
	isolated, err := RuntimeDir()
	if err != nil {
		t.Fatalf("RuntimeDir() error: %v", err)
	}
	if isolated == def {
		t.Fatalf("RuntimeDir() with test config = %q, want an isolated dir", isolated)
	}
	if !strings.HasPrefix(filepath.Base(isolated), DirName+"-") {
		t.Fatalf("RuntimeDir() = %q, want a %s- prefix", isolated, DirName)
	}

	t.Setenv("SSHEX_RUNTIME_DIR", filepath.Join(base, "explicit"))
	explicit, err := RuntimeDir()
	if err != nil {
		t.Fatalf("RuntimeDir() error: %v", err)
	}
	if explicit != filepath.Join(base, "explicit") {
		t.Fatalf("RuntimeDir() explicit = %q", explicit)
	}
}

func TestControlDir(t *testing.T) {
	t.Setenv("SSHEX_CONTROL_DIR", "")
	t.Setenv("SSHEX_CONTROL_BASE", t.TempDir())
	t.Setenv("SSHEX_CONFIG", "")

	def, err := ControlDir()
	if err != nil {
		t.Fatalf("ControlDir() error: %v", err)
	}
	if filepath.Base(def) != "control" {
		t.Fatalf("ControlDir() = %q, want a control dir", def)
	}
	if !strings.HasPrefix(filepath.Base(filepath.Dir(def)), DirName+"-") {
		t.Fatalf("ControlDir() parent = %q, want a %s- prefix", filepath.Dir(def), DirName)
	}

	isolated, err := ControlDirFor(filepath.Join(t.TempDir(), "test.yml"))
	if err != nil {
		t.Fatalf("ControlDirFor() error: %v", err)
	}
	if isolated == def {
		t.Fatalf("ControlDirFor() = %q, want an isolated dir", isolated)
	}

	t.Setenv("SSHEX_CONTROL_DIR", "/custom/control")
	explicit, err := ControlDir()
	if err != nil {
		t.Fatalf("ControlDir() error: %v", err)
	}
	if explicit != "/custom/control" {
		t.Fatalf("ControlDir() explicit = %q", explicit)
	}
}

func TestControlDirStaysShort(t *testing.T) {
	t.Setenv("SSHEX_CONTROL_DIR", "")
	t.Setenv("SSHEX_CONTROL_BASE", "")
	t.Setenv("SSHEX_CONFIG", "")
	dir, err := ControlDir()
	if err != nil {
		t.Fatalf("ControlDir() error: %v", err)
	}
	// OpenSSH appends a random suffix to the control path while creating it;
	// unix sockets allow roughly 104 bytes on macOS.
	if len(filepath.Join(dir, "0123456789abcdef.ctl"))+20 > 100 {
		t.Fatalf("ControlDir() = %q; control path too long for a unix socket", dir)
	}
}

func TestSaveThemeKeepsOtherSections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	cfg := Default()
	cfg.Commands = DefaultCommands()
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	if err := SaveTheme(path, "ocean"); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.UI.Theme != "ocean" || len(got.Commands) != len(cfg.Commands) {
		t.Fatalf("theme = %q, commands = %d", got.UI.Theme, len(got.Commands))
	}
}

func TestSaveThemeKeepsComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	src := "# my hosts\nserver:\n    transport: unix # keep\nui:\n    theme: mono\n"
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SaveTheme(path, "rose"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	got := string(data)
	for _, want := range []string{"# my hosts", "# keep", "theme: rose"} {
		if !strings.Contains(got, want) {
			t.Fatalf("config missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "mono") {
		t.Fatalf("old theme still present:\n%s", got)
	}
}
