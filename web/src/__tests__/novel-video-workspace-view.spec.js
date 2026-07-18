import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  getMe: vi.fn(),
  listVideoModels: vi.fn(),
  createNovelVideoProject: vi.fn(),
  listNovelVideoProjects: vi.fn(),
  getNovelVideoProject: vi.fn(),
  updateNovelVideoProject: vi.fn(),
  analyzeNovelVideoProject: vi.fn(),
  generateNovelVideoImagePlan: vi.fn(),
  updateNovelVideoCreature: vi.fn(),
  updateNovelVideoActor: vi.fn(),
  generateNovelVideoActorLockSheet: vi.fn(),
  generateNovelVideoCreatureImage: vi.fn(),
  generateNovelVideoAssets: vi.fn(),
  dedupeNovelVideoAssets: vi.fn(),
  updateNovelVideoAsset: vi.fn(),
  deleteNovelVideoAsset: vi.fn(),
  planNovelVideoEpisodes: vi.fn(),
  updateNovelVideoShot: vi.fn(),
  renderNovelVideoApprovedShots: vi.fn(),
  renderNovelVideoPreflight: vi.fn(),
  queueNovelVideoRender: vi.fn(),
  generateNovelVideoStoryboard: vi.fn(),
  generateNovelVideoGrids: vi.fn(),
  generateNovelVideoShotImages: vi.fn(),
  listNovelVideoShotImages: vi.fn(),
  updateNovelVideoShotImage: vi.fn(),
  getNovelVideoCostEstimate: vi.fn(),
  composeNovelVideoProject: vi.fn(),
  listNovelVideoCompositions: vi.fn(),
  listNovelVideoEvents: vi.fn(),
  exportNovelVideoProject: vi.fn(),
  exportNovelVideoProjectJSON: vi.fn(),
  exportNovelVideoProjectPackage: vi.fn()
}))

const parserMocks = vi.hoisted(() => ({
  parseNovelSourceFile: vi.fn()
}))

vi.mock('../api/client.js', () => ({
  api: {
    getMe: apiMocks.getMe,
    listVideoModels: apiMocks.listVideoModels,
    createNovelVideoProject: apiMocks.createNovelVideoProject,
    listNovelVideoProjects: apiMocks.listNovelVideoProjects,
    getNovelVideoProject: apiMocks.getNovelVideoProject,
    updateNovelVideoProject: apiMocks.updateNovelVideoProject,
    analyzeNovelVideoProject: apiMocks.analyzeNovelVideoProject,
    generateNovelVideoImagePlan: apiMocks.generateNovelVideoImagePlan,
    updateNovelVideoCreature: apiMocks.updateNovelVideoCreature,
    updateNovelVideoActor: apiMocks.updateNovelVideoActor,
    generateNovelVideoActorLockSheet: apiMocks.generateNovelVideoActorLockSheet,
    generateNovelVideoCreatureImage: apiMocks.generateNovelVideoCreatureImage,
    generateNovelVideoAssets: apiMocks.generateNovelVideoAssets,
    dedupeNovelVideoAssets: apiMocks.dedupeNovelVideoAssets,
    updateNovelVideoAsset: apiMocks.updateNovelVideoAsset,
    deleteNovelVideoAsset: apiMocks.deleteNovelVideoAsset,
    planNovelVideoEpisodes: apiMocks.planNovelVideoEpisodes,
    updateNovelVideoShot: apiMocks.updateNovelVideoShot,
    renderNovelVideoApprovedShots: apiMocks.renderNovelVideoApprovedShots,
    renderNovelVideoPreflight: apiMocks.renderNovelVideoPreflight,
    queueNovelVideoRender: apiMocks.queueNovelVideoRender,
    generateNovelVideoStoryboard: apiMocks.generateNovelVideoStoryboard,
    generateNovelVideoGrids: apiMocks.generateNovelVideoGrids,
    generateNovelVideoShotImages: apiMocks.generateNovelVideoShotImages,
    listNovelVideoShotImages: apiMocks.listNovelVideoShotImages,
    updateNovelVideoShotImage: apiMocks.updateNovelVideoShotImage,
    getNovelVideoCostEstimate: apiMocks.getNovelVideoCostEstimate,
    composeNovelVideoProject: apiMocks.composeNovelVideoProject,
    listNovelVideoCompositions: apiMocks.listNovelVideoCompositions,
    listNovelVideoEvents: apiMocks.listNovelVideoEvents,
    exportNovelVideoProject: apiMocks.exportNovelVideoProject,
    exportNovelVideoProjectJSON: apiMocks.exportNovelVideoProjectJSON,
    exportNovelVideoProjectPackage: apiMocks.exportNovelVideoProjectPackage
  }
}))

vi.mock('../utils/novelSourceFileParser.js', () => ({
  parseNovelSourceFile: parserMocks.parseNovelSourceFile
}))

import NovelVideoWorkspaceView from '../views/NovelVideoWorkspaceView.vue'
import ThemeToggle from '../components/ThemeToggle.vue'
import { clearCurrentUser } from '../stores/session.js'
import { chooseClickSelect, clickSelectOption } from './click-select-test-utils.js'

enableAutoUnmount(afterEach)

const novelWorkspaceSource = readFileSync(resolve(__dirname, '../views/NovelVideoWorkspaceView.vue'), 'utf8')
const grokImagineVideoModel = 'grok-imagine-video-1.5-preview'
const seedance2VideoModel = 'doubao-seedance-2-0-260128'
const videoModelCapabilities = [
  {
    name: 'Grok Imagine',
    runtime_model: grokImagineVideoModel,
    aspect_ratios: ['16:9', '9:16'],
    durations: ['1', '3', '6', '10', '15'],
    default_duration: '3',
    resolution_options: [],
    supports_generate_audio: false
  },
  {
    name: 'Doubao Seedance 2.0',
    runtime_model: seedance2VideoModel,
    aspect_ratios: ['16:9', '9:16'],
    durations: ['4', '5', '6', '7', '8', '9', '10', '11', '12', '13', '14', '15', '-1'],
    default_duration: '10',
    resolution_options: ['720p', '1080p'],
    default_resolution: '720p',
    supports_reference_video: true,
    supports_reference_audio: true,
    supports_generate_audio: true
  }
]

const analyzedProject = {
  id: 7,
  title: '灰塔兽群',
  source_text: '灰塔里有三种守门兽。',
  source_chars: 11,
  style_preset: '冷峻写实',
  aspect_ratio: '16:9',
  duration: '10',
  video_model: grokImagineVideoModel,
  status: 'analyzed',
  story_bible: { logline: '守塔人穿过兽群抵达塔顶' },
  content_risk_summary: '未发现明确高风险内容',
  content_mode: 'narration',
  schema_version: 2,
  generation_mode: 'storyboard',
  grid_size: 4,
  assets: [
    {
      id: 71,
      kind: 'scene',
      name: '灰塔走廊',
      description: '潮湿石塔走廊',
      prompt: '冷色写实灰塔走廊',
      version: 1,
      review_status: 'needs_review'
    }
  ],
  jobs: [],
  compositions: [],
  creatures: [
    {
      id: 21,
      name: '灰鳞门兽',
      creature_type: '守门生物',
      appearance: '岩片背脊',
      abilities: '听见石墙脚步',
      visual_consistency_prompt: '同一只灰鳞门兽',
      review_status: 'needs_review'
    }
  ],
  episodes: []
}

const plannedProject = {
  ...analyzedProject,
  status: 'planned',
  creatures: [{ ...analyzedProject.creatures[0], review_status: 'approved' }],
  episodes: [
    {
      id: 31,
      number: 1,
      title: '进入灰塔',
      summary: '主角发现兽群不是敌人。',
      status: 'needs_review',
      shots: [
        {
          id: 41,
          number: 1,
          title: '门兽抬头',
          prompt: '低机位，灰鳞门兽从雾气中抬头。',
          script_unit_type: 'action',
          source_excerpt: '门兽从雾气中抬头',
          duration_seconds: 6,
          image_prompt: '灰塔雾气中的门兽',
          video_prompt: '镜头低机位推进门兽抬头',
          voiceover_text: '门兽在雾里看见了他',
          asset_refs: [{ type: 'creature', id: 21, name: '灰鳞门兽' }],
          status: 'needs_review'
        }
      ]
    }
  ]
}

const imageSeriesProject = {
  ...plannedProject,
  title: '夜航候选图',
  content_mode: 'short_film_image',
  generation_mode: 'image_series',
  schema_version: 3,
  assets: [
    {
      id: 71,
      kind: 'actor_ref',
      name: '林岚参考图',
      description: '正脸、半身、全身三张强参考',
      prompt: '林岚，短发，深色风衣，自然光，表情中性',
      version: 1,
      review_status: 'approved',
      metadata: { actor_id: 21, lock_level: 'strict', approved: true }
    },
    {
      id: 72,
      kind: 'scene',
      name: '旧码头',
      description: '雨夜旧码头',
      prompt: '湿润地面、远处船灯',
      version: 1,
      review_status: 'approved'
    }
  ],
  creatures: [
    {
      id: 21,
      name: '林岚',
      creature_type: '主演演员',
      appearance: '短发，深色风衣，自然光，表情中性',
      abilities: '克制、敏锐',
      visual_consistency_prompt: '保持同一位林岚演员的五官、发型、服装主轮廓',
      review_status: 'needs_review',
      reference_asset_ids: [71],
      lock_level: 'strict'
    }
  ],
  episodes: [
    {
      id: 31,
      number: 1,
      title: '旧码头夜航',
      summary: '林岚在旧码头发现失踪线索。',
      status: 'needs_review',
      shots: [
        {
          id: 41,
          number: 1,
          title: '林岚近景',
          prompt: '电影剧照，林岚站在雨夜旧码头，神情克制。',
          script_unit_type: 'action',
          source_excerpt: '林岚停在码头边。',
          duration_seconds: 6,
          image_prompt: '林岚单人近景，雨夜旧码头',
          video_prompt: '',
          voiceover_text: '',
          asset_refs: [
            { type: 'actor', id: 21, role: 'lead', weight: 1, lock_level: 'strict' },
            { type: 'scene', id: 72, role: 'location', weight: 0.7 }
          ],
          creature_ids: [21],
          reference_asset_ids: [71, 72],
          status: 'approved'
        }
      ]
    }
  ]
}

