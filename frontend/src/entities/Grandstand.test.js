import { describe, expect, it, vi } from 'vitest';
import { Grandstand } from './Grandstand.js';

function makeMockCtx() {
  return {
    createLinearGradient: vi.fn(() => ({ addColorStop: vi.fn() })),
    createRadialGradient: vi.fn(() => ({ addColorStop: vi.fn() })),
    fillRect: vi.fn(),
    beginPath: vi.fn(),
    arc: vi.fn(),
    fill: vi.fn(),
    moveTo: vi.fn(),
    lineTo: vi.fn(),
    stroke: vi.fn(),
    fillStyle: '',
    strokeStyle: '',
    lineWidth: 1,
  };
}

function makeSpectator(normX, totalWidth) {
  return {
    x: normX * totalWidth,
    row: 0,
    skinColor: '#f5d0a9',
    shirtColor: '#e94560',
    bodyW: 2,
    bodyH: 5,
    headR: 2,
    phase: 0,
    cheerThreshold: 1,
  };
}

describe('Grandstand', () => {
  it('keeps the Mexican wave visible across about nine percent of the stand width', () => {
    const trackWidth = 1000;
    const totalWidth = trackWidth + 20;
    const stand = new Grandstand();
    stand._time = 0;
    stand._mexicanPhase = 0.5;
    stand._reactions = [{ type: 'mexican', t: 0, duration: 5 }];
    stand._spectators = [
      makeSpectator(0.5, totalWidth),
      makeSpectator(0.58, totalWidth),
      makeSpectator(0.595, totalWidth),
    ];
    stand._drawSpec = vi.fn();

    stand._drawAllSpectators(makeMockCtx(), 0, 0, trackWidth, 1, 0);

    const bounceByNormX = new Map(
      stand._drawSpec.mock.calls.map(([, , , spec, , bounce]) => [spec.x / totalWidth, bounce]),
    );

    expect(bounceByNormX.get(0.5)).toBeCloseTo(5, 5);
    expect(bounceByNormX.get(0.58)).toBeGreaterThan(0.5);
    expect(bounceByNormX.get(0.595)).toBe(0);
  });
});
