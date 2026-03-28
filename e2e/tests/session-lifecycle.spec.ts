import { test, expect, Page } from '@playwright/test';
import { waitForConnection, gotoApp } from './helpers.js';
const TIMEOUT_RACERS_APPEAR = 10_000;
const TIMEOUT_ACTIVITY = 15_000;
const TIMEOUT_ZONE_TRANSITION = 10_000;
const TIMEOUT_TEST = 90_000;

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

interface RacerZoneInfo {
  id: string;
  activity: string;
  inPit: boolean;
  inParkingLot: boolean;
  displayX: number;
  displayY: number;
  initialized: boolean;
}

/** Return zone info for every racer on the canvas. */
async function getAllRacerZones(page: Page): Promise<RacerZoneInfo[]> {
  return page.evaluate(() => {
    const rc = (window as any).raceCanvas;
    const result: any[] = [];
    for (const r of rc.racers.values()) {
      result.push({
        id: r.id,
        activity: r.state.activity,
        inPit: r.inPit,
        inParkingLot: r.inParkingLot,
        displayX: r.displayX,
        displayY: r.displayY,
        initialized: r.initialized,
      });
    }
    return result;
  });
}

/** Wait until at least `count` racers are rendered (displayX/Y > 0). */
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
    { timeout: TIMEOUT_RACERS_APPEAR },
  );
}

/** Wait until a specific racer reaches one of the given activities. */
async function waitForActivity(
  page: Page,
  racerId: string,
  activities: string[],
  timeout = TIMEOUT_ACTIVITY,
): Promise<void> {
  await page.waitForFunction(
    ({ id, acts }) => {
      const rc = (window as any).raceCanvas;
      const r = rc?.racers?.get(id);
      return r != null && acts.includes(r.state.activity);
    },
    { id: racerId, acts: activities },
    { timeout },
  );
}

/** Wait until a specific racer enters the pit lane. */
async function waitForPit(
  page: Page,
  racerId: string,
  timeout = TIMEOUT_ZONE_TRANSITION,
): Promise<void> {
  await page.waitForFunction(
    (id) => {
      const rc = (window as any).raceCanvas;
      const r = rc?.racers?.get(id);
      return r?.inPit === true;
    },
    racerId,
    { timeout },
  );
}

