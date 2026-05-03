package monitor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	awsession "github.com/mrf/agentwatch/session"
	awsource "github.com/mrf/agentwatch/source"

	"github.com/agent-racer/backend/internal/config"
	"github.com/agent-racer/backend/internal/session"
	"github.com/agent-racer/backend/internal/ws"
)

// testSource wraps real JSONL parsing with controllable discovery.
// Discover returns whatever handles are set; Parse delegates to
// ParseSessionJSONL -- the same code path the claude source uses.
// Set discoverErr to simulate Discover failures, and parseErrs
// to simulate Parse failures for specific session IDs.
type testSource struct {
	handles     []awsource.SessionHandle
	discoverErr error
	parseErrs   map[string]error // sessionID -> error
}

func (s *testSource) Name() string { return "claude" }

func (s *testSource) Discover(_ context.Context) ([]awsource.SessionHandle, error) {
	if s.discoverErr != nil {
		return nil, s.discoverErr
	}
	return s.handles, nil
}

func (s *testSource) Parse(_ context.Context, handle awsource.SessionHandle, cursor awsource.Cursor) (awsource.SourceUpdate, awsource.Cursor, error) {
	if s.parseErrs != nil {
		if err, ok := s.parseErrs[handle.ID]; ok {
			return awsource.SourceUpdate{}, cursor, err
		}
	}
	return parseJSONLHandle(handle, cursor)
}

// parseJSONLHandle is a shared test helper that parses a JSONL file from the
// given cursor and returns a SourceUpdate. Used by testSource, countingTestSource,
// and any other test source that delegates to real JSONL parsing.
func parseJSONLHandle(handle awsource.SessionHandle, cursor awsource.Cursor) (awsource.SourceUpdate, awsource.Cursor, error) {
	offset := cursorToOffset(cursor)
	result, newOffset, err := ParseSessionJSONL(handle.Path, offset, "", nil)
	if err != nil {
		return awsource.SourceUpdate{}, cursor, err
	}
	newCursor := offsetToCursor(newOffset)
	if newCursor == cursor {
		return awsource.SourceUpdate{}, cursor, nil
	}
	return sourceUpdateFromResult(result), newCursor, nil
}

