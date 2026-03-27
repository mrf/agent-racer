package monitor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agent-racer/backend/internal/config"
	"github.com/agent-racer/backend/internal/session"
	"github.com/agent-racer/backend/internal/ws"
	"github.com/gorilla/websocket"
)

// stubSource is a minimal Source for integration testing. It returns
// pre-configured handles and updates without reading files on disk.
type stubSource struct {
	name    string
	handles []SessionHandle
	updates map[string]SourceUpdate // sessionID -> update (consumed on first Parse)
}

func (s *stubSource) Name() string { return s.name }

func (s *stubSource) Discover() ([]SessionHandle, error) {
	return s.handles, nil
}

func (s *stubSource) Parse(handle SessionHandle, offset int64) (SourceUpdate, int64, error) {
	if u, ok := s.updates[handle.SessionID]; ok {
		delete(s.updates, handle.SessionID)
		return u, offset + 1, nil
	}
	return SourceUpdate{}, offset, nil
}

// pipelineEnv holds the shared infrastructure for pipeline integration tests:
// a store, broadcaster, monitor, HTTP test server, and a WebSocket upgrader.
type pipelineEnv struct {
	store       *session.Store
	broadcaster *ws.Broadcaster
	mon         *Monitor
	srv         *httptest.Server
}

// newPipelineEnv wires up store -> broadcaster -> monitor -> HTTP/WebSocket
// test server. The caller must defer env.cleanup().
func newPipelineEnv(t *testing.T, src Source) *pipelineEnv {
	t.Helper()

	store := session.NewStore()
	broadcaster := ws.NewBroadcaster(store, 10*time.Millisecond, 1*time.Hour, 0)

	cfg := &config.Config{
		Monitor: config.MonitorConfig{
			PollInterval:          1 * time.Second,
			SessionStaleAfter:     2 * time.Minute,
			CompletionRemoveAfter: -1, // disable auto-removal
		},
	}
	mon := NewMonitor(cfg, store, broadcaster, []Source{src})
	mon.discoverProcessActivity = nil
	mon.newTmuxResolver = nil

	upgrader := websocket.Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		_, _ = broadcaster.AddClient(conn)
	}))

	t.Cleanup(func() {
		srv.Close()
		broadcaster.Stop()
	})

	return &pipelineEnv{
		store:       store,
		broadcaster: broadcaster,
		mon:         mon,
		srv:         srv,
	}
}

// dialWS connects a WebSocket client to the test server. The connection is
// closed automatically when the test finishes.
func (env *pipelineEnv) dialWS(t *testing.T) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(env.srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// readWSMessage reads and unmarshals a single WebSocket message within the
// given deadline. Fails the test on timeout or read error.
func readWSMessage(t *testing.T, conn *websocket.Conn, deadline time.Duration) ws.WSMessage {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(deadline)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read ws message: %v", err)
	}
	var msg ws.WSMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal ws message: %v", err)
	}
	return msg
}

// readDelta reads the next WebSocket message and unmarshals it as a delta
// payload. Fails the test if the message type is not MsgDelta or if the
// payload contains no session updates.
func readDelta(t *testing.T, conn *websocket.Conn) (ws.WSMessage, ws.DeltaPayload) {
	t.Helper()
	msg := readWSMessage(t, conn, 2*time.Second)
	if msg.Type != ws.MsgDelta {
		t.Fatalf("expected %s, got %s", ws.MsgDelta, msg.Type)
	}
	var payload ws.DeltaPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		t.Fatalf("unmarshal delta payload: %v", err)
	}
	if len(payload.Updates) == 0 {
		t.Fatal("delta payload has no session updates")
	}
	return msg, payload
}

