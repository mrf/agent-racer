const SCENERY = new Set(['grandstand', 'tree', 'barrier']);

/**
 * Validates that track tiles form a connected layout with start+finish.
 * @param {string[][]} tiles
 * @returns {{ valid: boolean, message: string, disconnected: {row:number,col:number}[] }}
 */
export function validateTrack(tiles) {
  const h = tiles.length;
  const w = h > 0 ? tiles[0].length : 0;

  const trackCells = [];
  let hasStart = false;
  let hasFinish = false;

  for (let r = 0; r < h; r++) {
    for (let c = 0; c < w; c++) {
      const t = tiles[r][c];
      if (!t || SCENERY.has(t)) continue;
      trackCells.push({ row: r, col: c });
      if (t === 'start-line') hasStart = true;
      if (t === 'finish-line') hasFinish = true;
    }
  }

  if (trackCells.length === 0) {
    return { valid: false, message: 'No track tiles placed', disconnected: [] };
  }
  if (!hasStart) {
    return { valid: false, message: 'Missing start line', disconnected: [] };
  }
  if (!hasFinish) {
    return { valid: false, message: 'Missing finish line', disconnected: [] };
  }

  const cellSet = new Set();
  for (let i = 0; i < trackCells.length; i++) {
    cellSet.add(trackCells[i].row + ',' + trackCells[i].col);
  }

  const visited = new Set();
  const queue = [trackCells[0]];
  visited.add(trackCells[0].row + ',' + trackCells[0].col);

  while (queue.length > 0) {
    const cur = queue.shift();
    const dirs = [
      { row: cur.row - 1, col: cur.col },
      { row: cur.row + 1, col: cur.col },
      { row: cur.row, col: cur.col - 1 },
      { row: cur.row, col: cur.col + 1 },
    ];
    for (let i = 0; i < dirs.length; i++) {
      const n = dirs[i];
      const key = n.row + ',' + n.col;
      if (!visited.has(key) && cellSet.has(key)) {
        visited.add(key);
        queue.push(n);
      }
    }
  }

  const disconnected = [];
  for (let i = 0; i < trackCells.length; i++) {
    const c = trackCells[i];
    if (!visited.has(c.row + ',' + c.col)) {
      disconnected.push({ row: c.row, col: c.col });
    }
  }

  if (disconnected.length > 0) {
    return { valid: false, message: disconnected.length + ' disconnected tile(s)', disconnected };
  }

  return { valid: true, message: 'Track is valid', disconnected: [] };
}
