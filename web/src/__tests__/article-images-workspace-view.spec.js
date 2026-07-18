import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  planArticleImages: vi.fn(),
  createImageGeneration: vi.fn(),
  uploadReferenceAsset: vi.fn(),
  getImageGeneration: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    planArticleImages: apiMocks.planArticleImages,
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

import ArticleImagesWorkspaceView from '../views/ArticleImagesWorkspaceView.vue'

describe('ArticleImagesWorkspaceView', () => {
  const wrappers = []

  function planPayload(imageCount = 2) {
    return {
      article_summary: '这篇文章讲解品牌活动从预热到转化的完整方法。',
      safety_notes: ['图片中的标题由前端叠加，不要求模型生成中文文字。'],
      image_cards: Array.from({ length: imageCount }, (_, index) => ({
        slot: index + 1,
        role: index === 0 ? '封面图' : '段落配图',
        placement: index === 0 ? '文章开头' : '第二个小标题后',
        caption: index === 0 ? '活动增长方法论' : '三步拆解活动流程',
        visual_prompt: `清爽专业的公众号配图 ${index + 1}，蓝白配色，无文字，无水印`,
        aspect_ratio: index === 0 ? '16:9' : '1:1',
        overlay_title: index === 0 ? '活动增长方法论' : '三步拆解活动流程',
        layout: index === 0 ? 'cover_overlay' : 'step_card'
      }))
    }
  }

  async function mountReady() {
    const wrapper = mount(ArticleImagesWorkspaceView, {
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
    apiMocks.planArticleImages.mockResolvedValue(planPayload())
    apiMocks.createImageGeneration.mockImplementation((payload) => Promise.resolve({
      generation_id: 300 + Number(payload.batch_index || 1),
      status: 'queued',
      stage: 'queued',
      prompt: payload.prompt,
      parameters: payload
    }))
    apiMocks.uploadReferenceAsset.mockResolvedValue({
      id: 91,
      preview_url: '/api/reference-assets/91/file',
      original_filename: 'brand.png'
    })
  })

  afterEach(() => {
    wrappers.splice(0).forEach((wrapper) => wrapper.unmount())
    vi.resetAllMocks()
    window.localStorage.clear()
  })

  it('plans article images and creates one image task per card', async () => {
    const wrapper = await mountReady()

    await wrapper.get('[data-testid="article-images-title"]').setValue('活动增长方法论')
    await wrapper.get('[data-testid="article-images-body"]').setValue('第一段介绍活动背景。第二段说明三步流程。第三段总结复盘方法。')
    await wrapper.get('[data-testid="article-images-image-count"]').setValue('2')
    await wrapper.get('[data-testid="article-images-plan-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.planArticleImages).toHaveBeenCalledWith(expect.objectContaining({
      title: '活动增长方法论',
      body: '第一段介绍活动背景。第二段说明三步流程。第三段总结复盘方法。',
      image_count: 2,
      include_cover: true
    }))
    expect(apiMocks.createImageGeneration).toHaveBeenCalledTimes(2)
    expect(apiMocks.createImageGeneration).toHaveBeenNthCalledWith(1, expect.objectContaining({
      prompt: expect.stringContaining('清爽专业的公众号配图 1'),
      aspect_ratio: '16:9',
      tool_mode: 'generate',
      batch_index: 1,
      batch_total: 2
    }))
    expect(apiMocks.createImageGeneration.mock.calls[0][0].batch_id).toMatch(/^article-images-/)
    expect(wrapper.text()).toContain('这篇文章讲解品牌活动从预热到转化的完整方法。')
    expect(wrapper.get('[data-testid="article-images-task-301"]').text()).toContain('封面图')
  })

  it('uploads references and passes them to every batch generation task', async () => {
    const wrapper = await mountReady()

    const fileInput = wrapper.get('[data-testid="article-images-reference-input"]')
    Object.defineProperty(fileInput.element, 'files', {
      configurable: true,
      value: [new File(['fake'], 'brand.png', { type: 'image/png' })]
    })
    await fileInput.trigger('change')
    await flushPromises()

    await wrapper.get('[data-testid="article-images-title"]').setValue('品牌故事')
    await wrapper.get('[data-testid="article-images-body"]').setValue('这是一篇品牌故事文章，需要参考图保持人物与产品一致。')
    await wrapper.get('[data-testid="article-images-image-count"]').setValue('2')
    await wrapper.get('[data-testid="article-images-plan-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.uploadReferenceAsset).toHaveBeenCalledTimes(1)
    expect(apiMocks.planArticleImages).toHaveBeenCalledWith(expect.objectContaining({
      reference_asset_ids: [91]
    }))
    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith(expect.objectContaining({
      reference_asset_ids: [91],
      reference_weight: 80,
      reference_intent: 'compose'
    }))
  })

  it('retries a single card with the edited prompt instead of recreating the whole batch', async () => {
    const wrapper = await mountReady()

    await wrapper.get('[data-testid="article-images-title"]').setValue('活动增长方法论')
    await wrapper.get('[data-testid="article-images-body"]').setValue('第一段介绍活动背景。第二段说明三步流程。第三段总结复盘方法。')
    await wrapper.get('[data-testid="article-images-image-count"]').setValue('2')
    await wrapper.get('[data-testid="article-images-plan-form"]').trigger('submit.prevent')
    await flushPromises()
    expect(apiMocks.createImageGeneration).toHaveBeenCalledTimes(2)

    await wrapper.get('[data-testid="article-images-card-prompt-1"]').setValue('修改后的封面视觉 prompt，无文字，无水印')
    await wrapper.get('[data-testid="article-images-retry-1"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledTimes(3)
    expect(apiMocks.createImageGeneration).toHaveBeenLastCalledWith(expect.objectContaining({
      prompt: expect.stringContaining('修改后的封面视觉 prompt'),
      batch_index: 1,
      batch_total: 1
    }))
  })

  it('offers original and designed downloads for generated images', async () => {
    apiMocks.createImageGeneration.mockImplementation((payload) => Promise.resolve({
      generation_id: 300 + Number(payload.batch_index || 1),
      status: 'succeeded',
      stage: 'succeeded',
      preview_url: '/api/works/300/file',
      download_url: '/api/works/300/download',
      prompt: payload.prompt,
      parameters: payload
    }))
    const wrapper = await mountReady()

    await wrapper.get('[data-testid="article-images-title"]').setValue('活动增长方法论')
    await wrapper.get('[data-testid="article-images-body"]').setValue('第一段介绍活动背景。第二段说明三步流程。第三段总结复盘方法。')
    await wrapper.get('[data-testid="article-images-image-count"]').setValue('1')
    apiMocks.planArticleImages.mockResolvedValueOnce(planPayload(1))
    await wrapper.get('[data-testid="article-images-plan-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(wrapper.get('[data-testid="article-images-download-original-1"]').attributes('href')).toBe('/api/works/300/download')
    expect(wrapper.get('[data-testid="article-images-download-designed-1"]').text()).toContain('下载排版图')
  })
})
