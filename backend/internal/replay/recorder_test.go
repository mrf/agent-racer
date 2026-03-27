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
