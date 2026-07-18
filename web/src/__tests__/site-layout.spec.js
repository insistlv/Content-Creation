import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import SiteLayout from '../views/SiteLayout.vue'
import { clearCurrentUser, currentUser, setCurrentUser } from '../stores/session.js'

const stylesPath = resolve(process.cwd(), 'src/styles.css')
const readStyles = () => readFileSync(stylesPath, 'utf8').replace(/\r\n/g, '\n')

const routeState = vi.hoisted(() => ({
  path: '/'
}))

const layoutMocks = vi.hoisted(() => ({
  getMe: vi.fn(),
  logout: vi.fn(),
  pingPresence: vi.fn(),
  push: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    getMe: layoutMocks.getMe,
    logout: layoutMocks.logout,
    pingPresence: layoutMocks.pingPresence
  }
}))

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({
    push: layoutMocks.push
  })
}))

describe('site layout', () => {
  beforeEach(() => {
    window.localStorage.clear()
    clearCurrentUser()
    routeState.path = '/'
    layoutMocks.getMe.mockRejectedValue(new Error('not logged in'))
    layoutMocks.pingPresence.mockResolvedValue({ ok: true })
    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      value: 'visible'
    })
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.clearAllMocks()
    window.localStorage.clear()
  })

  function mountLayout() {
    return mount(SiteLayout, {
      global: {
        stubs: {
          RouterLink: {
            props: ['to'],
            emits: ['click'],
            computed: {
              href() {
                return typeof this.to === 'string' ? this.to : this.to.path
              },
              isActive() {
                return this.href.split('#')[0] === routeState.path
              }
            },
            template: '<a :href="href" :class="{ \'router-link-active\': isActive }" @click.prevent="$emit(\'click\', $event)"><slot /></a>'
          },
          RouterView: {
            template: '<div />'
          }
        }
      }
    })
  }

  it('shows separate login and register nav entries', () => {
    routeState.path = '/login'
    const wrapper = mountLayout()

    const navLinks = wrapper.findAll('a').map((node) => node.text())
    const linkTargets = wrapper.findAll('a').map((node) => [node.text(), node.attributes('href')])

    expect(wrapper.get('.site-shell').classes()).toContain('user-dark-shell')
    expect(wrapper.get('.site-shell').attributes('data-theme')).toBe('user-dark')
    expect(wrapper.get('[data-testid="site-theme-toggle"]').attributes('aria-label')).toBe('切换到亮色模式')
    expect(wrapper.get('.brand-mark').text()).toContain('白霖共享')
    expect(wrapper.get('.brand-mark').attributes('aria-label')).toBe('白霖共享 首页')
    expect(navLinks).toContain('登录')
    expect(navLinks).toContain('注册')
    expect(wrapper.find('[data-testid="site-user-menu-toggle"]').exists()).toBe(false)
    expect(navLinks).not.toContain('登录 / 注册')
    expect(linkTargets).toContainEqual(['工作台', '/workspace'])
    expect(linkTargets).toContainEqual(['作品库', '/works'])
    expect(linkTargets).toContainEqual(['套餐', '/pricing'])
    expect(linkTargets).toContainEqual(['账户', '/account'])
    expect(linkTargets).toContainEqual(['登录', '/login'])
    expect(linkTargets).toContainEqual(['注册', '/register'])
  })

  it('renders product user pages with the shared sidebar navigation instead of the top header nav', () => {
    routeState.path = '/works'
    const wrapper = mountLayout()

    expect(wrapper.find('.site-header').exists()).toBe(false)
    expect(wrapper.get('.site-user-layout').exists()).toBe(true)
    expect(wrapper.get('[data-testid="workspace-sidebar-shell"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="workspace-nav-workspace"]').text()).toContain('工作台')
    expect(wrapper.get('[data-testid="workspace-nav-works"]').text()).toContain('作品库')
    expect(wrapper.find('[data-testid="workspace-nav-pricing"]').exists()).toBe(false)
    expect(wrapper.get('.sidebar-nav').text()).not.toContain('套餐')
    expect(wrapper.get('[data-testid="workspace-nav-account"]').text()).toContain('账户')
    expect(wrapper.get('[data-testid="workspace-nav-support"]').text()).toContain('联系客服')
    expect(wrapper.get('[data-testid="workspace-nav-works"]').classes()).toContain('active')
    expect(wrapper.find('.site-nav').exists()).toBe(false)
    expect(wrapper.find('[data-testid="workspace-nav-text-to-image"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="site-theme-toggle"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="site-mobile-menu-toggle"]').attributes('aria-expanded')).toBe('false')
  })

  it('shows the avatar menu instead of login and register when a user is logged in', async () => {
    layoutMocks.getMe.mockResolvedValueOnce({
      username: 'admin',
      display_name: '管理员',
      available_credits: 38
    })
    const wrapper = mountLayout()

    await flushPromises()

    expect(wrapper.text()).not.toContain('登录')
    expect(wrapper.text()).not.toContain('注册')

    const avatarButton = wrapper.get('[data-testid="site-user-menu-toggle"]')
    expect(avatarButton.text()).toContain('管')
    expect(avatarButton.text()).toContain('管理员')
    expect(avatarButton.attributes('aria-expanded')).toBe('false')

    await avatarButton.trigger('click')

    expect(avatarButton.attributes('aria-expanded')).toBe('true')
    expect(wrapper.get('[data-testid="site-user-menu"]').classes()).toContain('site-user-menu-open')
    expect(wrapper.get('[data-testid="site-account-link"]').attributes('href')).toBe('/account')
    expect(wrapper.get('[data-testid="site-password-link"]').attributes('href')).toBe('/account#security')
    expect(wrapper.text()).toContain('账户中心')
    expect(wrapper.text()).toContain('修改密码')
    expect(wrapper.text()).toContain('退出平台')
    expect(wrapper.get('[data-testid="site-theme-toggle"]').exists()).toBe(true)
  })

  it('keeps the logged-in header state when loading the current user fails without 401', async () => {
    setCurrentUser({
      username: 'creator',
      display_name: '创作者'
    })
    layoutMocks.getMe.mockRejectedValueOnce(Object.assign(new Error('用户服务暂不可用'), { status: 500 }))

    const wrapper = mountLayout()

    await flushPromises()

    expect(currentUser.value?.username).toBe('creator')
    expect(wrapper.find('[data-testid="site-user-menu-toggle"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('创作者')
    expect(wrapper.text()).not.toContain('登录')
    expect(wrapper.text()).not.toContain('注册')
  })

  it('defaults to dark theme, toggles to light theme, and persists the choice', async () => {
    const wrapper = mountLayout()
    const shell = wrapper.get('.site-shell')
    const toggle = wrapper.get('[data-testid="site-theme-toggle"]')

    expect(shell.classes()).toContain('user-dark-shell')
    expect(shell.classes()).not.toContain('user-light-shell')
    expect(shell.attributes('data-theme')).toBe('user-dark')
    expect(toggle.attributes('aria-label')).toBe('切换到亮色模式')
    expect(window.localStorage.getItem('image_agent_user_theme:v1')).toBeNull()

    await toggle.trigger('click')

    expect(shell.classes()).toContain('user-light-shell')
    expect(shell.classes()).not.toContain('user-dark-shell')
    expect(shell.attributes('data-theme')).toBe('user-light')
    expect(toggle.attributes('aria-label')).toBe('切换到暗色模式')
    expect(window.localStorage.getItem('image_agent_user_theme:v1')).toBe('light')

    await toggle.trigger('click')

    expect(shell.classes()).toContain('user-dark-shell')
    expect(shell.attributes('data-theme')).toBe('user-dark')
    expect(window.localStorage.getItem('image_agent_user_theme:v1')).toBe('dark')
  })

  it('falls back to dark theme when the stored value is invalid', () => {
    window.localStorage.setItem('image_agent_user_theme:v1', 'sepia')

    const wrapper = mountLayout()

    expect(wrapper.get('.site-shell').classes()).toContain('user-dark-shell')
    expect(wrapper.get('.site-shell').attributes('data-theme')).toBe('user-dark')
    expect(wrapper.get('[data-testid="site-theme-toggle"]').attributes('aria-label')).toBe('切换到亮色模式')
  })

  it('starts presence heartbeat after login, pauses while hidden, and ignores heartbeat failures', async () => {
    vi.useFakeTimers()
    layoutMocks.getMe.mockResolvedValueOnce({
      username: 'creator',
      display_name: 'Creator'
    })
    layoutMocks.pingPresence
      .mockRejectedValueOnce(new Error('network down'))
      .mockResolvedValue({ ok: true })
    const wrapper = mountLayout()

    await flushPromises()

    expect(layoutMocks.pingPresence).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[data-testid="site-user-menu-toggle"]').text()).toContain('Creator')

    await vi.advanceTimersByTimeAsync(60_000)
    expect(layoutMocks.pingPresence).toHaveBeenCalledTimes(2)

    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      value: 'hidden'
    })
    document.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(60_000)
    expect(layoutMocks.pingPresence).toHaveBeenCalledTimes(2)

    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      value: 'visible'
    })
    document.dispatchEvent(new Event('visibilitychange'))
    await flushPromises()
    expect(layoutMocks.pingPresence).toHaveBeenCalledTimes(3)

    wrapper.unmount()
  })

  it('logs out from the avatar menu and redirects to login', async () => {
    layoutMocks.getMe.mockResolvedValueOnce({
      username: 'creator',
      display_name: 'Creator'
    })
    layoutMocks.logout.mockResolvedValueOnce({ ok: true })
    const wrapper = mountLayout()

    await flushPromises()
    await wrapper.get('[data-testid="site-user-menu-toggle"]').trigger('click')
    await wrapper.get('[data-testid="site-logout-button"]').trigger('click')
    await flushPromises()

    expect(layoutMocks.logout).toHaveBeenCalled()
    expect(layoutMocks.push).toHaveBeenCalledWith('/login')
    expect(wrapper.find('[data-testid="site-user-menu-toggle"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('登录')
    expect(wrapper.text()).toContain('注册')
  })

  it('collapses and expands the mobile navigation menu', async () => {
    routeState.path = '/login'
    const wrapper = mountLayout()
    const menuButton = wrapper.get('[data-testid="site-mobile-menu-toggle"]')
    const nav = wrapper.get('#site-primary-nav')

    expect(menuButton.attributes('aria-expanded')).toBe('false')
    expect(menuButton.text()).toContain('菜单')
    expect(nav.classes()).not.toContain('site-nav-open')

    await menuButton.trigger('click')

    expect(menuButton.attributes('aria-expanded')).toBe('true')
    expect(menuButton.text()).toContain('收起')
    expect(nav.classes()).toContain('site-nav-open')

    await menuButton.trigger('click')

    expect(menuButton.attributes('aria-expanded')).toBe('false')
    expect(nav.classes()).not.toContain('site-nav-open')
  })

  it('closes the expanded mobile navigation after a nav link click', async () => {
    routeState.path = '/login'
    const wrapper = mountLayout()

    await wrapper.get('[data-testid="site-mobile-menu-toggle"]').trigger('click')
    expect(wrapper.get('#site-primary-nav').classes()).toContain('site-nav-open')

    await wrapper.get('a[href="/works"]').trigger('click')

    expect(wrapper.get('[data-testid="site-mobile-menu-toggle"]').attributes('aria-expanded')).toBe('false')
    expect(wrapper.get('#site-primary-nav').classes()).not.toContain('site-nav-open')
  })

  it('adds a workspace shell class on the workspace route', () => {
    routeState.path = '/workspace'
    const wrapper = mountLayout()

    expect(wrapper.get('.site-shell').classes()).toContain('site-shell-workspace')
    routeState.path = '/'
  })

  it('adds a works shell class on the works route', () => {
    routeState.path = '/works'
    const wrapper = mountLayout()

    expect(wrapper.get('.site-shell').classes()).toContain('site-shell-works')
    routeState.path = '/'
  })

  it('adds a pricing shell class on the pricing route', () => {
    routeState.path = '/pricing'
    const wrapper = mountLayout()

    expect(wrapper.get('.site-shell').classes()).toContain('site-shell-pricing')
    routeState.path = '/'
  })

  it('adds an account shell class on the account route', () => {
    routeState.path = '/account'
    const wrapper = mountLayout()

    expect(wrapper.get('.site-shell').classes()).toContain('site-shell-account')
    routeState.path = '/'
  })

  it('only marks the account sidebar navigation item as active on the account route', () => {
    routeState.path = '/account'
    const wrapper = mountLayout()

    const accountLink = wrapper.get('[data-testid="workspace-nav-account"]')
    const workspaceLink = wrapper.get('[data-testid="workspace-nav-workspace"]')
    const inactiveLinks = wrapper
      .findAll('.sidebar-nav .nav-item')
      .filter((link) => link.attributes('data-testid') !== 'workspace-nav-account')

    expect(accountLink.classes()).toContain('active')
    expect(accountLink.classes()).not.toContain('nav-link-workspace')
    expect(workspaceLink.classes()).not.toContain('active')
    expect(workspaceLink.classes()).not.toContain('nav-link-workspace')
    expect(inactiveLinks.every((link) => !link.classes().includes('active'))).toBe(true)
    routeState.path = '/'
  })

  it('adds an auth shell class on login and register routes', () => {
    routeState.path = '/login'
    const loginWrapper = mountLayout()
    expect(loginWrapper.get('.site-shell').classes()).toContain('site-shell-auth')

    routeState.path = '/register'
    const registerWrapper = mountLayout()
    expect(registerWrapper.get('.site-shell').classes()).toContain('site-shell-auth')
    routeState.path = '/'
  })

  it('defines light theme selectors for the user site surfaces', () => {
    const css = readStyles()

    expect(css).toContain('.user-light-shell')
    expect(css).toContain('.site-shell.user-light-shell .site-header-shell')
    expect(css).toContain('.user-light-shell .primary-button')
    expect(css).toContain('.user-light-shell .soft-panel')
    expect(css).toContain('.site-shell.site-shell-auth.user-light-shell')
  })

  it('applies the workspace sidebar theme selectors to product user pages', () => {
    const css = readStyles()

    expect(css).toContain('.site-shell-user-sidebar.user-dark-shell .workspace-sidebar-shell')
    expect(css).toContain('.site-shell-user-sidebar.user-dark-shell .workspace-sidebar')
    expect(css).toContain('.site-shell-user-sidebar.user-dark-shell .nav-item.active')
    expect(css).toContain('.site-shell-user-sidebar.user-dark-shell .recharge-button')
    expect(css).toContain('.site-shell-user-sidebar.user-dark-shell .workspace-sidebar-toggle')

    expect(css).toContain('.site-shell-user-sidebar.user-light-shell .workspace-sidebar-shell')
    expect(css).toContain('.site-shell-user-sidebar.user-light-shell .workspace-sidebar')
    expect(css).toContain('.site-shell-user-sidebar.user-light-shell .nav-item.active')
    expect(css).toContain('.site-shell-user-sidebar.user-light-shell .recharge-button')
    expect(css).toContain('.site-shell-user-sidebar.user-light-shell .workspace-sidebar-toggle')
  })

  it('does not define workspace-only top navigation highlight styles', () => {
    const css = readStyles()

    expect(css).not.toContain('nav-link-workspace')
  })

  it('aligns the public works, account, and contact content shells with the desktop header width', () => {
    const css = readStyles()

    expect(css).toMatch(
      /\.site-shell-works \.site-content-shell,\n\.site-shell-account \.site-content-shell,\n\.site-shell-contact \.site-content-shell\s*\{\s*width: min\(1680px, 100%\);\s*max-width: none;\s*\}/
    )
    expect(css).toMatch(
      /@media \(max-width: 1180px\) \{[\s\S]*?\.site-shell-works \.site-content-shell,\n\s*\.site-shell-account \.site-content-shell,\n\s*\.site-shell-contact \.site-content-shell\s*\{\s*width: 100%;\s*\}/
    )
    expect(css).not.toContain('width: min(1230px, calc(100vw - 120px));')
    expect(css).not.toContain('width: min(1280px, calc(100vw - 220px));')
    expect(css).not.toContain('width: min(1280px, calc(100vw - 92px));')
  })
})
