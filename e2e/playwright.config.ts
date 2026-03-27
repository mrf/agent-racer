import { defineConfig, devices } from '@playwright/test';
import { randomBytes } from 'node:crypto';
import { readFileSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';

const SERVER_PORT = 8077;
const GENERATED_CONFIG = '.e2e-config.generated.yaml';

// Generate a random auth token for each test run so no static token is committed.
const E2E_AUTH_TOKEN = randomBytes(16).toString('hex');
process.env.E2E_AUTH_TOKEN = E2E_AUTH_TOKEN;

// Write a generated config that includes the random token.
const baseConfig = readFileSync(resolve(__dirname, 'e2e-config.yaml'), 'utf-8');
const generatedConfig = baseConfig.replace(
  /^(server:\n)/m,
  `$1  auth_token: "${E2E_AUTH_TOKEN}"\n`,
);
if (generatedConfig === baseConfig) {
  throw new Error('Failed to inject auth_token into e2e config — regex did not match');
}
const generatedConfigPath = resolve(__dirname, GENERATED_CONFIG);
writeFileSync(generatedConfigPath, generatedConfig);

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
    command: `cd ../backend && go run ./cmd/server --mock --dev --port ${SERVER_PORT} --config ../e2e/${GENERATED_CONFIG}`,
    port: SERVER_PORT,
    reuseExistingServer: !process.env.CI,
    timeout: 60_000,
  },
  projects: [
    {
      name: 'chromium',
      testIgnore: /connection-status/,
      use: { ...devices['Desktop Chrome'] },
    },
    {
      name: 'chromium-connection',
      testMatch: /connection-status/,
      dependencies: ['chromium'],
      use: { ...devices['Desktop Chrome'] },
    },
  ],
});
