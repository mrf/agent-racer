package monitor

import (
	"time"

	"github.com/agent-racer/backend/internal/ws"
)

// sourceHealth tracks consecutive failure counts for a single source.
// Used by the monitor to detect degraded/failed sources and emit WS alerts.
type sourceHealth struct {
	discoverFailures  int
	lastDiscoverErr   string
	lastDiscoverFail  time.Time
	parseFailures     map[string]int // keyed by session tracking key
	lastParseErr      string
	lastParseFail     time.Time
	lastEmittedStatus ws.SourceHealthStatus
	lastEmittedAt     time.Time
}

func newSourceHealth() *sourceHealth {
	return &sourceHealth{
		parseFailures:     make(map[string]int),
		lastEmittedStatus: ws.StatusHealthy,
	}
}

func (h *sourceHealth) recordDiscoverSuccess() {
	h.discoverFailures = 0
	h.lastDiscoverErr = ""
}

func (h *sourceHealth) recordDiscoverFailure(err error) {
	h.discoverFailures++
	h.lastDiscoverErr = err.Error()
	h.lastDiscoverFail = time.Now()
}

// recordPanic records a recovered panic as a discover failure. Panics are
// treated as severe failures — the source is marked failed via the same
// consecutive failure counter used by Discover errors.
func (h *sourceHealth) recordPanic(err error) {
	h.recordDiscoverFailure(err)
}

func (h *sourceHealth) recordParseSuccess(sessionKey string) {
	delete(h.parseFailures, sessionKey)
}

func (h *sourceHealth) recordParseFailure(sessionKey string, err error) {
	h.parseFailures[sessionKey]++
	h.lastParseErr = err.Error()
	h.lastParseFail = time.Now()
}

// removeSession cleans up parse failure tracking for a removed session.
func (h *sourceHealth) removeSession(sessionKey string) {
	delete(h.parseFailures, sessionKey)
}

// status computes the current health status for this source.
// A source is failed if discover failures >= threshold, degraded if
// any session has parse failures >= threshold, healthy otherwise.
func (h *sourceHealth) status(threshold int) ws.SourceHealthStatus {
	if h.discoverFailures >= threshold {
		return ws.StatusFailed
	}
	if h.degradedSessionCount(threshold) > 0 {
		return ws.StatusDegraded
	}
	return ws.StatusHealthy
}

// degradedSessionCount returns the number of sessions that have hit
// the parse failure threshold.
func (h *sourceHealth) degradedSessionCount(threshold int) int {
	count := 0
	for _, failures := range h.parseFailures {
		if failures >= threshold {
			count++
		}
	}
	return count
}

// lastError returns the most recent error string (discover or parse),
// preferring whichever occurred more recently.
func (h *sourceHealth) lastError() string {
	hasDiscover := h.lastDiscoverErr != ""
	hasParse := h.lastParseErr != ""

	switch {
	case hasDiscover && !hasParse:
		return h.lastDiscoverErr
	case hasDiscover && h.lastDiscoverFail.After(h.lastParseFail):
		return h.lastDiscoverErr
	default:
		return h.lastParseErr
	}
}
