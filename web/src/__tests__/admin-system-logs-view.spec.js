import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  listSystemLogs: vi.fn(),
  systemLogsExportURL: vi.fn((params = {}) => {
    const query = new URLSearchParams()
    Object.entries(params).forEach(([key, value]) => {
      if (value !== undefined && value !== null && value !== '') query.set(key, `${value}`)
    })
    const text = query.toString()
    return `/api/admin/system-logs/export${text ? `?${text}` : ''}`
  })
}))

vi.mock('../api/client.js', () => ({
  api: {
    listSystemLogs: apiMocks.listSystemLogs,
    systemLogsExportURL: apiMocks.systemLogsExportURL
  }
}))

import AdminSystemLogsView from '../views/AdminSystemLogsView.vue'

function makePayload(category, overrides = {}) {
  const payloads = {
    user_login: {
      items: [
        {
          id: 2,
          category: 'user_login',
          request_id: 'req-login-fail',
          level: 'warn',
          method: 'POST',
          path: '/api/auth/login',
          status_code: 401,
          duration_ms: 8,
          ip_address: '198.51.100.3',
          user_agent: 'curl/8',
          error_code: 'login_failed',
          error_message: '账号或密码错误',
          created_at: '2026-05-13T08:00:00Z'
        },
        {
          id: 1,
          category: 'user_login',
          request_id: 'req-login-ok',
          level: 'info',
          method: 'POST',
          path: '/api/auth/login',
          status_code: 200,
          duration_ms: 21,
          ip_address: '203.0.113.9',
          user_agent: 'Mozilla/5.0',
          user_id: 8,
          user_username: 'creator',
          created_at: '2026-05-13T07:30:00Z'
        }
      ],
      total: 2,
      page: 1,
      page_size: 30,
      summary: {
        total: 2,
        success_total: 1,
        failed_total: 1,
        recent_total: 2,
        last_event_at: '2026-05-13T08:00:00Z'
      }
    },
    user_operation: {
      items: [
        {
          id: 3,
          category: 'user_operation',
          request_id: 'req-image-create',
          level: 'error',
          method: 'POST',
          path: '/api/images/generations/async',
          status_code: 503,
          duration_ms: 420,
          ip_address: '203.0.113.12',
          user_agent: 'Mozilla/5.0',
          user_id: 8,
          user_username: 'creator',
          error_code: 'provider_timeout',
          error_detail: 'gpt-image-2 upstream timeout',
          created_at: '2026-05-13T08:10:00Z'
        }
      ],
      total: 1,
      page: 1,
      page_size: 30,
      summary: {
        total: 1,
        success_total: 0,
        failed_total: 1,
        recent_total: 1,
        last_event_at: '2026-05-13T08:10:00Z'
      }
    },
    system_operation: {
      items: [
        {
          id: 4,
          category: 'system_operation',
          level: 'info',
          admin_user_id: 2,
          admin_username: 'ops-admin',
          action: 'model.update',
          target_type: 'model_config',
          target_id: 7,
          ip_address: '198.51.100.8',
          detail: '{"model":"gpt-image-2","enabled":true}',
          created_at: '2026-05-13T08:20:00Z'
        }
      ],
      total: 1,
      page: 1,
      page_size: 30,
      summary: {
        total: 1,
        success_total: 1,
        failed_total: 0,
        recent_total: 1,
        last_event_at: '2026-05-13T08:20:00Z'
      }
    }
  }
  return {
    category,
    ...payloads[category],
    ...overrides
  }
}

