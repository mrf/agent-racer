const TOKEN_MARKERS = [
  { tokens: 50000, label: '50K' },
  { tokens: 100000, label: '100K' },
  { tokens: 150000, label: '150K' },
];

const PENNANT_COLORS = ['#a855f7', '#3b82f6', '#22c55e'];

const SKIN_TONES = ['#f5d0a9', '#d4a574', '#c68642', '#8d5524', '#e0ac69', '#f1c27d'];
const SHIRT_COLORS = [
  '#e94560', '#3b82f6', '#22c55e', '#a855f7', '#f59e0b',
  '#06b6d4', '#ec4899', '#f97316', '#84cc16', '#eee',
];
const CROWD_ROWS = [
  { spacing: 13, offsetY: 14, scale: 0.65, xShift: 0 },
  { spacing: 13, offsetY: 0, scale: 0.85, xShift: 6.5 },
];

const PIT_LANE_HEIGHT = 50;
const PIT_GAP = 30;
const PIT_PADDING_LEFT = 40;
const PIT_BOTTOM_PADDING = 40;
const PIT_ENTRY_OFFSET = 60;
const PIT_ENTRY_WIDTH = 40;

const PARKING_LOT_LANE_HEIGHT = 45;
const PARKING_LOT_GAP = 20;
const PARKING_LOT_PADDING_LEFT = 40;
const PARKING_LOT_BOTTOM_PADDING = 40;

export class Track {
  constructor() {
    this.trackPadding = { left: 200, right: 60, top: 60, bottom: 40 };
    this.laneHeight = 80;
    this.time = 0;
    // Pre-rendered offscreen canvases
    this._textureCanvas = null;
    this._spectators = null;
    this._lastWidth = 0;
    this._lastHeight = 0;
    this._lastLaneCount = 0;
  }

  getRequiredHeight(laneCount, pitLaneCount = 0, parkingLotLaneCount = 0) {
    const maxLanes = Math.max(laneCount, 1);
    const trackHeight = maxLanes * this.laneHeight + this.trackPadding.top + this.trackPadding.bottom;
    return trackHeight + this.getRequiredPitHeight(pitLaneCount) + this.getRequiredParkingLotHeight(parkingLotLaneCount);
  }

  getRequiredPitHeight(pitLaneCount) {
    const lanes = Math.max(pitLaneCount, 1);
    return PIT_GAP + lanes * PIT_LANE_HEIGHT + PIT_BOTTOM_PADDING;
  }

  getTrackBounds(canvasWidth, canvasHeight, laneCount) {
    const maxLanes = Math.max(laneCount, 1);
    const trackHeight = maxLanes * this.laneHeight;
    const totalHeight = trackHeight + this.trackPadding.top + this.trackPadding.bottom;
    const trackWidth = canvasWidth - this.trackPadding.left - this.trackPadding.right;

    return {
      x: this.trackPadding.left,
      y: this.trackPadding.top,
      width: trackWidth,
      height: trackHeight,
      totalHeight,
      laneHeight: this.laneHeight,
    };
  }

  getLaneY(bounds, lane) {
    return bounds.y + lane * bounds.laneHeight + bounds.laneHeight / 2;
  }

  getPositionX(bounds, utilization) {
    return bounds.x + utilization * bounds.width;
  }

  getPitBounds(canvasWidth, canvasHeight, activeLaneCount, pitLaneCount) {
    const lanes = Math.max(pitLaneCount, 1);
    const trackBounds = this.getTrackBounds(canvasWidth, canvasHeight, activeLaneCount);
    const pitTop = trackBounds.y + trackBounds.height + PIT_GAP;
    const pitX = trackBounds.x + PIT_PADDING_LEFT;
    const pitWidth = trackBounds.width - PIT_PADDING_LEFT;
    const pitHeight = lanes * PIT_LANE_HEIGHT;
    return {
      x: pitX,
      y: pitTop,
      width: pitWidth,
      height: pitHeight,
      laneHeight: PIT_LANE_HEIGHT,
    };
  }

