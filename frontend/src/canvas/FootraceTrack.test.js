import { describe, it, expect, beforeEach, vi } from 'vitest';
import { FootraceTrack } from './FootraceTrack.js';

describe('FootraceTrack', () => {
  const CANVAS_W = 1000;
  const CANVAS_H = 600;
  const LANE_COUNT = 3;

  let track;
  let pad;

  beforeEach(() => {
    track = new FootraceTrack();
    pad = track.trackPadding;
  });

  describe('getTrackBounds', () => {
    it('returns correct bounds for standard dimensions', () => {
      const bounds = track.getTrackBounds(CANVAS_W, CANVAS_H, LANE_COUNT);
      const expectedWidth = CANVAS_W - pad.left - pad.right;
      expect(bounds).toEqual({
        x: pad.left,
        y: pad.top,
        width: expectedWidth,
        height: LANE_COUNT * track.laneHeight,
        totalHeight: LANE_COUNT * track.laneHeight + pad.top + pad.bottom,
        laneHeight: track.laneHeight,
      });
    });

    it('clamps laneCount to minimum of 1', () => {
      const bounds = track.getTrackBounds(800, 400, 0);
      expect(bounds.height).toBe(track.laneHeight);
    });

    it('clamps negative laneCount to 1', () => {
      const bounds = track.getTrackBounds(800, 400, -5);
      expect(bounds.height).toBe(track.laneHeight);
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

    it('x and y are always padding values', () => {
      const b = track.getTrackBounds(2000, 1000, 10);
      expect(b.x).toBe(pad.left);
      expect(b.y).toBe(pad.top);
    });

    it('totalHeight includes top and bottom padding', () => {
      const b = track.getTrackBounds(CANVAS_W, CANVAS_H, 2);
      expect(b.totalHeight).toBe(b.height + pad.top + pad.bottom);
    });
  });

  describe('getLaneY', () => {
    it('returns center of first lane', () => {
      const bounds = track.getTrackBounds(CANVAS_W, CANVAS_H, LANE_COUNT);
      expect(track.getLaneY(bounds, 0)).toBe(bounds.y + bounds.laneHeight / 2);
    });

    it('returns center of second lane', () => {
      const bounds = track.getTrackBounds(CANVAS_W, CANVAS_H, LANE_COUNT);
      expect(track.getLaneY(bounds, 1)).toBe(bounds.y + bounds.laneHeight + bounds.laneHeight / 2);
    });

    it('works with pit bounds', () => {
      const pitBounds = { y: 330, laneHeight: 50 };
      expect(track.getLaneY(pitBounds, 0)).toBe(355);
      expect(track.getLaneY(pitBounds, 1)).toBe(405);
    });

    it('works with parking lot bounds', () => {
      const lotBounds = { y: 500, laneHeight: 45 };
      expect(track.getLaneY(lotBounds, 2)).toBe(612.5);
    });
  });

  describe('getTokenX', () => {
    it('returns left edge when tokens is 0', () => {
      const bounds = { x: 100, width: 800 };
      expect(track.getTokenX(bounds, 0, 200000)).toBe(100);
    });

    it('returns right edge when tokens equals globalMaxTokens', () => {
      const bounds = { x: 100, width: 800 };
      expect(track.getTokenX(bounds, 200000, 200000)).toBe(900);
    });

    it('returns midpoint at half tokens', () => {
      const bounds = { x: 100, width: 800 };
      expect(track.getTokenX(bounds, 100000, 200000)).toBe(500);
    });

    it('returns left edge when globalMaxTokens is 0', () => {
      const bounds = { x: 100, width: 800 };
      expect(track.getTokenX(bounds, 50000, 0)).toBe(100);
    });

    it('returns left edge when globalMaxTokens is negative', () => {
      const bounds = { x: 100, width: 800 };
      expect(track.getTokenX(bounds, 50000, -1)).toBe(100);
    });

    it('clamps tokens beyond max to the finish line', () => {
      const bounds = { x: 100, width: 800 };
      expect(track.getTokenX(bounds, 250000, 200000)).toBe(900);
    });
  });

  describe('getPositionX', () => {
    it('returns left edge at utilization 0', () => {
      const bounds = track.getTrackBounds(CANVAS_W, CANVAS_H, LANE_COUNT);
      expect(track.getPositionX(bounds, 0)).toBe(bounds.x);
    });

    it('returns right edge at utilization 1', () => {
      const bounds = track.getTrackBounds(CANVAS_W, CANVAS_H, LANE_COUNT);
      expect(track.getPositionX(bounds, 1)).toBe(bounds.x + bounds.width);
    });

    it('returns midpoint at utilization 0.5', () => {
      const bounds = track.getTrackBounds(CANVAS_W, CANVAS_H, LANE_COUNT);
      expect(track.getPositionX(bounds, 0.5)).toBe(bounds.x + bounds.width / 2);
    });

    it('clamps utilization beyond 1 to the finish line', () => {
      const bounds = { x: 200, width: 500 };
      expect(track.getPositionX(bounds, 1.5)).toBe(700);
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

    it('returns collapsed bounds when pit lane count is 0', () => {
      const pit = track.getPitBounds(CANVAS_W, CANVAS_H, LANE_COUNT, 0);
      expect(pit.height).toBe(14);
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
      const trackPart = LANE_COUNT * track.laneHeight + pad.top + pad.bottom;
      expect(h).toBe(trackPart + 30 + 14 + 8);
    });

    it('includes pit height based on pit lane count', () => {
      const h = track.getRequiredHeight(2, 3);
      const trackPart = 2 * track.laneHeight + pad.top + pad.bottom;
      const pitPart = 30 + 3 * 50 + 40;
      expect(h).toBe(trackPart + pitPart);
    });

    it('includes parking lot height when provided', () => {
      const h = track.getRequiredHeight(2, 1, 2);
      const trackPart = 2 * track.laneHeight + pad.top + pad.bottom;
      const pitPart = 30 + 1 * 50 + 40;
      const lotPart = 20 + 2 * 45 + 40;
      expect(h).toBe(trackPart + pitPart + lotPart);
    });
  });

  describe('getRequiredPitHeight', () => {
    it('returns gap + lanes*height + bottom padding', () => {
      expect(track.getRequiredPitHeight(2)).toBe(30 + 100 + 40);
    });

    it('returns collapsed height for 0 lanes', () => {
      expect(track.getRequiredPitHeight(0)).toBe(30 + 14 + 8);
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
      expect(track.getPitEntryX(trackBounds)).toBe(trackBounds.x + 60);
    });
  });

  describe('getMultiTrackLayout', () => {
    it('returns single layout matching getTrackBounds for one group', () => {
      const groups = [{ maxTokens: 200000, laneCount: 3 }];
      const layouts = track.getMultiTrackLayout(CANVAS_W, groups);
      const bounds = track.getTrackBounds(CANVAS_W, CANVAS_H, 3);

      expect(layouts).toHaveLength(1);
      expect(layouts[0].x).toBe(bounds.x);
      expect(layouts[0].y).toBe(bounds.y);
      expect(layouts[0].width).toBe(bounds.width);
      expect(layouts[0].height).toBe(bounds.height);
    });

    it('stacks multiple groups vertically with gaps', () => {
      const groups = [
        { maxTokens: 200000, laneCount: 2 },
        { maxTokens: 1000000, laneCount: 1 },
      ];
      const layouts = track.getMultiTrackLayout(CANVAS_W, groups);

      expect(layouts).toHaveLength(2);
      expect(layouts[0].y).toBe(pad.top);
      expect(layouts[0].height).toBe(2 * track.laneHeight);
      expect(layouts[1].y).toBe(pad.top + 2 * track.laneHeight + 20 + 16);
    });

    it('preserves maxTokens and laneCount on layout objects', () => {
      const groups = [
        { maxTokens: 200000, laneCount: 2 },
        { maxTokens: 1000000, laneCount: 3 },
      ];
      const layouts = track.getMultiTrackLayout(CANVAS_W, groups);
      expect(layouts[0].maxTokens).toBe(200000);
      expect(layouts[0].laneCount).toBe(2);
      expect(layouts[1].maxTokens).toBe(1000000);
      expect(layouts[1].laneCount).toBe(3);
    });

    it('clamps laneCount to minimum of 1', () => {
      const groups = [{ maxTokens: 200000, laneCount: 0 }];
      const layouts = track.getMultiTrackLayout(CANVAS_W, groups);
      expect(layouts[0].height).toBe(track.laneHeight);
      expect(layouts[0].laneCount).toBe(1);
    });

    it('returns empty array for empty groups', () => {
      expect(track.getMultiTrackLayout(CANVAS_W, [])).toEqual([]);
    });
  });

  describe('updateViewport', () => {
    it('sets full crowd mode for tall viewports', () => {
      track.updateViewport(600);
      expect(track._crowdMode).toBe('full');
      expect(track.trackPadding.top).toBe(60);
    });

    it('sets compact crowd mode for medium viewports', () => {
      track.updateViewport(400);
      expect(track._crowdMode).toBe('compact');
      expect(track.trackPadding.top).toBe(40);
    });

    it('sets hidden crowd mode for small viewports', () => {
      track.updateViewport(300);
      expect(track._crowdMode).toBe('hidden');
      expect(track.trackPadding.top).toBe(8);
    });

    it('clears spectator cache on mode change', () => {
      track._spectators = [{ fake: true }];
      track._crowdMode = 'full';
      track.updateViewport(300);
      expect(track._spectators).toBeNull();
    });

    it('preserves spectator cache when mode unchanged', () => {
      track._spectators = [{ fake: true }];
      track._crowdMode = 'full';
      track.updateViewport(600);
      expect(track._spectators).toEqual([{ fake: true }]);
    });
  });

  describe('drawMultiTrack', () => {
    it('skips the tree line when the crowd is hidden', () => {
      track._crowdMode = 'hidden';
      vi.spyOn(track, '_needsPrerender').mockReturnValue(false);
      vi.spyOn(track, '_drawTrackSurface').mockImplementation(() => {});
      vi.spyOn(track, '_drawLaneDividers').mockImplementation(() => {});
      vi.spyOn(track, '_drawStartLine').mockImplementation(() => {});
      vi.spyOn(track, '_drawFinishLine').mockImplementation(() => {});
      vi.spyOn(track, '_drawMileMarkers').mockImplementation(() => {});
      vi.spyOn(track, '_drawCrowd').mockImplementation(() => {});
      const treeSpy = vi.spyOn(track, '_drawTreeLine').mockImplementation(() => {});

      track.drawMultiTrack({}, CANVAS_W, CANVAS_H, [{ maxTokens: 200000, laneCount: LANE_COUNT }]);

      expect(treeSpy).not.toHaveBeenCalled();
    });
  });

  describe('_formatTokenLabel', () => {
    it('formats millions with one decimal', () => {
      expect(track._formatTokenLabel(1000000)).toBe('1.0M');
      expect(track._formatTokenLabel(1500000)).toBe('1.5M');
    });

    it('formats thousands as integers', () => {
      expect(track._formatTokenLabel(200000)).toBe('200K');
      expect(track._formatTokenLabel(50000)).toBe('50K');
    });

    it('returns raw number below 1000', () => {
      expect(track._formatTokenLabel(500)).toBe('500');
      expect(track._formatTokenLabel(0)).toBe('0');
    });
  });

  describe('_drawTreeLine', () => {
    it('draws triangle trunks and varies canopy radius deterministically', () => {
      const ctx = {
        beginPath: vi.fn(),
        moveTo: vi.fn(),
        lineTo: vi.fn(),
        closePath: vi.fn(),
        fill: vi.fn(),
        arc: vi.fn(),
        fillStyle: '',
        globalAlpha: 1,
      };

      track.time = 0;
      track._drawTreeLine(ctx, { x: 10, y: 100, width: 200 });

      expect(ctx.arc.mock.calls).toHaveLength(5);
      expect(ctx.arc.mock.calls.map(([, , radius]) => radius)).toEqual([3, 5, 7, 4, 6]);

      expect(ctx.moveTo).toHaveBeenNthCalledWith(1, 30, 84);
      expect(ctx.lineTo).toHaveBeenNthCalledWith(1, 28, 94);
      expect(ctx.lineTo).toHaveBeenNthCalledWith(2, 32, 94);
      expect(ctx.closePath).toHaveBeenCalledTimes(5);
      expect(ctx.fill).toHaveBeenCalledTimes(10);
    });
  });

  describe('layout consistency', () => {
    it('areas stack vertically without overlap', () => {
      const trackBounds = track.getTrackBounds(CANVAS_W, CANVAS_H, LANE_COUNT);
      const pitBounds = track.getPitBounds(CANVAS_W, CANVAS_H, LANE_COUNT, 2);
      const lotBounds = track.getParkingLotBounds(CANVAS_W, CANVAS_H, LANE_COUNT, 2, 2);

      const trackBottom = trackBounds.y + trackBounds.height;
      const pitBottom = pitBounds.y + pitBounds.height;

      expect(pitBounds.y).toBeGreaterThan(trackBottom);
      expect(lotBounds.y).toBeGreaterThan(pitBottom);
    });

    it('getLaneY and getTokenX work together for positioning', () => {
      const bounds = track.getTrackBounds(CANVAS_W, CANVAS_H, 4);
      const x = track.getPositionX(bounds, 0.5);
      const y = track.getLaneY(bounds, 2);
      expect(x).toBe(bounds.x + bounds.width / 2);
      expect(y).toBe(bounds.y + 2 * bounds.laneHeight + bounds.laneHeight / 2);
    });
  });

  describe('cache invalidation', () => {
    beforeEach(() => {
      track._lastWidth = 800;
      track._lastHeight = 600;
      track._lastLaneCount = 3;
    });

    it('needs prerender on fresh instance', () => {
      const freshTrack = new FootraceTrack();
      expect(freshTrack._needsPrerender(800, 600, 3)).toBe(true);
    });

    it('does not need prerender when dimensions match', () => {
      expect(track._needsPrerender(800, 600, 3)).toBe(false);
    });

    it('needs prerender when width changes', () => {
      expect(track._needsPrerender(900, 600, 3)).toBe(true);
    });
  });
});
