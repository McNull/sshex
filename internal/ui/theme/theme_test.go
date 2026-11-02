package theme

import (
	"fmt"
	"testing"
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
	if len(names) == 0 || names[0] != DefaultName {
		t.Fatalf("theme names = %v, want %q", names, DefaultName)
	}
}

func TestUnknownEmptyNameUsesDefault(t *testing.T) {
	if got := Load(Options{}); got.Name != DefaultName {
		t.Fatalf("theme name = %q, want %q", got.Name, DefaultName)
	}
}
