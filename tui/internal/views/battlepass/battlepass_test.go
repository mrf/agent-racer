package battlepass

import (
	"strings"
	"testing"

	"github.com/agent-racer/tui/internal/client"
)

func TestNew(t *testing.T) {
	m := New()
	if m.Season != "—" {
		t.Errorf("New().Season = %q, want %q", m.Season, "—")
	}
	if !m.loading {
		t.Error("New() should be in loading state")
	}
}

func TestSetLoaded(t *testing.T) {
	m := New()
	m.SetLoaded()
	if m.loading {
		t.Error("SetLoaded should clear loading state")
	}
}

func TestSetProgress(t *testing.T) {
	m := New()
	m.SetLoaded()
	m.SetProgress(client.BattlePassProgressPayload{
		Tier:         5,
		XP:           1200,
		TierProgress: 0.75,
		RecentXP: []client.XPEntry{
			{Reason: "session complete", Amount: 50},
		},
	})
	if m.Tier != 5 {
		t.Errorf("Tier = %d, want 5", m.Tier)
	}
	if m.XP != 1200 {
		t.Errorf("XP = %d, want 1200", m.XP)
	}
	if m.TierProgress != 0.75 {
		t.Errorf("TierProgress = %f, want 0.75", m.TierProgress)
	}
	if len(m.RecentXP) != 1 {
		t.Errorf("RecentXP len = %d, want 1", len(m.RecentXP))
	}
}

func TestSetProgress_EmptyRecentXP(t *testing.T) {
	m := New()
	m.SetLoaded()
	m.RecentXP = []client.XPEntry{{Reason: "old", Amount: 10}}
	m.SetProgress(client.BattlePassProgressPayload{
		Tier:         3,
		XP:           500,
		TierProgress: 0.5,
		RecentXP:     nil, // empty
	})
	// Should preserve existing RecentXP when payload has none
	if len(m.RecentXP) != 1 {
		t.Errorf("empty RecentXP in payload should preserve existing, got len %d", len(m.RecentXP))
	}
}

func TestSetChallenges(t *testing.T) {
	m := New()
	cs := []client.ChallengeProgress{
		{ID: "c1", Description: "Run 5 sessions", Current: 3, Target: 5},
	}
	m.SetChallenges(cs)
	if len(m.Challenges) != 1 {
		t.Errorf("Challenges len = %d, want 1", len(m.Challenges))
	}
}

func TestSetFromStats(t *testing.T) {
	m := New()
	m.SetLoaded()
	m.SetFromStats("Season 1", 3, 800)
	if m.Season != "Season 1" {
		t.Errorf("Season = %q, want %q", m.Season, "Season 1")
	}
	if m.Tier != 3 {
		t.Errorf("Tier = %d, want 3", m.Tier)
	}
	if m.XP != 800 {
		t.Errorf("XP = %d, want 800", m.XP)
	}
}

func TestSetFromStats_DoesNotOverwriteExisting(t *testing.T) {
	m := New()
	m.SetLoaded()
	m.SetProgress(client.BattlePassProgressPayload{
		Tier: 5,
		XP:   1200,
	})

	// Stats should not overwrite WS-set values
	m.SetFromStats("Season 1", 3, 800)
	if m.Tier != 5 {
		t.Errorf("Tier should not be overwritten, got %d", m.Tier)
	}
	if m.XP != 1200 {
		t.Errorf("XP should not be overwritten, got %d", m.XP)
	}
}

func TestSetFromStats_EmptySeason(t *testing.T) {
	m := New()
	m.SetLoaded()
	m.Season = "Existing"
	m.SetFromStats("", 3, 800)
	if m.Season != "Existing" {
		t.Errorf("empty season should not overwrite, got %q", m.Season)
	}
}

func TestCollapsedBar_Loading(t *testing.T) {
	m := New()
	m.Width = 80
	view := m.CollapsedBar()
	if !strings.Contains(view, "Loading battle pass") {
		t.Error("loading collapsed bar should show 'Loading battle pass'")
	}
}

