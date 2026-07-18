import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  planMomentsMarketing: vi.fn(),
  createImageGeneration: vi.fn(),
  uploadReferenceAsset: vi.fn(),
  getImageGeneration: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    planMomentsMarketing: apiMocks.planMomentsMarketing,
    createImageGeneration: apiMocks.createImageGeneration,
    uploadReferenceAsset: apiMocks.uploadReferenceAsset,
    getImageGeneration: apiMocks.getImageGeneration
  }
}))

vi.mock('vue-router', () => ({
  RouterLink: {
    name: 'RouterLink',
    props: ['to'],
    template: '<a :href="typeof to === \'string\' ? to : to.path"><slot /></a>'
  }
}))

import MomentsMarketingWorkspaceView from '../views/MomentsMarketingWorkspaceView.vue'
import ThemeToggle from '../components/ThemeToggle.vue'

const momentsWorkspaceSource = readFileSync(resolve(__dirname, '../views/MomentsMarketingWorkspaceView.vue'), 'utf8')

describe('MomentsMarketingWorkspaceView', () => {
  const wrappers = []

  function planPayload(imageCount = 2) {
    return {
      moments_text: '今天想把这家巷口咖啡推荐给附近朋友，现磨咖啡和低糖甜点都很稳。',
      hashtags: ['附近好店'],
      safety_notes: ['已避免绝对化承诺'],
      image_cards: Array.from({ length: imageCount }, (_, index) => ({
        slot: index + 1,
        role: index === 0 ? '开场' : '产品',
        caption: index === 0 ? '门店氛围' : '招牌拿铁',
        visual_prompt: `社区咖啡店宣传图 ${index + 1}，真实商业摄影，无文字`,
        overlay_title: index === 0 ? '巷口咖啡' : '招牌拿铁',
        overlay_subtitle: '现磨咖啡和低糖甜点',
        overlay_badge: '第二杯半价',
        cta: '私信预约',
        layout: 'bottom_gradient'
      }))
    }
  }

  async function mountReady() {
    const wrapper = mount(MomentsMarketingWorkspaceView, {
      global: {
        stubs: {
          RouterLink: {
            props: ['to'],
            template: '<a :href="typeof to === \'string\' ? to : to.path"><slot /></a>'
          }
        }
      }
    })
    wrappers.push(wrapper)
    await flushPromises()
    return wrapper
  }

  beforeEach(() => {
    apiMocks.planMomentsMarketing.mockResolvedValue(planPayload())
    apiMocks.createImageGeneration.mockImplementation((payload) => Promise.resolve({
      generation_id: 100 + Number(payload.batch_index || 1),
      status: 'queued',
      stage: 'queued',
      prompt: payload.prompt,
      parameters: payload
    }))
    apiMocks.uploadReferenceAsset.mockResolvedValue({
      id: 71,
      preview_url: '/api/reference-assets/71/file',
      original_filename: 'shop.png'
    })
  })

  afterEach(() => {
    wrappers.splice(0).forEach((wrapper) => wrapper.unmount())
    vi.resetAllMocks()
    window.localStorage.clear()
  })

  it('plans text marketing and creates one image task for each image card', async () => {
    const wrapper = await mountReady()

    await wrapper.get('[data-testid="moments-product-name"]').setValue('巷口咖啡')
    await wrapper.get('[data-testid="moments-selling-points"]').setValue('现磨咖啡、低糖甜点')
    await wrapper.get('[data-testid="moments-target-audience"]').setValue('附近上班族')
    await wrapper.get('[data-testid="moments-promotion"]').setValue('第二杯半价')
    await wrapper.get('[data-testid="moments-image-count"]').setValue('2')
    await wrapper.get('[data-testid="moments-plan-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.planMomentsMarketing).toHaveBeenCalledWith(expect.objectContaining({
      input_mode: 'text',
      output_type: 'copy_image_separate',
      image_count: 2,
      product_name: '巷口咖啡'
    }))
    expect(apiMocks.createImageGeneration).toHaveBeenCalledTimes(2)
    expect(apiMocks.createImageGeneration).toHaveBeenNthCalledWith(1, expect.objectContaining({
      prompt: expect.stringContaining('社区咖啡店宣传图 1'),
      aspect_ratio: '1:1',
      batch_index: 1,
      batch_total: 2
    }))
    expect(apiMocks.createImageGeneration.mock.calls[0][0].batch_id).toMatch(/^moments-/)
    expect(wrapper.text()).toContain('今天想把这家巷口咖啡推荐给附近朋友')
    expect(wrapper.get('[data-testid="moments-task-101"]').text()).toContain('开场')
  })

  it('uses the shared light theme and follows the global theme toggle back to dark', async () => {
    window.localStorage.setItem('image_agent_user_theme:v1', 'light')
    const wrapper = mount({
      components: { MomentsMarketingWorkspaceView, ThemeToggle },
      template: '<ThemeToggle /><MomentsMarketingWorkspaceView />',
      global: {
        stubs: {
          RouterLink: {
            props: ['to'],
            template: '<a :href="typeof to === \'string\' ? to : to.path"><slot /></a>'
          }
        }
      }
    })
    wrappers.push(wrapper)
    await flushPromises()

    const shell = wrapper.get('.moments-workspace')
    const toggle = wrapper.get('[data-testid="site-theme-toggle"]')

    expect(shell.attributes('data-theme')).toBe('light')
    expect(shell.classes()).toContain('moments-workspace-light')
    expect(toggle.attributes('aria-label')).toBe('切换到暗色模式')

    await toggle.trigger('click')

    expect(shell.attributes('data-theme')).toBe('dark')
    expect(shell.classes()).toContain('moments-workspace-dark')
    expect(window.localStorage.getItem('image_agent_user_theme:v1')).toBe('dark')
  })

  it('defines theme tokens for moments marketing workspace surfaces', () => {
    expect(momentsWorkspaceSource).toContain('.moments-workspace[data-theme="light"]')
    expect(momentsWorkspaceSource).toContain('.moments-workspace[data-theme="dark"]')
    for (const token of [
      '--moments-bg:',
      '--moments-panel:',
      '--moments-panel-muted:',
      '--moments-input:',
      '--moments-border:',
      '--moments-text:',
      '--moments-muted:',
      '--moments-accent:',
      '--moments-action-bg:',
      '--moments-media-bg:'
    ]) {
      expect(momentsWorkspaceSource).toContain(token)
    }
  })

  it('uses theme tokens for major moments marketing surfaces instead of fixed light colors', () => {
    const styleSource = momentsWorkspaceSource.slice(momentsWorkspaceSource.indexOf('<style scoped>'))

    expect(styleSource).toContain('background: var(--moments-bg);')
    expect(styleSource).toContain('background: var(--moments-panel);')
    expect(styleSource).toContain('background: var(--moments-input);')
    expect(styleSource).toContain('border: 1px solid var(--moments-border);')
    expect(styleSource).toContain('color: var(--moments-text);')
    expect(styleSource).not.toContain('background: #ffffff;')
    expect(styleSource).not.toContain('background: #f5f7f6;')
  })

  it('uploads photo references and sends them to generated image tasks', async () => {
    const wrapper = await mountReady()

    await wrapper.get('[data-testid="moments-input-photo"]').setValue(true)
    const fileInput = wrapper.get('[data-testid="moments-reference-input"]')
    Object.defineProperty(fileInput.element, 'files', {
      configurable: true,
      value: [new File(['fake'], 'shop.png', { type: 'image/png' })]
    })
    await fileInput.trigger('change')
    await flushPromises()

    await wrapper.get('[data-testid="moments-product-name"]').setValue('巷口咖啡')
    await wrapper.get('[data-testid="moments-selling-points"]').setValue('现磨咖啡、低糖甜点')
    await wrapper.get('[data-testid="moments-image-count"]').setValue('2')
    await wrapper.get('[data-testid="moments-plan-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.uploadReferenceAsset).toHaveBeenCalledTimes(1)
    expect(apiMocks.planMomentsMarketing).toHaveBeenCalledWith(expect.objectContaining({
      input_mode: 'photo',
      reference_asset_ids: [71]
    }))
    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith(expect.objectContaining({
      reference_asset_ids: [71],
      reference_weight: 80,
      reference_intent: 'compose'
    }))
  })
})
