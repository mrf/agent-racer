# PixiJS Migration Plan

Migrate the Agent Racer frontend from vanilla Canvas 2D to PixiJS v8,
structured for parallel execution via Claude Code agent-teams.

## Why

The frontend has grown to ~3,850 lines of hand-rolled game engine code:
particle system, bloom compositing, spring physics, procedural audio,
pre-rendered texture caching, and animated spectators. PixiJS v8 provides
these out of the box with WebGL/WebGPU acceleration.

**What PixiJS replaces:**
- Canvas 2D drawing → PixiJS Graphics/Sprites with GPU acceleration
- Custom bloom pass (offscreen canvas downscale) → BlurFilter
- Custom particle system (271 lines, 8 presets) → @pixi/particle-emitter
- Manual DPR scaling → automatic resolution handling
- Pre-rendered texture caching → native texture management
- Immediate-mode redraws → retained scene graph (only dirty regions update)

**What stays unchanged:**
- Backend Go service (monitor, WebSocket, session lifecycle)
- Game logic (zone assignment, activity classification, state machine)
- SoundEngine.js (Web Audio API is orthogonal)
- WebSocket protocol and message format

## Architecture: Before → After

### Current (Immediate Mode)
```
RAF loop → clearRect → draw everything → composite bloom → repeat
```
Every frame redraws the entire canvas from scratch.

### Target (Retained Scene Graph)
```
app.stage
├── trackContainer        (Track background, lanes, markers)
│   ├── asphaltGraphics
│   ├── laneMarkings
│   ├── startFinishLine
│   └── crowdContainer    (spectator sprites)
├── pitContainer          (Pit lane background)
├── parkingLotContainer   (Parking lot background)
├── particlesBehind       (exhaust, smoke — behind racers)
├── racerContainer        (sorted by Y for depth)
│   └── RacerSprite[]     (Container per racer: body + wheels + effects)
├── particlesFront        (sparks, confetti — in front of racers)
└── uiContainer           (connection overlay, empty state)
```
PixiJS diffs the scene graph and only redraws what changed.

