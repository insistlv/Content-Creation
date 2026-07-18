import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const routerPush = vi.hoisted(() => vi.fn())
const routeState = vi.hoisted(() => ({
  path: '/workspace'
}))
const apiMocks = vi.hoisted(() => ({
  getMe: vi.fn(),
  pingPresence: vi.fn(),
  logout: vi.fn(),
  getCaptcha: vi.fn(),
  login: vi.fn(),
  sendSMSCode: vi.fn(),
  registerPhone: vi.fn(),
  resetPassword: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    getMe: apiMocks.getMe,
    pingPresence: apiMocks.pingPresence,
    logout: apiMocks.logout,
    getCaptcha: apiMocks.getCaptcha,
    login: apiMocks.login,
    sendSMSCode: apiMocks.sendSMSCode,
    registerPhone: apiMocks.registerPhone,
    resetPassword: apiMocks.resetPassword
  }
}))

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({
    push: routerPush
  })
}))

vi.mock('../components/AuthModal.vue', () => ({
  default: {
    name: 'AuthModal',
    template: '<div data-testid="auth-modal-stub"></div>'
  }
}))

import WorkspaceLayout from '../views/WorkspaceLayout.vue'
import { applyAvailableCredits, clearCurrentUser, currentUser, setCurrentUser } from '../stores/session.js'
import { authModalState, closeAuthModal } from '../stores/auth-modal.js'

function getTeleportedUserMenuItem(testId) {
  const item = document.body.querySelector(`[data-testid="${testId}"]`)
  if (!item) {
    throw new Error(`Unable to get teleported user menu item: ${testId}`)
  }
  return item
}

