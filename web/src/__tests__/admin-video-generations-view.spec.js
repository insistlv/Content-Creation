import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  getAdminVideoGeneration: vi.fn(),
  listVideoGenerations: vi.fn(),
  videoGenerationExportURL: vi.fn((params = {}) => {
    const query = new URLSearchParams()
    Object.entries(params).forEach(([key, value]) => {
      if (value !== undefined && value !== null && value !== '') query.set(key, `${value}`)
    })
    const text = query.toString()
    return `/api/admin/video-generations/export${text ? `?${text}` : ''}`
  })
}))

vi.mock('../api/client.js', () => ({
  api: {
    getAdminVideoGeneration: apiMocks.getAdminVideoGeneration,
    listVideoGenerations: apiMocks.listVideoGenerations,
    videoGenerationExportURL: apiMocks.videoGenerationExportURL
  }
}))

import AdminVideoGenerationsView from '../views/AdminVideoGenerationsView.vue'

function makeListPayload(overrides = {}) {
  return {
    items: [
      {
        id: 301,
        generation_record_id: 9001,
        user_id: 8,
        user: { id: 8, username: 'creator_video', display_name: 'Video Creator', email: 'video@example.com' },
        source: 'workspace',
        prompt_summary: 'product launch video',
        aspect_ratio: '16:9',
        duration_seconds: 6,
        model_name: 'Grok Imagine',
        runtime_model: 'grok-imagine-video-1.5-preview',
        provider: 'Wuyin',
        provider_request_id: 'provider-video-301',
        status: 'succeeded',
        latency_ms: 2100,
        credits_cost: 5,
        preview_url: 'https://cdn.example.com/video.mp4',
        download_url: 'https://cdn.example.com/video.mp4',
        mime_type: 'video/mp4',
        created_at: '2026-05-01T10:00:00Z'
      }
    ],
    summary: {
      today_videos: 7,
      today_videos_delta_percent: 16.7,
      success_rate: 88.9,
      success_rate_delta_percent: 1.2,
      average_latency_ms: 2400,
      average_latency_delta_percent: -4.1,
      failed_tasks: 1,
      failed_tasks_delta_percent: 0
    },
    filters: {},
    total: 7,
    page: 1,
    page_size: 20,
    ...overrides
  }
}

function makeDetail(overrides = {}) {
  return {
    id: 301,
    generation_record_id: 9001,
    user_id: 8,
    user: { id: 8, username: 'creator_video', display_name: 'Video Creator', email: 'video@example.com' },
    source: 'workspace',
    prompt: 'product launch video with clean lighting',
    aspect_ratio: '16:9',
    duration_seconds: 6,
    model_name: 'Grok Imagine',
    runtime_model: 'grok-imagine-video-1.5-preview',
    provider: 'Wuyin',
    provider_request_id: 'provider-video-301',
    status: 'succeeded',
    latency_ms: 2100,
    credits_cost: 5,
    created_at: '2026-05-01T10:00:00Z',
    result_video: {
      work_id: 66,
      preview_url: 'https://cdn.example.com/video.mp4',
      download_url: 'https://cdn.example.com/video.mp4',
      mime_type: 'video/mp4'
    },
    reference_images: [],
    provider_diagnostics: {
      provider_http_status: 200
    },
    events: [],
    error: null,
    ...overrides
  }
}

describe('AdminVideoGenerationsView', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-05-01T08:00:00Z'))
    vi.stubGlobal('open', vi.fn())
  })

  afterEach(() => {
    vi.resetAllMocks()
    vi.useRealTimers()
    vi.unstubAllGlobals()
    document.body.innerHTML = ''
  })

  it('loads video records with default date range and renders KPI cards and rows', async () => {
    apiMocks.listVideoGenerations.mockResolvedValueOnce(makeListPayload())

    const wrapper = mount(AdminVideoGenerationsView)
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(apiMocks.listVideoGenerations).toHaveBeenNthCalledWith(1, expect.objectContaining({
      date_from: '2026-04-25',
      date_to: '2026-05-01',
      page: 1,
      page_size: 20
    }))
    expect(wrapper.text()).toContain('视频记录')
    expect(wrapper.text()).toContain('今日视频')
    expect(wrapper.text()).toContain('7')
    expect(wrapper.text()).toContain('Video Creator')
    expect(wrapper.text()).toContain('product launch video')
    expect(wrapper.find('[data-testid="video-generation-detail-modal"]').exists()).toBe(false)
    expect(wrapper.find('video').exists()).toBe(true)
  })

  it('opens details and exports with current filters', async () => {
    apiMocks.listVideoGenerations.mockResolvedValueOnce(makeListPayload())
    apiMocks.getAdminVideoGeneration.mockResolvedValueOnce(makeDetail())

    const wrapper = mount(AdminVideoGenerationsView)
    await flushPromises()
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="video-generation-view-301"]').trigger('click')
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(apiMocks.getAdminVideoGeneration).toHaveBeenCalledWith(301)
    const modal = wrapper.get('[data-testid="video-generation-detail-modal"]')
    expect(modal.text()).toContain('provider-video-301')
    expect(modal.find('video').attributes('src')).toBe('https://cdn.example.com/video.mp4')

    await wrapper.get('[data-testid="video-generation-export"]').trigger('click')
    expect(apiMocks.videoGenerationExportURL).toHaveBeenCalledWith(expect.objectContaining({
      date_from: '2026-04-25',
      date_to: '2026-05-01'
    }))
    expect(window.open).toHaveBeenCalledWith('/api/admin/video-generations/export?date_from=2026-04-25&date_to=2026-05-01', 'video-generations-export')
  })
})
