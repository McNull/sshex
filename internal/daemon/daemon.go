package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/mcnull/sshex/internal/config"
	"github.com/mcnull/sshex/internal/errs"
	"github.com/mcnull/sshex/internal/model"
)

const (
	RuntimeFileName = "sshexd.json"
	RuntimeDirMode  = 0o700
	RuntimeFileMode = 0o600
	LockFileName    = "sshexd.lock"
	LogFileName     = "sshexd.log"
	ProtocolVersion = 2

	startTimeout  = 5 * time.Second
	startInterval = 50 * time.Millisecond
	dialTimeout   = 300 * time.Millisecond
	stopTimeout   = 3 * time.Second
)

// spawn starts the background service process. It is a package variable so
// tests can substitute a hermetic implementation.
var spawn = spawnDetached

var signalProcess = syscall.Kill

func spawnDetached(_ context.Context, configPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	dir, err := RuntimeDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, RuntimeDirMode); err != nil {
		return err
	}
	logFile, err := os.OpenFile(filepath.Join(dir, LogFileName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, RuntimeFileMode)
	if err != nil {
		return err
	}
	defer logFile.Close()

	args := []string{"serve"}
	if configPath != "" {
		args = append(args, "--config", configPath)
	}
	cmd := exec.Command(exe, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}

// Ensure returns the running background service, starting one if necessary.
func Ensure(ctx context.Context, configPath string) (RuntimeInfo, error) {
	return EnsureVerbose(ctx, configPath, nil)
}

// EnsureVerbose behaves like Ensure and reports daemon reuse/restart decisions
// to log when it is non-nil.
func EnsureVerbose(ctx context.Context, configPath string, log io.Writer) (RuntimeInfo, error) {
	if info, err := ReadRuntime(); err == nil {
		if EndpointAlive(info.Endpoint) {
			logDaemon(log, "found running daemon: pid=%d version=%s protocol=%d", info.PID, displayVersion(info.Version), info.Protocol)
			if info.Protocol == ProtocolVersion {
				logDaemon(log, "reusing compatible daemon")
				return info, nil
			}
			logDaemon(log, "daemon protocol is incompatible; checking for safe restart")
		} else {
			logDaemon(log, "found stale daemon runtime: pid=%d version=%s protocol=%d endpoint is unavailable", info.PID, displayVersion(info.Version), info.Protocol)
		}
	}

	unlock, err := lock()
	if err != nil {
		return RuntimeInfo{}, err
	}
	defer unlock()

	if info, err := ReadRuntime(); err == nil {
		if EndpointAlive(info.Endpoint) {
			logDaemon(log, "found running daemon after lock: pid=%d version=%s protocol=%d", info.PID, displayVersion(info.Version), info.Protocol)
			if info.Protocol == ProtocolVersion {
				logDaemon(log, "reusing compatible daemon")
				return info, nil
			}
			logDaemon(log, "daemon protocol is incompatible; restarting")
			if err := stopIncompatible(ctx, info); err != nil {
				return RuntimeInfo{}, err
			}
		}
	}
	_ = RemoveRuntime()

	logDaemon(log, "starting daemon (protocol %d)", ProtocolVersion)
	if err := spawn(ctx, configPath); err != nil {
		return RuntimeInfo{}, fmt.Errorf("start sshex service: %w", err)
	}

	deadline := time.Now().Add(startTimeout)
	for time.Now().Before(deadline) {
		if info, err := ReadRuntime(); err == nil && EndpointAlive(info.Endpoint) {
			if info.Protocol == ProtocolVersion {
				logDaemon(log, "daemon ready: pid=%d version=%s protocol=%d", info.PID, displayVersion(info.Version), info.Protocol)
				return info, nil
			}
		}
		if err := wait(ctx, startInterval); err != nil {
			return RuntimeInfo{}, err
		}
	}
	return RuntimeInfo{}, fmt.Errorf("sshex service did not become ready")
}

func logDaemon(log io.Writer, format string, args ...any) {
	if log != nil {
		_, _ = fmt.Fprintf(log, "sshex: verbose: "+format+"\n", args...)
	}
}

func displayVersion(version string) string {
	if version == "" {
		return "unknown"
	}
	return version
}

func stopIncompatible(ctx context.Context, info RuntimeInfo) error {
	if info.PID <= 0 || info.PID == os.Getpid() {
		return fmt.Errorf("running sshex service uses incompatible protocol %d and cannot be safely restarted", info.Protocol)
	}
	if err := signalProcess(info.PID, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
		return fmt.Errorf("stop incompatible sshex service (protocol %d): %w", info.Protocol, err)
	}
	deadline := time.Now().Add(stopTimeout)
	for time.Now().Before(deadline) {
		if !EndpointAlive(info.Endpoint) {
			return RemoveRuntime()
		}
		if err := wait(ctx, startInterval); err != nil {
			return err
		}
	}
	return fmt.Errorf("incompatible sshex service (protocol %d) did not stop", info.Protocol)
}

// Stop terminates a running background service and clears its runtime file.
func Stop(info RuntimeInfo) error {
	if info.PID > 0 {
		_ = syscall.Kill(info.PID, syscall.SIGTERM)
	}
	return RemoveRuntime()
}

// EndpointAlive reports whether the service endpoint accepts connections.
func EndpointAlive(endpoint model.Endpoint) bool {
	var network, address string
	switch endpoint.Kind {
	case model.EndpointUnix:
		network, address = "unix", endpoint.Address
	case model.EndpointTCP:
		network, address = "tcp", endpoint.Address
	default:
		return false
	}
	conn, err := net.DialTimeout(network, address, dialTimeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func lock() (func(), error) {
	dir, err := RuntimeDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, RuntimeDirMode); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, LockFileName), os.O_CREATE|os.O_RDWR, RuntimeFileMode)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

func wait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type RuntimeInfo struct {
	PID      int            `json:"pid"`
	Endpoint model.Endpoint `json:"endpoint"`
	Token    string         `json:"token,omitempty"`
	Protocol int            `json:"protocol,omitempty"`
	Version  string         `json:"version,omitempty"`
}

type Daemon struct {
	config config.Config
}

func New(cfg config.Config) *Daemon {
	return &Daemon{config: cfg}
}

func (d *Daemon) Run(ctx context.Context) error {
	_ = ctx
	return errs.ErrNotImplemented
}

func RuntimeDir() (string, error) {
	return config.RuntimeDir()
}

func RuntimeFilePath() (string, error) {
	dir, err := RuntimeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, RuntimeFileName), nil
}

func WriteRuntime(info RuntimeInfo) error {
	dir, err := RuntimeDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, RuntimeDirMode); err != nil {
		return err
	}
	path := filepath.Join(dir, RuntimeFileName)
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, RuntimeFileMode)
}

func ReadRuntime() (RuntimeInfo, error) {
	path, err := RuntimeFilePath()
	if err != nil {
		return RuntimeInfo{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeInfo{}, err
	}
	var info RuntimeInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return RuntimeInfo{}, err
	}
	return info, nil
}

func RemoveRuntime() error {
	path, err := RuntimeFilePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
