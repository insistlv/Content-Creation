<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Clock, GitCompare, Music, Play, Plus, RefreshCw, Search, Shuffle, Trash2, Upload, X } from 'lucide-vue-next'
import SoftPanel from '../components/SoftPanel.vue'
import ClickSelect from '../components/ClickSelect.vue'
import VideoStylePresetLibrary from '../components/VideoStylePresetLibrary.vue'
import { api } from '../api/client.js'
import { applyAvailableCredits, loadCurrentUser, refreshCurrentUser } from '../stores/session.js'

const prompt = ref('')
const aspectRatio = ref('16:9')
const duration = ref('3')
const grokImagineVideoModel = 'grok-imagine-video-1.5-preview'
const doubaoSeedanceMiniVideoModel = 'doubao-seed-2-0-mini-260428'
const model = ref(grokImagineVideoModel)
const resolution = ref('')
const hd = ref(false)
const selectedReferenceIds = ref([])
const selectedReferenceVideoIds = ref([])
const selectedReferenceAudioIds = ref([])
const referenceAssets = ref([])
const referenceVideoAssets = ref([])
const referenceAudioAssets = ref([])
const referenceUploadInput = ref(null)
const referenceVideoUploadInput = ref(null)
const referenceAudioUploadInput = ref(null)
const referenceReplacementInputs = ref([])
const referenceUploading = ref(false)
const referenceDragging = ref(false)
const referencePreview = ref(null)
const me = ref(null)
const loading = ref(false)
const submitting = ref(false)
const task = ref(null)
const errorMessage = ref('')
const message = ref('')
const soundtrack = ref(null)
const soundtrackError = ref('')
const soundtrackGenerating = ref(false)
const soundtrackUploading = ref(false)
const soundtrackUploadInput = ref(null)
const videoStylePresets = ref([])
const videoStyleTemplates = ref([])
const selectedVideoStylePresetId = ref(null)
const selectedCustomVideoStyleId = ref(null)
const styleTemplateSaving = ref(false)
const historyLoading = ref(false)
const historyError = ref('')
const historyItems = ref([])
const historyPage = ref(1)
const historyPageSize = ref(8)
const historyTotal = ref(0)
const selectedHistoryGenerationId = ref(null)
const compareModalOpen = ref(false)
const compareHistoryItem = ref(null)
const historyFilterQuery = ref('')
const historyFilterStatus = ref('all')
const historyFilterEnhancement = ref('all')
const videoModels = ref([])
let videoDurationInitialized = false
const videoCreditEstimate = ref(null)
const videoCreditEstimateLoading = ref(false)
const videoCreditEstimateError = ref('')
let pollTimer = null
let videoCreditEstimateTimer = null
let videoCreditEstimateController = null
let videoCreditEstimateRequestId = 0
const maxPromptLength = 800
const maxReferenceImages = 4
const customStyleReferenceLimit = 3
const referenceImageRequiredMessage = '⚠️ 当前模型需参考图才能生成视频，暂不支持纯文本生成视频'
const aspectRatioOptions = [
  { value: '16:9', label: '16:9 横屏' },
  { value: '9:16', label: '9:16 竖屏' }
]
const wuyinDurationOptions = [
  { value: '1', label: '1 秒' },
  { value: '3', label: '3 秒' },
  { value: '6', label: '6 秒' },
  { value: '10', label: '10 秒' },
  { value: '15', label: '15 秒' }
]
const soraDurationOptions = [
  { value: '10', label: '10 秒' },
  { value: '15', label: '15 秒' },
  { value: '25', label: '25 秒 Pro' }
]
const modelOptions = [
  { value: grokImagineVideoModel, label: 'Grok Imagine' },
  { value: 'sora-2', label: 'Sora 2' },
  { value: 'sora-2-pro', label: 'Sora 2 Pro' }
]
const fallbackVideoModels = [
  {
    name: 'Grok Imagine',
    runtime_model: grokImagineVideoModel,
    provider: 'Wuyin',
    aspect_ratios: ['16:9', '9:16'],
    durations: ['1', '3', '6', '10', '15'],
    default_duration: '3',
    resolution_options: [],
    default_resolution: '',
    price_rules: [{ credits_per_second: 3 }],
    supports_hd: true,
    max_reference_images: maxReferenceImages,
    requires_reference_image: true,
    supports_reference_video: false,
    supports_reference_audio: false,
    max_reference_videos: 0,
    max_reference_audios: 0,
    supports_generate_audio: false
  },
  {
    name: 'Sora 2',
    runtime_model: 'sora-2',
    provider: 'GPT-Best',
    aspect_ratios: ['16:9', '9:16'],
    durations: ['10', '15', '25'],
    default_duration: '10',
    resolution_options: [],
    default_resolution: '',
    price_rules: [],
    supports_hd: true,
    max_reference_images: maxReferenceImages,
    requires_reference_image: false,
    supports_reference_video: false,
    supports_reference_audio: false,
    max_reference_videos: 0,
    max_reference_audios: 0,
    supports_generate_audio: false
  },
  {
    name: 'Sora 2 Pro',
    runtime_model: 'sora-2-pro',
    provider: 'GPT-Best',
    aspect_ratios: ['16:9', '9:16'],
    durations: ['10', '15', '25'],
    default_duration: '10',
    resolution_options: [],
    default_resolution: '',
    price_rules: [],
    supports_hd: true,
    max_reference_images: maxReferenceImages,
    requires_reference_image: false,
    supports_reference_video: false,
    supports_reference_audio: false,
    max_reference_videos: 0,
    max_reference_audios: 0,
    supports_generate_audio: false
  }
]
const aspectRatioLabels = {
  '16:9': '16:9 横屏',
  '4:3': '4:3 标准',
  '1:1': '1:1 方形',
  '3:4': '3:4 竖图',
  '9:16': '9:16 竖屏',
  '21:9': '21:9 宽银幕',
  adaptive: '自适应'
}
function canonicalVideoModelValue(value) {
  const raw = String(value || '').trim()
  if (!raw) return ''
  const normalized = raw.toLowerCase()
  if (normalized === 'doubao-seed-2-0-mini' || normalized === doubaoSeedanceMiniVideoModel) {
    return doubaoSeedanceMiniVideoModel
  }
  return raw
}

function fallbackVideoDurationsForModel(value) {
  return canonicalVideoModelValue(value) === grokImagineVideoModel ? ['1', '3', '6', '10', '15'] : ['10', '15', '25']
}

function isVisibleWorkspaceVideoModel(item = {}) {
  return item.permission !== 'internal' && item.available !== false
}

const visibleVideoModels = computed(() => videoModels.value.filter(isVisibleWorkspaceVideoModel))
const availableVideoModels = computed(() => {
  if (videoModels.value.length === 0) {
    return fallbackVideoModels
  }
  return visibleVideoModels.value.length > 0 ? visibleVideoModels.value : fallbackVideoModels
})
const videoModelOptions = computed(() => availableVideoModels.value.map((item) => ({
  value: canonicalVideoModelValue(item.runtime_model),
  label: videoModelOptionLabel(item)
})))
const selectedVideoModel = computed(() => (
  availableVideoModels.value.find((item) => canonicalVideoModelValue(item.runtime_model) === canonicalVideoModelValue(model.value)) ?? availableVideoModels.value[0] ?? fallbackVideoModels[0]
))
const selectedVideoCapability = computed(() => ({
  aspectRatios: selectedVideoModel.value?.aspect_ratios?.length ? selectedVideoModel.value.aspect_ratios : ['16:9', '9:16'],
  durations: selectedVideoModel.value?.durations?.length ? selectedVideoModel.value.durations : fallbackVideoDurationsForModel(selectedVideoModel.value?.runtime_model),
  defaultDuration: selectedVideoModel.value?.default_duration || '',
  resolutionOptions: selectedVideoModel.value?.resolution_options?.length ? selectedVideoModel.value.resolution_options : [],
  defaultResolution: selectedVideoModel.value?.default_resolution || '',
  priceRules: Array.isArray(selectedVideoModel.value?.price_rules) ? selectedVideoModel.value.price_rules : [],
  supportsHD: selectedVideoModel.value?.supports_hd !== false,
  maxReferenceImages: Number(selectedVideoModel.value?.max_reference_images) > 0 ? Number(selectedVideoModel.value.max_reference_images) : maxReferenceImages,
  requiresReferenceImage: selectedVideoModel.value?.requires_reference_image === true,
  supportsReferenceVideo: selectedVideoModel.value?.supports_reference_video === true,
  supportsReferenceAudio: selectedVideoModel.value?.supports_reference_audio === true,
  maxReferenceVideos: Number(selectedVideoModel.value?.max_reference_videos) > 0 ? Number(selectedVideoModel.value.max_reference_videos) : 0,
  maxReferenceAudios: Number(selectedVideoModel.value?.max_reference_audios) > 0 ? Number(selectedVideoModel.value.max_reference_audios) : 0,
  supportsGenerateAudio: selectedVideoModel.value?.supports_generate_audio === true
}))
const selectedVideoModelUnavailable = computed(() => selectedVideoModel.value?.available === false)
const selectedVideoModelDisabledReason = computed(() => selectedVideoModel.value?.disabled_reason || '\u5f53\u524d\u6a21\u578b\u6682\u4e0d\u53ef\u7528')
const videoAspectRatioOptions = computed(() => selectedVideoCapability.value.aspectRatios.map((value) => ({
  value,
  label: aspectRatioLabels[value] || value
})))
const videoDurationOptions = computed(() => selectedVideoCapability.value.durations.map((value) => ({
  value,
  label: value === '-1' ? '智能时长' : `${value} 秒`
})))

