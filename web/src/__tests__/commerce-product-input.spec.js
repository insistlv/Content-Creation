import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import CommerceProductInput from '../components/ecommerce/CommerceProductInput.vue'
describe('CommerceProductInput project isolation', () => {
  it('切换和新建项目时恢复或清空全部项目表单字段', async () => {
    const wrapper = mount(CommerceProductInput, { props: { product: { id: 1, name: '商品A', category: '家居', sku_code: 'A-1', user_requirements: '突出材质' } } })
    expect(wrapper.get('[data-field="sku_code"]').element.value).toBe('A-1')
    expect(wrapper.get('[data-field="user_requirements"]').element.value).toBe('突出材质')
    await wrapper.setProps({ product: { id: 2, name: '商品B', category: '服饰' } })
    expect(wrapper.get('[data-field="sku_code"]').element.value).toBe('')
    expect(wrapper.get('[data-field="user_requirements"]').element.value).toBe('')
    await wrapper.setProps({ product: null })
    expect(wrapper.get('[data-field="title"]').element.value).toBe('')
  })

  it('显示中文 SKU 说明并把适用规格传给上传流程', async () => {
    const wrapper = mount(CommerceProductInput, { props: {
      product: { id: 1, name: '商品A' }, categorySelection: { id: 1, name: '杯子' },
      skus: [{ id: 5, code: 'DEFAULT', status: 'active' }, { id: 6, code: 'BLUE', status: 'active' }]
    } })
    expect(wrapper.text()).toContain('商品规格编码（SKU，可选）')
    expect(wrapper.text()).toContain('不填将自动生成，创建后仍可修改')
    await wrapper.get('[data-testid="asset-sku-product_front"]').setValue('6')
    await wrapper.findAllComponents({ name: 'ImageUploadZone' })[0].vm.$emit('upload', new File(['x'], 'a.png'))
    expect(wrapper.emitted('upload')[0][1]).toMatchObject({ role: 'product_front', sku_id: 6 })
  })
})
