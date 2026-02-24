package gamification

import (
	"testing"
)

// --- awardXP ---

func TestAwardXP_EachAwardType(t *testing.T) {
	awards := []struct {
		name   string
		amount int
	}{
		{"XPSessionObserved", XPSessionObserved},
		{"XPSessionCompletes", XPSessionCompletes},
		{"XPContext50Pct", XPContext50Pct},
		{"XPContext90Pct", XPContext90Pct},
		{"XPNewModel", XPNewModel},
		{"XPNewSource", XPNewSource},
		{"XPWeeklyChallenge", XPWeeklyChallenge},
	}
	for _, tc := range awards {
		t.Run(tc.name, func(t *testing.T) {
			bp := &BattlePass{}
			awardXP(bp, tc.amount)
			if bp.XP != tc.amount {
				t.Errorf("XP = %d, want %d", bp.XP, tc.amount)
			}
		})
	}
}

func TestAwardXP_Accumulates(t *testing.T) {
	bp := &BattlePass{}
	awardXP(bp, XPSessionObserved)
	awardXP(bp, XPSessionCompletes)
	awardXP(bp, XPNewModel)
	want := XPSessionObserved + XPSessionCompletes + XPNewModel
	if bp.XP != want {
		t.Errorf("XP = %d, want %d", bp.XP, want)
	}
}

func TestAwardXP_ExactThresholdAdvancesTier(t *testing.T) {
	bp := &BattlePass{}
	awardXP(bp, xpPerTier)
	if bp.Tier != 2 {
		t.Errorf("Tier = %d, want 2 (at exactly %d XP)", bp.Tier, xpPerTier)
	}
	if bp.XP != xpPerTier {
		t.Errorf("XP = %d, want %d", bp.XP, xpPerTier)
	}
}

func TestAwardXP_BelowThresholdDoesNotAdvance(t *testing.T) {
	bp := &BattlePass{}
	awardXP(bp, xpPerTier-1)
	if bp.Tier != 1 {
		t.Errorf("Tier = %d, want 1 (at %d XP, below threshold %d)", bp.Tier, bp.XP, xpPerTier)
	}
}

func TestAwardXP_EachTierBoundary(t *testing.T) {
	for tier := 2; tier <= maxTiers; tier++ {
		bp := &BattlePass{}
		threshold := (tier - 1) * xpPerTier
		awardXP(bp, threshold)
		if bp.Tier != tier {
			t.Errorf("At %d XP: Tier = %d, want %d", threshold, bp.Tier, tier)
		}
	}
}

func TestAwardXP_SetsMinTierToOne(t *testing.T) {
	bp := &BattlePass{}
	awardXP(bp, 1)
	if bp.Tier != 1 {
		t.Errorf("Tier = %d, want 1 (minimum tier after any award)", bp.Tier)
	}
}

// --- getProgress ---

func TestGetProgress_TierAndXP(t *testing.T) {
	bp := &BattlePass{Tier: 3, XP: 2500}
	p := getProgress(bp)
	if p.Tier != 3 {
		t.Errorf("Tier = %d, want 3", p.Tier)
	}
	if p.XP != 2500 {
		t.Errorf("XP = %d, want 2500", p.XP)
	}
}

func TestGetProgress_PctWithinTier(t *testing.T) {
	bp := &BattlePass{Tier: 3, XP: 2500}
	p := getProgress(bp)
	if p.Pct != 0.5 {
		t.Errorf("Pct = %f, want 0.5", p.Pct)
	}
}

func TestGetProgress_PctAtTierStart(t *testing.T) {
	bp := &BattlePass{Tier: 2, XP: 1000}
	p := getProgress(bp)
	if p.Pct != 0.0 {
		t.Errorf("Pct = %f, want 0.0", p.Pct)
	}
}

func TestGetProgress_PctAtMaxTier(t *testing.T) {
	bp := &BattlePass{Tier: maxTiers, XP: (maxTiers - 1) * xpPerTier}
	p := getProgress(bp)
	if p.Pct != 1.0 {
		t.Errorf("Pct = %f, want 1.0 at max tier", p.Pct)
	}
}

func TestGetProgress_RewardsMatchTier(t *testing.T) {
	bp := &BattlePass{Tier: 5, XP: 4000}
	p := getProgress(bp)
	want := tierRewards(5)
	if len(p.Rewards) != len(want) {
		t.Fatalf("Rewards len = %d, want %d", len(p.Rewards), len(want))
	}
	for i, r := range p.Rewards {
		if r != want[i] {
			t.Errorf("Rewards[%d] = %q, want %q", i, r, want[i])
		}
	}
}

// --- tierRewards ---

func TestTierRewards_Tier1_Empty(t *testing.T) {
	r := tierRewards(1)
	if len(r) != 0 {
		t.Errorf("tierRewards(1) = %v, want empty", r)
	}
}

