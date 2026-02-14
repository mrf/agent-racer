# Gamification & Rewards Plan

## Goal

Add a battle-pass-style achievement and rewards system that gives users goals to chase while using Agent Racer. Achievements unlock cosmetic rewards (car skins, trail effects, sounds) that make the racetrack more personal and visually distinct as users hit milestones.

## Why

The racetrack visualization is inherently spectator-mode -- you watch sessions race. Gamification turns passive watching into active engagement: users start running sessions in new ways (try Gemini, run 10 concurrent sessions, push a session to max context) just to unlock the next achievement. It also provides a persistent sense of progression across sessions.

## Data Already Available

Everything needed to drive achievements is already tracked in `SessionState`:

| Signal | Source field | Example achievements |
|--------|-------------|---------------------|
| Session count | Cumulative sessions seen | "10 Sessions", "100 Sessions" |
| Agent source | `Source` ("claude", "gemini", "codex") | "First Gemini Run", "Triple Threat" |
| Model name | `Model` | "Opus Enthusiast", "Model Collector" |
| Model family/size | Derived from model string | "Full Spectrum" (used all sizes) |
| Context utilization | `ContextUtilization` | "Redline" (hit 95%+) |
| Concurrent active | Count of Thinking/ToolUse sessions | "Grid Full" (10+ simultaneous) |
| Tool call count | `ToolCallCount` | "Tool Fiend" (500+ tools in one session) |
| Completions | `Activity == Complete` | "Clean Finish", "Victory Lap" |
| Errors | `Activity == Errored` | "Crash Survivor", "Phoenix" |
| Burn rate | `BurnRatePerMinute` | "Afterburner" (high burn rate) |
| Session duration | `StartedAt` to `CompletedAt` | "Marathon" (2h+ session) |
| Message count | `MessageCount` | "Conversationalist" |

## Design

### Achievement System

Each achievement has:

```
ID:          stable string key (e.g., "sessions_10")
Name:        display name ("Pit Crew")
Description: how to earn it ("Run 10 sessions")
Icon:        emoji or small sprite
Tier:        bronze / silver / gold / platinum
Category:    grouping for the UI
Condition:   function(stats) -> bool
Reward:      what unlocks (cosmetic ID)
```

#### Categories & Achievements

**Session Milestones** -- cumulative session count across all sources.

| Achievement | Condition | Tier | Reward |
|------------|-----------|------|--------|
| First Lap | 1 session observed | Bronze | Default unlocked |
| Pit Crew | 10 sessions | Bronze | Paint: Matte Black |
| Veteran Driver | 50 sessions | Silver | Trail: Blue Flame |
| Century Club | 100 sessions | Gold | Paint: Chrome |
| Track Legend | 500 sessions | Platinum | Paint: Holographic |

**Source Diversity** -- using different agent CLIs.

| Achievement | Condition | Tier | Reward |
|------------|-----------|------|--------|
| Home Turf | Run 5 Claude sessions | Bronze | Badge: Claude logo |
| Gemini Rising | First Gemini session | Bronze | Paint: Gemini Teal |
| Codex Curious | First Codex session | Bronze | Paint: Codex Green |
| Triple Threat | Use all 3 sources | Silver | Trail: Rainbow |
| Polyglot | 10+ sessions from each source | Gold | Car body: Formula style |

**Model Collection** -- variety of models used.

| Achievement | Condition | Tier | Reward |
|------------|-----------|------|--------|
| Opus Enthusiast | 5 sessions with any Opus model | Bronze | Paint: Deep Purple |
| Sonnet Fan | 5 sessions with any Sonnet model | Bronze | Paint: Electric Blue |
| Haiku Speedster | 5 sessions with any Haiku model | Bronze | Paint: Lime Green |
| Full Spectrum | Use Opus + Sonnet + Haiku in same day | Silver | Trail: Prismatic |
| Model Collector | Use 5+ distinct model IDs | Silver | Badge: Collector |
| Connoisseur | Use 10+ distinct model IDs | Gold | Car body: Luxury sedan |

**Performance & Endurance** -- session behavior milestones.

| Achievement | Condition | Tier | Reward |
|------------|-----------|------|--------|
| Redline | Any session hits 95%+ context utilization | Bronze | Trail: Red Sparks |
| Afterburner | Burn rate exceeds 5K tokens/min | Silver | Trail: Afterburn |
| Marathon | Single session runs 2+ hours | Silver | Paint: Endurance Orange |
| Tool Fiend | Single session with 500+ tool calls | Silver | Badge: Wrench |
| Conversationalist | Single session with 200+ messages | Bronze | Badge: Chat Bubble |
| Clean Sweep | 10 sessions complete without errors | Silver | Paint: Racing Stripe |
| Photo Finish | Two sessions complete within 10s of each other | Gold | Trail: Confetti Burst |

**Spectacle** -- concurrent and dramatic moments.

