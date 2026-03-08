package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestNewWSClient(t *testing.T) {
	c := NewWSClient("ws://localhost:8080/ws", "tok")
	if c.url != "ws://localhost:8080/ws" {
		t.Errorf("url = %q", c.url)
	}
	if c.token != "tok" {
		t.Errorf("token = %q", c.token)
	}
	if c.Seq() != 0 {
		t.Errorf("initial Seq = %d, want 0", c.Seq())
	}
}

func TestResyncNotConnected(t *testing.T) {
	c := NewWSClient("ws://localhost:9999/ws", "")
	err := c.Resync()
	if err == nil {
		t.Error("Resync should return error when not connected")
	}
}

func TestDispatchSnapshot(t *testing.T) {
	c := NewWSClient("ws://localhost/ws", "")
	payload, _ := json.Marshal(SnapshotPayload{})
	msg := WSMessage{Type: MsgSnapshot, Seq: 1, Payload: json.RawMessage(payload)}
	got := c.dispatch(msg)
	if _, ok := got.(WSSnapshotMsg); !ok {
		t.Errorf("dispatch(snapshot) = %T, want WSSnapshotMsg", got)
	}
}

func TestDispatchDelta(t *testing.T) {
	c := NewWSClient("ws://localhost/ws", "")
	payload, _ := json.Marshal(DeltaPayload{})
	msg := WSMessage{Type: MsgDelta, Seq: 2, Payload: json.RawMessage(payload)}
	got := c.dispatch(msg)
	if _, ok := got.(WSDeltaMsg); !ok {
		t.Errorf("dispatch(delta) = %T, want WSDeltaMsg", got)
	}
}

func TestDispatchCompletion(t *testing.T) {
	c := NewWSClient("ws://localhost/ws", "")
	payload, _ := json.Marshal(CompletionPayload{SessionID: "s1", Activity: ActivityComplete})
	msg := WSMessage{Type: MsgCompletion, Payload: json.RawMessage(payload)}
	got := c.dispatch(msg)
	m, ok := got.(WSCompletionMsg)
	if !ok {
		t.Fatalf("dispatch(completion) = %T, want WSCompletionMsg", got)
	}
	if m.Payload.SessionID != "s1" {
		t.Errorf("SessionID = %q, want s1", m.Payload.SessionID)
	}
}

func TestDispatchEquipped(t *testing.T) {
	c := NewWSClient("ws://localhost/ws", "")
	payload, _ := json.Marshal(EquippedPayload{Loadout: Equipped{Paint: "red"}})
	msg := WSMessage{Type: MsgEquipped, Payload: json.RawMessage(payload)}
	got := c.dispatch(msg)
	if _, ok := got.(WSEquippedMsg); !ok {
		t.Errorf("dispatch(equipped) = %T, want WSEquippedMsg", got)
	}
}

func TestDispatchAchievement(t *testing.T) {
	c := NewWSClient("ws://localhost/ws", "")
	payload, _ := json.Marshal(AchievementUnlockedPayload{ID: "a1", Name: "First Lap"})
	msg := WSMessage{Type: MsgAchievementUnlocked, Payload: json.RawMessage(payload)}
	got := c.dispatch(msg)
	m, ok := got.(WSAchievementMsg)
	if !ok {
		t.Fatalf("dispatch(achievement) = %T, want WSAchievementMsg", got)
	}
	if m.Payload.ID != "a1" {
		t.Errorf("ID = %q, want a1", m.Payload.ID)
	}
}

func TestDispatchSourceHealth(t *testing.T) {
	c := NewWSClient("ws://localhost/ws", "")
	payload, _ := json.Marshal(SourceHealthPayload{Source: "claude", Status: StatusHealthy, Timestamp: time.Now()})
	msg := WSMessage{Type: MsgSourceHealth, Payload: json.RawMessage(payload)}
	got := c.dispatch(msg)
	if _, ok := got.(WSSourceHealthMsg); !ok {
		t.Errorf("dispatch(source_health) = %T, want WSSourceHealthMsg", got)
	}
}

func TestDispatchBattlePass(t *testing.T) {
	c := NewWSClient("ws://localhost/ws", "")
	payload, _ := json.Marshal(BattlePassProgressPayload{XP: 100, Tier: 3})
	msg := WSMessage{Type: MsgBattlePassProgress, Payload: json.RawMessage(payload)}
	got := c.dispatch(msg)
	m, ok := got.(WSBattlePassMsg)
	if !ok {
		t.Fatalf("dispatch(battlepass) = %T, want WSBattlePassMsg", got)
	}
	if m.Payload.XP != 100 {
		t.Errorf("XP = %d, want 100", m.Payload.XP)
	}
}

