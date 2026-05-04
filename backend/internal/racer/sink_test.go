package racer

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mrf/agentwatch/monitor"
	"github.com/mrf/agentwatch/session"
)

func makeSession(id, source string, util float64) session.SessionState {
	return session.SessionState{
		ID:                 id,
		Source:             source,
		Activity:           session.ActivityWorking,
		ContextUtilization: util,
		WorkingDir:         "/home/user/projects/" + id,
		StartedAt:          time.Now(),
	}
}

func makeTerminalSession(id, source string) session.SessionState {
	t := time.Now()
	return session.SessionState{
		ID:          id,
		Source:      source,
		Activity:    session.ActivityTerminal,
		WorkingDir:  "/home/user/projects/" + id,
		StartedAt:   time.Now(),
		CompletedAt: &t,
	}
}

// drain empties a stats event channel.
func drain(ch <-chan StatsEvent) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

// --- Lane Assignment ---

func TestLaneAssignmentOnSnapshot(t *testing.T) {
	sink := NewSink()

	ev := monitor.Event{
		Type: monitor.EventSnapshot,
		Sessions: []session.SessionState{
			makeSession("a", "claude", 0.5),
			makeSession("b", "codex", 0.3),
		},
	}
	if err := sink.HandleEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}

	all := sink.GetAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(all))
	}

	lanes := map[string]int{}
	for _, rs := range all {
		lanes[rs.ID] = rs.Lane
	}

	// Lanes should be distinct.
	if lanes["a"] == lanes["b"] {
		t.Errorf("sessions a and b have same lane %d", lanes["a"])
	}
}

func TestLanePreservedAcrossSnapshots(t *testing.T) {
	sink := NewSink()

	ev1 := monitor.Event{
		Type:     monitor.EventSnapshot,
		Sessions: []session.SessionState{makeSession("a", "claude", 0.5)},
	}
	if err := sink.HandleEvent(context.Background(), ev1); err != nil {
		t.Fatal(err)
	}

	laneA := sink.Get("a").Lane

	// Second snapshot with same session.
	ev2 := monitor.Event{
		Type:     monitor.EventSnapshot,
		Sessions: []session.SessionState{makeSession("a", "claude", 0.8)},
	}
	if err := sink.HandleEvent(context.Background(), ev2); err != nil {
		t.Fatal(err)
	}

	if got := sink.Get("a").Lane; got != laneA {
		t.Errorf("lane changed from %d to %d", laneA, got)
	}
}

func TestLanePreservedOnDeltaUpdate(t *testing.T) {
	sink := NewSink()

	// Initial snapshot.
	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type:     monitor.EventSnapshot,
		Sessions: []session.SessionState{makeSession("a", "claude", 0.3)},
	}); err != nil {
		t.Fatal(err)
	}

	laneA := sink.Get("a").Lane

	// Delta update for same session.
	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type:    monitor.EventDelta,
		Updates: []session.SessionState{makeSession("a", "claude", 0.7)},
	}); err != nil {
		t.Fatal(err)
	}

	if got := sink.Get("a").Lane; got != laneA {
		t.Errorf("lane changed from %d to %d after delta", laneA, got)
	}
}

func TestNewSessionInDeltaGetsLane(t *testing.T) {
	sink := NewSink()

	// Initial snapshot with one session.
	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type:     monitor.EventSnapshot,
		Sessions: []session.SessionState{makeSession("a", "claude", 0.3)},
	}); err != nil {
		t.Fatal(err)
	}

	// Delta introduces a new session.
	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type:    monitor.EventDelta,
		Updates: []session.SessionState{makeSession("b", "codex", 0.5)},
	}); err != nil {
		t.Fatal(err)
	}

	b := sink.Get("b")
	if b == nil {
		t.Fatal("session b not found")
	}
	if b.Lane == sink.Get("a").Lane {
		t.Error("new session b got same lane as a")
	}
}

// --- Position Computation ---

func TestPositionsByContextUtilization(t *testing.T) {
	sink := NewSink()

	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type: monitor.EventSnapshot,
		Sessions: []session.SessionState{
			makeSession("low", "claude", 0.2),
			makeSession("high", "claude", 0.9),
			makeSession("mid", "claude", 0.5),
		},
	}); err != nil {
		t.Fatal(err)
	}

	high := sink.Get("high")
	mid := sink.Get("mid")
	low := sink.Get("low")

	if high.Position != 1 {
		t.Errorf("high position = %d, want 1", high.Position)
	}
	if mid.Position != 2 {
		t.Errorf("mid position = %d, want 2", mid.Position)
	}
	if low.Position != 3 {
		t.Errorf("low position = %d, want 3", low.Position)
	}
}

