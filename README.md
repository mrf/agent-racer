# Agent Racer

A real-time racing visualization of all active Claude Code sessions on your machine. Each session is a car on a racetrack, and its position is driven by context window utilization — as a conversation consumes more tokens, the car advances toward the finish line.

![Agent Racer dashboard showing multiple Claude sessions racing on a track](docs/screenshot.png)

Sessions are discovered automatically via process scanning. State is read directly from Claude Code's JSONL session files. No configuration, wrappers, or hooks required — just start Claude Code sessions anywhere on your machine and watch them race.

## Quick Start

**Prerequisites:** Go 1.22+, a modern browser. Linux or macOS for real mode.

**Localhost-only:** Agent Racer is intended to run on your local machine only. It is not designed or supported for public or multi-user deployment.

```bash
# Clone and run in mock mode (demo with 5 simulated sessions)
cd agent-racer
make dev
# Open http://localhost:8080
```

```bash
# Run with real session monitoring
make run
# Open http://localhost:8080, then start Claude Code sessions in other terminals
```

```bash
# Build a single binary with embedded frontend
make build
./agent-racer --mock    # demo mode
./agent-racer           # real mode
```

## Installing

```bash
# Option 1: Build and install manually
make build
cp agent-racer /usr/local/bin/

# Option 2: Use the install script (also sets up SessionEnd hook)
./scripts/install.sh
```

## CLI Flags

```
Usage: agent-racer [flags]

  --mock            Use mock session data (demo mode)
  --dev             Serve frontend from filesystem (for development)
  --config string   Path to config file (default: ~/.config/agent-racer/config.yaml)
  --port int        Override server port
```

## What You'll See

### The Track

A dark asphalt racing surface where each active session gets its own lane:

- **Start line** (left) — checkerboard pattern at 0 tokens
- **Finish line** (right) — red checkerboard at max context (200K tokens for Claude)
- **Dotted markers** at 50K, 100K, and 150K tokens for reference
- **Pit area** below the track holds sessions that are idle or waiting for input

### The Cars

Each session appears as a car. Here's what the visual elements mean:

| Element | What it tells you |
|---------|-------------------|
| **Car color** | The model: Purple = Opus, Blue/Cyan = Sonnet, Green = Haiku, Teal = Codex, Blue = Gemini |
| **Horizontal position** | How much of the context window has been used (0% → 100%) |
| **Name label** (above car) | Session name, derived from the working directory |
| **Model badge** | Small colored badge showing the model family |
| **Token counter** (below car) | Current / max tokens (e.g. `142K/200K`) |
| **Tool indicator** (below car) | Name of the tool currently being used (Read, Edit, Bash, etc.) |

### Activity Animations — What Claude Is Doing

The car's visual effects change in real time to reflect what the agent is doing. This is the key to understanding session state at a glance:

| What you see | What Claude is doing |
|--------------|---------------------|
| **Car moving + exhaust particles + thought bubble** | **Thinking** — Claude is generating a response, reasoning through the problem |
| **Sparks flying + tool name displayed** | **Tool Use** — Claude is actively calling a tool (Read, Edit, Bash, Grep, etc.). The tool name appears below the car |
| **Amber hazard lights blinking** | **Waiting** — Session is paused, waiting for user input or approval |
| **Car stationary, no effects** | **Idle** — No recent activity; session may be between turns |
| **Subtle wheel rotation + occasional exhaust puff** | **Churning** — The process is active (CPU usage detected) but hasn't produced output yet. Often means Claude is processing a large context or preparing a response |
| **Trophy icon + confetti explosion** | **Complete** — Session finished successfully! |
| **Spin-out animation + smoke particles** | **Errored** — Session hit an error or crashed |
| **Car fades to transparent** | **Lost** — Session process disappeared without a clean exit |

### Pit Lane

Cars that aren't actively working (idle, waiting, or starting) automatically roll off the main track into the **pit area** below. This keeps the racing lanes uncluttered:

