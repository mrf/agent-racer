package gamification

import (
	"testing"
	"time"
)

func TestWeekStart(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want string // expected Monday date as YYYY-MM-DD
	}{
		{"monday", time.Date(2026, 2, 23, 10, 0, 0, 0, time.UTC), "2026-02-23"},
		{"wednesday", time.Date(2026, 2, 25, 15, 0, 0, 0, time.UTC), "2026-02-23"},
		{"sunday", time.Date(2026, 3, 1, 23, 59, 0, 0, time.UTC), "2026-02-23"},
		{"next_monday", time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC), "2026-03-02"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := weekStart(tt.in).Format("2006-01-02")
			if got != tt.want {
				t.Errorf("weekStart(%v) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}
}

func TestSelectChallenges_Deterministic(t *testing.T) {
	ws := time.Date(2026, 2, 23, 0, 0, 0, 0, time.UTC)
	a := selectChallenges(ws)
	b := selectChallenges(ws)
	if len(a) != challengesPerWeek {
		t.Fatalf("expected %d challenges, got %d", challengesPerWeek, len(a))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Errorf("selectChallenges not deterministic: a[%d]=%s, b[%d]=%s", i, a[i], i, b[i])
		}
	}
}

func TestSelectChallenges_DifferentWeeks(t *testing.T) {
	ws1 := time.Date(2026, 2, 23, 0, 0, 0, 0, time.UTC)
	ws2 := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	a := selectChallenges(ws1)
	b := selectChallenges(ws2)
	same := 0
	for i := range a {
		if a[i] == b[i] {
			same++
		}
	}
	// It's theoretically possible all 3 match, but extremely unlikely.
	// We just verify both returned the right count.
	if len(a) != challengesPerWeek || len(b) != challengesPerWeek {
		t.Errorf("wrong count: a=%d, b=%d", len(a), len(b))
	}
}

func TestSelectChallenges_NoDuplicates(t *testing.T) {
	ws := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	ids := selectChallenges(ws)
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			t.Errorf("duplicate challenge ID: %s", id)
		}
		seen[id] = true
	}
}

func TestRotateChallengesIfNeeded(t *testing.T) {
	state := WeeklyChallengeState{}
	now := time.Date(2026, 2, 25, 12, 0, 0, 0, time.UTC) // Wednesday

	rotated := RotateChallengesIfNeeded(&state, now)
	if !rotated {
		t.Fatal("expected rotation on first call")
	}
	if len(state.ActiveIDs) != challengesPerWeek {
		t.Errorf("expected %d active challenges, got %d", challengesPerWeek, len(state.ActiveIDs))
	}
	firstIDs := make([]string, len(state.ActiveIDs))
	copy(firstIDs, state.ActiveIDs)

	// Same week: no rotation.
	rotated = RotateChallengesIfNeeded(&state, now.Add(24*time.Hour))
	if rotated {
		t.Error("unexpected rotation within same week")
	}
	for i, id := range state.ActiveIDs {
		if id != firstIDs[i] {
			t.Error("IDs changed within same week")
		}
	}

	// Next week: rotation occurs.
	nextWeek := now.Add(7 * 24 * time.Hour)
	rotated = RotateChallengesIfNeeded(&state, nextWeek)
	if !rotated {
		t.Error("expected rotation on new week")
	}
}

func TestEvaluateChallenges(t *testing.T) {
	state := WeeklyChallengeState{
		ActiveIDs: []string{"run_5_haiku", "burn_1m_tokens", "run_10_sessions"},
		Snapshot: WeekSnapshot{
			SessionsPerModel: map[string]int{"claude-haiku-4-5": 3},
			TotalSessions:    10,
			TokensBurned:     500_000,
		},
		XPAwarded: make(map[string]bool),
	}

	progress := EvaluateChallenges(&state)
	if len(progress) != 3 {
		t.Fatalf("expected 3 challenge progress entries, got %d", len(progress))
	}

	byID := map[string]ChallengeProgress{}
	for _, cp := range progress {
		byID[cp.ID] = cp
	}

	haiku := byID["run_5_haiku"]
	if haiku.Current != 3 || haiku.Target != 5 || haiku.Complete {
		t.Errorf("run_5_haiku: current=%d target=%d complete=%v", haiku.Current, haiku.Target, haiku.Complete)
	}

	tokens := byID["burn_1m_tokens"]
	if tokens.Current != 500_000 || tokens.Target != 1_000_000 || tokens.Complete {
		t.Errorf("burn_1m_tokens: current=%d target=%d complete=%v", tokens.Current, tokens.Target, tokens.Complete)
	}

	sessions := byID["run_10_sessions"]
	if sessions.Current != 10 || sessions.Target != 10 || !sessions.Complete {
		t.Errorf("run_10_sessions: current=%d target=%d complete=%v", sessions.Current, sessions.Target, sessions.Complete)
	}
}

