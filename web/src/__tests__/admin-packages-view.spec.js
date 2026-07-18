import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  listAdminPackages: vi.fn(),
  createAdminPackage: vi.fn(),
  updateAdminPackage: vi.fn(),
  deleteAdminPackage: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    listAdminPackages: apiMocks.listAdminPackages,
    createAdminPackage: apiMocks.createAdminPackage,
    updateAdminPackage: apiMocks.updateAdminPackage,
    deleteAdminPackage: apiMocks.deleteAdminPackage
  }
}))

import AdminPackagesView from '../views/AdminPackagesView.vue'

const packagePayload = {
  items: [
    {
      id: 11,
      name: '创作包',
      description: '适合封面图、海报和产品图等创作',
      price_label: '99 元',
      price_cents: 9900,
      credits: 60,
      valid_days: 90,
      audience: '内容创作者',
      tags: ['高性价比', '海报'],
      icon: 'sparkles',
      theme: 'blue',
      badge: '高性价比',
      recommended: false,
      features: [],
      benefits: [],
      is_active: true,
      sort_order: 20
    },
    {
      id: 12,
      name: '团队包',
      description: '适合工作室团队协作和大批量内容排期',
      price_label: '399 元',
      price_cents: 39900,
      credits: 320,
      valid_days: 365,
      audience: '工作室团队',
      tags: ['团队', '高频'],
      icon: 'building',
      theme: 'violet',
      badge: '团队协作',
      recommended: true,
      features: ['团队协作', '商用授权'],
      benefits: [{ label: '团队席位', value: '5 人' }],
      is_active: false,
      sort_order: 40
    }
  ],
  summary: {
    active_packages: 1,
    active_packages_delta_percent: 0,
    active_packages_sparkline: [1, 1, 1, 1, 1, 1, 1],
    revenue_share_percent: 100,
    revenue_share_delta_percent: 12.5,
    revenue_share_sparkline: [0, 0, 20, 40, 60, 80, 100],
    average_order_cents: 9900,
    average_order_delta_percent: -5,
    average_order_sparkline: [0, 9900, 9900, 12000, 9900, 9900, 9900],
    monthly_orders: 3,
    monthly_orders_delta_percent: 50,
    monthly_orders_sparkline: [0, 0, 1, 0, 1, 0, 1]
  }
}

