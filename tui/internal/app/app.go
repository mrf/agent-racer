package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/agent-racer/tui/internal/client"
	"github.com/agent-racer/tui/internal/theme"
	"github.com/agent-racer/tui/internal/views/debug"
	"github.com/agent-racer/tui/internal/views/status"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Overlay identifies which modal is active.
type Overlay int

const (
	OverlayNone Overlay = iota
	OverlayDetail
	OverlayAchievements
	OverlayGarage
	OverlayDebug
	OverlayBattlePass
)

// Zone identifies a track zone.
type Zone int

const (
	ZoneRacing Zone = iota
	ZonePit
	ZoneParked
)

// Responsive breakpoints (terminal width).
const (
	breakpointCompact = 60  // minimal: no tool/model info
	breakpointNarrow  = 80  // condensed: short labels
	breakpointWide    = 120 // full: all columns
)

// Model is the root Bubble Tea model.
type Model struct {
	ws     *client.WSClient
	http   *client.HTTPClient
	ctx    context.Context
	cancel context.CancelFunc

	keys   KeyMap
	width  int
	height int

	// Session state.
	sessions map[string]*client.SessionState
	order    []string // sorted session IDs

	// Navigation.
	selectedIdx int
	activeZone  Zone
	overlay     Overlay

	// Sub-views.
	statusBar status.Model
	debugLog  debug.Model

	// Connection state.
	connected bool
	reading   bool
}

// New creates the root model.
func New(ws *client.WSClient, http *client.HTTPClient) Model {
	ctx, cancel := context.WithCancel(context.Background())
	m := Model{
		ws:        ws,
		http:      http,
		ctx:       ctx,
		cancel:    cancel,
		keys:      DefaultKeyMap(),
		sessions:  make(map[string]*client.SessionState),
		statusBar: status.New(),
		debugLog:  debug.New(),
	}
	m.debugLog.Add("nav", "TUI started")
	return m
}