| Achievement | Condition | Tier | Reward |
|------------|-----------|------|--------|
| Grid Start | 3+ sessions racing simultaneously | Bronze | Sound: Engine Rev |
| Full Grid | 5+ sessions racing simultaneously | Silver | Sound: Crowd Roar |
| Grid Full | 10+ sessions racing simultaneously | Gold | Track theme: Stadium |
| Crash Survivor | Session errors then a new session completes | Bronze | Badge: Phoenix |
| Burning Rubber | 3+ sessions all above 50% context | Silver | Trail: Tire Smoke |

**Streaks** -- consecutive session patterns.

| Achievement | Condition | Tier | Reward |
|------------|-----------|------|--------|
| Hat Trick | 3 completions in a row (no errors) | Bronze | Badge: Star x3 |
| On a Roll | 10 completions in a row | Silver | Paint: Gold Stripe |
| Untouchable | 25 completions in a row | Gold | Car body: Armored |

### Battle Pass (Seasonal Challenges)

A rotating set of weekly/monthly challenges layered on top of permanent achievements. Each challenge awards XP that fills a progress bar through tiers.

**XP Sources:**

| Action | XP |
|--------|-----|
| Session observed | 10 |
| Session completes | 25 |
| Session hits 50% context | 15 |
| Session hits 90% context | 30 |
| Use a new model (first time ever) | 50 |
| Use a new source (first time ever) | 100 |
| Achievement unlocked | 50-200 (by tier) |
| Weekly challenge completed | 150 |

**Weekly Challenge Examples:**

- "Run 5 sessions using Haiku models"
- "Complete 3 sessions without any errors"
- "Hit 90% context utilization twice"
- "Use 3 different models in one day"
- "Burn 1M total tokens this week"

**Battle Pass Tiers** (10 tiers per season, ~1000 XP each):

| Tier | Reward |
|------|--------|
| 1 | Paint: Season color base |
| 2 | Badge: Season emblem |
| 3 | Trail: Season particle |
| 4 | Sound: Season horn |
| 5 | Paint: Season color metallic |
| 6 | Car body: Season special |
| 7 | Track theme: Season backdrop |
| 8 | Trail: Season premium particle |
| 9 | Paint: Season color animated |
| 10 | Title: Season champion |

Seasons can be time-based (monthly) or manually rotated via config. The first version can skip seasons entirely and just ship permanent achievements.

### Reward Types

All rewards are cosmetic and rendered on the existing Canvas 2D racetrack.

**Paints** -- change the car's base fill color or add patterns.

- Solid colors (matte black, chrome, teal, etc.)
- Patterns (racing stripe, gradient, animated shimmer)
- Implementation: `Racer.js` already has model-based color logic; extend with a `paintOverride` that takes precedence when set.

**Trails** -- modify the exhaust/particle system behind the car.

- Color changes (blue flame, red sparks, rainbow)
- Shape changes (confetti, tire smoke, prismatic)
- Implementation: `Particles.js` already supports multiple particle types; add new presets keyed by trail ID.

**Car Bodies** -- swap the polygon shape of the car.

- Formula (low, wide), Luxury (longer), Armored (angular)
- Implementation: `Racer.js` draws the car body as a polygon; add alternate polygon definitions.

**Badges** -- small icons rendered next to the session name.

- Emoji-based initially (easy), sprite-based later
- Implementation: render in the name label area above the car.

**Sounds** -- unlock alternate SFX.

- Horn variants, engine tone changes
- Implementation: `SoundEngine.js` already has SFX infrastructure; add loadable sound sets.

**Track Themes** -- change the track background/atmosphere.

- Stadium (lights, stands), Night (dark + neon), Retro (pixel art)
- Implementation: `Track.js` background rendering; swap color palette + optional overlay.

**Titles** -- text shown on the leaderboard next to the user's name.

- "Track Legend", "Season 1 Champion", etc.
- Implementation: prepend to session name in `Dashboard.js` leaderboard.

## Persistence

### Stats Store

Achievement progress requires persistent stats across app restarts. Store in XDG state directory:

```
~/.local/state/agent-racer/stats.json
```

Schema:

```json
{
  "version": 1,
  "totalSessions": 142,
  "totalCompletions": 98,
  "totalErrors": 12,
  "consecutiveCompletions": 7,
  "sessionsPerSource": {
    "claude": 130,
    "gemini": 8,
    "codex": 4
  },
  "sessionsPerModel": {
    "claude-opus-4-5-20251101": 45,
    "claude-sonnet-4-20250514": 60,
    "claude-haiku-4-5-20251001": 25,
    "gemini-2.5-pro": 8,
    "codex-mini": 4
  },
  "distinctModelsUsed": ["claude-opus-4-5-20251101", "..."],
  "distinctSourcesUsed": ["claude", "gemini", "codex"],
  "maxContextUtilization": 0.97,
  "maxBurnRate": 6200.5,
  "maxConcurrentActive": 8,
  "maxToolCalls": 612,
  "maxMessages": 245,
  "maxSessionDurationSec": 8400,
  "achievementsUnlocked": {
    "sessions_10": "2025-06-15T10:30:00Z",
    "gemini_first": "2025-06-20T14:00:00Z"
  },
  "battlePass": {
    "season": "2025-07",
    "xp": 4200,
    "tier": 4,
    "challengesCompleted": ["weekly_haiku_5"]
  },
  "equipped": {
    "paint": "matte_black",
    "trail": "blue_flame",
    "body": "default",
    "badge": "claude_logo",
    "sound": "default",
    "title": ""
  },
  "lastUpdated": "2025-06-22T09:00:00Z"
}
```

