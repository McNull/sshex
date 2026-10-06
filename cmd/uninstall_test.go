package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mcnull/sshex/internal/sessionfile"
)

func TestConfirmUninstall(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"y\n", true},
		{"Y\n", true},
		{"yes\n", true},
		{" YES \n", true},
		{"n\n", false},
		{"\n", false},
		{"", false},
	}
	for _, tc := range cases {
		var out bytes.Buffer
		got, err := confirmUninstall(strings.NewReader(tc.input), &out, "example.com")
		if err != nil {
			t.Fatalf("confirmUninstall(%q) error: %v", tc.input, err)
		}
		if got != tc.want {
			t.Fatalf("confirmUninstall(%q) = %v, want %v", tc.input, got, tc.want)
		}
		if !strings.Contains(out.String(), "example.com") {
			t.Fatalf("confirmUninstall(%q) output missing target:\n%s", tc.input, out.String())
		}
		if !strings.Contains(out.String(), "Continue? (y/n): ") {
			t.Fatalf("confirmUninstall(%q) output missing prompt:\n%s", tc.input, out.String())
		}
	}
}

func TestUninstallCommandFlags(t *testing.T) {
	for _, name := range []string{"port", "identity", "option", "yes", "force"} {
		if uninstallCmd.Flags().Lookup(name) == nil {
			t.Fatalf("uninstall command missing --%s flag", name)
		}
	}
}

func TestPrintActiveSessions(t *testing.T) {
	var out bytes.Buffer
	printActiveSessions(&out, []sessionfile.File{
		{ID: "abc", Target: "bob@ssh02", Origin: "laptop"},
		{ID: "def"},
	})
	got := out.String()
	for _, want := range []string{"active sshex sessions:", "abc", "bob@ssh02", "from laptop", "def"} {
		if !strings.Contains(got, want) {
			t.Fatalf("printActiveSessions output missing %q:\n%s", want, got)
		}
	}
}
