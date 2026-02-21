# Frontend Refactor Plan (Modularization)

Goal: reduce `frontend/src/main.js` size and state complexity while preserving behavior. Keep the rendering pipeline UI‑agnostic and driven by `SessionState` inputs.

## Principles

- No backend rendering hints.
- Derive visuals from session state, not source identity.
- Extract pure helpers for formatting and state derivation.

## Proposed Modules

### 1) `ui/formatters.js`
Move these functions out of `main.js`:
- `formatTokens`
- `formatBurnRate`
- `formatTime`
- `formatElapsed`
- `basename`
- `esc`

### 2) `ui/detailFlyout.js`
Encapsulate flyout logic:
- `showDetailFlyout(state, carX, carY)`
- `positionFlyout(carX, carY)`
- `renderDetailFlyout(state)`
- local flyout positioning state

Expose a small interface for `main.js`:

```js
const flyout = createFlyout({
  detailFlyout,
  flyoutContent,
  canvas,
});
flyout.show(state, carX, carY);
flyout.hide();
flyout.updatePosition(carX, carY);
```

### 3) `ui/sessionTracker.js`
Track session lifecycle transitions and SFX cooldowns:
- Known session IDs
- Activity transitions
- SFX cooldown bookkeeping

Expose:
```js
const tracker = createSessionTracker();
tracker.onSnapshot(sessions);
tracker.onDelta(updates, removed);
```

### 4) `ui/ambientAudio.js`
Handle first‑interaction start of ambient audio and mute toggles.

## Step‑by‑Step Migration

1. Extract `formatters.js` and update imports in `main.js`.
2. Extract flyout logic into `ui/detailFlyout.js`.
3. Extract session tracking logic into `ui/sessionTracker.js`.
4. Extract ambient audio startup into `ui/ambientAudio.js`.
5. Run existing Vitest + Playwright tests to validate behavior.

## Expected Outcomes

- `main.js` shrinks into orchestration code.
- Flyout behavior becomes isolated and testable.
- Formatting helpers become reusable and unit-testable.

