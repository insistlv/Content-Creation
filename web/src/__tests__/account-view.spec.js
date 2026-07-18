import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const routerPush = vi.hoisted(() => vi.fn())
const stylesPath = resolve(process.cwd(), 'src/styles.css')
const readStyles = () => readFileSync(stylesPath, 'utf8').replace(/\r\n/g, '\n')
const cssRuleFor = (styles, selector) => {
  const escapedSelector = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const matches = [...styles.matchAll(new RegExp(`(?:^|\\n)${escapedSelector}\\s*\\{([^}]*)\\}`, 'g'))]
  return matches.at(-1)?.[1] ?? ''
}
const apiMocks = vi.hoisted(() => ({
  getMe: vi.fn(),
  getCredits: vi.fn(),
  getCreditTransactions: vi.fn(),
  sendSMSCode: vi.fn(),
  bindAccountPhone: vi.fn(),
  unbindAccountPhone: vi.fn(),
  updateProfile: vi.fn(),
  updateAccountEmail: vi.fn(),
  updateAccountPreferences: vi.fn(),
  changePassword: vi.fn(),
  logout: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    getMe: apiMocks.getMe,
    getCredits: apiMocks.getCredits,
    getCreditTransactions: apiMocks.getCreditTransactions,
    sendSMSCode: apiMocks.sendSMSCode,
    bindAccountPhone: apiMocks.bindAccountPhone,
    unbindAccountPhone: apiMocks.unbindAccountPhone,
    updateProfile: apiMocks.updateProfile,
    updateAccountEmail: apiMocks.updateAccountEmail,
    updateAccountPreferences: apiMocks.updateAccountPreferences,
    changePassword: apiMocks.changePassword,
    logout: apiMocks.logout
  }
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({
    query: {}
  }),
  useRouter: () => ({
    push: routerPush
  })
}))

import AccountView from '../views/AccountView.vue'
import { clearCurrentUser, currentUser } from '../stores/session.js'

const mePayload = {
  user_id: 7,
  username: 'admin',
  display_name: 'admin',
  email: '',
  status: 'active',
  available_credits: 38,
  login_notification_enabled: true,
  risk_notification_enabled: true,
  created_at: '2026-04-20T10:15:00Z',
  updated_at: '2026-04-30T01:04:11Z'
}

const creditsPayload = {
  user_id: 7,
  available_credits: 38,
  monthly_consumption: 12,
  total_recharged: 80,
  latest_transaction_at: '2026-04-29T08:00:00Z'
}

const transactionPayload = {
  total: 2,
  page: 1,
  page_size: 10,
  has_more: false,
  items: [
    {
      id: 1,
      type: 'generation_charge',
      amount: -1,
      balance_after: 38,
      reason: '图片生成扣点',
      created_at: '2026-04-30T01:04:11Z'
    },
    {
      id: 2,
      type: 'manual_topup',
      amount: 20,
      balance_after: 40,
      reason: '后台人工充值',
      created_at: '2026-04-28T03:09:00Z'
    }
  ]
}

function transactionPage(items, overrides = {}) {
  return {
    items,
    total: items.length,
    page: 1,
    page_size: 10,
    has_more: false,
    ...overrides
  }
}

function mockAccountLoad(overrides = {}) {
  apiMocks.getMe.mockResolvedValueOnce({ ...mePayload, ...(overrides.me ?? {}) })
  apiMocks.getCredits.mockResolvedValueOnce({ ...creditsPayload, ...(overrides.credits ?? {}) })
  apiMocks.getCreditTransactions.mockResolvedValueOnce(overrides.transactions ?? transactionPayload)
}

