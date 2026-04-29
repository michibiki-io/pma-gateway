import { defineConfig, devices } from '@playwright/test';

const baseURL = process.env.E2E_BASE_URL ?? 'http://127.0.0.1:3000';

export default defineConfig({
  testDir: './tests/e2e',
  timeout: 30_000,
  expect: {
    timeout: 5_000,
  },
  reporter: [
    ['html', { open: 'never', outputFolder: 'playwright-report' }],
    ['list'],
  ],
  outputDir: 'test-results',
  webServer:
    process.env.E2E_SKIP_WEBSERVER === 'true' || process.env.E2E_REAL_APP === 'true'
      ? undefined
      : {
          command: 'npm run dev -- --host 0.0.0.0 --port 3000',
          url: baseURL,
          reuseExistingServer: !process.env.CI,
          timeout: 120_000,
        },
  use: {
    baseURL,
    headless: true,
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
    trace: 'retain-on-failure',
  },
  projects: [
    {
      name: 'desktop-chrome',
      use: {
        ...devices['Desktop Chrome'],
        // The official Linux Playwright image includes Playwright Chromium,
        // not the branded Google Chrome channel.
      },
    },
    {
      name: 'desktop-webkit',
      use: {
        ...devices['Desktop Safari'],
      },
    },
    {
      name: 'mobile-chrome',
      use: {
        ...devices['Pixel 5'],
      },
    },
    {
      name: 'mobile-safari',
      use: {
        ...devices['iPhone 13'],
      },
    },
  ],
});
