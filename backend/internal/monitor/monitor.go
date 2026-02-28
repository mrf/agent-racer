package monitor

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/agent-racer/backend/internal/config"
	"github.com/agent-racer/backend/internal/session"
	"github.com/agent-racer/backend/internal/ws"
)

// tokenSnapshot stores a token count at a point in time for burn rate calculation.
type tokenSnapshot struct {
	tokens    int
	timestamp time.Time
}

// trackedSession holds per-session state used by the monitor between polls.
type trackedSession struct {
	handle         SessionHandle
	fileOffset     int64
	lastDataTime   time.Time
	tokenSnapshots []tokenSnapshot
}

// trackingKey returns the composite key used to identify a tracked session.
// Using source:sessionID avoids collisions across different agent sources.
func trackingKey(source, sessionID string) string {
	return source + ":" + sessionID
}

// sourceFromKey extracts the source name from a composite tracking key.
// Returns empty string if the key has no separator.
func sourceFromKey(key string) string {
	if i := strings.IndexByte(key, ':'); i >= 0 {
		return key[:i]
	}
	return ""
}

type sessionEndMarker struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	Reason         string `json:"reason"`
	Timestamp      string `json:"timestamp"`
}

type Monitor struct {
	mu               sync.RWMutex               // protects cfg, sources, health
	cfg              *config.Config
	store            *session.Store
	broadcaster      *ws.Broadcaster
	sources          []Source
	tracked          map[string]*trackedSession // keyed by source:sessionID
	pendingRemoval   map[string]time.Time
	removedKeys      map[string]bool // keys removed from store; prevents re-creation while file is still discovered
	prevCPU          map[int]cpuSample
	lastProcessPoll  time.Time
	statsEvents      chan<- session.Event       // nil disables stats event emission
	statsDropped     int64                      // events dropped since last log
	statsLastDropLog time.Time                  // last time a drop was logged
	health           map[string]*sourceHealth   // keyed by source name
}

func NewMonitor(cfg *config.Config, store *session.Store, broadcaster *ws.Broadcaster, sources []Source) *Monitor {
	healthMap := make(map[string]*sourceHealth, len(sources))
	for _, src := range sources {
		healthMap[src.Name()] = newSourceHealth()
	}
	m := &Monitor{
		cfg:             cfg,
		store:           store,
		broadcaster:     broadcaster,
		sources:         sources,
		tracked:         make(map[string]*trackedSession),
		pendingRemoval:  make(map[string]time.Time),
		removedKeys:     make(map[string]bool),
		prevCPU:         make(map[int]cpuSample),
		lastProcessPoll: time.Now(),
		health:          healthMap,
	}
	broadcaster.SetHealthHook(m.sourceHealthSnapshot)
	return m
}

// SetConfig replaces the monitor's config pointer. The new config is read on
// the next poll tick. Only fields consulted during polling are affected
// (models, token normalization, monitor timings, churning thresholds).
// Server-level settings (port, host, auth) are NOT applied — those require
// a full restart.
func (m *Monitor) SetConfig(cfg *config.Config) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = cfg
}

// SetSources replaces the monitor's source list with newSources. Sources that
// are no longer present are removed; new sources are added. Existing tracked
// sessions for removed sources are left in the store (they'll age out via
// stale detection). Health tracking is updated to match.
func (m *Monitor) SetSources(newSources []Source) {
	m.mu.Lock()
	defer m.mu.Unlock()
	newHealth := make(map[string]*sourceHealth, len(newSources))
	for _, src := range newSources {
		name := src.Name()
		if existing, ok := m.health[name]; ok {
			newHealth[name] = existing
		} else {
			newHealth[name] = newSourceHealth()
		}
	}
	m.sources = newSources
	m.health = newHealth
}

// SetStatsEvents configures a channel for session lifecycle events.
// The monitor sends events on new session discovery, per-poll updates,
// and terminal state transitions. Pass nil to disable.
func (m *Monitor) SetStatsEvents(ch chan<- session.Event) {
	m.statsEvents = ch
}

