# Subagent Visualization: Hamsters on Skateboards

## Context

When Claude Code spawns subagents via the Task tool, those subagents are logged as `type:"progress"` entries in the **same JSONL file** as the parent session. The data includes `toolUseID` (stable subagent ID), `parentToolUseID` (link to parent), `slug` (display name), model, tokens, and activity. Our parser currently ignores these entries. This feature parses them and renders each subagent as a hamster riding a skateboard, towed behind the parent car.

## Research Findings

### Data source: `type: "progress"` entries

Claude Code logs subagent activity as `progress` entries in the **same JSONL file** as the parent session. Each subagent turn produces one `progress` entry. These are currently ignored by our parser (`jsonl.go` only handles `assistant` and `user` types).

### Available fields per `progress` entry

| Field | Example | Purpose |
|---|---|---|
| `type` | `"progress"` | Entry type (filter key) |
| `toolUseID` | `"agent_msg_01F8z3..."` | Stable subagent identifier (same across all entries for one subagent) |
| `parentToolUseID` | `"toolu_0167Bs..."` | Links to parent session's `tool_use` block that spawned this subagent |
| `sessionId` | same as parent | Shared session file (no separate JSONL) |
| `slug` | `"dynamic-chasing-bunny"` | Human-friendly subagent name |
| `parentUuid` | UUID | Previous message in the subagent chain |
| `uuid` | UUID | This message's ID |
| `timestamp` | RFC3339Nano | When this turn occurred |
| `cwd` | `"/home/mrf/Projects/..."` | Working directory |
| `data.message.type` | `"assistant"` or `"user"` | Turn type within subagent |
| `data.message.message.model` | `"claude-haiku-4-5-20251001"` | Subagent model (often different from parent) |
| `data.message.message.content[]` | tool_use / tool_result / text blocks | What the subagent is doing |
| `data.message.message.usage` | `{input_tokens, output_tokens, ...}` | Token consumption |

### Key observations

- Subagents share the parent's `sessionId` and JSONL file -- they do **not** create separate session files.
- The `toolUseID` is consistent across all progress entries for one subagent invocation, making it a natural grouping key.
- The `parentToolUseID` matches the `id` field of the parent's `tool_use` content block (where `name: "Task"`), establishing the parent-child link.
- The `slug` field appears after the first turn and provides a display name.
- Subagents can use different models (e.g., parent on Opus, subagent on Haiku).
- Multiple subagents can run in parallel (each with a distinct `toolUseID`).

## Phased Implementation

### Phase 1: Backend — SubagentState + JSONL Parsing

**Files:** `backend/internal/session/state.go`, `backend/internal/monitor/jsonl.go`, `backend/internal/monitor/source.go`, `backend/internal/monitor/claude_source.go`, `backend/internal/monitor/monitor.go`

**1a. Data model** (`state.go`)
- Add `SubagentState` struct: `ID`, `ParentToolUseID`, `SessionID`, `Slug`, `Model`, `Activity` (reuse existing enum), `CurrentTool`, `TokensUsed`, `MessageCount`, `ToolCallCount`, `StartedAt`, `LastActivityAt`, `CompletedAt`
- Add `Subagents []SubagentState` field on `SessionState`

**1b. JSONL parser** (`jsonl.go`)
- Add `progressEntry` struct to parse `type:"progress"` entries with `toolUseID`, `parentToolUseID`, `slug`, `data`
- Add `SubagentParseResult` struct and `Subagents map[string]*SubagentParseResult` on `ParseResult`
- New `"progress"` case in `ParseSessionJSONL`: filter for `data.type == "agent_progress"`, extract model/usage/content from `data.message.message` (same logic as assistant messages), group by `toolUseID`
- Completion detection: in the `"user"` case, scan `tool_result` content blocks — if `tool_use_id` matches a subagent's `parentToolUseID`, mark that subagent complete

**1c. SourceUpdate extension** (`source.go`)
- Add `Subagents map[string]*SubagentParseResult` to `SourceUpdate` (internal, not serialized)
- Update `HasData()` to check `len(u.Subagents) > 0`

**1d. Monitor propagation** (`monitor.go`)
- Add `mergeSubagents(existing []SubagentState, incoming map[string]*SubagentParseResult) []SubagentState` helper
- On each poll: get existing subagents from store, merge incoming data (accumulate counts, update activity/tool/timestamps), append new subagents
- Convert and attach to `SessionState.Subagents` before store update

**1e. WebSocket — zero changes needed.** `Subagents` field auto-serializes as nested JSON.

---

### Phase 2: Frontend — Hamster Entity

**Files:** New `frontend/src/entities/Hamster.js`

