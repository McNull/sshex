package update

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// View implements screen.Screen.
func (m *Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	lines := []string{m.styles.ScreenTitle.Render("Update"), ""}
	lines = append(lines, m.facts()...)
	lines = append(lines, "")
	lines = append(lines, m.outcome()...)

	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	for len(lines) < m.height {
		lines = append(lines, "")
	}
	for i := range lines {
		lines[i] = padRight(lines[i], m.width)
	}
	return strings.Join(lines, "\n")
}

type fact struct {
	label string
	value string
}

func (m *Model) facts() []string {
	rows := []fact{
		{label: "current", value: versionOrDev(m.currentVersion)},
		{label: "latest", value: versionLabel(m.availableVersion)},
		{label: "checked", value: checkedLabel(m.checkedAt)},
	}
	if m.checkErr != "" {
		rows = append(rows, fact{label: "last error", value: m.checkErr})
	}
	if m.loadErr != nil {
		rows = append(rows, fact{label: "cache", value: "unreadable: " + m.loadErr.Error()})
	}

	const labelWidth = 11
	valueWidth := max(m.width-2-labelWidth-2, 1)
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		label := m.styles.ScreenSubtitle.Render(padRight(row.label, labelWidth))
		value := m.styles.ScreenBody.Render(truncate(row.value, valueWidth))
		lines = append(lines, "  "+label+"  "+value)
	}
	return lines
}

func (m *Model) outcome() []string {
	switch m.mode {
	case modeLoading:
		return []string{m.styles.ScreenHint.Render("reading cached update state…")}
	case modeChecking:
		return []string{m.busyView("checking for updates…")}
	case modeInstalling:
		label := "updating…"
		if m.availableVersion != "" {
			label = "updating to " + versionLabel(m.availableVersion) + "…"
		}
		return []string{m.busyView(label)}
	case modePrompt:
		return m.promptView()
	}
	if m.status != "" {
		if m.statusErr {
			return []string{m.styles.Error.Render(truncate(m.status, m.width))}
		}
		return []string{m.styles.ScreenHint.Render(truncate(m.status, m.width))}
	}
	return nil
}

// busyView renders the spinner and its label without truncating the styled
// spinner frame.
func (m *Model) busyView(label string) string {
	frame := m.spinner.View()
	width := max(m.width-lipgloss.Width(frame)-1, 1)
	return frame + " " + m.styles.ScreenHint.Render(truncate(label, width))
}

func (m *Model) promptView() []string {
	headline := fmt.Sprintf(
		"Update available: %s → %s",
		versionLabel(m.currentVersion),
		versionLabel(m.availableVersion),
	)
	return []string{
		m.styles.Warning.Render(truncate(headline, m.width)),
		m.styles.ScreenBody.Render(truncate(
			"The sshex executable will be replaced. Restart sshex to use the new version.",
			m.width,
		)),
		m.styles.ScreenHint.Render(truncate("enter confirm · esc dismiss", m.width)),
	}
}

func versionLabel(version string) string {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	switch version {
	case "":
		return "unknown"
	case "dev":
		return "dev"
	default:
		return "v" + version
	}
}

func versionOrDev(version string) string {
	if label := versionLabel(version); label != "unknown" {
		return label
	}
	return "dev"
}

func checkedLabel(checkedAt time.Time) string {
	if checkedAt.IsZero() {
		return "never"
	}
	return checkedAt.Format("2006-01-02 15:04")
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
