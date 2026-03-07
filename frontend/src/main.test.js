// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

const mocks = vi.hoisted(() => ({
  engine: null,
  activeView: null,
  conn: {},
}));

vi.mock('./websocket.js', () => ({
  RaceConnection: vi.fn((opts) => {
    Object.assign(mocks.conn, opts);
    return { connect: vi.fn() };
  }),
}));

function createMockView() {
  const entities = new Map();
  const view = {
    setAllRacers: vi.fn(),
    updateRacer: vi.fn(),
    removeRacer: vi.fn(),
    onComplete: vi.fn(),
    onError: vi.fn(),
    setConnected: vi.fn(),
    destroy: vi.fn(),
    entities,
    racers: entities,
    dt: 16,
    ctx: {},
    width: 800,
    onRacerClick: null,
    onHamsterClick: null,
    onAfterDraw: null,
  };
  mocks.activeView = view;
  return view;
}

vi.mock('./ViewRenderer.js', () => ({
  createView: vi.fn(() => createMockView()),
  getViewTypes: vi.fn(() => ['race']),
}));

vi.mock('./audio/SoundEngine.js', () => ({
  SoundEngine: vi.fn(() => {
    mocks.engine = {
      playAppear: vi.fn(),
      playDisappear: vi.fn(),
      playToolClick: vi.fn(),
      playGearShift: vi.fn(),
      playVictory: vi.fn(),
      playCrash: vi.fn(),
      startAmbient: vi.fn(),
      reconcileViewSwitch: vi.fn(),
      setMuted: vi.fn(),
      updateExcitement: vi.fn(),
      applyConfig: vi.fn(),
      stopEngine: vi.fn(),
      recordCompletion: vi.fn(),
      recordCrash: vi.fn(),
    };
    return mocks.engine;
  }),
}));

vi.mock('./notifications.js', () => ({
  requestPermission: vi.fn(),
  notifyCompletion: vi.fn(),
}));

function setupDOM() {
  document.body.innerHTML = `
    <div id="debug-panel" class="hidden">
      <div id="debug-log"></div>
      <button id="debug-close"></button>
    </div>
    <div id="detail-flyout" class="hidden">
      <div id="flyout-content"></div>
      <button id="flyout-close"></button>
    </div>
    <div id="connection-status"></div>
    <div id="session-count"></div>
    <canvas id="race-canvas"></canvas>
    <div id="battlepass-bar"></div>
    <div id="shortcut-bar"></div>
  `;
  document.getElementById('race-canvas').getBoundingClientRect = () => ({
    top: 0, left: 0, width: 800, height: 600, right: 800, bottom: 600, x: 0, y: 0,
  });
}

function makeSession(overrides = {}) {
  return {
    id: 'sess-1',
    activity: 'thinking',
    contextUtilization: 0.5,
    tokensUsed: 5000,
    maxContextTokens: 100000,
    burnRatePerMinute: 500,
    model: 'claude-sonnet-4-5-20250929',
    source: 'cli',
    workingDir: '/home/user/project',
    branch: 'main',
    tmuxTarget: 'dev:0',
    pid: 1234,
    messageCount: 10,
    toolCallCount: 5,
    currentTool: 'Read',
    startedAt: new Date().toISOString(),
    lastActivityAt: new Date().toISOString(),
    completedAt: null,
    isChurning: false,
    ...overrides,
  };
}

beforeEach(async () => {
  vi.resetModules();
  localStorage.clear();
  setupDOM();
  globalThis.fetch = vi.fn(() => Promise.resolve({ ok: false }));
  Object.defineProperty(window, 'innerWidth', { value: 1024, configurable: true, writable: true });
  Object.defineProperty(window, 'innerHeight', { value: 768, configurable: true, writable: true });
  await import('./main.js');
});

// ── Session count calculation ─────────────────────────────────────────

