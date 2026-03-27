import { authFetch } from '../auth.js';
import { getModelColor, hexToRgb } from '../session/colors.js';
import { DEFAULT_CONTEXT_WINDOW, TERMINAL_ACTIVITIES } from '../session/constants.js';
import { isParkingLotRacer, isPitRacer } from '../session/zones.js';
import { ParticleSystem } from './Particles.js';

const WHITE_RGB = { r: 255, g: 255, b: 255 };

/**
 * Bucket context window sizes into tiers for track grouping.
 * Sessions within the same tier share a track. Boundaries prevent splitting
 * sessions with similar context sizes (e.g. 128K vs 200K vs 258K) while
 * keeping vastly different scales (200K vs 1M) on separate tracks.
 */
export function getContextTier(maxTokens) {
  if (maxTokens <= 400000) return 400000;
  if (maxTokens <= 1500000) return 1500000;
  return Math.ceil(maxTokens / 1000000) * 1000000;
}

export class BaseCanvas {
  constructor(canvas, options) {
    this.canvas = canvas;
    this.ctx = canvas.getContext('2d');
    this.track = options.track;
    this.dashboard = options.dashboard;
    this.weather = options.weather;
    this.particles = new ParticleSystem();
    this.engine = options.engine || null;

    this._entityMapProperty = options.entityMapProperty;
    this[this._entityMapProperty] = options.entityMap || new Map();
    this._createEntity = options.createEntity;
    this._entityHitType = options.entityHitType;
    this._isInsideEntityHitbox = options.isInsideEntityHitbox;
    this._isInsideHamsterHitbox = options.isInsideHamsterHitbox;
    this._backgroundColor = options.backgroundColor;
    this._emptyStateColor = options.emptyStateColor;
    this._emptyStateText = options.emptyStateText || 'No active sessions detected';
    this._emptyStateSubtext = options.emptyStateSubtext || 'Start a session to see it race';

    this.connected = false;
    this.animFrameId = null;
    this.onRacerClick = null;
    this.onHamsterClick = null;
    this.onAfterDraw = null;

    this.lastFrameTime = 0;
    this.dt = 1 / 60;

    this.glowCanvas = document.createElement('canvas');
    this.glowCtx = this.glowCanvas.getContext('2d');

    this.shakeIntensity = 0;
    this.shakeDuration = 0;
    this.shakeTimer = 0;
    this.flashAlpha = 0;

    this._activeLaneCount = 1;
    this._pitLaneCount = 0;
    this._parkingLotLaneCount = 0;
    this._trackGroups = [{ maxTokens: DEFAULT_CONTEXT_WINDOW, laneCount: 1 }];
    this._trackGroupsKey = '';
    this._zoneCounts = { racing: 0, pit: 0, parked: 0 };
    this._needsResize = false;

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
    return this[this._entityMapProperty];
  }

  setEngine(engine) {
    this.engine = engine;
  }

  setConnected(connected) {
    this.connected = connected;
  }

  resize() {
    const dpr = window.devicePixelRatio || 1;
    const rect = this.canvas.parentElement.getBoundingClientRect();
    const viewportWidth = rect.width;
    const viewportHeight = rect.height;

    this.track.updateViewport(viewportHeight);

    const zonesHeight = this.track.getRequiredHeight(this._trackGroups, this._pitLaneCount, this._parkingLotLaneCount);
    const dashMinHeight = this.dashboard.getRequiredHeight(this.entities.size);
    const dashFromViewport = Math.max(0, viewportHeight - zonesHeight);
    const dashHeight = Math.max(dashMinHeight, dashFromViewport);
    const height = zonesHeight + dashHeight;

    if (this.width === viewportWidth && this.height === height && this._dpr === dpr) {
      return;
    }

    this.canvas.style.height = `${height}px`;
    this.canvas.width = viewportWidth * dpr;
    this.canvas.height = height * dpr;
    this.ctx.scale(dpr, dpr);
    this.width = viewportWidth;
    this.height = height;
    this._dpr = dpr;

    this.glowCanvas.width = Math.ceil(viewportWidth / 4);
    this.glowCanvas.height = Math.ceil(height / 4);
  }

