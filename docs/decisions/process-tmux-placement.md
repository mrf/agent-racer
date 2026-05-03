# ADR: process.go and tmux*.go Placement

**Date:** 2026-05-02
**Status:** Decided
**Beads:** agent-racer-dou

---

## Context

Two modules in `backend/internal/monitor/` add OS-level awareness beyond JSONL parsing:

| File(s) | Lines | What it does |
|---|---|---|
| `process.go` | 144 | Scans running processes via `gopsutil`, identifies agent binaries (claude, codex, gemini, node-based), computes per-PID CPU deltas, counts ESTABLISHED TCP connections, filters CWDs inside `~/.claude`. Exports `ProcessActivity`, `DiscoverProcessActivity`. |
| `tmux.go`, `tmux_linux.go`, `tmux_other.go` | 195 | Queries `tmux list-panes -a`, builds a PID→target map, walks the process tree (up to 10 ancestors via `/proc/<pid>/stat`) to resolve any agent PID to a tmux pane target string. Exports `TmuxResolver`, `TmuxPane`. Linux-only via build tags. |

The agentwatch migration plan (§5.4, §6, §10.2, §10.7) flagged both as open questions:

- `TmuxTarget`, `PID` explicitly removed from `session.SessionState` v1, with a note to decide whether to include behind a `monitor.WithProcessAwareness()` option.
- tmux.go flagged for possible `agentwatch/integrations/tmux` sub-package.

---

## Decision

**Keep both modules in agent-racer. Do not migrate to agentwatch.**

---

## Findings

### agentwatch v0.1.0 does not include process awareness

Verified by reading the agentwatch source at `/home/mrf/Projects/agentwatch`:

- `monitor/options.go` — no `WithProcessAwareness` option exists
- `session/state.go` — no `PID`, `TmuxTarget`, or process-awareness fields
- No `process.go`, `tmux.go`, or equivalent files anywhere in the repo
- `session.SessionState` matches the slimmed v1 design from the migration plan

### The code is correctly racer-specific today

`process.go` feeds the `IsChurning` metric, which drives visual effects in the frontend (pit/parking lot placement, churn indicators). `tmux.go` surfaces `TmuxTarget` so the frontend can display which tmux pane hosts an agent. Both fields are part of agent-racer's `SessionState` god struct — they serve the racing UI, not a generic monitoring consumer.

### Coupling concerns

- `process.go` depends on `github.com/shirou/gopsutil/v3` — a heavy OS-inspection library. Adding it to agentwatch would impose that dependency on all consumers, including those that don't want process scanning.
- `tmux.go` is Linux-only (build-tagged), executes a subprocess (`tmux`), and reads `/proc`. This is appropriate for an app but not for a library that advertises cross-platform portability.
- Neither module is imported anywhere in agentwatch; neither is referenced by the `Source` interface or `EventSink`.

---

## Rationale

The agentwatch migration plan explicitly deferred the process-awareness decision to a follow-up issue (open question §10.2). The v0.1.0 release resolved that question by omission: the library ships without it. Migrating now would require:

1. Deciding the public API shape (`WithProcessAwareness()`, or a separate `integrations/tmux` module)
2. Adding a gopsutil dependency to agentwatch
3. Resolving the Linux-only/cross-platform tension
4. Designing how consumers attach process data to session state without coupling to `PID`/`TmuxTarget` fields that aren't in `session.SessionState`

None of those are trivial. The migration plan already called out §10.7 ("Tmux integration packaging") as an unresolved design question. Doing it as part of the current migration wave would expand scope beyond what v0.1.0 targeted.

---

## Consequences

- `process.go` and `tmux*.go` remain in `backend/internal/monitor/` with no changes.
- When agent-racer migrates to consume agentwatch (Phase 5), it wraps the agentwatch `EventSink` with its own `racer.Sink` that calls `DiscoverProcessActivity` and `TmuxResolver.Resolve` locally — these stay as agent-racer internals.
- A follow-up issue should be filed in the agentwatch repo to design `WithProcessAwareness()` and/or `integrations/tmux` for a future minor version.

---

## Follow-up

File a beads issue in agentwatch: **"Design process/tmux awareness API for v0.2"** — covering `WithProcessAwareness()` option, gopsutil dependency decision, and cross-platform strategy for tmux integration.
