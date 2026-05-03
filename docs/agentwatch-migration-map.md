# Agentwatch v0.1.0 API Surface vs Agent-Racer Internals

Migration mapping for replacing agent-racer's `backend/internal/` packages with the agentwatch library.

---

## Key Questions

### 1. Does agentwatch `SessionState` have all fields agent-racer needs, or do we need a wrapper?

**Answer: We need a wrapper.** Agentwatch `session.SessionState` covers the core monitoring fields but is missing several agent-racer-specific fields:

| Missing field | Purpose | Wrapper strategy |
|---|---|---|
| `Name` | Display name derived from `WorkingDir` | Compute in wrapper |
| `PID` | Process ID from process discovery | Racer wrapper field |
| `IsChurning` | CPU/network activity signal | Racer wrapper field |
| `TmuxTarget` | Tmux pane identifier | Racer wrapper field |
| `Lane` | Stable visual lane assignment | Racer wrapper field |
| `BurnRatePerMinute` | Token consumption velocity | Racer wrapper field |
| `CompactionCount` | Context compaction events | Racer wrapper field |
| `LastAssistantText` | Recent assistant output snippet | Racer wrapper field |
| `Position` / `PositionDelta` | Racing position + overtake delta | Racer wrapper field |
| `LogPath` | Internal JSONL file path | Racer wrapper field |
| `TokensUsed` (naming) | Agentwatch uses `ContextTokens` + `OutputTokens` separately | Map `ContextTokens` -> `TokensUsed` |

Agentwatch `SessionState` **does** include: `ID`, `Source`, `Slug`, `Activity`, `Lifecycle`, `ContextTokens`, `OutputTokens`, `TokenEstimated`, `MaxContextTokens`, `ContextUtilization`, `Model`, `WorkingDir`, `Branch`, `CurrentTool`, `MessageCount`, `ToolCallCount`, `StartedAt`, `LastActivityAt`, `LastDataReceivedAt`, `CompletedAt`, `Subagents`.

**Recommendation:** Define `type RacerSession struct { session.SessionState; /* racer-only fields */ }` in a new `backend/internal/racer` package.

---

### 2. What is the EventSink contract -- does OnUpdate receive full state or deltas?

**Answer: Full state snapshots per changed session, plus removal IDs.**

The `monitor.EventSink` interface:
```go
type EventSink interface {
    HandleEvent(ctx context.Context, ev Event) error
}
```

The `Event` envelope (`monitor/events.go:28-39`):
```go
type Event struct {
    Seq       uint64
    At        time.Time
    Type      EventType           // "snapshot" | "delta" | "lifecycle" | "health"
    Sessions  []SessionState      // full snapshot (EventSnapshot only)
    Updates   []SessionState      // changed sessions (EventDelta)
    Removed   []string            // IDs removed (EventDelta)
    Lifecycle *LifecycleEvent     // state transition (EventLifecycle)
    Health    *Health             // source health change (EventHealth)
}
```

Key behaviors:
- **EventDelta** delivers `Updates` (full `SessionState` for each session that changed this poll) + `Removed` (IDs removed by retention sweep). This is NOT a field-level diff -- each entry is a complete snapshot of that session's current state.
- **EventLifecycle** fires per state transition (discovered, updated, resumed, terminal, stale, removed).
- **EventSnapshot** is only sent to new WebSocket clients on connect (initial state).
- **EventHealth** fires when a source's health status changes.

**Comparison to agent-racer:** Agent-racer's broadcaster already receives full `[]*session.SessionState` in `QueueUpdate()` and `[]string` in `QueueRemoval()`. The mapping is direct. The main difference is agentwatch bundles them into typed events with sequence numbers.

---

### 3. How does agentwatch monitor expose health checks?

**Answer:** Two mechanisms:

1. **`Monitor.Health() map[string]Health`** -- returns a snapshot of all source health states. Used by the HTTP handler's `/healthz` endpoint.

2. **EventHealth events via EventSink** -- pushed when a source transitions between `healthy`/`degraded`/`failed`.

Health model (`monitor/health.go`):
```go
type Health struct {
    Source           string       // source name
    Status           HealthStatus // "healthy" | "degraded" | "failed"
    DiscoverFailures int
    ParseFailures    int
    LastError        string       // sanitized (no paths/panics)
    UpdatedAt        time.Time
}
```

