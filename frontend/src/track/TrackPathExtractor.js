/**
 * TrackPathExtractor
 *
 * Takes a 2D tile grid (string[][]) and extracts an ordered centerline path.
 * Walks from the start-line tile through connected tiles using per-tile
 * connectivity rules, emitting centerline waypoints with cumulative arc length.
 *
 * Tile connectivity (which edges the track opens to):
 *   straight-h / chicane / start-line / finish-line / pit-entry / pit-exit : W + E
 *   straight-v                                                               : N + S
 *   curve-ne                                                                 : N + E
 *   curve-nw                                                                 : N + W
 *   curve-se                                                                 : S + E
 *   curve-sw                                                                 : S + W
 *
 * Waypoint generation:
 *   straight-like  : entry_mid → center → exit_mid  (3 pts, 1 internal)
 *   curve-*        : quarter-arc from entry_mid to exit_mid  (ARC_STEPS+1 pts)
 *   chicane        : cubic bezier S-curve from entry_mid to exit_mid  (BEZ_STEPS+1 pts)
 *
 * Returns: { waypoints: [{x, y, arcLength}], totalLength, isLoop }
 *          or null if no start-line tile found.
 */

const TILE_OPENINGS = {
  'straight-h':  ['W', 'E'],
  'straight-v':  ['N', 'S'],
  'curve-ne':    ['N', 'E'],
  'curve-nw':    ['N', 'W'],
  'curve-se':    ['S', 'E'],
  'curve-sw':    ['S', 'W'],
  'chicane':     ['W', 'E'],
  'start-line':  ['W', 'E'],
  'finish-line': ['W', 'E'],
  'pit-entry':   ['W', 'E'],
  'pit-exit':    ['W', 'E'],
};

const SCENERY = new Set(['grandstand', 'tree', 'barrier']);

// Direction → {row delta, col delta}
const DIR_DELTA = {
  N: { dr: -1, dc: 0 },
  S: { dr:  1, dc: 0 },
  E: { dr:  0, dc: 1 },
  W: { dr:  0, dc: -1 },
};

const OPPOSITE = { N: 'S', S: 'N', E: 'W', W: 'E' };

const ARC_STEPS = 8;   // points per quarter-arc curve
const BEZ_STEPS = 8;   // points per chicane bezier

// Midpoint of the named edge of tile (row, col) given tile size S
function edgeMid(row, col, dir, S) {
  switch (dir) {
    case 'N': return { x: col * S + S / 2, y: row * S };
    case 'S': return { x: col * S + S / 2, y: (row + 1) * S };
    case 'E': return { x: (col + 1) * S,   y: row * S + S / 2 };
    case 'W': return { x: col * S,          y: row * S + S / 2 };
    default:  return { x: col * S + S / 2, y: row * S + S / 2 };
  }
}

function tileCenter(row, col, S) {
  return { x: col * S + S / 2, y: row * S + S / 2 };
}

function ptDist(a, b) {
  const dx = b.x - a.x;
  const dy = b.y - a.y;
  return Math.sqrt(dx * dx + dy * dy);
}

// Arc center for a curve tile — the corner the path bends around.
// The corner is determined by the two openings (e.g., N+E → top-right corner).
function curveArcCenter(row, col, S, tileType) {
  switch (tileType) {
    case 'curve-ne': return { cx: (col + 1) * S, cy: row * S };        // top-right
    case 'curve-nw': return { cx: col * S,        cy: row * S };        // top-left
    case 'curve-se': return { cx: (col + 1) * S,  cy: (row + 1) * S }; // bottom-right
    case 'curve-sw': return { cx: col * S,         cy: (row + 1) * S }; // bottom-left
    default:         return { cx: col * S + S / 2, cy: row * S + S / 2 };
  }
}

// Generate quarter-arc waypoints from entryDir edge mid to exitDir edge mid.
// Includes both endpoints (pts[0] = entryMid, pts[last] = exitMid).
function arcWaypoints(row, col, S, entryDir, exitDir, tileType) {
  const { cx, cy } = curveArcCenter(row, col, S, tileType);
  const r = S / 2;

  const entry = edgeMid(row, col, entryDir, S);
  const exit_ = edgeMid(row, col, exitDir, S);

  const startAngle = Math.atan2(entry.y - cy, entry.x - cx);
  const endAngle   = Math.atan2(exit_.y - cy, exit_.x - cx);

  // Shortest sweep (quarter-arc = ±π/2)
  let sweep = endAngle - startAngle;
  while (sweep >  Math.PI) { sweep -= 2 * Math.PI; }
  while (sweep < -Math.PI) { sweep += 2 * Math.PI; }

  const pts = [];
  for (let i = 0; i <= ARC_STEPS; i++) {
    const a = startAngle + sweep * (i / ARC_STEPS);
    pts.push({ x: cx + r * Math.cos(a), y: cy + r * Math.sin(a) });
  }
  return pts;
}

