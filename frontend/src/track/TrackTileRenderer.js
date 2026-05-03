/**
 * TrackTileRenderer — pre-renders a tile-grid track to an offscreen canvas.
 *
 * Usage:
 *   const renderer = new TrackTileRenderer(80);
 *   renderer.render(track.tiles);          // build offscreen canvas once
 *   renderer.draw(ctx, offsetX, offsetY);  // blit each frame
 *
 * Tile types (from TilePalette.js):
 *   straight-h, straight-v, curve-ne, curve-nw, curve-se, curve-sw,
 *   chicane, pit-entry, pit-exit, start-line, finish-line,
 *   grandstand, tree, barrier
 */

// Track surface occupies 60 % of each tile dimension.
const TRACK_RATIO  = 0.6;
const TRACK_MARGIN = (1 - TRACK_RATIO) / 2;   // 0.2

// Curbing stripe band is 6 % of tile size.
const CURB_RATIO   = 0.06;
const CURB_STRIPES = 4;

// Colours — kept consistent with the existing Track.js aesthetic.
const C_GRASS   = '#1e3a1e';
const C_ASPHALT = '#2d2d40';
const C_ASPHALT2 = '#333345';
const C_PIT     = '#252535';
const C_CURB_R  = '#e03030';
const C_CURB_W  = '#f0f0f0';
const C_START_W = '#ffffff';
const C_START_K = '#111111';
const C_FINISH_R = '#e94560';
const C_FINISH_K = '#1a1a2e';
const SPECTATOR_COLORS = ['#ff6688', '#88aaff', '#ffdd44'];

export class TrackTileRenderer {
  /**
   * @param {number} tileSize - pixel size of one tile (default 80)
   */
  constructor(tileSize = 80) {
    this.tileSize = tileSize;
    this._canvas  = null;
    this._width   = 0;
    this._height  = 0;
  }

  /** Width of the pre-rendered canvas in pixels. */
  get width() { return this._width; }

  /** Height of the pre-rendered canvas in pixels. */
  get height() { return this._height; }

  /** The offscreen HTMLCanvasElement, or null before render() is called. */
  get canvas() { return this._canvas; }

  /**
   * Pre-render all tiles to an offscreen canvas.  Call once whenever the
   * tile grid changes; the result is cached until render() is called again.
   *
   * @param {string[][]} tiles - 2-D array where tiles[row][col] is a tile-type
   *   string (or empty string / undefined for an empty cell).
   */
  render(tiles) {
    const rows = tiles.length;
    const cols = rows > 0 ? tiles[0].length : 0;
    const sz   = this.tileSize;

    this._width  = cols * sz;
    this._height = rows * sz;

    this._canvas = document.createElement('canvas');
    this._canvas.width  = this._width;
    this._canvas.height = this._height;
    const ctx = this._canvas.getContext('2d');

    // Grass / terrain background.
    ctx.fillStyle = C_GRASS;
    ctx.fillRect(0, 0, this._width, this._height);

    for (let r = 0; r < rows; r++) {
      for (let c = 0; c < cols; c++) {
        const tile = tiles[r][c];
        if (!tile) continue;
        this._drawTile(ctx, c * sz, r * sz, sz, tile);
      }
    }
  }

  /**
   * Blit the pre-rendered track onto a canvas context.
   * @param {CanvasRenderingContext2D} ctx
   * @param {number} x - destination X offset
   * @param {number} y - destination Y offset
   */
  draw(ctx, x = 0, y = 0) {
    if (this._canvas) ctx.drawImage(this._canvas, x, y);
  }

  // ─────────────────────────────────────────────────────────────────────────────
  // Dispatch

  _drawTile(ctx, x, y, sz, tile) {
    ctx.save();
    switch (tile) {
      case 'straight-h':  this._drawStraightH(ctx, x, y, sz);  break;
      case 'straight-v':  this._drawStraightV(ctx, x, y, sz);  break;
      case 'curve-ne':    this._drawCurveNE(ctx, x, y, sz);    break;
      case 'curve-nw':    this._drawCurveNW(ctx, x, y, sz);    break;
      case 'curve-se':    this._drawCurveSE(ctx, x, y, sz);    break;
      case 'curve-sw':    this._drawCurveSW(ctx, x, y, sz);    break;
      case 'chicane':     this._drawChicane(ctx, x, y, sz);    break;
      case 'pit-entry':   this._drawPitEntry(ctx, x, y, sz);   break;
      case 'pit-exit':    this._drawPitExit(ctx, x, y, sz);    break;
      case 'start-line':  this._drawStartLine(ctx, x, y, sz);  break;
      case 'finish-line': this._drawFinishLine(ctx, x, y, sz); break;
      case 'grandstand':  this._drawGrandstand(ctx, x, y, sz); break;
      case 'tree':        this._drawTree(ctx, x, y, sz);       break;
      case 'barrier':     this._drawBarrier(ctx, x, y, sz);    break;
      default: break;
    }
    ctx.restore();
  }

