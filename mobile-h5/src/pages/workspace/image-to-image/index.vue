<script setup>
import { computed, onBeforeUnmount, ref } from 'vue'
import { onLoad } from '@dcloudio/uni-app'

import { api } from '../../../api/client.js'
import AnnouncementPopup from '../../../components/AnnouncementPopup.vue'
import AppTabbar from '../../../components/AppTabbar.vue'
import {
  addPendingGenerations,
  removePendingGenerations,
  stageProgress,
  updatePendingGeneration
} from '../../../utils/generation-tasks.js'
import { enableMiniProgramShare, navigateTo, requireAuth, routes } from '../../../utils/routes.js'

const staticAssetBaseURL = `${import.meta.env.VITE_STATIC_ASSET_BASE_URL || ''}`.replace(/\/+$/, '')

function staticAsset(path) {
  const normalizedPath = `${path || ''}`.trim().replace(/^\/+/, '').replace(/^static\/+/i, '')
  if (!normalizedPath) return staticAssetBaseURL
  if (staticAssetBaseURL) return `${staticAssetBaseURL}/${normalizedPath}`
  return `/${['static', normalizedPath].join('/')}`
}

function staticIcon(name) {
  const normalizedName = `${name || ''}`.trim().replace(/\.png$/i, '')
  return staticAsset(`icons/${normalizedName}.png`)
}

const icon = staticIcon
const blockedPlainTextImageSources = ['开始生成']

enableMiniProgramShare({
  title: '白霖共享 AI 生图工作台',
  path: routes.imageToImage
})

function normalizeImageSource(value) {
  const source = `${value || ''}`.trim()
  if (!source || blockedPlainTextImageSources.includes(source)) return ''
  if (/^(https?:|wxfile:|cloud:|blob:|data:image\/)/i.test(source)) return source
  if (/^\/(api|static|tmp|usr|store_|wxfile)/i.test(source)) return source
  return ''
}

const sizePresets = [
  { value: '1:1', label: '朋友圈/电商图', ratio: '1:1', pixels: '1024×1024' },
  { value: '3:4', label: '小红书封面', ratio: '3:4', pixels: '768×1024' },
  { value: '4:3', label: '横版配图', ratio: '4:3', pixels: '1024×768' },
  { value: '9:16', label: '手机海报', ratio: '9:16', pixels: '576×1024' },
  { value: '16:9', label: '视频封面', ratio: '16:9', pixels: '1024×576' }
]

const stylePresets = [
  { value: '', label: '无风格', icon: icon('style') },
  { value: '写实', label: '写实', icon: icon('photo') },
  { value: '插画', label: '插画', icon: icon('illustration') },
  { value: '漫画', label: '漫画', icon: icon('manga') },
  { value: '电商', label: '电商', icon: icon('ecommerce') },
  { value: '国风', label: '国风', icon: icon('guofeng') },
  { value: '赛博朋克', label: '赛博朋克', icon: icon('style') },
  { value: '3D渲染', label: '3D渲染', icon: icon('image-image') },
  { value: '自定义', label: '自定义', icon: icon('custom') }
]

const modeOptions = [
  { key: 'text', label: '文生图', description: '用提示词直接生成图片' },
  { key: 'image', label: '图生图', description: '上传参考图后重绘或改风格' }
]

const qualityOptions = [
  { key: 'medium', label: '标准' },
  { key: 'high', label: '高清' }
]

const prompt = ref('')
const negativePrompt = ref('')
const aspectRatio = ref('1:1')
const selectedSizePresetIndex = ref(0)
const quality = ref('high')
const variationMode = ref('balanced')
const stylePreset = ref('')
const references = ref([])
const submitting = ref(false)
const generationTasks = ref([])
const taskMessage = ref('')
const taskError = ref('')
const generationErrorCode = ref('')
const creditShortfall = ref(null)
const lastRequestPayload = ref(null)
const availableCredits = ref(null)
const showModeSheet = ref(false)
const showHistorySheet = ref(false)
const showTemplateSheet = ref(false)
const activeMode = ref('text')
const historyItems = ref([])
const historyLoading = ref(false)
const historyError = ref('')
const applyingHistoryWorkID = ref(null)
const templateItems = ref([])
const templateLoading = ref(false)
const templateError = ref('')
const usingTemplateID = ref(null)
const importedGenerationParams = ref({})
const promptOptimizerMode = ref('balanced')
const promptOptimizerRunning = ref(false)
const promptOptimizerError = ref('')

const promptOptimizationModes = [
  { key: 'balanced', label: '通用优化', description: '补齐画面主体、构图与光影' },
  { key: 'commercial', label: '商业出图', description: '适合商品图、海报和封面' },
  { key: 'detail', label: '细节增强', description: '强化镜头、环境和质感' },
  { key: 'portrait_detail', label: '人脸高清', description: '强化毛孔、绒毛、皮肤质感和摄影光影' },
  { key: 'safe', label: '安全改写', description: '优先规避生成失败风险' }
]

const promptOptimizerModeLabels = computed(() => promptOptimizationModes.map((mode) => mode.label))
const selectedPromptOptimizerMode = computed(
  () => promptOptimizationModes.find((mode) => mode.key === promptOptimizerMode.value) || promptOptimizationModes[0]
)
const promptOptimizerModeIndex = computed(() =>
  Math.max(0, promptOptimizationModes.findIndex((mode) => mode.key === promptOptimizerMode.value))
)

const variationModeOptions = [
  { key: 'stable', label: '稳定变化', description: '主体接近，只微调光影和细节' },
  { key: 'balanced', label: '均衡变化', description: '默认推荐，构图、角度和光影都有差异' },
  { key: 'bold', label: '大胆变化', description: '背景、构图和视觉冲击更明显' }
]

const variationPrompts = {
  stable: [
    '保持主体和构图接近，微调光影层次、材质细节和画面节奏',
    '保持同一商业方向，调整阴影、高光和局部细节，让成片略有区别',
    '保持主体占比稳定，轻微改变镜头距离、背景虚实和色彩层次',
    '保持整体风格一致，优化细节密度、边缘质感和画面完成度'
  ],
  balanced: [
    '正面构图，主体居中，干净商业摄影风格',
    '轻微俯拍角度，加入自然阴影和层次感',
    '侧向构图，突出产品材质和高光细节',
    '更强视觉冲击，适合广告首图排版'
  ],
  bold: [
    '更换为更有记忆点的背景和构图，保留核心主体但增强视觉冲击',
    '采用更大胆的镜头角度、明暗对比和空间层次，形成明显不同方案',
    '加入更强的场景氛围和广告感排版，突出可用于封面首图的效果',
    '用更鲜明的风格化表达重组画面，保留需求主题但扩大创意差异'
  ]
}

let pollTimer = null

const modeTitle = computed(() => modeOptions.find((item) => item.key === activeMode.value)?.label || '文生图')
const referenceSlots = computed(() =>
  Array.from({ length: 4 }, (_, index) => ({
    index,
    item: references.value[index] || null
  }))
)
const canUploadMore = computed(() => references.value.length < 4 && !submitting.value)
const uploadedReferenceIds = computed(() =>
  references.value
    .filter((item) => item.serverId)
    .map((item) => item.serverId)
)
const uploadedReferenceWorkIds = computed(() =>
  references.value
    .filter((item) => item.workId)
    .map((item) => item.workId)
)
const hasUploadedReference = computed(() => uploadedReferenceIds.value.length > 0 || uploadedReferenceWorkIds.value.length > 0)
const hasUploadingReferences = computed(() => references.value.some((item) => item.uploading))
const referenceTitle = computed(() => (activeMode.value === 'image' ? '参考图片' : '参考图片（可选）'))
const referenceHint = computed(() =>
  activeMode.value === 'image'
    ? '至少上传 1 张参考图，最多 4 张；上传成功后再开始生成。'
    : '文生图不需要上传图片，直接输入提示词即可生成。'
)
const uploadedReferenceCount = computed(() => uploadedReferenceIds.value.length + uploadedReferenceWorkIds.value.length)
const uploadedReferenceCreditCost = computed(() => (activeMode.value === 'image' ? uploadedReferenceCount.value : 0))
const singleGenerationCreditCost = 1
const estimatedGenerationCredits = computed(() => singleGenerationCreditCost + uploadedReferenceCreditCost.value)
const creditCostHint = computed(() => {
  if (activeMode.value === 'image') {
    return `预计消耗 ${estimatedGenerationCredits.value} 点（生成 1 张 + 参考图 ${uploadedReferenceCount.value} 张）`
  }
  return `预计消耗 ${estimatedGenerationCredits.value} 点`
})
const sizePresetLabels = computed(() => sizePresets.map(formatSizePreset))
const selectedSizePreset = computed(() => sizePresets[selectedSizePresetIndex.value] || sizePresets[4])
function formatSizePreset(item) {
  return `${item.label} · ${item.ratio} · ${item.pixels}`
}

function showToast(title) {
  uni.showToast({ title, icon: 'none' })
}

const insufficientCreditsCode = 'credits_insufficient'
const insufficientCreditsMessage = '点数不足，请先充值后再生成'

function isInsufficientCreditsError(error) {
  return error?.code === insufficientCreditsCode || error?.error?.code === insufficientCreditsCode
}

