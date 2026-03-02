// Package tail provides a live-tailing overlay for session JSONL output.
package tail

import (
	"fmt"
	"strings"
	"time"

	"github.com/agent-racer/tui/internal/client"
	"github.com/agent-racer/tui/internal/theme"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	maxEntries   = 2000
	pollInterval = 500 * time.Millisecond
)

// TailDataMsg carries new entries from a poll.
type TailDataMsg struct {
	Entries []client.TailEntry
	Offset  int64
	Err     error
}

// TickMsg triggers the next poll.
type TickMsg struct{}

// Model holds the tail view state.
type Model struct {
	SessionID   string
	SessionName string
	Activity    string

	entries  []client.TailEntry
	offset   int    // scroll offset from bottom (0 = at bottom)
	autoTail bool   // stay at bottom
	pollOff  int64  // byte offset for next poll
}

// New creates a tail model for the given session.
func New(s *client.SessionState) Model {
	name := s.Name
	if s.Slug != "" {
		name = s.Slug
	}
	return Model{
		SessionID:   s.ID,
		SessionName: name,
		Activity:    string(s.Activity),
		autoTail:    true,
	}
}

// FetchCmd returns a Cmd that fetches tail data from the backend.
func FetchCmd(httpClient *client.HTTPClient, sessionID string, offset int64) tea.Cmd {
	return func() tea.Msg {
		resp, err := httpClient.GetTail(sessionID, offset)
		if err != nil {
			return TailDataMsg{Err: err}
		}
		return TailDataMsg{
			Entries: resp.Entries,
			Offset:  resp.Offset,
		}
	}
}

// scheduleNextPoll returns a delayed Cmd to trigger the next poll.
func scheduleNextPoll() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg {
		return TickMsg{}
	})
}

// Update handles messages for the tail view.
// Returns updated model, command, and whether the message was consumed.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case TailDataMsg:
		if msg.Err != nil {
			// On error, schedule retry.
			return m, scheduleNextPoll()
		}
		if len(msg.Entries) > 0 {
			m.entries = append(m.entries, msg.Entries...)
			// Cap buffer.
			if len(m.entries) > maxEntries {
				m.entries = m.entries[len(m.entries)-maxEntries:]
			}
		}
		m.pollOff = msg.Offset
		return m, scheduleNextPoll()
	}
	return m, nil
}

// ScrollUp moves viewport up and disables auto-tail.
func (m *Model) ScrollUp(n int) {
	m.autoTail = false
	m.offset += n
	maxOff := len(m.entries) - 1
	if maxOff < 0 {
		maxOff = 0
	}
	if m.offset > maxOff {
		m.offset = maxOff
	}
}

// ScrollDown moves viewport down.
func (m *Model) ScrollDown(n int) {
	m.offset -= n
	if m.offset <= 0 {
		m.offset = 0
		m.autoTail = true
	}
}

// JumpToBottom re-enables auto-tail.
func (m *Model) JumpToBottom() {
	m.offset = 0
	m.autoTail = true
}

// PollOffset returns the current byte offset for the next poll.
func (m *Model) PollOffset() int64 {
	return m.pollOff
}

// View renders the tail overlay.
func (m Model) View(width, height int) string {
	innerW := width - 4
	if innerW < 30 {
		innerW = 30
	}
	visibleLines := height - 6
	if visibleLines < 3 {
		visibleLines = 3
	}

	// Header: session name + activity.
	actGlyph := theme.ActivityGlyph(m.Activity)
	actColor := theme.ActivityColor(m.Activity)
	actStyle := lipgloss.NewStyle().Foreground(actColor)
	header := fmt.Sprintf(" %s %s ",
		actStyle.Render(actGlyph),
		theme.StyleHeader.Render(m.SessionName),
	)

	// Footer.
	tailIndicator := ""
	if m.autoTail {
		tailIndicator = lipgloss.NewStyle().Foreground(theme.ColorComplete).Render(" LIVE")
	}
	footer := theme.StyleDimmed.Render(fmt.Sprintf(
		"j/k:scroll  G:bottom  f:focus  esc:close  %d entries%s",
		len(m.entries), tailIndicator,
	))

	if len(m.entries) == 0 {
		body := theme.StyleDimmed.Render("  Waiting for data...")
		content := lipgloss.JoinVertical(lipgloss.Left, header, "", body, "", footer)
		return panelStyle(innerW).Render(content)
	}

	// Build visible lines from bottom (minus offset).
	end := len(m.entries) - m.offset
	start := end - visibleLines
	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = 0
	}

	var lines []string
	for i := start; i < end; i++ {
		e := m.entries[i]
		lines = append(lines, renderEntry(e, innerW))
	}

	body := strings.Join(lines, "\n")

	scrollIndicator := ""
	if m.offset > 0 {
		scrollIndicator = theme.StyleDimmed.Render(fmt.Sprintf(" ↓ %d more", m.offset))
	}

	content := lipgloss.JoinVertical(lipgloss.Left, header, body, scrollIndicator, footer)
	return panelStyle(innerW).Render(content)
}

// UpdateActivity refreshes the displayed activity from current session state.
func (m *Model) UpdateActivity(activity string) {
	m.Activity = activity
}

// renderEntry formats a single tail entry as a styled line.
func renderEntry(e client.TailEntry, maxWidth int) string {
	ts := theme.StyleDimmed.Render(e.Timestamp.Format("15:04:05"))

	glyph, color := activityGlyphAndColor(e.Activity)
	glyphStr := lipgloss.NewStyle().Foreground(color).Width(3).Render(glyph)

	summary := e.Summary
	// Truncate summary to fit width (ts=8 + space + glyph=3 + space = ~14 chars overhead).
	maxSummary := maxWidth - 14
	if maxSummary < 20 {
		maxSummary = 20
	}
	if len(summary) > maxSummary {
		summary = summary[:maxSummary-1] + "…"
	}

	return fmt.Sprintf("%s %s %s", ts, glyphStr, summary)
}

// activityGlyphAndColor maps tail entry activities to display glyphs and colors.
func activityGlyphAndColor(activity string) (string, lipgloss.Color) {
	switch activity {
	case "thinking":
		return "●>", theme.ColorThinking
	case "tool_use":
		return "⚙>", theme.ColorToolUse
	case "tool_result":
		return "←", theme.ColorComplete
	case "text":
		return "··", theme.ColorBright
	case "subagent":
		return "◈", theme.ColorStarting
	case "compact":
		return "⟲", theme.ColorWarning
	case "system":
		return "◇", theme.ColorDimmed
	default:
		return "·", theme.ColorDimmed
	}
}

func panelStyle(width int) lipgloss.Style {
	return lipgloss.NewStyle().
		Width(width).
		Padding(1, 2).
		BorderStyle(lipgloss.DoubleBorder()).
		BorderForeground(theme.ColorBorder)
}
