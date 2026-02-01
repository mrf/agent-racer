# Tmux Focus Support

Click a car in the racing visualization to switch to its Claude session's tmux pane.

## Overview

The backend discovers which tmux pane each Claude process lives in by walking the process tree from the Claude PID upward to find a matching tmux pane PID. A new REST endpoint executes `tmux select-window` and `tmux select-pane` to focus the target. The frontend adds hit detection on car clicks and calls this endpoint.

## Backend Changes

### 1. Tmux Discovery (`backend/internal/monitor/tmux.go`) — new file

Responsible for mapping PIDs to tmux pane targets.

```go
package monitor

// TmuxPane represents a single tmux pane and its shell PID.
type TmuxPane struct {
    SessionName string // e.g. "main"
    WindowIndex int    // e.g. 2
    PaneIndex   int    // e.g. 0
    PanePID     int    // PID of the shell running inside this pane
    Target      string // Pre-formatted "main:2.0" for tmux commands
}

// TmuxResolver maps process PIDs to their containing tmux pane.
type TmuxResolver struct {
    panes []TmuxPane
}

// NewTmuxResolver queries tmux for all panes. Returns nil resolver
// (not an error) when tmux is not running or not installed.
func NewTmuxResolver() *TmuxResolver

// Resolve walks the process tree from pid upward (via /proc/<pid>/stat ppid)
// to find a PID that matches a tmux pane's PanePID. Returns the pane target
// string and true, or ("", false) if no match.
func (r *TmuxResolver) Resolve(pid int) (string, bool)
```

**Pane enumeration:**

```bash
tmux list-panes -a -F '#{pane_pid}\t#{session_name}\t#{window_index}\t#{pane_index}'
```

Parse each line into a `TmuxPane`. If `tmux` is not on `$PATH` or the command fails (no server running), return a nil resolver — the feature silently disables.

**PID tree walk:**

Claude runs as a child (or grandchild) of the tmux pane's shell:

```
tmux pane shell (PID 1234)
  └── node /usr/bin/claude (PID 5678)
```

Walk from the Claude PID upward via `/proc/<pid>/stat` field 4 (ppid), up to a depth limit of 10 hops. Stop at PID 1 or when a match is found.

**Caching:** The resolver is instantiated once per poll cycle in `Monitor.poll()`. Tmux state changes infrequently relative to the 1s poll interval, so re-querying each cycle is acceptable. No cross-cycle caching needed.

### 2. Session State (`backend/internal/session/state.go`)

Add one field to `SessionState`:

```go
type SessionState struct {
    // ... existing fields ...
    TmuxTarget string `json:"tmuxTarget,omitempty"` // e.g. "main:2.0"
}
```

`omitempty` keeps the JSON clean for non-tmux sessions. No changes to `UpdateUtilization()`, `IsTerminal()`, or any other method.

### 3. Process Scanner (`backend/internal/monitor/process.go`)

Add `TmuxTarget` to `ProcessInfo`:

```go
type ProcessInfo struct {
    PID        int
    WorkingDir string
    StartTime  time.Time
    CmdLine    string
    TmuxTarget string // New: populated by TmuxResolver
}
```

No changes to `DiscoverSessions()` itself. The tmux resolution happens in the monitor poll loop after discovery (see below), keeping process scanning and tmux resolution as separate concerns.

### 4. Monitor Poll Loop (`backend/internal/monitor/monitor.go`)

In `poll()`, after `DiscoverSessions()` returns:

```go
func (m *Monitor) poll() {
    procs := DiscoverSessions()

    // Resolve tmux targets for all discovered processes.
    resolver := NewTmuxResolver() // nil if tmux unavailable
    if resolver != nil {
        for i := range procs {
            if target, ok := resolver.Resolve(procs[i].PID); ok {
                procs[i].TmuxTarget = target
            }
        }
    }

    // ... existing tracking/update logic ...
}
```

When creating or updating a `SessionState` from a `ProcessInfo`, copy `TmuxTarget` through:

```go
state.TmuxTarget = proc.TmuxTarget
```

For already-tracked sessions, update `TmuxTarget` each cycle (tmux windows can be moved/renumbered). This is a cheap string assignment.

### 5. WebSocket Protocol (`backend/internal/ws/protocol.go`)

No structural changes. `TmuxTarget` is already part of `SessionState` and will be included in `SnapshotPayload.Sessions` and `DeltaPayload.Updates` automatically via JSON serialization.

Clients receive the tmux target as part of the session data they already consume.

### 6. Focus Endpoint (`backend/internal/ws/server.go`)

Add a new HTTP handler:

