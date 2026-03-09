import { describe, it, expect } from 'vitest';
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
});