// cursorToOffset converts an opaque Cursor to a byte offset for use with
// ParseSessionJSONL. The cursor is a decimal string encoding the offset.
func cursorToOffset(c awsource.Cursor) int64 {
	if c == "" {
		return 0
	}
	n, err := strconv.ParseInt(string(c), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// offsetToCursor encodes a byte offset as an opaque Cursor string.
func offsetToCursor(offset int64) awsource.Cursor {
	if offset == 0 {
		return ""
	}
	return awsource.Cursor(fmt.Sprintf("%d", offset))
}

// sourceUpdateFromResult converts a ParseResult into an awsource.SourceUpdate.
func sourceUpdateFromResult(r *ParseResult) awsource.SourceUpdate {
	update := awsource.SourceUpdate{
		SessionID:          r.SessionID,
		Slug:               r.Slug,
		Model:              r.Model,
		Activity:           parseResultActivity(r.LastActivity),
		MessageCountDelta:  r.MessageCount,
		ToolCallCountDelta: r.ToolCalls,
		CurrentTool:        r.LastTool,
		LastActivityAt:     r.LastTime,
		WorkingDir:         r.WorkingDir,
		Subagents:          parseResultSubagents(r.Subagents),
	}
	if r.LatestUsage != nil {
		update.ContextTokens = r.LatestUsage.TotalContext()
		update.OutputTokens = r.LatestUsage.OutputTokens
	}
	return update
}

// parseResultActivity maps a ParseResult activity string to awsession.Activity.
func parseResultActivity(s string) awsession.Activity {
	switch s {
	case "thinking", "tool_use":
		return awsession.ActivityWorking
	case "waiting":
		return awsession.ActivityWaiting
	default:
		return awsession.ActivityIdle
	}
}

// parseResultSubagents converts SubagentParseResult map to []awsession.SubagentState.
func parseResultSubagents(parsed map[string]*SubagentParseResult) []awsession.SubagentState {
	if len(parsed) == 0 {
		return nil
	}
	result := make([]awsession.SubagentState, 0, len(parsed))
	for _, pr := range parsed {
		activity := awsession.ActivityWorking
		switch pr.LastActivity {
		case "waiting":
			activity = awsession.ActivityWaiting
		case "":
			activity = awsession.ActivityIdle
		}
		if pr.Completed {
			activity = awsession.ActivityTerminal
		}
		result = append(result, awsession.SubagentState{
			ID:             pr.ID,
			ParentID:       pr.ParentToolUseID,
			Activity:       activity,
			CurrentTool:    pr.LastTool,
			StartedAt:      pr.FirstTime,
			LastActivityAt: pr.LastTime,
		})
	}
	return result
}

// jsonlLine builds a single JSONL entry with the given fields.
func jsonlLine(typ, sessionID, ts, model, toolName, cwd string) string {
	switch typ {
	case "user":
		line := fmt.Sprintf(
			`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hello"}]},"sessionId":"%s","timestamp":"%s"`,
			sessionID, ts,
		)
		if cwd != "" {
			line += fmt.Sprintf(`,"cwd":"%s"`, cwd)
		}
		return line + "}\n"
	case "assistant":
		content := `[{"type":"text","text":"hi"}]`
		if toolName != "" {
			content = fmt.Sprintf(`[{"type":"tool_use","name":"%s","id":"toolu_1","input":{}}]`, toolName)
		}
		line := fmt.Sprintf(
			`{"type":"assistant","message":{"model":"%s","role":"assistant","content":%s,"usage":{"input_tokens":100,"cache_creation_input_tokens":500,"cache_read_input_tokens":2000,"output_tokens":50}},"sessionId":"%s","timestamp":"%s"`,
			model, content, sessionID, ts,
		)
		if cwd != "" {
			line += fmt.Sprintf(`,"cwd":"%s"`, cwd)
		}
		return line + "}\n"
	}
	return ""
}

// writeJSONL writes JSONL content to a file, failing the test on error.
func writeJSONL(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// appendJSONL appends content to an existing JSONL file, failing the test on error.
func appendJSONL(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
}

// newTestHandle creates a SessionHandle with sensible defaults for poll tests.
func newTestHandle(sessionID, logPath, workingDir string, startedAt time.Time) awsource.SessionHandle {
	return awsource.SessionHandle{
		ID:         sessionID,
		Path:       logPath,
		WorkingDir: workingDir,
		Source:     "claude",
		StartedAt:  startedAt,
	}
}

// newPollTestMonitor creates a Monitor with real Store and Broadcaster,
// wired to the given test source and config overrides.
func newPollTestMonitor(src *testSource, cfg *config.Config) (*Monitor, *session.Store, *ws.Broadcaster) {
	return newPollTestMonitorWithSources([]awsource.Source{src}, cfg)
}

// newPollTestMonitorWithSources creates a Monitor wired to multiple sources.
// Use this when testing interactions between sources (e.g. panic isolation).
func newPollTestMonitorWithSources(sources []awsource.Source, cfg *config.Config) (*Monitor, *session.Store, *ws.Broadcaster) {
	store := session.NewStore()
	broadcaster := ws.NewBroadcaster(store, 50*time.Millisecond, 10*time.Second, 0)
	m := NewMonitor(cfg, store, broadcaster, sources)
	return m, store, broadcaster
}

func defaultTestConfig() *config.Config {
	return &config.Config{
		Monitor: config.MonitorConfig{
			PollInterval:          time.Second,
			SessionStaleAfter:     2 * time.Minute,
			CompletionRemoveAfter: -1, // disable auto-removal so we can inspect state
			BroadcastThrottle:     50 * time.Millisecond,
			SnapshotInterval:      10 * time.Second,
		},
		TokenNorm: config.TokenNormConfig{
			Strategies:       map[string]string{"claude": "usage", "default": "estimate"},
			TokensPerMessage: 2000,
		},
		Models: map[string]int{"default": 200000},
	}
}

func TestPollNormalSessionLifecycle(t *testing.T) {
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "session-abc.jsonl")

	now := time.Now().UTC()
	ts1 := now.Format(time.RFC3339Nano)
	ts2 := now.Add(time.Second).Format(time.RFC3339Nano)
	writeJSONL(t, jsonlPath,
		jsonlLine("user", "session-abc", ts1, "", "", "/home/user/project")+
			jsonlLine("assistant", "session-abc", ts2, "claude-opus-4-5-20251101", "", "/home/user/project"))

	src := &testSource{
		handles: []awsource.SessionHandle{newTestHandle("session-abc", jsonlPath, "/home/user/project", now)},
	}

	cfg := defaultTestConfig()
	m, store, _ := newPollTestMonitor(src, cfg)

	m.poll()

	state, ok := store.Get("claude:session-abc")
	if !ok {
		t.Fatal("session should exist in store after first poll")
	}
	if state.Source != "claude" {
		t.Errorf("Source = %q, want %q", state.Source, "claude")
	}
	if state.Model != "claude-opus-4-5-20251101" {
		t.Errorf("Model = %q, want %q", state.Model, "claude-opus-4-5-20251101")
	}
	if state.MessageCount != 2 {
		t.Errorf("MessageCount = %d, want 2", state.MessageCount)
	}
	if state.WorkingDir != "/home/user/project" {
		t.Errorf("WorkingDir = %q, want %q", state.WorkingDir, "/home/user/project")
	}
	if state.Name != "project" {
		t.Errorf("Name = %q, want %q", state.Name, "project")
	}
	if state.IsTerminal() {
		t.Error("session should not be terminal after first poll")
	}

	// Append more data and poll again: incremental parsing.
	ts3 := now.Add(2 * time.Second).Format(time.RFC3339Nano)
	appendJSONL(t, jsonlPath,
		jsonlLine("assistant", "session-abc", ts3, "claude-opus-4-5-20251101", "Read", "/home/user/project"))

	m.poll()

	state, _ = store.Get("claude:session-abc")
	if state.MessageCount != 3 {
		t.Errorf("MessageCount after second poll = %d, want 3", state.MessageCount)
	}
	if state.ToolCallCount != 1 {
		t.Errorf("ToolCallCount = %d, want 1", state.ToolCallCount)
	}
	if state.CurrentTool != "Read" {
		t.Errorf("CurrentTool = %q, want %q", state.CurrentTool, "Read")
	}

	// Poll with no new data: counts should not change.
	m.poll()

	state, _ = store.Get("claude:session-abc")
	if state.MessageCount != 3 {
		t.Errorf("MessageCount should stay at 3, got %d", state.MessageCount)
	}
}

func TestPollCachesProcessActivityBetweenRefreshes(t *testing.T) {
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "session-proc.jsonl")

	now := time.Now().UTC()
	ts1 := now.Format(time.RFC3339Nano)
	ts2 := now.Add(time.Second).Format(time.RFC3339Nano)
	writeJSONL(t, jsonlPath,
		jsonlLine("user", "session-proc", ts1, "", "", "/home/user/project")+
			jsonlLine("assistant", "session-proc", ts2, "claude-opus-4-5-20251101", "", "/home/user/project"))

	src := &testSource{
		handles: []awsource.SessionHandle{newTestHandle("session-proc", jsonlPath, "/home/user/project", now)},
	}

	cfg := defaultTestConfig()
	cfg.Monitor.ChurningCPUThreshold = 15.0
	m, store, _ := newPollTestMonitor(src, cfg)
	m.processPollInterval = 5 * time.Second

	callCount := 0
	m.discoverProcessActivity = func(prevCPU map[int]cpuSample, elapsed time.Duration) ([]ProcessActivity, map[int]cpuSample) {
		callCount++
		if callCount == 1 {
			return []ProcessActivity{{
				PID:        321,
				CPU:        30.0,
				TCPConns:   0,
				WorkingDir: "/home/user/project",
			}}, prevCPU
		}
		return []ProcessActivity{{
			PID:        321,
			CPU:        2.0,
			TCPConns:   0,
			WorkingDir: "/home/user/project",
		}}, prevCPU
	}

	m.poll()

	state, ok := store.Get("claude:session-proc")
	if !ok {
		t.Fatal("session should exist after first poll")
	}
	if callCount != 1 {
		t.Fatalf("discoverProcessActivity calls after first poll = %d, want 1", callCount)
	}
	if !state.IsChurning {
		t.Fatal("session should be churning after initial process discovery")
	}
	if state.PID != 321 {
		t.Fatalf("PID after first poll = %d, want 321", state.PID)
	}

	m.poll()

	state, ok = store.Get("claude:session-proc")
	if !ok {
		t.Fatal("session should exist after second poll")
	}
	if callCount != 1 {
		t.Fatalf("discoverProcessActivity calls after cached poll = %d, want 1", callCount)
	}
	if !state.IsChurning {
		t.Fatal("session should keep cached churning state before refresh interval elapses")
	}

	m.lastProcessPoll = time.Now().Add(-(m.processPollInterval + time.Second))
	m.poll()

	state, ok = store.Get("claude:session-proc")
	if !ok {
		t.Fatal("session should exist after third poll")
	}
	if callCount != 2 {
		t.Fatalf("discoverProcessActivity calls after refresh poll = %d, want 2", callCount)
	}
	if state.IsChurning {
		t.Fatal("session should clear churning after refreshed process activity drops below threshold")
	}
	if state.PID != 321 {
		t.Fatalf("PID after refresh = %d, want 321", state.PID)
	}
}

func TestPollTerminalSessionNotRecreatedAfterRemoval(t *testing.T) {
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "session-term.jsonl")

	now := time.Now().UTC()
	ts1 := now.Format(time.RFC3339Nano)
	writeJSONL(t, jsonlPath,
		jsonlLine("user", "session-term", ts1, "", "", "/tmp/proj")+
			jsonlLine("assistant", "session-term", ts1, "claude-opus-4-5-20251101", "", "/tmp/proj"))

	src := &testSource{
		handles: []awsource.SessionHandle{newTestHandle("session-term", jsonlPath, "/tmp/proj", now)},
	}

	cfg := defaultTestConfig()
	cfg.Monitor.CompletionRemoveAfter = 0 // immediate removal
	m, store, _ := newPollTestMonitor(src, cfg)

	m.poll()
	if _, ok := store.Get("claude:session-term"); !ok {
		t.Fatal("session should exist after first poll")
	}

	// Mark terminal and verify.
	state, _ := store.Get("claude:session-term")
	m.markTerminal(m.cfg, state, session.Complete, time.Now())

	state, _ = store.Get("claude:session-term")
	if !state.IsTerminal() {
		t.Fatal("session should be terminal after markTerminal")
	}

	// Flush removal (CompletionRemoveAfter=0 means immediate).
	m.poll()

	if _, ok := store.Get("claude:session-term"); ok {
		t.Error("terminal session should be removed from store after flush")
	}

	key := trackingKey("claude", "session-term")
	if !m.removedKeys[key] {
		t.Error("removedKeys should contain the terminal session key")
	}

	// File is still discovered but session must NOT be re-created.
	m.poll()
	if _, ok := store.Get("claude:session-term"); ok {
		t.Error("terminal session must not be re-created while in removedKeys")
	}
}

