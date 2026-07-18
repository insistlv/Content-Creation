import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  getAdminCustomerService: vi.fn(),
  updateAdminCustomerService: vi.fn(),
  uploadCustomerServiceQRCode: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    getAdminCustomerService: apiMocks.getAdminCustomerService,
    updateAdminCustomerService: apiMocks.updateAdminCustomerService,
    uploadCustomerServiceQRCode: apiMocks.uploadCustomerServiceQRCode
  }
}))

import AdminCustomerServiceView from '../views/AdminCustomerServiceView.vue'

describe('AdminCustomerServiceView', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('loads, edits and saves customer service page configuration', async () => {
    apiMocks.getAdminCustomerService.mockResolvedValue({
      title: '联系客服',
      eyebrow: 'CUSTOMER SERVICE',
      subtitle: '微信 / QQ 快速联系',
      description: '联系客服说明',
      wechat: { label: '微信客服', account: 'bailin_ai', qr_url: 'https://cdn.example.com/wechat.png' },
      qq: { label: 'QQ客服', account: '123456789', qr_url: 'https://cdn.example.com/qq.png' },
      service_tags: ['账号问题', '充值咨询'],
      stats: [{ label: '在线时间', value: '09:00 - 22:00' }],
      features: [{ title: '快速响应', text: '5 分钟内响应' }],
      faqs: [{ title: '充值未到账怎么办？', url: '/pricing' }]
    })
    apiMocks.updateAdminCustomerService.mockResolvedValue({ ok: true })

    const wrapper = mount(AdminCustomerServiceView)
    await flushPromises()

    expect(wrapper.text()).toContain('客服页面配置')
    expect(wrapper.get('[data-testid="contact-title"]').element.value).toBe('联系客服')

    await wrapper.get('[data-testid="contact-wechat-account"]').setValue('new_wechat')
    await wrapper.get('[data-testid="contact-service-tags"]').setValue('账号问题\n作品下载')
    await wrapper.get('[data-testid="contact-faqs"]').setValue('作品无法下载怎么办？|/works')
    await wrapper.get('[data-testid="customer-service-form"]').trigger('submit')
    await flushPromises()

    expect(apiMocks.updateAdminCustomerService).toHaveBeenCalledWith(expect.objectContaining({
      wechat: expect.objectContaining({ account: 'new_wechat' }),
      service_tags: ['账号问题', '作品下载'],
      faqs: [{ title: '作品无法下载怎么办？', url: '/works' }]
    }))
    expect(wrapper.text()).toContain('客服配置已保存')
  })

  it('uploads qr code images, writes the returned OSS url and saves the config', async () => {
    apiMocks.getAdminCustomerService.mockResolvedValue({
      title: '联系客服',
      eyebrow: 'CUSTOMER SERVICE',
      subtitle: '微信 / QQ 快速联系',
      description: '联系客服说明',
      wechat: { label: '微信客服', account: 'bailin_ai', qr_url: '' },
      qq: { label: 'QQ客服', account: '123456789', qr_url: '' },
      service_tags: [],
      stats: [],
      features: [],
      faqs: []
    })
    apiMocks.uploadCustomerServiceQRCode.mockResolvedValue({
      url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/2026/05/wechat.png'
    })
    apiMocks.updateAdminCustomerService.mockResolvedValue({ ok: true })

    const wrapper = mount(AdminCustomerServiceView)
    await flushPromises()

    const file = new File(['fake'], 'wechat.png', { type: 'image/png' })
    const input = wrapper.get('[data-testid="contact-wechat-qr-upload"]')
    Object.defineProperty(input.element, 'files', {
      value: [file],
      configurable: true
    })
    await input.trigger('change')
    await flushPromises()

    expect(apiMocks.uploadCustomerServiceQRCode).toHaveBeenCalledWith(file)
    expect(wrapper.get('[data-testid="contact-wechat-qr-url"]').element.value).toBe('https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/2026/05/wechat.png')
    expect(apiMocks.updateAdminCustomerService).toHaveBeenCalledWith(expect.objectContaining({
      wechat: expect.objectContaining({
        qr_url: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/2026/05/wechat.png'
      })
    }))
    expect(wrapper.text()).toContain('微信二维码已上传并保存')
  })
})
