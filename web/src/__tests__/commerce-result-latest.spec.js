import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import CommerceResultPanel from '../components/ecommerce/CommerceResultPanel.vue'

describe('CommerceResultPanel 最新结果', () => {
  it('优先展示服务端返回的最新批次作品', () => {
    const wrapper = mount(CommerceResultPanel, { props: {
      batches: [
        { id: 20, status: 'succeeded', items: [{ id: 201, status: 'succeeded', work_id: 2001, section: 'hero' }] },
        { id: 10, status: 'succeeded', items: [{ id: 101, status: 'succeeded', work_id: 1001, section: 'detail' }] },
      ],
      definition: { section_options: [{ value: 'hero', label: '首屏主视觉' }, { value: 'detail', label: '细节展示' }] },
    } })
    const latestCard = wrapper.get('[data-testid="commerce-console-card-latest"]')
    expect(latestCard.get('a[href="/api/works/2001/file"]').exists()).toBe(true)
    expect(latestCard.find('a[href="/api/works/1001/file"]').exists()).toBe(false)
  })

  it('同一批次内展示 ID 最大的最后成功作品', () => {
    const wrapper = mount(CommerceResultPanel, { props: {
      batches: [{ id: 20, status: 'succeeded', items: [
        { id: 201, status: 'succeeded', work_id: 2001, section: 'hero' },
        { id: 202, status: 'succeeded', work_id: 2002, section: 'detail' },
      ] }],
      definition: { section_options: [{ value: 'hero', label: '首屏主视觉' }, { value: 'detail', label: '细节展示' }] },
    } })
    const latestCard = wrapper.get('[data-testid="commerce-console-card-latest"]')
    expect(latestCard.get('a[href="/api/works/2002/file"]').exists()).toBe(true)
    expect(latestCard.find('a[href="/api/works/2001/file"]').exists()).toBe(false)
  })
})
