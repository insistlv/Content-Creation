import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  getMe: vi.fn(),
  listWorks: vi.fn(),
  listReferenceAssets: vi.fn(),
  getWorkspaceDiscovery: vi.fn(),
  createImageGeneration: vi.fn(),
  getImageGeneration: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    getMe: apiMocks.getMe,
    listWorks: apiMocks.listWorks,
    listReferenceAssets: apiMocks.listReferenceAssets,
    getWorkspaceDiscovery: apiMocks.getWorkspaceDiscovery,
    createImageGeneration: apiMocks.createImageGeneration,
    getImageGeneration: apiMocks.getImageGeneration
  }
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: vi.fn()
  })
}))

import WorkspaceView from '../views/WorkspaceView.vue'

function imageRect({ left, top, width, height }) {
  return {
    left,
    top,
    width,
    height,
    right: left + width,
    bottom: top + height,
    x: left,
    y: top,
    toJSON: () => {}
  }
}

describe('WorkspaceView preview zoom', () => {
  beforeEach(() => {
    apiMocks.listReferenceAssets.mockResolvedValue({ items: [] })
    apiMocks.getWorkspaceDiscovery.mockResolvedValue({
      tools: [],
      models: [],
      hot: [],
      inspiration: []
    })
  })

  afterEach(() => {
    document.body.innerHTML = ''
    document.body.className = ''
    vi.clearAllMocks()
  })

  it('opens the enlarged preview as a top-level fixed dialog', async () => {
    apiMocks.getMe.mockResolvedValueOnce({
      user_id: 9,
      username: 'creator_09',
      display_name: '栏目主理人',
      available_credits: 3
    })
    apiMocks.listWorks.mockResolvedValueOnce({
      items: [
        {
          work_id: 90,
          prompt: 'wide night campus panorama',
          preview_url: '/api/works/90/file',
          download_url: '/api/works/90/download',
          created_at: '2026-04-30T08:31:16Z'
        }
      ]
    })

    const wrapper = mount(WorkspaceView, { attachTo: document.body })
    await flushPromises()
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="workspace-tab-create"]').trigger('click')
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="workspace-preview-zoom-button"]').trigger('click')
    await wrapper.vm.$nextTick()

    const modal = document.body.querySelector('[data-testid="workspace-preview-modal"]')
    expect(modal).toBeTruthy()
    expect(wrapper.element.contains(modal)).toBe(false)
    expect(modal.getAttribute('role')).toBe('dialog')
    expect(modal.getAttribute('aria-modal')).toBe('true')
    expect(modal.classList.contains('workspace-preview-zoom')).toBe(true)
    expect(modal.querySelector('img')?.getAttribute('src')).toBe('/api/works/90/file')
    expect(modal.querySelector('a')?.getAttribute('href')).toBe('/api/works/90/download')

    await modal.querySelector('[data-testid="workspace-preview-close"]').click()
    await wrapper.vm.$nextTick()

    expect(document.body.querySelector('[data-testid="workspace-preview-modal"]')).toBeNull()
    wrapper.unmount()
  })

  it('zooms the preview image around the mouse position with translate and scale', async () => {
    apiMocks.getMe.mockResolvedValueOnce({
      user_id: 9,
      username: 'creator_09',
      display_name: '栏目主理人',
      available_credits: 3
    })
    apiMocks.listWorks.mockResolvedValueOnce({
      items: [
        {
          work_id: 90,
          prompt: 'wide night campus panorama',
          preview_url: '/api/works/90/file',
          download_url: '/api/works/90/download',
          created_at: '2026-04-30T08:31:16Z'
        }
      ]
    })

    const wrapper = mount(WorkspaceView, { attachTo: document.body })
    await flushPromises()
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="workspace-tab-create"]').trigger('click')
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="workspace-preview-zoom-button"]').trigger('click')
    await wrapper.vm.$nextTick()

    const zoomSurface = document.body.querySelector('[data-testid="workspace-preview-zoom-surface"]')
    const zoomImage = document.body.querySelector('[data-testid="workspace-preview-zoom-image"]')
    zoomImage.getBoundingClientRect = vi.fn(() => imageRect({
      left: 100,
      top: 50,
      width: 400,
      height: 300
    }))

    zoomSurface.dispatchEvent(new WheelEvent('wheel', {
      bubbles: true,
      cancelable: true,
      deltaY: -100,
      clientX: 300,
      clientY: 125
    }))
    await wrapper.vm.$nextTick()

    expect(zoomImage.style.transform).toBe('translate(-40px, -15px) scale(1.2)')
    expect(zoomImage.style.transformOrigin).toBe('0px 0px')
    wrapper.unmount()
  })

  it('keeps the pointed image pixel stable across consecutive wheel zooms', async () => {
    apiMocks.getMe.mockResolvedValueOnce({
      user_id: 9,
      username: 'creator_09',
      display_name: '栏目主理人',
      available_credits: 3
    })
    apiMocks.listWorks.mockResolvedValueOnce({
      items: [
        {
          work_id: 90,
          prompt: 'wide night campus panorama',
          preview_url: '/api/works/90/file',
          download_url: '/api/works/90/download',
          created_at: '2026-04-30T08:31:16Z'
        }
      ]
    })

    const wrapper = mount(WorkspaceView, { attachTo: document.body })
    await flushPromises()
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="workspace-tab-create"]').trigger('click')
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="workspace-preview-zoom-button"]').trigger('click')
    await wrapper.vm.$nextTick()

    const zoomSurface = document.body.querySelector('[data-testid="workspace-preview-zoom-surface"]')
    const zoomImage = document.body.querySelector('[data-testid="workspace-preview-zoom-image"]')
    zoomImage.getBoundingClientRect = vi.fn()
      .mockReturnValueOnce(imageRect({ left: 100, top: 50, width: 400, height: 300 }))
      .mockReturnValueOnce(imageRect({ left: 60, top: 35, width: 480, height: 360 }))

    zoomSurface.dispatchEvent(new WheelEvent('wheel', {
      bubbles: true,
      cancelable: true,
      deltaY: -100,
      clientX: 300,
      clientY: 125
    }))
    await wrapper.vm.$nextTick()

    zoomSurface.dispatchEvent(new WheelEvent('wheel', {
      bubbles: true,
      cancelable: true,
      deltaY: -100,
      clientX: 300,
      clientY: 125
    }))
    await wrapper.vm.$nextTick()

    expect(zoomImage.style.transform).toBe('translate(-80px, -30px) scale(1.4)')
    wrapper.unmount()
  })

  it('resets image translation when the preview zooms back to 1x', async () => {
    apiMocks.getMe.mockResolvedValueOnce({
      user_id: 9,
      username: 'creator_09',
      display_name: '栏目主理人',
      available_credits: 3
    })
    apiMocks.listWorks.mockResolvedValueOnce({
      items: [
        {
          work_id: 90,
          prompt: 'wide night campus panorama',
          preview_url: '/api/works/90/file',
          download_url: '/api/works/90/download',
          created_at: '2026-04-30T08:31:16Z'
        }
      ]
    })

    const wrapper = mount(WorkspaceView, { attachTo: document.body })
    await flushPromises()
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="workspace-tab-create"]').trigger('click')
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="workspace-preview-zoom-button"]').trigger('click')
    await wrapper.vm.$nextTick()

    const zoomSurface = document.body.querySelector('[data-testid="workspace-preview-zoom-surface"]')
    const zoomImage = document.body.querySelector('[data-testid="workspace-preview-zoom-image"]')
    zoomImage.getBoundingClientRect = vi.fn()
      .mockReturnValueOnce(imageRect({ left: 100, top: 50, width: 400, height: 300 }))
      .mockReturnValueOnce(imageRect({ left: 60, top: 35, width: 480, height: 360 }))

    zoomSurface.dispatchEvent(new WheelEvent('wheel', {
      bubbles: true,
      cancelable: true,
      deltaY: -100,
      clientX: 300,
      clientY: 125
    }))
    await wrapper.vm.$nextTick()

    zoomSurface.dispatchEvent(new WheelEvent('wheel', {
      bubbles: true,
      cancelable: true,
      deltaY: 100,
      clientX: 300,
      clientY: 125
    }))
    await wrapper.vm.$nextTick()

    expect(zoomImage.style.transform).toBe('translate(0px, 0px) scale(1)')
    wrapper.unmount()
  })
})
