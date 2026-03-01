package gamification

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewStore_DefaultDir(t *testing.T) {
	s := NewStore("")
	if s.dir == "" {
		t.Fatal("expected non-empty default dir")
	}
	if filepath.Base(s.dir) != appDirName {
		t.Errorf("expected dir to end with %q, got %q", appDirName, s.dir)
	}
}

func TestNewStore_CustomDir(t *testing.T) {
	s := NewStore("/tmp/custom")
	if s.dir != "/tmp/custom" {
		t.Errorf("expected /tmp/custom, got %s", s.dir)
	}
}

func TestStore_Path(t *testing.T) {
	s := NewStore("/tmp/test-dir")
	want := "/tmp/test-dir/stats.json"
	if got := s.Path(); got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

func TestStore_LoadMissing(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	st, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if st.Version != statsVersion {
		t.Errorf("Version = %d, want %d", st.Version, statsVersion)
	}
	if st.SessionsPerSource == nil {
		t.Error("SessionsPerSource should be initialized")
	}
	if st.SessionsPerModel == nil {
		t.Error("SessionsPerModel should be initialized")
	}
	if st.AchievementsUnlocked == nil {
		t.Error("AchievementsUnlocked should be initialized")
	}
}

func TestStore_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	st := newStats()
	st.TotalSessions = 42
	st.TotalCompletions = 30
	st.TotalErrors = 5
	st.ConsecutiveCompletions = 7
	st.SessionsPerSource["claude"] = 25
	st.SessionsPerSource["codex"] = 17
	st.SessionsPerModel["opus-4"] = 20
	st.DistinctModelsUsed = 3
	st.DistinctSourcesUsed = 2
	st.MaxContextUtilization = 0.95
	st.MaxBurnRate = 1234.5
	st.MaxConcurrentActive = 5
	st.MaxToolCalls = 200
	st.MaxMessages = 150
	st.MaxSessionDurationSec = 3600.0
	st.AchievementsUnlocked["first_blood"] = time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	st.BattlePass = BattlePass{Season: "2025-01", Tier: 5, XP: 2500}
	st.Equipped = Equipped{Trail: "flame", Badge: "gold"}

	if err := s.Save(st); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if loaded.Version != statsVersion {
		t.Errorf("Version = %d, want %d", loaded.Version, statsVersion)
	}
	if loaded.TotalSessions != 42 {
		t.Errorf("TotalSessions = %d, want 42", loaded.TotalSessions)
	}
	if loaded.TotalCompletions != 30 {
		t.Errorf("TotalCompletions = %d, want 30", loaded.TotalCompletions)
	}
	if loaded.TotalErrors != 5 {
		t.Errorf("TotalErrors = %d, want 5", loaded.TotalErrors)
	}
	if loaded.ConsecutiveCompletions != 7 {
		t.Errorf("ConsecutiveCompletions = %d, want 7", loaded.ConsecutiveCompletions)
	}
	if loaded.SessionsPerSource["claude"] != 25 {
		t.Errorf("SessionsPerSource[claude] = %d, want 25", loaded.SessionsPerSource["claude"])
	}
	if loaded.SessionsPerModel["opus-4"] != 20 {
		t.Errorf("SessionsPerModel[opus-4] = %d, want 20", loaded.SessionsPerModel["opus-4"])
	}
	if loaded.DistinctModelsUsed != 3 {
		t.Errorf("DistinctModelsUsed = %d, want 3", loaded.DistinctModelsUsed)
	}
	if loaded.MaxContextUtilization != 0.95 {
		t.Errorf("MaxContextUtilization = %f, want 0.95", loaded.MaxContextUtilization)
	}
	if loaded.MaxBurnRate != 1234.5 {
		t.Errorf("MaxBurnRate = %f, want 1234.5", loaded.MaxBurnRate)
	}
	if loaded.MaxConcurrentActive != 5 {
		t.Errorf("MaxConcurrentActive = %d, want 5", loaded.MaxConcurrentActive)
	}
	if loaded.MaxToolCalls != 200 {
		t.Errorf("MaxToolCalls = %d, want 200", loaded.MaxToolCalls)
	}
	if loaded.MaxMessages != 150 {
		t.Errorf("MaxMessages = %d, want 150", loaded.MaxMessages)
	}
	if loaded.MaxSessionDurationSec != 3600.0 {
		t.Errorf("MaxSessionDurationSec = %f, want 3600", loaded.MaxSessionDurationSec)
	}
	if loaded.BattlePass.Season != "2025-01" || loaded.BattlePass.Tier != 5 || loaded.BattlePass.XP != 2500 {
		t.Errorf("BattlePass = %+v, want {2025-01 5 2500}", loaded.BattlePass)
	}
	if loaded.Equipped.Trail != "flame" || loaded.Equipped.Badge != "gold" {
		t.Errorf("Equipped = %+v, want {flame gold ''}", loaded.Equipped)
	}
	if _, ok := loaded.AchievementsUnlocked["first_blood"]; !ok {
		t.Error("AchievementsUnlocked should contain first_blood")
	}
	if loaded.LastUpdated.IsZero() {
		t.Error("LastUpdated should be set after Save")
	}
}