// emitEvent sends a session event to the stats channel if configured.
// Uses non-blocking send to avoid stalling the monitor if the consumer
// falls behind. Dropped events are counted and logged at most once per
// 10 seconds to avoid log spam under sustained backpressure.
func (m *Monitor) emitEvent(evType session.EventType, state *session.SessionState) {
	if m.statsEvents == nil {
		return
	}
	snap := *state
	select {
	case m.statsEvents <- session.Event{
		Type:        evType,
		State:       &snap,
		ActiveCount: m.store.ActiveCount(),
	}:
	default:
		m.statsDropped++
		now := time.Now()
		if m.statsLastDropLog.IsZero() || now.Sub(m.statsLastDropLog) >= 10*time.Second {
			log.Printf("Stats events dropped: %d (channel full)", m.statsDropped)
			m.statsDropped = 0
			m.statsLastDropLog = now
		}
	}
}

func (m *Monitor) Start(ctx context.Context) {
	m.mu.RLock()
	pollInterval := m.cfg.Monitor.PollInterval
	sourceNames := make([]string, len(m.sources))
	for i, s := range m.sources {
		sourceNames[i] = s.Name()
	}
	m.mu.RUnlock()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	log.Printf("Monitor started with sources: %v", sourceNames)

	// Initial poll
	m.poll()

	for {
		select {
		case <-ctx.Done():
			log.Println("Monitor stopped")
			return
		case <-ticker.C:
			m.poll()
		}
	}
}