func TestDispatchError(t *testing.T) {
	c := NewWSClient("ws://localhost/ws", "")
	msg := WSMessage{Type: MsgError, Payload: json.RawMessage(`"something went wrong"`)}
	got := c.dispatch(msg)
	if _, ok := got.(WSErrorMsg); !ok {
		t.Errorf("dispatch(error) = %T, want WSErrorMsg", got)
	}
}

func TestDispatchUnknown(t *testing.T) {
	c := NewWSClient("ws://localhost/ws", "")
	msg := WSMessage{Type: "unknown_type", Payload: json.RawMessage(`{}`)}
	got := c.dispatch(msg)
	if got != nil {
		t.Errorf("dispatch(unknown) = %v, want nil", got)
	}
}

func TestDispatchInvalidPayload(t *testing.T) {
	c := NewWSClient("ws://localhost/ws", "")
	// A snapshot message with invalid JSON payload should return nil.
	msg := WSMessage{Type: MsgSnapshot, Payload: json.RawMessage(`not-json`)}
	got := c.dispatch(msg)
	if got != nil {
		t.Errorf("dispatch(invalid payload) = %v, want nil", got)
	}
}

// wsUpgrader is used to create test WebSocket servers.
var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// makeSnapshot returns a serialized WSMessage wrapping an empty snapshot payload.
func makeSnapshot(seq uint64) []byte {
	payload, _ := json.Marshal(SnapshotPayload{})
	msg, _ := json.Marshal(WSMessage{
		Type:    MsgSnapshot,
		Seq:     seq,
		Payload: json.RawMessage(payload),
	})
	return msg
}

// TestWSConnectAndDisconnect verifies that Listen returns WSConnectedMsg on
// success and ReadLoop returns WSDisconnectedMsg when the server closes.
func TestWSConnectAndDisconnect(t *testing.T) {
	connReady := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		connReady <- conn
		conn.ReadMessage() //nolint:errcheck
		_ = conn.Close()
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	c := NewWSClient(wsURL, "")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Connect.
	listenCmd := c.Listen(ctx)
	msg := listenCmd()
	if _, ok := msg.(WSConnectedMsg); !ok {
		t.Fatalf("Listen returned %T, want WSConnectedMsg", msg)
	}

	// Close the server-side connection to trigger a disconnect.
	srvConn := <-connReady
	if err := srvConn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye")); err != nil {
		t.Fatalf("srvConn.WriteMessage: %v", err)
	}

	// ReadLoop should detect the close and return WSDisconnectedMsg.
	readCmd := c.ReadLoop(ctx)
	msg2 := readCmd()
	if _, ok := msg2.(WSDisconnectedMsg); !ok {
		t.Fatalf("ReadLoop returned %T after server close, want WSDisconnectedMsg", msg2)
	}
}

// TestWSReconnectOnDisconnect verifies the client can reconnect after a drop.
// Listen is called twice, each time establishing a fresh connection to the server.
func TestWSReconnectOnDisconnect(t *testing.T) {
	connCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		connCount++
		if err := conn.WriteMessage(websocket.TextMessage, makeSnapshot(uint64(connCount))); err != nil {
			t.Errorf("conn.WriteMessage snapshot: %v", err)
		}
		if err := conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done")); err != nil {
			t.Errorf("conn.WriteMessage close: %v", err)
		}
		_ = conn.Close()
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	c := NewWSClient(wsURL, "")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// First Listen → connects to server (connection #1).
	msg1 := c.Listen(ctx)()
	if _, ok := msg1.(WSConnectedMsg); !ok {
		t.Fatalf("first Listen returned %T, want WSConnectedMsg", msg1)
	}
	if connCount != 1 {
		t.Errorf("after first Listen: connCount = %d, want 1", connCount)
	}

	// Second Listen simulates the reconnect path triggered by WSDisconnectedMsg.
	msg2 := c.Listen(ctx)()
	if _, ok := msg2.(WSConnectedMsg); !ok {
		t.Fatalf("second Listen returned %T, want WSConnectedMsg", msg2)
	}
	if connCount != 2 {
		t.Errorf("after second Listen: connCount = %d, want 2", connCount)
	}
}
