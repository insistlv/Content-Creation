import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  getCurrentAdminSession: vi.fn(),
  listAdminUsers: vi.fn(),
  listAdminCreditTransactions: vi.fn(),
  addAdminCredits: vi.fn(),
  adjustAdminCredits: vi.fn(),
  resetAdminUserPassword: vi.fn(),
  updateAdminUserWechatBinding: vi.fn(),
  deleteAdminUserWechatBinding: vi.fn(),
  deleteAdminUserPhoneBinding: vi.fn(),
  deleteAdminUser: vi.fn(),
  batchDeleteAdminUsers: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    listAdminUsers: apiMocks.listAdminUsers,
    listAdminCreditTransactions: apiMocks.listAdminCreditTransactions,
    addAdminCredits: apiMocks.addAdminCredits,
    adjustAdminCredits: apiMocks.adjustAdminCredits,
    resetAdminUserPassword: apiMocks.resetAdminUserPassword,
    updateAdminUserWechatBinding: apiMocks.updateAdminUserWechatBinding,
    deleteAdminUserWechatBinding: apiMocks.deleteAdminUserWechatBinding,
    deleteAdminUserPhoneBinding: apiMocks.deleteAdminUserPhoneBinding,
    deleteAdminUser: apiMocks.deleteAdminUser,
    batchDeleteAdminUsers: apiMocks.batchDeleteAdminUsers
  },
  getCurrentAdminSession: apiMocks.getCurrentAdminSession
}))

import AdminUsersView from '../views/AdminUsersView.vue'

const userPayload = {
  items: [
    {
      user_id: 7,
      username: 'creator_one',
      account: 'creator_one',
      phone: '13800138001',
      display_name: 'Creator One',
      email: 'creator@example.com',
      avatar_url: '',
      status: 'active',
      online: true,
      wechat_bound: true,
      wechat_open_id: 'wx-existing-openid',
      wechat_binding: {
        bound: true,
        openid: 'wx-existing-openid'
      },
      available_credits: 36,
      total_recharged: 120,
      last_login_at: '2026-04-30T08:30:00Z',
      role: {
        id: 1,
        code: 'standard_user',
        name: '普通用户',
        color: 'blue'
      },
      created_at: '2026-04-30T09:00:00Z'
    }
  ],
  total: 1,
  page: 1,
  page_size: 10,
  summary: {
    users_total: 3,
    active_users: 2,
    online_users: 1,
    today_new_users: 1,
    total_credits: 84,
    total_manual_topup: 120,
    users_total_delta_percent: 12,
    active_users_delta_percent: 5,
    today_new_users_delta_percent: -20,
    total_credits_delta_percent: 8,
    users_total_sparkline: [1, 1, 2, 2, 3, 3, 3],
    active_users_sparkline: [1, 1, 2, 2, 2, 2, 2],
    today_new_users_sparkline: [0, 0, 1, 0, 1, 0, 1],
    total_credits_sparkline: [0, 12, 24, 40, 60, 72, 84]
  }
}

const secondUser = {
  user_id: 9,
  username: 'team_admin',
  account: 'team_admin',
  phone: '13900139001',
  display_name: 'Team Admin',
  email: 'admin@example.com',
  avatar_url: '',
  status: 'active',
  online: false,
  wechat_bound: false,
  wechat_open_id: '',
  wechat_binding: {
    bound: false,
    openid: ''
  },
  available_credits: 12,
  total_recharged: 80,
  last_login_at: '2026-04-29T08:30:00Z',
  role: {
    id: 2,
    code: 'standard_admin',
    name: '普通管理员',
    color: 'purple'
  },
  created_at: '2026-04-29T09:00:00Z'
}

const multiUserPayload = {
  ...userPayload,
  items: [...userPayload.items, secondUser],
  total: 2
}

const transactionsPayload = {
  items: [
    {
      id: 11,
      user_id: 7,
      username: 'creator_one',
      type: 'manual_topup',
      amount: 12,
      balance_after: 36,
      admin_note: '补发活动点数',
      created_at: '2026-04-30T10:00:00Z'
    },
    {
      id: 12,
      user_id: 9,
      username: 'team_admin',
      type: 'manual_deduct',
      amount: -6,
      balance_after: 12,
      admin_note: '其他用户流水',
      created_at: '2026-04-30T11:00:00Z'
    }
  ],
  total: 2,
  page: 1,
  page_size: 8
}