  getPitEntryX(trackBounds) {
    return trackBounds.x + PIT_ENTRY_OFFSET;
  }

  getRequiredParkingLotHeight(parkingLotLaneCount) {
    if (parkingLotLaneCount <= 0) return 0;
    return PARKING_LOT_GAP + parkingLotLaneCount * PARKING_LOT_LANE_HEIGHT + PARKING_LOT_BOTTOM_PADDING;
  }

  getParkingLotBounds(canvasWidth, canvasHeight, activeLaneCount, pitLaneCount, parkingLotLaneCount) {
    if (parkingLotLaneCount <= 0) return null;
    const trackBounds = this.getTrackBounds(canvasWidth, canvasHeight, activeLaneCount);
    const pitHeight = this.getRequiredPitHeight(pitLaneCount);
    const lotTop = trackBounds.y + trackBounds.height + pitHeight + PARKING_LOT_GAP;
    const lotX = trackBounds.x + PARKING_LOT_PADDING_LEFT;
    const lotWidth = trackBounds.width - PARKING_LOT_PADDING_LEFT;
    const lotHeight = parkingLotLaneCount * PARKING_LOT_LANE_HEIGHT;
    return {
      x: lotX,
      y: lotTop,
      width: lotWidth,
      height: lotHeight,
      laneHeight: PARKING_LOT_LANE_HEIGHT,
    };
  }

  _needsPrerender(canvasWidth, canvasHeight, laneCount) {
    return canvasWidth !== this._lastWidth ||
           canvasHeight !== this._lastHeight ||
           laneCount !== this._lastLaneCount;
  }

  _prerenderTexture(bounds) {
    const w = bounds.width + 20;
    const h = bounds.height + 20;
    this._textureCanvas = document.createElement('canvas');
    this._textureCanvas.width = w;
    this._textureCanvas.height = h;
    const tc = this._textureCanvas.getContext('2d');

    // Cross-hatch pattern for asphalt texture
    tc.strokeStyle = 'rgba(255,255,255,0.03)';
    tc.lineWidth = 1;
    for (let i = -h; i < w + h; i += 20) {
      tc.beginPath();
      tc.moveTo(i, 0);
      tc.lineTo(i + h, h);
      tc.stroke();
      tc.beginPath();
      tc.moveTo(i + h, 0);
      tc.lineTo(i, h);
      tc.stroke();
    }
  }

  _prerenderCrowd(bounds) {
    const w = bounds.width + 20;
    this._spectators = [];

    for (const row of CROWD_ROWS) {
      const count = Math.floor(w / row.spacing);
      for (let i = 0; i < count; i++) {
        this._spectators.push({
          x: i * row.spacing + row.spacing / 2 + row.xShift,
          rowOffset: row.offsetY,
          scale: row.scale,
          headR: (3 + Math.random() * 1.5) * row.scale,
          bodyH: (9 + Math.random() * 4) * row.scale,
          skinColor: SKIN_TONES[Math.floor(Math.random() * SKIN_TONES.length)],
          shirtColor: SHIRT_COLORS[Math.floor(Math.random() * SHIRT_COLORS.length)],
          phase: Math.random() * Math.PI * 2,
          cheerThreshold: Math.random() * 0.95,
        });
      }
    }
  }