func TestTerminalSessionsHaveNoPosition(t *testing.T) {
	sink := NewSink()

	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type: monitor.EventSnapshot,
		Sessions: []session.SessionState{
			makeSession("active", "claude", 0.5),
			makeTerminalSession("done", "claude"),
		},
	}); err != nil {
		t.Fatal(err)
	}

	done := sink.Get("done")
	if done.Position != 0 {
		t.Errorf("terminal session position = %d, want 0", done.Position)
	}

	active := sink.Get("active")
	if active.Position != 1 {
		t.Errorf("active session position = %d, want 1", active.Position)
	}
}

func TestPositionDeltaOnUpdate(t *testing.T) {
	sink := NewSink()

	// Initial: a=P1, b=P2.
	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type: monitor.EventSnapshot,
		Sessions: []session.SessionState{
			makeSession("a", "claude", 0.8),
			makeSession("b", "claude", 0.3),
		},
	}); err != nil {
		t.Fatal(err)
	}

	// b overtakes a.
	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type: monitor.EventDelta,
		Updates: []session.SessionState{
			makeSession("b", "claude", 0.9),
			makeSession("a", "claude", 0.8),
		},
	}); err != nil {
		t.Fatal(err)
	}

	b := sink.Get("b")
	a := sink.Get("a")

	if b.Position != 1 {
		t.Errorf("b position = %d, want 1", b.Position)
	}
	if b.PositionDelta != 1 {
		t.Errorf("b positionDelta = %d, want 1 (moved up)", b.PositionDelta)
	}
	if a.Position != 2 {
		t.Errorf("a position = %d, want 2", a.Position)
	}
	if a.PositionDelta != -1 {
		t.Errorf("a positionDelta = %d, want -1 (dropped)", a.PositionDelta)
	}
}

// --- Overtake Detection ---

func TestOvertakeCallback(t *testing.T) {
	var mu sync.Mutex
	var overtakes []OvertakeEvent

	sink := NewSink(WithOvertakeCallback(func(ev OvertakeEvent) {
		mu.Lock()
		overtakes = append(overtakes, ev)
		mu.Unlock()
	}))

	// Initial: a=P1, b=P2.
	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type: monitor.EventSnapshot,
		Sessions: []session.SessionState{
			makeSession("a", "claude", 0.8),
			makeSession("b", "codex", 0.3),
		},
	}); err != nil {
		t.Fatal(err)
	}

	// b overtakes a.
	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type: monitor.EventDelta,
		Updates: []session.SessionState{
			makeSession("b", "codex", 0.9),
			makeSession("a", "claude", 0.8),
		},
	}); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(overtakes) != 1 {
		t.Fatalf("expected 1 overtake, got %d", len(overtakes))
	}
	ov := overtakes[0]
	if ov.OvertakerID != "b" {
		t.Errorf("overtaker = %s, want b", ov.OvertakerID)
	}
	if ov.OvertakenID != "a" {
		t.Errorf("overtaken = %s, want a", ov.OvertakenID)
	}
	if ov.NewPosition != 1 {
		t.Errorf("new position = %d, want 1", ov.NewPosition)
	}
}

func TestNoOvertakeWhenNoMovement(t *testing.T) {
	called := false
	sink := NewSink(WithOvertakeCallback(func(_ OvertakeEvent) {
		called = true
	}))

	// Initial: a=P1, b=P2.
	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type: monitor.EventSnapshot,
		Sessions: []session.SessionState{
			makeSession("a", "claude", 0.8),
			makeSession("b", "codex", 0.3),
		},
	}); err != nil {
		t.Fatal(err)
	}

	// Same order, no overtake.
	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type: monitor.EventDelta,
		Updates: []session.SessionState{
			makeSession("a", "claude", 0.9),
			makeSession("b", "codex", 0.4),
		},
	}); err != nil {
		t.Fatal(err)
	}

	if called {
		t.Error("overtake callback should not have been called")
	}
}

// --- Stats Channel ---

func TestStatsEventOnNewSession(t *testing.T) {
	ch := make(chan StatsEvent, 16)
	sink := NewSink(WithStatsChannel(ch))

	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type:    monitor.EventDelta,
		Updates: []session.SessionState{makeSession("a", "claude", 0.5)},
	}); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-ch:
		if ev.Type != StatsEventNew {
			t.Errorf("event type = %d, want StatsEventNew (%d)", ev.Type, StatsEventNew)
		}
		if ev.State.ID != "a" {
			t.Errorf("event session ID = %s, want a", ev.State.ID)
		}
	default:
		t.Fatal("expected stats event, got none")
	}
}

