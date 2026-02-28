package gamification

import (
	"testing"
	"time"
)

func hasID(achievements []Achievement, id string) bool {
	for _, a := range achievements {
		if a.ID == id {
			return true
		}
	}
	return false
}

func TestRegistry_ReturnsShallowCopy(t *testing.T) {
	e := NewAchievementEngine()
	r1 := e.Registry()
	r2 := e.Registry()
	if &r1[0] == &r2[0] {
		t.Error("Registry should return independent copies")
	}
}

func TestRegistry_AllCategoriesCovered(t *testing.T) {
	all := map[Category]bool{
		CategorySessionMilestones:    false,
		CategorySourceDiversity:      false,
		CategoryModelCollection:      false,
		CategoryPerformanceEndurance: false,
		CategorySpectacle:            false,
		CategoryStreaks:              false,
	}
	for _, a := range NewAchievementEngine().Registry() {
		all[a.Category] = true
	}
	for cat, seen := range all {
		if !seen {
			t.Errorf("category %q has no achievements", cat)
		}
	}
}

func TestRegistry_UniqueIDs(t *testing.T) {
	seen := make(map[string]bool)
	for _, a := range NewAchievementEngine().Registry() {
		if seen[a.ID] {
			t.Errorf("duplicate achievement ID: %s", a.ID)
		}
		seen[a.ID] = true
	}
}

func TestEvaluate_ZeroStats_NoUnlocks(t *testing.T) {
	e := NewAchievementEngine()
	unlocked := e.Evaluate(newStats())
	if len(unlocked) != 0 {
		t.Errorf("zero stats unlocked %d achievements, want 0", len(unlocked))
	}
}

func TestEvaluate_Idempotent(t *testing.T) {
	e := NewAchievementEngine()
	s := newStats()
	s.TotalSessions = 1

	first := e.Evaluate(s)
	if len(first) == 0 {
		t.Fatal("expected at least first_lap on first evaluate")
	}

	second := e.Evaluate(s)
	if len(second) != 0 {
		t.Errorf("second evaluate returned %d achievements, want 0 (idempotent)", len(second))
	}
}

func TestEvaluate_RecordsTimestamp(t *testing.T) {
	e := NewAchievementEngine()
	s := newStats()
	s.TotalSessions = 1

	before := time.Now().UTC().Add(-time.Second)
	e.Evaluate(s)
	after := time.Now().UTC().Add(time.Second)

	ts, ok := s.AchievementsUnlocked["first_lap"]
	if !ok {
		t.Fatal("first_lap not recorded in AchievementsUnlocked")
	}
	if ts.Before(before) || ts.After(after) {
		t.Errorf("timestamp %v not between %v and %v", ts, before, after)
	}
}

func TestSessionMilestones_FirstLap(t *testing.T) {
	e := NewAchievementEngine()

	s := newStats()
	s.TotalSessions = 0
	if u := e.Evaluate(s); hasID(u, "first_lap") {
		t.Error("first_lap unlocked at 0 sessions")
	}

	s = newStats()
	s.TotalSessions = 1
	if u := e.Evaluate(s); !hasID(u, "first_lap") {
		t.Error("first_lap not unlocked at 1 session")
	}
}

func TestSessionMilestones_AllTiers(t *testing.T) {
	tests := []struct {
		id        string
		threshold int
	}{
		{"first_lap", 1},
		{"pit_crew", 10},
		{"veteran_driver", 50},
		{"century_club", 100},
		{"track_legend", 500},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			e := NewAchievementEngine()

			below := newStats()
			below.TotalSessions = tt.threshold - 1
			if u := e.Evaluate(below); hasID(u, tt.id) {
				t.Errorf("%s unlocked at %d (below threshold %d)", tt.id, tt.threshold-1, tt.threshold)
			}

			e = NewAchievementEngine()
			at := newStats()
			at.TotalSessions = tt.threshold
			if u := e.Evaluate(at); !hasID(u, tt.id) {
				t.Errorf("%s NOT unlocked at threshold %d", tt.id, tt.threshold)
			}
		})
	}
}

func TestSourceDiversity_HomeTurf(t *testing.T) {
	e := NewAchievementEngine()

	s := newStats()
	s.SessionsPerSource["claude"] = 4
	if u := e.Evaluate(s); hasID(u, "home_turf") {
		t.Error("home_turf unlocked at 4 claude sessions")
	}

	e = NewAchievementEngine()
	s = newStats()
	s.SessionsPerSource["claude"] = 5
	if u := e.Evaluate(s); !hasID(u, "home_turf") {
		t.Error("home_turf not unlocked at 5 claude sessions")
	}
}

