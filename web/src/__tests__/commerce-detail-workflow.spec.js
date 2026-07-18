import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useCommerceDetailWorkflow } from '../composables/useCommerceDetailWorkflow.js'
import CommerceGenerationConfigurator from '../components/ecommerce/CommerceGenerationConfigurator.vue'

const api = vi.hoisted(() => ({
  getCommerceCapabilities: vi.fn().mockResolvedValue({ enabled: true, worker_enabled: true }),
	listCommerceCategories: vi.fn().mockResolvedValue({ version: 'cn-commerce-v1', system_categories: [], custom_categories: [], recent_categories: [] }),
	createCommerceCustomCategory: vi.fn().mockResolvedValue({ id: 9, source: 'user', path: '家居日用 / 咖啡器具' }),
	patchCommerceCustomCategory: vi.fn().mockResolvedValue({ id: 9 }),
  getCommerceProduct: vi.fn().mockResolvedValue({ id: 4, name: '已建档商品', category: '家居' }),
	patchCommerceProduct: vi.fn().mockResolvedValue({ id: 4, name: '已建档商品', category: '家居日用 / 杯壶餐具', category_id: 11, category_source: 'system', category_path: '家居日用 / 杯壶餐具' }),
  getLatestCommerceCreativeSpec: vi.fn().mockRejectedValue(Object.assign(new Error('not found'), { status: 404 })),
  listCommerceProjects: vi.fn().mockResolvedValue({ items: [] }),
  listCommerceSKUs: vi.fn().mockResolvedValue({ items: [] }), getCommerceSKUConfig: vi.fn().mockResolvedValue({ version: 0, dimensions: [], values: [], skus: [] }),
  listCommerceRecipes: vi.fn().mockResolvedValue({ items: [{ key: 'product_detail_set', version: 1, sections: ['hero'], aspect_ratios: ['4:5'], quality_tiers: ['standard'], layout_templates: ['clean'], parameters: { layout_template: 'clean' }, required_assets: [{ role: 'product_front' }], optional_assets: [{ role: 'product_detail' }, { role: 'product_back' }] }] }),
  getCredits: vi.fn().mockResolvedValue({ available_credits: 20 }),
  bootstrapCommerceProject: vi.fn().mockResolvedValue({ project: { id: 3, active_creative_spec_id: null }, sku: { id: 5 }, product: { id: 4 } }),
  listCommerceAssets: vi.fn().mockResolvedValue({ items: [{ id: 9, role: 'product_front' }] }),
  createCommerceAssetUploadPolicy: vi.fn().mockResolvedValue({ object_key: 'object', upload_token: 'token' }),
  uploadCommerceAssetBinary: vi.fn().mockResolvedValue({}),
  completeCommerceAssetUpload: vi.fn().mockResolvedValue({ id: 20, role: 'product_front' }),
  deleteCommerceAsset: vi.fn().mockResolvedValue({}),
  analyzeCommerceCreativeSpec: vi.fn().mockResolvedValue({ creative_spec: { id: 7, status: 'analyzing', version: 1 } }),
  getCommerceCreativeSpec: vi.fn().mockResolvedValue({ id: 7, status: 'draft', version: 1, missing_fields: [] }),
  patchCommerceCreativeSpec: vi.fn(), confirmCommerceCreativeSpec: vi.fn(),
  estimateCommerceBatch: vi.fn().mockResolvedValue({ pricing_snapshot_id: 'price-1', total_items: 1, estimated_credits: 4, pricing_expires_at: '2030-01-01T00:00:00Z' }),
  createCommerceBatch: vi.fn().mockResolvedValue({ id: 12, status: 'queued' }),
  listCommerceBatches: vi.fn().mockResolvedValue({ items: [] }), getCommerceBatch: vi.fn(), listCommerceBatchEvents: vi.fn()
}))
vi.mock('../api/client.js', () => ({ api }))

