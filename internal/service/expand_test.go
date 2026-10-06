package service

import (
	"strconv"
	"strings"
	"testing"
)

func TestRenderCommand(t *testing.T) {
	env := []string{"REMOTE_CWD=/home/bob", "REMOTE_USER=bob", "REMOTE_HOST=ssh01"}
	tenArgs := make([]string, 10)
	for i := range tenArgs {
		tenArgs[i] = strconv.Itoa(i + 1)
	}

	cases := []struct {
		name     string
		template string
		args     []string
		want     string
	}{
		{name: "braced", template: `code --folder-uri "vscode-remote://ssh-remote+${REMOTE_HOST}"`, want: `code --folder-uri "vscode-remote://ssh-remote+ssh01"`},
		{name: "unbraced", template: "echo $REMOTE_USER", want: "echo bob"},
		{name: "default cwd", template: `zed "ssh://${REMOTE_USER}@${REMOTE_HOST}${@:-${REMOTE_CWD}}"`, want: `zed "ssh://bob@ssh01/home/bob"`},
		{name: "override", template: `zed "ssh://${REMOTE_USER}@${REMOTE_HOST}${@:-${REMOTE_CWD}}"`, args: []string{"/srv/app"}, want: `zed "ssh://bob@ssh01/srv/app"`},
		{name: "positional", template: "cmd $0 $1 $2", args: []string{"a", "b"}, want: "cmd code a b"},
		{name: "positional braced", template: "cmd ${10}", args: tenArgs, want: "cmd 10"},
		{name: "all args", template: "echo $@", args: []string{"a", "b"}, want: "echo a b"},
		{name: "count", template: "echo $#", args: []string{"a", "b"}, want: "echo 2"},
		{name: "unset default", template: "echo ${NOPE:-fallback}", want: "echo fallback"},
		{name: "nested default", template: "echo ${NOPE:-${REMOTE_USER}}", want: "echo bob"},
		{name: "unknown empty", template: "echo ${NOPE}", want: "echo "},
		{name: "trailing dollar", template: "echo $", want: "echo $"},
		{name: "no variables", template: "echo hello", want: "echo hello"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderCommand(tc.template, env, "code", tc.args)
			if got != tc.want {
				t.Fatalf("RenderCommand(%q) = %q, want %q", tc.template, got, tc.want)
			}
		})
	}
}

func TestRenderCommandMultiDigitDefault(t *testing.T) {
	// ${@:-...} with many args joins them all.
	args := []string{"a", "b", "c"}
	got := RenderCommand(`run "${@:-fallback}"`, nil, "code", args)
	if got != `run "a b c"` {
		t.Fatalf("RenderCommand() = %q", got)
	}
	if !strings.Contains(RenderCommand("${#}", nil, "code", args), "3") {
		t.Fatal("RenderCommand() did not expand ${#}")
	}
}