function normalizeCreditEstimatePayload(payload = {}) {
  const source = payload?.error && !payload.required_credits ? { ...payload, ...payload.error } : payload
  const requiredCredits = Number(source?.required_credits ?? 0)
  const availableCreditsValue = Number(source?.available_credits ?? 0)
  const missingCredits = Number(source?.missing_credits ?? Math.max(requiredCredits - availableCreditsValue, 0))
  return {
    required_credits: Number.isFinite(requiredCredits) ? requiredCredits : 0,
    available_credits: Number.isFinite(availableCreditsValue) ? availableCreditsValue : 0,
    missing_credits: Number.isFinite(missingCredits) ? missingCredits : 0,
    enough: source?.enough === undefined ? missingCredits <= 0 : Boolean(source.enough),
    recommended_package: source?.recommended_package || null
  }
}

function recommendedPackageName(estimate) {
  return `${estimate?.recommended_package?.name || ''}`.trim()
}

function insufficientCreditsDetailText(estimate) {
  const packageName = recommendedPackageName(estimate)
  const suffix = packageName ? `，推荐套餐「${packageName}」` : ''
  return `点数不足，本次预计消耗 ${estimate.required_credits} 点，当前余额 ${estimate.available_credits} 点，还差 ${estimate.missing_credits} 点${suffix}`
}

function showInsufficientCreditsError(message = insufficientCreditsMessage, estimatePayload = null) {
  const estimate = estimatePayload ? normalizeCreditEstimatePayload(estimatePayload) : null
  creditShortfall.value = estimate
  generationErrorCode.value = insufficientCreditsCode
  taskMessage.value = ''
  taskError.value = estimate ? insufficientCreditsDetailText(estimate) : message
  if (estimate) {
    availableCredits.value = estimate.available_credits
  }
  showToast(message)
}

function applyInsufficientCreditsEstimate(estimatePayload) {
  const estimate = normalizeCreditEstimatePayload(estimatePayload)
  showInsufficientCreditsError(insufficientCreditsMessage, estimate)
}

function goPricing() {
  const estimate = creditShortfall.value
  navigateTo(routes.pricing, {
    missing_credits: estimate?.missing_credits,
    required_credits: estimate?.required_credits,
    package_id: estimate?.recommended_package?.id,
    source: activeMode.value === 'image' ? 'image_to_image' : 'text_to_image'
  })
}

function openHistory() {
  showHistorySheet.value = true
  void loadHistoryWorks()
}

function closeHistory() {
  if (applyingHistoryWorkID.value) return
  showHistorySheet.value = false
}

function openPromptTemplates() {
  showTemplateSheet.value = true
  if (templateItems.value.length === 0) {
    void loadPromptTemplates()
  }
}

function closePromptTemplates() {
  if (usingTemplateID.value) return
  showTemplateSheet.value = false
}

function normalizeTemplatePayload(payload) {
  if (Array.isArray(payload)) return payload
  if (Array.isArray(payload?.items)) return payload.items
  if (Array.isArray(payload?.templates)) return payload.templates
  if (Array.isArray(payload?.data)) return payload.data
  return []
}

function templatePreviewURL(template) {
  const previewURL = `${template?.preview_url || ''}`.trim()
  if (!previewURL) return ''
  return normalizeImageSource(api.assetURL(previewURL))
}

function previewPromptTemplate(template) {
  const previewURL = templatePreviewURL(template)
  if (!previewURL) {
    showToast('暂无预览图')
    return
  }
  uni.previewImage({
    urls: [previewURL],
    current: previewURL,
    fail() {
      showToast('预览图打开失败')
    }
  })
}

function isUsingTemplate(template) {
  return Number(usingTemplateID.value) === Number(template?.id)
}

async function loadPromptTemplates() {
  templateLoading.value = true
  templateError.value = ''
  try {
    const payload = await api.listPromptTemplates()
    templateItems.value = normalizeTemplatePayload(payload)
  } catch (error) {
    templateItems.value = []
    templateError.value = error.message || '提示词模板读取失败'
  } finally {
    templateLoading.value = false
  }
}

async function usePromptTemplate(template) {
  const templateID = Number(template?.id)
  if (!Number.isFinite(templateID) || templateID <= 0 || usingTemplateID.value) return
  usingTemplateID.value = templateID
  try {
    const payload = await api.usePromptTemplate(templateID)
    const nextPrompt = `${payload.prompt || ''}`.trim()
    if (!nextPrompt) {
      throw new Error('模板提示词为空')
    }
    prompt.value = nextPrompt
    negativePrompt.value = ''
    activeMode.value = 'text'
    setAspectRatioFromPresetValue(payload.aspect_ratio)
    stylePreset.value = `${payload.style_preset || ''}`.trim()
    if (payload.available_credits !== undefined) {
      availableCredits.value = payload.available_credits
    }
    showTemplateSheet.value = false
    showToast('已使用模板，扣除 1 点')
  } catch (error) {
    if (isInsufficientCreditsError(error)) {
      showInsufficientCreditsError('点数不足，无法使用模板')
      return
    }
    templateError.value = error.message || '模板使用失败'
    showToast(templateError.value)
  } finally {
    usingTemplateID.value = null
  }
}

function setAspectRatioFromPresetValue(value) {
  const nextValue = `${value || ''}`.trim()
  const exactIndex = sizePresets.findIndex((item) => item.value === nextValue && item.ratio === nextValue)
  const fallbackIndex = sizePresets.findIndex((item) => item.value === nextValue)
  const nextIndex = exactIndex >= 0 ? exactIndex : fallbackIndex
  if (nextIndex >= 0) {
    selectedSizePresetIndex.value = nextIndex
    aspectRatio.value = sizePresets[nextIndex].value
    return
  }
  selectedSizePresetIndex.value = 4
  aspectRatio.value = '1:1'
}

function selectSizePreset(event) {
  if (submitting.value) return
  const index = Number(event?.detail?.value ?? event)
  if (!Number.isInteger(index) || !sizePresets[index]) return
  selectedSizePresetIndex.value = index
  aspectRatio.value = sizePresets[index].value
}

function selectQuality(value) {
  if (submitting.value) return
  if (qualityOptions.some((item) => item.key === value)) {
    quality.value = value
  }
}

function decodeQueryValue(value) {
  if (typeof value !== 'string') return ''
  try {
    return decodeURIComponent(value)
  } catch {
    return value
  }
}

function prefillFromQuery(query = {}) {
  const nextPrompt = decodeQueryValue(query.prompt).trim()
  const nextAspect = decodeQueryValue(query.aspect).trim()
  const nextMode = decodeQueryValue(query.mode).trim()
  if (nextPrompt) {
    prompt.value = nextPrompt
  }
  if (nextMode === 'text' || nextMode === 'image') {
    activeMode.value = nextMode
  }
  if (nextAspect) {
    setAspectRatioFromPresetValue(nextAspect)
  }
}

function normalizeWorksPayload(payload) {
  if (Array.isArray(payload)) return payload
  if (Array.isArray(payload?.items)) return payload.items
  if (Array.isArray(payload?.works)) return payload.works
  if (Array.isArray(payload?.data)) return payload.data
  return []
}

function workID(work) {
  return work?.work_id || work?.id
}

function workCover(work) {
  return normalizeImageSource(work?.preview_url) ||
    normalizeImageSource(work?.download_url) ||
    normalizeImageSource(work?.thumbnail_url) ||
    normalizeImageSource(work?.cover_url) ||
    ''
}

function promptExcerpt(work) {
  const text = work?.prompt || work?.input_prompt || work?.title || '未命名作品'
  return text.length > 42 ? `${text.slice(0, 42)}...` : text
}

function formatHistoryTime(value) {
  if (!value) return '刚刚'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return `${value}`
  return `${date.getMonth() + 1}/${date.getDate()} ${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`
}

function numericOrDefault(value, fallback) {
  const next = Number(value)
  return Number.isFinite(next) ? next : fallback
}

function referenceFromReuseAsset(asset, index) {
  const previewURL = normalizeImageSource(asset.preview_url || asset.previewUrl)
  return {
    id: `history-${asset.id || index}`,
    path: previewURL,
    previewUrl: previewURL,
    displayPreviewUrl: previewURL,
    remotePreviewUrl: previewURL,
    name: asset.original_filename || asset.name || `历史参考图${index + 1}`,
    file: null,
    serverId: asset.id || asset.reference_asset_id || null,
    uploading: false,
    error: previewURL ? '' : '预览缺失'
  }
}

function referenceFromReuseWork(work, index) {
  const previewURL = normalizeImageSource(work.preview_url || work.previewUrl)
  return {
    id: `work-${work.id || work.work_id || index}`,
    path: previewURL,
    previewUrl: previewURL,
    displayPreviewUrl: previewURL,
    remotePreviewUrl: previewURL,
    name: work.title || `作品参考图${index + 1}`,
    file: null,
    serverId: null,
    workId: work.id || work.work_id || null,
    uploading: false,
    error: previewURL ? '' : '预览缺失'
  }
}

function applyReusePayload(payload = {}) {
  const reusePrompt = `${payload.prompt || ''}`.trim()
  prompt.value = reusePrompt
  negativePrompt.value = `${payload.negative_prompt || ''}`.trim()
  setAspectRatioFromPresetValue(payload.aspect_ratio)
  stylePreset.value = `${payload.style_preset || ''}`.trim()

  const referenceWorks = Array.isArray(payload.reference_works) ? payload.reference_works.slice(0, 4) : []
  const referenceAssets = Array.isArray(payload.reference_assets) ? payload.reference_assets.slice(0, 4) : []
  references.value = [
    ...referenceWorks.map(referenceFromReuseWork).filter((item) => item.workId),
    ...referenceAssets.map(referenceFromReuseAsset).filter((item) => item.serverId)
  ].slice(0, 4)
  activeMode.value = references.value.length > 0 ? 'image' : 'text'
  importedGenerationParams.value = {
    tool_mode: payload.tool_mode || 'generate',
    style_strength: numericOrDefault(payload.style_strength, 65),
    reference_weight: numericOrDefault(payload.reference_weight, 75),
    seed: `${payload.seed || ''}`.trim(),
    source_work_id: payload.source_work_id || null
  }

  const toolMode = `${payload.tool_mode || 'generate'}`.trim()
  showToast(toolMode && toolMode !== 'generate' ? '已导入可复用参数' : '已导入历史提示词')
}

