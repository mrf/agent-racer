// Package track implements the 3-zone ASCII race track view.
// It renders sessions grouped by zone (racing/pit/parked), with racer
// glyphs colored by model, progress indicators, and subagent display.
package track

import (
	"sort"
	"strings"

	"github.com/agent-racer/tui/internal/client"
	"github.com/agent-racer/tui/internal/theme"
	"github.com/charmbracelet/lipgloss"
)

// Model holds the track view state.
type Model struct {
	// Sessions grouped by zone, rebuilt on each SetSessions call.
	racing []*client.SessionState
	pit    []*client.SessionState
	parked []*client.SessionState

	// Navigation state.
	SelectedIdx int
	ActiveZone  Zone

	// Layout dimensions.
	Width  int
	Height int
}

// New creates a track model.
func New() Model {
	return Model{}
}

// SetSessions updates the session list and rebuilds zone groupings.
func (m *Model) SetSessions(sessions map[string]*client.SessionState) {
	m.racing = nil
	m.pit = nil
	m.parked = nil

	for _, s := range sessions {
		switch Classify(s) {
		case ZoneRacing:
			m.racing = append(m.racing, s)
		case ZonePit:
			m.pit = append(m.pit, s)
		case ZoneParked:
			m.parked = append(m.parked, s)
		}
	}

	// Sort racing by context utilization (highest first).
	sort.Slice(m.racing, func(i, j int) bool {
		return m.racing[i].ContextUtilization > m.racing[j].ContextUtilization
	})
	// Sort pit by last activity (most recent first).
	sort.Slice(m.pit, func(i, j int) bool {
		return m.pit[i].LastActivityAt.After(m.pit[j].LastActivityAt)
	})
	// Sort parked by completed time (most recent first).
	sort.Slice(m.parked, func(i, j int) bool {
		if m.parked[i].CompletedAt != nil && m.parked[j].CompletedAt != nil {
			return m.parked[i].CompletedAt.After(*m.parked[j].CompletedAt)
		}
		if m.parked[i].CompletedAt != nil {
			return true
		}
		return false
	})

	// Clamp selection.
	m.clampSelection()
}

// Counts returns the number of sessions in each zone.
func (m Model) Counts() (racing, pit, parked int) {
	return len(m.racing), len(m.pit), len(m.parked)
}

// MoveDown advances the selection cursor within the active zone.
func (m *Model) MoveDown() {
	count := m.activeZoneCount()
	if count > 0 {
		m.SelectedIdx = (m.SelectedIdx + 1) % count
	}
}

// MoveUp moves the selection cursor back within the active zone.
func (m *Model) MoveUp() {
	count := m.activeZoneCount()
	if count > 0 {
		m.SelectedIdx = (m.SelectedIdx - 1 + count) % count
	}
}

// CycleZone advances to the next zone.
func (m *Model) CycleZone() {
	m.ActiveZone = (m.ActiveZone + 1) % 3
	m.SelectedIdx = 0
}

// JumpToZone sets the active zone directly.
func (m *Model) JumpToZone(z Zone) {
	m.ActiveZone = z
	m.SelectedIdx = 0
}

// SelectedSession returns the currently selected session, if any.
func (m Model) SelectedSession() *client.SessionState {
	zone := m.activeZoneSessions()
	if m.SelectedIdx >= 0 && m.SelectedIdx < len(zone) {
		return zone[m.SelectedIdx]
	}
	return nil
}

// View renders the full track view.
func (m Model) View() string {
	width := m.Width
	if width < 60 {
		width = 60
	}

	var sections []string

	// Racing zone header.
	headerText := "═══ TRACK "
	finishText := " FINISH"
	fillLen := width - len(headerText) - len(finishText) - 2
	if fillLen < 4 {
		fillLen = 4
	}
	header := headerText + strings.Repeat("═", fillLen) + finishText
	sections = append(sections, theme.StyleHeader.Render(header))

	// Racing sessions.
	if len(m.racing) == 0 {
		sections = append(sections, theme.StyleDimmed.Render("  No active sessions"))
	}
	for i, s := range m.racing {
		selected := m.ActiveZone == ZoneRacing && i == m.SelectedIdx
		sections = append(sections, renderRacingLine(i, s, selected, width))
		for _, sub := range s.Subagents {
			sub := sub
			sections = append(sections, renderSubagentLine(&sub))
		}
	}

	// Pit zone header.
	pitHeader := "─── PIT " + strings.Repeat("─", width-10)
	sections = append(sections, theme.StyleDimmed.Render(pitHeader))

	if len(m.pit) == 0 {
		sections = append(sections, theme.StyleDimmed.Render("  No sessions in pit"))
	}
	for i, s := range m.pit {
		selected := m.ActiveZone == ZonePit && i == m.SelectedIdx
		sections = append(sections, renderPitLine(i, s, selected))
		for _, sub := range s.Subagents {
			sub := sub
			sections = append(sections, renderSubagentLine(&sub))
		}
	}

	// Parked zone header.
	parkedHeader := "─── PARKED " + strings.Repeat("─", width-13)
	sections = append(sections, theme.StyleDimmed.Render(parkedHeader))

	if len(m.parked) == 0 {
		sections = append(sections, theme.StyleDimmed.Render("  No parked sessions"))
	}
	for i, s := range m.parked {
		selected := m.ActiveZone == ZoneParked && i == m.SelectedIdx
		sections = append(sections, renderParkedLine(i, s, selected))
	}

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m Model) activeZoneCount() int {
	return len(m.activeZoneSessions())
}

func (m Model) activeZoneSessions() []*client.SessionState {
	switch m.ActiveZone {
	case ZoneRacing:
		return m.racing
	case ZonePit:
		return m.pit
	case ZoneParked:
		return m.parked
	default:
		return nil
	}
}

func (m *Model) clampSelection() {
	count := m.activeZoneCount()
	if count == 0 {
		m.SelectedIdx = 0
	} else if m.SelectedIdx >= count {
		m.SelectedIdx = count - 1
	}
}
