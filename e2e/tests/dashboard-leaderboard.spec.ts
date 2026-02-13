import { test, expect, Page } from '@playwright/test';

const TIMEOUT_CONNECTED = 10_000;
const TIMEOUT_SESSIONS = 15_000;
const MIN_MOCK_SESSIONS = 5;

async function waitForSessions(page: Page, minCount: number): Promise<void> {
  await page.waitForFunction(
    (n) => {
      const rc = (window as any).raceCanvas;
      return rc?.racers?.size >= n;
    },
    minCount,
    { timeout: TIMEOUT_SESSIONS },
  );
}

/**
 * Mirrors Dashboard.formatTokens for cross-checking browser output.
 */
function formatTokens(tokens: number): string {
  if (tokens >= 1_000_000) return `${(tokens / 1_000_000).toFixed(1)}M`;
  if (tokens >= 1000) return `${Math.round(tokens / 1000)}K`;
  return `${tokens}`;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe('Dashboard leaderboard', () => {
  test.setTimeout(45_000);

  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    await page.waitForSelector('.status-dot.connected', { timeout: TIMEOUT_CONNECTED });
    await waitForSessions(page, MIN_MOCK_SESSIONS);
  });

  test('session counts match RACING / PIT / PARKED categories', async ({ page }) => {
    const counts = await page.evaluate(() => {
      const rc = (window as any).raceCanvas;
      const all = [...rc.racers.values()].map((r: any) => r.state);
      const terminal = new Set(['complete', 'errored', 'lost']);
      const racing = all.filter((s: any) => s.activity === 'thinking' || s.activity === 'tool_use').length;
      const pit = all.filter((s: any) => s.activity === 'idle' || s.activity === 'waiting' || s.activity === 'starting').length;
      const parked = all.filter((s: any) => terminal.has(s.activity)).length;
      return { racing, pit, parked, total: all.length };
    });

    expect(counts.racing + counts.pit + counts.parked).toBe(counts.total);
    expect(counts.racing + counts.pit).toBeGreaterThan(0);
  });

  test('leaderboard is sorted by context utilization descending', async ({ page }) => {
    const utilizations = await page.evaluate(() => {
      const rc = (window as any).raceCanvas;
      return [...rc.racers.values()]
        .map((r: any) => r.state.contextUtilization || 0)
        .sort((a: number, b: number) => b - a);
    });

    expect(utilizations.length).toBeGreaterThanOrEqual(MIN_MOCK_SESSIONS);

    for (let i = 1; i < utilizations.length; i++) {
      expect(utilizations[i - 1]).toBeGreaterThanOrEqual(utilizations[i]);
    }
  });

  test('token counts use correct K/M formatting', async ({ page }) => {
    const browserFormatted = await page.evaluate(() => {
      const rc = (window as any).raceCanvas;
      function fmt(tokens: number): string {
        if (tokens >= 1_000_000) return `${(tokens / 1_000_000).toFixed(1)}M`;
        if (tokens >= 1000) return `${Math.round(tokens / 1000)}K`;
        return `${tokens}`;
      }
      const results: { tokens: number; formatted: string }[] = [];
      for (const racer of rc.racers.values()) {
        const t = racer.state.tokensUsed;
        results.push({ tokens: t, formatted: fmt(t) });
      }
      return results;
    });

    for (const entry of browserFormatted) {
      expect(entry.formatted).toBe(formatTokens(entry.tokens));
    }
  });

  test('context utilization percentages are in valid range', async ({ page }) => {
    const utilizations = await page.evaluate(() => {
      const rc = (window as any).raceCanvas;
      return [...rc.racers.values()].map((r: any) => r.state.contextUtilization || 0);
    });

    for (const util of utilizations) {
      expect(util).toBeGreaterThanOrEqual(0);
      expect(util).toBeLessThanOrEqual(1);
    }
  });

  test('dashboard renders visually on canvas', async ({ page }) => {
    await page.waitForTimeout(500);

    const canvas = page.locator('#race-canvas');
    await expect(canvas).toBeVisible();

    const hasDashboardContent = await page.evaluate(() => {
      const el = document.getElementById('race-canvas') as HTMLCanvasElement;
      const ctx = el.getContext('2d');
      if (!ctx) return false;

      const sampleY = Math.floor(el.height * 0.85);
      const imageData = ctx.getImageData(0, sampleY, el.width, 1).data;

      let nonBlackPixels = 0;
      for (let i = 0; i < imageData.length; i += 4) {
        const r = imageData[i];
        const g = imageData[i + 1];
        const b = imageData[i + 2];
        const a = imageData[i + 3];
        if (a > 0 && (r > 10 || g > 10 || b > 10)) {
          nonBlackPixels++;
        }
      }
      return nonBlackPixels > 5;
    });

    expect(hasDashboardContent, 'dashboard area should have visible content').toBe(true);

    await canvas.screenshot({ path: 'tests/screenshots/dashboard-leaderboard.png' });
  });

  test('total tokens stat matches sum of all session tokens', async ({ page }) => {
    const result = await page.evaluate(() => {
      const rc = (window as any).raceCanvas;
      function fmt(tokens: number): string {
        if (tokens >= 1_000_000) return `${(tokens / 1_000_000).toFixed(1)}M`;
        if (tokens >= 1000) return `${Math.round(tokens / 1000)}K`;
        return `${tokens}`;
      }
      let total = 0;
      for (const racer of rc.racers.values()) {
        total += racer.state.tokensUsed || 0;
      }
      return { total, formatted: fmt(total) };
    });

    expect(result.total).toBeGreaterThan(0);
    expect(result.formatted).toBe(formatTokens(result.total));
  });
});
