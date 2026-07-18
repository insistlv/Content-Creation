import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const routeState = vi.hoisted(() => ({
  params: { id: '42' }
}))
const apiMocks = vi.hoisted(() => ({
  getCoupleAlbum: vi.fn(),
  retryCoupleAlbumPage: vi.fn(),
  shareCoupleAlbum: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    getCoupleAlbum: apiMocks.getCoupleAlbum,
    retryCoupleAlbumPage: apiMocks.retryCoupleAlbumPage,
    shareCoupleAlbum: apiMocks.shareCoupleAlbum
  }
}))

vi.mock('vue-router', () => ({
  RouterLink: {
    props: ['to'],
    template: '<a :href="typeof to === \'string\' ? to : to.path"><slot /></a>'
  },
  useRoute: () => routeState
}))

import CoupleAlbumDetailView from '../views/CoupleAlbumDetailView.vue'

function albumPayload(overrides = {}) {
  return {
    id: 42,
    title: '西湖纪念日',
    location: '杭州',
    status: 'partial_failed',
    share_enabled: false,
    cover_page_id: 101,
    pages: [
      {
        id: 101,
        page_number: 1,
        page_title: '封面',
        caption: '把这次出发写进相册',
        status: 'succeeded',
        preview_url: '/api/works/101/preview',
        download_url: '/api/works/101/download'
      },
      {
        id: 102,
        page_number: 2,
        page_title: '出发',
        caption: '清晨的行李箱',
        status: 'failed',
        error_message: '模型生成失败',
        preview_url: ''
      }
    ],
    ...overrides
  }
}

describe('CoupleAlbumDetailView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    routeState.params = { id: '42' }
    vi.stubGlobal('navigator', {
      clipboard: {
        writeText: vi.fn().mockResolvedValue(undefined)
      }
    })
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('loads the album detail, progress, preview, and failed page retry action', async () => {
    apiMocks.getCoupleAlbum.mockResolvedValueOnce({ album: albumPayload() })
    apiMocks.retryCoupleAlbumPage.mockResolvedValueOnce({
      album: albumPayload({ status: 'generating', pages: [albumPayload().pages[0], { ...albumPayload().pages[1], status: 'queued', error_message: '' }] })
    })

    const wrapper = mount(CoupleAlbumDetailView)
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(apiMocks.getCoupleAlbum).toHaveBeenCalledWith('42')
    expect(wrapper.text()).toContain('西湖纪念日')
    expect(wrapper.text()).toContain('杭州')
    expect(wrapper.text()).toContain('1/8')
    expect(wrapper.get('[data-testid="couple-album-page-101"] img').attributes('src')).toBe('/api/works/101/preview')
    expect(wrapper.text()).toContain('模型生成失败')

    await wrapper.get('[data-testid="retry-couple-album-page-102"]').trigger('click')
    await flushPromises()

    expect(apiMocks.retryCoupleAlbumPage).toHaveBeenCalledWith('42', 102)
  })

  it('polls while the album is generating and stops after success', async () => {
    vi.useFakeTimers()
    apiMocks.getCoupleAlbum
      .mockResolvedValueOnce({
        album: albumPayload({
          status: 'generating',
          pages: [
            { ...albumPayload().pages[0], status: 'running', preview_url: '' },
            { ...albumPayload().pages[1], status: 'queued', error_message: '' }
          ]
        })
      })
      .mockResolvedValueOnce({ album: albumPayload() })

    const wrapper = mount(CoupleAlbumDetailView)
    await flushPromises()

    expect(wrapper.text()).toContain('0/8')

    await vi.advanceTimersByTimeAsync(1000)
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(apiMocks.getCoupleAlbum).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('1/8')
    expect(wrapper.text()).toContain('部分失败')
  })

  it('enables sharing and exposes a PC public share link', async () => {
    apiMocks.getCoupleAlbum.mockResolvedValueOnce({ album: albumPayload() })
    apiMocks.shareCoupleAlbum.mockResolvedValueOnce({
      share_token: 'public-token',
      album: albumPayload({ share_enabled: true, share_token: 'public-token' })
    })

    const wrapper = mount(CoupleAlbumDetailView)
    await flushPromises()

    await wrapper.get('[data-testid="couple-album-share"]').trigger('click')
    await flushPromises()
    await wrapper.vm.$nextTick()

    expect(apiMocks.shareCoupleAlbum).toHaveBeenCalledWith('42')
    expect(wrapper.get('[data-testid="couple-album-share-url"]').element.value).toContain('/couple-albums/share/public-token')

    await wrapper.get('[data-testid="couple-album-copy-share-url"]').trigger('click')
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(expect.stringContaining('/couple-albums/share/public-token'))
  })
})