### Update Flow

1. Monitor loop detects session state change (new session, completion, error, etc.)
2. Backend `StatsTracker` updates cumulative stats
3. `AchievementEngine` evaluates all locked achievements against current stats
4. Newly unlocked achievements emit a WebSocket event
5. Frontend shows unlock notification (toast + sound)
6. Stats file written to disk (debounced, every 30s or on shutdown)

## Architecture

### Backend

```
backend/internal/
├── gamification/
│   ├── stats.go          # StatsTracker: cumulative stat collection
│   ├── achievements.go   # AchievementEngine: condition evaluation
│   ├── battlepass.go     # BattlePass: XP, tiers, weekly challenges
│   ├── rewards.go        # Reward registry: all unlockable cosmetics
│   └── persistence.go   # Load/save stats.json (XDG state dir)
```

- `StatsTracker` subscribes to session state changes from the monitor loop.
- `AchievementEngine` runs after each stats update, checking conditions.
- New WebSocket message types: `achievement_unlocked`, `stats_update`, `battlepass_progress`.
- New REST endpoints: `GET /api/achievements`, `GET /api/stats`, `POST /api/equip`.

### Frontend

```
frontend/src/
├── gamification/
│   ├── AchievementPanel.js   # Achievement grid UI (canvas-rendered)
│   ├── BattlePassBar.js      # XP progress bar + tier display
│   ├── UnlockToast.js        # Pop-up notification on unlock
│   ├── RewardSelector.js     # Equip cosmetics UI
│   └── CosmeticRegistry.js   # Maps reward IDs to render params
```

- `Racer.js` reads equipped cosmetics from a global state to override paint/trail/body.
- `Particles.js` adds new particle presets for trail rewards.
- `SoundEngine.js` loads alternate SFX sets when equipped.
- New keyboard shortcut: `A` to toggle achievement panel.

### WebSocket Protocol Additions

```json
{
  "type": "achievement_unlocked",
  "payload": {
    "id": "gemini_first",
    "name": "Gemini Rising",
    "description": "Complete your first Gemini CLI session",
    "tier": "bronze",
    "reward": {"type": "paint", "id": "gemini_teal", "name": "Gemini Teal"}
  }
}
```

```json
{
  "type": "battlepass_progress",
  "payload": {
    "xp": 4350,
    "tier": 4,
    "tierProgress": 0.35,
    "recentXP": [
      {"reason": "Session completed", "amount": 25}
    ]
  }
}
```

## Implementation Phases

### Phase 1: Stats Tracking + Persistence

- Add `gamification/` package to backend
- Implement `StatsTracker` that hooks into the monitor loop
- Implement `persistence.go` for `stats.json` read/write
- Track: session counts, source/model diversity, peak metrics
- No frontend changes yet; verify via `GET /api/stats`

### Phase 2: Permanent Achievements

- Implement `AchievementEngine` with all permanent achievements
- Add `achievement_unlocked` WebSocket event
- Frontend: `UnlockToast.js` notification popup (canvas overlay)
- Frontend: `AchievementPanel.js` grid (toggle with `A` key)
- Ship the full achievement list from the tables above

### Phase 3: Cosmetic Rewards

- Implement `CosmeticRegistry` (frontend) and `rewards.go` (backend)
- Extend `Racer.js` with paint overrides
- Extend `Particles.js` with trail presets
- Add alternate car body polygons
- Add badge rendering in name labels
- Frontend: `RewardSelector.js` equip UI
- Backend: `POST /api/equip` endpoint

### Phase 4: Battle Pass

- Implement `battlepass.go` with XP calculations and tier logic
- Add weekly challenge definitions (config-driven or hardcoded initial set)
- Frontend: `BattlePassBar.js` progress display
- Season rotation logic (manual config flag initially)

## Open Questions

- **Multi-user?** Current design is single-user (one `stats.json`). If Agent Racer ever supports multiple viewers, stats would need to be per-user or shared.
- **Reset/prestige?** Should there be a way to reset stats and re-earn achievements at a harder difficulty?
- **Export?** Should achievements be exportable/shareable (screenshot, JSON)?
- **Sound budget?** Adding many new SFX could bloat the binary. Consider lazy-loading audio assets.
- **Offline challenges?** Battle pass challenges currently require the app to be running. Should we retroactively scan logs on startup to credit missed sessions?
