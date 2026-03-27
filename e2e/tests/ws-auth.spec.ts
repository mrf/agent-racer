import { test, expect } from '@playwright/test';
import { AUTH_TOKEN, gotoApp, waitForConnection } from './helpers.js';

test.describe('WebSocket auth handshake', () => {
  test('correct token is accepted and connection stays open', async ({ page }) => {
    await gotoApp(page);
    await waitForConnection(page);

    const statusDot = page.locator('#connection-status');
    await expect(statusDot).toHaveClass(/connected/);
  });

  test('wrong token is rejected with ClosePolicyViolation', async ({ page }) => {
    await page.goto('/?token=definitely-wrong-token');

    const statusDot = page.locator('#connection-status');
    // Backend sends close code 1008 (ClosePolicyViolation), frontend maps to 'unauthorized'.
    await expect(statusDot).toHaveClass(/unauthorized/, { timeout: 10_000 });

    // No reconnection should occur — status must stay unauthorized.
    await page.waitForTimeout(3_000);
    await expect(statusDot).toHaveClass(/unauthorized/);
  });

  test('connection closed when no auth message is sent within 5s', async ({ page }) => {
    test.setTimeout(30_000);

    // Load the app with a valid token so the page is fully functional,
    // then open a separate raw WebSocket that intentionally skips auth.
    await gotoApp(page);
    await waitForConnection(page);

    const result = await page.evaluate(() => {
      const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
      const url = `${protocol}//${location.host}/ws`;
      return new Promise((resolve) => {
        const start = Date.now();
        const ws = new WebSocket(url);
        // Intentionally do NOT send an auth message.
        ws.onclose = (event) => {
          resolve({ code: event.code, elapsed: Date.now() - start });
        };
        ws.onerror = () => {};
        // Safety net so the test doesn't hang forever.
        setTimeout(() => {
          ws.close();
          resolve({ code: -1, elapsed: Date.now() - start });
        }, 15_000);
      });
    });

    const { code, elapsed } = result as { code: number; elapsed: number };
    // The server should have closed the connection (not our safety timeout).
    expect(code).not.toBe(-1);
    // The server's auth read deadline is 5 seconds.
    expect(elapsed).toBeGreaterThanOrEqual(4_000);
    expect(elapsed).toBeLessThan(10_000);
  });
});

test.describe('API endpoint auth', () => {
  test('returns 200 with correct Bearer token', async ({ request }) => {
    const response = await request.get('/api/sessions', {
      headers: { Authorization: `Bearer ${AUTH_TOKEN}` },
    });
    expect(response.status()).toBe(200);
  });

  test('returns 401 without any token', async ({ request }) => {
    const response = await request.get('/api/sessions');
    expect(response.status()).toBe(401);
  });

  test('returns 401 with wrong Bearer token', async ({ request }) => {
    const response = await request.get('/api/sessions', {
      headers: { Authorization: 'Bearer wrong-token' },
    });
    expect(response.status()).toBe(401);
  });
});
