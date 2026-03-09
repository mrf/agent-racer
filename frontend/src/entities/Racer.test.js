import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { Racer } from './Racer.js';

function makeState(overrides = {}) {
  return {
    id: 'test-1',
    activity: 'thinking',
    model: 'claude-sonnet-4-5-20250929',
    source: 'claude',
    name: 'test',
    ...overrides,
  };
}

function makeParticles() {
  return { emit: vi.fn(), emitWithColor: vi.fn() };
}

function makeInfoCtx(width = 800, height = 400) {
  let font = '';

  return {
    canvas: {
      width,
      height,
      getBoundingClientRect: () => ({ width, height }),
    },
    beginPath: vi.fn(),
    moveTo: vi.fn(),
    lineTo: vi.fn(),
    stroke: vi.fn(),
    arc: vi.fn(),
    fill: vi.fn(),
    closePath: vi.fn(),
    fillText: vi.fn(),
    measureText: vi.fn((text) => ({ width: text.length * 7 })),
    set font(value) {
      font = value;
    },
    get font() {
      return font;
    },
    set fillStyle(_) {},
    set strokeStyle(_) {},
    set lineWidth(_) {},
    set textAlign(_) {},
    set textBaseline(_) {},
  };
}

function makeDrawCtx() {
  const gradient = { addColorStop: vi.fn() };
  return {
    save: vi.fn(),
    restore: vi.fn(),
    translate: vi.fn(),
    rotate: vi.fn(),
    scale: vi.fn(),
    beginPath: vi.fn(),
    moveTo: vi.fn(),
    lineTo: vi.fn(),
    quadraticCurveTo: vi.fn(),
    closePath: vi.fn(),
    fill: vi.fn(),
    stroke: vi.fn(),
    arc: vi.fn(),
    ellipse: vi.fn(),
    fillRect: vi.fn(),
    strokeRect: vi.fn(),
    roundRect: vi.fn(),
    fillText: vi.fn(),
    measureText: vi.fn(() => ({ width: 48 })),
    createRadialGradient: vi.fn(() => gradient),
    createLinearGradient: vi.fn(() => gradient),
    fillStyle: '',
    strokeStyle: '',
    lineWidth: 0,
    font: '',
    textAlign: 'left',
    textBaseline: 'alphabetic',
    globalAlpha: 1,
    filter: 'none',
  };
}

/** Run a tool_use racer at high speed and return the RGB passed to speedLines. */
function extractSpeedLineColor(model, source) {
  const particles = makeParticles();
  const racer = new Racer(makeState({ activity: 'tool_use', model, source }));
  racer.initialized = true;
  racer.displayX = 0;
  racer.targetX = 500;
  racer.animate(particles, 1 / 60);

  const calls = particles.emitWithColor.mock.calls.filter(c => c[0] === 'speedLines');
  if (calls.length === 0) return null;
  return calls[0][4];
}

describe('spring suspension convergence', () => {
  it('converges springY toward zero after impulse', () => {
    const racer = new Racer(makeState());
    racer.springVel = 5;

    for (let i = 0; i < 300; i++) racer.animate(null, 1 / 60);

    expect(Math.abs(racer.springY)).toBeLessThan(0.01);
    expect(Math.abs(racer.springVel)).toBeLessThan(0.01);
  });

  it('oscillates before settling', () => {
    const racer = new Racer(makeState());
    racer.springVel = 10;

    const positions = [];
    for (let i = 0; i < 60; i++) {
      racer.animate(null, 1 / 60);
      positions.push(racer.springY);
    }

    const crossings = positions.filter(
      (p, i) => i > 0 && Math.sign(p) !== Math.sign(positions[i - 1]),
    );
    expect(crossings.length).toBeGreaterThan(0);
  });

  it('damping factor controls decay rate', () => {
    const fast = new Racer(makeState());
    fast.springDamping = 0.8;
    fast.springVel = 5;

    const slow = new Racer(makeState());
    slow.springDamping = 0.98;
    slow.springVel = 5;

    for (let i = 0; i < 30; i++) {
      fast.animate(null, 1 / 60);
      slow.animate(null, 1 / 60);
    }

    expect(Math.abs(fast.springVel)).toBeLessThan(Math.abs(slow.springVel));
  });
});

