// Package garage provides the Garage overlay: a 7-slot cosmetic reward selector.
package garage

import (
	"strings"

	"github.com/agent-racer/tui/internal/client"
	"github.com/agent-racer/tui/internal/theme"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// StatsLoadedMsg is returned after fetching stats from the backend.
type StatsLoadedMsg struct {
	Stats *client.Stats
	Err   error
}

// EquipResultMsg is returned after an equip or unequip call.
type EquipResultMsg struct {
	Loadout *client.Equipped
	Err     error
}

// KeyMap holds the garage-specific key bindings.
type KeyMap struct {
	Left    key.Binding
	Right   key.Binding
	Up      key.Binding
	Down    key.Binding
	Equip   key.Binding
	Unequip key.Binding
	Escape  key.Binding
}

// DefaultKeyMap returns the default garage key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Left: key.NewBinding(
			key.WithKeys("h", "left"),
			key.WithHelp("h/←", "prev slot"),
		),
		Right: key.NewBinding(
			key.WithKeys("l", "right"),
			key.WithHelp("l/→", "next slot"),
		),
		Up: key.NewBinding(
			key.WithKeys("k", "up"),
			key.WithHelp("k/↑", "prev reward"),
		),
		Down: key.NewBinding(
			key.WithKeys("j", "down"),
			key.WithHelp("j/↓", "next reward"),
		),
		Equip: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "equip"),
		),
		Unequip: key.NewBinding(
			key.WithKeys("u", "x"),
			key.WithHelp("u/x", "unequip slot"),
		),
		Escape: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "close"),
		),
	}
}

// Model is the garage overlay model.
type Model struct {
	http *client.HTTPClient
	keys KeyMap

	// bySlot holds rewards grouped by slot type, in SlotTypes order.
	bySlot map[string][]client.RewardEntry

	// equipped is the current loadout (updated on equip/unequip and WS messages).
	equipped client.Equipped

	// unlocked is the set of achievement IDs the player has earned.
	unlocked map[string]bool

	// battlePassTier is the player's current battle pass tier.
	battlePassTier int

	// slotIdx is the focused column (index into client.SlotTypes).
	slotIdx int

	// rewardIdxPerSlot tracks the focused row within each slot column.
	rewardIdxPerSlot [7]int

	// loading is true while the initial stats fetch is in flight.
	loading bool

	// statusMsg is a transient status line (equip success/error).
	statusMsg string

	width  int
	height int
}

// New creates a garage model. It begins in the loading state.
func New(http *client.HTTPClient) Model {
	return Model{
		http:    http,
		keys:    DefaultKeyMap(),
		bySlot:  client.RewardsBySlot(),
		loading: true,
	}
}

// Init fetches the initial stats so the garage knows what's unlocked/equipped.
// It resets to the loading state and fires the stats fetch command.
func (m Model) Init() tea.Cmd {
	return fetchStats(m.http)
}

// SetEquipped updates the equipped loadout (called from the parent app on WS equipped messages).
func (m *Model) SetEquipped(eq client.Equipped) {
	m.equipped = eq
}

// SetSize updates the available rendering area.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// Update handles messages for the garage overlay.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case StatsLoadedMsg:
		m.loading = false
		if msg.Err == nil && msg.Stats != nil {
			m.equipped = msg.Stats.Equipped
			m.unlocked = make(map[string]bool, len(msg.Stats.AchievementsUnlocked))
			for id := range msg.Stats.AchievementsUnlocked {
				m.unlocked[id] = true
			}
			m.battlePassTier = msg.Stats.BattlePass.Tier
		}
		return m, nil

	case EquipResultMsg:
		if msg.Err != nil {
			m.statusMsg = "Error: " + msg.Err.Error()
		} else if msg.Loadout != nil {
			m.equipped = *msg.Loadout
			m.statusMsg = "Equipped!"
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Clear transient status on any key press.
	m.statusMsg = ""

	switch {
	case key.Matches(msg, m.keys.Left):
		if m.slotIdx > 0 {
			m.slotIdx--
		}

	case key.Matches(msg, m.keys.Right):
		if m.slotIdx < len(client.SlotTypes)-1 {
			m.slotIdx++
		}

	case key.Matches(msg, m.keys.Up):
		rewards := m.bySlot[client.SlotTypes[m.slotIdx]]
		if len(rewards) > 0 {
			m.rewardIdxPerSlot[m.slotIdx] = (m.rewardIdxPerSlot[m.slotIdx] - 1 + len(rewards)) % len(rewards)
		}

	case key.Matches(msg, m.keys.Down):
		rewards := m.bySlot[client.SlotTypes[m.slotIdx]]
		if len(rewards) > 0 {
			m.rewardIdxPerSlot[m.slotIdx] = (m.rewardIdxPerSlot[m.slotIdx] + 1) % len(rewards)
		}

	case key.Matches(msg, m.keys.Equip):
		slot := client.SlotTypes[m.slotIdx]
		rewards := m.bySlot[slot]
		if len(rewards) == 0 {
			break
		}
		idx := m.rewardIdxPerSlot[m.slotIdx]
		if idx >= len(rewards) {
			break
		}
		rw := rewards[idx]
		if !m.isUnlocked(rw) {
			m.statusMsg = "Locked — earn the achievement first"
			break
		}
		return m, doEquip(m.http, rw.ID, slot)

	case key.Matches(msg, m.keys.Unequip):
		slot := client.SlotTypes[m.slotIdx]
		if client.EquippedSlot(m.equipped, slot) == "" {
			m.statusMsg = "Nothing equipped in this slot"
			break
		}
		return m, doUnequip(m.http, slot)
	}

	return m, nil
}

