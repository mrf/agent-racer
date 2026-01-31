# Agent Racer

Real-time racing visualization of active Claude Code sessions. Process scanning discovers sessions, JSONL files provide state, context window utilization drives track position.

## Build & Run

```bash
make dev          # Mock mode + filesystem frontend (hot-reload)
make run          # Real mode + filesystem frontend
make build        # Single binary with embedded frontend
make test         # Run Go tests
make lint         # Run golangci-lint
make dist         # Cross-compile linux/darwin amd64/arm64
```

## Code Standards

### Go Backend

- **Go 1.22+** minimum. Use modern idioms (range-over-func when appropriate, structured logging).
- **golangci-lint** is the canonical linter. All code must pass with zero warnings. Preferred linters include `errcheck`, `govet`, `staticcheck`, `gosimple`, `ineffassign`, `unused`, `misspell`, `gofmt`, `goimports`.
- **Error handling**: Always check and propagate errors. Never ignore returned errors. Use `fmt.Errorf("context: %w", err)` for wrapping.
- **Naming**: Follow Go conventions. Exported names use PascalCase. Unexported use camelCase. Package names are lowercase, single-word where possible.
- **Concurrency**: Protect shared state with `sync.RWMutex`. Prefer channels for communication, mutexes for state protection. Document goroutine ownership.
- **No global state**: Pass dependencies explicitly. Use structs with methods, not package-level vars.

### Testing

- **Comprehensive tests without heavy mocking.** Prefer real implementations and test doubles over mock frameworks. The mock package (`internal/mock/`) is for the demo UI, not for test infrastructure.
- **Table-driven tests** for functions with multiple input/output cases.
- **Test file placement**: `_test.go` files live alongside the code they test.
- **Test naming**: `TestFunctionName_Scenario` (e.g., `TestParseSessionJSONL_IncrementalRead`).
- **Integration tests**: Test full poll cycles where feasible (process discovery -> JSONL parse -> store update -> broadcast).
- **No test pollution**: Tests must not depend on ordering, global state, or filesystem side effects outside `t.TempDir()`.
- **Coverage target**: Aim for meaningful coverage of business logic. Don't chase 100% -- cover edge cases and error paths that matter.
- Run tests with `go test -race ./...` to catch data races.

### Frontend

- **Vanilla JS with ES modules.** No bundler, no framework. `<script type="module">` only.
- **Canvas-based rendering** via `requestAnimationFrame`. No DOM manipulation in the render loop.
- **Class-per-file**: Each module exports a single class (RaceCanvas, Track, Racer, ParticleSystem, RaceConnection).
- **No external JS dependencies.** Sound effects are synthesized via AudioContext, not asset files.

## Architecture

```
Process Scanner (/proc)          Go Backend            Browser (Canvas)
 |                                  |                      |
 |-- pgrep claude, read PIDs ------>|                      |
 |-- /proc/<pid>/cwd ------------->|                      |
 |                                  |                      |
JSONL File Reader                   |-- WebSocket -------->|
 |-- ~/.claude/projects/...jsonl -->|   snapshots + deltas |
 |-- parse token usage ----------->|                      |
 |-- classify activity ----------->|                      |
```

### Session Discovery Flow

```
1. pgrep -fa claude -> PIDs of running claude processes
2. /proc/<pid>/cwd -> working directory per process
3. URL-encode working dir -> ~/.claude/projects/<encoded>/
4. Find most recent .jsonl file by mtime -> session file
5. Parse JSONL from last offset -> token usage, activity, tool calls
6. Process exit detected -> mark session complete
```

### Key Design Decisions

1. **No tmux**: Process scanning + JSONL file reading. Works with any Claude session.
2. **Position = context utilization**: `(input_tokens + cache_read + cache_creation) / model_max_context`.
3. **Incremental JSONL reading**: Track file byte offset, only read new entries each poll.
4. **Activity from JSONL entries**: Assistant content = thinking, tool_use blocks = toolUse, no growth = idle, process exit = complete.
5. **Delta WS protocol**: Only send changed fields. Full snapshot every 5s + on connect.
6. **Single binary**: `go:embed` bundles frontend for production.

### Go Dependencies

Only two external dependencies. Keep it minimal:
- `github.com/gorilla/websocket` -- WebSocket server
- `gopkg.in/yaml.v3` -- YAML config parsing

## Claude Session Data Paths

- Session JSONL: `~/.claude/projects/<url-encoded-project-path>/<session-uuid>.jsonl`
- History: `~/.claude/history.jsonl`
- Stats: `~/.claude/stats-cache.json`

## Project Layout

```
backend/
  cmd/server/main.go              # Entry point, flag parsing
  internal/
    config/config.go              # YAML config, model token limits
    session/
      state.go                    # SessionState, Activity enum (8 states)
      store.go                    # Thread-safe store with lane assignment
    monitor/
      process.go                  # /proc-based process discovery
      jsonl.go                    # Incremental JSONL parser
      monitor.go                  # Main 1s poll loop
    ws/
      protocol.go                 # Message types (snapshot, delta, completion)
      broadcast.go                # Per-client channels, throttled batching
      server.go                   # HTTP/WS handlers
    mock/generator.go             # 5 demo sessions with distinct patterns
    frontend/
      embed.go                    # go:embed for production
      noembed.go                  # Filesystem fallback for dev
frontend/
  index.html
  styles.css
  src/
    main.js                       # Bootstrap, shortcuts, sound, detail panel
    websocket.js                  # Auto-reconnecting WS client
    notifications.js              # Browser notifications
    canvas/
      RaceCanvas.js               # rAF loop, canvas orchestration
      Track.js                    # Track rendering, lane layout, markers
      Particles.js                # Particle presets
    entities/
      Racer.js                    # Car entity, activity animations
```

## Implementation Plan

See the full implementation plan with all groups (A-E) and file-by-file breakdown in [PLAN.md](./PLAN.md). The revised plan supersedes the original tmux-based plan that PLAN.md still contains.

### Implementation Groups (Revised Plan)

- **Group A**: Skeleton -- process monitor + mock server + minimal frontend (items 1-15)
- **Group B**: Canvas rendering -- racing visualization with particles and animations (items 16-24)
- **Group C**: Real session monitoring -- process scanning + JSONL reading (items 25-28)
- **Group D**: Polish -- detail panel, keyboard shortcuts, sound, edge cases (items 29-34)
- **Group E**: Packaging -- go:embed, cross-compile, install script (items 35-37)

All groups are implemented. See CHANGELOG.txt for details.
