import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const routeState = vi.hoisted(() => ({
  params: { token: 'public-token' }
}))
const apiMocks = vi.hoisted(() => ({
  getPublicCoupleAlbum: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    getPublicCoupleAlbum: apiMocks.getPublicCoupleAlbum
  }
}))

vi.mock('vue-router', () => ({
  useRoute: () => routeState
}))

import CoupleAlbumShareView from '../views/CoupleAlbumShareView.vue'

describe('CoupleAlbumShareView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    routeState.params = { token: 'public-token' }
  })

  it('loads a public token without requiring login and shows generated pages', async () => {
    apiMocks.getPublicCoupleAlbum.mockResolvedValueOnce({
      album: {
        id: 42,
        title: '西湖纪念日',
        location: '杭州',
        status: 'succeeded',
        cover_page_id: 101,
        pages: [
          {
            id: 101,
            page_number: 1,
            page_title: '封面',
            caption: '把这次出发写进相册',
            status: 'succeeded',
            preview_url: '/api/public/works/101/preview'
          },
          {
            id: 102,
            page_number: 2,
            page_title: '出发',
            caption: '清晨的行李箱',
            status: 'succeeded',
            preview_url: '/api/public/works/102/preview'
          }
        ]
      }
    })

    const wrapper = mount(CoupleAlbumShareView)
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(apiMocks.getPublicCoupleAlbum).toHaveBeenCalledWith('public-token')
    expect(wrapper.text()).toContain('西湖纪念日')
    expect(wrapper.text()).toContain('杭州')
    expect(wrapper.text()).toContain('2/8')
    expect(wrapper.findAll('[data-testid^="public-couple-album-page-"]')).toHaveLength(2)
    expect(wrapper.get('[data-testid="public-couple-album-page-101"] img').attributes('src')).toBe('/api/public/works/101/preview')
  })

  it('shows a stable error for invalid public tokens', async () => {
    apiMocks.getPublicCoupleAlbum.mockRejectedValueOnce(
      Object.assign(new Error('相册不存在'), {
        code: 'album_not_found',
        status: 404
      })
    )

    const wrapper = mount(CoupleAlbumShareView)
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('链接无效或分享已关闭')
  })
})
