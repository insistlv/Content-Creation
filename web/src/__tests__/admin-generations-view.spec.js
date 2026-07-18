import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  generationExportURL: vi.fn((params = {}) => {
    const query = new URLSearchParams()
    Object.entries(params).forEach(([key, value]) => {
      if (value !== undefined && value !== null && value !== '') query.set(key, `${value}`)
    })
    const text = query.toString()
    return `/api/admin/generations/export${text ? `?${text}` : ''}`
  }),
  getGeneration: vi.fn(),
  listGenerations: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    generationExportURL: apiMocks.generationExportURL,
    getGeneration: apiMocks.getGeneration,
    listGenerations: apiMocks.listGenerations
  }
}))

import AdminGenerationsView from '../views/AdminGenerationsView.vue'
import { clickSelectMenu, openClickSelect } from './click-select-test-utils.js'

function makeListPayload(overrides = {}) {
  return {
    items: [
      {
        id: 102,
        user_id: 8,
        user: { id: 8, username: 'creator_alpha', display_name: '阿尔法设计师', email: 'alpha@example.com' },
        prompt_summary: '赛博森林玻璃塔，蓝绿色光线',
        preview_images: [{ preview_url: '/api/works/102/file', download_url: '/api/works/102/download' }],
        model: 'gpt-image-2',
        status: 'succeeded',
        latency_ms: 1280,
        credits_cost: 1,
        created_at: '2026-05-01T10:00:00Z'
      },
      {
        id: 101,
        user_id: 9,
        user: { id: 9, username: 'creator_beta', display_name: 'Beta', email: 'beta@example.com' },
        prompt_summary: '水彩城市街景',
        preview_images: [],
        model: 'gpt-image-2-2026-04-21',
        status: 'failed',
        latency_ms: 2400,
        credits_cost: 0,
        created_at: '2026-05-01T09:00:00Z'
      }
    ],
    summary: {
      today_generations: 42,
      today_generations_delta_percent: 12.5,
      success_rate: 93.2,
      success_rate_delta_percent: 2.1,
      average_latency_ms: 1860,
      average_latency_delta_percent: -8.4,
      failed_tasks: 3,
      failed_tasks_delta_percent: -25
    },
    filters: {},
    total: 42,
    page: 1,
    page_size: 20,
    ...overrides
  }
}

function makeDetail(overrides = {}) {
  return {
    id: 102,
    user_id: 8,
    user: { id: 8, username: 'creator_alpha', display_name: '阿尔法设计师', email: 'alpha@example.com' },
    prompt: '赛博森林玻璃塔，蓝绿色光线',
    model: 'gpt-image-2',
    status: 'succeeded',
    latency_ms: 1280,
    credits_cost: 1,
    created_at: '2026-05-01T10:00:00Z',
    params: {
      negative_prompt: '低清晰度',
      aspect_ratio: '1:1',
      quality: 'high',
      style_preset: 'cinematic',
      tool_mode: 'generate',
      style_strength: 70,
      reference_weight: 45,
      seed: 'seed-42'
    },
    result_images: [
      { preview_url: '/api/works/102/file', download_url: '/api/works/102/download' },
      { preview_url: '/api/works/103/file', download_url: '/api/works/103/download' }
    ],
    reference_images: [
      {
        reference_asset_id: 21,
        preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/reference-assets/8/2026/05/style-ref.png',
        download_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/reference-assets/8/2026/05/style-ref.png',
        mime_type: 'image/png',
        original_filename: 'style-ref.png',
        sort_order: 0
      }
    ],
    error: null,
    ...overrides
  }
}

