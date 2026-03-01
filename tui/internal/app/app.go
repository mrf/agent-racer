package app

import (
	"context"
	"fmt"

	"github.com/agent-racer/tui/internal/client"
	"github.com/agent-racer/tui/internal/theme"
	"github.com/agent-racer/tui/internal/views/achievements"
	"github.com/agent-racer/tui/internal/views/battlepass"
	"github.com/agent-racer/tui/internal/views/dashboard"
	"github.com/agent-racer/tui/internal/views/debug"
	"github.com/agent-racer/tui/internal/views/detail"
	"github.com/agent-racer/tui/internal/views/garage"
	"github.com/agent-racer/tui/internal/views/status"
	"github.com/agent-racer/tui/internal/views/track"
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

// httpBattlePassMsg carries the result of the initial HTTP battle pass fetch.
type httpBattlePassMsg struct {
	stats      *client.Stats
	challenges []client.ChallengeProgress
	err        error
}

// Responsive breakpoints (terminal width).
const (
	breakpointCompact = 60  // minimal: short labels
	breakpointNarrow  = 80  // condensed
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

	// Navigation.
	overlay Overlay

	// Sub-views.
	statusBar    status.Model
	trackView    track.Model
	dashboard    dashboard.Model
	detailView   detail.Model
	achievements achievements.Model
	battlePass   battlepass.Model
	garageView   garage.Model
	debugLog     debug.Model

	// Connection state.
	connected bool
}

// focusResultMsg carries the result of a FocusSession HTTP call.
type focusResultMsg struct{ err error }

// New creates the root model.
func New(ws *client.WSClient, http *client.HTTPClient) Model {
	ctx, cancel := context.WithCancel(context.Background())
	m := Model{
		ws:           ws,
		http:         http,
		ctx:          ctx,
		cancel:       cancel,
		keys:         DefaultKeyMap(),
		sessions:     make(map[string]*client.SessionState),
		statusBar:    status.New(),
		trackView:    track.New(),
		dashboard:    dashboard.New(),
		achievements: achievements.New(),
		battlePass:   battlepass.New(),
		garageView:   garage.New(http),
		debugLog:     debug.New(),
	}
	m.debugLog.Add("nav", "TUI started")
	return m
}

// Init starts the WebSocket connection and fetches initial battle pass data.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.ws.Listen(m.ctx), m.loadBattlePassCmd())
}

