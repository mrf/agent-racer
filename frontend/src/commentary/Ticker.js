/**
 * Scrolling commentary ticker bar rendered at the bottom of the canvas.
 * Messages scroll from right to left, fading out at the edges.
 */

const TICKER_HEIGHT = 28;
const SCROLL_SPEED = 60; // pixels per second
const FADE_WIDTH = 40;
const MESSAGE_GAP = 120; // gap between repeated messages
const DISPLAY_DURATION_MS = 12000; // how long a message stays before it's considered stale

export class Ticker {
  constructor() {
    this._message = null;
    this._scrollX = 0;
    this._messageWidth = 0;
    this._startTime = 0;
  }

  /**
   * Set a new message to display. Resets scroll position.
   */
  setMessage(text) {
    if (text === this._message) return;
    this._message = text;
    this._scrollX = 0; // will be initialized in draw() with canvas width
    this._startTime = Date.now();
    this._messageWidth = 0; // recalculate on next draw
  }

  /**
   * Returns the height reserved by the ticker bar.
   */
  getHeight() {
    return this._message ? TICKER_HEIGHT : 0;
  }

  /**
   * Update scroll position.
   * @param {number} dt - delta time in seconds
   */
  update(dt) {
    if (!this._message) return;

    // Auto-clear stale messages
    if (Date.now() - this._startTime > DISPLAY_DURATION_MS) {
      this._message = null;
      return;
    }

    this._scrollX -= SCROLL_SPEED * dt;
  }

  /**
   * Draw the ticker bar at the bottom of the canvas.
   * @param {CanvasRenderingContext2D} ctx
   * @param {number} canvasWidth
   * @param {number} canvasHeight
   */
  draw(ctx, canvasWidth, canvasHeight) {
    if (!this._message) return;

    const y = canvasHeight - TICKER_HEIGHT;

    ctx.clearRect(0, y, canvasWidth, TICKER_HEIGHT);

    // Background bar
    ctx.fillStyle = 'rgba(0, 0, 0, 0.7)';
    ctx.fillRect(0, y, canvasWidth, TICKER_HEIGHT);

    // Top border accent
    ctx.fillStyle = 'rgba(255, 255, 255, 0.08)';
    ctx.fillRect(0, y, canvasWidth, 1);

    // Measure message width if needed
    ctx.font = 'bold 13px Courier New';
    if (this._messageWidth === 0) {
      this._messageWidth = ctx.measureText(this._message).width;
      // Start from off-screen right
      this._scrollX = canvasWidth;
    }

    // Calculate scroll position — wrap around when fully off left
    const totalWidth = this._messageWidth + MESSAGE_GAP;
    if (this._scrollX < -totalWidth) {
      this._scrollX += totalWidth;
    }

    ctx.save();
    ctx.beginPath();
    ctx.rect(0, y, canvasWidth, TICKER_HEIGHT);
    ctx.clip();

    // Draw message (potentially twice for seamless loop)
    ctx.fillStyle = '#f0c040';
    ctx.textBaseline = 'middle';
    ctx.textAlign = 'left';

    const textY = y + TICKER_HEIGHT / 2;
    let drawX = this._scrollX;

    // Draw enough copies to fill the visible area
    for (let i = 0; i < 3; i++) {
      if (drawX < canvasWidth + 10) {
        // Edge fade: text fades near left and right edges of the bar
        const leftAlpha = Math.max(0, drawX / FADE_WIDTH);
        const rightOverflow = (drawX + this._messageWidth) - (canvasWidth - FADE_WIDTH);
        const rightAlpha = rightOverflow > 0 ? Math.max(0, 1 - rightOverflow / FADE_WIDTH) : 1;

        ctx.globalAlpha = Math.max(0.3, Math.min(leftAlpha, rightAlpha));
        ctx.fillText(this._message, drawX, textY);
      }
      drawX += totalWidth;
    }

    ctx.globalAlpha = 1;
    ctx.restore();
  }
}
