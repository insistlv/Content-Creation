import { expect, test } from '@playwright/test'
import fs from 'node:fs'
import path from 'node:path'

const clientHeaders = { 'X-Image-Agent-Client': 'mp-weixin' }
const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=', 'base64')
const internalIdentifiers = /\b(?:hero|selling_points|material|detail|usage|specification|closing|standard|high_fidelity|clean|dark_gradient|brand_band)\b/

async function expectNoVisibleInternalIdentifiers(page) {
  await expect(page.locator('body')).not.toContainText(internalIdentifiers)
}

async function login(page) {
  const response = await page.request.post('http://127.0.0.1:8889/api/auth/login', {
    headers: clientHeaders,
    data: { username: 'workspace_e2e', password: 'test-password' }
  })
  expect(response.ok()).toBeTruthy()
}

test('AI 商品详情页走真实上传分析生成恢复重试链路', async ({ page, request }, testInfo) => {
  test.setTimeout(240_000)
  expect((await request.get('http://127.0.0.1:8889/api/ecommerce/projects')).status()).toBe(401)
  await login(page)
  await page.goto('/workspace/ai-commerce')
  await expect(page.getByRole('button', { name: '功能说明' })).toBeVisible()
  await expect(page.locator('.creator-topbar .credits')).toBeVisible()
  await page.getByRole('button', { name: '新建创作' }).click()

  const suffix = `${testInfo.project.name}-${Date.now()}`
  const projectPng = Buffer.concat([png, Buffer.from(suffix)])
  await page.locator('[data-field="title"]').fill(`E2E 保温杯 ${suffix}`)
  await page.getByTestId('category-trigger').click()
  await page.getByTestId('category-search').fill('保温杯')
  await page.locator('.category-results button[data-category-id]').first().click()
  await expect(page.getByTestId('category-trigger')).not.toHaveText('请选择商品品类')

  const uploadInputs = page.locator('.role-uploads input[type="file"]')
  await uploadInputs.nth(0).setInputFiles({ name: `front-${suffix}.png`, mimeType: 'image/png', buffer: projectPng })
  await expect(page.locator('.role-uploads article').nth(0).locator('.preview-image')).toBeVisible()
  await uploadInputs.nth(2).setInputFiles({ name: `细节-${suffix}.png`, mimeType: 'image/png', buffer: projectPng })
  await expect(page.locator('.role-uploads article').nth(2).locator('.preview-image')).toBeVisible()

  // 上传阶段会完成项目 bootstrap 并创建默认 SKU；重载后以服务端最新 sku_version 编辑矩阵。
  await page.reload()
  await expect(page.locator('[data-field="title"]')).toHaveValue(`E2E 保温杯 ${suffix}`)
  const bootstrapProject = (await (await page.request.get('http://127.0.0.1:8889/api/ecommerce/projects')).json()).items.find((item) => item.title === `E2E 保温杯 ${suffix}`)
  expect(bootstrapProject).toBeTruthy()

  await page.getByTestId('sku-mode-multiple').click()
  await page.getByTestId('sku-add-dimension').click()
  await page.getByTestId('sku-dimension-name').fill('颜色')
  await page.getByTestId('sku-dimension-values').fill('红色, 蓝色')
  await page.getByTestId('sku-add-dimension').click()
  await page.getByTestId('sku-dimension-name').nth(1).fill('尺寸')
  await page.getByTestId('sku-dimension-values').nth(1).fill('标准')
  await page.getByTestId('sku-preview').click()
  await expect(page.locator('.sku-preview')).toContainText('新增 2')
  const skuApplyResponsePromise = page.waitForResponse(response => response.request().method() === 'PUT' && response.url().includes('/sku-matrix'))
  await page.getByTestId('sku-apply').click()
  const skuApplyResponse = await skuApplyResponsePromise
  expect(skuApplyResponse.ok(), await skuApplyResponse.text()).toBeTruthy()
  const activeSKURows = page.locator('.sku-list article:has(button:has-text("停用"))')
  await expect(activeSKURows).toHaveCount(2)
  const secondaryDefaultButton = activeSKURows.nth(1).getByRole('button', { name: '设为主规格' })
  if (await secondaryDefaultButton.isEnabled()) {
    await secondaryDefaultButton.scrollIntoViewIfNeeded()
    const buttonBox = await secondaryDefaultButton.boundingBox()
    const viewport = testInfo.project.use.viewport
    expect(buttonBox && buttonBox.x >= 0 && buttonBox.y >= 0 && buttonBox.x + buttonBox.width <= viewport.width && buttonBox.y + buttonBox.height <= viewport.height).toBeTruthy()
    expect(await secondaryDefaultButton.evaluate((button) => {
      const box = button.getBoundingClientRect()
      const hit = document.elementFromPoint(box.left + box.width / 2, box.top + box.height / 2)
      return hit === button || button.contains(hit)
    })).toBeTruthy()
    await secondaryDefaultButton.click()
  }
  const skuIds = await activeSKURows.locator('input[data-testid^="sku-code-"]').evaluateAll(nodes => nodes.map(node => Number(node.dataset.testid.split('-').at(-1))))
  await page.getByTestId('asset-sku-product_detail').selectOption(String(skuIds[1]))
  await uploadInputs.nth(2).setInputFiles({ name: `专属细节-${suffix}.png`, mimeType: 'image/png', buffer: Buffer.concat([projectPng, Buffer.from('exclusive')]) })

  await page.getByRole('button', { name: '生成商品分析报告' }).click()
  await expect(page.getByText('图片可验证事实')).toBeVisible({ timeout: 15_000 })
  await expect(page.getByText('需要补充')).toBeVisible()
  await expect(page.locator('.report-form [data-field="selling_points"]')).toHaveValue('白色主体与深色顶部配件的对比外观清晰\n白色杯身外轮廓简洁利落')
  await expect(page.locator('.report-form [data-field="forbidden_changes"]')).toHaveValue('不得改变白色主体与深色顶部配件的配色关系\n不得改变图片中可见的商品外轮廓')
  await expect(page.locator('.report-form [data-field="brand_tone"]')).toHaveValue('简洁克制的现代家居视觉')
  await page.locator('.report-form [data-field="material"]').fill('304 不锈钢（用户补录）')
  await page.locator('.report-form [data-field="brand_tone"]').fill('克制、现代')
  await page.locator('.report-form [data-field="capacity"]').fill('用户未提供')
  await page.locator('.report-form [data-field="price"]').fill('用户未提供')
  await page.locator('.report-form [data-field="certification"]').fill('用户未提供')
  await page.locator('.report-form [data-field="efficacy"]').fill('用户未提供')
  await page.getByTestId('commerce-report-confirm').click()
  await expect(page.getByTestId('commerce-create-pane').getByText('已确认', { exact: true })).toBeVisible()
  const activeProject = (await (await page.request.get('http://127.0.0.1:8889/api/ecommerce/projects')).json()).items.find((item) => item.title === `E2E 保温杯 ${suffix}`)
  expect(activeProject).toBeTruthy()
  const confirmedSpec = await (await page.request.get(`http://127.0.0.1:8889/api/ecommerce/projects/${activeProject.id}/creative-specs/latest`)).json()
  expect(confirmedSpec).toMatchObject({
    status: 'confirmed',
    selling_points: ['白色主体与深色顶部配件的对比外观清晰', '白色杯身外轮廓简洁利落'],
    forbidden_changes: ['不得改变白色主体与深色顶部配件的配色关系', '不得改变图片中可见的商品外轮廓'],
    brand_tone: { description: '克制、现代' }
  })

  const sections = page.locator('.generation-config .section-scope-row label')
  await expect(sections).toHaveCount(7)
  await expect(page.locator('.generation-config input[name="aspect"]')).toHaveCount(4)
  await expect(page.locator('.generation-config fieldset').filter({ hasText: '质量档' }).locator('input')).toHaveCount(2)
  await expect(page.locator('.generation-config fieldset').filter({ hasText: '排版模板' }).locator('input')).toHaveCount(3)
  for (const label of ['首屏主视觉', '核心卖点', '高清', '品牌色带']) {
    await expect(page.locator('.generation-config').getByText(label, { exact: true })).toBeVisible()
  }
  await expectNoVisibleInternalIdentifiers(page)
  for (let index = 2; index < 7; index += 1) await sections.nth(index).click()
  await expect(page.locator('.generation-config .section-scope-row label.selected')).toHaveCount(2)
  await page.getByTestId('section-scope-detail').locator('select').selectOption('shared')
  await page.getByTestId(`generation-sku-${skuIds[1]}`).check()
  await page.getByTestId('generation-primary-sku').selectOption(String(skuIds[1]))
  // 生成 SKU 集合属于报告事实上下文；以最终双 SKU 选择重新分析并确认后再冻结估价。
  await page.getByRole('button', { name: '生成商品分析报告' }).click()
  await expect(page.getByTestId('commerce-report-confirm')).toBeVisible({ timeout: 15_000 })
  await page.locator('.report-form [data-field="material"]').fill('304 不锈钢（用户补录）')
  await page.locator('.report-form [data-field="brand_tone"]').fill('克制、现代')
  for (const field of ['capacity', 'price', 'certification', 'efficacy', 'name']) {
    await page.locator(`.report-form [data-field="${field}"]`).fill(field === 'name' ? `E2E 保温杯 ${suffix}` : '用户未提供')
  }
  await page.getByTestId('commerce-report-confirm').click()
  await expect(page.getByTestId('commerce-create-pane').getByText('已确认', { exact: true })).toBeVisible()
  await page.getByText('4:5', { exact: true }).click()
  await page.getByText('高清', { exact: true }).click()
  await page.getByText('品牌色带', { exact: true }).click()
  await expect(page.getByRole('radio', { name: '4:5' })).toBeChecked()
  await expect(page.getByRole('radio', { name: '高清' })).toBeChecked()
  await expect(page.getByRole('radio', { name: '品牌色带' })).toBeChecked()
  const estimateRequestPromise = page.waitForRequest((request) => request.method() === 'POST' && request.url().endsWith('/batches/estimate'))
  await page.getByRole('button', { name: '预估点数与时间' }).click()
  const estimateRequest = await estimateRequestPromise
  expect(estimateRequest.postDataJSON()).toMatchObject({ selected_sku_ids: skuIds, primary_sku_id: skuIds[1], aspect_ratio: '4:5', quality_tier: 'high_fidelity', parameters: { layout_template: 'brand_band', section_scopes: { detail: 'shared' } } })
  await expect(page.getByText('公共任务 1', { exact: true })).toBeVisible()
  await expect(page.getByText('规格任务 2', { exact: true })).toBeVisible()
  await expect(page.getByText('总图片 3', { exact: true })).toBeVisible()
  await expect(page.getByText(/预计用时/)).toBeVisible()
  const submitRequestPromise = page.waitForRequest((request) => request.method() === 'POST' && /\/api\/ecommerce\/projects\/\d+\/batches$/.test(new URL(request.url()).pathname))
  await page.getByRole('button', { name: '确认并开始生成' }).click()
  const submitRequest = await submitRequestPromise
  expect(submitRequest.postDataJSON()).toMatchObject({ selected_sku_ids: skuIds, primary_sku_id: skuIds[1], aspect_ratio: '4:5', quality_tier: 'high_fidelity', parameters: { layout_template: 'brand_band', section_scopes: { detail: 'shared' } } })

  if (testInfo.project.name === 'mobile') {
    await expect(page.getByTestId('commerce-mobile-tab-results')).toHaveAttribute('aria-selected', 'true')
  }
  await page.getByTestId('commerce-open-history').click()
  await expect(page.locator('.result-batch').first()).toContainText('部分完成', { timeout: 20_000 })
  await expect(page.locator('.result-batch').first()).toContainText('公共内容')
  await expect(page.locator('.result-batch').first().locator('.result-group')).toHaveCount(3)
  await expect(page.locator('.result-batch').first()).toContainText('预计剩余时间')
  await expect(page.locator('.result-batch').first()).toContainText('已结算 2 点 · 已释放 1 点')
  await expect(page.locator('.result-batch').first()).toContainText('已结束')
  await expectNoVisibleInternalIdentifiers(page)
  const originalBatchText = await page.locator('.result-batch').first().locator('header small').textContent()
  const originalBatchId = Number(originalBatchText.match(/#(\d+)/)?.[1])
  expect(originalBatchId).toBeGreaterThan(0)
  const persistedOriginal = await (await page.request.get(`http://127.0.0.1:8889/api/ecommerce/batches/${originalBatchId}`)).json()
  expect(persistedOriginal.batch).toMatchObject({ quality_tier: 'high_fidelity' })
  expect(persistedOriginal.items.some((item) => item.output_snapshot?.output_size === '1024x1280')).toBeTruthy()
  expect(persistedOriginal.items.every((item) => item.scope && item.section && Number.isInteger(item.progress_percent))).toBeTruthy()
  const frozenSKUItem = persistedOriginal.items.find((item) => item.scope === 'sku')
  expect(frozenSKUItem?.sku_snapshot?.code).toBeTruthy()
  const frozenSKUCode = frozenSKUItem.sku_snapshot.code

  const cancelEstimate = await page.request.post(`http://127.0.0.1:8889/api/ecommerce/projects/${activeProject.id}/batches/estimate`, { headers: clientHeaders, data: submitRequest.postDataJSON() })
  expect(cancelEstimate.ok()).toBeTruthy()
  const cancelEstimatePayload = await cancelEstimate.json()
  const cancelSubmit = await page.request.post(`http://127.0.0.1:8889/api/ecommerce/projects/${activeProject.id}/batches`, {
    headers: { ...clientHeaders, 'Idempotency-Key': `e2e-cancel-${suffix}` },
    data: { ...submitRequest.postDataJSON(), pricing_snapshot_id: cancelEstimatePayload.pricing_snapshot_id }
  })
  expect(cancelSubmit.status()).toBe(201)
  const cancelBatchId = (await cancelSubmit.json()).batch.id
  const cancelResponse = await page.request.post(`http://127.0.0.1:8889/api/ecommerce/batches/${cancelBatchId}/cancel`, { headers: clientHeaders, data: {} })
  expect(cancelResponse.ok()).toBeTruthy()
  await expect.poll(async () => (await (await page.request.get(`http://127.0.0.1:8889/api/ecommerce/batches/${cancelBatchId}`)).json()).batch.status, { timeout: 10_000 }).toBe('canceled')

  const renamedSKUCode = `RENAMED-${Date.now()}`
  const renameResponse = await page.request.patch(`http://127.0.0.1:8889/api/ecommerce/skus/${frozenSKUItem.sku_id}`, { headers: clientHeaders, data: { code: renamedSKUCode } })
  expect(renameResponse.ok()).toBeTruthy()
  expect((await renameResponse.json()).code).toBe(renamedSKUCode)

  await page.goto('/workspace')
  await page.goto('/workspace/ai-commerce')
  await page.getByTestId('commerce-project-select').selectOption({ label: `E2E 保温杯 ${suffix}` })
  if (testInfo.project.name === 'mobile') await page.getByTestId('commerce-mobile-tab-results').click()
  await page.getByRole('button', { name: '历史记录' }).click()
  await expect(page.locator('.result-batch').filter({ hasText: originalBatchText })).toContainText('部分完成')
  await expect(page.locator('.result-batch').filter({ hasText: originalBatchText })).toContainText(frozenSKUCode)
  await expect(page.locator('.result-batch').filter({ hasText: originalBatchText })).not.toContainText(renamedSKUCode)
  await expect(page.locator('.result-batch').filter({ hasText: `批次 #${cancelBatchId}` })).toContainText('已取消')

  const original = page.locator('.result-batch').filter({ hasText: originalBatchText })
  await original.getByRole('button', { name: '重试' }).click()
  let childBatchId = 0
  await expect.poll(async () => {
    const response = await page.request.get('http://127.0.0.1:8889/api/ecommerce/projects')
    const projects = (await response.json()).items
    const project = projects.find((item) => item.title === `E2E 保温杯 ${suffix}`)
    const batchesResponse = await page.request.get(`http://127.0.0.1:8889/api/ecommerce/projects/${project.id}/batches`)
    const child = (await batchesResponse.json()).items.find((item) => item.parent_batch_id === originalBatchId)
    childBatchId = child?.id || 0
    return child ? { parent_batch_id: child.parent_batch_id } : null
  }, { timeout: 10_000 }).toEqual({ parent_batch_id: originalBatchId })
  await expect.poll(async () => {
    const response = await page.request.get(`http://127.0.0.1:8889/api/ecommerce/batches/${childBatchId}`)
    const snapshot = await response.json()
    return { status: snapshot.batch.status, settled: snapshot.batch.settled_credits, released: snapshot.batch.released_credits }
  }, { timeout: 20_000 }).toEqual({ status: 'succeeded', settled: 1, released: 0 })
  const isolationProjects = (await (await page.request.get('http://127.0.0.1:8889/api/ecommerce/projects')).json()).items.filter((item) => item.title === `E2E 保温杯 ${suffix}`)
  expect(isolationProjects).toHaveLength(1)
  for (const field of ['id', 'product_id', 'default_sku_id', 'active_creative_spec_id']) {
    const values = isolationProjects.map((item) => item[field])
    expect(values.every(Boolean), `${field} 应全部存在`).toBeTruthy()
    expect(new Set(values).size, `${field} 必须跨三端隔离`).toBe(values.length)
  }
  const primaryBatchIds = await Promise.all(isolationProjects.map(async (project) => {
    const response = await page.request.get(`http://127.0.0.1:8889/api/ecommerce/projects/${project.id}/batches`)
    return (await response.json()).items.find((batch) => !batch.parent_batch_id && batch.recipe_key === 'product_detail_set')?.id
  }))
  expect(primaryBatchIds.every(Boolean), '当前项目必须有独立主批次').toBeTruthy()
  expect(new Set(primaryBatchIds).size).toBe(primaryBatchIds.length)
  const child = page.locator('.result-batch').filter({ hasText: `批次 #${childBatchId}` })
  await expect(child.locator('header b')).toHaveText('已完成', { timeout: 10_000 })
  const preview = child.getByRole('link', { name: '预览' })
  await expect(preview).toBeVisible()
  const download = child.getByRole('link', { name: '下载' })
  await expect(download).toBeVisible()
  const previewResponse = await page.request.get(await preview.getAttribute('href'))
  expect(previewResponse.ok()).toBeTruthy()
  expect(previewResponse.headers()['content-type']).toContain('image/png')
  const downloadResponse = await page.request.get(await download.getAttribute('href'))
  expect(downloadResponse.ok()).toBeTruthy()
  expect(downloadResponse.headers()['content-type']).toContain('image/png')
  const anonymousDownload = await request.get(`http://127.0.0.1:8889${await download.getAttribute('href')}`)
  expect(anonymousDownload.status()).toBe(401)
  const anonymousPreview = await request.get(`http://127.0.0.1:8889${await preview.getAttribute('href')}`)
  expect(anonymousPreview.status()).toBe(401)

  const bodyWidth = await page.evaluate(() => ({ scroll: document.documentElement.scrollWidth, client: document.documentElement.clientWidth }))
  expect(bodyWidth.scroll).toBeLessThanOrEqual(bodyWidth.client)
  if (testInfo.project.name === 'mobile') {
    await page.getByTestId('commerce-history-dialog').getByRole('button', { name: '关闭历史记录' }).click()
    await page.getByTestId('commerce-mobile-tab-create').click()
    await expect(page.getByTestId('commerce-create-pane')).toBeVisible()
    await page.getByTestId('commerce-mobile-tab-results').click()
    await expect(page.getByTestId('commerce-result-pane')).toBeVisible()
    await page.getByTestId('commerce-open-history').click()
  }

  await preview.focus()
  await page.keyboard.press('Tab')
  await expect(download).toBeFocused()
  const focusRing = await download.evaluate((node) => {
    const style = getComputedStyle(node)
    return { style: style.outlineStyle, width: Number.parseFloat(style.outlineWidth) }
  })
  expect(focusRing.style).not.toBe('none')
  expect(focusRing.width).toBeGreaterThanOrEqual(2)

  const artifactDir = path.resolve('..', '.superpowers', 'sdd', 'artifacts', 'task4')
  fs.mkdirSync(artifactDir, { recursive: true })
  const screenshotPath = path.join(artifactDir, `ai-commerce-${testInfo.project.name}.png`)
  await page.screenshot({ path: screenshotPath, fullPage: true })
  await testInfo.attach(`AI 电商 ${testInfo.project.name}`, { path: screenshotPath, contentType: 'image/png' })

  const viewport = testInfo.project.use.viewport
  await page.setViewportSize({ width: Math.max(180, Math.floor(viewport.width / 2)), height: Math.max(400, Math.floor(viewport.height / 2)) })
  const zoomedWidth = await page.evaluate(() => ({ scroll: document.documentElement.scrollWidth, client: document.documentElement.clientWidth }))
  expect(zoomedWidth.scroll).toBeLessThanOrEqual(zoomedWidth.client)
  await expect(page.getByRole('button', { name: '功能说明' })).toBeVisible()
  await expect(page.locator('.creator-topbar .credits')).toBeVisible()
})
