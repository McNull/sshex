package ui

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestRunRequiresTerminal(t *testing.T) {
	err := Run(Options{
		In:  strings.NewReader(""),
		Out: &bytes.Buffer{},
	})
	if !errors.Is(err, ErrNotTerminal) {
		t.Fatalf("Run() error = %v, want ErrNotTerminal", err)
	}
}
