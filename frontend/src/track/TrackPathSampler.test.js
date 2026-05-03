import { describe, expect, it } from 'vitest';
import { TrackPathSampler } from './TrackPathSampler.js';

// Minimal straight-line path: 3 waypoints at x=0,50,100 along y=0
function straightPath(length = 100) {
  return {
    waypoints: [
      { x: 0,            y: 0, arcLength: 0 },
      { x: length / 2,  y: 0, arcLength: length / 2 },
      { x: length,      y: 0, arcLength: length },
    ],
    totalLength: length,
    isLoop: false,
  };
}

// Path travelling straight down (positive y in screen coords)
function verticalPath(length = 100) {
  return {
    waypoints: [
      { x: 0, y: 0,           arcLength: 0 },
      { x: 0, y: length / 2,  arcLength: length / 2 },
      { x: 0, y: length,      arcLength: length },
    ],
    totalLength: length,
    isLoop: false,
  };
}

describe('TrackPathSampler', () => {
  describe('edge cases', () => {
    it('returns origin for an empty path', () => {
      const sampler = new TrackPathSampler({ waypoints: [], totalLength: 0, isLoop: false });
      expect(sampler.sample(0.5)).toEqual({ x: 0, y: 0, angle: 0 });
    });

    it('returns the single point for a one-waypoint path', () => {
      const sampler = new TrackPathSampler({
        waypoints: [{ x: 10, y: 20, arcLength: 0 }],
        totalLength: 0,
        isLoop: false,
      });
      const result = sampler.sample(0.5);
      expect(result.x).toBeCloseTo(10);
      expect(result.y).toBeCloseTo(20);
    });

    it('clamps t below 0 to 0', () => {
      const sampler = new TrackPathSampler(straightPath());
      const at0 = sampler.sample(0);
      const atNeg = sampler.sample(-1);
      expect(atNeg.x).toBeCloseTo(at0.x);
      expect(atNeg.y).toBeCloseTo(at0.y);
    });

    it('clamps t above 1 to 1', () => {
      const sampler = new TrackPathSampler(straightPath());
      const at1 = sampler.sample(1);
      const atBig = sampler.sample(2);
      expect(atBig.x).toBeCloseTo(at1.x);
      expect(atBig.y).toBeCloseTo(at1.y);
    });
  });

  describe('position on centerline (no lane offset)', () => {
    it('returns start point at t=0', () => {
      const sampler = new TrackPathSampler(straightPath(100));
      const { x, y } = sampler.sample(0);
      expect(x).toBeCloseTo(0);
      expect(y).toBeCloseTo(0);
    });

    it('returns end point at t=1', () => {
      const sampler = new TrackPathSampler(straightPath(100));
      const { x, y } = sampler.sample(1);
      expect(x).toBeCloseTo(100);
      expect(y).toBeCloseTo(0);
    });

    it('returns midpoint at t=0.5', () => {
      const sampler = new TrackPathSampler(straightPath(100));
      const { x, y } = sampler.sample(0.5);
      expect(x).toBeCloseTo(50);
      expect(y).toBeCloseTo(0);
    });

    it('returns quarter-point at t=0.25', () => {
      const sampler = new TrackPathSampler(straightPath(100));
      const { x, y } = sampler.sample(0.25);
      expect(x).toBeCloseTo(25);
      expect(y).toBeCloseTo(0);
    });

    it('returns three-quarter-point at t=0.75', () => {
      const sampler = new TrackPathSampler(straightPath(100));
      const { x, y } = sampler.sample(0.75);
      expect(x).toBeCloseTo(75);
      expect(y).toBeCloseTo(0);
    });
  });

  describe('angle', () => {
    it('returns angle=0 for a rightward path', () => {
      const sampler = new TrackPathSampler(straightPath(100));
      const { angle } = sampler.sample(0.5);
      expect(angle).toBeCloseTo(0);
    });

    it('returns angle=PI/2 for a downward path (screen coords)', () => {
      const sampler = new TrackPathSampler(verticalPath(100));
      const { angle } = sampler.sample(0.5);
      expect(angle).toBeCloseTo(Math.PI / 2);
    });

    it('returns correct angle for diagonal path', () => {
      const diag = {
        waypoints: [
          { x: 0,  y: 0,  arcLength: 0 },
          { x: 50, y: 50, arcLength: Math.sqrt(50 * 50 + 50 * 50) },
        ],
        totalLength: Math.sqrt(50 * 50 + 50 * 50),
        isLoop: false,
      };
      const sampler = new TrackPathSampler(diag);
      const { angle } = sampler.sample(0.5);
      expect(angle).toBeCloseTo(Math.PI / 4);
    });
  });

  describe('lane offset', () => {
    it('no offset when laneCount=1', () => {
      const sampler = new TrackPathSampler(straightPath(100));
      const center = sampler.sample(0.5, 0, 1, 20);
      expect(center.x).toBeCloseTo(50);
      expect(center.y).toBeCloseTo(0);
    });

    it('no offset for center lane when laneCount=3 and laneIndex=1', () => {
      const sampler = new TrackPathSampler(straightPath(100));
      const center = sampler.sample(0.5, 1, 3, 20);
      expect(center.x).toBeCloseTo(50);
      expect(center.y).toBeCloseTo(0);
    });

    it('offsets perpendicular to tangent for a rightward path', () => {
      const sampler = new TrackPathSampler(straightPath(100));
      // Rightward path: angle=0, left-perp = (-sin0, cos0) = (0, 1) → positive y
      // laneIndex=0 in 3-lane stack: offset = (0 - 1) * 20 = -20
      // delta = (-sin(0)*-20, cos(0)*-20) = (0, -20)
      const lane0 = sampler.sample(0.5, 0, 3, 20);
      expect(lane0.x).toBeCloseTo(50);
      expect(lane0.y).toBeCloseTo(-20);

      // laneIndex=2: offset = (2-1)*20 = 20 → (0, 20)
      const lane2 = sampler.sample(0.5, 2, 3, 20);
      expect(lane2.x).toBeCloseTo(50);
      expect(lane2.y).toBeCloseTo(20);
    });

    it('offsets perpendicular to tangent for a downward path', () => {
      const sampler = new TrackPathSampler(verticalPath(100));
      // Downward path: angle=PI/2, left-perp = (-sin(PI/2), cos(PI/2)) = (-1, 0)
      // laneIndex=0 in 3-lane: offset = -20 → delta = (-1*-20, 0*-20) = (20, 0)
      const lane0 = sampler.sample(0.5, 0, 3, 20);
      expect(lane0.x).toBeCloseTo(20);
      expect(lane0.y).toBeCloseTo(50);

      // laneIndex=2: offset = 20 → delta = (-20, 0)
      const lane2 = sampler.sample(0.5, 2, 3, 20);
      expect(lane2.x).toBeCloseTo(-20);
      expect(lane2.y).toBeCloseTo(50);
    });

    it('symmetric lanes are equidistant from centerline', () => {
      const sampler = new TrackPathSampler(straightPath(100));
      const center = sampler.sample(0.5, 1, 3, 20);
      const lane0  = sampler.sample(0.5, 0, 3, 20);
      const lane2  = sampler.sample(0.5, 2, 3, 20);

      const d0 = Math.sqrt(
        (lane0.x - center.x) ** 2 + (lane0.y - center.y) ** 2
      );
      const d2 = Math.sqrt(
        (lane2.x - center.x) ** 2 + (lane2.y - center.y) ** 2
      );
      expect(d0).toBeCloseTo(20);
      expect(d2).toBeCloseTo(20);
    });

    it('two-lane stack: lanes are laneWidth apart', () => {
      const sampler = new TrackPathSampler(straightPath(100));
      const lane0 = sampler.sample(0.5, 0, 2, 30);
      const lane1 = sampler.sample(0.5, 1, 2, 30);
      const dist = Math.sqrt(
        (lane1.x - lane0.x) ** 2 + (lane1.y - lane0.y) ** 2
      );
      expect(dist).toBeCloseTo(30);
    });
  });

  describe('binary search on non-uniform arc lengths', () => {
    it('correctly finds segment with uneven spacing', () => {
      // Waypoints with non-uniform arc lengths
      const path = {
        waypoints: [
          { x: 0,   y: 0, arcLength: 0 },
          { x: 10,  y: 0, arcLength: 10 },
          { x: 40,  y: 0, arcLength: 40 },
          { x: 100, y: 0, arcLength: 100 },
        ],
        totalLength: 100,
        isLoop: false,
      };
      const sampler = new TrackPathSampler(path);

      // t=0.25 → target=25, in segment [10,40]
      const { x } = sampler.sample(0.25);
      expect(x).toBeCloseTo(25);

      // t=0.55 → target=55, in segment [40,100]
      const r2 = sampler.sample(0.55);
      expect(r2.x).toBeCloseTo(55);

      // t=0.05 → target=5, in segment [0,10]
      const r3 = sampler.sample(0.05);
      expect(r3.x).toBeCloseTo(5);
    });

    it('handles exact segment boundary (t at waypoint)', () => {
      const sampler = new TrackPathSampler(straightPath(100));
      const { x } = sampler.sample(0.5);
      expect(x).toBeCloseTo(50);
    });
  });
});
