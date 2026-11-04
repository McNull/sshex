package theme

import (
	"fmt"
	"slices"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestLoadFallsBackToDefault(t *testing.T) {
	got := Load(Options{Name: "does-not-exist", HasDarkBackground: true})
	if got.Name != DefaultName {
		t.Fatalf("theme name = %q, want %q", got.Name, DefaultName)
	}
}

func TestLoadNoColorDropsTokens(t *testing.T) {
	got := Load(Options{Name: DefaultName, HasDarkBackground: true, NoColor: true})
	if !got.NoColor {
		t.Fatal("theme should be marked NoColor")
	}
	if got.Tokens.Foreground != nil || got.Tokens.Selection != nil {
		t.Fatal("NoColor theme should not define colors")
	}
	styles := NewStyles(got)
	if !styles.MenuRowSelected.GetReverse() {
		t.Fatal("NoColor selection should use reverse video")
	}
}

func TestListSelectionFocusStyles(t *testing.T) {
	dark := NewStyles(Load(Options{Name: DefaultName, HasDarkBackground: true}))
	if fmt.Sprint(dark.ListRowSelected.GetBackground()) == fmt.Sprint(dark.ListRowSelectedBlurred.GetBackground()) {
		t.Fatal("focused list selection should use a different background")
	}

	noColor := NewStyles(Load(Options{Name: DefaultName, NoColor: true}))
	if !noColor.ListRowSelected.GetBold() || noColor.ListRowSelectedBlurred.GetBold() {
		t.Fatal("only the focused list selection should be bold in no-color mode")
	}
	if !noColor.ListRowSelected.GetReverse() || !noColor.ListRowSelectedBlurred.GetReverse() {
		t.Fatal("both list selection states should use reverse video in no-color mode")
	}
}

func TestMonoAdaptsToBackground(t *testing.T) {
	dark := Load(Options{Name: DefaultName, HasDarkBackground: true})
	light := Load(Options{Name: DefaultName, HasDarkBackground: false})

	if fmt.Sprint(dark.Tokens.Foreground) == fmt.Sprint(light.Tokens.Foreground) {
		t.Fatal("dark and light themes should use different foreground colors")
	}
	if fmt.Sprint(dark.Tokens.Selection) == fmt.Sprint(light.Tokens.Selection) {
		t.Fatal("dark and light themes should use different selection colors")
	}
}

func TestNames(t *testing.T) {
	names := Names()
	if !slices.IsSorted(names) {
		t.Fatalf("theme names = %v, want alphabetical order", names)
	}
	if !slices.Contains(names, DefaultName) {
		t.Fatalf("theme names = %v, want %q included", names, DefaultName)
	}
}

func TestUnknownEmptyNameUsesDefault(t *testing.T) {
	if got := Load(Options{}); got.Name != DefaultName {
		t.Fatalf("theme name = %q, want %q", got.Name, DefaultName)
	}
}

func TestThemesDiffer(t *testing.T) {
	seen := map[string]string{}
	for _, name := range Names() {
		got := Load(Options{Name: name, HasDarkBackground: true})
		if got.Name != name {
			t.Fatalf("Load(%q) resolved to %q", name, got.Name)
		}
		accent := fmt.Sprint(got.Tokens.Accent)
		if other, ok := seen[accent]; ok {
			t.Fatalf("%q and %q share accent %s", name, other, accent)
		}
		seen[accent] = name
	}
}

func TestEveryThemeResolvesInLightAndDark(t *testing.T) {
	for _, name := range Names() {
		for _, dark := range []bool{true, false} {
			got := Load(Options{Name: name, HasDarkBackground: dark})
			if got.Name != name {
				t.Fatalf("Load(%q, dark=%v) resolved to %q", name, dark, got.Name)
			}
			if got.Tokens.Foreground == nil || got.Tokens.Selection == nil || got.Tokens.SelectionMuted == nil || got.Tokens.Border == nil {
				t.Fatalf("Load(%q, dark=%v) left tokens unset", name, dark)
			}
		}
	}
}

func TestBatmanUsesYellowSelection(t *testing.T) {
	got := Load(Options{Name: "batman", HasDarkBackground: true})
	want := fmt.Sprint(lipgloss.Color("#ffd400"))
	if fmt.Sprint(got.Tokens.Selection) != want {
		t.Fatalf("batman selection = %s, want #ffd400", fmt.Sprint(got.Tokens.Selection))
	}
	// The focused list selection matches the menu selection.
	if fmt.Sprint(got.Tokens.SelectionFocus) != want {
		t.Fatalf("batman focused selection = %s, want #ffd400", fmt.Sprint(got.Tokens.SelectionFocus))
	}
	// The selection foreground must stay readable on the yellow bar.
	if fmt.Sprint(got.Tokens.SelectionForeground) == fmt.Sprint(got.Tokens.Foreground) {
		t.Fatalf("batman selection foreground = %s, want a dark color distinct from the base foreground", fmt.Sprint(got.Tokens.SelectionForeground))
	}
}

func TestBatmanMutedSelectionForBlurredLists(t *testing.T) {
	styles := NewStyles(Load(Options{Name: "batman", HasDarkBackground: true}))

	muted := fmt.Sprint(lipgloss.Color("#4a3d0e"))
	if got := fmt.Sprint(styles.ListRowSelectedBlurred.GetBackground()); got != muted {
		t.Fatalf("blurred selection background = %s, want the muted %s", got, muted)
	}
	if got := fmt.Sprint(styles.ListRowSelected.GetBackground()); got == muted {
		t.Fatalf("focused selection background = %s, want it brighter than the blurred bar", got)
	}
}

func TestMutedSelectionMatchesSelectionForTintedThemes(t *testing.T) {
	for _, name := range Names() {
		if name == "batman" {
			continue
		}
		got := Load(Options{Name: name, HasDarkBackground: true})
		if fmt.Sprint(got.Tokens.SelectionMuted) != fmt.Sprint(got.Tokens.Selection) {
			t.Fatalf("%s: muted selection = %s, want the selection %s", name, fmt.Sprint(got.Tokens.SelectionMuted), fmt.Sprint(got.Tokens.Selection))
		}
	}
}