func TestSourceDiversity_GeminiRising(t *testing.T) {
	e := NewAchievementEngine()
	s := newStats()
	s.SessionsPerSource["gemini"] = 1
	if u := e.Evaluate(s); !hasID(u, "gemini_rising") {
		t.Error("gemini_rising not unlocked")
	}
}

func TestSourceDiversity_CodexCurious(t *testing.T) {
	e := NewAchievementEngine()
	s := newStats()
	s.SessionsPerSource["codex"] = 1
	if u := e.Evaluate(s); !hasID(u, "codex_curious") {
		t.Error("codex_curious not unlocked")
	}
}

func TestSourceDiversity_TripleThreat(t *testing.T) {
	e := NewAchievementEngine()

	// Missing one source
	s := newStats()
	s.SessionsPerSource["claude"] = 1
	s.SessionsPerSource["gemini"] = 1
	if u := e.Evaluate(s); hasID(u, "triple_threat") {
		t.Error("triple_threat unlocked with only 2 sources")
	}

	e = NewAchievementEngine()
	s = newStats()
	s.SessionsPerSource["claude"] = 1
	s.SessionsPerSource["gemini"] = 1
	s.SessionsPerSource["codex"] = 1
	if u := e.Evaluate(s); !hasID(u, "triple_threat") {
		t.Error("triple_threat not unlocked with all 3 sources")
	}
}

func TestSourceDiversity_Polyglot(t *testing.T) {
	e := NewAchievementEngine()

	s := newStats()
	s.SessionsPerSource["claude"] = 10
	s.SessionsPerSource["gemini"] = 10
	s.SessionsPerSource["codex"] = 9
	if u := e.Evaluate(s); hasID(u, "polyglot") {
		t.Error("polyglot unlocked with codex=9")
	}

	e = NewAchievementEngine()
	s = newStats()
	s.SessionsPerSource["claude"] = 10
	s.SessionsPerSource["gemini"] = 10
	s.SessionsPerSource["codex"] = 10
	if u := e.Evaluate(s); !hasID(u, "polyglot") {
		t.Error("polyglot not unlocked at 10 each")
	}
}

func TestModelCollection_OpusEnthusiast(t *testing.T) {
	e := NewAchievementEngine()

	s := newStats()
	s.SessionsPerModel["claude-opus-4"] = 4
	if u := e.Evaluate(s); hasID(u, "opus_enthusiast") {
		t.Error("opus_enthusiast unlocked at 4")
	}

	e = NewAchievementEngine()
	s = newStats()
	s.SessionsPerModel["claude-opus-4"] = 5
	if u := e.Evaluate(s); !hasID(u, "opus_enthusiast") {
		t.Error("opus_enthusiast not unlocked at 5")
	}
}

func TestModelCollection_OpusEnthusiast_CaseInsensitive(t *testing.T) {
	e := NewAchievementEngine()
	s := newStats()
	s.SessionsPerModel["Claude-OPUS-4"] = 5
	if u := e.Evaluate(s); !hasID(u, "opus_enthusiast") {
		t.Error("opus_enthusiast should match case-insensitively")
	}
}

func TestModelCollection_SonnetFan(t *testing.T) {
	e := NewAchievementEngine()
	s := newStats()
	s.SessionsPerModel["claude-sonnet-4"] = 5
	if u := e.Evaluate(s); !hasID(u, "sonnet_fan") {
		t.Error("sonnet_fan not unlocked at 5")
	}
}

func TestModelCollection_HaikuSpeedster(t *testing.T) {
	e := NewAchievementEngine()
	s := newStats()
	s.SessionsPerModel["claude-haiku-3.5"] = 5
	if u := e.Evaluate(s); !hasID(u, "haiku_speedster") {
		t.Error("haiku_speedster not unlocked at 5")
	}
}

func TestModelCollection_FamilySessionsSumAcrossModels(t *testing.T) {
	e := NewAchievementEngine()
	s := newStats()
	s.SessionsPerModel["claude-opus-4"] = 3
	s.SessionsPerModel["claude-opus-4-0519"] = 2
	if u := e.Evaluate(s); !hasID(u, "opus_enthusiast") {
		t.Error("opus_enthusiast should sum sessions across opus model variants")
	}
}

