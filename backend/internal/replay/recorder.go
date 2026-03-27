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

// replayPrivacy is a hardcoded privacy filter applied to all replay snapshots.
// Fields with zero replay value (PID, TmuxTarget) are always stripped, and
// working directories are always reduced to their basename — full filesystem
// paths have no playback value and persist on disk for the retention period.
var replayPrivacy = &session.PrivacyFilter{
	MaskWorkingDirs: true,
	MaskPIDs:        true,
	MaskTmuxTargets: true,
}

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
//
// Sessions are sanitized before writing: fields with zero replay value (PID,
// TmuxTarget) are always stripped, working directories are reduced to basename,
// and the user's privacy filter (path filtering, session ID masking) is applied.
type Recorder struct {
	mu      sync.Mutex
	file    snapshotFile
	encoder *json.Encoder

	privMu  sync.RWMutex
	privacy *session.PrivacyFilter // user-configured filter (may be nil)
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

// SetPrivacyFilter updates the user-configured privacy filter applied to
// replay snapshots. This is applied in addition to the hardcoded replay
// sanitization (PID, TmuxTarget, WorkingDir). Safe for concurrent use.
func (r *Recorder) SetPrivacyFilter(f *session.PrivacyFilter) {
	r.privMu.Lock()
	r.privacy = f
	r.privMu.Unlock()
}

func (r *Recorder) userPrivacy() *session.PrivacyFilter {
	r.privMu.RLock()
	f := r.privacy
	r.privMu.RUnlock()
	return f
}

// sanitize applies the user's privacy filter (path filtering, session ID
// masking) followed by the hardcoded replay sanitization (strip PID,
// TmuxTarget, reduce WorkingDir to basename).
func (r *Recorder) sanitize(sessions []*session.SessionState) []*session.SessionState {
	// Apply user's path-based filtering and field masking.
	filtered := sessions
	if userFilter := r.userPrivacy(); userFilter != nil {
		filtered = userFilter.FilterSlice(sessions)
	}

	// Apply hardcoded replay sanitization on top.
	result := make([]*session.SessionState, len(filtered))
	for i := 0; i < len(filtered); i++ {
		result[i] = replayPrivacy.Apply(filtered[i])
	}
	return result
}

// WriteSnapshot appends a snapshot to the replay file.
// sessions must be safe to read concurrently (use cloned copies from store.GetAll).
// Sessions are sanitized before writing (see Recorder doc).
func (r *Recorder) WriteSnapshot(sessions []*session.SessionState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return
	}
	snap := Snapshot{
		Timestamp: time.Now().UTC(),
		Sessions:  r.sanitize(sessions),
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
