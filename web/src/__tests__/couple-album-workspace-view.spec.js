import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const routerPush = vi.hoisted(() => vi.fn())
const routePath = vi.hoisted(() => ({ value: '/workspace/couple-album' }))
const apiMocks = vi.hoisted(() => ({
  getCoupleAlbumOptions: vi.fn(),
  listCoupleAlbums: vi.fn(),
  listReferenceAssets: vi.fn(),
  uploadReferenceAsset: vi.fn(),
  deleteReferenceAsset: vi.fn(),
  estimateCoupleAlbum: vi.fn(),
  createCoupleAlbum: vi.fn(),
  generateCoupleAlbum: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    getCoupleAlbumOptions: apiMocks.getCoupleAlbumOptions,
    listCoupleAlbums: apiMocks.listCoupleAlbums,
    listReferenceAssets: apiMocks.listReferenceAssets,
    uploadReferenceAsset: apiMocks.uploadReferenceAsset,
    deleteReferenceAsset: apiMocks.deleteReferenceAsset,
    estimateCoupleAlbum: apiMocks.estimateCoupleAlbum,
    createCoupleAlbum: apiMocks.createCoupleAlbum,
    generateCoupleAlbum: apiMocks.generateCoupleAlbum
  }
}))

vi.mock('vue-router', () => ({
  RouterLink: {
    props: ['to'],
    template: '<a :href="typeof to === \'string\' ? to : to.path"><slot /></a>'
  },
  useRoute: () => routePath.value,
  useRouter: () => ({
    push: routerPush
  })
}))

import ImageUploadZone from '../components/ImageUploadZone.vue'
import CoupleAlbumWorkspaceView from '../views/CoupleAlbumWorkspaceView.vue'

function mountWorkspace() {
  return mount(CoupleAlbumWorkspaceView, {
    global: {
      stubs: {
        RouterLink: {
          props: ['to'],
          template: '<a :href="typeof to === \'string\' ? to : to.path"><slot /></a>'
        }
      }
    }
  })
}

const optionsPayload = {
  locations: [
    { value: '杭州', label: '杭州西湖', description: '湖面与晚风', image_url: '/static/couple-album/hangzhou.png' }
  ],
  story_templates: [
    { value: 'anniversary', label: '纪念日', description: '重要日子的慢镜头' }
  ],
  styles: [
    { value: 'cinematic', label: '电影旅拍' }
  ]
}

const childhoodOptionsPayload = {
  locations: [
    { value: 'childhood_dream_stage', label: '童年梦想舞台', description: '职业梦想主舞台' },
    { value: 'childhood_space_adventure', label: '星际探索之旅', description: '火箭和星球的冒险' },
    { value: 'childhood_fairy_tale', label: '童话奇遇记', description: '城堡与魔法森林' },
    { value: 'childhood_nature_explorer', label: '自然小达人', description: '森林和昆虫观察' },
    { value: '杭州', label: '杭州西湖', description: '湖面与晚风' }
  ],
  story_templates: [
    { value: 'childhood_career_dream', label: '童年职业梦想', description: '8 页职业梦想故事' },
    { value: 'anniversary', label: '纪念日', description: '重要日子的慢镜头' }
  ],
  styles: [
    { value: 'children_storybook', label: '童话绘本' },
    { value: 'dreamy_watercolor', label: '梦幻水彩' },
    { value: 'animation_3d', label: '3D 动画电影' },
    { value: 'children_photo_poster', label: '儿童写真海报' },
    { value: 'cinematic', label: '电影旅拍' }
  ]
}

const referenceAssetsPayload = {
  items: [
    {
      id: 101,
      original_filename: 'first-person.png',
      preview_url: '/api/reference-assets/101/file',
      created_at: '2026-05-01T01:00:00Z'
    },
    {
      id: 102,
      original_filename: 'second-person.png',
      preview_url: '/api/reference-assets/102/file',
      created_at: '2026-05-02T01:00:00Z'
    }
  ]
}