// Generate cubic-bezier S-curve waypoints for a chicane tile.
// Includes both endpoints.
function chicaneWaypoints(row, col, S, entryDir) {
  const tx = col * S;
  const cy = row * S + S / 2;

  let p0x, p0y, p1x, p1y, p2x, p2y, p3x, p3y;
  if (entryDir === 'W') {
    // W → E: matches the TrackEditor visual bezier
    p0x = tx;           p0y = cy;
    p1x = tx + S * 0.3; p1y = cy - S * 0.3;
    p2x = tx + S * 0.7; p2y = cy + S * 0.3;
    p3x = tx + S;       p3y = cy;
  } else {
    // E → W: reversed control points
    p0x = tx + S;       p0y = cy;
    p1x = tx + S * 0.7; p1y = cy + S * 0.3;
    p2x = tx + S * 0.3; p2y = cy - S * 0.3;
    p3x = tx;           p3y = cy;
  }

  const pts = [];
  for (let i = 0; i <= BEZ_STEPS; i++) {
    const t  = i / BEZ_STEPS;
    const mt = 1 - t;
    const x = mt * mt * mt * p0x + 3 * mt * mt * t * p1x +
               3 * mt * t * t * p2x + t * t * t * p3x;
    const y = mt * mt * mt * p0y + 3 * mt * mt * t * p1y +
               3 * mt * t * t * p2y + t * t * t * p3y;
    pts.push({ x, y });
  }
  return pts;
}

// Waypoints for one tile traversal from entryDir to exitDir.
// Includes both edge-midpoint endpoints.
function tileWaypoints(row, col, S, entryDir, exitDir, tileType) {
  if (tileType.startsWith('curve-')) {
    return arcWaypoints(row, col, S, entryDir, exitDir, tileType);
  }
  if (tileType === 'chicane') {
    return chicaneWaypoints(row, col, S, entryDir);
  }
  // Straight-like: entry_mid → center → exit_mid
  const entry  = edgeMid(row, col, entryDir, S);
  const center = tileCenter(row, col, S);
  const exit_  = edgeMid(row, col, exitDir, S);
  return [entry, center, exit_];
}

// Append pts to waypoints, accumulating arc length.
// If skipFirst=true, skip pts[0] (it duplicates the previous last point).
function appendWaypoints(waypoints, pts, skipFirst) {
  const start = skipFirst ? 1 : 0;
  for (let i = start; i < pts.length; i++) {
    const prev = waypoints[waypoints.length - 1];
    const al = prev.arcLength + ptDist(prev, pts[i]);
    waypoints.push({ x: pts[i].x, y: pts[i].y, arcLength: al });
  }
}

/**
 * Extract the centerline path from a tile grid.
 *
 * Returns the main path plus an optional pitPath sub-path spanning from the
 * first pit-entry tile to the last pit-exit tile encountered during the walk.
 *
 * @param {string[][]} tiles   - 2D array of tile IDs (rows × cols)
 * @param {number}     tileSize - pixel size of one tile (default 32)
 * @returns {{ waypoints: {x:number, y:number, arcLength:number}[],
 *             totalLength: number,
 *             isLoop: boolean,
 *             pitPath: { waypoints: {x:number, y:number, arcLength:number}[],
 *                        totalLength: number } | null } | null}
 */
export function extractPath(tiles, tileSize) {
  const S = tileSize ?? 32;
  const h = tiles.length;
  const w = h > 0 ? tiles[0].length : 0;

  // Find the first start-line tile
  let startRow = -1;
  let startCol = -1;
  outer: for (let r = 0; r < h; r++) {
    for (let c = 0; c < w; c++) {
      if (tiles[r][c] === 'start-line') {
        startRow = r;
        startCol = c;
        break outer;
      }
    }
  }
  if (startRow === -1) return null;

  // Try each opening of start-line (W and E) as the initial travel direction
  const startOpenings = TILE_OPENINGS['start-line'];
  for (let oi = 0; oi < startOpenings.length; oi++) {
    const result = walkFrom(tiles, startRow, startCol, startOpenings[oi], S, h, w);
    if (result) return result;
  }
  return null;
}

