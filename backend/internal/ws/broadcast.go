package ws

import (
	"encoding/json"
	"errors"
	"log/slog"
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
	conn   *websocket.Conn
	send   chan []byte
	b      *Broadcaster
	mu     sync.Mutex
	closed bool
}

func newClient(conn *websocket.Conn, b *Broadcaster) *client {
	c := &client{
		conn: conn,
		send: make(chan []byte, 64),
		b:    b,
	}
	go c.writePump()
	return c
}

func (c *client) writePump() {
	defer func() { _ = c.conn.Close() }()
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			c.b.RemoveClient(c)
			return
		}
	}
}

func (c *client) close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	close(c.send)
	conn := c.conn
	c.mu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}
}

// trySend attempts a non-blocking send on the client's channel.
// Returns true if the message was sent, false if the buffer was full
// or the channel was already closed.
func (c *client) trySend(data []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}
	select {
	case c.send <- data:
		return true
	default:
		return false
	}
}

type Broadcaster struct {
	mu             sync.RWMutex
	clients        map[*client]bool
	maxConns       int
	store          *session.Store
	privacy        *session.PrivacyFilter
	throttle       time.Duration
	snapshotTicker *time.Ticker
	stop           chan struct{}
	snapshotReset  chan time.Duration // signals snapshotLoop to recreate its ticker
	pendingUpdates []*session.SessionState
	pendingRemoved []string
	flushTimer     *time.Timer
	flushMu        sync.Mutex
	healthHook     func() []SourceHealthPayload
	seq            atomic.Uint64
	stopOnce       sync.Once
}

func NewBroadcaster(store *session.Store, throttle, snapshotInterval time.Duration, maxConns int) *Broadcaster {
	b := &Broadcaster{
		clients:        make(map[*client]bool),
		maxConns:       maxConns,
		store:          store,
		privacy:        &session.PrivacyFilter{},
		throttle:       throttle,
		snapshotTicker: time.NewTicker(snapshotInterval),
		stop:           make(chan struct{}),
		snapshotReset:  make(chan time.Duration, 1),
	}
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
// status for inclusion in snapshot broadcasts. Safe for concurrent use.
func (b *Broadcaster) SetHealthHook(hook func() []SourceHealthPayload) {
	b.mu.Lock()
	b.healthHook = hook
	b.mu.Unlock()
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
		_ = conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "too many connections"))
		_ = conn.Close()
		return nil, ErrTooManyConnections
	}

	c := newClient(conn, b)
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
	msg, err := NewAchievementUnlockedMessage(payload)
	if err != nil {
		slog.Error("broadcast achievement marshal failed", "error", err)
		return
	}
	b.broadcast(msg)
}

func (b *Broadcaster) BroadcastBattlePassProgress(payload BattlePassProgressPayload) {
	msg, err := NewBattlePassProgressMessage(payload)
	if err != nil {
		slog.Error("broadcast battle pass progress marshal failed", "error", err)
		return
	}
	b.broadcast(msg)
}

func (b *Broadcaster) QueueCompletion(sessionID string, activity session.Activity, name string) {
	msg, err := NewCompletionMessage(CompletionPayload{
		SessionID: sessionID,
		Activity:  activity,
		Name:      name,
	})
	if err != nil {
		slog.Error("queue completion marshal failed", "error", err)
		return
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

	allSessions := b.privacyFilter().FilterSlice(b.store.GetAll())
	msg, err := NewDeltaMessage(DeltaPayload{
		Updates: filtered,
		Removed: removed,
		Teams:   session.ComputeTeams(allSessions),
	})
	if err != nil {
		slog.Error("flush marshal failed", "error", err)
		return
	}
	b.broadcast(msg)
}

// SetConfig applies timing changes from a new config. Takes effect on the
// next queue flush (throttle) and next snapshotLoop iteration (snapshot interval).
func (b *Broadcaster) SetConfig(throttle, snapshotInterval time.Duration) {
	b.flushMu.Lock()
	b.throttle = throttle
	b.flushMu.Unlock()

	// Drain any pending reset so the latest interval wins, then send.
	select {
	case <-b.snapshotReset:
	default:
	}
	b.snapshotReset <- snapshotInterval
}

func (b *Broadcaster) snapshotLoop() {
	ticker := b.snapshotTicker
	for {
		select {
		case <-ticker.C:
			b.broadcast(b.snapshotMessage())
		case d := <-b.snapshotReset:
			ticker.Stop()
			ticker = time.NewTicker(d)
		case <-b.stop:
			ticker.Stop()
			return
		}
	}
}

// snapshotMessage builds a full snapshot WSMessage including sessions, teams,
// and source health status (when a health hook is registered).
func (b *Broadcaster) snapshotMessage() WSMessage {
	allSessions := b.privacyFilter().FilterSlice(b.store.GetAll())
	payload := SnapshotPayload{
		Sessions: allSessions,
		Teams:    session.ComputeTeams(allSessions),
	}
	b.mu.RLock()
	hook := b.healthHook
	b.mu.RUnlock()
	if hook != nil {
		payload.SourceHealth = hook()
	}
	msg, err := NewSnapshotMessage(payload)
	if err != nil {
		slog.Error("snapshot message marshal failed", "error", err)
		return WSMessage{Type: MsgSnapshot}
	}
	return msg
}

func (b *Broadcaster) broadcast(msg WSMessage) {
	msg.Seq = b.seq.Add(1)
	data, err := json.Marshal(msg)
	if err != nil {
		slog.Error("broadcast marshal failed", "error", err)
		return
	}

	b.mu.RLock()
	clients := make([]*client, 0, len(b.clients))
	for c := range b.clients {
		clients = append(clients, c)
	}
	b.mu.RUnlock()

	for _, c := range clients {
		if !c.trySend(data) {
			// Client can't keep up or already closed, disconnect it
			slog.Warn("dropping slow ws client")
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
		slog.Error("snapshot marshal failed", "error", err)
		return
	}
	c.trySend(data)
}

// BroadcastMessage sends an arbitrary WSMessage to all connected clients.
func (b *Broadcaster) BroadcastMessage(msg WSMessage) {
	b.broadcast(msg)
}

// Stop stops the snapshot loop and disconnects all active clients.
func (b *Broadcaster) Stop() {
	b.stopOnce.Do(func() {
		if b.snapshotTicker != nil {
			b.snapshotTicker.Stop()
		}

		b.mu.Lock()
		clients := make([]*client, 0, len(b.clients))
		for c := range b.clients {
			clients = append(clients, c)
		}
		b.clients = make(map[*client]bool)
		stop := b.stop
		b.mu.Unlock()

		for i := 0; i < len(clients); i++ {
			clients[i].close()
		}

		if stop != nil {
			close(stop)
		}
	})
}

func (b *Broadcaster) ClientCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.clients)
}
