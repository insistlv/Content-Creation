import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  getAdminModel: vi.fn()
}))
const routerPush = vi.hoisted(() => vi.fn())
const routeState = vi.hoisted(() => ({
  params: { id: '3' }
}))

vi.mock('../api/client.js', () => ({
  api: {
    getAdminModel: apiMocks.getAdminModel
  }
}))

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({
    push: routerPush
  })
}))

import AdminModelDetailView from '../views/AdminModelDetailView.vue'

const detailPayload = {
  model: {
    id: 3,
    name: 'DALL-E 3',
    type: 'image',
    provider: 'OpenAI',
    status: 'online',
    runtime_model: 'gpt-image-2',
    cost_label: '5 点/次',
    permission: 'public'
  },
  usage: {
    total_calls: 24,
    succeeded_calls: 20,
    failed_calls: 4,
    today_calls: 5,
    last_7d_calls: 18,
    average_latency_ms: 1200
  },
  status_breakdown: [
    { status: 'succeeded', count: 20 },
    { status: 'failed', count: 4 }
  ],
  daily_trend: [
    { date: '2026-04-20', calls: 2, succeeded: 2, failed: 0 },
    { date: '2026-04-21', calls: 4, succeeded: 3, failed: 1 },
    { date: '2026-04-22', calls: 8, succeeded: 7, failed: 1 }
  ],
  recent_generations: [
    {
      id: 90,
      user_id: 7,
      model: 'gpt-image-2',
      status: 'succeeded',
      prompt_summary: '未来城市海报',
      latency_ms: 1180,
      created_at: '2026-05-03T04:01:00Z'
    }
  ]
}

describe('AdminModelDetailView', () => {
  afterEach(() => {
    vi.clearAllMocks()
    routerPush.mockReset()
  })

  it('renders model usage visualization and recent calls', async () => {
    apiMocks.getAdminModel.mockResolvedValue(detailPayload)

    const wrapper = mount(AdminModelDetailView)
    await flushPromises()

    expect(apiMocks.getAdminModel).toHaveBeenCalledWith('3')
    expect(wrapper.text()).toContain('DALL-E 3')
    expect(wrapper.text()).toContain('gpt-image-2')
    expect(wrapper.text()).toContain('总调用')
    expect(wrapper.text()).toContain('24')
    expect(wrapper.text()).toContain('成功率')
    expect(wrapper.text()).toContain('83%')
    expect(wrapper.findAll('[data-testid^="model-trend-bar-"]')).toHaveLength(3)
    expect(wrapper.text()).toContain('状态分布')
    expect(wrapper.text()).toContain('未来城市海报')
  })

  it('renders recent call attempts with success and failure diagnostics first', async () => {
    apiMocks.getAdminModel.mockResolvedValue({
      ...detailPayload,
      recent_call_attempts: [
        {
          id: 203,
          generation_record_id: 90,
          model_config_id: 3,
          attempt_index: 2,
          status: 'succeeded',
          latency_ms: 860,
          provider_request_id: 'req_success'
        },
        {
          id: 202,
          generation_record_id: 90,
          model_config_id: 3,
          attempt_index: 1,
          status: 'failed',
          latency_ms: 420,
          http_status: 502,
          error_code: 'provider_http_502',
          error_message: 'upstream failed'
        }
      ]
    })

    const wrapper = mount(AdminModelDetailView)
    await flushPromises()

    const rows = wrapper.findAll('[data-testid="model-call-attempt"]')
    expect(rows).toHaveLength(2)
    expect(rows[0].text()).toContain('第 2 次')
    expect(rows[0].text()).toContain('成功')
    expect(rows[0].classes()).toContain('is-success')
    expect(rows[1].text()).toContain('失败')
    expect(rows[1].text()).toContain('provider_http_502')
    expect(rows[1].text()).toContain('upstream failed')
    expect(rows[1].text()).toContain('HTTP 502')
    expect(rows[1].classes()).toContain('is-failed')
  })

  it('returns to model settings when clicking back', async () => {
    apiMocks.getAdminModel.mockResolvedValue(detailPayload)

    const wrapper = mount(AdminModelDetailView)
    await flushPromises()

    await wrapper.get('[data-testid="back-to-model-settings"]').trigger('click')

    expect(routerPush).toHaveBeenCalledWith('/admin/settings')
  })
})
