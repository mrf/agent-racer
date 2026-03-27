// @ts-check
import { test, expect } from '@playwright/test';

const SELECTOR_DIALOG = '[role="dialog"][aria-label="Replay selector"]';

// Synthetic NDJSON replay data — 5 snapshots with 1-second spacing.
const REPLAY_SNAPSHOTS = [
  { t: '2026-03-10T10:00:00Z', s: [{ id: 'sess-1', name: 'alpha', activity: 'working', progress: 10 }] },
  { t: '2026-03-10T10:00:01Z', s: [{ id: 'sess-1', name: 'alpha', activity: 'working', progress: 30 }] },
  { t: '2026-03-10T10:00:02Z', s: [{ id: 'sess-1', name: 'alpha', activity: 'working', progress: 50 }] },
  { t: '2026-03-10T10:00:03Z', s: [{ id: 'sess-1', name: 'alpha', activity: 'working', progress: 70 }] },
  { t: '2026-03-10T10:00:04Z', s: [{ id: 'sess-1', name: 'alpha', activity: 'complete', progress: 100 }] },
];

const REPLAY_NDJSON = REPLAY_SNAPSHOTS.map((s) => JSON.stringify(s)).join('\n');

const REPLAY_LIST = [
  { id: 'rec-001', name: 'race-2026-03-10.ndjson', size: 2048, createdAt: '2026-03-10T12:00:00Z' },
  { id: 'rec-002', name: 'race-2026-03-09.ndjson', size: 1024, createdAt: '2026-03-09T08:30:00Z' },
];

/**
 * Set up route mocks for /api/replays, /api/replays/:id, and /api/config.
 */
async function mockReplayRoutes(page) {
  await page.route('**/api/replays', (route) => {
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(REPLAY_LIST),
    });
  });

  await page.route('**/api/replays/*', (route) => {
    route.fulfill({
      status: 200,
      contentType: 'application/x-ndjson',
      body: REPLAY_NDJSON,
    });
  });

  await page.route('**/api/config', (route) => {
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({}),
    });
  });
}

/** Navigate to the app and wait for it to be ready. */
async function loadApp(page) {
  await mockReplayRoutes(page);
  await page.goto('/');
  await page.waitForSelector('#race-canvas');
}

/** Press 'r' to open the replay selector dialog. */
async function openReplaySelector(page) {
  await page.keyboard.press('r');
  await page.waitForSelector(SELECTOR_DIALOG);
}

/** Open the selector, click the first replay, and wait for the playback bar. */
async function loadFirstReplay(page) {
  await openReplaySelector(page);
  await page.locator('.ts-replay-item').first().click();
  await expect(page.locator('.ts-bar')).toBeVisible();
}