// Init starts the WebSocket connection.
func (m Model) Init() tea.Cmd {
	return m.ws.Listen(m.ctx)
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.statusBar.Width = msg.Width
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case client.WSConnectedMsg:
		m.connected = true
		m.statusBar.Connected = true
		m.reading = true
		m.debugLog.Add("ws", "connected")
		return m, m.ws.ReadLoop(m.ctx)

	case client.WSDisconnectedMsg:
		m.connected = false
		m.reading = false
		m.statusBar.Connected = false
		errStr := "unknown"
		if msg.Err != nil {
			errStr = msg.Err.Error()
		}
		m.debugLog.Add("ws", "disconnected: "+errStr)
		return m, m.ws.Listen(m.ctx)

	case client.WSSnapshotMsg:
		m.sessions = make(map[string]*client.SessionState)
		for _, s := range msg.Payload.Sessions {
			m.sessions[s.ID] = s
		}
		for _, h := range msg.Payload.SourceHealth {
			m.statusBar.SourceHealth[h.Source] = h
		}
		m.rebuildOrder()
		m.updateCounts()
		m.debugLog.Add("ws", fmt.Sprintf("snapshot: %d sessions", len(msg.Payload.Sessions)))
		return m, m.ws.ReadLoop(m.ctx)

	case client.WSDeltaMsg:
		for _, s := range msg.Payload.Updates {
			m.sessions[s.ID] = s
		}
		for _, id := range msg.Payload.Removed {
			delete(m.sessions, id)
		}
		m.rebuildOrder()
		m.updateCounts()
		m.debugLog.Add("ws", fmt.Sprintf("delta: +%d -%d", len(msg.Payload.Updates), len(msg.Payload.Removed)))
		return m, m.ws.ReadLoop(m.ctx)

	case client.WSCompletionMsg:
		if s, ok := m.sessions[msg.Payload.SessionID]; ok {
			s.Activity = msg.Payload.Activity
		}
		m.rebuildOrder()
		m.updateCounts()
		m.debugLog.Add("ws", fmt.Sprintf("completion: %s → %s", msg.Payload.Name, string(msg.Payload.Activity)))
		return m, m.ws.ReadLoop(m.ctx)

	case client.WSSourceHealthMsg:
		m.statusBar.SourceHealth[msg.Payload.Source] = msg.Payload
		m.debugLog.Add("hlth", fmt.Sprintf("%s: %s", msg.Payload.Source, string(msg.Payload.Status)))
		return m, m.ws.ReadLoop(m.ctx)

	case client.WSEquippedMsg:
		m.debugLog.Add("ws", "loadout changed")
		return m, m.ws.ReadLoop(m.ctx)

	case client.WSAchievementMsg:
		m.debugLog.Add("ws", fmt.Sprintf("achievement: %s", msg.Payload.Name))
		return m, m.ws.ReadLoop(m.ctx)

	case client.WSBattlePassMsg:
		m.debugLog.Add("ws", fmt.Sprintf("xp +%d (tier %d)", msg.Payload.XP, msg.Payload.Tier))
		return m, m.ws.ReadLoop(m.ctx)

	case client.WSErrorMsg:
		m.debugLog.Add("err", string(msg.Raw))
		return m, m.ws.ReadLoop(m.ctx)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Always allow quit.
	if key.Matches(msg, m.keys.Quit) {
		m.cancel()
		return m, tea.Quit
	}

	// Debug overlay has its own scroll keybindings.
	if m.overlay == OverlayDebug {
		switch {
		case key.Matches(msg, m.keys.Escape):
			m.overlay = OverlayNone
			return m, nil
		case key.Matches(msg, m.keys.Down):
			m.debugLog.ScrollDown(1)
			return m, nil
		case key.Matches(msg, m.keys.Up):
			m.debugLog.ScrollUp(1)
			return m, nil
		}
		return m, nil
	}

	// Other overlays: escape closes.
	if m.overlay != OverlayNone {
		if key.Matches(msg, m.keys.Escape) {
			m.overlay = OverlayNone
			return m, nil
		}
		return m, nil
	}

	switch {
	case key.Matches(msg, m.keys.Down):
		if len(m.order) > 0 {
			m.selectedIdx = (m.selectedIdx + 1) % len(m.order)
		}
		return m, nil

	case key.Matches(msg, m.keys.Up):
		if len(m.order) > 0 {
			m.selectedIdx = (m.selectedIdx - 1 + len(m.order)) % len(m.order)
		}
		return m, nil

	case key.Matches(msg, m.keys.Tab):
		m.activeZone = (m.activeZone + 1) % 3
		m.selectedIdx = 0
		return m, nil

	case key.Matches(msg, m.keys.Zone1):
		m.activeZone = ZoneRacing
		m.selectedIdx = 0
		return m, nil

	case key.Matches(msg, m.keys.Zone2):
		m.activeZone = ZonePit
		m.selectedIdx = 0
		return m, nil

	case key.Matches(msg, m.keys.Zone3):
		m.activeZone = ZoneParked
		m.selectedIdx = 0
		return m, nil

	case key.Matches(msg, m.keys.Achievements):
		m.overlay = OverlayAchievements
		return m, nil

	case key.Matches(msg, m.keys.Garage):
		m.overlay = OverlayGarage
		return m, nil

	case key.Matches(msg, m.keys.Debug):
		m.overlay = OverlayDebug
		return m, nil

	case key.Matches(msg, m.keys.BattlePass):
		m.overlay = OverlayBattlePass
		return m, nil

	case key.Matches(msg, m.keys.Resync):
		m.ws.Resync()
		m.debugLog.Add("nav", "resync requested")
		return m, nil

	case key.Matches(msg, m.keys.Enter):
		m.overlay = OverlayDetail
		return m, nil
	}

	return m, nil
}

// View renders the full TUI.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	// Disconnect overlay takes over the whole screen.
	if !m.connected {
		return m.renderDisconnectOverlay()
	}

	var sections []string

	sections = append(sections, m.statusBar.View())

	// Debug overlay replaces the track area.
	if m.overlay == OverlayDebug {
		sections = append(sections, m.debugLog.View(m.width, m.height-4))
	} else {
		sections = append(sections, m.renderTrack())
	}

	sections = append(sections, m.renderHelp())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// renderDisconnectOverlay shows a full-screen disconnect indicator.
