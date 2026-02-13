import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { SoundEngine } from './SoundEngine.js';

// --- AudioContext mock helpers ---

function mockAudioParam(initialValue = 0) {
  return {
    value: initialValue,
    setValueAtTime: vi.fn(),
    linearRampToValueAtTime: vi.fn(),
    exponentialRampToValueAtTime: vi.fn(),
    cancelScheduledValues: vi.fn(),
  };
}

function mockGainNode() {
  return { gain: mockAudioParam(1), connect: vi.fn() };
}

function mockOscillatorNode() {
  return {
    type: 'sine',
    frequency: mockAudioParam(440),
    connect: vi.fn(),
    start: vi.fn(),
    stop: vi.fn(),
  };
}

function mockFilterNode() {
  return {
    type: 'lowpass',
    frequency: mockAudioParam(350),
    Q: mockAudioParam(1),
    connect: vi.fn(),
  };
}

function mockCompressorNode() {
  return {
    threshold: mockAudioParam(-24),
    knee: mockAudioParam(30),
    ratio: mockAudioParam(12),
    attack: mockAudioParam(0.003),
    release: mockAudioParam(0.25),
    connect: vi.fn(),
  };
}

function mockBufferSource() {
  return {
    buffer: null,
    loop: false,
    connect: vi.fn(),
    start: vi.fn(),
    stop: vi.fn(),
  };
}

function mockConvolverNode() {
  return { buffer: null, connect: vi.fn() };
}

function createMockAudioContext() {
  return {
    sampleRate: 44100,
    currentTime: 0,
    state: 'running',
    destination: {},
    resume: vi.fn(),
    close: vi.fn().mockResolvedValue(undefined),
    createGain: vi.fn(() => mockGainNode()),
    createOscillator: vi.fn(() => mockOscillatorNode()),
    createBiquadFilter: vi.fn(() => mockFilterNode()),
    createDynamicsCompressor: vi.fn(() => mockCompressorNode()),
    createBuffer: vi.fn(() => ({ getChannelData: () => new Float32Array(44100) })),
    createBufferSource: vi.fn(() => mockBufferSource()),
    createConvolver: vi.fn(() => mockConvolverNode()),
  };
}

function installMockAudioContext() {
  const mockCtx = createMockAudioContext();
  globalThis.window = { AudioContext: vi.fn(() => mockCtx) };
  return mockCtx;
}

// --- Helpers ---

/** Reproduce the hash algorithm from SoundEngine.startEngine */
function computeHash(racerId) {
  let hash = 0;
  for (let i = 0; i < racerId.length; i++) {
    hash = ((hash << 5) - hash + racerId.charCodeAt(i)) | 0;
  }
  return hash;
}

function expectedBaseFreq(racerId) {
  return 65 + (Math.abs(computeHash(racerId)) % 31);
}

function expectedDetune(racerId) {
  return 1.5 + (Math.abs(computeHash(racerId) >> 8) % 20) / 10;
}