// View renders the garage overlay.
func (m Model) View() string {
	if m.loading {
		return theme.StyleBorder.Padding(1, 2).Render("Loading garage...")
	}

	title := theme.StyleHeader.Render("  GARAGE  ")
	header := lipgloss.NewStyle().
		Foreground(theme.ColorBright).
		Bold(true).
		Render("╔═ " + title + " ══════════════════════════════════════════╗")

	cols := m.renderColumns()

	help := theme.StyleDimmed.Render("  h/l: slot  j/k: reward  enter: equip  u: unequip  esc: close")

	sections := []string{header, "", cols, "", help}
	if m.statusMsg != "" {
		sections = append(sections,
			lipgloss.NewStyle().Foreground(theme.ColorWarning).Render("  "+m.statusMsg))
	}

	body := lipgloss.JoinVertical(lipgloss.Left, sections...)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.ColorBorder).
		Padding(0, 1).
		Render(body)
}

func (m Model) renderColumns() string {
	const colWidth = 18
	cols := make([]string, len(client.SlotTypes))
	for i, slot := range client.SlotTypes {
		cols[i] = m.renderColumn(i, slot, colWidth)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, cols...)
}

func (m Model) renderColumn(slotIdx int, slot string, colWidth int) string {
	focused := slotIdx == m.slotIdx
	rewards := m.bySlot[slot]
	equippedID := client.EquippedSlot(m.equipped, slot)
	selectedRow := m.rewardIdxPerSlot[slotIdx]

	// Pick accent color based on focus state.
	accentColor := theme.ColorBorder
	if focused {
		accentColor = theme.ColorBright
	}

	header := lipgloss.NewStyle().
		Bold(focused).
		Foreground(accentColor).
		Width(colWidth).
		Align(lipgloss.Center).
		Render(strings.ToUpper(slot))

	sep := lipgloss.NewStyle().
		Foreground(accentColor).
		Render(strings.Repeat("─", colWidth))

	var rows []string
	for i, rw := range rewards {
		rows = append(rows, renderRewardRow(rw, focused && i == selectedRow, rw.ID == equippedID, m.isUnlocked(rw), colWidth))
	}

	if equippedID == "" {
		rows = append(rows, lipgloss.NewStyle().
			Foreground(theme.ColorDimmed).
			Width(colWidth).
			Render("  (none equipped)"))
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		append([]string{header, sep}, rows...)...)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accentColor).
		Width(colWidth - 2).
		Render(content)
}

func renderRewardRow(rw client.RewardEntry, isSelected, isEquipped, isUnlocked bool, colWidth int) string {
	var prefix string
	if isEquipped {
		prefix = "▶ "
	} else if isSelected {
		prefix = "> "
	} else {
		prefix = "  "
	}

	name := rw.Name
	maxLen := colWidth - len(prefix) - 1
	if maxLen < 1 {
		maxLen = 1
	}
	if len(name) > maxLen {
		name = name[:maxLen-1] + "…"
	}

	// Determine color and bold based on state priority: equipped > selected > unlocked > locked.
	var color lipgloss.Color
	bold := isSelected
	switch {
	case isEquipped:
		color = theme.ColorHealthy
	case isSelected && isUnlocked:
		color = theme.ColorBright
	case isSelected:
		color = theme.ColorDimmed
	case isUnlocked:
		color = theme.ColorDefault
	default:
		color = theme.ColorBorder
	}

	return lipgloss.NewStyle().
		Bold(bold).
		Foreground(color).
		Width(colWidth).
		Render(prefix + name)
}

// isUnlocked reports whether the player has access to a reward.
func (m Model) isUnlocked(rw client.RewardEntry) bool {
	if rw.UnlockedBy == "" {
		// Battle pass reward — check tier. We don't know the exact tier here,
		// but the server will reject if not unlocked. Show all BP rewards as unlocked
		// based on the fact that Stats.Equipped only shows IDs the server allowed.
		// For display, we use a simple heuristic: if battlePassTier > 0, BP rewards are potentially unlocked.
		return m.battlePassTier > 0
	}
	return m.unlocked[rw.UnlockedBy]
}

func fetchStats(h *client.HTTPClient) tea.Cmd {
	return func() tea.Msg {
		stats, err := h.GetStats()
		return StatsLoadedMsg{Stats: stats, Err: err}
	}
}

func doEquip(h *client.HTTPClient, rewardID, slot string) tea.Cmd {
	return func() tea.Msg {
		loadout, err := h.Equip(rewardID, slot)
		return EquipResultMsg{Loadout: loadout, Err: err}
	}
}

func doUnequip(h *client.HTTPClient, slot string) tea.Cmd {
	return func() tea.Msg {
		loadout, err := h.Unequip(slot)
		return EquipResultMsg{Loadout: loadout, Err: err}
	}
}
