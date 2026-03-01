package app

import (
	"context"

	"github.com/agent-racer/tui/internal/client"
	"github.com/agent-racer/tui/internal/theme"
	"github.com/agent-racer/tui/internal/views/achievements"
	"github.com/agent-racer/tui/internal/views/battlepass"
	"github.com/agent-racer/tui/internal/views/dashboard"
	"github.com/agent-racer/tui/internal/views/detail"
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

	// Connection state.
	connected bool
}

// focusResultMsg carries the result of a FocusSession HTTP call.
type focusResultMsg struct{ err error }

// New creates the root model.
func New(ws *client.WSClient, http *client.HTTPClient) Model {
	ctx, cancel := context.WithCancel(context.Background())
	return Model{
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
	}
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

	case client.WSConnectedMsg:
		m.connected = true
		m.statusBar.Connected = true
		return m, m.ws.ReadLoop(m.ctx)

	case client.WSDisconnectedMsg:
		m.connected = false
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
		m.refreshTrack()
		return m, m.ws.ReadLoop(m.ctx)

	case client.WSDeltaMsg:
		for _, s := range msg.Payload.Updates {
			m.sessions[s.ID] = s
		}
		for _, id := range msg.Payload.Removed {
			delete(m.sessions, id)
		}
		m.refreshTrack()
		return m, m.ws.ReadLoop(m.ctx)

	case client.WSCompletionMsg:
		if s, ok := m.sessions[msg.Payload.SessionID]; ok {
			s.Activity = msg.Payload.Activity
		}
		m.refreshTrack()
		return m, m.ws.ReadLoop(m.ctx)

	case client.WSSourceHealthMsg:
		m.statusBar.SourceHealth[msg.Payload.Source] = msg.Payload
		return m, m.ws.ReadLoop(m.ctx)

	case client.WSEquippedMsg:
		return m, m.ws.ReadLoop(m.ctx)

	case achievements.LoadedMsg:
		m.achievements.ApplyLoaded(msg)
		return m, nil

	case client.WSAchievementMsg:
		m.achievements.ApplyUnlock(msg.Payload.ID)
		return m, m.ws.ReadLoop(m.ctx)

	case client.WSBattlePassMsg:
		m.battlePass.SetProgress(msg.Payload)
		return m, m.ws.ReadLoop(m.ctx)

	case client.WSErrorMsg:
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

	if m.overlay != OverlayNone {
		if key.Matches(msg, m.keys.Escape) {
			m.overlay = OverlayNone
			return m, nil
		}
		if m.overlay == OverlayAchievements {
			m.achievements = m.achievements.Update(msg)
		}
		return m, nil
	}

	switch {
	case key.Matches(msg, m.keys.Quit):
		m.cancel()
		return m, tea.Quit

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

	if m.overlay == OverlayAchievements {
		return m.achievements.ViewOverlay(m.width, m.height)
	}
	if m.overlay == OverlayBattlePass {
		return m.battlePass.View()
	}

	var sections []string

	sections = append(sections, m.statusBar.View())
	sections = append(sections, m.dashboard.View())
	sections = append(sections, m.trackView.View())
	sections = append(sections, m.battlePass.CollapsedBar())

	help := theme.StyleDimmed.Render("  j/k:navigate  tab:zone  1-3:jump  enter:detail  f:focus  a:achievements  g:garage  b:battlepass  d:debug  r:resync  q:quit")
	sections = append(sections, help)

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
