import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// Mocks -- keep FootraceCanvas from touching the real DOM / rAF

vi.mock('./Particles.js', () => ({
  ParticleSystem: vi.fn(function () {
    this.update = vi.fn();
    this.drawBehind = vi.fn();
    this.drawFront = vi.fn();
    this.clear = vi.fn();
    this.emit = vi.fn();
    this.emitWithColor = vi.fn();
  }),
}));

vi.mock('./FootraceTrack.js', () => ({
  FootraceTrack: vi.fn(function () {
    this.trackPadding = { left: 200, right: 60, top: 60, bottom: 40 };
    this.laneHeight = 80;
    this.updateViewport = vi.fn();
    this.getRequiredHeight = vi.fn((activeOrGroups, pit = 0, parking = 0) => {
      // Simplified formula matching real FootraceTrack layout constants
      let totalLanes;
      if (Array.isArray(activeOrGroups)) {
        totalLanes = activeOrGroups.reduce((sum, g) => sum + g.laneCount, 0) || 1;
      } else {
        totalLanes = activeOrGroups;
      }
      let h = 60 + totalLanes * 80 + 40;
      if (pit > 0) h += 30 + pit * 50 + 40;
      else h += 30 + 14 + 8; // collapsed pit
      if (parking > 0) h += 20 + parking * 45 + 40;
      return h;
    });
    this.getTrackBounds = vi.fn((w, h, lanes) => ({
      x: 200, y: 60, width: w - 260, height: lanes * 80, laneHeight: 80,
    }));
    this.getMultiTrackLayout = vi.fn((w, groups) => {
      let currentY = 60;
      return groups.map((g, i) => {
        if (i > 0) currentY += 36; // TRACK_GROUP_GAP + TRACK_GROUP_LABEL_HEIGHT
        const layout = {
          x: 200, y: currentY, width: w - 260,
          height: g.laneCount * 80, laneHeight: 80,
          maxTokens: g.maxTokens, laneCount: g.laneCount,
        };
        currentY += g.laneCount * 80;
        return layout;
      });
    });
    this.getPitBounds = vi.fn((w, h, activeOrGroups, pitCount) => {
      let trackBottom;
      if (Array.isArray(activeOrGroups)) {
        const totalLanes = activeOrGroups.reduce((sum, g) => sum + g.laneCount, 0) || 1;
        trackBottom = 60 + totalLanes * 80;
      } else {
        trackBottom = 60 + activeOrGroups * 80;
      }
      return {
        x: 200, y: trackBottom + 30,
        width: w - 260, height: pitCount * 50, laneHeight: 50,
      };
    });
    this.getParkingLotBounds = vi.fn((w, h, activeOrGroups, pit, lot) => {
      let trackBottom;
      if (Array.isArray(activeOrGroups)) {
        const totalLanes = activeOrGroups.reduce((sum, g) => sum + g.laneCount, 0) || 1;
        trackBottom = 60 + totalLanes * 80;
      } else {
        trackBottom = 60 + activeOrGroups * 80;
      }
      return {
        x: 200,
        y: trackBottom + (pit > 0 ? 30 + pit * 50 + 40 : 0) + 20,
        width: w - 260, height: lot * 45, laneHeight: 45,
      };
    });
    this.getLaneY = vi.fn((bounds, lane) => bounds.y + lane * bounds.laneHeight + bounds.laneHeight / 2);
    this.getPositionX = vi.fn((bounds, util) => bounds.x + util * bounds.width);
    this.getTokenX = vi.fn((bounds, tokens, max) => {
      if (max <= 0) return bounds.x;
      return bounds.x + (tokens / max) * bounds.width;
    });
    this.getPitEntryX = vi.fn((bounds) => bounds.x + 60);
    this.draw = vi.fn();
    this.drawMultiTrack = vi.fn();
    this.drawPit = vi.fn();
    this.drawParkingLot = vi.fn();
  }),
}));

vi.mock('./Dashboard.js', () => ({
  Dashboard: vi.fn(function () {
    this.getRequiredHeight = vi.fn(() => 160);
    this.getBounds = vi.fn();
    this.draw = vi.fn();
  }),
}));

