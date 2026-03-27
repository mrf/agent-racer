package monitor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agent-racer/backend/internal/session"
)

// writeEndMarker writes a JSON session-end marker file into dir and returns
// the full path.
func writeEndMarker(t *testing.T, dir, filename string, marker sessionEndMarker) string {
	t.Helper()
	path := filepath.Join(dir, filename)
	data, err := json.Marshal(marker)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// setupActiveSession creates a monitor with one active session in the store.
// Returns the monitor, the store, the session end dir, and the store key for
// the tracked session.
func setupActiveSession(t *testing.T) (*Monitor, *session.Store, string, string) {
	t.Helper()

	jsonlDir := t.TempDir()
	endDir := t.TempDir()
	const sid = "session-end-sess"

	jsonlPath := filepath.Join(jsonlDir, sid+".jsonl")
	now := time.Now().UTC()
	ts := now.Format(time.RFC3339Nano)
	writeJSONL(t, jsonlPath,
		jsonlLine("user", sid, ts, "", "", "/tmp/end-proj")+
			jsonlLine("assistant", sid, ts, "claude-opus-4-5-20251101", "", "/tmp/end-proj"))

	src := &testSource{
		handles: []SessionHandle{newTestHandle(sid, jsonlPath, "/tmp/end-proj", now)},
	}

	cfg := defaultTestConfig()
	cfg.Monitor.CompletionRemoveAfter = -1 // keep sessions in store after terminal
	cfg.Monitor.SessionEndDir = endDir

	m, store, _ := newPollTestMonitor(src, cfg)
	m.poll() // discover and track the session

	storeKey := trackingKey("claude", sid)
	if _, ok := store.Get(storeKey); !ok {
		t.Fatalf("session %q not in store after first poll", storeKey)
	}
	return m, store, endDir, storeKey
}

// TestSessionEndMarkerTerminatesActiveSession verifies that a well-formed
// marker for an active session marks it Complete and deletes the marker file.
func TestSessionEndMarkerTerminatesActiveSession(t *testing.T) {
	m, store, endDir, storeKey := setupActiveSession(t)

	markerPath := writeEndMarker(t, endDir, "end-1.json", sessionEndMarker{
		SessionID: "session-end-sess",
		Reason:    "",
	})

	m.poll()

	state, ok := store.Get(storeKey)
	if !ok {
		t.Fatal("session should still be in store (CompletionRemoveAfter = -1)")
	}
	if !state.IsTerminal() {
		t.Errorf("session activity = %q, want terminal", state.Activity)
	}
	if state.Activity != session.Complete {
		t.Errorf("session activity = %q, want Complete", state.Activity)
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Error("marker file should be deleted after processing")
	}
}

// TestSessionEndMarkerErrorReasonSetsErrored verifies that a reason containing
// an error indicator maps to the Errored terminal activity.
func TestSessionEndMarkerErrorReasonSetsErrored(t *testing.T) {
	m, store, endDir, storeKey := setupActiveSession(t)

	writeEndMarker(t, endDir, "end-err.json", sessionEndMarker{
		SessionID: "session-end-sess",
		Reason:    "process terminated with error",
	})

	m.poll()

	state, ok := store.Get(storeKey)
	if !ok {
		t.Fatal("session should still be in store")
	}
	if state.Activity != session.Errored {
		t.Errorf("session activity = %q, want Errored", state.Activity)
	}
}

// TestSessionEndMarkerNonErrorReasonSetsComplete verifies that a non-empty
// reason without error indicators maps to Complete.
func TestSessionEndMarkerNonErrorReasonSetsComplete(t *testing.T) {
	m, store, endDir, storeKey := setupActiveSession(t)

	writeEndMarker(t, endDir, "end-ok.json", sessionEndMarker{
		SessionID: "session-end-sess",
		Reason:    "user exited",
	})

	m.poll()

	state, ok := store.Get(storeKey)
	if !ok {
		t.Fatal("session should still be in store")
	}
	if state.Activity != session.Complete {
		t.Errorf("session activity = %q, want Complete", state.Activity)
	}
}

// TestSessionEndMarkerUnknownSessionIgnored verifies that a marker referencing
// a session not in the store is silently ignored and the file is still deleted.
func TestSessionEndMarkerUnknownSessionIgnored(t *testing.T) {
	m, store, endDir, _ := setupActiveSession(t)

	markerPath := writeEndMarker(t, endDir, "end-unknown.json", sessionEndMarker{
		SessionID:      "nonexistent-session-id",
		TranscriptPath: "/nowhere/also-not-found.jsonl",
	})

	m.poll()

	// The existing active session must be unaffected.
	storeKey := trackingKey("claude", "session-end-sess")
	if state, ok := store.Get(storeKey); ok && state.IsTerminal() {
		t.Error("active session should not be terminated by an unrelated marker")
	}

	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Error("marker file for unknown session should be deleted")
	}
}