function walkFrom(tiles, startRow, startCol, initialDir, S, h, w) {
  // entryDir for start tile = the opening opposite to our travel direction
  const entryDir = OPPOSITE[initialDir];
  const exitDir  = initialDir;

  // Seed the waypoints with the start tile
  const startPts = tileWaypoints(startRow, startCol, S, entryDir, exitDir, 'start-line');
  const waypoints = [{ x: startPts[0].x, y: startPts[0].y, arcLength: 0 }];
  appendWaypoints(waypoints, startPts, true);

  let curRow    = startRow;
  let curCol    = startCol;
  let curExit   = exitDir; // direction we leave the current tile

  // Pit sub-path tracking: record waypoint indices for pit-entry/exit tiles.
  // pitEntryWpIdx = index of last waypoint BEFORE processing pit-entry (its entry edge mid).
  // pitExitWpIdx  = index of last waypoint AFTER  processing pit-exit  (its exit  edge mid).
  let pitEntryWpIdx = -1;
  let pitExitWpIdx  = -1;

  const maxSteps = h * w + 4;
  for (let step = 0; step < maxSteps; step++) {
    const delta = DIR_DELTA[curExit];
    const nextRow = curRow + delta.dr;
    const nextCol = curCol + delta.dc;

    // Detect loop closure: returned to start tile
    if (nextRow === startRow && nextCol === startCol) {
      return {
        waypoints,
        totalLength: waypoints[waypoints.length - 1].arcLength,
        isLoop: true,
        pitPath: buildPitPath(waypoints, pitEntryWpIdx, pitExitWpIdx),
      };
    }

    // Bounds check
    if (nextRow < 0 || nextRow >= h || nextCol < 0 || nextCol >= w) break;

    const nextTile = tiles[nextRow][nextCol];
    if (!nextTile || SCENERY.has(nextTile)) break;

    const nextOpenings = TILE_OPENINGS[nextTile];
    if (!nextOpenings) break;

    // The neighbor must accept a connection from the direction we arrived
    const arriveFrom = OPPOSITE[curExit]; // e.g. curExit=E → arriveFrom=W
    if (nextOpenings[0] !== arriveFrom && nextOpenings[1] !== arriveFrom) break;

    // The exit of the next tile is the other opening
    const nextExit = nextOpenings[0] === arriveFrom ? nextOpenings[1] : nextOpenings[0];

    // Record pit-entry: the shared edge mid (current last waypoint) is the pit start.
    if (nextTile === 'pit-entry' && pitEntryWpIdx === -1) {
      pitEntryWpIdx = waypoints.length - 1;
    }

    // Generate and append waypoints for next tile (skip first: shared edge mid)
    const pts = tileWaypoints(nextRow, nextCol, S, arriveFrom, nextExit, nextTile);
    appendWaypoints(waypoints, pts, true);

    // Record pit-exit: after appending, last waypoint is the pit-exit edge mid.
    if (nextTile === 'pit-exit') {
      pitExitWpIdx = waypoints.length - 1;
    }

    curRow  = nextRow;
    curCol  = nextCol;
    curExit = nextExit;
  }

  // Non-loop path: return only if we visited more than just the start tile
  if (waypoints.length > startPts.length) {
    return {
      waypoints,
      totalLength: waypoints[waypoints.length - 1].arcLength,
      isLoop: false,
      pitPath: buildPitPath(waypoints, pitEntryWpIdx, pitExitWpIdx),
    };
  }
  return null;
}

/**
 * Slice the pit sub-path from the main waypoints array and re-base arc lengths
 * to start at 0.  Returns null when entry/exit indices are invalid.
 *
 * @param {{x:number, y:number, arcLength:number}[]} waypoints
 * @param {number} entryIdx - index of the pit-entry edge-mid in waypoints
 * @param {number} exitIdx  - index of the pit-exit  edge-mid in waypoints
 * @returns {{ waypoints: {x:number, y:number, arcLength:number}[],
 *             totalLength: number } | null}
 */
function buildPitPath(waypoints, entryIdx, exitIdx) {
  if (entryIdx < 0 || exitIdx < 0 || exitIdx <= entryIdx) return null;
  const sub = waypoints.slice(entryIdx, exitIdx + 1);
  const base = sub[0].arcLength;
  const rebased = [];
  for (let i = 0; i < sub.length; i++) {
    rebased.push({ x: sub[i].x, y: sub[i].y, arcLength: sub[i].arcLength - base });
  }
  return {
    waypoints: rebased,
    totalLength: rebased[rebased.length - 1].arcLength,
  };
}
