package racer

import (
	"testing"
	"time"

	"github.com/agent-racer/backend/internal/session"
	"github.com/agent-racer/backend/internal/ws"
)

// newTestBridge creates a Bridge backed by a real store and broadcaster.
func newTestBridge() (*Bridge, *session.Store) {
	store := session.NewStore()
	broadcaster := ws.NewBroadcaster(store, time.Hour, time.Hour, 0)
	bridge := NewBridge(store, broadcaster)
	return bridge, store
}

func makeLocalSession(id string, util float64) *session.SessionState {
	return &session.SessionState{
		ID:                 id,
		Name:               id,
		Source:             "claude",
		Activity:           session.Thinking,
		ContextTokens:      int(util * 200000),
		MaxContextTokens:   200000,
		ContextUtilization: util,
		WorkingDir:         "/home/user/projects/" + id,
		StartedAt:          time.Now(),
		LastActivityAt:     time.Now(),
	}
}

func makeLocalTerminal(id string) *session.SessionState {
	t := time.Now()
	return &session.SessionState{
		ID:          id,
		Name:        id,
		Source:      "claude",
		Activity:    session.Complete,
		WorkingDir:  "/home/user/projects/" + id,
		StartedAt:   time.Now(),
		CompletedAt: &t,
	}
}

// --- Position Enrichment ---

func TestBridge_PushUpdate_AssignsPositions(t *testing.T) {
	bridge, store := newTestBridge()

	updates := []*session.SessionState{
		makeLocalSession("high", 0.9),
		makeLocalSession("mid", 0.5),
		makeLocalSession("low", 0.2),
	}
	bridge.PushUpdate(updates)

	// Verify positions were set on the updates.
	posMap := make(map[string]int, len(updates))
	for _, u := range updates {
		posMap[u.ID] = u.Position
	}
	if posMap["high"] != 1 {
		t.Errorf("high position = %d, want 1", posMap["high"])
	}
	if posMap["mid"] != 2 {
		t.Errorf("mid position = %d, want 2", posMap["mid"])
	}
	if posMap["low"] != 3 {
		t.Errorf("low position = %d, want 3", posMap["low"])
	}

	// Verify store has the updates.
	all := store.GetAll()
	if len(all) != 3 {
		t.Fatalf("store has %d sessions, want 3", len(all))
	}
}

func TestBridge_PushUpdate_PositionDelta(t *testing.T) {
	bridge, _ := newTestBridge()

	// Initial: a=P1, b=P2.
	bridge.PushUpdate([]*session.SessionState{
		makeLocalSession("a", 0.8),
		makeLocalSession("b", 0.3),
	})

	// b overtakes a.
	updates := []*session.SessionState{
		makeLocalSession("b", 0.9),
		makeLocalSession("a", 0.8),
	}
	bridge.PushUpdate(updates)

	posMap := make(map[string]int, len(updates))
	deltaMap := make(map[string]int, len(updates))
	for _, u := range updates {
		posMap[u.ID] = u.Position
		deltaMap[u.ID] = u.PositionDelta
	}

	if posMap["b"] != 1 {
		t.Errorf("b position = %d, want 1", posMap["b"])
	}
	if deltaMap["b"] != 1 {
		t.Errorf("b positionDelta = %d, want 1 (moved up)", deltaMap["b"])
	}
	if posMap["a"] != 2 {
		t.Errorf("a position = %d, want 2", posMap["a"])
	}
	if deltaMap["a"] != -1 {
		t.Errorf("a positionDelta = %d, want -1 (dropped)", deltaMap["a"])
	}
}

func TestBridge_PushUpdate_TerminalHasNoPosition(t *testing.T) {
	bridge, _ := newTestBridge()

	updates := []*session.SessionState{
		makeLocalSession("active", 0.5),
		makeLocalTerminal("done"),
	}
	bridge.PushUpdate(updates)

	for _, u := range updates {
		if u.ID == "done" && u.Position != 0 {
			t.Errorf("terminal session position = %d, want 0", u.Position)
		}
		if u.ID == "active" && u.Position != 1 {
			t.Errorf("active session position = %d, want 1", u.Position)
		}
	}
}

func TestBridge_PushUpdate_PositionsIncludeStoreContext(t *testing.T) {
	bridge, store := newTestBridge()

	// Pre-populate store with a session not in this update.
	store.Update(makeLocalSession("existing", 0.7))

	// Push a new session with lower utilization.
	updates := []*session.SessionState{
		makeLocalSession("new", 0.3),
	}
	bridge.PushUpdate(updates)

	// "new" should be P2 (behind "existing" at 0.7).
	if updates[0].Position != 2 {
		t.Errorf("new session position = %d, want 2 (behind existing)", updates[0].Position)
	}
}

// --- Removals ---

func TestBridge_PushRemoval_CleansUpTracking(t *testing.T) {
	bridge, store := newTestBridge()

	bridge.PushUpdate([]*session.SessionState{
		makeLocalSession("a", 0.8),
		makeLocalSession("b", 0.3),
	})

	bridge.PushRemoval([]string{"a"})

	// "a" should be removed from store.
	if _, ok := store.Get("a"); ok {
		t.Error("session a should have been removed from store")
	}

	// "b" should still exist.
	if _, ok := store.Get("b"); !ok {
		t.Error("session b should still exist in store")
	}

	// Push an update for b — it should be P1 now (no more a).
	updates := []*session.SessionState{makeLocalSession("b", 0.4)}
	bridge.PushUpdate(updates)
	if updates[0].Position != 1 {
		t.Errorf("b position = %d, want 1 after a was removed", updates[0].Position)
	}
}

// --- Empty inputs ---

func TestBridge_PushUpdate_EmptySliceIsNoop(t *testing.T) {
	bridge, _ := newTestBridge()
	bridge.PushUpdate(nil)
	bridge.PushUpdate([]*session.SessionState{})
	// No panic, no error.
}

func TestBridge_PushRemoval_EmptySliceIsNoop(t *testing.T) {
	bridge, _ := newTestBridge()
	bridge.PushRemoval(nil)
	bridge.PushRemoval([]string{})
}

// --- Stable tiebreak ---

func TestBridge_PushUpdate_StableTiebreak(t *testing.T) {
	bridge, _ := newTestBridge()

	// Same utilization — positions should be stable by ID.
	updates := []*session.SessionState{
		makeLocalSession("z-last", 0.5),
		makeLocalSession("a-first", 0.5),
	}
	bridge.PushUpdate(updates)

	posMap := make(map[string]int, len(updates))
	for _, u := range updates {
		posMap[u.ID] = u.Position
	}

	// "a-first" sorts before "z-last" lexicographically.
	if posMap["a-first"] != 1 {
		t.Errorf("a-first position = %d, want 1 (tiebreak by ID)", posMap["a-first"])
	}
	if posMap["z-last"] != 2 {
		t.Errorf("z-last position = %d, want 2 (tiebreak by ID)", posMap["z-last"])
	}
}
