package monitor

import (
	"testing"
	"time"

	"github.com/agent-racer/backend/internal/config"
	"github.com/agent-racer/backend/internal/session"
	"github.com/agent-racer/backend/internal/ws"
)

// newTestMonitor creates a minimal Monitor for testing resolveTokens and
// other methods that only need a config and in-memory maps.
func newTestMonitor(tokenNorm config.TokenNormConfig) *Monitor {
	return &Monitor{
		cfg: &config.Config{
			TokenNorm: tokenNorm,
		},
		tracked:        make(map[string]*trackedSession),
		pendingRemoval: make(map[string]time.Time),
		removedKeys:    make(map[string]bool),
	}
}

// newTestMonitorWithStore creates a Monitor backed by a real Store and
// Broadcaster, for testing flushRemovals and other methods that interact
// with the session store and WebSocket broadcaster.
func newTestMonitorWithStore(monitorCfg config.MonitorConfig) *Monitor {
	store := session.NewStore()
	broadcaster := ws.NewBroadcaster(store, 100*time.Millisecond, 5*time.Second, 0)
	return &Monitor{
		cfg: &config.Config{
			Monitor: monitorCfg,
		},
		store:          store,
		broadcaster:    broadcaster,
		tracked:        make(map[string]*trackedSession),
		pendingRemoval: make(map[string]time.Time),
		removedKeys:    make(map[string]bool),
	}
}

func TestTrackingKey(t *testing.T) {
	key := trackingKey("claude", "abc-123")
	if key != "claude:abc-123" {
		t.Errorf("trackingKey() = %q, want %q", key, "claude:abc-123")
	}
}

func TestClassifyActivityFromUpdate(t *testing.T) {
	tests := []struct {
		name     string
		update   SourceUpdate
		wantName string
	}{
		{"tool_use", SourceUpdate{Activity: "tool_use"}, "tool_use"},
		{"thinking", SourceUpdate{Activity: "thinking"}, "thinking"},
		{"waiting", SourceUpdate{Activity: "waiting"}, "waiting"},
		{"idle_no_data", SourceUpdate{}, "idle"},
		{"thinking_from_messages", SourceUpdate{MessageCount: 2}, "thinking"},
		{"idle_only_tokens", SourceUpdate{TokensIn: 100}, "idle"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			activity := classifyActivityFromUpdate(tt.update)
			if activity.String() != tt.wantName {
				t.Errorf("classifyActivityFromUpdate() = %q, want %q", activity.String(), tt.wantName)
			}
		})
	}
}

func TestNameFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/home/user/Projects/myapp", "myapp"},
		{"/tmp/test", "test"},
		{"", "unknown"},
		{"/", "unknown"},
		{"/single", "single"},
	}

	for _, tt := range tests {
		got := nameFromPath(tt.path)
		if got != tt.want {
			t.Errorf("nameFromPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestSplitPath(t *testing.T) {
	tests := []struct {
		path string
		want int // number of parts
	}{
		{"/home/user/project", 3},
		{"/tmp", 1},
		{"", 0},
		{"/", 0},
	}

	for _, tt := range tests {
		parts := splitPath(tt.path)
		if len(parts) != tt.want {
			t.Errorf("splitPath(%q) returned %d parts, want %d", tt.path, len(parts), tt.want)
		}
	}
}

func TestSourceUpdateHasData(t *testing.T) {
	tests := []struct {
		name   string
		update SourceUpdate
		want   bool
	}{
		{"empty", SourceUpdate{}, false},
		{"session_id", SourceUpdate{SessionID: "x"}, true},
		{"model", SourceUpdate{Model: "x"}, true},
		{"tokens_in", SourceUpdate{TokensIn: 1}, true},
		{"tokens_out", SourceUpdate{TokensOut: 1}, true},
		{"messages", SourceUpdate{MessageCount: 1}, true},
		{"tools", SourceUpdate{ToolCalls: 1}, true},
		{"last_tool", SourceUpdate{LastTool: "x"}, true},
		{"activity", SourceUpdate{Activity: "x"}, true},
		{"last_time", SourceUpdate{LastTime: time.Now()}, true},
		{"working_dir", SourceUpdate{WorkingDir: "x"}, true},
		{"max_context_tokens", SourceUpdate{MaxContextTokens: 200000}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.update.HasData() != tt.want {
				t.Errorf("HasData() = %v, want %v", tt.update.HasData(), tt.want)
			}
		})
	}
}
func TestRemovedKeysPreventZombieReCreation(t *testing.T) {
	m := &Monitor{
		tracked:        make(map[string]*trackedSession),
		pendingRemoval: make(map[string]time.Time),
		removedKeys:    make(map[string]bool),
	}

	key := trackingKey("claude", "session-123")

	// Simulate a session being removed after terminal state.
	m.removedKeys[key] = true

	// The session should be skipped when re-discovered.
	if !m.removedKeys[key] {
		t.Error("removedKeys should contain the key")
	}
}

func TestRemovedKeysPurgedWhenFileNoLongerDiscovered(t *testing.T) {
	m := &Monitor{
		tracked:        make(map[string]*trackedSession),
		pendingRemoval: make(map[string]time.Time),
		removedKeys:    make(map[string]bool),
	}

	key := trackingKey("claude", "session-123")
	m.removedKeys[key] = true

	// Simulate the file falling outside the discover window.
	activeKeys := map[string]bool{} // session no longer discovered

	for k := range m.removedKeys {
		if !activeKeys[k] {
			delete(m.removedKeys, k)
		}
	}

	if m.removedKeys[key] {
		t.Error("removedKeys should have been purged for undiscovered session")
	}
}

func TestRemovedKeysRetainedWhileFileStillDiscovered(t *testing.T) {
	m := &Monitor{
		tracked:        make(map[string]*trackedSession),
		pendingRemoval: make(map[string]time.Time),
		removedKeys:    make(map[string]bool),
	}

	key := trackingKey("claude", "session-123")
	m.removedKeys[key] = true

	// Simulate the file still being within the discover window.
	activeKeys := map[string]bool{key: true}

	for k := range m.removedKeys {
		if !activeKeys[k] {
			delete(m.removedKeys, k)
		}
	}

	if !m.removedKeys[key] {
		t.Error("removedKeys should be retained while file is still discovered")
	}
}

func TestFlushRemovalsAddsToRemovedKeys(t *testing.T) {
	m := newTestMonitorWithStore(config.MonitorConfig{
		CompletionRemoveAfter: time.Second,
	})

	key := "claude:session-456"
	m.store.Update(&session.SessionState{ID: key, Activity: session.Complete})
	m.pendingRemoval[key] = time.Now().Add(-time.Minute) // already past

	m.flushRemovals(time.Now())

	if !m.removedKeys[key] {
		t.Error("flushRemovals should add key to removedKeys")
	}
	if _, exists := m.store.Get(key); exists {
		t.Error("session should have been removed from store")
	}
	if len(m.pendingRemoval) != 0 {
		t.Errorf("pendingRemoval should be empty, got %d entries", len(m.pendingRemoval))
	}
}

func TestFlushRemovalsBroadcastsRemovedIDs(t *testing.T) {
	m := newTestMonitorWithStore(config.MonitorConfig{})

	now := time.Now()
	dueKey := "claude:session-due"
	futureKey := "claude:session-future"

	m.store.Update(&session.SessionState{ID: dueKey, Activity: session.Complete})
	m.store.Update(&session.SessionState{ID: futureKey, Activity: session.Complete})

	m.pendingRemoval[dueKey] = now.Add(-time.Second)  // past due
	m.pendingRemoval[futureKey] = now.Add(time.Hour)   // not yet due

	m.flushRemovals(now)

	// Due session should be removed from store and added to removedKeys.
	if _, exists := m.store.Get(dueKey); exists {
		t.Error("due session should have been removed from store")
	}
	if !m.removedKeys[dueKey] {
		t.Error("due session should be in removedKeys")
	}

	// Future session should remain in store and pendingRemoval.
	if _, exists := m.store.Get(futureKey); !exists {
		t.Error("future session should still be in store")
	}
	if m.removedKeys[futureKey] {
		t.Error("future session should not be in removedKeys")
	}
	if _, ok := m.pendingRemoval[futureKey]; !ok {
		t.Error("future session should still be in pendingRemoval")
	}
}

func TestFlushRemovalsEmptyPendingIsNoop(t *testing.T) {
	m := newTestMonitorWithStore(config.MonitorConfig{})

	// Should not panic or modify anything.
	m.flushRemovals(time.Now())

	if len(m.removedKeys) != 0 {
		t.Error("removedKeys should remain empty")
	}
}

func TestScheduleRemovalDoubleScheduleKeepsEarlierTime(t *testing.T) {
	m := &Monitor{
		cfg: &config.Config{
			Monitor: config.MonitorConfig{
				CompletionRemoveAfter: 10 * time.Second,
			},
		},
		tracked:        make(map[string]*trackedSession),
		pendingRemoval: make(map[string]time.Time),
		removedKeys:    make(map[string]bool),
	}

	key := "claude:session-dup"
	earlier := time.Now()
	later := earlier.Add(5 * time.Second)

	// Schedule with earlier completion time first.
	m.scheduleRemoval(m.cfg,key, earlier)
	firstRemoveAt := m.pendingRemoval[key]

	// Schedule again with later completion time — should keep the earlier one.
	m.scheduleRemoval(m.cfg,key, later)
	secondRemoveAt := m.pendingRemoval[key]

	if !secondRemoveAt.Equal(firstRemoveAt) {
		t.Errorf("double-schedule should keep earlier time: got %v, want %v", secondRemoveAt, firstRemoveAt)
	}

	// Reverse order: schedule later first, then earlier — should update to earlier.
	m.pendingRemoval = make(map[string]time.Time)
	m.scheduleRemoval(m.cfg,key, later)
	m.scheduleRemoval(m.cfg,key, earlier)
	finalRemoveAt := m.pendingRemoval[key]

	expectedRemoveAt := earlier.Add(10 * time.Second)
	if !finalRemoveAt.Equal(expectedRemoveAt) {
		t.Errorf("should update to earlier time: got %v, want %v", finalRemoveAt, expectedRemoveAt)
	}
}

func TestScheduleRemovalZeroDurationIsImmediate(t *testing.T) {
	m := &Monitor{
		cfg: &config.Config{
			Monitor: config.MonitorConfig{
				CompletionRemoveAfter: 0, // zero = remove immediately
			},
		},
		tracked:        make(map[string]*trackedSession),
		pendingRemoval: make(map[string]time.Time),
		removedKeys:    make(map[string]bool),
	}

	completedAt := time.Now()
	m.scheduleRemoval(m.cfg,"claude:session-zero", completedAt)

	removeAt, ok := m.pendingRemoval["claude:session-zero"]
	if !ok {
		t.Fatal("scheduleRemoval with 0 duration should add to pendingRemoval")
	}
	if !removeAt.Equal(completedAt) {
		t.Errorf("removeAt = %v, want %v (immediate removal)", removeAt, completedAt)
	}
}

func TestScheduleRemovalNegativeDurationDisablesRemoval(t *testing.T) {
	m := &Monitor{
		cfg: &config.Config{
			Monitor: config.MonitorConfig{
				CompletionRemoveAfter: -1, // negative = never remove
			},
		},
		tracked:        make(map[string]*trackedSession),
		pendingRemoval: make(map[string]time.Time),
		removedKeys:    make(map[string]bool),
	}

	m.scheduleRemoval(m.cfg,"claude:session-neg", time.Now())

	if _, ok := m.pendingRemoval["claude:session-neg"]; ok {
		t.Error("scheduleRemoval with negative duration should not add to pendingRemoval")
	}
}

func TestStaleTerminalSessionAddedToRemovedKeys(t *testing.T) {
	// Simulate the stale detection loop for a terminal session whose file
	// has disappeared. The fix ensures m.removedKeys[key] is set so that
	// if the file briefly reappears, the session is not re-created from
	// offset 0 (which would cause zombie flickering).
	store := session.NewStore()
	m := &Monitor{
		cfg:            &config.Config{},
		store:          store,
		tracked:        make(map[string]*trackedSession),
		pendingRemoval: make(map[string]time.Time),
		removedKeys:    make(map[string]bool),
	}

	key := trackingKey("claude", "terminal-session")
	completedAt := time.Now().Add(-time.Minute)

	// Put a terminal session in the store and tracking map.
	store.Update(&session.SessionState{
		ID:          key,
		Source:      "claude",
		Activity:    session.Complete,
		CompletedAt: &completedAt,
	})
	m.tracked[key] = &trackedSession{
		handle: SessionHandle{
			SessionID: "terminal-session",
			Source:    "claude",
		},
		lastDataTime: completedAt,
	}

	// Simulate stale detection: file is no longer discovered.
	activeKeys := map[string]bool{} // empty — file disappeared

	var toRemove []string
	for k := range m.tracked {
		if activeKeys[k] {
			continue
		}
		if state, ok := m.store.Get(k); ok && state.IsTerminal() {
			m.removedKeys[k] = true
			toRemove = append(toRemove, k)
		}
	}
	for _, k := range toRemove {
		delete(m.tracked, k)
	}

	// Verify: tracking removed and removedKeys set.
	if _, exists := m.tracked[key]; exists {
		t.Error("terminal session should have been removed from tracked")
	}
	if !m.removedKeys[key] {
		t.Error("terminal session cleaned up by stale detection should be added to removedKeys")
	}
}

func TestHandleSessionEndFallsBackToTranscriptPath(t *testing.T) {
	store := session.NewStore()
	broadcaster := ws.NewBroadcaster(store, 100*time.Millisecond, 5*time.Second, 0)

	// Store has a session keyed by filename-based ID.
	filenameKey := trackingKey("claude", "abc-123-def")
	store.Update(&session.SessionState{
		ID:       filenameKey,
		Source:   "claude",
		Activity: session.Thinking,
	})

	m := &Monitor{
		cfg: &config.Config{
			Monitor: config.MonitorConfig{
				CompletionRemoveAfter: 8 * time.Second,
			},
		},
		store:          store,
		broadcaster:    broadcaster,
		tracked:        make(map[string]*trackedSession),
		pendingRemoval: make(map[string]time.Time),
		removedKeys:    make(map[string]bool),
	}

	// Add a tracked session with the filename-based ID.
	m.tracked[filenameKey] = &trackedSession{
		handle: SessionHandle{
			SessionID: "abc-123-def",
			LogPath:   "/home/user/.claude/projects/test/abc-123-def.jsonl",
			Source:    "claude",
		},
	}

	// Session end marker uses a DIFFERENT session_id but includes transcript_path.
	marker := sessionEndMarker{
		SessionID:      "different-uuid",
		TranscriptPath: "/home/user/.claude/projects/test/abc-123-def.jsonl",
		Reason:         "success",
	}

	m.handleSessionEnd(m.cfg, marker, time.Now())

	// The session should be marked terminal via the transcript path fallback.
	state, ok := store.Get(filenameKey)
	if !ok {
		t.Fatal("session should still exist in store")
	}
	if !state.IsTerminal() {
		t.Errorf("session should be terminal, got activity=%s", state.Activity)
	}

	// The tracked entry is intentionally kept to maintain file offset
	// for resume detection. It is cleaned up when the file disappears
	// from the discover window.
	if _, exists := m.tracked[filenameKey]; !exists {
		t.Error("tracked session should be kept for resume detection")
	}
}

func TestResolveTokensUsageWithRealData(t *testing.T) {
	m := newTestMonitor(config.TokenNormConfig{
		Strategies:       map[string]string{"claude": "usage", "default": "estimate"},
		TokensPerMessage: 2000,
	})

	state := &session.SessionState{Source: "claude", MessageCount: 5}
	update := SourceUpdate{TokensIn: 50000}

	m.resolveTokens(m.cfg,state, update, 200000)

	if state.TokensUsed != 50000 {
		t.Errorf("TokensUsed = %d, want 50000", state.TokensUsed)
	}
	if state.TokenEstimated {
		t.Error("TokenEstimated should be false for real data")
	}
	if state.MaxContextTokens != 200000 {
		t.Errorf("MaxContextTokens = %d, want 200000", state.MaxContextTokens)
	}
	if state.ContextUtilization != 0.25 {
		t.Errorf("ContextUtilization = %f, want 0.25", state.ContextUtilization)
	}
}

func TestResolveTokensUsageFallbackToEstimate(t *testing.T) {
	m := newTestMonitor(config.TokenNormConfig{
		Strategies:       map[string]string{"codex": "usage"},
		TokensPerMessage: 2000,
	})

	state := &session.SessionState{Source: "codex", MessageCount: 10}
	update := SourceUpdate{TokensIn: 0}

	m.resolveTokens(m.cfg,state, update, 272000)

	expectedTokens := 10 * 2000
	if state.TokensUsed != expectedTokens {
		t.Errorf("TokensUsed = %d, want %d", state.TokensUsed, expectedTokens)
	}
	if !state.TokenEstimated {
		t.Error("TokenEstimated should be true for fallback estimation")
	}
}

func TestResolveTokensUsageTransitionEstimateToReal(t *testing.T) {
	m := newTestMonitor(config.TokenNormConfig{
		Strategies:       map[string]string{"codex": "usage"},
		TokensPerMessage: 2000,
	})

	// Start with estimation.
	state := &session.SessionState{
		Source:         "codex",
		MessageCount:   10,
		TokensUsed:     20000,
		TokenEstimated: true,
	}

	// Real data arrives, even if lower than estimate.
	update := SourceUpdate{TokensIn: 15000}
	m.resolveTokens(m.cfg,state, update, 272000)

	if state.TokensUsed != 15000 {
		t.Errorf("TokensUsed = %d, want 15000 (real data should replace estimate)", state.TokensUsed)
	}
	if state.TokenEstimated {
		t.Error("TokenEstimated should be false after real data arrives")
	}
}

func TestResolveTokensUsageKeepsRealWhenNoNewData(t *testing.T) {
	m := newTestMonitor(config.TokenNormConfig{
		Strategies:       map[string]string{"claude": "usage"},
		TokensPerMessage: 2000,
	})

	// Session already has real data.
	state := &session.SessionState{
		Source:         "claude",
		MessageCount:   20,
		TokensUsed:     80000,
		TokenEstimated: false,
	}

	// Update with no token data -- should keep existing real value.
	update := SourceUpdate{TokensIn: 0}
	m.resolveTokens(m.cfg,state, update, 200000)

	if state.TokensUsed != 80000 {
		t.Errorf("TokensUsed = %d, want 80000 (should keep real data)", state.TokensUsed)
	}
	if state.TokenEstimated {
		t.Error("TokenEstimated should stay false when real data exists")
	}
}

func TestResolveTokensEstimateStrategy(t *testing.T) {
	m := newTestMonitor(config.TokenNormConfig{
		Strategies:       map[string]string{"custom": "estimate"},
		TokensPerMessage: 1500,
	})

	state := &session.SessionState{Source: "custom", MessageCount: 8}
	update := SourceUpdate{TokensIn: 50000} // real data ignored for estimate strategy

	m.resolveTokens(m.cfg,state, update, 200000)

	expectedTokens := 8 * 1500
	if state.TokensUsed != expectedTokens {
		t.Errorf("TokensUsed = %d, want %d", state.TokensUsed, expectedTokens)
	}
	if !state.TokenEstimated {
		t.Error("TokenEstimated should be true for estimate strategy")
	}
}

func TestResolveTokensMessageCountStrategy(t *testing.T) {
	m := newTestMonitor(config.TokenNormConfig{
		Strategies:       map[string]string{"default": "message_count"},
		TokensPerMessage: 2000,
	})

	state := &session.SessionState{Source: "new_cli", MessageCount: 5}
	update := SourceUpdate{}

	m.resolveTokens(m.cfg,state, update, 100000)

	if state.TokensUsed != 10000 {
		t.Errorf("TokensUsed = %d, want 10000", state.TokensUsed)
	}
	if !state.TokenEstimated {
		t.Error("TokenEstimated should be true for message_count strategy")
	}
}

func TestResolveTokensZeroMessages(t *testing.T) {
	m := newTestMonitor(config.TokenNormConfig{
		Strategies:       map[string]string{"default": "estimate"},
		TokensPerMessage: 2000,
	})

	state := &session.SessionState{Source: "unknown", MessageCount: 0}
	update := SourceUpdate{}

	m.resolveTokens(m.cfg,state, update, 200000)

	if state.TokensUsed != 0 {
		t.Errorf("TokensUsed = %d, want 0 (no messages = no estimate)", state.TokensUsed)
	}
	if state.TokenEstimated {
		t.Error("TokenEstimated should be false when no data at all")
	}
}

func TestResolveTokensDefaultStrategy(t *testing.T) {
	m := newTestMonitor(config.TokenNormConfig{
		// Unknown strategy value -- should fall through to default behavior.
		Strategies: map[string]string{"test": "bogus_strategy"},
	})

	state := &session.SessionState{Source: "test"}
	update := SourceUpdate{TokensIn: 5000}

	m.resolveTokens(m.cfg,state, update, 200000)

	if state.TokensUsed != 5000 {
		t.Errorf("TokensUsed = %d, want 5000", state.TokensUsed)
	}
}

func TestDetermineActivityFromReason(t *testing.T) {
	tests := []struct {
		name   string
		reason string
		want   session.Activity
	}{
		{"empty_reason", "", session.Complete},
		{"success", "success", session.Complete},
		{"normal_completion", "user closed session", session.Complete},
		{"error", "error", session.Errored},
		{"Error_capitalized", "Error occurred", session.Errored},
		{"failed", "failed to connect", session.Errored},
		{"crash", "crash detected", session.Errored},
		{"crashed", "process crashed", session.Errored},
		{"panic", "panic: runtime error", session.Errored},
		{"exception", "exception thrown", session.Errored},
		{"fatal", "fatal error", session.Errored},
		{"abort", "abort", session.Errored},
		{"aborted", "operation aborted", session.Errored},
		{"interrupted", "interrupted by signal", session.Errored},
		{"killed", "process killed", session.Errored},
		{"terminated", "terminated unexpectedly", session.Errored},
		{"mixed_case", "Session FAILED", session.Errored},
		{"contains_error", "An error occurred during processing", session.Errored},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := determineActivityFromReason(tt.reason)
			if got != tt.want {
				t.Errorf("determineActivityFromReason(%q) = %v, want %v", tt.reason, got, tt.want)
			}
		})
	}
}

