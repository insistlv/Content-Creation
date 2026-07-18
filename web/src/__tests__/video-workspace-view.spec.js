import { flushPromises, mount as mountBase } from '@vue/test-utils'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  getMe: vi.fn(),
  listReferenceAssets: vi.fn(),
  uploadReferenceAsset: vi.fn(),
  deleteReferenceAsset: vi.fn(),
  estimateVideoGeneration: vi.fn(),
  createVideoGeneration: vi.fn(),
  getVideoGeneration: vi.fn(),
  listVideoModels: vi.fn(),
  listUserVideoGenerations: vi.fn(),
  listVideoSoundtracks: vi.fn(),
  generateVideoSoundtrack: vi.fn(),
  uploadVideoSoundtrack: vi.fn(),
  listVideoStylePresets: vi.fn(),
  listVideoStyleTemplates: vi.fn(),
  createVideoStyleTemplate: vi.fn(),
  deleteVideoStyleTemplate: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    getMe: apiMocks.getMe,
    listReferenceAssets: apiMocks.listReferenceAssets,
    uploadReferenceAsset: apiMocks.uploadReferenceAsset,
    deleteReferenceAsset: apiMocks.deleteReferenceAsset,
    estimateVideoGeneration: apiMocks.estimateVideoGeneration,
    createVideoGeneration: apiMocks.createVideoGeneration,
    getVideoGeneration: apiMocks.getVideoGeneration,
    listVideoModels: apiMocks.listVideoModels,
    listUserVideoGenerations: apiMocks.listUserVideoGenerations,
    listVideoSoundtracks: apiMocks.listVideoSoundtracks,
    generateVideoSoundtrack: apiMocks.generateVideoSoundtrack,
    uploadVideoSoundtrack: apiMocks.uploadVideoSoundtrack,
    listVideoStylePresets: apiMocks.listVideoStylePresets,
    listVideoStyleTemplates: apiMocks.listVideoStyleTemplates,
    createVideoStyleTemplate: apiMocks.createVideoStyleTemplate,
    deleteVideoStyleTemplate: apiMocks.deleteVideoStyleTemplate
  }
}))

import VideoWorkspaceView from '../views/VideoWorkspaceView.vue'
import { clearCurrentUser, currentUser } from '../stores/session.js'
import { chooseClickSelect, clickSelectMenu, clickSelectOption } from './click-select-test-utils.js'

const mountedWrappers = new Set()

function mount(...args) {
  const wrapper = mountBase(...args)
  mountedWrappers.add(wrapper)
  return wrapper
}

const stylesSource = readFileSync(resolve(dirname(fileURLToPath(import.meta.url)), '../styles.css'), 'utf8')
const grokImagineVideoModel = 'grok-imagine-video-1.5-preview'

function cssRule(selector) {
  const escapedSelector = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  return stylesSource.match(new RegExp(`${escapedSelector}\\s*\\{([^}]*)\\}`, 's'))?.[1] ?? ''
}
const referenceImageRequiredMessage = '⚠️ 当前模型需参考图才能生成视频，暂不支持纯文本生成视频'

function referenceAsset(id, filename = `参考图 ${id}.png`, kind = 'image') {
  return {
    id,
    preview_url: `/assets/${id}.jpg`,
    original_filename: filename,
    kind
  }
}

function officialStyle(id = 1, overrides = {}) {
  const titles = {
    1: 'Cinematic Realism',
    2: 'Dream Motion',
    3: 'Product Macro'
  }
  return {
    id,
    slug: `style-${id}`,
    title: titles[id] ?? `Style ${id}`,
    category: 'film',
    description: 'soft lighting and film texture',
    tags: ['popular', 'beginner'],
    preview_url: `/style/${id}.jpg`,
    sort_order: 1,
    is_active: true,
    use_count: 12,
    ...overrides
  }
}

function customStyle(id = 9) {
  return {
    id,
    title: 'Brand Film',
    description: 'warm brand film tone',
    reference_asset_id: 77,
    preview_url: '/assets/77.jpg',
    style_prompt: 'warm brand film tone',
    is_active: true,
    use_count: 0
  }
}

function fileNamed(name) {
  return new File(['fake'], name, { type: 'image/png' })
}

function mediaFileNamed(name, type) {
  return new File(['fake'], name, { type })
}

function deferred() {
  let resolve
  let reject
  const promise = new Promise((promiseResolve, promiseReject) => {
    resolve = promiseResolve
    reject = promiseReject
  })
  return { promise, resolve, reject }
}

async function uploadFiles(wrapper, files) {
  const input = wrapper.get('[data-testid="video-reference-upload-input"]')
  Object.defineProperty(input.element, 'files', { value: files, configurable: true })
  await input.trigger('change')
  await flushPromises()
}

async function uploadFilesByTestId(wrapper, testID, files) {
  const input = wrapper.get(`[data-testid="${testID}"]`)
  Object.defineProperty(input.element, 'files', { value: files, configurable: true })
  await input.trigger('change')
  await flushPromises()
}

async function replaceFile(wrapper, index, file) {
  const input = wrapper.findAll('[data-testid="video-reference-replace-input"]')[index]
  Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
  await input.trigger('change')
  await flushPromises()
}

function mockCompletedVideo(id = 91) {
  apiMocks.createVideoGeneration.mockResolvedValueOnce({
    generation_id: id,
    status: 'queued',
    available_credits: 16
  })
  apiMocks.getVideoGeneration.mockResolvedValueOnce({
    generation_id: id,
    status: 'succeeded',
    available_credits: 15
  })
}

function videoHistoryItem(overrides = {}) {
  return {
    id: 1,
    generation_id: 101,
    work_id: 901,
    status: 'succeeded',
    prompt: '日系清新风格，柔和自然光的视频版本',
    prompt_summary: '日系清新风格，柔和自然光的视频版本',
    preview_url: '/api/works/901/file',
    download_url: '/api/works/901/download',
    aspect_ratio: '16:9',
    duration_seconds: 10,
    model_name: 'Sora2 Pro',
    runtime_model: 'sora-2-pro',
    style_preset: 'Brand Film',
    credits_cost: 8,
    created_at: '2026-06-11T08:00:00Z',
    enhancement_tags: ['高清', '参考图', '风格模板', 'Pro'],
    reference_asset_ids: [2, 3],
    reference_asset_count: 2,
    hd: true,
    ...overrides
  }
}

