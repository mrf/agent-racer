import { describe, it, expect, beforeEach } from 'vitest';
import { DraftDetector } from './DraftDetector.js';

function makeRacer(id, util) {
  return { id, state: { contextUtilization: util } };
}

describe('DraftDetector', () => {
  let detector;

  beforeEach(() => {
    detector = new DraftDetector();
  });

  describe('draft pairs', () => {
    it('returns no pairs when fewer than 2 racers', () => {
      const { draftPairs } = detector.detect([makeRacer('a', 0.5)], 0);
      expect(draftPairs).toHaveLength(0);
    });

    it('returns no pairs when gap exceeds 5%', () => {
      const racers = [makeRacer('a', 0.8), makeRacer('b', 0.7)];
      const { draftPairs } = detector.detect(racers, 0);
      expect(draftPairs).toHaveLength(0);
    });

    it('detects a draft pair within 5% gap', () => {
      const racers = [makeRacer('a', 0.80), makeRacer('b', 0.77)];
      const { draftPairs } = detector.detect(racers, 0);
      expect(draftPairs).toHaveLength(1);
      expect(draftPairs[0].leader.id).toBe('a');
      expect(draftPairs[0].drafter.id).toBe('b');
    });

    it('reports correct gap', () => {
      const racers = [makeRacer('a', 0.80), makeRacer('b', 0.77)];
      const { draftPairs } = detector.detect(racers, 0);
      expect(draftPairs[0].gap).toBeCloseTo(0.03, 5);
    });

    it('sorts racers by util before checking proximity', () => {
      // b has higher util than a, so b is leader and a is drafter
      const racers = [makeRacer('a', 0.50), makeRacer('b', 0.52)];
      const { draftPairs } = detector.detect(racers, 0);
      expect(draftPairs).toHaveLength(1);
      expect(draftPairs[0].leader.id).toBe('b');
      expect(draftPairs[0].drafter.id).toBe('a');
    });

    it('detects multiple draft pairs', () => {
      const racers = [
        makeRacer('a', 0.90),
        makeRacer('b', 0.88), // b within 5% of a
        makeRacer('c', 0.84), // c NOT within 5% of b (gap=0.04... wait 0.04 < 0.05 so yes)
        makeRacer('d', 0.70), // d NOT within 5% of c (gap=0.14)
      ];
      const { draftPairs } = detector.detect(racers, 0);
      // a-b: gap 0.02 ✓, b-c: gap 0.04 ✓, c-d: gap 0.14 ✗
      expect(draftPairs).toHaveLength(2);
    });
  });

  describe('overtake detection', () => {
    it('returns no overtakes on first call', () => {
      const racers = [makeRacer('a', 0.8), makeRacer('b', 0.6)];
      const { overtakes } = detector.detect(racers, 0);
      expect(overtakes).toHaveLength(0);
    });

    it('detects an overtake when b passes a', () => {
      // Poll 1: a leads, b follows
      detector.detect([makeRacer('a', 0.8), makeRacer('b', 0.6)], 0);
      // Poll 2: b now leads, a follows
      const { overtakes } = detector.detect([makeRacer('a', 0.5), makeRacer('b', 0.9)], 1);
      expect(overtakes).toHaveLength(1);
      expect(overtakes[0].overtaker.id).toBe('b');
      expect(overtakes[0].overtaken.id).toBe('a');
    });

    it('returns no overtake when order is unchanged', () => {
      detector.detect([makeRacer('a', 0.8), makeRacer('b', 0.6)], 0);
      const { overtakes } = detector.detect([makeRacer('a', 0.85), makeRacer('b', 0.65)], 1);
      expect(overtakes).toHaveLength(0);
    });

    it('handles new racers without false overtakes', () => {
      detector.detect([makeRacer('a', 0.8)], 0);
      // c is new — shouldn't count as overtake
      const { overtakes } = detector.detect([makeRacer('a', 0.8), makeRacer('c', 0.5)], 1);
      expect(overtakes).toHaveLength(0);
    });
  });

  describe('battle detection', () => {
    it('marks a battle when same pair swaps within BATTLE_WINDOW_S', () => {
      // First pass: b overtakes a
      detector.detect([makeRacer('a', 0.8), makeRacer('b', 0.6)], 0);
      detector.detect([makeRacer('a', 0.5), makeRacer('b', 0.9)], 1);
      // Second pass: a overtakes b back (within 10s)
      detector.detect([makeRacer('a', 0.95), makeRacer('b', 0.8)], 5);
      const { battles } = detector.detect([makeRacer('a', 0.95), makeRacer('b', 0.8)], 5);
      const key = ['a', 'b'].sort().join(':');
      expect(battles.has(key)).toBe(true);
    });

    it('does not mark battle when outside BATTLE_WINDOW_S', () => {
      detector.detect([makeRacer('a', 0.8), makeRacer('b', 0.6)], 0);
      // b overtakes a at t=1 — battle timer = 1
      detector.detect([makeRacer('a', 0.5), makeRacer('b', 0.9)], 1);
      // At t=12 no new overtake (b still leads) — battle should expire (12-1 > 10s window)
      const { battles } = detector.detect([makeRacer('a', 0.5), makeRacer('b', 0.9)], 12);
      const key = ['a', 'b'].sort().join(':');
      expect(battles.has(key)).toBe(false);
    });
  });
});
