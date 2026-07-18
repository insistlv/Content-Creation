import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  adminLogout: vi.fn(),
  changeAdminPassword: vi.fn(),
  searchAdmin: vi.fn(),
  getCurrentAdminSession: vi.fn()
}))

const routerMock = vi.hoisted(() => ({
  push: vi.fn()
}))

const routeMock = vi.hoisted(() => ({
  path: '/admin/system-logs'
}))

vi.mock('vue-router', () => ({
  useRoute: () => routeMock,
  useRouter: () => routerMock
}))

vi.mock('../api/client.js', () => ({
  api: {
    adminLogout: apiMocks.adminLogout,
    changeAdminPassword: apiMocks.changeAdminPassword,
    searchAdmin: apiMocks.searchAdmin
  },
  getCurrentAdminSession: apiMocks.getCurrentAdminSession
}))

import AdminLayout from '../views/AdminLayout.vue'

function mockAdminSession() {
  apiMocks.getCurrentAdminSession.mockResolvedValue({
    admin: {
      id: 1,
      username: 'admin',
      display_name: '总管理员'
    },
    menus: [
      { label: '概览', path: '/admin', permission: 'dashboard.read' },
      { label: '用户与点数', path: '/admin/users', permission: 'users.read' },
      { label: '生成记录', path: '/admin/generations', permission: 'generations.read' },
      { label: '视频记录', path: '/admin/video-generations', permission: 'generations.read' },
      { label: '套餐配置', path: '/admin/packages', permission: 'packages.read' },
      { label: '财务订单', path: '/admin/finance-orders', permission: 'finance_orders.read' },
      { label: '资源监控', path: '/admin/system-resources', permission: 'system_resources.read' },
      { label: '系统日志', path: '/admin/system-logs', permission: 'system_logs.read' },
      { label: '提示词模板', path: '/admin/prompt-templates', permission: 'prompt_templates.read' },
      { label: '灵感推荐', path: '/admin/inspiration-recommendations', permission: 'inspiration_recommendations.read' },
      { label: '相册配置', path: '/admin/couple-album-options', permission: 'couple_album_options.read' }
    ],
    permissions: ['dashboard.read', 'system_resources.read', 'system_logs.read', 'prompt_templates.read', 'inspiration_recommendations.read', 'couple_album_options.read']
  })
}

async function mountLayout() {
  mockAdminSession()
  const wrapper = mount(AdminLayout, {
    attachTo: document.body,
    global: {
      stubs: {
        RouterLink: {
          emits: ['click'],
          props: ['to'],
          template: '<a :href="to" @click.prevent="$emit(\'click\', $event)"><slot /></a>'
        },
        RouterView: {
          template: '<div data-testid="admin-router-view" />'
        }
      }
    }
  })
  await flushPromises()
  return wrapper
}