  draw(ctx, canvasWidth, canvasHeight, laneCount, maxTokens = 200000, excitement = 0) {
    const bounds = this.getTrackBounds(canvasWidth, canvasHeight, laneCount);
    this.time += 0.016; // ~60fps tick

    // Pre-render static elements on resize/lane change
    if (this._needsPrerender(canvasWidth, canvasHeight, laneCount)) {
      this._prerenderTexture(bounds);
      this._prerenderCrowd(bounds);
      this._lastWidth = canvasWidth;
      this._lastHeight = canvasHeight;
      this._lastLaneCount = laneCount;
    }

    // Track background (asphalt)
    ctx.fillStyle = '#2a2a3a';
    ctx.fillRect(bounds.x - 10, bounds.y - 10, bounds.width + 20, bounds.height + 20);

    // Track surface gradient
    const trackGrad = ctx.createLinearGradient(bounds.x, bounds.y, bounds.x, bounds.y + bounds.height);
    trackGrad.addColorStop(0, '#333345');
    trackGrad.addColorStop(0.5, '#2d2d40');
    trackGrad.addColorStop(1, '#333345');
    ctx.fillStyle = trackGrad;
    ctx.fillRect(bounds.x, bounds.y, bounds.width, bounds.height);

    // Asphalt texture overlay
    if (this._textureCanvas) {
      ctx.drawImage(this._textureCanvas, bounds.x - 10, bounds.y - 10);
    }

    // Track edge shadows for depth
    const topShadow = ctx.createLinearGradient(bounds.x, bounds.y - 10, bounds.x, bounds.y + 8);
    topShadow.addColorStop(0, 'rgba(0,0,0,0.3)');
    topShadow.addColorStop(1, 'rgba(0,0,0,0)');
    ctx.fillStyle = topShadow;
    ctx.fillRect(bounds.x - 10, bounds.y - 10, bounds.width + 20, 18);

    const botShadow = ctx.createLinearGradient(bounds.x, bounds.y + bounds.height + 10,
      bounds.x, bounds.y + bounds.height - 8);
    botShadow.addColorStop(0, 'rgba(0,0,0,0.3)');
    botShadow.addColorStop(1, 'rgba(0,0,0,0)');
    ctx.fillStyle = botShadow;
    ctx.fillRect(bounds.x - 10, bounds.y + bounds.height - 8, bounds.width + 20, 18);

    // Yellow edge lines
    ctx.strokeStyle = '#d4a017';
    ctx.lineWidth = 2;
    ctx.setLineDash([]);
    ctx.beginPath();
    ctx.moveTo(bounds.x, bounds.y - 1);
    ctx.lineTo(bounds.x + bounds.width, bounds.y - 1);
    ctx.stroke();
    ctx.beginPath();
    ctx.moveTo(bounds.x, bounds.y + bounds.height + 1);
    ctx.lineTo(bounds.x + bounds.width, bounds.y + bounds.height + 1);
    ctx.stroke();

    // White dashed lane dividers
    ctx.strokeStyle = '#ccccdd';
    ctx.lineWidth = 1;
    ctx.setLineDash([12, 8]);
    for (let i = 1; i < laneCount; i++) {
      const y = bounds.y + i * this.laneHeight;
      ctx.beginPath();
      ctx.moveTo(bounds.x, y);
      ctx.lineTo(bounds.x + bounds.width, y);
      ctx.stroke();
    }
    ctx.setLineDash([]);

    // Start line + checkerboard
    this._drawStartLine(ctx, bounds);

    // Finish line + animated checkerboard
    this._drawFinishLine(ctx, bounds, maxTokens);

    // Token markers with flag icons
    this._drawTokenMarkers(ctx, bounds, maxTokens);

    // Animated spectators above the track
    this._drawCrowd(ctx, bounds, excitement);

    // Pennant flags along top edge (drawn on top of spectators as barrier)
    this._drawPennants(ctx, bounds);

    return bounds;
  }

