package gamification

import (
	"errors"
	"fmt"
)

// RewardType identifies which cosmetic slot a reward occupies.
// Each slot holds at most one active reward at a time.
type RewardType string

const (
	RewardTypePaint RewardType = "paint"
	RewardTypeTrail RewardType = "trail"
	RewardTypeBody  RewardType = "body"
	RewardTypeBadge RewardType = "badge"
	RewardTypeSound RewardType = "sound"
	RewardTypeTheme RewardType = "theme"
	RewardTypeTitle RewardType = "title"
)

// ErrNotUnlocked is returned when equipping a reward the player has not yet earned.
var ErrNotUnlocked = errors.New("reward not unlocked")

// ErrUnknownReward is returned when a reward ID does not exist in the registry.
var ErrUnknownReward = errors.New("unknown reward")

// ErrSlotMismatch is returned when an unequip call receives an unrecognised slot name.
var ErrSlotMismatch = errors.New("unknown slot type")

// Reward describes a single cosmetic item in the registry.
type Reward struct {
	ID   string
	Type RewardType
	Name string
	// UnlockedBy is the achievement ID that grants this reward. An empty string
	// means the reward is granted by the battle pass (see tierRewards in battlepass.go).
	UnlockedBy string
}

// RewardRegistry holds the complete set of cosmetic rewards and provides
// equip and loadout operations against a Stats snapshot.
type RewardRegistry struct {
	rewards map[string]Reward // keyed by Reward.ID
}

// NewRewardRegistry creates a registry pre-loaded with all cosmetic rewards.
func NewRewardRegistry() *RewardRegistry {
	r := &RewardRegistry{rewards: make(map[string]Reward)}
	for _, rw := range buildRewardList() {
		r.rewards[rw.ID] = rw
	}
	return r
}

// Registry returns a copy of all rewards in an unspecified order.
func (r *RewardRegistry) Registry() []Reward {
	out := make([]Reward, 0, len(r.rewards))
	for _, rw := range r.rewards {
		out = append(out, rw)
	}
	return out
}

// IsUnlocked reports whether the player has earned the named reward.
// A reward is earned when its UnlockedBy achievement appears in
// stats.AchievementsUnlocked, or — for battle pass rewards — when the
// player's tier has reached the tier that grants it.
func (r *RewardRegistry) IsUnlocked(rewardID string, stats *Stats) bool {
	rw, ok := r.rewards[rewardID]
	if !ok {
		return false
	}
	if rw.UnlockedBy != "" {
		_, earned := stats.AchievementsUnlocked[rw.UnlockedBy]
		return earned
	}
	// Battle pass reward: find the tier that lists this ID and check progress.
	for tier := 1; tier <= maxTiers; tier++ {
		for _, id := range tierRewards(tier) {
			if id == rewardID {
				return stats.BattlePass.Tier >= tier
			}
		}
	}
	return false
}

// Equip places rewardID into its slot on stats.Equipped.
// It returns ErrUnknownReward if the ID is not in the registry, and
// ErrNotUnlocked if the player has not yet earned it.
// The caller is responsible for persisting stats after a successful call.
func (r *RewardRegistry) Equip(rewardID string, stats *Stats) error {
	rw, ok := r.rewards[rewardID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownReward, rewardID)
	}
	if !r.IsUnlocked(rewardID, stats) {
		return fmt.Errorf("%w: %s", ErrNotUnlocked, rewardID)
	}
	setEquippedSlot(&stats.Equipped, rw.Type, rewardID)
	return nil
}

// Unequip clears the given slot on stats.Equipped. It is a no-op when the
// slot is already empty. It returns ErrSlotMismatch for unrecognised slot names.
// The caller is responsible for persisting stats after a successful call.
func (r *RewardRegistry) Unequip(slot RewardType, stats *Stats) error {
	if !validSlot(slot) {
		return fmt.Errorf("%w: %s", ErrSlotMismatch, slot)
	}
	setEquippedSlot(&stats.Equipped, slot, "")
	return nil
}

// GetEquipped returns the current equipped loadout from stats.
func (r *RewardRegistry) GetEquipped(stats *Stats) Equipped {
	return stats.Equipped
}

// setEquippedSlot writes id into the field of eq that corresponds to slot.
func setEquippedSlot(eq *Equipped, slot RewardType, id string) {
	switch slot {
	case RewardTypePaint:
		eq.Paint = id
	case RewardTypeTrail:
		eq.Trail = id
	case RewardTypeBody:
		eq.Body = id
	case RewardTypeBadge:
		eq.Badge = id
	case RewardTypeSound:
		eq.Sound = id
	case RewardTypeTheme:
		eq.Theme = id
	case RewardTypeTitle:
		eq.Title = id
	}
}

// validSlot reports whether slot is one of the defined RewardType constants.
func validSlot(slot RewardType) bool {
	switch slot {
	case RewardTypePaint, RewardTypeTrail, RewardTypeBody, RewardTypeBadge,
		RewardTypeSound, RewardTypeTheme, RewardTypeTitle:
		return true
	}
	return false
}