func (m *Monitor) poll() {
	now := time.Now()

	// Snapshot mutable fields under the read lock so that concurrent
	// SetConfig/SetSources calls from the SIGHUP goroutine don't race
	// with this poll iteration.
	m.mu.RLock()
	cfg := m.cfg
	sources := m.sources
	health := m.health
	m.mu.RUnlock()

	m.consumeSessionEndMarkers(cfg, now)

	// Collect active session keys from all sources for stale detection.
	activeKeys := make(map[string]bool)

	var updates []*session.SessionState

	for _, src := range sources {
		sh := health[src.Name()]
		handles, err := src.Discover()
		if err != nil {
			log.Printf("[%s] discovery error: %v", src.Name(), err)
			sh.recordDiscoverFailure(err)
			continue
		}
		sh.recordDiscoverSuccess()

		for _, h := range handles {
			key := trackingKey(h.Source, h.SessionID)
			activeKeys[key] = true
		}

		for _, h := range handles {
			key := trackingKey(h.Source, h.SessionID)

			ts, exists := m.tracked[key]
			if !exists {
				// Skip removed sessions when we have no prior offset to
				// distinguish new data from old. Prevents zombie re-creation.
				if m.removedKeys[key] {
					continue
				}
				ts = &trackedSession{
					handle: h,
				}
				m.tracked[key] = ts
				log.Printf("[%s] Tracking new session: %s (offset=0)", src.Name(), h.SessionID)
			}

			oldOffset := ts.fileOffset
			ts.handle.KnownSlug = m.knownSlug(key)
			ts.handle.KnownSubagentParents = m.knownSubagentParents(key)
			update, newOffset, err := src.Parse(ts.handle, ts.fileOffset)
			if err != nil {
				log.Printf("[%s] parse error for %s: %v", src.Name(), h.SessionID, err)
				sh.recordParseFailure(key, err)
				continue
			}
			sh.recordParseSuccess(key)
			ts.fileOffset = newOffset
			hasNewData := newOffset > oldOffset || update.HasData()
			if hasNewData && newOffset > oldOffset {
				log.Printf("[%s] Parsed %d new bytes from %s (offset %d→%d)", src.Name(), newOffset-oldOffset, h.LogPath, oldOffset, newOffset)
			}
			if hasNewData {
				// Use the actual timestamp from parsed data when available
				// so that old sessions discovered on startup are immediately
				// detected as stale rather than appearing active for 2 minutes.
				if !update.LastTime.IsZero() {
					ts.lastDataTime = update.LastTime
				} else {
					ts.lastDataTime = now
				}
			}

			// Always use filename-based session ID to ensure session identity
			// remains stable across model switches and JSONL sessionId changes
			sessionID := h.SessionID
			storeKey := trackingKey(h.Source, sessionID)

			// Check for resumed sessions that were already removed from store.
			if m.removedKeys[key] {
				if !hasNewData {
					continue
				}
				delete(m.removedKeys, key)
				log.Printf("[%s] Session resumed after removal: %s (newData=%d bytes)", src.Name(), h.SessionID, newOffset-oldOffset)
			}

			state, existed := m.store.Get(storeKey)
			if existed && state.IsTerminal() {
				if !hasNewData {
					continue
				}
				// New JSONL data on a terminal session — it's being resumed.
				state.CompletedAt = nil
				delete(m.pendingRemoval, storeKey)
				log.Printf("[%s] Session resumed from %s: %s (newData=%d bytes)", src.Name(), state.Activity, h.SessionID, newOffset-oldOffset)
			}

			if !existed {
				// Skip sessions that are already stale on initial discovery.
				// This prevents dead session files from briefly appearing as
				// active on server startup.
				if !update.LastTime.IsZero() && cfg.Monitor.SessionStaleAfter > 0 {
					if now.Sub(update.LastTime) > cfg.Monitor.SessionStaleAfter {
						delete(m.tracked, key)
						m.removedKeys[key] = true
						continue
					}
				}
				startedAt := h.StartedAt
				if startedAt.IsZero() {
					startedAt = now
				}
				workingDir := h.WorkingDir
				if workingDir == "" {
					workingDir = update.WorkingDir
				}
				state = &session.SessionState{
					ID:         storeKey,
					Name:       nameFromPath(workingDir),
					Source:     h.Source,
					StartedAt:  startedAt,
					WorkingDir: workingDir,
					Branch:     detectBranch(workingDir),
				}
			}

			if update.WorkingDir != "" && update.WorkingDir != state.WorkingDir {
				state.WorkingDir = update.WorkingDir
				state.Name = nameFromPath(update.WorkingDir)
				state.Branch = detectBranch(update.WorkingDir)
			}

			// Only classify activity when we have new data or a fresh session.
			// No-data polls must not overwrite with Idle — the frontend
			// derives pit transitions from lastDataReceivedAt staleness.
			if hasNewData || !existed {
				state.Activity = classifyActivityFromUpdate(update)
			}

			if update.Model != "" {
				state.Model = update.Model
			}

			if update.Slug != "" && state.Slug == "" {
				state.Slug = update.Slug
			}

			// Prefer source-reported context ceiling; fall back to config.
			maxTokens := update.MaxContextTokens
			if maxTokens == 0 {
				modelForLookup := state.Model
				if modelForLookup == "" {
					modelForLookup = "unknown"
				}
				maxTokens = cfg.MaxContextTokens(modelForLookup)
			}

			if update.LastTime.IsZero() {
				state.LastActivityAt = now
			} else {
				state.LastActivityAt = update.LastTime
			}

			if hasNewData {
				state.LastDataReceivedAt = now
			}

			// Accumulate message/tool deltas before token resolution so
			// that estimation strategies can use the updated counts.
			state.MessageCount += update.MessageCount
			state.ToolCallCount += update.ToolCalls
			state.CompactionCount += update.CompactionCount
			if update.LastTool != "" {
				state.CurrentTool = update.LastTool
			}

			mergeSubagents(state, update.Subagents)

			m.resolveTokens(cfg, state, update, maxTokens)

			// Calculate burn rate from token history
			state.BurnRatePerMinute = m.calculateBurnRate(ts, state.TokensUsed, now)

			if !existed {
				m.emitEvent(session.EventNew, state)
			} else if hasNewData {
				m.emitEvent(session.EventUpdate, state)
			}
			updates = append(updates, state)
		}
	}

	// Emit health events for sources that crossed a status threshold.
	m.maybeEmitHealthEvents(cfg, sources, health)

	// Poll process activity for churning detection.
	elapsed := now.Sub(m.lastProcessPoll)
	activities, newCPU := DiscoverProcessActivity(m.prevCPU, elapsed)
	m.prevCPU = newCPU
	m.lastProcessPoll = now

	// Build a lookup of process activity by working directory.
	activityByDir := make(map[string]ProcessActivity, len(activities))
	for _, a := range activities {
		// If multiple processes share a CWD, keep the one with higher CPU.
		if existing, ok := activityByDir[a.WorkingDir]; ok {
			if a.CPU > existing.CPU {
				activityByDir[a.WorkingDir] = a
			}
		} else {
			activityByDir[a.WorkingDir] = a
		}
	}

	// Apply churning state to non-terminal, non-waiting sessions.
	// Terminal sessions are done; waiting means blocked on user input.
	// For active states (starting, idle, thinking, tool_use) the backend
	// sets the flag and lets the frontend decide visibility -- Racer.js
	// suppresses churning visuals when thinking/tool_use animations are
	// already playing.
	cpuThreshold := cfg.Monitor.ChurningCPUThreshold
	requireNetwork := cfg.Monitor.ChurningRequiresNetwork
	for _, state := range updates {
		churning := false
		if !state.IsTerminal() && state.Activity != session.Waiting {
			if pa, ok := activityByDir[state.WorkingDir]; ok {
				churning = pa.IsChurning(cpuThreshold, requireNetwork)
				if pa.PID > 0 && state.PID == 0 {
					state.PID = pa.PID
				}
			}
		}
		state.IsChurning = churning
	}

	// Resolve tmux targets for tracked sessions (after PID population).
	resolver := NewTmuxResolver() // nil if tmux unavailable
	for _, state := range updates {
		if state.PID == 0 {
			continue
		}
		target, ok := resolver.Resolve(state.PID)
		if !ok || state.TmuxTarget == target {
			continue
		}
		state.TmuxTarget = target
	}

	// Build a lookup of this cycle's updates for the stale detection
	// loop below, avoiding store.Get reads before the atomic commit.
	updatedByKey := make(map[string]*session.SessionState, len(updates))
	for _, state := range updates {
		updatedByKey[state.ID] = state
	}

	// getSessionState returns a session from this poll's updates first,
	// then falls back to the store for sessions not touched this cycle.
	getSessionState := func(key string) (*session.SessionState, bool) {
		if s, ok := updatedByKey[key]; ok {
			return s, true
		}
		return m.store.Get(key)
	}

	// Mark stale sessions as lost (disappeared without session end marker).
	var toRemove []string
	for key, ts := range m.tracked {
		if activeKeys[key] {
			// Terminal sessions stay tracked for resume detection;
			// skip stale marking for them.
			if state, ok := getSessionState(key); ok && state.IsTerminal() {
				continue
			}
			// Still discovered and not stale by time — skip.
			isStale := cfg.Monitor.SessionStaleAfter > 0 && now.Sub(ts.lastDataTime) > cfg.Monitor.SessionStaleAfter
			if !isStale {
				continue
			}
			log.Printf("Session %s stale (lastData=%s, age=%s, threshold=%s)",
				key, ts.lastDataTime.Format("15:04:05"), now.Sub(ts.lastDataTime).Round(time.Second), cfg.Monitor.SessionStaleAfter)
		}

		if state, ok := getSessionState(key); ok {
			if state.IsTerminal() {
				// Already terminal and file disappeared — just clean up tracking.
				// Add to removedKeys so the session isn't re-created with offset 0
				// if the file briefly reappears on the next poll cycle.
				log.Printf("Cleaning up terminal session %s (activity=%s, file gone)", key, state.Activity)
				m.removedKeys[key] = true
				toRemove = append(toRemove, key)
				continue
			}
			completedAt := now
			if state.CompletedAt != nil {
				completedAt = *state.CompletedAt
			}
			reason := "stale"
			if !activeKeys[key] {
				reason = "file gone"
			}
			log.Printf("Marking session %s as lost (reason=%s, activity=%s)", key, reason, state.Activity)
			m.markTerminal(cfg, state, session.Lost, completedAt)
		}
		// Keep tracked entry (and its file offset) while the file is still
		// discovered.  Without the offset, the next poll re-parses from 0,
		// sees "new data", resumes the terminal session, and the stale
		// detector immediately marks it Lost again — creating a 1-second
		// track→lost→track loop with repeated completion events.
		if activeKeys[key] {
			continue
		}
		toRemove = append(toRemove, key)
	}
	for _, key := range toRemove {
		delete(m.tracked, key)
		// Clean up parse failure tracking for removed sessions.
		if sh, ok := health[sourceFromKey(key)]; ok {
			sh.removeSession(key)
		}
	}

	// Purge removedKeys entries for sessions whose files have fallen
	// outside the discover window.  Once the file is no longer discovered
	// there is no risk of zombie re-creation.
	for key := range m.removedKeys {
		if !activeKeys[key] {
			delete(m.removedKeys, key)
		}
	}

	// Atomically commit all session updates to the store and queue the
	// broadcast under the same lock. This prevents HTTP handlers from
	// reading partial state via store.GetAll() before WebSocket clients
	// have been notified of the changes.
	if len(updates) > 0 {
		m.store.BatchUpdateAndNotify(updates, func() {
			m.broadcaster.QueueUpdate(updates)
		})
	}

	m.flushRemovals(now)
}

