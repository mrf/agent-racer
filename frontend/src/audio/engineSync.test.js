import { describe, it, expect, vi, beforeEach } from 'vitest';
import { syncEngineForEntity } from './engineSync.js';

function makeEngine() {
  return {
    startEngine: vi.fn(),
    stopEngine: vi.fn(),
  };
}

describe('syncEngineForEntity', () => {
  let engine;

  beforeEach(() => {
    engine = makeEngine();
  });

  describe('track zone', () => {
    it('starts engine with "thinking" when activity is thinking', () => {
      syncEngineForEntity(engine, 'r1', { activity: 'thinking', isChurning: false }, 'track');
      expect(engine.startEngine).toHaveBeenCalledWith('r1', 'thinking');
      expect(engine.stopEngine).not.toHaveBeenCalled();
    });

    it('starts engine with "tool_use" when activity is tool_use', () => {
      syncEngineForEntity(engine, 'r1', { activity: 'tool_use', isChurning: false }, 'track');
      expect(engine.startEngine).toHaveBeenCalledWith('r1', 'tool_use');
      expect(engine.stopEngine).not.toHaveBeenCalled();
    });

    it('starts engine with "churning" when idle and isChurning', () => {
      syncEngineForEntity(engine, 'r1', { activity: 'idle', isChurning: true }, 'track');
      expect(engine.startEngine).toHaveBeenCalledWith('r1', 'churning');
      expect(engine.stopEngine).not.toHaveBeenCalled();
    });

    it('starts engine with "churning" when starting and isChurning', () => {
      syncEngineForEntity(engine, 'r1', { activity: 'starting', isChurning: true }, 'track');
      expect(engine.startEngine).toHaveBeenCalledWith('r1', 'churning');
      expect(engine.stopEngine).not.toHaveBeenCalled();
    });

    it('stops engine when idle and NOT isChurning', () => {
      syncEngineForEntity(engine, 'r1', { activity: 'idle', isChurning: false }, 'track');
      expect(engine.stopEngine).toHaveBeenCalledWith('r1');
      expect(engine.startEngine).not.toHaveBeenCalled();
    });

    it('stops engine when activity is complete', () => {
      syncEngineForEntity(engine, 'r1', { activity: 'complete', isChurning: false }, 'track');
      expect(engine.stopEngine).toHaveBeenCalledWith('r1');
      expect(engine.startEngine).not.toHaveBeenCalled();
    });
  });

  describe('pit zone', () => {
    it('stops engine regardless of activity', () => {
      syncEngineForEntity(engine, 'r1', { activity: 'thinking', isChurning: false }, 'pit');
      expect(engine.stopEngine).toHaveBeenCalledWith('r1');
      expect(engine.startEngine).not.toHaveBeenCalled();
    });

    it('stops engine even when isChurning', () => {
      syncEngineForEntity(engine, 'r1', { activity: 'idle', isChurning: true }, 'pit');
      expect(engine.stopEngine).toHaveBeenCalledWith('r1');
      expect(engine.startEngine).not.toHaveBeenCalled();
    });
  });

  describe('parkingLot zone', () => {
    it('stops engine regardless of activity', () => {
      syncEngineForEntity(engine, 'r1', { activity: 'tool_use', isChurning: false }, 'parkingLot');
      expect(engine.stopEngine).toHaveBeenCalledWith('r1');
      expect(engine.startEngine).not.toHaveBeenCalled();
    });

    it('stops engine even when isChurning', () => {
      syncEngineForEntity(engine, 'r1', { activity: 'starting', isChurning: true }, 'parkingLot');
      expect(engine.stopEngine).toHaveBeenCalledWith('r1');
      expect(engine.startEngine).not.toHaveBeenCalled();
    });
  });

  describe('null engine guard', () => {
    it('does nothing when engine is null', () => {
      expect(() => {
        syncEngineForEntity(null, 'r1', { activity: 'thinking', isChurning: false }, 'track');
      }).not.toThrow();
    });

    it('does nothing when engine is undefined', () => {
      expect(() => {
        syncEngineForEntity(undefined, 'r1', { activity: 'thinking', isChurning: false }, 'track');
      }).not.toThrow();
    });
  });
});
