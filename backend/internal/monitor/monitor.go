package monitor

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

type sessionEndMarker struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	Reason         string `json:"reason"`
	Timestamp      string `json:"timestamp"`
}

type Monitor struct {
	cfg             *config.Config
	store           *session.Store
	broadcaster     *ws.Broadcaster
	sources         []Source
	tracked         map[string]*trackedSession // keyed by source:sessionID
	pendingRemoval  map[string]time.Time
	removedKeys     map[string]bool // keys removed from store; prevents re-creation while file is still discovered
	prevCPU         map[int]cpuSample
	lastProcessPoll time.Time
}

func NewMonitor(cfg *config.Config, store *session.Store, broadcaster *ws.Broadcaster, sources []Source) *Monitor {
	return &Monitor{
		cfg:             cfg,
		store:           store,
		broadcaster:     broadcaster,
		sources:         sources,
		tracked:         make(map[string]*trackedSession),
		pendingRemoval:  make(map[string]time.Time),
		removedKeys:     make(map[string]bool),
		prevCPU:         make(map[int]cpuSample),
		lastProcessPoll: time.Now(),
	}
}

func (m *Monitor) Start(ctx context.Context) {
	ticker := time.NewTicker(m.cfg.Monitor.PollInterval)
	defer ticker.Stop()

	sourceNames := make([]string, len(m.sources))
	for i, s := range m.sources {
		sourceNames[i] = s.Name()
	}
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
	m.consumeSessionEndMarkers(now)

	// Collect active session keys from all sources for stale detection.
	activeKeys := make(map[string]bool)

	var updates []*session.SessionState

	for _, src := range m.sources {
		handles, err := src.Discover()
		if err != nil {
			log.Printf("[%s] discovery error: %v", src.Name(), err)
			continue
		}

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
			update, newOffset, err := src.Parse(ts.handle, ts.fileOffset)
			if err != nil {
				log.Printf("[%s] parse error for %s: %v", src.Name(), h.SessionID, err)
				continue
			}
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
				if !update.LastTime.IsZero() && m.cfg.Monitor.SessionStaleAfter > 0 {
					if now.Sub(update.LastTime) > m.cfg.Monitor.SessionStaleAfter {
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

			// Prefer source-reported context ceiling; fall back to config.
			maxTokens := update.MaxContextTokens
			if maxTokens == 0 {
				modelForLookup := state.Model
				if modelForLookup == "" {
					modelForLookup = "unknown"
				}
				maxTokens = m.cfg.MaxContextTokens(modelForLookup)
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
			if update.LastTool != "" {
				state.CurrentTool = update.LastTool
			}

			if len(update.Subagents) > 0 {
				mergeSubagents(state, update.Subagents)
			}

			m.resolveTokens(state, update, maxTokens)

			// Calculate burn rate from token history
			state.BurnRatePerMinute = m.calculateBurnRate(ts, state.TokensUsed, now)

			m.store.Update(state)
			updates = append(updates, state)
		}
	}

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
	cpuThreshold := m.cfg.Monitor.ChurningCPUThreshold
	requireNetwork := m.cfg.Monitor.ChurningRequiresNetwork
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
		if state.IsChurning != churning {
			state.IsChurning = churning
			m.store.Update(state)
		}
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
		m.store.Update(state)
	}

	// Mark stale sessions as lost (disappeared without session end marker).
	var toRemove []string
	for key, ts := range m.tracked {
		if activeKeys[key] {
			// Terminal sessions stay tracked for resume detection;
			// skip stale marking for them.
			if state, ok := m.store.Get(key); ok && state.IsTerminal() {
				continue
			}
			// Still discovered and not stale by time — skip.
			isStale := m.cfg.Monitor.SessionStaleAfter > 0 && now.Sub(ts.lastDataTime) > m.cfg.Monitor.SessionStaleAfter
			if !isStale {
				continue
			}
			log.Printf("Session %s stale (lastData=%s, age=%s, threshold=%s)",
				key, ts.lastDataTime.Format("15:04:05"), now.Sub(ts.lastDataTime).Round(time.Second), m.cfg.Monitor.SessionStaleAfter)
		}

		if state, ok := m.store.Get(key); ok {
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
			m.markTerminal(state, session.Lost, completedAt)
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
	}

	// Purge removedKeys entries for sessions whose files have fallen
	// outside the discover window.  Once the file is no longer discovered
	// there is no risk of zombie re-creation.
	for key := range m.removedKeys {
		if !activeKeys[key] {
			delete(m.removedKeys, key)
		}
	}

	if len(updates) > 0 {
		m.broadcaster.QueueUpdate(updates)
	}

	m.flushRemovals(now)
}

// markTerminal marks a session with a terminal state (Complete, Errored, or Lost).
func (m *Monitor) markTerminal(state *session.SessionState, activity session.Activity, completedAt time.Time) {
	if state == nil {
		return
	}
	wasTerminal := state.IsTerminal()
	state.Activity = activity
	state.CompletedAt = &completedAt
	m.store.Update(state)
	if !wasTerminal {
		log.Printf("Session %s terminal: %s → %s (broadcasting completion)", state.ID, state.Name, activity)
		m.broadcaster.QueueCompletion(state.ID, activity, state.Name)
	}
	m.broadcaster.QueueUpdate([]*session.SessionState{state})
	m.scheduleRemoval(state.ID, completedAt)
}

// markComplete marks a session as successfully completed.
func (m *Monitor) markComplete(state *session.SessionState, completedAt time.Time) {
	m.markTerminal(state, session.Complete, completedAt)
}

// scheduleRemoval enqueues a session for removal after CompletionRemoveAfter.
// A zero duration removes immediately; a negative duration disables removal.
func (m *Monitor) scheduleRemoval(sessionID string, completedAt time.Time) {
	if m.cfg.Monitor.CompletionRemoveAfter < 0 {
		return
	}
	removeAt := completedAt.Add(m.cfg.Monitor.CompletionRemoveAfter)
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
			m.store.Remove(id)
			m.removedKeys[id] = true
		}
	}
	if len(removeIDs) > 0 {
		m.broadcaster.QueueRemoval(removeIDs)
	}
}

// consumeSessionEndMarkers handles Claude-specific SessionEnd hook markers.
// These are JSON files dropped into a directory by the Claude CLI when a
// session ends. Other sources don't use this mechanism.
func (m *Monitor) consumeSessionEndMarkers(now time.Time) {
	dir := m.cfg.Monitor.SessionEndDir
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

		m.handleSessionEnd(marker, now)

		if err := os.Remove(path); err != nil {
			log.Printf("Session end marker cleanup error: %v", err)
		}
	}
}

func (m *Monitor) handleSessionEnd(marker sessionEndMarker, now time.Time) {
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
	m.markTerminal(state, activity, completedAt)

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
func (m *Monitor) resolveTokens(state *session.SessionState, update SourceUpdate, maxTokens int) {
	strategy := m.cfg.TokenStrategy(state.Source)
	tokensPerMsg := m.cfg.TokenNorm.TokensPerMessage
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

const burnRateWindow = 60 * time.Second

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

// mergeSubagents converts SubagentParseResults into SubagentState entries
// on the session. It merges incrementally: existing subagents are updated
// with new data, and new subagents are appended.
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
