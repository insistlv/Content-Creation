import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  listAdminPromptTemplates: vi.fn(),
  createAdminPromptTemplate: vi.fn(),
  updateAdminPromptTemplate: vi.fn(),
  deleteAdminPromptTemplate: vi.fn(),
  generateAdminPromptTemplatePreview: vi.fn(),
  batchGenerateAdminPromptTemplatePreviews: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: apiMocks
}))

import AdminPromptTemplatesView from '../views/AdminPromptTemplatesView.vue'

const listPayload = {
  items: [
    {
      id: 1,
      slug: 'city-sleepless-index',
      title: '城市失眠指数海报',
      category: '数据海报',
      description: '把城市夜景和数据结合',
      prompt: '城市失眠指数数据海报',
      aspect_ratio: '4:3',
      style_preset: '海报',
      theme: 'city-data',
      workspace_section: 'hot',
      workspace_tool_mode: 'generate',
      workspace_sort: 20,
      preview_url: 'https://oss.example.com/city.png',
      preview_generated_at: '2026-05-16T10:00:00Z',
      sort_order: 20,
      is_active: true,
      cost_credits: 1
    },
    {
      id: 2,
      slug: 'missing-preview',
      title: '缺失预览模板',
      category: '测试',
      description: '没有预览图',
      prompt: '测试提示词',
      aspect_ratio: '1:1',
      style_preset: '',
      theme: 'test',
      workspace_section: 'inspiration',
      workspace_tool_mode: 'remove_background',
      workspace_sort: 30,
      preview_url: '',
      preview_status: 'failed',
      preview_error_message: '系统繁忙，请稍后再试',
      sort_order: 30,
      is_active: false,
      cost_credits: 1
    }
  ],
  total: 2,
  page: 1,
  page_size: 12
}

function mockInitialLoad() {
  apiMocks.listAdminPromptTemplates.mockResolvedValue(listPayload)
}

