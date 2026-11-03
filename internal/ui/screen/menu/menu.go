// Package menu implements the selectable menus of the sshex terminal UI.
package menu

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mcnull/sshex/internal/ui/component/logo"
	"github.com/mcnull/sshex/internal/ui/keys"
	"github.com/mcnull/sshex/internal/ui/screen"
	"github.com/mcnull/sshex/internal/ui/theme"
)

const (
	maxBlockWidth = 64
	minRowWidth   = 32
)

// Item is one entry of a menu. Open is called when the item is selected and
// returns the screen to push. Run, when set, is called instead and acts in
// place. Describe, when set, replaces Description on every render.
type Item struct {
	Kind        string
	Title       string
	Description string
	Describe    func() string
	Open        func() screen.Screen
	Run         func() tea.Cmd
}

func (i Item) description() string {
	if i.Describe != nil {
		return i.Describe()
	}
	return i.Description
}

// Options configures a menu.
type Options struct {
	Items           []Item
	Version         string
	UpdateAvailable string
	Styles          *theme.Styles
	// Title and Help label the menu in the help overlay.
	Title string
	Help  string
	// Nested marks a menu pushed above another menu. Escape returns to the
	// previous screen instead of quitting the UI, and q is not bound.
	Nested bool
	// Scope identifies the origin this menu manages.
	Scope screen.Scope
	// Selected is the initially highlighted item.
	Selected int
}

// Model is a menu screen.
type Model struct {
	items           []Item
	keys            keys.Menu
	back            keys.Binding
	styles          *theme.Styles
	title           string
	help            string
	nested          bool
	scope           screen.Scope
	version         string
	updateAvailable string
	selected        int
	width           int
	height          int
}

// New returns a menu for the given options.
func New(opts Options) *Model {
	title := strings.TrimSpace(opts.Title)
	if title == "" {
		title = "Menu"
	}
	help := strings.TrimSpace(opts.Help)
	if help == "" {
		help = "Choose a section."
	}
	return &Model{
		items:           opts.Items,
		keys:            keys.DefaultMenu(),
		back:            keys.Back(),
		styles:          opts.Styles,
		title:           title,
		help:            help,
		nested:          opts.Nested,
		scope:           opts.Scope,
		version:         opts.Version,
		updateAvailable: opts.UpdateAvailable,
		selected:        min(max(opts.Selected, 0), max(len(opts.Items)-1, 0)),
	}
}

// Init implements screen.Screen.
func (m *Model) Init() tea.Cmd { return nil }

// Title implements screen.Screen.
func (m *Model) Title() string { return m.title }

// Scope implements screen.Scoped.
func (m *Model) Scope() screen.Scope { return m.scope }

// HelpText implements screen.HelpProvider.
func (m *Model) HelpText() string { return m.help }

// Keys implements screen.Screen.
func (m *Model) Keys() []keys.Binding {
	if m.nested {
		return []keys.Binding{m.keys.Up, m.keys.Down, m.keys.Select, m.back}
	}
	return []keys.Binding{m.keys.Up, m.keys.Down, m.keys.Select, m.keys.Quit}
}

// Resize implements screen.Screen.
func (m *Model) Resize(width, height int) {
	m.width = width
	m.height = height
}

// Selected reports the index of the highlighted item.
func (m *Model) Selected() int { return m.selected }

// Update implements screen.Screen.
func (m *Model) Update(msg tea.Msg) (screen.Screen, tea.Cmd) {
	press, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}

	switch {
	case m.nested && keys.Matches(press, m.back):
		return m, screen.Pop()
	case !m.nested && keys.Matches(press, m.keys.Quit):
		return m, screen.Quit()
	case keys.Matches(press, m.keys.Up):
		m.selected = (m.selected - 1 + len(m.items)) % len(m.items)
	case keys.Matches(press, m.keys.Down):
		m.selected = (m.selected + 1) % len(m.items)
	case keys.Matches(press, m.keys.Select):
		if len(m.items) == 0 {
			return m, nil
		}
		item := m.items[m.selected]
		if item.Run != nil {
			return m, item.Run()
		}
		if item.Open == nil {
			return m, nil
		}
		return m, screen.Push(item.Open())
	}
	return m, nil
}

// View implements screen.Screen.
func (m *Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	block := m.block()
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, block)
}

func (m *Model) block() string {
	rowWidth := m.width - 4
	if rowWidth > maxBlockWidth {
		rowWidth = maxBlockWidth
	}
	if rowWidth < minRowWidth {
		rowWidth = minRowWidth
	}
	innerWidth := rowWidth - 2

	titleWidth := m.titleWidth()
	descWidth := innerWidth - titleWidth - 4
	descWidth = max(descWidth, 0)

	rows := make([]string, 0, len(m.items))
	for i, item := range m.items {
		rows = append(rows, m.row(item, i == m.selected, rowWidth, innerWidth, titleWidth, descWidth))
	}

	offset := max((rowWidth-logo.Width)/2, 0)

	return logo.View(m.styles, rowWidth) + "\n" + m.versionLine(rowWidth, offset) + "\n\n" + strings.Join(rows, "\n")
}

func (m *Model) versionLine(rowWidth, offset int) string {
	line := strings.Repeat(" ", offset) + m.styles.Version.Render(m.versionLabel())
	if update := m.updateLabel(); update != "" {
		right := m.styles.Accent.Render(update)
		gap := offset + logo.Width - lipgloss.Width(line) - lipgloss.Width(right)
		line += strings.Repeat(" ", max(gap, 1)) + right
	}
	return padRight(line, rowWidth)
}

func (m *Model) versionLabel() string {
	if version := normalizeVersion(m.version); version != "" {
		return version
	}
	return "dev"
}

func (m *Model) updateLabel() string {
	version := normalizeVersion(m.updateAvailable)
	if version == "" {
		return ""
	}
	return "(update available: " + version + ")"
}

func normalizeVersion(version string) string {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	if version == "" {
		return ""
	}
	return "v" + version
}

func (m *Model) row(item Item, selected bool, rowWidth, innerWidth, titleWidth, descWidth int) string {
	marker := " "
	if selected {
		marker = "›"
	}
	title := padRight(item.Title, titleWidth)
	desc := padRight(truncate(item.description(), descWidth), descWidth)

	if selected {
		line := padRight(marker+" "+title+"  "+desc, innerWidth)
		return m.styles.MenuRowSelected.Padding(0, 1).Width(rowWidth).Render(line)
	}

	line := m.styles.MenuMarker.Render(marker) + " " +
		m.styles.MenuItem.Render(title) + "  " +
		m.styles.MenuDescription.Render(desc)
	line = padRight(line, innerWidth)
	return m.styles.MenuRow.Padding(0, 1).Width(rowWidth).Render(line)
}

func (m *Model) titleWidth() int {
	longest := 0
	for _, item := range m.items {
		longest = max(longest, lipgloss.Width(item.Title))
	}
	return longest + 2
}

func padRight(s string, width int) string {
	if lipgloss.Width(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-lipgloss.Width(s))
}

func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	return string(runes[:width-1]) + "…"
}
