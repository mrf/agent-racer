package ws

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agent-racer/backend/internal/config"
)

func TestSecurityHeaders(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	securityHeaders(inner).ServeHTTP(rec, req)

	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"X-XSS-Protection":       "1; mode=block",
		"Referrer-Policy":        "no-referrer",
	}

	for header, expected := range want {
		if got := rec.Header().Get(header); got != expected {
			t.Errorf("header %s = %q, want %q", header, got, expected)
		}
	}

	// Verify Permissions-Policy disables unused features.
	pp := rec.Header().Get("Permissions-Policy")
	if pp == "" {
		t.Fatal("Permissions-Policy header is missing")
	}
	requiredPolicies := []string{
		"camera=()",
		"microphone=()",
		"geolocation=()",
		"payment=()",
	}
	for _, policy := range requiredPolicies {
		if !strings.Contains(pp, policy) {
			t.Errorf("Permissions-Policy %q missing directive %q", pp, policy)
		}
	}

	// Verify each required CSP directive is present.
	// The connect-src directive should restrict WebSocket origins to the
	// request's Host, not blanket ws:/wss: schemes.
	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("Content-Security-Policy header is missing")
	}
	requiredDirectives := []string{
		"default-src 'self'",
		"connect-src 'self' ws://example.com wss://example.com",
		"style-src 'self'",
		"img-src 'self' data:",
		"object-src 'none'",
		"base-uri 'self'",
	}
	for _, directive := range requiredDirectives {
		if !strings.Contains(csp, directive) {
			t.Errorf("CSP %q missing directive %q", csp, directive)
		}
	}

	// Verify blanket ws:/wss: schemes are NOT present.
	if strings.Contains(csp, " ws: ") || strings.Contains(csp, " wss:;") || strings.Contains(csp, " wss: ") {
		t.Errorf("CSP should not contain blanket ws:/wss: schemes, got %q", csp)
	}
}

func newTestServer(allowedOrigins []string) *Server {
	return NewServer(&config.Config{}, nil, nil, "", false, nil, allowedOrigins, "")
}

func newTestServerWithAuth(token string) *Server {
	return NewServer(&config.Config{}, nil, nil, "", false, nil, nil, token)
}

