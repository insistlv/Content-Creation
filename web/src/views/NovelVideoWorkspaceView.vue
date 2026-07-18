<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import {
  Check,
  ChevronLeft,
  ChevronRight,
  CircleAlert,
  Clock,
  Clapperboard,
  Copy,
  Download,
  Edit3,
  FileJson,
  FileText,
  History,
  Image,
  LayoutList,
  Library,
  LoaderCircle,
  Menu,
  Play,
  Plus,
  RefreshCw,
  Save,
  Settings,
  Sparkles,
  Trash2,
  Wand2,
  X
} from 'lucide-vue-next'
import { api } from '../api/client.js'
import ClickSelect from '../components/ClickSelect.vue'
import { useUserTheme } from '../composables/useUserTheme.js'
import { loadCurrentUser } from '../stores/session.js'
import { parseNovelSourceFile } from '../utils/novelSourceFileParser.js'

const title = ref('')
const sourceText = ref('')
const stylePreset = ref('电影感写实')
const contentMode = ref('short_film_image')
const generationMode = ref('image_series')
const gridSize = ref(4)
const aspectRatio = ref('16:9')
const duration = ref('10')
const grokImagineVideoModel = 'grok-imagine-video-1.5-preview'
const doubaoSeedance2Model = 'doubao-seedance-2-0-260128'
const RENDER_QUEUE_PAGE_SIZE = 10
const videoModel = ref(grokImagineVideoModel)
const resolution = ref('')
const generateAudio = ref(false)
const me = ref(null)
const project = ref(null)
const loading = ref(false)
const busyAction = ref('')
const message = ref('')
const errorMessage = ref('')
const exportOutput = ref('')
const activeStep = ref('import')
const selectedCreatureId = ref(null)
const selectedEpisodeId = ref(null)
const selectedShotId = ref(null)
const selectedAssetId = ref(null)
const collapsedAssetIds = ref(new Set())
const inspectorMode = ref('project')
const creatureViewMode = ref('cards')
const renderQueueExpanded = ref(true)
const renderQueuePage = ref(1)
const episodeShotsModalOpen = ref(false)
const episodeShotsPage = ref(1)
const episodeShotsPageSize = ref(10)
const exportMode = ref('markdown')
const projectSettingsOpen = ref(true)
const sidebarCollapsed = ref(false)
const inspectorOpen = ref(true)
const renderResult = ref(null)
const costEstimate = ref(null)
const gridItems = ref([])
const shotImages = ref([])
const imageBatchResult = ref(null)
const imagePlanShotCount = ref(20)
const imageCandidatesPerShot = ref(4)
const imageGenerationMode = ref('text_to_image')
const actorLockLevel = ref('strict')
const videoModels = ref([])
let videoDurationInitialized = false
const storyDraft = ref(defaultStoryDraft())
const shotImagesGenerating = ref(false)
const historyOpen = ref(false)
const projectHistory = ref([])
const historyLoading = ref(false)
const historyError = ref('')
const localDraftNotice = ref(null)
const draftDirty = ref(false)
const fileInput = ref(null)
const stepRefs = {}
let observer = null
let pollTimer = null
let draftDirtySuppressed = false
const aspectRatioOptions = [
  { value: '16:9', label: '16:9 横屏' },
  { value: '9:16', label: '9:16 竖屏' }
]
const contentModeOptions = [
  { value: 'short_film_image', label: '短电影图片' },
  { value: 'narration', label: '旁白成片' },
  { value: 'drama', label: '短剧分场' },
  { value: 'ad', label: '广告混剪' }
]
const generationModeOptions = [
  { value: 'image_series', label: 'Image series' },
  { value: 'storyboard', label: 'Storyboard' },
  { value: 'grid', label: 'Grid' },
  { value: 'reference_video', label: 'Reference video' }
]
const gridSizeOptions = [
  { value: 4, label: '4 grid' },
  { value: 6, label: '6 grid' },
  { value: 9, label: '9 grid' }
]
const fallbackVideoModels = [
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
    name: 'Sora 2',
    runtime_model: 'sora-2',
    aspect_ratios: ['16:9', '9:16'],
    durations: ['10', '15', '25'],
    default_duration: '10',
    resolution_options: [],
    supports_generate_audio: false
  },
  {
    name: 'Sora 2 Pro',
    runtime_model: 'sora-2-pro',
    aspect_ratios: ['16:9', '9:16'],
    durations: ['10', '15', '25'],
    default_duration: '10',
    resolution_options: [],
    supports_generate_audio: false
  },
  {
    name: 'Doubao Seedance 2.0',
    runtime_model: doubaoSeedance2Model,
    aspect_ratios: ['16:9', '9:16'],
    durations: ['4', '5', '6', '7', '8', '9', '10', '11', '12', '13', '14', '15', '-1'],
    default_duration: '10',
    resolution_options: ['720p', '1080p'],
    default_resolution: '720p',
    supports_generate_audio: true
  }
]

const { theme } = useUserTheme()
const studioThemeClass = computed(() => (theme.value === 'light' ? 'novel-studio-light' : 'novel-studio-dark'))
const sourceChars = computed(() => sourceText.value.trim().length)
const canCreate = computed(() => title.value.trim() && sourceText.value.trim() && sourceChars.value <= 50000 && !busyAction.value)
const availableVideoModels = computed(() => (videoModels.value.length ? videoModels.value : fallbackVideoModels))
const selectedVideoModel = computed(() => availableVideoModels.value.find((item) => item.runtime_model === videoModel.value) ?? availableVideoModels.value[0] ?? fallbackVideoModels[0])
const selectedVideoCapability = computed(() => ({
  aspectRatios: selectedVideoModel.value?.aspect_ratios?.length ? selectedVideoModel.value.aspect_ratios : ['16:9', '9:16'],
  durations: selectedVideoModel.value?.durations?.length ? selectedVideoModel.value.durations : fallbackVideoDurationsForModel(selectedVideoModel.value?.runtime_model),
  defaultDuration: selectedVideoModel.value?.default_duration || '',
  resolutionOptions: selectedVideoModel.value?.resolution_options?.length ? selectedVideoModel.value.resolution_options : [],
  defaultResolution: selectedVideoModel.value?.default_resolution || '',
  supportsGenerateAudio: selectedVideoModel.value?.supports_generate_audio === true
}))
const durationOptions = computed(() => selectedVideoCapability.value.durations.map((value) => ({
  value,
  label: value === '-1' ? '智能时长' : `${value} 秒`
})))
const videoModelOptions = computed(() => availableVideoModels.value.map((item) => ({
  value: item.runtime_model,
  label: videoModelOptionLabel(item)
})))
const shouldShowResolutionSelect = computed(() => selectedVideoCapability.value.resolutionOptions.length > 0)
const resolutionOptions = computed(() => selectedVideoCapability.value.resolutionOptions.map((value) => ({ value, label: value })))
const shouldShowGenerateAudioToggle = computed(() => selectedVideoCapability.value.supportsGenerateAudio)
const localDraftUserID = computed(() => me.value?.user_id ?? me.value?.id ?? 'anonymous')
const creatures = computed(() => project.value?.creatures ?? [])
const actors = computed(() => creatures.value)
const assets = computed(() => project.value?.assets ?? [])
const visibleAssets = computed(() => foldNovelVideoAssets(assets.value, collapsedAssetIds.value))
const episodes = computed(() => project.value?.episodes ?? [])
const jobs = computed(() => project.value?.jobs ?? [])
const assetImageJobs = computed(() => jobs.value.filter((job) => job.type === 'asset_image'))
const activeAssetImageJobs = computed(() => assetImageJobs.value.filter((job) => ['queued', 'running'].includes(job.status)))
const activeNovelVideoJobs = computed(() => jobs.value.filter((job) => ['asset_image', 'storyboard', 'shot_video'].includes(job.type) && ['queued', 'running'].includes(job.status)))
const activeCreatureGenerations = computed(() => creatures.value.filter(isActiveCreatureGeneration))
const assetGenerationSummary = computed(() => {
  const total = activeAssetImageJobs.value.length
  const done = 0
  const failed = 0
  const active = activeAssetImageJobs.value.length
  return { total, done, failed, active }
})
const hasActiveAssetImageJobs = computed(() => activeAssetImageJobs.value.length > 0)
const duplicateAssetGroups = computed(() => {
  const groups = new Map()
  const hasActorRef = assets.value.some((asset) => asset?.kind === 'actor_ref')
  const firstActorRef = assets.value.find((asset) => asset?.kind === 'actor_ref')
  assets.value.forEach((asset) => {
    let key = assetDedupeKey(asset, { hasActorRef })
    if (!key) return
    if (hasActorRef && asset.kind === 'character' && firstActorRef) {
      const actorKey = assetDedupeKey(firstActorRef, { hasActorRef })
      if (actorKey) key = actorKey
    }
    const current = groups.get(key) ?? []
    current.push(asset)
    groups.set(key, current)
  })
  return Array.from(groups.values()).filter((group) => group.length > 1)
})
const compositions = computed(() => project.value?.compositions ?? [])
const flatShots = computed(() => episodes.value.flatMap((episode) => (episode.shots ?? []).map((shot) => ({ ...shot, episode }))))
const renderQueueRows = computed(() => [
  ...jobs.value.map((job) => ({
    key: `job-${job.id}`,
    label: `#${job.id}`,
    type: job.type,
    status: job.status,
    progress: job.progress ?? 0
  })),
  ...flatShots.value.map((shot) => ({
    key: `shot-${shot.id}`,
    label: `${shot.episode?.number ?? '-'}-${shot.number ?? '-'}`,
    type: shot.title,
    status: shot.status,
    progress: progressOf(shot)
  }))
])
const renderQueueTotal = computed(() => renderQueueRows.value.length)
const renderQueuePageCount = computed(() => Math.max(1, Math.ceil(renderQueueTotal.value / RENDER_QUEUE_PAGE_SIZE)))
const renderQueueOffset = computed(() => (renderQueuePage.value - 1) * RENDER_QUEUE_PAGE_SIZE)
const paginatedRenderQueueRows = computed(() => renderQueueRows.value.slice(renderQueueOffset.value, renderQueueOffset.value + RENDER_QUEUE_PAGE_SIZE))
const shouldShowRenderQueuePagination = computed(() => renderQueueTotal.value > RENDER_QUEUE_PAGE_SIZE)
const renderQueueRangeLabel = computed(() => {
  if (!renderQueueTotal.value) return '第 1/1 页 · 0 / 0'
  const start = renderQueueOffset.value + 1
  const end = Math.min(renderQueueOffset.value + RENDER_QUEUE_PAGE_SIZE, renderQueueTotal.value)
  return `第 ${renderQueuePage.value}/${renderQueuePageCount.value} 页 · ${start}-${end} / ${renderQueueTotal.value}`
})
const selectedCreature = computed(() => creatures.value.find((item) => item.id === selectedCreatureId.value) ?? creatures.value[0] ?? null)
const selectedEpisode = computed(() => episodes.value.find((item) => item.id === selectedEpisodeId.value) ?? episodes.value[0] ?? null)
const selectedEpisodeShots = computed(() => selectedEpisode.value?.shots ?? [])
const episodeShotsPageCount = computed(() => Math.max(1, Math.ceil(selectedEpisodeShots.value.length / episodeShotsPageSize.value)))
const episodeShotsOffset = computed(() => (episodeShotsPage.value - 1) * episodeShotsPageSize.value)
const paginatedEpisodeShots = computed(() => selectedEpisodeShots.value.slice(episodeShotsOffset.value, episodeShotsOffset.value + episodeShotsPageSize.value))
const episodeShotsRangeLabel = computed(() => {
  const total = selectedEpisodeShots.value.length
  if (!total) return '0 / 0'
  const start = episodeShotsOffset.value + 1
  const end = Math.min(episodeShotsOffset.value + episodeShotsPageSize.value, total)
  return `${start}-${end} / ${total}`
})
const selectedShot = computed(() => flatShots.value.find((item) => item.id === selectedShotId.value) ?? flatShots.value[0] ?? null)
const selectedShotReferences = computed(() => {
  if (!selectedShot.value) return []
  const ids = selectedShot.value.creature_ids ?? []
  return creatures.value.filter((creature) => ids.includes(creature.id) && creature.work_preview_url)
})
const selectedAsset = computed(() => assets.value.find((item) => item.id === selectedAssetId.value) ?? null)
const selectedAssetJob = computed(() => selectedAsset.value ? jobForAsset(selectedAsset.value) : null)
const selectedAssetReferences = computed(() => {
  if (!selectedAsset.value?.id) return []
  const assetID = selectedAsset.value.id
  return flatShots.value.filter((shot) => {
    if (shot.reference_asset_id === assetID) return true
    if ((shot.reference_asset_ids ?? []).includes(assetID)) return true
    return (shot.asset_refs ?? []).some((ref) => ref.id === assetID && ref.type !== 'creature')
  })
})
const approvedShotCount = computed(() => flatShots.value.filter((item) => item.status === 'approved').length)
const imagePlanReadyShotCount = computed(() => flatShots.value.length)
const selectedShotImages = computed(() => {
  if (!selectedShot.value) return shotImages.value
  return shotImages.value.filter((item) => item.shot_id === selectedShot.value.id)
})
const shouldShowShotImagesGenerating = computed(() => shotImagesGenerating.value && !selectedShotImages.value.length)
const selectedImageCount = computed(() => shotImages.value.filter((item) => item.selected).length)
const shotImageRows = computed(() => flatShots.value.map(buildShotImageRow))
const expectedImageCandidateCount = computed(() => Math.min(80, Math.min(Math.max(imagePlanReadyShotCount.value, 1), 20) * Number(imageCandidatesPerShot.value || 4)))
const renderStats = computed(() => {
  const total = flatShots.value.length
  const done = flatShots.value.filter((item) => item.status === 'succeeded').length
  const failed = flatShots.value.filter((item) => item.status === 'failed').length
  const running = flatShots.value.filter((item) => ['queued', 'running'].includes(item.status)).length
  const requiredCredits = renderResult.value?.required_credits ?? flatShots.value.reduce((sum, item) => sum + (item.estimated_credits ?? 0), 0)
  return { total, done, failed, running, requiredCredits }
})
const currentRenderingShot = computed(() => flatShots.value.find((item) => ['running', 'queued'].includes(item.status)) ?? flatShots.value[0] ?? null)
const projectStatusText = computed(() => productionStatusText(project.value?.status))
const promptLineNumbers = computed(() => {
  const lines = `${selectedShot.value?.prompt ?? ''}`.split('\n').length
  return Array.from({ length: Math.max(lines, 5) }, (_, index) => index + 1)
})
const workflowSteps = computed(() => [
  { key: 'import', label: '导入', sectionTitle: '短电影导入', state: project.value?.id ? 'done' : 'current' },
  { key: 'bible', label: '故事圣经', sectionTitle: '故事圣经', state: project.value?.story_bible ? 'done' : project.value?.id ? 'current' : 'pending' },
  { key: 'creatures', label: '演员锁定', sectionTitle: '演员锁定', state: workflowReviewState(actors.value) },
  { key: 'assets', label: '资产板', sectionTitle: '资产板', state: workflowReviewState(assets.value) },
  { key: 'shots', label: '镜头图片', sectionTitle: '镜头图片计划', state: workflowReviewState(flatShots.value) },
  { key: 'render', label: '批量任务', sectionTitle: '批量图片任务', state: imageBatchResult.value?.queued ? 'running' : shotImages.value.length ? 'done' : flatShots.value.length ? 'current' : 'pending' },
  { key: 'export', label: '导出', sectionTitle: '图片包导出', state: selectedImageCount.value > 0 ? 'done' : project.value?.id ? 'current' : 'pending' }
])

watch(videoModel, () => {
  syncSupportedVideoSettings()
})

watch(videoModels, () => {
  syncSupportedVideoSettings()
})

watch([
  title,
  sourceText,
  stylePreset,
  contentMode,
  generationMode,
  gridSize,
  aspectRatio,
  duration,
  videoModel,
  resolution,
  generateAudio,
  storyDraft
], () => {
  if (!draftDirtySuppressed) {
    draftDirty.value = true
  }
}, { deep: true })

watch(renderQueuePageCount, () => {
  clampRenderQueuePage()
})

watch(episodeShotsPageCount, () => {
  episodeShotsPage.value = Math.min(episodeShotsPage.value, episodeShotsPageCount.value)
})

function defaultStoryDraft() {
  return {
    logline: '',
    world: '',
    conflict: '',
    visual_style: '',
    risk_highlight: ''
  }
}

function canonicalVideoModelValue(value) {
  return String(value || '').trim()
}

function normalizeVideoModelItems(items = []) {
  return (Array.isArray(items) ? items : [])
    .map((item) => ({
      ...item,
      name: item?.name || item?.runtime_model || item?.id || '',
      runtime_model: canonicalVideoModelValue(item?.runtime_model || item?.model || item?.id || ''),
      aspect_ratios: Array.isArray(item?.aspect_ratios) ? item.aspect_ratios.map(String) : [],
      durations: Array.isArray(item?.durations) ? item.durations.map((value) => String(value)) : [],
      default_duration: item?.default_duration ? String(item.default_duration) : '',
      resolution_options: Array.isArray(item?.resolution_options) ? item.resolution_options.map(String) : [],
      default_resolution: item?.default_resolution || '',
      supports_generate_audio: item?.supports_generate_audio === true,
      supports_reference_video: item?.supports_reference_video === true,
      supports_reference_audio: item?.supports_reference_audio === true
    }))
    .filter((item) => item.runtime_model)
}

function fallbackVideoDurationsForModel(value) {
  if (value === doubaoSeedance2Model) return fallbackVideoModels[3].durations
  if (value === grokImagineVideoModel) return fallbackVideoModels[0].durations
  return ['10', '15', '25']
}

function videoModelOptionLabel(item = {}) {
  const label = item.name || item.runtime_model || ''
  const badges = []
  if (`${item.runtime_model || ''} ${item.name || ''}`.toLowerCase().includes('seedance')) badges.push('Seedance')
  if (item.supports_reference_video) badges.push('视频参考')
  if (item.supports_reference_audio) badges.push('音频参考')
  return badges.length ? `${label} [${badges.join('] [')}]` : label
}

function syncSupportedVideoSettings() {
  if (availableVideoModels.value.length && !availableVideoModels.value.some((item) => item.runtime_model === videoModel.value)) {
    videoModel.value = availableVideoModels.value[0].runtime_model
    return
  }
  const durationValues = durationOptions.value.map((option) => option.value)
  if (!videoDurationInitialized && videoModels.value.length > 0 && durationValues.length) {
    duration.value = durationValues.includes(selectedVideoCapability.value.defaultDuration)
      ? selectedVideoCapability.value.defaultDuration
      : durationValues[0]
    videoDurationInitialized = true
  } else if (durationValues.length && !durationValues.includes(duration.value)) {
    duration.value = durationValues.includes(selectedVideoCapability.value.defaultDuration)
      ? selectedVideoCapability.value.defaultDuration
      : durationValues[0]
  }
  const resolutionValues = resolutionOptions.value.map((option) => option.value)
  if (!resolutionValues.length) {
    resolution.value = ''
  } else if (!resolutionValues.includes(resolution.value)) {
    resolution.value = selectedVideoCapability.value.defaultResolution || resolutionValues[0]
  }
  if (!selectedVideoCapability.value.supportsGenerateAudio) {
    generateAudio.value = false
  }
}

