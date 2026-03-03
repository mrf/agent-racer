import { ParticleSystem } from './Particles.js';
import { Track } from './Track.js';
import { Dashboard } from './Dashboard.js';
import { WeatherSystem } from './Weather.js';
import { Racer } from '../entities/Racer.js';
import { Grandstand } from '../entities/Grandstand.js';
import { PitCrew } from '../entities/PitCrew.js';
import { DraftDetector, DRAFT_GAP } from './DraftDetector.js';
import { getModelColor, hexToRgb } from '../session/colors.js';
import { authFetch } from '../auth.js';
import { DEFAULT_CONTEXT_WINDOW, TERMINAL_ACTIVITIES } from '../session/constants.js';
import { isParkingLotRacer, isPitRacer } from '../session/zones.js';
import { syncEngineForEntity } from '../audio/engineSync.js';

// Rectangular hit area matching the limo's elongated shape.
// Derived from car geometry: CAR_SCALE=2.3, LIMO_STRETCH=35.
const HIT_LEFT = 125;   // rear of limo: (17+35)*2.3 ≈ 120 + padding
const HIT_RIGHT = 60;   // nose of car: 23*2.3 ≈ 53 + padding
const HIT_TOP = 28;     // roof: 9*2.3 ≈ 21 + padding
const HIT_BOTTOM = 28;  // wheels: 10*2.3 ≈ 23 + padding

function isInsideHitbox(dx, dy) {
  return dx >= -HIT_LEFT && dx <= HIT_RIGHT && dy >= -HIT_TOP && dy <= HIT_BOTTOM;
}

// Hamster bounding box (20x15 hit area centered on displayX/displayY)
const HAMSTER_HIT_HX = 10;
const HAMSTER_HIT_HY = 7.5;

function isInsideHamsterHitbox(dx, dy) {
  return dx >= -HAMSTER_HIT_HX && dx <= HAMSTER_HIT_HX && dy >= -HAMSTER_HIT_HY && dy <= HAMSTER_HIT_HY;
}

export class RaceCanvas {
  constructor(canvas, engine = null) {
    this.canvas = canvas;
    this.ctx = canvas.getContext('2d');
    this.track = new Track();
    this.dashboard = new Dashboard();
    this.particles = new ParticleSystem();
    this.grandstand = new Grandstand();
    this.weather = new WeatherSystem();
    this.racers = new Map();

    // Pit crew: keyed by racer ID
    this.pitCrews = new Map();
    this.pitEntryTimers = new Map(); // seconds since entering pit
    this.prevPitIds = new Set();     // racer IDs that were in pit last frame

    this.connected = false;
    this.animFrameId = null;
    this.onRacerClick = null;
    this.onHamsterClick = null;
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

    // Draft/overtake mechanics
    this.draftDetector = new DraftDetector();
    this._draftPairs = [];    // [{drafter, leader, gap}]
    this._activeBattles = new Set(); // battle pair keys

    // Track lane counts for dynamic canvas resizing
    this._activeLaneCount = 1;
    this._pitLaneCount = 0;
    this._parkingLotLaneCount = 0;
    this._trackGroups = [{ maxTokens: DEFAULT_CONTEXT_WINDOW, laneCount: 1 }];
    this._trackGroupsKey = '';
    this._zoneCounts = { racing: 0, pit: 0, parked: 0 };

    // Grandstand event tracking
    this._racerMilestones = new Map();
    this._prevLeaderOrder = [];

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
    return this.racers;
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

    // Trigger crowd cheer when a racer crosses the 50% context mark
    if ((state.contextUtilization || 0) >= 0.5) {
      let milestones = this._racerMilestones.get(state.id);
      if (!milestones) {
        milestones = new Set();
        this._racerMilestones.set(state.id, milestones);
      }
      if (!milestones.has('50pct')) {
        milestones.add('50pct');
        this.grandstand.trigger('cheer', state.contextUtilization || 0.5);
      }
    }
  }

  removeRacer(id) {
    this.racers.delete(id);
    this._racerMilestones.delete(id);
    const crew = this.pitCrews.get(id);
    if (crew) crew.leave();
    this.pitEntryTimers.delete(id);
  }