```go
// POST /api/sessions/{id}/focus
func (s *Server) handleFocus(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }

    // Extract session ID from path: /api/sessions/<id>/focus
    sessionID := extractSessionID(r.URL.Path) // parse from path segments

    state := s.store.Get(sessionID)
    if state == nil {
        http.Error(w, "session not found", http.StatusNotFound)
        return
    }
    if state.TmuxTarget == "" {
        http.Error(w, "session has no tmux pane", http.StatusConflict)
        return
    }

    // Split target "main:2.0" into session:window and pane components.
    // select-window takes "session:window", select-pane takes full target.
    if err := tmuxFocus(state.TmuxTarget); err != nil {
        http.Error(w, fmt.Sprintf("tmux focus failed: %v", err), http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusNoContent)
}
```

```go
// tmuxFocus switches to the tmux pane identified by target (e.g. "main:2.0").
func tmuxFocus(target string) error {
    // select-window first (switches to the right window)
    // target "main:2.0" works directly with -t for both commands.
    if err := exec.Command("tmux", "select-window", "-t", target).Run(); err != nil {
        return fmt.Errorf("select-window: %w", err)
    }
    if err := exec.Command("tmux", "select-pane", "-t", target).Run(); err != nil {
        return fmt.Errorf("select-pane: %w", err)
    }
    return nil
}
```

Register the route in `SetupRoutes()`:

```go
mux.HandleFunc("/api/sessions/", s.handleSessionRoutes)
```

Where `handleSessionRoutes` dispatches based on path suffix (`/focus` vs returning session data).

### 7. Mock Generator (`backend/internal/mock/generator.go`)

Assign fake tmux targets to mock sessions so the click affordance renders in dev mode:

```go
{Name: "opus-refactor",  /* ... */ TmuxTarget: "dev:0.0"},
{Name: "sonnet-tests",   /* ... */ TmuxTarget: "dev:1.0"},
{Name: "opus-debug",     /* ... */ TmuxTarget: "dev:2.0"},
{Name: "sonnet-feature", /* ... */ TmuxTarget: ""}, // Intentionally empty: tests no-tmux fallback
{Name: "opus-review",    /* ... */ TmuxTarget: "dev:3.0"},
```

The focus endpoint will fail gracefully in mock mode since these tmux sessions don't exist, which is fine for testing the frontend flow.

## Frontend Changes

### 8. Racer Click Affordance (`frontend/src/entities/Racer.js`)

Add hover state tracking and visual feedback:

```javascript
class Racer {
    constructor(id, state) {
        // ... existing ...
        this.hovered = false;
        this.hasTmux = false; // set from state.tmuxTarget
    }

    update(state) {
        // ... existing ...
        this.hasTmux = !!state.tmuxTarget;
    }
}
```

