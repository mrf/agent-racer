package replay

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/agent-racer/backend/internal/session"
)

// Snapshot is a point-in-time capture of all active session states.
type Snapshot struct {
	Timestamp time.Time               `json:"t"`
	Sessions  []*session.SessionState `json:"s"`
}

type snapshotFile interface {
	io.Writer
	Sync() error
	Close() error
}

// Recorder appends session snapshots to a JSONL replay file.
// One file is created per server run; old files are pruned on startup.
type Recorder struct {
	mu      sync.Mutex
	file    snapshotFile
	encoder *json.Encoder
}

// NewRecorder opens a new replay file under dir and returns a Recorder.
// Files older than retentionDays are deleted. Returns nil, nil when dir is "".
func NewRecorder(dir string, retentionDays int) (*Recorder, error) {
	if dir == "" {
		return nil, nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("replay: create dir %s: %w", dir, err)
	}

	pruneOldFiles(dir, retentionDays)

	name := time.Now().Format("2006-01-02_15-04-05") + ".jsonl"
	path := filepath.Join(dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("replay: open file %s: %w", path, err)
	}

	slog.Info("recorder started", "component", "replay", "path", path)
	return &Recorder{file: f, encoder: json.NewEncoder(f)}, nil
}

// WriteSnapshot appends a snapshot to the replay file.
// sessions must be safe to read concurrently (use cloned copies from store.GetAll).
func (r *Recorder) WriteSnapshot(sessions []*session.SessionState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return
	}
	snap := Snapshot{
		Timestamp: time.Now().UTC(),
		Sessions:  sessions,
	}
	if err := r.encoder.Encode(snap); err != nil {
		slog.Error("write snapshot failed", "component", "replay", "error", err)
		return
	}
	if err := r.file.Sync(); err != nil {
		slog.Error("sync snapshot failed", "component", "replay", "error", err)
	}
}

// Close flushes and closes the replay file.
func (r *Recorder) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file != nil {
		if err := r.file.Sync(); err != nil {
			slog.Error("sync close failed", "component", "replay", "error", err)
		}
		_ = r.file.Close()
		r.file = nil
	}
}

// pruneOldFiles removes .jsonl files in dir older than retentionDays.
func pruneOldFiles(dir string, retentionDays int) {
	if retentionDays <= 0 {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".jsonl" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			path := filepath.Join(dir, e.Name())
			if removeErr := os.Remove(path); removeErr == nil {
				slog.Info("pruned old file", "component", "replay", "file", e.Name())
			}
		}
	}
}
