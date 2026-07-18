import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it } from 'vitest'
import App from '../App.vue'

describe('global image double-click preview', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('opens an enlarged preview for any routed image on double click', async () => {
    const wrapper = mount(App, {
      attachTo: document.body,
      global: {
        stubs: {
          RouterView: {
            template: '<main><img src="/api/works/10/file" alt="作品图" /></main>'
          }
        }
      }
    })

    await wrapper.get('img').trigger('dblclick')
    await wrapper.vm.$nextTick()

    const modal = document.body.querySelector('[data-testid="global-image-preview-modal"]')
    expect(modal).toBeTruthy()
    expect(modal.getAttribute('role')).toBe('dialog')
    expect(modal.getAttribute('aria-modal')).toBe('true')
    expect(modal.querySelector('img')?.getAttribute('src')).toBe('/api/works/10/file')
    expect(modal.querySelector('img')?.getAttribute('alt')).toBe('作品图')

    await modal.querySelector('[data-testid="global-image-preview-close"]').click()
    await wrapper.vm.$nextTick()

    expect(document.body.querySelector('[data-testid="global-image-preview-modal"]')).toBeNull()
    wrapper.unmount()
  })

  it('leaves images with their own preview handler alone', async () => {
    const wrapper = mount(App, {
      attachTo: document.body,
      global: {
        stubs: {
          RouterView: {
            template: '<main><img src="/api/works/20/file" alt="已有放大逻辑" data-skip-global-image-preview /></main>'
          }
        }
      }
    })

    await wrapper.get('img').trigger('dblclick')
    await wrapper.vm.$nextTick()

    expect(document.body.querySelector('[data-testid="global-image-preview-modal"]')).toBeNull()
    wrapper.unmount()
  })
})
