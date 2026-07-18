import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const routerPush = vi.hoisted(() => vi.fn())
const routeState = vi.hoisted(() => ({
  query: {}
}))
const apiMocks = vi.hoisted(() => ({
  getPackages: vi.fn(),
  getMe: vi.fn(),
  sendSMSCode: vi.fn(),
  bindAccountPhone: vi.fn(),
  createAlipayOrder: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    getPackages: apiMocks.getPackages,
    getMe: apiMocks.getMe,
    sendSMSCode: apiMocks.sendSMSCode,
    bindAccountPhone: apiMocks.bindAccountPhone,
    createAlipayOrder: apiMocks.createAlipayOrder
  }
}))

vi.mock('vue-router', () => ({
  RouterLink: {
    props: ['to'],
    template: '<a :href="typeof to === \'string\' ? to : to.path"><slot /></a>'
  },
  useRouter: () => ({
    push: routerPush
  }),
  useRoute: () => routeState
}))

import PricingView from '../views/PricingView.vue'
import { clearCurrentUser, currentUser } from '../stores/session.js'

const packageItems = [
  { id: 1, name: '体验包', credits: 50, price_label: '10 元', description: '适合新手快速体验', badge: '体验', theme: 'blue', valid_days: 30 },
  { id: 2, name: '入门包', credits: 188, price_label: '30 元', description: '适合稳定内容生产', badge: '入门', theme: 'green', valid_days: 90 },
  { id: 3, name: '常用包', credits: 688, price_label: '100 元', description: '适合日常高频创作', badge: '常用', theme: 'orange', valid_days: 180 },
  { id: 4, name: '进阶包', credits: 1488, price_label: '198 元', description: '适合进阶商业创作', badge: '进阶', theme: 'violet', valid_days: 365 },
  { id: 5, name: '专业包', credits: 2588, price_label: '298 元', description: '适合专业创作者持续产出', badge: '推荐', theme: 'rose', recommended: true, valid_days: 365 },
  { id: 6, name: '旗舰包', credits: 6188, price_label: '648 元', description: '适合大批量生成与长期储备', badge: '最划算', theme: 'gold', valid_days: 365 }
]

