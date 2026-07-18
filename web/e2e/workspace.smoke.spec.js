import { expect, test } from '@playwright/test'

test('workspace page and discovery API are available', async ({ page, request, baseURL }) => {
  const discovery = await request.get('/api/workspace/discovery')
  expect(discovery.ok()).toBeTruthy()
  const payload = await discovery.json()
  expect(Array.isArray(payload.tools)).toBeTruthy()
  expect(Array.isArray(payload.models)).toBeTruthy()
  expect(Array.isArray(payload.hot)).toBeTruthy()
  expect(Array.isArray(payload.inspiration)).toBeTruthy()

  await page.goto('/workspace')
  await expect(page.getByTestId('workspace-discovery-panel')).toBeVisible()
  await expect(page.getByTestId('workspace-tool-expand')).toBeVisible()

  const username = process.env.SMOKE_USER_USERNAME
  const password = process.env.SMOKE_USER_PASSWORD
  if (!username || !password) {
    test.info().annotations.push({
      type: 'smoke-login',
      description: 'Skipped login estimate because SMOKE_USER_USERNAME/SMOKE_USER_PASSWORD are not set'
    })
    return
  }

  const login = await request.post('/api/auth/login', {
    headers: { 'X-Image-Agent-Client': 'mp-weixin' },
    data: { username, password }
  })
  expect(login.ok()).toBeTruthy()

  const estimate = await request.post('/api/images/generations/estimate', {
    data: {
      prompt: '只做 smoke 点数估算，不提交生成',
      aspect_ratio: '1:1',
      tool_mode: 'generate'
    }
  })
  expect(estimate.ok(), `${baseURL} estimate should succeed without creating a generation`).toBeTruthy()
  const estimatePayload = await estimate.json()
  expect(estimatePayload.required_credits).toBeGreaterThan(0)
})