describe('lerp interpolation', () => {
  it('moves displayX toward targetX', () => {
    const racer = new Racer(makeState());
    racer.initialized = true;
    racer.displayX = 0;
    racer.targetX = 100;

    racer.animate(null, 1 / 60);

    expect(racer.displayX).toBeGreaterThan(0);
    expect(racer.displayX).toBeLessThan(100);
  });

  it('converges to target over many frames', () => {
    const racer = new Racer(makeState());
    racer.initialized = true;
    racer.displayX = 0;
    racer.targetX = 100;
    racer.displayY = 0;
    racer.targetY = 50;

    for (let i = 0; i < 300; i++) racer.animate(null, 1 / 60);

    expect(racer.displayX).toBeCloseTo(100, 0);
    expect(racer.displayY).toBeCloseTo(50, 0);
  });

  it('uses faster lerp during zone transitions', () => {
    const normal = new Racer(makeState());
    normal.initialized = true;
    normal.displayX = 0;
    normal.targetX = 100;
    normal.animate(null, 1 / 60);

    const transitioning = new Racer(makeState());
    transitioning.initialized = true;
    transitioning.displayX = 0;
    transitioning.transitionWaypoints = [{ x: 100, y: 0 }];
    transitioning.waypointIndex = 0;
    transitioning.animate(null, 1 / 60);

    expect(transitioning.displayX).toBeGreaterThan(normal.displayX);
  });

  it('snaps to initial position on first setTarget', () => {
    const racer = new Racer(makeState());
    expect(racer.initialized).toBe(false);

    racer.setTarget(200, 150);

    expect(racer.displayX).toBe(200);
    expect(racer.displayY).toBe(150);
    expect(racer.initialized).toBe(true);
  });

  it('does not snap on subsequent setTarget calls', () => {
    const racer = new Racer(makeState());
    racer.setTarget(100, 100);
    racer.setTarget(500, 500);

    expect(racer.displayX).toBe(100);
    expect(racer.displayY).toBe(100);
    expect(racer.targetX).toBe(500);
    expect(racer.targetY).toBe(500);
  });
});

describe('error stage progression', () => {
  it('stage 0 (skid) for first 0.5s', () => {
    const racer = new Racer(makeState({ activity: 'errored' }));

    for (let i = 0; i < 24; i++) racer.animate(null, 1 / 60); // ~0.4s

    expect(racer.errorStage).toBe(0);
    expect(racer.errorTimer).toBeLessThan(0.5);
  });

  it('stage 1 (spin) between 0.5–1.0s', () => {
    const racer = new Racer(makeState({ activity: 'errored' }));

    for (let i = 0; i < 45; i++) racer.animate(null, 1 / 60); // ~0.75s

    expect(racer.errorStage).toBe(1);
  });

  it('stage 2 (smoke) between 1.0–1.5s', () => {
    const racer = new Racer(makeState({ activity: 'errored' }));

    for (let i = 0; i < 75; i++) racer.animate(null, 1 / 60); // ~1.25s

    expect(racer.errorStage).toBe(2);
  });

  it('stage 3 (darken) after 1.5s', () => {
    const racer = new Racer(makeState({ activity: 'errored' }));

    for (let i = 0; i < 120; i++) racer.animate(null, 1 / 60); // ~2.0s

    expect(racer.errorStage).toBe(3);
  });

  it('emits skid marks in stage 0', () => {
    const particles = makeParticles();
    const racer = new Racer(makeState({ activity: 'errored' }));

    racer.animate(particles, 1 / 60);

    const skids = particles.emit.mock.calls.filter(c => c[0] === 'skidMarks');
    expect(skids.length).toBe(2); // rear + front wheels
  });

  it('emits smoke in stage 2', () => {
    const particles = makeParticles();
    const racer = new Racer(makeState({ activity: 'errored' }));

    for (let i = 0; i < 65; i++) racer.animate(particles, 1 / 60);

    const smoke = particles.emit.mock.calls.filter(c => c[0] === 'smoke');
    expect(smoke.length).toBeGreaterThan(0);
  });

  it('spin accelerates progressively through stages', () => {
    const racer = new Racer(makeState({ activity: 'errored' }));

    // Record spinAngle deltas per stage
    const deltaByStage = [0, 0, 0, 0];
    let prevAngle = racer.spinAngle;

    for (let i = 0; i < 120; i++) {
      racer.animate(null, 1 / 60);
      const delta = racer.spinAngle - prevAngle;
      deltaByStage[racer.errorStage] += delta;
      prevAngle = racer.spinAngle;
    }

    // Stage 1 should spin faster than stage 0
    expect(deltaByStage[1] / 30).toBeGreaterThan(deltaByStage[0] / 30);
  });
});

