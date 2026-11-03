package commands

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// View implements screen.Screen.
func (m *Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	m.input.SetStyles(inputStyles(m.styles))
	lines := make([]string, 0, m.height)
	lines = append(lines, m.input.View(), m.headerView())
	lines = append(lines, m.listView()...)

	for len(lines) < m.height-1 {
		lines = append(lines, "")
	}
	lines = append(lines, m.statusView())

	for i := range lines {
		lines[i] = padRight(lines[i], m.width)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) headerView() string {
	inner := max(m.width-2, 0)
	layout := newListLayout(inner)
	line := "  " +
		padRight(truncate("NAME", layout.name), layout.name) + "  " +
		padRight(truncate("ALIAS", layout.alias), layout.alias) + "  " +
		padLeft(truncate("STATE", layout.state), layout.state)
	return m.styles.ListHeader.Padding(0, 1).Width(m.width).Render(padRight(line, inner))
}

func (m *Model) listView() []string {
	rows := m.commandRows()
	if rows <= 0 {
		return nil
	}

	switch {
	case m.loading:
		return []string{m.styles.ScreenHint.Render("loading commands…")}
	case m.loadErr != nil:
		message := truncate("could not load commands: "+m.loadErr.Error(), m.width)
		return []string{m.styles.Error.Render(message)}
	case len(m.commands) == 0:
		return []string{m.styles.ScreenHint.Render("no commands")}
	case len(m.filtered) == 0:
		return []string{m.styles.ScreenHint.Render("no matching commands")}
	}

	lines := make([]string, 0, rows)
	for i := 0; i < rows; i++ {
		index := m.offset + i
		if index >= len(m.filtered) {
			break
		}
		lines = append(lines, m.row(m.filtered[index], index == m.selected))
	}
	return lines
}

func (m *Model) row(command Command, selected bool) string {
	inner := max(m.width-2, 0)
	layout := newListLayout(inner)
	marker := " "
	if selected {
		marker = "›"
	}

	state := "enabled"
	stateStyle := m.styles.Success
	if command.Disabled {
		state = "disabled"
		stateStyle = m.styles.Warning
	}

	name := padRight(truncate(command.Name, layout.name), layout.name)
	alias := padRight(truncate(command.Alias, layout.alias), layout.alias)
	state = padLeft(truncate(state, layout.state), layout.state)
	line := marker + " " + name + "  " + alias + "  " + state

	if selected {
		style := m.styles.ListRowSelected.Padding(0, 1).Width(m.width)
		return style.Render(padRight(truncate(line, inner), inner))
	}

	line = m.styles.ListMarker.Render(marker) + " " + m.styles.ListRow.Render(name) +
		"  " + m.styles.ListDescription.Render(alias) + "  " + stateStyle.Render(state)
	rowStyle := m.styles.ListRow.Padding(0, 1).Width(m.width)
	return rowStyle.Render(padRight(truncate(line, inner), inner))
}

// listLayout describes the fixed-width cells of the command list. The name
// column takes the remaining space after the alias and state columns.
type listLayout struct {
	name  int
	alias int
	state int
}

func newListLayout(inner int) listLayout {
	const (
		prefixWidth    = 2
		columnGap      = 2
		stateWidth     = 8
		preferredAlias = 16
		minNameWidth   = 8
	)

	content := max(inner-prefixWidth-2*columnGap, 0)
	state := min(stateWidth, content)
	remaining := content - state
	alias := min(preferredAlias, max(remaining-minNameWidth, 0))
	return listLayout{name: remaining - alias, alias: alias, state: state}
}

func padLeft(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = truncate(s, width)
	if lipgloss.Width(s) >= width {
		return s
	}
	return strings.Repeat(" ", width-lipgloss.Width(s)) + s
}

func (m *Model) statusView() string {
	if m.confirming {
		message := fmt.Sprintf("remove %s? enter confirm · esc cancel", m.confirmName)
		if command, ok := m.commandByName(m.confirmName); ok && command.Alias != "" {
			message += " — new shells lose the alias."
		}
		return m.styles.Warning.Render(truncate(message, m.width))
	}
	if m.status != "" {
		if m.statusErr {
			return m.styles.Error.Render(truncate(m.status, m.width))
		}
		return m.styles.ScreenHint.Render(truncate(m.status, m.width))
	}
	if m.loading || m.loadErr != nil {
		return ""
	}

	parts := make([]string, 0, 2)
	if !m.editable {
		parts = append(parts, "read-only · commands are managed on the origin")
	}
	if len(m.commands) > 0 {
		message := fmt.Sprintf("%d %s", len(m.commands), plural(len(m.commands), "command", "commands"))
		if strings.TrimSpace(m.query) != "" {
			message = fmt.Sprintf("%d of %d commands", len(m.filtered), len(m.commands))
		}
		parts = append(parts, message)
	}
	if len(parts) == 0 {
		return ""
	}
	return m.styles.ScreenHint.Render(truncate(strings.Join(parts, " · "), m.width))
}