Threshold logic: `computeStatus(totalFailures, threshold)` -- threshold=degraded, 2*threshold=failed. Default threshold: 3. A successful poll resets counters.

**Comparison to agent-racer:** Agent-racer's health system is more complex with per-session hysteresis (sticky degraded per session, consecutive success counting). Agentwatch uses simpler aggregate counters per source. Agent-racer would need to either:
- Accept the simpler model (sufficient for most cases), or
- Implement per-session tracking in a racer-side health wrapper

The HTTP API surface is equivalent: both expose `/healthz` with status + per-source breakdown.

---

### 4. Does agentwatch include process/tmux awareness or did those stay out?

**Answer: No. Process discovery and tmux resolution are NOT in agentwatch.**

Agentwatch is purely file-based: sources discover session files and parse them. There is no:
- Process scanning (no `gopsutil` dependency)
- PID detection
- CPU/network activity monitoring (`IsChurning`)
- Tmux pane resolution
- `ProcessActivity` type

These remain agent-racer-only concerns. The migration path is to keep `backend/internal/monitor/process.go` and `tmux.go` in agent-racer, running as a post-processing layer on top of agentwatch's session state.

---

### 5. How does the filewatch.Watcher config map to our config.yaml source settings?

**Answer:** The `internal/filewatch.Walker` is an internal (unexported to consumers) utility used by source implementations. Source configuration is done via per-source option functions:

| agent-racer `config.yaml` | agentwatch equivalent |
|---|---|
| `sources.claude: true` | `claude.New(claude.WithRoot("~/.claude/projects"))` |
| `sources.codex: true` | `codex.New(codex.WithRoot("~/.codex/sessions"))` |
| `sources.gemini: true` | `gemini.New(gemini.WithRoot(...))` |
| `monitor.poll_interval` | `monitor.WithPollInterval(d)` |
| `monitor.session_stale_after` | `monitor.WithStaleThreshold(d)` |
| `monitor.completion_remove_after` | `monitor.WithCompletionRetention(d)` |
| `monitor.health_warning_threshold` | `monitor.WithHealthThreshold(n)` |
| `monitor.session_end_dir` | `claude.WithSessionEndDir(path)` |
| `monitor.snapshot_interval` | Not in agentwatch (wsapi handles it via periodic snapshot sends on heartbeat) |
| `monitor.broadcast_throttle` | Not in agentwatch (wsapi handles fan-out synchronously) |
| `monitor.churning_cpu_threshold` | N/A (no process awareness) |
| `monitor.churning_requires_network` | N/A (no process awareness) |
| `monitor.mock_tick_interval` | N/A (use mock source from `sources/mock`) |
| `server.auth_token` | `wsapi.WithAuthenticator(impl)` |
| `server.max_connections` | `wsapi.WithMaxConnections(n)` |
| `server.allowed_origins` | `wsapi.WithAllowedOrigins(origins)` |

**Note:** Agentwatch's `internal/filewatch` is not exported. Each source configures its own discovery parameters. The agent-racer config file would be parsed in agent-racer and translated into source option calls.

---

## Type Mapping Table

### Source Interface

| agentwatch | agent-racer | Differences |
|---|---|---|
| `source.Source` interface | `monitor.Source` interface | Agentwatch adds `context.Context` param to `Discover` and `Parse`; uses opaque `Cursor` string instead of `int64` offset |
| `source.SessionHandle` | `monitor.SessionHandle` | Agentwatch: `{ID, Path, WorkingDir, StartedAt, Source}`. Agent-racer adds: `KnownSlug`, `KnownSubagentParents`. Note: agent-racer's `LogPath` maps to agentwatch's `Path` |
| `source.SourceUpdate` | `monitor.SourceUpdate` | See detailed comparison below |
| `source.Cursor` (string) | `int64` byte offset | Agentwatch's opaque cursor enables richer continuation tokens (e.g., claude source encodes parent maps) |

### SourceUpdate Field Comparison

