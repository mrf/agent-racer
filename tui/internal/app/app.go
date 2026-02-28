package app

import (
	"context"
	"sort"
	"time"

	"github.com/agent-racer/tui/internal/client"
	"github.com/agent-racer/tui/internal/theme"
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

	// Connection state.
	connected bool
	reading   bool
}

// New creates the root model.
func New(ws *client.WSClient, http *client.HTTPClient) Model {
	ctx, cancel := context.WithCancel(context.Background())
	return Model{
		ws:        ws,
		http:      http,
		ctx:       ctx,
		cancel:    cancel,
		keys:      DefaultKeyMap(),
		sessions:  make(map[string]*client.SessionState),
		statusBar: status.New(),
	}
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
		return m, m.ws.ReadLoop(m.ctx)

	case client.WSDisconnectedMsg:
		m.connected = false
		m.reading = false
		m.statusBar.Connected = false
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
		return m, m.ws.ReadLoop(m.ctx)

	case client.WSCompletionMsg:
		if s, ok := m.sessions[msg.Payload.SessionID]; ok {
			s.Activity = msg.Payload.Activity
		}
		m.rebuildOrder()
		m.updateCounts()
		return m, m.ws.ReadLoop(m.ctx)

	case client.WSSourceHealthMsg:
		m.statusBar.SourceHealth[msg.Payload.Source] = msg.Payload
		return m, m.ws.ReadLoop(m.ctx)

	case client.WSEquippedMsg:
		return m, m.ws.ReadLoop(m.ctx)

	case client.WSAchievementMsg:
		return m, m.ws.ReadLoop(m.ctx)

	case client.WSBattlePassMsg:
		return m, m.ws.ReadLoop(m.ctx)

	case client.WSErrorMsg:
		return m, m.ws.ReadLoop(m.ctx)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.overlay != OverlayNone {
		if key.Matches(msg, m.keys.Escape) {
			m.overlay = OverlayNone
			return m, nil
		}
		return m, nil
	}

	switch {
	case key.Matches(msg, m.keys.Quit):
		m.cancel()
		return m, tea.Quit

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

	var sections []string

	sections = append(sections, m.statusBar.View())
	sections = append(sections, m.renderTrackPlaceholder())

	help := theme.StyleDimmed.Render("  j/k:navigate  tab:zone  a:achievements  g:garage  b:battlepass  d:debug  q:quit")
	sections = append(sections, help)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m Model) renderTrackPlaceholder() string {
	racing, pit, parked := m.sessionsByZone()
	var lines []string

	header := theme.StyleHeader.Render("=== TRACK ====================================================== FINISH")
	lines = append(lines, header)

	for i, s := range racing {
		prefix := "  "
		if m.activeZone == ZoneRacing && i == m.selectedIdx {
			prefix = "> "
		}
		lines = append(lines, m.renderSessionLine(prefix, s))
	}

	lines = append(lines, theme.StyleDimmed.Render("--- PIT ---------------------------------------------------------------"))
	for i, s := range pit {
		prefix := "  "
		if m.activeZone == ZonePit && i == m.selectedIdx {
			prefix = "> "
		}
		lines = append(lines, m.renderSessionLine(prefix, s))
	}

	lines = append(lines, theme.StyleDimmed.Render("--- PARKED ----"))
	for i, s := range parked {
		prefix := "  "
		if m.activeZone == ZoneParked && i == m.selectedIdx {
			prefix = "> "
		}
		lines = append(lines, m.renderSessionLine(prefix, s))
	}

	if len(racing) == 0 && len(pit) == 0 && len(parked) == 0 {
		lines = append(lines, theme.StyleDimmed.Render("  No sessions detected"))
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m Model) renderSessionLine(prefix string, s *client.SessionState) string {
	glyph := activityGlyph(string(s.Activity))
	name := s.Name
	if name == "" {
		name = s.Slug
	}
	if name == "" && len(s.ID) >= 8 {
		name = s.ID[:8]
	}
	if len(name) > 24 {
		name = name[:23] + "..."
	}

	color := theme.ModelColor(s.Model)
	nameStr := lipgloss.NewStyle().Foreground(color).Render(name)
	actStr := lipgloss.NewStyle().Foreground(theme.ActivityColor(string(s.Activity))).Render(string(s.Activity))

	return prefix + glyph + " " + nameStr + "  " + actStr
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
		if !s.LastDataReceivedAt.IsZero() {
			if time.Since(s.LastDataReceivedAt).Seconds() < 30 {
				return ZoneRacing
			}
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
