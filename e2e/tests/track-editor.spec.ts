import { test, expect, Page } from '@playwright/test';
import { gotoApp, waitForConnection, AUTH_TOKEN } from './helpers.js';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Open the track editor via E key and wait for the toolbar to appear. */
async function openEditor(page: Page): Promise<void> {
  await page.keyboard.press('e');
  await page.waitForSelector('#track-editor-toolbar', { state: 'visible' });
}

/** Return viewport coordinates for the center of a grid cell. */
async function cellCenter(page: Page, row: number, col: number): Promise<{ x: number; y: number }> {
  return page.evaluate(
    ({ r, c }) => {
      const canvas = document.getElementById('race-canvas') as HTMLCanvasElement;
      const dpr = window.devicePixelRatio || 1;
      const rect = canvas.getBoundingClientRect();
      const scaleX = rect.width > 0 ? (canvas.width / dpr) / rect.width : 1;
      const scaleY = rect.height > 0 ? (canvas.height / dpr) / rect.height : 1;
      const CELL = 32;
      return {
        x: rect.left + (c * CELL + CELL / 2) / scaleX,
        y: rect.top + (r * CELL + CELL / 2) / scaleY,
      };
    },
    { r: row, c: col },
  );
}

/** Left-click on a grid cell to paint the currently selected tile. */
async function paintCell(page: Page, row: number, col: number): Promise<void> {
  const { x, y } = await cellCenter(page, row, col);
  await page.mouse.click(x, y);
}

/** Right-click on a grid cell to erase. */
async function eraseCell(page: Page, row: number, col: number): Promise<void> {
  const { x, y } = await cellCenter(page, row, col);
  await page.mouse.click(x, y, { button: 'right' });
}

