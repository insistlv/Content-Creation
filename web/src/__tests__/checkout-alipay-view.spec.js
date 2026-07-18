import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const routeState = vi.hoisted(() => ({
  params: { order_number: 'FO-ALIPAY-001' },
  query: {},
  path: '/checkout/alipay/FO-ALIPAY-001'
}))
const routerPush = vi.hoisted(() => vi.fn())
const apiMocks = vi.hoisted(() => ({
  getAlipayOrder: vi.fn(),
  payAlipayOrder: vi.fn(),
  queryAlipayOrder: vi.fn()
}))
const sessionMocks = vi.hoisted(() => ({
  applyAvailableCredits: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    getAlipayOrder: apiMocks.getAlipayOrder,
    payAlipayOrder: apiMocks.payAlipayOrder,
    queryAlipayOrder: apiMocks.queryAlipayOrder
  }
}))

vi.mock('../stores/session.js', () => ({
  applyAvailableCredits: sessionMocks.applyAvailableCredits
}))

vi.mock('vue-router', () => ({
  RouterLink: {
    props: ['to'],
    template: '<a :href="typeof to === \'string\' ? to : to.path"><slot /></a>'
  },
  useRoute: () => routeState,
  useRouter: () => ({
    push: routerPush
  })
}))

import CheckoutAlipayView from '../views/CheckoutAlipayView.vue'

const pendingOrder = {
  order_number: 'FO-ALIPAY-001',
  payment_status: 'pending',
  payment_method: 'alipay_page',
  package_name: '创作包',
  package_credits: 60,
  amount_cents: 9900,
  created_at: '2026-05-13T04:00:00Z',
  evidence_snapshot: {
    transaction_url: 'https://example.com/pricing',
    ordered_at: '2026-05-13T04:00:00Z',
    amount_cents: 9900,
    product_title: '创作包',
    product_content: '适合稳定内容生产，60 点，有效期 365 天',
    package_credits: 60,
    valid_days: 365,
    receipt_name: 'creator',
    receipt_address: '虚拟商品线上交付，支付成功后点数发放至当前账户，无需物流'
  }
}

