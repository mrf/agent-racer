import { describe, it, expect, beforeEach } from 'vitest';
import { Track } from './Track.js';

describe('Track', () => {
  const CANVAS_W = 1000;
  const CANVAS_H = 600;
  const LANE_COUNT = 3;

  let track;

  beforeEach(() => {
    track = new Track();
  });

  describe('getTrackBounds', () => {
    it('returns correct bounds for standard dimensions', () => {
      const bounds = track.getTrackBounds(CANVAS_W, CANVAS_H, LANE_COUNT);
      expect(bounds).toEqual({
        x: 200,
        y: 60,
        width: 740,
        height: 240,
        totalHeight: 340,
        laneHeight: 80,
      });
    });

    it('clamps laneCount to minimum of 1', () => {
      const bounds = track.getTrackBounds(800, 400, 0);
      expect(bounds.height).toBe(80);
    });

    it('clamps negative laneCount to 1', () => {
      const bounds = track.getTrackBounds(800, 400, -5);
      expect(bounds.height).toBe(80);
    });

    it('scales height linearly with lane count', () => {
      const b1 = track.getTrackBounds(CANVAS_W, CANVAS_H, 1);
      const b5 = track.getTrackBounds(CANVAS_W, CANVAS_H, 5);
      expect(b5.height).toBe(b1.height * 5);
    });

    it('width adjusts with canvas width', () => {
      const narrow = track.getTrackBounds(500, CANVAS_H, 2);
      const wide = track.getTrackBounds(1200, CANVAS_H, 2);
      expect(wide.width - narrow.width).toBe(700);
    });

    it('x and y are always padding values regardless of canvas size', () => {
      const b = track.getTrackBounds(2000, 1000, 10);
      expect(b.x).toBe(200);
      expect(b.y).toBe(60);
    });

    it('totalHeight includes top and bottom padding', () => {
      const b = track.getTrackBounds(CANVAS_W, CANVAS_H, 2);
      expect(b.totalHeight).toBe(b.height + 60 + 40);
    });
  });

  describe('getLaneY', () => {
    const defaultBounds = { y: 60, laneHeight: 80 };

    it('returns center of first lane (lane 0)', () => {
      expect(track.getLaneY(defaultBounds, 0)).toBe(100); // 60 + 0*80 + 40
    });

    it('returns center of second lane (lane 1)', () => {
      expect(track.getLaneY(defaultBounds, 1)).toBe(180); // 60 + 80 + 40
    });

    it('works with non-default laneHeight bounds (pit)', () => {
      const pitBounds = { y: 330, laneHeight: 50 };
      expect(track.getLaneY(pitBounds, 0)).toBe(355); // 330 + 0 + 25
      expect(track.getLaneY(pitBounds, 1)).toBe(405); // 330 + 50 + 25
    });

    it('works with parking lot laneHeight', () => {
      const lotBounds = { y: 500, laneHeight: 45 };
      expect(track.getLaneY(lotBounds, 2)).toBe(612.5); // 500 + 90 + 22.5
    });
  });

  describe('getPositionX', () => {
    const defaultBounds = { x: 200, width: 740 };

    it('returns left edge at utilization 0', () => {
      expect(track.getPositionX(defaultBounds, 0)).toBe(200);
    });

    it('returns right edge at utilization 1', () => {
      expect(track.getPositionX(defaultBounds, 1)).toBe(940);
    });

    it('returns midpoint at utilization 0.5', () => {
      expect(track.getPositionX(defaultBounds, 0.5)).toBe(570);
    });

    it('maps fractional utilization linearly', () => {
      const bounds = { x: 100, width: 1000 };
      expect(track.getPositionX(bounds, 0.25)).toBe(350);
      expect(track.getPositionX(bounds, 0.75)).toBe(850);
    });

    it('handles utilization beyond 1', () => {
      const bounds = { x: 200, width: 500 };
      expect(track.getPositionX(bounds, 1.5)).toBe(950);
    });
  });

  describe('getPitBounds', () => {
    it('positions pit below the track with correct gap', () => {
      const pit = track.getPitBounds(CANVAS_W, CANVAS_H, LANE_COUNT, 2);
      const trackBounds = track.getTrackBounds(CANVAS_W, CANVAS_H, LANE_COUNT);
      expect(pit.y).toBe(trackBounds.y + trackBounds.height + 30);
    });

    it('offsets x by PIT_PADDING_LEFT from track', () => {
      const pit = track.getPitBounds(CANVAS_W, CANVAS_H, LANE_COUNT, 2);
      const trackBounds = track.getTrackBounds(CANVAS_W, CANVAS_H, LANE_COUNT);
      expect(pit.x).toBe(trackBounds.x + 40);
    });

    it('width is track width minus PIT_PADDING_LEFT', () => {
      const pit = track.getPitBounds(CANVAS_W, CANVAS_H, LANE_COUNT, 2);
      const trackBounds = track.getTrackBounds(CANVAS_W, CANVAS_H, LANE_COUNT);
      expect(pit.width).toBe(trackBounds.width - 40);
    });

    it('uses PIT_LANE_HEIGHT of 50', () => {
      const pit = track.getPitBounds(CANVAS_W, CANVAS_H, LANE_COUNT, 2);
      expect(pit.laneHeight).toBe(50);
    });

    it('height scales with pit lane count', () => {
      const pit1 = track.getPitBounds(CANVAS_W, CANVAS_H, LANE_COUNT, 1);
      const pit3 = track.getPitBounds(CANVAS_W, CANVAS_H, LANE_COUNT, 3);
      expect(pit1.height).toBe(50);
      expect(pit3.height).toBe(150);
    });

    it('clamps pit lane count to minimum of 1', () => {
      const pit = track.getPitBounds(CANVAS_W, CANVAS_H, LANE_COUNT, 0);
      expect(pit.height).toBe(50);
    });

    it('pit position changes with active lane count', () => {
      const pit2 = track.getPitBounds(CANVAS_W, CANVAS_H, 2, 1);
      const pit4 = track.getPitBounds(CANVAS_W, CANVAS_H, 4, 1);
      expect(pit4.y).toBe(pit2.y + 160); // 2 extra lanes * 80
    });
  });

  describe('getParkingLotBounds', () => {
    it('returns null when parkingLotLaneCount is 0', () => {
      expect(track.getParkingLotBounds(CANVAS_W, CANVAS_H, LANE_COUNT, 2, 0)).toBeNull();
    });

    it('returns null when parkingLotLaneCount is negative', () => {
      expect(track.getParkingLotBounds(CANVAS_W, CANVAS_H, LANE_COUNT, 2, -1)).toBeNull();
    });

    it('positions below pit with PARKING_LOT_GAP', () => {
      const lot = track.getParkingLotBounds(CANVAS_W, CANVAS_H, LANE_COUNT, 2, 1);
      const trackBounds = track.getTrackBounds(CANVAS_W, CANVAS_H, LANE_COUNT);
      const pitHeight = track.getRequiredPitHeight(2);
      expect(lot.y).toBe(trackBounds.y + trackBounds.height + pitHeight + 20);
    });

    it('offsets x by PARKING_LOT_PADDING_LEFT', () => {
      const lot = track.getParkingLotBounds(CANVAS_W, CANVAS_H, LANE_COUNT, 2, 1);
      const trackBounds = track.getTrackBounds(CANVAS_W, CANVAS_H, LANE_COUNT);
      expect(lot.x).toBe(trackBounds.x + 40);
    });

    it('width is track width minus PARKING_LOT_PADDING_LEFT', () => {
      const lot = track.getParkingLotBounds(CANVAS_W, CANVAS_H, LANE_COUNT, 2, 1);
      const trackBounds = track.getTrackBounds(CANVAS_W, CANVAS_H, LANE_COUNT);
      expect(lot.width).toBe(trackBounds.width - 40);
    });

    it('uses PARKING_LOT_LANE_HEIGHT of 45', () => {
      const lot = track.getParkingLotBounds(CANVAS_W, CANVAS_H, LANE_COUNT, 2, 1);
      expect(lot.laneHeight).toBe(45);
    });

    it('height scales with parking lot lane count', () => {
      const lot2 = track.getParkingLotBounds(CANVAS_W, CANVAS_H, LANE_COUNT, 1, 2);
      const lot4 = track.getParkingLotBounds(CANVAS_W, CANVAS_H, LANE_COUNT, 1, 4);
      expect(lot2.height).toBe(90);
      expect(lot4.height).toBe(180);
    });
  });

  describe('getRequiredHeight', () => {
    it('accounts for track + padding with no pit or parking', () => {
      const h = track.getRequiredHeight(LANE_COUNT);
      expect(h).toBe(3 * 80 + 60 + 40 + 30 + 50 + 40); // track + pit(default 1 lane)
    });

    it('includes pit height based on pit lane count', () => {
      const h = track.getRequiredHeight(2, 3);
      const trackPart = 2 * 80 + 60 + 40;
      const pitPart = 30 + 3 * 50 + 40;
      expect(h).toBe(trackPart + pitPart);
    });

    it('includes parking lot height when provided', () => {
      const h = track.getRequiredHeight(2, 1, 2);
      const trackPart = 2 * 80 + 60 + 40;
      const pitPart = 30 + 1 * 50 + 40;
      const lotPart = 20 + 2 * 45 + 40;
      expect(h).toBe(trackPart + pitPart + lotPart);
    });

    it('returns 0 parking lot height for 0 parking lanes', () => {
      const h = track.getRequiredHeight(2, 1, 0);
      const trackPart = 2 * 80 + 60 + 40;
      const pitPart = 30 + 1 * 50 + 40;
      expect(h).toBe(trackPart + pitPart);
    });
  });

  describe('getRequiredPitHeight', () => {
    it('returns gap + lanes*height + bottom padding', () => {
      expect(track.getRequiredPitHeight(2)).toBe(30 + 100 + 40);
    });

    it('clamps to minimum 1 lane', () => {
      expect(track.getRequiredPitHeight(0)).toBe(30 + 50 + 40);
    });
  });

  describe('getRequiredParkingLotHeight', () => {
    it('returns 0 for 0 or negative lanes', () => {
      expect(track.getRequiredParkingLotHeight(0)).toBe(0);
      expect(track.getRequiredParkingLotHeight(-1)).toBe(0);
    });

    it('returns gap + lanes*height + bottom padding', () => {
      expect(track.getRequiredParkingLotHeight(3)).toBe(20 + 135 + 40);
    });
  });

  describe('getPitEntryX', () => {
    it('returns track x + PIT_ENTRY_OFFSET', () => {
      const trackBounds = track.getTrackBounds(CANVAS_W, CANVAS_H, LANE_COUNT);
      expect(track.getPitEntryX(trackBounds)).toBe(200 + 60);
    });
  });

  describe('cache invalidation (_needsPrerender)', () => {
    beforeEach(() => {
      track._lastWidth = 800;
      track._lastHeight = 600;
      track._lastLaneCount = 3;
    });

    it('needs prerender on fresh instance', () => {
      const freshTrack = new Track();
      expect(freshTrack._needsPrerender(800, 600, 3)).toBe(true);
    });

    it('does not need prerender when dimensions match', () => {
      expect(track._needsPrerender(800, 600, 3)).toBe(false);
    });

    it('needs prerender when width changes', () => {
      expect(track._needsPrerender(900, 600, 3)).toBe(true);
    });

    it('needs prerender when height changes', () => {
      expect(track._needsPrerender(800, 700, 3)).toBe(true);
    });

    it('needs prerender when lane count changes', () => {
      expect(track._needsPrerender(800, 600, 4)).toBe(true);
    });
  });

  describe('layout consistency', () => {
    it('getLaneY and getPositionX work together for positioning', () => {
      const bounds = track.getTrackBounds(CANVAS_W, CANVAS_H, 4);
      const x = track.getPositionX(bounds, 0.5);
      const y = track.getLaneY(bounds, 2);
      expect(x).toBe(200 + 370);
      expect(y).toBe(60 + 160 + 40);
    });

    it('pit getLaneY uses pit bounds correctly', () => {
      const pitBounds = track.getPitBounds(CANVAS_W, CANVAS_H, LANE_COUNT, 2);
      const y0 = track.getLaneY(pitBounds, 0);
      const y1 = track.getLaneY(pitBounds, 1);
      expect(y1 - y0).toBe(50); // PIT_LANE_HEIGHT
    });

    it('parking lot getLaneY uses parking bounds correctly', () => {
      const lotBounds = track.getParkingLotBounds(CANVAS_W, CANVAS_H, LANE_COUNT, 1, 2);
      const y0 = track.getLaneY(lotBounds, 0);
      const y1 = track.getLaneY(lotBounds, 1);
      expect(y1 - y0).toBe(45); // PARKING_LOT_LANE_HEIGHT
    });

    it('areas stack vertically without overlap', () => {
      const trackBounds = track.getTrackBounds(CANVAS_W, CANVAS_H, LANE_COUNT);
      const pitBounds = track.getPitBounds(CANVAS_W, CANVAS_H, LANE_COUNT, 2);
      const lotBounds = track.getParkingLotBounds(CANVAS_W, CANVAS_H, LANE_COUNT, 2, 2);

      const trackBottom = trackBounds.y + trackBounds.height;
      const pitBottom = pitBounds.y + pitBounds.height;

      expect(pitBounds.y).toBeGreaterThan(trackBottom);
      expect(lotBounds.y).toBeGreaterThan(pitBottom);
    });
  });
});
