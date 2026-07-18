import { expect, test } from '@playwright/test'

async function login(page) {
  const response = await page.request.post('/api/auth/login', {
    headers: { 'X-Image-Agent-Client': 'mp-weixin' },
    data: { username: 'workspace_e2e', password: 'test-password' }
  })
  expect(response.ok()).toBeTruthy()
}

const conversation = { id: 91, title: '验收产品宣传片', is_favorite: false, last_activity_at: '2026-07-12T01:00:00Z' }
const generations = [
  { id: 101, generation_record_id: 101, work_id: 201, prompt: '历史版本提示词', runtime_model: 'seedance', model_name: 'Seedance', status: 'succeeded', preview_url: '/e2e/video-1.mp4', download_url: '/e2e/video-1.mp4', aspect_ratio: '16:9', resolution: '1080p', duration_seconds: 5, created_at: '2026-07-12T01:01:00Z' },
  { id: 102, generation_record_id: 102, work_id: 202, prompt: '最新版本提示词', runtime_model: 'seedance', model_name: 'Seedance', status: 'succeeded', preview_url: '/e2e/video-2.mp4', download_url: '/e2e/video-2.mp4', aspect_ratio: '16:9', resolution: '720p', duration_seconds: 5, created_at: '2026-07-12T01:02:00Z' }
]

async function mockVideoWorkspace(page, { conversationCount = 1, conversationDelayMs = 0 } = {}) {
  let videoGenerationCalls = 0
  const conversationItems = Array.from({ length: conversationCount }, (_, index) => ({
    ...conversation,
    id: conversation.id + index,
    title: index === 0 ? conversation.title : `多会话账号记录 ${index + 1}`,
    last_activity_at: new Date(Date.parse(conversation.last_activity_at) - index * 60_000).toISOString(),
  }))
  await page.route('**/api/videos/generations/async', async route => {
    videoGenerationCalls += 1
    await route.fulfill({ status: 500, json: { error: { message: '测试禁止生成视频' } } })
  })
  await page.route('**/api/videos/models', route => route.fulfill({ json: { items: [{ name: 'Doubao Seedance 2.0 Mini 超长模型名称', runtime_model: 'seedance', durations: ['1', '3', '6', '10', '15'], default_duration: '3' }] } }))
  await page.route('**/api/reference-assets**', route => route.fulfill({ json: { items: [{ id: 1, original_filename: '参考图.png', preview_url: '/e2e/reference.png', mime_type: 'image/png' }] } }))
  await page.route('**/api/videos/*/soundtracks**', async route => {
    if (route.request().method() === 'GET') return route.fulfill({ json: { items: [] } })
    return route.fulfill({ json: { id: 1, audio_url: '/e2e/audio.mp3', download_url: '/e2e/audio.mp3', source: 'ai' } })
  })
  await page.route('**/api/videos/conversations**', async route => {
    const request = route.request()
    const url = new URL(request.url())
    if (request.method() === 'GET' && /\/conversations\/91$/.test(url.pathname)) {
      return route.fulfill({ json: { conversation, timeline: generations.map(generation => ({ type: 'generation', generation })) } })
    }
    if (request.method() === 'GET') {
      if (conversationDelayMs) await new Promise(resolve => setTimeout(resolve, conversationDelayMs))
      return route.fulfill({ json: { items: conversationItems, total: conversationItems.length, page: 1, page_size: 30 } })
    }
    if (request.method() === 'PATCH') return route.fulfill({ json: { ...conversation, is_favorite: true } })
    if (request.method() === 'POST' && url.pathname.endsWith('/messages')) {
      return route.fulfill({ status: 201, json: { message: { id: 301, role: 'user', content: '策划产品片', status: 'answered' }, reply: { id: 302, role: 'assistant', content: '建议采用慢推镜头', suggested_prompt: '产品慢推镜头', quick_replies: ['改成竖屏'] } } })
    }
    return route.fulfill({ status: 201, json: conversation })
  })
  return () => videoGenerationCalls
}

