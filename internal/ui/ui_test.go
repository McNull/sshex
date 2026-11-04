package ui

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/mcnull/sshex/internal/verbose"
)

func TestRunRequiresTerminal(t *testing.T) {
	verbose.Set(true)
	verbose.Suppress(false)
	t.Cleanup(func() {
		verbose.Set(false)
		verbose.Suppress(false)
	})
	err := Run(Options{
		In:  strings.NewReader(""),
		Out: &bytes.Buffer{},
	})
	if !errors.Is(err, ErrNotTerminal) {
		t.Fatalf("Run() error = %v, want ErrNotTerminal", err)
	}
	if !verbose.ShouldLog() {
		t.Fatal("UI run should remain verbose-suppressed after restoring prior state")
	}
}
