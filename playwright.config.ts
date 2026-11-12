import { defineConfig, devices } from '@playwright/test';

// The port MUST be identical in every process that loads this config (runner
// and workers each evaluate it); a dynamically allocated port would differ per
// process and the tests would target a port the server never bound.
// Override with TEST_PORT when 8931 is taken.
const port = Number(process.env.TEST_PORT ?? 8931);
const baseURL = process.env.BASE_URL ?? `http://127.0.0.1:${port}`;

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
        command: [
          `PORT=${port}`,
          'APP_ENV=test',
          `BASE_URL=http://127.0.0.1:${port}`,
          'DB_PATH=./test-run.db',
          'PHOTO_DIR=./test-run-photos',
          'SESSION_KEY=dGVzdC1zZXNzaW9uLWtleS0zMi1ieXRlcy1hYQ==',
          'IP_HASH_KEY=dGVzdC1pcGhhc2gta2V5LTMyLWJ5dGVzLWJi',
          'ADMIN_TOKEN=dGVzdC1hZG1pbi10b2tlbi0zMi1ieXRl',
          'TOWN_BBOX=42.70,-71.09,42.82,-70.96',
          'TZ=America/New_York',
          'bin/server',
        ].join(' '),
        url: `http://127.0.0.1:${port}/healthz`,
        reuseExistingServer: false,
        timeout: 30_000,
      },
});
