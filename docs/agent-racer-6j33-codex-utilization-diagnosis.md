# agent-racer-6j33 Codex utilization regression diagnosis

## Reproduction

Minimal sanitized fixture:

- `docs/fixtures/codex-last-token-usage-regression.jsonl`

Real failing session captured during diagnosis:

- `/home/mrf/.codex/sessions/2026/03/07/rollout-2026-03-07T10-51-47-019cc9a4-33ee-70b3-9551-efdbdc1c338b.jsonl`

Representative `token_count` snapshot from that real session:

- `total_token_usage.input_tokens = 64,151,475`
- `last_token_usage.input_tokens = 198,158`
- `model_context_window = 258,400`

Expected live context utilization from the same event:

- `198,158 / 258,400 = 0.767`

Current Agent Racer derived utilization from the same event:

- `64,151,475 / 258,400 = 248.26`
- Session state clamps this to `1.0`, so the racer reaches 100% immediately.

## Root Cause

The regression is not the March 7 denominator bug from `agent-racer-05gm`.

- The denominator is present and correct in current logs: `task_started.model_context_window = 258400`.
- The bad value is the numerator: [`backend/internal/monitor/codex_source.go`](/home/mrf/Projects/agent-racer--6j33-diagnose-codex-context-window-utilization-regression/backend/internal/monitor/codex_source.go#L439) reads `info.total_token_usage.input_tokens` into `parsed.tokensIn`.
- [`backend/internal/monitor/monitor.go`](/home/mrf/Projects/agent-racer--6j33-diagnose-codex-context-window-utilization-regression/backend/internal/monitor/monitor.go#L842) then treats `update.TokensIn` as the live context occupancy for `usage` strategy sessions.

For current Codex CLI logs, `info.total_token_usage` is a lifetime session counter. It can exceed the active turn's context window by orders of magnitude. The correct live-context numerator is `info.last_token_usage`.

## Distinction From agent-racer-05gm

`agent-racer-05gm` fixed missing or wrong context ceilings:

- fallback matching for Codex model names
- `task_started.model_context_window` ingestion

This regression remains even when that fix is present because the denominator is already correct. The new failure mode is a numerator mismatch inside the nested `token_count` payload.

## Follow-up Fix Direction

The parser/state path should:

- prefer `info.last_token_usage` for live context utilization when present
- keep `info.total_token_usage` only for lifetime accounting or diagnostics
- preserve support for older flat `token_count` formats that do not provide `last_token_usage`
- update Codex parser tests that currently encode `total_token_usage` as the utilization source
