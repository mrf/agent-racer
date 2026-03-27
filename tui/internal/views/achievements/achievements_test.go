package achievements

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/agent-racer/tui/internal/client"
	tea "github.com/charmbracelet/bubbletea"
)

func sampleItems() []client.AchievementResponse {
	now := time.Now()
	return []client.AchievementResponse{
		{ID: "first_lap", Name: "First Lap", Description: "Complete 1 session", Tier: "bronze", Category: "Session Milestones", Unlocked: true, UnlockedAt: &now},
		{ID: "pit_crew", Name: "Pit Crew", Description: "Have 3 sessions in pit", Tier: "silver", Category: "Session Milestones", Unlocked: false},
		{ID: "home_turf", Name: "Home Turf", Description: "Use one source", Tier: "bronze", Category: "Source Diversity", Unlocked: false},
		{ID: "opus_enth", Name: "Opus Enthusiast", Description: "Use opus", Tier: "gold", Category: "Model Collection", Unlocked: true, UnlockedAt: &now},
	}
}

func TestNew(t *testing.T) {
	m := New()
	if !m.loading {
		t.Error("New() should be in loading state")
	}
	if m.activeTab != 0 {
		t.Error("New() should start on tab 0")
	}
}

func TestApplyLoaded_Success(t *testing.T) {
	m := New()
	items := sampleItems()
	m.ApplyLoaded(LoadedMsg{Items: items})
	if m.loading {
		t.Error("loading should be false after ApplyLoaded")
	}
	if len(m.items) != len(items) {
		t.Errorf("expected %d items, got %d", len(items), len(m.items))
	}
	if m.fetchErr != "" {
		t.Errorf("expected no error, got %q", m.fetchErr)
	}
}

func TestApplyLoaded_Error(t *testing.T) {
	m := New()
	m.ApplyLoaded(LoadedMsg{Err: errors.New("fetch failed")})
	if m.loading {
		t.Error("loading should be false after ApplyLoaded with error")
	}
	if m.fetchErr != "fetch failed" {
		t.Errorf("expected 'fetch failed', got %q", m.fetchErr)
	}
}

func TestApplyUnlock(t *testing.T) {
	m := New()
	m.ApplyLoaded(LoadedMsg{Items: sampleItems()})

	m.ApplyUnlock("pit_crew")
	found := findItem(m.items, "pit_crew")
	if found == nil {
		t.Fatal("pit_crew not found in items")
	}
	if !found.Unlocked {
		t.Error("pit_crew should be unlocked after ApplyUnlock")
	}
	if found.UnlockedAt == nil {
		t.Error("pit_crew should have UnlockedAt set")
	}
}

func findItem(items []client.AchievementResponse, id string) *client.AchievementResponse {
	for i := 0; i < len(items); i++ {
		if items[i].ID == id {
			return &items[i]
		}
	}
	return nil
}

func TestApplyUnlock_AlreadyUnlocked(t *testing.T) {
	m := New()
	m.ApplyLoaded(LoadedMsg{Items: sampleItems()})

	origTime := m.items[0].UnlockedAt
	m.ApplyUnlock("first_lap")
	if m.items[0].UnlockedAt != origTime {
		t.Error("should preserve original UnlockedAt for already-unlocked items")
	}
}

func TestApplyUnlock_UnknownID(t *testing.T) {
	m := New()
	m.ApplyLoaded(LoadedMsg{Items: sampleItems()})
	m.ApplyUnlock("nonexistent") // must not panic
}

func TestUpdate_TabNavigation(t *testing.T) {
	m := New()
	m.ApplyLoaded(LoadedMsg{Items: sampleItems()})

	// Right arrow
	m = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if m.activeTab != 1 {
		t.Errorf("expected tab 1 after right, got %d", m.activeTab)
	}

	// Left arrow
	m = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	if m.activeTab != 0 {
		t.Errorf("expected tab 0 after left, got %d", m.activeTab)
	}

	// Left at 0 should stay at 0
	m = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	if m.activeTab != 0 {
		t.Error("left at tab 0 should stay at 0")
	}

	// Tab wraps around
	m.activeTab = len(categories) - 1
	m = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.activeTab != 0 {
		t.Errorf("tab at last category should wrap to 0, got %d", m.activeTab)
	}
}

func TestUpdate_TabResetsScroll(t *testing.T) {
	m := New()
	m.ApplyLoaded(LoadedMsg{Items: sampleItems()})
	m.scroll = 5

	m = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if m.scroll != 0 {
		t.Error("switching tabs should reset scroll to 0")
	}
}