async function responsiveLayout(page) {
  return page.evaluate(() => {
    const main = document.querySelector('.video-chat-main')
    const toolbar = document.querySelector('.video-chat-toolbar')
    const timeline = document.querySelector('.video-chat-timeline')
    const composer = document.querySelector('.video-unified-composer')
    const rail = document.querySelector('.video-conversation-rail')
    const railList = document.querySelector('.video-conversation-list')
    const workspace = document.querySelector('.video-chat-workspace')
    const conversationButton = document.querySelector('.video-mobile-conversations')
    const mainRect = main.getBoundingClientRect()
    const timelineRect = timeline.getBoundingClientRect()
    const composerRect = composer.getBoundingClientRect()
    const railRect = rail.getBoundingClientRect()
    const workspaceRect = workspace.getBoundingClientRect()
    const modeButtons = [...composer.querySelectorAll('.video-mode-switch button')]
    const cost = composer.querySelector('.video-cost')
    const send = composer.querySelector('.video-send')
    const fields = [...composer.querySelectorAll('.video-chat-composer-field')]
    const controls = [...modeButtons, cost, send, ...fields]
    const rects = controls.map((node) => node.getBoundingClientRect())
    return {
      mainOverflow: main.scrollWidth - main.clientWidth,
      toolbarOverflow: toolbar.scrollWidth - toolbar.clientWidth,
      composerOverflow: composer.scrollWidth - composer.clientWidth,
      composerContained:
        composerRect.left >= mainRect.left &&
        composerRect.right <= mainRect.right &&
        composerRect.top >= mainRect.top &&
        composerRect.bottom <= mainRect.bottom,
      workspaceHeight: workspaceRect.height,
      mainHeight: mainRect.height,
      railHeight: railRect.height,
      railListScrollable: railList.scrollHeight > railList.clientHeight,
      timelineBeforeComposer: timelineRect.bottom <= composerRect.top + 1,
      timelineScrollable: timeline.scrollHeight > timeline.clientHeight,
      railTransform: getComputedStyle(rail).transform,
      conversationButtonDisplay: getComputedStyle(conversationButton).display,
      toolbarButtonWhiteSpace: [...toolbar.querySelectorAll('button')].map((button) => getComputedStyle(button).whiteSpace),
      modeButtons: modeButtons.map((button) => ({
        width: button.getBoundingClientRect().width,
        whiteSpace: getComputedStyle(button).whiteSpace,
        wraps: button.scrollHeight > button.clientHeight,
      })),
      cost: {
        width: cost.getBoundingClientRect().width,
        whiteSpace: getComputedStyle(cost).whiteSpace,
        wraps: cost.scrollHeight > cost.clientHeight,
      },
      sendSize: { width: send.getBoundingClientRect().width, height: send.getBoundingClientRect().height },
      parameterFields: fields.map((field) => ({
        width: field.getBoundingClientRect().width,
        display: getComputedStyle(field.querySelector('select')).display,
        selectWidth: field.querySelector('select').getBoundingClientRect().width,
      })),
      controlsContained: rects.every((rect) => rect.left >= composerRect.left && rect.right <= composerRect.right && rect.top >= composerRect.top && rect.bottom <= composerRect.bottom),
      parameterColumns: getComputedStyle(composer.querySelector('.video-chat-composer-params')).gridTemplateColumns.split(' ').length,
    }
  })
}

