package pathx

import (
	"reflect"
	"testing"
)

func TestExpand(t *testing.T) {
	const cwd = "/home/null"
	const home = "/home/null"

	cases := []struct {
		name string
		arg  string
		want string
	}{
		{name: "dot", arg: ".", want: "/home/null"},
		{name: "dotdot", arg: "..", want: "/home"},
		{name: "dot slash", arg: "./.ssh", want: "/home/null/.ssh"},
		{name: "dotdot slash", arg: "../other", want: "/home/other"},
		{name: "home", arg: "~", want: "/home/null"},
		{name: "home slash", arg: "~/.ssh", want: "/home/null/.ssh"},
		{name: "absolute", arg: "/srv/app", want: "/srv/app"},
		{name: "flag", arg: "--new-window", want: "--new-window"},
		{name: "short flag", arg: "-n", want: "-n"},
		{name: "bare name", arg: "foo", want: "foo"},
		{name: "bare path", arg: "a/b", want: "a/b"},
		{name: "dotfile", arg: ".ssh", want: ".ssh"},
		{name: "embedded tilde", arg: "a~b", want: "a~b"},
		{name: "empty", arg: "", want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Expand(cwd, home, tc.arg); got != tc.want {
				t.Fatalf("Expand(%q, %q, %q) = %q, want %q", cwd, home, tc.arg, got, tc.want)
			}
		})
	}
}

func TestExpandUnknownBase(t *testing.T) {
	cases := []struct {
		name string
		cwd  string
		home string
		arg  string
		want string
	}{
		{name: "no cwd", cwd: "", home: "/home/null", arg: "./.ssh", want: "./.ssh"},
		{name: "no home", cwd: "/home/null", home: "", arg: "~/.ssh", want: "~/.ssh"},
		{name: "no home bare tilde", cwd: "/home/null", home: "", arg: "~", want: "~"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Expand(tc.cwd, tc.home, tc.arg); got != tc.want {
				t.Fatalf("Expand(%q, %q, %q) = %q, want %q", tc.cwd, tc.home, tc.arg, got, tc.want)
			}
		})
	}
}

func TestExpandArgs(t *testing.T) {
	args := []string{"./.ssh", "--new-window", "/srv/app", "~/.config", "foo"}
	want := []string{"/home/null/.ssh", "--new-window", "/srv/app", "/home/null/.config", "foo"}
	got := ExpandArgs("/home/null", "/home/null", args)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExpandArgs() = %v, want %v", got, want)
	}
}

func TestExpandArgsEmpty(t *testing.T) {
	if got := ExpandArgs("/home/null", "/home/null", nil); got != nil {
		t.Fatalf("ExpandArgs(nil) = %v, want nil", got)
	}
}