describe('CheckoutAlipayView', () => {
  beforeEach(() => {
    vi.useRealTimers()
    vi.clearAllMocks()
    routeState.params = { order_number: 'FO-ALIPAY-001' }
    routeState.query = {}
    routeState.path = '/checkout/alipay/FO-ALIPAY-001'
  })

  it('renders the Alipay evidence receipt for a pending order', async () => {
    apiMocks.getAlipayOrder.mockResolvedValueOnce(pendingOrder)

    const wrapper = mount(CheckoutAlipayView)
    await flushPromises()

    expect(apiMocks.getAlipayOrder).toHaveBeenCalledWith('FO-ALIPAY-001')
    expect(wrapper.text()).toContain('FO-ALIPAY-001')
    expect(wrapper.text()).toContain('待支付')
    expect(wrapper.text()).toContain('¥99.00')
    expect(wrapper.text()).toContain('https://example.com/pricing')
    expect(wrapper.text()).toContain('适合稳定内容生产，60 点，有效期 365 天')
    expect(wrapper.text()).toContain('creator')
    expect(wrapper.text()).toContain('虚拟商品线上交付，支付成功后点数发放至当前账户，无需物流')
    expect(wrapper.text()).toContain('支付宝')
    expect(apiMocks.queryAlipayOrder).not.toHaveBeenCalled()
  })

  it('auto-confirms a normal pending checkout page after the pay request was opened', async () => {
    apiMocks.getAlipayOrder.mockResolvedValueOnce({
      ...pendingOrder,
      payment_request_at: '2026-07-01T02:34:54Z'
    })
    apiMocks.queryAlipayOrder.mockResolvedValue({
      order: {
        ...pendingOrder,
        payment_status: 'paid',
        payment_request_at: '2026-07-01T02:34:54Z',
        alipay_trade_no: '2026070122000000000001'
      },
      available_credits: 188
    })

    mount(CheckoutAlipayView)
    await flushPromises()

    expect(apiMocks.getAlipayOrder).toHaveBeenCalledWith('FO-ALIPAY-001')
    expect(apiMocks.queryAlipayOrder).toHaveBeenCalledWith('FO-ALIPAY-001')
    expect(sessionMocks.applyAvailableCredits).toHaveBeenCalledWith(188)
  })

  it('submits the server Alipay form when the user starts payment', async () => {
    const submitSpy = vi.spyOn(HTMLFormElement.prototype, 'submit').mockImplementation(() => {})
    apiMocks.getAlipayOrder.mockResolvedValueOnce(pendingOrder)
    apiMocks.payAlipayOrder.mockResolvedValueOnce({
      order: pendingOrder,
      form_html: '<form id="auto-submit-alipay-form" action="https://openapi-sandbox.dl.alipaydev.com/gateway.do"></form>'
    })

    const wrapper = mount(CheckoutAlipayView)
    await flushPromises()

    await wrapper.get('[data-testid="checkout-alipay-pay"]').trigger('click')
    await flushPromises()

    expect(apiMocks.payAlipayOrder).toHaveBeenCalledWith('FO-ALIPAY-001')
    expect(wrapper.get('[data-testid="alipay-form-host"]').html()).toContain('auto-submit-alipay-form')
    expect(submitSpy).toHaveBeenCalledTimes(1)
    submitSpy.mockRestore()
  })

  it('shows a safe error when the Alipay form is missing from the response', async () => {
    const submitSpy = vi.spyOn(HTMLFormElement.prototype, 'submit').mockImplementation(() => {})
    apiMocks.getAlipayOrder.mockResolvedValueOnce(pendingOrder)
    apiMocks.payAlipayOrder.mockResolvedValueOnce({
      order: pendingOrder,
      form_html: ''
    })

    const wrapper = mount(CheckoutAlipayView)
    await flushPromises()

    await wrapper.get('[data-testid="checkout-alipay-pay"]').trigger('click')
    await flushPromises()

    expect(apiMocks.payAlipayOrder).toHaveBeenCalledWith('FO-ALIPAY-001')
    expect(submitSpy).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('支付宝支付页面打开失败，请稍后重试')
    submitSpy.mockRestore()
  })

  it('shows a customer-safe maintenance message when Alipay config is missing', async () => {
    apiMocks.getAlipayOrder.mockResolvedValueOnce(pendingOrder)
    apiMocks.payAlipayOrder.mockRejectedValueOnce(Object.assign(new Error('支付宝支付暂未配置'), {
      code: 'alipay_not_configured',
      status: 503
    }))

    const wrapper = mount(CheckoutAlipayView)
    await flushPromises()

    await wrapper.get('[data-testid="checkout-alipay-pay"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('支付通道维护中，请联系客服')
    expect(wrapper.text()).not.toContain('支付宝支付暂未配置')
  })

  it('uses the return query order number and automatically confirms paid status into the shared balance', async () => {
    routeState.params = {}
    routeState.query = { order_number: 'FO-ALIPAY-RETURN' }
    routeState.path = '/checkout/alipay/return'
    apiMocks.getAlipayOrder.mockResolvedValueOnce({
      ...pendingOrder,
      order_number: 'FO-ALIPAY-RETURN',
      payment_status: 'pending'
    })
    apiMocks.queryAlipayOrder.mockResolvedValueOnce({
      order: {
        ...pendingOrder,
        order_number: 'FO-ALIPAY-RETURN',
        payment_status: 'paid',
        alipay_trade_no: '2026051322000000000002'
      },
      available_credits: 65
    })

    const wrapper = mount(CheckoutAlipayView)
    await flushPromises()

    expect(apiMocks.getAlipayOrder).toHaveBeenCalledWith('FO-ALIPAY-RETURN')
    expect(apiMocks.queryAlipayOrder).toHaveBeenCalledWith('FO-ALIPAY-RETURN')
    expect(sessionMocks.applyAvailableCredits).toHaveBeenCalledWith(65)
    expect(wrapper.text()).toContain('已到账')
    expect(wrapper.text()).toContain('套餐购买成功，点数已到账')
    expect(wrapper.text()).toContain('查看余额')
    expect(wrapper.text()).toContain('去工作台')
  })

  it('keeps polling the return page while Alipay is still pending', async () => {
    vi.useFakeTimers()
    routeState.params = {}
    routeState.query = { order_number: 'FO-ALIPAY-PENDING' }
    routeState.path = '/checkout/alipay/return'
    apiMocks.getAlipayOrder.mockResolvedValueOnce({
      ...pendingOrder,
      order_number: 'FO-ALIPAY-PENDING',
      payment_status: 'pending'
    })
    apiMocks.queryAlipayOrder
      .mockResolvedValueOnce({
        order: { ...pendingOrder, order_number: 'FO-ALIPAY-PENDING', payment_status: 'pending' }
      })
      .mockResolvedValueOnce({
        order: { ...pendingOrder, order_number: 'FO-ALIPAY-PENDING', payment_status: 'paid' },
        available_credits: 80
      })

    const wrapper = mount(CheckoutAlipayView)
    await flushPromises()

    expect(wrapper.text()).toContain('支付已提交，系统正在确认到账')
    expect(apiMocks.queryAlipayOrder).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(3000)
    await flushPromises()

    expect(apiMocks.queryAlipayOrder).toHaveBeenCalledTimes(2)
    expect(sessionMocks.applyAvailableCredits).toHaveBeenCalledWith(80)
    expect(wrapper.text()).toContain('套餐购买成功，点数已到账')
  })

  it('does not report failed payment when automatic return query fails', async () => {
    routeState.params = {}
    routeState.query = { order_number: 'FO-ALIPAY-RETRY' }
    routeState.path = '/checkout/alipay/return'
    apiMocks.getAlipayOrder.mockResolvedValueOnce({
      ...pendingOrder,
      order_number: 'FO-ALIPAY-RETRY',
      payment_status: 'pending'
    })
    apiMocks.queryAlipayOrder.mockRejectedValueOnce(new Error('gateway timeout'))

    const wrapper = mount(CheckoutAlipayView)
    await flushPromises()

    expect(apiMocks.queryAlipayOrder).toHaveBeenCalledWith('FO-ALIPAY-RETRY')
    expect(wrapper.text()).toContain('支付已提交，系统正在确认到账；如长时间未到账请联系客服并提供订单号')
    expect(wrapper.text()).not.toContain('支付失败')
  })
})