const shouldShowResolutionSelect = computed(() => selectedVideoCapability.value.resolutionOptions.length > 0)
const videoResolutionOptions = computed(() => selectedVideoCapability.value.resolutionOptions.map((value) => ({
  value,
  label: value
})))

const availableCredits = computed(() => me.value?.available_credits ?? task.value?.available_credits ?? 0)
const trimmedPrompt = computed(() => prompt.value.trim())
const isPromptTooLong = computed(() => trimmedPrompt.value.length > maxPromptLength)
const promptValidationMessage = computed(() => (isPromptTooLong.value ? `提示词不能超过 ${maxPromptLength} 字` : ''))
const selectedVideoRequiresReferenceImage = computed(() => selectedVideoCapability.value.requiresReferenceImage)
const hasSelectedReferenceImage = computed(() => selectedReferenceIds.value.length > 0)
const referenceImageValidationMessage = computed(() => (
  selectedVideoRequiresReferenceImage.value && !hasSelectedReferenceImage.value ? referenceImageRequiredMessage : ''
))
const videoEstimateRequiredCredits = computed(() => Number(videoCreditEstimate.value?.required_credits || 0))
const videoEstimateAvailableCredits = computed(() => Number(videoCreditEstimate.value?.available_credits ?? availableCredits.value ?? 0))
const videoEstimateMissingCredits = computed(() => Number(videoCreditEstimate.value?.missing_credits || 0))
const videoEstimateInsufficient = computed(() => videoCreditEstimate.value?.enough === false)
const videoCreditEstimateLabel = computed(() => {
  if (videoCreditEstimateLoading.value) return '正在预估点数...'
  if (videoCreditEstimateError.value) return videoCreditEstimateError.value
  if (!videoCreditEstimate.value || videoEstimateRequiredCredits.value <= 0) return ''
  if (videoEstimateInsufficient.value) {
    return `点数不足，还差 ${videoEstimateMissingCredits.value} 点`
  }
  return `预计消耗 ${videoEstimateRequiredCredits.value} 点 · 当前 ${videoEstimateAvailableCredits.value} 点 · 生成成功后扣除，失败不扣点`
})
const videoRechargeHref = computed(() => {
  if (!videoEstimateInsufficient.value) return ''
  const params = new URLSearchParams({
    source: 'video_generation',
    missing_credits: String(videoEstimateMissingCredits.value),
    required_credits: String(videoEstimateRequiredCredits.value)
  })
  const packageID = videoCreditEstimate.value?.recommended_package?.id
  if (packageID !== undefined && packageID !== null) {
    params.set('package_id', String(packageID))
  }
  return `/pricing?${params.toString()}`
})
const creditValidationMessage = computed(() => (
  videoEstimateInsufficient.value ? `点数不足，还差 ${videoEstimateMissingCredits.value} 点` : ''
))
const videoSubmitLabel = computed(() => {
  if (submitting.value) return '提交中...'
  if (videoEstimateRequiredCredits.value > 0) return `生成视频 · 预计 ${videoEstimateRequiredCredits.value} 点`
  return '生成视频'
})
const canSubmit = computed(() => trimmedPrompt.value.length > 0 && !isPromptTooLong.value && !referenceImageValidationMessage.value && !creditValidationMessage.value && !videoCreditEstimateError.value && !submitting.value && !selectedVideoModelUnavailable.value)
const shouldEstimateVideoCredits = computed(() => (
  trimmedPrompt.value.length > 0 &&
  !isPromptTooLong.value &&
  !referenceImageValidationMessage.value &&
  !selectedVideoModelUnavailable.value
))
const videoCreditEstimateKey = computed(() => (
  shouldEstimateVideoCredits.value ? JSON.stringify(buildVideoGenerationRequestPayload()) : ''
))
const isWuyinVideoModel = computed(() => model.value === grokImagineVideoModel)
const durationOptions = computed(() => (isWuyinVideoModel.value ? wuyinDurationOptions : soraDurationOptions))
const taskStatusLabel = computed(() => {
  if (submitting.value) return '提交中'
  if (!task.value?.status) return '未开始'
  const labels = {
    queued: '排队中',
    running: '生成中',
    succeeded: '已完成',
    failed: '生成失败'
  }
  return labels[task.value.status] ?? task.value.status
})
const taskStatusDescription = computed(() => {
  if (submitting.value) return '正在提交任务，请保持页面打开。'
  if (!task.value?.status) return '提交后这里会显示生成进度和最终视频。'
  if (task.value.status === 'queued') return '任务已进入队列，生成时间通常需要 1-10 分钟。'
  if (task.value.status === 'running') return '视频正在生成，完成后会自动刷新预览。'
  if (task.value.status === 'succeeded') return '视频生成完成，可以预览、下载或前往作品库查看。'
  if (task.value.status === 'failed') return localizeVideoErrorMessage(task.value.error?.message) || '视频生成失败，请调整提示词后重试。'
  return '任务状态已更新。'
})
const hasActiveTask = computed(() => task.value?.status === 'queued' || task.value?.status === 'running' || submitting.value)
const completedVideoWorkID = computed(() => (task.value?.status === 'succeeded' && task.value?.preview_url ? task.value?.work_id : null))
const showSoundtrackTools = computed(() => Boolean(completedVideoWorkID.value))
const soundtrackBusy = computed(() => soundtrackGenerating.value || soundtrackUploading.value)
const selectedReferenceAssets = computed(() =>
  selectedReferenceIds.value
    .map((id) => referenceAssets.value.find((asset) => Number(asset.id) === Number(id)))
    .filter(Boolean)
)
const selectedReferenceVideoAssets = computed(() =>
  selectedReferenceVideoIds.value
    .map((id) => referenceVideoAssets.value.find((asset) => Number(asset.id) === Number(id)))
    .filter(Boolean)
)
const selectedReferenceAudioAssets = computed(() =>
  selectedReferenceAudioIds.value
    .map((id) => referenceAudioAssets.value.find((asset) => Number(asset.id) === Number(id)))
    .filter(Boolean)
)
const selectedReferenceCount = computed(() => selectedReferenceIds.value.length)
const selectedReferenceVideoCount = computed(() => selectedReferenceVideoIds.value.length)
const selectedReferenceAudioCount = computed(() => selectedReferenceAudioIds.value.length)
const selectedOfficialVideoStyle = computed(() =>
  videoStylePresets.value.find((item) => Number(item.id) === Number(selectedVideoStylePresetId.value)) ?? null
)
const selectedCustomVideoStyle = computed(() =>
  videoStyleTemplates.value.find((item) => Number(item.id) === Number(selectedCustomVideoStyleId.value)) ?? null
)
const selectedVideoStyleLabel = computed(() => selectedOfficialVideoStyle.value?.title || selectedCustomVideoStyle.value?.title || '')
const contentReferenceLimit = computed(() => (selectedCustomVideoStyle.value ? customStyleReferenceLimit : selectedVideoCapability.value.maxReferenceImages))
const referenceVideoLimit = computed(() => selectedVideoCapability.value.maxReferenceVideos)
const referenceAudioLimit = computed(() => selectedVideoCapability.value.maxReferenceAudios)
const referenceAtLimit = computed(() => selectedReferenceCount.value >= contentReferenceLimit.value)
const referenceVideoAtLimit = computed(() => selectedReferenceVideoCount.value >= referenceVideoLimit.value)
const referenceAudioAtLimit = computed(() => selectedReferenceAudioCount.value >= referenceAudioLimit.value)
const selectedHistoryItem = computed(() => {
  if (!selectedHistoryGenerationId.value) return null
  return historyItems.value.find((item) => Number(item.generation_id) === Number(selectedHistoryGenerationId.value)) ?? null
})
const selectedVideo = computed(() => selectedHistoryItem.value ?? task.value ?? (historyItems.value.length > 0 ? historyItems.value[0] : null))
const selectedVideoAspectClass = computed(() => (selectedVideo.value?.aspect_ratio === '9:16' ? 'portrait' : 'landscape'))
const selectedVideoStatusLabel = computed(() => {
  const source = selectedVideo.value
  if (!source) return taskStatusLabel.value
  return historyStatusLabel(source.status)
})
const selectedVideoDownloadURL = computed(() => selectedVideo.value?.download_url || '')
const selectedVideoPreviewURL = computed(() => selectedVideo.value?.preview_url || '')
const selectedVideoDescription = computed(() => {
  if (!selectedVideo.value) return taskStatusDescription.value
  if (selectedVideo.value.status === 'failed') {
    return localizeVideoErrorMessage(selectedVideo.value.error_message || selectedVideo.value.error?.message) || '该任务生成失败，可使用相同参数重试。'
  }
  return selectedVideo.value.prompt_summary || selectedVideo.value.prompt || taskStatusDescription.value
})

