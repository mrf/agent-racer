package garage

import (
	"strings"
	"testing"

	"github.com/agent-racer/tui/internal/client"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// keyMsg constructs a tea.KeyMsg from a key name. Named keys (left, right, etc.)
// produce the appropriate KeyType; single-char keys produce KeyRunes.
func keyMsg(k string) tea.KeyMsg {
	switch k {
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEscape}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
	}
}

func TestDefaultKeyMap(t *testing.T) {
	km := DefaultKeyMap()

	tests := []struct {
		name    string
		binding key.Binding
		keys    []string
	}{
		{"Left", km.Left, []string{"h", "left"}},
		{"Right", km.Right, []string{"l", "right"}},
		{"Up", km.Up, []string{"k", "up"}},
		{"Down", km.Down, []string{"j", "down"}},
		{"Equip", km.Equip, []string{"enter"}},
		{"Unequip", km.Unequip, []string{"u", "x"}},
		{"Escape", km.Escape, []string{"esc"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, k := range tt.keys {
				if !key.Matches(keyMsg(k), tt.binding) {
					t.Errorf("key %q should match %s binding", k, tt.name)
				}
			}
		})
	}
}

func TestDefaultKeyMap_HelpText(t *testing.T) {
	km := DefaultKeyMap()
	if km.Left.Help().Key != "h/←" {
		t.Errorf("Left help key = %q, want %q", km.Left.Help().Key, "h/←")
	}
	if km.Equip.Help().Desc != "equip" {
		t.Errorf("Equip help desc = %q, want %q", km.Equip.Help().Desc, "equip")
	}
}

func TestUpdate_StatsLoaded(t *testing.T) {
	m := Model{
		keys:   DefaultKeyMap(),
		bySlot: client.RewardsBySlot(),
		loading: true,
	}

	stats := &client.Stats{
		Equipped: client.Equipped{Paint: "rookie_paint"},
		AchievementsUnlocked: map[string]string{
			"first_lap": "2026-01-01T00:00:00Z",
		},
		BattlePass: client.BattlePass{Tier: 3},
	}

	m, _ = m.Update(StatsLoadedMsg{Stats: stats, Err: nil})

	if m.loading {
		t.Error("loading should be false after StatsLoaded")
	}
	if m.equipped.Paint != "rookie_paint" {
		t.Errorf("equipped paint = %q, want %q", m.equipped.Paint, "rookie_paint")
	}
	if !m.unlocked["first_lap"] {
		t.Error("first_lap should be unlocked")
	}
	if m.battlePassTier != 3 {
		t.Errorf("battlePassTier = %d, want 3", m.battlePassTier)
	}
}

func TestUpdate_StatsLoadedError(t *testing.T) {
	m := Model{
		keys:    DefaultKeyMap(),
		bySlot:  client.RewardsBySlot(),
		loading: true,
	}

	m, _ = m.Update(StatsLoadedMsg{Stats: nil, Err: nil})
	if m.loading {
		t.Error("loading should be false even with nil stats")
	}
}

func TestUpdate_EquipResult(t *testing.T) {
	m := Model{
		keys:   DefaultKeyMap(),
		bySlot: client.RewardsBySlot(),
	}

	loadout := &client.Equipped{Paint: "metallic_paint"}
	m, _ = m.Update(EquipResultMsg{Loadout: loadout, Err: nil})

	if m.equipped.Paint != "metallic_paint" {
		t.Errorf("equipped paint = %q, want %q", m.equipped.Paint, "metallic_paint")
	}
	if m.statusMsg != "Equipped!" {
		t.Errorf("statusMsg = %q, want %q", m.statusMsg, "Equipped!")
	}
}

func TestUpdate_EquipResultError(t *testing.T) {
	m := Model{
		keys:   DefaultKeyMap(),
		bySlot: client.RewardsBySlot(),
	}

	m, _ = m.Update(EquipResultMsg{Err: &testError{"equip failed"}})
	if !strings.Contains(m.statusMsg, "equip failed") {
		t.Errorf("statusMsg = %q, should contain error", m.statusMsg)
	}
}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func TestUpdate_KeyNavigation(t *testing.T) {
	m := Model{
		keys:   DefaultKeyMap(),
		bySlot: client.RewardsBySlot(),
	}

	// Move right
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if m.slotIdx != 1 {
		t.Errorf("slotIdx after right = %d, want 1", m.slotIdx)
	}

	// Move left
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	if m.slotIdx != 0 {
		t.Errorf("slotIdx after left = %d, want 0", m.slotIdx)
	}

	// Left at 0 stays
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	if m.slotIdx != 0 {
		t.Error("slotIdx should not go below 0")
	}

	// Right at max stays
	m.slotIdx = len(client.SlotTypes) - 1
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if m.slotIdx != len(client.SlotTypes)-1 {
		t.Error("slotIdx should not exceed max")
	}
}

