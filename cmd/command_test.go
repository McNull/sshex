package cmd

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestPromptFieldKeepsEmpty(t *testing.T) {
	var out bytes.Buffer
	value, err := promptField(bufio.NewReader(strings.NewReader("\n")), &out, "name", "code")
	if err != nil {
		t.Fatalf("promptField() error: %v", err)
	}
	if value != "code" {
		t.Fatalf("promptField() = %q, want code", value)
	}
	if !strings.Contains(out.String(), "name [code]") {
		t.Fatalf("prompt = %q", out.String())
	}
}

func TestPromptFieldReplaces(t *testing.T) {
	var out bytes.Buffer
	value, err := promptField(bufio.NewReader(strings.NewReader("editor\n")), &out, "name", "code")
	if err != nil {
		t.Fatalf("promptField() error: %v", err)
	}
	if value != "editor" {
		t.Fatalf("promptField() = %q, want editor", value)
	}
}

func TestPromptBool(t *testing.T) {
	cases := []struct {
		in      string
		current bool
		want    bool
	}{
		{"\n", true, true},
		{"\n", false, false},
		{"yes\n", false, true},
		{"n\n", true, false},
		{"maybe\n", true, true},
	}
	for _, tc := range cases {
		var out bytes.Buffer
		got, err := promptBool(bufio.NewReader(strings.NewReader(tc.in)), &out, "disabled", tc.current)
		if err != nil {
			t.Fatalf("promptBool(%q) error: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("promptBool(%q, %v) = %v, want %v", tc.in, tc.current, got, tc.want)
		}
	}
}

func TestCommandAddHasAliasFlag(t *testing.T) {
	if commandAddCmd.Flags().Lookup("alias") == nil {
		t.Fatal("command add is missing the --alias flag")
	}
	if commandAddCmd.Flags().Lookup("alias").NoOptDefVal != aliasFromName {
		t.Fatal("command add --alias is not optional-valued")
	}
}

// parseAliasAdd parses add arguments through a throwaway command configured like
// commandAddCmd, returning the resolved alias and positional arguments.
func parseAliasAdd(t *testing.T, args ...string) (string, []string) {
	t.Helper()
	cmd := &cobra.Command{Use: "add", Args: cobra.MinimumNArgs(2)}
	cmd.Flags().String("alias", "", "")
	cmd.Flags().Lookup("alias").NoOptDefVal = aliasFromName
	cmd.Flags().SetInterspersed(false)
	var alias string
	var positionals []string
	cmd.RunE = func(c *cobra.Command, a []string) error {
		alias, positionals = resolveAddAlias(c, a)
		return nil
	}
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("parse %v: %v", args, err)
	}
	return alias, positionals
}

func TestResolveAddAlias(t *testing.T) {
	cases := []struct {
		name        string
		args        []string
		wantAlias   string
		wantCommand string
	}{
		{"bare uses name", []string{"--alias", "code", "cmd"}, "code", "cmd"},
		{"explicit space", []string{"--alias", "ed", "code", "cmd"}, "ed", "cmd"},
		{"explicit equals", []string{"--alias=ed", "code", "cmd"}, "ed", "cmd"},
		{"absent", []string{"code", "cmd"}, "", "cmd"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			alias, positionals := parseAliasAdd(t, tc.args...)
			if alias != tc.wantAlias {
				t.Fatalf("alias = %q, want %q", alias, tc.wantAlias)
			}
			if len(positionals) != 2 || positionals[0] != "code" {
				t.Fatalf("positionals = %v, want [code ...]", positionals)
			}
			if strings.Join(positionals[1:], " ") != tc.wantCommand {
				t.Fatalf("command = %q, want %q", strings.Join(positionals[1:], " "), tc.wantCommand)
			}
		})
	}
}
