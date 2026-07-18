import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

const apiMocks = vi.hoisted(() => ({
  getSystemSettings: vi.fn(),
  updateSystemSettings: vi.fn(),
  systemSettingsExportURL: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    getSystemSettings: apiMocks.getSystemSettings,
    updateSystemSettings: apiMocks.updateSystemSettings,
    systemSettingsExportURL: apiMocks.systemSettingsExportURL
  }
}))

import AdminSystemSettingsView from '../views/AdminSystemSettingsView.vue'

const settingsPayload = {
  settings: {
    platform: {
      name: '白霖共享',
      short_name: 'IA',
      logo_url: 'https://cdn.example.com/logo.png',
      timezone: 'Asia/Shanghai',
      language: 'zh-CN',
      currency: 'CNY',
      icp_record_number: '沪ICP备20260501号',
      platform_domain: 'https://images.example.com'
    },
    storage: {
      storage_mode: 'local',
      provider: 'local',
      region: 'cn-shanghai',
      bucket: 'data/assets',
      cdn_domain: '',
      cdn_acceleration: false
    },
    generation: {
      upload_limit: 6,
      default_aspect_ratio: '1:1',
      retention_days: 30,
      concurrency_limit: 4,
      review_policy: 'standard',
      negative_prompt_enabled: true,
      advanced_parameters_enabled: true
    },
    notifications: {
      notification_email: 'ops@example.com',
      task_complete_notice: true,
      system_alert_notice: true,
      daily_summary_notice: false,
      webhook_url: 'https://hooks.example.com/system'
    },
    security: {
      login_policy: 'standard',
      password_min_length: 8,
      two_factor_enabled: false,
      failed_login_lock_enabled: true,
      admin_permission_management_enabled: true
    }
  },
  defaults: {
    platform: {
      name: '白霖共享',
      short_name: 'IA',
      logo_url: '',
      timezone: 'Asia/Shanghai',
      language: 'zh-CN',
      currency: 'CNY',
      icp_record_number: '',
      platform_domain: 'http://localhost:3000'
    },
    storage: {
      storage_mode: 'local',
      provider: 'local',
      region: '',
      bucket: 'data/assets',
      cdn_domain: '',
      cdn_acceleration: false
    },
    generation: {
      upload_limit: 6,
      default_aspect_ratio: '1:1',
      retention_days: 30,
      concurrency_limit: 4,
      review_policy: 'standard',
      negative_prompt_enabled: true,
      advanced_parameters_enabled: true
    },
    notifications: {
      notification_email: '',
      task_complete_notice: true,
      system_alert_notice: true,
      daily_summary_notice: false,
      webhook_url: ''
    },
    security: {
      login_policy: 'standard',
      password_min_length: 8,
      two_factor_enabled: false,
      failed_login_lock_enabled: true,
      admin_permission_management_enabled: true
    }
  },
  status: {
    runtime_status: 'running',
    database_status: 'connected',
    version: 'local',
    started_at: '2026-05-01T07:30:00Z',
    storage_mode: 'local',
    storage_provider: 'local',
    storage_bucket: 'data/assets',
    storage_used_bytes: 12,
    storage_capacity_bytes: 1024,
    cdn_status: 'disabled',
    cdn_traffic_bytes: 256,
    cdn_traffic_limit_bytes: 2048,
    today_generations: 2,
    daily_generation_limit: 50,
    queue_status: {
      queued: 1,
      running: 1
    },
    payment: {
      alipay: {
        configured: false,
        sandbox: true,
        gateway: 'https://openapi-sandbox.dl.alipaydev.com/gateway.do',
        notify_url: 'https://images.example.com/api/payments/alipay/notify',
        return_url_base: 'https://images.example.com/checkout/alipay/return',
        missing: ['ALIPAY_PUBLIC_KEY'],
        items: [
          { key: 'ALIPAY_APP_ID', label: '应用 ID', configured: true },
          { key: 'ALIPAY_PRIVATE_KEY', label: '应用私钥', configured: true },
          { key: 'ALIPAY_PUBLIC_KEY', label: '支付宝公钥', configured: false },
          { key: 'APP_BASE_URL', label: '公网 HTTPS 域名', configured: true },
          { key: 'ALIPAY_GATEWAY', label: '网关地址', configured: true }
        ]
      }
    },
    total_users: 12,
    total_works: 48,
    total_generations: 64
  },
  updated_at: '2026-05-01T08:00:00Z'
}

function mockInitialLoad() {
  apiMocks.getSystemSettings.mockResolvedValue(settingsPayload)
  apiMocks.updateSystemSettings.mockResolvedValue({ ok: true, settings: settingsPayload.settings })
  apiMocks.systemSettingsExportURL.mockReturnValue('/api/admin/system-settings/export')
}

