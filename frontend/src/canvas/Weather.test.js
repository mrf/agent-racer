import { describe, expect, it, vi } from 'vitest';
import { WeatherSystem } from './Weather.js';

const FRAME = 1 / 60;

function makeCtx() {
  const linearGradients = [];
  const radialGradients = [];
  const fillStyles = [];
  return {
    linearGradients,
    radialGradients,
    fillStyles,
    createLinearGradient: vi.fn(() => {
      const gradient = { addColorStop: vi.fn() };
      linearGradients.push(gradient);
      return gradient;
    }),
    createRadialGradient: vi.fn(() => {
      const gradient = { addColorStop: vi.fn() };
      radialGradients.push(gradient);
      return gradient;
    }),
    fillRect: vi.fn(),
    save: vi.fn(),
    restore: vi.fn(),
    beginPath: vi.fn(),
    arc: vi.fn(),
    ellipse: vi.fn(),
    fill: vi.fn(),
    moveTo: vi.fn(),
    lineTo: vi.fn(),
    stroke: vi.fn(),
    set fillStyle(value) { fillStyles.push(value); },
    set strokeStyle(_) {},
    set lineWidth(_) {},
    set lineCap(_) {},
    set lineJoin(_) {},
    set globalAlpha(_) {},
    set filter(_) {},
  };
}

function setState(weather, state) {
  weather.currentState = state;
  weather.targetState = state;
  weather.transitionProgress = 1.0;
}

/** Build a metrics object with sensible defaults for the fields not under test. */
function metrics(overrides = {}) {
  return {
    sessionCount: 0,
    activeCount: 0,
    errorCount: 0,
    totalBurnRate: 0,
    compactionCount: 0,
    allComplete: false,
    ...overrides,
  };
}

/** Set metrics and trigger evaluation. */
function evaluateWith(weather, m) {
  Object.assign(weather._metrics, m);
  weather._evaluateState();
}

/** Build a minimal mock session. */
function session(overrides = {}) {
  return {
    activity: 'active',
    burnRatePerMinute: 0,
    compactionCount: 0,
    ...overrides,
  };
}

