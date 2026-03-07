import { describe, expect, it, vi } from 'vitest';
import { WeatherSystem } from './Weather.js';

const FRAME = 1 / 60;

function makeCtx() {
  const gradient = { addColorStop: vi.fn() };
  return {
    gradient,
    createLinearGradient: vi.fn(() => gradient),
    fillRect: vi.fn(),
    save: vi.fn(),
    restore: vi.fn(),
    beginPath: vi.fn(),
    arc: vi.fn(),
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
});