func (m Model) renderDisconnectOverlay() string {
	w := m.width
	h := m.height
	if w < 40 {
		w = 40
	}

	icon := lipgloss.NewStyle().
		Foreground(theme.ColorDanger).
		Bold(true).
		Render("⚡ DISCONNECTED")

	sub := theme.StyleDimmed.Render("Reconnecting to backend...")
	hint := theme.StyleDimmed.Render("Press q to quit")

	box := lipgloss.JoinVertical(lipgloss.Center, "", icon, "", sub, "", hint, "")

	return lipgloss.NewStyle().
		Width(w).
		Height(h).
		Align(lipgloss.Center, lipgloss.Center).
		Render(box)
}

// renderHelp returns a responsive help bar.
func (m Model) renderHelp() string {
	if m.width < breakpointCompact {
		return theme.StyleDimmed.Render("  j/k tab d q")
	}
	if m.width < breakpointNarrow {
		return theme.StyleDimmed.Render("  j/k:nav  tab:zone  d:debug  r:resync  q:quit")
	}
	return theme.StyleDimmed.Render("  j/k:navigate  tab:zone  a:achievements  g:garage  b:battlepass  d:debug  r:resync  q:quit")
}

// renderTrack draws the three-zone track view.
func (m Model) renderTrack() string {
	racing, pit, parked := m.sessionsByZone()
	var lines []string

	isCompact := m.width < breakpointCompact
	isNarrow := m.width < breakpointNarrow

	// Track header — adapt to width.
	if isCompact {
		lines = append(lines, theme.StyleHeader.Render("== TRACK =="))
	} else if isNarrow {
		lines = append(lines, theme.StyleHeader.Render("=== TRACK ================================= FINISH"))
	} else {
		lines = append(lines, theme.StyleHeader.Render("=== TRACK ====================================================== FINISH"))
	}

	// Racing zone.
	lines = append(lines, m.renderZoneEntries(ZoneRacing, racing)...)

	// Pit divider.
	if isCompact {
		lines = append(lines, theme.StyleDimmed.Render("-- PIT --"))
	} else {
		lines = append(lines, theme.StyleDimmed.Render("--- PIT ---------------------------------------------------------------"))
	}

	lines = append(lines, m.renderZoneEntries(ZonePit, pit)...)

	// Parked divider.
	if isCompact {
		lines = append(lines, theme.StyleDimmed.Render("-- PARKED --"))
	} else {
		lines = append(lines, theme.StyleDimmed.Render("--- PARKED ----"))
	}

	lines = append(lines, m.renderZoneEntries(ZoneParked, parked)...)

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// renderEmptyZone returns a contextual empty state message.
func (m Model) renderEmptyZone(z Zone) string {
	if m.width < breakpointCompact {
		return theme.StyleDimmed.Render("  (empty)")
	}
	var msg string
	switch z {
	case ZoneRacing:
		msg = "  No active sessions — start a coding agent to see racers"
	case ZonePit:
		msg = "  Pit lane empty — idle sessions appear here"
	case ZoneParked:
		msg = "  No completed sessions yet"
	}
	return theme.StyleDimmed.Render(msg)
}

// renderZoneEntries returns the rendered lines for a zone's sessions,
// or an empty-state message if the zone has none.
func (m Model) renderZoneEntries(z Zone, sessions []*client.SessionState) []string {
	if len(sessions) == 0 {
		return []string{m.renderEmptyZone(z)}
	}
	lines := make([]string, 0, len(sessions))
	for i := 0; i < len(sessions); i++ {
		prefix := "  "
		if m.activeZone == z && i == m.selectedIdx {
			prefix = "> "
		}
		lines = append(lines, m.renderSessionLine(prefix, sessions[i]))
	}
	return lines
}

func (m Model) renderSessionLine(prefix string, s *client.SessionState) string {
	glyph := activityGlyph(string(s.Activity))
	name := sessionDisplayName(s)

	color := theme.ModelColor(s.Model)
	nameStr := lipgloss.NewStyle().Foreground(color).Render(name)
	actStr := lipgloss.NewStyle().Foreground(theme.ActivityColor(string(s.Activity))).Render(string(s.Activity))

	// Compact: glyph + name + activity only.
	if m.width < breakpointCompact {
		return prefix + glyph + " " + nameStr + " " + actStr
	}

	// Build extra columns for wider terminals.
	var extra []string

	// Activity label always shown.
	extra = append(extra, actStr)

	// Source badge.
	if m.width >= breakpointNarrow {
		extra = append(extra, theme.SourceBadge(s.Source))
	}

	// Current tool (wide only).
	if m.width >= breakpointWide && s.CurrentTool != "" {
		tool := s.CurrentTool
		if len(tool) > 16 {
			tool = tool[:15] + "…"
		}
		extra = append(extra, theme.StyleDimmed.Render(tool))
	}

	// Context utilization bar.
	if m.width >= breakpointNarrow && s.MaxContextTokens > 0 {
		extra = append(extra, renderContextBar(s.ContextUtilization))
	}

	// Terminal state decoration.
	switch s.Activity {
	case client.ActivityComplete:
		glyph = lipgloss.NewStyle().Foreground(theme.ColorComplete).Render("✓")
	case client.ActivityErrored:
		glyph = lipgloss.NewStyle().Foreground(theme.ColorErrored).Bold(true).Render("✗")
	case client.ActivityLost:
		glyph = lipgloss.NewStyle().Foreground(theme.ColorLost).Render("?")
	}

	return prefix + glyph + " " + nameStr + "  " + strings.Join(extra, "  ")
}

// renderContextBar draws a compact context utilization indicator.
func renderContextBar(pct float64) string {
	barWidth := 8
	filled := int(pct * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	empty := barWidth - filled
	color := theme.ContextBarColor(pct)
	bar := lipgloss.NewStyle().Foreground(color).Render(strings.Repeat("█", filled))
	bar += theme.StyleDimmed.Render(strings.Repeat("░", empty))
	label := lipgloss.NewStyle().Foreground(color).Render(fmt.Sprintf("%2.0f%%", pct*100))
	return "[" + bar + "] " + label
}

func sessionDisplayName(s *client.SessionState) string {
	name := s.Name
	if name == "" {
		name = s.Slug
	}
	if name == "" && len(s.ID) >= 8 {
		name = s.ID[:8]
	}
	if len(name) > 24 {
		name = name[:23] + "…"
	}
	return name
}

func activityGlyph(activity string) string {
	switch activity {
	case "thinking":
		return "●>"
	case "tool_use":
		return "⚙>"
	case "idle":
		return "○"
	case "waiting":
		return "◌"
	case "starting":
		return "◎"
	case "complete":
		return "✓"
	case "errored":
		return "✗"
	case "lost":
		return "?"
	default:
		return "·"
	}
}

func (m Model) sessionsByZone() (racing, pit, parked []*client.SessionState) {
	for _, id := range m.order {
		s := m.sessions[id]
		switch classifyZone(s) {
		case ZoneParked:
			parked = append(parked, s)
		case ZonePit:
			pit = append(pit, s)
		default:
			racing = append(racing, s)
		}
	}
	return
}

func classifyZone(s *client.SessionState) Zone {
	switch s.Activity {
	case client.ActivityComplete, client.ActivityErrored, client.ActivityLost:
		return ZoneParked
	case client.ActivityIdle, client.ActivityWaiting, client.ActivityStarting:
		if !s.LastDataReceivedAt.IsZero() && time.Since(s.LastDataReceivedAt).Seconds() < 30 {
			return ZoneRacing
		}
		return ZonePit
	default:
		return ZoneRacing
	}
}

func (m *Model) rebuildOrder() {
	m.order = make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		m.order = append(m.order, id)
	}
	sort.Slice(m.order, func(i, j int) bool {
		si := m.sessions[m.order[i]]
		sj := m.sessions[m.order[j]]
		zi := classifyZone(si)
		zj := classifyZone(sj)
		if zi != zj {
			return zi < zj
		}
		return si.ContextUtilization > sj.ContextUtilization
	})
}

func (m *Model) updateCounts() {
	racing, pit, parked := 0, 0, 0
	for _, s := range m.sessions {
		switch classifyZone(s) {
		case ZoneRacing:
			racing++
		case ZonePit:
			pit++
		case ZoneParked:
			parked++
		}
	}
	m.statusBar.SetCounts(racing, pit, parked)
}
