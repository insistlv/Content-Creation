import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  listFinanceOrders: vi.fn(),
  getFinanceOrder: vi.fn(),
  syncFinanceOrderPayment: vi.fn(),
  updateFinanceRefund: vi.fn(),
  updateFinanceInvoice: vi.fn(),
  financeOrdersExportURL: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    listFinanceOrders: apiMocks.listFinanceOrders,
    getFinanceOrder: apiMocks.getFinanceOrder,
    syncFinanceOrderPayment: apiMocks.syncFinanceOrderPayment,
    updateFinanceRefund: apiMocks.updateFinanceRefund,
    updateFinanceInvoice: apiMocks.updateFinanceInvoice,
    financeOrdersExportURL: apiMocks.financeOrdersExportURL
  }
}))

import AdminFinanceOrdersView from '../views/AdminFinanceOrdersView.vue'

const financePayload = {
  items: [
    {
      id: 42,
      order_number: 'FO-TEST-001',
      user: { id: 7, username: 'finance-customer', display_name: '财务客户', email: 'finance@example.com' },
      package_name: '团队包',
      amount_cents: 39900,
      order_type: 'package',
      payment_method: 'offline_transfer',
      payment_status: 'paid',
      invoice_status: 'pending',
      created_at: '2026-05-01T08:00:00Z',
      paid_at: '2026-05-01T08:05:00Z'
    }
  ],
  kpis: {
    today_revenue_cents: 39900,
    month_revenue_cents: 79800,
    pending_orders: 2,
    refunding_count: 1
  },
  trend: [
    { date: '2026-04-30', revenue_cents: 0, order_count: 0 },
    { date: '2026-05-01', revenue_cents: 39900, order_count: 1 }
  ],
  refund_overview: {
    total_refund_cents: 9900,
    pending_count: 1,
    processing_count: 0,
    completed_count: 0,
    items: [
      {
        id: 7,
        refund_number: 'FR-TEST-001',
        order_number: 'FO-TEST-001',
        amount_cents: 9900,
        reason: '客户申请退款',
        status: 'pending',
        requested_at: '2026-05-01T09:00:00Z'
      }
    ]
  },
  invoice_overview: {
    pending_count: 1,
    issued_count: 0,
    rejected_count: 0,
    items: [
      {
        id: 9,
        invoice_number: 'FI-TEST-001',
        order_number: 'FO-TEST-001',
        amount_cents: 39900,
        title: '财务客户',
        status: 'pending'
      }
    ]
  },
  total: 1,
  page: 1,
  page_size: 10
}