function syncDefaultOfficialVideoStyle() {
  const firstPreset = videoStylePresets.value[0] ?? null
  if (!firstPreset) {
    selectedVideoStylePresetId.value = null
    return
  }
  if (selectedCustomVideoStyleId.value) return
  const selectedPresetExists = videoStylePresets.value.some((item) => Number(item.id) === Number(selectedVideoStylePresetId.value))
  if (!selectedPresetExists) {
    selectedVideoStylePresetId.value = firstPreset.id
  }
}

function preferredVideoDuration(values = []) {
  if (values.includes(selectedVideoCapability.value.defaultDuration)) return selectedVideoCapability.value.defaultDuration
  return values[0] ?? '10'
}

function syncSupportedVideoDuration() {
  const values = videoDurationOptions.value.map((option) => option.value)
  if (!videoDurationInitialized && videoModels.value.length > 0 && values.length > 0) {
    duration.value = preferredVideoDuration(values)
    videoDurationInitialized = true
    return
  }
  if (!values.includes(duration.value)) {
    duration.value = preferredVideoDuration(values)
  }
}

function syncSupportedVideoResolution() {
  const values = videoResolutionOptions.value.map((option) => option.value)
  if (values.length === 0) {
    resolution.value = ''
    return
  }
  if (!values.includes(resolution.value)) {
    resolution.value = selectedVideoCapability.value.defaultResolution && values.includes(selectedVideoCapability.value.defaultResolution)
      ? selectedVideoCapability.value.defaultResolution
      : values[0]
  }
}

watch(model, () => {
  const canonicalModel = canonicalVideoModelValue(model.value)
  if (canonicalModel && canonicalModel !== model.value) {
    model.value = canonicalModel
    return
  }
  syncSupportedVideoDuration()
  syncSupportedVideoResolution()
  if (!videoAspectRatioOptions.value.some((option) => option.value === aspectRatio.value)) {
    aspectRatio.value = videoAspectRatioOptions.value[0]?.value ?? '16:9'
  }
  if (!selectedVideoCapability.value.supportsHD) {
    hd.value = false
  }
  if (selectedReferenceIds.value.length > contentReferenceLimit.value) {
    selectedReferenceIds.value = selectedReferenceIds.value.slice(0, contentReferenceLimit.value)
  }
  if (!selectedVideoCapability.value.supportsReferenceVideo) {
    selectedReferenceVideoIds.value = []
  } else if (selectedReferenceVideoIds.value.length > referenceVideoLimit.value) {
    selectedReferenceVideoIds.value = selectedReferenceVideoIds.value.slice(0, referenceVideoLimit.value)
  }
  if (!selectedVideoCapability.value.supportsReferenceAudio) {
    selectedReferenceAudioIds.value = []
  } else if (selectedReferenceAudioIds.value.length > referenceAudioLimit.value) {
    selectedReferenceAudioIds.value = selectedReferenceAudioIds.value.slice(0, referenceAudioLimit.value)
  }
})

watch(videoModels, () => {
  const canonicalModel = canonicalVideoModelValue(model.value)
  if (canonicalModel && canonicalModel !== model.value) {
    model.value = canonicalModel
  }
  if (!videoModelOptions.value.some((option) => option.value === canonicalVideoModelValue(model.value))) {
    model.value = videoModelOptions.value[0]?.value ?? grokImagineVideoModel
  }
  syncSupportedVideoDuration()
  syncSupportedVideoResolution()
  if (!videoAspectRatioOptions.value.some((option) => option.value === aspectRatio.value)) {
    aspectRatio.value = videoAspectRatioOptions.value[0]?.value ?? '16:9'
  }
})

watch(videoStylePresets, syncDefaultOfficialVideoStyle)

watch(selectedCustomVideoStyleId, (id) => {
  if (!id) {
    syncDefaultOfficialVideoStyle()
  }
})

watch(videoCreditEstimateKey, () => {
  scheduleVideoCreditEstimate()
})

function syncAvailableCredits(payload) {
  if (payload?.available_credits === undefined) return
  const sharedUser = applyAvailableCredits(payload.available_credits)
  me.value = {
    ...(me.value ?? sharedUser ?? {}),
    available_credits: sharedUser?.available_credits ?? payload.available_credits
  }
}

function buildVideoGenerationRequestPayload() {
  const requestPayload = {
    prompt: trimmedPrompt.value,
    aspect_ratio: aspectRatio.value,
    duration: duration.value,
    model: canonicalVideoModelValue(model.value),
    hd: resolution.value ? ['720p', '1080p'].includes(resolution.value) : hd.value,
    reference_asset_ids: selectedReferenceIds.value.slice(0, contentReferenceLimit.value)
  }
  if (selectedVideoCapability.value.supportsReferenceVideo) {
    requestPayload.reference_video_asset_ids = selectedReferenceVideoIds.value.slice(0, referenceVideoLimit.value)
  }
  if (selectedVideoCapability.value.supportsReferenceAudio) {
    requestPayload.reference_audio_asset_ids = selectedReferenceAudioIds.value.slice(0, referenceAudioLimit.value)
    if (requestPayload.reference_audio_asset_ids.length > 0) {
      requestPayload.generate_audio = true
    }
  }
  if (resolution.value) {
    requestPayload.resolution = resolution.value
  }
  if (selectedOfficialVideoStyle.value) {
    requestPayload.video_style_preset_id = selectedOfficialVideoStyle.value.id
  }
  if (selectedCustomVideoStyle.value) {
    requestPayload.custom_video_style_id = selectedCustomVideoStyle.value.id
  }
  if (selectedVideoStyleLabel.value) {
    requestPayload.style_preset = selectedVideoStyleLabel.value
  }
  return requestPayload
}

function clearVideoCreditEstimateTimer() {
  if (videoCreditEstimateTimer) {
    clearTimeout(videoCreditEstimateTimer)
    videoCreditEstimateTimer = null
  }
}

function abortVideoCreditEstimate() {
  if (videoCreditEstimateController) {
    videoCreditEstimateController.abort()
    videoCreditEstimateController = null
  }
}

function resetVideoCreditEstimate() {
  clearVideoCreditEstimateTimer()
  abortVideoCreditEstimate()
  videoCreditEstimateRequestId += 1
  videoCreditEstimate.value = null
  videoCreditEstimateLoading.value = false
  videoCreditEstimateError.value = ''
}

function scheduleVideoCreditEstimate() {
  clearVideoCreditEstimateTimer()
  abortVideoCreditEstimate()
  if (!shouldEstimateVideoCredits.value) {
    resetVideoCreditEstimate()
    return
  }
  videoCreditEstimateLoading.value = true
  videoCreditEstimateError.value = ''
  videoCreditEstimateTimer = setTimeout(() => {
    void fetchVideoCreditEstimate()
  }, 300)
}

async function fetchVideoCreditEstimate() {
  const requestId = videoCreditEstimateRequestId + 1
  videoCreditEstimateRequestId = requestId
  const controller = new AbortController()
  videoCreditEstimateController = controller
  try {
    const payload = await api.estimateVideoGeneration(buildVideoGenerationRequestPayload(), { signal: controller.signal })
    if (requestId !== videoCreditEstimateRequestId) return
    videoCreditEstimate.value = payload
    videoCreditEstimateError.value = ''
    syncAvailableCredits(payload)
  } catch (error) {
    if (controller.signal.aborted || error?.name === 'AbortError' || requestId !== videoCreditEstimateRequestId) return
    videoCreditEstimate.value = null
    videoCreditEstimateError.value = localizeVideoErrorMessage(error.message) || '点数预估失败，请稍后重试'
  } finally {
    if (requestId === videoCreditEstimateRequestId) {
      videoCreditEstimateLoading.value = false
      if (videoCreditEstimateController === controller) {
        videoCreditEstimateController = null
      }
    }
  }
}

function isSeedanceModel(item = {}) {
  const text = `${item.runtime_model || ''} ${item.name || ''} ${item.provider || ''}`.toLowerCase()
  return text.includes('seedance') || text.includes('doubao') || text.includes('volcengine')
}

function videoModelOptionLabel(item = {}) {
  const label = item.name || item.runtime_model || ''
  const badges = []
  if (isSeedanceModel(item)) {
    badges.push('Seedance')
  }
  if (item.permission === 'internal') {
    badges.push('\u5185\u6d4b')
  }
  if (item.available === false && isSeedanceModel(item) && item.api_key_set === false) {
    badges.push('\u5f85\u914d\u7f6e')
  }
  if (item.available === false && String(item.disabled_reason || '').includes('\u4e0d\u652f\u6301\u89c6\u9891\u751f\u6210 API')) {
    badges.push('\u4e0d\u652f\u6301\u89c6\u9891\u751f\u6210 API')
  } else if (item.available === false && String(item.disabled_reason || '').includes('\u80fd\u529b\u6821\u9a8c')) {
    badges.push('\u5f85\u6821\u9a8c')
  }
  if (item.available === false && !badges.includes('\u4e0d\u53ef\u7528')) {
    badges.push('\u4e0d\u53ef\u7528')
  }
  return badges.length ? `${label} [${badges.join('] [')}]` : label
}

