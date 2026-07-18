import { expect, test } from '@playwright/test'

const clientHeaders = { 'X-Image-Agent-Client': 'mp-weixin' }

async function login(page) {
  const response = await page.request.post('http://127.0.0.1:8889/api/auth/login', {
    headers: clientHeaders,
    data: { username: 'workspace_e2e', password: 'test-password' }
  })
  expect(response.ok()).toBeTruthy()
}

test('AI 电商顶部导航仅在桌面和平板吸顶', async ({ page }, testInfo) => {
  await login(page)
  await page.goto('/workspace/ai-commerce')

  const topbar = page.getByTestId('commerce-creator-topbar')
  await expect(topbar).toBeVisible()

  if (testInfo.project.name === 'mobile') {
    await expect(topbar).toHaveCSS('position', 'static')
    return
  }

  await expect(topbar).toHaveCSS('position', 'sticky')
  const content = page.locator('.workspace-content')
  const scrollState = await content.evaluate((element) => {
    element.scrollTo({ top: element.scrollHeight / 2, behavior: 'instant' })
    return { scrollHeight: element.scrollHeight, clientHeight: element.clientHeight }
  })
  expect(scrollState.scrollHeight).toBeGreaterThan(scrollState.clientHeight)
  const stickyTop = await topbar.evaluate((element) => element.getBoundingClientRect().top)
  await content.evaluate((element) => element.scrollTo({ top: element.scrollHeight, behavior: 'instant' }))
  await expect.poll(() => topbar.evaluate((element) => element.getBoundingClientRect().top)).toBeCloseTo(stickyTop, 0)

  const resultPaneTop = await page.getByTestId('commerce-result-pane').evaluate((element) => element.getBoundingClientRect().top)
  const topbarBottom = await topbar.evaluate((element) => element.getBoundingClientRect().bottom)
  expect(resultPaneTop).toBeGreaterThanOrEqual(topbarBottom)
})

test('AI 电商桌面端创作工具栏四项水平对齐且控件等高', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name === 'mobile')
  await login(page)
  await page.goto('/workspace/ai-commerce')

  const items = [
    page.getByTestId('commerce-project-label'),
    page.getByTestId('commerce-project-select'),
    page.getByTestId('commerce-project-new'),
    page.getByTestId('commerce-project-title'),
  ]
  const boxes = await Promise.all(items.map((item) => item.boundingBox()))
  expect(boxes.every(Boolean)).toBeTruthy()

  const centers = boxes.map((box) => box.y + box.height / 2)
  expect(Math.max(...centers) - Math.min(...centers)).toBeLessThanOrEqual(1)
  expect(boxes[1].height).toBe(boxes[2].height)
  for (let index = 1; index < boxes.length; index += 1) {
    expect(boxes[index].x).toBeGreaterThanOrEqual(boxes[index - 1].x + boxes[index - 1].width)
  }

  const title = items[3]
  await expect(title).toHaveCSS('white-space', 'nowrap')
  await expect(title).toHaveCSS('overflow', 'hidden')
  await expect(title).toHaveCSS('text-overflow', 'ellipsis')
})

test('AI 电商工作区和 Teleport 弹窗跟随全局主题', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'desktop')
  await page.addInitScript(() => window.localStorage.setItem('image_agent_user_theme:v1', 'light'))
  await login(page)
  await page.goto('/workspace/ai-commerce')

  const shell = page.getByTestId('commerce-creator-shell')
  const console = page.getByTestId('commerce-production-console')
  await expect(shell).toHaveAttribute('data-theme', 'light')
  await expect(console).toHaveAttribute('data-theme', 'light')
  await expect(shell).toHaveCSS('background-color', 'rgb(244, 247, 251)')
  expect(await page.getByTestId('commerce-creator-topbar').evaluate((element) => getComputedStyle(element).getPropertyValue('--commerce-surface').trim())).toBe('#ffffff')

  await page.getByTestId('commerce-open-cases').click()
  const dialog = page.getByTestId('commerce-cases-dialog')
  await expect(dialog).toHaveAttribute('data-theme', 'light')
  await expect(dialog.locator('.commerce-dialog')).toHaveCSS('background-color', 'rgb(255, 255, 255)')
  await dialog.getByRole('button', { name: '关闭案例库' }).click()

  await page.getByTestId('site-theme-toggle').click()
  await expect(shell).toHaveAttribute('data-theme', 'dark')
  await expect(console).toHaveAttribute('data-theme', 'dark')
  await expect(shell).toHaveCSS('background-color', 'rgb(7, 9, 12)')
})

test('AI 电商保留左侧创作流程并将右侧改为响应式生产控制台', async ({ page }, testInfo) => {
  await login(page)
  await page.goto('/workspace/ai-commerce')

  const createPane = page.getByTestId('commerce-create-pane')
  await expect(createPane.locator('.creator-step')).toHaveCount(4)
  for (const heading of ['上传商品，生成分析报告', '规格管理', '核对商品报告', '配置商品详情页']) {
    await expect(createPane.getByRole('heading', { name: heading, exact: true })).toBeVisible()
  }
  if (testInfo.project.name === 'mobile') await page.getByTestId('commerce-mobile-tab-results').click()
  const console = page.getByTestId('commerce-production-console')
  await expect(console).toBeVisible()
  await expect(console.locator('[data-testid^="commerce-console-card-"]')).toHaveCount(5)

  const cardGrid = console.locator('.console-card-grid')
  if (testInfo.project.name === 'mobile') {
    await expect(page.getByTestId('commerce-open-fullscreen')).toBeHidden()
    const columns = await cardGrid.evaluate((element) => getComputedStyle(element).gridTemplateColumns.split(' ').length)
    expect(columns).toBe(1)
    await page.getByTestId('commerce-open-cases').click()
    const dialog = page.getByTestId('commerce-cases-dialog')
    await expect(dialog).toBeVisible()
    await expect(dialog.locator('.commerce-dialog')).toHaveCSS('width', `${testInfo.project.use.viewport.width}px`)
    await dialog.getByRole('button', { name: '关闭案例库' }).click()
    return
  }

  const columns = await cardGrid.evaluate((element) => getComputedStyle(element).gridTemplateColumns.split(' ').length)
  expect(columns).toBe(2)
  await expect(console.getByTestId('commerce-console-card-latest')).toHaveCSS('grid-column-start', '1')

  await page.getByTestId('commerce-open-cases').click()
  await expect(page.getByTestId('commerce-cases-dialog')).toBeVisible()
  await page.getByTestId('commerce-cases-dialog').getByRole('button', { name: '关闭案例库' }).click()

  const fullscreenTrigger = page.getByTestId('commerce-open-fullscreen')
  await fullscreenTrigger.click()
  const fullscreen = page.getByTestId('commerce-fullscreen-console')
  await expect(fullscreen).toBeVisible()
  await expect(page.locator('body')).toHaveCSS('overflow', 'hidden')
  await fullscreen.getByRole('button', { name: '历史记录' }).click()
  await expect(page.getByTestId('commerce-history-dialog')).toBeVisible()
  await page.getByTestId('commerce-history-dialog').getByRole('button', { name: '关闭历史记录' }).click()
  await page.keyboard.press('Escape')
  await expect(fullscreen).toBeHidden()
  await expect(fullscreenTrigger).toBeFocused()
})