- Cars smoothly animate between track and pit with opacity/scale dimming
- When a session becomes active again (thinking, tool use, or churning), the car drives back onto the track
- Terminal states (complete, errored, lost) stay on track for their exit animations before being removed

### Sound Effects — What You'll Hear

All audio is synthesized in real time — no sound files, just the Web Audio API. Sounds give you an audio layer of awareness for what's happening across sessions, even when you're not looking at the dashboard.

#### Ambient Background

| Sound | What it means |
|-------|---------------|
| **Crowd murmur** | Background ambiance that grows louder as more sessions are actively working. A quiet murmur = few active sessions; a loud crowd = lots of activity |
| **Wind / brown noise** | Subtle background texture with occasional gusts every 5-15 seconds |
| **Engine hums** | Each active racer has its own low-frequency hum. The pitch varies: normal during thinking, higher during tool use, lower and quieter during churning |

#### One-Shot Sound Effects

| Sound | When it plays |
|-------|---------------|
| **Victory fanfare** | A three-chord celebratory chime (C-E-G → G-B-D → C-E-G) with reverb — a session completed successfully |
| **Crash** | A noise burst + descending sawtooth + low rumble — a session errored out |
| **Gear shift** | A quick rising tone (300→600Hz) — Claude switched between thinking and tool use |
| **Tool click** | A short high-pitched click — Claude started using a tool |
| **Appear whoosh** | A rising filtered noise sweep — a new session appeared on the track |
| **Disappear whoosh** | A falling filtered noise sweep — a session left the track |
| **Unlock chime** | A two-note chime (pitch varies by tier: bronze, silver, gold, platinum) — an achievement was unlocked |

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `D` | Toggle debug panel (shows raw WebSocket messages) |
| `M` | Toggle sound effects (mute/unmute) |
| `F` | Toggle fullscreen |
| `Esc` | Close detail panel |

### Detail Panel

**Click on any car** to open a side panel with full session details:

- Current activity and token progress bar
- Model name and working directory
- Git branch
- Message count and tool call count
- Current tool being used
- Timestamps (started, last activity)
- PID and session ID
- Subagents (if the session spawned Task tool invocations)

## Session Lifecycle

Here's what happens from the moment you start a Claude Code session to when it leaves the track:

1. **Discovery** — Agent Racer scans for running Claude Code processes and finds their JSONL session files in `~/.claude/projects/`
2. **Appears on track** — A new car appears with an "appear whoosh" sound
3. **Racing** — As Claude responds and uses tokens, the car advances. Activity animations show real-time state
4. **Pit stops** — When waiting for your input, the car rolls into the pit lane
5. **Back to racing** — When you respond and Claude starts working again, the car returns to the track
6. **Finish** — When the session ends cleanly (via SessionEnd hook or inactivity timeout), a completion animation plays with confetti and victory fanfare
7. **Removal** — After the animation, the car is removed from the track

### SessionEnd Hook (Recommended)

For instant completion detection, install the SessionEnd hook. This tells Agent Racer the moment a session ends, rather than waiting for an inactivity timeout:

```bash
# The install script sets this up automatically:
./scripts/install.sh
```

If you manage hooks manually, add a `SessionEnd` hook that runs:
```bash
~/.config/agent-racer/hooks/session-end.sh
```

Completion markers are written to `~/.local/state/agent-racer/session-end/` by default (XDG state directory). Configure `monitor.session_end_dir` if you want a different path.

## Mock Mode

Mock mode (`--mock`) simulates 5 sessions with distinct behaviors for demo and testing:

| Session | Model | Behavior |
|---------|-------|----------|
| **opus-refactor** | Opus | Steady token growth to 180K, completes successfully |
| **sonnet-tests** | Sonnet | Burst pattern (fast writes), completes quickly |
| **opus-debug** | Opus | Stalls mid-conversation (waiting for user input), then resumes |
| **sonnet-feature** | Sonnet | Errors out at ~60% context utilization |
| **opus-review** | Opus | Slow and methodical, heavy tool use (Read, LSP, Grep) |