func TestPollDeadSessionSkippedOnStartup(t *testing.T) {
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "session-dead.jsonl")

	// Timestamps far in the past (beyond 2-minute stale threshold).
	oldTime := time.Now().UTC().Add(-10 * time.Minute)
	ts1 := oldTime.Format(time.RFC3339Nano)
	ts2 := oldTime.Add(time.Second).Format(time.RFC3339Nano)
	writeJSONL(t, jsonlPath,
		jsonlLine("user", "session-dead", ts1, "", "", "/tmp/dead")+
			jsonlLine("assistant", "session-dead", ts2, "claude-opus-4-5-20251101", "", "/tmp/dead"))

	src := &testSource{
		handles: []awsource.SessionHandle{newTestHandle("session-dead", jsonlPath, "/tmp/dead", oldTime)},
	}

	cfg := defaultTestConfig()
	cfg.Monitor.SessionStaleAfter = 2 * time.Minute
	m, store, _ := newPollTestMonitor(src, cfg)

	m.poll()

	if _, ok := store.Get("claude:session-dead"); ok {
		t.Error("dead session should not appear in store on startup")
	}

	key := trackingKey("claude", "session-dead")
	if !m.removedKeys[key] {
		t.Error("dead session should be added to removedKeys")
	}
	ts, ok := m.tracked[key]
	if !ok {
		t.Fatal("dead session should retain tracked offset for restart recovery")
	}
	if ts.cursor == "" {
		t.Error("dead session should retain a non-empty tracked cursor")
	}
}

func TestPollWorkingDirUpdatedMidSession(t *testing.T) {
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "session-cwd.jsonl")

	now := time.Now().UTC()
	ts1 := now.Format(time.RFC3339Nano)
	ts2 := now.Add(time.Second).Format(time.RFC3339Nano)

	writeJSONL(t, jsonlPath,
		jsonlLine("user", "session-cwd", ts1, "", "", "/home/user/project-a")+
			jsonlLine("assistant", "session-cwd", ts2, "claude-opus-4-5-20251101", "", "/home/user/project-a"))

	src := &testSource{
		handles: []awsource.SessionHandle{newTestHandle("session-cwd", jsonlPath, "/home/user/project-a", now)},
	}

	cfg := defaultTestConfig()
	m, store, _ := newPollTestMonitor(src, cfg)

	m.poll()

	state, ok := store.Get("claude:session-cwd")
	if !ok {
		t.Fatal("session should exist after first poll")
	}
	if state.WorkingDir != "/home/user/project-a" {
		t.Errorf("initial WorkingDir = %q, want %q", state.WorkingDir, "/home/user/project-a")
	}
	if state.Name != "project-a" {
		t.Errorf("initial Name = %q, want %q", state.Name, "project-a")
	}

	// Append entries with a different cwd.
	ts3 := now.Add(2 * time.Second).Format(time.RFC3339Nano)
	ts4 := now.Add(3 * time.Second).Format(time.RFC3339Nano)
	appendJSONL(t, jsonlPath,
		jsonlLine("user", "session-cwd", ts3, "", "", "/home/user/project-b")+
			jsonlLine("assistant", "session-cwd", ts4, "claude-opus-4-5-20251101", "", "/home/user/project-b"))

	m.poll()

	state, _ = store.Get("claude:session-cwd")
	if state.WorkingDir != "/home/user/project-b" {
		t.Errorf("updated WorkingDir = %q, want %q", state.WorkingDir, "/home/user/project-b")
	}
	if state.Name != "project-b" {
		t.Errorf("updated Name = %q, want %q", state.Name, "project-b")
	}
}

func TestPollStaleSessionMarkedLost(t *testing.T) {
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "session-stale.jsonl")

	now := time.Now().UTC()
	ts1 := now.Format(time.RFC3339Nano)
	writeJSONL(t, jsonlPath,
		jsonlLine("user", "session-stale", ts1, "", "", "/tmp/stale")+
			jsonlLine("assistant", "session-stale", ts1, "claude-opus-4-5-20251101", "", "/tmp/stale"))

	src := &testSource{
		handles: []awsource.SessionHandle{newTestHandle("session-stale", jsonlPath, "/tmp/stale", now)},
	}

	cfg := defaultTestConfig()
	cfg.Monitor.SessionStaleAfter = 2 * time.Minute
	m, store, _ := newPollTestMonitor(src, cfg)

	m.poll()
	if _, ok := store.Get("claude:session-stale"); !ok {
		t.Fatal("session should exist after first poll")
	}

	// Simulate file disappearing from discovery.
	src.handles = nil

	m.poll()

	state, ok := store.Get("claude:session-stale")
	if !ok {
		t.Fatal("session should still exist in store (marked lost, not removed)")
	}
	if state.Activity != session.Lost {
		t.Errorf("Activity = %s, want lost", state.Activity)
	}
	if state.CompletedAt == nil {
		t.Error("CompletedAt should be set for lost session")
	}
}