  drawPit(ctx, canvasWidth, canvasHeight, activeLaneCount, pitLaneCount) {
    const pitBounds = this.getPitBounds(canvasWidth, canvasHeight, activeLaneCount, pitLaneCount);
    const trackBounds = this.getTrackBounds(canvasWidth, canvasHeight, activeLaneCount);

    // Connecting lane between track and pit at the entry point
    const entryX = this.getPitEntryX(trackBounds);
    const laneLeft = entryX - PIT_ENTRY_WIDTH / 2;
    const gapTop = trackBounds.y + trackBounds.height;
    const gapBottom = pitBounds.y;
    const gapHeight = gapBottom - gapTop;

    // Dark surface fill
    ctx.fillStyle = '#252535';
    ctx.fillRect(laneLeft, gapTop, PIT_ENTRY_WIDTH, gapHeight);

    // Dashed side borders
    ctx.strokeStyle = 'rgba(100,100,120,0.5)';
    ctx.lineWidth = 1;
    ctx.setLineDash([4, 6]);
    ctx.beginPath();
    ctx.moveTo(laneLeft, gapTop);
    ctx.lineTo(laneLeft, gapBottom);
    ctx.stroke();
    ctx.beginPath();
    ctx.moveTo(laneLeft + PIT_ENTRY_WIDTH, gapTop);
    ctx.lineTo(laneLeft + PIT_ENTRY_WIDTH, gapBottom);
    ctx.stroke();
    ctx.setLineDash([]);

    // Down-chevron arrows inside the lane
    const chevronCount = Math.max(1, Math.floor(gapHeight / 14));
    ctx.strokeStyle = 'rgba(100,100,120,0.4)';
    ctx.lineWidth = 1.5;
    for (let i = 0; i < chevronCount; i++) {
      const cy = gapTop + 8 + i * (gapHeight / chevronCount);
      ctx.beginPath();
      ctx.moveTo(entryX - 6, cy - 3);
      ctx.lineTo(entryX, cy + 3);
      ctx.lineTo(entryX + 6, cy - 3);
      ctx.stroke();
    }

    this._drawAreaSurface(ctx, pitBounds, pitLaneCount, {
      bg: '#1e1e2e',
      gradientEdge: '#282838',
      gradientMid: '#222232',
      border: '#555',
      borderDash: [8, 6],
      label: 'PIT',
      labelColor: '#555',
      divider: '#333350',
    });

    return pitBounds;
  }

  drawParkingLot(ctx, canvasWidth, canvasHeight, activeLaneCount, pitLaneCount, parkingLotLaneCount) {
    if (parkingLotLaneCount <= 0) return null;
    const lotBounds = this.getParkingLotBounds(canvasWidth, canvasHeight, activeLaneCount, pitLaneCount, parkingLotLaneCount);

    this._drawAreaSurface(ctx, lotBounds, parkingLotLaneCount, {
      bg: '#161624',
      gradientEdge: '#1e1e2c',
      gradientMid: '#1a1a28',
      border: '#444',
      borderDash: [6, 8],
      label: 'PARKED',
      labelColor: '#444',
      divider: '#282840',
    });

    return lotBounds;
  }

  _drawAreaSurface(ctx, bounds, laneCount, style) {
    // Surface background
    ctx.fillStyle = style.bg;
    ctx.fillRect(bounds.x - 5, bounds.y - 5, bounds.width + 10, bounds.height + 10);

    // Surface gradient
    const grad = ctx.createLinearGradient(bounds.x, bounds.y, bounds.x, bounds.y + bounds.height);
    grad.addColorStop(0, style.gradientEdge);
    grad.addColorStop(0.5, style.gradientMid);
    grad.addColorStop(1, style.gradientEdge);
    ctx.fillStyle = grad;
    ctx.fillRect(bounds.x, bounds.y, bounds.width, bounds.height);

    // Dashed border
    ctx.strokeStyle = style.border;
    ctx.lineWidth = 1;
    ctx.setLineDash(style.borderDash);
    ctx.strokeRect(bounds.x, bounds.y, bounds.width, bounds.height);
    ctx.setLineDash([]);

    // Label
    ctx.fillStyle = style.labelColor;
    ctx.font = 'bold 14px Courier New';
    ctx.textAlign = 'right';
    ctx.textBaseline = 'middle';
    ctx.fillText(style.label, bounds.x - 10, bounds.y + bounds.height / 2);
    ctx.textBaseline = 'alphabetic';
    ctx.textAlign = 'center';

    // Lane dividers
    if (laneCount > 1) {
      ctx.strokeStyle = style.divider;
      ctx.lineWidth = 0.5;
      ctx.setLineDash([6, 8]);
      for (let i = 1; i < laneCount; i++) {
        const y = bounds.y + i * bounds.laneHeight;
        ctx.beginPath();
        ctx.moveTo(bounds.x, y);
        ctx.lineTo(bounds.x + bounds.width, y);
        ctx.stroke();
      }
      ctx.setLineDash([]);
    }
  }

