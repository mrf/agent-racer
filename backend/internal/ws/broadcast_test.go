package ws

import (
	"testing"
	"time"

	"github.com/agent-racer/backend/internal/session"
)

func newTestBroadcaster(store *session.Store, filter *session.PrivacyFilter) *Broadcaster {
	if filter == nil {
		filter = &session.PrivacyFilter{}
	}
	return &Broadcaster{
		clients: make(map[*client]bool),
		store:   store,
		privacy: filter,
	}
}

// assertSessionIDs checks that the result slice contains exactly the expected
// session IDs, in order.
func assertSessionIDs(t *testing.T, result []*session.SessionState, expected ...string) {
	t.Helper()
	if len(result) != len(expected) {
		t.Fatalf("expected %d sessions, got %d", len(expected), len(result))
	}
	for i, id := range expected {
		if result[i].ID != id {
			t.Errorf("result[%d]: expected %s, got %s", i, id, result[i].ID)
		}
	}
}

func TestFilterSessions_NoFilter(t *testing.T) {
	b := newTestBroadcaster(session.NewStore(), nil)

	sessions := []*session.SessionState{
		{ID: "s1", WorkingDir: "/home/user/project-a", PID: 100},
		{ID: "s2", WorkingDir: "/home/user/project-b", PID: 200},
	}

	assertSessionIDs(t, b.FilterSessions(sessions), "s1", "s2")
}

func TestFilterSessions_PathFiltering(t *testing.T) {
	tests := []struct {
		name     string
		filter   *session.PrivacyFilter
		sessions []*session.SessionState
		wantIDs  []string
	}{
		{
			name: "BlockedPaths",
			filter: &session.PrivacyFilter{
				BlockedPaths: []string{"/tmp/*"},
			},
			sessions: []*session.SessionState{
				{ID: "s1", WorkingDir: "/home/user/project"},
				{ID: "s2", WorkingDir: "/tmp/scratch"},
				{ID: "s3", WorkingDir: "/tmp/other"},
			},
			wantIDs: []string{"s1"},
		},
		{
			name: "AllowedPaths",
			filter: &session.PrivacyFilter{
				AllowedPaths: []string{"/home/user/work/*"},
			},
			sessions: []*session.SessionState{
				{ID: "s1", WorkingDir: "/home/user/work/project-a"},
				{ID: "s2", WorkingDir: "/home/user/personal/diary"},
				{ID: "s3", WorkingDir: "/other/path"},
			},
			wantIDs: []string{"s1"},
		},
		{
			name: "AllowAndBlock",
			filter: &session.PrivacyFilter{
				AllowedPaths: []string{"/home/user/*"},
				BlockedPaths: []string{"/home/user/secret"},
			},
			sessions: []*session.SessionState{
				{ID: "s1", WorkingDir: "/home/user/project"},
				{ID: "s2", WorkingDir: "/home/user/secret"},
				{ID: "s3", WorkingDir: "/other/place"},
			},
			wantIDs: []string{"s1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTestBroadcaster(session.NewStore(), tt.filter)
			assertSessionIDs(t, b.FilterSessions(tt.sessions), tt.wantIDs...)
		})
	}
}

func TestFilterSessions_Masking(t *testing.T) {
	b := newTestBroadcaster(session.NewStore(), &session.PrivacyFilter{
		MaskWorkingDirs: true,
		MaskPIDs:        true,
		MaskTmuxTargets: true,
	})

	sessions := []*session.SessionState{
		{
			ID:         "s1",
			WorkingDir: "/home/user/projects/myapp",
			PID:        12345,
			TmuxTarget: "main:2.0",
		},
	}

	result := b.FilterSessions(sessions)
	if len(result) != 1 {
		t.Fatalf("expected 1 session, got %d", len(result))
	}

	s := result[0]
	if s.WorkingDir != "myapp" {
		t.Errorf("WorkingDir should be masked to basename, got %q", s.WorkingDir)
	}
	if s.PID != 0 {
		t.Errorf("PID should be masked to 0, got %d", s.PID)
	}
	if s.TmuxTarget != "" {
		t.Errorf("TmuxTarget should be masked to empty, got %q", s.TmuxTarget)
	}
}

func TestFilterSessions_MaskSessionIDs(t *testing.T) {
	b := newTestBroadcaster(session.NewStore(), &session.PrivacyFilter{
		MaskSessionIDs: true,
	})

	sessions := []*session.SessionState{
		{ID: "claude:abc123"},
	}

	result := b.FilterSessions(sessions)
	if len(result) != 1 {
		t.Fatalf("expected 1 session, got %d", len(result))
	}
	if result[0].ID == "claude:abc123" {
		t.Error("session ID should have been masked")
	}
	if result[0].ID == "" {
		t.Error("masked session ID should not be empty")
	}
}

func TestFilterSessions_EmptySlice(t *testing.T) {
	b := newTestBroadcaster(session.NewStore(), &session.PrivacyFilter{
		BlockedPaths: []string{"/tmp/*"},
	})

	assertSessionIDs(t, b.FilterSessions(nil))
	assertSessionIDs(t, b.FilterSessions([]*session.SessionState{}))
}

func TestFilterSessions_EmptyWorkingDir(t *testing.T) {
	b := newTestBroadcaster(session.NewStore(), &session.PrivacyFilter{
		AllowedPaths: []string{"/home/user/*"},
	})

	sessions := []*session.SessionState{
		{ID: "s1", WorkingDir: ""},
		{ID: "s2", WorkingDir: "/home/user/project"},
	}

	assertSessionIDs(t, b.FilterSessions(sessions), "s1", "s2")
}