function mockSuccessfulLoad(payload = userPayload) {
  apiMocks.getCurrentAdminSession.mockResolvedValue({
    admin: { id: 1, username: 'admin' },
    permissions: ['users.read', 'users.update', 'users.delete', 'users.credits.add', 'users.password.reset'],
    menus: []
  })
  apiMocks.listAdminUsers.mockResolvedValue(payload)
  apiMocks.listAdminCreditTransactions.mockResolvedValue(transactionsPayload)
}

describe('AdminUsersView', () => {
  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
    vi.clearAllMocks()
  })

  it('loads users, summary cards, and credit transactions from the admin APIs', async () => {
    mockSuccessfulLoad()

    const wrapper = mount(AdminUsersView)
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(apiMocks.listAdminUsers).toHaveBeenCalledWith({
      page: 1,
      page_size: 10,
      q: '',
      role: '',
      status: 'all'
    })
    expect(apiMocks.listAdminCreditTransactions).toHaveBeenCalledWith({ page: 1, page_size: 8 })
    expect(wrapper.text()).toContain('用户总数')
    expect(wrapper.text()).toContain('实时在线')
    expect(wrapper.text()).toContain('近 5 分钟')
    expect(wrapper.text()).not.toContain('活跃用户')
    expect(wrapper.text()).toContain('3')
    expect(wrapper.text()).toContain('creator_one')
    expect(wrapper.text()).toContain('13800138001')
    expect(wrapper.text()).toContain('普通用户')
    expect(wrapper.text()).toContain('在线')
    expect(wrapper.text()).toContain('已绑定')
    expect(wrapper.text()).toContain('120')
    expect(wrapper.find('.admin-side-stack').exists()).toBe(false)
    expect(wrapper.find('[data-testid="admin-user-detail-modal"]').exists()).toBe(false)
  })

  it('renders KPI sparklines as accessible SVG line charts with point titles', async () => {
    mockSuccessfulLoad()

    const wrapper = mount(AdminUsersView)
    await flushPromises()
    await wrapper.vm.$nextTick()

    const charts = wrapper.findAll('.users-kpi-card .users-sparkline svg')

    expect(charts).toHaveLength(4)
    expect(wrapper.findAll('.users-kpi-card .users-sparkline i')).toHaveLength(0)

    charts.forEach((chart) => {
      expect(chart.attributes('role')).toBe('img')
      expect(chart.attributes('aria-label')).toMatch(/最近 7 日趋势/)
      expect(chart.findAll('.users-sparkline-point')).toHaveLength(7)
      expect(chart.findAll('title')).toHaveLength(7)
    })

    expect(charts[0].find('title').text()).toBe('用户总数 第 1 天：1')
    expect(charts[3].findAll('title')[6].text()).toBe('剩余总点数 第 7 天：84')
  })

  it('keeps all-zero KPI sparklines stable without NaN coordinates', async () => {
    mockSuccessfulLoad({
      ...userPayload,
      summary: {
        ...userPayload.summary,
        users_total_sparkline: [0, 0, 0, 0, 0, 0, 0],
        active_users_sparkline: [0, 0, 0, 0, 0, 0, 0],
        today_new_users_sparkline: [0, 0, 0, 0, 0, 0, 0],
        total_credits_sparkline: [0, 0, 0, 0, 0, 0, 0]
      }
    })

    const wrapper = mount(AdminUsersView)
    await flushPromises()
    await wrapper.vm.$nextTick()

    const charts = wrapper.findAll('.users-kpi-card .users-sparkline svg')

    expect(charts).toHaveLength(4)
    charts.forEach((chart) => {
      const markup = chart.html()
      expect(markup).not.toContain('NaN')
      expect(chart.get('.users-sparkline-line').attributes('points')).not.toContain('NaN')
      expect(chart.findAll('.users-sparkline-point')).toHaveLength(7)
    })
  })

  it('opens a user detail modal from the row action with user information and filtered credit transactions', async () => {
    mockSuccessfulLoad(multiUserPayload)

    const wrapper = mount(AdminUsersView)
    await flushPromises()

    expect(wrapper.find('.admin-side-stack').exists()).toBe(false)
    expect(wrapper.find('[aria-label="调整点数"]').exists()).toBe(false)

    await wrapper.get('[data-testid="view-user-7"]').trigger('click')
    await wrapper.vm.$nextTick()

    const modal = wrapper.get('[data-testid="admin-user-detail-modal"]')
    expect(modal.text()).toContain('Creator One')
    expect(modal.text()).toContain('#7')
    expect(modal.text()).toContain('creator_one')
    expect(modal.text()).toContain('13800138001')
    expect(modal.text()).toContain('creator@example.com')
    expect(modal.text()).toContain('普通用户')
    expect(modal.text()).toContain('正常')
    expect(modal.text()).toContain('36')
    expect(modal.text()).toContain('120')
    expect(modal.text()).toContain('wx-existing-openid')
    expect(modal.text()).toContain('补发活动点数')
    expect(modal.text()).not.toContain('其他用户流水')
    expect(wrapper.find('[data-testid="credit-user-select"]').exists()).toBe(false)
  })

  it('submits search, role, and status filters to the user API', async () => {
    mockSuccessfulLoad()
    apiMocks.listAdminUsers.mockResolvedValueOnce(userPayload).mockResolvedValueOnce({
      ...userPayload,
      items: []
    })

    const wrapper = mount(AdminUsersView)
    await flushPromises()

    expect(wrapper.get('[data-testid="admin-users-search"]').attributes('placeholder')).toBe('搜索用户、账号、手机号或邮箱')

    await wrapper.get('[data-testid="admin-users-search"]').setValue('13800138001')
    await wrapper.get('[data-testid="admin-users-role"]').setValue('standard_user')
    await wrapper.get('[data-testid="admin-users-status"]').setValue('online')
    await wrapper.get('[data-testid="admin-users-filter"]').trigger('submit')
    await flushPromises()

    expect(apiMocks.listAdminUsers).toHaveBeenLastCalledWith({
      page: 1,
      page_size: 10,
      q: '13800138001',
      role: 'standard_user',
      status: 'online'
    })
  })

  it('sorts highlighted columns and keeps sort parameters across pagination and auto refresh', async () => {
    vi.useFakeTimers()
    const pagedPayload = { ...multiUserPayload, total: 20 }
    mockSuccessfulLoad(pagedPayload)
    apiMocks.listAdminUsers.mockImplementation(async (params = {}) => ({
      ...pagedPayload,
      page: params.page ?? 1
    }))

    const wrapper = mount(AdminUsersView)
    await flushPromises()

    expect(apiMocks.listAdminUsers).toHaveBeenNthCalledWith(1, {
      page: 1,
      page_size: 10,
      q: '',
      role: '',
      status: 'all'
    })

    const creditsSort = wrapper.get('[data-testid="admin-users-sort-available-credits"]')
    const totalSort = wrapper.get('[data-testid="admin-users-sort-total-recharged"]')
    expect(creditsSort.attributes('aria-sort')).toBe('none')

    await creditsSort.trigger('click')
    await flushPromises()

    expect(apiMocks.listAdminUsers).toHaveBeenLastCalledWith({
      page: 1,
      page_size: 10,
      q: '',
      role: '',
      status: 'all',
      sort_by: 'available_credits',
      sort_dir: 'desc'
    })
    expect(wrapper.get('[data-testid="admin-users-sort-available-credits"]').attributes('aria-sort')).toBe('descending')
    expect(wrapper.get('[data-testid="admin-users-sort-available-credits"]').classes()).toContain('active')
    expect(totalSort.classes()).not.toContain('active')

    await wrapper.get('[data-testid="admin-users-sort-available-credits"]').trigger('click')
    await flushPromises()

    expect(apiMocks.listAdminUsers).toHaveBeenLastCalledWith({
      page: 1,
      page_size: 10,
      q: '',
      role: '',
      status: 'all',
      sort_by: 'available_credits',
      sort_dir: 'asc'
    })
    expect(wrapper.get('[data-testid="admin-users-sort-available-credits"]').attributes('aria-sort')).toBe('ascending')

    await wrapper.get('.admin-pagination .mini-button:last-child').trigger('click')
    await flushPromises()

    expect(apiMocks.listAdminUsers).toHaveBeenLastCalledWith({
      page: 2,
      page_size: 10,
      q: '',
      role: '',
      status: 'all',
      sort_by: 'available_credits',
      sort_dir: 'asc'
    })

    const callsBeforeRefresh = apiMocks.listAdminUsers.mock.calls.length
    await vi.advanceTimersByTimeAsync(60_000)

    expect(apiMocks.listAdminUsers).toHaveBeenCalledTimes(callsBeforeRefresh + 1)
    expect(apiMocks.listAdminUsers).toHaveBeenLastCalledWith({
      page: 2,
      page_size: 10,
      q: '',
      role: '',
      status: 'all',
      sort_by: 'available_credits',
      sort_dir: 'asc'
    })

    wrapper.unmount()
    vi.useRealTimers()
  })

  it('auto refreshes visible admin users every 60 seconds with the current filters and page', async () => {
    vi.useFakeTimers()
    const pagedPayload = { ...multiUserPayload, total: 20 }
    mockSuccessfulLoad(pagedPayload)
    apiMocks.listAdminUsers
      .mockResolvedValueOnce({ ...pagedPayload, page: 1 })
      .mockResolvedValueOnce({ ...pagedPayload, page: 1 })
      .mockResolvedValueOnce({ ...pagedPayload, page: 2 })

    const wrapper = mount(AdminUsersView)
    await flushPromises()

    await wrapper.get('[data-testid="admin-users-search"]').setValue('creator')
    await wrapper.get('[data-testid="admin-users-status"]').setValue('online')
    await wrapper.get('[data-testid="admin-users-filter"]').trigger('submit')
    await flushPromises()
    await wrapper.get('.admin-pagination .mini-button:last-child').trigger('click')
    await flushPromises()

    const callsBeforeRefresh = apiMocks.listAdminUsers.mock.calls.length
    await vi.advanceTimersByTimeAsync(60_000)

    expect(apiMocks.listAdminUsers).toHaveBeenCalledTimes(callsBeforeRefresh + 1)
    expect(apiMocks.listAdminUsers).toHaveBeenLastCalledWith({
      page: 2,
      page_size: 10,
      q: 'creator',
      role: '',
      status: 'online'
    })

    wrapper.unmount()
    vi.useRealTimers()
  })

  it('deletes a single user after confirmation and refreshes the list', async () => {
    mockSuccessfulLoad()
    apiMocks.deleteAdminUser.mockResolvedValue({ ok: true })
    vi.spyOn(window, 'confirm').mockReturnValue(true)

    const wrapper = mount(AdminUsersView)
    await flushPromises()

    await wrapper.get('[data-testid="delete-user-7"]').trigger('click')
    await flushPromises()

    expect(window.confirm).toHaveBeenCalledWith('删除用户「Creator One」？删除后该用户将无法登录。')
    expect(apiMocks.deleteAdminUser).toHaveBeenCalledWith(7)
    expect(apiMocks.listAdminUsers).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('用户已删除')
  })

  it('does not delete a user when confirmation is cancelled', async () => {
    mockSuccessfulLoad()
    vi.spyOn(window, 'confirm').mockReturnValue(false)

    const wrapper = mount(AdminUsersView)
    await flushPromises()

    await wrapper.get('[data-testid="delete-user-7"]').trigger('click')
    await flushPromises()

    expect(apiMocks.deleteAdminUser).not.toHaveBeenCalled()
    expect(apiMocks.listAdminUsers).toHaveBeenCalledTimes(1)
  })

  it('renders the user selection column without entering bulk mode', async () => {
    mockSuccessfulLoad()

    const wrapper = mount(AdminUsersView)
    await flushPromises()

    expect(wrapper.find('[data-testid="toggle-bulk-users"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="select-visible-users"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="select-user-7"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="users-bulk-toolbar"]').exists()).toBe(false)
  })

  it('selects and unselects visible users from the header checkbox', async () => {
    mockSuccessfulLoad(multiUserPayload)

    const wrapper = mount(AdminUsersView)
    await flushPromises()

    await wrapper.get('[data-testid="select-visible-users"]').setValue(true)
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="select-user-7"]').element.checked).toBe(true)
    expect(wrapper.get('[data-testid="select-user-9"]').element.checked).toBe(true)
    expect(wrapper.get('[data-testid="users-bulk-toolbar"]').text()).toContain('已选择 2 个用户')

    await wrapper.get('[data-testid="select-visible-users"]').setValue(false)
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="select-user-7"]').element.checked).toBe(false)
    expect(wrapper.get('[data-testid="select-user-9"]').element.checked).toBe(false)
    expect(wrapper.find('[data-testid="users-bulk-toolbar"]').exists()).toBe(false)
  })

  it('does not batch delete users when confirmation is cancelled', async () => {
    mockSuccessfulLoad()
    vi.spyOn(window, 'confirm').mockReturnValue(false)

    const wrapper = mount(AdminUsersView)
    await flushPromises()

    await wrapper.get('[data-testid="select-user-7"]').setValue(true)
    await wrapper.get('[data-testid="bulk-delete-users"]').trigger('click')
    await flushPromises()

    expect(window.confirm).toHaveBeenCalledWith('删除选中的 1 个用户？删除后这些用户将无法登录。')
    expect(apiMocks.batchDeleteAdminUsers).not.toHaveBeenCalled()
    expect(apiMocks.listAdminUsers).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[data-testid="select-user-7"]').element.checked).toBe(true)
  })

  it('batch deletes only selected users and clears selection after refresh', async () => {
    mockSuccessfulLoad()
    apiMocks.batchDeleteAdminUsers.mockResolvedValue({ ok: true, deleted_count: 1 })
    vi.spyOn(window, 'confirm').mockReturnValue(true)

    const wrapper = mount(AdminUsersView)
    await flushPromises()

    await wrapper.get('[data-testid="select-user-7"]').setValue(true)
    expect(wrapper.get('[data-testid="users-bulk-toolbar"]').text()).toContain('已选择 1 个用户')

    await wrapper.get('[data-testid="bulk-delete-users"]').trigger('click')
    await flushPromises()

    expect(window.confirm).toHaveBeenCalledWith('删除选中的 1 个用户？删除后这些用户将无法登录。')
    expect(apiMocks.batchDeleteAdminUsers).toHaveBeenCalledWith([7])
    expect(apiMocks.listAdminUsers).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('已批量删除用户')
    expect(wrapper.find('[data-testid="select-user-7"]').element.checked).toBe(false)
  })

  it('adds credits, closes the detail modal, shows a toast, and refreshes users and transactions', async () => {
    mockSuccessfulLoad()
    apiMocks.adjustAdminCredits.mockResolvedValue({ user_id: 7, available_credits: 48 })

    const wrapper = mount(AdminUsersView)
    await flushPromises()

    await wrapper.get('[data-testid="view-user-7"]').trigger('click')
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="credit-amount"]').setValue('12')
    await wrapper.get('[data-testid="credit-note"]').setValue('补发活动点数')
    await wrapper.get('[data-testid="credit-adjustment-form"]').trigger('submit')
    await flushPromises()

    expect(apiMocks.adjustAdminCredits).toHaveBeenCalledWith(7, {
      type: 'add',
      amount: 12,
      note: '补发活动点数'
    })
    expect(apiMocks.listAdminUsers).toHaveBeenCalledTimes(2)
    expect(apiMocks.listAdminCreditTransactions).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-testid="admin-user-detail-modal"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="credit-adjustment-message"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="credit-success-dialog"]').exists()).toBe(false)

    const toast = wrapper.get('[data-testid="credit-adjustment-toast"]')
    expect(toast.attributes('role')).toBe('status')
    expect(toast.attributes('aria-live')).toBe('polite')
    expect(toast.text()).toBe('加点成功！Creator One 当前剩余 48 点')
  })

  it('deducts credits, closes the detail modal, and shows a deduct success toast', async () => {
    mockSuccessfulLoad()
    apiMocks.adjustAdminCredits.mockResolvedValue({ user_id: 7, available_credits: 31 })

    const wrapper = mount(AdminUsersView)
    await flushPromises()

    await wrapper.get('[data-testid="view-user-7"]').trigger('click')
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="credit-tab-deduct"]').trigger('click')
    await wrapper.get('[data-testid="credit-amount"]').setValue('5')
    await wrapper.get('[data-testid="credit-note"]').setValue('人工扣减')
    await wrapper.get('[data-testid="credit-adjustment-form"]').trigger('submit')
    await flushPromises()

    expect(apiMocks.adjustAdminCredits).toHaveBeenLastCalledWith(7, {
      type: 'deduct',
      amount: 5,
      note: '人工扣减'
    })
    expect(wrapper.find('[data-testid="admin-user-detail-modal"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="credit-adjustment-message"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="credit-success-dialog"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="credit-adjustment-toast"]').text()).toBe('扣点成功，Creator One 当前剩余 31 点')
  })

  it('hides the credit adjustment toast after 2.5 seconds', async () => {
    vi.useFakeTimers()
    mockSuccessfulLoad()
    apiMocks.adjustAdminCredits.mockResolvedValue({ user_id: 7, available_credits: 48 })

    const wrapper = mount(AdminUsersView)
    await flushPromises()

    await wrapper.get('[data-testid="view-user-7"]').trigger('click')
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="credit-amount"]').setValue('12')
    await wrapper.get('[data-testid="credit-adjustment-form"]').trigger('submit')
    await flushPromises()

    expect(wrapper.find('[data-testid="credit-adjustment-toast"]').exists()).toBe(true)

    await vi.advanceTimersByTimeAsync(2500)
    await wrapper.vm.$nextTick()

    expect(wrapper.find('[data-testid="credit-adjustment-toast"]').exists()).toBe(false)
    wrapper.unmount()
    vi.useRealTimers()
  })

  it('disables the credit submit button and shows processing text while submitting', async () => {
    mockSuccessfulLoad()
    let resolveAdjustment
    apiMocks.adjustAdminCredits.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveAdjustment = resolve
        })
    )

    const wrapper = mount(AdminUsersView)
    await flushPromises()

    await wrapper.get('[data-testid="view-user-7"]').trigger('click')
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="credit-amount"]').setValue('12')
    await wrapper.get('[data-testid="credit-adjustment-form"]').trigger('submit')
    await wrapper.vm.$nextTick()

    const submitButton = wrapper.get('[data-testid="credit-adjustment-submit"]')
    expect(submitButton.element.disabled).toBe(true)
    expect(submitButton.attributes('aria-busy')).toBe('true')
    expect(submitButton.text()).toBe('处理中...')

    resolveAdjustment({ user_id: 7, available_credits: 48 })
    await flushPromises()
  })

  it('shows invalid credit amount errors inside the credit adjustment form', async () => {
    mockSuccessfulLoad()

    const wrapper = mount(AdminUsersView)
    await flushPromises()

    await wrapper.get('[data-testid="view-user-7"]').trigger('click')
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="credit-amount"]').setValue('0')
    await wrapper.get('[data-testid="credit-adjustment-form"]').trigger('submit')
    await wrapper.vm.$nextTick()

    const error = wrapper.get('[data-testid="credit-adjustment-error"]')
    expect(error.attributes('role')).toBe('alert')
    expect(error.text()).toBe('请选择用户并输入有效点数')
    expect(wrapper.find('.admin-users-page > .status-error').exists()).toBe(false)
    expect(wrapper.find('[data-testid="admin-user-detail-modal"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="credit-adjustment-toast"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="credit-success-dialog"]').exists()).toBe(false)
  })

  it('keeps the detail modal open and shows the form error when credit adjustment fails', async () => {
    mockSuccessfulLoad()
    apiMocks.adjustAdminCredits.mockRejectedValue(new Error('点数调整失败'))

    const wrapper = mount(AdminUsersView)
    await flushPromises()

    await wrapper.get('[data-testid="view-user-7"]').trigger('click')
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="credit-amount"]').setValue('12')
    await wrapper.get('[data-testid="credit-adjustment-form"]').trigger('submit')
    await flushPromises()

    expect(wrapper.get('[data-testid="credit-adjustment-error"]').text()).toBe('点数调整失败')
    expect(wrapper.find('[data-testid="admin-user-detail-modal"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="credit-adjustment-toast"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="credit-success-dialog"]').exists()).toBe(false)
  })

  it('updates and unbinds a user WeChat binding from the users page', async () => {
    mockSuccessfulLoad()
    apiMocks.updateAdminUserWechatBinding.mockResolvedValue({
      user_id: 7,
      wechat_bound: true,
      wechat_open_id: 'wx-updated-openid'
    })
    apiMocks.deleteAdminUserWechatBinding.mockResolvedValue({
      user_id: 7,
      wechat_bound: false,
      wechat_open_id: ''
    })

    const wrapper = mount(AdminUsersView)
    await flushPromises()

    await wrapper.get('[data-testid="view-user-7"]').trigger('click')
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="wechat-binding-openid"]').element.value).toBe('wx-existing-openid')

    await wrapper.get('[data-testid="wechat-binding-openid"]').setValue('wx-updated-openid')
    await wrapper.get('[data-testid="wechat-binding-note"]').setValue('客服核验后修正')
    await wrapper.get('[data-testid="wechat-binding-form"]').trigger('submit')
    await flushPromises()

    expect(apiMocks.updateAdminUserWechatBinding).toHaveBeenCalledWith(7, {
      openid: 'wx-updated-openid',
      note: '客服核验后修正'
    })

    await wrapper.get('[data-testid="wechat-binding-note"]').setValue('用户要求解绑')
    await wrapper.get('[data-testid="wechat-binding-unbind"]').trigger('click')
    await flushPromises()

    expect(apiMocks.deleteAdminUserWechatBinding).toHaveBeenCalledWith(7, {
      note: '用户要求解绑'
    })
    expect(apiMocks.listAdminUsers).toHaveBeenCalledTimes(3)
  })

  it('shows password reset actions only for admins with reset permission', async () => {
    mockSuccessfulLoad()

    const wrapper = mount(AdminUsersView)
    await flushPromises()

    expect(wrapper.get('[data-testid="reset-user-password-7"]').exists()).toBe(true)

    await wrapper.get('[data-testid="reset-user-password-7"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="user-password-reset-form"]').exists()).toBe(true)

    wrapper.unmount()
    vi.clearAllMocks()
    mockSuccessfulLoad()
    apiMocks.getCurrentAdminSession.mockResolvedValue({
      admin: { id: 1, username: 'auditor' },
      permissions: ['users.read'],
      menus: []
    })

    const limitedWrapper = mount(AdminUsersView)
    await flushPromises()

    expect(limitedWrapper.find('[data-testid="reset-user-password-7"]').exists()).toBe(false)
    await limitedWrapper.get('[data-testid="view-user-7"]').trigger('click')
    await limitedWrapper.vm.$nextTick()
    expect(limitedWrapper.find('[data-testid="user-password-reset-form"]').exists()).toBe(false)
  })

  it('validates and submits user password reset from the detail modal', async () => {
    mockSuccessfulLoad()
    apiMocks.resetAdminUserPassword.mockResolvedValue({ ok: true })

    const wrapper = mount(AdminUsersView)
    await flushPromises()

    await wrapper.get('[data-testid="reset-user-password-7"]').trigger('click')
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="user-reset-password-new"]').setValue('short')
    await wrapper.get('[data-testid="user-reset-password-confirm"]').setValue('short')
    await wrapper.get('[data-testid="user-password-reset-form"]').trigger('submit')
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('新密码至少 8 位')
    expect(apiMocks.resetAdminUserPassword).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="user-reset-password-new"]').setValue('NewPass456')
    await wrapper.get('[data-testid="user-reset-password-confirm"]').setValue('OtherPass456')
    await wrapper.get('[data-testid="user-password-reset-form"]').trigger('submit')
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('两次输入的新密码不一致')
    expect(apiMocks.resetAdminUserPassword).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="user-reset-password-confirm"]').setValue('NewPass456')
    await wrapper.get('[data-testid="user-password-reset-form"]').trigger('submit')
    await flushPromises()

    expect(apiMocks.resetAdminUserPassword).toHaveBeenCalledWith(7, { password: 'NewPass456' })
    expect(wrapper.get('[data-testid="user-reset-password-new"]').element.value).toBe('')
    expect(wrapper.get('[data-testid="user-reset-password-confirm"]').element.value).toBe('')
    expect(wrapper.text()).toContain('密码已重置，用户需要重新登录')
  })

  it('shows API errors from user password reset submissions', async () => {
    mockSuccessfulLoad()
    apiMocks.resetAdminUserPassword.mockRejectedValue(new Error('reset_failed'))

    const wrapper = mount(AdminUsersView)
    await flushPromises()

    await wrapper.get('[data-testid="reset-user-password-7"]').trigger('click')
    await wrapper.get('[data-testid="user-reset-password-new"]').setValue('NewPass456')
    await wrapper.get('[data-testid="user-reset-password-confirm"]').setValue('NewPass456')
    await wrapper.get('[data-testid="user-password-reset-form"]').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('reset_failed')
  })

  it('unbinds a user phone from the detail modal after confirmation and refreshes the detail data', async () => {
    apiMocks.listAdminUsers
      .mockResolvedValueOnce(userPayload)
      .mockResolvedValueOnce({
        ...userPayload,
        items: [{ ...userPayload.items[0], phone: null }]
      })
    apiMocks.listAdminCreditTransactions.mockResolvedValue(transactionsPayload)
    apiMocks.deleteAdminUserPhoneBinding.mockResolvedValue({
      user_id: 7,
      phone: null,
      phone_bound: false
    })
    vi.spyOn(window, 'confirm').mockReturnValue(true)

    const wrapper = mount(AdminUsersView)
    await flushPromises()

    await wrapper.get('[data-testid="view-user-7"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="phone-binding-unbind"]').exists()).toBe(true)
    await wrapper.get('[data-testid="phone-binding-unbind"]').trigger('click')
    await flushPromises()

    expect(window.confirm).toHaveBeenCalledWith('解绑用户「Creator One」的手机号 13800138001？解绑后该用户将无法使用手机号登录。')
    expect(apiMocks.deleteAdminUserPhoneBinding).toHaveBeenCalledWith(7, {
      note: '后台解绑手机号'
    })
    expect(apiMocks.listAdminUsers).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[data-testid="admin-user-detail-modal"]').text()).toContain('未绑定手机号')
  })

  it('hides the detail phone unbind action for users without a phone', async () => {
    mockSuccessfulLoad({
      ...userPayload,
      items: [{ ...userPayload.items[0], phone: null }]
    })

    const wrapper = mount(AdminUsersView)
    await flushPromises()

    await wrapper.get('[data-testid="view-user-7"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="admin-user-detail-modal"]').text()).toContain('未绑定手机号')
    expect(wrapper.find('[data-testid="phone-binding-unbind"]').exists()).toBe(false)
  })

  it('opens the same detail modal from the WeChat binding status and closes it without affecting table selection', async () => {
    mockSuccessfulLoad()

    const wrapper = mount(AdminUsersView)
    await flushPromises()

    await wrapper.get('[data-testid="open-wechat-binding-7"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="admin-user-detail-modal"]').text()).toContain('微信绑定')

    await wrapper.get('[data-testid="close-user-detail-modal"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.find('[data-testid="admin-user-detail-modal"]').exists()).toBe(false)

    await wrapper.get('[data-testid="select-user-7"]').setValue(true)
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="users-bulk-toolbar"]').text()).toContain('已选择 1 个用户')
  })

  it('shows the API error message when loading fails', async () => {
    apiMocks.listAdminUsers.mockRejectedValueOnce(new Error('users_load_failed'))
    apiMocks.listAdminCreditTransactions.mockResolvedValueOnce({ items: [], total: 0, page: 1, page_size: 8 })

    const wrapper = mount(AdminUsersView)
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('users_load_failed')
  })
})