describe('session count calculation', () => {
  it('counts all sessions as active when none are terminal', () => {
    const el = document.getElementById('session-count');
    mocks.conn.onSnapshot({
      sessions: [
        makeSession({ id: 's1', activity: 'thinking' }),
        makeSession({ id: 's2', activity: 'tool_use' }),
      ],
    });
    expect(el.textContent).toBe('2 active / 2 total');
  });

  it('excludes complete, errored, and lost from active count', () => {
    const el = document.getElementById('session-count');
    mocks.conn.onSnapshot({
      sessions: [
        makeSession({ id: 's1', activity: 'thinking' }),
        makeSession({ id: 's2', activity: 'complete' }),
        makeSession({ id: 's3', activity: 'errored' }),
        makeSession({ id: 's4', activity: 'lost' }),
        makeSession({ id: 's5', activity: 'tool_use' }),
      ],
    });
    expect(el.textContent).toBe('2 active / 5 total');
  });

  it('shows 0 active / 0 total for empty snapshot', () => {
    const el = document.getElementById('session-count');
    mocks.conn.onSnapshot({ sessions: [] });
    expect(el.textContent).toBe('0 active / 0 total');
  });

  it('updates count after delta adds and removes sessions', () => {
    const el = document.getElementById('session-count');
    mocks.conn.onSnapshot({
      sessions: [
        makeSession({ id: 's1', activity: 'thinking' }),
        makeSession({ id: 's2', activity: 'thinking' }),
      ],
    });
    expect(el.textContent).toBe('2 active / 2 total');

    mocks.conn.onDelta({
      updates: [makeSession({ id: 's3', activity: 'complete' })],
      removed: ['s1'],
    });
    expect(el.textContent).toBe('1 active / 2 total');
  });
});

// ── Session appear/disappear detection ────────────────────────────────

describe('session appear/disappear detection', () => {
  it('plays appear sound for new sessions in snapshot', () => {
    mocks.conn.onSnapshot({
      sessions: [makeSession({ id: 's1' }), makeSession({ id: 's2' })],
    });
    expect(mocks.engine.playAppear).toHaveBeenCalledTimes(2);
  });

  it('plays disappear sound for removed sessions between snapshots', () => {
    mocks.conn.onSnapshot({
      sessions: [makeSession({ id: 's1' }), makeSession({ id: 's2' })],
    });
    mocks.engine.playAppear.mockClear();
    mocks.engine.playDisappear.mockClear();

    mocks.conn.onSnapshot({
      sessions: [makeSession({ id: 's2' })],
    });
    expect(mocks.engine.playDisappear).toHaveBeenCalledTimes(1);
    expect(mocks.engine.playAppear).not.toHaveBeenCalled();
  });

  it('plays appear for new sessions in delta updates', () => {
    mocks.conn.onSnapshot({ sessions: [makeSession({ id: 's1' })] });
    mocks.engine.playAppear.mockClear();

    mocks.conn.onDelta({
      updates: [makeSession({ id: 's2' })],
    });
    expect(mocks.engine.playAppear).toHaveBeenCalledTimes(1);
  });

  it('does not play appear for existing sessions in delta', () => {
    mocks.conn.onSnapshot({ sessions: [makeSession({ id: 's1' })] });
    mocks.engine.playAppear.mockClear();

    mocks.conn.onDelta({
      updates: [makeSession({ id: 's1', activity: 'tool_use' })],
    });
    expect(mocks.engine.playAppear).not.toHaveBeenCalled();
  });

  it('plays disappear and stops engine for removed sessions in delta', () => {
    mocks.conn.onSnapshot({ sessions: [makeSession({ id: 's1' })] });
    mocks.engine.playDisappear.mockClear();

    mocks.conn.onDelta({ removed: ['s1'] });
    expect(mocks.engine.playDisappear).toHaveBeenCalledTimes(1);
    expect(mocks.engine.stopEngine).toHaveBeenCalledWith('s1');
  });

  it('cleans up session tracking on removal so re-add triggers appear', () => {
    mocks.conn.onSnapshot({ sessions: [makeSession({ id: 's1' })] });
    mocks.engine.playAppear.mockClear();

    mocks.conn.onDelta({ removed: ['s1'] });

    mocks.conn.onDelta({
      updates: [makeSession({ id: 's1' })],
    });
    expect(mocks.engine.playAppear).toHaveBeenCalledTimes(1);
  });
});

// ── SFX debounce (3s cooldown) ────────────────────────────────────────

