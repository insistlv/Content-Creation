<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { Download, RefreshCw, Shirt, Sparkles, Wand2 } from 'lucide-vue-next'
import { useRouter } from 'vue-router'
import { api } from '../api/client.js'
import ImageUploadZone from '../components/ImageUploadZone.vue'

const router = useRouter()

const sceneGroups = [
  {
    key: 'work_business',
    label: '职场商务',
    subScenes: [
      { key: 'office', label: '办公室' },
      { key: 'meeting', label: '商务会议' },
      { key: 'commute', label: '通勤路上' }
    ]
  },
  {
    key: 'social_etiquette',
    label: '社交礼仪',
    subScenes: [
      { key: 'banquet', label: '晚宴' },
      { key: 'date', label: '约会' },
      { key: 'ceremony', label: '典礼' }
    ]
  },
  {
    key: 'sports_outdoor',
    label: '运动户外',
    subScenes: [
      { key: 'running', label: '跑步' },
      { key: 'hiking', label: '徒步' },
      { key: 'fitness', label: '健身房' }
    ]
  },
  {
    key: 'home_private',
    label: '居家私密',
    subScenes: [
      { key: 'living_room', label: '客厅' },
      { key: 'bedroom', label: '卧室' }
    ]
  },
  {
    key: 'special_protection',
    label: '特殊防护',
    subScenes: [
      { key: 'rain', label: '雨天' },
      { key: 'cold', label: '寒冷户外' },
      { key: 'sun', label: '防晒场景' }
    ]
  }
]

const bodyProfile = ref({
  height_cm: '',
  weight_kg: '',
  shoulder_cm: '',
  chest_cm: '',
  waist_cm: '',
  hip_cm: '',
  body_type: '',
  body_fat_label: '',
  fit_preference: '',
  style_preference: ''
})
const bodyMeasurementRules = [
  { field: 'height_cm', label: '身高', min: 80, max: 230, unit: 'cm', required: true },
  { field: 'weight_kg', label: '体重', min: 25, max: 250, unit: 'kg', required: true },
  { field: 'shoulder_cm', label: '肩宽', min: 20, max: 80, unit: 'cm', required: false },
  { field: 'chest_cm', label: '胸围', min: 40, max: 180, unit: 'cm', required: false },
  { field: 'waist_cm', label: '腰围', min: 40, max: 180, unit: 'cm', required: false },
  { field: 'hip_cm', label: '臀围', min: 40, max: 180, unit: 'cm', required: false }
]
const garment = ref({
  category: '',
  size: '',
  material: '',
  color: '',
  fit: '',
  details: ''
})
const scene = ref({
  category: 'work_business',
  sub_scene: 'office',
  pose: '',
  background_preference: '',
  custom_description: ''
})
const generation = ref({
  model_id: null,
  quality: 'medium',
  aspect_ratio: '3:4'
})

const models = ref([])
const garmentAsset = ref(null)
const bodyAsset = ref(null)
const garmentUploading = ref(false)
const bodyUploading = ref(false)
const uploadError = ref('')
const loading = ref(false)
const estimating = ref(false)
const submitting = ref(false)
const estimate = ref(null)
const task = ref(null)
const result = ref(null)
const submitError = ref('')
const bodyFieldErrors = ref({})
const history = ref([])
const polling = ref(false)
let pollTimer = null
let pollDeadline = 0

const generationPollIntervalMs = 2000
const generationPollTimeoutMs = 120000

const selectedSceneGroup = computed(() => sceneGroups.find((item) => item.key === scene.value.category) || sceneGroups[0])
const canSubmit = computed(() => Boolean(
  garmentAsset.value?.id &&
  numberOrNull(bodyProfile.value.height_cm) &&
  numberOrNull(bodyProfile.value.weight_kg) &&
  generation.value.model_id &&
  !garmentUploading.value &&
  !bodyUploading.value &&
  !submitting.value &&
  !polling.value
))
const generationInProgress = computed(() => submitting.value || polling.value)
const generationStatusText = computed(() => {
  if (submitting.value) return '正在提交生成任务'
  if (!polling.value) return ''
  const status = task.value?.status || 'queued'
  if (status === 'queued') return '任务已提交，正在排队生成'
  if (status === 'running' || status === 'processing') return '正在生成，上身效果通常需要几十秒'
  return '正在同步生成结果'
})
const creditEstimateText = computed(() => {
  if (!estimate.value) return '填写身型和服装后可预估点数'
  if (estimate.value.enough === false) {
    return `点数不足，还差 ${estimate.value.missing_credits || 0} 点`
  }
  return `预计 ${estimate.value.required_credits || 0} 点`
})

onMounted(async () => {
  await loadInitialData()
})

onUnmounted(() => {
  clearGenerationPolling()
})

watch(() => scene.value.category, () => {
  scene.value.sub_scene = selectedSceneGroup.value.subScenes[0]?.key || ''
})

watch(bodyProfile, () => {
  clearResolvedBodyFieldErrors()
}, { deep: true })