// TestPipelineIntegration exercises the full data path:
//
//	monitor.poll() -> store.BatchUpdateAndNotify -> broadcaster.QueueUpdate -> flush -> WebSocket client
//
// It verifies that a session discovered by a stub Source flows through the
// store and broadcaster and arrives at a connected WebSocket client as a
// properly-formed delta message with the expected fields.
func TestPipelineIntegration(t *testing.T) {
	now := time.Now()

	src := &stubSource{
		name: "test",
		handles: []SessionHandle{{
			SessionID: "sess-001",
			LogPath:   "/fake/path.jsonl",
			Source:    "test",
			StartedAt: now,
		}},
		updates: map[string]SourceUpdate{
			"sess-001": {
				SessionID:    "sess-001",
				Model:        "claude-opus-4-5",
				TokensIn:     5000,
				TokensOut:    500,
				MessageCount: 3,
				ToolCalls:    2,
				LastTool:     "Read",
				Activity:     "tool_use",
				LastTime:     now,
			},
		},
	}

	env := newPipelineEnv(t, src)
	conn := env.dialWS(t)

	// Read the initial snapshot. The store is empty so it should have zero sessions.
	snapshotMsg := readWSMessage(t, conn, 2*time.Second)
	if snapshotMsg.Type != ws.MsgSnapshot {
		t.Fatalf("first message: expected %s, got %s", ws.MsgSnapshot, snapshotMsg.Type)
	}
	var snapPayload ws.SnapshotPayload
	if err := json.Unmarshal(snapshotMsg.Payload, &snapPayload); err != nil {
		t.Fatalf("unmarshal snapshot payload: %v", err)
	}
	if len(snapPayload.Sessions) != 0 {
		t.Errorf("initial snapshot: expected 0 sessions, got %d", len(snapPayload.Sessions))
	}

	// Trigger the poll: source -> monitor -> store -> broadcaster.
	env.mon.poll()

	deltaMsg, deltaPayload := readDelta(t, conn)

	// Verify session fields propagated through the pipeline.
	s := deltaPayload.Updates[0]
	wantID := "test:sess-001"
	if s.ID != wantID {
		t.Errorf("session ID = %q, want %q", s.ID, wantID)
	}
	if s.Source != "test" {
		t.Errorf("source = %q, want %q", s.Source, "test")
	}
	if s.Model != "claude-opus-4-5" {
		t.Errorf("model = %q, want %q", s.Model, "claude-opus-4-5")
	}
	if s.Activity != session.ToolUse {
		t.Errorf("activity = %v, want %v", s.Activity, session.ToolUse)
	}
	if s.MessageCount != 3 {
		t.Errorf("message count = %d, want 3", s.MessageCount)
	}
	if s.ToolCallCount != 2 {
		t.Errorf("tool call count = %d, want 2", s.ToolCallCount)
	}
	if s.CurrentTool != "Read" {
		t.Errorf("current tool = %q, want %q", s.CurrentTool, "Read")
	}

	// Sequence numbers must increase monotonically.
	if deltaMsg.Seq <= snapshotMsg.Seq {
		t.Errorf("delta seq %d should be > snapshot seq %d", deltaMsg.Seq, snapshotMsg.Seq)
	}

	// Store should also reflect the session.
	stored, ok := env.store.Get(wantID)
	if !ok {
		t.Fatalf("session %q not found in store", wantID)
	}
	if stored.Model != "claude-opus-4-5" {
		t.Errorf("store model = %q, want %q", stored.Model, "claude-opus-4-5")
	}
}

// TestPipelineIntegration_MultipleClients verifies that a delta broadcast
// reaches all connected WebSocket clients.
func TestPipelineIntegration_MultipleClients(t *testing.T) {
	now := time.Now()

	src := &stubSource{
		name: "multi",
		handles: []SessionHandle{{
			SessionID: "sess-multi",
			LogPath:   "/fake/multi.jsonl",
			Source:    "multi",
			StartedAt: now,
		}},
		updates: map[string]SourceUpdate{
			"sess-multi": {
				SessionID:    "sess-multi",
				Model:        "gemini-2.0",
				MessageCount: 1,
				Activity:     "thinking",
				LastTime:     now,
			},
		},
	}

	env := newPipelineEnv(t, src)

	conn1 := env.dialWS(t)
	conn2 := env.dialWS(t)

	// Drain initial snapshots.
	readWSMessage(t, conn1, 2*time.Second)
	readWSMessage(t, conn2, 2*time.Second)

	env.mon.poll()

	// Both clients should receive the delta.
	_, p1 := readDelta(t, conn1)
	_, p2 := readDelta(t, conn2)

	if p1.Updates[0].ID != p2.Updates[0].ID {
		t.Errorf("session IDs differ: %q vs %q", p1.Updates[0].ID, p2.Updates[0].ID)
	}
}

// TestPipelineIntegration_PrivacyFilter verifies that the privacy filter
// masks session data before it reaches WebSocket clients.
func TestPipelineIntegration_PrivacyFilter(t *testing.T) {
	now := time.Now()

	src := &stubSource{
		name: "private",
		handles: []SessionHandle{{
			SessionID:  "sess-priv",
			LogPath:    "/fake/priv.jsonl",
			Source:     "private",
			WorkingDir: "/home/user/secret-project",
			StartedAt:  now,
		}},
		updates: map[string]SourceUpdate{
			"sess-priv": {
				SessionID:    "sess-priv",
				Model:        "claude-opus-4-5",
				MessageCount: 1,
				Activity:     "thinking",
				LastTime:     now,
				WorkingDir:   "/home/user/secret-project",
			},
		},
	}

	env := newPipelineEnv(t, src)

	// Enable working directory masking.
	env.broadcaster.SetPrivacyFilter(&session.PrivacyFilter{
		MaskWorkingDirs: true,
	})

	conn := env.dialWS(t)

	// Drain initial snapshot.
	readWSMessage(t, conn, 2*time.Second)

	env.mon.poll()

	_, payload := readDelta(t, conn)

	// Working dir should be masked to only the last path component.
	if payload.Updates[0].WorkingDir != "secret-project" {
		t.Errorf("working dir = %q, want %q", payload.Updates[0].WorkingDir, "secret-project")
	}
}
