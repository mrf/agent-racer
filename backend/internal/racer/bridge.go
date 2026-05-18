package racer

import (
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/agent-racer/backend/internal/session"
	"github.com/agent-racer/backend/internal/ws"
)

// Bridge enriches session updates with position and overtake data before
// forwarding them to the WebSocket broadcaster. It sits between the monitor
// (or mock generator) and the broadcaster in the update pipeline.
//
// Flow: monitor.poll() -> bridge.PushUpdate() -> enrich Position/PositionDelta
//       -> store.BatchUpdateAndNotify() -> broadcaster.QueueUpdate()
type Bridge struct {
	mu          sync.Mutex
	store       *session.Store
	broadcaster *ws.Broadcaster
	prevPos     map[string]int
	prevNames   map[string]string
}

// NewBridge creates a Bridge that enriches updates with racing position data
// and forwards them to the given broadcaster. The store is used both for
// reading the current session set (to compute global positions) and for
// writing enriched updates.
func NewBridge(store *session.Store, broadcaster *ws.Broadcaster) *Bridge {
	return &Bridge{
		store:       store,
		broadcaster: broadcaster,
		prevPos:     make(map[string]int),
		prevNames:   make(map[string]string),
	}
}

// PushUpdate enriches updates with Position and PositionDelta, writes them
// to the store, queues them for broadcast, and broadcasts any overtake events.
//
// It reads the current store state to compute global positions across all
// sessions (not just the ones being updated this cycle). The store write
// happens before the broadcast queue to ensure snapshot consistency.
func (b *Bridge) PushUpdate(updates []*session.SessionState) {
	if len(updates) == 0 {
		return
	}

	// Read current store state for position computation.
	allSessions := b.store.GetAll()

	// Build merged view: existing sessions + incoming updates (updates override).
	merged := make(map[string]*session.SessionState, len(allSessions)+len(updates))
	for i := 0; i < len(allSessions); i++ {
		merged[allSessions[i].ID] = allSessions[i]
	}
	for i := 0; i < len(updates); i++ {
		merged[updates[i].ID] = updates[i]
	}

	b.mu.Lock()

	// Compute positions from all non-terminal sessions in the merged view.
	type entry struct {
		id   string
		util float64
	}
	racing := make([]entry, 0, len(merged))
	for id, s := range merged {
		if !s.IsTerminal() {
			racing = append(racing, entry{id, s.ContextUtilization})
		}
	}
	sort.Slice(racing, func(i, j int) bool {
		if racing[i].util != racing[j].util {
			return racing[i].util > racing[j].util
		}
		return racing[i].id < racing[j].id // stable tiebreak
	})

	posMap := make(map[string]int, len(racing))
	for i := 0; i < len(racing); i++ {
		posMap[racing[i].id] = i + 1
	}

	// Build reverse map of previous positions for overtake detection.
	prevOrder := make(map[int]string, len(b.prevPos))
	for id, pos := range b.prevPos {
		if pos > 0 {
			prevOrder[pos] = id
		}
	}

	// Apply positions to updates and detect overtakes.
	var overtakes []OvertakeEvent
	for i := 0; i < len(updates); i++ {
		u := updates[i]
		newPos := posMap[u.ID] // 0 for terminal sessions
		oldPos := b.prevPos[u.ID]

		u.Position = newPos
		if oldPos > 0 && newPos > 0 {
			u.PositionDelta = oldPos - newPos // positive = moved up
		} else {
			u.PositionDelta = 0
		}

		// Overtake: session moved up and displaced someone at its new position.
		if newPos > 0 && u.PositionDelta > 0 {
			overtakenID, ok := prevOrder[newPos]
			if ok && overtakenID != u.ID {
				overtakenName := b.prevNames[overtakenID]
				if overtakenName == "" {
					overtakenName = overtakenID
				}
				overtakes = append(overtakes, OvertakeEvent{
					OvertakerID:   u.ID,
					OvertakerName: u.Name,
					OvertakenID:   overtakenID,
					OvertakenName: overtakenName,
					NewPosition:   newPos,
					At:            time.Now(),
				})
			}
		}
	}

	// Update position tracking for next cycle.
	b.prevPos = posMap
	nameMap := make(map[string]string, len(merged))
	for id, s := range merged {
		nameMap[id] = s.Name
	}
	b.prevNames = nameMap

	b.mu.Unlock()

	// Write enriched updates to store, then queue broadcast.
	// Order matters: store write before broadcast ensures snapshot consistency.
	b.store.BatchUpdateAndNotify(updates, func() {
		b.broadcaster.QueueUpdate(updates)
	})

	// Broadcast overtake events.
	for i := 0; i < len(overtakes); i++ {
		ov := &overtakes[i]
		msg, err := ws.NewOvertakeMessage(ws.OvertakePayload{
			OvertakerID:   ov.OvertakerID,
			OvertakerName: ov.OvertakerName,
			OvertakenID:   ov.OvertakenID,
			OvertakenName: ov.OvertakenName,
			NewPosition:   ov.NewPosition,
		})
		if err != nil {
			slog.Error("overtake message marshal failed", "component", "racer", "error", err)
			continue
		}
		b.broadcaster.BroadcastMessage(msg)
	}
}

// PushRemoval forwards session removals to the store and broadcaster,
// cleaning up internal position tracking for the removed sessions.
func (b *Bridge) PushRemoval(ids []string) {
	if len(ids) == 0 {
		return
	}

	b.mu.Lock()
	for i := 0; i < len(ids); i++ {
		delete(b.prevPos, ids[i])
		delete(b.prevNames, ids[i])
	}
	b.mu.Unlock()

	b.store.BatchRemoveAndNotify(ids, func() {
		b.broadcaster.QueueRemoval(ids)
	})
}

// QueueCompletion forwards a completion event to the broadcaster.
// Completions do not involve racing enrichment — they are pass-through.
func (b *Bridge) QueueCompletion(sessionID string, activity session.Activity, name string) {
	b.broadcaster.QueueCompletion(sessionID, activity, name)
}
