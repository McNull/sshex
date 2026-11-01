package cmd

import (
	"os"
	"strings"
	"sync"
)

// recordDemoMode reports whether SSHEX_RECORD_DEMO asks for clean output while
// recording a demo. When enabled, explanatory descriptions and post-edit hints
// are suppressed. The environment is read once and cached for the rest of the
// process.
var recordDemoMode = sync.OnceValue(func() bool {
	return parseDemoMode(os.Getenv("SSHEX_RECORD_DEMO"))
})

// parseDemoMode interprets the SSHEX_RECORD_DEMO value. It accepts the usual
// truthy spellings, ignoring case and surrounding whitespace.
func parseDemoMode(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
