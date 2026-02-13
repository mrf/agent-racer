import { test, expect } from '@playwright/test';
import { execSync, spawn } from 'node:child_process';
import { resolve } from 'node:path';

const SERVER_PORT = 8077;
const BACKEND_DIR = resolve(__dirname, '../../backend');
const TIMEOUT_WS = 10_000;
const TIMEOUT_RECONNECT = 30_000;
const TIMEOUT_SERVER_STARTUP = 20_000;

/** Kill all processes listening on the given TCP port. */
function killPort(port: number): void {
  try {
    execSync(`lsof -ti tcp:${port} | xargs kill -TERM 2>/dev/null`, {
      stdio: 'ignore',
    });
  } catch {
    // Expected when no process is listening on the port
  }
}

/** Wait until the server is accepting HTTP requests. */
async function waitForServer(
  port: number,
  timeout = TIMEOUT_SERVER_STARTUP,
): Promise<void> {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    try {
      await fetch(`http://localhost:${port}/`);
      return;
    } catch {
      await new Promise((done) => setTimeout(done, 250));
    }
  }
  throw new Error(`Server on port ${port} did not start within ${timeout}ms`);
}

test.describe('Connection status indicator', () => {
  test('shows connected when WebSocket is open', async ({ page }) => {
    await page.goto('/');
    const statusDot = page.locator('#connection-status');
    await expect(statusDot).toHaveClass(/connected/, { timeout: TIMEOUT_WS });
  });

  test('shows disconnected after backend stops, reconnects after restart', async ({
    page,
  }) => {
    test.setTimeout(60_000);

    await page.goto('/');
    const statusDot = page.locator('#connection-status');

    // 1. Verify initially connected
    await expect(statusDot).toHaveClass(/connected/, { timeout: TIMEOUT_WS });

    // 2. Stop the backend
    killPort(SERVER_PORT);

    // 3. Verify indicator changes to disconnected
    await expect(statusDot).toHaveClass(/disconnected/, {
      timeout: TIMEOUT_WS,
    });

    // 4. Restart the backend
    const server = spawn(
      'go',
      [
        'run',
        './cmd/server',
        '--mock',
        '--dev',
        '--port',
        String(SERVER_PORT),
      ],
      { cwd: BACKEND_DIR, stdio: 'ignore', detached: true },
    );
    server.unref();
    await waitForServer(SERVER_PORT);

    // 5. Verify reconnection restores connected status
    await expect(statusDot).toHaveClass(/connected/, {
      timeout: TIMEOUT_RECONNECT,
    });
  });
});