  setAllRacers(sessions) {
    const newIds = new Set(sessions.map((session) => session.id));

    for (const id of this.entities.keys()) {
      if (!newIds.has(id)) {
        const entity = this.entities.get(id);
        this.entities.delete(id);
        this.onEntityRemoved(id, entity);
      }
    }

    for (const session of sessions) {
      if (this.entities.has(session.id)) {
        this.entities.get(session.id).update(session);
      } else {
        this.entities.set(session.id, this._createEntity(session));
      }
    }

    this.afterEntityCollectionChange();
  }

  updateRacer(state) {
    if (this.entities.has(state.id)) {
      this.entities.get(state.id).update(state);
    } else {
      this.entities.set(state.id, this._createEntity(state));
    }

    this.afterEntityCollectionChange();
  }

  removeRacer(id) {
    const entity = this.entities.get(id);
    if (!entity) {
      return;
    }

    this.entities.delete(id);
    this.onEntityRemoved(id, entity);
    this.afterEntityCollectionChange();
  }

  startLoop() {
    const loop = (timestamp) => {
      if (this.lastFrameTime === 0) {
        this.lastFrameTime = timestamp;
      }
      const rawDt = (timestamp - this.lastFrameTime) / 1000;
      this.dt = Math.min(rawDt, 0.05);
      this.lastFrameTime = timestamp;

      if (this._needsResize) {
        this._needsResize = false;
        this.resize();
      }

      this.update();
      this.draw();
      if (this.onAfterDraw) {
        this.onAfterDraw();
      }
      this.animFrameId = requestAnimationFrame(loop);
    };

    this.animFrameId = requestAnimationFrame(loop);
  }

  _buildZoneLayout() {
    const trackEntities = [];
    const pitEntities = [];
    const parkingLotEntities = [];

    for (const entity of this.entities.values()) {
      if (isParkingLotRacer(entity.state)) {
        parkingLotEntities.push(entity);
      } else if (isPitRacer(entity.state)) {
        pitEntities.push(entity);
      } else {
        trackEntities.push(entity);
      }
    }

    const pitLaneCount = pitEntities.length;
    const parkingLotLaneCount = parkingLotEntities.length;
    this._zoneCounts = {
      racing: trackEntities.length,
      pit: pitLaneCount,
      parked: parkingLotLaneCount,
    };

    const tierMap = new Map();
    for (const entity of trackEntities) {
      const maxTokens = entity.state.maxContextTokens || DEFAULT_CONTEXT_WINDOW;
      const tier = getContextTier(maxTokens);
      if (!tierMap.has(tier)) {
        tierMap.set(tier, { entities: [], maxTokens: 0 });
      }
      const group = tierMap.get(tier);
      group.entities.push(entity);
      group.maxTokens = Math.max(group.maxTokens, maxTokens);
    }

    const sortedGroups = [...tierMap.entries()]
      .sort(([a], [b]) => a - b)
      .map(([, group]) => group);
    const trackGroups = sortedGroups.length > 0
      ? sortedGroups.map(({ maxTokens, entities }) => ({ maxTokens, laneCount: entities.length }))
      : [{ maxTokens: DEFAULT_CONTEXT_WINDOW, laneCount: 1 }];

    this._syncTrackGroups(trackGroups, pitLaneCount, parkingLotLaneCount);

    let globalMaxTokens = DEFAULT_CONTEXT_WINDOW;
    for (const entity of this.entities.values()) {
      const maxTokens = entity.state.maxContextTokens || DEFAULT_CONTEXT_WINDOW;
      if (maxTokens > globalMaxTokens) {
        globalMaxTokens = maxTokens;
      }
    }

    const layouts = this.track.getMultiTrackLayout(this.width, trackGroups);
    const firstLayout = layouts[0];
    const lastLayout = layouts[layouts.length - 1];

    return {
      trackEntities,
      pitEntities,
      parkingLotEntities,
      pitLaneCount,
      parkingLotLaneCount,
      sortedGroups,
      trackGroups,
      layouts,
      trackZoneBottom: lastLayout.y + lastLayout.height,
      entryX: this.track.getPitEntryX(firstLayout),
      globalMaxTokens,
    };
  }