describe('CoupleAlbumWorkspaceView', () => {
  beforeEach(() => {
    Object.values(apiMocks).forEach((mock) => mock.mockReset())
    routerPush.mockReset()
    routePath.value = { path: '/workspace/couple-album' }
    apiMocks.getCoupleAlbumOptions.mockResolvedValue(optionsPayload)
    apiMocks.listCoupleAlbums.mockResolvedValue({
      albums: [
        {
          id: 8,
          title: '西湖纪念日',
          location: '杭州',
          status: 'succeeded',
          pages: [{ id: 81, status: 'succeeded', preview_url: '/api/works/81/preview' }]
        }
      ]
    })
    apiMocks.listReferenceAssets.mockResolvedValue(referenceAssetsPayload)
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('loads backend options and recent albums', async () => {
    const wrapper = mountWorkspace()
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(apiMocks.getCoupleAlbumOptions).toHaveBeenCalled()
    expect(apiMocks.listCoupleAlbums).toHaveBeenCalled()
    expect(wrapper.text()).toContain('情侣相册')
    expect(wrapper.text()).toContain('杭州西湖')
    expect(wrapper.text()).toContain('纪念日')
    expect(wrapper.text()).toContain('电影旅拍')
    expect(wrapper.text()).toContain('西湖纪念日')
    expect(wrapper.get('[data-testid="recent-couple-album-8"]').attributes('href')).toBe('/workspace/couple-album/8')
  })

  it('uploads both reference images and stops before create when credits are insufficient', async () => {
    apiMocks.uploadReferenceAsset
      .mockResolvedValueOnce({ id: 1, preview_url: '/api/reference-assets/1/preview', original_filename: 'male.png' })
      .mockResolvedValueOnce({ id: 2, preview_url: '/api/reference-assets/2/preview', original_filename: 'female.png' })
    apiMocks.estimateCoupleAlbum.mockResolvedValueOnce({
      required_credits: 24,
      available_credits: 8,
      missing_credits: 16,
      enough: false,
      recommended_package: { id: 3, name: '高频包' }
    })

    const wrapper = mountWorkspace()
    await flushPromises()

    const uploadZones = wrapper.findAllComponents(ImageUploadZone)
    uploadZones[0].vm.$emit('upload', new File(['male'], 'male.png', { type: 'image/png' }))
    uploadZones[1].vm.$emit('upload', new File(['female'], 'female.png', { type: 'image/png' }))
    await flushPromises()
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="couple-album-submit"]').trigger('click')
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(apiMocks.estimateCoupleAlbum).toHaveBeenCalledWith({
      title: '我们的 520 旅行相册',
      location: '杭州',
      story_template: 'anniversary',
      style: 'cinematic',
      male_reference_asset_id: 1,
      female_reference_asset_id: 2
    })
    expect(apiMocks.createCoupleAlbum).not.toHaveBeenCalled()
    expect(apiMocks.generateCoupleAlbum).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('本次预计消耗 24 点')
    expect(wrapper.text()).toContain('还差 16 点')

    await wrapper.get('[data-testid="couple-album-recharge"]').trigger('click')
    expect(routerPush).toHaveBeenCalledWith({
      path: '/pricing',
      query: {
        source: 'couple_album',
        missing_credits: 16,
        required_credits: 24,
        package_id: 3
      }
    })
  })

  it('creates, generates, and navigates to the album detail page when credits are enough', async () => {
    apiMocks.uploadReferenceAsset
      .mockResolvedValueOnce({ id: 1, preview_url: '/api/reference-assets/1/preview', original_filename: 'male.png' })
      .mockResolvedValueOnce({ id: 2, preview_url: '/api/reference-assets/2/preview', original_filename: 'female.png' })
    apiMocks.estimateCoupleAlbum.mockResolvedValueOnce({
      required_credits: 24,
      available_credits: 80,
      missing_credits: 0,
      enough: true
    })
    apiMocks.createCoupleAlbum.mockResolvedValueOnce({ album: { id: 18 } })
    apiMocks.generateCoupleAlbum.mockResolvedValueOnce({ album: { id: 18, status: 'generating' } })

    const wrapper = mountWorkspace()
    await flushPromises()

    const uploadZones = wrapper.findAllComponents(ImageUploadZone)
    uploadZones[0].vm.$emit('upload', new File(['male'], 'male.png', { type: 'image/png' }))
    uploadZones[1].vm.$emit('upload', new File(['female'], 'female.png', { type: 'image/png' }))
    await flushPromises()

    await wrapper.get('[data-testid="couple-album-submit"]').trigger('click')
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(apiMocks.createCoupleAlbum).toHaveBeenCalledOnce()
    expect(apiMocks.generateCoupleAlbum).toHaveBeenCalledWith(18)
    expect(routerPush).toHaveBeenCalledWith('/workspace/couple-album/18')
  })

  it('selects existing assets from the library for both couple album roles and submits their asset ids', async () => {
    apiMocks.estimateCoupleAlbum.mockResolvedValueOnce({
      required_credits: 24,
      available_credits: 80,
      missing_credits: 0,
      enough: true
    })
    apiMocks.createCoupleAlbum.mockResolvedValueOnce({ album: { id: 36 } })
    apiMocks.generateCoupleAlbum.mockResolvedValueOnce({ album: { id: 36, status: 'generating' } })

    const wrapper = mountWorkspace()
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="couple-album-male-upload"]').text()).toContain('从素材库选择')
    expect(wrapper.get('[data-testid="couple-album-female-upload"]').text()).toContain('点击上传、拖拽图片或从素材库选择')

    await wrapper.get('[data-testid="couple-album-open-asset-picker-male"]').trigger('click')
    await flushPromises()
    expect(apiMocks.listReferenceAssets).toHaveBeenCalled()
    expect(wrapper.get('[data-testid="couple-album-asset-picker"]').text()).toContain('first-person.png')
    await wrapper.get('[data-testid="couple-album-asset-option-101"]').trigger('click')
    await wrapper.get('[data-testid="couple-album-asset-picker-confirm"]').trigger('click')
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="couple-album-open-asset-picker-female"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="couple-album-asset-option-102"]').trigger('click')
    await wrapper.get('[data-testid="couple-album-asset-picker-confirm"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="couple-album-male-upload"] img').attributes('src')).toBe('/api/reference-assets/101/file')
    expect(wrapper.get('[data-testid="couple-album-female-upload"] img').attributes('src')).toBe('/api/reference-assets/102/file')

    await wrapper.get('[data-testid="couple-album-submit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.uploadReferenceAsset).not.toHaveBeenCalled()
    expect(apiMocks.estimateCoupleAlbum).toHaveBeenCalledWith({
      title: '我们的 520 旅行相册',
      location: '杭州',
      story_template: 'anniversary',
      style: 'cinematic',
      male_reference_asset_id: 101,
      female_reference_asset_id: 102
    })
    expect(apiMocks.generateCoupleAlbum).toHaveBeenCalledWith(36)
    expect(routerPush).toHaveBeenCalledWith('/workspace/couple-album/36')
  })

  it('does not delete library assets when removing a selected library image from a role slot', async () => {
    const wrapper = mountWorkspace()
    await flushPromises()

    await wrapper.get('[data-testid="couple-album-open-asset-picker-male"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="couple-album-asset-option-101"]').trigger('click')
    await wrapper.get('[data-testid="couple-album-asset-picker-confirm"]').trigger('click')
    await wrapper.vm.$nextTick()

    wrapper.findAllComponents(ImageUploadZone)[0].vm.$emit('remove', {
      id: 101,
      preview_url: '/api/reference-assets/101/file',
      original_filename: 'first-person.png',
      _selectedFromLibrary: true
    })
    await wrapper.vm.$nextTick()

    expect(apiMocks.deleteReferenceAsset).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="couple-album-male-upload"] img').exists()).toBe(false)
  })

  it('shows empty and error states in the asset picker without blocking local uploads', async () => {
    apiMocks.listReferenceAssets.mockResolvedValueOnce({ items: [] }).mockRejectedValueOnce(new Error('素材读取失败'))

    const wrapper = mountWorkspace()
    await flushPromises()

    await wrapper.get('[data-testid="couple-album-open-asset-picker-male"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="couple-album-asset-picker"]').text()).toContain('素材库暂无图片')
    expect(wrapper.find('[data-testid="couple-album-asset-picker-confirm"]').attributes('disabled')).toBeDefined()

    await wrapper.get('[data-testid="couple-album-asset-picker-close"]').trigger('click')
    await wrapper.vm.$nextTick()
    await wrapper.get('[data-testid="couple-album-open-asset-picker-female"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="couple-album-asset-picker"]').text()).toContain('素材读取失败')
    expect(wrapper.findAllComponents(ImageUploadZone)).toHaveLength(2)
  })

  it('submits a childhood dream album with only the child photo required', async () => {
    routePath.value = { path: '/workspace/childhood-dream-album' }
    apiMocks.getCoupleAlbumOptions.mockResolvedValueOnce(childhoodOptionsPayload)
    apiMocks.uploadReferenceAsset.mockResolvedValueOnce({
      id: 11,
      preview_url: '/api/reference-assets/11/preview',
      original_filename: 'child.png'
    })
    apiMocks.estimateCoupleAlbum.mockResolvedValueOnce({
      required_credits: 24,
      available_credits: 80,
      missing_credits: 0,
      enough: true
    })
    apiMocks.createCoupleAlbum.mockResolvedValueOnce({ album: { id: 28 } })
    apiMocks.generateCoupleAlbum.mockResolvedValueOnce({ album: { id: 28, status: 'generating' } })

    const wrapper = mountWorkspace()
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('童年梦想相册')
    expect(wrapper.get('[data-testid="childhood-dream-workspace"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="couple-album-title"]').element.value).toBe('我的六一梦想相册')
    expect(wrapper.text()).toContain('孩子照片')
    expect(wrapper.text()).toContain('补充全身照')
    expect(wrapper.get('[data-testid="couple-album-male-upload"]').text()).toContain('从素材库选择')
    expect(wrapper.get('[data-testid="couple-album-female-upload"]').text()).toContain('从素材库选择')
    expect(wrapper.findAll('[data-testid^="childhood-dream-step-"]')).toHaveLength(3)
    expect(wrapper.findAll('[data-testid^="childhood-dream-theme-"]')).toHaveLength(4)
    expect(wrapper.text()).toContain('相册内容')
    expect(wrapper.findAll('[data-testid^="childhood-dream-style-"]')).toHaveLength(4)
    expect(wrapper.text()).toContain('为什么选择童年梦想相册')
    expect(wrapper.text()).toContain('童话绘本')
    expect(wrapper.text()).toContain('梦幻水彩')
    expect(wrapper.text()).not.toContain('电影旅拍')

    const uploadZones = wrapper.findAllComponents(ImageUploadZone)
    uploadZones[0].vm.$emit('upload', new File(['child'], 'child.png', { type: 'image/png' }))
    await flushPromises()
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="couple-album-submit"]').trigger('click')
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(apiMocks.estimateCoupleAlbum).toHaveBeenCalledWith({
      title: '我的六一梦想相册',
      location: 'childhood_dream_stage',
      story_template: 'childhood_career_dream',
      style: 'children_storybook',
      male_reference_asset_id: 11,
      female_reference_asset_id: 0
    })
    expect(apiMocks.createCoupleAlbum).toHaveBeenCalledOnce()
    expect(apiMocks.generateCoupleAlbum).toHaveBeenCalledWith(28)
    expect(routerPush).toHaveBeenCalledWith('/workspace/couple-album/28')
  })

  it('uses the selected childhood dream theme in submit payload and only lists childhood recent albums', async () => {
    routePath.value = { path: '/workspace/childhood-dream-album' }
    apiMocks.getCoupleAlbumOptions.mockResolvedValueOnce(childhoodOptionsPayload)
    apiMocks.listCoupleAlbums.mockResolvedValueOnce({
      albums: [
        {
          id: 31,
          title: '太空梦想',
          location: 'childhood_space_adventure',
          story_template: 'childhood_career_dream',
          status: 'generating',
          pages: [
            { id: 311, page_number: 1, status: 'succeeded', preview_url: '/api/works/311/preview' },
            { id: 312, page_number: 2, status: 'queued', preview_url: '' }
          ]
        },
        {
          id: 8,
          title: '西湖纪念日',
          location: '杭州',
          story_template: 'anniversary',
          status: 'succeeded',
          pages: [{ id: 81, status: 'succeeded', preview_url: '/api/works/81/preview' }]
        }
      ]
    })
    apiMocks.uploadReferenceAsset.mockResolvedValueOnce({
      id: 12,
      preview_url: '/api/reference-assets/12/preview',
      original_filename: 'child.png'
    })
    apiMocks.estimateCoupleAlbum.mockResolvedValueOnce({
      required_credits: 24,
      available_credits: 80,
      missing_credits: 0,
      enough: true
    })
    apiMocks.createCoupleAlbum.mockResolvedValueOnce({ album: { id: 32 } })
    apiMocks.generateCoupleAlbum.mockResolvedValueOnce({ album: { id: 32, status: 'generating' } })

    const wrapper = mountWorkspace()
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('太空梦想')
    expect(wrapper.text()).toContain('1/8')
    expect(wrapper.text()).not.toContain('西湖纪念日')

    await wrapper.get('[data-testid="childhood-dream-theme-childhood_space_adventure"]').trigger('click')
    const uploadZones = wrapper.findAllComponents(ImageUploadZone)
    uploadZones[0].vm.$emit('upload', new File(['child'], 'child.png', { type: 'image/png' }))
    await flushPromises()

    await wrapper.get('[data-testid="couple-album-submit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.estimateCoupleAlbum).toHaveBeenCalledWith(expect.objectContaining({
      location: 'childhood_space_adventure',
      story_template: 'childhood_career_dream',
      male_reference_asset_id: 12,
      female_reference_asset_id: 0
    }))
  })
})
