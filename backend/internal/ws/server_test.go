package ws

import (
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
	}

	for header, expected := range want {
		if got := rec.Header().Get(header); got != expected {
			t.Errorf("header %s = %q, want %q", header, got, expected)
		}
	}

	// Verify each required CSP directive is present.
	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("Content-Security-Policy header is missing")
	}
	requiredDirectives := []string{
		"default-src 'self'",
		"connect-src 'self' ws: wss:",
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data:",
		"object-src 'none'",
		"base-uri 'self'",
	}
	for _, directive := range requiredDirectives {
		if !strings.Contains(csp, directive) {
			t.Errorf("CSP %q missing directive %q", csp, directive)
		}
	}
}

func newTestServer(allowedOrigins []string) *Server {
	return NewServer(&config.Config{}, nil, nil, "", false, nil, allowedOrigins, "")
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