func TestModelCollection_FullSpectrum(t *testing.T) {
	e := NewAchievementEngine()

	// Missing haiku
	s := newStats()
	s.SessionsPerModel["claude-opus-4"] = 1
	s.SessionsPerModel["claude-sonnet-4"] = 1
	if u := e.Evaluate(s); hasID(u, "full_spectrum") {
		t.Error("full_spectrum unlocked without haiku")
	}

	e = NewAchievementEngine()
	s = newStats()
	s.SessionsPerModel["claude-opus-4"] = 1
	s.SessionsPerModel["claude-sonnet-4"] = 1
	s.SessionsPerModel["claude-haiku-3.5"] = 1
	if u := e.Evaluate(s); !hasID(u, "full_spectrum") {
		t.Error("full_spectrum not unlocked with all 3 families")
	}
}

func TestModelCollection_ModelCollector(t *testing.T) {
	e := NewAchievementEngine()

	s := newStats()
	s.DistinctModelsUsed = 4
	if u := e.Evaluate(s); hasID(u, "model_collector") {
		t.Error("model_collector unlocked at 4")
	}

	e = NewAchievementEngine()
	s = newStats()
	s.DistinctModelsUsed = 5
	if u := e.Evaluate(s); !hasID(u, "model_collector") {
		t.Error("model_collector not unlocked at 5")
	}
}

func TestModelCollection_Connoisseur(t *testing.T) {
	e := NewAchievementEngine()

	s := newStats()
	s.DistinctModelsUsed = 9
	if u := e.Evaluate(s); hasID(u, "connoisseur") {
		t.Error("connoisseur unlocked at 9")
	}

	e = NewAchievementEngine()
	s = newStats()
	s.DistinctModelsUsed = 10
	if u := e.Evaluate(s); !hasID(u, "connoisseur") {
		t.Error("connoisseur not unlocked at 10")
	}
}

func TestPerformance_Redline(t *testing.T) {
	e := NewAchievementEngine()

	s := newStats()
	s.MaxContextUtilization = 0.94
	if u := e.Evaluate(s); hasID(u, "redline") {
		t.Error("redline unlocked at 0.94")
	}

	e = NewAchievementEngine()
	s = newStats()
	s.MaxContextUtilization = 0.95
	if u := e.Evaluate(s); !hasID(u, "redline") {
		t.Error("redline not unlocked at 0.95")
	}
}

func TestPerformance_Afterburner(t *testing.T) {
	e := NewAchievementEngine()

	s := newStats()
	s.MaxBurnRate = 4999
	if u := e.Evaluate(s); hasID(u, "afterburner") {
		t.Error("afterburner unlocked at 4999")
	}

	e = NewAchievementEngine()
	s = newStats()
	s.MaxBurnRate = 5000
	if u := e.Evaluate(s); !hasID(u, "afterburner") {
		t.Error("afterburner not unlocked at 5000")
	}
}

func TestPerformance_Marathon(t *testing.T) {
	e := NewAchievementEngine()

	s := newStats()
	s.MaxSessionDurationSec = 7199
	if u := e.Evaluate(s); hasID(u, "marathon") {
		t.Error("marathon unlocked at 7199s")
	}

	e = NewAchievementEngine()
	s = newStats()
	s.MaxSessionDurationSec = 7200
	if u := e.Evaluate(s); !hasID(u, "marathon") {
		t.Error("marathon not unlocked at 7200s")
	}
}

func TestPerformance_ToolFiend(t *testing.T) {
	e := NewAchievementEngine()

	s := newStats()
	s.MaxToolCalls = 499
	if u := e.Evaluate(s); hasID(u, "tool_fiend") {
		t.Error("tool_fiend unlocked at 499")
	}

	e = NewAchievementEngine()
	s = newStats()
	s.MaxToolCalls = 500
	if u := e.Evaluate(s); !hasID(u, "tool_fiend") {
		t.Error("tool_fiend not unlocked at 500")
	}
}

func TestPerformance_Conversationalist(t *testing.T) {
	e := NewAchievementEngine()

	s := newStats()
	s.MaxMessages = 199
	if u := e.Evaluate(s); hasID(u, "conversationalist") {
		t.Error("conversationalist unlocked at 199")
	}

	e = NewAchievementEngine()
	s = newStats()
	s.MaxMessages = 200
	if u := e.Evaluate(s); !hasID(u, "conversationalist") {
		t.Error("conversationalist not unlocked at 200")
	}
}

func TestPerformance_CleanSweep(t *testing.T) {
	e := NewAchievementEngine()

	s := newStats()
	s.ConsecutiveCompletions = 9
	if u := e.Evaluate(s); hasID(u, "clean_sweep") {
		t.Error("clean_sweep unlocked at 9")
	}

	e = NewAchievementEngine()
	s = newStats()
	s.ConsecutiveCompletions = 10
	if u := e.Evaluate(s); !hasID(u, "clean_sweep") {
		t.Error("clean_sweep not unlocked at 10")
	}
}

