import { describe, it, expect, vi, beforeEach } from 'vitest';
import { Dashboard } from './Dashboard.js';

const DEFAULT_BOUNDS = { x: 0, y: 0, width: 800, height: 400 };

function makeSession(overrides = {}) {
  return {
    name: 'session-1',
    activity: 'thinking',
    model: 'claude-opus-4-5-20250514',
    tokensUsed: 50000,
    toolCallCount: 10,
    messageCount: 5,
    contextUtilization: 0.3,
    startedAt: new Date(Date.now() - 120_000).toISOString(),
    ...overrides,
  };
}

function stubCtx() {
  return {
    fillStyle: '',
    strokeStyle: '',
    lineWidth: 0,
    font: '',
    textAlign: 'left',
    textBaseline: 'alphabetic',
    globalAlpha: 1.0,
    beginPath: vi.fn(),
    moveTo: vi.fn(),
    lineTo: vi.fn(),
    stroke: vi.fn(),
    fillText: vi.fn(),
    fillRect: vi.fn(),
    arc: vi.fn(),
    fill: vi.fn(),
  };
}

/** Creates a stub ctx that records every fillStyle value at the moment fillRect is called. */
function stubTrackingCtx() {
  const ctx = stubCtx();
  const fillStyles = [];
  let currentFillStyle = '';
  Object.defineProperty(ctx, 'fillStyle', {
    get() { return currentFillStyle; },
    set(v) { currentFillStyle = v; },
  });
  ctx.fillRect = vi.fn(function () { fillStyles.push(currentFillStyle); });
  return { ctx, fillStyles };
}

