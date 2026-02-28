package gamification

import (
	"fmt"
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

func TestTierRewards_AllTiersHaveDefinedCase(t *testing.T) {
	// Verify every tier from 1 to maxTiers has an explicit case in tierRewards.
	// This test uses a traditional for loop as required.
	for tier := 1; tier <= maxTiers; tier++ {
		r := tierRewards(tier)
		// Tier 1 should return empty, tiers 2+ should return non-nil slice.
		if r == nil {
			t.Errorf("tierRewards(%d) returned nil, want []string (may be empty)", tier)
		}
		if tier == 1 && len(r) != 0 {
			t.Errorf("tierRewards(1) = %v, want empty slice", r)
		}
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

// --- awardXP / getProgress: tier advancement boundary values ---

func TestAwardXP_UniformCostModel(t *testing.T) {
	// Verify each tier costs exactly xpPerTier (uniform, not progressive).
	// Cumulative threshold to reach tier N is (N-1)*xpPerTier.
	for tier := 2; tier <= maxTiers; tier++ {
		threshold := (tier - 1) * xpPerTier

		t.Run(fmt.Sprintf("tier%d/below", tier), func(t *testing.T) {
			bp := &BattlePass{}
			awardXP(bp, threshold-1)
			if bp.Tier != tier-1 {
				t.Errorf("Tier = %d, want %d", bp.Tier, tier-1)
			}
		})

		t.Run(fmt.Sprintf("tier%d/exact", tier), func(t *testing.T) {
			bp := &BattlePass{}
			awardXP(bp, threshold)
			if bp.Tier != tier {
				t.Errorf("Tier = %d, want %d", bp.Tier, tier)
			}
		})

		t.Run(fmt.Sprintf("tier%d/above", tier), func(t *testing.T) {
			bp := &BattlePass{}
			awardXP(bp, threshold+1)
			if bp.Tier != tier {
				t.Errorf("Tier = %d, want %d", bp.Tier, tier)
			}
		})
	}
}

func TestGetProgress_BoundaryPct(t *testing.T) {
	// At tier start: pct should be 0.0.
	// At tier midpoint: pct should be 0.5.
	// One XP below next threshold: pct should be just under 1.0.
	cases := []struct {
		name string
		tier int
		xp   int
		pct  float64
	}{
		{"tier2 start", 2, 1000, 0.0},
		{"tier2 mid", 2, 1500, 0.5},
		{"tier2 end", 2, 1999, 0.999},
		{"tier5 start", 5, 4000, 0.0},
		{"tier5 mid", 5, 4500, 0.5},
		{"tier9 end", 9, 8999, 0.999},
		{"maxTier always 1.0", maxTiers, 9000, 1.0},
		{"maxTier with excess XP", maxTiers, 50000, 1.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bp := &BattlePass{Tier: tc.tier, XP: tc.xp}
			p := getProgress(bp)
			if p.Pct != tc.pct {
				t.Errorf("Pct = %f, want %f", p.Pct, tc.pct)
			}
		})
	}
}

func TestAwardXP_IncrementalAdvancement(t *testing.T) {
	// Simulate earning XP in small increments — verify each tier boundary.
	bp := &BattlePass{}
	for tier := 1; tier <= maxTiers; tier++ {
		threshold := tier * xpPerTier
		// Award XP in 100-XP chunks up to just below the next threshold.
		for bp.XP < threshold-100 {
			awardXP(bp, 100)
		}
		if bp.Tier != tier {
			t.Errorf("At %d XP: Tier = %d, want %d", bp.XP, bp.Tier, tier)
		}
		// The next 100 XP should cross the threshold (unless at max).
		if tier < maxTiers {
			awardXP(bp, 100)
			if bp.Tier != tier+1 {
				t.Errorf("After crossing threshold at %d XP: Tier = %d, want %d",
					bp.XP, bp.Tier, tier+1)
			}
		}
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

// --- BattlePass.UnmarshalJSON migration tests ---

func TestBattlePass_UnmarshalJSON_IntegerSeasonZero(t *testing.T) {
	// Legacy integer season field with value 0 should migrate to empty string
	jsonData := []byte(`{"season": 0, "tier": 3, "xp": 1500}`)
	var bp BattlePass
	if err := bp.UnmarshalJSON(jsonData); err != nil {
		t.Fatalf("UnmarshalJSON error: %v", err)
	}
	if bp.Season != "" {
		t.Errorf("Season = %q, want empty string for legacy 0", bp.Season)
	}
	if bp.Tier != 3 {
		t.Errorf("Tier = %d, want 3", bp.Tier)
	}
	if bp.XP != 1500 {
		t.Errorf("XP = %d, want 1500", bp.XP)
	}
}

func TestBattlePass_UnmarshalJSON_IntegerSeasonOne(t *testing.T) {
	// Legacy integer season field with value 1 should migrate to string "1"
	jsonData := []byte(`{"season": 1, "tier": 2, "xp": 500}`)
	var bp BattlePass
	if err := bp.UnmarshalJSON(jsonData); err != nil {
		t.Fatalf("UnmarshalJSON error: %v", err)
	}
	if bp.Season != "1" {
		t.Errorf("Season = %q, want \"1\"", bp.Season)
	}
	if bp.Tier != 2 {
		t.Errorf("Tier = %d, want 2", bp.Tier)
	}
	if bp.XP != 500 {
		t.Errorf("XP = %d, want 500", bp.XP)
	}
}

func TestBattlePass_UnmarshalJSON_IntegerSeasonSeven(t *testing.T) {
	// Legacy integer season field with value 7 should migrate to string "7"
	jsonData := []byte(`{"season": 7, "tier": 5, "xp": 3000}`)
	var bp BattlePass
	if err := bp.UnmarshalJSON(jsonData); err != nil {
		t.Fatalf("UnmarshalJSON error: %v", err)
	}
	if bp.Season != "7" {
		t.Errorf("Season = %q, want \"7\"", bp.Season)
	}
	if bp.Tier != 5 {
		t.Errorf("Tier = %d, want 5", bp.Tier)
	}
	if bp.XP != 3000 {
		t.Errorf("XP = %d, want 3000", bp.XP)
	}
}

func TestBattlePass_UnmarshalJSON_StringSeasonFormat(t *testing.T) {
	// Modern string season field should be preserved as-is
	jsonData := []byte(`{"season": "2025-07", "tier": 8, "xp": 4500}`)
	var bp BattlePass
	if err := bp.UnmarshalJSON(jsonData); err != nil {
		t.Fatalf("UnmarshalJSON error: %v", err)
	}
	if bp.Season != "2025-07" {
		t.Errorf("Season = %q, want \"2025-07\"", bp.Season)
	}
	if bp.Tier != 8 {
		t.Errorf("Tier = %d, want 8", bp.Tier)
	}
	if bp.XP != 4500 {
		t.Errorf("XP = %d, want 4500", bp.XP)
	}
}

func TestBattlePass_UnmarshalJSON_MalformedSeasonFieldReturnsError(t *testing.T) {
	// Malformed season field (not string or number) should return error
	jsonData := []byte(`{"season": {"invalid": "object"}, "tier": 3, "xp": 1500}`)
	var bp BattlePass
	err := bp.UnmarshalJSON(jsonData)
	if err == nil {
		t.Fatal("UnmarshalJSON should return error for malformed season field")
	}
}

func TestBattlePass_UnmarshalJSON_IntegerSeasonsMultipleValues(t *testing.T) {
	// Test multiple legacy integer values to ensure conversion is consistent
	// Using traditional for loop as specified in requirements
	testCases := []struct {
		jsonSeason     string
		expectedSeason string
	}{
		{"0", ""},
		{"1", "1"},
		{"2", "2"},
		{"5", "5"},
		{"7", "7"},
		{"10", "10"},
	}
	for i := 0; i < len(testCases); i++ {
		tc := testCases[i]
		t.Run(fmt.Sprintf("season_%s", tc.jsonSeason), func(t *testing.T) {
			jsonData := []byte(`{"season": ` + tc.jsonSeason + `, "tier": 1, "xp": 0}`)
			var bp BattlePass
			if err := bp.UnmarshalJSON(jsonData); err != nil {
				t.Fatalf("UnmarshalJSON error: %v", err)
			}
			if bp.Season != tc.expectedSeason {
				t.Errorf("Season = %q, want %q", bp.Season, tc.expectedSeason)
			}
		})
	}
}

func TestBattlePass_UnmarshalJSON_EmptySeasonField(t *testing.T) {
	// Empty season field should be left unchanged (no migration needed)
	jsonData := []byte(`{"season": "", "tier": 2, "xp": 1000}`)
	var bp BattlePass
	if err := bp.UnmarshalJSON(jsonData); err != nil {
		t.Fatalf("UnmarshalJSON error: %v", err)
	}
	if bp.Season != "" {
		t.Errorf("Season = %q, want empty string", bp.Season)
	}
}

func TestBattlePass_UnmarshalJSON_NoSeasonFieldOmitted(t *testing.T) {
	// When season field is omitted from JSON, it should remain empty
	jsonData := []byte(`{"tier": 4, "xp": 2500}`)
	var bp BattlePass
	if err := bp.UnmarshalJSON(jsonData); err != nil {
		t.Fatalf("UnmarshalJSON error: %v", err)
	}
	if bp.Season != "" {
		t.Errorf("Season = %q, want empty string when omitted", bp.Season)
	}
	if bp.Tier != 4 {
		t.Errorf("Tier = %d, want 4", bp.Tier)
	}
}

func TestStatsTracker_AwardXP_FiresCallback(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	tracker, _, err := NewStatsTracker(store, 0, nil)
	if err != nil {
		t.Fatalf("NewStatsTracker error: %v", err)
	}

	var callbackFired bool
	var capturedProgress BattlePassProgress
	var capturedXPEntries []XPEntry

	tracker.OnBattlePassProgress(func(progress BattlePassProgress, xpEntries []XPEntry) {
		callbackFired = true
		capturedProgress = progress
		capturedXPEntries = xpEntries
	})

	tracker.AwardXP(500, "test_reason")

	if !callbackFired {
		t.Fatal("onBattlePass callback did not fire")
	}

	if capturedProgress.XP != 500 {
		t.Errorf("callback progress XP = %d, want 500", capturedProgress.XP)
	}

	if capturedProgress.Tier != 1 {
		t.Errorf("callback progress Tier = %d, want 1", capturedProgress.Tier)
	}

	if len(capturedXPEntries) != 1 {
		t.Errorf("callback received %d XP entries, want 1", len(capturedXPEntries))
	} else {
		if capturedXPEntries[0].Amount != 500 {
			t.Errorf("callback XP entry amount = %d, want 500", capturedXPEntries[0].Amount)
		}
		if capturedXPEntries[0].Reason != "test_reason" {
			t.Errorf("callback XP entry reason = %q, want %q", capturedXPEntries[0].Reason, "test_reason")
		}
	}
}

func TestStatsTracker_AwardXP_CallbackIncludesUpdatedProgress(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	tracker, _, err := NewStatsTracker(store, 0, nil)
	if err != nil {
		t.Fatalf("NewStatsTracker error: %v", err)
	}

	// Award XP up to just before tier advancement.
	tracker.AwardXP(xpPerTier-100, "setup")

	var capturedProgress BattlePassProgress

	tracker.OnBattlePassProgress(func(progress BattlePassProgress, xpEntries []XPEntry) {
		capturedProgress = progress
	})

	// Award XP that will cross the tier threshold.
	tracker.AwardXP(150, "trigger_advance")

	if capturedProgress.Tier != 2 {
		t.Errorf("callback progress Tier = %d, want 2 (tier should advance)", capturedProgress.Tier)
	}

	if capturedProgress.XP != xpPerTier+50 {
		t.Errorf("callback progress XP = %d, want %d", capturedProgress.XP, xpPerTier+50)
	}
}

// --- getProgress pct: tier boundary edge cases ---

func TestGetProgress_PctEdgeCase_ZeroXPTierOne(t *testing.T) {
	// XP = 0, Tier = 1: pct should be 0.0 (just started)
	bp := &BattlePass{Tier: 1, XP: 0}
	p := getProgress(bp)
	if p.Pct != 0.0 {
		t.Errorf("Pct = %f, want 0.0 (zero XP at tier 1)", p.Pct)
	}
}

func TestGetProgress_PctEdgeCase_ExactlyAtTierThreshold(t *testing.T) {
	// XP exactly at tier threshold transitions to new tier.
	// At Tier 2, XP = 1000 (threshold for tier 2), pct should be 0.0
	// (start of the new tier, need 1000 more to reach tier 3).
	bp := &BattlePass{Tier: 2, XP: 1000}
	p := getProgress(bp)
	if p.Pct != 0.0 {
		t.Errorf("Pct = %f, want 0.0 (exactly at tier 2 threshold)", p.Pct)
	}
}

func TestGetProgress_PctEdgeCase_ExactlyAtHigherTierThreshold(t *testing.T) {
	// Verify the pattern holds at higher tiers too.
	// At Tier 5, XP = 4000 (threshold for tier 5), pct should be 0.0
	bp := &BattlePass{Tier: 5, XP: 4000}
	p := getProgress(bp)
	if p.Pct != 0.0 {
		t.Errorf("Pct = %f, want 0.0 (exactly at tier 5 threshold)", p.Pct)
	}
}

func TestGetProgress_PctEdgeCase_JustBelowNextThreshold(t *testing.T) {
	// One XP below the next tier threshold should be just under 1.0
	// At Tier 3, XP = 2999 (one below threshold 3000), pct should be 0.999
	bp := &BattlePass{Tier: 3, XP: 2999}
	p := getProgress(bp)
	if p.Pct != 0.999 {
		t.Errorf("Pct = %f, want 0.999 (one below next threshold)", p.Pct)
	}
}

func TestGetProgress_PctEdgeCase_MaxTierWithMinXP(t *testing.T) {
	// At max tier with minimum XP for that tier, pct should be 1.0
	// (no further advancement possible)
	bp := &BattlePass{Tier: maxTiers, XP: (maxTiers - 1) * xpPerTier}
	p := getProgress(bp)
	if p.Pct != 1.0 {
		t.Errorf("Pct = %f, want 1.0 (at max tier, minimum XP)", p.Pct)
	}
}

func TestGetProgress_PctEdgeCase_MaxTierWithExcessXP(t *testing.T) {
	// At max tier with excess XP beyond threshold, pct should still be 1.0
	// (clamped, no further advancement)
	bp := &BattlePass{Tier: maxTiers, XP: (maxTiers - 1)*xpPerTier + 5000}
	p := getProgress(bp)
	if p.Pct != 1.0 {
		t.Errorf("Pct = %f, want 1.0 (at max tier, excess XP)", p.Pct)
	}
}

func TestGetProgress_PctEdgeCase_NegativeXPClampsToZero(t *testing.T) {
	// Negative XP (edge case for formula validation) should be clamped to 0.0
	// Pct formula: (XP - (tier-1)*xpPerTier) / xpPerTier
	// At Tier 2, XP = 500: (500 - 1000) / 1000 = -0.5 → clamped to 0.0
	bp := &BattlePass{Tier: 2, XP: 500}
	p := getProgress(bp)
	if p.Pct != 0.0 {
		t.Errorf("Pct = %f, want 0.0 (negative calc clamped)", p.Pct)
	}
}

func TestGetProgress_PctEdgeCase_AllTierBoundaries(t *testing.T) {
	// Verify pct = 0.0 at the exact threshold for each tier (except maxTiers).
	// At maxTiers, pct is always 1.0 per the special rule in getProgress.
	// Uses traditional for loop as required.
	for tier := 1; tier <= maxTiers; tier++ {
		threshold := (tier - 1) * xpPerTier
		t.Run(fmt.Sprintf("tier%d_at_threshold", tier), func(t *testing.T) {
			bp := &BattlePass{Tier: tier, XP: threshold}
			p := getProgress(bp)
			if tier == maxTiers {
				// At maxTiers, pct is always 1.0 (no further advancement)
				if p.Pct != 1.0 {
					t.Errorf("Pct = %f, want 1.0 (at max tier)", p.Pct)
				}
			} else {
				// Before maxTiers, pct = 0.0 at the tier start
				if p.Pct != 0.0 {
					t.Errorf("Pct = %f, want 0.0 (at tier %d threshold)", p.Pct, tier)
				}
			}
		})
	}
}

func TestGetProgress_PctEdgeCase_BoundaryClampingRange(t *testing.T) {
	// Verify min/max clamping keeps pct in [0.0, 1.0] across all scenarios
	testCases := []struct {
		name      string
		tier      int
		xp        int
		expectMin float64 // should be >= this
		expectMax float64 // should be <= this
	}{
		{"tier1_zero", 1, 0, 0.0, 1.0},
		{"tier2_negative_calc", 2, 500, 0.0, 1.0},
		{"tier3_midpoint", 3, 2500, 0.0, 1.0},
		{"tier5_just_below", 5, 4999, 0.0, 1.0},
		{"maxTier_any_xp", maxTiers, 50000, 0.0, 1.0},
	}

	for i := 0; i < len(testCases); i++ {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			bp := &BattlePass{Tier: tc.tier, XP: tc.xp}
			p := getProgress(bp)
			if p.Pct < tc.expectMin {
				t.Errorf("Pct = %f, want >= %f (below minimum)", p.Pct, tc.expectMin)
			}
			if p.Pct > tc.expectMax {
				t.Errorf("Pct = %f, want <= %f (above maximum)", p.Pct, tc.expectMax)
			}
		})
	}
}