async function importReuseWorkByID(id, options = {}) {
  const workIDValue = Number(id)
  if (!Number.isFinite(workIDValue) || workIDValue <= 0) {
    showToast('历史作品无效')
    return
  }
  applyingHistoryWorkID.value = workIDValue
  try {
    const payload = await api.reuseWork(workIDValue)
    applyReusePayload(payload)
    if (options.closeHistory !== false) {
      showHistorySheet.value = false
    }
  } catch (error) {
    const message = error.message || '历史提示词导入失败'
    historyError.value = message
    showToast(message)
  } finally {
    applyingHistoryWorkID.value = null
  }
}

async function importHistoryWork(work) {
  const id = workID(work)
  if (!id) {
    showToast('生成完成后可导入')
    return
  }
  await importReuseWorkByID(id)
}

function isApplyingHistoryWork(work) {
  return Number(applyingHistoryWorkID.value) === Number(workID(work))
}

async function loadHistoryWorks() {
  historyLoading.value = true
  historyError.value = ''
  try {
    const payload = await api.listWorks({ category: 'image', status: 'succeeded', page: 1, page_size: 20 })
    historyItems.value = normalizeWorksPayload(payload)
  } catch (error) {
    historyItems.value = []
    historyError.value = error.message || '历史提示词读取失败'
  } finally {
    historyLoading.value = false
  }
}

function setStyle(value) {
  if (submitting.value) return
  stylePreset.value = stylePreset.value === value ? '' : value
}

function setVariationMode(value) {
  if (submitting.value) return
  if (variationModeOptions.some((item) => item.key === value)) {
    variationMode.value = value
  }
}

function variationPromptForIndex(index, mode) {
  const prompts = variationPrompts[mode] || variationPrompts.balanced
  return prompts[index % prompts.length]
}

function fileNameFromPath(path, index) {
  const fallback = `参考图${index + 1}.png`
  if (!path) return fallback
  return decodeURIComponent(path.split('/').pop() || fallback)
}

function patchReference(id, patch) {
  const index = references.value.findIndex((item) => item.id === id)
  if (index < 0) return
  references.value.splice(index, 1, { ...references.value[index], ...patch })
}

async function uploadReference(item) {
  patchReference(item.id, { uploading: true, error: '' })
  try {
    const uploaded = await api.uploadReferenceAsset({
      path: item.path,
      file: item.file
    })
    if (!uploaded.id) {
      throw new Error('参考图上传响应无效')
    }
    patchReference(item.id, {
      serverId: uploaded.id,
      remotePreviewUrl: uploaded.preview_url || '',
      displayPreviewUrl: normalizeImageSource(uploaded.preview_url) || item.path,
      name: uploaded.original_filename || item.name,
      uploading: false,
      error: ''
    })
  } catch (error) {
    const message = error.message || '上传失败'
    patchReference(item.id, {
      uploading: false,
      error: message
    })
    taskError.value = message
    showToast(message)
  }
}

function handleReferenceImageError(item) {
  if (!item?.id) return
  if (item.displayPreviewUrl !== item.previewUrl && item.previewUrl) {
    patchReference(item.id, { displayPreviewUrl: item.previewUrl })
  }
}

function chooseReferences() {
  if (!canUploadMore.value) return

  uni.chooseImage({
    count: 4 - references.value.length,
    sizeType: ['compressed', 'original'],
    sourceType: ['album', 'camera'],
    success(result) {
      const paths = result.tempFilePaths || []
      const files = result.tempFiles || []
      paths.forEach((path, index) => {
        const file = files[index] || {}
        const name = file.name || fileNameFromPath(path, references.value.length + index)
        const ref = {
          id: `${Date.now()}-${index}-${Math.random()}`,
          path,
          previewUrl: path,
          displayPreviewUrl: path,
          remotePreviewUrl: '',
          name,
          file,
          serverId: null,
          uploading: false,
          error: ''
        }
        references.value.push(ref)
        void uploadReference(ref)
      })
    }
  })
}

function removeReference(id) {
  if (submitting.value) return
  references.value = references.value.filter((item) => item.id !== id)
}

function clearReferences() {
  if (submitting.value) return
  references.value = []
}

function selectPromptOptimizerMode(event) {
  if (promptOptimizerRunning.value || submitting.value) return
  const index = Number(event?.detail?.value)
  if (!Number.isInteger(index) || !promptOptimizationModes[index]) return
  promptOptimizerMode.value = promptOptimizationModes[index].key
  promptOptimizerError.value = ''
}

async function optimizePrompt() {
  if (submitting.value) return
  if (!prompt.value.trim()) {
    showToast('请先输入提示词')
    return
  }
  const sourcePrompt = prompt.value.trim()
  if (!sourcePrompt || promptOptimizerRunning.value) return
  promptOptimizerRunning.value = true
  promptOptimizerError.value = ''
  try {
    const payload = await api.optimizePrompt({
      prompt: sourcePrompt,
      mode: promptOptimizerMode.value,
      aspect_ratio: aspectRatio.value,
      style_preset: stylePreset.value
    })
    const optimized = `${payload.optimized_prompt || ''}`.trim()
    if (!optimized) {
      promptOptimizerError.value = '未返回可用提示词，请调整后重试'
      showToast(promptOptimizerError.value)
      return
    }
    prompt.value = optimized
    showToast('提示词已优化')
  } catch (error) {
    promptOptimizerError.value = error.message || '提示词优化失败'
    showToast(promptOptimizerError.value)
  } finally {
    promptOptimizerRunning.value = false
  }
}

function randomInspiration() {
  const ideas = [
    '山谷日落，奇幻写实风格，远处云层透出暖光，层次丰富',
    '香水瓶漂浮在清透水面上，柔和高光，电商广告质感',
    '角色半身重绘，精致服饰，冷暖对比光，插画风格',
    '古风建筑雨后夜景，灯笼暖光，电影级构图'
  ]
  prompt.value = ideas[Math.floor(Math.random() * ideas.length)]
}

function chooseTextMode() {
  showModeSheet.value = false
  activeMode.value = 'text'
}

function chooseImageMode() {
  showModeSheet.value = false
  activeMode.value = 'image'
}

function openCoupleAlbumMode() {
  showModeSheet.value = false
  navigateTo(routes.coupleAlbumCreate)
}

function openChildhoodDreamAlbumMode() {
  showModeSheet.value = false
  navigateTo(routes.coupleAlbumCreate, { mode: 'childhood-dream' })
}

