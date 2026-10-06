package cmd

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"
)

func forceTerminal(t *testing.T, terminal bool) {
	t.Helper()
	orig := isTerminalWriter
	isTerminalWriter = func(io.Writer) bool { return terminal }
	t.Cleanup(func() { isTerminalWriter = orig })
}

func TestSpinnerNonTerminalIsSilent(t *testing.T) {
	forceTerminal(t, false)

	var out bytes.Buffer
	sp := startSpinner(&out, "installing remote sshex command...")
	sp.StopWith("✓ done.")

	if out.Len() != 0 {
		t.Fatalf("non-terminal spinner wrote %q, want no output", out.String())
	}
}

func TestSpinnerStopWithReplacesLine(t *testing.T) {
	forceTerminal(t, true)

	var out bytes.Buffer
	sp := startSpinner(&out, "installing remote sshex command...")
	time.Sleep(2 * spinnerInterval)
	sp.StopWith("✓ done.")

	got := out.String()
	if !strings.Contains(got, "installing remote sshex command...") {
		t.Fatalf("spinner never rendered the label: %q", got)
	}
	if !strings.HasSuffix(got, "\r\x1b[K✓ done.\n") {
		t.Fatalf("spinner did not replace the line with the final message: %q", got)
	}
}

func TestSpinnerStopErasesLine(t *testing.T) {
	forceTerminal(t, true)

	var out bytes.Buffer
	sp := startSpinner(&out, "working...")
	time.Sleep(2 * spinnerInterval)
	sp.Stop()

	if !strings.HasSuffix(out.String(), "\r\x1b[K") {
		t.Fatalf("Stop did not erase the line: %q", out.String())
	}
}

func TestSpinnerStopIsIdempotent(t *testing.T) {
	forceTerminal(t, true)

	var out bytes.Buffer
	sp := startSpinner(&out, "working...")
	sp.Stop()
	sp.Stop()
	sp.StopWith("✓ done.")

	if got := out.String(); !strings.HasSuffix(got, "\r\x1b[K") {
		t.Fatalf("repeated Stop changed the output: %q", got)
	}
}
