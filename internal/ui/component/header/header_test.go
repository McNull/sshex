package header

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mcnull/sshex/internal/ui/theme"
)

func TestViewRightAlignsIdentity(t *testing.T) {
	styles := theme.NewStyles(theme.Load(theme.Options{NoColor: true}))
	m := New(&styles, "0.4.6", "broker-host", "alice@ssh01")
	m.Resize(80)

	line := strings.Split(m.View(), "\n")[0]
	plain := ansi.Strip(line)
	if !strings.HasSuffix(plain, "client: alice@ssh01 origin: broker-host") {
		t.Fatalf("header line = %q, want right-aligned identity", plain)
	}
	if strings.Contains(plain, "[") || strings.Contains(plain, "]") {
		t.Fatalf("header line = %q, brackets should not be rendered", plain)
	}
	if got := lipgloss.Width(line); got != 80 {
		t.Fatalf("header line width = %d, want 80", got)
	}
	if !strings.HasPrefix(ansi.Strip(line), "sshex: v0.4.6") {
		t.Fatalf("header line = %q, want sshex version at left", ansi.Strip(line))
	}
	if !strings.Contains(line, styles.Brand.Render("ssh")+styles.BrandAccent.Render("ex")) {
		t.Fatal("sshex wordmark is not rendered with the brand style")
	}
	if !strings.Contains(line, styles.Version.Render("v0.4.6")) {
		t.Fatal("version is not rendered with the muted version style")
	}
	if !strings.Contains(line, styles.HeaderIdentity.Render("alice@ssh01")) {
		t.Fatal("client hostname is not rendered with the identity style")
	}
	if !strings.Contains(line, styles.HeaderIdentity.Render("broker-host")) {
		t.Fatal("origin hostname is not rendered with the identity style")
	}
}

func TestViewCentersTitle(t *testing.T) {
	styles := theme.NewStyles(theme.Load(theme.Options{NoColor: true}))
	m := New(&styles, "0.4.6", "", "")
	m.SetTitle("Sessions")
	m.Resize(80)

	line := strings.Split(m.View(), "\n")[0]
	plain := ansi.Strip(line)
	wantStart := (80 - len("Sessions")) / 2
	if got := strings.Index(plain, "Sessions"); got != wantStart {
		t.Fatalf("title starts at %d, want %d: %q", got, wantStart, plain)
	}
	if got := lipgloss.Width(line); got != 80 {
		t.Fatalf("header line width = %d, want 80", got)
	}
	if !strings.Contains(line, styles.HeaderTitle.Render("Sessions")) {
		t.Fatal("title is not rendered with the header title style")
	}
}

func TestViewTruncatesTitleAndIdentityToFit(t *testing.T) {
	styles := theme.NewStyles(theme.Load(theme.Options{NoColor: true}))
	m := New(&styles, "0.4.6", "a-very-long-broker-hostname", "alice@a-very-long-remote-hostname")
	m.SetTitle("Select session")
	m.Resize(48)

	line := strings.Split(m.View(), "\n")[0]
	if got := lipgloss.Width(line); got != 48 {
		t.Fatalf("header line width = %d, want 48: %q", got, line)
	}
	if !strings.Contains(ansi.Strip(line), "Select session") {
		t.Fatalf("header line = %q, want title", ansi.Strip(line))
	}
	if !strings.Contains(line, "…") {
		t.Fatalf("header line = %q, want truncated identity", line)
	}
}

func TestViewTruncatesIdentityToFit(t *testing.T) {
	styles := theme.NewStyles(theme.Load(theme.Options{NoColor: true}))
	m := New(&styles, "0.4.6", "a-very-long-broker-hostname", "alice@a-very-long-remote-hostname")
	m.Resize(48)

	line := strings.Split(m.View(), "\n")[0]
	if got := lipgloss.Width(line); got > 48 {
		t.Fatalf("header line width = %d, want <= 48: %q", got, line)
	}
	if !strings.Contains(line, "…") {
		t.Fatalf("header line = %q, want truncated identity", line)
	}
}

func TestVersionLabelTrimsDevelopmentSuffix(t *testing.T) {
	got := versionLabel("v0.4.6-0.20261103081212-ab52028b64f5+dirty")
	if got != "v0.4.6" {
		t.Fatalf("versionLabel() = %q, want v0.4.6", got)
	}
}
