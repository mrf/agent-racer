package monitor

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agent-racer/backend/internal/config"
	"github.com/agent-racer/backend/internal/session"
	"github.com/agent-racer/backend/internal/ws"
)

// countingTestSource tracks Discover calls via an atomic counter and delegates
// Parse to ParseSessionJSONL — the same code path production sources use.
// The handles field is protected by a mutex so tests can safely update it
// while Start() is running.
type countingTestSource struct {
	mu        sync.Mutex
	handles   []SessionHandle
	pollCount atomic.Int64
}

func (s *countingTestSource) Name() string { return "claude" }

func (s *countingTestSource) Discover() ([]SessionHandle, error) {
	s.pollCount.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	h := make([]SessionHandle, len(s.handles))
	copy(h, s.handles)
	return h, nil
}

func (s *countingTestSource) Parse(handle SessionHandle, offset int64) (SourceUpdate, int64, error) {
	return parseJSONLHandle(handle, offset)
}

// waitForPolls blocks until the source has been polled at least n times,
// failing the test if the deadline is exceeded.
func waitForPolls(t *testing.T, src *countingTestSource, n int64, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for src.pollCount.Load() < n && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if src.pollCount.Load() < n {
		t.Fatalf("expected >= %d polls within %v, got %d", n, timeout, src.pollCount.Load())
	}
}

// newIntegrationMonitor creates a Monitor wired to a countingTestSource,
// suitable for integration tests that exercise Start(). Process activity
// discovery is stubbed out to avoid scanning real system processes.
func newIntegrationMonitor(src *countingTestSource, cfg *config.Config) (*Monitor, *session.Store) {
	store := session.NewStore()
	broadcaster := ws.NewBroadcaster(store, 50*time.Millisecond, 10*time.Second, 0)
	m := NewMonitor(cfg, store, broadcaster, []Source{src})
	m.discoverProcessActivity = func(prevCPU map[int]cpuSample, elapsed time.Duration) ([]ProcessActivity, map[int]cpuSample) {
		return nil, prevCPU
	}
	return m, store
}

