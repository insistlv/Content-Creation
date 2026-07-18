<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import {
  ArrowUpRight,
  Bot,
  Brush,
  Camera,
  Edit3,
  Heart,
  Image as ImageIcon,
  ImagePlus,
  Maximize2,
  Megaphone,
  Newspaper,
  MousePointer2,
  PanelsTopLeft,
  RefreshCw,
  Send,
  Shirt,
  ShoppingBag,
  Sparkles,
  Type,
  Video,
  Wand2,
  X
} from 'lucide-vue-next'
import { useRouter } from 'vue-router'
import { api } from '../api/client.js'
import { applyAvailableCredits, currentUser, loadCurrentUser, refreshCurrentUser } from '../stores/session.js'
import { confirmLoginBeforeUse } from '../auth-navigation.js'
import WorkspaceComposerPanel from '../components/workspace/WorkspaceComposerPanel.vue'
import WorkspaceDiscoverySurface from '../components/workspace/WorkspaceDiscoverySurface.vue'
import AgentWorkspacePanel from '../components/workspace/AgentWorkspacePanel.vue'
import { usePointerZoom } from '../composables/usePointerZoom.js'
import productImage from '../image/image-agent/commerce-showcase.png'
import portraitImage from '../image/image-agent/portrait.png'
import interiorImage from '../image/image-agent/interior.png'
import landscapeImage from '../image/image-agent/landscape.png'
import cityImage from '../image/image-agent/city.png'

// 状态管理
const router = useRouter()
const WORKS_PAGE_SIZE = 18
const referenceAssetUploadMaxBytes = 50 * 1024 * 1024
const me = ref(currentUser.value)
const works = ref([])
const recentWorks = ref([])
const worksPage = ref(1)
const worksPageSize = WORKS_PAGE_SIZE
const worksTotal = ref(0)
const worksLoading = ref(false)
const referenceAssets = ref([])
const selectedReferenceIds = ref([])
const selectedReferenceWorkIds = ref([])
const selectedReferenceWorkSnapshots = ref({})
const selectedSourceWorkId = ref(null)
const referenceUploading = ref(false)
const referenceError = ref('')
const worksError = ref('')
const discoveryError = ref('')
const loading = ref(true)
const submitting = ref(false)
const pageError = ref('')
const taskError = ref('')
const resultError = ref('')
const successMessage = ref('')
const prompt = ref('')
const negativePrompt = ref('')
const aspectRatio = ref('1:1')
const stylePreset = ref('')
const toolMode = ref('generate')
const quality = ref('medium')
const referenceWeight = ref(75)
const selectedModelId = ref(null)
const workspaceModels = ref([])
const workspaceTools = ref([])
const toolOptions = ref({})
const editInstruction = ref('')
const creditEstimate = ref(null)
const creditEstimateError = ref('')
const creditEstimateLoading = ref(false)
const creditEstimateCanCreateWithoutEstimate = ref(false)
const creditEstimateNotice = ref('')
const autoTranslate = ref(false)
const workspaceTab = ref('discover')
const workspaceMode = ref('create')
const discoveryFilter = ref('all')
const discoveryHotTemplates = ref([])
const discoveryInspirationTemplates = ref([])
const discoveryRecommendations = ref([])
const recommendationPreview = ref(null)
const activeTasks = ref([])
const cancellingTaskIds = ref([])
const regeneratingResult = ref(false)
const regenerateResultError = ref('')
const selectedTaskId = ref(null)
const result = ref(null)
const previewZoomOpen = ref(false)
const promptOptimizerOpen = ref(false)
const assistantMessages = ref([])
const assistantInput = ref('')
const assistantRunning = ref(false)
const assistantRunningAction = ref('')
const assistantError = ref('')
const assistantStructuredPrompt = ref({
  subject: '',
  scene: '',
  style: '',
  usage: ''
})
const assistantDraftPrompt = ref('')
const assistantDirections = ref([])
const assistantNotes = ref([])
const assistantEditingField = ref('')
const assistantFieldDraft = ref('')
const assistantMessagesRef = ref(null)
const assistantInputRef = ref(null)
const composerPanelRef = ref(null)
const eraseMaskCanvasRef = ref(null)
const assistantLastRequest = ref(null)
const assistantAbortController = ref(null)
const agentMessages = ref([
  {
    role: 'assistant',
    content: '描述你的创作目标，或上传参考图。我会先整理可编辑方案，确认后再提交生成。'
  }
])
const agentPlan = ref(null)
const agentCandidates = ref([])
const agentSelectedCandidateId = ref('')
const agentPlanning = ref(false)
const agentModeError = ref('')
const agentFailure = ref(null)
const agentSafetyNotes = ref([])
const agentExecutionTaskId = ref(null)
const agentStep = ref('describe')
const agentEstimateDirty = ref(false)
const agentLastEstimatedPayload = ref(null)
const agentClarificationPrompt = ref('')

const expandDefaultPrompt = '只补外扩背景，原图人物和主体区域将保持不变；让新增边界与原图光线、透视、材质和画风自然衔接。'
const eraseDefaultPrompt = '移除圈选区域中的物体，自然补全背景并保持其他区域不变。'
const removeBackgroundDefaultPrompt = '移除图片背景，保留主体和主体边缘细节，输出透明背景 PNG，不要改变主体材质、颜色、比例或构图。'
const upscaleDefaultPrompt = '提升图片清晰度、纹理细节和边缘质量，保持主体、颜色、构图和内容不变'
const expandEdgeKeys = ['top', 'bottom', 'left', 'right']
const expandShortcutPresets = [
  { key: 'all', label: '四周 20%', values: { top: 20, bottom: 20, left: 20, right: 20 } },
  { key: 'wide', label: '横向 30%', values: { top: 0, bottom: 0, left: 30, right: 30 } },
  { key: 'tall', label: '纵向 30%', values: { top: 30, bottom: 30, left: 0, right: 0 } },
  { key: 'clear', label: '清零', values: { top: 0, bottom: 0, left: 0, right: 0 } }
]

let promptOptimizerTriggerElement = null
let assistantRequestSeq = 0
let agentEstimateTimer = null
let creditEstimateTimer = null
let creditEstimateAbortController = null
let creditEstimateRequestSeq = 0
let syncingAgentPlanToComposer = false
let suppressCreditEstimateWatch = false

const creditEstimateDebounceMs = 600
const creditEstimateUnavailableNotice = '点数预估暂不可用，不影响提交'

const promptAssistantFields = [
  { key: 'subject', label: '主体', icon: Sparkles },
  { key: 'scene', label: '场景', icon: ImageIcon },
  { key: 'style', label: '风格', icon: Brush },
  { key: 'usage', label: '用途', icon: Camera }
]

const assistantFieldEmptyText = 'AI 暂未判断，可补充'

const promptAssistantStarters = [
  {
    key: 'portrait',
    label: '人像',
    message: '人像方向：成年人物肖像，突出外貌气质、服装、镜头、光线和背景氛围。'
  },
  {
    key: 'product',
    label: '商品',
    message: '商品方向：突出单个产品主体、材质、陈列场景、商业摄影光线和使用场景。'
  },
  {
    key: 'scene',
    label: '场景',
    message: '场景方向：整理空间环境、时间、天气、构图、氛围和视觉风格。'
  }
]

// 示例提示词
const examplePrompts = [
  '一只可爱的猫咪在花园里玩耍',
  '未来城市的夜景，霓虹灯闪烁',
  '宁静的湖边日落，倒影清晰'
]

// 风格预设
const stylePresets = ['写实', '插画', '海报', '电商', '科幻', '国风']

const supportedGenerationToolModes = new Set([
  'generate',
  'redraw',
  'erase',
  'expand',
  'upscale',
  'remove_background',
  'precision_edit'
])
const supportedDiscoveryToolModes = new Set([
  'expand',
  'erase',
  'remove_background',
  'upscale',
  'precision_edit'
])

const workspaceCardImages = {
  expand: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/workspace-card-covers/2026-06-02/expand.png',
  erase: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/workspace-card-covers/2026-06-02/erase.png',
  remove_background: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/workspace-card-covers/2026-06-02/remove-background.png',
  upscale: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/workspace-card-covers/2026-06-02/upscale.png',
  precision_edit: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/workspace-card-covers/2026-06-02/precision-edit.png',
  old_photo_restoration: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/workspace-card-covers/2026-06-02/old-photo-restoration.png',
  momentsMarketing: productImage,
  articleImages: landscapeImage,
  coupleAlbum: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/workspace-card-covers/2026-06-02/couple-album.png',
  childhoodDreamAlbum: 'https://example-assets.oss-cn-shenzhen.aliyuncs.com/assets/workspace-card-covers/2026-06-02/childhood-dream-album.png',
  virtualTryOn: portraitImage
}

const fallbackToolCards = [
  { mode: 'expand', title: '智能扩图', description: '延展画面边界', icon: Maximize2, image: workspaceCardImages.expand },
  { mode: 'erase', title: '移除物体', description: '清理干扰元素', icon: MousePointer2, image: workspaceCardImages.erase },
  {
    mode: 'remove_background',
    title: '移除背景',
    description: '保留主体轮廓',
    icon: ImageIcon,
    image: workspaceCardImages.remove_background,
    requires_source: true,
    source_limit: 1,
    form_schema: [
      { key: 'edit_instruction', label: '主体保留说明（可选）', type: 'textarea' }
    ]
  },
  {
    mode: 'upscale',
    title: '高清放大',
    description: '增强细节质感',
    icon: Sparkles,
    image: workspaceCardImages.upscale,
    requires_source: true,
    source_limit: 1,
    form_schema: [
      { key: 'scale', label: '倍率', type: 'select', default: '2x', options: ['2x', '4x', '8x'] },
      { key: 'edit_instruction', label: '增强说明（可选）', type: 'textarea' }
    ]
  },
  {
    mode: 'precision_edit',
    title: '精细编辑',
    description: '圈选局部定向改图',
    icon: Edit3,
    image: workspaceCardImages.precision_edit,
    requires_source: true,
    source_limit: 1,
    form_schema: [
      { key: 'edit_instruction', label: '编辑指令', type: 'textarea' },
      { key: 'mask', label: '蒙版', type: 'mask' }
    ]
  },
]

const playgroundCards = [
  {
    id: 'virtual-try-on',
    title: '建模试衣',
    description: '上传身型参数和服装图，生成一张上身效果图。',
    tags: ['身型参数', '服装上身', '场景试衣'],
    route: '/workspace/virtual-try-on',
    icon: Shirt,
    image: workspaceCardImages.virtualTryOn
  },
  {
    id: 'moments-marketing',
    title: '朋友圈广告营销',
    description: '输入卖点或上传实图，一键生成朋友圈文案和宣传图。',
    tags: ['朋友圈文案', '营销海报', '九宫格'],
    route: '/workspace/moments-marketing',
    icon: Megaphone,
    image: workspaceCardImages.momentsMarketing
  },
  {
    id: 'article-images',
    title: '公众号文章配图',
    description: '粘贴文章正文，一键拆解封面、段落配图和金句卡片。',
    tags: ['公众号封面', '段落配图', '金句卡片'],
    route: '/workspace/article-images',
    icon: Newspaper,
    image: workspaceCardImages.articleImages
  },
  {
    id: 'couple-album',
    title: '情侣相册',
    description: '上传双人照片，生成可分享的旅行故事相册。',
    tags: ['双人照片', '旅行故事', '分享相册'],
    route: '/workspace/couple-album',
    icon: Heart,
    image: workspaceCardImages.coupleAlbum
  },
  {
    id: 'childhood-dream-album',
    title: '童年梦想相册',
    description: '用儿童照片生成职业梦想主题的连续故事相册。',
    tags: ['儿童照片', '职业梦想', '故事相册'],
    route: '/workspace/childhood-dream-album',
    icon: Sparkles,
    image: workspaceCardImages.childhoodDreamAlbum
  }
]

const quickPromptChips = [
  {
    key: 'commerce-hero',
    label: '电商主图直出',
    prompt: '电商主图，单个商品居中展示，干净浅色背景，商业摄影布光，突出材质、卖点和高级质感，适合直接用于商品首图。',
    aspect_ratio: '1:1',
    style_preset: '电商',
    tool_mode: 'generate'
  },
  {
    key: 'scene-remix',
    label: '爆款场景复刻',
    prompt: '参考爆款电商场景，重建同款构图、灯光、陈列层次和消费氛围，商品主体清晰，画面适合社媒投放。',
    aspect_ratio: '4:3',
    style_preset: '电商',
    tool_mode: 'generate'
  },
  {
    key: 'detail-page',
    label: '商品详情页',
    prompt: '商品详情页视觉，展示产品核心卖点、局部细节、使用场景和质感特写，版式干净，适合长图页面切片。',
    aspect_ratio: '3:4',
    style_preset: '电商',
    tool_mode: 'generate'
  },
  {
    key: 'ai-model',
    label: 'AI模特试衣',
    prompt: '成年 AI 模特试衣图，模特自然站姿，服装版型清楚，面料纹理真实，棚拍柔光，适合服饰电商展示。',
    aspect_ratio: '3:4',
    style_preset: '写实',
    tool_mode: 'generate'
  }
]

const fallbackWorkflowCards = [
  {
    id: 'hero-product',
    title: '电商主图直出',
    description: '商品、背景、卖点氛围一次成图',
    preview_url: productImage,
    prompt: quickPromptChips[0].prompt,
    aspect_ratio: '1:1',
    style_preset: '电商',
    tool_mode: 'generate'
  },
  {
    id: 'model-fitting',
    title: 'AI 模特试衣',
    description: '服饰上身、姿态和棚拍光线',
    preview_url: portraitImage,
    prompt: quickPromptChips[3].prompt,
    aspect_ratio: '3:4',
    style_preset: '写实',
    tool_mode: 'generate'
  },
  {
    id: 'detail-visual',
    title: '详情页卖点图',
    description: '细节、材质、场景组合展示',
    preview_url: interiorImage,
    prompt: quickPromptChips[2].prompt,
    aspect_ratio: '3:4',
    style_preset: '电商',
    tool_mode: 'generate'
  },
  {
    id: 'scene-remix',
    title: '爆款场景复刻',
    description: '复刻构图、灯光和陈列层次',
    preview_url: cityImage,
    prompt: quickPromptChips[1].prompt,
    aspect_ratio: '4:3',
    style_preset: '电商',
    tool_mode: 'generate'
  },
  {
    id: 'background-clean',
    title: '商品背景清理',
    description: '主体保留，背景替换为干净商业图',
    preview_url: workspaceCardImages.remove_background,
    prompt: removeBackgroundDefaultPrompt,
    aspect_ratio: '1:1',
    style_preset: '电商',
    tool_mode: 'remove_background'
  },
  {
    id: 'high-res',
    title: '高清放大交付',
    description: '增强细节纹理，适合投放素材',
    preview_url: workspaceCardImages.upscale,
    prompt: upscaleDefaultPrompt,
    aspect_ratio: '1:1',
    style_preset: '写实',
    tool_mode: 'upscale'
  }
]

const qualityOptions = [
  { key: 'low', label: '0.5K' },
  { key: 'medium', label: '1K' },
  { key: 'high', label: '2K' },
  { key: 'ultra', label: '4K' }
]


const templateFallbackImages = [
  portraitImage,
  cityImage,
  landscapeImage,
  productImage,
  interiorImage
]

const activeGenerationStorageKey = 'image-agent.workspace.active-generation'
const workspacePrefillStorageKey = 'image_agent_workspace_prefill:v1'
const workspaceDraftStorageKey = 'image_agent_workspace_draft:v1'
const activeGenerationStatuses = new Set(['queued', 'running'])
const isLoggedIn = computed(() => Boolean(me.value?.username))
let restoringWorkspaceState = false

// 任务状态映射
const stagePresentationMap = {
  queued: {
    shortLabel: '排队中',
    title: '任务已创建',
    description: '服务端已接收请求，正在排队处理。'
  },
  requesting_provider: {
    shortLabel: '请求模型',
    title: '正在请求模型',
    description: '平台正在调用图片生成链路。'
  },
  persisting_result: {
    shortLabel: '保存结果',
    title: '正在保存作品',
    description: '结果已返回，正在落到正式作品库。'
  },
  succeeded: {
    shortLabel: '已完成',
    title: '作品已完成',
    description: '图片已写入作品库，可随时回看和下载。'
  },
  failed: {
    shortLabel: '失败',
    title: '任务失败',
    description: '这次生成未能完成，请检查提示词后重试。'
  }
}

let pollTimer = null
let eraseDrawing = false
let eraseActiveStroke = null

watch(currentUser, (payload) => {
  me.value = payload
})

const {
  imageRef: previewZoomImageRef,
  zoomStyle: previewZoomStyle,
  handleWheel: handlePreviewWheel,
  resetZoom: resetPreviewZoom
} = usePointerZoom()