  onComplete(sessionId) {
    const racer = this.racers.get(sessionId);
    if (racer) {
      racer.confettiEmitted = false;
    }
    // Flash effect on completion
    this.flashAlpha = 0.3;
    this.grandstand.trigger('ovation');
    if (this.engine) this.engine.playCrowdCheer();
  }

  onOvertake(payload) {
    const racer = this.racers.get(payload.overtakerId);
    if (racer) {
      racer.overtakeFlash = 1.0;
      // Swoosh particles burst forward from the nose of the overtaking car
      const S = 2.3; // CAR_SCALE
      const noseX = racer.displayX + 21 * S;
      this.particles.emit('overtakeSwoosh', noseX, racer.displayY, 10);
    }
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
    this.grandstand.trigger('gasp');
    if (this.engine) this.engine.playCrowdGasp();
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
    const currentPitIds = new Set(pitRacers.map(r => r.id));

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
        syncEngineForEntity(this.engine, racer.id, racer.state, 'track');
      }
    }

    // Draft/overtake detection on track racers
    {
      const nowS = performance.now() / 1000;
      const { draftPairs, overtakes, battles } = this.draftDetector.detect(trackRacers, nowS);
      this._draftPairs = draftPairs;
      this._activeBattles = battles;

      // Set draft intensity on racers
      for (const racer of trackRacers) {
        racer.draftIntensity = 0;
      }
      for (const { drafter, gap } of draftPairs) {
        drafter.draftIntensity = 1 - gap / 0.05; // 1.0 when exactly at leader, 0.0 at 5% gap
      }

      // Update position badges from server state
      for (const racer of trackRacers) {
        racer.position = racer.state.position || 0;
      }

      // Local overtake flash (frontend-detected, complements server events)
      for (const { overtaker } of overtakes) {
        if (overtaker.overtakeFlash <= 0) {
          overtaker.overtakeFlash = 0.8;
          const S = 2.3;
          const noseX = overtaker.displayX + 21 * S;
          this.particles.emit('overtakeSwoosh', noseX, overtaker.displayY, 8);
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

        syncEngineForEntity(this.engine, racer.id, racer.state, 'pit');
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

        syncEngineForEntity(this.engine, racer.id, racer.state, 'parkingLot');
      }
    }

    // --- Pit crew management ---
    // Detect racers that have left the pit → trigger crew departure
    for (const id of this.prevPitIds) {
      if (!currentPitIds.has(id)) {
        const crew = this.pitCrews.get(id);
        if (crew) crew.leave();
        this.pitEntryTimers.delete(id);
      }
    }

    // Spawn / update crew for current pit racers
    for (const racer of pitRacers) {
      const timer = (this.pitEntryTimers.get(racer.id) || 0) + dt;
      this.pitEntryTimers.set(racer.id, timer);

      if (timer >= 0.5 && !this.pitCrews.has(racer.id)) {
        const color = getModelColor(racer.state.model, racer.state.source);
        this.pitCrews.set(racer.id, new PitCrew(racer.displayX, racer.displayY + racer.springY, color));
      }

      const crew = this.pitCrews.get(racer.id);
      if (crew) {
        crew.updatePosition(racer.displayX, racer.displayY + racer.springY);
        // Trigger celebration when session completes while in pit
        if (racer.state.activity === 'complete' && !crew.celebrating) {
          crew.celebrate();
        }
        crew.update(dt);
      }
    }

    // Remove crews that have fully departed
    for (const [id, crew] of this.pitCrews) {
      if (crew.isDone()) this.pitCrews.delete(id);
    }

    this.prevPitIds = currentPitIds;

    this.particles.update(dt);

    // Detect overtakes (track racer position order change)
    const orderedRacers = trackRacers
      .filter(r => (r.state.tokensUsed || 0) > 0)
      .sort((a, b) => (b.state.tokensUsed || 0) - (a.state.tokensUsed || 0));
    const currentOrder = orderedRacers.map(r => r.id);
    if (this._prevLeaderOrder.length === currentOrder.length && currentOrder.length >= 2) {
      for (let i = 0; i < currentOrder.length; i++) {
        if (currentOrder[i] !== this._prevLeaderOrder[i]) {
          const leader = orderedRacers[0];
          const normX = leader ? Math.min(1, Math.max(0, leader.displayX / this.width)) : 0.5;
          this.grandstand.trigger('wave', normX);
          break;
        }
      }
    }
    this._prevLeaderOrder = currentOrder;

    // Mexican wave when 3+ sessions are actively thinking/running
    let activeCount = 0;
    for (const racer of this.racers.values()) {
      if (racer.state.activity === 'thinking' || racer.state.activity === 'tool_use') {
        activeCount++;
      }
    }
    if (activeCount >= 3) {
      this.grandstand.trigger('mexican');
    }

    this.grandstand.update(dt);

    // Update weather system
    this.weather.updateMetrics([...this.racers.values()].map(r => r.state));
    this.weather.update(dt, this.width, this.height);

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

    // Weather behind layer (sky tint, stars, wind)
    this.weather.drawBehind(ctx, this.width, this.height);

    const pitLaneCount = this._pitLaneCount;
    const parkingLotLaneCount = this._parkingLotLaneCount;
    const groups = this._trackGroups;

    const excitement = this.engine ? this.engine.currentExcitement : 0;

    // Draw grandstand before the track so pennants layer on top of spectators
    if (this.track._crowdMode !== 'hidden') {
      const firstLayout = this.track.getMultiTrackLayout(this.width, groups)[0];
      if (firstLayout) {
        this.grandstand.draw(ctx, firstLayout, this.track._crowdMode, excitement);
      }
    }

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

    // Draw draft wind lines between drafting pairs (behind cars)
    if (this._draftPairs.length > 0) {
      this._drawDraftLines(ctx);
    }

    // Draw all racers sorted by Y for correct layering
    const sorted = [...this.racers.values()].sort((a, b) => a.displayY - b.displayY);

    // Draw racers
    for (const racer of sorted) {
      racer.draw(ctx);
    }

    // Draw pit crew figures (in front of cars so they're visible)
    for (const crew of this.pitCrews.values()) {
      crew.draw(ctx);
    }

    // Draw particles in front of racers
    this.particles.drawFront(ctx);

    // Weather front layer (rain, lightning, fog, haze)
    this.weather.drawFront(ctx, this.width, this.height);

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

  _drawDraftLines(ctx) {
    const S = 2.3; // CAR_SCALE
    const LIMO = 35; // LIMO_STRETCH

    for (const { drafter, leader, gap } of this._draftPairs) {
      // Intensity: stronger when gap is smaller (0 gap = full intensity)
      const intensity = Math.max(0, 1 - gap / DRAFT_GAP);

      // Connect leader's rear (left side) to drafter's nose (right side)
      const leaderRearX = leader.displayX - (17 + LIMO) * S;
      const leaderY = leader.displayY;
      const drafterNoseX = drafter.displayX + 21 * S;
      const drafterY = drafter.displayY;

      const lineAlpha = intensity * 0.35;
      const numLines = 4;
      const nowS = performance.now() / 1000;

      ctx.save();
      for (let i = 0; i < numLines; i++) {
        // Animated offset: lines scroll from leader rear toward drafter
        const t = ((nowS * 2 + i / numLines) % 1);
        const lx = leaderRearX + (drafterNoseX - leaderRearX) * t;
        const ly = leaderY + (drafterY - leaderY) * t;
        const yOff = (i - (numLines - 1) / 2) * 4;

        ctx.strokeStyle = `rgba(180,210,255,${lineAlpha * (1 - Math.abs(t - 0.5) * 1.5)})`;
        ctx.lineWidth = 1 + intensity;
        ctx.beginPath();
        ctx.moveTo(lx - 8, ly + yOff);
        ctx.lineTo(lx + 8, ly + yOff);
        ctx.stroke();
      }

      // Turbulence cone behind leader
      const coneAlpha = intensity * 0.12;
      const grad = ctx.createLinearGradient(leaderRearX, leaderY, drafterNoseX, drafterY);
      grad.addColorStop(0, `rgba(160,190,255,${coneAlpha})`);
      grad.addColorStop(1, 'rgba(160,190,255,0)');
      ctx.fillStyle = grad;
      ctx.beginPath();
      ctx.moveTo(leaderRearX, leaderY - 8);
      ctx.lineTo(leaderRearX, leaderY + 8);
      ctx.lineTo(drafterNoseX, drafterY + 16);
      ctx.lineTo(drafterNoseX, drafterY - 16);
      ctx.closePath();
      ctx.fill();

      // Speed delta badge between the two cars
      const leaderBurn = leader.state.burnRatePerMinute || 0;
      const drafterBurn = drafter.state.burnRatePerMinute || 0;
      const deltaBurn = leaderBurn - drafterBurn;
      if (Math.abs(deltaBurn) > 100) {
        const midX = (leaderRearX + drafterNoseX) / 2;
        const midY = (leaderY + drafterY) / 2 - 14;
        const sign = deltaBurn > 0 ? '+' : '';
        const label = `${sign}${Math.round(deltaBurn / 100) / 10}K tok/min`;
        ctx.font = '8px Courier New';
        const tw = ctx.measureText(label).width;
        ctx.fillStyle = `rgba(20,20,40,${intensity * 0.7})`;
        ctx.fillRect(midX - tw / 2 - 3, midY - 7, tw + 6, 11);
        ctx.fillStyle = `rgba(180,210,255,${intensity * 0.9})`;
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillText(label, midX, midY);
        ctx.textBaseline = 'alphabetic';
      }

      // Battle indicator: show "BATTLE" badge above the midpoint
      const battleKey = [leader.id, drafter.id].sort().join(':');
      if (this._activeBattles.has(battleKey)) {
        const midX = (leaderRearX + drafterNoseX) / 2;
        const midY = Math.min(leaderY, drafterY) - 28;
        const label = 'BATTLE';
        ctx.font = 'bold 8px Courier New';
        const tw = ctx.measureText(label).width;
        const pulse = 0.6 + 0.4 * Math.sin(performance.now() / 200);
        ctx.fillStyle = `rgba(255,60,60,${0.8 * pulse})`;
        ctx.fillRect(midX - tw / 2 - 4, midY - 7, tw + 8, 12);
        ctx.fillStyle = '#fff';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillText(label, midX, midY);
        ctx.textBaseline = 'alphabetic';
      }

      ctx.restore();
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

      // Hamster glow contribution (colored by model)
      if (racer.hamsters) {
        for (const hamster of racer.hamsters.values()) {
          if (hamster.glowIntensity <= 0.02) continue;
          const hx = hamster.displayX;
          const hy = hamster.displayY + hamster.springY;
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

    // Composite the glow canvas onto main canvas with additive blending
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
          parentState: hit.racer.state,
        });
      }
      return;
    }

    const racer = hit.racer;
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

    // Check hamster hover
    for (const racer of this.racers.values()) {
      if (!racer.hamsters) continue;
      for (const hamster of racer.hamsters.values()) {
        const dx = mx - hamster.displayX;
        const dy = my - hamster.displayY;
        if (isInsideHamsterHitbox(dx, dy)) {
          hoveredAny = true;
        }
      }
    }

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

    // Check hamster hitboxes first (smaller targets get priority)
    for (const racer of this.racers.values()) {
      if (!racer.hamsters) continue;
      for (const hamster of racer.hamsters.values()) {
        const dx = mx - hamster.displayX;
        const dy = my - hamster.displayY;
        if (isInsideHamsterHitbox(dx, dy)) {
          return { type: 'hamster', hamster, racer };
        }
      }
    }

    // Then check racer hitboxes
    for (const racer of this.racers.values()) {
      const dx = mx - racer.displayX;
      const dy = my - racer.displayY;
      if (isInsideHitbox(dx, dy)) {
        return { type: 'racer', racer };
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

    // Clear racer references
    this.racers.clear();
    this._racerMilestones.clear();

    // Clear pit crew state
    this.pitCrews.clear();
    this.pitEntryTimers.clear();
    this.prevPitIds.clear();

    // Clear particle system
    this.particles.clear();

    // Release offscreen canvas
    this.glowCanvas.width = 0;
    this.glowCanvas.height = 0;
    this.glowCanvas = null;
    this.glowCtx = null;

    // Clear callback references
    this.onRacerClick = null;
    this.onHamsterClick = null;
    this.onAfterDraw = null;
    this.engine = null;
  }
}
