// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

vi.mock('../auth.js', () => ({
  authFetch: vi.fn(),
}));

import { TrackEditor } from './TrackEditor.js';

function makeCtx() {
  return {
    fillRect: vi.fn(),
    strokeRect: vi.fn(),
  };
}

function makeCanvas(ctx, overrides = {}) {
  return {
    width: 1600,
    height: 1200,
    style: {},
    getContext: vi.fn(() => ctx),
    getBoundingClientRect: vi.fn(() => ({
      left: 100,
      top: 50,
      width: 800,
      height: 600,
    })),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    ...overrides,
  };
}

describe('TrackEditor', () => {
  beforeEach(() => {
    Object.defineProperty(window, 'devicePixelRatio', {
      value: 2,
      configurable: true,
      writable: true,
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('draws against logical canvas size instead of physical backing pixels', () => {
    const ctx = makeCtx();
    const canvas = makeCanvas(ctx);
    const editor = new TrackEditor(canvas);
    editor.active = true;
    editor.width = 1;
    editor.height = 1;
    editor.tiles = [['']];

    editor.draw();

    expect(ctx.fillRect.mock.calls[0]).toEqual([0, 0, 800, 600]);
  });

  it('maps pointer coordinates through the DPR-scaled canvas space before picking a cell', () => {
    const ctx = makeCtx();
    const canvas = makeCanvas(ctx, {
      getBoundingClientRect: vi.fn(() => ({
        left: 100,
        top: 50,
        width: 400,
        height: 300,
      })),
    });
    const editor = new TrackEditor(canvas);

    expect(editor._cellAt({ clientX: 116, clientY: 66 })).toEqual({ row: 1, col: 1 });
  });
});
