import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  getMe: vi.fn(),
  listWorks: vi.fn(),
  createImageGeneration: vi.fn(),
  getImageGeneration: vi.fn(),
  getWorkspaceDiscovery: vi.fn(),
  estimateImageGeneration: vi.fn(),
  listReferenceAssets: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    getMe: apiMocks.getMe,
    listWorks: apiMocks.listWorks,
    createImageGeneration: apiMocks.createImageGeneration,
    getImageGeneration: apiMocks.getImageGeneration,
    getWorkspaceDiscovery: apiMocks.getWorkspaceDiscovery,
    estimateImageGeneration: apiMocks.estimateImageGeneration,
    listReferenceAssets: apiMocks.listReferenceAssets
  }
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: vi.fn()
  })
}))

import WorkspaceView from '../views/WorkspaceView.vue'

function mountWorkspace() {
  return mount(WorkspaceView, {
    global: {
      stubs: {
        RouterLink: {
          props: ['to'],
          template: '<a :href="typeof to === \'string\' ? to : to.path"><slot /></a>'
        }
      }
    }
  })
}

async function settleWorkspace(wrapper) {
  await flushPromises()
  await wrapper.vm.$nextTick()
}

async function submitPrompt(wrapper, prompt = 'mist over bamboo lake') {
  await wrapper.get('[data-testid="workspace-prompt-input"]').setValue(prompt)
  await wrapper.get('[data-testid="workspace-composer-form"]').trigger('submit.prevent')
  await settleWorkspace(wrapper)
}

async function runNextPoll(wrapper) {
  await vi.advanceTimersByTimeAsync(1000)
  await settleWorkspace(wrapper)
}

