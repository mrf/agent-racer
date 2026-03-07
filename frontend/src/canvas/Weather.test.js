import { describe, expect, it, vi } from 'vitest';
import { WeatherSystem } from './Weather.js';

const FRAME = 1 / 60;

function makeCtx() {
  const gradient = { addColorStop: vi.fn() };
  const radialGradient = { addColorStop: vi.fn() };
  return {
    gradient,
    radialGradient,
    createLinearGradient: vi.fn(() => gradient),
    createRadialGradient: vi.fn(() => radialGradient),
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
    set fillStyle(_) {},
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

describe('WeatherSystem', () => {
  it('draws storm tint across the full canvas', () => {
    const weather = new WeatherSystem();
    const ctx = makeCtx();

    setState(weather, 'storm');
    weather.drawBehind(ctx, 400, 300);

    expect(ctx.createLinearGradient).toHaveBeenCalledWith(0, 0, 0, 300);
    expect(ctx.gradient.addColorStop).toHaveBeenNthCalledWith(1, 0, 'rgba(20,15,30,0.45)');
    expect(ctx.gradient.addColorStop).toHaveBeenNthCalledWith(2, 1, 'rgba(26,26,46,0.45)');
    expect(ctx.fillRect).toHaveBeenCalledWith(0, 0, 400, 300);
  });

  it('draws golden tint across the full canvas', () => {
    const weather = new WeatherSystem();
    const ctx = makeCtx();

    setState(weather, 'golden');
    weather.drawBehind(ctx, 640, 360);

    expect(ctx.createLinearGradient).toHaveBeenCalledWith(0, 0, 0, 360);
    expect(ctx.gradient.addColorStop).toHaveBeenNthCalledWith(1, 0, 'rgba(80,50,20,0.3)');
    expect(ctx.gradient.addColorStop).toHaveBeenNthCalledWith(2, 1, 'rgba(50,30,15,0.3)');
    expect(ctx.fillRect).toHaveBeenCalledWith(0, 0, 640, 360);
  });

  it('spawns a splash when rain hits the bottom of travel', () => {
    const weather = new WeatherSystem();
    weather.currentState = 'storm';
    weather.targetState = 'storm';
    weather.transitionProgress = 1;
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