  _syncTrackGroups(trackGroups, pitLaneCount, parkingLotLaneCount) {
    const groupsKey = trackGroups.map((group) => `${group.maxTokens}:${group.laneCount}`).join(',');
    if (
      groupsKey !== this._trackGroupsKey ||
      pitLaneCount !== this._pitLaneCount ||
      parkingLotLaneCount !== this._parkingLotLaneCount
    ) {
      this._trackGroups = trackGroups;
      this._trackGroupsKey = groupsKey;
      this._activeLaneCount = trackGroups.reduce((sum, group) => sum + group.laneCount, 0) || 1;
      this._pitLaneCount = pitLaneCount;
      this._parkingLotLaneCount = parkingLotLaneCount;
      this._needsResize = true;
    }
  }

  _positionTrackEntities(zoneLayout, afterPosition) {
    const { sortedGroups, layouts, entryX, trackZoneBottom } = zoneLayout;

    for (let groupIndex = 0; groupIndex < sortedGroups.length; groupIndex++) {
      const { maxTokens: groupMaxTokens, entities: groupEntities } = sortedGroups[groupIndex];
      const layout = layouts[groupIndex];
      const sorted = groupEntities.sort((a, b) => a.state.lane - b.state.lane);

      for (let i = 0; i < sorted.length; i++) {
        const entity = sorted[i];
        const targetX = this.track.getTokenX(layout, entity.state.tokensUsed || 0, groupMaxTokens);
        const targetY = this.track.getLaneY(layout, i);

        if ((entity.inPit || entity.inParkingLot) && entity.initialized) {
          entity.startZoneTransition([
            { x: entryX, y: entity.displayY },
            { x: entryX, y: trackZoneBottom },
            { x: targetX, y: targetY },
          ]);
        }

        entity.setTarget(targetX, targetY);
        entity.inPit = false;
        entity.inParkingLot = false;
        entity.animate(this.particles, this.dt);

        if (afterPosition) {
          afterPosition(entity, layout, i);
        }
      }
    }
  }

  _positionPitEntities(zoneLayout, afterPosition) {
    const { pitEntities, trackGroups, pitLaneCount, entryX, trackZoneBottom, globalMaxTokens } = zoneLayout;
    if (pitLaneCount === 0) {
      return;
    }

    const pitBounds = this.track.getPitBounds(this.width, this.height, trackGroups, pitLaneCount);
    const sortedPit = pitEntities.sort((a, b) => a.state.lane - b.state.lane);

    for (let i = 0; i < sortedPit.length; i++) {
      const entity = sortedPit[i];
      const targetX = this.track.getTokenX(pitBounds, entity.state.tokensUsed || 0, globalMaxTokens);
      const targetY = this.track.getLaneY(pitBounds, i);

      if (!entity.inPit && entity.initialized) {
        if (entity.inParkingLot) {
          entity.startZoneTransition([
            { x: entryX, y: entity.displayY },
            { x: entryX, y: targetY },
            { x: targetX, y: targetY },
          ]);
        } else {
          entity.startZoneTransition([
            { x: entryX, y: trackZoneBottom },
            { x: entryX, y: pitBounds.y },
            { x: targetX, y: targetY },
          ]);
        }
      }

      entity.setTarget(targetX, targetY);
      entity.inPit = true;
      entity.inParkingLot = false;
      entity.animate(this.particles, this.dt);

      if (afterPosition) {
        afterPosition(entity, pitBounds, i);
      }
    }
  }

  _positionParkingLotEntities(zoneLayout, afterPosition) {
    const {
      parkingLotEntities,
      trackGroups,
      pitLaneCount,
      parkingLotLaneCount,
      entryX,
      globalMaxTokens,
    } = zoneLayout;
    if (parkingLotLaneCount === 0) {
      return;
    }

    const lotBounds = this.track.getParkingLotBounds(
      this.width,
      this.height,
      trackGroups,
      pitLaneCount,
      parkingLotLaneCount
    );
    const sortedLot = parkingLotEntities.sort((a, b) => a.state.lane - b.state.lane);

    for (let i = 0; i < sortedLot.length; i++) {
      const entity = sortedLot[i];
      const targetX = this.track.getTokenX(lotBounds, entity.state.tokensUsed || 0, globalMaxTokens);
      const targetY = this.track.getLaneY(lotBounds, i);

      if (!entity.inParkingLot && entity.initialized) {
        entity.startZoneTransition([
          { x: entryX, y: entity.displayY },
          { x: entryX, y: lotBounds.y },
          { x: targetX, y: targetY },
        ]);
      }

      entity.setTarget(targetX, targetY);
      entity.inPit = false;
      entity.inParkingLot = true;
      entity.animate(this.particles, this.dt);

      if (afterPosition) {
        afterPosition(entity, lotBounds, i);
      }
    }
  }