func TestCollapsedBar_WithData(t *testing.T) {
	m := New()
	m.SetLoaded()
	m.Width = 80
	m.Season = "Season 1"
	m.Tier = 5
	m.XP = 1200
	m.TierProgress = 0.6

	view := m.CollapsedBar()
	if !strings.Contains(view, "Tier 5") {
		t.Error("collapsed bar should show 'Tier 5'")
	}
	if !strings.Contains(view, "Season 1") {
		t.Error("collapsed bar should show season name")
	}
	if !strings.Contains(view, "60%") {
		t.Error("collapsed bar should show percentage")
	}
	if !strings.Contains(view, "1200 XP") {
		t.Error("collapsed bar should show XP")
	}
	if !strings.Contains(view, "[b] expand") {
		t.Error("collapsed bar should show expand hint")
	}
}

func TestCollapsedBar_NarrowWidth(t *testing.T) {
	m := New()
	m.SetLoaded()
	m.Width = 20 // below 40 threshold
	m.Tier = 1
	// Should not panic and should default to 80
	view := m.CollapsedBar()
	if view == "" {
		t.Error("narrow collapsed bar should not be empty")
	}
}

func TestView_Loading(t *testing.T) {
	m := New()
	m.Width = 80
	view := m.View()
	if !strings.Contains(view, "BATTLE PASS") {
		t.Error("loading view should show title")
	}
	if !strings.Contains(view, "Loading") {
		t.Error("loading view should show 'Loading'")
	}
}

func TestView_WithData(t *testing.T) {
	m := New()
	m.SetLoaded()
	m.Width = 80
	m.Season = "Season 1"
	m.Tier = 5
	m.XP = 1200
	m.TierProgress = 0.6
	m.RecentXP = []client.XPEntry{
		{Reason: "session done", Amount: 50},
		{Reason: "challenge", Amount: 100},
	}
	m.Challenges = []client.ChallengeProgress{
		{ID: "c1", Description: "Run 5 sessions", Current: 3, Target: 5},
	}

	view := m.View()
	if !strings.Contains(view, "BATTLE PASS") {
		t.Error("should show title")
	}
	if !strings.Contains(view, "Season 1") {
		t.Error("should show season name")
	}
	if !strings.Contains(view, "1200 XP total") {
		t.Error("should show XP total")
	}
	if !strings.Contains(view, "Weekly Challenges") {
		t.Error("should show challenges header")
	}
	if !strings.Contains(view, "Run 5 sessions") {
		t.Error("should show challenge description")
	}
	if !strings.Contains(view, "Recent XP") {
		t.Error("should show recent XP header")
	}
	if !strings.Contains(view, "session done") {
		t.Error("should show XP reason")
	}
	if !strings.Contains(view, "[esc] close") {
		t.Error("should show dismiss hint")
	}
}

func TestView_NoChallenges(t *testing.T) {
	m := New()
	m.SetLoaded()
	m.Width = 80
	m.Tier = 1

	view := m.View()
	if !strings.Contains(view, "No active challenges") {
		t.Error("should show 'No active challenges' when empty")
	}
}

func TestView_NoRecentXP(t *testing.T) {
	m := New()
	m.SetLoaded()
	m.Width = 80
	m.Tier = 1

	view := m.View()
	if !strings.Contains(view, "No recent XP") {
		t.Error("should show 'No recent XP' when empty")
	}
}

func TestRenderBar(t *testing.T) {
	if got := renderBar(0, 0); got != "[]" {
		t.Errorf("renderBar(0, 0) = %q, want %q", got, "[]")
	}
	if got := renderBar(3, 10); !strings.Contains(got, "[") || !strings.Contains(got, "]") {
		t.Error("renderBar(3, 10) should have brackets")
	}
}

func TestRenderTierTrack(t *testing.T) {
	for _, tier := range []int{0, 1, 5} {
		if got := renderTierTrack(tier); got == "" {
			t.Errorf("renderTierTrack(%d) should not be empty", tier)
		}
	}
}