  // ─────────────────────────────────────────────────────────────────────────────
  // Straight tiles

  _drawStraightH(ctx, x, y, sz) {
    const m  = sz * TRACK_MARGIN;
    const tw = sz * TRACK_RATIO;
    const cw = sz * CURB_RATIO;

    // Asphalt surface.
    ctx.fillStyle = C_ASPHALT;
    ctx.fillRect(x, y + m, sz, tw);

    // Depth gradient (darker at edges).
    const g = ctx.createLinearGradient(x, y + m, x, y + m + tw);
    g.addColorStop(0,   C_ASPHALT2);
    g.addColorStop(0.3, C_ASPHALT);
    g.addColorStop(0.7, C_ASPHALT);
    g.addColorStop(1,   C_ASPHALT2);
    ctx.fillStyle = g;
    ctx.fillRect(x, y + m, sz, tw);

    // Curbing bands on top and bottom edges.
    this._curbH(ctx, x, y + m,            sz, cw);
    this._curbH(ctx, x, y + m + tw - cw,  sz, cw);
  }

  _drawStraightV(ctx, x, y, sz) {
    const m  = sz * TRACK_MARGIN;
    const tw = sz * TRACK_RATIO;
    const cw = sz * CURB_RATIO;

    ctx.fillStyle = C_ASPHALT;
    ctx.fillRect(x + m, y, tw, sz);

    const g = ctx.createLinearGradient(x + m, y, x + m + tw, y);
    g.addColorStop(0,   C_ASPHALT2);
    g.addColorStop(0.3, C_ASPHALT);
    g.addColorStop(0.7, C_ASPHALT);
    g.addColorStop(1,   C_ASPHALT2);
    ctx.fillStyle = g;
    ctx.fillRect(x + m, y, tw, sz);

    // Curbing bands on left and right edges.
    this._curbV(ctx, x + m,           y, cw, sz);
    this._curbV(ctx, x + m + tw - cw, y, cw, sz);
  }

  // ─────────────────────────────────────────────────────────────────────────────
  // Curbing helpers

  /** Horizontal curbing — alternating red/white stripes across width w. */
  _curbH(ctx, x, y, w, cw) {
    const sw = w / CURB_STRIPES;
    for (let i = 0; i < CURB_STRIPES; i++) {
      ctx.fillStyle = i % 2 === 0 ? C_CURB_R : C_CURB_W;
      ctx.fillRect(x + i * sw, y, sw, cw);
    }
  }

  /** Vertical curbing — alternating red/white stripes across height h. */
  _curbV(ctx, x, y, cw, h) {
    const sh = h / CURB_STRIPES;
    for (let i = 0; i < CURB_STRIPES; i++) {
      ctx.fillStyle = i % 2 === 0 ? C_CURB_R : C_CURB_W;
      ctx.fillRect(x, y + i * sh, cw, sh);
    }
  }

  // ─────────────────────────────────────────────────────────────────────────────
  // Curve tiles
  //
  // Naming convention matches TilePalette.js + editor arc origins:
  //   curve-ne : arc center at tile bottom-left  → connects LEFT  ↔ BOTTOM edge
  //   curve-nw : arc center at tile bottom-right → connects RIGHT ↔ BOTTOM edge
  //   curve-se : arc center at tile top-left     → connects LEFT  ↔ TOP    edge
  //   curve-sw : arc center at tile top-right    → connects RIGHT ↔ TOP    edge

  _drawCurveNE(ctx, x, y, sz) {
    this._arcTrack(ctx, x,      y + sz, sz, -Math.PI / 2, 0);
  }

  _drawCurveNW(ctx, x, y, sz) {
    this._arcTrack(ctx, x + sz, y + sz, sz, Math.PI, Math.PI * 1.5);
  }

  _drawCurveSE(ctx, x, y, sz) {
    this._arcTrack(ctx, x,      y,      sz, 0, Math.PI / 2);
  }

  _drawCurveSW(ctx, x, y, sz) {
    this._arcTrack(ctx, x + sz, y,      sz, Math.PI / 2, Math.PI);
  }