function numberOrNull(value) {
  if (value === '' || value === null || value === undefined) return null
  const next = Number(value)
  return Number.isFinite(next) ? next : null
}

function cleanString(value) {
  return `${value || ''}`.trim()
}

function resultKey(item) {
  return item?.work_id || item?.id || item?.generation_id || ''
}

function prependHistoryResult(item) {
  if (!item?.preview_url) return
  const key = resultKey(item)
  history.value = [
    item,
    ...history.value.filter((entry) => resultKey(entry) !== key)
  ].slice(0, 6)
}

function clearGenerationPolling() {
  if (pollTimer) {
    clearTimeout(pollTimer)
    pollTimer = null
  }
  polling.value = false
}

function optionalNumber(value) {
  const next = numberOrNull(value)
  return next === null || next === 0 ? undefined : next
}

function formatMeasurementNumber(value) {
  const numeric = Number(value)
  if (!Number.isFinite(numeric)) return `${value}`
  return Number.isInteger(numeric) ? `${numeric}` : `${numeric.toFixed(1).replace(/\.0$/, '')}`
}

function bodyValidationMessage(error) {
  if (!error) return ''
  const label = error.label || bodyMeasurementRules.find((rule) => rule.field === error.field)?.label || error.field
  const unit = error.unit || ''
  const range = `${formatMeasurementNumber(error.min)}-${formatMeasurementNumber(error.max)} ${unit}`.trim()
  if (error.value === null || error.value === undefined) {
    return `${label}不能为空：可用范围 ${range}`
  }
  return `${label}超出范围：当前 ${formatMeasurementNumber(error.value)} ${unit}，可用范围 ${range}`.trim()
}

function normalizedBodyValidationError(rule, value) {
  return {
    field: rule.field,
    label: rule.label,
    value,
    min: rule.min,
    max: rule.max,
    unit: rule.unit,
    required: rule.required
  }
}

function currentBodyFieldError(rule) {
  const value = numberOrNull(bodyProfile.value[rule.field])
  if (value === null) {
    return rule.required ? normalizedBodyValidationError(rule, null) : null
  }
  if (value < rule.min || value > rule.max) {
    return normalizedBodyValidationError(rule, value)
  }
  return null
}

function validateBodyProfile() {
  const errors = {}
  bodyMeasurementRules.forEach((rule) => {
    const error = currentBodyFieldError(rule)
    if (error) {
      errors[rule.field] = error
    }
  })
  bodyFieldErrors.value = errors
  const messages = Object.values(errors).map(bodyValidationMessage).filter(Boolean)
  if (messages.length > 0) {
    submitError.value = messages.join('；')
    return false
  }
  return true
}

function normalizeBackendBodyValidationErrors(error) {
  const items = Array.isArray(error?.validation_errors) ? error.validation_errors : []
  return items.reduce((next, item) => {
    if (item?.field) {
      next[item.field] = item
    }
    return next
  }, {})
}

function applyBackendBodyValidationErrors(error) {
  const errors = normalizeBackendBodyValidationErrors(error)
  if (Object.keys(errors).length === 0) return false
  bodyFieldErrors.value = errors
  submitError.value = Object.values(errors).map(bodyValidationMessage).filter(Boolean).join('；') || error.message || '身型参数填写有误，请按提示修改'
  return true
}

function clearResolvedBodyFieldErrors() {
  const current = bodyFieldErrors.value
  if (!Object.keys(current).length) return
  const next = { ...current }
  bodyMeasurementRules.forEach((rule) => {
    if (!next[rule.field]) return
    if (!currentBodyFieldError(rule)) {
      delete next[rule.field]
    }
  })
  bodyFieldErrors.value = next
  if (Object.keys(next).length === 0 && /身高|体重|肩宽|胸围|腰围|臀围|身型参数/.test(submitError.value)) {
    submitError.value = ''
  }
}

async function loadInitialData() {
  loading.value = true
  submitError.value = ''
  try {
    const discovery = await api.getWorkspaceDiscovery()
    models.value = Array.isArray(discovery?.models) ? discovery.models : []
    generation.value.model_id = models.value[0]?.id ?? generation.value.model_id
  } catch (error) {
    submitError.value = error.message || '建模试衣工作台加载失败'
  } finally {
    loading.value = false
  }
}

async function uploadAsset(file, kind) {
  uploadError.value = ''
  const uploading = kind === 'garment' ? garmentUploading : bodyUploading
  uploading.value = true
  try {
    const uploaded = await api.uploadReferenceAsset(file)
    if (kind === 'garment') {
      garmentAsset.value = uploaded
    } else {
      bodyAsset.value = uploaded
    }
    estimate.value = null
  } catch (error) {
    uploadError.value = error.message || '参考图上传失败'
  } finally {
    uploading.value = false
  }
}