const shotImageCandidates = [
  {
    id: 91,
    shot_id: 41,
    generation_record_id: 301,
    kind: 'shot_image',
    version: 1,
    prompt: '林岚单人近景，雨夜旧码头',
    actor_ids: [21],
    reference_asset_ids: [71, 72],
    reference_intent: 'compose',
    mode: 'text_to_image',
    lock_level: 'strict',
    selected: false,
    review_status: 'needs_review',
    preview_url: '/generated/shot-91.png'
  },
  {
    id: 92,
    shot_id: 41,
    generation_record_id: 302,
    kind: 'shot_image',
    version: 2,
    prompt: '林岚单人近景，雨夜旧码头',
    actor_ids: [21],
    reference_asset_ids: [71, 72],
    reference_intent: 'compose',
    mode: 'text_to_image',
    lock_level: 'strict',
    selected: false,
    review_status: 'needs_review',
    preview_url: '/generated/shot-92.png'
  }
]

const secondApprovedShot = {
  ...imageSeriesProject.episodes[0].shots[0],
  id: 42,
  number: 2,
  title: '鏋楀矚杩滄櫙',
  prompt: '鐢靛奖鍓х収锛屾灄宀氳蛋鍚戞棫鐮佸ご杩滃',
  image_prompt: '鏋楀矚杩滄櫙锛岄洦澶滄棫鐮佸ご',
  status: 'approved'
}

const twoShotImageSeriesProject = {
  ...imageSeriesProject,
  episodes: [{
    ...imageSeriesProject.episodes[0],
    shots: [imageSeriesProject.episodes[0].shots[0], secondApprovedShot]
  }]
}

async function openEpisodeShots(wrapper, episodeID = 31) {
  await wrapper.get(`[data-testid="episode-card-${episodeID}"]`).trigger('click')
}

