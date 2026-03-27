import { test, expect } from '@playwright/test';
import { waitForConnection, waitForRacers, gotoApp } from './helpers.js';

/** Brightness threshold for detecting drawn content on the minimap canvas. */
const PIXEL_BRIGHTNESS_THRESHOLD = 10;

test.describe('Minimap', () => {
  test.beforeEach(async ({ page }) => {
    await gotoApp(page);
    await waitForConnection(page);
  });

  test('minimap canvas is visible by default with correct ARIA', async ({ page }) => {
    const minimap = page.locator('.minimap-canvas');
    await expect(minimap).toBeVisible();
    await expect(minimap).toHaveAttribute('role', 'img');
    await expect(minimap).toHaveAttribute('aria-label', 'Session radar minimap');
  });

  test('N key toggles minimap visibility', async ({ page }) => {
    const minimap = page.locator('.minimap-canvas');
    await expect(minimap).toBeVisible();

    // Hide
    await page.keyboard.press('n');
    await expect(minimap).toBeHidden();

    // Show again
    await page.keyboard.press('n');
    await expect(minimap).toBeVisible();
  });

  test('N toggle logs status to debug log', async ({ page }) => {
    const debugLog = page.locator('#debug-log');

    await page.keyboard.press('n');
    await expect(debugLog).toContainText('Mini-map hidden');

    await page.keyboard.press('n');
    await expect(debugLog).toContainText('Mini-map shown');
  });

  test('Shift+N toggles zoom (pinned scale)', async ({ page }) => {
    const minimap = page.locator('.minimap-canvas');

    // Zoom in
    await page.keyboard.press('Shift+n');
    await expect(minimap).toHaveCSS('transform', 'matrix(2, 0, 0, 2, 0, 0)');

    // Zoom out
    await page.keyboard.press('Shift+n');
    const transform = await minimap.evaluate(
      (el) => (el as HTMLElement).style.transform,
    );
    expect(transform).toBe('');
  });

  test('minimap draws dots when racers are present', async ({ page }) => {
    await waitForRacers(page, 1);

    // Poll the minimap canvas until it has non-background colored pixels,
    // indicating dots have been drawn for the active sessions.
    await expect
      .poll(
        () =>
          page.evaluate((threshold) => {
            const canvas = document.querySelector('.minimap-canvas') as HTMLCanvasElement;
            if (!canvas) return false;
            const ctx = canvas.getContext('2d');
            if (!ctx) return false;

            // Sample a horizontal strip through the middle of the drawing area
            // (below the RADAR label, above bottom padding).
            const y = Math.floor(canvas.height * 0.55);
            const data = ctx.getImageData(0, y, canvas.width, 1).data;

            for (let i = 0; i < data.length; i += 4) {
              const r = data[i], g = data[i + 1], b = data[i + 2], a = data[i + 3];
              if (a > 0 && (r > threshold || g > threshold || b > threshold)) {
                return true;
              }
            }
            return false;
          }, PIXEL_BRIGHTNESS_THRESHOLD),
        { message: 'waiting for minimap to draw session dots', timeout: 10_000 },
      )
      .toBe(true);
  });

  test('minimap shows "no sessions" text when canvas has zero racers', async ({
    page,
  }) => {
    // Temporarily clear all racers from the raceCanvas to trigger the
    // "no sessions" empty-state text in the minimap.
    const hasEmptyText = await page.evaluate(() => {
      const rc = (window as any).raceCanvas;
      const saved = new Map(rc.racers);
      rc.racers.clear();

      // Wait one minimap frame (100ms) for it to redraw
      return new Promise<boolean>((resolve) => {
        setTimeout(() => {
          const canvas = document.querySelector('.minimap-canvas') as HTMLCanvasElement;
          const ctx = canvas?.getContext('2d');
          if (!ctx) {
            rc.racers = saved;
            resolve(false);
            return;
          }
          // Sample a pixel strip for the "no sessions" text — rendered at y ~ HEIGHT/2+4
          const y = Math.floor(canvas.height / 2) + 4;
          const data = ctx.getImageData(0, y, canvas.width, 1).data;
          let found = false;
          for (let i = 0; i < data.length; i += 4) {
            if (data[i + 3] > 0 && (data[i] > 5 || data[i + 1] > 5 || data[i + 2] > 5)) {
              found = true;
              break;
            }
          }
          // Restore racers
          for (const [k, v] of saved) rc.racers.set(k, v);
          resolve(found);
        }, 200);
      });
    });

    expect(hasEmptyText, 'minimap should render "no sessions" when empty').toBe(true);
  });

  test('hidden minimap canvas has display:none', async ({ page }) => {
    const minimap = page.locator('.minimap-canvas');

    await page.keyboard.press('n');
    await expect(minimap).toHaveCSS('display', 'none');

    await page.keyboard.press('n');
    await expect(minimap).toHaveCSS('display', 'block');
  });
});