describe('WorkspaceLayout', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    window.localStorage.clear()
    clearCurrentUser()
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    routeState.path = '/workspace'
    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      value: 'visible'
    })
    apiMocks.getMe.mockResolvedValue({
      username: 'creator',
      available_credits: 12,
      tier: 'Free'
    })
    apiMocks.getCaptcha.mockResolvedValue({
      captcha_id: 'cap-user',
      image_base64: 'png-user',
      expires_in: 300
    })
    apiMocks.pingPresence.mockResolvedValue({ ok: true })
    apiMocks.logout.mockResolvedValue({ ok: true })
  })

  afterEach(async () => {
    vi.restoreAllMocks()
    vi.useRealTimers()
    window.localStorage.clear()
    closeAuthModal()
    await flushPromises()
    document.body.innerHTML = ''
  })

  it('renders the workspace shell immediately while session loading is still pending', async () => {
    apiMocks.getMe.mockReturnValueOnce(new Promise(() => {}))
    const wrapper = mount(WorkspaceLayout, {
      global: {
        stubs: {
          RouterView: {
            template: '<main data-testid="workspace-route-page">page</main>'
          }
        }
      }
    })

    expect(wrapper.find('.workspace-loading').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('加载中...')
    expect(wrapper.get('.workspace-with-sidebar').exists()).toBe(true)
    expect(wrapper.get('[data-testid="workspace-sidebar-shell"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="workspace-route-page"]').exists()).toBe(true)
    expect(apiMocks.pingPresence).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it('shows the workspace shell for guests when session loading returns 401', async () => {
    apiMocks.getMe.mockRejectedValueOnce(Object.assign(new Error('unauthorized'), { status: 401 }))
    const wrapper = mount(WorkspaceLayout, {
      global: {
        stubs: {
          RouterView: {
            template: '<main data-testid="workspace-route-page">page</main>'
          }
        }
      }
    })
    await flushPromises()

    expect(wrapper.find('.workspace-error').exists()).toBe(false)
    expect(wrapper.get('[data-testid="workspace-route-page"]').exists()).toBe(true)
    expect(apiMocks.pingPresence).not.toHaveBeenCalled()
  })

  it('uses the shared user theme in the workspace shell and toggles it', async () => {
    window.localStorage.setItem('image_agent_user_theme:v1', 'light')
    const wrapper = mount(WorkspaceLayout, {
      global: {
        stubs: {
          RouterView: {
            template: '<main data-testid="workspace-route-page">page</main>'
          }
        }
      }
    })
    await flushPromises()

    const shell = wrapper.get('.workspace-with-sidebar')
    const toggle = wrapper.get('[data-testid="site-theme-toggle"]')

    expect(shell.classes()).toContain('user-light-shell')
    expect(shell.classes()).not.toContain('user-dark-shell')
    expect(shell.attributes('data-theme')).toBe('user-light')
    expect(toggle.attributes('aria-label')).toBe('切换到暗色模式')

    await toggle.trigger('click')

    expect(shell.classes()).toContain('user-dark-shell')
    expect(shell.attributes('data-theme')).toBe('user-dark')
    expect(window.localStorage.getItem('image_agent_user_theme:v1')).toBe('dark')
  })

  it('lets mobile users expand and collapse the workspace sidebar', async () => {
    const wrapper = mount(WorkspaceLayout, {
      global: {
        stubs: {
          RouterView: {
            template: '<main data-testid="workspace-route-page">page</main>'
          }
        }
      }
    })
    await flushPromises()

    const shell = wrapper.get('.workspace-with-sidebar')
    const toggle = wrapper.get('[data-testid="workspace-sidebar-toggle"]')

    expect(shell.classes()).toContain('user-dark-shell')
    expect(shell.attributes('data-theme')).toBe('user-dark')
    expect(shell.classes()).not.toContain('workspace-sidebar-open')
    expect(toggle.attributes('aria-expanded')).toBe('false')
    expect(toggle.text()).toContain('展开菜单')

    await toggle.trigger('click')

    expect(shell.classes()).toContain('workspace-sidebar-open')
    expect(toggle.attributes('aria-expanded')).toBe('true')
    expect(toggle.text()).toContain('收起菜单')

    await wrapper.get('[data-testid="workspace-sidebar-backdrop"]').trigger('click')

    expect(shell.classes()).not.toContain('workspace-sidebar-open')

    await toggle.trigger('click')
    await wrapper.get('[data-testid="workspace-sidebar-close"]').trigger('click')

    expect(shell.classes()).not.toContain('workspace-sidebar-open')
  })

  it('closes the sidebar after navigating from the mobile drawer', async () => {
    const wrapper = mount(WorkspaceLayout, {
      global: {
        stubs: {
          RouterView: {
            template: '<main data-testid="workspace-route-page">page</main>'
          }
        }
      }
    })
    await flushPromises()

    await wrapper.get('[data-testid="workspace-sidebar-toggle"]').trigger('click')
    await wrapper.get('[data-testid="workspace-nav-text-to-image"]').trigger('click')

    expect(routerPush).toHaveBeenCalledWith('/workspace')
    expect(wrapper.get('.workspace-with-sidebar').classes()).not.toContain('workspace-sidebar-open')
  })

  it('shows the session loading error instead of rendering workspace content when session fails unexpectedly', async () => {
    apiMocks.getMe.mockRejectedValueOnce(Object.assign(new Error('用户服务暂不可用'), { status: 500 }))
    const wrapper = mount(WorkspaceLayout, {
      global: {
        stubs: {
          RouterView: {
            template: '<main data-testid="workspace-route-page">page</main>'
          }
        }
      }
    })
    await flushPromises()

    expect(wrapper.get('.workspace-error').text()).toContain('用户服务暂不可用')
    expect(wrapper.find('[data-testid="workspace-route-page"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="workspace-sidebar-shell"]').exists()).toBe(false)
  })

  it('highlights the direct workspace route in the sidebar for logged-in users', async () => {
    const routes = [
      ['/workspace/video', 'workspace-nav-text-to-video'],
      ['/workspace', 'workspace-nav-text-to-image']
    ]

    for (const [path, testId] of routes) {
      routeState.path = path
      const wrapper = mount(WorkspaceLayout, {
        global: {
          stubs: {
            RouterView: {
              template: '<main data-testid="workspace-route-page">page</main>'
            }
          }
        }
      })
      await flushPromises()

      expect(wrapper.get(`[data-testid="${testId}"]`).classes()).toContain('active')
      wrapper.unmount()
    }
  })

  it('opens the auth modal before guests open protected sidebar entries or recharge', async () => {
    apiMocks.getMe.mockRejectedValueOnce(Object.assign(new Error('unauthorized'), { status: 401 }))
    const wrapper = mount(WorkspaceLayout, {
      global: {
        stubs: {
          RouterView: {
            template: '<main data-testid="workspace-route-page">page</main>'
          }
        }
      }
    })
    await flushPromises()
    apiMocks.getMe.mockRejectedValue(Object.assign(new Error('unauthorized'), { status: 401 }))

    await wrapper.get('[data-testid="workspace-nav-works"]').trigger('click')
    await flushPromises()
    expect(window.confirm).not.toHaveBeenCalled()
    expect(routerPush).not.toHaveBeenCalledWith({ path: '/login', query: { redirect: '/works' } })

    expect(authModalState.open).toBe(true)
    expect(authModalState.redirect).toBe('/works')
  })

  it('opens protected sidebar entries when shared state is empty but cookie session is valid', async () => {
    const wrapper = mount(WorkspaceLayout, {
      global: {
        stubs: {
          RouterView: {
            template: '<main data-testid="workspace-route-page">page</main>'
          }
        }
      }
    })
    await flushPromises()

    clearCurrentUser()
    apiMocks.getMe.mockResolvedValueOnce({
      username: 'cookie-user',
      available_credits: 31,
      tier: 'Pro'
    })

    await wrapper.get('[data-testid="workspace-nav-text-to-video"]').trigger('click')
    await flushPromises()

    expect(window.confirm).not.toHaveBeenCalled()
    expect(routerPush).toHaveBeenLastCalledWith('/workspace/video')
    expect(currentUser.value?.username).toBe('cookie-user')

    clearCurrentUser()
    apiMocks.getMe.mockResolvedValueOnce({
      username: 'cookie-user',
      available_credits: 31,
      tier: 'Pro'
    })

    await wrapper.get('[data-testid="workspace-nav-novel-video"]').trigger('click')
    await flushPromises()

    expect(window.confirm).not.toHaveBeenCalled()
    expect(routerPush).toHaveBeenLastCalledWith('/workspace/novel-video')

    clearCurrentUser()
    apiMocks.getMe.mockResolvedValueOnce({
      username: 'cookie-user',
      available_credits: 31,
      tier: 'Pro'
    })

    await wrapper.get('[data-testid="workspace-nav-assets"]').trigger('click')
    await flushPromises()

    expect(window.confirm).not.toHaveBeenCalled()
    expect(routerPush).toHaveBeenLastCalledWith('/assets')

    clearCurrentUser()
    apiMocks.getMe.mockResolvedValueOnce({
      username: 'cookie-user',
      available_credits: 31,
      tier: 'Pro'
    })

    await wrapper.get('[data-testid="workspace-nav-works"]').trigger('click')
    await flushPromises()

    expect(window.confirm).not.toHaveBeenCalled()
    expect(routerPush).toHaveBeenLastCalledWith('/works')
  })

  it('redirects a real 401 sidebar click to login with the clicked route as redirect', async () => {
    setCurrentUser({
      username: 'stale-user',
      available_credits: 12
    })
    const wrapper = mount(WorkspaceLayout, {
      global: {
        stubs: {
          RouterView: {
            template: '<main data-testid="workspace-route-page">page</main>'
          }
        }
      }
    })
    await flushPromises()

    apiMocks.getMe.mockRejectedValueOnce(Object.assign(new Error('unauthorized'), { status: 401 }))

    await wrapper.get('[data-testid="workspace-nav-text-to-video"]').trigger('click')
    await flushPromises()

    expect(window.confirm).not.toHaveBeenCalled()
    expect(authModalState.open).toBe(true)
    expect(authModalState.redirect).toBe('/workspace/video')
    expect(currentUser.value).toBeNull()
  })

  it('keeps album detail routes accessible without adding album sidebar entries', async () => {
    routeState.path = '/workspace/couple-album/42'
    const wrapper = mount(WorkspaceLayout, {
      global: {
        stubs: {
          RouterView: {
            template: '<main data-testid="workspace-route-page">page</main>'
          }
        }
      }
    })
    await flushPromises()

    expect(wrapper.get('[data-testid="workspace-nav-text-to-image"]').text()).toContain('图像工坊')
    expect(wrapper.find('[data-testid="workspace-nav-couple-album"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="workspace-nav-childhood-dream-album"]').exists()).toBe(false)
  })

  it('updates sidebar credits from the shared session state without remounting', async () => {
    const wrapper = mount(WorkspaceLayout, {
      global: {
        stubs: {
          RouterView: {
            template: '<main data-testid="workspace-route-page">page</main>'
          }
        }
      }
    })
    await flushPromises()

    expect(wrapper.text()).toContain('12')

    applyAvailableCredits(9)
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('9')
  })

  it('uses the full sidebar menu on the image generator route', async () => {
    routeState.path = '/workspace'
    const wrapper = mount(WorkspaceLayout, {
      global: {
        stubs: {
          RouterView: {
            template: '<main data-testid="workspace-route-page">page</main>'
          }
        }
      }
    })
    await flushPromises()

    expect(wrapper.get('.workspace-with-sidebar').classes()).not.toContain('workspace-image-generator-shell')
    expect(wrapper.get('[data-testid="workspace-nav-text-to-image"]').text()).toContain('图像工坊')
    expect(wrapper.find('[data-testid="workspace-nav-old-photo-restoration"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="workspace-nav-couple-album"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="workspace-nav-childhood-dream-album"]').exists()).toBe(false)
  })

  it('keeps the same full sidebar menu on secondary workspace routes', async () => {
    routeState.path = '/workspace/video'
    const wrapper = mount(WorkspaceLayout, {
      global: {
        stubs: {
          RouterView: {
            template: '<main data-testid="workspace-route-page">page</main>'
          }
        }
      }
    })
    await flushPromises()

    expect(wrapper.get('.workspace-with-sidebar').classes()).not.toContain('workspace-image-generator-shell')
    expect(wrapper.get('[data-testid="workspace-nav-text-to-image"]').text()).toContain('图像工坊')
    expect(wrapper.find('[data-testid="workspace-nav-old-photo-restoration"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="workspace-nav-couple-album"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="workspace-nav-childhood-dream-album"]').exists()).toBe(false)
  })

  it('starts workspace presence heartbeat after the user session loads', async () => {
    vi.useFakeTimers()
    const wrapper = mount(WorkspaceLayout, {
      global: {
        stubs: {
          RouterView: {
            template: '<main data-testid="workspace-route-page">page</main>'
          }
        }
      }
    })
    await flushPromises()

    expect(apiMocks.pingPresence).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(60_000)
    expect(apiMocks.pingPresence).toHaveBeenCalledTimes(2)

    wrapper.unmount()
    vi.useRealTimers()
  })

  it('routes user center account links through the protected navigation and closes the mobile drawer', async () => {
    const wrapper = mount(WorkspaceLayout, {
      global: {
        stubs: {
          RouterView: {
            template: '<main data-testid="workspace-route-page">page</main>'
          }
        }
      }
    })
    await flushPromises()

    await wrapper.get('[data-testid="workspace-sidebar-toggle"]').trigger('click')
    await wrapper.get('[data-testid="workspace-user-menu-trigger"]').trigger('click')
    getTeleportedUserMenuItem('workspace-user-menu-credits').click()
    await flushPromises()

    expect(routerPush).toHaveBeenLastCalledWith('/account#credits')
    expect(wrapper.get('.workspace-with-sidebar').classes()).not.toContain('workspace-sidebar-open')

    await wrapper.get('[data-testid="workspace-user-menu-trigger"]').trigger('click')
    getTeleportedUserMenuItem('workspace-user-menu-ledger').click()
    await flushPromises()
    expect(routerPush).toHaveBeenLastCalledWith('/account#ledger')

    await wrapper.get('[data-testid="workspace-user-menu-trigger"]').trigger('click')
    getTeleportedUserMenuItem('workspace-user-menu-profile').click()
    await flushPromises()
    expect(routerPush).toHaveBeenLastCalledWith('/account#profile')

    await wrapper.get('[data-testid="workspace-user-menu-trigger"]').trigger('click')
    getTeleportedUserMenuItem('workspace-user-menu-pricing').click()
    await flushPromises()
    expect(routerPush).toHaveBeenLastCalledWith('/pricing')
    expect(wrapper.get('.workspace-with-sidebar').classes()).not.toContain('workspace-sidebar-open')
  })

  it('opens user center support without login protection and enterprise upgrade through pricing', async () => {
    apiMocks.getMe.mockRejectedValueOnce(Object.assign(new Error('unauthorized'), { status: 401 }))
    const wrapper = mount(WorkspaceLayout, {
      global: {
        stubs: {
          RouterView: {
            template: '<main data-testid="workspace-route-page">page</main>'
          }
        }
      }
    })
    await flushPromises()
    apiMocks.getMe.mockRejectedValue(Object.assign(new Error('unauthorized'), { status: 401 }))

    await wrapper.get('[data-testid="workspace-nav-support"]').trigger('click')
    expect(window.confirm).not.toHaveBeenCalled()
    expect(routerPush).toHaveBeenLastCalledWith('/contact')

    await wrapper.get('.recharge-button').trigger('click')
    expect(authModalState.open).toBe(true)
    expect(authModalState.redirect).toBe('/workspace')

    setCurrentUser({ username: 'creator', available_credits: 12 })
    await wrapper.get('.recharge-button').trigger('click')
    expect(routerPush).toHaveBeenLastCalledWith('/pricing')
  })

  it('logs out from the workspace user center menu and clears the mobile drawer state', async () => {
    const wrapper = mount(WorkspaceLayout, {
      global: {
        stubs: {
          RouterView: {
            template: '<main data-testid="workspace-route-page">page</main>'
          }
        }
      }
    })
    await flushPromises()

    await wrapper.get('[data-testid="workspace-sidebar-toggle"]').trigger('click')
    expect(wrapper.get('.workspace-with-sidebar').classes()).toContain('workspace-sidebar-open')

    await wrapper.get('[data-testid="workspace-user-menu-trigger"]').trigger('click')
    getTeleportedUserMenuItem('workspace-user-menu-logout').click()
    await flushPromises()

    expect(apiMocks.logout).toHaveBeenCalled()
    expect(currentUser.value).toBeNull()
    expect(routerPush).toHaveBeenLastCalledWith('/login')
    expect(wrapper.get('.workspace-with-sidebar').classes()).not.toContain('workspace-sidebar-open')
  })
})
