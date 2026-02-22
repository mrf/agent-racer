package gamification

import (
	"errors"
	"testing"
	"time"
)

func achievementIDs() map[string]bool {
	engine := NewAchievementEngine()
	ids := make(map[string]bool)
	for _, a := range engine.Registry() {
		ids[a.ID] = true
	}
	return ids
}

func battlePassRewardIDs() map[string]bool {
	ids := make(map[string]bool)
	for tier := 1; tier <= maxTiers; tier++ {
		for _, id := range tierRewards(tier) {
			ids[id] = true
		}
	}
	return ids
}

// unlockAllSlotAchievements unlocks one achievement per reward slot on stats,
// allowing one reward of each type to be equipped.
func unlockAllSlotAchievements(stats *Stats) {
	for _, id := range []string{
		"first_lap",       // rookie_paint  (paint)
		"home_turf",       // home_trail    (trail)
		"triple_threat",   // triple_body   (body)
		"grid_start",      // grid_badge    (badge)
		"opus_enthusiast", // opus_sound    (sound)
		"polyglot",        // polyglot_theme(theme)
		"veteran_driver",  // veteran_title (title)
	} {
		stats.AchievementsUnlocked[id] = time.Now()
	}
}

func TestRegistry_AllRewardsRegistered(t *testing.T) {
	reg := NewRewardRegistry()
	rewards := reg.Registry()

	wantIDs := make(map[string]bool)
	for _, rw := range buildRewardList() {
		wantIDs[rw.ID] = true
	}

	gotIDs := make(map[string]bool)
	for _, rw := range rewards {
		gotIDs[rw.ID] = true
	}

	for id := range wantIDs {
		if !gotIDs[id] {
			t.Errorf("reward %q from buildRewardList() missing in Registry()", id)
		}
	}
	if len(gotIDs) != len(wantIDs) {
		t.Errorf("Registry() returned %d rewards, buildRewardList() has %d", len(gotIDs), len(wantIDs))
	}
}

func TestRegistry_NoDuplicateIDs(t *testing.T) {
	seen := make(map[string]bool)
	for _, rw := range buildRewardList() {
		if seen[rw.ID] {
			t.Errorf("duplicate reward ID: %q", rw.ID)
		}
		seen[rw.ID] = true
	}
}

func TestRegistry_AchievementRewardsLinkToValidIDs(t *testing.T) {
	achIDs := achievementIDs()

	for _, rw := range buildRewardList() {
		if rw.UnlockedBy == "" {
			continue // battle pass reward
		}
		if !achIDs[rw.UnlockedBy] {
			t.Errorf("reward %q references achievement %q which does not exist", rw.ID, rw.UnlockedBy)
		}
	}
}

func TestRegistry_BattlePassRewardsHaveEmptyUnlockedBy(t *testing.T) {
	bpIDs := battlePassRewardIDs()
	reg := NewRewardRegistry()

	for _, rw := range reg.Registry() {
		if bpIDs[rw.ID] && rw.UnlockedBy != "" {
			t.Errorf("battle pass reward %q should have empty UnlockedBy, got %q", rw.ID, rw.UnlockedBy)
		}
	}
}

func TestRegistry_AllRewardsHaveValidType(t *testing.T) {
	for _, rw := range buildRewardList() {
		if !validSlot(rw.Type) {
			t.Errorf("reward %q has invalid type %q", rw.ID, rw.Type)
		}
	}
}

func TestRegistry_AllRewardsHaveNonEmptyName(t *testing.T) {
	for _, rw := range buildRewardList() {
		if rw.Name == "" {
			t.Errorf("reward %q has empty Name", rw.ID)
		}
	}
}

func TestIsUnlocked_AchievementReward_Unlocked(t *testing.T) {
	reg := NewRewardRegistry()
	stats := newStats()
	stats.AchievementsUnlocked["first_lap"] = time.Now()

	if !reg.IsUnlocked("rookie_paint", stats) {
		t.Error("rookie_paint should be unlocked when first_lap achievement is earned")
	}
}

func TestIsUnlocked_AchievementReward_Locked(t *testing.T) {
	reg := NewRewardRegistry()
	stats := newStats()

	if reg.IsUnlocked("rookie_paint", stats) {
		t.Error("rookie_paint should be locked when first_lap achievement is not earned")
	}
}

func TestIsUnlocked_BattlePassReward_TierReached(t *testing.T) {
	reg := NewRewardRegistry()
	stats := newStats()
	stats.BattlePass.Tier = 5

	if !reg.IsUnlocked("metallic_paint", stats) {
		t.Error("metallic_paint (tier 5) should be unlocked at tier 5")
	}
	if !reg.IsUnlocked("bronze_badge", stats) {
		t.Error("bronze_badge (tier 2) should be unlocked at tier 5")
	}
}

func TestIsUnlocked_BattlePassReward_TierNotReached(t *testing.T) {
	reg := NewRewardRegistry()
	stats := newStats()
	stats.BattlePass.Tier = 1

	if reg.IsUnlocked("metallic_paint", stats) {
		t.Error("metallic_paint (tier 5) should be locked at tier 1")
	}
}