// TestSessionEndMarkerMalformedJSONHandledGracefully verifies that a file
// containing invalid JSON is deleted and does not crash the monitor.
func TestSessionEndMarkerMalformedJSONHandledGracefully(t *testing.T) {
	m, store, endDir, storeKey := setupActiveSession(t)

	badPath := filepath.Join(endDir, "bad.json")
	if err := os.WriteFile(badPath, []byte(`{not valid json`), 0644); err != nil {
		t.Fatal(err)
	}

	m.poll() // must not panic

	// Active session must be unaffected.
	if state, ok := store.Get(storeKey); ok && state.IsTerminal() {
		t.Error("active session should not be terminated by a malformed marker")
	}

	if _, err := os.Stat(badPath); !os.IsNotExist(err) {
		t.Error("malformed marker file should be deleted")
	}
}

// TestSessionEndMarkerEmptySessionIDDeletesFile verifies that a well-formed
// JSON file with an empty session_id field is deleted without processing.
func TestSessionEndMarkerEmptySessionIDDeletesFile(t *testing.T) {
	m, store, endDir, storeKey := setupActiveSession(t)

	markerPath := writeEndMarker(t, endDir, "end-empty-id.json", sessionEndMarker{
		SessionID: "",
		Reason:    "completed",
	})

	m.poll()

	if state, ok := store.Get(storeKey); ok && state.IsTerminal() {
		t.Error("active session should not be terminated by an empty-id marker")
	}

	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Error("empty-session-id marker file should be deleted")
	}
}

// TestSessionEndMarkerTranscriptPathFallback verifies that when the marker's
// session_id does not match a store entry but the transcript_path filename
// (without extension) does, the session is still terminated.
func TestSessionEndMarkerTranscriptPathFallback(t *testing.T) {
	m, store, endDir, storeKey := setupActiveSession(t)

	// Use a different session_id in the marker, but a transcript_path whose
	// base filename matches the tracked session's ID.
	writeEndMarker(t, endDir, "end-fallback.json", sessionEndMarker{
		SessionID:      "some-uuid-not-in-store",
		TranscriptPath: "/home/user/.claude/projects/proj/session-end-sess.jsonl",
	})

	m.poll()

	state, ok := store.Get(storeKey)
	if !ok {
		t.Fatal("session should still be in store (CompletionRemoveAfter = -1)")
	}
	if !state.IsTerminal() {
		t.Errorf("session should be terminal via transcript_path fallback, got activity %q", state.Activity)
	}
}

// TestSessionEndMarkerTimestampUsedAsCompletedAt verifies that when the marker
// contains a valid RFC3339Nano timestamp it is used as CompletedAt rather than
// the current time.
func TestSessionEndMarkerTimestampUsedAsCompletedAt(t *testing.T) {
	m, store, endDir, storeKey := setupActiveSession(t)

	completedAt := time.Now().UTC().Add(-5 * time.Minute).Truncate(time.Second)

	writeEndMarker(t, endDir, "end-ts.json", sessionEndMarker{
		SessionID: "session-end-sess",
		Timestamp: completedAt.Format(time.RFC3339Nano),
	})

	m.poll()

	state, ok := store.Get(storeKey)
	if !ok {
		t.Fatal("session should still be in store")
	}
	if state.CompletedAt == nil {
		t.Fatal("CompletedAt should be set")
	}
	// Allow a 1-second tolerance for RFC3339 sub-second rounding.
	diff := state.CompletedAt.Sub(completedAt)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("CompletedAt = %v, want ~%v (diff=%v)", state.CompletedAt, completedAt, diff)
	}
}

