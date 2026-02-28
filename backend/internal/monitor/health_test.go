package monitor

import (
	"fmt"
	"testing"

	"github.com/agent-racer/backend/internal/ws"
)

func TestSourceHealthDiscoverFailureTracking(t *testing.T) {
	h := newSourceHealth()

	if h.status(3) != ws.StatusHealthy {
		t.Fatal("new health should be healthy")
	}

	// Accumulate failures below threshold
	h.recordDiscoverFailure(fmt.Errorf("connection refused"))
	h.recordDiscoverFailure(fmt.Errorf("timeout"))
	if h.status(3) != ws.StatusHealthy {
		t.Error("should still be healthy below threshold")
	}

	// Hit threshold
	h.recordDiscoverFailure(fmt.Errorf("still broken"))
	if h.status(3) != ws.StatusFailed {
		t.Error("should be failed at threshold")
	}
	if h.lastError() != "still broken" {
		t.Errorf("lastError = %q, want %q", h.lastError(), "still broken")
	}
}

func TestSourceHealthDiscoverRecovery(t *testing.T) {
	h := newSourceHealth()

	for i := 0; i < 5; i++ {
		h.recordDiscoverFailure(fmt.Errorf("fail %d", i))
	}
	if h.status(3) != ws.StatusFailed {
		t.Fatal("should be failed")
	}

	// Single success must NOT immediately recover — hysteresis requires threshold consecutive successes.
	h.recordDiscoverSuccess()
	if h.status(3) != ws.StatusFailed {
		t.Error("single success should not recover from failed status")
	}

	// threshold consecutive successes should recover.
	h.recordDiscoverSuccess()
	h.recordDiscoverSuccess()
	if h.status(3) != ws.StatusHealthy {
		t.Error("should recover to healthy after threshold consecutive successes")
	}
	if h.discoverFailures != 0 {
		t.Errorf("discoverFailures = %d, want 0 after recovery", h.discoverFailures)
	}
}

// TestSourceHealthDiscoverFlapping demonstrates the hysteresis fix:
// a source that alternates success/failure after crossing the failure
// threshold must remain in Failed state, not oscillate back to Healthy.
func TestSourceHealthDiscoverFlapping(t *testing.T) {
	h := newSourceHealth()
	threshold := 3

	// Cross the failure threshold.
	for i := 0; i < threshold; i++ {
		h.recordDiscoverFailure(fmt.Errorf("fail %d", i))
	}
	if h.status(threshold) != ws.StatusFailed {
		t.Fatal("should be failed after threshold failures")
	}

	// Flapping: alternating success/failure must not recover to Healthy.
	for i := 0; i < 5; i++ {
		h.recordDiscoverSuccess()
		if s := h.status(threshold); s != ws.StatusFailed {
			t.Errorf("flap iteration %d: got %s after success, want StatusFailed", i, s)
		}
		h.recordDiscoverFailure(fmt.Errorf("flap %d", i))
		if s := h.status(threshold); s != ws.StatusFailed {
			t.Errorf("flap iteration %d: got %s after failure, want StatusFailed", i, s)
		}
	}

	// Full recovery: threshold consecutive successes.
	for i := 0; i < threshold; i++ {
		h.recordDiscoverSuccess()
	}
	if s := h.status(threshold); s != ws.StatusHealthy {
		t.Errorf("got %s after consecutive successes, want StatusHealthy", s)
	}
}

func TestSourceHealthParseFailureTracking(t *testing.T) {
	h := newSourceHealth()

	// Parse failures on one session
	h.recordParseFailure("claude:sess1", fmt.Errorf("bad json"))
	h.recordParseFailure("claude:sess1", fmt.Errorf("bad json"))
	if h.status(3) != ws.StatusHealthy {
		t.Error("should be healthy below threshold")
	}
	if h.degradedSessionCount(3) != 0 {
		t.Error("no sessions should be degraded below threshold")
	}

	// Hit threshold
	h.recordParseFailure("claude:sess1", fmt.Errorf("bad json"))
	if h.status(3) != ws.StatusDegraded {
		t.Error("should be degraded at threshold")
	}
	if h.degradedSessionCount(3) != 1 {
		t.Errorf("degradedSessionCount = %d, want 1", h.degradedSessionCount(3))
	}
}

func TestSourceHealthParseRecovery(t *testing.T) {
	h := newSourceHealth()

	for i := 0; i < 5; i++ {
		h.recordParseFailure("claude:sess1", fmt.Errorf("fail"))
	}
	if h.status(3) != ws.StatusDegraded {
		t.Fatal("should be degraded")
	}

	// Single success must NOT immediately recover — hysteresis requires threshold consecutive successes.
	h.recordParseSuccess("claude:sess1")
	if h.status(3) != ws.StatusDegraded {
		t.Error("single success should not recover from degraded status")
	}

	// threshold consecutive successes should recover.
	h.recordParseSuccess("claude:sess1")
	h.recordParseSuccess("claude:sess1")
	if h.status(3) != ws.StatusHealthy {
		t.Error("should recover to healthy after threshold consecutive successes")
	}
}