describe('SoundEngine', () => {
  let engine;
  let mockCtx;

  beforeEach(() => {
    vi.useFakeTimers();
    mockCtx = installMockAudioContext();
    engine = new SoundEngine();
  });

  afterEach(() => {
    vi.useRealTimers();
    delete globalThis.window;
  });

  describe('excitement tieredScore', () => {
    beforeEach(() => {
      engine._ensureCtx();
      engine.ambientRunning = true;
    });

    it('returns 0 excitement when no sessions are active', () => {
      engine.updateExcitement([]);
      expect(engine.targetExcitement).toBe(0);
    });

    it('scores active sessions by tier thresholds', () => {
      // 1 active (thinking) → ACTIVE_SESSION_TIERS: 0.2
      engine.updateExcitement([{ activity: 'thinking', contextUtilization: 0 }]);
      expect(engine.targetExcitement).toBeCloseTo(0.2);

      // 2 active → 0.4
      engine.updateExcitement([
        { activity: 'thinking', contextUtilization: 0 },
        { activity: 'tool_use', contextUtilization: 0 },
      ]);
      expect(engine.targetExcitement).toBeCloseTo(0.4);

      // 3 active → 0.55
      engine.updateExcitement([
        { activity: 'thinking', contextUtilization: 0 },
        { activity: 'tool_use', contextUtilization: 0 },
        { activity: 'thinking', contextUtilization: 0 },
      ]);
      expect(engine.targetExcitement).toBeCloseTo(0.55);

      // 4 active → 0.7
      engine.updateExcitement([
        { activity: 'thinking', contextUtilization: 0 },
        { activity: 'tool_use', contextUtilization: 0 },
        { activity: 'thinking', contextUtilization: 0 },
        { activity: 'tool_use', contextUtilization: 0 },
      ]);
      expect(engine.targetExcitement).toBeCloseTo(0.7);
    });

    it('scores near-finish sessions (contextUtilization > 0.8)', () => {
      // 1 near-finish, non-active → NEAR_FINISH_TIERS: 0.1
      engine.updateExcitement([{ activity: 'waiting', contextUtilization: 0.85 }]);
      expect(engine.targetExcitement).toBeCloseTo(0.1);

      // 2 near-finish → 0.2
      engine.updateExcitement([
        { activity: 'waiting', contextUtilization: 0.9 },
        { activity: 'waiting', contextUtilization: 0.95 },
      ]);
      expect(engine.targetExcitement).toBeCloseTo(0.2);

      // 3 near-finish → 0.3
      engine.updateExcitement([
        { activity: 'waiting', contextUtilization: 0.85 },
        { activity: 'waiting', contextUtilization: 0.9 },
        { activity: 'waiting', contextUtilization: 0.95 },
      ]);
      expect(engine.targetExcitement).toBeCloseTo(0.3);
    });

    it('excludes complete/errored/lost sessions from near-finish count', () => {
      engine.updateExcitement([
        { activity: 'complete', contextUtilization: 0.95 },
        { activity: 'errored', contextUtilization: 0.99 },
        { activity: 'lost', contextUtilization: 0.85 },
      ]);
      expect(engine.targetExcitement).toBe(0);
    });

    it('scores recent events by tier', () => {
      // 1 recent event → RECENT_EVENT_TIERS: 0.15
      engine.recordCompletion();
      engine.updateExcitement([]);
      expect(engine.targetExcitement).toBeCloseTo(0.15);

      // 2 recent events → 0.2
      engine.recordCrash();
      engine.updateExcitement([]);
      expect(engine.targetExcitement).toBeCloseTo(0.2);

      // 3 recent events → 0.3
      engine.recordCompletion();
      engine.updateExcitement([]);
      expect(engine.targetExcitement).toBeCloseTo(0.3);
    });

    it('combines all three tier categories', () => {
      // 2 active (0.4) + 1 near-finish (0.1) + 1 event (0.15) = 0.65
      engine.recordCompletion();
      engine.updateExcitement([
        { activity: 'thinking', contextUtilization: 0 },
        { activity: 'tool_use', contextUtilization: 0.85 },
      ]);
      expect(engine.targetExcitement).toBeCloseTo(0.65);
    });

    it('clamps excitement to 1', () => {
      // 4 active (0.7) + 3 near-finish (0.3) + 3 events (0.3) = 1.3 → clamped to 1
      engine.recordCompletion();
      engine.recordCrash();
      engine.recordCompletion();
      engine.updateExcitement([
        { activity: 'thinking', contextUtilization: 0.85 },
        { activity: 'tool_use', contextUtilization: 0.9 },
        { activity: 'thinking', contextUtilization: 0.95 },
        { activity: 'tool_use', contextUtilization: 0 },
      ]);
      expect(engine.targetExcitement).toBe(1);
    });

    it('discards events older than EVENT_WINDOW_MS (10s)', () => {
      engine.recordCompletion();
      // Advance time past the 10s window
      vi.advanceTimersByTime(11000);
      engine.updateExcitement([]);
      expect(engine.targetExcitement).toBe(0);
    });

    it('does nothing when ambient is not running', () => {
      engine.ambientRunning = false;
      engine.recordCompletion();
      engine.updateExcitement([{ activity: 'thinking', contextUtilization: 0 }]);
      expect(engine.targetExcitement).toBe(0);
    });
  });

  describe('hash-based frequency stability', () => {
    beforeEach(() => {
      engine._ensureCtx();
    });

    it('derives a stable base frequency from racerId', () => {
      const racerId = 'racer-alpha';
      const freq = expectedBaseFreq(racerId);
      expect(freq).toBeGreaterThanOrEqual(65);
      expect(freq).toBeLessThanOrEqual(95);

      // Call startEngine twice — should set same frequency both times
      engine.startEngine(racerId, 'thinking');
      const osc1 = mockCtx.createOscillator.mock.results.at(-2).value;
      expect(osc1.frequency.value).toBe(freq);

      // Stop and restart — must produce same frequency
      engine.stopEngine(racerId);
      vi.advanceTimersByTime(500);
      engine.startEngine(racerId, 'thinking');
      const osc1b = mockCtx.createOscillator.mock.results.at(-2).value;
      expect(osc1b.frequency.value).toBe(freq);
    });

    it('produces different frequencies for different racerIds', () => {
      const ids = ['racer-a', 'racer-b', 'racer-c', 'racer-xyz-99'];
      const freqs = ids.map(expectedBaseFreq);
      // At least some should differ (hash collision is theoretically possible
      // but extremely unlikely for these distinct strings)
      const unique = new Set(freqs);
      expect(unique.size).toBeGreaterThan(1);
    });

    it('keeps base frequency within [65, 95] Hz range', () => {
      const ids = ['a', 'bb', 'ccc', 'test-12345', 'x'.repeat(100)];
      for (const id of ids) {
        const freq = expectedBaseFreq(id);
        expect(freq).toBeGreaterThanOrEqual(65);
        expect(freq).toBeLessThanOrEqual(95);
      }
    });

    it('sets detune oscillator to baseFreq + detune offset', () => {
      const racerId = 'racer-detune-test';
      const base = expectedBaseFreq(racerId);
      const det = expectedDetune(racerId);

      engine.startEngine(racerId, 'thinking');
      const osc2 = mockCtx.createOscillator.mock.results.at(-1).value;
      expect(osc2.frequency.value).toBeCloseTo(base + det);
    });
  });

  describe('pitch multipliers by activity', () => {
    beforeEach(() => {
      engine._ensureCtx();
    });

    it('uses 1.0x pitch for thinking activity', () => {
      const racerId = 'pitch-test';
      const base = expectedBaseFreq(racerId);

      engine.startEngine(racerId, 'thinking');
      const osc1 = mockCtx.createOscillator.mock.results.at(-2).value;
      expect(osc1.frequency.value).toBe(base);
    });

    it('uses 1.4x pitch for tool_use activity', () => {
      const racerId = 'pitch-test';
      const base = expectedBaseFreq(racerId);

      engine.startEngine(racerId, 'tool_use');
      const osc1 = mockCtx.createOscillator.mock.results.at(-2).value;
      expect(osc1.frequency.value).toBeCloseTo(base * 1.4);
    });

    it('uses 0.7x pitch for churning activity', () => {
      const racerId = 'pitch-test';
      const base = expectedBaseFreq(racerId);

      engine.startEngine(racerId, 'churning');
      const osc1 = mockCtx.createOscillator.mock.results.at(-2).value;
      expect(osc1.frequency.value).toBeCloseTo(base * 0.7);
    });

    it('sets lower volume for churning (0.003 vs 0.008)', () => {
      const racerId = 'volume-test';
      engine.startEngine(racerId, 'churning');
      const gain = mockCtx.createGain.mock.results.at(-1).value;
      expect(gain.gain.linearRampToValueAtTime).toHaveBeenCalledWith(0.003, expect.any(Number));
    });

    it('sets standard volume for thinking (0.008)', () => {
      const racerId = 'volume-test-2';
      engine.startEngine(racerId, 'thinking');
      const gain = mockCtx.createGain.mock.results.at(-1).value;
      expect(gain.gain.linearRampToValueAtTime).toHaveBeenCalledWith(0.008, expect.any(Number));
    });

    it('stops engine for non-engine activities', () => {
      const racerId = 'activity-stop';
      engine.startEngine(racerId, 'thinking');
      expect(engine.engineNodes.has(racerId)).toBe(true);

      engine.startEngine(racerId, 'waiting');
      expect(engine.engineNodes.has(racerId)).toBe(false);
    });

    it('applies pitch to filter cutoff as baseFreq * 2.5 * pitchMult', () => {
      const racerId = 'filter-test';
      const base = expectedBaseFreq(racerId);

      engine.startEngine(racerId, 'tool_use');
      const filter = mockCtx.createBiquadFilter.mock.results.at(-1).value;
      expect(filter.frequency.value).toBeCloseTo(base * 2.5 * 1.4);
    });
  });

  describe('mute/unmute state', () => {
    beforeEach(() => {
      engine._ensureCtx();
    });

    it('starts unmuted by default', () => {
      expect(engine.muted).toBe(false);
    });

    it('sets masterGain to 0 when muted', () => {
      engine.setMuted(true);
      expect(engine.muted).toBe(true);
      expect(engine.masterGain.gain.value).toBe(0);
    });

    it('sets masterGain to 1 when unmuted', () => {
      engine.setMuted(true);
      engine.setMuted(false);
      expect(engine.muted).toBe(false);
      expect(engine.masterGain.gain.value).toBe(1);
    });

    it('stops all running engines when muted', () => {
      engine.startEngine('r1', 'thinking');
      engine.startEngine('r2', 'tool_use');
      expect(engine.engineNodes.size).toBe(2);

      engine.setMuted(true);
      expect(engine.engineNodes.size).toBe(0);
    });

    it('prevents startEngine from creating nodes when muted', () => {
      engine.setMuted(true);
      engine.startEngine('r1', 'thinking');
      expect(engine.engineNodes.size).toBe(0);
    });

    it('prevents SFX from playing when muted', () => {
      engine.setMuted(true);
      const oscCountBefore = mockCtx.createOscillator.mock.calls.length;
      engine.playGearShift();
      engine.playToolClick();
      engine.playCrash();
      engine.playVictory();
      engine.playAppear();
      engine.playDisappear();
      expect(mockCtx.createOscillator.mock.calls.length).toBe(oscCountBefore);
    });

    it('allows SFX after unmuting', () => {
      engine.setMuted(true);
      engine.setMuted(false);
      const oscCountBefore = mockCtx.createOscillator.mock.calls.length;
      engine.playGearShift();
      expect(mockCtx.createOscillator.mock.calls.length).toBeGreaterThan(oscCountBefore);
    });

    it('applyConfig with enabled=false mutes', () => {
      engine.applyConfig({ enabled: false });
      expect(engine.muted).toBe(true);
      expect(engine.masterGain.gain.value).toBe(0);
    });

    it('applyConfig with enabled=true unmutes', () => {
      engine.setMuted(true);
      engine.applyConfig({ enabled: true });
      expect(engine.muted).toBe(false);
      expect(engine.masterGain.gain.value).toBe(1);
    });
  });

  describe('engine lifecycle', () => {
    beforeEach(() => {
      engine._ensureCtx();
    });

    it('smoothly updates pitch when activity changes on existing engine', () => {
      const racerId = 'smooth-update';
      const base = expectedBaseFreq(racerId);
      const det = expectedDetune(racerId);

      engine.startEngine(racerId, 'thinking');
      const nodes = engine.engineNodes.get(racerId);

      // Switch to tool_use — should ramp frequency, not recreate
      engine.startEngine(racerId, 'tool_use');
      expect(nodes.osc1.frequency.linearRampToValueAtTime)
        .toHaveBeenCalledWith(base * 1.4, expect.any(Number));
      expect(nodes.osc2.frequency.linearRampToValueAtTime)
        .toHaveBeenCalledWith((base + det) * 1.4, expect.any(Number));
    });

    it('cancels pending stop when restarting engine', () => {
      const racerId = 'cancel-stop';
      engine.startEngine(racerId, 'thinking');
      engine.stopEngine(racerId);
      expect(engine.engineStopTimeouts.has(racerId)).toBe(true);

      // Restart before stop timeout fires
      engine.startEngine(racerId, 'thinking');
      expect(engine.engineStopTimeouts.has(racerId)).toBe(false);
    });
  });

  describe('event recording', () => {
    it('records completion events with timestamp', () => {
      engine.recordCompletion();
      expect(engine.recentEvents).toHaveLength(1);
      expect(engine.recentEvents[0].type).toBe('completion');
      expect(engine.recentEvents[0].timestamp).toBeTypeOf('number');
    });

    it('records crash events with timestamp', () => {
      engine.recordCrash();
      expect(engine.recentEvents).toHaveLength(1);
      expect(engine.recentEvents[0].type).toBe('crash');
      expect(engine.recentEvents[0].timestamp).toBeTypeOf('number');
    });

    it('accumulates multiple events', () => {
      engine.recordCompletion();
      engine.recordCrash();
      engine.recordCompletion();
      expect(engine.recentEvents).toHaveLength(3);
    });
  });
});