| agentwatch `source.SourceUpdate` | agent-racer `monitor.SourceUpdate` | Notes |
|---|---|---|
| `SessionID` | `SessionID` | Same |
| `Slug` | `Slug` | Same |
| `Activity` (session.Activity string) | `Activity` (string) | Agentwatch uses typed string; agent-racer uses raw string classified later |
| `Model` | `Model` | Same |
| `ContextTokens` | `TokensIn` | Rename only |
| `OutputTokens` | `TokensOut` | Rename only |
| `MaxContextTokens` | `MaxContextTokens` | Same |
| `TokenEstimated` | -- | Agentwatch sets at source level; agent-racer derives in monitor |
| `MessageCountDelta` | `MessageCount` | Same semantics (delta), different name |
| `ToolCallCountDelta` | `ToolCalls` | Same semantics (delta), different name |
| `CurrentTool` | `LastTool` | Rename only |
| `WorkingDir` | `WorkingDir` | Same |
| `Branch` | `Branch` | Same |
| `StartedAt` | -- | Agentwatch carries in update; agent-racer uses handle only |
| `LastActivityAt` | `LastTime` | Rename only |
| `Subagents []SubagentState` | `Subagents map[string]*SubagentParseResult` | Different shape (see below) |
| `Terminal` | -- | Agentwatch has explicit terminal signal; agent-racer uses session-end markers separately |
| `EndReason` | -- | Agentwatch carries in update |
| `EndedAt` | -- | Agentwatch carries in update |
| -- | `CompactionCount` | Agent-racer only |
| -- | `LastAssistantText` | Agent-racer only |
| -- | `HasData()` method | Agent-racer only (agentwatch checks `SessionID == ""`) |

### Session State

| agentwatch `session.SessionState` | agent-racer `session.SessionState` | Notes |
|---|---|---|
| `ID` | `ID` | Same |
| `Source` | `Source` | Same |
| `Slug` | `Slug` | Same |
| `Activity` (string type) | `Activity` (int enum) | Different representation; agentwatch uses string constants, agent-racer uses iota enum |
| `Lifecycle` (LifecycleState) | -- | Agentwatch has explicit `active`/`terminal`; agent-racer derives from Activity enum |
| `ContextTokens` | `TokensUsed` | Different name; agentwatch separates input/output |
| `OutputTokens` | -- | Agent-racer combines into `TokensUsed` |
| `TokenEstimated` | `TokenEstimated` | Same |
| `MaxContextTokens` | `MaxContextTokens` | Same |
| `ContextUtilization` | `ContextUtilization` | Same (computed) |
| `Model` | `Model` | Same |
| `WorkingDir` | `WorkingDir` | Same |
| `Branch` | `Branch` | Same |
| `CurrentTool` | `CurrentTool` | Same |
| `MessageCount` | `MessageCount` | Same |
| `ToolCallCount` | `ToolCallCount` | Same |
| `StartedAt` | `StartedAt` | Same |
| `LastActivityAt` | `LastActivityAt` | Same |
| `LastDataReceivedAt` | `LastDataReceivedAt` | Same |
| `CompletedAt` | `CompletedAt` | Same (`*time.Time`) |
| `Subagents []SubagentState` | `Subagents []SubagentState` | Different fields (see below) |
| -- | `Name` | Agent-racer only (display name from path) |
| -- | `PID` | Agent-racer only (process discovery) |
| -- | `IsChurning` | Agent-racer only (CPU heuristic) |
| -- | `TmuxTarget` | Agent-racer only (tmux pane) |
| -- | `Lane` | Agent-racer only (visual slot) |
| -- | `BurnRatePerMinute` | Agent-racer only (token velocity) |
| -- | `CompactionCount` | Agent-racer only |
| -- | `LastAssistantText` | Agent-racer only |
| -- | `Position` / `PositionDelta` | Agent-racer only (racing positions) |
| -- | `LogPath` | Agent-racer only (internal path) |

### SubagentState