function stopPolling() {
  if (pollTimer !== null) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

function currentReferencePreviewUrls() {
  if (activeMode.value !== 'image') return []
  return references.value
    .map((item) =>
      normalizeImageSource(item.remotePreviewUrl) ||
      normalizeImageSource(item.displayPreviewUrl) ||
      normalizeImageSource(item.previewUrl) ||
      item.path
    )
    .filter(Boolean)
}

function buildPendingGenerationTask(task, requestPayload, createdAt) {
  return {
    generation_id: task.generation_id,
    status: task.status || 'queued',
    stage: task.stage || 'queued',
    batch_id: task.batch_id || requestPayload.batch_id || '',
    batch_index: Number.isFinite(Number(task.batch_index ?? requestPayload.batch_index))
      ? Number(task.batch_index ?? requestPayload.batch_index)
      : 0,
    batch_total: Number.isFinite(Number(task.batch_total ?? requestPayload.batch_total))
      ? Number(task.batch_total ?? requestPayload.batch_total)
      : 1,
    variation_mode: task.variation_mode || requestPayload.variation_mode || '',
    variation_prompt: task.variation_prompt || requestPayload.variation_prompt || '',
    seed: task.seed || requestPayload.seed || '',
    progress: stageProgress(task.stage, task.status),
    prompt: requestPayload.prompt,
    negative_prompt: requestPayload.negative_prompt || '',
    aspect_ratio: aspectRatio.value,
    style_preset: requestPayload.style_preset,
    reference_asset_ids: requestPayload.reference_asset_ids || [],
    reference_work_ids: requestPayload.reference_work_ids || [],
    reference_preview_urls: requestPayload.reference_asset_ids?.length || requestPayload.reference_work_ids?.length ? currentReferencePreviewUrls() : [],
    image_count: 1,
    mode: activeMode.value,
    tool_mode: 'generate',
    created_at: createdAt
  }
}

function applyOptionalStylePayload(payload) {
  if (stylePreset.value) {
    payload.style_preset = stylePreset.value
    payload.style_strength = numericOrDefault(importedGenerationParams.value.style_strength, 65)
  }
  return payload
}

async function pollGenerations(ids) {
  try {
    const payloads = await Promise.all(ids.map((id) => api.getImageGeneration(id)))
    generationTasks.value = payloads
    payloads.forEach((payload) => {
      if (!payload?.generation_id) return
      const currentTask = generationTasks.value.find((task) => task.generation_id === payload.generation_id) || {}
      updatePendingGeneration(payload.generation_id, {
        ...payload,
        batch_id: payload.batch_id || currentTask.batch_id || '',
        batch_index: Number.isFinite(Number(payload.batch_index))
          ? Number(payload.batch_index)
          : Number(currentTask.batch_index) || 0,
        batch_total: Number.isFinite(Number(payload.batch_total)) && Number(payload.batch_total) > 0
          ? Number(payload.batch_total)
          : Number(currentTask.batch_total) || 1,
        progress: stageProgress(payload.stage, payload.status)
      })
    })

    const lastPayload = payloads[payloads.length - 1]
    if (lastPayload?.available_credits !== undefined) {
      availableCredits.value = lastPayload.available_credits
    }

    const failed = payloads.find((payload) => payload.status === 'failed')
    if (failed) {
      stopPolling()
      submitting.value = false
      generationErrorCode.value = failed?.error?.code || ''
      taskError.value = failed?.error?.message || '生成失败，请稍后再试'
      updatePendingGeneration(failed.generation_id, {
        status: 'failed',
        stage: failed.stage,
        progress: 100,
        error: failed?.error?.message || '生成失败，请稍后再试'
      })
      return
    }

    const allCompleted = payloads.every((payload) => payload.status === 'succeeded')
    if (allCompleted) {
      stopPolling()
      submitting.value = false
      taskMessage.value = '生成完成，作品已写入作品库'
      showToast('生成完成')
      removePendingGenerations(ids)
      return
    }

    taskMessage.value = '任务处理中'
  } catch (error) {
    stopPolling()
    submitting.value = false
    generationErrorCode.value = error?.code || ''
    taskError.value = error.message || '任务查询失败'
  }
}

function startPolling(ids) {
	stopPolling()
	pollTimer = setInterval(() => {
		void pollGenerations(ids)
	}, 1200)
}

function replaceGenerationTask(payload) {
	if (!payload?.generation_id) return
	const index = generationTasks.value.findIndex((task) => task.generation_id === payload.generation_id)
	if (index >= 0) {
		generationTasks.value.splice(index, 1, payload)
		return
	}
	generationTasks.value.push(payload)
}

async function createSingleGeneration(requestPayload) {
  stopPolling()
  generationTasks.value = []
  taskMessage.value = '任务已提交，正在生成'
  const created = await api.createImageGeneration(requestPayload)
  replaceGenerationTask(created)
  if (!created?.generation_id) {
    throw new Error('生成任务创建失败')
  }
  const pendingTask = buildPendingGenerationTask(created, requestPayload, new Date().toISOString())
  addPendingGenerations([pendingTask])
  taskMessage.value = '任务已提交，正在生成'
  startPolling([created.generation_id])
}

async function ensureUploadedReferences() {
	const failed = references.value.filter((item) => item.error && !item.serverId)
	await Promise.all(failed.map((item) => uploadReference(item)))
	return references.value.every((item) => !item.error && !item.uploading)
}

async function submitGeneration(options = {}) {
  taskError.value = ''
  taskMessage.value = ''
  generationErrorCode.value = ''
  creditShortfall.value = null

  if (activeMode.value === 'image' && references.value.length === 0) {
    showToast('请至少上传1张参考图')
    taskError.value = '图生图需要至少 1 张参考图'
    return
  }

  if (activeMode.value === 'image' && hasUploadingReferences.value) {
    showToast('参考图正在上传，请稍候')
    return
  }

  if (activeMode.value === 'image') {
    const uploadReady = await ensureUploadedReferences()
    if (!uploadReady) {
      taskError.value = '参考图上传失败，请删除后重新上传'
      return
    }

    if (!hasUploadedReference.value) {
      showToast('请至少上传1张参考图')
      taskError.value = '图生图需要至少 1 张上传成功的参考图'
      return
    }
  }

  const cleanPrompt = `${options.promptOverride ?? prompt.value}`.trim()
  if (!cleanPrompt) {
    showToast('请输入提示词')
    return
  }

  submitting.value = true
  try {
    const requestPayload = {
      prompt: cleanPrompt,
      negative_prompt: negativePrompt.value.trim() || undefined,
      aspect_ratio: aspectRatio.value,
      quality: quality.value,
      reference_weight: numericOrDefault(importedGenerationParams.value.reference_weight, 75),
      reference_asset_ids: uploadedReferenceIds.value,
      reference_work_ids: uploadedReferenceWorkIds.value,
      tool_mode: 'generate',
      variation_mode: variationMode.value,
      variation_prompt: variationPromptForIndex(0, variationMode.value)
    }
    applyOptionalStylePayload(requestPayload)
    if (importedGenerationParams.value.seed) {
      requestPayload.seed = importedGenerationParams.value.seed
    }
    if (activeMode.value === 'image' && uploadedReferenceCount.value >= 2) {
      requestPayload.reference_intent = 'compose'
      delete requestPayload.variation_mode
      delete requestPayload.variation_prompt
      delete requestPayload.seed
    }
    if (activeMode.value === 'text') {
      delete requestPayload.reference_asset_ids
      delete requestPayload.reference_work_ids
    }
    lastRequestPayload.value = { ...requestPayload }
    const estimate = normalizeCreditEstimatePayload(await api.estimateImageGeneration(requestPayload))
    availableCredits.value = estimate.available_credits
    if (!estimate.enough) {
      applyInsufficientCreditsEstimate(estimate)
      submitting.value = false
      return
    }
    showToast('已开始生成')
    await createSingleGeneration(requestPayload)
  } catch (error) {
    submitting.value = false
    if (isInsufficientCreditsError(error)) {
      applyInsufficientCreditsEstimate(error)
      return
    }
    generationErrorCode.value = error?.code || ''
    taskError.value = error.message || '生成请求失败'
  }
}

function retryLastGeneration() {
  if (submitting.value || !lastRequestPayload.value) return
  void submitGeneration()
}

onBeforeUnmount(() => {
	submitting.value = false
	stopPolling()
})
onLoad((query) => {
  prefillFromQuery(query)
  void requireAuth().then((user) => {
    if (!user) return
    if (user?.available_credits !== undefined) {
      availableCredits.value = user.available_credits
    }
    const reuseWorkID = decodeQueryValue(query?.reuse_work_id).trim()
    if (reuseWorkID) {
      void importReuseWorkByID(reuseWorkID, { closeHistory: false })
    }
  })
})
</script>

<template>
  <view class="workspace-page">
    <view class="app-shell">
      <view class="topbar">
        <view class="brand">
          <image class="brand-icon" :src="icon('logo-star')" mode="aspectFit" />
          <view>
            <text class="brand-name">白霖共享</text>
            <text class="brand-subtitle">创作者 AI 图片平台</text>
          </view>
        </view>
        <button class="history-button" type="button" @click="openHistory">
          <image :src="icon('history')" mode="aspectFit" />
        </button>
      </view>

      <view class="mode-tabs">
        <view class="mode-label">
          <text>工作模式</text>
        </view>
        <button class="mode-current active" type="button" @click="showModeSheet = true">
          <text>{{ modeTitle }}</text>
          <text class="chevron">⌄</text>
        </button>
      </view>

      <view v-if="activeMode === 'image'" class="section-head reference-head">
        <view>
          <text class="section-title">{{ referenceTitle }}</text>
          <text class="help-dot">?</text>
        </view>
        <button type="button" class="clear-button" @click="clearReferences">
          <image :src="icon('delete')" mode="aspectFit" />
          <text>清空</text>
        </button>
      </view>

      <view v-if="activeMode === 'image'" class="reference-panel">
        <view
          v-for="slot in referenceSlots"
          :key="slot.index"
          class="reference-slot"
          :class="{ filled: slot.item }"
          @click="slot.item ? undefined : chooseReferences()"
        >
          <template v-if="slot.item">
            <image
              :key="slot.item.displayPreviewUrl || slot.item.remotePreviewUrl || slot.item.previewUrl"
              class="reference-image"
              :src="slot.item.displayPreviewUrl || slot.item.previewUrl"
              mode="aspectFill"
              @error="handleReferenceImageError(slot.item)"
            />
            <button type="button" class="remove-reference" @click.stop="removeReference(slot.item.id)">×</button>
            <text v-if="slot.item.uploading" class="uploading-mask">上传中</text>
            <text v-if="slot.item.error" class="uploading-mask error">失败</text>
          </template>
          <template v-else>
            <image class="slot-icon" :src="icon('add-image')" mode="aspectFit" />
            <text>上传图片</text>
          </template>
        </view>
      </view>
      <text v-if="activeMode === 'image'" class="reference-hint">{{ referenceHint }}</text>

      <view class="creator-card prompt-card">
        <view class="section-head prompt-head">
          <view>
            <text class="section-title">提示词</text>
            <text class="help-dot">?</text>
          </view>
          <text class="counter">{{ prompt.length }}/1000</text>
        </view>

        <view class="prompt-panel">
          <textarea
            v-model="prompt"
            maxlength="1000"
            auto-height
            placeholder="描述你想要的画面内容、主体、风格、光线、镜头等，越具体越好..."
            :disabled="submitting"
          />
          <view class="prompt-actions">
            <picker
              mode="selector"
              :range="promptOptimizerModeLabels"
              :value="promptOptimizerModeIndex"
              :disabled="submitting || promptOptimizerRunning"
              @change="selectPromptOptimizerMode"
            >
              <view class="prompt-mode-picker" :class="{ disabled: submitting || promptOptimizerRunning }">
                <text>{{ selectedPromptOptimizerMode.label }}</text>
                <text class="picker-chevron">⌄</text>
              </view>
            </picker>
            <button
              type="button"
              class="prompt-optimize-button"
              :disabled="submitting || promptOptimizerRunning || !prompt.trim()"
              @click="optimizePrompt"
            >
              <image :src="icon('generate')" mode="aspectFit" />
              <text>{{ promptOptimizerRunning ? '优化中' : 'AI优化' }}</text>
            </button>
            <button type="button" @click="randomInspiration">
              <image :src="icon('style')" mode="aspectFit" />
              <text>随机灵感</text>
            </button>
            <button type="button" @click="openPromptTemplates">
              <image :src="icon('prompt')" mode="aspectFit" />
              <text>提示词模板</text>
            </button>
          </view>
          <text v-if="promptOptimizerError" class="prompt-inline-error">{{ promptOptimizerError }}</text>
        </view>
      </view>

      <view class="creator-card compact-card">
        <view>
          <text class="section-title">反向提示词</text>
          <text class="optional">可选</text>
        </view>
        <textarea
          v-model="negativePrompt"
          maxlength="500"
          auto-height
          class="negative-input"
          placeholder="不希望出现的元素、风格、颜色、物体等..."
          :disabled="submitting"
        />
        <text class="counter inline-counter">{{ negativePrompt.length }}/500</text>
      </view>

      <view class="creator-card">
        <text class="section-title">风格偏好</text>
        <view class="style-grid">
          <button
            v-for="item in stylePresets"
            :key="item.value"
            type="button"
            class="style-card"
            :class="{ active: stylePreset === item.value }"
            @click="setStyle(item.value)"
          >
            <image :src="item.icon" mode="aspectFit" />
            <text>{{ item.label }}</text>
          </button>
        </view>
      </view>

      <view class="creator-card">
        <picker
          mode="selector"
          :range="sizePresetLabels"
          :value="selectedSizePresetIndex"
          :disabled="submitting"
          @change="selectSizePreset"
        >
          <view class="size-picker-trigger" :class="{ disabled: submitting }">
            <view class="size-head">
              <view>
                <text class="section-title">图片尺寸</text>
                <text class="size-summary">
                  <image :src="icon('ratio')" mode="aspectFit" />
                  {{ formatSizePreset(selectedSizePreset) }}
                </text>
              </view>
              <text class="size-picker-arrow">⌄</text>
            </view>
          </view>
        </picker>
        <view class="size-card-row">
          <button
            v-for="(item, index) in sizePresets"
            :key="item.ratio"
            type="button"
            class="size-card"
            :class="{ active: selectedSizePresetIndex === index }"
            :disabled="submitting"
            @click="selectSizePreset(index)"
          >
            <text>{{ item.ratio }}</text>
            <text>{{ item.pixels }}</text>
          </button>
        </view>
      </view>

      <view class="generation-settings-grid">
        <view class="settings-panel">
          <text class="section-title">画质</text>
          <view class="count-row compact">
            <button
              v-for="item in qualityOptions"
              :key="item.key"
              type="button"
              :class="{ active: quality === item.key }"
              @click="selectQuality(item.key)"
            >
              {{ item.label }}
            </button>
          </view>
        </view>

        <view class="settings-panel">
          <text class="section-title">创意程度</text>
          <view class="variation-row compact">
            <button
              v-for="item in variationModeOptions"
              :key="item.key"
              type="button"
              :class="{ active: variationMode === item.key }"
              :disabled="submitting"
              @click="setVariationMode(item.key)"
            >
              <text>{{ item.label }}</text>
            </button>
          </view>
        </view>
      </view>
      <text class="credit-cost-hint">{{ creditCostHint }}</text>

      <view v-if="taskMessage || taskError" class="task-note" :class="{ error: taskError }">
        <view class="task-note-copy">
          <text>{{ taskError || taskMessage }}</text>
          <view v-if="creditShortfall" class="credit-shortfall-detail">
            <text>预计消耗 {{ creditShortfall.required_credits }} 点</text>
            <text>当前余额 {{ creditShortfall.available_credits }} 点</text>
            <text>还差 {{ creditShortfall.missing_credits }} 点</text>
            <text v-if="creditShortfall.recommended_package">推荐套餐 {{ creditShortfall.recommended_package.name }}</text>
          </view>
          <text v-else-if="availableCredits !== null">剩余 {{ availableCredits }} 点</text>
        </view>
        <button
          v-if="generationErrorCode === 'credits_insufficient'"
          type="button"
          class="task-retry-button pricing"
          @click="goPricing"
        >
          去充值
        </button>
        <button
          v-if="taskError && lastRequestPayload && generationErrorCode !== 'credits_insufficient'"
          type="button"
          class="task-retry-button"
          data-testid="mobile-generation-retry"
          @click="retryLastGeneration"
        >
          重新生成
        </button>
      </view>

    </view>

    <view class="generate-actions">
      <button type="button" class="generate-button" :class="{ loading: submitting }" @click="submitGeneration">
        <image :src="icon('generate')" mode="aspectFit" />
        <text>{{ submitting ? '生成中...' : '开始生成' }}</text>
      </button>
    </view>
    <AppTabbar active-key="workspace" extra-space="110rpx" />
    <AnnouncementPopup />

    <view v-if="showModeSheet" class="modal-backdrop" @click="showModeSheet = false">
      <view class="mode-modal" @click.stop>
        <view class="drag-handle"></view>
        <text class="modal-title">选择工作模式</text>
        <text class="modal-subtitle">选择适合你的创作方式</text>

        <button type="button" class="mode-card" :class="{ selected: activeMode === 'text' }" @click="chooseTextMode">
          <image :src="icon('text-image')" mode="aspectFit" />
          <view>
            <text>文生图</text>
            <text>通过文字描述生成全新图片</text>
            <text>适合创意构思、概念设计、海报和电商图</text>
          </view>
          <text :class="activeMode === 'text' ? 'selected-dot' : 'arrow'">{{ activeMode === 'text' ? '✓' : '›' }}</text>
        </button>

        <button type="button" class="mode-card" :class="{ selected: activeMode === 'image' }" @click="chooseImageMode">
          <image :src="icon('image-image')" mode="aspectFit" />
          <view>
            <text>图生图</text>
            <text>基于参考图进行风格转换或内容重绘</text>
            <text>适合风格迁移、细节调整、延展创作</text>
          </view>
          <text :class="activeMode === 'image' ? 'selected-dot' : 'arrow'">{{ activeMode === 'image' ? '✓' : '›' }}</text>
        </button>

        <button type="button" class="mode-card mode-card-link" @click="openCoupleAlbumMode">
          <image :src="icon('favorite')" mode="aspectFit" />
          <view>
            <text>情侣相册</text>
            <text>上传双人照片，生成可分享的旅行相册</text>
            <text>适合纪念日、旅拍记录和 520 分享卡片</text>
          </view>
          <text class="arrow">›</text>
        </button>

        <button type="button" class="mode-card mode-card-link" @click="openChildhoodDreamAlbumMode">
          <image :src="icon('logo-star')" mode="aspectFit" />
          <view>
            <text>童年梦想相册</text>
            <text>上传孩子照片，生成六一职业梦想相册</text>
            <text>8 页连续故事，适合儿童节分享保存</text>
          </view>
          <text class="arrow">›</text>
        </button>

        <view class="mode-tips">
          <text>{{ activeMode === 'image' ? '图生图功能亮点' : '文生图功能亮点' }}</text>
          <text>{{ activeMode === 'image' ? '保留参考图构图与结构' : '直接输入提示词生成，无需先选图片' }}</text>
          <text>{{ activeMode === 'image' ? '智能识别主体与风格特征' : '支持风格、尺寸、画质和创意程度控制' }}</text>
          <text>{{ activeMode === 'image' ? '支持多图参考融合创作' : '生成结果会自动写入作品库' }}</text>
        </view>
      </view>
    </view>

    <view v-if="showTemplateSheet" class="template-backdrop" @click="closePromptTemplates">
      <view class="template-sheet" @click.stop>
        <view class="drag-handle"></view>
        <view class="template-sheet-head">
          <view>
            <text class="modal-title">提示词模板库</text>
            <text class="modal-subtitle">浏览免费，使用模板扣 1 点并回填到提示词</text>
          </view>
          <button type="button" class="history-close" @click="closePromptTemplates">×</button>
        </view>

        <view class="template-list">
          <view v-if="templateLoading" class="history-state">
            <text>正在读取模板...</text>
          </view>
          <view v-else-if="templateError" class="history-state error">
            <text>{{ templateError }}</text>
            <button type="button" @click="loadPromptTemplates">重试</button>
          </view>
          <view v-else-if="templateItems.length === 0" class="history-state">
            <text>暂无可用模板</text>
          </view>
          <template v-else>
            <view v-for="item in templateItems" :key="item.id" class="template-card">
              <image
                v-if="templatePreviewURL(item)"
                :src="templatePreviewURL(item)"
                mode="aspectFill"
                @click.stop="previewPromptTemplate(item)"
              />
              <view v-else class="template-preview-placeholder" @click.stop="previewPromptTemplate(item)">
                <image :src="icon('prompt')" mode="aspectFit" />
              </view>
              <view class="template-card-body">
                <view class="template-card-title">
                  <text>{{ item.title }}</text>
                  <text class="template-ratio-badge" @click.stop="previewPromptTemplate(item)">
                    {{ item.aspect_ratio || '1:1' }}
                  </text>
                </view>
                <text class="template-desc">{{ item.description }}</text>
                <view class="template-meta">
                  <text>{{ item.category }}</text>
                  <text>{{ item.style_preset || '无风格' }}</text>
                </view>
                <button
                  type="button"
                  class="template-use-button"
                  :disabled="Boolean(usingTemplateID)"
                  @click="usePromptTemplate(item)"
                >
                  {{ isUsingTemplate(item) ? '使用中' : '使用 1 点' }}
                </button>
              </view>
            </view>
          </template>
        </view>
      </view>
    </view>

    <view v-if="showHistorySheet" class="history-backdrop" @click="closeHistory">
      <view class="history-sheet" @click.stop>
        <view class="drag-handle"></view>
        <view class="history-sheet-head">
          <view>
            <text class="modal-title">历史提示词</text>
            <text class="modal-subtitle">选择历史作品，直接复刻提示词、尺寸和参考图</text>
          </view>
          <button type="button" class="history-close" @click="closeHistory">×</button>
        </view>

        <view class="history-list">
          <view v-if="historyLoading" class="history-state">
            <text>正在读取历史...</text>
          </view>
          <view v-else-if="historyError" class="history-state error">
            <text>{{ historyError }}</text>
            <button type="button" @click="loadHistoryWorks">重试</button>
          </view>
          <view v-else-if="historyItems.length === 0" class="history-state">
            <text>暂无可导入的历史作品</text>
          </view>
          <template v-else>
            <button
              v-for="item in historyItems"
              :key="workID(item)"
              type="button"
              class="history-import-item"
              :disabled="Boolean(applyingHistoryWorkID)"
              @click="importHistoryWork(item)"
            >
              <image v-if="workCover(item)" :src="workCover(item)" mode="aspectFill" />
              <view v-else class="history-placeholder">
                <image :src="icon('prompt')" mode="aspectFit" />
              </view>
              <view class="history-import-copy">
                <text>{{ promptExcerpt(item) }}</text>
                <text>{{ item.aspect_ratio || '1:1' }} · {{ formatHistoryTime(item.created_at) }}</text>
              </view>
              <text class="history-import-status">
                {{ isApplyingHistoryWork(item) ? '导入中' : '导入' }}
              </text>
            </button>
          </template>
        </view>
      </view>
    </view>
  </view>
</template>

<style lang="scss" scoped>
@use '../../../styles/tokens.scss' as *;

.workspace-page {
  min-height: 100vh;
  background:
    radial-gradient(circle at 2% 0, rgba(255, 219, 233, 0.9), transparent 33%),
    radial-gradient(circle at 100% 0, rgba(220, 238, 255, 0.92), transparent 35%),
    linear-gradient(180deg, #fff9fd 0%, #f8fbff 52%, #eef6ff 100%);
  color: #111827;
}

.workspace-page button {
  margin: 0;
  padding: 0;
  border: 0;
  line-height: 1.2;
  overflow: visible;
}

.workspace-page button::after {
  border: 0;
}

.app-shell {
  min-height: 100vh;
  padding: calc(26rpx + $phone-float-clearance-top + env(safe-area-inset-top)) 28rpx 0;
}

.topbar,
.brand,
.mode-tabs,
.section-head,
.section-head view,
.clear-button,
.prompt-actions,
.count-row,
.mode-card,
.mode-tips text {
  display: flex;
  align-items: center;
}

.brand-name,
.brand-subtitle,
.section-title,
.modal-title,
.modal-subtitle,
.mode-card text,
.mode-tips text {
  display: block;
}

.history-button {
  display: grid;
  place-items: center;
  width: 58rpx;
  height: 58rpx;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.9);
  box-shadow: 0 10rpx 24rpx rgba(41, 57, 94, 0.12);
}

.history-button image,
.clear-button image,
.prompt-actions image,
.generate-button image {
  width: 30rpx;
  height: 30rpx;
}

.topbar {
  justify-content: space-between;
}

.brand {
  gap: 14rpx;
}

.brand-icon {
  width: 54rpx;
  height: 54rpx;
}

.brand-name {
  color: #10182d;
  font-size: 30rpx;
  font-weight: 950;
  line-height: 1.08;
}

.brand-subtitle {
  margin-top: 6rpx;
  color: #6f7890;
  font-size: 22rpx;
  font-weight: 700;
}

.mode-tabs {
  gap: 8rpx;
  height: 86rpx;
  margin-top: 36rpx;
  padding: 6rpx;
  border: 1rpx solid rgba(140, 151, 177, 0.16);
  border-radius: 18rpx;
  background: rgba(255, 255, 255, 0.82);
  box-shadow: 0 10rpx 26rpx rgba(36, 47, 82, 0.06);
}

.mode-label,
.mode-tabs button {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  min-width: 0;
  height: 74rpx;
  border-radius: 15rpx;
  white-space: nowrap;
}

.mode-label {
  background: rgba(244, 247, 252, 0.74);
  color: #6f7d96;
  font-size: 25rpx;
  font-weight: 900;
  box-shadow: inset 0 0 0 1rpx rgba(142, 153, 177, 0.08);
}

.mode-tabs button {
  color: #1f2a44;
  font-size: 25rpx;
  font-weight: 900;
}

.mode-tabs .mode-current {
  flex: 1.12;
  justify-content: center;
  gap: 10rpx;
  background: linear-gradient(135deg, #a94af3 0%, #1767ff 100%);
  color: #fff;
  box-shadow: 0 14rpx 30rpx rgba(82, 90, 235, 0.3);
}

.mode-label text,
.mode-current text {
  display: block;
  min-width: 0;
  white-space: nowrap;
}

.chevron {
  flex: 0 0 auto;
  font-size: 30rpx;
  line-height: 1;
}

.section-head {
  justify-content: space-between;
  margin-top: 0;
  min-height: 40rpx;
}

.creator-card,
.settings-panel {
  margin-top: 24rpx;
  padding: 22rpx;
  border: 1rpx solid rgba(143, 154, 177, 0.13);
  border-radius: 18rpx;
  background: rgba(255, 255, 255, 0.86);
  box-shadow: 0 16rpx 38rpx rgba(31, 45, 82, 0.045);
}

.prompt-card {
  margin-top: 24rpx;
}

.compact-card {
  position: relative;
}

.compact-card > view {
  display: flex;
  align-items: baseline;
  gap: 8rpx;
}

.inline-counter {
  position: absolute;
  top: 24rpx;
  right: 24rpx;
}

.section-title {
  color: #172033;
  font-size: 27rpx;
  font-weight: 950;
}

.help-dot {
  display: grid;
  place-items: center;
  width: 24rpx;
  height: 24rpx;
  margin-left: 10rpx;
  border: 2rpx solid #a6afbf;
  border-radius: 50%;
  color: #8b95a8;
  font-size: 17rpx;
  font-weight: 900;
}

.clear-button {
  gap: 6rpx;
  color: #6d7588;
  font-size: 22rpx;
  font-weight: 800;
}

.reference-head {
  margin-top: 32rpx;
}

.reference-panel {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 14rpx;
  margin-top: 14rpx;
  padding: 14rpx;
  border: 1rpx solid rgba(143, 154, 177, 0.16);
  border-radius: 17rpx;
  background: rgba(255, 255, 255, 0.8);
  box-shadow: 0 16rpx 38rpx rgba(31, 45, 82, 0.05);
}

.reference-hint {
  display: block;
  margin-top: 11rpx;
  color: #7d8799;
  font-size: 21rpx;
  font-weight: 750;
  line-height: 1.35;
}

.reference-slot {
  position: relative;
  display: grid;
  place-items: center;
  align-content: center;
  gap: 10rpx;
  aspect-ratio: 1 / 1;
  min-width: 0;
  overflow: hidden;
  border: 1rpx dashed rgba(133, 143, 166, 0.28);
  border-radius: 14rpx;
  background: rgba(247, 249, 253, 0.82);
  color: #66718a;
  font-size: 20rpx;
  font-weight: 800;
}

.reference-slot.filled {
  border-style: solid;
  background: #eef3ff;
}

.slot-icon {
  width: 38rpx;
  height: 38rpx;
}

.reference-image {
  position: absolute;
  inset: 0;
  display: block;
  width: 100%;
  height: 100%;
}

.reference-image div,
.reference-image img {
  width: 100%;
  height: 100%;
}

.remove-reference {
  position: absolute;
  top: 6rpx;
  right: 6rpx;
  z-index: 2;
  display: grid;
  place-items: center;
  width: 30rpx;
  height: 30rpx;
  border-radius: 50%;
  background: rgba(20, 29, 48, 0.72);
  color: #fff;
  font-size: 24rpx;
  font-weight: 900;
}

.uploading-mask {
  position: absolute;
  inset: 0;
  z-index: 1;
  display: grid;
  place-items: center;
  background: rgba(255, 255, 255, 0.72);
  color: #245cff;
  font-size: 21rpx;
  font-weight: 900;
}

.uploading-mask.error {
  color: #dd3d3d;
}

.prompt-head {
  margin-top: 28rpx;
}

.counter,
.optional {
  color: #8a94a8;
  font-size: 22rpx;
  font-weight: 800;
}

.prompt-panel {
  margin-top: 13rpx;
  padding: 0;
  border: 1rpx solid rgba(143, 154, 177, 0.16);
  border-radius: 17rpx;
  background: rgba(255, 255, 255, 0.82);
  box-shadow: none;
}

.prompt-panel textarea {
  width: 100%;
  min-height: 190rpx;
  padding: 22rpx;
  color: #172033;
  font-size: 23rpx;
  font-weight: 600;
  line-height: 1.45;
  overflow: visible;
}

.prompt-actions {
  gap: 14rpx;
  padding: 0 22rpx 20rpx;
  flex-wrap: wrap;
}

.prompt-actions button,
.prompt-mode-picker {
  display: flex;
  align-items: center;
  gap: 7rpx;
  min-height: 46rpx;
  padding: 0 16rpx;
  border-radius: 10rpx;
  background: rgba(126, 79, 246, 0.09);
  color: #7145e8;
  font-size: 21rpx;
  font-weight: 900;
}

.prompt-mode-picker {
  min-width: 132rpx;
  justify-content: center;
  background: rgba(31, 103, 255, 0.08);
  color: #2d5bea;
}

.prompt-mode-picker.disabled {
  opacity: 0.58;
}

.picker-chevron {
  flex: 0 0 auto;
  font-size: 24rpx;
  line-height: 1;
}

.prompt-optimize-button {
  background: rgba(126, 79, 246, 0.12);
}

.prompt-inline-error {
  display: block;
  padding: 0 22rpx 18rpx;
  color: #d33c3c;
  font-size: 21rpx;
  font-weight: 800;
  line-height: 1.35;
}

.field-block {
  margin-top: 24rpx;
}

.section-head.compact {
  justify-content: flex-start;
  gap: 8rpx;
  margin-top: 0;
}

.section-head.compact .counter {
  margin-left: auto;
}

.negative-input {
  width: 100%;
  min-height: 66rpx;
  margin-top: 12rpx;
  padding: 17rpx 18rpx;
  border: 1rpx solid rgba(143, 154, 177, 0.16);
  border-radius: 14rpx;
  background: rgba(255, 255, 255, 0.82);
  color: #172033;
  font-size: 23rpx;
  font-weight: 700;
  line-height: 1.45;
  overflow: visible;
}

.size-picker-trigger {
  display: block;
  width: 100%;
  border-radius: 14rpx;
}

.size-picker-trigger.disabled {
  opacity: 0.58;
}

.size-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18rpx;
}

.size-summary {
  display: flex;
  align-items: center;
  gap: 10rpx;
  margin-top: 10rpx;
  color: #747e94;
  font-size: 22rpx;
  font-weight: 800;
  line-height: 1.25;
}

.size-summary image {
  width: 26rpx;
  height: 26rpx;
}

.size-card-row {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 14rpx;
  margin-top: 18rpx;
}

.size-card {
  display: grid;
  place-items: center;
  align-content: center;
  gap: 8rpx;
  min-width: 0;
  min-height: 78rpx;
  border: 1rpx solid rgba(143, 154, 177, 0.16);
  border-radius: 14rpx;
  background: rgba(255, 255, 255, 0.88);
  color: #172033;
  box-shadow: 0 10rpx 22rpx rgba(31, 45, 82, 0.04);
}

.size-card text:first-child {
  color: #182238;
  font-size: 25rpx;
  font-weight: 950;
  line-height: 1;
}

.size-card text:last-child {
  color: #7b8598;
  font-size: 19rpx;
  font-weight: 800;
  line-height: 1.1;
}

.size-card.active {
  border-color: rgba(117, 85, 255, 0.88);
  background: rgba(248, 246, 255, 0.98);
  color: #5260ff;
  box-shadow:
    0 12rpx 24rpx rgba(102, 83, 255, 0.14),
    inset 0 0 0 1rpx rgba(255, 255, 255, 0.78);
}

.size-card.active text {
  color: #5260ff;
}

.count-row {
  gap: 14rpx;
  margin-top: 14rpx;
}

.count-row button,
.count-row uni-button,
.style-card {
  min-width: 0;
  border: 1rpx solid rgba(143, 154, 177, 0.16);
  background: rgba(255, 255, 255, 0.84);
  color: #273149;
  box-shadow: 0 10rpx 24rpx rgba(31, 45, 82, 0.04);
}

.size-picker-arrow {
  flex: 0 0 auto;
  color: #334155;
  font-size: 32rpx;
  font-weight: 900;
  line-height: 1;
}

.count-row button.active,
.count-row uni-button.active,
.style-card.active {
  border-color: #7b4fff;
  background: rgba(244, 241, 255, 0.95);
  color: #4f54f5;
}

.count-row button,
.count-row uni-button {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  height: 62rpx;
  border-radius: 15rpx;
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.96), rgba(249, 251, 255, 0.92));
  color: #182238;
  font-size: 22rpx;
  font-weight: 900;
  line-height: 1;
  box-shadow: 0 12rpx 26rpx rgba(31, 45, 82, 0.06);
}

