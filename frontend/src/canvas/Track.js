const PENNANT_COLORS = ['#a855f7', '#3b82f6', '#22c55e'];

const PIT_LANE_HEIGHT = 50;
const PIT_GAP = 30;
const PIT_PADDING_LEFT = 40;
const PIT_BOTTOM_PADDING = 40;
const PIT_ENTRY_OFFSET = 60;
const PIT_ENTRY_WIDTH = 40;
const PIT_COLLAPSED_HEIGHT = 14;
const PIT_COLLAPSED_PADDING = 8;

// Viewport height breakpoints for crowd visibility
const CROWD_FULL_MIN_HEIGHT = 500;
const CROWD_COMPACT_MIN_HEIGHT = 350;
const TRACK_TOP_PADDING_FULL = 60;
const TRACK_TOP_PADDING_COMPACT = 40;
const TRACK_TOP_PADDING_HIDDEN = 8;

const PARKING_LOT_LANE_HEIGHT = 45;
const PARKING_LOT_GAP = 20;
const PARKING_LOT_PADDING_LEFT = 40;
const PARKING_LOT_BOTTOM_PADDING = 40;

const TRACK_GROUP_GAP = 20;
const TRACK_GROUP_LABEL_HEIGHT = 16;
const ZONE_LABEL_HEIGHT = 22;
const ZONE_LABEL_PADDING_X = 10;

const PIT_ZONE_STYLE = Object.freeze({
  backdrop: 'rgba(60, 68, 128, 0.24)',
  tint: 'rgba(125, 134, 222, 0.12)',
  gradientEdge: '#2e3350',
  gradientMid: '#252a43',
  border: '#7c86d6',
  borderDash: [10, 6],
  rail: 'rgba(164, 172, 255, 0.65)',
  borderWidth: 1.5,
  label: 'PIT',
  labelColor: '#eef0ff',
  labelBg: 'rgba(30, 34, 60, 0.96)',
  labelBorder: 'rgba(164, 172, 255, 0.8)',
  divider: 'rgba(124, 134, 214, 0.78)',
  dividerWidth: 1,
});

const PARKING_ZONE_STYLE = Object.freeze({
  backdrop: 'rgba(45, 72, 118, 0.22)',
  tint: 'rgba(108, 168, 224, 0.1)',
  gradientEdge: '#243247',
  gradientMid: '#1d293c',
  border: '#6f9cd4',
  borderDash: [8, 6],
  rail: 'rgba(142, 196, 255, 0.58)',
  borderWidth: 1.5,
  label: 'PARKED',
  labelColor: '#e4f3ff',
  labelBg: 'rgba(22, 32, 48, 0.96)',
  labelBorder: 'rgba(142, 196, 255, 0.78)',
  divider: 'rgba(111, 156, 212, 0.72)',
  dividerWidth: 1,
});

function clampUnit(value) {
  if (!Number.isFinite(value)) return 0;
  return Math.max(0, Math.min(value, 1));
}

export class Track {
  constructor() {
    this.trackPadding = { left: 65, right: 60, top: TRACK_TOP_PADDING_FULL, bottom: 40 };
    this.laneHeight = 80;
    this.time = 0;
    // Pre-rendered offscreen canvases
    this._textureCanvas = null;
    this._spectators = null;
    this._lastWidth = 0;
    this._lastHeight = 0;
    this._lastLaneCount = 0;
    // Crowd visibility: 'full', 'compact', 'hidden'
    this._crowdMode = 'full';
  }

  updateViewport(viewportHeight) {
    let mode;
    if (viewportHeight >= CROWD_FULL_MIN_HEIGHT) {
      mode = 'full';
      this.trackPadding.top = TRACK_TOP_PADDING_FULL;
    } else if (viewportHeight >= CROWD_COMPACT_MIN_HEIGHT) {
      mode = 'compact';
      this.trackPadding.top = TRACK_TOP_PADDING_COMPACT;
    } else {
      mode = 'hidden';
      this.trackPadding.top = TRACK_TOP_PADDING_HIDDEN;
    }
    if (mode !== this._crowdMode) {
      this._crowdMode = mode;
      this._spectators = null;
    }
  }

