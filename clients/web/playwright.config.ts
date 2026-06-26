import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  // Serial by default: the suite has many timing-sensitive tests (WebSocket
  // ACK timeouts, route-delayed uploads, cross-context realtime) that flake
  // under parallel resource contention against a shared backend. Reliability
  // of the signal matters more than the ~20s parallel saves (full run ~50s).
  workers: 1,
  // One retry: 49 real-time tests against one backend instance accumulate
  // WS/presence state and occasionally trip timing-sensitive assertions on the
  // first try. A retry that passes is reported as "flaky" (distinct from a
  // clean pass) — a real regression fails twice, so this doesn't mask bugs.
  retries: 1,
  reporter: 'html',
  timeout: 30000,
  use: {
    baseURL: process.env.E2E_BASE_URL || 'http://localhost:3000',
    trace: 'on-first-retry',
    locale: 'zh-CN',
  },
  // No webServer — Docker Compose should already be running.
  projects: [
    {
      name: 'chromium',
      use: { browserName: 'chromium' },
    },
  ],
});