describe('useCommerceDetailWorkflow', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    api.getCommerceCapabilities.mockResolvedValue({ enabled: true, worker_enabled: true })
    api.getLatestCommerceCreativeSpec.mockReset().mockRejectedValue(Object.assign(new Error('not found'), { status: 404 }))
    api.getCommerceCreativeSpec.mockReset().mockResolvedValue({ id: 7, status: 'draft', version: 1, missing_fields: [] })
  })
  it('无项目时原子 bootstrap，并为重试复用同一个幂等键', async () => {
    const flow = useCommerceDetailWorkflow({ pollDelay: 0 })
    await flow.initialize()
    const input = { title: '保温杯', category: '家居' }
    await flow.ensureProject(input)
    const firstKey = api.bootstrapCommerceProject.mock.calls[0][1]
    expect(firstKey).toMatch(/^commerce-bootstrap-/)
    expect(api.bootstrapCommerceProject).toHaveBeenCalledWith(expect.objectContaining({ title: '保温杯', pipeline: 'general' }), firstKey)
    expect(flow.currentProject.value.id).toBe(3)
  })

  it('确认前禁止估价，确认后仅 Estimate 一次和 Submit 一次', async () => {
    const flow = useCommerceDetailWorkflow({ pollDelay: 0 })
    await flow.initialize(); await flow.ensureProject({ title: '杯子', category: '家居' })
    await expect(flow.estimate()).rejects.toThrow('请先确认商品报告')
    flow.creativeSpec.value = { id: 7, status: 'confirmed', version: 2 }
    flow.assets.value = [{ id: 9, role: 'product_front' }]
    await flow.estimate()
    await flow.submit()
    expect(api.estimateCommerceBatch).toHaveBeenCalledTimes(1)
    expect(api.createCommerceBatch).toHaveBeenCalledTimes(1)
    expect(api.createCommerceBatch.mock.calls[0][2]).toMatch(/^commerce-batch-/)
    expect(flow.mobileTab.value).toBe('results')
  })

  it('提交严格复用估价时冻结的请求而不重新读取异步变化的素材状态', async () => {
    const flow = useCommerceDetailWorkflow({ pollDelay: 0 })
    await flow.initialize(); await flow.ensureProject({ title: '杯子', category: '家居' })
    flow.creativeSpec.value = { id: 7, status: 'confirmed', version: 2 }
    flow.assets.value = [{ id: 9, role: 'product_front' }]
    await flow.estimate()
    const estimatedPayload = JSON.parse(JSON.stringify(api.estimateCommerceBatch.mock.calls.at(-1)[1]))

    flow.assets.value = [{ id: 99, role: 'product_front' }]
    await flow.submit()

    expect(api.createCommerceBatch.mock.calls.at(-1)[1]).toEqual({
      ...estimatedPayload,
      pricing_snapshot_id: 'price-1'
    })
  })

  it('输入改变会清空旧估价，缺少 product_front 时拒绝估价', async () => {
    const flow = useCommerceDetailWorkflow({ pollDelay: 0 })
    await flow.initialize()
    flow.currentProject.value = { id: 3, default_sku_id: 5 }
    flow.creativeSpec.value = { id: 7, status: 'confirmed' }
    flow.estimateResult.value = { pricing_snapshot_id: 'old' }
    flow.assets.value = []
    flow.setAspectRatio('4:5')
    expect(flow.estimateResult.value).toBeNull()
    await expect(flow.estimate()).rejects.toThrow('商品主图')
  })

  it('Vision 503 明确降级为手工报告，不伪造分析结果', async () => {
    const unavailable = Object.assign(new Error('视觉服务不可用'), { status: 503, code: 'request_failed' })
    api.analyzeCommerceCreativeSpec.mockRejectedValueOnce(unavailable)
    api.createManualCommerceCreativeSpec = vi.fn().mockResolvedValue({ id: 8, status: 'draft', version: 1 })
    const flow = useCommerceDetailWorkflow({ pollDelay: 0 })
    flow.currentProject.value = { id: 3, default_sku_id: 5 }
    flow.assets.value = [{ id: 9, role: 'product_front' }]
    await expect(flow.analyze({ title: '杯子' })).rejects.toThrow('视觉服务不可用')
    expect(flow.creativeSpec.value).toBeNull()
    expect(flow.notice.value).toContain('自动识别未配置')
    await flow.createManualReport()
    expect(flow.creativeSpec.value.status).toBe('draft')
  })

  it('PATCH 409 会重读服务端版本并提示合并', async () => {
    const conflict = Object.assign(new Error('版本冲突'), { status: 409, code: 'version_conflict' })
    api.patchCommerceCreativeSpec.mockRejectedValueOnce(conflict)
    api.getCommerceCreativeSpec.mockResolvedValueOnce({ id: 7, status: 'draft', version: 3 })
    const flow = useCommerceDetailWorkflow()
    flow.creativeSpec.value = { id: 7, status: 'draft', version: 2 }
    await expect(flow.saveReport({ material: '陶瓷' })).rejects.toThrow('版本冲突')
    expect(flow.creativeSpec.value.version).toBe(3)
    expect(flow.notice.value).toContain('合并')
  })

  it('离页重进从服务端项目、CreativeSpec 和批次恢复到历史结果', async () => {
    api.listCommerceProjects.mockResolvedValueOnce({ items: [{ id: 21, title: '已建档商品', default_sku_id: 5, active_creative_spec_id: 31 }] })
    api.getCommerceCreativeSpec.mockResolvedValueOnce({ id: 31, status: 'confirmed', version: 4 })
    api.listCommerceBatches.mockResolvedValueOnce({ items: [{ id: 41, status: 'succeeded', total_items: 1 }] })
    api.getCommerceBatch.mockResolvedValueOnce({ batch: { id: 41, status: 'succeeded' }, items: [{ id: 51, status: 'succeeded', work_id: 61 }] })
    const flow = useCommerceDetailWorkflow()
    await flow.initialize()
    expect(flow.currentProject.value.id).toBe(21)
    expect(flow.creativeSpec.value.id).toBe(31)
    expect(flow.batches.batches.value[0].items[0].work_id).toBe(61)
    expect(flow.resultMode.value).toBe('history')
    flow.batches.stop()
  })

  it('恢复项目时按 default_sku_id 选择 SKU，切换和新建不会串值', async () => {
    api.listCommerceProjects.mockResolvedValueOnce({ items: [{ id: 21, product_id: 4, default_sku_id: 6 }] })
    api.listCommerceSKUs.mockResolvedValueOnce({ items: [{ id: 5, code: 'A' }, { id: 6, code: 'B' }] })
    const flow = useCommerceDetailWorkflow(); await flow.initialize()
    expect(flow.currentSKU.value.id).toBe(6)
    expect(flow.skus.value).toHaveLength(2)
    api.listCommerceSKUs.mockResolvedValueOnce({ items: [{ id: 9, code: 'C' }] })
    await flow.selectProject({ id: 22, product_id: 8, default_sku_id: 9 })
    expect(flow.currentSKU.value.id).toBe(9)
    flow.batches.batches.value = [{ id: 41, status: 'failed' }]
    flow.batches.events.value = [{ id: 51, event_type: 'job_failed' }]
    flow.batches.error.value = '旧项目批次加载失败'
    flow.newCreation()
    expect(flow.currentSKU.value).toBeNull()
    expect(flow.skus.value).toEqual([])
    expect(flow.batches.batches.value).toEqual([])
    expect(flow.batches.events.value).toEqual([])
    expect(flow.batches.error.value).toBe('')
    expect(flow.resultMode.value).toBe('cases')
  })

  it('SKU 选择和规格变更统一清除估价并要求已确认报告重新确认', async () => {
    const flow = useCommerceDetailWorkflow(); await flow.initialize()
    flow.creativeSpec.value = { id: 7, status: 'confirmed' }; flow.estimateResult.value = { pricing_snapshot_id: 'old' }
    flow.skus.value = [{ id: 5, status: 'active' }, { id: 6, status: 'active' }]
    flow.setSelectedSKUs([5, 6]); flow.setPrimarySKU(6)
    expect(flow.estimateResult.value).toBeNull()
    expect(flow.notice.value).toContain('重新分析并确认')
    expect(flow.requestPayload()).toMatchObject({ primary_sku_id: 6, selected_sku_ids: [5, 6] })
    expect(() => flow.setSelectedSKUs([])).toThrow('至少选择一个规格')
  })

  it('SKU 上下文变脏后强制拒绝估价和提交，重新分析并确认后才解锁', async () => {
    const flow = useCommerceDetailWorkflow({ pollDelay: 0 }); await flow.initialize()
    flow.currentProject.value = { id: 3, default_sku_id: 5 }; flow.currentProduct.value = { id: 4 }
    flow.skus.value = [{ id: 5, status: 'active' }, { id: 6, status: 'active' }]
    flow.selectedSkuIds.value = [5]; flow.primarySkuId.value = 5
    flow.creativeSpec.value = { id: 7, status: 'confirmed' }; flow.assets.value = [{ id: 9, role: 'product_front' }]
    await flow.estimate(); flow.setSelectedSKUs([5, 6])
    expect(flow.skuContextDirty.value).toBe(true)
    expect(flow.needsReconfirmation.value).toBe(true)
    await expect(flow.estimate()).rejects.toThrow('重新分析并确认')
    await expect(flow.submit()).rejects.toThrow('重新分析并确认')
    await flow.analyze({ title: '杯子' })
    expect(flow.skuContextDirty.value).toBe(false)
    expect(flow.needsReconfirmation.value).toBe(true)
    api.confirmCommerceCreativeSpec.mockResolvedValueOnce({ id: 7, status: 'confirmed' })
    await flow.confirmReport()
    expect(flow.needsReconfirmation.value).toBe(false)
  })

  it('SKU 专属素材上传和删除触发重新确认，公共素材不携带 sku_id', async () => {
    const flow = useCommerceDetailWorkflow(); await flow.initialize()
    flow.currentProject.value = { id: 3 }; flow.creativeSpec.value = { id: 7, status: 'confirmed' }
    await flow.upload(3, new File(['x'], 'a.png'), { role: 'product_front', lifecycle: 'project' })
    expect(api.completeCommerceAssetUpload.mock.calls.at(-1)[1]).not.toHaveProperty('sku_id')
    expect(flow.needsReconfirmation.value).toBe(false)
    await flow.upload(3, new File(['x'], 'b.png'), { role: 'product_front', lifecycle: 'project', sku_id: 5 })
    expect(flow.needsReconfirmation.value).toBe(true)
    flow.needsReconfirmation.value = false; flow.skuContextDirty.value = false
    flow.assets.value = [{ id: 20, sku_id: 5, role: 'product_front' }]
    api.deleteCommerceAsset = vi.fn().mockResolvedValue({})
    await flow.remove(3, 20)
    expect(flow.needsReconfirmation.value).toBe(true)
  })

  it('停用已选规格后清理选择并回退到安全有效主规格', async () => {
    api.patchCommerceSKU = vi.fn().mockResolvedValue({ id: 6, status: 'disabled', code: 'B' })
    const flow = useCommerceDetailWorkflow(); flow.skus.value = [{ id: 5, status: 'active' }, { id: 6, status: 'active' }, { id: 7, status: 'active' }]
    flow.selectedSkuIds.value = [5, 6]; flow.primarySkuId.value = 6; flow.currentSKU.value = flow.skus.value[1]
    await flow.patchSKU({ id: 6, input: { status: 'disabled' } })
    expect(flow.selectedSkuIds.value).toEqual([5])
    expect(flow.primarySkuId.value).toBe(5)
    expect(flow.currentSKU.value.id).toBe(5)
  })

  it('currentSKU 始终代表项目默认 SKU，生成主 SKU 改变或停用都不改写它', async () => {
    api.patchCommerceSKU = vi.fn().mockResolvedValue({ id: 6, status: 'disabled', code: 'B' })
    const flow = useCommerceDetailWorkflow(); flow.currentProject.value = { id: 3, default_sku_id: 5 }
    flow.skus.value = [{ id: 5, status: 'active' }, { id: 6, status: 'active' }, { id: 7, status: 'active' }]
    flow.currentSKU.value = flow.skus.value[0]; flow.selectedSkuIds.value = [5, 6]; flow.primarySkuId.value = 5
    flow.setPrimarySKU(6)
    expect(flow.currentSKU.value.id).toBe(5)
    await flow.patchSKU({ id: 6, input: { status: 'disabled' } })
    expect(flow.currentSKU.value.id).toBe(5)
    expect(flow.primarySkuId.value).toBe(5)
  })

  it('乱序矩阵预览只接受最新请求，编辑失效会丢弃在途响应', async () => {
    const deferred = () => { let resolve; const promise = new Promise(done => { resolve = done }); return { promise, resolve } }
    const a = deferred(), b = deferred(), c = deferred()
    api.previewCommerceSKUMatrix = vi.fn().mockReturnValueOnce(a.promise).mockReturnValueOnce(b.promise).mockReturnValueOnce(c.promise)
    const flow = useCommerceDetailWorkflow(); flow.currentProduct.value = { id: 4 }
    const first = flow.previewSKUMatrix({ expected_version: 1, dimensions: [{ name: 'A', values: [{ name: '1' }] }] })
    const second = flow.previewSKUMatrix({ expected_version: 1, dimensions: [{ name: 'B', values: [{ name: '2' }] }] })
    b.resolve({ add: [{ key: 'B' }], keep: [], disable: [], conflicts: [] }); await second
    a.resolve({ add: [{ key: 'A' }], keep: [], disable: [], conflicts: [] }); await first
    expect(flow.skuPreview.value.add[0].key).toBe('B')
    const third = flow.previewSKUMatrix({ expected_version: 1, dimensions: [{ name: 'C', values: [{ name: '3' }] }] })
    flow.clearSKUPreview()
    c.resolve({ add: [{ key: 'C' }], keep: [], disable: [], conflicts: [] }); await third
    expect(flow.skuPreview.value).toBeNull()
    await expect(flow.applySKUMatrix()).rejects.toThrow('请先预览')
  })

  it('矩阵应用后统一协调选择，优先回退到仍启用的项目默认 SKU', async () => {
    api.previewCommerceSKUMatrix = vi.fn().mockResolvedValue({ add: [], keep: [], disable: [{ key: 'B' }], conflicts: [] })
    api.applyCommerceSKUMatrix = vi.fn().mockResolvedValue({ version: 3, dimensions: [], values: [], skus: [{ id: 5, status: 'active' }, { id: 6, status: 'disabled' }, { id: 7, status: 'active' }] })
    const flow = useCommerceDetailWorkflow(); flow.currentProduct.value = { id: 4 }; flow.currentProject.value = { id: 3, default_sku_id: 5 }
    flow.currentSKU.value = { id: 5, status: 'active' }; flow.skus.value = [{ id: 5, status: 'active' }, { id: 6, status: 'active' }]
    flow.selectedSkuIds.value = [6]; flow.primarySkuId.value = 6
    await flow.previewSKUMatrix({ expected_version: 2, dimensions: [] }); await flow.applySKUMatrix()
    expect(flow.selectedSkuIds.value).toEqual([5])
    expect(flow.primarySkuId.value).toBe(5)
    expect(flow.currentSKU.value.id).toBe(5)
  })

  it('矩阵预览冻结请求，应用忽略后续可变输入且版本冲突刷新配置', async () => {
    const flow = useCommerceDetailWorkflow(); flow.currentProduct.value = { id: 4 }
    const request = { expected_version: 2, dimensions: [{ name: '颜色', values: [{ name: '红' }] }] }
    api.previewCommerceSKUMatrix = vi.fn().mockResolvedValue({ add: [{ key: '红' }], keep: [], disable: [], conflicts: [] })
    api.applyCommerceSKUMatrix = vi.fn().mockResolvedValue({ version: 3, dimensions: [], values: [], skus: [] })
    await flow.previewSKUMatrix(request); request.dimensions[0].values[0].name = '蓝'
    await flow.applySKUMatrix({ expected_version: 99, dimensions: [] })
    expect(api.applyCommerceSKUMatrix.mock.calls.at(-1)[1]).toMatchObject({ expected_version: 2, dimensions: [{ values: [{ name: '红' }] }] })
    const conflict = Object.assign(new Error('冲突'), { status: 409, code: 'sku_version_conflict' })
    api.applyCommerceSKUMatrix.mockRejectedValueOnce(conflict)
    api.getCommerceSKUConfig.mockResolvedValueOnce({ version: 4, dimensions: [], values: [], skus: [] })
    await flow.previewSKUMatrix({ expected_version: 3, dimensions: [] })
    await expect(flow.applySKUMatrix()).rejects.toThrow('冲突')
    expect(flow.skuConfig.value.version).toBe(4)
  })

  it('恢复 analyzing 最新报告并继续轮询到 draft', async () => {
    api.listCommerceProjects.mockResolvedValueOnce({ items: [{ id: 22, title: '分析中', product_id: 4, default_sku_id: 5 }] })
    api.getLatestCommerceCreativeSpec.mockResolvedValueOnce({ id: 32, project_id: 22, status: 'analyzing', version: 1 })
    api.getCommerceCreativeSpec.mockResolvedValueOnce({ id: 32, status: 'draft', version: 1, missing_fields: ['name'] })
    const flow = useCommerceDetailWorkflow({ pollDelay: 0 }); await flow.initialize()
    expect(flow.creativeSpec.value.status).toBe('draft')
    expect(api.getCommerceCreativeSpec).toHaveBeenCalledWith(32)
  })

  it('按真实余额和 Estimate 契约计算低余额并阻止提交', async () => {
    api.getCredits.mockResolvedValueOnce({ available_credits: 2 })
    const flow = useCommerceDetailWorkflow(); await flow.initialize()
    flow.currentProject.value = { id: 3, default_sku_id: 5 }; flow.creativeSpec.value = { id: 7, status: 'confirmed' }; flow.assets.value = [{ id: 9, role: 'product_front' }]
    await flow.estimate()
    expect(flow.credits.value).toBe(2)
    expect(flow.estimateResult.value.enough).toBe(false)
    await expect(flow.submit()).rejects.toThrow('点数余额不足')
  })

  it('估价过期后清空快照，重新估价后的新提交使用新幂等键', async () => {
    const stale = Object.assign(new Error('估价过期'), { code: 'pricing_snapshot_expired' })
    api.createCommerceBatch.mockRejectedValueOnce(stale).mockResolvedValueOnce({ id: 13, status: 'queued' })
    const flow = useCommerceDetailWorkflow(); await flow.initialize()
    flow.currentProject.value = { id: 3, default_sku_id: 5 }; flow.creativeSpec.value = { id: 7, status: 'confirmed' }; flow.assets.value = [{ id: 9, role: 'product_front' }]
    await flow.estimate(); await expect(flow.submit()).rejects.toThrow('估价过期')
    expect(flow.estimateResult.value).toBeNull()
    await flow.estimate(); await flow.submit()
    expect(api.createCommerceBatch.mock.calls[0][2]).not.toBe(api.createCommerceBatch.mock.calls[1][2])
  })

  it('识别服务端真实 pricing_snapshot_stale 并清空估价上下文', async () => {
    const stale = Object.assign(new Error('价格快照已失效'), { code: 'pricing_snapshot_stale', status: 409 })
    api.createCommerceBatch.mockRejectedValueOnce(stale)
    const flow = useCommerceDetailWorkflow(); await flow.initialize()
    flow.currentProject.value = { id: 3, default_sku_id: 5 }; flow.creativeSpec.value = { id: 7, status: 'confirmed' }; flow.assets.value = [{ id: 9, role: 'product_front' }]
    await flow.estimate(); await expect(flow.submit()).rejects.toThrow('价格快照已失效')
    expect(flow.estimateResult.value).toBeNull()
    expect(flow.notice.value).toContain('重新估价')
  })

  it('前端发现估价已超过可见有效期时不发送批次请求', async () => {
    api.estimateCommerceBatch.mockResolvedValueOnce({ pricing_snapshot_id: 'expired-price', total_items: 1, estimated_credits: 4, pricing_expires_at: '2020-01-01T00:00:00Z' })
    const flow = useCommerceDetailWorkflow(); await flow.initialize()
    flow.currentProject.value = { id: 3, default_sku_id: 5 }; flow.creativeSpec.value = { id: 7, status: 'confirmed' }; flow.assets.value = [{ id: 9, role: 'product_front' }]
    await flow.estimate(); await expect(flow.submit()).rejects.toThrow('估价已失效')
    expect(api.createCommerceBatch).not.toHaveBeenCalled()
    expect(flow.estimateResult.value).toBeNull()
  })

  it('普通网络失败重试继续复用同一提交幂等键', async () => {
    api.createCommerceBatch.mockRejectedValueOnce(new Error('网络暂时不可用')).mockResolvedValueOnce({ id: 14, status: 'queued' })
    const flow = useCommerceDetailWorkflow(); await flow.initialize()
    flow.currentProject.value = { id: 3, default_sku_id: 5 }; flow.creativeSpec.value = { id: 7, status: 'confirmed' }; flow.assets.value = [{ id: 9, role: 'product_front' }]
    await flow.estimate(); await expect(flow.submit()).rejects.toThrow('网络暂时不可用'); await flow.submit()
    expect(api.createCommerceBatch.mock.calls[0][2]).toBe(api.createCommerceBatch.mock.calls[1][2])
  })

  it('素材刷新变化后立即使旧估价失效', async () => {
    const flow = useCommerceDetailWorkflow(); await flow.initialize()
    flow.currentProject.value = { id: 3, default_sku_id: 5 }; flow.creativeSpec.value = { id: 7, status: 'confirmed' }; flow.assets.value = [{ id: 9, role: 'product_front' }]
    await flow.estimate()
    api.listCommerceAssets.mockResolvedValueOnce({ items: [{ id: 10, role: 'product_front' }] })
    await flow.refreshAssets(3)
    expect(flow.estimateResult.value).toBeNull()
    await expect(flow.submit()).rejects.toThrow('请先完成点数估价')
  })
  it('PATCH 将文案字段与 user_overrides 按真实接口分开提交', async () => {
    api.patchCommerceCreativeSpec.mockResolvedValueOnce({ id: 7, status: 'draft', version: 2 })
    const flow = useCommerceDetailWorkflow(); flow.capabilities.value = { enabled: true }; flow.creativeSpec.value = { id: 7, status: 'draft', version: 1 }
    await flow.saveReport({ user_overrides: { material: '陶瓷' }, selling_points: ['便携'], forbidden_changes: ['杯盖'], brand_tone: { description: '简约' } })
    expect(api.patchCommerceCreativeSpec).toHaveBeenCalledWith(7, { expected_version: 1, user_overrides: { material: '陶瓷' }, selling_points: ['便携'], forbidden_changes: ['杯盖'], brand_tone: { description: '简约' } })
  })

  it('Estimate 与 Submit 使用 Definition 驱动的同一完整素材绑定', async () => {
    const flow = useCommerceDetailWorkflow(); await flow.initialize()
    flow.currentProject.value = { id: 3, default_sku_id: 5 }; flow.creativeSpec.value = { id: 7, status: 'confirmed' }
    flow.assets.value = [{ id: 9, role: 'product_front' }, { id: 10, role: 'product_detail' }, { id: 11, role: 'product_back' }, { id: 12, role: 'unsupported' }]
    await flow.estimate(); await flow.submit()
    const estimated = api.estimateCommerceBatch.mock.calls.at(-1)[1].asset_bindings
    const submitted = api.createCommerceBatch.mock.calls.at(-1)[1].asset_bindings
    expect(estimated).toEqual({ product_front: [9], product_detail: [10], product_back: [11] })
    expect(submitted).toEqual(estimated)
  })

  it('章节作用域变化使估价失效，重新估价后 Submit 复用同一冻结请求', async () => {
    const flow = useCommerceDetailWorkflow(); await flow.initialize()
    flow.currentProject.value = { id: 3, default_sku_id: 5 }
    flow.creativeSpec.value = { id: 7, status: 'confirmed' }
    flow.assets.value = [{ id: 9, role: 'product_front' }]
    flow.skus.value = [{ id: 5, status: 'active' }, { id: 6, status: 'active' }]
    flow.selectedSkuIds.value = [5, 6]; flow.primarySkuId.value = 6
    flow.setSectionScopes({ detail: 'shared' })
    await flow.estimate()
    const estimated = structuredClone(api.estimateCommerceBatch.mock.calls.at(-1)[1])
    flow.setSectionScopes({ detail: 'sku' })
    expect(flow.estimateResult.value).toBeNull()
    await flow.estimate(); await flow.submit()
    const latestEstimate = api.estimateCommerceBatch.mock.calls.at(-1)[1]
    const submitted = api.createCommerceBatch.mock.calls.at(-1)[1]
    expect(estimated.parameters.section_scopes).toEqual({ detail: 'shared' })
    expect(latestEstimate).toMatchObject({ selected_sku_ids: [5, 6], primary_sku_id: 6, parameters: { section_scopes: { detail: 'sku' } } })
    expect(submitted).toMatchObject(latestEstimate)
  })

  it('标准化 Recipe 时保留服务端提供的三类中文显示选项', async () => {
    api.listCommerceRecipes.mockResolvedValueOnce({ items: [{
      key: 'product_detail_set', version: 1,
      sections: ['hero'], quality_tiers: ['high_fidelity'], layout_templates: ['brand_band'],
      section_options: [{ value: 'hero', label: '首屏主视觉' }],
      quality_options: [{ value: 'high_fidelity', label: '高清' }],
      layout_template_options: [{ value: 'brand_band', label: '品牌色带' }]
    }] })
    const flow = useCommerceDetailWorkflow()
    await flow.initialize()
    expect(flow.definition.value).toMatchObject({
      section_options: [{ value: 'hero', label: '首屏主视觉' }],
      quality_options: [{ value: 'high_fidelity', label: '高清' }],
      layout_template_options: [{ value: 'brand_band', label: '品牌色带' }]
    })
  })

  it('点击中文配置后 Estimate 与 Submit 仍提交英文 value', async () => {
    api.listCommerceRecipes.mockResolvedValueOnce({ items: [{
      key: 'product_detail_set', version: 1,
      sections: ['hero', 'selling_points'], aspect_ratios: ['4:5'],
      quality_tiers: ['standard', 'high_fidelity'], layout_templates: ['clean', 'brand_band'],
      allowed_output_counts: [1, 2], required_assets: [{ role: 'product_front' }],
      section_options: [{ value: 'hero', label: '首屏主视觉' }, { value: 'selling_points', label: '核心卖点' }],
      quality_options: [{ value: 'standard', label: '标准' }, { value: 'high_fidelity', label: '高清' }],
      layout_template_options: [{ value: 'clean', label: '简洁留白' }, { value: 'brand_band', label: '品牌色带' }]
    }] })
    const flow = useCommerceDetailWorkflow()
    await flow.initialize()
    flow.currentProject.value = { id: 3, default_sku_id: 5 }
    flow.creativeSpec.value = { id: 7, status: 'confirmed' }
    flow.assets.value = [{ id: 9, role: 'product_front' }]
    flow.setSections(['hero'])
    const wrapper = mount(CommerceGenerationConfigurator, { props: {
      recipe: flow.recipe.value, definition: flow.definition.value, specConfirmed: true,
      selectedSections: flow.selectedSections.value, aspectRatio: flow.aspectRatio.value,
      qualityTier: flow.qualityTier.value, layoutTemplate: flow.layoutTemplate.value
    } })
    await wrapper.findAll('label').find(label => label.text() === '核心卖点').get('input').trigger('change')
    await wrapper.findAll('label').find(label => label.text() === '高清').get('input').trigger('change')
    await wrapper.findAll('label').find(label => label.text() === '品牌色带').get('input').trigger('change')
    flow.setSections(wrapper.emitted('sections').at(-1)[0])
    flow.setQualityTier(wrapper.emitted('quality').at(-1)[0])
    flow.setLayoutTemplate(wrapper.emitted('layout').at(-1)[0])
    await flow.estimate()
    await flow.submit()
    const estimated = api.estimateCommerceBatch.mock.calls.at(-1)[1]
    const submitted = api.createCommerceBatch.mock.calls.at(-1)[1]
    expect(estimated.parameters.detail_sections).toEqual(['hero', 'selling_points'])
    expect(estimated.quality_tier).toBe('high_fidelity')
    expect(estimated.parameters.layout_template).toBe('brand_band')
    expect(submitted).toMatchObject({ quality_tier: 'high_fidelity', parameters: {
      detail_sections: ['hero', 'selling_points'], layout_template: 'brand_band'
    } })
  })
})
