# Architecture

This document covers the technical architecture of Agent Racer — how the backend discovers and parses sessions, how state flows to the browser, the WebSocket API, and the directory structure.

## How It Works

```
JSONL File Discovery               Go Backend            Browser (Canvas)
 |                                  |                      |
 |-- ~/.claude/projects/*/*.jsonl ->|                      |
 |-- parse token usage ----------->|                      |
 |-- classify activity ----------->|                      |
 |                                  |-- WebSocket -------->|
 |                                  |   snapshots + deltas |
                                    |                      |
                                    |   Canvas renders:    |
                                    |   - Race track       |
                                    |   - Cars per session  |
                                    |   - Particles/FX     |
                                    |   - Detail panel     |
```

**Position = context window fill.** Each API response in Claude Code's session log contains token usage (`input_tokens + cache_read_input_tokens + cache_creation_input_tokens`). That total divided by the model's max context (200K) gives a 0.0–1.0 utilization value that maps to track position. As the conversation grows, the car moves forward. Reaching the limit means the finish line (or compaction time).

## Session Lifecycle (Backend)

1. **Discovery** — The monitor polls for running agent processes (via gopsutil) and locates their JSONL session files
2. **Incremental Parsing** — Only new bytes are read each poll (1s interval). The JSONL parser tracks byte offsets per file
3. **State Extraction** — Token usage, model, activity classification, and tool calls are extracted from parsed entries
4. **Store Update** — Parsed data is merged into a thread-safe `SessionStore`
5. **Broadcasting** — Deltas are throttled (100ms minimum) and sent to all connected WebSocket clients. Full snapshots are sent on connect and every 5s
6. **Activity Classification** — Sessions are classified as thinking, tool_use, waiting, idle, churning, complete, errored, or lost
7. **Churning Detection** — If a session has no new JSONL data but the process shows CPU usage above the threshold (default 15%), it's classified as "churning" rather than idle
8. **Completion** — Sessions are marked complete via a SessionEnd hook marker file or after a configurable inactivity timeout (default 2m)
9. **Removal** — Completed/errored sessions are removed after the frontend animation finishes (configurable delay)

## Multi-Source Architecture

Agent sources implement the `Source` interface (`monitor/source.go`):

```go
type Source interface {
    Name() string
    Discover() ([]DiscoveredSession, error)
    Parse(session *DiscoveredSession) (*ParseResult, error)
}
```

Current implementations:
- **Claude** (`claude_source.go`) — Stable. Discovers via `~/.claude/projects/*/*.jsonl`
- **Codex** (`codex_source.go`) — Pre-alpha. Discovers via `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl`
- **Gemini** (`gemini_source.go`) — Pre-alpha. Discovers via `~/.gemini/tmp/<hash>/chats/session-*.json`

Shared JSONL parsing logic lives in `monitor/jsonl.go` to avoid duplication across sources.

## Token Normalization

Different agent sources report token usage differently. The `token_normalization` config section controls how raw data maps to the 0.0–1.0 utilization value:

| Strategy | Description |
|----------|-------------|
| `usage` | Use real token counts from API responses (preferred for Claude, Codex, Gemini) |
| `estimate` / `message_count` | Derive tokens from `message_count × tokens_per_message` (fallback) |

Model context windows are configured in the `models` section. Exact model name matches take priority, then longest prefix match for wildcard patterns.

## WebSocket API

### Connection

```
ws://localhost:8080/ws?token=<auth_token>
```

The auth token is auto-generated each startup (logged to stdout) or can be set in config for persistence. The client auto-reconnects on disconnection.

### Message Types

All messages are JSON with a `type` field and a monotonically increasing `seq` number.

