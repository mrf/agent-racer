/**
 * Pixel-art commentator character with speech bubble overlay.
 * Drawn in the top-right corner of the canvas.
 */

const BUBBLE_MAX_WIDTH = 320;
const BUBBLE_PADDING = 10;
const BUBBLE_RADIUS = 8;
const BUBBLE_TAIL_SIZE = 8;
const BUBBLE_FONT = '12px Courier New';
const BUBBLE_LINE_HEIGHT = 16;
const DISPLAY_DURATION_MS = 8000;
const FADE_DURATION_MS = 500;

// Character dimensions (pixel art scaled up)
const CHAR_SCALE = 3;
const CHAR_WIDTH = 12 * CHAR_SCALE;  // 36px
const CHAR_HEIGHT = 16 * CHAR_SCALE; // 48px
const MARGIN_RIGHT = 16;
const MARGIN_TOP = 12;

export class Announcer {
  constructor() {
    this._message = null;
    this._startTime = 0;
    this._alpha = 0;
    this._lines = [];
  }

  /**
   * Set a new message to display.
   */
  setMessage(text) {
    this._message = text;
    this._startTime = Date.now();
    this._alpha = 1;
    this._lines = []; // recalculate on draw
  }

  /**
   * Update fade state.
   */
  update() {
    if (!this._message) return;

    const elapsed = Date.now() - this._startTime;
    if (elapsed > DISPLAY_DURATION_MS) {
      const fadeProgress = (elapsed - DISPLAY_DURATION_MS) / FADE_DURATION_MS;
      this._alpha = Math.max(0, 1 - fadeProgress);
      if (this._alpha <= 0) {
        this._message = null;
      }
    }
  }

  /**
   * Draw the announcer character and speech bubble.
   * @param {CanvasRenderingContext2D} ctx
   * @param {number} canvasWidth
   */
  draw(ctx, canvasWidth) {
    if (!this._message || this._alpha <= 0) return;

    ctx.save();
    ctx.globalAlpha = this._alpha;

    // Character position (top-right)
    const charX = canvasWidth - MARGIN_RIGHT - CHAR_WIDTH;
    const charY = MARGIN_TOP;

    // Draw character
    this._drawCharacter(ctx, charX, charY);

    // Word-wrap the message
    if (this._lines.length === 0) {
      this._lines = this._wrapText(ctx, this._message, BUBBLE_MAX_WIDTH - BUBBLE_PADDING * 2);
    }

    // Draw speech bubble to the left of the character
    const bubbleHeight = this._lines.length * BUBBLE_LINE_HEIGHT + BUBBLE_PADDING * 2;
    const bubbleWidth = Math.min(BUBBLE_MAX_WIDTH, this._measureBubbleWidth(ctx) + BUBBLE_PADDING * 2);
    const bubbleX = charX - bubbleWidth - BUBBLE_TAIL_SIZE - 4;
    const bubbleY = charY + 4;

    this._drawBubble(ctx, bubbleX, bubbleY, bubbleWidth, bubbleHeight, charY + CHAR_HEIGHT / 3);

    // Draw text inside bubble
    ctx.fillStyle = '#111';
    ctx.font = BUBBLE_FONT;
    ctx.textAlign = 'left';
    ctx.textBaseline = 'top';

    for (let i = 0; i < this._lines.length; i++) {
      ctx.fillText(this._lines[i], bubbleX + BUBBLE_PADDING, bubbleY + BUBBLE_PADDING + i * BUBBLE_LINE_HEIGHT);
    }

    ctx.restore();
  }