// 计算属性
const lowCredit = computed(() => (me.value?.available_credits ?? 0) <= 0)
const latestWorks = computed(() => Array.isArray(recentWorks.value) ? recentWorks.value.slice(0, 6) : [])
const latestSavedWork = computed(() => latestWorks.value[0] || null)
const discoveryCaseTemplates = computed(() => {
  const seen = new Set()
  return [...discoveryHotTemplates.value, ...discoveryInspirationTemplates.value]
    .filter((item) => {
      if (!item?.id || seen.has(item.id)) return false
      seen.add(item.id)
      return true
    })
})
const ecommerceWorkflowCards = computed(() => {
  const cards = discoveryCaseTemplates.value.map((item) => ({
    ...item,
    description: item.description || '后台模板，可直接套用到当前创作框'
  }))
  const seen = new Set(cards.map((item) => String(item.id)))
  fallbackWorkflowCards.forEach((item) => {
    if (seen.has(String(item.id))) return
    cards.push(item)
    seen.add(String(item.id))
  })
  return cards.slice(0, 6)
})
const workshopFeatureEntries = computed(() => [
  { key: 'ai-canvas', label: 'AI 画布', icon: PanelsTopLeft, disabled: true, message: 'AI 画布即将开放' },
  { key: 'skill-hub', label: 'SKILL HUB', icon: Sparkles, disabled: true, message: 'SKILL HUB 即将开放' },
  { key: 'ai-commerce', label: 'AI 电商', icon: ShoppingBag, action: 'commerce' },
  { key: 'video', label: '视频创作', icon: Video, route: '/workspace/video' },
  { key: 'image', label: '图片创作', icon: ImagePlus, mode: 'generate' },
  { key: 'image-edit', label: 'AI 改图', icon: Edit3, mode: 'precision_edit' },
  { key: 'text-edit', label: 'AI 改字', icon: Type, disabled: true, message: 'AI 改字即将开放' }
])
const selectedReferenceImages = computed(() =>
  selectedReferenceIds.value
    .map((assetId) => referenceAssets.value.find((item) => item.id === assetId))
    .filter(Boolean)
)
const selectedReferenceWorks = computed(() =>
  selectedReferenceWorkIds.value
    .map((workId) => selectedReferenceWorkSnapshots.value[workId] ?? findCachedWorkById(workId))
    .filter(Boolean)
    .map((work) => ({
      id: `work-${workID(work)}`,
      reference_kind: 'work',
      work_id: workID(work),
      preview_url: work.preview_url,
      original_filename: work.prompt || `作品 ${workID(work)}`
    }))
)
const selectedReferenceItems = computed(() => [
  ...selectedReferenceImages.value,
  ...selectedReferenceWorks.value
])
const editToolModes = new Set(['redraw', 'erase', 'expand', 'upscale', 'remove_background', 'precision_edit'])
const isEditTool = computed(() => editToolModes.has(toolMode.value))
const selectedReferenceCount = computed(() => selectedReferenceIds.value.length + selectedReferenceWorkIds.value.length)
const hasGenerationReferences = computed(() => selectedReferenceCount.value > 0)
const hasEditSourceImage = computed(() => Boolean(selectedSourceWorkId.value) || selectedReferenceCount.value > 0)
const showReferenceStrength = computed(() => !isEditTool.value && hasGenerationReferences.value)
const selectedModel = computed(() => workspaceModels.value.find((item) => Number(item.id) === Number(selectedModelId.value)) || null)
const displayedModelName = computed(() => selectedModel.value?.name || '白霖图像模型')
const aiToolCards = computed(() => {
  const iconMap = {
    maximize: Maximize2,
    eraser: MousePointer2,
    image: ImageIcon,
    sparkles: Sparkles,
    edit: Edit3
  }
  const imageMap = {
    expand: workspaceCardImages.expand,
    erase: workspaceCardImages.erase,
    remove_background: workspaceCardImages.remove_background,
    upscale: workspaceCardImages.upscale,
    precision_edit: workspaceCardImages.precision_edit,
    old_photo_restoration: workspaceCardImages.old_photo_restoration
  }
  const configuredByMode = new Map(workspaceTools.value.map((tool) => [tool.mode, tool]))
  const configuredExtras = workspaceTools.value.filter((tool) => !fallbackToolCards.some((fallback) => fallback.mode === tool.mode))
  const source = [
    ...fallbackToolCards.map((fallback) => ({
      ...fallback,
      ...(configuredByMode.get(fallback.mode) || {})
    })),
    ...configuredExtras
  ]
  return source
    .filter((tool) => tool && tool.enabled !== false)
    .map((tool) => ({
      ...tool,
      icon: typeof tool.icon === 'string' ? (iconMap[tool.icon] || Sparkles) : (tool.icon || Sparkles),
      image: tool.image || imageMap[tool.mode] || productImage,
      form_schema: Array.isArray(tool.form_schema) ? tool.form_schema : []
    }))
})
const selectedTool = computed(() => aiToolCards.value.find((item) => item.mode === toolMode.value) || null)
const selectedToolFields = computed(() => selectedTool.value?.form_schema || [])
const renderedToolFields = computed(() => selectedToolFields.value.filter((field) => field.type !== 'mask' && !(isPrecisionEditTool.value && field.key === 'edit_instruction')))
const isExpandTool = computed(() => toolMode.value === 'expand')
const isEraseTool = computed(() => toolMode.value === 'erase')
const isRemoveBackgroundTool = computed(() => toolMode.value === 'remove_background')
const isUpscaleTool = computed(() => toolMode.value === 'upscale')
const isPrecisionEditTool = computed(() => toolMode.value === 'precision_edit')
const isMaskSelectionTool = computed(() => isEraseTool.value || isPrecisionEditTool.value)
const activeReferenceToolMode = computed(() => workspaceMode.value === 'agent' && agentPlan.value?.tool_mode ? agentPlan.value.tool_mode : toolMode.value)
const singleSourceToolModes = new Set(['expand', 'upscale', 'remove_background', 'precision_edit'])
const sourceImageLimit = computed(() => Number(selectedTool.value?.source_limit) || (singleSourceToolModes.has(activeReferenceToolMode.value) ? 1 : 4))
const effectivePrompt = computed(() => {
  const cleanPrompt = prompt.value.trim()
  if (cleanPrompt) {
    if (isUpscaleTool.value && editInstruction.value.trim()) {
      return `${cleanPrompt}\n增强说明：${editInstruction.value.trim()}`
    }
    return cleanPrompt
  }
  if (isPrecisionEditTool.value) return ''
  if (isExpandTool.value && hasEditSourceImage.value) return expandDefaultPrompt
  if (isEraseTool.value && hasEditSourceImage.value) {
    if (editInstruction.value.trim()) return editInstruction.value.trim()
    if (hasEraseMask.value) return eraseDefaultPrompt
  }
  if (isRemoveBackgroundTool.value && hasEditSourceImage.value) {
    const instruction = editInstruction.value.trim()
    return instruction ? `${removeBackgroundDefaultPrompt}\n主体保留说明：${instruction}` : removeBackgroundDefaultPrompt
  }
  if (isUpscaleTool.value) {
    const instruction = editInstruction.value.trim()
    return instruction ? `${upscaleDefaultPrompt}\n增强说明：${instruction}` : upscaleDefaultPrompt
  }
  return ''
})
const currentEstimatedCredits = computed(() => {
  if (creditEstimate.value?.required_credits !== undefined) {
    return Number(creditEstimate.value.required_credits) || 0
  }
  return 1
})
const estimateBlocksSubmit = computed(() => Boolean(effectivePrompt.value) && (
  creditEstimateLoading.value ||
  creditEstimate.value?.enough === false ||
  (Boolean(creditEstimateError.value) && !creditEstimateCanCreateWithoutEstimate.value)
))

const selectedTask = computed(() => {
  if (!selectedTaskId.value) return null
  return activeTasks.value.find((item) => Number(item.generation_id) === Number(selectedTaskId.value)) || null
})
const task = computed(() => selectedTask.value && isActiveGenerationStatus(selectedTask.value.status) ? selectedTask.value : null)

// 渐进聚焦 hero：欢迎态 → 工作态单向锁存，避免失焦回弹
const heroEngaged = ref(false)

function engageHero() {
  heroEngaged.value = true
}

watch(
  [prompt, task, latestWorks, agentMessages],
  () => {
    if (heroEngaged.value) return
    if (
      prompt.value.trim() !== '' ||
      task.value !== null ||
      latestWorks.value.length > 0 ||
      agentMessages.value.some((message) => message.role === 'user')
    ) {
      heroEngaged.value = true
    }
  },
  { immediate: true, deep: true }
)

const failedTask = computed(() => selectedTask.value?.status === 'failed' ? selectedTask.value : null)
const selectedResultError = computed(() => failedTask.value ? 'failed' : resultError.value)
const agentExecutionTask = computed(() => {
  if (!agentExecutionTaskId.value) return null
  return activeTasks.value.find((item) => Number(item.generation_id) === Number(agentExecutionTaskId.value)) || null
})
const agentExecutionStageCopy = computed(() => {
  const currentTask = agentExecutionTask.value
  if (!currentTask) return null
  if (Number(currentTask.poll_failure_count || 0) > 0) {
    return {
      shortLabel: '重连中',
      title: '正在重新连接任务状态',
      description: '网络状态暂时不稳定，任务仍在服务端继续生成。'
    }
  }
  const key = currentTask.stage || currentTask.status || 'queued'
  return stagePresentationMap[key] ?? stagePresentationMap.queued
})
const agentPlanRequiresSource = computed(() => editToolModes.has(agentPlan.value?.tool_mode || ''))
const agentMissingSource = computed(() => Boolean(agentPlan.value?.prompt) && agentPlanRequiresSource.value && !hasEditSourceImage.value)
const agentHasLatestEstimate = computed(() => {
  if (!agentPlan.value?.prompt || agentEstimateDirty.value || !creditEstimate.value) return false
  const payloadKey = payloadSignature(buildAgentGenerationPayload({ syncComposer: false }) || {})
  return payloadKey === payloadSignature(agentLastEstimatedPayload.value || {})
})
const agentCanConfirmGenerate = computed(() => Boolean(
  agentPlan.value?.prompt &&
  !agentPlanning.value &&
  !submitting.value &&
  !creditEstimateLoading.value &&
  !agentClarificationPrompt.value &&
  !agentMissingSource.value &&
  agentHasLatestEstimate.value &&
  creditEstimate.value?.enough !== false &&
  !creditEstimateError.value
))
const agentConfirmDisabledReason = computed(() => {
  if (agentPlanning.value) return '正在规划：请等待 Agent 完成方案。'
  if (agentClarificationPrompt.value) return '需要先补充需求，确认追问后再生成。'
  if (!agentPlan.value?.prompt) return '无方案：请先描述需求，让 Agent 整理方案。'
  if (agentMissingSource.value) return '缺少编辑源图：请上传参考图或选择作品库图片。'
  if (creditEstimateLoading.value || agentEstimateDirty.value) return '正在预估最新点数，完成后才能扣点生成。'
  if (creditEstimateError.value) return '预估失败：请先重试预估。'
  if (creditEstimate.value?.enough === false) return '点数不足：请先前往套餐与充值。'
  if (!agentHasLatestEstimate.value) return '等待最新点数预估。'
  if (submitting.value) return '正在提交生成任务。'
  return ''
})

const previewAsset = computed(() => {
  if (selectedTask.value?.status === 'failed') {
    return null
  }
  if (selectedTask.value?.status === 'succeeded') {
    const matched = selectedTask.value.work_id ? findCachedWorkById(selectedTask.value.work_id) : null
    return {
      ...(matched ?? {}),
      ...selectedTask.value
    }
  }
  if (selectedTask.value && isActiveGenerationStatus(selectedTask.value.status)) {
    if (selectedTask.value.preview_url) {
      return selectedTask.value
    }
    return null
  }
  if (result.value) {
    const matched = result.value.work_id ? findCachedWorkById(result.value.work_id) : null
    return {
      ...(matched ?? {}),
      ...result.value
    }
  }
  return latestSavedWork.value
})
const isRemoveBackgroundResult = computed(() => previewAsset.value?.tool_mode === 'remove_background')
const promptLabel = computed(() => {
  if (isEraseTool.value) return '描述要移除的物体'
  if (isPrecisionEditTool.value) return '局部编辑指令'
  if (isExpandTool.value) return '描述外扩区域要补全的内容'
  if (isRemoveBackgroundTool.value) return '主体保留说明（可选）'
  if (isUpscaleTool.value) return '增强说明（可选）'
  return '根据文本描述或参考图片生成图片'
})
const promptPlaceholder = computed(() => {
  if (isEraseTool.value) return '例如：移除画面左侧路人，补全墙面纹理和光影'
  if (isPrecisionEditTool.value) return '输入局部编辑指令，例如：把圈选区域改成红色礼盒，保持周围光影和材质不变'
  if (isExpandTool.value) return expandDefaultPrompt
  if (isRemoveBackgroundTool.value) return '可补充要保留的细节，例如：保留发丝、商品阴影轮廓和透明材质边缘'
  if (isUpscaleTool.value) return upscaleDefaultPrompt
  return '描述你的想法，或上传参考图生成图片'
})
const referenceUploadTitle = computed(() => {
  if (isPrecisionEditTool.value) return '上传需要精细编辑的图片'
  if (isEraseTool.value) return '上传需要清理的图片'
  if (isExpandTool.value) return '上传一张需要扩展的图片'
  if (isRemoveBackgroundTool.value) return '上传需要移除背景的图片'
  if (isUpscaleTool.value) return '上传需要高清放大的图片'
  return '点击/拖拽/粘贴以上传图片'
})
const referenceUploadHint = computed(() => {
  if (isPrecisionEditTool.value) return 'JPG/PNG/WEBP，精细编辑仅使用 1 张源图，单张小于50MB'
  if (isEraseTool.value) return 'JPG/PNG/WEBP，移除物体仅使用 1 张源图，单张小于50MB'
  if (isExpandTool.value) return 'JPG/PNG/WEBP，智能扩图仅使用 1 张源图，单张小于50MB'
  if (isRemoveBackgroundTool.value) return 'JPG/PNG/WEBP，移除背景仅使用 1 张源图，单张小于50MB'
  if (isUpscaleTool.value) return 'JPG/PNG/WEBP，高清放大仅使用 1 张源图，单张小于50MB'
  return 'JPG/PNG/WEBP，单张小于50MB'
})

const activeStageKey = computed(() => {
  return task.value?.stage || task.value?.status || null
})

const stageCopy = computed(() => {
  if (task.value && Number(task.value.poll_failure_count || 0) > 0) {
    return {
      shortLabel: '重连中',
      title: '正在重新连接任务状态',
      description: '网络状态暂时不稳定，任务仍在服务端继续生成。'
    }
  }
  if (!activeStageKey.value) return null
  return stagePresentationMap[activeStageKey.value] ?? stagePresentationMap.queued
})

const hasEraseOperation = computed(() => !isEraseTool.value || Boolean(prompt.value.trim() || editInstruction.value.trim() || hasEraseMask.value))
const hasPrecisionEditOperation = computed(() => !isPrecisionEditTool.value || Boolean(hasEditSourceImage.value && prompt.value.trim() && hasEraseMask.value))
const canSubmit = computed(() => !submitting.value && !!effectivePrompt.value && (!isEditTool.value || hasEditSourceImage.value) && hasEraseOperation.value && hasPrecisionEditOperation.value && !estimateBlocksSubmit.value)
const canRetryGeneration = computed(() => !!failedTask.value && !submitting.value)
const selectedTaskCancelled = computed(() => failedTask.value ? isUserCancelledGeneration(failedTask.value) : false)
const canCancelGeneration = computed(() => !!task.value && !isTaskCancelling(task.value))
const cancelGenerationLoading = computed(() => task.value ? isTaskCancelling(task.value) : false)
const showAssistantStarters = computed(() => {
  return !prompt.value.trim() &&
    !assistantDraftPrompt.value.trim() &&
    assistantMessages.value.length <= 1 &&
    !assistantRunning.value
})
const showAssistantResultBubble = computed(() => {
  return !assistantRunning.value &&
    (
      Boolean(assistantDraftPrompt.value.trim()) ||
      hasAssistantStructuredValue(assistantStructuredPrompt.value) ||
      assistantDirections.value.length > 0 ||
      assistantNotes.value.length > 0 ||
      Boolean(assistantError.value)
    )
})

const assistantRunningMessage = computed(() => {
  const action = assistantRunningAction.value
  if (action === 'start') return '正在初步整理...'
  if (action === 'make_realistic') return '正在写实化...'
  if (action === 'change_direction') return '正在换方向...'
  if (action === 'rewrite') return '正在重新整理...'
  return '正在整理...'
})

const selectedSourceWork = computed(() => {
  if (!selectedSourceWorkId.value) return null
  return findCachedWorkById(selectedSourceWorkId.value)
})

const expandSourcePreview = computed(() => {
  if (!isExpandTool.value) return null
  if (selectedReferenceImages.value.length > 0) {
    return selectedReferenceImages.value[0]
  }
  return selectedSourceWork.value
})
const eraseSourcePreview = computed(() => {
  if (!isMaskSelectionTool.value) return null
  if (selectedReferenceImages.value.length > 0) {
    return selectedReferenceImages.value[0]
  }
  return selectedSourceWork.value
})
const eraseBrushSize = ref(34)
const maskSelectionMode = ref('brush')
const eraseMaskStrokes = ref([])
const hasEraseMask = computed(() => eraseMaskStrokes.value.length > 0)
const eraseMaskRegions = computed(() => eraseMaskStrokes.value
  .map((stroke) => eraseStrokeRegion(stroke))
  .filter(Boolean)
)

const expandEdges = computed(() => {
  const edges = {}
  expandEdgeKeys.forEach((key) => {
    edges[key] = boundedToolOption(toolOptions.value[key], 0, 100, 20)
  })
  return edges
})

const expandPreviewStyle = computed(() => {
  const edges = expandEdges.value
  const totalWidth = 100 + edges.left + edges.right
  const totalHeight = 100 + edges.top + edges.bottom
  return {
    aspectRatio: `${totalWidth} / ${totalHeight}`
  }
})

const expandOriginalStyle = computed(() => {
  const edges = expandEdges.value
  const totalWidth = 100 + edges.left + edges.right
  const totalHeight = 100 + edges.top + edges.bottom
  return {
    left: `${(edges.left / totalWidth) * 100}%`,
    top: `${(edges.top / totalHeight) * 100}%`,
    width: `${(100 / totalWidth) * 100}%`,
    height: `${(100 / totalHeight) * 100}%`
  }
})

function canvasPointFromEvent(event) {
  const canvas = eraseMaskCanvasRef.value
  if (!canvas) return null
  const rect = canvas.getBoundingClientRect()
  const width = rect.width || canvas.width || 1
  const height = rect.height || canvas.height || 1
  const x = Math.min(1, Math.max(0, ((event.clientX ?? rect.left) - rect.left) / width))
  const y = Math.min(1, Math.max(0, ((event.clientY ?? rect.top) - rect.top) / height))
  return { x, y }
}

function eraseStrokeRegion(stroke) {
  if (!stroke?.points?.length) return null
  const xs = stroke.points.map((point) => point.x)
  const ys = stroke.points.map((point) => point.y)
  const pad = stroke.mode === 'lasso' ? 0 : Math.min(0.18, Math.max(0.025, Number(stroke.brushSize || 34) / 720))
  const left = Math.max(0, Math.min(...xs) - pad)
  const top = Math.max(0, Math.min(...ys) - pad)
  const right = Math.min(1, Math.max(...xs) + pad)
  const bottom = Math.min(1, Math.max(...ys) + pad)
  return {
    x: Number(left.toFixed(4)),
    y: Number(top.toFixed(4)),
    width: Number(Math.max(0.001, right - left).toFixed(4)),
    height: Number(Math.max(0.001, bottom - top).toFixed(4))
  }
}

function redrawEraseMaskPreview() {
  const canvas = eraseMaskCanvasRef.value
  if (!canvas) return
  if (eraseMaskStrokes.value.length === 0) {
    canvas.width = canvas.width || 720
    return
  }
  const context = canvas.getContext?.('2d')
  if (!context) return
  const width = canvas.width || 720
  const height = canvas.height || 480
  context.clearRect(0, 0, width, height)
  context.lineCap = 'round'
  context.lineJoin = 'round'
  eraseMaskStrokes.value.forEach((stroke) => {
    if (!stroke.points?.length) return
    drawMaskSelectionShape(context, stroke, width, height, {
      brushColor: 'rgba(248, 113, 113, 0.72)',
      lassoColor: 'rgba(248, 113, 113, 0.38)'
    })
  })
}

function beginEraseMaskStroke(event) {
  if (!isMaskSelectionTool.value || !eraseSourcePreview.value || submitting.value) return
  eraseMaskCanvasRef.value = event.currentTarget || eraseMaskCanvasRef.value
  const point = canvasPointFromEvent(event)
  if (!point) return
  eraseDrawing = true
  eraseActiveStroke = {
    mode: isPrecisionEditTool.value ? maskSelectionMode.value : 'brush',
    brushSize: eraseBrushSize.value,
    points: [point]
  }
  if (eraseActiveStroke.mode !== 'lasso') {
    eraseMaskStrokes.value = [...eraseMaskStrokes.value, eraseActiveStroke]
  }
  event.currentTarget?.setPointerCapture?.(event.pointerId)
  redrawEraseMaskPreview()
}

