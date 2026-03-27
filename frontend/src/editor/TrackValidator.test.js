import { describe, expect, it } from 'vitest';
import { validateTrack } from './TrackValidator.js';

/** Helper: create an h x w grid filled with a value. */
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

describe('TrackValidator', () => {
  describe('empty / no-track grids', () => {
    it('rejects an empty 0x0 grid', () => {
      const result = validateTrack([]);
      expect(result.valid).toBe(false);
      expect(result.message).toBe('No track tiles placed');
      expect(result.disconnected).toEqual([]);
    });

    it('rejects a grid of all empty strings', () => {
      const grid = makeGrid(3, 3);
      const result = validateTrack(grid);
      expect(result.valid).toBe(false);
      expect(result.message).toBe('No track tiles placed');
    });

    it('rejects a grid containing only scenery tiles', () => {
      const grid = [
        ['grandstand', 'tree', 'barrier'],
        ['tree', 'barrier', 'grandstand'],
      ];
      const result = validateTrack(grid);
      expect(result.valid).toBe(false);
      expect(result.message).toBe('No track tiles placed');
    });
  });

  describe('missing start / finish', () => {
    it('reports missing start line', () => {
      const grid = makeGrid(3, 3);
      grid[1][0] = 'straight-h';
      grid[1][1] = 'straight-h';
      grid[1][2] = 'finish-line';
      const result = validateTrack(grid);
      expect(result.valid).toBe(false);
      expect(result.message).toBe('Missing start line');
    });

    it('reports missing finish line', () => {
      const grid = makeGrid(3, 3);
      grid[1][0] = 'start-line';
      grid[1][1] = 'straight-h';
      grid[1][2] = 'straight-h';
      const result = validateTrack(grid);
      expect(result.valid).toBe(false);
      expect(result.message).toBe('Missing finish line');
    });

    it('checks start before finish (start missing takes priority)', () => {
      // No start or finish — should report start first
      const grid = makeGrid(2, 2, 'straight-h');
      const result = validateTrack(grid);
      expect(result.valid).toBe(false);
      expect(result.message).toBe('Missing start line');
    });
  });

  describe('valid connected tracks', () => {
    it('accepts a simple horizontal track', () => {
      const grid = makeGrid(1, 4);
      grid[0][0] = 'start-line';
      grid[0][1] = 'straight-h';
      grid[0][2] = 'straight-h';
      grid[0][3] = 'finish-line';
      const result = validateTrack(grid);
      expect(result.valid).toBe(true);
      expect(result.message).toBe('Track is valid');
      expect(result.disconnected).toEqual([]);
    });

    it('accepts a vertical track', () => {
      const grid = makeGrid(4, 1);
      grid[0][0] = 'start-line';
      grid[1][0] = 'straight-v';
      grid[2][0] = 'straight-v';
      grid[3][0] = 'finish-line';
      const result = validateTrack(grid);
      expect(result.valid).toBe(true);
    });

    it('accepts a large oval layout', () => {
      const grid = makeGrid(6, 6);
      // Top row (left to right)
      grid[0][1] = 'start-line';
      grid[0][2] = 'straight-h';
      grid[0][3] = 'straight-h';
      grid[0][4] = 'finish-line';
      // Right column (top to bottom)
      grid[1][4] = 'straight-v';
      grid[2][4] = 'straight-v';
      grid[3][4] = 'straight-v';
      grid[4][4] = 'straight-v';
      // Bottom row (right to left, with corners)
      grid[5][4] = 'corner-bl';
      grid[5][3] = 'straight-h';
      grid[5][2] = 'straight-h';
      grid[5][1] = 'corner-br';
      // Left column (bottom to top)
      grid[4][1] = 'straight-v';
      grid[3][1] = 'straight-v';
      grid[2][1] = 'straight-v';
      grid[1][1] = 'straight-v';

      const result = validateTrack(grid);
      expect(result.valid).toBe(true);
      expect(result.disconnected).toEqual([]);
    });
  });

  describe('disconnected tiles', () => {
    it('detects a disconnected island', () => {
      const grid = makeGrid(3, 5);
      // Main connected track
      grid[0][0] = 'start-line';
      grid[0][1] = 'straight-h';
      grid[0][2] = 'finish-line';
      // Isolated island
      grid[2][4] = 'straight-h';

      const result = validateTrack(grid);
      expect(result.valid).toBe(false);
      expect(result.message).toBe('1 disconnected tile(s)');
      expect(result.disconnected).toEqual([{ row: 2, col: 4 }]);
    });

    it('detects multiple disconnected tiles', () => {
      const grid = makeGrid(5, 5);
      // Main track cluster
      grid[0][0] = 'start-line';
      grid[0][1] = 'finish-line';
      // Two separate islands
      grid[3][3] = 'straight-h';
      grid[4][4] = 'corner-tl';

      const result = validateTrack(grid);
      expect(result.valid).toBe(false);
      expect(result.message).toBe('2 disconnected tile(s)');
      expect(result.disconnected).toHaveLength(2);
      expect(result.disconnected).toContainEqual({ row: 3, col: 3 });
      expect(result.disconnected).toContainEqual({ row: 4, col: 4 });
    });

    it('does not count diagonal adjacency as connected', () => {
      const grid = makeGrid(3, 3);
      grid[0][0] = 'start-line';
      grid[0][1] = 'finish-line';
      // Diagonally adjacent but not orthogonally connected
      grid[1][2] = 'straight-h';

      const result = validateTrack(grid);
      expect(result.valid).toBe(false);
      expect(result.disconnected).toEqual([{ row: 1, col: 2 }]);
    });
  });

  describe('scenery exclusion', () => {
    it('ignores scenery tiles in connectivity check', () => {
      const grid = makeGrid(1, 5);
      grid[0][0] = 'start-line';
      grid[0][1] = 'tree';
      grid[0][2] = 'grandstand';
      grid[0][3] = 'barrier';
      grid[0][4] = 'finish-line';

      // start-line at col 0 and finish-line at col 4 are not
      // orthogonally adjacent (scenery in between doesn't count)
      const result = validateTrack(grid);
      expect(result.valid).toBe(false);
      expect(result.message).toBe('1 disconnected tile(s)');
    });

    it('scenery tiles do not appear in disconnected list', () => {
      const grid = makeGrid(2, 3);
      grid[0][0] = 'start-line';
      grid[0][1] = 'finish-line';
      grid[1][0] = 'tree';
      grid[1][1] = 'grandstand';
      grid[1][2] = 'barrier';

      const result = validateTrack(grid);
      expect(result.valid).toBe(true);
      expect(result.disconnected).toEqual([]);
    });
  });

  describe('degenerate cases', () => {
    it('rejects a single start-line tile with no finish', () => {
      const result = validateTrack([['start-line']]);
      expect(result.valid).toBe(false);
      expect(result.message).toBe('Missing finish line');
    });

    it('accepts two adjacent tiles: start + finish', () => {
      const grid = [['start-line', 'finish-line']];
      const result = validateTrack(grid);
      expect(result.valid).toBe(true);
      expect(result.message).toBe('Track is valid');
    });

    it('handles a 1x1 grid with no track tile', () => {
      const result = validateTrack([['']]);
      expect(result.valid).toBe(false);
      expect(result.message).toBe('No track tiles placed');
    });

    it('handles a 1x1 grid with scenery', () => {
      const result = validateTrack([['tree']]);
      expect(result.valid).toBe(false);
      expect(result.message).toBe('No track tiles placed');
    });
  });
});
