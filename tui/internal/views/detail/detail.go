// Package detail renders the session info flyout overlay.
package detail

import (
	"fmt"
	"strings"
	"time"

	"github.com/agent-racer/tui/internal/client"
	"github.com/agent-racer/tui/internal/theme"
	"github.com/charmbracelet/lipgloss"
)

const (
	panelWidth = 64
	barWidth   = 20
	labelWidth = 14
)

var (
	stylePanel = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(theme.ColorBorder).
			Padding(0, 1)

	styleLabel = lipgloss.NewStyle().
			Foreground(theme.ColorDimmed).
			Width(labelWidth)

	styleValue = lipgloss.NewStyle().
			Foreground(theme.ColorBright)

	styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.ColorBright)

	styleFooter = lipgloss.NewStyle().
			Foreground(theme.ColorDimmed)

	styleSectionHeader = lipgloss.NewStyle().
				Bold(true).
				Foreground(theme.ColorDimmed)

	styleError = lipgloss.NewStyle().
			Foreground(theme.ColorDanger)
)

// Model holds the state for the detail overlay.
type Model struct {
	Session    *client.SessionState
	FocusError string
}

// New creates a detail model for the given session.
func New(s *client.SessionState) Model {
	return Model{Session: s}
}

// View renders the detail panel. Returns an empty string if no session is set.
func (m Model) View() string {
	if m.Session == nil {
		return ""
	}
	s := m.Session
	inner := m.renderInner(s)
	panel := stylePanel.Width(panelWidth).Render(inner)
	return panel
}

func (m Model) renderInner(s *client.SessionState) string {
	var b strings.Builder

	// Title row.
	name := DisplayName(s)
	title := styleTitle.Render("Session: " + name)
	b.WriteString(title + "\n")
	b.WriteString(strings.Repeat("─", panelWidth-4) + "\n")

	// Identity.
	writeRow(&b, "ID", truncate(s.ID, 36))
	writeRow(&b, "Source", theme.SourceBadge(s.Source)+" "+s.Source)
	writeRow(&b, "Model", lipgloss.NewStyle().Foreground(theme.ModelColor(s.Model)).Render(s.Model))

	actColor := theme.ActivityColor(string(s.Activity))
	writeRow(&b, "Activity", lipgloss.NewStyle().Foreground(actColor).Render(string(s.Activity)))

	if s.CurrentTool != "" {
		writeRow(&b, "Tool", s.CurrentTool)
	}

	b.WriteString("\n")

	// Context.
	ctxPct := s.ContextUtilization
	ctxBar := renderBar(ctxPct, barWidth, theme.ContextBarColor(ctxPct))
	ctxPctStr := fmt.Sprintf("%.0f%%", ctxPct*100)
	tokLabel := "est"
	if !s.TokenEstimated {
		tokLabel = "exact"
	}
	ctxDetail := fmt.Sprintf("(%s, %s)", formatTokens(s.TokensUsed), tokLabel)
	if s.MaxContextTokens > 0 {
		ctxDetail = fmt.Sprintf("(%s / %s, %s)", formatTokens(s.TokensUsed), formatTokens(s.MaxContextTokens), tokLabel)
	}
	writeRow(&b, "Context", ctxBar+" "+ctxPctStr+" "+ctxDetail)

	if s.BurnRatePerMinute > 0 {
		writeRow(&b, "Burn Rate", fmt.Sprintf("%.0f tok/min", s.BurnRatePerMinute))
	}

	writeRow(&b, "Messages", fmt.Sprintf("%d msgs  %d tool calls  %d compactions",
		s.MessageCount, s.ToolCallCount, s.CompactionCount))

	b.WriteString("\n")

	// Location.
	if s.Branch != "" {
		writeRow(&b, "Branch", s.Branch)
	}
	if s.WorkingDir != "" {
		writeRow(&b, "Working Dir", truncate(s.WorkingDir, 40))
	}
	if s.TmuxTarget != "" {
		writeRow(&b, "Tmux", s.TmuxTarget)
	}
	if s.PID != 0 {
		writeRow(&b, "PID", fmt.Sprintf("%d", s.PID))
	}

	b.WriteString("\n")

	// Timing.
	if !s.StartedAt.IsZero() {
		writeRow(&b, "Started", formatAge(s.StartedAt))
	}
	if !s.LastActivityAt.IsZero() {
		writeRow(&b, "Last Active", formatAge(s.LastActivityAt))
	}
	if s.CompletedAt != nil {
		writeRow(&b, "Completed", formatAge(*s.CompletedAt))
	}

	// Subagents.
	if len(s.Subagents) > 0 {
		b.WriteString("\n")
		b.WriteString(styleSectionHeader.Render(fmt.Sprintf("Subagents (%d)", len(s.Subagents))) + "\n")
		for i := 0; i < len(s.Subagents); i++ {
			sa := s.Subagents[i]
			b.WriteString(renderSubagent(sa) + "\n")
		}
	}

	// Error (focus failure).
	if m.FocusError != "" {
		b.WriteString("\n")
		b.WriteString(styleError.Render("Focus error: "+m.FocusError) + "\n")
	}

	// Footer.
	b.WriteString("\n")
	footer := "[f] focus tmux  [esc] close"
	if s.TmuxTarget == "" {
		footer = "[esc] close  (no tmux target)"
	}
	b.WriteString(styleFooter.Render(footer))

	return b.String()
}

func renderSubagent(sa client.SubagentState) string {
	glyph := theme.ActivityGlyph(string(sa.Activity))
	actColor := theme.ActivityColor(string(sa.Activity))
	actStr := lipgloss.NewStyle().Foreground(actColor).Render(string(sa.Activity))

	slug := sa.Slug
	if slug == "" {
		slug = truncate(sa.ID, 12)
	}
	if len(slug) > 20 {
		slug = slug[:19] + "…"
	}

	detail := fmt.Sprintf("  %s %-22s %s  tok:%-6s msg:%d",
		glyph,
		lipgloss.NewStyle().Foreground(theme.ModelColor(sa.Model)).Render(slug),
		actStr,
		formatTokens(sa.TokensUsed),
		sa.MessageCount,
	)
	if sa.CurrentTool != "" {
		detail += "  [" + sa.CurrentTool + "]"
	}
	return detail
}

func writeRow(b *strings.Builder, label, value string) {
	b.WriteString(styleLabel.Render(label+":") + styleValue.Render(value) + "\n")
}

func renderBar(pct float64, width int, color lipgloss.Color) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	filled := int(pct * float64(width))
	empty := width - filled
	bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)
	return lipgloss.NewStyle().Foreground(color).Render(bar)
}

// DisplayName returns a human-readable label for a session, preferring
// Name, then Slug, then a truncated ID.
func DisplayName(s *client.SessionState) string {
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

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func formatTokens(n int) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

func formatAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm %ds ago", int(d.Minutes()), int(d.Seconds())%60)
	default:
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		return fmt.Sprintf("%dh %dm ago", h, m)
	}
}