describe('AdminGenerationsView', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-05-01T08:00:00Z'))
    vi.stubGlobal('open', vi.fn())
    apiMocks.generationExportURL.mockImplementation((params = {}) => {
      const query = new URLSearchParams()
      Object.entries(params).forEach(([key, value]) => {
        if (value !== undefined && value !== null && value !== '') query.set(key, `${value}`)
      })
      const text = query.toString()
      return `/api/admin/generations/export${text ? `?${text}` : ''}`
    })
  })

  afterEach(() => {
    vi.resetAllMocks()
    vi.useRealTimers()
    vi.unstubAllGlobals()
    document.body.innerHTML = ''
  })

  it('renders the KPI cards, filter bar and table without loading a default detail', async () => {
    apiMocks.listGenerations
      .mockResolvedValueOnce(makeListPayload())

    const wrapper = mount(AdminGenerationsView)
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(apiMocks.listGenerations).toHaveBeenNthCalledWith(1, expect.objectContaining({
      date_from: '2026-04-25',
      date_to: '2026-05-01',
      page: 1,
      page_size: 20
    }))
    expect(apiMocks.getGeneration).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('后台中心 / 生成记录')
    expect(wrapper.text()).toContain('今日生成')
    expect(wrapper.text()).toContain('42')
    expect(wrapper.text()).toContain('成功率')
    expect(wrapper.text()).toContain('93.2%')
    expect(wrapper.text()).toContain('阿尔法设计师')
    expect(wrapper.text()).toContain('赛博森林玻璃塔')
    expect(wrapper.find('[data-testid="generation-detail-modal"]').exists()).toBe(false)
    expect(wrapper.findAll('.generation-preview-thumb')).toHaveLength(1)

    await wrapper.findAll('tbody tr')[0].trigger('click')
    expect(apiMocks.getGeneration).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="generation-detail-modal"]').exists()).toBe(false)
  })

  it('opens a detail modal from the view button and omits the reference image section when empty', async () => {
    apiMocks.listGenerations.mockResolvedValueOnce(makeListPayload())
    apiMocks.getGeneration.mockResolvedValueOnce(makeDetail({ reference_images: [] }))

    const wrapper = mount(AdminGenerationsView)
    await flushPromises()
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="generation-view-102"]').trigger('click')
    await flushPromises()
    await wrapper.vm.$nextTick()

    const modal = wrapper.get('[data-testid="generation-detail-modal"]')
    expect(apiMocks.getGeneration).toHaveBeenCalledWith(102)
    expect(modal.text()).toContain('任务详情')
    expect(modal.text()).toContain('seed-42')
    expect(modal.text()).toContain('结果文件')
    expect(modal.text()).not.toContain('参考图片')
    expect(modal.find('.generation-reference-grid').exists()).toBe(false)
    expect(modal.findAll('.generation-result-grid img')).toHaveLength(2)
  })

  it('opens result and reference media in a preview modal without triggering thumbnail downloads', async () => {
    apiMocks.listGenerations.mockResolvedValueOnce(makeListPayload())
    apiMocks.getGeneration.mockResolvedValueOnce(makeDetail())

    const wrapper = mount(AdminGenerationsView, { attachTo: document.body })
    await flushPromises()
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="generation-view-102"]').trigger('click')
    await flushPromises()
    await wrapper.vm.$nextTick()
    window.open.mockClear()

    await wrapper.get('[data-testid="generation-result-media-0"]').trigger('click')
    await wrapper.vm.$nextTick()

    let previewModal = document.body.querySelector('[data-testid="generation-media-preview-modal"]')
    expect(window.open).not.toHaveBeenCalled()
    expect(previewModal).not.toBeNull()
    expect(previewModal.querySelector('img').getAttribute('src')).toBe('/api/works/102/file')
    expect(previewModal.querySelector('[data-testid="generation-media-preview-download"]').getAttribute('href')).toBe('/api/works/102/download')

    document.body.querySelector('[data-testid="generation-media-preview-close"]').click()
    await wrapper.vm.$nextTick()
    expect(document.body.querySelector('[data-testid="generation-media-preview-modal"]')).toBeNull()
    expect(wrapper.find('[data-testid="generation-detail-modal"]').exists()).toBe(true)

    await wrapper.get('[data-testid="generation-reference-media-0"]').trigger('click')
    await wrapper.vm.$nextTick()

    previewModal = document.body.querySelector('[data-testid="generation-media-preview-modal"]')
    expect(window.open).not.toHaveBeenCalled()
    expect(previewModal).not.toBeNull()
    expect(previewModal.textContent).toContain('style-ref.png')
    expect(previewModal.querySelector('img').getAttribute('src')).toBe('https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/reference-assets/8/2026/05/style-ref.png')

    previewModal.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await wrapper.vm.$nextTick()
    expect(document.body.querySelector('[data-testid="generation-media-preview-modal"]')).toBeNull()
    expect(wrapper.find('[data-testid="generation-detail-modal"]').exists()).toBe(true)

    await wrapper.get('[data-testid="generation-result-media-0"]').trigger('click')
    await wrapper.vm.$nextTick()
    globalThis.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wrapper.vm.$nextTick()
    expect(document.body.querySelector('[data-testid="generation-media-preview-modal"]')).toBeNull()
    expect(wrapper.find('[data-testid="generation-detail-modal"]').exists()).toBe(true)
  })

  it('queries, resets, paginates, exports CSV and downloads modal result images with current filters', async () => {
    apiMocks.listGenerations
      .mockResolvedValueOnce(makeListPayload())
      .mockResolvedValueOnce(makeListPayload({ total: 1 }))
      .mockResolvedValueOnce(makeListPayload())
      .mockResolvedValueOnce(makeListPayload({ page: 2, total: 42 }))
    apiMocks.getGeneration.mockResolvedValue(makeDetail())

    const wrapper = mount(AdminGenerationsView)
    await flushPromises()
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="generations-q"]').setValue('海报')
    await wrapper.get('[data-testid="generations-model"]').setValue('gpt-image-2')
    await wrapper.get('[data-testid="generations-user-keyword"]').setValue('creator_alpha')
    await wrapper.get('[data-testid="generations-status"]').setValue('succeeded')
    await wrapper.get('[data-testid="generations-query"]').trigger('click')
    await flushPromises()

    const queryParams = apiMocks.listGenerations.mock.calls[1][0]
    expect(queryParams).toEqual(expect.objectContaining({
      q: '海报',
      model: 'gpt-image-2',
      user_keyword: 'creator_alpha',
      status: 'succeeded',
      page: 1,
      page_size: 20
    }))
    expect(queryParams).not.toHaveProperty('user_id')

    await wrapper.get('[data-testid="generations-export"]').trigger('click')
    const exportParams = apiMocks.generationExportURL.mock.calls[0][0]
    expect(exportParams).toEqual(expect.objectContaining({
      q: '海报',
      model: 'gpt-image-2',
      user_keyword: 'creator_alpha',
      status: 'succeeded'
    }))
    expect(exportParams).not.toHaveProperty('user_id')
    expect(window.open).toHaveBeenCalledWith(expect.stringContaining('/api/admin/generations/export?'), 'generations-export')

    await wrapper.get('[data-testid="generation-view-102"]').trigger('click')
    await flushPromises()
    await wrapper.vm.$nextTick()
    await wrapper.get('[data-testid="generation-download-all"]').trigger('click')
    expect(window.open).toHaveBeenCalledWith('/api/works/102/download', '_blank')
    expect(window.open).toHaveBeenCalledWith('/api/works/103/download', '_blank')

    await wrapper.get('[data-testid="generations-reset"]').trigger('click')
    await flushPromises()
    expect(apiMocks.listGenerations).toHaveBeenNthCalledWith(3, expect.objectContaining({
      q: '',
      model: '',
      user_keyword: '',
      status: '',
      date_from: '2026-04-25',
      date_to: '2026-05-01',
      page: 1,
      page_size: 20
    }))
    expect(wrapper.find('[data-testid="generation-detail-modal"]').exists()).toBe(false)

    expect(wrapper.get('[data-testid="generations-prev"]').attributes()).toHaveProperty('disabled')
    expect(wrapper.get('[data-testid="generations-next"]').attributes('disabled')).toBeUndefined()

    await wrapper.get('[data-testid="generations-next"]').trigger('click')
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(apiMocks.listGenerations).toHaveBeenNthCalledWith(4, expect.objectContaining({ page: 2, page_size: 20 }))
    expect(wrapper.text()).toContain('第 21-22 条 / 共 42 条')
    expect(wrapper.get('[data-testid="generations-prev"]').attributes('disabled')).toBeUndefined()
  })

  it('shows failed task diagnostics in the detail modal and disables downloads without result images', async () => {
    apiMocks.listGenerations.mockResolvedValueOnce(makeListPayload())
    apiMocks.getGeneration
      .mockResolvedValueOnce(makeDetail({
        id: 101,
        status: 'failed',
        result_images: [],
        error: {
          code: 'provider_policy_rejected',
          message: '提交中含有违反平台政策的内容，请你立即停止或调整你的提交内容（traceid: trace-unsafe-1）'
        },
        provider_diagnostics: {
          provider_http_status: 500,
          provider_error_code: '<nil>',
          provider_error_message: '提交中含有违反平台政策的内容，请你立即停止或调整你的提交内容（traceid: trace-unsafe-1）',
          provider_failure_stage: 'image_generation_request',
          provider_attempt_count: 1
        },
        events: [
          {
            id: 1,
            trace_id: 'gen-trace-policy',
            level: 'info',
            stage: 'requesting_provider',
            event: 'provider_request_start',
            message: '开始请求图片供应商',
            metadata: { model: 'gpt-image-2', endpoint: '/v1/images/generations' },
            created_at: '2026-05-01T09:00:01Z'
          },
          {
            id: 2,
            trace_id: 'gen-trace-policy',
            level: 'error',
            stage: 'image_generation_request',
            event: 'provider_request_failed',
            message: '供应商图片生成请求失败',
            metadata: { provider_http_status: 500, provider_trace_id: 'trace-unsafe-1' },
            created_at: '2026-05-01T09:00:09Z'
          }
        ]
      }))

    const wrapper = mount(AdminGenerationsView)
    await flushPromises()
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="generation-view-101"]').trigger('click')
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(apiMocks.getGeneration).toHaveBeenLastCalledWith(101)
    const modal = wrapper.get('[data-testid="generation-detail-modal"]')
    expect(modal.text()).toContain('提交中含有违反平台政策的内容，请你立即停止或调整你的提交内容（traceid: trace-unsafe-1）')
    expect(modal.text()).toContain('供应商诊断')
    expect(modal.text()).toContain('HTTP 500')
    expect(modal.text()).toContain('提交中含有违反平台政策')
    expect(modal.text()).toContain('调用流程日志')
    expect(modal.text()).toContain('provider_request_start')
    expect(modal.text()).toContain('provider_request_failed')
    expect(modal.text()).toContain('trace-unsafe-1')
    expect(modal.get('[data-testid="generation-download-all"]').attributes()).toHaveProperty('disabled')
  })

  it('closes the detail modal from the close button, backdrop and Escape key', async () => {
    apiMocks.listGenerations.mockResolvedValueOnce(makeListPayload())
    apiMocks.getGeneration.mockResolvedValue(makeDetail())

    const wrapper = mount(AdminGenerationsView)
    await flushPromises()
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="generation-view-102"]').trigger('click')
    await flushPromises()
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="generation-detail-modal"]').exists()).toBe(true)

    await wrapper.get('[data-testid="generation-detail-modal-close"]').trigger('click')
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="generation-detail-modal"]').exists()).toBe(false)

    await wrapper.get('[data-testid="generation-view-102"]').trigger('click')
    await flushPromises()
    await wrapper.vm.$nextTick()
    await wrapper.get('[data-testid="generation-detail-modal-backdrop"]').trigger('click')
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="generation-detail-modal"]').exists()).toBe(false)

    await wrapper.get('[data-testid="generation-view-102"]').trigger('click')
    await flushPromises()
    await wrapper.vm.$nextTick()
    globalThis.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="generation-detail-modal"]').exists()).toBe(false)
  })

  it('keeps the newest modal detail when an earlier request resolves later', async () => {
    let resolveFirstDetail
    apiMocks.listGenerations.mockResolvedValueOnce(makeListPayload())
    apiMocks.getGeneration
      .mockReturnValueOnce(new Promise((resolve) => {
        resolveFirstDetail = resolve
      }))
      .mockResolvedValueOnce(makeDetail({
        id: 101,
        task_id: 'GEN-101-NEW',
        prompt: '水彩城市街景详情',
        params: { ...makeDetail().params, seed: 'seed-101-new' },
        result_images: []
      }))

    const wrapper = mount(AdminGenerationsView)
    await flushPromises()
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="generation-view-102"]').trigger('click')
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="generation-detail-modal"]').text()).toContain('详情加载中')

    await wrapper.get('[data-testid="generation-view-101"]').trigger('click')
    await flushPromises()
    await wrapper.vm.$nextTick()
    expect(apiMocks.getGeneration).toHaveBeenLastCalledWith(101)
    expect(wrapper.get('[data-testid="generation-detail-modal"]').text()).toContain('GEN-101-NEW')
    expect(wrapper.get('[data-testid="generation-detail-modal"]').text()).toContain('seed-101-new')

    resolveFirstDetail(makeDetail({
      task_id: 'GEN-102-LATE',
      params: { ...makeDetail().params, seed: 'seed-102-late' }
    }))
    await flushPromises()
    await wrapper.vm.$nextTick()

    const modal = wrapper.get('[data-testid="generation-detail-modal"]')
    expect(modal.text()).toContain('GEN-101-NEW')
    expect(modal.text()).toContain('seed-101-new')
    expect(modal.text()).not.toContain('GEN-102-LATE')
    expect(modal.text()).not.toContain('seed-102-late')
  })

  it('displays model names from model_config_id and queries duplicate runtimes by id', async () => {
    apiMocks.listGenerations
      .mockResolvedValueOnce(makeListPayload({
        items: [
          {
            id: 202,
            user_id: 8,
            user: { id: 8, username: 'creator_alpha', display_name: '阿尔法设计师', email: 'alpha@example.com' },
            prompt_summary: '官方线路生成',
            preview_images: [],
            model_config_id: 3,
            model_name: 'DALL-E 3',
            runtime_model: 'gpt-image-2',
            model: 'gpt-image-2',
            status: 'succeeded',
            latency_ms: 980,
            credits_cost: 1,
            created_at: '2026-05-01T10:00:00Z'
          },
          {
            id: 201,
            user_id: 9,
            user: { id: 9, username: 'creator_beta', display_name: 'Beta', email: 'beta@example.com' },
            prompt_summary: '分销线路生成',
            preview_images: [],
            model_config_id: 5,
            model_name: '生图模型2 ithinkai',
            runtime_model: 'gpt-image-2',
            model: 'gpt-image-2',
            status: 'succeeded',
            latency_ms: 720,
            credits_cost: 1,
            created_at: '2026-05-01T09:00:00Z'
          }
        ]
      }))
      .mockResolvedValueOnce(makeListPayload({ items: [], total: 0 }))
    const wrapper = mount(AdminGenerationsView)
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('DALL-E 3')
    expect(wrapper.text()).toContain('生图模型2 ithinkai')
    await openClickSelect(wrapper, 'generations-model')
    expect(clickSelectMenu('generations-model').textContent).toContain('DALL-E 3 · gpt-image-2')
    expect(clickSelectMenu('generations-model').textContent).toContain('生图模型2 ithinkai · gpt-image-2')
    expect(apiMocks.getGeneration).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="generations-model"]').setValue('id:5')
    await wrapper.get('[data-testid="generations-query"]').trigger('click')
    await flushPromises()

    expect(apiMocks.listGenerations).toHaveBeenNthCalledWith(2, expect.objectContaining({
      model: '',
      model_config_id: '5',
      page: 1,
      page_size: 20
    }))
  })

  it('shows loading, empty and API error states without breaking the page', async () => {
    let resolveRequest
    apiMocks.listGenerations.mockReturnValueOnce(new Promise((resolve) => {
      resolveRequest = resolve
    }))

    const loadingWrapper = mount(AdminGenerationsView)
    await loadingWrapper.vm.$nextTick()

    expect(loadingWrapper.text()).toContain('加载中...')
    expect(loadingWrapper.get('[data-testid="generations-prev"]').attributes()).toHaveProperty('disabled')
    expect(loadingWrapper.get('[data-testid="generations-next"]').attributes()).toHaveProperty('disabled')

    resolveRequest(makeListPayload({ items: [], total: 0 }))
    await flushPromises()

    apiMocks.listGenerations.mockRejectedValueOnce(new Error('generations_load_failed'))

    const wrapper = mount(AdminGenerationsView)
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('generations_load_failed')
  })
})