  /**
   * Draw a donut-slice track segment (arc from startAngle to endAngle) plus
   * curbing on both arc edges.  All arcs are counter-clockwise=false.
   */
  _arcTrack(ctx, cx, cy, sz, startAngle, endAngle) {
    const outer = sz * (TRACK_MARGIN + TRACK_RATIO);  // 0.8
    const inner = sz * TRACK_MARGIN;                  // 0.2
    const cw    = sz * CURB_RATIO;

    // Asphalt donut slice.
    this._donutSlicePath(ctx, cx, cy, outer, inner, startAngle, endAngle);
    ctx.fillStyle = C_ASPHALT;
    ctx.fill();

    // Depth tint — subtle radial gradient.
    const rg = ctx.createRadialGradient(cx, cy, inner, cx, cy, outer);
    rg.addColorStop(0,   'rgba(51,51,69,0.5)');
    rg.addColorStop(0.4, 'rgba(45,45,64,0)');
    rg.addColorStop(1,   'rgba(51,51,69,0.5)');
    this._donutSlicePath(ctx, cx, cy, outer, inner, startAngle, endAngle);
    ctx.fillStyle = rg;
    ctx.fill();

    // Curbing on outer and inner edges.
    this._arcCurb(ctx, cx, cy, outer - cw, outer, startAngle, endAngle, false);
    this._arcCurb(ctx, cx, cy, inner, inner + cw, startAngle, endAngle, false);
  }

  /** Trace a donut-slice (annular sector) path without filling. */
  _donutSlicePath(ctx, cx, cy, outerR, innerR, startAngle, endAngle) {
    ctx.beginPath();
    ctx.arc(cx, cy, outerR, startAngle, endAngle, false);
    ctx.arc(cx, cy, innerR, endAngle,   startAngle, true);
    ctx.closePath();
  }

  /**
   * Draw arc-shaped curbing between innerR and outerR divided into CURB_STRIPES
   * alternating red/white donut-slice segments.
   */
  _arcCurb(ctx, cx, cy, innerR, outerR, startAngle, endAngle, ccw) {
    const span  = (endAngle - startAngle + Math.PI * 4) % (Math.PI * 2);
    const sAngle = span / CURB_STRIPES;

    for (let i = 0; i < CURB_STRIPES; i++) {
      const a0 = startAngle + i * sAngle;
      const a1 = a0 + sAngle;

      ctx.beginPath();
      ctx.arc(cx, cy, outerR, a0, a1, ccw);
      ctx.arc(cx, cy, innerR, a1, a0, !ccw);
      ctx.closePath();
      ctx.fillStyle = i % 2 === 0 ? C_CURB_R : C_CURB_W;
      ctx.fill();
    }
  }

  // ─────────────────────────────────────────────────────────────────────────────
  // Chicane (horizontal S-curve)

  _drawChicane(ctx, x, y, sz) {
    const m      = sz * TRACK_MARGIN;
    const tw     = sz * TRACK_RATIO;
    const cw     = sz * CURB_RATIO;
    const amp    = sz * 0.18;   // S-curve amplitude

    // Control point X offsets.
    const cp1x = x + sz * 0.3;
    const cp2x = x + sz * 0.7;

    // Fill track surface as a closed bezier shape.
    ctx.beginPath();
    ctx.moveTo(x,      y + m);
    ctx.bezierCurveTo(cp1x, y + m - amp,      cp2x, y + m + amp,      x + sz, y + m);
    ctx.lineTo(x + sz, y + m + tw);
    ctx.bezierCurveTo(cp2x, y + m + tw + amp, cp1x, y + m + tw - amp, x,      y + m + tw);
    ctx.closePath();
    ctx.fillStyle = C_ASPHALT;
    ctx.fill();

    // Curbing — approximate with short rects placed along the bezier.
    const steps = CURB_STRIPES * 2;
    const sw    = sz / steps;

    for (let i = 0; i < steps; i++) {
      const t   = (i + 0.5) / steps;
      const topY = this._bezierY(y + m,      y + m - amp,      y + m + amp,      y + m,      t);
      const botY = this._bezierY(y + m + tw, y + m + tw + amp, y + m + tw - amp, y + m + tw, t);

      ctx.fillStyle = i % 2 === 0 ? C_CURB_R : C_CURB_W;
      ctx.fillRect(x + i * sw, topY,        sw + 1, cw);
      ctx.fillRect(x + i * sw, botY - cw,   sw + 1, cw);
    }
  }