function normalizeVideoModelItems(items = []) {
  return (Array.isArray(items) ? items : [])
    .map((item) => ({
      ...item,
      name: item?.name || item?.runtime_model || item?.id || '',
      runtime_model: canonicalVideoModelValue(item?.runtime_model || item?.model || item?.id || ''),
      permission: item?.permission || 'public',
      available: item?.available !== false,
      disabled_reason: item?.disabled_reason || '',
      api_key_set: item?.api_key_set === true,
      aspect_ratios: Array.isArray(item?.aspect_ratios) && item.aspect_ratios.length > 0 ? item.aspect_ratios : ['16:9', '9:16'],
      durations: Array.isArray(item?.durations) && item.durations.length > 0 ? item.durations.map((value) => String(value)) : fallbackVideoDurationsForModel(item?.runtime_model || item?.model || item?.id || ''),
      default_duration: item?.default_duration ? String(item.default_duration) : '',
      resolution_options: Array.isArray(item?.resolution_options) ? item.resolution_options.map((value) => String(value)) : [],
      default_resolution: item?.default_resolution ? String(item.default_resolution) : '',
      price_rules: Array.isArray(item?.price_rules) ? item.price_rules : [],
      supports_hd: item?.supports_hd !== false,
      max_reference_images: Number(item?.max_reference_images) > 0 ? Number(item.max_reference_images) : maxReferenceImages,
      requires_reference_image: item?.requires_reference_image === true,
      supports_reference_video: item?.supports_reference_video === true,
      supports_reference_audio: item?.supports_reference_audio === true,
      max_reference_videos: Number(item?.max_reference_videos) > 0 ? Number(item.max_reference_videos) : 0,
      max_reference_audios: Number(item?.max_reference_audios) > 0 ? Number(item.max_reference_audios) : 0,
      supports_generate_audio: item?.supports_generate_audio === true
    }))
    .filter((item) => item.runtime_model)
}

function localizeVideoErrorMessage(message) {
  const raw = String(message || '').trim()
  if (!raw) return ''
  const normalized = raw.toLowerCase()
  const referenceImageSignals = [
    'requires an input image',
    'input image is required',
    'requires input image',
    'image input is required',
    'text-to-video is not supported',
    'text to video is not supported'
  ]
  return referenceImageSignals.some((signal) => normalized.includes(signal)) ? referenceImageRequiredMessage : raw
}

function normalizedHistoryItem(item = {}) {
  return {
    ...item,
    generation_id: item.generation_id ?? item.id,
    prompt: item.prompt ?? item.prompt_summary ?? '',
    prompt_summary: item.prompt_summary ?? item.prompt ?? '',
    status: item.status ?? 'succeeded',
    error_message: localizeVideoErrorMessage(item.error_message),
    error: item.error ? { ...item.error, message: localizeVideoErrorMessage(item.error.message) } : item.error,
    enhancement_tags: Array.isArray(item.enhancement_tags) ? item.enhancement_tags : [],
    reference_asset_ids: Array.isArray(item.reference_asset_ids) ? item.reference_asset_ids : []
  }
}

function historyItemKey(item) {
  return item?.generation_id ?? item?.id
}

function visibleEnhancementTags(item) {
  return (item?.enhancement_tags ?? []).slice(0, 3)
}

function hiddenEnhancementCount(item) {
  return Math.max((item?.enhancement_tags ?? []).length - 3, 0)
}

function formatHistoryTime(value) {
  if (!value) return '刚刚'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '刚刚'
  return date.toLocaleString('zh-CN', {
    month: 'numeric',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit'
  })
}

function historyStatusLabel(status) {
  const labels = {
    queued: '排队中',
    running: '生成中',
    succeeded: '已完成',
    failed: '失败'
  }
  return labels[status] ?? status ?? '未知'
}

function historyModelLabel(item) {
  return item?.model_name || item?.runtime_model || '视频模型'
}

function historyMetaLine(item) {
  return [
    historyModelLabel(item),
    item?.duration_seconds ? `${item.duration_seconds}s` : '',
    item?.aspect_ratio || '',
    Number(item?.credits_cost) > 0 ? `${item.credits_cost}点` : ''
  ].filter(Boolean).join(' · ')
}

function normalizeVideoDurationValue(value) {
  const raw = String(value || '').trim()
  if (raw === '-1') return '-1'
  const normalized = raw.replace(/[^\d]/g, '')
  return normalized || '10'
}

function modelValueFromHistory(item) {
  const runtime = canonicalVideoModelValue(item?.runtime_model || item?.model || '')
  if (!runtime) return model.value
  if (videoModelOptions.value.some((option) => option.value === runtime)) {
    return runtime
  }
  return runtime.includes('sora-2-pro') ? 'sora-2-pro' : runtime.includes('sora-2') ? 'sora-2' : grokImagineVideoModel
}

async function refreshSessionCredits() {
  const payload = await refreshCurrentUser()
  if (payload) {
    me.value = payload
  }
}

async function load() {
  loading.value = true
  errorMessage.value = ''
  try {
    const [mePayload, assetsPayload, videoAssetsPayload, audioAssetsPayload, modelsPayload, presetsPayload, templatesPayload] = await Promise.all([
      loadCurrentUser({ force: true }),
      api.listReferenceAssets({ kind: 'image' }).catch(() => ({ items: [] })),
      api.listReferenceAssets({ kind: 'video' }).catch(() => ({ items: [] })),
      api.listReferenceAssets({ kind: 'audio' }).catch(() => ({ items: [] })),
      api.listVideoModels().catch(() => ({ items: [] })),
      api.listVideoStylePresets().catch(() => ({ items: [] })),
      api.listVideoStyleTemplates().catch(() => ({ items: [] }))
    ])
    me.value = mePayload
    referenceAssets.value = assetsPayload.items ?? []
    referenceVideoAssets.value = videoAssetsPayload.items ?? []
    referenceAudioAssets.value = audioAssetsPayload.items ?? []
    videoModels.value = normalizeVideoModelItems(modelsPayload.items)
    videoStylePresets.value = presetsPayload.items ?? []
    videoStyleTemplates.value = templatesPayload.items ?? []
    syncDefaultOfficialVideoStyle()
  } catch (error) {
    errorMessage.value = localizeVideoErrorMessage(error.message)
  } finally {
    loading.value = false
  }
}

async function loadHistory(page = historyPage.value) {
  historyLoading.value = true
  historyError.value = ''
  const params = {
    page,
    page_size: historyPageSize.value
  }
  if (historyFilterQuery.value.trim()) params.q = historyFilterQuery.value.trim()
  if (historyFilterStatus.value !== 'all') params.status = historyFilterStatus.value
  if (historyFilterEnhancement.value !== 'all') params.enhancement = historyFilterEnhancement.value
  try {
    const payload = await api.listUserVideoGenerations(params)
    historyItems.value = (payload.items ?? []).map(normalizedHistoryItem)
    historyPage.value = payload.page ?? page
    historyPageSize.value = payload.page_size ?? historyPageSize.value
    historyTotal.value = payload.total ?? historyItems.value.length
    if (!selectedHistoryGenerationId.value && historyItems.value.length > 0 && !task.value?.preview_url) {
      selectedHistoryGenerationId.value = historyItemKey(historyItems.value[0])
    }
    if (selectedHistoryGenerationId.value && !historyItems.value.some((item) => Number(historyItemKey(item)) === Number(selectedHistoryGenerationId.value))) {
      selectedHistoryGenerationId.value = historyItems.value.length > 0 && !task.value?.preview_url ? historyItemKey(historyItems.value[0]) : null
    }
  } catch (error) {
    historyError.value = localizeVideoErrorMessage(error.message) || '历史任务读取失败'
  } finally {
    historyLoading.value = false
  }
}

function selectHistoryItem(item) {
  selectedHistoryGenerationId.value = historyItemKey(item)
  resetSoundtrackState()
}

function refillComposerFromHistory(item) {
  if (!item) return
  prompt.value = item.prompt || item.prompt_summary || ''
  aspectRatio.value = item.aspect_ratio || aspectRatio.value
  model.value = modelValueFromHistory(item)
  duration.value = normalizeVideoDurationValue(item.duration_seconds || item.duration)
  hd.value = Boolean(item.hd)
  selectedVideoStylePresetId.value = null
  selectedCustomVideoStyleId.value = null
  selectedReferenceIds.value = (item.reference_asset_ids ?? []).slice(0, contentReferenceLimit.value)
  selectedHistoryGenerationId.value = historyItemKey(item)
  message.value = '已回填历史任务参数，可编辑后再次生成。'
  errorMessage.value = ''
}

