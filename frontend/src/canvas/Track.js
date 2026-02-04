const TOKEN_MARKERS = [
  { tokens: 50000, label: '50K' },
  { tokens: 100000, label: '100K' },
  { tokens: 150000, label: '150K' },
];

const PENNANT_COLORS = ['#a855f7', '#3b82f6', '#22c55e'];

export class Track {
  constructor() {
    this.trackPadding = { left: 200, right: 60, top: 60, bottom: 40 };
    this.laneHeight = 80;
    this.time = 0;
    // Pre-rendered offscreen canvases
    this._textureCanvas = null;
    this._crowdCanvas = null;
    this._lastWidth = 0;
    this._lastHeight = 0;
    this._lastLaneCount = 0;
  }

  getRequiredHeight(laneCount) {
    const maxLanes = Math.max(laneCount, 1);
    return maxLanes * this.laneHeight + this.trackPadding.top + this.trackPadding.bottom;
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
    const crowdH = 30;
    this._crowdCanvas = document.createElement('canvas');
    this._crowdCanvas.width = w;
    this._crowdCanvas.height = crowdH;
    const cc = this._crowdCanvas.getContext('2d');

    // Row of semicircle heads
    const spacing = 12;
    const count = Math.floor(w / spacing);
    for (let i = 0; i < count; i++) {
      const cx = i * spacing + spacing / 2;
      const r = 4 + Math.random() * 2;
      cc.fillStyle = `rgb(${35 + Math.random() * 10},${35 + Math.random() * 10},${50 + Math.random() * 10})`;
      cc.beginPath();
      cc.arc(cx, crowdH - 2, r, Math.PI, 0);
      cc.fill();
    }
    // Store head positions for animation
    this._crowdHeads = [];
    for (let i = 0; i < count; i++) {
      this._crowdHeads.push({ x: i * spacing + spacing / 2, baseR: 4 + Math.random() * 2 });
    }
  }

  draw(ctx, canvasWidth, canvasHeight, laneCount, maxTokens = 200000) {
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

    // Pennant flags along top edge
    this._drawPennants(ctx, bounds);

    // Crowd silhouettes along bottom (with bobbing animation)
    this._drawCrowd(ctx, bounds);

    return bounds;
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

      // Dashed line
      ctx.strokeStyle = '#444460';
      ctx.lineWidth = 1;
      ctx.setLineDash([4, 6]);
      ctx.beginPath();
      ctx.moveTo(markerX, bounds.y);
      ctx.lineTo(markerX, bounds.y + bounds.height);
      ctx.stroke();
      ctx.setLineDash([]);

      // Mile-marker flag icon
      ctx.fillStyle = '#555570';
      ctx.fillRect(markerX - 1, bounds.y - 14, 2, 10); // pole
      // Triangle flag
      ctx.beginPath();
      ctx.moveTo(markerX + 1, bounds.y - 14);
      ctx.lineTo(markerX + 8, bounds.y - 11);
      ctx.lineTo(markerX + 1, bounds.y - 8);
      ctx.closePath();
      ctx.fill();

      // Label
      ctx.fillStyle = '#aaa';
      ctx.font = 'bold 11px Courier New';
      ctx.textAlign = 'center';
      ctx.fillText(marker.label, markerX, bounds.y - 18);
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

  _drawCrowd(ctx, bounds) {
    if (!this._crowdHeads) return;
    const crowdY = bounds.y + bounds.height + 8;

    for (let i = 0; i < this._crowdHeads.length; i++) {
      const head = this._crowdHeads[i];
      const hx = bounds.x - 10 + head.x;
      const hy = crowdY;

      ctx.fillStyle = '#252535';
      ctx.beginPath();
      ctx.arc(hx, hy, head.baseR, Math.PI, 0);
      ctx.fill();
    }
  }
}
