package ws

import (
	"errors"
	"testing"
	"time"

	"github.com/agent-racer/backend/internal/session"
	"github.com/gorilla/websocket"
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
	_ = serverConn.Close()

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

// TestWritePump_WriteDeadlinePreventsHang verifies that writePump sets a write
// deadline so a stalled client connection doesn't block the goroutine forever.
// We shorten writeWait, then saturate TCP buffers without reading from the
// client side. The writePump must hit the deadline and remove the client.
func TestWritePump_WriteDeadlinePreventsHang(t *testing.T) {
	orig := writeWait
	writeWait = 100 * time.Millisecond
	defer func() { writeWait = orig }()

	srv, clientConn, serverConn := dialTestWSPair(t)
	defer srv.Close()
	defer func() { _ = clientConn.Close() }()

	store := session.NewStore()
	b := NewBroadcaster(store, time.Hour, time.Hour, 0)
	defer b.Stop()

	c := &client{
		conn: serverConn,
		b:    b,
		send: make(chan []byte, 64),
	}
	b.mu.Lock()
	b.clients[c] = true
	b.mu.Unlock()

	// Start writePump — it will try to write to the server-side conn.
	go c.writePump()

	// Fill the send channel with large messages. Without reading on the
	// client side, TCP buffers fill up and WriteMessage blocks until the
	// write deadline fires.
	bigMsg := make([]byte, 64*1024)
	for i := 0; i < 64; i++ {
		if !c.trySend(bigMsg) {
			break
		}
	}

	// The writePump should hit the write deadline and remove the client
	// well within 3 seconds (writeWait is only 100ms).
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if b.ClientCount() == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("writePump did not exit after write deadline; goroutine may be leaked")
}

func TestStop_DisconnectsActiveClients(t *testing.T) {
	srv, clientConn, serverConn := dialTestWSPair(t)
	defer srv.Close()
	defer func() { _ = clientConn.Close() }()

	store := session.NewStore()
	b := NewBroadcaster(store, time.Hour, time.Hour, 0)

	c, err := b.AddClient(serverConn)
	if err != nil {
		t.Fatalf("AddClient unexpected error: %v", err)
	}

	if got := b.ClientCount(); got != 1 {
		t.Fatalf("expected 1 client before Stop, got %d", got)
	}

	b.Stop()
	b.Stop() // idempotent

	if got := b.ClientCount(); got != 0 {
		t.Fatalf("expected 0 clients after Stop, got %d", got)
	}
	if c.trySend([]byte(`{"type":"test"}`)) {
		t.Fatal("expected trySend to fail after Stop")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_ = clientConn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		_, _, err := clientConn.ReadMessage()
		if err == nil {
			continue
		}

		var closeErr *websocket.CloseError
		if errors.As(err, &closeErr) &&
			closeErr.Code != websocket.CloseNormalClosure &&
			closeErr.Code != websocket.CloseGoingAway &&
			closeErr.Code != websocket.CloseAbnormalClosure {
			t.Fatalf("unexpected close code: %d", closeErr.Code)
		}
		return
	}

	t.Fatal("client connection remained open after Stop")
}