/** Select a tile from the palette by its data-tile-id. */
async function selectTile(page: Page, tileId: string): Promise<void> {
  await page.click(`#tile-palette button[data-tile-id="${tileId}"]`);
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe('Track editor', () => {
  test.beforeEach(async ({ page }) => {
    await gotoApp(page);
    await waitForConnection(page);
  });

  // --- Activation / Deactivation ---

  test('E key toggles editor on and off', async ({ page }) => {
    await expect(page.locator('#track-editor-toolbar')).toBeHidden();
    await expect(page.locator('#tile-palette')).toBeHidden();

    await page.keyboard.press('e');
    await expect(page.locator('#track-editor-toolbar')).toBeVisible();
    await expect(page.locator('#tile-palette')).toBeVisible();
    await expect(page.locator('.te-validation')).toBeVisible();

    await page.keyboard.press('e');
    await expect(page.locator('#track-editor-toolbar')).toBeHidden();
    await expect(page.locator('#tile-palette')).toBeHidden();
    await expect(page.locator('.te-validation')).toBeHidden();
  });

  test('Close button deactivates editor', async ({ page }) => {
    await openEditor(page);
    await page.locator('#track-editor-toolbar button', { hasText: 'Close' }).click();
    await expect(page.locator('#track-editor-toolbar')).toBeHidden();
  });

  // --- Tile Palette ---

  test('tile palette selection updates active class', async ({ page }) => {
    await openEditor(page);

    const straightH = page.locator('#tile-palette button[data-tile-id="straight-h"]');
    await expect(straightH).toHaveClass(/active/);

    await selectTile(page, 'curve-ne');
    const curveNE = page.locator('#tile-palette button[data-tile-id="curve-ne"]');
    await expect(curveNE).toHaveClass(/active/);
    await expect(straightH).not.toHaveClass(/active/);
  });

  test('R key rotates straight and curve tiles', async ({ page }) => {
    await openEditor(page);
    await selectTile(page, 'straight-h');

    await page.keyboard.press('r');
    await expect(page.locator('#tile-palette button[data-tile-id="straight-v"]')).toHaveClass(/active/);

    await page.keyboard.press('r');
    await expect(page.locator('#tile-palette button[data-tile-id="straight-h"]')).toHaveClass(/active/);
  });

  // --- Painting & Validation ---

  test('painting tiles progresses through validation states', async ({ page }) => {
    await openEditor(page);
    const validation = page.locator('.te-validation');

    // Empty grid
    await expect(validation).toContainText('No track tiles placed');

    // Place a straight tile
    await selectTile(page, 'straight-h');
    await paintCell(page, 3, 5);
    await expect(validation).toContainText('Missing start line');

    // Place start line adjacent
    await selectTile(page, 'start-line');
    await paintCell(page, 3, 6);
    await expect(validation).toContainText('Missing finish line');

    // Place finish line adjacent — all connected
    await selectTile(page, 'finish-line');
    await paintCell(page, 3, 7);
    await expect(validation).toContainText('Track is valid');
    await expect(validation).toHaveClass(/valid/);
  });

  test('right-click erases a painted tile', async ({ page }) => {
    await openEditor(page);
    const validation = page.locator('.te-validation');

    await selectTile(page, 'straight-h');
    await paintCell(page, 3, 5);
    await expect(validation).toContainText('Missing start line');

    await eraseCell(page, 3, 5);
    await expect(validation).toContainText('No track tiles placed');
  });

  test('disconnected tiles show validation error', async ({ page }) => {
    await openEditor(page);
    const validation = page.locator('.te-validation');

    // Place start and finish far apart (not adjacent)
    await selectTile(page, 'start-line');
    await paintCell(page, 2, 2);
    await selectTile(page, 'finish-line');
    await paintCell(page, 2, 10);

    await expect(validation).toContainText('disconnected tile(s)');
    await expect(validation).toHaveClass(/invalid/);
  });

  // --- Undo / Redo ---

  test('Ctrl+Z undoes and Ctrl+Y redoes tile placement', async ({ page }) => {
    await openEditor(page);
    const validation = page.locator('.te-validation');

    await selectTile(page, 'straight-h');
    await paintCell(page, 3, 5);
    await expect(validation).toContainText('Missing start line');

    // Undo
    await page.keyboard.press('Control+z');
    await expect(validation).toContainText('No track tiles placed');

    // Redo
    await page.keyboard.press('Control+y');
    await expect(validation).toContainText('Missing start line');
  });

  test('Clear button resets the entire grid', async ({ page }) => {
    await openEditor(page);
    const validation = page.locator('.te-validation');

    await selectTile(page, 'start-line');
    await paintCell(page, 3, 5);
    await selectTile(page, 'finish-line');
    await paintCell(page, 3, 6);
    await expect(validation).toContainText('Track is valid');

    await page.locator('#track-editor-toolbar button', { hasText: 'Clear' }).click();
    await expect(validation).toContainText('No track tiles placed');
  });

  // --- Save ---

  test('save track to server via inline form', async ({ page, request }) => {
    await openEditor(page);

    // Build a minimal valid track
    await selectTile(page, 'start-line');
    await paintCell(page, 3, 5);
    await selectTile(page, 'finish-line');
    await paintCell(page, 3, 6);

    // Fill name and save
    const nameInput = page.locator('.te-save-input');
    await nameInput.fill('E2E Save Test');
    await page.locator('.te-save-btn').click();

    const status = page.locator('#track-editor-save-status');
    await expect(status).toContainText('Saved: E2E Save Test');
    await expect(status).toHaveClass(/success/);

    // Verify via API — the ID is derived from the name
    const resp = await request.get('/api/tracks/e2e-save-test', {
      headers: { Authorization: `Bearer ${AUTH_TOKEN}` },
    });
    expect(resp.status()).toBe(200);
    const track = await resp.json();
    expect(track.name).toBe('E2E Save Test');
    expect(track.tiles[3][5]).toBe('start-line');
    expect(track.tiles[3][6]).toBe('finish-line');

    // Cleanup
    await request.delete('/api/tracks/e2e-save-test', {
      headers: { Authorization: `Bearer ${AUTH_TOKEN}` },
    });
  });

  test('save shows error when name is empty', async ({ page }) => {
    await openEditor(page);

    const nameInput = page.locator('.te-save-input');
    await nameInput.fill('');
    await page.locator('.te-save-btn').click();

    const status = page.locator('#track-editor-save-status');
    await expect(status).toContainText('Enter a track name');
    await expect(status).toHaveClass(/error/);
  });

  // --- Load ---

  test('load a preset track from the track list', async ({ page }) => {
    await openEditor(page);

    // Open track list
    await page.locator('#track-editor-toolbar button', { hasText: 'Tracks' }).click();
    const trackList = page.locator('.te-track-list');
    await expect(trackList).toBeVisible();
    await expect(trackList.locator('.te-track-list-header')).toContainText('Select Track');

    // Should have at least the 3 presets
    const rows = trackList.locator('.te-track-list-row');
    expect(await rows.count()).toBeGreaterThanOrEqual(3);

    // Load the first track and capture its name
    const firstName = await rows.first().locator('.te-track-list-row-name').textContent();
    await rows.first().locator('.te-track-list-load-btn').click();

    // Track list closes after loading
    await expect(trackList).toBeHidden();

    // Save name input prefilled with loaded track name
    await expect(page.locator('.te-save-input')).toHaveValue(firstName!);

    // Preset tracks are valid
    await expect(page.locator('.te-validation')).toContainText('Track is valid');
  });

  test('track list close button dismisses the modal', async ({ page }) => {
    await openEditor(page);

    await page.locator('#track-editor-toolbar button', { hasText: 'Tracks' }).click();
    const trackList = page.locator('.te-track-list');
    await expect(trackList).toBeVisible();

    await trackList.locator('.te-track-list-close-btn').click();
    await expect(trackList).toBeHidden();
  });

  // --- REST API CRUD ---

  test('track CRUD via REST API', async ({ request }) => {
    const trackId = `e2e-crud-${Date.now()}`;
    const tiles: string[][] = [];
    for (let r = 0; r < 16; r++) {
      tiles.push(new Array(32).fill(''));
    }
    tiles[0][0] = 'start-line';
    tiles[0][1] = 'straight-h';
    tiles[0][2] = 'finish-line';

    const headers = {
      Authorization: `Bearer ${AUTH_TOKEN}`,
      'Content-Type': 'application/json',
    };

    // Create
    const createResp = await request.post('/api/tracks', {
      headers,
      data: { id: trackId, name: 'CRUD Test', width: 32, height: 16, tiles },
    });
    expect(createResp.status()).toBe(201);
    const created = await createResp.json();
    expect(created.id).toBe(trackId);

    // Read
    const getResp = await request.get(`/api/tracks/${trackId}`, { headers });
    expect(getResp.status()).toBe(200);
    const fetched = await getResp.json();
    expect(fetched.name).toBe('CRUD Test');
    expect(fetched.tiles[0][0]).toBe('start-line');

    // Update
    tiles[0][3] = 'straight-h';
    const updateResp = await request.put(`/api/tracks/${trackId}`, {
      headers,
      data: { id: trackId, name: 'CRUD Test Updated', width: 32, height: 16, tiles },
    });
    expect(updateResp.status()).toBe(200);
    const updated = await updateResp.json();
    expect(updated.name).toBe('CRUD Test Updated');

    // List includes our track
    const listResp = await request.get('/api/tracks', { headers });
    expect(listResp.status()).toBe(200);
    const list: { id: string }[] = await listResp.json();
    expect(list.some((t) => t.id === trackId)).toBe(true);

    // Delete
    const deleteResp = await request.delete(`/api/tracks/${trackId}`, { headers });
    expect(deleteResp.status()).toBe(204);

    // Verify gone
    const afterDelete = await request.get(`/api/tracks/${trackId}`, { headers });
    expect(afterDelete.status()).toBe(404);
  });

  test('preset tracks are read-only', async ({ request }) => {
    const headers = {
      Authorization: `Bearer ${AUTH_TOKEN}`,
      'Content-Type': 'application/json',
    };

    // Cannot update a preset
    const updateResp = await request.put('/api/tracks/oval', {
      headers,
      data: { id: 'oval', name: 'Hacked Oval', width: 32, height: 16, tiles: [] },
    });
    expect(updateResp.status()).toBe(403);

    // Cannot delete a preset
    const deleteResp = await request.delete('/api/tracks/oval', { headers });
    expect(deleteResp.status()).toBe(403);
  });
});