// markTerminal marks a session with a terminal state (Complete, Errored, or Lost).
// The store update and broadcast are performed atomically so that HTTP readers
// cannot observe the terminal state before WebSocket clients are notified.
func (m *Monitor) markTerminal(cfg *config.Config, state *session.SessionState, activity session.Activity, completedAt time.Time) {
	if state == nil {
		return
	}
	wasTerminal := state.IsTerminal()
	state.Activity = activity
	state.CompletedAt = &completedAt
	m.store.UpdateAndNotify(state, func() {
		if !wasTerminal {
			log.Printf("Session %s terminal: %s → %s (broadcasting completion)", state.ID, state.Name, activity)
			m.broadcaster.QueueCompletion(state.ID, activity, state.Name)
			m.emitEvent(session.EventTerminal, state)
		}
		m.broadcaster.QueueUpdate([]*session.SessionState{state})
	})
	m.scheduleRemoval(cfg, state.ID, completedAt)
}

// markComplete marks a session as successfully completed.
func (m *Monitor) markComplete(cfg *config.Config, state *session.SessionState, completedAt time.Time) {
	m.markTerminal(cfg, state, session.Complete, completedAt)
}

// scheduleRemoval enqueues a session for removal after CompletionRemoveAfter.
// A zero duration removes immediately; a negative duration disables removal.
func (m *Monitor) scheduleRemoval(cfg *config.Config, sessionID string, completedAt time.Time) {
	if cfg.Monitor.CompletionRemoveAfter < 0 {
		return
	}
	removeAt := completedAt.Add(cfg.Monitor.CompletionRemoveAfter)
	if existing, ok := m.pendingRemoval[sessionID]; ok && existing.Before(removeAt) {
		return
	}
	m.pendingRemoval[sessionID] = removeAt
}