async function removeAsset(asset, kind) {
  uploadError.value = ''
  try {
    if (asset?.id) {
      await api.deleteReferenceAsset(asset.id)
    }
    if (kind === 'garment') {
      garmentAsset.value = null
    } else {
      bodyAsset.value = null
    }
    estimate.value = null
  } catch (error) {
    uploadError.value = error.message || '参考图移除失败'
  }
}

function buildPayload() {
  return {
    body_profile: {
      height_cm: numberOrNull(bodyProfile.value.height_cm),
      weight_kg: numberOrNull(bodyProfile.value.weight_kg),
      shoulder_cm: optionalNumber(bodyProfile.value.shoulder_cm),
      chest_cm: optionalNumber(bodyProfile.value.chest_cm),
      waist_cm: optionalNumber(bodyProfile.value.waist_cm),
      hip_cm: optionalNumber(bodyProfile.value.hip_cm),
      body_type: cleanString(bodyProfile.value.body_type),
      body_fat_label: cleanString(bodyProfile.value.body_fat_label),
      fit_preference: cleanString(bodyProfile.value.fit_preference),
      style_preference: cleanString(bodyProfile.value.style_preference),
      body_reference_asset_id: bodyAsset.value?.id
    },
    garment: {
      garment_reference_asset_id: garmentAsset.value?.id,
      category: cleanString(garment.value.category),
      size: cleanString(garment.value.size),
      material: cleanString(garment.value.material),
      color: cleanString(garment.value.color),
      fit: cleanString(garment.value.fit),
      details: cleanString(garment.value.details)
    },
    scene: {
      category: scene.value.category,
      sub_scene: scene.value.sub_scene,
      pose: cleanString(scene.value.pose),
      background_preference: cleanString(scene.value.background_preference),
      custom_description: cleanString(scene.value.custom_description)
    },
    generation: {
      model_id: Number(generation.value.model_id),
      quality: generation.value.quality,
      aspect_ratio: generation.value.aspect_ratio
    }
  }
}

async function estimateCredits() {
  if (!garmentAsset.value?.id) {
    submitError.value = '请先上传服装图'
    return null
  }
  submitError.value = ''
  if (!validateBodyProfile()) {
    return null
  }
  estimating.value = true
  try {
    const payload = buildPayload()
    const nextEstimate = await api.estimateVirtualTryOn(payload)
    estimate.value = nextEstimate
    return { payload, estimate: nextEstimate }
  } catch (error) {
    if (applyBackendBodyValidationErrors(error)) {
      return null
    }
    submitError.value = error.message || '点数预估失败'
    return null
  } finally {
    estimating.value = false
  }
}

async function submitGeneration() {
  if (!canSubmit.value) return
  clearGenerationPolling()
  submitting.value = true
  submitError.value = ''
  result.value = null
  task.value = null
  try {
    const estimated = await estimateCredits()
    if (!estimated || estimated.estimate?.enough === false) {
      return
    }
    const created = await api.createVirtualTryOn(estimated.payload)
    task.value = created
    const generationId = created?.generation_id || created?.id
    if (generationId) {
      pollDeadline = Date.now() + generationPollTimeoutMs
      polling.value = true
      await pollGenerationResult(generationId, created)
    }
  } catch (error) {
    clearGenerationPolling()
    submitError.value = error.message || '生成任务创建失败'
  } finally {
    submitting.value = false
  }
}

async function pollGenerationResult(generationId, baseTask = {}) {
  try {
    const latest = await api.getImageGeneration(generationId)
    const nextTask = { ...baseTask, ...latest }
    task.value = nextTask

    if (latest?.status === 'succeeded' && latest?.preview_url) {
      result.value = latest
      prependHistoryResult(latest)
      clearGenerationPolling()
      return
    }

    if (latest?.status === 'failed') {
      submitError.value = latest.failure_message || latest.error_message || '生成失败'
      clearGenerationPolling()
      return
    }

    if (Date.now() >= pollDeadline) {
      submitError.value = '生成时间较长，请稍后在作品库查看结果'
      clearGenerationPolling()
      return
    }

    pollTimer = setTimeout(() => {
      pollGenerationResult(generationId, nextTask)
    }, generationPollIntervalMs)
  } catch (error) {
    submitError.value = error.message || '生成状态刷新失败，请稍后在作品库查看结果'
    clearGenerationPolling()
  }
}

function continueWithResultReference() {
  if (!result.value?.work_id) return
  router.push({
    path: '/workspace',
    query: {
      reference_work_id: result.value.work_id
    }
  })
}

function openResultInWorks() {
  router.push('/works')
}
</script>

