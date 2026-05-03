/**
 * Samples a position and heading from a centerline path produced by
 * TrackPathExtractor.extractPath(). Supports multi-lane stacking via a
 * perpendicular offset from the centerline.
 */
export class TrackPathSampler {
  /** @param {{ waypoints: {x:number, y:number, arcLength:number}[], totalLength: number, isLoop: boolean }} path */
  constructor(path) {
    this._waypoints = path.waypoints;
    this._totalLength = path.totalLength;
  }

  /**
   * Sample position and angle at parameter t along the path.
   *
   * Lane stacking is perpendicular to the path tangent. Lane 0 is offset to
   * the left of the direction of travel; higher indices move right.
   *
   * @param {number} t          - Path parameter in [0, 1]
   * @param {number} laneIndex  - Zero-based lane index (default 0)
   * @param {number} laneCount  - Total lanes in the stack (default 1)
   * @param {number} laneWidth  - Pixel width of each lane (default 0)
   * @returns {{ x: number, y: number, angle: number }}
   */
  sample(t, laneIndex = 0, laneCount = 1, laneWidth = 0) {
    const waypoints = this._waypoints;
    const n = waypoints.length;

    if (n === 0) return { x: 0, y: 0, angle: 0 };
    if (n === 1) return { x: waypoints[0].x, y: waypoints[0].y, angle: 0 };

    const clamped = Math.max(0, Math.min(1, t));
    const target = clamped * this._totalLength;

    // Binary search: find lo such that waypoints[lo].arcLength <= target < waypoints[lo+1].arcLength
    let lo = 0;
    let hi = n - 1;
    while (lo < hi - 1) {
      const mid = (lo + hi) >> 1;
      if (waypoints[mid].arcLength <= target) {
        lo = mid;
      } else {
        hi = mid;
      }
    }

    const p0 = waypoints[lo];
    const p1 = waypoints[hi];

    const angle = Math.atan2(p1.y - p0.y, p1.x - p0.x);

    let x, y;
    const segLen = p1.arcLength - p0.arcLength;
    if (segLen <= 0) {
      x = p0.x;
      y = p0.y;
    } else {
      const u = (target - p0.arcLength) / segLen;
      x = p0.x + u * (p1.x - p0.x);
      y = p0.y + u * (p1.y - p0.y);
    }

    if (laneCount > 1 && laneWidth > 0) {
      const centerLane = (laneCount - 1) / 2;
      const laneOffset = (laneIndex - centerLane) * laneWidth;
      // Left-perpendicular of direction angle: (-sin, cos)
      x += -Math.sin(angle) * laneOffset;
      y +=  Math.cos(angle) * laneOffset;
    }

    return { x, y, angle };
  }
}
