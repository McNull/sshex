package help

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mcnull/sshex/internal/ui/keys"
	"github.com/mcnull/sshex/internal/ui/theme"
)

func TestViewShowsOptionalTextAndCompleteHotkeys(t *testing.T) {
	styles := theme.NewStyles(theme.Load(theme.Options{NoColor: true}))
	panel := New(&styles)
	panel.Resize(64, 18)
	bindings := []keys.Binding{
		keys.NewBinding(keys.WithKeys("up", "k"), keys.WithHelp("↑/k", "up")),
		keys.NewBinding(keys.WithKeys("down", "j"), keys.WithHelp("↓/j", "down")),
		keys.NewBinding(keys.WithKeys("r"), keys.WithHelp("r", "refresh", "Reload the active session list now.")),
		keys.NewBinding(keys.WithKeys("ctrl+c"), keys.WithHelp("ctrl+c", "quit")),
	}

	view := ansi.Strip(panel.View("Sessions", "Active sessions are listed here.", bindings))
	for _, want := range []string{"Sessions help", "Active sessions are listed here.", "r  Reload the active session", "list now.", "ctrl+c  quit", "↑/k  up", "↓/j  down", "Press esc to close help"} {
		if !strings.Contains(view, want) {
			t.Errorf("help overlay missing %q:\n%s", want, view)
		}
	}
	for _, line := range strings.Split(view, "\n") {
		if width := lipgloss.Width(line); width > 64 {
			t.Errorf("overlay line width = %d, want <= 64: %q", width, line)
		}
	}
	if got := len(strings.Split(view, "\n")); got > 18 {
		t.Fatalf("overlay height = %d, want <= 18:\n%s", got, view)
	}
}

func TestViewOmitsEmptyOptionalText(t *testing.T) {
	styles := theme.NewStyles(theme.Load(theme.Options{NoColor: true}))
	panel := New(&styles)
	panel.Resize(48, 16)
	view := ansi.Strip(panel.View("Connect", "  ", nil))
	if strings.Contains(view, "Connect help\n\n\n") {
		t.Fatalf("empty help text left an unnecessary section:\n%s", view)
	}
}

func TestLongHelpFallsBackToShortDescription(t *testing.T) {
	binding := keys.NewBinding(keys.WithKeys("a"), keys.WithHelp("a", "add"))
	if got := binding.LongHelp(); got != "add" {
		t.Fatalf("LongHelp() = %q, want short description fallback", got)
	}
}

func TestLongHelpIsShownInOverlay(t *testing.T) {
	styles := theme.NewStyles(theme.Load(theme.Options{NoColor: true}))
	panel := New(&styles)
	panel.Resize(64, 18)
	binding := keys.NewBinding(
		keys.WithKeys("a"),
		keys.WithHelp("a", "add", "Open the form to create a new command."),
	)
	view := ansi.Strip(panel.View("Commands", "", []keys.Binding{binding}))
	if !strings.Contains(view, "a  Open the form to create a new command.") {
		t.Fatalf("help overlay omitted long help description:\n%s", view)
	}
}