  _updateSharedFrameState(dt) {
    this.particles.update(dt);
    this.weather.updateMetrics([...this.entities.values()].map((entity) => entity.state));
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

    if (this.shakeTimer < this.shakeDuration && this.shakeIntensity > 0) {
      const progress = this.shakeTimer / this.shakeDuration;
      const currentIntensity = this.shakeIntensity * (1 - progress);
      const sx = (Math.random() * 2 - 1) * currentIntensity;
      const sy = (Math.random() * 2 - 1) * currentIntensity;
      ctx.translate(sx, sy);
    }

    ctx.fillStyle = this._backgroundColor;
    ctx.fillRect(-10, -10, this.width + 20, this.height + 20);
    this.weather.drawBehind(ctx, this.width, this.height);

    const pitLaneCount = this._pitLaneCount;
    const parkingLotLaneCount = this._parkingLotLaneCount;
    const groups = this._trackGroups;
    const excitement = this.engine ? this.engine.currentExcitement : 0;

    this.drawBeforeTrack(ctx, groups, excitement);
    this.track.drawMultiTrack(ctx, this.width, this.height, groups, excitement);
    this.track.drawPit(ctx, this.width, this.height, groups, pitLaneCount);

    if (parkingLotLaneCount > 0) {
      this.track.drawParkingLot(ctx, this.width, this.height, groups, pitLaneCount, parkingLotLaneCount);
    }

    this._drawDashboard(ctx, groups, pitLaneCount, parkingLotLaneCount);

    this.particles.drawBehind(ctx);
    this.drawBeforeEntities(ctx, groups, excitement);

    const sorted = [...this.entities.values()].sort((a, b) => a.displayY - b.displayY);
    for (const entity of sorted) {
      entity.draw(ctx);
    }

    this.drawAfterEntities(ctx, groups, excitement);
    this.particles.drawFront(ctx);
    this.weather.drawFront(ctx, this.width, this.height);
    this._drawBloom(ctx);

    if (this.flashAlpha > 0) {
      ctx.fillStyle = `rgba(255,255,255,${this.flashAlpha})`;
      ctx.fillRect(-10, -10, this.width + 20, this.height + 20);
    }

    ctx.restore();

    if (!this.connected) {
      ctx.fillStyle = 'rgba(0,0,0,0.6)';
      ctx.fillRect(0, 0, this.width, this.height);
      ctx.fillStyle = '#e94560';
      ctx.font = 'bold 20px Courier New';
      ctx.textAlign = 'center';
      ctx.fillText('Connecting...', this.width / 2, this.height / 2);
    }

    if (this.connected && this.entities.size === 0) {
      ctx.fillStyle = this._emptyStateColor;
      ctx.font = '16px Courier New';
      ctx.textAlign = 'center';
      ctx.fillText(this._emptyStateText, this.width / 2, this.height / 2 - 10);
      ctx.font = '12px Courier New';
      ctx.fillText(this._emptyStateSubtext, this.width / 2, this.height / 2 + 14);
    }
  }

  _drawDashboard(ctx, groups, pitLaneCount, parkingLotLaneCount) {
    const zonesHeight = this.track.getRequiredHeight(groups, pitLaneCount, parkingLotLaneCount);
    const dashAvailable = this.height - zonesHeight;
    if (dashAvailable <= 40) {
      return;
    }

    const dashBounds = this.dashboard.getBounds(this.width, zonesHeight, dashAvailable);
    const sessions = [...this.entities.values()].map((entity) => entity.state);
    this.drawDashboard(ctx, dashBounds, sessions);
  }