function openCompareModal(item) {
  compareHistoryItem.value = item
  compareModalOpen.value = true
}

function closeCompareModal() {
  compareModalOpen.value = false
  compareHistoryItem.value = null
}

function useCompareHistoryVersion() {
  if (compareHistoryItem.value) {
    selectHistoryItem(compareHistoryItem.value)
  }
  closeCompareModal()
}

async function applyHistoryFilters() {
  historyPage.value = 1
  await loadHistory(1)
}

function upsertReferenceAsset(asset) {
  if (!asset?.id) return
  referenceAssets.value = [
    ...referenceAssets.value.filter((item) => Number(item.id) !== Number(asset.id)),
    asset
  ]
}

function upsertReferenceMediaAsset(asset, kind) {
  if (!asset?.id) return
  const target = kind === 'audio' ? referenceAudioAssets : kind === 'video' ? referenceVideoAssets : referenceAssets
  target.value = [
    ...target.value.filter((item) => Number(item.id) !== Number(asset.id)),
    asset
  ]
}

function resetFileInput(event) {
  if (event?.target) {
    event.target.value = ''
  }
}

async function uploadReferenceFile(file) {
  const uploaded = await api.uploadReferenceAsset(file)
  upsertReferenceAsset(uploaded)
  return uploaded
}

async function uploadReferenceMediaFile(file, kind) {
  const uploaded = await api.uploadReferenceAsset(file)
  upsertReferenceMediaAsset(uploaded, kind)
  return uploaded
}

async function uploadReferenceFiles(files) {
  const pendingFiles = Array.from(files || [])
  const remainingSlots = Math.max(contentReferenceLimit.value - selectedReferenceCount.value, 0)
  const acceptedFiles = pendingFiles.slice(0, remainingSlots)
  if (!acceptedFiles.length) {
    return
  }

  referenceUploading.value = true
  errorMessage.value = ''
  try {
    for (const file of acceptedFiles) {
      const uploaded = await uploadReferenceFile(file)
      if (!selectedReferenceIds.value.includes(uploaded.id)) {
        selectedReferenceIds.value = [...selectedReferenceIds.value, uploaded.id].slice(0, contentReferenceLimit.value)
      }
    }
  } catch (error) {
    errorMessage.value = localizeVideoErrorMessage(error.message) || '参考图上传失败'
  } finally {
    referenceUploading.value = false
  }
}

async function uploadReferenceMediaFiles(files, kind) {
  const pendingFiles = Array.from(files || [])
  const selectedIds = kind === 'audio' ? selectedReferenceAudioIds : selectedReferenceVideoIds
  const limit = kind === 'audio' ? referenceAudioLimit.value : referenceVideoLimit.value
  const remainingSlots = Math.max(limit - selectedIds.value.length, 0)
  const acceptedFiles = pendingFiles.slice(0, remainingSlots)
  if (!acceptedFiles.length) {
    return
  }

  referenceUploading.value = true
  errorMessage.value = ''
  try {
    for (const file of acceptedFiles) {
      const uploaded = await uploadReferenceMediaFile(file, kind)
      if (!selectedIds.value.includes(uploaded.id)) {
        selectedIds.value = [...selectedIds.value, uploaded.id].slice(0, limit)
      }
    }
  } catch (error) {
    errorMessage.value = localizeVideoErrorMessage(error.message) || (kind === 'audio' ? '参考音频上传失败' : '参考视频上传失败')
  } finally {
    referenceUploading.value = false
  }
}

async function handleReferenceUpload(event) {
  try {
    await uploadReferenceFiles(event?.target?.files)
  } finally {
    resetFileInput(event)
  }
}

async function handleReferenceVideoUpload(event) {
  try {
    await uploadReferenceMediaFiles(event?.target?.files, 'video')
  } finally {
    resetFileInput(event)
  }
}

async function handleReferenceAudioUpload(event) {
  try {
    await uploadReferenceMediaFiles(event?.target?.files, 'audio')
  } finally {
    resetFileInput(event)
  }
}

async function handleReferenceDrop(event) {
  referenceDragging.value = false
  if (referenceAtLimit.value || referenceUploading.value) return
  await uploadReferenceFiles(event?.dataTransfer?.files)
}

function handleReferenceDragEnter() {
  if (referenceAtLimit.value || referenceUploading.value) return
  referenceDragging.value = true
}

function handleReferenceDragLeave() {
  referenceDragging.value = false
}

async function replaceReferenceAt(index, event) {
  const file = Array.from(event?.target?.files || [])[0]
  if (!file || index < 0 || index >= selectedReferenceIds.value.length) {
    resetFileInput(event)
    return
  }

  referenceUploading.value = true
  errorMessage.value = ''
  try {
    const uploaded = await uploadReferenceFile(file)
    selectedReferenceIds.value = selectedReferenceIds.value.map((id, currentIndex) => (
      currentIndex === index ? uploaded.id : id
    ))
  } catch (error) {
    errorMessage.value = localizeVideoErrorMessage(error.message) || '参考图替换失败'
  } finally {
    referenceUploading.value = false
    resetFileInput(event)
  }
}

function openReferenceUpload() {
  if (referenceAtLimit.value || referenceUploading.value) return
  referenceUploadInput.value?.click()
}

function openReferenceVideoUpload() {
  if (!selectedVideoCapability.value.supportsReferenceVideo || referenceVideoAtLimit.value || referenceUploading.value) return
  referenceVideoUploadInput.value?.click()
}

function openReferenceAudioUpload() {
  if (!selectedVideoCapability.value.supportsReferenceAudio || referenceAudioAtLimit.value || referenceUploading.value) return
  referenceAudioUploadInput.value?.click()
}

function setReferenceReplacementInput(element, index) {
  if (element) {
    referenceReplacementInputs.value[index] = element
  }
}

function openReferenceReplacement(index) {
  referenceReplacementInputs.value[index]?.click()
}

function removeReference(id) {
  selectedReferenceIds.value = selectedReferenceIds.value.filter((item) => Number(item) !== Number(id))
}

function removeReferenceVideo(id) {
  selectedReferenceVideoIds.value = selectedReferenceVideoIds.value.filter((item) => Number(item) !== Number(id))
}

function removeReferenceAudio(id) {
  selectedReferenceAudioIds.value = selectedReferenceAudioIds.value.filter((item) => Number(item) !== Number(id))
}

function selectOfficialVideoStyle(preset) {
  selectedVideoStylePresetId.value = preset?.id ?? null
  selectedCustomVideoStyleId.value = null
}

function selectCustomVideoStyle(template) {
  selectedCustomVideoStyleId.value = template?.id ?? null
  selectedVideoStylePresetId.value = null
  selectedReferenceIds.value = selectedReferenceIds.value.slice(0, customStyleReferenceLimit)
}

async function createCustomVideoStyleTemplate(input) {
  if (!input?.file) return
  styleTemplateSaving.value = true
  errorMessage.value = ''
  try {
    const uploaded = await uploadReferenceFile(input.file)
    const template = await api.createVideoStyleTemplate({
      title: input.title,
      description: input.description,
      reference_asset_id: uploaded.id,
      style_prompt: input.style_prompt
    })
    videoStyleTemplates.value = [
      template,
      ...videoStyleTemplates.value.filter((item) => Number(item.id) !== Number(template.id))
    ]
    selectCustomVideoStyle(template)
    message.value = '已保存自定义视频风格'
  } catch (error) {
    errorMessage.value = localizeVideoErrorMessage(error.message) || '自定义视频风格保存失败'
  } finally {
    styleTemplateSaving.value = false
  }
}

async function deleteCustomVideoStyleTemplate(template) {
  if (!template?.id) return
  if (typeof window !== 'undefined' && window.confirm && !window.confirm(`删除视频风格模板「${template.title}」？`)) {
    return
  }
  errorMessage.value = ''
  try {
    await api.deleteVideoStyleTemplate(template.id)
    videoStyleTemplates.value = videoStyleTemplates.value.filter((item) => Number(item.id) !== Number(template.id))
    if (Number(selectedCustomVideoStyleId.value) === Number(template.id)) {
      selectedCustomVideoStyleId.value = null
    }
  } catch (error) {
    errorMessage.value = localizeVideoErrorMessage(error.message) || '自定义视频风格删除失败'
  }
}

function openReferencePreview(asset) {
  if (!asset?.preview_url) return
  referencePreview.value = asset
}

function closeReferencePreview() {
  referencePreview.value = null
}

async function submitVideo() {
  if (isPromptTooLong.value) {
    errorMessage.value = `提示词不能超过 ${maxPromptLength} 字`
    return
  }
  if (referenceImageValidationMessage.value) {
    return
  }
  if (creditValidationMessage.value) {
    errorMessage.value = creditValidationMessage.value
    return
  }
  if (!canSubmit.value) return
  submitting.value = true
  selectedHistoryGenerationId.value = null
  message.value = ''
  errorMessage.value = ''
  resetSoundtrackState()
  try {
    const payload = await api.createVideoGeneration(buildVideoGenerationRequestPayload())
    task.value = payload
    syncAvailableCredits(payload)
    message.value = '视频任务已提交，生成时间通常需要 1-10 分钟。'
    await pollVideo(payload.generation_id)
  } catch (error) {
    errorMessage.value = localizeVideoErrorMessage(error.message)
  } finally {
    submitting.value = false
  }
}