test('video conversation workspace supports non-generation workflows', async ({ page }, testInfo) => {
  await login(page)
  const getVideoGenerationCalls = await mockVideoWorkspace(page)
  await page.goto('/workspace/video?conversation=91')

  await expect(page.getByTestId('video-conversation-workspace')).toBeVisible()
  await expect(page.locator('.video-unified-composer')).toBeVisible()
  await expect(page.getByLabel('视频时长')).toHaveValue('3')
  await expect(page.getByLabel('视频时长').locator('option')).toHaveCount(5)
  const isCompactViewport = testInfo.project.name === 'tablet' || testInfo.project.name === 'mobile'
  if (isCompactViewport) {
    await expect(page.getByRole('button', { name: '会话', exact: true })).toBeVisible()
    await page.getByRole('button', { name: '会话', exact: true }).click()
    await expect(page.locator('.video-conversation-rail')).toHaveClass(/open/)
    await page.getByRole('button', { name: '关闭会话列表' }).click()
  } else {
    await expect(page.getByTestId('workspace-sidebar-shell')).toHaveCSS('width', '260px')
    await expect(page.locator('.video-conversation-rail')).toHaveCSS('width', '292px')
  }

  await page.locator('.video-chat-toolbar').getByRole('button', { name: '素材库' }).click()
  await expect(page.getByRole('heading', { name: '选择参考素材' })).toBeVisible()
  await page.getByRole('button', { name: '参考图.png' }).click()
  await page.getByRole('button', { name: '关闭素材库' }).click()

  await page.getByRole('button', { name: '创意对话' }).click()
  await page.getByRole('textbox', { name: '视频创意描述' }).fill('策划产品片')
  await page.getByRole('button', { name: '发送' }).click()
  await expect(page.getByText('建议采用慢推镜头')).toBeVisible()
  await page.getByRole('button', { name: '使用此提示词' }).click()
  await expect(page.getByRole('textbox', { name: '视频创意描述' })).toHaveValue('产品慢推镜头')

  const compareButton = page.getByRole('button', { name: '版本对比' }).first()
  await expect(compareButton).toBeEnabled()
  await compareButton.click()
  await expect(page.getByRole('heading', { name: '版本对比' })).toBeVisible()
  await page.getByRole('button', { name: '关闭版本对比' }).click()

  const soundtrackButton = page.getByRole('button', { name: '智能配乐' }).first()
  await soundtrackButton.click()
  await expect(page.locator('.video-soundtrack-inline audio')).toBeVisible()
  expect(getVideoGenerationCalls()).toBe(0)
})

test('video conversation workspace and teleported dialogs follow the global theme', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'desktop')
  await page.addInitScript(() => window.localStorage.setItem('image_agent_user_theme:v1', 'light'))
  await login(page)
  await mockVideoWorkspace(page)
  await page.goto('/workspace/video?conversation=91')

  const workspace = page.getByTestId('video-conversation-workspace')
  await expect(workspace).toHaveAttribute('data-theme', 'light')
  await expect(workspace).toHaveCSS('background-color', 'rgb(244, 247, 251)')
  await expect(page.locator('.video-conversation-rail')).toHaveCSS('background-color', 'rgb(255, 255, 255)')

  await page.locator('.video-chat-toolbar').getByRole('button', { name: '素材库' }).click()
  const modal = page.locator('.video-asset-modal')
  await expect(modal).toHaveAttribute('data-theme', 'light')
  await expect(modal.locator(':scope > div')).toHaveCSS('background-color', 'rgb(255, 255, 255)')
  await page.getByRole('button', { name: '关闭素材库' }).click()

  await page.getByTestId('site-theme-toggle').click()
  await expect(workspace).toHaveAttribute('data-theme', 'dark')
  await expect(workspace).toHaveCSS('background-color', 'rgb(8, 10, 14)')
})