func TestFilterSessions_DoesNotMutateInput(t *testing.T) {
	b := newTestBroadcaster(session.NewStore(), &session.PrivacyFilter{
		MaskWorkingDirs: true,
		MaskPIDs:        true,
		BlockedPaths:    []string{"/tmp/*"},
	})

	original := []*session.SessionState{
		{ID: "s1", WorkingDir: "/home/user/project", PID: 100},
		{ID: "s2", WorkingDir: "/tmp/scratch", PID: 200},
	}

	b.FilterSessions(original)

	if original[0].WorkingDir != "/home/user/project" {
		t.Error("input slice element was mutated")
	}
	if original[0].PID != 100 {
		t.Error("input slice element PID was mutated")
	}
	if len(original) != 2 {
		t.Error("input slice length was mutated")
	}
}

func TestSetPrivacyFilter(t *testing.T) {
	b := newTestBroadcaster(session.NewStore(), nil)

	sessions := []*session.SessionState{
		{ID: "s1", WorkingDir: "/tmp/scratch"},
		{ID: "s2", WorkingDir: "/home/user/project"},
	}

	// Default: no filtering
	assertSessionIDs(t, b.FilterSessions(sessions), "s1", "s2")

	// Set a filter that blocks /tmp
	b.SetPrivacyFilter(&session.PrivacyFilter{
		BlockedPaths: []string{"/tmp/*"},
	})
	assertSessionIDs(t, b.FilterSessions(sessions), "s2")

	// Replace filter: now block /home instead
	b.SetPrivacyFilter(&session.PrivacyFilter{
		BlockedPaths: []string{"/home/*"},
	})
	assertSessionIDs(t, b.FilterSessions(sessions), "s1")
}

func TestFilterSessions_WithStoreData(t *testing.T) {
	store := session.NewStore()
	store.Update(&session.SessionState{
		ID:         "stored-1",
		WorkingDir: "/home/user/project-a",
		Activity:   session.Thinking,
	})
	store.Update(&session.SessionState{
		ID:         "stored-2",
		WorkingDir: "/tmp/scratch",
		Activity:   session.ToolUse,
	})

	b := newTestBroadcaster(store, &session.PrivacyFilter{
		BlockedPaths: []string{"/tmp/*"},
		MaskPIDs:     true,
	})

	assertSessionIDs(t, b.FilterSessions(store.GetAll()), "stored-1")
}

func TestNewBroadcaster_DefaultPrivacyFilter(t *testing.T) {
	b := NewBroadcaster(session.NewStore(), 100*time.Millisecond, time.Hour, 0)
	defer b.snapshotTicker.Stop()

	if b.privacy == nil {
		t.Fatal("default privacy filter should not be nil")
	}
	if !b.privacy.IsNoop() {
		t.Error("default privacy filter should be a no-op")
	}

	sessions := []*session.SessionState{
		{ID: "s1", WorkingDir: "/any/path", PID: 42},
	}
	result := b.FilterSessions(sessions)
	if len(result) != 1 {
		t.Fatalf("default filter should pass all, got %d", len(result))
	}
	if result[0].PID != 42 {
		t.Error("default filter should not mask PID")
	}
}

func TestBroadcaster_SequenceNumberWrapAround(t *testing.T) {
	// Test that sequence numbers wrap correctly at uint64 max.
	// This is a theoretical concern since 2^64 messages would take centuries at normal rates,
	// but the behavior is well-defined and should be verified.

	b := newTestBroadcaster(session.NewStore(), nil)

	// Set seq to near max uint64, leaving room to test wrapping
	maxUint64 := ^uint64(0)
	b.seq.Store(maxUint64 - 3)

	// Collect sequence numbers from increments
	var seqs []uint64
	for i := 0; i < 5; i++ {
		seq := b.seq.Add(1)
		seqs = append(seqs, seq)
	}

	// Verify expected sequence: max-2, max-1, max, 0, 1
	expected := []uint64{maxUint64 - 2, maxUint64 - 1, maxUint64, 0, 1}
	if len(seqs) != len(expected) {
		t.Fatalf("expected %d sequence numbers, got %d", len(expected), len(seqs))
	}

	for i := 0; i < len(expected); i++ {
		if seqs[i] != expected[i] {
			t.Errorf("seq[%d]: expected %d, got %d", i, expected[i], seqs[i])
		}
	}
}

func TestBroadcaster_SequenceNumberIncrement(t *testing.T) {
	// Test that sequence numbers increment correctly in normal operation.

	b := newTestBroadcaster(session.NewStore(), nil)

	// Verify seq starts at 0
	if b.seq.Load() != 0 {
		t.Errorf("expected initial seq to be 0, got %d", b.seq.Load())
	}

	// Increment and verify sequential increase
	var seqs []uint64
	for i := 0; i < 5; i++ {
		seq := b.seq.Add(1)
		seqs = append(seqs, seq)
	}

	// Verify 1, 2, 3, 4, 5
	for i := 0; i < 5; i++ {
		expected := uint64(i + 1)
		if seqs[i] != expected {
			t.Errorf("seq[%d]: expected %d, got %d", i, expected, seqs[i])
		}
	}
}
