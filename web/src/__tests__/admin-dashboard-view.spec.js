import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  getDashboard: vi.fn(),
  createAnnouncement: vi.fn(),
  updateAnnouncementStatus: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    getDashboard: apiMocks.getDashboard,
    createAnnouncement: apiMocks.createAnnouncement,
    updateAnnouncementStatus: apiMocks.updateAnnouncementStatus
  }
}))

import AdminDashboardView from '../views/AdminDashboardView.vue'

const dashboardPayload = {
  active_image_model: 'gpt-image-2',
  kpis: {
    users_total: 12,
    generation_total: 31,
    generation_succeeded: 26,
    generation_failed: 2,
    revenue_completed: '￥99.00'
  },
  packages: [
    { id: 1, name: '创作包', price_label: '99 元', credits: 60, badge: '推荐', is_active: true }
  ],
  models: [
    { name: 'gpt-image-2', active: true, request_timeout_seconds: 600 }
  ],
  generation_trend: [
    { date: '2026-04-29', total: 1, succeeded: 1, failed: 0 },
    { date: '2026-04-30', total: 3, succeeded: 2, failed: 1 }
  ],
  invite_summary: {
    active: 2,
    remaining: 18,
    used: 4,
    total: 22
  },
  recent_generations: [
    { id: 9, prompt: '春季产品主视觉', model: 'gpt-image-2', status: 'succeeded', preview_url: '/preview.png', created_at: '2026-04-30T10:00:00Z' }
  ],
  announcements: [
    { id: 5, title: '维护公告', content: '今晚 23 点短暂维护', level: 'important', status: 'published', created_at: '2026-04-30T10:00:00Z' }
  ],
  operation_logs: [
    { id: 3, action: 'admin.login', target_type: 'admin_user', target_id: 1, created_at: '2026-04-30T08:00:00Z' }
  ]
}

function mountDashboardView() {
  return mount(AdminDashboardView, {
    global: {
      stubs: {
        RouterLink: {
          props: ['to'],
          template: '<a :href="to"><slot /></a>'
        }
      }
    }
  })
}

describe('AdminDashboardView', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('renders dashboard panels from the structured payload', async () => {
    apiMocks.getDashboard.mockResolvedValue(dashboardPayload)

    const wrapper = mountDashboardView()
    await flushPromises()

    expect(apiMocks.getDashboard).toHaveBeenCalled()
    expect(wrapper.text()).toContain('￥99.00')
    expect(wrapper.text()).toContain('创作包')
    expect(wrapper.text()).not.toContain('购买意向')
    expect(wrapper.text()).toContain('gpt-image-2')
    expect(wrapper.text()).toContain('春季产品主视觉')
    expect(wrapper.text()).toContain('维护公告')
    await wrapper.findAll('.dashboard-tabs button')[1].trigger('click')
    expect(wrapper.text()).toContain('admin.login')
    expect(wrapper.find('[data-testid="dashboard-trend-chart"]').exists()).toBe(true)
  })

  it('links the announcement action to the full announcement management page', async () => {
    apiMocks.getDashboard.mockResolvedValue(dashboardPayload)

    const wrapper = mountDashboardView()
    await flushPromises()

    expect(wrapper.get('[data-testid="open-announcement-page"]').attributes('href')).toBe('/admin/announcements')
    expect(wrapper.find('[data-testid="announcement-form"]').exists()).toBe(false)
  })
})
