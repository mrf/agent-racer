import { ParticleSystem } from './Particles.js';
import { Track } from './Track.js';
import { Dashboard } from './Dashboard.js';
import { Racer } from '../entities/Racer.js';

const DEFAULT_CONTEXT_WINDOW = 200000;
const TERMINAL_ACTIVITIES = new Set(['complete', 'errored', 'lost']);

// Rectangular hit area matching the limo's elongated shape.
// Derived from car geometry: CAR_SCALE=2.3, LIMO_STRETCH=35.
const HIT_LEFT = 125;   // rear of limo: (17+35)*2.3 ≈ 120 + padding
const HIT_RIGHT = 60;   // nose of car: 23*2.3 ≈ 53 + padding
const HIT_TOP = 28;     // roof: 9*2.3 ≈ 21 + padding
const HIT_BOTTOM = 28;  // wheels: 10*2.3 ≈ 23 + padding

function isInsideHitbox(dx, dy) {
  return dx >= -HIT_LEFT && dx <= HIT_RIGHT && dy >= -HIT_TOP && dy <= HIT_BOTTOM;
}

// How long after the last data receipt to keep a session on track.
// Bridges brief gaps between parsed entries during active agent loops.
const DATA_FRESHNESS_MS = 30_000;

function isParkingLotRacer(state) {
  return TERMINAL_ACTIVITIES.has(state.activity);
}

const PIT_ACTIVITIES = new Set(['idle', 'waiting', 'starting']);

// Determines whether a racer belongs in the pit lane. Uses data freshness
// as the sole zone determinant — isChurning is reserved for visual effects
// only, since CPU jitter caused track/pit oscillation.
function isPitRacer(state) {
  if (isParkingLotRacer(state)) return false;
  if (!PIT_ACTIVITIES.has(state.activity)) return false;

  if (state.lastDataReceivedAt) {
    const age = Date.now() - new Date(state.lastDataReceivedAt).getTime();
    if (age < DATA_FRESHNESS_MS) return false;
  }
  return true;
}

export class RaceCanvas {
  constructor(canvas, engine = null) {
    this.canvas = canvas;
    this.ctx = canvas.getContext('2d');
    this.track = new Track();
    this.dashboard = new Dashboard();
    this.particles = new ParticleSystem();
    this.racers = new Map();
    this.connected = false;
    this.animFrameId = null;
    this.onRacerClick = null;
    this.onAfterDraw = null;
    this.engine = engine;

    // Timing for dt-based animation
    this.lastFrameTime = 0;
    this.dt = 1 / 60;

    // Glow/bloom offscreen canvas
    this.glowCanvas = document.createElement('canvas');
    this.glowCtx = this.glowCanvas.getContext('2d');

    // Screen shake state
    this.shakeIntensity = 0;
    this.shakeDuration = 0;
    this.shakeTimer = 0;

    // Flash effect state
    this.flashAlpha = 0;

    // Track lane counts for dynamic canvas resizing
    this._activeLaneCount = 1;
    this._pitLaneCount = 0;
    this._parkingLotLaneCount = 0;
    this._trackGroups = [{ maxTokens: DEFAULT_CONTEXT_WINDOW, laneCount: 1 }];
    this._trackGroupsKey = '';
    this._zoneCounts = { racing: 0, pit: 0, parked: 0 };

    this.resize();
    this._resizeHandler = () => this.resize();
    this._clickHandler = (e) => this.handleClick(e);
    this._mouseMoveHandler = (e) => this.handleMouseMove(e);
    window.addEventListener('resize', this._resizeHandler);
    this.canvas.addEventListener('click', this._clickHandler);
    this.canvas.addEventListener('mousemove', this._mouseMoveHandler);
    this.startLoop();
  }

  setEngine(engine) {
    this.engine = engine;
  }

  resize() {
    const dpr = window.devicePixelRatio || 1;
    const rect = this.canvas.parentElement.getBoundingClientRect();
    const viewportWidth = rect.width;
    const viewportHeight = rect.height;

    this.track.updateViewport(viewportHeight);

    // Track zones height (track + pit + parking lot)
    const zonesHeight = this.track.getRequiredHeight(this._trackGroups, this._pitLaneCount, this._parkingLotLaneCount);

    // Dashboard fills remaining viewport space, with a guaranteed minimum
    const dashMinHeight = this.dashboard.getRequiredHeight(this.racers.size);
    const dashFromViewport = Math.max(0, viewportHeight - zonesHeight);
    const dashHeight = Math.max(dashMinHeight, dashFromViewport);
    const height = zonesHeight + dashHeight;

    this.canvas.style.height = height + 'px';
    this.canvas.width = viewportWidth * dpr;
    this.canvas.height = height * dpr;
    this.ctx.scale(dpr, dpr);
    this.width = viewportWidth;
    this.height = height;

    // Resize glow canvas to match (at reduced resolution for blur)
    this.glowCanvas.width = Math.ceil(viewportWidth / 4);
    this.glowCanvas.height = Math.ceil(height / 4);
  }