describe('AdminLayout', () => {
  afterEach(() => {
    vi.clearAllMocks()
    routeMock.path = '/admin/system-logs'
    document.body.innerHTML = ''
  })

  it('opens and closes the top-right admin profile menu', async () => {
    const wrapper = await mountLayout()

    expect(wrapper.find('[data-testid="admin-profile-menu"]').exists()).toBe(false)

    await wrapper.get('[data-testid="admin-profile-button"]').trigger('click')
    expect(wrapper.get('[data-testid="admin-profile-menu"]').text()).toContain('修改密码')
    expect(wrapper.get('[data-testid="admin-profile-menu"]').text()).toContain('退出登录')
    expect(wrapper.text()).toContain('admin')

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="admin-profile-menu"]').exists()).toBe(false)

    await wrapper.get('[data-testid="admin-profile-button"]').trigger('click')
    document.body.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="admin-profile-menu"]').exists()).toBe(false)
  })

  it('logs out from the profile menu and redirects to admin login', async () => {
    apiMocks.adminLogout.mockResolvedValue({ ok: true })
    const wrapper = await mountLayout()

    await wrapper.get('[data-testid="admin-profile-button"]').trigger('click')
    await wrapper.get('[data-testid="admin-menu-logout"]').trigger('click')
    await flushPromises()

    expect(apiMocks.adminLogout).toHaveBeenCalled()
    expect(routerMock.push).toHaveBeenCalledWith('/admin/login')
    expect(wrapper.find('[data-testid="admin-profile-menu"]').exists()).toBe(false)
  })

  it('renders admin menus inside non-empty sidebar categories only', async () => {
    const wrapper = await mountLayout()

    expect(wrapper.text()).toContain('白霖共享')
    expect(wrapper.text()).toContain('核心运营')
    expect(wrapper.text()).toContain('交易增长')
    expect(wrapper.text()).toContain('配置中心')
    expect(wrapper.text()).toContain('系统管理')
    expect(wrapper.text()).not.toContain('内容治理')
    expect(wrapper.text()).toContain('资源监控')
    expect(wrapper.text()).toContain('系统日志')

    await wrapper.get('[data-testid="admin-nav-group-config"]').trigger('click')

    expect(wrapper.text()).toContain('提示词模板')
    expect(wrapper.text()).toContain('灵感推荐')
    expect(wrapper.text()).toContain('相册配置')
    expect(wrapper.find('[data-testid="admin-nav-link-/admin/inspiration-recommendations"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('邀请码')
  })

  it('renders the video style preset admin menu inside the config category', async () => {
    apiMocks.getCurrentAdminSession.mockResolvedValue({
      admin: {
        id: 1,
        username: 'admin',
        display_name: 'Admin'
      },
      menus: [
        { label: '视频风格预设', path: '/admin/video-style-presets', permission: 'video_style_presets.read' }
      ],
      permissions: ['video_style_presets.read']
    })
    routeMock.path = '/admin/video-style-presets'

    const wrapper = mount(AdminLayout, {
      attachTo: document.body,
      global: {
        stubs: {
          RouterLink: {
            emits: ['click'],
            props: ['to'],
            template: '<a :href="to" @click.prevent="$emit(\'click\', $event)"><slot /></a>'
          },
          RouterView: {
            template: '<div data-testid="admin-router-view" />'
          }
        }
      }
    })
    await flushPromises()

    expect(wrapper.get('[data-testid="admin-nav-group-config"]').attributes('aria-expanded')).toBe('true')
    expect(wrapper.text()).toContain('视频风格预设')
    expect(wrapper.find('[data-testid="admin-nav-link-/admin/video-style-presets"]').exists()).toBe(true)
  })

  it('places video records directly below generation records in core operations', async () => {
    routeMock.path = '/admin/generations'
    const wrapper = await mountLayout()

    const coreLinks = wrapper.findAll('#admin-nav-group-panel-core a')
    expect(coreLinks.map((link) => link.attributes('href'))).toEqual([
      '/admin',
      '/admin/users',
      '/admin/generations',
      '/admin/video-generations'
    ])
  })

  it('expands the current route category and keeps other categories collapsed by default', async () => {
    const wrapper = await mountLayout()

    const systemButton = wrapper.get('[data-testid="admin-nav-group-system"]')
    const coreButton = wrapper.get('[data-testid="admin-nav-group-core"]')

    expect(systemButton.attributes('aria-expanded')).toBe('true')
    expect(wrapper.find('[data-testid="admin-nav-link-/admin/system-logs"]').exists()).toBe(true)
    expect(coreButton.attributes('aria-expanded')).toBe('false')
    expect(wrapper.find('[data-testid="admin-nav-link-/admin/users"]').exists()).toBe(false)
  })

  it('toggles a sidebar category with aria-expanded state', async () => {
    const wrapper = await mountLayout()

    const coreButton = wrapper.get('[data-testid="admin-nav-group-core"]')
    await coreButton.trigger('click')

    expect(coreButton.attributes('aria-expanded')).toBe('true')
    expect(wrapper.find('[data-testid="admin-nav-link-/admin/users"]').exists()).toBe(true)

    await coreButton.trigger('click')

    expect(coreButton.attributes('aria-expanded')).toBe('false')
    expect(wrapper.find('[data-testid="admin-nav-link-/admin/users"]').exists()).toBe(false)
  })

  it('closes the mobile sidebar after clicking a nested admin menu link', async () => {
    routeMock.path = '/admin'
    const wrapper = await mountLayout()

    await wrapper.get('.admin-menu-toggle').trigger('click')
    expect(wrapper.get('.admin-shell').classes()).toContain('sidebar-open')

    await wrapper.get('[data-testid="admin-nav-link-/admin/users"]').trigger('click')

    expect(wrapper.get('.admin-shell').classes()).not.toContain('sidebar-open')
  })

  it('validates password changes before calling the API', async () => {
    const wrapper = await mountLayout()

    await wrapper.get('[data-testid="admin-profile-button"]').trigger('click')
    await wrapper.get('[data-testid="admin-menu-change-password"]').trigger('click')
    await wrapper.get('[data-testid="admin-password-current"]').setValue('OldPass123')
    await wrapper.get('[data-testid="admin-password-new"]').setValue('short')
    await wrapper.get('[data-testid="admin-password-confirm"]').setValue('short')
    await wrapper.get('[data-testid="admin-password-form"]').trigger('submit')
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('新密码至少 8 位')
    expect(apiMocks.changeAdminPassword).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="admin-password-new"]').setValue('NewPass456')
    await wrapper.get('[data-testid="admin-password-confirm"]').setValue('OtherPass456')
    await wrapper.get('[data-testid="admin-password-form"]').trigger('submit')
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('两次输入的新密码不一致')
    expect(apiMocks.changeAdminPassword).not.toHaveBeenCalled()
  })

  it('changes the current admin password and redirects to login', async () => {
    apiMocks.changeAdminPassword.mockResolvedValue({ ok: true })
    const wrapper = await mountLayout()

    await wrapper.get('[data-testid="admin-profile-button"]').trigger('click')
    await wrapper.get('[data-testid="admin-menu-change-password"]').trigger('click')
    await wrapper.get('[data-testid="admin-password-current"]').setValue('OldPass123')
    await wrapper.get('[data-testid="admin-password-new"]').setValue('NewPass456')
    await wrapper.get('[data-testid="admin-password-confirm"]').setValue('NewPass456')
    await wrapper.get('[data-testid="admin-password-form"]').trigger('submit')
    await flushPromises()

    expect(apiMocks.changeAdminPassword).toHaveBeenCalledWith({
      current_password: 'OldPass123',
      new_password: 'NewPass456'
    })
    expect(routerMock.push).toHaveBeenCalledWith('/admin/login')
    expect(wrapper.find('[data-testid="admin-password-modal"]').exists()).toBe(false)
  })

  it('submits global admin search from enter and button clicks', async () => {
    apiMocks.searchAdmin.mockResolvedValue({
      query: 'creator',
      sections: [
        {
          key: 'users',
          label: '用户',
          items: [
            { id: '7', title: 'creator_alpha', subtitle: '13800138001 / alpha@example.com', to: '/admin/users?q=creator' }
          ]
        }
      ]
    })
    const wrapper = await mountLayout()

    await wrapper.get('[data-testid="admin-global-search-input"]').setValue('creator')
    await wrapper.get('[data-testid="admin-global-search-form"]').trigger('submit')
    await flushPromises()

    expect(apiMocks.searchAdmin).toHaveBeenCalledWith({ q: 'creator' })
    expect(wrapper.get('[data-testid="admin-global-search-panel"]').text()).toContain('creator_alpha')

    await wrapper.get('[data-testid="admin-global-search-input"]').setValue('海报')
    await wrapper.get('[data-testid="admin-global-search-submit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.searchAdmin).toHaveBeenLastCalledWith({ q: '海报' })
  })

  it('shows loading, empty and error states for global admin search', async () => {
    let resolveSearch
    apiMocks.searchAdmin.mockReturnValue(new Promise((resolve) => {
      resolveSearch = resolve
    }))
    const wrapper = await mountLayout()

    await wrapper.get('[data-testid="admin-global-search-input"]').setValue('missing')
    await wrapper.get('[data-testid="admin-global-search-form"]').trigger('submit')
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="admin-global-search-submit"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="admin-global-search-submit"]').text()).toContain('搜索中')

    resolveSearch({ query: 'missing', sections: [] })
    await flushPromises()

    expect(wrapper.get('[data-testid="admin-global-search-panel"]').text()).toContain('没有找到匹配结果')

    apiMocks.searchAdmin.mockRejectedValueOnce(new Error('搜索服务暂不可用'))
    await wrapper.get('[data-testid="admin-global-search-input"]').setValue('error')
    await wrapper.get('[data-testid="admin-global-search-form"]').trigger('submit')
    await flushPromises()

    expect(wrapper.get('[data-testid="admin-global-search-panel"]').text()).toContain('搜索服务暂不可用')
  })

  it('navigates from a global search result and closes the panel with escape', async () => {
    apiMocks.searchAdmin.mockResolvedValue({
      query: '系统',
      sections: [
        {
          key: 'config',
          label: '配置入口',
          items: [
            { id: 'system-settings', title: '系统设置', subtitle: '配置中心', to: '/admin/system-settings' }
          ]
        }
      ]
    })
    const wrapper = await mountLayout()

    await wrapper.get('[data-testid="admin-global-search-input"]').setValue('系统')
    await wrapper.get('[data-testid="admin-global-search-form"]').trigger('submit')
    await flushPromises()

    await wrapper.get('[data-testid="admin-global-search-result-system-settings"]').trigger('click')

    expect(routerMock.push).toHaveBeenCalledWith('/admin/system-settings')
    expect(wrapper.find('[data-testid="admin-global-search-panel"]').exists()).toBe(false)

    await wrapper.get('[data-testid="admin-global-search-input"]').setValue('系统')
    await wrapper.get('[data-testid="admin-global-search-form"]').trigger('submit')
    await flushPromises()

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wrapper.vm.$nextTick()

    expect(wrapper.find('[data-testid="admin-global-search-panel"]').exists()).toBe(false)
  })
})
