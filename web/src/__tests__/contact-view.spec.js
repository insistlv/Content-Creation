import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

const stylesPath = resolve(process.cwd(), 'src/styles.css')
const readStyles = () => readFileSync(stylesPath, 'utf8').replace(/\r\n/g, '\n')

const apiMocks = vi.hoisted(() => ({
  getCustomerService: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    getCustomerService: apiMocks.getCustomerService
  }
}))

import ContactView from '../views/ContactView.vue'

const contactPayload = {
  title: '联系客服',
  eyebrow: 'CUSTOMER SERVICE',
  subtitle: '微信 / QQ 快速联系，移动端支持长按二维码添加微信',
  description: '如您在使用过程中遇到账户问题、充值相关、生成异常或合作咨询等需求，请随时联系我们。',
  wechat: { label: '微信客服', account: 'bailin_ai', qr_url: 'https://cdn.example.com/wechat.png' },
  qq: { label: 'QQ客服', account: '123456789', qr_url: 'https://cdn.example.com/qq.png' },
  service_tags: ['账号问题', '充值咨询', '作品下载'],
  stats: [
    { label: '在线时间', value: '09:00 - 22:00' },
    { label: '平均响应', value: '5 分钟内' }
  ],
  features: [
    { title: '快速响应', text: '专属客服团队在线服务，平均 5 分钟内响应' },
    { title: '多渠道联系', text: '微信 / QQ 双渠道支持' }
  ],
  faqs: [{ title: '充值未到账怎么办？', url: '/pricing' }]
}

describe('ContactView', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('renders the configured customer service page', async () => {
    apiMocks.getCustomerService.mockResolvedValue(contactPayload)

    const wrapper = mount(ContactView, {
      global: {
        stubs: {
          RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' }
        }
      }
    })
    await flushPromises()

    expect(apiMocks.getCustomerService).toHaveBeenCalled()
    expect(wrapper.text()).toContain('联系客服')
    expect(wrapper.text()).toContain('微信 / QQ 快速联系')
    expect(wrapper.text()).toContain('微信号： bailin_ai')
    expect(wrapper.text()).toContain('QQ： 123456789')
    expect(wrapper.text()).toContain('快速响应')
    expect(wrapper.text()).toContain('常见问题')
    expect(wrapper.get('[data-testid="wechat-qr"]').attributes('src')).toBe('https://cdn.example.com/wechat.png')
    expect(wrapper.get('[data-testid="qq-qr"]').attributes('src')).toBe('https://cdn.example.com/qq.png')
  })

  it('keeps the content report entry on the contact page', async () => {
    apiMocks.getCustomerService.mockResolvedValue(contactPayload)

    const wrapper = mount(ContactView, {
      global: {
        stubs: {
          RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' }
        }
      }
    })
    await flushPromises()

    const reportLink = wrapper.get('.contact-copy-button.report')
    expect(reportLink.attributes('href')).toBe('/content-report')
  })

  it('locks the primary contact actions to an aligned responsive grid', () => {
    const stylesSource = readStyles()

    expect(stylesSource).toMatch(
      /\.contact-primary-actions\s*\{[\s\S]*display:\s*grid;[\s\S]*grid-template-columns:\s*repeat\(2,\s*minmax\(0,\s*1fr\)\);[\s\S]*gap:\s*12px;/
    )
    expect(stylesSource).toMatch(
      /\.contact-copy-button\s*\{[\s\S]*display:\s*inline-flex;[\s\S]*align-items:\s*center;[\s\S]*justify-content:\s*center;[\s\S]*width:\s*100%;[\s\S]*box-sizing:\s*border-box;/
    )
    expect(stylesSource).toMatch(
      /\.contact-copy-button\.report\s*\{[\s\S]*grid-column:\s*1\s*\/\s*-1;[\s\S]*border:\s*1px solid rgba\(83,\s*97,\s*255,\s*0\.24\);/
    )
    expect(stylesSource).toMatch(
      /@media \(max-width:\s*760px\)\s*\{[\s\S]*\.contact-primary-actions\s*\{[\s\S]*grid-template-columns:\s*1fr;[\s\S]*\.contact-copy-button\.report\s*\{[\s\S]*grid-column:\s*auto;/
    )
  })
})