  setConnected(connected) {
    this.connected = connected;
  }

  setAllRacers(sessions) {
    const newIds = new Set(sessions.map(s => s.id));

    // Remove racers no longer present
    for (const id of this.racers.keys()) {
      if (!newIds.has(id)) {
        this.racers.delete(id);
      }
    }

    // Add or update
    for (const s of sessions) {
      if (this.racers.has(s.id)) {
        this.racers.get(s.id).update(s);
      } else {
        this.racers.set(s.id, new Racer(s));
      }
    }
  }

  updateRacer(state) {
    if (this.racers.has(state.id)) {
      this.racers.get(state.id).update(state);
    } else {
      this.racers.set(state.id, new Racer(state));
    }
  }

  removeRacer(id) {
    this.racers.delete(id);
  }

  onComplete(sessionId) {
    const racer = this.racers.get(sessionId);
    if (racer) {
      racer.confettiEmitted = false;
    }
    // Flash effect on completion
    this.flashAlpha = 0.3;
  }

  onError(sessionId) {
    const racer = this.racers.get(sessionId);
    if (racer) {
      racer.smokeEmitted = false;
      racer.skidEmitted = false;
      racer.errorTimer = 0;
      racer.errorStage = 0;
    }
    // Screen shake on error
    this.shakeIntensity = 6;
    this.shakeDuration = 0.3;
    this.shakeTimer = 0;
  }

  startLoop() {
    const loop = (timestamp) => {
      // Compute delta time
      if (this.lastFrameTime === 0) {
        this.lastFrameTime = timestamp;
      }
      const rawDt = (timestamp - this.lastFrameTime) / 1000;
      this.dt = Math.min(rawDt, 0.05); // Cap at 50ms to avoid spiral
      this.lastFrameTime = timestamp;

      this.update();
      this.draw();
      if (this.onAfterDraw) this.onAfterDraw();
      this.animFrameId = requestAnimationFrame(loop);
    };
    this.animFrameId = requestAnimationFrame(loop);
  }

