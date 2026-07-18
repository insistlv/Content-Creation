import { flushPromises, mount } from '@vue/test-utils'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const routerPush = vi.hoisted(() => vi.fn())
const viewPath = resolve(process.cwd(), 'src/views/VirtualTryOnWorkspaceView.vue')
const readViewStyles = () => {
  const source = readFileSync(viewPath, 'utf8').replace(/\r\n/g, '\n')
  return source.match(/<style scoped>([\s\S]*?)<\/style>/)?.[1] ?? ''
}
const cssRuleFor = (styles, selector) => {
  const escapedSelector = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const matches = [...styles.matchAll(new RegExp(`(?:^|\\n)${escapedSelector}\\s*\\{([^}]*)\\}`, 'g'))]
  return matches.at(-1)?.[1] ?? ''
}
const apiMocks = vi.hoisted(() => ({
  getWorkspaceDiscovery: vi.fn(),
  listReferenceAssets: vi.fn(),
  uploadReferenceAsset: vi.fn(),
  deleteReferenceAsset: vi.fn(),
  estimateVirtualTryOn: vi.fn(),
  createVirtualTryOn: vi.fn(),
  getImageGeneration: vi.fn(),
  listWorks: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    getWorkspaceDiscovery: apiMocks.getWorkspaceDiscovery,
    listReferenceAssets: apiMocks.listReferenceAssets,
    uploadReferenceAsset: apiMocks.uploadReferenceAsset,
    deleteReferenceAsset: apiMocks.deleteReferenceAsset,
    estimateVirtualTryOn: apiMocks.estimateVirtualTryOn,
    createVirtualTryOn: apiMocks.createVirtualTryOn,
    getImageGeneration: apiMocks.getImageGeneration,
    listWorks: apiMocks.listWorks
  }
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: routerPush
  })
}))

import ImageUploadZone from '../components/ImageUploadZone.vue'
import VirtualTryOnWorkspaceView from '../views/VirtualTryOnWorkspaceView.vue'

function mountWorkspace() {
  return mount(VirtualTryOnWorkspaceView)
}

function fileNamed(name) {
  return new File(['fake'], name, { type: 'image/png' })
}

async function fillMinimumGenerationForm(wrapper) {
  wrapper.findAllComponents(ImageUploadZone)[1].vm.$emit('upload', fileNamed('garment.png'))
  await flushPromises()
  await wrapper.get('[data-testid="tryon-height"]').setValue(170)
  await wrapper.get('[data-testid="tryon-weight"]').setValue(58)
  await wrapper.vm.$nextTick()
}

