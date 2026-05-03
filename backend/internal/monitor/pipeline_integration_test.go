package monitor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	awsession "github.com/mrf/agentwatch/session"
	awsource "github.com/mrf/agentwatch/source"

	"github.com/agent-racer/backend/internal/session"
	"github.com/agent-racer/backend/internal/ws"
	"github.com/gorilla/websocket"
)

// TestPipelineIntegration verifies the full path: source → agentwatch monitor →
// bridge sink → local store → broadcaster → WebSocket client.
func TestPipelineIntegration(t *testing.T) {
	src := newStubSource("claude")
	mon, store, broadcaster := newTestEnv(src)
	defer broadcaster.Stop()

	now := time.Now()
	src.setHandles([]awsource.SessionHandle{
		{ID: "pipe-1", Source: "claude", WorkingDir: "/home/user/myproject", StartedAt: now},
	})
	src.setUpdate("pipe-1", awsource.SourceUpdate{
		SessionID:         "pipe-1",
		Activity:          awsession.ActivityWorking,
		Model:             "claude-opus-4-6",
		CurrentTool:       "Read",
		MessageCountDelta: 2,
		LastActivityAt:    now,
		WorkingDir:        "/home/user/myproject",
	})

	mon.poll(context.Background())

	// Verify store was populated.
	state, ok := store.Get("claude:pipe-1")
	if !ok {
		t.Fatal("session not in store after poll")
	}
	if state.Name != "myproject" {
		t.Errorf("Name = %q, want %q", state.Name, "myproject")
	}
	if state.Activity != session.ToolUse {
		t.Errorf("Activity = %q, want %q", state.Activity, session.ToolUse)
	}

	// Verify broadcaster can serve a snapshot via HTTP.
	// The handler must stay alive until the client has read the snapshot;
	// otherwise the deferred conn.Close() races with the writePump.
	handlerDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		defer func() { _ = conn.Close() }()

		_, err = broadcaster.AddClient(conn)
		if err != nil {
			return
		}
		// AddClient already sends the snapshot. Wait for the test to
		// signal it has read the message before closing the conn.
		<-handlerDone
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial failed: %v", err)
	}
	defer func() { _ = conn.Close() }()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msgBytes, err := conn.ReadMessage()
	close(handlerDone) // unblock server handler so it can close cleanly
	if err != nil {
		t.Fatalf("ws read failed: %v", err)
	}

	var msg ws.WSMessage
	if err := json.Unmarshal(msgBytes, &msg); err != nil {
		t.Fatalf("unmarshal ws message: %v", err)
	}
	if msg.Type != ws.MsgSnapshot {
		t.Errorf("message type = %q, want %q", msg.Type, ws.MsgSnapshot)
	}
}
