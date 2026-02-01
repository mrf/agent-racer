package monitor

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/agent-racer/backend/internal/config"
	"github.com/agent-racer/backend/internal/session"
	"github.com/agent-racer/backend/internal/ws"
)

type trackedSession struct {
	sessionFile  string
	fileOffset   int64
	lastDataTime time.Time
	workingDir   string
}

type Monitor struct {
	cfg         *config.Config
	store       *session.Store
	broadcaster *ws.Broadcaster
	tracked     map[string]*trackedSession // keyed by session file path
}

func NewMonitor(cfg *config.Config, store *session.Store, broadcaster *ws.Broadcaster) *Monitor {
	return &Monitor{
		cfg:         cfg,
		store:       store,
		broadcaster: broadcaster,
		tracked:     make(map[string]*trackedSession),
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
	sessionFiles, err := FindRecentSessionFiles(10 * time.Minute)
	if err != nil {
		log.Printf("Session file discovery error: %v", err)
		return
	}

	activeFiles := make(map[string]bool)
	for _, path := range sessionFiles {
		activeFiles[path] = true
	}

	now := time.Now()
	var toRemove []string
	for path, ts := range m.tracked {
		stale := !activeFiles[path]
		if !stale && m.cfg.Monitor.SessionStaleAfter > 0 && now.Sub(ts.lastDataTime) > m.cfg.Monitor.SessionStaleAfter {
			stale = true
		}
		if !stale {
			continue
		}

		sessionID := SessionIDFromPath(path)
		if state, ok := m.store.Get(sessionID); ok {
			state.Activity = session.Complete
			completedAt := time.Now()
			state.CompletedAt = &completedAt
			m.store.Update(state)
			m.broadcaster.QueueCompletion(sessionID, session.Complete, state.Name)
			m.broadcaster.QueueUpdate([]*session.SessionState{state})
		}
		toRemove = append(toRemove, path)
	}
	for _, path := range toRemove {
		delete(m.tracked, path)
	}

	var updates []*session.SessionState

	for _, path := range sessionFiles {
		ts, exists := m.tracked[path]
		if !exists {
			workingDir := workingDirFromFile(path)
			ts = &trackedSession{
				sessionFile: path,
				workingDir:  workingDir,
			}
			m.tracked[path] = ts
			log.Printf("Tracking new session file: %s", path)
		}

		oldOffset := ts.fileOffset
		result, newOffset, err := ParseSessionJSONL(ts.sessionFile, ts.fileOffset)
		if err != nil {
			log.Printf("JSONL parse error for file %s: %v", ts.sessionFile, err)
			continue
		}
		ts.fileOffset = newOffset
		if newOffset > oldOffset {
			ts.lastDataTime = now
		}

		sessionID := result.SessionID
		if sessionID == "" {
			sessionID = SessionIDFromPath(ts.sessionFile)
		}

		state, existed := m.store.Get(sessionID)
		if existed && state.IsTerminal() {
			continue
		}

		if !existed {
			startedAt := now
			if info, err := os.Stat(ts.sessionFile); err == nil {
				startedAt = info.ModTime()
			}
			state = &session.SessionState{
				ID:         sessionID,
				Name:       nameFromPath(ts.workingDir),
				StartedAt:  startedAt,
				WorkingDir: ts.workingDir,
			}
		}

		activity := classifyActivity(result, ts)
		state.Activity = activity
		if result.Model != "" {
			state.Model = result.Model
		}

		modelForLookup := state.Model
		if modelForLookup == "" {
			modelForLookup = "unknown"
		}
		maxTokens := m.cfg.MaxContextTokens(modelForLookup)

		if result.LastTime.IsZero() {
			state.LastActivityAt = now
		} else {
			state.LastActivityAt = result.LastTime
		}

		if result.LatestUsage != nil {
			tokensUsed := result.LatestUsage.TotalContext()
			if tokensUsed > state.TokensUsed {
				state.TokensUsed = tokensUsed
			}
		}
		state.MaxContextTokens = maxTokens
		state.UpdateUtilization()

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

func workingDirFromFile(sessionFile string) string {
	projectDir := filepath.Base(filepath.Dir(sessionFile))
	if projectDir == "" || projectDir == "." || projectDir == "/" {
		return ""
	}
	return DecodeProjectPath(projectDir)
}