function novelVideoDraftKey(projectID = project.value?.id) {
  const suffix = projectID ? `project:${projectID}` : 'new'
  return `novel-video:draft:${localDraftUserID.value}:${suffix}`
}

function readJSONStorage(key) {
  if (typeof window === 'undefined' || !window.localStorage) return null
  try {
    const raw = window.localStorage.getItem(key)
    return raw ? JSON.parse(raw) : null
  } catch {
    return null
  }
}

function hasDraftContent(draft) {
  if (!draft) return false
  return Boolean(
    `${draft.title || ''}`.trim() ||
    `${draft.source_text || ''}`.trim() ||
    `${draft.style_preset || ''}`.trim() ||
    Object.values(draft.story_bible ?? {}).some((value) => `${value || ''}`.trim())
  )
}

function currentDraftPayload() {
  return {
    title: title.value,
    source_text: sourceText.value,
    style_preset: stylePreset.value,
    content_mode: contentMode.value,
    generation_mode: generationMode.value,
    grid_size: Number(gridSize.value || 4),
    video_settings: videoSettingsPayload(),
    story_bible: { ...storyDraft.value },
    updated_at: new Date().toISOString()
  }
}

function persistNovelVideoDraft(projectID = project.value?.id) {
  const draft = currentDraftPayload()
  if (!hasDraftContent(draft)) return
  window.localStorage?.setItem(novelVideoDraftKey(projectID), JSON.stringify(draft))
}

function persistNovelVideoDraftIfNeeded(projectID = project.value?.id) {
  if (!draftDirty.value) return
  persistNovelVideoDraft(projectID)
}

function loadNovelVideoDraft(projectID = project.value?.id) {
  return readJSONStorage(novelVideoDraftKey(projectID))
}

function discardNovelVideoDraft(projectID = project.value?.id) {
  window.localStorage?.removeItem(novelVideoDraftKey(projectID))
  if ((projectID || null) === (localDraftNotice.value?.projectID || null)) {
    localDraftNotice.value = null
  }
}

function applyNovelVideoDraft(draft) {
  if (!draft) return
  title.value = draft.title ?? title.value
  sourceText.value = draft.source_text ?? sourceText.value
  stylePreset.value = draft.style_preset ?? stylePreset.value
  contentMode.value = draft.content_mode ?? contentMode.value
  generationMode.value = draft.generation_mode ?? generationMode.value
  gridSize.value = draft.grid_size ?? gridSize.value
  const videoSettings = draft.video_settings ?? {}
  aspectRatio.value = videoSettings.aspect_ratio ?? aspectRatio.value
  duration.value = String(videoSettings.duration ?? duration.value)
  videoModel.value = videoSettings.model ?? videoModel.value
  resolution.value = videoSettings.resolution ?? resolution.value
  generateAudio.value = Boolean(videoSettings.generate_audio ?? generateAudio.value)
  storyDraft.value = {
    ...defaultStoryDraft(),
    ...(draft.story_bible ?? {})
  }
  syncSupportedVideoSettings()
  draftDirty.value = true
}

function restoreLocalDraftNotice() {
  const notice = localDraftNotice.value
  if (!notice?.draft) return
  applyNovelVideoDraft(notice.draft)
  message.value = '已恢复本机草稿'
  localDraftNotice.value = null
}

function discardLocalDraftNotice() {
  discardNovelVideoDraft(localDraftNotice.value?.projectID)
  message.value = '已丢弃本机草稿'
}

function updateProjectURL(projectID) {
  if (typeof window === 'undefined') return
  const url = new URL(window.location.href)
  if (projectID) {
    url.searchParams.set('project_id', projectID)
    url.searchParams.delete('id')
  } else {
    url.searchParams.delete('project_id')
    url.searchParams.delete('id')
  }
  window.history.replaceState({}, '', `${url.pathname}${url.search}${url.hash}`)
}

function normalizeHistoryItems(payload) {
  const items = Array.isArray(payload) ? payload : payload?.items ?? payload?.projects ?? []
  return items
    .filter((item) => item?.id)
    .slice(0, 50)
}

function resetProjectSessionState() {
  stopProjectPolling()
  closeEpisodeShots()
  selectedCreatureId.value = null
  selectedEpisodeId.value = null
  selectedShotId.value = null
  selectedAssetId.value = null
  inspectorMode.value = 'project'
  activeStep.value = 'import'
  renderResult.value = null
  costEstimate.value = null
  gridItems.value = []
  shotImages.value = []
  imageBatchResult.value = null
  shotImagesGenerating.value = false
  renderQueuePage.value = 1
  exportOutput.value = ''
  localDraftNotice.value = null
}

function clampRenderQueuePage() {
  if (renderQueuePage.value > renderQueuePageCount.value) {
    renderQueuePage.value = renderQueuePageCount.value
  }
  if (renderQueuePage.value < 1) {
    renderQueuePage.value = 1
  }
}

function goToRenderQueuePage(page) {
  renderQueuePage.value = Math.min(Math.max(1, page), renderQueuePageCount.value)
}

function upsertProjectHistory(item) {
  if (!item?.id) return
  projectHistory.value = [
    item,
    ...projectHistory.value.filter((entry) => entry.id !== item.id)
  ].slice(0, 50)
}

async function loadProjectHistory() {
  historyLoading.value = true
  historyError.value = ''
  try {
    const payload = await api.listNovelVideoProjects()
    projectHistory.value = normalizeHistoryItems(payload)
  } catch (error) {
    historyError.value = error.message
  } finally {
    historyLoading.value = false
  }
}

async function openProjectHistory() {
  historyOpen.value = true
  await loadProjectHistory()
}

function projectHasLocalDraft(projectID) {
  return hasDraftContent(loadNovelVideoDraft(projectID))
}

function newLocalDraft() {
  const draft = loadNovelVideoDraft(null)
  return hasDraftContent(draft) ? draft : null
}

function applyLoadedProject(payload) {
  resetProjectSessionState()
  setProject(payload)
  updateProjectURL(payload?.id)
  const draft = loadNovelVideoDraft(payload?.id)
  if (hasDraftContent(draft)) {
    localDraftNotice.value = {
      projectID: payload.id,
      draft
    }
  }
  if (activeNovelVideoJobs.value.length) startNovelVideoJobPolling()
  if (activeCreatureGenerations.value.length) startProjectPolling()
}

async function selectHistoryProject(projectSummary) {
  if (!projectSummary?.id) return
  persistNovelVideoDraftIfNeeded()
  historyError.value = ''
  historyLoading.value = true
  try {
    const payload = await api.getNovelVideoProject(projectSummary.id)
    applyLoadedProject(payload)
    upsertProjectHistory(payload)
    historyOpen.value = false
  } catch (error) {
    historyError.value = error.message
  } finally {
    historyLoading.value = false
  }
}

function selectNewDraft() {
  const draft = newLocalDraft()
  if (!draft) return
  persistNovelVideoDraftIfNeeded()
  resetProjectSessionState()
  project.value = null
  applyNovelVideoDraft(draft)
  updateProjectURL(null)
  historyOpen.value = false
  message.value = '已载入本机草稿'
}

function formatHistoryDate(value) {
  if (!value) return '未记录'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  })
}

function videoSettingsPayload() {
  const payload = {
    model: videoModel.value,
    aspect_ratio: aspectRatio.value,
    duration: duration.value,
    generate_audio: Boolean(generateAudio.value && selectedVideoCapability.value.supportsGenerateAudio)
  }
  if (resolution.value) payload.resolution = resolution.value
  return payload
}

async function loadVideoModels() {
  const payload = await api.listVideoModels().catch(() => ({ items: [] }))
  videoModels.value = normalizeVideoModelItems(payload.items)
  syncSupportedVideoSettings()
}

function setProject(payload) {
  draftDirtySuppressed = true
  payload = normalizeNovelVideoProject(payload)
  project.value = payload
  collapsedAssetIds.value = new Set(payload?.collapsed_ids ?? [])
  if (!payload) {
    draftDirty.value = false
    nextTick(() => {
      draftDirtySuppressed = false
    })
    return
  }
  const videoSettings = payload.video_settings ?? {}
  title.value = payload.title ?? title.value
  sourceText.value = payload.source_text ?? sourceText.value
  stylePreset.value = payload.style_preset ?? stylePreset.value
  contentMode.value = payload.content_mode ?? contentMode.value
  generationMode.value = payload.generation_mode ?? generationMode.value
  gridSize.value = payload.grid_size ?? gridSize.value
  aspectRatio.value = videoSettings.aspect_ratio ?? payload.aspect_ratio ?? aspectRatio.value
  duration.value = String(videoSettings.duration ?? payload.duration ?? duration.value)
  videoModel.value = videoSettings.model ?? payload.video_model ?? videoModel.value
  resolution.value = videoSettings.resolution ?? resolution.value
  generateAudio.value = Boolean(videoSettings.generate_audio ?? generateAudio.value)
  storyDraft.value = {
    ...defaultStoryDraft(),
    ...(payload.story_bible ?? {}),
    risk_highlight: payload.content_risk_summary ?? payload.story_bible?.risk_highlight ?? ''
  }
  if (!selectedCreatureId.value && payload.creatures?.length) selectedCreatureId.value = payload.creatures[0].id
  if (!selectedEpisodeId.value && payload.episodes?.length) selectedEpisodeId.value = payload.episodes[0].id
  const firstShot = payload.episodes?.flatMap((episode) => episode.shots ?? [])[0]
  if (!selectedShotId.value && firstShot) selectedShotId.value = firstShot.id
  if (Array.isArray(payload.images)) shotImages.value = payload.images.map(normalizeNovelVideoShotImage)
  syncSupportedVideoSettings()
  draftDirty.value = false
  nextTick(() => {
    draftDirtySuppressed = false
  })
}

function normalizeNovelVideoProject(payload) {
  if (!payload) return payload
  return {
    ...payload,
    assets: payload.assets ?? [],
    jobs: payload.jobs ?? [],
    episodes: (payload.episodes ?? []).map((episode) => ({
      ...episode,
      shots: (episode.shots ?? []).map((shot) => ({
        ...shot,
        video_prompt: shot.video_prompt ?? shot.prompt ?? '',
        voiceover_text: shot.voiceover_text ?? shot.subtitle_text ?? '',
        duration_seconds: shot.duration_seconds ?? Number(duration.value || 0),
        asset_refs: shot.asset_refs ?? [],
        asset_refs_json: JSON.stringify(shot.asset_refs ?? [], null, 2)
      }))
    })),
    images: (payload.images ?? []).map(normalizeNovelVideoShotImage)
  }
}

function mergeNovelVideoJobs(incoming = []) {
  if (!project.value || !Array.isArray(incoming) || !incoming.length) return
  const byID = new Map((project.value.jobs ?? []).map((job) => [job.id, job]))
  incoming.forEach((job) => {
    if (job?.id) byID.set(job.id, { ...(byID.get(job.id) ?? {}), ...job })
  })
  project.value.jobs = Array.from(byID.values()).sort((a, b) => (b.id ?? 0) - (a.id ?? 0))
}

function mergeNovelVideoAssets(incoming = [], replace = false) {
  if (!project.value || !Array.isArray(incoming)) return
  if (replace) {
    project.value.assets = incoming
    return
  }
  const byID = new Map((project.value.assets ?? []).map((asset) => [asset.id, asset]))
  incoming.forEach((asset) => {
    if (asset?.id) byID.set(asset.id, { ...(byID.get(asset.id) ?? {}), ...asset })
  })
  project.value.assets = Array.from(byID.values())
}

function normalizeNovelVideoShotImage(image) {
  if (!image) return image
  const generationStatus = image.generation_status || inferShotImageGenerationStatus(image)
  return {
    ...image,
    actor_ids: image.actor_ids ?? [],
    reference_asset_ids: image.reference_asset_ids ?? [],
    review_status: image.review_status || 'needs_review',
    generation_status: generationStatus,
    generation_stage: image.generation_stage || generationStatus,
    generation_progress: typeof image.generation_progress === 'number' ? image.generation_progress : shotImageGenerationProgress(generationStatus, image.generation_stage),
    version: image.version || 1
  }
}

function inferShotImageGenerationStatus(image) {
  if (image?.error_message || image?.error_code) return 'failed'
  if (image?.preview_url || image?.download_url || image?.selected || image?.review_status === 'approved') return 'succeeded'
  return ''
}

function shotImageGenerationProgress(status, stage) {
  if (status === 'queued') return 5
  if (status === 'running') return stage === 'persisting_result' ? 85 : 35
  if (['succeeded', 'failed'].includes(status)) return 100
  return 0
}

function buildShotImageRow(shot) {
  const images = shotImages.value
    .filter((item) => item.shot_id === shot.id)
    .slice()
    .sort((a, b) => (a.version ?? 0) - (b.version ?? 0) || (a.id ?? 0) - (b.id ?? 0))
  const selectedImage = images.find((item) => item.selected) ?? null
  const latestImage = images[images.length - 1] ?? null
  const activeImage = images.find((item) => ['queued', 'running'].includes(item.generation_status)) ?? null
  const failedImage = [...images].reverse().find((item) => item.generation_status === 'failed') ?? null
  const displayImage = selectedImage ?? latestImage
  const thumbnailUrl = selectedImage?.preview_url || selectedImage?.download_url || latestImage?.preview_url || latestImage?.download_url || ''
  const stateImage = activeImage ?? failedImage ?? selectedImage ?? latestImage
  const generationStatus = activeImage?.generation_status || failedImage?.generation_status || (selectedImage ? 'succeeded' : latestImage?.generation_status || '')
  const progress = activeImage ? activeImage.generation_progress : failedImage ? 100 : selectedImage || latestImage ? shotImageGenerationProgress(generationStatus || 'succeeded', stateImage?.generation_stage) : 0

  return {
    shot,
    images,
    selectedImage,
    latestImage,
    displayImage,
    thumbnailUrl,
    candidateCount: images.length,
    selected: Boolean(selectedImage),
    reviewStatus: selectedImage?.review_status || latestImage?.review_status || 'needs_review',
    generationStatus,
    generationStage: stateImage?.generation_stage || '',
    progress,
    errorMessage: failedImage?.error_message || failedImage?.error_code || '',
    canGenerate: Boolean(project.value?.id && shot?.id && !busyAction.value),
    canRetry: Boolean(failedImage && !busyAction.value),
    canApprove: Boolean(latestImage && !selectedImage),
    canOpenCandidates: images.length > 0
  }
}

function shotImageRowStatusText(row) {
  if (row.generationStatus === 'queued') return '排队中'
  if (row.generationStatus === 'running') return row.generationStage === 'persisting_result' ? '保存结果' : '生成中'
  if (row.generationStatus === 'failed') return '失败'
  if (row.selected) return '已定稿'
  if (row.candidateCount > 0) return '已出图'
  return '待生成'
}

function shotImageRowFor(shot) {
  return shotImageRows.value.find((row) => row.shot.id === shot.id) ?? buildShotImageRow(shot)
}

function assetDedupeKey(asset, options = {}) {
  const kind = `${asset?.kind ?? ''}`.trim()
  if (!kind) return ''
  const actorID = Number(asset?.metadata?.actor_id || 0)
  if (kind === 'actor_ref' && actorID > 0) return `actor_ref\u0000actor:${actorID}`
  if (kind === 'character' && options.hasActorRef) return 'character\u0000covered-by-actor-ref'
  const intent = normalizedAssetIntent(asset)
  return intent ? `${kind}\u0000${intent}` : ''
}

function normalizedAssetIntent(asset) {
  const raw = normalizeAssetIntentText(asset?.name) || normalizeAssetIntentText(asset?.description) || normalizeAssetIntentText(asset?.prompt)
  if (!raw) return ''
  switch (`${asset?.kind ?? ''}`.trim()) {
    case 'scene':
      return stripAssetIntentTerms(raw, ['主场景', '核心场景', '场景参考', '场景', 'location', 'scene'])
    case 'prop':
      return stripAssetIntentTerms(raw, ['关键道具', '核心物件', '道具参考', '道具', 'prop', 'object'])
    case 'style':
      return stripAssetIntentTerms(raw, ['视觉风格', '风格参考', '统一风格', '风格', 'style'])
    case 'clue':
      return stripAssetIntentTerms(raw, ['悬念线索', '视觉线索', '线索参考', '线索', 'clue'])
    case 'character':
      return stripAssetIntentTerms(raw, ['主角视觉锚点', '角色锚点', '视觉锚点', '角色', 'character'])
    default:
      return raw
  }
}

function normalizeAssetIntentText(value) {
  return `${value ?? ''}`
    .trim()
    .toLowerCase()
    .replace(/[\s\t\r\n\-_:：,，.。、()[\]（）【】·•]/g, '')
}

function stripAssetIntentTerms(value, terms) {
  let result = value
  terms.forEach((term) => {
    result = result.replaceAll(normalizeAssetIntentText(term), '')
  })
  return result || value
}

function foldNovelVideoAssets(items = [], collapsedIDs = new Set()) {
  const hasActorRef = items.some((asset) => asset?.kind === 'actor_ref')
  const hiddenIDs = new Set(collapsedIDs)
  const groups = new Map()
  items.forEach((asset) => {
    if (!asset?.id || hiddenIDs.has(asset.id)) return
    if (asset.kind === 'character' && hasActorRef) {
      hiddenIDs.add(asset.id)
      return
    }
    const key = assetDedupeKey(asset, { hasActorRef })
    if (!key) {
      groups.set(`id:${asset.id}`, asset)
      return
    }
    const current = groups.get(key)
    if (!current || betterAssetCard(asset, current)) {
      if (current?.id) hiddenIDs.add(current.id)
      groups.set(key, asset)
    } else {
      hiddenIDs.add(asset.id)
    }
  })
  const visibleIDs = new Set(Array.from(groups.values()).map((asset) => asset.id))
  return items.filter((asset) => visibleIDs.has(asset?.id) && !hiddenIDs.has(asset.id))
}

function betterAssetCard(candidate, current) {
  const candidateScore = assetCardScore(candidate)
  const currentScore = assetCardScore(current)
  if (candidateScore !== currentScore) return candidateScore > currentScore
  return Number(candidate?.id || 0) < Number(current?.id || 0)
}

function assetCardScore(asset) {
  if (asset?.review_status === 'approved') return 3
  if (asset?.asset_url || asset?.reference_url || asset?.work_id || asset?.generation_record_id || jobForAsset(asset)) return 2
  return 1
}

