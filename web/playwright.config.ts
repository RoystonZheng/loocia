import { defineConfig, devices } from '@playwright/test'

const port = Number(process.env.PLAYWRIGHT_PORT || 5176)
const baseURL = process.env.PLAYWRIGHT_BASE_URL || `http://127.0.0.1:${port}`
const noProxyEntries = [process.env.NO_PROXY, process.env.no_proxy]
  .flatMap((value) => (value ? value.split(',') : []))
  .concat(['127.0.0.1', 'localhost'])
const loopbackNoProxy = [...new Set(noProxyEntries.map((value) => value.trim()).filter(Boolean))].join(',')

process.env.NO_PROXY = loopbackNoProxy
process.env.no_proxy = loopbackNoProxy

export default defineConfig({
  testDir: './e2e',
  timeout: 30_000,
  expect: { timeout: 5_000 },
  fullyParallel: false,
  reporter: [
    ['list', { printSteps: true }],
    ['json', { outputFile: 'test-results/ai-tool-cases.json' }],
    ['html', { outputFolder: 'playwright-report', open: 'never' }],
  ],
  use: {
    baseURL,
    channel: process.env.PLAYWRIGHT_CHANNEL || 'chrome',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  projects: [
    {
      name: 'desktop-chrome',
      use: {
        ...devices['Desktop Chrome'],
        viewport: { width: 1440, height: 900 },
      },
    },
  ],
  webServer: process.env.PLAYWRIGHT_BASE_URL ? undefined : {
    command: `npm run dev -- --host 127.0.0.1 --port ${port}`,
    url: baseURL,
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
})
