package status

import (
	"fmt"
	"strings"

	"github.com/agent-racer/tui/internal/client"
	"github.com/agent-racer/tui/internal/theme"
	"github.com/charmbracelet/lipgloss"
)

// Model holds the status bar state.
type Model struct {
	Connected    bool
	Racing       int
	Pit          int
	Parked       int
	SourceHealth map[string]client.SourceHealthPayload
	Width        int
}

// New creates a status bar model.
func New() Model {
	return Model{
		SourceHealth: make(map[string]client.SourceHealthPayload),
	}
}

// SetCounts updates the zone counts.
func (m *Model) SetCounts(racing, pit, parked int) {
	m.Racing = racing
	m.Pit = pit
	m.Parked = parked
}

// View renders the status bar.
func (m Model) View() string {
	width := m.Width
	if width < 40 {
		width = 40
	}

	var connStr string
	if m.Connected {
		connStr = lipgloss.NewStyle().Foreground(theme.ColorHealthy).Render("● Connected")
	} else {
		connStr = lipgloss.NewStyle().Foreground(theme.ColorDanger).Render("○ Connecting...")
	}

	counts := fmt.Sprintf("%d racing  %d pit  %d parked",
		m.Racing, m.Pit, m.Parked)

	var healthParts []string
	for _, h := range m.SourceHealth {
		var color lipgloss.Color
		switch h.Status {
		case client.StatusHealthy:
			color = theme.ColorHealthy
		case client.StatusDegraded:
			color = theme.ColorWarning
		case client.StatusFailed:
			color = theme.ColorDanger
		default:
			color = theme.ColorDimmed
		}
		healthParts = append(healthParts, lipgloss.NewStyle().Foreground(color).Render(
			fmt.Sprintf("%s: %s", h.Source, string(h.Status)),
		))
	}
	healthStr := strings.Join(healthParts, "  ")

	sep := lipgloss.NewStyle().Foreground(theme.ColorBorder).Render(" | ")
	content := connStr + sep + counts
	if healthStr != "" {
		content += sep + healthStr
	}

	bar := lipgloss.NewStyle().
		Width(width).
		Padding(0, 1).
		BorderStyle(lipgloss.DoubleBorder()).
		BorderForeground(theme.ColorBorder).
		Render(content)

	return bar
}