describe('VirtualTryOnWorkspaceView', () => {
  beforeEach(() => {
    Object.values(apiMocks).forEach((mock) => mock.mockReset())
    routerPush.mockReset()
    apiMocks.getWorkspaceDiscovery.mockResolvedValue({
      models: [
        { id: 7, name: '白霖通用模型', default_credits_cost: 1 },
        { id: 9, name: '高清人像模型', default_credits_cost: 2 }
      ]
    })
    apiMocks.listReferenceAssets.mockResolvedValue({ items: [] })
    apiMocks.listWorks.mockResolvedValue({ items: [] })
    apiMocks.uploadReferenceAsset.mockImplementation((file) => Promise.resolve({
      id: file.name.includes('body') ? 102 : 101,
      original_filename: file.name,
      preview_url: `/api/reference-assets/${file.name}/file`
    }))
    apiMocks.estimateVirtualTryOn.mockResolvedValue({
      required_credits: 1,
      available_credits: 8,
      missing_credits: 0,
      enough: true
    })
    apiMocks.createVirtualTryOn.mockResolvedValue({
      generation_id: 88,
      status: 'queued',
      credits_cost: 1,
      available_credits: 7
    })
    apiMocks.getImageGeneration.mockResolvedValue({
      generation_id: 88,
      status: 'succeeded',
      work_id: 66,
      preview_url: '/api/works/66/file',
      download_url: '/api/works/66/download',
      mime_type: 'image/png'
    })
  })

  it('renders four form sections, scene choices, and the privacy notice', async () => {
    const wrapper = mountWorkspace()
    await flushPromises()

    expect(wrapper.get('[data-testid="virtual-try-on-workspace"]').text()).toContain('建模试衣')
    expect(wrapper.get('[data-testid="virtual-try-on-body-section"]').text()).toContain('身型')
    expect(wrapper.get('[data-testid="virtual-try-on-garment-section"]').text()).toContain('服装')
    expect(wrapper.get('[data-testid="virtual-try-on-scene-section"]').text()).toContain('职场商务')
    expect(wrapper.get('[data-testid="virtual-try-on-generate-section"]').text()).toContain('生成')
    expect(wrapper.text()).toContain('真人参考图和身体围度默认只用于本次生成')
    expect(wrapper.find('[data-testid="virtual-try-on-submit"]').attributes('disabled')).toBeDefined()
  })

  it('renders localized Chinese placeholder examples for body, garment, and scene inputs', async () => {
    const wrapper = mountWorkspace()
    await flushPromises()

    const expectedPlaceholders = {
      'tryon-body-type': '如：标准、偏瘦、微胖、梨形',
      'tryon-fit-preference': '如：合身、宽松、显瘦',
      'tryon-garment-category': '如：衬衫、连衣裙、外套',
      'tryon-garment-size': '如：中码、均码、身高165适用',
      'tryon-garment-material': '如：棉、羊毛、牛仔、雪纺',
      'tryon-garment-color': '如：白色、黑色、米色',
      'tryon-garment-fit': '如：常规、宽松、修身',
      'tryon-garment-details': '如：领型、袖长、口袋、纹理等',
      'tryon-scene-pose': '如：自然站立、侧身、走路',
      'tryon-scene-background': '如：明亮办公空间'
    }

    Object.entries(expectedPlaceholders).forEach(([testId, placeholder]) => {
      expect(wrapper.get(`[data-testid="${testId}"]`).attributes('placeholder')).toBe(placeholder)
    })
    ;[
      'standard / slim / curvy',
      'regular / loose / fitted',
      'shirt / dress / coat',
      'M',
      'cotton',
      'white',
      'regular',
      'standing',
      '领型、袖长、口袋、纹理等',
      '明亮办公空间'
    ].forEach((placeholder) => {
      expect(wrapper.html()).not.toContain(`placeholder="${placeholder}"`)
    })
  })

  it('defines workspace-shell theme tokens for the virtual try-on surface', () => {
    const styles = readViewStyles()

    expect(styles).toContain(':global(.workspace-with-sidebar.user-light-shell .virtual-try-on-workspace)')
    expect(styles).toContain(':global(.workspace-with-sidebar.user-dark-shell .virtual-try-on-workspace)')
    ;[
      '--tryon-page-text',
      '--tryon-page-muted',
      '--tryon-panel-bg',
      '--tryon-panel-border',
      '--tryon-input-bg',
      '--tryon-upload-bg',
      '--tryon-accent',
      '--tryon-accent-soft',
      '--tryon-danger'
    ].forEach((token) => {
      expect(styles).toContain(token)
    })

    expect(styles).not.toContain('--surface-primary')
    expect(styles).not.toContain('--text-primary')
    expect(styles).not.toContain('--border-color')
    expect(cssRuleFor(styles, '.virtual-try-on-workspace')).toContain('color: var(--tryon-page-text)')
    expect(cssRuleFor(styles, '.preview-panel')).not.toContain('position: sticky')
    expect(cssRuleFor(styles, '.preview-panel')).not.toContain('top: 20px')
    expect(cssRuleFor(styles, '.tryon-section')).toContain('background: var(--tryon-panel-bg)')
    expect(cssRuleFor(styles, '.tryon-grid input')).toContain('background: var(--tryon-input-bg)')
    expect(cssRuleFor(styles, '.scene-tabs button.active')).toContain('background: var(--tryon-accent-soft)')
    expect(cssRuleFor(styles, '.preview-placeholder')).toContain('border: 1px dashed var(--tryon-panel-border)')
    expect(cssRuleFor(styles, '.history-item')).toContain('background: var(--tryon-panel-bg)')
    expect(cssRuleFor(styles, ':deep(.image-upload-zone)')).toContain('background: var(--tryon-upload-bg)')
    expect(cssRuleFor(styles, ':deep(.upload-title)')).toContain('color: var(--tryon-page-text)')
    expect(cssRuleFor(styles, ':deep(.remove-button)')).toContain('background: var(--tryon-danger)')
  })

  it('starts with only current-page history and does not preload works', async () => {
    apiMocks.listWorks.mockResolvedValueOnce({
      items: [
        {
          work_id: 11,
          preview_url: '/api/works/11/file',
          prompt: 'external virtual try-on work'
        }
      ]
    })

    const wrapper = mountWorkspace()
    await flushPromises()

    expect(apiMocks.listWorks).not.toHaveBeenCalled()
    expect(wrapper.findAll('.history-item')).toHaveLength(0)
    expect(wrapper.text()).toContain('本次暂无生成结果')
    expect(wrapper.text()).not.toContain('external virtual try-on work')
  })

  it('keeps generated results in current-page history and can restore one to preview', async () => {
    apiMocks.getImageGeneration
      .mockResolvedValueOnce({
        generation_id: 88,
        status: 'succeeded',
        work_id: 66,
        preview_url: '/api/works/66/file',
        download_url: '/api/works/66/download',
        prompt: 'first current-page result',
        mime_type: 'image/png'
      })
      .mockResolvedValueOnce({
        generation_id: 88,
        status: 'succeeded',
        work_id: 77,
        preview_url: '/api/works/77/file',
        download_url: '/api/works/77/download',
        prompt: 'second current-page result',
        mime_type: 'image/png'
      })

    const wrapper = mountWorkspace()
    await flushPromises()
    await fillMinimumGenerationForm(wrapper)

    await wrapper.get('[data-testid="virtual-try-on-submit"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="virtual-try-on-submit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.listWorks).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="virtual-try-on-result"]').find('img').attributes('src')).toBe('/api/works/77/file')

    const historyItems = wrapper.findAll('.history-item')
    expect(historyItems).toHaveLength(2)
    expect(historyItems[0].text()).toContain('second current-page result')
    expect(historyItems[1].text()).toContain('first current-page result')

    await historyItems[1].trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="virtual-try-on-result"]').find('img').attributes('src')).toBe('/api/works/66/file')
    expect(wrapper.get('[data-testid="virtual-try-on-download"]').attributes('href')).toBe('/api/works/66/download')
  })

  it('uploads garment and body references, estimates credits, creates a generation, and shows the result', async () => {
    const wrapper = mountWorkspace()
    await flushPromises()

    const uploadZones = wrapper.findAllComponents(ImageUploadZone)
    uploadZones[0].vm.$emit('upload', fileNamed('body.png'))
    uploadZones[1].vm.$emit('upload', fileNamed('garment.png'))
    await flushPromises()

    await wrapper.get('[data-testid="tryon-height"]').setValue(172)
    await wrapper.get('[data-testid="tryon-weight"]').setValue(62)
    await wrapper.get('[data-testid="tryon-shoulder"]').setValue(40)
    await wrapper.get('[data-testid="tryon-chest"]').setValue(86)
    await wrapper.get('[data-testid="tryon-waist"]').setValue(68)
    await wrapper.get('[data-testid="tryon-hip"]').setValue(92)
    await wrapper.get('[data-testid="tryon-body-type"]').setValue('standard')
    await wrapper.get('[data-testid="tryon-fit-preference"]').setValue('regular')
    await wrapper.get('[data-testid="tryon-garment-category"]').setValue('shirt')
    await wrapper.get('[data-testid="tryon-garment-size"]').setValue('M')
    await wrapper.get('[data-testid="tryon-garment-material"]').setValue('cotton')
    await wrapper.get('[data-testid="tryon-garment-color"]').setValue('white')
    await wrapper.get('[data-testid="tryon-garment-fit"]').setValue('regular')
    await wrapper.get('[data-testid="tryon-garment-details"]').setValue('立领、长袖')
    await wrapper.get('[data-testid="tryon-scene-pose"]').setValue('standing')
    await wrapper.get('[data-testid="tryon-scene-background"]').setValue('明亮办公空间')
    await wrapper.get('[data-testid="tryon-quality"]').setValue('high')
    await wrapper.get('[data-testid="tryon-aspect-ratio"]').setValue('3:4')
    await wrapper.vm.$nextTick()

    expect(wrapper.find('[data-testid="virtual-try-on-submit"]').attributes('disabled')).toBeUndefined()

    await wrapper.get('[data-testid="virtual-try-on-estimate"]').trigger('click')
    await flushPromises()

    expect(apiMocks.estimateVirtualTryOn).toHaveBeenCalledWith(expect.objectContaining({
      body_profile: expect.objectContaining({
        height_cm: 172,
        weight_kg: 62,
        body_reference_asset_id: 102
      }),
      garment: expect.objectContaining({
        garment_reference_asset_id: 101,
        category: 'shirt',
        details: '立领、长袖'
      }),
      scene: expect.objectContaining({
        category: 'work_business',
        sub_scene: 'office'
      }),
      generation: expect.objectContaining({
        model_id: 7,
        quality: 'high',
        aspect_ratio: '3:4'
      })
    }))
    expect(wrapper.get('[data-testid="virtual-try-on-credit-estimate"]').text()).toContain('预计 1 点')

    await wrapper.get('[data-testid="virtual-try-on-submit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createVirtualTryOn).toHaveBeenCalledWith(apiMocks.estimateVirtualTryOn.mock.calls[0][0])
    expect(apiMocks.getImageGeneration).toHaveBeenCalledWith(88)
    expect(wrapper.get('[data-testid="virtual-try-on-result"]').find('img').attributes('src')).toBe('/api/works/66/file')
    expect(wrapper.get('[data-testid="virtual-try-on-download"]').attributes('href')).toBe('/api/works/66/download')
  })

  it('keeps polling running virtual try-on tasks and renders the final image automatically', async () => {
    vi.useFakeTimers()
    apiMocks.getImageGeneration
      .mockResolvedValueOnce({
        generation_id: 88,
        status: 'running',
        stage: 'requesting_provider'
      })
      .mockResolvedValueOnce({
        generation_id: 88,
        status: 'succeeded',
        work_id: 66,
        preview_url: '/api/works/66/file',
        download_url: '/api/works/66/download',
        mime_type: 'image/png'
      })

    const wrapper = mountWorkspace()
    await flushPromises()

    wrapper.findAllComponents(ImageUploadZone)[1].vm.$emit('upload', fileNamed('garment.png'))
    await flushPromises()
    await wrapper.get('[data-testid="tryon-height"]').setValue(170)
    await wrapper.get('[data-testid="tryon-weight"]').setValue(58)
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="virtual-try-on-empty"]').text()).toContain('暂无结果，请完善左侧信息并点击生成')

    await wrapper.get('[data-testid="virtual-try-on-submit"]').trigger('click')
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(apiMocks.getImageGeneration).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[data-testid="virtual-try-on-progress"]').text()).toContain('正在生成')
    expect(wrapper.find('[data-testid="virtual-try-on-result"]').exists()).toBe(false)

    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(apiMocks.getImageGeneration).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[data-testid="virtual-try-on-result"]').find('img').attributes('src')).toBe('/api/works/66/file')
    expect(wrapper.get('[data-testid="virtual-try-on-download"]').attributes('href')).toBe('/api/works/66/download')
    expect(wrapper.get('[data-testid="virtual-try-on-save"]').text()).toContain('保存')
    expect(wrapper.find('[data-testid="virtual-try-on-progress"]').exists()).toBe(false)

    vi.useRealTimers()
  })

  it('validates all out-of-range body measurements before estimating', async () => {
    const wrapper = mountWorkspace()
    await flushPromises()

    wrapper.findAllComponents(ImageUploadZone)[1].vm.$emit('upload', fileNamed('garment.png'))
    await flushPromises()
    await wrapper.get('[data-testid="tryon-height"]').setValue(30)
    await wrapper.get('[data-testid="tryon-weight"]').setValue(300)
    await wrapper.get('[data-testid="tryon-shoulder"]').setValue(10)
    await wrapper.get('[data-testid="tryon-chest"]').setValue(200)
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="virtual-try-on-estimate"]').trigger('click')
    await flushPromises()

    expect(apiMocks.estimateVirtualTryOn).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="tryon-height-error"]').text()).toContain('当前 30 cm，可用范围 80-230 cm')
    expect(wrapper.get('[data-testid="tryon-weight-error"]').text()).toContain('当前 300 kg，可用范围 25-250 kg')
    expect(wrapper.get('[data-testid="tryon-shoulder-error"]').text()).toContain('当前 10 cm，可用范围 20-80 cm')
    expect(wrapper.get('[data-testid="tryon-chest-error"]').text()).toContain('当前 200 cm，可用范围 40-180 cm')
    expect(wrapper.get('[data-testid="tryon-height"]').classes()).toContain('invalid')
    expect(wrapper.text()).toContain('身高超出范围')
    expect(wrapper.text()).toContain('胸围超出范围')
  })

  it('renders structured backend body validation errors on fields', async () => {
    apiMocks.uploadReferenceAsset.mockResolvedValueOnce({
      id: 101,
      original_filename: 'garment.png',
      preview_url: '/api/reference-assets/101/file'
    })
    apiMocks.estimateVirtualTryOn.mockRejectedValueOnce(Object.assign(new Error('身型参数填写有误，请按提示修改'), {
      code: 'invalid_body_profile',
      validation_errors: [
        {
          field: 'height_cm',
          label: '身高',
          value: 30,
          min: 80,
          max: 230,
          unit: 'cm',
          required: true
        },
        {
          field: 'waist_cm',
          label: '腰围',
          value: 20,
          min: 40,
          max: 180,
          unit: 'cm',
          required: false
        }
      ]
    }))
    const wrapper = mountWorkspace()
    await flushPromises()

    wrapper.findAllComponents(ImageUploadZone)[1].vm.$emit('upload', fileNamed('garment.png'))
    await flushPromises()
    await wrapper.get('[data-testid="tryon-height"]').setValue(170)
    await wrapper.get('[data-testid="tryon-weight"]').setValue(58)
    await wrapper.get('[data-testid="tryon-waist"]').setValue(60)
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="virtual-try-on-estimate"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="tryon-height-error"]').text()).toContain('身高超出范围：当前 30 cm，可用范围 80-230 cm')
    expect(wrapper.get('[data-testid="tryon-waist-error"]').text()).toContain('腰围超出范围：当前 20 cm，可用范围 40-180 cm')
    expect(wrapper.get('[data-testid="tryon-height"]').classes()).toContain('invalid')
    expect(wrapper.get('[data-testid="tryon-waist"]').classes()).toContain('invalid')
  })

  it('clears body field errors after correcting invalid values', async () => {
    const wrapper = mountWorkspace()
    await flushPromises()

    wrapper.findAllComponents(ImageUploadZone)[1].vm.$emit('upload', fileNamed('garment.png'))
    await flushPromises()
    await wrapper.get('[data-testid="tryon-height"]').setValue(30)
    await wrapper.get('[data-testid="tryon-weight"]').setValue(58)
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="virtual-try-on-submit"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="tryon-height-error"]').exists()).toBe(true)
    expect(apiMocks.createVirtualTryOn).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="tryon-height"]').setValue(170)
    await wrapper.vm.$nextTick()

    expect(wrapper.find('[data-testid="tryon-height-error"]').exists()).toBe(false)
    await wrapper.get('[data-testid="virtual-try-on-submit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createVirtualTryOn).toHaveBeenCalled()
  })

  it('shows credit shortfall and does not create when estimate says credits are insufficient', async () => {
    apiMocks.uploadReferenceAsset.mockResolvedValueOnce({
      id: 101,
      original_filename: 'garment.png',
      preview_url: '/api/reference-assets/101/file'
    })
    apiMocks.estimateVirtualTryOn.mockResolvedValueOnce({
      required_credits: 1,
      available_credits: 0,
      missing_credits: 1,
      enough: false
    })
    const wrapper = mountWorkspace()
    await flushPromises()

    wrapper.findAllComponents(ImageUploadZone)[1].vm.$emit('upload', fileNamed('garment.png'))
    await flushPromises()
    await wrapper.get('[data-testid="tryon-height"]').setValue(170)
    await wrapper.get('[data-testid="tryon-weight"]').setValue(58)
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="virtual-try-on-submit"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createVirtualTryOn).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('点数不足')
    expect(wrapper.text()).toContain('还差 1 点')
  })

  it('can continue creation from the generated result as a workspace reference', async () => {
    const wrapper = mountWorkspace()
    await flushPromises()

    wrapper.findAllComponents(ImageUploadZone)[1].vm.$emit('upload', fileNamed('garment.png'))
    await flushPromises()
    await wrapper.get('[data-testid="tryon-height"]').setValue(170)
    await wrapper.get('[data-testid="tryon-weight"]').setValue(58)
    await wrapper.get('[data-testid="virtual-try-on-submit"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="virtual-try-on-use-reference"]').trigger('click')

    expect(routerPush).toHaveBeenCalledWith({
      path: '/workspace',
      query: {
        reference_work_id: 66
      }
    })
  })
})