  update() {
    const dt = this.dt;

    // Partition racers into track, pit, and parking lot groups
    const trackRacers = [];
    const pitRacers = [];
    const parkingLotRacers = [];

    for (const racer of this.racers.values()) {
      if (isParkingLotRacer(racer.state)) {
        parkingLotRacers.push(racer);
      } else if (isPitRacer(racer.state)) {
        pitRacers.push(racer);
      } else {
        trackRacers.push(racer);
      }
    }

    const pitLaneCount = pitRacers.length;
    const parkingLotLaneCount = parkingLotRacers.length;

    // Store zone counts for dashboard (single source of truth)
    this._zoneCounts = {
      racing: trackRacers.length,
      pit: pitLaneCount,
      parked: parkingLotLaneCount,
    };

    // Group track racers by maxContextTokens for separate tracks
    const groupMap = new Map();
    for (const racer of trackRacers) {
      const maxTokens = racer.state.maxContextTokens || DEFAULT_CONTEXT_WINDOW;
      if (!groupMap.has(maxTokens)) groupMap.set(maxTokens, []);
      groupMap.get(maxTokens).push(racer);
    }
    const sortedGroupEntries = [...groupMap.entries()].sort(([a], [b]) => a - b);
    const trackGroups = sortedGroupEntries.length > 0
      ? sortedGroupEntries.map(([maxTokens, racers]) => ({ maxTokens, laneCount: racers.length }))
      : [{ maxTokens: DEFAULT_CONTEXT_WINDOW, laneCount: 1 }];

    // Resize canvas when group composition or lane counts change
    const groupsKey = trackGroups.map(g => `${g.maxTokens}:${g.laneCount}`).join(',');
    if (groupsKey !== this._trackGroupsKey ||
        pitLaneCount !== this._pitLaneCount ||
        parkingLotLaneCount !== this._parkingLotLaneCount) {
      this._trackGroups = trackGroups;
      this._trackGroupsKey = groupsKey;
      this._activeLaneCount = trackGroups.reduce((sum, g) => sum + g.laneCount, 0) || 1;
      this._pitLaneCount = pitLaneCount;
      this._parkingLotLaneCount = parkingLotLaneCount;
      this.resize();
    }

    // Compute globalMaxTokens from ALL racers (track + pit + parking lot)
    // so pit/parking lot scale stays stable across zones.
    let globalMaxTokens = DEFAULT_CONTEXT_WINDOW;
    for (const racer of this.racers.values()) {
      const max = racer.state.maxContextTokens || DEFAULT_CONTEXT_WINDOW;
      if (max > globalMaxTokens) globalMaxTokens = max;
    }

    // Position track racers per group
    const layouts = this.track.getMultiTrackLayout(this.width, trackGroups);
    const lastLayout = layouts[layouts.length - 1];
    const trackZoneBottom = lastLayout.y + lastLayout.height;
    const entryX = this.track.getPitEntryX(layouts[0]);

    for (let gi = 0; gi < sortedGroupEntries.length; gi++) {
      const [groupMaxTokens, groupRacers] = sortedGroupEntries[gi];
      const layout = layouts[gi];
      const sorted = groupRacers.sort((a, b) => a.state.lane - b.state.lane);

      for (let i = 0; i < sorted.length; i++) {
        const racer = sorted[i];
        const targetX = this.track.getTokenX(layout, racer.state.tokensUsed || 0, groupMaxTokens);
        const targetY = this.track.getLaneY(layout, i);

        // Detect leaving pit or parking lot -> track transition
        if ((racer.inPit || racer.inParkingLot) && racer.initialized) {
          racer.startZoneTransition([
            { x: entryX, y: racer.displayY },
            { x: entryX, y: trackZoneBottom },
            { x: targetX, y: targetY },
          ]);
        }

        racer.setTarget(targetX, targetY);
        racer.inPit = false;
        racer.pitDimTarget = 0;
        racer.inParkingLot = false;
        racer.parkingLotDimTarget = 0;
        racer.animate(this.particles, dt);

        // Sync engine audio
        if (this.engine) {
          const activity = racer.state.activity;
          if (activity === 'thinking' || activity === 'tool_use') {
            this.engine.startEngine(racer.id, activity);
          } else if (racer.state.isChurning && (activity === 'idle' || activity === 'starting')) {
            this.engine.startEngine(racer.id, 'churning');
          } else {
            this.engine.stopEngine(racer.id);
          }
        }
      }
    }

    // Position pit racers
    if (pitLaneCount > 0) {
      const pitBounds = this.track.getPitBounds(this.width, this.height, trackGroups, pitLaneCount);
      const sortedPit = pitRacers.sort((a, b) => a.state.lane - b.state.lane);

      for (let i = 0; i < sortedPit.length; i++) {
        const racer = sortedPit[i];
        const targetX = this.track.getTokenX(pitBounds, racer.state.tokensUsed || 0, globalMaxTokens);
        const targetY = this.track.getLaneY(pitBounds, i);

        // Detect entering pit from track or parking lot
        if (!racer.inPit && racer.initialized) {
          if (racer.inParkingLot) {
            racer.startZoneTransition([
              { x: entryX, y: racer.displayY },
              { x: entryX, y: targetY },
              { x: targetX, y: targetY },
            ]);
          } else {
            racer.startZoneTransition([
              { x: entryX, y: trackZoneBottom },
              { x: entryX, y: pitBounds.y },
              { x: targetX, y: targetY },
            ]);
          }
        }

        racer.setTarget(targetX, targetY);
        racer.inPit = true;
        racer.pitDimTarget = 1;
        racer.inParkingLot = false;
        racer.parkingLotDimTarget = 0;
        racer.animate(this.particles, dt);

        if (this.engine) {
          this.engine.stopEngine(racer.id);
        }
      }
    }

    // Position parking lot racers
    if (parkingLotLaneCount > 0) {
      const lotBounds = this.track.getParkingLotBounds(this.width, this.height, trackGroups, pitLaneCount, parkingLotLaneCount);
      const sortedLot = parkingLotRacers.sort((a, b) => a.state.lane - b.state.lane);

      for (let i = 0; i < sortedLot.length; i++) {
        const racer = sortedLot[i];
        const targetX = this.track.getTokenX(lotBounds, racer.state.tokensUsed || 0, globalMaxTokens);
        const targetY = this.track.getLaneY(lotBounds, i);

        if (!racer.inParkingLot && racer.initialized) {
          racer.startZoneTransition([
            { x: entryX, y: racer.displayY },
            { x: entryX, y: lotBounds.y },
            { x: targetX, y: targetY },
          ]);
        }

        racer.setTarget(targetX, targetY);
        racer.inPit = false;
        racer.pitDimTarget = 0;
        racer.inParkingLot = true;
        racer.parkingLotDimTarget = 1;
        racer.animate(this.particles, dt);

        if (this.engine) {
          this.engine.stopEngine(racer.id);
        }
      }
    }

    this.particles.update(dt);

    // Update screen shake
    if (this.shakeTimer < this.shakeDuration) {
      this.shakeTimer += dt;
    }

    // Decay flash
    if (this.flashAlpha > 0) {
      this.flashAlpha = Math.max(0, this.flashAlpha - dt * 1.5); // fade over ~0.2s
    }
  }

