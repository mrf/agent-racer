# Agent Instructions

This project uses **bd** (beads) for issue tracking. Run `bd onboard` to get started.

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

**Follow XDG Base Directory spec** for config, state, and cache paths whenever applicable (e.g., config in `~/.config`, state in `~/.local/state`). Avoid writing new files directly into `$HOME` unless there is a clear exception.

**Place research and planning docs in `docs/`.** All design documents, research notes, and implementation plans go in the `docs/` directory — not the project root. Keep the root clean (only README.md, AGENTS.md, and config files).

## Scope Discipline

**Test tasks must only add tests.** When a beads issue says "add tests for X", the spawned session must NOT refactor, rewrite, or extend the production code it's testing. If a test reveals a bug or improvement opportunity, file a new beads issue — don't fix it in the test branch. Production changes hiding in "test" branches bypass review and cause regressions.

**Spawned worktree sessions must stay in scope.** The seed prompt defines the task boundary. If the agent discovers adjacent work, it should create a beads issue (with `discovered-from` dependency) rather than expanding scope.

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

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   bd sync
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds



<!-- BEGIN BEADS INTEGRATION -->
## Issue Tracking with bd (beads)

**IMPORTANT**: This project uses **bd (beads)** for ALL issue tracking. Do NOT use markdown TODOs, task lists, or other tracking methods.

### Why bd?

- Dependency-aware: Track blockers and relationships between issues
- Git-friendly: Auto-syncs to JSONL for version control
- Agent-optimized: JSON output, ready work detection, discovered-from links
- Prevents duplicate tracking systems and confusion

### Quick Start

**Check for ready work:**

```bash
bd ready --json
```

**Create new issues:**

```bash
bd create "Issue title" --description="Detailed context" -t bug|feature|task -p 0-4 --json
bd create "Issue title" --description="What this issue is about" -p 1 --deps discovered-from:bd-123 --json
```

**Claim and update:**

```bash
bd update bd-42 --status in_progress --json
bd update bd-42 --priority 1 --json
```

**Complete work:**

```bash
bd close bd-42 --reason "Completed" --json
```

### Issue Types

- `bug` - Something broken
- `feature` - New functionality
- `task` - Work item (tests, docs, refactoring)
- `epic` - Large feature with subtasks
- `chore` - Maintenance (dependencies, tooling)

### Priorities

- `0` - Critical (security, data loss, broken builds)
- `1` - High (major features, important bugs)
- `2` - Medium (default, nice-to-have)
- `3` - Low (polish, optimization)
- `4` - Backlog (future ideas)

### Workflow for AI Agents

1. **Check ready work**: `bd ready` shows unblocked issues
2. **Claim your task**: `bd update <id> --status in_progress`
3. **Work on it**: Implement, test, document
4. **Discover new work?** Create linked issue:
   - `bd create "Found bug" --description="Details about what was found" -p 1 --deps discovered-from:<parent-id>`
5. **Complete**: `bd close <id> --reason "Done"`

### Auto-Sync

bd automatically syncs with git:

- Exports to `.beads/issues.jsonl` after changes (5s debounce)
- Imports from JSONL when newer (e.g., after `git pull`)
- No manual export/import needed!

### Important Rules

- ✅ Use bd for ALL task tracking
- ✅ Always use `--json` flag for programmatic use
- ✅ Link discovered work with `discovered-from` dependencies
- ✅ Check `bd ready` before asking "what should I work on?"
- ❌ Do NOT create markdown TODO lists
- ❌ Do NOT use external issue trackers
- ❌ Do NOT duplicate tracking systems

For more details, see README.md and docs/QUICKSTART.md.

<!-- END BEADS INTEGRATION -->