func TestPollSessionResumesAfterTerminal(t *testing.T) {
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "session-resume.jsonl")

	now := time.Now().UTC()
	ts1 := now.Format(time.RFC3339Nano)
	ts2 := now.Add(time.Second).Format(time.RFC3339Nano)
	writeJSONL(t, jsonlPath,
		jsonlLine("user", "session-resume", ts1, "", "", "/tmp/resume")+
			jsonlLine("assistant", "session-resume", ts2, "claude-opus-4-5-20251101", "", "/tmp/resume"))

	src := &testSource{
		handles: []awsource.SessionHandle{newTestHandle("session-resume", jsonlPath, "/tmp/resume", now)},
	}

	cfg := defaultTestConfig()
	m, store, _ := newPollTestMonitor(src, cfg)

	m.poll()

	state, _ := store.Get("claude:session-resume")
	m.markTerminal(m.cfg, state, session.Complete, time.Now())

	state, _ = store.Get("claude:session-resume")
	if !state.IsTerminal() {
		t.Fatal("session should be terminal")
	}

	// Append new data (simulates session resuming).
	ts3 := now.Add(5 * time.Second).Format(time.RFC3339Nano)
	appendJSONL(t, jsonlPath,
		jsonlLine("user", "session-resume", ts3, "", "", "/tmp/resume")+
			jsonlLine("assistant", "session-resume", ts3, "claude-opus-4-5-20251101", "", "/tmp/resume"))

	m.poll()

	state, _ = store.Get("claude:session-resume")
	if state.IsTerminal() {
		t.Error("session should no longer be terminal after resuming")
	}
	if state.CompletedAt != nil {
		t.Error("CompletedAt should be cleared on resume")
	}
	if state.MessageCount != 4 {
		t.Errorf("MessageCount = %d, want 4 (original 2 + resumed 2)", state.MessageCount)
	}
}

func TestPollMultipleSources(t *testing.T) {
	dir := t.TempDir()
	path1 := filepath.Join(dir, "session-1.jsonl")
	path2 := filepath.Join(dir, "session-2.jsonl")

	now := time.Now().UTC()
	ts := now.Format(time.RFC3339Nano)

	writeJSONL(t, path1,
		jsonlLine("user", "session-1", ts, "", "", "/tmp/proj1")+
			jsonlLine("assistant", "session-1", ts, "claude-opus-4-5-20251101", "", "/tmp/proj1"))

	writeJSONL(t, path2,
		jsonlLine("user", "session-2", ts, "", "", "/tmp/proj2")+
			jsonlLine("assistant", "session-2", ts, "claude-opus-4-5-20251101", "Bash", "/tmp/proj2"))

	src := &testSource{
		handles: []awsource.SessionHandle{
			newTestHandle("session-1", path1, "/tmp/proj1", now),
			newTestHandle("session-2", path2, "/tmp/proj2", now),
		},
	}

	cfg := defaultTestConfig()
	m, store, _ := newPollTestMonitor(src, cfg)

	m.poll()

	if _, ok := store.Get("claude:session-1"); !ok {
		t.Error("session-1 should exist")
	}
	state2, ok := store.Get("claude:session-2")
	if !ok {
		t.Error("session-2 should exist")
	}
	if state2.CurrentTool != "Bash" {
		t.Errorf("session-2 CurrentTool = %q, want %q", state2.CurrentTool, "Bash")
	}
	if store.ActiveCount() != 2 {
		t.Errorf("ActiveCount = %d, want 2", store.ActiveCount())
	}
}

func TestPollTokenResolutionEndToEnd(t *testing.T) {
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "session-tokens.jsonl")

	now := time.Now().UTC()
	ts := now.Format(time.RFC3339Nano)
	// Assistant message usage: input=100, cache_creation=500, cache_read=2000, output=50
	// TotalContext = 100 + 500 + 2000 = 2600
	writeJSONL(t, jsonlPath,
		jsonlLine("user", "session-tokens", ts, "", "", "/tmp/tokens")+
			jsonlLine("assistant", "session-tokens", ts, "claude-opus-4-5-20251101", "", "/tmp/tokens"))

	src := &testSource{
		handles: []awsource.SessionHandle{newTestHandle("session-tokens", jsonlPath, "/tmp/tokens", now)},
	}

	cfg := defaultTestConfig()
	m, store, _ := newPollTestMonitor(src, cfg)

	m.poll()

	state, _ := store.Get("claude:session-tokens")
	if state.TokensUsed != 2600 {
		t.Errorf("TokensUsed = %d, want 2600", state.TokensUsed)
	}
	if state.TokenEstimated {
		t.Error("TokenEstimated should be false (usage strategy with real data)")
	}
	if state.MaxContextTokens != 200000 {
		t.Errorf("MaxContextTokens = %d, want 200000", state.MaxContextTokens)
	}
	expectedUtil := 2600.0 / 200000.0
	if state.ContextUtilization != expectedUtil {
		t.Errorf("ContextUtilization = %f, want %f", state.ContextUtilization, expectedUtil)
	}
}

// terminalSource is a test source that returns Terminal=true on the second Parse call.
// This tests the monitor's handling of update.Terminal=true from a source.
type terminalSource struct {
	handle     awsource.SessionHandle
	callCount  int
	endReason  string
	endedAt    time.Time
}

func (s *terminalSource) Name() string { return "claude" }

func (s *terminalSource) Discover(_ context.Context) ([]awsource.SessionHandle, error) {
	return []awsource.SessionHandle{s.handle}, nil
}

func (s *terminalSource) Parse(_ context.Context, handle awsource.SessionHandle, cursor awsource.Cursor) (awsource.SourceUpdate, awsource.Cursor, error) {
	s.callCount++
	// First call: return normal data. Second call: signal terminal.
	if s.callCount == 1 {
		return parseJSONLHandle(handle, cursor)
	}
	newCursor := awsource.Cursor(fmt.Sprintf("%d", s.callCount))
	return awsource.SourceUpdate{
		SessionID: handle.ID,
		Terminal:  true,
		EndReason: s.endReason,
		EndedAt:   s.endedAt,
	}, newCursor, nil
}

// TestPollTerminalUpdate verifies that when a source returns update.Terminal=true,
// the monitor marks the session as Complete and sets CompletedAt.
// Previously session-end markers were consumed by the monitor directly; now
// the source (e.g. the claude source in agentwatch) handles marker files and
// signals Terminal=true in the SourceUpdate.
func TestPollTerminalUpdate(t *testing.T) {
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "session-end-test.jsonl")
	now := time.Now().UTC()
	ts := now.Format(time.RFC3339Nano)
	writeJSONL(t, jsonlPath,
		jsonlLine("user", "session-end-test", ts, "", "", "/tmp/endtest")+
			jsonlLine("assistant", "session-end-test", ts, "claude-opus-4-5-20251101", "", "/tmp/endtest"))

	endedAt := now.Add(2 * time.Second)
	src := &terminalSource{
		handle:    newTestHandle("session-end-test", jsonlPath, "/tmp/endtest", now),
		endReason: "success",
		endedAt:   endedAt,
	}

	cfg := defaultTestConfig()
	m, store, _ := newPollTestMonitorWithSources([]awsource.Source{src}, cfg)

	m.poll()
	if _, ok := store.Get("claude:session-end-test"); !ok {
		t.Fatal("session should exist after first poll")
	}

	// Second poll: source returns Terminal=true.
	m.poll()

	state, ok := store.Get("claude:session-end-test")
	if !ok {
		t.Fatal("session should still exist in store after terminal update")
	}
	if state.Activity != session.Complete {
		t.Errorf("Activity = %s, want complete", state.Activity)
	}
	if state.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
}

