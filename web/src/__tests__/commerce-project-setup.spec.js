import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import CommerceProductSetup from '../components/ecommerce/CommerceProductSetup.vue'
import CommerceAssetManager from '../components/ecommerce/CommerceAssetManager.vue'

describe('CommerceProductSetup', () => {
  it('严格呈现四步建档引导', () => {
    const wrapper = mount(CommerceProductSetup, { props: { project: { id: 1, name: 'SKU A' } } })
    expect(wrapper.findAll('[data-testid="commerce-setup-step"]')).toHaveLength(4)
    expect(wrapper.text()).toContain('商品建档')
    expect(wrapper.text()).toContain('素材上传')
    expect(wrapper.text()).toContain('商品报告状态')
    expect(wrapper.text()).toContain('生产计划')
  })
})

describe('CommerceAssetManager', () => {
  it('persists category as role and lifecycle while limiting roles by pipeline', async () => {
    const wrapper = mount(CommerceAssetManager, {
      props: { pipeline: 'general', assets: [{ id: 8, role: 'product_front', lifecycle: 'temporary', retain_until: '2026-07-20T00:00:00Z', preview_url: '/a' }] }
    })
    const options = wrapper.findAll('option').map((item) => item.attributes('value'))
    expect(options).toContain('product_front')
    expect(options).toContain('logo')
    expect(options).not.toContain('garment_front')
    expect(wrapper.text()).toContain('temporary')
    expect(wrapper.text()).toContain('2026-07-20')
    await wrapper.getComponent({ name: 'ImageUploadZone' }).vm.$emit('upload', new File(['x'], 'a.webp', { type: 'image/webp' }))
    expect(wrapper.emitted('upload')?.[0][1]).toMatchObject({ role: 'product_front', lifecycle: 'project' })
    await wrapper.get('[data-testid="commerce-asset-delete-8"]').trigger('click')
    expect(wrapper.emitted('delete')?.[0][0]).toMatchObject({ id: 8 })
  })

  it('uses fashion roles for fashion and the complete set for mixed', () => {
    const fashion = mount(CommerceAssetManager, { props: { pipeline: 'fashion' } })
    const fashionRoles = fashion.findAll('option').map((item) => item.attributes('value'))
    expect(fashionRoles).toContain('garment_front')
    expect(fashionRoles).toContain('model_reference')
    expect(fashionRoles).not.toContain('product_front')
    const mixed = mount(CommerceAssetManager, { props: { pipeline: 'mixed' } })
    expect(mixed.find('label select').findAll('option')).toHaveLength(13)
  })
})