/** Wait until a specific racer enters the parking lot. */
async function waitForParkingLot(
  page: Page,
  racerId: string,
  timeout = TIMEOUT_ZONE_TRANSITION,
): Promise<void> {
  await page.waitForFunction(
    (id) => {
      const rc = (window as any).raceCanvas;
      const r = rc?.racers?.get(id);
      return r?.inParkingLot === true;
    },
    racerId,
    { timeout },
  );
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe('Session lifecycle', () => {
  // E2E config uses 100ms/tick so lifecycle transitions happen in ~4-7s.
  test.setTimeout(TIMEOUT_TEST);

  test.beforeEach(async ({ page }) => {
    // Debug hooks: capture browser-side errors so CI failures are diagnosable.
    const consoleLogs: string[] = [];
    const pageErrors: string[] = [];
    const failedResponses: string[] = [];
    const scriptResponses: string[] = [];

    page.on('console', (msg) => {
      const text = `[${msg.type()}] ${msg.text()}`;
      consoleLogs.push(text);
      if (msg.type() === 'error' || msg.type() === 'warning') {
        console.log(`  browser ${text}`);
      }
    });

    page.on('pageerror', (err) => {
      const text = `${err.name}: ${err.message}`;
      pageErrors.push(text);
      console.log(`  browser PAGE ERROR: ${text}`);
    });

    page.on('response', (resp) => {
      const url = resp.url();
      // Track all JS/CSS/WS responses for debugging script loading issues.
      if (url.endsWith('.js') || url.endsWith('.css') || url.includes('/ws')) {
        const contentType = resp.headers()['content-type'] || 'no-content-type';
        scriptResponses.push(`${resp.status()} ${contentType} ${url}`);
      }
      if (resp.status() >= 400) {
        const text = `${resp.status()} ${url}`;
        failedResponses.push(text);
        console.log(`  browser FAILED RESPONSE: ${text}`);
      }
    });

    page.on('requestfailed', (req) => {
      console.log(`  browser REQUEST FAILED: ${req.url()} ${req.failure()?.errorText}`);
    });

    // Inject CSP violation listener before navigation so we catch everything.
    await page.addInitScript(() => {
      (window as any).__cspViolations = [];
      document.addEventListener('securitypolicyviolation', (e) => {
        const info = `blocked: ${e.blockedURI} directive: ${e.violatedDirective} policy: ${e.originalPolicy?.slice(0, 120)}`;
        (window as any).__cspViolations.push(info);
        console.error(`CSP VIOLATION: ${info}`);
      });
    });

    await gotoApp(page);

    // If waitForConnection fails, dump everything we captured.
    try {
      await waitForConnection(page);
    } catch (e) {
      console.log('\n=== DEBUG: waitForConnection timed out ===');
      console.log('Console logs:', JSON.stringify(consoleLogs, null, 2));
      console.log('Page errors:', JSON.stringify(pageErrors, null, 2));
      console.log('Failed responses:', JSON.stringify(failedResponses, null, 2));
      console.log('Script/CSS/WS responses:', JSON.stringify(scriptResponses, null, 2));

      // Capture the current page state for additional diagnostics.
      const statusEl = await page.$('#connection-status');
      const statusClasses = statusEl ? await statusEl.getAttribute('class') : 'NOT FOUND';
      const statusLabel = await page.$eval('#connection-status-label', (el: Element) => el.textContent).catch(() => 'NOT FOUND');
      console.log('Connection status classes:', statusClasses);
      console.log('Connection status label:', statusLabel);

      // Check if main.js even loaded by testing for known globals.
      const jsState = await page.evaluate(() => {
        const w = window as any;
        return {
          hasRaceCanvas: !!w.raceCanvas,
          hasRaceConnection: !!w.raceConnection,
          cspViolations: w.__cspViolations || [],
          locationHref: location.href,
          locationHost: location.host,
        };
      }).catch(() => ({ error: 'evaluate failed' }));
      console.log('JS state:', JSON.stringify(jsState));

      // Dump the CSP header from an unauthenticated fetch (HTML pages don't need auth).
      const cspHeader = await page.evaluate(async () => {
        try {
          const resp = await fetch('/', { cache: 'no-store' });
          return resp.headers.get('content-security-policy');
        } catch (err) { return 'fetch failed: ' + String(err); }
      }).catch(() => 'evaluate failed');
      console.log('CSP header:', cspHeader);

      // Log first 500 chars of HTML to verify the page was served correctly.
      const html = await page.content();
      console.log('Page HTML (first 500):', html.slice(0, 500));
      console.log('=== END DEBUG ===\n');

      throw e;
    }

    await waitForRacers(page, 3);
  });

  test('new sessions render with initialized positions', async ({ page }) => {
    // All mock sessions start with activity=starting, then transition.
    // By the time we get here, racers should be initialized on the canvas.
    const zones = await getAllRacerZones(page);
    expect(zones.length).toBeGreaterThanOrEqual(3);

    // Every racer should be initialized with valid display coordinates
    for (const racer of zones) {
      expect(racer.initialized).toBe(true);
      expect(racer.displayX).toBeGreaterThan(0);
      expect(racer.displayY).toBeGreaterThan(0);
    }
  });

  test('idle session moves to pit area after staleness threshold', async ({
    page,
  }) => {
    // mock-opus-debug works for 40 ticks then permanently enters "waiting".
    // In mock mode, lastDataReceivedAt is unset (zero), so the freshness
    // check treats it as stale immediately and the racer moves to pit as
    // soon as activity becomes "waiting".
    const debugId = 'mock-opus-debug';

    await waitForActivity(page, debugId, ['waiting']);

    // With zero lastDataReceivedAt, pit classification is immediate once waiting
    await waitForPit(page, debugId);

    // Verify the racer is now in the pit zone
    const zones = await getAllRacerZones(page);
    const debugRacer = zones.find((r) => r.id === debugId);
    expect(debugRacer).toBeDefined();
    expect(debugRacer!.inPit).toBe(true);
    expect(debugRacer!.inParkingLot).toBe(false);
    expect(debugRacer!.activity).toBe('waiting');
  });

  test('errored session moves to parking lot', async ({ page }) => {
    // mock-sonnet-feature errors at 60% context utilization (~67 ticks).
    const featureId = 'mock-sonnet-feature';

    await waitForActivity(page, featureId, ['errored']);
    await waitForParkingLot(page, featureId);

    const zones = await getAllRacerZones(page);
    const erroredRacer = zones.find((r) => r.id === featureId);
    expect(erroredRacer).toBeDefined();
    expect(erroredRacer!.inParkingLot).toBe(true);
    expect(erroredRacer!.inPit).toBe(false);
    expect(erroredRacer!.activity).toBe('errored');
  });

  test('completed session moves to parking lot', async ({ page }) => {
    // mock-sonnet-tests completes at 140K tokens (~40 ticks).
    const testsId = 'mock-sonnet-tests';

    await waitForActivity(page, testsId, ['complete']);
    await waitForParkingLot(page, testsId);

    const zones = await getAllRacerZones(page);
    const completedRacer = zones.find((r) => r.id === testsId);
    expect(completedRacer).toBeDefined();
    expect(completedRacer!.inParkingLot).toBe(true);
    expect(completedRacer!.inPit).toBe(false);
    expect(completedRacer!.activity).toBe('complete');
  });

  test('zone assignments are consistent with racer activity', async ({
    page,
  }) => {
    // Wait for the mock to produce zone diversity (feature errors at ~67 ticks).
    await waitForActivity(page, 'mock-sonnet-feature', ['errored']);
    // Give the animation loop a moment to settle zone assignments
    await page.waitForTimeout(500);

    const zones = await getAllRacerZones(page);

    const terminalActivities = ['errored', 'complete', 'lost'];
    const pitEligibleActivities = ['idle', 'waiting', 'starting'];

    for (const racer of zones) {
      if (terminalActivities.includes(racer.activity)) {
        expect(racer.inParkingLot).toBe(true);
        expect(racer.inPit).toBe(false);
      } else if (racer.inPit) {
        expect(pitEligibleActivities).toContain(racer.activity);
      }
    }
  });
});
