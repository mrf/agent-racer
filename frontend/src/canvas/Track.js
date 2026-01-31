const TOKEN_MARKERS = [
  { tokens: 50000, label: '50K' },
  { tokens: 100000, label: '100K' },
  { tokens: 150000, label: '150K' },
];

export class Track {
  constructor() {
    this.trackPadding = { left: 140, right: 60, top: 60, bottom: 40 };
    this.laneHeight = 80;
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

  draw(ctx, canvasWidth, canvasHeight, laneCount, maxTokens = 200000) {
    const bounds = this.getTrackBounds(canvasWidth, canvasHeight, laneCount);

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

    // Lane dividers
    ctx.strokeStyle = '#555570';
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

    // Start line
    ctx.strokeStyle = '#ffffff';
    ctx.lineWidth = 3;
    ctx.beginPath();
    ctx.moveTo(bounds.x, bounds.y - 10);
    ctx.lineTo(bounds.x, bounds.y + bounds.height + 10);
    ctx.stroke();

    // Checkerboard start pattern
    const checkSize = 8;
    for (let row = 0; row < Math.ceil((bounds.height + 20) / checkSize); row++) {
      for (let col = 0; col < 3; col++) {
        if ((row + col) % 2 === 0) {
          ctx.fillStyle = '#ffffff';
        } else {
          ctx.fillStyle = '#000000';
        }
        ctx.fillRect(
          bounds.x - 24 + col * checkSize,
          bounds.y - 10 + row * checkSize,
          checkSize, checkSize
        );
      }
    }

    // Finish line
    const finishX = bounds.x + bounds.width;
    ctx.strokeStyle = '#e94560';
    ctx.lineWidth = 3;
    ctx.beginPath();
    ctx.moveTo(finishX, bounds.y - 10);
    ctx.lineTo(finishX, bounds.y + bounds.height + 10);
    ctx.stroke();

    // Checkerboard finish pattern
    for (let row = 0; row < Math.ceil((bounds.height + 20) / checkSize); row++) {
      for (let col = 0; col < 3; col++) {
        if ((row + col) % 2 === 0) {
          ctx.fillStyle = '#e94560';
        } else {
          ctx.fillStyle = '#1a1a2e';
        }
        ctx.fillRect(
          finishX + 2 + col * checkSize,
          bounds.y - 10 + row * checkSize,
          checkSize, checkSize
        );
      }
    }

    // Finish label
    ctx.fillStyle = '#e94560';
    ctx.font = 'bold 12px Courier New';
    ctx.textAlign = 'center';
    ctx.fillText(`${Math.round(maxTokens / 1000)}K`, finishX, bounds.y - 16);

    // Start label
    ctx.fillStyle = '#888';
    ctx.fillText('0', bounds.x, bounds.y - 16);

    // Token markers along track
    for (const marker of TOKEN_MARKERS) {
      if (marker.tokens >= maxTokens) continue;
      const markerX = this.getPositionX(bounds, marker.tokens / maxTokens);

      ctx.strokeStyle = '#444460';
      ctx.lineWidth = 1;
      ctx.setLineDash([4, 6]);
      ctx.beginPath();
      ctx.moveTo(markerX, bounds.y);
      ctx.lineTo(markerX, bounds.y + bounds.height);
      ctx.stroke();
      ctx.setLineDash([]);

      ctx.fillStyle = '#666';
      ctx.font = '10px Courier New';
      ctx.textAlign = 'center';
      ctx.fillText(marker.label, markerX, bounds.y - 6);
    }

    return bounds;
  }
}