<template>
  <main class="virtual-try-on-workspace" data-testid="virtual-try-on-workspace">
    <section class="tryon-config">
      <header class="tryon-header">
        <p class="tryon-kicker">AI 创作 / 消费者试衣</p>
        <h1>建模试衣</h1>
        <p>输入身型参数，上传服装图，选择场景后生成一张上身效果图。</p>
      </header>

      <form class="tryon-form" @submit.prevent="submitGeneration">
        <section class="tryon-section" data-testid="virtual-try-on-body-section">
          <div class="section-heading">
            <Sparkles :size="18" />
            <div>
              <h2>身型</h2>
              <p>身高和体重必填，真人全身参考图可选。</p>
            </div>
          </div>
          <div class="tryon-grid">
            <label>
              <span class="field-label"><span>身高 cm</span><small>80-230 cm</small></span>
              <input v-model="bodyProfile.height_cm" data-testid="tryon-height" type="number" min="80" max="230" :class="{ invalid: bodyFieldErrors.height_cm }" />
              <p v-if="bodyFieldErrors.height_cm" class="field-error" data-testid="tryon-height-error">{{ bodyValidationMessage(bodyFieldErrors.height_cm) }}</p>
            </label>
            <label>
              <span class="field-label"><span>体重 kg</span><small>25-250 kg</small></span>
              <input v-model="bodyProfile.weight_kg" data-testid="tryon-weight" type="number" min="25" max="250" :class="{ invalid: bodyFieldErrors.weight_kg }" />
              <p v-if="bodyFieldErrors.weight_kg" class="field-error" data-testid="tryon-weight-error">{{ bodyValidationMessage(bodyFieldErrors.weight_kg) }}</p>
            </label>
            <label>
              <span class="field-label"><span>肩宽 cm</span><small>20-80 cm</small></span>
              <input v-model="bodyProfile.shoulder_cm" data-testid="tryon-shoulder" type="number" min="20" max="80" :class="{ invalid: bodyFieldErrors.shoulder_cm }" />
              <p v-if="bodyFieldErrors.shoulder_cm" class="field-error" data-testid="tryon-shoulder-error">{{ bodyValidationMessage(bodyFieldErrors.shoulder_cm) }}</p>
            </label>
            <label>
              <span class="field-label"><span>胸围 cm</span><small>40-180 cm</small></span>
              <input v-model="bodyProfile.chest_cm" data-testid="tryon-chest" type="number" min="40" max="180" :class="{ invalid: bodyFieldErrors.chest_cm }" />
              <p v-if="bodyFieldErrors.chest_cm" class="field-error" data-testid="tryon-chest-error">{{ bodyValidationMessage(bodyFieldErrors.chest_cm) }}</p>
            </label>
            <label>
              <span class="field-label"><span>腰围 cm</span><small>40-180 cm</small></span>
              <input v-model="bodyProfile.waist_cm" data-testid="tryon-waist" type="number" min="40" max="180" :class="{ invalid: bodyFieldErrors.waist_cm }" />
              <p v-if="bodyFieldErrors.waist_cm" class="field-error" data-testid="tryon-waist-error">{{ bodyValidationMessage(bodyFieldErrors.waist_cm) }}</p>
            </label>
            <label>
              <span class="field-label"><span>臀围 cm</span><small>40-180 cm</small></span>
              <input v-model="bodyProfile.hip_cm" data-testid="tryon-hip" type="number" min="40" max="180" :class="{ invalid: bodyFieldErrors.hip_cm }" />
              <p v-if="bodyFieldErrors.hip_cm" class="field-error" data-testid="tryon-hip-error">{{ bodyValidationMessage(bodyFieldErrors.hip_cm) }}</p>
            </label>
            <label>
              <span>体型标签</span>
              <input v-model="bodyProfile.body_type" data-testid="tryon-body-type" placeholder="如：标准、偏瘦、微胖、梨形" />
            </label>
            <label>
              <span>穿衣偏好</span>
              <input v-model="bodyProfile.fit_preference" data-testid="tryon-fit-preference" placeholder="如：合身、宽松、显瘦" />
            </label>
          </div>
          <ImageUploadZone
            :images="bodyAsset ? [bodyAsset] : []"
            :max-images="1"
            :uploading="bodyUploading"
            empty-title="上传真人全身参考图"
            empty-hint="可选，建议正面自然站姿"
            @upload="(file) => uploadAsset(file, 'body')"
            @remove="(asset) => removeAsset(asset, 'body')"
          />
        </section>

        <section class="tryon-section" data-testid="virtual-try-on-garment-section">
          <div class="section-heading">
            <Shirt :size="18" />
            <div>
              <h2>服装</h2>
              <p>首版以用户上传商品图为服装来源。</p>
            </div>
          </div>
          <ImageUploadZone
            :images="garmentAsset ? [garmentAsset] : []"
            :max-images="1"
            :uploading="garmentUploading"
            empty-title="上传服装图"
            empty-hint="必填，建议白底或平铺商品图"
            @upload="(file) => uploadAsset(file, 'garment')"
            @remove="(asset) => removeAsset(asset, 'garment')"
          />
          <div class="tryon-grid">
            <label>
              <span>品类</span>
              <input v-model="garment.category" data-testid="tryon-garment-category" placeholder="如：衬衫、连衣裙、外套" />
            </label>
            <label>
              <span>尺码</span>
              <input v-model="garment.size" data-testid="tryon-garment-size" placeholder="如：中码、均码、身高165适用" />
            </label>
            <label>
              <span>材质</span>
              <input v-model="garment.material" data-testid="tryon-garment-material" placeholder="如：棉、羊毛、牛仔、雪纺" />
            </label>
            <label>
              <span>颜色</span>
              <input v-model="garment.color" data-testid="tryon-garment-color" placeholder="如：白色、黑色、米色" />
            </label>
            <label>
              <span>版型</span>
              <input v-model="garment.fit" data-testid="tryon-garment-fit" placeholder="如：常规、宽松、修身" />
            </label>
            <label class="wide-field">
              <span>关键细节</span>
              <textarea v-model="garment.details" data-testid="tryon-garment-details" rows="3" placeholder="如：领型、袖长、口袋、纹理等" />
            </label>
          </div>
        </section>

        <section class="tryon-section" data-testid="virtual-try-on-scene-section">
          <div class="section-heading">
            <Wand2 :size="18" />
            <div>
              <h2>场景</h2>
              <p>选择穿衣场景，必要时补充姿态和背景偏好。</p>
            </div>
          </div>
          <div class="scene-tabs" role="tablist" aria-label="试衣场景">
            <button
              v-for="item in sceneGroups"
              :key="item.key"
              type="button"
              :class="{ active: scene.category === item.key }"
              @click="scene.category = item.key"
            >
              {{ item.label }}
            </button>
          </div>
          <div class="tryon-grid">
            <label>
              <span>子场景</span>
              <select v-model="scene.sub_scene" data-testid="tryon-scene-subscene">
                <option v-for="item in selectedSceneGroup.subScenes" :key="item.key" :value="item.key">
                  {{ item.label }}
                </option>
              </select>
            </label>
            <label>
              <span>姿态</span>
              <input v-model="scene.pose" data-testid="tryon-scene-pose" placeholder="如：自然站立、侧身、走路" />
            </label>
            <label class="wide-field">
              <span>背景偏好</span>
              <input v-model="scene.background_preference" data-testid="tryon-scene-background" placeholder="如：明亮办公空间" />
            </label>
            <label class="wide-field">
              <span>自定义说明</span>
              <textarea v-model="scene.custom_description" data-testid="tryon-scene-custom" rows="3" />
            </label>
          </div>
        </section>

        <section class="tryon-section" data-testid="virtual-try-on-generate-section">
          <div class="section-heading">
            <RefreshCw :size="18" />
            <div>
              <h2>生成</h2>
              <p>真人参考图和身体围度默认只用于本次生成，不会保存为体型档案。</p>
            </div>
          </div>
          <div class="tryon-grid">
            <label>
              <span>模型</span>
              <select v-model="generation.model_id" data-testid="tryon-model">
                <option v-for="model in models" :key="model.id" :value="model.id">{{ model.name }}</option>
              </select>
            </label>
            <label>
              <span>质量</span>
              <select v-model="generation.quality" data-testid="tryon-quality">
                <option value="low">0.5K</option>
                <option value="medium">1K</option>
                <option value="high">2K</option>
              </select>
            </label>
            <label>
              <span>比例</span>
              <select v-model="generation.aspect_ratio" data-testid="tryon-aspect-ratio">
                <option value="3:4">3:4</option>
                <option value="1:1">1:1</option>
                <option value="4:3">4:3</option>
                <option value="9:16">9:16</option>
              </select>
            </label>
          </div>
          <p class="privacy-note">真人参考图和身体围度默认只用于本次生成</p>
          <p v-if="uploadError" class="tryon-error" role="alert">{{ uploadError }}</p>
          <p v-if="submitError" class="tryon-error" role="alert">{{ submitError }}</p>
          <p data-testid="virtual-try-on-credit-estimate" class="credit-estimate" :class="{ warning: estimate?.enough === false }">
            {{ creditEstimateText }}
          </p>
          <div class="action-row">
            <button
              type="button"
              data-testid="virtual-try-on-estimate"
              class="secondary-action"
              :disabled="!canSubmit || estimating"
              @click="estimateCredits"
            >
              {{ estimating ? '预估中...' : '预估点数' }}
            </button>
            <button
              type="button"
              data-testid="virtual-try-on-submit"
              class="primary-action"
              :disabled="!canSubmit"
              @click="submitGeneration"
            >
              {{ submitting ? '生成中...' : '开始试衣' }}
            </button>
          </div>
        </section>
      </form>
    </section>

    <aside class="tryon-preview">
      <section class="preview-panel">
        <h2>预览与结果</h2>
        <div v-if="loading" class="preview-placeholder">工作台加载中...</div>
        <div v-else-if="generationInProgress" data-testid="virtual-try-on-progress" class="preview-progress">
          <div class="loading-ring" aria-hidden="true"></div>
          <strong>{{ generationStatusText }}</strong>
          <p>请保持页面打开，生成完成后图片会自动显示在这里。</p>
          <div class="progress-track" aria-hidden="true">
            <span></span>
          </div>
        </div>
        <div v-else-if="result" data-testid="virtual-try-on-result" class="result-card">
          <div class="result-image-frame">
            <img :src="result.preview_url" alt="建模试衣生成结果" />
          </div>
          <div class="result-actions">
            <a data-testid="virtual-try-on-download" :href="result.download_url || result.preview_url" class="secondary-action">
              <Download :size="16" />
              下载
            </a>
            <button type="button" data-testid="virtual-try-on-save" class="secondary-action" @click="openResultInWorks">
              已保存，查看作品库
            </button>
            <button type="button" data-testid="virtual-try-on-use-reference" class="secondary-action" @click="continueWithResultReference">
              作为参考图继续创作
            </button>
            <button type="button" class="secondary-action" :disabled="generationInProgress" @click="submitGeneration">
              重新生成
            </button>
          </div>
        </div>
        <div v-else data-testid="virtual-try-on-empty" class="preview-placeholder">
          <Shirt :size="40" />
          <p>暂无结果，请完善左侧信息并点击生成</p>
        </div>
        <div v-if="task && !result" class="task-status">
          <strong>任务状态</strong>
          <span>{{ task.status || 'queued' }}</span>
          <p v-if="task.failure_message">{{ task.failure_message }}</p>
        </div>
      </section>

      <section class="history-panel">
        <h2>历史结果</h2>
        <div v-if="history.length === 0" class="history-empty">本次暂无生成结果</div>
        <button
          v-for="item in history"
          :key="item.work_id || item.id"
          type="button"
          class="history-item"
          @click="result = item"
        >
          <img v-if="item.preview_url" :src="item.preview_url" alt="" />
          <span>{{ item.prompt || '建模试衣结果' }}</span>
        </button>
      </section>
    </aside>
  </main>