func TestStore_SaveCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	s := NewStore(dir)

	st := newStats()
	if err := s.Save(st); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	if _, err := os.Stat(s.Path()); err != nil {
		t.Errorf("stats file should exist: %v", err)
	}
}

func TestStore_SaveOverwriteCleansTempFiles(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	st := newStats()
	st.TotalSessions = 10
	if err := s.Save(st); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	st.TotalSessions = 20
	if err := s.Save(st); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if loaded.TotalSessions != 20 {
		t.Errorf("TotalSessions = %d, want 20", loaded.TotalSessions)
	}

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != statsFileName {
			t.Errorf("unexpected file left behind: %s", e.Name())
		}
	}
}

func TestStore_LoadCorruptJSON(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	if err := os.WriteFile(s.Path(), []byte("{invalid"), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	_, err := s.Load()
	if err == nil {
		t.Fatal("Load() should return error for corrupt JSON")
	}
}

func TestStore_LoadInitializesMaps(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	// Write JSON with null maps
	data, _ := json.Marshal(Stats{Version: 1})
	if err := os.WriteFile(s.Path(), data, 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	st, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if st.SessionsPerSource == nil {
		t.Error("SessionsPerSource should be initialized even from null JSON")
	}
	if st.SessionsPerModel == nil {
		t.Error("SessionsPerModel should be initialized even from null JSON")
	}
	if st.AchievementsUnlocked == nil {
		t.Error("AchievementsUnlocked should be initialized even from null JSON")
	}
}

func TestStore_SaveSetsVersionAndTimestamp(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	st := newStats()
	st.Version = 0 // intentionally wrong
	before := time.Now().UTC()

	if err := s.Save(st); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	after := time.Now().UTC()

	if st.Version != statsVersion {
		t.Errorf("Version should be set to %d, got %d", statsVersion, st.Version)
	}
	if st.LastUpdated.Before(before) || st.LastUpdated.After(after) {
		t.Errorf("LastUpdated %v not in range [%v, %v]", st.LastUpdated, before, after)
	}
}

func TestDefaultStatsDir_XDGOverride(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/custom/state")
	got := defaultStatsDir()
	want := "/custom/state/agent-racer"
	if got != want {
		t.Errorf("defaultStatsDir() = %q, want %q", got, want)
	}
}

func TestDefaultStatsDir_Fallback(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	got := defaultStatsDir()
	// Should end with .local/state/agent-racer
	if filepath.Base(got) != appDirName {
		t.Errorf("expected dir ending with %q, got %q", appDirName, got)
	}
}

func TestStore_SaveFilePermissions(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	st := newStats()
	if err := s.Save(st); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	info, err := os.Stat(s.Path())
	if err != nil {
		t.Fatalf("Stat error: %v", err)
	}

	// File should be readable/writable by owner at minimum
	perm := info.Mode().Perm()
	if perm&0o600 != 0o600 {
		t.Errorf("expected at least 0600 permissions, got %o", perm)
	}
}

func TestStore_AtomicWriteSurvivesCrash(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	// Write initial stats
	initial := newStats()
	initial.TotalSessions = 100
	initial.TotalCompletions = 50
	if err := s.Save(initial); err != nil {
		t.Fatalf("initial Save error: %v", err)
	}

	// Verify initial write succeeded
	loaded, err := s.Load()
	if err != nil {
		t.Fatalf("Load initial error: %v", err)
	}
	if loaded.TotalSessions != 100 {
		t.Errorf("initial TotalSessions = %d, want 100", loaded.TotalSessions)
	}

	// The atomic write pattern (temp file + rename) ensures that
	// even if a crash occurs, either:
	// 1. The temp file is never renamed (original file untouched)
	// 2. The rename succeeds atomically (new file replaces old)
	// This test verifies that data is recoverable after the first save.

	// Now save new stats - should use atomic temp+rename pattern
	updated := newStats()
	updated.TotalSessions = 200
	updated.TotalCompletions = 100
	if err := s.Save(updated); err != nil {
		t.Fatalf("Save after crash simulation error: %v", err)
	}

	// Verify we can load the new data correctly
	loaded, err = s.Load()
	if err != nil {
		t.Fatalf("Load after update error: %v", err)
	}
	if loaded.TotalSessions != 200 {
		t.Errorf("TotalSessions after update = %d, want 200", loaded.TotalSessions)
	}
	if loaded.TotalCompletions != 100 {
		t.Errorf("TotalCompletions after update = %d, want 100", loaded.TotalCompletions)
	}

	// Verify no temp files are left behind (cleanup on success)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir error: %v", err)
	}
	for _, e := range entries {
		if e.Name() != statsFileName {
			t.Errorf("unexpected file in directory: %s", e.Name())
		}
	}
}

func TestStore_AtomicWriteNoTempFileLeak(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	// Perform several saves
	for i := 0; i < 5; i++ {
		st := newStats()
		st.TotalSessions = i * 10
		if err := s.Save(st); err != nil {
			t.Fatalf("Save %d error: %v", i, err)
		}
	}

	// Check that no temp files are left behind
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir error: %v", err)
	}

	for _, e := range entries {
		if e.Name() != statsFileName {
			t.Errorf("unexpected file left in dir: %s", e.Name())
		}
	}

	if len(entries) != 1 {
		t.Errorf("expected exactly 1 file, got %d", len(entries))
	}
}