describe('AdminSystemLogsView', () => {
  let openMock
  let clipboardMock

  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-05-13T10:00:00Z'))
    openMock = vi.fn()
    clipboardMock = vi.fn().mockResolvedValue()
    vi.stubGlobal('open', openMock)
    Object.defineProperty(globalThis.navigator, 'clipboard', {
      configurable: true,
      value: { writeText: clipboardMock }
    })
  })

  afterEach(() => {
    vi.resetAllMocks()
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it('loads user login logs by default with full-width table and opens details from view action', async () => {
    apiMocks.listSystemLogs.mockResolvedValueOnce(makePayload('user_login'))

    const wrapper = mount(AdminSystemLogsView)
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(apiMocks.listSystemLogs).toHaveBeenCalledWith(expect.objectContaining({
      category: 'user_login',
      page: 1,
      page_size: 30
    }))
    expect(wrapper.text()).toContain('日志管理')
    expect(wrapper.text()).toContain('用户登录日志')
    expect(wrapper.text()).toContain('用户操作日志')
    expect(wrapper.text()).toContain('系统操作日志')
    expect(wrapper.text()).toContain('成功登录')
    expect(wrapper.text()).toContain('失败登录')
    expect(wrapper.find('thead').text()).toContain('登录结果')
    expect(wrapper.find('thead').text()).toContain('用户')
    expect(wrapper.find('thead').text()).toContain('操作')
    expect(wrapper.text()).toContain('req-login-fail')
    expect(wrapper.text()).toContain('login_failed')
    expect(wrapper.text()).not.toContain('失败登录只展示 IP、UA、状态和错误码')
    expect(wrapper.find('.admin-split-layout').exists()).toBe(false)
    expect(wrapper.find('[data-testid="system-logs-detail-modal"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="system-logs-copy"]').exists()).toBe(false)
    const viewButtons = wrapper.findAll('[data-testid="system-logs-view"]')
    expect(viewButtons).toHaveLength(2)
    expect(viewButtons[0].text()).toContain('查看')

    await viewButtons[0].trigger('click')
    await wrapper.vm.$nextTick()
    const modal = wrapper.get('[data-testid="system-logs-detail-modal"]')
    expect(modal.text()).toContain('登录详情')
    expect(modal.text()).toContain('失败登录只展示 IP、UA、状态和错误码')
    expect(modal.text()).toContain('req-login-fail')
    expect(modal.text()).toContain('账号或密码错误')
    expect(wrapper.text()).not.toContain('删除')
    expect(wrapper.text()).not.toContain('备注')
  })

  it('switches tabs with category requests and renders operation-specific columns', async () => {
    apiMocks.listSystemLogs
      .mockResolvedValueOnce(makePayload('user_login'))
      .mockResolvedValueOnce(makePayload('user_operation'))
      .mockResolvedValueOnce(makePayload('system_operation'))

    const wrapper = mount(AdminSystemLogsView)
    await flushPromises()

    await wrapper.get('[data-testid="system-logs-tab-user_operation"]').trigger('click')
    await flushPromises()
    expect(apiMocks.listSystemLogs).toHaveBeenNthCalledWith(2, expect.objectContaining({
      category: 'user_operation',
      page: 1
    }))
    expect(wrapper.find('thead').text()).toContain('请求路径')
    expect(wrapper.find('thead').text()).toContain('模型/支付诊断')
    expect(wrapper.find('thead').text()).toContain('操作')
    expect(wrapper.text()).toContain('/api/images/generations/async')
    expect(wrapper.text()).toContain('gpt-image-2 upstream timeout')
    await wrapper.get('[data-testid="system-logs-view"]').trigger('click')
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="system-logs-detail-modal"]').text()).toContain('操作详情')
    expect(wrapper.get('[data-testid="system-logs-detail-modal"]').text()).toContain('provider_timeout')
    await wrapper.get('[data-testid="system-logs-modal-close"]').trigger('click')
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="system-logs-tab-system_operation"]').trigger('click')
    await flushPromises()
    expect(apiMocks.listSystemLogs).toHaveBeenNthCalledWith(3, expect.objectContaining({
      category: 'system_operation',
      page: 1
    }))
    expect(wrapper.find('thead').text()).toContain('管理员')
    expect(wrapper.find('thead').text()).toContain('动作')
    expect(wrapper.find('thead').text()).toContain('目标对象')
    expect(wrapper.find('thead').text()).toContain('操作')
    expect(wrapper.text()).toContain('model.update')
    expect(wrapper.text()).toContain('{"model":"gpt-image-2","enabled":true}')
    await wrapper.get('[data-testid="system-logs-view"]').trigger('click')
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="system-logs-detail-modal"]').text()).toContain('后台操作详情')
    expect(wrapper.get('[data-testid="system-logs-detail-modal"]').text()).toContain('model_config #7')
    globalThis.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="system-logs-detail-modal"]').exists()).toBe(false)
  })

  it('filters, exports current category, copies modal details and paginates', async () => {
    apiMocks.listSystemLogs
      .mockResolvedValueOnce(makePayload('user_login', { total: 62 }))
      .mockResolvedValueOnce(makePayload('user_login', { total: 1 }))
      .mockResolvedValueOnce(makePayload('user_login', { total: 62 }))
      .mockResolvedValueOnce(makePayload('user_login', { page: 2, total: 62 }))

    const wrapper = mount(AdminSystemLogsView)
    await flushPromises()

    await wrapper.get('[data-testid="system-logs-level"]').setValue('warn')
    await wrapper.get('[data-testid="system-logs-status"]').setValue('401')
    await wrapper.get('[data-testid="system-logs-keyword"]').setValue('login_failed')
    await wrapper.get('[data-testid="system-logs-date-from"]').setValue('2026-05-13')
    await wrapper.get('[data-testid="system-logs-date-to"]').setValue('2026-05-13')
    await wrapper.get('[data-testid="system-logs-query"]').trigger('click')
    await flushPromises()

    expect(apiMocks.listSystemLogs).toHaveBeenNthCalledWith(2, expect.objectContaining({
      category: 'user_login',
      level: 'warn',
      status: '401',
      keyword: 'login_failed',
      date_from: '2026-05-13',
      date_to: '2026-05-13',
      page: 1
    }))

    await wrapper.get('[data-testid="system-logs-export"]').trigger('click')
    expect(apiMocks.systemLogsExportURL).toHaveBeenCalledWith(expect.objectContaining({
      category: 'user_login',
      keyword: 'login_failed'
    }))
    expect(openMock).toHaveBeenCalledWith(expect.stringContaining('/api/admin/system-logs/export?'), 'system-logs-export')

    await wrapper.get('[data-testid="system-logs-view"]').trigger('click')
    await wrapper.vm.$nextTick()
    await wrapper.get('[data-testid="system-logs-copy"]').trigger('click')
    expect(clipboardMock).toHaveBeenCalledWith(expect.stringContaining('req-login-fail'))
    await wrapper.get('[data-testid="system-logs-modal-backdrop"]').trigger('click')
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="system-logs-detail-modal"]').exists()).toBe(false)

    await wrapper.get('[data-testid="system-logs-reset"]').trigger('click')
    await flushPromises()
    expect(apiMocks.listSystemLogs).toHaveBeenNthCalledWith(3, expect.objectContaining({
      category: 'user_login',
      level: '',
      status: '',
      keyword: '',
      page: 1
    }))

    await wrapper.get('[data-testid="system-logs-next"]').trigger('click')
    await flushPromises()
    expect(apiMocks.listSystemLogs).toHaveBeenNthCalledWith(4, expect.objectContaining({
      category: 'user_login',
      page: 2,
      page_size: 30
    }))
  })

  it('shows loading, empty and error states per active category', async () => {
    let resolveRequest
    apiMocks.listSystemLogs.mockReturnValueOnce(new Promise((resolve) => {
      resolveRequest = resolve
    }))

    const loadingWrapper = mount(AdminSystemLogsView)
    await loadingWrapper.vm.$nextTick()
    expect(loadingWrapper.text()).toContain('加载中...')

    resolveRequest(makePayload('user_login', { items: [], total: 0 }))
    await flushPromises()
    expect(loadingWrapper.text()).toContain('暂无用户登录日志')
    expect(loadingWrapper.text()).toContain('调整筛选条件后重试')

    apiMocks.listSystemLogs.mockRejectedValueOnce(new Error('system_logs_load_failed'))
    const errorWrapper = mount(AdminSystemLogsView)
    await flushPromises()
    expect(errorWrapper.text()).toContain('system_logs_load_failed')
  })
})