</template>

<style scoped>
.virtual-try-on-workspace {
  --tryon-page-text: #15202b;
  --tryon-page-muted: #4b5563;
  --tryon-page-subtle: #6b7280;
  --tryon-panel-bg: #ffffff;
  --tryon-panel-border: #e5e7eb;
  --tryon-input-bg: #ffffff;
  --tryon-input-border: #d1d5db;
  --tryon-upload-bg: #f8fafc;
  --tryon-upload-hover-bg: #eef6ff;
  --tryon-accent: #2563eb;
  --tryon-accent-strong: #1d4ed8;
  --tryon-accent-soft: #eff6ff;
  --tryon-accent-text: #ffffff;
  --tryon-danger: #b91c1c;
  --tryon-danger-soft: #fef2f2;
  --tryon-result-bg: #f8fafc;
  --tryon-shadow: 0 18px 46px rgba(15, 23, 42, 0.08);
  display: grid;
  grid-template-columns: minmax(0, 1.15fr) minmax(320px, 0.85fr);
  gap: 24px;
  width: min(1280px, calc(100vw - 32px));
  margin: 0 auto;
  padding: 28px 0 48px;
  color: var(--tryon-page-text);
}

:global(.workspace-with-sidebar.user-light-shell .virtual-try-on-workspace) {
  --tryon-page-text: #15202b;
  --tryon-page-muted: #4b5563;
  --tryon-page-subtle: #6b7280;
  --tryon-panel-bg: rgba(255, 255, 255, 0.95);
  --tryon-panel-border: rgba(203, 213, 225, 0.88);
  --tryon-input-bg: #ffffff;
  --tryon-input-border: #cbd5e1;
  --tryon-upload-bg: #f8fafc;
  --tryon-upload-hover-bg: #eef6ff;
  --tryon-accent: #2563eb;
  --tryon-accent-strong: #1d4ed8;
  --tryon-accent-soft: #eff6ff;
  --tryon-accent-text: #ffffff;
  --tryon-danger: #b91c1c;
  --tryon-danger-soft: #fef2f2;
  --tryon-result-bg: #f8fafc;
  --tryon-shadow: 0 18px 46px rgba(15, 23, 42, 0.08);
}