func TestIsUnlocked_UnknownReward(t *testing.T) {
	reg := NewRewardRegistry()
	stats := newStats()

	if reg.IsUnlocked("nonexistent_reward", stats) {
		t.Error("unknown reward ID should return false")
	}
}

func TestEquip_UnlockedReward_Succeeds(t *testing.T) {
	reg := NewRewardRegistry()
	stats := newStats()
	stats.AchievementsUnlocked["first_lap"] = time.Now()

	if err := reg.Equip("rookie_paint", stats); err != nil {
		t.Fatalf("Equip() unexpected error: %v", err)
	}
	if stats.Equipped.Paint != "rookie_paint" {
		t.Errorf("Equipped.Paint = %q, want %q", stats.Equipped.Paint, "rookie_paint")
	}
}

func TestEquip_LockedReward_Fails(t *testing.T) {
	reg := NewRewardRegistry()
	stats := newStats()

	err := reg.Equip("rookie_paint", stats)
	if err == nil {
		t.Fatal("Equip() should fail for locked reward")
	}
	if !errors.Is(err, ErrNotUnlocked) {
		t.Errorf("expected ErrNotUnlocked, got %v", err)
	}
}

func TestEquip_UnknownReward_Fails(t *testing.T) {
	reg := NewRewardRegistry()
	stats := newStats()

	err := reg.Equip("totally_fake_reward", stats)
	if err == nil {
		t.Fatal("Equip() should fail for unknown reward")
	}
	if !errors.Is(err, ErrUnknownReward) {
		t.Errorf("expected ErrUnknownReward, got %v", err)
	}
}

func TestEquip_OnlyOneItemPerSlot(t *testing.T) {
	reg := NewRewardRegistry()
	stats := newStats()
	stats.AchievementsUnlocked["first_lap"] = time.Now()
	stats.AchievementsUnlocked["century_club"] = time.Now()

	if err := reg.Equip("rookie_paint", stats); err != nil {
		t.Fatalf("Equip(rookie_paint) error: %v", err)
	}
	if stats.Equipped.Paint != "rookie_paint" {
		t.Fatalf("Equipped.Paint = %q, want rookie_paint", stats.Equipped.Paint)
	}

	if err := reg.Equip("century_paint", stats); err != nil {
		t.Fatalf("Equip(century_paint) error: %v", err)
	}
	if stats.Equipped.Paint != "century_paint" {
		t.Errorf("Equipped.Paint = %q, want century_paint (should replace previous)", stats.Equipped.Paint)
	}
}

func TestEquip_DifferentSlots_Independent(t *testing.T) {
	reg := NewRewardRegistry()
	stats := newStats()
	stats.AchievementsUnlocked["first_lap"] = time.Now()
	stats.AchievementsUnlocked["home_turf"] = time.Now()

	if err := reg.Equip("rookie_paint", stats); err != nil {
		t.Fatalf("Equip(rookie_paint) error: %v", err)
	}
	if err := reg.Equip("home_trail", stats); err != nil {
		t.Fatalf("Equip(home_trail) error: %v", err)
	}

	if stats.Equipped.Paint != "rookie_paint" {
		t.Errorf("Equipped.Paint = %q, want rookie_paint", stats.Equipped.Paint)
	}
	if stats.Equipped.Trail != "home_trail" {
		t.Errorf("Equipped.Trail = %q, want home_trail", stats.Equipped.Trail)
	}
}

func TestEquip_AllSlotTypes(t *testing.T) {
	reg := NewRewardRegistry()
	stats := newStats()

	unlockAllSlotAchievements(stats)

	cases := []struct {
		rewardID string
		slot     func(Equipped) string
		slotName string
	}{
		{"rookie_paint", func(e Equipped) string { return e.Paint }, "Paint"},
		{"home_trail", func(e Equipped) string { return e.Trail }, "Trail"},
		{"triple_body", func(e Equipped) string { return e.Body }, "Body"},
		{"grid_badge", func(e Equipped) string { return e.Badge }, "Badge"},
		{"opus_sound", func(e Equipped) string { return e.Sound }, "Sound"},
		{"polyglot_theme", func(e Equipped) string { return e.Theme }, "Theme"},
		{"veteran_title", func(e Equipped) string { return e.Title }, "Title"},
	}

	for _, tc := range cases {
		t.Run(tc.slotName, func(t *testing.T) {
			if err := reg.Equip(tc.rewardID, stats); err != nil {
				t.Fatalf("Equip(%q) error: %v", tc.rewardID, err)
			}
			if got := tc.slot(stats.Equipped); got != tc.rewardID {
				t.Errorf("Equipped.%s = %q, want %q", tc.slotName, got, tc.rewardID)
			}
		})
	}
}