describe('glow intensity by activity', () => {
  it('thinking → targetGlow 0.08', () => {
    const racer = new Racer(makeState({ activity: 'thinking' }));
    racer.animate(null, 1 / 60);
    expect(racer.targetGlow).toBeCloseTo(0.08);
  });

  it('tool_use → targetGlow 0.12', () => {
    const racer = new Racer(makeState({ activity: 'tool_use' }));
    racer.animate(null, 1 / 60);
    expect(racer.targetGlow).toBeCloseTo(0.12);
  });

  it('waiting → targetGlow 0.05', () => {
    const racer = new Racer(makeState({ activity: 'waiting' }));
    racer.animate(null, 1 / 60);
    expect(racer.targetGlow).toBeCloseTo(0.05);
  });

  it('complete → targetGlow 0.15', () => {
    const racer = new Racer(makeState({ activity: 'complete' }));
    racer.animate(null, 1 / 60);
    expect(racer.targetGlow).toBeCloseTo(0.15);
  });

  it('lost → targetGlow 0', () => {
    const racer = new Racer(makeState({ activity: 'lost' }));
    racer.animate(null, 1 / 60);
    expect(racer.targetGlow).toBe(0);
  });

  it('clamps targetGlow in pit', () => {
    const racer = new Racer(makeState({ activity: 'thinking' }));
    racer.inPit = true;
    racer.animate(null, 1 / 60);
    expect(racer.targetGlow).toBeLessThanOrEqual(0.02);
  });

  it('clamps targetGlow in parking lot', () => {
    const racer = new Racer(makeState({ activity: 'tool_use' }));
    racer.inParkingLot = true;
    racer.animate(null, 1 / 60);
    expect(racer.targetGlow).toBeLessThanOrEqual(0.02);
  });

  it('glowIntensity interpolates toward targetGlow', () => {
    const racer = new Racer(makeState({ activity: 'complete' }));
    racer.glowIntensity = 0;

    for (let i = 0; i < 80; i++) racer.animate(null, 1 / 60);

    expect(racer.glowIntensity).toBeGreaterThan(0.1);
  });

  it('higher burn rate increases targetGlow for thinking', () => {
    const racer = new Racer(makeState({ activity: 'thinking', burnRatePerMinute: 6000 }));
    racer.animate(null, 1 / 60);
    // burnIntensity 3 → 0.08 + 3*0.02 = 0.14
    expect(racer.targetGlow).toBeCloseTo(0.14);
  });

  it('higher burn rate increases targetGlow for tool_use', () => {
    const racer = new Racer(makeState({ activity: 'tool_use', burnRatePerMinute: 3000 }));
    racer.animate(null, 1 / 60);
    // burnIntensity 2 → 0.12 + 2*0.02 = 0.16
    expect(racer.targetGlow).toBeCloseTo(0.16);
  });
});

