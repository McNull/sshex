// Package verbose holds the process-wide diagnostic setting.
package verbose

import (
	"os"
	"sync/atomic"
)

const EnvironmentVariable = "SSHEX_VERBOSE"

var enabled atomic.Bool
var suppressed atomic.Bool

func init() { enabled.Store(os.Getenv(EnvironmentVariable) == "1") }

// Set enables or disables diagnostic output for the current process.
func Set(value bool) { enabled.Store(value) }

// Enabled reports whether diagnostic output is enabled.
func Enabled() bool { return enabled.Load() || os.Getenv(EnvironmentVariable) == "1" }

// Suppress prevents diagnostics from writing while the full-screen UI owns the
// terminal, without disabling the flag or preventing child processes inheriting it.
func Suppress(value bool) { suppressed.Store(value) }

// ShouldLog reports whether diagnostic output should be written now.
func ShouldLog() bool { return Enabled() && !suppressed.Load() }

// Environment returns child-process environment with verbose mode propagated.
func Environment() []string {
	env := os.Environ()
	if !Enabled() {
		return env
	}
	filtered := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if len(entry) >= len(EnvironmentVariable)+1 && entry[:len(EnvironmentVariable)+1] == EnvironmentVariable+"=" {
			continue
		}
		filtered = append(filtered, entry)
	}
	return append(filtered, EnvironmentVariable+"=1")
}

// SetEnvironment propagates the current state to child processes.
func SetEnvironment() error {
	if Enabled() {
		return os.Setenv(EnvironmentVariable, "1")
	}
	return nil
}