function jobForAsset(asset) {
  if (!asset?.id) return null
  return assetImageJobs.value.find((job) => job.asset_id === asset.id) ?? null
}

function activeJobForAsset(asset) {
  if (!asset?.id) return null
  return activeAssetImageJobs.value.find((job) => job.asset_id === asset.id) ?? null
}

function assetHasQueuedJob(asset) {
  return activeJobForAsset(asset)?.status === 'queued'
}

function assetHasRunningJob(asset) {
  return activeJobForAsset(asset)?.status === 'running'
}

function assetDeleteDisabled(asset) {
  return Boolean(assetHasRunningJob(asset) || busyAction.value === `asset-delete-${asset?.id}`)
}

function assetDeleteTitle(asset) {
  if (assetHasRunningJob(asset)) return '生成中的资产不能删除'
  if (assetHasQueuedJob(asset)) return '解除排队并删除资产'
  return '删除资产'
}

function assetGenerationState(asset) {
  const job = jobForAsset(asset)
  if (job) {
    if (job.status === 'succeeded') return null
    return {
      status: job.status,
      label: statusText(job.status),
      error: job.error_message || asset.error_message || '',
      retryable: job.status === 'failed'
    }
  }
  if (asset?.error_message) {
    return { status: 'failed', label: '失败', error: asset.error_message, retryable: true }
  }
  return null
}

function creatureGenerationState(creature) {
  if (!creature?.id) return null
  if (busyAction.value === `creature-image-${creature.id}`) {
    return { status: 'running', label: '生成中', error: '' }
  }
  if (isActiveCreatureGeneration(creature)) {
    return { status: creature.generation_status, label: creatureGenerationLabel(creature.generation_status), error: '' }
  }
  if (creature.latest_error || creature.error_message) {
    return { status: 'failed', label: '失败', error: creature.latest_error || creature.error_message }
  }
  return null
}

function isActiveCreatureGeneration(creature) {
  return ['queued', 'running'].includes(creature?.generation_status)
}

function projectHasActiveCreatureGeneration(payload = project.value) {
  return (payload?.creatures ?? []).some(isActiveCreatureGeneration)
}

function creatureGenerationLabel(status) {
  if (status === 'running') return '生成时间较长，请稍候'
  return statusText(status)
}

async function runAction(name, task) {
  busyAction.value = name
  errorMessage.value = ''
  message.value = ''
  try {
    return await task()
  } catch (error) {
    errorMessage.value = error.message
    return null
  } finally {
    busyAction.value = ''
  }
}

async function createAndAnalyzeProject() {
  if (!project.value?.id && !canCreate.value) return
  const payload = await runAction('create', async () => {
    let current = project.value
    if (!current?.id) {
      current = await api.createNovelVideoProject(projectPayload())
      setProject(current)
      updateProjectURL(current.id)
      discardNovelVideoDraft(null)
      upsertProjectHistory(current)
    } else {
      current = await api.updateNovelVideoProject(current.id, projectPatchPayload())
      setProject(current)
      upsertProjectHistory(current)
    }
    return api.analyzeNovelVideoProject(current.id)
  })
  if (payload) {
    setProject(payload)
    updateProjectURL(payload.id)
    upsertProjectHistory(payload)
    activeStep.value = 'bible'
    message.value = '故事圣经和生物卡已生成'
  }
}

async function analyzeProject() {
  if (!project.value?.id) return
  const payload = await runAction('analyze', () => api.analyzeNovelVideoProject(project.value.id))
  if (payload) {
    setProject(payload)
    activeStep.value = 'bible'
    message.value = '故事圣经和生物卡已生成'
  }
}

async function planImageSeries() {
  if (!project.value?.id) return
  const payload = await runAction('image-plan', () => api.generateNovelVideoImagePlan(project.value.id, {
    shot_count: Number(imagePlanShotCount.value || 20),
    candidates_per_shot: Number(imageCandidatesPerShot.value || 4),
    lock_level: actorLockLevel.value
  }))
  if (payload) {
    setProject(payload)
    activeStep.value = 'shots'
    message.value = '图片镜头计划已生成'
  }
}

async function saveProjectSettings() {
  if (!project.value?.id) return
  const payload = await runAction('project-save', () => api.updateNovelVideoProject(project.value.id, projectPatchPayload()))
  if (payload) {
    setProject(payload)
    message.value = '项目设置已保存'
  }
}

async function saveStoryBible() {
  if (!project.value?.id) return
  const payload = await runAction('story-save', () => api.updateNovelVideoProject(project.value.id, {
    story_bible: {
      logline: storyDraft.value.logline,
      world: storyDraft.value.world,
      conflict: storyDraft.value.conflict,
      visual_style: storyDraft.value.visual_style,
      risk_highlight: storyDraft.value.risk_highlight
    },
    content_risk_summary: storyDraft.value.risk_highlight
  }))
  if (payload) {
    setProject(payload)
    message.value = '故事圣经已保存'
  }
}

async function approveCreature(creature) {
  if (!project.value?.id || !creature?.id) return
  const payload = await runAction(`creature-${creature.id}`, () => api.updateNovelVideoCreature(project.value.id, creature.id, {
    review_status: 'approved'
  }))
  if (payload) replaceCreature(payload)
}

async function approveActor(actor) {
  if (!project.value?.id || !actor?.id) return
  const payload = await runAction(`actor-${actor.id}`, () => api.updateNovelVideoActor(project.value.id, actor.id, {
    review_status: 'approved',
    lock_level: actor.lock_level || actorLockLevel.value,
    visual_consistency_prompt: actor.visual_consistency_prompt,
    negative_identity_prompt: actor.negative_identity_prompt || '避免五官、发型、年龄、服装主轮廓漂移',
    reference_asset_ids: actor.reference_asset_ids ?? [],
    canonical_asset_id: actor.canonical_asset_id || null,
    approved_version: actor.approved_version || 1
  }))
  if (payload) {
    replaceCreature(payload)
    message.value = '演员锁定信息已批准'
  }
}

async function generateActorLockSheet(actor) {
  if (!project.value?.id || !actor?.id) return
  const payload = await runAction(`actor-lock-sheet-${actor.id}`, () => api.generateNovelVideoActorLockSheet(project.value.id, actor.id))
  const item = payload?.item ?? payload?.asset
  if (item) {
    project.value.assets = [item, ...(project.value.assets ?? []).filter((asset) => asset.id !== item.id)]
    activeStep.value = 'creatures'
    message.value = '演员定妆图任务已创建'
  }
}

async function generateCreatureImage(creature) {
  if (!project.value?.id || !creature?.id) return
  const payload = await runAction(`creature-image-${creature.id}`, () => api.generateNovelVideoCreatureImage(project.value.id, creature.id))
  const item = payload?.item ?? payload?.asset ?? payload
  if (item) {
    replaceCreature(item)
    if (isActiveCreatureGeneration(item)) startProjectPolling()
  }
}

async function planEpisodes() {
  if (!project.value?.id) return
  const payload = await runAction('plan', () => api.planNovelVideoEpisodes(project.value.id))
  if (payload) {
    setProject(payload)
    activeStep.value = 'shots'
    message.value = '分集故事线和镜头表已生成'
  }
}

async function generateAssets() {
  if (!project.value?.id || hasActiveAssetImageJobs.value) return
  const payload = await runAction('assets-generate', () => api.generateNovelVideoAssets(project.value.id, {
    kinds: ['character', 'scene', 'prop', 'clue', 'style']
  }))
  if (payload?.items) {
    mergeNovelVideoAssets(payload.items, true)
    if (payload.jobs?.length) {
      mergeNovelVideoJobs(payload.jobs)
      startNovelVideoJobPolling()
    }
    activeStep.value = 'assets'
    message.value = '资产草案已生成，请审核后再进入分镜和渲染'
  }
}

async function dedupeAssets() {
  if (!project.value?.id) return
  const payload = await runAction('assets-dedupe', () => api.dedupeNovelVideoAssets(project.value.id))
  if (payload?.items) {
    collapsedAssetIds.value = new Set(payload.collapsed_ids ?? [])
    mergeNovelVideoAssets(payload.items, true)
    message.value = payload.removed > 0
      ? `已清理 ${payload.removed} 个重复资产草案`
      : (payload.collapsed_ids?.length ? `已折叠 ${payload.collapsed_ids.length} 个重复资产` : '没有可安全清理的重复资产')
  }
}

function replaceAsset(payload) {
  if (!project.value || !payload?.id) return
  const index = (project.value.assets ?? []).findIndex((asset) => asset.id === payload.id)
  if (index >= 0) {
    project.value.assets[index] = payload
  } else {
    project.value.assets = [payload, ...(project.value.assets ?? [])]
  }
  selectedAssetId.value = payload.id
  inspectorMode.value = 'asset'
}

function refreshAssetsAfterDelete(deletedAsset, payload) {
  if (!project.value || !deletedAsset?.id) return
  const nextAssets = Array.isArray(payload?.items)
    ? payload.items
    : (project.value.assets ?? []).filter((asset) => asset.id !== deletedAsset.id)
  project.value.assets = nextAssets
  collapsedAssetIds.value = new Set([...collapsedAssetIds.value].filter((id) => id !== deletedAsset.id))
  const preservedJobs = (project.value.jobs ?? []).filter((job) => job.type !== 'asset_image')
  const refreshedJobs = Array.isArray(payload?.jobs) ? payload.jobs : []
  project.value.jobs = [...preservedJobs, ...refreshedJobs].sort((a, b) => (b.id ?? 0) - (a.id ?? 0))
  if (selectedAssetId.value === deletedAsset.id) {
    const nextSameKind = nextAssets.find((asset) => asset.kind === deletedAsset.kind)
    selectedAssetId.value = nextSameKind?.id ?? null
    inspectorMode.value = nextSameKind ? 'asset' : 'project'
  }
}

async function deleteAsset(asset) {
  if (!project.value?.id || !asset?.id || assetDeleteDisabled(asset)) return
  const prompt = assetHasQueuedJob(asset)
    ? `资产「${asset.name}」仍在排队中，删除会同时取消排队任务。确定解除排队并删除资产？`
    : asset.review_status === 'approved'
    ? `资产「${asset.name}」已审核通过，删除后会从资产板移除。确定删除？`
    : `确定删除资产「${asset.name}」？`
  if (!window.confirm(prompt)) return
  const payload = await runAction(`asset-delete-${asset.id}`, () => api.deleteNovelVideoAsset(project.value.id, asset.id))
  if (payload) {
    refreshAssetsAfterDelete(asset, payload)
    message.value = '资产已删除'
  }
}

async function updateAssetReview(asset, reviewStatus) {
  if (!project.value?.id || !asset?.id) return
  const payload = await runAction(`asset-review-${asset.id}`, () => api.updateNovelVideoAsset(project.value.id, asset.id, {
    review_status: reviewStatus
  }))
  if (payload) replaceAsset(payload)
}

async function retryAssetImage(asset) {
  if (!project.value?.id || !asset?.id) return
  const payload = await runAction(`asset-retry-${asset.id}`, () => api.generateNovelVideoAssets(project.value.id, {
    asset_id: asset.id
  }))
  if (payload?.items) mergeNovelVideoAssets(payload.items)
  if (payload?.jobs?.length) {
    mergeNovelVideoJobs(payload.jobs)
    startNovelVideoJobPolling()
  }
}

async function saveShot(shot = selectedShot.value) {
  if (!project.value?.id || !shot?.id) return null
  const generationSettings = {
    ...(shot.generation_settings ?? {}),
    reference_asset_ids: shot.reference_asset_ids ?? (shot.reference_asset_id ? [shot.reference_asset_id] : []),
    reference_video_asset_ids: shot.reference_video_asset_ids ?? [],
    reference_audio_asset_ids: shot.reference_audio_asset_ids ?? [],
    generate_audio: Boolean(shot.generate_audio ?? generateAudio.value)
  }
  const payload = await runAction(`shot-save-${shot.id}`, () => api.updateNovelVideoShot(project.value.id, shot.id, {
    title: shot.title,
    prompt: shot.prompt,
    script_unit_type: shot.script_unit_type,
    source_excerpt: shot.source_excerpt,
    duration_seconds: Number(shot.duration_seconds || duration.value || 0),
    image_prompt: shot.image_prompt,
    video_prompt: shot.video_prompt,
    voiceover_text: shot.voiceover_text,
    asset_refs: parseShotAssetRefs(shot),
    asset_refs_set: true,
    status: shot.status,
    reference_asset_id: shot.reference_asset_id,
    reference_asset_ids: generationSettings.reference_asset_ids,
    reference_video_asset_ids: generationSettings.reference_video_asset_ids,
    reference_audio_asset_ids: generationSettings.reference_audio_asset_ids,
    generate_audio: generationSettings.generate_audio,
    generation_settings: generationSettings,
    creature_ids: shot.creature_ids ?? [],
    creature_ids_set: true
  }))
  if (payload) replaceShot(payload)
  return payload
}

function parseShotAssetRefs(shot) {
  const raw = typeof shot.asset_refs_json === 'string' ? shot.asset_refs_json.trim() : ''
  if (!raw) return shot.asset_refs ?? []
  try {
    const parsed = JSON.parse(raw)
    return Array.isArray(parsed) ? parsed : []
  } catch {
    return shot.asset_refs ?? []
  }
}

async function approveShot(shot) {
  if (!project.value?.id || !shot?.id) return
  const payload = await runAction(`shot-${shot.id}`, () => api.updateNovelVideoShot(project.value.id, shot.id, {
    status: 'approved'
  }))
  if (payload) replaceShot(payload)
}

async function renderShots() {
  if (!project.value?.id) return
  const payload = await runAction('render', async () => {
    const preflight = await api.renderNovelVideoPreflight(project.value.id)
    renderResult.value = preflight
    if ((preflight.blocked ?? 0) > 0) {
      throw new Error(`有 ${preflight.blocked} 个镜头未通过预检，请先处理后再渲染`)
    }
    if (preflight.enough === false) {
      throw new Error('点数不足，请先充值后再渲染')
    }
    return api.queueNovelVideoRender(project.value.id)
  })
  if (payload) {
    renderResult.value = payload
    if (payload.jobs?.length) {
      project.value.jobs = payload.jobs
    }
    renderQueuePage.value = 1
    message.value = `队列 ${payload.queued ?? 0} 个镜头，跳过 ${payload.skipped ?? 0} 个`
    startProjectPolling()
  }
}

async function preflightRenderShots() {
  if (!project.value?.id) return
  const payload = await runAction('render-preflight', () => api.renderNovelVideoPreflight(project.value.id))
  if (payload) {
    renderResult.value = payload
    message.value = `可渲染 ${payload.renderable ?? 0} 个镜头，阻塞 ${payload.blocked ?? 0} 个`
  }
}

async function generateStoryboard(shot = selectedShot.value) {
  if (!project.value?.id || !shot?.id) return
  const payload = await runAction(`storyboard-${shot.id}`, () => api.generateNovelVideoStoryboard(project.value.id, shot.id))
  if (payload?.shot) replaceShot(payload.shot)
  if (payload?.job) {
    mergeNovelVideoJobs([payload.job])
    message.value = `storyboard 任务已入队：${payload.job.id}`
    startNovelVideoJobPolling()
  }
}

async function generateGrids() {
  if (!project.value?.id) return
  const payload = await runAction('grids-generate', () => api.generateNovelVideoGrids(project.value.id, {
    grid_size: Number(gridSize.value || 4)
  }))
  if (payload?.items) {
    gridItems.value = payload.items
    generationMode.value = 'grid'
    message.value = `Grid ${gridSize.value} 已生成 ${payload.items.length} 组`
  }
}

async function generateShotImages(options = {}) {
  return generateShotImagesV2(options)

  if (!project.value?.id) return
  const approvedShots = flatShots.value.filter((shot) => shot.status === 'approved')
  const fallbackShots = selectedShot.value ? [selectedShot.value] : []
  const shotIDs = (approvedShots.length ? approvedShots : fallbackShots).slice(0, 20).map((shot) => shot.id)
  if (!shotIDs.length) {
    errorMessage.value = '请先生成并批准至少一个镜头'
    return
  }
  const payload = await runAction('shot-images-generate', () => api.generateNovelVideoShotImages(project.value.id, {
    shot_ids: shotIDs,
    candidates_per_shot: Number(imageCandidatesPerShot.value || 4),
    mode: imageGenerationMode.value,
    lock_level: actorLockLevel.value
  }))
  if (payload) {
    imageBatchResult.value = payload
    if (payload.items?.length) {
      mergeShotImages(payload.items)
      shotImagesGenerating.value = payload.items.some((item) => ['queued', 'running'].includes(normalizeNovelVideoShotImage(item).generation_status))
      if (shotImagesGenerating.value) startNovelVideoJobPolling()
    } else if ((payload.queued ?? 0) > 0) {
      shotImagesGenerating.value = true
      startNovelVideoJobPolling()
    }
    activeStep.value = 'shots'
    message.value = `已创建 ${payload.queued ?? 0} 张候选图`
  }
}

async function loadShotImages(options = {}) {
  return loadShotImagesV2(options)

  if (!project.value?.id) return
  const task = () => api.listNovelVideoShotImages(project.value.id, selectedShot.value?.id ? {
    shot_id: selectedShot.value.id
  } : {})
  const payload = options.silent ? await task().catch((error) => {
    errorMessage.value = error.message
    return null
  }) : await runAction('shot-images-list', task)
  if (payload?.items) {
    const incoming = payload.items.map(normalizeNovelVideoShotImage)
    mergeShotImages(incoming, selectedShot.value?.id)
    message.value = `已刷新 ${incoming.length} 张候选图`
    if (incoming.length > 0 && !incoming.some((item) => ['queued', 'running'].includes(item.generation_status))) shotImagesGenerating.value = false
    return incoming.length
  }
  return 0
}

async function generateShotImagesV2(options = {}) {
  if (!project.value?.id) return
  const approvedShots = flatShots.value.filter((shot) => shot.status === 'approved')
  const fallbackShots = selectedShot.value ? [selectedShot.value] : []
  const shotIDs = options.shotIDs?.length ? options.shotIDs : (approvedShots.length ? approvedShots : fallbackShots).slice(0, 20).map((shot) => shot.id)
  if (!shotIDs.length) {
    errorMessage.value = '请先生成并批准至少一个镜头'
    return
  }
  const requestPayload = {
    shot_ids: shotIDs,
    candidates_per_shot: Number(options.candidatesPerShot || imageCandidatesPerShot.value || 4),
    mode: options.mode || imageGenerationMode.value,
    lock_level: actorLockLevel.value
  }
  if (options.sourceWorkID) requestPayload.source_work_id = options.sourceWorkID
  const payload = await runAction('shot-images-generate', () => api.generateNovelVideoShotImages(project.value.id, requestPayload))
  if (payload) {
    imageBatchResult.value = payload
    if (payload.items?.length) {
      mergeShotImages(payload.items)
      shotImagesGenerating.value = payload.items.some((item) => ['queued', 'running'].includes(normalizeNovelVideoShotImage(item).generation_status))
      if (shotImagesGenerating.value) startNovelVideoJobPolling()
    } else if ((payload.queued ?? 0) > 0) {
      shotImagesGenerating.value = true
      startNovelVideoJobPolling()
    }
    activeStep.value = 'shots'
    message.value = `已创建 ${payload.queued ?? 0} 张候选图`
  }
}

