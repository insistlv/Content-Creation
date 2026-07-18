import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const stylesPath = resolve(process.cwd(), 'src/styles.css')
const readStyles = () => readFileSync(stylesPath, 'utf8').replace(/\r\n/g, '\n')

const routerPush = vi.hoisted(() => vi.fn())
const TestApiError = vi.hoisted(() => class ApiError extends Error {
  constructor(code, message, status, details = {}) {
    super(message)
    this.code = code
    this.status = status
    Object.assign(this, details)
  }
})
const apiMocks = vi.hoisted(() => ({
  getMe: vi.fn(),
  listWorks: vi.fn(),
  createImageGeneration: vi.fn(),
  estimateImageGeneration: vi.fn(),
  getImageGeneration: vi.fn(),
  cancelImageGeneration: vi.fn(),
  updateWork: vi.fn(),
  getWorkspaceDiscovery: vi.fn(),
  useInspirationRecommendation: vi.fn(),
  optimizePrompt: vi.fn(),
  planImageAgent: vi.fn(),
  listReferenceAssets: vi.fn(),
  uploadReferenceAsset: vi.fn(),
  deleteReferenceAsset: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  ApiError: TestApiError,
  api: {
    getMe: apiMocks.getMe,
    listWorks: apiMocks.listWorks,
    createImageGeneration: apiMocks.createImageGeneration,
    estimateImageGeneration: apiMocks.estimateImageGeneration,
    getImageGeneration: apiMocks.getImageGeneration,
    cancelImageGeneration: apiMocks.cancelImageGeneration,
    updateWork: apiMocks.updateWork,
    getWorkspaceDiscovery: apiMocks.getWorkspaceDiscovery,
    useInspirationRecommendation: apiMocks.useInspirationRecommendation,
    optimizePrompt: apiMocks.optimizePrompt,
    planImageAgent: apiMocks.planImageAgent,
    listReferenceAssets: apiMocks.listReferenceAssets,
    uploadReferenceAsset: apiMocks.uploadReferenceAsset,
    deleteReferenceAsset: apiMocks.deleteReferenceAsset
  }
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({
    fullPath: '/workspace',
    query: {}
  }),
  useRouter: () => ({
    push: routerPush
  })
}))

import WorkspaceView from '../views/WorkspaceView.vue'
import WorkspaceComposerPanel from '../components/workspace/WorkspaceComposerPanel.vue'
import { chooseClickSelect, clickSelectMenu, openClickSelect } from './click-select-test-utils.js'
import { ApiError } from '../api/client.js'
import { authModalState, closeAuthModal } from '../stores/auth-modal.js'
import { clearCurrentUser, setCurrentUser } from '../stores/session.js'