  drawDashboard(ctx, dashBounds, sessions) {
    this.dashboard.draw(ctx, dashBounds, sessions, this._zoneCounts);
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

    for (const entity of this.entities.values()) {
      const x = entity.displayX;
      const y = entity.displayY + (entity.springY || 0);

      this.drawEntityBloom(gc, entity, x, y);

      if (entity.hamsters) {
        for (const hamster of entity.hamsters.values()) {
          if (!hamster.glowIntensity || hamster.glowIntensity <= 0.02) {
            continue;
          }

          const rgb = hexToRgb(getModelColor(hamster.state.model, hamster.state.source).main);
          this._drawGlow(gc, hamster.displayX, hamster.displayY + (hamster.springY || 0), 15, rgb, hamster.glowIntensity * 2);
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

  _drawGlow(ctx, x, y, radius, rgb = WHITE_RGB, alpha = 1) {
    const grad = ctx.createRadialGradient(x, y, 0, x, y, radius);
    grad.addColorStop(0, `rgba(${rgb.r},${rgb.g},${rgb.b},${alpha})`);
    grad.addColorStop(1, `rgba(${rgb.r},${rgb.g},${rgb.b},0)`);
    ctx.fillStyle = grad;
    ctx.beginPath();
    ctx.arc(x, y, radius, 0, Math.PI * 2);
    ctx.fill();
  }

  _getMousePosition(e) {
    const rect = this.canvas.getBoundingClientRect();
    return {
      x: e.clientX - rect.left,
      y: e.clientY - rect.top,
    };
  }

  handleClick(e) {
    const { x, y } = this._getMousePosition(e);
    if (this.beforeCanvasClick(x, y)) {
      return;
    }

    const hit = this._hitTestAt(x, y);
    if (!hit) {
      return;
    }

    if (hit.type === 'hamster') {
      if (this.onHamsterClick) {
        this.onHamsterClick({
          hamsterState: hit.hamster.state,
          parentState: hit[this._entityHitType].state,
        });
      }
      return;
    }

    const entity = hit[this._entityHitType];
    if (this.onRacerClick) {
      this.onRacerClick(entity.state);
    }
    if (entity.hasTmux && !TERMINAL_ACTIVITIES.has(entity.state.activity)) {
      this.focusSession(entity.state.id);
    }
  }

  handleMouseMove(e) {
    const { x, y } = this._getMousePosition(e);
    let hoveredAny = false;

    for (const entity of this.entities.values()) {
      if (!entity.hamsters) {
        continue;
      }
      for (const hamster of entity.hamsters.values()) {
        const dx = x - hamster.displayX;
        const dy = y - hamster.displayY;
        if (this._isInsideHamsterHitbox(dx, dy)) {
          hoveredAny = true;
        }
      }
    }

    for (const entity of this.entities.values()) {
      const dx = x - entity.displayX;
      const dy = y - entity.displayY;
      entity.hovered = this._isInsideEntityHitbox(dx, dy);
      if (entity.hovered && entity.hasTmux) {
        hoveredAny = true;
      }
    }

    this.canvas.style.cursor = hoveredAny ? 'pointer' : 'default';
  }

  _hitTest(e) {
    const { x, y } = this._getMousePosition(e);
    return this._hitTestAt(x, y);
  }

  _hitTestAt(x, y) {
    for (const entity of this.entities.values()) {
      if (!entity.hamsters) {
        continue;
      }
      for (const hamster of entity.hamsters.values()) {
        const dx = x - hamster.displayX;
        const dy = y - hamster.displayY;
        if (this._isInsideHamsterHitbox(dx, dy)) {
          return { type: 'hamster', hamster, [this._entityHitType]: entity };
        }
      }
    }

    for (const entity of this.entities.values()) {
      const dx = x - entity.displayX;
      const dy = y - entity.displayY;
      if (this._isInsideEntityHitbox(dx, dy)) {
        return { type: this._entityHitType, [this._entityHitType]: entity };
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

    this.entities.clear();
    this.particles.clear();

    this.glowCanvas.width = 0;
    this.glowCanvas.height = 0;
    this.glowCanvas = null;
    this.glowCtx = null;

    this.afterDestroy();

    this.onRacerClick = null;
    this.onHamsterClick = null;
    this.onAfterDraw = null;
    this.engine = null;
  }

  afterEntityCollectionChange() {}

  onEntityRemoved() {}

  beforeCanvasClick() {
    return false;
  }

  drawBeforeTrack() {}

  drawBeforeEntities() {}

  drawAfterEntities() {}

  drawEntityBloom() {}

  afterDestroy() {}
}