:global(.workspace-with-sidebar.user-dark-shell .virtual-try-on-workspace) {
  --tryon-page-text: #e5eef8;
  --tryon-page-muted: #a7b7ca;
  --tryon-page-subtle: #8da1b8;
  --tryon-panel-bg: rgba(10, 18, 32, 0.78);
  --tryon-panel-border: rgba(148, 163, 184, 0.22);
  --tryon-input-bg: rgba(15, 23, 42, 0.74);
  --tryon-input-border: rgba(148, 163, 184, 0.28);
  --tryon-upload-bg: rgba(13, 25, 43, 0.78);
  --tryon-upload-hover-bg: rgba(14, 165, 233, 0.12);
  --tryon-accent: #38bdf8;
  --tryon-accent-strong: #7dd3fc;
  --tryon-accent-soft: rgba(56, 189, 248, 0.16);
  --tryon-accent-text: #082f49;
  --tryon-danger: #fca5a5;
  --tryon-danger-soft: rgba(127, 29, 29, 0.25);
  --tryon-result-bg: rgba(8, 15, 26, 0.58);
  --tryon-shadow: 0 20px 60px rgba(0, 0, 0, 0.32);
}

.tryon-header {
  margin-bottom: 18px;
}

.tryon-kicker {
  margin: 0 0 6px;
  color: var(--tryon-accent);
  font-size: 13px;
  font-weight: 700;
}

