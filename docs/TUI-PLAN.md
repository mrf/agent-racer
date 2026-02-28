# TUI Frontend for Agent Racer

## Context

Agent Racer has a Go backend that broadcasts real-time AI session data over WebSocket and a canvas-based web frontend that renders it as a racing visualization. To prove the backend is truly UI-agnostic, we're building a second frontend: a full-featured TUI using the Charmbracelet stack. The TUI will be a standalone Go binary at `tui/` that connects to the same backend WebSocket — it does NOT import backend packages, only mirrors the wire protocol types.

## Tech Stack

- **Bubble Tea** — Elm-architecture TUI framework
- **Lip Gloss** — Declarative terminal styling
- **Bubbles** — Pre-built components (table, progress, spinner, viewport)
- **gorilla/websocket** — WebSocket client

## Package Structure

```
tui/
├── cmd/racer-tui/
│   └── main.go                 # Entry point, flags (--url, --token)
├── internal/
│   ├── app/
│   │   ├── app.go              # Root Bubble Tea model, dispatch, overlay mgmt
│   │   ├── keys.go             # Keymap definitions
│   │   └── theme.go            # Lip Gloss color palette + reusable styles
│   ├── client/
│   │   ├── ws.go               # WebSocket client, reconnect, seq tracking
│   │   ├── http.go             # REST client (stats, achievements, challenges, equip, focus)
│   │   └── types.go            # Wire protocol types (mirrors backend JSON, no imports)
│   └── views/
│       ├── track/
│       │   ├── track.go        # 3-zone ASCII race track (racing/pit/parked)
│       │   ├── racer.go        # Racer glyph rendering with model colors
│       │   └── zone.go         # Zone classification logic
│       ├── dashboard/
│       │   └── dashboard.go    # Stats row + leaderboard table
│       ├── detail/
│       │   └── detail.go       # Session detail panel
│       ├── achievements/
│       │   └── achievements.go # Achievement modal
│       ├── battlepass/
│       │   └── battlepass.go   # BP bar + expanded view
│       ├── garage/
│       │   └── garage.go       # Reward selector
│       ├── debug/
│       │   └── debug.go        # Scrollable event log
│       └── status/
│           └── status.go       # Connection + source health bar
├── go.mod
└── go.sum
```

## ASCII Race Track Design

```
╔═ STATUS ═══════════════════════════════════════════════════════════╗
║ ● Connected  │  3 racing  1 pit  2 parked  │  claude: ok          ║
╚════════════════════════════════════════════════════════════════════╝
┌─ BATTLE PASS ─── Season 1 ── Tier 3 ── [████████░░] 750/1000 XP ─┐

═══ TRACK (200K) ══════════════════════════════════════════════ FINISH
  1│ ▸ fix-login ···············●>·····························│ 62%
  2│ ▸ add-cache ·······●>·····································│ 35%
─── PIT ───────────────────────────────────────────────────────────
  3│ ○ run-tests ···○···                                       │ idle
─── PARKED ────────────────────────────────────────────────────────
  4│ ✓ refactor-db                                             │ done
  5│ ✗ deploy-fix                                              │ error

┌─ DASHBOARD ────────────────────────────────────────────────────────┐
│ RACING: 2   PIT: 1   PARKED: 2   TOKENS: 145K   TOOLS: 23        │
├────────────────────────────────────────────────────────────────────┤
│  #  SESSION          MODEL      TOKENS  CONTEXT          %  TIME  │
│ >1  fix-login        opus-4      28K    [████████░░]    62%  12m  │
│  2  add-cache        sonnet-4    15K    [████░░░░░░]    35%   8m  │
└────────────────────────────────────────────────────────────────────┘
```

## Racer Glyphs

- `●>` thinking
- `⚙>` tool_use
- `○` idle
- `◌` waiting
- `◎` starting
- `✓` complete
- `✗` errored
- `?` lost

## Zone Classification

Matches `frontend/src/canvas/RaceCanvas.js:33-50`:
- **Parked**: activity in {complete, errored, lost}
- **Pit**: activity in {idle, waiting, starting} AND lastDataReceivedAt age > 30s
- **Racing**: everything else

## Color Scheme

| Model | Hex |
|---|---|
| Opus | `#a855f7` |
| Sonnet 4 | `#3b82f6` |
| Sonnet 4.5 | `#06b6d4` |
| Haiku | `#22c55e` |
| Gemini | `#4285f4` |
| Codex | `#10b981` |

| Source | Badge |
|---|---|
| claude | `[C]` purple |
| codex | `[X]` green |
| gemini | `[G]` blue |

Context bar: green (<50%) → amber (50-80%) → red (>80%)
Tier colors: bronze `#d97706`, silver `#9ca3af`, gold `#f59e0b`, platinum `#67e8f9`

## Keyboard Navigation

| Key | Action | Key | Action |
|---|---|---|---|
| `j/↓` | Next racer | `a` | Toggle achievements |
| `k/↑` | Prev racer | `g` | Toggle garage |
| `Enter` | Detail / tmux focus | `d` | Toggle debug |
| `Tab` | Cycle zone | `b` | Toggle battle pass |
| `1-3` | Jump to zone | `r` | Force resync |
| `Esc` | Close overlay | `q` | Quit |

## Dependency Graph

```
Epic: TUI Frontend
  │
  ├── Phase 1: Foundation
  │     └── Phase 2: Track + Racers
  │           └── Phase 3: Dashboard
  │                 ├── Phase 4: Detail Panel    ─┐
  │                 ├── Phase 5: Battle Pass     ─┤
  │                 ├── Phase 6: Achievements    ─┤ (parallel)
  │                 └── Phase 7: Garage          ─┤
  │                                               └── Phase 8: Polish
  │                                                     └── Phase 9: Build
  └──────────────────────────────────────────────────────────────────────
```

## Key Files to Reference

- `backend/internal/ws/protocol.go` — WS message types + payloads
- `backend/internal/session/state.go` — SessionState, SubagentState, Activity
- `backend/internal/gamification/persistence.go` — Stats, Equipped, BattlePass
- `backend/internal/gamification/stats.go` — XPEntry
- `backend/internal/ws/server.go:85-101` — HTTP route registration
- `frontend/src/canvas/RaceCanvas.js:33-50` — zone classification logic
- `frontend/src/canvas/Dashboard.js` — leaderboard layout reference