func TestStatsEventOnUpdate(t *testing.T) {
	ch := make(chan StatsEvent, 16)
	sink := NewSink(WithStatsChannel(ch))

	// First event: new session.
	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type:     monitor.EventSnapshot,
		Sessions: []session.SessionState{makeSession("a", "claude", 0.3)},
	}); err != nil {
		t.Fatal(err)
	}

	// Drain snapshot events (snapshots don't emit individual stats currently).
	drain(ch)

	// Delta update for existing session.
	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type:    monitor.EventDelta,
		Updates: []session.SessionState{makeSession("a", "claude", 0.7)},
	}); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-ch:
		if ev.Type != StatsEventUpdate {
			t.Errorf("event type = %d, want StatsEventUpdate (%d)", ev.Type, StatsEventUpdate)
		}
	default:
		t.Fatal("expected stats update event, got none")
	}
}

func TestStatsEventOnTerminal(t *testing.T) {
	ch := make(chan StatsEvent, 16)
	sink := NewSink(WithStatsChannel(ch))

	// Seed a session.
	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type:     monitor.EventSnapshot,
		Sessions: []session.SessionState{makeSession("a", "claude", 0.5)},
	}); err != nil {
		t.Fatal(err)
	}

	drain(ch)

	// Terminal lifecycle event.
	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type: monitor.EventLifecycle,
		Lifecycle: &session.LifecycleEvent{
			Type:      session.EventTerminal,
			SessionID: "a",
			At:        time.Now(),
			Reason:    "complete",
		},
	}); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-ch:
		if ev.Type != StatsEventTerminal {
			t.Errorf("event type = %d, want StatsEventTerminal (%d)", ev.Type, StatsEventTerminal)
		}
	default:
		t.Fatal("expected stats terminal event, got none")
	}
}

func TestStatsDropOnFullChannel(t *testing.T) {
	// Channel of size 0 (unbuffered) — sends will always be dropped
	// since no goroutine is receiving.
	ch := make(chan StatsEvent)
	sink := NewSink(WithStatsChannel(ch))

	// This should not block even though nobody reads the channel.
	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type:    monitor.EventDelta,
		Updates: []session.SessionState{makeSession("a", "claude", 0.5)},
	}); err != nil {
		t.Fatal(err)
	}
}

// --- Lifecycle Events ---

func TestLifecycleTerminalClearsPosition(t *testing.T) {
	sink := NewSink()

	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type: monitor.EventSnapshot,
		Sessions: []session.SessionState{
			makeSession("a", "claude", 0.8),
			makeSession("b", "codex", 0.3),
		},
	}); err != nil {
		t.Fatal(err)
	}

	// a was P1 — mark it terminal.
	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type: monitor.EventLifecycle,
		Lifecycle: &session.LifecycleEvent{
			Type:      session.EventTerminal,
			SessionID: "a",
			At:        time.Now(),
		},
	}); err != nil {
		t.Fatal(err)
	}

	a := sink.Get("a")
	if a.Position != 0 {
		t.Errorf("terminal session a position = %d, want 0", a.Position)
	}

	// b should now be P1.
	b := sink.Get("b")
	if b.Position != 1 {
		t.Errorf("session b position = %d, want 1", b.Position)
	}
}

func TestLifecycleRemovedDeletesSession(t *testing.T) {
	sink := NewSink()

	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type:     monitor.EventSnapshot,
		Sessions: []session.SessionState{makeSession("a", "claude", 0.5)},
	}); err != nil {
		t.Fatal(err)
	}

	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type: monitor.EventLifecycle,
		Lifecycle: &session.LifecycleEvent{
			Type:      session.EventRemoved,
			SessionID: "a",
			At:        time.Now(),
		},
	}); err != nil {
		t.Fatal(err)
	}

	if got := sink.Get("a"); got != nil {
		t.Error("session a should have been removed")
	}
}

// --- Naming ---

func TestNameFromWorktreePath(t *testing.T) {
	ss := session.SessionState{
		ID:         "test",
		WorkingDir: "/home/user/projects/myrepo/.claude/worktrees/fix-login-bug",
	}
	got := nameFromSession(&ss)
	if got != "fix-login-bug" {
		t.Errorf("name = %q, want %q", got, "fix-login-bug")
	}
}

func TestNameFromSlug(t *testing.T) {
	ss := session.SessionState{
		ID:         "test",
		Slug:       "mighty-castle",
		WorkingDir: "/home/user/projects/myrepo",
	}
	got := nameFromSession(&ss)
	if got != "mighty-castle" {
		t.Errorf("name = %q, want %q", got, "mighty-castle")
	}
}

