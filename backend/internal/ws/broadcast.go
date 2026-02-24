package ws

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/agent-racer/backend/internal/session"
	"github.com/gorilla/websocket"
)

type client struct {
	conn *websocket.Conn
	send chan []byte
}

func newClient(conn *websocket.Conn) *client {
	c := &client{
		conn: conn,
		send: make(chan []byte, 64),
	}
	go c.writePump()
	return c
}

func (c *client) writePump() {
	defer c.conn.Close()
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}

func (c *client) close() {
	close(c.send)
}

type Broadcaster struct {
	mu             sync.RWMutex
	clients        map[*client]bool
	store          *session.Store
	privacy        *session.PrivacyFilter
	throttle       time.Duration
	snapshotTicker *time.Ticker
	pendingUpdates []*session.SessionState
	pendingRemoved []string
	flushTimer     *time.Timer
	flushMu        sync.Mutex
	healthHook     func() []SourceHealthPayload
}

func NewBroadcaster(store *session.Store, throttle, snapshotInterval time.Duration) *Broadcaster {
	b := &Broadcaster{
		clients:  make(map[*client]bool),
		store:    store,
		privacy:  &session.PrivacyFilter{},
		throttle: throttle,
	}

	b.snapshotTicker = time.NewTicker(snapshotInterval)
	go b.snapshotLoop()

	return b
}

// SetPrivacyFilter configures the privacy filter applied to all outgoing
// session data. Must be called before any clients connect.
func (b *Broadcaster) SetPrivacyFilter(f *session.PrivacyFilter) {
	b.privacy = f
}

// SetHealthHook registers a function that returns the current source health
// status for inclusion in snapshot broadcasts.
func (b *Broadcaster) SetHealthHook(hook func() []SourceHealthPayload) {
	b.healthHook = hook
}

// FilterSessions applies the privacy filter to the given sessions, removing
// blocked sessions and masking sensitive fields.
func (b *Broadcaster) FilterSessions(sessions []*session.SessionState) []*session.SessionState {
	return b.privacy.FilterSlice(sessions)
}

func (b *Broadcaster) AddClient(conn *websocket.Conn) *client {
	c := newClient(conn)

	b.mu.Lock()
	b.clients[c] = true
	b.mu.Unlock()

	data, _ := json.Marshal(b.snapshotMessage())

	select {
	case c.send <- data:
	default:
		// Client too slow, drop the snapshot
	}

	return c
}

func (b *Broadcaster) RemoveClient(c *client) {
	b.mu.Lock()
	if _, ok := b.clients[c]; ok {
		delete(b.clients, c)
		c.close()
	}
	b.mu.Unlock()
}

func (b *Broadcaster) QueueUpdate(states []*session.SessionState) {
	b.flushMu.Lock()
	defer b.flushMu.Unlock()

	b.pendingUpdates = append(b.pendingUpdates, states...)

	if b.flushTimer == nil {
		b.flushTimer = time.AfterFunc(b.throttle, b.flush)
	}
}

func (b *Broadcaster) QueueRemoval(ids []string) {
	b.flushMu.Lock()
	defer b.flushMu.Unlock()

	b.pendingRemoved = append(b.pendingRemoved, ids...)

	if b.flushTimer == nil {
		b.flushTimer = time.AfterFunc(b.throttle, b.flush)
	}
}

func (b *Broadcaster) BroadcastAchievement(payload AchievementUnlockedPayload) {
	b.broadcast(WSMessage{
		Type:    MsgAchievementUnlocked,
		Payload: payload,
	})
}

func (b *Broadcaster) QueueCompletion(sessionID string, activity session.Activity, name string) {
	msg := WSMessage{
		Type: MsgCompletion,
		Payload: CompletionPayload{
			SessionID: sessionID,
			Activity:  activity,
			Name:      name,
		},
	}
	b.broadcast(msg)
}

func (b *Broadcaster) flush() {
	b.flushMu.Lock()
	updates := b.pendingUpdates
	removed := b.pendingRemoved
	b.pendingUpdates = nil
	b.pendingRemoved = nil
	b.flushTimer = nil
	b.flushMu.Unlock()

	if len(updates) == 0 && len(removed) == 0 {
		return
	}

	filtered := b.privacy.FilterSlice(updates)
	if len(filtered) == 0 && len(removed) == 0 {
		return
	}

	msg := WSMessage{
		Type: MsgDelta,
		Payload: DeltaPayload{
			Updates: filtered,
			Removed: removed,
		},
	}
	b.broadcast(msg)
}

func (b *Broadcaster) snapshotLoop() {
	for range b.snapshotTicker.C {
		b.broadcast(b.snapshotMessage())
	}
}

// snapshotMessage builds a full snapshot WSMessage including sessions and
// source health status (when a health hook is registered).
func (b *Broadcaster) snapshotMessage() WSMessage {
	payload := SnapshotPayload{
		Sessions: b.privacy.FilterSlice(b.store.GetAll()),
	}
	if b.healthHook != nil {
		payload.SourceHealth = b.healthHook()
	}
	return WSMessage{
		Type:    MsgSnapshot,
		Payload: payload,
	}
}

func (b *Broadcaster) broadcast(msg WSMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("broadcast marshal error: %v", err)
		return
	}

	b.mu.RLock()
	clients := make([]*client, 0, len(b.clients))
	for c := range b.clients {
		clients = append(clients, c)
	}
	b.mu.RUnlock()

	for _, c := range clients {
		select {
		case c.send <- data:
		default:
			// Client can't keep up, disconnect it
			log.Printf("ws client too slow, disconnecting")
			b.RemoveClient(c)
		}
	}
}

// BroadcastMessage sends an arbitrary WSMessage to all connected clients.
func (b *Broadcaster) BroadcastMessage(msg WSMessage) {
	b.broadcast(msg)
}

func (b *Broadcaster) ClientCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.clients)
}
