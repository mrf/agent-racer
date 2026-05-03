import { BaseCanvas } from './BaseCanvas.js';
import { Track } from './Track.js';
import { Dashboard } from './Dashboard.js';
import { WeatherSystem } from './Weather.js';
import { Racer } from '../entities/Racer.js';
import { Grandstand } from '../entities/Grandstand.js';
import { PitCrew } from '../entities/PitCrew.js';
import { DraftDetector, DRAFT_GAP } from './DraftDetector.js';
import { getModelColor } from '../session/colors.js';
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

export class RaceCanvas extends BaseCanvas {
  constructor(canvas, engine = null) {
    super(canvas, {
      engine,
      track: new Track(),
      dashboard: new Dashboard(),
      weather: new WeatherSystem(),
      entityMapProperty: 'racers',
      createEntity: (state) => new Racer(state),
      entityHitType: 'racer',
      backgroundColor: '#1a1a2e',
      emptyStateColor: '#666',
      emptyStateText: 'No active sessions detected',
      emptyStateSubtext: 'Start a session to see it race',
      isInsideEntityHitbox: isInsideHitbox,
      isInsideHamsterHitbox,
    });

    this.teams = [];
    this.grandstand = new Grandstand();
    this.pitCrews = new Map();
    this.pitEntryTimers = new Map();
    this.prevPitIds = new Set();
    this.draftDetector = new DraftDetector();
    this._draftPairs = [];
    this._activeBattles = new Set();
    this._racerMilestones = new Map();
    this._prevLeaderOrder = [];
    this.ambientDustCooldown = 0;
    // Per-racer state for curve-dust and chicane-wobble effects.
    this._prevRacerAngles = new Map();
    this._curveDustCooldowns = new Map();
    this._chicaneWobbleCooldowns = new Map();
  }

  setTeams(teams) {
    this.teams = teams || [];
    this._applyTeamColors();
  }

  _applyTeamColors() {
    const colorMap = new Map();
    for (const team of this.teams) {
      if (!team.memberIds) continue;
      for (const id of team.memberIds) {
        colorMap.set(id, team.color);
      }
    }
    for (const [id, racer] of this.racers) {
      racer.teamColor = colorMap.get(id) || null;
    }
  }