function moveEraseMaskStroke(event) {
  if (!eraseDrawing || !eraseActiveStroke) return
  eraseMaskCanvasRef.value = event.currentTarget || eraseMaskCanvasRef.value
  const point = canvasPointFromEvent(event)
  if (!point) return
  eraseActiveStroke.points.push(point)
  eraseMaskStrokes.value = [...eraseMaskStrokes.value]
  redrawEraseMaskPreview()
}

function endEraseMaskStroke(event) {
  if (!eraseDrawing) return
  eraseMaskCanvasRef.value = event?.currentTarget || eraseMaskCanvasRef.value
  if (eraseActiveStroke?.mode === 'lasso') {
    if (eraseActiveStroke.points.length >= 3) {
      eraseMaskStrokes.value = [...eraseMaskStrokes.value, eraseActiveStroke]
    }
  } else if (eraseActiveStroke && eraseActiveStroke.points.length === 1) {
    eraseActiveStroke.points.push({ ...eraseActiveStroke.points[0] })
    eraseMaskStrokes.value = [...eraseMaskStrokes.value]
  }
  eraseDrawing = false
  eraseActiveStroke = null
  event?.currentTarget?.releasePointerCapture?.(event.pointerId)
  redrawEraseMaskPreview()
}

function undoEraseMaskStroke() {
  if (submitting.value || eraseMaskStrokes.value.length === 0) return
  eraseMaskStrokes.value = eraseMaskStrokes.value.slice(0, -1)
  void nextTick(redrawEraseMaskPreview)
}

function clearEraseMask() {
  eraseDrawing = false
  eraseActiveStroke = null
  eraseMaskStrokes.value = []
  void nextTick(redrawEraseMaskPreview)
}

function selectMaskSelectionMode(mode) {
  if (submitting.value) return
  maskSelectionMode.value = mode === 'lasso' ? 'lasso' : 'brush'
}

function drawMaskSelectionShape(context, stroke, width, height, { brushColor, lassoColor }) {
  if (!stroke?.points?.length) return
  context.beginPath()
  context.moveTo(stroke.points[0].x * width, stroke.points[0].y * height)
  stroke.points.slice(1).forEach((point) => {
    context.lineTo(point.x * width, point.y * height)
  })
  if (stroke.mode === 'lasso') {
    context.closePath?.()
    context.fillStyle = lassoColor
    context.fill()
    return
  }
  context.lineWidth = Number(stroke.brushSize) || 34
  context.strokeStyle = brushColor
  context.stroke()
}

function createEraseMaskFile() {
  const canvas = document.createElement('canvas')
  canvas.width = 1024
  canvas.height = 1024
  const context = canvas.getContext?.('2d')
  if (context) {
    context.fillStyle = '#000000'
    context.fillRect(0, 0, canvas.width, canvas.height)
    context.lineCap = 'round'
    context.lineJoin = 'round'
    eraseMaskStrokes.value.forEach((stroke) => {
      if (!stroke.points?.length) return
      drawMaskSelectionShape(context, {
        ...stroke,
        brushSize: Math.max(12, Math.round((Number(stroke.brushSize) || 34) * 2.8))
      }, canvas.width, canvas.height, {
        brushColor: '#ffffff',
        lassoColor: '#ffffff'
      })
    })
  }
  return new Promise((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (!blob) {
        reject(new Error('蒙版生成失败'))
        return
      }
      const prefix = isPrecisionEditTool.value ? 'precision-edit-mask' : 'erase-mask'
      resolve(new File([blob], `${prefix}-${Date.now()}.png`, { type: 'image/png' }))
    }, 'image/png')
  })
}

function responseItems(payload) {
  return Array.isArray(payload) ? payload : (payload?.items ?? [])
}

function isMissingReferenceEstimateError(error) {
  return error?.code === 'reference_asset_not_found' || error?.code === 'reference_work_not_found'
}

function estimateErrorMessage(error) {
  const message = `${error?.message || ''}`.trim()
  if (isMissingReferenceEstimateError(error)) {
    return `点数预估失败：${message || '参考素材不存在'}，请重新选择图片`
  }
  if (message && Number(error?.status) > 0 && Number(error?.status) < 500) {
    return `点数预估失败：${message}`
  }
  return '点数预估失败，可重试或直接创建'
}

function isSoftCreditEstimateError(error) {
  const status = Number(error?.status ?? 0)
  return error?.name === 'AbortError' ||
    error?.code === 'too_many_requests' ||
    error?.code === 'network_unreachable' ||
    status === 0 ||
    status >= 500
}

async function refreshReferencesAfterEstimateError(error) {
  if (!isMissingReferenceEstimateError(error)) return
  suppressCreditEstimateWatch = true
  try {
    if (error.code === 'reference_asset_not_found') {
      selectedReferenceIds.value = []
    }
    if (error.code === 'reference_work_not_found') {
      selectedReferenceWorkIds.value = []
      selectedReferenceWorkSnapshots.value = {}
    }
    selectedSourceWorkId.value = null
    await nextTick()
  } finally {
    suppressCreditEstimateWatch = false
  }
  if (error.code === 'reference_asset_not_found') {
    await loadReferenceAssets()
  } else if (error.code === 'reference_work_not_found') {
    await loadWorks()
  }
}

function workID(work) {
  return Number(work?.work_id ?? work?.id ?? 0)
}

function workCategory(work) {
  const category = String(work?.category || work?.type || '').trim()
  if (category) return category

  const mimeType = String(work?.mime_type || '').toLowerCase()
  if (mimeType.startsWith('video/')) return 'video'
  if (mimeType.startsWith('audio/')) return 'audio'
  return 'image'
}

const imageWorkshopCategories = new Set(['', 'image', 'poster_kv', 'product_main', 'cover'])

function isImageWorkshopWork(work) {
  if (!work) return false
  const category = String(work?.category || work?.type || '').trim().toLowerCase()
  const mimeType = String(work?.mime_type || '').trim().toLowerCase()
  const toolMode = String(work?.tool_mode || work?.parameters?.tool_mode || '').trim().toLowerCase()

  if (category === 'video' || category === 'audio') return false
  if (mimeType.startsWith('video/') || mimeType.startsWith('audio/')) return false
  if (toolMode === 'video') return false
  return imageWorkshopCategories.has(category)
}

function canUseWorkAsReference(work) {
  return !!workID(work) && !!work?.preview_url && workCategory(work) === 'image'
}

function findCachedWorkById(id) {
  const normalizedId = Number(id)
  if (!normalizedId) return null
  const caches = [
    works.value,
    recentWorks.value,
    Object.values(selectedReferenceWorkSnapshots.value)
  ]
  for (const cache of caches) {
    const matched = (Array.isArray(cache) ? cache : []).find((item) => workID(item) === normalizedId)
    if (matched) return matched
  }
  return null
}

function updateCachedWork(id, updater) {
  const normalizedId = Number(id)
  if (!normalizedId) return
  const applyUpdate = (items) => items.map((work) => workID(work) === normalizedId ? updater(work) : work)
  works.value = applyUpdate(works.value)
  recentWorks.value = applyUpdate(recentWorks.value)
  const snapshot = selectedReferenceWorkSnapshots.value[normalizedId]
  if (snapshot) {
    selectedReferenceWorkSnapshots.value = {
      ...selectedReferenceWorkSnapshots.value,
      [normalizedId]: updater(snapshot)
    }
  }
}

function readJSONStorage(storage, key) {
  try {
    const raw = storage?.getItem(key)
    return raw ? JSON.parse(raw) : null
  } catch (error) {
    storage?.removeItem(key)
    return null
  }
}

function clearWorkspaceDraft() {
  try {
    window.localStorage?.removeItem(workspaceDraftStorageKey)
  } catch (error) {
    console.warn('Failed to clear workspace draft:', error)
  }
}

function workspaceFormSnapshot() {
  return {
    prompt: prompt.value,
    negative_prompt: negativePrompt.value,
    aspect_ratio: aspectRatio.value,
    style_preset: stylePreset.value,
    tool_mode: toolMode.value,
    quality: quality.value,
    reference_weight: referenceWeight.value,
    model_id: selectedModelId.value,
    reference_asset_ids: [...selectedReferenceIds.value],
    reference_work_ids: [...selectedReferenceWorkIds.value],
    tool_options: toolOptions.value,
    edit_instruction: editInstruction.value
  }
}

function hasWorkspaceDraftValue(snapshot) {
  return Boolean(
    snapshot.prompt?.trim() ||
    snapshot.negative_prompt?.trim() ||
    snapshot.style_preset ||
    snapshot.tool_mode !== 'generate' ||
    snapshot.aspect_ratio !== '1:1' ||
    snapshot.reference_asset_ids?.length ||
    snapshot.reference_work_ids?.length ||
    snapshot.edit_instruction?.trim()
  )
}

function persistWorkspaceDraft() {
  if (restoringWorkspaceState || loading.value) return
  const snapshot = workspaceFormSnapshot()
  try {
    if (hasWorkspaceDraftValue(snapshot)) {
      window.localStorage?.setItem(workspaceDraftStorageKey, JSON.stringify(snapshot))
    } else {
      clearWorkspaceDraft()
    }
  } catch (error) {
    console.warn('Failed to persist workspace draft:', error)
  }
}

function applyWorkspaceState(payload = {}, options = {}) {
  const { activateCreate = true } = options
  restoringWorkspaceState = true
  try {
    prompt.value = payload.prompt || ''
    negativePrompt.value = payload.negative_prompt || ''
    aspectRatio.value = payload.aspect_ratio || payload.aspect || '1:1'
    stylePreset.value = payload.style_preset || ''
    toolMode.value = payload.tool_mode || 'generate'
    quality.value = payload.quality || 'medium'
    referenceWeight.value = Number(payload.reference_weight ?? 75)
    if (payload.model_id) {
      selectedModelId.value = Number(payload.model_id)
    }
    toolOptions.value = payload.tool_options && typeof payload.tool_options === 'object'
      ? payload.tool_options
      : defaultToolOptions(selectedTool.value)
    editInstruction.value = payload.edit_instruction || ''
    selectedReferenceIds.value = Array.isArray(payload.reference_asset_ids)
      ? payload.reference_asset_ids.map(Number).filter(Boolean)
      : []
    const prefillWorkIds = [
      ...(Array.isArray(payload.reference_work_ids) ? payload.reference_work_ids : []),
      payload.reference_work_id
    ].map(Number).filter(Boolean)
    selectedReferenceWorkIds.value = prefillWorkIds
    selectedReferenceWorkSnapshots.value = Object.fromEntries(prefillWorkIds
      .map((id) => findCachedWorkById(id))
      .filter(Boolean)
      .map((work) => [workID(work), { ...work, category: workCategory(work) }])
    )
    selectedSourceWorkId.value = Number(payload.source_work_id || 0) || null
    if (activateCreate) {
      workspaceTab.value = 'create'
    }
    enforceSourceLimit()
  } finally {
    restoringWorkspaceState = false
  }
}

function restoreWorkspacePrefillOrDraft() {
  const prefill = readJSONStorage(window.sessionStorage, workspacePrefillStorageKey)
  if (prefill) {
    window.sessionStorage?.removeItem(workspacePrefillStorageKey)
    applyWorkspaceState(prefill, { activateCreate: false })
    return
  }
  const draft = readJSONStorage(window.localStorage, workspaceDraftStorageKey)
  if (draft) {
    applyWorkspaceState(draft, { activateCreate: false })
  }
}

function readableLoadError(error, fallback) {
  const message = `${error?.message || ''}`.trim()
  if (!message || message === 'Failed to fetch') return fallback
  return message
}

// 方法
async function loadSession() {
  try {
    me.value = await loadCurrentUser({ force: true, clearOnError: false })
  } catch (error) {
    if (error?.status === 401) {
      me.value = null
      return
    }
    me.value = currentUser.value
    if (!me.value) {
      pageError.value = error.message || '登录状态读取失败，请刷新重试'
    }
  }
}

function syncAvailableCredits(payload) {
  if (payload?.available_credits === undefined) return
  const sharedUser = applyAvailableCredits(payload.available_credits)
  me.value = {
    ...(me.value ?? sharedUser ?? {}),
    available_credits: sharedUser?.available_credits ?? payload.available_credits
  }
}

async function refreshSessionCredits() {
  const payload = await refreshCurrentUser()
  if (payload) {
    me.value = payload
  }
}

async function loadWorks() {
  worksError.value = ''
  worksLoading.value = true
  try {
    const payload = await api.listWorks({ media_type: 'image', page: 1, page_size: worksPageSize })
    const items = responseItems(payload).filter(isImageWorkshopWork)
    works.value = items
    recentWorks.value = items.slice(0, 6)
    worksPage.value = Number(payload?.page ?? 1) || 1
    worksTotal.value = Number(payload?.total ?? items.length) || 0
  } catch (error) {
    console.error('Failed to load works:', error)
    worksError.value = readableLoadError(error, '作品记录读取失败，可稍后重试')
  } finally {
    worksLoading.value = false
  }
}

async function changeWorksPage(page) {
  const nextPage = Number(page)
  const totalPages = Math.max(1, Math.ceil(worksTotal.value / worksPageSize))
  if (!Number.isFinite(nextPage) || nextPage < 1 || nextPage > totalPages || nextPage === worksPage.value || worksLoading.value) {
    return
  }
  worksError.value = ''
  worksLoading.value = true
  try {
    const payload = await api.listWorks({ media_type: 'image', page: nextPage, page_size: worksPageSize })
    const items = responseItems(payload).filter(isImageWorkshopWork)
    works.value = items
    worksPage.value = Number(payload?.page ?? nextPage) || nextPage
    worksTotal.value = Number(payload?.total ?? worksTotal.value) || items.length
    if (worksPage.value === 1) {
      recentWorks.value = items.slice(0, 6)
    }
  } catch (error) {
    console.error('Failed to load works page:', error)
    worksError.value = readableLoadError(error, '作品记录读取失败，可稍后重试')
  } finally {
    worksLoading.value = false
  }
}

async function loadReferenceAssets() {
  referenceError.value = ''
  try {
    referenceAssets.value = responseItems(await api.listReferenceAssets())
  } catch (error) {
    referenceError.value = readableLoadError(error, '参考图读取失败，可稍后重试')
  }
}

function normalizeDiscoveryTemplates(items = []) {
  return (Array.isArray(items) ? items : [])
    .filter((item) => item && item.title)
    .map((item, index) => ({
      id: item.id || `${item.slug || item.title}-${index}`,
      slug: item.slug || '',
      title: item.title || '',
      category: item.category || '',
      description: item.description || '',
      prompt: item.prompt || '',
      preview_url: item.preview_url || templateFallbackImages[index % templateFallbackImages.length],
      aspect_ratio: item.aspect_ratio || '1:1',
      style_preset: item.style_preset || '',
      tool_mode: supportedGenerationToolModes.has(item.tool_mode) ? item.tool_mode : 'generate',
      model_id: item.model_id || null
    }))
}

function normalizeInspirationRecommendations(items = []) {
  return (Array.isArray(items) ? items : [])
    .filter((item) => item && item.title && item.prompt)
    .map((item, index) => ({
      id: item.id || `${item.slug || item.title}-${index}`,
      slug: item.slug || '',
      title: item.title || '',
      category: item.category || '',
      description: item.description || '',
      heat_tags: Array.isArray(item.heat_tags) ? item.heat_tags.filter(Boolean) : [],
      prompt: item.prompt || '',
      negative_prompt: item.negative_prompt || '',
      preview_url: item.preview_url || templateFallbackImages[index % templateFallbackImages.length],
      aspect_ratio: item.aspect_ratio || '1:1',
      style_preset: item.style_preset || '',
      theme: item.theme || '',
      tool_mode: supportedGenerationToolModes.has(item.tool_mode) ? item.tool_mode : 'generate',
      model_id: item.model_id ? Number(item.model_id) : null,
      params: item.params && typeof item.params === 'object' && !Array.isArray(item.params) ? item.params : {},
      sort_order: Number(item.sort_order ?? index),
      use_count: Number(item.use_count || 0)
    }))
    .sort((left, right) => (left.sort_order || 0) - (right.sort_order || 0))
}

function normalizeWorkspaceTools(items = []) {
  return (Array.isArray(items) ? items : [])
    .filter((item) => item && item.mode && supportedDiscoveryToolModes.has(item.mode) && item.title && item.enabled !== false)
    .map((item, index) => ({
      mode: item.mode,
      title: item.title,
      description: item.description || '',
      icon: item.icon || '',
      enabled: item.enabled !== false,
      sort_order: Number(item.sort_order ?? index),
      requires_source: item.requires_source === true,
      source_limit: Number(item.source_limit) || 0,
      form_schema: Array.isArray(item.form_schema) ? item.form_schema : []
    }))
    .sort((left, right) => (left.sort_order || 0) - (right.sort_order || 0))
}

function normalizeWorkspaceModels(items = []) {
  return (Array.isArray(items) ? items : [])
    .filter((item) => item && item.id && item.name)
    .map((item, index) => ({
      id: Number(item.id),
      name: item.name || '',
      default_credits_cost: Number(item.default_credits_cost) || 1,
      capability_tags: Array.isArray(item.capability_tags) ? item.capability_tags : [],
      sort_order: Number(item.sort_order ?? index)
    }))
    .sort((left, right) => (left.sort_order || 0) - (right.sort_order || 0))
}

async function loadWorkspaceDiscovery() {
  discoveryError.value = ''
  if (typeof api.getWorkspaceDiscovery !== 'function') {
    discoveryHotTemplates.value = []
    discoveryInspirationTemplates.value = []
    discoveryRecommendations.value = []
    return
  }
  try {
    const payload = await api.getWorkspaceDiscovery()
    workspaceTools.value = normalizeWorkspaceTools(payload?.tools)
    workspaceModels.value = normalizeWorkspaceModels(payload?.models)
    if (!selectedModelId.value && workspaceModels.value.length > 0) {
      selectedModelId.value = workspaceModels.value[0].id
    }
    discoveryHotTemplates.value = normalizeDiscoveryTemplates(payload?.hot)
    discoveryInspirationTemplates.value = normalizeDiscoveryTemplates(payload?.inspiration)
    discoveryRecommendations.value = normalizeInspirationRecommendations(payload?.recommendations)
  } catch (error) {
    console.error('Failed to load workspace discovery:', error)
    discoveryError.value = readableLoadError(error, '发现内容读取失败，可稍后重试')
    workspaceTools.value = []
    workspaceModels.value = []
    discoveryHotTemplates.value = []
    discoveryInspirationTemplates.value = []
    discoveryRecommendations.value = []
  }
}

