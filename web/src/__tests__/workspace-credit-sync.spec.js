import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  getMe: vi.fn(),
  listWorks: vi.fn(),
  createImageGeneration: vi.fn(),
  getImageGeneration: vi.fn(),
  listReferenceAssets: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    getMe: apiMocks.getMe,
    listWorks: apiMocks.listWorks,
    createImageGeneration: apiMocks.createImageGeneration,
    getImageGeneration: apiMocks.getImageGeneration,
    listReferenceAssets: apiMocks.listReferenceAssets
  }
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({
    fullPath: '/workspace',
    query: {}
  }),
  useRouter: () => ({
    push: vi.fn()
  })
}))

import { clearCurrentUser, currentUser } from '../stores/session.js'
import WorkspaceView from '../views/WorkspaceView.vue'

describe('WorkspaceView credit sync', () => {
  afterEach(() => {
    vi.useRealTimers()
    vi.clearAllMocks()
    clearCurrentUser()
  })

  it('syncs shared credits after a successful image generation', async () => {
    vi.useFakeTimers()

    apiMocks.getMe.mockResolvedValueOnce({
      user_id: 9,
      username: 'creator_09',
      available_credits: 3
    })
    apiMocks.listWorks
      .mockResolvedValueOnce({ items: [] })
      .mockResolvedValueOnce({
        items: [
          {
            work_id: 90,
            prompt: 'mist over bamboo lake',
            preview_url: '/api/works/90/file',
            download_url: '/api/works/90/download',
            created_at: '2026-04-28T10:00:00Z'
          }
        ]
      })
    apiMocks.listReferenceAssets.mockResolvedValueOnce({ items: [] })
    apiMocks.createImageGeneration.mockResolvedValueOnce({
      generation_id: 12,
      status: 'queued',
      stage: 'queued',
      available_credits: 3
    })
    apiMocks.getImageGeneration.mockResolvedValueOnce({
      generation_id: 12,
      work_id: 90,
      status: 'succeeded',
      stage: 'succeeded',
      preview_url: '/api/works/90/file',
      download_url: '/api/works/90/download',
      available_credits: 2
    })

    const wrapper = mount(WorkspaceView, {
      global: {
        stubs: {
          RouterLink: {
            props: ['to'],
            template: '<a :href="typeof to === \'string\' ? to : to.path"><slot /></a>'
          }
        }
      }
    })
    await flushPromises()

    await wrapper.get('[data-testid="workspace-prompt-input"]').setValue('mist over bamboo lake')
    await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
    await flushPromises()
    await vi.advanceTimersByTimeAsync(1000)
    await flushPromises()

    expect(currentUser.value?.available_credits).toBe(2)
    expect(wrapper.text()).toContain('最新作品已写入作品库。')
  })
})