**Hover effect (in `draw()`):** When `this.hovered && this.hasTmux`, draw a subtle highlight border around the car body (2px stroke in the model's light color at 50% opacity). This signals clickability without cluttering the default view.

**Cursor:** Handled by `RaceCanvas` (see below), not by Racer directly.

### 9. Canvas Hit Detection & Focus (`frontend/src/canvas/RaceCanvas.js`)

The existing `handleClick(e)` already finds the nearest racer within 30px and calls `this.onRacerClick(racer.state)`. Extend this:

```javascript
handleClick(e) {
    const rect = this.canvas.getBoundingClientRect();
    const x = (e.clientX - rect.left) * (this.canvas.width / rect.width);
    const y = (e.clientY - rect.top) * (this.canvas.height / rect.height);

    for (const racer of this.racers.values()) {
        const dx = x - racer.displayX;
        const dy = y - racer.displayY;
        if (Math.sqrt(dx * dx + dy * dy) < 30) {
            if (this.onRacerClick) this.onRacerClick(racer.state);
            if (racer.state.tmuxTarget) {
                this.focusSession(racer.state.id);
            }
            return;
        }
    }
    // ... existing: close detail panel on background click ...
}
```

```javascript
async focusSession(sessionId) {
    try {
        const resp = await fetch(`/api/sessions/${sessionId}/focus`, {
            method: 'POST',
        });
        if (!resp.ok) {
            const text = await resp.text();
            console.warn(`Focus failed: ${text}`);
        }
    } catch (err) {
        console.warn('Focus request failed:', err);
    }
}
```

**Mouse move (hover detection):** Add a `mousemove` listener to update racer hover states and set cursor style:

```javascript
constructor(canvas) {
    // ... existing ...
    this.canvas.addEventListener('mousemove', (e) => this.handleMouseMove(e));
}

handleMouseMove(e) {
    const rect = this.canvas.getBoundingClientRect();
    const x = (e.clientX - rect.left) * (this.canvas.width / rect.width);
    const y = (e.clientY - rect.top) * (this.canvas.height / rect.height);

    let hoveredAny = false;
    for (const racer of this.racers.values()) {
        const dx = x - racer.displayX;
        const dy = y - racer.displayY;
        const isNear = Math.sqrt(dx * dx + dy * dy) < 30;
        racer.hovered = isNear;
        if (isNear && racer.hasTmux) hoveredAny = true;
    }
    this.canvas.style.cursor = hoveredAny ? 'pointer' : 'default';
}
```

### 10. Detail Panel Indicator (`frontend/src/main.js`)

In the detail panel rendering (the `showDetailPanel` logic), add a tmux indicator:

```javascript
// After existing detail fields...
if (session.tmuxTarget) {
    addField('Tmux', session.tmuxTarget);
} else {
    addField('Tmux', 'not in tmux');
}
```

This gives the user visibility into which sessions are tmux-focusable without needing to hover.

### 11. Click Behavior Summary

| State | Click behavior |
|-------|---------------|
| `tmuxTarget` present | Focus tmux pane + show detail panel |
| `tmuxTarget` empty | Show detail panel only |
| Session terminal (complete/errored) | Show detail panel only (no focus) |

For terminal sessions, skip the focus call even if `tmuxTarget` is set — the pane may have been reused.

## New Files

| File | Purpose |
|------|---------|
| `backend/internal/monitor/tmux.go` | `TmuxResolver`, pane enumeration, PID tree walk |
| `backend/internal/monitor/tmux_test.go` | Tests for PID tree walk, target parsing |

## Modified Files

| File | Change |
|------|--------|
| `backend/internal/session/state.go` | Add `TmuxTarget` field |
| `backend/internal/monitor/process.go` | Add `TmuxTarget` to `ProcessInfo` |
| `backend/internal/monitor/monitor.go` | Create resolver per poll, populate targets |
| `backend/internal/ws/server.go` | Add `POST /api/sessions/{id}/focus` endpoint |
| `backend/internal/mock/generator.go` | Assign fake tmux targets to mock sessions |
| `frontend/src/entities/Racer.js` | Hover state, `hasTmux` flag, highlight border |
| `frontend/src/canvas/RaceCanvas.js` | `focusSession()`, `handleMouseMove()`, cursor |
| `frontend/src/main.js` | Tmux field in detail panel |

## Testing

### `tmux_test.go`

- **`TestParseTmuxPanes`**: Feed sample `tmux list-panes` output, verify correct `TmuxPane` structs.
- **`TestResolvePID_DirectChild`**: Mock `/proc/<pid>/stat` with a two-level tree (shell → claude). Verify resolution.
- **`TestResolvePID_GrandChild`**: Three-level tree (shell → node → claude). Verify upward walk finds pane.
- **`TestResolvePID_NoMatch`**: PID tree that doesn't contain any pane PID. Verify `("", false)`.
- **`TestResolvePID_DepthLimit`**: Deep process tree (>10 levels). Verify walk stops at limit.
- **`TestNewTmuxResolver_NoTmux`**: `tmux` not on PATH. Verify nil resolver, no error.

Use `t.TempDir()` with synthetic `/proc`-like directory trees for the PID walk tests. For the `tmux list-panes` parsing, inject output as a string rather than calling the real binary.

### Focus endpoint test

- **`TestHandleFocus_Success`**: Session with `TmuxTarget` set. Verify 204 response (mock the exec call).
- **`TestHandleFocus_NoTmux`**: Session with empty `TmuxTarget`. Verify 409.
- **`TestHandleFocus_NotFound`**: Nonexistent session ID. Verify 404.

### Frontend

Manual testing in mock mode:
- Hover over car with tmux target → pointer cursor, highlight border.
- Hover over car without tmux target → default cursor, no highlight.
- Click car with tmux target → detail panel opens, focus request fires (will 500 in mock, logged to console).
- Click car without tmux target → detail panel opens, no focus request.

## Edge Cases

1. **Tmux not installed or no server running.** `NewTmuxResolver()` returns nil. All sessions have empty `TmuxTarget`. Frontend shows detail panel on click, no focus affordance. No user-facing errors.

2. **Session moves between tmux windows.** `TmuxTarget` updates each poll cycle via re-resolution. The 1s lag is imperceptible.

3. **Tmux pane closed while session tracked.** The focus endpoint's `tmux select-window` will fail. The endpoint returns 500. Next poll cycle, the process likely disappears and gets marked `Complete`/`Lost`, removing the stale target.

4. **Multiple Claude sessions in same tmux window (different panes).** Each has a unique pane index in the target string (e.g. `main:2.0` vs `main:2.1`). Resolution is per-PID, so both map correctly.

5. **Claude process is deeply nested** (e.g. inside docker exec, or multiple shell layers). The 10-hop depth limit handles reasonable nesting. Docker containers have separate PID namespaces, so `/proc` traversal won't cross the boundary — these sessions simply won't get a tmux target.

6. **Race condition: poll resolves target, user clicks, pane was just destroyed.** The focus endpoint returns 500, logged to browser console. Not surfaced to the user as a modal or toast — transient failures don't warrant UI noise.
