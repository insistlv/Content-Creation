import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import AICommerceWorkspaceView from '../views/AICommerceWorkspaceView.vue'
import CommerceRecipeSelector from '../components/ecommerce/CommerceRecipeSelector.vue'
import commerceCreatorShellSource from '../components/ecommerce/CommerceCreatorShell.vue?raw'
import { useUserTheme } from '../composables/useUserTheme.js'

const api = vi.hoisted(() => ({
  getCommerceCapabilities: vi.fn(), listCommerceProjects: vi.fn(), listCommerceRecipes: vi.fn(),
	listCommerceCategories: vi.fn(),
  listCommerceAssets: vi.fn(), listCommerceBatches: vi.fn(), getCommerceBatch: vi.fn(),
  listCommerceBatchEvents: vi.fn(), getCredits: vi.fn()
}))
vi.mock('../api/client.js', () => ({ api }))

describe('AICommerceWorkspaceView A 方案', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    window.localStorage.removeItem('image_agent_user_theme:v1')
    api.getCommerceCapabilities.mockResolvedValue({ enabled: true, worker_enabled: true })
    api.listCommerceProjects.mockResolvedValue({ items: [] })
	api.listCommerceCategories.mockResolvedValue({ version: 'cn-commerce-v1', system_categories: [], custom_categories: [], recent_categories: [] })
    api.listCommerceRecipes.mockResolvedValue({ items: [{ key: 'product_detail_set', version: 1,
      sections: ['hero', 'detail'], aspect_ratios: ['3:4', '4:5'], quality_tiers: ['standard'],
      layout_templates: ['clean'], parameters: { layout_template: 'clean' }, allowed_output_counts: [1, 2],
      section_options: [{ value: 'hero', label: '首屏主视觉' }, { value: 'detail', label: '细节展示' }],
      quality_options: [{ value: 'standard', label: '标准' }],
      layout_template_options: [{ value: 'clean', label: '简洁留白' }]
    }] })
    api.listCommerceAssets.mockResolvedValue({ items: [] })
    api.listCommerceBatches.mockResolvedValue({ items: [] })
    api.getCredits.mockResolvedValue({ balance: 88 })
  })

  it('跟随共享主题实时更新工作台和结果控制台', async () => {
    window.localStorage.setItem('image_agent_user_theme:v1', 'light')
    const wrapper = mount(AICommerceWorkspaceView)
    await flushPromises()

    expect(wrapper.get('[data-testid="commerce-creator-shell"]').attributes('data-theme')).toBe('light')
    expect(wrapper.get('[data-testid="commerce-production-console"]').attributes('data-theme')).toBe('light')

    useUserTheme().toggleTheme()
    await nextTick()
    expect(wrapper.get('[data-testid="commerce-creator-shell"]').attributes('data-theme')).toBe('dark')
    expect(wrapper.get('[data-testid="commerce-production-console"]').attributes('data-theme')).toBe('dark')
  })

  it('为 AI 电商主要表面定义亮暗主题语义变量', () => {
    expect(commerceCreatorShellSource).toContain('.commerce-creator-shell[data-theme="light"]')
    for (const token of ['--commerce-bg', '--commerce-surface', '--commerce-input', '--commerce-text', '--commerce-border']) {
      expect(commerceCreatorShellSource).toContain(token)
    }
  })

  it('桌面和平板端顶部导航吸顶且手机端恢复普通文档流', async () => {
    const wrapper = mount(AICommerceWorkspaceView)
    await flushPromises()

    expect(wrapper.get('[data-testid="commerce-creator-topbar"]').exists()).toBe(true)
    expect(commerceCreatorShellSource).toMatch(/\.creator-topbar\{[^}]*position:sticky;[^}]*top:0;[^}]*z-index:\d+/)
    expect(commerceCreatorShellSource).toMatch(/\.result-pane\{[^}]*top:var\(--commerce-topbar-offset\)/)
    expect(commerceCreatorShellSource).toMatch(/@media\(max-width:767px\)\{[^]*?\.creator-topbar\{[^}]*position:static/)
  })

  it('渲染黑曜双栏创作台并默认展示案例库，不出现旧五页签或 prompt 建档', async () => {
    const prompt = vi.spyOn(window, 'prompt')
    const wrapper = mount(AICommerceWorkspaceView)
    await flushPromises()
    expect(wrapper.get('[data-testid="commerce-creator-shell"]').classes()).toContain('commerce-creator-shell')
    expect(wrapper.get('[data-testid="commerce-create-pane"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="commerce-result-pane"]').text()).toContain('案例库')
    expect(wrapper.text()).toContain('AI 商品详情页')
    expect(wrapper.text()).not.toContain('商品与 SKU')
    expect(wrapper.find('.commerce-tabs').exists()).toBe(false)
    expect(prompt).not.toHaveBeenCalled()
  })

  it('Recipe 选项只渲染后端 Definition，未知 Recipe 不渲染', async () => {
    api.listCommerceRecipes.mockResolvedValue({ items: [
      { key: 'mystery_tool', version: 9, aspect_ratios: ['99:1'] },
      { key: 'product_detail_set', version: 1, sections: ['hero'], aspect_ratios: ['4:5'], quality_tiers: ['fine'], layout_templates: ['clean'], parameters: { layout_template: 'clean' } }
    ] })
    const wrapper = mount(AICommerceWorkspaceView)
    await flushPromises()
    expect(wrapper.text()).toContain('4:5')
    expect(wrapper.text()).toContain('未知质量档')
    expect(wrapper.text()).not.toContain('fine')
    expect(wrapper.text()).not.toContain('99:1')
    expect(wrapper.text()).not.toContain('mystery_tool')
  })

  it('使用 Definition 中文标签展示选项且不泄露内部英文值', async () => {
    api.listCommerceRecipes.mockResolvedValue({ items: [{
      key: 'product_detail_set', version: 1,
      sections: ['hero', 'selling_points'], aspect_ratios: ['4:5'],
      quality_tiers: ['high_fidelity'], layout_templates: ['brand_band'],
      parameters: { layout_template: 'brand_band' },
      section_options: [{ value: 'hero', label: '首屏主视觉' }, { value: 'selling_points', label: '核心卖点' }],
      quality_options: [{ value: 'high_fidelity', label: '高清' }],
      layout_template_options: [{ value: 'brand_band', label: '品牌色带' }]
    }] })
    const wrapper = mount(AICommerceWorkspaceView)
    await flushPromises()
    expect(wrapper.text()).toContain('AI 电商')
    expect(wrapper.text()).toContain('首屏主视觉')
    expect(wrapper.text()).toContain('高清')
    expect(wrapper.text()).toContain('品牌色带')
    for (const token of [
      'hero', 'selling_points', 'material', 'detail', 'usage', 'specification', 'closing',
      'standard', 'high_fidelity', 'clean', 'dark_gradient', 'brand_band', 'ETA', 'AI COMMERCE',
      'internal_recipe_key', 'backend_secret_field', 'upstream', 'gateway', 'timeout',
      'provider unavailable', 'database connection', 'unknown_backend_failure',
    ]) expect(wrapper.text()).not.toContain(token)
    expect(wrapper.text()).not.toContain('Recipe')
  })

  it('旧生产方案入口也不显示 Recipe 英文标识', () => {
    const wrapper = mount(CommerceRecipeSelector, { props: { recipes: [] } })
    expect(wrapper.text()).toContain('生产方案尚未开放')
    expect(wrapper.text()).not.toContain('Recipe')
  })

  it('旧生产方案入口不回显内部 key', () => {
    const wrapper = mount(CommerceRecipeSelector, { props: { recipes: [{ key: 'internal_recipe_key' }] } })
    expect(wrapper.text()).toContain('未命名生成方案')
    expect(wrapper.text()).not.toContain('internal_recipe_key')
  })

  it('工作台初始化失败时不显示英文后端错误', async () => {
    api.getCommerceCapabilities.mockRejectedValueOnce(Object.assign(new Error('upstream gateway timeout'), { code: 'unknown_backend_failure' }))
    const wrapper = mount(AICommerceWorkspaceView)
    await flushPromises()
    expect(wrapper.text()).toContain('AI 电商工作台加载失败')
    expect(wrapper.text()).not.toContain('upstream gateway timeout')
    expect(wrapper.text()).not.toContain('unknown_backend_failure')
  })

  it('手机端提供创作/结果页签', async () => {
    const wrapper = mount(AICommerceWorkspaceView)
    await flushPromises()
    const tabs = wrapper.findAll('[data-testid^="commerce-mobile-tab-"]')
    expect(tabs.map((tab) => tab.text())).toEqual(['创作', '结果'])
    await tabs[1].trigger('click')
    expect(tabs[1].attributes('aria-selected')).toBe('true')
  })

  it('feature disabled 时严格锁死页面且不加载任何业务 API', async () => {
    api.getCommerceCapabilities.mockResolvedValueOnce({ enabled: false, worker_enabled: false })
    const wrapper = mount(AICommerceWorkspaceView)
    await flushPromises()
    expect(wrapper.text()).toContain('AI 电商未开启')
    expect(wrapper.find('[data-testid="commerce-create-pane"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="commerce-result-pane"]').exists()).toBe(false)
    expect(api.listCommerceProjects).not.toHaveBeenCalled()
    expect(api.listCommerceRecipes).not.toHaveBeenCalled()
    expect(api.getCredits).not.toHaveBeenCalled()
    expect(api.listCommerceAssets).not.toHaveBeenCalled()
    expect(api.listCommerceBatches).not.toHaveBeenCalled()
  })
})