vi.mock('./Weather.js', () => ({
  WeatherSystem: vi.fn(function () {
    this.enabled = true;
    this.toggle = vi.fn(function () {
      this.enabled = !this.enabled;
      return this.enabled;
    });
    this.getStateLabel = vi.fn(() => 'Clear');
    this.updateMetrics = vi.fn();
    this.update = vi.fn();
    this.drawBehind = vi.fn();
    this.drawFront = vi.fn();
  }),
}));

vi.mock('../entities/Character.js', () => ({
  Character: vi.fn(function (state) {
    this.id = state.id;
    this.state = state;
    this.displayX = 0;
    this.displayY = 0;
    this.targetX = 0;
    this.targetY = 0;
    this.springY = 0;
    this.initialized = false;
    this.inPit = false;
    this.inParkingLot = false;
    this.glowIntensity = 0;
    this.hovered = false;
    this.hasTmux = !!state.tmuxTarget;
    this.confettiEmitted = false;
    this.stumbleEmitted = false;
    this.starsEmitted = false;
    this.errorTimer = 0;
    this.errorStage = 0;
    this.hamsters = new Map();
    this.update = vi.fn(function (s) { this.state = s; this.hasTmux = !!s.tmuxTarget; });
    this.setTarget = vi.fn(function (x, y) {
      this.targetX = x;
      this.targetY = y;
      if (!this.initialized) { this.displayX = x; this.displayY = y; this.initialized = true; }
    });
    this.animate = vi.fn();
    this.draw = vi.fn();
    this.startZoneTransition = vi.fn();
  }),
}));

// Stub factories

function makeCtx() {
  return {
    scale: vi.fn(),
    clearRect: vi.fn(),
    save: vi.fn(),
    restore: vi.fn(),
    translate: vi.fn(),
    fillRect: vi.fn(),
    fillText: vi.fn(),
    drawImage: vi.fn(),
    beginPath: vi.fn(),
    arc: vi.fn(),
    fill: vi.fn(),
    createRadialGradient: vi.fn(() => ({ addColorStop: vi.fn() })),
    set fillStyle(_) {},
    set strokeStyle(_) {},
    set lineWidth(_) {},
    set font(_) {},
    set textAlign(_) {},
    set globalCompositeOperation(_) {},
    set globalAlpha(_) {},
  };
}

function makeCanvas() {
  const ctx = makeCtx();
  return {
    getContext: vi.fn(() => ctx),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    parentElement: { getBoundingClientRect: () => ({ width: 800, height: 600 }) },
    getBoundingClientRect: () => ({ left: 0, top: 0, width: 800, height: 600 }),
    style: {},
    width: 0,
    height: 0,
    _ctx: ctx,
  };
}

function setupGlobals() {
  globalThis.window = globalThis.window || {};
  window.devicePixelRatio = 1;
  window.addEventListener = vi.fn();
  window.removeEventListener = vi.fn();

  globalThis.document = globalThis.document || {};
  document.createElement = vi.fn(() => ({
    getContext: vi.fn(() => makeCtx()), width: 0, height: 0,
  }));

  globalThis.requestAnimationFrame = vi.fn(() => 42);
  globalThis.cancelAnimationFrame = vi.fn();
}

const { FootraceCanvas } = await import('./FootraceCanvas.js');

function agoISO(ms) {
  return new Date(Date.now() - ms).toISOString();
}

function makeState(overrides = {}) {
  return {
    id: 'sess-1',
    activity: 'thinking',
    lane: 0,
    contextUtilization: 0.5,
    lastDataReceivedAt: agoISO(0),
    model: 'claude-sonnet-4-5-20250929',
    source: 'claude',
    ...overrides,
  };
}

