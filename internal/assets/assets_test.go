package assets

import (
	"strings"
	"testing"
)

func TestVersion(t *testing.T) {
	v := Version()
	if v == "" {
		t.Fatal("Version() is empty")
	}
	if strings.TrimSpace(v) != v {
		t.Fatalf("Version() = %q, want trimmed", v)
	}
}

func TestVersionInjected(t *testing.T) {
	orig := version
	version = "v1.2.3"
	t.Cleanup(func() { version = orig })

	if got := Version(); got != "1.2.3" {
		t.Fatalf("Version() = %q, want %q", got, "1.2.3")
	}
}

func TestBuildVersionIgnoresDevel(t *testing.T) {
	if got := buildVersion(); got == "(devel)" {
		t.Fatalf("buildVersion() = %q, want empty for local builds", got)
	}
}

func TestLogo(t *testing.T) {
	logo := Logo()
	if logo == "" {
		t.Fatal("Logo() is empty")
	}
	if strings.Contains(logo, `\033`) {
		t.Error("Logo() still contains literal \\033 escape text")
	}
	if !strings.Contains(logo, "\x1b[") {
		t.Error("Logo() does not contain ANSI escape sequences")
	}
	if strings.Contains(logo, "{VERSION}") {
		t.Error("Logo() still contains the {VERSION} placeholder")
	}
	if !strings.Contains(logo, "v"+Version()) {
		t.Errorf("Logo() does not contain version %q", "v"+Version())
	}
}