describe('WorkspaceView', () => {
  const referenceAssetUploadMaxBytes = 50 * 1024 * 1024
  const activeGenerationStorageKey = 'image-agent.workspace.active-generation'
  const mountedWrappers = []
  const mountOptions = {
    global: {
      stubs: {
        RouterLink: {
          props: ['to'],
          template: '<a :href="typeof to === \'string\' ? to : to.path"><slot /></a>'
        }
      }
    }
  }

  function mockUser(overrides = {}) {
    apiMocks.getMe.mockResolvedValueOnce({
      user_id: 9,
      username: 'creator_09',
      display_name: '栏目主理人',
      available_credits: 3,
      ...overrides
    })
  }

  function mockWorks(items = [], overrides = {}) {
    apiMocks.listWorks.mockResolvedValueOnce({ items, ...overrides })
  }

  function makeWork(id, overrides = {}) {
    return {
      work_id: id,
      prompt: `作品 ${id}`,
      preview_url: `/api/works/${id}/file`,
      download_url: `/api/works/${id}/download`,
      aspect_ratio: '1:1',
      category: 'image',
      created_at: `2026-06-01T10:${String(id % 60).padStart(2, '0')}:00Z`,
      ...overrides
    }
  }

  function mockReferenceAssets(items = []) {
    apiMocks.listReferenceAssets.mockResolvedValueOnce({ items })
  }

  function mockDiscovery(payload = {}) {
    apiMocks.getWorkspaceDiscovery.mockResolvedValueOnce({
      tools: [
        {
          mode: 'expand',
          title: '智能扩图',
          description: '按方向延展画面边界',
          icon: 'maximize',
          enabled: true,
          requires_source: true,
          source_limit: 1,
          form_schema: [
            { key: 'top', label: '上', type: 'number', default: 20, min: 0, max: 100, step: 5 },
            { key: 'bottom', label: '下', type: 'number', default: 20, min: 0, max: 100, step: 5 },
            { key: 'left', label: '左', type: 'number', default: 20, min: 0, max: 100, step: 5 },
            { key: 'right', label: '右', type: 'number', default: 20, min: 0, max: 100, step: 5 }
          ]
        },
        {
          mode: 'upscale',
          title: '高清放大',
          description: '选择倍率增强细节',
          icon: 'sparkles',
          enabled: true,
          requires_source: true,
          source_limit: 1,
          form_schema: [
            { key: 'scale', label: '倍率', type: 'select', default: '2x', options: ['2x', '4x', '8x'] },
            { key: 'edit_instruction', label: '增强说明（可选）', type: 'textarea' }
          ]
        },
        {
          mode: 'precision_edit',
          title: '精细编辑',
          description: '圈选局部并输入编辑指令',
          icon: 'edit',
          enabled: true,
          requires_source: true,
          source_limit: 1,
          form_schema: [
            { key: 'edit_instruction', label: '编辑指令', type: 'textarea' },
            { key: 'mask', label: '蒙版', type: 'mask' }
          ]
        }
      ],
      models: [
        {
          id: 7,
          name: '白霖通用模型',
          default_credits_cost: 2,
          capability_tags: ['image', 'reference']
        }
      ],
      hot: [
        {
          id: 101,
          title: '拍立得风格',
          description: '将照片转换为拍立得风格',
          prompt: '拍立得风格人像照片，白色相纸边框，自然闪光灯',
          preview_url: 'https://oss.example.com/polaroid.png',
          aspect_ratio: '1:1',
          style_preset: '写实',
          tool_mode: 'generate'
        }
      ],
      inspiration: [
        {
          id: 201,
          title: '高级氛围感香薰蜡烛',
          description: '暖光、静物与商业摄影质感',
          prompt: '高级氛围感香薰蜡烛，暖色棚拍，商业摄影',
          preview_url: 'https://oss.example.com/candle.png',
          aspect_ratio: '4:3',
          style_preset: '电商',
          tool_mode: 'generate'
        }
      ],
      recommendations: [
        {
          id: 501,
          slug: 'weekly-cyber-city',
          title: 'Cyberpunk City',
          category: 'concept',
          description: 'Neon skyline sample',
          heat_tags: ['weekly-hot', 'beginner'],
          prompt: 'cyberpunk city at rainy night',
          negative_prompt: 'low quality',
          preview_url: 'https://oss.example.com/recommendations/cyber-city.png',
          aspect_ratio: '16:9',
          style_preset: 'cinematic',
          tool_mode: 'generate',
          model_id: 7,
          params: { seed: 918, guidance: 7 },
          sort_order: 1,
          use_count: 12
        }
      ],
      ...payload
    })
  }

  function mountComposerPanel(props = {}) {
    return mount(WorkspaceComposerPanel, {
      props: {
        displayedModelName: '白霖通用模型',
        workspaceModels: [{ id: 7, name: '白霖通用模型' }],
        selectedReferenceImages: [],
        sourceImageLimit: 4,
        referenceUploadTitle: '上传图片',
        referenceUploadHint: 'JPG/PNG/WEBP，单张小于50MB',
        promptLabel: '提示词',
        promptPlaceholder: '描述你的想法',
        stylePresets: ['写实', '国风'],
        qualityOptions: [
          { key: 'low', label: '0.5K' },
          { key: 'medium', label: '1K' },
          { key: 'high', label: '2K' },
          { key: 'ultra', label: '4K' }
        ],
        canSubmit: true,
        currentEstimatedCredits: 1,
        ...props
      },
      global: mountOptions.global
    })
  }

  async function mountReady(options = mountOptions) {
    if (apiMocks.listReferenceAssets.mock.calls.length === 0) {
      mockReferenceAssets()
    }
    const wrapper = mount(WorkspaceView, options)
    mountedWrappers.push(wrapper)
    await flushPromises()
    await wrapper.vm.$nextTick()
    return wrapper
  }

  function deferred() {
    let resolve
    let reject
    const promise = new Promise((promiseResolve, promiseReject) => {
      resolve = promiseResolve
      reject = promiseReject
    })
    return { promise, resolve, reject }
  }

  async function advanceCreditEstimateDebounce(ms = 600) {
    await vi.advanceTimersByTimeAsync(ms)
    await flushPromises()
  }

  function file(name, type = 'image/png') {
    return new File(['fake'], name, { type })
  }

  function oversizedImageFile() {
    const uploadFile = new File(['fake'], 'too-large.png', { type: 'image/png' })
    Object.defineProperty(uploadFile, 'size', {
      value: referenceAssetUploadMaxBytes + 1,
      configurable: true
    })
    return uploadFile
  }

  function setInputFiles(input, files) {
    Object.defineProperty(input.element, 'files', {
      value: files,
      configurable: true
    })
  }

  async function uploadWorkspaceReference(wrapper, uploadFile) {
    const input = wrapper.get('[data-testid="workspace-reference-file-input"]')
    setInputFiles(input, [uploadFile])
    await input.trigger('change')
  }

  async function dropWorkspaceReference(wrapper, files) {
    await wrapper.get('[data-testid="workspace-reference-dropzone"]').trigger('drop', {
      dataTransfer: { files }
    })
  }

  async function submitWorkspacePrompt(wrapper, promptText) {
    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue(promptText)
    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()
    await wrapper.vm.$nextTick()
  }

  async function openHomeAdvancedPanel(wrapper) {
    const toggle = wrapper.get('[data-testid="workspace-home-advanced-toggle"]')
    if (toggle.attributes('aria-expanded') !== 'true') {
      await toggle.trigger('click')
      await wrapper.vm.$nextTick()
    }

    const panel = document.querySelector('[data-testid="workspace-home-advanced-panel"]')
    expect(panel).not.toBeNull()
    return panel
  }

  function homeAdvancedStyleChip(panel, label) {
    return [...panel.querySelectorAll('.style-chip')].find((node) => node.textContent === label)
  }

  async function setHomeAdvancedInput(wrapper, panel, testid, value) {
    const input = panel.querySelector(`[data-testid="${testid}"]`)
    expect(input).not.toBeNull()
    input.value = value
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await wrapper.vm.$nextTick()
  }

  function mockCanvasDrawing() {
    const canvasContext = {
      clearRect: vi.fn(),
      fillRect: vi.fn(),
      beginPath: vi.fn(),
      moveTo: vi.fn(),
      lineTo: vi.fn(),
      stroke: vi.fn(),
      closePath: vi.fn(),
      fill: vi.fn(),
      lineCap: '',
      lineJoin: '',
      lineWidth: 0,
      strokeStyle: '',
      fillStyle: ''
    }
    const getContextSpy = vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(canvasContext)
    const toBlobSpy = vi.spyOn(HTMLCanvasElement.prototype, 'toBlob').mockImplementation((callback) => {
      callback(new Blob(['mask'], { type: 'image/png' }))
    })
    return {
      canvasContext,
      restore() {
        getContextSpy.mockRestore()
        toBlobSpy.mockRestore()
      }
    }
  }

  async function drawMaskStroke(canvas, points) {
    canvas.element.getBoundingClientRect = () => ({
      left: 0,
      top: 0,
      right: 100,
      bottom: 100,
      width: 100,
      height: 100
    })
    const [first, ...rest] = points
    await canvas.trigger('pointerdown', { clientX: first[0], clientY: first[1] })
    for (const point of rest) {
      await canvas.trigger('pointermove', { clientX: point[0], clientY: point[1] })
    }
    const last = rest.at(-1) || first
    await canvas.trigger('pointerup', { clientX: last[0], clientY: last[1] })
  }

  beforeEach(() => {
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    apiMocks.getWorkspaceDiscovery.mockResolvedValue({
      tools: [
        {
          mode: 'expand',
          title: '智能扩图',
          description: '按方向延展画面边界',
          icon: 'maximize',
          enabled: true,
          requires_source: true,
          source_limit: 1,
          form_schema: [
            { key: 'top', label: '上', type: 'number', default: 20, min: 0, max: 100, step: 5 },
            { key: 'bottom', label: '下', type: 'number', default: 20, min: 0, max: 100, step: 5 },
            { key: 'left', label: '左', type: 'number', default: 20, min: 0, max: 100, step: 5 },
            { key: 'right', label: '右', type: 'number', default: 20, min: 0, max: 100, step: 5 }
          ]
        },
        {
          mode: 'upscale',
          title: '高清放大',
          description: '选择倍率增强细节',
          icon: 'sparkles',
          enabled: true,
          requires_source: true,
          source_limit: 1,
          form_schema: [
            { key: 'scale', label: '倍率', type: 'select', default: '2x', options: ['2x', '4x', '8x'] },
            { key: 'edit_instruction', label: '增强说明（可选）', type: 'textarea' }
          ]
        },
        {
          mode: 'precision_edit',
          title: '精细编辑',
          description: '圈选局部并输入编辑指令',
          icon: 'edit',
          enabled: true,
          requires_source: true,
          source_limit: 1,
          form_schema: [
            { key: 'edit_instruction', label: '编辑指令', type: 'textarea' },
            { key: 'mask', label: '蒙版', type: 'mask' }
          ]
        }
      ],
      models: [
        {
          id: 7,
          name: '白霖通用模型',
          default_credits_cost: 2,
          capability_tags: ['image', 'reference']
        }
      ],
      hot: [
        {
          id: 101,
          title: '拍立得风格',
          description: '将照片转换为拍立得风格',
          prompt: '拍立得风格人像照片，白色相纸边框，自然闪光灯',
          preview_url: 'https://oss.example.com/polaroid.png',
          aspect_ratio: '1:1',
          style_preset: '写实',
          tool_mode: 'generate'
        }
      ],
      inspiration: [
        {
          id: 201,
          title: '高级氛围感香薰蜡烛',
          description: '暖光、静物与商业摄影质感',
          prompt: '高级氛围感香薰蜡烛，暖色棚拍，商业摄影',
          preview_url: 'https://oss.example.com/candle.png',
          aspect_ratio: '4:3',
          style_preset: '电商',
          tool_mode: 'generate'
        }
      ],
      recommendations: [
        {
          id: 501,
          slug: 'weekly-cyber-city',
          title: 'Cyberpunk City',
          category: 'concept',
          description: 'Neon skyline sample',
          heat_tags: ['weekly-hot', 'beginner'],
          prompt: 'cyberpunk city at rainy night',
          negative_prompt: 'low quality',
          preview_url: 'https://oss.example.com/recommendations/cyber-city.png',
          aspect_ratio: '16:9',
          style_preset: 'cinematic',
          tool_mode: 'generate',
          model_id: 7,
          params: { seed: 918, guidance: 7 },
          sort_order: 1,
          use_count: 12
        }
      ]
    })
    apiMocks.estimateImageGeneration.mockResolvedValue({
      required_credits: 2,
      available_credits: 3,
      missing_credits: 0,
      enough: true
    })
    apiMocks.useInspirationRecommendation.mockResolvedValue({ id: 501, use_count: 13 })
  })

  afterEach(() => {
    mountedWrappers.splice(0).forEach((wrapper) => {
      wrapper.unmount()
    })
    window.sessionStorage.clear()
    window.localStorage.clear()
    vi.useRealTimers()
    vi.restoreAllMocks()
    vi.resetAllMocks()
    clearCurrentUser()
    closeAuthModal()
    document.body.innerHTML = ''
  })

  it('lets guests browse public workspace discovery without loading personal workspace data', async () => {
    apiMocks.getMe.mockRejectedValueOnce(Object.assign(new Error('unauthorized'), { status: 401 }))
    mockDiscovery({
      hot: [
        {
          id: 301,
          title: '游客可见后台优秀案例',
          description: '后台配置的公开案例',
          prompt: '公开案例提示词',
          preview_url: 'https://oss.example.com/public-case.png',
          aspect_ratio: '4:3',
          style_preset: '海报',
          tool_mode: 'generate'
        }
      ],
      inspiration: []
    })

    const wrapper = await mountReady()

    expect(wrapper.find('.workspace-error').exists()).toBe(false)
    expect(wrapper.get('[data-testid="workspace-composer-form"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="workspace-discovery-panel"]').text()).toContain('AI 工具')
    expect(wrapper.get('[data-testid="workspace-tool-expand"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="workspace-discovery-panel"]').text()).toContain('优秀案例')
    expect(wrapper.get('[data-testid="workspace-discovery-panel"]').text()).toContain('游客可见后台优秀案例')
    expect(wrapper.get('[data-testid="workspace-template-301"]').exists()).toBe(true)
    expect(apiMocks.listWorks).not.toHaveBeenCalled()
    expect(apiMocks.listReferenceAssets).not.toHaveBeenCalled()
    expect(apiMocks.getWorkspaceDiscovery).toHaveBeenCalledTimes(1)

    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('访客先写一个提示词')
    await flushPromises()
    expect(apiMocks.estimateImageGeneration).not.toHaveBeenCalled()
  })

  it('opens the auth modal before guests create upload optimize or open playground tools', async () => {
    apiMocks.getMe.mockRejectedValue(Object.assign(new Error('unauthorized'), { status: 401 }))

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('访客提示词')
    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()
    expect(window.confirm).not.toHaveBeenCalled()
    expect(routerPush).not.toHaveBeenCalledWith({ path: '/login', query: { redirect: '/workspace' } })
    expect(authModalState.open).toBe(true)
    expect(authModalState.message).toBe('需要登录才能使用该功能')
    expect(apiMocks.createImageGeneration).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="workspace-open-prompt-optimizer"]').trigger('click')
    expect(authModalState.open).toBe(true)
    expect(apiMocks.optimizePrompt).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="workspace-reference-add"]').trigger('click')
    expect(authModalState.open).toBe(true)
    expect(apiMocks.uploadReferenceAsset).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="workspace-playground-couple-album"]').trigger('click')
    expect(authModalState.open).toBe(true)
  })

  it('uses shared session updates after modal login without reloading the workspace', async () => {
    apiMocks.getMe.mockRejectedValueOnce(Object.assign(new Error('unauthorized'), { status: 401 }))
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 880,
      status: 'queued',
      prompt: 'visitor prompt after login',
      parameters: {},
      available_credits: 4
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('visitor prompt after login')
    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(authModalState.open).toBe(true)
    expect(apiMocks.createImageGeneration).not.toHaveBeenCalled()

    closeAuthModal()
    setCurrentUser({
      user_id: 9,
      username: 'creator_09',
      display_name: 'Creator',
      available_credits: 5
    })
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(authModalState.open).toBe(false)
    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: 'visitor prompt after login',
      tool_mode: 'generate'
    }))
  })

  it('keeps the welcome hero for a fresh user without works', async () => {
    mockUser()
    mockWorks()
    mockDiscovery()
    const wrapper = await mountReady()

    expect(wrapper.get('.workshop-hero').classes()).not.toContain('compact')
  })

  it('switches the hero to compact mode once the prompt has content and latches it', async () => {
    mockUser()
    mockWorks()
    mockDiscovery()
    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('一只赛博朋克猫')
    expect(wrapper.get('.workshop-hero').classes()).toContain('compact')

    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('')
    expect(wrapper.get('.workshop-hero').classes()).toContain('compact')
  })

  it('switches the hero to compact mode when the composer area gains focus', async () => {
    mockUser()
    mockWorks()
    mockDiscovery()
    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-composer-engage-zone"]').trigger('focusin')
    expect(wrapper.get('.workshop-hero').classes()).toContain('compact')
  })

  it('starts in compact hero mode for returning users with works', async () => {
    mockUser()
    mockWorks([makeWork(1)])
    mockDiscovery()
    const wrapper = await mountReady()

    expect(wrapper.get('.workshop-hero').classes()).toContain('compact')
  })

  it('keeps the mode tabs operable in compact hero mode', async () => {
    mockUser()
    mockWorks([makeWork(1)])
    mockDiscovery()
    const wrapper = await mountReady()

    expect(wrapper.get('.workshop-hero').classes()).toContain('compact')
    await wrapper.get('[data-testid="workspace-mode-video"]').trigger('click')
    expect(routerPush).toHaveBeenCalledWith('/workspace/video')
  })

  it('renders the workshop home hero, quick prompts, feature entries and ecommerce workflow', async () => {
    mockUser()
    mockWorks()
    mockReferenceAssets()

    const wrapper = await mountReady()

    expect(wrapper.get('[data-testid="workshop-home"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="workshop-hero-title"]').text()).toContain('今天你想要 创作 什么？')
    expect(wrapper.get('[data-testid="workspace-mode-agent"]').text()).toBe('Agent模式')
    expect(wrapper.get('[data-testid="workspace-tab-create"]').text()).toBe('图片生成')
    expect(wrapper.get('[data-testid="workspace-mode-video"]').text()).toBe('视频生成')
    expect(wrapper.find('[data-testid="workspace-mode-virtual-try-on"]').exists()).toBe(false)
    expect(wrapper.get('.workshop-mode-tabs').text()).not.toContain('建模试衣')
    expect(wrapper.get('[data-testid="workspace-tab-create"]').classes()).toContain('active')
    expect(wrapper.get('[data-testid="workspace-prompt-input"]').attributes('placeholder')).toBe('描述你的想法，或上传参考图生成图片')

    expect(wrapper.findAll('[data-testid^="workspace-quick-prompt-"]')).toHaveLength(4)
    const featureEntries = wrapper.findAll('[data-testid^="workspace-feature-"]')
    expect(featureEntries).toHaveLength(7)
    expect(wrapper.get('[data-testid="workspace-feature-ai-canvas"]').text()).toContain('AI 画布')
    expect(wrapper.get('[data-testid="workspace-feature-ai-canvas"]').text()).toContain('即将开放')
    expect(wrapper.get('[data-testid="workspace-feature-skill-hub"]').text()).toContain('SKILL HUB')
    expect(wrapper.get('[data-testid="workspace-feature-skill-hub"]').text()).toContain('即将开放')
    expect(wrapper.get('[data-testid="workspace-feature-ai-commerce"]').text()).toContain('AI 电商')
    expect(wrapper.get('[data-testid="workspace-feature-video"]').text()).toContain('视频创作')
    expect(wrapper.get('[data-testid="workspace-feature-image"]').text()).toContain('图片创作')
    expect(wrapper.get('[data-testid="workspace-feature-image-edit"]').text()).toContain('AI 改图')
    expect(wrapper.get('[data-testid="workspace-feature-text-edit"]').text()).toContain('AI 改字')
    expect(wrapper.get('[data-testid="workspace-feature-text-edit"]').text()).toContain('即将开放')
    featureEntries.forEach((entry) => {
      const svg = entry.get('svg')
      expect(svg.attributes('width')).toBe('26')
      expect(svg.attributes('height')).toBe('26')
    })

    const styles = readStyles()
    expect(styles).toMatch(/\.workshop-feature-grid\s*{[^}]*grid-template-columns:\s*repeat\(7,\s*minmax\(0,\s*1fr\)\);/)
    expect(styles).not.toMatch(/\.workshop-feature-grid\s*{[^}]*grid-template-columns:\s*repeat\(4,\s*minmax\(0,\s*1fr\)\);/)

    const workflow = wrapper.get('[data-testid="workspace-ecommerce-workflow"]')
    expect(workflow.text()).toContain('电商工作流')
    expect(workflow.findAll('[data-testid^="workspace-workflow-card-"]').length).toBeGreaterThanOrEqual(6)
    expect(workflow.text()).toContain('拍立得风格')
  })

  it('opens the dedicated AI commerce workspace while other upcoming entries stay disabled', async () => {
    mockUser()
    mockWorks()
    mockReferenceAssets()

    const wrapper = await mountReady()

    expect(wrapper.get('[data-testid="workspace-feature-ai-canvas"]').attributes('aria-disabled')).toBe('true')
    expect(wrapper.get('[data-testid="workspace-feature-skill-hub"]').attributes('aria-disabled')).toBe('true')
    expect(wrapper.get('[data-testid="workspace-feature-text-edit"]').attributes('aria-disabled')).toBe('true')

    await wrapper.get('[data-testid="workspace-discovery-filter-tool"]').trigger('click')
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="workspace-ecommerce-workflow"]').exists()).toBe(false)

    await wrapper.get('[data-testid="workspace-feature-ai-commerce"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(routerPush).toHaveBeenCalledWith('/workspace/ai-commerce')
  })

  it('renders the home composer as a compact manuscript-style panel', async () => {
    mockUser()
    mockWorks()
    mockReferenceAssets()
    mockDiscovery({
      models: [
        {
          id: 7,
          name: 'BailinAI GPT Image 2',
          default_credits_cost: 2,
          capability_tags: ['image', 'reference']
        }
      ]
    })

    const wrapper = await mountReady()
    const composer = wrapper.get('[data-testid="workspace-composer-form"]')

    expect(composer.classes()).toContain('imini-composer-card--home')
    const referenceUpload = composer.get('[data-testid="workspace-reference-upload"]')
    const promptInput = composer.get('[data-testid="workspace-prompt-input"]')
    expect(referenceUpload.classes()).toContain('imini-home-reference-pane')
    expect(promptInput.classes()).toContain('imini-home-prompt-input')
    const controlBar = composer.get('[data-testid="workspace-home-control-bar"]')
    expect(controlBar.exists()).toBe(true)
    const createButton = composer.get('[data-testid="workspace-create-button"]')
    const composerChildren = Array.from(composer.element.children)
    expect(composerChildren.indexOf(referenceUpload.element)).toBeLessThan(composerChildren.indexOf(promptInput.element.parentElement))
    expect(composerChildren.indexOf(promptInput.element.parentElement)).toBeLessThan(composerChildren.indexOf(controlBar.element))
    expect(composerChildren.indexOf(controlBar.element)).toBeLessThan(composerChildren.indexOf(createButton.element))
    const advancedToggle = controlBar.get('[data-testid="workspace-home-advanced-toggle"]')
    expect(advancedToggle.text()).toContain('高级')
    expect(advancedToggle.attributes('aria-label')).toBe('高级选项')
    expect(advancedToggle.attributes('aria-expanded')).toBe('false')
    expect(advancedToggle.attributes('aria-controls')).toBe('workspace-home-advanced-panel')
    expect(composer.find('[data-testid="workspace-home-advanced-panel"]').exists()).toBe(false)
    expect(composer.get('[data-testid="workspace-model-select"]').exists()).toBe(true)
    expect(composer.get('[data-testid="workspace-size-select"]').exists()).toBe(true)
    expect(composer.get('[data-testid="workspace-quality-select"]').exists()).toBe(true)
    expect(composer.find('[data-testid="workspace-quality-ultra"]').exists()).toBe(false)
    expect(createButton.classes()).toContain('imini-create-button--round')
    expect(createButton.attributes('aria-label')).toContain('创建图片')

    expect(composer.get('[data-testid="workspace-model-select"]').text()).toBe('模型')
    expect(composer.get('[data-testid="workspace-size-select"]').text()).toBe('比例')
    expect(composer.get('[data-testid="workspace-quality-select"]').text()).toBe('分辨率')

    await openClickSelect(wrapper, 'workspace-model-select')
    expect(clickSelectMenu('workspace-model-select')?.textContent).toContain('BailinAI GPT Image 2')
    await wrapper.get('[data-testid="workspace-model-select"]').trigger('click')

    await openClickSelect(wrapper, 'workspace-size-select')
    expect(clickSelectMenu('workspace-size-select')?.textContent).toContain('9:21 长屏')
    await wrapper.get('[data-testid="workspace-size-select"]').trigger('click')

    await openClickSelect(wrapper, 'workspace-quality-select')
    expect(clickSelectMenu('workspace-quality-select')?.textContent).toContain('4K')
  })

  it('keeps segmented quality buttons in the default composer layout', () => {
    const wrapper = mountComposerPanel()

    expect(wrapper.get('[data-testid="workspace-quality-ultra"]').text()).toBe('4K')
    expect(wrapper.find('[data-testid="workspace-quality-select"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="workspace-composer-form"]').classes()).not.toContain('imini-composer-card--home')
  })

  it('emits reference uploads from the compact empty attachment entry by select and drop', async () => {
    const wrapper = mountComposerPanel()
    const selectedFile = file('select.png')
    const droppedFile = file('drop.jpg', 'image/jpeg')

    await uploadWorkspaceReference(wrapper, selectedFile)
    await dropWorkspaceReference(wrapper, [droppedFile])

    expect(wrapper.emitted('upload-reference')).toEqual([[selectedFile], [droppedFile]])
  })

  it('shows uploaded references as a compact stack and expands to a removable grid', async () => {
    const first = {
      id: 42,
      preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/first.png',
      original_filename: 'first.png'
    }
    const second = {
      id: 43,
      preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/second.png',
      original_filename: 'second.png'
    }
    const wrapper = mountComposerPanel({
      selectedReferenceImages: [first, second],
      sourceImageLimit: 4
    })

    expect(wrapper.get('[data-testid="workspace-reference-count"]').text()).toContain('已选 2/4')
    expect(wrapper.get('.workspace-reference-stack').attributes('aria-expanded')).toBe('false')
    expect(wrapper.findAll('[data-testid="workspace-reference-stack-thumb"]')).toHaveLength(2)
    expect(wrapper.text()).toContain('first.png')
    expect(wrapper.get('[data-testid="workspace-reference-add"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="workspace-reference-toggle"]').attributes('aria-expanded')).toBe('false')
    expect(wrapper.find('[data-testid="workspace-reference-grid"]').exists()).toBe(false)

    await wrapper.get('[data-testid="workspace-reference-toggle"]').trigger('click')

    expect(wrapper.get('[data-testid="workspace-reference-toggle"]').text()).toContain('收起')
    expect(wrapper.findAll('[data-testid="workspace-reference-grid-item"]')).toHaveLength(2)
    expect(wrapper.get('[data-testid="workspace-reference-more"]').attributes('disabled')).toBeUndefined()

    await wrapper.findAll('[data-testid="workspace-reference-remove"]')[0].trigger('click')
    expect(wrapper.emitted('remove-reference')).toEqual([[first]])
  })

  it('sizes selected reference thumbnails large enough to recognize the uploaded image', () => {
    const styles = readStyles()

    expect(styles).toMatch(
      /\.workspace-reference-stack\s*{[^}]*width:\s*108px;[^}]*height:\s*78px;[^}]*}/s
    )
    expect(styles).toMatch(
      /\.workspace-reference-stack-thumb\s*{[^}]*width:\s*72px;[^}]*height:\s*72px;[^}]*}/s
    )
    expect(styles).toMatch(
      /\.workspace-reference-stack-thumb img,\s*\.workspace-reference-grid-item img\s*{[^}]*object-fit:\s*cover;[^}]*}/s
    )
  })

  it('disables the expanded add entry at the source image limit', async () => {
    const wrapper = mountComposerPanel({
      selectedReferenceImages: [
        { id: 42, preview_url: 'https://example.com/first.png', original_filename: 'first.png' },
        { id: 43, preview_url: 'https://example.com/second.png', original_filename: 'second.png' }
      ],
      sourceImageLimit: 2
    })

    await wrapper.get('[data-testid="workspace-reference-toggle"]').trigger('click')

    expect(wrapper.get('[data-testid="workspace-reference-more"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="workspace-reference-more"]').text()).toContain('最多 2 张')
  })

  it('disables upload and removal while reference upload or submit is running', async () => {
    const wrapper = mountComposerPanel({
      selectedReferenceImages: [
        { id: 42, preview_url: 'https://example.com/source.png', original_filename: 'source.png' }
      ],
      referenceUploading: true
    })

    await wrapper.get('[data-testid="workspace-reference-toggle"]').trigger('click')

    expect(wrapper.get('[data-testid="workspace-reference-file-input"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="workspace-reference-remove"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="workspace-reference-upload-status"]').text()).toContain('上传中')
  })

  it('opens home advanced options as a floating panel without changing advanced payload fields', async () => {
    mockUser({ available_credits: 5 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.uploadReferenceAsset.mockResolvedValueOnce({
      id: 42,
      preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/ref.png',
      original_filename: 'ref.png'
    })
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 12,
      status: 'queued',
      stage: 'queued',
      available_credits: 4
    })

    const wrapper = await mountReady()

    await uploadWorkspaceReference(wrapper, new File(['fake'], 'ref.png', { type: 'image/png' }))
    await flushPromises()
    await wrapper.get('[data-testid="workspace-home-advanced-toggle"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="workspace-home-advanced-toggle"]').attributes('aria-expanded')).toBe('true')
    const advancedPanelElement = document.querySelector('[data-testid="workspace-home-advanced-panel"]')
    expect(advancedPanelElement).not.toBeNull()
    const advancedPanel = wrapper.find('[data-testid="workspace-home-advanced-panel"]')
    expect(advancedPanel.exists()).toBe(false)
    expect(advancedPanelElement.getAttribute('open')).toBeNull()
    expect(advancedPanelElement.getAttribute('role')).toBe('dialog')
    expect(advancedPanelElement.classList.contains('workspace-home-advanced-panel')).toBe(true)
    expect(advancedPanelElement.classList.contains('imini-advanced-options--home')).toBe(true)
    expect(advancedPanelElement.querySelector('[data-testid="workspace-negative-prompt"]')).not.toBeNull()
    expect(advancedPanelElement.querySelector('[data-testid="workspace-reference-strength"]')).not.toBeNull()

    const negativePromptInput = advancedPanelElement.querySelector('[data-testid="workspace-negative-prompt"]')
    negativePromptInput.value = 'noise, watermark'
    negativePromptInput.dispatchEvent(new Event('input', { bubbles: true }))
    advancedPanelElement.querySelectorAll('.style-chip').forEach((node) => {
      if (node.textContent === '国风') {
        node.dispatchEvent(new MouseEvent('click', { bubbles: true }))
      }
    })
    const referenceStrengthInput = advancedPanelElement.querySelector('[data-testid="workspace-reference-strength"]')
    referenceStrengthInput.value = '58'
    referenceStrengthInput.dispatchEvent(new Event('input', { bubbles: true }))
    await wrapper.vm.$nextTick()
    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('compose these references into one poster')
    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: 'compose these references into one poster',
      negative_prompt: 'noise, watermark',
      style_preset: '国风',
      reference_asset_ids: [42],
      reference_weight: 58
    }))
  })

  it('keeps the previous single source selected when replacement upload fails', async () => {
    mockUser({ available_credits: 5 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.uploadReferenceAsset
      .mockResolvedValueOnce({
        id: 42,
        preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/old-ref.png',
        original_filename: 'old-ref.png'
      })
      .mockRejectedValueOnce(new Error('OSS 上传失败，请重试'))
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 22,
      status: 'queued',
      stage: 'queued',
      available_credits: 4
    })

    const wrapper = await mountReady()
    await wrapper.get('[data-testid="workspace-tool-expand"]').trigger('click')
    await uploadWorkspaceReference(wrapper, new File(['old'], 'old-ref.png', { type: 'image/png' }))
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="workspace-reference-upload"]').text()).toContain('old-ref.png')

    await uploadWorkspaceReference(wrapper, new File(['new'], 'new-ref.png', { type: 'image/png' }))
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="workspace-reference-upload"]').text()).toContain('OSS 上传失败，请重试')
    expect(wrapper.get('[data-testid="workspace-reference-upload"]').text()).toContain('old-ref.png')

    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('延展背景')
    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith(expect.objectContaining({
      tool_mode: 'expand',
      reference_asset_ids: [42]
    }))
  })

  it('rejects reference images larger than 50MB before calling the upload API', async () => {
    mockUser({ available_credits: 5 })
    mockWorks()
    mockReferenceAssets()

    const wrapper = await mountReady()
    await uploadWorkspaceReference(wrapper, oversizedImageFile())
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(apiMocks.uploadReferenceAsset).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="workspace-reference-upload"]').text()).toContain('单张图片不能超过 50MB')
  })

  it('fills quick prompt chips and routes the video segmented mode', async () => {
    mockUser()
    mockWorks()
    mockReferenceAssets()

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-quick-prompt-commerce-hero"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="workspace-prompt-input"]').element.value).toContain('电商主图')
    expect(routerPush).not.toHaveBeenCalledWith('/workspace/video')

    await wrapper.get('[data-testid="workspace-mode-video"]').trigger('click')
    expect(routerPush).toHaveBeenCalledWith('/workspace/video')
  })

  it('shows a recharge warning when the user submits without credits', async () => {
    mockUser({ available_credits: 0 })
    mockWorks()
    apiMocks.estimateImageGeneration.mockResolvedValueOnce({
      required_credits: 2,
      available_credits: 0,
      missing_credits: 2,
      enough: false
    })

    const wrapper = await mountReady()
    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('需要生成的图')
    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('点数不足')
    expect(wrapper.text()).toContain('套餐与充值')
    expect(apiMocks.createImageGeneration).not.toHaveBeenCalled()
  })

  it('keeps creation available when credit estimation fails over the network', async () => {
    vi.useFakeTimers()
    mockUser({ available_credits: 3 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.estimateImageGeneration.mockRejectedValueOnce(new Error('Failed to fetch'))
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 18,
      status: 'queued',
      stage: 'queued',
      available_credits: 1
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('弱网下也允许真实提交')
    await advanceCreditEstimateDebounce()

    expect(wrapper.find('.error-message').exists()).toBe(false)
    expect(wrapper.get('[data-testid="workspace-credit-estimate-notice"]').attributes('title')).toBe('点数预估暂不可用，不影响提交')
    expect(wrapper.get('[data-testid="workspace-create-button"]').attributes('disabled')).toBeUndefined()

    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: '弱网下也允许真实提交'
    }))
  })

  it('debounces regular workspace credit estimates while the prompt is changing', async () => {
    vi.useFakeTimers()
    mockUser({ available_credits: 3 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.estimateImageGeneration.mockResolvedValue({
      required_credits: 2,
      available_credits: 3,
      missing_credits: 0,
      enough: true
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('first draft')
    await advanceCreditEstimateDebounce(599)
    expect(apiMocks.estimateImageGeneration).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('final prompt')
    await advanceCreditEstimateDebounce(599)
    expect(apiMocks.estimateImageGeneration).not.toHaveBeenCalled()

    await advanceCreditEstimateDebounce(1)
    expect(apiMocks.estimateImageGeneration).toHaveBeenCalledTimes(1)
    expect(apiMocks.estimateImageGeneration).toHaveBeenLastCalledWith(
      expect.objectContaining({ prompt: 'final prompt' }),
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
  })

  it('aborts an in-flight regular credit estimate when the prompt changes again', async () => {
    vi.useFakeTimers()
    mockUser({ available_credits: 3 })
    mockWorks()
    mockReferenceAssets()
    const pendingEstimate = deferred()
    apiMocks.estimateImageGeneration
      .mockReturnValueOnce(pendingEstimate.promise)
      .mockResolvedValueOnce({
        required_credits: 1,
        available_credits: 3,
        missing_credits: 0,
        enough: true
      })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('first prompt')
    await advanceCreditEstimateDebounce()
    const firstSignal = apiMocks.estimateImageGeneration.mock.calls[0][1]?.signal
    expect(firstSignal).toBeInstanceOf(AbortSignal)
    expect(firstSignal.aborted).toBe(false)

    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('second prompt')
    expect(firstSignal.aborted).toBe(true)

    await advanceCreditEstimateDebounce()
    expect(apiMocks.estimateImageGeneration).toHaveBeenCalledTimes(2)
    expect(apiMocks.estimateImageGeneration).toHaveBeenLastCalledWith(
      expect.objectContaining({ prompt: 'second prompt' }),
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
  })

  it('ignores stale regular credit estimate responses after a newer estimate wins', async () => {
    vi.useFakeTimers()
    mockUser({ available_credits: 3 })
    mockWorks()
    mockReferenceAssets()
    const staleEstimate = deferred()
    apiMocks.estimateImageGeneration
      .mockReturnValueOnce(staleEstimate.promise)
      .mockResolvedValueOnce({
        required_credits: 1,
        available_credits: 3,
        missing_credits: 0,
        enough: true
      })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('slow prompt')
    await advanceCreditEstimateDebounce()
    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('fresh prompt')
    await advanceCreditEstimateDebounce()

    expect(wrapper.find('.error-message').exists()).toBe(false)
    expect(wrapper.get('[data-testid="workspace-create-button"]').attributes('disabled')).toBeUndefined()

    staleEstimate.resolve({
      required_credits: 9,
      available_credits: 0,
      missing_credits: 9,
      enough: false
    })
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(wrapper.find('.error-message').exists()).toBe(false)
    expect(wrapper.get('[data-testid="workspace-create-button"]').attributes('disabled')).toBeUndefined()
  })

  it('shows a non-blocking notice instead of an error bar when regular credit estimates are rate limited', async () => {
    vi.useFakeTimers()
    mockUser({ available_credits: 3 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.estimateImageGeneration.mockRejectedValueOnce(
      new ApiError('too_many_requests', 'too many estimate requests', 429)
    )

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('rate limited prompt')
    await advanceCreditEstimateDebounce()

    expect(wrapper.find('.error-message').exists()).toBe(false)
    expect(wrapper.get('[data-testid="workspace-create-button"]').attributes('disabled')).toBeUndefined()
    const notice = wrapper.get('[data-testid="workspace-credit-estimate-notice"]')
    expect(notice.attributes('title')).toBe('点数预估暂不可用，不影响提交')
    expect(notice.attributes('aria-label')).toBe('点数预估暂不可用，不影响提交')
  })

  it('shows reference asset estimate errors and clears stale selected assets', async () => {
    vi.useFakeTimers()
    mockUser({ available_credits: 3 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.uploadReferenceAsset.mockResolvedValueOnce({
      id: 42,
      preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/ref.png',
      original_filename: 'ref.png'
    })
    apiMocks.estimateImageGeneration.mockRejectedValueOnce(
      new ApiError('reference_asset_not_found', '参考素材不存在', 404)
    )
    apiMocks.listReferenceAssets.mockResolvedValueOnce({ items: [] })

    const wrapper = await mountReady()

    await uploadWorkspaceReference(wrapper, new File(['fake'], 'ref.png', { type: 'image/png' }))
    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('用参考图生成头像')
    await advanceCreditEstimateDebounce()
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('点数预估失败：参考素材不存在，请重新选择图片')
    expect(wrapper.get('[data-testid="workspace-create-button"]').attributes('disabled')).toBeDefined()
    expect(apiMocks.listReferenceAssets).toHaveBeenCalledTimes(2)

    await flushPromises()
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="workspace-reference-upload"]').text()).not.toContain('ref.png')
  })

  it('shows reference work estimate errors and clears stale selected works', async () => {
    vi.useFakeTimers()
    mockUser({ available_credits: 3 })
    mockWorks([
      {
        work_id: 90,
        prompt: 'existing portrait',
        preview_url: '/api/works/90/file',
        download_url: '/api/works/90/download',
        aspect_ratio: '1:1',
        category: 'image',
        created_at: '2026-04-28T10:00:00Z'
      }
    ])
    mockReferenceAssets()
    apiMocks.estimateImageGeneration.mockRejectedValueOnce(
      new ApiError('reference_work_not_found', '参考作品不存在', 404)
    )
    apiMocks.listWorks.mockResolvedValueOnce({ items: [], page: 1, total: 0 })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tab-create"]').trigger('click')
    await wrapper.get('[data-testid="workspace-history-use-as-reference"]').trigger('click')
    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('用历史作品生成头像')
    await advanceCreditEstimateDebounce()
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('点数预估失败：参考作品不存在，请重新选择图片')
    expect(wrapper.get('[data-testid="workspace-create-button"]').attributes('disabled')).toBeDefined()
    expect(apiMocks.listWorks).toHaveBeenCalledTimes(2)

    await flushPromises()
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="workspace-reference-upload"]').text()).not.toContain('existing portrait')
  })

  it('shows csrf estimate errors without the generic fallback', async () => {
    vi.useFakeTimers()
    mockUser({ available_credits: 3 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.estimateImageGeneration.mockRejectedValueOnce(
      new ApiError('csrf_invalid', 'CSRF Token 无效，请刷新页面后重试', 403)
    )

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('普通创作')
    await advanceCreditEstimateDebounce()

    expect(wrapper.text()).toContain('点数预估失败：CSRF Token 无效，请刷新页面后重试')
    expect(wrapper.text()).not.toContain('点数预估失败，可重试或直接创建')
  })

  it('still blocks creation when credit estimation confirms insufficient credits', async () => {
    vi.useFakeTimers()
    mockUser({ available_credits: 0 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.estimateImageGeneration.mockResolvedValueOnce({
      required_credits: 2,
      available_credits: 0,
      missing_credits: 2,
      enough: false
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('点数不足的请求')
    await advanceCreditEstimateDebounce()

    expect(wrapper.text()).toContain('点数不足，还差 2 点')
    expect(wrapper.get('[data-testid="workspace-create-button"]').attributes('disabled')).toBeDefined()
  })

  it('renders the main workspace when non-critical bootstrap requests fail', async () => {
    mockUser({ available_credits: 3 })
    apiMocks.listWorks.mockRejectedValueOnce(new Error('Failed to fetch'))
    apiMocks.listReferenceAssets.mockRejectedValueOnce(new Error('参考素材读取失败'))
    apiMocks.getWorkspaceDiscovery.mockRejectedValueOnce(new Error('模板读取失败'))

    const wrapper = await mountReady()

    expect(wrapper.find('[data-testid="workspace-composer-form"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="workshop-home"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('参考素材读取失败')
    expect(wrapper.text()).toContain('模板读取失败')
    expect(wrapper.text()).not.toContain('Failed to fetch')

    await wrapper.get('[data-testid="workspace-tab-create"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('作品记录读取失败')
  })

  it('uses the latest saved work as the default main preview', async () => {
    mockUser()
    mockWorks([
      {
        work_id: 90,
        prompt: 'mist over bamboo lake',
        preview_url: '/api/works/90/file',
        download_url: '/api/works/90/download',
        created_at: '2026-04-28T10:00:00Z'
      }
    ])

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tab-create"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="workspace-create-panel"]').classes()).toContain('imini-create-surface')
    expect(wrapper.get('[data-testid="workspace-result-stage"]').classes()).toContain('imini-result-stage')
    expect(wrapper.get('[data-testid="workspace-result-preview"]').classes()).toContain('imini-result-frame')
    expect(wrapper.get('.preview-image').attributes('src')).toBe('/api/works/90/file')
    expect(wrapper.text()).toContain('mist over bamboo lake')
  })

  it('keeps discovery as the default view for logged-in users with saved works', async () => {
    mockUser()
    mockWorks([
      {
        work_id: 90,
        prompt: 'mist over bamboo lake',
        preview_url: '/api/works/90/file',
        download_url: '/api/works/90/download',
        created_at: '2026-04-28T10:00:00Z'
      }
    ])

    const wrapper = await mountReady()

    expect(wrapper.get('[data-testid="workspace-discovery-panel"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="workspace-create-panel"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="workspace-discovery-panel"]').text()).toContain('AI 工具')
  })

  it('centers the empty create state in the result stage without changing submission payloads', async () => {
    mockUser({ available_credits: 5 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 12,
      status: 'queued',
      stage: 'queued',
      available_credits: 4
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tab-create"]').trigger('click')
    await wrapper.vm.$nextTick()

    const createPanel = wrapper.get('[data-testid="workspace-create-panel"]')
    expect(createPanel.get('[data-testid="workspace-result-stage"]').exists()).toBe(true)
    expect(createPanel.get('[data-testid="workspace-result-empty"]').text()).toContain('创建您的第一个创作~')
    expect(createPanel.text()).not.toContain('等待生成结果')
    expect(createPanel.text()).toContain('生成记录（共 0）')

    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('minimal payload check')
    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: 'minimal payload check',
      aspect_ratio: '1:1',
      model_id: 7,
      tool_mode: 'generate'
    }))
  })

  it('loads 18 generation history items and paginates without refreshing the whole workspace', async () => {
    mockUser()
    const firstPageWorks = Array.from({ length: 18 }, (_, index) => makeWork(100 - index))
    const secondPageWorks = [makeWork(10, { prompt: '第 2 页作品' }), makeWork(9, { prompt: '第 2 页旧作品' })]
    apiMocks.listWorks
      .mockResolvedValueOnce({
        items: firstPageWorks,
        page: 1,
        page_size: 18,
        total: 20
      })
      .mockResolvedValueOnce({
        items: secondPageWorks,
        page: 2,
        page_size: 18,
        total: 20
      })
      .mockResolvedValueOnce({
        items: firstPageWorks,
        page: 1,
        page_size: 18,
        total: 20
      })
    mockReferenceAssets()

    const wrapper = await mountReady()

    expect(apiMocks.listWorks).toHaveBeenNthCalledWith(1, { media_type: 'image', page: 1, page_size: 18 })

    await wrapper.get('[data-testid="workspace-tab-create"]').trigger('click')
    await wrapper.vm.$nextTick()

    const createPanel = wrapper.get('[data-testid="workspace-create-panel"]')
    expect(createPanel.text()).toContain('生成记录（共 20）')
    expect(createPanel.findAll('.history-card')).toHaveLength(18)
    expect(createPanel.get('[data-testid="workspace-works-page-status"]').text()).toContain('第 1 / 2 页')
    expect(createPanel.get('[data-testid="workspace-works-prev"]').attributes('disabled')).toBeDefined()
    expect(createPanel.get('[data-testid="workspace-works-next"]').attributes('disabled')).toBeUndefined()

    await createPanel.get('[data-testid="workspace-works-next"]').trigger('click')
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(apiMocks.listWorks).toHaveBeenNthCalledWith(2, { media_type: 'image', page: 2, page_size: 18 })
    expect(wrapper.get('[data-testid="workspace-create-panel"]').findAll('.history-card')).toHaveLength(2)
    expect(wrapper.get('[data-testid="workspace-create-panel"]').text()).toContain('第 2 页作品')
    expect(wrapper.get('[data-testid="workspace-works-page-status"]').text()).toContain('第 2 / 2 页')

    await wrapper.get('[data-testid="workspace-works-prev"]').trigger('click')
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(apiMocks.listWorks).toHaveBeenNthCalledWith(3, { media_type: 'image', page: 1, page_size: 18 })
    expect(wrapper.get('[data-testid="workspace-create-panel"]').findAll('.history-card')).toHaveLength(18)
    expect(wrapper.get('[data-testid="workspace-works-page-status"]').text()).toContain('第 1 / 2 页')
  })

  it('keeps the current generation history page when pagination loading fails', async () => {
    mockUser()
    const firstPageWorks = Array.from({ length: 18 }, (_, index) => makeWork(200 - index))
    apiMocks.listWorks
      .mockResolvedValueOnce({
        items: firstPageWorks,
        page: 1,
        page_size: 18,
        total: 20
      })
      .mockRejectedValueOnce(new Error('第 2 页读取失败'))
    mockReferenceAssets()

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tab-create"]').trigger('click')
    await wrapper.vm.$nextTick()
    await wrapper.get('[data-testid="workspace-works-next"]').trigger('click')
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(apiMocks.listWorks).toHaveBeenNthCalledWith(2, { media_type: 'image', page: 2, page_size: 18 })
    expect(wrapper.get('[data-testid="workspace-create-panel"]').findAll('.history-card')).toHaveLength(18)
    expect(wrapper.get('[data-testid="workspace-works-page-status"]').text()).toContain('第 1 / 2 页')
    expect(wrapper.get('[data-testid="workspace-create-panel"]').text()).toContain('第 2 页读取失败')
  })

  it('keeps the composer editable and submits another image while the previous task is still running', async () => {
    vi.useFakeTimers()
    mockUser({ available_credits: 8 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.createImageGeneration
      .mockResolvedValueOnce({
        generation_id: 101,
        status: 'queued',
        stage: 'queued',
        created_at: '2026-06-06T10:00:00Z',
        available_credits: 7
      })
      .mockResolvedValueOnce({
        generation_id: 102,
        status: 'queued',
        stage: 'queued',
        created_at: '2026-06-06T10:00:03Z',
        available_credits: 6
      })
    apiMocks.getImageGeneration.mockResolvedValue({
      generation_id: 101,
      status: 'running',
      stage: 'requesting_provider',
      prompt: '第一张竹林湖面',
      available_credits: 7
    })

    const wrapper = await mountReady()

    await submitWorkspacePrompt(wrapper, '第一张竹林湖面')

    expect(wrapper.get('[data-testid="workspace-prompt-input"]').attributes('disabled')).toBeUndefined()
    expect(wrapper.get('[data-testid="workspace-create-button"]').attributes('disabled')).toBeUndefined()
    expect(wrapper.get('[data-testid="workspace-generation-task-101"]').text()).toContain('排队中')

    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('第二张城市夜景')
    await chooseClickSelect(wrapper, 'workspace-quality-select', 'high')
    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledTimes(2)
    expect(apiMocks.createImageGeneration).toHaveBeenNthCalledWith(1, expect.objectContaining({
      prompt: '第一张竹林湖面'
    }))
    expect(apiMocks.createImageGeneration.mock.calls[0][0]).not.toHaveProperty('quality')
    expect(apiMocks.createImageGeneration).toHaveBeenNthCalledWith(2, expect.objectContaining({
      prompt: '第二张城市夜景',
      quality: 'high'
    }))
    expect(wrapper.get('[data-testid="workspace-generation-task-102"]').classes()).toContain('active')
    expect(wrapper.get('[data-testid="workspace-generation-tasks"]').text()).toContain('生成任务 (2)')
  })

  it('cancels a running image generation without browser confirm and keeps retry payload', async () => {
    mockUser({ available_credits: 8 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 101,
      status: 'queued',
      stage: 'queued',
      created_at: '2026-06-06T10:00:00Z',
      prompt: 'wrong prompt',
      parameters: {
        prompt: 'wrong prompt',
        aspect_ratio: '1:1',
        tool_mode: 'generate'
      },
      available_credits: 7
    })
    apiMocks.cancelImageGeneration.mockResolvedValueOnce({
      generation_id: 101,
      status: 'failed',
      stage: 'failed',
      created_at: '2026-06-06T10:00:00Z',
      prompt: 'wrong prompt',
      parameters: {
        prompt: 'wrong prompt',
        aspect_ratio: '1:1',
        tool_mode: 'generate'
      },
      available_credits: 7,
      credits_cost: 1,
      credits_deducted: false,
      error: {
        code: 'user_cancelled',
        message: '已取消生成，未扣点。',
        retryable: true
      }
    })

    const wrapper = await mountReady()
    await submitWorkspacePrompt(wrapper, 'wrong prompt')

    expect(wrapper.get('[data-testid="workspace-cancel-generation"]').text()).toContain('取消生成')
    expect(wrapper.get('[data-testid="workspace-result-cancel-generation"]').text()).toContain('取消生成')
    expect(wrapper.get('[data-testid="workspace-generation-task-101-cancel"]').text()).toContain('取消')

    await wrapper.get('[data-testid="workspace-cancel-generation"]').trigger('click')
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(window.confirm).not.toHaveBeenCalled()
    expect(apiMocks.cancelImageGeneration).toHaveBeenCalledWith(101)
    expect(wrapper.get('[data-testid="workspace-generation-failure-notice"]').text()).toContain('已取消生成')
    expect(wrapper.get('[data-testid="workspace-result-error"]').text()).toContain('已取消生成')
    expect(wrapper.text()).not.toContain('网络超时')
    expect(window.sessionStorage.getItem(activeGenerationStorageKey)).toBeNull()

    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 102,
      status: 'queued',
      stage: 'queued',
      available_credits: 6
    })
    await wrapper.get('[data-testid="workspace-failure-retry-generation"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledTimes(2)
    expect(apiMocks.createImageGeneration).toHaveBeenLastCalledWith(expect.objectContaining({
      prompt: 'wrong prompt',
      aspect_ratio: '1:1',
      tool_mode: 'generate'
    }))
  })

  it('cancels only the selected concurrent task and keeps polling the other one', async () => {
    vi.useFakeTimers()
    mockUser({ available_credits: 8 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.createImageGeneration
      .mockResolvedValueOnce({
        generation_id: 101,
        status: 'queued',
        stage: 'queued',
        created_at: '2026-06-06T10:00:00Z',
        prompt: 'first prompt',
        parameters: { prompt: 'first prompt', aspect_ratio: '1:1', tool_mode: 'generate' },
        available_credits: 7
      })
      .mockResolvedValueOnce({
        generation_id: 102,
        status: 'queued',
        stage: 'queued',
        created_at: '2026-06-06T10:00:02Z',
        prompt: 'second prompt',
        parameters: { prompt: 'second prompt', aspect_ratio: '1:1', tool_mode: 'generate' },
        available_credits: 6
      })
    apiMocks.cancelImageGeneration.mockResolvedValueOnce({
      generation_id: 101,
      status: 'failed',
      stage: 'failed',
      prompt: 'first prompt',
      parameters: { prompt: 'first prompt', aspect_ratio: '1:1', tool_mode: 'generate' },
      available_credits: 6,
      credits_deducted: false,
      error: { code: 'user_cancelled', message: '已取消生成，未扣点。', retryable: true }
    })
    apiMocks.getImageGeneration.mockImplementation(async (id) => ({
      generation_id: id,
      status: 'running',
      stage: 'requesting_provider',
      prompt: id === 101 ? 'first prompt' : 'second prompt',
      available_credits: 6
    }))

    const wrapper = await mountReady()
    await submitWorkspacePrompt(wrapper, 'first prompt')
    await submitWorkspacePrompt(wrapper, 'second prompt')

    await wrapper.get('[data-testid="workspace-generation-task-101-cancel"]').trigger('click')
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(apiMocks.cancelImageGeneration).toHaveBeenCalledWith(101)
    expect(wrapper.get('[data-testid="workspace-generation-task-101"]').classes()).toContain('failed')
    expect(wrapper.get('[data-testid="workspace-generation-task-102"]').classes()).not.toContain('failed')

    apiMocks.getImageGeneration.mockClear()
    await vi.advanceTimersByTimeAsync(1000)
    await flushPromises()

    expect(apiMocks.getImageGeneration).toHaveBeenCalledWith(102)
    expect(apiMocks.getImageGeneration).not.toHaveBeenCalledWith(101)
  })

  it('uses a succeeded payload when cancel races with provider completion', async () => {
    mockUser({ available_credits: 8 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 101,
      status: 'queued',
      stage: 'queued',
      created_at: '2026-06-06T10:00:00Z',
      prompt: 'prompt that finishes',
      parameters: { prompt: 'prompt that finishes', aspect_ratio: '1:1', tool_mode: 'generate' },
      available_credits: 7
    })
    apiMocks.cancelImageGeneration.mockResolvedValueOnce({
      generation_id: 101,
      work_id: 201,
      status: 'succeeded',
      stage: 'succeeded',
      prompt: 'prompt that finishes',
      preview_url: '/api/works/201/file',
      download_url: '/api/works/201/download',
      mime_type: 'image/png',
      available_credits: 7,
      credits_deducted: true
    })

    const wrapper = await mountReady()
    await submitWorkspacePrompt(wrapper, 'prompt that finishes')
    await wrapper.get('[data-testid="workspace-cancel-generation"]').trigger('click')
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(apiMocks.cancelImageGeneration).toHaveBeenCalledWith(101)
    expect(wrapper.get('.preview-image').attributes('src')).toBe('/api/works/201/file')
    expect(wrapper.text()).not.toContain('已取消生成')
  })

  it('polls concurrent generation tasks independently and switches the selected preview', async () => {
    vi.useFakeTimers()
    mockUser({ available_credits: 8 })
    apiMocks.listWorks
      .mockResolvedValueOnce({ items: [] })
      .mockResolvedValueOnce({
        items: [
          {
            work_id: 202,
            prompt: '第二张城市夜景',
            preview_url: '/api/works/202/file',
            download_url: '/api/works/202/download',
            created_at: '2026-06-06T10:02:00Z'
          }
        ]
      })
      .mockResolvedValueOnce({
        items: [
          {
            work_id: 201,
            prompt: '第一张竹林湖面',
            preview_url: '/api/works/201/file',
            download_url: '/api/works/201/download',
            created_at: '2026-06-06T10:03:00Z'
          },
          {
            work_id: 202,
            prompt: '第二张城市夜景',
            preview_url: '/api/works/202/file',
            download_url: '/api/works/202/download',
            created_at: '2026-06-06T10:02:00Z'
          }
        ]
      })
    mockReferenceAssets()
    apiMocks.createImageGeneration
      .mockResolvedValueOnce({ generation_id: 101, status: 'queued', stage: 'queued', available_credits: 7 })
      .mockResolvedValueOnce({ generation_id: 102, status: 'queued', stage: 'queued', available_credits: 6 })
    apiMocks.getImageGeneration.mockImplementation(async (id) => {
      if (id === 101) {
        return {
          generation_id: 101,
          status: 'running',
          stage: 'requesting_provider',
          prompt: '第一张竹林湖面',
          available_credits: 6
        }
      }
      return {
        generation_id: 102,
        work_id: 202,
        status: 'succeeded',
        stage: 'succeeded',
        prompt: '第二张城市夜景',
        preview_url: '/api/works/202/file',
        download_url: '/api/works/202/download',
        available_credits: 6
      }
    })

    const wrapper = await mountReady()

    await submitWorkspacePrompt(wrapper, '第一张竹林湖面')
    await submitWorkspacePrompt(wrapper, '第二张城市夜景')
    await vi.advanceTimersByTimeAsync(1000)
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(apiMocks.getImageGeneration).toHaveBeenCalledWith(101)
    expect(apiMocks.getImageGeneration).toHaveBeenCalledWith(102)
    expect(wrapper.get('[data-testid="workspace-generation-task-101"]').text()).toContain('请求模型')
    expect(wrapper.get('[data-testid="workspace-generation-task-102"]').text()).toContain('已完成')
    expect(wrapper.get('[data-testid="workspace-generation-task-102"]').classes()).toContain('active')
    expect(wrapper.get('.preview-image').attributes('src')).toBe('/api/works/202/file')

    await wrapper.get('[data-testid="workspace-generation-task-101"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="workspace-result-stage"]').text()).toContain('正在请求模型')
    expect(wrapper.find('.preview-image').exists()).toBe(false)
  })

  it('shows a single failed task and retries it without affecting other task rows', async () => {
    vi.useFakeTimers()
    mockUser({ available_credits: 8 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.createImageGeneration
      .mockResolvedValueOnce({ generation_id: 101, status: 'queued', stage: 'queued', available_credits: 7 })
      .mockResolvedValueOnce({ generation_id: 102, status: 'queued', stage: 'queued', available_credits: 6 })
      .mockResolvedValueOnce({ generation_id: 103, status: 'queued', stage: 'queued', available_credits: 5 })
    apiMocks.getImageGeneration.mockImplementation(async (id) => {
      if (id === 101) {
        return {
          generation_id: 101,
          status: 'failed',
          stage: 'failed',
          prompt: '第一张失败图',
          available_credits: 6,
          error: { message: '模型超时 traceid: task-101' }
        }
      }
      return {
        generation_id: 102,
        status: 'running',
        stage: 'requesting_provider',
        prompt: '第二张继续生成',
        available_credits: 6
      }
    })

    const wrapper = await mountReady()

    await submitWorkspacePrompt(wrapper, '第一张失败图')
    await submitWorkspacePrompt(wrapper, '第二张继续生成')
    await vi.advanceTimersByTimeAsync(1000)
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="workspace-generation-task-101"]').text()).toContain('失败')
    expect(wrapper.get('[data-testid="workspace-generation-task-102"]').text()).toContain('请求模型')

    await wrapper.get('[data-testid="workspace-generation-task-101"]').trigger('click')
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="workspace-generation-failure-notice"]').text()).toContain('网络超时，生成失败')
    expect(wrapper.get('[data-testid="workspace-result-error"]').text()).toContain('生成失败')
    expect(wrapper.text()).not.toContain('模型超时 traceid: task-101')

    await wrapper.get('[data-testid="workspace-generation-task-101-retry"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledTimes(3)
    expect(apiMocks.createImageGeneration).toHaveBeenLastCalledWith(expect.objectContaining({
      prompt: '第一张失败图',
      aspect_ratio: '1:1',
      tool_mode: 'generate'
    }))
    expect(wrapper.get('[data-testid="workspace-generation-task-102"]').text()).toContain('请求模型')
    expect(wrapper.get('[data-testid="workspace-generation-task-103"]').classes()).toContain('active')
  })

  it('shows a sanitized failure reason instead of the failed prompt in generation task rows', async () => {
    vi.useFakeTimers()
    mockUser({ available_credits: 8 })
    mockWorks()
    mockReferenceAssets()
    const unsafePrompt = 'photorealistic commercial poster with a very long unsafe prompt that should stay hidden from failed task summaries'
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 201,
      status: 'queued',
      stage: 'queued',
      prompt: unsafePrompt,
      parameters: { prompt: unsafePrompt, aspect_ratio: '1:1', tool_mode: 'generate' },
      available_credits: 7
    })
    apiMocks.getImageGeneration.mockResolvedValueOnce({
      generation_id: 201,
      status: 'failed',
      stage: 'failed',
      prompt: unsafePrompt,
      parameters: { prompt: unsafePrompt, aspect_ratio: '1:1', tool_mode: 'generate' },
      available_credits: 7,
      error: {
        code: 'provider_policy_rejected',
        message: '提交内容触发平台安全策略，请调整提示词后重试。',
        retryable: true
      }
    })

    const wrapper = await mountReady()

    await submitWorkspacePrompt(wrapper, unsafePrompt)
    await vi.advanceTimersByTimeAsync(1000)
    await flushPromises()
    await wrapper.vm.$nextTick()

    const taskRow = wrapper.get('[data-testid="workspace-generation-task-201"]')
    expect(taskRow.text()).toContain('提示词可能触发平台安全策略，请调整后重试。')
    expect(taskRow.text()).not.toContain(unsafePrompt)
    expect(taskRow.text()).toContain('失败')
    expect(wrapper.get('[data-testid="workspace-generation-task-201-retry"]').exists()).toBe(true)
  })

  it('keeps technical provider diagnostics out of failed generation task rows', async () => {
    vi.useFakeTimers()
    mockUser({ available_credits: 8 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 202,
      status: 'queued',
      stage: 'queued',
      prompt: 'technical failure prompt',
      parameters: { prompt: 'technical failure prompt', aspect_ratio: '1:1', tool_mode: 'generate' },
      available_credits: 7
    })
    apiMocks.getImageGeneration.mockResolvedValueOnce({
      generation_id: 202,
      status: 'failed',
      stage: 'failed',
      prompt: 'technical failure prompt',
      parameters: { prompt: 'technical failure prompt', aspect_ratio: '1:1', tool_mode: 'generate' },
      available_credits: 7,
      error: {
        code: 'provider_request_failed',
        message: 'POST "https://provider.example/v1/images" failed traceid: trace-unsafe-202',
        retryable: true
      }
    })

    const wrapper = await mountReady()

    await submitWorkspacePrompt(wrapper, 'technical failure prompt')
    await vi.advanceTimersByTimeAsync(1000)
    await flushPromises()
    await wrapper.vm.$nextTick()

    const taskRow = wrapper.get('[data-testid="workspace-generation-task-202"]')
    expect(taskRow.text()).toContain('图片生成失败，请稍后再试')
    expect(taskRow.text()).not.toContain('trace-unsafe-202')
    expect(taskRow.text()).not.toContain('technical failure prompt')
  })

  it('shows the generated image without cropping and opens a zoom preview on double click', async () => {
    mockUser()
    mockWorks([
      {
        work_id: 90,
        prompt: 'wide night campus panorama',
        preview_url: '/api/works/90/file',
        download_url: '/api/works/90/download',
        created_at: '2026-04-30T08:31:16Z'
      }
    ])

    const wrapper = await mountReady({
      ...mountOptions,
      attachTo: document.body
    })

    await wrapper.get('[data-testid="workspace-tab-create"]').trigger('click')
    await wrapper.vm.$nextTick()

    const previewImage = wrapper.get('.preview-image')
    expect(previewImage.attributes('src')).toBe('/api/works/90/file')
    expect(previewImage.attributes('title')).toBe('双击放大查看')

    await previewImage.trigger('dblclick')
    await wrapper.vm.$nextTick()

    const modal = document.body.querySelector('[data-testid="workspace-preview-modal"]')
    expect(modal).not.toBeNull()
    expect(modal.getAttribute('role')).toBe('dialog')
    expect(modal.getAttribute('aria-modal')).toBe('true')
    expect(modal.querySelector('[data-testid="workspace-preview-zoom-image"]').getAttribute('src')).toBe('/api/works/90/file')
    expect(modal.textContent).toContain('wide night campus panorama')
    expect(modal.querySelector('a').getAttribute('href')).toBe('/api/works/90/download')

    modal.querySelector('[data-testid="workspace-preview-close"]').click()
    await wrapper.vm.$nextTick()

    expect(document.body.querySelector('[data-testid="workspace-preview-modal"]')).toBeNull()
    wrapper.unmount()
  })

  it('does not open the zoom preview when there is no generated image', async () => {
    mockUser()
    mockWorks()

    const wrapper = await mountReady()

    await wrapper.get('.preview-container').trigger('dblclick')
    await wrapper.vm.$nextTick()

    expect(wrapper.find('.preview-image').exists()).toBe(false)
    expect(wrapper.find('[data-testid="workspace-preview-modal"]').exists()).toBe(false)
  })

  it('renders the imini-inspired unified image workspace layout', async () => {
    mockUser()
    mockWorks()
    mockReferenceAssets()

    const wrapper = await mountReady()

    expect(wrapper.get('[data-testid="workspace-composer-form"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="workspace-prompt-input"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('图片生成')
    expect(wrapper.text()).toContain('白霖通用模型')
    expect(wrapper.get('[data-testid="workspace-reference-upload"]').text()).toContain('上传图片')
    expect(wrapper.get('[data-testid="workspace-reference-upload"]').text()).toContain('JPG/PNG/WEBP，单张小于50MB')
    expect(wrapper.get('[data-testid="workspace-reference-upload"]').text()).not.toContain('上传参考图')
    expect(wrapper.text()).toContain('根据文本描述或参考图片生成图片')
    expect(wrapper.text()).toContain('精细编辑')
    expect(wrapper.get('[data-testid="workspace-auto-translate"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('AI 工具')
    expect(wrapper.text()).toContain('创作乐园')
    expect(wrapper.get('[data-testid="workspace-discovery-panel"]').text().indexOf('AI 工具')).toBeLessThan(
      wrapper.get('[data-testid="workspace-discovery-panel"]').text().indexOf('创作乐园')
    )
    expect(wrapper.find('[data-testid="workspace-tool-old_photo_restoration"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="workspace-playground-article-images"]').text()).toContain('公众号文章配图')
    expect(wrapper.get('[data-testid="workspace-playground-couple-album"]').text()).toContain('情侣相册')
    expect(wrapper.get('[data-testid="workspace-playground-childhood-dream-album"]').text()).toContain('童年梦想相册')
    expect(wrapper.text()).toContain('智能扩图')
    expect(wrapper.text()).toContain('电商工作流')
    expect(wrapper.text()).toContain('拍立得风格')
    expect(wrapper.get('[data-testid="workspace-discovery-filter-all"]').text()).toBe('全部')
    expect(wrapper.get('[data-testid="workspace-discovery-filter-image"]').text()).toBe('图片')
    expect(wrapper.get('[data-testid="workspace-discovery-filter-video"]').text()).toBe('视频')
    expect(wrapper.get('[data-testid="workspace-discovery-filter-tool"]').text()).toBe('工具')
    expect(wrapper.text()).not.toContain('生成记录（共 0）')
    expect(wrapper.find('[data-testid="workspace-size-selector"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="workspace-size-select"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('推荐输出尺寸')
    expect(wrapper.text()).toContain('1024x1024')
    await openClickSelect(wrapper, 'workspace-size-select')
    const options = Array.from(clickSelectMenu('workspace-size-select').querySelectorAll('.click-select-option'))
    expect(options.map((node) => node.dataset.testid.replace('workspace-size-select-option-', ''))).toEqual([
      '21:9',
      '16:9',
      '4:3',
      '3:2',
      '1:1',
      '2:3',
      '3:4',
      '9:16',
      '9:21'
    ])
    expect(options.map((node) => node.textContent)).toContain('21:9 超宽屏 · 横幅 / 影院感 · 推荐输出尺寸 1536x1024')
    expect(options.map((node) => node.textContent)).toContain('9:16 手机竖屏 · 短视频 / 壁纸 · 推荐输出尺寸 1024x1536')
    expect(wrapper.findAll('.ratio-preview-frame')).toHaveLength(1)
    expect(wrapper.get('[data-testid="workspace-create-button"]').text()).toContain('创建')
    expect(wrapper.get('[data-testid="workspace-create-button"]').text()).toContain('1 点')
    expect(wrapper.find('[data-testid="workspace-quality-select"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="workspace-quality-ultra"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="workspace-num-select"]').exists()).toBe(false)
  })

  it('renders workspace tool and playground cards with dedicated OSS cover images', async () => {
    mockUser()
    mockWorks()
    mockReferenceAssets()

    const wrapper = await mountReady()

    const expectedImages = {
      'workspace-tool-expand': 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/workspace-card-covers/2026-06-02/expand.png',
      'workspace-tool-erase': 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/workspace-card-covers/2026-06-02/erase.png',
      'workspace-tool-remove_background': 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/workspace-card-covers/2026-06-02/remove-background.png',
      'workspace-tool-upscale': 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/workspace-card-covers/2026-06-02/upscale.png',
      'workspace-tool-precision_edit': 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/workspace-card-covers/2026-06-02/precision-edit.png',
      'workspace-playground-couple-album': 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/workspace-card-covers/2026-06-02/couple-album.png',
      'workspace-playground-childhood-dream-album': 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/workspace-card-covers/2026-06-02/childhood-dream-album.png'
    }

    for (const [testId, expectedSrc] of Object.entries(expectedImages)) {
      expect(wrapper.get(`[data-testid="${testId}"] img`).attributes('src')).toBe(expectedSrc)
    }
  })

  it('renders inspiration recommendations before tools and applies one-click same parameters without auto generation', async () => {
    mockUser({ available_credits: 5 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 12,
      status: 'queued',
      stage: 'queued',
      available_credits: 4
    })

    const wrapper = await mountReady()
    const panel = wrapper.get('[data-testid="workspace-discovery-panel"]')
    const orderedTestIds = [...panel.element.querySelectorAll('[data-testid]')].map((node) => node.dataset.testid)

    expect(wrapper.get('[data-testid="workspace-inspiration-recommendations"]').exists()).toBe(true)
    expect(orderedTestIds.indexOf('workspace-inspiration-recommendations')).toBeLessThan(
      orderedTestIds.indexOf('workspace-tool-expand')
    )

    const card = wrapper.get('[data-testid="workspace-recommendation-501"]')
    expect(card.text()).toContain('Cyberpunk City')
    expect(card.text()).toContain('weekly-hot')
    expect(card.get('img').attributes('src')).toBe('https://oss.example.com/recommendations/cyber-city.png')
    expect(card.get('[data-testid="workspace-recommendation-use-501"]').exists()).toBe(true)

    await wrapper.get('[data-testid="workspace-recommendation-use-501"]').trigger('click')
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(apiMocks.useInspirationRecommendation).toHaveBeenCalledWith(501)
    expect(apiMocks.createImageGeneration).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="workspace-tab-create"]').classes()).toContain('active')
    expect(wrapper.get('[data-testid="workspace-prompt-input"]').element.value).toBe('cyberpunk city at rainy night')
    expect(wrapper.get('[data-testid="workspace-size-select"]').element.value).toBe('16:9')
    const advancedPanel = await openHomeAdvancedPanel(wrapper)
    expect(advancedPanel.querySelector('[data-testid="workspace-negative-prompt"]').value).toBe('low quality')
    expect(wrapper.get('[data-testid="workspace-model-select"]').attributes('value')).toBe('7')

    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: 'cyberpunk city at rainy night',
      negative_prompt: 'low quality',
      aspect_ratio: '16:9',
      style_preset: 'cinematic',
      model_id: 7,
      tool_mode: 'generate',
      tool_options: { seed: 918, guidance: 7 }
    }))
  })

  it('lets guests apply inspiration recommendations and opens auth only when they submit generation', async () => {
    apiMocks.getMe.mockRejectedValue(Object.assign(new Error('unauthorized'), { status: 401 }))

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-recommendation-use-501"]').trigger('click')
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(authModalState.open).toBe(false)
    expect(wrapper.get('[data-testid="workspace-prompt-input"]').element.value).toBe('cyberpunk city at rainy night')
    expect(apiMocks.createImageGeneration).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(authModalState.open).toBe(true)
    expect(apiMocks.createImageGeneration).not.toHaveBeenCalled()
  })

  it('renders home AI tool cards as old horizontal cards with icon copy and right media', async () => {
    mockUser()
    mockWorks()
    mockReferenceAssets()

    const wrapper = await mountReady()

    const toolRow = wrapper.get('.workshop-tool-row')
    expect(toolRow.get('.workshop-tool-head').text()).toContain('AI 工具')
    expect(toolRow.get('.workshop-tool-head').text()).toContain('发现更多可能')
    expect(toolRow.get('.workshop-section-icon').exists()).toBe(true)

    const precisionEditTool = wrapper.get('[data-testid="workspace-tool-precision_edit"]')
    expect(precisionEditTool.get('.workshop-tool-card-copy').text()).toContain('精细编辑')
    expect(precisionEditTool.get('.workshop-tool-card-copy').text()).toContain('圈选局部')
    expect(precisionEditTool.get('.workshop-tool-icon').exists()).toBe(true)
    expect(precisionEditTool.get('.workshop-tool-enter').exists()).toBe(true)
    expect(precisionEditTool.get('.workshop-tool-card-media img').attributes('src')).toContain('precision-edit.png')

    const coupleAlbum = wrapper.get('[data-testid="workspace-playground-couple-album"]')
    expect(coupleAlbum.get('.imini-playground-content').text()).toContain('情侣相册')
    expect(coupleAlbum.get('.imini-playground-content').text()).toContain('旅行故事相册')
    expect(coupleAlbum.get('.imini-card-enter').exists()).toBe(true)
    expect(coupleAlbum.get('.imini-playground-media img').attributes('src')).toContain('couple-album.png')
    expect(coupleAlbum.find('.workshop-tool-card-copy').exists()).toBe(false)

    const caseCard = wrapper.get('[data-testid="workspace-template-101"]')
    expect(caseCard.get('.imini-case-card-content').text()).toContain('拍立得风格')
    expect(caseCard.get('.imini-case-card-content').text()).toContain('将照片转换为拍立得风格')
    expect(caseCard.get('.imini-card-enter').exists()).toBe(true)
    expect(caseCard.get('.imini-case-card-media img').attributes('src')).toBe('https://oss.example.com/polaroid.png')

    expect(wrapper.find('.imini-playground-visual').exists()).toBe(false)
    expect(wrapper.find('.imini-template-card > img').exists()).toBe(false)
  })

  it('filters discovery content locally without changing backend data flow', async () => {
    mockUser()
    mockWorks()
    mockReferenceAssets()

    const wrapper = await mountReady()

    expect(wrapper.find('[data-testid="workspace-tool-expand"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="workspace-playground-article-images"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="workspace-playground-couple-album"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="workspace-playground-childhood-dream-album"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="workspace-recommendation-501"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="workspace-template-101"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="workspace-template-201"]').exists()).toBe(true)

    await wrapper.get('[data-testid="workspace-discovery-filter-tool"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.find('[data-testid="workspace-tool-expand"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="workspace-playground-article-images"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="workspace-playground-couple-album"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="workspace-playground-childhood-dream-album"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="workspace-recommendation-501"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="workspace-template-101"]').exists()).toBe(false)

    await wrapper.get('[data-testid="workspace-discovery-filter-image"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.find('[data-testid="workspace-tool-expand"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="workspace-playground-article-images"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="workspace-playground-couple-album"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="workspace-playground-childhood-dream-album"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="workspace-recommendation-501"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="workspace-template-101"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="workspace-template-201"]').exists()).toBe(true)

    await wrapper.get('[data-testid="workspace-discovery-filter-video"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.find('[data-testid="workspace-tool-expand"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="workspace-playground-article-images"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="workspace-playground-couple-album"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="workspace-playground-childhood-dream-album"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="workspace-recommendation-501"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="workspace-template-101"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="workspace-discovery-panel"]').text()).toContain('暂无视频案例')

    expect(apiMocks.getWorkspaceDiscovery).toHaveBeenCalledTimes(1)
  })

  it('opens playground album routes from discovery cards without switching to create mode', async () => {
    mockUser()
    mockWorks()
    mockReferenceAssets()

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-playground-article-images"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(routerPush).toHaveBeenCalledWith('/workspace/article-images')
    expect(wrapper.get('[data-testid="workspace-discovery-panel"]').exists()).toBe(true)

    await wrapper.get('[data-testid="workspace-playground-couple-album"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(routerPush).toHaveBeenCalledWith('/workspace/couple-album')
    expect(wrapper.get('[data-testid="workspace-discovery-panel"]').exists()).toBe(true)

    await wrapper.get('[data-testid="workspace-playground-childhood-dream-album"]').trigger('click')

    expect(routerPush).toHaveBeenCalledWith('/workspace/childhood-dream-album')
  })

  it('does not expose old photo restoration in the generic AI tool grid', async () => {
    mockUser()
    mockWorks()
    mockReferenceAssets()

    const wrapper = await mountReady()

    expect(wrapper.find('[data-testid="workspace-tool-old_photo_restoration"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="workspace-discovery-panel"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="workspace-create-panel"]').exists()).toBe(false)
  })

  it('filters unsupported backend tools out of the AI tool grid for guests too', async () => {
    apiMocks.getMe.mockRejectedValue(Object.assign(new Error('unauthorized'), { status: 401 }))
    mockDiscovery({
      tools: [
        {
          mode: 'old_photo_restoration',
          title: '老照片修复',
          description: '修复划痕褪色',
          icon: 'refresh',
          enabled: true
        }
      ]
    })

    const wrapper = await mountReady()

    expect(wrapper.find('[data-testid="workspace-tool-old_photo_restoration"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="workspace-tool-expand"]').exists()).toBe(true)
  })

  it('estimates credits from backend response and omits num before submitting', async () => {
    vi.useFakeTimers()
    mockUser({ available_credits: 80 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.estimateImageGeneration.mockResolvedValue({
      required_credits: 1,
      available_credits: 80,
      missing_credits: 0,
      enough: true
    })
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 12,
      status: 'queued',
      stage: 'queued',
      available_credits: 31
    })
    apiMocks.uploadReferenceAsset.mockResolvedValueOnce({
      id: 42,
      preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/ref.png',
      original_filename: 'ref.png'
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tool-upscale"]').trigger('click')
    await uploadWorkspaceReference(wrapper, new File(['fake'], 'ref.png', { type: 'image/png' }))
    await flushPromises()
    await chooseClickSelect(wrapper, 'workspace-quality-select', 'ultra')
    await wrapper.get('[data-testid="workspace-tool-option-scale"]').setValue('4x')
    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('提升商品主图清晰度')
    await advanceCreditEstimateDebounce()

    const estimatePayload = apiMocks.estimateImageGeneration.mock.calls.at(-1)[0]
    expect(estimatePayload).toEqual(expect.objectContaining({
      prompt: '提升商品主图清晰度',
      model_id: 7,
      quality: 'ultra',
      tool_mode: 'upscale',
      tool_options: { scale: '4x' },
      reference_asset_ids: [42]
    }))
    expect(estimatePayload).not.toHaveProperty('num')
    expect(wrapper.get('[data-testid="workspace-create-button"]').text()).toContain('1 点')

    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    const createPayload = apiMocks.createImageGeneration.mock.calls.at(-1)[0]
    expect(createPayload).toEqual(expect.objectContaining({
      prompt: '提升商品主图清晰度',
      model_id: 7,
      quality: 'ultra',
      tool_mode: 'upscale',
      tool_options: { scale: '4x' },
      reference_asset_ids: [42]
    }))
    expect(createPayload).not.toHaveProperty('num')
  })

  it('requires exactly one source image for upscale before creating', async () => {
    mockUser({ available_credits: 5 })
    mockWorks()
    mockReferenceAssets()

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tool-upscale"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="workspace-tab-create"]').classes()).toContain('active')
    expect(wrapper.get('[data-testid="workspace-reference-upload"]').text()).toContain('上传需要高清放大的图片')
    expect(wrapper.get('[data-testid="workspace-prompt-input"]').attributes('placeholder')).toContain('保持主体、颜色、构图和内容不变')
    expect(wrapper.get('[data-testid="workspace-create-button"]').attributes('disabled')).toBeDefined()

    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('请先上传图片或选择作品作为编辑来源')
    expect(apiMocks.createImageGeneration).not.toHaveBeenCalled()
  })

  it('submits upscale with one source image and the default enhancement prompt when notes are empty', async () => {
    mockUser({ available_credits: 10 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 13,
      status: 'queued',
      stage: 'queued',
      available_credits: 8
    })
    apiMocks.uploadReferenceAsset.mockResolvedValueOnce({
      id: 42,
      preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/upscale.png',
      original_filename: 'upscale.png'
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tool-upscale"]').trigger('click')
    await uploadWorkspaceReference(wrapper, new File(['fake'], 'upscale.png', { type: 'image/png' }))
    await flushPromises()
    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: '提升图片清晰度、纹理细节和边缘质量，保持主体、颜色、构图和内容不变',
      model_id: 7,
      tool_mode: 'upscale',
      tool_options: { scale: '2x' },
      reference_asset_ids: [42]
    }))
    expect(apiMocks.createImageGeneration.mock.calls[0][0].edit_instruction).toBeUndefined()
  })

  it('sends upscale edit instruction and selected scale while keeping a single source image', async () => {
    vi.useFakeTimers()
    mockUser({ available_credits: 10 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 14,
      status: 'queued',
      stage: 'queued',
      available_credits: 8
    })
    apiMocks.uploadReferenceAsset
      .mockResolvedValueOnce({
        id: 42,
        preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/first.png',
        original_filename: 'first.png'
      })
      .mockResolvedValueOnce({
        id: 43,
        preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/second.png',
        original_filename: 'second.png'
      })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tool-upscale"]').trigger('click')
    await uploadWorkspaceReference(wrapper, new File(['first'], 'first.png', { type: 'image/png' }))
    await flushPromises()
    await uploadWorkspaceReference(wrapper, new File(['second'], 'second.png', { type: 'image/png' }))
    await flushPromises()
    await wrapper.get('[data-testid="workspace-tool-option-scale"]').setValue('8x')
    await wrapper.get('[data-testid="workspace-tool-option-edit_instruction"]').setValue('增强发丝、织物纹理和边缘锐度')
    await advanceCreditEstimateDebounce()

    expect(apiMocks.estimateImageGeneration).toHaveBeenLastCalledWith(
      expect.objectContaining({
        prompt: expect.stringContaining('增强发丝、织物纹理和边缘锐度'),
        tool_mode: 'upscale',
        tool_options: { scale: '8x' },
        edit_instruction: '增强发丝、织物纹理和边缘锐度',
        reference_asset_ids: [43]
      }),
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )

    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: expect.stringContaining('增强发丝、织物纹理和边缘锐度'),
      edit_instruction: '增强发丝、织物纹理和边缘锐度',
      tool_mode: 'upscale',
      tool_options: { scale: '8x' },
      reference_asset_ids: [43]
    }))
    expect(apiMocks.createImageGeneration.mock.calls[0][0].reference_asset_ids).toHaveLength(1)
  })

  it('switches between discovery creation and Agent planning views', async () => {
    mockUser()
    mockWorks()
    mockReferenceAssets()

    const wrapper = await mountReady()

    expect(wrapper.get('[data-testid="workspace-discovery-panel"]').text()).toContain('AI 工具')
    expect(wrapper.find('[data-testid="workspace-create-panel"]').exists()).toBe(false)

    await wrapper.get('[data-testid="workspace-tab-create"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="workspace-create-panel"]').text()).toContain('生成记录（共 0）')
    expect(wrapper.find('[data-testid="workspace-discovery-panel"]').exists()).toBe(false)

    await wrapper.get('[data-testid="workspace-tab-discovery"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="workspace-agent-panel"]').text()).toContain('创作任务代理')
    expect(wrapper.find('[data-testid="workspace-discovery-panel"]').exists()).toBe(false)
  })

  it('opens Agent mode as a planning workspace instead of showing an upcoming notice', async () => {
    mockUser()
    mockWorks()
    mockReferenceAssets()

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-mode-agent"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="workspace-agent-panel"]').text()).toContain('创作任务代理')
    expect(wrapper.get('[data-testid="workspace-mode-agent"]').classes()).toContain('active')
    expect(wrapper.find('[data-testid="workspace-discovery-panel"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('Agent模式即将开放')
  })

  it('keeps Agent recent works on the latest first-page cache after browsing history page 2', async () => {
    mockUser()
    const latestWorks = Array.from({ length: 18 }, (_, index) => makeWork(300 - index, {
      prompt: index === 0 ? '首页最新作品' : `首页作品 ${index}`
    }))
    const olderWorks = [makeWork(20, { prompt: '第二页旧作品' })]
    apiMocks.listWorks
      .mockResolvedValueOnce({
        items: latestWorks,
        page: 1,
        page_size: 18,
        total: 19
      })
      .mockResolvedValueOnce({
        items: olderWorks,
        page: 2,
        page_size: 18,
        total: 19
      })
    mockReferenceAssets()

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tab-create"]').trigger('click')
    await wrapper.vm.$nextTick()
    await wrapper.get('[data-testid="workspace-works-next"]').trigger('click')
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="workspace-create-panel"]').text()).toContain('第二页旧作品')

    await wrapper.get('[data-testid="workspace-mode-agent"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="agent-task-input-panel"]').text()).toContain('首页最新作品')
    expect(wrapper.get('[data-testid="agent-task-input-panel"]').text()).not.toContain('第二页旧作品')
  })

  it('renders Agent mode as a compact three-step task workbench', async () => {
    mockUser()
    mockWorks([
      {
        work_id: 71,
        prompt: '可复用的商品图',
        preview_url: '/api/works/71/file',
        created_at: '2026-06-06T10:00:00Z'
      }
    ])
    mockReferenceAssets()

    const wrapper = await mountReady()
    await wrapper.get('[data-testid="workspace-mode-agent"]').trigger('click')
    await wrapper.vm.$nextTick()

    const panel = wrapper.get('[data-testid="workspace-agent-panel"]')
    expect(panel.get('[data-testid="agent-step-describe"]').classes()).toContain('active')
    expect(panel.text()).toContain('描述需求')
    expect(panel.text()).toContain('确认方案')
    expect(panel.text()).toContain('生成结果')
    expect(panel.get('[data-testid="agent-task-input-panel"]').text()).toContain('任务输入')
    expect(panel.get('[data-testid="agent-chat-input"]').exists()).toBe(true)
    expect(panel.get('[data-testid="agent-upload-reference"]').text()).toContain('上传参考图')
    expect(panel.get('[data-testid="agent-work-reference-71"]').text()).toContain('可复用的商品图')
    expect(panel.get('[data-testid="agent-plan-empty"]').text()).toContain('描述任务/加参考')
    expect(panel.get('[data-testid="agent-execution-status"]').text()).toContain('等待方案')
  })

  it('shows Agent clarification prompts and blocks generation until the user supplements the task', async () => {
    mockUser()
    mockWorks()
    mockReferenceAssets()
    apiMocks.planImageAgent.mockResolvedValueOnce({
      reply: '还需要确认用途。',
      needs_clarification: true,
      clarification_prompt: '这张图主要用于电商主图还是社媒海报？',
      plan: null,
      candidates: []
    })

    const wrapper = await mountReady()
    await wrapper.get('[data-testid="workspace-mode-agent"]').trigger('click')
    await wrapper.get('[data-testid="agent-chat-input"]').setValue('帮我做一张香薰图')
    await wrapper.get('[data-testid="agent-chat-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(wrapper.get('[data-testid="agent-clarification"]').text()).toContain('这张图主要用于电商主图还是社媒海报？')
    expect(wrapper.get('[data-testid="agent-confirm-generate"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="agent-execution-reason"]').text()).toContain('需要先补充需求')
    expect(apiMocks.estimateImageGeneration).not.toHaveBeenCalled()
    expect(apiMocks.createImageGeneration).not.toHaveBeenCalled()
  })

  it('plans an Agent image task and renders editable candidate parameters', async () => {
    mockUser()
    mockWorks()
    mockReferenceAssets()
    apiMocks.planImageAgent.mockResolvedValueOnce({
      reply: '我整理了 2 个方向，先用电商主图。',
      needs_clarification: false,
      plan: {
        title: '玻璃香薰电商主图',
        intent: 'text_to_image',
        tool_mode: 'generate',
        prompt: '透明玻璃香薰瓶居中，浅色背景，商业摄影布光',
        negative_prompt: '文字，水印',
        aspect_ratio: '1:1',
        style_preset: '电商',
        quality: 'high',
        reference_weight: 75,
        tool_options: {},
        requires_confirmation: true
      },
      candidates: [
        {
          id: 'commerce-main',
          title: '电商主图',
          prompt: '透明玻璃香薰瓶居中，浅色背景，商业摄影布光',
          aspect_ratio: '1:1',
          style_preset: '电商',
          quality: 'high'
        },
        {
          id: 'poster-kv',
          title: '海报 KV',
          prompt: '透明玻璃香薰瓶与光影背景，海报构图',
          aspect_ratio: '3:4',
          style_preset: '海报',
          quality: 'medium'
        }
      ],
      safety_notes: ['已规避品牌标识']
    })

    const wrapper = await mountReady()
    await wrapper.get('[data-testid="workspace-mode-agent"]').trigger('click')
    await wrapper.get('[data-testid="agent-chat-input"]').setValue('帮我做一张香薰电商主图')
    await wrapper.get('[data-testid="agent-chat-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.planImageAgent).toHaveBeenCalledWith(expect.objectContaining({
      message: '帮我做一张香薰电商主图',
      reference_asset_ids: [],
      reference_work_ids: [],
      current_plan: null
    }))
    expect(wrapper.get('[data-testid="agent-plan-title"]').text()).toContain('玻璃香薰电商主图')
    expect(wrapper.get('[data-testid="agent-plan-prompt"]').element.value).toContain('透明玻璃香薰瓶')
    expect(wrapper.get('[data-testid="agent-candidate-commerce-main"]').classes()).toContain('active')
    expect(wrapper.get('[data-testid="agent-candidate-poster-kv"]').text()).toContain('海报 KV')

    await wrapper.get('[data-testid="agent-candidate-poster-kv"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="agent-plan-title"]').text()).toContain('海报 KV')
    expect(wrapper.get('[data-testid="agent-plan-aspect-ratio"]').element.value).toBe('3:4')
  })

  it('estimates the edited Agent plan and confirms generation through the existing image API', async () => {
    mockUser({ available_credits: 8 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.planImageAgent.mockResolvedValueOnce({
      reply: '已生成方案，请确认后执行。',
      needs_clarification: false,
      plan: {
        title: '社媒海报',
        intent: 'text_to_image',
        tool_mode: 'generate',
        prompt: '咖啡杯海报，暖色自然光',
        aspect_ratio: '1:1',
        style_preset: '海报',
        quality: 'medium',
        reference_weight: 75,
        tool_options: {},
        requires_confirmation: true
      },
      candidates: []
    })
    apiMocks.estimateImageGeneration.mockResolvedValueOnce({
      required_credits: 2,
      available_credits: 8,
      missing_credits: 0,
      enough: true
    })
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 44,
      status: 'queued',
      stage: 'queued',
      available_credits: 6
    })

    const wrapper = await mountReady()
    await wrapper.get('[data-testid="workspace-mode-agent"]').trigger('click')
    await wrapper.get('[data-testid="agent-chat-input"]').setValue('做一张咖啡社媒图')
    await wrapper.get('[data-testid="agent-chat-form"]').trigger('submit.prevent')
    await flushPromises()

    await wrapper.get('[data-testid="agent-plan-prompt"]').setValue('咖啡杯社媒海报，暖色自然光，桌面有低糖甜点')
    await wrapper.get('[data-testid="agent-plan-aspect-ratio"]').setValue('3:4')
    await wrapper.get('[data-testid="agent-plan-quality"]').setValue('high')
    await wrapper.get('[data-testid="agent-estimate-button"]').trigger('click')
    await flushPromises()

    expect(apiMocks.estimateImageGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: '咖啡杯社媒海报，暖色自然光，桌面有低糖甜点',
      aspect_ratio: '3:4',
      tool_mode: 'generate',
      style_preset: '海报',
      quality: 'high'
    }))
    expect(wrapper.get('[data-testid="agent-credit-estimate"]').text()).toContain('预计 2 点')

    await wrapper.get('[data-testid="agent-confirm-generate"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: '咖啡杯社媒海报，暖色自然光，桌面有低糖甜点',
      aspect_ratio: '3:4',
      tool_mode: 'generate',
      style_preset: '海报',
      quality: 'high'
    }))
    expect(wrapper.get('[data-testid="agent-execution-status"]').text()).toContain('排队中')
  })

  it('shows a retryable Agent planning failure without polluting chat history and reuses the original payload', async () => {
    mockUser({ available_credits: 8 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.uploadReferenceAsset.mockResolvedValueOnce({
      id: 42,
      preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/ref.png',
      original_filename: 'ref.png'
    })
    apiMocks.planImageAgent
      .mockRejectedValueOnce(new ApiError('network_unreachable', '当前网络已断开，请检查网络后重试', 0, { retryable: true }))
      .mockResolvedValueOnce({
        reply: '已重新生成方案，请确认。',
        needs_clarification: false,
        plan: {
          title: '香薰商品主图',
          intent: 'text_to_image',
          tool_mode: 'generate',
          prompt: '透明香薰瓶居中，浅色背景，商业摄影布光',
          aspect_ratio: '1:1',
          style_preset: '电商',
          quality: 'medium',
          reference_weight: 75,
          tool_options: {},
          requires_confirmation: true
        },
        candidates: []
      })

    const wrapper = await mountReady()
    await uploadWorkspaceReference(wrapper, new File(['fake'], 'ref.png', { type: 'image/png' }))
    await flushPromises()
    await wrapper.get('[data-testid="workspace-mode-agent"]').trigger('click')
    await wrapper.get('[data-testid="agent-chat-input"]').setValue('帮我做一张香薰电商主图')
    await wrapper.get('[data-testid="agent-chat-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(wrapper.get('[data-testid="agent-plan-failure"]').text()).toContain('网络不稳定')
    expect(wrapper.get('[data-testid="agent-plan-failure"]').text()).toContain('检查连接后重试')
    expect(wrapper.get('[data-testid="agent-retry-plan"]').text()).toContain('重新生成方案')
    expect(wrapper.get('[data-testid="agent-message-list"]').text()).not.toContain('请求失败')
    expect(wrapper.findAll('.agent-message.is-user')).toHaveLength(1)
    expect(apiMocks.planImageAgent).toHaveBeenCalledWith(expect.objectContaining({
      message: '帮我做一张香薰电商主图',
      reference_asset_ids: [42]
    }))
    const firstPayload = apiMocks.planImageAgent.mock.calls[0][0]

    await wrapper.get('[data-testid="agent-retry-plan"]').trigger('click')
    await flushPromises()

    expect(apiMocks.planImageAgent).toHaveBeenCalledTimes(2)
    expect(apiMocks.planImageAgent.mock.calls[1][0]).toEqual(firstPayload)
    expect(wrapper.findAll('.agent-message.is-user')).toHaveLength(1)
    expect(wrapper.find('[data-testid="agent-plan-failure"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="agent-plan-title"]').text()).toContain('香薰商品主图')
  })

  it('classifies Agent planning service failures as busy and guides retry later', async () => {
    mockUser({ available_credits: 8 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.planImageAgent.mockRejectedValueOnce(
      new ApiError('agent_image_plan_failed', '规划服务暂时不可用', 503, { retryable: true })
    )

    const wrapper = await mountReady()
    await wrapper.get('[data-testid="workspace-mode-agent"]').trigger('click')
    await wrapper.get('[data-testid="agent-chat-input"]').setValue('帮我做一张咖啡海报')
    await wrapper.get('[data-testid="agent-chat-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(wrapper.get('[data-testid="agent-plan-failure"]').text()).toContain('规划服务繁忙')
    expect(wrapper.get('[data-testid="agent-plan-failure"]').text()).toContain('稍后重试')
    expect(wrapper.get('[data-testid="agent-retry-plan"]').text()).toContain('重新生成方案')
  })

  it('shows Agent generation submission failures in the execution panel and retries the saved request body', async () => {
    mockUser({ available_credits: 8 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.planImageAgent.mockResolvedValueOnce({
      reply: '已生成方案，请确认后执行。',
      needs_clarification: false,
      plan: {
        title: '咖啡社媒海报',
        intent: 'text_to_image',
        tool_mode: 'generate',
        prompt: '咖啡杯社媒海报，暖色自然光',
        aspect_ratio: '1:1',
        style_preset: '海报',
        quality: 'medium',
        reference_weight: 75,
        tool_options: {},
        requires_confirmation: true
      },
      candidates: []
    })
    apiMocks.estimateImageGeneration.mockResolvedValueOnce({
      required_credits: 2,
      available_credits: 8,
      missing_credits: 0,
      enough: true
    })
    apiMocks.createImageGeneration
      .mockRejectedValueOnce(new ApiError('request_failed', '生成服务暂时不可用', 503, { retryable: true }))
      .mockResolvedValueOnce({
        generation_id: 45,
        status: 'queued',
        stage: 'queued',
        available_credits: 6
      })

    const wrapper = await mountReady()
    await wrapper.get('[data-testid="workspace-mode-agent"]').trigger('click')
    await wrapper.get('[data-testid="agent-chat-input"]').setValue('做一张咖啡社媒图')
    await wrapper.get('[data-testid="agent-chat-form"]').trigger('submit.prevent')
    await flushPromises()
    await wrapper.get('[data-testid="agent-estimate-button"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="agent-confirm-generate"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="agent-generation-failure"]').text()).toContain('生成提交失败')
    expect(wrapper.get('[data-testid="agent-generation-failure"]').text()).toContain('系统暂时异常')
    expect(wrapper.get('[data-testid="agent-retry-generate"]').text()).toContain('重新生成')
    const firstPayload = apiMocks.createImageGeneration.mock.calls[0][0]

    await wrapper.get('[data-testid="agent-retry-generate"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledTimes(2)
    expect(apiMocks.createImageGeneration.mock.calls[1][0]).toEqual(firstPayload)
    expect(wrapper.find('[data-testid="agent-generation-failure"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="agent-execution-status"]').text()).toContain('排队中')
  })

  it('guides users to replace incompatible Agent references when planning rejects reference assets', async () => {
    mockUser({ available_credits: 8 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.planImageAgent.mockRejectedValueOnce(
      new ApiError('invalid_reference_asset_type', '参考素材格式不支持', 400, { retryable: false })
    )

    const wrapper = await mountReady()
    await wrapper.get('[data-testid="workspace-mode-agent"]').trigger('click')
    await wrapper.get('[data-testid="agent-chat-input"]').setValue('参考这张图做一张商品图')
    await wrapper.get('[data-testid="agent-chat-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(wrapper.get('[data-testid="agent-plan-failure"]').text()).toContain('参考素材不可用')
    expect(wrapper.get('[data-testid="agent-plan-failure"]').text()).toContain('重新上传或减少素材')
    expect(wrapper.get('[data-testid="agent-retry-plan"]').attributes('disabled')).toBeDefined()
  })

  it('automatically refreshes Agent credit estimates after plan edits and generates only with the latest estimated payload', async () => {
    vi.useFakeTimers()
    mockUser({ available_credits: 8 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.planImageAgent.mockResolvedValueOnce({
      reply: '已生成方案，请确认后执行。',
      needs_clarification: false,
      plan: {
        title: '社媒海报',
        intent: 'text_to_image',
        tool_mode: 'generate',
        prompt: '咖啡杯海报，暖色自然光',
        aspect_ratio: '1:1',
        style_preset: '海报',
        quality: 'medium',
        reference_weight: 75,
        tool_options: {},
        requires_confirmation: true
      },
      candidates: []
    })
    apiMocks.estimateImageGeneration.mockResolvedValue({
      required_credits: 2,
      available_credits: 8,
      missing_credits: 0,
      enough: true
    })
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 45,
      status: 'queued',
      stage: 'queued',
      available_credits: 6
    })

    const wrapper = await mountReady()
    await wrapper.get('[data-testid="workspace-mode-agent"]').trigger('click')
    await wrapper.get('[data-testid="agent-chat-input"]').setValue('做一张咖啡社媒图')
    await wrapper.get('[data-testid="agent-chat-form"]').trigger('submit.prevent')
    await flushPromises()

    await vi.advanceTimersByTimeAsync(350)
    await flushPromises()
    expect(apiMocks.estimateImageGeneration).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[data-testid="agent-confirm-generate"]').text()).toContain('确认生成 · 预计 2 点')

    await wrapper.get('[data-testid="agent-plan-prompt"]').setValue('咖啡杯社媒海报，暖色自然光，桌面有低糖甜点')
    await wrapper.get('[data-testid="agent-plan-aspect-ratio"]').setValue('3:4')
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="agent-confirm-generate"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="agent-execution-reason"]').text()).toContain('正在预估最新点数')

    await vi.advanceTimersByTimeAsync(350)
    await flushPromises()

    expect(apiMocks.estimateImageGeneration).toHaveBeenCalledTimes(2)
    expect(apiMocks.estimateImageGeneration).toHaveBeenLastCalledWith(expect.objectContaining({
      prompt: '咖啡杯社媒海报，暖色自然光，桌面有低糖甜点',
      aspect_ratio: '3:4',
      tool_mode: 'generate',
      style_preset: '海报'
    }))

    await wrapper.get('[data-testid="agent-confirm-generate"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: '咖啡杯社媒海报，暖色自然光，桌面有低糖甜点',
      aspect_ratio: '3:4',
      tool_mode: 'generate',
      style_preset: '海报'
    }))
  })

  it('shows selected Agent work references, deduplicates repeated picks and allows removing them', async () => {
    mockUser({ available_credits: 5 })
    mockWorks([
      {
        work_id: 88,
        prompt: '玻璃瓶商品图',
        preview_url: '/api/works/88/file',
        created_at: '2026-06-06T10:00:00Z'
      }
    ])
    mockReferenceAssets()

    const wrapper = await mountReady()
    await wrapper.get('[data-testid="workspace-mode-agent"]').trigger('click')
    await wrapper.get('[data-testid="agent-work-reference-88"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="agent-work-reference-88"]').classes()).toContain('active')
    expect(wrapper.get('[data-testid="agent-work-reference-88"]').text()).toContain('已引用')
    expect(wrapper.findAll('[data-testid^="agent-reference-item-"]')).toHaveLength(1)

    await wrapper.get('[data-testid="agent-work-reference-88"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.findAll('[data-testid^="agent-reference-item-"]')).toHaveLength(1)

    await wrapper.get('[data-testid="agent-reference-item-work-88"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.findAll('[data-testid^="agent-reference-item-"]')).toHaveLength(0)
    expect(wrapper.get('[data-testid="agent-work-reference-88"]').classes()).not.toContain('active')
  })

  it('selects an AI edit tool and requires an image source before creating', async () => {
    mockUser({ available_credits: 5 })
    mockWorks()
    mockReferenceAssets()

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tool-remove_background"]').trigger('click')
    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('保留主体，移除背景')
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="workspace-tab-create"]').classes()).toContain('active')
    expect(wrapper.get('[data-testid="workspace-create-button"]').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('请先上传图片或选择作品作为编辑来源')
    expect(apiMocks.createImageGeneration).not.toHaveBeenCalled()
  })

  it('selects erase and requires an image source before creating', async () => {
    mockUser({ available_credits: 5 })
    mockWorks()
    mockReferenceAssets()
    mockDiscovery({
      tools: [
        {
          mode: 'erase',
          title: '移除物体',
          description: '清理图中干扰元素',
          icon: 'eraser',
          enabled: true,
          requires_source: true,
          source_limit: 1,
          form_schema: [
            { key: 'edit_instruction', label: '移除说明', type: 'textarea' },
            { key: 'mask', label: '蒙版', type: 'mask' }
          ]
        }
      ]
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tool-erase"]').trigger('click')
    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('移除画面左侧路人')
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="workspace-create-button"]').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('请先上传图片或选择作品作为编辑来源')
    expect(wrapper.get('[data-testid="workspace-reference-upload"]').text()).toContain('上传需要清理的图片')
    expect(wrapper.get('[data-testid="workspace-tool-option-edit_instruction"]').element.closest('.imini-tool-option')?.classList.contains('imini-tool-option--wide')).toBe(true)
    expect(apiMocks.createImageGeneration).not.toHaveBeenCalled()
  })

  it('submits erase text instruction with a single uploaded source image', async () => {
    mockUser({ available_credits: 5 })
    mockWorks()
    mockReferenceAssets()
    mockDiscovery({
      tools: [
        {
          mode: 'erase',
          title: '移除物体',
          description: '清理图中干扰元素',
          icon: 'eraser',
          enabled: true,
          requires_source: true,
          source_limit: 1,
          form_schema: [
            { key: 'edit_instruction', label: '移除说明', type: 'textarea' },
            { key: 'mask', label: '蒙版', type: 'mask' }
          ]
        }
      ]
    })
    apiMocks.uploadReferenceAsset.mockResolvedValueOnce({
      id: 42,
      preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/erase-source.png',
      original_filename: 'erase-source.png'
    })
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 21,
      status: 'queued',
      stage: 'queued',
      available_credits: 4
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tool-erase"]').trigger('click')
    await uploadWorkspaceReference(wrapper, new File(['source'], 'erase-source.png', { type: 'image/png' }))
    await flushPromises()
    await wrapper.get('[data-testid="workspace-tool-option-edit_instruction"]').setValue('移除画面左侧路人')
    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('移除画面左侧路人')
    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: '移除画面左侧路人',
      tool_mode: 'erase',
      edit_instruction: '移除画面左侧路人',
      reference_asset_ids: [42],
      reference_weight: 75
    }))
  })

  it('uploads an erase mask before submitting selected mask regions', async () => {
    const canvasContext = {
      clearRect: vi.fn(),
      fillRect: vi.fn(),
      beginPath: vi.fn(),
      moveTo: vi.fn(),
      lineTo: vi.fn(),
      stroke: vi.fn(),
      lineCap: '',
      lineJoin: '',
      lineWidth: 0,
      strokeStyle: '',
      fillStyle: ''
    }
    const getContextSpy = vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(canvasContext)
    const toBlobSpy = vi.spyOn(HTMLCanvasElement.prototype, 'toBlob').mockImplementation((callback) => {
      callback(new Blob(['mask'], { type: 'image/png' }))
    })
    mockUser({ available_credits: 5 })
    mockWorks()
    mockReferenceAssets()
    mockDiscovery({
      tools: [
        {
          mode: 'erase',
          title: '移除物体',
          description: '清理图中干扰元素',
          icon: 'eraser',
          enabled: true,
          requires_source: true,
          source_limit: 1,
          form_schema: [
            { key: 'edit_instruction', label: '移除说明', type: 'textarea' },
            { key: 'mask', label: '蒙版', type: 'mask' }
          ]
        }
      ]
    })
    apiMocks.uploadReferenceAsset
      .mockResolvedValueOnce({
        id: 42,
        preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/erase-source.png',
        original_filename: 'erase-source.png'
      })
      .mockResolvedValueOnce({
        id: 77,
        preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/erase-mask.png',
        original_filename: 'erase-mask.png'
      })
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 22,
      status: 'queued',
      stage: 'queued',
      available_credits: 4
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tool-erase"]').trigger('click')
    await uploadWorkspaceReference(wrapper, new File(['source'], 'erase-source.png', { type: 'image/png' }))
    await flushPromises()
    const canvas = wrapper.get('[data-testid="workspace-erase-mask-canvas"]')
    canvas.element.getBoundingClientRect = () => ({
      left: 0,
      top: 0,
      right: 100,
      bottom: 100,
      width: 100,
      height: 100
    })
    await canvas.trigger('pointerdown', { clientX: 10, clientY: 20 })
    await canvas.trigger('pointermove', { clientX: 40, clientY: 60 })
    await canvas.trigger('pointerup', { clientX: 40, clientY: 60 })
    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('移除圈选区域中的杂物')
    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.uploadReferenceAsset).toHaveBeenCalledTimes(2)
    expect(apiMocks.uploadReferenceAsset).toHaveBeenNthCalledWith(2, expect.any(File))
    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: '移除圈选区域中的杂物',
      tool_mode: 'erase',
      reference_asset_ids: [42],
      mask_asset_id: 77,
      tool_options: {
        mask_regions: [
          expect.objectContaining({
            x: expect.any(Number),
            y: expect.any(Number),
            width: expect.any(Number),
            height: expect.any(Number)
          })
        ]
      }
    }))

    getContextSpy.mockRestore()
    toBlobSpy.mockRestore()
  })

  it('clears erase mask regions before submitting', async () => {
    const canvasContext = {
      clearRect: vi.fn(),
      fillRect: vi.fn(),
      beginPath: vi.fn(),
      moveTo: vi.fn(),
      lineTo: vi.fn(),
      stroke: vi.fn(),
      lineCap: '',
      lineJoin: '',
      lineWidth: 0,
      strokeStyle: '',
      fillStyle: ''
    }
    const getContextSpy = vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(canvasContext)
    mockUser({ available_credits: 5 })
    mockWorks()
    mockReferenceAssets()
    mockDiscovery({
      tools: [
        {
          mode: 'erase',
          title: '移除物体',
          description: '清理图中干扰元素',
          icon: 'eraser',
          enabled: true,
          requires_source: true,
          source_limit: 1,
          form_schema: [
            { key: 'edit_instruction', label: '移除说明', type: 'textarea' },
            { key: 'mask', label: '蒙版', type: 'mask' }
          ]
        }
      ]
    })
    apiMocks.uploadReferenceAsset.mockResolvedValueOnce({
      id: 42,
      preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/erase-source.png',
      original_filename: 'erase-source.png'
    })
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 23,
      status: 'queued',
      stage: 'queued',
      available_credits: 4
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tool-erase"]').trigger('click')
    await uploadWorkspaceReference(wrapper, new File(['source'], 'erase-source.png', { type: 'image/png' }))
    await flushPromises()
    const canvas = wrapper.get('[data-testid="workspace-erase-mask-canvas"]')
    canvas.element.getBoundingClientRect = () => ({
      left: 0,
      top: 0,
      right: 100,
      bottom: 100,
      width: 100,
      height: 100
    })
    await canvas.trigger('pointerdown', { clientX: 10, clientY: 20 })
    await canvas.trigger('pointermove', { clientX: 40, clientY: 60 })
    await canvas.trigger('pointerup', { clientX: 40, clientY: 60 })
    await wrapper.get('[data-testid="workspace-erase-mask-clear"]').trigger('click')
    await wrapper.get('[data-testid="workspace-tool-option-edit_instruction"]').setValue('移除画面左侧路人')
    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('移除画面左侧路人')
    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.uploadReferenceAsset).toHaveBeenCalledTimes(1)
    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith(expect.not.objectContaining({
      mask_asset_id: expect.anything()
    }))
    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith(expect.not.objectContaining({
      tool_options: expect.objectContaining({
        mask_regions: expect.anything()
      })
    }))

    getContextSpy.mockRestore()
  })

  it('selects precision edit and requires one source, instruction and selected region', async () => {
    const drawing = mockCanvasDrawing()
    mockUser({ available_credits: 5 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.uploadReferenceAsset.mockResolvedValueOnce({
      id: 42,
      preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/precision-source.png',
      original_filename: 'precision-source.png'
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tool-precision_edit"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="workspace-tab-create"]').classes()).toContain('active')
    expect(wrapper.get('[data-testid="workspace-reference-upload"]').text()).toContain('上传需要精细编辑的图片')
    expect(wrapper.get('[data-testid="workspace-reference-upload"]').text()).toContain('精细编辑仅使用 1 张源图')
    expect(wrapper.get('[data-testid="workspace-reference-upload"]').text()).toContain('单张小于50MB')
    expect(wrapper.get('[data-testid="workspace-prompt-input"]').attributes('placeholder')).toContain('局部编辑指令')
    expect(wrapper.find('[data-testid="workspace-tool-option-edit_instruction"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="workspace-create-button"]').attributes('disabled')).toBeDefined()

    await uploadWorkspaceReference(wrapper, new File(['source'], 'precision-source.png', { type: 'image/png' }))
    await flushPromises()
    expect(wrapper.get('[data-testid="workspace-reference-count"]').text()).toContain('已选 1/1')
    expect(wrapper.get('[data-testid="workspace-precision-mask-panel"]').text()).toContain('画笔')
    expect(wrapper.get('[data-testid="workspace-precision-mask-panel"]').text()).toContain('套索')
    expect(wrapper.get('[data-testid="workspace-create-button"]').attributes('disabled')).toBeDefined()

    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('把圈选区域改成红色礼盒')
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="workspace-create-button"]').attributes('disabled')).toBeDefined()

    await drawMaskStroke(wrapper.get('[data-testid="workspace-precision-mask-canvas"]'), [[10, 20], [40, 60]])
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="workspace-create-button"]').attributes('disabled')).toBeUndefined()

    drawing.restore()
  })

  it('uploads a precision edit brush mask before submitting local edit payload', async () => {
    const drawing = mockCanvasDrawing()
    mockUser({ available_credits: 5 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.uploadReferenceAsset
      .mockResolvedValueOnce({
        id: 42,
        preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/precision-source.png',
        original_filename: 'precision-source.png'
      })
      .mockResolvedValueOnce({
        id: 77,
        preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/precision-mask.png',
        original_filename: 'precision-mask.png'
      })
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 31,
      status: 'queued',
      stage: 'queued',
      available_credits: 4
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tool-precision_edit"]').trigger('click')
    await uploadWorkspaceReference(wrapper, new File(['source'], 'precision-source.png', { type: 'image/png' }))
    await flushPromises()
    await drawMaskStroke(wrapper.get('[data-testid="workspace-precision-mask-canvas"]'), [[10, 20], [40, 60]])
    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('把圈选区域改成红色礼盒')
    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.uploadReferenceAsset).toHaveBeenCalledTimes(2)
    expect(apiMocks.uploadReferenceAsset).toHaveBeenNthCalledWith(2, expect.any(File))
    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: '把圈选区域改成红色礼盒',
      edit_instruction: '把圈选区域改成红色礼盒',
      tool_mode: 'precision_edit',
      reference_asset_ids: [42],
      mask_asset_id: 77,
      tool_options: {
        mask_regions: [
          expect.objectContaining({
            x: expect.any(Number),
            y: expect.any(Number),
            width: expect.any(Number),
            height: expect.any(Number)
          })
        ]
      }
    }))
    expect(apiMocks.createImageGeneration.mock.calls[0][0].reference_asset_ids).toHaveLength(1)

    drawing.restore()
  })

  it('submits precision edit lasso mask and clears selection when replacing source', async () => {
    const drawing = mockCanvasDrawing()
    mockUser({ available_credits: 5 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.uploadReferenceAsset
      .mockResolvedValueOnce({
        id: 42,
        preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/first.png',
        original_filename: 'first.png'
      })
      .mockResolvedValueOnce({
        id: 43,
        preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/second.png',
        original_filename: 'second.png'
      })
      .mockResolvedValueOnce({
        id: 78,
        preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/lasso-mask.png',
        original_filename: 'lasso-mask.png'
      })
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 32,
      status: 'queued',
      stage: 'queued',
      available_credits: 4
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tool-precision_edit"]').trigger('click')
    await uploadWorkspaceReference(wrapper, new File(['first'], 'first.png', { type: 'image/png' }))
    await flushPromises()
    await wrapper.get('[data-testid="workspace-mask-mode-lasso"]').trigger('click')
    await drawMaskStroke(wrapper.get('[data-testid="workspace-precision-mask-canvas"]'), [[10, 20], [40, 20], [30, 60]])
    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('把圈选区域换成蓝色丝带')
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="workspace-create-button"]').attributes('disabled')).toBeUndefined()

    await uploadWorkspaceReference(wrapper, new File(['second'], 'second.png', { type: 'image/png' }))
    await flushPromises()
    expect(wrapper.get('[data-testid="workspace-reference-upload"]').text()).toContain('second.png')
    expect(wrapper.get('[data-testid="workspace-reference-upload"]').text()).not.toContain('first.png')
    expect(wrapper.get('[data-testid="workspace-create-button"]').attributes('disabled')).toBeDefined()

    await drawMaskStroke(wrapper.get('[data-testid="workspace-precision-mask-canvas"]'), [[20, 20], [60, 30], [40, 70]])
    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(drawing.canvasContext.fill).toHaveBeenCalled()
    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: '把圈选区域换成蓝色丝带',
      edit_instruction: '把圈选区域换成蓝色丝带',
      tool_mode: 'precision_edit',
      reference_asset_ids: [43],
      mask_asset_id: 78,
      tool_options: {
        mask_regions: [
          expect.objectContaining({
            x: expect.any(Number),
            y: expect.any(Number),
            width: expect.any(Number),
            height: expect.any(Number)
          })
        ]
      }
    }))

    drawing.restore()
  })

  it('disables precision edit submit after undoing or clearing the last region', async () => {
    const drawing = mockCanvasDrawing()
    mockUser({ available_credits: 5 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.uploadReferenceAsset.mockResolvedValueOnce({
      id: 42,
      preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/precision-source.png',
      original_filename: 'precision-source.png'
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tool-precision_edit"]').trigger('click')
    await uploadWorkspaceReference(wrapper, new File(['source'], 'precision-source.png', { type: 'image/png' }))
    await flushPromises()
    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('把圈选区域调亮')
    await drawMaskStroke(wrapper.get('[data-testid="workspace-precision-mask-canvas"]'), [[10, 20], [40, 60]])
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="workspace-create-button"]').attributes('disabled')).toBeUndefined()

    await wrapper.get('[data-testid="workspace-precision-mask-undo"]').trigger('click')
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="workspace-create-button"]').attributes('disabled')).toBeDefined()

    await drawMaskStroke(wrapper.get('[data-testid="workspace-precision-mask-canvas"]'), [[10, 20], [40, 60]])
    await wrapper.get('[data-testid="workspace-precision-mask-clear"]').trigger('click')
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="workspace-create-button"]').attributes('disabled')).toBeDefined()
    expect(apiMocks.createImageGeneration).not.toHaveBeenCalled()

    drawing.restore()
  })

  it('submits selected edit tool, uploaded source reference and quality', async () => {
    mockUser({ available_credits: 5 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.uploadReferenceAsset.mockResolvedValueOnce({
      id: 42,
      preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/ref.png',
      original_filename: 'ref.png'
    })
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 12,
      status: 'queued',
      stage: 'queued',
      available_credits: 4
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tool-remove_background"]').trigger('click')
    await uploadWorkspaceReference(wrapper, new File(['fake'], 'ref.png', { type: 'image/png' }))
    await flushPromises()
    await chooseClickSelect(wrapper, 'workspace-quality-select', 'high')
    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('保留主体，移除背景')
    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith({
      prompt: '保留主体，移除背景',
      negative_prompt: undefined,
      aspect_ratio: '1:1',
      model_id: 7,
      reference_asset_ids: [42],
      reference_weight: 75,
      tool_mode: 'remove_background',
      quality: 'high'
    })
  })

  it('submits remove background with one source image and the default transparent prompt', async () => {
    mockUser({ available_credits: 5 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.uploadReferenceAsset.mockResolvedValueOnce({
      id: 42,
      preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/cup.png',
      original_filename: 'cup.png'
    })
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 24,
      status: 'queued',
      stage: 'queued',
      available_credits: 4
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tool-remove_background"]').trigger('click')
    expect(wrapper.get('[data-testid="workspace-reference-upload"]').text()).toContain('上传需要移除背景的图片')
    await uploadWorkspaceReference(wrapper, new File(['source'], 'cup.png', { type: 'image/png' }))
    await flushPromises()

    expect(wrapper.get('[data-testid="workspace-reference-count"]').text()).toContain('已选 1/1')
    expect(wrapper.get('[data-testid="workspace-create-button"]').attributes('disabled')).toBeUndefined()

    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: expect.stringContaining('透明背景'),
      tool_mode: 'remove_background',
      reference_asset_ids: [42]
    }))
    expect(apiMocks.createImageGeneration.mock.calls[0][0].reference_asset_ids).toHaveLength(1)
  })

  it('sends remove background subject preservation notes as edit_instruction', async () => {
    mockUser({ available_credits: 5 })
    mockWorks()
    mockReferenceAssets()
    mockDiscovery({
      tools: [
        {
          mode: 'remove_background',
          title: '移除背景',
          description: '保留主体轮廓',
          icon: 'image',
          enabled: true,
          requires_source: true,
          source_limit: 1,
          form_schema: [
            { key: 'edit_instruction', label: '主体保留说明（可选）', type: 'textarea' }
          ]
        }
      ]
    })
    apiMocks.uploadReferenceAsset.mockResolvedValueOnce({
      id: 42,
      preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/bag.png',
      original_filename: 'bag.png'
    })
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 25,
      status: 'queued',
      stage: 'queued',
      available_credits: 4
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tool-remove_background"]').trigger('click')
    await uploadWorkspaceReference(wrapper, new File(['source'], 'bag.png', { type: 'image/png' }))
    await flushPromises()
    await wrapper.get('[data-testid="workspace-tool-option-edit_instruction"]').setValue('保留包带镂空和金属扣边缘')
    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: expect.stringContaining('透明背景'),
      edit_instruction: '保留包带镂空和金属扣边缘',
      tool_mode: 'remove_background',
      reference_asset_ids: [42]
    }))
  })

  it('replaces the previous remove background source when a second image is uploaded', async () => {
    mockUser({ available_credits: 5 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.uploadReferenceAsset
      .mockResolvedValueOnce({
        id: 42,
        preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/first.png',
        original_filename: 'first.png'
      })
      .mockResolvedValueOnce({
        id: 43,
        preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/second.png',
        original_filename: 'second.png'
      })
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 26,
      status: 'queued',
      stage: 'queued',
      available_credits: 4
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tool-remove_background"]').trigger('click')
    await uploadWorkspaceReference(wrapper, new File(['first'], 'first.png', { type: 'image/png' }))
    await flushPromises()
    await uploadWorkspaceReference(wrapper, new File(['second'], 'second.png', { type: 'image/png' }))
    await flushPromises()

    expect(wrapper.get('[data-testid="workspace-reference-upload"]').text()).toContain('second.png')
    expect(wrapper.get('[data-testid="workspace-reference-upload"]').text()).not.toContain('first.png')

    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith(expect.objectContaining({
      tool_mode: 'remove_background',
      reference_asset_ids: [43]
    }))
  })

  it('keeps the previous single-source image when replacement upload fails', async () => {
    mockUser({ available_credits: 6 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.uploadReferenceAsset
      .mockResolvedValueOnce({
        id: 42,
        preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/original.png',
        original_filename: 'original.png'
      })
      .mockRejectedValueOnce(new Error('上传超时，请重试'))
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 18,
      status: 'queued',
      stage: 'queued',
      available_credits: 5
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tool-remove_background"]').trigger('click')
    await uploadWorkspaceReference(wrapper, new File(['first'], 'first.png', { type: 'image/png' }))
    await flushPromises()
    await uploadWorkspaceReference(wrapper, new File(['second'], 'second.png', { type: 'image/png' }))
    await flushPromises()

    expect(wrapper.text()).toContain('上传超时，请重试')
    expect(wrapper.get('[data-testid="workspace-reference-upload"]').text()).toContain('original.png')
    expect(wrapper.get('[data-testid="workspace-reference-upload"]').text()).not.toContain('second.png')

    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith(expect.objectContaining({
      tool_mode: 'remove_background',
      reference_asset_ids: [42]
    }))
  })

  it('uses a checkerboard preview background for remove background results', async () => {
    mockUser({ available_credits: 5 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.uploadReferenceAsset.mockResolvedValueOnce({
      id: 42,
      preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/source.png',
      original_filename: 'source.png'
    })
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 27,
      status: 'queued',
      stage: 'queued',
      available_credits: 4
    })
    apiMocks.getImageGeneration.mockResolvedValueOnce({
      generation_id: 27,
      work_id: 99,
      status: 'succeeded',
      stage: 'succeeded',
      tool_mode: 'remove_background',
      prompt: '透明背景抠图',
      preview_url: '/api/works/99/file',
      download_url: '/api/works/99/download',
      available_credits: 4
    })
    apiMocks.listWorks.mockResolvedValueOnce({
      items: [
        {
          work_id: 99,
          tool_mode: 'remove_background',
          prompt: '透明背景抠图',
          preview_url: '/api/works/99/file',
          download_url: '/api/works/99/download'
        }
      ]
    })
    vi.useFakeTimers()

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tool-remove_background"]').trigger('click')
    await uploadWorkspaceReference(wrapper, new File(['source'], 'source.png', { type: 'image/png' }))
    await flushPromises()
    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()
    await vi.advanceTimersByTimeAsync(1000)
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="workspace-result-preview"]').classes()).toContain('workspace-transparent-preview')
  })

  it('submits smart expand with percent edges, one source image and default prompt', async () => {
    mockUser({ available_credits: 8 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.uploadReferenceAsset.mockResolvedValueOnce({
      id: 42,
      preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/expand-source.png',
      original_filename: 'expand-source.png'
    })
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 18,
      status: 'queued',
      stage: 'queued',
      available_credits: 7
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tool-expand"]').trigger('click')
    await uploadWorkspaceReference(wrapper, new File(['fake'], 'expand-source.png', { type: 'image/png' }))
    await flushPromises()
    await wrapper.get('[data-testid="workspace-tool-option-top"]').setValue('50')
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="workspace-reference-count"]').text()).toContain('已选 1/1')
    expect(wrapper.get('[data-testid="workspace-expand-preview"]').text()).toContain('目标画布')
    expect(wrapper.get('[data-testid="workspace-expand-preview"]').text()).toContain('上 50%')
    expect(wrapper.get('[data-testid="workspace-create-button"]').attributes('disabled')).toBeUndefined()

    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: '只补外扩背景，原图人物和主体区域将保持不变；让新增边界与原图光线、透视、材质和画风自然衔接。',
      aspect_ratio: '1:1',
      model_id: 7,
      tool_mode: 'expand',
      tool_options: {
        unit: 'percent',
        top: 50,
        bottom: 20,
        left: 20,
        right: 20
      },
      reference_asset_ids: [42],
      reference_weight: 75
    }))
  })

  it('keeps smart expand upload progress inside the reference upload zone', async () => {
    mockUser({ available_credits: 8 })
    mockWorks()
    mockReferenceAssets()
    const pendingUpload = deferred()
    apiMocks.uploadReferenceAsset.mockReturnValueOnce(pendingUpload.promise)

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tool-expand"]').trigger('click')
    await uploadWorkspaceReference(wrapper, new File(['fake'], 'expand-source.png', { type: 'image/png' }))
    await flushPromises()
    await wrapper.vm.$nextTick()

    const referenceUpload = wrapper.get('[data-testid="workspace-reference-upload"]')
    const uploadZone = referenceUpload.get('[data-testid="workspace-reference-dropzone"]')
    const status = referenceUpload.get('[data-testid="workspace-reference-upload-status"]')

    expect(status.text()).toContain('上传中...')
    expect(uploadZone.element.contains(status.element)).toBe(true)

    pendingUpload.resolve({
      id: 42,
      preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/expand-source.png',
      original_filename: 'expand-source.png'
    })
    await flushPromises()
  })

  it('fills creation parameters from backend discovery templates without using paid template endpoint', async () => {
    mockUser({ available_credits: 5 })
    mockWorks()
    mockReferenceAssets()
    mockDiscovery({
      hot: [
        {
          id: 301,
          title: '后台热门商品图',
          description: '后台模板描述',
          prompt: '后台模板提示词',
          preview_url: 'https://oss.example.com/product.png',
          aspect_ratio: '4:3',
          style_preset: '电商',
          tool_mode: 'generate'
        }
      ],
      inspiration: []
    })
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 12,
      status: 'queued',
      stage: 'queued',
      available_credits: 4
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-template-301"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="workspace-prompt-input"]').element.value).toBe('后台模板提示词')
    expect(wrapper.get('[data-testid="workspace-size-select"]').element.value).toBe('4:3')
    const advancedPanel = await openHomeAdvancedPanel(wrapper)
    expect(homeAdvancedStyleChip(advancedPanel, '电商').classList.contains('active')).toBe(true)
    expect(wrapper.get('[data-testid="workspace-tab-create"]').classes()).toContain('active')

    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: '后台模板提示词',
      aspect_ratio: '4:3',
      style_preset: '电商',
      tool_mode: 'generate'
    }))
  })

  it('ignores unsupported discovery tool modes and falls templates back to text-to-image', async () => {
    mockUser({ available_credits: 5 })
    mockWorks()
    mockReferenceAssets()
    mockDiscovery({
      tools: [
        {
          mode: 'experimental_magic',
          title: '实验工具',
          description: '后端误下发的未开放工具',
          icon: 'sparkles',
          enabled: true,
          sort_order: 1,
          requires_source: false,
          form_schema: [
            { key: 'strength', label: '强度', type: 'number', default: 99, min: 0, max: 100 }
          ]
        }
      ],
      hot: [
        {
          id: 401,
          title: '异常工具模板',
          description: '模板不应把未知工具模式写入 payload',
          prompt: '后台模板提示词',
          preview_url: 'https://oss.example.com/unknown-tool.png',
          aspect_ratio: '1:1',
          style_preset: '',
          tool_mode: 'experimental_magic'
        }
      ],
      inspiration: []
    })
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 12,
      status: 'queued',
      stage: 'queued',
      available_credits: 4
    })

    const wrapper = await mountReady()

    expect(wrapper.find('[data-testid="workspace-tool-experimental_magic"]').exists()).toBe(false)

    await wrapper.get('[data-testid="workspace-template-401"]').trigger('click')
    await wrapper.vm.$nextTick()
    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: '后台模板提示词',
      tool_mode: 'generate'
    }))
    expect(apiMocks.createImageGeneration.mock.calls.at(-1)[0]).not.toHaveProperty('tool_options')
  })

  it('submits uploaded references from the unified image workspace', async () => {
    mockUser({ available_credits: 5 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.uploadReferenceAsset.mockResolvedValueOnce({
      id: 42,
      preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/ref.png',
      original_filename: 'ref.png'
    })
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 12,
      status: 'queued',
      stage: 'queued',
      available_credits: 4
    })

    const wrapper = await mountReady()

    await uploadWorkspaceReference(wrapper, new File(['fake'], 'ref.png', { type: 'image/png' }))
    await flushPromises()
    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('make this product poster-ready')
    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.uploadReferenceAsset).toHaveBeenCalled()
    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith({
      prompt: 'make this product poster-ready',
      negative_prompt: undefined,
      aspect_ratio: '1:1',
      model_id: 7,
      reference_asset_ids: [42],
      reference_weight: 75,
      tool_mode: 'generate'
    })
  })

  it('submits history work references from the unified image workspace', async () => {
    mockUser({ available_credits: 5 })
    mockWorks([
      {
        work_id: 90,
        prompt: 'existing portrait',
        preview_url: '/api/works/90/file',
        download_url: '/api/works/90/download',
        aspect_ratio: '1:1',
        created_at: '2026-04-28T10:00:00Z'
      }
    ])
    mockReferenceAssets()
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 12,
      status: 'queued',
      stage: 'queued',
      available_credits: 4
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tab-create"]').trigger('click')
    await wrapper.get('[data-testid="workspace-history-use-as-reference"]').trigger('click')
    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('use this face as visual reference')
    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: 'use this face as visual reference',
      reference_work_ids: [90],
      reference_weight: 75
    }))
    expect(apiMocks.createImageGeneration.mock.calls[0][0]).not.toHaveProperty('source_work_id')
  })

  it('marks mixed multi references as compose requests and sends reference strength to estimate and create', async () => {
    vi.useFakeTimers()
    mockUser({ available_credits: 5 })
    mockWorks([
      {
        work_id: 90,
        prompt: 'first reference',
        preview_url: '/api/works/90/file',
        download_url: '/api/works/90/download',
        aspect_ratio: '1:1',
        created_at: '2026-04-28T10:00:00Z'
      }
    ])
    mockReferenceAssets()
    apiMocks.estimateImageGeneration.mockResolvedValue({
      required_credits: 2,
      enough: true
    })
    apiMocks.uploadReferenceAsset.mockResolvedValueOnce({
      id: 42,
      preview_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/ref.png',
      original_filename: 'ref.png'
    })
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 12,
      status: 'queued',
      stage: 'queued',
      available_credits: 4
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tab-create"]').trigger('click')
    await uploadWorkspaceReference(wrapper, new File(['fake'], 'ref.png', { type: 'image/png' }))
    await flushPromises()
    await wrapper.get('[data-testid="workspace-history-use-as-reference"]').trigger('click')
    const advancedPanel = await openHomeAdvancedPanel(wrapper)
    await setHomeAdvancedInput(wrapper, advancedPanel, 'workspace-reference-strength', '58')
    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('compose these references into one poster')
    await advanceCreditEstimateDebounce()

    expect(apiMocks.estimateImageGeneration).toHaveBeenLastCalledWith(
      expect.objectContaining({
        prompt: 'compose these references into one poster',
        reference_asset_ids: [42],
        reference_work_ids: [90],
        reference_weight: 58,
        reference_intent: 'compose'
      }),
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )

    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: 'compose these references into one poster',
      reference_asset_ids: [42],
      reference_work_ids: [90],
      reference_weight: 58,
      reference_intent: 'compose'
    }))
  })

  it('submits a prompt-only generation without style fields by default', async () => {
    mockUser()
    mockWorks()
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 12,
      status: 'queued',
      stage: 'queued',
      available_credits: 3
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-size-select"]').setValue('21:9')
    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('plain prompt scene')
    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith({
      prompt: 'plain prompt scene',
      negative_prompt: undefined,
      aspect_ratio: '21:9',
      model_id: 7,
      tool_mode: 'generate'
    })
  })

  it('opens the AI prompt assistant, auto starts from the current prompt, sends chat context, and applies the draft prompt', async () => {
    mockUser()
    mockWorks()
    apiMocks.optimizePrompt
      .mockResolvedValueOnce({
        reply: '已根据当前提示词整理出第一版。',
        optimized_prompt: '一只小猫在花园里，柔和自然光，温暖治愈。',
        structured_prompt: {
          subject: '小猫',
          scene: '花园',
          style: '自然光，温暖治愈',
          usage: ''
        },
        model: 'deepseek-v4'
      })
      .mockResolvedValueOnce({
        reply: '明白了！我为你整理了提示词，你也可以继续补充或调整。',
        optimized_prompt: '一只橘白相间的猫坐在花园石板路上，阳光柔和，写实风格，温暖治愈。',
        structured_prompt: {
          subject: '橘白相间的猫',
          scene: '花园整体，石板路，周围有花',
          style: '写实，自然光，温暖治愈',
          usage: '社交媒体配图'
        },
        safety_notes: ['已规避可能导致生成失败的敏感描述'],
        model: 'deepseek-v4'
      })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('一直小猫')
    await wrapper.get('[data-testid="workspace-open-prompt-optimizer"]').trigger('click')
    await wrapper.vm.$nextTick()

    const modal = document.body.querySelector('[data-testid="workspace-prompt-optimizer-modal"]')
    expect(modal).not.toBeNull()
    expect(modal.textContent).toContain('AI 提示词助手')
    expect(modal.textContent).toContain('一直小猫')
    expect(modal.textContent).toContain('主体')
    await flushPromises()

    expect(apiMocks.optimizePrompt.mock.calls[0][0]).toEqual(expect.objectContaining({
      prompt: '一直小猫',
      mode: 'chat',
      action: 'start',
      message: '',
      aspect_ratio: '1:1',
      style_preset: ''
    }))
    expect(document.body.querySelector('[data-testid="workspace-assistant-draft-prompt"]').value).toContain('一只小猫在花园里')

    const assistantInput = modal.querySelector('[data-testid="workspace-prompt-assistant-input"]')
    assistantInput.value = '偏写实，花园整体，温暖治愈。'
    assistantInput.dispatchEvent(new Event('input'))
    await wrapper.vm.$nextTick()
    modal.querySelector('[data-testid="workspace-prompt-assistant-send"]').click()
    await flushPromises()

    expect(apiMocks.optimizePrompt.mock.calls[1][0]).toEqual(expect.objectContaining({
      prompt: '一直小猫',
      mode: 'chat',
      action: 'continue',
      message: '偏写实，花园整体，温暖治愈。',
      aspect_ratio: '1:1',
      style_preset: ''
    }))
    expect(apiMocks.optimizePrompt.mock.calls[1][0].history).toEqual([
      { role: 'user', content: '一直小猫' },
      {
        role: 'assistant',
        content: expect.stringContaining('第一版')
      },
      { role: 'user', content: '偏写实，花园整体，温暖治愈。' }
    ])

    expect(document.body.textContent).toContain('橘白相间的猫')
    expect(document.body.textContent).toContain('花园整体，石板路，周围有花')
    expect(document.body.textContent).toContain('已规避可能导致生成失败的敏感描述')
    const messages = modal.querySelector('.prompt-assistant-messages')
    const userMessages = [...messages.querySelectorAll('.prompt-assistant-message.is-user p')]
    const assistantMessages = [...messages.querySelectorAll('.prompt-assistant-message.is-assistant p')]
    const resultBubble = messages.querySelector('[data-testid="workspace-assistant-result-bubble"]')
    expect(userMessages.at(-1).textContent).toBe('偏写实，花园整体，温暖治愈。')
    expect(assistantMessages.some((message) => message.textContent.includes('明白了'))).toBe(true)
    expect(resultBubble).not.toBeNull()
    expect(resultBubble.textContent).toContain('主体')
    expect(resultBubble.textContent).toContain('场景')
    expect(resultBubble.textContent).toContain('风格')
    expect(resultBubble.textContent).toContain('用途')
    expect(resultBubble.textContent).toContain('提示词预览')
    expect(resultBubble.querySelector('[data-testid="workspace-assistant-draft-prompt"]')).not.toBeNull()
    expect(modal.querySelector('.prompt-assistant-result-collapse')).toBeNull()
    const draftInput = document.body.querySelector('[data-testid="workspace-assistant-draft-prompt"]')
    expect(draftInput.value).toContain('橘白相间的猫坐在花园石板路上')

    document.body.querySelector('[data-testid="workspace-apply-assistant-prompt"]').click()
    await flushPromises()

    expect(wrapper.get('[data-testid="workspace-prompt-input"]').element.value).toBe('一只橘白相间的猫坐在花园石板路上，阳光柔和，写实风格，温暖治愈。')
    expect(document.body.querySelector('[data-testid="workspace-prompt-optimizer-modal"]')).toBeNull()
    expect(apiMocks.createImageGeneration).not.toHaveBeenCalled()
  })

  it('shows inferred structured fields and a soft empty state for fields the AI leaves blank', async () => {
    mockUser()
    mockWorks()
    apiMocks.optimizePrompt.mockResolvedValueOnce({
      reply: '已根据当前提示词整理出第一版。',
      optimized_prompt: '图中的人物全身站姿，透明背景，适合 AI 剧本角色包。',
      structured_prompt: {
        subject: '图中的人物，全身站姿',
        scene: '透明背景角色展示',
        style: '写实角色素材',
        usage: ''
      },
      model: 'deepseek-v4'
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('把图中的人物抠出来做AI剧本角色包，要人物全身图')
    await wrapper.get('[data-testid="workspace-open-prompt-optimizer"]').trigger('click')
    await flushPromises()

    const resultBubble = document.body.querySelector('[data-testid="workspace-assistant-result-bubble"]')
    expect(resultBubble.textContent).toContain('图中的人物，全身站姿')
    expect(resultBubble.textContent).toContain('透明背景角色展示')
    expect(resultBubble.textContent).toContain('写实角色素材')
    expect(resultBubble.textContent).toContain('AI 暂未判断，可补充')
    expect(resultBubble.textContent).not.toContain('待补充')
    expect(document.body.querySelector('[data-testid="workspace-assistant-draft-prompt"]').value).toBe('图中的人物全身站姿，透明背景，适合 AI 剧本角色包。')

    document.body.querySelector('[data-testid="workspace-apply-assistant-prompt"]').click()
    await flushPromises()

    expect(wrapper.get('[data-testid="workspace-prompt-input"]').element.value).toBe('图中的人物全身站姿，透明背景，适合 AI 剧本角色包。')
  })

  it('requests realistic refinement and lets users select a returned direction', async () => {
    mockUser()
    mockWorks()
    apiMocks.optimizePrompt
      .mockResolvedValueOnce({
        reply: '已根据当前提示词整理出第一版。',
        optimized_prompt: '橘白猫在花园中，自然光。',
        structured_prompt: {
          subject: '橘白猫',
          scene: '花园',
          style: '自然光',
          usage: ''
        },
        model: 'deepseek-v4'
      })
      .mockResolvedValueOnce({
        reply: '已增强写实摄影描述。',
        optimized_prompt: '橘白猫在花园中，85mm 镜头，自然光，浅景深，写实摄影。',
        structured_prompt: {
          subject: '橘白猫',
          scene: '花园',
          style: '写实摄影，85mm 镜头，自然光',
          usage: '社交媒体配图'
        },
        model: 'deepseek-v4'
      })
      .mockResolvedValueOnce({
        reply: '可以换成下面 3 个方向。',
        optimized_prompt: '橘白猫在花园中，85mm 镜头，自然光，浅景深，写实摄影。',
        structured_prompt: {
          subject: '橘白猫',
          scene: '花园',
          style: '写实摄影，85mm 镜头，自然光',
          usage: '社交媒体配图'
        },
        directions: [
          {
            title: '温暖写实',
            summary: '自然光下的花园猫咪',
            prompt: '一只橘白猫坐在晨光花园里，写实摄影，柔和浅景深。'
          },
          {
            title: '清新插画',
            summary: '色彩轻盈的花园插画',
            prompt: '一只橘白猫在花园里散步，清新插画风格，明亮色彩。',
            structured_prompt: {
              subject: '橘白猫',
              scene: '花园散步',
              style: '清新插画，明亮色彩',
              usage: '儿童读物插图'
            }
          }
        ],
        model: 'deepseek-v4'
      })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('花园里的猫')
    await wrapper.get('[data-testid="workspace-open-prompt-optimizer"]').trigger('click')
    await wrapper.vm.$nextTick()

    document.body.querySelector('[data-testid="workspace-assistant-realistic"]').click()
    await flushPromises()

    expect(apiMocks.optimizePrompt.mock.calls[0][0]).toEqual(expect.objectContaining({
      prompt: '花园里的猫',
      mode: 'chat',
      action: 'start',
      message: ''
    }))

    expect(apiMocks.optimizePrompt.mock.calls[1][0]).toEqual(expect.objectContaining({
      prompt: '花园里的猫',
      mode: 'realistic',
      action: 'make_realistic',
      message: '更偏写实'
    }))
    expect(document.body.querySelector('[data-testid="workspace-assistant-draft-prompt"]').value).toContain('85mm 镜头')

    document.body.querySelector('[data-testid="workspace-assistant-change-direction"]').click()
    await flushPromises()

    expect(apiMocks.optimizePrompt.mock.calls[2][0]).toEqual(expect.objectContaining({
      mode: 'direction',
      action: 'change_direction',
      message: '换个方向'
    }))
    expect(document.body.textContent).toContain('清新插画')

    document.body.querySelector('[data-testid="workspace-assistant-direction-1"]').click()
    await wrapper.vm.$nextTick()

    expect(document.body.querySelector('[data-testid="workspace-assistant-draft-prompt"]').value).toBe('一只橘白猫在花园里散步，清新插画风格，明亮色彩。')
    expect(document.body.textContent).toContain('花园散步')
    expect(document.body.textContent).toContain('儿童读物插图')
    expect(document.body.textContent).toContain('已切换到「清新插画」方向')
  })

  it('rebuilds the prompt preview immediately after structured fields are edited', async () => {
    mockUser()
    mockWorks()
    apiMocks.optimizePrompt.mockResolvedValueOnce({
      reply: '已整理。',
      optimized_prompt: '橘白猫在花园里，写实摄影。',
      structured_prompt: {
        subject: '橘白猫',
        scene: '花园草地',
        style: '写实摄影',
        usage: '社交媒体配图'
      },
      model: 'deepseek-v4'
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('花园里的猫')
    await wrapper.get('[data-testid="workspace-open-prompt-optimizer"]').trigger('click')
    await flushPromises()

    document.body.querySelector('button[aria-label="编辑风格"]').click()
    await wrapper.vm.$nextTick()
    const fieldInput = document.body.querySelector('.prompt-assistant-field-input')
    fieldInput.value = '水彩插画，轻盈色彩'
    fieldInput.dispatchEvent(new Event('input'))
    fieldInput.dispatchEvent(new FocusEvent('blur'))
    await wrapper.vm.$nextTick()

    expect(document.body.querySelector('[data-testid="workspace-assistant-draft-prompt"]').value).toBe('橘白猫，花园草地，水彩插画，轻盈色彩，用途：社交媒体配图')
  })

  it('shows the assistant prompt preview in a taller editing box', async () => {
    mockUser()
    mockWorks()
    apiMocks.optimizePrompt.mockResolvedValueOnce({
      reply: '已整理。',
      optimized_prompt: '一位亚洲青年模特，身穿深蓝色水手服风格套装与百褶裙，黑色齐耳短发，面容素净温柔，站在洒满阳光的学校走廊，窗外透进柔和自然光，青春日系校园摄影风格。',
      structured_prompt: {
        subject: '亚洲青年模特',
        scene: '学校走廊',
        style: '青春日系校园摄影风格',
        usage: ''
      },
      model: 'deepseek-v4'
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('校园风青年模特')
    await wrapper.get('[data-testid="workspace-open-prompt-optimizer"]').trigger('click')
    await flushPromises()

    expect(document.body.querySelector('[data-testid="workspace-assistant-draft-prompt"]').getAttribute('rows')).toBe('10')
  })

  it('allows opening the prompt assistant with an empty prompt', async () => {
    mockUser()
    mockWorks()
    apiMocks.optimizePrompt.mockResolvedValueOnce({
      reply: '我先帮你整理一个基础方向。',
      optimized_prompt: '未来城市夜景，霓虹灯闪烁，电影感构图。',
      structured_prompt: {
        subject: '未来城市',
        scene: '夜景街区',
        style: '电影感',
        usage: '概念图'
      },
      model: 'deepseek-v4'
    })

    const wrapper = await mountReady()

    expect(wrapper.get('[data-testid="workspace-open-prompt-optimizer"]').attributes('disabled')).toBeUndefined()
    await wrapper.get('[data-testid="workspace-open-prompt-optimizer"]').trigger('click')
    await wrapper.vm.$nextTick()

    const modal = document.body.querySelector('[data-testid="workspace-prompt-optimizer-modal"]')
    expect(modal.textContent).toContain('告诉我你想生成什么画面')
    expect(modal.textContent).toContain('人像')
    expect(modal.textContent).toContain('商品')
    expect(modal.textContent).toContain('场景')
    expect(apiMocks.optimizePrompt).not.toHaveBeenCalled()

    const assistantInput = modal.querySelector('[data-testid="workspace-prompt-assistant-input"]')
    assistantInput.value = '未来城市夜景'
    assistantInput.dispatchEvent(new Event('input'))
    await wrapper.vm.$nextTick()
    modal.querySelector('[data-testid="workspace-prompt-assistant-send"]').click()
    await flushPromises()

    expect(apiMocks.optimizePrompt.mock.calls[0][0]).toEqual(expect.objectContaining({
      prompt: '未来城市夜景',
      mode: 'chat',
      action: 'continue',
      message: '未来城市夜景',
      aspect_ratio: '1:1',
      style_preset: ''
    }))
    expect(document.body.querySelector('[data-testid="workspace-assistant-draft-prompt"]').value).toContain('未来城市夜景')
  })

  it('retries the failed assistant action without clearing the draft or duplicating history', async () => {
    mockUser()
    mockWorks()
    apiMocks.optimizePrompt
      .mockRejectedValueOnce(new Error('网络暂时不可用'))
      .mockResolvedValueOnce({
        reply: '已恢复并整理完成。',
        optimized_prompt: '未来城市夜景，霓虹灯闪烁，电影感构图。',
        structured_prompt: {
          subject: '未来城市',
          scene: '夜景街区',
          style: '电影感',
          usage: ''
        },
        model: 'deepseek-v4'
      })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-open-prompt-optimizer"]').trigger('click')
    await wrapper.vm.$nextTick()
    const assistantInput = document.body.querySelector('[data-testid="workspace-prompt-assistant-input"]')
    assistantInput.value = '未来城市夜景'
    assistantInput.dispatchEvent(new Event('input'))
    await wrapper.vm.$nextTick()
    document.body.querySelector('[data-testid="workspace-prompt-assistant-send"]').click()
    await flushPromises()

    expect(document.body.textContent).toContain('网络暂时不可用')
    expect(document.body.querySelector('[data-testid="workspace-assistant-draft-prompt"]').value).toBe('')

    document.body.querySelector('[data-testid="workspace-assistant-retry"]').click()
    await flushPromises()

    expect(apiMocks.optimizePrompt).toHaveBeenCalledTimes(2)
    expect(apiMocks.optimizePrompt.mock.calls[1][0]).toEqual(expect.objectContaining({
      prompt: '未来城市夜景',
      action: 'continue',
      message: '未来城市夜景'
    }))
    expect(apiMocks.optimizePrompt.mock.calls[1][0].history).toEqual([
      { role: 'assistant', content: expect.stringContaining('告诉我') },
      { role: 'user', content: '未来城市夜景' }
    ])
    expect(document.body.querySelector('[data-testid="workspace-assistant-draft-prompt"]').value).toContain('未来城市夜景')
  })

  it('cancels a running assistant request when the modal is closed', async () => {
    mockUser()
    mockWorks()
    const pending = deferred()
    let capturedSignal
    apiMocks.optimizePrompt.mockImplementationOnce((_payload, options) => {
      capturedSignal = options?.signal
      return pending.promise
    })

    const wrapper = await mountReady({
      ...mountOptions,
      attachTo: document.body.appendChild(document.createElement('div'))
    })

    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('花园里的猫')
    await wrapper.get('[data-testid="workspace-open-prompt-optimizer"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(document.body.textContent).toContain('正在初步整理')
    expect(document.body.querySelector('[data-testid="workspace-assistant-result-bubble"]')).toBeNull()
    document.body.querySelector('button[aria-label="关闭提示词助手"]').click()
    await wrapper.vm.$nextTick()

    expect(capturedSignal?.aborted).toBe(true)
    expect(document.body.querySelector('[data-testid="workspace-prompt-optimizer-modal"]')).toBeNull()
  })

  it('focuses the assistant input, traps tab inside the modal, and restores focus after Escape', async () => {
    mockUser()
    mockWorks()
    const wrapper = await mountReady({
      ...mountOptions,
      attachTo: document.body.appendChild(document.createElement('div'))
    })

    const openButton = wrapper.get('[data-testid="workspace-open-prompt-optimizer"]').element
    openButton.focus()
    await wrapper.get('[data-testid="workspace-open-prompt-optimizer"]').trigger('click')
    await wrapper.vm.$nextTick()
    await wrapper.vm.$nextTick()

    const modal = document.body.querySelector('[data-testid="workspace-prompt-optimizer-modal"]')
    const assistantInput = modal.querySelector('[data-testid="workspace-prompt-assistant-input"]')
    const closeButton = modal.querySelector('button[aria-label="关闭提示词助手"]')
    expect(document.activeElement).toBe(assistantInput)

    closeButton.focus()
    modal.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true }))
    await wrapper.vm.$nextTick()
    expect(modal.contains(document.activeElement)).toBe(true)
    expect(document.activeElement).not.toBe(openButton)

    modal.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await wrapper.vm.$nextTick()
    expect(document.body.querySelector('[data-testid="workspace-prompt-optimizer-modal"]')).toBeNull()
    expect(document.activeElement).toBe(openButton)
  })

  it('lets users select and then cancel a workspace style', async () => {
    mockUser()
    mockWorks()
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 12,
      status: 'queued',
      stage: 'queued',
      available_credits: 3
    })

    const wrapper = await mountReady()

    const advancedPanel = await openHomeAdvancedPanel(wrapper)
    const guofeng = homeAdvancedStyleChip(advancedPanel, '国风')
    guofeng.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await wrapper.vm.$nextTick()
    expect(guofeng.classList.contains('active')).toBe(true)
    guofeng.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await wrapper.vm.$nextTick()
    expect(guofeng.classList.contains('active')).toBe(false)
    expect(advancedPanel.querySelector('.style-chip.active').textContent.trim()).toBe('无风格')

    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('plain prompt after cancel')
    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.createImageGeneration.mock.calls[0][0]).not.toHaveProperty('style_preset')
  })

  it('submits negative prompt and selected style when a style is selected', async () => {
    mockUser()
    mockWorks()
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 12,
      status: 'queued',
      stage: 'queued',
      available_credits: 3
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('make the source image sharper')
    const advancedPanel = await openHomeAdvancedPanel(wrapper)
    await setHomeAdvancedInput(wrapper, advancedPanel, 'workspace-negative-prompt', 'noise, watermark')
    homeAdvancedStyleChip(advancedPanel, '国风').dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await wrapper.vm.$nextTick()
    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith({
      prompt: 'make the source image sharper',
      negative_prompt: 'noise, watermark',
      aspect_ratio: '1:1',
      model_id: 7,
      tool_mode: 'generate',
      style_preset: '国风'
    })
  })

  it('shows the selected running task progress and refreshes after success', async () => {
    vi.useFakeTimers()
    mockUser()
    apiMocks.listWorks
      .mockResolvedValueOnce({
        items: [
          {
            work_id: 90,
            prompt: 'mist over bamboo lake',
            preview_url: '/api/works/90/file',
            download_url: '/api/works/90/download',
            created_at: '2026-04-28T10:00:00Z'
          }
        ]
      })
      .mockResolvedValueOnce({
        items: [
          {
            work_id: 91,
            prompt: 'new bamboo lake',
            preview_url: '/api/works/91/file',
            download_url: '/api/works/91/download',
            created_at: '2026-04-28T10:01:00Z'
          }
        ]
      })
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 12,
      status: 'queued',
      stage: 'queued',
      available_credits: 3
    })
    apiMocks.getImageGeneration
      .mockResolvedValueOnce({
        generation_id: 12,
        status: 'running',
        stage: 'requesting_provider',
        available_credits: 3
      })
      .mockResolvedValueOnce({
        generation_id: 12,
        work_id: 91,
        status: 'succeeded',
        stage: 'succeeded',
        prompt: 'new bamboo lake',
        preview_url: '/api/works/91/file',
        download_url: '/api/works/91/download',
        available_credits: 2
      })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tab-create"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.get('.preview-image').attributes('src')).toBe('/api/works/90/file')
    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('new bamboo lake')
    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(wrapper.find('.preview-image').exists()).toBe(false)
    expect(wrapper.text()).toContain('任务已创建')

    await vi.advanceTimersByTimeAsync(1000)
    await flushPromises()
    expect(wrapper.text()).toContain('正在请求模型')

    await vi.advanceTimersByTimeAsync(1000)
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(apiMocks.getImageGeneration).toHaveBeenCalledWith(12)
    expect(apiMocks.listWorks).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('最新作品已写入作品库。')
    expect(wrapper.get('.preview-image').attributes('src')).toBe('/api/works/91/file')
  })

  it('persists a newly created generation and restores a running task after remounting', async () => {
    vi.useFakeTimers()
    mockUser({ available_credits: 5 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 12,
      status: 'queued',
      stage: 'queued',
      created_at: '2026-05-30T10:00:00Z',
      available_credits: 4
    })
    apiMocks.getImageGeneration.mockResolvedValue({
      generation_id: 12,
      status: 'running',
      stage: 'requesting_provider',
      created_at: '2026-05-30T10:00:00Z',
      prompt: 'mist over bamboo lake',
      available_credits: 4
    })

    const wrapper = await mountReady()
    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('mist over bamboo lake')
    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    expect(JSON.parse(window.sessionStorage.getItem(activeGenerationStorageKey))).toEqual(expect.objectContaining({
      generation_id: 12,
      status: 'queued',
      stage: 'queued',
      prompt: 'mist over bamboo lake',
      available_credits: 4
    }))

    wrapper.unmount()
    mountedWrappers.splice(mountedWrappers.indexOf(wrapper), 1)

    mockUser({ available_credits: 4 })
    mockWorks()
    mockReferenceAssets()
    const restored = await mountReady()

    expect(apiMocks.getImageGeneration).toHaveBeenCalledWith(12)
    expect(restored.get('[data-testid="workspace-create-panel"]').text()).toContain('正在请求模型')
    expect(restored.find('[data-testid="workspace-result-error"]').exists()).toBe(false)
    expect(JSON.parse(window.sessionStorage.getItem(activeGenerationStorageKey))).toEqual(expect.objectContaining({
      generation_id: 12,
      status: 'running',
      stage: 'requesting_provider',
      prompt: 'mist over bamboo lake',
      available_credits: 4
    }))
  })

  it('opens the create tab when restoring an active queued generation snapshot', async () => {
    vi.useFakeTimers()
    window.sessionStorage.setItem(activeGenerationStorageKey, JSON.stringify({
      generation_id: 72,
      status: 'queued',
      stage: 'queued',
      created_at: '2026-05-30T10:00:00Z',
      prompt: '正在生成的草图',
      parameters: { aspect_ratio: '1:1' },
      available_credits: 4
    }))
    mockUser({ available_credits: 4 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.getImageGeneration.mockResolvedValueOnce({
      generation_id: 72,
      status: 'running',
      stage: 'requesting_provider',
      prompt: '正在生成的草图',
      available_credits: 4
    })

    const wrapper = await mountReady()

    expect(apiMocks.getImageGeneration).toHaveBeenCalledWith(72)
    expect(wrapper.get('[data-testid="workspace-tab-create"]').classes()).toContain('active')
    expect(wrapper.get('[data-testid="workspace-create-panel"]').text()).toContain('正在请求模型')
    expect(wrapper.find('[data-testid="workspace-discovery-panel"]').exists()).toBe(false)

    vi.useRealTimers()
  })

  it('restores multiple running generation snapshots from local storage and keeps polling each one', async () => {
    vi.useFakeTimers()
    window.localStorage.setItem(activeGenerationStorageKey, JSON.stringify([
      {
        generation_id: 81,
        status: 'queued',
        stage: 'queued',
        created_at: '2026-06-06T10:00:00Z',
        prompt: '第一条恢复任务',
        parameters: { prompt: '第一条恢复任务', aspect_ratio: '1:1', tool_mode: 'generate' }
      },
      {
        generation_id: 82,
        status: 'running',
        stage: 'requesting_provider',
        created_at: '2026-06-06T10:01:00Z',
        prompt: '第二条恢复任务',
        parameters: { prompt: '第二条恢复任务', aspect_ratio: '1:1', tool_mode: 'generate' }
      }
    ]))
    mockUser({ available_credits: 4 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.getImageGeneration.mockImplementation(async (id) => ({
      generation_id: id,
      status: 'running',
      stage: id === 81 ? 'queued' : 'requesting_provider',
      prompt: id === 81 ? '第一条恢复任务' : '第二条恢复任务',
      available_credits: 4
    }))

    const wrapper = await mountReady()

    expect(wrapper.get('[data-testid="workspace-generation-task-81"]').text()).toContain('第一条恢复任务')
    expect(wrapper.get('[data-testid="workspace-generation-task-82"]').text()).toContain('第二条恢复任务')
    expect(wrapper.get('[data-testid="workspace-generation-task-81"]').classes()).toContain('active')

    await vi.advanceTimersByTimeAsync(1000)
    await flushPromises()

    expect(apiMocks.getImageGeneration).toHaveBeenCalledWith(81)
    expect(apiMocks.getImageGeneration).toHaveBeenCalledWith(82)
  })

  it('keeps showing generation progress during transient polling network errors', async () => {
    vi.useFakeTimers()
    mockUser({ available_credits: 5 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 12,
      status: 'queued',
      stage: 'queued',
      available_credits: 4
    })
    apiMocks.getImageGeneration
      .mockRejectedValueOnce(new TypeError('Failed to fetch'))
      .mockRejectedValueOnce(new TypeError('Failed to fetch'))
      .mockResolvedValueOnce({
        generation_id: 12,
        status: 'running',
        stage: 'requesting_provider',
        available_credits: 4
      })

    const wrapper = await mountReady()
    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('mist over bamboo lake')
    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()

    await vi.advanceTimersByTimeAsync(1000)
    await flushPromises()
    await vi.advanceTimersByTimeAsync(1000)
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(wrapper.find('[data-testid="workspace-result-error"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="workspace-result-stage"]').text()).toContain('正在重新连接任务状态')
    expect(wrapper.get('[data-testid="workspace-result-stage"]').text()).toContain('生成中')

    await vi.advanceTimersByTimeAsync(1000)
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(wrapper.find('[data-testid="workspace-result-error"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="workspace-result-stage"]').text()).toContain('正在请求模型')
    expect(window.sessionStorage.getItem(activeGenerationStorageKey)).toContain('"status":"running"')
  })

  it('clears the persisted generation after completion or an explicit backend failure', async () => {
    vi.useFakeTimers()
    mockUser({ available_credits: 5 })
    apiMocks.listWorks
      .mockResolvedValueOnce({ items: [] })
      .mockResolvedValueOnce({
        items: [
          {
            work_id: 91,
            prompt: 'new bamboo lake',
            preview_url: '/api/works/91/file',
            download_url: '/api/works/91/download',
            created_at: '2026-04-28T10:01:00Z'
          }
        ]
      })
    mockReferenceAssets()
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 12,
      status: 'queued',
      stage: 'queued',
      available_credits: 4
    })
    apiMocks.getImageGeneration.mockResolvedValueOnce({
      generation_id: 12,
      work_id: 91,
      status: 'succeeded',
      stage: 'succeeded',
      prompt: 'new bamboo lake',
      preview_url: '/api/works/91/file',
      download_url: '/api/works/91/download',
      available_credits: 3
    })

    const wrapper = await mountReady()
    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('new bamboo lake')
    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()
    expect(window.sessionStorage.getItem(activeGenerationStorageKey)).not.toBeNull()

    await vi.advanceTimersByTimeAsync(1000)
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(wrapper.get('.preview-image').attributes('src')).toBe('/api/works/91/file')
    expect(window.sessionStorage.getItem(activeGenerationStorageKey)).toBeNull()

    wrapper.unmount()
    mountedWrappers.splice(mountedWrappers.indexOf(wrapper), 1)
    mockUser({ available_credits: 5 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 13,
      status: 'queued',
      stage: 'queued',
      available_credits: 4
    })
    apiMocks.getImageGeneration.mockResolvedValueOnce({
      generation_id: 13,
      status: 'failed',
      stage: 'failed',
      available_credits: 4,
      error: { message: '后端明确失败' }
    })

    const failedWrapper = await mountReady()
    await failedWrapper.get('[data-testid="workspace-prompt-input"]').setValue('failed bamboo lake')
    await failedWrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()
    await vi.advanceTimersByTimeAsync(1000)
    await flushPromises()
    await failedWrapper.vm.$nextTick()

    expect(failedWrapper.get('[data-testid="workspace-generation-failure-notice"]').text()).toContain('图片生成失败，请稍后再试')
    expect(failedWrapper.get('[data-testid="workspace-result-error"]').text()).toContain('生成失败')
    expect(failedWrapper.text()).not.toContain('后端明确失败')
    expect(window.sessionStorage.getItem(activeGenerationStorageKey)).toBeNull()
  })

  it('shows failed generation reasons and retries with the previous prompt', async () => {
    vi.useFakeTimers()
    mockUser()
    mockWorks()
    apiMocks.createImageGeneration
      .mockResolvedValueOnce({
        generation_id: 12,
        status: 'queued',
        stage: 'queued',
        available_credits: 3
      })
      .mockResolvedValueOnce({
        generation_id: 13,
        status: 'queued',
        stage: 'queued',
        available_credits: 3
      })
    apiMocks.getImageGeneration.mockResolvedValueOnce({
      generation_id: 12,
      status: 'failed',
      stage: 'failed',
      available_credits: 3,
      error: {
        code: 'provider_timeout',
        message: '图片服务响应超时，系统已自动重试 2 次仍未完成，请稍后重新生成。',
        retryable: true
      }
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('mist over bamboo lake')
    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()
    await vi.advanceTimersByTimeAsync(1000)
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="workspace-generation-failure-notice"]').text()).toContain('网络超时，生成失败')
    expect(wrapper.get('[data-testid="workspace-generation-failure-notice"]').text()).toContain('点击重试')
    expect(wrapper.get('[data-testid="workspace-result-error"]').text()).not.toContain('图片服务响应超时')
    await wrapper.get('[data-testid="workspace-failure-retry-generation"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledTimes(2)
    expect(apiMocks.createImageGeneration).toHaveBeenLastCalledWith(expect.objectContaining({
      prompt: 'mist over bamboo lake',
      aspect_ratio: '1:1',
      tool_mode: 'generate'
    }))
  })

  it('switches the preview when a history item is selected', async () => {
    mockUser()
    mockWorks([
      {
        work_id: 90,
        prompt: 'first work',
        preview_url: '/api/works/90/file',
        download_url: '/api/works/90/download',
        created_at: '2026-04-28T10:00:00Z'
      },
      {
        work_id: 91,
        prompt: 'second work',
        preview_url: '/api/works/91/file',
        download_url: '/api/works/91/download',
        created_at: '2026-04-28T10:01:00Z'
      }
    ])

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tab-create"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.get('.preview-image').attributes('src')).toBe('/api/works/90/file')
    await wrapper.findAll('.history-card')[1].trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.get('.preview-image').attributes('src')).toBe('/api/works/91/file')
    expect(wrapper.text()).toContain('second work')
  })

  it('loads and renders only image media in the image workspace history', async () => {
    mockUser()
    mockWorks([
      makeWork(80, {
        prompt: 'video should stay out of image workspace',
        category: 'video',
        mime_type: 'video/mp4',
        preview_url: '/api/works/80/file'
      }),
      makeWork(81, {
        prompt: 'plain image stays visible',
        category: 'image',
        preview_url: '/api/works/81/file'
      }),
      makeWork(82, {
        prompt: 'audio should stay out of image workspace',
        category: 'audio',
        mime_type: 'audio/mpeg',
        preview_url: '/api/works/82/file'
      }),
      makeWork(84, {
        prompt: 'poster image stays visible',
        category: 'poster_kv',
        preview_url: '/api/works/84/file'
      }),
      makeWork(83, {
        prompt: 'legacy image without preview stays selectable',
        category: '',
        preview_url: ''
      })
    ], { total: 4, page: 1, page_size: 18 })
    mockReferenceAssets()
    mockDiscovery()

    const wrapper = await mountReady()

    expect(apiMocks.listWorks).toHaveBeenCalledWith({
      media_type: 'image',
      page: 1,
      page_size: 18
    })

    await wrapper.get('[data-testid="workspace-tab-create"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.findAll('.history-card')).toHaveLength(3)
    expect(wrapper.text()).toContain('plain image stays visible')
    expect(wrapper.text()).toContain('poster image stays visible')
    expect(wrapper.text()).toContain('legacy image without preview stays selectable')
    expect(wrapper.text()).not.toContain('video should stay out of image workspace')
    expect(wrapper.text()).not.toContain('audio should stay out of image workspace')

    expect(wrapper.get('.preview-image').attributes('src')).toBe('/api/works/81/file')

    const historyCards = wrapper.findAll('.history-card')
    expect(historyCards[2].find('.history-card-image img').exists()).toBe(false)
    expect(historyCards[2].text()).toContain('暂无预览')

    await historyCards[0].find('[data-testid="workspace-history-use-as-reference"]').trigger('click')
    expect(wrapper.get('[data-testid="workspace-reference-upload"]').text()).toContain('plain image stays visible')
  })

  it('keeps failure retry controls inside the centered result stage', async () => {
    vi.useFakeTimers()
    mockUser({ available_credits: 5 })
    mockWorks()
    mockReferenceAssets()
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 12,
      status: 'queued',
      stage: 'queued',
      available_credits: 3
    })
    apiMocks.getImageGeneration.mockResolvedValueOnce({
      generation_id: 12,
      status: 'failed',
      stage: 'failed',
      available_credits: 3,
      error: {
        code: 'provider_timeout',
        message: '图片服务响应超时，请稍后重新生成。',
        retryable: true
      }
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('mist over bamboo lake')
    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()
    await vi.advanceTimersByTimeAsync(1000)
    await flushPromises()
    await wrapper.vm.$nextTick()

    const stage = wrapper.get('[data-testid="workspace-result-stage"]')
    expect(wrapper.get('[data-testid="workspace-generation-failure-notice"]').text()).toContain('网络超时，生成失败')
    expect(stage.get('[data-testid="workspace-result-error"]').text()).toContain('生成失败')
    expect(stage.get('[data-testid="workspace-result-error"]').text()).not.toContain('图片服务响应超时')
    expect(stage.get('[data-testid="workspace-result-retry-generation"]').text()).toContain('重新生成')

    vi.useRealTimers()
  })

  it('restores one-time workspace prefill from reused works and removes it after reading', async () => {
    window.sessionStorage.setItem('image_agent_workspace_prefill:v1', JSON.stringify({
      prompt: '复用作品提示词',
      negative_prompt: '不要低清晰度',
      aspect_ratio: '16:9',
      style_preset: '写实',
      tool_mode: 'generate',
      model_id: 7,
      reference_work_id: 55
    }))
    mockUser({ available_credits: 5 })
    mockWorks([
      {
        work_id: 55,
        prompt: '原作品',
        aspect_ratio: '1:1',
        category: 'image',
        preview_url: '/api/works/55/file',
        created_at: '2026-05-01T01:00:00Z'
      }
    ])
    mockReferenceAssets()
    mockDiscovery()

    const wrapper = await mountReady()

    expect(wrapper.get('[data-testid="workspace-discovery-panel"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="workspace-create-panel"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="workspace-tab-create"]').classes()).toContain('active')

    await wrapper.get('[data-testid="workspace-tab-create"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="workspace-prompt-input"]').element.value).toBe('复用作品提示词')
    expect(wrapper.get('[data-testid="workspace-size-select"]').element.value).toBe('16:9')
    const advancedPanel = await openHomeAdvancedPanel(wrapper)
    expect(homeAdvancedStyleChip(advancedPanel, '写实').classList.contains('active')).toBe(true)
    expect(wrapper.get('[data-testid="workspace-tab-create"]').classes()).toContain('active')
    expect(wrapper.get('[data-testid="workspace-create-panel"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="workspace-reference-upload"]').text()).toContain('原作品')
    expect(window.sessionStorage.getItem('image_agent_workspace_prefill:v1')).toBeNull()
  })

  it('saves workspace drafts locally, restores them, and keeps the draft after a successful generation for follow-up edits', async () => {
    vi.useFakeTimers()
    mockUser({ available_credits: 5 })
    mockWorks()
    mockReferenceAssets()
    mockDiscovery()
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 88,
      status: 'queued',
      stage: 'queued',
      available_credits: 4
    })
    apiMocks.getImageGeneration.mockResolvedValueOnce({
      generation_id: 88,
      status: 'succeeded',
      work_id: 90,
      preview_url: '/api/works/90/file',
      prompt: '草稿提示词'
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('草稿提示词')
    await wrapper.get('[data-testid="workspace-size-select"]').setValue('4:3')
    await flushPromises()

    expect(window.localStorage.getItem('image_agent_workspace_draft:v1')).toContain('草稿提示词')

    wrapper.unmount()
    mockUser({ available_credits: 5 })
    mockWorks()
    mockReferenceAssets()
    mockDiscovery()
    const restored = await mountReady()

    expect(restored.get('[data-testid="workspace-discovery-panel"]').exists()).toBe(true)
    expect(restored.find('[data-testid="workspace-create-panel"]').exists()).toBe(false)
    expect(restored.get('[data-testid="workspace-tab-create"]').classes()).toContain('active')

    await restored.get('[data-testid="workspace-tab-create"]').trigger('click')
    await restored.vm.$nextTick()

    expect(restored.get('[data-testid="workspace-prompt-input"]').element.value).toBe('草稿提示词')
    expect(restored.get('[data-testid="workspace-size-select"]').element.value).toBe('4:3')
    expect(restored.get('[data-testid="workspace-create-panel"]').exists()).toBe(true)

    await restored.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()
    await vi.advanceTimersByTimeAsync(1000)
    await flushPromises()

    expect(window.localStorage.getItem('image_agent_workspace_draft:v1')).toContain('草稿提示词')
    vi.useRealTimers()
  })

  it('shows direct result actions for regenerating, referencing, favoriting, sharing, and opening the library', async () => {
    mockUser({ available_credits: 5 })
    mockWorks([
      {
        work_id: 91,
        prompt: '结果作品',
        aspect_ratio: '1:1',
        category: 'image',
        visibility: 'private',
        preview_url: '/api/works/91/file',
        download_url: '/api/works/91/download',
        created_at: '2026-05-01T01:00:00Z'
      }
    ])
    mockReferenceAssets()
    mockDiscovery()
    apiMocks.updateWork
      .mockResolvedValueOnce({ work_id: 91, is_favorite: true })
      .mockResolvedValueOnce({ work_id: 91, visibility: 'public' })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tab-create"]').trigger('click')
    expect(wrapper.get('[data-testid="workspace-result-regenerate"]').text()).toContain('再次生成')
    expect(wrapper.get('[data-testid="workspace-result-use-reference"]').text()).toContain('作为参考图')
    expect(wrapper.get('[data-testid="workspace-result-favorite"]').text()).toContain('收藏')
    expect(wrapper.get('[data-testid="workspace-result-share"]').text()).toContain('分享')

    await wrapper.get('[data-testid="workspace-result-use-reference"]').trigger('click')
    expect(wrapper.get('[data-testid="workspace-reference-upload"]').text()).toContain('结果作品')

    await wrapper.get('[data-testid="workspace-result-favorite"]').trigger('click')
    await flushPromises()
    expect(apiMocks.updateWork).toHaveBeenCalledWith(91, { is_favorite: true })

    await wrapper.get('[data-testid="workspace-result-share"]').trigger('click')
    await flushPromises()
    expect(apiMocks.updateWork).toHaveBeenCalledWith(91, { visibility: 'public' })
    expect(routerPush).toHaveBeenCalledWith('/works/share?ids=91')

    await wrapper.get('[data-testid="workspace-result-open-library"]').trigger('click')
    expect(routerPush).toHaveBeenCalledWith('/works')
  })

  it('shows immediate loading feedback while submitting regenerate from the current result', async () => {
    mockUser({ available_credits: 5 })
    mockWorks([
      {
        work_id: 91,
        prompt: '结果作品',
        aspect_ratio: '1:1',
        category: 'image',
        visibility: 'private',
        preview_url: '/api/works/91/file',
        download_url: '/api/works/91/download',
        created_at: '2026-05-01T01:00:00Z',
        parameters: {
          prompt: '结果作品',
          aspect_ratio: '1:1',
          tool_mode: 'generate'
        }
      }
    ])
    mockReferenceAssets()
    mockDiscovery()
    const pendingRegenerate = deferred()
    apiMocks.createImageGeneration.mockReturnValueOnce(pendingRegenerate.promise)

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tab-create"]').trigger('click')
    await wrapper.get('[data-testid="workspace-result-regenerate"]').trigger('click')
    await wrapper.vm.$nextTick()

    const regenerateButton = wrapper.get('[data-testid="workspace-result-regenerate"]')
    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: '结果作品',
      aspect_ratio: '1:1',
      tool_mode: 'generate'
    }))
    expect(regenerateButton.text()).toContain('提交中')
    expect(regenerateButton.attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="workspace-result-stage"]').text()).toContain('正在提交再次生成')
  })

  it('creates and selects a new task after regenerating from a result', async () => {
    mockUser({ available_credits: 5 })
    mockWorks([
      {
        work_id: 91,
        prompt: '结果作品',
        negative_prompt: '低清',
        aspect_ratio: '4:3',
        category: 'image',
        visibility: 'private',
        preview_url: '/api/works/91/file',
        created_at: '2026-05-01T01:00:00Z',
        parameters: {
          prompt: '结果作品',
          negative_prompt: '低清',
          aspect_ratio: '4:3',
          style_preset: '写实',
          tool_mode: 'generate'
        }
      }
    ])
    mockReferenceAssets()
    mockDiscovery()
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 108,
      status: 'queued',
      stage: 'queued',
      created_at: '2026-06-06T10:08:00Z',
      prompt: '结果作品',
      parameters: {
        prompt: '结果作品',
        negative_prompt: '低清',
        aspect_ratio: '4:3',
        style_preset: '写实',
        tool_mode: 'generate'
      },
      available_credits: 4
    })

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tab-create"]').trigger('click')
    await wrapper.get('[data-testid="workspace-result-regenerate"]').trigger('click')
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(apiMocks.createImageGeneration).toHaveBeenCalledWith(expect.objectContaining({
      prompt: '结果作品',
      negative_prompt: '低清',
      aspect_ratio: '4:3',
      style_preset: '写实',
      tool_mode: 'generate'
    }))
    expect(wrapper.get('[data-testid="workspace-generation-task-108"]').classes()).toContain('active')
    expect(wrapper.get('[data-testid="workspace-generation-tasks"]').text()).toContain('生成任务 (1)')
    expect(wrapper.get('[data-testid="workspace-result-stage"]').text()).toContain('正在排队')
  })

  it('keeps the current preview visible and reports regenerate submission failures', async () => {
    mockUser({ available_credits: 5 })
    mockWorks([
      {
        work_id: 91,
        prompt: '结果作品',
        aspect_ratio: '1:1',
        category: 'image',
        preview_url: '/api/works/91/file',
        created_at: '2026-05-01T01:00:00Z'
      }
    ])
    mockReferenceAssets()
    mockDiscovery()
    apiMocks.createImageGeneration.mockRejectedValueOnce(new Error('服务暂时不可用'))

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tab-create"]').trigger('click')
    await wrapper.get('[data-testid="workspace-result-regenerate"]').trigger('click')
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-testid="workspace-result-regenerate"]').text()).toContain('再次生成')
    expect(wrapper.get('[data-testid="workspace-result-regenerate"]').attributes('disabled')).toBeUndefined()
    expect(wrapper.get('.preview-image').attributes('src')).toBe('/api/works/91/file')
    expect(wrapper.get('[data-testid="workspace-result-regenerate-error"]').text()).toContain('再次生成提交失败')
  })

  it('does not submit regenerate when the result has no prompt', async () => {
    mockUser({ available_credits: 5 })
    mockWorks([
      {
        work_id: 91,
        prompt: '',
        aspect_ratio: '1:1',
        category: 'image',
        preview_url: '/api/works/91/file',
        created_at: '2026-05-01T01:00:00Z'
      }
    ])
    mockReferenceAssets()
    mockDiscovery()

    const wrapper = await mountReady()

    await wrapper.get('[data-testid="workspace-tab-create"]').trigger('click')
    await wrapper.get('[data-testid="workspace-result-regenerate"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(apiMocks.createImageGeneration).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="workspace-result-regenerate-error"]').text()).toContain('该作品缺少提示词')
    expect(wrapper.get('.preview-image').attributes('src')).toBe('/api/works/91/file')
  })

  it('regenerates only the current preview while existing running tasks continue polling', async () => {
    vi.useFakeTimers()
    mockUser({ available_credits: 8 })
    apiMocks.listWorks.mockResolvedValueOnce({
      items: [
        {
          work_id: 91,
          prompt: '结果作品',
          aspect_ratio: '1:1',
          category: 'image',
          preview_url: '/api/works/91/file',
          created_at: '2026-05-01T01:00:00Z'
        }
      ]
    })
    mockReferenceAssets()
    mockDiscovery()
    apiMocks.createImageGeneration
      .mockResolvedValueOnce({
        generation_id: 101,
        status: 'queued',
        stage: 'queued',
        prompt: '先运行的任务',
        parameters: { prompt: '先运行的任务', aspect_ratio: '1:1', tool_mode: 'generate' },
        available_credits: 7
      })
      .mockResolvedValueOnce({
        generation_id: 109,
        status: 'queued',
        stage: 'queued',
        prompt: '结果作品',
        parameters: { prompt: '结果作品', aspect_ratio: '1:1', tool_mode: 'generate' },
        available_credits: 6
      })
    apiMocks.getImageGeneration.mockImplementation(async (id) => ({
      generation_id: id,
      status: 'running',
      stage: 'requesting_provider',
      prompt: id === 101 ? '先运行的任务' : '结果作品',
      available_credits: 6
    }))

    const wrapper = await mountReady()

    await submitWorkspacePrompt(wrapper, '先运行的任务')
    await wrapper.get('[data-testid="workspace-generation-task-101"]').trigger('click')
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="workspace-generation-task-101"]').classes()).toContain('active')

    await wrapper.get('.history-card').trigger('click')
    await wrapper.vm.$nextTick()
    await wrapper.get('[data-testid="workspace-result-regenerate"]').trigger('click')
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(apiMocks.createImageGeneration).toHaveBeenLastCalledWith(expect.objectContaining({
      prompt: '结果作品',
      aspect_ratio: '1:1',
      tool_mode: 'generate'
    }))
    expect(wrapper.get('[data-testid="workspace-generation-task-109"]').classes()).toContain('active')

    apiMocks.getImageGeneration.mockClear()
    await vi.advanceTimersByTimeAsync(1000)
    await flushPromises()

    expect(apiMocks.getImageGeneration).toHaveBeenCalledWith(101)
    expect(apiMocks.getImageGeneration).toHaveBeenCalledWith(109)
    vi.useRealTimers()
  })
})
