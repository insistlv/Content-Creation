import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const routeState = vi.hoisted(() => ({ query: {} }))
const routerMocks = vi.hoisted(() => ({ replace: vi.fn(async ({ query }) => { routeState.query = query }) }))
const apiMocks = vi.hoisted(() => ({
  getMe: vi.fn(),
  listVideoConversations: vi.fn(),
  getVideoConversation: vi.fn(),
  createVideoConversation: vi.fn(),
  patchVideoConversation: vi.fn(),
  createVideoConversationMessage: vi.fn(),
  createVideoGeneration: vi.fn(),
  getVideoGeneration: vi.fn(),
  listVideoModels: vi.fn(),
  listReferenceAssets: vi.fn(),
  estimateVideoGeneration: vi.fn(),
  listVideoSoundtracks: vi.fn(),
  generateVideoSoundtrack: vi.fn(),
  uploadVideoSoundtrack: vi.fn()
}))

vi.mock('vue-router', () => ({ useRoute: () => routeState, useRouter: () => routerMocks }))
vi.mock('../api/client.js', () => ({ api: apiMocks }))

import VideoConversationWorkspaceView from '../views/VideoConversationWorkspaceView.vue'
import { useUserTheme } from '../composables/useUserTheme.js'
import { currentUser } from '../stores/session.js'

const wrappers = new Set()
const conversation = { id: 1, title: '产品宣传片', is_favorite: false, last_activity_at: '2026-07-12T01:00:00Z' }

function generation(id, overrides = {}) {
  return { id, generation_record_id: id, work_id: id + 100, prompt: `提示词 ${id}`, status: 'succeeded', preview_url: `/video/${id}.mp4`, download_url: `/video/${id}/download`, runtime_model: 'seedance', aspect_ratio: '16:9', resolution: '1080p', duration_seconds: 5, reference_asset_ids: [8], ...overrides }
}

function mountView(query = {}) {
  routeState.query = query
  const wrapper = mount(VideoConversationWorkspaceView, { attachTo: document.body })
  wrappers.add(wrapper)
  return wrapper
}

beforeEach(() => {
  currentUser.value = { username: 'tester', available_credits: 20 }
  routerMocks.replace.mockClear()
  Object.values(apiMocks).forEach(mock => mock.mockReset())
  apiMocks.getMe.mockResolvedValue(currentUser.value)
  apiMocks.listVideoConversations.mockResolvedValue({ items: [conversation] })
  apiMocks.listVideoModels.mockResolvedValue({ items: [{ name: 'Seedance', runtime_model: 'seedance' }] })
  apiMocks.getVideoConversation.mockResolvedValue({ conversation, timeline: [] })
  apiMocks.listReferenceAssets.mockResolvedValue({ items: [] })
  apiMocks.estimateVideoGeneration.mockResolvedValue({ required_credits: 3 })
  apiMocks.listVideoSoundtracks.mockResolvedValue({ items: [] })
})

afterEach(() => {
  wrappers.forEach(wrapper => wrapper.unmount())
  wrappers.clear()
  document.body.innerHTML = ''
  document.body.style.overflow = ''
  window.localStorage.removeItem('image_agent_user_theme:v1')
  vi.useRealTimers()
})