describe('WorkspaceView result error display', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    apiMocks.getMe.mockResolvedValue({
      user_id: 9,
      username: 'creator_09',
      display_name: '栏目主理人',
      available_credits: 3
    })
    apiMocks.createImageGeneration.mockResolvedValue({
      generation_id: 12,
      status: 'queued',
      stage: 'queued',
      available_credits: 3
    })
    apiMocks.getWorkspaceDiscovery.mockResolvedValue({
      tools: [],
      models: [],
      hot: [],
      inspiration: []
    })
    apiMocks.estimateImageGeneration.mockResolvedValue({
      required_credits: 1,
      available_credits: 3,
      missing_credits: 0,
      enough: true
    })
    apiMocks.listReferenceAssets.mockResolvedValue({ items: [] })
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.clearAllMocks()
  })

  it('shows a single friendly failure notice without raw provider details', async () => {
    const rawError = 'Post "https://bailinai.net/v1/images/edits": context deadline exceeded'
    apiMocks.listWorks.mockResolvedValue({ items: [] })
    apiMocks.getImageGeneration.mockResolvedValueOnce({
      generation_id: 12,
      status: 'failed',
      stage: 'failed',
      available_credits: 3,
      credits_deducted: false,
      error: {
        code: 'provider_timeout',
        message: rawError,
        retryable: true
      }
    })

    const wrapper = mountWorkspace()
    await settleWorkspace(wrapper)

    await submitPrompt(wrapper, 'future city skyline')
    await runNextPoll(wrapper)

    const notices = wrapper.findAll('[data-testid="workspace-generation-failure-notice"]')
    expect(notices).toHaveLength(1)
    expect(notices[0].attributes('role')).toBe('alert')
    expect(notices[0].text()).toContain('网络超时，生成失败')
    expect(notices[0].text()).toContain('未扣点')
    expect(notices[0].text()).toContain('点击重试')
    expect(wrapper.text()).not.toContain('https://bailinai.net')
    expect(wrapper.text()).not.toContain('context deadline exceeded')
    expect(wrapper.text()).not.toContain('Post "')

    const resultError = wrapper.get('[data-testid="workspace-result-error"]')
    expect(resultError.attributes('role')).toBe('status')
    expect(resultError.text()).toContain('生成失败')
    expect(resultError.text()).not.toContain('网络超时')
    expect(wrapper.find('.preview-empty').exists()).toBe(false)
  })

  it('explains whether credits were deducted and whether the failed generation is retryable', async () => {
    apiMocks.listWorks.mockResolvedValue({ items: [] })
    apiMocks.getImageGeneration.mockResolvedValueOnce({
      generation_id: 12,
      status: 'failed',
      stage: 'failed',
      available_credits: 3,
      credits_cost: 1,
      credits_deducted: false,
      error: {
        code: 'provider_timeout',
        message: '模型超时 traceid: credit-state',
        retryable: true
      }
    })

    const wrapper = mountWorkspace()
    await settleWorkspace(wrapper)

    await submitPrompt(wrapper, 'future city skyline')
    await runNextPoll(wrapper)

    const text = wrapper.get('[data-testid="workspace-generation-failure-notice"]').text()
    expect(text).toContain('网络超时，生成失败')
    expect(text).toContain('未扣点')
    expect(text).toContain('点击重试')
    expect(wrapper.findAll('[data-testid="workspace-generation-failure-notice"]')).toHaveLength(1)
    expect(wrapper.get('[data-testid="workspace-result-error"]').text()).not.toContain('模型超时 traceid')
  })

  it('shows the failure instead of a stale previous image', async () => {
    const rawError = '供应商返回原始失败信息 traceid: abc123'
    apiMocks.listWorks.mockResolvedValue({
      items: [
        {
          work_id: 90,
          prompt: 'previous successful work',
          preview_url: '/api/works/90/file',
          download_url: '/api/works/90/download',
          created_at: '2026-04-28T10:00:00Z'
        }
      ]
    })
    apiMocks.getImageGeneration.mockResolvedValueOnce({
      generation_id: 12,
      status: 'failed',
      stage: 'failed',
      available_credits: 3,
      error: {
        code: 'provider_error',
        message: rawError,
        retryable: true
      }
    })

    const wrapper = mountWorkspace()
    await settleWorkspace(wrapper)

    await wrapper.get('[data-testid="workspace-tab-create"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.find('.preview-image').exists()).toBe(true)

    await submitPrompt(wrapper, 'new failed prompt')
    await runNextPoll(wrapper)

    expect(wrapper.get('[data-testid="workspace-generation-failure-notice"]').text()).toContain('图片生成失败，请稍后再试')
    expect(wrapper.text()).not.toContain(rawError)
    expect(wrapper.find('.preview-image').exists()).toBe(false)
  })

  it('retries from the right result error card with the previous prompt', async () => {
    apiMocks.listWorks.mockResolvedValue({ items: [] })
    apiMocks.createImageGeneration
      .mockResolvedValueOnce({
        generation_id: 12,
        status: 'queued',
        stage: 'queued',
        available_credits: 3
      })
      .mockResolvedValueOnce({
        generation_id: 13,
        status: 'queued',
        stage: 'queued',
        available_credits: 3
      })
    apiMocks.getImageGeneration.mockResolvedValueOnce({
      generation_id: 12,
      status: 'failed',
      stage: 'failed',
      available_credits: 3,
      error: {
        code: 'provider_timeout',
        message: '模型超时 traceid: retry-1',
        retryable: true
      }
    })

    const wrapper = mountWorkspace()
    await settleWorkspace(wrapper)

    await submitPrompt(wrapper, 'mist over bamboo lake')
    await runNextPoll(wrapper)
    await wrapper.get('[data-testid="workspace-failure-retry-generation"]').trigger('click')
    await settleWorkspace(wrapper)

    expect(apiMocks.createImageGeneration).toHaveBeenCalledTimes(2)
    expect(apiMocks.createImageGeneration).toHaveBeenLastCalledWith(expect.objectContaining({
      prompt: 'mist over bamboo lake',
      aspect_ratio: '1:1',
      tool_mode: 'generate'
    }))
    expect(wrapper.find('[data-testid="workspace-result-error"]').exists()).toBe(false)
  })
})