describe('AdminPackagesView', () => {
  afterEach(() => {
    vi.clearAllMocks()
    vi.unstubAllGlobals()
  })

  it('renders package KPIs, table, pagination and editor from the admin payload', async () => {
    apiMocks.listAdminPackages.mockResolvedValue(packagePayload)

    const wrapper = mount(AdminPackagesView)
    await flushPromises()

    expect(apiMocks.listAdminPackages).toHaveBeenCalled()
    expect(wrapper.text()).toContain('套餐配置')
    expect(wrapper.text()).toContain('在售套餐')
    expect(wrapper.text()).toContain('套餐收入占比')
    expect(wrapper.text()).toContain('平均客单价')
    expect(wrapper.text()).toContain('本月订单')
    expect(wrapper.text()).toContain('创作包')
    expect(wrapper.text()).toContain('90 天')
    expect(wrapper.text()).toContain('团队包')
    expect(wrapper.find('[data-testid="packages-pagination"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="package-editor-form"]').exists()).toBe(false)
  })

  it('opens a visible editor dialog and persists package changes through the admin API', async () => {
    apiMocks.listAdminPackages.mockResolvedValue(packagePayload)
    apiMocks.createAdminPackage.mockResolvedValue({ id: 21 })
    apiMocks.updateAdminPackage.mockResolvedValue({ ok: true })

    const wrapper = mount(AdminPackagesView)
    await flushPromises()

    await wrapper.get('[data-testid="edit-package-11"]').trigger('click')
    expect(wrapper.get('[data-testid="package-editor-form"]').text()).toContain('编辑套餐')
    expect(wrapper.get('[data-testid="package-features"]').element.value).toContain('支持图片生成')
    expect(wrapper.get('[data-testid="package-features"]').element.value).toContain('支持视频生成')
    expect(wrapper.get('[data-testid="package-features"]').element.value).toContain('支持参考图 / 图生视频')
    expect(wrapper.get('[data-testid="package-features"]').element.value).toContain('失败任务不扣点')
    expect(wrapper.get('[data-testid="package-benefits"]').element.value).toContain('点数|60 点')
    expect(wrapper.get('[data-testid="package-benefits"]').element.value).toContain('图片生成|✓')
    expect(wrapper.get('[data-testid="package-benefits"]').element.value).toContain('视频生成|✓')
    expect(wrapper.get('[data-testid="package-benefits"]').element.value).toContain('图生视频 / 参考图能力|✓')
    expect(wrapper.get('[data-testid="package-benefits"]').element.value).toContain('适合人群|内容创作者')
    expect(wrapper.get('[data-testid="package-benefits"]').element.value).not.toMatch(/点\/秒/)
    expect(wrapper.get('[data-testid="package-benefits"]').element.value).not.toMatch(/约\s*\d+\s*秒/)

    await wrapper.get('[data-testid="package-name"]').setValue('创作包 Pro')
    await wrapper.get('[data-testid="package-price"]').setValue('129')
    await wrapper.get('[data-testid="package-credits"]').setValue(88)
    await wrapper.get('[data-testid="package-valid-days"]').setValue(120)
    await wrapper.get('[data-testid="package-audience"]').setValue('商业设计师')
    await wrapper.get('[data-testid="package-tags"]').setValue('商用, 高级')
    await wrapper.get('[data-testid="package-icon"]').setValue('★')
    await wrapper.get('[data-testid="package-theme"]').setValue('teal')
    await wrapper.get('[data-testid="package-badge"]').setValue('商用推荐')
    await wrapper.get('[data-testid="package-sort-order"]').setValue(8)
    await wrapper.get('[data-testid="package-recommended"]').setValue(true)
    await wrapper.get('[data-testid="package-features"]').setValue('商用授权\n加急排队')
    await wrapper.get('[data-testid="package-benefits"]').setValue('商用授权|支持\n团队席位|3 人')
    await wrapper.get('[data-testid="package-description"]').setValue('升级后的创作套餐')
    await wrapper.get('[data-testid="package-editor-form"]').trigger('submit')
    await flushPromises()

    expect(apiMocks.updateAdminPackage).toHaveBeenCalledWith(11, expect.objectContaining({
      name: '创作包 Pro',
      price_cents: 12900,
      credits: 88,
      valid_days: 120,
      audience: '商业设计师',
      tags: ['商用', '高级'],
      icon: '★',
      theme: 'teal',
      badge: '商用推荐',
      sort_order: 8,
      recommended: true,
      features: ['商用授权', '加急排队'],
      benefits: [
        { label: '商用授权', value: '支持' },
        { label: '团队席位', value: '3 人' }
      ],
      description: '升级后的创作套餐',
      is_active: true
    }))
    expect(wrapper.find('[data-testid="package-editor-form"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('套餐已保存')

    await wrapper.get('[data-testid="new-package"]').trigger('click')
    expect(wrapper.get('[data-testid="package-editor-form"]').text()).toContain('新增套餐')

    await wrapper.get('[data-testid="package-name"]').setValue('短期体验包')
    await wrapper.get('[data-testid="package-price"]').setValue('19.9')
    await wrapper.get('[data-testid="package-credits"]').setValue(8)
    await wrapper.get('[data-testid="package-valid-days"]').setValue(14)
    await wrapper.get('[data-testid="package-audience"]').setValue('新用户')
    await wrapper.get('[data-testid="package-tags"]').setValue('体验, 入门')
    await wrapper.get('[data-testid="package-description"]').setValue('适合新用户快速体验')
    await wrapper.get('[data-testid="package-editor-form"]').trigger('submit')
    await flushPromises()

    expect(apiMocks.createAdminPackage).toHaveBeenCalledWith(expect.objectContaining({
      name: '短期体验包',
      price_cents: 1990,
      credits: 8,
      valid_days: 14,
      audience: '新用户',
      tags: ['体验', '入门'],
      is_active: true
    }))
    expect(wrapper.find('[data-testid="package-editor-form"]').exists()).toBe(false)
  })

  it('offers the six pricing theme presets in the package editor preview', async () => {
    apiMocks.listAdminPackages.mockResolvedValue(packagePayload)

    const wrapper = mount(AdminPackagesView)
    await flushPromises()

    await wrapper.get('[data-testid="new-package"]').trigger('click')
    const themeOptions = wrapper.findAll('[data-testid^="package-theme-option-"]')

    expect(themeOptions.map((item) => item.text())).toEqual(['blue', 'green', 'orange', 'violet', 'rose', 'gold'])
    await wrapper.get('[data-testid="package-theme-option-gold"]').trigger('click')

    expect(wrapper.get('[data-testid="package-theme"]').element.value).toBe('gold')
    expect(wrapper.get('[data-testid="package-preview-icon"]').classes()).toContain('package-theme-gold')
  })

  it('copies, toggles, deletes and batch manages packages through the admin API', async () => {
    apiMocks.listAdminPackages.mockResolvedValue(packagePayload)
    apiMocks.createAdminPackage.mockResolvedValue({ id: 21 })
    apiMocks.updateAdminPackage.mockResolvedValue({ ok: true })
    apiMocks.deleteAdminPackage.mockResolvedValue({ ok: true })
    vi.stubGlobal('confirm', vi.fn().mockReturnValue(true))

    const wrapper = mount(AdminPackagesView)
    await flushPromises()

    await wrapper.get('[data-testid="copy-package-11"]').trigger('click')
    expect(apiMocks.createAdminPackage).toHaveBeenCalledWith(expect.objectContaining({
      name: '创作包 副本',
      is_active: false
    }))

    await wrapper.get('[data-testid="toggle-package-11"]').trigger('click')
    expect(apiMocks.updateAdminPackage).toHaveBeenCalledWith(11, { is_active: false })

    await wrapper.get('[data-testid="delete-package-11"]').trigger('click')
    expect(apiMocks.deleteAdminPackage).toHaveBeenCalledWith(11)

    await wrapper.get('[data-testid="toggle-bulk-packages"]').trigger('click')
    expect(wrapper.text()).toContain('已选择 0 个套餐')

    await wrapper.get('[data-testid="select-package-11"]').setValue(true)
    await wrapper.get('[data-testid="select-package-12"]').setValue(true)
    expect(wrapper.text()).toContain('已选择 2 个套餐')

    await wrapper.get('[data-testid="bulk-disable-packages"]').trigger('click')
    expect(apiMocks.updateAdminPackage).toHaveBeenCalledWith(11, { is_active: false })
    expect(apiMocks.updateAdminPackage).toHaveBeenCalledWith(12, { is_active: false })

    await wrapper.get('[data-testid="select-package-11"]').setValue(true)
    await wrapper.get('[data-testid="bulk-delete-packages"]').trigger('click')
    expect(apiMocks.deleteAdminPackage).toHaveBeenCalledWith(11)
  })
})
