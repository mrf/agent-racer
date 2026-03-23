// Package dashboard provides a stats summary row and leaderboard table
// for the Agent Racer TUI.
package dashboard

import (
	"fmt"
	"sort"
	"strings"

	"github.com/agent-racer/tui/internal/client"
	"github.com/agent-racer/tui/internal/theme"
	"github.com/agent-racer/tui/internal/views/detail"
	"github.com/agent-racer/tui/internal/views/track"
	bubbletable "github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
)

// leaderboardMaxVisible caps the number of rows visible at once; sessions
// beyond this limit are reachable by scrolling the table.
const leaderboardMaxVisible = 12

var (
	styleStat = lipgloss.NewStyle().Padding(0, 1)

	styleSeparator = lipgloss.NewStyle().
			Foreground(theme.ColorBorder)

	styleStatsBox = lipgloss.NewStyle().
			Padding(0, 1).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(theme.ColorBorder)
)

// Model holds the dashboard state.
type Model struct {
	Width    int
	sessions []*client.SessionState
	table    bubbletable.Model
}

// New creates a dashboard model.
func New() Model {
	s := bubbletable.DefaultStyles()
	s.Header = lipgloss.NewStyle().
		Bold(true).
		Foreground(theme.ColorDimmed).
		Padding(0, 1)
	s.Cell = lipgloss.NewStyle().Padding(0, 1)
	s.Selected = lipgloss.NewStyle().Bold(true).Foreground(theme.ColorBright)

	t := bubbletable.New(
		bubbletable.WithColumns([]bubbletable.Column{
			{Title: "#", Width: 4},
			{Title: "Name", Width: 22},
			{Title: "Model", Width: 10},
			{Title: "Ctx%", Width: 6},
			{Title: "Tokens", Width: 8},
			{Title: "Activity", Width: 12},
		}),
		bubbletable.WithStyles(s),
	)

	return Model{table: t}
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

	rows := make([]bubbletable.Row, 0, len(m.sessions))
	for i, s := range m.sessions {
		rows = append(rows, bubbletable.Row{
			fmt.Sprintf("%d", i+1),
			truncateName(detail.DisplayName(s), 22),
			shortModel(s.Model),
			fmt.Sprintf("%d%%", int(s.ContextUtilization*100)),
			formatCount(s.TokensUsed),
			sessionActivity(s),
		})
	}
	m.table.SetRows(rows)
}

// View renders the full dashboard: stats row + leaderboard.
func (m Model) View() string {
	width := m.Width
	if width < 40 {
		width = 40
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		m.renderStatsRow(width),
		m.renderLeaderboard(),
	)
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

	stats := []string{
		styleStat.Foreground(theme.ColorBright).Render(
			fmt.Sprintf("Racing: %d", racing)),
		styleStat.Foreground(theme.ColorWarning).Render(
			fmt.Sprintf("Pit: %d", pit)),
		styleStat.Foreground(theme.ColorDimmed).Render(
			fmt.Sprintf("Parked: %d", parked)),
		styleStat.Foreground(theme.ColorSonnet4).Render(
			fmt.Sprintf("Tokens: %s", formatCount(totalTokens))),
		styleStat.Foreground(theme.ColorToolUse).Render(
			fmt.Sprintf("Tools: %d", totalTools)),
		styleStat.Foreground(theme.ColorThinking).Render(
			fmt.Sprintf("Msgs: %d", totalMsgs)),
	}

	sep := styleSeparator.Render(" | ")
	content := strings.Join(stats, sep)

	return styleStatsBox.Width(width).Render(content)
}

// renderLeaderboard renders the bubbles/table leaderboard.
func (m Model) renderLeaderboard() string {
	header := theme.StyleHeader.Render("  Leaderboard")

	if len(m.sessions) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left,
			header,
			theme.StyleDimmed.Render("  No sessions"),
		)
	}

	// Use a local copy of the table to set height for this render
	// without mutating the stored model.
	t := m.table
	h := len(m.sessions)
	if h > leaderboardMaxVisible {
		h = leaderboardMaxVisible
	}
	t.SetHeight(h + 1) // +1 accounts for the header row consumed by SetHeight

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		t.View(),
	)
}

// sessionActivity returns the activity string for non-terminal sessions.
func sessionActivity(s *client.SessionState) string {
	if s.Activity.IsTerminal() {
		return ""
	}
	return string(s.Activity)
}

// truncateName clips a name to fit within maxLen visual cells, appending
// "…" when truncated.
func truncateName(name string, maxLen int) string {
	if len(name) > maxLen-1 {
		return name[:maxLen-2] + "…"
	}
	return name
}

// shortModel returns a compact model label.
func shortModel(model string) string {
	switch {
	case strings.Contains(model, "opus") && strings.Contains(model, "4-6"):
		return "opus-4.6"
	case strings.Contains(model, "opus"):
		return "opus"
	case strings.Contains(model, "sonnet") && strings.Contains(model, "4-6"):
		return "sonnet-4.6"
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
