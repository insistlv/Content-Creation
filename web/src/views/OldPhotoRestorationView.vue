<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  CheckCircle2,
  ChevronsLeftRight,
  Clock,
  Download,
  Expand,
  Hand,
  HelpCircle,
  ImageUp,
  RotateCcw,
  Save,
  Search,
  ShieldCheck,
  Sparkles,
  Upload,
  X
} from 'lucide-vue-next'

import familyPortrait from '../image/old-photo-family.png'
import { api } from '../api/client.js'
import { applyAvailableCredits, loadCurrentUser, refreshCurrentUser } from '../stores/session.js'
import ClickSelect from '../components/ClickSelect.vue'
import SoftPanel from '../components/SoftPanel.vue'

const maxUploadBytes = 50 * 1024 * 1024
const acceptedImageTypes = new Set(['image/jpeg', 'image/png', 'image/webp'])

const router = useRouter()
const uploadInput = ref(null)
const selectedFileName = ref('家庭合影修复')
const previewUrl = ref('')
const uploadedAsset = ref(null)
const restoredResult = ref(null)
const historyItems = ref([])
const historyLoading = ref(false)
const historyError = ref('')
const me = ref(null)
const restoreMode = ref('smart')
const restoreStrength = ref(80)
const colorLevel = ref(70)
const sharpnessLevel = ref(60)
const faceEnhance = ref(true)
const detailPreserve = ref(true)
const noiseSuppression = ref('standard')
const advancedOpen = ref(false)
const comparisonMode = ref('split')
const comparePosition = ref(50)
const zoomLevel = ref(100)
const panMode = ref(false)
const fullscreenOpen = ref(false)
const dragActive = ref(false)
const panOffset = ref({ x: 0, y: 0 })
const panStart = ref(null)
const uploading = ref(false)
const submitting = ref(false)
const uploadError = ref('')
const taskError = ref('')
const statusMessage = ref('')
const task = ref(null)
const failedTask = ref(null)
const pollFailureCount = ref(0)
let pollTimer = null
const restoreModeOptions = [
  { value: 'smart', label: '智能修复（推荐）' },
  { value: 'scratch', label: '划痕修复' },
  { value: 'colorize', label: '黑白上色' }
]
const noiseSuppressionOptions = [
  { value: 'off', label: '关闭' },
  { value: 'standard', label: '标准' },
  { value: 'strong', label: '强' }
]

const sourcePhoto = computed(() => uploadedAsset.value?.preview_url || previewUrl.value || familyPortrait)
const restoredPhoto = computed(() => restoredResult.value?.preview_url || sourcePhoto.value)
const canStart = computed(() => !!uploadedAsset.value?.id && !uploading.value && !submitting.value)
const canDownload = computed(() => !!restoredResult.value?.download_url)
const availableCredits = computed(() => me.value?.available_credits ?? task.value?.available_credits ?? null)
const availableCreditCount = computed(() => Number(availableCredits.value ?? 0))
const restorationCreditCost = computed(() => 2)
const canRetryGeneration = computed(() => !!failedTask.value && canStart.value)
const zoomStyle = computed(() => ({
  transform: `translate(${panOffset.value.x}px, ${panOffset.value.y}px) scale(${zoomLevel.value / 100})`
}))

const uploadedItems = computed(() => {
  if (!uploadedAsset.value?.id && !previewUrl.value) return []
  return [
    {
      id: 'active',
      title: selectedFileName.value,
      src: sourcePhoto.value
    }
  ]
})

function chooseFile() {
  uploadInput.value?.click()
}

