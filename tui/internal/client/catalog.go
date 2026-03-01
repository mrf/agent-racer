package client

// RewardEntry describes a single cosmetic item in the client-side catalog.
// It mirrors the backend's gamification.Reward struct without importing backend packages.
type RewardEntry struct {
	ID         string
	SlotType   string // "paint", "trail", "body", "badge", "sound", "theme", "title"
	Name       string
	UnlockedBy string // achievement ID, or "" for battle-pass rewards
}

// SlotTypes lists the 7 cosmetic slot names in display order.
var SlotTypes = []string{"paint", "trail", "body", "badge", "sound", "theme", "title"}

// Catalog returns the full static reward list, mirroring buildRewardList() in the backend.
func Catalog() []RewardEntry {
	return []RewardEntry{
		// Battle pass tier rewards
		{ID: "bronze_badge", SlotType: "badge", Name: "Bronze Badge"},
		{ID: "spark_trail", SlotType: "trail", Name: "Spark Trail"},
		{ID: "rev_sound", SlotType: "sound", Name: "Rev Sound"},
		{ID: "metallic_paint", SlotType: "paint", Name: "Metallic Paint"},
		{ID: "silver_badge", SlotType: "badge", Name: "Silver Badge"},
		{ID: "flame_trail", SlotType: "trail", Name: "Flame Trail"},
		{ID: "aero_body", SlotType: "body", Name: "Aero Body"},
		{ID: "dark_theme", SlotType: "theme", Name: "Dark Theme"},
		{ID: "champion_title", SlotType: "title", Name: "Champion"},
		{ID: "gold_badge", SlotType: "badge", Name: "Gold Badge"},

		// Achievement rewards — Session Milestones
		{ID: "rookie_paint", SlotType: "paint", Name: "Rookie Paint", UnlockedBy: "first_lap"},
		{ID: "pit_badge", SlotType: "badge", Name: "Pit Badge", UnlockedBy: "pit_crew"},
		{ID: "veteran_title", SlotType: "title", Name: "Veteran", UnlockedBy: "veteran_driver"},
		{ID: "century_paint", SlotType: "paint", Name: "Century Paint", UnlockedBy: "century_club"},
		{ID: "legend_title", SlotType: "title", Name: "Legend", UnlockedBy: "track_legend"},

		// Achievement rewards — Source Diversity
		{ID: "home_trail", SlotType: "trail", Name: "Home Trail", UnlockedBy: "home_turf"},
		{ID: "gemini_paint", SlotType: "paint", Name: "Gemini Paint", UnlockedBy: "gemini_rising"},
		{ID: "codex_paint", SlotType: "paint", Name: "Codex Paint", UnlockedBy: "codex_curious"},
		{ID: "triple_body", SlotType: "body", Name: "Triple Body", UnlockedBy: "triple_threat"},
		{ID: "polyglot_theme", SlotType: "theme", Name: "Polyglot Theme", UnlockedBy: "polyglot"},

		// Achievement rewards — Model Collection
		{ID: "opus_sound", SlotType: "sound", Name: "Opus Sound", UnlockedBy: "opus_enthusiast"},
		{ID: "sonnet_sound", SlotType: "sound", Name: "Sonnet Sound", UnlockedBy: "sonnet_fan"},
		{ID: "haiku_trail", SlotType: "trail", Name: "Haiku Trail", UnlockedBy: "haiku_speedster"},
		{ID: "spectrum_paint", SlotType: "paint", Name: "Full Spectrum Paint", UnlockedBy: "full_spectrum"},
		{ID: "collector_badge", SlotType: "badge", Name: "Collector Badge", UnlockedBy: "model_collector"},
		{ID: "connoisseur_body", SlotType: "body", Name: "Connoisseur Body", UnlockedBy: "connoisseur"},

		// Achievement rewards — Performance & Endurance
		{ID: "redline_trail", SlotType: "trail", Name: "Redline Trail", UnlockedBy: "redline"},
		{ID: "afterburner_sound", SlotType: "sound", Name: "Afterburner Sound", UnlockedBy: "afterburner"},
		{ID: "marathon_title", SlotType: "title", Name: "Marathoner", UnlockedBy: "marathon"},
		{ID: "tool_fiend_body", SlotType: "body", Name: "Tool Fiend Body", UnlockedBy: "tool_fiend"},
		{ID: "clean_sweep_paint", SlotType: "paint", Name: "Clean Sweep Paint", UnlockedBy: "clean_sweep"},

		// Achievement rewards — Spectacle
		{ID: "grid_badge", SlotType: "badge", Name: "Grid Badge", UnlockedBy: "grid_start"},
		{ID: "full_grid_theme", SlotType: "theme", Name: "Full Grid Theme", UnlockedBy: "full_grid"},
		{ID: "crash_survivor_trail", SlotType: "trail", Name: "Survivor Trail", UnlockedBy: "crash_survivor"},
		{ID: "burning_rubber_sound", SlotType: "sound", Name: "Burning Rubber Sound", UnlockedBy: "burning_rubber"},

		// Achievement rewards — Streaks
		{ID: "hat_trick_badge", SlotType: "badge", Name: "Hat Trick Badge", UnlockedBy: "hat_trick"},
		{ID: "on_a_roll_trail", SlotType: "trail", Name: "On a Roll Trail", UnlockedBy: "on_a_roll"},
		{ID: "untouchable_title", SlotType: "title", Name: "Untouchable", UnlockedBy: "untouchable"},
	}
}

// RewardsBySlot returns a map from slot type to the rewards in that slot.
func RewardsBySlot() map[string][]RewardEntry {
	out := make(map[string][]RewardEntry, len(SlotTypes))
	for _, e := range Catalog() {
		out[e.SlotType] = append(out[e.SlotType], e)
	}
	return out
}

// EquippedSlot returns the reward ID currently equipped in the named slot.
func EquippedSlot(eq Equipped, slot string) string {
	switch slot {
	case "paint":
		return eq.Paint
	case "trail":
		return eq.Trail
	case "body":
		return eq.Body
	case "badge":
		return eq.Badge
	case "sound":
		return eq.Sound
	case "theme":
		return eq.Theme
	case "title":
		return eq.Title
	}
	return ""
}
