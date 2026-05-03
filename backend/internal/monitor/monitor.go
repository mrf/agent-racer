package monitor

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	awsession "github.com/mrf/agentwatch/session"
	awsource "github.com/mrf/agentwatch/source"

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
	handle         awsource.SessionHandle
	cursor         awsource.Cursor
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

const defaultTmuxResolverTTL = 5 * time.Second
const defaultProcessActivityInterval = 5 * time.Second

// SnapshotHook is called after each poll with the current snapshot of all sessions.
// It is called synchronously from the poll goroutine; implementations must not block.
type SnapshotHook func([]*session.SessionState)

type Monitor struct {
	mu                      sync.RWMutex // protects cfg, sources, health
	cfg                     *config.Config
	store                   *session.Store
	broadcaster             *ws.Broadcaster
	sources                 []awsource.Source
	tracked                 map[string]*trackedSession // keyed by source:sessionID
	pendingRemoval          map[string]time.Time
	removedKeys             map[string]bool // keys removed from store; prevents re-creation while file is still discovered
	prevCPU                 map[int]cpuSample
	lastProcessPoll         time.Time
	processActivity         map[string]ProcessActivity
	statsEvents             chan<- session.Event     // nil disables stats event emission
	statsDropped            int64                    // events dropped since last log
	statsLastDropLog        time.Time                // last time a drop was logged
	health                  map[string]*sourceHealth // keyed by source name
	reconfigureCh           chan struct{}             // signals Start() to recreate its poll ticker
	snapshotHook            SnapshotHook             // optional hook called after each poll
	discoverProcessActivity func(map[int]cpuSample, time.Duration) ([]ProcessActivity, map[int]cpuSample)
	processPollInterval     time.Duration
	newTmuxResolver         func() *TmuxResolver // injectable for tests
	tmuxResolverTTL         time.Duration        // cache TTL; <=0 disables cache
	tmuxResolver            *TmuxResolver        // cached resolver (nil means tmux unavailable)
	tmuxResolverNext        time.Time            // next refresh time for cached resolver
	tmuxResolverSet         bool                 // true after first resolver attempt
}

func NewMonitor(cfg *config.Config, store *session.Store, broadcaster *ws.Broadcaster, sources []awsource.Source) *Monitor {
	healthMap := make(map[string]*sourceHealth, len(sources))
	for _, src := range sources {
		healthMap[src.Name()] = newSourceHealth()
	}
	m := &Monitor{
		cfg:                     cfg,
		store:                   store,
		broadcaster:             broadcaster,
		sources:                 sources,
		tracked:                 make(map[string]*trackedSession),
		pendingRemoval:          make(map[string]time.Time),
		removedKeys:             make(map[string]bool),
		prevCPU:                 make(map[int]cpuSample),
		processActivity:         make(map[string]ProcessActivity),
		discoverProcessActivity: DiscoverProcessActivity,
		processPollInterval:     defaultProcessActivityInterval,
		health:                  healthMap,
		reconfigureCh:           make(chan struct{}, 1),
		newTmuxResolver:         NewTmuxResolver,
		tmuxResolverTTL:         defaultTmuxResolverTTL,
	}
	broadcaster.SetHealthHook(m.SourceHealthSnapshot)
	return m
}

// SetConfig replaces the monitor's config pointer. The new config is read on
// the next poll tick. Only fields consulted during polling are affected
// (models, token normalization, monitor timings, churning thresholds).
// Server-level settings (port, host, auth) are NOT applied — those require
// a full restart.
//
// If PollInterval changed, SetConfig signals the Start() goroutine to
// recreate its ticker so the new interval takes effect immediately.
func (m *Monitor) SetConfig(cfg *config.Config) {
	m.mu.Lock()
	m.cfg = cfg
	m.mu.Unlock()

	// Signal Start() to recreate its ticker with the updated poll interval.
	// Non-blocking: if a signal is already pending, the new config will be
	// picked up when that pending signal is processed.
	select {
	case m.reconfigureCh <- struct{}{}:
	default:
	}
}

