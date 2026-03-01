package gamification

// XP award amounts for various in-session milestones.
const (
	xpPerTier = 1000
	maxTiers  = 10

	XPSessionObserved  = 10
	XPSessionCompletes = 25
	XPContext50Pct     = 15
	XPContext90Pct     = 30
	XPNewModel         = 50
	XPNewSource        = 100
	XPWeeklyChallenge  = 150
)

// AchievementXP returns the XP award for unlocking an achievement of the given tier.
func AchievementXP(tier Tier) int {
	switch tier {
	case TierBronze:
		return 50
	case TierSilver:
		return 100
	case TierGold:
		return 150
	case TierPlatinum:
		return 200
	default:
		return 50
	}
}

// BattlePassProgress describes the player's current position within the season.
type BattlePassProgress struct {
	Tier    int      `json:"tier"`
	XP      int      `json:"xp"`
	Pct     float64  `json:"pct"`     // progress within current tier, 0.0–1.0
	Rewards []string `json:"rewards"` // cosmetic rewards unlocked at current tier
}

// awardXP adds amount to bp.XP and advances the tier as thresholds are crossed.
//
// Tier cost model (uniform): every tier costs exactly xpPerTier XP.
// The cumulative XP threshold to reach tier N is (N-1)*xpPerTier:
//
//	tier 2 at 1000, tier 3 at 2000, … tier 10 at 9000.
//
// The loop compares cumulative XP against bp.Tier*xpPerTier because bp.Tier
// is the current tier before increment — so the threshold for advancing FROM
// tier T is T*xpPerTier (i.e., the cumulative cost of the first T tiers).
//
// It is called inside the StatsTracker mutex; callers must not acquire it again.
func awardXP(bp *BattlePass, amount int) {
	bp.XP += amount
	bp.Tier = max(bp.Tier, 1)
	for bp.Tier < maxTiers && bp.XP >= bp.Tier*xpPerTier {
		bp.Tier++
	}
}

// getProgress computes the display-ready progress for bp.
//
// Within-tier progress: (XP - cumulativeThreshold) / xpPerTier, where
// cumulativeThreshold = (tier-1)*xpPerTier. This gives 0.0 at the start
// of the tier and 1.0 at the threshold for the next tier. At maxTiers,
// progress is always 1.0 (tier is complete, no further advancement).
func getProgress(bp *BattlePass) BattlePassProgress {
	tier := max(min(bp.Tier, maxTiers), 1)

	pct := 1.0
	if tier < maxTiers {
		pct = min(max(float64(bp.XP-(tier-1)*xpPerTier)/float64(xpPerTier), 0), 1)
	}

	return BattlePassProgress{
		Tier:    tier,
		XP:      bp.XP,
		Pct:     pct,
		Rewards: tierRewards(tier),
	}
}

// tierRewards returns the cosmetic reward names unlocked at the given tier.
// Tier 1 is the entry tier with no rewards. Tiers 2-10 each unlock cosmetic rewards.
func tierRewards(tier int) []string {
	switch tier {
	case 1:
		return []string{}
	case 2:
		return []string{"bronze_badge"}
	case 3:
		return []string{"spark_trail"}
	case 4:
		return []string{"rev_sound"}
	case 5:
		return []string{"metallic_paint"}
	case 6:
		return []string{"silver_badge"}
	case 7:
		return []string{"flame_trail"}
	case 8:
		return []string{"aero_body"}
	case 9:
		return []string{"dark_theme"}
	case 10:
		return []string{"champion_title", "gold_badge"}
	default:
		return []string{}
	}
}

// AwardXP adds XP and advances tiers. Safe for concurrent callers
// outside the event-processing loop (e.g. the achievement engine).
func (t *StatsTracker) AwardXP(amount int, reason string) {
	t.mu.Lock()
	awardXP(&t.stats.BattlePass, amount)
	t.dirty = true
	// Capture progress while still holding the lock.
	progress := getProgress(&t.stats.BattlePass)
	xpEntries := []XPEntry{{Reason: reason, Amount: amount}}
	t.mu.Unlock()

	// Fire the callback outside the lock to avoid holding it during broadcast.
	if t.onBattlePass != nil {
		t.onBattlePass(progress, xpEntries)
	}
}

// GetProgress returns a snapshot of the current battle pass progress.
func (t *StatsTracker) GetProgress() BattlePassProgress {
	t.mu.Lock()
	defer t.mu.Unlock()
	return getProgress(&t.stats.BattlePass)
}
