import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import CommerceCategorySelector from '../components/ecommerce/CommerceCategorySelector.vue'

const catalog = {
  version: 'cn-commerce-v1',
  system_categories: [
    { id: 1, name: '家居日用', source: 'system', children: [{ id: 11, parent_id: 1, source: 'system', name: '杯壶餐具', path: '家居日用 / 杯壶餐具', aliases: ['水杯', '保温杯'] }] },
    { id: 2, name: '美妆个护', source: 'system', children: [{ id: 21, parent_id: 2, source: 'system', name: '护肤', path: '美妆个护 / 护肤', aliases: ['面霜'] }] }
  ],
  custom_categories: [{ id: 31, parent_id: 1, source: 'user', name: '咖啡器具', path: '家居日用 / 咖啡器具', status: 'active' }],
  recent_categories: [{ id: 11, parent_id: 1, source: 'system', name: '杯壶餐具', path: '家居日用 / 杯壶餐具' }]
}

describe('CommerceCategorySelector', () => {
  it('按一级分组选择二级品类，并通过别名搜索', async () => {
    const wrapper = mount(CommerceCategorySelector, { props: { catalog } })
    await wrapper.get('[data-testid="category-trigger"]').trigger('click')
    expect(wrapper.text()).toContain('最近使用')
    await wrapper.get('[data-testid="category-search"]').setValue('保温杯')
    expect(wrapper.text()).toContain('家居日用 / 杯壶餐具')
    await wrapper.get('[data-category-id="system-11"]').trigger('click')
    expect(wrapper.emitted('select')[0][0]).toMatchObject({ id: 11, source: 'system', path: '家居日用 / 杯壶餐具' })
  })

  it('搜索无结果时在当前一级类目新增个人品类', async () => {
    const wrapper = mount(CommerceCategorySelector, { props: { catalog } })
    await wrapper.get('[data-testid="category-trigger"]').trigger('click')
    await wrapper.get('[data-testid="category-search"]').setValue('手冲器具')
    await wrapper.get('[data-testid="category-create-custom"]').trigger('click')
    expect(wrapper.emitted('create-custom')[0][0]).toEqual({ parent_id: 1, name: '手冲器具' })
  })

  it('管理个人品类时发出改名、停用和恢复请求', async () => {
    const wrapper = mount(CommerceCategorySelector, { props: { catalog } })
    await wrapper.get('[data-testid="category-trigger"]').trigger('click')
    await wrapper.get('[data-testid="category-manage-custom"]').trigger('click')
    await wrapper.get('[data-testid="category-custom-name-31"]').setValue('手冲咖啡器具')
    await wrapper.get('[data-testid="category-custom-save-31"]').trigger('click')
    await wrapper.get('[data-testid="category-custom-toggle-31"]').trigger('click')
    expect(wrapper.emitted('patch-custom')[0][0]).toEqual({ id: 31, input: { name: '手冲咖啡器具' } })
    expect(wrapper.emitted('patch-custom')[1][0]).toEqual({ id: 31, input: { status: 'inactive' } })
  })

	it('支持方向键和回车选择搜索结果', async () => {
	  const wrapper = mount(CommerceCategorySelector, { props: { catalog } })
	  await wrapper.get('[data-testid="category-trigger"]').trigger('click')
	  const search = wrapper.get('[data-testid="category-search"]')
	  await search.setValue('水杯')
	  await search.trigger('keydown', { key: 'ArrowDown' })
	  await search.trigger('keydown', { key: 'Enter' })
	  expect(wrapper.emitted('select')[0][0]).toMatchObject({ id: 11, source: 'system' })
	})
})