async function handleReferenceUpload(file) {
  if (!isLoggedIn.value) {
    requireLogin()
    return
  }
  if (referenceUploading.value || submitting.value) return
  if (file?.size > referenceAssetUploadMaxBytes) {
    referenceError.value = '单张图片不能超过 50MB'
    return
  }
  const maxImages = sourceImageLimit.value
  const replacingSingleSource = maxImages === 1 && (selectedReferenceCount.value >= maxImages || selectedSourceWorkId.value)
  if (selectedReferenceCount.value >= maxImages && !replacingSingleSource) {
    referenceError.value = `最多只能选择 ${maxImages} 张参考图`
    return
  }

  referenceError.value = ''
  referenceUploading.value = true
  try {
    const uploaded = await api.uploadReferenceAsset(file)
    referenceAssets.value = [
      uploaded,
      ...referenceAssets.value.filter((item) => item.id !== uploaded.id)
    ]
    if (replacingSingleSource) {
      selectedReferenceIds.value = []
      selectedReferenceWorkIds.value = []
      selectedReferenceWorkSnapshots.value = {}
      selectedSourceWorkId.value = null
    }
    selectedReferenceIds.value = [...selectedReferenceIds.value, uploaded.id]
    selectedSourceWorkId.value = null
    if (workspaceMode.value === 'agent' && agentPlan.value?.prompt) {
      markAgentEstimateDirty()
    }
  } catch (error) {
    referenceError.value = error.message || '参考图上传失败'
  } finally {
    referenceUploading.value = false
  }
}

function selectWorkspaceTab(tab) {
  workspaceMode.value = 'create'
  workspaceTab.value = tab === 'create' ? 'create' : 'discover'
}

function focusWorkshopPrompt() {
  void nextTick(() => {
    composerPanelRef.value?.focusPrompt?.()
  })
}

function selectWorkshopMode(mode) {
  if (mode === 'video') {
    router.push('/workspace/video')
    return
  }
  if (mode === 'virtual-try-on') {
    router.push('/workspace/virtual-try-on')
    return
  }
  if (mode === 'agent') {
    workspaceMode.value = 'agent'
    successMessage.value = ''
    agentModeError.value = ''
    agentStep.value = agentExecutionTask.value ? 'result' : (agentPlan.value?.prompt ? 'plan' : 'describe')
    workspaceTab.value = 'discover'
    return
  }
  workspaceMode.value = 'create'
  workspaceTab.value = 'create'
  focusWorkshopPrompt()
}

function normalizeAgentPlanForClient(plan = {}, fallback = {}) {
  const nextPlan = {
    ...fallback,
    ...plan
  }
  return {
    id: String(nextPlan.id || fallback.id || ''),
    title: (nextPlan.title || fallback.title || '图片创作方案').trim(),
    intent: (nextPlan.intent || fallback.intent || 'text_to_image').trim(),
    tool_mode: nextPlan.tool_mode || fallback.tool_mode || 'generate',
    prompt: (nextPlan.prompt || fallback.prompt || '').trim(),
    negative_prompt: (nextPlan.negative_prompt || fallback.negative_prompt || '').trim(),
    aspect_ratio: nextPlan.aspect_ratio || fallback.aspect_ratio || '1:1',
    style_preset: (nextPlan.style_preset || fallback.style_preset || '').trim(),
    quality: nextPlan.quality || fallback.quality || 'medium',
    reference_weight: Number(nextPlan.reference_weight ?? fallback.reference_weight ?? 75),
    tool_options: nextPlan.tool_options && typeof nextPlan.tool_options === 'object' ? nextPlan.tool_options : {},
    edit_instruction: (nextPlan.edit_instruction || fallback.edit_instruction || '').trim(),
    requires_confirmation: true
  }
}

function normalizeAgentCandidates(candidates = [], plan = null) {
  const source = Array.isArray(candidates) && candidates.length > 0
    ? candidates
    : (plan ? [{ ...plan, id: 'primary' }] : [])
  return source
    .map((candidate, index) => normalizeAgentPlanForClient({
      ...candidate,
      id: candidate.id || `candidate-${index + 1}`
    }, plan || {}))
    .filter((candidate) => candidate.prompt)
}

function payloadSignature(value) {
  if (!value || typeof value !== 'object') return JSON.stringify(value ?? null)
  if (Array.isArray(value)) {
    return `[${value.map((item) => payloadSignature(item)).join(',')}]`
  }
  return `{${Object.keys(value)
    .sort()
    .map((key) => `${JSON.stringify(key)}:${payloadSignature(value[key])}`)
    .join(',')}}`
}

function clearAgentEstimateTimer() {
  if (agentEstimateTimer !== null) {
    window.clearTimeout(agentEstimateTimer)
    agentEstimateTimer = null
  }
}

function cloneAgentRetryPayload(value) {
  if (Array.isArray(value)) {
    return value.map((item) => cloneAgentRetryPayload(item))
  }
  if (value && typeof value === 'object') {
    const prototype = Object.getPrototypeOf(value)
    if (prototype !== Object.prototype && prototype !== null) {
      return value
    }
    return Object.fromEntries(
      Object.entries(value).map(([key, item]) => [key, cloneAgentRetryPayload(item)])
    )
  }
  return value
}

function clearAgentFailure(phase = '') {
  if (!phase || agentFailure.value?.phase === phase) {
    agentFailure.value = null
  }
}

function isAgentReferenceError(error) {
  const code = String(error?.code || '')
  return code.startsWith('reference_asset_') ||
    code.startsWith('reference_work_') ||
    code === 'invalid_reference_asset_type'
}

function agentRetryable(error) {
  if (isAgentReferenceError(error)) return false
  if (error?.retryable !== undefined) return Boolean(error.retryable)
  const status = Number(error?.status ?? 0)
  if (error?.code === 'too_many_requests' || status === 429) return true
  if (error?.code === 'network_unreachable' || status === 0 || status >= 500) return true
  return false
}

function classifyAgentFailure(phase, error, retryPayload) {
  const status = Number(error?.status ?? 0)
  const code = String(error?.code || '')
  const retryAfter = Number(error?.retry_after_seconds ?? 0)
  const retryLabel = phase === 'plan' ? '重新生成方案' : '重新生成'
  const base = {
    phase,
    retryLabel,
    retryable: agentRetryable(error),
    retryPayload: cloneAgentRetryPayload(retryPayload || {})
  }

  if (code === 'network_unreachable' || status === 0) {
    return {
      ...base,
      reasonTitle: '网络不稳定',
      message: error?.message || '当前网络连接中断，请检查网络后重试。',
      suggestion: '请检查连接后重试，当前任务和参考素材会保留。'
    }
  }

  if (code === 'too_many_requests' || status === 429) {
    return {
      ...base,
      reasonTitle: '请求过于频繁',
      message: retryAfter > 0 ? `请求过于频繁，请等待 ${retryAfter} 秒后再试。` : (error?.message || '请求过于频繁，请稍后再试。'),
      suggestion: '请稍等片刻，不需要重新输入任务描述。'
    }
  }

  if (isAgentReferenceError(error)) {
    return {
      ...base,
      reasonTitle: '参考素材不可用',
      message: error?.message || '当前参考素材不可用或格式不兼容。',
      suggestion: '请重新上传或减少素材后再生成方案。'
    }
  }

  if (phase === 'plan' && (code === 'agent_image_plan_failed' || [502, 503, 504].includes(status))) {
    return {
      ...base,
      reasonTitle: '规划服务繁忙',
      message: error?.message || '方案规划服务繁忙或超时。',
      suggestion: '请稍后重试，当前任务描述和参考素材会保留。'
    }
  }

  return {
    ...base,
    reasonTitle: phase === 'plan' ? '方案规划失败' : '生成提交失败',
    message: phase === 'plan'
      ? '系统暂时异常，方案没有生成成功。'
      : '系统暂时异常，生成请求没有提交成功。',
    suggestion: phase === 'plan'
      ? '请稍后重试；也可以微调描述或参考素材后再提交。'
      : '请稍后重新生成；当前方案参数和点数预估上下文会保留。'
  }
}

function scheduleAgentEstimate() {
  clearAgentEstimateTimer()
  if (!agentPlan.value?.prompt || agentClarificationPrompt.value || agentMissingSource.value || !isLoggedIn.value) return
  agentEstimateTimer = window.setTimeout(() => {
    agentEstimateTimer = null
    void estimateAgentPlan()
  }, 300)
}

function markAgentEstimateDirty({ schedule = true } = {}) {
  agentEstimateDirty.value = Boolean(agentPlan.value?.prompt)
  agentLastEstimatedPayload.value = null
  creditEstimate.value = null
  creditEstimateError.value = ''
  creditEstimateNotice.value = ''
  if (schedule) {
    scheduleAgentEstimate()
  } else {
    clearAgentEstimateTimer()
  }
}

async function sendAgentMessage(message) {
  const cleanMessage = String(message || '').trim()
  if (!cleanMessage || agentPlanning.value) return
  if (!isLoggedIn.value) {
    requireLogin()
    return
  }
  agentModeError.value = ''
  clearAgentFailure()
  creditEstimateError.value = ''
  creditEstimateNotice.value = ''
  agentClarificationPrompt.value = ''
  markAgentEstimateDirty({ schedule: false })
  const nextMessages = [
    ...agentMessages.value,
    { role: 'user', content: cleanMessage }
  ]
  const requestPayload = {
    message: cleanMessage,
    history: nextMessages.slice(-12).map((item) => ({
      role: item.role,
      content: item.content
    })),
    reference_asset_ids: [...selectedReferenceIds.value],
    reference_work_ids: [...selectedReferenceWorkIds.value],
    current_plan: agentPlan.value ? { ...agentPlan.value } : null
  }
  agentMessages.value = nextMessages
  agentPlanning.value = true
  try {
    const payload = await api.planImageAgent(requestPayload)
    agentClarificationPrompt.value = payload.needs_clarification
      ? (payload.clarification_prompt || payload.reply || '请继续补充需求。')
      : ''
    const nextPlan = payload.plan ? normalizeAgentPlanForClient(payload.plan) : null
    const nextCandidates = normalizeAgentCandidates(payload.candidates, nextPlan)
    agentPlan.value = nextPlan
    agentCandidates.value = nextCandidates
    agentSelectedCandidateId.value = nextCandidates[0]?.id || nextPlan?.id || ''
    agentSafetyNotes.value = Array.isArray(payload.safety_notes) ? payload.safety_notes : []
    agentStep.value = agentClarificationPrompt.value ? 'describe' : (nextPlan?.prompt ? 'plan' : 'describe')
    clearAgentFailure('plan')
    if (nextPlan?.prompt && !agentClarificationPrompt.value) {
      markAgentEstimateDirty()
    }
    if (payload.reply) {
      agentMessages.value = [
        ...agentMessages.value,
        { role: 'assistant', content: payload.reply }
      ]
    }
  } catch (error) {
    agentModeError.value = ''
    agentFailure.value = classifyAgentFailure('plan', error, requestPayload)
  } finally {
    agentPlanning.value = false
  }
}

async function retryAgentPlan() {
  const failure = agentFailure.value
  if (failure?.phase !== 'plan' || !failure.retryPayload || agentPlanning.value) return
  if (!isLoggedIn.value) {
    requireLogin()
    return
  }
  agentModeError.value = ''
  creditEstimateError.value = ''
  creditEstimateNotice.value = ''
  agentPlanning.value = true
  const requestPayload = cloneAgentRetryPayload(failure.retryPayload)
  try {
    const payload = await api.planImageAgent(requestPayload)
    agentClarificationPrompt.value = payload.needs_clarification
      ? (payload.clarification_prompt || payload.reply || '请继续补充需求。')
      : ''
    const nextPlan = payload.plan ? normalizeAgentPlanForClient(payload.plan) : null
    const nextCandidates = normalizeAgentCandidates(payload.candidates, nextPlan)
    agentPlan.value = nextPlan
    agentCandidates.value = nextCandidates
    agentSelectedCandidateId.value = nextCandidates[0]?.id || nextPlan?.id || ''
    agentSafetyNotes.value = Array.isArray(payload.safety_notes) ? payload.safety_notes : []
    agentStep.value = agentClarificationPrompt.value ? 'describe' : (nextPlan?.prompt ? 'plan' : 'describe')
    clearAgentFailure('plan')
    if (nextPlan?.prompt && !agentClarificationPrompt.value) {
      markAgentEstimateDirty()
    }
    if (payload.reply) {
      agentMessages.value = [
        ...agentMessages.value,
        { role: 'assistant', content: payload.reply }
      ]
    }
  } catch (error) {
    agentFailure.value = classifyAgentFailure('plan', error, requestPayload)
  } finally {
    agentPlanning.value = false
  }
}

function mergeAgentPlanEdit(nextPlan = {}, fallback = {}) {
  const merged = { ...fallback, ...nextPlan }
  return {
    id: String(merged.id ?? ''),
    title: typeof merged.title === 'string' ? merged.title : (merged.title ?? ''),
    intent: merged.intent || 'text_to_image',
    tool_mode: merged.tool_mode || 'generate',
    prompt: typeof merged.prompt === 'string' ? merged.prompt : '',
    negative_prompt: typeof merged.negative_prompt === 'string' ? merged.negative_prompt : '',
    aspect_ratio: merged.aspect_ratio || '1:1',
    style_preset: typeof merged.style_preset === 'string' ? merged.style_preset : '',
    quality: merged.quality || 'medium',
    reference_weight: Number(merged.reference_weight ?? 75),
    tool_options: merged.tool_options && typeof merged.tool_options === 'object' ? merged.tool_options : {},
    edit_instruction: typeof merged.edit_instruction === 'string' ? merged.edit_instruction : '',
    requires_confirmation: true
  }
}

function updateAgentPlan(nextPlan) {
  agentPlan.value = mergeAgentPlanEdit(nextPlan, agentPlan.value || {})
  agentClarificationPrompt.value = ''
  agentStep.value = 'plan'
  clearAgentFailure('generate')
  markAgentEstimateDirty()
}

function selectAgentCandidate(candidateId) {
  const candidate = agentCandidates.value.find((item) => (item.id || item.title) === candidateId)
  if (!candidate) return
  agentSelectedCandidateId.value = candidate.id || candidate.title
  updateAgentPlan(candidate)
}

function applyAgentPlanToComposer(plan) {
  const nextPlan = normalizeAgentPlanForClient(plan)
  syncingAgentPlanToComposer = true
  try {
    prompt.value = nextPlan.prompt
    negativePrompt.value = nextPlan.negative_prompt || ''
    aspectRatio.value = nextPlan.aspect_ratio || '1:1'
    stylePreset.value = nextPlan.style_preset || ''
    toolMode.value = nextPlan.tool_mode || 'generate'
    quality.value = nextPlan.quality || 'medium'
    referenceWeight.value = Number(nextPlan.reference_weight ?? 75)
    toolOptions.value = nextPlan.tool_options && typeof nextPlan.tool_options === 'object' ? nextPlan.tool_options : {}
    editInstruction.value = nextPlan.edit_instruction || ''
    enforceSourceLimit()
  } finally {
    void nextTick(() => {
      syncingAgentPlanToComposer = false
    })
  }
  return nextPlan
}

function buildAgentPayloadFromPlan(plan = agentPlan.value) {
  if (!plan?.prompt) return null
  const nextPlan = normalizeAgentPlanForClient(plan)
  const requestPayload = {
    prompt: nextPlan.prompt,
    negative_prompt: nextPlan.negative_prompt || undefined,
    aspect_ratio: nextPlan.aspect_ratio || '1:1',
    model_id: selectedModelId.value || undefined,
    tool_mode: nextPlan.tool_mode || 'generate'
  }
  const cleanEditInstruction = nextPlan.tool_mode === 'precision_edit'
    ? nextPlan.prompt
    : nextPlan.edit_instruction
  if (cleanEditInstruction) {
    requestPayload.edit_instruction = cleanEditInstruction
  }
  if (nextPlan.quality && nextPlan.quality !== 'medium') {
    requestPayload.quality = nextPlan.quality
  }
  if (nextPlan.tool_options && Object.keys(nextPlan.tool_options).length > 0) {
    requestPayload.tool_options = { ...nextPlan.tool_options }
  }
  if (selectedSourceWorkId.value) {
    requestPayload.source_work_id = selectedSourceWorkId.value
  }
  if (selectedReferenceCount.value > 0) {
    requestPayload.reference_weight = Math.max(0, Math.min(100, Number(nextPlan.reference_weight ?? 75) || 75))
  }
  if (selectedReferenceIds.value.length > 0) {
    requestPayload.reference_asset_ids = [...selectedReferenceIds.value]
  }
  if (selectedReferenceWorkIds.value.length > 0) {
    requestPayload.reference_work_ids = [...selectedReferenceWorkIds.value]
  }
  if (selectedReferenceCount.value >= 2) {
    requestPayload.reference_intent = 'compose'
  }
  if (nextPlan.style_preset) {
    requestPayload.style_preset = nextPlan.style_preset
  }
  return requestPayload
}

function buildAgentGenerationPayload(options = {}) {
  if (!agentPlan.value?.prompt) return null
  if (options.syncComposer === false) {
    return buildAgentPayloadFromPlan(agentPlan.value)
  }
  const nextPlan = applyAgentPlanToComposer(agentPlan.value)
  return buildGenerationPayload(nextPlan.prompt)
}

async function estimateAgentPlan() {
  if (!isLoggedIn.value) {
    requireLogin()
    return
  }
  if (agentClarificationPrompt.value) return
  const requestPayload = buildAgentGenerationPayload({ syncComposer: false })
  if (!requestPayload || typeof api.estimateImageGeneration !== 'function') return
  if (editToolModes.has(requestPayload.tool_mode) && !hasEditSourceImage.value) {
    creditEstimate.value = null
    creditEstimateError.value = ''
    creditEstimateNotice.value = ''
    agentEstimateDirty.value = true
    return
  }
  agentModeError.value = ''
  creditEstimateError.value = ''
  creditEstimateNotice.value = ''
  creditEstimateLoading.value = true
  try {
    const payload = await api.estimateImageGeneration(requestPayload)
    creditEstimate.value = payload
    agentLastEstimatedPayload.value = { ...requestPayload }
    agentEstimateDirty.value = false
    syncAvailableCredits(payload)
    if (payload?.enough === false) {
      creditEstimateError.value = `点数不足，还差 ${payload.missing_credits ?? 0} 点`
    }
  } catch (error) {
    creditEstimate.value = null
    agentLastEstimatedPayload.value = null
    agentEstimateDirty.value = false
    creditEstimateCanCreateWithoutEstimate.value = false
    creditEstimateError.value = estimateErrorMessage(error)
    await refreshReferencesAfterEstimateError(error)
  } finally {
    creditEstimateLoading.value = false
  }
}

async function confirmAgentGeneration() {
  if (!isLoggedIn.value) {
    requireLogin()
    return
  }
  const requestPayload = agentLastEstimatedPayload.value ? { ...agentLastEstimatedPayload.value } : buildAgentGenerationPayload({ syncComposer: false })
  if (!requestPayload?.prompt || submitting.value) return
  agentModeError.value = ''
  clearAgentFailure('generate')
  taskError.value = ''
  resultError.value = ''
  if (!agentCanConfirmGenerate.value) {
    agentModeError.value = agentConfirmDisabledReason.value || '请先完成点数预估后再生成。'
    return
  }
  if (editToolModes.has(requestPayload.tool_mode) && !hasEditSourceImage.value) {
    agentModeError.value = '请先上传图片或选择作品作为编辑来源'
    return
  }

  submitting.value = true
  try {
    applyAgentPlanToComposer(agentPlan.value)
    const payload = await appendEraseMaskPayload(requestPayload)
    const nextTask = await createGenerationTask(payload, payload.prompt)
    agentExecutionTaskId.value = nextTask.generation_id
    agentStep.value = 'result'
    clearAgentFailure('generate')
    agentMessages.value = [
      ...agentMessages.value,
      { role: 'assistant', content: '任务已提交，我会在右侧跟踪生成进度。' }
    ]
  } catch (error) {
    agentModeError.value = ''
    agentFailure.value = classifyAgentFailure('generate', error, requestPayload)
  } finally {
    submitting.value = false
  }
}