Dashboard moves to DOM (it's text + bars, not game graphics).

---

## Phases

### Phase 1: Build Infrastructure

**Goal:** Add Vite + PixiJS to the frontend. Existing app still works.

**Work:**
- Add `package.json` with `pixi.js`, `pixi-filters`, `vite` dependencies
- Configure Vite for dev server (proxy WebSocket to Go backend on :8080)
- Move `index.html` to Vite entry point
- Convert bare ES module imports to work with Vite's module resolution
- Verify the existing app builds and runs identically through Vite
- Add `.gitignore` entries for `node_modules/`, `dist/`

**Blocks:** Phases 2–7 (everything needs the build tooling)

**Acceptance:**
- `npm run dev` serves the app with hot reload
- `npm run build` produces a `dist/` bundle
- All existing functionality works unchanged
- No rendering regressions

**Seed prompt:**
```
Add Vite build tooling to the Agent Racer frontend (frontend/).
Currently it uses plain ES modules with no bundler.

1. Create frontend/package.json with dependencies:
   - pixi.js (^8.16)
   - pixi-filters
   - vite (dev dependency)
2. Configure Vite:
   - Dev server proxies /api and /ws to localhost:8080 (the Go backend)
   - Entry point is frontend/index.html
3. Ensure all existing ES module imports resolve through Vite
4. Do NOT change any rendering code — just get the build working
5. Verify with: cd frontend && npm install && npm run dev

The app should behave identically to before, just served through Vite.
```

---

### Phase 2: Application Shell + Scene Graph

**Goal:** Replace raw canvas + RAF loop with PixiJS Application. Create
the Container hierarchy. All drawing still happens via Canvas 2D
temporarily, rendered into a PixiJS Sprite as a bridge.

**Work:**
- Create `frontend/src/PixiApp.js`:
  - `await new Application().init()` with resolution, background, resizeTo
  - Build the Container tree (see Architecture above)
  - Expose the Ticker for game logic updates
- Update `RaceCanvas.js`:
  - Replace `startLoop()` RAF with PixiJS Ticker callback
  - Replace manual DPR handling with PixiJS resolution
  - Keep `update()` logic intact (zone partitioning, racer animation)
  - Bridge: render existing `draw()` output into a PixiJS Sprite texture
    so the app works during the transition
- Update `main.js` to initialize PixiApp, pass containers to subsystems
- Update `index.html`: remove raw `<canvas>`, let PixiJS create its own

**Blocks:** Phases 3–7

**Acceptance:**
- App renders through PixiJS (check DevTools → canvas has WebGL context)
- All zones (track, pit, parking lot) display correctly
- Racer positioning and zone transitions work
- Screen shake and flash effects work
- Click/hover hit testing works

**Seed prompt:**
```
Create the PixiJS v8 Application shell for Agent Racer.

Current state: RaceCanvas.js owns a raw <canvas>, runs its own RAF loop
via startLoop(), and draws everything with Canvas 2D ctx calls.

Your job:
1. Create frontend/src/PixiApp.js:
   - Import { Application, Container } from 'pixi.js'
   - async init() that creates the Application with:
     - resolution: window.devicePixelRatio
     - autoDensity: true
     - background: '#1a1a2e'
     - resizeTo: container element
   - Build Container hierarchy:
     trackContainer, pitContainer, parkingLotContainer,
     particlesBehind, racerContainer, particlesFront, uiContainer
   - Expose app.ticker for game loop

2. Update RaceCanvas.js:
   - Remove startLoop() RAF — use PixiJS Ticker instead
   - Remove manual DPR scaling
   - KEEP the update() logic exactly as-is (zone partitioning, racer animation)
   - KEEP draw() temporarily — render it to a CanvasTexture displayed
     as a Sprite in the scene graph. This is a bridge so the app works
     while individual renderers are ported in Phases 3-6.

3. Update main.js to create PixiApp, mount it, pass to RaceCanvas

4. Update index.html — remove the raw <canvas id="race-canvas">,
   let PixiJS create its canvas.

IMPORTANT: PixiJS v8 uses async init:
  const app = new Application();
  await app.init({ ... });
  document.body.appendChild(app.canvas);

Read the existing RaceCanvas.js, main.js, and Track.js thoroughly
before making changes. The game logic in update() must not change.
```

---

### Phase 3: Track Renderer

**Goal:** Port Track.js from Canvas 2D to PixiJS Graphics objects.

**Work:**
- Create `frontend/src/pixi/TrackRenderer.js`:
  - Extends or wraps a PixiJS Container
  - `asphaltGraphics`: filled rectangles with gradients → Graphics.rect().fill()
  - Lane dividers: dashed lines → Graphics with dash pattern
  - Start/finish lines: checkerboard → Graphics or tiling sprite
  - Token markers (50K/100K/150K): Graphics + BitmapText
  - Top/bottom shadows: Graphics with alpha gradients
  - Pit lane background + chevron corridor
  - Parking lot background
- Port crowd/spectator rendering:
  - Each spectator → small Graphics or pre-rendered Sprite
  - Crowd container with parallax offset
  - Bounce animation driven by excitement level
- Port pennant flags with wave animation
- Remove old Track.js Canvas 2D code

**Can parallel with:** Phases 4, 5 (after Phase 2 merges)

**Acceptance:**
- Track renders identically to current appearance
- Dynamic resizing works when lane counts change
- Crowd reacts to excitement level
- Pennant flags animate
- No visual regressions vs. current Canvas 2D rendering

**Seed prompt:**
```
Port the Track renderer from Canvas 2D to PixiJS v8 Graphics.

Current: Track.js (581 lines) draws directly to a Canvas 2D context.
It renders: asphalt surfaces, lane dividers, start/finish lines,
token progress markers, pit lane with chevron corridor, parking lot,
spectator crowd (2 rows), and pennant flags.

Target: TrackRenderer.js — a PixiJS Container subclass that builds
the track as a scene graph of Graphics objects and child Containers.

Key mapping:
- ctx.fillRect() with gradients → Graphics.rect().fill(color)
  (PixiJS Graphics support FillGradient for linear gradients)
- ctx.setLineDash() → manual dash segments via Graphics
- Pre-rendered texture canvas → PixiJS caches Graphics as textures
  automatically. Use cacheAsTexture() for static elements.
- ctx.drawImage(textureCanvas) → no longer needed
- Spectator circles/rects → Graphics or Sprites in a Container
- Pennant triangles → Graphics paths

Read Track.js thoroughly first. Preserve all visual details:
- Gradient colors (#333345, #2d2d40)
- Gold finish line (#d4a017)
- Checker pattern at start/finish
- Crowd skin/shirt color randomization
- Pennant wave animation

The TrackRenderer receives bounds from RaceCanvas.update() just like
Track.js does today. The public API should match:
- draw(width, height, laneCount, maxTokens, excitement)
- drawPit(width, height, activeLaneCount, pitLaneCount)
- drawParkingLot(...)
- getTrackBounds(), getPitBounds(), getParkingLotBounds()
- getLaneY(), getPositionX(), getPitEntryX()

The bounds/layout calculation methods can be copied directly —
only the drawing code changes.
```

---

### Phase 4: Racer Entity

**Goal:** Port Racer.js from Canvas 2D to PixiJS Container with children.

This is the largest migration unit (1,134 lines). Each racer becomes a
Container with child Graphics for body, wheels, effects, labels.

**Work:**
- Create `frontend/src/pixi/RacerSprite.js`:
  - Extends Container
  - Children:
    - `shadowGraphics` — ellipse under car
    - `glowGraphics` — radial gradient aura (use BlurFilter on a circle)
    - `bodyGraphics` — car body polygon (the complex path)
    - `windshieldGraphics` — transparent overlay
    - `wheelContainers[2]` — each with tire circle + hub + rotating spokes
    - `headlightGraphics` — cone beam for tool_use
    - `taillightGraphics` — red glow
    - `effectsContainer`:
      - `thoughtBubble` — rounded rect + animated dots
      - `hammerContainer` — rotating hammer for tool_use
      - `hazardLights` — pulsing orange circles
      - `checkerFlag` — waving flag for completion
    - `labelContainer`:
      - `directoryFlag` — swallowtail pennant with text
      - `modelDecal` — source badge circle
      - `metricsLabel` — rounded pill with stats
  - Port `animate()` logic:
    - Smooth lerp position interpolation
    - Spring suspension (springStiffness: 0.15, springDamping: 0.92)
    - Zone transition waypoint following
    - Wheel rotation synced to dx
    - Activity-specific timers and phases
  - Port all activity-state visual effects:
    - thinking: thought bubble + exhaust particles
    - tool_use: headlight beam + hammer swing + sparks
    - waiting: hazard light pulse
    - complete: gold tint + checker flag + confetti
    - errored: skid → spin → smoke → grayscale (multi-stage)
    - lost: fade to ghost + position trail
    - churning: subtle idle animation

**Can parallel with:** Phases 3, 5 (after Phase 2 merges)

**Acceptance:**
- All 7 activity states render correctly
- Error multi-stage animation (skid → spin → smoke → grayscale) works
- Spring suspension bounces on activity transitions
- Zone transitions animate through waypoints
- Pit/parking lot dimming and scale work
- Click hit testing works on racers
- Hover cursor changes work
- Directory flags, model decals, metrics labels display correctly

**Seed prompt:**
```
Port the Racer entity from Canvas 2D to a PixiJS v8 Container.

Current: Racer.js (1,134 lines) draws a side-profile racing car with
activity-based visual effects using Canvas 2D immediate-mode drawing.

Target: RacerSprite.js — a PixiJS Container subclass with child
Graphics/Sprites for each visual element.

IMPORTANT: This is the most complex rendering unit. Read Racer.js
completely before starting. Key sections:

1. Animation (lines 227-410): Position lerping, spring physics,
   zone transition waypoints, wheel rotation. This logic stays
   almost identical — just update this.position.set(x, y) on the
   Container instead of storing displayX/displayY.

2. Car body (lines 500-621): Complex polygon path. Convert to
   Graphics with the same vertices. The path starts at rear bottom
   and traces the car profile with lineTo + quadraticCurveTo.

3. Activity effects: Each activity has unique visuals. Build each
   as a child Container that gets shown/hidden based on activity:
   - thinking: thought bubble (rounded rect) with 3 animated dots
   - tool_use: headlight cone (radial gradient → use BlurFilter on
     a white circle instead), hammer swing (rotating Graphics)
   - waiting: hazard lights (pulsing circles with glow)
   - complete: gold tint (use tint property), checker flag
   - errored: 4-stage animation (rotation, scale, alpha, grayscale)
   - lost: alpha fade + ghost trail

4. Labels: directory flag (swallowtail pennant), source badge
   (colored circle with letter), metrics pill (rounded rect + text)

Key PixiJS patterns:
- Position: container.position.set(x, y) instead of displayX/displayY
- Rotation: container.rotation = angle (for error spin)
- Scale: container.scale.set(s) (for pit/parking lot)
- Alpha: container.alpha = value (for pit dimming, lost fading)
- Tint: graphics.tint = 0xFFD700 (for gold completion tint)
- Visibility: child.visible = false (hide inactive effects)
- Filters: [new BlurFilter()] for glow effects

For the glow aura, use a circle Sprite with BlurFilter and
blendMode = 'add' instead of the manual radial gradient.

The public API should match Racer.js:
- constructor(state)
- update(state) — update session state
- animate(particles, dt) — physics + visual state updates
- draw is implicit (PixiJS renders the Container tree)
- setTarget(x, y), startZoneTransition(waypoints)
- hovered, inPit, inParkingLot properties
```

---

### Phase 5: Particle System

**Goal:** Replace custom Particles.js with PixiJS particle emitters.

**Work:**
- Evaluate particle approach:
  - Option A: Use `@spd789562/particle-emitter` (v8-compatible fork)
  - Option B: Custom particle pool using PixiJS Graphics + Container
    (simpler, no extra dependency, follows current architecture)
  - **Recommend Option B** — the current system is only 271 lines and
    the presets are domain-specific. A thin wrapper over PixiJS Graphics
    in a ParticleContainer will be more maintainable than configuring
    a generic emitter for 8 custom presets.
- Create `frontend/src/pixi/ParticleSystem.js`:
  - Pool of Graphics objects in a Container
  - Port 8 presets: exhaust, sparks, smoke, confetti, speedLines,
    celebration, skidMarks
  - Two render layers: `behindContainer` and `frontContainer`
  - Same emit(preset, x, y, options) API
  - Physics: velocity, gravity, flutter, life decay, bloom curve
  - Circle vs rectangle vs rotated-rectangle rendering modes
  - Color interpolation (start → end)

**Can parallel with:** Phases 3, 4 (after Phase 2 merges)

**Acceptance:**
- All 8 particle presets produce identical visual results
- Particles layer correctly (exhaust behind racers, confetti in front)
- Life decay and alpha curves match current behavior
- Flutter oscillation on confetti works
- No particle leaks (particles removed when life <= 0)

**Seed prompt:**
```
Port the particle system from Canvas 2D to PixiJS v8.

Current: Particles.js (271 lines) — a simple particle emitter with
8 presets, layer-sorted rendering (behind/front), and physics
(velocity, gravity, flutter, life decay).

Target: ParticleSystem.js — PixiJS Container-based particle system.

Approach: Use two Containers (behindContainer, frontContainer) with
pooled Graphics children. Do NOT use @pixi/particle-emitter — the
current system is simple enough that a direct port is cleaner.

Read Particles.js thoroughly. Key details:

Presets and their properties:
- exhaust: gray circles, 4-8px, 'bloom' life curve (grow then shrink)
- sparks: yellow→orange circles, 1-3px, gravity, fast decay
- smoke: large gray circles 8-16px, slow fade, slight upward drift
- confetti: rainbow, rotates, gravity, flutter oscillation
- speedLines: directional rects, model-tinted color
- celebration: mix of confetti + tall thin streamer rects
- skidMarks: small dark circles, minimal motion, long life

Physics per particle:
- x, y, vx, vy — position and velocity
- gravity — applied to vy each frame
- life — 0→1 decay, particle removed at 0
- decay — rate of life decrease per second
- size, width, height — dimensions
- rotation, rotationSpeed — for confetti/streamers
- flutter, flutterSpeed — sine wave x-oscillation
- color (r,g,b), endColor — interpolated over life
- shape: 'circle' | 'rect'
- layer: 'behind' | 'front'
- lifeCurve: 'linear' | 'bloom' — bloom = size peaks mid-life

PixiJS mapping:
- Each particle → a Graphics object (circle or rect)
- Position → graphics.position.set(x, y)
- Rotation → graphics.rotation
- Alpha → graphics.alpha (based on life + curve)
- Scale → graphics.scale.set(sizeMult) (based on life curve)
- Color → graphics.tint (interpolate start→end as hex)
- Add to behindContainer or frontContainer based on layer

Pool management:
- Pre-allocate a pool of Graphics objects
- On emit: grab from pool, configure, add to container
- On death (life <= 0): remove from container, return to pool

Public API (same as current):
- emit(preset, x, y, overrides)
- update(dt)
- clear()
```

---

### Phase 6: Bloom + Screen Effects

**Goal:** Replace the custom bloom pass and screen effects with PixiJS
filters and Container transforms.

**Work:**
- Replace `_drawBloom()`:
  - Create a `glowContainer` that holds glow sprites per racer
  - Apply BlurFilter to the container
  - Set blendMode = 'add' for additive compositing
  - Remove the offscreen glow canvas entirely
- Replace screen shake:
  - Apply transform offset to app.stage (or a shake wrapper container)
  - Same intensity decay math, just use container.position instead of
    ctx.translate
- Replace flash effect:
  - White Graphics overlay rectangle
  - Animate alpha from 0.3 → 0 on completion events
- Replace connection overlay:
  - DOM overlay (simpler, avoids polluting the scene graph)
  - Or Graphics rectangle with text

**Depends on:** Phase 4 (bloom draws racer glow positions)

**Acceptance:**
- Glow auras visible around active racers
- Headlight glow on tool_use
- Hazard glow on waiting
- Screen shakes on error events
- White flash on completion events
- Connection overlay displays when WebSocket is disconnected

**Seed prompt:**
```
Port bloom/glow, screen shake, and flash effects to PixiJS v8.

Current implementation in RaceCanvas.js:
- _drawBloom(): Draws racer glows to a 1/4 resolution offscreen canvas,
  then composites back with globalCompositeOperation = 'lighter' at 50%
  alpha. This creates a cheap blur/glow effect.
- Screen shake: ctx.translate() with random offset, decaying intensity
- Flash: white fillRect with decaying alpha on completion

PixiJS replacement:

1. BLOOM: Create a glowLayer Container between racerContainer and
   particlesFront. For each racer, add a circle Sprite/Graphics to
   glowLayer at the racer's position. Apply BlurFilter to the entire
   glowLayer. Set glowLayer.blendMode = 'add'.

   Per-racer glow types (from _drawBloom):
   - Aura: white circle, radius 35, alpha = glowIntensity * 2
   - Headlight (tool_use): warm circle at x+21, radius 20, alpha 0.4
   - Hazard (waiting): orange circle, radius 25, alpha 0.3 * sin(phase)

   Update glow positions each frame from racer positions.

2. SCREEN SHAKE: Wrap app.stage children in a shakeContainer.
   On error: set shakeContainer.position to random offsets with
   linearly decaying intensity (same math as current shakeIntensity *
   (1 - progress)).

3. FLASH: Add a white Graphics rect covering the full screen to
   uiContainer. Set alpha = 0.3 on completion, decay by dt * 1.5
   each frame.

4. CONNECTION OVERLAY: Use a DOM element overlaying the canvas
   (position: absolute, pointer-events: none). Show/hide based on
   WebSocket connection state. This keeps UI text crisp and out of
   the game scene graph.

Read RaceCanvas.js _drawBloom() (lines 439-501), screen shake
(lines 337-365), and flash (lines 343-345) before starting.
```

---

### Phase 7: Dashboard to DOM

**Goal:** Extract Dashboard from canvas rendering to HTML/CSS.

**Work:**
- Create `frontend/src/ui/Dashboard.js` (DOM-based):
  - Stats bar: RACING / PIT / PARKED / TOKENS / TOOLS / MSGS
  - Leaderboard table: rank, name, model, tokens, context bar, elapsed
  - Activity indicator dots per session
  - Context utilization bar with color thresholds (green/amber/red)
- Style with CSS (dark theme matching current look)
- Remove canvas Dashboard.js
- Position below the PixiJS canvas element (or as a flex sibling)
- Update from session state via a lightweight `render(sessions)` method

**Can parallel with:** Phases 3–6 (independent of PixiJS rendering)

**Acceptance:**
- Stats display matches current canvas Dashboard
- Leaderboard sorts by context utilization
- Context bars color-coded (green < 50%, amber 50-80%, red > 80%)
- Activity dots show correct colors
- 12-row max with proper truncation
- Responsive to window resize

**Seed prompt:**
```
Extract the Dashboard from canvas rendering to DOM HTML/CSS.

Current: Dashboard.js (261 lines) draws a stats bar and leaderboard
table directly onto the canvas using ctx.fillText, ctx.fillRect, etc.

This is purely text + rectangles — it gains nothing from being on
canvas and would be sharper, more accessible, and easier to style as
DOM elements.

Target: frontend/src/ui/DashboardPanel.js — a class that creates and
manages DOM elements for the dashboard.

Structure:
1. Stats bar (top section):
   - 6 stat boxes in a row: RACING, PIT, PARKED, TOKENS, TOOLS, MSGS
   - Each shows a label + numeric value
   - Use a flexbox row

2. Leaderboard (below stats):
   - Table/grid with columns: Rank, Name, Model, Tokens, Context, Time
   - Max 12 rows
   - Sorted by contextUtilization descending
   - Activity dot (colored circle) before name:
     - thinking: #00ff88
     - tool_use: #ffaa00
     - waiting: #ff6600
     - idle/starting: #666
     - complete: #00ff88
     - errored: #e94560
   - Context bar: div with width = utilization%, colored:
     - green (#00ff88) < 50%
     - amber (#ffaa00) 50-80%
     - red (#e94560) > 80%
   - Elapsed time: format as Xm Ys from startedAt

Style: Dark theme matching the canvas aesthetic:
- Background: transparent (sits below the canvas)
- Text: #aaa for labels, #ddd for values
- Font: Courier New, monospace
- Subtle borders: rgba(255,255,255,0.06)

Public API:
- constructor(containerElement) — builds DOM structure
- update(sessions) — receives array of session state objects
- destroy() — removes DOM elements

Read the current Dashboard.js to match all visual details exactly.
```

---

### Phase 8: Integration + Cleanup

**Goal:** Wire everything together, remove legacy code, verify end-to-end.

**Work:**
- Update `main.js` to use new PixiJS components:
  - Initialize PixiApp
  - Wire WebSocket → RaceCanvas.update() → scene graph
  - Wire SoundEngine excitement to crowd animation
  - Wire click/hover events through PixiJS interaction system
  - Wire detail flyout positioning to racer Container positions
- Remove legacy files:
  - `canvas/Track.js` → replaced by `pixi/TrackRenderer.js`
  - `canvas/Particles.js` → replaced by `pixi/ParticleSystem.js`
  - `canvas/Dashboard.js` → replaced by `ui/DashboardPanel.js`
  - `entities/Racer.js` → replaced by `pixi/RacerSprite.js`
  - Remove bloom/glow offscreen canvas code from RaceCanvas
  - Remove the Canvas 2D bridge texture from Phase 2
- Update `RaceCanvas.js`:
  - Becomes a thin orchestrator: owns the PixiApp, manages racers
    map, delegates zone partitioning to update(), delegates rendering
    to the scene graph
  - Hit testing uses PixiJS eventMode/interactive instead of manual
    distance checks
- Performance audit:
  - Verify WebGL context (not falling back to Canvas)
  - Check GPU memory usage
  - Profile with 10+ concurrent sessions
  - Verify no memory leaks on racer add/remove cycles
- Visual regression check:
  - Screenshot comparison of all activity states
  - Zone transitions (track → pit → parking lot → track)
  - Error multi-stage animation
  - Completion confetti + flag
  - Crowd excitement response

**Depends on:** All Phases 2–7

**Seed prompt:**
```
Final integration of the PixiJS migration for Agent Racer.

All rendering components have been ported:
- pixi/TrackRenderer.js (track, pit, parking lot backgrounds)
- pixi/RacerSprite.js (car entities with activity effects)
- pixi/ParticleSystem.js (8 particle presets)
- Bloom/glow via BlurFilter
- Screen shake via container transforms
- ui/DashboardPanel.js (DOM-based stats + leaderboard)

Your job:
1. Wire main.js to use the new PixiJS components
2. Remove all legacy Canvas 2D rendering code
3. Remove the Canvas 2D bridge texture from Phase 2
4. Update RaceCanvas.js to be a thin orchestrator
5. Switch hit testing to PixiJS interaction (eventMode: 'static')
6. Verify SoundEngine integration still works
7. Test all activity states, zone transitions, and effects
8. Performance check: confirm WebGL context is active

Do NOT change game logic, WebSocket protocol, or SoundEngine.
Focus only on wiring and cleanup.
```

---

## Team Topology

```
Team Lead (you)
├── agent-1: Phase 1 — Build Infrastructure
│   (blocks everything, must merge first)
├── agent-2: Phase 2 — Application Shell
│   (blocks 3-7, must merge second)
├── agent-3: Phase 3 — Track Renderer        ─┐
├── agent-4: Phase 4 — Racer Entity           ─┼─ parallel after Phase 2
├── agent-5: Phase 5 — Particle System        ─┤
├── agent-7: Phase 7 — Dashboard to DOM       ─┘
├── agent-6: Phase 6 — Bloom + Screen Effects
│   (depends on Phase 4 for racer glow positions)
└── agent-8: Phase 8 — Integration + Cleanup
    (depends on all of 2-7)
```

### Execution order

```
Time →

Phase 1 ████
              Phase 2 ████████
                              Phase 3 ████████  ─┐
                              Phase 4 ████████████  ─┼─ parallel
                              Phase 5 ██████    ─┤
                              Phase 7 ██████    ─┘
                                        Phase 6 ██████  (after 4)
                                                    Phase 8 ████████
```

### Spawning commands

Each phase runs in its own worktree via `/tmux-spawn`:

```bash
# Sequential (must merge before next)
/tmux-spawn Phase 1: Add Vite + PixiJS build tooling to frontend
# → merge, then:
/tmux-spawn Phase 2: Create PixiJS Application shell with scene graph

# → merge, then parallel:
/tmux-spawn Phase 3: Port Track.js to PixiJS TrackRenderer
/tmux-spawn Phase 4: Port Racer.js to PixiJS RacerSprite
/tmux-spawn Phase 5: Port Particles.js to PixiJS ParticleSystem
/tmux-spawn Phase 7: Extract Dashboard from canvas to DOM

# → after Phase 4 merges:
/tmux-spawn Phase 6: Port bloom, screen shake, flash to PixiJS filters

# → after all merge:
/tmux-spawn Phase 8: Final integration and legacy code cleanup
```

### Beads issue structure

Create an epic with child tasks:

```bash
bd create --title="Migrate frontend to PixiJS v8" --type=epic -p 1
# → bd-XXX (epic)

bd create --title="Add Vite + PixiJS build infrastructure" --type=task -p 1
bd create --title="Create PixiJS Application shell + scene graph" --type=task -p 1
bd create --title="Port Track renderer to PixiJS Graphics" --type=task -p 2
bd create --title="Port Racer entity to PixiJS Container" --type=task -p 1
bd create --title="Port Particle system to PixiJS" --type=task -p 2
bd create --title="Port bloom/shake/flash to PixiJS filters" --type=task -p 2
bd create --title="Extract Dashboard to DOM" --type=task -p 2
bd create --title="Integration, cleanup, and visual regression check" --type=task -p 1

# Dependencies (bd dep add <issue> <depends-on>)
bd dep add <phase-2> <phase-1>
bd dep add <phase-3> <phase-2>
bd dep add <phase-4> <phase-2>
bd dep add <phase-5> <phase-2>
bd dep add <phase-6> <phase-4>
bd dep add <phase-7> <phase-2>
bd dep add <phase-8> <phase-3>
bd dep add <phase-8> <phase-4>
bd dep add <phase-8> <phase-5>
bd dep add <phase-8> <phase-6>
bd dep add <phase-8> <phase-7>
```

---

## Risk Register

| Risk | Mitigation |
|------|-----------|
| PixiJS v8 particle-emitter not compatible | Use custom particle pool (Phase 5 Option B) — already recommended |
| Visual regressions hard to catch | Screenshot before/after comparison in Phase 8 |
| WebGL context lost on some hardware | PixiJS v8.16 has experimental Canvas fallback; test on target machines |
| Bundle size increase | Tree-shake with Vite; PixiJS v8 is significantly smaller than v7 |
| Phase 2 bridge (Canvas→Sprite texture) is slow | It's temporary; only exists during the transition. Remove in Phase 8 |
| Racer hit testing changes | PixiJS eventMode:'static' + hitArea works; test in Phase 8 |
| Dashboard DOM layout shifts | Use fixed/absolute positioning below canvas; test responsive |

## File Mapping

| Current File | New File | Phase |
|-------------|----------|-------|
| — | `frontend/package.json` | 1 |
| — | `frontend/vite.config.js` | 1 |
| — | `frontend/src/PixiApp.js` | 2 |
| `frontend/src/canvas/RaceCanvas.js` | Modified (thin orchestrator) | 2, 8 |
| `frontend/src/canvas/Track.js` | `frontend/src/pixi/TrackRenderer.js` | 3 |
| `frontend/src/entities/Racer.js` | `frontend/src/pixi/RacerSprite.js` | 4 |
| `frontend/src/canvas/Particles.js` | `frontend/src/pixi/ParticleSystem.js` | 5 |
| Bloom code in RaceCanvas.js | Removed; BlurFilter in Phase 6 | 6 |
| `frontend/src/canvas/Dashboard.js` | `frontend/src/ui/DashboardPanel.js` | 7 |
| `frontend/src/audio/SoundEngine.js` | Unchanged | — |
| `frontend/src/websocket.js` | Unchanged | — |
| `frontend/src/notifications.js` | Unchanged | — |
| `frontend/src/main.js` | Modified (wiring) | 2, 8 |
| `frontend/index.html` | Modified (Vite entry, remove raw canvas) | 1, 2 |
