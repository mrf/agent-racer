package ws

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientRateLimiter(t *testing.T) {
	limiter := newClientRateLimiter(2, time.Minute, 2)
	if limiter == nil {
		t.Fatal("newClientRateLimiter returned nil")
	}

	base := time.Unix(100, 0)
	limiter.now = func() time.Time { return base }

	for i := 0; i < 2; i++ {
		if decision := limiter.Allow("127.0.0.1"); !decision.Allowed {
			t.Fatalf("Allow[%d] rejected unexpectedly", i)
		}
	}

	decision := limiter.Allow("127.0.0.1")
	if decision.Allowed {
		t.Fatal("third request should be rate limited")
	}
	if decision.RetryAfter != 30*time.Second {
		t.Fatalf("RetryAfter = %v, want %v", decision.RetryAfter, 30*time.Second)
	}

	base = base.Add(30 * time.Second)
	if decision := limiter.Allow("127.0.0.1"); !decision.Allowed {
		t.Fatal("request after refill should be allowed")
	}
}

func TestClientRateLimiter_Penalize(t *testing.T) {
	limiter := newClientRateLimiter(2, time.Minute, 10)
	if limiter == nil {
		t.Fatal("newClientRateLimiter returned nil")
	}

	base := time.Unix(100, 0)
	limiter.now = func() time.Time { return base }

	// Use 1 token normally.
	if decision := limiter.Allow("127.0.0.1"); !decision.Allowed {
		t.Fatal("first request should be allowed")
	}

	// Penalize by 5 tokens (bucket had 10, used 1 = 9, minus 5 = 4 remaining).
	limiter.Penalize("127.0.0.1", 5)

	// Should still have 4 tokens left.
	for i := 0; i < 4; i++ {
		if decision := limiter.Allow("127.0.0.1"); !decision.Allowed {
			t.Fatalf("Allow[%d] rejected after penalize, expected allowed", i)
		}
	}

	// Next should be rate-limited.
	if decision := limiter.Allow("127.0.0.1"); decision.Allowed {
		t.Fatal("request should be rate limited after penalize exhausted tokens")
	}
}

func TestClientRateLimiter_PenalizeNewClient(t *testing.T) {
	limiter := newClientRateLimiter(2, time.Minute, 10)
	if limiter == nil {
		t.Fatal("newClientRateLimiter returned nil")
	}

	base := time.Unix(100, 0)
	limiter.now = func() time.Time { return base }

	// Penalize a client that hasn't been seen yet (burst=10, penalty=8 → 2 left).
	limiter.Penalize("192.168.1.1", 8)

	// Should have 2 tokens.
	for i := 0; i < 2; i++ {
		if decision := limiter.Allow("192.168.1.1"); !decision.Allowed {
			t.Fatalf("Allow[%d] rejected, expected allowed with 2 remaining tokens", i)
		}
	}

	if decision := limiter.Allow("192.168.1.1"); decision.Allowed {
		t.Fatal("request should be rate limited")
	}
}

func TestClientRateLimiter_PenalizeNilLimiter(t *testing.T) {
	var limiter *clientRateLimiter
	// Should not panic.
	limiter.Penalize("127.0.0.1", 5)
}

func TestClientAddress(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string]string
		remote   string
		expected string
	}{
		{
			name:     "x-forwarded-for takes first value",
			headers:  map[string]string{"X-Forwarded-For": "198.51.100.7, 203.0.113.9"},
			remote:   "127.0.0.1:1234",
			expected: "198.51.100.7",
		},
		{
			name:     "x-real-ip fallback",
			headers:  map[string]string{"X-Real-IP": "203.0.113.10"},
			remote:   "127.0.0.1:1234",
			expected: "203.0.113.10",
		},
		{
			name:     "remote host without port fallback",
			remote:   "127.0.0.1:1234",
			expected: "127.0.0.1",
		},
		{
			name:     "raw remote address fallback",
			remote:   "unix-socket-client",
			expected: "unix-socket-client",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
			req.RemoteAddr = tt.remote
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			if got := clientAddress(req); got != tt.expected {
				t.Fatalf("clientAddress() = %q, want %q", got, tt.expected)
			}
		})
	}
}
