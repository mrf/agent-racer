package monitor

import (
	"context"
	"log"
	"path/filepath"
	"time"

	"github.com/agent-racer/backend/internal/config"
	"github.com/agent-racer/backend/internal/session"
	"github.com/agent-racer/backend/internal/ws"
)

type trackedSession struct {
	pid          int
	sessionFile  string
	fileOffset   int64
	lastPollTime time.Time
	workingDir   string
}

type Monitor struct {
	cfg           *config.Config
	store         *session.Store
	broadcaster   *ws.Broadcaster
	tracked       map[int]*trackedSession // keyed by PID
	skipped       map[int]bool            // PIDs we failed to find session files for
	fileToSession map[string]int          // session file path -> tracking PID (dedup)
}

func NewMonitor(cfg *config.Config, store *session.Store, broadcaster *ws.Broadcaster) *Monitor {
	return &Monitor{
		cfg:           cfg,
		store:         store,
		broadcaster:   broadcaster,
		tracked:       make(map[int]*trackedSession),
		skipped:       make(map[int]bool),
		fileToSession: make(map[string]int),
	}
}

func (m *Monitor) Start(ctx context.Context) {
	ticker := time.NewTicker(m.cfg.Monitor.PollInterval)
	defer ticker.Stop()

	log.Println("Monitor started, polling for Claude sessions")

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
	processes, err := DiscoverSessions()
	if err != nil {
		log.Printf("Process discovery error: %v", err)
		return
	}

	activePIDs := make(map[int]bool)
	for _, p := range processes {
		activePIDs[p.PID] = true
	}

	// Clean up skipped PIDs that are gone so we re-check if they reappear
	for pid := range m.skipped {
		if !activePIDs[pid] {
			delete(m.skipped, pid)
		}
	}

	// Detect disappeared processes
	var disappeared []int
	for pid := range m.tracked {
		if !activePIDs[pid] {
			disappeared = append(disappeared, pid)
		}
	}

	for _, pid := range disappeared {
		ts := m.tracked[pid]
		log.Printf("Process %d disappeared (session in %s)", pid, ts.workingDir)

		sessionID := SessionIDFromPath(ts.sessionFile)
		if state, ok := m.store.Get(sessionID); ok {
			state.Activity = session.Complete
			now := time.Now()
			state.CompletedAt = &now
			m.store.Update(state)
			m.broadcaster.QueueCompletion(sessionID, session.Complete, state.Name)
			m.broadcaster.QueueUpdate([]*session.SessionState{state})
		}
		delete(m.fileToSession, ts.sessionFile)
		delete(m.tracked, pid)
	}

	// Process active sessions
	var updates []*session.SessionState

	for _, proc := range processes {
		if m.skipped[proc.PID] {
			continue
		}

		ts, exists := m.tracked[proc.PID]

		if !exists {
			// New process found
			sessionFile, err := FindSessionForProcess(proc.WorkingDir, proc.StartTime)
			if err != nil {
				log.Printf("Skipping PID %d (%s): no session file found", proc.PID, proc.WorkingDir)
				m.skipped[proc.PID] = true
				continue
			}

			// Deduplicate: skip if another PID is already tracking this session file
			if trackingPID, ok := m.fileToSession[sessionFile]; ok {
				log.Printf("PID %d shares session file with PID %d, skipping", proc.PID, trackingPID)
				m.skipped[proc.PID] = true
				continue
			}

			ts = &trackedSession{
				pid:         proc.PID,
				sessionFile: sessionFile,
				workingDir:  proc.WorkingDir,
			}
			m.tracked[proc.PID] = ts
			m.fileToSession[sessionFile] = proc.PID
			log.Printf("Tracking new session: PID %d, dir %s, file %s", proc.PID, proc.WorkingDir, sessionFile)
		}

		// Read new JSONL entries
		result, newOffset, err := ParseSessionJSONL(ts.sessionFile, ts.fileOffset)
		if err != nil {
			log.Printf("JSONL parse error for PID %d: %v", proc.PID, err)
			continue
		}
		ts.fileOffset = newOffset
		ts.lastPollTime = time.Now()

		sessionID := SessionIDFromPath(ts.sessionFile)
		model := result.Model
		if model == "" {
			model = "unknown"
		}

		maxTokens := m.cfg.MaxContextTokens(model)

		var tokensUsed int
		if result.LatestUsage != nil {
			tokensUsed = result.LatestUsage.TotalContext()
		}

		// Determine activity
		activity := classifyActivity(result, ts)

		// Get existing state or create new
		state, existed := m.store.Get(sessionID)
		if !existed {
			state = &session.SessionState{
				ID:               sessionID,
				Name:             nameFromPath(ts.workingDir),
				StartedAt:        proc.StartTime,
				MaxContextTokens: maxTokens,
				WorkingDir:       ts.workingDir,
				PID:              proc.PID,
			}
		}

		state.Activity = activity
		if result.Model != "" {
			state.Model = model
		}
		state.LastActivityAt = time.Now()

		// Token count: use the latest usage from the most recent assistant message
		// This represents the total context window used in that API call
		if tokensUsed > state.TokensUsed {
			state.TokensUsed = tokensUsed
		}
		state.MaxContextTokens = maxTokens
		state.UpdateUtilization()

		// Accumulate counts
		state.MessageCount += result.MessageCount
		state.ToolCallCount += result.ToolCalls
		if result.LastTool != "" {
			state.CurrentTool = result.LastTool
		}

		m.store.Update(state)
		updates = append(updates, state)
	}

	if len(updates) > 0 {
		m.broadcaster.QueueUpdate(updates)
	}
}

func classifyActivity(result *ParseResult, ts *trackedSession) session.Activity {
	switch result.LastActivity {
	case "tool_use":
		return session.ToolUse
	case "thinking":
		return session.Thinking
	case "waiting":
		return session.Waiting
	default:
		// No new entries — idle
		if result.MessageCount == 0 {
			return session.Idle
		}
		return session.Thinking
	}
}

func nameFromPath(path string) string {
	// Use the last directory component as the session name
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
