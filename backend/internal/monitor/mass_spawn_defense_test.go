package monitor

import (
	"context"
	"testing"
	"time"

	awmonitor "github.com/mrf/agentwatch/monitor"
	awsession "github.com/mrf/agentwatch/session"
	awsource "github.com/mrf/agentwatch/source"

	"github.com/agent-racer/backend/internal/session"
)

// TestDeltaRemovedTriggersStoreRemoval verifies that sessions listed in
// ev.Removed are removed from the local store even without a preceding
// lifecycle EventRemoved (defense-in-depth).
func TestDeltaRemovedTriggersStoreRemoval(t *testing.T) {
	src := newStubSource("claude")
	mon, store, broadcaster := newTestEnv(src)
	defer broadcaster.Stop()

	now := time.Now()

	// Discover and poll a session so it lands in the store.
	src.setHandles([]awsource.SessionHandle{
		{ID: "sess-1", Source: "claude", StartedAt: now},
	})
	src.setUpdate("sess-1", awsource.SourceUpdate{
		SessionID:      "sess-1",
		Activity:       awsession.ActivityWorking,
		LastActivityAt: now,
	})
	mon.poll(context.Background())

	if _, ok := store.Get("claude:sess-1"); !ok {
		t.Fatal("session should exist after poll")
	}

	// Simulate a delta event with ev.Removed but NO lifecycle EventRemoved.
	// This exercises the defense-in-depth path via sessionSources mapping.
	deltaEv := awmonitor.Event{
		Type:    awmonitor.EventDelta,
		Removed: []string{"sess-1"},
	}
	mon.handleDeltaEvent(deltaEv)

	if _, ok := store.Get("claude:sess-1"); ok {
		t.Error("session should be removed from store after ev.Removed processing")
	}
}

// TestDeltaRemovedDeduplicatesWithPendingRemovals verifies that a session
// appearing in both pendingRemovals (from lifecycle) and ev.Removed is not
// double-removed (no panic, no redundant work).
func TestDeltaRemovedDeduplicatesWithPendingRemovals(t *testing.T) {
	src := newStubSource("claude")
	mon, store, broadcaster := newTestEnv(src)
	defer broadcaster.Stop()

	now := time.Now()

	src.setHandles([]awsource.SessionHandle{
		{ID: "sess-1", Source: "claude", StartedAt: now},
	})
	src.setUpdate("sess-1", awsource.SourceUpdate{
		SessionID:      "sess-1",
		Activity:       awsession.ActivityWorking,
		LastActivityAt: now,
	})
	mon.poll(context.Background())

	// Simulate lifecycle EventRemoved (populates pendingRemovals).
	lcEv := awmonitor.Event{
		Type: awmonitor.EventLifecycle,
		Lifecycle: &awsession.LifecycleEvent{
			Source:    "claude",
			SessionID: "sess-1",
			Type:      awsession.EventRemoved,
		},
	}
	mon.handleLifecycleEvent(lcEv)

	// Now handle delta with the same session in ev.Removed.
	deltaEv := awmonitor.Event{
		Type:    awmonitor.EventDelta,
		Removed: []string{"sess-1"},
	}
	// Should not panic and should clean up the session.
	mon.handleDeltaEvent(deltaEv)

	if _, ok := store.Get("claude:sess-1"); ok {
		t.Error("session should be removed")
	}
}

// TestDeltaRemovedUnknownSourceIgnored verifies that ev.Removed entries with
// no prior update (unknown source) are safely ignored.
func TestDeltaRemovedUnknownSourceIgnored(t *testing.T) {
	src := newStubSource("claude")
	mon, _, broadcaster := newTestEnv(src)
	defer broadcaster.Stop()

	deltaEv := awmonitor.Event{
		Type:    awmonitor.EventDelta,
		Removed: []string{"never-seen-session"},
	}
	// Should not panic.
	mon.handleDeltaEvent(deltaEv)
}

// TestReconcileStoreOrphanedTerminalSessions verifies that reconcileStore
// prunes terminal sessions whose last data is older than the configured
// threshold.
func TestReconcileStoreOrphanedTerminalSessions(t *testing.T) {
	src := newStubSource("claude")
	mon, store, broadcaster := newTestEnv(src)
	defer broadcaster.Stop()

	now := time.Now()

	// Manually insert sessions into the store: one active, one old terminal.
	completedAt := now.Add(-10 * time.Minute)
	store.Update(&session.SessionState{
		ID:                 "claude:active-1",
		Activity:           session.Thinking,
		LastDataReceivedAt: now,
	})
	store.Update(&session.SessionState{
		ID:                 "claude:old-terminal",
		Activity:           session.Complete,
		CompletedAt:        &completedAt,
		LastDataReceivedAt: now.Add(-10 * time.Minute),
	})
	store.Update(&session.SessionState{
		ID:                 "claude:recent-terminal",
		Activity:           session.Complete,
		CompletedAt:        &completedAt,
		LastDataReceivedAt: now.Add(-30 * time.Second),
	})

	if store.Count() != 3 {
		t.Fatalf("expected 3 sessions, got %d", store.Count())
	}

	// Force reconciliation by backdating lastReconcile.
	mon.lastReconcile = time.Time{}

	mon.mu.RLock()
	cfg := mon.cfg
	mon.mu.RUnlock()

	// cfg defaults: SessionStaleAfter=2m, CompletionRemoveAfter=30s
	// maxAge = 2m + 30s + 30s = 3m. Cutoff = now - 3m.
	// old-terminal (10m old) is past cutoff → pruned.
	// recent-terminal (30s old) is within cutoff → kept.
	mon.reconcileStore(now, cfg)

	if _, ok := store.Get("claude:active-1"); !ok {
		t.Error("active session should not be pruned")
	}
	if _, ok := store.Get("claude:old-terminal"); ok {
		t.Error("old terminal session should be pruned by reconciliation")
	}
	if _, ok := store.Get("claude:recent-terminal"); !ok {
		t.Error("recent terminal session should not be pruned")
	}
}

// TestReconcileStoreThrottled verifies that reconcileStore skips when called
// within the reconcile interval.
func TestReconcileStoreThrottled(t *testing.T) {
	src := newStubSource("claude")
	mon, store, broadcaster := newTestEnv(src)
	defer broadcaster.Stop()

	now := time.Now()

	completedAt := now.Add(-10 * time.Minute)
	store.Update(&session.SessionState{
		ID:                 "claude:old",
		Activity:           session.Complete,
		CompletedAt:        &completedAt,
		LastDataReceivedAt: now.Add(-10 * time.Minute),
	})

	mon.mu.RLock()
	cfg := mon.cfg
	mon.mu.RUnlock()

	// First call: prunes.
	mon.lastReconcile = time.Time{}
	mon.reconcileStore(now, cfg)
	if _, ok := store.Get("claude:old"); ok {
		t.Error("should be pruned on first reconcile")
	}

	// Re-add and call again immediately: should be throttled.
	store.Update(&session.SessionState{
		ID:                 "claude:old2",
		Activity:           session.Complete,
		CompletedAt:        &completedAt,
		LastDataReceivedAt: now.Add(-10 * time.Minute),
	})
	mon.reconcileStore(now, cfg)
	if _, ok := store.Get("claude:old2"); !ok {
		t.Error("should NOT be pruned — reconcile is throttled")
	}
}