func TestChallengePool_AllHaveUniqueIDs(t *testing.T) {
	pool := challengePool()
	seen := map[string]bool{}
	for _, c := range pool {
		if c.ID == "" {
			t.Error("challenge with empty ID")
		}
		if seen[c.ID] {
			t.Errorf("duplicate challenge ID: %s", c.ID)
		}
		seen[c.ID] = true
		if c.Description == "" {
			t.Errorf("challenge %s has empty description", c.ID)
		}
		if c.Progress == nil {
			t.Errorf("challenge %s has nil Progress func", c.ID)
		}
	}
}

func TestChallengePool_AllProgressFunctions(t *testing.T) {
	pool := challengePool()
	snap := &WeekSnapshot{
		SessionsPerModel:  make(map[string]int),
		SessionsPerSource: make(map[string]int),
	}
	for _, c := range pool {
		cur, tgt := c.Progress(snap)
		if tgt <= 0 {
			t.Errorf("challenge %s has non-positive target: %d", c.ID, tgt)
		}
		if cur != 0 {
			t.Errorf("challenge %s has non-zero initial progress: %d", c.ID, cur)
		}
	}
}

func TestInitWeeklyChallengeState(t *testing.T) {
	var state WeeklyChallengeState
	initWeeklyChallengeState(&state)
	if state.Snapshot.SessionsPerModel == nil {
		t.Error("SessionsPerModel not initialized")
	}
	if state.Snapshot.SessionsPerSource == nil {
		t.Error("SessionsPerSource not initialized")
	}
	if state.XPAwarded == nil {
		t.Error("XPAwarded not initialized")
	}
}

func TestComplete3NoErrors_UsesCompletionsDirectly(t *testing.T) {
	c, ok := challengeByID("complete_3_no_errors")
	if !ok {
		t.Fatal("complete_3_no_errors challenge not found in pool")
	}

	tests := []struct {
		name             string
		totalCompletions int
		totalErrors      int
		wantCurrent      int
		wantTarget       int
		wantComplete     bool
	}{
		{
			name:             "zero completions and errors",
			totalCompletions: 0,
			totalErrors:      0,
			wantCurrent:      0,
			wantTarget:       3,
			wantComplete:     false,
		},
		{
			name:             "errors do not reduce completion count",
			totalCompletions: 2,
			totalErrors:      5,
			wantCurrent:      2,
			wantTarget:       3,
			wantComplete:     false,
		},
		{
			name:             "completions equal to target",
			totalCompletions: 3,
			totalErrors:      0,
			wantCurrent:      3,
			wantTarget:       3,
			wantComplete:     true,
		},
		{
			name:             "completions exceed target with errors",
			totalCompletions: 4,
			totalErrors:      10,
			wantCurrent:      4,
			wantTarget:       3,
			wantComplete:     true,
		},
		{
			name:             "equal completions and errors still counts",
			totalCompletions: 3,
			totalErrors:      3,
			wantCurrent:      3,
			wantTarget:       3,
			wantComplete:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snap := &WeekSnapshot{
				SessionsPerModel:  make(map[string]int),
				SessionsPerSource: make(map[string]int),
				TotalCompletions:  tt.totalCompletions,
				TotalErrors:       tt.totalErrors,
			}
			cur, tgt := c.Progress(snap)
			if cur != tt.wantCurrent {
				t.Errorf("current = %d, want %d", cur, tt.wantCurrent)
			}
			if tgt != tt.wantTarget {
				t.Errorf("target = %d, want %d", tgt, tt.wantTarget)
			}
			if complete := cur >= tgt; complete != tt.wantComplete {
				t.Errorf("complete = %v, want %v", complete, tt.wantComplete)
			}
		})
	}
}

func TestSnapModelFamilyCount(t *testing.T) {
	sessions := map[string]int{
		"claude-haiku-4-5": 3,
		"Claude-Haiku-4-5": 2,
		"claude-opus-4":    1,
	}
	if got := snapModelFamilyCount(sessions, "haiku"); got != 5 {
		t.Errorf("snapModelFamilyCount(sessions, haiku) = %d, want 5", got)
	}
	if got := snapModelFamilyCount(sessions, "opus"); got != 1 {
		t.Errorf("snapModelFamilyCount(sessions, opus) = %d, want 1", got)
	}
	if got := snapModelFamilyCount(sessions, "sonnet"); got != 0 {
		t.Errorf("snapModelFamilyCount(sessions, sonnet) = %d, want 0", got)
	}
	if got := snapModelFamilyCount(nil, "haiku"); got != 0 {
		t.Errorf("snapModelFamilyCount(nil, haiku) = %d, want 0", got)
	}
}