.count-row button.active,
.count-row uni-button.active {
  border-color: rgba(117, 85, 255, 0.88);
  background: linear-gradient(180deg, rgba(249, 247, 255, 0.98), rgba(241, 238, 255, 0.96));
  color: #5260ff;
  box-shadow:
    0 12rpx 24rpx rgba(102, 83, 255, 0.14),
    inset 0 0 0 1rpx rgba(255, 255, 255, 0.78);
}

.credit-cost-hint {
  display: block;
  margin-top: 12rpx;
  color: #6b7280;
  font-size: 22rpx;
  font-weight: 700;
  line-height: 1.35;
}

.variation-row {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 14rpx;
  margin-top: 14rpx;
}

.generation-settings-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16rpx;
  margin-top: 24rpx;
}

.settings-panel {
  min-width: 0;
  margin-top: 0;
  padding: 18rpx;
}

.count-row.compact {
  gap: 10rpx;
}

.variation-row.compact {
  gap: 10rpx;
}

.variation-row.compact button,
.variation-row.compact uni-button {
  min-height: 58rpx;
  padding: 0 8rpx;
  text-align: center;
  place-items: center;
}

.variation-row button,
.variation-row uni-button {
  display: grid;
  gap: 8rpx;
  min-width: 0;
  min-height: 108rpx;
  padding: 16rpx 12rpx;
  border: 1rpx solid rgba(143, 154, 177, 0.16);
  border-radius: 18rpx;
  background: rgba(255, 255, 255, 0.84);
  color: #273149;
  text-align: left;
  box-shadow: 0 10rpx 24rpx rgba(31, 45, 82, 0.04);
}

