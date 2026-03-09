import { describe, it, expect, vi, afterEach } from 'vitest';
import { isParkingLotRacer, isPitRacer, classifyZone } from './zones.js';
import { DATA_FRESHNESS_MS } from './constants.js';

function agoISO(ms) {
  return new Date(Date.now() - ms).toISOString();
}

function makeState(overrides = {}) {
  return {
    activity: 'thinking',
    lastDataReceivedAt: agoISO(0),
    ...overrides,
  };
}

afterEach(() => {
  vi.useRealTimers();
});

describe('isParkingLotRacer', () => {
  it('returns true for complete', () => {
    expect(isParkingLotRacer(makeState({ activity: 'complete' }))).toBe(true);
  });

  it('returns true for errored', () => {
    expect(isParkingLotRacer(makeState({ activity: 'errored' }))).toBe(true);
  });

  it('returns true for lost', () => {
    expect(isParkingLotRacer(makeState({ activity: 'lost' }))).toBe(true);
  });

  it('returns false for thinking', () => {
    expect(isParkingLotRacer(makeState({ activity: 'thinking' }))).toBe(false);
  });

  it('returns false for tool_use', () => {
    expect(isParkingLotRacer(makeState({ activity: 'tool_use' }))).toBe(false);
  });

  it('returns false for idle', () => {
    expect(isParkingLotRacer(makeState({ activity: 'idle' }))).toBe(false);
  });

  it('returns false for waiting', () => {
    expect(isParkingLotRacer(makeState({ activity: 'waiting' }))).toBe(false);
  });

  it('returns false for starting', () => {
    expect(isParkingLotRacer(makeState({ activity: 'starting' }))).toBe(false);
  });
});

describe('isPitRacer', () => {
  it('returns true for idle with stale data', () => {
    expect(isPitRacer(makeState({ activity: 'idle', lastDataReceivedAt: agoISO(31_000) }))).toBe(true);
  });

  it('returns true for waiting with stale data', () => {
    expect(isPitRacer(makeState({ activity: 'waiting', lastDataReceivedAt: agoISO(31_000) }))).toBe(true);
  });

  it('returns true for starting with stale data', () => {
    expect(isPitRacer(makeState({ activity: 'starting', lastDataReceivedAt: agoISO(31_000) }))).toBe(true);
  });

  it('returns false for idle with fresh data', () => {
    expect(isPitRacer(makeState({ activity: 'idle', lastDataReceivedAt: agoISO(0) }))).toBe(false);
  });

  it('returns false for thinking even with stale data', () => {
    expect(isPitRacer(makeState({ activity: 'thinking', lastDataReceivedAt: agoISO(60_000) }))).toBe(false);
  });

  it('returns false for tool_use even with stale data', () => {
    expect(isPitRacer(makeState({ activity: 'tool_use', lastDataReceivedAt: agoISO(60_000) }))).toBe(false);
  });

  it('returns false for terminal activities regardless of freshness', () => {
    expect(isPitRacer(makeState({ activity: 'complete', lastDataReceivedAt: agoISO(60_000) }))).toBe(false);
    expect(isPitRacer(makeState({ activity: 'errored', lastDataReceivedAt: agoISO(60_000) }))).toBe(false);
    expect(isPitRacer(makeState({ activity: 'lost', lastDataReceivedAt: agoISO(60_000) }))).toBe(false);
  });

  it('returns true for idle with no lastDataReceivedAt', () => {
    expect(isPitRacer(makeState({ activity: 'idle', lastDataReceivedAt: null }))).toBe(true);
  });

  it('boundary: exactly at DATA_FRESHNESS_MS threshold goes to pit', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2025-01-01T00:00:00Z'));
    // age === DATA_FRESHNESS_MS means age < DATA_FRESHNESS_MS is false → pit
    expect(isPitRacer(makeState({ activity: 'idle', lastDataReceivedAt: agoISO(DATA_FRESHNESS_MS) }))).toBe(true);
  });

  it('boundary: 1ms under DATA_FRESHNESS_MS threshold stays on track', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2025-01-01T00:00:00Z'));
    expect(isPitRacer(makeState({ activity: 'idle', lastDataReceivedAt: agoISO(DATA_FRESHNESS_MS - 1) }))).toBe(false);
  });
});

describe('classifyZone', () => {
  it('returns parkingLot for terminal activities', () => {
    expect(classifyZone(makeState({ activity: 'complete' }))).toBe('parkingLot');
    expect(classifyZone(makeState({ activity: 'errored' }))).toBe('parkingLot');
    expect(classifyZone(makeState({ activity: 'lost' }))).toBe('parkingLot');
  });

  it('returns pit for stale pit activities', () => {
    const stale = agoISO(31_000);
    expect(classifyZone(makeState({ activity: 'idle', lastDataReceivedAt: stale }))).toBe('pit');
    expect(classifyZone(makeState({ activity: 'waiting', lastDataReceivedAt: stale }))).toBe('pit');
    expect(classifyZone(makeState({ activity: 'starting', lastDataReceivedAt: stale }))).toBe('pit');
  });

  it('returns track for active activities', () => {
    expect(classifyZone(makeState({ activity: 'thinking' }))).toBe('track');
    expect(classifyZone(makeState({ activity: 'tool_use' }))).toBe('track');
  });

  it('returns track for pit activities with fresh data', () => {
    expect(classifyZone(makeState({ activity: 'idle', lastDataReceivedAt: agoISO(0) }))).toBe('track');
  });

  it('terminal activities go to parkingLot even with fresh data', () => {
    expect(classifyZone(makeState({ activity: 'complete', lastDataReceivedAt: agoISO(0) }))).toBe('parkingLot');
  });
});
