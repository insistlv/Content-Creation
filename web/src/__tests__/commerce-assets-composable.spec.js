import { describe, expect, it, vi } from 'vitest'
import { useCommerceAssets } from '../composables/useCommerceAssets.js'

const api = vi.hoisted(() => ({
  createCommerceAssetUploadPolicy: vi.fn().mockResolvedValue({ object_key: 'object', upload_token: 'token' }),
  uploadCommerceAssetBinary: vi.fn().mockResolvedValue(undefined),
  completeCommerceAssetUpload: vi.fn().mockResolvedValue({}),
  listCommerceAssets: vi.fn().mockResolvedValue({ items: [] }),
  deleteCommerceAsset: vi.fn()
}))
vi.mock('../api/client.js', () => ({ api }))

describe('useCommerceAssets', () => {
  it('keeps policy DTO minimal and completes with role lifecycle and optional ordering fields', async () => {
    const file = new File(['x'], 'front.webp', { type: 'image/webp' })
    await useCommerceAssets().upload(7, file, { role: 'garment_front', lifecycle: 'temporary', sku_id: 12, sort_order: 3 })
    expect(api.createCommerceAssetUploadPolicy).toHaveBeenCalledWith(7, { filename: 'front.webp', mime_type: 'image/webp', size: 1 })
    expect(api.completeCommerceAssetUpload).toHaveBeenCalledWith(7, {
      object_key: 'object', upload_token: 'token', role: 'garment_front', lifecycle: 'temporary', sku_id: 12, sort_order: 3
    })
  })

  it('does not let an older project response overwrite the latest project assets or loading state', async () => {
    let resolveA
    api.listCommerceAssets
      .mockImplementationOnce(() => new Promise((resolve) => { resolveA = resolve }))
      .mockResolvedValueOnce({ items: [{ id: 2, role: 'product_front' }] })
    const commerce = useCommerceAssets()
    const old = commerce.refresh(1)
    await commerce.refresh(2)
    expect(commerce.assets.value).toEqual([{ id: 2, role: 'product_front' }])
    expect(commerce.loading.value).toBe(false)
    resolveA({ items: [{ id: 1, role: 'stale' }] })
    await old
    expect(commerce.assets.value).toEqual([{ id: 2, role: 'product_front' }])
    expect(commerce.loading.value).toBe(false)
    expect(commerce.error.value).toBe('')
  })
})