  _drawStartLine(ctx, bounds) {
    // Start line
    ctx.strokeStyle = '#ffffff';
    ctx.lineWidth = 3;
    ctx.beginPath();
    ctx.moveTo(bounds.x, bounds.y - 10);
    ctx.lineTo(bounds.x, bounds.y + bounds.height + 10);
    ctx.stroke();

    // Checkerboard start pattern (12px squares, 4 columns)
    const checkSize = 12;
    const cols = 4;
    for (let row = 0; row < Math.ceil((bounds.height + 20) / checkSize); row++) {
      for (let col = 0; col < cols; col++) {
        ctx.fillStyle = (row + col) % 2 === 0 ? '#ffffff' : '#000000';
        ctx.fillRect(
          bounds.x - cols * checkSize - 2 + col * checkSize,
          bounds.y - 10 + row * checkSize,
          checkSize, checkSize
        );
      }
    }

    // Start label
    ctx.fillStyle = '#888';
    ctx.font = 'bold 12px Courier New';
    ctx.textAlign = 'center';
    ctx.fillText('0', bounds.x, bounds.y - 16);
  }

  _drawFinishLine(ctx, bounds, maxTokens) {
    const finishX = bounds.x + bounds.width;

    // Finish line
    ctx.strokeStyle = '#e94560';
    ctx.lineWidth = 3;
    ctx.beginPath();
    ctx.moveTo(finishX, bounds.y - 10);
    ctx.lineTo(finishX, bounds.y + bounds.height + 10);
    ctx.stroke();

    // Static checkerboard pattern
    const checkSize = 12;
    const cols = 4;
    for (let row = 0; row < Math.ceil((bounds.height + 20) / checkSize); row++) {
      for (let col = 0; col < cols; col++) {
        ctx.fillStyle = (row + col) % 2 === 0 ? '#e94560' : '#1a1a2e';
        ctx.fillRect(
          finishX + 2 + col * checkSize,
          bounds.y - 10 + row * checkSize,
          checkSize, checkSize
        );
      }
    }

    // "FINISH" text above
    ctx.fillStyle = '#e94560';
    ctx.font = 'bold 11px Courier New';
    ctx.textAlign = 'center';
    ctx.fillText('FINISH', finishX + cols * checkSize / 2 + 2, bounds.y - 20);

    // Token count label
    ctx.fillStyle = '#e94560';
    ctx.font = 'bold 12px Courier New';
    ctx.fillText(`${Math.round(maxTokens / 1000)}K`, finishX, bounds.y - 8);

    // Small checkered flag icon at finish
    this._drawCheckerFlag(ctx, finishX + cols * checkSize / 2 + 2, bounds.y - 30, 10);
  }

  _drawCheckerFlag(ctx, x, y, size) {
    // Flag pole
    ctx.strokeStyle = '#888';
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(x - size / 2, y + size);
    ctx.lineTo(x - size / 2, y - 2);
    ctx.stroke();

    // Flag with checkerboard
    const s = size / 4;
    for (let r = 0; r < 3; r++) {
      for (let c = 0; c < 4; c++) {
        ctx.fillStyle = (r + c) % 2 === 0 ? '#fff' : '#222';
        ctx.fillRect(x - size / 2 + c * s, y - 2 + r * s, s, s);
      }
    }
  }