  draw() {
    const ctx = this.ctx;

    // Clear
    ctx.clearRect(0, 0, this.width, this.height);

    ctx.save();

    // Apply screen shake
    if (this.shakeTimer < this.shakeDuration && this.shakeIntensity > 0) {
      const progress = this.shakeTimer / this.shakeDuration;
      const currentIntensity = this.shakeIntensity * (1 - progress); // linear decay
      const sx = (Math.random() * 2 - 1) * currentIntensity;
      const sy = (Math.random() * 2 - 1) * currentIntensity;
      ctx.translate(sx, sy);
    }

    // Background
    ctx.fillStyle = '#1a1a2e';
    ctx.fillRect(-10, -10, this.width + 20, this.height + 20);

    const pitLaneCount = this._pitLaneCount;
    const parkingLotLaneCount = this._parkingLotLaneCount;
    const groups = this._trackGroups;

    const excitement = this.engine ? this.engine.currentExcitement : 0;
    this.track.drawMultiTrack(ctx, this.width, this.height, groups, excitement);

    // Draw pit area (always visible, even when empty)
    this.track.drawPit(ctx, this.width, this.height, groups, pitLaneCount);

    // Draw parking lot area when there are parked racers
    if (parkingLotLaneCount > 0) {
      this.track.drawParkingLot(ctx, this.width, this.height, groups, pitLaneCount, parkingLotLaneCount);
    }

    // Draw dashboard below the track zones
    const zonesHeight = this.track.getRequiredHeight(groups, pitLaneCount, parkingLotLaneCount);
    const dashAvailable = this.height - zonesHeight;
    if (dashAvailable > 40) {
      const dashBounds = this.dashboard.getBounds(this.width, zonesHeight, dashAvailable);
      const sessions = [...this.racers.values()].map(r => r.state);
      this.dashboard.draw(ctx, dashBounds, sessions, this._zoneCounts);
    }

    // Draw particles behind racers
    this.particles.drawBehind(ctx);

    // Draw all racers sorted by Y for correct layering
    const sorted = [...this.racers.values()].sort((a, b) => a.displayY - b.displayY);

    // Draw racers
    for (const racer of sorted) {
      racer.draw(ctx);
    }

    // Draw particles in front of racers
    this.particles.drawFront(ctx);

    // Glow/bloom composite pass
    this._drawBloom(ctx);

    // Flash effect
    if (this.flashAlpha > 0) {
      ctx.fillStyle = `rgba(255,255,255,${this.flashAlpha})`;
      ctx.fillRect(-10, -10, this.width + 20, this.height + 20);
    }

    ctx.restore();

    // Connection overlay (drawn outside shake transform)
    if (!this.connected) {
      ctx.fillStyle = 'rgba(0,0,0,0.6)';
      ctx.fillRect(0, 0, this.width, this.height);
      ctx.fillStyle = '#e94560';
      ctx.font = 'bold 20px Courier New';
      ctx.textAlign = 'center';
      ctx.fillText('Connecting...', this.width / 2, this.height / 2);
    }

    // Empty state
    if (this.connected && this.racers.size === 0) {
      ctx.fillStyle = '#666';
      ctx.font = '16px Courier New';
      ctx.textAlign = 'center';
      ctx.fillText('No active Claude sessions detected', this.width / 2, this.height / 2 - 10);
      ctx.font = '12px Courier New';
      ctx.fillText('Start a Claude Code session to see it race', this.width / 2, this.height / 2 + 14);
    }
  }

