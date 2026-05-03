import { describe, expect, it } from 'vitest';
import { extractPath } from './TrackPathExtractor.js';

const S = 32; // tile size used throughout tests

function makeGrid(h, w, fill = '') {
  const grid = [];
  for (let r = 0; r < h; r++) {
    const row = [];
    for (let c = 0; c < w; c++) {
      row.push(fill);
    }
    grid.push(row);
  }
  return grid;
}

// ── Helpers ──────────────────────────────────────────────────────────────────

function edgeMid(row, col, dir) {
  switch (dir) {
    case 'N': return { x: col * S + S / 2, y: row * S };
    case 'S': return { x: col * S + S / 2, y: (row + 1) * S };
    case 'E': return { x: (col + 1) * S,   y: row * S + S / 2 };
    case 'W': return { x: col * S,          y: row * S + S / 2 };
    default:  return { x: col * S + S / 2, y: row * S + S / 2 };
  }
}

function center(row, col) {
  return { x: col * S + S / 2, y: row * S + S / 2 };
}

function ptDist(a, b) {
  return Math.sqrt((b.x - a.x) ** 2 + (b.y - a.y) ** 2);
}

function totalArcLen(waypoints) {
  return waypoints[waypoints.length - 1].arcLength;
}

// ── Null / missing start ──────────────────────────────────────────────────────

describe('extractPath — missing start-line', () => {
  it('returns null for empty grid', () => {
    expect(extractPath([])).toBeNull();
  });

  it('returns null when no start-line tile exists', () => {
    const grid = makeGrid(2, 4);
    grid[0][0] = 'straight-h';
    grid[0][1] = 'straight-h';
    expect(extractPath(grid, S)).toBeNull();
  });

  it('returns null for a grid of only scenery', () => {
    const grid = [['grandstand', 'tree'], ['barrier', 'tree']];
    expect(extractPath(grid, S)).toBeNull();
  });
});

// ── Straight horizontal track (open-ended) ────────────────────────────────────

describe('extractPath — straight horizontal track', () => {
  //  [start-line][straight-h][straight-h][finish-line]
  const g = makeGrid(1, 4);
  g[0][0] = 'start-line';
  g[0][1] = 'straight-h';
  g[0][2] = 'straight-h';
  g[0][3] = 'finish-line';

  it('returns a result for straight-h track', () => {
    const res = extractPath(g, S);
    expect(res).not.toBeNull();
  });

  it('has strictly increasing arcLength', () => {
    const res = extractPath(g, S);
    const wps = res.waypoints;
    for (let i = 1; i < wps.length; i++) {
      expect(wps[i].arcLength).toBeGreaterThan(wps[i - 1].arcLength);
    }
  });

  it('first waypoint has arcLength 0', () => {
    const res = extractPath(g, S);
    expect(res.waypoints[0].arcLength).toBe(0);
  });

  it('path starts at the W edge of the start-line tile', () => {
    const res = extractPath(g, S);
    const first = res.waypoints[0];
    const expected = edgeMid(0, 0, 'W');
    expect(first.x).toBeCloseTo(expected.x);
    expect(first.y).toBeCloseTo(expected.y);
  });

  it('totalLength equals reported totalLength', () => {
    const res = extractPath(g, S);
    expect(res.totalLength).toBeCloseTo(totalArcLen(res.waypoints));
  });

  it('totalLength is approximately 4 * S (4 tiles wide)', () => {
    const res = extractPath(g, S);
    // path spans 4 tiles from W-edge of col 0 to E-edge of col 3 = 4*S
    expect(res.totalLength).toBeCloseTo(4 * S, 0);
  });
});

// ── Loop detection ────────────────────────────────────────────────────────────

