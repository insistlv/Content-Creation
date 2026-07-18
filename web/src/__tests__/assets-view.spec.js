import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const routerPush = vi.hoisted(() => vi.fn())
const apiMocks = vi.hoisted(() => ({
  listReferenceAssets: vi.fn(),
  uploadReferenceAsset: vi.fn(),
  deleteReferenceAsset: vi.fn(),
  updateReferenceAsset: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    listReferenceAssets: apiMocks.listReferenceAssets,
    uploadReferenceAsset: apiMocks.uploadReferenceAsset,
    deleteReferenceAsset: apiMocks.deleteReferenceAsset,
    updateReferenceAsset: apiMocks.updateReferenceAsset
  }
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: routerPush
  })
}))

import AssetsView from '../views/AssetsView.vue'

describe('AssetsView', () => {
  beforeEach(() => {
    Object.values(apiMocks).forEach((mock) => mock.mockReset())
    routerPush.mockReset()
    window.sessionStorage.clear()
    vi.spyOn(window, 'confirm').mockReturnValue(true)
  })

  function mockAssets(items = defaultAssets()) {
    apiMocks.listReferenceAssets.mockResolvedValueOnce({
      items
    })
  }

  function defaultAssets() {
    return [
      {
        id: 41,
        original_filename: 'very-long-product-reference-filename-that-should-not-push-actions-out-of-line.png',
        display_name: '商品参考图',
        preview_url: '/api/reference-assets/41/file',
        mime_type: 'image/png',
        created_at: '2026-06-08T01:00:00Z'
      },
      {
        id: 42,
        original_filename: 'logo-reference.jpg',
        preview_url: '/api/reference-assets/42/file',
        mime_type: 'image/jpeg',
        created_at: '2026-05-01T01:00:00Z'
      },
      {
        id: 43,
        original_filename: 'banner.webp',
        preview_url: '/api/reference-assets/43/file',
        mime_type: 'image/webp',
        created_at: '2026-04-01T01:00:00Z'
      },
      {
        id: 44,
        original_filename: 'transparent.png',
        preview_url: '/api/reference-assets/44/file',
        mime_type: 'image/png',
        created_at: '2026-03-01T01:00:00Z'
      },
      {
        id: 45,
        original_filename: 'fifth.png',
        preview_url: '/api/reference-assets/45/file',
        mime_type: 'image/png',
        created_at: '2026-02-01T01:00:00Z'
      }
    ]
  }

  it('lists, uploads, deletes, and sends one reference asset into the workspace', async () => {
    mockAssets([defaultAssets()[1]])
    apiMocks.uploadReferenceAsset.mockResolvedValueOnce({
      id: 46,
      original_filename: 'new-ref.png',
      preview_url: '/api/reference-assets/43/file'
    })
    apiMocks.deleteReferenceAsset.mockResolvedValueOnce({ ok: true })

    const wrapper = mount(AssetsView)
    await flushPromises()

    expect(wrapper.get('[data-testid="asset-card-42"] img').attributes('src')).toBe('/api/reference-assets/42/file')

    const file = new File(['image'], 'new-ref.png', { type: 'image/png' })
    const input = wrapper.get('[data-testid="asset-upload-input"]')
    Object.defineProperty(input.element, 'files', {
      value: [file],
      configurable: true
    })
    await input.trigger('change')
    await flushPromises()

    expect(apiMocks.uploadReferenceAsset).toHaveBeenCalledWith(file)
    expect(wrapper.find('[data-testid="asset-card-46"]').exists()).toBe(true)

    await wrapper.get('[data-testid="asset-use-46"]').trigger('click')

    expect(window.sessionStorage.getItem('image_agent_workspace_prefill:v1')).toBe(JSON.stringify({
      reference_asset_ids: [46]
    }))
    expect(routerPush).toHaveBeenCalledWith('/workspace')

    await wrapper.get('[data-testid="asset-delete-42"]').trigger('click')
    await flushPromises()

    expect(apiMocks.deleteReferenceAsset).toHaveBeenCalledWith(42)
    expect(wrapper.find('[data-testid="asset-card-42"]').exists()).toBe(false)
  })

  it('shows stable grid metadata with display names, full title, and date-only timestamps', async () => {
    mockAssets()

    const wrapper = mount(AssetsView)
    await flushPromises()

    const card = wrapper.get('[data-testid="asset-card-41"]')
    const name = card.get('[data-testid="asset-name-41"]')
    expect(name.text()).toBe('商品参考图')
    expect(name.attributes('title')).toBe('very-long-product-reference-filename-that-should-not-push-actions-out-of-line.png')
    expect(name.classes()).toContain('asset-name')
    expect(card.get('[data-testid="asset-date-41"]').text()).toMatch(/2026\/6\/8|2026\/06\/08/)
    expect(card.get('[data-testid="asset-date-41"]').text()).not.toContain(':')
    expect(card.find('[data-testid="asset-card-actions-41"]').exists()).toBe(true)
  })

  it('filters assets by image mime type and keeps selected items across grid and list views', async () => {
    mockAssets()

    const wrapper = mount(AssetsView)
    await flushPromises()

    await wrapper.get('[data-testid="asset-select-41"]').setValue(true)
    await wrapper.get('[data-testid="asset-filter-png"]').trigger('click')

    expect(wrapper.find('[data-testid="asset-card-41"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="asset-card-42"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="asset-card-43"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="selected-count"]').text()).toContain('已选 1 项')

    await wrapper.get('[data-testid="asset-view-list"]').trigger('click')

    expect(wrapper.find('[data-testid="asset-list"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="asset-row-41"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="asset-select-41"]').element.checked).toBe(true)

    await wrapper.get('[data-testid="asset-filter-webp"]').trigger('click')
    expect(wrapper.find('[data-testid="asset-row-43"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="asset-row-41"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="selected-count"]').text()).toContain('已选 1 项')
  })

  it('sends 1 to 4 selected assets into the workspace and blocks larger selections', async () => {
    mockAssets()

    const wrapper = mount(AssetsView)
    await flushPromises()

    for (const id of [41, 42, 43, 44]) {
      await wrapper.get(`[data-testid="asset-select-${id}"]`).setValue(true)
    }
    await wrapper.get('[data-testid="asset-bulk-use"]').trigger('click')

    expect(window.sessionStorage.getItem('image_agent_workspace_prefill:v1')).toBe(JSON.stringify({
      reference_asset_ids: [41, 42, 43, 44]
    }))
    expect(routerPush).toHaveBeenCalledWith('/workspace')

    routerPush.mockClear()
    window.sessionStorage.clear()
    await wrapper.get('[data-testid="asset-select-45"]').setValue(true)

    expect(wrapper.get('[data-testid="asset-bulk-use"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="asset-selection-limit"]').text()).toContain('最多送入 4 张参考图')

    await wrapper.get('[data-testid="asset-bulk-use"]').trigger('click')
    expect(routerPush).not.toHaveBeenCalled()
    expect(window.sessionStorage.getItem('image_agent_workspace_prefill:v1')).toBeNull()
  })

  it('bulk deletes selected assets, removes successful deletes, and keeps failed ones selected', async () => {
    mockAssets(defaultAssets().slice(0, 3))
    apiMocks.deleteReferenceAsset
      .mockResolvedValueOnce({ ok: true })
      .mockRejectedValueOnce(new Error('delete failed'))

    const wrapper = mount(AssetsView)
    await flushPromises()

    await wrapper.get('[data-testid="asset-select-41"]').setValue(true)
    await wrapper.get('[data-testid="asset-select-42"]').setValue(true)
    await wrapper.get('[data-testid="asset-bulk-delete"]').trigger('click')
    await flushPromises()

    expect(window.confirm).toHaveBeenCalledWith('删除选中的 2 个素材？')
    expect(apiMocks.deleteReferenceAsset).toHaveBeenCalledWith(41)
    expect(apiMocks.deleteReferenceAsset).toHaveBeenCalledWith(42)
    expect(wrapper.find('[data-testid="asset-card-41"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="asset-card-42"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="asset-select-42"]').element.checked).toBe(true)
    expect(wrapper.get('.status-error').text()).toContain('部分素材删除失败')
  })

  it('renames assets inline and falls back to original filenames after clearing display names', async () => {
    mockAssets([defaultAssets()[0]])
    apiMocks.updateReferenceAsset
      .mockResolvedValueOnce({
        ...defaultAssets()[0],
        display_name: '新的展示名'
      })
      .mockResolvedValueOnce({
        ...defaultAssets()[0],
        display_name: ''
      })

    const wrapper = mount(AssetsView)
    await flushPromises()

    await wrapper.get('[data-testid="asset-rename-41"]').trigger('click')
    await wrapper.get('[data-testid="asset-rename-input-41"]').setValue('  新的展示名  ')
    await wrapper.get('[data-testid="asset-rename-save-41"]').trigger('click')
    await flushPromises()

    expect(apiMocks.updateReferenceAsset).toHaveBeenCalledWith(41, { display_name: '新的展示名' })
    expect(wrapper.get('[data-testid="asset-name-41"]').text()).toBe('新的展示名')

    await wrapper.get('[data-testid="asset-rename-41"]').trigger('click')
    await wrapper.get('[data-testid="asset-rename-input-41"]').setValue('   ')
    await wrapper.get('[data-testid="asset-rename-save-41"]').trigger('click')
    await flushPromises()

    expect(apiMocks.updateReferenceAsset).toHaveBeenLastCalledWith(41, { display_name: '' })
    expect(wrapper.get('[data-testid="asset-name-41"]').text()).toBe(defaultAssets()[0].original_filename)
  })
})