async function retryAgentGeneration() {
  const failure = agentFailure.value
  if (failure?.phase !== 'generate' || !failure.retryPayload || submitting.value) return
  if (!isLoggedIn.value) {
    requireLogin()
    return
  }
  agentModeError.value = ''
  taskError.value = ''
  resultError.value = ''
  submitting.value = true
  const requestPayload = cloneAgentRetryPayload(failure.retryPayload)
  try {
    const nextTask = await createGenerationTask(requestPayload, requestPayload.prompt)
    agentExecutionTaskId.value = nextTask.generation_id
    agentStep.value = 'result'
    clearAgentFailure('generate')
    agentMessages.value = [
      ...agentMessages.value,
      { role: 'assistant', content: '任务已提交，我会在右侧跟踪生成进度。' }
    ]
  } catch (error) {
    agentFailure.value = classifyAgentFailure('generate', error, requestPayload)
  } finally {
    submitting.value = false
  }
}

function applyQuickPromptChip(chip) {
  if (!chip || submitting.value) return
  applyDiscoveryTemplate(chip)
}

function openWorkshopFeature(entry) {
  if (!entry || submitting.value) return
  if (entry.disabled) {
    successMessage.value = entry.message || '该功能即将开放。'
    return
  }
  if (entry.action === 'commerce') {
    router.push('/workspace/ai-commerce')
    return
  }
  if (entry.route) {
    router.push(entry.route)
    return
  }
  if (entry.mode) {
    const tool = aiToolCards.value.find((item) => item.mode === entry.mode) || { mode: entry.mode }
    selectToolCard(tool)
    return
  }
  successMessage.value = entry.message || '该功能即将开放。'
}

function selectToolCard(tool) {
  if (submitting.value) return
  if (tool?.route) {
    if (!isLoggedIn.value) {
      requireLogin()
      return
    }
    router.push(tool.route)
    return
  }
  toolMode.value = tool?.mode || 'generate'
  toolOptions.value = defaultToolOptions(tool)
  enforceSourceLimit()
  workspaceTab.value = 'create'
  referenceError.value = ''
  focusWorkshopPrompt()
}

function applyDiscoveryTemplate(template) {
  if (!template || submitting.value) return
  prompt.value = (template.prompt || '').trim()
  aspectRatio.value = template.aspect_ratio || '1:1'
  stylePreset.value = template.style_preset || ''
  toolMode.value = template.tool_mode || 'generate'
  if (template.model_id) {
    selectedModelId.value = Number(template.model_id)
  }
  toolOptions.value = defaultToolOptions(selectedTool.value)
  enforceSourceLimit()
  workspaceTab.value = 'create'
  referenceError.value = ''
  focusWorkshopPrompt()
}

function openRecommendationPreview(recommendation) {
  if (!recommendation) return
  recommendationPreview.value = recommendation
}

function closeRecommendationPreview() {
  recommendationPreview.value = null
}

function applyInspirationRecommendation(recommendation) {
  if (!recommendation || submitting.value) return
  prompt.value = (recommendation.prompt || '').trim()
  negativePrompt.value = recommendation.negative_prompt || ''
  aspectRatio.value = recommendation.aspect_ratio || '1:1'
  stylePreset.value = recommendation.style_preset || ''
  toolMode.value = recommendation.tool_mode || 'generate'
  if (recommendation.model_id) {
    selectedModelId.value = Number(recommendation.model_id)
  }
  toolOptions.value = {
    ...defaultToolOptions(selectedTool.value),
    ...(recommendation.params && typeof recommendation.params === 'object' ? recommendation.params : {})
  }
  enforceSourceLimit()
  workspaceTab.value = 'create'
  referenceError.value = ''
  successMessage.value = '已套用同款参数'
  closeRecommendationPreview()
  if (typeof api.useInspirationRecommendation === 'function' && recommendation.id) {
    void api.useInspirationRecommendation(recommendation.id).catch((error) => {
      console.warn('Failed to record inspiration recommendation use:', error)
    })
  }
  focusWorkshopPrompt()
}

function defaultToolOptions(tool = selectedTool.value) {
  const next = {}
  const fields = Array.isArray(tool?.form_schema) ? tool.form_schema : []
  fields.forEach((field) => {
    if (!field?.key || ['mask', 'edit_instruction'].includes(field.key)) return
    if (field.default !== undefined) {
      next[field.key] = field.default
      return
    }
    if (field.type === 'select' && Array.isArray(field.options) && field.options.length > 0) {
      next[field.key] = field.options[0]
      return
    }
    if (field.type === 'number') {
      next[field.key] = Number(field.min ?? 0)
    }
  })
  return next
}

function setToolOption(key, value) {
  toolOptions.value = {
    ...toolOptions.value,
    [key]: value
  }
}

function boundedToolOption(value, min = 0, max = 100, fallback = 0) {
  const next = Number(value)
  if (!Number.isFinite(next)) return fallback
  return Math.min(max, Math.max(min, Math.round(next)))
}

function applyExpandPreset(values) {
  if (submitting.value) return
  toolOptions.value = {
    ...toolOptions.value,
    ...values
  }
}

function enforceSourceLimit() {
  const limit = sourceImageLimit.value
  if (limit > 0 && selectedReferenceCount.value > limit) {
    const remainingUploadedIds = selectedReferenceIds.value.slice(0, limit)
    const remainingWorkSlots = Math.max(0, limit - remainingUploadedIds.length)
    const remainingWorkIds = selectedReferenceWorkIds.value.slice(0, remainingWorkSlots)
    selectedReferenceIds.value = remainingUploadedIds
    selectedReferenceWorkIds.value = remainingWorkIds
    const remainingWorkIdSet = new Set(remainingWorkIds)
    selectedReferenceWorkSnapshots.value = Object.fromEntries(
      Object.entries(selectedReferenceWorkSnapshots.value).filter(([id]) => remainingWorkIdSet.has(Number(id)))
    )
  }
  if (limit === 1 && selectedSourceWorkId.value && selectedReferenceCount.value > 0) {
    selectedSourceWorkId.value = null
  }
}

function normalizedToolOptionsForPayload() {
  const fields = selectedToolFields.value.filter((field) => field.key && !['mask', 'edit_instruction'].includes(field.key))
  const payload = {}
  Object.entries(toolOptions.value || {}).forEach(([key, value]) => {
    if (!key || ['mask', 'edit_instruction'].includes(key) || value === undefined || value === '') return
    payload[key] = value
  })
  if (isExpandTool.value) {
    payload.unit = 'percent'
  }
  fields.forEach((field) => {
    let value = toolOptions.value[field.key]
    if (field.type === 'number') {
      value = Number(value)
      if (!Number.isFinite(value)) value = Number(field.default ?? field.min ?? 0)
      value = boundedToolOption(value, Number(field.min ?? 0), Number(field.max ?? 100), Number(field.default ?? field.min ?? 0))
    }
    if (value !== undefined && value !== '') {
      payload[field.key] = value
    }
  })
  return Object.keys(payload).length > 0 ? payload : undefined
}

function buildGenerationPayload(promptText) {
  const requestPayload = {
    prompt: promptText,
    negative_prompt: negativePrompt.value.trim() || undefined,
    aspect_ratio: aspectRatio.value,
    model_id: selectedModelId.value || undefined,
    tool_mode: toolMode.value
  }
  const cleanEditInstruction = isPrecisionEditTool.value
    ? prompt.value.trim()
    : (editInstruction.value.trim() || (isEraseTool.value ? prompt.value.trim() : ''))
  if (cleanEditInstruction) {
    requestPayload.edit_instruction = cleanEditInstruction
  }
  if (quality.value && quality.value !== 'medium') {
    requestPayload.quality = quality.value
  }
  const toolOptionsPayload = normalizedToolOptionsForPayload()
  if (toolOptionsPayload) {
    requestPayload.tool_options = toolOptionsPayload
  }
  if (isMaskSelectionTool.value && eraseMaskRegions.value.length > 0) {
    requestPayload.tool_options = {
      ...(requestPayload.tool_options || {}),
      mask_regions: eraseMaskRegions.value
    }
  }
  if (selectedSourceWorkId.value) {
    requestPayload.source_work_id = selectedSourceWorkId.value
  }
  if (hasGenerationReferences.value) {
    const weight = boundedToolOption(referenceWeight.value, 0, 100, 75)
    referenceWeight.value = weight
    requestPayload.reference_weight = weight
  }
  if (selectedReferenceIds.value.length > 0) {
    requestPayload.reference_asset_ids = [...selectedReferenceIds.value]
  }
  if (selectedReferenceWorkIds.value.length > 0) {
    requestPayload.reference_work_ids = [...selectedReferenceWorkIds.value]
  }
  if (selectedReferenceCount.value >= 2) {
    requestPayload.reference_intent = 'compose'
  }
  if (stylePreset.value) {
    requestPayload.style_preset = stylePreset.value
  }
  return requestPayload
}

async function appendEraseMaskPayload(requestPayload) {
  if (!isMaskSelectionTool.value || !hasEraseMask.value) return requestPayload
  const maskFile = await createEraseMaskFile()
  const uploadedMask = await api.uploadReferenceAsset(maskFile)
  return {
    ...requestPayload,
    mask_asset_id: uploadedMask.id,
    tool_options: {
      ...(requestPayload.tool_options || {}),
      mask_regions: eraseMaskRegions.value
    }
  }
}

function clearCreditEstimateTimer() {
  if (creditEstimateTimer !== null) {
    window.clearTimeout(creditEstimateTimer)
    creditEstimateTimer = null
  }
}

function abortCreditEstimateRequest() {
  if (creditEstimateAbortController) {
    creditEstimateAbortController.abort()
    creditEstimateAbortController = null
  }
}

function cancelScheduledCreditEstimate() {
  clearCreditEstimateTimer()
  abortCreditEstimateRequest()
  creditEstimateRequestSeq += 1
}

function clearCurrentCreditEstimateFeedback() {
  creditEstimateNotice.value = ''
  creditEstimateError.value = ''
  creditEstimateCanCreateWithoutEstimate.value = false
}

function scheduleCreditEstimate() {
  cancelScheduledCreditEstimate()
  clearCurrentCreditEstimateFeedback()
  creditEstimateLoading.value = false

  const cleanPrompt = effectivePrompt.value
  if (!isLoggedIn.value || !cleanPrompt || (isEditTool.value && !hasEditSourceImage.value) || (isPrecisionEditTool.value && !hasEraseMask.value) || typeof api.estimateImageGeneration !== 'function') {
    creditEstimate.value = null
    return
  }

  if (creditEstimate.value?.enough === false) {
    creditEstimate.value = null
  }

  creditEstimateTimer = window.setTimeout(() => {
    creditEstimateTimer = null
    void updateCreditEstimate()
  }, creditEstimateDebounceMs)
}

async function updateCreditEstimate() {
  const cleanPrompt = effectivePrompt.value
  clearCurrentCreditEstimateFeedback()
  if (!isLoggedIn.value || !cleanPrompt || (isEditTool.value && !hasEditSourceImage.value) || (isPrecisionEditTool.value && !hasEraseMask.value) || typeof api.estimateImageGeneration !== 'function') {
    creditEstimate.value = null
    return
  }
  const requestPayload = buildGenerationPayload(cleanPrompt)
  const requestSeq = ++creditEstimateRequestSeq
  const abortController = typeof AbortController !== 'undefined' ? new AbortController() : null
  creditEstimateAbortController = abortController
  creditEstimateLoading.value = true
  try {
    const payload = await api.estimateImageGeneration(
      requestPayload,
      abortController ? { signal: abortController.signal } : {}
    )
    if (requestSeq !== creditEstimateRequestSeq) return
    creditEstimate.value = payload
    syncAvailableCredits(payload)
    if (payload?.enough === false) {
      creditEstimateError.value = `点数不足，还差 ${payload.missing_credits ?? 0} 点`
    }
  } catch (error) {
    if (requestSeq !== creditEstimateRequestSeq || error?.name === 'AbortError') return
    creditEstimate.value = null
    if (isSoftCreditEstimateError(error)) {
      creditEstimateCanCreateWithoutEstimate.value = true
      creditEstimateNotice.value = creditEstimateUnavailableNotice
      return
    }
    creditEstimateCanCreateWithoutEstimate.value = false
    creditEstimateError.value = estimateErrorMessage(error)
    await refreshReferencesAfterEstimateError(error)
  } finally {
    if (requestSeq === creditEstimateRequestSeq) {
      creditEstimateLoading.value = false
      if (creditEstimateAbortController === abortController) {
        creditEstimateAbortController = null
      }
    }
  }
}

function selectHistoryWork(work) {
  selectedTaskId.value = null
  result.value = work
  resultError.value = ''
  regenerateResultError.value = ''
  if (isEditTool.value && work?.work_id) {
    selectedSourceWorkId.value = work.work_id
    selectedReferenceIds.value = []
    selectedReferenceWorkIds.value = []
    selectedReferenceWorkSnapshots.value = {}
    referenceError.value = ''
  }
}

function selectGenerationTask(taskItem) {
  if (!taskItem?.generation_id) return
  selectedTaskId.value = Number(taskItem.generation_id)
  result.value = null
  regenerateResultError.value = ''
  if (taskItem.status === 'failed') {
    const message = failureMessage(taskItem)
    taskError.value = message
    resultError.value = 'failed'
  } else {
    taskError.value = ''
    resultError.value = ''
  }
  workspaceTab.value = 'create'
}

function handleUseWorkAsReference(work) {
  if (referenceUploading.value || submitting.value || !canUseWorkAsReference(work)) return
  const id = workID(work)
  if (selectedReferenceWorkIds.value.includes(id)) {
    return
  }
  const maxImages = sourceImageLimit.value
  if (selectedReferenceCount.value >= maxImages) {
    if (maxImages === 1) {
      selectedReferenceIds.value = []
      selectedReferenceWorkIds.value = []
      selectedReferenceWorkSnapshots.value = {}
      selectedSourceWorkId.value = null
    } else {
      referenceError.value = `最多只能选择 ${maxImages} 张参考图`
      return
    }
  }
  referenceError.value = ''
  selectedSourceWorkId.value = null
  selectedReferenceWorkSnapshots.value = {
    ...selectedReferenceWorkSnapshots.value,
    [id]: {
      ...work,
      work_id: id,
      category: workCategory(work)
    }
  }
  selectedReferenceWorkIds.value = [...selectedReferenceWorkIds.value, id]
  workspaceTab.value = 'create'
  if (workspaceMode.value === 'agent' && agentPlan.value?.prompt) {
    markAgentEstimateDirty()
  }
}

function openPlaygroundItem(item) {
  if (!item?.route) return
  if (!isLoggedIn.value) {
    requireLogin()
    return
  }
  router.push(item.route)
}

async function handleReferenceRemove(image) {
  if (referenceUploading.value || submitting.value) return
  referenceError.value = ''
  if (image?.reference_kind === 'work') {
    selectedReferenceWorkIds.value = selectedReferenceWorkIds.value.filter((id) => id !== image.work_id)
    const snapshots = { ...selectedReferenceWorkSnapshots.value }
    delete snapshots[image.work_id]
    selectedReferenceWorkSnapshots.value = snapshots
    if (workspaceMode.value === 'agent' && agentPlan.value?.prompt) {
      markAgentEstimateDirty()
    }
    return
  }
  selectedReferenceIds.value = selectedReferenceIds.value.filter((id) => id !== image.id)
  try {
    await api.deleteReferenceAsset(image.id)
    referenceAssets.value = referenceAssets.value.filter((item) => item.id !== image.id)
  } catch (error) {
    referenceError.value = error.message || '参考图移除失败'
  }
  if (workspaceMode.value === 'agent' && agentPlan.value?.prompt) {
    markAgentEstimateDirty()
  }
}

function clearAgentReferences() {
  if (referenceUploading.value || submitting.value || selectedReferenceCount.value === 0) return
  selectedReferenceIds.value = []
  selectedReferenceWorkIds.value = []
  selectedReferenceWorkSnapshots.value = {}
  selectedSourceWorkId.value = null
  referenceError.value = ''
  if (agentPlan.value?.prompt) {
    markAgentEstimateDirty()
  }
}

function stopPolling() {
  if (pollTimer !== null) {
    window.clearInterval(pollTimer)
    pollTimer = null
  }
}

function isTechnicalFailureMessage(message = '') {
  const text = String(message || '').toLowerCase()
  return (
    /https?:\/\//i.test(message)
    || /\b(post|get|put|patch|delete)\s+"/i.test(message)
    || text.includes('context deadline exceeded')
    || text.includes('client.timeout')
    || text.includes('connection refused')
    || text.includes('connection reset')
    || text.includes('traceid')
    || text.includes('trace id')
  )
}

function isTimeoutFailureMessage(message = '') {
  const text = String(message || '').toLowerCase()
  return (
    /响应超时|网络超时|模型超时/.test(message)
    || text.includes('context deadline exceeded')
    || text.includes('client.timeout')
    || text.includes('timeout')
  )
}

function generationErrorCode(payload) {
  return String(payload?.error?.code || payload?.error_code || payload?.code || '').trim()
}

function isUserCancelledGeneration(payload) {
  return generationErrorCode(payload) === 'user_cancelled'
}

function isTaskCancelling(taskItem) {
  if (!taskItem?.generation_id) return false
  return cancellingTaskIds.value.includes(Number(taskItem.generation_id))
}

function setTaskCancelling(taskID, cancelling) {
  const id = Number(taskID)
  if (!id) return
  const next = new Set(cancellingTaskIds.value)
  if (cancelling) {
    next.add(id)
  } else {
    next.delete(id)
  }
  cancellingTaskIds.value = Array.from(next)
}

function failureBaseMessage(payload, fallback = '图片生成失败，请稍后再试') {
  if (isUserCancelledGeneration(payload)) {
    return '已取消生成，未扣点，可修改提示词后重新生成。'
  }
  const code = String(payload?.error?.code || payload?.code || '').trim()
  const rawMessage = String(payload?.error?.message || payload?.message || '').trim()
  if (code === 'provider_timeout' || isTimeoutFailureMessage(rawMessage)) {
    return '网络超时，生成失败'
  }
  if (isTechnicalFailureMessage(rawMessage)) {
    return fallback
  }
  if (code === 'provider_policy_rejected') {
    return '提示词可能触发平台安全策略，请调整后重试。'
  }
  if (code === 'provider_rate_limited') {
    return '图片服务当前繁忙，请稍后重试。'
  }
  if (code === 'provider_unavailable' || code === 'provider_request_failed') {
    return '图片服务暂时不可用，请稍后重试。'
  }
  if (code === 'provider_asset_fetch_failed') {
    return '模型已返回图片，但平台保存失败，请稍后重试。'
  }
  if (code === 'provider_empty_image') {
    return '图片服务未返回可用结果，请稍后重试。'
  }
  if (rawMessage && !isTechnicalFailureMessage(rawMessage) && /请|点数|余额|提示词|安全|策略|稍后|重试|超时/.test(rawMessage)) {
    return rawMessage
  }
  return fallback
}

