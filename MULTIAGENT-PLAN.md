# Multiagent Plan: Include Codex (ChatGPT CLI) + Gemini CLI in Agent Racer

## Goal
Extend Agent Racer beyond Claude Code JSONL sessions to include **Codex (ChatGPT CLI)** and **Gemini CLI** sessions in the race, using each tool’s available local logs or metadata and a unified session abstraction.

## Scope
- **Codex (ChatGPT CLI)**
- **Gemini CLI**
- Keep current Claude Code support intact.
- Support mock mode unchanged.

## Assumptions / Unknowns
- Each CLI must leave **local artifacts** we can read (logs, history files, session metadata, cache dirs).
- We need to confirm file locations, schema, and update frequency for each CLI.

## Plan

### 1) Discovery research (per CLI)
**Deliverable:** A short doc snippet or table of where to find session logs and how to map to a working directory.

- **Codex (ChatGPT CLI)**
  - Identify: session log location, per-project/session file naming, schema (tokens, model, tool usage, timestamps).
  - Determine: how to map a session to a working dir (if available).
  - Determine: if there is a process signature to correlate with log entries.

- **Gemini CLI**
  - Identify: session/history storage path, schema, update behavior.
  - Determine: mapping to working dir and session identity.

**Output:** `docs/multiagent-sources.md` with:
- CLI name
- Log file path(s)
- Schema summary
- Session ID derivation
- Working dir inference
- Token/usage availability

### 2) Define a unified session ingestion interface
**Deliverable:** New interface and implementations for each agent source.

Create a small interface in `backend/internal/monitor/sources`:
```go
type Source interface {
    Name() string
    Discover() ([]SessionHandle, error) // returns list of session files/IDs
    Parse(handle SessionHandle, offset int64) (*ParseResult, int64, error)
    Metadata(handle SessionHandle) (SessionMetadata, error) // working dir, startedAt, model
}
```
- `SessionHandle` may be `{ID, Path, AgentType}`.
- `ParseResult` should normalize:
  - `LatestUsage` (tokens)
  - `MessageCount`
  - `ToolCalls`
  - `LastTool`
  - `LastActivity`
  - `LastTime`
- `SessionMetadata`: `WorkingDir`, `Model`, `StartedAt`, etc.

Implementations:
- `ClaudeSource` (wrap current JSONL logic)
- `CodexSource`
- `GeminiSource`

### 3) Replace monitor discovery with multi-source polling
**Deliverable:** New monitor loop that aggregates sources.

- Replace `DiscoverSessions()` or file scanning with:
  - `sources := []Source{ClaudeSource, CodexSource, GeminiSource}`
  - Gather session handles across sources each poll.
- Track sessions by `(AgentType, SessionID)` (composite key).
- Maintain per-handle offsets for incremental parsing.

### 4) Extend session state with agent type
**Deliverable:** Display and filtering improvements.

- Add `AgentType` to `SessionState`:
  - values: `claude`, `codex`, `gemini`
- In frontend, display badge or icon by agent type.
- Use distinct color palette per agent family.

### 5) Token normalization
**Deliverable:** Consistent progress across agents.

- If a CLI does **not** provide full context usage:
  - Option A: estimate from token counts in messages if available.
  - Option B: fall back to message count → heuristic progress bar (explicitly labeled).
- Add config per agent:
  - `max_context_tokens` defaults
  - `token_strategy` (`usage`, `estimate`, `message_count`)

### 6) UI: multi-agent cues and filters
**Deliverable:** UX that stays readable.

- Add a **filter** dropdown (All / Claude / Codex / Gemini).
- Add legend for model families (optional).
- Provide small tooltip: “Estimated” when token usage is heuristic.

### 7) Testing & validation
**Deliverable:** Confidence that each agent updates live.

- Unit tests for each parser on sample log fixtures.
- Integration manual test:
  - start one session of each CLI
  - confirm each shows in dashboard
  - verify activity updates and completion states.

## Milestones
1. **Research complete**: log paths + schema for Codex + Gemini
2. **Unified ingestion interface** merged
3. **Codex ingestion working**
4. **Gemini ingestion working**
5. **UI updates complete**

## Risks
- CLI logs may not include token usage or tool events.
- Some CLIs may not persist session logs by default.
- Mapping to working directory may be unreliable or absent.

## Follow-up Questions
- Which CLI installations are being used (official vs community wrappers)?
- Are you ok with **opt-in** support via config paths if logs are non-standard?
- Should the agent type be shown as the primary label, or the model name?
