import { expect, test } from '@playwright/test'

async function login(page) {
  const response = await page.request.post('/api/auth/login', {
    headers: { 'X-Image-Agent-Client': 'mp-weixin' },
    data: {
      username: 'workspace_e2e',
      password: 'test-password'
    }
  })
  expect(response.ok()).toBeTruthy()
}

test('guest can browse discovery and protected actions open login flow', async ({ page }) => {
  await page.goto('/workspace')

  await expect(page.getByTestId('workspace-discovery-panel')).toContainText('AI 工具')
  await expect(page.getByTestId('workspace-tool-expand')).toBeVisible()
  await expect(page.getByTestId('workspace-feature-ai-commerce')).toContainText('AI 电商')

  await page.getByTestId('workspace-prompt-input').fill('访客尝试创建')
  await page.getByTestId('workspace-create-button').click()
  await expect(page.getByTestId('auth-modal')).toContainText('需要登录才能使用该功能')
  await expect(page).toHaveURL(/\/workspace/)
})

test('AI commerce entry focuses the ecommerce workflow and upcoming entries are marked', async ({ page }) => {
  await login(page)
  await page.goto('/workspace')

  await expect(page.getByTestId('workspace-feature-ai-canvas')).toContainText('即将开放')
  await expect(page.getByTestId('workspace-feature-skill-hub')).toContainText('即将开放')
  await expect(page.getByTestId('workspace-feature-text-edit')).toContainText('即将开放')

  await page.getByTestId('workspace-discovery-filter-tool').click()
  await expect(page.getByTestId('workspace-ecommerce-workflow')).toBeHidden()
  await page.getByTestId('workspace-feature-ai-commerce').click()
  await expect(page.getByTestId('workspace-discovery-filter-image')).toHaveClass(/active/)
  await expect(page.getByTestId('workspace-ecommerce-workflow')).toBeVisible()
})

test('logged-in text-to-image generation uses local stub and returns credit state', async ({ page }) => {
  await login(page)
  await page.goto('/workspace')

  await page.getByTestId('workspace-prompt-input').fill('E2E stub image generation')
  await page.getByTestId('workspace-create-button').click()

  await expect(page.getByTestId('workspace-generation-tasks')).toBeVisible()
  await expect(page.getByTestId('workspace-result-preview')).toContainText(/生成中|创建您的第一个创作/)
  await expect(page.locator('.preview-image')).toBeVisible({ timeout: 15_000 })
  await expect(page.getByTestId('workspace-result-error')).toHaveCount(0)
})

test('home advanced options open as a solid overlay without covering upload guidance', async ({ page }, testInfo) => {
  await login(page)
  await page.goto('/workspace')

  const controlBar = page.getByTestId('workspace-home-control-bar')
  const uploadCard = page.getByTestId('workspace-reference-upload')
  const toggle = controlBar.getByTestId('workspace-home-advanced-toggle')

  await expect(toggle).toBeVisible()
  await expect(toggle).toHaveAttribute('aria-expanded', 'false')
  await toggle.click()

  const panel = page.getByTestId('workspace-home-advanced-panel')
  await expect(toggle).toHaveAttribute('aria-expanded', 'true')
  await expect(panel).toBeVisible()
  await expect(panel.getByTestId('workspace-negative-prompt')).toBeVisible()
  await expect(panel.getByRole('button', { name: '国风' })).toBeVisible()

  const backgroundColor = await panel.evaluate((node) => window.getComputedStyle(node).backgroundColor)
  expect(backgroundColor).not.toMatch(/rgba\([^)]*,\s*0(?:\.\d+)?\s*\)$/)

  const viewport = testInfo.project.use.viewport
  const panelBox = await panel.boundingBox()
  const uploadBox = await uploadCard.boundingBox()
  expect(panelBox).not.toBeNull()
  expect(uploadBox).not.toBeNull()

  if ((viewport?.width ?? 1440) <= 720) {
    expect(panelBox.y + panelBox.height).toBeGreaterThan((viewport?.height ?? 844) - 140)
  } else {
    const overlapsUpload = !(
      panelBox.x + panelBox.width <= uploadBox.x
      || uploadBox.x + uploadBox.width <= panelBox.x
      || panelBox.y + panelBox.height <= uploadBox.y
      || uploadBox.y + uploadBox.height <= panelBox.y
    )
    expect(overlapsUpload).toBe(false)
  }

  await panel.getByTestId('workspace-negative-prompt').fill('blur, watermark')
  await page.keyboard.press('Escape')
  await expect(panel).toHaveCount(0)
  await expect(toggle).toHaveAttribute('aria-expanded', 'false')

  await toggle.click()
  await expect(panel).toBeVisible()
  await page.locator('body').click({ position: { x: 4, y: 4 } })
  await expect(panel).toHaveCount(0)
})

test('direct upload can be route-mocked without touching OSS', async ({ page }) => {
  await login(page)

  // 上传完成后前端会重新拉取参考素材列表做权威同步，
  // 因此列表接口也必须在 complete-upload 之后返回 mock 资产，否则乐观更新会被清空。
  let uploadCompleted = false
  const mockedAsset = {
    id: 999,
    preview_url: '/api/reference-assets/999/file',
    original_filename: 'playwright-upload.png',
    mime_type: 'image/png'
  }
  await page.route('**/api/reference-assets', async (route) => {
    if (route.request().method() !== 'GET' || !uploadCompleted) {
      await route.fallback()
      return
    }
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ items: [mockedAsset] })
    })
  })
  await page.route('**/api/reference-assets/upload-policy', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        upload_url: '/workspace-e2e-oss-upload',
        object_key: 'e2e/uploaded.png',
        upload_token: 'e2e-token',
        form_data: { key: 'e2e/uploaded.png' }
      })
    })
  })
  await page.route('**/workspace-e2e-oss-upload', async (route) => {
    await route.fulfill({ status: 201, body: '' })
  })
  await page.route('**/api/reference-assets/complete-upload', async (route) => {
    uploadCompleted = true
    await route.fulfill({
      status: 201,
      contentType: 'application/json',
      body: JSON.stringify(mockedAsset)
    })
  })
  // 选中参考图变化会触发点数估算；真实后端不认识 mock 的资产 999，
  // 会返回 reference_asset_not_found 并导致前端清空选中态，因此估算也必须 mock。
  await page.route('**/api/images/generations/estimate', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        required_credits: 1,
        available_credits: 100,
        missing_credits: 0,
        enough: true,
        recommended_package: null
      })
    })
  })

  await page.goto('/workspace')
  await page.getByTestId('workspace-tool-expand').click()
  await page.getByTestId('workspace-reference-file-input').setInputFiles({
    name: 'playwright-upload.png',
    mimeType: 'image/png',
    buffer: Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=', 'base64')
  })

  // 新版上传区以缩略图堆栈 + 计数展示已选参考图，不再显示文件名文本。
  await expect(page.getByTestId('workspace-reference-stack-thumb').first()).toBeVisible()
})