  getRequiredHeight(laneCountOrGroups, pitLaneCount = 0, parkingLotLaneCount = 0) {
    let trackZoneHeight;
    if (Array.isArray(laneCountOrGroups)) {
      const groups = laneCountOrGroups;
      const totalLanes = groups.reduce((sum, g) => sum + Math.max(g.laneCount, 1), 0);
      const gaps = groups.length > 1 ? (groups.length - 1) * (TRACK_GROUP_GAP + TRACK_GROUP_LABEL_HEIGHT) : 0;
      trackZoneHeight = totalLanes * this.laneHeight + this.trackPadding.top + this.trackPadding.bottom + gaps;
    } else {
      const maxLanes = Math.max(laneCountOrGroups, 1);
      trackZoneHeight = maxLanes * this.laneHeight + this.trackPadding.top + this.trackPadding.bottom;
    }
    return trackZoneHeight + this.getRequiredPitHeight(pitLaneCount) + this.getRequiredParkingLotHeight(parkingLotLaneCount);
  }

  getRequiredPitHeight(pitLaneCount) {
    if (pitLaneCount <= 0) {
      return PIT_GAP + PIT_COLLAPSED_HEIGHT + PIT_COLLAPSED_PADDING;
    }
    return PIT_GAP + pitLaneCount * PIT_LANE_HEIGHT + PIT_BOTTOM_PADDING;
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
    return bounds.x + clampUnit(utilization) * bounds.width;
  }

  getTokenX(bounds, tokens, globalMaxTokens) {
    if (globalMaxTokens <= 0) return bounds.x;
    return bounds.x + clampUnit(tokens / globalMaxTokens) * bounds.width;
  }

  getMultiTrackLayout(canvasWidth, groups) {
    const trackWidth = canvasWidth - this.trackPadding.left - this.trackPadding.right;
    const layouts = [];
    let currentY = this.trackPadding.top;

    for (let i = 0; i < groups.length; i++) {
      if (i > 0) {
        currentY += TRACK_GROUP_GAP + TRACK_GROUP_LABEL_HEIGHT;
      }
      const laneCount = Math.max(groups[i].laneCount, 1);
      const trackHeight = laneCount * this.laneHeight;

      layouts.push({
        x: this.trackPadding.left,
        y: currentY,
        width: trackWidth,
        height: trackHeight,
        laneHeight: this.laneHeight,
        maxTokens: groups[i].maxTokens,
        laneCount,
      });

      currentY += trackHeight;
    }

    return layouts;
  }

  _getTrackBottomY(canvasWidth, canvasHeight, activeLaneCountOrGroups) {
    if (Array.isArray(activeLaneCountOrGroups)) {
      const layouts = this.getMultiTrackLayout(canvasWidth, activeLaneCountOrGroups);
      if (layouts.length === 0) return this.trackPadding.top;
      const last = layouts[layouts.length - 1];
      return last.y + last.height;
    }
    const bounds = this.getTrackBounds(canvasWidth, canvasHeight, activeLaneCountOrGroups);
    return bounds.y + bounds.height;
  }