func TestEquip_BattlePassReward_Succeeds(t *testing.T) {
	reg := NewRewardRegistry()
	stats := newStats()
	stats.BattlePass.Tier = 3

	if err := reg.Equip("spark_trail", stats); err != nil {
		t.Fatalf("Equip(spark_trail) error: %v", err)
	}
	if stats.Equipped.Trail != "spark_trail" {
		t.Errorf("Equipped.Trail = %q, want spark_trail", stats.Equipped.Trail)
	}
}

func TestEquip_BattlePassReward_Locked(t *testing.T) {
	reg := NewRewardRegistry()
	stats := newStats()
	stats.BattlePass.Tier = 2

	err := reg.Equip("spark_trail", stats)
	if err == nil {
		t.Fatal("Equip() should fail when tier not reached")
	}
	if !errors.Is(err, ErrNotUnlocked) {
		t.Errorf("expected ErrNotUnlocked, got %v", err)
	}
}

func TestUnequip_ClearsSlot(t *testing.T) {
	reg := NewRewardRegistry()
	stats := newStats()
	stats.AchievementsUnlocked["first_lap"] = time.Now()

	if err := reg.Equip("rookie_paint", stats); err != nil {
		t.Fatalf("Equip() error: %v", err)
	}

	if err := reg.Unequip(RewardTypePaint, stats); err != nil {
		t.Fatalf("Unequip() error: %v", err)
	}
	if stats.Equipped.Paint != "" {
		t.Errorf("Equipped.Paint = %q, want empty after unequip", stats.Equipped.Paint)
	}
}

func TestUnequip_EmptySlot_NoOp(t *testing.T) {
	reg := NewRewardRegistry()
	stats := newStats()

	if err := reg.Unequip(RewardTypePaint, stats); err != nil {
		t.Fatalf("Unequip() on empty slot should succeed, got: %v", err)
	}
}

func TestUnequip_InvalidSlot_ReturnsError(t *testing.T) {
	reg := NewRewardRegistry()
	stats := newStats()

	err := reg.Unequip(RewardType("invalid_slot"), stats)
	if err == nil {
		t.Fatal("Unequip() should fail for invalid slot")
	}
	if !errors.Is(err, ErrSlotMismatch) {
		t.Errorf("expected ErrSlotMismatch, got %v", err)
	}
}

func TestGetEquipped_ReturnsCurrentLoadout(t *testing.T) {
	reg := NewRewardRegistry()
	stats := newStats()
	stats.AchievementsUnlocked["first_lap"] = time.Now()
	stats.AchievementsUnlocked["home_turf"] = time.Now()

	_ = reg.Equip("rookie_paint", stats)
	_ = reg.Equip("home_trail", stats)

	eq := reg.GetEquipped(stats)
	if eq.Paint != "rookie_paint" {
		t.Errorf("GetEquipped().Paint = %q, want rookie_paint", eq.Paint)
	}
	if eq.Trail != "home_trail" {
		t.Errorf("GetEquipped().Trail = %q, want home_trail", eq.Trail)
	}
}

func TestEquip_PersistsToStatsJSON(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	reg := NewRewardRegistry()

	stats := newStats()
	stats.AchievementsUnlocked["first_lap"] = time.Now()

	if err := reg.Equip("rookie_paint", stats); err != nil {
		t.Fatalf("Equip() error: %v", err)
	}

	if err := store.Save(stats); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if loaded.Equipped.Paint != "rookie_paint" {
		t.Errorf("Loaded Equipped.Paint = %q, want rookie_paint", loaded.Equipped.Paint)
	}
}

func TestEquip_FullLoadout_PersistsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	reg := NewRewardRegistry()

	stats := newStats()
	unlockAllSlotAchievements(stats)

	_ = reg.Equip("rookie_paint", stats)
	_ = reg.Equip("home_trail", stats)
	_ = reg.Equip("triple_body", stats)
	_ = reg.Equip("grid_badge", stats)
	_ = reg.Equip("opus_sound", stats)
	_ = reg.Equip("polyglot_theme", stats)
	_ = reg.Equip("veteran_title", stats)

	if err := store.Save(stats); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	want := Equipped{
		Paint: "rookie_paint",
		Trail: "home_trail",
		Body:  "triple_body",
		Badge: "grid_badge",
		Sound: "opus_sound",
		Theme: "polyglot_theme",
		Title: "veteran_title",
	}
	if loaded.Equipped != want {
		t.Errorf("Loaded Equipped = %+v, want %+v", loaded.Equipped, want)
	}
}

func TestValidSlot_AllKnownSlots(t *testing.T) {
	slots := []RewardType{
		RewardTypePaint, RewardTypeTrail, RewardTypeBody,
		RewardTypeBadge, RewardTypeSound, RewardTypeTheme, RewardTypeTitle,
	}
	for _, s := range slots {
		if !validSlot(s) {
			t.Errorf("validSlot(%q) = false, want true", s)
		}
	}
}

func TestValidSlot_UnknownSlot(t *testing.T) {
	if validSlot(RewardType("rocket")) {
		t.Error("validSlot(rocket) = true, want false")
	}
}