describe('extractPath — loop track', () => {
  // Minimal rectangular loop (4×1 tiles):
  //   [start-line][straight-h][straight-h][straight-h]
  // Wrap not possible with 1 row — use a proper oval.
  //
  // 3×4 grid (rows × cols):
  //   row 0:  start-line  straight-h  straight-h  curve-sw
  //   row 1:  straight-v  (empty)     (empty)     straight-v
  //   row 2:  curve-ne    straight-h  straight-h  curve-nw
  //
  // Path (going E then S then W then N):
  //   start at (0,0) → E → (0,1) → E → (0,2) → E → (0,3)[curve-sw] → S
  //   → (1,3) → S → (2,3)[curve-nw] → W → (2,2) → W → (2,1) → W
  //   → (2,0)[curve-ne] → N → (1,0) → N → (0,0) [start] → LOOP ✓

  function buildOval() {
    const grid = makeGrid(3, 4);
    grid[0][0] = 'start-line';
    grid[0][1] = 'straight-h';
    grid[0][2] = 'straight-h';
    grid[0][3] = 'curve-sw';
    grid[1][0] = 'straight-v';
    grid[1][3] = 'straight-v';
    grid[2][0] = 'curve-ne';
    grid[2][1] = 'straight-h';
    grid[2][2] = 'straight-h';
    grid[2][3] = 'curve-nw';
    return grid;
  }

  it('detects a closed loop', () => {
    const res = extractPath(buildOval(), S);
    expect(res).not.toBeNull();
    expect(res.isLoop).toBe(true);
  });

  it('has strictly increasing arcLength', () => {
    const res = extractPath(buildOval(), S);
    const wps = res.waypoints;
    for (let i = 1; i < wps.length; i++) {
      expect(wps[i].arcLength).toBeGreaterThan(wps[i - 1].arcLength);
    }
  });

  it('first waypoint has arcLength 0', () => {
    const res = extractPath(buildOval(), S);
    expect(res.waypoints[0].arcLength).toBe(0);
  });

  it('totalLength is positive and plausible', () => {
    const res = extractPath(buildOval(), S);
    // Oval has perimeter > 2*(3+4)*S/2 ≈ some minimum
    expect(res.totalLength).toBeGreaterThan(S * 4);
  });
});

// ── Curve arc waypoints ────────────────────────────────────────────────────────

describe('extractPath — curve tiles produce arc waypoints', () => {
  // Single curve in a 3-tile L-path: straight-h → curve-sw → straight-v
  //   (0,0) straight-h  (0,1) curve-sw
  //                     (1,1) straight-v  ← dead end (only 3 tiles, no loop)
  // Actually we need start-line: replace (0,0) with start-line.
  //   (0,0) start-line, (0,1) curve-sw, (1,1) straight-v
  // Path: start E-edge → center(0,0) → (0,1) curve_sw entry=W exit=S → (1,1) v
  function buildLPath() {
    const grid = makeGrid(2, 2);
    grid[0][0] = 'start-line';
    grid[0][1] = 'curve-sw';
    grid[1][1] = 'straight-v';
    return grid;
  }

  it('returns a result for L-shaped path', () => {
    const res = extractPath(buildLPath(), S);
    expect(res).not.toBeNull();
  });

  it('curve produces more than 3 waypoints (arc sub-division)', () => {
    const res = extractPath(buildLPath(), S);
    expect(res).not.toBeNull();
    // Start tile (3 pts) + curve (8 internal) + straight-v (2 pts after shared) = 13
    expect(res.waypoints.length).toBeGreaterThan(5);
  });

  it('arc endpoints land on correct edge midpoints', () => {
    const res = extractPath(buildLPath(), S);
    // The curve (0,1) curve-sw opens S and W.
    // Entry from W (left edge), exit to S (bottom edge).
    const wps = res.waypoints;
    // Find the point closest to W edge mid of (0,1)
    const wEdge = edgeMid(0, 1, 'W');
    const sEdge = edgeMid(0, 1, 'S');
    const closeToW = wps.some(p => Math.abs(p.x - wEdge.x) < 0.5 && Math.abs(p.y - wEdge.y) < 0.5);
    const closeToS = wps.some(p => Math.abs(p.x - sEdge.x) < 0.5 && Math.abs(p.y - sEdge.y) < 0.5);
    expect(closeToW).toBe(true);
    expect(closeToS).toBe(true);
  });
});

// ── Chicane waypoints ─────────────────────────────────────────────────────────

