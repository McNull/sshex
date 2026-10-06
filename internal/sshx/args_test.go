package sshx

import (
	"reflect"
	"testing"
)

func TestSplitArgs(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{name: "empty", in: "", want: nil},
		{name: "whitespace only", in: "   \t\n", want: nil},
		{name: "single token", in: "StrictHostKeyChecking=no", want: []string{"StrictHostKeyChecking=no"}},
		{name: "flags and value", in: "-J nulls-probe", want: []string{"-J", "nulls-probe"}},
		{name: "repeated flags", in: "-o A=1 -o B=2", want: []string{"-o", "A=1", "-o", "B=2"}},
		{name: "double quotes preserve spaces", in: `-o "ProxyCommand=ssh -W %h:%p bastion"`, want: []string{"-o", "ProxyCommand=ssh -W %h:%p bastion"}},
		{name: "single quotes are literal", in: `-o 'A=1 2'`, want: []string{"-o", "A=1 2"}},
		{name: "backslash escapes space", in: `A=1\ 2`, want: []string{"A=1 2"}},
		{name: "backslash escapes quote", in: `A=\"1\"`, want: []string{`A="1"`}},
		{name: "empty quoted token", in: `""`, want: []string{""}},
		{name: "adjacent quoted segments", in: `a"b c"d`, want: []string{"ab cd"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SplitArgs(tc.in)
			if err != nil {
				t.Fatalf("SplitArgs(%q) error: %v", tc.in, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("SplitArgs(%q) = %#v; want %#v", tc.in, got, tc.want)
			}
		})
	}
}

func TestSplitArgsErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{name: "unterminated single quote", in: `-o 'A=1`},
		{name: "unterminated double quote", in: `-o "A=1`},
		{name: "trailing backslash", in: `A=1\`},
		{name: "trailing backslash in double quotes", in: `"A=1\`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := SplitArgs(tc.in); err == nil {
				t.Fatalf("SplitArgs(%q) error = nil, want error", tc.in)
			}
		})
	}
}
