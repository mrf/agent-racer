import { defineConfig } from '@playwright/test';

const SERVER_PORT = 8077;

export default defineConfig({
  testDir: './tests',
  timeout: 30_000,
  retries: 1,
  use: {
    baseURL: `http://localhost:${SERVER_PORT}`,
    screenshot: 'only-on-failure',
  },
  webServer: {
    command: `cd ../backend && go run ./cmd/server --mock --dev --port ${SERVER_PORT} --config ../config.yaml`,
    port: SERVER_PORT,
    reuseExistingServer: !process.env.CI,
    timeout: 30_000,
  },
});