describe('activity transition detection', () => {
  it('sets transitionTimer and prevActivity on change', () => {
    const racer = new Racer(makeState({ activity: 'thinking' }));
    racer.update(makeState({ activity: 'waiting' }));

    expect(racer.transitionTimer).toBe(0.3);
    expect(racer.prevActivity).toBe('thinking');
  });

  it('skips spring energy for thinking↔tool_use', () => {
    const racer = new Racer(makeState({ activity: 'thinking' }));
    const before = racer.springVel;
    racer.update(makeState({ activity: 'tool_use' }));
    expect(racer.springVel).toBe(before);
  });

  it('adds spring energy for other transitions', () => {
    const racer = new Racer(makeState({ activity: 'waiting' }));
    racer.update(makeState({ activity: 'thinking' }));
    expect(racer.springVel).toBe(2.5);
  });

  it('resets error flags on errored transition', () => {
    const racer = new Racer(makeState({ activity: 'thinking' }));
    racer.update(makeState({ activity: 'errored' }));

    expect(racer.errorStage).toBe(0);
    expect(racer.errorTimer).toBe(0);
    expect(racer.skidEmitted).toBe(false);
    expect(racer.smokeEmitted).toBe(false);
    expect(racer.spinAngle).toBe(0);
  });

  it('resets completion flags on complete transition', () => {
    const racer = new Racer(makeState({ activity: 'thinking' }));
    racer.update(makeState({ activity: 'complete' }));

    expect(racer.confettiEmitted).toBe(false);
    expect(racer.completionTimer).toBe(0);
    expect(racer.goldFlash).toBe(0);
  });

  it('activates hammer on tool_use transition', () => {
    const racer = new Racer(makeState({ activity: 'thinking' }));
    racer.update(makeState({ activity: 'tool_use' }));

    expect(racer.hammerActive).toBe(true);
    expect(racer.hammerSwing).toBe(0);
    expect(racer.hammerImpactEmitted).toBe(false);
  });

  it('restores opacity when resuming from terminal activity', () => {
    const racer = new Racer(makeState({ activity: 'lost' }));
    racer.opacity = 0.2;
    racer.spinAngle = 1.5;

    racer.update(makeState({ activity: 'thinking' }));

    expect(racer.opacity).toBe(1.0);
    expect(racer.spinAngle).toBe(0);
  });

  it('does not restore opacity for terminal→terminal', () => {
    const racer = new Racer(makeState({ activity: 'errored' }));
    racer.opacity = 0.5;

    racer.update(makeState({ activity: 'lost' }));

    // lost is terminal, errored is terminal — no reset
    expect(racer.opacity).toBe(0.5);
  });

  it('transitionTimer counts down in animate', () => {
    const racer = new Racer(makeState({ activity: 'thinking' }));
    racer.update(makeState({ activity: 'waiting' }));
    expect(racer.transitionTimer).toBe(0.3);

    racer.animate(null, 0.1);
    expect(racer.transitionTimer).toBeCloseTo(0.2, 5);

    racer.animate(null, 0.1);
    expect(racer.transitionTimer).toBeCloseTo(0.1, 5);

    racer.animate(null, 0.15);
    expect(racer.transitionTimer).toBe(0);
  });

  it('adds spring bounce on churning start', () => {
    const racer = new Racer(makeState({ activity: 'waiting', isChurning: false }));
    racer.update(makeState({ activity: 'waiting', isChurning: true }));
    expect(racer.springVel).toBeCloseTo(1.2);
  });

  it('no-ops when activity stays the same', () => {
    const racer = new Racer(makeState({ activity: 'thinking' }));
    racer.transitionTimer = 0;

    racer.update(makeState({ activity: 'thinking' }));

    expect(racer.transitionTimer).toBe(0);
    expect(racer.prevActivity).toBe('thinking');
  });
});

describe('directory flag rendering', () => {
  it('prefers the branch name when the worktree basename already embeds it', () => {
    const racer = new Racer(makeState({
      name: 'agent-racer--feature-fast-flags',
      workingDir: '/tmp/agent-racer--feature-fast-flags',
      branch: 'feature-fast-flags',
    }));

    expect(racer._getDirectoryFlagLabel()).toBe('feature-fast-flags');
  });

  it('keeps the pennant inside the canvas on narrow viewports', () => {
    const racer = new Racer(makeState({
      name: 'very-long-directory-name-for-session-alpha',
      workingDir: '/tmp/very-long-directory-name-for-session-alpha',
      branch: 'feature-with-an-even-longer-name-for-visibility',
    }));
    const ctx = makeInfoCtx(300, 140);
    const layout = racer._getDirectoryFlagLayout(ctx, 90, 38, racer._getDirectoryFlagLabel());

    expect(layout.flagLeft).toBeGreaterThanOrEqual(6);
    expect(layout.flagRight).toBeLessThanOrEqual(294);
    expect(layout.flagTop).toBeGreaterThanOrEqual(6);
    expect(layout.label).toContain('...');
  });

  it('scales the pennant font up on wider viewports', () => {
    const racer = new Racer(makeState({
      name: 'session-a',
      workingDir: '/tmp/session-a',
    }));

    const narrow = racer._getDirectoryFlagLayout(makeInfoCtx(320, 180), 220, 90, racer._getDirectoryFlagLabel());
    const wide = racer._getDirectoryFlagLayout(makeInfoCtx(1440, 180), 220, 90, racer._getDirectoryFlagLabel());

    expect(wide.fontSize).toBeGreaterThan(narrow.fontSize);
  });

  it('draws the compacted directory label on the pennant', () => {
    const racer = new Racer(makeState({
      name: 'agent-racer--feature-fast-flags',
      workingDir: '/tmp/agent-racer--feature-fast-flags',
      branch: 'feature-fast-flags',
    }));
    const ctx = makeInfoCtx(1200, 220);

    racer.drawInfo(ctx, 220, 90, { name: 'claude' }, 'thinking');

    expect(ctx.fillText).toHaveBeenCalledWith('feature-fast-flags', expect.any(Number), expect.any(Number));
  });
});

