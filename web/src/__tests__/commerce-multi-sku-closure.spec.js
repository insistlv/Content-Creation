import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import CommerceGenerationConfigurator from '../components/ecommerce/CommerceGenerationConfigurator.vue'
import CommerceResultPanel from '../components/ecommerce/CommerceResultPanel.vue'

const definition = {
  sections: ['hero', 'selling_points', 'detail'],
  section_options: [
    { value: 'hero', label: '首屏主视觉' },
    { value: 'selling_points', label: '核心卖点' },
    { value: 'detail', label: '细节展示' }
  ],
  section_scopes: {
    hero: { scope: 'sku' },
    selling_points: { scope: 'shared' },
    detail: { scope: 'sku', configurable: true }
  }
}

describe('多 SKU 生成闭环', () => {
  it('按 Definition 展示中文作用域，且只有 configurable 章节可切换', async () => {
    const wrapper = mount(CommerceGenerationConfigurator, { props: {
      recipe: { key: 'product_detail_set' }, definition, specConfirmed: true,
      selectedSections: definition.sections, sectionScopes: { detail: 'shared' },
      skus: [{ id: 1, code: 'DEFAULT', status: 'active' }, { id: 2, code: 'RED-L', status: 'active' }],
      selectedSkuIds: [1, 2], primarySkuId: 2
    } })
    expect(wrapper.get('[data-testid="section-scope-hero"]').text()).toContain('按规格生成')
    expect(wrapper.get('[data-testid="section-scope-selling_points"]').text()).toContain('公共内容')
    expect(wrapper.get('[data-testid="section-scope-detail"]').text()).toContain('公共内容')
    expect(wrapper.find('[data-testid="section-scope-hero"] select').exists()).toBe(false)
    await wrapper.get('[data-testid="section-scope-detail"] select').setValue('sku')
    expect(wrapper.emitted('section-scopes')?.at(-1)[0]).toEqual({ detail: 'sku' })
  })

  it('估价明确展示公共、规格、总图、点数和 ETA', () => {
    const wrapper = mount(CommerceGenerationConfigurator, { props: {
      recipe: { key: 'product_detail_set' }, definition, specConfirmed: true,
      selectedSections: definition.sections, sectionScopes: {}, skus: [], selectedSkuIds: [],
      estimate: { shared_items: 2, sku_items: 4, total_items: 6, estimated_credits: 18, eta_seconds: 42 }
    } })
    for (const text of ['公共任务 2', '规格任务 4', '总图片 6', '总点数 18', '42 秒']) expect(wrapper.text()).toContain(text)
  })

  it('按公共内容和 SKU 快照分组，DEFAULT 仅显示默认规格', () => {
    const wrapper = mount(CommerceResultPanel, { props: { mode: 'history', definition, batches: [{
      id: 9, status: 'partial_succeeded', items: [
        { id: 1, scope: 'shared', section: 'selling_points', status: 'succeeded', estimated_credits: 2 },
        { id: 2, scope: 'sku', section: 'hero', status: 'succeeded', sku_snapshot: { code: 'DEFAULT', specification_path: '默认 / 默认' }, estimated_credits: 3 },
        { id: 3, scope: 'sku', section: 'detail', status: 'failed', sku_code: 'RED-L', specification_path: '红色 / L', estimated_credits: 4 }
      ]
    }] } })
    expect(wrapper.text()).toContain('公共内容')
    expect(wrapper.text()).toContain('默认规格')
    expect(wrapper.text()).not.toContain('DEFAULT')
    expect(wrapper.text()).toContain('红色 / L')
    expect(wrapper.text()).toContain('RED-L')
    expect(wrapper.text()).toContain('4 点')
  })
})
