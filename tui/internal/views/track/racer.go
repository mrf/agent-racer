package track

import (
	"fmt"
	"strings"
	"time"

	"github.com/agent-racer/tui/internal/client"
	"github.com/agent-racer/tui/internal/theme"
	"github.com/charmbracelet/lipgloss"
)

const nameWidth = 20

// activityGlyph returns the symbol for a session's current activity.
func activityGlyph(a client.Activity) string {
	switch a {
	case client.ActivityThinking:
		return "●>"
	case client.ActivityToolUse:
		return "⚙>"
	case client.ActivityIdle:
		return "○"
	case client.ActivityWaiting:
		return "◌"
	case client.ActivityStarting:
		return "◎"
	case client.ActivityComplete:
		return "✓"
	case client.ActivityErrored:
		return "✗"
	case client.ActivityLost:
		return "?"
	default:
		return "·"
	}
}

// glyphWidth returns the visual width of the glyph (accounting for multi-byte).
func glyphWidth(a client.Activity) int {
	switch a {
	case client.ActivityThinking, client.ActivityToolUse:
		return 2
	default:
		return 1
	}
}

// displayName returns a truncated session name for display.
func displayName(s *client.SessionState, maxLen int) string {
	name := s.Name
	if name == "" {
		name = s.Slug
	}
	if name == "" && len(s.ID) >= 8 {
		name = s.ID[:8]
	}
	if len(name) > maxLen {
		name = name[:maxLen-1] + "…"
	}
	return name
}

// formatTokens renders a token count in human-readable form.
func formatTokens(tokens int) string {
	if tokens >= 1000 {
		return fmt.Sprintf("%dK", tokens/1000)
	}
	return fmt.Sprintf("%d", tokens)
}

// formatDuration renders elapsed time since start as a compact string.
func formatDuration(start time.Time) string {
	if start.IsZero() {
		return ""
	}
	return formatElapsed(time.Since(start))
}

// formatElapsed renders a duration as a compact string (e.g. "42s", "3m").
func formatElapsed(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm", int(d.Minutes()))
}

// linePrefix writes the common prefix shared by all session lines:
// selection cursor, number, separator, styled glyph, badge, and padded name.
func linePrefix(b *strings.Builder, idx int, activity client.Activity, source, model, name string, selected bool) {
	if selected {
		b.WriteString(lipgloss.NewStyle().Foreground(theme.ColorBright).Bold(true).Render("> "))
	} else {
		b.WriteString("  ")
	}

	b.WriteString(theme.StyleDimmed.Render(fmt.Sprintf("%2d", idx+1)))
	b.WriteString("│ ")

	glyphStyle := lipgloss.NewStyle().Foreground(theme.ActivityColor(string(activity)))
	b.WriteString(glyphStyle.Render(activityGlyph(activity)))
	if glyphWidth(activity) < 2 {
		b.WriteByte(' ')
	}
	b.WriteByte(' ')

	b.WriteString(theme.SourceBadge(source))
	b.WriteByte(' ')

	modelStyle := lipgloss.NewStyle().Foreground(theme.ModelColor(model))
	b.WriteString(modelStyle.Render(name))
	if len(name) < nameWidth {
		b.WriteString(strings.Repeat(" ", nameWidth-len(name)))
	}
}

// renderRacingLine renders a session on the racing track with a progress bar.
func renderRacingLine(idx int, s *client.SessionState, selected bool, width int) string {
	name := displayName(s, nameWidth)

	pctStr := fmt.Sprintf("%3d%%", int(s.ContextUtilization*100))
	tokens := formatTokens(s.TokensUsed)
	elapsed := formatDuration(s.StartedAt)
	rightSide := fmt.Sprintf(" %s  %5s  %4s", pctStr, tokens, elapsed)

	// Calculate available track width for the progress bar.
	// Layout: prefix(2) + num(2) + sep(2) + glyph(1-2) + space(1) + badge(3) + space(1) + name(<=20) + space(1) + [track] + rightSide
	fixedWidth := 2 + 2 + 2 + glyphWidth(s.Activity) + 1 + 3 + 1 + len(name) + 1 + len(rightSide)
	trackWidth := width - fixedWidth
	if trackWidth < 10 {
		trackWidth = 10
	}

	var b strings.Builder
	linePrefix(&b, idx, s.Activity, s.Source, s.Model, name, selected)
	b.WriteByte(' ')
	b.WriteString(renderProgressTrack(s.ContextUtilization, trackWidth))
	b.WriteString(theme.StyleDimmed.Render(rightSide))

	return b.String()
}

