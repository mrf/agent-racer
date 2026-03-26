package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/agent-racer/backend/internal/config"
	"github.com/agent-racer/backend/internal/gamification"
	"github.com/agent-racer/backend/internal/session"
	"github.com/agent-racer/backend/internal/ws"
)

func TestParseArgsVersionFlag(t *testing.T) {
	var stderr bytes.Buffer

	opts, err := parseArgs([]string{"--version"}, &stderr)
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if !opts.showVersion {
		t.Fatal("showVersion = false, want true")
	}
}

func TestPrintVersion(t *testing.T) {
	originalVersion := version
	version = "test-version"
	t.Cleanup(func() {
		version = originalVersion
	})

	var stdout bytes.Buffer
	printVersion(&stdout)

	if got := stdout.String(); got != "test-version\n" {
		t.Fatalf("printVersion() = %q, want %q", got, "test-version\n")
	}
}

func TestBuildSources(t *testing.T) {
	tests := []struct {
		name    string
		sources config.SourcesConfig
		want    []string
	}{
		{
			name:    "none enabled",
			sources: config.SourcesConfig{},
			want:    []string{},
		},
		{
			name:    "claude only",
			sources: config.SourcesConfig{Claude: true},
			want:    []string{"claude"},
		},
		{
			name:    "codex only",
			sources: config.SourcesConfig{Codex: true},
			want:    []string{"codex"},
		},
		{
			name:    "gemini only",
			sources: config.SourcesConfig{Gemini: true},
			want:    []string{"gemini"},
		},
		{
			name:    "claude and gemini",
			sources: config.SourcesConfig{Claude: true, Gemini: true},
			want:    []string{"claude", "gemini"},
		},
		{
			name:    "all enabled",
			sources: config.SourcesConfig{Claude: true, Codex: true, Gemini: true},
			want:    []string{"claude", "codex", "gemini"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{Sources: tt.sources}
			sources := buildSources(cfg)
			if len(sources) != len(tt.want) {
				t.Fatalf("got %d sources, want %d", len(sources), len(tt.want))
			}
			names := make(map[string]bool)
			for _, s := range sources {
				names[s.Name()] = true
			}
			for _, w := range tt.want {
				if !names[w] {
					t.Errorf("missing source %q", w)
				}
			}
		})
	}
}

// newTestStack builds the full server stack used by main() without starting a
// real listener. The stack is torn down automatically when the test finishes.
func newTestStack(t *testing.T) *http.ServeMux {
	mux, _ := newTestStackWithStore(t)
	return mux
}

func newTestStackWithStore(t *testing.T) (*http.ServeMux, *session.Store) {
	t.Helper()

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:           0,
			Host:           "127.0.0.1",
			MaxConnections: 10,
		},
		Monitor: config.MonitorConfig{
			BroadcastThrottle: 50 * time.Millisecond,
			SnapshotInterval:  10 * time.Second,
			StatsEventBuffer:  16,
		},
	}

	store := session.NewStore()
	broadcaster := ws.NewBroadcaster(store, cfg.Monitor.BroadcastThrottle, cfg.Monitor.SnapshotInterval, cfg.Server.MaxConnections)
	broadcaster.SetPrivacyFilter(cfg.Privacy.NewPrivacyFilter())
	t.Cleanup(func() { broadcaster.Stop() })

	// No auth token: all API requests are authorized.
	server := ws.NewServer(cfg, store, broadcaster, "", false, nil, nil, "")

	gamStore := gamification.NewStore(t.TempDir())
	seasonCfg := &gamification.SeasonConfig{Enabled: false}
	tracker, _, err := gamification.NewStatsTracker(gamStore, cfg.Monitor.StatsEventBuffer, seasonCfg)
	if err != nil {
		t.Fatalf("NewStatsTracker: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		tracker.Run(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	server.SetStatsTracker(tracker)

	mux := http.NewServeMux()
	server.SetupRoutes(mux)
	return mux, store
}

func TestServerWiring_SessionsEndpoint(t *testing.T) {
	mux := newTestStack(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/sessions: status %d, want %d", rec.Code, http.StatusOK)
	}

	var sessions []json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("response is not valid JSON array: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected empty session list, got %d", len(sessions))
	}
}

