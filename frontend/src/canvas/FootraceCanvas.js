import { ParticleSystem } from './Particles.js';
import { FootraceTrack } from './FootraceTrack.js';
import { Dashboard } from './Dashboard.js';
import { WeatherSystem } from './Weather.js';
import { Character } from '../entities/Character.js';
import { getModelColor, hexToRgb } from '../session/colors.js';
import { authFetch } from '../auth.js';
import { DEFAULT_CONTEXT_WINDOW, TERMINAL_ACTIVITIES } from '../session/constants.js';
import { isParkingLotRacer, isPitRacer } from '../session/zones.js';
import { syncEngineForEntity } from '../audio/engineSync.js';

// Character hit area — smaller than car, centered on character body.
const HIT_HX = 20;
const HIT_HY = 25;

function isInsideHitbox(dx, dy) {
  return dx >= -HIT_HX && dx <= HIT_HX && dy >= -HIT_HY && dy <= HIT_HY;
}

// Hamster bounding box
const HAMSTER_HIT_HX = 10;
const HAMSTER_HIT_HY = 7.5;

function isInsideHamsterHitbox(dx, dy) {
  return dx >= -HAMSTER_HIT_HX && dx <= HAMSTER_HIT_HX && dy >= -HAMSTER_HIT_HY && dy <= HAMSTER_HIT_HY;
}

