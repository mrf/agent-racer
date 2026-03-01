package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gorilla/websocket"
)

const (
	reconnectBaseDelay = 1 * time.Second
	reconnectMaxDelay  = 30 * time.Second
	writeTimeout       = 10 * time.Second
	pongTimeout        = 60 * time.Second
	pingInterval       = 30 * time.Second
)

// WSClient manages the WebSocket connection to the Agent Racer backend.
type WSClient struct {
	url   string
	token string

	mu      sync.Mutex
	writeMu sync.Mutex // serialises all conn writes (ping, resync, auth)
	conn    *websocket.Conn
	seq     uint64
	pingCtx context.CancelFunc // cancels the active ping goroutine
}

// NewWSClient creates a client that connects to the given WebSocket URL.
func NewWSClient(url, token string) *WSClient {
	return &WSClient{url: url, token: token}
}

// --- Bubble Tea messages ---

// WSConnectedMsg is sent when the WebSocket connects.
type WSConnectedMsg struct{}

// WSDisconnectedMsg is sent when the connection drops.
type WSDisconnectedMsg struct{ Err error }

// WSSnapshotMsg delivers a full session snapshot.
type WSSnapshotMsg struct{ Payload SnapshotPayload }

// WSDeltaMsg delivers incremental session updates.
type WSDeltaMsg struct{ Payload DeltaPayload }

// WSCompletionMsg is sent when a session completes.
type WSCompletionMsg struct{ Payload CompletionPayload }

// WSEquippedMsg broadcasts a cosmetic loadout change.
type WSEquippedMsg struct{ Payload EquippedPayload }

// WSAchievementMsg is sent when an achievement unlocks.
type WSAchievementMsg struct{ Payload AchievementUnlockedPayload }

// WSSourceHealthMsg reports source health changes.
type WSSourceHealthMsg struct{ Payload SourceHealthPayload }

// WSBattlePassMsg is sent when XP is awarded.
type WSBattlePassMsg struct{ Payload BattlePassProgressPayload }

// WSErrorMsg wraps a server-side error.
type WSErrorMsg struct{ Raw json.RawMessage }

// Listen returns a Bubble Tea command that connects and dispatches messages.
// It reconnects automatically on disconnect.
func (c *WSClient) Listen(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		delay := reconnectBaseDelay
		for {
			select {
			case <-ctx.Done():
				return nil
			default:
			}

			conn, _, err := websocket.DefaultDialer.Dial(c.url, nil)
			if err != nil {
				log.Printf("ws dial error: %v (retry in %v)", err, delay)
				time.Sleep(delay)
				delay = min(delay*2, reconnectMaxDelay)
				continue
			}

			// Authenticate if token is set. No write mutex needed here
			// because the connection isn't shared yet (not stored in c.conn).
			if c.token != "" {
				auth := map[string]string{"type": "auth", "token": c.token}
				if err := conn.WriteJSON(auth); err != nil {
					conn.Close()
					continue
				}
			}

			// Cancel any previous ping goroutine.
			c.mu.Lock()
			if c.pingCtx != nil {
				c.pingCtx()
			}
			pingCtx, pingCancel := context.WithCancel(ctx)
			c.conn = conn
			c.seq = 0
			c.pingCtx = pingCancel
			c.mu.Unlock()

			// Start a single ping ticker for this connection.
			go c.pingLoop(pingCtx, conn)

			return WSConnectedMsg{}
		}
	}
}

// ReadLoop returns a Bubble Tea command that reads messages from the connection.
// It should be started after receiving WSConnectedMsg.
func (c *WSClient) ReadLoop(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		c.mu.Lock()
		conn := c.conn
		c.mu.Unlock()
		if conn == nil {
			return WSDisconnectedMsg{Err: fmt.Errorf("no connection")}
		}

		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(pongTimeout))
			return nil
		})
		conn.SetReadDeadline(time.Now().Add(pongTimeout))

		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				c.mu.Lock()
				if c.conn == conn {
					c.conn = nil
				}
				c.mu.Unlock()
				conn.Close()
				return WSDisconnectedMsg{Err: err}
			}

			var msg WSMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}

			c.mu.Lock()
			c.seq = msg.Seq
			c.mu.Unlock()

			teaMsg := c.dispatch(msg)
			if teaMsg != nil {
				return teaMsg
			}
		}
	}
}

// pingLoop sends periodic pings on the given connection. It exits when the
// context is cancelled or the connection changes.
func (c *WSClient) pingLoop(ctx context.Context, conn *websocket.Conn) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.mu.Lock()
			cc := c.conn
			c.mu.Unlock()
			if cc != conn {
				return
			}
			c.writeMu.Lock()
			conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			err := conn.WriteMessage(websocket.PingMessage, nil)
			c.writeMu.Unlock()
			if err != nil {
				return
			}
		}
	}
}

// Resync sends a resync request to the server.
func (c *WSClient) Resync() error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("not connected")
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return conn.WriteJSON(map[string]string{"type": "resync"})
}

// Seq returns the last seen sequence number.
func (c *WSClient) Seq() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.seq
}

func (c *WSClient) dispatch(msg WSMessage) tea.Msg {
	switch msg.Type {
	case MsgSnapshot:
		var p SnapshotPayload
		if json.Unmarshal(msg.Payload, &p) == nil {
			return WSSnapshotMsg{Payload: p}
		}
	case MsgDelta:
		var p DeltaPayload
		if json.Unmarshal(msg.Payload, &p) == nil {
			return WSDeltaMsg{Payload: p}
		}
	case MsgCompletion:
		var p CompletionPayload
		if json.Unmarshal(msg.Payload, &p) == nil {
			return WSCompletionMsg{Payload: p}
		}
	case MsgEquipped:
		var p EquippedPayload
		if json.Unmarshal(msg.Payload, &p) == nil {
			return WSEquippedMsg{Payload: p}
		}
	case MsgAchievementUnlocked:
		var p AchievementUnlockedPayload
		if json.Unmarshal(msg.Payload, &p) == nil {
			return WSAchievementMsg{Payload: p}
		}
	case MsgSourceHealth:
		var p SourceHealthPayload
		if json.Unmarshal(msg.Payload, &p) == nil {
			return WSSourceHealthMsg{Payload: p}
		}
	case MsgBattlePassProgress:
		var p BattlePassProgressPayload
		if json.Unmarshal(msg.Payload, &p) == nil {
			return WSBattlePassMsg{Payload: p}
		}
	case MsgError:
		return WSErrorMsg{Raw: msg.Payload}
	}
	return nil
}
