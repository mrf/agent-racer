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
    await gotoApp(page);
    await waitForConnection(page);
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