// buildRewardList returns the authoritative list of cosmetic rewards.
// Battle pass rewards leave UnlockedBy empty; achievement rewards name the
// achievement ID that grants them.
func buildRewardList() []Reward {
	return []Reward{

		// ── Battle pass tier rewards ───────────────────────────────────────
		// IDs must match the strings returned by tierRewards() in battlepass.go.

		{ID: "bronze_badge", Type: RewardTypeBadge, Name: "Bronze Badge"},    // tier 2
		{ID: "spark_trail", Type: RewardTypeTrail, Name: "Spark Trail"},       // tier 3
		{ID: "rev_sound", Type: RewardTypeSound, Name: "Rev Sound"},           // tier 4
		{ID: "metallic_paint", Type: RewardTypePaint, Name: "Metallic Paint"}, // tier 5
		{ID: "silver_badge", Type: RewardTypeBadge, Name: "Silver Badge"},     // tier 6
		{ID: "flame_trail", Type: RewardTypeTrail, Name: "Flame Trail"},       // tier 7
		{ID: "aero_body", Type: RewardTypeBody, Name: "Aero Body"},            // tier 8
		{ID: "dark_theme", Type: RewardTypeTheme, Name: "Dark Theme"},         // tier 9
		{ID: "champion_title", Type: RewardTypeTitle, Name: "Champion"},       // tier 10
		{ID: "gold_badge", Type: RewardTypeBadge, Name: "Gold Badge"},         // tier 10

		// ── Achievement rewards ────────────────────────────────────────────

		// Session Milestones
		{ID: "rookie_paint", Type: RewardTypePaint, Name: "Rookie Paint", UnlockedBy: "first_lap"},
		{ID: "pit_badge", Type: RewardTypeBadge, Name: "Pit Badge", UnlockedBy: "pit_crew"},
		{ID: "veteran_title", Type: RewardTypeTitle, Name: "Veteran", UnlockedBy: "veteran_driver"},
		{ID: "century_paint", Type: RewardTypePaint, Name: "Century Paint", UnlockedBy: "century_club"},
		{ID: "legend_title", Type: RewardTypeTitle, Name: "Legend", UnlockedBy: "track_legend"},

		// Source Diversity
		{ID: "home_trail", Type: RewardTypeTrail, Name: "Home Trail", UnlockedBy: "home_turf"},
		{ID: "gemini_paint", Type: RewardTypePaint, Name: "Gemini Paint", UnlockedBy: "gemini_rising"},
		{ID: "codex_paint", Type: RewardTypePaint, Name: "Codex Paint", UnlockedBy: "codex_curious"},
		{ID: "triple_body", Type: RewardTypeBody, Name: "Triple Body", UnlockedBy: "triple_threat"},
		{ID: "polyglot_theme", Type: RewardTypeTheme, Name: "Polyglot Theme", UnlockedBy: "polyglot"},

		// Model Collection
		{ID: "opus_sound", Type: RewardTypeSound, Name: "Opus Sound", UnlockedBy: "opus_enthusiast"},
		{ID: "sonnet_sound", Type: RewardTypeSound, Name: "Sonnet Sound", UnlockedBy: "sonnet_fan"},
		{ID: "haiku_trail", Type: RewardTypeTrail, Name: "Haiku Trail", UnlockedBy: "haiku_speedster"},
		{ID: "spectrum_paint", Type: RewardTypePaint, Name: "Full Spectrum Paint", UnlockedBy: "full_spectrum"},
		{ID: "collector_badge", Type: RewardTypeBadge, Name: "Collector Badge", UnlockedBy: "model_collector"},
		{ID: "connoisseur_body", Type: RewardTypeBody, Name: "Connoisseur Body", UnlockedBy: "connoisseur"},

		// Performance & Endurance
		{ID: "redline_trail", Type: RewardTypeTrail, Name: "Redline Trail", UnlockedBy: "redline"},
		{ID: "afterburner_sound", Type: RewardTypeSound, Name: "Afterburner Sound", UnlockedBy: "afterburner"},
		{ID: "marathon_title", Type: RewardTypeTitle, Name: "Marathoner", UnlockedBy: "marathon"},
		{ID: "tool_fiend_body", Type: RewardTypeBody, Name: "Tool Fiend Body", UnlockedBy: "tool_fiend"},
		{ID: "clean_sweep_paint", Type: RewardTypePaint, Name: "Clean Sweep Paint", UnlockedBy: "clean_sweep"},

		// Spectacle
		{ID: "grid_badge", Type: RewardTypeBadge, Name: "Grid Badge", UnlockedBy: "grid_start"},
		{ID: "full_grid_theme", Type: RewardTypeTheme, Name: "Full Grid Theme", UnlockedBy: "full_grid"},
		{ID: "crash_survivor_trail", Type: RewardTypeTrail, Name: "Survivor Trail", UnlockedBy: "crash_survivor"},
		{ID: "burning_rubber_sound", Type: RewardTypeSound, Name: "Burning Rubber Sound", UnlockedBy: "burning_rubber"},

		// Streaks
		{ID: "hat_trick_badge", Type: RewardTypeBadge, Name: "Hat Trick Badge", UnlockedBy: "hat_trick"},
		{ID: "on_a_roll_trail", Type: RewardTypeTrail, Name: "On a Roll Trail", UnlockedBy: "on_a_roll"},
		{ID: "untouchable_title", Type: RewardTypeTitle, Name: "Untouchable", UnlockedBy: "untouchable"},
	}
}