async function loadShotImagesV2(options = {}) {
  if (!project.value?.id) return
  const params = options.allProject ? {} : (selectedShot.value?.id ? { shot_id: selectedShot.value.id } : {})
  const task = () => api.listNovelVideoShotImages(project.value.id, params)
  const payload = options.silent ? await task().catch((error) => {
    errorMessage.value = error.message
    return null
  }) : await runAction('shot-images-list', task)
  if (payload?.items) {
    const incoming = payload.items.map(normalizeNovelVideoShotImage)
    mergeShotImages(incoming, options.allProject ? null : selectedShot.value?.id)
    message.value = `已刷新 ${incoming.length} 张候选图`
    if (incoming.length > 0 && !incoming.some((item) => ['queued', 'running'].includes(item.generation_status))) shotImagesGenerating.value = false
    return incoming.length
  }
  return 0
}

async function selectShotImage(image) {
  if (!project.value?.id || !image?.id) return
  const payload = await runAction(`shot-image-${image.id}`, () => api.updateNovelVideoShotImage(project.value.id, image.id, {
    selected: true,
    review_status: 'approved'
  }))
  if (payload) replaceShotImage(payload)
}

async function markShotImageDrift(image) {
  if (!project.value?.id || !image?.id) return
  const payload = await runAction(`shot-image-drift-${image.id}`, () => api.updateNovelVideoShotImage(project.value.id, image.id, {
    review_status: 'identity_drift',
    review_note: '演员身份漂移，需重生或补充参考图'
  }))
  if (payload) replaceShotImage(payload)
}

async function retryShotImages(shot) {
  if (!shot?.id) return
  selectShot(shot)
  await generateShotImages({
    shotIDs: [shot.id],
    candidatesPerShot: 1,
    mode: imageGenerationMode.value || 'text_to_image'
  })
}

async function regenerateShotImageFrom(image) {
  if (!image?.shot_id) return
  const sourceWorkID = Number(image.work_id || image.source_work_id || 0)
  if (!sourceWorkID) {
    errorMessage.value = '当前候选图还没有可用于图生图重生的源作品'
    return
  }
  imageGenerationMode.value = 'image_to_image'
  await generateShotImages({
    shotIDs: [image.shot_id],
    candidatesPerShot: 1,
    mode: 'image_to_image',
    sourceWorkID
  })
}

async function refreshCostEstimate() {
  if (!project.value?.id) return
  const payload = await runAction('cost-estimate', () => api.getNovelVideoCostEstimate(project.value.id))
  if (payload) {
    costEstimate.value = payload
    message.value = `预计点数 ${payload.project?.total_credits ?? 0}`
  }
}

async function composeProject() {
  if (!project.value?.id) return
  const payload = await runAction('compose', () => api.composeNovelVideoProject(project.value.id))
  if (payload) {
    project.value.compositions = [payload, ...(project.value.compositions ?? [])]
    message.value = payload.output_url ? '合成完成，可进入导出' : '合成任务已更新'
  }
}

async function exportProject() {
  if (!project.value?.id) return
  if (exportMode.value === 'json') {
    const payload = await runAction('export-json', () => api.exportNovelVideoProjectJSON(project.value.id))
    if (payload) {
      downloadJSON(payload, `${safeFilename(project.value.title)}-shot-package.json`)
      message.value = 'JSON 镜头包已导出'
    }
    return
  }
  if (['zip', 'jianying', 'image_package'].includes(exportMode.value)) {
    const blob = await runAction(`export-${exportMode.value}`, () => api.exportNovelVideoProjectPackage(project.value.id, exportMode.value))
    if (blob) {
      downloadBlob(blob, `${safeFilename(project.value.title)}-${exportMode.value}.zip`)
      message.value = exportMode.value === 'jianying' ? '剪映草稿 ZIP 已导出' : exportMode.value === 'image_package' ? '图片包 ZIP 已导出' : '项目 ZIP 已导出'
    }
    return
  }
  const payload = await runAction('export', () => api.exportNovelVideoProject(project.value.id))
  if (payload !== null) {
    exportOutput.value = payload
    message.value = 'Markdown 镜头包已导出'
  }
}

function projectPayload() {
  return {
    title: title.value.trim(),
    source_text: sourceText.value.trim(),
    style_preset: stylePreset.value.trim(),
    content_mode: contentMode.value,
    generation_mode: generationMode.value,
    grid_size: Number(gridSize.value || 4),
    aspect_ratio: aspectRatio.value,
    duration: duration.value,
    video_model: videoModel.value,
    video_settings: videoSettingsPayload()
  }
}

function projectPatchPayload() {
  return {
    title: title.value.trim(),
    style_preset: stylePreset.value.trim(),
    content_mode: contentMode.value,
    generation_mode: generationMode.value,
    grid_size: Number(gridSize.value || 4),
    aspect_ratio: aspectRatio.value,
    duration: duration.value,
    video_model: videoModel.value,
    video_settings: videoSettingsPayload()
  }
}

function replaceCreature(payload) {
  const index = creatures.value.findIndex((item) => item.id === payload.id)
  if (index >= 0) project.value.creatures[index] = payload
  selectedCreatureId.value = payload.id
  inspectorMode.value = 'creature'
}

function replaceShot(payload) {
  payload = normalizeNovelVideoShot(payload)
  for (const episode of episodes.value) {
    const index = (episode.shots ?? []).findIndex((shot) => shot.id === payload.id)
    if (index >= 0) {
      episode.shots[index] = payload
      selectedShotId.value = payload.id
      inspectorMode.value = 'shot'
      return
    }
  }
}

function replaceShotImage(payload) {
  const image = normalizeNovelVideoShotImage(payload)
  if (image.selected) {
    shotImages.value = shotImages.value.map((item) => item.shot_id === image.shot_id ? { ...item, selected: false } : item)
  }
  const index = shotImages.value.findIndex((item) => item.id === image.id)
  if (index >= 0) {
    shotImages.value[index] = image
  } else {
    shotImages.value = [image, ...shotImages.value]
  }
}

function mergeShotImages(items, replaceShotID = null) {
  const incoming = items.map(normalizeNovelVideoShotImage)
  const incomingIDs = new Set(incoming.map((item) => item.id))
  shotImages.value = [
    ...shotImages.value.filter((item) => !incomingIDs.has(item.id) && (!replaceShotID || item.shot_id !== replaceShotID)),
    ...incoming
  ]
}

function normalizeNovelVideoShot(shot) {
  if (!shot) return shot
  return {
    ...shot,
    video_prompt: shot.video_prompt ?? shot.prompt ?? '',
    voiceover_text: shot.voiceover_text ?? shot.subtitle_text ?? '',
    duration_seconds: shot.duration_seconds ?? Number(duration.value || 0),
    asset_refs: shot.asset_refs ?? [],
    asset_refs_json: JSON.stringify(shot.asset_refs ?? [], null, 2)
  }
}

function selectCreature(creature) {
  selectedCreatureId.value = creature.id
  inspectorMode.value = 'creature'
  inspectorOpen.value = true
}

function selectAsset(asset) {
  selectedAssetId.value = asset.id
  inspectorMode.value = 'asset'
  inspectorOpen.value = true
}

function selectEpisode(episode) {
  selectedEpisodeId.value = episode.id
  const firstShot = episode.shots?.[0]
  if (firstShot) selectedShotId.value = firstShot.id
}

function episodeProductionStatus(episode) {
  const shots = episode.shots ?? []
  if (!shots.length) return { key: 'pending', label: '待生成' }
  const statuses = shots.map((shot) => shotImageRowFor(shot).generationStatus || shot.status)
  if (statuses.some((status) => status === 'failed')) return { key: 'failed', label: '有失败' }
  if (statuses.some((status) => ['queued', 'running'].includes(status))) return { key: 'running', label: '生成中' }
  if (statuses.every((status) => ['approved', 'succeeded', 'completed'].includes(status))) return { key: 'succeeded', label: '已完成' }
  return { key: 'pending', label: '待生成' }
}

function openEpisodeShots(episode) {
  selectEpisode(episode)
  episodeShotsPage.value = 1
  episodeShotsPageSize.value = 10
  episodeShotsModalOpen.value = true
}

function closeEpisodeShots() {
  episodeShotsModalOpen.value = false
  episodeShotsPage.value = 1
  episodeShotsPageSize.value = 10
}

function changeEpisodeShotsPageSize() {
  episodeShotsPage.value = 1
}

function handleWorkspaceKeydown(event) {
  if (event.key === 'Escape' && episodeShotsModalOpen.value) closeEpisodeShots()
}

function selectShot(shot) {
  selectedShotId.value = shot.id
  selectedEpisodeId.value = shot.episode_id ?? shot.episode?.id
  inspectorMode.value = 'shot'
  inspectorOpen.value = true
}

function scrollToStep(stepKey) {
  activeStep.value = stepKey
  stepRefs[stepKey]?.scrollIntoView?.({ behavior: 'smooth', block: 'start' })
}

function setStepRef(key, element) {
  if (element) stepRefs[key] = element
}

function statusText(status) {
  const labels = {
    draft: '草稿',
    needs_review: '待审核',
    approved: '已批准',
    queued: '排队中',
    running: '生成中',
    succeeded: '已生成',
    failed: '失败',
    partial_failed: '部分失败'
  }
  return labels[status] ?? status ?? '待审核'
}

function productionStatusText(status) {
  if (!status) return '未开始'
  if (['draft', 'analyzed', 'planned', 'rendering'].includes(status)) return '制作中'
  if (status === 'succeeded') return '已完成'
  if (['failed', 'partial_failed'].includes(status)) return '需处理'
  return statusText(status)
}

function workflowReviewState(items) {
  if (!items.length) return 'pending'
  if (items.some((item) => ['failed', 'partial_failed'].includes(item.status ?? item.review_status))) return 'failed'
  if (items.every((item) => ['approved', 'succeeded'].includes(item.status ?? item.review_status))) return 'done'
  return 'current'
}

function stepIcon(state) {
  if (state === 'done') return Check
  if (state === 'running') return LoaderCircle
  if (state === 'failed') return CircleAlert
  return Clock
}

function progressOf(shot) {
  if (typeof shot?.generation_progress === 'number') return shot.generation_progress
  if (shot?.status === 'running') return 65
  if (shot?.status === 'succeeded') return 100
  return 0
}

function preflightBlockedReason(shot) {
  return shot?.blocked_reason || (Array.isArray(shot?.block_reasons) ? shot.block_reasons.join('；') : '')
}

function triggerDocumentImport() {
  fileInput.value?.click()
}

async function handleDocumentFile(event) {
  const file = event.target.files?.[0]
  if (!file) return
  message.value = ''
  errorMessage.value = ''
  try {
    const result = await parseNovelSourceFile(file)
    const importedText = result.text.slice(0, 50000)
    sourceText.value = importedText
    message.value = result.text.length > 50000
      ? '文档已导入，内容已截断为前 50,000 字'
      : '文档已导入'
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    event.target.value = ''
  }
}

function copyPrompt() {
  if (!selectedShot.value?.prompt) return
  navigator.clipboard?.writeText(selectedShot.value.prompt)
  message.value = 'Prompt 已复制'
}

function clearPrompt() {
  if (selectedShot.value) selectedShot.value.prompt = ''
}

function openAssets() {
  window.location.assign('/assets')
}

function goTo(path) {
  window.location.assign(path)
}

function toast(text) {
  message.value = text
}