async function loadSession() {
  try {
    me.value = await loadCurrentUser({ force: true })
  } catch {
    me.value = null
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

function validatePhotoFile(file) {
  if (!file) return '请选择一张需要修复的照片'
  if (!acceptedImageTypes.has(file.type)) {
    return '仅支持 JPG、PNG、WEBP 图片'
  }
  if (file.size > maxUploadBytes) {
    return '单张图片不能超过 50MB'
  }
  return ''
}

function resetViewState() {
  restoredResult.value = null
  failedTask.value = null
  task.value = null
  taskError.value = ''
  pollFailureCount.value = 0
  stopPolling()
  resetStageView()
}

function resetStageView() {
  zoomLevel.value = 100
  panMode.value = false
  panOffset.value = { x: 0, y: 0 }
  panStart.value = null
  comparePosition.value = 50
}

async function uploadPhotoFile(file) {
  const validationMessage = validatePhotoFile(file)
  if (validationMessage) {
    uploadError.value = validationMessage
    statusMessage.value = ''
    return
  }

  if (previewUrl.value) {
    URL.revokeObjectURL(previewUrl.value)
  }

  previewUrl.value = URL.createObjectURL(file)
  selectedFileName.value = file.name.replace(/\.[^.]+$/, '') || '已上传照片'
  uploadError.value = ''
  taskError.value = ''
  resetViewState()
  statusMessage.value = '正在上传照片...'
  uploading.value = true

  try {
    uploadedAsset.value = await api.uploadReferenceAsset(file)
    if (uploadedAsset.value?.original_filename) {
      selectedFileName.value = uploadedAsset.value.original_filename.replace(/\.[^.]+$/, '') || selectedFileName.value
    }
    statusMessage.value = '照片已上传，可以开始修复。'
  } catch (error) {
    uploadedAsset.value = null
    uploadError.value = error.message || '照片上传失败'
    statusMessage.value = ''
  } finally {
    uploading.value = false
  }
}

async function handleFileChange(event) {
  const [file] = Array.from(event.target.files ?? [])
  if (!file) return
  await uploadPhotoFile(file)
  event.target.value = ''
}

function handleDragEnter() {
  dragActive.value = true
}

function handleDragLeave(event) {
  if (!event.currentTarget?.contains(event.relatedTarget)) {
    dragActive.value = false
  }
}

async function handleDrop(event) {
  dragActive.value = false
  const [file] = Array.from(event.dataTransfer?.files ?? [])
  if (!file) return
  await uploadPhotoFile(file)
}

function removeUploadedPhoto() {
  if (previewUrl.value) {
    URL.revokeObjectURL(previewUrl.value)
  }
  previewUrl.value = ''
  uploadedAsset.value = null
  selectedFileName.value = '家庭合影修复'
  uploadError.value = ''
  statusMessage.value = ''
  resetViewState()
}

function restorationPrompt() {
  const modeText = {
    smart: '智能修复，修补破损、划痕、折痕和褪色区域',
    scratch: '重点去除划痕、裂纹、污渍和纸张破损',
    colorize: '在保持年代质感的前提下为黑白照片自然上色'
  }[restoreMode.value]
  const noiseText = {
    off: '不额外进行噪点抑制。',
    standard: '标准抑制照片噪点，保留自然胶片颗粒。',
    strong: '强力抑制照片噪点，尽量减少颗粒、色块和扫描噪声。'
  }[noiseSuppression.value]

  return [
    '老照片修复。',
    modeText,
    `修复强度 ${restoreStrength.value}%，上色程度 ${colorLevel.value}%，锐化程度 ${sharpnessLevel.value}%。`,
    noiseText,
    faceEnhance.value ? '增强人物面部清晰度和五官细节。' : '不要额外强化面部。',
    detailPreserve.value ? '保留原始照片纹理、服饰细节和年代感。' : '优先输出干净平滑的修复结果。',
    '保持人物身份、姿态、构图和背景关系不变，输出高清修复版。'
  ].join(' ')
}

function restorationNegativePrompt() {
  return '不要改变人物身份、年龄、表情、发型、服装、姿态或人数，不要生成新人物，不要改变构图，不要过度磨皮，不要卡通化，不要现代化服饰，不要文字水印。'
}

function stopPolling() {
  if (pollTimer !== null) {
    window.clearTimeout(pollTimer)
    pollTimer = null
  }
}

function failureMessage(payload, fallback = '修复失败，请稍后再试') {
  return payload?.error?.message || payload?.message || fallback
}

async function pollRestoration(generationId) {
  if (!generationId) return

  try {
    const payload = await api.getImageGeneration(generationId)
    pollFailureCount.value = 0
    task.value = payload
    syncAvailableCredits(payload)

    if (payload.status === 'succeeded') {
      restoredResult.value = payload
      statusMessage.value = '修复完成，结果已保存到作品库。'
      submitting.value = false
      stopPolling()
      if (payload.available_credits === undefined) {
        void refreshSessionCredits()
      }
      void loadHistory()
      return
    }

    if (payload.status === 'failed') {
      failedTask.value = payload
      taskError.value = failureMessage(payload)
      statusMessage.value = ''
      submitting.value = false
      stopPolling()
      if (payload.available_credits === undefined) {
        void refreshSessionCredits()
      }
      return
    }

    statusMessage.value = payload.stage === 'persisting_result' ? '正在保存修复结果...' : '正在修复照片...'
    pollTimer = window.setTimeout(() => {
      void pollRestoration(generationId)
    }, 2000)
  } catch (error) {
    pollFailureCount.value += 1
    if (pollFailureCount.value >= 2) {
      taskError.value = error.message || '任务状态查询失败，请稍后重试'
    }
    pollTimer = window.setTimeout(() => {
      void pollRestoration(generationId)
    }, 2000)
  }
}

async function startRestoration() {
  if (!uploadedAsset.value?.id) {
    taskError.value = '请先上传一张需要修复的老照片。'
    return
  }
  if (availableCredits.value !== null && availableCreditCount.value < restorationCreditCost.value) {
    taskError.value = `点数不足，本次预计消耗 ${restorationCreditCost.value} 点`
    return
  }

  taskError.value = ''
  statusMessage.value = '修复任务已提交，正在排队处理...'
  restoredResult.value = null
  failedTask.value = null
  pollFailureCount.value = 0
  submitting.value = true

  try {
    const payload = await api.createImageGeneration({
      prompt: restorationPrompt(),
      negative_prompt: restorationNegativePrompt(),
      aspect_ratio: '1:1',
      quality: 'high',
      style_preset: '老照片修复',
      tool_mode: 'generate',
      style_strength: restoreStrength.value,
      reference_weight: colorLevel.value,
      reference_asset_ids: [uploadedAsset.value.id]
    })
    task.value = payload
    syncAvailableCredits(payload)
    await pollRestoration(payload.generation_id)
  } catch (error) {
    taskError.value = error.message || '修复任务提交失败'
    statusMessage.value = ''
    submitting.value = false
  }
}

function retryGeneration() {
  if (!canRetryGeneration.value) return
  void startRestoration()
}

function resetSettings() {
  restoreMode.value = 'smart'
  restoreStrength.value = 80
  colorLevel.value = 70
  sharpnessLevel.value = 60
  faceEnhance.value = true
  detailPreserve.value = true
  noiseSuppression.value = 'standard'
  resetStageView()
  taskError.value = ''
  uploadError.value = ''
  statusMessage.value = uploadedAsset.value?.id ? '参数已重置，可以重新开始修复。' : ''
}

function downloadRestoredImage() {
  if (!restoredResult.value?.download_url) {
    taskError.value = '修复完成后才能下载高清图。'
    return
  }
  window.open(restoredResult.value.download_url, '_blank')
}

function saveToWorks() {
  if (!restoredResult.value?.work_id) {
    taskError.value = '修复完成后会自动保存到作品库。'
    return
  }
  router.push('/works')
}

function clampZoom(value) {
  return Math.min(200, Math.max(50, value))
}

function zoomIn() {
  zoomLevel.value = clampZoom(zoomLevel.value + 25)
}

function zoomOut() {
  zoomLevel.value = clampZoom(zoomLevel.value - 25)
}

function togglePanMode() {
  panMode.value = !panMode.value
}

function openFullscreen() {
  fullscreenOpen.value = true
}

function closeFullscreen() {
  fullscreenOpen.value = false
}

function startPan(event) {
  if (!panMode.value) return
  panStart.value = {
    pointerId: event.pointerId,
    clientX: event.clientX,
    clientY: event.clientY,
    offsetX: panOffset.value.x,
    offsetY: panOffset.value.y
  }
  event.currentTarget?.setPointerCapture?.(event.pointerId)
}

function movePan(event) {
  if (!panMode.value || !panStart.value) return
  panOffset.value = {
    x: panStart.value.offsetX + event.clientX - panStart.value.clientX,
    y: panStart.value.offsetY + event.clientY - panStart.value.clientY
  }
}

function endPan(event) {
  if (panStart.value?.pointerId === event.pointerId) {
    event.currentTarget?.releasePointerCapture?.(event.pointerId)
  }
  panStart.value = null
}

function isOldPhotoWork(work) {
  const stylePreset = work.style_preset ?? work.stylePreset ?? ''
  const prompt = work.prompt ?? work.prompt_summary ?? ''
  return stylePreset === '老照片修复' || prompt.includes('老照片修复')
}

function formatHistoryTime(value) {
  if (!value) return '刚刚'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  })
}

