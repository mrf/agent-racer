import { defineConfig, devices } from '@playwright/test';

const SERVER_PORT = 8077;
export default defineConfig({
  testDir: './tests',
  timeout: 90_000,
  retries: 1,
  reporter: [['html', { open: 'never' }], ['list']],
  use: {
    baseURL: `http://localhost:${SERVER_PORT}`,
    viewport: { width: 1280, height: 720 },
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
  },
  webServer: {
    command: `cd ../backend && go run ./cmd/server --mock --dev --port ${SERVER_PORT} --config ../e2e/e2e-config.yaml`,
    port: SERVER_PORT,
    reuseExistingServer: !process.env.CI,
    timeout: 60_000,
  },
  projects: [
    { name: 'chromium', device: 'Desktop Chrome' },
    { name: 'firefox', device: 'Desktop Firefox' },
  ].flatMap(({ name, device }) => [
    {
      name,
      testIgnore: /connection-status/,
      use: { ...devices[device] },
    },
    {
      name: `${name}-connection`,
      testMatch: /connection-status/,
      dependencies: [name],
      use: { ...devices[device] },
    },
  ]),
});
