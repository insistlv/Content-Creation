import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  getModelCenterOverview: vi.fn(),
  createModelCenterModel: vi.fn(),
  updateModelCenterModel: vi.fn(),
  deleteModelCenterModel: vi.fn(),
  createModelCenterProvider: vi.fn(),
  updateModelCenterProvider: vi.fn(),
  deleteModelCenterProvider: vi.fn(),
  createModelCenterChannel: vi.fn(),
  updateModelCenterChannel: vi.fn(),
  deleteModelCenterChannel: vi.fn(),
  listModelCenterChannelCallAttempts: vi.fn(),
  updateModelCenterRouting: vi.fn(),
  listModelCenterAuditLogs: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: apiMocks
}))

import AdminSettingsView from '../views/AdminSettingsView.vue'

const overviewPayload = {
  summary: {
    models: 2,
    providers: 2,
    channels: 3
  },
  models: [
    {
      id: 11,
      name: 'GPT Image 2',
      modality: 'image',
      status: 'online',
      visibility: 'public',
      default_credits_cost: 3,
      capability_tags: ['image', 'reference'],
      sort_order: 1
    },
    {
      id: 12,
      name: 'Sora 2',
      modality: 'video',
      status: 'online',
      visibility: 'internal',
      default_credits_cost: 8,
      capability_tags: ['video'],
      video_durations: ['10', '15', '25'],
      default_video_duration: '10',
      sort_order: 2
    }
  ],
  providers: [
    {
      id: 21,
      name: 'OpenAI 官方',
      provider: 'openai',
      base_url: 'https://official.example',
      api_key: '',
      api_key_set: true,
      default_timeout_seconds: 45,
      concurrency_limit: 2,
      status: 'online'
    },
    {
      id: 22,
      name: 'iThinkAI 分销',
      provider: 'ithinkai',
      base_url: 'https://ithinkai.example',
      api_key: '',
      api_key_set: true,
      default_timeout_seconds: 30,
      concurrency_limit: 4,
      status: 'online'
    }
  ],
  channels: [
    {
      id: 31,
      model_id: 11,
      model_name: 'GPT Image 2',
      provider_id: 21,
      provider_name: 'OpenAI 官方',
      name: '官方直连',
      runtime_model: 'gpt-image-2',
      endpoint: '/v1/images/generations',
      weight: 70,
      priority: 1,
      status: 'online',
      health_status: 'healthy'
    },
    {
      id: 32,
      model_id: 11,
      model_name: 'GPT Image 2',
      provider_id: 22,
      provider_name: 'iThinkAI 分销',
      name: '分销线路',
      runtime_model: 'gpt-image-2',
      endpoint: '/v1/images/generations',
      weight: 30,
      priority: 2,
      status: 'online',
      health_status: 'degraded'
    },
    {
      id: 33,
      model_id: 12,
      model_name: 'Sora 2',
      provider_id: 21,
      provider_name: 'OpenAI 官方',
      name: '视频直连',
      runtime_model: 'sora-2',
      video_durations: ['10', '15', '25'],
      endpoint: '/v1/videos/generations',
      weight: 100,
      priority: 1,
      status: 'online',
      health_status: 'healthy'
    }
  ],
  routing: [
    {
      id: 41,
      modality: 'image',
      default_model_id: 11,
      fallback_model_id: 11,
      routing_enabled: true,
      routing_strategy: 'weighted',
      source: 'model_center',
      entries: [
        { id: 51, model_id: 11, channel_id: 31, enabled: true, weight: 70, priority: 1 },
        { id: 52, model_id: 11, channel_id: 32, enabled: true, weight: 30, priority: 2 }
      ]
    },
    {
      id: 42,
      modality: 'video',
      default_model_id: 12,
      fallback_model_id: 12,
      routing_enabled: true,
      routing_strategy: 'default',
      source: 'model_center',
      entries: [
        { id: 53, model_id: 12, channel_id: 33, enabled: true, weight: 100, priority: 1 }
      ]
    }
  ],
  monitoring: [
    {
      model_id: 11,
      channel_id: 31,
      total_calls: 10,
      succeeded_calls: 9,
      failed_calls: 1,
      average_latency_ms: 840
    },
    {
      model_id: 11,
      channel_id: 32,
      total_calls: 5,
      succeeded_calls: 4,
      failed_calls: 1,
      average_latency_ms: 1200
    }
  ]
}

