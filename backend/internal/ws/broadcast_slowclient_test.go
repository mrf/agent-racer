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
	for i := 0; i < cap(c.send); i++ {
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

// TestBroadcast_MultipleSlowClientsEvicted verifies that all slow clients are
// evicted in a single broadcast call, not just the first one encountered.
func TestBroadcast_MultipleSlowClientsEvicted(t *testing.T) {
	b := newTestBroadcaster(session.NewStore(), nil)

	const numSlow = 5
	slow := make([]*client, numSlow)
	for i := 0; i < numSlow; i++ {
		slow[i] = makeClient(b)
		fillSendBuffer(slow[i])
	}

	fast := makeClient(b)

	if got := b.ClientCount(); got != numSlow+1 {
		t.Fatalf("expected %d clients before broadcast, got %d", numSlow+1, got)
	}

	b.broadcast(WSMessage{Type: MsgDelta})

	if got := b.ClientCount(); got != 1 {
		t.Fatalf("expected 1 client after eviction, got %d", got)
	}

	b.mu.RLock()
	_, fastPresent := b.clients[fast]
	b.mu.RUnlock()

	if !fastPresent {
		t.Error("fast client was unexpectedly evicted")
	}
}

// TestBroadcast_EvictedClientSendChannelClosed verifies that an evicted
// client's send channel is closed (writePump uses range, so closing signals
// it to stop).
func TestBroadcast_EvictedClientSendChannelClosed(t *testing.T) {
	b := newTestBroadcaster(session.NewStore(), nil)

	slow := makeClient(b)
	fillSendBuffer(slow)

	b.broadcast(WSMessage{Type: MsgDelta})

	// Drain the buffer so we can check if the channel is closed.
	for i := 0; i < cap(slow.send); i++ {
		<-slow.send
	}

	// A receive on a closed channel returns the zero value immediately.
	val, ok := <-slow.send
	if ok {
		t.Error("expected send channel to be closed after eviction")
	}
	if val != nil {
		t.Errorf("expected nil from closed channel, got %v", val)
	}
}

// TestRemoveClient_Idempotent verifies that calling RemoveClient twice on the
// same client does not panic or corrupt state.
func TestRemoveClient_Idempotent(t *testing.T) {
	b := newTestBroadcaster(session.NewStore(), nil)

	c := makeClient(b)

	b.RemoveClient(c)
	// Second call must not panic (double-close of channel would panic).
	b.RemoveClient(c)

	if got := b.ClientCount(); got != 0 {
		t.Fatalf("expected 0 clients, got %d", got)
	}
}

// TestBroadcast_ConcurrentSendToFastClients verifies that concurrent
// broadcasts to fast clients don't race or lose messages. Slow-client
// concurrent eviction has a known race (agent-racer-ygz5) so we only test
// the non-eviction concurrent path here.
func TestBroadcast_ConcurrentSendToFastClients(t *testing.T) {
	b := newTestBroadcaster(session.NewStore(), nil)

	const numClients = 4
	clients := make([]*client, numClients)
	for i := 0; i < numClients; i++ {
		clients[i] = makeClient(b)
	}

	// Fire concurrent broadcasts; run with -race to verify no data races.
	var wg sync.WaitGroup
	const numBroadcasts = 10
	wg.Add(numBroadcasts)
	for i := 0; i < numBroadcasts; i++ {
		go func() {
			defer wg.Done()
			b.broadcast(WSMessage{Type: MsgDelta})
		}()
	}
	wg.Wait()

	// All clients should still be present (none were slow).
	if got := b.ClientCount(); got != numClients {
		t.Fatalf("expected %d clients after concurrent broadcasts, got %d", numClients, got)
	}

	// Each client should have received all broadcasts.
	for i := 0; i < numClients; i++ {
		count := len(clients[i].send)
		if count != numBroadcasts {
			t.Errorf("client[%d]: expected %d messages, got %d", i, numBroadcasts, count)
		}
	}
}