func (m *Monitor) flushRemovals(now time.Time) {
	if len(m.pendingRemoval) == 0 {
		return
	}
	var removeIDs []string
	for id, removeAt := range m.pendingRemoval {
		if !now.Before(removeAt) {
			log.Printf("Removing session %s from store (scheduled at %s)", id, removeAt.Format("15:04:05"))
			removeIDs = append(removeIDs, id)
			delete(m.pendingRemoval, id)
			m.removedKeys[id] = true
		}
	}
	if len(removeIDs) > 0 {
		m.store.BatchRemoveAndNotify(removeIDs, func() {
			m.broadcaster.QueueRemoval(removeIDs)
		})
	}
}

// consumeSessionEndMarkers handles Claude-specific SessionEnd hook markers.
// These are JSON files dropped into a directory by the Claude CLI when a
// session ends. Other sources don't use this mechanism.
func (m *Monitor) consumeSessionEndMarkers(cfg *config.Config, now time.Time) {
	dir := cfg.Monitor.SessionEndDir
	if dir == "" {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		log.Printf("Session end dir read error: %v", err)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("Session end marker read error: %v", err)
			continue
		}

		var marker sessionEndMarker
		if err := json.Unmarshal(data, &marker); err != nil {
			log.Printf("Session end marker parse error: %v", err)
			_ = os.Remove(path)
			continue
		}
		if marker.SessionID == "" {
			_ = os.Remove(path)
			continue
		}

		m.handleSessionEnd(cfg, marker, now)

		if err := os.Remove(path); err != nil {
			log.Printf("Session end marker cleanup error: %v", err)
		}
	}
}

func (m *Monitor) handleSessionEnd(cfg *config.Config, marker sessionEndMarker, now time.Time) {
	// Session end markers use the claude source prefix.
	// Try the marker's session_id first, then fall back to the filename-
	// based ID derived from the transcript path.  The monitor tracks
	// sessions by filename-based IDs so the two may differ.
	storeKey := trackingKey("claude", marker.SessionID)
	state, ok := m.store.Get(storeKey)
	if !ok && marker.TranscriptPath != "" {
		filenameID := SessionIDFromPath(marker.TranscriptPath)
		altKey := trackingKey("claude", filenameID)
		if altState, found := m.store.Get(altKey); found {
			storeKey = altKey
			state = altState
			ok = true
		}
	}

	if !ok {
		workingDir := marker.Cwd
		if workingDir == "" && marker.TranscriptPath != "" {
			workingDir = workingDirFromFile(marker.TranscriptPath)
		}
		state = &session.SessionState{
			ID:         storeKey,
			Name:       nameFromPath(workingDir),
			Source:     "claude",
			WorkingDir: workingDir,
			Branch:     detectBranch(workingDir),
			StartedAt:  now,
		}
	}

	completedAt := now
	if marker.Timestamp != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, marker.Timestamp); err == nil {
			completedAt = parsed
		}
	}

	// Determine terminal activity based on reason field
	activity := determineActivityFromReason(marker.Reason)
	m.markTerminal(cfg, state, activity, completedAt)

	// Note: tracked sessions are intentionally kept after session end to
	// maintain file offset for resume detection. They are cleaned up when
	// the file falls outside the discover window (stale detection).
}

