package monitor

import (
	"context"
	"sync"
	"testing"
	"time"

	awmonitor "github.com/mrf/agentwatch/monitor"
	awsession "github.com/mrf/agentwatch/session"
	awsource "github.com/mrf/agentwatch/source"

	"github.com/agent-racer/backend/internal/config"
	"github.com/agent-racer/backend/internal/session"
	"github.com/agent-racer/backend/internal/ws"
)

// stubSource returns fixed handles and updates for testing.
type stubSource struct {
	mu       sync.Mutex
	name     string
	handles  []awsource.SessionHandle
	updates  map[string]awsource.SourceUpdate
	discErr  error
	parseErr map[string]error
}

func newStubSource(name string) *stubSource {
	return &stubSource{
		name:    name,
		updates: make(map[string]awsource.SourceUpdate),
	}
}

func (s *stubSource) Name() string { return s.name }

func (s *stubSource) Discover(_ context.Context) ([]awsource.SessionHandle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.discErr != nil {
		return nil, s.discErr
	}
	return s.handles, nil
}

func (s *stubSource) Parse(_ context.Context, h awsource.SessionHandle, cursor awsource.Cursor) (awsource.SourceUpdate, awsource.Cursor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.parseErr != nil {
		if err, ok := s.parseErr[h.ID]; ok {
			return awsource.SourceUpdate{}, cursor, err
		}
	}
	u, ok := s.updates[h.ID]
	if !ok {
		return awsource.SourceUpdate{}, cursor, nil
	}
	// Return data once, then empty (simulates cursor advancement).
	delete(s.updates, h.ID)
	newCursor := awsource.Cursor(h.ID + "-parsed")
	return u, newCursor, nil
}

func (s *stubSource) setHandles(handles []awsource.SessionHandle) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handles = handles
}

func (s *stubSource) setUpdate(id string, u awsource.SourceUpdate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updates[id] = u
}

// testConfig returns a config suitable for testing.
func testConfig() *config.Config {
	cfg, _, _ := config.LoadOrDefault("/tmp/nonexistent-agent-racer-test-config.yaml")
	return cfg
}

// newTestEnv creates a Monitor wired to a stubSource, a local store, and a
// broadcaster. Process and tmux enrichments are disabled.
func newTestEnv(src *stubSource) (*Monitor, *session.Store, *ws.Broadcaster) {
	cfg := testConfig()
	cfg.Monitor.PollInterval = 100 * time.Millisecond
	cfg.Monitor.SessionStaleAfter = 2 * time.Minute
	cfg.Monitor.CompletionRemoveAfter = 30 * time.Second

	store := session.NewStore()
	broadcaster := ws.NewBroadcaster(store, 100*time.Millisecond, 5*time.Second, 0)

	mon := NewMonitor(cfg, store, broadcaster, []awsource.Source{src})
	mon.discoverProcessActivity = nil // disable process scanning in tests
	mon.newTmuxResolver = nil         // disable tmux in tests

	return mon, store, broadcaster
}

func TestPollNormalSessionLifecycle(t *testing.T) {
	src := newStubSource("claude")
	mon, store, broadcaster := newTestEnv(src)
	defer broadcaster.Stop()

	// Discover a new session.
	src.setHandles([]awsource.SessionHandle{
		{ID: "sess-1", Source: "claude", WorkingDir: "/home/user/project", StartedAt: time.Now()},
	})
	src.setUpdate("sess-1", awsource.SourceUpdate{
		SessionID:         "sess-1",
		Activity:          awsession.ActivityWorking,
		MessageCountDelta: 3,
		CurrentTool:       "Read",
		Model:             "claude-opus-4-6",
		WorkingDir:        "/home/user/project",
		LastActivityAt:    time.Now(),
	})

	mon.poll(context.Background())

	// Session should appear in the local store with composite key.
	state, ok := store.Get("claude:sess-1")
	if !ok {
		t.Fatal("session not found in store after poll")
	}
	if state.Activity != session.ToolUse {
		t.Errorf("Activity = %q, want %q", state.Activity, session.ToolUse)
	}
	if state.Model != "claude-opus-4-6" {
		t.Errorf("Model = %q, want %q", state.Model, "claude-opus-4-6")
	}
	if state.Name != "project" {
		t.Errorf("Name = %q, want %q", state.Name, "project")
	}
}

func TestPollTerminalSession(t *testing.T) {
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

	// Now make it terminal.
	src.setUpdate("sess-1", awsource.SourceUpdate{
		SessionID:      "sess-1",
		Terminal:        true,
		EndReason:       "completed successfully",
		LastActivityAt: now.Add(time.Second),
	})
	mon.poll(context.Background())

	state, ok := store.Get("claude:sess-1")
	if !ok {
		t.Fatal("session not found")
	}
	if state.Activity != session.Complete {
		t.Errorf("Activity = %q, want %q", state.Activity, session.Complete)
	}
	if state.CompletedAt == nil {
		t.Error("CompletedAt should be set for terminal session")
	}
}

