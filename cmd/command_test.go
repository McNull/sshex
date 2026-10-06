package cmd

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/mcnull/sshex/internal/model"
	"github.com/spf13/cobra"
)

func TestPromptFieldRejectsEmptyRequired(t *testing.T) {
	var out, errOut bytes.Buffer
	p := newPrompter(strings.NewReader("\ncode\n"), &out, &errOut)
	validate := func(value string) error {
		if value == "" {
			return errors.New("name is required")
		}
		return nil
	}
	value, err := p.field("name", "The name used by sshex exec.", "code", true, validate)
	if err != nil {
		t.Fatalf("field() error: %v", err)
	}
	if value != "code" {
		t.Fatalf("field() = %q, want code", value)
	}
	if !strings.Contains(errOut.String(), "name is required") {
		t.Fatalf("errOut = %q, want validation message", errOut.String())
	}
	if n := strings.Count(out.String(), "# The name used by sshex exec."); n != 1 {
		t.Fatalf("description printed %d times, want 1:\n%s", n, out.String())
	}
	if !strings.Contains(out.String(), "name [code]") {
		t.Fatalf("prompt = %q", out.String())
	}
}

func TestPromptFieldOptionalBlankClears(t *testing.T) {
	var out bytes.Buffer
	p := newPrompter(strings.NewReader("\n"), &out, io.Discard)
	value, err := p.field("alias", "Optional shell alias.", "ed", false, func(string) error {
		t.Fatal("validator called for a cleared optional field")
		return nil
	})
	if err != nil {
		t.Fatalf("field() error: %v", err)
	}
	if value != "" {
		t.Fatalf("field() = %q, want empty", value)
	}
}

func TestPromptFieldRepromptsOnValidationError(t *testing.T) {
	var out, errOut bytes.Buffer
	p := newPrompter(strings.NewReader("taken\nfree\n"), &out, &errOut)
	validate := func(value string) error {
		if value == "taken" {
			return errors.New(`name "taken" is already in use`)
		}
		return nil
	}
	value, err := p.field("name", "The name used by sshex exec.", "code", true, validate)
	if err != nil {
		t.Fatalf("field() error: %v", err)
	}
	if value != "free" {
		t.Fatalf("field() = %q, want free", value)
	}
	if !strings.Contains(errOut.String(), "already in use") {
		t.Fatalf("errOut = %q, want conflict message", errOut.String())
	}
	if n := strings.Count(out.String(), "# The name used by sshex exec."); n != 1 {
		t.Fatalf("description printed %d times, want 1:\n%s", n, out.String())
	}
}

func TestDimmed(t *testing.T) {
	if got := dimmed("# note", false); got != "# note" {
		t.Fatalf("dimmed(false) = %q, want plain", got)
	}
	if got, want := dimmed("# note", true), dimStart+"# note"+dimEnd; got != want {
		t.Fatalf("dimmed(true) = %q, want %q", got, want)
	}
}

func TestPromptDescriptionPlainWithoutTerminal(t *testing.T) {
	var out bytes.Buffer
	p := newPrompter(strings.NewReader("code\n"), &out, io.Discard)
	if _, err := p.field("name", "The name used by sshex exec.", "code", true, func(string) error { return nil }); err != nil {
		t.Fatalf("field() error: %v", err)
	}
	if strings.Contains(out.String(), "\x1b[") {
		t.Fatalf("description contains ANSI escapes for non-terminal writer: %q", out.String())
	}
}

func TestPromptFieldReplaces(t *testing.T) {
	var out bytes.Buffer
	p := newPrompter(strings.NewReader("editor\n"), &out, io.Discard)
	value, err := p.field("name", "The name used by sshex exec.", "code", true, func(string) error { return nil })
	if err != nil {
		t.Fatalf("field() error: %v", err)
	}
	if value != "editor" {
		t.Fatalf("field() = %q, want editor", value)
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
		p := newPrompter(strings.NewReader(tc.in), &out, io.Discard)
		got, err := p.boolean("disabled", "A disabled command is listed but cannot be executed.", tc.current)
		if err != nil {
			t.Fatalf("boolean(%q) error: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("boolean(%q, %v) = %v, want %v", tc.in, tc.current, got, tc.want)
		}
	}
}

func TestCommandVariableLegendIsSingleSource(t *testing.T) {
	for _, text := range []string{execHelp, commandAddHelp, commandFieldHelp} {
		if !strings.Contains(text, "Variables available in a command template:") {
			t.Fatalf("text is missing the variable legend:\n%s", text)
		}
		for _, v := range commandVariables {
			if n := strings.Count(text, v.desc); n != 1 {
				t.Fatalf("variable %q appears %d times, want 1, in:\n%s", v.name, n, text)
			}
		}
	}
}

func TestNewPrompterFallsBackWithoutTerminal(t *testing.T) {
	p := newPrompter(strings.NewReader(""), io.Discard, io.Discard)
	if _, ok := p.editor.(*plainEditor); !ok {
		t.Fatalf("editor = %T, want *plainEditor", p.editor)
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

func TestRequireCommand(t *testing.T) {
	if err := requireCommand(""); err == nil {
		t.Fatal("requireCommand(empty) = nil, want error")
	}
	if err := requireCommand("code"); err != nil {
		t.Fatalf("requireCommand(code) error = %v", err)
	}
}

func TestCommandNameValidator(t *testing.T) {
	commands := []model.Command{{Name: "code", Alias: "ed"}, {Name: "zed"}}
	validate := commandNameValidator(commands, "code")

	if err := validate(""); err == nil {
		t.Fatal("empty name should be rejected")
	}
	if err := validate("bad name"); err == nil {
		t.Fatal("invalid name should be rejected")
	}
	if err := validate("code"); err != nil {
		t.Fatalf("keeping own name rejected: %v", err)
	}
	if err := validate("zed"); err == nil {
		t.Fatal("duplicate name should be rejected")
	}
	if err := validate("editor"); err != nil {
		t.Fatalf("fresh name rejected: %v", err)
	}
}

func TestCommandAliasValidator(t *testing.T) {
	commands := []model.Command{{Name: "code", Alias: "ed"}, {Name: "zed", Alias: "zd"}}
	validate := commandAliasValidator(commands, "code")

	if err := validate("bad alias"); err == nil {
		t.Fatal("invalid alias should be rejected")
	}
	if err := validate("ed"); err != nil {
		t.Fatalf("keeping own alias rejected: %v", err)
	}
	if err := validate("zd"); err == nil {
		t.Fatal("duplicate alias should be rejected")
	}
	if err := validate("x"); err != nil {
		t.Fatalf("fresh alias rejected: %v", err)
	}
}