func TestNameFromPlainPath(t *testing.T) {
	ss := session.SessionState{
		ID:         "test",
		WorkingDir: "/home/user/projects/myrepo",
	}
	got := nameFromSession(&ss)
	if got != "myrepo" {
		t.Errorf("name = %q, want %q", got, "myrepo")
	}
}

// --- Delta Removal ---

func TestDeltaRemovesSession(t *testing.T) {
	sink := NewSink()

	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type: monitor.EventSnapshot,
		Sessions: []session.SessionState{
			makeSession("a", "claude", 0.5),
			makeSession("b", "codex", 0.3),
		},
	}); err != nil {
		t.Fatal(err)
	}

	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type:    monitor.EventDelta,
		Removed: []string{"a"},
	}); err != nil {
		t.Fatal(err)
	}

	if sink.Get("a") != nil {
		t.Error("session a should have been removed")
	}
	if sink.Get("b") == nil {
		t.Error("session b should still exist")
	}
}

// --- Health Events ---

func TestHealthEventIsIgnored(t *testing.T) {
	sink := NewSink()

	// Seed a session.
	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type:     monitor.EventSnapshot,
		Sessions: []session.SessionState{makeSession("a", "claude", 0.5)},
	}); err != nil {
		t.Fatal(err)
	}

	// Health event should be a no-op.
	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type: monitor.EventHealth,
		Health: &monitor.Health{
			Source: "claude",
			Status: monitor.HealthDegraded,
		},
	}); err != nil {
		t.Fatal(err)
	}

	// Session should be unchanged.
	a := sink.Get("a")
	if a == nil {
		t.Fatal("session a disappeared after health event")
	}
}

// --- Concurrent Safety ---

func TestConcurrentAccess(t *testing.T) {
	sink := NewSink()

	done := make(chan struct{})
	var wg sync.WaitGroup

	// Writer goroutine.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = sink.HandleEvent(context.Background(), monitor.Event{
				Type:    monitor.EventDelta,
				Updates: []session.SessionState{makeSession("a", "claude", float64(i)/100)},
			})
		}
	}()

	// Reader goroutine.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = sink.GetAll()
			_ = sink.Get("a")
		}
	}()

	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent test timed out — possible deadlock")
	}
}

// --- Deadlock Detection ---
// Ported from session/store_test.go to verify that the Sink's lock
// discipline allows safe concurrent access during event processing.

// deadlockTimeout is the maximum time we allow a Sink operation to complete
// before declaring a deadlock.
const deadlockTimeout = 2 * time.Second

// mustCompleteWithin runs f in a goroutine and fails the test if f does not
// return within the given timeout. A timeout means the goroutine is permanently
// blocked — the classic symptom of mutex re-entrancy.
func mustCompleteWithin(t *testing.T, timeout time.Duration, desc string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		f()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Errorf("DEADLOCK: %s did not complete within %v (goroutine is permanently blocked)", desc, timeout)
	}
}

func TestConcurrentHandleEventAndGet(t *testing.T) {
	sink := NewSink()

	// Seed some data.
	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type: monitor.EventSnapshot,
		Sessions: []session.SessionState{
			makeSession("a", "claude", 0.5),
			makeSession("b", "codex", 0.3),
		},
	}); err != nil {
		t.Fatal(err)
	}

	// Verify GetAll and Get complete without deadlock during HandleEvent.
	mustCompleteWithin(t, deadlockTimeout, "GetAll during snapshot", func() {
		for i := 0; i < 50; i++ {
			_ = sink.GetAll()
		}
	})

	mustCompleteWithin(t, deadlockTimeout, "Get during delta", func() {
		for i := 0; i < 50; i++ {
			_ = sink.Get("a")
		}
	})
}

func TestConcurrentOvertakeCallbackDoesNotDeadlock(t *testing.T) {
	// The overtake callback runs outside the Sink's lock. Verify that
	// calling Get/GetAll from inside the callback does not deadlock.
	var sinkRef *Sink
	sink := NewSink(WithOvertakeCallback(func(ev OvertakeEvent) {
		// This should NOT deadlock — the callback runs outside the lock.
		mustCompleteWithin(t, deadlockTimeout, "Get inside overtake callback", func() {
			_ = sinkRef.GetAll()
		})
	}))
	sinkRef = sink

	// Establish initial positions: a=P1, b=P2.
	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type: monitor.EventSnapshot,
		Sessions: []session.SessionState{
			makeSession("a", "claude", 0.8),
			makeSession("b", "codex", 0.3),
		},
	}); err != nil {
		t.Fatal(err)
	}

	// b overtakes a — triggers callback.
	mustCompleteWithin(t, deadlockTimeout, "HandleEvent with overtake callback", func() {
		if err := sink.HandleEvent(context.Background(), monitor.Event{
			Type: monitor.EventDelta,
			Updates: []session.SessionState{
				makeSession("b", "codex", 0.9),
				makeSession("a", "claude", 0.8),
			},
		}); err != nil {
			t.Error(err)
		}
	})
}