describe('model color lookup', () => {
  it('exact match: claude-sonnet-4-5-20250929 → #06b6d4', () => {
    const rgb = extractSpeedLineColor('claude-sonnet-4-5-20250929', 'claude');
    expect(rgb).toEqual({ r: 6, g: 182, b: 212 });
  });

  it('exact match: claude-opus-4-5-20251101 → #a855f7', () => {
    const rgb = extractSpeedLineColor('claude-opus-4-5-20251101', 'claude');
    expect(rgb).toEqual({ r: 168, g: 85, b: 247 });
  });

  it('exact match: claude-haiku-4-5-20251001 → #22c55e', () => {
    const rgb = extractSpeedLineColor('claude-haiku-4-5-20251001', 'claude');
    expect(rgb).toEqual({ r: 34, g: 197, b: 94 });
  });

  it('fuzzy match: model containing "opus" gets opus color', () => {
    const rgb = extractSpeedLineColor('custom-opus-variant', 'claude');
    expect(rgb).toEqual({ r: 168, g: 85, b: 247 });
  });

  it('fuzzy match: model containing "sonnet" gets sonnet color', () => {
    const rgb = extractSpeedLineColor('my-sonnet-model', 'claude');
    expect(rgb).toEqual({ r: 6, g: 182, b: 212 });
  });

  it('fuzzy match: model containing "haiku" gets haiku color', () => {
    const rgb = extractSpeedLineColor('custom-haiku', 'claude');
    expect(rgb).toEqual({ r: 34, g: 197, b: 94 });
  });

  it('gemini models get blue color', () => {
    const rgb = extractSpeedLineColor('gemini-2.0-flash', 'gemini');
    expect(rgb).toEqual({ r: 66, g: 133, b: 244 });
  });

  it('codex/openai models get green color', () => {
    const rgb = extractSpeedLineColor('o1-preview', 'codex');
    expect(rgb).toEqual({ r: 16, g: 185, b: 129 });
  });

  it('unknown model falls back to gray', () => {
    const rgb = extractSpeedLineColor('totally-unknown-model', undefined);
    expect(rgb).toEqual({ r: 107, g: 114, b: 128 });
  });

  it('null model with source falls back to gray', () => {
    const rgb = extractSpeedLineColor(null, 'custom');
    expect(rgb).toEqual({ r: 107, g: 114, b: 128 });
  });
});

/* ── Racer utility methods (_formatTokenCount, _buildMetricsLabel) ── */

function makeRacer(overrides = {}) {
  return new Racer(makeState(overrides));
}

describe('Racer._formatTokenCount', () => {
  const racer = makeRacer();

  it('returns plain number below 1000', () => {
    expect(racer._formatTokenCount(0)).toBe('0');
    expect(racer._formatTokenCount(500)).toBe('500');
    expect(racer._formatTokenCount(999)).toBe('999');
  });

  it('returns K format at 1000', () => {
    expect(racer._formatTokenCount(1000)).toBe('1K');
  });

  it('rounds to nearest K', () => {
    expect(racer._formatTokenCount(1499)).toBe('1K');
    expect(racer._formatTokenCount(1500)).toBe('2K');
    expect(racer._formatTokenCount(50000)).toBe('50K');
  });
});