function failureMessage(payload, fallback = '图片生成失败，请稍后再试') {
  if (isUserCancelledGeneration(payload)) {
    return '已取消生成，未扣点，可修改提示词后重新生成。'
  }
  const baseMessage = failureBaseMessage(payload, fallback)
  const hints = []
  if (payload?.credits_deducted === true) {
    hints.push(`已扣 ${Number(payload.credits_cost || 1)} 点`)
  } else if (payload?.credits_deducted === false) {
    hints.push('未扣点')
  }
  if (payload?.error?.retryable === true) {
    hints.push('点击重试')
  } else if (payload?.error?.retryable === false) {
    hints.push('不可重试')
  }
  return hints.length > 0 ? `${baseMessage}（${hints.join('，')}）` : baseMessage
}

function activeGenerationSnapshot(payload, promptText = '', parameters = null) {
  if (!payload?.generation_id) return null
  return {
    generation_id: payload.generation_id,
    status: payload.status || 'queued',
    stage: payload.stage || payload.status || 'queued',
    created_at: payload.created_at || new Date().toISOString(),
    prompt: payload.prompt || promptText || '',
    parameters: payload.parameters || parameters || null,
    available_credits: payload.available_credits,
    credits_cost: payload.credits_cost,
    credits_deducted: payload.credits_deducted
  }
}

function isActiveGenerationStatus(status) {
  return activeGenerationStatuses.has(String(status || '').toLowerCase())
}

function upsertGenerationTask(nextTask) {
  if (!nextTask?.generation_id) return
  const id = Number(nextTask.generation_id)
  const existing = activeTasks.value.find((item) => Number(item.generation_id) === id)
  const nextStatus = nextTask.status || existing?.status || 'queued'
  const merged = {
    ...(existing || {}),
    ...nextTask,
    generation_id: id,
    prompt: nextTask.prompt || existing?.prompt || '',
    parameters: nextTask.parameters || existing?.parameters || null,
    stage: nextTask.stage || nextTask.status || existing?.stage || 'queued',
    status: nextStatus,
    created_at: nextTask.created_at || existing?.created_at || new Date().toISOString(),
    poll_failure_count: Number(nextTask.poll_failure_count ?? existing?.poll_failure_count ?? 0)
  }
  if (String(nextStatus).toLowerCase() === 'failed') {
    merged.failure_message = nextTask.failure_message || failureMessage(merged)
  } else {
    delete merged.failure_message
    delete merged.error
    delete merged.error_code
  }
  activeTasks.value = existing
    ? activeTasks.value.map((item) => Number(item.generation_id) === id ? merged : item)
    : [merged, ...activeTasks.value]
}

function activeGenerationSnapshots() {
  return activeTasks.value
    .filter((item) => isActiveGenerationStatus(item.status))
    .map((item) => activeGenerationSnapshot(item, item.prompt, item.parameters))
    .filter(Boolean)
}

function persistActiveGenerations() {
  const snapshots = activeGenerationSnapshots()
  try {
    if (snapshots.length === 0) {
      window.localStorage?.removeItem(activeGenerationStorageKey)
      window.sessionStorage?.removeItem(activeGenerationStorageKey)
      return
    }
    window.localStorage?.setItem(activeGenerationStorageKey, JSON.stringify(snapshots))
    window.sessionStorage?.setItem(activeGenerationStorageKey, JSON.stringify(snapshots.length === 1 ? snapshots[0] : snapshots))
  } catch (error) {
    console.warn('Failed to persist active generations:', error)
  }
}

function clearActiveGeneration() {
  try {
    window.localStorage?.removeItem(activeGenerationStorageKey)
    window.sessionStorage?.removeItem(activeGenerationStorageKey)
  } catch (error) {
    console.warn('Failed to clear active generation:', error)
  }
}

function readActiveGenerations() {
  try {
    const raw = window.localStorage?.getItem(activeGenerationStorageKey) || window.sessionStorage?.getItem(activeGenerationStorageKey)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    const snapshots = Array.isArray(parsed) ? parsed : [parsed]
    const validSnapshots = snapshots.filter((snapshot) => snapshot?.generation_id)
    if (validSnapshots.length === 0) {
      clearActiveGeneration()
    }
    return validSnapshots
  } catch (error) {
    clearActiveGeneration()
    return []
  }
}

async function pollOneTask(currentTask) {
  try {
    const payload = await api.getImageGeneration(currentTask.generation_id)
    const nextTask = {
      ...currentTask,
      ...payload,
      prompt: payload.prompt || currentTask.prompt,
      parameters: payload.parameters || currentTask.parameters,
      poll_failure_count: 0
    }
    upsertGenerationTask(nextTask)
    taskError.value = ''
    resultError.value = ''
    syncAvailableCredits(payload)

    if (payload.status === 'succeeded') {
      result.value = payload
      resultError.value = ''
      successMessage.value = '最新作品已写入作品库。'
      if (payload.available_credits === undefined) {
        void refreshSessionCredits()
      }
      await loadWorks()
    } else if (payload.status === 'failed') {
      const message = failureMessage(payload)
      if (Number(selectedTaskId.value) === Number(currentTask.generation_id)) {
        taskError.value = message
        resultError.value = 'failed'
      }
      if (payload.available_credits === undefined) {
        void refreshSessionCredits()
      }
    }
  } catch (error) {
    upsertGenerationTask({
      ...currentTask,
      poll_failure_count: Number(currentTask.poll_failure_count || 0) + 1
    })
  }
}

async function pollTasks() {
  const pendingTasks = activeTasks.value.filter((item) => isActiveGenerationStatus(item.status))
  if (pendingTasks.length === 0) {
    persistActiveGenerations()
    stopPolling()
    return
  }
  await Promise.all(pendingTasks.map((item) => pollOneTask(item)))
  persistActiveGenerations()
  if (!activeTasks.value.some((item) => isActiveGenerationStatus(item.status))) {
    stopPolling()
  }
}

async function cancelGeneration(taskToCancel = task.value) {
  const currentTask = taskToCancel?.generation_id ? taskToCancel : task.value
  if (!currentTask?.generation_id || !isActiveGenerationStatus(currentTask.status) || isTaskCancelling(currentTask)) {
    return
  }
  const taskID = Number(currentTask.generation_id)
  setTaskCancelling(taskID, true)
  taskError.value = ''
  resultError.value = ''
  try {
    const payload = await api.cancelImageGeneration(taskID)
    const nextTask = {
      ...currentTask,
      ...payload,
      prompt: payload.prompt || currentTask.prompt,
      parameters: payload.parameters || currentTask.parameters,
      poll_failure_count: 0
    }
    upsertGenerationTask(nextTask)
    syncAvailableCredits(payload)

    if (payload.status === 'succeeded') {
      if (Number(selectedTaskId.value) === taskID) {
        result.value = payload
        taskError.value = ''
        resultError.value = ''
      }
      if (payload.available_credits === undefined) {
        void refreshSessionCredits()
      }
      await loadWorks()
    } else if (payload.status === 'failed') {
      const message = failureMessage(nextTask)
      if (Number(selectedTaskId.value) === taskID) {
        taskError.value = message
        resultError.value = 'failed'
        result.value = null
      }
      if (payload.available_credits === undefined) {
        void refreshSessionCredits()
      }
    }

    persistActiveGenerations()
    if (!activeTasks.value.some((item) => isActiveGenerationStatus(item.status))) {
      stopPolling()
    }
  } catch (error) {
    if (Number(selectedTaskId.value) === taskID) {
      taskError.value = error.message || '取消生成失败，请稍后再试'
    }
  } finally {
    setTaskCancelling(taskID, false)
  }
}

function startPolling() {
  if (pollTimer !== null) return
  pollTimer = window.setInterval(() => {
    void pollTasks()
  }, 1000)
}

async function restoreActiveGeneration() {
  const snapshots = readActiveGenerations()
  if (snapshots.length === 0) return

  workspaceTab.value = 'create'
  taskError.value = ''
  resultError.value = ''
  successMessage.value = ''
  snapshots.forEach((snapshot) => {
    upsertGenerationTask(snapshot)
    syncAvailableCredits(snapshot)
  })
  selectedTaskId.value = snapshots[0].generation_id

  await Promise.all(snapshots.map(async (snapshot) => {
    try {
      const payload = await api.getImageGeneration(snapshot.generation_id)
      const restoredTask = {
        ...snapshot,
        ...payload,
        prompt: payload.prompt || snapshot.prompt,
        parameters: payload.parameters || snapshot.parameters,
        poll_failure_count: 0
      }
      upsertGenerationTask(restoredTask)
      syncAvailableCredits(payload)

      if (payload.status === 'succeeded') {
        result.value = payload
        if (payload.available_credits === undefined) {
          void refreshSessionCredits()
        }
        await loadWorks()
        successMessage.value = '最新作品已写入作品库。'
      } else if (payload.status === 'failed') {
        const message = failureMessage(payload)
        if (Number(selectedTaskId.value) === Number(snapshot.generation_id)) {
          taskError.value = message
          resultError.value = 'failed'
        }
        if (payload.available_credits === undefined) {
          void refreshSessionCredits()
        }
      }
    } catch (error) {
      upsertGenerationTask({
        ...snapshot,
        poll_failure_count: 1
      })
    }
  }))
  persistActiveGenerations()
  if (activeTasks.value.some((item) => isActiveGenerationStatus(item.status))) {
    startPolling()
  }
}

function selectStylePreset(style) {
  if (submitting.value) return
  stylePreset.value = stylePreset.value === style ? '' : style
}

async function createGenerationTask(requestPayload, submittedPrompt) {
  const payload = await api.createImageGeneration(requestPayload)
  const nextTask = {
    ...payload,
    prompt: payload.prompt || submittedPrompt,
    parameters: payload.parameters || requestPayload,
    poll_failure_count: 0
  }
  upsertGenerationTask(nextTask)
  selectedTaskId.value = nextTask.generation_id
  persistActiveGenerations()
  workspaceTab.value = 'create'
  syncAvailableCredits(payload)
  result.value = null
  regenerateResultError.value = ''
  startPolling()
  return nextTask
}

async function submit(options = {}) {
  if (!isLoggedIn.value) {
    requireLogin()
    return
  }
  const submittedPrompt = (options.promptOverride ?? effectivePrompt.value).trim()
  if (!submittedPrompt) return

  taskError.value = ''
  resultError.value = ''
  successMessage.value = ''
  regenerateResultError.value = ''
  referenceError.value = ''

  if (lowCredit.value) {
    taskError.value = '点数不足，请先前往套餐与充值页面购买点数。'
    return
  }

  if (isEditTool.value && !hasEditSourceImage.value) {
    referenceError.value = '请先上传图片或选择作品作为编辑来源'
    taskError.value = '请先上传图片或选择作品作为编辑来源'
    workspaceTab.value = 'create'
    return
  }

  submitting.value = true
  try {
    const requestPayload = await appendEraseMaskPayload(buildGenerationPayload(submittedPrompt))
    await createGenerationTask(requestPayload, submittedPrompt)
  } catch (error) {
    const message = failureMessage({
      error: {
        code: error.code || '',
        message: error.message || '',
        retryable: true
      }
    })
    taskError.value = message
    resultError.value = 'failed'
  } finally {
    submitting.value = false
  }
}

function openPromptOptimizer() {
  if (!isLoggedIn.value) {
    requireLogin()
    return
  }
  if (submitting.value) return
  promptOptimizerTriggerElement = document.activeElement instanceof HTMLElement ? document.activeElement : null
  resetPromptAssistant()
  promptOptimizerOpen.value = true
  void nextTick(() => {
    focusAssistantInput()
    if (prompt.value.trim()) {
      void requestPromptAssistant({
        mode: 'chat',
        action: 'start',
        message: '',
        appendUserMessage: false
      })
    }
  })
}

function closePromptOptimizer() {
  cancelAssistantRequest()
  promptOptimizerOpen.value = false
  assistantEditingField.value = ''
  void nextTick(() => {
    restorePromptOptimizerFocus()
  })
}

function resetPromptAssistant() {
  cancelAssistantRequest()
  const sourcePrompt = prompt.value.trim()
  assistantInput.value = ''
  assistantError.value = ''
  assistantRunning.value = false
  assistantRunningAction.value = ''
  assistantStructuredPrompt.value = {
    subject: '',
    scene: '',
    style: '',
    usage: ''
  }
  assistantDraftPrompt.value = sourcePrompt
  assistantDirections.value = []
  assistantNotes.value = []
  assistantEditingField.value = ''
  assistantFieldDraft.value = ''
  assistantLastRequest.value = null
  assistantMessages.value = sourcePrompt
    ? [
        { role: 'user', content: sourcePrompt }
      ]
    : [
        {
          role: 'assistant',
          content: '告诉我你想生成什么画面，我会帮你补齐主体、场景、风格和细节。'
        }
      ]
}

function focusAssistantInput() {
  assistantInputRef.value?.focus?.()
}

function restorePromptOptimizerFocus() {
  if (promptOptimizerTriggerElement?.isConnected) {
    promptOptimizerTriggerElement.focus()
  }
  promptOptimizerTriggerElement = null
}

function cancelAssistantRequest() {
  if (assistantAbortController.value) {
    assistantAbortController.value.abort()
    assistantAbortController.value = null
  }
  assistantRunning.value = false
  assistantRunningAction.value = ''
}

function normalizedAssistantStructuredPrompt(value = {}) {
  return {
    subject: (value.subject || '').trim(),
    scene: (value.scene || '').trim(),
    style: (value.style || '').trim(),
    usage: (value.usage || '').trim()
  }
}

function hasAssistantStructuredValue(value) {
  return Object.values(value).some((item) => item.trim())
}

function assistantStructuredFieldValue(key) {
  return (assistantStructuredPrompt.value[key] || '').trim()
}

function composeAssistantDraftPrompt(value = {}) {
  const fields = normalizedAssistantStructuredPrompt(value)
  const parts = [
    fields.subject,
    fields.scene,
    fields.style,
    fields.usage ? `用途：${fields.usage}` : ''
  ].filter(Boolean)
  return parts.join('，')
}

function assistantHistoryPayload(messages = assistantMessages.value) {
  return messages
    .filter((message) => ['user', 'assistant'].includes(message.role) && message.content.trim())
    .map((message) => ({
      role: message.role,
      content: message.content.trim()
    }))
}

function assistantSourcePrompt(message = '') {
  return (prompt.value.trim() || assistantDraftPrompt.value.trim() || message.trim()).trim()
}

function applyPromptAssistantPayload(payload = {}) {
  const nextStructured = normalizedAssistantStructuredPrompt(payload.structured_prompt || {})
  const hasStructuredPayload = Object.prototype.hasOwnProperty.call(payload, 'structured_prompt')
  const structuredHasValue = hasAssistantStructuredValue(nextStructured)
  if (hasStructuredPayload || structuredHasValue) {
    assistantStructuredPrompt.value = nextStructured
  }
  if (payload.optimized_prompt) {
    assistantDraftPrompt.value = payload.optimized_prompt
  } else if (structuredHasValue) {
    assistantDraftPrompt.value = composeAssistantDraftPrompt(nextStructured)
  }
  assistantDirections.value = Array.isArray(payload.directions)
    ? payload.directions
        .filter((direction) => direction && (direction.title || direction.summary || direction.prompt))
        .map((direction) => ({
          title: (direction.title || '').trim(),
          summary: (direction.summary || '').trim(),
          prompt: (direction.prompt || '').trim(),
          structured_prompt: normalizedAssistantStructuredPrompt(direction.structured_prompt || {})
        }))
    : []
  const nextNotes = Array.isArray(payload.safety_notes) ? payload.safety_notes : []
  assistantNotes.value = payload.optimized_prompt && !structuredHasValue
    ? [...nextNotes, '结构化信息可继续补充']
    : nextNotes
  if (payload.reply) {
    assistantMessages.value = [
      ...assistantMessages.value,
      { role: 'assistant', content: payload.reply }
    ]
  }
}

async function requestPromptAssistant({ mode = 'chat', action = 'continue', message = '', appendUserMessage = true } = {}) {
  const userMessage = message.trim()
  const sourcePrompt = assistantSourcePrompt(userMessage)
  if (!sourcePrompt || assistantRunning.value) return
  const nextMessages = appendUserMessage && userMessage
    ? [
        ...assistantMessages.value,
        { role: 'user', content: userMessage }
      ]
    : [...assistantMessages.value]
  assistantMessages.value = nextMessages
  if (appendUserMessage) {
    assistantInput.value = ''
  }
  assistantRunning.value = true
  assistantRunningAction.value = action
  assistantError.value = ''
  const requestId = ++assistantRequestSeq
  const abortController = new AbortController()
  assistantAbortController.value = abortController
  assistantLastRequest.value = {
    mode,
    action,
    message: userMessage,
    appendUserMessage: false
  }
  try {
    const payload = await api.optimizePrompt({
      prompt: sourcePrompt,
      mode,
      aspect_ratio: aspectRatio.value,
      style_preset: stylePreset.value,
      message: userMessage,
      history: assistantHistoryPayload(nextMessages),
      structured_prompt: assistantStructuredPrompt.value,
      action
    }, {
      signal: abortController.signal
    })
    if (requestId !== assistantRequestSeq || abortController.signal.aborted) return
    applyPromptAssistantPayload(payload)
    if (!assistantDraftPrompt.value.trim()) {
      assistantError.value = '未返回可用提示词，请继续补充描述后重试'
    }
  } catch (error) {
    if (abortController.signal.aborted || error?.name === 'AbortError') {
      return
    }
    assistantError.value = error.message || '提示词助手暂时不可用，请稍后重试'
  } finally {
    if (requestId === assistantRequestSeq) {
      assistantRunning.value = false
      assistantRunningAction.value = ''
      if (assistantAbortController.value === abortController) {
        assistantAbortController.value = null
      }
    }
  }
}

function retryAssistantRequest() {
  if (!assistantLastRequest.value || assistantRunning.value) return
  void requestPromptAssistant(assistantLastRequest.value)
}

function sendAssistantMessage() {
  void requestPromptAssistant({
    mode: 'chat',
    action: 'continue',
    message: assistantInput.value
  })
}

function runAssistantQuickAction(action) {
  const actionMap = {
    refine: {
      mode: 'chat',
      action: 'continue',
      message: '继续细化'
    },
    realistic: {
      mode: 'realistic',
      action: 'make_realistic',
      message: '更偏写实'
    },
    direction: {
      mode: 'direction',
      action: 'change_direction',
      message: '换个方向'
    },
    rewrite: {
      mode: 'rewrite',
      action: 'rewrite',
      message: '重新整理'
    }
  }
  void requestPromptAssistant(actionMap[action])
}

