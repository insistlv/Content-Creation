import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AdminCommerceCategoriesView from '../views/AdminCommerceCategoriesView.vue'

const mocks = vi.hoisted(() => ({
  list: vi.fn(), create: vi.fn(), patch: vi.fn()
}))
vi.mock('../api/client.js', () => ({ api: {
  listAdminCommerceCategories: mocks.list,
  createAdminCommerceCategory: mocks.create,
  patchAdminCommerceCategory: mocks.patch
} }))

describe('AdminCommerceCategoriesView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.list.mockResolvedValue({ version: 'cn-commerce-v1', items: [
      { id: 1, level: 1, name: '家居日用', aliases: ['家居百货'], sort_order: 10, status: 'active' },
      { id: 11, parent_id: 1, level: 2, name: '杯壶餐具', aliases: ['水杯'], sort_order: 10, status: 'active' }
    ] })
    mocks.create.mockResolvedValue({ id: 2 })
    mocks.patch.mockResolvedValue({ id: 11 })
  })

  it('展示两级目录并支持新增、编辑和停用', async () => {
    const wrapper = mount(AdminCommerceCategoriesView)
    await flushPromises()
    expect(wrapper.text()).toContain('家居日用')
    expect(wrapper.text()).toContain('杯壶餐具')
    await wrapper.get('[data-testid="category-add-root"]').trigger('click')
    await wrapper.get('[data-testid="category-form-name"]').setValue('测试大类')
    await wrapper.get('[data-testid="category-form-submit"]').trigger('click')
    expect(mocks.create).toHaveBeenCalledWith(expect.objectContaining({ level: 1, name: '测试大类' }))
    await wrapper.get('[data-testid="category-toggle-11"]').trigger('click')
    expect(mocks.patch).toHaveBeenCalledWith(11, { status: 'inactive' })
  })
})