  _drawBloom(ctx) {
    // Draw bright elements (glows, headlights) to small offscreen canvas,
    // which acts as a blur due to the downscale, then composite back.
    const gc = this.glowCtx;
    const gw = this.glowCanvas.width;
    const gh = this.glowCanvas.height;
    const scaleX = gw / this.width;
    const scaleY = gh / this.height;

    gc.clearRect(0, 0, gw, gh);

    // Draw just the glow elements at reduced resolution
    gc.save();
    gc.scale(scaleX, scaleY);

    for (const racer of this.racers.values()) {
      const x = racer.displayX;
      const y = racer.displayY + racer.springY;

      // Racer glow aura
      if (racer.glowIntensity > 0.02) {
        const glowR = 35;
        const grad = gc.createRadialGradient(x, y, 0, x, y, glowR);
        grad.addColorStop(0, `rgba(255,255,255,${racer.glowIntensity * 2})`);
        grad.addColorStop(1, 'rgba(255,255,255,0)');
        gc.fillStyle = grad;
        gc.beginPath();
        gc.arc(x, y, glowR, 0, Math.PI * 2);
        gc.fill();
      }

      // Headlight glow for tool_use
      if (racer.state.activity === 'tool_use') {
        const hlGrad = gc.createRadialGradient(x + 21, y, 0, x + 21, y, 20);
        hlGrad.addColorStop(0, 'rgba(255,255,200,0.4)');
        hlGrad.addColorStop(1, 'rgba(255,255,200,0)');
        gc.fillStyle = hlGrad;
        gc.beginPath();
        gc.arc(x + 22, y, 20, 0, Math.PI * 2);
        gc.fill();
      }

      // Hazard glow for waiting
      if (racer.state.activity === 'waiting' && Math.sin(racer.hazardPhase) > 0) {
        const hzGrad = gc.createRadialGradient(x, y, 0, x, y, 25);
        hzGrad.addColorStop(0, 'rgba(255,170,0,0.3)');
        hzGrad.addColorStop(1, 'rgba(255,170,0,0)');
        gc.fillStyle = hzGrad;
        gc.beginPath();
        gc.arc(x, y, 25, 0, Math.PI * 2);
        gc.fill();
      }
    }

    gc.restore();

    // Composite the glow canvas onto main canvas with additive blending
    ctx.save();
    ctx.globalCompositeOperation = 'lighter';
    ctx.globalAlpha = 0.5;
    ctx.drawImage(this.glowCanvas, 0, 0, gw, gh, 0, 0, this.width, this.height);
    ctx.restore();
  }

  handleClick(e) {
    const racer = this._hitTest(e);
    if (!racer) return;

    if (this.onRacerClick) {
      this.onRacerClick(racer.state);
    }
    // Focus tmux pane for non-terminal sessions with a tmux target
    if (racer.hasTmux && !TERMINAL_ACTIVITIES.has(racer.state.activity)) {
      this.focusSession(racer.state.id);
    }
  }

  handleMouseMove(e) {
    const rect = this.canvas.getBoundingClientRect();
    const mx = e.clientX - rect.left;
    const my = e.clientY - rect.top;

    let hoveredAny = false;
    for (const racer of this.racers.values()) {
      const dx = mx - racer.displayX;
      const dy = my - racer.displayY;
      racer.hovered = isInsideHitbox(dx, dy);
      if (racer.hovered && racer.hasTmux) hoveredAny = true;
    }
    this.canvas.style.cursor = hoveredAny ? 'pointer' : 'default';
  }

  _hitTest(e) {
    const rect = this.canvas.getBoundingClientRect();
    const mx = e.clientX - rect.left;
    const my = e.clientY - rect.top;

    for (const racer of this.racers.values()) {
      const dx = mx - racer.displayX;
      const dy = my - racer.displayY;
      if (isInsideHitbox(dx, dy)) {
        return racer;
      }
    }
    return null;
  }

  async focusSession(sessionId) {
    try {
      const resp = await fetch(`/api/sessions/${encodeURIComponent(sessionId)}/focus`, {
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

  destroy() {
    if (this.animFrameId) {
      cancelAnimationFrame(this.animFrameId);
      this.animFrameId = null;
    }
    window.removeEventListener('resize', this._resizeHandler);
    this.canvas.removeEventListener('click', this._clickHandler);
    this.canvas.removeEventListener('mousemove', this._mouseMoveHandler);

    // Clear racer references
    this.racers.clear();

    // Clear particle system
    this.particles.clear();

    // Release offscreen canvas
    this.glowCanvas.width = 0;
    this.glowCanvas.height = 0;
    this.glowCanvas = null;
    this.glowCtx = null;

    // Clear callback references
    this.onRacerClick = null;
    this.onAfterDraw = null;
    this.engine = null;
  }
}