**Visual design** (canvas 2D, ~40% scale of parent car):
- **Skateboard**: rounded rect deck (wood brown), 4 tiny wheels with spinning spokes
- **Hamster body**: warm brown ellipse with model-color helmet/harness, round ears (pink interior), dot eyes with highlights, tiny pink nose, small paws gripping deck edges, curved tail with wag animation
- **Activity indicators** (simplified):
  - Thinking: tiny 3-dot thought bubble above head
  - Tool_use: small wrench badge below skateboard
  - Complete: star burst above + golden tint
  - General: subtle underglow matching activity color

**Animation** (mirrors Racer patterns):
- Spring physics (lighter: damping=0.90, stiffness=0.12)
- Wheel spin driven by movement delta
- Position lerp with configurable follow delay (0.15 + random jitter)
- Ear wiggle and tail wag continuous animations
- Completion: rope snap timer (0.3s), then fade to 0.3 opacity over 5s

---

### Phase 3: Tow Rope Rendering

**Integrated into:** `frontend/src/entities/Racer.js` (new `drawTowRope` method)

- Quadratic bezier from parent car rear to hamster skateboard front
- Sag amount: `8 + distance * 0.02` pixels
- Rope color: `#8B7355` (natural rope), 1.5px with highlight stroke
- On completion: rope "snaps" — two dangling stubs with dashed stroke, gravity droop animation

**Attachment points:**
- Parent rear: `x - (17 + LIMO_STRETCH) * CAR_SCALE`, `y + 1 * CAR_SCALE`
- Hamster front: `hamster.displayX + 10`, `hamster.displayY`

---

### Phase 4: Racer Integration + Positioning

**Files:** `frontend/src/entities/Racer.js`, `frontend/src/canvas/RaceCanvas.js`

**Racer.js changes:**
- Add `this.hamsters = new Map()` — subagentId to Hamster instances
- In `update(state)`: sync hamsters from `state.subagents` (create/update/remove)
- In `animate(dt)`: position hamsters in fan pattern behind car, then animate each
- In `draw(ctx)`: draw tow ropes, then delegate `hamster.draw(ctx)` for each

**Fan positioning** (behind parent car):
- Base X: `parentX - carRearOffset - 30`
- Y spread: `(i - (count-1)/2) * 25` (vertical fan)
- X stagger: `-i * 15` (each subsequent hamster further back)
- Cap visual spread at lane height; compress spacing when count > 4

**Zone behavior:**
- Hamsters follow parent through track/pit/parking transitions (no independent zone)
- Inherit parent's dimming and desaturation
- Follow with slight delay via lerp

---

### Phase 5: Hit Testing + Detail Flyout

**Files:** `frontend/src/canvas/RaceCanvas.js`, `frontend/src/main.js`

- Extend `handleClick` to check hamster bounding boxes (20x15 hit area) before racer hit boxes
- Add `onHamsterClick` callback alongside existing `onRacerClick`
- Hamster flyout shows: slug, model, activity, current tool, message/tool counts, duration
- Parent car flyout gains "Subagents" section listing each child with activity + current tool

---

### Phase 6: Bloom + Particles

**Files:** `frontend/src/canvas/RaceCanvas.js`, `frontend/src/canvas/Particles.js`

- Hamsters contribute to bloom pass (small glow when active)
- Tiny exhaust puffs from skateboard rear when active
- Small confetti burst on completion (15 particles vs car's 60)
- Spark effect at rope break point

---

## Worktree Parallelization

| Worktree | Phases | Blocked By |
|---|---|---|
| `backend-subagent-parse` | 1a–1e | — |
| `frontend-hamster-entity` | 2, 3 | — |
| `frontend-racer-integration` | 4, 5 | Both above |
| `frontend-bloom-particles` | 6 | `frontend-hamster-entity` |
| `tests-backend` | Backend tests | `backend-subagent-parse` |
| `tests-frontend` | Frontend tests | `frontend-hamster-entity` |

**Execution order:** `backend-subagent-parse` + `frontend-hamster-entity` in parallel first, then `frontend-racer-integration`, then remainder.

## Scope

- **Claude only** — only Claude Code logs subagent progress entries
- **One level deep** — no recursive subagent nesting (data supports it, UI doesn't)
- **No separate lanes** — hamsters share parent's lane space
- **Canvas 2D** — follows current rendering approach; PixiJS migration converts Hamster same as Racer

## Verification

1. **Backend**: Run `go test ./backend/internal/monitor/...` — new tests for progress parsing and subagent completion detection
2. **Frontend**: Run `npm test` in `frontend/` — Hamster entity tests for spring physics, lerp, activity transitions
3. **E2E**: `make dev`, spawn a Claude session that uses Task tool subagents, verify hamsters appear towed behind the parent car, click hamster for detail flyout
4. **Edge cases**: Multiple simultaneous subagents (fan layout), subagent completion (rope snap), parent moving to pit (hamsters follow), parent completion (hamsters fade with parent)