function safeFilename(value) {
  return `${value || 'novel-video-project'}`.replace(/[\\/:*?"<>|]+/g, '-')
}

function downloadJSON(payload, filename) {
  const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json;charset=utf-8' })
  downloadBlob(blob, filename)
}

function downloadBlob(blob, filename) {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  link.click()
  URL.revokeObjectURL(url)
}

function startNovelVideoJobPolling() {
  stopProjectPolling()
  if (!project.value?.id) return
  pollTimer = window.setInterval(async () => {
    const projectID = project.value?.id
    if (!projectID) {
      stopProjectPolling()
      return
    }
    const payload = await api.listNovelVideoEvents(projectID).catch((error) => {
      errorMessage.value = error.message
      return null
    })
    if (payload?.items) mergeNovelVideoJobs(payload.items)

    const hasActiveJobs = activeNovelVideoJobs.value.length > 0
    if (hasActiveJobs || assetImageJobs.value.some((job) => job.status === 'succeeded')) {
      const latestProject = await api.getNovelVideoProject(projectID).catch(() => null)
      if (latestProject) setProject(latestProject)
    }
    if (shotImagesGenerating.value) {
      await loadShotImages({ silent: true, allProject: true })
    }
    if (!activeNovelVideoJobs.value.length && !shotImagesGenerating.value) {
      stopProjectPolling()
    }
  }, 3000)
}

function startProjectPolling() {
  stopProjectPolling()
  if (!project.value?.id) return
  pollTimer = window.setInterval(async () => {
    const payload = await api.getNovelVideoProject(project.value.id).catch((error) => {
      errorMessage.value = error.message
      return null
    })
    if (payload) {
      setProject(payload)
      if (payload.status !== 'rendering' && !projectHasActiveCreatureGeneration(payload)) stopProjectPolling()
    }
  }, 5000)
}

function stopProjectPolling() {
  if (pollTimer) {
    window.clearInterval(pollTimer)
    pollTimer = null
  }
}

function setupObserver() {
  if (typeof IntersectionObserver === 'undefined') return
  observer?.disconnect()
  observer = new IntersectionObserver((entries) => {
    const visible = entries.filter((entry) => entry.isIntersecting).sort((a, b) => b.intersectionRatio - a.intersectionRatio)[0]
    if (visible?.target?.dataset?.stepKey) activeStep.value = visible.target.dataset.stepKey
  }, { rootMargin: '-20% 0px -65% 0px', threshold: [0.15, 0.4, 0.75] })
  Object.values(stepRefs).forEach((element) => observer.observe(element))
}

function handleBeforeUnload() {
  persistNovelVideoDraftIfNeeded()
}

onMounted(async () => {
  loading.value = true
  window.addEventListener('beforeunload', handleBeforeUnload)
  window.addEventListener('keydown', handleWorkspaceKeydown)
  if (window.innerWidth <= 900) {
    inspectorOpen.value = false
  }
  try {
    const [user] = await Promise.all([
      loadCurrentUser({ force: true }),
      loadVideoModels()
    ])
    me.value = user
    const params = new URLSearchParams(window.location.search)
    const projectID = params.get('project_id') || params.get('id')
    if (projectID) {
      const payload = await api.getNovelVideoProject(projectID)
      applyLoadedProject(payload)
    }
  } catch {
    me.value = null
  } finally {
    loading.value = false
    await nextTick()
    setupObserver()
  }
})

onUnmounted(() => {
  observer?.disconnect()
  window.removeEventListener('beforeunload', handleBeforeUnload)
  window.removeEventListener('keydown', handleWorkspaceKeydown)
  stopProjectPolling()
})
</script>

<template>
  <section class="novel-studio-shell" :class="studioThemeClass" :data-theme="theme">
    <aside class="novel-studio-sidebar" :class="{ collapsed: sidebarCollapsed }">
      <div class="studio-brand">
        <div>
          <span class="studio-kicker">Novel Video</span>
          <h1 v-if="!sidebarCollapsed">小说视频工作台</h1>
        </div>
        <div class="studio-brand-actions">
          <button
            type="button"
            class="icon-button"
            data-testid="novel-history-open"
            title="历史小说项目"
            aria-label="历史小说项目"
            @click="openProjectHistory"
          >
            <History :size="15" />
          </button>
          <button type="button" class="icon-button" title="编辑项目名" @click="projectSettingsOpen = !projectSettingsOpen">
            <Edit3 :size="15" />
          </button>
          <button
            type="button"
            class="icon-button sidebar-collapse-button"
            data-testid="novel-sidebar-toggle"
            title="折叠侧栏"
            :aria-label="sidebarCollapsed ? '展开侧栏' : '折叠侧栏'"
            @click="sidebarCollapsed = !sidebarCollapsed"
          >
            <ChevronLeft :size="16" />
          </button>
        </div>
      </div>

      <div class="project-card">
        <span>{{ projectStatusText }}</span>
        <strong v-if="!sidebarCollapsed">{{ project?.title || title || '未创建项目' }}</strong>
        <small v-if="!sidebarCollapsed">{{ me?.available_credits ?? 0 }} 点可用</small>
      </div>

      <nav class="workflow-nav" aria-label="小说视频流程">
        <button
          v-for="(step, index) in workflowSteps"
          :key="step.key"
          type="button"
          class="workflow-step"
          :class="[step.state, { active: activeStep === step.key }]"
          :data-testid="`workflow-step-${step.key}`"
          @click="scrollToStep(step.key)"
        >
          <span class="workflow-number">{{ index + 1 }}</span>
          <span v-if="!sidebarCollapsed" class="workflow-copy">
            <strong>{{ step.label }}</strong>
            <small>{{ statusText(step.state === 'done' ? 'approved' : step.state === 'running' ? 'running' : step.state === 'failed' ? 'failed' : 'needs_review') }}</small>
          </span>
          <component :is="stepIcon(step.state)" v-if="!sidebarCollapsed" :size="15" class="workflow-icon" />
        </button>
      </nav>
    </aside>

    <main class="novel-studio-main">
      <div v-if="localDraftNotice" class="local-draft-notice" data-testid="novel-local-draft-notice">
        <div>
          <strong>本机草稿</strong>
          <span>当前项目有未提交的本机草稿，服务器内容未被覆盖。</span>
        </div>
        <div class="local-draft-actions">
          <button type="button" class="secondary-action compact" data-testid="novel-local-draft-restore" @click="restoreLocalDraftNotice">恢复本机草稿</button>
          <button type="button" class="ghost-button compact" data-testid="novel-local-draft-discard" @click="discardLocalDraftNotice">丢弃</button>
        </div>
      </div>
      <section :ref="(el) => setStepRef('import', el)" data-step-key="import" class="studio-section import-section">
        <header class="section-header">
          <div>
            <span>Step 01</span>
            <h2>小说导入</h2>
          </div>
          <button type="button" class="ghost-button" @click="triggerDocumentImport">
            <FileText :size="15" />
            <span>导入文档</span>
          </button>
          <input
            ref="fileInput"
            data-testid="novel-source-file"
            type="file"
            accept=".md,.markdown,.txt,.docx,.pdf,text/markdown,text/plain,application/vnd.openxmlformats-officedocument.wordprocessingml.document,application/pdf"
            hidden
            @change="handleDocumentFile"
          />
        </header>

        <div class="import-grid">
          <label class="studio-field source-field">
            <span>小说正文</span>
            <textarea v-model="sourceText" data-testid="novel-source" maxlength="50000" />
            <small :class="{ warn: sourceChars > 50000 }">{{ sourceChars.toLocaleString() }} / 50,000</small>
          </label>

          <div class="settings-panel" :class="{ closed: !projectSettingsOpen }">
            <button type="button" class="settings-toggle" @click="projectSettingsOpen = !projectSettingsOpen">
              <Settings :size="15" />
              <span>项目设置</span>
            </button>
            <div v-if="projectSettingsOpen" class="settings-grid">
              <label class="studio-field">
                <span>项目标题</span>
                <input v-model="title" data-testid="novel-title" type="text" />
              </label>
              <label class="studio-field">
                <span>风格</span>
                <input v-model="stylePreset" data-testid="novel-style" type="text" />
              </label>
              <label class="studio-field">
                <span>内容模式</span>
                <ClickSelect v-model="contentMode" :options="contentModeOptions" data-testid="novel-content-mode" aria-label="内容模式" />
              </label>
              <label class="studio-field">
                <span>生成模式</span>
                <ClickSelect v-model="generationMode" :options="generationModeOptions" data-testid="novel-generation-mode" aria-label="生成模式" />
              </label>
              <label class="studio-field">
                <span>宫格</span>
                <ClickSelect v-model="gridSize" :options="gridSizeOptions" data-testid="novel-grid-size" aria-label="宫格" />
              </label>
              <label class="studio-field">
                <span>画幅</span>
                <ClickSelect v-model="aspectRatio" :options="aspectRatioOptions" aria-label="画幅" />
              </label>
              <label class="studio-field">
                <span>单镜头时长</span>
                <ClickSelect v-model="duration" :options="durationOptions" data-testid="novel-duration" aria-label="单镜头时长" />
              </label>
              <label v-if="shouldShowResolutionSelect" class="studio-field">
                <span>分辨率</span>
                <ClickSelect v-model="resolution" :options="resolutionOptions" data-testid="novel-resolution" aria-label="分辨率" />
              </label>
              <label v-if="shouldShowGenerateAudioToggle" class="studio-toggle">
                <input v-model="generateAudio" data-testid="novel-generate-audio" type="checkbox" />
                <span>生成音频</span>
              </label>
              <label class="studio-field full">
                <span>视频模型</span>
                <ClickSelect v-model="videoModel" :options="videoModelOptions" data-testid="novel-video-model" aria-label="视频模型" />
              </label>
            </div>
          </div>
        </div>

        <div class="section-actions">
          <button type="button" class="primary-action" data-testid="novel-create" :disabled="(!project?.id && !canCreate) || Boolean(busyAction)" @click="createAndAnalyzeProject">
            <Wand2 :size="16" />
            <span>{{ busyAction === 'create' ? '解析中...' : project?.id ? '保存并重新解析' : '创建项目并解析' }}</span>
          </button>
          <button type="button" class="secondary-action" data-testid="novel-analyze" :disabled="!project?.id || Boolean(busyAction)" @click="analyzeProject">
            <Sparkles :size="16" />
            <span>仅重新解析</span>
          </button>
          <button type="button" class="secondary-action" :disabled="!project?.id || Boolean(busyAction)" @click="saveProjectSettings">
            <Save :size="16" />
            <span>保存设置</span>
          </button>
        </div>
      </section>

      <section :ref="(el) => setStepRef('bible', el)" data-step-key="bible" class="studio-section">
        <header class="section-header">
          <div>
            <span>Step 02</span>
            <h2>故事圣经</h2>
          </div>
          <button type="button" class="ghost-button" :disabled="!project?.id || Boolean(busyAction)" @click="saveStoryBible">
            <Save :size="15" />
            <span>保存</span>
          </button>
        </header>
        <div class="bible-grid">
          <label v-for="field in [
            ['logline', '一句话故事'],
            ['world', '世界观'],
            ['conflict', '主要冲突'],
            ['visual_style', '视觉风格'],
            ['risk_highlight', '内容风险提示']
          ]" :key="field[0]" class="bible-card">
            <span>{{ field[1] }}</span>
            <textarea v-model="storyDraft[field[0]]" rows="3" />
          </label>
        </div>
      </section>

      <section :ref="(el) => setStepRef('assets', el)" data-step-key="assets" class="studio-section">
        <header class="section-header">
          <div>
            <span>Step 03</span>
            <h2>资产板</h2>
          </div>
          <div class="section-header-actions">
            <button
              v-if="duplicateAssetGroups.length > 0"
              type="button"
              class="ghost-button"
              data-testid="asset-dedupe-assets"
              :disabled="!project?.id || Boolean(busyAction)"
              @click="dedupeAssets"
            >
              <Trash2 :size="15" />
              <span>{{ busyAction === 'assets-dedupe' ? '清理中...' : '清理重复草案' }}</span>
            </button>
            <button type="button" class="ghost-button" data-testid="novel-generate-assets" :disabled="!project?.id || Boolean(busyAction) || hasActiveAssetImageJobs" @click="generateAssets">
              <Sparkles :size="15" />
              <span>{{ hasActiveAssetImageJobs ? '资产图生成中...' : busyAction === 'assets-generate' ? '生成中...' : '生成资产草案' }}</span>
            </button>
          </div>
        </header>
        <div v-if="assetGenerationSummary.active > 0" class="generation-status-strip" data-testid="asset-generation-status">
          <LoaderCircle :size="15" />
          <span>正在生成资产图 {{ assetGenerationSummary.done }}/{{ assetGenerationSummary.total }}，可离开页面，后台继续处理</span>
        </div>
        <div class="creature-grid">
          <article
            v-for="asset in visibleAssets"
            :key="asset.id"
            class="creature-card asset-card"
            :class="{ selected: selectedAsset?.id === asset.id }"
            :data-testid="`asset-card-${asset.id}`"
            @click="selectAsset(asset)"
          >
            <button
              type="button"
              class="asset-card-delete icon-button"
              :data-testid="`asset-delete-${asset.id}`"
              :disabled="assetDeleteDisabled(asset)"
              :title="assetDeleteTitle(asset)"
              :aria-label="`删除资产 ${asset.name}`"
              @click.stop="deleteAsset(asset)"
            >
              <Trash2 :size="14" />
            </button>
            <div class="creature-image">
              <img v-if="asset.asset_url || asset.reference_url" :src="asset.asset_url || asset.reference_url" :alt="asset.name" />
              <Image v-else :size="30" />
              <div v-if="assetGenerationState(asset)" :class="['generation-overlay', assetGenerationState(asset).status]" :data-testid="`asset-generation-overlay-${asset.id}`">
                <strong>{{ assetGenerationState(asset).label }}</strong>
                <small v-if="assetGenerationState(asset).error">{{ assetGenerationState(asset).error }}</small>
                <button v-if="assetGenerationState(asset).retryable" type="button" :data-testid="`asset-retry-${asset.id}`" @click.stop="retryAssetImage(asset)">重试</button>
              </div>
            </div>
            <div class="creature-body">
              <div class="card-title-row">
                <h3>{{ asset.name }}</h3>
                <span :class="['status-badge', asset.review_status]">{{ statusText(asset.review_status) }}</span>
              </div>
              <small>{{ asset.kind }} · v{{ asset.version || 1 }}</small>
              <p>{{ asset.description }}</p>
              <code>{{ asset.prompt }}</code>
            </div>
          </article>
          <p v-if="!visibleAssets.length" class="empty-line">故事圣经确认后生成角色、场景、道具、线索和风格参考资产。</p>
        </div>
      </section>

      <section :ref="(el) => setStepRef('creatures', el)" data-step-key="creatures" class="studio-section">
        <header class="section-header">
          <div>
            <span>Step 03</span>
            <h2>演员锁定</h2>
          </div>
          <div class="segmented">
            <button type="button" :class="{ active: creatureViewMode === 'cards' }" @click="creatureViewMode = 'cards'"><Image :size="14" /></button>
            <button type="button" :class="{ active: creatureViewMode === 'list' }" @click="creatureViewMode = 'list'"><LayoutList :size="14" /></button>
          </div>
        </header>
        <div class="creature-grid" :class="{ list: creatureViewMode === 'list' }">
          <article
            v-for="creature in creatures"
            :key="creature.id"
            class="creature-card actor-card"
            :class="{ selected: selectedCreatureId === creature.id }"
            :data-testid="`creature-card-${creature.id}`"
            @click="selectCreature(creature)"
          >
            <div class="creature-card-main">
              <div class="creature-image">
                <img v-if="creature.work_preview_url || creature.asset_url" :src="creature.work_preview_url || creature.asset_url" :alt="creature.name" />
                <Image v-else :size="30" />
                <div v-if="creatureGenerationState(creature)" :class="['generation-overlay', creatureGenerationState(creature).status]" :data-testid="`creature-generation-overlay-${creature.id}`">
                  <strong>{{ creatureGenerationState(creature).label }}</strong>
                  <small v-if="creatureGenerationState(creature).error">{{ creatureGenerationState(creature).error }}</small>
                </div>
              </div>
              <div class="creature-body creature-summary">
                <div class="card-title-row">
                  <h3>{{ creature.name }}</h3>
                  <span :class="['status-badge', creature.review_status]">{{ statusText(creature.review_status) }}</span>
                </div>
                <small>{{ creature.creature_type || '未分类' }}</small>
                <p>{{ creature.appearance }}</p>
                <p>{{ creature.abilities }}</p>
              </div>
            </div>
            <div class="creature-card-footer">
              <code class="creature-consistency-prompt">{{ creature.visual_consistency_prompt }}</code>
              <small class="creature-reference-status">参考图 {{ creature.reference_asset_ids?.length ?? 0 }} 张 · {{ creature.lock_level || actorLockLevel }}</small>
              <div class="row-actions creature-card-actions">
                <button type="button" :data-testid="`creature-approve-${creature.id}`" @click.stop="approveCreature(creature)"><Check :size="14" />批准</button>
                <button type="button" :data-testid="`actor-approve-${creature.id}`" @click.stop="approveActor(creature)"><Check :size="14" />锁定演员</button>
                <button type="button" :data-testid="`actor-lock-sheet-${creature.id}`" @click.stop="generateActorLockSheet(creature)"><Image :size="14" />定妆图</button>
                <button type="button" @click.stop="selectCreature(creature)"><Edit3 :size="14" />编辑</button>
                <button type="button" @click.stop="generateCreatureImage(creature)"><Image :size="14" />设定图</button>
                <button type="button" @click.stop="generateCreatureImage(creature)"><RefreshCw :size="14" />重试</button>
              </div>
            </div>
          </article>
          <p v-if="!creatures.length" class="empty-line">生成图片计划后将出现演员卡；每位演员至少绑定一组参考图后再进入严格批量生图。</p>
        </div>
      </section>

      <section :ref="(el) => setStepRef('shots', el)" data-step-key="shots" class="studio-section shots-section">
        <header class="section-header">
          <div>
            <span>Step 04</span>
            <h2>镜头图片生产台</h2>
          </div>
          <div class="section-header-actions">
            <label class="inline-number">
              <span>候选/镜头</span>
              <input v-model.number="imageCandidatesPerShot" type="number" min="1" max="8" />
            </label>
            <div class="segmented text" aria-label="图片生成模式">
              <button type="button" :class="{ active: imageGenerationMode === 'text_to_image' }" @click="imageGenerationMode = 'text_to_image'">文生图</button>
              <button type="button" :class="{ active: imageGenerationMode === 'image_to_image' }" @click="imageGenerationMode = 'image_to_image'">图生图</button>
            </div>
            <div class="segmented text" aria-label="演员锁定强度">
              <button type="button" :class="{ active: actorLockLevel === 'strict' }" @click="actorLockLevel = 'strict'">强锁定</button>
              <button type="button" :class="{ active: actorLockLevel === 'medium' }" @click="actorLockLevel = 'medium'">中等</button>
            </div>
            <button type="button" class="primary-action compact" data-testid="novel-generate-shot-images" :disabled="!project?.id || !flatShots.length || Boolean(busyAction)" @click="generateShotImages">
              <Image :size="15" />
              <span>{{ busyAction === 'shot-images-generate' ? '创建中...' : `生成 ${expectedImageCandidateCount} 张候选图` }}</span>
            </button>
            <button type="button" class="secondary-action compact" :disabled="!project?.id || Boolean(busyAction)" @click="loadShotImages">
              <RefreshCw :size="15" />
              <span>刷新候选</span>
            </button>
            <label class="inline-number">
              <span>计划镜头</span>
              <input v-model.number="imagePlanShotCount" type="number" min="1" max="80" />
            </label>
            <button type="button" class="ghost-button" data-testid="novel-image-plan" :disabled="!project?.id || Boolean(busyAction)" @click="planImageSeries">
              <Image :size="15" />
              <span>生成图片计划</span>
            </button>
            <button type="button" class="ghost-button" data-testid="novel-plan-episodes" :disabled="!project?.id || Boolean(busyAction)" @click="planEpisodes">
              <Clapperboard :size="15" />
              <span>生成镜头表</span>
            </button>
          </div>
        </header>
        <div v-if="episodes.length" class="episode-card-grid">
          <button
            v-for="episode in episodes"
            :key="episode.id"
            type="button"
            class="episode-entry-card"
            :data-testid="`episode-card-${episode.id}`"
            @click="openEpisodeShots(episode)"
          >
            <strong>第 {{ episode.number }} 集</strong>
            <span :class="['status-badge', episodeProductionStatus(episode).key]">{{ episodeProductionStatus(episode).label }}</span>
          </button>
        </div>
        <p v-else class="empty-line">请先生成镜头表，再按分集查看镜头详情。</p>
      </section>

      <div v-if="episodeShotsModalOpen" class="episode-modal-backdrop" @click.self="closeEpisodeShots">
        <section class="episode-shots-modal" role="dialog" aria-modal="true" :aria-label="`第 ${selectedEpisode?.number} 集镜头详情`" data-testid="episode-shots-modal">
          <header class="episode-modal-header">
            <div>
              <span>镜头图片生产台</span>
              <h2>第 {{ selectedEpisode?.number }} 集镜头详情</h2>
            </div>
            <button type="button" class="icon-button" aria-label="关闭镜头详情" data-testid="episode-shots-close" @click="closeEpisodeShots"><X :size="18" /></button>
          </header>
          <div class="episode-modal-content">
            <div class="shot-table-wrap">
              <table class="shot-table production-table episode-shots-table" data-testid="episode-shots-table">
                <colgroup>
                  <col class="shot-column-number" style="width: 60px" />
                  <col class="shot-column-title" style="width: 200px" />
                  <col class="shot-column-prompt" style="width: 280px" />
                  <col class="shot-column-creatures" style="width: 90px" />
                  <col class="shot-column-reference" style="width: 90px" />
                  <col class="shot-column-status" style="width: 150px" />
                  <col class="shot-column-actions" style="width: 300px" />
                </colgroup>
              <thead>
                <tr>
                  <th>镜头</th>
                  <th>镜头标题</th>
                  <th>Prompt 摘要</th>
                  <th>出场生物</th>
                  <th>参考图</th>
                  <th>状态</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="shot in paginatedEpisodeShots"
                  :key="shot.id"
                  :class="{ selected: selectedShotId === shot.id }"
                  :data-testid="`shot-image-row-${shot.id}`"
                  @click="selectShot(shot)"
                >
                  <td>{{ shot.number }}</td>
                  <td>
                    <strong>{{ shot.title }}</strong>
                    <div class="shot-image-thumb">
                      <img v-if="shotImageRowFor(shot).thumbnailUrl" :src="shotImageRowFor(shot).thumbnailUrl" :alt="`${shot.title} 定稿或候选图`" :data-testid="`shot-image-thumb-${shot.id}`" />
                      <Image v-else :size="22" />
                    </div>
                  </td>
                  <td><div class="shot-prompt-clamp">{{ shot.image_prompt || shot.prompt }}</div></td>
                  <td>{{ (shot.creature_ids ?? []).length || '-' }}</td>
                  <td>{{ shot.reference_asset_id ? '已绑定' : '-' }}</td>
                  <td class="shot-status-cell">
                    <span :class="['status-badge', shotImageRowFor(shot).generationStatus || shot.status]">{{ shotImageRowStatusText(shotImageRowFor(shot)) }}</span>
                    <div class="progress-track compact">
                      <i :data-testid="`shot-image-progress-${shot.id}`" :style="{ width: `${shotImageRowFor(shot).progress}%` }" />
                    </div>
                    <small>{{ shotImageRowFor(shot).progress }}%</small>
                    <small v-if="shotImageRowFor(shot).errorMessage" class="error-inline">{{ shotImageRowFor(shot).errorMessage }}</small>
                  </td>
                  <td class="shot-actions-cell">
                    <div class="shot-row-actions" :data-testid="`shot-row-actions-${shot.id}`">
                      <strong class="shot-candidate-count">{{ shotImageRowFor(shot).candidateCount }}</strong>
                      <span v-if="shotImageRowFor(shot).selected" class="selected-pill" :data-testid="`shot-image-selected-${shot.id}`">定稿</span>
                      <button type="button" :data-testid="`shot-image-open-candidates-${shot.id}`" @click.stop="selectShot(shot)"><Image :size="13" />查看候选</button>
                      <button v-if="shotImageRowFor(shot).canRetry" type="button" :data-testid="`shot-image-retry-${shot.id}`" @click.stop="retryShotImages(shot)"><RefreshCw :size="13" />重试</button>
                      <button v-if="shot.work_preview_url" type="button" @click.stop="window.open(shot.work_preview_url, '_blank')"><Play :size="13" />播放</button>
                      <button v-else type="button" :data-testid="`shot-approve-${shot.id}`" @click.stop="approveShot(shot)"><Check :size="13" />批准</button>
                      <button type="button" :data-testid="`novel-storyboard-${shot.id}`" @click.stop="generateStoryboard(shot)"><Image :size="13" />分镜图</button>
                    </div>
                  </td>
                </tr>
              </tbody>
              </table>
              <p v-if="!selectedEpisodeShots.length" class="empty-line">该集暂无镜头。</p>
            </div>
            <div v-if="selectedEpisodeShots.length" class="episode-pagination">
              <span>{{ episodeShotsRangeLabel }}</span>
              <label>
                <span>每页</span>
                <select v-model.number="episodeShotsPageSize" data-testid="episode-shots-page-size" @change="changeEpisodeShotsPageSize">
                  <option :value="10">10 条</option>
                  <option :value="20">20 条</option>
                  <option :value="50">50 条</option>
                </select>
              </label>
              <button type="button" :disabled="episodeShotsPage <= 1" data-testid="episode-shots-previous-page" @click="episodeShotsPage -= 1"><ChevronLeft :size="14" />上一页</button>
              <strong>第 {{ episodeShotsPage }} / {{ episodeShotsPageCount }} 页</strong>
              <button type="button" :disabled="episodeShotsPage >= episodeShotsPageCount" data-testid="episode-shots-next-page" @click="episodeShotsPage += 1">下一页<ChevronRight :size="14" /></button>
            </div>
            <div class="shot-image-grid" data-testid="shot-image-grid">
              <article
                v-for="imageItem in selectedShotImages"
                :key="imageItem.id"
                class="shot-image-card"
                :class="{ selected: imageItem.selected }"
                :data-testid="`shot-image-card-${imageItem.id}`"
              >
            <div class="shot-image-preview">
              <img v-if="imageItem.preview_url || imageItem.download_url" :src="imageItem.preview_url || imageItem.download_url" :alt="`候选图 ${imageItem.version}`" />
              <Image v-else :size="30" />
            </div>
            <div class="shot-image-body">
              <div class="card-title-row">
                <h3>候选图 v{{ imageItem.version }}</h3>
                <span :class="['status-badge', imageItem.selected ? 'approved' : imageItem.generation_status || imageItem.review_status]">{{ imageItem.selected ? '定稿' : statusText(imageItem.generation_status || imageItem.review_status) }}</span>
              </div>
              <small>{{ imageItem.mode }} · {{ imageItem.reference_intent }} · 参考 {{ imageItem.reference_asset_ids?.length ?? 0 }} · {{ imageItem.generation_progress ?? 0 }}%</small>
              <p>{{ imageItem.prompt }}</p>
              <p v-if="imageItem.error_message" class="error-inline">{{ imageItem.error_message }}</p>
              <div class="row-actions">
                <button type="button" :data-testid="`shot-image-select-${imageItem.id}`" @click="selectShotImage(imageItem)"><Check :size="14" />设为定稿</button>
                <button type="button" :data-testid="`shot-image-regenerate-${imageItem.id}`" @click="regenerateShotImageFrom(imageItem)"><RefreshCw :size="14" />图生图重生</button>
                <button type="button" @click="markShotImageDrift(imageItem)"><CircleAlert :size="14" />标记漂移</button>
              </div>
            </div>
              </article>
              <p v-if="shouldShowShotImagesGenerating" class="empty-line">候选图生成中，完成后会自动刷新到当前镜头。</p>
              <p v-else-if="!selectedShotImages.length" class="empty-line">批量生成后，当前镜头的候选图会显示在这里，可设为定稿或标记演员漂移。</p>
            </div>
          </div>
        </section>
      </div>

      <section :ref="(el) => setStepRef('render', el)" data-step-key="render" class="studio-section render-section">
        <header class="section-header">
          <div>
            <span>Step 05</span>
            <h2>渲染与导出任务</h2>
          </div>
          <div class="section-header-actions">
            <button type="button" class="secondary-action compact" data-testid="novel-generate-grids" :disabled="!project?.id || approvedShotCount === 0 || Boolean(busyAction)" @click="generateGrids">
              <LayoutList :size="15" />
              <span>{{ busyAction === 'grids-generate' ? '生成中...' : '宫格' }}</span>
            </button>
            <button type="button" class="secondary-action compact" data-testid="novel-cost-estimate" :disabled="!project?.id || Boolean(busyAction)" @click="refreshCostEstimate">
              <Clock :size="15" />
              <span>估算</span>
            </button>
            <button type="button" class="secondary-action compact" data-testid="novel-render-preflight" :disabled="!project?.id || approvedShotCount === 0 || Boolean(busyAction)" @click="preflightRenderShots">
              <Check :size="15" />
              <span>{{ busyAction === 'render-preflight' ? '预检中...' : '预检' }}</span>
            </button>
            <button type="button" class="primary-action compact" data-testid="novel-render" :disabled="!project?.id || approvedShotCount === 0 || Boolean(busyAction)" @click="renderShots">
              <Play :size="15" />
              <span>{{ busyAction === 'render' ? '入队中...' : `渲染 ${approvedShotCount} 个镜头` }}</span>
            </button>
          </div>
        </header>
        <div class="image-batch-summary">
          <div><span>镜头</span><strong>{{ imagePlanReadyShotCount }}</strong></div>
          <div><span>预计候选</span><strong>{{ expectedImageCandidateCount }}</strong></div>
          <div><span>已选定稿</span><strong>{{ selectedImageCount }}</strong></div>
          <div><span>引用约束</span><strong>{{ actorLockLevel }}</strong></div>
        </div>
        <div class="render-board">
          <div class="render-now">
            <span>当前生成镜头</span>
            <strong>{{ currentRenderingShot?.title || '暂无镜头' }}</strong>
            <div class="progress-track">
              <i :style="{ width: `${progressOf(currentRenderingShot)}%` }" />
            </div>
            <small>{{ progressOf(currentRenderingShot) }}%</small>
          </div>
          <div class="render-metrics">
            <div><span>已完成</span><strong>{{ renderStats.done }}</strong></div>
            <div><span>失败</span><strong>{{ renderStats.failed }}</strong></div>
            <div><span>可渲染</span><strong>{{ renderResult?.renderable ?? approvedShotCount }}</strong></div>
            <div><span>阻塞</span><strong>{{ renderResult?.blocked ?? 0 }}</strong></div>
            <div><span>队列</span><strong>{{ renderResult?.queued ?? renderStats.running }}</strong></div>
            <div><span>预计点数</span><strong>{{ renderResult?.required_credits ?? renderStats.requiredCredits }}</strong></div>
          </div>
        </div>
        <div v-if="costEstimate?.project" class="cost-estimate" data-testid="novel-cost-estimate-summary">
          <span>Cost</span>
          <strong>{{ costEstimate.project.total_credits ?? 0 }}</strong>
          <small>Shots {{ costEstimate.project.shot_credits ?? 0 }} · Grid {{ costEstimate.project.grid_credits ?? 0 }}</small>
        </div>
        <div v-if="renderResult?.shots?.some((shot) => preflightBlockedReason(shot))" class="preflight-list" data-testid="novel-render-preflight-list">
          <div v-for="shot in renderResult.shots.filter((item) => preflightBlockedReason(item))" :key="shot.shot_id" class="preflight-row">
            <span>{{ shot.episode_number }}-{{ shot.shot_number }}</span>
            <strong>{{ shot.title }}</strong>
            <small>{{ preflightBlockedReason(shot) }}</small>
          </div>
        </div>
        <div class="queue-toolbar">
          <button type="button" class="queue-toggle" @click="renderQueueExpanded = !renderQueueExpanded">
            <Menu :size="14" />
            <span>{{ renderQueueExpanded ? '收起队列' : '展开队列' }}</span>
          </button>
          <span v-if="renderQueueExpanded && shouldShowRenderQueuePagination" class="queue-summary" data-testid="render-queue-summary">{{ renderQueueRangeLabel }}</span>
        </div>
        <table v-if="renderQueueExpanded" class="queue-table">
          <tbody>
            <tr v-for="row in paginatedRenderQueueRows" :key="row.key">
              <td>{{ row.label }}</td>
              <td>{{ row.type }}</td>
              <td><span :class="['status-badge', row.status]">{{ statusText(row.status) }}</span></td>
              <td>{{ row.progress }}%</td>
            </tr>
          </tbody>
        </table>
        <div v-if="renderQueueExpanded && shouldShowRenderQueuePagination" class="queue-pagination">
          <button type="button" class="queue-page-button" data-testid="render-queue-prev" :disabled="renderQueuePage <= 1" @click="goToRenderQueuePage(renderQueuePage - 1)">
            <ChevronLeft :size="14" />
            <span>上一页</span>
          </button>
          <button type="button" class="queue-page-button" data-testid="render-queue-next" :disabled="renderQueuePage >= renderQueuePageCount" @click="goToRenderQueuePage(renderQueuePage + 1)">
            <span>下一页</span>
            <ChevronRight :size="14" />
          </button>
        </div>
      </section>

      <section :ref="(el) => setStepRef('export', el)" data-step-key="export" class="studio-section export-section">
        <header class="section-header">
          <div>
            <span>Step 06</span>
            <h2>导出镜头包</h2>
          </div>
          <div class="segmented text">
            <button type="button" data-testid="export-mode-markdown" :class="{ active: exportMode === 'markdown' }" @click="exportMode = 'markdown'">MD</button>
            <button type="button" data-testid="export-mode-json" :class="{ active: exportMode === 'json' }" @click="exportMode = 'json'">JSON</button>
            <button type="button" data-testid="export-mode-zip" :class="{ active: exportMode === 'zip' }" @click="exportMode = 'zip'">ZIP</button>
            <button type="button" data-testid="export-mode-image-package" :class="{ active: exportMode === 'image_package' }" @click="exportMode = 'image_package'">图片包</button>
            <button type="button" data-testid="export-mode-jianying" :class="{ active: exportMode === 'jianying' }" @click="exportMode = 'jianying'">剪映</button>
          </div>
        </header>
        <div class="export-preview-grid">
          <article><Image :size="18" /><span>每集封面</span><strong>{{ episodes.length }}</strong></article>
          <article><Image :size="18" /><span>生物设定图</span><strong>{{ creatures.filter((item) => item.work_preview_url || item.asset_url).length }}</strong></article>
          <article><Image :size="18" /><span>镜头候选图</span><strong>{{ shotImages.length }}</strong></article>
          <article><Check :size="18" /><span>已选定稿</span><strong>{{ selectedImageCount }}</strong></article>
          <article><Play :size="18" /><span>视频短镜头</span><strong>{{ renderStats.done }}</strong></article>
          <article><FileText :size="18" /><span>Prompt</span><strong>{{ flatShots.length }}</strong></article>
          <article><FileJson :size="18" /><span>素材 URL</span><strong>{{ flatShots.filter((item) => item.work_preview_url).length }}</strong></article>
        </div>
        <div class="section-actions">
          <button type="button" class="gold-action" data-testid="novel-compose" :disabled="!project?.id || Boolean(busyAction)" @click="composeProject">
            <Clapperboard :size="16" />
            <span>{{ busyAction === 'compose' ? '合成中...' : '合成成片' }}</span>
          </button>
          <button type="button" class="primary-action" data-testid="novel-export" :disabled="!project?.id || Boolean(busyAction)" @click="exportProject">
            <Download :size="16" />
            <span>{{ exportMode === 'json' ? '导出 JSON' : exportMode === 'zip' ? '导出 ZIP' : exportMode === 'image_package' ? '导出图片包' : exportMode === 'jianying' ? '导出剪映' : '导出 Markdown' }}</span>
          </button>
          <button type="button" class="secondary-action" @click="goTo('/works')">
            <Library :size="16" />
            <span>作品库</span>
          </button>
        </div>
        <div v-if="compositions.length" class="attempt-list">
          <div v-for="composition in compositions" :key="composition.id" class="attempt-row">
            <span :class="['status-badge', composition.status]">{{ statusText(composition.status) }}</span>
            <small>#{{ composition.id }}</small>
            <small>{{ composition.status === 'succeeded' ? '合成完成' : composition.error_message || '等待合成结果' }} {{ composition.output_url }}</small>
          </div>
        </div>
        <pre v-if="exportOutput" class="export-output" data-testid="novel-export-output">{{ exportOutput }}</pre>
      </section>
    </main>

    <aside class="novel-studio-inspector" :class="{ open: inspectorOpen }">
      <button type="button" class="mobile-close" @click="inspectorOpen = false"><X :size="16" /></button>
      <template v-if="inspectorMode === 'creature' && selectedCreature">
        <div class="inspector-head" data-testid="creature-inspector">
          <span>Creature Inspector</span>
          <h2>{{ selectedCreature.name }}</h2>
          <small>{{ selectedCreature.creature_type }} · {{ statusText(selectedCreature.review_status) }}</small>
        </div>
        <img v-if="selectedCreature.work_preview_url || selectedCreature.asset_url" class="inspector-media" :src="selectedCreature.work_preview_url || selectedCreature.asset_url" :alt="selectedCreature.name" />
        <label class="studio-field"><span>外形</span><textarea v-model="selectedCreature.appearance" rows="4" /></label>
        <label class="studio-field"><span>能力 / 习性</span><textarea v-model="selectedCreature.abilities" rows="4" /></label>
        <label class="studio-field"><span>一致性 Prompt</span><textarea v-model="selectedCreature.visual_consistency_prompt" rows="5" /></label>
        <p v-if="busyAction === `creature-image-${selectedCreature.id}`" class="generation-hint">正在调用模型，通常需要几十秒</p>
        <div class="inspector-actions">
          <button type="button" class="primary-action compact" @click="approveCreature(selectedCreature)"><Check :size="15" />批准</button>
          <button type="button" class="gold-action" data-testid="creature-inspector-generate" :disabled="busyAction === `creature-image-${selectedCreature.id}`" @click="generateCreatureImage(selectedCreature)"><RefreshCw :size="15" />{{ busyAction === `creature-image-${selectedCreature.id}` ? '生成中...' : '生成/重试' }}</button>
        </div>
      </template>

      <template v-else-if="inspectorMode === 'asset' && selectedAsset">
        <div class="inspector-head" data-testid="asset-inspector">
          <span>Asset Inspector</span>
          <h2>{{ selectedAsset.name }}</h2>
          <small>{{ selectedAsset.kind }} · v{{ selectedAsset.version || 1 }} · {{ statusText(selectedAsset.review_status) }}</small>
        </div>
        <img v-if="selectedAsset.asset_url || selectedAsset.reference_url" class="inspector-media" :src="selectedAsset.asset_url || selectedAsset.reference_url" :alt="selectedAsset.name" />
        <div v-else class="inspector-media asset-placeholder">
          <LoaderCircle v-if="selectedAssetJob && ['queued', 'running'].includes(selectedAssetJob.status)" :size="28" />
          <CircleAlert v-else-if="selectedAsset.error_message" :size="28" />
          <Image v-else :size="30" />
          <span>{{ selectedAssetJob ? statusText(selectedAssetJob.status) : selectedAsset.error_message ? '生成失败' : '暂无预览' }}</span>
        </div>
        <div class="shot-meta">
          <div><span>审核</span><strong>{{ statusText(selectedAsset.review_status) }}</strong></div>
          <div><span>任务</span><strong>{{ selectedAssetJob ? statusText(selectedAssetJob.status) : '无活跃任务' }}</strong></div>
          <div><span>引用</span><strong>{{ selectedAssetReferences.length }}</strong></div>
        </div>
        <label class="studio-field"><span>描述</span><textarea v-model="selectedAsset.description" rows="3" /></label>
        <label class="studio-field"><span>Prompt</span><textarea v-model="selectedAsset.prompt" data-testid="asset-inspector-prompt" rows="6" /></label>
        <div class="asset-detail-list">
          <div><span>metadata.source</span><strong>{{ selectedAsset.metadata?.source || '-' }}</strong></div>
          <div><span>content_mode</span><strong>{{ selectedAsset.metadata?.content_mode || contentMode }}</strong></div>
          <div><span>error_code</span><strong>{{ selectedAsset.error_code || '-' }}</strong></div>
          <div><span>error_message</span><strong>{{ selectedAsset.error_message || '-' }}</strong></div>
        </div>
        <div class="attempt-list">
          <h3>引用镜头</h3>
          <button v-for="shot in selectedAssetReferences" :key="shot.id" type="button" class="attempt-row asset-reference-row" @click="selectShot(shot)">
            <span>{{ shot.episode?.number }}-{{ shot.number }}</span>
            <strong>{{ shot.title }}</strong>
            <small>{{ statusText(shot.status) }}</small>
          </button>
          <p v-if="!selectedAssetReferences.length" class="empty-line">暂无镜头引用。</p>
        </div>
        <div class="inspector-actions sticky">
          <button type="button" class="primary-action compact" @click="updateAssetReview(selectedAsset, 'approved')"><Check :size="15" />审核通过</button>
          <button type="button" class="secondary-action compact" @click="updateAssetReview(selectedAsset, 'needs_review')"><Edit3 :size="15" />退回待审</button>
          <button type="button" class="gold-action" @click="retryAssetImage(selectedAsset)"><RefreshCw :size="15" />重试生成</button>
          <button type="button" class="secondary-action compact" :disabled="assetDeleteDisabled(selectedAsset)" :title="assetDeleteTitle(selectedAsset)" @click="deleteAsset(selectedAsset)"><Trash2 :size="15" />{{ assetHasQueuedJob(selectedAsset) ? '解除排队并删除资产' : '删除资产' }}</button>
        </div>
      </template>

      <template v-else-if="inspectorMode === 'shot' && selectedShot">
        <div class="inspector-head" data-testid="shot-inspector">
          <span>Shot Inspector</span>
          <h2>{{ selectedShot.title }}</h2>
          <small>镜头 {{ selectedShot.episode?.number }}-{{ selectedShot.number }} · {{ statusText(selectedShot.status) }}</small>
        </div>
        <label class="studio-field"><span>镜头标题</span><input v-model="selectedShot.title" type="text" /></label>
        <div class="shot-meta">
          <div><span>预计扣点</span><strong>{{ selectedShot.estimated_credits ?? 0 }}</strong></div>
          <div><span>时长</span><strong>{{ selectedShot.duration_seconds || duration }}s</strong></div>
          <div><span>画幅</span><strong>{{ aspectRatio }}</strong></div>
        </div>
        <div class="structured-shot-fields" data-testid="structured-shot-fields">
          <label class="studio-field"><span>剧本单元</span><input v-model="selectedShot.script_unit_type" type="text" /></label>
          <label class="studio-field"><span>时长</span><input v-model.number="selectedShot.duration_seconds" min="1" type="number" /></label>
          <label class="studio-field full"><span>剧本文本</span><textarea v-model="selectedShot.source_excerpt" rows="3" /></label>
          <label class="studio-field full"><span>图片提示词</span><textarea v-model="selectedShot.image_prompt" rows="3" /></label>
          <label class="studio-field full"><span>视频提示词</span><textarea v-model="selectedShot.video_prompt" data-testid="shot-video-prompt" rows="4" /></label>
          <label class="studio-field full"><span>旁白</span><textarea v-model="selectedShot.voiceover_text" rows="3" /></label>
          <label class="studio-field full"><span>引用资产</span><textarea v-model="selectedShot.asset_refs_json" rows="4" spellcheck="false" /></label>
        </div>
        <div class="prompt-editor" data-testid="prompt-editor">
          <div class="editor-toolbar">
            <span>Prompt</span>
            <button type="button" @click="copyPrompt"><Copy :size="14" /></button>
          </div>
          <div class="editor-body">
            <div class="line-numbers"><span v-for="line in promptLineNumbers" :key="line">{{ line }}</span></div>
            <textarea v-model="selectedShot.prompt" rows="10" />
          </div>
          <div class="editor-actions">
            <button type="button" @click="toast('模板库稍后开放')">模板</button>
            <button type="button" @click="toast('Prompt 优化稍后开放')">优化</button>
            <button type="button" @click="copyPrompt">复制</button>
            <button type="button" @click="clearPrompt">清空</button>
            <button type="button" data-testid="shot-save-structured" @click="saveShot(selectedShot)">保存</button>
          </div>
        </div>
        <div class="reference-strip">
          <button type="button" class="reference-add" @click="openAssets"><Plus :size="16" /></button>
          <img v-for="creature in selectedShotReferences" :key="creature.id" :src="creature.work_preview_url" :alt="creature.name" />
        </div>
        <div class="attempt-list">
          <h3>生成记录</h3>
          <div v-for="attempt in selectedShot.generation_attempts ?? []" :key="attempt.id || attempt.generation_record_id" class="attempt-row">
            <span :class="['status-badge', attempt.status]">{{ statusText(attempt.status) }}</span>
            <strong>{{ attempt.progress ?? 0 }}%</strong>
            <small>{{ attempt.error_message }}</small>
          </div>
          <p v-if="!(selectedShot.generation_attempts ?? []).length" class="empty-line">暂无生成记录。</p>
        </div>
        <div class="inspector-actions sticky">
          <button type="button" class="primary-action compact" @click="renderShots"><Play :size="15" />继续生成</button>
          <button type="button" class="gold-action" @click="renderShots"><RefreshCw :size="15" />重试</button>
          <button type="button" class="secondary-action compact" @click="saveShot(selectedShot)"><Edit3 :size="15" />编辑镜头</button>
        </div>
      </template>

      <template v-else>
        <div class="inspector-head">
          <span>Project Inspector</span>
          <h2>{{ project?.title || title || '未创建项目' }}</h2>
          <small>{{ projectStatusText }}</small>
        </div>
        <div class="project-summary">
          <div><span>小说字数</span><strong>{{ sourceChars.toLocaleString() }}</strong></div>
          <div><span>生物设定</span><strong>{{ creatures.length }}</strong></div>
          <div><span>分集</span><strong>{{ episodes.length }}</strong></div>
          <div><span>镜头</span><strong>{{ flatShots.length }}</strong></div>
        </div>
        <button v-if="!project?.id" type="button" class="secondary-action" data-testid="novel-inspector-history-open" @click="openProjectHistory">
          <History :size="16" />
          打开历史记录
        </button>
        <button type="button" class="primary-action" :disabled="!project?.id" @click="saveStoryBible"><Save :size="16" />保存故事圣经</button>
        <div v-if="message" class="studio-message" role="status">{{ message }}</div>
        <div v-if="errorMessage" class="studio-error" role="alert">{{ errorMessage }}</div>
      </template>
    </aside>

    <div v-if="historyOpen" class="history-backdrop" data-testid="novel-history-panel" @click.self="historyOpen = false">
      <section class="history-drawer" aria-label="历史小说项目">
        <header class="history-drawer-head">
          <div>
            <span>History</span>
            <h2>历史小说项目</h2>
          </div>
          <button type="button" class="icon-button" aria-label="关闭历史记录" @click="historyOpen = false">
            <X :size="16" />
          </button>
        </header>

        <div v-if="newLocalDraft()" class="history-local-draft">
          <span>本机草稿</span>
          <button type="button" data-testid="novel-history-draft-new" @click="selectNewDraft">
            <strong>{{ newLocalDraft().title || '未创建项目草稿' }}</strong>
            <small>{{ newLocalDraft().source_text?.length ?? 0 }} 字 · 仅保存在本机</small>
          </button>
        </div>

        <div v-if="historyLoading" class="history-state">
          <LoaderCircle :size="16" class="workflow-icon" />
          <span>正在加载历史记录...</span>
        </div>
        <div v-else-if="historyError" class="studio-error">{{ historyError }}</div>
        <div v-else-if="!projectHistory.length && !newLocalDraft()" class="history-state">
          <span>暂无历史项目</span>
        </div>
        <div v-else class="history-list">
          <button
            v-for="item in projectHistory"
            :key="item.id"
            type="button"
            class="history-project"
            :class="{ active: project?.id === item.id }"
            :data-testid="`novel-history-project-${item.id}`"
            @click="selectHistoryProject(item)"
          >
            <span class="history-project-main">
              <strong>{{ item.title || `项目 #${item.id}` }}</strong>
              <small>{{ statusText(item.status) }} · {{ formatHistoryDate(item.updated_at || item.created_at) }}</small>
            </span>
            <span class="history-project-meta">
              <i v-if="project?.id === item.id">当前</i>
              <i v-if="projectHasLocalDraft(item.id)">本机草稿</i>
              <small>{{ (item.source_chars ?? item.source_text?.length ?? 0).toLocaleString() }} 字</small>
            </span>
          </button>
        </div>
      </section>
    </div>
  </section>
</template>

<style scoped>
.novel-studio-shell {
  --nv-bg: #070b10;
  --nv-sidebar: #0b1016;
  --nv-panel: #10161d;
  --nv-panel-2: #0d131a;
  --nv-border: rgba(148, 163, 184, 0.22);
  --nv-shell-border: rgba(148, 163, 184, 0.16);
  --nv-text: #e5edf6;
  --nv-muted: #8c9aaa;
  --nv-gold: #b17921;
  --nv-blue: #2f6de0;
  --nv-green: #19a77c;
  --nv-red: #db4f4f;
  --nv-card: #101720;
  --nv-input: #080d13;
  --nv-button: #111923;
  --nv-row-button: #0b1118;
  --nv-icon-bg: rgba(255, 255, 255, 0.04);
  --nv-hover: rgba(255, 255, 255, 0.06);
  --nv-active-bg: rgba(47, 109, 224, 0.14);
  --nv-active-border: rgba(47, 109, 224, 0.85);
  --nv-step-bg: #151c25;
  --nv-row-border: rgba(148, 163, 184, 0.14);
  --nv-progress-track: #070b10;
  --nv-code-text: #dce7f3;
  --nv-muted-code-text: #b6c5d6;
  --nv-line-number-text: #566170;
  --nv-sticky-actions: rgba(11, 16, 22, 0.96);
  --nv-badge-text: #b9c5d2;
  --nv-badge-bg: rgba(148, 163, 184, 0.12);
  --nv-badge-border: rgba(148, 163, 184, 0.18);
  --nv-success-text: #a7f3d0;
  --nv-running-text: #bfdbfe;
  --nv-error-text: #fecaca;
  --nv-success-message-text: #bbf7d0;
  display: grid;
  grid-template-columns: 216px minmax(0, 1fr) 344px;
  min-height: calc(100vh - 32px);
  overflow: hidden;
  color: var(--nv-text);
  background: var(--nv-bg);
  border: 1px solid var(--nv-shell-border);
  border-radius: 7px;
  font-variant-numeric: tabular-nums;
}

.novel-studio-shell[data-theme="light"] {
  --nv-bg: #eef4fb;
  --nv-sidebar: #f8fafc;
  --nv-panel: #ffffff;
  --nv-panel-2: #f6f8fc;
  --nv-border: rgba(100, 116, 139, 0.24);
  --nv-shell-border: rgba(100, 116, 139, 0.18);
  --nv-text: #172033;
  --nv-muted: #64748b;
  --nv-gold: #b7791f;
  --nv-blue: #2563eb;
  --nv-green: #0f9f76;
  --nv-red: #dc2626;
  --nv-card: #f1f5f9;
  --nv-input: #ffffff;
  --nv-button: #f2f6fb;
  --nv-row-button: #f8fafc;
  --nv-icon-bg: rgba(37, 99, 235, 0.06);
  --nv-hover: rgba(37, 99, 235, 0.08);
  --nv-active-bg: rgba(37, 99, 235, 0.1);
  --nv-active-border: rgba(37, 99, 235, 0.72);
  --nv-step-bg: #e8eef7;
  --nv-row-border: rgba(100, 116, 139, 0.16);
  --nv-progress-track: #dbe7f3;
  --nv-code-text: #334155;
  --nv-muted-code-text: #475569;
  --nv-line-number-text: #94a3b8;
  --nv-sticky-actions: rgba(248, 250, 252, 0.96);
  --nv-badge-text: #475569;
  --nv-badge-bg: rgba(100, 116, 139, 0.1);
  --nv-badge-border: rgba(100, 116, 139, 0.2);
  --nv-success-text: #047857;
  --nv-running-text: #1d4ed8;
  --nv-error-text: #b91c1c;
  --nv-success-message-text: #047857;
}

button,
input,
textarea,
select {
  font: inherit;
}

button {
  cursor: pointer;
}

.novel-studio-sidebar,
.novel-studio-inspector {
  background: var(--nv-sidebar);
}

.novel-studio-sidebar {
  display: flex;
  flex-direction: column;
  gap: 14px;
  min-width: 0;
  padding: 14px;
  border-right: 1px solid var(--nv-border);
}

.novel-studio-sidebar.collapsed {
  align-items: center;
}

.studio-brand,
.section-header,
.card-title-row,
.row-actions,
.section-actions,
.editor-toolbar,
.editor-actions,
.inspector-actions {
  display: flex;
  align-items: center;
}

.studio-brand {
  justify-content: space-between;
  gap: 8px;
}

.studio-brand-actions {
  display: flex;
  align-items: center;
  gap: 6px;
}

.studio-kicker,
.section-header span,
.inspector-head span {
  color: var(--nv-gold);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0;
  text-transform: uppercase;
}

.studio-brand h1,
.section-header h2,
.inspector-head h2 {
  margin: 3px 0 0;
  font-size: 18px;
  font-weight: 600;
  letter-spacing: 0;
}

.icon-button,
.segmented button,
.workflow-step,
.ghost-button,
.secondary-action,
.primary-action,
.gold-action,
.row-actions button,
.editor-actions button,
.queue-toggle,
.queue-page-button,
.shot-table button {
  border-radius: 5px;
}

.icon-button {
  display: inline-grid;
  place-items: center;
  width: 30px;
  height: 30px;
  color: var(--nv-muted);
  background: var(--nv-icon-bg);
  border: 1px solid var(--nv-border);
}

.project-card {
  display: grid;
  gap: 4px;
  padding: 11px;
  background: var(--nv-card);
  border: 1px solid var(--nv-border);
  border-radius: 7px;
}

.project-card span,
.project-card small,
.workflow-copy small,
.studio-field span,
.shot-meta span,
.project-summary span,
.render-metrics span,
.render-now span,
.export-preview-grid span {
  color: var(--nv-muted);
  font-size: 11px;
}

.project-card strong {
  min-width: 0;
  overflow: hidden;
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.local-draft-notice {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  background: rgba(177, 121, 33, 0.14);
  border: 1px solid rgba(177, 121, 33, 0.45);
  border-radius: 7px;
}

.local-draft-notice div:first-child {
  display: grid;
  gap: 3px;
}

.local-draft-notice strong {
  font-size: 13px;
}

.local-draft-notice span {
  color: var(--nv-muted);
  font-size: 12px;
}

.local-draft-actions {
  display: inline-flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}

.history-backdrop {
  position: fixed;
  inset: 0;
  z-index: 20;
  display: grid;
  justify-content: end;
  background: rgba(3, 7, 12, 0.58);
}

.history-drawer {
  display: grid;
  align-content: start;
  gap: 12px;
  width: min(440px, 100vw);
  height: 100vh;
  padding: 16px;
  overflow: auto;
  color: var(--nv-text);
  background: var(--nv-panel);
  border-left: 1px solid var(--nv-border);
  box-shadow: -18px 0 42px rgba(0, 0, 0, 0.32);
}

.history-drawer-head,
.history-project {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.history-drawer-head span,
.history-local-draft > span,
.history-project small,
.history-state {
  color: var(--nv-muted);
  font-size: 12px;
}

.history-drawer-head h2 {
  margin: 2px 0 0;
  font-size: 18px;
}

.history-local-draft,
.history-list {
  display: grid;
  gap: 8px;
}

.history-local-draft button,
.history-project {
  width: 100%;
  min-width: 0;
  padding: 10px;
  color: var(--nv-text);
  background: var(--nv-panel-2);
  border: 1px solid var(--nv-border);
  border-radius: 7px;
  text-align: left;
}

.history-local-draft button {
  display: grid;
  gap: 4px;
}

.history-project.active {
  border-color: var(--nv-active-border);
  background: var(--nv-active-bg);
}

.history-project-main,
.history-project-meta {
  display: grid;
  gap: 4px;
  min-width: 0;
}

.history-project-main strong {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.history-project-meta {
  justify-items: end;
  flex: 0 0 auto;
}

.history-project-meta i {
  width: fit-content;
  padding: 2px 6px;
  color: var(--nv-running-text);
  background: rgba(47, 109, 224, 0.14);
  border: 1px solid rgba(47, 109, 224, 0.35);
  border-radius: 999px;
  font-size: 11px;
  font-style: normal;
}

.history-state {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-height: 42px;
}

.workflow-nav,
.creature-grid,
.settings-grid,
.bible-grid,
.attempt-list {
  display: grid;
  gap: 8px;
}

.workflow-step {
  display: grid;
  grid-template-columns: 28px minmax(0, 1fr) 18px;
  align-items: center;
  gap: 8px;
  width: 100%;
  min-height: 42px;
  padding: 7px;
  color: var(--nv-text);
  background: transparent;
  border: 1px solid transparent;
  text-align: left;
}

.workflow-step.active {
  background: var(--nv-active-bg);
  border-color: var(--nv-active-border);
}

.workflow-number {
  display: grid;
  place-items: center;
  width: 26px;
  height: 26px;
  color: var(--nv-muted);
  background: var(--nv-step-bg);
  border: 1px solid var(--nv-border);
  border-radius: 50%;
  font-size: 12px;
}

.workflow-step.done .workflow-number,
.workflow-step.running .workflow-number {
  color: white;
  background: var(--nv-blue);
  border-color: var(--nv-blue);
}

.workflow-copy {
  display: grid;
  min-width: 0;
}

.workflow-copy strong {
  font-size: 13px;
}

.workflow-icon {
  color: var(--nv-muted);
}

.workflow-step.running .workflow-icon {
  color: var(--nv-blue);
  animation: spin 1s linear infinite;
}

.workflow-step.failed .workflow-icon {
  color: var(--nv-red);
}

.row-actions button:hover,
.editor-actions button:hover {
  color: var(--nv-text);
  background: var(--nv-hover);
}

.sidebar-collapse-button svg {
  transition: transform 0.18s ease;
}

.novel-studio-sidebar.collapsed .sidebar-collapse-button svg {
  transform: rotate(180deg);
}

.novel-studio-main {
  display: grid;
  align-content: start;
  gap: 12px;
  min-width: 0;
  max-height: calc(100vh - 34px);
  overflow: auto;
  padding: 12px;
  background: var(--nv-bg);
}

.studio-section {
  display: grid;
  gap: 12px;
  padding: 12px;
  background: var(--nv-panel);
  border: 1px solid var(--nv-border);
  border-radius: 7px;
}

.section-header {
  justify-content: space-between;
  gap: 10px;
}

.section-header-actions {
  display: inline-flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}

.section-header h2 {
  font-size: 15px;
}

.import-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(280px, 38%);
  gap: 12px;
}

.studio-field {
  display: grid;
  gap: 6px;
}

.studio-field.full {
  grid-column: 1 / -1;
}

.studio-toggle {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-height: 34px;
  padding: 0 10px;
  color: var(--nv-text);
  background: var(--nv-input);
  border: 1px solid var(--nv-border);
  border-radius: 5px;
}

.studio-toggle input {
  width: 15px;
  height: 15px;
}

.studio-field input,
.studio-field textarea,
.studio-field select,
.bible-card textarea,
.source-field textarea {
  width: 100%;
  min-width: 0;
  color: var(--nv-text);
  background: var(--nv-input);
  border: 1px solid var(--nv-border);
  border-radius: 5px;
  outline: none;
}

.studio-field input,
.studio-field select {
  height: 34px;
  padding: 0 10px;
}

.studio-field textarea,
.bible-card textarea,
.source-field textarea {
  resize: vertical;
  padding: 9px 10px;
  line-height: 1.55;
}

.source-field textarea {
  min-height: 210px;
}

.source-field small {
  justify-self: end;
  color: var(--nv-muted);
  font-size: 12px;
}

.source-field small.warn {
  color: var(--nv-red);
}

.settings-panel {
  display: grid;
  align-content: start;
  gap: 10px;
  padding: 10px;
  background: var(--nv-panel-2);
  border: 1px solid var(--nv-border);
  border-radius: 7px;
}

.settings-toggle,
.queue-toggle {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  color: var(--nv-muted);
  background: transparent;
  border: 0;
}

.queue-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.queue-summary {
  color: var(--nv-muted);
  font-size: 12px;
}

.queue-pagination {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 8px;
  margin-top: 10px;
}

.queue-page-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-height: 30px;
  padding: 0 10px;
  color: var(--nv-text);
  border: 1px solid var(--nv-border);
  background: var(--nv-button);
}

.settings-grid {
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 160px), 1fr));
  column-gap: 12px;
  row-gap: 10px;
}

