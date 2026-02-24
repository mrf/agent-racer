package ws

import (
	"encoding/json"
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agent-racer/backend/internal/session"
	"github.com/gorilla/websocket"
)

// ErrTooManyConnections is returned by AddClient when the maximum number of
// concurrent WebSocket connections has been reached.
var ErrTooManyConnections = errors.New("too many WebSocket connections")

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
	maxConns       int
	store          *session.Store
	privacy        *session.PrivacyFilter
	throttle       time.Duration
	snapshotTicker *time.Ticker
	pendingUpdates []*session.SessionState
	pendingRemoved []string
	flushTimer     *time.Timer
	flushMu        sync.Mutex
	healthHook     func() []SourceHealthPayload
	seq            atomic.Uint64
}

func NewBroadcaster(store *session.Store, throttle, snapshotInterval time.Duration, maxConns int) *Broadcaster {
	b := &Broadcaster{
		clients:  make(map[*client]bool),
		maxConns: maxConns,
		store:    store,
		privacy:  &session.PrivacyFilter{},
		throttle: throttle,
	}

	b.snapshotTicker = time.NewTicker(snapshotInterval)
	go b.snapshotLoop()

	return b
}

// SetPrivacyFilter configures the privacy filter applied to all outgoing
// session data. Safe for concurrent use.
func (b *Broadcaster) SetPrivacyFilter(f *session.PrivacyFilter) {
	b.mu.Lock()
	b.privacy = f
	b.mu.Unlock()
}

// SetHealthHook registers a function that returns the current source health
// status for inclusion in snapshot broadcasts.
func (b *Broadcaster) SetHealthHook(hook func() []SourceHealthPayload) {
	b.healthHook = hook
}

// privacyFilter returns the current privacy filter under lock.
func (b *Broadcaster) privacyFilter() *session.PrivacyFilter {
	b.mu.RLock()
	f := b.privacy
	b.mu.RUnlock()
	return f
}

// FilterSessions applies the privacy filter to the given sessions, removing
// blocked sessions and masking sensitive fields.
func (b *Broadcaster) FilterSessions(sessions []*session.SessionState) []*session.SessionState {
	return b.privacyFilter().FilterSlice(sessions)
}

func (b *Broadcaster) AddClient(conn *websocket.Conn) (*client, error) {
	b.mu.Lock()
	if b.maxConns > 0 && len(b.clients) >= b.maxConns {
		b.mu.Unlock()
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "too many connections"))
		conn.Close()
		return nil, ErrTooManyConnections
	}

	c := newClient(conn)
	b.clients[c] = true
	b.mu.Unlock()

	b.SendSnapshot(c)

	return c, nil
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

	filtered := b.privacyFilter().FilterSlice(updates)
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
		Sessions: b.privacyFilter().FilterSlice(b.store.GetAll()),
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
	msg.Seq = b.seq.Add(1)
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

// SendSnapshot sends a sequenced snapshot to a single client.
func (b *Broadcaster) SendSnapshot(c *client) {
	msg := b.snapshotMessage()
	msg.Seq = b.seq.Add(1)
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("snapshot marshal error: %v", err)
		return
	}
	select {
	case c.send <- data:
	default:
	}
}

// BroadcastMessage sends an arbitrary WSMessage to all connected clients.
func (b *Broadcaster) BroadcastMessage(msg WSMessage) {
	b.broadcast(msg)
}

// Stop stops the snapshot ticker, preventing further broadcast ticks.
func (b *Broadcaster) Stop() {
	b.snapshotTicker.Stop()
}

func (b *Broadcaster) ClientCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.clients)
}
