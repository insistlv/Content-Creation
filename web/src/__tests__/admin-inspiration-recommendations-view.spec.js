import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  listAdminInspirationRecommendations: vi.fn(),
  createAdminInspirationRecommendation: vi.fn(),
  updateAdminInspirationRecommendation: vi.fn(),
  deleteAdminInspirationRecommendation: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: apiMocks
}))

import AdminInspirationRecommendationsView from '../views/AdminInspirationRecommendationsView.vue'

const listPayload = {
  items: [
    {
      id: 1,
      slug: 'cyberpunk-city',
      title: 'Cyberpunk City',
      category: 'concept',
      description: 'Neon skyline sample',
      heat_tags: ['weekly-hot', 'beginner'],
      preview_url: 'https://oss.example.com/cyber.png',
      prompt: 'cyberpunk city at rainy night',
      negative_prompt: 'low quality',
      aspect_ratio: '16:9',
      style_preset: 'cinematic',
      theme: 'cyber',
      tool_mode: 'generate',
      model_id: 7,
      params: { seed: 918 },
      sort_order: 5,
      is_active: true,
      use_count: 12,
      view_count: 34
    },
    {
      id: 2,
      slug: 'hidden-item',
      title: 'Hidden Item',
      category: 'draft',
      description: '',
      heat_tags: [],
      preview_url: 'https://oss.example.com/hidden.png',
      prompt: 'hidden prompt',
      negative_prompt: '',
      aspect_ratio: '1:1',
      style_preset: '',
      theme: '',
      tool_mode: 'generate',
      model_id: 0,
      params: {},
      sort_order: 10,
      is_active: false,
      use_count: 0,
      view_count: 0
    }
  ],
  total: 2,
  page: 1,
  page_size: 12
}

function mockInitialLoad() {
  apiMocks.listAdminInspirationRecommendations.mockResolvedValue(listPayload)
}

describe('AdminInspirationRecommendationsView', () => {
  afterEach(() => {
    vi.clearAllMocks()
    vi.restoreAllMocks()
  })

  it('renders inspiration recommendations from the admin API', async () => {
    mockInitialLoad()
    const wrapper = mount(AdminInspirationRecommendationsView)
    await flushPromises()

    expect(apiMocks.listAdminInspirationRecommendations).toHaveBeenCalledWith({ page: 1, page_size: 12 })
    expect(wrapper.get('[data-testid="recommendation-row-1"]').text()).toContain('Cyberpunk City')
    expect(wrapper.get('[data-testid="recommendation-row-1"]').text()).toContain('weekly-hot')
    expect(wrapper.get('[data-testid="recommendation-row-1"] img').attributes('src')).toBe('https://oss.example.com/cyber.png')
    expect(wrapper.get('[data-testid="recommendation-row-2"]').text()).toContain('Hidden Item')
  })

  it('creates, edits, deletes, and reloads inspiration recommendations', async () => {
    mockInitialLoad()
    apiMocks.createAdminInspirationRecommendation.mockResolvedValue({ id: 9 })
    apiMocks.updateAdminInspirationRecommendation.mockResolvedValue({ id: 1 })
    apiMocks.deleteAdminInspirationRecommendation.mockResolvedValue({ ok: true })
    vi.spyOn(window, 'confirm').mockReturnValue(true)

    const wrapper = mount(AdminInspirationRecommendationsView)
    await flushPromises()

    await wrapper.get('[data-testid="new-recommendation"]').trigger('click')
    expect(wrapper.get('[data-testid="inspiration-recommendation-modal"]').text()).toContain('新增灵感推荐')
    await wrapper.get('[data-testid="recommendation-title"]').setValue('New Cyber City')
    await wrapper.get('[data-testid="recommendation-slug"]').setValue('new-cyber-city')
    await wrapper.get('[data-testid="recommendation-category"]').setValue('concept')
    await wrapper.get('[data-testid="recommendation-heat-tags"]').setValue('weekly-hot, beginner')
    await wrapper.get('[data-testid="recommendation-preview-url"]').setValue('https://oss.example.com/new-cyber.png')
    await wrapper.get('[data-testid="recommendation-prompt"]').setValue('new cyber prompt')
    await wrapper.get('[data-testid="recommendation-negative-prompt"]').setValue('low quality')
    await wrapper.get('[data-testid="recommendation-aspect-ratio"]').setValue('16:9')
    await wrapper.get('[data-testid="recommendation-style-preset"]').setValue('cinematic')
    await wrapper.get('[data-testid="recommendation-model-id"]').setValue('7')
    await wrapper.get('[data-testid="recommendation-params"]').setValue('{"seed":918}')
    await wrapper.get('[data-testid="recommendation-sort-order"]').setValue('15')
    await wrapper.get('[data-testid="save-recommendation"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createAdminInspirationRecommendation).toHaveBeenCalledWith(expect.objectContaining({
      title: 'New Cyber City',
      slug: 'new-cyber-city',
      heat_tags: ['weekly-hot', 'beginner'],
      prompt: 'new cyber prompt',
      negative_prompt: 'low quality',
      aspect_ratio: '16:9',
      style_preset: 'cinematic',
      model_id: 7,
      params: { seed: 918 },
      sort_order: 15,
      is_active: true
    }))

    await wrapper.get('[data-testid="edit-recommendation-1"]').trigger('click')
    await wrapper.get('[data-testid="recommendation-title"]').setValue('Updated Cyber City')
    await wrapper.get('[data-testid="save-recommendation"]').trigger('click')
    await flushPromises()

    expect(apiMocks.updateAdminInspirationRecommendation).toHaveBeenCalledWith(1, expect.objectContaining({
      title: 'Updated Cyber City',
      heat_tags: ['weekly-hot', 'beginner'],
      params: { seed: 918 }
    }))

    await wrapper.get('[data-testid="delete-recommendation-1"]').trigger('click')
    await flushPromises()

    expect(apiMocks.deleteAdminInspirationRecommendation).toHaveBeenCalledWith(1)
  })
})
