import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import CommerceSKUManager from '../components/ecommerce/CommerceSKUManager.vue'
import CommerceGenerationConfigurator from '../components/ecommerce/CommerceGenerationConfigurator.vue'
import skuManagerSource from '../components/ecommerce/CommerceSKUManager.vue?raw'

const skus = [
  { id: 5, code: 'DEFAULT', status: 'active', specification: '默认' },
  { id: 6, code: 'BLUE-M', status: 'active', specification: '蓝色 / M' }
]

describe('CommerceSKUManager', () => {
  it('用中文显示默认规格并支持两维预览和确认', async () => {
    const wrapper = mount(CommerceSKUManager, { props: { skus, defaultSkuId: 5, config: { version: 2, dimensions: [], values: [], skus } } })
    expect(wrapper.text()).toContain('默认规格')
    await wrapper.get('[data-testid="sku-mode-multiple"]').trigger('click')
    await wrapper.get('[data-testid="sku-add-dimension"]').trigger('click')
    const names = wrapper.findAll('[data-testid="sku-dimension-name"]')
    await names[0].setValue('颜色')
    await wrapper.findAll('[data-testid="sku-dimension-values"]')[0].setValue('红色, 蓝色')
    await wrapper.get('[data-testid="sku-add-dimension"]').trigger('click')
    await wrapper.findAll('[data-testid="sku-dimension-name"]')[1].setValue('尺寸')
    await wrapper.findAll('[data-testid="sku-dimension-values"]')[1].setValue('S, M')
    await wrapper.get('[data-testid="sku-preview"]').trigger('click')
    expect(wrapper.emitted('preview')[0][0]).toEqual({ expected_version: 2, dimensions: [
      { name: '颜色', values: [{ name: '红色' }, { name: '蓝色' }], sort_order: 0 },
      { name: '尺寸', values: [{ name: 'S' }, { name: 'M' }], sort_order: 1 }
    ] })
    await wrapper.setProps({ preview: { add: [{ key: '1' }, { key: '2' }, { key: '3' }, { key: '4' }], keep: [], disable: [], conflicts: [] } })
    expect(wrapper.text()).toContain('4 个组合')
    await wrapper.get('[data-testid="sku-apply"]').trigger('click')
    expect(wrapper.emitted('apply')).toHaveLength(1)
  })

  it('前端阻止超过三维、每维二十值和一百组合', async () => {
    const wrapper = mount(CommerceSKUManager, { props: { skus, defaultSkuId: 5, config: { version: 0, dimensions: [], values: [], skus } } })
    await wrapper.get('[data-testid="sku-mode-multiple"]').trigger('click')
    for (let i = 0; i < 3; i += 1) await wrapper.get('[data-testid="sku-add-dimension"]').trigger('click')
    expect(wrapper.get('[data-testid="sku-add-dimension"]').attributes('disabled')).toBeDefined()
    for (const input of wrapper.findAll('[data-testid="sku-dimension-name"]')) if (!input.element.value) await input.setValue('维度')
    await wrapper.findAll('[data-testid="sku-dimension-values"]')[1].setValue('一')
    await wrapper.findAll('[data-testid="sku-dimension-values"]')[2].setValue('一')
    await wrapper.findAll('[data-testid="sku-dimension-values"]')[0].setValue(Array.from({ length: 21 }, (_, i) => `值${i}`).join(','))
    await wrapper.get('[data-testid="sku-preview"]').trigger('click')
    expect(wrapper.text()).toContain('每个维度最多 20 个值')
    expect(wrapper.emitted('preview')).toBeUndefined()
  })

  it('拒绝每维合法但总数为 105 的组合', async () => {
    const wrapper = mount(CommerceSKUManager, { props: { skus, defaultSkuId: 5, config: { version: 0, dimensions: [], values: [], skus } } })
    await wrapper.get('[data-testid="sku-mode-multiple"]').trigger('click')
    for (let i = 0; i < 3; i += 1) await wrapper.get('[data-testid="sku-add-dimension"]').trigger('click')
    for (const [index, count] of [3, 5, 7].entries()) {
      await wrapper.findAll('[data-testid="sku-dimension-name"]')[index].setValue(`维度${index}`)
      await wrapper.findAll('[data-testid="sku-dimension-values"]')[index].setValue(Array.from({ length: count }, (_, n) => `值${n}`).join(','))
    }
    await wrapper.get('[data-testid="sku-preview"]').trigger('click')
    expect(wrapper.text()).toContain('规格组合最多 100 个')
  })

  it('成功预览冻结快照，编辑后预览失效且应用只提交冻结请求', async () => {
    const wrapper = mount(CommerceSKUManager, { props: { skus, defaultSkuId: 5, config: { version: 2, dimensions: [], values: [], skus } } })
    await wrapper.get('[data-testid="sku-mode-multiple"]').trigger('click'); await wrapper.get('[data-testid="sku-add-dimension"]').trigger('click')
    await wrapper.get('[data-testid="sku-dimension-name"]').setValue('颜色'); await wrapper.get('[data-testid="sku-dimension-values"]').setValue('红,蓝')
    await wrapper.get('[data-testid="sku-preview"]').trigger('click')
    await wrapper.setProps({ preview: { add: [{ key: '红' }], keep: [{ key: '蓝' }], disable: [{ key: '旧' }], conflicts: [] } })
    expect(wrapper.text()).toContain('新增 1、保留 1、停用 1')
    await wrapper.get('[data-testid="sku-apply"]').trigger('click')
    expect(wrapper.emitted('apply')[0][0]).toMatchObject({ expected_version: 2, dimensions: [{ name: '颜色' }] })
    await wrapper.get('[data-testid="sku-dimension-values"]').setValue('绿')
    expect(wrapper.find('[data-testid="sku-apply"]').exists()).toBe(false)
  })

  it('单规格切换先预览空维度矩阵再明确确认应用', async () => {
    const config = { version: 3, dimensions: [{ id: 1, name: '颜色', status: 'active' }], values: [{ id: 1, dimension_id: 1, name: '红', status: 'active' }], skus }
    const wrapper = mount(CommerceSKUManager, { props: { skus, defaultSkuId: 5, config } })
    await wrapper.get('[data-testid="sku-mode-single"]').trigger('click')
    expect(wrapper.emitted('preview').at(-1)[0]).toEqual({ expected_version: 3, dimensions: [] })
    await wrapper.setProps({ preview: { add: [], keep: [{ key: 'default' }], disable: [{ key: 'red' }], conflicts: [] } })
    await wrapper.get('[data-testid="sku-apply"]').trigger('click')
    expect(wrapper.emitted('apply').at(-1)[0]).toEqual({ expected_version: 3, dimensions: [] })
  })

  it('手机窄屏为单列可滚动语义，关键按钮可操作且可获得键盘焦点', async () => {
    vi.stubGlobal('innerWidth', 390)
    vi.stubGlobal('matchMedia', vi.fn(query => ({ matches: query.includes('767px'), media: query, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
    const wrapper = mount(CommerceSKUManager, { attachTo: document.body, props: { skus, defaultSkuId: 5, config: { version: 0, dimensions: [], values: [], skus } } })
    expect(skuManagerSource).toContain('@media(max-width:900px)')
    expect(skuManagerSource).toContain('grid-template-columns:1fr')
    expect(skuManagerSource).toContain('overflow:visible')
    const multi = wrapper.get('[data-testid="sku-mode-multiple"]'); multi.element.focus()
    expect(document.activeElement).toBe(multi.element)
    await multi.trigger('click'); await wrapper.get('[data-testid="sku-add-dimension"]').trigger('click')
    expect(wrapper.get('[data-testid="sku-preview"]').element.disabled).toBe(false)
    wrapper.unmount(); vi.unstubAllGlobals()
  })

  it('编辑编码、设置主规格和停用恢复均发出中文语义事件', async () => {
    const wrapper = mount(CommerceSKUManager, { props: { skus, defaultSkuId: 5, config: { version: 1, dimensions: [], values: [], skus } } })
    await wrapper.get('[data-testid="sku-code-6"]').setValue('BLUE-L')
    await wrapper.get('[data-testid="sku-save-6"]').trigger('click')
    await wrapper.get('[data-testid="sku-default-6"]').trigger('click')
    await wrapper.get('[data-testid="sku-status-5"]').trigger('click')
    expect(wrapper.emitted('patch')[0][0]).toEqual({ id: 6, input: { code: 'BLUE-L' } })
    expect(wrapper.emitted('set-default')[0][0]).toBe(6)
    expect(wrapper.text()).toContain('主规格不能停用')
  })
})

describe('CommerceGenerationConfigurator SKU 约束', () => {
  it('至少选择一个规格且主规格必须在选中集合', async () => {
    const wrapper = mount(CommerceGenerationConfigurator, { props: {
      recipe: { key: 'product_detail_set' }, definition: {}, specConfirmed: true,
      selectedSections: [], skus, selectedSkuIds: [5], primarySkuId: 5
    } })
    await wrapper.get('[data-testid="generation-sku-6"]').setValue(true)
    await wrapper.setProps({ selectedSkuIds: [5, 6] })
    await wrapper.get('[data-testid="generation-primary-sku"]').setValue('6')
    expect(wrapper.emitted('skus').at(-1)[0]).toEqual([5, 6])
    expect(wrapper.emitted('primary-sku').at(-1)[0]).toBe(6)
    await wrapper.get('[data-testid="generation-sku-5"]').setValue(false)
    await wrapper.setProps({ selectedSkuIds: [6], primarySkuId: 6 })
    await wrapper.get('[data-testid="generation-sku-6"]').setValue(false)
    await wrapper.setProps({ selectedSkuIds: [] })
    expect(wrapper.text()).toContain('至少选择一个规格')
  })

  it('SKU 与章节配置在桌面和手机语义中同时存在', () => {
    const wrapper = mount(CommerceGenerationConfigurator, { props: { recipe: { key: 'product_detail_set' }, definition: { sections: ['hero'] }, specConfirmed: true, selectedSections: ['hero'], skus, selectedSkuIds: [5], primarySkuId: 5 } })
    expect(wrapper.findAll('fieldset').length).toBeGreaterThanOrEqual(2)
    expect(wrapper.text()).toContain('生成规格')
    expect(wrapper.text()).toContain('选择详情页章节')
  })
})