// determineActivityFromReason inspects the reason field from a session end marker
// and returns the appropriate terminal activity (Complete, Errored, or Lost).
func determineActivityFromReason(reason string) session.Activity {
	if reason == "" {
		return session.Complete
	}

	// Check for error indicators in the reason string
	lowerReason := strings.ToLower(reason)
	errorIndicators := []string{
		"error", "err", "failed", "failure", "crash", "crashed",
		"panic", "exception", "abort", "aborted", "fatal",
		"interrupted", "killed", "terminated",
	}

	for _, indicator := range errorIndicators {
		if strings.Contains(lowerReason, indicator) {
			return session.Errored
		}
	}

	return session.Complete
}

// classifyActivityFromUpdate converts a SourceUpdate's activity string into
// the session.Activity enum.
func classifyActivityFromUpdate(update SourceUpdate) session.Activity {
	switch update.Activity {
	case "tool_use":
		return session.ToolUse
	case "thinking":
		return session.Thinking
	case "waiting":
		return session.Waiting
	default:
		if update.MessageCount == 0 && !update.HasData() {
			return session.Idle
		}
		if update.MessageCount > 0 {
			return session.Thinking
		}
		return session.Idle
	}
}

// resolveTokens applies the configured token normalization strategy for the
// session's source. For "usage" it prefers real token data and falls back to
// estimation when unavailable. For "estimate" and "message_count" it always
// derives tokens from the accumulated message count.
//
// This method sets TokensUsed, TokenEstimated, MaxContextTokens, and
// ContextUtilization on the session state.
func (m *Monitor) resolveTokens(cfg *config.Config, state *session.SessionState, update SourceUpdate, maxTokens int) {
	strategy := cfg.TokenStrategy(state.Source)
	tokensPerMsg := cfg.TokenNorm.TokensPerMessage
	if tokensPerMsg <= 0 {
		tokensPerMsg = 2000
	}

	switch strategy {
	case "usage":
		if update.TokensIn > 0 {
			// Real token data always wins. When transitioning from
			// estimated to actual, accept the real value even if lower.
			if state.TokenEstimated || update.TokensIn > state.TokensUsed {
				state.TokensUsed = update.TokensIn
				state.TokenEstimated = false
			}
		} else if state.TokenEstimated || state.TokensUsed == 0 {
			// No real data yet -- fall back to estimation.
			if state.MessageCount > 0 {
				estimated := state.MessageCount * tokensPerMsg
				if estimated > state.TokensUsed {
					state.TokensUsed = estimated
					state.TokenEstimated = true
				}
			}
		}

	case "estimate", "message_count":
		if state.MessageCount > 0 {
			estimated := state.MessageCount * tokensPerMsg
			if estimated > state.TokensUsed {
				state.TokensUsed = estimated
			}
			state.TokenEstimated = true
		}

	default:
		// Unknown strategy: use real data only, no estimation.
		if update.TokensIn > 0 && update.TokensIn > state.TokensUsed {
			state.TokensUsed = update.TokensIn
		}
	}

	state.MaxContextTokens = maxTokens
	state.UpdateUtilization()
}

const (
	burnRateWindow    = 60 * time.Second
	maxTokenSnapshots = 120
)

