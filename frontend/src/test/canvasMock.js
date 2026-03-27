import { vi } from 'vitest';

/**
 * Create a mock CanvasRenderingContext2D suitable for jsdom tests.
 *
 * Covers the drawing, path, transform, gradient, and text-measurement
 * APIs commonly used by canvas code.  Style properties are no-op setters
 * so assignments succeed without storing state.
 */
export function createMockCanvasContext() {
  return {
    clearRect: vi.fn(),
    fillRect: vi.fn(),
    strokeRect: vi.fn(),
    fillText: vi.fn(),
    beginPath: vi.fn(),
    moveTo: vi.fn(),
    lineTo: vi.fn(),
    arc: vi.fn(),
    fill: vi.fn(),
    stroke: vi.fn(),
    save: vi.fn(),
    restore: vi.fn(),
    scale: vi.fn(),
    translate: vi.fn(),
    drawImage: vi.fn(),
    roundRect: vi.fn(),
    rect: vi.fn(),
    closePath: vi.fn(),
    bezierCurveTo: vi.fn(),
    setLineDash: vi.fn(),
    createLinearGradient: vi.fn(() => ({ addColorStop: vi.fn() })),
    createRadialGradient: vi.fn(() => ({ addColorStop: vi.fn() })),
    measureText: vi.fn(() => ({ width: 0 })),
    set fillStyle(_) {},
    set strokeStyle(_) {},
    set lineWidth(_) {},
    set lineCap(_) {},
    set font(_) {},
    set textAlign(_) {},
    set textBaseline(_) {},
    set globalAlpha(_) {},
    set globalCompositeOperation(_) {},
  };
}

/**
 * Spy on HTMLCanvasElement.prototype.getContext so every `<canvas>`
 * created in the test gets the mock context.  Call `vi.restoreAllMocks()`
 * (or let a top-level afterEach do it) to undo the spy.
 */
export function installCanvasContextMock() {
  const ctx = createMockCanvasContext();
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(ctx);
  return ctx;
}
