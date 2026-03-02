import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { createSessionTracker } from './sessionTracker.js';

function mockEngine() {
  return {
    playAppear: vi.fn(),
    playDisappear: vi.fn(),
    playToolClick: vi.fn(),
    playGearShift: vi.fn(),
    stopEngine: vi.fn(),
  };
}

describe('createSessionTracker', () => {
  let engine;
  let tracker;

  beforeEach(() => {
    vi.useFakeTimers();
    engine = mockEngine();
    tracker = createSessionTracker(engine);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  describe('onSnapshot', () => {
    it('plays appear SFX for each new session', () => {
      tracker.onSnapshot([
        { id: 's1', activity: 'thinking' },
        { id: 's2', activity: 'tool_use' },
      ]);
      expect(engine.playAppear).toHaveBeenCalledTimes(2);
    });

    it('does not play appear SFX for sessions already known', () => {
      tracker.onSnapshot([{ id: 's1', activity: 'thinking' }]);
      engine.playAppear.mockClear();

      tracker.onSnapshot([{ id: 's1', activity: 'tool_use' }]);
      expect(engine.playAppear).not.toHaveBeenCalled();
    });

    it('plays disappear SFX for sessions removed between snapshots', () => {
      tracker.onSnapshot([
        { id: 's1', activity: 'thinking' },
        { id: 's2', activity: 'thinking' },
      ]);
      engine.playAppear.mockClear();

      tracker.onSnapshot([{ id: 's1', activity: 'thinking' }]);
      expect(engine.playDisappear).toHaveBeenCalledTimes(1);
    });

    it('plays both appear and disappear when sessions swap in one snapshot', () => {
      tracker.onSnapshot([{ id: 's1', activity: 'thinking' }]);
      engine.playAppear.mockClear();

      tracker.onSnapshot([{ id: 's2', activity: 'tool_use' }]);
      expect(engine.playDisappear).toHaveBeenCalledTimes(1);
      expect(engine.playAppear).toHaveBeenCalledTimes(1);
    });

    it('handles empty snapshot after populated one', () => {
      tracker.onSnapshot([
        { id: 's1', activity: 'thinking' },
        { id: 's2', activity: 'thinking' },
      ]);
      engine.playAppear.mockClear();

      tracker.onSnapshot([]);
      expect(engine.playDisappear).toHaveBeenCalledTimes(2);
    });

    it('handles empty snapshot when no sessions known', () => {
      tracker.onSnapshot([]);
      expect(engine.playAppear).not.toHaveBeenCalled();
      expect(engine.playDisappear).not.toHaveBeenCalled();
    });
  });

  describe('onDelta', () => {
    it('plays appear SFX for new sessions in updates', () => {
      tracker.onDelta([{ id: 's1', activity: 'thinking' }], null);
      expect(engine.playAppear).toHaveBeenCalledTimes(1);
    });

    it('does not play appear SFX for already-known sessions in updates', () => {
      tracker.onSnapshot([{ id: 's1', activity: 'thinking' }]);
      engine.playAppear.mockClear();

      tracker.onDelta([{ id: 's1', activity: 'tool_use' }], null);
      expect(engine.playAppear).not.toHaveBeenCalled();
    });

    it('plays disappear SFX and stops engine for each removed session', () => {
      tracker.onSnapshot([
        { id: 's1', activity: 'thinking' },
        { id: 's2', activity: 'thinking' },
      ]);

      tracker.onDelta(null, ['s1', 's2']);
      expect(engine.playDisappear).toHaveBeenCalledTimes(2);
      expect(engine.stopEngine).toHaveBeenCalledWith('s1');
      expect(engine.stopEngine).toHaveBeenCalledWith('s2');
    });

    it('cleans up internal state for removed sessions', () => {
      tracker.onSnapshot([{ id: 's1', activity: 'thinking' }]);
      tracker.onDelta(null, ['s1']);

      // Re-appearing after removal should trigger appear SFX again
      tracker.onDelta([{ id: 's1', activity: 'thinking' }], null);
      expect(engine.playAppear).toHaveBeenCalledTimes(2);
    });

    it('handles null updates gracefully', () => {
      tracker.onDelta(null, null);
      expect(engine.playAppear).not.toHaveBeenCalled();
      expect(engine.playDisappear).not.toHaveBeenCalled();
    });

    it('handles undefined updates and removed gracefully', () => {
      tracker.onDelta(undefined, undefined);
      expect(engine.playAppear).not.toHaveBeenCalled();
      expect(engine.playDisappear).not.toHaveBeenCalled();
    });
  });

  describe('transition SFX', () => {
    it('plays toolClick when transitioning to tool_use', () => {
      tracker.onSnapshot([{ id: 's1', activity: 'thinking' }]);
      tracker.onDelta([{ id: 's1', activity: 'tool_use' }], null);
      expect(engine.playToolClick).toHaveBeenCalledTimes(1);
    });

    it('plays gearShift when transitioning between two active activities', () => {
      tracker.onSnapshot([{ id: 's1', activity: 'tool_use' }]);
      tracker.onDelta([{ id: 's1', activity: 'thinking' }], null);
      expect(engine.playGearShift).toHaveBeenCalledTimes(1);
    });

    it('does not play gearShift when transitioning from inactive to active', () => {
      tracker.onSnapshot([{ id: 's1', activity: 'waiting' }]);
      tracker.onDelta([{ id: 's1', activity: 'thinking' }], null);
      expect(engine.playGearShift).not.toHaveBeenCalled();
    });

    it('does not play transition SFX when activity has not changed', () => {
      tracker.onSnapshot([{ id: 's1', activity: 'thinking' }]);
      tracker.onDelta([{ id: 's1', activity: 'thinking' }], null);
      expect(engine.playToolClick).not.toHaveBeenCalled();
      expect(engine.playGearShift).not.toHaveBeenCalled();
    });

    it('does not play transition SFX for brand-new sessions', () => {
      tracker.onDelta([{ id: 's1', activity: 'tool_use' }], null);
      expect(engine.playToolClick).not.toHaveBeenCalled();
      expect(engine.playGearShift).not.toHaveBeenCalled();
    });

    it('respects cooldown period between transition SFX', () => {
      tracker.onSnapshot([{ id: 's1', activity: 'thinking' }]);

      tracker.onDelta([{ id: 's1', activity: 'tool_use' }], null);
      expect(engine.playToolClick).toHaveBeenCalledTimes(1);

      // Transition again within the 3000ms cooldown
      tracker.onDelta([{ id: 's1', activity: 'thinking' }], null);
      tracker.onDelta([{ id: 's1', activity: 'tool_use' }], null);
      expect(engine.playToolClick).toHaveBeenCalledTimes(1);
    });

    it('allows transition SFX after cooldown expires', () => {
      tracker.onSnapshot([{ id: 's1', activity: 'thinking' }]);

      tracker.onDelta([{ id: 's1', activity: 'tool_use' }], null);
      expect(engine.playToolClick).toHaveBeenCalledTimes(1);

      vi.advanceTimersByTime(3001);

      // Transition back to thinking first (resets the cooldown timestamp)
      tracker.onDelta([{ id: 's1', activity: 'thinking' }], null);
      vi.advanceTimersByTime(3001);

      tracker.onDelta([{ id: 's1', activity: 'tool_use' }], null);
      expect(engine.playToolClick).toHaveBeenCalledTimes(2);
    });

    it('tracks cooldowns independently per session', () => {
      tracker.onSnapshot([
        { id: 's1', activity: 'thinking' },
        { id: 's2', activity: 'thinking' },
      ]);

      tracker.onDelta([{ id: 's1', activity: 'tool_use' }], null);
      expect(engine.playToolClick).toHaveBeenCalledTimes(1);

      tracker.onDelta([{ id: 's2', activity: 'tool_use' }], null);
      expect(engine.playToolClick).toHaveBeenCalledTimes(2);
    });

    it('plays both toolClick and gearShift on active-to-tool_use transition', () => {
      tracker.onSnapshot([{ id: 's1', activity: 'thinking' }]);
      tracker.onDelta([{ id: 's1', activity: 'tool_use' }], null);
      expect(engine.playToolClick).toHaveBeenCalledTimes(1);
      expect(engine.playGearShift).toHaveBeenCalledTimes(1);
    });
  });
});
