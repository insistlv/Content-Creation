import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  getSystemResources: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    getSystemResources: apiMocks.getSystemResources
  }
}))

import AdminSystemResourcesView from '../views/AdminSystemResourcesView.vue'

const firstPayload = {
  sampled_at: '2026-05-21T01:02:03Z',
  cpu: {
    usage_percent: 42.6,
    cores: 8,
    load_average: [0.5, 0.8, 1.2]
  },
  memory: {
    total_bytes: 16 * 1024 * 1024 * 1024,
    used_bytes: 9 * 1024 * 1024 * 1024,
    available_bytes: 7 * 1024 * 1024 * 1024,
    usage_percent: 56.25,
    swap_total_bytes: 4 * 1024 * 1024 * 1024,
    swap_used_bytes: 2 * 1024 * 1024 * 1024,
    swap_usage_percent: 50
  },
  disk: {
    path: '/',
    total_bytes: 512 * 1024 * 1024 * 1024,
    used_bytes: 300 * 1024 * 1024 * 1024,
    free_bytes: 212 * 1024 * 1024 * 1024,
    usage_percent: 58.6
  },
  processes: [
    { pid: 2840, name: 'image-agent', cpu_percent: 31.5, memory_percent: 12.4, rss_bytes: 384 * 1024 * 1024, status: 'running' },
    { pid: 1200, name: 'postgres', cpu_percent: 4.2, memory_percent: 8.8, rss_bytes: 96 * 1024 * 1024, status: 'sleeping' }
  ],
  generation: {
    queued: 12,
    running: 3,
    retry_waiting: 2,
    oldest_queue_age_ms: 65000,
    concurrency_limit: 4,
    used_slots: 3,
    queue_wait_p95_ms: 2100,
    provider_latency_p95_ms: 42000,
    provider_429_rate: 4.5,
    failure_rate: 2.25,
    lease_expired_count: 1,
    active_by_provider: { 3: 2 },
    active_by_channel: { 8: 2 },
    active_by_entry_point: { workspace_async: 3 }
  }
}

const secondPayload = {
  ...firstPayload,
  sampled_at: '2026-05-21T01:04:05Z',
  cpu: { ...firstPayload.cpu, usage_percent: 18.2 },
  processes: [
    { pid: 3001, name: 'nginx', cpu_percent: 6.1, memory_percent: 1.2, rss_bytes: 24 * 1024 * 1024, status: 'sleeping' }
  ]
}

function expectedSampledTime(value) {
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit'
  }).format(new Date(value))
}

describe('AdminSystemResourcesView', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-05-21T09:00:00+08:00'))
  })

  afterEach(() => {
    vi.clearAllMocks()
    vi.useRealTimers()
  })

  it('renders resource overview cards and process table from the admin API', async () => {
    apiMocks.getSystemResources.mockResolvedValueOnce(firstPayload)

    const wrapper = mount(AdminSystemResourcesView)
    await flushPromises()

    expect(apiMocks.getSystemResources).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('资源监控')
    expect(wrapper.text()).toContain('最近采样')
    expect(wrapper.text()).toContain('CPU 使用率')
    expect(wrapper.text()).toContain('42.6%')
    expect(wrapper.text()).toContain('内存使用')
    expect(wrapper.text()).toContain('9 GB / 16 GB')
    expect(wrapper.text()).toContain('磁盘使用')
    expect(wrapper.text()).toContain('300 GB / 512 GB')
    expect(wrapper.find('thead').text()).toContain('PID')
    expect(wrapper.find('thead').text()).toContain('进程名')
    expect(wrapper.find('thead').text()).toContain('CPU%')
    expect(wrapper.find('thead').text()).toContain('内存%')
    expect(wrapper.find('thead').text()).toContain('RSS')
    expect(wrapper.find('thead').text()).toContain('状态')
    expect(wrapper.text()).toContain('image-agent')
    expect(wrapper.text()).toContain('postgres')
    expect(wrapper.get('[data-testid="generation-queue-kpi"]').text()).toContain('12')
    expect(wrapper.get('[data-testid="generation-queue-panel"]').text()).toContain('3 / 4')
    expect(wrapper.get('[data-testid="generation-queue-panel"]').text()).toContain('2100 ms')
    expect(wrapper.get('[data-testid="generation-queue-panel"]').text()).toContain('2 GB / 4 GB')
    expect(wrapper.get('[data-testid="generation-queue-panel"]').text()).toContain('workspace_async: 3')
    expect(wrapper.get('[data-testid="generation-queue-panel"]').text()).toContain('租约过期次数')
    expect(wrapper.text()).toContain('384 MB')
    expect(wrapper.text()).toContain('运行中')
  })

  it('refreshes manually and updates sampled time', async () => {
    apiMocks.getSystemResources
      .mockResolvedValueOnce(firstPayload)
      .mockResolvedValueOnce(secondPayload)

    const wrapper = mount(AdminSystemResourcesView)
    await flushPromises()

    expect(wrapper.text()).toContain(expectedSampledTime(firstPayload.sampled_at))
    await wrapper.get('[data-testid="system-resources-refresh"]').trigger('click')
    await flushPromises()

    expect(apiMocks.getSystemResources).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain(expectedSampledTime(secondPayload.sampled_at))
    expect(wrapper.text()).toContain('18.2%')
    expect(wrapper.text()).toContain('nginx')
  })
})