| agentwatch `session.SubagentState` | agent-racer `session.SubagentState` | Notes |
|---|---|---|
| `ID` | `ID` | Same |
| `ParentID` | `ParentToolUseID` | Different name |
| `Activity` | `Activity` | Same (typed differently) |
| `CurrentTool` | `CurrentTool` | Same |
| `StartedAt` | `StartedAt` | Same |
| `LastActivityAt` | `LastActivityAt` | Same |
| -- | `SessionID` | Agent-racer only (parent session reference) |
| -- | `Slug` | Agent-racer only |
| -- | `Model` | Agent-racer only |
| -- | `TokensUsed` | Agent-racer only |
| -- | `MessageCount` | Agent-racer only |
| -- | `ToolCallCount` | Agent-racer only |
| -- | `CompletedAt` | Agent-racer only |

### Activity Values

| agentwatch | agent-racer | Notes |
|---|---|---|
| `"idle"` | `Idle` (4) | Same concept |
| `"working"` | `Thinking` (1) / `ToolUse` (2) | Agentwatch merges into one; agent-racer splits |
| `"waiting"` | `Waiting` (3) | Same |
| `"terminal"` | `Complete` (5) / `Errored` (6) / `Lost` (7) | Agentwatch uses Lifecycle state; agent-racer uses Activity enum |
| -- | `Starting` (0) | Agent-racer only |

### Lifecycle Events

| agentwatch `session.LifecycleEvent` | agent-racer `session.Event` | Notes |
|---|---|---|
| `EventDiscovered` | `EventNew` | Same semantics |
| `EventUpdated` | `EventUpdate` | Same semantics |
| `EventTerminal` | `EventTerminal` | Same semantics |
| `EventResumed` | -- | Agent-racer handles inline (clears CompletedAt) |
| `EventStale` | -- | Agent-racer marks Lost via Activity change |
| `EventRemoved` | -- | Agent-racer uses store.Remove + broadcaster |
| Has `From`/`To`/`Reason` | Has `ActiveCount` | Different metadata |

### Monitor

| agentwatch `monitor.Monitor` | agent-racer `monitor.Monitor` | Notes |
|---|---|---|
| `New(opts...)` | `NewMonitor(cfg, store, broadcaster, sources)` | Agentwatch uses functional options; agent-racer uses explicit deps |
| `Run(ctx)` | `Start(ctx)` | Same semantics |
| `PollOnce(ctx)` | `poll()` (unexported) | Agentwatch exposes for testing |
| `Snapshot()` | `store.GetAll()` | Agentwatch owns its store; agent-racer has separate Store |
| `Get(id)` | `store.Get(id)` | Same |
| `Health()` | `SourceHealthSnapshot()` | Similar |
| `Sources()` | -- (via config) | Agentwatch exposes registered source names |
| Internal store | External `session.Store` | Agent-racer's store is shared with broadcaster/handlers |

### Transport (WebSocket)

| agentwatch `transport/wsapi` | agent-racer `internal/ws` | Notes |
|---|---|---|
| `wsapi.Server` (implements EventSink + http.Handler) | `ws.Broadcaster` + `ws.Server` | Agentwatch combines; agent-racer splits concerns |
| Event = `monitor.Event` JSON | `WSMessage{Type, Seq, Payload}` envelope | Different wire format |
| Event types: snapshot, delta, lifecycle, health | Message types: snapshot, delta, completion, equipped, source_health, achievement_unlocked, battlepass_progress, overtake | Agent-racer has many gamification message types |
| `coder/websocket` library | `gorilla/websocket` library | Different WS libraries |
| Origin check + optional Authenticator | Origin check + auth token | Similar |
| Rate limiting (per-IP) | Rate limiting (per-IP) | Similar |
| Slow-client eviction | Slow-client eviction | Same pattern |

### Transport (HTTP)

| agentwatch `transport/httpapi` | agent-racer | Notes |
|---|---|---|
| `GET /sessions` | `GET /api/sessions` (in ws.Server) | Same semantics |
| `GET /sessions/{id}` | -- | Agent-racer doesn't have single-session HTTP endpoint |
| `GET /healthz` | `GET /api/healthz` | Same semantics |
| `GET /sources` | -- | Agent-racer doesn't expose this |
| Privacy via `session.Policy` | Privacy via `session.PrivacyFilter` | See below |

### Privacy