func TestPollRemovedKeysPurgedWhenFileDisappears(t *testing.T) {
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "session-purge.jsonl")

	now := time.Now().UTC()
	ts := now.Format(time.RFC3339Nano)
	writeJSONL(t, jsonlPath,
		jsonlLine("user", "session-purge", ts, "", "", "/tmp/purge")+
			jsonlLine("assistant", "session-purge", ts, "claude-opus-4-5-20251101", "", "/tmp/purge"))

	src := &testSource{
		handles: []awsource.SessionHandle{newTestHandle("session-purge", jsonlPath, "/tmp/purge", now)},
	}

	cfg := defaultTestConfig()
	cfg.Monitor.CompletionRemoveAfter = 0
	m, store, _ := newPollTestMonitor(src, cfg)

	m.poll()

	// Mark terminal and flush.
	state, _ := store.Get("claude:session-purge")
	m.markTerminal(m.cfg, state, session.Complete, time.Now())
	m.poll()

	key := trackingKey("claude", "session-purge")
	if !m.removedKeys[key] {
		t.Fatal("removedKeys should contain key after removal")
	}

	// File still discovered: removedKeys retained.
	m.poll()
	if !m.removedKeys[key] {
		t.Error("removedKeys should be retained while file is still discovered")
	}

	// File disappears: removedKeys should be purged.
	src.handles = nil
	m.poll()
	if m.removedKeys[key] {
		t.Error("removedKeys should be purged when file is no longer discovered")
	}
}

// TestPollRemovedSessionResumesAfterStaleCleanup verifies that a session
// removed by flushRemovals can still resume when new data arrives, even
// after stale detection would have cleaned up the tracked entry.
//
// This reproduces the bug where:
// 1. flushRemovals removes session from store + adds to removedKeys
// 2. Stale detection removes session from tracked (no store entry)
// 3. removedKeys blocks re-tracking → session permanently invisible
func TestPollRemovedSessionResumesAfterStaleCleanup(t *testing.T) {
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "session-revive.jsonl")

	now := time.Now().UTC()
	ts1 := now.Format(time.RFC3339Nano)
	writeJSONL(t, jsonlPath,
		jsonlLine("user", "session-revive", ts1, "", "", "/tmp/proj")+
			jsonlLine("assistant", "session-revive", ts1, "claude-opus-4-5-20251101", "", "/tmp/proj"))

	src := &testSource{
		handles: []awsource.SessionHandle{newTestHandle("session-revive", jsonlPath, "/tmp/proj", now)},
	}

	cfg := defaultTestConfig()
	cfg.Monitor.CompletionRemoveAfter = 0       // immediate removal
	cfg.Monitor.SessionStaleAfter = time.Second // short stale window for test
	m, store, _ := newPollTestMonitor(src, cfg)

	// Poll 1: session discovered and created in store.
	m.poll()
	if _, ok := store.Get("claude:session-revive"); !ok {
		t.Fatal("session should exist after first poll")
	}

	// Mark terminal → immediate removal by flushRemovals.
	state, _ := store.Get("claude:session-revive")
	m.markTerminal(m.cfg, state, session.Complete, time.Now())
	m.poll()

	if _, ok := store.Get("claude:session-revive"); ok {
		t.Fatal("session should be removed from store after flush")
	}

	key := trackingKey("claude", "session-revive")
	if !m.removedKeys[key] {
		t.Fatal("session should be in removedKeys after removal")
	}

	// Simulate stale threshold passing by backdating lastDataTime.
	// Before the fix, stale detection would delete the tracked entry
	// (session not in store + stale = cleanup). After the fix,
	// tracked is preserved because the key is in removedKeys and the
	// file is still discovered.
	ts := m.tracked[key]
	ts.lastDataTime = time.Now().Add(-2 * cfg.Monitor.SessionStaleAfter)
	m.tracked[key] = ts
	m.poll()

	// The tracked entry must survive for resume detection.
	if _, ok := m.tracked[key]; !ok {
		t.Fatal("tracked entry should be preserved for removedKeys sessions with active files")
	}

	// Now "revitalize" the session: append new data to the JSONL file.
	ts2 := time.Now().UTC().Format(time.RFC3339Nano)
	appendJSONL(t, jsonlPath,
		jsonlLine("user", "session-revive", ts2, "", "", "/tmp/proj")+
			jsonlLine("assistant", "session-revive", ts2, "claude-opus-4-5-20251101", "", "/tmp/proj"))

	m.poll()

	// Session should be back in the store.
	if _, ok := store.Get("claude:session-revive"); !ok {
		t.Error("session should resume after new data arrives — removedKeys must not block permanently")
	}

	// removedKeys should be cleared for the resumed session.
	if m.removedKeys[key] {
		t.Error("removedKeys should be cleared after session resumes")
	}
}

// TestPollStaleSessionDoesNotLoop verifies that an old session file (stale
// data timestamps) does not enter a track→lost→track loop.  Before the fix,
// stale detection deleted the tracked entry, causing the next poll to
// re-parse from offset 0, see "new data", resume the terminal session,
// and immediately mark it lost again — repeating every poll.
func TestPollStaleSessionDoesNotLoop(t *testing.T) {
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "session-loop.jsonl")

	// Write data with timestamps well in the past (>2 min ago).
	oldTime := time.Now().UTC().Add(-10 * time.Minute)
	ts1 := oldTime.Format(time.RFC3339Nano)
	ts2 := oldTime.Add(time.Second).Format(time.RFC3339Nano)
	writeJSONL(t, jsonlPath,
		jsonlLine("user", "session-loop", ts1, "", "", "/tmp/proj")+
			jsonlLine("assistant", "session-loop", ts2, "claude-opus-4-5-20251101", "", "/tmp/proj"))

	src := &testSource{
		handles: []awsource.SessionHandle{newTestHandle("session-loop", jsonlPath, "/tmp/proj", oldTime)},
	}

	cfg := defaultTestConfig()
	cfg.Monitor.CompletionRemoveAfter = 90 * time.Second // realistic setting
	cfg.Monitor.SessionStaleAfter = 2 * time.Minute
	m, store, broadcaster := newPollTestMonitor(src, cfg)

	key := trackingKey("claude", "session-loop")

	// Poll 1: session is discovered, data is old → should be skipped
	// (initial stale detection) OR created then immediately marked stale.
	m.poll()

	// The session should either not exist (skipped as stale on initial
	// discovery) or exist as terminal (marked Lost).
	state, exists := store.Get(key)
	if exists && !state.IsTerminal() {
		t.Fatalf("old session should be terminal or absent, got activity=%s", state.Activity)
	}

	// Record completion count from broadcaster to detect repeated events.
	initialClientCount := broadcaster.ClientCount()
	_ = initialClientCount // just ensure broadcaster is wired up

	// Poll 2-5: simulate multiple poll cycles. The bug would cause
	// "Tracking new session" + "Session resumed from lost" on every poll.
	for i := 0; i < 4; i++ {
		m.poll()
	}

	// After multiple polls, verify:
	// 1. The tracked entry should still exist (offset preserved)
	ts, tracked := m.tracked[key]
	if !tracked {
		// It's also acceptable for the session to not be tracked if it was
		// skipped by initial stale detection and added to removedKeys.
		if !m.removedKeys[key] {
			t.Fatal("session should either be tracked (with preserved offset) or in removedKeys")
		}
	} else {
		// If tracked, the offset should be the full file size, NOT 0.
		if ts.cursor == "" {
			t.Fatal("tracked entry has empty cursor — file would be re-parsed from scratch (loop bug)")
		}
	}

	// 2. The session should NOT have been re-created as non-terminal.
	state, exists = store.Get(key)
	if exists && !state.IsTerminal() {
		t.Fatalf("session should remain terminal after multiple polls, got activity=%s", state.Activity)
	}
}