  getPitBounds(canvasWidth, canvasHeight, activeLaneCountOrGroups, pitLaneCount) {
    const trackBottom = this._getTrackBottomY(canvasWidth, canvasHeight, activeLaneCountOrGroups);
    const trackWidth = canvasWidth - this.trackPadding.left - this.trackPadding.right;
    const pitTop = trackBottom + PIT_GAP;
    const pitX = this.trackPadding.left + PIT_PADDING_LEFT;
    const pitWidth = trackWidth - PIT_PADDING_LEFT;
    if (pitLaneCount <= 0) {
      return {
        x: pitX,
        y: pitTop,
        width: pitWidth,
        height: PIT_COLLAPSED_HEIGHT,
        laneHeight: PIT_COLLAPSED_HEIGHT,
      };
    }
    const pitHeight = pitLaneCount * PIT_LANE_HEIGHT;
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

  getParkingLotBounds(canvasWidth, canvasHeight, activeLaneCountOrGroups, pitLaneCount, parkingLotLaneCount) {
    if (parkingLotLaneCount <= 0) return null;
    const trackBottom = this._getTrackBottomY(canvasWidth, canvasHeight, activeLaneCountOrGroups);
    const trackWidth = canvasWidth - this.trackPadding.left - this.trackPadding.right;
    const pitHeight = this.getRequiredPitHeight(pitLaneCount);
    const lotTop = trackBottom + pitHeight + PARKING_LOT_GAP;
    const lotX = this.trackPadding.left + PARKING_LOT_PADDING_LEFT;
    const lotWidth = trackWidth - PARKING_LOT_PADDING_LEFT;
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


  draw(ctx, canvasWidth, canvasHeight, laneCount, globalMaxTokens = 200000, excitement = 0) {
    const groups = [{ maxTokens: globalMaxTokens, laneCount: Math.max(laneCount, 1) }];
    const layouts = this.drawMultiTrack(ctx, canvasWidth, canvasHeight, groups, excitement);
    return layouts.length > 0 ? layouts[0] : this.getTrackBounds(canvasWidth, canvasHeight, laneCount);
  }

  drawMultiTrack(ctx, canvasWidth, canvasHeight, groups, excitement = 0) {
    const layouts = this.getMultiTrackLayout(canvasWidth, groups);
    if (layouts.length === 0) return layouts;

    this.time += 0.016; // ~60fps tick

    // Pre-render static elements when layout changes
    const totalLanes = groups.reduce((sum, g) => sum + Math.max(g.laneCount, 1), 0);
    if (this._needsPrerender(canvasWidth, canvasHeight, totalLanes)) {
      const maxHeight = Math.max(...layouts.map(l => l.height));
      this._prerenderTexture({ width: layouts[0].width, height: maxHeight });
      this._lastWidth = canvasWidth;
      this._lastHeight = canvasHeight;
      this._lastLaneCount = totalLanes;
    }

    // Draw each track group
    for (let i = 0; i < layouts.length; i++) {
      const layout = layouts[i];

      this._drawTrackSurface(ctx, layout);
      this._drawLaneDividers(ctx, layout, layout.laneCount);
      this._drawStartLine(ctx, layout);
      this._drawFinishLine(ctx, layout, layout.maxTokens);
      this._drawTokenMarkers(ctx, layout, layout.maxTokens);

      // Group separators and labels (multi-group only)
      if (layouts.length > 1) {
        if (i > 0) {
          this._drawGroupSeparator(ctx, layouts[i - 1], layout);
        }
        this._drawGroupLabel(ctx, layout);
      }
    }

    if (this._crowdMode !== 'hidden') {
      this._drawPennants(ctx, layouts[0]);
    }

    return layouts;
  }

  _drawTrackSurface(ctx, bounds) {
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

    // Asphalt texture overlay (clipped to group bounds)
    if (this._textureCanvas) {
      ctx.save();
      ctx.beginPath();
      ctx.rect(bounds.x - 10, bounds.y - 10, bounds.width + 20, bounds.height + 20);
      ctx.clip();
      ctx.drawImage(this._textureCanvas, bounds.x - 10, bounds.y - 10);
      ctx.restore();
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
  }

  _drawLaneDividers(ctx, bounds, laneCount) {
    ctx.strokeStyle = '#ccccdd';
    ctx.lineWidth = 1;
    ctx.setLineDash([12, 8]);
    for (let i = 1; i < laneCount; i++) {
      const y = bounds.y + i * bounds.laneHeight;
      ctx.beginPath();
      ctx.moveTo(bounds.x, y);
      ctx.lineTo(bounds.x + bounds.width, y);
      ctx.stroke();
    }
    ctx.setLineDash([]);
  }

  _drawGroupLabel(ctx, layout) {
    const label = this._formatTokenLabel(layout.maxTokens);
    ctx.fillStyle = '#666';
    ctx.font = 'bold 12px Courier New';
    ctx.textAlign = 'right';
    ctx.textBaseline = 'middle';
    ctx.fillText(label, layout.x - 55, layout.y + layout.height / 2);
    ctx.textBaseline = 'alphabetic';
    ctx.textAlign = 'center';
  }

  _drawGroupSeparator(ctx, upperLayout, lowerLayout) {
    const upperBottom = upperLayout.y + upperLayout.height;
    const separatorY = (upperBottom + lowerLayout.y) / 2;
    ctx.strokeStyle = '#444';
    ctx.lineWidth = 1;
    ctx.setLineDash([6, 8]);
    ctx.beginPath();
    ctx.moveTo(upperLayout.x, separatorY);
    ctx.lineTo(upperLayout.x + upperLayout.width, separatorY);
    ctx.stroke();
    ctx.setLineDash([]);
  }

  drawPit(ctx, canvasWidth, canvasHeight, activeLaneCount, pitLaneCount) {
    const pitBounds = this.getPitBounds(canvasWidth, canvasHeight, activeLaneCount, pitLaneCount);

    if (pitLaneCount <= 0) {
      const midY = pitBounds.y + pitBounds.height / 2;
      ctx.fillStyle = PIT_ZONE_STYLE.backdrop;
      ctx.fillRect(pitBounds.x - 5, pitBounds.y - 5, pitBounds.width + 10, pitBounds.height + 10);

      ctx.strokeStyle = PIT_ZONE_STYLE.border;
      ctx.lineWidth = 2;
      ctx.setLineDash([10, 6]);
      ctx.beginPath();
      ctx.moveTo(pitBounds.x, midY);
      ctx.lineTo(pitBounds.x + pitBounds.width, midY);
      ctx.stroke();
      ctx.setLineDash([]);

      this._drawZoneLabel(ctx, pitBounds, PIT_ZONE_STYLE);
      return pitBounds;
    }

    const trackBottom = this._getTrackBottomY(canvasWidth, canvasHeight, activeLaneCount);

    // Connecting lane between track and pit at the entry point
    const entryX = this.trackPadding.left + PIT_ENTRY_OFFSET;
    const laneLeft = entryX - PIT_ENTRY_WIDTH / 2;
    const gapTop = trackBottom;
    const gapBottom = pitBounds.y;
    const gapHeight = gapBottom - gapTop;

    // Dark surface fill
    ctx.fillStyle = '#252535';
    ctx.fillRect(laneLeft, gapTop, PIT_ENTRY_WIDTH, gapHeight);

    // Dashed side borders
    ctx.strokeStyle = 'rgba(124,134,214,0.65)';
    ctx.lineWidth = 1.5;
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
    ctx.strokeStyle = 'rgba(164,172,255,0.55)';
    ctx.lineWidth = 1.5;
    for (let i = 0; i < chevronCount; i++) {
      const cy = gapTop + 8 + i * (gapHeight / chevronCount);
      ctx.beginPath();
      ctx.moveTo(entryX - 6, cy - 3);
      ctx.lineTo(entryX, cy + 3);
      ctx.lineTo(entryX + 6, cy - 3);
      ctx.stroke();
    }

    this._drawAreaSurface(ctx, pitBounds, pitLaneCount, PIT_ZONE_STYLE);

    return pitBounds;
  }

  drawParkingLot(ctx, canvasWidth, canvasHeight, activeLaneCount, pitLaneCount, parkingLotLaneCount) {
    if (parkingLotLaneCount <= 0) return null;
    const lotBounds = this.getParkingLotBounds(canvasWidth, canvasHeight, activeLaneCount, pitLaneCount, parkingLotLaneCount);

    this._drawAreaSurface(ctx, lotBounds, parkingLotLaneCount, PARKING_ZONE_STYLE);

    return lotBounds;
  }

  _drawAreaSurface(ctx, bounds, laneCount, style) {
    ctx.fillStyle = style.backdrop;
    ctx.fillRect(bounds.x - 5, bounds.y - 5, bounds.width + 10, bounds.height + 10);

    const grad = ctx.createLinearGradient(bounds.x, bounds.y, bounds.x, bounds.y + bounds.height);
    grad.addColorStop(0, style.gradientEdge);
    grad.addColorStop(0.55, style.gradientMid);
    grad.addColorStop(1, style.gradientEdge);
    ctx.fillStyle = grad;
    ctx.fillRect(bounds.x, bounds.y, bounds.width, bounds.height);

    ctx.fillStyle = style.tint;
    ctx.fillRect(bounds.x, bounds.y, bounds.width, bounds.height);

    ctx.strokeStyle = style.rail;
    ctx.lineWidth = 2;
    ctx.setLineDash([]);
    ctx.beginPath();
    ctx.moveTo(bounds.x - 2, bounds.y);
    ctx.lineTo(bounds.x + bounds.width + 2, bounds.y);
    ctx.stroke();
    ctx.beginPath();
    ctx.moveTo(bounds.x - 2, bounds.y + bounds.height);
    ctx.lineTo(bounds.x + bounds.width + 2, bounds.y + bounds.height);
    ctx.stroke();

    ctx.strokeStyle = style.border;
    ctx.lineWidth = style.borderWidth;
    ctx.setLineDash(style.borderDash);
    ctx.strokeRect(bounds.x, bounds.y, bounds.width, bounds.height);
    ctx.setLineDash([]);

    this._drawZoneLabel(ctx, bounds, style);

    if (laneCount > 1) {
      ctx.strokeStyle = style.divider;
      ctx.lineWidth = style.dividerWidth;
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

  _drawZoneLabel(ctx, bounds, style) {
    const labelWidth = Math.max(56, Math.ceil(ctx.measureText(style.label).width) + ZONE_LABEL_PADDING_X * 2);
    const labelX = bounds.x - 12 - labelWidth;
    const labelY = bounds.y + bounds.height / 2 - ZONE_LABEL_HEIGHT / 2;

    ctx.fillStyle = style.labelBg;
    ctx.fillRect(labelX, labelY, labelWidth, ZONE_LABEL_HEIGHT);
    ctx.strokeStyle = style.labelBorder;
    ctx.lineWidth = 1;
    ctx.setLineDash([]);
    ctx.strokeRect(labelX, labelY, labelWidth, ZONE_LABEL_HEIGHT);

    ctx.fillStyle = style.labelColor;
    ctx.font = 'bold 12px Courier New';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText(style.label, labelX + labelWidth / 2, labelY + ZONE_LABEL_HEIGHT / 2);
    ctx.textBaseline = 'alphabetic';
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

    // Finish line (constrained to track surface)
    ctx.strokeStyle = '#e94560';
    ctx.lineWidth = 3;
    ctx.beginPath();
    ctx.moveTo(finishX, bounds.y);
    ctx.lineTo(finishX, bounds.y + bounds.height);
    ctx.stroke();

    // Checkerboard pattern constrained to track surface
    const checkSize = 12;
    const cols = 4;
    const checkerLeft = finishX - cols * checkSize - 2;
    const checkerCenterX = checkerLeft + cols * checkSize / 2;
    const rowCount = Math.ceil(bounds.height / checkSize);
    for (let row = 0; row < rowCount; row++) {
      for (let col = 0; col < cols; col++) {
        ctx.fillStyle = (row + col) % 2 === 0 ? '#e94560' : '#1a1a2e';
        ctx.fillRect(
          checkerLeft + col * checkSize,
          bounds.y + row * checkSize,
          checkSize, checkSize
        );
      }
    }

    // Labels above the track (top to bottom: flag, "FINISH", token count)
    this._drawCheckerFlag(ctx, checkerCenterX, bounds.y - 38, 10);
    ctx.fillStyle = '#e94560';
    ctx.textAlign = 'center';
    ctx.font = 'bold 11px Courier New';
    ctx.fillText('FINISH', checkerCenterX, bounds.y - 28);
    ctx.font = 'bold 12px Courier New';
    ctx.fillText(this._formatTokenLabel(maxTokens), finishX, bounds.y - 16);
  }

  _formatTokenLabel(tokens) {
    if (tokens >= 1_000_000) {
      const m = tokens / 1_000_000;
      return Number.isInteger(m) ? `${m}M` : `${m.toFixed(1)}M`;
    }
    if (tokens >= 1000) return `${Math.round(tokens / 1000)}K`;
    return `${tokens}`;
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

  _drawTokenMarkers(ctx, bounds, globalMaxTokens) {
    const markers = this._computeTokenMarkers(globalMaxTokens);
    for (const marker of markers) {
      if (marker.tokens >= globalMaxTokens) continue;
      const markerX = this.getTokenX(bounds, marker.tokens, globalMaxTokens);

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

  _computeTokenMarkers(globalMaxTokens) {
    // Choose a step that gives 2-6 markers for the current scale.
    const steps = [25000, 50000, 100000, 200000, 250000, 500000, 1000000];
    const step = steps.find(s => {
      const count = Math.floor(globalMaxTokens / s);
      return count >= 2 && count <= 6;
    }) ?? 50000;
    const markers = [];
    for (let t = step; t < globalMaxTokens; t += step) {
      markers.push({ tokens: t, label: this._formatTokenLabel(t) });
    }
    return markers;
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

}
