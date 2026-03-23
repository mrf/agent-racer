const PIT_LANE_HEIGHT = 50;
const PIT_GAP = 30;
const PIT_PADDING_LEFT = 40;
const PIT_BOTTOM_PADDING = 40;
const PIT_ENTRY_OFFSET = 60;
const PIT_ENTRY_WIDTH = 40;
const PIT_COLLAPSED_HEIGHT = 14;
const PIT_COLLAPSED_PADDING = 8;

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

const CANOPY_COLORS = ['#2d6b2d', '#3a7a3a', '#4a8a4a'];

const SKIN_TONES = ['#f5d0a9', '#d4a574', '#c68642', '#8d5524', '#e0ac69', '#f1c27d'];
const SHIRT_COLORS = [
  '#e94560', '#3b82f6', '#22c55e', '#a855f7', '#f59e0b',
  '#06b6d4', '#ec4899', '#f97316', '#84cc16', '#eee',
];
const CROWD_ROWS = [
  { spacing: 13, offsetY: 14, scale: 0.65, xShift: 0 },
  { spacing: 13, offsetY: 0, scale: 0.85, xShift: 6.5 },
];

function clampUnit(value) {
  if (!Number.isFinite(value)) return 0;
  return Math.max(0, Math.min(value, 1));
}

export class FootraceTrack {
  constructor() {
    this.trackPadding = { left: 65, right: 60, top: TRACK_TOP_PADDING_FULL, bottom: 40 };
    this.laneHeight = 80;
    this.time = 0;
    this._textureCanvas = null;
    this._spectators = null;
    this._lastWidth = 0;
    this._lastHeight = 0;
    this._lastLaneCount = 0;
    this._crowdMode = 'full';
  }

  // ──── viewport ────

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

  // ──── geometry (identical API to Track) ────

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

  // ──── prerender helpers ────

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

