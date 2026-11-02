// Package theme defines the semantic colors and reusable styles of the sshex
// terminal UI. Screens never reference raw colors: they use the styles built
// here, so adding or changing a theme never requires touching a screen.
package theme

import (
	"image/color"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
)

// DefaultName is the theme used when no other theme is requested.
const DefaultName = "mono"

// Tokens are the semantic colors of a theme. A token is never tied to a
// specific screen; screens ask for the meaning (muted, selection, ...).
type Tokens struct {
	Foreground          color.Color
	Muted               color.Color
	Faint               color.Color
	Accent              color.Color
	AccentForeground    color.Color
	Selection           color.Color
	SelectionFocus      color.Color
	SelectionForeground color.Color
	Border              color.Color
	Success             color.Color
	Warning             color.Color
	Error               color.Color
}

// Theme is a resolved theme for one terminal.
type Theme struct {
	Name    string
	Dark    bool
	NoColor bool
	Tokens  Tokens
}

// Options selects and resolves a theme.
type Options struct {
	Name              string
	HasDarkBackground bool
	NoColor           bool
}

type paletteFunc func(dark bool) Tokens

var palettes = map[string]paletteFunc{
	DefaultName: mono,
}

// Load resolves a theme by name. Unknown names fall back to the default theme.
func Load(opts Options) Theme {
	name := strings.ToLower(strings.TrimSpace(opts.Name))
	if name == "" {
		name = DefaultName
	}
	palette, ok := palettes[name]
	if !ok {
		name = DefaultName
		palette = palettes[name]
	}

	theme := Theme{
		Name:    name,
		Dark:    opts.HasDarkBackground,
		NoColor: opts.NoColor,
	}
	if !opts.NoColor {
		theme.Tokens = palette(opts.HasDarkBackground)
	}
	return theme
}