func TestPerformance_PhotoFinish(t *testing.T) {
	e := NewAchievementEngine()

	s := newStats()
	s.PhotoFinishSeen = false
	if u := e.Evaluate(s); hasID(u, "photo_finish") {
		t.Error("photo_finish unlocked without near-simultaneous completions")
	}

	e = NewAchievementEngine()
	s = newStats()
	s.PhotoFinishSeen = true
	if u := e.Evaluate(s); !hasID(u, "photo_finish") {
		t.Error("photo_finish not unlocked when PhotoFinishSeen is true")
	}
}

func TestSpectacle_GridStart(t *testing.T) {
	e := NewAchievementEngine()

	s := newStats()
	s.MaxConcurrentActive = 2
	if u := e.Evaluate(s); hasID(u, "grid_start") {
		t.Error("grid_start unlocked at 2")
	}

	e = NewAchievementEngine()
	s = newStats()
	s.MaxConcurrentActive = 3
	if u := e.Evaluate(s); !hasID(u, "grid_start") {
		t.Error("grid_start not unlocked at 3")
	}
}

func TestSpectacle_FullGrid(t *testing.T) {
	e := NewAchievementEngine()

	s := newStats()
	s.MaxConcurrentActive = 4
	if u := e.Evaluate(s); hasID(u, "full_grid") {
		t.Error("full_grid unlocked at 4")
	}

	e = NewAchievementEngine()
	s = newStats()
	s.MaxConcurrentActive = 5
	if u := e.Evaluate(s); !hasID(u, "full_grid") {
		t.Error("full_grid not unlocked at 5")
	}
}

func TestSpectacle_GridFull(t *testing.T) {
	e := NewAchievementEngine()

	s := newStats()
	s.MaxConcurrentActive = 9
	if u := e.Evaluate(s); hasID(u, "grid_full") {
		t.Error("grid_full unlocked at 9")
	}

	e = NewAchievementEngine()
	s = newStats()
	s.MaxConcurrentActive = 10
	if u := e.Evaluate(s); !hasID(u, "grid_full") {
		t.Error("grid_full not unlocked at 10")
	}
}

func TestSpectacle_CrashSurvivor(t *testing.T) {
	e := NewAchievementEngine()

	// Only errors, no completions
	s := newStats()
	s.TotalErrors = 1
	if u := e.Evaluate(s); hasID(u, "crash_survivor") {
		t.Error("crash_survivor unlocked without completions")
	}

	// Only completions, no errors
	e = NewAchievementEngine()
	s = newStats()
	s.TotalCompletions = 1
	if u := e.Evaluate(s); hasID(u, "crash_survivor") {
		t.Error("crash_survivor unlocked without errors")
	}

	// Both
	e = NewAchievementEngine()
	s = newStats()
	s.TotalErrors = 1
	s.TotalCompletions = 1
	if u := e.Evaluate(s); !hasID(u, "crash_survivor") {
		t.Error("crash_survivor not unlocked with error + completion")
	}
}

func TestSpectacle_BurningRubber(t *testing.T) {
	e := NewAchievementEngine()

	// Below threshold: only 2 sessions simultaneously above 50%
	s := newStats()
	s.MaxHighUtilizationSimultaneous = 2
	if u := e.Evaluate(s); hasID(u, "burning_rubber") {
		t.Error("burning_rubber unlocked with only 2 simultaneous high-utilization sessions")
	}

	// At threshold: exactly 3 sessions simultaneously above 50%
	e = NewAchievementEngine()
	s = newStats()
	s.MaxHighUtilizationSimultaneous = 3
	if u := e.Evaluate(s); !hasID(u, "burning_rubber") {
		t.Error("burning_rubber not unlocked with 3 simultaneous high-utilization sessions")
	}
}

func TestStreaks_HatTrick(t *testing.T) {
	e := NewAchievementEngine()

	s := newStats()
	s.ConsecutiveCompletions = 2
	if u := e.Evaluate(s); hasID(u, "hat_trick") {
		t.Error("hat_trick unlocked at 2")
	}

	e = NewAchievementEngine()
	s = newStats()
	s.ConsecutiveCompletions = 3
	if u := e.Evaluate(s); !hasID(u, "hat_trick") {
		t.Error("hat_trick not unlocked at 3")
	}
}