.settings-grid > * {
  min-width: 0;
}

.settings-grid .studio-field.full {
  grid-column: 1 / -1;
}

.settings-grid :deep(.click-select-trigger) {
  min-height: 40px;
  padding: 10px 12px;
  gap: 8px;
}

.primary-action,
.secondary-action,
.gold-action,
.ghost-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  min-height: 34px;
  padding: 0 12px;
  color: var(--nv-text);
  border: 1px solid var(--nv-border);
}

.primary-action {
  background: var(--nv-blue);
  border-color: var(--nv-blue);
}

.primary-action.compact,
.secondary-action.compact {
  min-height: 30px;
  padding: 0 10px;
}

.secondary-action,
.ghost-button {
  background: var(--nv-button);
}

.gold-action {
  background: rgba(177, 121, 33, 0.18);
  border-color: rgba(177, 121, 33, 0.7);
}

.primary-action:disabled,
.secondary-action:disabled,
.ghost-button:disabled,
.queue-page-button:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.section-actions {
  flex-wrap: wrap;
  gap: 8px;
}

.bible-grid {
  grid-template-columns: repeat(5, minmax(0, 1fr));
}

.bible-card {
  display: grid;
  gap: 7px;
  padding: 9px;
  background: var(--nv-panel-2);
  border: 1px solid var(--nv-border);
  border-radius: 7px;
}