describe('FootraceCanvas', () => {
  let canvas;
  let fc;

  beforeEach(() => {
    vi.useFakeTimers();
    setupGlobals();
    canvas = makeCanvas();
    fc = new FootraceCanvas(canvas);
  });

  afterEach(() => {
    if (fc && fc.animFrameId !== null) fc.destroy();
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  describe('construction', () => {
    it('creates a FootraceTrack instance', () => {
      expect(fc.track).toBeDefined();
    });

    it('creates a WeatherSystem instance', () => {
      expect(fc.weather).toBeDefined();
    });

    it('starts the animation loop', () => {
      expect(requestAnimationFrame).toHaveBeenCalled();
      expect(fc.animFrameId).toBe(42);
    });

    it('sets initial dimensions', () => {
      expect(fc.width).toBe(800);
      expect(fc.height).toBeGreaterThan(0);
    });
  });

  describe('entities alias', () => {
    it('entities returns the characters Map', () => {
      expect(fc.entities).toBe(fc.characters);
    });

    it('entities reflects character additions', () => {
      fc.setAllRacers([makeState({ id: 'a' }), makeState({ id: 'b' })]);
      expect(fc.entities.size).toBe(2);
      expect(fc.entities.has('a')).toBe(true);
    });
  });

  describe('character management', () => {
    it('adds characters via setAllRacers', () => {
      fc.setAllRacers([makeState({ id: 'a' }), makeState({ id: 'b' })]);
      expect(fc.characters.size).toBe(2);
    });

    it('removes characters no longer in the list', () => {
      fc.setAllRacers([makeState({ id: 'a' }), makeState({ id: 'b' })]);
      fc.setAllRacers([makeState({ id: 'b' })]);
      expect(fc.characters.size).toBe(1);
      expect(fc.characters.has('a')).toBe(false);
    });

    it('updates existing characters via setAllRacers', () => {
      fc.setAllRacers([makeState({ id: 'a', activity: 'thinking' })]);
      fc.setAllRacers([makeState({ id: 'a', activity: 'tool_use' })]);
      const ch = fc.characters.get('a');
      expect(ch.update).toHaveBeenCalled();
    });

    it('adds a character via updateRacer', () => {
      fc.updateRacer(makeState({ id: 'new' }));
      expect(fc.characters.has('new')).toBe(true);
    });

    it('removes a character via removeRacer', () => {
      fc.setAllRacers([makeState({ id: 'a' })]);
      fc.removeRacer('a');
      expect(fc.characters.has('a')).toBe(false);
    });
  });

  describe('zone assignment', () => {
    it('assigns thinking characters to track', () => {
      fc.setAllRacers([makeState({ id: 'a', activity: 'thinking' })]);
      fc.update();
      const ch = fc.characters.get('a');
      expect(ch.inPit).toBe(false);
      expect(ch.inParkingLot).toBe(false);
    });

    it('assigns stale idle characters to pit', () => {
      fc.setAllRacers([makeState({ id: 'a', activity: 'idle', lastDataReceivedAt: agoISO(31_000) })]);
      fc.update();
      expect(fc.characters.get('a').inPit).toBe(true);
    });

    it('assigns completed characters to parking lot', () => {
      fc.setAllRacers([makeState({ id: 'a', activity: 'complete' })]);
      fc.update();
      expect(fc.characters.get('a').inParkingLot).toBe(true);
    });

    it('assigns errored characters to parking lot', () => {
      fc.setAllRacers([makeState({ id: 'a', activity: 'errored' })]);
      fc.update();
      expect(fc.characters.get('a').inParkingLot).toBe(true);
    });

    it('partitions mixed sessions correctly', () => {
      const stale = agoISO(31_000);
      const fresh = agoISO(0);
      fc.setAllRacers([
        makeState({ id: 'track1', activity: 'thinking', lastDataReceivedAt: fresh }),
        makeState({ id: 'pit1', activity: 'idle', lastDataReceivedAt: stale }),
        makeState({ id: 'lot1', activity: 'complete' }),
      ]);
      fc.update();

      expect(fc.characters.get('track1').inPit).toBe(false);
      expect(fc.characters.get('track1').inParkingLot).toBe(false);
      expect(fc.characters.get('pit1').inPit).toBe(true);
      expect(fc.characters.get('lot1').inParkingLot).toBe(true);
    });
  });

  describe('hit testing and interaction', () => {
    function clickEvent(clientX, clientY) {
      return { clientX, clientY };
    }

    function placeCharacter(overrides = {}) {
      fc.setAllRacers([makeState({ id: 'c1', ...overrides })]);
      fc.update();
      const character = fc.characters.get('c1');
      character.displayX = 400;
      character.displayY = 300;
      return character;
    }

    it('returns character hits with the subclass-specific result key', () => {
      const character = placeCharacter();
      const result = fc._hitTest(clickEvent(400, 300));
      expect(result).toEqual({ type: 'character', character });
    });

    it('handleClick invokes onRacerClick for character hits', () => {
      const cb = vi.fn();
      fc.onRacerClick = cb;
      const character = placeCharacter();

      fc.handleClick(clickEvent(400, 300));
      expect(cb).toHaveBeenCalledWith(character.state);
    });

    it('handleMouseMove updates hovered state and pointer cursor', () => {
      const character = placeCharacter({ tmuxTarget: 'pane-1' });

      fc.handleMouseMove({ clientX: 410, clientY: 300 });
      expect(character.hovered).toBe(true);
      expect(canvas.style.cursor).toBe('pointer');
    });
  });

  describe('event effects', () => {
    it('onComplete sets flashAlpha', () => {
      fc.setAllRacers([makeState({ id: 'a' })]);
      fc.onComplete('a');
      expect(fc.flashAlpha).toBe(0.3);
    });

    it('onError sets shake state', () => {
      fc.setAllRacers([makeState({ id: 'a' })]);
      fc.onError('a');
      expect(fc.shakeIntensity).toBe(6);
      expect(fc.shakeDuration).toBe(0.3);
    });

    it('onComplete resets confettiEmitted', () => {
      fc.setAllRacers([makeState({ id: 'a' })]);
      const ch = fc.characters.get('a');
      ch.confettiEmitted = true;
      fc.onComplete('a');
      expect(ch.confettiEmitted).toBe(false);
    });

    it('onError resets error animation state', () => {
      fc.setAllRacers([makeState({ id: 'a' })]);
      const ch = fc.characters.get('a');
      ch.stumbleEmitted = true;
      ch.starsEmitted = true;
      ch.errorTimer = 5;
      ch.errorStage = 3;
      fc.onError('a');
      expect(ch.stumbleEmitted).toBe(false);
      expect(ch.starsEmitted).toBe(false);
      expect(ch.errorTimer).toBe(0);
      expect(ch.errorStage).toBe(0);
    });
  });

  describe('setConnected', () => {
    it('sets connected state', () => {
      fc.setConnected(true);
      expect(fc.connected).toBe(true);
      fc.setConnected(false);
      expect(fc.connected).toBe(false);
    });
  });

  describe('setEngine', () => {
    it('sets engine reference', () => {
      const engine = { currentExcitement: 0.5 };
      fc.setEngine(engine);
      expect(fc.engine).toBe(engine);
    });
  });

  describe('canvas resize', () => {
    it('resizes when active lane count changes', () => {
      const resizeSpy = vi.spyOn(fc, 'resize');
      fc.setAllRacers([
        makeState({ id: 'a', activity: 'thinking', lane: 0 }),
        makeState({ id: 'b', activity: 'thinking', lane: 1 }),
      ]);
      fc.update();
      expect(fc._activeLaneCount).toBe(2);
      expect(resizeSpy).toHaveBeenCalled();
    });

    it('only resizes when lane counts actually change', () => {
      fc.setAllRacers([makeState({ id: 'a', activity: 'thinking' })]);
      fc.update();
      const resizeSpy = vi.spyOn(fc, 'resize');
      fc.update();
      expect(resizeSpy).not.toHaveBeenCalled();
    });
  });

  describe('track grouping', () => {
    it('groups characters with same maxContextTokens', () => {
      fc.setAllRacers([
        makeState({ id: 'a', activity: 'thinking', maxContextTokens: 200000 }),
        makeState({ id: 'b', activity: 'thinking', maxContextTokens: 200000 }),
      ]);
      fc.update();
      expect(fc._trackGroups).toEqual([{ maxTokens: 200000, laneCount: 2 }]);
    });

    it('creates separate groups for different maxContextTokens', () => {
      fc.setAllRacers([
        makeState({ id: 'a', activity: 'thinking', maxContextTokens: 200000 }),
        makeState({ id: 'b', activity: 'thinking', maxContextTokens: 1000000 }),
      ]);
      fc.update();
      expect(fc._trackGroups).toEqual([
        { maxTokens: 200000, laneCount: 1 },
        { maxTokens: 1000000, laneCount: 1 },
      ]);
    });
  });

  describe('zone transitions', () => {
    it('triggers zone transition when character moves from pit to track', () => {
      const stale = agoISO(31_000);
      fc.setAllRacers([makeState({ id: 'a', activity: 'idle', lastDataReceivedAt: stale })]);
      fc.update();
      const ch = fc.characters.get('a');
      expect(ch.inPit).toBe(true);

      fc.updateRacer(makeState({ id: 'a', activity: 'thinking', lastDataReceivedAt: agoISO(0) }));
      fc.update();
      expect(ch.inPit).toBe(false);
      expect(ch.startZoneTransition).toHaveBeenCalled();
    });

    it('triggers zone transition when character moves to parking lot', () => {
      fc.setAllRacers([makeState({ id: 'a', activity: 'thinking' })]);
      fc.update();
      const ch = fc.characters.get('a');

      fc.updateRacer(makeState({ id: 'a', activity: 'complete' }));
      fc.update();
      expect(ch.inParkingLot).toBe(true);
      expect(ch.startZoneTransition).toHaveBeenCalled();
    });
  });

  describe('animation loop', () => {
    it('cancels animation frame on destroy', () => {
      fc.destroy();
      expect(cancelAnimationFrame).toHaveBeenCalledWith(42);
      expect(fc.animFrameId).toBeNull();
    });

    it('cleans up event listeners on destroy', () => {
      fc.destroy();
      expect(window.removeEventListener).toHaveBeenCalledWith('resize', fc._resizeHandler);
      expect(canvas.removeEventListener).toHaveBeenCalledWith('click', expect.any(Function));
      expect(canvas.removeEventListener).toHaveBeenCalledWith('mousemove', expect.any(Function));
    });

    it('clears characters and particles on destroy', () => {
      fc.setAllRacers([makeState({ id: 'a' })]);
      expect(fc.characters.size).toBe(1);
      fc.destroy();
      expect(fc.characters.size).toBe(0);
    });
  });

  describe('destroy cleanup', () => {
    it('releases glow canvas references', () => {
      fc.destroy();
      expect(fc.glowCanvas).toBeNull();
      expect(fc.glowCtx).toBeNull();
    });

    it('clears callback references', () => {
      fc.onRacerClick = vi.fn();
      fc.onHamsterClick = vi.fn();
      fc.onAfterDraw = vi.fn();
      fc.engine = {};
      fc.destroy();
      expect(fc.onRacerClick).toBeNull();
      expect(fc.onHamsterClick).toBeNull();
      expect(fc.onAfterDraw).toBeNull();
      expect(fc.engine).toBeNull();
    });
  });

  describe('draw overlays', () => {
    it('draw runs without error when disconnected', () => {
      fc.setConnected(false);
      expect(() => fc.draw()).not.toThrow();
    });

    it('draw runs without error when connected and empty', () => {
      fc.setConnected(true);
      expect(() => fc.draw()).not.toThrow();
    });

    it('draw runs without error with flash active', () => {
      fc.flashAlpha = 0.3;
      expect(() => fc.draw()).not.toThrow();
    });

    it('renders empty state message when connected and no characters', () => {
      fc.setConnected(true);
      fc.draw();
      const calls = fc.ctx.fillText.mock.calls.map(c => c[0]);
      expect(calls).toContain('No active Claude sessions detected');
      expect(calls).toContain('Start a Claude Code session to see it race');
    });

    it('does not render empty state message when sessions exist', () => {
      fc.setConnected(true);
      fc.setAllRacers([makeState({ id: 'a' })]);
      fc.draw();
      const calls = fc.ctx.fillText.mock.calls.map(c => c[0]);
      expect(calls).not.toContain('No active Claude sessions detected');
    });

    it('does not render empty state message when disconnected', () => {
      fc.setConnected(false);
      fc.draw();
      const calls = fc.ctx.fillText.mock.calls.map(c => c[0]);
      expect(calls).not.toContain('No active Claude sessions detected');
    });

    it('renders dashboard when space is available below track', () => {
      fc.draw();
      expect(fc.dashboard.draw).toHaveBeenCalled();
    });

    it('passes session list to dashboard draw', () => {
      fc.setAllRacers([makeState({ id: 'a' }), makeState({ id: 'b' })]);
      fc.draw();
      const [, , sessions] = fc.dashboard.draw.mock.calls[0];
      expect(sessions).toHaveLength(2);
    });
  });

  describe('flash and shake decay', () => {
    it('flash decays over time', () => {
      fc.flashAlpha = 0.3;
      fc.dt = 1 / 60;
      fc.update();
      expect(fc.flashAlpha).toBeLessThan(0.3);
      expect(fc.flashAlpha).toBeGreaterThanOrEqual(0);
    });

    it('shake timer advances', () => {
      fc.shakeIntensity = 6;
      fc.shakeDuration = 0.3;
      fc.shakeTimer = 0;
      fc.dt = 0.1;
      fc.update();
      expect(fc.shakeTimer).toBeCloseTo(0.1, 5);
    });
  });

  describe('zone counts', () => {
    it('tracks zone counts after update', () => {
      const stale = agoISO(31_000);
      fc.setAllRacers([
        makeState({ id: 'a', activity: 'thinking' }),
        makeState({ id: 'b', activity: 'tool_use' }),
        makeState({ id: 'c', activity: 'idle', lastDataReceivedAt: stale }),
        makeState({ id: 'd', activity: 'complete' }),
        makeState({ id: 'e', activity: 'errored' }),
      ]);
      fc.update();
      expect(fc._zoneCounts).toEqual({ racing: 2, pit: 1, parked: 2 });
    });

    it('zone counts are zero with no characters', () => {
      fc.update();
      expect(fc._zoneCounts).toEqual({ racing: 0, pit: 0, parked: 0 });
    });
  });

  describe('weather integration', () => {
    it('updates weather metrics from character state each frame', () => {
      fc.setAllRacers([makeState({ id: 'a', activity: 'thinking' }), makeState({ id: 'b', activity: 'complete' })]);
      fc.update();
      expect(fc.weather.updateMetrics).toHaveBeenCalledWith([
        expect.objectContaining({ id: 'a', activity: 'thinking' }),
        expect.objectContaining({ id: 'b', activity: 'complete' }),
      ]);
      expect(fc.weather.update).toHaveBeenCalledWith(fc.dt, fc.width, fc.height);
    });

    it('draws weather behind and in front of the scene', () => {
      fc.draw();
      expect(fc.weather.drawBehind).toHaveBeenCalledWith(fc.ctx, fc.width, fc.height);
      expect(fc.weather.drawFront).toHaveBeenCalledWith(fc.ctx, fc.width, fc.height);
    });
  });

  describe('engine audio sync', () => {
    let engine;

    beforeEach(() => {
      engine = {
        startEngine: vi.fn(),
        stopEngine: vi.fn(),
        currentExcitement: 0,
      };
      fc.setEngine(engine);
    });

    it('starts engine for thinking characters', () => {
      fc.setAllRacers([makeState({ id: 'a', activity: 'thinking' })]);
      fc.update();
      expect(engine.startEngine).toHaveBeenCalledWith('a', 'thinking');
    });

    it('stops engine for pit characters', () => {
      const stale = agoISO(31_000);
      fc.setAllRacers([makeState({ id: 'a', activity: 'idle', lastDataReceivedAt: stale })]);
      fc.update();
      expect(engine.stopEngine).toHaveBeenCalledWith('a');
    });

    it('stops engine for parking lot characters', () => {
      fc.setAllRacers([makeState({ id: 'a', activity: 'complete' })]);
      fc.update();
      expect(engine.stopEngine).toHaveBeenCalledWith('a');
    });
  });
});