// calculateBurnRate computes the token consumption rate (tokens per minute)
// using a rolling window of recent token snapshots.
func (m *Monitor) calculateBurnRate(ts *trackedSession, currentTokens int, now time.Time) float64 {
	if currentTokens <= 0 {
		return 0
	}

	// Append current snapshot
	ts.tokenSnapshots = append(ts.tokenSnapshots, tokenSnapshot{
		tokens:    currentTokens,
		timestamp: now,
	})

	// Trim snapshots older than window
	cutoff := now.Add(-burnRateWindow)
	startIdx := 0
	for i, snap := range ts.tokenSnapshots {
		if snap.timestamp.After(cutoff) {
			startIdx = i
			break
		}
		startIdx = i + 1
	}
	if startIdx > 0 && startIdx < len(ts.tokenSnapshots) {
		ts.tokenSnapshots = ts.tokenSnapshots[startIdx:]
	} else if startIdx >= len(ts.tokenSnapshots) {
		ts.tokenSnapshots = ts.tokenSnapshots[:0]
	}

	// Hard cap to prevent unbounded growth if time-based trim is insufficient
	if len(ts.tokenSnapshots) > maxTokenSnapshots {
		ts.tokenSnapshots = append([]tokenSnapshot(nil), ts.tokenSnapshots[len(ts.tokenSnapshots)-maxTokenSnapshots:]...)
	}

	// Need at least 2 snapshots for rate calculation
	if len(ts.tokenSnapshots) < 2 {
		return 0
	}

	oldest := ts.tokenSnapshots[0]
	latest := ts.tokenSnapshots[len(ts.tokenSnapshots)-1]

	tokenDelta := latest.tokens - oldest.tokens
	timeDelta := latest.timestamp.Sub(oldest.timestamp)

	// Require minimum 5 seconds to avoid noisy rates
	if timeDelta.Seconds() < 5 {
		return 0
	}

	// Convert to per-minute rate
	minutes := timeDelta.Minutes()
	if minutes > 0 && tokenDelta > 0 {
		return float64(tokenDelta) / minutes
	}
	return 0
}

// healthThreshold returns the configured health warning threshold,
// falling back to 3 if unconfigured or zero.
func healthThreshold(cfg *config.Config) int {
	if t := cfg.Monitor.HealthWarningThreshold; t > 0 {
		return t
	}
	return 3
}

// maybeEmitHealthEvents checks each source's health status and emits a
// source_health WS event when the status transitions (e.g. healthy -> failed).
func (m *Monitor) maybeEmitHealthEvents(cfg *config.Config, sources []Source, health map[string]*sourceHealth) {
	threshold := healthThreshold(cfg)
	now := time.Now()
	for _, src := range sources {
		sh := health[src.Name()]
		status, discoverFailures, parseFailures, lastErr, changed := sh.snapshotAndEmit(threshold)
		if !changed {
			continue
		}
		m.broadcaster.BroadcastMessage(ws.WSMessage{
			Type: ws.MsgSourceHealth,
			Payload: ws.SourceHealthPayload{
				Source:           src.Name(),
				Status:           status,
				DiscoverFailures: discoverFailures,
				ParseFailures:    parseFailures,
				LastError:        lastErr,
				Timestamp:        now,
			},
		})
		log.Printf("[%s] health status: %s (discover=%d, parseDegraded=%d)",
			src.Name(), status, discoverFailures, parseFailures)
	}
}

// sourceHealthSnapshot builds SourceHealthPayload entries for all non-healthy
// sources. Used by the broadcaster's health hook for snapshot broadcasts.
func (m *Monitor) sourceHealthSnapshot() []ws.SourceHealthPayload {
	m.mu.RLock()
	cfg := m.cfg
	sources := m.sources
	health := m.health
	m.mu.RUnlock()

	threshold := healthThreshold(cfg)
	var result []ws.SourceHealthPayload
	now := time.Now()
	for _, src := range sources {
		sh := health[src.Name()]
		status, discoverFailures, parseFailures, lastErr := sh.snapshot(threshold)
		if status == ws.StatusHealthy {
			continue
		}
		result = append(result, ws.SourceHealthPayload{
			Source:           src.Name(),
			Status:           status,
			DiscoverFailures: discoverFailures,
			ParseFailures:    parseFailures,
			LastError:        lastErr,
			Timestamp:        now,
		})
	}
	return result
}