describe('NovelVideoWorkspaceView', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    clearCurrentUser()
    window.localStorage.clear()
    window.history.pushState({}, '', '/workspace/novel-video')
    apiMocks.getMe.mockResolvedValue({ user_id: 12, available_credits: 80 })
    apiMocks.listVideoModels.mockResolvedValue({ items: videoModelCapabilities })
    apiMocks.listNovelVideoProjects.mockResolvedValue({
      items: [
        {
          id: 8,
          title: '时间裂缝旅店',
          status: 'planned',
          updated_at: '2026-06-26T08:00:00Z',
          source_chars: 3210
        },
        {
          id: 7,
          title: '灰塔兽群',
          status: 'analyzed',
          updated_at: '2026-06-25T08:00:00Z',
          source_chars: 11
        }
      ]
    })
    apiMocks.getNovelVideoProject.mockResolvedValue(plannedProject)
    apiMocks.renderNovelVideoPreflight.mockResolvedValue({
      status: 'ready',
      renderable: 1,
      blocked: 0,
      required_credits: 2,
      available_credits: 80,
      enough: true,
      shots: []
    })
    apiMocks.generateNovelVideoAssets.mockResolvedValue({
      items: analyzedProject.assets
    })
    apiMocks.dedupeNovelVideoAssets.mockResolvedValue({
      removed: 0,
      items: analyzedProject.assets
    })
    apiMocks.deleteNovelVideoAsset.mockResolvedValue({
      deleted_id: 71,
      items: [],
      jobs: []
    })
    apiMocks.generateNovelVideoImagePlan.mockResolvedValue(imageSeriesProject)
    apiMocks.updateNovelVideoActor.mockResolvedValue({
      ...imageSeriesProject.creatures[0],
      review_status: 'approved',
      lock_level: 'strict',
      reference_asset_ids: [71]
    })
    apiMocks.generateNovelVideoActorLockSheet.mockResolvedValue({
      item: imageSeriesProject.assets[0]
    })
    apiMocks.queueNovelVideoRender.mockResolvedValue({
      status: 'queued',
      queued: 1,
      jobs: [{ id: 501, type: 'shot_video', status: 'queued', shot_id: 41 }]
    })
    apiMocks.generateNovelVideoStoryboard.mockResolvedValue({
      job: { id: 502, type: 'storyboard', status: 'queued', shot_id: 41 }
    })
    apiMocks.generateNovelVideoGrids.mockResolvedValue({
      items: [{ id: 81, grid_type: 'grid_4', grid_size: 4, shot_ids: [41] }]
    })
    apiMocks.generateNovelVideoShotImages.mockResolvedValue({
      queued: 2,
      total_candidates: 2,
      items: shotImageCandidates,
      reference_warnings: []
    })
    apiMocks.listNovelVideoShotImages.mockResolvedValue({ items: shotImageCandidates })
    apiMocks.updateNovelVideoShotImage.mockResolvedValue({
      ...shotImageCandidates[0],
      selected: true,
      review_status: 'approved'
    })
    apiMocks.getNovelVideoCostEstimate.mockResolvedValue({
      project: { total_credits: 3, shot_credits: 2, grid_credits: 1 },
      episodes: [{ episode_id: 31, shot_credits: 2, grid_credits: 1 }],
      shots: [{ shot_id: 41, render_credits: 2 }]
    })
    apiMocks.composeNovelVideoProject.mockResolvedValue({
      id: 61,
      status: 'succeeded',
      output_url: '/exports/final.mp4'
    })
    apiMocks.listNovelVideoCompositions.mockResolvedValue({ items: [] })
    apiMocks.listNovelVideoEvents.mockResolvedValue({ items: [] })
    apiMocks.exportNovelVideoProjectPackage.mockResolvedValue(new Blob(['zip']))
    window.IntersectionObserver = vi.fn(() => ({
      observe: vi.fn(),
      unobserve: vi.fn(),
      disconnect: vi.fn()
    }))
    window.URL.createObjectURL = vi.fn(() => 'blob:novel-json')
    window.URL.revokeObjectURL = vi.fn()
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    parserMocks.parseNovelSourceFile.mockResolvedValue({ text: '导入后的小说正文', format: 'txt' })
  })

  afterEach(() => {
    vi.useRealTimers()
    window.history.pushState({}, '', '/workspace/novel-video')
    window.localStorage.clear()
  })

  it('renders a three-column studio shell with six production sections', async () => {
    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    expect(wrapper.find('.novel-studio-sidebar').exists()).toBe(true)
    expect(wrapper.find('.novel-studio-main').exists()).toBe(true)
    expect(wrapper.find('.novel-studio-inspector').exists()).toBe(true)
    for (const text of ['小说导入', '故事圣经', '演员锁定', '资产板', '镜头图片', '批量任务', '导出镜头包']) {
      expect(wrapper.text()).toContain(text)
    }
  })

  it('removes duplicate inner sidebar links while keeping the top collapse action', async () => {
    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    expect(wrapper.find('.sidebar-links').exists()).toBe(false)
    expect(wrapper.find('.sidebar-user').exists()).toBe(false)

    const sidebar = wrapper.get('.novel-studio-sidebar')
    const toggle = wrapper.get('[data-testid="novel-sidebar-toggle"]')

    await toggle.trigger('click')

    expect(sidebar.classes()).toContain('collapsed')
  })

  it('opens asset detail in the inspector when an asset card is selected', async () => {
    window.history.pushState({}, '', '/workspace/novel-video?project_id=7')
    apiMocks.getNovelVideoProject.mockResolvedValueOnce({
      ...imageSeriesProject,
      assets: [
        {
          ...imageSeriesProject.assets[0],
          asset_url: '/assets/actor-71.png',
          metadata: { source: 'image_plan', content_mode: 'short_film_image' },
          error_message: 'provider timeout'
        },
        imageSeriesProject.assets[1]
      ]
    })
    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    await wrapper.get('[data-testid="asset-card-71"]').trigger('click')
    await flushPromises()

    const inspector = wrapper.get('[data-testid="asset-inspector"]')
    expect(inspector.text()).toContain('林岚参考图')
    expect(inspector.text()).toContain('actor_ref')
    expect(wrapper.get('[data-testid="asset-inspector-prompt"]').element.value).toContain('林岚')
    expect(wrapper.text()).toContain('image_plan')
    expect(wrapper.text()).toContain('short_film_image')
    expect(wrapper.get('[data-testid="asset-card-71"]').classes()).toContain('selected')
  })

  it('deletes a selected asset after confirmation and refreshes the asset board', async () => {
    window.history.pushState({}, '', '/workspace/novel-video?project_id=7')
    apiMocks.getNovelVideoProject.mockResolvedValueOnce(imageSeriesProject)
    apiMocks.deleteNovelVideoAsset.mockResolvedValueOnce({
      deleted_id: 71,
      items: [imageSeriesProject.assets[1]],
      jobs: []
    })
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    await wrapper.get('[data-testid="asset-card-71"]').trigger('click')
    await wrapper.get('[data-testid="asset-delete-71"]').trigger('click')
    await flushPromises()

    expect(window.confirm).toHaveBeenCalled()
    expect(apiMocks.deleteNovelVideoAsset).toHaveBeenCalledWith(7, 71)
    expect(wrapper.find('[data-testid="asset-card-71"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="asset-card-72"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="asset-inspector"]').exists()).toBe(false)
  })

  it('allows queued asset delete controls to cancel the queue and refresh active jobs', async () => {
    window.history.pushState({}, '', '/workspace/novel-video?project_id=7')
    apiMocks.getNovelVideoProject.mockResolvedValueOnce({
      ...imageSeriesProject,
      jobs: [{ id: 801, type: 'asset_image', status: 'queued', asset_id: 71 }]
    })
    apiMocks.deleteNovelVideoAsset.mockResolvedValueOnce({
      deleted_id: 71,
      items: [imageSeriesProject.assets[1]],
      jobs: []
    })
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    const button = wrapper.get('[data-testid="asset-delete-71"]')
    expect(button.attributes('disabled')).toBeUndefined()
    expect(button.attributes('title')).toBe('解除排队并删除资产')
    expect(wrapper.find('[data-testid="asset-generation-status"]').exists()).toBe(true)

    await button.trigger('click')
    await flushPromises()

    expect(window.confirm).toHaveBeenCalledWith(expect.stringContaining('取消排队'))
    expect(apiMocks.deleteNovelVideoAsset).toHaveBeenCalledWith(7, 71)
    expect(wrapper.find('[data-testid="asset-card-71"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="asset-generation-status"]').exists()).toBe(false)
  })

  it('keeps running asset delete controls disabled while images are generating', async () => {
    window.history.pushState({}, '', '/workspace/novel-video?project_id=7')
    apiMocks.getNovelVideoProject.mockResolvedValueOnce({
      ...imageSeriesProject,
      jobs: [{ id: 802, type: 'asset_image', status: 'running', asset_id: 71 }]
    })
    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    const button = wrapper.get('[data-testid="asset-delete-71"]')
    expect(button.attributes('disabled')).toBeDefined()
    expect(button.attributes('title')).toBe('生成中的资产不能删除')
  })

  it('shows backend asset delete conflicts without removing the card', async () => {
    window.history.pushState({}, '', '/workspace/novel-video?project_id=7')
    apiMocks.getNovelVideoProject.mockResolvedValueOnce(imageSeriesProject)
    apiMocks.deleteNovelVideoAsset.mockRejectedValueOnce(new Error('资产已被镜头 1-1 引用'))
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    await wrapper.get('[data-testid="asset-delete-71"]').trigger('click')
    await flushPromises()

    expect(apiMocks.deleteNovelVideoAsset).toHaveBeenCalledWith(7, 71)
    expect(wrapper.find('[data-testid="asset-card-71"]').exists()).toBe(true)
    expect(wrapper.get('.studio-error').text()).toContain('资产已被镜头 1-1 引用')
  })

  it('opens project history and loads the selected project into the workspace', async () => {
    const historyProject = {
      ...imageSeriesProject,
      id: 8,
      title: '时间裂缝旅店',
      source_text: '第七间客房每晚倒退一分钟。',
      story_bible: {
        logline: '旅店老板必须在黎明前修复时间裂缝。',
        world: '旧城区的钟表旅店',
        conflict: '客人记忆不断倒带',
        visual_style: '冷暖交错的悬疑电影感'
      },
      content_risk_summary: '无明显高风险内容',
      images: shotImageCandidates
    }
    apiMocks.getNovelVideoProject.mockResolvedValueOnce(historyProject)
    const wrapper = mount(NovelVideoWorkspaceView, {
      attachTo: document.body
    })
    await flushPromises()

    await wrapper.get('[data-testid="novel-history-open"]').trigger('click')
    await flushPromises()

    expect(apiMocks.listNovelVideoProjects).toHaveBeenCalled()
    expect(wrapper.get('[data-testid="novel-history-panel"]').text()).toContain('时间裂缝旅店')

    await wrapper.get('[data-testid="novel-history-project-8"]').trigger('click')
    await flushPromises()

    expect(apiMocks.getNovelVideoProject).toHaveBeenCalledWith(8)
    expect(wrapper.get('[data-testid="novel-title"]').element.value).toBe('时间裂缝旅店')
    expect(wrapper.get('[data-testid="novel-source"]').element.value).toBe('第七间客房每晚倒退一分钟。')
    await openEpisodeShots(wrapper)
    expect(wrapper.findAll('[data-testid^="shot-image-card-"]')).toHaveLength(2)
    expect(window.location.pathname + window.location.search).toBe('/workspace/novel-video?project_id=8')
    expect(wrapper.find('[data-testid="novel-history-panel"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it('opens history from an empty inspector and restores an uncreated local draft', async () => {
    window.localStorage.setItem('novel-video:draft:12:new', JSON.stringify({
      title: '本机未创建草稿',
      source_text: '这段小说还没有创建服务器项目。',
      style_preset: '赛博悬疑',
      content_mode: 'drama',
      generation_mode: 'grid',
      grid_size: 6,
      video_settings: {
        model: seedance2VideoModel,
        aspect_ratio: '9:16',
        duration: '11',
        resolution: '1080p',
        generate_audio: true
      },
      story_bible: {
        logline: '未创建项目的本机故事线'
      }
    }))
    const wrapper = mount(NovelVideoWorkspaceView, {
      attachTo: document.body
    })
    await flushPromises()

    await wrapper.get('[data-testid="novel-inspector-history-open"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="novel-history-draft-new"]').text()).toContain('本机未创建草稿')

    await wrapper.get('[data-testid="novel-history-draft-new"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="novel-title"]').element.value).toBe('本机未创建草稿')
    expect(wrapper.get('[data-testid="novel-source"]').element.value).toBe('这段小说还没有创建服务器项目。')
    expect(wrapper.get('[data-testid="novel-style"]').element.value).toBe('赛博悬疑')
    expect(wrapper.get('[data-testid="novel-grid-size"]').text()).toContain('6 grid')
    expect(window.location.pathname + window.location.search).toBe('/workspace/novel-video')

    wrapper.unmount()
  })

  it('shows a restore prompt for an existing project local draft without overwriting server data', async () => {
    window.history.pushState({}, '', '/workspace/novel-video?project_id=7')
    window.localStorage.setItem('novel-video:draft:12:project:7', JSON.stringify({
      title: '本机覆盖标题',
      source_text: '本机覆盖正文',
      style_preset: '本机风格',
      story_bible: {
        logline: '本机故事圣经'
      }
    }))

    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    expect(wrapper.get('[data-testid="novel-title"]').element.value).toBe(plannedProject.title)
    expect(wrapper.get('[data-testid="novel-local-draft-notice"]').text()).toContain('本机草稿')

    await wrapper.get('[data-testid="novel-local-draft-restore"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="novel-title"]').element.value).toBe('本机覆盖标题')
    expect(wrapper.get('[data-testid="novel-source"]').element.value).toBe('本机覆盖正文')
  })

  it('uses Grok Imagine by default and limits novel video durations to supported values', async () => {
    const wrapper = mount(NovelVideoWorkspaceView, {
      attachTo: document.body
    })
    await flushPromises()

    expect(wrapper.get('[data-testid="novel-video-model"]').text()).toContain('Grok Imagine')
    expect(wrapper.get('[data-testid="novel-duration"]').text()).toContain('3')

    await wrapper.get('[data-testid="novel-duration"]').trigger('click')

    expect(clickSelectOption('novel-duration', '1')).not.toBeNull()
    expect(clickSelectOption('novel-duration', '3')).not.toBeNull()
    expect(clickSelectOption('novel-duration', '6')).not.toBeNull()
    expect(clickSelectOption('novel-duration', '10')).not.toBeNull()
    expect(clickSelectOption('novel-duration', '15')).not.toBeNull()
    expect(clickSelectOption('novel-duration', '25')).toBeNull()

    wrapper.unmount()
  })

  it('loads Seedance 2.0 video settings from model capabilities', async () => {
    const wrapper = mount(NovelVideoWorkspaceView, {
      attachTo: document.body
    })
    await flushPromises()

    await chooseClickSelect(wrapper, 'novel-video-model', seedance2VideoModel)
    await chooseClickSelect(wrapper, 'novel-duration', '11')
    await chooseClickSelect(wrapper, 'novel-resolution', '1080p')
    await wrapper.get('[data-testid="novel-generate-audio"]').setValue(true)

    await wrapper.get('[data-testid="novel-title"]').setValue('灰塔兽群')
    await wrapper.get('[data-testid="novel-source"]').setValue('灰塔里有三种守门兽。')
    apiMocks.createNovelVideoProject.mockResolvedValueOnce({
      ...analyzedProject,
      id: 7,
      video_settings: {
        model: seedance2VideoModel,
        aspect_ratio: '16:9',
        duration: '11',
        resolution: '1080p',
        generate_audio: true
      }
    })
    apiMocks.analyzeNovelVideoProject.mockResolvedValueOnce(analyzedProject)

    await wrapper.get('[data-testid="novel-create"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createNovelVideoProject).toHaveBeenCalledWith(expect.objectContaining({
      video_settings: expect.objectContaining({
        model: seedance2VideoModel,
        duration: '11',
        resolution: '1080p',
        generate_audio: true
      })
    }))

    wrapper.unmount()
  })

  it('uses the shared light theme and follows the global theme toggle back to dark', async () => {
    window.localStorage.setItem('image_agent_user_theme:v1', 'light')
    const wrapper = mount({
      components: { NovelVideoWorkspaceView, ThemeToggle },
      template: '<ThemeToggle /><NovelVideoWorkspaceView />'
    })
    await flushPromises()

    const shell = wrapper.get('.novel-studio-shell')
    const toggle = wrapper.get('[data-testid="site-theme-toggle"]')

    expect(shell.attributes('data-theme')).toBe('light')
    expect(shell.classes()).toContain('novel-studio-light')
    expect(toggle.attributes('aria-label')).toBe('切换到暗色模式')

    await toggle.trigger('click')

    expect(shell.attributes('data-theme')).toBe('dark')
    expect(shell.classes()).toContain('novel-studio-dark')
    expect(window.localStorage.getItem('image_agent_user_theme:v1')).toBe('dark')
  })

  it('defines light theme tokens for novel video studio surfaces', () => {
    expect(novelWorkspaceSource).toContain('.novel-studio-shell[data-theme="light"]')
    for (const token of [
      '--nv-bg:',
      '--nv-sidebar:',
      '--nv-panel:',
      '--nv-panel-2:',
      '--nv-input:',
      '--nv-button:',
      '--nv-hover:',
      '--nv-progress-track:',
      '--nv-code-text:'
    ]) {
      expect(novelWorkspaceSource).toContain(token)
    }
  })

  it('uses theme tokens for major studio surfaces instead of fixed dark surfaces', () => {
    const styleSource = novelWorkspaceSource.slice(novelWorkspaceSource.indexOf('<style scoped>'))

    expect(styleSource).toContain('background: var(--nv-bg);')
    expect(styleSource).toContain('background: var(--nv-panel);')
    expect(styleSource).toContain('background: var(--nv-input);')
    expect(styleSource).toContain('background: var(--nv-button);')
    expect(styleSource).toContain('background: var(--nv-progress-track);')
    expect(styleSource).toContain('color: var(--nv-code-text);')
    expect(styleSource).not.toContain('background: #070b10;')
    expect(styleSource).not.toContain('background: #080d13;')
    expect(styleSource).not.toContain('background: #0b1118;')
    expect(styleSource).not.toContain('background: #101720;')
    expect(styleSource).not.toContain('background: #111923;')
  })

  it('keeps project settings controls inside a container-responsive grid', () => {
    const styleSource = novelWorkspaceSource.slice(novelWorkspaceSource.indexOf('<style scoped>'))

    expect(styleSource).toMatch(/\.import-grid\s*{[^}]*grid-template-columns:\s*minmax\(0,\s*1fr\)\s+minmax\(280px,\s*38%\);/s)
    expect(styleSource).toMatch(/\.settings-grid\s*{[^}]*grid-template-columns:\s*repeat\(auto-fit,\s*minmax\(min\(100%,\s*160px\),\s*1fr\)\);/s)
    expect(styleSource).toMatch(/\.settings-grid\s*>\s*\*\s*{[^}]*min-width:\s*0;/s)
    expect(styleSource).toMatch(/\.settings-grid\s+\.studio-field\.full\s*{[^}]*grid-column:\s*1\s*\/\s*-1;/s)
    expect(styleSource).toMatch(/\.settings-grid\s+:deep\(\.click-select-trigger\)\s*{[^}]*min-height:\s*40px;[^}]*padding:\s*10px\s+12px;/s)
  })

  it('switches active workflow step from the sidebar', async () => {
    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    await wrapper.get('[data-testid="workflow-step-creatures"]').trigger('click')

    expect(wrapper.get('[data-testid="workflow-step-creatures"]').classes()).toContain('active')
  })

  it('opens the creature inspector from a creature card', async () => {
    window.history.pushState({}, '', '/workspace/novel-video?project_id=7')
    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    await wrapper.get('[data-testid="creature-card-21"]').trigger('click')

    expect(wrapper.find('[data-testid="creature-inspector"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('灰鳞门兽')
  })

  it('uses a fixed-height actor card with aligned main content and a full-width footer', async () => {
    window.history.pushState({}, '', '/workspace/novel-video?project_id=7')
    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    const card = wrapper.get('[data-testid="creature-card-21"]')
    const main = card.get('.creature-card-main')
    expect(main.find('.creature-image').exists()).toBe(true)
    expect(main.find('.creature-summary').exists()).toBe(true)

    const footer = card.get('.creature-card-footer')
    expect(footer.find('.creature-consistency-prompt').exists()).toBe(true)
    expect(footer.find('.creature-reference-status').exists()).toBe(true)
    const actions = footer.get('.creature-card-actions')
    expect(actions.findAll('button')).toHaveLength(6)
    expect(actions.text()).toContain('批准')
    expect(actions.text()).toContain('锁定演员')
    expect(actions.text()).toContain('定妆图')
    expect(actions.text()).toContain('编辑')
    expect(actions.text()).toContain('设定图')
    expect(actions.text()).toContain('重试')

    expect(novelWorkspaceSource).toMatch(/\.creature-grid:not\(\.list\)\s+\.actor-card\s*{[^}]*height:\s*320px;/s)
    expect(novelWorkspaceSource).toMatch(/\.creature-grid\.list\s+\.actor-card\s*{[^}]*height:\s*auto;/s)
    expect(novelWorkspaceSource).toMatch(/\.creature-summary\s+p\s*{[^}]*-webkit-line-clamp:\s*2;/s)
    expect(novelWorkspaceSource).toMatch(/@media\s*\(max-width:\s*640px\)[\s\S]*\.creature-grid:not\(\.list\)\s+\.actor-card\s*{[^}]*height:\s*auto;/s)
  })

  it('opens the shot inspector and prompt editor from a shot row', async () => {
    window.history.pushState({}, '', '/workspace/novel-video?project_id=7')
    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    await openEpisodeShots(wrapper)
    await wrapper.get('[data-testid="shot-image-row-41"]').trigger('click')

    expect(wrapper.find('[data-testid="shot-inspector"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="prompt-editor"]').exists()).toBe(true)
  })

  it('exposes ArcReel-style asset board storyboard compose and package exports', async () => {
    window.history.pushState({}, '', '/workspace/novel-video?project_id=7')
    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    expect(wrapper.text()).toContain('资产板')
    expect(wrapper.text()).toContain('灰塔走廊')

    await wrapper.get('[data-testid="novel-generate-assets"]').trigger('click')
    await flushPromises()

    expect(apiMocks.generateNovelVideoAssets).toHaveBeenCalledWith(7, {
      kinds: ['character', 'scene', 'prop', 'clue', 'style']
    })

    await openEpisodeShots(wrapper)
    await wrapper.get('[data-testid="shot-image-row-41"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="novel-storyboard-41"]').trigger('click')
    await flushPromises()

    expect(apiMocks.generateNovelVideoStoryboard).toHaveBeenCalledWith(7, 41)
    expect(wrapper.text()).toContain('storyboard')

    apiMocks.updateNovelVideoShot.mockResolvedValueOnce({
      ...plannedProject.episodes[0].shots[0],
      video_prompt: '镜头绕行门兽，雾气打开',
      status: 'approved'
    })
    await wrapper.get('[data-testid="shot-video-prompt"]').setValue('镜头绕行门兽，雾气打开')
    await wrapper.get('[data-testid="shot-save-structured"]').trigger('click')
    await flushPromises()
    expect(apiMocks.updateNovelVideoShot).toHaveBeenCalledWith(7, 41, expect.objectContaining({
      video_prompt: '镜头绕行门兽，雾气打开',
      image_prompt: '灰塔雾气中的门兽',
      voiceover_text: '门兽在雾里看见了他',
      asset_refs: [{ type: 'creature', id: 21, name: '灰鳞门兽' }],
      asset_refs_set: true
    }))

    await wrapper.get('[data-testid="novel-generate-grids"]').trigger('click')
    await flushPromises()
    expect(apiMocks.generateNovelVideoGrids).toHaveBeenCalledWith(7, { grid_size: 4 })

    await wrapper.get('[data-testid="novel-cost-estimate"]').trigger('click')
    await flushPromises()
    expect(apiMocks.getNovelVideoCostEstimate).toHaveBeenCalledWith(7)
    expect(wrapper.get('[data-testid="novel-cost-estimate-summary"]').text()).toContain('3')

    await wrapper.get('[data-testid="novel-compose"]').trigger('click')
    await flushPromises()

    expect(apiMocks.composeNovelVideoProject).toHaveBeenCalledWith(7)
    expect(wrapper.text()).toContain('合成完成')

    await wrapper.get('[data-testid="export-mode-zip"]').trigger('click')
    await wrapper.get('[data-testid="novel-export"]').trigger('click')
    await flushPromises()
    expect(apiMocks.exportNovelVideoProjectPackage).toHaveBeenCalledWith(7, 'zip')

    await wrapper.get('[data-testid="export-mode-jianying"]').trigger('click')
    await wrapper.get('[data-testid="novel-export"]').trigger('click')
    await flushPromises()
    expect(apiMocks.exportNovelVideoProjectPackage).toHaveBeenCalledWith(7, 'jianying')
  })

  it('runs the short film image workflow with actor locking candidates review and image package export', async () => {
    window.history.pushState({}, '', '/workspace/novel-video?project_id=7')
    apiMocks.getNovelVideoProject.mockResolvedValueOnce(imageSeriesProject)
    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    expect(wrapper.text()).toContain('演员锁定')
    expect(wrapper.text()).toContain('镜头图片')
    expect(wrapper.text()).toContain('林岚')

    await wrapper.get('[data-testid="novel-image-plan"]').trigger('click')
    await flushPromises()

    expect(apiMocks.generateNovelVideoImagePlan).toHaveBeenCalledWith(7, expect.objectContaining({
      shot_count: 20
    }))

    await wrapper.get('[data-testid="actor-approve-21"]').trigger('click')
    await flushPromises()

    expect(apiMocks.updateNovelVideoActor).toHaveBeenCalledWith(7, 21, expect.objectContaining({
      review_status: 'approved',
      lock_level: 'strict'
    }))

    await wrapper.get('[data-testid="actor-lock-sheet-21"]').trigger('click')
    await flushPromises()

    expect(apiMocks.generateNovelVideoActorLockSheet).toHaveBeenCalledWith(7, 21)

    await wrapper.get('[data-testid="novel-generate-shot-images"]').trigger('click')
    await flushPromises()

    expect(apiMocks.generateNovelVideoShotImages).toHaveBeenCalledWith(7, expect.objectContaining({
      shot_ids: [41],
      candidates_per_shot: 4,
      mode: 'text_to_image',
      lock_level: 'strict'
    }))
    await openEpisodeShots(wrapper)
    expect(wrapper.text()).toContain('候选图')
    expect(wrapper.findAll('[data-testid^="shot-image-card-"]')).toHaveLength(2)

    await wrapper.get('[data-testid="shot-image-select-91"]').trigger('click')
    await flushPromises()

    expect(apiMocks.updateNovelVideoShotImage).toHaveBeenCalledWith(7, 91, expect.objectContaining({
      selected: true,
      review_status: 'approved'
    }))

    await wrapper.get('[data-testid="export-mode-image-package"]').trigger('click')
    await wrapper.get('[data-testid="novel-export"]').trigger('click')
    await flushPromises()

    expect(apiMocks.exportNovelVideoProjectPackage).toHaveBeenCalledWith(7, 'image_package')
  })

  it('shows shot image production rows with thumbnails counts and candidate actions in step 04', async () => {
    window.history.pushState({}, '', '/workspace/novel-video?project_id=7')
    apiMocks.getNovelVideoProject.mockResolvedValueOnce({
      ...imageSeriesProject,
      images: [
        { ...shotImageCandidates[0], selected: true, review_status: 'approved', generation_status: 'succeeded', generation_stage: 'succeeded', generation_progress: 100 },
        { ...shotImageCandidates[1], generation_status: 'succeeded', generation_stage: 'succeeded', generation_progress: 100 }
      ]
    })
    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    const shotsSection = wrapper.get('[data-step-key="shots"]')
    expect(shotsSection.find('[data-testid="novel-generate-shot-images"]').exists()).toBe(true)

    await openEpisodeShots(wrapper)
    const row = wrapper.get('[data-testid="shot-image-row-41"]')
    expect(row.text()).toContain('2')
    expect(row.text()).toContain('100%')
    expect(row.find('[data-testid="shot-image-thumb-41"]').attributes('src')).toBe('/generated/shot-91.png')
    expect(row.find('[data-testid="shot-image-selected-41"]').exists()).toBe(true)

    await row.find('[data-testid="shot-image-open-candidates-41"]').trigger('click')
    await flushPromises()

    expect(wrapper.findAll('[data-testid^="shot-image-card-"]')).toHaveLength(2)
    await wrapper.get('[data-testid="shot-image-select-91"]').trigger('click')
    await flushPromises()
    expect(apiMocks.updateNovelVideoShotImage).toHaveBeenCalledWith(7, 91, expect.objectContaining({
      selected: true,
      review_status: 'approved'
    }))
  })

  it('opens episode shots in a paginated modal and resets pagination when reopened', async () => {
    window.history.pushState({}, '', '/workspace/novel-video?project_id=7')
    const shots = Array.from({ length: 12 }, (_, index) => ({
      ...imageSeriesProject.episodes[0].shots[0],
      id: 100 + index,
      number: index + 1,
      title: `镜头 ${index + 1}`,
      status: index === 0 ? 'failed' : 'approved'
    }))
    apiMocks.getNovelVideoProject.mockResolvedValueOnce({
      ...imageSeriesProject,
      episodes: [{ ...imageSeriesProject.episodes[0], shots }]
    })

    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    const shotsSection = wrapper.get('[data-step-key="shots"]')
    expect(shotsSection.find('[data-testid="episode-card-31"]').text()).toContain('第 1 集')
    expect(shotsSection.find('[data-testid="episode-card-31"]').text()).toContain('有失败')
    expect(shotsSection.find('.shot-table').exists()).toBe(false)
    expect(shotsSection.find('[data-testid="shot-image-grid"]').exists()).toBe(false)

    await shotsSection.get('[data-testid="episode-card-31"]').trigger('click')

    const modal = wrapper.get('[data-testid="episode-shots-modal"]')
    expect(modal.text()).toContain('第 1 集镜头详情')
    expect(modal.findAll('[data-testid^="shot-image-row-"]')).toHaveLength(10)
    expect(modal.text()).toContain('1-10 / 12')

    await modal.get('[data-testid="episode-shots-next-page"]').trigger('click')
    expect(modal.findAll('[data-testid^="shot-image-row-"]')).toHaveLength(2)
    expect(modal.text()).toContain('11-12 / 12')

    await modal.get('[data-testid="episode-shots-page-size"]').setValue('20')
    expect(modal.findAll('[data-testid^="shot-image-row-"]')).toHaveLength(12)
    expect(modal.text()).toContain('1-12 / 12')

    await modal.get('[data-testid="episode-shots-close"]').trigger('click')
    expect(wrapper.find('[data-testid="episode-shots-modal"]').exists()).toBe(false)
    await shotsSection.get('[data-testid="episode-card-31"]').trigger('click')
    expect(wrapper.get('[data-testid="episode-shots-page-size"]').element.value).toBe('10')
    expect(wrapper.get('[data-testid="episode-shots-modal"]').findAll('[data-testid^="shot-image-row-"]')).toHaveLength(10)
  })

  it('summarizes episode production states and closes an empty episode modal with Escape', async () => {
    window.history.pushState({}, '', '/workspace/novel-video?project_id=7')
    const baseShot = imageSeriesProject.episodes[0].shots[0]
    apiMocks.getNovelVideoProject.mockResolvedValueOnce({
      ...imageSeriesProject,
      images: [],
      episodes: [
        { id: 31, number: 1, shots: [{ ...baseShot, id: 41, status: 'failed' }] },
        { id: 32, number: 2, shots: [{ ...baseShot, id: 42, status: 'running' }] },
        { id: 33, number: 3, shots: [{ ...baseShot, id: 43, status: 'approved' }] },
        { id: 34, number: 4, shots: [] }
      ]
    })

    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    expect(wrapper.get('[data-testid="episode-card-31"]').text()).toContain('有失败')
    expect(wrapper.get('[data-testid="episode-card-32"]').text()).toContain('生成中')
    expect(wrapper.get('[data-testid="episode-card-33"]').text()).toContain('已完成')
    expect(wrapper.get('[data-testid="episode-card-34"]').text()).toContain('待生成')

    await openEpisodeShots(wrapper, 34)
    expect(wrapper.get('[data-testid="episode-shots-modal"]').text()).toContain('该集暂无镜头')
    expect(wrapper.find('[data-testid="episode-shots-page-size"]').exists()).toBe(false)

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="episode-shots-modal"]').exists()).toBe(false)
  })

  it('keeps semantic shot columns fixed and all row actions on one line', async () => {
    window.history.pushState({}, '', '/workspace/novel-video?project_id=7')
    apiMocks.getNovelVideoProject.mockResolvedValueOnce(imageSeriesProject)

    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()
    await openEpisodeShots(wrapper)

    const table = wrapper.get('[data-testid="episode-shots-table"]')
    expect(table.findAll('col')).toHaveLength(7)
    expect(table.find('col.shot-column-number').attributes('style')).toContain('60px')
    expect(table.find('col.shot-column-title').attributes('style')).toContain('200px')
    expect(table.find('col.shot-column-prompt').attributes('style')).toContain('280px')
    expect(table.find('col.shot-column-creatures').attributes('style')).toContain('90px')
    expect(table.find('col.shot-column-reference').attributes('style')).toContain('90px')
    expect(table.find('col.shot-column-status').attributes('style')).toContain('150px')
    expect(table.find('col.shot-column-actions').attributes('style')).toContain('300px')

    const actions = table.get('[data-testid="shot-row-actions-41"]')
    expect(actions.find('[data-testid="shot-image-open-candidates-41"]').exists()).toBe(true)
    expect(actions.find('[data-testid="shot-approve-41"]').exists()).toBe(true)
    expect(actions.find('[data-testid="novel-storyboard-41"]').exists()).toBe(true)
    expect(novelWorkspaceSource).toMatch(/\.shot-row-actions\s*{[^}]*flex-wrap:\s*nowrap;[^}]*white-space:\s*nowrap;/s)
    expect(novelWorkspaceSource).toMatch(/\.shot-prompt-clamp\s*{[^}]*overflow-wrap:\s*anywhere;/s)
  })

  it('keeps existing thumbnails visible while a new shot image generation is running', async () => {
    window.history.pushState({}, '', '/workspace/novel-video?project_id=7')
    const runningCandidate = {
      ...shotImageCandidates[1],
      id: 93,
      generation_record_id: 303,
      generation_status: 'running',
      generation_stage: 'requesting_provider',
      generation_progress: 35,
      preview_url: ''
    }
    apiMocks.getNovelVideoProject.mockResolvedValueOnce({
      ...imageSeriesProject,
      images: [
        { ...shotImageCandidates[0], selected: true, review_status: 'approved', generation_status: 'succeeded', generation_stage: 'succeeded', generation_progress: 100 },
        runningCandidate
      ]
    })
    apiMocks.generateNovelVideoShotImages.mockResolvedValueOnce({
      queued: 1,
      total_candidates: 1,
      items: [runningCandidate],
      reference_warnings: []
    })

    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()
    await wrapper.get('[data-testid="novel-generate-shot-images"]').trigger('click')
    await flushPromises()

    await openEpisodeShots(wrapper)
    const row = wrapper.get('[data-testid="shot-image-row-41"]')
    expect(row.find('[data-testid="shot-image-thumb-41"]').attributes('src')).toBe('/generated/shot-91.png')
    expect(row.text()).toContain('35%')
    expect(row.find('[data-testid="shot-image-progress-41"]').attributes('style')).toContain('35%')
  })

  it('shows failed shot image state and keeps long prompts clamped in the production table', async () => {
    window.history.pushState({}, '', '/workspace/novel-video?project_id=7')
    const failedCandidate = {
      ...shotImageCandidates[0],
      generation_status: 'failed',
      generation_stage: 'failed',
      generation_progress: 100,
      error_message: 'provider timeout',
      preview_url: ''
    }
    apiMocks.getNovelVideoProject.mockResolvedValueOnce({
      ...imageSeriesProject,
      episodes: [{
        ...imageSeriesProject.episodes[0],
        shots: [{
          ...imageSeriesProject.episodes[0].shots[0],
          prompt: 'Long prompt '.repeat(80)
        }]
      }],
      images: [failedCandidate]
    })

    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    await openEpisodeShots(wrapper)
    const row = wrapper.get('[data-testid="shot-image-row-41"]')
    expect(row.text()).toContain('provider timeout')
    const actions = row.get('[data-testid="shot-row-actions-41"]')
    expect(actions.find('[data-testid="shot-image-open-candidates-41"]').exists()).toBe(true)
    expect(actions.find('[data-testid="shot-image-retry-41"]').exists()).toBe(true)
    expect(actions.find('[data-testid="shot-approve-41"]').exists()).toBe(true)
    expect(actions.find('[data-testid="novel-storyboard-41"]').exists()).toBe(true)
    expect(novelWorkspaceSource).toContain('shot-prompt-clamp')
    expect(novelWorkspaceSource).toContain('-webkit-line-clamp: 2')
  })

  it('shows queued render stats and polls project details after rendering', async () => {
    vi.useFakeTimers()
    window.history.pushState({}, '', '/workspace/novel-video?project_id=7')
    const renderableProject = {
      ...plannedProject,
      episodes: [
        {
          ...plannedProject.episodes[0],
          shots: [{ ...plannedProject.episodes[0].shots[0], status: 'approved', estimated_credits: 2 }]
        }
      ]
    }
    apiMocks.queueNovelVideoRender.mockResolvedValueOnce({
      status: 'queued',
      queued: 1,
      skipped: 0,
      required_credits: 2,
      available_credits: 80,
      total: 1
    })
    apiMocks.getNovelVideoProject.mockResolvedValueOnce(renderableProject).mockResolvedValueOnce({
      ...plannedProject,
      status: 'rendering',
      episodes: [
        {
          ...plannedProject.episodes[0],
          shots: [{ ...plannedProject.episodes[0].shots[0], status: 'running', generation_progress: 65 }]
        }
      ]
    }).mockResolvedValueOnce({
      ...plannedProject,
      status: 'succeeded',
      episodes: [
        {
          ...plannedProject.episodes[0],
          shots: [{ ...plannedProject.episodes[0].shots[0], status: 'succeeded', generation_progress: 100 }]
        }
      ]
    })

    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    await wrapper.get('[data-testid="novel-render"]').trigger('click')
    await flushPromises()

    expect(apiMocks.renderNovelVideoPreflight).toHaveBeenCalledWith(7)
    expect(apiMocks.queueNovelVideoRender).toHaveBeenCalledWith(7)
    expect(wrapper.text()).toContain('预计点数2')
    expect(wrapper.text()).toContain('队列1')

    await vi.advanceTimersByTimeAsync(5000)
    await flushPromises()

    expect(apiMocks.getNovelVideoProject).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('paginates the render queue at ten rows per page', async () => {
    window.history.pushState({}, '', '/workspace/novel-video?project_id=7')
    const queueJobs = Array.from({ length: 12 }, (_, index) => ({
      id: 612 - index,
      type: index < 2 ? 'asset_image' : 'shot_video',
      status: index < 4 ? 'queued' : 'succeeded',
      progress: index < 4 ? 25 : 100
    }))
    apiMocks.getNovelVideoProject.mockResolvedValueOnce({
      ...plannedProject,
      episodes: [],
      jobs: queueJobs
    })

    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    expect(wrapper.findAll('.queue-table tbody tr')).toHaveLength(10)
    expect(wrapper.get('[data-testid="render-queue-summary"]').text()).toContain('1-10 / 12')
    expect(wrapper.text()).toContain('#612')
    expect(wrapper.text()).not.toContain('#602')

    const previousButton = wrapper.get('[data-testid="render-queue-prev"]')
    const nextButton = wrapper.get('[data-testid="render-queue-next"]')
    expect(previousButton.attributes('disabled')).toBeDefined()
    expect(nextButton.attributes('disabled')).toBeUndefined()

    await nextButton.trigger('click')
    await flushPromises()

    expect(wrapper.findAll('.queue-table tbody tr')).toHaveLength(2)
    expect(wrapper.get('[data-testid="render-queue-summary"]').text()).toContain('11-12 / 12')
    expect(wrapper.text()).toContain('#602')
    expect(wrapper.text()).toContain('#601')
    expect(wrapper.get('[data-testid="render-queue-next"]').attributes('disabled')).toBeDefined()
  })

  it('clamps the render queue page when polling returns fewer jobs', async () => {
    vi.useFakeTimers()
    window.history.pushState({}, '', '/workspace/novel-video?project_id=7')
    const queueJobs = Array.from({ length: 12 }, (_, index) => ({
      id: 712 - index,
      type: 'shot_video',
      status: 'queued',
      progress: 0
    }))
    apiMocks.getNovelVideoProject.mockResolvedValueOnce({
      ...plannedProject,
      episodes: [],
      jobs: queueJobs
    }).mockResolvedValueOnce({
      ...plannedProject,
      episodes: [],
      jobs: queueJobs.slice(0, 2)
    })
    apiMocks.listNovelVideoEvents.mockResolvedValueOnce({ items: queueJobs.slice(0, 2) })

    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    await wrapper.get('[data-testid="render-queue-next"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="render-queue-summary"]').text()).toContain('11-12 / 12')

    await vi.advanceTimersByTimeAsync(3000)
    await flushPromises()

    expect(wrapper.findAll('.queue-table tbody tr')).toHaveLength(2)
    expect(wrapper.find('[data-testid="render-queue-summary"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="render-queue-next"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('#712')
    wrapper.unmount()
  })

  it('polls project details after creature image generation returns an active record', async () => {
    vi.useFakeTimers()
    window.history.pushState({}, '', '/workspace/novel-video?project_id=7')
    const staleCreature = {
      ...imageSeriesProject.creatures[0],
      error_message: 'old failed message',
      latest_error: 'old failed message'
    }
    const activeCreature = {
      ...staleCreature,
      generation_record_id: 401,
      generation_status: 'queued',
      error_message: '',
      latest_error: ''
    }
    const finishedCreature = {
      ...activeCreature,
      generation_status: 'succeeded',
      asset_url: '/generated/creature-21.png',
      work_preview_url: '/generated/creature-21.png'
    }
    const finishedActorAsset = {
      ...imageSeriesProject.assets[0],
      asset_url: '/generated/creature-21.png',
      work_id: 991,
      generation_record_id: 401,
      metadata: { ...imageSeriesProject.assets[0].metadata, actor_id: 21 }
    }
    apiMocks.getNovelVideoProject.mockResolvedValueOnce({
      ...imageSeriesProject,
      creatures: [staleCreature]
    }).mockResolvedValueOnce({
      ...imageSeriesProject,
      creatures: [finishedCreature],
      assets: [finishedActorAsset, imageSeriesProject.assets[1]]
    })
    apiMocks.generateNovelVideoCreatureImage.mockResolvedValueOnce(activeCreature)

    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    const generateButton = wrapper.findAll('button').find((button) => button.text().includes('设定图'))
    expect(generateButton).toBeTruthy()
    await generateButton.trigger('click')
    await flushPromises()

    expect(apiMocks.generateNovelVideoCreatureImage).toHaveBeenCalledWith(7, 21)
    expect(wrapper.get('[data-testid="creature-generation-overlay-21"]').text()).toContain('排队中')
    expect(wrapper.get('[data-testid="creature-generation-overlay-21"]').text()).not.toContain('old failed message')

    await vi.advanceTimersByTimeAsync(5000)
    await flushPromises()

    expect(apiMocks.getNovelVideoProject).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-testid="creature-generation-overlay-21"]').exists()).toBe(false)
    expect(wrapper.find('img[src="/generated/creature-21.png"]').exists()).toBe(true)
    const actorAssetCards = wrapper.findAll('.asset-card').filter((card) => card.text().includes('actor_ref'))
    expect(actorAssetCards).toHaveLength(1)
    expect(actorAssetCards[0].find('img[src="/generated/creature-21.png"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('shows asset image job status on the board and refreshes the finished asset', async () => {
    vi.useFakeTimers()
    window.history.pushState({}, '', '/workspace/novel-video?project_id=7')
    const queuedAsset = {
      id: 81,
      kind: 'prop',
      name: '发光钥匙',
      description: '可复用关键道具',
      prompt: '冷色写实发光钥匙',
      version: 1,
      review_status: 'needs_review'
    }
    apiMocks.getNovelVideoProject.mockResolvedValueOnce({ ...analyzedProject, assets: [], jobs: [] }).mockResolvedValueOnce({
      ...analyzedProject,
      assets: [{ ...queuedAsset, asset_url: '/generated/asset-81.png' }],
      jobs: [{ id: 701, type: 'asset_image', status: 'succeeded', progress: 100, asset_id: 81 }]
    })
    apiMocks.generateNovelVideoAssets.mockResolvedValueOnce({
      items: [queuedAsset],
      jobs: [{ id: 701, type: 'asset_image', status: 'queued', progress: 0, asset_id: 81 }]
    })
    apiMocks.listNovelVideoEvents.mockResolvedValueOnce({
      items: [{ id: 701, type: 'asset_image', status: 'running', progress: 45, asset_id: 81 }]
    }).mockResolvedValueOnce({
      items: [{ id: 701, type: 'asset_image', status: 'succeeded', progress: 100, asset_id: 81 }]
    })

    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    await wrapper.get('[data-testid="novel-generate-assets"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="asset-generation-status"]').text()).toContain('正在生成资产图 0/1')
    expect(wrapper.get('[data-testid="asset-generation-overlay-81"]').text()).toContain('排队中')

    await vi.advanceTimersByTimeAsync(3000)
    await flushPromises()

    expect(apiMocks.listNovelVideoEvents).toHaveBeenCalledWith(7)
    expect(apiMocks.getNovelVideoProject).toHaveBeenCalledWith(7)

    await vi.advanceTimersByTimeAsync(3000)
    await flushPromises()

    expect(wrapper.find('[data-testid="asset-generation-overlay-81"]').exists()).toBe(false)
    expect(wrapper.find('img[src="/generated/asset-81.png"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('disables asset draft generation while asset images are active', async () => {
    window.history.pushState({}, '', '/workspace/novel-video?project_id=7')
    apiMocks.getNovelVideoProject.mockResolvedValueOnce({
      ...analyzedProject,
      assets: [{
        id: 81,
        kind: 'prop',
        name: '发光钥匙',
        description: '可复用关键道具',
        prompt: '冷色写实发光钥匙',
        version: 1,
        review_status: 'needs_review'
      }],
      jobs: [{ id: 701, type: 'asset_image', status: 'queued', progress: 0, asset_id: 81 }]
    })

    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    const button = wrapper.get('[data-testid="novel-generate-assets"]')
    expect(button.attributes('disabled')).toBeDefined()
    expect(button.text()).toContain('资产图生成中')

    await button.trigger('click')
    await flushPromises()

    expect(apiMocks.generateNovelVideoAssets).not.toHaveBeenCalled()
  })

  it('shows duplicate asset cleanup and refreshes the asset board after dedupe', async () => {
    window.history.pushState({}, '', '/workspace/novel-video?project_id=7')
    const firstAsset = {
      id: 81,
      kind: 'prop',
      name: '发光钥匙',
      description: '可复用关键道具',
      prompt: '冷色写实发光钥匙',
      version: 1,
      review_status: 'approved',
      metadata: { source: 'fallback' }
    }
    const duplicateAsset = {
      ...firstAsset,
      id: 82,
      review_status: 'needs_review'
    }
    apiMocks.getNovelVideoProject.mockResolvedValueOnce({
      ...analyzedProject,
      assets: [firstAsset, duplicateAsset],
      jobs: []
    })
    apiMocks.dedupeNovelVideoAssets.mockResolvedValueOnce({
      removed: 1,
      items: [firstAsset]
    })

    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    expect(wrapper.find('[data-testid="asset-dedupe-assets"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="asset-card-82"]').exists()).toBe(false)

    await wrapper.get('[data-testid="asset-dedupe-assets"]').trigger('click')
    await flushPromises()

    expect(apiMocks.dedupeNovelVideoAssets).toHaveBeenCalledWith(7)
    expect(wrapper.find('[data-testid="asset-card-82"]').exists()).toBe(false)
  })

  it('folds semantic duplicate assets on the board and keeps unsafe duplicates collapsed after dedupe', async () => {
    window.history.pushState({}, '', '/workspace/novel-video?project_id=7')
    const actorRef = {
      id: 71,
      kind: 'actor_ref',
      name: 'Lead actor reference',
      description: 'primary actor identity',
      prompt: 'same lead actor',
      version: 1,
      review_status: 'approved',
      metadata: { actor_id: 21, source: 'image_plan' }
    }
    const characterDuplicate = {
      id: 81,
      kind: 'character',
      name: 'Lead visual anchor',
      description: 'duplicate actor identity',
      prompt: 'same lead actor',
      version: 1,
      review_status: 'approved',
      metadata: { actor_id: 21, source: 'fallback' }
    }
    const scene = {
      id: 72,
      kind: 'scene',
      name: 'Harbor core scene',
      description: 'rainy harbor',
      prompt: 'rainy harbor scene',
      version: 1,
      review_status: 'approved',
      metadata: { source: 'image_plan' }
    }
    const sceneDuplicate = {
      ...scene,
      id: 82,
      review_status: 'needs_review',
      metadata: { source: 'fallback' }
    }
    apiMocks.getNovelVideoProject.mockResolvedValueOnce({
      ...imageSeriesProject,
      assets: [actorRef, characterDuplicate, scene, sceneDuplicate],
      jobs: []
    })
    apiMocks.dedupeNovelVideoAssets.mockResolvedValueOnce({
      removed: 1,
      removed_ids: [82],
      collapsed_ids: [81],
      items: [actorRef, characterDuplicate, scene]
    })

    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    expect(wrapper.find('[data-testid="asset-card-71"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="asset-card-72"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="asset-card-81"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="asset-card-82"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="asset-dedupe-assets"]').exists()).toBe(true)

    await wrapper.get('[data-testid="asset-dedupe-assets"]').trigger('click')
    await flushPromises()

    expect(apiMocks.dedupeNovelVideoAssets).toHaveBeenCalledWith(7)
    expect(wrapper.find('[data-testid="asset-card-71"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="asset-card-72"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="asset-card-81"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="asset-card-82"]').exists()).toBe(false)
  })

  it('keeps failed asset image errors visible with a retry action', async () => {
    window.history.pushState({}, '', '/workspace/novel-video?project_id=7')
    apiMocks.getNovelVideoProject.mockResolvedValueOnce({
      ...analyzedProject,
      assets: [{
        id: 81,
        kind: 'prop',
        name: '发光钥匙',
        description: '可复用关键道具',
        prompt: '冷色写实发光钥匙',
        version: 1,
        review_status: 'needs_review',
        error_message: 'provider unavailable'
      }],
      jobs: [{ id: 701, type: 'asset_image', status: 'failed', progress: 0, asset_id: 81, error_message: 'provider unavailable' }]
    })
    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    expect(wrapper.get('[data-testid="asset-generation-overlay-81"]').text()).toContain('provider unavailable')

    await wrapper.get('[data-testid="asset-retry-81"]').trigger('click')
    await flushPromises()

    expect(apiMocks.generateNovelVideoAssets).toHaveBeenCalledWith(7, expect.objectContaining({
      asset_id: 81
    }))
  })

  it('disables the creature inspector generate button while the image request is running', async () => {
    window.history.pushState({}, '', '/workspace/novel-video?project_id=7')
    let resolveCreatureImage
    apiMocks.generateNovelVideoCreatureImage.mockReturnValueOnce(new Promise((resolve) => {
      resolveCreatureImage = resolve
    }))
    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    await wrapper.get('[data-testid="creature-card-21"]').trigger('click')
    await wrapper.get('[data-testid="creature-inspector-generate"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="creature-inspector-generate"]').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('正在调用模型，通常需要几十秒')

    resolveCreatureImage({ ...plannedProject.creatures[0], asset_url: '/generated/creature-21.png', work_preview_url: '/generated/creature-21.png' })
    await flushPromises()
    wrapper.unmount()
  })

  it('shows shot candidates as generating and refreshes all project shot image rows', async () => {
    vi.useFakeTimers()
    window.history.pushState({}, '', '/workspace/novel-video?project_id=7')
    const runningSecondShotCandidate = {
      ...shotImageCandidates[0],
      id: 94,
      shot_id: 42,
      generation_record_id: 304,
      version: 1,
      generation_status: 'running',
      generation_stage: 'requesting_provider',
      generation_progress: 35,
      preview_url: ''
    }
    apiMocks.getNovelVideoProject.mockResolvedValueOnce(twoShotImageSeriesProject)
    apiMocks.generateNovelVideoShotImages.mockResolvedValueOnce({
      queued: 2,
      total_candidates: 2,
      items: [],
      reference_warnings: []
    })
    apiMocks.listNovelVideoShotImages.mockResolvedValueOnce({ items: [] }).mockResolvedValueOnce({ items: [...shotImageCandidates, runningSecondShotCandidate] })

    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    await openEpisodeShots(wrapper)
    await wrapper.get('[data-testid="novel-generate-shot-images"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="shot-image-grid"]').text()).toContain('候选图生成中')

    await vi.advanceTimersByTimeAsync(3000)
    await flushPromises()
    await vi.advanceTimersByTimeAsync(3000)
    await flushPromises()

    expect(apiMocks.listNovelVideoShotImages).toHaveBeenCalledWith(7, {})
    expect(wrapper.get('[data-testid="shot-image-progress-42"]').attributes('style')).toContain('35%')
    expect(wrapper.findAll('[data-testid^="shot-image-card-"]')).toHaveLength(2)
    wrapper.unmount()
  })

  it('retries only the failed shot image row', async () => {
    window.history.pushState({}, '', '/workspace/novel-video?project_id=7')
    const failedCandidate = {
      ...shotImageCandidates[0],
      generation_status: 'failed',
      generation_stage: 'failed',
      generation_progress: 100,
      error_message: 'provider timeout',
      preview_url: ''
    }
    apiMocks.getNovelVideoProject.mockResolvedValueOnce({
      ...twoShotImageSeriesProject,
      images: [failedCandidate]
    })
    apiMocks.generateNovelVideoShotImages.mockResolvedValueOnce({
      queued: 1,
      total_candidates: 1,
      items: [{ ...failedCandidate, id: 95, generation_status: 'queued', generation_stage: 'queued', error_message: '' }],
      reference_warnings: []
    })

    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    await openEpisodeShots(wrapper)
    await wrapper.get('[data-testid="shot-image-retry-41"]').trigger('click')
    await flushPromises()

    expect(apiMocks.generateNovelVideoShotImages).toHaveBeenCalledWith(7, expect.objectContaining({
      shot_ids: [41],
      candidates_per_shot: 1,
      mode: 'text_to_image'
    }))
  })

  it('regenerates a shot image from the current candidate work id', async () => {
    window.history.pushState({}, '', '/workspace/novel-video?project_id=7')
    const sourceCandidate = {
      ...shotImageCandidates[0],
      work_id: 501,
      generation_status: 'succeeded',
      generation_stage: 'succeeded',
      generation_progress: 100
    }
    apiMocks.getNovelVideoProject.mockResolvedValueOnce({
      ...imageSeriesProject,
      images: [sourceCandidate]
    })
    apiMocks.generateNovelVideoShotImages.mockResolvedValueOnce({
      queued: 1,
      total_candidates: 1,
      items: [{ ...sourceCandidate, id: 96, mode: 'image_to_image', generation_status: 'queued' }],
      reference_warnings: []
    })

    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    await openEpisodeShots(wrapper)
    await wrapper.get('[data-testid="shot-image-regenerate-91"]').trigger('click')
    await flushPromises()

    expect(apiMocks.generateNovelVideoShotImages).toHaveBeenCalledWith(7, expect.objectContaining({
      shot_ids: [41],
      candidates_per_shot: 1,
      mode: 'image_to_image',
      source_work_id: 501
    }))
  })

  it('shows render preflight blockers without queueing shots', async () => {
    window.history.pushState({}, '', '/workspace/novel-video?project_id=7')
    apiMocks.getNovelVideoProject.mockResolvedValueOnce({
      ...plannedProject,
      episodes: [
        {
          ...plannedProject.episodes[0],
          shots: [{ ...plannedProject.episodes[0].shots[0], status: 'approved', estimated_credits: 2 }]
        }
      ]
    })
    apiMocks.renderNovelVideoPreflight.mockResolvedValueOnce({
      status: 'blocked',
      renderable: 0,
      blocked: 1,
      required_credits: 0,
      available_credits: 80,
      enough: true,
      shots: [
        {
          shot_id: 41,
          episode_number: 1,
          shot_number: 1,
          title: '门兽抬头',
          blocked_reason: '参考视频素材不存在'
        }
      ]
    })

    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    await wrapper.get('[data-testid="novel-render"]').trigger('click')
    await flushPromises()

    expect(apiMocks.renderNovelVideoPreflight).toHaveBeenCalledWith(7)
    expect(apiMocks.renderNovelVideoApprovedShots).not.toHaveBeenCalled()
    expect(apiMocks.queueNovelVideoRender).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('参考视频素材不存在')
    expect(wrapper.get('.studio-error').text()).toContain('未通过预检')
  })

  it('shows backend create failure detail in the project inspector alert', async () => {
    apiMocks.createNovelVideoProject.mockRejectedValueOnce(new Error('小说视频项目数据表未初始化，请联系管理员执行数据库迁移后重试'))

    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    await wrapper.get('[data-testid="novel-title"]').setValue('灰塔兽群')
    await wrapper.get('[data-testid="novel-source"]').setValue('灰塔里有三种守门兽。')
    await wrapper.get('[data-testid="novel-create"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('.studio-error').text()).toBe('小说视频项目数据表未初始化，请联系管理员执行数据库迁移后重试')
  })

  it('moves a novel project through review planning rendering and export actions', async () => {
    apiMocks.createNovelVideoProject.mockResolvedValueOnce({
      id: 7,
      title: '灰塔兽群',
      source_text: '灰塔里有三种守门兽。',
      source_chars: 11,
      style_preset: '冷峻写实',
      aspect_ratio: '16:9',
      duration: '10',
      video_model: grokImagineVideoModel,
      status: 'draft',
      creatures: [],
      episodes: []
    })
    apiMocks.analyzeNovelVideoProject.mockResolvedValueOnce(analyzedProject)
    apiMocks.updateNovelVideoCreature.mockResolvedValueOnce({
      ...analyzedProject.creatures[0],
      review_status: 'approved'
    })
    apiMocks.planNovelVideoEpisodes.mockResolvedValueOnce(plannedProject)
    apiMocks.updateNovelVideoShot.mockResolvedValueOnce({
      ...plannedProject.episodes[0].shots[0],
      status: 'approved'
    })
    apiMocks.queueNovelVideoRender.mockResolvedValueOnce({ status: 'queued', queued: 1, skipped: 0, required_credits: 2, available_credits: 80, total: 1 })
    apiMocks.exportNovelVideoProject.mockResolvedValueOnce('# 灰塔兽群\n\n低机位')

    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    await wrapper.get('[data-testid="novel-title"]').setValue('灰塔兽群')
    await wrapper.get('[data-testid="novel-source"]').setValue('灰塔里有三种守门兽。')
    await wrapper.get('[data-testid="novel-style"]').setValue('冷峻写实')
    await wrapper.get('[data-testid="novel-create"]').trigger('click')
    await flushPromises()

    expect(apiMocks.createNovelVideoProject).toHaveBeenCalledWith(expect.objectContaining({
      title: '灰塔兽群',
      source_text: '灰塔里有三种守门兽。',
      style_preset: '冷峻写实'
    }))
    expect(apiMocks.createNovelVideoProject).toHaveBeenCalledWith(expect.objectContaining({
      duration: '3',
      video_model: grokImagineVideoModel,
      video_settings: expect.objectContaining({
        model: grokImagineVideoModel,
        duration: '3',
        aspect_ratio: '16:9'
      })
    }))
    expect(apiMocks.analyzeNovelVideoProject).toHaveBeenCalledWith(7)

    expect(wrapper.text()).toContain('灰鳞门兽')
    expect(wrapper.findAll('.bible-card textarea').some((item) => item.element.value === '守塔人穿过兽群抵达塔顶')).toBe(true)

    await wrapper.get('[data-testid="creature-approve-21"]').trigger('click')
    await flushPromises()

    expect(apiMocks.updateNovelVideoCreature).toHaveBeenCalledWith(7, 21, { review_status: 'approved' })
    expect(wrapper.text()).toContain('已批准')

    await wrapper.get('[data-testid="novel-plan-episodes"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('第 1 集')
    await openEpisodeShots(wrapper)
    expect(wrapper.text()).toContain('门兽抬头')

    await wrapper.get('[data-testid="shot-approve-41"]').trigger('click')
    await flushPromises()

    expect(apiMocks.updateNovelVideoShot).toHaveBeenCalledWith(7, 41, { status: 'approved' })

    await wrapper.get('[data-testid="novel-render"]').trigger('click')
    await flushPromises()

    expect(apiMocks.renderNovelVideoPreflight).toHaveBeenCalledWith(7)
    expect(apiMocks.queueNovelVideoRender).toHaveBeenCalledWith(7)
    expect(wrapper.text()).toContain('队列1')

    await wrapper.get('[data-testid="novel-export"]').trigger('click')
    await flushPromises()

    expect(apiMocks.exportNovelVideoProject).toHaveBeenCalledWith(7)
    expect(wrapper.get('[data-testid="novel-export-output"]').text()).toContain('低机位')
  })

  it('imports md txt docx and pdf files into the novel source textarea', async () => {
    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    for (const name of ['story.md', 'story.txt', 'story.docx', 'story.pdf']) {
      parserMocks.parseNovelSourceFile.mockResolvedValueOnce({ text: `${name} 正文`, format: name.split('.').pop() })

      const input = wrapper.get('[data-testid="novel-source-file"]')
      Object.defineProperty(input.element, 'files', { value: [new File(['fake'], name)], configurable: true })
      await input.trigger('change')
      await flushPromises()

      expect(wrapper.get('[data-testid="novel-source"]').element.value).toBe(`${name} 正文`)
      expect(wrapper.text()).toContain('已导入')
    }
  })

  it('truncates imported novel source text to 50000 characters and tells the user', async () => {
    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    parserMocks.parseNovelSourceFile.mockResolvedValueOnce({ text: '灰'.repeat(50010), format: 'txt' })
    const input = wrapper.get('[data-testid="novel-source-file"]')
    Object.defineProperty(input.element, 'files', { value: [new File(['fake'], 'long.txt')], configurable: true })
    await input.trigger('change')
    await flushPromises()

    expect(wrapper.get('[data-testid="novel-source"]').element.value).toHaveLength(50000)
    expect(wrapper.text()).toContain('已截断为前 50,000 字')
  })

  it('keeps existing novel source when document import fails', async () => {
    const wrapper = mount(NovelVideoWorkspaceView)
    await flushPromises()

    await wrapper.get('[data-testid="novel-source"]').setValue('原有正文')
    parserMocks.parseNovelSourceFile.mockRejectedValueOnce(new Error('暂不支持 .doc，请另存为 .docx 后再导入'))
    const input = wrapper.get('[data-testid="novel-source-file"]')
    Object.defineProperty(input.element, 'files', { value: [new File(['fake'], 'old.doc')] })
    await input.trigger('change')
    await flushPromises()

    expect(wrapper.get('[data-testid="novel-source"]').element.value).toBe('原有正文')
    expect(wrapper.text()).toContain('暂不支持 .doc')
  })
})