func TestPollHealthDiscoverFailureTracking(t *testing.T) {
	src := &testSource{}

	cfg := defaultTestConfig()
	cfg.Monitor.HealthWarningThreshold = 3
	m, _, _ := newPollTestMonitor(src, cfg)

	// Verify health starts healthy.
	sh := m.health["claude"]
	if sh.status(3) != ws.StatusHealthy {
		t.Fatal("source should start healthy")
	}

	// Simulate discover failures.
	src.discoverErr = fmt.Errorf("connection refused")
	for i := 0; i < 3; i++ {
		m.poll()
	}

	if sh.discoverFailures != 3 {
		t.Errorf("discoverFailures = %d, want 3", sh.discoverFailures)
	}
	if sh.status(3) != ws.StatusFailed {
		t.Errorf("status = %s, want failed", sh.status(3))
	}
}

func TestPollHealthDiscoverRecovery(t *testing.T) {
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "session-health.jsonl")
	now := time.Now().UTC()
	ts := now.Format(time.RFC3339Nano)
	writeJSONL(t, jsonlPath,
		jsonlLine("user", "session-health", ts, "", "", "/tmp/h")+
			jsonlLine("assistant", "session-health", ts, "claude-opus-4-5-20251101", "", "/tmp/h"))

	src := &testSource{
		handles: []awsource.SessionHandle{newTestHandle("session-health", jsonlPath, "/tmp/h", now)},
	}

	cfg := defaultTestConfig()
	cfg.Monitor.HealthWarningThreshold = 2
	m, _, _ := newPollTestMonitor(src, cfg)

	// Fail discover
	src.discoverErr = fmt.Errorf("fail")
	m.poll()
	m.poll()

	sh := m.health["claude"]
	if sh.status(2) != ws.StatusFailed {
		t.Fatal("should be failed")
	}

	// Recover: hysteresis requires threshold (2) consecutive successes.
	src.discoverErr = nil
	m.poll()

	if sh.status(2) != ws.StatusFailed {
		t.Error("single success should not immediately recover from failed status")
	}

	m.poll()
	if sh.status(2) != ws.StatusHealthy {
		t.Errorf("status = %s, want healthy after threshold consecutive successes", sh.status(2))
	}
}

func TestPollHealthParseFailureTracking(t *testing.T) {
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "session-parse.jsonl")
	now := time.Now().UTC()
	ts := now.Format(time.RFC3339Nano)
	writeJSONL(t, jsonlPath,
		jsonlLine("user", "session-parse", ts, "", "", "/tmp/p")+
			jsonlLine("assistant", "session-parse", ts, "claude-opus-4-5-20251101", "", "/tmp/p"))

	src := &testSource{
		handles: []awsource.SessionHandle{newTestHandle("session-parse", jsonlPath, "/tmp/p", now)},
	}

	cfg := defaultTestConfig()
	cfg.Monitor.HealthWarningThreshold = 2
	m, _, _ := newPollTestMonitor(src, cfg)

	// First poll: normal (creates the session).
	m.poll()

	sh := m.health["claude"]
	if sh.status(2) != ws.StatusHealthy {
		t.Fatal("should start healthy")
	}

	// Simulate parse errors.
	src.parseErrs = map[string]error{
		"session-parse": fmt.Errorf("corrupt jsonl"),
	}
	m.poll()
	m.poll()

	if sh.status(2) != ws.StatusDegraded {
		t.Errorf("status = %s, want degraded", sh.status(2))
	}

	// Recover: hysteresis requires threshold (2) consecutive successes.
	src.parseErrs = nil
	m.poll()

	if sh.status(2) != ws.StatusDegraded {
		t.Error("single success should not immediately recover from degraded status")
	}

	m.poll()
	if sh.status(2) != ws.StatusHealthy {
		t.Errorf("status = %s, want healthy after threshold consecutive successes", sh.status(2))
	}
}

func TestPollHealthNotEmittedBelowThreshold(t *testing.T) {
	src := &testSource{
		discoverErr: fmt.Errorf("fail"),
	}

	cfg := defaultTestConfig()
	cfg.Monitor.HealthWarningThreshold = 5
	m, _, _ := newPollTestMonitor(src, cfg)

	sh := m.health["claude"]

	// Poll fewer times than threshold.
	m.poll()
	m.poll()

	if sh.status(5) != ws.StatusHealthy {
		t.Error("should still be healthy below threshold")
	}
	// lastEmittedStatus should still be the initial value (healthy).
	if sh.lastEmittedStatus != ws.StatusHealthy {
		t.Errorf("lastEmittedStatus = %s, want healthy (no transition)", sh.lastEmittedStatus)
	}
}

func TestPollHealthSnapshotIncludesFailingSources(t *testing.T) {
	src := &testSource{
		discoverErr: fmt.Errorf("fail"),
	}

	cfg := defaultTestConfig()
	cfg.Monitor.HealthWarningThreshold = 2
	m, _, _ := newPollTestMonitor(src, cfg)

	// Below threshold: snapshot should be empty.
	m.poll()
	snap := m.SourceHealthSnapshot()
	if len(snap) != 0 {
		t.Errorf("snapshot should be empty below threshold, got %d entries", len(snap))
	}

	// At threshold: snapshot should include the failing source.
	m.poll()
	snap = m.SourceHealthSnapshot()
	if len(snap) != 1 {
		t.Fatalf("snapshot should have 1 entry, got %d", len(snap))
	}
	if snap[0].Source != "claude" {
		t.Errorf("Source = %q, want %q", snap[0].Source, "claude")
	}
	if snap[0].Status != ws.StatusFailed {
		t.Errorf("Status = %s, want failed", snap[0].Status)
	}
	if snap[0].DiscoverFailures != 2 {
		t.Errorf("DiscoverFailures = %d, want 2", snap[0].DiscoverFailures)
	}
}

// TestPollSetConfigRace verifies that concurrent SetConfig calls do not
// race with poll(). This test is meaningful only under -race.
func TestPollSetConfigRace(t *testing.T) {
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "session-race.jsonl")

	now := time.Now().UTC()
	ts := now.Format(time.RFC3339Nano)
	writeJSONL(t, jsonlPath,
		jsonlLine("user", "session-race", ts, "", "", "/tmp/race")+
			jsonlLine("assistant", "session-race", ts, "claude-opus-4-5-20251101", "", "/tmp/race"))

	src := &testSource{
		handles: []awsource.SessionHandle{newTestHandle("session-race", jsonlPath, "/tmp/race", now)},
	}

	cfg := defaultTestConfig()
	m, _, _ := newPollTestMonitor(src, cfg)

	var wg sync.WaitGroup
	const iterations = 100

	// Concurrent SetConfig calls (simulating SIGHUP handler).
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			newCfg := defaultTestConfig()
			newCfg.Monitor.SessionStaleAfter = time.Duration(i+1) * time.Minute
			m.SetConfig(newCfg)
		}
	}()

	// Concurrent poll calls (simulating monitor goroutine).
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			m.poll()
		}
	}()

	wg.Wait()
}

