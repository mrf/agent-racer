package monitor

import (
	"sync"
	"time"

	"github.com/agent-racer/backend/internal/ws"
)

// sourceHealth tracks failure and recovery counts for a single source.
// Used by the monitor to detect degraded/failed sources and emit WS alerts.
//
// Hysteresis: entering a bad state requires threshold consecutive failures;
// recovering back to Healthy requires threshold consecutive successes.
// A flapping source (alternating success/failure) stays in its degraded
// state until it accumulates enough consecutive successes.
//
// Fields are protected by mu because poll() writes them from the monitor
// goroutine while sourceHealthSnapshot() reads them from the broadcaster.
type sourceHealth struct {
	mu                  sync.Mutex
	discoverFailures    int  // consecutive discover failures; resets on success
	discoverSuccesses   int  // consecutive discover successes; resets on failure
	discoverInFailed    bool // sticky: true once discover crossed the failure threshold
	lastDiscoverErr     string
	lastDiscoverFail    time.Time
	parseFailures       map[string]int  // consecutive parse failures per session
	parseSuccesses      map[string]int  // consecutive parse successes per session
	parseStickyDegraded map[string]bool // sticky: true once session crossed the parse threshold
	lastParseErr        string
	lastParseFail       time.Time
	lastEmittedStatus   ws.SourceHealthStatus
	lastEmittedAt       time.Time
}

func newSourceHealth() *sourceHealth {
	return &sourceHealth{
		parseFailures:       make(map[string]int),
		parseSuccesses:      make(map[string]int),
		parseStickyDegraded: make(map[string]bool),
		lastEmittedStatus:   ws.StatusHealthy,
	}
}

func (h *sourceHealth) recordDiscoverSuccess() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.discoverFailures = 0
	h.discoverSuccesses++
	h.lastDiscoverErr = ""
}

func (h *sourceHealth) recordDiscoverFailure(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.discoverSuccesses = 0
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
	h.parseSuccesses[sessionKey]++
}

func (h *sourceHealth) recordParseFailure(sessionKey string, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.parseSuccesses[sessionKey] = 0
	h.parseFailures[sessionKey]++
	h.lastParseErr = err.Error()
	h.lastParseFail = time.Now()
}

// removeSession cleans up parse failure tracking for a removed session.
func (h *sourceHealth) removeSession(sessionKey string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.parseFailures, sessionKey)
	delete(h.parseSuccesses, sessionKey)
	delete(h.parseStickyDegraded, sessionKey)
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

// statusLocked computes health status with hysteresis:
//   - Enter Failed when discover failures reach threshold
//   - Exit Failed only after threshold consecutive successes
//   - Enter Degraded when any session's parse failures reach threshold
//   - Exit Degraded per session only after threshold consecutive successes
//
// May update discoverInFailed and parseStickyDegraded as side effects.
// Caller must hold h.mu.
func (h *sourceHealth) statusLocked(threshold int) ws.SourceHealthStatus {
	if h.discoverFailures >= threshold {
		h.discoverInFailed = true
	}
	if h.discoverInFailed && h.discoverSuccesses >= threshold {
		h.discoverInFailed = false
		h.discoverSuccesses = 0
	} else if h.discoverInFailed {
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
// the parse failure threshold and not yet recovered.
func (h *sourceHealth) degradedSessionCount(threshold int) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.degradedSessionCountLocked(threshold)
}

// degradedSessionCountLocked counts sessions in degraded state with hysteresis:
// a session exits degraded only after threshold consecutive successes.
// May update parseStickyDegraded as a side effect. Caller must hold h.mu.
func (h *sourceHealth) degradedSessionCountLocked(threshold int) int {
	// Mark sessions that just hit the failure threshold as sticky-degraded.
	for key, failures := range h.parseFailures {
		if failures >= threshold {
			h.parseStickyDegraded[key] = true
		}
	}

	count := 0
	for key := range h.parseStickyDegraded {
		if h.parseSuccesses[key] >= threshold {
			// Session has recovered: clear sticky state so it starts fresh.
			delete(h.parseStickyDegraded, key)
			delete(h.parseSuccesses, key)
			continue
		}
		count++
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
