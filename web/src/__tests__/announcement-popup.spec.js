import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  listPopupAnnouncements: vi.fn(),
  dismissAnnouncement: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    listPopupAnnouncements: apiMocks.listPopupAnnouncements,
    dismissAnnouncement: apiMocks.dismissAnnouncement
  }
}))

import AnnouncementPopup from '../components/AnnouncementPopup.vue'

const popupItems = [
  {
    id: 5,
    title: '高优先级公告',
    content: '新模型已上线',
    level: 'important',
    action_text: '查看套餐',
    action_url: '/pricing',
    published_at: '2026-05-18T08:30:00Z'
  },
  {
    id: 6,
    title: '普通公告',
    content: '工作台细节优化',
    level: 'info',
    published_at: '2026-05-18T07:30:00Z'
  }
]

describe('AnnouncementPopup', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('loads popup announcements, steps through remaining items, and dismisses per client', async () => {
    apiMocks.listPopupAnnouncements.mockResolvedValue({ items: popupItems })
    apiMocks.dismissAnnouncement.mockResolvedValue({ ok: true })

    const wrapper = mount(AnnouncementPopup, {
      props: {
        enabled: true,
        client: 'web'
      }
    })
    await flushPromises()

    expect(apiMocks.listPopupAnnouncements).toHaveBeenCalledWith('web')
    expect(wrapper.get('[data-testid="announcement-popup"]').text()).toContain('高优先级公告')
    expect(wrapper.text()).toContain('1 / 2')
    expect(wrapper.get('[data-testid="announcement-popup-action"]').attributes('href')).toBe('/pricing')

    await wrapper.get('[data-testid="announcement-popup-next"]').trigger('click')

    expect(wrapper.get('[data-testid="announcement-popup"]').text()).toContain('普通公告')
    expect(wrapper.text()).toContain('2 / 2')

    await wrapper.get('[data-testid="announcement-popup-close"]').trigger('click')
    await flushPromises()

    expect(apiMocks.dismissAnnouncement).toHaveBeenCalledWith(6, 'web')
    expect(wrapper.find('[data-testid="announcement-popup"]').exists()).toBe(false)
  })

  it('does not load announcements until the user session is enabled', async () => {
    apiMocks.listPopupAnnouncements.mockResolvedValue({ items: popupItems })

    const wrapper = mount(AnnouncementPopup, {
      props: {
        enabled: false,
        client: 'web'
      }
    })
    await flushPromises()

    expect(apiMocks.listPopupAnnouncements).not.toHaveBeenCalled()

    await wrapper.setProps({ enabled: true })
    await flushPromises()

    expect(apiMocks.listPopupAnnouncements).toHaveBeenCalledWith('web')
  })
})
