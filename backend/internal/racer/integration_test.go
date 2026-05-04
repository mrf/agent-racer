package racer

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mrf/agentwatch/monitor"
	"github.com/mrf/agentwatch/session"
	"github.com/mrf/agentwatch/source"
)

// stubIntegrationSource provides canned data for integration tests that
// exercise agentwatch monitor → racer.Sink end-to-end. It returns the
// configured handles and updates, clearing updates after first parse to
// simulate cursor advancement.
type stubIntegrationSource struct {
	mu      sync.Mutex
	name    string
	handles []source.SessionHandle
	updates map[string]source.SourceUpdate
}

func newStubIntegrationSource(name string) *stubIntegrationSource {
	return &stubIntegrationSource{
		name:    name,
		updates: make(map[string]source.SourceUpdate),
	}
}

func (s *stubIntegrationSource) Name() string { return s.name }

func (s *stubIntegrationSource) Discover(_ context.Context) ([]source.SessionHandle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.handles, nil
}

func (s *stubIntegrationSource) Parse(_ context.Context, h source.SessionHandle, cursor source.Cursor) (source.SourceUpdate, source.Cursor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.updates[h.ID]
	if !ok {
		return source.SourceUpdate{}, cursor, nil
	}
	delete(s.updates, h.ID)
	return u, source.Cursor(h.ID + "-parsed"), nil
}

func (s *stubIntegrationSource) setHandles(h []source.SessionHandle) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handles = h
}

func (s *stubIntegrationSource) setUpdate(id string, u source.SourceUpdate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updates[id] = u
}

// TestIntegration_MonitorToSink verifies that an agentwatch Monitor with a
// real source delivers events to a racer.Sink, which correctly computes
// lane assignments, positions, and triggers overtake callbacks.
func TestIntegration_MonitorToSink(t *testing.T) {
	src := newStubIntegrationSource("claude")
	sink := NewSink()

	awMon, err := monitor.New(
		monitor.WithSources(src),
		monitor.WithPollInterval(50*time.Millisecond),
		monitor.WithSink(sink),
		monitor.WithStaleThreshold(2*time.Minute),
		monitor.WithCompletionRetention(30*time.Second),
	)
	if err != nil {
		t.Fatalf("failed to create agentwatch monitor: %v", err)
	}

	now := time.Now()
	src.setHandles([]source.SessionHandle{
		{ID: "sess-a", Source: "claude", WorkingDir: "/home/user/alpha", StartedAt: now},
		{ID: "sess-b", Source: "claude", WorkingDir: "/home/user/beta", StartedAt: now},
	})
	src.setUpdate("sess-a", source.SourceUpdate{
		SessionID:         "sess-a",
		Activity:          session.ActivityWorking,
		Model:             "claude-opus-4-6",
		MessageCountDelta: 5,
		ContextTokens:     10000,
		MaxContextTokens:  200000,
		LastActivityAt:    now,
		WorkingDir:        "/home/user/alpha",
	})
	src.setUpdate("sess-b", source.SourceUpdate{
		SessionID:         "sess-b",
		Activity:          session.ActivityWorking,
		Model:             "claude-sonnet-4-6",
		MessageCountDelta: 3,
		ContextTokens:     5000,
		MaxContextTokens:  200000,
		LastActivityAt:    now,
		WorkingDir:        "/home/user/beta",
	})

	// Single poll cycle.
	if err := awMon.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce failed: %v", err)
	}

	// Verify the sink has both sessions.
	all := sink.GetAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(all))
	}

	// Verify lanes are distinct.
	if all[0].Lane == all[1].Lane {
		t.Errorf("sessions should have different lanes: both have %d", all[0].Lane)
	}

	// Verify positions are assigned (highest utilization = P1).
	sessA := sink.Get("sess-a")
	sessB := sink.Get("sess-b")
	if sessA == nil || sessB == nil {
		t.Fatal("one or both sessions not found in sink")
	}

	// sess-a has higher context tokens (10000 vs 5000), so higher utilization.
	if sessA.Position != 1 {
		t.Errorf("sess-a Position = %d, want 1 (higher utilization)", sessA.Position)
	}
	if sessB.Position != 2 {
		t.Errorf("sess-b Position = %d, want 2 (lower utilization)", sessB.Position)
	}

	// Verify naming from WorkingDir.
	if sessA.Name != "alpha" {
		t.Errorf("sess-a Name = %q, want %q", sessA.Name, "alpha")
	}
	if sessB.Name != "beta" {
		t.Errorf("sess-b Name = %q, want %q", sessB.Name, "beta")
	}
}

