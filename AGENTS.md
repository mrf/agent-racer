# Agent Instructions

This project uses **bd** (beads) for issue tracking. Run `bd onboard` to get started.

## General Rules

- When asked to create an issue, ONLY create the issue. Do not make code changes, mark issues in-progress, or take any action beyond what was explicitly requested.
- When a fix attempt fails twice for the same root cause, stop and re-evaluate the approach from scratch rather than iterating on the same strategy. Especially for WebGL/rendering issues — check whether the underlying library even supports the approach.

## Global Engineering Principle

**Favor modular design and minimal duplication.** Keep backend logic UI-agnostic so the Go service can power alternative GUIs. For multi-agent work, reuse shared abstractions instead of duplicating parsing or state logic, and avoid tight coupling between agent sources and frontend rendering.

**DO:**
- Add new sources via the `Source` interface
- Put shared parsing in `monitor/jsonl.go`
- Derive frontend visuals from session state, not source identity

**DO NOT:**
- Add rendering hints to backend structs
- Duplicate JSONL parsing per source
- Branch frontend logic on source name
- Couple source discovery to a specific UI

**Go code must use traditional `for i := 0; i < n; i++` loops** — do not use range-over-integer syntax, for compatibility.

**Follow XDG Base Directory spec** for config, state, and cache paths whenever applicable (e.g., config in `~/.config`, state in `~/.local/state`). Avoid writing new files directly into `$HOME` unless there is a clear exception.

**Place research and planning docs in `docs/`.** All design documents, research notes, and implementation plans go in the `docs/` directory — not the project root. Keep the root clean (only README.md, AGENTS.md, and config files).

## Architecture

- **Frontend** is served via Vite dev server (`make dev`), NOT the Go backend. When debugging 404s or frontend issues, ensure the Vite dev server is running.
- **Backend** is a Go service in `backend/`. The Go backend serves the built frontend only in production mode (`make run`).

## Scope Discipline

**Test tasks must only add tests.** When a beads issue says "add tests for X", the spawned session must NOT refactor, rewrite, or extend the production code it's testing. If a test reveals a bug or improvement opportunity, file a new beads issue — don't fix it in the test branch. Production changes hiding in "test" branches bypass review and cause regressions.

**Spawned worktree sessions must stay in scope.** The seed prompt defines the task boundary. If the agent discovers adjacent work, it should create a beads issue (with `discovered-from` dependency) rather than expanding scope.

## Git Worktrees

- Never delete a worktree directory while your CWD is inside it. Always `cd` to the main repo directory first before running cleanup commands.
- When spawning commands in tmux or subshells, keep shell quoting simple. Do not over-escape or nest quotes. Prefer heredocs or temp files for complex multi-line prompts.

## Post-Merge Checklist

After merging a frontend branch to main, always verify the app loads correctly in the browser (check for runtime errors, not just build success). Run `curl localhost:<port>` or check dev server output for errors.

## Regression Investigation

When a regression is reported after merges, follow this diagnostic order:

1. `git diff --stat` across recent merges — find unexpected production file changes
2. Check test-only branches for production code modifications (common source of regressions)
3. Trace the affected lifecycle end-to-end in the backend before touching code
4. Write a failing test that reproduces the bug before writing the fix

## Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --status in_progress  # Claim work
bd close <id>         # Complete work
bd sync               # Sync with git
```

## Landing the Plane (Session Completion)

**When ending a work session**, complete ALL steps below before stopping.

1. **File issues for remaining work** — `bd create` for anything that needs follow-up
2. **Run quality gates** (if code changed) — tests, linters, builds
3. **Update issue status** — `bd close` finished work, update in-progress items
4. **Commit all changes** — everything staged and committed, nothing left dirty
5. **Sync beads** — `bd sync`
6. **Hand off** — provide context for the next session

**Rules:**
- Do NOT perform remote git operations (push, pull, fetch) — the user handles those
- Worktree sessions: commit and stop. The parent session handles merge and cleanup.