    // Grass-grain texture (subtle horizontal streaks)
    tc.strokeStyle = 'rgba(255,255,255,0.04)';
    tc.lineWidth = 1;
    for (let y = 0; y < h; y += 6) {
      tc.beginPath();
      tc.moveTo(0, y + Math.random() * 2);
      tc.lineTo(w, y + Math.random() * 2);
      tc.stroke();
    }
  }

  _prerenderCrowd(bounds) {
    this._spectators = [];
    if (this._crowdMode === 'hidden') return;

    const rows = this._crowdMode === 'compact' ? [CROWD_ROWS[1]] : CROWD_ROWS;
    const w = bounds.width + 20;

    for (const row of rows) {
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

  // ──── drawing ────

  draw(ctx, canvasWidth, canvasHeight, laneCount, globalMaxTokens = 200000, excitement = 0) {
    const groups = [{ maxTokens: globalMaxTokens, laneCount: Math.max(laneCount, 1) }];
    const layouts = this.drawMultiTrack(ctx, canvasWidth, canvasHeight, groups, excitement);
    return layouts.length > 0 ? layouts[0] : this.getTrackBounds(canvasWidth, canvasHeight, laneCount);
  }

  drawMultiTrack(ctx, canvasWidth, canvasHeight, groups, excitement = 0) {
    const layouts = this.getMultiTrackLayout(canvasWidth, groups);
    if (layouts.length === 0) return layouts;

    this.time += 0.016;

    const totalLanes = groups.reduce((sum, g) => sum + Math.max(g.laneCount, 1), 0);
    if (this._needsPrerender(canvasWidth, canvasHeight, totalLanes)) {
      const maxHeight = Math.max(...layouts.map(l => l.height));
      this._prerenderTexture({ width: layouts[0].width, height: maxHeight });
      this._prerenderCrowd(layouts[0]);
      this._lastWidth = canvasWidth;
      this._lastHeight = canvasHeight;
      this._lastLaneCount = totalLanes;
    }

    for (let i = 0; i < layouts.length; i++) {
      const layout = layouts[i];
      this._drawTrackSurface(ctx, layout);
      this._drawLaneDividers(ctx, layout, layout.laneCount);
      this._drawStartLine(ctx, layout);
      this._drawFinishLine(ctx, layout, layout.maxTokens);
      this._drawMileMarkers(ctx, layout, layout.maxTokens);

      if (layouts.length > 1) {
        if (i > 0) {
          this._drawGroupSeparator(ctx, layouts[i - 1], layout);
        }
        this._drawGroupLabel(ctx, layout);
      }
    }

    this._drawCrowd(ctx, layouts[0], excitement);
    if (this._crowdMode !== 'hidden') {
      this._drawTreeLine(ctx, layouts[0]);
    }

    return layouts;
  }

  _drawTrackSurface(ctx, bounds) {
    // Dirt/grass running track
    ctx.fillStyle = '#3d5c3a';
    ctx.fillRect(bounds.x - 10, bounds.y - 10, bounds.width + 20, bounds.height + 20);

    const trackGrad = ctx.createLinearGradient(bounds.x, bounds.y, bounds.x, bounds.y + bounds.height);
    trackGrad.addColorStop(0, '#4a6e46');
    trackGrad.addColorStop(0.5, '#3d5c3a');
    trackGrad.addColorStop(1, '#4a6e46');
    ctx.fillStyle = trackGrad;
    ctx.fillRect(bounds.x, bounds.y, bounds.width, bounds.height);

    // Texture overlay
    if (this._textureCanvas) {
      ctx.save();
      ctx.beginPath();
      ctx.rect(bounds.x - 10, bounds.y - 10, bounds.width + 20, bounds.height + 20);
      ctx.clip();
      ctx.drawImage(this._textureCanvas, bounds.x - 10, bounds.y - 10);
      ctx.restore();
    }

    // Track edge shadows
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

    // White edge lines (lane ropes)
    ctx.strokeStyle = '#ddd';
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
    // Lane rope lines (solid white, thin)
    ctx.strokeStyle = 'rgba(255,255,255,0.25)';
    ctx.lineWidth = 1;
    ctx.setLineDash([]);
    for (let i = 1; i < laneCount; i++) {
      const y = bounds.y + i * bounds.laneHeight;
      ctx.beginPath();
      ctx.moveTo(bounds.x, y);
      ctx.lineTo(bounds.x + bounds.width, y);
      ctx.stroke();
    }
  }

  _drawGroupLabel(ctx, layout) {
    const label = this._formatTokenLabel(layout.maxTokens);
    ctx.fillStyle = '#7a9a76';
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
    ctx.strokeStyle = '#5a7a56';
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
      ctx.strokeStyle = '#5a7a56';
      ctx.lineWidth = 1;
      ctx.setLineDash([6, 8]);
      ctx.beginPath();
      ctx.moveTo(pitBounds.x, midY);
      ctx.lineTo(pitBounds.x + pitBounds.width, midY);
      ctx.stroke();
      ctx.setLineDash([]);

      ctx.fillStyle = '#5a7a56';
      ctx.font = 'bold 11px Courier New';
      ctx.textAlign = 'right';
      ctx.textBaseline = 'middle';
      ctx.fillText('REST', pitBounds.x - 10, midY);
      ctx.textBaseline = 'alphabetic';
      ctx.textAlign = 'center';
      return pitBounds;
    }

    const trackBottom = this._getTrackBottomY(canvasWidth, canvasHeight, activeLaneCount);

    // Connecting path between track and rest area
    const entryX = this.trackPadding.left + PIT_ENTRY_OFFSET;
    const laneLeft = entryX - PIT_ENTRY_WIDTH / 2;
    const gapTop = trackBottom;
    const gapBottom = pitBounds.y;
    const gapHeight = gapBottom - gapTop;

    ctx.fillStyle = '#2e4a2b';
    ctx.fillRect(laneLeft, gapTop, PIT_ENTRY_WIDTH, gapHeight);

    ctx.strokeStyle = 'rgba(100,140,100,0.5)';
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

    // Down-arrow footprints in the path
    ctx.strokeStyle = 'rgba(100,140,100,0.4)';
    ctx.lineWidth = 1.5;
    const chevronCount = Math.max(1, Math.floor(gapHeight / 14));
    for (let i = 0; i < chevronCount; i++) {
      const cy = gapTop + 8 + i * (gapHeight / chevronCount);
      ctx.beginPath();
      ctx.moveTo(entryX - 6, cy - 3);
      ctx.lineTo(entryX, cy + 3);
      ctx.lineTo(entryX + 6, cy - 3);
      ctx.stroke();
    }

    this._drawAreaSurface(ctx, pitBounds, pitLaneCount, {
      bg: '#2e4a2b',
      gradientEdge: '#365535',
      gradientMid: '#2e4a2b',
      border: '#5a7a56',
      borderDash: [8, 6],
      label: 'REST',
      labelColor: '#5a7a56',
      divider: '#3a5a36',
    });

    return pitBounds;
  }

  drawParkingLot(ctx, canvasWidth, canvasHeight, activeLaneCount, pitLaneCount, parkingLotLaneCount) {
    if (parkingLotLaneCount <= 0) return null;
    const lotBounds = this.getParkingLotBounds(canvasWidth, canvasHeight, activeLaneCount, pitLaneCount, parkingLotLaneCount);

    this._drawAreaSurface(ctx, lotBounds, parkingLotLaneCount, {
      bg: '#1e3a1b',
      gradientEdge: '#264a22',
      gradientMid: '#1e3a1b',
      border: '#4a6a46',
      borderDash: [6, 8],
      label: 'FINISH',
      labelColor: '#4a6a46',
      divider: '#2a4a26',
    });

    return lotBounds;
  }

  _drawAreaSurface(ctx, bounds, laneCount, style) {
    ctx.fillStyle = style.bg;
    ctx.fillRect(bounds.x - 5, bounds.y - 5, bounds.width + 10, bounds.height + 10);

    const grad = ctx.createLinearGradient(bounds.x, bounds.y, bounds.x, bounds.y + bounds.height);
    grad.addColorStop(0, style.gradientEdge);
    grad.addColorStop(0.5, style.gradientMid);
    grad.addColorStop(1, style.gradientEdge);
    ctx.fillStyle = grad;
    ctx.fillRect(bounds.x, bounds.y, bounds.width, bounds.height);

    ctx.strokeStyle = style.border;
    ctx.lineWidth = 1;
    ctx.setLineDash(style.borderDash);
    ctx.strokeRect(bounds.x, bounds.y, bounds.width, bounds.height);
    ctx.setLineDash([]);

    ctx.fillStyle = style.labelColor;
    ctx.font = 'bold 14px Courier New';
    ctx.textAlign = 'right';
    ctx.textBaseline = 'middle';
    ctx.fillText(style.label, bounds.x - 10, bounds.y + bounds.height / 2);
    ctx.textBaseline = 'alphabetic';
    ctx.textAlign = 'center';

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
    // Starting blocks
    ctx.strokeStyle = '#ffffff';
    ctx.lineWidth = 3;
    ctx.beginPath();
    ctx.moveTo(bounds.x, bounds.y - 10);
    ctx.lineTo(bounds.x, bounds.y + bounds.height + 10);
    ctx.stroke();

    // Starting block markers (small rectangles per lane)
    for (let row = 0; row < Math.ceil((bounds.height + 20) / bounds.laneHeight); row++) {
      const blockY = bounds.y + row * bounds.laneHeight + bounds.laneHeight / 2 - 6;
      ctx.fillStyle = '#fff';
      ctx.fillRect(bounds.x - 20, blockY, 16, 12);
      ctx.strokeStyle = '#888';
      ctx.lineWidth = 0.5;
      ctx.strokeRect(bounds.x - 20, blockY, 16, 12);
    }

    // Start label
    ctx.fillStyle = '#8a8';
    ctx.font = 'bold 12px Courier New';
    ctx.textAlign = 'center';
    ctx.fillText('START', bounds.x, bounds.y - 16);
  }

  _drawFinishLine(ctx, bounds, maxTokens) {
    const finishX = bounds.x + bounds.width;

    // Finish tape
    ctx.strokeStyle = '#e94560';
    ctx.lineWidth = 3;
    ctx.beginPath();
    ctx.moveTo(finishX, bounds.y);
    ctx.lineTo(finishX, bounds.y + bounds.height);
    ctx.stroke();

    // Finish banner (alternating red/white)
    const bannerW = 48;
    const bannerH = bounds.height;
    const bannerLeft = finishX - bannerW - 2;
    const stripeH = 8;
    const stripeCount = Math.ceil(bannerH / stripeH);
    for (let i = 0; i < stripeCount; i++) {
      ctx.fillStyle = i % 2 === 0 ? '#e94560' : '#fff';
      ctx.fillRect(bannerLeft, bounds.y + i * stripeH, bannerW, stripeH);
    }

    // Banner text
    this._drawFinishBanner(ctx, bannerLeft + bannerW / 2, bounds.y - 38, 10);
    ctx.fillStyle = '#e94560';
    ctx.textAlign = 'center';
    ctx.font = 'bold 11px Courier New';
    ctx.fillText('FINISH', bannerLeft + bannerW / 2, bounds.y - 28);
    ctx.font = 'bold 12px Courier New';
    ctx.fillText(this._formatTokenLabel(maxTokens), finishX, bounds.y - 16);
  }

  _drawFinishBanner(ctx, x, y, size) {
    // Small pennant/flag
    ctx.strokeStyle = '#888';
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(x - size / 2, y + size);
    ctx.lineTo(x - size / 2, y - 2);
    ctx.stroke();

    // Triangular banner
    ctx.fillStyle = '#e94560';
    ctx.beginPath();
    ctx.moveTo(x - size / 2, y - 2);
    ctx.lineTo(x + size / 2, y + size / 3);
    ctx.lineTo(x - size / 2, y + size * 0.7);
    ctx.closePath();
    ctx.fill();
  }

  _drawMileMarkers(ctx, bounds, globalMaxTokens) {
    const markers = this._computeTokenMarkers(globalMaxTokens);
    for (const marker of markers) {
      if (marker.tokens >= globalMaxTokens) continue;
      const markerX = this.getTokenX(bounds, marker.tokens, globalMaxTokens);

      // Small post
      ctx.strokeStyle = 'rgba(80,120,80,0.6)';
      ctx.lineWidth = 1;
      ctx.setLineDash([4, 6]);
      ctx.beginPath();
      ctx.moveTo(markerX, bounds.y);
      ctx.lineTo(markerX, bounds.y + bounds.height);
      ctx.stroke();
      ctx.setLineDash([]);

      // Mile marker label
      ctx.fillStyle = '#8a8';
      ctx.font = 'bold 12px Courier New';
      ctx.textAlign = 'center';
      ctx.fillText(marker.label, markerX, bounds.y + 16);
    }
  }

  _formatTokenLabel(tokens) {
    if (tokens >= 1_000_000) {
      const m = tokens / 1_000_000;
      return Number.isInteger(m) ? `${m}M` : `${m.toFixed(1)}M`;
    }
    if (tokens >= 1000) return `${Math.round(tokens / 1000)}K`;
    return `${tokens}`;
  }

  _computeTokenMarkers(globalMaxTokens) {
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

  _getTreeCanopyRadius(index) {
    return 3 + (index * 2) % 5;
  }

  _drawTreeLine(ctx, bounds) {
    // Small trees/bushes above the track instead of pennants
    const spacing = 40;
    const count = Math.floor(bounds.width / spacing);
    const lineY = bounds.y - 12;

    for (let i = 0; i < count; i++) {
      const tx = bounds.x + i * spacing + spacing / 2;
      const wave = Math.sin(this.time * 1.5 + i * 0.7) * 1;
      const canopyRadius = this._getTreeCanopyRadius(i);
      const canopyY = lineY - 6 + wave;
      const trunkBaseY = lineY + 6;
      const trunkHalfWidth = Math.max(2, canopyRadius - 3);

      // Slight triangular trunk peeking out from the canopy.
      ctx.fillStyle = '#6b4423';
      ctx.beginPath();
      ctx.moveTo(tx, canopyY + canopyRadius - 1);
      ctx.lineTo(tx - trunkHalfWidth, trunkBaseY);
      ctx.lineTo(tx + trunkHalfWidth, trunkBaseY);
      ctx.closePath();
      ctx.fill();

      // Canopy
      ctx.fillStyle = CANOPY_COLORS[i % 3];
      ctx.globalAlpha = 0.8;
      ctx.beginPath();
      ctx.arc(tx, canopyY, canopyRadius, 0, Math.PI * 2);
      ctx.fill();
      ctx.globalAlpha = 1.0;
    }
  }

  _drawCrowd(ctx, bounds, excitement) {
    if (!this._spectators || this._crowdMode === 'hidden') return;

    const baseY = bounds.y - 14;

    for (const spec of this._spectators) {
      const isCheering = excitement > spec.cheerThreshold;
      const hx = bounds.x - 10 + spec.x;

      const bounce = isCheering
        ? Math.abs(Math.sin(this.time * 5 + spec.phase)) * 3 * spec.scale
        : Math.sin(this.time * 0.5 + spec.phase) * 0.3;

      const feetY = baseY - spec.rowOffset;
      const bodyTop = feetY - spec.bodyH - bounce;
      const headCY = bodyTop - spec.headR;
      const halfW = 3 * spec.scale;

      ctx.fillStyle = spec.shirtColor;
      ctx.fillRect(hx - halfW, bodyTop, halfW * 2, feetY - bodyTop);

      ctx.fillStyle = spec.skinColor;
      ctx.beginPath();
      ctx.arc(hx, headCY, spec.headR, 0, Math.PI * 2);
      ctx.fill();

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
