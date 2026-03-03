import { test, expect, type WebSocketRoute } from '@playwright/test';
import { gotoApp, waitForConnection } from './helpers.js';

// ── Achievement Panel (A key) ───────────────────────────────────────────────

test.describe('Achievement panel', () => {
  test.setTimeout(45_000);

  test.beforeEach(async ({ page }) => {
    await gotoApp(page);
    await waitForConnection(page);
  });

  test('A key opens achievement panel', async ({ page }) => {
    const panel = page.locator('#achievement-panel');
    await expect(panel).toBeHidden();

    await page.keyboard.press('a');
    await expect(panel).toBeVisible();
  });

  test('achievement panel renders tiles with unlock counter', async ({ page }) => {
    await page.keyboard.press('a');
    await expect(page.locator('#achievement-panel')).toBeVisible();

    const firstTile = page.locator('.ap-tile').first();
    await expect(firstTile).toBeVisible({ timeout: 8_000 });

    // Footer counter shows "X / Y unlocked".
    const counter = page.locator('.ap-counter');
    await expect(counter).toHaveText(/\d+ \/ \d+ unlocked/);

    await page.screenshot({ path: 'tests/screenshots/achievement-panel-open.png' });
  });

  test('A key toggles achievement panel closed', async ({ page }) => {
    const panel = page.locator('#achievement-panel');

    await page.keyboard.press('a');
    await expect(panel).toBeVisible();

    await page.keyboard.press('a');
    await expect(panel).toBeHidden();
  });

  test('Escape closes achievement panel', async ({ page }) => {
    const panel = page.locator('#achievement-panel');

    await page.keyboard.press('a');
    await expect(panel).toBeVisible();

    await page.keyboard.press('Escape');
    await expect(panel).toBeHidden();
  });
});

// ── Reward Selector / Garage (G key) ────────────────────────────────────────

test.describe('Reward selector (Garage)', () => {
  test.setTimeout(45_000);

  test.beforeEach(async ({ page }) => {
    await gotoApp(page);
    await waitForConnection(page);
  });

  test('G key opens reward selector', async ({ page }) => {
    const selector = page.locator('#reward-selector');
    await expect(selector).toBeHidden();

    await page.keyboard.press('g');
    await expect(selector).toBeVisible();
  });

  test('reward selector shows Garage title and slot columns', async ({ page }) => {
    await page.keyboard.press('g');
    const selector = page.locator('#reward-selector');
    await expect(selector).toBeVisible();

    await expect(selector.locator('.rs-title')).toContainText('Garage');

    // Slot columns (Paint, Trail, Body, Badge, etc.) appear after API fetches.
    const firstCol = selector.locator('.rs-column').first();
    await expect(firstCol).toBeVisible({ timeout: 8_000 });

    const count = await selector.locator('.rs-column').count();
    expect(count).toBeGreaterThanOrEqual(4);

    await page.screenshot({ path: 'tests/screenshots/reward-selector-open.png' });
  });

  test('G key toggles reward selector closed', async ({ page }) => {
    const selector = page.locator('#reward-selector');

    await page.keyboard.press('g');
    await expect(selector).toBeVisible();

    await page.keyboard.press('g');
    await expect(selector).toBeHidden();
  });

  test('Escape closes reward selector', async ({ page }) => {
    const selector = page.locator('#reward-selector');

    await page.keyboard.press('g');
    await expect(selector).toBeVisible();

    await page.keyboard.press('Escape');
    await expect(selector).toBeHidden();
  });
});

// ── Battle Pass Bar ─────────────────────────────────────────────────────────

test.describe('Battle pass bar', () => {
  test.setTimeout(45_000);

  test.beforeEach(async ({ page }) => {
    await gotoApp(page);
    await waitForConnection(page);
  });

  test('renders tier badge and XP label on initial load', async ({ page }) => {
    const tierBadge = page.locator('.bp-tier-badge');
    await expect(tierBadge).toBeVisible({ timeout: 8_000 });
    await expect(tierBadge).toContainText('Tier');

    // Label contains either "X / 1000 XP" or "X XP -- MAX".
    const xpLabel = page.locator('.bp-xp-bar-label');
    await expect(xpLabel).toHaveText(/\d/);
  });
});

// ── WebSocket-driven gamification events ────────────────────────────────────
// These tests use Playwright's routeWebSocket to proxy the real WS connection
// and inject additional messages, avoiding the timing issues with addInitScript.

test.describe('Gamification WS events', () => {
  let wsRoute: WebSocketRoute;

  test.beforeEach(async ({ page }) => {
    await page.routeWebSocket(/\/ws$/, route => {
      const server = route.connectToServer();
      route.onMessage(msg => server.send(msg));
      server.onMessage(msg => route.send(msg));
      wsRoute = route;
    });

    await gotoApp(page);
    await waitForConnection(page);
  });

  test('battlepass_progress message updates tier badge and XP label', async ({ page }) => {
    wsRoute.send(
      JSON.stringify({
        type: 'battlepass_progress',
        seq: 0,
        payload: {
          xp: 1500,
          tier: 2,
          tierProgress: 0.5,
          recentXP: [{ reason: 'session_completed', amount: 200 }],
          rewards: ['Bronze Badge'],
        },
      }),
    );

    const tierBadge = page.locator('.bp-tier-badge');
    await expect(tierBadge).toContainText('Tier 2');

    // xp=1500, tier=2 => tierXP = 1500 - (2-1)*1000 = 500
    const xpLabel = page.locator('.bp-xp-bar-label');
    await expect(xpLabel).toContainText('500 / 1000 XP');

    await page.waitForFunction(
      () => document.querySelector('#debug-log')?.textContent?.includes('Battle Pass: Tier 2'),
      { timeout: 5_000 },
    );
  });

  test('achievement_unlocked message logs to debug and marks panel dirty', async ({ page }) => {
    wsRoute.send(
      JSON.stringify({
        type: 'achievement_unlocked',
        seq: 0,
        payload: {
          id: 'first_lap',
          name: 'First Lap',
          description: 'Complete your first session',
          tier: 'bronze',
        },
      }),
    );

    await page.waitForFunction(
      () =>
        document
          .querySelector('#debug-log')
          ?.textContent?.includes('Achievement unlocked: First Lap'),
      { timeout: 5_000 },
    );

    // Panel was marked dirty, so opening it triggers a re-fetch.
    await page.keyboard.press('a');
    const panel = page.locator('#achievement-panel');
    await expect(panel).toBeVisible();

    const firstTile = page.locator('.ap-tile').first();
    await expect(firstTile).toBeVisible({ timeout: 8_000 });

    await page.screenshot({ path: 'tests/screenshots/achievement-unlock-panel.png' });
  });
});