This is a great way to explore the dashboard, see all the animations, and hear all the sounds without running real sessions.

## Multi-Agent Support (Pre-Alpha)

Agent Racer has early support for monitoring **OpenAI Codex CLI** and **Google Gemini CLI** sessions alongside Claude Code. This is **pre-alpha** — expect bugs with progress tracking, model labels, and session lifecycle. Both sources are disabled by default.

To opt in, enable them in your config:

```yaml
sources:
  codex: true
  gemini: true
```

See `docs/multi-agent-guide.md` for detailed documentation on supported CLIs, configuration options, and a manual validation checklist.

## Configuration

Agent Racer follows the **XDG Base Directory Specification**. The default config location is:

```
~/.config/agent-racer/config.yaml
```

You can override this with `--config` or set `XDG_CONFIG_HOME` to use a custom config directory.

See `config.example.yaml` for a complete example. Key configuration options:

```yaml
server:
  port: 8080          # HTTP/WebSocket port
  host: "127.0.0.1"   # Bind address (localhost by default)

sources:
  claude: true        # Claude Code session monitoring (default: true)
  codex: false        # OpenAI Codex CLI monitoring (default: false, pre-alpha)
  gemini: false       # Google Gemini CLI monitoring (default: false, pre-alpha)

monitor:
  poll_interval: 1s                # How often to scan for processes and read JSONL
  snapshot_interval: 5s            # Full state broadcast interval
  broadcast_throttle: 100ms        # Minimum time between delta broadcasts
  session_stale_after: 2m          # Mark sessions complete after no new data
  completion_remove_after: 8s      # Remove racers after completion animation
  session_end_dir: ""              # Defaults to $XDG_STATE_HOME/agent-racer/session-end
  churning_cpu_threshold: 15.0     # CPU% for detecting active processing
  churning_requires_network: false # Require TCP connections for churning state

models:
  claude-opus-4-5-20251101: 200000
  claude-sonnet-4-5-20250929: 200000
  gpt-5-codex: 272000              # Codex models
  gemini-2.5-pro: 1048576          # Gemini models (1M tokens)
  default: 200000                  # Fallback for unrecognized models

sound:
  enabled: true           # Master enable/disable
  master_volume: 1.0      # 0.0 - 1.0
  ambient_volume: 1.0     # Crowd, wind, engine hums
  sfx_volume: 1.0         # Gear shifts, victory, crashes
  enable_ambient: true
  enable_sfx: true

privacy:
  mask_working_dirs: false   # Show only last path component
  mask_session_ids: false    # Replace with opaque hashes
  mask_pids: false           # Hide process IDs
  allowed_paths: []          # Glob allowlist (empty = all sessions shown)
  blocked_paths: []          # Glob blocklist
```

If no config file exists, agent-racer uses sensible defaults. See `docs/configuration.md` for detailed documentation.

## Make Targets

| Target | Description |
|--------|-------------|
| `make dev` | Run mock mode with filesystem frontend (hot-reload friendly) |
| `make run` | Run real mode with filesystem frontend fallback |
| `make build` | Embed frontend into Go binary, produce `./agent-racer` |
| `make dist` | Cross-compile for linux/darwin amd64/arm64 |
| `make test` | Run all Go tests |
| `make test-frontend` | Run frontend Vitest suite |
| `make test-e2e` | Run Playwright E2E tests |
| `make lint` | Run `go vet` on backend |
| `make ci` | Run all checks: test, lint, test-frontend, test-e2e |
| `make clean` | Remove binary and embedded files |
| `make deps` | Download Go dependencies |

## Requirements

- **Go 1.22+** for building
- **Linux or macOS** for real mode (process discovery via gopsutil)
- Mock mode works on any platform
- Modern browser with Canvas support

## Architecture

For technical architecture, WebSocket API, directory structure, and contributor information, see [docs/architecture.md](docs/architecture.md).

## License

[MIT](LICENSE)
