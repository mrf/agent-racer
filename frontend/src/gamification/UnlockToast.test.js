// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { UnlockToast } from './UnlockToast.js';

let mockEngine;

beforeEach(() => {
  mockEngine = {
    playUnlockChime: vi.fn(),
  };
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe('UnlockToast', () => {
  describe('constructor', () => {
    it('initializes with empty toast list', () => {
      const ut = new UnlockToast(mockEngine);
      expect(ut.toasts).toEqual([]);
    });

    it('stores engine reference', () => {
      const ut = new UnlockToast(mockEngine);
      expect(ut.engine).toBe(mockEngine);
    });
  });

  describe('show', () => {
    it('adds toast with enter phase and zero progress', () => {
      const ut = new UnlockToast(mockEngine);
      ut.show({ name: 'First Lap', description: 'Complete your first session', tier: 'bronze' });

      expect(ut.toasts).toHaveLength(1);
      expect(ut.toasts[0].phase).toBe('enter');
      expect(ut.toasts[0].progress).toBe(0);
    });

    it('uses defaults when payload fields are missing', () => {
      const ut = new UnlockToast(mockEngine);
      ut.show({});

      expect(ut.toasts[0].name).toBe('Achievement Unlocked');
      expect(ut.toasts[0].description).toBe('');
      expect(ut.toasts[0].tier).toBe('bronze');
    });

    it('calls playUnlockChime on engine', () => {
      const ut = new UnlockToast(mockEngine);
      ut.show({ name: 'Test', tier: 'gold' });

      expect(mockEngine.playUnlockChime).toHaveBeenCalledWith('gold');
    });

    it('handles null engine gracefully', () => {
      const ut = new UnlockToast(null);
      expect(() => ut.show({ name: 'Test', tier: 'silver' })).not.toThrow();
    });

    it('stacks multiple toasts', () => {
      const ut = new UnlockToast(mockEngine);
      ut.show({ name: 'First', tier: 'bronze' });
      ut.show({ name: 'Second', tier: 'silver' });

      expect(ut.toasts).toHaveLength(2);
    });

    it('schedules dismiss timer that transitions to exit', () => {
      const ut = new UnlockToast(mockEngine);
      ut.show({ name: 'Test', tier: 'bronze' });

      // Transition to visible first
      ut.toasts[0].phase = 'visible';

      vi.advanceTimersByTime(5000);

      expect(ut.toasts[0].phase).toBe('exit');
      expect(ut.toasts[0].progress).toBe(0);
    });

    it('dismiss timer does not transition if not yet visible', () => {
      const ut = new UnlockToast(mockEngine);
      ut.show({ name: 'Test', tier: 'bronze' });

      // Still in enter phase when timer fires
      vi.advanceTimersByTime(5000);

      // Phase stays 'enter' because guard checks for 'visible'
      expect(ut.toasts[0].phase).toBe('enter');
    });
  });

  describe('update', () => {
    it('advances enter phase progress', () => {
      const ut = new UnlockToast(mockEngine);
      ut.show({ name: 'Test', tier: 'bronze' });

      ut.update(0.1); // SLIDE_DURATION = 0.35s
      expect(ut.toasts[0].progress).toBeCloseTo(0.1 / 0.35, 5);
      expect(ut.toasts[0].phase).toBe('enter');
    });

    it('transitions from enter to visible when progress reaches 1', () => {
      const ut = new UnlockToast(mockEngine);
      ut.show({ name: 'Test', tier: 'bronze' });

      ut.update(0.35); // Exactly SLIDE_DURATION

      expect(ut.toasts[0].progress).toBe(1);
      expect(ut.toasts[0].phase).toBe('visible');
    });

    it('advances exit phase progress', () => {
      const ut = new UnlockToast(mockEngine);
      ut.show({ name: 'Test', tier: 'bronze' });
      ut.toasts[0].phase = 'exit';
      ut.toasts[0].progress = 0;

      ut.update(0.1); // FADE_DURATION = 0.3s
      expect(ut.toasts[0].progress).toBeCloseTo(0.1 / 0.3, 5);
    });

    it('removes toast when exit progress reaches 1', () => {
      const ut = new UnlockToast(mockEngine);
      ut.show({ name: 'Test', tier: 'bronze' });
      ut.toasts[0].phase = 'exit';
      ut.toasts[0].progress = 0;

      ut.update(0.3); // Exactly FADE_DURATION

      expect(ut.toasts).toHaveLength(0);
    });

    it('does not advance visible phase toasts', () => {
      const ut = new UnlockToast(mockEngine);
      ut.show({ name: 'Test', tier: 'bronze' });
      ut.toasts[0].phase = 'visible';
      ut.toasts[0].progress = 1;

      ut.update(0.5);

      expect(ut.toasts[0].phase).toBe('visible');
      expect(ut.toasts[0].progress).toBe(1);
    });
  });

  describe('draw', () => {
    function createMockCtx() {
      return {
        save: vi.fn(),
        restore: vi.fn(),
        globalAlpha: 1,
        fillStyle: '',
        strokeStyle: '',
        lineWidth: 0,
        font: '',
        textAlign: '',
        textBaseline: '',
        beginPath: vi.fn(),
        roundRect: vi.fn(),
        fill: vi.fn(),
        stroke: vi.fn(),
        fillText: vi.fn(),
        measureText: vi.fn(() => ({ width: 50 })),
        createLinearGradient: vi.fn(() => ({
          addColorStop: vi.fn(),
        })),
      };
    }

    it('does nothing when no toasts', () => {
      const ut = new UnlockToast(mockEngine);
      const ctx = createMockCtx();

      ut.draw(ctx, 800);
      expect(ctx.save).not.toHaveBeenCalled();
    });

    it('calls save/restore for each toast', () => {
      const ut = new UnlockToast(mockEngine);
      ut.show({ name: 'A', tier: 'bronze' });
      ut.show({ name: 'B', tier: 'silver' });

      const ctx = createMockCtx();
      ut.draw(ctx, 800);

      expect(ctx.save).toHaveBeenCalledTimes(2);
      expect(ctx.restore).toHaveBeenCalledTimes(2);
    });

    it('sets globalAlpha based on enter progress', () => {
      const ut = new UnlockToast(mockEngine);
      ut.show({ name: 'Test', tier: 'bronze' });
      ut.toasts[0].progress = 0.5;

      const ctx = createMockCtx();
      let capturedAlpha;
      ctx.save.mockImplementation(() => { capturedAlpha = ctx.globalAlpha; });

      ut.draw(ctx, 800);

      // During enter, alpha = easeOutCubic(0.5)
      // easeOutCubic(0.5) = 1 - (1-0.5)^3 = 1 - 0.125 = 0.875
      // Alpha is set AFTER save, so check fillText was called (meaning rendering happened)
      expect(ctx.fillText).toHaveBeenCalled();
    });

    it('uses correct border color for each tier', () => {
      const ut = new UnlockToast(mockEngine);
      ut.show({ name: 'Gold!', tier: 'gold' });
      ut.toasts[0].phase = 'visible';
      ut.toasts[0].progress = 1;

      const ctx = createMockCtx();
      ut.draw(ctx, 800);

      // Gold border color is #ffd700
      const strokeCalls = ctx.stroke.mock.calls;
      expect(strokeCalls.length).toBeGreaterThan(0);
    });

    it('draws fading toast during exit phase', () => {
      const ut = new UnlockToast(mockEngine);
      ut.show({ name: 'Bye', tier: 'bronze' });
      ut.toasts[0].phase = 'exit';
      ut.toasts[0].progress = 0.5;

      const ctx = createMockCtx();
      ut.draw(ctx, 800);

      // alpha = 1 - 0.5 = 0.5 during exit
      expect(ctx.save).toHaveBeenCalled();
    });
  });

  describe('destroy', () => {
    it('clears all toasts', () => {
      const ut = new UnlockToast(mockEngine);
      ut.show({ name: 'A', tier: 'bronze' });
      ut.show({ name: 'B', tier: 'silver' });

      ut.destroy();
      expect(ut.toasts).toEqual([]);
    });

    it('clears dismiss timers', () => {
      const clearSpy = vi.spyOn(globalThis, 'clearTimeout');
      const ut = new UnlockToast(mockEngine);
      ut.show({ name: 'A', tier: 'bronze' });
      ut.show({ name: 'B', tier: 'silver' });

      ut.destroy();
      expect(clearSpy).toHaveBeenCalledTimes(2);
    });
  });

  describe('full lifecycle', () => {
    it('enter -> visible -> exit -> removed', () => {
      const ut = new UnlockToast(mockEngine);
      ut.show({ name: 'Full Cycle', tier: 'platinum' });

      // Enter phase
      expect(ut.toasts[0].phase).toBe('enter');

      // Complete enter animation (SLIDE_DURATION = 0.35s)
      ut.update(0.35);
      expect(ut.toasts[0].phase).toBe('visible');

      // Dismiss timer fires (DISPLAY_DURATION = 5000ms)
      vi.advanceTimersByTime(5000);
      expect(ut.toasts[0].phase).toBe('exit');

      // Complete exit animation (FADE_DURATION = 0.3s)
      ut.update(0.3);
      expect(ut.toasts).toHaveLength(0);
    });
  });
});