func TestCheckOrigin(t *testing.T) {
	tests := []struct {
		name           string
		allowedOrigins []string
		origin         string
		host           string
		want           bool
	}{
		// --- With allowedOrigins configured ---
		{
			name:           "allowlist: matching origin accepted",
			allowedOrigins: []string{"http://example.com"},
			origin:         "http://example.com",
			host:           "example.com",
			want:           true,
		},
		{
			name:           "allowlist: matching host accepted",
			allowedOrigins: []string{"http://example.com:8080"},
			origin:         "https://example.com:8080",
			host:           "example.com:8080",
			want:           true,
		},
		{
			name:           "allowlist: non-matching origin rejected",
			allowedOrigins: []string{"http://example.com"},
			origin:         "http://evil.com",
			host:           "example.com",
			want:           false,
		},
		{
			name:           "allowlist: missing origin rejected",
			allowedOrigins: []string{"http://example.com"},
			origin:         "",
			host:           "example.com",
			want:           false,
		},
		{
			name:           "allowlist: localhost origin rejected when not in list",
			allowedOrigins: []string{"http://example.com"},
			origin:         "http://localhost:8080",
			host:           "example.com",
			want:           false,
		},

		// --- Without allowedOrigins (dev-only fallback) ---
		{
			name:   "no allowlist: missing origin accepted",
			origin: "",
			host:   "localhost:8080",
			want:   true,
		},
		{
			name:   "no allowlist: same host accepted",
			origin: "http://myhost:8080",
			host:   "myhost:8080",
			want:   true,
		},
		{
			name:   "no allowlist: localhost accepted",
			origin: "http://localhost:8080",
			host:   "other:8080",
			want:   true,
		},
		{
			name:   "no allowlist: 127.0.0.1 accepted",
			origin: "http://127.0.0.1:8080",
			host:   "other:8080",
			want:   true,
		},
		{
			name:   "no allowlist: [::1] accepted",
			origin: "http://[::1]:8080",
			host:   "other:8080",
			want:   true,
		},
		{
			name:   "no allowlist: external origin rejected",
			origin: "http://evil.com",
			host:   "localhost:8080",
			want:   false,
		},
		{
			name:   "no allowlist: invalid origin rejected",
			origin: "://bad",
			host:   "localhost:8080",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(tt.allowedOrigins)
			req := httptest.NewRequest(http.MethodGet, "/ws", nil)
			req.Host = tt.host
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if got := s.checkOrigin(req); got != tt.want {
				t.Errorf("checkOrigin() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHandleHealth_Liveness(t *testing.T) {
	s := newTestServer(nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	s.handleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("status = %q, want %q", resp["status"], "ok")
	}
	if _, ok := resp["uptime"]; !ok {
		t.Error("missing uptime field")
	}
}

func TestHandleHealth_MethodNotAllowed(t *testing.T) {
	s := newTestServer(nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/health", nil)
	s.handleHealth(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleHealth_ReadyNoHealthCheck(t *testing.T) {
	s := newTestServer(nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health?probe=ready", nil)
	s.handleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("status = %q, want %q", resp["status"], "ok")
	}
}

func TestHandleHealth_ReadyAllHealthy(t *testing.T) {
	s := newTestServer(nil)
	s.SetHealthCheck(func() []SourceHealthPayload {
		return nil // no unhealthy sources
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health?probe=ready", nil)
	s.handleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("status = %q, want %q", resp["status"], "ok")
	}
}

func TestHandleHealth_ReadySourceFailed(t *testing.T) {
	s := newTestServer(nil)
	s.SetHealthCheck(func() []SourceHealthPayload {
		return []SourceHealthPayload{
			{Source: "claude", Status: StatusFailed, LastError: "discover timeout"},
		}
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health?probe=ready", nil)
	s.handleHealth(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "degraded" {
		t.Errorf("status = %q, want %q", resp["status"], "degraded")
	}
	sources, ok := resp["sources"].([]interface{})
	if !ok || len(sources) != 1 {
		t.Fatalf("expected 1 source, got %v", resp["sources"])
	}
	src := sources[0].(map[string]interface{})
	if src["source"] != "claude" {
		t.Errorf("source = %q, want %q", src["source"], "claude")
	}
	if src["error"] != "discover timeout" {
		t.Errorf("error = %q, want %q", src["error"], "discover timeout")
	}
}

func TestHandleHealth_ReadySourceDegraded(t *testing.T) {
	s := newTestServer(nil)
	s.SetHealthCheck(func() []SourceHealthPayload {
		return []SourceHealthPayload{
			{Source: "codex", Status: StatusDegraded},
		}
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health?probe=ready", nil)
	s.handleHealth(rec, req)

	// Degraded sources are reported but don't trigger 503 — only Failed does.
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("status = %q, want %q", resp["status"], "ok")
	}
}

func TestAuthorize(t *testing.T) {
	tests := []struct {
		name      string
		authToken string // server token ("" = auth disabled)
		header    string // Authorization header value ("" = omit header)
		want      bool
	}{
		{
			name:      "empty server token always allows",
			authToken: "",
			header:    "",
			want:      true,
		},
		{
			name:      "empty server token allows even with header",
			authToken: "",
			header:    "Bearer something",
			want:      true,
		},
		{
			name:      "correct Bearer token allowed",
			authToken: "secret123",
			header:    "Bearer secret123",
			want:      true,
		},
		{
			name:      "missing header denied",
			authToken: "secret123",
			header:    "",
			want:      false,
		},
		{
			name:      "wrong token denied",
			authToken: "secret123",
			header:    "Bearer wrong-token",
			want:      false,
		},
		{
			name:      "non-Bearer scheme denied",
			authToken: "secret123",
			header:    "Basic secret123",
			want:      false,
		},
		{
			name:      "Bearer prefix without space denied",
			authToken: "secret123",
			header:    "Bearersecret123",
			want:      false,
		},
		{
			name:      "empty Bearer value denied",
			authToken: "secret123",
			header:    "Bearer ",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServerWithAuth(tt.authToken)
			req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			if got := s.authorize(req); got != tt.want {
				t.Errorf("authorize() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAuthorizeExported(t *testing.T) {
	s := newTestServerWithAuth("tok")

	// Valid token: exported method should delegate to authorize and allow.
	allowed := httptest.NewRequest(http.MethodGet, "/", nil)
	allowed.Header.Set("Authorization", "Bearer tok")
	if !s.Authorize(allowed) {
		t.Error("Authorize() should allow correct Bearer token")
	}

	// Missing header: exported method should delegate to authorize and deny.
	denied := httptest.NewRequest(http.MethodGet, "/", nil)
	if s.Authorize(denied) {
		t.Error("Authorize() should deny missing header")
	}
}