// renderProgressTrack draws a progress bar showing context utilization.
func renderProgressTrack(pct float64, width int) string {
	if width <= 0 {
		return ""
	}

	pos := int(pct * float64(width-1))
	if pos < 0 {
		pos = 0
	}
	if pos >= width {
		pos = width - 1
	}

	barColor := theme.ContextBarColor(pct)
	dimStyle := lipgloss.NewStyle().Foreground(theme.ColorDimmed)
	posStyle := lipgloss.NewStyle().Foreground(barColor).Bold(true)

	var b strings.Builder
	for i := 0; i < width; i++ {
		if i == pos {
			b.WriteString(posStyle.Render("●"))
		} else {
			b.WriteString(dimStyle.Render("·"))
		}
	}

	return b.String()
}

// renderPitLine renders a session in the pit zone.
func renderPitLine(idx int, s *client.SessionState, selected bool) string {
	name := displayName(s, nameWidth)
	tokens := formatTokens(s.TokensUsed)
	elapsed := formatDuration(s.StartedAt)

	glyphStyle := lipgloss.NewStyle().Foreground(theme.ActivityColor(string(s.Activity)))

	var b strings.Builder
	linePrefix(&b, idx, s.Activity, s.Source, s.Model, name, selected)
	b.WriteString("  ")
	b.WriteString(glyphStyle.Render(string(s.Activity)))
	b.WriteString(theme.StyleDimmed.Render(fmt.Sprintf("  %5s  %4s", tokens, elapsed)))

	return b.String()
}

// renderParkedLine renders a terminal session.
func renderParkedLine(idx int, s *client.SessionState, selected bool) string {
	name := displayName(s, nameWidth)
	tokens := formatTokens(s.TokensUsed)

	var duration string
	if s.CompletedAt != nil && !s.StartedAt.IsZero() {
		duration = formatElapsed(s.CompletedAt.Sub(s.StartedAt))
	}

	glyphStyle := lipgloss.NewStyle().Foreground(theme.ActivityColor(string(s.Activity)))

	var b strings.Builder
	linePrefix(&b, idx, s.Activity, s.Source, s.Model, name, selected)
	b.WriteString("  ")
	b.WriteString(glyphStyle.Render(string(s.Activity)))
	b.WriteString(theme.StyleDimmed.Render(fmt.Sprintf("  %5s  %4s", tokens, duration)))

	return b.String()
}

// renderSubagentLine renders a subagent indented under its parent.
func renderSubagentLine(sub *client.SubagentState) string {
	glyph := activityGlyph(sub.Activity)
	glyphStyle := lipgloss.NewStyle().Foreground(theme.ActivityColor(string(sub.Activity)))
	modelStyle := lipgloss.NewStyle().Foreground(theme.ModelColor(sub.Model))

	name := sub.Slug
	if name == "" && len(sub.ID) >= 8 {
		name = sub.ID[:8]
	}
	if len(name) > 18 {
		name = name[:17] + "…"
	}

	tokens := formatTokens(sub.TokensUsed)

	var b strings.Builder
	b.WriteString(theme.StyleDimmed.Render("      └─ "))
	b.WriteString(glyphStyle.Render(glyph))
	if glyphWidth(sub.Activity) < 2 {
		b.WriteByte(' ')
	}
	b.WriteByte(' ')
	b.WriteString(modelStyle.Render(name))
	b.WriteString("  ")
	b.WriteString(glyphStyle.Render(string(sub.Activity)))
	b.WriteString(theme.StyleDimmed.Render(fmt.Sprintf("  %s", tokens)))

	return b.String()
}