**`snapshot`** — Full state of all sessions (sent on connect and every 5s):
```json
{
  "type": "snapshot",
  "seq": 42,
  "payload": {
    "sessions": [
      {
        "id": "abc-123",
        "name": "my-project",
        "source": "claude",
        "activity": "thinking",
        "tokensUsed": 142000,
        "tokenEstimated": false,
        "maxContextTokens": 200000,
        "contextUtilization": 0.71,
        "currentTool": "",
        "model": "claude-opus-4-5-20251101",
        "workingDir": "/home/user/my-project",
        "branch": "main",
        "startedAt": "2026-01-30T10:00:00Z",
        "lastActivityAt": "2026-01-30T10:05:00Z",
        "lastDataReceivedAt": "2026-01-30T10:05:10Z",
        "messageCount": 42,
        "toolCallCount": 18,
        "pid": 12345,
        "isChurning": true,
        "tmuxTarget": "dev:0.0",
        "lane": 0,
        "burnRatePerMinute": 3500.5,
        "compactionCount": 1,
        "subagents": []
      }
    ],
    "sourceHealth": [
      {
        "source": "claude",
        "status": "healthy",
        "discoverFailures": 0,
        "parseFailures": 0,
        "lastError": "",
        "timestamp": "2026-01-30T10:05:00Z"
      }
    ]
  }
}
```

**`delta`** — Only changed sessions (throttled to 100ms):
```json
{
  "type": "delta",
  "seq": 43,
  "payload": {
    "updates": [ /* ...session objects... */ ],
    "removed": [ "session-id-1" ]
  }
}
```

**`completion`** — Session finished:
```json
{
  "type": "completion",
  "seq": 44,
  "payload": {
    "sessionId": "abc-123",
    "activity": "complete",
    "name": "my-project"
  }
}
```

**`achievement_unlocked`** — Gamification event:
```json
{
  "type": "achievement_unlocked",
  "seq": 45,
  "payload": {
    "id": "speedrunner",
    "name": "Speedrunner",
    "description": "Complete a session in under 5 minutes",
    "tier": "bronze",
    "reward": {
      "type": "cosmetic",
      "id": "exhaust_blue",
      "name": "Blue Exhaust"
    }
  }
}
```

**`battlepass_progress`** — Season progress update:
```json
{
  "type": "battlepass_progress",
  "seq": 46,
  "payload": {
    "xp": 1250,
    "tier": 3,
    "tierProgress": 0.45,
    "recentXP": [
      { "source": "session_complete", "xp": 100, "timestamp": "2026-01-30T10:05:00Z" }
    ],
    "rewards": [ "cosmetic_1", "cosmetic_2" ]
  }
}
```

### REST Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /api/sessions` | JSON array of all current session states (same format as snapshot payload) |
| `GET /api/config` | Server configuration (sound settings, privacy, etc.) |

## Directory Structure

