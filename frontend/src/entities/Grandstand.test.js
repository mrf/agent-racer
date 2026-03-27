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
  it('applies idle bounce from sine wave', () => {
    const trackWidth = 1000;
    const totalWidth = trackWidth + 20;
    const stand = new Grandstand();
    stand._time = 0;
    stand._reactions = [];
    stand._spectators = [makeSpectator(0.5, totalWidth)];
    stand._drawSpec = vi.fn();

    stand._drawAllSpectators(makeMockCtx(), 0, 0, trackWidth, 1, 0);

    const bounce = stand._drawSpec.mock.calls[0][5];
    // At _time=0 with phase=0, sin(0) = 0, so bounce ≈ 0
    expect(bounce).toBeCloseTo(0, 1);
  });

  it('falls back to default duration for unrecognized reaction types', () => {
    const stand = new Grandstand();
    stand.trigger('unknown');
    expect(stand._reactions.length).toBe(1);
    expect(stand._reactions[0].duration).toBe(1.0);
  });
});
