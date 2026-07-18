import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  getMe: vi.fn(),
  listWorks: vi.fn(),
  uploadReferenceAsset: vi.fn(),
  createImageGeneration: vi.fn(),
  getImageGeneration: vi.fn()
}))

const routerMocks = vi.hoisted(() => ({
  push: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    getMe: apiMocks.getMe,
    listWorks: apiMocks.listWorks,
    uploadReferenceAsset: apiMocks.uploadReferenceAsset,
    createImageGeneration: apiMocks.createImageGeneration,
    getImageGeneration: apiMocks.getImageGeneration
  }
}))

vi.mock('vue-router', () => ({
  useRouter: () => routerMocks
}))

import OldPhotoRestorationView from '../views/OldPhotoRestorationView.vue'
import { clearCurrentUser, currentUser } from '../stores/session.js'

describe('OldPhotoRestorationView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    clearCurrentUser()
    apiMocks.getMe.mockResolvedValue({ user_id: 12, username: 'creator', available_credits: 8 })
    apiMocks.listWorks.mockResolvedValue({ items: [] })
    vi.stubGlobal('URL', {
      createObjectURL: vi.fn(() => 'blob:old-photo-preview'),
      revokeObjectURL: vi.fn()
    })
    vi.stubGlobal('open', vi.fn())
  })

  it('renders the restored-photo workspace with upload, comparison, settings, and history areas', () => {
    const wrapper = mount(OldPhotoRestorationView)

    expect(wrapper.get('[data-testid="old-photo-upload-panel"]').text()).toContain('上传你的老照片')
    expect(wrapper.get('[data-testid="old-photo-comparison-panel"]').text()).toContain('修复效果对比')
    expect(wrapper.get('[data-testid="old-photo-settings-panel"]').text()).toContain('修复参数设置')
    expect(wrapper.get('[data-testid="old-photo-history-panel"]').text()).toContain('修复历史')
  })

  it('keeps the default restoration controls aligned with the referenced mockup', () => {
    const wrapper = mount(OldPhotoRestorationView)

    expect(wrapper.get('[data-testid="old-photo-mode"]').element.value).toBe('smart')
    expect(wrapper.get('[data-testid="old-photo-strength"]').element.value).toBe('80')
    expect(wrapper.get('[data-testid="old-photo-color"]').element.value).toBe('70')
    expect(wrapper.get('[data-testid="old-photo-sharpness"]').element.value).toBe('60')
    expect(wrapper.get('[data-testid="old-photo-face-enhance"]').element.checked).toBe(true)
    expect(wrapper.get('[data-testid="old-photo-detail-preserve"]').element.checked).toBe(true)
  })

  it('places processing information and primary actions in the lower action bar', () => {
    const wrapper = mount(OldPhotoRestorationView)
    const bottomBar = wrapper.get('[data-testid="old-photo-bottom-actions"]')

    expect(bottomBar.text()).toContain('处理信息')
    expect(bottomBar.text()).toContain('预计耗时 20s')
    expect(bottomBar.text()).toContain('预计消耗 2 点')
    expect(bottomBar.text()).toContain('高清导出')
    expect(bottomBar.text()).toContain('私密处理')
    expect(bottomBar.text()).toContain('重置')
    expect(bottomBar.text()).toContain('开始修复')
    expect(bottomBar.text()).toContain('下载高清图')
    expect(bottomBar.text()).toContain('保存到作品库')
    expect(wrapper.find('[data-testid="old-photo-settings-panel"] .old-photo-reset').exists()).toBe(false)
  })

  it('blocks old-photo restoration when credits do not cover upload plus generated image', async () => {
    apiMocks.getMe.mockResolvedValueOnce({ user_id: 12, username: 'creator', available_credits: 1 })
    apiMocks.uploadReferenceAsset.mockResolvedValue({
      id: 7,
      preview_url: '/api/reference-assets/7/file',
      original_filename: 'family.png'
    })
    const wrapper = mount(OldPhotoRestorationView)
    const input = wrapper.get('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [new File(['fake'], 'family.png', { type: 'image/png' })], configurable: true })
    await input.trigger('change')
    await flushPromises()

    await wrapper.get('[data-testid="old-photo-start"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createImageGeneration).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('点数不足，本次预计消耗 2 点')
  })

  it('uploads the selected photo through the reference asset API and uses the uploaded preview', async () => {
    apiMocks.uploadReferenceAsset.mockResolvedValue({
      id: 7,
      preview_url: '/api/reference-assets/7/file',
      original_filename: 'grandparents.png'
    })
    const wrapper = mount(OldPhotoRestorationView)
    const file = new File(['fake'], 'grandparents.png', { type: 'image/png' })
    const input = wrapper.get('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [file], configurable: true })

    await input.trigger('change')
    await flushPromises()

    expect(apiMocks.uploadReferenceAsset).toHaveBeenCalledWith(file)
    expect(wrapper.get('[data-testid="old-photo-active-thumb"]').attributes('src')).toBe('/api/reference-assets/7/file')
  })

  it('uploads dropped photos and rejects invalid or oversized files before upload', async () => {
    apiMocks.uploadReferenceAsset.mockResolvedValue({
      id: 7,
      preview_url: '/api/reference-assets/7/file',
      original_filename: 'dropped.png'
    })
    const wrapper = mount(OldPhotoRestorationView)
    const dropzone = wrapper.get('[data-testid="old-photo-dropzone"]')

    const oversized = new File(['fake'], 'too-large.png', { type: 'image/png' })
    Object.defineProperty(oversized, 'size', { value: 51 * 1024 * 1024 })
    await dropzone.trigger('drop', { dataTransfer: { files: [oversized] } })
    await flushPromises()

    expect(apiMocks.uploadReferenceAsset).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('单张图片不能超过 50MB')

    const invalid = new File(['fake'], 'notes.txt', { type: 'text/plain' })
    await dropzone.trigger('drop', { dataTransfer: { files: [invalid] } })
    await flushPromises()

    expect(apiMocks.uploadReferenceAsset).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('仅支持 JPG、PNG、WEBP 图片')

    const valid = new File(['fake'], 'dropped.png', { type: 'image/png' })
    await dropzone.trigger('drop', { dataTransfer: { files: [valid] } })
    await flushPromises()

    expect(apiMocks.uploadReferenceAsset).toHaveBeenCalledWith(valid)
    expect(wrapper.get('[data-testid="old-photo-active-thumb"]').attributes('src')).toBe('/api/reference-assets/7/file')
  })

  it('removes the uploaded photo and clears generated state', async () => {
    apiMocks.uploadReferenceAsset.mockResolvedValue({
      id: 7,
      preview_url: '/api/reference-assets/7/file',
      original_filename: 'family.png'
    })
    apiMocks.createImageGeneration.mockResolvedValue({ generation_id: 99, status: 'queued' })
    apiMocks.getImageGeneration.mockResolvedValue({
      generation_id: 99,
      status: 'succeeded',
      work_id: 101,
      preview_url: '/api/works/101/file',
      download_url: '/api/works/101/download'
    })
    const wrapper = mount(OldPhotoRestorationView)
    const input = wrapper.get('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [new File(['fake'], 'family.png', { type: 'image/png' })], configurable: true })
    await input.trigger('change')
    await wrapper.get('[data-testid="old-photo-start"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="old-photo-restored-image"]').attributes('src')).toBe('/api/works/101/file')
    await wrapper.get('[data-testid="old-photo-remove"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="old-photo-active-thumb"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="old-photo-start"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="old-photo-restored-image"]').attributes('src')).toContain('old-photo-family')
  })

  it('switches between split and draggable slide comparison modes', async () => {
    const wrapper = mount(OldPhotoRestorationView)

    expect(wrapper.find('[data-testid="old-photo-split-stage"]').exists()).toBe(true)
    await wrapper.get('[data-testid="old-photo-mode-slide"]').trigger('click')

    expect(wrapper.find('[data-testid="old-photo-split-stage"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="old-photo-slide-stage"]').exists()).toBe(true)
    const slider = wrapper.get('[data-testid="old-photo-compare-slider"]')
    expect(slider.element.value).toBe('50')

    await slider.setValue('68')
    expect(wrapper.get('[data-testid="old-photo-slide-after"]').attributes('style')).toContain('68%')
  })

  it('updates zoom, toggles pan mode, and opens fullscreen preview', async () => {
    const wrapper = mount(OldPhotoRestorationView)

    await wrapper.get('[data-testid="old-photo-zoom-in"]').trigger('click')
    expect(wrapper.get('[data-testid="old-photo-zoom-label"]').text()).toBe('125%')

    await wrapper.get('[data-testid="old-photo-zoom-out"]').trigger('click')
    expect(wrapper.get('[data-testid="old-photo-zoom-label"]').text()).toBe('100%')

    await wrapper.get('[data-testid="old-photo-pan-toggle"]').trigger('click')
    expect(wrapper.get('[data-testid="old-photo-pan-toggle"]').classes()).toContain('active')

    await wrapper.get('[data-testid="old-photo-fullscreen"]').trigger('click')
    expect(wrapper.find('[data-testid="old-photo-fullscreen-modal"]').exists()).toBe(true)
    await wrapper.get('[data-testid="old-photo-fullscreen-close"]').trigger('click')
    expect(wrapper.find('[data-testid="old-photo-fullscreen-modal"]').exists()).toBe(false)
  })

  it('includes advanced noise suppression in the restoration prompt and resets it', async () => {
    apiMocks.uploadReferenceAsset.mockResolvedValue({
      id: 7,
      preview_url: '/api/reference-assets/7/file',
      original_filename: 'family.png'
    })
    apiMocks.createImageGeneration.mockResolvedValue({ generation_id: 99, status: 'queued' })
    apiMocks.getImageGeneration.mockResolvedValue({ generation_id: 99, status: 'failed', error: { message: 'stopped' } })
    const wrapper = mount(OldPhotoRestorationView)
    const input = wrapper.get('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [new File(['fake'], 'family.png', { type: 'image/png' })], configurable: true })
    await input.trigger('change')
    await flushPromises()

    await wrapper.get('[data-testid="old-photo-advanced-toggle"]').trigger('click')
    await wrapper.get('[data-testid="old-photo-noise-level"]').setValue('strong')
    await wrapper.get('[data-testid="old-photo-start"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: expect.stringContaining('强力抑制照片噪点')
    }))

    await wrapper.get('.old-photo-footer-button').trigger('click')
    expect(wrapper.get('[data-testid="old-photo-noise-level"]').element.value).toBe('standard')
  })

  it('loads real old-photo history and shows an empty state when none match', async () => {
    apiMocks.listWorks.mockResolvedValueOnce({
      items: [
        {
          work_id: 10,
          prompt: '老照片修复。增强人物面部清晰度。',
          preview_url: '/api/works/10/file',
          created_at: '2026-05-09T08:00:00Z',
          status: 'succeeded'
        },
        {
          work_id: 11,
          prompt: '普通海报',
          preview_url: '/api/works/11/file',
          created_at: '2026-05-09T07:00:00Z',
          status: 'succeeded'
        }
      ]
    })
    const wrapper = mount(OldPhotoRestorationView)
    await flushPromises()

    expect(apiMocks.listWorks).toHaveBeenCalledWith({ category: 'image', page: 1, page_size: 3 })
    expect(wrapper.text()).toContain('老照片修复。增强人物面部清晰度。')
    expect(wrapper.text()).not.toContain('普通海报')
    expect(wrapper.text()).not.toContain('2024-05-18')

    apiMocks.listWorks.mockResolvedValueOnce({ items: [] })
    const emptyWrapper = mount(OldPhotoRestorationView)
    await flushPromises()

    expect(emptyWrapper.text()).toContain('暂无修复历史')
    expect(emptyWrapper.text()).not.toContain('家庭合影修复')
  })

  it('starts a real restoration generation, polls completion, and renders the restored result', async () => {
    apiMocks.uploadReferenceAsset.mockResolvedValue({
      id: 7,
      preview_url: '/api/reference-assets/7/file',
      original_filename: 'family.png'
    })
    apiMocks.createImageGeneration.mockResolvedValue({
      generation_id: 99,
      status: 'queued',
      stage: 'queued',
      available_credits: 8
    })
    apiMocks.getImageGeneration.mockResolvedValue({
      generation_id: 99,
      status: 'succeeded',
      work_id: 101,
      preview_url: '/api/works/101/file',
      download_url: '/api/works/101/download',
      available_credits: 7
    })
    const wrapper = mount(OldPhotoRestorationView)
    const file = new File(['fake'], 'family.png', { type: 'image/png' })
    const input = wrapper.get('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
    await input.trigger('change')
    await flushPromises()

    await wrapper.get('[data-testid="old-photo-start"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith({
      prompt: expect.stringContaining('老照片修复'),
      negative_prompt: expect.stringContaining('不要改变人物身份'),
      aspect_ratio: '1:1',
      quality: 'high',
      style_preset: '老照片修复',
      tool_mode: 'generate',
      style_strength: 80,
      reference_weight: 70,
      reference_asset_ids: [7]
    })
    expect(apiMocks.getImageGeneration).toHaveBeenCalledWith(99)
    expect(wrapper.get('[data-testid="old-photo-restored-image"]').attributes('src')).toBe('/api/works/101/file')
    expect(wrapper.text()).toContain('修复完成')
    expect(wrapper.text()).toContain('剩余点数 7')
    expect(currentUser.value?.available_credits).toBe(7)
  })

  it('shows restoration failure reasons and retries the same uploaded photo', async () => {
    apiMocks.uploadReferenceAsset.mockResolvedValue({
      id: 7,
      preview_url: '/api/reference-assets/7/file',
      original_filename: 'family.png'
    })
    apiMocks.createImageGeneration
      .mockResolvedValueOnce({
        generation_id: 99,
        status: 'queued',
        stage: 'queued',
        available_credits: 8
      })
      .mockResolvedValueOnce({
        generation_id: 100,
        status: 'queued',
        stage: 'queued',
        available_credits: 8
      })
    apiMocks.getImageGeneration.mockResolvedValueOnce({
      generation_id: 99,
      status: 'failed',
      stage: 'failed',
      error: {
        code: 'provider_timeout',
        message: '图片服务响应超时，系统已自动重试 2 次仍未完成，请稍后重新生成。',
        retryable: true
      }
    })
    const wrapper = mount(OldPhotoRestorationView)
    const file = new File(['fake'], 'family.png', { type: 'image/png' })
    const input = wrapper.get('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
    await input.trigger('change')
    await flushPromises()

    await wrapper.get('[data-testid="old-photo-start"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('图片服务响应超时，系统已自动重试 2 次仍未完成，请稍后重新生成。')
    await wrapper.get('[data-testid="old-photo-retry-generation"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledTimes(2)
    expect(apiMocks.createImageGeneration).toHaveBeenLastCalledWith(expect.objectContaining({
      prompt: expect.stringContaining('老照片修复'),
      reference_asset_ids: [7]
    }))
  })

  it('downloads the restored image and navigates to the works library after completion', async () => {
    apiMocks.uploadReferenceAsset.mockResolvedValue({
      id: 7,
      preview_url: '/api/reference-assets/7/file',
      original_filename: 'family.png'
    })
    apiMocks.createImageGeneration.mockResolvedValue({ generation_id: 99, status: 'queued' })
    apiMocks.getImageGeneration.mockResolvedValue({
      generation_id: 99,
      status: 'succeeded',
      work_id: 101,
      preview_url: '/api/works/101/file',
      download_url: '/api/works/101/download',
      available_credits: 7
    })
    const wrapper = mount(OldPhotoRestorationView)
    const file = new File(['fake'], 'family.png', { type: 'image/png' })
    const input = wrapper.get('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
    await input.trigger('change')
    await wrapper.get('[data-testid="old-photo-start"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="old-photo-download"]').trigger('click')
    await wrapper.get('[data-testid="old-photo-save"]').trigger('click')

    expect(window.open).toHaveBeenCalledWith('/api/works/101/download', '_blank')
    expect(routerMocks.push).toHaveBeenCalledWith('/works')
  })
})
