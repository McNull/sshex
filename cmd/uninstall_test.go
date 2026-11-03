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
		got, err := confirmUninstall(strings.NewReader(tc.input), &out, "example.com", false)
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

func TestConfirmUninstallMentionsConfigOnlyWhenRequested(t *testing.T) {
	var without bytes.Buffer
	if _, err := confirmUninstall(strings.NewReader("n\n"), &without, "example.com", false); err != nil {
		t.Fatalf("confirmUninstall() error: %v", err)
	}
	if strings.Contains(without.String(), "sshex config") {
		t.Fatalf("confirmUninstall() unexpectedly mentions config:\n%s", without.String())
	}

	var with bytes.Buffer
	if _, err := confirmUninstall(strings.NewReader("n\n"), &with, "example.com", true); err != nil {
		t.Fatalf("confirmUninstall() error: %v", err)
	}
	if !strings.Contains(with.String(), "sshex config") {
		t.Fatalf("confirmUninstall() output missing config description:\n%s", with.String())
	}
}

func TestUninstallCommandFlags(t *testing.T) {
	for _, name := range []string{"port", "identity", "option", "yes", "force", "config"} {
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
