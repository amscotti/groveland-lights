import { defineConfig, devices } from '@playwright/test';

const TEST_PORT = Number(process.env.TEST_PORT ?? 0);

async function freePort(): Promise<number> {
  if (TEST_PORT > 0) return TEST_PORT;
  const { createServer } = await import('node:net');
  return new Promise((resolve) => {
    const srv = createServer();
    srv.listen(0, '127.0.0.1', () => {
      const addr = srv.address();
      const port = typeof addr === 'object' && addr ? addr.port : 8931;
      srv.close(() => resolve(port));
    });
  });
}

const port = await freePort();
const baseURL = process.env.BASE_URL ?? `http://localhost:${port}`;

export default defineConfig({
  testDir: 'acceptance/e2e',
  timeout: 45_000,
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  reporter: [['list']],
  use: {
    baseURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  expect: {
    toHaveScreenshot: { maxDiffPixelRatio: 0.01, animations: 'disabled' },
  },
  projects: [
    {
      name: 'desktop-chromium',
      use: { ...devices['Desktop Chrome'] },
    },
    {
      name: 'pixel-9',
      use: { ...devices['Pixel 9'] },
    },
    {
      name: 'iphone-16',
      use: { ...devices['iPhone 16'], browserName: 'webkit' },
    },
    {
      name: 'iphone-se-320',
      use: {
        browserName: 'chromium',
        viewport: { width: 320, height: 640 },
        isMobile: true,
        hasTouch: true,
      },
    },
  ],
  webServer: process.env.BASE_URL
    ? undefined
    : {
        command: 'bin/server',
        url: baseURL,
        reuseExistingServer: false,
        timeout: 30_000,
        env: {
          PORT: String(port),
          APP_ENV: 'test',
          BASE_URL: baseURL,
          DB_PATH: './test-run.db',
          PHOTO_DIR: './test-run-photos',
          SESSION_KEY: 'dGVzdC1zZXNzaW9uLWtleS0zMi1ieXRlcy1hYQ==',
          IP_HASH_KEY: 'dGVzdC1pcGhhc2gta2V5LTMyLWJ5dGVzLWJi',
          ADMIN_TOKEN: 'dGVzdC1hZG1pbi10b2tlbi0zMi1ieXRl',
          TOWN_BBOX: '42.70,-71.09,42.82,-70.96',
          TZ: 'America/New_York',
        },
      },
});