function runAssistantStarter(starter) {
  void requestPromptAssistant({
    mode: 'chat',
    action: 'start',
    message: starter.message
  })
}

function applyAssistantPrompt() {
  const text = assistantDraftPrompt.value.trim()
  if (!text) return
  prompt.value = text
  closePromptOptimizer()
}

function startEditingAssistantField(key) {
  assistantEditingField.value = key
  assistantFieldDraft.value = assistantStructuredPrompt.value[key] || ''
}

function saveAssistantField() {
  const key = assistantEditingField.value
  if (!key) return
  const nextStructuredPrompt = {
    ...assistantStructuredPrompt.value,
    [key]: assistantFieldDraft.value.trim()
  }
  assistantStructuredPrompt.value = nextStructuredPrompt
  assistantDraftPrompt.value = composeAssistantDraftPrompt(nextStructuredPrompt)
  assistantEditingField.value = ''
  assistantFieldDraft.value = ''
}

function cancelAssistantFieldEdit() {
  assistantEditingField.value = ''
  assistantFieldDraft.value = ''
}

function selectAssistantDirection(direction) {
  const nextPrompt = (direction?.prompt || '').trim()
  const nextStructuredPrompt = normalizedAssistantStructuredPrompt(direction?.structured_prompt || {})
  if (!nextPrompt && !hasAssistantStructuredValue(nextStructuredPrompt)) return
  if (hasAssistantStructuredValue(nextStructuredPrompt)) {
    assistantStructuredPrompt.value = nextStructuredPrompt
  }
  const title = (direction?.title || '').trim()
  assistantDraftPrompt.value = nextPrompt || composeAssistantDraftPrompt(nextStructuredPrompt)
  assistantDirections.value = []
  assistantMessages.value = [
    ...assistantMessages.value,
    {
      role: 'assistant',
      content: title ? `已切换到「${title}」方向，字段和提示词预览已同步。` : '已切换方向，字段和提示词预览已同步。'
    }
  ]
}

function retryGeneration(taskToRetry = failedTask.value) {
  const retryTask = taskToRetry?.generation_id ? taskToRetry : failedTask.value
  if (!retryTask?.parameters || submitting.value) return
  // 服务端 parameters 是重试请求体的权威来源；旧任务可能缺 prompt，
  // 用任务上的本地 prompt 兜底，仍为空则提示用户调整而不是发出必败请求。
  const retryPrompt = String(retryTask.parameters.prompt || retryTask.prompt || '').trim()
  if (!retryPrompt) {
    taskError.value = '该任务缺少提示词，请在输入框填写后重新生成'
    return
  }
  const retryPayload = { ...retryTask.parameters, prompt: retryPrompt }
  taskError.value = ''
  resultError.value = ''
  successMessage.value = ''
  regenerateResultError.value = ''
  submitting.value = true
  void createGenerationTask(retryPayload, retryPrompt)
    .catch((error) => {
      const message = failureMessage({
        error: {
          code: error.code || '',
          message: error.message || '',
          retryable: true
        }
      })
      taskError.value = message
      resultError.value = 'failed'
    })
    .finally(() => {
      submitting.value = false
    })
}

function compactGenerationPayload(payload = {}) {
  return Object.fromEntries(
    Object.entries(payload).filter(([, value]) => value !== undefined && value !== null && value !== '')
  )
}

function buildRegeneratePayloadFromResult(source) {
  if (!source) return null
  const parameterPayload = source.parameters && typeof source.parameters === 'object'
    ? { ...source.parameters }
    : null
  if (parameterPayload) {
    const nextPrompt = String(parameterPayload.prompt || source.prompt || '').trim()
    if (!nextPrompt) return null
    return compactGenerationPayload({
      ...parameterPayload,
      prompt: nextPrompt
    })
  }
  const nextPrompt = String(source.prompt || '').trim()
  if (!nextPrompt) return null
  return compactGenerationPayload({
    prompt: nextPrompt,
    negative_prompt: source.negative_prompt,
    aspect_ratio: source.aspect_ratio || aspectRatio.value,
    style_preset: source.style_preset,
    model_id: source.model_id,
    tool_mode: source.tool_mode || toolMode.value || 'generate'
  })
}

async function regenerateFromResult() {
  const source = previewAsset.value
  if (!source) return
  if (!isLoggedIn.value) {
    requireLogin()
    return
  }
  if (submitting.value || regeneratingResult.value) return
  const requestPayload = buildRegeneratePayloadFromResult(source)
  if (!requestPayload?.prompt) {
    regenerateResultError.value = '该作品缺少提示词，无法再次生成'
    return
  }
  if (lowCredit.value) {
    const message = '点数不足，请先前往套餐与充值页面购买点数。'
    regenerateResultError.value = message
    taskError.value = message
    return
  }
  prompt.value = source.prompt || prompt.value
  aspectRatio.value = source.aspect_ratio || aspectRatio.value
  stylePreset.value = source.style_preset || stylePreset.value
  toolMode.value = source.tool_mode || toolMode.value
  workspaceTab.value = 'create'
  regenerateResultError.value = ''
  taskError.value = ''
  resultError.value = ''
  successMessage.value = ''
  regeneratingResult.value = true
  submitting.value = true
  try {
    await createGenerationTask(requestPayload, requestPayload.prompt)
  } catch (error) {
    regenerateResultError.value = error?.message
      ? `再次生成提交失败：${error.message}`
      : '再次生成提交失败，请稍后重试'
  } finally {
    regeneratingResult.value = false
    submitting.value = false
  }
}

function usePreviewAssetAsReference() {
  if (previewAsset.value) {
    handleUseWorkAsReference(previewAsset.value)
  }
}

async function togglePreviewAssetFavorite() {
  const id = workID(previewAsset.value)
  if (!id) return
  const nextValue = !previewAsset.value?.is_favorite
  try {
    await api.updateWork(id, { is_favorite: nextValue })
    updateCachedWork(id, (work) => ({ ...work, is_favorite: nextValue }))
    if (result.value && workID(result.value) === id) {
      result.value = { ...result.value, is_favorite: nextValue }
    }
    successMessage.value = nextValue ? '已收藏。' : '已取消收藏。'
  } catch (error) {
    resultError.value = error.message || '收藏失败'
  }
}

async function sharePreviewAsset() {
  const source = previewAsset.value
  const id = workID(source)
  if (!id) return
  try {
    if (source?.visibility === 'private') {
      await api.updateWork(id, { visibility: 'public' })
      updateCachedWork(id, (work) => ({ ...work, visibility: 'public' }))
      if (result.value && workID(result.value) === id) {
        result.value = { ...result.value, visibility: 'public' }
      }
    }
    router.push(`/works/share?ids=${id}`)
  } catch (error) {
    resultError.value = error.message || '分享失败'
  }
}

function openWorksLibrary() {
  router.push('/works')
}

function downloadImage() {
  if (!previewAsset.value?.download_url) return
  window.open(previewAsset.value.download_url, '_blank')
}

function openPreviewZoom() {
  if (!previewAsset.value?.preview_url) return
  resetPreviewZoom()
  previewZoomOpen.value = true
}

function closePreviewZoom() {
  previewZoomOpen.value = false
  resetPreviewZoom()
}

function handlePreviewKeydown(event) {
  if (event.key === 'Escape' && previewZoomOpen.value) {
    closePreviewZoom()
    return
  }
  if (event.key === 'Escape' && promptOptimizerOpen.value) {
    closePromptOptimizer()
  }
}

function focusablePromptOptimizerElements(container) {
  return Array.from(container.querySelectorAll([
    'button:not([disabled])',
    'textarea:not([disabled])',
    'input:not([disabled])',
    'select:not([disabled])',
    'a[href]',
    '[tabindex]:not([tabindex="-1"])'
  ].join(','))).filter((element) => element instanceof HTMLElement)
}

function handlePromptOptimizerKeydown(event) {
  if (event.key === 'Escape') {
    event.preventDefault()
    closePromptOptimizer()
    return
  }
  if (event.key !== 'Tab') return
  const focusableElements = focusablePromptOptimizerElements(event.currentTarget)
  if (!focusableElements.length) return
  event.preventDefault()
  const currentIndex = focusableElements.indexOf(document.activeElement)
  const nextIndex = currentIndex < 0
    ? (event.shiftKey ? focusableElements.length - 1 : 0)
    : event.shiftKey
    ? (currentIndex <= 0 ? focusableElements.length - 1 : currentIndex - 1)
    : (currentIndex < 0 ? 0 : (currentIndex + 1) % focusableElements.length)
  focusableElements[nextIndex]?.focus()
}

function scrollAssistantMessagesToBottom() {
  void nextTick(() => {
    const element = assistantMessagesRef.value
    if (!element) return
    element.scrollTop = element.scrollHeight
  })
}

async function bootstrap() {
  loading.value = true
  pageError.value = ''

  try {
    await loadSession()
    const discoveryPromise = loadWorkspaceDiscovery()
    if (me.value?.username) {
      await Promise.all([loadWorks(), loadReferenceAssets(), discoveryPromise])
      restoreWorkspacePrefillOrDraft()
      await restoreActiveGeneration()
    } else {
      await discoveryPromise
    }
  } catch (error) {
    pageError.value = error.message
  } finally {
    loading.value = false
  }
}

function requireLogin() {
  confirmLoginBeforeUse(router, '/workspace')
}

onMounted(() => {
  bootstrap()
  window.addEventListener('keydown', handlePreviewKeydown)
})

onBeforeUnmount(() => {
  stopPolling()
  cancelScheduledCreditEstimate()
  clearAgentEstimateTimer()
  cancelAssistantRequest()
  window.removeEventListener('keydown', handlePreviewKeydown)
})

watch([assistantMessages, assistantRunning], () => {
  scrollAssistantMessagesToBottom()
}, { deep: true })

watch([
  prompt,
  negativePrompt,
  aspectRatio,
  quality,
  referenceWeight,
  selectedModelId,
  toolMode,
  stylePreset,
  editInstruction,
  () => selectedReferenceIds.value.join(','),
  () => selectedReferenceWorkIds.value.join(','),
  () => selectedSourceWorkId.value || '',
  () => JSON.stringify(eraseMaskRegions.value),
  () => JSON.stringify(toolOptions.value)
], () => {
  if (suppressCreditEstimateWatch) {
    persistWorkspaceDraft()
    return
  }
  if (workspaceMode.value === 'agent') {
    if (!syncingAgentPlanToComposer && agentPlan.value?.prompt) {
      markAgentEstimateDirty()
    }
  } else {
    scheduleCreditEstimate()
  }
  persistWorkspaceDraft()
})

watch([
  toolMode,
  () => selectedReferenceIds.value.join(','),
  () => selectedReferenceWorkIds.value.join(','),
  () => selectedSourceWorkId.value || ''
], () => {
  clearEraseMask()
  maskSelectionMode.value = 'brush'
})

watch([eraseMaskStrokes, eraseSourcePreview], () => {
  void nextTick(redrawEraseMaskPreview)
}, { deep: true })
</script>

