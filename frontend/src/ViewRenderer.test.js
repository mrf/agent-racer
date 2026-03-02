// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('./canvas/RaceCanvas.js', () => ({
  RaceCanvas: vi.fn(() => ({
    setEngine: vi.fn(),
    racers: new Map(),
    entities: new Map(),
  })),
}));

vi.mock('./canvas/FootraceCanvas.js', () => ({
  FootraceCanvas: vi.fn(() => ({
    setEngine: vi.fn(),
    setAllRacers: vi.fn(),
    updateRacer: vi.fn(),
    removeRacer: vi.fn(),
    onComplete: vi.fn(),
    onError: vi.fn(),
    setConnected: vi.fn(),
    destroy: vi.fn(),
    entities: new Map(),
  })),
}));

let registerView, createView, getViewTypes;

beforeEach(async () => {
  vi.resetModules();
  const mod = await import('./ViewRenderer.js');
  registerView = mod.registerView;
  createView = mod.createView;
  getViewTypes = mod.getViewTypes;
});

describe('ViewRenderer factory', () => {
  it('registers and creates the built-in race view', () => {
    const canvas = document.createElement('canvas');
    const view = createView('race', canvas, null);
    expect(view).toBeDefined();
    expect(view.setEngine).toBeDefined();
  });

  it('throws on unknown view type', () => {
    const canvas = document.createElement('canvas');
    expect(() => createView('nonexistent', canvas, null)).toThrow('Unknown view type: nonexistent');
  });

  it('lists registered view types', () => {
    const types = getViewTypes();
    expect(types).toContain('race');
  });

  it('registers a custom view type', () => {
    const factory = vi.fn(() => ({ fake: true }));
    registerView('custom', factory);

    const canvas = document.createElement('canvas');
    const view = createView('custom', canvas, 'engine');
    expect(factory).toHaveBeenCalledWith(canvas, 'engine');
    expect(view).toEqual({ fake: true });
    expect(getViewTypes()).toContain('custom');
  });

  it('passes engine to setEngine for built-in race view', () => {
    const canvas = document.createElement('canvas');
    const mockEngine = { fake: true };
    const view = createView('race', canvas, mockEngine);
    expect(view.setEngine).toHaveBeenCalledWith(mockEngine);
  });

  it('skips setEngine when engine is null', () => {
    const canvas = document.createElement('canvas');
    const view = createView('race', canvas, null);
    expect(view.setEngine).not.toHaveBeenCalled();
  });

  it('registers and creates the built-in footrace view', () => {
    const canvas = document.createElement('canvas');
    const view = createView('footrace', canvas, null);
    expect(view).toBeDefined();
    expect(view.setEngine).toBeDefined();
    expect(getViewTypes()).toContain('footrace');
  });

  it('passes engine to setEngine for built-in footrace view', () => {
    const canvas = document.createElement('canvas');
    const mockEngine = { fake: true };
    const view = createView('footrace', canvas, mockEngine);
    expect(view.setEngine).toHaveBeenCalledWith(mockEngine);
  });
});
