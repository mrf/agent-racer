package gamification

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/agent-racer/backend/internal/session"
)

// startTracker creates a StatsTracker backed by a temp directory, starts its
// Run loop, and returns the tracker plus its event channel. The Run goroutine
// and context are cleaned up automatically when the test finishes.
func startTracker(t *testing.T) (*StatsTracker, chan<- session.Event) {
	t.Helper()
	dir := t.TempDir()
	store := NewStore(dir)
	tracker, eventCh, err := NewStatsTracker(store, 0)
	if err != nil {
		t.Fatalf("NewStatsTracker error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		tracker.Run(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	return tracker, eventCh
}

func TestStatsTracker_NewStatsTracker_LoadsExistingStats(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	// Pre-populate with some stats
	initial := newStats()
	initial.TotalSessions = 100
	initial.TotalCompletions = 50
	initial.TotalErrors = 5
	if err := store.Save(initial); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	// Create tracker - should load existing stats
	tracker, _, err := NewStatsTracker(store, 0)
	if err != nil {
		t.Fatalf("NewStatsTracker error: %v", err)
	}

	stats := tracker.Stats()
	if stats.TotalSessions != 100 {
		t.Errorf("TotalSessions = %d, want 100", stats.TotalSessions)
	}
	if stats.TotalCompletions != 50 {
		t.Errorf("TotalCompletions = %d, want 50", stats.TotalCompletions)
	}
	if stats.TotalErrors != 5 {
		t.Errorf("TotalErrors = %d, want 5", stats.TotalErrors)
	}
}

func TestStatsTracker_EventNew_IncrementsTotalSessions(t *testing.T) {
	tracker, eventCh := startTracker(t)

	eventCh <- session.Event{
		Type: session.EventNew,
		State: &session.SessionState{
			ID:     "session1",
			Source: "claude-code",
			Model:  "claude-opus-4",
		},
		ActiveCount: 1,
	}

	time.Sleep(100 * time.Millisecond)

	stats := tracker.Stats()
	if stats.TotalSessions != 1 {
		t.Errorf("TotalSessions = %d, want 1", stats.TotalSessions)
	}
}

func TestStatsTracker_EventNew_CountsSessionsPerSource(t *testing.T) {
	tracker, eventCh := startTracker(t)

	eventCh <- session.Event{
		Type: session.EventNew,
		State: &session.SessionState{
			ID:     "s1",
			Source: "claude-code",
		},
		ActiveCount: 1,
	}
	eventCh <- session.Event{
		Type: session.EventNew,
		State: &session.SessionState{
			ID:     "s2",
			Source: "cli",
		},
		ActiveCount: 2,
	}
	eventCh <- session.Event{
		Type: session.EventNew,
		State: &session.SessionState{
			ID:     "s3",
			Source: "claude-code",
		},
		ActiveCount: 3,
	}

	time.Sleep(100 * time.Millisecond)

	stats := tracker.Stats()
	if stats.TotalSessions != 3 {
		t.Errorf("TotalSessions = %d, want 3", stats.TotalSessions)
	}
	if stats.SessionsPerSource["claude-code"] != 2 {
		t.Errorf("SessionsPerSource[claude-code] = %d, want 2", stats.SessionsPerSource["claude-code"])
	}
	if stats.SessionsPerSource["cli"] != 1 {
		t.Errorf("SessionsPerSource[cli] = %d, want 1", stats.SessionsPerSource["cli"])
	}
	if stats.DistinctSourcesUsed != 2 {
		t.Errorf("DistinctSourcesUsed = %d, want 2", stats.DistinctSourcesUsed)
	}
}

func TestStatsTracker_EventNew_TracksMaxConcurrentActive(t *testing.T) {
	tracker, eventCh := startTracker(t)

	eventCh <- session.Event{
		Type:        session.EventNew,
		State:       &session.SessionState{ID: "s1", Source: "test"},
		ActiveCount: 5,
	}
	eventCh <- session.Event{
		Type:        session.EventNew,
		State:       &session.SessionState{ID: "s2", Source: "test"},
		ActiveCount: 12,
	}
	eventCh <- session.Event{
		Type:        session.EventNew,
		State:       &session.SessionState{ID: "s3", Source: "test"},
		ActiveCount: 8,
	}

	time.Sleep(100 * time.Millisecond)

	stats := tracker.Stats()
	if stats.MaxConcurrentActive != 12 {
		t.Errorf("MaxConcurrentActive = %d, want 12", stats.MaxConcurrentActive)
	}
}

func TestStatsTracker_EventTerminal_Complete_IncrementsCompletions(t *testing.T) {
	tracker, eventCh := startTracker(t)

	// Register the session first
	eventCh <- session.Event{
		Type:        session.EventNew,
		State:       &session.SessionState{ID: "s1", Source: "test"},
		ActiveCount: 1,
	}

	// Then complete it
	completedAt := time.Now()
	eventCh <- session.Event{
		Type: session.EventTerminal,
		State: &session.SessionState{
			ID:          "s1",
			Source:      "test",
			Activity:    session.Complete,
			CompletedAt: &completedAt,
		},
		ActiveCount: 0,
	}

	time.Sleep(100 * time.Millisecond)

	stats := tracker.Stats()
	if stats.TotalCompletions != 1 {
		t.Errorf("TotalCompletions = %d, want 1", stats.TotalCompletions)
	}
	if stats.ConsecutiveCompletions != 1 {
		t.Errorf("ConsecutiveCompletions = %d, want 1", stats.ConsecutiveCompletions)
	}
}

func TestStatsTracker_EventTerminal_Error_ResetsStreak(t *testing.T) {
	tracker, eventCh := startTracker(t)

	// Send successful completions first
	completedAt := time.Now()
	for i := 0; i < 3; i++ {
		eventCh <- session.Event{
			Type:        session.EventNew,
			State:       &session.SessionState{ID: fmt.Sprintf("s%d", i), Source: "test"},
			ActiveCount: 1,
		}
	}
	time.Sleep(50 * time.Millisecond)

	for i := 0; i < 3; i++ {
		eventCh <- session.Event{
			Type: session.EventTerminal,
			State: &session.SessionState{
				ID:          fmt.Sprintf("s%d", i),
				Source:      "test",
				Activity:    session.Complete,
				CompletedAt: &completedAt,
			},
			ActiveCount: 0,
		}
	}
	time.Sleep(50 * time.Millisecond)

	stats := tracker.Stats()
	if stats.ConsecutiveCompletions != 3 {
		t.Errorf("After 3 completions, ConsecutiveCompletions = %d, want 3", stats.ConsecutiveCompletions)
	}

	// Now error should reset streak
	eventCh <- session.Event{
		Type: session.EventTerminal,
		State: &session.SessionState{
			ID:       "s4",
			Source:   "test",
			Activity: session.Errored,
		},
		ActiveCount: 0,
	}
	time.Sleep(50 * time.Millisecond)

	stats = tracker.Stats()
	if stats.ConsecutiveCompletions != 0 {
		t.Errorf("After error, ConsecutiveCompletions = %d, want 0", stats.ConsecutiveCompletions)
	}
	if stats.TotalErrors != 1 {
		t.Errorf("TotalErrors = %d, want 1", stats.TotalErrors)
	}
}

func TestStatsTracker_EventTerminal_Lost_ResetsStreak(t *testing.T) {
	tracker, eventCh := startTracker(t)

	// Build up a streak
	completedAt := time.Now()
	eventCh <- session.Event{
		Type: session.EventTerminal,
		State: &session.SessionState{
			ID:          "s1",
			Activity:    session.Complete,
			CompletedAt: &completedAt,
		},
		ActiveCount: 0,
	}
	time.Sleep(50 * time.Millisecond)

	// Lost resets streak without incrementing errors
	eventCh <- session.Event{
		Type: session.EventTerminal,
		State: &session.SessionState{
			ID:       "s2",
			Activity: session.Lost,
		},
		ActiveCount: 0,
	}
	time.Sleep(50 * time.Millisecond)

	stats := tracker.Stats()
	if stats.ConsecutiveCompletions != 0 {
		t.Errorf("After Lost, ConsecutiveCompletions = %d, want 0", stats.ConsecutiveCompletions)
	}
	if stats.TotalErrors != 0 {
		t.Errorf("After Lost, TotalErrors = %d, want 0", stats.TotalErrors)
	}
}

func TestStatsTracker_EventTerminal_IncrementsErrors(t *testing.T) {
	tracker, eventCh := startTracker(t)

	eventCh <- session.Event{
		Type: session.EventTerminal,
		State: &session.SessionState{
			ID:       "s1",
			Activity: session.Errored,
		},
		ActiveCount: 0,
	}
	eventCh <- session.Event{
		Type: session.EventTerminal,
		State: &session.SessionState{
			ID:       "s2",
			Activity: session.Errored,
		},
		ActiveCount: 0,
	}

	time.Sleep(100 * time.Millisecond)

	stats := tracker.Stats()
	if stats.TotalErrors != 2 {
		t.Errorf("TotalErrors = %d, want 2", stats.TotalErrors)
	}
}

func TestStatsTracker_PeakMetrics_OnlyIncrease(t *testing.T) {
	tracker, eventCh := startTracker(t)

	// Send update with high context utilization
	eventCh <- session.Event{
		Type: session.EventUpdate,
		State: &session.SessionState{
			ID:                 "s1",
			ContextUtilization: 0.95,
			BurnRatePerMinute:  1000.0,
		},
		ActiveCount: 1,
	}
	time.Sleep(50 * time.Millisecond)

	stats := tracker.Stats()
	if stats.MaxContextUtilization != 0.95 {
		t.Errorf("MaxContextUtilization = %f, want 0.95", stats.MaxContextUtilization)
	}
	if stats.MaxBurnRate != 1000.0 {
		t.Errorf("MaxBurnRate = %f, want 1000.0", stats.MaxBurnRate)
	}

	// Send lower values - should not decrease
	eventCh <- session.Event{
		Type: session.EventUpdate,
		State: &session.SessionState{
			ID:                 "s2",
			ContextUtilization: 0.5,
			BurnRatePerMinute:  500.0,
		},
		ActiveCount: 2,
	}
	time.Sleep(50 * time.Millisecond)

	stats = tracker.Stats()
	if stats.MaxContextUtilization != 0.95 {
		t.Errorf("MaxContextUtilization should not decrease, got %f, want 0.95", stats.MaxContextUtilization)
	}
	if stats.MaxBurnRate != 1000.0 {
		t.Errorf("MaxBurnRate should not decrease, got %f, want 1000.0", stats.MaxBurnRate)
	}
}

func TestStatsTracker_EventTerminal_TracksPeakMetrics(t *testing.T) {
	tracker, eventCh := startTracker(t)

	completedAt := time.Now()
	startedAt := completedAt.Add(-1 * time.Hour)

	eventCh <- session.Event{
		Type: session.EventTerminal,
		State: &session.SessionState{
			ID:            "s1",
			Activity:      session.Complete,
			CompletedAt:   &completedAt,
			StartedAt:     startedAt,
			Model:         "claude-opus-4",
			ToolCallCount: 42,
			MessageCount:  150,
		},
		ActiveCount: 0,
	}

	time.Sleep(100 * time.Millisecond)

	stats := tracker.Stats()
	if stats.SessionsPerModel["claude-opus-4"] != 1 {
		t.Errorf("SessionsPerModel[claude-opus-4] = %d, want 1", stats.SessionsPerModel["claude-opus-4"])
	}
	if stats.DistinctModelsUsed != 1 {
		t.Errorf("DistinctModelsUsed = %d, want 1", stats.DistinctModelsUsed)
	}
	if stats.MaxToolCalls != 42 {
		t.Errorf("MaxToolCalls = %d, want 42", stats.MaxToolCalls)
	}
	if stats.MaxMessages != 150 {
		t.Errorf("MaxMessages = %d, want 150", stats.MaxMessages)
	}
	if stats.MaxSessionDurationSec != 3600.0 {
		t.Errorf("MaxSessionDurationSec = %f, want 3600.0", stats.MaxSessionDurationSec)
	}
}

func TestStatsTracker_DeduplicatesSessions(t *testing.T) {
	tracker, eventCh := startTracker(t)

	// Send EventNew for same session multiple times
	for i := 0; i < 3; i++ {
		eventCh <- session.Event{
			Type:        session.EventNew,
			State:       &session.SessionState{ID: "s1", Source: "test"},
			ActiveCount: 1,
		}
	}

	time.Sleep(100 * time.Millisecond)

	stats := tracker.Stats()
	if stats.TotalSessions != 1 {
		t.Errorf("TotalSessions = %d, want 1 (duplicates should be ignored)", stats.TotalSessions)
	}
}

func TestStatsTracker_Stats_ReturnsCopy(t *testing.T) {
	tracker, eventCh := startTracker(t)

	eventCh <- session.Event{
		Type:        session.EventNew,
		State:       &session.SessionState{ID: "s1", Source: "test"},
		ActiveCount: 1,
	}
	time.Sleep(50 * time.Millisecond)

	stats1 := tracker.Stats()
	// Modify the returned copy
	stats1.TotalSessions = 999
	stats1.SessionsPerSource["modified"] = 123

	// Get new copy
	stats2 := tracker.Stats()
	if stats2.TotalSessions != 1 {
		t.Errorf("Stats should return a copy; TotalSessions = %d, want 1", stats2.TotalSessions)
	}
	if _, ok := stats2.SessionsPerSource["modified"]; ok {
		t.Error("Stats should return a copy; modifications should not affect internal state")
	}
}

func TestStatsTracker_DebouncedSave(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	tracker, eventCh, err := NewStatsTracker(store, 0)
	if err != nil {
		t.Fatalf("NewStatsTracker error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		tracker.Run(ctx)
		close(done)
	}()

	// Send multiple rapid events
	for i := 0; i < 10; i++ {
		eventCh <- session.Event{
			Type:        session.EventNew,
			State:       &session.SessionState{ID: fmt.Sprintf("s%d", i), Source: "test"},
			ActiveCount: 1,
		}
	}

	// Stats should be updated immediately in memory
	time.Sleep(50 * time.Millisecond)
	stats := tracker.Stats()
	if stats.TotalSessions != 10 {
		t.Errorf("TotalSessions in memory = %d, want 10", stats.TotalSessions)
	}

	// Cancel context triggers a final save
	cancel()
	<-done

	// Load from disk to verify persistence
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if loaded.TotalSessions != 10 {
		t.Errorf("Persisted TotalSessions = %d, want 10", loaded.TotalSessions)
	}
}

func TestStatsTracker_SavesOnContextCancel(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	tracker, eventCh, err := NewStatsTracker(store, 0)
	if err != nil {
		t.Fatalf("NewStatsTracker error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		tracker.Run(ctx)
		close(done)
	}()

	eventCh <- session.Event{
		Type:        session.EventNew,
		State:       &session.SessionState{ID: "s1", Source: "test"},
		ActiveCount: 1,
	}

	time.Sleep(50 * time.Millisecond)

	// Cancel context - should trigger final save
	cancel()
	<-done

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if loaded.TotalSessions != 1 {
		t.Errorf("TotalSessions after context cancel = %d, want 1", loaded.TotalSessions)
	}
}

func TestStatsTracker_ThreadSafety(t *testing.T) {
	tracker, eventCh := startTracker(t)

	var wg sync.WaitGroup
	numGoroutines := 10
	eventsPerGoroutine := 50

	// Multiple goroutines sending events and reading stats concurrently
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < eventsPerGoroutine; i++ {
				eventCh <- session.Event{
					Type:        session.EventNew,
					State:       &session.SessionState{ID: fmt.Sprintf("s%d-%d", id, i), Source: "test"},
					ActiveCount: 1,
				}

				// Interleave Stats() calls to test concurrency
				_ = tracker.Stats()
			}
		}(g)
	}

	// Continuously read stats from another goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = tracker.Stats()
			time.Sleep(1 * time.Millisecond)
		}
	}()

	wg.Wait()

	stats := tracker.Stats()
	if stats.TotalSessions != numGoroutines*eventsPerGoroutine {
		t.Errorf("TotalSessions = %d, want %d", stats.TotalSessions, numGoroutines*eventsPerGoroutine)
	}
}
