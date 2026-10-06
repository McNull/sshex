package local

import (
	"bytes"
	"context"
	"testing"
)

func TestExecutorExpansion(t *testing.T) {
	env := []string{"REMOTE_CWD=/remote", "REMOTE_USER=bob"}
	cases := []struct {
		name   string
		script string
		args   []string
		want   string
	}{
		{name: "positional", script: `echo "$0|$1|$@"`, args: []string{"a", "b"}, want: "cmd|a|a b\n"},
		{name: "default", script: `echo "${@:-${REMOTE_CWD}}"`, want: "/remote\n"},
		{name: "override", script: `echo "${@:-${REMOTE_CWD}}"`, args: []string{"/p"}, want: "/p\n"},
		{name: "env", script: `echo "$REMOTE_USER"`, want: "bob\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			code, err := NewExecutor().Run(context.Background(), tc.script, "cmd", tc.args, env, &out, nil)
			if err != nil {
				t.Fatalf("Run() error: %v", err)
			}
			if code != 0 {
				t.Fatalf("Run() code = %d, want 0", code)
			}
			if out.String() != tc.want {
				t.Fatalf("Run() stdout = %q, want %q", out.String(), tc.want)
			}
		})
	}
}

func TestExecutorExitCode(t *testing.T) {
	code, err := NewExecutor().Run(context.Background(), "exit 7", "cmd", nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if code != 7 {
		t.Fatalf("Run() code = %d, want 7", code)
	}
}

func TestExecutorStderr(t *testing.T) {
	var stderr bytes.Buffer
	if _, err := NewExecutor().Run(context.Background(), "echo oops >&2", "cmd", nil, nil, nil, &stderr); err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if stderr.String() != "oops\n" {
		t.Fatalf("Run() stderr = %q, want %q", stderr.String(), "oops\n")
	}
}
