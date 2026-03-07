import { describe, expect, it, vi } from 'vitest';
import { WeatherSystem } from './Weather.js';

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
});