.variation-row button.active,
.variation-row uni-button.active {
  border-color: #7b4fff;
  background: rgba(244, 241, 255, 0.95);
  color: #4f54f5;
}

.variation-row text:first-child {
  font-size: 24rpx;
  font-weight: 950;
  line-height: 1.15;
}

.variation-row.compact text:first-child {
  font-size: 20rpx;
  line-height: 1;
}

.variation-row text:last-child {
  color: #667085;
  font-size: 20rpx;
  font-weight: 700;
  line-height: 1.35;
}

.style-grid {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 13rpx;
  margin-top: 14rpx;
}

.style-card {
  display: grid;
  place-items: center;
  align-content: center;
  gap: 7rpx;
  min-height: 86rpx;
  border-radius: 14rpx;
  font-size: 19rpx;
  font-weight: 900;
}

.style-card image {
  width: 32rpx;
  height: 32rpx;
}

.task-note {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16rpx;
  margin-top: 24rpx;
  padding: 16rpx 18rpx;
  border-radius: 16rpx;
  background: rgba(31, 103, 255, 0.08);
  color: #1f67ff;
  font-size: 22rpx;
  font-weight: 800;
}

.task-note.error {
  background: rgba(223, 61, 61, 0.08);
  color: #d33c3c;
}

.task-note-copy {
  display: grid;
  gap: 8rpx;
  min-width: 0;
}