func TestTierRewards_Tier10_HasTwoRewards(t *testing.T) {
	r := tierRewards(10)
	if len(r) != 2 {
		t.Fatalf("tierRewards(10) len = %d, want 2", len(r))
	}
	if r[0] != "champion_title" {
		t.Errorf("tierRewards(10)[0] = %q, want champion_title", r[0])
	}
	if r[1] != "gold_badge" {
		t.Errorf("tierRewards(10)[1] = %q, want gold_badge", r[1])
	}
}

// --- seasonReset ---

func TestSeasonReset_ClearsXPAndTier_PreservesAchievements(t *testing.T) {
	stats := newStats()
	stats.BattlePass = BattlePass{Season: "s1", Tier: 5, XP: 4500}
	stats.AchievementsUnlocked["first_session"] = stats.LastUpdated

	stats.BattlePass = BattlePass{Season: "s2"}

	if stats.BattlePass.XP != 0 {
		t.Errorf("XP = %d, want 0 after season reset", stats.BattlePass.XP)
	}
	if stats.BattlePass.Tier != 0 {
		t.Errorf("Tier = %d, want 0 after season reset", stats.BattlePass.Tier)
	}
	if stats.BattlePass.Season != "s2" {
		t.Errorf("Season = %s, want s2", stats.BattlePass.Season)
	}
	if _, ok := stats.AchievementsUnlocked["first_session"]; !ok {
		t.Error("Achievements should be preserved across season reset")
	}
}

// --- Edge cases ---

func TestAwardXP_MaxTierReached_NoFurtherAdvancement(t *testing.T) {
	bp := &BattlePass{}
	awardXP(bp, (maxTiers-1)*xpPerTier)
	if bp.Tier != maxTiers {
		t.Fatalf("Tier = %d, want %d (max)", bp.Tier, maxTiers)
	}

	awardXP(bp, 5000)
	if bp.Tier != maxTiers {
		t.Errorf("Tier = %d after extra XP, want %d (capped at max)", bp.Tier, maxTiers)
	}
	wantXP := (maxTiers-1)*xpPerTier + 5000
	if bp.XP != wantXP {
		t.Errorf("XP = %d, want %d (should still accumulate past max tier)", bp.XP, wantXP)
	}
}

func TestAwardXP_ZeroXP(t *testing.T) {
	bp := &BattlePass{}
	awardXP(bp, 0)
	if bp.XP != 0 {
		t.Errorf("XP = %d, want 0", bp.XP)
	}
	if bp.Tier != 1 {
		t.Errorf("Tier = %d, want 1 (min floor even with zero XP)", bp.Tier)
	}
}

func TestAwardXP_MultipleTierAdvancementInSingleAward(t *testing.T) {
	bp := &BattlePass{}
	awardXP(bp, 5*xpPerTier)
	if bp.Tier != 6 {
		t.Errorf("Tier = %d, want 6 (multiple tiers in one award)", bp.Tier)
	}
}

func TestAwardXP_MultipleTierSkipToMax(t *testing.T) {
	bp := &BattlePass{}
	awardXP(bp, 100*xpPerTier)
	if bp.Tier != maxTiers {
		t.Errorf("Tier = %d, want %d (capped at max)", bp.Tier, maxTiers)
	}
	if bp.XP != 100*xpPerTier {
		t.Errorf("XP = %d, want %d", bp.XP, 100*xpPerTier)
	}
}

func TestGetProgress_ZeroTier_ClampsToOne(t *testing.T) {
	bp := &BattlePass{Tier: 0, XP: 0}
	p := getProgress(bp)
	if p.Tier != 1 {
		t.Errorf("Tier = %d, want 1 (clamped from 0)", p.Tier)
	}
}

// --- StatsTracker.AwardXP / GetProgress ---

func TestStatsTracker_AwardXP_AndGetProgress(t *testing.T) {
	tracker, _ := startTracker(t)

	tracker.AwardXP(500, "test")
	p := tracker.GetProgress()
	if p.XP != 500 {
		t.Errorf("XP = %d, want 500", p.XP)
	}
	if p.Tier != 1 {
		t.Errorf("Tier = %d, want 1", p.Tier)
	}

	tracker.AwardXP(500, "test")
	p = tracker.GetProgress()
	if p.XP != 1000 {
		t.Errorf("XP = %d, want 1000", p.XP)
	}
	if p.Tier != 2 {
		t.Errorf("Tier = %d, want 2 after reaching threshold", p.Tier)
	}
}

func TestStatsTracker_AwardXP_SetsDirtyFlag(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	tracker, _, err := NewStatsTracker(store, 0, nil)
	if err != nil {
		t.Fatalf("NewStatsTracker error: %v", err)
	}

	tracker.AwardXP(100, "test")

	tracker.mu.Lock()
	dirty := tracker.dirty
	tracker.mu.Unlock()

	if !dirty {
		t.Error("dirty flag should be set after AwardXP")
	}
}
