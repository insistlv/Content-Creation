import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import CommerceReportEditor from '../components/ecommerce/CommerceReportEditor.vue'

describe('CommerceReportEditor real contract', () => {
  it('使用 name 并为所有 missing_fields 动态创建补录输入', async () => {
    const wrapper = mount(CommerceReportEditor, { props: { spec: { id: 1, status: 'draft', version: 1, user_overrides: { name: '杯子' }, missing_fields: ['material', 'capacity'] } } })
    expect(wrapper.get('[data-field="name"]').element.value).toBe('杯子')
    expect(wrapper.get('[data-field="material"]').exists()).toBe(true)
    expect(wrapper.get('[data-field="capacity"]').exists()).toBe(true)
  })
  it('切换 spec 时清空旧表单，analysis_failed 仍允许手工补录', async () => {
    const wrapper = mount(CommerceReportEditor, { props: { spec: { id: 1, status: 'draft', version: 1, user_overrides: { name: '旧商品', material: '陶瓷' }, missing_fields: [] } } })
    await wrapper.setProps({ spec: { id: 2, status: 'analysis_failed', version: 1, user_overrides: {}, missing_fields: ['name'] } })
    expect(wrapper.get('[data-field="name"]').element.value).toBe('')
    expect(wrapper.text()).toContain('分析失败')
    expect(wrapper.get('[data-testid="commerce-report-save"]').attributes('disabled')).toBeUndefined()
  })
  it('报告有未保存修改时执行保存并确认，不直接确认旧版本', async () => {
    const wrapper = mount(CommerceReportEditor, { props: { spec: { id: 1, status: 'draft', version: 1, user_overrides: { name: '旧名称' }, missing_fields: [] } } })
    await wrapper.get('[data-field="name"]').setValue('新名称')
    expect(wrapper.text()).toContain('有未保存修改')
    await wrapper.get('[data-testid="commerce-report-confirm"]').trigger('click')
    expect(wrapper.emitted('save-confirm')?.[0][0]).toMatchObject({ user_overrides: { name: '新名称' } })
    expect(wrapper.emitted('confirm')).toBeUndefined()
  })
  it('报告文案字段与 missing facts 分离为真实 PATCH payload', async () => {
    const wrapper = mount(CommerceReportEditor, { props: { spec: { id: 1, status: 'draft', version: 1, user_overrides: { material: '陶瓷' }, missing_fields: ['material'], selling_points: ['防漏','便携'], forbidden_changes: ['杯盖结构'], brand_tone: { description: '简约' } } } })
    await wrapper.get('[data-field="material"]').setValue('不锈钢')
    await wrapper.get('[data-field="selling_points"]').setValue('耐用\n便携')
    await wrapper.get('[data-field="forbidden_changes"]').setValue('不得改变杯盖')
    await wrapper.get('[data-field="brand_tone"]').setValue('科技感')
    await wrapper.get('[data-testid="commerce-report-save"]').trigger('click')
    expect(wrapper.emitted('save')?.[0][0]).toEqual({
      user_overrides: { material: '不锈钢' }, selling_points: ['耐用','便携'], forbidden_changes: ['不得改变杯盖'], brand_tone: { description: '科技感' }
    })
  })

  it('事实、缺失项和表单字段只显示中文字段名，未知字段不回显英文', () => {
    const wrapper = mount(CommerceReportEditor, { props: { spec: {
      id: 1, status: 'draft', version: 1,
      observed_facts: [
        { field: 'color', value: '蓝色', confidence: 0.9 },
        { field: 'internal_unknown_field', value: '内部值', confidence: 0.8 }
      ],
      missing_fields: ['capacity', 'backend_secret_field']
    } } })
    expect(wrapper.text()).toContain('颜色')
    expect(wrapper.text()).toContain('容量')
    expect(wrapper.text()).toContain('其他商品信息')
    for (const token of ['color', 'capacity', 'internal_unknown_field', 'backend_secret_field']) {
      expect(wrapper.text()).not.toContain(token)
    }
  })
})
