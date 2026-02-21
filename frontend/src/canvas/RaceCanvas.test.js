import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// Mocks -- keep RaceCanvas from touching the real DOM / rAF

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

vi.mock('./Track.js', () => ({
  Track: vi.fn(function () {
    this.trackPadding = { left: 200, right: 60, top: 60, bottom: 40 };
    this.laneHeight = 80;
    this.updateViewport = vi.fn();
    this.getRequiredHeight = vi.fn((activeOrGroups, pit = 0, parking = 0) => {
      // Simplified formula matching real Track layout constants
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

vi.mock('../entities/Racer.js', () => ({
  Racer: vi.fn(function (state) {
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
    this.pitDimTarget = 0;
    this.parkingLotDimTarget = 0;
    this.glowIntensity = 0;
    this.hovered = false;
    this.hasTmux = !!state.tmuxTarget;
    this.confettiEmitted = false;
    this.smokeEmitted = false;
    this.skidEmitted = false;
    this.errorTimer = 0;
    this.errorStage = 0;
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

  // rAF / cAF stubs -- do NOT run the loop automatically
  globalThis.requestAnimationFrame = vi.fn(() => 42);
  globalThis.cancelAnimationFrame = vi.fn();
}

// Import under test (after mocks are wired)
const { RaceCanvas } = await import('./RaceCanvas.js');

// Helpers

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

describe('RaceCanvas', () => {
  let canvas;
  let rc;

  beforeEach(() => {
    vi.useFakeTimers();
    setupGlobals();
    canvas = makeCanvas();
    rc = new RaceCanvas(canvas);
  });

  afterEach(() => {
    if (rc && rc.animFrameId !== null) rc.destroy();
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  describe('zone assignment', () => {
    it('assigns active racers (thinking) to the track', () => {
      rc.setAllRacers([makeState({ id: 'a', activity: 'thinking' })]);
      rc.update();
      const racer = rc.racers.get('a');
      expect(racer.inPit).toBe(false);
      expect(racer.inParkingLot).toBe(false);
    });

    it('assigns active racers (tool_use) to the track', () => {
      rc.setAllRacers([makeState({ id: 'a', activity: 'tool_use' })]);
      rc.update();
      const racer = rc.racers.get('a');
      expect(racer.inPit).toBe(false);
      expect(racer.inParkingLot).toBe(false);
    });

    it('assigns idle racers with stale data to the pit', () => {
      rc.setAllRacers([makeState({ id: 'a', activity: 'idle', lastDataReceivedAt: agoISO(31_000) })]);
      rc.update();
      const racer = rc.racers.get('a');
      expect(racer.inPit).toBe(true);
      expect(racer.inParkingLot).toBe(false);
    });

    it('keeps idle racers with fresh data on the track', () => {
      rc.setAllRacers([makeState({ id: 'a', activity: 'idle', lastDataReceivedAt: agoISO(0) })]);
      rc.update();
      const racer = rc.racers.get('a');
      expect(racer.inPit).toBe(false);
      expect(racer.inParkingLot).toBe(false);
    });

    it('assigns waiting racers with stale data to the pit', () => {
      rc.setAllRacers([makeState({ id: 'a', activity: 'waiting', lastDataReceivedAt: agoISO(31_000) })]);
      rc.update();
      const racer = rc.racers.get('a');
      expect(racer.inPit).toBe(true);
    });

    it('assigns starting racers with stale data to the pit', () => {
      rc.setAllRacers([makeState({ id: 'a', activity: 'starting', lastDataReceivedAt: agoISO(31_000) })]);
      rc.update();
      const racer = rc.racers.get('a');
      expect(racer.inPit).toBe(true);
    });

    it('assigns idle racers with no lastDataReceivedAt to the pit', () => {
      rc.setAllRacers([makeState({ id: 'a', activity: 'idle', lastDataReceivedAt: null })]);
      rc.update();
      const racer = rc.racers.get('a');
      expect(racer.inPit).toBe(true);
    });

    it('assigns completed racers to the parking lot', () => {
      rc.setAllRacers([makeState({ id: 'a', activity: 'complete' })]);
      rc.update();
      const racer = rc.racers.get('a');
      expect(racer.inParkingLot).toBe(true);
      expect(racer.inPit).toBe(false);
    });

    it('assigns errored racers to the parking lot', () => {
      rc.setAllRacers([makeState({ id: 'a', activity: 'errored' })]);
      rc.update();
      const racer = rc.racers.get('a');
      expect(racer.inParkingLot).toBe(true);
    });

    it('assigns lost racers to the parking lot', () => {
      rc.setAllRacers([makeState({ id: 'a', activity: 'lost' })]);
      rc.update();
      const racer = rc.racers.get('a');
      expect(racer.inParkingLot).toBe(true);
    });

    it('terminal activities always go to parking lot regardless of freshness', () => {
      rc.setAllRacers([makeState({ id: 'a', activity: 'complete', lastDataReceivedAt: agoISO(0) })]);
      rc.update();
      expect(rc.racers.get('a').inParkingLot).toBe(true);
    });

    it('partitions mixed sessions correctly across all three zones', () => {
      const stale = agoISO(31_000);
      const fresh = agoISO(0);
      rc.setAllRacers([
        makeState({ id: 'track1', activity: 'thinking', lastDataReceivedAt: fresh }),
        makeState({ id: 'track2', activity: 'tool_use', lastDataReceivedAt: fresh }),
        makeState({ id: 'pit1', activity: 'idle', lastDataReceivedAt: stale }),
        makeState({ id: 'pit2', activity: 'waiting', lastDataReceivedAt: stale }),
        makeState({ id: 'lot1', activity: 'complete' }),
        makeState({ id: 'lot2', activity: 'errored' }),
      ]);
      rc.update();

      expect(rc.racers.get('track1').inPit).toBe(false);
      expect(rc.racers.get('track1').inParkingLot).toBe(false);
      expect(rc.racers.get('track2').inPit).toBe(false);
      expect(rc.racers.get('track2').inParkingLot).toBe(false);

      expect(rc.racers.get('pit1').inPit).toBe(true);
      expect(rc.racers.get('pit2').inPit).toBe(true);

      expect(rc.racers.get('lot1').inParkingLot).toBe(true);
      expect(rc.racers.get('lot2').inParkingLot).toBe(true);
    });

    it('DATA_FRESHNESS_MS boundary: exactly at threshold goes to pit', () => {
      rc.setAllRacers([makeState({ id: 'a', activity: 'idle', lastDataReceivedAt: agoISO(30_000) })]);
      rc.update();
      expect(rc.racers.get('a').inPit).toBe(true);
    });

    it('DATA_FRESHNESS_MS boundary: 1ms under threshold stays on track', () => {
      rc.setAllRacers([makeState({ id: 'a', activity: 'idle', lastDataReceivedAt: agoISO(29_999) })]);
      rc.update();
      expect(rc.racers.get('a').inPit).toBe(false);
    });
  });

  describe('hit testing (rectangular hitbox)', () => {
    function clickEvent(clientX, clientY) {
      return { clientX, clientY };
    }

    // Place a single racer at (400, 300) and return it.
    function placeRacer() {
      rc.setAllRacers([makeState({ id: 'r1' })]);
      rc.update();
      const racer = rc.racers.get('r1');
      racer.displayX = 400;
      racer.displayY = 300;
      return racer;
    }

    it('detects a hit near the center of the car', () => {
      const racer = placeRacer();
      const result = rc._hitTest(clickEvent(430, 300)); // 30px right
      expect(result).toEqual({ type: 'racer', racer });
    });

    it('detects a hit at the rear of the limo', () => {
      const racer = placeRacer();
      // 120px left of center — within HIT_LEFT=125
      const result = rc._hitTest(clickEvent(280, 300));
      expect(result).toEqual({ type: 'racer', racer });
    });

    it('misses beyond the rear of the limo', () => {
      placeRacer();
      // 130px left of center — outside HIT_LEFT=125
      const result = rc._hitTest(clickEvent(270, 300));
      expect(result).toBeNull();
    });

    it('misses beyond the front of the car', () => {
      placeRacer();
      // 65px right of center — outside HIT_RIGHT=60
      const result = rc._hitTest(clickEvent(465, 300));
      expect(result).toBeNull();
    });

    it('detects a hit at the top edge of the car', () => {
      const racer = placeRacer();
      // 25px above — within HIT_TOP=28
      const result = rc._hitTest(clickEvent(400, 275));
      expect(result).toEqual({ type: 'racer', racer });
    });

    it('misses above the car', () => {
      placeRacer();
      // 30px above — outside HIT_TOP=28
      const result = rc._hitTest(clickEvent(400, 270));
      expect(result).toBeNull();
    });

    it('returns null when there are no racers', () => {
      const result = rc._hitTest(clickEvent(400, 300));
      expect(result).toBeNull();
    });

    it('hits the first racer when multiple overlap', () => {
      rc.setAllRacers([
        makeState({ id: 'r1' }),
        makeState({ id: 'r2' }),
      ]);
      rc.update();
      const r1 = rc.racers.get('r1');
      const r2 = rc.racers.get('r2');
      r1.displayX = 400;
      r1.displayY = 300;
      r2.displayX = 410;
      r2.displayY = 300;

      const result = rc._hitTest(clickEvent(405, 300));
      expect(result).toEqual({ type: 'racer', racer: r1 });
    });

    it('catches rear corner clicks the old circular hitbox missed', () => {
      const racer = placeRacer();
      // 120px left, 15px up — circular distance ~121px (miss with old radius)
      // but within rectangular bounds (HIT_LEFT=125, HIT_TOP=28)
      const result = rc._hitTest(clickEvent(280, 285));
      expect(result).toEqual({ type: 'racer', racer });
    });

    it('handleClick invokes onRacerClick callback on hit', () => {
      const cb = vi.fn();
      rc.onRacerClick = cb;
      const racer = placeRacer();

      rc.handleClick(clickEvent(400, 300));
      expect(cb).toHaveBeenCalledWith(racer.state);
    });

    it('handleClick does not invoke callback on miss', () => {
      const cb = vi.fn();
      rc.onRacerClick = cb;
      rc.handleClick(clickEvent(9999, 9999));
      expect(cb).not.toHaveBeenCalled();
    });
  });

  describe('canvas resize', () => {
    it('sets initial canvas dimensions on construction', () => {
      expect(rc.width).toBe(800);
      expect(rc.height).toBeGreaterThan(0);
    });

    it('resizes when active lane count changes', () => {
      const resizeSpy = vi.spyOn(rc, 'resize');

      rc.setAllRacers([
        makeState({ id: 'a', activity: 'thinking', lane: 0 }),
        makeState({ id: 'b', activity: 'thinking', lane: 1 }),
        makeState({ id: 'c', activity: 'thinking', lane: 2 }),
      ]);
      rc.update();

      expect(rc._activeLaneCount).toBe(3);
      expect(resizeSpy).toHaveBeenCalled();
    });

    it('resizes when pit lane count changes', () => {
      const stale = agoISO(31_000);
      const resizeSpy = vi.spyOn(rc, 'resize');

      rc.setAllRacers([
        makeState({ id: 'a', activity: 'thinking' }),
        makeState({ id: 'pit1', activity: 'idle', lastDataReceivedAt: stale }),
        makeState({ id: 'pit2', activity: 'idle', lastDataReceivedAt: stale }),
      ]);
      rc.update();

      expect(rc._pitLaneCount).toBe(2);
      expect(resizeSpy).toHaveBeenCalled();
    });

    it('resizes when parking lot lane count changes', () => {
      const resizeSpy = vi.spyOn(rc, 'resize');

      rc.setAllRacers([
        makeState({ id: 'a', activity: 'thinking' }),
        makeState({ id: 'lot1', activity: 'complete' }),
        makeState({ id: 'lot2', activity: 'errored' }),
      ]);
      rc.update();

      expect(rc._parkingLotLaneCount).toBe(2);
      expect(resizeSpy).toHaveBeenCalled();
    });

    it('only resizes when lane counts actually change', () => {
      rc.setAllRacers([makeState({ id: 'a', activity: 'thinking' })]);
      rc.update();

      const resizeSpy = vi.spyOn(rc, 'resize');
      rc.update();
      expect(resizeSpy).not.toHaveBeenCalled();
    });

    it('defaults _activeLaneCount to 1 when all racers are off-track', () => {
      rc.setAllRacers([makeState({ id: 'a', activity: 'complete' })]);
      rc.update();
      expect(rc._activeLaneCount).toBe(1);
    });

    it('height grows with all three zones populated', () => {
      rc.setAllRacers([
        makeState({ id: 'a1', activity: 'thinking', lane: 0 }),
        makeState({ id: 'a2', activity: 'thinking', lane: 1 }),
        makeState({ id: 'p1', activity: 'idle', lastDataReceivedAt: agoISO(31_000) }),
        makeState({ id: 'l1', activity: 'complete' }),
      ]);
      rc.update();

      expect(rc._activeLaneCount).toBe(2);
      expect(rc._pitLaneCount).toBe(1);
      expect(rc._parkingLotLaneCount).toBe(1);

      const zonesHeight = rc.track.getRequiredHeight(2, 1, 1);
      expect(rc.height).toBeGreaterThanOrEqual(zonesHeight);
    });
  });

  describe('animation loop', () => {
    it('requests an animation frame on construction', () => {
      expect(requestAnimationFrame).toHaveBeenCalled();
      expect(rc.animFrameId).toBe(42);
    });

    it('cancels animation frame on destroy', () => {
      rc.destroy();
      expect(cancelAnimationFrame).toHaveBeenCalledWith(42);
      expect(rc.animFrameId).toBeNull();
    });

    it('cleans up event listeners on destroy', () => {
      rc.destroy();
      expect(window.removeEventListener).toHaveBeenCalledWith('resize', rc._resizeHandler);
      expect(canvas.removeEventListener).toHaveBeenCalledWith('click', expect.any(Function));
      expect(canvas.removeEventListener).toHaveBeenCalledWith('mousemove', expect.any(Function));
    });

    it('clears racers and particles on destroy', () => {
      rc.setAllRacers([makeState({ id: 'a' })]);
      expect(rc.racers.size).toBe(1);
      rc.destroy();
      expect(rc.racers.size).toBe(0);
    });
  });

  describe('racer management', () => {
    it('adds new racers via setAllRacers', () => {
      rc.setAllRacers([makeState({ id: 'a' }), makeState({ id: 'b' })]);
      expect(rc.racers.size).toBe(2);
      expect(rc.racers.has('a')).toBe(true);
      expect(rc.racers.has('b')).toBe(true);
    });

    it('removes racers no longer in the list', () => {
      rc.setAllRacers([makeState({ id: 'a' }), makeState({ id: 'b' })]);
      rc.setAllRacers([makeState({ id: 'b' })]);
      expect(rc.racers.size).toBe(1);
      expect(rc.racers.has('a')).toBe(false);
    });

    it('updates existing racers via setAllRacers', () => {
      rc.setAllRacers([makeState({ id: 'a', activity: 'thinking' })]);
      rc.setAllRacers([makeState({ id: 'a', activity: 'tool_use' })]);
      const racer = rc.racers.get('a');
      expect(racer.update).toHaveBeenCalled();
    });

    it('adds a racer via updateRacer', () => {
      rc.updateRacer(makeState({ id: 'new' }));
      expect(rc.racers.has('new')).toBe(true);
    });

    it('updates existing racer via updateRacer', () => {
      rc.setAllRacers([makeState({ id: 'a' })]);
      rc.updateRacer(makeState({ id: 'a', activity: 'complete' }));
      expect(rc.racers.get('a').update).toHaveBeenCalled();
    });

    it('removes a racer via removeRacer', () => {
      rc.setAllRacers([makeState({ id: 'a' })]);
      rc.removeRacer('a');
      expect(rc.racers.has('a')).toBe(false);
    });
  });

  describe('event effects', () => {
    it('onComplete sets flashAlpha', () => {
      rc.setAllRacers([makeState({ id: 'a' })]);
      rc.onComplete('a');
      expect(rc.flashAlpha).toBe(0.3);
    });

    it('onError sets shake state', () => {
      rc.setAllRacers([makeState({ id: 'a' })]);
      rc.onError('a');
      expect(rc.shakeIntensity).toBe(6);
      expect(rc.shakeDuration).toBe(0.3);
    });

    it('onComplete resets confettiEmitted on the racer', () => {
      rc.setAllRacers([makeState({ id: 'a' })]);
      const racer = rc.racers.get('a');
      racer.confettiEmitted = true;
      rc.onComplete('a');
      expect(racer.confettiEmitted).toBe(false);
    });

    it('onError resets error animation state on the racer', () => {
      rc.setAllRacers([makeState({ id: 'a' })]);
      const racer = rc.racers.get('a');
      racer.smokeEmitted = true;
      racer.skidEmitted = true;
      racer.errorTimer = 5;
      racer.errorStage = 3;
      rc.onError('a');
      expect(racer.smokeEmitted).toBe(false);
      expect(racer.skidEmitted).toBe(false);
      expect(racer.errorTimer).toBe(0);
      expect(racer.errorStage).toBe(0);
    });
  });

  describe('track grouping by context window', () => {
    it('groups racers with same maxContextTokens into one track group', () => {
      rc.setAllRacers([
        makeState({ id: 'a', activity: 'thinking', maxContextTokens: 200000 }),
        makeState({ id: 'b', activity: 'thinking', maxContextTokens: 200000 }),
      ]);
      rc.update();

      expect(rc._trackGroups).toEqual([{ maxTokens: 200000, laneCount: 2 }]);
    });

    it('creates separate groups for different maxContextTokens', () => {
      rc.setAllRacers([
        makeState({ id: 'a', activity: 'thinking', maxContextTokens: 200000 }),
        makeState({ id: 'b', activity: 'thinking', maxContextTokens: 1000000 }),
      ]);
      rc.update();

      expect(rc._trackGroups).toEqual([
        { maxTokens: 200000, laneCount: 1 },
        { maxTokens: 1000000, laneCount: 1 },
      ]);
    });

    it('sorts groups by maxTokens ascending', () => {
      rc.setAllRacers([
        makeState({ id: 'a', activity: 'thinking', maxContextTokens: 1000000 }),
        makeState({ id: 'b', activity: 'thinking', maxContextTokens: 200000 }),
        makeState({ id: 'c', activity: 'thinking', maxContextTokens: 500000 }),
      ]);
      rc.update();

      expect(rc._trackGroups.map(g => g.maxTokens)).toEqual([200000, 500000, 1000000]);
    });

    it('defaults to DEFAULT_CONTEXT_WINDOW when maxContextTokens is missing', () => {
      rc.setAllRacers([
        makeState({ id: 'a', activity: 'thinking' }),
      ]);
      rc.update();

      expect(rc._trackGroups).toEqual([{ maxTokens: 200000, laneCount: 1 }]);
    });

    it('uses single default group when no track racers exist', () => {
      rc.setAllRacers([
        makeState({ id: 'a', activity: 'complete' }),
      ]);
      rc.update();

      expect(rc._trackGroups).toEqual([{ maxTokens: 200000, laneCount: 1 }]);
    });

    it('calls getMultiTrackLayout with groups', () => {
      rc.setAllRacers([
        makeState({ id: 'a', activity: 'thinking', maxContextTokens: 200000 }),
        makeState({ id: 'b', activity: 'thinking', maxContextTokens: 1000000 }),
      ]);
      rc.update();

      expect(rc.track.getMultiTrackLayout).toHaveBeenCalledWith(
        rc.width,
        [
          { maxTokens: 200000, laneCount: 1 },
          { maxTokens: 1000000, laneCount: 1 },
        ]
      );
    });

    it('passes groups to drawMultiTrack in draw()', () => {
      rc.setAllRacers([
        makeState({ id: 'a', activity: 'thinking', maxContextTokens: 200000 }),
        makeState({ id: 'b', activity: 'thinking', maxContextTokens: 1000000 }),
      ]);
      rc.update();
      rc.draw();

      expect(rc.track.drawMultiTrack).toHaveBeenCalled();
      const callArgs = rc.track.drawMultiTrack.mock.calls[0];
      expect(callArgs[3]).toEqual([
        { maxTokens: 200000, laneCount: 1 },
        { maxTokens: 1000000, laneCount: 1 },
      ]);
    });

    it('resizes when group composition changes', () => {
      rc.setAllRacers([
        makeState({ id: 'a', activity: 'thinking', maxContextTokens: 200000 }),
      ]);
      rc.update();

      const resizeSpy = vi.spyOn(rc, 'resize');
      rc.setAllRacers([
        makeState({ id: 'a', activity: 'thinking', maxContextTokens: 200000 }),
        makeState({ id: 'b', activity: 'thinking', maxContextTokens: 1000000 }),
      ]);
      rc.update();

      expect(resizeSpy).toHaveBeenCalled();
    });

    it('does not resize when groups are unchanged', () => {
      rc.setAllRacers([
        makeState({ id: 'a', activity: 'thinking', maxContextTokens: 200000 }),
      ]);
      rc.update();

      const resizeSpy = vi.spyOn(rc, 'resize');
      rc.update();
      expect(resizeSpy).not.toHaveBeenCalled();
    });
  });

  describe('handleMouseMove', () => {
    it('sets hovered on racer within hit radius', () => {
      rc.setAllRacers([makeState({ id: 'r1', tmuxTarget: 'my-pane' })]);
      rc.update();
      const racer = rc.racers.get('r1');
      racer.displayX = 200;
      racer.displayY = 200;

      rc.handleMouseMove({ clientX: 210, clientY: 200 });
      expect(racer.hovered).toBe(true);
      expect(canvas.style.cursor).toBe('pointer');
    });

    it('clears hovered on racer outside hit radius', () => {
      rc.setAllRacers([makeState({ id: 'r1', tmuxTarget: 'my-pane' })]);
      rc.update();
      const racer = rc.racers.get('r1');
      racer.displayX = 200;
      racer.displayY = 200;

      rc.handleMouseMove({ clientX: 300, clientY: 300 });
      expect(racer.hovered).toBe(false);
      expect(canvas.style.cursor).toBe('default');
    });
  });
});