test('video conversation workspace keeps tablet content visible across intermediate widths', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'desktop')
  await page.setViewportSize({ width: 1270, height: 1300 })
  await login(page)
  await mockVideoWorkspace(page)
  await page.goto('/workspace/video?conversation=91')
  await expect(page.getByTestId('video-conversation-workspace')).toBeVisible()

  for (const viewport of [
    { width: 2048, height: 1080, rail: 'visible' },
    { width: 1600, height: 900, rail: 'visible' },
    { width: 1440, height: 900, rail: 'visible' },
    { width: 1270, height: 1300, rail: 'visible' },
    { width: 1200, height: 900, rail: 'visible' },
    { width: 1199, height: 900, rail: 'drawer' },
    { width: 1024, height: 768, rail: 'drawer' },
    { width: 900, height: 1024, rail: 'drawer' },
    { width: 390, height: 844, rail: 'drawer' },
  ]) {
    await page.setViewportSize(viewport)
    const layout = await responsiveLayout(page)
    expect(layout.mainOverflow).toBeLessThanOrEqual(0)
    expect(layout.toolbarOverflow).toBeLessThanOrEqual(0)
    expect(layout.composerOverflow).toBeLessThanOrEqual(0)
    expect(layout.composerContained).toBe(true)
    expect(layout.timelineBeforeComposer).toBe(true)
    expect(layout.toolbarButtonWhiteSpace.every((value) => value === 'nowrap')).toBe(true)
    expect(layout.modeButtons.every((button) => button.width >= 64 && button.whiteSpace === 'nowrap' && !button.wraps)).toBe(true)
    expect(layout.cost.width).toBeGreaterThan(0)
    expect(layout.cost.whiteSpace).toBe('nowrap')
    expect(layout.cost.wraps).toBe(false)
    expect(layout.sendSize.width).toBeGreaterThanOrEqual(42)
    expect(layout.sendSize.height).toBeGreaterThanOrEqual(42)
    expect(layout.parameterFields).toHaveLength(4)
    expect(layout.parameterFields.every((field) => field.display !== 'none' && field.width > 0 && field.selectWidth > 0)).toBe(true)
    expect(layout.controlsContained).toBe(true)
    expect(layout.parameterColumns).toBe(viewport.width <= 768 ? 2 : 4)
    if (viewport.rail === 'visible') {
      expect(layout.railTransform).toBe('none')
      expect(layout.conversationButtonDisplay).toBe('none')
    } else {
      expect(layout.railTransform).not.toBe('none')
      expect(layout.conversationButtonDisplay).toBe('flex')
    }
    if (viewport.width === 1024) expect(layout.timelineScrollable).toBe(true)
  }

  await page.setViewportSize({ width: 900, height: 1024 })
  await page.reload()
  await expect(page.locator('.video-unified-composer')).toBeVisible()
  const refreshed = await responsiveLayout(page)
  expect(refreshed.composerContained).toBe(true)
  expect(refreshed.composerOverflow).toBeLessThanOrEqual(0)
})

test('video conversation workspace keeps the composer visible after a large delayed conversation list loads', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'desktop')
  await page.setViewportSize({ width: 2048, height: 1080 })
  await login(page)
  await mockVideoWorkspace(page, { conversationCount: 30, conversationDelayMs: 1200 })
  await page.goto('/workspace/video?conversation=91')

  const composer = page.locator('.video-unified-composer')
  await expect(composer).toBeVisible()
  const initial = await responsiveLayout(page)
  expect(initial.composerContained).toBe(true)

  await expect(page.locator('.video-conversation-row')).toHaveCount(30)
  for (const viewport of [
    { width: 2048, height: 1080 },
    { width: 1600, height: 900 },
    { width: 1270, height: 1300 },
    { width: 1024, height: 768 },
    { width: 900, height: 1024 },
  ]) {
    await page.setViewportSize(viewport)
    await expect(composer).toBeVisible()
    const layout = await responsiveLayout(page)
    expect(layout.composerContained).toBe(true)
    expect(layout.railListScrollable).toBe(true)
    expect(layout.mainHeight).toBeLessThanOrEqual(layout.workspaceHeight)
    expect(layout.railHeight).toBeLessThanOrEqual(layout.workspaceHeight)
  }

  await page.setViewportSize({ width: 2048, height: 1080 })
  await page.reload()
  await expect(page.locator('.video-conversation-row')).toHaveCount(30)
  await expect(composer).toBeVisible()
  expect((await responsiveLayout(page)).composerContained).toBe(true)
})