const auditPayload = {
  items: [
    {
      id: 61,
      action: 'model_center.routing.update',
      target_type: 'model_routing_policy',
      target_id: 41,
      admin_user_id: 1,
      detail: '{"after":{"routing_strategy":"weighted"}}',
      created_at: '2026-05-17T12:00:00Z'
    }
  ]
}

const channelCallAttemptsPage = {
  channel: {
    id: 31,
    model_id: 11,
    model_name: 'GPT Image 2',
    provider_id: 21,
    provider_name: 'OpenAI 官方',
    name: '官方直连',
    runtime_model: 'gpt-image-2',
    endpoint: '/v1/images/generations'
  },
  model_id: 11,
  items: [
    {
      id: 203,
      generation_record_id: 90,
      model_id: 11,
      channel_id: 31,
      model_config_id: 3,
      attempt_index: 2,
      status: 'succeeded',
      latency_ms: 860,
      http_status: 200,
      error_code: '',
      error_message: '',
      failure_stage: '',
      provider_request_id: 'req_success',
      started_at: '2026-05-18T10:00:00Z',
      finished_at: '2026-05-18T10:00:01Z',
      created_at: '2026-05-18T10:00:00Z'
    },
    {
      id: 202,
      generation_record_id: 89,
      model_id: 11,
      channel_id: 31,
      model_config_id: 3,
      attempt_index: 1,
      status: 'failed',
      latency_ms: 1200,
      http_status: 502,
      error_code: 'provider_http_502',
      error_message: 'upstream failed',
      failure_stage: 'image_generation_request',
      provider_request_id: 'req_failed',
      started_at: '2026-05-18T09:00:00Z',
      finished_at: '2026-05-18T09:00:01Z',
      created_at: '2026-05-18T09:00:00Z'
    }
  ],
  page: 1,
  page_size: 20,
  total: 42
}

function mockInitialLoad() {
  apiMocks.getModelCenterOverview.mockResolvedValue(structuredClone(overviewPayload))
  apiMocks.listModelCenterAuditLogs.mockResolvedValue(auditPayload)
  apiMocks.listModelCenterChannelCallAttempts.mockResolvedValue(structuredClone(channelCallAttemptsPage))
}

