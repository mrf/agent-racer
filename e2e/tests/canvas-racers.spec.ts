import { test, expect } from '@playwright/test';
import { gotoApp } from './helpers.js';

const TIMEOUT_WS_CONNECTION = 10_000;
const TIMEOUT_RACERS_APPEAR = 10_000;
const PIXEL_BRIGHTNESS_THRESHOLD = 10;

test.describe('Canvas racer rendering', () => {
  test('renders racers from WebSocket snapshot', async ({ page }) => {
    await gotoApp(page);

    const statusDot = page.locator('#connection-status');
    await expect(statusDot).toHaveClass(/connected/, { timeout: TIMEOUT_WS_CONNECTION });

    await expect
      .poll(
        () => page.evaluate(() => (window as any).raceCanvas?.racers?.size ?? 0),
        {
          message: 'waiting for racers to appear on canvas',
          timeout: TIMEOUT_RACERS_APPEAR
        },
      )
      .toBeGreaterThan(0);

    const sessionCount = page.locator('#session-count');
    await expect(sessionCount).not.toHaveText('0 sessions');

    const canvas = page.locator('#race-canvas');
    await expect(canvas).toBeVisible();
    await canvas.screenshot({ path: 'tests/screenshots/canvas-racers.png' });

    const hasContent = await page.evaluate((threshold) => {
      const canvasElement = document.getElementById('race-canvas') as HTMLCanvasElement;
      const ctx = canvasElement.getContext('2d');
      if (!ctx) {
        return false;
      }

      const y = Math.floor(canvasElement.height / 2);
      const imageData = ctx.getImageData(0, y, canvasElement.width, 1).data;

      for (let i = 0; i < imageData.length; i += 4) {
        const r = imageData[i];
        const g = imageData[i + 1];
        const b = imageData[i + 2];
        const a = imageData[i + 3];

        if (a > 0 && (r > threshold || g > threshold || b > threshold)) {
          return true;
        }
      }

      return false;
    }, PIXEL_BRIGHTNESS_THRESHOLD);

    expect(hasContent, 'canvas should have visible content drawn').toBe(true);
  });
});