// TestStartPopulatesStoreFromRealJSONL verifies that Monitor.Start()
// discovers and parses multiple sessions from real JSONL fixture files,
// populating the session store with correct state.
func TestStartPopulatesStoreFromRealJSONL(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	ts1 := now.Format(time.RFC3339Nano)
	ts2 := now.Add(time.Second).Format(time.RFC3339Nano)

	// Session A: user + assistant with a tool call.
	pathA := filepath.Join(dir, "session-a.jsonl")
	writeJSONL(t, pathA,
		jsonlLine("user", "session-a", ts1, "", "", "/home/user/alpha")+
			jsonlLine("assistant", "session-a", ts2, "claude-opus-4-5-20251101", "Read", "/home/user/alpha"))

	// Session B: user message only.
	pathB := filepath.Join(dir, "session-b.jsonl")
	writeJSONL(t, pathB,
		jsonlLine("user", "session-b", ts1, "", "", "/home/user/beta"))

	src := &countingTestSource{
		handles: []SessionHandle{
			newTestHandle("session-a", pathA, "/home/user/alpha", now),
			newTestHandle("session-b", pathB, "/home/user/beta", now),
		},
	}

	cfg := defaultTestConfig()
	cfg.Monitor.PollInterval = 50 * time.Millisecond

	m, store := newIntegrationMonitor(src, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go m.Start(ctx)

	// Wait for at least 2 polls: when Discover fires the 2nd time, the 1st
	// poll's store update is guaranteed complete (poll() is synchronous).
	waitForPolls(t, src, 2, 2*time.Second)

	// Assert session A.
	stA, ok := store.Get("claude:session-a")
	if !ok {
		t.Fatal("session-a should exist in store")
	}
	if stA.Model != "claude-opus-4-5-20251101" {
		t.Errorf("session-a Model = %q, want %q", stA.Model, "claude-opus-4-5-20251101")
	}
	if stA.MessageCount != 2 {
		t.Errorf("session-a MessageCount = %d, want 2", stA.MessageCount)
	}
	if stA.CurrentTool != "Read" {
		t.Errorf("session-a CurrentTool = %q, want %q", stA.CurrentTool, "Read")
	}
	if stA.Name != "alpha" {
		t.Errorf("session-a Name = %q, want %q", stA.Name, "alpha")
	}

	// Assert session B.
	stB, ok := store.Get("claude:session-b")
	if !ok {
		t.Fatal("session-b should exist in store")
	}
	if stB.MessageCount != 1 {
		t.Errorf("session-b MessageCount = %d, want 1", stB.MessageCount)
	}
	if stB.Name != "beta" {
		t.Errorf("session-b Name = %q, want %q", stB.Name, "beta")
	}
}

// TestStartIncrementalParsingAcrossTicks verifies that data appended to a
// JSONL file between poll ticks is picked up incrementally — the monitor
// resumes parsing from the last offset rather than re-reading the entire file.
func TestStartIncrementalParsingAcrossTicks(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	ts1 := now.Format(time.RFC3339Nano)
	ts2 := now.Add(time.Second).Format(time.RFC3339Nano)

	path := filepath.Join(dir, "session-inc.jsonl")
	writeJSONL(t, path,
		jsonlLine("user", "session-inc", ts1, "", "", "/home/user/proj")+
			jsonlLine("assistant", "session-inc", ts2, "claude-opus-4-5-20251101", "", "/home/user/proj"))

	src := &countingTestSource{
		handles: []SessionHandle{newTestHandle("session-inc", path, "/home/user/proj", now)},
	}

	cfg := defaultTestConfig()
	cfg.Monitor.PollInterval = 50 * time.Millisecond

	m, store := newIntegrationMonitor(src, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go m.Start(ctx)

	// Wait for the initial poll to commit.
	waitForPolls(t, src, 2, 2*time.Second)

	st, ok := store.Get("claude:session-inc")
	if !ok {
		t.Fatal("session should exist after initial poll")
	}
	if st.MessageCount != 2 {
		t.Fatalf("MessageCount after first poll = %d, want 2", st.MessageCount)
	}

	// Append a tool-use message between ticks.
	ts3 := now.Add(2 * time.Second).Format(time.RFC3339Nano)
	appendJSONL(t, path,
		jsonlLine("assistant", "session-inc", ts3, "claude-opus-4-5-20251101", "Edit", "/home/user/proj"))

	// Wait for at least one full poll cycle after the append.
	pollsBefore := src.pollCount.Load()
	waitForPolls(t, src, pollsBefore+2, 2*time.Second)

	st, _ = store.Get("claude:session-inc")
	if st.MessageCount != 3 {
		t.Errorf("MessageCount after incremental poll = %d, want 3", st.MessageCount)
	}
	if st.CurrentTool != "Edit" {
		t.Errorf("CurrentTool = %q, want %q", st.CurrentTool, "Edit")
	}
	if st.ToolCallCount != 1 {
		t.Errorf("ToolCallCount = %d, want 1", st.ToolCallCount)
	}
}

// TestStartSetConfigChangesPollingWithRealData verifies that calling
// SetConfig with a shorter PollInterval causes the Start() goroutine to
// recreate its ticker and pick up new JSONL data on the faster cadence.
func TestStartSetConfigChangesPollingWithRealData(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	ts1 := now.Format(time.RFC3339Nano)
	ts2 := now.Add(time.Second).Format(time.RFC3339Nano)

	path := filepath.Join(dir, "session-cfg.jsonl")
	writeJSONL(t, path,
		jsonlLine("user", "session-cfg", ts1, "", "", "/home/user/proj")+
			jsonlLine("assistant", "session-cfg", ts2, "claude-opus-4-5-20251101", "", "/home/user/proj"))

	src := &countingTestSource{
		handles: []SessionHandle{newTestHandle("session-cfg", path, "/home/user/proj", now)},
	}

	// Start with a very long poll interval — only the initial poll fires.
	cfg := defaultTestConfig()
	cfg.Monitor.PollInterval = 10 * time.Second

	m, store := newIntegrationMonitor(src, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go m.Start(ctx)

	// Wait for initial poll (Discover fires once on Start).
	waitForPolls(t, src, 1, 2*time.Second)

	// With a 10s interval, no more polls should happen in 200ms.
	time.Sleep(200 * time.Millisecond)
	if src.pollCount.Load() != 1 {
		t.Fatalf("unexpected extra polls during 10s interval: got %d", src.pollCount.Load())
	}

	// Verify initial data was committed. waitForPolls only guarantees Discover
	// was called, not that the full poll cycle (parse + store update) finished.
	// With only 1 poll, we need to poll the store briefly.
	deadline := time.Now().Add(500 * time.Millisecond)
	var st *session.SessionState
	var ok bool
	for time.Now().Before(deadline) {
		st, ok = store.Get("claude:session-cfg")
		if ok {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !ok {
		t.Fatal("session should exist after initial poll")
	}
	if st.MessageCount != 2 {
		t.Fatalf("MessageCount = %d, want 2", st.MessageCount)
	}

	// Append data and hot-reload to a short poll interval.
	ts3 := now.Add(2 * time.Second).Format(time.RFC3339Nano)
	appendJSONL(t, path,
		jsonlLine("assistant", "session-cfg", ts3, "claude-opus-4-5-20251101", "Write", "/home/user/proj"))

	shortCfg := defaultTestConfig()
	shortCfg.Monitor.PollInterval = 30 * time.Millisecond
	m.SetConfig(shortCfg)

	// With 30ms polls the new data should be picked up quickly.
	pollsBefore := src.pollCount.Load()
	waitForPolls(t, src, pollsBefore+3, 2*time.Second)

	st, _ = store.Get("claude:session-cfg")
	if st.MessageCount != 3 {
		t.Errorf("MessageCount after hot-reload = %d, want 3", st.MessageCount)
	}
	if st.CurrentTool != "Write" {
		t.Errorf("CurrentTool = %q, want %q", st.CurrentTool, "Write")
	}
}

// TestStartContextCancellationStopsPolling verifies that cancelling the
// context causes Start() to return promptly and stops all polling.
func TestStartContextCancellationStopsPolling(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	ts := now.Format(time.RFC3339Nano)

	path := filepath.Join(dir, "session-stop.jsonl")
	writeJSONL(t, path,
		jsonlLine("user", "session-stop", ts, "", "", "/home/user/proj"))

	src := &countingTestSource{
		handles: []SessionHandle{newTestHandle("session-stop", path, "/home/user/proj", now)},
	}

	cfg := defaultTestConfig()
	cfg.Monitor.PollInterval = 20 * time.Millisecond

	m, _ := newIntegrationMonitor(src, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		m.Start(ctx)
		close(done)
	}()

	// Let a few polls run.
	waitForPolls(t, src, 3, 2*time.Second)

	// Cancel and verify Start() exits.
	cancel()

	select {
	case <-done:
		// Start returned.
	case <-time.After(2 * time.Second):
		t.Fatal("Start() did not return within 2s of context cancellation")
	}

	// Verify polling stopped.
	afterCancel := src.pollCount.Load()
	time.Sleep(100 * time.Millisecond)
	if src.pollCount.Load() != afterCancel {
		t.Errorf("polls continued after cancellation: before=%d, after=%d",
			afterCancel, src.pollCount.Load())
	}
}