describe('AdminSettingsView', () => {
  afterEach(() => {
    vi.clearAllMocks()
    vi.unstubAllGlobals()
  })

  it('renders the model center overview with distinct business models and channels', async () => {
    mockInitialLoad()

    const wrapper = mount(AdminSettingsView)
    await flushPromises()

    expect(apiMocks.getModelCenterOverview).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('模型配置中心')
    expect(wrapper.text()).toContain('业务模型2')
    expect(wrapper.text()).toContain('供应商账号2')
    expect(wrapper.text()).toContain('调用渠道3')
    expect(wrapper.text()).toContain('GPT Image 2')
    expect(wrapper.text()).toContain('image / reference')

    await wrapper.get('[data-testid="tab-channels"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('官方直连')
    expect(wrapper.text()).toContain('分销线路')
    expect(wrapper.text()).toContain('gpt-image-2')
    expect(wrapper.text()).toContain('已设置')
  })

  it('creates and edits business models using model ids instead of runtime strings', async () => {
    mockInitialLoad()
    apiMocks.createModelCenterModel.mockResolvedValue({ id: 99 })
    apiMocks.updateModelCenterModel.mockResolvedValue({ ok: true })

    const wrapper = mount(AdminSettingsView)
    await flushPromises()

    await wrapper.get('[data-testid="open-model-create"]').trigger('click')
    await wrapper.get('[data-testid="model-name-input"]').setValue('GPT Image 3')
    await wrapper.get('[data-testid="model-modality-input"]').setValue('image')
    await wrapper.get('[data-testid="model-status-input"]').setValue('online')
    await wrapper.get('[data-testid="model-visibility-input"]').setValue('public')
    await wrapper.get('[data-testid="model-credits-input"]').setValue(5)
    await wrapper.get('[data-testid="model-tags-input"]').setValue('image, hd')
    await wrapper.get('[data-testid="model-sort-input"]').setValue(3)
    await wrapper.get('[data-testid="model-center-form"]').trigger('submit')
    await flushPromises()

    expect(apiMocks.createModelCenterModel).toHaveBeenCalledWith({
      name: 'GPT Image 3',
      modality: 'image',
      status: 'online',
      visibility: 'public',
      default_credits_cost: 5,
      capability_tags: ['image', 'hd'],
      video_durations: [],
      default_video_duration: '',
      sort_order: 3
    })

    await wrapper.get('[data-testid="edit-model-11"]').trigger('click')
    await wrapper.get('[data-testid="model-credits-input"]').setValue(4)
    await wrapper.get('[data-testid="model-center-form"]').trigger('submit')
    await flushPromises()

    expect(apiMocks.updateModelCenterModel).toHaveBeenCalledWith(11, expect.objectContaining({
      name: 'GPT Image 2',
      default_credits_cost: 4,
      capability_tags: ['image', 'reference']
    }))
  })

  it('creates an internal chat model for AI commerce vision analysis', async () => {
    mockInitialLoad()
    apiMocks.createModelCenterModel.mockResolvedValue({ id: 100 })

    const wrapper = mount(AdminSettingsView)
    await flushPromises()

    await wrapper.get('[data-testid="open-model-create"]').trigger('click')
    expect(wrapper.get('[data-testid="model-credits-input"]').attributes('min')).toBe('1')
    await wrapper.get('[data-testid="model-name-input"]').setValue('AI 电商视觉分析')
    const modalityInput = wrapper.get('[data-testid="model-modality-input"]')
    await modalityInput.trigger('keydown', { key: 'End' })
    await modalityInput.trigger('keydown', { key: 'Enter' })
    await flushPromises()
    const visibilityInput = wrapper.get('[data-testid="model-visibility-input"]')
    await visibilityInput.trigger('keydown', { key: 'End' })
    await visibilityInput.trigger('keydown', { key: 'Enter' })
    expect(wrapper.get('[data-testid="model-credits-input"]').attributes('min')).toBe('0')
    await wrapper.get('[data-testid="model-credits-input"]').setValue(0)
    await wrapper.get('[data-testid="model-tags-input"]').setValue('vision, commerce_vision')
    await wrapper.get('[data-testid="model-center-form"]').trigger('submit')
    await flushPromises()

    expect(apiMocks.createModelCenterModel).toHaveBeenCalledWith(expect.objectContaining({
      name: 'AI 电商视觉分析',
      modality: 'chat',
      visibility: 'internal',
      default_credits_cost: 0,
      capability_tags: ['vision', 'commerce_vision']
    }))
  })

  it('writes and clears provider API keys without expecting plaintext in the table', async () => {
    mockInitialLoad()
    apiMocks.createModelCenterProvider.mockResolvedValue({ id: 98, api_key: '', api_key_set: true })
    apiMocks.updateModelCenterProvider.mockResolvedValue({ ok: true })

    const wrapper = mount(AdminSettingsView)
    await flushPromises()
    await wrapper.get('[data-testid="tab-channels"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('已设置')
    expect(wrapper.text()).not.toContain('official-secret')

    await wrapper.get('[data-testid="open-provider-create"]').trigger('click')
    await wrapper.get('[data-testid="provider-name-input"]').setValue('备用供应商')
    await wrapper.get('[data-testid="provider-code-input"]').setValue('backup')
    await wrapper.get('[data-testid="provider-base-url-input"]').setValue('https://backup.example')
    await wrapper.get('[data-testid="provider-api-key-input"]').setValue('new-secret')
    await wrapper.get('[data-testid="provider-timeout-input"]').setValue(60)
    await wrapper.get('[data-testid="provider-concurrency-input"]').setValue(3)
    await wrapper.get('[data-testid="model-center-form"]').trigger('submit')
    await flushPromises()

    expect(apiMocks.createModelCenterProvider).toHaveBeenCalledWith({
      name: '备用供应商',
      provider: 'backup',
      base_url: 'https://backup.example',
      default_timeout_seconds: 60,
      concurrency_limit: 3,
      status: 'online',
      api_key: 'new-secret'
    })

    await wrapper.get('[data-testid="edit-provider-21"]').trigger('click')
    await wrapper.get('[data-testid="provider-clear-key-input"]').setValue(true)
    await wrapper.get('[data-testid="model-center-form"]').trigger('submit')
    await flushPromises()

    expect(apiMocks.updateModelCenterProvider).toHaveBeenCalledWith(21, expect.objectContaining({
      clear_api_key: true
    }))
  })

  it('creates channels with business model id, provider id, and runtime model as diagnostics', async () => {
    mockInitialLoad()
    apiMocks.createModelCenterChannel.mockResolvedValue({ id: 97 })
    apiMocks.updateModelCenterChannel.mockResolvedValue({ ok: true })

    const wrapper = mount(AdminSettingsView)
    await flushPromises()
    await wrapper.get('[data-testid="tab-channels"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="open-channel-create"]').trigger('click')
    await wrapper.get('[data-testid="channel-name-input"]').setValue('官方备用')
    await wrapper.get('[data-testid="channel-model-input"]').setValue('11')
    await wrapper.get('[data-testid="channel-provider-input"]').setValue('22')
    await wrapper.get('[data-testid="channel-runtime-input"]').setValue('gpt-image-2')
    await wrapper.get('[data-testid="channel-endpoint-input"]').setValue('/v1/images/generations')
    await wrapper.get('[data-testid="channel-weight-input"]').setValue(20)
    await wrapper.get('[data-testid="channel-priority-input"]').setValue(3)
    await wrapper.get('[data-testid="model-center-form"]').trigger('submit')
    await flushPromises()

    expect(apiMocks.createModelCenterChannel).toHaveBeenCalledWith({
      model_id: 11,
      provider_id: 22,
      name: '官方备用',
      runtime_model: 'gpt-image-2',
      video_durations: [],
      endpoint: '/v1/images/generations',
      weight: 20,
      priority: 3,
      status: 'online',
      health_status: 'healthy'
    })

    await wrapper.get('[data-testid="edit-channel-32"]').trigger('click')
    await wrapper.get('[data-testid="channel-health-input"]').setValue('healthy')
    await wrapper.get('[data-testid="model-center-form"]').trigger('submit')
    await flushPromises()

    expect(apiMocks.updateModelCenterChannel).toHaveBeenCalledWith(32, expect.objectContaining({
      model_id: 11,
      provider_id: 22,
      health_status: 'healthy'
    }))
  })

  it('saves routing with model ids and channel ids rather than runtime_model identity', async () => {
    mockInitialLoad()
    apiMocks.updateModelCenterRouting.mockResolvedValue({ ok: true })

    const wrapper = mount(AdminSettingsView)
    await flushPromises()
    await wrapper.get('[data-testid="tab-routing"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="routing-image-strategy"]').setValue('speed_first')
    await wrapper.get('[data-testid="routing-entry-31-weight"]').setValue(60)
    await wrapper.get('[data-testid="routing-entry-32-weight"]').setValue(40)
    await wrapper.get('[data-testid="routing-entry-32-priority"]').setValue(1)
    await wrapper.get('[data-testid="save-routing"]').trigger('click')
    await flushPromises()

    expect(apiMocks.updateModelCenterRouting).toHaveBeenCalledWith({
      routes: [
        {
          modality: 'image',
          default_model_id: 11,
          fallback_model_id: 11,
          routing_enabled: true,
          routing_strategy: 'speed_first',
          entries: [
            { model_id: 11, channel_id: 31, enabled: true, weight: 60, priority: 1 },
            { model_id: 11, channel_id: 32, enabled: true, weight: 40, priority: 1 }
          ]
        },
        {
          modality: 'video',
          default_model_id: 12,
          fallback_model_id: 12,
          routing_enabled: true,
          routing_strategy: 'default',
          entries: [
            { model_id: 12, channel_id: 33, enabled: true, weight: 100, priority: 1 }
          ]
        }
      ]
    })
    expect(JSON.stringify(apiMocks.updateModelCenterRouting.mock.calls[0][0])).not.toContain('runtime_model')
  })

  it('edits video model and channel duration capabilities from the model center', async () => {
    mockInitialLoad()
    apiMocks.updateModelCenterModel.mockResolvedValue({ ok: true })
    apiMocks.updateModelCenterChannel.mockResolvedValue({ ok: true })

    const wrapper = mount(AdminSettingsView)
    await flushPromises()

    await wrapper.get('[data-testid="edit-model-12"]').trigger('click')
    expect(wrapper.get('[data-testid="model-video-duration-editor"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="model-default-duration-input"]').exists()).toBe(true)
    await wrapper.get('[data-testid="model-center-form"]').trigger('submit')
    await flushPromises()
    expect(apiMocks.updateModelCenterModel).toHaveBeenCalledWith(12, expect.objectContaining({
      video_durations: ['10', '15', '25'],
      default_video_duration: '10'
    }))

    await wrapper.get('[data-testid="tab-channels"]').trigger('click')
    await wrapper.get('[data-testid="edit-channel-33"]').trigger('click')
    expect(wrapper.get('[data-testid="channel-video-duration-editor"]').exists()).toBe(true)
    await wrapper.get('[data-testid="model-center-form"]').trigger('submit')
    await flushPromises()
    expect(apiMocks.updateModelCenterChannel).toHaveBeenCalledWith(33, expect.objectContaining({
      video_durations: ['10', '15', '25']
    }))
  })

  it('renders and saves the chat routing when a commerce vision model exists', async () => {
    const payload = structuredClone(overviewPayload)
    payload.models.push({
      id: 13,
      name: 'AI 电商视觉分析',
      modality: 'chat',
      status: 'online',
      visibility: 'internal',
      default_credits_cost: 0,
      capability_tags: ['vision', 'commerce_vision'],
      sort_order: 50
    })
    payload.channels.push({
      id: 34,
      model_id: 13,
      model_name: 'AI 电商视觉分析',
      provider_id: 22,
      provider_name: 'iThinkAI 分销',
      name: 'iThinkAI Gemini 视觉分析',
      runtime_model: 'gemini-3.5-flash',
      endpoint: '/v1/chat/completions',
      weight: 100,
      priority: 1,
      status: 'online',
      health_status: 'healthy'
    })
    payload.routing.push({
      id: 43,
      modality: 'chat',
      default_model_id: 13,
      fallback_model_id: 13,
      routing_enabled: true,
      routing_strategy: 'default',
      source: 'model_center',
      entries: [{ id: 54, model_id: 13, channel_id: 34, enabled: true, weight: 100, priority: 1 }]
    })
    apiMocks.getModelCenterOverview.mockResolvedValue(payload)
    apiMocks.listModelCenterAuditLogs.mockResolvedValue(auditPayload)
    apiMocks.updateModelCenterRouting.mockResolvedValue({ ok: true })

    const wrapper = mount(AdminSettingsView)
    await flushPromises()
    await wrapper.get('[data-testid="tab-routing"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="routing-chat-default-model"]').attributes('value')).toBe('13')
    expect(wrapper.get('[data-testid="routing-entry-34-enabled"]').element.checked).toBe(true)
    await wrapper.get('[data-testid="save-routing"]').trigger('click')
    await flushPromises()

    expect(apiMocks.updateModelCenterRouting).toHaveBeenCalledWith(expect.objectContaining({
      routes: expect.arrayContaining([
        expect.objectContaining({
          modality: 'chat',
          default_model_id: 13,
          fallback_model_id: 13,
          entries: [expect.objectContaining({ model_id: 13, channel_id: 34, enabled: true })]
        })
      ])
    }))
  })

  it('shows monitoring drill-down and audit logs', async () => {
    mockInitialLoad()

    const wrapper = mount(AdminSettingsView)
    await flushPromises()

    await wrapper.get('[data-testid="tab-monitoring"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('按业务模型汇总')
    expect(wrapper.text()).toContain('15')
    expect(wrapper.text()).toContain('87%')
    expect(wrapper.text()).toContain('官方直连')
    expect(wrapper.text()).toContain('分销线路')

    await wrapper.get('[data-testid="tab-audit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.listModelCenterAuditLogs).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('model_center.routing.update')
    expect(wrapper.text()).toContain('model_routing_policy')
  })

  it('opens channel call attempt diagnostics from monitoring rows', async () => {
    mockInitialLoad()

    const wrapper = mount(AdminSettingsView)
    await flushPromises()

    await wrapper.get('[data-testid="tab-monitoring"]').trigger('click')
    await flushPromises()

    const viewButton = wrapper.get('[data-testid="monitoring-channel-calls-31"]')
    expect(viewButton.text()).toContain('查看')

    await viewButton.trigger('click')
    await flushPromises()

    expect(apiMocks.listModelCenterChannelCallAttempts).toHaveBeenCalledWith(31, {
      model_id: 11,
      page: 1,
      page_size: 20,
      status: '',
      date_from: '',
      date_to: ''
    })
    expect(wrapper.get('[data-testid="channel-call-attempt-modal"]').text()).toContain('官方直连')
    expect(wrapper.get('[data-testid="channel-call-attempt-modal"]').text()).toContain('GPT Image 2')
    expect(wrapper.text()).toContain('req_success')
    expect(wrapper.text()).toContain('provider_http_502')
    expect(wrapper.text()).toContain('upstream failed')
  })

  it('filters, paginates, and resets channel call attempts without reloading the overview', async () => {
    mockInitialLoad()
    apiMocks.listModelCenterChannelCallAttempts.mockImplementation((channelId, params) => Promise.resolve({
      ...structuredClone(channelCallAttemptsPage),
      items: params.page === 2 ? [
        {
          ...channelCallAttemptsPage.items[1],
          id: 201,
          generation_record_id: 88,
          provider_request_id: 'req_page_2'
        }
      ] : structuredClone(channelCallAttemptsPage.items),
      page: params.page,
      total: 42
    }))

    const wrapper = mount(AdminSettingsView)
    await flushPromises()

    await wrapper.get('[data-testid="tab-monitoring"]').trigger('click')
    await wrapper.get('[data-testid="monitoring-channel-calls-31"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="channel-call-attempt-status"]').setValue('failed')
    await wrapper.get('[data-testid="channel-call-attempt-date-from"]').setValue('2026-05-01')
    await wrapper.get('[data-testid="channel-call-attempt-date-to"]').setValue('2026-05-18')
    await wrapper.get('[data-testid="channel-call-attempt-query"]').trigger('click')
    await flushPromises()

    expect(apiMocks.listModelCenterChannelCallAttempts).toHaveBeenLastCalledWith(31, {
      model_id: 11,
      page: 1,
      page_size: 20,
      status: 'failed',
      date_from: '2026-05-01',
      date_to: '2026-05-18'
    })

    await wrapper.get('[data-testid="channel-call-attempt-next"]').trigger('click')
    await flushPromises()

    expect(apiMocks.listModelCenterChannelCallAttempts).toHaveBeenLastCalledWith(31, {
      model_id: 11,
      page: 2,
      page_size: 20,
      status: 'failed',
      date_from: '2026-05-01',
      date_to: '2026-05-18'
    })
    expect(wrapper.text()).toContain('21-40 / 42')
    expect(wrapper.text()).toContain('req_page_2')

    await wrapper.get('[data-testid="channel-call-attempt-prev"]').trigger('click')
    await flushPromises()

    expect(apiMocks.listModelCenterChannelCallAttempts).toHaveBeenLastCalledWith(31, expect.objectContaining({
      page: 1,
      status: 'failed',
      date_from: '2026-05-01',
      date_to: '2026-05-18'
    }))

    await wrapper.get('[data-testid="channel-call-attempt-reset"]').trigger('click')
    await flushPromises()

    expect(apiMocks.listModelCenterChannelCallAttempts).toHaveBeenLastCalledWith(31, {
      model_id: 11,
      page: 1,
      page_size: 20,
      status: '',
      date_from: '',
      date_to: ''
    })
    expect(wrapper.get('[data-testid="channel-call-attempt-status"]').element.value).toBe('')
    expect(wrapper.get('[data-testid="channel-call-attempt-date-from"]').element.value).toBe('')
    expect(wrapper.get('[data-testid="channel-call-attempt-date-to"]').element.value).toBe('')
    expect(apiMocks.getModelCenterOverview).toHaveBeenCalledTimes(1)
  })

  it('closes channel call attempt modal by button, backdrop, and Escape while clearing filters', async () => {
    mockInitialLoad()

    const wrapper = mount(AdminSettingsView)
    await flushPromises()
    await wrapper.get('[data-testid="tab-monitoring"]').trigger('click')

    await wrapper.get('[data-testid="monitoring-channel-calls-31"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="channel-call-attempt-status"]').setValue('failed')
    await wrapper.get('[data-testid="channel-call-attempt-modal-close"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="channel-call-attempt-modal"]').exists()).toBe(false)

    await wrapper.get('[data-testid="monitoring-channel-calls-31"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="channel-call-attempt-status"]').element.value).toBe('')
    await wrapper.get('[data-testid="channel-call-attempt-modal-backdrop"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="channel-call-attempt-modal"]').exists()).toBe(false)

    await wrapper.get('[data-testid="monitoring-channel-calls-31"]').trigger('click')
    await flushPromises()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(wrapper.find('[data-testid="channel-call-attempt-modal"]').exists()).toBe(false)
  })

  it('shows loading, empty, and failure states inside the call attempt modal', async () => {
    mockInitialLoad()
    let resolveAttempts
    apiMocks.listModelCenterChannelCallAttempts.mockReturnValueOnce(new Promise((resolve) => {
      resolveAttempts = resolve
    }))

    const wrapper = mount(AdminSettingsView)
    await flushPromises()
    await wrapper.get('[data-testid="tab-monitoring"]').trigger('click')
    await wrapper.get('[data-testid="monitoring-channel-calls-31"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('调用尝试加载中...')
    resolveAttempts({ ...structuredClone(channelCallAttemptsPage), items: [], total: 0 })
    await flushPromises()
    expect(wrapper.text()).toContain('暂无调用尝试')

    apiMocks.listModelCenterChannelCallAttempts.mockRejectedValueOnce(new Error('调用尝试读取失败'))
    await wrapper.get('[data-testid="channel-call-attempt-query"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('调用尝试读取失败')
    expect(wrapper.text()).toContain('按渠道钻取')
  })

  it('deletes channels after confirmation and refreshes the overview', async () => {
    mockInitialLoad()
    apiMocks.deleteModelCenterChannel.mockResolvedValue({ ok: true })
    vi.stubGlobal('confirm', vi.fn().mockReturnValue(true))

    const wrapper = mount(AdminSettingsView)
    await flushPromises()
    await wrapper.get('[data-testid="tab-channels"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="delete-channel-32"]').trigger('click')
    await flushPromises()

    expect(window.confirm).toHaveBeenCalledWith('删除渠道「分销线路」？')
    expect(apiMocks.deleteModelCenterChannel).toHaveBeenCalledWith(32)
    expect(apiMocks.getModelCenterOverview).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('渠道已删除')
  })

  it('removes a deleted provider and its channels from the refreshed overview', async () => {
    const refreshedOverview = structuredClone(overviewPayload)
    refreshedOverview.summary = { models: 2, providers: 1, channels: 2 }
    refreshedOverview.providers = refreshedOverview.providers.filter((provider) => provider.id !== 22)
    refreshedOverview.channels = refreshedOverview.channels.filter((channel) => channel.provider_id !== 22)
    refreshedOverview.routing = refreshedOverview.routing.map((route) => ({
      ...route,
      entries: (route.entries ?? []).filter((entry) => entry.channel_id !== 32)
    }))
    apiMocks.getModelCenterOverview
      .mockResolvedValueOnce(structuredClone(overviewPayload))
      .mockResolvedValueOnce(refreshedOverview)
    apiMocks.listModelCenterAuditLogs.mockResolvedValue(auditPayload)
    apiMocks.deleteModelCenterProvider.mockResolvedValue({ ok: true })
    vi.stubGlobal('confirm', vi.fn().mockReturnValue(true))

    const wrapper = mount(AdminSettingsView)
    await flushPromises()
    await wrapper.get('[data-testid="tab-channels"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('iThinkAI 分销')
    expect(wrapper.text()).toContain('分销线路')

    await wrapper.get('[data-testid="delete-provider-22"]').trigger('click')
    await flushPromises()

    expect(window.confirm).toHaveBeenCalledWith('删除供应商「iThinkAI 分销」？')
    expect(apiMocks.deleteModelCenterProvider).toHaveBeenCalledWith(22)
    expect(apiMocks.getModelCenterOverview).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('供应商已删除')
    expect(wrapper.text()).toContain('供应商账号1')
    expect(wrapper.text()).toContain('调用渠道2')
    expect(wrapper.text()).not.toContain('iThinkAI 分销')
    expect(wrapper.text()).not.toContain('分销线路')
  })
})