describe('AdminSystemSettingsView', () => {
  afterEach(() => {
    vi.clearAllMocks()
    vi.unstubAllGlobals()
  })

  it('renders system settings sections, tabs, status and quick actions from backend data', async () => {
    mockInitialLoad()

    const wrapper = mount(AdminSystemSettingsView)
    await flushPromises()

    expect(apiMocks.getSystemSettings).toHaveBeenCalled()
    expect(wrapper.text()).toContain('系统设置')
    expect(wrapper.text()).toContain('配置平台运行参数、功能策略与权限管理')
    expect(wrapper.text()).toContain('基础设置')
    expect(wrapper.text()).toContain('存储与 CDN')
    expect(wrapper.text()).toContain('生成策略')
    expect(wrapper.text()).toContain('消息通知')
    expect(wrapper.text()).toContain('安全权限')
    expect(wrapper.text()).toContain('平台信息')
    expect(wrapper.text()).toContain('Provider')
    expect(wrapper.text()).toContain('上传数量上限')
    expect(wrapper.text()).toContain('通知邮箱')
    expect(wrapper.text()).toContain('登录安全策略')
    expect(wrapper.text()).toContain('系统状态')
    expect(wrapper.text()).toContain('快捷操作')
    expect(wrapper.text()).toContain('导出配置')
    expect(wrapper.text()).toContain('白霖共享')
    expect(wrapper.text()).toContain('12')
    expect(wrapper.text()).toContain('今日生成')
    expect(wrapper.text()).toContain('2 / 50')
    expect(wrapper.text()).toContain('存储用量')
    expect(wrapper.text()).toContain('12 B / 1 KB')
    expect(wrapper.text()).toContain('CDN 流量')
    expect(wrapper.text()).toContain('256 B / 2 KB')
    expect(wrapper.text()).toContain('队列')
    expect(wrapper.text()).toContain('1 等待 / 1 运行')
    expect(wrapper.text()).toContain('支付宝支付')
    expect(wrapper.text()).toContain('沙箱联调')
    expect(wrapper.text()).toContain('ALIPAY_PUBLIC_KEY')
    expect(wrapper.text()).toContain('缺失')
    expect(wrapper.text()).toContain('https://images.example.com/api/payments/alipay/notify')
    expect(wrapper.find('[data-testid="system-settings-savebar"]').exists()).toBe(true)
    await wrapper.get('[aria-label="默认画幅"]').trigger('click')
    await nextTick()
    const aspectOptions = Array.from(document.body.querySelector('[role="listbox"][aria-label="默认画幅"]').querySelectorAll('.click-select-option'))
    expect(aspectOptions.map((node) => node.textContent)).toEqual(expect.arrayContaining([
      '21:9',
      '16:9',
      '4:3',
      '3:2',
      '1:1',
      '2:3',
      '3:4',
      '9:16',
      '9:21'
    ]))
  })

  it('saves edits across all visible setting groups as a full payload', async () => {
    mockInitialLoad()

    const wrapper = mount(AdminSystemSettingsView)
    await flushPromises()

    await wrapper.get('[data-testid="platform-name"]').setValue('生成平台')
    await wrapper.get('[data-testid="storage-provider"]').setValue('aliyun-oss')
    expect(wrapper.get('[data-testid="platform-name"]').element.value).toBe('生成平台')

    await wrapper.get('[data-testid="save-system-settings"]').trigger('click')
    await flushPromises()

    expect(apiMocks.updateSystemSettings).toHaveBeenCalledWith(expect.objectContaining({
      platform: expect.objectContaining({ name: '生成平台' }),
      storage: expect.objectContaining({ provider: 'aliyun-oss' }),
      generation: expect.objectContaining({ upload_limit: 6 }),
      notifications: expect.objectContaining({ notification_email: 'ops@example.com' }),
      security: expect.objectContaining({ password_min_length: 8 })
    }))
    expect(wrapper.text()).toContain('系统设置已保存')
  })

  it('restores backend defaults locally and exports settings through the export endpoint', async () => {
    mockInitialLoad()
    const openMock = vi.fn()
    vi.stubGlobal('open', openMock)

    const wrapper = mount(AdminSystemSettingsView)
    await flushPromises()

    await wrapper.get('[data-testid="platform-name"]').setValue('临时名称')
    await wrapper.get('[data-testid="restore-system-defaults"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="platform-name"]').element.value).toBe('白霖共享')
    expect(apiMocks.updateSystemSettings).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="export-system-settings"]').trigger('click')
    expect(apiMocks.systemSettingsExportURL).toHaveBeenCalled()
    expect(openMock).toHaveBeenCalledWith('/api/admin/system-settings/export', 'system-settings-export')
  })

  it('shows unconfigured labels for capacity based status limits', async () => {
    const payload = JSON.parse(JSON.stringify(settingsPayload))
    payload.status.storage_capacity_bytes = 0
    payload.status.cdn_traffic_limit_bytes = 0
    payload.status.daily_generation_limit = 0
    apiMocks.getSystemSettings.mockResolvedValue(payload)
    apiMocks.updateSystemSettings.mockResolvedValue({ ok: true, settings: payload.settings })
    apiMocks.systemSettingsExportURL.mockReturnValue('/api/admin/system-settings/export')

    const wrapper = mount(AdminSystemSettingsView)
    await flushPromises()

    expect(wrapper.text()).toContain('存储用量')
    expect(wrapper.text()).toContain('未配置')
  })

  it('shows placeholder feedback for maintenance actions and load errors', async () => {
    mockInitialLoad()

    const wrapper = mount(AdminSystemSettingsView)
    await flushPromises()

    await wrapper.get('[data-testid="clean-temp-files"]').trigger('click')
    expect(wrapper.text()).toContain('暂未接入')

    apiMocks.getSystemSettings.mockRejectedValueOnce(new Error('加载失败'))
    const errorWrapper = mount(AdminSystemSettingsView)
    await flushPromises()
    expect(errorWrapper.text()).toContain('加载失败')
  })
})
