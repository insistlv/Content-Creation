import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import CommerceBatchCenter from '../components/ecommerce/CommerceBatchCenter.vue'

const api = vi.hoisted(() => ({
  listCommerceBatches: vi.fn(),
  getCommerceBatch: vi.fn(),
  listCommerceBatchEvents: vi.fn(),
  cancelCommerceBatch: vi.fn(),
  cancelCommerceItem: vi.fn(),
  retryCommerceItem: vi.fn()
}))
vi.mock('../api/client.js', () => ({ api }))

describe('CommerceBatchCenter', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    Object.values(api).forEach((mock) => mock.mockReset())
    api.listCommerceBatches.mockResolvedValue([{ id: 9, status: 'running', item_count: 2, held_credits: 4, settled_credits: 1, released_credits: 0 }])
    api.getCommerceBatch.mockResolvedValue({ id: 9, status: 'running', items: [] })
    api.listCommerceBatchEvents.mockResolvedValue([{ id: 11, type: 'batch_running' }])
  })
  afterEach(() => vi.useRealTimers())

  it('从服务端恢复运行批次并用文字与图标表达状态', async () => {
    const wrapper = mount(CommerceBatchCenter, { props: { projectId: 3 } })
    await vi.runOnlyPendingTimersAsync()
    expect(api.listCommerceBatches).toHaveBeenCalledWith(3)
    expect(wrapper.get('[data-testid="commerce-batch-status"]').text()).toContain('运行中')
    expect(wrapper.get('[data-testid="commerce-batch-status"]').attributes('aria-label')).toContain('运行中')
    expect(wrapper.text()).toContain('预占 4 点')
    expect(wrapper.text()).toContain('已结算')
    expect(wrapper.text()).toContain('已释放')
    for (const token of ['item', 'held', 'settled', 'released', 'ETA']) {
      expect(wrapper.text()).not.toContain(token)
    }
  })

  it('使用 after_id 增量拉取事件并在卸载时清理轮询', async () => {
    const clearSpy = vi.spyOn(window, 'clearTimeout')
    const wrapper = mount(CommerceBatchCenter, { props: { projectId: 3 } })
    await vi.runOnlyPendingTimersAsync()
    await vi.advanceTimersByTimeAsync(2000)
    expect(api.listCommerceBatchEvents).toHaveBeenCalledWith(9, { after_id: 11 })
    wrapper.unmount()
    expect(clearSpy).toHaveBeenCalled()
  })

  it('polls every 2 seconds while visible and every 10 seconds while hidden', async () => {
    const timeout = vi.spyOn(window, 'setTimeout')
    const original = document.visibilityState
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
    const wrapper = mount(CommerceBatchCenter, { props: { projectId: 3 } })
    await vi.runOnlyPendingTimersAsync()
    expect(timeout).toHaveBeenCalledWith(expect.any(Function), 2000)
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'hidden' })
    await vi.advanceTimersByTimeAsync(2000)
    expect(timeout).toHaveBeenCalledWith(expect.any(Function), 10000)
    wrapper.unmount()
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: original })
  })

  it('switches projects without allowing stale responses to overwrite the new project', async () => {
    let resolveOld
    api.listCommerceBatches
      .mockImplementationOnce(() => new Promise((resolve) => { resolveOld = resolve }))
      .mockResolvedValueOnce([{ id: 22, status: 'succeeded', total_items: 1 }])
    api.getCommerceBatch.mockResolvedValueOnce({ batch: { id: 22, status: 'succeeded' }, items: [] })
    const wrapper = mount(CommerceBatchCenter, { props: { projectId: 1 } })
    await wrapper.setProps({ projectId: 2 })
    await vi.waitFor(() => expect(wrapper.text()).toContain('#22'))
    resolveOld([{ id: 11, status: 'running' }])
    await Promise.resolve()
    expect(wrapper.text()).toContain('#22')
    expect(wrapper.text()).not.toContain('#11')
  })

  it('cancels active items and retries failed items once with a stable idempotency key', async () => {
    api.getCommerceBatch.mockResolvedValue({ batch: { id: 9, status: 'running' }, items: [
      { id: 31, status: 'queued' }, { id: 32, status: 'retrying' }, { id: 33, status: 'failed' }
    ] })
    api.cancelCommerceItem.mockResolvedValue({})
    api.retryCommerceItem.mockImplementation(() => new Promise(() => {}))
    const wrapper = mount(CommerceBatchCenter, { props: { projectId: 3 } })
    await vi.runOnlyPendingTimersAsync()
    await wrapper.get('[data-testid="commerce-item-cancel-31"]').trigger('click')
    expect(api.cancelCommerceItem).toHaveBeenCalledWith(31)
    const retry = wrapper.get('[data-testid="commerce-item-retry-33"]')
    await retry.trigger('click'); await retry.trigger('click')
    expect(api.retryCommerceItem).toHaveBeenCalledTimes(1)
    expect(api.retryCommerceItem).toHaveBeenCalledWith(33, expect.stringMatching(/^commerce-retry-33-/))
  })

  it('显示中文结果状态、预计时间、点数核算和鉴权下载链接', async () => {
    api.listCommerceBatches.mockResolvedValue([{ id: 9, status: 'succeeded', total_items: 1, reserved_credits: 4, settled_credits: 3, released_credits: 1, eta_seconds: 0 }])
    api.getCommerceBatch.mockResolvedValue({ batch: { id: 9, status: 'succeeded' }, items: [{ id: 40, status: 'succeeded', work_id: 77 }] })
    const wrapper = mount(CommerceBatchCenter, { props: { projectId: 3 } })
    await vi.runOnlyPendingTimersAsync()
    expect(wrapper.text()).toContain('已完成')
    expect(wrapper.text()).toContain('预占 4')
    expect(wrapper.text()).toContain('已结算 3')
    expect(wrapper.text()).toContain('已释放 1')
    expect(wrapper.text()).toContain('预计剩余时间：已结束')
    for (const token of ['item', 'held', 'settled', 'released', 'ETA']) {
      expect(wrapper.text()).not.toContain(token)
    }
    expect(wrapper.get('[data-testid="commerce-item-download-40"]').attributes('href')).toBe('/api/works/77/download')
  })

  it('使用可信的运行中预计时间并从快照恢复已取消批次的下载', async () => {
    api.listCommerceBatches.mockResolvedValue([
      { id: 50, status: 'running', eta_seconds: 47 },
      { id: 51, status: 'canceled', eta_seconds: 0 }
    ])
    api.getCommerceBatch
      .mockResolvedValueOnce({ batch: { id: 50, status: 'running', eta_seconds: 47 }, items: [] })
      .mockResolvedValueOnce({ batch: { id: 51, status: 'canceled' }, items: [{ id: 61, status: 'succeeded', work_id: 88 }] })
    const wrapper = mount(CommerceBatchCenter, { props: { projectId: 3 } })
    await flushPromises()
    expect(api.getCommerceBatch).toHaveBeenCalledWith(51)
    expect(wrapper.text()).toContain('预计剩余时间：47 秒')
    expect(wrapper.text()).not.toContain('ETA')
    expect(wrapper.get('[data-testid="commerce-item-download-61"]').attributes('href')).toBe('/api/works/88/download')
  })

  it('部分完成属于终态，未知批次和条目状态不回显原始值', async () => {
    api.listCommerceBatches.mockResolvedValue([
      { id: 70, status: 'partial_succeeded', eta_seconds: 0 },
      { id: 71, status: 'future_batch_status', eta_seconds: 0, items: [{ id: 72, status: 'future_item_status' }] }
    ])
    api.getCommerceBatch
      .mockResolvedValueOnce({ batch: { id: 70, status: 'partial_succeeded' }, items: [] })
    const wrapper = mount(CommerceBatchCenter, { props: { projectId: 3 } })
    await flushPromises()
    expect(wrapper.text()).toContain('部分完成')
    expect(wrapper.text()).toContain('预计剩余时间：已结束')
    expect(wrapper.text().match(/未知状态/g)?.length).toBeGreaterThanOrEqual(2)
    expect(wrapper.text()).not.toContain('future_batch_status')
    expect(wrapper.text()).not.toContain('future_item_status')
  })

  it('批次 API 英文错误只显示中文兜底', async () => {
    api.listCommerceBatches.mockRejectedValue(new Error('database connection refused'))
    const wrapper = mount(CommerceBatchCenter, { props: { projectId: 3 } })
    await flushPromises()
    expect(wrapper.text()).toContain('批次加载失败，请稍后重试')
    expect(wrapper.text()).not.toContain('database connection refused')
  })
})