describe('VideoConversationWorkspaceView interactions', () => {
  it('follows the shared theme immediately for the workspace and teleported asset modal', async () => {
    window.localStorage.setItem('image_agent_user_theme:v1', 'light')
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-testid="video-conversation-workspace"]').attributes('data-theme')).toBe('light')
    await wrapper.get('.video-chat-toolbar > button:last-child').trigger('click')
    await flushPromises()
    expect(document.body.querySelector('.video-asset-modal')?.dataset.theme).toBe('light')

    useUserTheme().toggleTheme()
    await nextTick()
    expect(wrapper.get('[data-testid="video-conversation-workspace"]').attributes('data-theme')).toBe('dark')
    expect(document.body.querySelector('.video-asset-modal')?.dataset.theme).toBe('dark')
  })

  it('loads a conversation, favorites it and never submits video generation', async () => {
    apiMocks.patchVideoConversation.mockResolvedValue({ ...conversation, is_favorite: true })
    const wrapper = mountView({ conversation: '1' })
    await flushPromises()

    expect(apiMocks.getVideoConversation).toHaveBeenCalledWith(1)
    await wrapper.get('[aria-label="收藏会话"]').trigger('click')
    await flushPromises()
    expect(apiMocks.patchVideoConversation).toHaveBeenCalledWith(1, { is_favorite: true })
    expect(apiMocks.createVideoGeneration).not.toHaveBeenCalled()
  })

  it('shows asset errors, retries, and explains the 12 asset limit', async () => {
    apiMocks.listReferenceAssets.mockRejectedValueOnce(new Error('素材服务不可用')).mockResolvedValueOnce({ items: Array.from({ length: 13 }, (_, index) => ({ id: index + 1, original_filename: `${index + 1}.png` })) })
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('.video-add-asset').trigger('click')
    await flushPromises()
    expect(document.body.textContent).toContain('素材服务不可用')
    await document.body.querySelector('.video-action-error button').click()
    await flushPromises()
    const buttons = Array.from(document.body.querySelectorAll('.video-asset-modal section > button'))
    for (const button of buttons) button.click()
    await flushPromises()
    expect(wrapper.text()).toContain('最多选择 12 个参考素材')
    expect(apiMocks.createVideoGeneration).not.toHaveBeenCalled()
  })

  it('uses the assistant reply and refills its suggested prompt without generating video', async () => {
    apiMocks.getVideoConversation.mockResolvedValue({ conversation, timeline: [] })
    apiMocks.createVideoConversationMessage.mockResolvedValue({ message: { id: 2, role: 'user', content: '策划产品片', status: 'answered' }, reply: { id: 3, role: 'assistant', content: '建议使用慢推镜头', suggested_prompt: '产品慢推镜头', quick_replies: ['改成竖屏'] } })
    const wrapper = mountView({ conversation: '1' })
    await flushPromises()
    await wrapper.get('.video-mode-switch button:first-child').trigger('click')
    await wrapper.get('textarea').setValue('策划产品片')
    await wrapper.get('.video-send').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('建议使用慢推镜头')
    await wrapper.get('.video-suggested button').trigger('click')
    expect(wrapper.get('textarea').element.value).toBe('产品慢推镜头')
    expect(apiMocks.createVideoGeneration).not.toHaveBeenCalled()
  })

  it('loads and generates soundtracks, compares versions, and refills all reusable parameters', async () => {
    const older = generation(11)
    const latest = generation(12, { prompt: '最新提示词' })
    apiMocks.getVideoConversation.mockResolvedValue({ conversation, timeline: [{ type: 'generation', generation: older }, { type: 'generation', generation: latest }] })
    apiMocks.generateVideoSoundtrack.mockResolvedValue({ audio_url: '/audio/1.mp3', download_url: '/audio/1/download' })
    apiMocks.listReferenceAssets.mockResolvedValue({ items: [{ id: 8, original_filename: '参考图.png' }] })
    const wrapper = mountView({ conversation: '1' })
    await flushPromises()

    const smartButton = wrapper.findAll('.video-result-actions button').find(button => button.text().includes('智能配乐'))
    await smartButton.trigger('click')
    await flushPromises()
    expect(apiMocks.generateVideoSoundtrack).toHaveBeenCalledWith(111, { variation: 'smart' })
    expect(wrapper.find('audio').exists()).toBe(true)

    const compareButton = wrapper.findAll('.video-result-actions button').find(button => button.text().includes('版本对比') && !button.attributes('disabled'))
    await compareButton.trigger('click')
    await flushPromises()
    expect(document.body.textContent).toContain('版本对比')
    document.body.querySelector('[aria-label="关闭版本对比"]').click()

    const regenerateButton = wrapper.findAll('.video-result-actions button').find(button => button.text().includes('再次生成'))
    await regenerateButton.trigger('click')
    expect(wrapper.get('textarea').element.value).toBe('提示词 11')
    expect(wrapper.get('[aria-label="分辨率"]').element.value).toBe('1080p')
    expect(apiMocks.createVideoGeneration).not.toHaveBeenCalled()
  })
})