  /** Cubic bezier Y at parameter t (Y-values only; X advances linearly). */
  _bezierY(p0, p1, p2, p3, t) {
    const u = 1 - t;
    return u * u * u * p0
         + 3 * u * u * t * p1
         + 3 * u * t * t * p2
         + t * t * t * p3;
  }

  // ─────────────────────────────────────────────────────────────────────────────
  // Pit tiles

  _drawPitEntry(ctx, x, y, sz) {
    this._drawPitSurface(ctx, x, y, sz);
    this._pitLabel(ctx, x, y, sz, 'P', true);
  }

  _drawPitExit(ctx, x, y, sz) {
    this._drawPitSurface(ctx, x, y, sz);
    this._pitLabel(ctx, x, y, sz, 'P', false);
  }

  _drawPitSurface(ctx, x, y, sz) {
    const m  = sz * TRACK_MARGIN;
    const tw = sz * TRACK_RATIO;

    ctx.fillStyle = C_PIT;
    ctx.fillRect(x, y + m, sz, tw);

    // Yellow boundary lines.
    ctx.strokeStyle = '#ffaa00';
    ctx.lineWidth   = Math.max(1, sz * 0.025);
    ctx.setLineDash([]);
    ctx.beginPath(); ctx.moveTo(x, y + m);      ctx.lineTo(x + sz, y + m);      ctx.stroke();
    ctx.beginPath(); ctx.moveTo(x, y + m + tw); ctx.lineTo(x + sz, y + m + tw); ctx.stroke();
  }

  _pitLabel(ctx, x, y, sz, letter, entry) {
    const arrowSz = Math.max(6, Math.floor(sz * 0.18));
    const cx = x + sz / 2;
    const cy = y + sz / 2;
    const aw = arrowSz * 0.6;

    ctx.fillStyle = '#ffaa00';
    ctx.font = `bold ${Math.floor(sz * 0.22)}px Courier New`;
    ctx.textAlign    = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText(letter, cx - arrowSz * 0.7, cy);

    // Draw small down or up arrow.
    const ay = cy;
    const dir = entry ? 1 : -1;
    ctx.beginPath();
    ctx.moveTo(cx + aw * 0.2,         ay - dir * arrowSz * 0.5);
    ctx.lineTo(cx + aw * 0.2,         ay + dir * arrowSz * 0.2);
    ctx.lineTo(cx + aw * 0.2 + aw,    ay + dir * arrowSz * 0.2);
    ctx.lineTo(cx + aw * 0.2,         ay + dir * arrowSz * 0.8);
    ctx.lineTo(cx + aw * 0.2 - aw,    ay + dir * arrowSz * 0.2);
    ctx.lineTo(cx + aw * 0.2 - aw * 0.4, ay + dir * arrowSz * 0.2);
    ctx.lineTo(cx + aw * 0.2 - aw * 0.4, ay - dir * arrowSz * 0.5);
    ctx.closePath();
    ctx.fill();

    ctx.textBaseline = 'alphabetic';
  }

  // ─────────────────────────────────────────────────────────────────────────────
  // Start / Finish lines

  _drawStartLine(ctx, x, y, sz) {
    this._drawStraightH(ctx, x, y, sz);
    this._drawCheckerPattern(ctx, x, y, sz, C_START_W, C_START_K);
    this._trackBadge(ctx, x, y, sz, 'S', '#ffffff');
  }

  _drawFinishLine(ctx, x, y, sz) {
    this._drawStraightH(ctx, x, y, sz);
    this._drawCheckerPattern(ctx, x, y, sz, C_FINISH_R, C_FINISH_K);
    this._trackBadge(ctx, x, y, sz, 'F', C_FINISH_R);
  }

  /** Draw a letter badge above the track surface. */
  _trackBadge(ctx, x, y, sz, letter, color) {
    ctx.fillStyle    = color;
    ctx.font         = `bold ${Math.floor(sz * 0.28)}px Courier New`;
    ctx.textAlign    = 'center';
    ctx.textBaseline = 'bottom';
    ctx.fillText(letter, x + sz * 0.5, y + sz * TRACK_MARGIN - 2);
    ctx.textBaseline = 'alphabetic';
  }