describe('SFX debounce (3s cooldown)', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('plays tool click on first transition to tool_use', () => {
    mocks.conn.onSnapshot({
      sessions: [makeSession({ id: 's1', activity: 'thinking' })],
    });

    mocks.conn.onDelta({
      updates: [makeSession({ id: 's1', activity: 'tool_use' })],
    });

    expect(mocks.engine.playToolClick).toHaveBeenCalledTimes(1);
  });

  it('plays gear shift on thinking <-> tool_use transition', () => {
    mocks.conn.onSnapshot({
      sessions: [makeSession({ id: 's1', activity: 'thinking' })],
    });

    mocks.conn.onDelta({
      updates: [makeSession({ id: 's1', activity: 'tool_use' })],
    });

    expect(mocks.engine.playGearShift).toHaveBeenCalledTimes(1);
  });

  it('suppresses SFX within 3s cooldown', () => {
    mocks.conn.onSnapshot({
      sessions: [makeSession({ id: 's1', activity: 'thinking' })],
    });

    mocks.conn.onDelta({
      updates: [makeSession({ id: 's1', activity: 'tool_use' })],
    });
    expect(mocks.engine.playGearShift).toHaveBeenCalledTimes(1);

    vi.advanceTimersByTime(2000);

    mocks.conn.onDelta({
      updates: [makeSession({ id: 's1', activity: 'thinking' })],
    });
    // Gear shift suppressed — still only the initial one
    expect(mocks.engine.playGearShift).toHaveBeenCalledTimes(1);
  });

  it('allows SFX after 3s cooldown expires', () => {
    mocks.conn.onSnapshot({
      sessions: [makeSession({ id: 's1', activity: 'thinking' })],
    });

    mocks.conn.onDelta({
      updates: [makeSession({ id: 's1', activity: 'tool_use' })],
    });
    expect(mocks.engine.playGearShift).toHaveBeenCalledTimes(1);

    vi.advanceTimersByTime(3000);

    mocks.conn.onDelta({
      updates: [makeSession({ id: 's1', activity: 'thinking' })],
    });
    expect(mocks.engine.playGearShift).toHaveBeenCalledTimes(2);
  });

  it('tracks cooldown independently per session', () => {
    mocks.conn.onSnapshot({
      sessions: [
        makeSession({ id: 's1', activity: 'thinking' }),
        makeSession({ id: 's2', activity: 'thinking' }),
      ],
    });

    mocks.conn.onDelta({
      updates: [
        makeSession({ id: 's1', activity: 'tool_use' }),
        makeSession({ id: 's2', activity: 'tool_use' }),
      ],
    });
    expect(mocks.engine.playToolClick).toHaveBeenCalledTimes(2);

    vi.advanceTimersByTime(2000);

    // s1 within cooldown — suppressed
    mocks.conn.onDelta({
      updates: [makeSession({ id: 's1', activity: 'thinking' })],
    });
    expect(mocks.engine.playGearShift).toHaveBeenCalledTimes(2);

    vi.advanceTimersByTime(1500); // now 3.5s since both started

    // s2 past cooldown — should play
    mocks.conn.onDelta({
      updates: [makeSession({ id: 's2', activity: 'thinking' })],
    });
    expect(mocks.engine.playGearShift).toHaveBeenCalledTimes(3);
  });

  it('does not play gear shift for non-active transitions', () => {
    mocks.conn.onSnapshot({
      sessions: [makeSession({ id: 's1', activity: 'thinking' })],
    });

    mocks.conn.onDelta({
      updates: [makeSession({ id: 's1', activity: 'complete' })],
    });

    expect(mocks.engine.playGearShift).not.toHaveBeenCalled();
  });

  it('resets cooldown after session removal and re-addition', () => {
    mocks.conn.onSnapshot({
      sessions: [makeSession({ id: 's1', activity: 'thinking' })],
    });

    mocks.conn.onDelta({
      updates: [makeSession({ id: 's1', activity: 'tool_use' })],
    });
    expect(mocks.engine.playToolClick).toHaveBeenCalledTimes(1);

    // Remove then re-add
    mocks.conn.onDelta({ removed: ['s1'] });
    mocks.conn.onDelta({
      updates: [makeSession({ id: 's1', activity: 'thinking' })],
    });
    mocks.conn.onDelta({
      updates: [makeSession({ id: 's1', activity: 'tool_use' })],
    });

    // Cooldown was cleared by removal, so SFX plays again
    expect(mocks.engine.playToolClick).toHaveBeenCalledTimes(2);
  });
});

