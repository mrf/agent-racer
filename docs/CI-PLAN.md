# CI Plan (Localhost-Focused)

This project is intended for localhost use only, so the CI plan focuses on developer ergonomics, fast feedback, and preventing regressions — not deployment hardening.

## Goals

- Ensure backend + frontend tests run on every change.
- Keep CI quick (unit tests + lightweight linting).
- Provide a single command developers can run locally to match CI.

## Recommended CI Workflow

### 1) Backend (Go)
- `go test ./...`
- `go vet ./...` (optional but fast)

### 2) Frontend (Vitest)
- `npm test` in `frontend/` (Vitest run)

### 3) E2E (Playwright)
- `npm test` in `e2e/`
- Consider running E2E only on main branch or nightly if runtime is high.

## Proposed Make Targets

Add top-level targets (not required now, but recommended):

```make
.PHONY: test-frontend test-e2e lint ci

test-frontend:
	cd frontend && npm test

test-e2e:
	cd e2e && npm test

lint:
	cd backend && go vet ./...

ci: test lint test-frontend test-e2e
```

## Suggested GitHub Actions (Outline)

**Trigger:** `push` and `pull_request`

**Job: test**
- Checkout
- Set up Go 1.22
- `make test`
- Set up Node (use .nvmrc if present, otherwise Node 20)
- `npm ci` in `frontend/`, then `npm test`
- `npm ci` in `e2e/`, then `npm test`

**Optional:** Cache Go build and npm directories for speed.

## Local Parity

Document a single command in README, e.g.:

```bash
make ci
```

This ensures devs can reproduce CI locally.