  updateRacer(state) {
    super.updateRacer(state);

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

  update() {
    const dt = this.dt;
    const zoneLayout = this._buildZoneLayout();
    const { trackEntities: trackRacers, pitEntities: pitRacers, layouts, trackZoneBottom } = zoneLayout;
    const currentPitIds = new Set(pitRacers.map(r => r.id));
    const busyRacerCount = this._countBusyRacers();

    this._positionTrackEntities(zoneLayout, (racer) => {
      syncEngineForEntity(this.engine, racer.id, racer.state, 'track');
    });

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

    this._positionPitEntities(zoneLayout, (racer) => {
      syncEngineForEntity(this.engine, racer.id, racer.state, 'pit');
    });
    this._positionParkingLotEntities(zoneLayout, (racer) => {
      syncEngineForEntity(this.engine, racer.id, racer.state, 'parkingLot');
    });

    this._updateAmbientDust(dt, layouts[0], trackZoneBottom, busyRacerCount);
    if (this._customSampler) {
      this._updateCurveDust(dt, trackRacers);
      this._updateChicaneWobble(trackRacers);
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

    for (const [id, crew] of this.pitCrews) {
      if (!currentPitIds.has(id)) {
        crew.update(dt);
      }
    }

    // Remove crews that have fully departed
    for (const [id, crew] of this.pitCrews) {
      if (crew.isDone()) this.pitCrews.delete(id);
    }

    this.prevPitIds = currentPitIds;

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

    this.grandstand.update(dt);
    this._updateSharedFrameState(dt);
  }

  _countBusyRacers() {
    let busyCount = 0;
    for (const racer of this.racers.values()) {
      if (racer.state.activity === 'thinking' || racer.state.activity === 'tool_use' || racer.state.isChurning) {
        busyCount++;
      }
    }
    return busyCount;
  }

  _updateAmbientDust(dt, trackBounds, trackZoneBottom, busyRacerCount) {
    this.ambientDustCooldown = Math.max(0, this.ambientDustCooldown - dt);
    if (!trackBounds || busyRacerCount > 0 || this.ambientDustCooldown > 0) {
      return;
    }

    const litBandHeight = Math.max((trackZoneBottom - trackBounds.y) * 0.55, 1);
    const x = trackBounds.x + Math.random() * trackBounds.width;
    const y = trackBounds.y + Math.random() * litBandHeight;

    this.particles.emit('ambientDust', x, y, 1);
    this.ambientDustCooldown = 0.18 + Math.random() * 0.24;
  }

  /**
   * Emit brownish dust particles behind racers that are actively cornering on
   * a tile-based track.  Angular velocity above a threshold indicates a curve.
   */
  _updateCurveDust(dt, trackEntities) {
    for (const racer of trackEntities) {
      const curAngle = racer.pathAngle || 0;
      const prevAngle = this._prevRacerAngles.get(racer.id) || curAngle;

      // Normalise angle delta to [-π, π].
      let delta = curAngle - prevAngle;
      while (delta >  Math.PI) { delta -= 2 * Math.PI; }
      while (delta < -Math.PI) { delta += 2 * Math.PI; }

      const angularSpeed = Math.abs(delta) / Math.max(dt, 0.001);

      const cooldown = (this._curveDustCooldowns.get(racer.id) || 0) - dt;
      if (angularSpeed > 0.8 && cooldown <= 0) {
        // Emit a few dust particles near the rear tyres.
        const S = 2.3;
        const dustX = racer.displayX - 10 * S * 0.3 + (Math.random() - 0.5) * 12;
        const dustY = racer.displayY + racer.springY + (Math.random() - 0.5) * 6;
        this.particles.emit('curveDust', dustX, dustY, 2);
        this._curveDustCooldowns.set(racer.id, 0.08 + Math.random() * 0.06);
      } else {
        this._curveDustCooldowns.set(racer.id, Math.max(0, cooldown));
      }

      this._prevRacerAngles.set(racer.id, curAngle);
    }
  }

  /**
   * Apply a vertical spring impulse to racers traversing chicane tile sections,
   * simulating the car being jostled through the tight S-curve.
   */
  _updateChicaneWobble(trackEntities) {
    if (!this._chicaneRanges || this._chicaneRanges.length === 0) return;
    const totalLen = this._customTotalLength;
    if (totalLen <= 0) return;

    for (const racer of trackEntities) {
      const t = racer.trackT;
      if (t === undefined) continue;
      const arcLen = t * totalLen;

      let inChicane = false;
      for (let i = 0; i < this._chicaneRanges.length; i++) {
        const range = this._chicaneRanges[i];
        if (arcLen >= range.arcStart && arcLen <= range.arcEnd) {
          inChicane = true;
          break;
        }
      }

      const cooldown = this._chicaneWobbleCooldowns.get(racer.id) || 0;
      if (inChicane && cooldown <= 0) {
        // Kick the spring so the car bounces through the chicane.
        racer.springVel -= 2.2 + Math.random() * 1.2;
        this._chicaneWobbleCooldowns.set(racer.id, 0.22);
      } else {
        this._chicaneWobbleCooldowns.set(racer.id, Math.max(0, cooldown - this.dt));
      }
    }
  }

  drawBeforeTrack(ctx, groups, excitement) {
    if (this.track._crowdMode === 'hidden') return;

    const layouts = this.track.getMultiTrackLayout(this.width, groups);
    const firstLayout = layouts[0];
    if (!firstLayout) return;

    if (this._customSampler && this._grandstandRects && this._grandstandRects.length > 0) {
      // Tile-based track: draw animated spectators at each grandstand tile position.
      const scale = this._customTrackPixelW > 0
        ? firstLayout.width / this._customTrackPixelW
        : 1;
      for (let i = 0; i < this._grandstandRects.length; i++) {
        const r = this._grandstandRects[i];
        const canvasRect = {
          x: firstLayout.x + r.x * scale,
          y: firstLayout.y + r.y * scale,
          w: r.w * scale,
          h: r.h * scale,
        };
        this.grandstand.drawAtTile(ctx, canvasRect, excitement);
      }
    } else {
      // Linear track: draw the crowd strip above the track as usual.
      this.grandstand.draw(ctx, firstLayout, this.track._crowdMode, excitement);
    }
  }

  drawDashboard(ctx, dashBounds, sessions) {
    this.dashboard.draw(ctx, dashBounds, sessions, this._zoneCounts, this.teams);
  }

  drawBeforeEntities(ctx) {
    if (this._draftPairs.length > 0) {
      this._drawDraftLines(ctx);
    }
  }

  drawAfterEntities(ctx) {
    for (const crew of this.pitCrews.values()) {
      crew.draw(ctx);
    }
  }

  drawEntityBloom(gc, racer, x, y) {
    if (racer.glowIntensity > 0.02) {
      this._drawGlow(gc, x, y, 35, undefined, racer.glowIntensity * 2);
    }

    if (racer.state.activity === 'tool_use') {
      this._drawGlow(gc, x + 22, y, 20, { r: 255, g: 255, b: 200 }, 0.4);
    }

    if (racer.state.activity === 'waiting' && Math.sin(racer.hazardPhase) > 0) {
      this._drawGlow(gc, x, y, 25, { r: 255, g: 170, b: 0 }, 0.3);
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

  beforeCanvasClick(mx, my) {
    return this.dashboard.handleClick(mx, my);
  }

  afterEntityCollectionChange() {
    this._applyTeamColors();
  }

  onEntityRemoved(id) {
    this._racerMilestones.delete(id);
    const crew = this.pitCrews.get(id);
    if (crew) {
      crew.leave();
    }
    this.pitEntryTimers.delete(id);
    this._prevRacerAngles.delete(id);
    this._curveDustCooldowns.delete(id);
    this._chicaneWobbleCooldowns.delete(id);
  }

  afterDestroy() {
    this._racerMilestones.clear();
    this.pitCrews.clear();
    this.pitEntryTimers.clear();
    this.prevPitIds.clear();
    this._prevRacerAngles.clear();
    this._curveDustCooldowns.clear();
    this._chicaneWobbleCooldowns.clear();
  }
}