describe('AdminPromptTemplatesView', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('renders prompt templates and preview status from the admin API', async () => {
    mockInitialLoad()
    const wrapper = mount(AdminPromptTemplatesView)
    await flushPromises()

    expect(apiMocks.listAdminPromptTemplates).toHaveBeenCalledWith({ page: 1, page_size: 12 })
    expect(wrapper.text()).toContain('提示词模板')
    expect(wrapper.text()).toContain('城市失眠指数海报')
    expect(wrapper.text()).toContain('缺失预览模板')
    expect(wrapper.text()).toContain('已生成')
    expect(wrapper.text()).toContain('工作台 热门')
    expect(wrapper.text()).toContain('generate')
    expect(wrapper.text()).toContain('生成失败')
    expect(wrapper.text()).toContain('系统繁忙，请稍后再试')
    expect(wrapper.find('[data-testid="template-preview-1"]').attributes('src')).toBe('https://oss.example.com/city.png')
  })

  it('creates, edits, deletes, and reloads prompt templates', async () => {
    mockInitialLoad()
    apiMocks.createAdminPromptTemplate.mockResolvedValue({ id: 9 })
    apiMocks.updateAdminPromptTemplate.mockResolvedValue({ id: 1 })
    apiMocks.deleteAdminPromptTemplate.mockResolvedValue({ ok: true })
    vi.spyOn(window, 'confirm').mockReturnValue(true)

    const wrapper = mount(AdminPromptTemplatesView)
    await flushPromises()

    await wrapper.get('[data-testid="new-template"]').trigger('click')
    const createModal = wrapper.get('[data-testid="prompt-template-modal"]')
    expect(createModal.text()).toContain('新增提示词模板')
    expect(createModal.text()).toContain('预览 / 规格')
    expect(createModal.text()).toContain('暂无预览图')
    expect(createModal.text()).toContain('保存并生成预览')
    await wrapper.get('[data-testid="template-title"]').setValue('新模板')
    await wrapper.get('[data-testid="template-slug"]').setValue('new-template')
    await wrapper.get('[data-testid="template-workspace-section"]').setValue('hot')
    await wrapper.get('[data-testid="template-workspace-tool-mode"]').setValue('remove_background')
    await wrapper.get('[data-testid="template-workspace-sort"]').setValue(15)
    await wrapper.get('[data-testid="template-prompt"]').setValue('新模板提示词')
    await wrapper.get('[data-testid="save-template"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createAdminPromptTemplate).toHaveBeenCalledWith(
      expect.objectContaining({ title: '新模板', slug: 'new-template', prompt: '新模板提示词' })
    )
    expect(apiMocks.createAdminPromptTemplate).toHaveBeenCalledWith(expect.objectContaining({
      workspace_section: 'hot',
      workspace_tool_mode: 'remove_background',
      workspace_sort: 15
    }))

    await wrapper.get('[data-testid="edit-template-1"]').trigger('click')
    await wrapper.get('[data-testid="template-title"]').setValue('更新模板')
    await wrapper.get('[data-testid="save-template"]').trigger('click')
    await flushPromises()

    expect(apiMocks.updateAdminPromptTemplate).toHaveBeenCalledWith(1, expect.objectContaining({ title: '更新模板' }))

    await wrapper.get('[data-testid="delete-template-1"]').trigger('click')
    await flushPromises()

    expect(apiMocks.deleteAdminPromptTemplate).toHaveBeenCalledWith(1)
  })

  it('saves a new prompt template and queues preview generation from the modal', async () => {
    mockInitialLoad()
    apiMocks.createAdminPromptTemplate.mockResolvedValue({ id: 9 })
    apiMocks.generateAdminPromptTemplatePreview.mockResolvedValue({
      status: 'queued',
      queued: 1,
      template_ids: [9]
    })

    const wrapper = mount(AdminPromptTemplatesView)
    await flushPromises()

    await wrapper.get('[data-testid="new-template"]').trigger('click')
    await wrapper.get('[data-testid="template-title"]').setValue('预览模板')
    await wrapper.get('[data-testid="template-slug"]').setValue('preview-template')
    await wrapper.get('[data-testid="template-category"]').setValue('数据海报')
    await wrapper.get('[data-testid="template-aspect-ratio"]').setValue('4:3')
    await wrapper.get('[data-testid="template-style-preset"]').setValue('海报')
    await wrapper.get('[data-testid="template-theme"]').setValue('city-data')
    await wrapper.get('[data-testid="template-workspace-section"]').setValue('inspiration')
    await wrapper.get('[data-testid="template-workspace-tool-mode"]').setValue('expand')
    await wrapper.get('[data-testid="template-workspace-sort"]').setValue(9)
    await wrapper.get('[data-testid="template-prompt"]').setValue('生成一张城市数据海报')
    await wrapper.get('[data-testid="save-template-and-generate-preview"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createAdminPromptTemplate).toHaveBeenCalledWith(expect.objectContaining({
      title: '预览模板',
      slug: 'preview-template',
      category: '数据海报',
      aspect_ratio: '4:3',
      style_preset: '海报',
      theme: 'city-data',
      workspace_section: 'inspiration',
      workspace_tool_mode: 'expand',
      workspace_sort: 9,
      prompt: '生成一张城市数据海报'
    }))
    expect(apiMocks.generateAdminPromptTemplatePreview).toHaveBeenCalledWith(9, { force: true })
    expect(wrapper.text()).toContain('已开始生成 1 张预览')
  })

  it('generates single and missing preview images from the admin page', async () => {
    mockInitialLoad()
    apiMocks.generateAdminPromptTemplatePreview.mockResolvedValue({
      status: 'queued',
      queued: 1,
      template_ids: [2]
    })
    apiMocks.batchGenerateAdminPromptTemplatePreviews.mockResolvedValue({
      status: 'queued',
      queued: 3,
      template_ids: [1, 2, 3]
    })

    const wrapper = mount(AdminPromptTemplatesView)
    await flushPromises()

    await wrapper.get('[data-testid="generate-template-preview-2"]').trigger('click')
    await flushPromises()

    expect(apiMocks.generateAdminPromptTemplatePreview).toHaveBeenCalledWith(2, { force: true })
    expect(wrapper.text()).toContain('已开始生成 1 张预览')

    await wrapper.get('[data-testid="generate-missing-previews"]').trigger('click')
    await flushPromises()

    expect(apiMocks.batchGenerateAdminPromptTemplatePreviews).toHaveBeenCalledWith({ limit: 12 })
    expect(wrapper.text()).toContain('已开始生成 3 张预览')
  })
})