```
agent-racer/
├── Makefile
├── config.example.yaml
├── backend/
│   ├── go.mod
│   ├── cmd/server/
│   │   └── main.go                  # Entry point, flag parsing
│   └── internal/
│       ├── config/config.go          # YAML config loading, defaults
│       ├── session/
│       │   ├── state.go              # SessionState, Activity enum
│       │   └── store.go              # Thread-safe session store
│       ├── ws/
│       │   ├── protocol.go           # WebSocket message types
│       │   ├── broadcast.go          # Per-client write channels, throttled broadcasting
│       │   └── server.go             # HTTP/WS handlers
│       ├── monitor/
│       │   ├── source.go             # Unified Source interface for multi-agent support
│       │   ├── claude_source.go      # Claude Code session discovery + parsing
│       │   ├── codex_source.go       # OpenAI Codex CLI session discovery
│       │   ├── gemini_source.go      # Google Gemini CLI session discovery
│       │   ├── process.go            # Process scanning via gopsutil
│       │   ├── jsonl.go              # Incremental JSONL parser (shared)
│       │   └── monitor.go            # Main poll loop + churning detection
│       ├── mock/
│       │   └── generator.go          # 5 simulated sessions for demo mode
│       ├── gamification/             # Battle pass, achievements, stats tracking
│       └── frontend/
│           ├── embed.go              # go:embed for production builds
│           └── noembed.go            # Filesystem fallback for dev
└── frontend/
    ├── index.html
    ├── styles.css
    └── src/
        ├── main.js                   # Bootstrap, shortcuts, sound, detail panel
        ├── websocket.js              # Auto-reconnecting WebSocket client
        ├── notifications.js          # Browser notifications
        ├── auth.js                   # Token handling
        ├── canvas/
        │   ├── Particles.js          # Exhaust, sparks, smoke, confetti
        │   ├── Track.js              # Track rendering with markers
        │   └── RaceCanvas.js         # requestAnimationFrame loop + glow pass
        ├── entities/
        │   ├── Racer.js              # Car entity with activity animations
        │   └── Hamster.js            # Subagent (Task tool) entity
        ├── ui/
        │   ├── detailFlyout.js       # Side panel with session details
        │   ├── sessionTracker.js     # Session lifecycle + sound triggering
        │   ├── formatters.js         # Number/time formatting
        │   └── ambientAudio.js       # Background sound management
        ├── audio/
        │   └── SoundEngine.js        # Web Audio API synthesizer
        └── gamification/
            ├── BattlePassBar.js      # XP progress visualization
            ├── AchievementPanel.js   # Achievement display
            ├── UnlockToast.js        # Canvas toast notifications
            ├── CosmeticRegistry.js   # Equipped cosmetics state
            └── RewardSelector.js     # Garage UI for cosmetic selection
```

**No build tools for the frontend.** Vanilla JS with ES modules, served directly. The backend is a single Go binary with minimal dependencies (`gorilla/websocket`, `gopsutil`, `yaml.v3`).

**Frontend embedding:** The `frontend/` directory is the single source of truth. During builds (`make build`), it's copied to `backend/internal/frontend/static/` as a build artifact (git-ignored) for Go's embed directive.

## Rendering Pipeline

The frontend uses a Canvas-based rendering loop at 60fps:

1. Background fill (dark navy #1a1a2e)
2. Track surfaces (asphalt, lane dividers, markers)
3. Pit area and parking lot
4. Particles (behind layer — exhaust, smoke)
5. Racers (Y-sorted for proper layering)
6. Particles (front layer — sparks, confetti)
7. Glow/bloom composite pass (1/4 resolution offscreen canvas, additive blend)
8. Flash overlay (on completion events)
9. Dashboard stats and leaderboard
10. UI overlays (connection status, empty state)

## Audio Architecture

All sound is synthesized in real time using the Web Audio API — no audio files.

```
AudioContext
├── Master Gain
│   ├── Ambient Bus
│   │   ├── Crowd (bandpass-filtered white noise + LFO modulation)
│   │   ├── Wind (lowpass-filtered brown noise + gusts)
│   │   └── Engine Bus (compressor -24dB threshold)
│   │       └── Per-Racer Hums (hash-derived 65-95Hz base frequency)
│   │
│   └── SFX Bus
│       ├── Victory fanfare (3-chord with reverb convolver)
│       ├── Crash (noise burst + descending sawtooth + rumble)
│       ├── Gear shift / Tool click
│       ├── Appear / Disappear whoosh
│       └── Unlock chimes (tier-scaled pitch)
```

The excitement system dynamically adjusts crowd volume based on the number of active sessions, sessions near the finish line, and recent completion/error events.

## Design Principles

See `CLAUDE.md` for engineering principles. Key points:

- **Source interface** — New agent sources are added by implementing the `Source` interface, not by modifying existing code
- **Shared parsing** — JSONL parsing lives in `monitor/jsonl.go`, not duplicated per source
- **State-driven rendering** — The frontend derives all visuals from session state. No rendering hints in backend structs, no branching on source name
- **XDG compliance** — Config in `~/.config/agent-racer/`, state in `~/.local/state/agent-racer/`