func TestConcurrentStatsChannelDoesNotDeadlock(t *testing.T) {
	// The stats channel uses non-blocking sends. Verify that even with a
	// full channel, HandleEvent completes without deadlocking.
	ch := make(chan StatsEvent, 1) // small buffer
	sink := NewSink(WithStatsChannel(ch))

	// Fill the channel so sends will be dropped.
	ch <- StatsEvent{}

	mustCompleteWithin(t, deadlockTimeout, "HandleEvent with full stats channel", func() {
		_ = sink.HandleEvent(context.Background(), monitor.Event{
			Type:    monitor.EventDelta,
			Updates: []session.SessionState{makeSession("a", "claude", 0.5)},
		})
	})
}

// --- Snapshot Replacement ---

func TestSnapshotReplacesAllSessions(t *testing.T) {
	sink := NewSink()

	// Initial snapshot with a and b.
	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type: monitor.EventSnapshot,
		Sessions: []session.SessionState{
			makeSession("a", "claude", 0.5),
			makeSession("b", "codex", 0.3),
		},
	}); err != nil {
		t.Fatal(err)
	}

	if len(sink.GetAll()) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sink.GetAll()))
	}

	// Second snapshot with only c — a and b should be gone.
	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type: monitor.EventSnapshot,
		Sessions: []session.SessionState{
			makeSession("c", "gemini", 0.7),
		},
	}); err != nil {
		t.Fatal(err)
	}

	all := sink.GetAll()
	if len(all) != 1 {
		t.Fatalf("expected 1 session after replacement snapshot, got %d", len(all))
	}
	if all[0].ID != "c" {
		t.Errorf("remaining session ID = %s, want c", all[0].ID)
	}
	if sink.Get("a") != nil {
		t.Error("session a should have been replaced")
	}
	if sink.Get("b") != nil {
		t.Error("session b should have been replaced")
	}
}

// --- Stats ActiveCount Accuracy ---

func TestStatsEventActiveCount(t *testing.T) {
	ch := make(chan StatsEvent, 16)
	sink := NewSink(WithStatsChannel(ch))

	// Create 3 sessions via delta — all active.
	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type: monitor.EventDelta,
		Updates: []session.SessionState{
			makeSession("a", "claude", 0.5),
			makeSession("b", "codex", 0.3),
			makeSession("c", "gemini", 0.8),
		},
	}); err != nil {
		t.Fatal(err)
	}

	// Drain — the last event should have ActiveCount=3.
	var lastEvent StatsEvent
	eventCount := 0
loop:
	for {
		select {
		case ev := <-ch:
			lastEvent = ev
			eventCount++
		default:
			break loop
		}
	}
	if eventCount == 0 {
		t.Fatal("expected stats events")
	}
	// All 3 sessions are active (non-terminal).
	if lastEvent.ActiveCount != 3 {
		t.Errorf("ActiveCount = %d, want 3", lastEvent.ActiveCount)
	}
}

// --- GetAll Returns Copies ---

func TestGetAllReturnsCopies(t *testing.T) {
	sink := NewSink()

	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type:     monitor.EventSnapshot,
		Sessions: []session.SessionState{makeSession("a", "claude", 0.5)},
	}); err != nil {
		t.Fatal(err)
	}

	all := sink.GetAll()
	all[0].Lane = 999

	fresh := sink.Get("a")
	if fresh.Lane == 999 {
		t.Error("GetAll did not return a copy — mutation leaked into sink")
	}
}

func TestGetReturnsCopy(t *testing.T) {
	sink := NewSink()

	if err := sink.HandleEvent(context.Background(), monitor.Event{
		Type:     monitor.EventSnapshot,
		Sessions: []session.SessionState{makeSession("a", "claude", 0.5)},
	}); err != nil {
		t.Fatal(err)
	}

	got := sink.Get("a")
	got.Lane = 999

	fresh := sink.Get("a")
	if fresh.Lane == 999 {
		t.Error("Get did not return a copy — mutation leaked into sink")
	}
}