// TestPollSetSourcesRace verifies that concurrent SetSources calls do not
// race with poll(). This test is meaningful only under -race.
func TestPollSetSourcesRace(t *testing.T) {
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "session-src-race.jsonl")

	now := time.Now().UTC()
	ts := now.Format(time.RFC3339Nano)
	writeJSONL(t, jsonlPath,
		jsonlLine("user", "session-src-race", ts, "", "", "/tmp/srace")+
			jsonlLine("assistant", "session-src-race", ts, "claude-opus-4-5-20251101", "", "/tmp/srace"))

	src1 := &testSource{
		handles: []awsource.SessionHandle{newTestHandle("session-src-race", jsonlPath, "/tmp/srace", now)},
	}
	src2 := &testSource{} // empty source

	cfg := defaultTestConfig()
	m, _, _ := newPollTestMonitor(src1, cfg)

	var wg sync.WaitGroup
	const iterations = 100

	// Concurrent SetSources calls (simulating SIGHUP handler).
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			if i%2 == 0 {
				m.SetSources([]awsource.Source{src1})
			} else {
				m.SetSources([]awsource.Source{src2})
			}
		}
	}()

	// Concurrent poll calls (simulating monitor goroutine).
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			m.poll()
		}
	}()

	wg.Wait()
}

// TestPollSourceHealthSnapshotRace verifies that concurrent
// sourceHealthSnapshot calls (from the broadcaster goroutine) do not
// race with SetSources or poll.
func TestPollSourceHealthSnapshotRace(t *testing.T) {
	src := &testSource{
		discoverErr: fmt.Errorf("fail"),
	}

	cfg := defaultTestConfig()
	cfg.Monitor.HealthWarningThreshold = 2
	m, _, _ := newPollTestMonitor(src, cfg)

	var wg sync.WaitGroup
	const iterations = 100

	// Concurrent sourceHealthSnapshot calls (simulating broadcaster hook).
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = m.SourceHealthSnapshot()
		}
	}()

	// Concurrent SetSources + poll (simulating SIGHUP + monitor).
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			if i%10 == 0 {
				m.SetSources([]awsource.Source{src})
			}
			m.poll()
		}
	}()

	wg.Wait()
}

// panicSource is a source that panics in Discover or Parse, for testing
// panic recovery in the poll loop.
type panicSource struct {
	name            string
	panicOnDiscover bool
	panicOnParse    bool
	handles         []awsource.SessionHandle
}

func (s *panicSource) Name() string { return s.name }

func (s *panicSource) Discover(_ context.Context) ([]awsource.SessionHandle, error) {
	if s.panicOnDiscover {
		panic("discover: nil pointer dereference")
	}
	return s.handles, nil
}

func (s *panicSource) Parse(_ context.Context, _ awsource.SessionHandle, cursor awsource.Cursor) (awsource.SourceUpdate, awsource.Cursor, error) {
	if s.panicOnParse {
		panic("parse: index out of range")
	}
	return awsource.SourceUpdate{}, cursor, nil
}

func TestPollRecoversPanicInDiscover(t *testing.T) {
	panicker := &panicSource{
		name:            "panicky",
		panicOnDiscover: true,
	}

	cfg := defaultTestConfig()
	m, _, _ := newPollTestMonitorWithSources([]awsource.Source{panicker}, cfg)

	// poll() must not panic -- the recover should catch it.
	m.poll()

	// The panicking source should be recorded as failed in health.
	sh := m.health["panicky"]
	if sh.discoverFailures != 1 {
		t.Errorf("discoverFailures = %d, want 1 (panic should be recorded)", sh.discoverFailures)
	}
	if sh.lastDiscoverErr == "" {
		t.Error("lastDiscoverErr should be set after panic")
	}
}

func TestPollRecoversPanicInParse(t *testing.T) {
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "session-panic.jsonl")
	now := time.Now().UTC()
	ts := now.Format(time.RFC3339Nano)
	writeJSONL(t, jsonlPath,
		jsonlLine("user", "session-panic", ts, "", "", "/tmp/panic"))

	panicker := &panicSource{
		name:         "panicky",
		panicOnParse: true,
		handles: []awsource.SessionHandle{{
			ID:         "session-panic",
			Path:       jsonlPath,
			WorkingDir: "/tmp/panic",
			Source:     "panicky",
			StartedAt:  now,
		}},
	}

	cfg := defaultTestConfig()
	m, _, _ := newPollTestMonitorWithSources([]awsource.Source{panicker}, cfg)

	// poll() must not panic.
	m.poll()

	// The source should be recorded as failed.
	sh := m.health["panicky"]
	if sh.discoverFailures != 1 {
		t.Errorf("discoverFailures = %d, want 1 (panic should be recorded)", sh.discoverFailures)
	}
}

func TestPollPanicInOneSourceDoesNotAffectOthers(t *testing.T) {
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "session-ok.jsonl")
	now := time.Now().UTC()
	ts := now.Format(time.RFC3339Nano)
	writeJSONL(t, jsonlPath,
		jsonlLine("user", "session-ok", ts, "", "", "/tmp/ok")+
			jsonlLine("assistant", "session-ok", ts, "claude-opus-4-5-20251101", "", "/tmp/ok"))

	panicker := &panicSource{
		name:            "panicky",
		panicOnDiscover: true,
	}
	healthy := &testSource{
		handles: []awsource.SessionHandle{newTestHandle("session-ok", jsonlPath, "/tmp/ok", now)},
	}

	cfg := defaultTestConfig()
	// panicky source is first -- it must not prevent the healthy source from running.
	m, store, _ := newPollTestMonitorWithSources([]awsource.Source{panicker, healthy}, cfg)

	m.poll()

	// The healthy source should have discovered and stored its session.
	state, ok := store.Get("claude:session-ok")
	if !ok {
		t.Fatal("healthy source's session should exist despite panicky source")
	}
	if state.Model != "claude-opus-4-5-20251101" {
		t.Errorf("Model = %q, want %q", state.Model, "claude-opus-4-5-20251101")
	}

	// panicky source should be tracked as failed.
	sh := m.health["panicky"]
	if sh.discoverFailures != 1 {
		t.Errorf("panicky discoverFailures = %d, want 1", sh.discoverFailures)
	}

	// healthy source should be healthy.
	sh2 := m.health["claude"]
	if sh2.discoverFailures != 0 {
		t.Errorf("healthy discoverFailures = %d, want 0", sh2.discoverFailures)
	}
}

