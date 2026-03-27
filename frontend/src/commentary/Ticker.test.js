import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { Ticker } from './Ticker.js';

function createTickerCtx(canvasWidth, canvasHeight) {
  let currentRect = null;
  let currentAlpha = 1;
  const visibleTexts = [];

  function overlaps(aStart, aEnd, bStart, bEnd) {
    return aStart < bEnd && aEnd > bStart;
  }

  return {
    fillStyle: '',
    font: '',
    textBaseline: 'alphabetic',
    textAlign: 'left',
    beginPath() {},
    clip() {},
    fillRect() {},
    rect(x, y, width, height) {
      currentRect = { x, y, width, height };
    },
    save() {},
    restore() {},
    measureText(text) {
      return { width: text.length * 10 };
    },
    clearRect(x, y, width, height) {
      for (let i = visibleTexts.length - 1; i >= 0; i--) {
        const entry = visibleTexts[i];
        const textBottom = entry.y + 13;
        if (
          overlaps(entry.x, entry.x + entry.width, x, x + width) &&
          overlaps(entry.y - 13, textBottom, y, y + height)
        ) {
          visibleTexts.splice(i, 1);
        }
      }
    },
    fillText(text, x, y) {
      const width = this.measureText(text).width;
      const withinCanvas = overlaps(x, x + width, 0, canvasWidth) && overlaps(y - 13, y + 13, 0, canvasHeight);
      if (!withinCanvas || currentAlpha <= 0) return;
      visibleTexts.push({ text, x, y, width, alpha: currentAlpha });
    },
    get globalAlpha() {
      return currentAlpha;
    },
    set globalAlpha(value) {
      currentAlpha = value;
    },
    getVisibleTexts() {
      return visibleTexts.map((entry) => entry.text);
    },
    getTextPositions(text) {
      return visibleTexts.filter((entry) => entry.text === text).map((entry) => entry.x);
    },
    get clipRect() {
      return currentRect;
    },
  };
}

describe('Ticker', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-03-07T12:00:00Z'));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  // --- draw tests (existing) ---

  it('clears stale loop copies before drawing a replacement message', () => {
    const ticker = new Ticker();
    const ctx = createTickerCtx(260, 100);

    ticker.setMessage('old');
    ticker._messageWidth = 30;
    ticker._scrollX = -10;
    ticker.draw(ctx, 260, 100);

    expect(ctx.getVisibleTexts()).toEqual(['old', 'old']);

    ticker.setMessage('new');
    ticker.draw(ctx, 260, 100);
    ticker.update(1);
    ticker.draw(ctx, 260, 100);

    expect(ctx.getVisibleTexts()).toEqual(['new']);
  });

  it('still draws repeated copies for seamless looping', () => {
    const ticker = new Ticker();
    const ctx = createTickerCtx(260, 100);

    ticker.setMessage('lap');
    ticker._messageWidth = 30;
    ticker._scrollX = -10;
    ticker.draw(ctx, 260, 100);

    expect(ctx.getTextPositions('lap')).toEqual([-10, 140]);
    expect(ctx.clipRect).toEqual({ x: 0, y: 72, width: 260, height: 28 });
  });

  // --- setMessage ---

  it('ignores duplicate messages', () => {
    const ticker = new Ticker();
    ticker.setMessage('same');
    const firstStart = ticker._startTime;

    vi.advanceTimersByTime(1000);
    ticker.setMessage('same');

    // startTime should not have been reset
    expect(ticker._startTime).toBe(firstStart);
  });

  it('accepts a different message and resets scroll', () => {
    const ticker = new Ticker();
    ticker.setMessage('first');
    ticker._scrollX = -100;

    ticker.setMessage('second');
    expect(ticker._message).toBe('second');
    expect(ticker._scrollX).toBe(0);
    expect(ticker._messageWidth).toBe(0);
  });

  // --- getHeight ---

  it('returns 0 when no message is set', () => {
    const ticker = new Ticker();
    expect(ticker.getHeight()).toBe(0);
  });

  it('returns ticker height when a message is active', () => {
    const ticker = new Ticker();
    ticker.setMessage('hello');
    expect(ticker.getHeight()).toBe(28);
  });

  // --- update ---

  it('is a no-op when no message is set', () => {
    const ticker = new Ticker();
    ticker.update(0.016);
    expect(ticker._message).toBeNull();
    expect(ticker._scrollX).toBe(0);
  });

  it('advances scroll position based on delta time', () => {
    const ticker = new Ticker();
    ticker.setMessage('scrolling');

    ticker.update(1); // 1 second at 60 px/s
    expect(ticker._scrollX).toBe(-60);
  });

  it('accumulates scroll over multiple updates', () => {
    const ticker = new Ticker();
    ticker.setMessage('scrolling');

    ticker.update(0.5);
    ticker.update(0.5);
    expect(ticker._scrollX).toBe(-60);
  });

  it('clears stale message after display duration', () => {
    const ticker = new Ticker();
    ticker.setMessage('stale');

    // Advance past 12s display duration
    vi.advanceTimersByTime(13000);
    ticker.update(0);

    expect(ticker._message).toBeNull();
  });

  it('keeps message alive within display duration', () => {
    const ticker = new Ticker();
    ticker.setMessage('fresh');

    vi.advanceTimersByTime(11000);
    ticker.update(0);

    expect(ticker._message).toBe('fresh');
  });
});
