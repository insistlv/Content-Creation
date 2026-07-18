import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  listAdminVideoStylePresets: vi.fn(),
  createAdminVideoStylePreset: vi.fn(),
  updateAdminVideoStylePreset: vi.fn(),
  deleteAdminVideoStylePreset: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: apiMocks
}))

import AdminVideoStylePresetsView from '../views/AdminVideoStylePresetsView.vue'

const listPayload = {
  items: [
    {
      id: 1,
      slug: 'cinematic-realism',
      title: 'Cinematic Realism',
      category: 'film',
      description: 'soft lighting and film texture',
      tags: ['popular', 'beginner'],
      preview_url: 'https://oss.example.com/cinematic.png',
      preview_asset_key: 'video-styles/cinematic.png',
      style_prompt: 'cinematic realism with film texture',
      sort_order: 5,
      is_active: true,
      use_count: 12
    },
    {
      id: 2,
      slug: 'disabled-style',
      title: 'Disabled Style',
      category: 'draft',
      description: '',
      tags: [],
      preview_url: '',
      preview_asset_key: '',
      style_prompt: 'draft style',
      sort_order: 10,
      is_active: false,
      use_count: 0
    }
  ],
  total: 2,
  page: 1,
  page_size: 12
}

function mockInitialLoad() {
  apiMocks.listAdminVideoStylePresets.mockResolvedValue(listPayload)
}

describe('AdminVideoStylePresetsView', () => {
  afterEach(() => {
    vi.clearAllMocks()
    vi.restoreAllMocks()
  })

  it('renders video style presets from the admin API', async () => {
    mockInitialLoad()
    const wrapper = mount(AdminVideoStylePresetsView)
    await flushPromises()

    expect(apiMocks.listAdminVideoStylePresets).toHaveBeenCalledWith({ page: 1, page_size: 12 })
    expect(wrapper.get('[data-testid="video-style-preset-row-1"]').text()).toContain('Cinematic Realism')
    expect(wrapper.get('[data-testid="video-style-preset-row-1"]').text()).toContain('popular')
    expect(wrapper.get('[data-testid="video-style-preset-row-1"] img').attributes('src')).toBe('https://oss.example.com/cinematic.png')
    expect(wrapper.get('[data-testid="video-style-preset-row-2"]').text()).toContain('Disabled Style')
  })

  it('creates, edits, deletes, and reloads video style presets', async () => {
    mockInitialLoad()
    apiMocks.createAdminVideoStylePreset.mockResolvedValue({ id: 9 })
    apiMocks.updateAdminVideoStylePreset.mockResolvedValue({ id: 1 })
    apiMocks.deleteAdminVideoStylePreset.mockResolvedValue({ ok: true })
    vi.spyOn(window, 'confirm').mockReturnValue(true)

    const wrapper = mount(AdminVideoStylePresetsView)
    await flushPromises()

    await wrapper.get('[data-testid="new-video-style-preset"]').trigger('click')
    expect(wrapper.get('[data-testid="video-style-preset-modal"]').text()).toContain('新增视频风格')
    await wrapper.get('[data-testid="video-style-preset-title"]').setValue('New Film Style')
    await wrapper.get('[data-testid="video-style-preset-slug"]').setValue('new-film-style')
    await wrapper.get('[data-testid="video-style-preset-category"]').setValue('film')
    await wrapper.get('[data-testid="video-style-preset-tags"]').setValue('popular, beginner')
    await wrapper.get('[data-testid="video-style-preset-preview-url"]').setValue('https://oss.example.com/new-film.png')
    await wrapper.get('[data-testid="video-style-preset-preview-asset-key"]').setValue('video-styles/new-film.png')
    await wrapper.get('[data-testid="video-style-preset-description"]').setValue('new film description')
    await wrapper.get('[data-testid="video-style-preset-style-prompt"]').setValue('new film style prompt')
    await wrapper.get('[data-testid="video-style-preset-sort-order"]').setValue('15')
    await wrapper.get('[data-testid="save-video-style-preset"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createAdminVideoStylePreset).toHaveBeenCalledWith(expect.objectContaining({
      title: 'New Film Style',
      slug: 'new-film-style',
      category: 'film',
      tags: ['popular', 'beginner'],
      preview_url: 'https://oss.example.com/new-film.png',
      preview_asset_key: 'video-styles/new-film.png',
      description: 'new film description',
      style_prompt: 'new film style prompt',
      sort_order: 15,
      is_active: true
    }))

    await wrapper.get('[data-testid="edit-video-style-preset-1"]').trigger('click')
    await wrapper.get('[data-testid="video-style-preset-title"]').setValue('Updated Film Style')
    await wrapper.get('[data-testid="save-video-style-preset"]').trigger('click')
    await flushPromises()

    expect(apiMocks.updateAdminVideoStylePreset).toHaveBeenCalledWith(1, expect.objectContaining({
      title: 'Updated Film Style',
      tags: ['popular', 'beginner'],
      style_prompt: 'cinematic realism with film texture'
    }))

    await wrapper.get('[data-testid="delete-video-style-preset-1"]').trigger('click')
    await flushPromises()

    expect(apiMocks.deleteAdminVideoStylePreset).toHaveBeenCalledWith(1)
  })
})