func TestUpdate_KeyNavigationUpDown(t *testing.T) {
	m := Model{
		keys:   DefaultKeyMap(),
		bySlot: client.RewardsBySlot(),
	}

	// Navigate down in the first slot (paint)
	paintRewards := m.bySlot["paint"]
	if len(paintRewards) == 0 {
		t.Skip("no paint rewards in catalog")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.rewardIdxPerSlot[0] != 1 {
		t.Errorf("reward idx after down = %d, want 1", m.rewardIdxPerSlot[0])
	}

	// Navigate up
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if m.rewardIdxPerSlot[0] != 0 {
		t.Errorf("reward idx after up = %d, want 0", m.rewardIdxPerSlot[0])
	}

	// Up wraps around
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if m.rewardIdxPerSlot[0] != len(paintRewards)-1 {
		t.Errorf("reward idx after wrap = %d, want %d", m.rewardIdxPerSlot[0], len(paintRewards)-1)
	}
}

func TestUpdate_KeyClearsStatus(t *testing.T) {
	m := Model{
		keys:      DefaultKeyMap(),
		bySlot:    client.RewardsBySlot(),
		statusMsg: "old status",
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	if m.statusMsg != "" {
		t.Error("key press should clear statusMsg")
	}
}

func TestSetEquipped(t *testing.T) {
	m := Model{keys: DefaultKeyMap(), bySlot: client.RewardsBySlot()}
	eq := client.Equipped{Trail: "flame_trail"}
	m.SetEquipped(eq)
	if m.equipped.Trail != "flame_trail" {
		t.Errorf("equipped trail = %q, want %q", m.equipped.Trail, "flame_trail")
	}
}

func TestSetSize(t *testing.T) {
	m := Model{keys: DefaultKeyMap(), bySlot: client.RewardsBySlot()}
	m.SetSize(120, 40)
	if m.width != 120 || m.height != 40 {
		t.Errorf("size = %dx%d, want 120x40", m.width, m.height)
	}
}

func TestView_Loading(t *testing.T) {
	m := Model{
		keys:    DefaultKeyMap(),
		bySlot:  client.RewardsBySlot(),
		loading: true,
	}
	view := m.View()
	if !strings.Contains(view, "Loading garage") {
		t.Error("loading view should show 'Loading garage'")
	}
}

func TestView_WithData(t *testing.T) {
	m := Model{
		keys:           DefaultKeyMap(),
		bySlot:         client.RewardsBySlot(),
		equipped:       client.Equipped{Paint: "rookie_paint"},
		unlocked:       map[string]bool{"first_lap": true},
		battlePassTier: 3,
	}

	view := m.View()
	if !strings.Contains(view, "GARAGE") {
		t.Error("should contain 'GARAGE' title")
	}
	if !strings.Contains(view, "equip") {
		t.Error("should contain help text with 'equip'")
	}
}

func TestView_StatusMessage(t *testing.T) {
	m := Model{
		keys:      DefaultKeyMap(),
		bySlot:    client.RewardsBySlot(),
		statusMsg: "Equipped!",
	}
	view := m.View()
	if !strings.Contains(view, "Equipped!") {
		t.Error("should show status message")
	}
}

func TestIsUnlocked(t *testing.T) {
	m := Model{
		unlocked:       map[string]bool{"first_lap": true},
		battlePassTier: 3,
	}

	// Achievement-unlocked reward
	if !m.isUnlocked(client.RewardEntry{UnlockedBy: "first_lap"}) {
		t.Error("first_lap reward should be unlocked")
	}
	if m.isUnlocked(client.RewardEntry{UnlockedBy: "veteran_driver"}) {
		t.Error("veteran_driver reward should be locked")
	}

	// Battle pass reward (no UnlockedBy)
	if !m.isUnlocked(client.RewardEntry{UnlockedBy: ""}) {
		t.Error("BP reward should be unlocked when battlePassTier > 0")
	}

	// No battle pass tier
	m.battlePassTier = 0
	if m.isUnlocked(client.RewardEntry{UnlockedBy: ""}) {
		t.Error("BP reward should be locked when battlePassTier = 0")
	}
}