  _drawBubble(ctx, x, y, w, h, tailTargetY) {
    const r = BUBBLE_RADIUS;

    ctx.fillStyle = 'rgba(255, 255, 255, 0.95)';
    ctx.strokeStyle = 'rgba(0, 0, 0, 0.3)';
    ctx.lineWidth = 1;

    // Rounded rectangle
    ctx.beginPath();
    ctx.moveTo(x + r, y);
    ctx.lineTo(x + w - r, y);
    ctx.quadraticCurveTo(x + w, y, x + w, y + r);
    ctx.lineTo(x + w, y + h - r);
    ctx.quadraticCurveTo(x + w, y + h, x + w - r, y + h);
    ctx.lineTo(x + r, y + h);
    ctx.quadraticCurveTo(x, y + h, x, y + h - r);
    ctx.lineTo(x, y + r);
    ctx.quadraticCurveTo(x, y, x + r, y);
    ctx.closePath();
    ctx.fill();
    ctx.stroke();

    // Speech bubble tail (triangle pointing right toward character)
    const tailBaseY = Math.min(y + h - 10, Math.max(y + 10, tailTargetY));
    ctx.beginPath();
    ctx.moveTo(x + w, tailBaseY - 5);
    ctx.lineTo(x + w + BUBBLE_TAIL_SIZE, tailBaseY);
    ctx.lineTo(x + w, tailBaseY + 5);
    ctx.closePath();
    ctx.fill();
    ctx.stroke();
    // Cover the inner edge of the tail
    ctx.fillRect(x + w - 1, tailBaseY - 4, 2, 8);
  }

  _drawCharacter(ctx, x, y) {
    const s = CHAR_SCALE;

    // Simple pixel-art commentator (megaphone guy)
    // Hat
    ctx.fillStyle = '#e94560';
    ctx.fillRect(x + 2 * s, y, 8 * s, 2 * s);
    ctx.fillRect(x + 3 * s, y - 1 * s, 6 * s, 1 * s);

    // Head
    ctx.fillStyle = '#fbbf77';
    ctx.fillRect(x + 3 * s, y + 2 * s, 6 * s, 5 * s);

    // Eyes
    ctx.fillStyle = '#222';
    ctx.fillRect(x + 4 * s, y + 4 * s, 1 * s, 1 * s);
    ctx.fillRect(x + 7 * s, y + 4 * s, 1 * s, 1 * s);

    // Mouth (open, shouting)
    ctx.fillStyle = '#c44';
    ctx.fillRect(x + 5 * s, y + 6 * s, 2 * s, 1 * s);

    // Body (vest)
    ctx.fillStyle = '#2563eb';
    ctx.fillRect(x + 3 * s, y + 7 * s, 6 * s, 5 * s);

    // Vest stripes
    ctx.fillStyle = '#1d4ed8';
    ctx.fillRect(x + 5 * s, y + 7 * s, 2 * s, 5 * s);

    // Arms
    ctx.fillStyle = '#fbbf77';
    // Left arm (holding megaphone)
    ctx.fillRect(x + 1 * s, y + 8 * s, 2 * s, 1 * s);
    ctx.fillRect(x + 0 * s, y + 7 * s, 2 * s, 1 * s);

    // Megaphone
    ctx.fillStyle = '#f0c040';
    ctx.fillRect(x - 2 * s, y + 6 * s, 3 * s, 1 * s);
    ctx.fillRect(x - 3 * s, y + 5 * s, 4 * s, 1 * s);
    ctx.fillRect(x - 3 * s, y + 7 * s, 4 * s, 1 * s);

    // Right arm
    ctx.fillStyle = '#fbbf77';
    ctx.fillRect(x + 9 * s, y + 8 * s, 2 * s, 1 * s);

    // Legs
    ctx.fillStyle = '#333';
    ctx.fillRect(x + 3 * s, y + 12 * s, 2 * s, 3 * s);
    ctx.fillRect(x + 7 * s, y + 12 * s, 2 * s, 3 * s);

    // Shoes
    ctx.fillStyle = '#555';
    ctx.fillRect(x + 2 * s, y + 15 * s, 3 * s, 1 * s);
    ctx.fillRect(x + 7 * s, y + 15 * s, 3 * s, 1 * s);
  }

  _wrapText(ctx, text, maxWidth) {
    ctx.font = BUBBLE_FONT;
    const words = text.split(' ');
    const lines = [];
    let currentLine = '';

    for (const word of words) {
      const test = currentLine ? currentLine + ' ' + word : word;
      if (ctx.measureText(test).width > maxWidth && currentLine) {
        lines.push(currentLine);
        currentLine = word;
      } else {
        currentLine = test;
      }
    }
    if (currentLine) lines.push(currentLine);
    return lines;
  }

  _measureBubbleWidth(ctx) {
    ctx.font = BUBBLE_FONT;
    let maxWidth = 0;
    for (const line of this._lines) {
      const w = ctx.measureText(line).width;
      if (w > maxWidth) maxWidth = w;
    }
    return maxWidth;
  }
}
