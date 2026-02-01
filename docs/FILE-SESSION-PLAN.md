# Refactor: File-based session discovery (replace PID scanning)

## Motivation

PID-based discovery (`/proc` scanning) is fragile: multiple PIDs map to one JSONL file, subprocess agents share working directories, and process enumeration misses sessions. The JSONL files themselves are the source of truth. `FindRecentSessionFiles()` already exists in `jsonl.go` and scans `~/.claude/projects/*/` directly.

The store, broadcaster, WebSocket protocol, and mock generator are already PID-agnostic. Only `monitor.go` needs rewriting.

## Changes

### File: `backend/internal/config/config.go`

Add `SessionStaleAfter` to `MonitorConfig`:
```go
SessionStaleAfter time.Duration `yaml:"session_stale_after"`
```
Default: `2 * time.Minute`. Sessions with no new JSONL data for this duration are marked Complete.

### File: `backend/internal/monitor/monitor.go` (main rewrite)

**1. Rekey `trackedSession` from PID to session file path**

```go
type trackedSession struct {
    sessionFile  string
    fileOffset   int64
    lastDataTime time.Time  // last poll that read new data
    workingDir   string
}
```

Remove `pid` field. Add `lastDataTime` for staleness detection.

**2. Simplify `Monitor` struct**

```go
type Monitor struct {
    cfg         *config.Config
    store       *session.Store
    broadcaster *ws.Broadcaster
    tracked     map[string]*trackedSession // keyed by session file path
}
```

Remove `skipped`, `fileToSession` — no longer needed. Files ARE the identity.

**3. Rewrite `poll()`**

New flow:
1. `FindRecentSessionFiles(10 * time.Minute)` → list of active session file paths
2. Build `activeFiles` set from results
3. **Stale detection**: for each tracked file, if not in `activeFiles` OR `time.Since(ts.lastDataTime) > cfg.SessionStaleAfter` → mark Complete, broadcast completion, remove from tracked
4. **Process active files**: for each file in results:
   - If already tracked → incremental JSONL parse, update state
   - If not tracked → check store: if session already terminal (`IsTerminal()`), skip; otherwise create tracked entry, parse from offset 0, create session state
5. Derive `workingDir` from session file path: `DecodeProjectPath(filepath.Base(filepath.Dir(sessionFile)))`
6. `StartedAt` for new sessions: use the file modification time via `os.Stat()`

**4. Fix model/maxTokens calculation**

After setting `state.Model` conditionally (only when `result.Model != ""`), compute `maxTokens` from `state.Model` instead of from `result.Model`:
```go
if result.Model != "" {
    state.Model = result.Model
}
modelForLookup := state.Model
if modelForLookup == "" {
    modelForLookup = "unknown"
}
maxTokens := m.cfg.MaxContextTokens(modelForLookup)
```

**5. Add helper: `workingDirFromFile(sessionFile string) string`**

Extracts the encoded project dir from the file path and calls `DecodeProjectPath()`.

### Files NOT changed

- `process.go` — kept as-is (not called, but no reason to delete)
- `jsonl.go` — already has `FindRecentSessionFiles()`, `DecodeProjectPath()`, `SessionIDFromPath()`
- `session/state.go` — PID field stays (optional, omitempty)
- `session/store.go` — already keyed by session ID
- `ws/*` — protocol unchanged
- `mock/generator.go` — already PID-free
- `cmd/server/main.go` — `NewMonitor()` signature unchanged

## Verification

1. `cd backend && go test -race ./...` — all tests pass
2. `make dev` — mock mode still works (mock generator is independent)
3. `make run` with multiple Claude sessions open:
   - Each unique JSONL file gets its own racer
   - "Tracking new session" log per file, no PID references
   - Sessions mark Complete after going stale (no new data for 2 min)
   - Model badge persists across idle polls
   - No double-counting of messages/tools
