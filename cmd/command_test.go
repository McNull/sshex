package cmd

import (
	"bytes"
	"context"
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

func TestCommandExamplesAreSingleSource(t *testing.T) {
	for _, e := range commandExamples {
		for _, text := range []string{commandAddHelp, commandFieldHelp} {
			if n := strings.Count(text, e.desc); n != 1 {
				t.Fatalf("example description %q appears %d times, want 1, in:\n%s", e.desc, n, text)
			}
			if n := strings.Count(text, e.command); n != 1 {
				t.Fatalf("example command %q appears %d times, want 1, in:\n%s", e.command, n, text)
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

func TestParseDemoMode(t *testing.T) {
	cases := []struct {
		value string
		want  bool
	}{
		{"1", true},
		{"true", true},
		{"TRUE", true},
		{" yes ", true},
		{"On", true},
		{"0", false},
		{"false", false},
		{"no", false},
		{"off", false},
		{"", false},
		{"maybe", false},
	}
	for _, tc := range cases {
		if got := parseDemoMode(tc.value); got != tc.want {
			t.Fatalf("parseDemoMode(%q) = %v, want %v", tc.value, got, tc.want)
		}
	}
}

func TestQuietPrompterSuppressesDescriptions(t *testing.T) {
	var out bytes.Buffer
	p := newPrompter(strings.NewReader("code\n"), &out, io.Discard)
	p.quiet = true
	if _, err := p.field("name", "The name used by sshex exec.", "", true, func(string) error { return nil }); err != nil {
		t.Fatalf("field() error: %v", err)
	}
	p.separate()
	got := out.String()
	if strings.Contains(got, "#") {
		t.Fatalf("quiet output contains a description:\n%s", got)
	}
	if strings.Contains(got, "\n\n") {
		t.Fatalf("quiet output contains a blank separator:\n%s", got)
	}
	if !strings.Contains(got, "name []") {
		t.Fatalf("quiet output missing prompt:\n%s", got)
	}
}

func TestCommandListHasShortFlag(t *testing.T) {
	flag := commandListCmd.Flags().Lookup("short")
	if flag == nil {
		t.Fatal("command list is missing the --short flag")
	}
	if flag.Shorthand != "s" {
		t.Fatalf("command list --short shorthand = %q, want s", flag.Shorthand)
	}
}

func TestWriteCommandTable(t *testing.T) {
	commands := []model.Command{
		{Name: "code", Command: "code ${REMOTE_HOST}", Alias: "ed"},
		{Name: "logs", Command: "tail -f /var/log/syslog", Alias: "lg", Disabled: true},
	}

	cases := []struct {
		name      string
		short     bool
		wantHead  string
		wantCell  []string
		forbidden string
	}{
		{
			name:      "full",
			short:     false,
			wantHead:  "NAME",
			wantCell:  []string{"COMMAND", "code ${REMOTE_HOST}", "tail -f /var/log/syslog"},
			forbidden: "",
		},
		{
			name:      "short",
			short:     true,
			wantHead:  "NAME",
			wantCell:  []string{"ALIAS", "DISABLED", "ed", "lg"},
			forbidden: "COMMAND",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := writeCommandTable(&out, commands, tc.short); err != nil {
				t.Fatalf("writeCommandTable() error: %v", err)
			}
			got := out.String()
			if !strings.Contains(got, tc.wantHead) {
				t.Fatalf("output missing %q:\n%s", tc.wantHead, got)
			}
			for _, cell := range tc.wantCell {
				if !strings.Contains(got, cell) {
					t.Fatalf("output missing %q:\n%s", cell, got)
				}
			}
			if tc.forbidden != "" && strings.Contains(got, tc.forbidden) {
				t.Fatalf("output should not contain %q:\n%s", tc.forbidden, got)
			}
		})
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

func TestCommandAddArgs(t *testing.T) {
	cases := []struct {
		args    []string
		wantErr bool
	}{
		{nil, false},
		{[]string{"name", "cmd"}, false},
		{[]string{"name", "cmd", "arg"}, false},
		{[]string{"name"}, true},
	}
	for _, tc := range cases {
		err := commandAddCmd.Args(commandAddCmd, tc.args)
		if (err != nil) != tc.wantErr {
			t.Fatalf("Args(%v) error = %v, wantErr %v", tc.args, err, tc.wantErr)
		}
	}
}

// newPromptCommand builds a throwaway command whose prompts read from in.
func newPromptCommand(in string) (*cobra.Command, *bytes.Buffer) {
	out := &bytes.Buffer{}
	cmd := &cobra.Command{Use: "add"}
	cmd.SetIn(strings.NewReader(in))
	cmd.SetOut(out)
	cmd.SetErr(io.Discard)
	return cmd, out
}

func TestPromptCommandFields(t *testing.T) {
	cmd, _ := newPromptCommand("demo\nmy cmd\n\n\n")
	got, err := promptCommandFields(cmd, commandEditorOptions{}, nil)
	if err != nil {
		t.Fatalf("promptCommandFields() error: %v", err)
	}
	want := model.Command{Name: "demo", Command: "my cmd"}
	if got != want {
		t.Fatalf("promptCommandFields() = %+v, want %+v", got, want)
	}
}

func TestPromptCommandFieldsAliasFromName(t *testing.T) {
	cmd, out := newPromptCommand("demo\nmy cmd\n\n\n")
	if _, err := promptCommandFields(cmd, commandEditorOptions{aliasFromName: true}, nil); err != nil {
		t.Fatalf("promptCommandFields() error: %v", err)
	}
	if !strings.Contains(out.String(), "alias [demo]") {
		t.Fatalf("alias prompt does not default to the name:\n%s", out.String())
	}
}

func TestPromptCommandFieldsPrefills(t *testing.T) {
	cmd, out := newPromptCommand("code\ncode ${REMOTE_HOST}\ned\nyes\n")
	initial := model.Command{Name: "code", Command: "code ${REMOTE_HOST}", Alias: "ed", Disabled: true}
	if _, err := promptCommandFields(cmd, commandEditorOptions{command: initial}, []model.Command{initial}); err != nil {
		t.Fatalf("promptCommandFields() error: %v", err)
	}
	for _, want := range []string{"name [code]", "command [code ${REMOTE_HOST}]", "alias [ed]", "disabled (yes/no) [yes]"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("prompt %q missing from:\n%s", want, out.String())
		}
	}
}

func TestPromptCancelled(t *testing.T) {
	var out bytes.Buffer
	cmd := &cobra.Command{Use: "add"}
	cmd.SetOut(&out)
	if err := promptCancelled(cmd, errEditCancelled, "add"); err != nil {
		t.Fatalf("promptCancelled() error: %v", err)
	}
	if !strings.Contains(out.String(), "add cancelled") {
		t.Fatalf("output = %q, want cancellation message", out.String())
	}
	wantErr := errors.New("boom")
	if err := promptCancelled(cmd, wantErr, "add"); !errors.Is(err, wantErr) {
		t.Fatalf("promptCancelled() = %v, want %v", err, wantErr)
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

func TestPrintCommandHints(t *testing.T) {
	cases := []struct {
		name         string
		aliasChanged bool
		quiet        bool
	}{
		{"with alias change", true, false},
		{"without alias change", false, false},
		{"quiet", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			printCommandHints(&out, "code", tc.aliasChanged, tc.quiet)
			if tc.quiet {
				if out.Len() != 0 {
					t.Fatalf("quiet output = %q, want empty", out.String())
				}
				return
			}
			if !strings.Contains(out.String(), "Command can be executed with `sshex exec code`") {
				t.Fatalf("output missing exec hint: %q", out.String())
			}
			restart := strings.Contains(out.String(), "Restart sshex sessions for the alias to take effect")
			if restart != tc.aliasChanged {
				t.Fatalf("restart hint present = %v, want %v: %q", restart, tc.aliasChanged, out.String())
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

func TestRemoteCommandsReadOnly(t *testing.T) {
	manager := remoteCommands{canManage: false}
	if manager.CanManage() {
		t.Fatal("remoteCommands without the commands capability reports manageable")
	}
	ctx := context.Background()
	if err := manager.Add(ctx, "code", "code", "", false); !errors.Is(err, errCommandReadOnly) {
		t.Fatalf("Add() error = %v, want errCommandReadOnly", err)
	}
	if err := manager.Update(ctx, "code", model.Command{Name: "code"}); !errors.Is(err, errCommandReadOnly) {
		t.Fatalf("Update() error = %v, want errCommandReadOnly", err)
	}
	if err := manager.Remove(ctx, "code"); !errors.Is(err, errCommandReadOnly) {
		t.Fatalf("Remove() error = %v, want errCommandReadOnly", err)
	}
	if err := manager.SetDisabled(ctx, "code", true); !errors.Is(err, errCommandReadOnly) {
		t.Fatalf("SetDisabled() error = %v, want errCommandReadOnly", err)
	}
	if err := requireCommandEditable(manager); !errors.Is(err, errCommandReadOnly) {
		t.Fatalf("requireCommandEditable() error = %v, want errCommandReadOnly", err)
	}
}

func TestLocalCommandsManageable(t *testing.T) {
	if !(localCommands{}).CanManage() {
		t.Fatal("localCommands reports not manageable")
	}
	if err := requireCommandEditable(localCommands{}); err != nil {
		t.Fatalf("requireCommandEditable(localCommands) error = %v", err)
	}
}