describe('Dashboard', () => {
  let dashboard;

  beforeEach(() => {
    dashboard = new Dashboard();
  });

  describe('zone counts from RaceCanvas (racing / pit / parked)', () => {
    it('renders racing count from zoneCounts', () => {
      const ctx = stubCtx();
      const sessions = [makeSession(), makeSession(), makeSession()];
      dashboard.draw(ctx, DEFAULT_BOUNDS, sessions, { racing: 2, pit: 1, parked: 0 });

      const racingCall = ctx.fillText.mock.calls.find(
        ([val, , ], idx) => val === '2' && ctx.fillText.mock.calls[idx + 1]?.[0] === 'RACING',
      );
      expect(racingCall).toBeDefined();
    });

    it('renders pit count from zoneCounts', () => {
      const ctx = stubCtx();
      const sessions = [makeSession(), makeSession(), makeSession(), makeSession()];
      dashboard.draw(ctx, DEFAULT_BOUNDS, sessions, { racing: 1, pit: 3, parked: 0 });

      const pitCall = ctx.fillText.mock.calls.find(
        ([val, , ], idx) => val === '3' && ctx.fillText.mock.calls[idx + 1]?.[0] === 'PIT',
      );
      expect(pitCall).toBeDefined();
    });

    it('renders parked count from zoneCounts', () => {
      const ctx = stubCtx();
      const sessions = [makeSession(), makeSession(), makeSession(), makeSession()];
      dashboard.draw(ctx, DEFAULT_BOUNDS, sessions, { racing: 1, pit: 0, parked: 3 });

      const parkedCall = ctx.fillText.mock.calls.find(
        ([val, , ], idx) => val === '3' && ctx.fillText.mock.calls[idx + 1]?.[0] === 'PARKED',
      );
      expect(parkedCall).toBeDefined();
    });

    it('shows zero counts when zoneCounts missing', () => {
      const ctx = stubCtx();
      dashboard.draw(ctx, DEFAULT_BOUNDS, []);

      const labels = ctx.fillText.mock.calls.map(c => c[0]);
      expect(labels).toContain('RACING');
      expect(labels).toContain('PIT');
      expect(labels).toContain('PARKED');
      const racingIdx = labels.indexOf('RACING');
      expect(labels[racingIdx - 1]).toBe('0');
    });
  });

  describe('leaderboard sorting by context utilization', () => {
    it('sorts rows by contextUtilization descending', () => {
      const ctx = stubCtx();
      const sessions = [
        makeSession({ name: 'low', contextUtilization: 0.1 }),
        makeSession({ name: 'high', contextUtilization: 0.9 }),
        makeSession({ name: 'mid', contextUtilization: 0.5 }),
      ];
      dashboard.draw(ctx, DEFAULT_BOUNDS, sessions);

      const nameArgs = ctx.fillText.mock.calls
        .filter(([val]) => ['low', 'mid', 'high'].includes(val))
        .map(([val]) => val);
      expect(nameArgs).toEqual(['high', 'mid', 'low']);
    });

    it('treats missing contextUtilization as 0', () => {
      const ctx = stubCtx();
      const sessions = [
        makeSession({ name: 'none', contextUtilization: undefined }),
        makeSession({ name: 'some', contextUtilization: 0.4 }),
      ];
      dashboard.draw(ctx, DEFAULT_BOUNDS, sessions);

      const nameArgs = ctx.fillText.mock.calls
        .filter(([val]) => ['none', 'some'].includes(val))
        .map(([val]) => val);
      expect(nameArgs).toEqual(['some', 'none']);
    });
  });

  describe('leaderboard max 12 row truncation', () => {
    it('renders at most 12 rows', () => {
      const ctx = stubCtx();
      const names = Array.from({ length: 15 }, (_, i) => `sess-${i}`);
      const sessions = names.map((name, i) =>
        makeSession({ name, contextUtilization: (15 - i) / 100 }),
      );
      dashboard.draw(ctx, { x: 0, y: 0, width: 800, height: 800 }, sessions);

      const renderedNames = ctx.fillText.mock.calls
        .filter(([val]) => typeof val === 'string' && val.startsWith('sess-'))
        .map(([val]) => val);
      expect(renderedNames).toHaveLength(12);
    });

    it('getRequiredHeight caps row count at 12', () => {
      const h12 = dashboard.getRequiredHeight(12);
      const h20 = dashboard.getRequiredHeight(20);
      expect(h20).toBe(h12);
    });
  });

  describe('token formatting', () => {
    it('formats millions with one decimal', () => {
      const ctx = stubCtx();
      dashboard.draw(ctx, DEFAULT_BOUNDS, [makeSession({ tokensUsed: 2_500_000 })]);

      const labels = ctx.fillText.mock.calls.map(c => c[0]);
      expect(labels).toContain('2.5M');
    });

    it('formats thousands as rounded K', () => {
      const ctx = stubCtx();
      dashboard.draw(ctx, DEFAULT_BOUNDS, [makeSession({ tokensUsed: 45_200 })]);

      const labels = ctx.fillText.mock.calls.map(c => c[0]);
      expect(labels).toContain('45K');
    });

    it('formats small values as plain numbers', () => {
      const ctx = stubCtx();
      dashboard.draw(ctx, DEFAULT_BOUNDS, [makeSession({ tokensUsed: 500 })]);

      const labels = ctx.fillText.mock.calls.map(c => c[0]);
      expect(labels).toContain('500');
    });

    it('formats zero tokens', () => {
      const ctx = stubCtx();
      dashboard.draw(ctx, DEFAULT_BOUNDS, [makeSession({ tokensUsed: 0 })]);

      const labels = ctx.fillText.mock.calls.map(c => c[0]);
      expect(labels).toContain('0');
    });

    it('formats exactly 1M', () => {
      const ctx = stubCtx();
      dashboard.draw(ctx, DEFAULT_BOUNDS, [makeSession({ tokensUsed: 1_000_000 })]);

      const labels = ctx.fillText.mock.calls.map(c => c[0]);
      expect(labels).toContain('1.0M');
    });
  });

  describe('elapsed time display', () => {
    it('shows minutes for < 60 minutes', () => {
      const ctx = stubCtx();
      dashboard.draw(ctx, DEFAULT_BOUNDS, [
        makeSession({ startedAt: new Date(Date.now() - 25 * 60_000).toISOString() }),
      ]);

      const labels = ctx.fillText.mock.calls.map(c => c[0]);
      expect(labels).toContain('25m');
    });

    it('shows hours and minutes for >= 60 minutes', () => {
      const ctx = stubCtx();
      dashboard.draw(ctx, DEFAULT_BOUNDS, [
        makeSession({ startedAt: new Date(Date.now() - 90 * 60_000).toISOString() }),
      ]);

      const labels = ctx.fillText.mock.calls.map(c => c[0]);
      expect(labels).toContain('1h30m');
    });

    it('shows empty string for missing startedAt', () => {
      const ctx = stubCtx();
      dashboard.draw(ctx, DEFAULT_BOUNDS, [makeSession({ startedAt: null })]);

      const labels = ctx.fillText.mock.calls.map(c => c[0]);
      expect(labels).toContain('');
    });

    it('shows 0m for just-started sessions', () => {
      const ctx = stubCtx();
      dashboard.draw(ctx, DEFAULT_BOUNDS, [
        makeSession({ startedAt: new Date().toISOString() }),
      ]);

      const labels = ctx.fillText.mock.calls.map(c => c[0]);
      expect(labels).toContain('0m');
    });
  });

  describe('context bar color thresholds', () => {
    it('uses green (#22c55e) for utilization < 50%', () => {
      const { ctx, fillStyles } = stubTrackingCtx();
      dashboard.draw(ctx, DEFAULT_BOUNDS, [makeSession({ contextUtilization: 0.3 })]);
      expect(fillStyles).toContain('#22c55e');
    });

    it('uses amber (#d97706) for utilization > 50% and <= 80%', () => {
      const { ctx, fillStyles } = stubTrackingCtx();
      dashboard.draw(ctx, DEFAULT_BOUNDS, [makeSession({ contextUtilization: 0.65 })]);
      expect(fillStyles).toContain('#d97706');
    });

    it('uses red (#e94560) for utilization > 80%', () => {
      const { ctx, fillStyles } = stubTrackingCtx();
      dashboard.draw(ctx, DEFAULT_BOUNDS, [makeSession({ contextUtilization: 0.95 })]);
      expect(fillStyles).toContain('#e94560');
    });

    it('uses green at exactly 50% (boundary)', () => {
      const { ctx, fillStyles } = stubTrackingCtx();
      dashboard.draw(ctx, DEFAULT_BOUNDS, [makeSession({ contextUtilization: 0.5 })]);
      expect(fillStyles).toContain('#22c55e');
      expect(fillStyles).not.toContain('#d97706');
      expect(fillStyles).not.toContain('#e94560');
    });

    it('uses amber at exactly 80% (boundary)', () => {
      const { ctx, fillStyles } = stubTrackingCtx();
      dashboard.draw(ctx, DEFAULT_BOUNDS, [makeSession({ contextUtilization: 0.8 })]);
      expect(fillStyles).toContain('#d97706');
      expect(fillStyles).not.toContain('#e94560');
    });
  });

  describe('edge cases', () => {
    it('skips drawing when bounds height < 40', () => {
      const ctx = stubCtx();
      dashboard.draw(ctx, { x: 0, y: 0, width: 800, height: 30 }, [makeSession()]);
      expect(ctx.fillText).not.toHaveBeenCalled();
    });

    it('skips drawing when bounds is null', () => {
      const ctx = stubCtx();
      dashboard.draw(ctx, null, [makeSession()]);
      expect(ctx.fillText).not.toHaveBeenCalled();
    });

    it('getMinHeight returns expected constant', () => {
      expect(dashboard.getMinHeight()).toBe(160);
    });
  });
});
