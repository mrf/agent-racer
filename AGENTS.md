# Agent Instructions

Go backend + vanilla JS frontend. Explore the code before making changes — read existing files rather than assuming structure.

## Commands

```bash
make test              # Go tests
make test-frontend     # Vitest suite
make lint              # go vet
make build             # Embed frontend + compile binary
make dev               # Mock mode, filesystem frontend
make run               # Real mode
```

## Issue Tracking (beads)

```bash
bd onboard             # Get started
bd ready               # Find available work
bd show <id>           # View issue details
bd update <id> --status in_progress
bd close <id>          # Complete work
bd sync                # Sync with git
```

## Boundaries

- **Test tasks only add tests.** Don't refactor or extend production code in a test branch. File a new beads issue instead.
- **Stay in scope.** If you discover adjacent work, `bd create` an issue rather than expanding the current task.
- **XDG paths.** Config in `~/.config`, state in `~/.local/state`. Don't write to `$HOME` directly.
- **Docs in `docs/`.** Design docs and plans go in `docs/`, not the project root.
- **No rendering hints in backend structs.** Frontend derives visuals from session state, not source identity.
- **No remote git operations.** The user handles push/pull/fetch.

## Session Completion

Before stopping, complete ALL of these:

1. `bd create` for any remaining follow-up work
2. Run quality gates if code changed: `make test && make lint`
3. `bd close` finished work
4. Commit all changes — nothing left dirty
5. `bd sync`
6. Hand off context for the next session

Worktree sessions: commit and stop. Parent session handles merge.
