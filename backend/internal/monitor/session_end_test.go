package monitor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestValidateEndMarker_ValidMarker(t *testing.T) {
	now := time.Now().UTC()
	m := &sessionEndMarker{
		SessionID:      "01234567-abcd-ef01-2345-67890abcdef0",
		TranscriptPath: "/home/user/.claude/projects/proj/session.jsonl",
		Reason:         "user exited",
		Timestamp:      now.Add(-30 * time.Second).Format(time.RFC3339Nano),
	}
	if err := validateEndMarker(m, now); err != nil {
		t.Errorf("expected valid marker, got error: %v", err)
	}
}

func TestValidateEndMarker_Rejected(t *testing.T) {
	cases := []struct {
		name   string
		marker sessionEndMarker
	}{
		{"empty session_id", sessionEndMarker{SessionID: ""}},
		{"session_id too long", sessionEndMarker{SessionID: strings.Repeat("a", maxSessionIDLen+1)}},
		{"session_id with spaces", sessionEndMarker{SessionID: "session id"}},
		{"session_id with newline", sessionEndMarker{SessionID: "session\nid"}},
		{"session_id path traversal", sessionEndMarker{SessionID: "../../../etc/passwd"}},
		{"session_id shell metachar", sessionEndMarker{SessionID: "session;rm -rf /"}},
		{"session_id null byte", sessionEndMarker{SessionID: "session\x00id"}},
		{"transcript_path not .jsonl", sessionEndMarker{SessionID: "valid-session", TranscriptPath: "/home/user/transcript.txt"}},
		{"transcript_path traversal", sessionEndMarker{SessionID: "valid-session", TranscriptPath: "/home/user/../../../etc/shadow.jsonl"}},
		{"transcript_path too long", sessionEndMarker{SessionID: "valid-session", TranscriptPath: "/" + strings.Repeat("a", maxTranscriptPathLen) + ".jsonl"}},
	}
	now := time.Now().UTC()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateEndMarker(&tc.marker, now); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestValidateEndMarker_ReasonTruncated(t *testing.T) {
	now := time.Now().UTC()
	m := &sessionEndMarker{
		SessionID: "valid-session",
		Reason:    strings.Repeat("x", maxReasonLen+100),
	}
	if err := validateEndMarker(m, now); err != nil {
		t.Errorf("long reason should not cause error, got: %v", err)
	}
	if len(m.Reason) != maxReasonLen {
		t.Errorf("reason should be truncated to %d, got %d", maxReasonLen, len(m.Reason))
	}
}

func TestValidateEndMarker_TimestampSanitized(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		name      string
		timestamp string
	}{
		{"invalid format", "not-a-timestamp"},
		{"far future", now.Add(2 * time.Hour).Format(time.RFC3339Nano)},
		{"far past", now.Add(-2 * time.Hour).Format(time.RFC3339Nano)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &sessionEndMarker{
				SessionID: "valid-session",
				Timestamp: tc.timestamp,
			}
			if err := validateEndMarker(m, now); err != nil {
				t.Errorf("should not return error, got: %v", err)
			}
			if m.Timestamp != "" {
				t.Errorf("timestamp should be cleared, got %q", m.Timestamp)
			}
		})
	}
}

func TestValidateEndMarker_TimestampWithinSkewPreserved(t *testing.T) {
	now := time.Now().UTC()
	ts := now.Add(-30 * time.Minute).Format(time.RFC3339Nano)
	m := &sessionEndMarker{
		SessionID: "valid-session",
		Timestamp: ts,
	}
	if err := validateEndMarker(m, now); err != nil {
		t.Errorf("recent timestamp should not cause error, got: %v", err)
	}
	if m.Timestamp != ts {
		t.Error("timestamp within skew window should be preserved")
	}
}

func TestSessionEndMarkerOversizedFileRejected(t *testing.T) {
	m, store, endDir, storeKey := setupActiveSession(t)

	// Write a file larger than maxEndMarkerFileSize.
	bigPath := filepath.Join(endDir, "big.json")
	bigData := []byte(`{"session_id":"session-end-sess","reason":"` + strings.Repeat("x", maxEndMarkerFileSize) + `"}`)
	if err := os.WriteFile(bigPath, bigData, 0644); err != nil {
		t.Fatal(err)
	}

	m.poll()

	// Active session must be unaffected.
	if state, ok := store.Get(storeKey); ok && state.IsTerminal() {
		t.Error("active session should not be terminated by an oversized marker")
	}
	// File should be removed.
	if _, err := os.Stat(bigPath); !os.IsNotExist(err) {
		t.Error("oversized marker file should be deleted")
	}
}

func TestSessionEndMarkerInvalidSessionIDRejected(t *testing.T) {
	m, store, endDir, storeKey := setupActiveSession(t)

	// Write a marker with shell metacharacters in session_id.
	markerPath := writeEndMarker(t, endDir, "end-inject.json", sessionEndMarker{
		SessionID: "session; rm -rf /",
		Reason:    "",
	})

	m.poll()

	if state, ok := store.Get(storeKey); ok && state.IsTerminal() {
		t.Error("active session should not be terminated by a marker with invalid session_id")
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Error("marker with invalid session_id should be deleted")
	}
}

func TestSessionEndMarkerBadTranscriptPathRejected(t *testing.T) {
	m, store, endDir, storeKey := setupActiveSession(t)

	markerPath := writeEndMarker(t, endDir, "end-badpath.json", sessionEndMarker{
		SessionID:      "session-end-sess",
		TranscriptPath: "/etc/passwd",
	})

	m.poll()

	if state, ok := store.Get(storeKey); ok && state.IsTerminal() {
		t.Error("active session should not be terminated by a marker with bad transcript_path")
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Error("marker with bad transcript_path should be deleted")
	}
}