describe('extractPath — chicane produces bezier waypoints', () => {
  // start-line (0,0) → chicane (0,1) → finish-line (0,2)
  function buildChicane() {
    const grid = makeGrid(1, 3);
    grid[0][0] = 'start-line';
    grid[0][1] = 'chicane';
    grid[0][2] = 'finish-line';
    return grid;
  }

  it('returns a result for chicane track', () => {
    expect(extractPath(buildChicane(), S)).not.toBeNull();
  });

  it('chicane produces more waypoints than a plain straight', () => {
    // Straight path: start + straight-h + finish
    const straight = makeGrid(1, 3);
    straight[0][0] = 'start-line';
    straight[0][1] = 'straight-h';
    straight[0][2] = 'finish-line';

    const chicane = buildChicane();
    const r1 = extractPath(straight, S);
    const r2 = extractPath(chicane, S);
    expect(r2.waypoints.length).toBeGreaterThan(r1.waypoints.length);
  });

  it('chicane starts and ends on the horizontal centerline', () => {
    const res = extractPath(buildChicane(), S);
    const wps = res.waypoints;
    // Entry and exit of chicane tile should be on the horizontal centerline
    const wEdge = edgeMid(0, 1, 'W');
    const eEdge = edgeMid(0, 1, 'E');
    const hasW = wps.some(p => Math.abs(p.x - wEdge.x) < 0.5 && Math.abs(p.y - wEdge.y) < 0.5);
    const hasE = wps.some(p => Math.abs(p.x - eEdge.x) < 0.5 && Math.abs(p.y - eEdge.y) < 0.5);
    expect(hasW).toBe(true);
    expect(hasE).toBe(true);
  });
});

// ── arcLength monotonicity across all tile types ───────────────────────────────

describe('extractPath — arcLength is always monotonically increasing', () => {
  const cases = [
    {
      label: 'horizontal straight line',
      build() {
        const g = makeGrid(1, 3);
        g[0][0] = 'start-line';
        g[0][1] = 'straight-h';
        g[0][2] = 'finish-line';
        return g;
      },
    },
    {
      // start-line opens W+E only, so a pure vertical path is invalid.
      // Use an L-shape instead: start-line → curve-sw → straight-v
      label: 'L-shape with straight-v segment',
      build() {
        const g = makeGrid(2, 2);
        g[0][0] = 'start-line';
        g[0][1] = 'curve-sw';
        g[1][1] = 'straight-v';
        return g;
      },
    },
    {
      label: 'single curve turn',
      build() {
        const g = makeGrid(2, 2);
        g[0][0] = 'start-line';
        g[0][1] = 'curve-sw';
        g[1][1] = 'finish-line';
        return g;
      },
    },
    {
      label: 'chicane',
      build() {
        const g = makeGrid(1, 3);
        g[0][0] = 'start-line';
        g[0][1] = 'chicane';
        g[0][2] = 'finish-line';
        return g;
      },
    },
  ];

  for (let i = 0; i < cases.length; i++) {
    const tc = cases[i];
    it(tc.label, () => {
      const res = extractPath(tc.build(), S);
      expect(res).not.toBeNull();
      const wps = res.waypoints;
      for (let j = 1; j < wps.length; j++) {
        expect(wps[j].arcLength).toBeGreaterThanOrEqual(wps[j - 1].arcLength);
      }
    });
  }
});

// ── Scenery tiles are ignored ─────────────────────────────────────────────────

describe('extractPath — scenery tiles do not break path', () => {
  it('grandstand next to track does not block extraction', () => {
    const g = makeGrid(2, 3);
    g[0][0] = 'start-line';
    g[0][1] = 'straight-h';
    g[0][2] = 'finish-line';
    g[1][0] = 'grandstand';
    g[1][1] = 'tree';
    g[1][2] = 'barrier';
    const res = extractPath(g, S);
    expect(res).not.toBeNull();
  });
});

// ── Pit-entry / pit-exit treated as straight ──────────────────────────────────

describe('extractPath — pit-entry and pit-exit tiles', () => {
  it('traverses pit-entry like a straight-h', () => {
    const g = makeGrid(1, 3);
    g[0][0] = 'start-line';
    g[0][1] = 'pit-entry';
    g[0][2] = 'finish-line';
    const res = extractPath(g, S);
    expect(res).not.toBeNull();
    // Center of pit-entry tile should appear in waypoints
    const c = center(0, 1);
    const found = res.waypoints.some(p => Math.abs(p.x - c.x) < 0.5 && Math.abs(p.y - c.y) < 0.5);
    expect(found).toBe(true);
  });
});

// ── Default tile size ─────────────────────────────────────────────────────────

describe('extractPath — default tileSize = 32', () => {
  it('uses 32 px tile size when tileSize is omitted', () => {
    const g = makeGrid(1, 2);
    g[0][0] = 'start-line';
    g[0][1] = 'finish-line';
    const res = extractPath(g);
    expect(res).not.toBeNull();
    // W edge of (0,0) with S=32 → x=0, y=16
    expect(res.waypoints[0].x).toBeCloseTo(0);
    expect(res.waypoints[0].y).toBeCloseTo(16);
  });
});