func TestStreaks_OnARoll(t *testing.T) {
	e := NewAchievementEngine()

	s := newStats()
	s.ConsecutiveCompletions = 9
	if u := e.Evaluate(s); hasID(u, "on_a_roll") {
		t.Error("on_a_roll unlocked at 9")
	}

	e = NewAchievementEngine()
	s = newStats()
	s.ConsecutiveCompletions = 10
	if u := e.Evaluate(s); !hasID(u, "on_a_roll") {
		t.Error("on_a_roll not unlocked at 10")
	}
}

func TestStreaks_Untouchable(t *testing.T) {
	e := NewAchievementEngine()

	s := newStats()
	s.ConsecutiveCompletions = 24
	if u := e.Evaluate(s); hasID(u, "untouchable") {
		t.Error("untouchable unlocked at 24")
	}

	e = NewAchievementEngine()
	s = newStats()
	s.ConsecutiveCompletions = 25
	if u := e.Evaluate(s); !hasID(u, "untouchable") {
		t.Error("untouchable not unlocked at 25")
	}
}

func TestEdge_SimultaneousMultiUnlock(t *testing.T) {
	e := NewAchievementEngine()
	s := newStats()

	// Stats that satisfy many achievements at once
	s.TotalSessions = 500
	s.TotalCompletions = 100
	s.TotalErrors = 5
	s.ConsecutiveCompletions = 25
	s.SessionsPerSource["claude"] = 10
	s.SessionsPerSource["gemini"] = 10
	s.SessionsPerSource["codex"] = 10
	s.SessionsPerModel["claude-opus-4"] = 5
	s.SessionsPerModel["claude-sonnet-4"] = 5
	s.SessionsPerModel["claude-haiku-3.5"] = 5
	s.DistinctModelsUsed = 10
	s.MaxContextUtilization = 0.99
	s.MaxBurnRate = 10000
	s.MaxConcurrentActive = 10
	s.MaxHighUtilizationSimultaneous = 10
	s.MaxToolCalls = 500
	s.MaxMessages = 200
	s.MaxSessionDurationSec = 7200

	unlocked := e.Evaluate(s)
	if len(unlocked) < 10 {
		t.Errorf("expected many simultaneous unlocks, got %d", len(unlocked))
	}

	// Every returned achievement should be recorded
	for _, a := range unlocked {
		if _, ok := s.AchievementsUnlocked[a.ID]; !ok {
			t.Errorf("unlocked achievement %q not in AchievementsUnlocked map", a.ID)
		}
	}
}

func TestEdge_AllAchievementsUnlocked_ReturnsEmpty(t *testing.T) {
	e := NewAchievementEngine()
	s := newStats()

	// Pre-mark every achievement as already unlocked
	for _, a := range e.Registry() {
		s.AchievementsUnlocked[a.ID] = time.Now().UTC()
	}

	unlocked := e.Evaluate(s)
	if len(unlocked) != 0 {
		t.Errorf("all pre-unlocked: got %d new unlocks, want 0", len(unlocked))
	}
}

func TestEdge_CleanSweepAndOnARoll_BothUnlockAtSameThreshold(t *testing.T) {
	// clean_sweep (Performance & Endurance) and on_a_roll (Streaks) share
	// the same threshold of 10 consecutive completions.
	e := NewAchievementEngine()
	s := newStats()
	s.ConsecutiveCompletions = 10

	unlocked := e.Evaluate(s)
	if !hasID(unlocked, "clean_sweep") {
		t.Error("clean_sweep not unlocked at ConsecutiveCompletions=10")
	}
	if !hasID(unlocked, "on_a_roll") {
		t.Error("on_a_roll not unlocked at ConsecutiveCompletions=10")
	}
}

func TestEdge_PartialPreUnlock_OnlyNewReturned(t *testing.T) {
	e := NewAchievementEngine()
	s := newStats()
	s.TotalSessions = 10

	// Pre-unlock first_lap
	s.AchievementsUnlocked["first_lap"] = time.Now().UTC()

	unlocked := e.Evaluate(s)
	if hasID(unlocked, "first_lap") {
		t.Error("first_lap re-emitted despite being pre-unlocked")
	}
	if !hasID(unlocked, "pit_crew") {
		t.Error("pit_crew not unlocked at TotalSessions=10")
	}
}

func TestEdge_AboveThreshold_StillUnlocks(t *testing.T) {
	e := NewAchievementEngine()
	s := newStats()
	s.TotalSessions = 999 // well above 500

	unlocked := e.Evaluate(s)
	if !hasID(unlocked, "track_legend") {
		t.Error("track_legend should unlock when well above threshold")
	}
}
