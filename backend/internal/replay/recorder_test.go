package replay

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-racer/backend/internal/session"
)

type stubSnapshotFile struct {
	mu       sync.Mutex
	buffer   bytes.Buffer
	calls    []string
	syncErr  error
	closeErr error
}

func (f *stubSnapshotFile) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "write")
	return f.buffer.Write(p)
}

func (f *stubSnapshotFile) Sync() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "sync")
	return f.syncErr
}

func (f *stubSnapshotFile) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "close")
	return f.closeErr
}

func (f *stubSnapshotFile) getCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.calls))
	copy(out, f.calls)
	return out
}

// newStubRecorder creates a stubSnapshotFile and a Recorder wired to it.
func newStubRecorder(syncErr, closeErr error) (*stubSnapshotFile, *Recorder) {
	file := &stubSnapshotFile{syncErr: syncErr, closeErr: closeErr}
	rec := &Recorder{
		file:    file,
		encoder: json.NewEncoder(file),
	}
	return file, rec
}

// assertCalls verifies the stub recorded the expected call sequence.
func assertCalls(t *testing.T, file *stubSnapshotFile, want []string) {
	t.Helper()
	got := file.getCalls()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("call order = %v, want %v", got, want)
	}
}

func TestRecorder_WriteSnapshotSyncsAfterEncode(t *testing.T) {
	file, rec := newStubRecorder(nil, nil)

	rec.WriteSnapshot(nil)

	assertCalls(t, file, []string{"write", "sync"})
	if file.buffer.Len() == 0 {
		t.Fatal("expected encoded snapshot to be written")
	}
}

func TestRecorder_WriteSnapshotKeepsDataOnSyncError(t *testing.T) {
	file, rec := newStubRecorder(errors.New("sync failed"), nil)

	rec.WriteSnapshot(nil)

	assertCalls(t, file, []string{"write", "sync"})
	if file.buffer.Len() == 0 {
		t.Fatal("expected encoded snapshot to be written before sync failure")
	}
}

func TestRecorder_CloseSyncsBeforeClose(t *testing.T) {
	file, rec := newStubRecorder(nil, nil)

	rec.Close()

	assertCalls(t, file, []string{"sync", "close"})
	if rec.file != nil {
		t.Fatal("expected recorder file to be cleared on close")
	}
}

func TestRecorder_CloseIsIdempotent(t *testing.T) {
	file, rec := newStubRecorder(nil, nil)

	rec.Close()
	rec.Close()

	assertCalls(t, file, []string{"sync", "close"})
}

func TestRecorder_WriteSnapshotNilFileIsNoop(t *testing.T) {
	rec := &Recorder{file: nil}
	rec.WriteSnapshot(nil) // should not panic
}

func TestRecorder_WriteSnapshotEncodesJSON(t *testing.T) {
	file, rec := newStubRecorder(nil, nil)

	sessions := []*session.SessionState{
		{ID: "s1", Name: "test-session", Source: "claude"},
	}
	rec.WriteSnapshot(sessions)

	var snap Snapshot
	if err := json.Unmarshal(file.buffer.Bytes(), &snap); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if len(snap.Sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(snap.Sessions))
	}
	if snap.Sessions[0].ID != "s1" {
		t.Fatalf("session ID = %q, want %q", snap.Sessions[0].ID, "s1")
	}
	if snap.Timestamp.IsZero() {
		t.Fatal("expected non-zero timestamp")
	}
}

func TestRecorder_WriteSnapshotConcurrent(t *testing.T) {
	file, rec := newStubRecorder(nil, nil)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec.WriteSnapshot(nil)
		}()
	}
	wg.Wait()

	calls := file.getCalls()
	writeCount := 0
	syncCount := 0
	for _, c := range calls {
		switch c {
		case "write":
			writeCount++
		case "sync":
			syncCount++
		}
	}
	if writeCount != 10 {
		t.Fatalf("write count = %d, want 10", writeCount)
	}
	if syncCount != 10 {
		t.Fatalf("sync count = %d, want 10", syncCount)
	}
}

func TestNewRecorder_EmptyDirReturnsNil(t *testing.T) {
	rec, err := NewRecorder("", 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec != nil {
		t.Fatal("expected nil recorder for empty dir")
	}
}

func TestNewRecorder_CreatesDirAndFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "replays")
	rec, err := NewRecorder(dir, 7)
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}
	defer rec.Close()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d files, want 1", len(entries))
	}
	if !strings.HasSuffix(entries[0].Name(), ".jsonl") {
		t.Fatalf("file %q does not have .jsonl extension", entries[0].Name())
	}
}

func TestNewRecorder_DirHasOwnerOnlyPerms(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "replays")
	rec, err := NewRecorder(dir, 7)
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}
	defer rec.Close()

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat dir: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Fatalf("dir perm = %o, want 0700", perm)
	}
}

func TestNewRecorder_FileHasOwnerOnlyPerms(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "replays")
	rec, err := NewRecorder(dir, 7)
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}
	defer rec.Close()

	matches, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("got %d .jsonl files, want 1", len(matches))
	}
	info, err := os.Stat(matches[0])
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("file perm = %o, want 0600", perm)
	}
}

// createOldFile writes a .jsonl file with its modtime set to daysAgo.
func createOldFile(t *testing.T, dir, name string, daysAgo int) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	if daysAgo > 0 {
		past := time.Now().Add(-time.Duration(daysAgo) * 24 * time.Hour)
		if err := os.Chtimes(path, past, past); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestPruneOldFiles_NonPositiveRetentionIsNoop(t *testing.T) {
	cases := []struct {
		name          string
		retentionDays int
	}{
		{"zero", 0},
		{"negative", -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := createOldFile(t, dir, "old.jsonl", 30)

			pruneOldFiles(dir, tc.retentionDays)

			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Fatalf("file should not be pruned when retentionDays is %d", tc.retentionDays)
			}
		})
	}
}

func TestPruneOldFiles_DeletesOldKeepsRecent(t *testing.T) {
	dir := t.TempDir()
	oldFile := createOldFile(t, dir, "old.jsonl", 10)
	recentFile := createOldFile(t, dir, "recent.jsonl", 0)

	pruneOldFiles(dir, 7)

	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatal("old file should have been pruned")
	}
	if _, err := os.Stat(recentFile); os.IsNotExist(err) {
		t.Fatal("recent file should be kept")
	}
}

func TestPruneOldFiles_IgnoresDirectories(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "subdir.jsonl")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(subDir, past, past); err != nil {
		t.Fatal(err)
	}

	pruneOldFiles(dir, 7)

	if _, err := os.Stat(subDir); os.IsNotExist(err) {
		t.Fatal("directory should not be pruned even if old")
	}
}

func TestPruneOldFiles_IgnoresNonJSONL(t *testing.T) {
	dir := t.TempDir()
	txtFile := createOldFile(t, dir, "notes.txt", 30)

	pruneOldFiles(dir, 7)

	if _, err := os.Stat(txtFile); os.IsNotExist(err) {
		t.Fatal("non-jsonl file should not be pruned")
	}
}
