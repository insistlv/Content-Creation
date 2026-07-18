import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const routeQuery = vi.hoisted(() => ({ ids: '10,11' }))
const apiMocks = vi.hoisted(() => ({
  getPublicWorks: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    getPublicWorks: apiMocks.getPublicWorks
  }
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({
    query: routeQuery
  })
}))

import WorksShareView from '../views/WorksShareView.vue'

describe('WorksShareView', () => {
  beforeEach(() => {
    apiMocks.getPublicWorks.mockReset()
    routeQuery.ids = '10,11'
  })

  it('loads public shared works by comma-separated ids', async () => {
    apiMocks.getPublicWorks.mockResolvedValueOnce({
      items: [
        {
          work_id: 10,
          prompt: '公开作品 A',
          preview_url: '/api/public/works/10/file',
          aspect_ratio: '1:1'
        },
        {
          work_id: 11,
          prompt: '公开作品 B',
          preview_url: '/api/public/works/11/file',
          aspect_ratio: '16:9'
        }
      ]
    })

    const wrapper = mount(WorksShareView)
    await flushPromises()

    expect(apiMocks.getPublicWorks).toHaveBeenCalledWith({ ids: '10,11' })
    expect(wrapper.get('[data-testid="works-share-card-10"] img').attributes('src')).toBe('/api/public/works/10/file')
    expect(wrapper.get('[data-testid="works-share-card-10"]').text()).toContain('AI生成')
    expect(wrapper.text()).toContain('公开作品 A')
    expect(wrapper.text()).toContain('公开作品 B')
  })

  it('shows empty and error states for invalid or failed share links', async () => {
    routeQuery.ids = ''

    const emptyWrapper = mount(WorksShareView)
    await flushPromises()

    expect(apiMocks.getPublicWorks).not.toHaveBeenCalled()
    expect(emptyWrapper.text()).toContain('分享链接无效')

    routeQuery.ids = '99'
    apiMocks.getPublicWorks.mockRejectedValueOnce(new Error('公开作品不可见'))

    const errorWrapper = mount(WorksShareView)
    await flushPromises()

    expect(errorWrapper.text()).toContain('公开作品不可见')
  })
})