test.describe('Replay UI workflow', () => {
  test.beforeEach(async ({ page }) => {
    await loadApp(page);
  });

  test('pressing R opens the replay selector dialog', async ({ page }) => {
    await openReplaySelector(page);

    const dialog = page.locator(SELECTOR_DIALOG);
    await expect(dialog).toBeVisible();
    await expect(dialog).toHaveAttribute('aria-modal', 'true');
    await expect(dialog.locator('.ts-title')).toContainText('Replay');
  });

  test('replay selector lists available replays from /api/replays', async ({ page }) => {
    await openReplaySelector(page);

    const items = page.locator('.ts-replay-item');
    await expect(items).toHaveCount(2);

    await expect(items.nth(0).locator('.ts-replay-item-name')).toHaveText('race-2026-03-10.ndjson');
    await expect(items.nth(0).locator('.ts-replay-item-size')).toHaveText('2.0 KB');

    await expect(items.nth(1).locator('.ts-replay-item-name')).toHaveText('race-2026-03-09.ndjson');
    await expect(items.nth(1).locator('.ts-replay-item-size')).toHaveText('1.0 KB');
  });

  test('selecting a replay loads it and shows the playback bar', async ({ page }) => {
    await openReplaySelector(page);
    await page.locator('.ts-replay-item').first().click();

    await expect(page.locator(SELECTOR_DIALOG)).toHaveCount(0);

    const bar = page.locator('.ts-bar');
    await expect(bar).toBeVisible();
    await expect(bar.locator('.ts-replay-name')).toHaveText('rec-001');
    await expect(bar.locator('.ts-replay-label')).toContainText('REPLAY');
  });

  test('playback bar has play/pause, step, speed, and slider controls', async ({ page }) => {
    await loadFirstReplay(page);

    // Playback auto-starts, so the button shows "Pause".
    await expect(page.locator('[aria-label="Pause replay"]')).toBeVisible();

    await expect(page.locator('[aria-label="Step backward one replay frame"]')).toBeVisible();
    await expect(page.locator('[aria-label="Step forward one replay frame"]')).toBeVisible();

    await expect(page.locator('[aria-label="Set replay speed to 1x"]')).toBeVisible();
    await expect(page.locator('[aria-label="Set replay speed to 2x"]')).toBeVisible();
    await expect(page.locator('[aria-label="Set replay speed to 4x"]')).toBeVisible();

    // 1x active by default.
    await expect(page.locator('[aria-label="Set replay speed to 1x"]')).toHaveClass(/active/);

    const slider = page.locator('.ts-slider');
    await expect(slider).toBeVisible();
    await expect(slider).toHaveAttribute('aria-label', 'Replay timeline position');
    // max = total snapshots - 1 = 4.
    await expect(slider).toHaveAttribute('max', '4');
  });

  test('play/pause button toggles playback state', async ({ page }) => {
    await loadFirstReplay(page);

    const playBtn = page.locator('.ts-play-btn');
    // Auto-starts playing.
    await expect(playBtn).toHaveAttribute('aria-label', 'Pause replay');

    await playBtn.click();
    await expect(playBtn).toHaveAttribute('aria-label', 'Play replay');

    await playBtn.click();
    await expect(playBtn).toHaveAttribute('aria-label', 'Pause replay');
  });

  test('speed buttons switch active state', async ({ page }) => {
    await loadFirstReplay(page);

    const speed1x = page.locator('[aria-label="Set replay speed to 1x"]');
    const speed2x = page.locator('[aria-label="Set replay speed to 2x"]');
    const speed4x = page.locator('[aria-label="Set replay speed to 4x"]');

    await expect(speed1x).toHaveClass(/active/);

    await speed4x.click();
    await expect(speed4x).toHaveClass(/active/);
    await expect(speed1x).not.toHaveClass(/active/);
    await expect(speed2x).not.toHaveClass(/active/);

    await speed2x.click();
    await expect(speed2x).toHaveClass(/active/);
    await expect(speed4x).not.toHaveClass(/active/);
  });

  test('scrubber slider reflects playback position', async ({ page }) => {
    await loadFirstReplay(page);
    await page.locator('.ts-play-btn').click(); // pause to control manually

    const slider = page.locator('.ts-slider');
    const stepFwd = page.locator('[aria-label="Step forward one replay frame"]');
    const stepBack = page.locator('[aria-label="Step backward one replay frame"]');

    await stepFwd.click();
    await expect(slider).toHaveValue('1');

    await stepFwd.click();
    await expect(slider).toHaveValue('2');

    await stepBack.click();
    await expect(slider).toHaveValue('1');
  });

  test('time display updates when stepping through frames', async ({ page }) => {
    await loadFirstReplay(page);
    await page.locator('.ts-play-btn').click(); // pause

    const timeEl = page.locator('.ts-time');
    await expect(timeEl).toContainText('/5');

    await page.locator('[aria-label="Step forward one replay frame"]').click();
    await expect(timeEl).toContainText('2/5');
  });

  test('clicking Exit Replay closes the playback bar', async ({ page }) => {
    await loadFirstReplay(page);

    await page.locator('[aria-label="Exit replay mode"]').click();
    await expect(page.locator('.ts-bar')).toHaveCount(0);
  });

  test('pressing Escape during playback exits replay mode', async ({ page }) => {
    await loadFirstReplay(page);

    await page.keyboard.press('Escape');
    await expect(page.locator('.ts-bar')).toHaveCount(0);
  });

  test('pressing Escape in the selector closes it without opening the bar', async ({ page }) => {
    await openReplaySelector(page);

    await page.keyboard.press('Escape');

    await expect(page.locator(SELECTOR_DIALOG)).toHaveCount(0);
    await expect(page.locator('.ts-bar')).toHaveCount(0);
  });

  test('pressing R again after exiting reopens the selector', async ({ page }) => {
    await loadFirstReplay(page);

    await page.keyboard.press('Escape');
    await expect(page.locator('.ts-bar')).toHaveCount(0);

    await page.keyboard.press('r');
    await page.waitForSelector(SELECTOR_DIALOG);
    await expect(page.locator('.ts-replay-item')).toHaveCount(2);
  });

  test('selector shows empty state when no replays exist', async ({ page }) => {
    // Override default mock to return an empty list (last-registered route wins).
    await page.route('**/api/replays', (route) => {
      route.fulfill({ status: 200, contentType: 'application/json', body: '[]' });
    });

    await openReplaySelector(page);

    const empty = page.locator('.ts-empty');
    await expect(empty).toBeVisible();
    await expect(empty).toContainText('No replays found');
  });

  test('selector shows error when /api/replays fails', async ({ page }) => {
    await page.route('**/api/replays', (route) => {
      route.fulfill({ status: 500, body: 'Internal Server Error' });
    });

    await openReplaySelector(page);

    const error = page.locator('.ts-error');
    await expect(error).toBeVisible();
    await expect(error).toContainText('Error loading replays');
  });
});
