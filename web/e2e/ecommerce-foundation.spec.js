import { expect, test } from '@playwright/test'

const clientHeaders = { 'X-Image-Agent-Client': 'mp-weixin' }
async function login(page) {
  const response = await page.request.post('/api/auth/login', { headers: clientHeaders, data: { username: 'workspace_e2e', password: 'test-password' } })
  await expectStatus(response, 200, '登录')
}

async function expectStatus(response, expected, label) {
  if (response.status() === expected) return
  const body = await response.text()
  throw new Error(`${label}返回 ${response.status()}，预期 ${expected}：${body}`)
}

test('commerce foundation executes and restores one test-created lifecycle', async ({ page, request }, testInfo) => {
  test.setTimeout(90_000)
  expect((await request.get('/api/ecommerce/projects')).status()).toBe(401)
  await login(page)

  const seed = (await (await page.request.get('/api/ecommerce/projects')).json()).items.find((item) => item.title === 'Foundation E2E 项目')
  const create = await page.request.post('/api/ecommerce/projects', { headers: clientHeaders, data: {
    title: `真实链路 ${test.info().project.name} ${Date.now()}`, product_id: seed.product_id,
    default_sku_id: seed.default_sku_id, pipeline: 'general'
  } })
  await expectStatus(create, 201, '创建项目')
  const project = await create.json()

  const policyResponse = await page.request.post(`/api/ecommerce/projects/${project.id}/assets/upload-policy`, {
    headers: clientHeaders, data: { filename: 'playwright.png', mime_type: 'image/png', size: 68 }
  })
  await expectStatus(policyResponse, 200, '获取上传凭证')
  const policy = await policyResponse.json()
  const uploaded = await page.request.post(policy.upload_url, { multipart: {
    ...policy.form_data,
    file: { name: 'playwright.png', mimeType: 'image/png', buffer: Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=', 'base64') }
  } })
  await expectStatus(uploaded, 201, '上传商品素材')
  const completed = await page.request.post(`/api/ecommerce/projects/${project.id}/assets/complete-upload`, {
    headers: clientHeaders, data: { object_key: policy.object_key, upload_token: policy.upload_token, sku_id: seed.default_sku_id, role: 'product', lifecycle: 'project' }
  })
  await expectStatus(completed, 201, '确认商品素材')

  const draft = await page.request.post(`/api/ecommerce/projects/${project.id}/creative-specs`, {
    headers: clientHeaders,
    data: {
      product_facts: {
        name: `真实链路 ${test.info().project.name}`,
        material: '用户未提供',
        capacity: '用户未提供',
        price: '用户未提供',
        certification: '用户未提供',
        efficacy: '用户未提供',
        source: 'playwright'
      },
      selling_points: ['真实链路']
    }
  })
  await expectStatus(draft, 201, '创建商品报告')
  const spec = await draft.json()
  const confirmed = await page.request.post(`/api/ecommerce/creative-specs/${spec.id}/confirm`, { headers: clientHeaders, data: {} })
  await expectStatus(confirmed, 200, '确认商品报告')

  const batchInput = { recipe_key: 'workspace-e2e-recipe', recipe_version: 1, output_count: 2,
    creative_spec_id: spec.id, primary_sku_id: seed.default_sku_id, quality_tier: 'standard', aspect_ratio: '1:1' }
  const estimate = await page.request.post(`/api/ecommerce/projects/${project.id}/batches/estimate`, { headers: clientHeaders, data: batchInput })
  await expectStatus(estimate, 200, '批次估价')
  const pricing = await estimate.json()
  const submitted = await page.request.post(`/api/ecommerce/projects/${project.id}/batches`, {
    headers: { ...clientHeaders, 'Idempotency-Key': `submit-${Date.now()}` }, data: { ...batchInput, pricing_snapshot_id: pricing.pricing_snapshot_id }
  })
  await expectStatus(submitted, 201, '提交批次')
  const submittedSnapshot = await submitted.json()
  const batchId = submittedSnapshot.batch.id

  await expect.poll(async () => (await (await page.request.get(`/api/ecommerce/batches/${batchId}`)).json()), { timeout: 15_000 }).toMatchObject({
    batch: { status: 'partial_succeeded', succeeded_items: 1, failed_items: 1, settled_credits: 1, released_credits: 1 }
  })

  await page.goto('/workspace')
  await page.goto('/workspace/ai-commerce')
  await page.getByTestId('commerce-project-select').selectOption({ label: project.title })
  if (testInfo.project.name === 'mobile') await page.getByTestId('commerce-mobile-tab-results').click()
  await page.getByTestId('commerce-open-history').click()
  const restored = page.locator('.result-batch').filter({ hasText: `批次 #${batchId}` })
  await expect(restored).toContainText('部分完成')
  await expect(restored).toContainText('失败')

  const snapshot = await (await page.request.get(`/api/ecommerce/batches/${batchId}`)).json()
  const failed = snapshot.items.find((item) => item.status === 'failed')
  const retry = await page.request.post(`/api/ecommerce/items/${failed.id}/retry`, {
    headers: { ...clientHeaders, 'Idempotency-Key': `retry-${Date.now()}` }, data: {}
  })
  await expectStatus(retry, 201, '重试失败任务')
  const child = await retry.json()
  expect(child.batch.parent_batch_id).toBe(batchId)
  await expect.poll(async () => (await (await page.request.get(`/api/ecommerce/batches/${child.batch.id}`)).json()).batch.status, { timeout: 15_000 }).toBe('succeeded')
  const childDone = await (await page.request.get(`/api/ecommerce/batches/${child.batch.id}`)).json()
  expect(childDone.batch.settled_credits).toBe(1)
  expect(childDone.batch.released_credits).toBe(0)
})
