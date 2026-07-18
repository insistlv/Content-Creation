import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  listAnnouncements: vi.fn(),
  createAnnouncement: vi.fn(),
  updateAnnouncement: vi.fn(),
  updateAnnouncementStatus: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    listAnnouncements: apiMocks.listAnnouncements,
    createAnnouncement: apiMocks.createAnnouncement,
    updateAnnouncement: apiMocks.updateAnnouncement,
    updateAnnouncementStatus: apiMocks.updateAnnouncementStatus
  }
}))

import AdminAnnouncementsView from '../views/AdminAnnouncementsView.vue'

const announcementPayload = {
  items: [
    {
      id: 7,
      title: '版本更新公告',
      content: '新模型与小程序弹窗已上线',
      level: 'important',
      status: 'published',
      target_clients: ['web', 'mp-weixin'],
      popup_enabled: true,
      starts_at: '2026-05-18T08:00:00Z',
      ends_at: '2026-05-20T08:00:00Z',
      priority: 40,
      action_text: '立即查看',
      action_url: '/pricing',
      published_at: '2026-05-18T08:30:00Z',
      created_at: '2026-05-18T08:00:00Z'
    }
  ],
  total: 1,
  page: 1,
  page_size: 12,
  summary: {
    total: 3,
    published: 1,
    draft: 1,
    offline: 1,
    popup_enabled: 2
  }
}

function mockList(payload = announcementPayload) {
  apiMocks.listAnnouncements.mockResolvedValue(payload)
}

describe('AdminAnnouncementsView', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('loads announcement KPIs, table rows, and the side preview', async () => {
    mockList()

    const wrapper = mount(AdminAnnouncementsView)
    await flushPromises()

    expect(apiMocks.listAnnouncements).toHaveBeenCalledWith({
      page: 1,
      page_size: 12,
      status: 'all',
      level: 'all',
      client: 'all',
      keyword: ''
    })
    expect(wrapper.text()).toContain('公告通知')
    expect(wrapper.text()).toContain('已发布')
    expect(wrapper.text()).toContain('版本更新公告')
    expect(wrapper.text()).toContain('Web / 小程序')
    expect(wrapper.get('[data-testid="announcement-preview"]').text()).toContain('立即查看')
  })

  it('creates a draft announcement with popup delivery settings', async () => {
    mockList()
    apiMocks.createAnnouncement.mockResolvedValue({ id: 9 })

    const wrapper = mount(AdminAnnouncementsView)
    await flushPromises()

    await wrapper.get('[data-testid="open-announcement-create"]').trigger('click')
    await wrapper.get('[data-testid="announcement-title"]').setValue('维护提醒')
    await wrapper.get('[data-testid="announcement-content"]').setValue('今晚 23 点短暂维护')
    await wrapper.get('[data-testid="announcement-level"]').setValue('warning')
    await wrapper.get('[data-testid="announcement-status"]').setValue('draft')
    await wrapper.get('[data-testid="announcement-target"]').setValue('web')
    await wrapper.get('[data-testid="announcement-popup"]').setValue(true)
    await wrapper.get('[data-testid="announcement-priority"]').setValue('30')
    await wrapper.get('[data-testid="announcement-action-text"]').setValue('查看详情')
    await wrapper.get('[data-testid="announcement-action-url"]').setValue('/works')
    await wrapper.get('[data-testid="announcement-form"]').trigger('submit')
    await flushPromises()

    expect(apiMocks.createAnnouncement).toHaveBeenCalledWith(expect.objectContaining({
      title: '维护提醒',
      content: '今晚 23 点短暂维护',
      level: 'warning',
      status: 'draft',
      target_clients: ['web'],
      popup_enabled: true,
      priority: 30,
      action_text: '查看详情',
      action_url: '/works'
    }))
    expect(apiMocks.listAnnouncements).toHaveBeenCalledTimes(2)
  })

  it('edits an announcement and changes published state without deleting it', async () => {
    mockList()
    apiMocks.updateAnnouncement.mockResolvedValue({ ok: true })
    apiMocks.updateAnnouncementStatus.mockResolvedValue({ ok: true })

    const wrapper = mount(AdminAnnouncementsView)
    await flushPromises()

    await wrapper.get('[data-testid="edit-announcement-7"]').trigger('click')
    await wrapper.get('[data-testid="announcement-title"]').setValue('版本更新公告 v2')
    await wrapper.get('[data-testid="announcement-target"]').setValue('all')
    await wrapper.get('[data-testid="announcement-form"]').trigger('submit')
    await flushPromises()

    expect(apiMocks.updateAnnouncement).toHaveBeenCalledWith(7, expect.objectContaining({
      title: '版本更新公告 v2',
      target_clients: ['all']
    }))

    await wrapper.get('[data-testid="offline-announcement-7"]').trigger('click')
    await flushPromises()

    expect(apiMocks.updateAnnouncementStatus).toHaveBeenCalledWith(7, 'offline')
    expect(apiMocks.listAnnouncements).toHaveBeenCalledTimes(3)
  })
})