| agentwatch `session.Policy` | agent-racer `session.PrivacyFilter` | Notes |
|---|---|---|
| `RedactWorkingDir` | `MaskWorkingDirs` | Agentwatch: clears field. Agent-racer: keeps basename |
| `RedactBranch` | -- | Agentwatch only |
| `RedactModel` | -- | Agentwatch only |
| `RedactSessionID` | `MaskSessionIDs` | Agentwatch: clears. Agent-racer: short hash |
| `RedactSource` | -- | Agentwatch only |
| -- | `MaskPIDs` | Agent-racer only |
| -- | `MaskTmuxTargets` | Agent-racer only |
| -- | `AllowedPaths` / `BlockedPaths` | Agent-racer only (path-based session filtering) |

---

## Gap Analysis: Agent-Racer Features with NO Agentwatch Equivalent

These features require racer-side wrappers or extensions:

### Session State Extensions
1. **`Name`** -- derived from `WorkingDir` (worktree slug or dir basename)
2. **`PID`** -- OS process ID for the agent
3. **`IsChurning`** -- CPU + network activity heuristic
4. **`TmuxTarget`** -- tmux window:pane string
5. **`Lane`** -- stable visual slot assignment (auto-increment)
6. **`BurnRatePerMinute`** -- rolling token consumption rate
7. **`CompactionCount`** -- context compaction events
8. **`LastAssistantText`** -- recent output text snippet
9. **`Position` / `PositionDelta`** -- racing position tracking
10. **`LogPath`** -- internal file reference

### Source Update Extensions
1. **`CompactionCount`** delta
2. **`LastAssistantText`**
3. **`HasData()`** method (agentwatch uses `SessionID == ""` check)

### Monitor Features
1. **Process activity discovery** (`DiscoverProcessActivity`, `ProcessActivity`)
2. **Tmux resolution** (`TmuxResolver`, PID-to-pane mapping)
3. **Burn rate calculation** (rolling token snapshots)
4. **Position/overtake computation**
5. **Session end marker consumption** (now in agentwatch's claude source, but agent-racer has additional cross-source lookup logic)
6. **Config hot-reload** (`SetConfig`, `SetSources`, `reconfigureCh`)
7. **Token normalization strategies** (usage/estimate/message_count per source)
8. **Gamification hooks** (stats events, achievements, battle pass)

### Broadcaster/WebSocket Extensions
1. **Gamification message types** (completion, equipped, achievement_unlocked, battlepass_progress, overtake)
2. **Team computation** (`ComputeTeams` -- groups sessions by project)
3. **Snapshot loop** (periodic full-state broadcasts at configurable interval)
4. **Throttled flush** (batch delta broadcasts with configurable delay)
5. **Health hook** (include source health in snapshot broadcasts)

### Privacy Extensions
1. **Path-based filtering** (AllowedPaths/BlockedPaths globs)
2. **Short-hash masking** (vs agentwatch's full redaction)
3. **PID/TmuxTarget masking**

### SubagentState Extensions
1. **`SessionID`** (parent reference)
2. **`Slug`**, **`Model`** (richer metadata)
3. **`TokensUsed`**, **`MessageCount`**, **`ToolCallCount`** (accumulated stats)
4. **`CompletedAt`** (completion timestamp)

---

## Migration Strategy Summary

The migration can proceed in layers:

1. **Replace source implementations** -- Agent-racer's `claude_source.go`, `codex_source.go`, `gemini_source.go` can delegate to agentwatch source packages. The `Cursor` abstraction eliminates the need for racer to manage byte offsets.

2. **Adopt the monitor core** -- Replace agent-racer's `poll()` and state management with `monitor.New(...).Run(ctx)`. Wire an `EventSink` that translates agentwatch events into racer's enrichment pipeline.

3. **Keep racer-side enrichment** -- Process discovery, tmux resolution, burn rate, positions, gamification, and teams remain in agent-racer as a post-processing layer between the agentwatch monitor and the broadcaster.

4. **Keep racer's broadcaster** -- Agentwatch's wsapi is simpler (no gamification messages, no teams, no throttled flush). Agent-racer should consume agentwatch events and feed its own broadcaster rather than using wsapi directly.

5. **Unify session state** -- Define a `RacerSession` wrapper that embeds `session.SessionState` and adds racer-only fields. This is the type that flows through the broadcaster to clients.