.bible-card span {
  color: var(--nv-muted);
  font-size: 11px;
}

.bible-card textarea {
  min-height: 86px;
  font-size: 12px;
}

.segmented {
  display: inline-flex;
  gap: 3px;
  padding: 3px;
  background: var(--nv-input);
  border: 1px solid var(--nv-border);
  border-radius: 6px;
}

.segmented button {
  display: inline-grid;
  place-items: center;
  min-width: 28px;
  height: 25px;
  color: var(--nv-muted);
  background: transparent;
  border: 0;
}

.segmented.text button {
  padding: 0 8px;
  font-size: 12px;
}

.segmented button.active {
  color: white;
  background: var(--nv-blue);
}

.creature-grid {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.creature-grid.list {
  grid-template-columns: 1fr;
}

.creature-card {
  position: relative;
  display: grid;
  grid-template-columns: 148px minmax(0, 1fr);
  gap: 10px;
  padding: 9px;
  background: var(--nv-panel-2);
  border: 1px solid var(--nv-border);
  border-radius: 7px;
}

.creature-card.selected {
  border-color: var(--nv-active-border);
}

.actor-card {
  grid-template-columns: 1fr;
  grid-template-rows: minmax(0, 1fr) auto;
  gap: 10px;
}

.creature-grid:not(.list) .actor-card {
  height: 320px;
  overflow: hidden;
}

.creature-grid.list .actor-card {
  height: auto;
  overflow: visible;
}

.creature-card-main {
  display: grid;
  grid-template-columns: 148px minmax(0, 1fr);
  align-items: start;
  gap: 10px;
  min-height: 0;
  overflow: hidden;
}

.creature-summary {
  align-content: start;
  min-height: 0;
  overflow: hidden;
}

.creature-summary p {
  display: -webkit-box;
  overflow: hidden;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

.creature-card-footer {
  display: grid;
  gap: 6px;
  min-width: 0;
}

.creature-consistency-prompt {
  display: block;
  overflow: hidden;
  color: var(--nv-muted-code-text);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.creature-reference-status {
  color: var(--nv-muted);
  font-size: 12px;
}

.creature-card-actions {
  width: 100%;
  align-content: start;
}

.asset-card {
  cursor: pointer;
}

.asset-card-delete {
  position: absolute;
  top: 8px;
  right: 8px;
  z-index: 2;
  background: var(--nv-panel);
}

.asset-card-delete:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.creature-image {
  position: relative;
  display: grid;
  place-items: center;
  aspect-ratio: 4 / 3;
  color: var(--nv-muted);
  background: var(--nv-input);
  border: 1px solid var(--nv-border);
  border-radius: 6px;
  overflow: hidden;
}

.generation-status-strip {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 34px;
  padding: 8px 10px;
  margin-bottom: 10px;
  color: var(--nv-text);
  background: rgba(47, 109, 224, 0.16);
  border: 1px solid rgba(47, 109, 224, 0.45);
  border-radius: 6px;
}

.generation-overlay {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  align-content: center;
  gap: 7px;
  padding: 10px;
  color: white;
  text-align: center;
  background: rgba(5, 10, 18, 0.76);
}

.generation-overlay strong,
.generation-overlay small {
  max-width: 100%;
  overflow-wrap: anywhere;
}

.generation-overlay.failed {
  background: rgba(72, 16, 20, 0.84);
}

.generation-overlay button {
  min-height: 26px;
  padding: 0 10px;
  color: white;
  background: rgba(255, 255, 255, 0.12);
  border: 1px solid rgba(255, 255, 255, 0.35);
}

.generation-hint {
  margin: 0;
  color: var(--nv-muted);
  font-size: 12px;
}

.creature-image img,
.inspector-media,
.reference-strip img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.creature-body {
  display: grid;
  gap: 6px;
  min-width: 0;
}

.creature-body h3 {
  margin: 0;
  font-size: 14px;
}

.creature-body small,
.creature-body p {
  margin: 0;
  color: var(--nv-muted);
  font-size: 12px;
  line-height: 1.45;
}

.creature-body code {
  overflow: hidden;
  color: var(--nv-muted-code-text);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.status-badge {
  display: inline-flex;
  align-items: center;
  width: fit-content;
  min-height: 20px;
  padding: 0 7px;
  color: var(--nv-badge-text);
  background: var(--nv-badge-bg);
  border: 1px solid var(--nv-badge-border);
  border-radius: 999px;
  font-size: 11px;
}

.status-badge.approved,
.status-badge.succeeded {
  color: var(--nv-success-text);
  border-color: rgba(25, 167, 124, 0.45);
}

.status-badge.running,
.status-badge.queued {
  color: var(--nv-running-text);
  border-color: rgba(47, 109, 224, 0.55);
}

.status-badge.failed {
  color: var(--nv-error-text);
  border-color: rgba(219, 79, 79, 0.55);
}

.row-actions {
  flex-wrap: wrap;
  gap: 6px;
}

.inline-number {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--nv-muted);
  font-size: 12px;
}

.inline-number input {
  width: 58px;
  min-height: 31px;
  padding: 0 8px;
  color: var(--nv-text);
  background: var(--nv-input);
  border: 1px solid var(--nv-border);
  border-radius: 5px;
}

.row-actions button,
.shot-table button,
.editor-actions button {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  min-height: 26px;
  padding: 0 7px;
  color: var(--nv-muted);
  background: var(--nv-row-button);
  border: 1px solid var(--nv-border);
  font-size: 12px;
}

.episode-card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  gap: 10px;
}

.episode-entry-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-height: 72px;
  padding: 14px;
  color: var(--nv-text);
  background: var(--nv-panel-2);
  border: 1px solid var(--nv-border);
  border-radius: 8px;
  text-align: left;
  cursor: pointer;
}

.episode-entry-card:hover,
.episode-entry-card:focus-visible {
  border-color: var(--nv-blue);
}

.episode-modal-backdrop {
  position: fixed;
  inset: 0;
  z-index: 30;
  display: grid;
  place-items: center;
  padding: 24px;
  background: rgba(3, 7, 12, 0.72);
}

.episode-shots-modal {
  display: grid;
  grid-template-rows: auto minmax(0, 1fr);
  width: min(1180px, 96vw);
  max-height: 90vh;
  overflow: hidden;
  color: var(--nv-text);
  background: var(--nv-panel);
  border: 1px solid var(--nv-border);
  border-radius: 12px;
  box-shadow: 0 24px 70px rgba(0, 0, 0, 0.42);
}

.episode-modal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 16px 18px;
  border-bottom: 1px solid var(--nv-border);
}

