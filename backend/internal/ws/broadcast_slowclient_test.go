package ws

import (
	"sync"
	"testing"

	"github.com/agent-racer/backend/internal/session"
)

const clientSendBufSize = 64 // matches make(chan []byte, 64) in newClient

// makeClient creates a client registered in b but with no writePump running.
func makeClient(b *Broadcaster) *client {
	c := &client{
		conn: nil,
		b:    b,
		send: make(chan []byte, clientSendBufSize),
	}
	b.mu.Lock()
	b.clients[c] = true
	b.mu.Unlock()
	return c
}

// fillSendBuffer fills c.send to capacity with placeholder bytes.
func fillSendBuffer(c *client) {
	placeholder := []byte(`{}`)
	for i := 0; i < clientSendBufSize; i++ {
		c.send <- placeholder
	}
}

// TestBroadcast_SlowClientEvicted verifies that broadcast() removes a client
// whose send channel is full.
func TestBroadcast_SlowClientEvicted(t *testing.T) {
	b := newTestBroadcaster(session.NewStore(), nil)

	slow := makeClient(b)
	fillSendBuffer(slow)

	if got := b.ClientCount(); got != 1 {
		t.Fatalf("expected 1 client before broadcast, got %d", got)
	}

	// send channel is full: this broadcast should evict the slow client.
	b.broadcast(WSMessage{Type: MsgDelta})

	if got := b.ClientCount(); got != 0 {
		t.Fatalf("expected 0 clients after eviction, got %d", got)
	}
}

// TestBroadcast_FastClientUnaffectedByEviction verifies that a healthy client
// continues to receive messages after a slow peer is evicted.
func TestBroadcast_FastClientUnaffectedByEviction(t *testing.T) {
	b := newTestBroadcaster(session.NewStore(), nil)

	slow := makeClient(b)
	fillSendBuffer(slow)

	fast := makeClient(b)

	if got := b.ClientCount(); got != 2 {
		t.Fatalf("expected 2 clients before broadcast, got %d", got)
	}

	b.broadcast(WSMessage{Type: MsgDelta})

	if got := b.ClientCount(); got != 1 {
		t.Fatalf("expected 1 client after eviction, got %d", got)
	}

	b.mu.RLock()
	_, fastPresent := b.clients[fast]
	_, slowPresent := b.clients[slow]
	b.mu.RUnlock()

	if !fastPresent {
		t.Error("fast client was unexpectedly evicted")
	}
	if slowPresent {
		t.Error("slow client was not evicted")
	}

	select {
	case msg := <-fast.send:
		if len(msg) == 0 {
			t.Error("fast client received empty message")
		}
	default:
		t.Error("fast client did not receive the broadcast message")
	}
}

// TestBroadcast_SubsequentBroadcastsSkipEvictedClient verifies that broadcasts
// after eviction do not attempt to send to the evicted client (which would
// panic on its closed channel).
func TestBroadcast_SubsequentBroadcastsSkipEvictedClient(t *testing.T) {
	b := newTestBroadcaster(session.NewStore(), nil)

	slow := makeClient(b)
	fillSendBuffer(slow)

	// Evict the slow client.
	b.broadcast(WSMessage{Type: MsgDelta})

	if got := b.ClientCount(); got != 0 {
		t.Fatalf("expected 0 clients after eviction, got %d", got)
	}

	// Subsequent broadcasts must not panic (sending to a closed channel panics).
	for i := 0; i < 5; i++ {
		b.broadcast(WSMessage{Type: MsgDelta})
	}
}

// TestBroadcast_ConcurrentBroadcastEviction fires many concurrent broadcasts
// at a client with a full send buffer. Before the fix, two goroutines could
// both see the full buffer, both call RemoveClient, and the second would
// panic sending on an already-closed channel.
func TestBroadcast_ConcurrentBroadcastEviction(t *testing.T) {
	b := newTestBroadcaster(session.NewStore(), nil)

	slow := makeClient(b)
	fillSendBuffer(slow)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.broadcast(WSMessage{Type: MsgDelta})
		}()
	}
	wg.Wait()

	if got := b.ClientCount(); got != 0 {
		t.Fatalf("expected 0 clients after concurrent eviction, got %d", got)
	}
}

// TestBroadcast_ConcurrentRemoveClient calls RemoveClient from many
// goroutines simultaneously to verify the mutex guard prevents double-close.
func TestBroadcast_ConcurrentRemoveClient(t *testing.T) {
	b := newTestBroadcaster(session.NewStore(), nil)
	c := makeClient(b)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.RemoveClient(c)
		}()
	}
	wg.Wait()

	if got := b.ClientCount(); got != 0 {
		t.Fatalf("expected 0 clients after concurrent removal, got %d", got)
	}
}
