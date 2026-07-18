import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

import WorkspaceSidebar from '../components/WorkspaceSidebar.vue'

const originalInnerWidth = window.innerWidth
const originalInnerHeight = window.innerHeight

function setViewport(width, height = 800) {
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: width })
  Object.defineProperty(window, 'innerHeight', { configurable: true, value: height })
}

function rect({ left, top, width, height }) {
  return {
    x: left,
    y: top,
    left,
    top,
    width,
    height,
    right: left + width,
    bottom: top + height,
    toJSON: () => {}
  }
}

function getTeleportedUserMenu() {
  return document.body.querySelector('[data-testid="workspace-user-menu"]')
}

describe('WorkspaceSidebar', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    document.body.innerHTML = ''
    setViewport(originalInnerWidth, originalInnerHeight)
  })

  it('shows a soft unavailable prompt for workspace tool items without pages', async () => {
    const alertSpy = vi.spyOn(window, 'alert').mockImplementation(() => {})
    const wrapper = mount(WorkspaceSidebar, {
      props: {
        currentRoute: '/workspace',
        me: { username: 'creator', available_credits: 8 }
      }
    })

    await wrapper.get('[data-testid="workspace-nav-ai-avatar"]').trigger('click')

    expect(alertSpy).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="workspace-unavailable-tip"]').text()).toBe('功能暂未开放，敬请期待!')
    expect(wrapper.emitted('navigate')).toBeUndefined()
  })

  it('collapses the workspace drawer on non-workspace routes and expands it on click', async () => {
    const wrapper = mount(WorkspaceSidebar, {
      props: {
        currentRoute: '/works',
        me: { username: 'creator', available_credits: 8 }
      }
    })

    expect(wrapper.find('[data-testid="workspace-nav-subdrawer"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="workspace-nav-text-to-image"]').exists()).toBe(false)

    const workspaceItem = wrapper.get('[data-testid="workspace-nav-workspace"]')
    expect(workspaceItem.attributes('aria-expanded')).toBe('false')
    expect(workspaceItem.find('.nav-parent-chevron').exists()).toBe(true)

    await workspaceItem.trigger('click')

    expect(workspaceItem.attributes('aria-expanded')).toBe('true')
    const subdrawerText = wrapper.get('[data-testid="workspace-nav-subdrawer"]').text()
    expect(subdrawerText).toContain('AI创作')
    expect(subdrawerText).not.toContain('AI工具')
    expect(subdrawerText).toContain('图像工坊')
    expect(subdrawerText).toContain('视频生成')
    expect(subdrawerText).toContain('小说视频')
    expect(wrapper.emitted('navigate')?.[0]).toEqual(['/workspace'])
  })

  it('auto-expands the workspace drawer on workspace routes and collapses it when another primary item is clicked', async () => {
    const wrapper = mount(WorkspaceSidebar, {
      props: {
        currentRoute: '/workspace/video',
        me: { username: 'creator', available_credits: 8 }
      }
    })

    expect(wrapper.get('[data-testid="workspace-nav-workspace"]').classes()).toContain('active')
    expect(wrapper.find('[data-testid="workspace-nav-subdrawer"]').exists()).toBe(true)

    await wrapper.get('[data-testid="workspace-nav-works"]').trigger('click')

    expect(wrapper.find('[data-testid="workspace-nav-subdrawer"]').exists()).toBe(false)
    expect(wrapper.emitted('navigate')?.[0]).toEqual(['/works'])
  })

  it('toggles the workspace drawer when workspace is re-clicked on a workspace route', async () => {
    const wrapper = mount(WorkspaceSidebar, {
      props: {
        currentRoute: '/workspace',
        me: { username: 'creator', available_credits: 8 }
      }
    })

    expect(wrapper.find('[data-testid="workspace-nav-subdrawer"]').exists()).toBe(true)

    await wrapper.get('[data-testid="workspace-nav-workspace"]').trigger('click')
    expect(wrapper.find('[data-testid="workspace-nav-subdrawer"]').exists()).toBe(false)

    await wrapper.get('[data-testid="workspace-nav-workspace"]').trigger('click')
    expect(wrapper.find('[data-testid="workspace-nav-subdrawer"]').exists()).toBe(true)
  })

  it('shows one unified image generation entry and no standalone image-to-image entry', async () => {
    const alertSpy = vi.spyOn(window, 'alert').mockImplementation(() => {})
    const wrapper = mount(WorkspaceSidebar, {
      props: {
        currentRoute: '/workspace',
        me: { username: 'creator', available_credits: 8 }
      }
    })

    expect(wrapper.find('[data-testid="workspace-nav-image-to-image"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="workspace-nav-text-to-image"]').text()).toContain('图像工坊')

    await wrapper.get('[data-testid="workspace-nav-text-to-image"]').trigger('click')

    expect(alertSpy).not.toHaveBeenCalled()
    expect(wrapper.emitted('navigate')?.[0]).toEqual(['/workspace'])
  })

  it('shows one AI commerce entry and navigates to its workspace', async () => {
    const wrapper = mount(WorkspaceSidebar, { props: { currentRoute: '/workspace', me: { username: 'creator' } } })
    expect(wrapper.findAll('[data-testid="workspace-nav-ai-commerce"]')).toHaveLength(1)
    await wrapper.get('[data-testid="workspace-nav-ai-commerce"]').trigger('click')
    expect(wrapper.emitted('navigate')?.[0]).toEqual(['/workspace/ai-commerce'])
  })

  it('shows the streamlined AI generation menu without standalone album entries', () => {
    const wrapper = mount(WorkspaceSidebar, {
      props: {
        currentRoute: '/workspace',
        me: { username: 'creator', available_credits: 8 }
      }
    })

    expect(wrapper.get('[data-testid="workspace-nav-text-to-image"]').text()).toContain('图像工坊')
    expect(wrapper.find('[data-testid="workspace-nav-couple-album"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="workspace-nav-childhood-dream-album"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="workspace-nav-text-to-image"]').text()).not.toBe('生成')
  })

  it('keeps album routes out of the sidebar even when they are directly visited', () => {
    const wrapper = mount(WorkspaceSidebar, {
      props: {
        currentRoute: '/workspace/childhood-dream-album',
        me: { username: 'creator', available_credits: 8 }
      }
    })

    expect(wrapper.find('[data-testid="workspace-nav-couple-album"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="workspace-nav-childhood-dream-album"]').exists()).toBe(false)
  })

  it('keeps old photo restoration out of the sidebar AI tools menu', () => {
    const wrapper = mount(WorkspaceSidebar, {
      props: {
        currentRoute: '/workspace/old-photo-restoration',
        me: { username: 'creator', available_credits: 8 }
      }
    })

    expect(wrapper.find('[data-testid="workspace-nav-old-photo-restoration"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('老照片修复')
  })

  it('enables the assets library entry', async () => {
    const wrapper = mount(WorkspaceSidebar, {
      props: {
        currentRoute: '/assets',
        me: { username: 'creator', available_credits: 8 }
      }
    })

    await wrapper.get('[data-testid="workspace-nav-assets"]').trigger('click')

    expect(wrapper.get('[data-testid="workspace-nav-assets"]').text()).toContain('素材库')
    expect(wrapper.emitted('navigate')?.[0]).toEqual(['/assets'])
    expect(wrapper.get('[data-testid="workspace-nav-assets"]').classes()).toContain('active')
  })

  it('keeps pricing out of the primary sidebar navigation', () => {
    const wrapper = mount(WorkspaceSidebar, {
      props: {
        currentRoute: '/works',
        me: { username: 'creator', available_credits: 8 }
      }
    })

    expect(wrapper.find('[data-testid="workspace-nav-pricing"]').exists()).toBe(false)
    expect(wrapper.get('.sidebar-nav').text()).not.toContain('套餐')
  })

  it('opens and closes the user center menu by click, hover, escape and outside click', async () => {
    const wrapper = mount(WorkspaceSidebar, {
      attachTo: document.body,
      props: {
        currentRoute: '/workspace',
        me: { username: 'creator', user_id: 'u_1001', available_credits: 1280, tier: 'Pro' }
      }
    })

    const trigger = wrapper.get('[data-testid="workspace-user-menu-trigger"]')
    expect(trigger.attributes('aria-expanded')).toBe('false')
    expect(getTeleportedUserMenu()).toBeNull()

    await trigger.trigger('click')
    expect(trigger.attributes('aria-expanded')).toBe('true')
    expect(getTeleportedUserMenu()?.textContent).toContain('u_1001')
    expect(getTeleportedUserMenu()?.textContent).toContain('Pro')
    expect(getTeleportedUserMenu()?.textContent).toContain('1,280')

    await trigger.trigger('click')
    expect(trigger.attributes('aria-expanded')).toBe('false')

    await trigger.trigger('mouseenter')
    expect(getTeleportedUserMenu()).not.toBeNull()

    getTeleportedUserMenu().dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await wrapper.vm.$nextTick()
    expect(getTeleportedUserMenu()).toBeNull()

    await trigger.trigger('mouseenter')
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await wrapper.vm.$nextTick()
    expect(getTeleportedUserMenu()).toBeNull()

    await trigger.trigger('mouseenter')
    document.body.dispatchEvent(new MouseEvent('pointerdown', { bubbles: true }))
    await wrapper.vm.$nextTick()
    expect(getTeleportedUserMenu()).toBeNull()

    wrapper.unmount()
  })

  it('keeps the user center menu open when a real click follows pointer hover', async () => {
    const wrapper = mount(WorkspaceSidebar, {
      attachTo: document.body,
      props: {
        currentRoute: '/workspace',
        me: { username: 'creator', user_id: 'u_1001', available_credits: 1280, tier: 'Pro' }
      }
    })

    const trigger = wrapper.get('[data-testid="workspace-user-menu-trigger"]')
    await trigger.trigger('mouseenter')
    await trigger.trigger('click')

    expect(trigger.attributes('aria-expanded')).toBe('true')
    expect(getTeleportedUserMenu()).not.toBeNull()

    wrapper.unmount()
  })

  it('teleports the user center menu to the body and positions it beside the sidebar on desktop', async () => {
    setViewport(1200, 800)
    const wrapper = mount(WorkspaceSidebar, {
      attachTo: document.body,
      props: {
        currentRoute: '/workspace',
        me: { username: 'creator', user_id: 'u_1001', available_credits: 1280, tier: 'Pro' }
      }
    })
    const trigger = wrapper.get('[data-testid="workspace-user-menu-trigger"]')
    vi.spyOn(trigger.element, 'getBoundingClientRect').mockReturnValue(rect({
      left: 16,
      top: 600,
      width: 228,
      height: 56
    }))

    await trigger.trigger('click')

    const menu = getTeleportedUserMenu()
    expect(menu).not.toBeNull()
    expect(menu.parentElement).toBe(document.body)
    expect(parseFloat(menu.style.left)).toBeGreaterThan(244)
    expect(parseFloat(menu.style.top)).toBeLessThan(656)
    expect(menu.style.position).toBe('fixed')
    expect(Number(menu.style.zIndex)).toBeGreaterThanOrEqual(1000)

    wrapper.unmount()
  })

  it('opens the teleported user center menu above the trigger on narrow screens', async () => {
    setViewport(390, 720)
    const wrapper = mount(WorkspaceSidebar, {
      attachTo: document.body,
      props: {
        currentRoute: '/workspace',
        me: { username: 'creator', user_id: 'u_1001', available_credits: 1280, tier: 'Pro' }
      }
    })
    const trigger = wrapper.get('[data-testid="workspace-user-menu-trigger"]')
    vi.spyOn(trigger.element, 'getBoundingClientRect').mockReturnValue(rect({
      left: 24,
      top: 580,
      width: 292,
      height: 52
    }))

    await trigger.trigger('click')

    const menu = getTeleportedUserMenu()
    expect(menu).not.toBeNull()
    expect(menu.parentElement).toBe(document.body)
    expect(menu.style.width).toBe('292px')
    expect(parseFloat(menu.style.top)).toBeLessThan(580)
    expect(menu.style.maxHeight).toBe('420px')
    expect(menu.style.overflowY).toBe('auto')

    wrapper.unmount()
  })

  it('emits user center menu actions and closes the menu after selecting an item', async () => {
    const wrapper = mount(WorkspaceSidebar, {
      props: {
        currentRoute: '/workspace',
        me: { username: 'creator', user_id: 'u_1001', available_credits: 8, tier: 'Pro' }
      }
    })

    await wrapper.get('[data-testid="workspace-user-menu-trigger"]').trigger('click')
    expect(getTeleportedUserMenu().textContent).toContain('点数套餐')
    expect(getTeleportedUserMenu().textContent).not.toContain('升级企业版')

    getTeleportedUserMenu().querySelector('[data-testid="workspace-user-menu-credits"]').click()
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('navigate')?.[0]).toEqual(['/account#credits'])
    expect(getTeleportedUserMenu()).toBeNull()

    await wrapper.get('[data-testid="workspace-user-menu-trigger"]').trigger('click')
    getTeleportedUserMenu().querySelector('[data-testid="workspace-user-menu-ledger"]').click()
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('navigate')?.[1]).toEqual(['/account#ledger'])

    await wrapper.get('[data-testid="workspace-user-menu-trigger"]').trigger('click')
    getTeleportedUserMenu().querySelector('[data-testid="workspace-user-menu-profile"]').click()
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('navigate')?.[2]).toEqual(['/account#profile'])

    await wrapper.get('[data-testid="workspace-user-menu-trigger"]').trigger('click')
    getTeleportedUserMenu().querySelector('[data-testid="workspace-user-menu-support"]').click()
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('support')?.[0]).toEqual([])

    await wrapper.get('[data-testid="workspace-user-menu-trigger"]').trigger('click')
    getTeleportedUserMenu().querySelector('[data-testid="workspace-user-menu-pricing"]').click()
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('recharge')?.[0]).toEqual([])
    expect(getTeleportedUserMenu()).toBeNull()

    await wrapper.get('[data-testid="workspace-user-menu-trigger"]').trigger('click')
    getTeleportedUserMenu().querySelector('[data-testid="workspace-user-menu-logout"]').click()
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('logout')?.[0]).toEqual([])
  })

  it('shows guest login and register actions without a user menu', async () => {
    const wrapper = mount(WorkspaceSidebar, {
      props: {
        currentRoute: '/workspace',
        me: null
      }
    })

    await wrapper.get('[data-testid="site-login-link"]').trigger('click')
    await wrapper.get('[data-testid="site-register-link"]').trigger('click')

    expect(wrapper.emitted('navigate')?.[0]).toEqual(['/login'])
    expect(wrapper.emitted('navigate')?.[1]).toEqual(['/register'])
    expect(wrapper.find('[data-testid="workspace-user-menu-trigger"]').exists()).toBe(false)
    expect(getTeleportedUserMenu()).toBeNull()
  })
})
