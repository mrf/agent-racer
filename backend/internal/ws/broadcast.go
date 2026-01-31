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
	throttle       time.Duration
	snapshotTicker *time.Ticker
	pendingUpdates []*session.SessionState
	pendingRemoved []string
	flushTimer     *time.Timer
	flushMu        sync.Mutex
}

func NewBroadcaster(store *session.Store, throttle, snapshotInterval time.Duration) *Broadcaster {
	b := &Broadcaster{
		clients:  make(map[*client]bool),
		store:    store,
		throttle: throttle,
	}

	b.snapshotTicker = time.NewTicker(snapshotInterval)
	go b.snapshotLoop()

	return b
}

func (b *Broadcaster) AddClient(conn *websocket.Conn) *client {
	c := newClient(conn)

	b.mu.Lock()
	b.clients[c] = true
	b.mu.Unlock()

	snapshot := WSMessage{
		Type: MsgSnapshot,
		Payload: SnapshotPayload{
			Sessions: b.store.GetAll(),
		},
	}
	data, _ := json.Marshal(snapshot)

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

	msg := WSMessage{
		Type: MsgDelta,
		Payload: DeltaPayload{
			Updates: updates,
			Removed: removed,
		},
	}
	b.broadcast(msg)
}

func (b *Broadcaster) snapshotLoop() {
	for range b.snapshotTicker.C {
		msg := WSMessage{
			Type: MsgSnapshot,
			Payload: SnapshotPayload{
				Sessions: b.store.GetAll(),
			},
		}
		b.broadcast(msg)
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

func (b *Broadcaster) ClientCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.clients)
}