describe('VideoWorkspaceView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    clearCurrentUser()
    apiMocks.estimateVideoGeneration.mockReset()
    apiMocks.createVideoGeneration.mockReset()
    apiMocks.getVideoGeneration.mockReset()
    apiMocks.listVideoModels.mockResolvedValue({
      items: [
        {
          name: 'Grok Imagine',
          runtime_model: grokImagineVideoModel,
          provider: 'Wuyin',
          aspect_ratios: ['16:9', '9:16'],
          durations: ['1', '3', '6', '10', '15'],
          default_duration: '3',
          supports_hd: true,
          max_reference_images: 4
        },
        {
          name: 'Sora2',
          runtime_model: 'sora-2',
          provider: 'GPT-Best',
          aspect_ratios: ['16:9', '9:16'],
          durations: ['10', '15', '25'],
          default_duration: '10',
          supports_hd: true,
          max_reference_images: 4
        },
        {
          name: 'Sora2 Pro',
          runtime_model: 'sora-2-pro',
          provider: 'GPT-Best',
          aspect_ratios: ['16:9', '9:16'],
          durations: ['10', '15', '25'],
          default_duration: '10',
          supports_hd: true,
          max_reference_images: 4
        }
      ]
    })
    apiMocks.listUserVideoGenerations.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 8 })
    apiMocks.getMe.mockResolvedValue({ user_id: 12, available_credits: 20 })
    apiMocks.estimateVideoGeneration.mockResolvedValue({
      required_credits: 9,
      available_credits: 20,
      missing_credits: 0,
      enough: true,
      billing_policy: 'success_only',
      message: '提交前预估，生成成功后扣点，失败不扣点'
    })
    apiMocks.listReferenceAssets.mockResolvedValue({ items: [] })
    apiMocks.uploadReferenceAsset.mockReset()
    apiMocks.deleteReferenceAsset.mockReset()
    apiMocks.listVideoSoundtracks.mockResolvedValue({ items: [] })
    apiMocks.generateVideoSoundtrack.mockReset()
    apiMocks.uploadVideoSoundtrack.mockReset()
    apiMocks.listVideoStylePresets.mockResolvedValue({ items: [officialStyle()] })
    apiMocks.listVideoStyleTemplates.mockResolvedValue({ items: [] })
    apiMocks.createVideoStyleTemplate.mockReset()
    apiMocks.deleteVideoStyleTemplate.mockReset()
  })

  afterEach(() => {
    for (const wrapper of mountedWrappers) {
      wrapper.unmount()
    }
    mountedWrappers.clear()
    vi.useRealTimers()
  })

  it('renders the first official video style as a text-only preview by default', async () => {
    apiMocks.listVideoStylePresets.mockResolvedValueOnce({
      items: [
        officialStyle(1, {
          category: 'film',
          description: 'soft lighting and film texture',
          tags: ['popular', 'beginner']
        }),
        officialStyle(2, {
          category: 'motion',
          description: 'surreal camera moves with pastel atmosphere'
        })
      ]
    })

    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()

    const assistGrid = wrapper.get('[data-testid="video-creation-assist-grid"]')
    const library = wrapper.get('[data-testid="video-style-library"]')
    expect(assistGrid.find('[data-testid="video-style-library"]').exists()).toBe(true)
    expect(assistGrid.find('[data-testid="video-reference-pool"]').exists()).toBe(true)
    expect(apiMocks.listVideoStylePresets).toHaveBeenCalled()
    expect(library.classes()).toContain('video-style-library')
    expect(library.text()).toContain('视觉风格')
    expect(library.text()).toContain('Cinematic Realism')
    expect(library.text()).toContain('Official')
    expect(wrapper.get('[data-testid="video-style-preset-select"]').text()).toBe('Cinematic Realism')
    const previewCard = wrapper.get('[data-testid="video-style-preset-1"]')
    expect(previewCard.classes()).toContain('is-selected')
    expect(previewCard.find('img').exists()).toBe(false)
    expect(previewCard.text()).toContain('film')
    expect(previewCard.text()).toContain('soft lighting and film texture')
    expect(previewCard.text()).toContain('popular')
    expect(previewCard.text()).toContain('beginner')
    expect(wrapper.find('[data-testid="video-style-preset-2"]').exists()).toBe(false)
    expect(library.text()).toContain('1/2')
  })

  it('submits the first official video style by default without a click', async () => {
    mockCompletedVideo(100)

    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()

    await wrapper.get('[data-testid="video-prompt"]').setValue('make a product launch video')
    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createVideoGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: 'make a product launch video',
      video_style_preset_id: 1,
      style_preset: 'Cinematic Realism'
    }))
  })

  it('opens the hidden reference file input from the large reference dropzone', async () => {
    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()

    const input = wrapper.get('[data-testid="video-reference-upload-input"]')
    const clickSpy = vi.spyOn(input.element, 'click')
    const dropzone = wrapper.get('[data-testid="video-reference-dropzone"]')

    expect(dropzone.classes()).toContain('video-reference-dropzone')
    await dropzone.trigger('click')

    expect(clickSpy).toHaveBeenCalledTimes(1)
  })

  it('opens all official style choices from the preset dropdown', async () => {
    apiMocks.listVideoStylePresets.mockResolvedValueOnce({
      items: [
        officialStyle(1, {
          category: 'film',
          description: 'soft lighting and film texture'
        }),
        officialStyle(2, {
          category: 'motion',
          description: 'surreal camera moves with pastel atmosphere'
        }),
        officialStyle(3, {
          category: 'product',
          description: 'macro closeups with polished reflections'
        })
      ]
    })

    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()

    await wrapper.get('[data-testid="video-style-preset-select"]').trigger('click')
    await flushPromises()

    expect(clickSelectMenu('video-style-preset-select')).not.toBeNull()
    expect(clickSelectMenu('video-style-preset-select').querySelector('img')).toBeNull()
    expect(clickSelectOption('video-style-preset-select', 1)?.textContent).toBe('Cinematic Realism')
    expect(clickSelectOption('video-style-preset-select', 2)?.textContent).toBe('Dream Motion')
    expect(clickSelectOption('video-style-preset-select', 3)?.textContent).toBe('Product Macro')
    expect(clickSelectMenu('video-style-preset-select').textContent).not.toContain('film')
    expect(clickSelectMenu('video-style-preset-select').textContent).not.toContain('soft lighting and film texture')
    expect(clickSelectMenu('video-style-preset-select').textContent).not.toContain('motion')
    expect(clickSelectMenu('video-style-preset-select').textContent).not.toContain('surreal camera moves with pastel atmosphere')

    wrapper.unmount()
  })

  it('selects an official video style from the dropdown without generating and submits it later', async () => {
    apiMocks.listVideoStylePresets.mockResolvedValueOnce({
      items: [
        officialStyle(1),
        officialStyle(2, {
          category: 'motion',
          description: 'surreal camera moves with pastel atmosphere',
          tags: ['dream', 'story']
        })
      ]
    })
    mockCompletedVideo(101)

    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()

    await chooseClickSelect(wrapper, 'video-style-preset-select', 2)
    await flushPromises()

    expect(apiMocks.createVideoGeneration).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="video-style-preset-2"]').classes()).toContain('is-selected')
    expect(wrapper.get('[data-testid="video-style-preset-2"]').find('img').exists()).toBe(false)
    expect(wrapper.find('[data-testid="video-style-preset-1"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="video-style-preset-select"]').text()).toBe('Dream Motion')
    const selectedPreviewText = wrapper.get('[data-testid="video-style-library"]').text()
    expect(selectedPreviewText).toContain('motion')
    expect(selectedPreviewText).toContain('surreal camera moves with pastel atmosphere')
    expect(selectedPreviewText).toContain('dream')
    expect(selectedPreviewText).toContain('story')
    expect(selectedPreviewText).toContain('2/2')

    await wrapper.get('[data-testid="video-prompt"]').setValue('make a product launch video')
    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createVideoGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: 'make a product launch video',
      video_style_preset_id: 2,
      style_preset: 'Dream Motion'
    }))
  })

  it('shows the official empty state without injecting a default style id', async () => {
    apiMocks.listVideoStylePresets.mockResolvedValueOnce({ items: [] })
    mockCompletedVideo(103)

    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()

    expect(wrapper.get('[data-testid="video-style-empty-official"]').exists()).toBe(true)

    await wrapper.get('[data-testid="video-prompt"]').setValue('make a style-free video')
    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    await flushPromises()

    const payload = apiMocks.createVideoGeneration.mock.calls[0][0]
    expect(payload.prompt).toBe('make a style-free video')
    expect(payload).not.toHaveProperty('video_style_preset_id')
    expect(payload).not.toHaveProperty('style_preset')
  })

  it('keeps video style and reference panels on local theme variables', () => {
    expect(stylesSource).toContain('.workspace-with-sidebar.user-dark-shell .video-workspace-page')
    expect(stylesSource).toContain('.workspace-with-sidebar.user-light-shell .video-workspace-page')
    expect(stylesSource).toContain('--video-assist-panel-bg')
    expect(stylesSource).toContain('--video-assist-card-bg')
    expect(stylesSource).toContain('--video-assist-muted')
    expect(stylesSource).toContain('--video-assist-accent-bg')
    expect(stylesSource).toMatch(/\.video-style-library\s*\{[^}]*background:\s*var\(--video-assist-panel-bg/s)
    expect(stylesSource).toMatch(/\.video-reference-pool\s*\{[^}]*background:\s*var\(--video-assist-panel-bg/s)
    expect(stylesSource).toMatch(/\.video-style-card\s*\{[^}]*background:\s*var\(--video-assist-card-bg/s)
    expect(stylesSource).toMatch(/\.video-reference-dropzone\s*\{[^}]*background:\s*var\(--video-assist-card-bg/s)
    expect(stylesSource).not.toMatch(/\.video-style-preset-select\s*\{[^}]*background:\s*rgba\(255,\s*255,\s*255/s)
  })

  it('uses a soft cinema backdrop for contained result videos', () => {
    const frameRule = cssRule('.video-player-frame')
    const videoRule = cssRule('.video-player-frame video')

    expect(frameRule).toMatch(/background:\s*(?:[^;]*gradient[^;]*,\s*)+[^;]+;/s)
    expect(frameRule).toMatch(/border:\s*1px solid rgba\(/)
    expect(frameRule).toMatch(/box-shadow:\s*[^;]*inset[^;]*;/s)
    expect(frameRule).not.toMatch(/background:\s*#(?:000|10182d)\s*;/i)
    expect(videoRule).toMatch(/object-fit:\s*contain;/)
  })

  it('creates and selects a custom video style template and limits content references to three', async () => {
    apiMocks.uploadReferenceAsset
      .mockResolvedValueOnce(referenceAsset(77, 'style.png'))
      .mockResolvedValueOnce(referenceAsset(1))
      .mockResolvedValueOnce(referenceAsset(2))
      .mockResolvedValueOnce(referenceAsset(3))
      .mockResolvedValueOnce(referenceAsset(4))
    apiMocks.createVideoStyleTemplate.mockResolvedValueOnce(customStyle())
    mockCompletedVideo(102)

    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()

    await wrapper.get('[data-testid="video-style-tab-custom"]').trigger('click')
    await wrapper.get('[data-testid="video-style-create"]').trigger('click')
    await wrapper.get('[data-testid="video-style-template-title"]').setValue('Brand Film')
    await wrapper.get('[data-testid="video-style-template-description"]').setValue('warm brand film tone')
    await wrapper.get('[data-testid="video-style-template-prompt"]').setValue('warm brand film tone')
    const styleInput = wrapper.get('[data-testid="video-style-template-upload-input"]')
    Object.defineProperty(styleInput.element, 'files', { value: [fileNamed('style.png')], configurable: true })
    await styleInput.trigger('change')
    await flushPromises()
    await wrapper.get('[data-testid="video-style-template-save"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createVideoStyleTemplate).toHaveBeenCalledWith({
      title: 'Brand Film',
      description: 'warm brand film tone',
      reference_asset_id: 77,
      style_prompt: 'warm brand film tone'
    })
    expect(wrapper.get('[data-testid="video-style-template-9"]').classes()).toContain('is-selected')

    await uploadFiles(wrapper, [
      fileNamed('1.png'),
      fileNamed('2.png'),
      fileNamed('3.png'),
      fileNamed('4.png')
    ])

    const referencePool = wrapper.get('[data-testid="video-reference-pool"]')
    expect(referencePool.text()).toContain('(3/3)')
    expect(wrapper.findAll('[data-testid="video-reference-thumb"]')).toHaveLength(3)

    await wrapper.get('[data-testid="video-prompt"]').setValue('make a branded fashion video')
    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createVideoGeneration).toHaveBeenCalledWith(expect.objectContaining({
      reference_asset_ids: [1, 2, 3],
      custom_video_style_id: 9,
      style_preset: 'Brand Film'
    }))
    expect(apiMocks.createVideoGeneration.mock.calls[0][0]).not.toHaveProperty('video_style_preset_id')
  })

  it('submits a Sora video job and renders the completed video', async () => {
    apiMocks.createVideoGeneration.mockResolvedValueOnce({
      generation_id: 88,
      status: 'queued',
      available_credits: 20
    })
    apiMocks.getVideoGeneration.mockResolvedValueOnce({
      generation_id: 88,
      status: 'succeeded',
      work_id: 66,
      preview_url: '/api/works/66/file',
      download_url: '/api/works/66/download',
      mime_type: 'video/mp4',
      available_credits: 15
    })

    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()

    await wrapper.get('[data-testid="video-prompt"]').setValue('产品主图动起来')
    await wrapper.get('[data-testid="video-aspect-ratio"]').setValue('9:16')
    await chooseClickSelect(wrapper, 'video-duration', '15')
    await chooseClickSelect(wrapper, 'video-model', 'sora-2-pro')
    await wrapper.get('[data-testid="video-hd"]').setValue(true)
    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createVideoGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: '产品主图动起来',
      aspect_ratio: '9:16',
      duration: '15',
      model: 'sora-2-pro',
      hd: true,
      reference_asset_ids: [],
      video_style_preset_id: 1,
      style_preset: 'Cinematic Realism'
    }))
    expect(apiMocks.getVideoGeneration).toHaveBeenCalledWith(88)
    expect(wrapper.text()).toContain('视频生成完成')
    expect(wrapper.find('video').attributes('src')).toBe('/api/works/66/file')
    expect(wrapper.text()).toContain('剩余 15 点')
    expect(currentUser.value?.available_credits).toBe(15)
  })

  it('uses Grok Imagine by default and selects 3 seconds for Wuyin durations', async () => {
    mockCompletedVideo(92)

    const wrapper = mount(VideoWorkspaceView, {
      attachTo: document.body
    })
    await flushPromises()

    expect(wrapper.get('[data-testid="video-model"]').text()).toContain('Grok Imagine')
    expect(wrapper.get('[data-testid="video-duration"]').text()).toContain('3')

    await wrapper.get('[data-testid="video-duration"]').trigger('click')

    expect(clickSelectOption('video-duration', '1')).not.toBeNull()
    expect(clickSelectOption('video-duration', '3')).not.toBeNull()
    expect(clickSelectOption('video-duration', '6')).not.toBeNull()
    expect(clickSelectOption('video-duration', '10')).not.toBeNull()
    expect(clickSelectOption('video-duration', '15')).not.toBeNull()
    expect(clickSelectOption('video-duration', '25')).toBeNull()

    await wrapper.get('[data-testid="video-prompt"]').setValue('default Grok video')
    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createVideoGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: 'default Grok video',
      duration: '3',
      model: grokImagineVideoModel
    }))

    wrapper.unmount()
  })

  it('submits a manually selected Wuyin short duration', async () => {
    mockCompletedVideo(94)

    const wrapper = mount(VideoWorkspaceView, {
      attachTo: document.body
    })
    await flushPromises()

    await chooseClickSelect(wrapper, 'video-duration', '1')
    await wrapper.get('[data-testid="video-prompt"]').setValue('one second Grok video')
    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createVideoGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: 'one second Grok video',
      duration: '1',
      model: grokImagineVideoModel
    }))

    wrapper.unmount()
  })

  it('loads Doubao Seedance capabilities and submits the selected runtime model', async () => {
    apiMocks.listVideoModels.mockResolvedValueOnce({
      items: [
        {
          name: 'Grok Imagine',
          runtime_model: grokImagineVideoModel,
          provider: 'Wuyin',
          aspect_ratios: ['16:9', '9:16'],
          durations: ['1', '3', '6', '10', '15'],
          supports_hd: true,
          max_reference_images: 4
        },
        {
          name: 'Doubao Seedance 2.0 Mini',
          runtime_model: 'doubao-seed-2-0-mini-260428',
          provider: 'Volcengine Ark',
          aspect_ratios: ['16:9', '4:3', '1:1', '3:4', '9:16', '21:9', 'adaptive'],
          durations: ['4', '5', '6', '8', '10', '12', '15', '-1'],
          supports_hd: true,
          max_reference_images: 9
        }
      ]
    })
    mockCompletedVideo(93)

    const wrapper = mount(VideoWorkspaceView, {
      attachTo: document.body
    })
    await flushPromises()

    await chooseClickSelect(wrapper, 'video-model', 'doubao-seed-2-0-mini-260428')

    expect(wrapper.get('[data-testid="video-model"]').text()).toContain('Doubao Seedance 2.0 Mini')
    expect(wrapper.get('[data-testid="video-reference-pool"]').text()).toContain('(0/9)')

    await wrapper.get('[data-testid="video-duration"]').trigger('click')

    expect(clickSelectOption('video-duration', '12')).not.toBeNull()
    expect(clickSelectOption('video-duration', '-1')).not.toBeNull()
    expect(clickSelectOption('video-duration', '25')).toBeNull()

    clickSelectOption('video-duration', '12').click()
    await flushPromises()

    await wrapper.get('[data-testid="video-aspect-ratio"]').trigger('click')

    expect(clickSelectOption('video-aspect-ratio', '21:9')).not.toBeNull()
    expect(clickSelectOption('video-aspect-ratio', 'adaptive')).not.toBeNull()

    clickSelectOption('video-aspect-ratio', 'adaptive').click()
    await flushPromises()

    await wrapper.get('[data-testid="video-prompt"]').setValue('make this reference image move naturally')
    await wrapper.get('[data-testid="video-hd"]').setValue(true)
    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createVideoGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: 'make this reference image move naturally',
      aspect_ratio: 'adaptive',
      duration: '12',
      model: 'doubao-seed-2-0-mini-260428',
      hd: true
    }))

    wrapper.unmount()
  })

  it('selects the DS 2.0 model from API capabilities and submits resolution', async () => {
    apiMocks.listVideoModels.mockResolvedValueOnce({
      items: [
        {
          name: 'Grok Imagine',
          runtime_model: grokImagineVideoModel,
          provider: 'Wuyin',
          aspect_ratios: ['16:9', '9:16'],
          durations: ['1', '3', '6', '10', '15'],
          supports_hd: true,
          max_reference_images: 4
        },
        {
          name: 'DS 2.0',
          runtime_model: 'video-ds-2.0',
          provider: 'ZZ API',
          available: true,
          api_key_set: true,
          aspect_ratios: ['16:9', '9:16', '1:1'],
          durations: ['15'],
          resolution_options: ['480p', '720p'],
          default_resolution: '480p',
          price_rules: [
            { resolution: '480p', credits_per_second: 18 },
            { resolution: '720p', credits_per_second: 24 }
          ],
          supports_hd: true,
          max_reference_images: 4,
          supports_reference_video: true,
          supports_reference_audio: true,
          max_reference_videos: 3,
          max_reference_audios: 3
        }
      ]
    })
    apiMocks.uploadReferenceAsset
      .mockResolvedValueOnce(referenceAsset(21, 'motion.mp4', 'video'))
      .mockResolvedValueOnce(referenceAsset(31, 'voice.mp3', 'audio'))
    mockCompletedVideo(97)

    const wrapper = mount(VideoWorkspaceView, {
      attachTo: document.body
    })
    await flushPromises()

    await chooseClickSelect(wrapper, 'video-model', 'video-ds-2.0')
    await flushPromises()

    expect(wrapper.get('[data-testid="video-model"]').text()).toContain('DS 2.0')
    expect(wrapper.get('[data-testid="video-model"]').text()).not.toContain('Video DS 2.0')
    expect(wrapper.get('[data-testid="video-duration"]').text()).toContain('15')
    expect(wrapper.get('[data-testid="video-resolution"]').text()).toContain('480p')
    expect(wrapper.find('[data-testid="video-reference-video-pool"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="video-reference-audio-pool"]').exists()).toBe(true)

    await wrapper.get('[data-testid="video-aspect-ratio"]').trigger('click')
    expect(clickSelectOption('video-aspect-ratio', '1:1')).not.toBeNull()
    clickSelectOption('video-aspect-ratio', '1:1').click()
    await flushPromises()

    await uploadFilesByTestId(wrapper, 'video-reference-video-upload-input', [mediaFileNamed('motion.mp4', 'video/mp4')])
    await uploadFilesByTestId(wrapper, 'video-reference-audio-upload-input', [mediaFileNamed('voice.mp3', 'audio/mpeg')])

    await wrapper.get('[data-testid="video-prompt"]').setValue('make a ZZ model video')
    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createVideoGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: 'make a ZZ model video',
      aspect_ratio: '1:1',
      duration: '15',
      model: 'video-ds-2.0',
      resolution: '480p',
      reference_video_asset_ids: [21],
      reference_audio_asset_ids: [31],
      generate_audio: true
    }))

    wrapper.unmount()
  })

  it('estimates DS 2.0 by default 480p and refreshes after switching to 720p', async () => {
    vi.useFakeTimers()
    apiMocks.getMe.mockResolvedValueOnce({ user_id: 12, available_credits: 500 })
    apiMocks.estimateVideoGeneration
      .mockResolvedValueOnce({
        required_credits: 270,
        available_credits: 500,
        missing_credits: 0,
        enough: true,
        billing_policy: 'success_only'
      })
      .mockResolvedValueOnce({
        required_credits: 360,
        available_credits: 500,
        missing_credits: 0,
        enough: true,
        billing_policy: 'success_only'
      })
    apiMocks.listVideoModels.mockResolvedValueOnce({
      items: [
        {
          name: 'DS 2.0',
          runtime_model: 'video-ds-2.0',
          provider: 'ZZ API',
          available: true,
          api_key_set: true,
          aspect_ratios: ['16:9', '9:16', '1:1'],
          durations: ['15'],
          resolution_options: ['480p', '720p'],
          default_resolution: '480p',
          price_rules: [
            { resolution: '480p', credits_per_second: 18 },
            { resolution: '720p', credits_per_second: 24 }
          ],
          supports_hd: true,
          max_reference_images: 4,
          supports_reference_video: true,
          supports_reference_audio: true,
          max_reference_videos: 3,
          max_reference_audios: 3
        }
      ]
    })

    const wrapper = mount(VideoWorkspaceView, {
      attachTo: document.body
    })
    await flushPromises()

    expect(wrapper.get('[data-testid="video-model"]').text()).toContain('DS 2.0')
    expect(wrapper.get('[data-testid="video-model"]').text()).not.toContain('Video DS 2.0')
    expect(wrapper.get('[data-testid="video-duration"]').text()).toContain('15')
    expect(wrapper.get('[data-testid="video-resolution"]').text()).toContain('480p')

    await wrapper.get('[data-testid="video-prompt"]').setValue('bill Video DS by selected resolution')
    await vi.advanceTimersByTimeAsync(350)
    await flushPromises()

    expect(apiMocks.estimateVideoGeneration).toHaveBeenLastCalledWith(expect.objectContaining({
      model: 'video-ds-2.0',
      duration: '15',
      resolution: '480p'
    }), expect.any(Object))
    expect(wrapper.get('[data-testid="video-credit-estimate"]').text()).toContain('270')
    expect(wrapper.get('[data-testid="video-submit"]').text()).toContain('270')

    await chooseClickSelect(wrapper, 'video-resolution', '720p')
    await vi.advanceTimersByTimeAsync(350)
    await flushPromises()

    expect(apiMocks.estimateVideoGeneration).toHaveBeenLastCalledWith(expect.objectContaining({
      model: 'video-ds-2.0',
      duration: '15',
      resolution: '720p'
    }), expect.any(Object))
    expect(wrapper.get('[data-testid="video-credit-estimate"]').text()).toContain('360')
    expect(wrapper.get('[data-testid="video-submit"]').text()).toContain('360')

    wrapper.unmount()
  })

  it('uses model resolution options to request backend estimates and submit resolution', async () => {
    vi.useFakeTimers()
    apiMocks.getMe.mockResolvedValueOnce({ user_id: 12, available_credits: 300 })
    apiMocks.estimateVideoGeneration
      .mockResolvedValueOnce({
        required_credits: 120,
        available_credits: 300,
        missing_credits: 0,
        enough: true,
        billing_policy: 'success_only'
      })
      .mockResolvedValueOnce({
        required_credits: 180,
        available_credits: 300,
        missing_credits: 0,
        enough: true,
        billing_policy: 'success_only'
      })
    apiMocks.listVideoModels.mockResolvedValueOnce({
      items: [
        {
          name: 'Doubao Seedance 2.0 Mini',
          runtime_model: 'doubao-seed-2-0-mini-260428',
          provider: 'Volcengine Ark',
          aspect_ratios: ['16:9', '9:16'],
          durations: ['4', '12'],
          supports_hd: true,
          max_reference_images: 9,
          resolution_options: ['480p', '720p'],
          default_resolution: '480p',
          price_rules: [
            { resolution: '480p', credits_per_second: 10 },
            { resolution: '720p', credits_per_second: 15 }
          ]
        },
        {
          name: 'Doubao Seedance 2.0',
          runtime_model: 'doubao-seedance-2-0-260128',
          provider: 'Volcengine Ark',
          aspect_ratios: ['16:9', '9:16'],
          durations: ['10'],
          supports_hd: true,
          max_reference_images: 9,
          resolution_options: ['720p', '1080p'],
          default_resolution: '720p',
          price_rules: [
            { resolution: '720p', credits_per_second: 30 },
            { resolution: '1080p', credits_per_second: 50 }
          ]
        }
      ]
    })
    mockCompletedVideo(96)

    const wrapper = mount(VideoWorkspaceView, {
      attachTo: document.body
    })
    await flushPromises()

    await chooseClickSelect(wrapper, 'video-model', 'doubao-seed-2-0-mini-260428')
    await chooseClickSelect(wrapper, 'video-duration', '12')

    expect(wrapper.get('[data-testid="video-resolution"]').text()).toContain('480p')
    await wrapper.get('[data-testid="video-prompt"]').setValue('bill seedance by resolution seconds')
    await vi.advanceTimersByTimeAsync(350)
    await flushPromises()

    expect(apiMocks.estimateVideoGeneration).toHaveBeenLastCalledWith(expect.objectContaining({
      model: 'doubao-seed-2-0-mini-260428',
      duration: '12',
      resolution: '480p'
    }), expect.any(Object))
    expect(wrapper.get('[data-testid="video-credit-estimate"]').text()).toContain('预计消耗 120 点')

    await chooseClickSelect(wrapper, 'video-resolution', '720p')
    await vi.advanceTimersByTimeAsync(350)
    await flushPromises()

    expect(apiMocks.estimateVideoGeneration).toHaveBeenLastCalledWith(expect.objectContaining({
      model: 'doubao-seed-2-0-mini-260428',
      duration: '12',
      resolution: '720p'
    }), expect.any(Object))
    expect(wrapper.get('[data-testid="video-credit-estimate"]').text()).toContain('预计消耗 180 点')

    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createVideoGeneration).toHaveBeenCalledWith(expect.objectContaining({
      model: 'doubao-seed-2-0-mini-260428',
      duration: '12',
      resolution: '720p'
    }))

    wrapper.unmount()
  })

  it('uploads Seedance 2.0 image video and audio references into estimate and submit payloads', async () => {
    vi.useFakeTimers()
    apiMocks.getMe.mockResolvedValueOnce({ user_id: 12, available_credits: 1000 })
    apiMocks.listVideoModels.mockResolvedValueOnce({
      items: [
        {
          name: 'Doubao Seedance 2.0',
          runtime_model: 'doubao-seedance-2-0-260128',
          provider: 'Volcengine Ark',
          available: true,
          aspect_ratios: ['16:9', '9:16'],
          durations: ['4', '5', '6', '7', '8', '9', '10', '11', '12', '13', '14', '15', '-1'],
          supports_hd: true,
          max_reference_images: 9,
          supports_reference_video: true,
          supports_reference_audio: true,
          max_reference_videos: 3,
          max_reference_audios: 3,
          supports_generate_audio: true,
          resolution_options: ['720p', '1080p'],
          default_resolution: '720p',
          price_rules: [
            { resolution: '720p', credits_per_second: 30 },
            { resolution: '1080p', credits_per_second: 50 }
          ]
        }
      ]
    })
    apiMocks.listReferenceAssets.mockImplementation(({ kind } = {}) => {
      if (kind === 'video') return Promise.resolve({ items: [] })
      if (kind === 'audio') return Promise.resolve({ items: [] })
      return Promise.resolve({ items: [] })
    })
    apiMocks.uploadReferenceAsset
      .mockResolvedValueOnce(referenceAsset(11, 'image.png', 'image'))
      .mockResolvedValueOnce(referenceAsset(21, 'clip.mp4', 'video'))
      .mockResolvedValueOnce(referenceAsset(31, 'music.mp3', 'audio'))
    apiMocks.estimateVideoGeneration.mockResolvedValue({
      required_credits: 550,
      available_credits: 1000,
      missing_credits: 0,
      enough: true,
      billing_policy: 'success_only'
    })
    mockCompletedVideo(146)

    const wrapper = mount(VideoWorkspaceView, {
      attachTo: document.body
    })
    await flushPromises()

    await chooseClickSelect(wrapper, 'video-duration', '11')
    await chooseClickSelect(wrapper, 'video-resolution', '1080p')
    await uploadFiles(wrapper, [mediaFileNamed('image.png', 'image/png')])
    await uploadFilesByTestId(wrapper, 'video-reference-video-upload-input', [mediaFileNamed('clip.mp4', 'video/mp4')])
    await uploadFilesByTestId(wrapper, 'video-reference-audio-upload-input', [mediaFileNamed('music.mp3', 'audio/mpeg')])
    await wrapper.get('[data-testid="video-prompt"]').setValue('make a multimodal Seedance 2.0 video')
    await vi.advanceTimersByTimeAsync(350)
    await flushPromises()

    expect(apiMocks.listReferenceAssets).toHaveBeenCalledWith({ kind: 'image' })
    expect(apiMocks.listReferenceAssets).toHaveBeenCalledWith({ kind: 'video' })
    expect(apiMocks.listReferenceAssets).toHaveBeenCalledWith({ kind: 'audio' })
    expect(apiMocks.estimateVideoGeneration).toHaveBeenLastCalledWith(expect.objectContaining({
      model: 'doubao-seedance-2-0-260128',
      duration: '11',
      resolution: '1080p',
      reference_asset_ids: [11],
      reference_video_asset_ids: [21],
      reference_audio_asset_ids: [31],
      generate_audio: true
    }), expect.any(Object))

    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createVideoGeneration).toHaveBeenCalledWith(expect.objectContaining({
      model: 'doubao-seedance-2-0-260128',
      reference_asset_ids: [11],
      reference_video_asset_ids: [21],
      reference_audio_asset_ids: [31],
      generate_audio: true
    }))

    wrapper.unmount()
  })

  it('hides internal or unavailable video models from the workspace selector', async () => {
    apiMocks.listVideoModels.mockResolvedValueOnce({
      items: [
        {
          name: 'Grok Imagine',
          runtime_model: grokImagineVideoModel,
          provider: 'Wuyin',
          permission: 'public',
          available: true,
          aspect_ratios: ['16:9', '9:16'],
          durations: ['1', '3', '6', '10', '15'],
          supports_hd: true,
          max_reference_images: 4
        },
        {
          name: 'Doubao Seedance 2.0 Mini',
          runtime_model: 'doubao-seed-2-0-mini-260428',
          provider: 'Volcengine Ark',
          permission: 'internal',
          available: false,
          disabled_reason: '\u5185\u6d4b\u4e2d\uff0c\u9700\u914d\u7f6e\u706b\u5c71\u65b9\u821f\u5bc6\u94a5\u540e\u516c\u5f00',
          api_key_set: false,
          aspect_ratios: ['16:9', '4:3', '1:1', '3:4', '9:16', '21:9', 'adaptive'],
          durations: ['4', '5', '6', '8', '10', '12', '15', '-1'],
          supports_hd: true,
          max_reference_images: 9
        },
        {
          name: 'Doubao Seedance 2.0',
          runtime_model: 'doubao-seedance-2-0-260128',
          provider: 'Volcengine Ark',
          permission: 'public',
          available: false,
          disabled_reason: '当前模型不支持视频生成 API，请更换 Ark 视频模型或等待开通',
          api_key_set: true,
          aspect_ratios: ['16:9', '9:16'],
          durations: ['10'],
          supports_hd: true,
          max_reference_images: 9
        }
      ]
    })

    const wrapper = mount(VideoWorkspaceView, {
      attachTo: document.body
    })
    await flushPromises()

    expect(wrapper.get('[data-testid="video-model"]').text()).toContain('Grok Imagine')
    expect(wrapper.find('[data-testid="video-model-readiness"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('\u706b\u5c71\u65b9\u821f\u5bc6\u94a5')

    await wrapper.get('[data-testid="video-model"]').trigger('click')
    await flushPromises()

    expect(clickSelectOption('video-model', 'doubao-seed-2-0-mini-260428')).toBeNull()
    expect(clickSelectOption('video-model', 'doubao-seedance-2-0-260128')).toBeNull()
    expect(clickSelectMenu('video-model').textContent).not.toContain('\u5185\u6d4b')
    expect(clickSelectMenu('video-model').textContent).not.toContain('\u4e0d\u53ef\u7528')

    wrapper.unmount()
  })

  it('falls back to the first visible model when the API also returns unavailable models', async () => {
    mockCompletedVideo(145)
    apiMocks.listVideoModels.mockResolvedValueOnce({
      items: [
        {
          name: 'Doubao Seedance 2.0 Mini',
          runtime_model: 'doubao-seed-2-0-mini-260428',
          provider: 'Volcengine Ark',
          permission: 'public',
          available: false,
          disabled_reason: '当前模型不支持视频生成 API，请更换 Ark 视频模型或等待开通',
          api_key_set: true,
          aspect_ratios: ['16:9', '4:3', '1:1', '3:4', '9:16', '21:9', 'adaptive'],
          durations: ['4', '5', '6', '8', '10', '12', '15', '-1'],
          supports_hd: true,
          max_reference_images: 9
        },
        {
          name: 'Grok Imagine',
          runtime_model: grokImagineVideoModel,
          provider: 'Wuyin',
          available: true,
          aspect_ratios: ['16:9', '9:16'],
          durations: ['6', '10', '15'],
          supports_hd: true,
          max_reference_images: 4
        }
      ]
    })

    const wrapper = mount(VideoWorkspaceView, {
      attachTo: document.body
    })
    await flushPromises()

    expect(wrapper.get('[data-testid="video-model"]').text()).toContain('Grok Imagine')
    expect(wrapper.find('[data-testid="video-model-readiness"]').exists()).toBe(false)

    await wrapper.get('[data-testid="video-model"]').trigger('click')
    await flushPromises()

    expect(clickSelectOption('video-model', 'doubao-seed-2-0-mini-260428')).toBeNull()
    expect(clickSelectMenu('video-model').textContent).not.toContain('Seedance')
    expect(clickSelectMenu('video-model').textContent).not.toContain('不可用')
    expect(clickSelectMenu('video-model').textContent).not.toContain('不支持视频生成 API')

    await wrapper.get('[data-testid="video-prompt"]').setValue('try unsupported seedance')
    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createVideoGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: 'try unsupported seedance',
      model: grokImagineVideoModel
    }))

    wrapper.unmount()
  })

  it('keeps empty or overlong prompts from creating video jobs', async () => {
    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()

    expect(wrapper.get('[data-testid="video-submit"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    expect(apiMocks.createVideoGeneration).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="video-prompt"]').setValue('镜头'.repeat(401))
    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createVideoGeneration).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('提示词不能超过 800 字')
  })

  it('estimates video credits on the backend and shows success-only billing copy', async () => {
    vi.useFakeTimers()
    apiMocks.getMe.mockResolvedValueOnce({ user_id: 12, available_credits: 40 })
    apiMocks.listVideoModels.mockResolvedValueOnce({
      items: [
        {
          name: 'Sora2',
          runtime_model: 'sora-2',
          provider: 'GPT-Best',
          aspect_ratios: ['16:9', '9:16'],
          durations: ['10', '15', '25'],
          default_duration: '10',
          supports_hd: true,
          max_reference_images: 4,
          requires_reference_image: false
        }
      ]
    })
    apiMocks.estimateVideoGeneration.mockResolvedValue({
      required_credits: 30,
      available_credits: 40,
      missing_credits: 0,
      enough: true,
      billing_policy: 'success_only',
      message: '提交前预估，生成成功后扣点，失败不扣点'
    })

    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()

    await wrapper.get('[data-testid="video-prompt"]').setValue('cinematic product video')
    await vi.advanceTimersByTimeAsync(350)
    await flushPromises()

    expect(apiMocks.estimateVideoGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: 'cinematic product video',
      aspect_ratio: '16:9',
      duration: '10',
      model: 'sora-2',
      hd: false,
      reference_asset_ids: []
    }), expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.get('[data-testid="video-credit-estimate"]').text()).toBe('预计消耗 30 点 · 当前 40 点 · 生成成功后扣除，失败不扣点')
    expect(wrapper.get('[data-testid="video-submit"]').text()).toBe('生成视频 · 预计 30 点')
  })

  it('disables video submit and links to pricing when the estimate is short on credits', async () => {
    vi.useFakeTimers()
    apiMocks.estimateVideoGeneration.mockResolvedValueOnce({
      required_credits: 18,
      available_credits: 10,
      missing_credits: 8,
      enough: false,
      recommended_package: { id: 1, name: '灵感包', credits: 20 },
      billing_policy: 'success_only',
      message: '提交前预估，生成成功后扣点，失败不扣点'
    })

    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()

    await wrapper.get('[data-testid="video-prompt"]').setValue('grok video with low balance')
    await vi.advanceTimersByTimeAsync(350)
    await flushPromises()

    expect(wrapper.get('[data-testid="video-credit-estimate"]').text()).toContain('点数不足，还差 8 点')
    expect(wrapper.get('[data-testid="video-submit"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="video-recharge-link"]').attributes('href')).toBe('/pricing?source=video_generation&missing_credits=8&required_credits=18&package_id=1')
  })

  it('aborts stale video credit estimates and ignores older responses', async () => {
    vi.useFakeTimers()
    const firstEstimate = deferred()
    const secondEstimate = deferred()
    apiMocks.estimateVideoGeneration
      .mockReturnValueOnce(firstEstimate.promise)
      .mockReturnValueOnce(secondEstimate.promise)

    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()

    await wrapper.get('[data-testid="video-prompt"]').setValue('first video prompt')
    await vi.advanceTimersByTimeAsync(350)
    await flushPromises()
    const firstSignal = apiMocks.estimateVideoGeneration.mock.calls[0][1].signal

    await wrapper.get('[data-testid="video-prompt"]').setValue('newer video prompt')
    await vi.advanceTimersByTimeAsync(350)
    await flushPromises()

    expect(firstSignal.aborted).toBe(true)
    secondEstimate.resolve({
      required_credits: 12,
      available_credits: 20,
      missing_credits: 0,
      enough: true,
      billing_policy: 'success_only'
    })
    await flushPromises()
    firstEstimate.resolve({
      required_credits: 3,
      available_credits: 20,
      missing_credits: 0,
      enough: true,
      billing_policy: 'success_only'
    })
    await flushPromises()

    expect(wrapper.get('[data-testid="video-credit-estimate"]').text()).toContain('预计消耗 12 点')
    expect(wrapper.get('[data-testid="video-credit-estimate"]').text()).not.toContain('预计消耗 3 点')
  })

  it('blocks Grok text-to-video before submit when the model requires a reference image', async () => {
    apiMocks.listVideoModels.mockResolvedValueOnce({
      items: [
        {
          name: 'Grok Imagine',
          runtime_model: grokImagineVideoModel,
          provider: 'Wuyin',
          aspect_ratios: ['16:9', '9:16'],
          durations: ['1', '3', '6'],
          supports_hd: true,
          max_reference_images: 4,
          requires_reference_image: true
        },
        {
          name: 'Sora2',
          runtime_model: 'sora-2',
          provider: 'GPT-Best',
          aspect_ratios: ['16:9', '9:16'],
          durations: ['10', '15', '25'],
          default_duration: '10',
          supports_hd: true,
          max_reference_images: 4,
          requires_reference_image: false
        }
      ]
    })
    apiMocks.uploadReferenceAsset.mockResolvedValueOnce(referenceAsset(42, 'reference.png'))
    mockCompletedVideo(104)

    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()

    await wrapper.get('[data-testid="video-prompt"]').setValue('text only grok video')

    expect(wrapper.get('[data-testid="video-submit"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="video-reference-pool"]').text()).toContain(referenceImageRequiredMessage)
    expect(wrapper.get('[data-testid="video-preview-stage"]').text()).not.toContain(referenceImageRequiredMessage)
    expect(wrapper.find('.video-result-panel .status-error').exists()).toBe(false)

    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createVideoGeneration).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="video-preview-stage"]').text()).not.toContain(referenceImageRequiredMessage)
    expect(wrapper.find('.video-result-panel .status-error').exists()).toBe(false)

    await uploadFiles(wrapper, [fileNamed('reference.png')])

    expect(wrapper.get('[data-testid="video-submit"]').attributes('disabled')).toBeUndefined()
    expect(wrapper.get('[data-testid="video-reference-pool"]').text()).not.toContain(referenceImageRequiredMessage)

    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createVideoGeneration).toHaveBeenCalledWith(expect.objectContaining({
      model: grokImagineVideoModel,
      reference_asset_ids: [42]
    }))
  })

  it('allows Sora text-to-video when the selected model does not require a reference image', async () => {
    apiMocks.listVideoModels.mockResolvedValueOnce({
      items: [
        {
          name: 'Grok Imagine',
          runtime_model: grokImagineVideoModel,
          provider: 'Wuyin',
          aspect_ratios: ['16:9', '9:16'],
          durations: ['1', '3', '6'],
          supports_hd: true,
          max_reference_images: 4,
          requires_reference_image: true
        },
        {
          name: 'Sora2',
          runtime_model: 'sora-2',
          provider: 'GPT-Best',
          aspect_ratios: ['16:9', '9:16'],
          durations: ['10', '15', '25'],
          supports_hd: true,
          max_reference_images: 4,
          requires_reference_image: false
        }
      ]
    })
    mockCompletedVideo(105)

    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()

    await chooseClickSelect(wrapper, 'video-model', 'sora-2')
    await wrapper.get('[data-testid="video-prompt"]').setValue('text only sora video')

    expect(wrapper.get('[data-testid="video-reference-pool"]').text()).not.toContain(referenceImageRequiredMessage)
    expect(wrapper.get('[data-testid="video-submit"]').attributes('disabled')).toBeUndefined()

    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createVideoGeneration).toHaveBeenCalledWith(expect.objectContaining({
      model: 'sora-2',
      reference_asset_ids: []
    }))
  })

  it('passes duration quality and model choices through the video payload', async () => {
    apiMocks.createVideoGeneration.mockResolvedValueOnce({
      generation_id: 89,
      status: 'queued',
      available_credits: 12
    })
    apiMocks.getVideoGeneration.mockResolvedValueOnce({
      generation_id: 89,
      status: 'failed',
      error: { message: '生成排队超时' },
      available_credits: 12
    })

    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()

    await wrapper.get('[data-testid="video-prompt"]').setValue('电影感街头慢动作')
    await chooseClickSelect(wrapper, 'video-model', 'sora-2-pro')
    await chooseClickSelect(wrapper, 'video-duration', '25')
    await wrapper.get('[data-testid="video-hd"]').setValue(true)
    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createVideoGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: '电影感街头慢动作',
      duration: '25',
      model: 'sora-2-pro',
      hd: true
    }))
    expect(wrapper.text()).toContain('生成排队超时')
    expect(wrapper.find('video').exists()).toBe(false)
  })

  it('keeps the upload slot visible after uploading the first reference image', async () => {
    apiMocks.uploadReferenceAsset.mockResolvedValueOnce(referenceAsset(1, 'first.png'))

    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()

    await uploadFiles(wrapper, [fileNamed('first.png')])

    const referencePool = wrapper.get('[data-testid="video-reference-pool"]')
    expect(apiMocks.uploadReferenceAsset).toHaveBeenCalledTimes(1)
    expect(wrapper.findAll('[data-testid="video-reference-thumb"]')).toHaveLength(1)
    expect(wrapper.get('[data-testid="video-reference-dropzone"]').attributes('disabled')).toBeUndefined()
    expect(referencePool.text()).toContain('(1/4)')
    expect(referencePool.text()).toContain('支持 jpg/png，点击可预览，悬停可替换')
  })

  it('uploads multiple reference images in order and submits at most four ids', async () => {
    apiMocks.uploadReferenceAsset
      .mockResolvedValueOnce(referenceAsset(1))
      .mockResolvedValueOnce(referenceAsset(2))
      .mockResolvedValueOnce(referenceAsset(3))
      .mockResolvedValueOnce(referenceAsset(4))
      .mockResolvedValueOnce(referenceAsset(5))
    mockCompletedVideo()

    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()

    await uploadFiles(wrapper, [
      fileNamed('1.png'),
      fileNamed('2.png'),
      fileNamed('3.png'),
      fileNamed('4.png'),
      fileNamed('5.png')
    ])

    expect(apiMocks.uploadReferenceAsset).toHaveBeenCalledTimes(4)
    expect(wrapper.findAll('[data-testid="video-reference-thumb"] img').map((image) => image.attributes('src'))).toEqual([
      '/assets/1.jpg',
      '/assets/2.jpg',
      '/assets/3.jpg',
      '/assets/4.jpg'
    ])

    await wrapper.get('[data-testid="video-prompt"]').setValue('从多张产品图生成开箱短片')
    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createVideoGeneration).toHaveBeenCalledWith(expect.objectContaining({
      reference_asset_ids: [1, 2, 3, 4]
    }))
  })

  it('removes a thumbnail only from the current video references', async () => {
    apiMocks.uploadReferenceAsset
      .mockResolvedValueOnce(referenceAsset(1))
      .mockResolvedValueOnce(referenceAsset(2))
    mockCompletedVideo()

    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()

    await uploadFiles(wrapper, [fileNamed('1.png'), fileNamed('2.png')])
    await wrapper.findAll('[data-testid="video-reference-delete"]')[0].trigger('click')

    const referencePool = wrapper.get('[data-testid="video-reference-pool"]')
    expect(referencePool.text()).toContain('(1/4)')
    expect(apiMocks.deleteReferenceAsset).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="video-prompt"]').setValue('只保留第二张参考图')
    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createVideoGeneration).toHaveBeenCalledWith(expect.objectContaining({
      reference_asset_ids: [2]
    }))
  })

  it('replaces a thumbnail with a newly uploaded asset at the same position', async () => {
    apiMocks.uploadReferenceAsset
      .mockResolvedValueOnce(referenceAsset(1))
      .mockResolvedValueOnce(referenceAsset(2))
      .mockResolvedValueOnce(referenceAsset(99, 'replacement.png'))
    mockCompletedVideo()

    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()

    await uploadFiles(wrapper, [fileNamed('1.png'), fileNamed('2.png')])
    await replaceFile(wrapper, 1, fileNamed('replacement.png'))

    expect(wrapper.findAll('[data-testid="video-reference-thumb"] img').map((image) => image.attributes('src'))).toEqual([
      '/assets/1.jpg',
      '/assets/99.jpg'
    ])

    await wrapper.get('[data-testid="video-prompt"]').setValue('替换第二张参考图后生成')
    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createVideoGeneration).toHaveBeenCalledWith(expect.objectContaining({
      reference_asset_ids: [1, 99]
    }))
    expect(apiMocks.createVideoGeneration.mock.calls[0][0].reference_asset_ids).not.toContain(2)
  })

  it('opens a preview from a thumbnail and exposes replace and delete controls on focus', async () => {
    apiMocks.uploadReferenceAsset.mockResolvedValueOnce(referenceAsset(1, 'preview.png'))

    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()

    await uploadFiles(wrapper, [fileNamed('preview.png')])

    const thumb = wrapper.get('[data-testid="video-reference-thumb"]')
    await thumb.trigger('focusin')

    expect(thumb.get('[data-testid="video-reference-delete"]').attributes('aria-label')).toContain('移除')
    expect(thumb.get('[data-testid="video-reference-replace"]').attributes('aria-label')).toContain('替换')

    await wrapper.get('[data-testid="video-reference-preview-button"]').trigger('click')
    await flushPromises()

    const modal = document.body.querySelector('[data-testid="video-reference-preview-modal"]')
    expect(modal?.querySelector('img')?.getAttribute('src')).toBe('/assets/1.jpg')
    expect(modal?.textContent).toContain('preview.png')
  })

  it('keeps the upload slot visible but disabled after four images are selected', async () => {
    apiMocks.uploadReferenceAsset
      .mockResolvedValueOnce(referenceAsset(1))
      .mockResolvedValueOnce(referenceAsset(2))
      .mockResolvedValueOnce(referenceAsset(3))
      .mockResolvedValueOnce(referenceAsset(4))

    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()

    await uploadFiles(wrapper, [fileNamed('1.png'), fileNamed('2.png'), fileNamed('3.png'), fileNamed('4.png')])

    const uploadButton = wrapper.get('[data-testid="video-reference-dropzone"]')
    expect(uploadButton.exists()).toBe(true)
    expect(uploadButton.attributes('disabled')).toBeDefined()
    expect(uploadButton.text()).toContain('(4/4)')
  })

  it('keeps task status and result actions inside the result panel', async () => {
    apiMocks.createVideoGeneration.mockResolvedValueOnce({
      generation_id: 92,
      status: 'queued',
      available_credits: 19
    })
    apiMocks.getVideoGeneration.mockResolvedValueOnce({
      generation_id: 92,
      status: 'succeeded',
      work_id: 67,
      preview_url: '/api/works/67/file',
      download_url: '/api/works/67/download',
      available_credits: 18
    })

    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()

    expect(wrapper.get('[data-testid="video-preview-stage"]').text()).toContain('等待视频任务')

    await wrapper.get('[data-testid="video-prompt"]').setValue('生成一个城市延时视频')
    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    await flushPromises()

    const resultPanel = wrapper.get('[data-testid="video-result-panel"]')
    expect(resultPanel.text()).toContain('视频生成完成')
    expect(resultPanel.get('[data-testid="video-task-status"]').text()).toContain('已完成')
    expect(resultPanel.find('video').attributes('src')).toBe('/api/works/67/file')
    expect(resultPanel.find('a[href="/api/works/67/download"]').exists()).toBe(true)
    expect(resultPanel.find('a[href="/works?category=video"]').exists()).toBe(true)
  })

  it('loads video generation history and switches preview when selecting a history item', async () => {
    apiMocks.listUserVideoGenerations.mockResolvedValueOnce({
      items: [
        videoHistoryItem(),
        videoHistoryItem({
          id: 2,
          generation_id: 102,
          work_id: 902,
          prompt: '赛博城市夜景视频',
          prompt_summary: '赛博城市夜景视频',
          preview_url: '/api/works/902/file',
          download_url: '/api/works/902/download',
          aspect_ratio: '9:16',
          duration_seconds: 6,
          runtime_model: grokImagineVideoModel,
          model_name: 'Grok Imagine',
          enhancement_tags: ['补帧', '精修'],
          reference_asset_ids: [],
          reference_asset_count: 0,
          hd: false
        })
      ],
      total: 2,
      page: 1,
      page_size: 8
    })

    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()

    expect(apiMocks.listUserVideoGenerations).toHaveBeenCalledWith({ page: 1, page_size: 8 })
    const historyPanel = wrapper.get('[data-testid="video-history-panel"]')
    expect(historyPanel.text()).toContain('历史任务')
    expect(historyPanel.text()).toContain('高清')
    expect(historyPanel.text()).toContain('参考图')
    expect(historyPanel.text()).toContain('风格模板')
    expect(historyPanel.text()).toContain('+1')

    await wrapper.get('[data-testid="video-history-card-102"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="video-preview-stage"] video').attributes('src')).toBe('/api/works/902/file')
    expect(wrapper.get('[data-testid="video-history-card-102"]').classes()).toContain('is-selected')
  })

  it('shows an empty history state when the user has no video records', async () => {
    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()

    expect(wrapper.get('[data-testid="video-history-panel"]').text()).toContain('暂无视频历史')
  })

  it('applies history filters and surfaces failed history details', async () => {
    apiMocks.listUserVideoGenerations
      .mockResolvedValueOnce({ items: [], total: 0, page: 1, page_size: 8 })
      .mockResolvedValueOnce({
        items: [
          videoHistoryItem({
            id: 3,
            generation_id: 103,
            work_id: null,
            status: 'failed',
            prompt: '故障排查视频 精修',
            prompt_summary: '故障排查视频 精修',
            preview_url: '',
            download_url: '',
            error_message: '模型返回空视频，请调整提示词后重试。',
            enhancement_tags: ['精修']
          })
        ],
        total: 1,
        page: 1,
        page_size: 8
      })
      .mockResolvedValue({
        items: [
          videoHistoryItem({
            id: 3,
            generation_id: 103,
            work_id: null,
            status: 'failed',
            prompt: '故障排查视频 精修',
            prompt_summary: '故障排查视频 精修',
            preview_url: '',
            download_url: '',
            error_message: '模型返回空视频，请调整提示词后重试。',
            enhancement_tags: ['精修']
          })
        ],
        total: 1,
        page: 1,
        page_size: 8
      })

    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()

    await wrapper.get('[data-testid="video-history-search"]').setValue('故障排查')
    await wrapper.get('[data-testid="video-history-status-filter"]').setValue('failed')
    await wrapper.get('[data-testid="video-history-enhancement-filter"]').setValue('精修')
    await flushPromises()

    expect(apiMocks.listUserVideoGenerations).toHaveBeenLastCalledWith({
      page: 1,
      page_size: 8,
      q: '故障排查',
      status: 'failed',
      enhancement: '精修'
    })
    expect(wrapper.get('[data-testid="video-history-card-103"]').text()).toContain('失败')
    expect(wrapper.get('[data-testid="video-history-card-103"]').text()).toContain('精修')
    expect(wrapper.get('[data-testid="video-preview-stage"]').text()).toContain('模型返回空视频，请调整提示词后重试。')
  })

  it('localizes known provider reference-image errors from history records', async () => {
    const englishError = 'This model requires an input image. Text-to-video is not supported for this model.'
    apiMocks.listUserVideoGenerations.mockResolvedValueOnce({
      items: [
        videoHistoryItem({
          id: 4,
          generation_id: 104,
          work_id: null,
          status: 'failed',
          preview_url: '',
          download_url: '',
          error_message: englishError
        })
      ],
      total: 1,
      page: 1,
      page_size: 8
    })

    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()

    const previewText = wrapper.get('[data-testid="video-preview-stage"]').text()
    expect(previewText).toContain(referenceImageRequiredMessage)
    expect(previewText).not.toContain('requires an input image')
    expect(previewText).not.toContain('Text-to-video is not supported')
  })

  it('localizes known provider reference-image errors from polling failures and API exceptions', async () => {
    const englishError = 'This model requires an input image. Text-to-video is not supported for this model.'
    apiMocks.createVideoGeneration
      .mockResolvedValueOnce({
        generation_id: 106,
        status: 'queued',
        available_credits: 19
      })
      .mockRejectedValueOnce(new Error(englishError))
    apiMocks.getVideoGeneration.mockResolvedValueOnce({
      generation_id: 106,
      status: 'failed',
      error: { message: englishError },
      available_credits: 19
    })

    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()

    await chooseClickSelect(wrapper, 'video-model', 'sora-2')
    await wrapper.get('[data-testid="video-prompt"]').setValue('polling returns provider english')
    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain(referenceImageRequiredMessage)
    expect(wrapper.text()).not.toContain('requires an input image')

    await wrapper.get('[data-testid="video-prompt"]').setValue('api throws provider english')
    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain(referenceImageRequiredMessage)
    expect(wrapper.text()).not.toContain('Text-to-video is not supported')
  })

  it('refills the composer from a history item for edit-before-regenerate', async () => {
    apiMocks.listReferenceAssets.mockResolvedValueOnce({ items: [referenceAsset(2), referenceAsset(3)] })
    apiMocks.listUserVideoGenerations.mockResolvedValueOnce({
      items: [videoHistoryItem()],
      total: 1,
      page: 1,
      page_size: 8
    })

    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()

    await wrapper.get('[data-testid="video-history-regenerate-101"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="video-prompt"]').element.value).toBe('日系清新风格，柔和自然光的视频版本')
    expect(apiMocks.createVideoGeneration).not.toHaveBeenCalled()
    expect(wrapper.findAll('[data-testid="video-reference-thumb"]')).toHaveLength(2)
    expect(wrapper.text()).toContain('已回填历史任务参数，可编辑后再次生成。')
  })

  it('canonicalizes legacy Doubao history model before regenerate submit', async () => {
    apiMocks.listVideoModels.mockResolvedValueOnce({
      items: [
        {
          name: 'Grok Imagine',
          runtime_model: grokImagineVideoModel,
          provider: 'Wuyin',
          aspect_ratios: ['16:9', '9:16'],
          durations: ['6', '10', '15'],
          supports_hd: true,
          max_reference_images: 4
        },
        {
          name: 'Doubao Seedance 2.0 Mini',
          runtime_model: 'doubao-seed-2-0-mini-260428',
          provider: 'Volcengine Ark',
          aspect_ratios: ['16:9', '4:3', '1:1', '3:4', '9:16', '21:9', 'adaptive'],
          durations: ['4', '5', '6', '8', '10', '12', '15', '-1'],
          supports_hd: true,
          max_reference_images: 9
        }
      ]
    })
    apiMocks.listUserVideoGenerations.mockResolvedValueOnce({
      items: [videoHistoryItem({
        runtime_model: 'doubao-seed-2-0-mini',
        model_name: 'Doubao Seedance 2.0 Mini',
        aspect_ratio: '21:9',
        duration_seconds: 12,
        reference_asset_ids: []
      })],
      total: 1,
      page: 1,
      page_size: 8
    })
    mockCompletedVideo(111)

    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()

    await wrapper.get('[data-testid="video-history-regenerate-101"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="video-model"]').text()).toContain('Doubao Seedance 2.0 Mini')

    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createVideoGeneration).toHaveBeenCalledWith(expect.objectContaining({
      model: 'doubao-seed-2-0-mini-260428',
      aspect_ratio: '21:9',
      duration: '12'
    }))
  })

  it('opens a comparison modal for the current result and a history version', async () => {
    apiMocks.listUserVideoGenerations.mockResolvedValue({
      items: [videoHistoryItem()],
      total: 1,
      page: 1,
      page_size: 8
    })
    apiMocks.createVideoGeneration.mockResolvedValueOnce({
      generation_id: 110,
      status: 'queued',
      available_credits: 19
    })
    apiMocks.getVideoGeneration.mockResolvedValueOnce({
      generation_id: 110,
      status: 'succeeded',
      work_id: 910,
      prompt: '当前最新视频',
      preview_url: '/api/works/910/file',
      download_url: '/api/works/910/download',
      available_credits: 18
    })

    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()
    await wrapper.get('[data-testid="video-prompt"]').setValue('当前最新视频')
    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="video-history-compare-101"]').trigger('click')
    await flushPromises()

    const modal = document.body.querySelector('[data-testid="video-compare-modal"]')
    expect(modal).not.toBeNull()
    expect(modal.textContent).toContain('版本对比')
    expect(modal.textContent).toContain('当前版本')
    expect(modal.textContent).toContain('历史版本')
    expect(modal.querySelector('video[src="/api/works/910/file"]')).not.toBeNull()
    expect(modal.querySelector('video[src="/api/works/901/file"]')).not.toBeNull()

    modal.querySelector('[data-testid="video-compare-use-history"]').click()
    await flushPromises()

    expect(wrapper.get('[data-testid="video-preview-stage"] video').attributes('src')).toBe('/api/works/901/file')
  })

  it('shows soundtrack actions only after a completed video work is available', async () => {
    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()

    expect(wrapper.find('[data-testid="video-soundtrack-tools"]').exists()).toBe(false)

    apiMocks.createVideoGeneration.mockResolvedValueOnce({
      generation_id: 93,
      status: 'queued',
      available_credits: 19
    })
    apiMocks.getVideoGeneration.mockResolvedValueOnce({
      generation_id: 93,
      status: 'succeeded',
      work_id: 73,
      preview_url: '/api/works/73/file',
      download_url: '/api/works/73/download',
      available_credits: 18
    })

    await wrapper.get('[data-testid="video-prompt"]').setValue('完成后显示配乐入口')
    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    await flushPromises()

    const tools = wrapper.get('[data-testid="video-soundtrack-tools"]')
    expect(tools.text()).toContain('智能配乐')
    expect(tools.text()).toContain('换一首')
    expect(tools.text()).toContain('上传音乐')
    expect(apiMocks.listVideoSoundtracks).toHaveBeenCalledWith(73)
  })

  it('generates smart soundtrack and renders audio playback with download link', async () => {
    apiMocks.createVideoGeneration.mockResolvedValueOnce({
      generation_id: 94,
      status: 'queued',
      available_credits: 19
    })
    apiMocks.getVideoGeneration.mockResolvedValueOnce({
      generation_id: 94,
      status: 'succeeded',
      work_id: 74,
      preview_url: '/api/works/74/file',
      download_url: '/api/works/74/download',
      available_credits: 18
    })
    apiMocks.generateVideoSoundtrack.mockResolvedValueOnce({
      id: 1,
      audio_work_id: 201,
      source: 'ai',
      title: '智能配乐',
      audio_url: '/api/works/201/file',
      download_url: '/api/works/201/download',
      mime_type: 'audio/mpeg'
    })

    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()
    await wrapper.get('[data-testid="video-prompt"]').setValue('给视频生成配乐')
    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="video-soundtrack-smart"]').trigger('click')
    await flushPromises()

    expect(apiMocks.generateVideoSoundtrack).toHaveBeenCalledWith(74, { variation: 'smart' })
    expect(wrapper.get('[data-testid="video-soundtrack-player"]').attributes('src')).toBe('/api/works/201/file')
    expect(wrapper.get('[data-testid="video-soundtrack-download"]').attributes('href')).toBe('/api/works/201/download')
    expect(wrapper.text()).toContain('智能配乐')
  })

  it('replaces the current soundtrack when clicking replace', async () => {
    apiMocks.createVideoGeneration.mockResolvedValueOnce({
      generation_id: 95,
      status: 'queued',
      available_credits: 19
    })
    apiMocks.getVideoGeneration.mockResolvedValueOnce({
      generation_id: 95,
      status: 'succeeded',
      work_id: 75,
      preview_url: '/api/works/75/file',
      download_url: '/api/works/75/download',
      available_credits: 18
    })
    apiMocks.generateVideoSoundtrack.mockResolvedValueOnce({
      id: 2,
      audio_work_id: 202,
      source: 'ai',
      title: '智能配乐',
      audio_url: '/api/works/202/file',
      download_url: '/api/works/202/download',
      mime_type: 'audio/mpeg'
    })

    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()
    await wrapper.get('[data-testid="video-prompt"]').setValue('换一首背景音乐')
    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="video-soundtrack-replace"]').trigger('click')
    await flushPromises()

    expect(apiMocks.generateVideoSoundtrack).toHaveBeenCalledWith(75, { variation: 'replace' })
    expect(wrapper.get('[data-testid="video-soundtrack-player"]').attributes('src')).toBe('/api/works/202/file')
  })

  it('uploads a soundtrack file and renders the uploaded audio', async () => {
    apiMocks.createVideoGeneration.mockResolvedValueOnce({
      generation_id: 96,
      status: 'queued',
      available_credits: 19
    })
    apiMocks.getVideoGeneration.mockResolvedValueOnce({
      generation_id: 96,
      status: 'succeeded',
      work_id: 76,
      preview_url: '/api/works/76/file',
      download_url: '/api/works/76/download',
      available_credits: 18
    })
    apiMocks.uploadVideoSoundtrack.mockResolvedValueOnce({
      id: 3,
      audio_work_id: 203,
      source: 'upload',
      title: '上传音乐',
      audio_url: '/api/works/203/file',
      download_url: '/api/works/203/download',
      mime_type: 'audio/mpeg'
    })

    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()
    await wrapper.get('[data-testid="video-prompt"]').setValue('上传自己的音乐')
    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    await flushPromises()

    const file = new File(['music'], 'song.mp3', { type: 'audio/mpeg' })
    const input = wrapper.get('[data-testid="video-soundtrack-upload-input"]')
    Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
    await input.trigger('change')
    await flushPromises()

    expect(apiMocks.uploadVideoSoundtrack).toHaveBeenCalledWith(76, file)
    expect(wrapper.get('[data-testid="video-soundtrack-player"]').attributes('src')).toBe('/api/works/203/file')
    expect(wrapper.text()).toContain('上传音乐')
  })

  it('shows soundtrack errors and restores controls after failures', async () => {
    apiMocks.createVideoGeneration.mockResolvedValueOnce({
      generation_id: 97,
      status: 'queued',
      available_credits: 19
    })
    apiMocks.getVideoGeneration.mockResolvedValueOnce({
      generation_id: 97,
      status: 'succeeded',
      work_id: 77,
      preview_url: '/api/works/77/file',
      download_url: '/api/works/77/download',
      available_credits: 18
    })
    apiMocks.generateVideoSoundtrack.mockRejectedValueOnce(new Error('配乐生成失败'))

    const wrapper = mount(VideoWorkspaceView)
    await flushPromises()
    await wrapper.get('[data-testid="video-prompt"]').setValue('配乐失败展示错误')
    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="video-soundtrack-smart"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('配乐生成失败')
    expect(wrapper.get('[data-testid="video-soundtrack-smart"]').attributes('disabled')).toBeUndefined()
  })

  it('opens duration choices only on click and closes after selecting a value', async () => {
    apiMocks.createVideoGeneration.mockResolvedValueOnce({
      generation_id: 90,
      status: 'queued',
      available_credits: 18
    })
    apiMocks.getVideoGeneration.mockResolvedValueOnce({
      generation_id: 90,
      status: 'succeeded',
      available_credits: 17
    })

    const wrapper = mount(VideoWorkspaceView, {
      attachTo: document.body
    })
    await flushPromises()

    const trigger = wrapper.get('[data-testid="video-duration"]')
    await trigger.trigger('mouseenter')

    expect(clickSelectMenu('video-duration')).toBeNull()
    expect(trigger.attributes('aria-expanded')).toBe('false')

    await trigger.trigger('click')

    expect(clickSelectMenu('video-duration')).not.toBeNull()
    expect(trigger.attributes('aria-expanded')).toBe('true')

    clickSelectOption('video-duration', '15').click()
    await flushPromises()

    expect(wrapper.get('[data-testid="video-duration"]').text()).toContain('15 秒')
    expect(clickSelectMenu('video-duration')).toBeNull()
    expect(wrapper.get('[data-testid="video-duration"]').attributes('aria-expanded')).toBe('false')

    await wrapper.get('[data-testid="video-prompt"]').setValue('街角咖啡杯蒸汽特写')
    await wrapper.get('[data-testid="video-submit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createVideoGeneration).toHaveBeenCalledWith(expect.objectContaining({
      duration: '15'
    }))

    wrapper.unmount()
  })

  it('closes the duration choices from outside click or Escape', async () => {
    const wrapper = mount(VideoWorkspaceView, {
      attachTo: document.body
    })
    await flushPromises()

    await wrapper.get('[data-testid="video-duration"]').trigger('click')
    expect(clickSelectMenu('video-duration')).not.toBeNull()

    document.body.dispatchEvent(new MouseEvent('pointerdown', { bubbles: true }))
    await flushPromises()

    expect(clickSelectMenu('video-duration')).toBeNull()

    await wrapper.get('[data-testid="video-duration"]').trigger('click')
    expect(clickSelectMenu('video-duration')).not.toBeNull()

    await wrapper.get('[data-testid="video-duration"]').trigger('keydown', { key: 'Escape' })

    expect(clickSelectMenu('video-duration')).toBeNull()

    wrapper.unmount()
  })
})
