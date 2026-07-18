import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  listInvites: vi.fn(),
  batchCreateInvites: vi.fn(),
  updateInvite: vi.fn(),
  listInviteRedemptions: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    listInvites: apiMocks.listInvites,
    batchCreateInvites: apiMocks.batchCreateInvites,
    updateInvite: apiMocks.updateInvite,
    listInviteRedemptions: apiMocks.listInviteRedemptions
  }
}))

import AdminInvitesView from '../views/AdminInvitesView.vue'

const invitePayload = {
  items: [
    {
      id: 1,
      code: 'OPS-A1B2-C3D4',
      label: '运营后台',
      status: 'active',
      total_quota: 5,
      used_quota: 2,
      expires_at: '2026-05-31T00:00:00Z',
      notes: 'KOL 批量导入',
      created_at: '2026-05-01T08:00:00Z'
    },
    {
      id: 2,
      code: 'OPS-E5F6-G7H8',
      label: '活动页',
      status: 'active',
      total_quota: 1,
      used_quota: 1,
      expires_at: null,
      notes: '',
      created_at: '2026-05-01T09:00:00Z'
    }
  ],
  summary: {
    available_invites: 6,
    available_invites_delta_percent: 20,
    used_invites: 3,
    used_invites_delta_percent: 50,
    today_new_invite_users: 2,
    today_new_invite_users_delta_percent: 100,
    invite_conversion_rate: 25,
    invite_conversion_rate_delta_percent: -5
  },
  total: 2,
  page: 1,
  page_size: 10
}

const redemptionPayload = {
  items: [
    {
      id: 11,
      invite_code: 'OPS-A1B2-C3D4',
      inviter_name: '运营后台',
      user_id: 42,
      username: 'creator',
      display_name: '内容创作者',
      email: 'creator@example.com',
      registered_at: '2026-05-01T08:12:00Z',
      conversion_result: 'converted'
    }
  ],
  total: 1,
  page: 1,
  page_size: 10
}

describe('AdminInvitesView', () => {
  afterEach(() => {
    vi.clearAllMocks()
    vi.unstubAllGlobals()
  })

  it('renders invite KPIs, batch form, invite table and redemption table', async () => {
    apiMocks.listInvites.mockResolvedValue(invitePayload)
    apiMocks.listInviteRedemptions.mockResolvedValue(redemptionPayload)

    const wrapper = mount(AdminInvitesView)
    await flushPromises()

    expect(apiMocks.listInvites).toHaveBeenCalledWith(expect.objectContaining({
      page: 1,
      page_size: 10
    }))
    expect(apiMocks.listInviteRedemptions).toHaveBeenCalledWith(expect.objectContaining({
      page: 1,
      page_size: 10
    }))
    expect(wrapper.text()).toContain('邀请码管理')
    expect(wrapper.text()).toContain('后台管理中心 / 邀请码管理')
    expect(wrapper.text()).toContain('可用邀请码')
    expect(wrapper.text()).toContain('已使用')
    expect(wrapper.text()).toContain('今日新增邀请用户')
    expect(wrapper.text()).toContain('邀请转化率')
    expect(wrapper.text()).toContain('生成邀请码')
    expect(wrapper.text()).toContain('邀请码列表')
    expect(wrapper.text()).toContain('邀请记录')
    expect(wrapper.text()).toContain('OPS-A1B2-C3D4')
    expect(wrapper.text()).toContain('已过期')
    expect(wrapper.text()).toContain('内容创作者')
    expect(wrapper.text()).toContain('已转化')
    expect(wrapper.find('[data-testid="invites-pagination"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="redemptions-pagination"]').exists()).toBe(true)
  })

  it('applies filters, batch-generates, toggles status and copies codes', async () => {
    const writeText = vi.fn().mockResolvedValue()
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    apiMocks.listInvites.mockResolvedValue(invitePayload)
    apiMocks.listInviteRedemptions.mockResolvedValue(redemptionPayload)
    apiMocks.batchCreateInvites.mockResolvedValue({ items: [] })
    apiMocks.updateInvite.mockResolvedValue({ ok: true })

    const wrapper = mount(AdminInvitesView)
    await flushPromises()

    await wrapper.get('[data-testid="invite-prefix"]').setValue('VIP')
    await wrapper.get('[data-testid="invite-quantity"]').setValue(6)
    await wrapper.get('[data-testid="invite-total-quota"]').setValue(3)
    await wrapper.get('[data-testid="invite-expires-at"]').setValue('2026-06-01')
    await wrapper.get('[data-testid="invite-batch-form"]').trigger('submit')
    await flushPromises()

    expect(apiMocks.batchCreateInvites).toHaveBeenCalledWith({
      prefix: 'VIP',
      quantity: 6,
      expires_at: '2026-06-01T23:59:59+08:00',
      total_quota: 3
    })

    await wrapper.get('[data-testid="invite-search"]').setValue('OPS')
    await wrapper.get('[data-testid="invite-status"]').setValue('partial')
    await wrapper.get('[data-testid="invite-filter-form"]').trigger('submit')
    await flushPromises()

    expect(apiMocks.listInvites).toHaveBeenLastCalledWith(expect.objectContaining({
      q: 'OPS',
      status: 'partial',
      page: 1,
      page_size: 10
    }))

    await wrapper.get('[data-testid="invite-copy-1"]').trigger('click')
    await flushPromises()
    expect(writeText).toHaveBeenCalledWith('OPS-A1B2-C3D4')

    await wrapper.get('[data-testid="invite-toggle-1"]').trigger('click')
    await flushPromises()
    expect(apiMocks.updateInvite).toHaveBeenCalledWith(1, expect.objectContaining({
      status: 'disabled'
    }))

    await wrapper.get('[data-testid="redemption-result"]').setValue('converted')
    await wrapper.get('[data-testid="redemption-filter-form"]').trigger('submit')
    await flushPromises()

    expect(apiMocks.listInviteRedemptions).toHaveBeenLastCalledWith(expect.objectContaining({
      result: 'converted',
      page: 1,
      page_size: 10
    }))
  })
})