describe('Racer._buildMetricsLabel', () => {
  const racer = makeRacer();

  it('includes context utilization percentage', () => {
    const label = racer._buildMetricsLabel({ contextUtilization: 0.75 });
    expect(label).toContain('75%');
  });

  it('rounds utilization to integer', () => {
    const label = racer._buildMetricsLabel({ contextUtilization: 0.333 });
    expect(label).toContain('33%');
  });

  it('includes token usage with max', () => {
    const label = racer._buildMetricsLabel({
      contextUtilization: 0.5,
      tokensUsed: 5000,
      maxContextTokens: 100000,
    });
    expect(label).toContain('5K/100K');
  });

  it('includes token usage without max', () => {
    const label = racer._buildMetricsLabel({
      contextUtilization: 0.5,
      tokensUsed: 5000,
    });
    expect(label).toContain('5K');
    expect(label).not.toContain('/');
  });

  it('includes formatted burn rate', () => {
    const label = racer._buildMetricsLabel({
      contextUtilization: 0.5,
      burnRatePerMinute: 566093.5,
    });
    expect(label).toContain('566K/min');
  });

  it.each(['complete', 'errored', 'lost'])(
    'shows a deliberate dash instead of stale burn rate for %s sessions',
    (activity) => {
      const label = racer._buildMetricsLabel({
        activity,
        contextUtilization: 0.5,
        burnRatePerMinute: 566093.5,
      });
      if (activity === 'complete') {
        expect(label).toContain('DONE');
        expect(label).not.toContain('50%');
      } else {
        expect(label).toContain('50%');
      }
      expect(label).toContain('· -');
      expect(label).not.toContain('566K/min');
    },
  );

  describe('with fake timers', () => {
    beforeEach(() => { vi.useFakeTimers(); });
    afterEach(() => { vi.useRealTimers(); });

    it('includes elapsed minutes', () => {
      vi.setSystemTime(new Date('2025-01-01T00:05:00Z'));
      const label = racer._buildMetricsLabel({
        contextUtilization: 0,
        startedAt: '2025-01-01T00:00:00Z',
      });
      expect(label).toContain('5m');
    });

    it('includes elapsed seconds for short durations', () => {
      vi.setSystemTime(new Date('2025-01-01T00:00:30Z'));
      const label = racer._buildMetricsLabel({
        contextUtilization: 0,
        startedAt: '2025-01-01T00:00:00Z',
      });
      expect(label).toContain('30s');
    });

    it('joins parts with dot separator', () => {
      vi.setSystemTime(new Date('2025-01-01T00:05:00Z'));
      const label = racer._buildMetricsLabel({
        contextUtilization: 0.5,
        tokensUsed: 5000,
        maxContextTokens: 100000,
        burnRatePerMinute: 43955.1,
        startedAt: '2025-01-01T00:00:00Z',
      });
      expect(label).toBe('50% · 5K/100K · 44.0K/min · 5m');
    });

    it('keeps the burn-rate slot intentional for terminal sessions', () => {
      vi.setSystemTime(new Date('2025-01-01T00:05:00Z'));
      const label = racer._buildMetricsLabel({
        activity: 'complete',
        contextUtilization: 0.5,
        tokensUsed: 5000,
        maxContextTokens: 100000,
        burnRatePerMinute: 43955.1,
        startedAt: '2025-01-01T00:00:00Z',
      });
      expect(label).toBe('DONE · 5K/100K · - · 5m');
    });
  });
});

describe('completed track rendering', () => {
  it('treats completed track racers as a distinct visual state', () => {
    const racer = new Racer(makeState({ activity: 'complete', contextUtilization: 1, tokensUsed: 5000 }));
    racer.initialized = true;
    racer.displayX = 120;
    racer.displayY = 80;
    racer.targetX = 120;
    racer.targetY = 80;

    const ctx = makeDrawCtx();
    const completionBadge = vi.spyOn(racer, '_drawCompletionBadge').mockImplementation(() => {});
    const checkerFlag = vi.spyOn(racer, '_drawCheckerFlag').mockImplementation(() => {});
    vi.spyOn(racer, 'drawCar').mockImplementation(() => {});
    vi.spyOn(racer, 'drawInfo').mockImplementation(() => {});

    racer.draw(ctx);

    expect(completionBadge).toHaveBeenCalled();
    expect(checkerFlag).not.toHaveBeenCalled();
    expect(ctx.globalAlpha).toBeCloseTo(0.72);
    expect(ctx.filter).toBe('grayscale(0.35) saturate(0.45)');
  });

  it('uses DONE instead of 100% in completed metrics labels', () => {
    const racer = new Racer(makeState());
    const label = racer._buildMetricsLabel({
      activity: 'complete',
      contextUtilization: 1,
      tokensUsed: 5000,
      maxContextTokens: 5000,
    });

    expect(label).toContain('DONE');
    expect(label).not.toContain('100%');
  });
});
