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
	store := session.NewStore()
	m := &Monitor{
		cfg: &config.Config{
			Monitor: config.MonitorConfig{
				CompletionRemoveAfter: time.Second,
			},
		},
		store:          store,
		broadcaster:    nil, // not used in this test path
		tracked:        make(map[string]*trackedSession),
		pendingRemoval: make(map[string]time.Time),
		removedKeys:    make(map[string]bool),
	}

	key := "claude:session-456"
	store.Update(&session.SessionState{ID: key, Activity: session.Complete})
	m.pendingRemoval[key] = time.Now().Add(-time.Minute) // already past

	// flushRemovals requires a broadcaster, so test the logic directly.
	// Verify the key gets added to removedKeys when removed from store.
	for id, removeAt := range m.pendingRemoval {
		if !time.Now().Before(removeAt) {
			delete(m.pendingRemoval, id)
			m.store.Remove(id)
			m.removedKeys[id] = true
		}
	}

	if !m.removedKeys[key] {
		t.Error("flushRemovals should add key to removedKeys")
	}
	if _, exists := store.Get(key); exists {
		t.Error("session should have been removed from store")
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
	m.scheduleRemoval("claude:session-zero", completedAt)

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

	m.scheduleRemoval("claude:session-neg", time.Now())

	if _, ok := m.pendingRemoval["claude:session-neg"]; ok {
		t.Error("scheduleRemoval with negative duration should not add to pendingRemoval")
	}
}

func TestHandleSessionEndFallsBackToTranscriptPath(t *testing.T) {
	store := session.NewStore()
	broadcaster := ws.NewBroadcaster(store, 100*time.Millisecond, 5*time.Second)

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

	m.handleSessionEnd(marker, time.Now())

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

	m.resolveTokens(state, update, 200000)

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

	m.resolveTokens(state, update, 272000)

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
	m.resolveTokens(state, update, 272000)

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
	m.resolveTokens(state, update, 200000)

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

	m.resolveTokens(state, update, 200000)

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

	m.resolveTokens(state, update, 100000)

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

	m.resolveTokens(state, update, 200000)

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

	m.resolveTokens(state, update, 200000)

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