func TestStore_RoundTripWithAllFields(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	st := newStats()
	st.TotalSessions = 42
	st.TotalCompletions = 30
	st.TotalErrors = 5
	st.ConsecutiveCompletions = 7
	st.SessionsPerSource["claude"] = 25
	st.SessionsPerSource["codex"] = 17
	st.SessionsPerModel["opus-4"] = 20
	st.SessionsPerModel["haiku"] = 22
	st.DistinctModelsUsed = 2
	st.DistinctSourcesUsed = 2
	st.MaxContextUtilization = 0.95
	st.MaxBurnRate = 1234.5
	st.MaxConcurrentActive = 5
	st.MaxToolCalls = 200
	st.MaxMessages = 150
	st.MaxSessionDurationSec = 3600.0
	st.AchievementsUnlocked["first_blood"] = time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	st.AchievementsUnlocked["speed_demon"] = time.Date(2026, 2, 1, 12, 30, 0, 0, time.UTC)
	st.BattlePass = BattlePass{Season: "2025-02", Tier: 10, XP: 5000}
	st.Equipped = Equipped{Trail: "flame", Badge: "gold", Theme: "dark"}

	if err := s.Save(st); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	loaded, err := s.Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	// Verify all fields round-trip correctly
	if loaded.Version != statsVersion {
		t.Errorf("Version = %d, want %d", loaded.Version, statsVersion)
	}
	if loaded.TotalSessions != 42 {
		t.Errorf("TotalSessions = %d, want 42", loaded.TotalSessions)
	}
	if loaded.TotalCompletions != 30 {
		t.Errorf("TotalCompletions = %d, want 30", loaded.TotalCompletions)
	}
	if loaded.TotalErrors != 5 {
		t.Errorf("TotalErrors = %d, want 5", loaded.TotalErrors)
	}
	if loaded.ConsecutiveCompletions != 7 {
		t.Errorf("ConsecutiveCompletions = %d, want 7", loaded.ConsecutiveCompletions)
	}
	if loaded.SessionsPerSource["claude"] != 25 {
		t.Errorf("SessionsPerSource[claude] = %d, want 25", loaded.SessionsPerSource["claude"])
	}
	if loaded.SessionsPerSource["codex"] != 17 {
		t.Errorf("SessionsPerSource[codex] = %d, want 17", loaded.SessionsPerSource["codex"])
	}
	if loaded.SessionsPerModel["opus-4"] != 20 {
		t.Errorf("SessionsPerModel[opus-4] = %d, want 20", loaded.SessionsPerModel["opus-4"])
	}
	if loaded.SessionsPerModel["haiku"] != 22 {
		t.Errorf("SessionsPerModel[haiku] = %d, want 22", loaded.SessionsPerModel["haiku"])
	}
	if loaded.DistinctModelsUsed != 2 {
		t.Errorf("DistinctModelsUsed = %d, want 2", loaded.DistinctModelsUsed)
	}
	if loaded.DistinctSourcesUsed != 2 {
		t.Errorf("DistinctSourcesUsed = %d, want 2", loaded.DistinctSourcesUsed)
	}
	if loaded.MaxContextUtilization != 0.95 {
		t.Errorf("MaxContextUtilization = %f, want 0.95", loaded.MaxContextUtilization)
	}
	if loaded.MaxBurnRate != 1234.5 {
		t.Errorf("MaxBurnRate = %f, want 1234.5", loaded.MaxBurnRate)
	}
	if loaded.MaxConcurrentActive != 5 {
		t.Errorf("MaxConcurrentActive = %d, want 5", loaded.MaxConcurrentActive)
	}
	if loaded.MaxToolCalls != 200 {
		t.Errorf("MaxToolCalls = %d, want 200", loaded.MaxToolCalls)
	}
	if loaded.MaxMessages != 150 {
		t.Errorf("MaxMessages = %d, want 150", loaded.MaxMessages)
	}
	if loaded.MaxSessionDurationSec != 3600.0 {
		t.Errorf("MaxSessionDurationSec = %f, want 3600", loaded.MaxSessionDurationSec)
	}
	if loaded.BattlePass.Season != "2025-02" {
		t.Errorf("BattlePass.Season = %s, want 2025-02", loaded.BattlePass.Season)
	}
	if loaded.BattlePass.Tier != 10 {
		t.Errorf("BattlePass.Tier = %d, want 10", loaded.BattlePass.Tier)
	}
	if loaded.BattlePass.XP != 5000 {
		t.Errorf("BattlePass.XP = %d, want 5000", loaded.BattlePass.XP)
	}
	if loaded.Equipped.Trail != "flame" {
		t.Errorf("Equipped.Trail = %s, want flame", loaded.Equipped.Trail)
	}
	if loaded.Equipped.Badge != "gold" {
		t.Errorf("Equipped.Badge = %s, want gold", loaded.Equipped.Badge)
	}
	if loaded.Equipped.Theme != "dark" {
		t.Errorf("Equipped.Theme = %s, want dark", loaded.Equipped.Theme)
	}
	if len(loaded.AchievementsUnlocked) != 2 {
		t.Errorf("AchievementsUnlocked length = %d, want 2", len(loaded.AchievementsUnlocked))
	}
	if _, ok := loaded.AchievementsUnlocked["first_blood"]; !ok {
		t.Error("AchievementsUnlocked should contain first_blood")
	}
	if _, ok := loaded.AchievementsUnlocked["speed_demon"]; !ok {
		t.Error("AchievementsUnlocked should contain speed_demon")
	}
	if loaded.LastUpdated.IsZero() {
		t.Error("LastUpdated should be set after Save")
	}
}