  /** Draw a checkered pattern over the track surface (between the curbing bands). */
  _drawCheckerPattern(ctx, x, y, sz, colorA, colorB) {
    const m   = sz * TRACK_MARGIN;
    const tw  = sz * TRACK_RATIO;
    const cw  = sz * CURB_RATIO;
    const patY = y + m + cw;
    const patH = tw - 2 * cw;
    const cs   = Math.max(3, Math.floor(sz * 0.1));
    const cols = Math.ceil(sz / cs) + 1;
    const rows = Math.ceil(patH / cs) + 1;

    ctx.save();
    ctx.beginPath();
    ctx.rect(x, patY, sz, patH);
    ctx.clip();

    for (let r = 0; r < rows; r++) {
      for (let c = 0; c < cols; c++) {
        ctx.fillStyle = (r + c) % 2 === 0 ? colorA : colorB;
        ctx.fillRect(x + c * cs, patY + r * cs, cs, cs);
      }
    }

    ctx.restore();
  }

  // ─────────────────────────────────────────────────────────────────────────────
  // Scenery tiles

  _drawGrandstand(ctx, x, y, sz) {
    const pad  = sz * 0.05;
    const bx   = x + pad;
    const by   = y + pad;
    const bw   = sz - 2 * pad;
    const bh   = sz - 2 * pad;
    const rows = 3;
    const rh   = bh / rows;

    // Foundation.
    ctx.fillStyle = '#3a2a55';
    ctx.fillRect(bx, by, bw, bh);

    // Tiered rows.
    for (let r = 0; r < rows; r++) {
      ctx.fillStyle = r % 2 === 0 ? '#553388' : '#442277';
      ctx.fillRect(bx, by + r * rh, bw, rh);
    }

    // Spectator dots.
    const dotGap = Math.max(6, Math.floor(sz * 0.1));
    for (let r = 0; r < rows; r++) {
      const numDots = Math.floor(bw / dotGap);
      for (let d = 0; d < numDots; d++) {
        const dotX = bx + d * dotGap + dotGap / 2;
        const dotY = by + r * rh + rh / 2;
        ctx.fillStyle = SPECTATOR_COLORS[d % 3];
        ctx.beginPath();
        ctx.arc(dotX, dotY, Math.max(1, sz * 0.025), 0, Math.PI * 2);
        ctx.fill();
      }
    }

    // Label.
    ctx.fillStyle    = '#bb88ff';
    ctx.font         = `bold ${Math.max(7, Math.floor(sz * 0.15))}px Courier New`;
    ctx.textAlign    = 'center';
    ctx.textBaseline = 'bottom';
    ctx.fillText('STAND', x + sz / 2, y + sz - pad / 2);
    ctx.textBaseline = 'alphabetic';
  }

  _drawTree(ctx, x, y, sz) {
    const cx = x + sz / 2;
    const cy = y + sz * 0.55;
    const r  = sz * 0.32;

    // Shadow.
    ctx.fillStyle = 'rgba(0,0,0,0.18)';
    ctx.beginPath();
    ctx.arc(cx + sz * 0.06, cy + sz * 0.08, r * 0.85, 0, Math.PI * 2);
    ctx.fill();

    // Main canopy.
    ctx.fillStyle = '#228833';
    ctx.beginPath();
    ctx.arc(cx, cy, r, 0, Math.PI * 2);
    ctx.fill();

    // Highlight clusters.
    ctx.fillStyle = '#44aa55';
    ctx.beginPath();
    ctx.arc(cx - sz * 0.1, cy - sz * 0.08, r * 0.55, 0, Math.PI * 2);
    ctx.fill();

    ctx.fillStyle = '#55cc66';
    ctx.beginPath();
    ctx.arc(cx + sz * 0.07, cy - sz * 0.14, r * 0.38, 0, Math.PI * 2);
    ctx.fill();
  }

  _drawBarrier(ctx, x, y, sz) {
    const pad = sz * 0.08;
    const bh  = sz * 0.32;
    const bx  = x + pad;
    const by  = y + (sz - bh) / 2;
    const bw  = sz - 2 * pad;
    const cap = sz * 0.12;

    // Body.
    ctx.fillStyle = '#cc3333';
    ctx.fillRect(bx, by, bw, bh);

    // White centre stripe.
    ctx.fillStyle = '#f0f0f0';
    ctx.fillRect(bx, by + bh * 0.35, bw, bh * 0.3);

    // Red end caps (brighter).
    ctx.fillStyle = '#ff4444';
    ctx.fillRect(bx,          by, cap, bh);
    ctx.fillRect(bx + bw - cap, by, cap, bh);

    // Ground shadow.
    ctx.fillStyle = 'rgba(0,0,0,0.25)';
    ctx.fillRect(bx + 3, by + bh, bw - 2, sz * 0.07);
  }
}