// TestIntegration_OvertakeViaMonitor verifies that overtake detection works
// through the full agentwatch monitor → Sink pipeline.
func TestIntegration_OvertakeViaMonitor(t *testing.T) {
	src := newStubIntegrationSource("claude")

	var mu sync.Mutex
	var overtakes []OvertakeEvent

	sink := NewSink(WithOvertakeCallback(func(ev OvertakeEvent) {
		mu.Lock()
		overtakes = append(overtakes, ev)
		mu.Unlock()
	}))

	awMon, err := monitor.New(
		monitor.WithSources(src),
		monitor.WithPollInterval(50*time.Millisecond),
		monitor.WithSink(sink),
		monitor.WithStaleThreshold(2*time.Minute),
		monitor.WithCompletionRetention(30*time.Second),
	)
	if err != nil {
		t.Fatalf("failed to create agentwatch monitor: %v", err)
	}

	now := time.Now()

	// Poll 1: a=P1 (higher util), b=P2.
	src.setHandles([]source.SessionHandle{
		{ID: "a", Source: "claude", WorkingDir: "/a", StartedAt: now},
		{ID: "b", Source: "claude", WorkingDir: "/b", StartedAt: now},
	})
	src.setUpdate("a", source.SourceUpdate{
		SessionID: "a", Activity: session.ActivityWorking,
		ContextTokens: 18000, MaxContextTokens: 200000, LastActivityAt: now,
	})
	src.setUpdate("b", source.SourceUpdate{
		SessionID: "b", Activity: session.ActivityWorking,
		ContextTokens: 6000, MaxContextTokens: 200000, LastActivityAt: now,
	})

	if err := awMon.PollOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Verify initial positions.
	if a := sink.Get("a"); a.Position != 1 {
		t.Errorf("poll 1: a.Position = %d, want 1", a.Position)
	}

	// Poll 2: b jumps ahead of a.
	src.setUpdate("b", source.SourceUpdate{
		SessionID: "b", Activity: session.ActivityWorking,
		ContextTokens: 180000, MaxContextTokens: 200000, LastActivityAt: now.Add(time.Second),
	})
	src.setUpdate("a", source.SourceUpdate{
		SessionID: "a", Activity: session.ActivityWorking,
		LastActivityAt: now.Add(time.Second),
	})

	if err := awMon.PollOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Verify overtake.
	b := sink.Get("b")
	if b.Position != 1 {
		t.Errorf("poll 2: b.Position = %d, want 1", b.Position)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(overtakes) != 1 {
		t.Fatalf("expected 1 overtake event, got %d", len(overtakes))
	}
	if overtakes[0].OvertakerID != "b" {
		t.Errorf("overtaker = %s, want b", overtakes[0].OvertakerID)
	}
	if overtakes[0].OvertakenID != "a" {
		t.Errorf("overtaken = %s, want a", overtakes[0].OvertakenID)
	}
}

// TestIntegration_TerminalViaLifecycleEvent verifies that when agentwatch
// marks a session terminal via a lifecycle event, the Sink correctly clears
// its position and recalculates remaining positions.
//
// NOTE: The terminal SourceUpdate must explicitly set Activity to
// ActivityTerminal. Without it, agentwatch propagates the previous Activity
// (e.g. ActivityWorking) in the delta event, and the Sink's handleDelta
// overwrites the ActivityTerminal set by handleLifecycle. This is a known
// limitation — the Sink's handleDelta replaces the entire SessionState
// without preserving lifecycle-set Activity.
func TestIntegration_TerminalViaLifecycleEvent(t *testing.T) {
	src := newStubIntegrationSource("claude")
	sink := NewSink()

	awMon, err := monitor.New(
		monitor.WithSources(src),
		monitor.WithPollInterval(50*time.Millisecond),
		monitor.WithSink(sink),
		monitor.WithStaleThreshold(500*time.Millisecond),
		monitor.WithCompletionRetention(100*time.Millisecond),
	)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	src.setHandles([]source.SessionHandle{
		{ID: "a", Source: "claude", WorkingDir: "/a", StartedAt: now},
		{ID: "b", Source: "claude", WorkingDir: "/b", StartedAt: now},
	})
	src.setUpdate("a", source.SourceUpdate{
		SessionID: "a", Activity: session.ActivityWorking,
		ContextTokens: 10000, MaxContextTokens: 200000, LastActivityAt: now,
	})
	src.setUpdate("b", source.SourceUpdate{
		SessionID: "b", Activity: session.ActivityWorking,
		ContextTokens: 5000, MaxContextTokens: 200000, LastActivityAt: now,
	})

	if err := awMon.PollOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	// a=P1, b=P2.
	if a := sink.Get("a"); a == nil || a.Position != 1 {
		t.Fatal("expected a at position 1")
	}

	// Make a terminal. Explicitly set Activity to ActivityTerminal so
	// the delta's SessionState reflects the terminal state.
	src.setUpdate("a", source.SourceUpdate{
		SessionID: "a", Activity: session.ActivityTerminal,
		Terminal: true, EndReason: "complete",
		LastActivityAt: now.Add(time.Second),
	})
	// b still active, provide some data to trigger a delta.
	src.setUpdate("b", source.SourceUpdate{
		SessionID: "b", Activity: session.ActivityWorking,
		LastActivityAt: now.Add(time.Second),
	})

	if err := awMon.PollOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	// a should be terminal with position 0; b should be P1.
	a := sink.Get("a")
	if a == nil {
		t.Fatal("session a should still exist (terminal, not yet removed)")
	}
	if a.Position != 0 {
		t.Errorf("terminal session a.Position = %d, want 0", a.Position)
	}

	b := sink.Get("b")
	if b == nil {
		t.Fatal("session b not found")
	}
	if b.Position != 1 {
		t.Errorf("b.Position = %d, want 1 (only non-terminal session)", b.Position)
	}
}

// TestIntegration_StatsEventsViaMonitor verifies that stats events are
// emitted through the full monitor → Sink pipeline.
func TestIntegration_StatsEventsViaMonitor(t *testing.T) {
	src := newStubIntegrationSource("claude")
	ch := make(chan StatsEvent, 16)
	sink := NewSink(WithStatsChannel(ch))

	awMon, err := monitor.New(
		monitor.WithSources(src),
		monitor.WithPollInterval(50*time.Millisecond),
		monitor.WithSink(sink),
		monitor.WithStaleThreshold(2*time.Minute),
		monitor.WithCompletionRetention(30*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	src.setHandles([]source.SessionHandle{
		{ID: "s1", Source: "claude", WorkingDir: "/s1", StartedAt: now},
	})
	src.setUpdate("s1", source.SourceUpdate{
		SessionID: "s1", Activity: session.ActivityWorking,
		LastActivityAt: now,
	})

	if err := awMon.PollOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Should have received a StatsEventNew for the new session.
	select {
	case ev := <-ch:
		if ev.Type != StatsEventNew {
			t.Errorf("first event type = %d, want StatsEventNew (%d)", ev.Type, StatsEventNew)
		}
		if ev.State.ID != "s1" {
			t.Errorf("event session ID = %s, want s1", ev.State.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for stats event")
	}
}

// TestIntegration_ConcurrentPollAndRead verifies that concurrent PollOnce
// and Sink.GetAll/Get calls do not deadlock. This is the integration-level
// counterpart of TestConcurrentAccess.
func TestIntegration_ConcurrentPollAndRead(t *testing.T) {
	src := newStubIntegrationSource("claude")
	sink := NewSink()

	awMon, err := monitor.New(
		monitor.WithSources(src),
		monitor.WithPollInterval(50*time.Millisecond),
		monitor.WithSink(sink),
		monitor.WithStaleThreshold(2*time.Minute),
		monitor.WithCompletionRetention(30*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	src.setHandles([]source.SessionHandle{
		{ID: "race", Source: "claude", WorkingDir: "/race", StartedAt: now},
	})

	done := make(chan struct{})
	var wg sync.WaitGroup

	// Poller goroutine.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			src.setUpdate("race", source.SourceUpdate{
				SessionID: "race", Activity: session.ActivityWorking,
				ContextTokens: i * 100, MaxContextTokens: 200000,
				LastActivityAt: now.Add(time.Duration(i) * time.Millisecond),
			})
			_ = awMon.PollOnce(context.Background())
		}
	}()

	// Reader goroutine.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = sink.GetAll()
			_ = sink.Get("race")
		}
	}()

	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// No deadlock.
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent poll+read test timed out — possible deadlock")
	}
}
