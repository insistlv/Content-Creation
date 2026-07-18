import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it } from 'vitest'

import CommerceResultPanel from '../components/ecommerce/CommerceResultPanel.vue'

const definition = {
  sections: ['hero', 'detail'],
  aspect_ratios: ['1:1'],
  section_options: [
    { value: 'hero', label: '首屏主视觉' },
    { value: 'detail', label: '细节展示' },
  ],
}

const batch = {
  id: 1,
  status: 'partial_succeeded',
  total_items: 2,
  reserved_credits: 6,
  settled_credits: 3,
  released_credits: 3,
  eta_seconds: 28,
  items: [
    { id: 2, status: 'succeeded', slot_key: 'product_detail_set:v1:sku-1:hero', work_id: 9, output_snapshot: { output_size: '1024x1280' } },
    { id: 3, status: 'failed', slot_key: 'product_detail_set:v1:sku-1:detail', error_code: 'provider_policy_rejected' },
  ],
}

const mountedWrappers = []

function mountPanel(props = {}) {
  const wrapper = mount(CommerceResultPanel, {
    attachTo: document.body,
    props: {
      definition,
      batches: [batch],
      assets: [{ id: 1 }, { id: 2 }, { id: 3 }],
      creativeSpec: { status: 'confirmed', product_facts: { material: '不锈钢' }, forbidden_changes: ['不改变杯盖'] },
      selectedSections: ['hero', 'detail'],
      aspectRatio: '4:5',
      qualityTier: 'high_fidelity',
      layoutTemplate: 'brand_band',
      estimate: { estimated_credits: 6 },
      events: [{ id: 8, event_type: 'item_succeeded', created_at: '2026-07-11T11:42:00Z' }],
      currentProject: { id: 7, title: '便携式水杯' },
      ...props,
    },
  })
  mountedWrappers.push(wrapper)
  return wrapper
}

afterEach(() => {
  while (mountedWrappers.length) mountedWrappers.pop().unmount()
  document.body.style.overflow = ''
})

describe('CommerceResultPanel', () => {
  it('默认以双列生产控制台展示五类业务卡片', () => {
    const wrapper = mountPanel()
    expect(wrapper.get('[data-testid="commerce-production-console"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid^="commerce-console-card-"]')).toHaveLength(5)
    expect(wrapper.text()).toContain('任务队列')
    expect(wrapper.text()).toContain('实时事件')
    expect(wrapper.text()).toContain('输入与约束')
    expect(wrapper.text()).toContain('成本与进度')
    expect(wrapper.text()).toContain('最新结果')
    expect(wrapper.text()).toContain('首屏主视觉')
    expect(wrapper.get('[data-testid="commerce-console-card-costs"]').text()).toContain('已结算3 点')
    expect(wrapper.get('a[href="/api/works/9/file"]').text()).toContain('打开预览')
  })

  it('没有项目或批次时展示生产准备检查而不是大型案例', () => {
    const wrapper = mountPanel({ currentProject: null, batches: [], assets: [], creativeSpec: null, estimate: null })
    expect(wrapper.get('[data-testid="commerce-console-readiness"]').text()).toContain('生产准备检查')
    expect(wrapper.text()).toContain('创建商品项目')
    expect(wrapper.find('.case-library').exists()).toBe(false)
    expect(wrapper.find('.history-list').exists()).toBe(false)
  })

  it('案例库和历史记录仅在对应弹窗打开后显示', async () => {
    const wrapper = mountPanel()
    expect(document.body.querySelector('[data-testid="commerce-cases-dialog"]')).toBeNull()
    expect(document.body.querySelector('[data-testid="commerce-history-dialog"]')).toBeNull()

    await wrapper.get('[data-testid="commerce-open-cases"]').trigger('click')
    expect(document.body.querySelector('[data-testid="commerce-cases-dialog"]')).not.toBeNull()
    expect(document.body.textContent).toContain('高转化商品详情')
    document.body.querySelector('[data-testid="commerce-dialog-close"]').click()
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="commerce-open-history"]').trigger('click')
    expect(document.body.querySelector('[data-testid="commerce-history-dialog"]')).not.toBeNull()
    expect(document.body.textContent).toContain('批次 #1')
  })

  it('将当前主题传递给案例、历史和全屏 Teleport 弹窗', async () => {
    const wrapper = mountPanel({ theme: 'light' })
    expect(wrapper.get('[data-testid="commerce-production-console"]').attributes('data-theme')).toBe('light')

    await wrapper.get('[data-testid="commerce-open-cases"]').trigger('click')
    expect(document.body.querySelector('[data-testid="commerce-cases-dialog"]')?.dataset.theme).toBe('light')
    document.body.querySelector('[data-testid="commerce-dialog-close"]').click()
    await wrapper.vm.$nextTick()

    await wrapper.get('[data-testid="commerce-open-fullscreen"]').trigger('click')
    const fullscreen = document.body.querySelector('[data-testid="commerce-fullscreen-console"]')
    expect(fullscreen?.dataset.theme).toBe('light')
    await wrapper.setProps({ theme: 'dark' })
    expect(fullscreen?.dataset.theme).toBe('dark')
  })

  it('历史记录保留取消和重试事件', async () => {
    const wrapper = mountPanel({ batches: [{ ...batch, status: 'running' }] })
    await wrapper.get('[data-testid="commerce-open-history"]').trigger('click')
    const history = document.body.querySelector('[data-testid="commerce-history-dialog"]')
    ;[...history.querySelectorAll('button')].find((button) => button.textContent.includes('取消批次')).click()
    ;[...history.querySelectorAll('button')].find((button) => button.textContent.includes('重试')).click()
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('cancel-batch')?.[0][0].id).toBe(1)
    expect(wrapper.emitted('retry-item')?.[0][0].id).toBe(3)
  })

  it('全屏控制台锁定滚动并支持 Esc 退出和焦点返回', async () => {
    const wrapper = mountPanel()
    const trigger = wrapper.get('[data-testid="commerce-open-fullscreen"]')
    trigger.element.focus()
    await trigger.trigger('click')
    expect(document.body.style.overflow).toBe('hidden')
    expect(document.body.querySelector('[data-testid="commerce-fullscreen-console"]')).not.toBeNull()

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wrapper.vm.$nextTick()
    expect(document.body.querySelector('[data-testid="commerce-fullscreen-console"]')).toBeNull()
    expect(document.body.style.overflow).toBe('')
    expect(document.activeElement).toBe(trigger.element)
  })

  it('不向控制台泄露后端英文错误和未知内部状态', () => {
    const wrapper = mountPanel({
      error: 'upstream timeout: provider unavailable',
      batches: [{ id: 1, status: 'future_batch_state', items: [{ id: 2, status: 'failed', section: 'future_section', error_message: 'unsafe content rejected by provider' }] }],
    })
    for (const token of ['upstream', 'timeout', 'provider unavailable', 'unsafe content', 'future_batch_state', 'future_section']) {
      expect(wrapper.text()).not.toContain(token)
    }
    expect(wrapper.text()).toContain('未知状态')
  })

  it('按持久进度计算批次均值并将终态归一为 100', () => {
    const wrapper = mount(CommerceResultPanel, { props: {
      mode: 'history',
      batches: [{ id: 8, status: 'running', items: [
        { id: 1, status: 'running', progress_percent: 40 },
        { id: 2, status: 'succeeded', progress_percent: 73 }
      ] }]
    } })
    expect(wrapper.text()).toContain('70%')
    expect(wrapper.text()).toContain('生成中 · 40%')
    expect(wrapper.text()).toContain('已完成 · 100%')
  })
})