func TestPollRepeatedPanicsAccumulateFailures(t *testing.T) {
	panicker := &panicSource{
		name:            "panicky",
		panicOnDiscover: true,
	}

	cfg := defaultTestConfig()
	cfg.Monitor.HealthWarningThreshold = 3
	m, _, _ := newPollTestMonitorWithSources([]awsource.Source{panicker}, cfg)

	// Poll multiple times -- each panic should increment the failure counter.
	for i := 0; i < 5; i++ {
		m.poll()
	}

	sh := m.health["panicky"]
	if sh.discoverFailures != 5 {
		t.Errorf("discoverFailures = %d, want 5", sh.discoverFailures)
	}
	if sh.status(3) != ws.StatusFailed {
		t.Errorf("status = %s, want failed", sh.status(3))
	}
}

// ---------------------------------------------------------------------------
// Poll-level deadlock regression tests
//
// These test the full poll() path that calls markTerminal(), verifying that
// the store remains accessible after a terminal transition during polling.
// ---------------------------------------------------------------------------

// pollDeadlockTimeout is the deadline for poll-level deadlock tests.
const pollDeadlockTimeout = 3 * time.Second



// TestPollMarkTerminalViaStaleDetectionDoesNotDeadlock verifies that the
// stale detection path (which calls markTerminal() for disappeared sessions)
// also does not deadlock. Stale detection is a separate code path from
// session end markers — it calls markTerminal() directly in the poll loop.
func TestPollMarkTerminalViaStaleDetectionDoesNotDeadlock(t *testing.T) {
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "session-stale-dl.jsonl")

	now := time.Now().UTC()
	ts := now.Format(time.RFC3339Nano)
	writeJSONL(t, jsonlPath,
		jsonlLine("user", "session-stale-dl", ts, "", "", "/tmp/stale-dl")+
			jsonlLine("assistant", "session-stale-dl", ts, "claude-opus-4-5-20251101", "", "/tmp/stale-dl"))

	src := &testSource{
		handles: []awsource.SessionHandle{newTestHandle("session-stale-dl", jsonlPath, "/tmp/stale-dl", now)},
	}

	cfg := defaultTestConfig()
	cfg.Monitor.CompletionRemoveAfter = -1
	m, store, _ := newPollTestMonitor(src, cfg)

	statsEvents := make(chan session.Event, 50)
	m.statsEvents = statsEvents

	// Poll 1: discover the session.
	m.poll()
	if _, ok := store.Get("claude:session-stale-dl"); !ok {
		t.Fatal("session should exist after first poll")
	}

	// Remove the file from discovery (simulates the process exiting and
	// the JSONL file disappearing from the discover window).
	src.handles = nil

	// Poll 2: stale detection runs → calls markTerminal(state, Lost, ...).
	// Before the fix: deadlock. After the fix: completes.
	pollDone := make(chan struct{})
	go func() {
		m.poll()
		close(pollDone)
	}()

	select {
	case <-pollDone:
		// poll() completed without deadlock.
	case <-time.After(pollDeadlockTimeout):
		t.Fatal("DEADLOCK: poll() blocked during stale detection markTerminal()")
	}

	// Store must be accessible after the poll.
	getAllDone := make(chan struct{})
	go func() {
		_ = store.GetAll()
		_ = store.ActiveCount()
		close(getAllDone)
	}()

	select {
	case <-getAllDone:
		// All store operations completed.
	case <-time.After(pollDeadlockTimeout):
		t.Fatal("DEADLOCK: store.GetAll()/ActiveCount() blocked after stale detection poll")
	}

	// The session must be terminal (Lost).
	state, ok := store.Get("claude:session-stale-dl")
	if !ok {
		t.Fatal("session should still be in store (CompletionRemoveAfter=-1)")
	}
	if state.Activity != session.Lost {
		t.Errorf("activity = %s, want lost (stale detection)", state.Activity)
	}
}

// TestPollConcurrentGetAllDuringMarkTerminal verifies that a goroutine calling
// store.GetAll() concurrently with poll() processing a session end marker does
// not deadlock. This is the exact production scenario: the HTTP handler for
// /api/sessions calls GetAll() while the monitor goroutine calls poll().
func TestPollConcurrentGetAllDuringMarkTerminal(t *testing.T) {
	dir := t.TempDir()
	sessionEndDir := filepath.Join(dir, "end")
	if err := os.MkdirAll(sessionEndDir, 0755); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	ts := now.Format(time.RFC3339Nano)

	// Create multiple sessions to make the race window larger.
	const numSessions = 3
	handles := make([]awsource.SessionHandle, numSessions)
	for i := 0; i < numSessions; i++ {
		id := fmt.Sprintf("session-concurrent-%d", i)
		path := filepath.Join(dir, id+".jsonl")
		writeJSONL(t, path,
			jsonlLine("user", id, ts, "", "", "/tmp/concurrent")+
				jsonlLine("assistant", id, ts, "claude-opus-4-5-20251101", "", "/tmp/concurrent"))
		handles[i] = newTestHandle(id, path, "/tmp/concurrent", now)
	}

	src := &testSource{handles: handles}

	cfg := defaultTestConfig()
	cfg.Monitor.SessionEndDir = sessionEndDir
	cfg.Monitor.CompletionRemoveAfter = -1
	m, store, _ := newPollTestMonitor(src, cfg)

	statsEvents := make(chan session.Event, 100)
	m.statsEvents = statsEvents

	// Poll 1: discover all sessions.
	m.poll()

	// Drop session end markers for all sessions simultaneously.
	for i := 0; i < numSessions; i++ {
		id := fmt.Sprintf("session-concurrent-%d", i)
		logPath := handles[i].Path
		markerContent := fmt.Sprintf(
			`{"session_id":"%s","transcript_path":"%s","cwd":"/tmp/concurrent","reason":"success","timestamp":"%s"}`,
			id, logPath, now.Add(2*time.Second).Format(time.RFC3339Nano),
		)
		markerPath := filepath.Join(sessionEndDir, id+".json")
		if err := os.WriteFile(markerPath, []byte(markerContent), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Launch concurrent readers (simulating HTTP handlers).
	var readersWg sync.WaitGroup
	const numReaders = 5
	for i := 0; i < numReaders; i++ {
		readersWg.Add(1)
		go func() {
			defer readersWg.Done()
			for j := 0; j < 10; j++ {
				_ = store.GetAll()
				_ = store.ActiveCount()
			}
		}()
	}

	// Poll 2: process all end markers (multiple markTerminal calls).
	pollDone := make(chan struct{})
	go func() {
		m.poll()
		close(pollDone)
	}()

	// Both the poll and all readers must complete within the deadline.
	allDone := make(chan struct{})
	go func() {
		<-pollDone
		readersWg.Wait()
		close(allDone)
	}()

	select {
	case <-allDone:
		// No deadlock.
	case <-time.After(pollDeadlockTimeout):
		t.Fatal("DEADLOCK: concurrent poll()+GetAll()/ActiveCount() did not complete — store.mu permanently locked")
	}
}