describe('PricingView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    clearCurrentUser()
    routerPush.mockReset()
    routeState.query = {}
  })

  it('renders the price-list page with six real packages', async () => {
    apiMocks.getPackages.mockResolvedValueOnce({ items: packageItems })
    apiMocks.getMe.mockResolvedValueOnce({
      user_id: 12,
      username: 'creator',
      display_name: '创作者',
      available_credits: 5,
      phone: '13800138000'
    })

    const wrapper = mount(PricingView)
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('AI Image + Video')
    expect(wrapper.text()).toContain('白霖共享 AI 价目表')
    expect(wrapper.text()).toContain('点数可用于图片生成、视频生成、作品管理等创作能力')
    expect(wrapper.text()).toContain('支付宝自动到账')
    expect(wrapper.text()).toContain('虚拟商品线上交付')
    expect(wrapper.text()).not.toContain('一次购买')
    expect(wrapper.text()).not.toContain('企业方案')
    expect(wrapper.find('[data-testid="pricing-mode-single"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="pricing-mode-enterprise"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-testid^="pricing-package-card-"]')).toHaveLength(6)
    expect(wrapper.text()).toContain('体验包')
    expect(wrapper.text()).toContain('10 元')
    expect(wrapper.text()).toContain('50 点')
    expect(wrapper.text()).toContain('专业包')
    expect(wrapper.text()).toContain('298 元')
    expect(wrapper.text()).toContain('2588 点')
    expect(wrapper.text()).toContain('旗舰包')
    expect(wrapper.text()).toContain('648 元')
    expect(wrapper.text()).toContain('6188 点')
    expect(wrapper.text()).toContain('最划算')
    expect(wrapper.text()).toContain('图片生成')
    expect(wrapper.text()).toContain('视频生成')
    expect(wrapper.text()).toContain('图生视频 / 参考图能力')
    expect(wrapper.text()).toContain('作品入库')
    expect(wrapper.text()).toContain('套餐权益对比')
    expect(wrapper.text()).toContain('常见问题')
    expect(wrapper.get('[data-testid="pricing-comparison-table"]').attributes('style')).toContain('--pricing-plan-count: 6')
    expect(wrapper.text()).toContain('失败任务不扣点')
    expect(wrapper.text()).toContain('视频如何扣点')
    expect(wrapper.text()).toContain('提交前工作台会实时预估')
    expect(wrapper.text()).toContain('失败会扣点吗')
    expect(wrapper.text()).not.toMatch(/点\/秒/)
    expect(wrapper.text()).not.toMatch(/约\s*\d+\s*秒/)
    expect(wrapper.text()).not.toMatch(/约\s*\d+\s*张/)
    expect(wrapper.text()).not.toMatch(/Sora|Seedance|Kling|Runway/i)
    expect(wrapper.get('#recharge-guide').text()).toContain('支付后多久到账')
    expect(wrapper.get('#points-rules').text()).toContain('视频如何扣点')
  })

  it('renders configured package card features and comparison benefits', async () => {
    apiMocks.getPackages.mockResolvedValueOnce({
      items: [
        {
          id: 9,
          name: '商业包',
          credits: 88,
          price_label: '128 元',
          description: '适合商业主图',
          icon: '★',
          badge: '商用推荐',
          theme: 'teal',
          recommended: true,
          features: ['商用授权', '加急排队'],
          benefits: [
            { label: '商用授权', value: '支持' },
            { label: '团队席位', value: '3 人' }
          ]
        }
      ]
    })
    apiMocks.getMe.mockResolvedValueOnce({ user_id: 12 })

    const wrapper = mount(PricingView)
    await flushPromises()

    expect(wrapper.text()).toContain('商用推荐')
    expect(wrapper.text()).toContain('商用授权')
    expect(wrapper.text()).toContain('加急排队')
    expect(wrapper.text()).toContain('团队席位')
    expect(wrapper.text()).toContain('3 人')
    expect(wrapper.get('[data-testid="pricing-package-card-9"]').classes()).toContain('pricing-plan-teal')
  })

  it('fills required video descriptions when configured pricing data is missing video benefits', async () => {
    apiMocks.getPackages.mockResolvedValueOnce({
      items: [
        {
          id: 21,
          name: '灵感包',
          credits: 20,
          price_label: '39 元',
          description: '适合轻量日常创作',
          features: ['高清下载', '私有作品库'],
          benefits: [
            { label: '点数', value: '20 点' },
            { label: '图片生成', value: '✓' },
            { label: '高清下载', value: '✓' }
          ]
        },
        {
          id: 22,
          name: '创作包',
          credits: 60,
          price_label: '99 元',
          description: '适合稳定内容生产',
          features: ['优先队列', '商用授权'],
          benefits: [
            { label: '点数', value: '60 点' },
            { label: '图片生成', value: '✓' },
            { label: '高清下载', value: '✓' }
          ]
        }
      ]
    })
    apiMocks.getMe.mockResolvedValueOnce({ user_id: 12 })

    const wrapper = mount(PricingView)
    await flushPromises()

    const firstCardText = wrapper.get('[data-testid="pricing-package-card-21"]').text()
    expect(firstCardText).toContain('高清下载')
    expect(firstCardText).toContain('私有作品库')
    expect(firstCardText).toContain('支持视频生成')
    expect(firstCardText).toContain('支持参考图 / 图生视频')
    expect(firstCardText).toContain('失败任务不扣点，以生成页实时提示为准')

    expect(wrapper.text()).toContain('视频生成')
    expect(wrapper.text()).toContain('图生视频 / 参考图能力')
    expect(wrapper.text()).not.toMatch(/点\s*\/\s*秒/)
    expect(wrapper.text()).not.toMatch(/约\s*\d+\s*秒/)
    expect(wrapper.text()).not.toMatch(/约\s*\d+\s*张/)
    expect(wrapper.text()).not.toMatch(/Sora|Seedance|Kling|Runway/i)
  })

  it('shows couple album credit shortfall guidance and keeps the recommended package visible', async () => {
    routeState.query = {
      source: 'couple_album',
      missing_credits: '38',
      required_credits: '56',
      package_id: '7'
    }
    apiMocks.getPackages.mockResolvedValueOnce({
      items: [
        ...packageItems,
        { id: 7, name: '补点包', credits: 80, price_label: '128 元', description: '适合一次补足相册点数', badge: '本次推荐' }
      ]
    })
    apiMocks.getMe.mockResolvedValueOnce({ user_id: 12 })

    const wrapper = mount(PricingView)
    await flushPromises()
    await wrapper.vm.$nextTick()

    const banner = wrapper.get('[data-testid="pricing-couple-album-shortfall"]')
    expect(banner.text()).toContain('本次还差 38 点')
    expect(banner.text()).toContain('本次预计消耗 56 点')
    expect(banner.text()).toContain('推荐购买「补点包」')
    expect(wrapper.findAll('[data-testid^="pricing-package-card-"]')).toHaveLength(7)
    expect(wrapper.get('[data-testid="pricing-package-card-7"]').classes()).toContain('pricing-plan-context-recommended')
  })

  it('shows video generation credit shortfall guidance and keeps the recommended package visible', async () => {
    routeState.query = {
      source: 'video_generation',
      missing_credits: '8',
      required_credits: '18',
      package_id: '7'
    }
    apiMocks.getPackages.mockResolvedValueOnce({
      items: [
        ...packageItems,
        { id: 7, name: '补点包', credits: 80, price_label: '128 元', description: '适合一次补足视频点数', badge: '本次推荐' }
      ]
    })
    apiMocks.getMe.mockResolvedValueOnce({ user_id: 12 })

    const wrapper = mount(PricingView)
    await flushPromises()
    await wrapper.vm.$nextTick()

    const banner = wrapper.get('[data-testid="pricing-video-generation-shortfall"]')
    expect(banner.text()).toContain('视频生成点数不足')
    expect(banner.text()).toContain('本次还差 8 点')
    expect(banner.text()).toContain('本次预计消耗 18 点')
    expect(banner.text()).toContain('推荐购买「补点包」')
    expect(wrapper.findAll('[data-testid^="pricing-package-card-"]')).toHaveLength(7)
    expect(wrapper.get('[data-testid="pricing-package-card-7"]').classes()).toContain('pricing-plan-context-recommended')
  })

  it('creates an Alipay order and sends a logged-in user to checkout', async () => {
    apiMocks.getPackages.mockResolvedValueOnce({ items: packageItems })
    apiMocks.getMe.mockResolvedValueOnce({
      user_id: 12,
      username: 'creator',
      display_name: '创作者',
      available_credits: 5,
      phone: '13800138000'
    })
    apiMocks.createAlipayOrder.mockResolvedValueOnce({
      order_number: 'FO-ALIPAY-001',
      payment_status: 'pending'
    })

    const wrapper = mount(PricingView)
    await flushPromises()
    await wrapper.vm.$nextTick()

    await wrapper.get('button[data-package-id="3"]').trigger('click')
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(apiMocks.createAlipayOrder).toHaveBeenCalledWith({
      package_id: 3
    })
    expect(routerPush).toHaveBeenCalledWith('/checkout/alipay/FO-ALIPAY-001')
  })

  it('opens a phone binding modal before checkout for logged-in users without a phone', async () => {
    apiMocks.getPackages.mockResolvedValueOnce({ items: packageItems })
    apiMocks.getMe.mockResolvedValueOnce({
      user_id: 12,
      username: 'creator',
      display_name: '创作者',
      available_credits: 5,
      phone: null
    })

    const wrapper = mount(PricingView)
    await flushPromises()
    await wrapper.vm.$nextTick()

    await wrapper.get('button[data-package-id="2"]').trigger('click')
    await flushPromises()
    await wrapper.vm.$nextTick()

    const modal = wrapper.get('[data-testid="pricing-phone-bind-modal"]')
    expect(modal.attributes('role')).toBe('dialog')
    expect(modal.attributes('aria-modal')).toBe('true')
    expect(modal.text()).toContain('入门包')
    expect(modal.text()).toContain('30 元')
    expect(wrapper.get('[data-testid="pricing-send-bind-phone-code"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="pricing-bind-phone-submit"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="pricing-bind-phone-cancel"]').exists()).toBe(true)
    expect(apiMocks.createAlipayOrder).not.toHaveBeenCalled()
  })

  it('binds a phone from the pricing modal and continues the selected Alipay checkout', async () => {
    apiMocks.getPackages.mockResolvedValueOnce({ items: packageItems })
    apiMocks.getMe.mockResolvedValueOnce({
      user_id: 12,
      username: 'creator',
      display_name: '创作者',
      available_credits: 5,
      phone: ''
    })
    apiMocks.sendSMSCode.mockResolvedValueOnce({})
    apiMocks.bindAccountPhone.mockResolvedValueOnce({
      user_id: 12,
      username: 'creator',
      display_name: '创作者',
      available_credits: 5,
      phone: '13800138000'
    })
    apiMocks.createAlipayOrder.mockResolvedValueOnce({
      order_number: 'FO-ALIPAY-BIND',
      payment_status: 'pending'
    })

    const wrapper = mount(PricingView)
    await flushPromises()

    await wrapper.get('button[data-package-id="3"]').trigger('click')
    await wrapper.get('[data-testid="pricing-bind-phone-input"]').setValue('13800138000')

    const sendButton = wrapper.get('[data-testid="pricing-send-bind-phone-code"]')
    expect(sendButton.attributes('disabled')).toBeUndefined()
    await sendButton.trigger('click')
    await flushPromises()

    expect(apiMocks.sendSMSCode).toHaveBeenCalledWith({
      phone: '13800138000',
      purpose: 'bind_phone'
    })
    expect(wrapper.get('[data-testid="pricing-send-bind-phone-code"]').text()).toBe('60s')

    await wrapper.get('[data-testid="pricing-bind-phone-code"]').setValue('123456')
    await wrapper.get('[data-testid="pricing-bind-phone-submit"]').trigger('click')
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(apiMocks.bindAccountPhone).toHaveBeenCalledWith({
      phone: '13800138000',
      verification_code: '123456'
    })
    expect(currentUser.value?.phone).toBe('13800138000')
    expect(apiMocks.createAlipayOrder).toHaveBeenCalledWith({
      package_id: 3
    })
    expect(routerPush).toHaveBeenCalledWith('/checkout/alipay/FO-ALIPAY-BIND')
    expect(wrapper.find('[data-testid="pricing-phone-bind-modal"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it('opens the phone binding modal when Alipay order creation requires a phone binding', async () => {
    apiMocks.getPackages.mockResolvedValueOnce({ items: packageItems })
    apiMocks.getMe.mockResolvedValueOnce({
      user_id: 12,
      username: 'creator',
      display_name: '创作者',
      available_credits: 5,
      phone: '13800138000'
    })
    apiMocks.createAlipayOrder.mockRejectedValueOnce(
      Object.assign(new Error('请先绑定手机号后再发起支付'), {
        code: 'phone_binding_required',
        status: 409
      })
    )

    const wrapper = mount(PricingView)
    await flushPromises()

    await wrapper.get('button[data-package-id="4"]').trigger('click')
    await flushPromises()
    await wrapper.vm.$nextTick()

    const modal = wrapper.get('[data-testid="pricing-phone-bind-modal"]')
    expect(modal.text()).toContain('进阶包')
    expect(modal.text()).toContain('198 元')
    expect(apiMocks.createAlipayOrder).toHaveBeenCalledWith({
      package_id: 4
    })
    expect(wrapper.text()).not.toContain('请先绑定手机号后再发起支付')
  })

  it('sends an anonymous user to login before checkout', async () => {
    apiMocks.getPackages.mockResolvedValueOnce({ items: packageItems })
    apiMocks.getMe.mockRejectedValueOnce(new Error('unauthorized'))

    const wrapper = mount(PricingView)
    await flushPromises()
    await wrapper.vm.$nextTick()

    await wrapper.get('button[data-package-id="1"]').trigger('click')
    await flushPromises()

    expect(routerPush).toHaveBeenCalledWith('/login')
    expect(apiMocks.createAlipayOrder).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="pricing-phone-bind-modal"]').exists()).toBe(false)
  })
})
