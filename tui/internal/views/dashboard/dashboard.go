// Package dashboard provides a stats summary row and leaderboard table
// for the Agent Racer TUI.
package dashboard

import (
	"fmt"
	"sort"
	"strings"

	"github.com/agent-racer/tui/internal/client"
	"github.com/agent-racer/tui/internal/theme"
	"github.com/agent-racer/tui/internal/views/track"
	"github.com/charmbracelet/lipgloss"
)

// Model holds the dashboard state.
type Model struct {
	Width    int
	sessions []*client.SessionState
}

// New creates a dashboard model.
func New() Model {
	return Model{}
}

// SetSessions updates the session list. The dashboard sorts its own copy
// for the leaderboard so callers need not pre-sort.
func (m *Model) SetSessions(sessions map[string]*client.SessionState) {
	m.sessions = make([]*client.SessionState, 0, len(sessions))
	for _, s := range sessions {
		m.sessions = append(m.sessions, s)
	}
	sort.Slice(m.sessions, func(i, j int) bool {
		return m.sessions[i].ContextUtilization > m.sessions[j].ContextUtilization
	})
}

// View renders the full dashboard: stats row + leaderboard.
func (m Model) View() string {
	width := m.Width
	if width < 40 {
		width = 40
	}

	sections := []string{
		m.renderStatsRow(width),
		m.renderLeaderboard(width),
	}
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// renderStatsRow shows aggregate counts in a single row.
func (m Model) renderStatsRow(width int) string {
	var racing, pit, parked int
	var totalTokens, totalTools, totalMsgs int

	for _, s := range m.sessions {
		switch track.Classify(s) {
		case track.ZoneRacing:
			racing++
		case track.ZonePit:
			pit++
		case track.ZoneParked:
			parked++
		}
		totalTokens += s.TokensUsed
		totalTools += s.ToolCallCount
		totalMsgs += s.MessageCount
	}

	statStyle := lipgloss.NewStyle().Padding(0, 1)

	stats := []string{
		statStyle.Foreground(theme.ColorBright).Render(
			fmt.Sprintf("Racing: %d", racing)),
		statStyle.Foreground(theme.ColorWarning).Render(
			fmt.Sprintf("Pit: %d", pit)),
		statStyle.Foreground(theme.ColorDimmed).Render(
			fmt.Sprintf("Parked: %d", parked)),
		statStyle.Foreground(theme.ColorSonnet4).Render(
			fmt.Sprintf("Tokens: %s", formatCount(totalTokens))),
		statStyle.Foreground(theme.ColorToolUse).Render(
			fmt.Sprintf("Tools: %d", totalTools)),
		statStyle.Foreground(theme.ColorThinking).Render(
			fmt.Sprintf("Msgs: %d", totalMsgs)),
	}

	content := strings.Join(stats, lipgloss.NewStyle().Foreground(theme.ColorBorder).Render(" | "))

	return lipgloss.NewStyle().
		Width(width).
		Padding(0, 1).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(theme.ColorBorder).
		Render(content)
}

// renderLeaderboard renders a table of sessions sorted by context utilization.
func (m Model) renderLeaderboard(width int) string {
	header := lipgloss.NewStyle().Bold(true).Foreground(theme.ColorBright).
		Render("  Leaderboard")

	if len(m.sessions) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left,
			header,
			theme.StyleDimmed.Render("  No sessions"),
		)
	}

	// Column widths (fixed layout).
	colRank := 4
	colName := 24
	colModel := 14
	colCtx := 18
	colTokens := 10
	colTools := 7
	colMsgs := 7
	colActivity := 12

	dimStyle := lipgloss.NewStyle().Foreground(theme.ColorDimmed)
	brightStyle := lipgloss.NewStyle().Foreground(theme.ColorBright).Bold(true)

	// Table header row.
	tableHeader := fmt.Sprintf("  %-*s %-*s %-*s %-*s %*s %*s %*s %-*s",
		colRank, "#",
		colName, "Name",
		colModel, "Model",
		colCtx, "Context",
		colTokens, "Tokens",
		colTools, "Tools",
		colMsgs, "Msgs",
		colActivity, "Activity",
	)
	lines := []string{
		header,
		dimStyle.Render(tableHeader),
		dimStyle.Render("  " + strings.Repeat("─", min(width-4, colRank+colName+colModel+colCtx+colTokens+colTools+colMsgs+colActivity+7))),
	}

	for i, s := range m.sessions {
		rank := fmt.Sprintf("%-*d", colRank, i+1)

		name := sessionName(s)
		if len(name) > colName-1 {
			name = name[:colName-2] + "…"
		}
		nameStr := lipgloss.NewStyle().Foreground(theme.ModelColor(s.Model)).
			Width(colName).Render(name)

		modelStr := dimStyle.Width(colModel).Render(shortModel(s.Model))

		ctxBar := renderContextBar(s.ContextUtilization, colCtx-1)
		ctxStr := lipgloss.NewStyle().Width(colCtx).Render(ctxBar)

		tokStr := brightStyle.Width(colTokens).Align(lipgloss.Right).
			Render(formatCount(s.TokensUsed))
		toolStr := brightStyle.Width(colTools).Align(lipgloss.Right).
			Render(fmt.Sprintf("%d", s.ToolCallCount))
		msgStr := brightStyle.Width(colMsgs).Align(lipgloss.Right).
			Render(fmt.Sprintf("%d", s.MessageCount))

		actColor := theme.ActivityColor(string(s.Activity))
		actStr := lipgloss.NewStyle().Foreground(actColor).Width(colActivity).
			Render(string(s.Activity))

		line := fmt.Sprintf("  %s %s %s %s %s %s %s %s",
			rank, nameStr, modelStr, ctxStr, tokStr, toolStr, msgStr, actStr)
		lines = append(lines, line)
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// renderContextBar draws a small progress bar for context utilization.
func renderContextBar(pct float64, barWidth int) string {
	if barWidth < 8 {
		barWidth = 8
	}

	// Reserve space for percentage label (e.g. " 100%").
	labelWidth := 5
	fillWidth := barWidth - labelWidth
	if fillWidth < 3 {
		fillWidth = 3
	}

	filled := max(0, min(int(pct*float64(fillWidth)), fillWidth))
	empty := fillWidth - filled

	color := theme.ContextBarColor(pct)
	bar := lipgloss.NewStyle().Foreground(color).Render(strings.Repeat("█", filled))
	bar += lipgloss.NewStyle().Foreground(theme.ColorBorder).Render(strings.Repeat("░", empty))
	label := fmt.Sprintf(" %3.0f%%", pct*100)

	return bar + lipgloss.NewStyle().Foreground(color).Render(label)
}

// sessionName returns the best display name for a session.
func sessionName(s *client.SessionState) string {
	if s.Name != "" {
		return s.Name
	}
	if s.Slug != "" {
		return s.Slug
	}
	if len(s.ID) >= 8 {
		return s.ID[:8]
	}
	return s.ID
}

// shortModel returns a compact model label.
func shortModel(model string) string {
	switch {
	case strings.Contains(model, "opus"):
		return "opus"
	case strings.Contains(model, "sonnet") && strings.Contains(model, "4-5"):
		return "sonnet-4.5"
	case strings.Contains(model, "sonnet"):
		return "sonnet"
	case strings.Contains(model, "haiku"):
		return "haiku"
	case strings.Contains(model, "gemini"):
		return "gemini"
	case strings.Contains(model, "codex"):
		return "codex"
	default:
		if len(model) > 12 {
			return model[:12]
		}
		return model
	}
}

// formatCount formats large numbers with K/M suffixes.
func formatCount(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}