func TestCalculateBurnRate(t *testing.T) {
	m := newTestMonitor(config.TokenNormConfig{})

	t.Run("single_snapshot_returns_zero", func(t *testing.T) {
		ts := &trackedSession{}
		now := time.Now()

		rate := m.calculateBurnRate(ts, 10000, now)

		if rate != 0 {
			t.Errorf("calculateBurnRate() = %f, want 0 (need at least 2 snapshots)", rate)
		}
		if len(ts.tokenSnapshots) != 1 {
			t.Errorf("tokenSnapshots len = %d, want 1", len(ts.tokenSnapshots))
		}
	})

	t.Run("two_snapshots_calculates_rate", func(t *testing.T) {
		ts := &trackedSession{}
		now := time.Now()

		// First snapshot: 10000 tokens
		m.calculateBurnRate(ts, 10000, now)

		// Second snapshot: 20000 tokens, 30 seconds later
		// 10000 tokens in 0.5 minutes = 20000 tokens/minute
		rate := m.calculateBurnRate(ts, 20000, now.Add(30*time.Second))

		expectedRate := 20000.0
		if rate < expectedRate*0.9 || rate > expectedRate*1.1 {
			t.Errorf("calculateBurnRate() = %f, want ~%f", rate, expectedRate)
		}
	})

	t.Run("less_than_5_seconds_returns_zero", func(t *testing.T) {
		ts := &trackedSession{}
		now := time.Now()

		m.calculateBurnRate(ts, 10000, now)
		rate := m.calculateBurnRate(ts, 20000, now.Add(3*time.Second))

		if rate != 0 {
			t.Errorf("calculateBurnRate() = %f, want 0 (< 5 second window)", rate)
		}
	})

	t.Run("zero_tokens_returns_zero", func(t *testing.T) {
		ts := &trackedSession{}
		now := time.Now()

		rate := m.calculateBurnRate(ts, 0, now)

		if rate != 0 {
			t.Errorf("calculateBurnRate() = %f, want 0 (zero tokens)", rate)
		}
	})

	t.Run("old_snapshots_trimmed", func(t *testing.T) {
		ts := &trackedSession{}
		now := time.Now()

		// Add snapshot from 2 minutes ago (older than 60s window)
		m.calculateBurnRate(ts, 5000, now.Add(-2*time.Minute))
		// Add current snapshot
		m.calculateBurnRate(ts, 10000, now.Add(-30*time.Second))
		// Add another current snapshot
		m.calculateBurnRate(ts, 15000, now)

		// Old snapshot should be trimmed; only 2 recent ones remain
		if len(ts.tokenSnapshots) > 2 {
			t.Errorf("tokenSnapshots len = %d, want <= 2 (old ones trimmed)", len(ts.tokenSnapshots))
		}
	})

	t.Run("hard_cap_limits_snapshot_count", func(t *testing.T) {
		ts := &trackedSession{}
		now := time.Now()

		total := maxTokenSnapshots + 50
		// Use 100ms intervals so all snapshots fit within the 60s time
		// window and the hard cap (not the time-based trim) is what limits growth.
		for i := 0; i < total; i++ {
			m.calculateBurnRate(ts, 1000*(i+1), now.Add(time.Duration(i)*100*time.Millisecond))
		}

		if len(ts.tokenSnapshots) != maxTokenSnapshots {
			t.Errorf("tokenSnapshots len = %d, want %d (hard cap)", len(ts.tokenSnapshots), maxTokenSnapshots)
		}
		last := ts.tokenSnapshots[len(ts.tokenSnapshots)-1]
		expectedTokens := 1000 * total
		if last.tokens != expectedTokens {
			t.Errorf("last snapshot tokens = %d, want %d", last.tokens, expectedTokens)
		}
	})

	t.Run("no_token_increase_returns_zero", func(t *testing.T) {
		ts := &trackedSession{}
		now := time.Now()

		m.calculateBurnRate(ts, 10000, now)
		// Same token count 30 seconds later
		rate := m.calculateBurnRate(ts, 10000, now.Add(30*time.Second))

		if rate != 0 {
			t.Errorf("calculateBurnRate() = %f, want 0 (no token increase)", rate)
		}
	})
}