describe('AccountView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    clearCurrentUser()
    routerPush.mockReset()
  })

  it('renders the screenshot-style account center with real account data', async () => {
    mockAccountLoad()

    const wrapper = mount(AccountView)
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(wrapper.find('[data-testid="account-side-panel"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="account-tab-profile"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="account-profile-section"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="account-security-section"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="account-credits-section"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="account-support-section"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('退出登录')
    expect(wrapper.text()).toContain('个人资料')
    expect(wrapper.text()).toContain('安全与绑定中心')
    expect(wrapper.text()).toContain('点数明细与套餐')
    expect(wrapper.get('#security').exists()).toBe(true)
    expect(wrapper.text()).toContain('帮助与支持')
    expect(wrapper.text()).toContain('admin')
    expect(wrapper.text()).toContain('当前余额')
    expect(wrapper.text()).toContain('38')
    expect(wrapper.text()).toContain('12')
    expect(wrapper.text()).toContain('80')
    expect(wrapper.text()).toContain('2026/04/20')
    expect(wrapper.text()).toContain('图片生成扣点')
    expect(wrapper.text()).toContain('后台人工充值')
    expect(currentUser.value?.available_credits).toBe(38)
    expect(apiMocks.getCreditTransactions).toHaveBeenCalledWith({ page: 1, page_size: 10 })
  })

  it('defines account theme tokens for dark and light shells', () => {
    const styles = readStyles()

    expect(styles).toContain('.user-dark-shell .account-center-page')
    expect(styles).toContain('.user-light-shell .account-center-page')
    expect(styles).toContain('--account-page-card-bg')
    expect(styles).toContain('--account-page-muted-bg')
    expect(styles).toContain('--account-page-modal-bg')
    expect(styles).toContain('--account-page-toggle-track')
    expect(styles).toContain('.account-settings-list')
    expect(styles).toContain('.account-modal')
    expect(styles).toContain('.account-credit-stats article')

    expect(cssRuleFor(styles, '.account-settings-list')).toContain('background: var(--account-page-muted-bg)')
    expect(cssRuleFor(styles, '.account-setting-row')).toContain('background: var(--account-page-row-bg)')
    expect(cssRuleFor(styles, '.account-modal')).toContain('background: var(--account-page-modal-bg)')
    expect(cssRuleFor(styles, '.account-credit-stats article')).toContain('background: var(--account-page-stat-bg)')
  })

  it('keeps the main ledger compact and opens full history in a modal', async () => {
    const items = Array.from({ length: 10 }, (_, index) => ({
      id: index + 1,
      type: index % 2 === 0 ? 'generation_charge' : 'manual_topup',
      amount: index % 2 === 0 ? -1 : 20,
      balance_after: 100 - index,
      reason: `流水 ${index + 1}`,
      created_at: `2026-04-${String(30 - index).padStart(2, '0')}T01:04:11Z`
    }))
    mockAccountLoad({
      transactions: transactionPage(items, { total: 27, page: 1, page_size: 10, has_more: true })
    })

    const wrapper = mount(AccountView)
    await flushPromises()

    expect(wrapper.findAll('[data-testid^="account-recent-transaction-"]')).toHaveLength(8)
    expect(wrapper.text()).not.toContain('流水 9')
    expect(wrapper.find('[data-testid="account-history-modal"]').exists()).toBe(false)

    await wrapper.get('[data-testid="account-open-history"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="account-history-modal"]').text()).toContain('全部历史记录')
    expect(wrapper.get('[data-testid="account-history-modal"]').text()).toContain('流水 9')
    expect(wrapper.get('[data-testid="account-ledger-range"]').text()).toContain('第 1-10 条 / 共 27 条')
  })

  it('saves changed profile name and account email through one profile action', async () => {
    mockAccountLoad({ me: { email: 'old@example.com' } })
    apiMocks.updateProfile.mockResolvedValueOnce({
      ...mePayload,
      display_name: '视觉主理人',
      email: 'old@example.com'
    })
    apiMocks.updateAccountEmail.mockResolvedValueOnce({
      ...mePayload,
      display_name: '视觉主理人',
      email: 'creator@example.com'
    })

    const wrapper = mount(AccountView)
    await flushPromises()

    await wrapper.get('[data-testid="account-edit-profile"]').trigger('click')
    expect(wrapper.get('[data-testid="account-profile-modal"]').exists()).toBe(true)
    await wrapper.get('[data-testid="account-display-name-input"]').setValue('视觉主理人')
    await wrapper.get('[data-testid="account-email-input"]').setValue('creator@example.com')
    await wrapper.get('[data-testid="account-save-profile-fields"]').trigger('click')
    await flushPromises()

    expect(apiMocks.updateProfile).toHaveBeenCalledTimes(1)
    expect(apiMocks.updateProfile).toHaveBeenCalledWith({ display_name: '视觉主理人' })
    expect(apiMocks.updateAccountEmail).toHaveBeenCalledTimes(1)
    expect(apiMocks.updateAccountEmail).toHaveBeenCalledWith({ email: 'creator@example.com' })
    expect(apiMocks.updateProfile.mock.invocationCallOrder[0]).toBeLessThan(
      apiMocks.updateAccountEmail.mock.invocationCallOrder[0]
    )
    expect(wrapper.text()).toContain('资料已保存。')
    expect(currentUser.value?.email).toBe('creator@example.com')
  })

  it('saves only the changed display name from the profile action', async () => {
    mockAccountLoad({ me: { email: 'old@example.com' } })
    apiMocks.updateProfile.mockResolvedValueOnce({
      ...mePayload,
      display_name: '视觉主理人',
      email: 'old@example.com'
    })

    const wrapper = mount(AccountView)
    await flushPromises()

    await wrapper.get('[data-testid="account-edit-profile"]').trigger('click')
    await wrapper.get('[data-testid="account-display-name-input"]').setValue('视觉主理人')
    await wrapper.get('[data-testid="account-save-profile-fields"]').trigger('click')
    await flushPromises()

    expect(apiMocks.updateProfile).toHaveBeenCalledWith({ display_name: '视觉主理人' })
    expect(apiMocks.updateAccountEmail).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('资料已保存。')
  })

  it('saves only the changed email from the profile action', async () => {
    mockAccountLoad({ me: { email: 'old@example.com' } })
    apiMocks.updateAccountEmail.mockResolvedValueOnce({
      ...mePayload,
      email: 'creator@example.com'
    })

    const wrapper = mount(AccountView)
    await flushPromises()

    await wrapper.get('[data-testid="account-bind-email"]').trigger('click')
    await wrapper.get('[data-testid="account-email-input"]').setValue('creator@example.com')
    await wrapper.get('[data-testid="account-save-profile-fields"]').trigger('click')
    await flushPromises()

    expect(apiMocks.updateProfile).not.toHaveBeenCalled()
    expect(apiMocks.updateAccountEmail).toHaveBeenCalledWith({ email: 'creator@example.com' })
    expect(wrapper.text()).toContain('资料已保存。')
  })

  it('clears only the email draft before the unified profile save is clicked', async () => {
    mockAccountLoad({ me: { email: 'old@example.com' } })
    apiMocks.updateAccountEmail.mockResolvedValueOnce({ ...mePayload, email: '' })

    const wrapper = mount(AccountView)
    await flushPromises()

    await wrapper.get('[data-testid="account-edit-profile"]').trigger('click')
    await wrapper.get('[data-testid="account-clear-email"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="account-email-input"]').element.value).toBe('')
    expect(apiMocks.updateAccountEmail).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="account-save-profile-fields"]').trigger('click')
    await flushPromises()

    expect(apiMocks.updateAccountEmail).toHaveBeenCalledWith({ email: '' })
    expect(wrapper.text()).toContain('资料已保存。')
  })

  it('disables the unified profile save when no profile fields changed', async () => {
    mockAccountLoad({ me: { email: 'old@example.com' } })

    const wrapper = mount(AccountView)
    await flushPromises()

    await wrapper.get('[data-testid="account-edit-profile"]').trigger('click')
    expect(wrapper.get('[data-testid="account-save-profile-fields"]').attributes('disabled')).toBeDefined()
  })

  it('stops the unified profile save on API failure and shows the error', async () => {
    mockAccountLoad({ me: { email: 'old@example.com' } })
    apiMocks.updateProfile.mockRejectedValueOnce(new Error('显示名称不可用'))

    const wrapper = mount(AccountView)
    await flushPromises()

    await wrapper.get('[data-testid="account-edit-profile"]').trigger('click')
    await wrapper.get('[data-testid="account-display-name-input"]').setValue('视觉主理人')
    await wrapper.get('[data-testid="account-email-input"]').setValue('creator@example.com')
    await wrapper.get('[data-testid="account-save-profile-fields"]').trigger('click')
    await flushPromises()

    expect(apiMocks.updateProfile).toHaveBeenCalledWith({ display_name: '视觉主理人' })
    expect(apiMocks.updateAccountEmail).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('显示名称不可用')
    expect(wrapper.text()).not.toContain('资料已保存。')
  })

  it('prompts legacy accounts to bind a phone and saves it with SMS verification', async () => {
    mockAccountLoad({ me: { phone: null } })
    apiMocks.sendSMSCode.mockResolvedValueOnce({})
    apiMocks.bindAccountPhone.mockResolvedValueOnce({ ...mePayload, phone: '13800138000' })

    const wrapper = mount(AccountView)
    await flushPromises()

    expect(wrapper.get('[data-testid="account-phone-status"]').text()).toContain('未绑定')
    await wrapper.get('[data-testid="account-manage-phone"]').trigger('click')
    await wrapper.get('[data-testid="account-bind-phone-input"]').setValue('13800138000')
    await wrapper.get('[data-testid="account-send-bind-phone-code"]').trigger('click')
    await flushPromises()

    expect(apiMocks.sendSMSCode).toHaveBeenCalledWith({
      phone: '13800138000',
      purpose: 'bind_phone'
    })

    await wrapper.get('[data-testid="account-bind-phone-code"]').setValue('123456')
    await wrapper.get('[data-testid="account-bind-phone-submit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.bindAccountPhone).toHaveBeenCalledWith({
      phone: '13800138000',
      verification_code: '123456'
    })
    expect(wrapper.text()).toContain('手机号已绑定')
  })

  it('shows a bound phone and unbinds it with the current password', async () => {
    mockAccountLoad({ me: { phone: '13800138000' } })
    apiMocks.unbindAccountPhone.mockResolvedValueOnce({ ...mePayload, phone: null })

    const wrapper = mount(AccountView)
    await flushPromises()

    expect(wrapper.get('[data-testid="account-bound-phone"]').text()).toContain('138****8000')
    expect(wrapper.find('[data-testid="account-bind-phone-input"]').exists()).toBe(false)

    await wrapper.get('[data-testid="account-manage-phone"]').trigger('click')
    await wrapper.get('[data-testid="account-unbind-phone-password"]').setValue('test-password')
    await wrapper.get('[data-testid="account-unbind-phone-submit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.unbindAccountPhone).toHaveBeenCalledWith({
      current_password: 'test-password'
    })
    expect(currentUser.value?.phone).toBe(null)
    expect(wrapper.text()).toContain('手机号已解绑')
    expect(wrapper.get('[data-testid="account-phone-status"]').text()).toContain('未绑定')
  })

  it('saves notification preferences when toggles change', async () => {
    mockAccountLoad()
    apiMocks.updateAccountPreferences.mockResolvedValueOnce({
      ...mePayload,
      login_notification_enabled: false,
      risk_notification_enabled: true
    })

    const wrapper = mount(AccountView)
    await flushPromises()

    await wrapper.get('[data-testid="account-login-notification"]').setValue(false)
    await flushPromises()

    expect(apiMocks.updateAccountPreferences).toHaveBeenCalledWith({
      login_notification_enabled: false,
      risk_notification_enabled: true
    })
    expect(wrapper.text()).toContain('偏好已保存')
  })

  it('validates password confirmation before calling the password API', async () => {
    mockAccountLoad()
    apiMocks.changePassword.mockResolvedValueOnce({ ok: true })

    const wrapper = mount(AccountView)
    await flushPromises()

    await wrapper.get('[data-testid="account-change-password"]').trigger('click')
    await wrapper.get('[data-testid="account-current-password"]').setValue('OldPass123')
    await wrapper.get('[data-testid="account-new-password"]').setValue('NewPass456')
    await wrapper.get('[data-testid="account-confirm-password"]').setValue('Mismatch456')
    await wrapper.get('[data-testid="account-update-password"]').trigger('click')

    expect(apiMocks.changePassword).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('两次输入的新密码不一致')

    await wrapper.get('[data-testid="account-confirm-password"]').setValue('NewPass456')
    await wrapper.get('[data-testid="account-update-password"]').trigger('click')
    await flushPromises()

    expect(apiMocks.changePassword).toHaveBeenCalledWith({
      current_password: 'OldPass123',
      new_password: 'NewPass456'
    })
    expect(wrapper.text()).toContain('密码已更新')
  })

  it('paginates credit transactions and resets to the first page when filters change', async () => {
    const firstPageItems = [
      {
        id: 1,
        type: 'generation_charge',
        amount: -1,
        balance_after: 38,
        reason: '第一页扣点',
        created_at: '2026-04-30T01:04:11Z'
      }
    ]
    const secondPageItems = [
      {
        id: 2,
        type: 'manual_topup',
        amount: 20,
        balance_after: 40,
        reason: '第二页充值',
        created_at: '2026-04-28T03:09:00Z'
      }
    ]
    mockAccountLoad({
      transactions: transactionPage(firstPageItems, { total: 11, page: 1, page_size: 10, has_more: true })
    })
    apiMocks.getCreditTransactions
      .mockResolvedValueOnce(transactionPage(secondPageItems, { total: 11, page: 2, page_size: 10, has_more: false }))
      .mockResolvedValueOnce(transactionPage(firstPageItems, { total: 11, page: 1, page_size: 10, has_more: true }))
      .mockResolvedValueOnce(transactionPage(firstPageItems, { total: 3, page: 1, page_size: 10, has_more: false }))
      .mockResolvedValueOnce(transactionPage(secondPageItems, { total: 2, page: 1, page_size: 10, has_more: false }))

    const wrapper = mount(AccountView)
    await flushPromises()

    expect(apiMocks.getCreditTransactions).toHaveBeenNthCalledWith(1, { page: 1, page_size: 10 })
    await wrapper.get('[data-testid="account-open-history"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="account-ledger-range"]').text()).toContain('第 1-10 条 / 共 11 条')
    expect(wrapper.get('[data-testid="account-ledger-prev"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="account-ledger-next"]').attributes('disabled')).toBeUndefined()

    await wrapper.get('[data-testid="account-ledger-next"]').trigger('click')
    await flushPromises()

    expect(apiMocks.getCreditTransactions).toHaveBeenNthCalledWith(2, { page: 2, page_size: 10 })
    expect(wrapper.text()).toContain('第二页充值')
    expect(wrapper.get('[data-testid="account-ledger-range"]').text()).toContain('第 11-11 条 / 共 11 条')
    expect(wrapper.get('[data-testid="account-ledger-next"]').attributes('disabled')).toBeDefined()

    await wrapper.get('[data-testid="account-ledger-prev"]').trigger('click')
    await flushPromises()

    expect(apiMocks.getCreditTransactions).toHaveBeenNthCalledWith(3, { page: 1, page_size: 10 })
    expect(wrapper.text()).toContain('第一页扣点')

    await wrapper.get('[data-testid="account-ledger-filter-charge"]').trigger('click')
    await flushPromises()
    expect(apiMocks.getCreditTransactions).toHaveBeenNthCalledWith(4, {
      page: 1,
      page_size: 10,
      kind: 'consume'
    })

    await wrapper.get('[data-testid="account-ledger-filter-topup"]').trigger('click')
    await flushPromises()
    expect(apiMocks.getCreditTransactions).toHaveBeenNthCalledWith(5, {
      page: 1,
      page_size: 10,
      kind: 'recharge'
    })
  })

  it('keeps recharge/logout navigation wired', async () => {
    mockAccountLoad()
    apiMocks.logout.mockResolvedValueOnce({ ok: true })

    const wrapper = mount(AccountView)
    await flushPromises()

    await wrapper.get('[data-testid="account-go-pricing"]').trigger('click')
    expect(routerPush).toHaveBeenCalledWith('/pricing')

    await wrapper.get('[data-testid="account-logout"]').trigger('click')
    await flushPromises()
    expect(apiMocks.logout).toHaveBeenCalled()
    expect(routerPush).toHaveBeenCalledWith('/login')
  })

  it('navigates help and support entries to their matching pages and anchors', async () => {
    mockAccountLoad()

    const wrapper = mount(AccountView)
    await flushPromises()

    await wrapper.get('[data-testid="account-help-recharge"]').trigger('click')
    expect(routerPush).toHaveBeenLastCalledWith({ path: '/pricing', hash: '#recharge-guide' })

    await wrapper.get('[data-testid="account-help-points"]').trigger('click')
    expect(routerPush).toHaveBeenLastCalledWith({ path: '/pricing', hash: '#points-rules' })

    await wrapper.get('[data-testid="account-help-contact"]').trigger('click')
    expect(routerPush).toHaveBeenLastCalledWith('/contact')

    await wrapper.get('[data-testid="account-help-faq"]').trigger('click')
    expect(routerPush).toHaveBeenLastCalledWith('/contact')
  })
})