describe('WeatherSystem', () => {
  it('draws storm tint across the full canvas', () => {
    const weather = new WeatherSystem();
    const ctx = makeCtx();

    setState(weather, 'storm');
    weather.drawBehind(ctx, 400, 300);

    expect(ctx.createLinearGradient).toHaveBeenCalledWith(0, 0, 0, 300);
    expect(ctx.linearGradients[0].addColorStop).toHaveBeenNthCalledWith(1, 0, 'rgba(20,15,30,0.45)');
    expect(ctx.linearGradients[0].addColorStop).toHaveBeenNthCalledWith(2, 1, 'rgba(26,26,46,0.45)');
    expect(ctx.fillRect).toHaveBeenCalledWith(0, 0, 400, 300);
  });

  it('draws golden tint across the full canvas', () => {
    const weather = new WeatherSystem();
    const ctx = makeCtx();

    setState(weather, 'golden');
    weather.drawBehind(ctx, 640, 360);

    expect(ctx.createLinearGradient).toHaveBeenCalledWith(0, 0, 0, 360);
    expect(ctx.linearGradients[0].addColorStop).toHaveBeenNthCalledWith(1, 0, 'rgba(162,98,48,0.42)');
    expect(ctx.linearGradients[0].addColorStop).toHaveBeenNthCalledWith(2, 1, 'rgba(104,56,26,0.42)');
    expect(ctx.fillRect).toHaveBeenCalledWith(0, 0, 640, 360);
  });

  it('layers a warm wash and neutral veil during golden hour', () => {
    const weather = new WeatherSystem();
    const ctx = makeCtx();

    setState(weather, 'golden');
    weather.drawFront(ctx, 640, 360);

    expect(ctx.createLinearGradient).toHaveBeenCalledWith(0, 0, 0, 360);
    expect(ctx.linearGradients[0].addColorStop).toHaveBeenNthCalledWith(1, 0, 'rgba(255,210,140,0.12)');
    expect(ctx.linearGradients[0].addColorStop).toHaveBeenNthCalledWith(2, 0.55, 'rgba(224,168,112,0.08)');
    expect(ctx.linearGradients[0].addColorStop).toHaveBeenNthCalledWith(3, 1, 'rgba(140,92,58,0.06)');
    expect(ctx.createRadialGradient).toHaveBeenCalledWith(352, 50.400000000000006, 0, 352, 50.400000000000006, 448);
    expect(ctx.radialGradients[0].addColorStop).toHaveBeenNthCalledWith(1, 0, 'rgba(255,220,150,0.14)');
    expect(ctx.radialGradients[0].addColorStop).toHaveBeenNthCalledWith(2, 0.45, 'rgba(255,168,78,0.08)');
    expect(ctx.radialGradients[0].addColorStop).toHaveBeenNthCalledWith(3, 1, 'rgba(255,120,20,0)');
    expect(ctx.fillStyles[1]).toBe('rgba(190,170,155,0.06)');
    expect(ctx.fillRect).toHaveBeenCalledTimes(3);
    expect(ctx.fillRect).toHaveBeenNthCalledWith(1, 0, 0, 640, 360);
    expect(ctx.fillRect).toHaveBeenNthCalledWith(2, 0, 0, 640, 360);
    expect(ctx.fillRect).toHaveBeenNthCalledWith(3, 0, 0, 640, 360);
  });

  it('spawns a splash when rain hits the bottom of travel', () => {
    const weather = new WeatherSystem();
    setState(weather, 'storm');
    weather._maxRain = 1;
    weather._rain = [{
      x: 24,
      y: 95,
      len: 10,
      speed: 6,
      alpha: 0.2,
    }];

    weather.update(FRAME, 100, 100);

    expect(weather._rain).toHaveLength(1);
    expect(weather._rain[0].y).toBeLessThan(0);
    expect(weather._rainSplashes).toHaveLength(1);
    expect(weather._rainSplashes[0].x).toBeCloseTo(27.5, 5);
    expect(weather._rainSplashes[0].y).toBe(99);
    expect(weather._rainSplashes[0].age).toBeCloseTo(1, 5);
  });

  it('expires rain splashes after three frames', () => {
    const weather = new WeatherSystem();
    weather._rainSplashes = [{
      x: 10,
      y: 20,
      age: 0,
      maxAge: 3,
      spread: 2,
      height: 2,
      alpha: 0.2,
    }];

    weather.update(FRAME * 3, 100, 100);

    expect(weather._rainSplashes).toHaveLength(0);
  });

  it('initializes four fog clouds across layered parallax depths', () => {
    const weather = new WeatherSystem();

    expect(weather._fogClouds).toHaveLength(4);
    expect(new Set(weather._fogClouds.map((cloud) => cloud.layer))).toEqual(new Set([0, 1, 2]));
    expect(Math.max(...weather._fogClouds.map((cloud) => cloud.speed)))
      .toBeGreaterThan(Math.min(...weather._fogClouds.map((cloud) => cloud.speed)));
    expect(Math.min(...weather._fogClouds.map((cloud) => cloud.alpha))).toBeGreaterThanOrEqual(0.14);
  });

  it('moves nearer fog clouds faster than distant ones', () => {
    const weather = new WeatherSystem();
    weather.time = 12;

    const slowCloud = {
      layer: 0,
      startX: 0.2,
      y: 0.3,
      width: 0.24,
      height: 0.12,
      speed: 0.015,
      bobAmplitude: 0,
      bobSpeed: 0,
      phase: 0,
      alpha: 0.16,
    };
    const fastCloud = {
      ...slowCloud,
      layer: 2,
      speed: 0.045,
    };

    const slowFrame = weather._getFogCloudFrame(slowCloud, 1000, 600);
    const fastFrame = weather._getFogCloudFrame(fastCloud, 1000, 600);

    expect(fastFrame.x).toBeGreaterThan(slowFrame.x);
    expect(fastFrame.y).toBe(slowFrame.y);
  });

  it('renders an ellipse for each fog cloud while fog is active', () => {
    const weather = new WeatherSystem();
    const ctx = makeCtx();

    setState(weather, 'fog');
    weather.time = 5;
    weather._fogClouds = [
      {
        layer: 0,
        startX: 0.2,
        y: 0.28,
        width: 0.24,
        height: 0.12,
        speed: 0.02,
        bobAmplitude: 0,
        bobSpeed: 0,
        phase: 0,
        alpha: 0.18,
      },
      {
        layer: 2,
        startX: 0.45,
        y: 0.46,
        width: 0.32,
        height: 0.16,
        speed: 0.04,
        bobAmplitude: 0,
        bobSpeed: 0,
        phase: 0,
        alpha: 0.22,
      },
    ];

    weather.drawFront(ctx, 800, 600);

    expect(ctx.save).toHaveBeenCalledTimes(1);
    expect(ctx.restore).toHaveBeenCalledTimes(1);
    expect(ctx.beginPath).toHaveBeenCalledTimes(2);
    expect(ctx.ellipse).toHaveBeenCalledTimes(2);
    expect(ctx.fill).toHaveBeenCalledTimes(2);

    const firstCloud = ctx.ellipse.mock.calls[0];
    const secondCloud = ctx.ellipse.mock.calls[1];

    expect(firstCloud[0]).toBeCloseTo(48, 5);
    expect(firstCloud[1]).toBeCloseTo(168, 5);
    expect(firstCloud[2]).toBeCloseTo(96, 5);
    expect(firstCloud[3]).toBeCloseTo(36, 5);
    expect(firstCloud[4]).toBe(0);
    expect(firstCloud[5]).toBe(0);
    expect(firstCloud[6]).toBe(Math.PI * 2);

    expect(secondCloud[0]).toBeCloseTo(264, 5);
    expect(secondCloud[1]).toBeCloseTo(276, 5);
    expect(secondCloud[2]).toBeCloseTo(128, 5);
    expect(secondCloud[3]).toBeCloseTo(48, 5);
    expect(secondCloud[4]).toBe(0);
    expect(secondCloud[5]).toBe(0);
    expect(secondCloud[6]).toBe(Math.PI * 2);
  });

  describe('state classification', () => {
    it('zero sessions → clear', () => {
      const w = new WeatherSystem();
      evaluateWith(w, metrics());
      expect(w.targetState).toBe('clear');
    });

    it('all sessions complete → golden', () => {
      const w = new WeatherSystem();
      evaluateWith(w, metrics({ sessionCount: 3, allComplete: true }));
      expect(w.targetState).toBe('golden');
    });

    it('2+ errors → storm (overrides golden)', () => {
      const w = new WeatherSystem();
      evaluateWith(w, metrics({ sessionCount: 3, activeCount: 1, errorCount: 2 }));
      expect(w.targetState).toBe('storm');
    });

    it('2+ compactions → fog (when no errors)', () => {
      const w = new WeatherSystem();
      evaluateWith(w, metrics({ sessionCount: 2, activeCount: 2, compactionCount: 3 }));
      expect(w.targetState).toBe('fog');
    });

    it('high burn rate → haze', () => {
      const w = new WeatherSystem();
      evaluateWith(w, metrics({ sessionCount: 2, activeCount: 2, totalBurnRate: 5000 }));
      expect(w.targetState).toBe('haze');
    });

    it('3+ active sessions → cloudy', () => {
      const w = new WeatherSystem();
      evaluateWith(w, metrics({ sessionCount: 3, activeCount: 3, totalBurnRate: 1000 }));
      expect(w.targetState).toBe('cloudy');
    });

    it('1-2 active sessions → sunny', () => {
      const w = new WeatherSystem();
      evaluateWith(w, metrics({ sessionCount: 2, activeCount: 2, totalBurnRate: 100 }));
      expect(w.targetState).toBe('sunny');
    });

    it('single active session → sunny', () => {
      const w = new WeatherSystem();
      evaluateWith(w, metrics({ sessionCount: 1, activeCount: 1 }));
      expect(w.targetState).toBe('sunny');
    });
  });

  describe('state classification priority', () => {
    it('storm beats fog when both conditions met', () => {
      const w = new WeatherSystem();
      evaluateWith(w, metrics({ sessionCount: 4, activeCount: 2, errorCount: 3, totalBurnRate: 6000, compactionCount: 5 }));
      expect(w.targetState).toBe('storm');
    });

    it('fog beats haze when both conditions met', () => {
      const w = new WeatherSystem();
      evaluateWith(w, metrics({ sessionCount: 2, activeCount: 2, totalBurnRate: 6000, compactionCount: 3 }));
      expect(w.targetState).toBe('fog');
    });

    it('haze beats cloudy when both conditions met', () => {
      const w = new WeatherSystem();
      evaluateWith(w, metrics({ sessionCount: 4, activeCount: 4, totalBurnRate: 8000 }));
      expect(w.targetState).toBe('haze');
    });

    it('golden beats all non-error states', () => {
      const w = new WeatherSystem();
      evaluateWith(w, metrics({ sessionCount: 5, allComplete: true }));
      expect(w.targetState).toBe('golden');
    });

    it('1 error is not enough for storm (falls through to other checks)', () => {
      const w = new WeatherSystem();
      evaluateWith(w, metrics({ sessionCount: 3, activeCount: 3, errorCount: 1, totalBurnRate: 100 }));
      expect(w.targetState).toBe('cloudy');
    });
  });

  describe('updateMetrics', () => {
    it('counts active vs errored vs complete sessions', () => {
      const w = new WeatherSystem();
      w.updateMetrics([
        session({ activity: 'active' }),
        session({ activity: 'errored' }),
        session({ activity: 'complete' }),
        session({ activity: 'lost' }),
        session({ activity: 'active' }),
      ]);
      expect(w._metrics.sessionCount).toBe(5);
      expect(w._metrics.activeCount).toBe(2);
      expect(w._metrics.errorCount).toBe(2); // errored + lost
      expect(w._metrics.allComplete).toBe(false);
    });

    it('sets allComplete when all sessions are complete', () => {
      const w = new WeatherSystem();
      w.updateMetrics([
        session({ activity: 'complete' }),
        session({ activity: 'complete' }),
      ]);
      expect(w._metrics.allComplete).toBe(true);
      expect(w._metrics.activeCount).toBe(0);
    });

    it('allComplete is false for empty session list', () => {
      const w = new WeatherSystem();
      w.updateMetrics([]);
      expect(w._metrics.allComplete).toBe(false);
      expect(w._metrics.sessionCount).toBe(0);
    });

    it('sums burn rates across sessions', () => {
      const w = new WeatherSystem();
      w.updateMetrics([
        session({ activity: 'active', burnRatePerMinute: 2000 }),
        session({ activity: 'active', burnRatePerMinute: 3500 }),
      ]);
      expect(w._metrics.totalBurnRate).toBe(5500);
    });

    it('sums compaction counts', () => {
      const w = new WeatherSystem();
      w.updateMetrics([
        session({ activity: 'active', compactionCount: 1 }),
        session({ activity: 'active', compactionCount: 3 }),
      ]);
      expect(w._metrics.compactionCount).toBe(4);
    });

    it('treats missing burnRatePerMinute as zero', () => {
      const w = new WeatherSystem();
      w.updateMetrics([{ activity: 'active' }]);
      expect(w._metrics.totalBurnRate).toBe(0);
    });

    it('errored sessions count as errors but not active', () => {
      const w = new WeatherSystem();
      w.updateMetrics([
        session({ activity: 'errored' }),
        session({ activity: 'errored' }),
      ]);
      expect(w._metrics.errorCount).toBe(2);
      expect(w._metrics.activeCount).toBe(0);
      // errored sessions don't reset allComplete (only non-terminal activities do)
      expect(w._metrics.allComplete).toBe(true);
    });
  });

  describe('transitions', () => {
    it('starts a transition when state changes', () => {
      const w = new WeatherSystem();
      expect(w.currentState).toBe('clear');
      expect(w.targetState).toBe('clear');
      expect(w.transitionProgress).toBe(1.0);

      evaluateWith(w, metrics({ sessionCount: 1, activeCount: 1 }));
      expect(w.currentState).toBe('clear');
      expect(w.targetState).toBe('sunny');
      expect(w.transitionProgress).toBe(0);
    });

    it('does not restart transition if target state unchanged', () => {
      const w = new WeatherSystem();
      const sunny = metrics({ sessionCount: 1, activeCount: 1 });
      evaluateWith(w, sunny);
      w.transitionProgress = 0.5; // mid-transition

      evaluateWith(w, sunny);
      expect(w.transitionProgress).toBe(0.5);
    });

    it('completes transition after TRANSITION_DURATION', () => {
      const w = new WeatherSystem();
      evaluateWith(w, metrics({ sessionCount: 1, activeCount: 1 }));
      expect(w.transitionProgress).toBe(0);

      // 2 seconds = TRANSITION_DURATION
      w.update(2.0, 800, 600);
      expect(w.transitionProgress).toBe(1.0);
      expect(w.currentState).toBe('sunny');
    });

    it('clamps transition progress at 1.0', () => {
      const w = new WeatherSystem();
      evaluateWith(w, metrics({ sessionCount: 1, activeCount: 1 }));

      w.update(5.0, 800, 600); // well past duration
      expect(w.transitionProgress).toBe(1.0);
    });
  });

  describe('_getEffectWeight', () => {
    it('returns 1 when fully at target state', () => {
      const w = new WeatherSystem();
      setState(w, 'storm');
      expect(w._getEffectWeight('storm')).toBe(1);
    });

    it('returns 0 for inactive state', () => {
      const w = new WeatherSystem();
      setState(w, 'clear');
      expect(w._getEffectWeight('storm')).toBe(0);
    });

    it('blends during transition', () => {
      const w = new WeatherSystem();
      w.currentState = 'clear';
      w.targetState = 'storm';
      w.transitionProgress = 0.6;

      expect(w._getEffectWeight('clear')).toBeCloseTo(0.4);
      expect(w._getEffectWeight('storm')).toBeCloseTo(0.6);
      expect(w._getEffectWeight('sunny')).toBe(0);
    });

    it('sums to 1 when transitioning between two different states', () => {
      const w = new WeatherSystem();
      w.currentState = 'fog';
      w.targetState = 'haze';
      w.transitionProgress = 0.3;

      const total = w._getEffectWeight('fog') + w._getEffectWeight('haze');
      expect(total).toBeCloseTo(1.0);
    });
  });

  describe('lerp/lerpRGB (via sky gradient transitions)', () => {
    it('blends sky palettes during a transition', () => {
      const w = new WeatherSystem();
      w.currentState = 'clear';
      w.targetState = 'storm';
      w.transitionProgress = 0.5;

      const ctx = makeCtx();
      w.drawBehind(ctx, 400, 300);

      // clear top=[10,10,30], storm top=[20,15,30] → midpoint=[15,13,30]
      // clear alpha=0.35, storm alpha=0.45 → midpoint=0.4
      expect(ctx.linearGradients[0].addColorStop).toHaveBeenNthCalledWith(
        1, 0, 'rgba(15,13,30,0.4)',
      );
    });

    it('shows pure source palette at t=0', () => {
      const w = new WeatherSystem();
      w.currentState = 'sunny';
      w.targetState = 'cloudy';
      w.transitionProgress = 0;

      const ctx = makeCtx();
      w.drawBehind(ctx, 400, 300);

      // At t=0, should show sunny palette: top=[40,50,90], alpha=0.20
      expect(ctx.linearGradients[0].addColorStop).toHaveBeenNthCalledWith(
        1, 0, 'rgba(40,50,90,0.2)',
      );
    });

    it('shows pure target palette at t=1', () => {
      const w = new WeatherSystem();
      w.currentState = 'sunny';
      w.targetState = 'cloudy';
      w.transitionProgress = 1;

      const ctx = makeCtx();
      w.drawBehind(ctx, 400, 300);

      // At t=1, should show cloudy palette: top=[50,50,60], alpha=0.30
      expect(ctx.linearGradients[0].addColorStop).toHaveBeenNthCalledWith(
        1, 0, 'rgba(50,50,60,0.3)',
      );
    });
  });

  describe('generateStars (via constructor)', () => {
    it('creates 80 stars', () => {
      const w = new WeatherSystem();
      expect(w._stars).toHaveLength(80);
    });

    it('all stars have valid position, size, phase, and speed', () => {
      const w = new WeatherSystem();
      for (let i = 0; i < w._stars.length; i++) {
        const star = w._stars[i];
        expect(star.x).toBeGreaterThanOrEqual(0);
        expect(star.x).toBeLessThan(1);
        expect(star.y).toBeGreaterThanOrEqual(0);
        expect(star.y).toBeLessThan(0.4);
        expect(star.size).toBeGreaterThanOrEqual(0.5);
        expect(star.size).toBeLessThan(2.0);
        expect(star.phase).toBeGreaterThanOrEqual(0);
        expect(star.phase).toBeLessThan(Math.PI * 2);
        expect(star.speed).toBeGreaterThanOrEqual(0.8);
        expect(star.speed).toBeLessThan(2.3);
      }
    });
  });

  describe('generateBolt (via lightning)', () => {
    it('lightning bolt has segments with valid coordinates', () => {
      const w = new WeatherSystem();
      setState(w, 'storm');
      w._metrics.errorCount = 5;
      w._lightningCooldown = 0;

      // Force lightning by making Math.random always return 0.001
      // (below the 0.005 threshold for lightning trigger)
      const origRandom = Math.random;
      Math.random = () => 0.001;

      try {
        w.update(FRAME, 800, 600);
      } finally {
        Math.random = origRandom;
      }

      expect(w._lightning).not.toBeNull();
      expect(w._lightning.segments.length).toBeGreaterThan(0);
      expect(w._lightning.flashAlpha).toBeGreaterThan(0);
      expect(w._lightning.timer).toBeGreaterThan(0);

      for (let i = 0; i < w._lightning.segments.length; i++) {
        const seg = w._lightning.segments[i];
        expect(seg).toHaveProperty('x1');
        expect(seg).toHaveProperty('y1');
        expect(seg).toHaveProperty('x2');
        expect(seg).toHaveProperty('y2');
        expect(seg).toHaveProperty('alpha');
        expect(seg).toHaveProperty('width');
        expect(seg.alpha).toBeGreaterThan(0);
        expect(seg.alpha).toBeLessThanOrEqual(1);
        expect(seg.width).toBeGreaterThanOrEqual(1);
      }
    });

    it('lightning expires after its timer runs out', () => {
      const w = new WeatherSystem();
      setState(w, 'storm');
      w._lightning = { segments: [], flashAlpha: 0.25, timer: 0.1 };

      w.update(0.15, 800, 600);
      expect(w._lightning).toBeNull();
    });
  });

  describe('toggle', () => {
    it('toggles enabled state', () => {
      const w = new WeatherSystem();
      expect(w.enabled).toBe(true);
      expect(w.toggle()).toBe(false);
      expect(w.enabled).toBe(false);
      expect(w.toggle()).toBe(true);
      expect(w.enabled).toBe(true);
    });

    it('skips update when disabled', () => {
      const w = new WeatherSystem();
      w.toggle();
      w.update(1.0, 800, 600);
      expect(w.time).toBe(0); // time not advanced
    });

    it('skips drawBehind when disabled', () => {
      const w = new WeatherSystem();
      const ctx = makeCtx();
      w.toggle();
      w.drawBehind(ctx, 800, 600);
      expect(ctx.createLinearGradient).not.toHaveBeenCalled();
    });

    it('skips drawFront when disabled', () => {
      const w = new WeatherSystem();
      const ctx = makeCtx();
      setState(w, 'golden');
      w.toggle();
      w.drawFront(ctx, 800, 600);
      expect(ctx.createLinearGradient).not.toHaveBeenCalled();
    });
  });

  describe('getStateLabel', () => {
    it('returns human-readable labels for all states', () => {
      const w = new WeatherSystem();
      const expected = {
        clear: 'Clear',
        sunny: 'Sunny',
        cloudy: 'Overcast',
        storm: 'Storm',
        haze: 'Heat Haze',
        golden: 'Golden Hour',
        fog: 'Fog',
      };

      for (const [state, label] of Object.entries(expected)) {
        setState(w, state);
        expect(w.getStateLabel()).toBe(label);
      }
    });

    it('falls back to Clear for unknown state', () => {
      const w = new WeatherSystem();
      w.targetState = 'nonexistent';
      expect(w.getStateLabel()).toBe('Clear');
    });
  });
});
