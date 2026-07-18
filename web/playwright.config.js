import { defineConfig, devices } from '@playwright/test'

const baseURL = process.env.PLAYWRIGHT_BASE_URL || 'http://127.0.0.1:15173'

export default defineConfig({
  testDir: './e2e',
  // 所有用例共享同一个 workspace_e2e 账号（点数、生成并发锁、参考素材），
  // 并行执行会互相争抢导致偶发失败，固定串行保证确定性。
  workers: 1,
  timeout: 30_000,
  expect: {
    timeout: 8_000
  },
  reporter: [['list'], ['html', { open: 'never' }]],
  use: {
    baseURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure'
  },
  projects: [
    {
      name: 'desktop',
      testIgnore: /.*\.smoke\.spec\.js/,
      use: {
        ...devices['Desktop Chrome'],
        viewport: { width: 1440, height: 900 }
      }
    },
    {
      name: 'tablet',
      testIgnore: /.*\.smoke\.spec\.js/,
      use: {
        ...devices['Desktop Chrome'],
        viewport: { width: 768, height: 1024 },
        isMobile: false,
        hasTouch: true
      }
    },
    {
      name: 'mobile',
      testIgnore: /.*\.smoke\.spec\.js/,
      use: {
        ...devices['Pixel 5'],
        viewport: { width: 390, height: 844 }
      }
    },
    {
      name: 'smoke',
      testMatch: /.*\.smoke\.spec\.js/,
      use: {
        ...devices['Desktop Chrome'],
        viewport: { width: 1440, height: 900 }
      }
    }
  ]
})