async function pollVideo(id) {
  if (!id) return
  const payload = await api.getVideoGeneration(id)
  task.value = payload
  syncAvailableCredits(payload)
  if (payload.status === 'queued' || payload.status === 'running') {
    pollTimer = window.setTimeout(() => pollVideo(id), 5000)
  } else if (payload.status === 'succeeded') {
    message.value = '视频生成完成'
    await loadVideoSoundtracks(payload.work_id)
    void loadHistory(1)
    if (payload.available_credits === undefined) {
      void refreshSessionCredits()
    }
  } else if (payload.status === 'failed') {
    errorMessage.value = localizeVideoErrorMessage(payload.error?.message) || '视频生成失败'
    void loadHistory(1)
    if (payload.available_credits === undefined) {
      void refreshSessionCredits()
    }
  }
}

function resetSoundtrackState() {
  soundtrack.value = null
  soundtrackError.value = ''
  soundtrackGenerating.value = false
  soundtrackUploading.value = false
}

async function loadVideoSoundtracks(workID = completedVideoWorkID.value) {
  if (!workID) return
  try {
    const payload = await api.listVideoSoundtracks(workID)
    soundtrack.value = payload.items?.[0] ?? null
  } catch {
    soundtrack.value = null
  }
}

async function generateSoundtrack(variation) {
  const workID = completedVideoWorkID.value
  if (!workID || soundtrackBusy.value) return
  soundtrackGenerating.value = true
  soundtrackError.value = ''
  try {
    soundtrack.value = await api.generateVideoSoundtrack(workID, { variation })
  } catch (error) {
    soundtrackError.value = localizeVideoErrorMessage(error.message) || '配乐生成失败'
  } finally {
    soundtrackGenerating.value = false
  }
}

function openSoundtrackUpload() {
  if (!completedVideoWorkID.value || soundtrackBusy.value) return
  soundtrackUploadInput.value?.click()
}

async function handleSoundtrackUpload(event) {
  const file = Array.from(event?.target?.files || [])[0]
  if (!file || !completedVideoWorkID.value) {
    resetFileInput(event)
    return
  }
  soundtrackUploading.value = true
  soundtrackError.value = ''
  try {
    soundtrack.value = await api.uploadVideoSoundtrack(completedVideoWorkID.value, file)
  } catch (error) {
    soundtrackError.value = localizeVideoErrorMessage(error.message) || '音乐上传失败'
  } finally {
    soundtrackUploading.value = false
    resetFileInput(event)
  }
}

onMounted(() => {
  void load()
  void loadHistory(1)
})
onBeforeUnmount(() => {
  if (pollTimer) {
    window.clearTimeout(pollTimer)
  }
  clearVideoCreditEstimateTimer()
  abortVideoCreditEstimate()
})
</script>

