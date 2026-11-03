package footer

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/mcnull/sshex/internal/ui/keys"
	"github.com/mcnull/sshex/internal/ui/theme"
)

func TestViewOrdersCommonHintsAndOmitsSecondaryActions(t *testing.T) {
	styles := theme.NewStyles(theme.Load(theme.Options{NoColor: true}))
	footer := New(&styles)
	footer.Resize(100)
	bindings := []keys.Binding{
		binding([]string{"up", "k"}, "↑/k", "up"),
		binding([]string{"down", "j"}, "↓/j", "down"),
		binding([]string{"x"}, "x", "close session"),
		binding([]string{"r"}, "r", "refresh"),
		binding([]string{"esc"}, "esc", "back"),
	}

	view := ansi.Strip(strings.Split(footer.View(bindings), "\n")[1])
	want := "esc back  ·  h help  ·  ↑/↓ navigate  ·  x close session"
	if view != want {
		t.Fatalf("footer = %q, want compact hints %q", view, want)
	}
	for _, hint := range []string{"esc back", "h help", "↑/↓ navigate", "x close session"} {
		if !strings.Contains(view, hint) {
			t.Errorf("footer %q missing %q", view, hint)
		}
	}
	for _, hint := range []string{"↑/k", "↓/j", "r refresh"} {
		if strings.Contains(view, hint) {
			t.Errorf("footer %q should omit %q", view, hint)
		}
	}
}

func TestViewWithoutHelpAndHelpClose(t *testing.T) {
	styles := theme.NewStyles(theme.Load(theme.Options{NoColor: true}))
	footer := New(&styles)
	footer.Resize(80)

	withoutHelp := ansi.Strip(strings.Split(footer.ViewWithoutHelp(nil), "\n")[1])
	if strings.Contains(withoutHelp, "h help") {
		t.Fatalf("footer unexpectedly advertises help while h is text input: %q", withoutHelp)
	}
	closeHelp := ansi.Strip(strings.Split(footer.ViewHelpClose(), "\n")[1])
	if !strings.Contains(closeHelp, "esc close help") || strings.Contains(closeHelp, "h help") {
		t.Fatalf("help footer = %q", closeHelp)
	}
}

func TestViewOrdersRootQuitAfterCommonHints(t *testing.T) {
	styles := theme.NewStyles(theme.Load(theme.Options{NoColor: true}))
	footer := New(&styles)
	footer.Resize(80)
	bindings := []keys.Binding{
		binding([]string{"up", "k"}, "↑/k", "up"),
		binding([]string{"down", "j"}, "↓/j", "down"),
		binding([]string{"enter"}, "enter", "select"),
		binding([]string{"q", "esc"}, "q/esc", "quit"),
	}
	view := ansi.Strip(strings.Split(footer.View(bindings), "\n")[1])
	want := "esc quit  ·  h help  ·  ↑/↓ navigate  ·  enter select"
	if view != want {
		t.Fatalf("footer = %q, want %q", view, want)
	}
}

func binding(keyNames []string, label, description string) keys.Binding {
	return keys.NewBinding(keys.WithKeys(keyNames...), keys.WithHelp(label, description, description+" long help"))
}
