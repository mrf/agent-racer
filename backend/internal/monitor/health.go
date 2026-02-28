package monitor

import (
	"sync"
	"time"

	"github.com/agent-racer/backend/internal/ws"
)

// sourceHealth tracks consecutive failure counts for a single source.
// Used by the monitor to detect degraded/failed sources and emit WS alerts.
// Fields are protected by mu because poll() writes them from the monitor
// goroutine while sourceHealthSnapshot() reads them from the broadcaster.
type sourceHealth struct {
	mu                sync.Mutex
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
	h.mu.Lock()
	defer h.mu.Unlock()
	h.discoverFailures = 0
	h.lastDiscoverErr = ""
}

func (h *sourceHealth) recordDiscoverFailure(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
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
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.parseFailures, sessionKey)
}

func (h *sourceHealth) recordParseFailure(sessionKey string, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.parseFailures[sessionKey]++
	h.lastParseErr = err.Error()
	h.lastParseFail = time.Now()
}

// removeSession cleans up parse failure tracking for a removed session.
func (h *sourceHealth) removeSession(sessionKey string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.parseFailures, sessionKey)
}

// snapshot returns a consistent copy of all health fields under the lock.
// Use this when reading from a different goroutine (e.g. broadcaster).
func (h *sourceHealth) snapshot(threshold int) (status ws.SourceHealthStatus, discoverFailures int, parseFailures int, lastErr string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	status = h.statusLocked(threshold)
	discoverFailures = h.discoverFailures
	parseFailures = h.degradedSessionCountLocked(threshold)
	lastErr = h.lastErrorLocked()
	return
}

// snapshotAndEmit returns a consistent copy of all health fields and whether
// the status changed since the last emission. If the status changed, it
// updates lastEmittedStatus. This combines snapshot + emission check in a
// single lock acquisition.
func (h *sourceHealth) snapshotAndEmit(threshold int) (status ws.SourceHealthStatus, discoverFailures int, parseFailures int, lastErr string, changed bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	status = h.statusLocked(threshold)
	changed = status != h.lastEmittedStatus
	if changed {
		h.lastEmittedStatus = status
		h.lastEmittedAt = time.Now()
	}
	discoverFailures = h.discoverFailures
	parseFailures = h.degradedSessionCountLocked(threshold)
	lastErr = h.lastErrorLocked()
	return
}

// statusLocked computes health status. Caller must hold h.mu.
func (h *sourceHealth) statusLocked(threshold int) ws.SourceHealthStatus {
	if h.discoverFailures >= threshold {
		return ws.StatusFailed
	}
	if h.degradedSessionCountLocked(threshold) > 0 {
		return ws.StatusDegraded
	}
	return ws.StatusHealthy
}

// status computes the current health status for this source.
func (h *sourceHealth) status(threshold int) ws.SourceHealthStatus {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.statusLocked(threshold)
}

// degradedSessionCount returns the number of sessions that have hit
// the parse failure threshold.
func (h *sourceHealth) degradedSessionCount(threshold int) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.degradedSessionCountLocked(threshold)
}

// degradedSessionCountLocked is the lock-free version. Caller must hold h.mu.
func (h *sourceHealth) degradedSessionCountLocked(threshold int) int {
	count := 0
	for _, failures := range h.parseFailures {
		if failures >= threshold {
			count++
		}
	}
	return count
}

// lastError returns the most recent error string (discover or parse).
func (h *sourceHealth) lastError() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.lastErrorLocked()
}

// lastErrorLocked returns the most recent error, preferring whichever
// (discover or parse) occurred more recently. Caller must hold h.mu.
func (h *sourceHealth) lastErrorLocked() string {
	if h.lastDiscoverErr != "" && (h.lastParseErr == "" || h.lastDiscoverFail.After(h.lastParseFail)) {
		return h.lastDiscoverErr
	}
	return h.lastParseErr
}