<template>
  <section class="video-workspace-page">
    <div class="video-workspace-grid">
      <SoftPanel class="video-composer-panel" tone="default" roomy>
        <div class="video-composer-scroll">
          <div class="video-section-head">
            <div>
              <p class="eyebrow">AI Video</p>
              <h1>视频生成</h1>
            </div>
            <span>剩余 {{ availableCredits }} 点</span>
          </div>

          <label class="video-field video-prompt-field">
            <span>提示词</span>
            <textarea
              v-model="prompt"
              data-testid="video-prompt"
              class="text-input"
              rows="5"
              :maxlength="maxPromptLength"
              placeholder="描述镜头、主体动作、环境和风格"
            />
          </label>

          <div class="video-form-grid">
            <label class="video-field">
              <span>画幅</span>
              <ClickSelect v-model="aspectRatio" :options="videoAspectRatioOptions" data-testid="video-aspect-ratio" class="text-input" aria-label="画幅" />
            </label>

            <div class="video-field video-duration-field">
              <span>时长</span>
              <ClickSelect v-model="duration" :options="videoDurationOptions" data-testid="video-duration" class="text-input" aria-label="视频时长" />
            </div>

            <label class="video-field">
              <span>模型</span>
              <ClickSelect v-model="model" :options="videoModelOptions" data-testid="video-model" class="text-input" aria-label="模型" />
              <small v-if="selectedVideoModelUnavailable" class="video-model-readiness" data-testid="video-model-readiness">{{ selectedVideoModelDisabledReason }}</small>
            </label>

            <label v-if="shouldShowResolutionSelect" class="video-field">
              <span>分辨率</span>
              <ClickSelect v-model="resolution" :options="videoResolutionOptions" data-testid="video-resolution" class="text-input" aria-label="分辨率" />
            </label>

            <label v-else class="video-toggle">
              <input v-model="hd" data-testid="video-hd" type="checkbox" :disabled="!selectedVideoCapability.supportsHD" />
              <span>高清输出</span>
            </label>
          </div>

          <div v-if="videoCreditEstimateLabel" class="video-credit-estimate" data-testid="video-credit-estimate">
            <span>{{ videoCreditEstimateLabel }}</span>
            <a
              v-if="videoEstimateInsufficient"
              class="video-recharge-link"
              data-testid="video-recharge-link"
              :href="videoRechargeHref"
            >
              去充值
            </a>
          </div>

          <div class="video-creation-assist-grid" data-testid="video-creation-assist-grid">
            <VideoStylePresetLibrary
              :presets="videoStylePresets"
              :templates="videoStyleTemplates"
              :selected-preset-id="selectedVideoStylePresetId"
              :selected-template-id="selectedCustomVideoStyleId"
              :content-reference-limit="contentReferenceLimit"
              :saving="styleTemplateSaving"
              @select-preset="selectOfficialVideoStyle"
              @select-template="selectCustomVideoStyle"
              @create-template="createCustomVideoStyleTemplate"
              @delete-template="deleteCustomVideoStyleTemplate"
            />

            <div class="video-reference-pool" data-testid="video-reference-pool">
              <div class="video-reference-head">
                <div>
                  <strong>参考图</strong>
                  <span>最多选择 {{ contentReferenceLimit }} 张</span>
                </div>
                <small>{{ selectedReferenceCount }}/{{ contentReferenceLimit }}</small>
              </div>

              <p v-if="referenceImageValidationMessage" class="video-reference-warning" data-testid="video-reference-required-warning">{{ referenceImageValidationMessage }}</p>

              <div class="video-reference-strip">
                <article
                  v-for="(asset, index) in selectedReferenceAssets"
                  :key="asset.id"
                  class="video-reference-thumb"
                  data-testid="video-reference-thumb"
                  tabindex="0"
                >
                  <button
                    class="video-reference-preview-button"
                    data-testid="video-reference-preview-button"
                    type="button"
                    :aria-label="`预览 ${asset.original_filename || '参考图'}`"
                    @click="openReferencePreview(asset)"
                  >
                    <img :src="asset.preview_url" :alt="asset.original_filename || '参考图'" />
                  </button>
                  <div class="video-reference-actions">
                    <button
                      class="video-reference-action"
                      data-testid="video-reference-delete"
                      type="button"
                      :aria-label="`移除 ${asset.original_filename || '参考图'}`"
                      @click.stop="removeReference(asset.id)"
                    >
                      <Trash2 :size="15" />
                    </button>
                    <button
                      class="video-reference-action"
                      data-testid="video-reference-replace"
                      type="button"
                      :aria-label="`替换 ${asset.original_filename || '参考图'}`"
                      @click.stop="openReferenceReplacement(index)"
                    >
                      <RefreshCw :size="15" />
                    </button>
                  </div>
                  <input
                    :ref="(element) => setReferenceReplacementInput(element, index)"
                    class="video-reference-file-input"
                    data-testid="video-reference-replace-input"
                    type="file"
                    accept="image/jpeg,image/png"
                    @change="replaceReferenceAt(index, $event)"
                  />
                </article>

                <div
                  class="video-reference-upload-slot"
                  :class="{ 'is-empty': selectedReferenceCount === 0 }"
                >
                  <button
                    class="video-reference-upload-button video-reference-dropzone"
                    :class="{ 'is-dragging': referenceDragging }"
                    data-testid="video-reference-dropzone"
                    type="button"
                    :disabled="referenceAtLimit || referenceUploading"
                    @click="openReferenceUpload"
                    @dragenter.prevent="handleReferenceDragEnter"
                    @dragover.prevent="handleReferenceDragEnter"
                    @dragleave.prevent="handleReferenceDragLeave"
                    @drop.prevent="handleReferenceDrop"
                  >
                    <span class="video-reference-dropzone-icon">
                      <Plus :size="26" />
                    </span>
                    <strong>{{ referenceUploading ? '上传中...' : referenceAtLimit ? '已达上限' : selectedReferenceCount ? '继续添加参考图' : '上传参考图' }}</strong>
                    <span>({{ selectedReferenceCount }}/{{ contentReferenceLimit }})</span>
                    <small>点击选择或拖拽到这里</small>
                  </button>
                  <input
                    ref="referenceUploadInput"
                    class="video-reference-file-input"
                    data-testid="video-reference-upload-input"
                    type="file"
                    accept="image/jpeg,image/png"
                    multiple
                    :disabled="referenceAtLimit || referenceUploading"
                    @change="handleReferenceUpload"
                  />
                </div>
              </div>
              <p class="video-reference-help">支持 jpg/png，点击可预览，悬停可替换</p>
            </div>
            <div v-if="selectedVideoCapability.supportsReferenceVideo" class="video-reference-pool" data-testid="video-reference-video-pool">
              <div class="video-reference-head">
                <div>
                  <strong>参考视频</strong>
                  <span>最多选择 {{ referenceVideoLimit }} 个</span>
                </div>
                <small>{{ selectedReferenceVideoCount }}/{{ referenceVideoLimit }}</small>
              </div>

              <div class="video-reference-strip compact">
                <article
                  v-for="asset in selectedReferenceVideoAssets"
                  :key="asset.id"
                  class="video-reference-thumb video-reference-media-thumb"
                  data-testid="video-reference-video-thumb"
                  tabindex="0"
                >
                  <div class="video-reference-media-card">
                    <Play :size="24" />
                    <strong>{{ asset.original_filename || '参考视频' }}</strong>
                  </div>
                  <div class="video-reference-actions">
                    <button
                      class="video-reference-action"
                      data-testid="video-reference-video-delete"
                      type="button"
                      :aria-label="`移除 ${asset.original_filename || '参考视频'}`"
                      @click.stop="removeReferenceVideo(asset.id)"
                    >
                      <Trash2 :size="15" />
                    </button>
                  </div>
                </article>

                <div class="video-reference-upload-slot" :class="{ 'is-empty': selectedReferenceVideoCount === 0 }">
                  <button
                    class="video-reference-upload-button video-reference-dropzone"
                    data-testid="video-reference-video-dropzone"
                    type="button"
                    :disabled="referenceVideoAtLimit || referenceUploading"
                    @click="openReferenceVideoUpload"
                  >
                    <span class="video-reference-dropzone-icon">
                      <Upload :size="24" />
                    </span>
                    <strong>{{ referenceUploading ? '上传中...' : referenceVideoAtLimit ? '已达上限' : selectedReferenceVideoCount ? '继续添加参考视频' : '上传参考视频' }}</strong>
                    <span>({{ selectedReferenceVideoCount }}/{{ referenceVideoLimit }})</span>
                    <small>支持 mp4/webm/mov</small>
                  </button>
                  <input
                    ref="referenceVideoUploadInput"
                    class="video-reference-file-input"
                    data-testid="video-reference-video-upload-input"
                    type="file"
                    accept="video/mp4,video/webm,video/quicktime"
                    multiple
                    :disabled="referenceVideoAtLimit || referenceUploading"
                    @change="handleReferenceVideoUpload"
                  />
                </div>
              </div>
            </div>

            <div v-if="selectedVideoCapability.supportsReferenceAudio" class="video-reference-pool" data-testid="video-reference-audio-pool">
              <div class="video-reference-head">
                <div>
                  <strong>参考音频</strong>
                  <span>最多选择 {{ referenceAudioLimit }} 个，存在音频时自动生成音频</span>
                </div>
                <small>{{ selectedReferenceAudioCount }}/{{ referenceAudioLimit }}</small>
              </div>

              <div class="video-reference-strip compact">
                <article
                  v-for="asset in selectedReferenceAudioAssets"
                  :key="asset.id"
                  class="video-reference-thumb video-reference-media-thumb"
                  data-testid="video-reference-audio-thumb"
                  tabindex="0"
                >
                  <div class="video-reference-media-card">
                    <Music :size="24" />
                    <strong>{{ asset.original_filename || '参考音频' }}</strong>
                  </div>
                  <div class="video-reference-actions">
                    <button
                      class="video-reference-action"
                      data-testid="video-reference-audio-delete"
                      type="button"
                      :aria-label="`移除 ${asset.original_filename || '参考音频'}`"
                      @click.stop="removeReferenceAudio(asset.id)"
                    >
                      <Trash2 :size="15" />
                    </button>
                  </div>
                </article>

                <div class="video-reference-upload-slot" :class="{ 'is-empty': selectedReferenceAudioCount === 0 }">
                  <button
                    class="video-reference-upload-button video-reference-dropzone"
                    data-testid="video-reference-audio-dropzone"
                    type="button"
                    :disabled="referenceAudioAtLimit || referenceUploading"
                    @click="openReferenceAudioUpload"
                  >
                    <span class="video-reference-dropzone-icon">
                      <Music :size="24" />
                    </span>
                    <strong>{{ referenceUploading ? '上传中...' : referenceAudioAtLimit ? '已达上限' : selectedReferenceAudioCount ? '继续添加参考音频' : '上传参考音频' }}</strong>
                    <span>({{ selectedReferenceAudioCount }}/{{ referenceAudioLimit }})</span>
                    <small>支持 mp3/wav/m4a/aac/ogg</small>
                  </button>
                  <input
                    ref="referenceAudioUploadInput"
                    class="video-reference-file-input"
                    data-testid="video-reference-audio-upload-input"
                    type="file"
                    accept="audio/mpeg,audio/wav,audio/mp4,audio/aac,audio/ogg,audio/webm"
                    multiple
                    :disabled="referenceAudioAtLimit || referenceUploading"
                    @change="handleReferenceAudioUpload"
                  />
                </div>
              </div>
            </div>

          </div>
        </div>

        <Teleport to="body">
          <div
            v-if="referencePreview"
            class="video-reference-preview-modal"
            data-testid="video-reference-preview-modal"
            role="dialog"
            aria-modal="true"
            aria-label="参考图预览"
            @click="closeReferencePreview"
          >
            <div class="video-reference-preview-dialog" @click.stop>
              <button
                class="video-reference-preview-close"
                type="button"
                aria-label="关闭参考图预览"
                @click="closeReferencePreview"
              >
                <X :size="20" />
              </button>
              <img :src="referencePreview.preview_url" :alt="referencePreview.original_filename || '参考图预览'" />
              <strong>{{ referencePreview.original_filename || '参考图预览' }}</strong>
            </div>
          </div>
        </Teleport>

        <div class="video-composer-footer">
          <button
            class="primary-button video-submit-button"
            data-testid="video-submit"
            type="button"
            :disabled="!canSubmit"
            @click="submitVideo"
          >
            {{ videoSubmitLabel }}
          </button>
        </div>
      </SoftPanel>

      <SoftPanel class="video-result-panel" tone="default" data-testid="video-result-panel">
        <div class="video-result-stack">
          <div class="video-result-primary">
            <div class="video-section-head compact">
              <div>
                <p class="eyebrow">Result</p>
                <h2>生成结果</h2>
              </div>
              <span data-testid="video-task-status">{{ selectedVideoStatusLabel }}</span>
            </div>

            <div class="video-preview-stage" data-testid="video-preview-stage">
              <div v-if="selectedVideoPreviewURL" :class="['video-player-frame', selectedVideoAspectClass]">
                <video :src="selectedVideoPreviewURL" controls playsinline />
              </div>
              <div v-else class="video-empty-state">
                <strong>{{ hasActiveTask ? taskStatusLabel : '等待视频任务' }}</strong>
                <p>{{ selectedVideoDescription }}</p>
              </div>
            </div>

            <div class="video-result-bottom">
              <div v-if="hasActiveTask" class="video-progress-track" aria-hidden="true">
                <span />
              </div>
              <div v-if="showSoundtrackTools && !selectedHistoryGenerationId" class="video-soundtrack-tools" data-testid="video-soundtrack-tools">
                <div class="video-soundtrack-actions">
                  <button
                    class="secondary-button video-soundtrack-button"
                    data-testid="video-soundtrack-smart"
                    type="button"
                    :disabled="soundtrackBusy"
                    @click="generateSoundtrack('smart')"
                  >
                    <Music :size="16" />
                    <span>{{ soundtrackGenerating ? '生成中...' : '智能配乐' }}</span>
                  </button>
                  <button
                    class="secondary-button video-soundtrack-button"
                    data-testid="video-soundtrack-replace"
                    type="button"
                    :disabled="soundtrackBusy"
                    @click="generateSoundtrack('replace')"
                  >
                    <Shuffle :size="16" />
                    <span>{{ soundtrackGenerating ? '生成中...' : '换一首' }}</span>
                  </button>
                  <button
                    class="secondary-button video-soundtrack-button"
                    data-testid="video-soundtrack-upload"
                    type="button"
                    :disabled="soundtrackBusy"
                    @click="openSoundtrackUpload"
                  >
                    <Upload :size="16" />
                    <span>{{ soundtrackUploading ? '上传中...' : '上传音乐' }}</span>
                  </button>
                  <input
                    ref="soundtrackUploadInput"
                    class="video-soundtrack-file-input"
                    data-testid="video-soundtrack-upload-input"
                    type="file"
                    accept="audio/mpeg,audio/wav,audio/x-wav,audio/mp4,audio/aac,audio/ogg"
                    :disabled="soundtrackBusy"
                    @change="handleSoundtrackUpload"
                  />
                </div>
                <div v-if="soundtrack" class="video-soundtrack-player-box">
                  <div class="video-soundtrack-meta">
                    <strong>{{ soundtrack.title || (soundtrack.source === 'upload' ? '上传音乐' : '智能配乐') }}</strong>
                    <span>{{ soundtrack.source === 'upload' ? '用户上传' : 'AI 生成' }}</span>
                  </div>
                  <audio
                    class="video-soundtrack-player"
                    data-testid="video-soundtrack-player"
                    :src="soundtrack.audio_url"
                    controls
                  />
                  <a
                    v-if="soundtrack.download_url"
                    class="video-soundtrack-download"
                    data-testid="video-soundtrack-download"
                    :href="soundtrack.download_url"
                  >
                    下载音乐
                  </a>
                </div>
              </div>
              <div v-if="selectedVideoDownloadURL" class="video-result-actions">
                <a class="secondary-button" :href="selectedVideoDownloadURL">下载视频</a>
                <a class="secondary-button" href="/works?category=video">查看作品库</a>
              </div>

              <div v-if="selectedVideo?.generation_id" class="video-result-badges">
                <span v-for="tag in visibleEnhancementTags(selectedVideo)" :key="`${selectedVideo.generation_id}-${tag}`" class="video-value-tag">{{ tag }}</span>
                <span v-if="hiddenEnhancementCount(selectedVideo) > 0" class="video-value-tag is-muted">+{{ hiddenEnhancementCount(selectedVideo) }}</span>
              </div>

              <p v-if="message" class="status-success">{{ message }}</p>
              <p v-if="promptValidationMessage || creditValidationMessage || errorMessage || soundtrackError" class="status-error">{{ promptValidationMessage || creditValidationMessage || errorMessage || soundtrackError }}</p>
              <p v-if="loading" class="page-status">加载中...</p>
            </div>
          </div>

          <section class="video-history-panel" data-testid="video-history-panel">
            <div class="video-history-head">
              <div>
                <p class="eyebrow">History</p>
                <h3>历史任务</h3>
              </div>
              <span>{{ historyTotal }} 条</span>
            </div>

            <div class="video-history-filters">
              <label class="video-history-filter">
                <span>搜索</span>
                <div class="video-history-search">
                  <Search :size="14" />
                  <input
                    v-model="historyFilterQuery"
                    data-testid="video-history-search"
                    type="search"
                    placeholder="按提示词筛选"
                    @keydown.enter.prevent="applyHistoryFilters"
                  />
                </div>
              </label>
              <label class="video-history-filter">
                <span>状态</span>
                <select v-model="historyFilterStatus" data-testid="video-history-status-filter" @change="applyHistoryFilters">
                  <option value="all">全部</option>
                  <option value="succeeded">已完成</option>
                  <option value="running">生成中</option>
                  <option value="failed">失败</option>
                </select>
              </label>
              <label class="video-history-filter">
                <span>增强</span>
                <select v-model="historyFilterEnhancement" data-testid="video-history-enhancement-filter" @change="applyHistoryFilters">
                  <option value="all">全部</option>
                  <option value="高清">高清</option>
                  <option value="参考图">参考图</option>
                  <option value="风格模板">风格模板</option>
                  <option value="Pro">Pro</option>
                  <option value="Seedance">Seedance</option>
                  <option value="补帧">补帧</option>
                  <option value="超分">超分</option>
                  <option value="精修">精修</option>
                </select>
              </label>
            </div>

            <div v-if="historyLoading" class="video-history-empty">正在读取历史任务...</div>
            <div v-else-if="historyError" class="video-history-empty">
              <p>{{ historyError }}</p>
              <button class="secondary-button" type="button" @click="loadHistory(1)">重试</button>
            </div>
            <div v-else-if="historyItems.length === 0" class="video-history-empty">
              <p>暂无视频历史，生成完成后会在这里沉淀版本。</p>
            </div>
            <div v-else class="video-history-list">
              <article
                v-for="item in historyItems"
                :key="historyItemKey(item)"
                class="video-history-card"
                :class="{ 'is-selected': Number(selectedHistoryGenerationId) === Number(item.generation_id) }"
                :data-testid="`video-history-card-${item.generation_id}`"
                @click="selectHistoryItem(item)"
              >
                <button
                  type="button"
                  class="video-history-preview"
                  :aria-label="`回看 ${item.prompt_summary || item.prompt}`"
                >
                  <video v-if="item.preview_url" :src="item.preview_url" muted playsinline />
                  <div v-else class="video-history-placeholder"><Play :size="20" /></div>
                </button>
                <div class="video-history-copy">
                  <div class="video-history-copy-head">
                    <strong>{{ item.prompt_summary || item.prompt }}</strong>
                    <span>{{ historyStatusLabel(item.status) }}</span>
                  </div>
                  <p>{{ historyMetaLine(item) }}</p>
                  <div class="video-history-date">
                    <Clock :size="13" />
                    <span>{{ formatHistoryTime(item.created_at) }}</span>
                  </div>
                  <div class="video-history-tags">
                    <span v-for="tag in visibleEnhancementTags(item)" :key="`${item.generation_id}-${tag}`" class="video-value-tag">{{ tag }}</span>
                    <span v-if="hiddenEnhancementCount(item) > 0" class="video-value-tag is-muted">+{{ hiddenEnhancementCount(item) }}</span>
                  </div>
                  <div class="video-history-actions">
                    <button class="mini-button" type="button" :data-testid="`video-history-open-${item.generation_id}`" @click.stop="selectHistoryItem(item)"><Play :size="14" />回看</button>
                    <button class="mini-button" type="button" :data-testid="`video-history-regenerate-${item.generation_id}`" @click.stop="refillComposerFromHistory(item)"><RefreshCw :size="14" />再次生成</button>
                    <button class="mini-button" type="button" :data-testid="`video-history-compare-${item.generation_id}`" @click.stop="openCompareModal(item)"><GitCompare :size="14" />对比</button>
                  </div>
                </div>
              </article>
            </div>
          </section>
        </div>
      </SoftPanel>
    </div>
  </section>

  <Teleport to="body">
    <div v-if="compareModalOpen && compareHistoryItem" class="video-compare-backdrop" data-testid="video-compare-modal" role="dialog" aria-modal="true" @click="closeCompareModal">
      <section class="video-compare-modal" @click.stop>
        <header class="video-compare-head">
          <div>
            <p class="eyebrow">Compare</p>
            <h3>版本对比</h3>
          </div>
          <button class="mini-button icon-only" type="button" aria-label="关闭对比" @click="closeCompareModal">
            <X :size="16" />
          </button>
        </header>

        <div class="video-compare-grid">
          <article class="video-compare-card">
            <strong>当前版本</strong>
            <div v-if="selectedVideoPreviewURL" :class="['video-player-frame', selectedVideoAspectClass]">
              <video :src="selectedVideoPreviewURL" controls playsinline />
            </div>
            <div v-else class="video-empty-state">
              <strong>{{ selectedVideoStatusLabel }}</strong>
              <p>{{ selectedVideoDescription }}</p>
            </div>
            <p>{{ selectedVideoDescription }}</p>
          </article>
          <article class="video-compare-card">
            <strong>历史版本</strong>
            <div v-if="compareHistoryItem.preview_url" :class="['video-player-frame', compareHistoryItem.aspect_ratio === '9:16' ? 'portrait' : 'landscape']">
              <video :src="compareHistoryItem.preview_url" controls playsinline />
            </div>
            <div v-else class="video-empty-state">
              <strong>{{ historyStatusLabel(compareHistoryItem.status) }}</strong>
              <p>{{ compareHistoryItem.error_message || '该历史版本暂无可预览视频。' }}</p>
            </div>
            <p>{{ compareHistoryItem.prompt_summary || compareHistoryItem.prompt }}</p>
          </article>
        </div>

        <div class="video-compare-meta">
          <div>
            <span>当前</span>
            <strong>{{ selectedVideoStatusLabel }}</strong>
          </div>
          <div>
            <span>历史</span>
            <strong>{{ historyStatusLabel(compareHistoryItem.status) }}</strong>
          </div>
          <div>
            <span>增强</span>
            <strong>{{ (compareHistoryItem.enhancement_tags || []).join(' / ') || '无' }}</strong>
          </div>
        </div>

        <div class="video-compare-actions">
          <button class="secondary-button" data-testid="video-compare-use-history" type="button" @click="useCompareHistoryVersion">设为当前预览</button>
          <button class="primary-button" type="button" @click="closeCompareModal">关闭</button>
        </div>
      </section>
    </div>
  </Teleport>
</template>