func TestPollTerminalWithErrorReason(t *testing.T) {
	src := newStubSource("claude")
	mon, store, broadcaster := newTestEnv(src)
	defer broadcaster.Stop()

	now := time.Now()
	src.setHandles([]awsource.SessionHandle{
		{ID: "sess-err", Source: "claude", StartedAt: now},
	})
	src.setUpdate("sess-err", awsource.SourceUpdate{
		SessionID:      "sess-err",
		Activity:       awsession.ActivityWorking,
		LastActivityAt: now,
	})
	mon.poll(context.Background())

	src.setUpdate("sess-err", awsource.SourceUpdate{
		SessionID:      "sess-err",
		Terminal:        true,
		EndReason:       "error: connection refused",
		LastActivityAt: now.Add(time.Second),
	})
	mon.poll(context.Background())

	state, ok := store.Get("claude:sess-err")
	if !ok {
		t.Fatal("session not found")
	}
	if state.Activity != session.Errored {
		t.Errorf("Activity = %q, want %q (error reason should produce Errored)", state.Activity, session.Errored)
	}
}

func TestPollStatsEvents(t *testing.T) {
	src := newStubSource("claude")
	mon, _, broadcaster := newTestEnv(src)
	defer broadcaster.Stop()

	ch := make(chan session.Event, 10)
	mon.SetStatsEvents(ch)

	src.setHandles([]awsource.SessionHandle{
		{ID: "sess-1", Source: "claude", StartedAt: time.Now()},
	})
	src.setUpdate("sess-1", awsource.SourceUpdate{
		SessionID:      "sess-1",
		Activity:       awsession.ActivityWorking,
		LastActivityAt: time.Now(),
	})

	mon.poll(context.Background())

	select {
	case ev := <-ch:
		if ev.Type != session.EventNew {
			t.Errorf("first event type = %d, want EventNew (%d)", ev.Type, session.EventNew)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for stats event")
	}
}

func TestPollMultipleSources(t *testing.T) {
	src1 := newStubSource("claude")
	src2 := newStubSource("codex")

	cfg := testConfig()
	cfg.Monitor.PollInterval = 100 * time.Millisecond
	store := session.NewStore()
	broadcaster := ws.NewBroadcaster(store, 100*time.Millisecond, 5*time.Second, 0)
	defer broadcaster.Stop()

	mon := NewMonitor(cfg, store, broadcaster, []awsource.Source{src1, src2})
	mon.discoverProcessActivity = nil
	mon.newTmuxResolver = nil

	now := time.Now()
	src1.setHandles([]awsource.SessionHandle{
		{ID: "s1", Source: "claude", StartedAt: now},
	})
	src1.setUpdate("s1", awsource.SourceUpdate{
		SessionID: "s1", Activity: awsession.ActivityWorking, LastActivityAt: now,
	})
	src2.setHandles([]awsource.SessionHandle{
		{ID: "s2", Source: "codex", StartedAt: now},
	})
	src2.setUpdate("s2", awsource.SourceUpdate{
		SessionID: "s2", Activity: awsession.ActivityWaiting, LastActivityAt: now,
	})

	mon.poll(context.Background())

	if _, ok := store.Get("claude:s1"); !ok {
		t.Error("claude session not found")
	}
	if _, ok := store.Get("codex:s2"); !ok {
		t.Error("codex session not found")
	}
}

func TestPollSnapshotHook(t *testing.T) {
	src := newStubSource("claude")
	mon, _, broadcaster := newTestEnv(src)
	defer broadcaster.Stop()

	var hookCalled bool
	mon.SetSnapshotHook(func(states []*session.SessionState) {
		hookCalled = true
	})

	src.setHandles([]awsource.SessionHandle{
		{ID: "sess-1", Source: "claude", StartedAt: time.Now()},
	})
	src.setUpdate("sess-1", awsource.SourceUpdate{
		SessionID:      "sess-1",
		Activity:       awsession.ActivityWorking,
		LastActivityAt: time.Now(),
	})

	mon.poll(context.Background())

	if !hookCalled {
		t.Error("snapshot hook was not called")
	}
}

func TestPollProcessActivityEnrichment(t *testing.T) {
	src := newStubSource("claude")
	mon, store, broadcaster := newTestEnv(src)
	defer broadcaster.Stop()

	// Mock process activity.
	mon.discoverProcessActivity = func(prev map[int]cpuSample, elapsed time.Duration) ([]ProcessActivity, map[int]cpuSample) {
		return []ProcessActivity{
			{PID: 12345, CPU: 50.0, TCPConns: 2, WorkingDir: "/home/user/project"},
		}, prev
	}
	mon.processPollInterval = 0 // always refresh

	now := time.Now()
	src.setHandles([]awsource.SessionHandle{
		{ID: "sess-1", Source: "claude", WorkingDir: "/home/user/project", StartedAt: now},
	})
	src.setUpdate("sess-1", awsource.SourceUpdate{
		SessionID:      "sess-1",
		Activity:       awsession.ActivityWorking,
		WorkingDir:     "/home/user/project",
		LastActivityAt: now,
	})

	mon.poll(context.Background())

	state, ok := store.Get("claude:sess-1")
	if !ok {
		t.Fatal("session not found")
	}
	if state.PID != 12345 {
		t.Errorf("PID = %d, want 12345", state.PID)
	}
	if !state.IsChurning {
		t.Error("expected IsChurning=true with high CPU and TCP conns")
	}
}

func TestProcessScanThrottle(t *testing.T) {
	src := newStubSource("claude")
	mon, store, broadcaster := newTestEnv(src)
	defer broadcaster.Stop()

	scanCount := 0
	mon.discoverProcessActivity = func(prev map[int]cpuSample, elapsed time.Duration) ([]ProcessActivity, map[int]cpuSample) {
		scanCount++
		return []ProcessActivity{
			{PID: 12345, CPU: 50.0, TCPConns: 2, WorkingDir: "/home/user/project"},
		}, prev
	}
	mon.processPollInterval = 5 * time.Second

	now := time.Now()
	src.setHandles([]awsource.SessionHandle{
		{ID: "sess-1", Source: "claude", WorkingDir: "/home/user/project", StartedAt: now},
	})
	src.setUpdate("sess-1", awsource.SourceUpdate{
		SessionID:      "sess-1",
		Activity:       awsession.ActivityWorking,
		WorkingDir:     "/home/user/project",
		LastActivityAt: now,
	})

	// First poll: should scan (lastProcessPoll is zero).
	mon.poll(context.Background())
	if scanCount != 1 {
		t.Fatalf("after first poll: scanCount = %d, want 1", scanCount)
	}

	state, ok := store.Get("claude:sess-1")
	if !ok {
		t.Fatal("session not found after first poll")
	}
	if state.PID != 12345 {
		t.Errorf("PID = %d, want 12345 after first poll", state.PID)
	}

	// Second poll immediately after: should use cached data, not scan again.
	src.setUpdate("sess-1", awsource.SourceUpdate{
		SessionID:      "sess-1",
		Activity:       awsession.ActivityWorking,
		WorkingDir:     "/home/user/project",
		LastActivityAt: now.Add(time.Second),
	})
	mon.poll(context.Background())
	if scanCount != 1 {
		t.Errorf("after second poll: scanCount = %d, want 1 (throttled)", scanCount)
	}

	// Simulate time passing beyond the interval by backdating lastProcessPoll.
	mon.lastProcessPoll = time.Now().Add(-6 * time.Second)

	src.setUpdate("sess-1", awsource.SourceUpdate{
		SessionID:      "sess-1",
		Activity:       awsession.ActivityWorking,
		WorkingDir:     "/home/user/project",
		LastActivityAt: now.Add(7 * time.Second),
	})
	mon.poll(context.Background())
	if scanCount != 2 {
		t.Errorf("after interval elapsed: scanCount = %d, want 2", scanCount)
	}
}

func TestSetConfigUpdatesInterval(t *testing.T) {
	src := newStubSource("claude")
	mon, _, broadcaster := newTestEnv(src)
	defer broadcaster.Stop()

	newCfg := testConfig()
	newCfg.Monitor.PollInterval = 5 * time.Second
	mon.SetConfig(newCfg)

	// reconfigureCh should have a signal.
	select {
	case <-mon.reconfigureCh:
		// OK
	default:
		t.Error("expected reconfigureCh signal after SetConfig")
	}
}

func TestSetSourcesRebuildsMonitor(t *testing.T) {
	src1 := newStubSource("claude")
	mon, _, broadcaster := newTestEnv(src1)
	defer broadcaster.Stop()

	oldMon := mon.awMon

	src2 := newStubSource("codex")
	mon.SetSources([]awsource.Source{src1, src2})

	if mon.awMon == oldMon {
		t.Error("expected agentwatch monitor to be rebuilt")
	}
}

func TestHealthEventBroadcast(t *testing.T) {
	m := &Monitor{
		cfg:             testConfig(),
		store:           session.NewStore(),
		broadcaster:     ws.NewBroadcaster(session.NewStore(), 100*time.Millisecond, 5*time.Second, 0),
		tokenSnapshots:  make(map[string][]tokenSnapshot),
		terminalReasons: make(map[string]terminalInfo),
	}
	defer m.broadcaster.Stop()

	// Simulate a health event from agentwatch.
	ev := awmonitor.Event{
		Type: awmonitor.EventHealth,
		Health: &awmonitor.Health{
			Source:           "claude",
			Status:           awmonitor.HealthFailed,
			DiscoverFailures: 5,
			LastError:        "connection refused",
			UpdatedAt:        time.Now(),
		},
	}
	// Should not panic.
	_ = m.handleEvent(context.Background(), ev)
}