.task-note-copy > text {
  display: block;
  line-height: 1.35;
}

.credit-shortfall-detail {
  display: flex;
  flex-wrap: wrap;
  gap: 8rpx 14rpx;
  color: #8f3131;
  font-size: 20rpx;
  font-weight: 850;
  line-height: 1.25;
}

.credit-shortfall-detail text {
  display: block;
}

.task-retry-button {
  flex-shrink: 0;
  min-width: 132rpx;
  height: 52rpx;
  border: 0;
  border-radius: 999rpx;
  background: #d33c3c;
  color: #fff;
  font-size: 22rpx;
  font-weight: 900;
  line-height: 52rpx;
}

.task-retry-button.pricing {
  background: #1f67ff;
}

.generate-actions {
  position: fixed;
  left: 0;
  right: 0;
  bottom: calc(112rpx + env(safe-area-inset-bottom));
  z-index: 21;
  padding: 14rpx 28rpx 10rpx;
  background: linear-gradient(180deg, rgba(248, 251, 255, 0) 0%, rgba(255, 255, 255, 0.96) 42%, #fff 100%);
}

.generate-button {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 9rpx;
  width: 100%;
  height: 76rpx;
  border-radius: 15rpx;
  background: linear-gradient(135deg, #b342ee 0%, #136aff 100%);
  color: #fff;
  font-size: 27rpx;
  font-weight: 950;
  box-shadow: 0 18rpx 36rpx rgba(78, 87, 232, 0.28);
}

.generate-button.loading {
  opacity: 0.72;
}

.modal-backdrop {
  position: fixed;
  inset: 0;
  z-index: 40;
  display: grid;
  place-items: center;
  padding: 40rpx;
  background: rgba(23, 31, 49, 0.08);
  backdrop-filter: blur(4rpx);
}

.history-backdrop {
  position: fixed;
  inset: 0;
  z-index: 45;
  display: flex;
  align-items: flex-end;
  justify-content: center;
  padding: 0 20rpx calc(20rpx + env(safe-area-inset-bottom));
  background: rgba(23, 31, 49, 0.18);
  backdrop-filter: blur(5rpx);
}

.template-backdrop {
  position: fixed;
  inset: 0;
  z-index: 46;
  display: flex;
  align-items: flex-end;
  justify-content: center;
  padding: 0 20rpx calc(20rpx + env(safe-area-inset-bottom));
  background: rgba(23, 31, 49, 0.2);
  backdrop-filter: blur(5rpx);
}

.mode-modal {
  width: min(100%, 560rpx);
  max-height: calc(100vh - 80rpx);
  padding: 24rpx;
  overflow-y: auto;
  border-radius: 28rpx;
  background: rgba(255, 255, 255, 0.96);
  box-shadow: 0 24rpx 70rpx rgba(37, 49, 83, 0.16);
}

.history-sheet {
  width: min(100%, 690rpx);
  max-height: 76vh;
  padding: 22rpx;
  overflow: hidden;
  border-radius: 30rpx 30rpx 24rpx 24rpx;
  background: rgba(255, 255, 255, 0.98);
  box-shadow: 0 -18rpx 70rpx rgba(37, 49, 83, 0.2);
}

.template-sheet {
  width: min(100%, 700rpx);
  max-height: 82vh;
  padding: 22rpx;
  overflow: hidden;
  border-radius: 30rpx 30rpx 24rpx 24rpx;
  background: rgba(255, 255, 255, 0.98);
  box-shadow: 0 -20rpx 76rpx rgba(37, 49, 83, 0.22);
}

.drag-handle {
  width: 72rpx;
  height: 6rpx;
  margin: 0 auto 40rpx;
  border-radius: 999rpx;
  background: #d5d9e2;
}

.history-sheet .drag-handle {
  margin-bottom: 24rpx;
}

.modal-title {
  color: #111827;
  text-align: center;
  font-size: 30rpx;
  font-weight: 950;
}

.modal-subtitle {
  margin-top: 13rpx;
  color: #7c8699;
  text-align: center;
  font-size: 22rpx;
  font-weight: 800;
}

.history-sheet-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20rpx;
}

.history-sheet-head .modal-title,
.history-sheet-head .modal-subtitle,
.template-sheet-head .modal-title,
.template-sheet-head .modal-subtitle {
  text-align: left;
}

.template-sheet-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20rpx;
}

