package replay

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
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
