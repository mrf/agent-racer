import { test, expect } from '@playwright/test';

import { waitForConnection, waitForRacers, clickFirstRacer } from './helpers.js';

test.describe('Keyboard shortcuts', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    await waitForConnection(page);
  });

  test('D toggles debug panel visibility', async ({ page }) => {
    const debugPanel = page.locator('#debug-panel');
    await expect(debugPanel).toBeHidden();

    await page.keyboard.press('d');
    await expect(debugPanel).toBeVisible();

    await page.keyboard.press('d');
    await expect(debugPanel).toBeHidden();
  });

  test('M toggles mute state', async ({ page }) => {
    const debugLog = page.locator('#debug-log');

    await page.keyboard.press('m');
    await expect(debugLog).toContainText('Sound muted');

    await page.keyboard.press('m');
    await expect(debugLog).toContainText('Sound unmuted');
  });

  test('F toggles fullscreen', async ({ page }) => {
    // Spy on fullscreen API calls since headless browsers may block activation
    await page.evaluate(() => {
      (window as any).__fullscreenCalls = [];
      const orig = document.documentElement.requestFullscreen.bind(document.documentElement);
      document.documentElement.requestFullscreen = function (...args: any[]) {
        (window as any).__fullscreenCalls.push('enter');
        return orig(...args);
      };
      const origExit = document.exitFullscreen.bind(document);
      document.exitFullscreen = function () {
        (window as any).__fullscreenCalls.push('exit');
        return origExit();
      };
    });

    await page.keyboard.press('f');
    const callsAfterEnter = await page.evaluate(() => (window as any).__fullscreenCalls);
    expect(callsAfterEnter).toContain('enter');

    // If fullscreen actually activated, press F again to test exit path
    const activated = await page.evaluate(() => !!document.fullscreenElement);
    if (activated) {
      await page.keyboard.press('f');
      const callsAfterExit = await page.evaluate(() => (window as any).__fullscreenCalls);
      expect(callsAfterExit).toContain('exit');
    }
  });

  test('Escape closes flyout if open', async ({ page }) => {
    await waitForRacers(page, 1);
    await clickFirstRacer(page);

    const flyout = page.locator('#detail-flyout');
    await expect(flyout).toBeVisible();

    await page.keyboard.press('Escape');
    await expect(flyout).toBeHidden();
  });
});