export class FootraceCanvas {
  constructor(canvas, engine = null) {
    this.canvas = canvas;
    this.ctx = canvas.getContext('2d');
    this.track = new FootraceTrack();
    this.dashboard = new Dashboard();
    this.particles = new ParticleSystem();
    this.weather = new WeatherSystem();
    this.characters = new Map();
    this.connected = false;
    this.animFrameId = null;
    this.onRacerClick = null;
    this.onHamsterClick = null;
    this.onAfterDraw = null;
    this.engine = engine;

    // Timing
    this.lastFrameTime = 0;
    this.dt = 1 / 60;

    // Glow/bloom offscreen canvas
    this.glowCanvas = document.createElement('canvas');
    this.glowCtx = this.glowCanvas.getContext('2d');

    // Screen shake state
    this.shakeIntensity = 0;
    this.shakeDuration = 0;
    this.shakeTimer = 0;

    // Flash effect
    this.flashAlpha = 0;

    // Track lane counts
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

  get entities() {
    return this.characters;
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

    const zonesHeight = this.track.getRequiredHeight(this._trackGroups, this._pitLaneCount, this._parkingLotLaneCount);
    const dashMinHeight = this.dashboard.getRequiredHeight(this.characters.size);
    const dashFromViewport = Math.max(0, viewportHeight - zonesHeight);
    const dashHeight = Math.max(dashMinHeight, dashFromViewport);
    const height = zonesHeight + dashHeight;

    this.canvas.style.height = height + 'px';
    this.canvas.width = viewportWidth * dpr;
    this.canvas.height = height * dpr;
    this.ctx.scale(dpr, dpr);
    this.width = viewportWidth;
    this.height = height;

    this.glowCanvas.width = Math.ceil(viewportWidth / 4);
    this.glowCanvas.height = Math.ceil(height / 4);
  }

  setConnected(connected) {
    this.connected = connected;
  }

  setAllRacers(sessions) {
    const newIds = new Set(sessions.map(s => s.id));
    for (const id of this.characters.keys()) {
      if (!newIds.has(id)) {
        this.characters.delete(id);
      }
    }
    for (const s of sessions) {
      if (this.characters.has(s.id)) {
        this.characters.get(s.id).update(s);
      } else {
        this.characters.set(s.id, new Character(s));
      }
    }
  }

  updateRacer(state) {
    if (this.characters.has(state.id)) {
      this.characters.get(state.id).update(state);
    } else {
      this.characters.set(state.id, new Character(state));
    }
  }

  removeRacer(id) {
    this.characters.delete(id);
  }

  onComplete(sessionId) {
    const ch = this.characters.get(sessionId);
    if (ch) {
      ch.confettiEmitted = false;
    }
    this.flashAlpha = 0.3;
  }

  onError(sessionId) {
    const ch = this.characters.get(sessionId);
    if (ch) {
      ch.stumbleEmitted = false;
      ch.starsEmitted = false;
      ch.errorTimer = 0;
      ch.errorStage = 0;
    }
    this.shakeIntensity = 6;
    this.shakeDuration = 0.3;
    this.shakeTimer = 0;
  }

  startLoop() {
    const loop = (timestamp) => {
      if (this.lastFrameTime === 0) {
        this.lastFrameTime = timestamp;
      }
      const rawDt = (timestamp - this.lastFrameTime) / 1000;
      this.dt = Math.min(rawDt, 0.05);
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

    const trackChars = [];
    const pitChars = [];
    const parkingLotChars = [];

    for (const ch of this.characters.values()) {
      if (isParkingLotRacer(ch.state)) {
        parkingLotChars.push(ch);
      } else if (isPitRacer(ch.state)) {
        pitChars.push(ch);
      } else {
        trackChars.push(ch);
      }
    }

    const pitLaneCount = pitChars.length;
    const parkingLotLaneCount = parkingLotChars.length;

    this._zoneCounts = {
      racing: trackChars.length,
      pit: pitLaneCount,
      parked: parkingLotLaneCount,
    };

    // Group track characters by maxContextTokens
    const groupMap = new Map();
    for (const ch of trackChars) {
      const maxTokens = ch.state.maxContextTokens || DEFAULT_CONTEXT_WINDOW;
      if (!groupMap.has(maxTokens)) groupMap.set(maxTokens, []);
      groupMap.get(maxTokens).push(ch);
    }
    const sortedGroupEntries = [...groupMap.entries()].sort(([a], [b]) => a - b);
    const trackGroups = sortedGroupEntries.length > 0
      ? sortedGroupEntries.map(([maxTokens, chars]) => ({ maxTokens, laneCount: chars.length }))
      : [{ maxTokens: DEFAULT_CONTEXT_WINDOW, laneCount: 1 }];

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

    // Global max tokens across all zones
    let globalMaxTokens = DEFAULT_CONTEXT_WINDOW;
    for (const ch of this.characters.values()) {
      const max = ch.state.maxContextTokens || DEFAULT_CONTEXT_WINDOW;
      if (max > globalMaxTokens) globalMaxTokens = max;
    }

    // Position track characters per group
    const layouts = this.track.getMultiTrackLayout(this.width, trackGroups);
    const lastLayout = layouts[layouts.length - 1];
    const trackZoneBottom = lastLayout.y + lastLayout.height;
    const entryX = this.track.getPitEntryX(layouts[0]);

    for (let gi = 0; gi < sortedGroupEntries.length; gi++) {
      const [groupMaxTokens, groupChars] = sortedGroupEntries[gi];
      const layout = layouts[gi];
      const sorted = groupChars.sort((a, b) => a.state.lane - b.state.lane);

      for (let i = 0; i < sorted.length; i++) {
        const ch = sorted[i];
        const targetX = this.track.getTokenX(layout, ch.state.tokensUsed || 0, groupMaxTokens);
        const targetY = this.track.getLaneY(layout, i);

        if ((ch.inPit || ch.inParkingLot) && ch.initialized) {
          ch.startZoneTransition([
            { x: entryX, y: ch.displayY },
            { x: entryX, y: trackZoneBottom },
            { x: targetX, y: targetY },
          ]);
        }

        ch.setTarget(targetX, targetY);
        ch.inPit = false;
        ch.pitDimTarget = 0;
        ch.inParkingLot = false;
        ch.parkingLotDimTarget = 0;
        ch.animate(this.particles, dt);

        syncEngineForEntity(this.engine, ch.id, ch.state, 'track');
      }
    }

    // Position pit characters
    if (pitLaneCount > 0) {
      const pitBounds = this.track.getPitBounds(this.width, this.height, trackGroups, pitLaneCount);
      const sortedPit = pitChars.sort((a, b) => a.state.lane - b.state.lane);

      for (let i = 0; i < sortedPit.length; i++) {
        const ch = sortedPit[i];
        const targetX = this.track.getTokenX(pitBounds, ch.state.tokensUsed || 0, globalMaxTokens);
        const targetY = this.track.getLaneY(pitBounds, i);

        if (!ch.inPit && ch.initialized) {
          if (ch.inParkingLot) {
            ch.startZoneTransition([
              { x: entryX, y: ch.displayY },
              { x: entryX, y: targetY },
              { x: targetX, y: targetY },
            ]);
          } else {
            ch.startZoneTransition([
              { x: entryX, y: trackZoneBottom },
              { x: entryX, y: pitBounds.y },
              { x: targetX, y: targetY },
            ]);
          }
        }

        ch.setTarget(targetX, targetY);
        ch.inPit = true;
        ch.pitDimTarget = 1;
        ch.inParkingLot = false;
        ch.parkingLotDimTarget = 0;
        ch.animate(this.particles, dt);

        syncEngineForEntity(this.engine, ch.id, ch.state, 'pit');
      }
    }

    // Position parking lot characters
    if (parkingLotLaneCount > 0) {
      const lotBounds = this.track.getParkingLotBounds(this.width, this.height, trackGroups, pitLaneCount, parkingLotLaneCount);
      const sortedLot = parkingLotChars.sort((a, b) => a.state.lane - b.state.lane);

      for (let i = 0; i < sortedLot.length; i++) {
        const ch = sortedLot[i];
        const targetX = this.track.getTokenX(lotBounds, ch.state.tokensUsed || 0, globalMaxTokens);
        const targetY = this.track.getLaneY(lotBounds, i);

        if (!ch.inParkingLot && ch.initialized) {
          ch.startZoneTransition([
            { x: entryX, y: ch.displayY },
            { x: entryX, y: lotBounds.y },
            { x: targetX, y: targetY },
          ]);
        }

        ch.setTarget(targetX, targetY);
        ch.inPit = false;
        ch.pitDimTarget = 0;
        ch.inParkingLot = true;
        ch.parkingLotDimTarget = 1;
        ch.animate(this.particles, dt);

        syncEngineForEntity(this.engine, ch.id, ch.state, 'parkingLot');
      }
    }

    this.particles.update(dt);
    this.weather.updateMetrics([...this.characters.values()].map(ch => ch.state));
    this.weather.update(dt, this.width, this.height);

    if (this.shakeTimer < this.shakeDuration) {
      this.shakeTimer += dt;
    }

    if (this.flashAlpha > 0) {
      this.flashAlpha = Math.max(0, this.flashAlpha - dt * 1.5);
    }
  }

  draw() {
    const ctx = this.ctx;
    ctx.clearRect(0, 0, this.width, this.height);
    ctx.save();

    // Screen shake
    if (this.shakeTimer < this.shakeDuration && this.shakeIntensity > 0) {
      const progress = this.shakeTimer / this.shakeDuration;
      const currentIntensity = this.shakeIntensity * (1 - progress);
      const sx = (Math.random() * 2 - 1) * currentIntensity;
      const sy = (Math.random() * 2 - 1) * currentIntensity;
      ctx.translate(sx, sy);
    }

    // Background — earthy dark green
    ctx.fillStyle = '#1a2e1a';
    ctx.fillRect(-10, -10, this.width + 20, this.height + 20);
    this.weather.drawBehind(ctx, this.width, this.height);

    const pitLaneCount = this._pitLaneCount;
    const parkingLotLaneCount = this._parkingLotLaneCount;
    const groups = this._trackGroups;

    const excitement = this.engine ? this.engine.currentExcitement : 0;
    this.track.drawMultiTrack(ctx, this.width, this.height, groups, excitement);

    this.track.drawPit(ctx, this.width, this.height, groups, pitLaneCount);

    if (parkingLotLaneCount > 0) {
      this.track.drawParkingLot(ctx, this.width, this.height, groups, pitLaneCount, parkingLotLaneCount);
    }

    // Dashboard
    const zonesHeight = this.track.getRequiredHeight(groups, pitLaneCount, parkingLotLaneCount);
    const dashAvailable = this.height - zonesHeight;
    if (dashAvailable > 40) {
      const dashBounds = this.dashboard.getBounds(this.width, zonesHeight, dashAvailable);
      const sessions = [...this.characters.values()].map(ch => ch.state);
      this.dashboard.draw(ctx, dashBounds, sessions, this._zoneCounts);
    }

    // Particles behind
    this.particles.drawBehind(ctx);

    // Characters sorted by Y
    const sorted = [...this.characters.values()].sort((a, b) => a.displayY - b.displayY);
    for (const ch of sorted) {
      ch.draw(ctx);
    }

    // Particles in front
    this.particles.drawFront(ctx);
    this.weather.drawFront(ctx, this.width, this.height);

    // Bloom
    this._drawBloom(ctx);

    // Flash
    if (this.flashAlpha > 0) {
      ctx.fillStyle = `rgba(255,255,255,${this.flashAlpha})`;
      ctx.fillRect(-10, -10, this.width + 20, this.height + 20);
    }

    ctx.restore();

    // Connection overlay
    if (!this.connected) {
      ctx.fillStyle = 'rgba(0,0,0,0.6)';
      ctx.fillRect(0, 0, this.width, this.height);
      ctx.fillStyle = '#e94560';
      ctx.font = 'bold 20px Courier New';
      ctx.textAlign = 'center';
      ctx.fillText('Connecting...', this.width / 2, this.height / 2);
    }

    // Empty state
    if (this.connected && this.characters.size === 0) {
      ctx.fillStyle = '#8a8';
      ctx.font = '16px Courier New';
      ctx.textAlign = 'center';
      ctx.fillText('No active Claude sessions detected', this.width / 2, this.height / 2 - 10);
      ctx.font = '12px Courier New';
      ctx.fillText('Start a Claude Code session to see it race', this.width / 2, this.height / 2 + 14);
    }
  }

  _drawBloom(ctx) {
    const gc = this.glowCtx;
    const gw = this.glowCanvas.width;
    const gh = this.glowCanvas.height;
    const scaleX = gw / this.width;
    const scaleY = gh / this.height;

    gc.clearRect(0, 0, gw, gh);
    gc.save();
    gc.scale(scaleX, scaleY);

    for (const ch of this.characters.values()) {
      const x = ch.displayX;
      const y = ch.displayY + ch.springY;

      if (ch.glowIntensity > 0.02) {
        const glowR = 25;
        const grad = gc.createRadialGradient(x, y, 0, x, y, glowR);
        grad.addColorStop(0, `rgba(255,255,255,${ch.glowIntensity * 2})`);
        grad.addColorStop(1, 'rgba(255,255,255,0)');
        gc.fillStyle = grad;
        gc.beginPath();
        gc.arc(x, y, glowR, 0, Math.PI * 2);
        gc.fill();
      }

      // Hamster glow
      if (ch.hamsters) {
        for (const hamster of ch.hamsters.values()) {
          if (!hamster.glowIntensity || hamster.glowIntensity <= 0.02) continue;
          const hx = hamster.displayX;
          const hy = hamster.displayY + (hamster.springY || 0);
          const rgb = hexToRgb(getModelColor(hamster.state.model, hamster.state.source).main);
          const glowR = 15;
          const grad = gc.createRadialGradient(hx, hy, 0, hx, hy, glowR);
          grad.addColorStop(0, `rgba(${rgb.r},${rgb.g},${rgb.b},${hamster.glowIntensity * 2})`);
          grad.addColorStop(1, `rgba(${rgb.r},${rgb.g},${rgb.b},0)`);
          gc.fillStyle = grad;
          gc.beginPath();
          gc.arc(hx, hy, glowR, 0, Math.PI * 2);
          gc.fill();
        }
      }
    }

    gc.restore();

    ctx.save();
    ctx.globalCompositeOperation = 'lighter';
    ctx.globalAlpha = 0.5;
    ctx.drawImage(this.glowCanvas, 0, 0, gw, gh, 0, 0, this.width, this.height);
    ctx.restore();
  }

  handleClick(e) {
    const hit = this._hitTest(e);
    if (!hit) return;

    if (hit.type === 'hamster') {
      if (this.onHamsterClick) {
        this.onHamsterClick({
          hamsterState: hit.hamster.state,
          parentState: hit.character.state,
        });
      }
      return;
    }

    const ch = hit.character;
    if (this.onRacerClick) {
      this.onRacerClick(ch.state);
    }
    if (ch.hasTmux && !TERMINAL_ACTIVITIES.has(ch.state.activity)) {
      this.focusSession(ch.state.id);
    }
  }

  handleMouseMove(e) {
    const rect = this.canvas.getBoundingClientRect();
    const mx = e.clientX - rect.left;
    const my = e.clientY - rect.top;

    let hoveredAny = false;

    for (const ch of this.characters.values()) {
      if (!ch.hamsters) continue;
      for (const hamster of ch.hamsters.values()) {
        const dx = mx - hamster.displayX;
        const dy = my - hamster.displayY;
        if (isInsideHamsterHitbox(dx, dy)) {
          hoveredAny = true;
        }
      }
    }

    for (const ch of this.characters.values()) {
      const dx = mx - ch.displayX;
      const dy = my - ch.displayY;
      ch.hovered = isInsideHitbox(dx, dy);
      if (ch.hovered && ch.hasTmux) hoveredAny = true;
    }
    this.canvas.style.cursor = hoveredAny ? 'pointer' : 'default';
  }

  _hitTest(e) {
    const rect = this.canvas.getBoundingClientRect();
    const mx = e.clientX - rect.left;
    const my = e.clientY - rect.top;

    // Hamster hitboxes first
    for (const ch of this.characters.values()) {
      if (!ch.hamsters) continue;
      for (const hamster of ch.hamsters.values()) {
        const dx = mx - hamster.displayX;
        const dy = my - hamster.displayY;
        if (isInsideHamsterHitbox(dx, dy)) {
          return { type: 'hamster', hamster, character: ch };
        }
      }
    }

    for (const ch of this.characters.values()) {
      const dx = mx - ch.displayX;
      const dy = my - ch.displayY;
      if (isInsideHitbox(dx, dy)) {
        return { type: 'character', character: ch };
      }
    }
    return null;
  }

  async focusSession(sessionId) {
    try {
      const resp = await authFetch(`/api/sessions/${encodeURIComponent(sessionId)}/focus`, {
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

    this.characters.clear();
    this.particles.clear();

    this.glowCanvas.width = 0;
    this.glowCanvas.height = 0;
    this.glowCanvas = null;
    this.glowCtx = null;

    this.onRacerClick = null;
    this.onHamsterClick = null;
    this.onAfterDraw = null;
    this.engine = null;
  }
}
