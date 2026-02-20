import type { Page } from '@playwright/test';

export interface RacerInfo {
  id: string;
  x: number;
  y: number;
  state: string;
}

/**
 * Waits until the WebSocket connection is established.
 * Uses waitForFunction instead of waitForSelector to avoid
 * Playwright selector stability issues across versions.
 */
export async function waitForConnection(page: Page, timeout = 15_000): Promise<void> {
  await page.waitForFunction(
    () => document.querySelector('#connection-status')?.classList.contains('connected'),
    { timeout },
  );
}

/**
 * Returns the canvas-relative position of the first rendered racer,
 * or null if no racer has displayX/displayY > 0.
 */
export async function getFirstRacerPosition(page: Page): Promise<RacerInfo | null> {
  return page.evaluate(() => {
    const rc = (window as any).raceCanvas;
    for (const racer of rc.racers.values()) {
      if (racer.displayX > 0 && racer.displayY > 0) {
        return {
          id: racer.id,
          x: racer.displayX,
          y: racer.displayY,
          state: racer.state,
        };
      }
    }
    return null;
  });
}

/**
 * Waits until at least `count` racers are rendered on the canvas.
 */
export async function waitForRacers(page: Page, count = 1): Promise<void> {
  await page.waitForFunction(
    (n) => {
      const rc = (window as any).raceCanvas;
      if (!rc?.racers) return false;
      let rendered = 0;
      for (const r of rc.racers.values()) {
        if (r.displayX > 0 && r.displayY > 0) rendered++;
      }
      return rendered >= n;
    },
    count,
    { timeout: 10_000 },
  );
}

/**
 * Clicks on the canvas at the given racer's current position.
 * Fetches fresh coordinates at click time to handle animation drift.
 */
export async function clickRacerOnCanvas(page: Page, racerId: string): Promise<void> {
  const pos = await page.evaluate((targetId) => {
    const rc = (window as any).raceCanvas;
    const r = rc.racers.get(targetId);
    return r ? { x: r.displayX, y: r.displayY } : null;
  }, racerId);
  if (!pos) throw new Error(`Racer ${racerId} not found`);

  const box = await page.locator('#race-canvas').boundingBox();
  if (!box) throw new Error('Canvas not found');

  await page.mouse.click(box.x + pos.x, box.y + pos.y);
}

/**
 * Clicks the first rendered racer on the canvas.
 * Triggers onRacerClick atomically to avoid race conditions between
 * reading the racer position and performing the click during animation.
 * Returns the racer info for assertions.
 */
export async function clickFirstRacer(page: Page): Promise<RacerInfo> {
  const info: RacerInfo | null = await page.evaluate(() => {
    const rc = (window as any).raceCanvas;
    for (const racer of rc.racers.values()) {
      if (racer.displayX > 0 && racer.displayY > 0) {
        rc.onRacerClick(racer.state);
        return {
          id: racer.id,
          x: racer.displayX,
          y: racer.displayY,
          state: racer.state,
        };
      }
    }
    return null;
  });
  if (!info) throw new Error('No racer found on canvas');
  return info;
}