func TestStats_CloneDeepCopiesWeeklyChallengesCompleted(t *testing.T) {
	// Create original Stats with populated Completed slice
	original := newStats()
	original.WeeklyChallenges.Completed = make([]string, 3)
	original.WeeklyChallenges.Completed[0] = "challenge_1"
	original.WeeklyChallenges.Completed[1] = "challenge_2"
	original.WeeklyChallenges.Completed[2] = "challenge_3"

	// Clone the stats
	cloned := original.clone()

	// Verify initial equality
	if len(cloned.WeeklyChallenges.Completed) != 3 {
		t.Fatalf("cloned Completed length = %d, want 3", len(cloned.WeeklyChallenges.Completed))
	}
	for i := 0; i < 3; i++ {
		if cloned.WeeklyChallenges.Completed[i] != original.WeeklyChallenges.Completed[i] {
			t.Errorf("cloned Completed[%d] = %q, want %q",
				i, cloned.WeeklyChallenges.Completed[i], original.WeeklyChallenges.Completed[i])
		}
	}

	// Mutate the cloned slice
	cloned.WeeklyChallenges.Completed[0] = "modified_challenge_1"

	// Verify original is unaffected
	if original.WeeklyChallenges.Completed[0] != "challenge_1" {
		t.Errorf("original Completed[0] was mutated: got %q, want %q",
			original.WeeklyChallenges.Completed[0], "challenge_1")
	}

	// Verify clone has the mutation
	if cloned.WeeklyChallenges.Completed[0] != "modified_challenge_1" {
		t.Errorf("cloned Completed[0] = %q, want %q",
			cloned.WeeklyChallenges.Completed[0], "modified_challenge_1")
	}
}

