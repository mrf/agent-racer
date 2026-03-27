package replay

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/agent-racer/backend/internal/session"
)

type stubSnapshotFile struct {
	buffer   bytes.Buffer
	calls    []string
	syncErr  error
	closeErr error
}

func (f *stubSnapshotFile) Write(p []byte) (int, error) {
	f.calls = append(f.calls, "write")
	return f.buffer.Write(p)
}

func (f *stubSnapshotFile) Sync() error {
	f.calls = append(f.calls, "sync")
	return f.syncErr
}

func (f *stubSnapshotFile) Close() error {
	f.calls = append(f.calls, "close")
	return f.closeErr
}

func TestRecorder_WriteSnapshotSyncsAfterEncode(t *testing.T) {
	file := &stubSnapshotFile{}
	rec := &Recorder{
		file:    file,
		encoder: json.NewEncoder(file),
	}

	rec.WriteSnapshot(nil)

	want := []string{"write", "sync"}
	if !reflect.DeepEqual(file.calls, want) {
		t.Fatalf("call order = %v, want %v", file.calls, want)
	}
	if file.buffer.Len() == 0 {
		t.Fatal("expected encoded snapshot to be written")
	}
}

func TestRecorder_WriteSnapshotKeepsDataOnSyncError(t *testing.T) {
	file := &stubSnapshotFile{syncErr: errors.New("sync failed")}
	rec := &Recorder{
		file:    file,
		encoder: json.NewEncoder(file),
	}

	rec.WriteSnapshot(nil)

	want := []string{"write", "sync"}
	if !reflect.DeepEqual(file.calls, want) {
		t.Fatalf("call order = %v, want %v", file.calls, want)
	}
	if file.buffer.Len() == 0 {
		t.Fatal("expected encoded snapshot to be written before sync failure")
	}
}

func TestNewRecorder_OwnerOnlyPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permissions not applicable on Windows")
	}
	dir := filepath.Join(t.TempDir(), "replay")
	rec, err := NewRecorder(dir, 0)
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}
	defer rec.Close()

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0700 {
		t.Errorf("dir perm = %04o, want 0700", perm)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 replay file, got %d", len(entries))
	}
	fInfo, err := entries[0].Info()
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if perm := fInfo.Mode().Perm(); perm != 0600 {
		t.Errorf("file perm = %04o, want 0600", perm)
	}
}

func TestRecorder_CloseSyncsBeforeClose(t *testing.T) {
	file := &stubSnapshotFile{}
	rec := &Recorder{
		file:    file,
		encoder: json.NewEncoder(file),
	}

	rec.Close()

	want := []string{"sync", "close"}
	if !reflect.DeepEqual(file.calls, want) {
		t.Fatalf("call order = %v, want %v", file.calls, want)
	}
	if rec.file != nil {
		t.Fatal("expected recorder file to be cleared on close")
	}
}

// decodeSnapshot is a test helper that decodes the first JSONL line from buf.
func decodeSnapshot(t *testing.T, buf *bytes.Buffer) Snapshot {
	t.Helper()
	var snap Snapshot
	if err := json.NewDecoder(buf).Decode(&snap); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	return snap
}

func TestRecorder_WriteSnapshot_AlwaysStripsPIDAndTmuxTarget(t *testing.T) {
	file := &stubSnapshotFile{}
	rec := &Recorder{
		file:    file,
		encoder: json.NewEncoder(file),
	}

	rec.WriteSnapshot([]*session.SessionState{
		{
			ID:         "claude:abc123",
			PID:        42,
			TmuxTarget: "main:2.0",
			WorkingDir: "/home/user/projects/secret-project",
		},
	})

	snap := decodeSnapshot(t, &file.buffer)
	if len(snap.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(snap.Sessions))
	}
	s := snap.Sessions[0]
	if s.PID != 0 {
		t.Errorf("PID should be stripped in replay, got %d", s.PID)
	}
	if s.TmuxTarget != "" {
		t.Errorf("TmuxTarget should be stripped in replay, got %q", s.TmuxTarget)
	}
}

func TestRecorder_WriteSnapshot_AlwaysMasksWorkingDir(t *testing.T) {
	file := &stubSnapshotFile{}
	rec := &Recorder{
		file:    file,
		encoder: json.NewEncoder(file),
	}

	rec.WriteSnapshot([]*session.SessionState{
		{ID: "s1", WorkingDir: "/home/user/projects/secret-project"},
	})

	snap := decodeSnapshot(t, &file.buffer)
	s := snap.Sessions[0]
	if s.WorkingDir != "secret-project" {
		t.Errorf("WorkingDir should be basename only, got %q", s.WorkingDir)
	}
}

func TestRecorder_WriteSnapshot_AppliesUserPrivacyFilter(t *testing.T) {
	file := &stubSnapshotFile{}
	rec := &Recorder{
		file:    file,
		encoder: json.NewEncoder(file),
	}

	// Set a user filter that blocks /tmp paths and masks session IDs.
	rec.SetPrivacyFilter(&session.PrivacyFilter{
		MaskSessionIDs: true,
		BlockedPaths:   []string{"/tmp/*"},
	})

	rec.WriteSnapshot([]*session.SessionState{
		{ID: "claude:keep", WorkingDir: "/home/user/project"},
		{ID: "claude:block", WorkingDir: "/tmp/scratch"},
	})

	snap := decodeSnapshot(t, &file.buffer)
	if len(snap.Sessions) != 1 {
		t.Fatalf("expected 1 session (blocked filtered out), got %d", len(snap.Sessions))
	}
	s := snap.Sessions[0]
	// Session ID should be hashed.
	if s.ID == "claude:keep" {
		t.Error("session ID should have been masked by user privacy filter")
	}
	if s.ID == "" {
		t.Error("masked session ID should not be empty")
	}
}

func TestRecorder_WriteSnapshot_NoUserFilterStillSanitizes(t *testing.T) {
	file := &stubSnapshotFile{}
	rec := &Recorder{
		file:    file,
		encoder: json.NewEncoder(file),
	}
	// No SetPrivacyFilter call — user filter is nil.

	rec.WriteSnapshot([]*session.SessionState{
		{
			ID:         "claude:raw",
			PID:        9999,
			TmuxTarget: "dev:1.0",
			WorkingDir: "/home/user/secret",
		},
	})

	snap := decodeSnapshot(t, &file.buffer)
	s := snap.Sessions[0]
	// Hardcoded replay sanitization still applies.
	if s.PID != 0 {
		t.Errorf("PID should be stripped even without user filter, got %d", s.PID)
	}
	if s.TmuxTarget != "" {
		t.Errorf("TmuxTarget should be stripped even without user filter, got %q", s.TmuxTarget)
	}
	if s.WorkingDir != "secret" {
		t.Errorf("WorkingDir should be basename even without user filter, got %q", s.WorkingDir)
	}
	// Session ID is NOT masked (no user filter requesting it).
	if s.ID != "claude:raw" {
		t.Errorf("session ID should be unmasked without user filter, got %q", s.ID)
	}
}

func TestRecorder_SetPrivacyFilter_ConcurrentSafe(t *testing.T) {
	file := &stubSnapshotFile{}
	rec := &Recorder{
		file:    file,
		encoder: json.NewEncoder(file),
	}

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			rec.SetPrivacyFilter(&session.PrivacyFilter{MaskPIDs: true})
		}
		close(done)
	}()

	for i := 0; i < 100; i++ {
		rec.WriteSnapshot([]*session.SessionState{
			{ID: "s1", WorkingDir: "/home/user/project"},
		})
	}
	<-done
}