// TestSourceHealthParseFlapping demonstrates the hysteresis fix for parse errors:
// a session that alternates parse success/failure after crossing the threshold
// must remain in Degraded state, not oscillate back to Healthy.
func TestSourceHealthParseFlapping(t *testing.T) {
	h := newSourceHealth()
	threshold := 3

	// Cross the parse failure threshold.
	for i := 0; i < threshold; i++ {
		h.recordParseFailure("claude:sess1", fmt.Errorf("fail %d", i))
	}
	if h.status(threshold) != ws.StatusDegraded {
		t.Fatal("should be degraded after threshold parse failures")
	}

	// Flapping: alternating success/failure must not recover to Healthy.
	for i := 0; i < 5; i++ {
		h.recordParseSuccess("claude:sess1")
		if s := h.status(threshold); s != ws.StatusDegraded {
			t.Errorf("flap iteration %d: got %s after success, want StatusDegraded", i, s)
		}
		h.recordParseFailure("claude:sess1", fmt.Errorf("flap %d", i))
		if s := h.status(threshold); s != ws.StatusDegraded {
			t.Errorf("flap iteration %d: got %s after failure, want StatusDegraded", i, s)
		}
	}

	// Full recovery: threshold consecutive successes.
	for i := 0; i < threshold; i++ {
		h.recordParseSuccess("claude:sess1")
	}
	if s := h.status(threshold); s != ws.StatusHealthy {
		t.Errorf("got %s after consecutive successes, want StatusHealthy", s)
	}
}

func TestSourceHealthMultipleSessionsParseFail(t *testing.T) {
	h := newSourceHealth()

	// Two sessions with failures at threshold
	for i := 0; i < 3; i++ {
		h.recordParseFailure("claude:sess1", fmt.Errorf("fail"))
		h.recordParseFailure("claude:sess2", fmt.Errorf("fail"))
	}
	if h.degradedSessionCount(3) != 2 {
		t.Errorf("degradedSessionCount = %d, want 2", h.degradedSessionCount(3))
	}

	// Fix one session: requires threshold consecutive successes due to hysteresis.
	for i := 0; i < 3; i++ {
		h.recordParseSuccess("claude:sess1")
	}
	if h.degradedSessionCount(3) != 1 {
		t.Errorf("degradedSessionCount after fix = %d, want 1", h.degradedSessionCount(3))
	}
	if h.status(3) != ws.StatusDegraded {
		t.Error("should still be degraded with one failing session")
	}
}

func TestSourceHealthDiscoverOverridesParse(t *testing.T) {
	h := newSourceHealth()

	// Parse failures make it degraded
	for i := 0; i < 5; i++ {
		h.recordParseFailure("claude:sess1", fmt.Errorf("fail"))
	}
	if h.status(3) != ws.StatusDegraded {
		t.Fatal("should be degraded")
	}

	// Discover failures make it failed (overrides degraded)
	for i := 0; i < 3; i++ {
		h.recordDiscoverFailure(fmt.Errorf("fail"))
	}
	if h.status(3) != ws.StatusFailed {
		t.Error("discover failure should override to failed status")
	}
}

func TestSourceHealthRemoveSession(t *testing.T) {
	h := newSourceHealth()

	for i := 0; i < 5; i++ {
		h.recordParseFailure("claude:sess1", fmt.Errorf("fail"))
	}
	if h.status(3) != ws.StatusDegraded {
		t.Fatal("should be degraded")
	}

	h.removeSession("claude:sess1")
	if h.status(3) != ws.StatusHealthy {
		t.Error("should be healthy after removing the failing session")
	}
}

func TestSourceHealthLastError(t *testing.T) {
	h := newSourceHealth()

	// No errors
	if h.lastError() != "" {
		t.Error("should have no error initially")
	}

	// Discover error only
	h.recordDiscoverFailure(fmt.Errorf("discover fail"))
	if h.lastError() != "discover fail" {
		t.Errorf("lastError = %q, want %q", h.lastError(), "discover fail")
	}

	// Parse error after discover error (parse is more recent)
	h.recordParseFailure("claude:sess1", fmt.Errorf("parse fail"))
	if h.lastError() != "parse fail" {
		t.Errorf("lastError = %q, want %q", h.lastError(), "parse fail")
	}
}
