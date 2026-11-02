package logo

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/mcnull/sshex/internal/ui/theme"
)

func TestViewCentersArt(t *testing.T) {
	styles := theme.NewStyles(theme.Load(theme.Options{NoColor: true}))
	got := View(&styles, 60)
	lines := strings.Split(got, "\n")
	if len(lines) != Height {
		t.Fatalf("logo rendered %d lines, want %d", len(lines), Height)
	}
	for i, line := range lines {
		if width := lipgloss.Width(line); width != 60 {
			t.Fatalf("line %d width = %d, want 60", i, width)
		}
	}
	if !strings.Contains(got, "┌──────┐") {
		t.Fatalf("logo art is missing: %q", got)
	}
}