function mapHistoryItem(work) {
  const prompt = work.prompt ?? work.prompt_summary ?? '老照片修复作品'
  return {
    id: work.work_id ?? work.id ?? prompt,
    title: prompt,
    meta: [work.aspect_ratio, work.model].filter(Boolean).join(' | ') || '老照片修复',
    time: formatHistoryTime(work.created_at),
    src: work.preview_url || work.download_url || familyPortrait,
    status: work.status === 'failed' ? '失败' : '完成'
  }
}

async function loadHistory() {
  historyLoading.value = true
  historyError.value = ''
  try {
    const payload = await api.listWorks({ category: 'image', page: 1, page_size: 3 })
    historyItems.value = (payload.items ?? []).filter(isOldPhotoWork).slice(0, 3).map(mapHistoryItem)
  } catch (error) {
    historyItems.value = []
    historyError.value = error.message || '修复历史读取失败'
  } finally {
    historyLoading.value = false
  }
}

onMounted(() => {
  void loadSession()
  void loadHistory()
})

onBeforeUnmount(() => {
  stopPolling()
  if (previewUrl.value) {
    URL.revokeObjectURL(previewUrl.value)
  }
})
</script>

<template>
  <section class="old-photo-page" aria-label="老照片修复工作台">
    <SoftPanel class="old-photo-upload-panel" tone="default" data-testid="old-photo-upload-panel">
      <div class="old-photo-panel-title">
        <h2>上传你的老照片</h2>
        <HelpCircle :size="18" />
      </div>

      <button
        class="old-photo-dropzone"
        :class="{ 'old-photo-dropzone-active': dragActive }"
        data-testid="old-photo-dropzone"
        type="button"
        @click="chooseFile"
        @dragenter.prevent="handleDragEnter"
        @dragover.prevent="handleDragEnter"
        @dragleave.prevent="handleDragLeave"
        @drop.prevent="handleDrop"
      >
        <span class="old-photo-upload-orb">
          <Upload :size="28" />
        </span>
        <strong>点击上传或拖拽图片到此处</strong>
        <small>支持 JPG / PNG / WEBP 格式，单张不超过 50MB</small>
      </button>

      <input
        ref="uploadInput"
        class="sr-only"
        type="file"
        accept="image/jpeg,image/png,image/webp"
        @change="handleFileChange"
      />

      <div class="old-photo-uploaded">
        <h3>已上传（{{ uploadedItems.length }}/1）</h3>
        <div
          v-for="item in uploadedItems"
          :key="item.id"
          class="old-photo-thumb active"
        >
          <img data-testid="old-photo-active-thumb" :src="item.src" :alt="item.title" />
          <CheckCircle2 class="old-photo-thumb-check" :size="20" />
          <button data-testid="old-photo-remove" type="button" aria-label="移除图片" @click="removeUploadedPhoto">
            <X :size="18" />
          </button>
        </div>
        <p v-if="uploadedItems.length === 0" class="old-photo-upload-empty">尚未上传照片</p>
      </div>

      <button class="old-photo-change-button" type="button" @click="chooseFile">
        <ImageUp :size="18" />
        <span>更换图片</span>
      </button>

      <p v-if="uploadError" class="old-photo-inline-error">{{ uploadError }}</p>
    </SoftPanel>

    <SoftPanel class="old-photo-comparison-panel" tone="default" data-testid="old-photo-comparison-panel">
      <div class="old-photo-comparison-head">
        <h2>修复效果对比</h2>
        <div class="old-photo-mode-tabs" aria-label="对比方式">
          <button
            data-testid="old-photo-mode-split"
            type="button"
            :class="{ active: comparisonMode === 'split' }"
            @click="comparisonMode = 'split'"
          >
            <ChevronsLeftRight :size="17" />
            <span>左右对比</span>
          </button>
          <button
            data-testid="old-photo-mode-slide"
            type="button"
            :class="{ active: comparisonMode === 'slide' }"
            @click="comparisonMode = 'slide'"
          >
            <Expand :size="17" />
            <span>滑动对比</span>
          </button>
        </div>
      </div>

      <div
        v-if="comparisonMode === 'split'"
        class="old-photo-stage"
        :class="{ 'old-photo-stage-pan': panMode }"
        data-testid="old-photo-split-stage"
        @pointerdown="startPan"
        @pointermove="movePan"
        @pointerup="endPan"
        @pointercancel="endPan"
      >
        <div class="old-photo-pane old-photo-before">
          <img :style="zoomStyle" :src="sourcePhoto" alt="修复前老照片" />
          <span class="old-photo-badge">修复前</span>
        </div>
        <div class="old-photo-pane old-photo-after">
          <img data-testid="old-photo-restored-image" :style="zoomStyle" :src="restoredPhoto" alt="修复后老照片" />
          <span class="old-photo-badge">修复后</span>
        </div>
        <div class="old-photo-divider">
          <span>
            <ChevronLeft :size="21" />
            <ChevronRight :size="21" />
          </span>
        </div>
      </div>
      <div
        v-else
        class="old-photo-stage old-photo-slide-stage"
        :class="{ 'old-photo-stage-pan': panMode }"
        data-testid="old-photo-slide-stage"
        @pointerdown="startPan"
        @pointermove="movePan"
        @pointerup="endPan"
        @pointercancel="endPan"
      >
        <img class="old-photo-slide-image old-photo-slide-before" :style="zoomStyle" :src="sourcePhoto" alt="修复前老照片" />
        <div
          class="old-photo-slide-after"
          data-testid="old-photo-slide-after"
          :style="{ '--compare-position': `${comparePosition}%`, clipPath: `inset(0 ${100 - comparePosition}% 0 0)` }"
        >
          <img data-testid="old-photo-restored-image" :style="zoomStyle" :src="restoredPhoto" alt="修复后老照片" />
        </div>
        <input
          v-model.number="comparePosition"
          class="old-photo-compare-slider"
          data-testid="old-photo-compare-slider"
          type="range"
          min="0"
          max="100"
          aria-label="滑动对比位置"
        />
        <div class="old-photo-divider" :style="{ left: `${comparePosition}%` }">
          <span>
            <ChevronLeft :size="21" />
            <ChevronRight :size="21" />
          </span>
        </div>
      </div>

      <div class="old-photo-stage-tools">
        <div class="old-photo-view-actions">
          <button data-testid="old-photo-zoom-in" type="button" aria-label="放大" @click="zoomIn">
            <Search :size="22" />
          </button>
          <button data-testid="old-photo-zoom-out" type="button" aria-label="缩小" @click="zoomOut">
            <Search :size="18" />
          </button>
          <button data-testid="old-photo-pan-toggle" type="button" aria-label="拖动画布" :class="{ active: panMode }" @click="togglePanMode">
            <Hand :size="20" />
          </button>
          <button data-testid="old-photo-fullscreen" type="button" aria-label="全屏查看" @click="openFullscreen">
            <Expand :size="20" />
          </button>
        </div>
        <div class="old-photo-zoom-control">
          <button type="button" aria-label="缩小视图" @click="zoomOut">
            <ChevronLeft :size="18" />
          </button>
          <strong data-testid="old-photo-zoom-label">{{ zoomLevel }}%</strong>
          <button type="button" aria-label="放大视图" @click="zoomIn">
            <ChevronRight :size="18" />
          </button>
        </div>
      </div>
    </SoftPanel>

    <div class="old-photo-side-stack">
      <SoftPanel class="old-photo-settings-panel" tone="default" data-testid="old-photo-settings-panel">
        <div class="old-photo-panel-title">
          <h2>修复参数设置</h2>
          <HelpCircle :size="18" />
        </div>

        <label class="old-photo-field">
          <span>修复模式</span>
          <ClickSelect v-model="restoreMode" :options="restoreModeOptions" data-testid="old-photo-mode" aria-label="修复模式" />
        </label>

        <label class="old-photo-range-row">
          <span>修复强度</span>
          <input v-model.number="restoreStrength" data-testid="old-photo-strength" type="range" min="0" max="100" />
          <strong>{{ restoreStrength }}%</strong>
        </label>

        <label class="old-photo-range-row">
          <span>上色程度</span>
          <input v-model.number="colorLevel" data-testid="old-photo-color" type="range" min="0" max="100" />
          <strong>{{ colorLevel }}%</strong>
        </label>

        <label class="old-photo-range-row">
          <span>锐化程度</span>
          <input v-model.number="sharpnessLevel" data-testid="old-photo-sharpness" type="range" min="0" max="100" />
          <strong>{{ sharpnessLevel }}%</strong>
        </label>

        <label class="old-photo-toggle-row">
          <span>面部增强</span>
          <input v-model="faceEnhance" data-testid="old-photo-face-enhance" type="checkbox" />
        </label>

        <label class="old-photo-toggle-row">
          <span>细节保留</span>
          <input v-model="detailPreserve" data-testid="old-photo-detail-preserve" type="checkbox" />
        </label>

        <button class="old-photo-advanced" data-testid="old-photo-advanced-toggle" type="button" @click="advancedOpen = !advancedOpen">
          <span>高级设置</span>
          <ChevronDown :size="18" />
        </button>

        <div v-if="advancedOpen" class="old-photo-advanced-body">
          <label class="old-photo-field old-photo-advanced-field">
            <span>噪点抑制</span>
            <ClickSelect v-model="noiseSuppression" :options="noiseSuppressionOptions" data-testid="old-photo-noise-level" aria-label="噪点抑制" />
          </label>
        </div>
      </SoftPanel>

      <SoftPanel class="old-photo-history-panel" tone="default" data-testid="old-photo-history-panel">
        <div class="old-photo-history-head">
          <h2>修复历史</h2>
          <a href="/works">查看全部</a>
        </div>

        <div class="old-photo-history-list">
          <p v-if="historyLoading" class="old-photo-history-empty">正在读取修复历史...</p>
          <p v-else-if="historyError" class="old-photo-history-empty">{{ historyError }}</p>
          <p v-else-if="historyItems.length === 0" class="old-photo-history-empty">暂无修复历史</p>
          <article
            v-else
            v-for="item in historyItems"
            :key="item.id"
            class="old-photo-history-item"
          >
            <img :src="item.src" :alt="item.title" />
            <div>
              <h3>{{ item.title }}</h3>
              <p>{{ item.meta }}</p>
              <time>{{ item.time }}</time>
            </div>
            <span>{{ item.status }}</span>
          </article>
        </div>
      </SoftPanel>
    </div>

    <SoftPanel class="old-photo-bottom-actions" tone="default" data-testid="old-photo-bottom-actions">
      <div class="old-photo-process-info">
        <strong>处理信息</strong>
        <span v-if="availableCredits !== null">剩余点数 {{ availableCredits }}</span>
        <span>预计消耗 {{ restorationCreditCost }} 点</span>
        <span>
          <Clock :size="17" />
          预计耗时 20s
        </span>
        <span>
          <Sparkles :size="17" />
          高清导出
        </span>
        <span>
          <ShieldCheck :size="17" />
          私密处理
        </span>
      </div>

      <div class="old-photo-bottom-buttons">
        <button class="old-photo-footer-button" type="button" @click="resetSettings">
          <RotateCcw :size="17" />
          <span>重置</span>
        </button>
        <button
          class="old-photo-footer-button primary"
          data-testid="old-photo-start"
          type="button"
          :disabled="!canStart"
          @click="startRestoration"
        >
          <span>{{ submitting ? '修复中...' : uploading ? '上传中...' : '开始修复' }}</span>
          <Sparkles :size="17" />
        </button>
        <button
          class="old-photo-footer-button"
          data-testid="old-photo-download"
          type="button"
          :disabled="!canDownload"
          @click="downloadRestoredImage"
        >
          <Download :size="17" />
          <span>下载高清图</span>
        </button>
        <button
          class="old-photo-footer-button"
          data-testid="old-photo-save"
          type="button"
          :disabled="!restoredResult?.work_id"
          @click="saveToWorks"
        >
          <Save :size="17" />
          <span>保存到作品库</span>
        </button>
      </div>

      <p v-if="statusMessage" class="old-photo-footer-status">{{ statusMessage }}</p>
      <p v-if="taskError" class="old-photo-footer-error">{{ taskError }}</p>
      <button
        v-if="canRetryGeneration"
        class="old-photo-footer-button"
        data-testid="old-photo-retry-generation"
        type="button"
        @click="retryGeneration"
      >
        <RotateCcw :size="17" />
        <span>重新生成</span>
      </button>
    </SoftPanel>

    <div
      v-if="fullscreenOpen"
      class="old-photo-fullscreen-modal"
      data-testid="old-photo-fullscreen-modal"
      @click.self="closeFullscreen"
    >
      <div class="old-photo-fullscreen-content">
        <button
          class="old-photo-fullscreen-close"
          data-testid="old-photo-fullscreen-close"
          type="button"
          aria-label="关闭全屏预览"
          @click="closeFullscreen"
        >
          <X :size="20" />
        </button>
        <div
          class="old-photo-stage old-photo-slide-stage old-photo-fullscreen-stage"
          :class="{ 'old-photo-stage-pan': panMode }"
          @pointerdown="startPan"
          @pointermove="movePan"
          @pointerup="endPan"
          @pointercancel="endPan"
        >
          <img class="old-photo-slide-image old-photo-slide-before" :style="zoomStyle" :src="sourcePhoto" alt="修复前老照片" />
          <div class="old-photo-slide-after" :style="{ '--compare-position': `${comparePosition}%`, clipPath: `inset(0 ${100 - comparePosition}% 0 0)` }">
            <img :style="zoomStyle" :src="restoredPhoto" alt="修复后老照片" />
          </div>
          <input
            v-model.number="comparePosition"
            class="old-photo-compare-slider"
            type="range"
            min="0"
            max="100"
            aria-label="全屏滑动对比位置"
          />
          <div class="old-photo-divider" :style="{ left: `${comparePosition}%` }">
            <span>
              <ChevronLeft :size="21" />
              <ChevronRight :size="21" />
            </span>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>
