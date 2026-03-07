import { describe, it, expect } from 'vitest';
import { WeatherSystem } from './Weather.js';

const FRAME = 1 / 60;

describe('WeatherSystem', () => {
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