<template>
  <section class="workspace-page-v2">
    <div v-if="loading" class="page-status">正在加载工作台...</div>
    <p v-else-if="pageError" class="status-error">{{ pageError }}</p>

    <div v-else class="workshop-home" data-testid="workshop-home">
      <header class="workshop-hero" :class="{ compact: heroEngaged }">
        <h1 data-testid="workshop-hero-title">今天你想要 <span>创作</span> 什么？</h1>
        <div class="workshop-mode-tabs" aria-label="创作模式">
          <button
            type="button"
            data-testid="workspace-mode-agent"
            :class="{ active: workspaceMode === 'agent' }"
            @click="selectWorkshopMode('agent')"
          >
            <span data-testid="workspace-tab-discovery">Agent模式</span>
          </button>
          <button
            type="button"
            data-testid="workspace-tab-create"
            :class="{ active: workspaceMode === 'create' }"
            @click="selectWorkshopMode('image')"
          >
            图片生成
          </button>
          <button
            type="button"
            data-testid="workspace-mode-video"
            @click="selectWorkshopMode('video')"
          >
            视频生成
          </button>
        </div>
      </header>

      <div
        class="workshop-engage-zone"
        data-testid="workspace-composer-engage-zone"
        @focusin="engageHero"
      >
      <WorkspaceComposerPanel
        v-if="workspaceMode === 'create'"
        ref="composerPanelRef"
        layout-variant="home"
        v-model:selected-model-id="selectedModelId"
        v-model:prompt="prompt"
        v-model:negative-prompt="negativePrompt"
        v-model:style-preset="stylePreset"
        v-model:auto-translate="autoTranslate"
        v-model:aspect-ratio="aspectRatio"
        v-model:quality="quality"
        v-model:reference-weight="referenceWeight"
        v-model:edit-instruction="editInstruction"
        v-model:erase-brush-size="eraseBrushSize"
        v-model:mask-selection-mode="maskSelectionMode"
        :me="me"
        :requires-auth="!isLoggedIn"
        :displayed-model-name="displayedModelName"
        :workspace-models="workspaceModels"
        :selected-reference-images="selectedReferenceItems"
        :source-image-limit="sourceImageLimit"
        :reference-uploading="referenceUploading"
        :reference-error="referenceError"
        :reference-upload-title="referenceUploadTitle"
        :reference-upload-hint="referenceUploadHint"
        :is-expand-tool="isExpandTool"
        :expand-edges="expandEdges"
        :expand-preview-style="expandPreviewStyle"
        :expand-original-style="expandOriginalStyle"
        :expand-source-preview="expandSourcePreview"
        :is-mask-selection-tool="isMaskSelectionTool"
        :erase-source-preview="eraseSourcePreview"
        :is-precision-edit-tool="isPrecisionEditTool"
        :has-erase-mask="hasEraseMask"
        :erase-mask-regions="eraseMaskRegions"
        :prompt-label="promptLabel"
        :prompt-placeholder="promptPlaceholder"
        :task="task"
        :submitting="submitting"
        :style-presets="stylePresets"
        :quality-options="qualityOptions"
        :rendered-tool-fields="renderedToolFields"
        :expand-shortcut-presets="expandShortcutPresets"
        :tool-options="toolOptions"
        :can-submit="canSubmit"
        :current-estimated-credits="currentEstimatedCredits"
        :task-error="taskError"
        :credit-estimate-error="creditEstimateError"
        :credit-estimate-notice="creditEstimateNotice"
        :is-edit-tool="isEditTool"
        :show-reference-strength="showReferenceStrength"
        :has-edit-source-image="hasEditSourceImage"
        :effective-prompt="effectivePrompt"
        :can-retry-generation="canRetryGeneration"
        :can-cancel-generation="canCancelGeneration"
        :cancel-generation-loading="cancelGenerationLoading"
        :is-cancelled-task="selectedTaskCancelled"
        :success-message="successMessage"
        @submit="submit"
        @upload-reference="handleReferenceUpload"
        @remove-reference="handleReferenceRemove"
        @retry-reference-assets="loadReferenceAssets"
        @require-auth="requireLogin"
        @open-prompt-optimizer="openPromptOptimizer"
        @select-style-preset="selectStylePreset"
        @apply-expand-preset="applyExpandPreset"
        @set-tool-option="setToolOption"
        @mask-pointer-down="beginEraseMaskStroke"
        @mask-pointer-move="moveEraseMaskStroke"
        @mask-pointer-up="endEraseMaskStroke"
        @undo-mask="undoEraseMaskStroke"
        @clear-mask="clearEraseMask"
        @select-mask-mode="selectMaskSelectionMode"
        @retry-generation="retryGeneration"
        @cancel-generation="cancelGeneration"
      />

      <AgentWorkspacePanel
        v-if="workspaceMode === 'agent'"
        :step="agentStep"
        :messages="agentMessages"
        :plan="agentPlan"
        :candidates="agentCandidates"
        :selected-candidate-id="agentSelectedCandidateId"
        :safety-notes="agentSafetyNotes"
        :planning="agentPlanning"
        :clarification-prompt="agentClarificationPrompt"
        :reference-items="selectedReferenceItems"
        :works="latestWorks"
        :reference-uploading="referenceUploading"
        :requires-auth="!isLoggedIn"
        :credit-estimate="creditEstimate"
        :credit-estimate-loading="creditEstimateLoading"
        :credit-estimate-error="creditEstimateError"
        :estimate-dirty="agentEstimateDirty"
        :can-confirm-generate="agentCanConfirmGenerate"
        :confirm-disabled-reason="agentConfirmDisabledReason"
        :submitting="submitting"
        :task="agentExecutionTask"
        :error="agentModeError"
        :failure="agentFailure"
        :stage-copy="agentExecutionStageCopy"
        @send-message="sendAgentMessage"
        @upload-reference="handleReferenceUpload"
        @remove-reference="handleReferenceRemove"
        @clear-references="clearAgentReferences"
        @use-work-reference="handleUseWorkAsReference"
        @require-auth="requireLogin"
        @update-plan="updateAgentPlan"
        @select-candidate="selectAgentCandidate"
        @estimate="estimateAgentPlan"
        @confirm-generate="confirmAgentGeneration"
        @retry-plan="retryAgentPlan"
        @retry-generate="retryAgentGeneration"
        @open-result="openPreviewZoom"
        @use-result-reference="handleUseWorkAsReference"
        @view-works="openWorksLibrary"
      />
      </div>

      <div
        v-if="workspaceMode === 'create'"
        class="workshop-toolbar-row"
        :class="{ 'has-filters': workspaceTab === 'discover' }"
      >
        <div class="workshop-quick-prompts" aria-label="快捷提示词">
          <button
            v-for="chip in quickPromptChips"
            :key="chip.key"
            type="button"
            :data-testid="`workspace-quick-prompt-${chip.key}`"
            :disabled="submitting"
            @click="applyQuickPromptChip(chip)"
          >
            {{ chip.label }}
          </button>
        </div>

        <div
          v-if="workspaceTab === 'discover'"
          class="imini-discovery-filters workshop-filter-tabs"
          aria-label="发现筛选"
        >
          <button
            type="button"
            :class="{ active: discoveryFilter === 'all' }"
            data-testid="workspace-discovery-filter-all"
            @click="discoveryFilter = 'all'"
          >
            全部
          </button>
          <button
            type="button"
            :class="{ active: discoveryFilter === 'image' }"
            data-testid="workspace-discovery-filter-image"
            @click="discoveryFilter = 'image'"
          >
            图片
          </button>
          <button
            type="button"
            :class="{ active: discoveryFilter === 'video' }"
            data-testid="workspace-discovery-filter-video"
            @click="discoveryFilter = 'video'"
          >
            视频
          </button>
          <button
            type="button"
            :class="{ active: discoveryFilter === 'tool' }"
            data-testid="workspace-discovery-filter-tool"
            @click="discoveryFilter = 'tool'"
          >
            工具
          </button>
        </div>
      </div>

      <section v-if="workspaceMode === 'create' && workspaceTab === 'discover'" class="workshop-discovery" data-testid="workspace-discovery-panel">
        <div class="preview-container workspace-preview-probe" aria-hidden="true" @dblclick="openPreviewZoom"></div>

        <div v-if="discoveryError" class="workspace-inline-error" role="alert">
          <p class="error-message">{{ discoveryError }}</p>
          <button type="button" class="secondary-button" @click="loadWorkspaceDiscovery">重试</button>
        </div>

        <section
          v-if="['all', 'image'].includes(discoveryFilter) && discoveryRecommendations.length > 0"
          class="workshop-recommendations"
          data-testid="workspace-inspiration-recommendations"
        >
          <div class="workshop-recommendation-head">
            <span class="workshop-tool-title">
              <span class="workshop-section-icon" aria-hidden="true">
                <Sparkles :size="19" />
              </span>
              <span>
                <h2>热门推荐</h2>
                <small>高质量样图，一键套用参数开始创作</small>
              </span>
            </span>
          </div>
          <div class="workshop-recommendation-masonry">
            <article
              v-for="item in discoveryRecommendations"
              :key="item.id"
              class="workshop-recommendation-card"
              :data-testid="`workspace-recommendation-${item.id}`"
            >
              <button
                type="button"
                class="workshop-recommendation-media"
                :aria-label="`预览 ${item.title}`"
                @click="openRecommendationPreview(item)"
              >
                <img :src="item.preview_url" :alt="item.title" />
              </button>
              <div class="workshop-recommendation-content">
                <div class="workshop-recommendation-title">
                  <span v-if="item.category">{{ item.category }}</span>
                  <strong>{{ item.title }}</strong>
                </div>
                <p v-if="item.description">{{ item.description }}</p>
                <div v-if="item.heat_tags.length" class="workshop-recommendation-tags">
                  <span v-for="tag in item.heat_tags" :key="tag">{{ tag }}</span>
                </div>
                <button
                  type="button"
                  class="workshop-recommendation-use"
                  :data-testid="`workspace-recommendation-use-${item.id}`"
                  @click.stop="applyInspirationRecommendation(item)"
                >
                  <Sparkles :size="15" />
                  一键同款
                </button>
              </div>
            </article>
          </div>
        </section>

        <section v-if="['all', 'tool'].includes(discoveryFilter)" class="workshop-feature-section">
          <div class="workshop-feature-grid">
            <button
              v-for="entry in workshopFeatureEntries"
              :key="entry.key"
              type="button"
              class="workshop-feature-item"
              :data-testid="`workspace-feature-${entry.key}`"
              @click="openWorkshopFeature(entry)"
              :aria-disabled="entry.disabled ? 'true' : 'false'"
            >
              <span aria-hidden="true"><component :is="entry.icon" :size="26" /></span>
              <strong>{{ entry.label }}</strong>
              <small v-if="entry.disabled">即将开放</small>
            </button>
          </div>
        </section>

        <section v-if="['all', 'tool'].includes(discoveryFilter)" class="workshop-tool-row" aria-label="AI 工具">
          <div class="workshop-tool-head">
            <span class="workshop-tool-title">
              <span class="workshop-section-icon" aria-hidden="true">
                <Wand2 :size="19" />
              </span>
              <h2>AI 工具</h2>
            </span>
            <span class="workshop-tool-badge">发现更多可能</span>
          </div>
          <div class="workshop-tool-grid">
            <button
              v-for="tool in aiToolCards"
              :key="tool.mode"
              type="button"
              class="workshop-tool-card"
              :class="{ active: toolMode === tool.mode }"
              :data-testid="`workspace-tool-${tool.mode}`"
              :disabled="submitting"
              @click="selectToolCard(tool)"
            >
              <span class="workshop-tool-card-copy">
                <span class="workshop-tool-copy-head">
                  <span class="workshop-tool-icon" aria-hidden="true">
                    <component :is="tool.icon" :size="18" />
                  </span>
                  <span class="workshop-tool-enter" aria-hidden="true">
                    <ArrowUpRight :size="16" />
                  </span>
                </span>
                <strong>{{ tool.title }}</strong>
                <small>{{ tool.description }}</small>
              </span>
              <span class="workshop-tool-card-media">
                <img :src="tool.image" :alt="tool.title" />
              </span>
            </button>
          </div>
        </section>

        <section v-if="['all', 'tool'].includes(discoveryFilter)" class="workshop-playground-row">
          <h2>创作乐园</h2>
          <div class="workshop-playground-grid">
            <button
              v-for="item in playgroundCards"
              :key="item.id"
              type="button"
              class="workshop-playground-card"
              :data-testid="`workspace-playground-${item.id}`"
              @click="openPlaygroundItem(item)"
            >
              <span class="imini-playground-media">
                <img :src="item.image" :alt="item.title" />
              </span>
              <span class="imini-playground-content">
                <span class="imini-card-enter" aria-hidden="true"></span>
                <strong>{{ item.title }}</strong>
                <small>{{ item.description }}</small>
              </span>
            </button>
          </div>
        </section>

        <section v-if="['all', 'image'].includes(discoveryFilter)" class="workshop-workflow" data-testid="workspace-ecommerce-workflow">
          <div class="workshop-section-head">
            <h2>电商工作流</h2>
            <span>选择一个模板开始</span>
          </div>
          <div class="workshop-workflow-grid">
            <button
              v-for="item in ecommerceWorkflowCards"
              :key="item.id"
              type="button"
              class="workshop-workflow-card"
              :data-testid="`workspace-template-${item.id}`"
              @click="applyDiscoveryTemplate(item)"
            >
              <span class="workshop-template-hit" :data-testid="`workspace-workflow-card-${item.id}`" aria-hidden="true"></span>
              <span class="workshop-workflow-title imini-case-card-content">
                <span class="imini-card-enter" aria-hidden="true"></span>
                <strong>{{ item.title }}</strong>
                <span>{{ item.description }}</span>
              </span>
              <span class="imini-case-card-media">
                <img :src="item.preview_url" :alt="item.title" />
              </span>
              <span class="workshop-workflow-copy">{{ item.description }}</span>
            </button>
          </div>
        </section>

        <section v-if="discoveryFilter === 'video'" class="imini-section">
          <div class="imini-empty-line imini-video-empty">暂无视频案例</div>
        </section>
      </section>

      <WorkspaceDiscoverySurface
        v-if="workspaceMode === 'create' && workspaceTab === 'create'"
        v-model:workspace-tab="workspaceTab"
        v-model:discovery-filter="discoveryFilter"
        :ai-tool-cards="aiToolCards"
        :playground-cards="playgroundCards"
        :case-templates="discoveryCaseTemplates"
        :tool-mode="toolMode"
        :task="task"
        :submitting="submitting"
        :works="works"
        :works-total="worksTotal"
        :works-page="worksPage"
        :works-page-size="worksPageSize"
        :works-loading="worksLoading"
        :works-error="worksError"
        :discovery-error="discoveryError"
        :result-error="selectedResultError"
        :preview-asset="previewAsset"
        :is-remove-background-result="isRemoveBackgroundResult"
        :stage-copy="stageCopy"
        :can-retry-generation="canRetryGeneration"
        :can-cancel-generation="canCancelGeneration"
        :cancel-generation-loading="cancelGenerationLoading"
        :regenerate-result-loading="regeneratingResult"
        :regenerate-result-error="regenerateResultError"
        :cancelling-task-ids="cancellingTaskIds"
        :is-cancelled-task="selectedTaskCancelled"
        :active-tasks="activeTasks"
        :selected-task-id="selectedTaskId"
        @select-tool="selectToolCard"
        @open-playground-item="openPlaygroundItem"
        @select-template="applyDiscoveryTemplate"
        @open-preview-zoom="openPreviewZoom"
        @regenerate-result="regenerateFromResult"
        @use-result-as-reference="usePreviewAssetAsReference"
        @toggle-result-favorite="togglePreviewAssetFavorite"
        @share-result="sharePreviewAsset"
        @open-works-library="openWorksLibrary"
        @retry-generation="retryGeneration"
        @cancel-generation="cancelGeneration"
        @select-generation-task="selectGenerationTask"
        @retry-generation-task="retryGeneration"
        @cancel-generation-task="cancelGeneration"
        @retry-works="loadWorks"
        @change-works-page="changeWorksPage"
        @retry-discovery="loadWorkspaceDiscovery"
        @download-image="downloadImage"
        @select-history-work="selectHistoryWork"
        @use-work-as-reference="handleUseWorkAsReference"
      />
    </div>

    <Teleport to="body">
      <div
        v-if="promptOptimizerOpen"
        class="prompt-optimizer-modal"
        data-testid="workspace-prompt-optimizer-modal"
        role="dialog"
        aria-modal="true"
        aria-label="AI 提示词助手"
        @click="closePromptOptimizer"
        @keydown="handlePromptOptimizerKeydown"
      >
        <div class="prompt-optimizer-sheet" @click.stop>
          <div class="prompt-optimizer-head">
            <div>
              <strong>AI 提示词助手</strong>
              <span>描述你的想法，我会整理成可直接生成的图片提示词</span>
            </div>
            <button
              class="workspace-preview-close"
              type="button"
              aria-label="关闭提示词助手"
              title="关闭"
              @click="closePromptOptimizer"
            >
              <X :size="18" />
            </button>
          </div>

          <div class="prompt-assistant-layout">
            <section class="prompt-assistant-chat" aria-label="AI 对话区">
              <div ref="assistantMessagesRef" class="prompt-assistant-messages">
                <div
                  v-for="(message, index) in assistantMessages"
                  :key="`${message.role}-${index}-${message.content}`"
                  class="prompt-assistant-message"
                  :class="`is-${message.role}`"
                >
                  <span v-if="message.role === 'assistant'" class="prompt-assistant-avatar" aria-hidden="true">
                    <Bot :size="17" />
                  </span>
                  <p>{{ message.content }}</p>
                </div>
                <div v-if="assistantRunning" class="prompt-assistant-message is-assistant">
                  <span class="prompt-assistant-avatar" aria-hidden="true">
                    <Bot :size="17" />
                  </span>
                  <div class="prompt-assistant-loading-bubble">
                    <p>{{ assistantRunningMessage }}</p>
                    <button
                      class="mini-button"
                      type="button"
                      data-testid="workspace-assistant-cancel"
                      @click="cancelAssistantRequest"
                    >
                      取消
                    </button>
                  </div>
                </div>

                <div
                  v-if="showAssistantResultBubble"
                  class="prompt-assistant-message is-assistant is-result"
                >
                  <span class="prompt-assistant-avatar" aria-hidden="true">
                    <Bot :size="17" />
                  </span>

                  <section
                    class="prompt-assistant-result prompt-assistant-result-bubble"
                    data-testid="workspace-assistant-result-bubble"
                    aria-label="实时整理结果"
                  >
                    <div class="prompt-assistant-result-head">
                      <strong>实时整理结果</strong>
                      <span>可编辑后再应用</span>
                    </div>

                    <div class="prompt-assistant-fields">
                      <div
                        v-for="field in promptAssistantFields"
                        :key="field.key"
                        class="prompt-assistant-field"
                      >
                        <span class="prompt-assistant-field-icon" aria-hidden="true">
                          <component :is="field.icon" :size="17" />
                        </span>
                        <div class="prompt-assistant-field-content">
                          <span class="prompt-assistant-field-label">{{ field.label }}</span>
                          <input
                            v-if="assistantEditingField === field.key"
                            v-model="assistantFieldDraft"
                            class="prompt-assistant-field-input"
                            :aria-label="`编辑${field.label}`"
                            @keydown.enter.prevent="saveAssistantField"
                            @keydown.esc.prevent="cancelAssistantFieldEdit"
                            @blur="saveAssistantField"
                          />
                          <span
                            v-else
                            class="prompt-assistant-field-value"
                            :class="{ 'is-empty': !assistantStructuredFieldValue(field.key) }"
                          >
                            {{ assistantStructuredFieldValue(field.key) || assistantFieldEmptyText }}
                          </span>
                        </div>
                        <button
                          class="prompt-assistant-edit"
                          type="button"
                          :aria-label="`编辑${field.label}`"
                          :title="`编辑${field.label}`"
                          @click="startEditingAssistantField(field.key)"
                        >
                          <Edit3 :size="15" />
                        </button>
                      </div>
                    </div>

                    <label class="prompt-assistant-preview-label" for="workspace-assistant-draft-prompt">
                      提示词预览
                    </label>
                    <textarea
                      id="workspace-assistant-draft-prompt"
                      v-model="assistantDraftPrompt"
                      data-testid="workspace-assistant-draft-prompt"
                      class="prompt-assistant-preview"
                      placeholder="AI 会在这里整理出完整提示词"
                      rows="10"
                      maxlength="1200"
                    />

                    <div v-if="assistantNotes.length" class="prompt-optimizer-notes">
                      <span v-for="note in assistantNotes" :key="note">{{ note }}</span>
                    </div>

                    <div class="prompt-assistant-quick-actions" aria-label="快捷操作">
                      <button
                        class="secondary-button"
                        type="button"
                        :disabled="assistantRunning"
                        @click="runAssistantQuickAction('refine')"
                      >
                        <Wand2 :size="15" />
                        {{ assistantRunningAction === 'continue' ? '正在整理...' : '继续细化' }}
                      </button>
                      <button
                        class="secondary-button"
                        type="button"
                        data-testid="workspace-assistant-realistic"
                        :disabled="assistantRunning"
                        @click="runAssistantQuickAction('realistic')"
                      >
                        <Sparkles :size="15" />
                        {{ assistantRunningAction === 'make_realistic' ? '正在写实化...' : '更偏写实' }}
                      </button>
                      <button
                        class="secondary-button"
                        type="button"
                        data-testid="workspace-assistant-change-direction"
                        :disabled="assistantRunning"
                        @click="runAssistantQuickAction('direction')"
                      >
                        <RefreshCw :size="15" />
                        {{ assistantRunningAction === 'change_direction' ? '正在换方向...' : '换个方向' }}
                      </button>
                    </div>

                    <div v-if="assistantDirections.length" class="prompt-assistant-directions">
                      <button
                        v-for="(direction, index) in assistantDirections"
                        :key="`${direction.title}-${index}`"
                        type="button"
                        class="prompt-assistant-direction"
                        :data-testid="`workspace-assistant-direction-${index}`"
                        @click="selectAssistantDirection(direction)"
                      >
                        <strong>{{ direction.title || `方向 ${index + 1}` }}</strong>
                        <span>{{ direction.summary || direction.prompt }}</span>
                      </button>
                    </div>

                    <div class="prompt-assistant-footer">
                      <button
                        class="secondary-button"
                        type="button"
                        :disabled="assistantRunning"
                        @click="runAssistantQuickAction('rewrite')"
                      >
                        {{ assistantRunningAction === 'rewrite' ? '正在重新整理...' : '重新整理' }}
                      </button>
                      <button
                        class="primary-button"
                        type="button"
                        data-testid="workspace-apply-assistant-prompt"
                        :disabled="assistantRunning || !assistantDraftPrompt.trim()"
                        @click="applyAssistantPrompt"
                      >
                        应用到提示词
                      </button>
                    </div>
                  </section>
                </div>
              </div>

              <div v-if="assistantError" class="prompt-assistant-error" role="alert">
                <span>{{ assistantError }}</span>
                <button
                  class="mini-button"
                  type="button"
                  data-testid="workspace-assistant-retry"
                  :disabled="assistantRunning"
                  @click="retryAssistantRequest"
                >
                  重试本次操作
                </button>
              </div>

              <div v-if="showAssistantStarters" class="prompt-assistant-starters" aria-label="方向入口">
                <button
                  v-for="starter in promptAssistantStarters"
                  :key="starter.key"
                  type="button"
                  class="secondary-button"
                  :data-testid="`workspace-assistant-starter-${starter.key}`"
                  @click="runAssistantStarter(starter)"
                >
                  {{ starter.label }}
                </button>
              </div>

              <form class="prompt-assistant-input-row" @submit.prevent="sendAssistantMessage">
                <textarea
                  ref="assistantInputRef"
                  v-model="assistantInput"
                  data-testid="workspace-prompt-assistant-input"
                  placeholder="继续描述你的想法或方向..."
                  rows="2"
                  maxlength="600"
                  :disabled="assistantRunning"
                  @keydown.enter.exact.prevent="sendAssistantMessage"
                />
                <button
                  class="prompt-assistant-send"
                  type="submit"
                  data-testid="workspace-prompt-assistant-send"
                  aria-label="发送"
                  title="发送"
                  :disabled="assistantRunning || !assistantInput.trim()"
                >
                  <Send :size="18" />
                </button>
              </form>
            </section>
          </div>
        </div>
      </div>

      <div
        v-if="recommendationPreview"
        class="workspace-preview-zoom workspace-recommendation-preview"
        data-testid="workspace-recommendation-preview-modal"
        role="dialog"
        aria-modal="true"
        aria-label="灵感推荐预览"
        @click="closeRecommendationPreview"
      >
        <div class="workspace-preview-zoom-inner" @click.stop>
          <div class="workspace-preview-zoom-header">
            <div class="workspace-preview-zoom-title">
              <strong>{{ recommendationPreview.title }}</strong>
              <span v-if="recommendationPreview.description">{{ recommendationPreview.description }}</span>
            </div>
            <div class="workspace-preview-zoom-actions">
              <button
                class="workspace-preview-download"
                type="button"
                :data-testid="`workspace-recommendation-preview-use-${recommendationPreview.id}`"
                @click="applyInspirationRecommendation(recommendationPreview)"
              >
                一键同款
              </button>
              <button
                class="workspace-preview-close"
                type="button"
                aria-label="关闭灵感推荐预览"
                @click="closeRecommendationPreview"
              >
                ×
              </button>
            </div>
          </div>
          <div class="workspace-preview-image-wrap">
            <img
              :src="recommendationPreview.preview_url"
              :alt="recommendationPreview.title"
              data-testid="workspace-recommendation-preview-image"
            />
          </div>
        </div>
      </div>

      <div
        v-if="previewZoomOpen && previewAsset?.preview_url"
        class="workspace-preview-zoom"
        data-testid="workspace-preview-modal"
        role="dialog"
        aria-modal="true"
        aria-label="图片放大预览"
        @click="closePreviewZoom"
      >
        <div class="workspace-preview-zoom-inner" @click.stop>
          <div class="workspace-preview-zoom-header">
            <div class="workspace-preview-zoom-title">
              <strong>放大查看</strong>
              <span v-if="previewAsset.prompt">{{ previewAsset.prompt }}</span>
            </div>
            <div class="workspace-preview-zoom-actions">
              <a
                v-if="previewAsset.download_url"
                class="workspace-preview-download"
                :href="previewAsset.download_url"
                target="_blank"
                rel="noopener noreferrer"
              >
                下载原图
              </a>
              <button
                class="workspace-preview-close"
                type="button"
                data-testid="workspace-preview-close"
                aria-label="关闭图片预览"
                @click="closePreviewZoom"
              >
                ×
              </button>
            </div>
          </div>
          <div
            class="workspace-preview-image-wrap"
            data-testid="workspace-preview-zoom-surface"
            @wheel="handlePreviewWheel"
          >
            <img
              ref="previewZoomImageRef"
              :src="previewAsset.preview_url"
              :style="previewZoomStyle"
              alt="当前成果放大预览"
              data-testid="workspace-preview-zoom-image"
            />
          </div>
        </div>
      </div>
    </Teleport>
  </section>
</template>
