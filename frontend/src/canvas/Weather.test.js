import { describe, it, expect, vi } from 'vitest';
import { WeatherSystem } from './Weather.js';

function makeCtx() {
  return {
    save: vi.fn(),
    restore: vi.fn(),
    beginPath: vi.fn(),
    ellipse: vi.fn(),
    fill: vi.fn(),
    fillRect: vi.fn(),
    createRadialGradient: vi.fn(() => ({ addColorStop: vi.fn() })),
    set fillStyle(_) {},
    set globalAlpha(_) {},
    set filter(_) {},
  };
}

describe('WeatherSystem fog', () => {
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

    weather.currentState = 'fog';
    weather.targetState = 'fog';
    weather.transitionProgress = 1;
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
});