// ── Flyout helpers ────────────────────────────────────────────────────

function showFlyout(carX, carY, id = 'flyout-test') {
  const state = makeSession({ id });
  mocks.activeView.entities.set(id, { displayX: carX, displayY: carY });
  mocks.activeView.onRacerClick(state);
  return document.getElementById('detail-flyout');
}

function setViewport(width, height) {
  Object.defineProperty(window, 'innerWidth', { value: width, configurable: true, writable: true });
  Object.defineProperty(window, 'innerHeight', { value: height, configurable: true, writable: true });
}

// ── Flyout anchor selection ───────────────────────────────────────────

describe('flyout anchor selection', () => {
  it('anchors to the right by default when space is available', () => {
    const flyout = showFlyout(200, 300);
    // right anchor: targetX = carVX + margin(50) = 250
    expect(flyout.style.left).toBe('250px');
    expect(flyout.className).toContain('arrow-left');
  });

  it('falls back to left when right side overflows', () => {
    // right: 900 + 50 + 380 + 10 = 1340 > 1024 — no
    // left:  900 - 50 - 380 = 470 > 10 — yes
    const flyout = showFlyout(900, 300);
    expect(flyout.style.left).toBe('470px');
    expect(flyout.className).toContain('arrow-right');
  });

  it('falls back to below when left and right both overflow', () => {
    setViewport(400, 768);
    // right: 200 + 50 + 380 + 10 = 640 > 400 — no
    // left:  200 - 50 - 380 = -230 < 10 — no
    // below: 100 + 50 + 200 + 10 = 360 < 768 — yes
    const flyout = showFlyout(200, 100);
    expect(flyout.className).toContain('arrow-up');
  });

  it('falls back to above as last resort', () => {
    setViewport(400, 300);
    // right: no, left: no
    // below: 280 + 50 + 200 + 10 = 540 > 300 — no
    // above: 280 - 50 - 200 = 30 > 10 — yes
    const flyout = showFlyout(200, 280);
    expect(flyout.className).toContain('arrow-down');
  });

  it('defaults to right when nothing fits', () => {
    setViewport(100, 100);
    const flyout = showFlyout(50, 50);
    expect(flyout.className).toContain('arrow-left');
  });

  it('keeps existing anchor when it still fits (sticky)', () => {
    showFlyout(200, 300, 'sticky');
    const flyout = document.getElementById('detail-flyout');
    expect(flyout.className).toContain('arrow-left'); // right anchor

    // Move car — right still fits at carVX=500
    mocks.activeView.entities.set('sticky', { displayX: 500, displayY: 300 });
    mocks.activeView.onAfterDraw();

    expect(flyout.className).toContain('arrow-left'); // stays right
  });

  it('switches anchor when current one no longer fits', () => {
    showFlyout(200, 300, 'switch');
    const flyout = document.getElementById('detail-flyout');
    expect(flyout.className).toContain('arrow-left'); // right anchor

    // Move car to right edge — right no longer fits
    mocks.activeView.entities.set('switch', { displayX: 900, displayY: 300 });
    mocks.activeView.onAfterDraw();

    expect(flyout.className).toContain('arrow-right'); // switched to left
  });
});

// ── Flyout boundary clamping ──────────────────────────────────────────

describe('flyout boundary clamping', () => {
  it('clamps top edge to padding when car is near top', () => {
    // right anchor: targetY = carVY - flyoutHeight/2 = 50 - 100 = -50, clamped to 10
    const flyout = showFlyout(200, 50, 'clamp-top');
    expect(parseInt(flyout.style.top)).toBe(10);
  });

  it('clamps bottom edge to viewport', () => {
    // right anchor: targetY = 700 - 100 = 600
    // max Y = 768 - 200 - 10 = 558, clamped to 558
    const flyout = showFlyout(200, 700, 'clamp-bottom');
    expect(parseInt(flyout.style.top)).toBe(558);
  });

  it('clamps left edge when anchor positions flyout off-screen', () => {
    setViewport(400, 300);
    // above anchor: targetX = carVX - flyoutWidth/2 = 50 - 190 = -140, clamped to 10
    // above: 280 - 50 - 200 = 30 > 10 — fits
    const flyout = showFlyout(50, 280, 'clamp-left');
    expect(parseInt(flyout.style.left)).toBe(10);
  });
});

