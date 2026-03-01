// Package battlepass provides the Battle Pass overlay and collapsed bar for
// the Agent Racer TUI. It renders season progress, tier track, weekly
// challenges, and a recent XP log.
package battlepass

import (
	"fmt"
	"strings"

	"github.com/agent-racer/tui/internal/client"
	"github.com/agent-racer/tui/internal/theme"
	"github.com/charmbracelet/lipgloss"
)

const (
	barWidthCollapsed = 15
	barWidthExpanded  = 30
	maxRecentXP       = 8
	maxChallenges     = 8
	visibleTiers      = 9
)

// Model holds the Battle Pass view state.
type Model struct {
	Season       string
	Tier         int
	XP           int
	TierProgress float64 // 0.0–1.0 within current tier
	RecentXP     []client.XPEntry
	Challenges   []client.ChallengeProgress
	Width        int
}

// New returns a zero-state Model.
func New() Model {
	return Model{Season: "—"}
}

// SetProgress applies a battlepass_progress WS payload.
func (m *Model) SetProgress(p client.BattlePassProgressPayload) {
	m.Tier = p.Tier
	m.XP = p.XP
	m.TierProgress = p.TierProgress
	if len(p.RecentXP) > 0 {
		m.RecentXP = p.RecentXP
	}
}

// SetChallenges replaces the challenge list.
func (m *Model) SetChallenges(cs []client.ChallengeProgress) {
	m.Challenges = cs
}

// SetFromStats seeds initial state from the /api/stats response.
func (m *Model) SetFromStats(season string, tier, xp int) {
	if season != "" {
		m.Season = season
	}
	if m.Tier == 0 && tier > 0 {
		m.Tier = tier
	}
	if m.XP == 0 && xp > 0 {
		m.XP = xp
	}
}

// CollapsedBar renders a single-line summary bar shown at the bottom of the
// main track view.
func (m Model) CollapsedBar() string {
	width := m.Width
	if width < 40 {
		width = 80
	}

	tierStr := lipgloss.NewStyle().Foreground(theme.ColorGold).Bold(true).
		Render(fmt.Sprintf("Tier %d", m.Tier))

	fill := int(m.TierProgress * float64(barWidthCollapsed))
	if fill < 0 {
		fill = 0
	}
	if fill > barWidthCollapsed {
		fill = barWidthCollapsed
	}
	bar := renderBar(fill, barWidthCollapsed)

	pct := fmt.Sprintf("%.0f%%", m.TierProgress*100)
	xpStr := lipgloss.NewStyle().Foreground(theme.ColorDimmed).
		Render(fmt.Sprintf("(%d XP)", m.XP))

	seasonStr := lipgloss.NewStyle().Foreground(theme.ColorBright).Render(m.Season)
	hint := theme.StyleDimmed.Render("  [b] expand")
	sep := lipgloss.NewStyle().Foreground(theme.ColorBorder).Render(" │ ")

	content := "  " + seasonStr + sep + tierStr + "  " + bar + "  " + pct + "  " + xpStr + hint

	return lipgloss.NewStyle().
		Width(width).
		BorderStyle(lipgloss.NormalBorder()).
		BorderTop(true).
		BorderForeground(theme.ColorBorder).
		Render(content)
}