.tryon-header h1 {
  margin: 0;
  font-size: 32px;
  line-height: 1.15;
  letter-spacing: 0;
  color: var(--tryon-page-text);
}

.tryon-header p {
  margin: 8px 0 0;
  color: var(--tryon-page-muted);
}

.tryon-form,
.tryon-preview {
  display: grid;
  gap: 16px;
}

.tryon-section {
  border: 1px solid var(--tryon-panel-border);
  border-radius: 8px;
  background: var(--tryon-panel-bg);
  box-shadow: var(--tryon-shadow);
  padding: 18px;
}

.section-heading {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  margin-bottom: 14px;
}

.section-heading h2,
.preview-panel h2,
.history-panel h2 {
  margin: 0;
  font-size: 18px;
  line-height: 1.3;
  letter-spacing: 0;
  color: var(--tryon-page-text);
}

.section-heading p {
  margin: 4px 0 0;
  color: var(--tryon-page-muted);
  font-size: 13px;
}

.section-heading svg {
  color: var(--tryon-accent);
}

.tryon-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  margin: 12px 0;
}

.tryon-grid label {
  display: grid;
  gap: 6px;
  min-width: 0;
}

.tryon-grid span {
  font-size: 13px;
  color: var(--tryon-page-muted);
}

.field-label {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
}

.field-label small {
  color: var(--tryon-page-subtle);
  font-size: 12px;
  font-weight: 500;
}

.tryon-grid input {
  width: 100%;
  min-height: 40px;
  border: 1px solid var(--tryon-input-border);
  border-radius: 6px;
  background: var(--tryon-input-bg);
  color: var(--tryon-page-text);
  padding: 9px 10px;
  font: inherit;
}

.tryon-grid input.invalid {
  border-color: var(--tryon-danger);
  background: var(--tryon-danger-soft);
}

.field-error {
  margin: -2px 0 0;
  color: var(--tryon-danger);
  font-size: 12px;
  line-height: 1.4;
}

.tryon-grid select,
.tryon-grid textarea {
  width: 100%;
  min-height: 40px;
  border: 1px solid var(--tryon-input-border);
  border-radius: 6px;
  background: var(--tryon-input-bg);
  color: var(--tryon-page-text);
  padding: 9px 10px;
  font: inherit;
}

.tryon-grid input::placeholder,
.tryon-grid textarea::placeholder {
  color: var(--tryon-page-subtle);
}

.tryon-grid input:focus,
.tryon-grid select:focus,
.tryon-grid textarea:focus {
  border-color: var(--tryon-accent);
  box-shadow: 0 0 0 3px var(--tryon-accent-soft);
  outline: none;
}

.tryon-grid textarea {
  resize: vertical;
}

.wide-field {
  grid-column: 1 / -1;
}

.scene-tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 10px;
}

.scene-tabs button {
  border: 1px solid var(--tryon-panel-border);
  border-radius: 999px;
  background: var(--tryon-upload-bg);
  color: var(--tryon-page-muted);
  padding: 8px 12px;
  font: inherit;
  cursor: pointer;
}

.scene-tabs button:hover {
  border-color: var(--tryon-accent);
  color: var(--tryon-page-text);
}

.scene-tabs button.active {
  border-color: var(--tryon-accent);
  color: var(--tryon-accent-strong);
  background: var(--tryon-accent-soft);
}

.privacy-note,
.credit-estimate,
.tryon-error {
  margin: 10px 0 0;
  font-size: 14px;
}

.privacy-note {
  color: var(--tryon-page-muted);
}

.credit-estimate {
  color: var(--tryon-page-text);
  font-weight: 600;
}

.credit-estimate.warning,
.tryon-error {
  color: var(--tryon-danger);
}

.action-row,
.result-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-top: 14px;
}

.primary-action,
.secondary-action {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-height: 40px;
  border-radius: 6px;
  padding: 0 14px;
  font: inherit;
  text-decoration: none;
  cursor: pointer;
}

.primary-action {
  border: 1px solid var(--tryon-accent);
  background: var(--tryon-accent);
  color: var(--tryon-accent-text);
}

.secondary-action {
  border: 1px solid var(--tryon-panel-border);
  background: var(--tryon-input-bg);
  color: var(--tryon-page-text);
}

.primary-action:disabled,
.secondary-action:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.preview-panel {
  border: 1px solid var(--tryon-panel-border);
  border-radius: 8px;
  background: var(--tryon-panel-bg);
  box-shadow: var(--tryon-shadow);
  padding: 18px;
}

.history-panel {
  display: grid;
  gap: 10px;
  border: 1px solid var(--tryon-panel-border);
  border-radius: 8px;
  background: var(--tryon-panel-bg);
  box-shadow: var(--tryon-shadow);
  padding: 18px;
}