// ── View switching ────────────────────────────────────────────────────

describe('view switching', () => {
  it('V key does nothing when only one view type is registered', async () => {
    const viewBefore = mocks.activeView;
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'v' }));
    expect(mocks.activeView).toBe(viewBefore);
    expect(viewBefore.destroy).not.toHaveBeenCalled();
  });

  it('V key switches view when multiple types are registered', async () => {
    const { getViewTypes } = await import('./ViewRenderer.js');
    getViewTypes.mockReturnValue(['race', 'footrace']);

    const firstView = mocks.activeView;
    mocks.conn.onSnapshot({
      sessions: [makeSession({ id: 's1' }), makeSession({ id: 's2' })],
    });

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'v' }));

    expect(firstView.destroy).toHaveBeenCalled();
    expect(mocks.activeView).not.toBe(firstView);
    expect(mocks.activeView.setAllRacers).toHaveBeenCalled();
  });

  it('switchView destroys minimap before recreating it', async () => {
    const { getViewTypes } = await import('./ViewRenderer.js');
    const { Minimap } = await import('./ui/Minimap.js');
    getViewTypes.mockReturnValue(['race', 'footrace']);

    const destroySpy = vi.spyOn(Minimap.prototype, 'destroy');
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'v' }));

    expect(destroySpy).toHaveBeenCalled();
    destroySpy.mockRestore();
  });

  it('switchView persists view type to localStorage', async () => {
    const { getViewTypes } = await import('./ViewRenderer.js');
    getViewTypes.mockReturnValue(['race', 'footrace']);

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'v' }));

    expect(localStorage.getItem('agent-racer-view')).toBe('footrace');
  });

  it('switchView replays current sessions into new view', async () => {
    const { getViewTypes } = await import('./ViewRenderer.js');
    getViewTypes.mockReturnValue(['race', 'footrace']);

    mocks.conn.onSnapshot({
      sessions: [
        makeSession({ id: 's1', activity: 'thinking' }),
        makeSession({ id: 's2', activity: 'tool_use' }),
      ],
    });

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'v' }));

    const replayCall = mocks.activeView.setAllRacers.mock.calls[0][0];
    expect(replayCall).toHaveLength(2);
    expect(replayCall.map(s => s.id).sort()).toEqual(['s1', 's2']);
  });

  it('switchView sets connected state on new view', async () => {
    const { getViewTypes } = await import('./ViewRenderer.js');
    getViewTypes.mockReturnValue(['race', 'footrace']);

    mocks.conn.onStatus('connected');

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'v' }));

    expect(mocks.activeView.setConnected).toHaveBeenCalledWith(true);
  });

  it('switchView reconciles shared sound engine state', async () => {
    const { getViewTypes } = await import('./ViewRenderer.js');
    getViewTypes.mockReturnValue(['race', 'footrace']);

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'v' }));

    expect(mocks.engine.reconcileViewSwitch).toHaveBeenCalledTimes(1);
  });

  it('V key cycles through view types', async () => {
    const { getViewTypes, createView } = await import('./ViewRenderer.js');
    getViewTypes.mockReturnValue(['race', 'footrace']);

    // First press: race → footrace
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'v' }));
    expect(createView).toHaveBeenLastCalledWith('footrace', expect.anything(), expect.anything());

    // Second press: footrace → race
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'v' }));
    expect(createView).toHaveBeenLastCalledWith('race', expect.anything(), expect.anything());
  });

  it('clears persisted auth token when WS auth fails', () => {
    sessionStorage.setItem('agent-racer-auth-token', 'bad-token');

    mocks.conn.onAuthFailure();

    expect(sessionStorage.getItem('agent-racer-auth-token')).toBeNull();
  });
});
