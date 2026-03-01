package ws

import (
	"testing"
	"time"

	"github.com/agent-racer/backend/internal/session"
)

// TestWritePump_RemovesClientOnWriteError verifies that when writePump
// encounters a write error it calls RemoveClient so the dead client is
// removed from the broadcaster's client map.
func TestWritePump_RemovesClientOnWriteError(t *testing.T) {
	srv, serverConn := dialTestWS(t)
	defer srv.Close()

	store := session.NewStore()
	b := NewBroadcaster(store, time.Hour, time.Hour, 0)
	defer b.Stop()

	// Build a client directly so we control when writePump starts.
	c := &client{
		conn: serverConn,
		b:    b,
		send: make(chan []byte, 64),
	}
	b.mu.Lock()
	b.clients[c] = true
	b.mu.Unlock()

	if got := b.ClientCount(); got != 1 {
		t.Fatalf("expected 1 client before test, got %d", got)
	}

	// Close the connection so any write attempt will immediately fail.
	serverConn.Close()

	// Queue a message (buffered channel, non-blocking).
	c.send <- []byte(`{"type":"test"}`)

	// Start writePump now: it reads the queued message, write fails on the
	// closed connection, and should call b.RemoveClient(c).
	go c.writePump()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if b.ClientCount() == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("client not removed after write error; ClientCount = %d", b.ClientCount())
}