// Names lists the available theme names in sorted order.
func Names() []string {
	names := make([]string, 0, len(palettes))
	for name := range palettes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// mono is the default black-and-white theme: contrast, weight and spacing do
// the work that color would otherwise do.
func mono(dark bool) Tokens {
	if dark {
		return Tokens{
			Foreground:          lipgloss.Color("#f2f2f2"),
			Muted:               lipgloss.Color("#9a9a9a"),
			Faint:               lipgloss.Color("#626262"),
			Accent:              lipgloss.Color("#ffffff"),
			AccentForeground:    lipgloss.Color("#0b0b0b"),
			Selection:           lipgloss.Color("#262626"),
			SelectionFocus:      lipgloss.Color("#3f3f3f"),
			SelectionForeground: lipgloss.Color("#ffffff"),
			Border:              lipgloss.Color("#3a3a3a"),
			Success:             lipgloss.Color("#7ac77a"),
			Warning:             lipgloss.Color("#d7b45b"),
			Error:               lipgloss.Color("#e06c6c"),
		}
	}
	return Tokens{
		Foreground:          lipgloss.Color("#1a1a1a"),
		Muted:               lipgloss.Color("#666666"),
		Faint:               lipgloss.Color("#999999"),
		Accent:              lipgloss.Color("#111111"),
		AccentForeground:    lipgloss.Color("#ffffff"),
		Selection:           lipgloss.Color("#e6e6e6"),
		SelectionFocus:      lipgloss.Color("#d0d0d0"),
		SelectionForeground: lipgloss.Color("#111111"),
		Border:              lipgloss.Color("#cccccc"),
		Success:             lipgloss.Color("#2f7d32"),
		Warning:             lipgloss.Color("#8a6d1f"),
		Error:               lipgloss.Color("#b3261e"),
	}
}

// Styles are the reusable styles derived from a theme. Construct them once,
// then pass them to every screen.
type Styles struct {
	Brand                  lipgloss.Style
	BrandAccent            lipgloss.Style
	Version                lipgloss.Style
	Rule                   lipgloss.Style
	FooterKey              lipgloss.Style
	FooterDesc             lipgloss.Style
	FooterSeparator        lipgloss.Style
	Logo                   lipgloss.Style
	MenuItem               lipgloss.Style
	MenuRow                lipgloss.Style
	MenuRowSelected        lipgloss.Style
	MenuMarker             lipgloss.Style
	MenuDescription        lipgloss.Style
	InputText              lipgloss.Style
	InputPlaceholder       lipgloss.Style
	InputPrompt            lipgloss.Style
	Cursor                 lipgloss.Style
	ListRow                lipgloss.Style
	ListRowSelected        lipgloss.Style
	ListRowSelectedBlurred lipgloss.Style
	ListHeader             lipgloss.Style
	ListMarker             lipgloss.Style
	ListDescription        lipgloss.Style
	ScreenTitle            lipgloss.Style
	ScreenSubtitle         lipgloss.Style
	ScreenBody             lipgloss.Style
	ScreenHint             lipgloss.Style
	Foreground             lipgloss.Style
	Muted                  lipgloss.Style
	Accent                 lipgloss.Style
	Error                  lipgloss.Style
}

// NewStyles builds the style set for a resolved theme.
func NewStyles(t Theme) Styles {
	if t.NoColor {
		return noColorStyles()
	}

	return Styles{
		Brand:           lipgloss.NewStyle().Bold(true).Foreground(t.Tokens.Foreground),
		BrandAccent:     lipgloss.NewStyle().Bold(true).Foreground(t.Tokens.Accent),
		Version:         lipgloss.NewStyle().Foreground(t.Tokens.Muted),
		Rule:            lipgloss.NewStyle().Foreground(t.Tokens.Border),
		FooterKey:       lipgloss.NewStyle().Foreground(t.Tokens.Foreground).Bold(true),
		FooterDesc:      lipgloss.NewStyle().Foreground(t.Tokens.Muted),
		FooterSeparator: lipgloss.NewStyle().Foreground(t.Tokens.Faint),
		Logo:            lipgloss.NewStyle().Foreground(t.Tokens.Accent),
		MenuItem:        lipgloss.NewStyle().Foreground(t.Tokens.Foreground),
		MenuRow:         lipgloss.NewStyle(),
		MenuRowSelected: lipgloss.NewStyle().
			Foreground(t.Tokens.SelectionForeground).
			Background(t.Tokens.Selection).
			Bold(true),
		MenuMarker:       lipgloss.NewStyle().Foreground(t.Tokens.Accent).Bold(true),
		MenuDescription:  lipgloss.NewStyle().Foreground(t.Tokens.Muted),
		InputText:        lipgloss.NewStyle().Foreground(t.Tokens.Foreground),
		InputPlaceholder: lipgloss.NewStyle().Foreground(t.Tokens.Faint),
		InputPrompt:      lipgloss.NewStyle().Foreground(t.Tokens.Accent).Bold(true),
		Cursor:           lipgloss.NewStyle().Foreground(t.Tokens.Accent),
		ListRow:          lipgloss.NewStyle(),
		ListRowSelected: lipgloss.NewStyle().
			Foreground(t.Tokens.SelectionForeground).
			Background(t.Tokens.SelectionFocus).
			Bold(true),
		ListRowSelectedBlurred: lipgloss.NewStyle().
			Foreground(t.Tokens.Muted).
			Background(t.Tokens.Selection),
		ListHeader:      lipgloss.NewStyle().Foreground(t.Tokens.Muted).Bold(true),
		ListMarker:      lipgloss.NewStyle().Foreground(t.Tokens.Accent).Bold(true),
		ListDescription: lipgloss.NewStyle().Foreground(t.Tokens.Muted),
		ScreenTitle:     lipgloss.NewStyle().Foreground(t.Tokens.Foreground).Bold(true),
		ScreenSubtitle:  lipgloss.NewStyle().Foreground(t.Tokens.Muted),
		ScreenBody:      lipgloss.NewStyle().Foreground(t.Tokens.Muted),
		ScreenHint:      lipgloss.NewStyle().Foreground(t.Tokens.Faint),
		Foreground:      lipgloss.NewStyle().Foreground(t.Tokens.Foreground),
		Muted:           lipgloss.NewStyle().Foreground(t.Tokens.Muted),
		Accent:          lipgloss.NewStyle().Foreground(t.Tokens.Accent),
		Error:           lipgloss.NewStyle().Foreground(t.Tokens.Error),
	}
}

func noColorStyles() Styles {
	return Styles{
		Brand:           lipgloss.NewStyle().Bold(true),
		BrandAccent:     lipgloss.NewStyle().Bold(true),
		Version:         lipgloss.NewStyle(),
		Rule:            lipgloss.NewStyle(),
		FooterKey:       lipgloss.NewStyle().Bold(true),
		FooterDesc:      lipgloss.NewStyle(),
		FooterSeparator: lipgloss.NewStyle(),
		Logo:            lipgloss.NewStyle(),
		MenuItem:        lipgloss.NewStyle(),
		MenuRow:         lipgloss.NewStyle(),
		// Reverse keeps the selection visible without relying on color.
		MenuRowSelected:  lipgloss.NewStyle().Bold(true).Reverse(true),
		MenuMarker:       lipgloss.NewStyle().Bold(true),
		MenuDescription:  lipgloss.NewStyle(),
		InputText:        lipgloss.NewStyle(),
		InputPlaceholder: lipgloss.NewStyle(),
		InputPrompt:      lipgloss.NewStyle().Bold(true),
		Cursor:           lipgloss.NewStyle(),
		ListRow:          lipgloss.NewStyle(),
		// Reverse keeps the selection visible without relying on color; the
		// focused row is bold so focus still reads.
		ListRowSelected:        lipgloss.NewStyle().Bold(true).Reverse(true),
		ListRowSelectedBlurred: lipgloss.NewStyle().Reverse(true),
		ListHeader:             lipgloss.NewStyle().Bold(true),
		ListMarker:             lipgloss.NewStyle().Bold(true),
		ListDescription:        lipgloss.NewStyle(),
		ScreenTitle:            lipgloss.NewStyle().Bold(true),
		ScreenSubtitle:         lipgloss.NewStyle(),
		ScreenBody:             lipgloss.NewStyle(),
		ScreenHint:             lipgloss.NewStyle(),
		Foreground:             lipgloss.NewStyle(),
		Muted:                  lipgloss.NewStyle(),
		Accent:                 lipgloss.NewStyle().Bold(true),
		Error:                  lipgloss.NewStyle().Bold(true),
	}
}