// View renders the full expanded overlay panel.
func (m Model) View() string {
	width := m.Width
	if width < 40 {
		width = 80
	}
	inner := width - 6 // subtract border + padding

	var sb strings.Builder

	// Header.
	sb.WriteString(theme.StyleHeader.Render("BATTLE PASS"))
	sb.WriteString("  ")
	sb.WriteString(lipgloss.NewStyle().Foreground(theme.ColorGold).Render(m.Season))
	sb.WriteString("\n\n")

	// Tier track.
	sb.WriteString(renderTierTrack(m.Tier))
	sb.WriteString("\n\n")

	// XP progress bar.
	sb.WriteString(renderXPSection(m.XP, m.TierProgress, inner))
	sb.WriteString("\n\n")

	// Weekly challenges.
	if len(m.Challenges) > 0 {
		sb.WriteString(theme.StyleHeader.Render("Weekly Challenges"))
		sb.WriteString("\n")
		count := len(m.Challenges)
		if count > maxChallenges {
			count = maxChallenges
		}
		for i := 0; i < count; i++ {
			sb.WriteString(renderChallenge(m.Challenges[i], inner))
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString(theme.StyleDimmed.Render("No active challenges"))
		sb.WriteString("\n\n")
	}

	// Recent XP log.
	sb.WriteString(theme.StyleHeader.Render("Recent XP"))
	sb.WriteString("\n")
	if len(m.RecentXP) == 0 {
		sb.WriteString(theme.StyleDimmed.Render("  No recent XP"))
		sb.WriteString("\n")
	} else {
		limit := len(m.RecentXP)
		if limit > maxRecentXP {
			limit = maxRecentXP
		}
		for i := 0; i < limit; i++ {
			sb.WriteString(renderXPEntry(m.RecentXP[i]))
			sb.WriteString("\n")
		}
	}

	// Dismiss hint.
	sb.WriteString("\n")
	sb.WriteString(theme.StyleDimmed.Render("[esc] close"))

	return lipgloss.NewStyle().
		Width(width).
		Padding(1, 2).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(theme.ColorBorder).
		Render(sb.String())
}

// renderTierTrack renders a horizontal sequence of tier nodes centered on the
// current tier, with completed tiers highlighted and a forward arrow at the end.
func renderTierTrack(currentTier int) string {
	if currentTier < 1 {
		currentTier = 1
	}

	start := currentTier - visibleTiers/2
	if start < 1 {
		start = 1
	}
	end := start + visibleTiers - 1

	var parts []string
	for t := start; t <= end; t++ {
		var node string
		if t == currentTier {
			node = lipgloss.NewStyle().
				Foreground(theme.ColorGold).
				Bold(true).
				Render(fmt.Sprintf("❙%d❙", t))
		} else if t < currentTier {
			node = lipgloss.NewStyle().
				Foreground(theme.ColorComplete).
				Render(fmt.Sprintf("[%d]", t))
		} else {
			node = lipgloss.NewStyle().
				Foreground(theme.ColorDimmed).
				Render(fmt.Sprintf("[%d]", t))
		}
		parts = append(parts, node)
	}

	connector := theme.StyleDimmed.Render("──")
	track := strings.Join(parts, connector)
	track += theme.StyleDimmed.Render("──>")

	return track
}

// renderXPSection renders the XP bar and label.
func renderXPSection(xp int, tierProgress float64, width int) string {
	barWidth := barWidthExpanded
	if width/2 < barWidth {
		barWidth = width / 2
	}
	if barWidth < 10 {
		barWidth = 10
	}

	fill := int(tierProgress * float64(barWidth))
	if fill < 0 {
		fill = 0
	}
	if fill > barWidth {
		fill = barWidth
	}

	bar := renderBar(fill, barWidth)
	pct := fmt.Sprintf("%.0f%%", tierProgress*100)
	xpStr := lipgloss.NewStyle().Foreground(theme.ColorBright).
		Render(fmt.Sprintf("%d XP total", xp))

	return bar + "  " + pct + " to next tier    " + xpStr
}

// renderChallenge renders a single challenge row with a mini progress bar.
func renderChallenge(c client.ChallengeProgress, width int) string {
	const miniBar = 12

	var fill int
	if c.Target > 0 {
		fill = c.Current * miniBar / c.Target
	}
	if fill > miniBar {
		fill = miniBar
	}

	bar := renderBar(fill, miniBar)

	var status string
	if c.Complete {
		status = lipgloss.NewStyle().Foreground(theme.ColorComplete).Render(" ✓")
	} else {
		status = lipgloss.NewStyle().Foreground(theme.ColorDimmed).
			Render(fmt.Sprintf(" %d/%d", c.Current, c.Target))
	}

	desc := c.Description
	maxDesc := width - miniBar - 16
	if maxDesc < 10 {
		maxDesc = 10
	}
	if len(desc) > maxDesc {
		desc = desc[:maxDesc-1] + "…"
	}
	descStr := fmt.Sprintf("%-*s", maxDesc, desc)

	return "  " + descStr + "  " + bar + status
}

// renderXPEntry renders a single XP log entry.
func renderXPEntry(e client.XPEntry) string {
	amountStr := lipgloss.NewStyle().Foreground(theme.ColorGold).Bold(true).
		Render(fmt.Sprintf("+%-3d", e.Amount))
	return "  " + amountStr + "  " + e.Reason
}

// renderBar renders a filled/empty ASCII progress bar using block characters.
func renderBar(fill, total int) string {
	if total <= 0 {
		return "[]"
	}
	filled := strings.Repeat("█", fill)
	empty := strings.Repeat("░", total-fill)
	bar := lipgloss.NewStyle().Foreground(theme.ColorGold).Render(filled) +
		lipgloss.NewStyle().Foreground(theme.ColorDimmed).Render(empty)
	return "[" + bar + "]"
}
