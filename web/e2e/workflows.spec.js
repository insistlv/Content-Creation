import { expect, test } from '@playwright/test'

// 覆盖审计报告 P1-2：workspace 之外的关键业务工作流（作品库 / 视频 / 情侣相册 / 套餐）
// 的页面可用性、登录门禁与核心交互。生成提交类操作仅在有 stub 的图像链路执行，
// 视频等无 stub 链路只验证表单与门禁，不触发真实生成。

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

test.describe('登录门禁', () => {
  test('guest visiting works library opens the workspace auth modal host', async ({ page }) => {
    await page.goto('/works')
    await expect(page).toHaveURL(/\/workspace\?auth=login&redirect=(%2Fworks|\/works)/)
    await expect(page.getByTestId('auth-modal')).toContainText('需要登录才能使用该功能')
  })

  test('guest visiting video workspace opens the workspace auth modal host', async ({ page }) => {
    await page.goto('/workspace/video')
    await expect(page).toHaveURL(/\/workspace\?auth=login&redirect=(%2Fworkspace%2Fvideo|\/workspace\/video)/)
    await expect(page.getByTestId('auth-modal')).toContainText('需要登录才能使用该功能')
  })

  test('guest visiting couple album workspace opens the workspace auth modal host', async ({ page }) => {
    await page.goto('/workspace/couple-album')
    await expect(page).toHaveURL(/\/workspace\?auth=login&redirect=(%2Fworkspace%2Fcouple-album|\/workspace\/couple-album)/)
    await expect(page.getByTestId('auth-modal')).toContainText('需要登录才能使用该功能')
  })
})

test.describe('作品库', () => {
  test('seeded history work is listed and searchable', async ({ page }) => {
    await login(page)
    await page.goto('/works')

    await expect(page.getByTestId('works-filter-bar')).toBeVisible()
    await expect(page.getByTestId('works-search-input')).toBeVisible()
    await expect(page.locator('[data-testid^="works-card-"]').first()).toBeVisible()

    await page.getByTestId('works-search-input').fill('不存在的作品关键字XYZ')
    await page.getByTestId('works-search-input').press('Enter')
    await expect(page.locator('[data-testid^="works-card-"]')).toHaveCount(0)

    await page.getByTestId('works-search-input').fill('')
    await page.getByTestId('works-search-input').press('Enter')
    await expect(page.locator('[data-testid^="works-card-"]').first()).toBeVisible()
  })

  test('work preview modal opens and closes', async ({ page }) => {
    await login(page)
    await page.goto('/works')

    const firstCard = page.locator('[data-testid^="works-card-"]').first()
    await expect(firstCard).toBeVisible()
    await firstCard.hover()
    await page.locator('[data-testid^="works-view-"]').first().click()

    await expect(page.getByTestId('works-preview-modal')).toBeVisible()
    await page.getByTestId('works-preview-close').click()
    await expect(page.getByTestId('works-preview-modal')).toBeHidden()
  })
})

test.describe('视频工作台', () => {
  test('video workspace renders form controls without submitting generation', async ({ page }) => {
    await login(page)
    await page.goto('/workspace/video')

    await expect(page.getByTestId('video-prompt')).toBeVisible()
    await expect(page.getByTestId('video-aspect-ratio')).toBeVisible()
    await expect(page.getByTestId('video-duration')).toBeVisible()
    await expect(page.getByTestId('video-submit')).toBeVisible()
    await expect(page.getByTestId('video-result-panel')).toBeVisible()
  })
})

test.describe('情侣相册工作台', () => {
  test('couple album workspace renders title input and submit entry', async ({ page }) => {
    await login(page)
    await page.goto('/workspace/couple-album')

    await expect(page.getByTestId('couple-album-title')).toBeVisible()
    await expect(page.getByTestId('couple-album-submit')).toBeVisible()
  })

  test('childhood dream variant renders style and theme options', async ({ page }) => {
    await login(page)
    await page.goto('/workspace/childhood-dream-album')

    await expect(page.getByTestId('childhood-dream-workspace')).toBeVisible()
    await expect(page.locator('[data-testid^="childhood-dream-style-"]').first()).toBeVisible()
    await expect(page.locator('[data-testid^="childhood-dream-theme-"]').first()).toBeVisible()
  })
})

test.describe('套餐页', () => {
  test('pricing page lists seeded packages for guests', async ({ page }) => {
    await page.goto('/pricing')
    await expect(page.locator('[data-testid^="pricing-package-card-"]').first()).toBeVisible()
  })
})
