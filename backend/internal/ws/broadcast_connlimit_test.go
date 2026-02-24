package ws

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agent-racer/backend/internal/session"
	"github.com/gorilla/websocket"
)

// dialTestWS creates a test HTTP server that upgrades to WebSocket and returns
// the server-side connection. The caller must close both the server and the
// returned connection.
func dialTestWS(t *testing.T) (*httptest.Server, *websocket.Conn) {
	t.Helper()

	connCh := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		connCh <- c
	}))

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		srv.Close()
		t.Fatalf("dial: %v", err)
	}
	// We only need the server-side conn for AddClient; close the client side later.
	_ = clientConn.Close()

	select {
	case serverConn := <-connCh:
		return srv, serverConn
	case <-time.After(2 * time.Second):
		srv.Close()
		t.Fatal("timed out waiting for server-side WebSocket connection")
		return nil, nil
	}
}

func TestAddClient_MaxConnections(t *testing.T) {
	const maxConns = 2
	store := session.NewStore()
	b := NewBroadcaster(store, 100*time.Millisecond, time.Hour, maxConns)
	defer b.snapshotTicker.Stop()

	// Fill up to the limit.
	var clients []*client
	var servers []*httptest.Server
	for i := 0; i < maxConns; i++ {
		srv, conn := dialTestWS(t)
		servers = append(servers, srv)

		c, err := b.AddClient(conn)
		if err != nil {
			t.Fatalf("AddClient[%d]: unexpected error: %v", i, err)
		}
		clients = append(clients, c)
	}

	if got := b.ClientCount(); got != maxConns {
		t.Fatalf("expected %d clients, got %d", maxConns, got)
	}

	// Next connection should be rejected.
	srv, conn := dialTestWS(t)
	servers = append(servers, srv)

	_, err := b.AddClient(conn)
	if !errors.Is(err, ErrTooManyConnections) {
		t.Fatalf("expected ErrTooManyConnections, got %v", err)
	}

	if got := b.ClientCount(); got != maxConns {
		t.Fatalf("expected %d clients after rejection, got %d", maxConns, got)
	}

	// Remove one client, then adding should succeed again.
	b.RemoveClient(clients[0])

	srv2, conn2 := dialTestWS(t)
	servers = append(servers, srv2)

	_, err = b.AddClient(conn2)
	if err != nil {
		t.Fatalf("AddClient after removal: unexpected error: %v", err)
	}

	if got := b.ClientCount(); got != maxConns {
		t.Fatalf("expected %d clients after re-add, got %d", maxConns, got)
	}

	// Cleanup.
	for _, srv := range servers {
		srv.Close()
	}
}

func TestAddClient_ZeroMaxConnections_Unlimited(t *testing.T) {
	store := session.NewStore()
	b := NewBroadcaster(store, 100*time.Millisecond, time.Hour, 0)
	defer b.snapshotTicker.Stop()

	// Should be able to add many connections without rejection.
	var servers []*httptest.Server
	for i := 0; i < 10; i++ {
		srv, conn := dialTestWS(t)
		servers = append(servers, srv)

		_, err := b.AddClient(conn)
		if err != nil {
			t.Fatalf("AddClient[%d]: unexpected error with maxConns=0: %v", i, err)
		}
	}

	if got := b.ClientCount(); got != 10 {
		t.Fatalf("expected 10 clients, got %d", got)
	}

	for _, srv := range servers {
		srv.Close()
	}
}