  _drawTokenMarkers(ctx, bounds, maxTokens) {
    for (const marker of TOKEN_MARKERS) {
      if (marker.tokens >= maxTokens) continue;
      const markerX = this.getPositionX(bounds, marker.tokens / maxTokens);

      // Dashed line across track
      ctx.strokeStyle = '#444460';
      ctx.lineWidth = 1;
      ctx.setLineDash([4, 6]);
      ctx.beginPath();
      ctx.moveTo(markerX, bounds.y);
      ctx.lineTo(markerX, bounds.y + bounds.height);
      ctx.stroke();
      ctx.setLineDash([]);

      // Label on track surface (top area)
      ctx.fillStyle = '#888';
      ctx.font = 'bold 12px Courier New';
      ctx.textAlign = 'center';
      ctx.fillText(marker.label, markerX, bounds.y + 16);
    }
  }

  _drawPennants(ctx, bounds) {
    const spacing = 30;
    const count = Math.floor(bounds.width / spacing);
    const lineY = bounds.y - 12;

    // String line
    ctx.strokeStyle = '#555';
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(bounds.x, lineY);
    ctx.lineTo(bounds.x + bounds.width, lineY);
    ctx.stroke();

    // Triangular pennant flags
    for (let i = 0; i < count; i++) {
      const fx = bounds.x + i * spacing + spacing / 2;
      const color = PENNANT_COLORS[i % PENNANT_COLORS.length];
      const wave = Math.sin(this.time * 2 + i * 0.5) * 2;

      ctx.fillStyle = color;
      ctx.globalAlpha = 0.7;
      ctx.beginPath();
      ctx.moveTo(fx - 5, lineY);
      ctx.lineTo(fx + 5, lineY);
      ctx.lineTo(fx, lineY + 10 + wave);
      ctx.closePath();
      ctx.fill();
      ctx.globalAlpha = 1.0;
    }
  }

  _drawCrowd(ctx, bounds, excitement) {
    if (!this._spectators) return;

    // Spectators positioned above the track, just above the pennant line
    const baseY = bounds.y - 14;

    for (const spec of this._spectators) {
      const isCheering = excitement > spec.cheerThreshold;
      const hx = bounds.x - 10 + spec.x;

      // Bounce when cheering, subtle sway when idle
      const bounce = isCheering
        ? Math.abs(Math.sin(this.time * 5 + spec.phase)) * 3 * spec.scale
        : Math.sin(this.time * 0.5 + spec.phase) * 0.3;

      const feetY = baseY - spec.rowOffset;
      const bodyTop = feetY - spec.bodyH - bounce;
      const headCY = bodyTop - spec.headR;
      const halfW = 3 * spec.scale;

      // Torso
      ctx.fillStyle = spec.shirtColor;
      ctx.fillRect(hx - halfW, bodyTop, halfW * 2, feetY - bodyTop);

      // Head
      ctx.fillStyle = spec.skinColor;
      ctx.beginPath();
      ctx.arc(hx, headCY, spec.headR, 0, Math.PI * 2);
      ctx.fill();

      // Arms
      ctx.strokeStyle = spec.skinColor;
      ctx.lineWidth = Math.max(1, 1.2 * spec.scale);
      const shoulderY = bodyTop + 2 * spec.scale;
      const armLen = 5 * spec.scale;

      if (isCheering) {
        const wave = Math.sin(this.time * 8 + spec.phase);
        ctx.beginPath();
        ctx.moveTo(hx - halfW, shoulderY);
        ctx.lineTo(hx - halfW - armLen + wave, headCY - armLen * 0.5);
        ctx.stroke();
        ctx.beginPath();
        ctx.moveTo(hx + halfW, shoulderY);
        ctx.lineTo(hx + halfW + armLen - wave, headCY - armLen * 0.5);
        ctx.stroke();
      } else {
        ctx.beginPath();
        ctx.moveTo(hx - halfW, shoulderY);
        ctx.lineTo(hx - halfW - 1, feetY - 1);
        ctx.stroke();
        ctx.beginPath();
        ctx.moveTo(hx + halfW, shoulderY);
        ctx.lineTo(hx + halfW + 1, feetY - 1);
        ctx.stroke();
      }
    }
  }
}