func TestServerWiring_ConfigEndpoint(t *testing.T) {
	mux := newTestStack(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/config: status %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
}

func TestServerWiring_SessionsEndpointRejectsNonGET(t *testing.T) {
	mux := newTestStack(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/sessions", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/sessions: status %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestServerWiring_ConfigEndpointRejectsNonGET(t *testing.T) {
	mux := newTestStack(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/config", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/config: status %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestServerWiring_TailEndpointRejectsNonGET(t *testing.T) {
	mux, store := newTestStackWithStore(t)

	logPath := t.TempDir() + "/session.jsonl"
	if err := os.WriteFile(logPath, []byte{}, 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	store.Update(&session.SessionState{
		ID:      "session-1",
		Name:    "session-1",
		LogPath: logPath,
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/session-1/tail", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/sessions/session-1/tail: status %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestServerWiring_StatsEndpoint(t *testing.T) {
	mux := newTestStack(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/stats: status %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
}

func TestServerWiring_AchievementsEndpoint(t *testing.T) {
	mux := newTestStack(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/achievements", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/achievements: status %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestServerWiring_HealthzEndpoint(t *testing.T) {
	mux := newTestStack(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /healthz: status %d, want %d", rec.Code, http.StatusOK)
	}

	var resp struct {
		Status        string  `json:"status"`
		Uptime        string  `json:"uptime"`
		UptimeSeconds float64 `json:"uptimeSeconds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("status = %q, want %q", resp.Status, "ok")
	}
	if resp.Uptime == "" {
		t.Error("uptime should not be empty")
	}
	if resp.UptimeSeconds <= 0 {
		t.Error("uptimeSeconds should be positive")
	}
}

func TestServerWiring_HealthzNoAuth(t *testing.T) {
	// /healthz must be accessible without auth (for load balancers).
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:           0,
			Host:           "127.0.0.1",
			MaxConnections: 10,
		},
		Monitor: config.MonitorConfig{
			BroadcastThrottle: 50 * time.Millisecond,
			SnapshotInterval:  10 * time.Second,
		},
	}

	store := session.NewStore()
	broadcaster := ws.NewBroadcaster(store, cfg.Monitor.BroadcastThrottle, cfg.Monitor.SnapshotInterval, cfg.Server.MaxConnections)
	t.Cleanup(func() { broadcaster.Stop() })

	server := ws.NewServer(cfg, store, broadcaster, "", false, nil, nil, "secret-token")
	mux := http.NewServeMux()
	server.SetupRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	// No Authorization header.
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /healthz without auth: status %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestServerWiring_HealthzWithSources(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:           0,
			Host:           "127.0.0.1",
			MaxConnections: 10,
		},
		Monitor: config.MonitorConfig{
			BroadcastThrottle: 50 * time.Millisecond,
			SnapshotInterval:  10 * time.Second,
		},
	}

	store := session.NewStore()
	broadcaster := ws.NewBroadcaster(store, cfg.Monitor.BroadcastThrottle, cfg.Monitor.SnapshotInterval, cfg.Server.MaxConnections)
	t.Cleanup(func() { broadcaster.Stop() })

	server := ws.NewServer(cfg, store, broadcaster, "", false, nil, nil, "")

	// Wire a health hook that returns a degraded source.
	server.SetHealthHook(func() []ws.SourceHealthPayload {
		return []ws.SourceHealthPayload{
			{
				Source:           "test-source",
				Status:           ws.StatusDegraded,
				DiscoverFailures: 0,
				ParseFailures:    3,
				LastError:        "parse timeout",
				Timestamp:        time.Now(),
			},
		}
	})

	mux := http.NewServeMux()
	server.SetupRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /healthz: status %d, want %d", rec.Code, http.StatusOK)
	}

	var resp struct {
		Status  string `json:"status"`
		Sources []struct {
			Source string `json:"source"`
			Status string `json:"status"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if resp.Status != "degraded" {
		t.Errorf("status = %q, want %q", resp.Status, "degraded")
	}
	if len(resp.Sources) != 1 {
		t.Fatalf("sources count = %d, want 1", len(resp.Sources))
	}
	if resp.Sources[0].Source != "test-source" {
		t.Errorf("source name = %q, want %q", resp.Sources[0].Source, "test-source")
	}
}

func TestServerWiring_HealthzRejectsNonGET(t *testing.T) {
	mux := newTestStack(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/healthz", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /healthz: status %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestServerWiring_AuthRequired(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:           0,
			Host:           "127.0.0.1",
			MaxConnections: 10,
		},
		Monitor: config.MonitorConfig{
			BroadcastThrottle: 50 * time.Millisecond,
			SnapshotInterval:  10 * time.Second,
		},
	}

	store := session.NewStore()
	broadcaster := ws.NewBroadcaster(store, cfg.Monitor.BroadcastThrottle, cfg.Monitor.SnapshotInterval, cfg.Server.MaxConnections)
	t.Cleanup(func() { broadcaster.Stop() })

	const token = "test-secret-token"
	server := ws.NewServer(cfg, store, broadcaster, "", false, nil, nil, token)

	mux := http.NewServeMux()
	server.SetupRoutes(mux)

	tests := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{"no token", "", http.StatusUnauthorized},
		{"valid token", "Bearer " + token, http.StatusOK},
		{"wrong token", "Bearer wrong-token", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			mux.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("status %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestListenAndServe_AcceptsConnections(t *testing.T) {
	// Find a free port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatalf("ln.Close: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	go ws.ListenAndServe("127.0.0.1", port, mux) //nolint:errcheck

	// Poll until the server is ready.
	addr := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	var resp *http.Response
	for i := 0; i < 50; i++ {
		resp, err = http.Get(addr)
		if err == nil {
			if closeErr := resp.Body.Close(); closeErr != nil {
				t.Fatalf("resp.Body.Close: %v", closeErr)
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("server did not start: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /health: status %d, want %d", resp.StatusCode, http.StatusOK)
	}
}
