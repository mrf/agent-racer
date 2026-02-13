import { test, expect, Page } from '@playwright/test';

interface RacerInfo {
  id: string;
  x: number;
  y: number;
  state: string;
}

/**
 * Returns the canvas-relative position of the first rendered racer,
 * or null if no racer has displayX/displayY > 0.
 */
async function getFirstRacerPosition(page: Page): Promise<RacerInfo | null> {
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
async function waitForRacers(page: Page, count = 1): Promise<void> {
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
async function clickRacerOnCanvas(page: Page, racerId: string): Promise<void> {
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
 * Returns the racer info for assertions.
 */
async function clickFirstRacer(page: Page): Promise<RacerInfo> {
  const info = await getFirstRacerPosition(page);
  if (!info) throw new Error('No racer found on canvas');

  await clickRacerOnCanvas(page, info.id);
  return info;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe('Racer detail flyout', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    // Wait for WebSocket to connect and racers to render
    await page.waitForSelector('.status-dot.connected', { timeout: 10_000 });
    await waitForRacers(page, 1);
  });

  test('clicking a racer opens the detail flyout with session info', async ({
    page,
  }) => {
    const racer = await clickFirstRacer(page);
    const flyout = page.locator('#detail-flyout');

    await expect(flyout).toBeVisible();

    // Verify session info fields are present
    const content = page.locator('#flyout-content');
    await expect(content.locator('.label:text-is("Session ID")')).toBeVisible();
    await expect(content.locator('.label:text-is("Model")')).toBeVisible();
    await expect(content.locator('.label:text-is("Activity")')).toBeVisible();
    await expect(content.locator('.label:text-is("Working Dir")')).toBeVisible();

    // The session ID in the flyout should match the clicked racer
    const idRow = content
      .locator('.detail-row')
      .filter({ has: page.locator('.label:text("Session ID")') });
    await expect(idRow.locator('.value')).toHaveText(racer.id);
  });

  test('flyout displays token progress bar', async ({ page }) => {
    await clickFirstRacer(page);

    const progressBar = page.locator('#flyout-content .detail-progress');
    await expect(progressBar).toBeVisible();

    const bar = progressBar.locator('.detail-progress-bar');
    await expect(bar).toBeVisible();

    // Bar should have a non-zero width (style attribute contains width:)
    const style = await bar.getAttribute('style');
    expect(style).toContain('width:');
  });

  test('flyout shows activity badge', async ({ page }) => {
    await clickFirstRacer(page);

    const activityBadge = page.locator('#flyout-content .detail-activity');
    await expect(activityBadge.first()).toBeVisible();

    // Activity text should be one of the known activity types
    const text = await activityBadge.first().textContent();
    const validActivities = [
      'starting',
      'thinking',
      'tool_use',
      'waiting',
      'idle',
      'complete',
      'errored',
      'lost',
    ];
    expect(validActivities).toContain(text?.toLowerCase().trim());
  });

  test('pressing Escape closes the flyout', async ({ page }) => {
    await clickFirstRacer(page);

    const flyout = page.locator('#detail-flyout');
    await expect(flyout).toBeVisible();

    await page.keyboard.press('Escape');
    await expect(flyout).toBeHidden();
  });

  test('clicking the close button dismisses the flyout', async ({ page }) => {
    await clickFirstRacer(page);

    const flyout = page.locator('#detail-flyout');
    await expect(flyout).toBeVisible();

    await page.locator('#flyout-close').click();
    await expect(flyout).toBeHidden();
  });

  test('clicking another racer updates flyout content', async ({ page }) => {
    // Need at least 2 racers
    await waitForRacers(page, 2);

    // Click the first racer via canvas to open the flyout
    const firstRacer = await clickFirstRacer(page);
    const flyout = page.locator('#detail-flyout');
    await expect(flyout).toBeVisible();

    const idRow = page
      .locator('#flyout-content .detail-row')
      .filter({ has: page.locator('.label:text-is("Session ID")') });
    await expect(idRow.locator('.value')).toHaveText(firstRacer.id);

    // Trigger onRacerClick for a different racer programmatically.
    // Canvas hit-testing uses a 45px radius and Map iteration order, so
    // overlapping racers make a second mouse.click unreliable. Invoking
    // the callback directly still exercises the flyout update path.
    const secondId = await page.evaluate((skipId) => {
      const rc = (window as any).raceCanvas;
      for (const racer of rc.racers.values()) {
        if (racer.id !== skipId) {
          rc.onRacerClick(racer.state);
          return racer.id;
        }
      }
      return null;
    }, firstRacer.id);

    expect(secondId).not.toBeNull();
    await expect(idRow.locator('.value')).toHaveText(secondId!);
  });

  test('flyout stays within viewport bounds', async ({ page }) => {
    await clickFirstRacer(page);

    const flyout = page.locator('#detail-flyout');
    await expect(flyout).toBeVisible();

    const flyoutBox = await flyout.boundingBox();
    expect(flyoutBox).not.toBeNull();

    const viewport = page.viewportSize();
    expect(viewport).not.toBeNull();

    // Flyout must be fully within the viewport
    expect(flyoutBox!.x).toBeGreaterThanOrEqual(0);
    expect(flyoutBox!.y).toBeGreaterThanOrEqual(0);
    expect(flyoutBox!.x + flyoutBox!.width).toBeLessThanOrEqual(
      viewport!.width,
    );
    expect(flyoutBox!.y + flyoutBox!.height).toBeLessThanOrEqual(
      viewport!.height,
    );
  });

  test('flyout displays all expected detail fields', async ({ page }) => {
    await clickFirstRacer(page);

    const content = page.locator('#flyout-content');
    const expectedLabels = [
      'Activity',
      'Burn Rate',
      'Model',
      'Source',
      'Working Dir',
      'Branch',
      'Session ID',
      'PID',
      'Messages',
      'Tool Calls',
      'Current Tool',
      'Started',
      'Last Activity',
      'Elapsed',
      'Input Tokens',
      'Max Tokens',
      'Context %',
    ];

    for (const label of expectedLabels) {
      await expect(
        content.locator('.label').filter({ hasText: label }).first(),
      ).toBeVisible();
    }
  });
});