// loadBattlePassCmd fetches stats and challenges from the HTTP API.
func (m Model) loadBattlePassCmd() tea.Cmd {
	return func() tea.Msg {
		stats, err := m.http.GetStats()
		if err != nil {
			return httpBattlePassMsg{err: err}
		}
		challenges, _ := m.http.GetChallenges()
		return httpBattlePassMsg{stats: stats, challenges: challenges}
	}
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.statusBar.Width = msg.Width
		m.trackView.Width = msg.Width
		m.trackView.Height = msg.Height
		m.dashboard.Width = msg.Width
		m.battlePass.Width = msg.Width
		m.garageView.SetSize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case httpBattlePassMsg:
		if msg.stats != nil {
			bp := msg.stats.BattlePass
			m.battlePass.SetFromStats(bp.Season, bp.Tier, bp.XP)
		}
		if msg.challenges != nil {
			m.battlePass.SetChallenges(msg.challenges)
		}
		return m, nil

	case garage.StatsLoadedMsg, garage.EquipResultMsg:
		var cmd tea.Cmd
		m.garageView, cmd = m.garageView.Update(msg)
		return m, cmd

	case client.WSConnectedMsg:
		m.connected = true
		m.statusBar.Connected = true
		m.debugLog.Add("ws", "connected")
		return m, m.ws.ReadLoop(m.ctx)

	case client.WSDisconnectedMsg:
		m.connected = false
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
		m.refreshTrack()
		m.debugLog.Add("ws", fmt.Sprintf("snapshot: %d sessions", len(msg.Payload.Sessions)))
		return m, m.ws.ReadLoop(m.ctx)

	case client.WSDeltaMsg:
		for _, s := range msg.Payload.Updates {
			m.sessions[s.ID] = s
		}
		for _, id := range msg.Payload.Removed {
			delete(m.sessions, id)
		}
		m.refreshTrack()
		m.debugLog.Add("ws", fmt.Sprintf("delta: +%d -%d", len(msg.Payload.Updates), len(msg.Payload.Removed)))
		return m, m.ws.ReadLoop(m.ctx)

	case client.WSCompletionMsg:
		if s, ok := m.sessions[msg.Payload.SessionID]; ok {
			s.Activity = msg.Payload.Activity
		}
		m.refreshTrack()
		m.debugLog.Add("ws", fmt.Sprintf("completion: %s → %s", msg.Payload.Name, string(msg.Payload.Activity)))
		return m, m.ws.ReadLoop(m.ctx)

	case client.WSSourceHealthMsg:
		m.statusBar.SourceHealth[msg.Payload.Source] = msg.Payload
		m.debugLog.Add("hlth", fmt.Sprintf("%s: %s", msg.Payload.Source, string(msg.Payload.Status)))
		return m, m.ws.ReadLoop(m.ctx)

	case client.WSEquippedMsg:
		m.garageView.SetEquipped(msg.Payload.Loadout)
		m.debugLog.Add("ws", "loadout changed")
		return m, m.ws.ReadLoop(m.ctx)

	case achievements.LoadedMsg:
		m.achievements.ApplyLoaded(msg)
		return m, nil

	case client.WSAchievementMsg:
		m.achievements.ApplyUnlock(msg.Payload.ID)
		m.debugLog.Add("ws", fmt.Sprintf("achievement: %s", msg.Payload.Name))
		return m, m.ws.ReadLoop(m.ctx)

	case client.WSBattlePassMsg:
		m.battlePass.SetProgress(msg.Payload)
		m.debugLog.Add("ws", fmt.Sprintf("xp +%d (tier %d)", msg.Payload.XP, msg.Payload.Tier))
		return m, m.ws.ReadLoop(m.ctx)

	case client.WSErrorMsg:
		m.debugLog.Add("err", string(msg.Raw))
		return m, m.ws.ReadLoop(m.ctx)

	case focusResultMsg:
		if msg.err != nil {
			m.detailView.FocusError = msg.err.Error()
		} else {
			m.detailView.FocusError = ""
		}
		return m, nil
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Always allow quit.
	if key.Matches(msg, m.keys.Quit) {
		m.cancel()
		return m, tea.Quit
	}

	// Detail overlay has focus key.
	if m.overlay == OverlayDetail {
		switch {
		case key.Matches(msg, m.keys.Escape):
			m.overlay = OverlayNone
			m.detailView.FocusError = ""
			return m, nil
		case key.Matches(msg, m.keys.Focus):
			if s := m.detailView.Session; s != nil && s.TmuxTarget != "" {
				return m, m.cmdFocusSession(s.ID)
			}
		}
		return m, nil
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

	// Other overlays: escape closes, delegate keys.
	if m.overlay != OverlayNone {
		if key.Matches(msg, m.keys.Escape) {
			m.overlay = OverlayNone
			return m, nil
		}
		if m.overlay == OverlayAchievements {
			m.achievements = m.achievements.Update(msg)
		}
		if m.overlay == OverlayGarage {
			var cmd tea.Cmd
			m.garageView, cmd = m.garageView.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	switch {
	case key.Matches(msg, m.keys.Down):
		m.trackView.MoveDown()
		return m, nil

	case key.Matches(msg, m.keys.Up):
		m.trackView.MoveUp()
		return m, nil

	case key.Matches(msg, m.keys.Tab):
		m.trackView.CycleZone()
		return m, nil

	case key.Matches(msg, m.keys.Zone1):
		m.trackView.JumpToZone(track.ZoneRacing)
		return m, nil

	case key.Matches(msg, m.keys.Zone2):
		m.trackView.JumpToZone(track.ZonePit)
		return m, nil

	case key.Matches(msg, m.keys.Zone3):
		m.trackView.JumpToZone(track.ZoneParked)
		return m, nil

	case key.Matches(msg, m.keys.Achievements):
		m.overlay = OverlayAchievements
		m.achievements = achievements.New()
		return m, achievements.FetchCmd(m.http)

	case key.Matches(msg, m.keys.Garage):
		m.overlay = OverlayGarage
		m.garageView = garage.New(m.http)
		return m, m.garageView.Init()

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
		if s := m.trackView.SelectedSession(); s != nil {
			m.detailView = detail.New(s)
			m.overlay = OverlayDetail
		}
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

	// Full-screen overlays.
	if m.overlay == OverlayAchievements {
		return m.achievements.ViewOverlay(m.width, m.height)
	}
	if m.overlay == OverlayBattlePass {
		return m.battlePass.View()
	}
	if m.overlay == OverlayGarage {
		return lipgloss.JoinVertical(lipgloss.Left,
			m.statusBar.View(),
			m.garageView.View(),
			theme.StyleDimmed.Render("  esc: close garage"),
		)
	}

	var sections []string

	sections = append(sections, m.statusBar.View())
	sections = append(sections, m.dashboard.View())

	// Debug overlay replaces the track area.
	if m.overlay == OverlayDebug {
		sections = append(sections, m.debugLog.View(m.width, m.height-4))
	} else {
		sections = append(sections, m.trackView.View())
	}

	sections = append(sections, m.battlePass.CollapsedBar())
	sections = append(sections, m.renderHelp())

	base := lipgloss.JoinVertical(lipgloss.Left, sections...)

	if m.overlay == OverlayDetail {
		panel := m.detailView.View()
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel,
			lipgloss.WithWhitespaceChars(" "),
			lipgloss.WithWhitespaceForeground(theme.ColorBg),
		)
	}

	return base
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
	return theme.StyleDimmed.Render("  j/k:navigate  tab:zone  1-3:jump  enter:detail  f:focus  a:achievements  g:garage  b:battlepass  d:debug  r:resync  q:quit")
}

// refreshTrack rebuilds the track view, dashboard, and updates status bar counts.
func (m *Model) refreshTrack() {
	m.trackView.SetSessions(m.sessions)
	racing, pit, parked := m.trackView.Counts()
	m.statusBar.SetCounts(racing, pit, parked)
	m.dashboard.SetSessions(m.sessions)
}

// cmdFocusSession returns a Cmd that calls POST /api/sessions/{id}/focus.
func (m Model) cmdFocusSession(sessionID string) tea.Cmd {
	return func() tea.Msg {
		err := m.http.FocusSession(sessionID)
		return focusResultMsg{err: err}
	}
}
