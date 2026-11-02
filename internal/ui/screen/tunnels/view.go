package tunnels

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

	lines := make([]string, 0, m.height)
	lines = append(lines, m.headerView())
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
	left := "  TUNNEL"
	right := "STATE"
	gap := max(inner-lipgloss.Width(left)-lipgloss.Width(right), 1)
	line := left + strings.Repeat(" ", gap) + right
	return m.styles.ListHeader.Padding(0, 1).Width(m.width).Render(padRight(line, inner))
}

func (m *Model) listView() []string {
	rows := m.tunnelRows()
	if rows <= 0 {
		return nil
	}

	switch {
	case m.loading:
		return []string{m.styles.ScreenHint.Render("loading tunnels…")}
	case m.loadErr != nil:
		message := truncate("could not load tunnels: "+m.loadErr.Error(), m.width)
		return []string{m.styles.Error.Render(message)}
	case m.noSessions():
		return []string{m.styles.ScreenHint.Render("no active sessions")}
	case len(m.tunnels) == 0:
		return []string{m.styles.ScreenHint.Render("no active tunnels")}
	}

	lines := make([]string, 0, rows*2)
	for i := 0; i < rows; i++ {
		index := m.offset + i
		if index >= len(m.tunnels) {
			break
		}
		first, second := m.row(m.tunnels[index], index == m.selected)
		lines = append(lines, first, second)
	}
	return lines
}

func (m *Model) row(tunnel Tunnel, selected bool) (string, string) {
	inner := max(m.width-2, 0)
	marker := " "
	if selected {
		marker = "›"
	}

	state := tunnel.State
	if state == "" {
		state = "unknown"
	}
	forwardWidth := max(inner-lipgloss.Width(state)-3, 1)
	forward := truncate(tunnel.Forward(), forwardWidth)
	left := marker + " " + forward
	gap := max(inner-lipgloss.Width(left)-lipgloss.Width(state), 1)
	first := left + strings.Repeat(" ", gap) + state
	second := m.detail(tunnel, selected, inner)

	if selected {
		style := m.styles.ListRowSelected.Padding(0, 1).Width(m.width)
		return style.Render(padRight(first, inner)), style.Render(padRight(second, inner))
	}

	stateStyle := m.stateStyle(tunnel.State)
	first = m.styles.ListMarker.Render(marker) + " " + m.styles.ListRow.Render(forward) +
		strings.Repeat(" ", gap) + stateStyle.Render(state)

	rowStyle := m.styles.ListRow.Padding(0, 1).Width(m.width)
	return rowStyle.Render(padRight(first, inner)), rowStyle.Render(padRight(second, inner))
}

func (m *Model) detail(tunnel Tunnel, selected bool, inner int) string {
	parts := []string{tunnel.ID}
	if target := m.targetFor(tunnel); target != "" {
		parts = append(parts, target)
	}
	direction := "local"
	if tunnel.Direction == DirectionRemote {
		direction = "remote"
	}
	parts = append(parts, direction)
	base := "  " + strings.Join(parts, " · ")

	type suffix struct {
		text  string
		style lipgloss.Style
	}
	var suffixes []suffix
	if tunnel.Stale {
		suffixes = append(suffixes, suffix{" · stale", m.styles.Warning})
	}
	if tunnel.Error != "" {
		suffixes = append(suffixes, suffix{" · " + tunnel.Error, m.styles.Error})
	}
	if len(suffixes) == 0 {
		if selected {
			return truncate(base, inner)
		}
		return m.styles.ListDescription.Render(truncate(base, inner))
	}

	plain := base
	suffixWidth := 0
	for _, s := range suffixes {
		plain += s.text
		suffixWidth += lipgloss.Width(s.text)
	}
	if selected || suffixWidth >= inner {
		return truncate(plain, inner)
	}

	out := m.styles.ListDescription.Render(truncate(base, inner-suffixWidth))
	for _, s := range suffixes {
		out += s.style.Render(s.text)
	}
	return out
}

func (m *Model) stateStyle(state string) lipgloss.Style {
	switch state {
	case "active":
		return m.styles.Success
	case "failed":
		return m.styles.Error
	case "pending":
		return m.styles.Warning
	default:
		return m.styles.Muted
	}
}

func (m *Model) statusView() string {
	if m.confirming {
		message := fmt.Sprintf("remove %s? the forward stops immediately.", m.confirmFor)
		return m.styles.Warning.Render(truncate(message, m.width))
	}
	if m.status != "" {
		if m.statusErr {
			return m.styles.Error.Render(truncate(m.status, m.width))
		}
		return m.styles.ScreenHint.Render(truncate(m.status, m.width))
	}
	if m.loading || m.loadErr != nil || len(m.tunnels) == 0 {
		return ""
	}
	message := fmt.Sprintf("%d %s", len(m.tunnels), plural(len(m.tunnels), "tunnel", "tunnels"))
	return m.styles.ScreenHint.Render(truncate(message, m.width))
}

func plural(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
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