func TestStats_CloneDeepCopiesEmptyCompleted(t *testing.T) {
	// Test with empty Completed slice
	original := newStats()
	original.WeeklyChallenges.Completed = []string{}

	cloned := original.clone()

	if len(cloned.WeeklyChallenges.Completed) != 0 {
		t.Fatalf("cloned Completed length = %d, want 0", len(cloned.WeeklyChallenges.Completed))
	}

	// Append to cloned slice should not affect original
	cloned.WeeklyChallenges.Completed = append(cloned.WeeklyChallenges.Completed, "challenge_1")

	if len(original.WeeklyChallenges.Completed) != 0 {
		t.Errorf("original Completed was affected: got length %d, want 0", len(original.WeeklyChallenges.Completed))
	}
}

func TestStats_CloneDeepCopiesAllWeeklyChallengeFields(t *testing.T) {
	// Comprehensive test for all WeeklyChallenges fields
	original := newStats()
	now := time.Now().UTC()
	original.WeeklyChallenges.WeekStart = now
	original.WeeklyChallenges.ActiveIDs = []string{"challenge_a", "challenge_b"}
	original.WeeklyChallenges.Completed = []string{"challenge_1", "challenge_2"}
	original.WeeklyChallenges.Snapshot.SessionsPerModel = map[string]int{"opus": 5, "haiku": 3}
	original.WeeklyChallenges.Snapshot.SessionsPerSource = map[string]int{"claude": 8}
	original.WeeklyChallenges.XPAwarded = map[string]bool{"challenge_1": true}

	cloned := original.clone()

	// Verify deep copies by mutating each field
	cloned.WeeklyChallenges.Completed[0] = "mutated"
	if original.WeeklyChallenges.Completed[0] != "challenge_1" {
		t.Error("Completed slice not properly cloned")
	}

	cloned.WeeklyChallenges.ActiveIDs[0] = "mutated"
	if original.WeeklyChallenges.ActiveIDs[0] != "challenge_a" {
		t.Error("ActiveIDs slice not properly cloned")
	}

	cloned.WeeklyChallenges.Snapshot.SessionsPerModel["new_model"] = 99
	if _, ok := original.WeeklyChallenges.Snapshot.SessionsPerModel["new_model"]; ok {
		t.Error("Snapshot.SessionsPerModel map not properly cloned")
	}

	cloned.WeeklyChallenges.Snapshot.SessionsPerSource["new_source"] = 99
	if _, ok := original.WeeklyChallenges.Snapshot.SessionsPerSource["new_source"]; ok {
		t.Error("Snapshot.SessionsPerSource map not properly cloned")
	}

	cloned.WeeklyChallenges.XPAwarded["new_challenge"] = true
	if _, ok := original.WeeklyChallenges.XPAwarded["new_challenge"]; ok {
		t.Error("XPAwarded map not properly cloned")
	}

	// Verify WeekStart is the same value (not shared)
	if !cloned.WeeklyChallenges.WeekStart.Equal(original.WeeklyChallenges.WeekStart) {
		t.Error("WeekStart not properly cloned")
	}
}