// mergeSubagents converts SubagentParseResults into SubagentState entries
// on the session. It merges incrementally: existing subagents are updated
// with new data, new subagents are appended, and subagents absent from the
// parsed set are pruned (unless already completed).
func mergeSubagents(state *session.SessionState, parsed map[string]*SubagentParseResult) {
	// Build index of existing subagents by ID for fast lookup.
	existing := make(map[string]int, len(state.Subagents))
	for i, sub := range state.Subagents {
		existing[sub.ID] = i
	}

	for _, pr := range parsed {
		activity := classifySubagentActivity(pr)
		tokens := 0
		if pr.LatestUsage != nil {
			tokens = pr.LatestUsage.TotalContext()
		}

		var sub *session.SubagentState

		if idx, ok := existing[pr.ID]; ok {
			// Update existing subagent.
			sub = &state.Subagents[idx]
			if pr.Slug != "" {
				sub.Slug = pr.Slug
			}
			if pr.Model != "" {
				sub.Model = pr.Model
			}
			sub.Activity = activity
			if pr.LastTool != "" {
				sub.CurrentTool = pr.LastTool
			}
			if tokens > sub.TokensUsed {
				sub.TokensUsed = tokens
			}
			sub.MessageCount += pr.MessageCount
			sub.ToolCallCount += pr.ToolCalls
			if !pr.LastTime.IsZero() {
				sub.LastActivityAt = pr.LastTime
			}
		} else {
			// Append new subagent; take a pointer to the appended element.
			state.Subagents = append(state.Subagents, session.SubagentState{
				ID:              pr.ID,
				ParentToolUseID: pr.ParentToolUseID,
				SessionID:       state.ID,
				Slug:            pr.Slug,
				Model:           pr.Model,
				Activity:        activity,
				CurrentTool:     pr.LastTool,
				TokensUsed:      tokens,
				MessageCount:    pr.MessageCount,
				ToolCallCount:   pr.ToolCalls,
				StartedAt:       pr.FirstTime,
				LastActivityAt:  pr.LastTime,
			})
			sub = &state.Subagents[len(state.Subagents)-1]
		}

		if pr.Completed {
			completedAt := pr.LastTime
			sub.CompletedAt = &completedAt
			sub.Activity = session.Complete
		}
	}

	// Prune subagents absent from the current batch. Retain subagents
	// that are completed (frontend shows final state) or that have
	// accumulated real data (MessageCount > 0) — these are genuine
	// subagents between progress entry batches, not phantoms. The
	// phantom filter in parseProgressEntry prevents fake entries from
	// accumulating messages, so only truly stale zero-message entries
	// get pruned here.
	n := 0
	for i := range state.Subagents {
		_, inParsed := parsed[state.Subagents[i].ID]
		keep := inParsed ||
			state.Subagents[i].Activity == session.Complete ||
			state.Subagents[i].MessageCount > 0
		if keep {
			state.Subagents[n] = state.Subagents[i]
			n++
		}
	}
	state.Subagents = state.Subagents[:n]
}

// knownSlug returns the session's slug from the store, or "" if unknown.
// The monitor passes this into ParseSessionJSONL so that incremental
// batches (which may contain only progress entries) can filter
// self-progress even when no non-progress entries set the slug.
func (m *Monitor) knownSlug(storeKey string) string {
	state, ok := m.store.Get(storeKey)
	if !ok {
		return ""
	}
	return state.Slug
}

// knownSubagentParents builds a parentToolUseID -> toolUseID map from the
// session's existing subagent state. This enables cross-batch completion
// detection when a tool_result arrives in a batch with no new progress entries.
// Returns nil when the session has no subagents.
func (m *Monitor) knownSubagentParents(storeKey string) map[string]string {
	state, ok := m.store.Get(storeKey)
	if !ok || len(state.Subagents) == 0 {
		return nil
	}
	parents := make(map[string]string, len(state.Subagents))
	for _, sub := range state.Subagents {
		if sub.ParentToolUseID != "" {
			parents[sub.ParentToolUseID] = sub.ID
		}
	}
	return parents
}

// classifySubagentActivity maps a SubagentParseResult's last activity string
// to a session.Activity value.
func classifySubagentActivity(pr *SubagentParseResult) session.Activity {
	switch pr.LastActivity {
	case "tool_use":
		return session.ToolUse
	case "thinking":
		return session.Thinking
	case "waiting":
		return session.Waiting
	default:
		if pr.MessageCount > 0 {
			return session.Thinking
		}
		return session.Idle
	}
}

func nameFromPath(path string) string {
	parts := splitPath(path)
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return "unknown"
}

func splitPath(path string) []string {
	var parts []string
	for path != "" && path != "/" {
		dir, file := filepath.Split(path)
		if file != "" {
			parts = append([]string{file}, parts...)
		}
		path = dir
		if path == "" || path == "/" {
			break
		}
		path = path[:len(path)-1]
	}
	return parts
}

// detectBranch runs git rev-parse in the given directory to determine
// the current branch name. Returns empty string on any error.
func detectBranch(dir string) string {
	if dir == "" {
		return ""
	}
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	branch := strings.TrimSpace(string(out))
	if branch == "HEAD" {
		return "" // detached HEAD, not useful
	}
	return branch
}

func workingDirFromFile(sessionFile string) string {
	projectDir := filepath.Base(filepath.Dir(sessionFile))
	if projectDir == "" || projectDir == "." || projectDir == "/" {
		return ""
	}
	return DecodeProjectPath(projectDir)
}