// TestSessionEndMarkerDirectoryEntriesSkipped verifies that directories inside
// the SessionEndDir are not read as marker files.
func TestSessionEndMarkerDirectoryEntriesSkipped(t *testing.T) {
	m, store, endDir, storeKey := setupActiveSession(t)

	// Create a subdirectory with a JSON filename — it must not be processed.
	subdir := filepath.Join(endDir, "subdir.json")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatal(err)
	}

	m.poll() // must not panic or misinterpret the directory

	if state, ok := store.Get(storeKey); ok && state.IsTerminal() {
		t.Error("active session should not be terminated by a directory entry")
	}
}

// TestSessionEndMarkerNonexistentDirIsNoOp verifies that a nonexistent
// SessionEndDir does not crash the monitor.
func TestSessionEndMarkerNonexistentDirIsNoOp(t *testing.T) {
	jsonlDir := t.TempDir()
	const sid = "session-nodir"
	jsonlPath := filepath.Join(jsonlDir, sid+".jsonl")
	now := time.Now().UTC()
	ts := now.Format(time.RFC3339Nano)
	writeJSONL(t, jsonlPath,
		jsonlLine("user", sid, ts, "", "", "/tmp/nd"))

	src := &testSource{
		handles: []SessionHandle{newTestHandle(sid, jsonlPath, "/tmp/nd", now)},
	}

	cfg := defaultTestConfig()
	cfg.Monitor.SessionEndDir = "/nonexistent/path/that/does/not/exist"

	m, _, _ := newPollTestMonitor(src, cfg)
	m.poll() // must not panic
}

// TestSessionEndMarkerEmptyDirIsNoOp verifies that an empty SessionEndDir
// causes no session state changes.
func TestSessionEndMarkerEmptyDirIsNoOp(t *testing.T) {
	m, store, _, storeKey := setupActiveSession(t)

	// endDir is already empty — no marker files written.
	m.poll()

	state, ok := store.Get(storeKey)
	if !ok {
		t.Fatal("session should still be in store")
	}
	if state.IsTerminal() {
		t.Error("session should not be terminal when end dir is empty")
	}
}

// TestSessionEndMarkerLimitPerPoll verifies that at most maxEndMarkersPerPoll
// files are processed in a single poll cycle, leaving the rest for later.
func TestSessionEndMarkerLimitPerPoll(t *testing.T) {
	endDir := t.TempDir()
	total := maxEndMarkersPerPoll + 50

	for i := 0; i < total; i++ {
		name := fmt.Sprintf("end-%04d.json", i)
		marker := sessionEndMarker{
			SessionID: fmt.Sprintf("sess-%04d", i),
		}
		writeEndMarker(t, endDir, name, marker)
	}

	src := &testSource{handles: []SessionHandle{}}
	cfg := defaultTestConfig()
	cfg.Monitor.CompletionRemoveAfter = -1
	cfg.Monitor.SessionEndDir = endDir

	m, _, _ := newPollTestMonitor(src, cfg)
	m.poll()

	// Markers reference unknown sessions so they are ignored but still
	// read and removed. At most maxEndMarkersPerPoll should be gone.
	after, err := os.ReadDir(endDir)
	if err != nil {
		t.Fatal(err)
	}
	removed := total - len(after)
	if removed > maxEndMarkersPerPoll {
		t.Errorf("removed %d files in one poll, want at most %d", removed, maxEndMarkersPerPoll)
	}
	if len(after) == 0 {
		t.Error("all files were consumed in one poll — limit is not enforced")
	}
}

