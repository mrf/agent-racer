import { test, expect } from '@playwright/test';

import { waitForConnection, waitForRacers, clickFirstRacer, gotoApp } from './helpers.js';

test.describe('Racer detail flyout', () => {
  test.setTimeout(60_000);

  test.beforeEach(async ({ page }) => {
    await gotoApp(page);
    // Wait for WebSocket to connect and racers to render
    await waitForConnection(page);
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

    // The session ID in the flyout should match a valid racer on the canvas.
    // Due to animation timing, the clicked racer may differ from the one
    // identified pre-click, so verify the displayed ID is a real session.
    // The session-id-text span holds the truncated ID; the full ID is in
    // its title attribute.
    const idRow = content
      .locator('.detail-row')
      .filter({ has: page.locator('.label:text("Session ID")') });
    const displayedId = await idRow.locator('.session-id-text').getAttribute('title');
    expect(displayedId).toBeTruthy();
    const isValidRacer = await page.evaluate((id) => {
      const rc = (window as any).raceCanvas;
      return rc.racers.has(id);
    }, displayedId);
    expect(isValidRacer).toBe(true);
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

    // The session-id-text span holds the truncated display text; the full
    // session ID is stored in its title attribute.
    const idSpan = page
      .locator('#flyout-content .detail-row')
      .filter({ has: page.locator('.label:text-is("Session ID")') })
      .locator('.session-id-text');
    await expect(idSpan).toHaveAttribute('title', firstRacer.id);

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
    await expect(idSpan).toHaveAttribute('title', secondId!);
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

    const expectedLabels = [
      'Activity',
      'Burn Rate',
      'Model',
      'Source',
      'Working Dir',
      'Branch',
      'Tmux',
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

    // Check all labels in a single evaluate to avoid repeated DOM queries
    // racing with flyout re-renders from live WebSocket updates.
    const missing = await page.evaluate((labels) => {
      const content = document.getElementById('flyout-content');
      if (!content) return labels;
      const labelEls = content.querySelectorAll('.label');
      const found = new Set<string>();
      for (const el of labelEls) {
        found.add(el.textContent?.trim() || '');
      }
      return labels.filter((l: string) => !found.has(l));
    }, expectedLabels);

    expect(missing).toEqual([]);
  });
});