describe('AdminFinanceOrdersView', () => {
  afterEach(() => {
    vi.clearAllMocks()
    vi.unstubAllGlobals()
  })

  it('renders KPI cards, trend chart, order table and finance overviews', async () => {
    apiMocks.listFinanceOrders.mockResolvedValue(financePayload)

    const wrapper = mount(AdminFinanceOrdersView)
    await flushPromises()

    expect(apiMocks.listFinanceOrders).toHaveBeenCalledWith(expect.objectContaining({
      page: 1,
      page_size: 10
    }))
    expect(wrapper.text()).toContain('财务订单')
    expect(wrapper.text()).toContain('今日收入')
    expect(wrapper.text()).toContain('本月收入')
    expect(wrapper.text()).toContain('待处理订单')
    expect(wrapper.text()).toContain('退款中')
    expect(wrapper.text()).toContain('FO-TEST-001')
    expect(wrapper.text()).toContain('财务客户')
    expect(wrapper.text()).toContain('团队包')
    expect(wrapper.text()).toContain('FR-TEST-001')
    expect(wrapper.text()).toContain('FI-TEST-001')
    expect(wrapper.find('[data-testid="finance-trend-chart"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="finance-orders-pagination"]').exists()).toBe(true)
  })

  it('applies filters, exports, views details and updates refund and invoice status', async () => {
    apiMocks.listFinanceOrders.mockResolvedValue(financePayload)
    apiMocks.getFinanceOrder.mockResolvedValue({
      ...financePayload.items[0],
      refunds: financePayload.refund_overview.items,
      invoice: financePayload.invoice_overview.items[0]
    })
    apiMocks.updateFinanceRefund.mockResolvedValue({ ok: true })
    apiMocks.updateFinanceInvoice.mockResolvedValue({ ok: true })
    apiMocks.financeOrdersExportURL.mockReturnValue('/api/admin/finance-orders/export?type=package')
    const openMock = vi.fn()
    vi.stubGlobal('open', openMock)

    const wrapper = mount(AdminFinanceOrdersView)
    await flushPromises()

    await wrapper.get('[data-testid="finance-order-search"]').setValue('FO-TEST')
    await wrapper.get('[data-testid="finance-order-type"]').setValue('package')
    await wrapper.get('[data-testid="finance-payment-status"]').setValue('paid')
    await wrapper.get('[data-testid="finance-orders-filter"]').trigger('submit')
    await flushPromises()

    expect(apiMocks.listFinanceOrders).toHaveBeenLastCalledWith(expect.objectContaining({
      type: 'package',
      payment_status: 'paid',
      q: 'FO-TEST',
      page: 1,
      page_size: 10
    }))

    await wrapper.get('[data-testid="finance-export"]').trigger('click')
    expect(apiMocks.financeOrdersExportURL).toHaveBeenCalledWith(expect.objectContaining({
      type: 'package',
      payment_status: 'paid',
      q: 'FO-TEST'
    }))
    expect(openMock).toHaveBeenCalledWith('/api/admin/finance-orders/export?type=package', 'finance-orders-export')

    await wrapper.get('[data-testid="finance-view-order"]').trigger('click')
    await flushPromises()
    expect(apiMocks.getFinanceOrder).toHaveBeenCalledWith(42)
    expect(wrapper.find('[data-testid="finance-order-detail"]').text()).toContain('FO-TEST-001')

    await wrapper.get('[data-testid="finance-refund-complete"]').trigger('click')
    await flushPromises()
    expect(apiMocks.updateFinanceRefund).toHaveBeenCalledWith(7, { status: 'completed' })

    await wrapper.get('[data-testid="finance-invoice-issue"]').trigger('click')
    await flushPromises()
    expect(apiMocks.updateFinanceInvoice).toHaveBeenCalledWith(9, { status: 'issued' })
  })

  it('syncs a pending Alipay finance order from the order list', async () => {
    const alipayPendingOrder = {
      ...financePayload.items[0],
      id: 77,
      order_number: 'FO-ALIPAY-PENDING',
      payment_method: 'alipay_page',
      payment_status: 'pending'
    }
    apiMocks.listFinanceOrders.mockResolvedValue({
      ...financePayload,
      items: [alipayPendingOrder]
    })
    apiMocks.syncFinanceOrderPayment.mockResolvedValue({
      order: {
        ...alipayPendingOrder,
        payment_status: 'paid'
      },
      available_credits: 188
    })

    const wrapper = mount(AdminFinanceOrdersView)
    await flushPromises()

    await wrapper.get('[data-testid="finance-sync-payment"]').trigger('click')
    await flushPromises()

    expect(apiMocks.syncFinanceOrderPayment).toHaveBeenCalledWith(77)
    expect(apiMocks.listFinanceOrders).toHaveBeenCalledTimes(2)
  })

  it('shows Alipay identifiers and evidence snapshot in finance order detail', async () => {
    const alipayOrder = {
      ...financePayload.items[0],
      id: 88,
      order_number: 'FO-ALIPAY-001',
      payment_method: 'alipay_page',
      alipay_trade_no: '2026051322000000000888',
      alipay_notify_at: '2026-05-13T04:05:00Z',
      payment_record: {
        payment_number: 'PR-ALIPAY-001',
        provider: 'alipay',
        provider_method: 'page_pay',
        provider_trade_no: '2026051322000000000888',
        status: 'paid',
        request_count: 1,
        notify_count: 2,
        query_count: 1,
        last_error_code: 'alipay_amount_mismatch',
        last_error_message: '支付宝订单金额不一致'
      },
      transaction_url: 'https://example.com/pricing',
      evidence_snapshot: {
        product_content: '创作包，60 点，有效期 365 天',
        receipt_name: 'creator',
        receipt_address: '虚拟商品线上交付，支付成功后点数发放至当前账户，无需物流'
      }
    }
    apiMocks.listFinanceOrders.mockResolvedValue({
      ...financePayload,
      items: [alipayOrder]
    })
    apiMocks.getFinanceOrder.mockResolvedValue({
      ...alipayOrder,
      refunds: [],
      invoice: null
    })

    const wrapper = mount(AdminFinanceOrdersView)
    await flushPromises()

    expect(wrapper.text()).toContain('支付宝电脑网站支付')

    await wrapper.get('[data-testid="finance-view-order"]').trigger('click')
    await flushPromises()

    const detailText = wrapper.get('[data-testid="finance-order-detail"]').text()
    expect(detailText).toContain('PR-ALIPAY-001')
    expect(detailText).toContain('支付记录状态')
    expect(detailText).toContain('请求 1 / 通知 2 / 查询 1')
    expect(detailText).toContain('alipay_amount_mismatch')
    expect(detailText).toContain('2026051322000000000888')
    expect(detailText).toContain('https://example.com/pricing')
    expect(detailText).toContain('虚拟商品线上交付')
  })
})
