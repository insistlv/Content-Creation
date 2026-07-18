import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api/client.js'

const ok = () => ({ ok: true, status: 200, headers: new Headers(), json: vi.fn().mockResolvedValue({ items: [] }) })

describe('commerce api client contract', () => {
  let fetchMock
  beforeEach(() => {
    document.cookie = 'csrf_token=commerce-csrf; path=/'
    fetchMock = vi.fn().mockImplementation(async () => ok())
    vi.stubGlobal('fetch', fetchMock)
  })
  afterEach(() => { vi.unstubAllGlobals(); document.cookie = 'csrf_token=; Max-Age=0; path=/' })

  it('maps every Task 7 method to the real Gin /api/ecommerce contract', async () => {
    await api.getCommerceCapabilities()
    await api.listCommerceProjects()
    await api.createCommerceProject({ product_id: 2 })
    await api.getCommerceProject(3)
    await api.deleteCommerceProject(3)
    await api.listCommerceAssets(3)
    await api.createCommerceAssetUploadPolicy(3, { filename: 'a.webp', mime_type: 'image/webp', size: 4 })
    await api.completeCommerceAssetUpload(3, { object_key: 'k', upload_token: 't', role: 'product_front', lifecycle: 'project' })
    await api.deleteCommerceAsset(3, 8)
    await api.listCommerceRecipes({ pipeline: 'general' })
    await api.estimateCommerceBatch(3, { recipe_key: 'r' })
    await api.createCommerceBatch(3, { recipe_key: 'r' }, 'batch-key')
    await api.getCommerceBatch(9)
    await api.listCommerceBatches(3)
    await api.listCommerceBatchEvents(9, { after_id: 12 })
    await api.cancelCommerceBatch(9)
    await api.cancelCommerceItem(10)
    await api.retryCommerceItem(10, 'retry-key')

    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual([
      '/api/ecommerce/capabilities', '/api/ecommerce/projects', '/api/ecommerce/projects',
      '/api/ecommerce/projects/3', '/api/ecommerce/projects/3', '/api/ecommerce/projects/3/assets',
      '/api/ecommerce/projects/3/assets/upload-policy', '/api/ecommerce/projects/3/assets/complete-upload',
      '/api/ecommerce/projects/3/assets/8', '/api/ecommerce/recipes?pipeline=general',
      '/api/ecommerce/projects/3/batches/estimate', '/api/ecommerce/projects/3/batches',
      '/api/ecommerce/batches/9', '/api/ecommerce/projects/3/batches',
      '/api/ecommerce/batches/9/events?after_id=12', '/api/ecommerce/batches/9/cancel',
      '/api/ecommerce/items/10/cancel', '/api/ecommerce/items/10/retry'
    ])
    expect(fetchMock.mock.calls[11][1].headers['Idempotency-Key']).toBe('batch-key')
    expect(fetchMock.mock.calls[17][1].headers['Idempotency-Key']).toBe('retry-key')
  })

  it('rejects retry without an idempotency key before sending a request', async () => {
    await expect(api.retryCommerceItem(10)).rejects.toThrow('Idempotency-Key')
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('接入 SKU 配置、矩阵和默认 SKU 接口', async () => {
    await api.listCommerceSKUs(2)
    await api.createCommerceSKU(2, { code: 'RED-S' })
    await api.patchCommerceSKU(8, { code: 'RED-M' })
    await api.getCommerceSKUConfig(2)
    await api.previewCommerceSKUMatrix(2, { expected_version: 1, dimensions: [] })
    await api.applyCommerceSKUMatrix(2, { expected_version: 1, dimensions: [] }, 'matrix-key')
    await api.patchCommerceProject(3, { default_sku_id: 8 })

    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual([
      '/api/ecommerce/products/2/skus', '/api/ecommerce/products/2/skus', '/api/ecommerce/skus/8',
      '/api/ecommerce/products/2/sku-config', '/api/ecommerce/products/2/sku-matrix/preview',
      '/api/ecommerce/products/2/sku-matrix', '/api/ecommerce/projects/3'
    ])
    expect(fetchMock.mock.calls[5][1].headers['Idempotency-Key']).toBe('matrix-key')
  })

  it('maps atomic bootstrap and creative-spec lifecycle with idempotency/version payloads', async () => {
    await api.bootstrapCommerceProject({ title: '杯子', pipeline: 'general' }, 'bootstrap-key')
    await api.analyzeCommerceCreativeSpec(3, { source_asset_ids: [8] }, 'analysis-key')
    await api.getCommerceCreativeSpec(7)
    await api.patchCommerceCreativeSpec(7, { expected_version: 2, user_overrides: { material: '陶瓷' } })
    await api.confirmCommerceCreativeSpec(7)
    await api.getLatestCommerceCreativeSpec(3)
    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual([
      '/api/ecommerce/projects/bootstrap', '/api/ecommerce/projects/3/creative-specs/analyze',
      '/api/ecommerce/creative-specs/7', '/api/ecommerce/creative-specs/7', '/api/ecommerce/creative-specs/7/confirm',
      '/api/ecommerce/projects/3/creative-specs/latest'
    ])
    expect(fetchMock.mock.calls[0][1].headers['Idempotency-Key']).toBe('bootstrap-key')
    expect(fetchMock.mock.calls[1][1].headers['Idempotency-Key']).toBe('analysis-key')
    expect(JSON.parse(fetchMock.mock.calls[3][1].body).expected_version).toBe(2)
  })
})