.episode-modal-header span {
  color: var(--nv-muted);
  font-size: 12px;
}

.episode-modal-header h2 {
  margin: 3px 0 0;
  font-size: 20px;
}

.episode-modal-content {
  padding: 16px 18px 18px;
  overflow: auto;
}

.episode-pagination {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 12px;
  color: var(--nv-muted);
  font-size: 12px;
}

.episode-pagination label,
.episode-pagination button {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.episode-pagination select,
.episode-pagination button {
  min-height: 32px;
  color: var(--nv-text);
  background: var(--nv-button);
  border: 1px solid var(--nv-border);
  border-radius: 6px;
}

.episode-pagination select {
  padding: 0 8px;
}

.episode-pagination button {
  padding: 0 10px;
}

.episode-pagination button:disabled {
  opacity: 0.45;
}

.shot-table-wrap {
  min-width: 0;
  overflow: auto;
}

.shot-table,
.queue-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}

.shot-table th,
.shot-table td,
.queue-table td {
  padding: 9px 8px;
  border-bottom: 1px solid var(--nv-row-border);
  text-align: left;
  vertical-align: top;
}

.shot-table th {
  color: var(--nv-muted);
  font-weight: 600;
}

.shot-table tr.selected {
  outline: 1px solid var(--nv-active-border);
  outline-offset: -1px;
}

.shot-table td:nth-child(3) {
  max-width: 320px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.production-table {
  table-layout: fixed;
}

.episode-shots-table {
  min-width: 1170px;
  table-layout: fixed;
}

.shot-prompt-clamp {
  display: -webkit-box;
  max-width: 100%;
  overflow: hidden;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow-wrap: anywhere;
  white-space: normal;
  line-height: 1.45;
}

.shot-status-cell {
  padding-right: 6px;
  padding-left: 6px;
}

.shot-status-cell .progress-track {
  width: 100%;
}

.shot-actions-cell {
  min-width: 300px;
}

.shot-row-actions {
  display: flex;
  flex-wrap: nowrap;
  align-items: center;
  gap: 6px;
  white-space: nowrap;
}

.shot-row-actions button {
  flex: 0 0 auto;
  padding-right: 7px;
  padding-left: 7px;
  white-space: nowrap;
}

.shot-candidate-count {
  flex: 0 0 auto;
  min-width: 12px;
}

.shot-image-thumb {
  display: grid;
  place-items: center;
  width: 104px;
  height: 58px;
  margin-top: 6px;
  overflow: hidden;
  color: var(--nv-muted);
  background: var(--nv-input);
  border: 1px solid var(--nv-border);
  border-radius: 6px;
}

.shot-image-thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.selected-pill {
  display: inline-flex;
  margin-left: 6px;
  padding: 2px 6px;
  color: var(--nv-green);
  background: color-mix(in srgb, var(--nv-green) 14%, transparent);
  border: 1px solid color-mix(in srgb, var(--nv-green) 34%, transparent);
  border-radius: 999px;
  font-size: 11px;
}

.progress-track.compact {
  height: 5px;
  margin: 6px 0 3px;
}

.error-inline {
  display: block;
  color: var(--nv-red);
}

.render-board {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 420px;
  gap: 12px;
}

.image-batch-summary {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 8px;
  margin-bottom: 12px;
}

.image-batch-summary > div,
.shot-image-card {
  border: 1px solid var(--nv-border);
  background: var(--nv-panel-2);
  border-radius: 7px;
}

.image-batch-summary > div {
  padding: 12px;
}

.image-batch-summary span,
.shot-image-body small {
  color: var(--nv-muted);
  font-size: 12px;
}

.image-batch-summary strong {
  display: block;
  margin-top: 4px;
  font-size: 18px;
}

.shot-image-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
  margin-bottom: 12px;
}

.shot-image-card {
  display: grid;
  grid-template-columns: 132px minmax(0, 1fr);
  overflow: hidden;
}

.shot-image-card.selected {
  border-color: var(--nv-green);
}

.shot-image-preview {
  display: grid;
  place-items: center;
  min-height: 132px;
  color: var(--nv-muted);
  background: var(--nv-input);
}

.shot-image-preview img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.shot-image-body {
  min-width: 0;
  padding: 10px;
}

.shot-image-body h3,
.shot-image-body p {
  margin: 0;
}

.shot-image-body p {
  margin: 8px 0;
  color: var(--nv-muted);
  font-size: 12px;
  line-height: 1.5;
}

.render-now,
.render-metrics,
.export-preview-grid,
.project-summary,
.shot-meta {
  display: grid;
  gap: 8px;
}

.render-now,
.render-metrics div,
.export-preview-grid article,
.project-summary div,
.shot-meta div {
  padding: 10px;
  background: var(--nv-panel-2);
  border: 1px solid var(--nv-border);
  border-radius: 7px;
}

.render-now strong {
  font-size: 15px;
}

.render-metrics {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.render-metrics strong,
.export-preview-grid strong,
.project-summary strong,
.shot-meta strong {
  display: block;
  margin-top: 4px;
  font-size: 18px;
}

.progress-track {
  height: 8px;
  overflow: hidden;
  background: var(--nv-progress-track);
  border-radius: 999px;
}

.progress-track i {
  display: block;
  height: 100%;
  background: linear-gradient(90deg, var(--nv-blue), var(--nv-green));
}

.preflight-list {
  display: grid;
  gap: 8px;
}

.preflight-row {
  display: grid;
  grid-template-columns: 56px minmax(0, 1fr) minmax(220px, 1.4fr);
  gap: 8px;
  align-items: center;
  padding: 9px 10px;
  background: var(--nv-panel-2);
  border: 1px solid var(--nv-border);
  border-radius: 7px;
  font-size: 12px;
}

.preflight-row span,
.preflight-row small {
  color: var(--nv-muted);
}

.preflight-row strong,
.preflight-row small {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.export-preview-grid {
  grid-template-columns: repeat(5, minmax(0, 1fr));
}

.structured-shot-fields {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
  margin: 12px 0;
}

.structured-shot-fields .full {
  grid-column: 1 / -1;
}

.cost-estimate {
  display: grid;
  gap: 4px;
  margin-top: 12px;
  padding: 10px;
  border: 1px solid var(--nv-border);
  border-radius: 8px;
  background: var(--nv-card);
}

.export-preview-grid article {
  align-content: start;
  min-height: 92px;
}

.export-output {
  max-height: 280px;
  overflow: auto;
  padding: 10px;
  color: var(--nv-code-text);
  background: var(--nv-input);
  border: 1px solid var(--nv-border);
  border-radius: 7px;
  white-space: pre-wrap;
}

.novel-studio-inspector {
  position: relative;
  display: grid;
  align-content: start;
  gap: 12px;
  min-width: 0;
  max-height: calc(100vh - 34px);
  overflow: auto;
  padding: 14px;
  border-left: 1px solid var(--nv-border);
}

.mobile-close {
  display: none;
}

.inspector-head h2 {
  font-size: 18px;
}

.inspector-head small {
  color: var(--nv-muted);
}

.inspector-media {
  aspect-ratio: 4 / 3;
  border: 1px solid var(--nv-border);
  border-radius: 7px;
}

.asset-placeholder {
  display: grid;
  place-items: center;
  gap: 8px;
  color: var(--nv-muted);
  background: var(--nv-input);
}

.asset-detail-list {
  display: grid;
  gap: 8px;
}

.asset-detail-list div {
  display: grid;
  gap: 3px;
  padding: 9px;
  background: var(--nv-panel-2);
  border: 1px solid var(--nv-border);
  border-radius: 7px;
}

.asset-detail-list span {
  color: var(--nv-muted);
  font-size: 11px;
}

.asset-detail-list strong {
  min-width: 0;
  overflow-wrap: anywhere;
  font-size: 12px;
  font-weight: 500;
}

.asset-reference-row {
  width: 100%;
  color: inherit;
  text-align: left;
  background: transparent;
  border: 0;
}

.shot-meta,
.project-summary {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.prompt-editor {
  overflow: hidden;
  background: var(--nv-input);
  border: 1px solid var(--nv-border);
  border-radius: 7px;
}

.editor-toolbar {
  justify-content: space-between;
  min-height: 34px;
  padding: 0 8px;
  border-bottom: 1px solid var(--nv-border);
}

.editor-toolbar button {
  display: inline-grid;
  place-items: center;
  width: 26px;
  height: 26px;
  color: var(--nv-muted);
  background: transparent;
  border: 0;
}

.editor-body {
  display: grid;
  grid-template-columns: 38px minmax(0, 1fr);
}

.line-numbers {
  display: grid;
  align-content: start;
  gap: 0;
  padding: 9px 0;
  color: var(--nv-line-number-text);
  background: var(--nv-progress-track);
  border-right: 1px solid var(--nv-border);
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
  line-height: 1.55;
  text-align: center;
}

.editor-body textarea {
  width: 100%;
  resize: vertical;
  padding: 9px;
  color: var(--nv-code-text);
  background: transparent;
  border: 0;
  outline: none;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
  line-height: 1.55;
}

.editor-actions {
  flex-wrap: wrap;
  gap: 6px;
  padding: 8px;
  border-top: 1px solid var(--nv-border);
}

.reference-strip {
  display: flex;
  gap: 8px;
  overflow: auto;
}

.reference-strip img,
.reference-add {
  flex: 0 0 58px;
  width: 58px;
  height: 58px;
  border: 1px solid var(--nv-border);
  border-radius: 6px;
}

.reference-add {
  display: grid;
  place-items: center;
  color: var(--nv-muted);
  background: var(--nv-input);
}

.attempt-row {
  display: grid;
  grid-template-columns: auto 44px minmax(0, 1fr);
  align-items: center;
  gap: 8px;
  min-height: 32px;
  font-size: 12px;
}

.attempt-row small,
.empty-line {
  color: var(--nv-muted);
}

.inspector-actions {
  flex-wrap: wrap;
  gap: 8px;
}

.inspector-actions.sticky {
  position: sticky;
  bottom: -14px;
  margin: 0 -14px -14px;
  padding: 10px 14px;
  background: var(--nv-sticky-actions);
  border-top: 1px solid var(--nv-border);
}

.studio-message,
.studio-error {
  padding: 10px;
  border-radius: 7px;
  font-size: 13px;
}

.studio-message {
  color: var(--nv-success-message-text);
  background: rgba(25, 167, 124, 0.12);
  border: 1px solid rgba(25, 167, 124, 0.35);
}

.studio-error {
  color: var(--nv-error-text);
  background: rgba(219, 79, 79, 0.12);
  border: 1px solid rgba(219, 79, 79, 0.35);
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 1200px) {
  .novel-studio-shell {
    grid-template-columns: 200px minmax(0, 1fr) 320px;
  }

  .bible-grid,
  .export-preview-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }

  .creature-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 900px) {
  .novel-studio-shell {
    grid-template-columns: 1fr;
    overflow: visible;
  }

  .novel-studio-sidebar {
    position: sticky;
    top: 0;
    z-index: 3;
    border-right: 0;
    border-bottom: 1px solid var(--nv-border);
  }

  .workflow-nav {
    display: flex;
    overflow-x: auto;
  }

  .workflow-step {
    flex: 0 0 164px;
  }

  .novel-studio-main {
    max-height: none;
  }

  .novel-studio-inspector {
    position: fixed;
    right: 0;
    bottom: 0;
    left: 0;
    z-index: 5;
    max-height: 58vh;
    border-top: 1px solid var(--nv-border);
    border-left: 0;
    transform: translateY(calc(100% - 48px));
    transition: transform 0.2s ease;
  }

  .novel-studio-inspector.open {
    transform: translateY(0);
  }

  .mobile-close {
    display: inline-grid;
    place-items: center;
    justify-self: end;
    width: 30px;
    height: 30px;
    color: var(--nv-muted);
    background: var(--nv-button);
    border: 1px solid var(--nv-border);
    border-radius: 5px;
  }

  .import-grid,
  .episode-shot-grid,
  .render-board {
    grid-template-columns: 1fr;
  }

  .image-batch-summary,
  .shot-image-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 640px) {
  .creature-grid:not(.list) .actor-card {
    height: auto;
    overflow: visible;
  }

  .creature-card-main {
    grid-template-columns: 1fr;
  }

  .episode-modal-backdrop {
    padding: 0;
  }

  .episode-shots-modal {
    width: 100vw;
    max-height: 100vh;
    border-radius: 0;
  }

  .episode-modal-content {
    padding: 12px;
  }

  .novel-studio-shell {
    border-radius: 0;
  }

  .studio-brand h1 {
    font-size: 16px;
  }

  .settings-grid,
  .bible-grid,
  .render-metrics,
  .export-preview-grid,
  .shot-meta,
  .project-summary {
    grid-template-columns: 1fr;
  }

  .creature-card {
    grid-template-columns: 1fr;
  }
}
</style>
