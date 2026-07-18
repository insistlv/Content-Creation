import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  listAdminCoupleAlbumOptions: vi.fn(),
  createAdminCoupleAlbumOption: vi.fn(),
  updateAdminCoupleAlbumOption: vi.fn(),
  deleteAdminCoupleAlbumOption: vi.fn(),
  uploadCoupleAlbumOptionAsset: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: apiMocks
}))

import AdminCoupleAlbumOptionsView from '../views/AdminCoupleAlbumOptionsView.vue'

const listPayload = {
  items: [
    {
      id: 1,
      type: 'location',
      value: '大理',
      label: '大理洱海',
      description: '风吹洱海的蓝色午后',
      image_url: '/static/couple-album/dali-erhai.png',
      icon_url: '',
      prompt_label: '大理洱海',
      sort_order: 10,
      is_active: true
    },
    {
      id: 2,
      type: 'story_template',
      value: 'city_walk',
      label: '城市漫游',
      description: '街角、咖啡和夜色',
      image_url: '',
      icon_url: '/static/icons/works.png',
      prompt_label: '城市漫游',
      sort_order: 10,
      is_active: true
    },
    {
      id: 3,
      type: 'style',
      value: 'film',
      label: '旅行胶片',
      description: '',
      image_url: '',
      icon_url: '/static/icons/photo.png',
      prompt_label: '旅行胶片',
      sort_order: 10,
      is_active: false
    }
  ],
  total: 3,
  page: 1,
  page_size: 100
}

function mockInitialLoad() {
  apiMocks.listAdminCoupleAlbumOptions.mockResolvedValue(listPayload)
}

describe('AdminCoupleAlbumOptionsView', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('renders grouped couple album options and switches tabs', async () => {
    mockInitialLoad()
    const wrapper = mount(AdminCoupleAlbumOptionsView)
    await flushPromises()

    expect(apiMocks.listAdminCoupleAlbumOptions).toHaveBeenCalledWith({ page: 1, page_size: 100 })
    expect(wrapper.text()).toContain('情侣相册配置')
    expect(wrapper.text()).toContain('大理洱海')
    expect(wrapper.text()).toContain('启用选项 2')
    expect(wrapper.text()).toContain('停用 1')

    await wrapper.get('[data-testid="album-options-tab-story_template"]').trigger('click')
    expect(wrapper.text()).toContain('城市漫游')
    expect(wrapper.text()).not.toContain('大理洱海')

    await wrapper.get('[data-testid="album-options-tab-style"]').trigger('click')
    expect(wrapper.text()).toContain('旅行胶片')
    expect(wrapper.text()).toContain('停用')
  })

  it('creates and updates album options from the editor drawer', async () => {
    mockInitialLoad()
    apiMocks.createAdminCoupleAlbumOption.mockResolvedValue({ id: 9 })
    apiMocks.updateAdminCoupleAlbumOption.mockResolvedValue({ id: 1 })

    const wrapper = mount(AdminCoupleAlbumOptionsView)
    await flushPromises()

    await wrapper.get('[data-testid="new-album-option"]').trigger('click')
    await wrapper.get('[data-testid="album-option-value"]').setValue('杭州')
    await wrapper.get('[data-testid="album-option-label"]').setValue('杭州西湖')
    await wrapper.get('[data-testid="album-option-description"]').setValue('西湖边的电影感午后')
    await wrapper.get('[data-testid="album-option-image-url"]').setValue('/static/couple-album/hangzhou-westlake.png')
    await wrapper.get('[data-testid="album-option-prompt-label"]').setValue('杭州西湖')
    await wrapper.get('[data-testid="album-option-save"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createAdminCoupleAlbumOption).toHaveBeenCalledWith(expect.objectContaining({
      type: 'location',
      value: '杭州',
      label: '杭州西湖',
      description: '西湖边的电影感午后',
      image_url: '/static/couple-album/hangzhou-westlake.png',
      prompt_label: '杭州西湖'
    }))

    await wrapper.get('[data-testid="edit-album-option-1"]').trigger('click')
    await wrapper.get('[data-testid="album-option-label"]').setValue('大理洱海旅拍')
    await wrapper.get('[data-testid="album-option-save"]').trigger('click')
    await flushPromises()

    expect(apiMocks.updateAdminCoupleAlbumOption).toHaveBeenCalledWith(1, expect.objectContaining({
      type: 'location',
      value: '大理',
      label: '大理洱海旅拍'
    }))
  })

  it('deletes an album option after confirmation', async () => {
    mockInitialLoad()
    apiMocks.deleteAdminCoupleAlbumOption.mockResolvedValue({ ok: true })
    vi.spyOn(window, 'confirm').mockReturnValue(true)

    const wrapper = mount(AdminCoupleAlbumOptionsView)
    await flushPromises()

    await wrapper.get('[data-testid="delete-album-option-1"]').trigger('click')
    await flushPromises()

    expect(apiMocks.deleteAdminCoupleAlbumOption).toHaveBeenCalledWith(1)
  })
})
