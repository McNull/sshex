package local

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunnerRun(t *testing.T) {
	var out bytes.Buffer
	if err := NewRunner().Run(context.Background(), "printf hello", nil, &out, nil); err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if out.String() != "hello" {
		t.Fatalf("Run() stdout = %q, want %q", out.String(), "hello")
	}
}

func TestRunnerRunError(t *testing.T) {
	err := NewRunner().Run(context.Background(), "exit 3", nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "local command failed") {
		t.Fatalf("Run() error = %v, want failure", err)
	}
}