// SetSources replaces the monitor's source list with newSources. Sources that
// are no longer present are removed; new sources are added. Existing tracked
// sessions for removed sources are left in the store (they'll age out via
// stale detection). Health tracking is updated to match.
func (m *Monitor) SetSources(newSources []awsource.Source) {
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

// SetSnapshotHook registers a function to be called after each poll with a
// snapshot of all current sessions. Pass nil to disable. The hook is called
// synchronously; it must not block.
func (m *Monitor) SetSnapshotHook(fn SnapshotHook) {
	m.snapshotHook = fn
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
			slog.Warn("stats events dropped", "count", m.statsDropped)
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

	slog.Info("monitor started", "sources", sourceNames)

	// Initial poll
	m.pollCtx(ctx)

	for {
		select {
		case <-ctx.Done():
			slog.Info("monitor stopped")
			return
		case <-m.reconfigureCh:
			// PollInterval may have changed — recreate the ticker.
			ticker.Stop()
			m.mu.RLock()
			newInterval := m.cfg.Monitor.PollInterval
			m.mu.RUnlock()
			ticker = time.NewTicker(newInterval)
			slog.Info("monitor poll interval updated", "interval", newInterval)
		case <-ticker.C:
			m.pollCtx(ctx)
		}
	}
}

// poll runs a single poll iteration with a background context.
// Used by tests that call m.poll() directly.
func (m *Monitor) poll() {
	m.pollCtx(context.Background())
}

func (m *Monitor) pollCtx(ctx context.Context) {
	now := time.Now()

	// Snapshot mutable fields under the read lock so that concurrent
	// SetConfig/SetSources calls from the SIGHUP goroutine don't race
	// with this poll iteration.
	m.mu.RLock()
	cfg := m.cfg
	sources := m.sources
	health := m.health
	m.mu.RUnlock()

	// Collect active session keys from all sources for stale detection.
	activeKeys := make(map[string]bool)

	var updates []*session.SessionState

	for _, src := range sources {
		srcUpdates, srcActiveKeys := m.pollSource(ctx, src, cfg, health[src.Name()], now)
		for k := range srcActiveKeys {
			activeKeys[k] = true
		}
		updates = append(updates, srcUpdates...)
	}

	// Emit health events for sources that crossed a status threshold.
	m.maybeEmitHealthEvents(cfg, sources, health)

	activityByDir := m.refreshProcessActivity(now)

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
	needsTmuxResolve := false
	for _, state := range updates {
		if state.PID > 0 {
			needsTmuxResolve = true
			break
		}
	}
	if needsTmuxResolve {
		resolver := m.cachedTmuxResolver(now)
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
			slog.Info("session stale", "session", key, "lastData", ts.lastDataTime.Format("15:04:05"), "age", now.Sub(ts.lastDataTime).Round(time.Second), "threshold", cfg.Monitor.SessionStaleAfter)
		}

		if state, ok := getSessionState(key); ok {
			if state.IsTerminal() {
				// Already terminal and file disappeared — just clean up tracking.
				// Add to removedKeys so the session isn't re-created with cursor ""
				// if the file briefly reappears on the next poll cycle.
				slog.Debug("cleaning up terminal session", "session", key, "activity", state.Activity)
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
			slog.Info("marking session as lost", "session", key, "reason", reason, "activity", state.Activity)
			m.markTerminal(cfg, state, session.Lost, completedAt)
		}
		// Keep tracked entry (and its cursor) while the file is still
		// discovered.  Without the cursor, the next poll re-parses from
		// the start, sees "new data", resumes the terminal session, and
		// the stale detector immediately marks it Lost again — creating a
		// 1-second track→lost→track loop with repeated completion events.
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

	// Compute racing positions and atomically commit all session updates.
	// The notify callback runs after the write lock is released to avoid
	// lock inversion with the broadcaster.
	if len(updates) > 0 {
		m.updatePositions(updates)
		m.store.BatchUpdateAndNotify(updates, func() {
			m.broadcaster.QueueUpdate(updates)
		})
	}

	m.flushRemovals(now)

	if m.snapshotHook != nil {
		m.snapshotHook(m.store.GetAll())
	}
}

func (m *Monitor) refreshProcessActivity(now time.Time) map[string]ProcessActivity {
	if m.discoverProcessActivity == nil {
		return m.processActivity
	}
	if m.processPollInterval > 0 && !m.lastProcessPoll.IsZero() && now.Sub(m.lastProcessPoll) < m.processPollInterval {
		return m.processActivity
	}

	elapsed := time.Duration(0)
	if !m.lastProcessPoll.IsZero() {
		elapsed = now.Sub(m.lastProcessPoll)
	}
	activities, newCPU := m.discoverProcessActivity(m.prevCPU, elapsed)
	m.prevCPU = newCPU
	m.lastProcessPoll = now

	activityByDir := make(map[string]ProcessActivity, len(activities))
	for _, a := range activities {
		// If multiple processes share a CWD, keep the one with higher CPU.
		if existing, ok := activityByDir[a.WorkingDir]; ok {
			if a.CPU > existing.CPU {
				activityByDir[a.WorkingDir] = a
			}
			continue
		}
		activityByDir[a.WorkingDir] = a
	}
	m.processActivity = activityByDir
	return m.processActivity
}

func (m *Monitor) cachedTmuxResolver(now time.Time) *TmuxResolver {
	if m.newTmuxResolver == nil {
		return nil
	}
	if m.tmuxResolverTTL <= 0 {
		return m.newTmuxResolver()
	}
	if m.tmuxResolverSet && now.Before(m.tmuxResolverNext) {
		return m.tmuxResolver
	}
	m.tmuxResolver = m.newTmuxResolver()
	m.tmuxResolverNext = now.Add(m.tmuxResolverTTL)
	m.tmuxResolverSet = true
	return m.tmuxResolver
}

// pollSource processes a single source within a deferred recover, so that a
// panic in Discover or Parse does not crash the entire server. Returns the
// session updates and the set of active tracking keys discovered.
func (m *Monitor) pollSource(ctx context.Context, src awsource.Source, cfg *config.Config, sh *sourceHealth, now time.Time) (updates []*session.SessionState, activeKeys map[string]bool) {
	activeKeys = make(map[string]bool)

	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			slog.Error("panic recovered in poll", "source", src.Name(), "error", r, "stack", string(buf[:n]))
			sh.recordPanic(fmt.Errorf("panic: %v", r))
		}
	}()

	handles, err := src.Discover(ctx)
	if err != nil {
		slog.Warn("discovery error", "source", src.Name(), "error", err)
		sh.recordDiscoverFailure(err)
		return nil, activeKeys
	}
	sh.recordDiscoverSuccess()

	for _, h := range handles {
		key := trackingKey(h.Source, h.ID)
		activeKeys[key] = true
	}

	for _, h := range handles {
		key := trackingKey(h.Source, h.ID)

		ts, exists := m.tracked[key]
		if !exists {
			// Skip removed sessions when we have no prior cursor to
			// distinguish new data from old. Prevents zombie re-creation.
			if m.removedKeys[key] {
				continue
			}
			ts = &trackedSession{
				handle: h,
			}
			m.tracked[key] = ts
			slog.Debug("tracking new session", "source", src.Name(), "session", h.ID)
		}

		oldCursor := ts.cursor
		update, newCursor, err := src.Parse(ctx, ts.handle, ts.cursor)
		if err != nil {
			slog.Warn("parse error", "source", src.Name(), "session", h.ID, "error", err)
			sh.recordParseFailure(key, err)
			continue
		}
		sh.recordParseSuccess(key)
		ts.cursor = newCursor
		hasNewData := string(newCursor) != string(oldCursor) || sourceUpdateHasData(update)
		if update.WorkingDir != "" && ts.handle.WorkingDir == "" {
			ts.handle.WorkingDir = update.WorkingDir
		}
		if hasNewData && string(newCursor) != string(oldCursor) {
			slog.Debug("parsed new data", "source", src.Name(), "path", h.Path)
		}
		if hasNewData {
			// Use the actual timestamp from parsed data when available
			// so that old sessions discovered on startup are immediately
			// detected as stale rather than appearing active for 2 minutes.
			if !update.LastActivityAt.IsZero() {
				ts.lastDataTime = update.LastActivityAt
			} else {
				ts.lastDataTime = now
			}
		}

		// Check for resumed sessions that were already removed from store.
		if m.removedKeys[key] {
			if !hasNewData {
				continue
			}
			delete(m.removedKeys, key)
			slog.Info("session resumed after removal", "source", src.Name(), "session", h.ID)
		}

		state, existed := m.store.Get(key)
		if existed && state.IsTerminal() {
			if !hasNewData {
				continue
			}
			// New data on a terminal session — it's being resumed.
			state.CompletedAt = nil
			state.Subagents = nil // Reset stale subagent state to prevent double-counting.
			delete(m.pendingRemoval, key)
			slog.Info("session resumed", "source", src.Name(), "from", state.Activity, "session", h.ID)
		}

		if !existed {
			// Skip sessions that are already stale on initial discovery.
			// Keep the tracked cursor so a resumed session can reappear
			// without re-reading the whole file from the start.
			if !update.LastActivityAt.IsZero() && cfg.Monitor.SessionStaleAfter > 0 {
				if now.Sub(update.LastActivityAt) > cfg.Monitor.SessionStaleAfter {
					m.removedKeys[key] = true
					slog.Debug("suppressing stale session on initial discovery", "source", src.Name(), "session", h.ID, "lastData", update.LastActivityAt.Format(time.RFC3339Nano))
					continue
				}
			}
			startedAt := h.StartedAt
			if startedAt.IsZero() {
				startedAt = now
			}
			workingDir := h.WorkingDir
			if workingDir == "" {
				workingDir = ts.handle.WorkingDir
			}
			if workingDir == "" {
				workingDir = update.WorkingDir
			}
			state = &session.SessionState{
				ID:         key,
				Name:       nameFromPath(workingDir),
				Source:     h.Source,
				StartedAt:  startedAt,
				WorkingDir: workingDir,
				Branch:     detectBranch(workingDir),
				LogPath:    h.Path,
			}
		}

		if h.Path != "" && h.Path != state.LogPath {
			state.LogPath = h.Path
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

		if update.LastActivityAt.IsZero() {
			state.LastActivityAt = now
		} else {
			state.LastActivityAt = update.LastActivityAt
		}

		if hasNewData {
			state.LastDataReceivedAt = now
		}

		// Accumulate message/tool deltas before token resolution so
		// that estimation strategies can use the updated counts.
		state.MessageCount += update.MessageCountDelta
		state.ToolCallCount += update.ToolCallCountDelta
		if update.CurrentTool != "" {
			state.CurrentTool = update.CurrentTool
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

		// Handle terminal updates from the source (e.g. session-end markers
		// consumed by the claude source internally).
		if update.Terminal {
			completedAt := update.EndedAt
			if completedAt.IsZero() {
				completedAt = now
			}
			activity := determineActivityFromReason(update.EndReason)
			m.markTerminal(cfg, state, activity, completedAt)
		}
	}

	return updates, activeKeys
}

// determineActivityFromReason inspects the reason field and returns the
// appropriate terminal activity (Complete, Errored, or Lost).
func determineActivityFromReason(reason string) session.Activity {
	if reason == "" {
		return session.Complete
	}

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

// markTerminal marks a session with a terminal state (Complete, Errored, or Lost).
// The store update is atomic; the broadcast is queued after the lock is released.
func (m *Monitor) markTerminal(cfg *config.Config, state *session.SessionState, activity session.Activity, completedAt time.Time) {
	if state == nil {
		return
	}
	wasTerminal := state.IsTerminal()
	state.Activity = activity
	state.CompletedAt = &completedAt
	m.store.UpdateAndNotify(state, func() {
		if !wasTerminal {
			slog.Info("session terminal", "session", state.ID, "name", state.Name, "activity", activity)
			m.broadcaster.QueueCompletion(state.ID, activity, state.Name)
		}
		m.broadcaster.QueueUpdate([]*session.SessionState{state})
	})
	if !wasTerminal {
		m.emitEvent(session.EventTerminal, state)
	}
	m.scheduleRemoval(cfg, state.ID, completedAt)
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
			slog.Debug("removing session from store", "session", id, "scheduledAt", removeAt.Format("15:04:05"))
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

// sourceUpdateHasData reports whether u contains any meaningful data.
func sourceUpdateHasData(u awsource.SourceUpdate) bool {
	return u.SessionID != "" ||
		u.Model != "" ||
		u.ContextTokens > 0 ||
		u.OutputTokens > 0 ||
		u.MessageCountDelta > 0 ||
		u.ToolCallCountDelta > 0 ||
		u.CurrentTool != "" ||
		u.Activity != "" ||
		!u.LastActivityAt.IsZero() ||
		u.WorkingDir != "" ||
		u.Branch != "" ||
		u.MaxContextTokens > 0 ||
		len(u.Subagents) > 0 ||
		u.Terminal ||
		u.EndReason != "" ||
		u.TokenEstimated
}

// classifyActivityFromUpdate converts a SourceUpdate's activity into the
// local session.Activity enum.
func classifyActivityFromUpdate(update awsource.SourceUpdate) session.Activity {
	switch update.Activity {
	case awsession.ActivityWorking:
		if update.CurrentTool != "" {
			return session.ToolUse
		}
		return session.Thinking
	case awsession.ActivityWaiting:
		return session.Waiting
	default: // ActivityIdle or empty
		if !sourceUpdateHasData(update) {
			return session.Idle
		}
		if update.MessageCountDelta > 0 {
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
func (m *Monitor) resolveTokens(cfg *config.Config, state *session.SessionState, update awsource.SourceUpdate, maxTokens int) {
	strategy := cfg.TokenStrategy(state.Source)
	tokensPerMsg := cfg.TokenNorm.TokensPerMessage
	if tokensPerMsg <= 0 {
		tokensPerMsg = 2000
	}

	switch strategy {
	case "usage":
		if update.ContextTokens > 0 {
			// Real token data always wins. When transitioning from
			// estimated to actual, accept the real value even if lower.
			if state.TokenEstimated || update.ContextTokens > state.TokensUsed {
				state.TokensUsed = update.ContextTokens
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
		if update.ContextTokens > 0 && update.ContextTokens > state.TokensUsed {
			state.TokensUsed = update.ContextTokens
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
	for i := 0; i < len(ts.tokenSnapshots); i++ {
		if ts.tokenSnapshots[i].timestamp.After(cutoff) {
			startIdx = i
			break
		}
		startIdx = i + 1
	}
	if startIdx > 0 {
		ts.tokenSnapshots = ts.tokenSnapshots[startIdx:]
	}

	// Hard cap to prevent unbounded growth if time-based trim is insufficient
	if len(ts.tokenSnapshots) > maxTokenSnapshots {
		ts.tokenSnapshots = ts.tokenSnapshots[len(ts.tokenSnapshots)-maxTokenSnapshots:]
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
func (m *Monitor) maybeEmitHealthEvents(cfg *config.Config, sources []awsource.Source, health map[string]*sourceHealth) {
	threshold := healthThreshold(cfg)
	now := time.Now()
	for _, src := range sources {
		sh := health[src.Name()]
		status, discoverFailures, parseFailures, lastErr, changed := sh.snapshotAndEmit(threshold)
		if !changed {
			continue
		}
		msg, err := ws.NewSourceHealthMessage(ws.SourceHealthPayload{
			Source:           src.Name(),
			Status:           status,
			DiscoverFailures: discoverFailures,
			ParseFailures:    parseFailures,
			LastError:        sanitizeHealthError(lastErr),
			Timestamp:        now,
		})
		if err != nil {
			slog.Error("source health marshal failed", "source", src.Name(), "error", err)
			continue
		}
		m.broadcaster.BroadcastMessage(msg)
		slog.Info("health status changed", "source", src.Name(), "status", status, "discoverFailures", discoverFailures, "parseFailures", parseFailures)
	}
}

// SourceHealthSnapshot builds SourceHealthPayload entries for all non-healthy
// sources. Used by the broadcaster's health hook for snapshot broadcasts
// and the /healthz HTTP endpoint.
func (m *Monitor) SourceHealthSnapshot() []ws.SourceHealthPayload {
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
			LastError:        sanitizeHealthError(lastErr),
			Timestamp:        now,
		})
	}
	return result
}

// mergeSubagents merges slim agentwatch SubagentState entries into the
// session's richer local SubagentState list. Existing subagents are updated,
// new ones are appended, and absent ones are pruned (unless completed).
func mergeSubagents(state *session.SessionState, parsed []awsession.SubagentState) {
	// Build index of existing subagents by ID for fast lookup.
	existing := make(map[string]int, len(state.Subagents))
	for i, sub := range state.Subagents {
		existing[sub.ID] = i
	}

	inParsed := make(map[string]bool, len(parsed))
	for _, pr := range parsed {
		inParsed[pr.ID] = true
		activity := convertSubagentActivity(pr.Activity)

		if idx, ok := existing[pr.ID]; ok {
			sub := &state.Subagents[idx]
			sub.Activity = activity
			if pr.CurrentTool != "" {
				sub.CurrentTool = pr.CurrentTool
			}
			if !pr.LastActivityAt.IsZero() {
				sub.LastActivityAt = pr.LastActivityAt
			}
		} else {
			state.Subagents = append(state.Subagents, session.SubagentState{
				ID:              pr.ID,
				ParentToolUseID: pr.ParentID,
				SessionID:       state.ID,
				Activity:        activity,
				CurrentTool:     pr.CurrentTool,
				StartedAt:       pr.StartedAt,
				LastActivityAt:  pr.LastActivityAt,
			})
			// Update existing map so the terminal block below can find
			// the just-appended entry without a separate code path.
			existing[pr.ID] = len(state.Subagents) - 1
		}

		if pr.Activity == awsession.ActivityTerminal {
			sub := &state.Subagents[existing[pr.ID]]
			if sub.CompletedAt == nil {
				completedAt := pr.LastActivityAt
				sub.CompletedAt = &completedAt
				sub.Activity = session.Complete
			}
		}
	}

	// Prune absent subagents (keep completed ones and those with real data).
	n := 0
	for i := range state.Subagents {
		keep := inParsed[state.Subagents[i].ID] ||
			state.Subagents[i].Activity == session.Complete ||
			state.Subagents[i].MessageCount > 0
		if keep {
			state.Subagents[n] = state.Subagents[i]
			n++
		}
	}
	state.Subagents = state.Subagents[:n]
}

// convertSubagentActivity maps an agentwatch session.Activity to the local
// session.Activity enum used by agent-racer.
func convertSubagentActivity(a awsession.Activity) session.Activity {
	switch a {
	case awsession.ActivityWorking:
		return session.Thinking
	case awsession.ActivityWaiting:
		return session.Waiting
	case awsession.ActivityTerminal:
		return session.Complete
	default:
		return session.Idle
	}
}

func nameFromPath(path string) string {
	parts := splitPath(path)
	// If the path is inside a .claude/worktrees/<slug>/ directory,
	// use the worktree slug as the display name.
	for i := 0; i < len(parts)-2; i++ {
		if parts[i] == ".claude" && parts[i+1] == "worktrees" {
			return parts[i+2]
		}
	}
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
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, gitPath, "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
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

// updatePositions computes racing positions for all non-terminal sessions,
// sets Position/PositionDelta on the supplied updates, and broadcasts
// overtake events when a session passes another.
func (m *Monitor) updatePositions(updates []*session.SessionState) {
	// Get current store state (these carry the previous positions).
	allSessions := m.store.GetAll()

	// Build a lookup of previous positions and names from the store.
	prevPos := make(map[string]int, len(allSessions))
	prevNames := make(map[string]string, len(allSessions))
	for _, s := range allSessions {
		if s.Position > 0 {
			prevPos[s.ID] = s.Position
		}
		prevNames[s.ID] = s.Name
	}

	// Build a combined view: updates override store entries.
	updateMap := make(map[string]*session.SessionState, len(updates))
	for _, u := range updates {
		updateMap[u.ID] = u
	}
	combined := make([]*session.SessionState, 0, len(allSessions)+len(updates))
	seen := make(map[string]bool, len(allSessions))
	for _, s := range allSessions {
		seen[s.ID] = true
		if u, ok := updateMap[s.ID]; ok {
			combined = append(combined, u)
		} else {
			combined = append(combined, s)
		}
	}
	for _, u := range updates {
		if !seen[u.ID] {
			combined = append(combined, u)
		}
	}

	// Collect non-terminal sessions and sort by context utilization descending.
	type utilEntry struct {
		id   string
		name string
		util float64
	}
	racing := make([]utilEntry, 0, len(combined))
	for _, s := range combined {
		if !s.IsTerminal() {
			racing = append(racing, utilEntry{s.ID, s.Name, s.ContextUtilization})
		}
	}
	sort.Slice(racing, func(i, j int) bool {
		return racing[i].util > racing[j].util
	})

	// Assign new positions.
	newPos := make(map[string]int, len(racing))
	for i := 0; i < len(racing); i++ {
		newPos[racing[i].id] = i + 1
	}

	// Build reverse map of previous order: position -> ID.
	prevOrder := make(map[int]string, len(allSessions))
	for _, s := range allSessions {
		if s.Position > 0 && !s.IsTerminal() {
			prevOrder[s.Position] = s.ID
		}
	}

	// Set position/delta on updates and emit overtake events.
	for _, u := range updates {
		np, active := newPos[u.ID]
		if !active {
			u.Position = 0
			u.PositionDelta = 0
			continue
		}
		pp := prevPos[u.ID]
		u.Position = np
		if pp > 0 {
			u.PositionDelta = pp - np // positive = moved up
		}

		// Overtake: moved up and the session now occupying our previous slot is a different car.
		if u.PositionDelta <= 0 {
			continue
		}
		overtakenID, ok := prevOrder[np]
		if !ok || overtakenID == u.ID {
			continue
		}
		overtakenName := prevNames[overtakenID]
		if overtakenName == "" {
			overtakenName = overtakenID
		}
		msg, err := ws.NewOvertakeMessage(ws.OvertakePayload{
			OvertakerID:   u.ID,
			OvertakerName: u.Name,
			OvertakenID:   overtakenID,
			OvertakenName: overtakenName,
			NewPosition:   np,
		})
		if err != nil {
			slog.Error("marshal overtake event failed", "error", err)
			continue
		}
		m.broadcaster.BroadcastMessage(msg)
	}
}