.preview-placeholder {
  display: grid;
  place-items: center;
  min-height: 360px;
  border: 1px dashed var(--tryon-panel-border);
  border-radius: 8px;
  background: var(--tryon-result-bg);
  color: var(--tryon-page-subtle);
  text-align: center;
  padding: 24px;
}

.preview-placeholder svg {
  color: var(--tryon-accent);
}

.preview-progress {
  display: grid;
  place-items: center;
  min-height: 360px;
  border: 1px dashed var(--tryon-panel-border);
  border-radius: 8px;
  background: var(--tryon-result-bg);
  color: var(--tryon-page-muted);
  text-align: center;
  padding: 28px;
}

.preview-progress strong {
  margin-top: 14px;
  color: var(--tryon-page-text);
  font-size: 16px;
}

.preview-progress p {
  max-width: 320px;
  margin: 8px 0 16px;
  color: var(--tryon-page-muted);
  line-height: 1.6;
}

.loading-ring {
  width: 48px;
  height: 48px;
  border: 3px solid var(--tryon-panel-border);
  border-top-color: var(--tryon-accent);
  border-radius: 50%;
  animation: tryon-spin 0.85s linear infinite;
}

.progress-track {
  width: min(320px, 100%);
  height: 8px;
  overflow: hidden;
  border-radius: 999px;
  background: var(--tryon-input-bg);
  border: 1px solid var(--tryon-panel-border);
}

.progress-track span {
  display: block;
  width: 42%;
  height: 100%;
  border-radius: inherit;
  background: linear-gradient(90deg, transparent, var(--tryon-accent), transparent);
  animation: tryon-progress 1.35s ease-in-out infinite;
}

.result-card {
  display: grid;
  gap: 14px;
}

.result-image-frame {
  overflow: hidden;
  border: 1px solid var(--tryon-panel-border);
  border-radius: 12px;
  background: var(--tryon-result-bg);
  box-shadow: 0 20px 48px rgba(15, 23, 42, 0.22);
}

.result-image-frame img {
  display: block;
  width: 100%;
  aspect-ratio: 3 / 4;
  object-fit: contain;
}

@keyframes tryon-spin {
  to {
    transform: rotate(360deg);
  }
}

@keyframes tryon-progress {
  0% {
    transform: translateX(-120%);
  }

  100% {
    transform: translateX(240%);
  }
}

.task-status {
  margin-top: 14px;
  border-top: 1px solid var(--tryon-panel-border);
  padding-top: 12px;
}

.task-status span {
  margin-left: 8px;
  color: var(--tryon-page-muted);
}

.history-empty {
  color: var(--tryon-page-subtle);
  font-size: 14px;
}

.history-item {
  display: grid;
  grid-template-columns: 64px minmax(0, 1fr);
  align-items: center;
  gap: 10px;
  border: 1px solid var(--tryon-panel-border);
  border-radius: 8px;
  background: var(--tryon-panel-bg);
  color: inherit;
  padding: 8px;
  text-align: left;
  cursor: pointer;
}

.history-item img {
  width: 64px;
  height: 64px;
  border-radius: 6px;
  object-fit: cover;
}

.history-item span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

:deep(.image-upload-zone) {
  border: 1px dashed var(--tryon-panel-border);
  border-radius: 8px;
  background: var(--tryon-upload-bg);
  color: var(--tryon-page-muted);
}

:deep(.image-upload-zone:hover:not(.disabled):not(.uploading)),
:deep(.image-upload-zone.dragging) {
  border-color: var(--tryon-accent);
  background: var(--tryon-upload-hover-bg);
}

:deep(.image-upload-zone.uploading) {
  border-color: var(--tryon-accent);
  background: var(--tryon-accent-soft);
}

:deep(.upload-icon) {
  color: var(--tryon-accent);
}

:deep(.upload-title) {
  color: var(--tryon-page-text);
  font-weight: 600;
}

:deep(.upload-hint),
:deep(.upload-hint-secondary),
:deep(.image-preview-name),
:deep(.upload-status) {
  color: var(--tryon-page-muted);
}

:deep(.image-preview-item),
:deep(.add-more-button) {
  border-color: var(--tryon-panel-border);
  background: var(--tryon-input-bg);
  color: var(--tryon-page-text);
}

:deep(.preview-image) {
  background: var(--tryon-result-bg);
}

:deep(.remove-button) {
  background: var(--tryon-danger);
  color: #ffffff;
}

:deep(.upload-status-indicator span) {
  background: var(--tryon-accent);
}

@media (max-width: 900px) {
  .virtual-try-on-workspace {
    grid-template-columns: 1fr;
    width: min(100% - 24px, 680px);
    padding-top: 18px;
  }

}

@media (max-width: 560px) {
  .tryon-grid {
    grid-template-columns: 1fr;
  }

  .tryon-header h1 {
    font-size: 26px;
  }

  .action-row,
  .result-actions {
    flex-direction: column;
  }
}
</style>
