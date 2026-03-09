package ws

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/agent-racer/backend/internal/config"
	"github.com/agent-racer/backend/internal/replay"
	"github.com/agent-racer/backend/internal/session"
	"github.com/gorilla/websocket"
)

func newRateLimitedServer(t *testing.T, authToken string) *Server {
	t.Helper()

	store := session.NewStore()
	broadcaster := NewBroadcaster(store, 10*time.Millisecond, time.Second, 10)
	t.Cleanup(func() {
		broadcaster.Stop()
	})

	server := NewServer(&config.Config{}, store, broadcaster, "", false, nil, nil, authToken)
	server.apiRateLimiter = newClientRateLimiter(2, time.Minute, 2)
	server.wsAuthRateLimiter = newClientRateLimiter(2, time.Minute, 2)

	return server
}

func startServer(t *testing.T, server *Server) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	server.SetupRoutes(mux)

	testServer := httptest.NewServer(mux)
	t.Cleanup(func() {
		testServer.Close()
	})

	return testServer
}

func TestSetupRoutes_RateLimitsReplayAPI(t *testing.T) {
	server := newRateLimitedServer(t, "test-token")

	replayDir := t.TempDir()
	if err := os.WriteFile(replayDir+"/session.jsonl", []byte("{\"event\":\"lap\"}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	server.SetReplayHandler(replay.NewHandler(replayDir, server.Authorize))
	testServer := startServer(t, server)

	client := testServer.Client()
	for i := 0; i < 2; i++ {
		req, err := http.NewRequest(http.MethodGet, testServer.URL+"/api/replays", nil)
		if err != nil {
			t.Fatalf("NewRequest[%d]: %v", i, err)
		}
		req.Header.Set("Authorization", "Bearer test-token")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Do[%d]: %v", i, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET /api/replays attempt %d: status %d, want %d", i, resp.StatusCode, http.StatusOK)
		}
		if err := resp.Body.Close(); err != nil {
			t.Fatalf("Body.Close[%d]: %v", i, err)
		}
	}

	req, err := http.NewRequest(http.MethodGet, testServer.URL+"/api/replays", nil)
	if err != nil {
		t.Fatalf("NewRequest[limited]: %v", err)
	}
	req.Header.Set("Authorization", "Bearer test-token")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do[limited]: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("GET /api/replays limited status %d, want %d", resp.StatusCode, http.StatusTooManyRequests)
	}
	if got := resp.Header.Get("Retry-After"); got == "" {
		t.Fatal("Retry-After header missing")
	}
}

func TestSetupRoutes_RateLimitsWebSocketAuthAttempts(t *testing.T) {
	testServer := startServer(t, newRateLimitedServer(t, "secret-token"))

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http") + "/ws"
	headers := http.Header{}

	for i := 0; i < 2; i++ {
		conn, resp, err := websocket.DefaultDialer.Dial(wsURL, headers)
		if err != nil {
			status := 0
			if resp != nil {
				status = resp.StatusCode
			}
			t.Fatalf("Dial[%d]: %v (status=%d)", i, err, status)
		}

		if err := conn.WriteJSON(wsAuthMessage{Type: "auth", Token: "secret-token"}); err != nil {
			t.Fatalf("WriteJSON[%d]: %v", i, err)
		}
		if err := conn.Close(); err != nil {
			t.Fatalf("Close[%d]: %v", i, err)
		}
	}

	_, resp, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if err == nil {
		t.Fatal("third WebSocket attempt should be rate limited")
	}
	if resp == nil {
		t.Fatal("rate-limited WebSocket dial did not return an HTTP response")
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("rate-limited WebSocket status %d, want %d", resp.StatusCode, http.StatusTooManyRequests)
	}
	if got := resp.Header.Get("Retry-After"); got == "" {
		t.Fatal("Retry-After header missing on WebSocket rate limit response")
	}
}
