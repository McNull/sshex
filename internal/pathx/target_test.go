package pathx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveTarget(t *testing.T) {
	const cwd = "/home/bob/project"
	const home = "/home/bob"

	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "no args uses cwd", want: cwd},
		{name: "absolute", args: []string{"/srv/app"}, want: "/srv/app"},
		{name: "explicit relative", args: []string{"./cmd"}, want: cwd + "/cmd"},
		{name: "bare relative", args: []string{"cmd"}, want: cwd + "/cmd"},
		{name: "home", args: []string{"~/.ssh"}, want: home + "/.ssh"},
		{name: "flag unchanged", args: []string{"--new-window"}, want: "--new-window"},
		{name: "first arg wins", args: []string{"a", "b"}, want: cwd + "/a"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveTarget(cwd, home, tc.args); got != tc.want {
				t.Fatalf("ResolveTarget(%q, %q, %v) = %q, want %q", cwd, home, tc.args, got, tc.want)
			}
		})
	}
}

func TestResolveTargetUnknownCwd(t *testing.T) {
	if got := ResolveTarget("", "/home/bob", []string{"cmd"}); got != "cmd" {
		t.Fatalf("ResolveTarget() = %q, want cmd", got)
	}
}

func TestClassifyTarget(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	cases := []struct {
		name string
		path string
		want string
	}{
		{name: "folder", path: dir, want: KindFolder},
		{name: "file", path: file, want: KindFile},
		{name: "missing", path: filepath.Join(dir, "missing"), want: KindFile},
		{name: "empty", path: "", want: KindFile},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyTarget(tc.path); got != tc.want {
				t.Fatalf("ClassifyTarget(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}
