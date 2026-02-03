package monitor

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agent-racer/backend/internal/config"
	"github.com/agent-racer/backend/internal/session"
	"github.com/agent-racer/backend/internal/ws"
)

// trackedSession holds per-session state used by the monitor between polls.
type trackedSession struct {
	handle       SessionHandle
	fileOffset   int64
	lastDataTime time.Time
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
	cfg              *config.Config
	store            *session.Store
	broadcaster      *ws.Broadcaster
	sources          []Source
	tracked          map[string]*trackedSession // keyed by source:sessionID
	pendingRemoval   map[string]time.Time
	prevCPU          map[int]cpuSample
	lastProcessPoll  time.Time
}

func NewMonitor(cfg *config.Config, store *session.Store, broadcaster *ws.Broadcaster, sources []Source) *Monitor {
	return &Monitor{
		cfg:             cfg,
		store:           store,
		broadcaster:     broadcaster,
		sources:         sources,
		tracked:         make(map[string]*trackedSession),
		pendingRemoval:  make(map[string]time.Time),
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
				ts = &trackedSession{
					handle: h,
				}
				m.tracked[key] = ts
				log.Printf("[%s] Tracking new session: %s", src.Name(), h.SessionID)
			}

			oldOffset := ts.fileOffset
			update, newOffset, err := src.Parse(ts.handle, ts.fileOffset)
			if err != nil {
				log.Printf("[%s] parse error for %s: %v", src.Name(), h.SessionID, err)
				continue
			}
			ts.fileOffset = newOffset
			if newOffset > oldOffset || update.HasData() {
				ts.lastDataTime = now
			}

			// Always use filename-based session ID to ensure session identity
			// remains stable across model switches and JSONL sessionId changes
			sessionID := h.SessionID
			storeKey := trackingKey(h.Source, sessionID)
			state, existed := m.store.Get(storeKey)
			if existed && state.IsTerminal() {
				delete(m.tracked, key)
				continue
			}

			if !existed {
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
				}
			}

			if update.WorkingDir != "" && state.WorkingDir == "" {
				state.WorkingDir = update.WorkingDir
				state.Name = nameFromPath(update.WorkingDir)
			}

			activity := classifyActivityFromUpdate(update)
			state.Activity = activity

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

			if update.TokensIn > 0 && update.TokensIn > state.TokensUsed {
				state.TokensUsed = update.TokensIn
			}
			state.MaxContextTokens = maxTokens
			state.UpdateUtilization()

			state.MessageCount += update.MessageCount
			state.ToolCallCount += update.ToolCalls
			if update.LastTool != "" {
				state.CurrentTool = update.LastTool
			}

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

	// Apply churning state to updated sessions. Churning only makes sense
	// for sessions that would otherwise appear idle or starting -- not for
	// sessions already producing output, waiting for input, or terminal.
	cpuThreshold := m.cfg.Monitor.ChurningCPUThreshold
	requireNetwork := m.cfg.Monitor.ChurningRequiresNetwork
	for _, state := range updates {
		churning := false
		if state.Activity == session.Starting || state.Activity == session.Idle {
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

	// Mark stale sessions as lost (disappeared without session end marker).
	var toRemove []string
	for key, ts := range m.tracked {
		if activeKeys[key] {
			// Still discovered, check time-based staleness.
			if m.cfg.Monitor.SessionStaleAfter > 0 && now.Sub(ts.lastDataTime) > m.cfg.Monitor.SessionStaleAfter {
				// Stale by time.
			} else {
				continue
			}
		}

		if state, ok := m.store.Get(key); ok {
			completedAt := now
			if state.CompletedAt != nil {
				completedAt = *state.CompletedAt
			}
			// Sessions that disappear without session end marker are marked as Lost
			m.markTerminal(state, session.Lost, completedAt)
		}
		toRemove = append(toRemove, key)
	}
	for _, key := range toRemove {
		delete(m.tracked, key)
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
		m.broadcaster.QueueCompletion(state.ID, activity, state.Name)
	}
	m.broadcaster.QueueUpdate([]*session.SessionState{state})
	m.scheduleRemoval(state.ID, completedAt)
}

// markComplete marks a session as successfully completed.
func (m *Monitor) markComplete(state *session.SessionState, completedAt time.Time) {
	m.markTerminal(state, session.Complete, completedAt)
}

func (m *Monitor) scheduleRemoval(sessionID string, completedAt time.Time) {
	if m.cfg.Monitor.CompletionRemoveAfter <= 0 {
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
			removeIDs = append(removeIDs, id)
			delete(m.pendingRemoval, id)
			m.store.Remove(id)
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
	storeKey := trackingKey("claude", marker.SessionID)

	state, ok := m.store.Get(storeKey)
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

	// Clean up tracked session by matching on claude source.
	for key := range m.tracked {
		ts := m.tracked[key]
		if ts.handle.Source == "claude" && ts.handle.SessionID == marker.SessionID {
			delete(m.tracked, key)
			break
		}
	}
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
		if path != "/" {
			path = path[:len(path)-1]
		} else {
			break
		}
	}
	return parts
}

func workingDirFromFile(sessionFile string) string {
	projectDir := filepath.Base(filepath.Dir(sessionFile))
	if projectDir == "" || projectDir == "." || projectDir == "/" {
		return ""
	}
	return DecodeProjectPath(projectDir)
}
