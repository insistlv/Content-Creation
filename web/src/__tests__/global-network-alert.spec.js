import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

import App from '../App.vue'

function mountApp() {
  return mount(App, {
    attachTo: document.body,
    global: {
      stubs: {
        RouterView: {
          template: '<main>页面内容</main>'
        }
      }
    }
  })
}

function dispatchNetworkError(path = '/api/packages') {
  window.dispatchEvent(new CustomEvent('image-agent:network-error', {
    detail: {
      code: 'network_unreachable',
      message: '网络连接不稳定，暂时无法连接服务器，请稍后重试',
      method: 'GET',
      path,
      online: true,
      timestamp: new Date().toISOString()
    }
  }))
}

describe('global network alert', () => {
  afterEach(() => {
    vi.useRealTimers()
    document.body.innerHTML = ''
  })

  it('shows network errors in a global live alert and allows manual dismiss', async () => {
    const wrapper = mountApp()

    dispatchNetworkError('/api/packages')
    await wrapper.vm.$nextTick()

    const alert = document.body.querySelector('[data-testid="global-network-alert"]')
    expect(alert).toBeTruthy()
    expect(alert.getAttribute('role')).toBe('alert')
    expect(alert.getAttribute('aria-live')).toBe('assertive')
    expect(alert.textContent).toContain('网络连接不稳定，暂时无法连接服务器，请稍后重试')
    expect(alert.textContent).toContain('GET /api/packages')

    alert.querySelector('[data-testid="global-network-alert-close"]').click()
    await wrapper.vm.$nextTick()

    expect(document.body.querySelector('[data-testid="global-network-alert"]')).toBeNull()
    wrapper.unmount()
  })

  it('auto dismisses and deduplicates repeated network errors for the same path', async () => {
    vi.useFakeTimers()
    const wrapper = mountApp()

    dispatchNetworkError('/api/packages')
    dispatchNetworkError('/api/packages')
    await wrapper.vm.$nextTick()

    expect(document.body.querySelectorAll('[data-testid="global-network-alert"]')).toHaveLength(1)

    vi.advanceTimersByTime(6000)
    await wrapper.vm.$nextTick()

    expect(document.body.querySelector('[data-testid="global-network-alert"]')).toBeNull()
    wrapper.unmount()
  })
})