func TestUpdate_ScrollUpDown(t *testing.T) {
	m := New()
	m.ApplyLoaded(LoadedMsg{Items: sampleItems()})

	m = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.scroll != 1 {
		t.Errorf("expected scroll 1 after j, got %d", m.scroll)
	}

	m = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if m.scroll != 0 {
		t.Errorf("expected scroll 0 after k, got %d", m.scroll)
	}

	// Can't scroll above 0
	m = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if m.scroll != 0 {
		t.Error("scroll should not go below 0")
	}
}

func TestUpdate_RightAtMax(t *testing.T) {
	m := New()
	m.activeTab = len(categories) - 1

	m = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if m.activeTab != len(categories)-1 {
		t.Error("right at last tab should stay at last")
	}
}

func TestViewOverlay_Loading(t *testing.T) {
	m := New()
	view := m.ViewOverlay(100, 40)
	if !strings.Contains(view, "Loading") {
		t.Error("loading view should contain 'Loading'")
	}
	if !strings.Contains(view, "ACHIEVEMENTS") {
		t.Error("loading view should contain title")
	}
}

func TestViewOverlay_Error(t *testing.T) {
	m := New()
	m.ApplyLoaded(LoadedMsg{Err: errors.New("network error")})
	view := m.ViewOverlay(100, 40)
	if !strings.Contains(view, "network error") {
		t.Error("error view should contain error message")
	}
}

func TestViewOverlay_WithItems(t *testing.T) {
	m := New()
	m.ApplyLoaded(LoadedMsg{Items: sampleItems()})
	view := m.ViewOverlay(100, 40)

	if !strings.Contains(view, "ACHIEVEMENTS") {
		t.Error("should contain title")
	}
	// First tab is "Session Milestones" with 2 items, 1 unlocked
	if !strings.Contains(view, "1 / 2 unlocked") {
		t.Error("should show unlock count for Session Milestones")
	}
	if !strings.Contains(view, "First Lap") {
		t.Error("should show first_lap achievement name")
	}
	if !strings.Contains(view, "Pit Crew") {
		t.Error("should show pit_crew achievement name")
	}
}

func TestViewOverlay_EmptyCategory(t *testing.T) {
	m := New()
	m.ApplyLoaded(LoadedMsg{Items: sampleItems()})
	// "Performance & Endurance" has no items in our sample
	m.activeTab = 3
	view := m.ViewOverlay(100, 40)
	if !strings.Contains(view, "No achievements in this category") {
		t.Error("empty category should show no-achievements message")
	}
}

func TestViewOverlay_HelpRow(t *testing.T) {
	m := New()
	m.ApplyLoaded(LoadedMsg{Items: sampleItems()})
	view := m.ViewOverlay(100, 40)
	if !strings.Contains(view, "esc close") {
		t.Error("should contain help row with esc close")
	}
}

func TestFilterByCategory(t *testing.T) {
	items := sampleItems()
	filtered := filterByCategory(items, "Session Milestones")
	if len(filtered) != 2 {
		t.Errorf("expected 2 Session Milestones items, got %d", len(filtered))
	}
	filtered = filterByCategory(items, "Nonexistent")
	if len(filtered) != 0 {
		t.Errorf("expected 0 for nonexistent category, got %d", len(filtered))
	}
}

func TestCountUnlocked(t *testing.T) {
	items := sampleItems()
	milestones := filterByCategory(items, "Session Milestones")
	n := countUnlocked(milestones)
	if n != 1 {
		t.Errorf("expected 1 unlocked milestone, got %d", n)
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		s      string
		maxLen int
		want   string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is too long", 10, "this is..."},
		{"ab", 0, "ab"},
		{"abc", 3, "abc"},
		{"abcd", 3, "abc"},
	}
	for _, tt := range tests {
		got := truncate(tt.s, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.maxLen, got, tt.want)
		}
	}
}

func TestClamp(t *testing.T) {
	tests := []struct {
		v, lo, hi int
		want      int
	}{
		{5, 0, 10, 5},
		{-1, 0, 10, 0},
		{15, 0, 10, 10},
		{0, 0, 0, 0},
	}
	for _, tt := range tests {
		got := clamp(tt.v, tt.lo, tt.hi)
		if got != tt.want {
			t.Errorf("clamp(%d, %d, %d) = %d, want %d", tt.v, tt.lo, tt.hi, got, tt.want)
		}
	}
}

func TestTabShortName(t *testing.T) {
	if got := tabShortName("Performance & Endurance"); got != "Perf & Endurance" {
		t.Errorf("expected 'Perf & Endurance', got %q", got)
	}
	if got := tabShortName("Streaks"); got != "Streaks" {
		t.Errorf("expected 'Streaks', got %q", got)
	}
}
