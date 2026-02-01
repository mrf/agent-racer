# Subagent Support Plan

## Goal

Track and visualize Claude Code subagents (Task tool invocations) as child racers linked to their parent session, using data already present in the JSONL logs.

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

## Implementation Plan

### 1) Data model: `SubagentState`

Add to `backend/internal/session/state.go`:

```go
type SubagentState struct {
    ID               string    `json:"id"`               // toolUseID
    ParentToolUseID  string    `json:"parentToolUseId"`  // links to parent's tool_use
    SessionID        string    `json:"sessionId"`        // parent session ID
    Slug             string    `json:"slug"`             // display name
    Model            string    `json:"model"`
    Activity         Activity  `json:"activity"`
    CurrentTool      string    `json:"currentTool,omitempty"`
    TokensUsed       int       `json:"tokensUsed"`
    MessageCount     int       `json:"messageCount"`
    ToolCallCount    int       `json:"toolCallCount"`
    StartedAt        time.Time `json:"startedAt"`
    LastActivityAt   time.Time `json:"lastActivityAt"`
    CompletedAt      *time.Time `json:"completedAt,omitempty"`
}
```

Extend `SessionState` with:

```go
Subagents []SubagentState `json:"subagents,omitempty"`
```

### 2) Parser changes: handle `progress` entries

In `jsonl.go`, extend `ParseResult`:

```go
type SubagentParseResult struct {
    ID              string
    ParentToolUseID string
    Slug            string
    Model           string
    LatestUsage     *TokenUsage
    MessageCount    int
    ToolCalls       int
    LastTool        string
    LastActivity    string
    FirstTime       time.Time
    LastTime        time.Time
}

// Add to ParseResult:
Subagents map[string]*SubagentParseResult // keyed by toolUseID
```

Add a `"progress"` case in the `ParseSessionJSONL` switch:

- Extract `toolUseID`, `parentToolUseID`, `slug`, `timestamp` from the entry.
- Parse `data.message.message` for model, usage, content blocks (same logic as `parseAssistantMessage`).
- Group into the `Subagents` map by `toolUseID`.
- Detect completion: when the parent session's next `user` entry contains a `tool_result` whose `tool_use_id` matches `parentToolUseID`, the subagent is done.

### 3) Monitor: propagate subagent state

In the monitor loop (`backend/internal/monitor/`), after parsing:

- Convert each `SubagentParseResult` to a `SubagentState`.
- Attach to the parent `SessionState.Subagents`.
- Broadcast updated state via WebSocket (subagents included in the session payload).

### 4) WebSocket protocol

No protocol change needed -- subagents are nested inside the existing `SessionState` JSON. The frontend receives them automatically:

```json
{
  "id": "89eb3ed5-...",
  "name": "agent-racer",
  "activity": "tool_use",
  "currentTool": "Task",
  "subagents": [
    {
      "id": "agent_msg_01F8z3...",
      "slug": "dynamic-chasing-bunny",
      "model": "claude-haiku-4-5-20251001",
      "activity": "tool_use",
      "currentTool": "Read",
      "messageCount": 35,
      "toolCallCount": 20
    }
  ]
}
```

### 5) Frontend rendering

Options (pick one or combine):

**A) Sidecar mini-cars** -- Small cars on sub-lanes beneath the parent, indented. Show slug as label, model badge. Move independently based on subagent activity. Disappear when subagent completes.

**B) Indicator on parent car** -- Badge count ("2 subagents") on the parent car. Click to expand detail panel showing subagent list with activity, model, current tool.

**C) Particle trail** -- Subagents shown as smaller sprites trailing the parent car, with tool badges. Visual link (line or trail) to parent.

Recommendation: **Start with B** (lowest visual complexity, most informative), then optionally add A for a richer view.

### 6) Detail panel updates

When clicking a session that has active subagents, the detail panel should show:

- Existing session info (unchanged)
- New "Subagents" section listing each with: slug, model, activity, current tool, message/tool counts, duration

## Scope & non-goals

- **In scope:** Claude Code subagents only (Task tool `progress` entries).
- **Not in scope:** Recursive subagents (subagents spawning subagents) -- the data supports it but the UI shouldn't go deeper than one level initially.
- **Not in scope:** Subagent token usage contributing to parent's context utilization (they share the context window but the relationship is complex).

## Dependencies

None. This uses data already in the JSONL files. No hooks, no API changes, no new data sources.

## Files to modify

- `backend/internal/session/state.go` -- Add `SubagentState`, extend `SessionState`
- `backend/internal/monitor/jsonl.go` -- Parse `progress` entries, add `SubagentParseResult`
- `backend/internal/monitor/monitor.go` -- Propagate subagent state
- `frontend/src/entities/Racer.js` -- Accept and store subagent data
- `frontend/src/canvas/RaceCanvas.js` -- Render subagent indicators
- `frontend/src/ui/DetailPanel.js` -- Show subagent details