.history-close {
  flex: 0 0 auto;
  display: grid;
  place-items: center;
  width: 48rpx;
  height: 48rpx;
  border-radius: 50%;
  background: rgba(239, 242, 248, 0.95);
  color: #657089;
  font-size: 34rpx;
  font-weight: 800;
}

.history-list {
  display: grid;
  gap: 14rpx;
  max-height: calc(76vh - 150rpx);
  margin-top: 22rpx;
  overflow-y: auto;
  padding-right: 2rpx;
}

.template-list {
  display: grid;
  gap: 16rpx;
  max-height: calc(82vh - 210rpx);
  margin-top: 20rpx;
  overflow-y: auto;
  padding-right: 2rpx;
}

.history-state {
  display: grid;
  place-items: center;
  min-height: 180rpx;
  color: #747f95;
  font-size: 24rpx;
  font-weight: 850;
}

.history-state.error {
  gap: 16rpx;
  color: #d84141;
}

.history-state button {
  min-height: 48rpx;
  padding: 0 22rpx;
  border-radius: 12rpx;
  background: rgba(126, 79, 246, 0.1);
  color: #6247e8;
  font-size: 22rpx;
  font-weight: 900;
}

.history-import-item {
  display: flex;
  align-items: center;
  width: 100%;
  gap: 16rpx;
  min-height: 118rpx;
  padding: 14rpx;
  border: 1rpx solid rgba(143, 154, 177, 0.16);
  border-radius: 18rpx;
  background: rgba(249, 251, 255, 0.86);
  text-align: left;
}

.history-import-item image,
.history-placeholder {
  flex: 0 0 auto;
  width: 90rpx;
  height: 90rpx;
  border-radius: 14rpx;
  overflow: hidden;
  background: #eef3ff;
}

.history-placeholder {
  display: grid;
  place-items: center;
}

.history-placeholder image {
  width: 38rpx;
  height: 38rpx;
}

.history-import-copy {
  flex: 1;
  min-width: 0;
}

.history-import-copy text {
  display: block;
  min-width: 0;
}

.history-import-copy text:first-child {
  color: #182238;
  font-size: 23rpx;
  font-weight: 900;
  line-height: 1.35;
}

.history-import-copy text:last-child {
  margin-top: 8rpx;
  color: #7b8598;
  font-size: 20rpx;
  font-weight: 800;
}

.history-import-status {
  flex: 0 0 auto;
  color: #4f54f5;
  font-size: 22rpx;
  font-weight: 950;
}

.template-card {
  display: grid;
  grid-template-columns: 198rpx minmax(0, 1fr);
  gap: 18rpx;
  padding: 16rpx;
  border: 1rpx solid rgba(143, 154, 177, 0.14);
  border-radius: 20rpx;
  background: linear-gradient(180deg, rgba(249, 251, 255, 0.96), rgba(255, 255, 255, 0.96));
}

.template-card > image,
.template-preview-placeholder {
  width: 198rpx;
  height: 132rpx;
  border-radius: 16rpx;
  overflow: hidden;
  background: #eef3ff;
}

.template-preview-placeholder {
  display: grid;
  place-items: center;
}

.template-preview-placeholder image {
  width: 48rpx;
  height: 48rpx;
}

.template-card-body {
  min-width: 0;
  display: grid;
  gap: 9rpx;
}

.template-card-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12rpx;
}

.template-card-title text:first-child {
  flex: 1;
  min-width: 0;
  color: #172033;
  font-size: 24rpx;
  font-weight: 950;
  line-height: 1.28;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.template-ratio-badge {
  flex: 0 0 auto;
  padding: 5rpx 10rpx;
  border-radius: 10rpx;
  background: rgba(126, 79, 246, 0.1);
  color: #6b4df4;
  font-size: 18rpx;
  font-weight: 950;
}

.template-desc {
  color: #748099;
  font-size: 20rpx;
  font-weight: 800;
  line-height: 1.38;
  display: -webkit-box;
  overflow: hidden;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.template-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8rpx;
}

.template-meta text {
  padding: 5rpx 10rpx;
  border-radius: 10rpx;
  background: rgba(226, 232, 240, 0.72);
  color: #657089;
  font-size: 18rpx;
  font-weight: 900;
}

.template-use-button {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 150rpx;
  min-height: 50rpx;
  margin: 0;
  border-radius: 14rpx;
  background: linear-gradient(135deg, #8f4fff, #176df2);
  color: #fff;
  font-size: 21rpx;
  font-weight: 950;
  line-height: 1;
  box-shadow: 0 10rpx 22rpx rgba(37, 99, 235, 0.18);
}

.template-use-button[disabled] {
  opacity: 0.62;
}

.mode-card {
  position: relative;
  width: 100%;
  min-height: 126rpx;
  margin-top: 28rpx;
  gap: 18rpx;
  padding: 20rpx 18rpx;
  border: 1rpx solid rgba(142, 153, 177, 0.18);
  border-radius: 15rpx;
  background: rgba(255, 255, 255, 0.92);
  text-align: left;
}

.mode-card image {
  flex: 0 0 auto;
  width: 58rpx;
  height: 58rpx;
}

.mode-card view {
  flex: 1;
  min-width: 0;
}

.mode-card view text:first-child {
  color: #172033;
  font-size: 26rpx;
  font-weight: 950;
}

.mode-card view text:nth-child(2) {
  margin-top: 10rpx;
  color: #4f5b73;
  font-size: 21rpx;
  font-weight: 800;
}

.mode-card view text:nth-child(3) {
  margin-top: 9rpx;
  color: #8a94a8;
  font-size: 20rpx;
  font-weight: 700;
}

.mode-card.selected {
  border-color: #7b4fff;
  background: rgba(250, 249, 255, 0.98);
}

.mode-card.selected view text:first-child {
  color: #4e55f5;
}

.mode-card-link {
  background: linear-gradient(180deg, rgba(255, 250, 253, 0.98), rgba(255, 255, 255, 0.96));
}

.arrow {
  color: #718096;
  font-size: 42rpx;
}

.selected-dot {
  display: grid;
  place-items: center;
  width: 32rpx;
  height: 32rpx;
  border-radius: 50%;
  background: #4f62ff;
  color: #fff;
  font-size: 22rpx;
  font-weight: 950;
}

.mode-tips {
  display: grid;
  gap: 15rpx;
  margin-top: 28rpx;
  padding: 22rpx;
  border-radius: 16rpx;
  background: linear-gradient(180deg, rgba(248, 247, 255, 0.96), rgba(244, 244, 252, 0.94));
}

.mode-tips text {
  gap: 10rpx;
  color: #687389;
  font-size: 21rpx;
  font-weight: 800;
}

.mode-tips text:first-child {
  color: #6958ff;
  font-size: 22rpx;
  font-weight: 950;
}

.mode-tips text:not(:first-child)::before {
  content: '';
  width: 10rpx;
  height: 10rpx;
  border: 2rpx solid #6958ff;
  border-radius: 3rpx;
  transform: rotate(45deg);
}

@media (max-width: 360px) {
  .app-shell {
    padding-left: 22rpx;
    padding-right: 22rpx;
  }

  .reference-panel,
  .count-row {
    gap: 10rpx;
  }

  .size-card-row,
  .generation-settings-grid {
    gap: 10rpx;
  }

  .size-card text:last-child {
    font-size: 17rpx;
  }

  .style-grid {
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 9rpx;
  }

  .style-card {
    font-size: 18rpx;
  }
}
</style>
