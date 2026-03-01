// Package debug provides a scrollable debug event log overlay.
package debug

import (
	"fmt"
	"strings"
	"time"

	"github.com/agent-racer/tui/internal/theme"
	"github.com/charmbracelet/lipgloss"
)

const maxEntries = 200

// Entry is a single event log line.
type Entry struct {
	Time    time.Time
	Kind    string // "ws", "nav", "err", etc.
	Message string
}

// Model holds debug log state.
type Model struct {
	Entries []Entry
	Offset  int // scroll offset (from bottom)
}

// New creates an empty debug model.
func New() Model {
	return Model{}
}

// Add appends a log entry and caps the buffer.
func (m *Model) Add(kind, message string) {
	m.Entries = append(m.Entries, Entry{
		Time:    time.Now(),
		Kind:    kind,
		Message: message,
	})
	if len(m.Entries) > maxEntries {
		m.Entries = m.Entries[len(m.Entries)-maxEntries:]
	}
	// Reset scroll to bottom on new entry.
	m.Offset = 0
}

// ScrollUp moves the viewport up.
func (m *Model) ScrollUp(n int) {
	m.Offset += n
	max := len(m.Entries) - 1
	if max < 0 {
		max = 0
	}
	if m.Offset > max {
		m.Offset = max
	}
}

// ScrollDown moves the viewport down.
func (m *Model) ScrollDown(n int) {
	m.Offset -= n
	if m.Offset < 0 {
		m.Offset = 0
	}
}

// panelStyle returns the shared border style for the debug overlay.
func panelStyle(width int) lipgloss.Style {
	return lipgloss.NewStyle().
		Width(width).
		Padding(1, 2).
		BorderStyle(lipgloss.DoubleBorder()).
		BorderForeground(theme.ColorBorder)
}

// View renders the debug log as an overlay panel.
func (m Model) View(width, height int) string {
	innerW := width - 4
	if innerW < 20 {
		innerW = 20
	}
	visibleLines := height - 6
	if visibleLines < 3 {
		visibleLines = 3
	}

	title := theme.StyleHeader.Render(" DEBUG LOG ")
	help := theme.StyleDimmed.Render(fmt.Sprintf("j/k:scroll  esc:close  %d entries", len(m.Entries)))

	if len(m.Entries) == 0 {
		body := theme.StyleDimmed.Render("  No events recorded yet.")
		content := lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", help)
		return panelStyle(innerW).Render(content)
	}

	// Build visible lines from bottom (minus offset).
	end := len(m.Entries) - m.Offset
	start := end - visibleLines
	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = 0
	}

	var lines []string
	for i := start; i < end; i++ {
		e := m.Entries[i]
		tsStr := theme.StyleDimmed.Render(e.Time.Format("15:04:05.000"))
		kindStr := lipgloss.NewStyle().Foreground(kindToColor(e.Kind)).Width(4).Render(e.Kind)
		msgStr := e.Message
		if len(msgStr) > innerW-20 && innerW > 20 {
			msgStr = msgStr[:innerW-23] + "..."
		}
		lines = append(lines, fmt.Sprintf("%s %s %s", tsStr, kindStr, msgStr))
	}

	body := strings.Join(lines, "\n")
	scrollIndicator := ""
	if m.Offset > 0 {
		scrollIndicator = theme.StyleDimmed.Render(fmt.Sprintf(" ↓ %d more", m.Offset))
	}

	content := lipgloss.JoinVertical(lipgloss.Left, title, body, scrollIndicator, help)
	return panelStyle(innerW).Render(content)
}

func kindToColor(kind string) lipgloss.Color {
	switch kind {
	case "ws":
		return theme.ColorThinking
	case "err":
		return theme.ColorErrored
	case "nav":
		return theme.ColorStarting
	case "hlth":
		return theme.ColorWarning
	default:
		return theme.ColorDimmed
	}
}
