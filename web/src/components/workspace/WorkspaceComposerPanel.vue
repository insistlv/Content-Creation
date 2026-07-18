<script setup>
import { computed, nextTick, onBeforeUnmount, ref } from 'vue'
import {
  CircleHelp,
  ChevronDown,
  ChevronUp,
  Edit3,
  ImagePlus,
  Languages,
  Plus,
  SlidersHorizontal,
  Sparkles,
  Upload,
  X
} from 'lucide-vue-next'
import AspectRatioSelector from '../AspectRatioSelector.vue'
import ClickSelect from '../ClickSelect.vue'
import PillTag from '../PillTag.vue'

const props = defineProps({
  me: { type: Object, default: null },
  layoutVariant: { type: String, default: 'workspace' },
  requiresAuth: { type: Boolean, default: false },
  displayedModelName: { type: String, required: true },
  workspaceModels: { type: Array, default: () => [] },
  selectedReferenceImages: { type: Array, default: () => [] },
  sourceImageLimit: { type: Number, required: true },
  referenceUploading: { type: Boolean, default: false },
  referenceError: { type: String, default: '' },
  referenceUploadTitle: { type: String, required: true },
  referenceUploadHint: { type: String, required: true },
  isExpandTool: { type: Boolean, default: false },
  expandEdges: { type: Object, default: () => ({}) },
  expandPreviewStyle: { type: Object, default: () => ({}) },
  expandOriginalStyle: { type: Object, default: () => ({}) },
  expandSourcePreview: { type: Object, default: null },
  isMaskSelectionTool: { type: Boolean, default: false },
  eraseSourcePreview: { type: Object, default: null },
  isPrecisionEditTool: { type: Boolean, default: false },
  hasEraseMask: { type: Boolean, default: false },
  eraseMaskRegions: { type: Array, default: () => [] },
  promptLabel: { type: String, required: true },
  promptPlaceholder: { type: String, required: true },
  task: { type: Object, default: null },
  submitting: { type: Boolean, default: false },
  stylePresets: { type: Array, default: () => [] },
  qualityOptions: { type: Array, default: () => [] },
  renderedToolFields: { type: Array, default: () => [] },
  expandShortcutPresets: { type: Array, default: () => [] },
  toolOptions: { type: Object, default: () => ({}) },
  canSubmit: { type: Boolean, default: false },
  currentEstimatedCredits: { type: Number, default: 0 },
  taskError: { type: String, default: '' },
  creditEstimateError: { type: String, default: '' },
  creditEstimateNotice: { type: String, default: '' },
  isEditTool: { type: Boolean, default: false },
  showReferenceStrength: { type: Boolean, default: false },
  hasEditSourceImage: { type: Boolean, default: false },
  effectivePrompt: { type: String, default: '' },
  canRetryGeneration: { type: Boolean, default: false },
  canCancelGeneration: { type: Boolean, default: false },
  cancelGenerationLoading: { type: Boolean, default: false },
  isCancelledTask: { type: Boolean, default: false },
  successMessage: { type: String, default: '' }
})

const selectedModelId = defineModel('selectedModelId', { type: Number, default: null })
const prompt = defineModel('prompt', { type: String, default: '' })
const negativePrompt = defineModel('negativePrompt', { type: String, default: '' })
const stylePreset = defineModel('stylePreset', { type: String, default: '' })
const autoTranslate = defineModel('autoTranslate', { type: Boolean, default: false })
const aspectRatio = defineModel('aspectRatio', { type: String, default: '1:1' })
const quality = defineModel('quality', { type: String, default: 'medium' })
const referenceWeight = defineModel('referenceWeight', { type: Number, default: 75 })
const editInstruction = defineModel('editInstruction', { type: String, default: '' })
const eraseBrushSize = defineModel('eraseBrushSize', { type: Number, default: 34 })
const maskSelectionMode = defineModel('maskSelectionMode', { type: String, default: 'brush' })

const emit = defineEmits([
  'submit',
  'upload-reference',
  'remove-reference',
  'retry-reference-assets',
  'require-auth',
  'open-prompt-optimizer',
  'select-style-preset',
  'apply-expand-preset',
  'set-tool-option',
  'mask-pointer-down',
  'mask-pointer-move',
  'mask-pointer-up',
  'undo-mask',
  'clear-mask',
  'select-mask-mode',
  'retry-generation',
  'cancel-generation'
])

const promptInputRef = ref(null)
const referenceFileInputRef = ref(null)
const referenceExpanded = ref(false)
const referenceDragging = ref(false)
const homeAdvancedToggleRef = ref(null)
const homeAdvancedPanelRef = ref(null)
const homeAdvancedOpen = ref(false)
const homeAdvancedPanelStyle = ref({})
const homeAdvancedTone = ref('light')
const homeAdvancedPanelId = 'workspace-home-advanced-panel'
const isHomeLayout = computed(() => props.layoutVariant === 'home')
const qualitySelectOptions = computed(() => props.qualityOptions.map((item) => ({
  value: item.key,
  label: item.label
})))
const selectedReferenceCount = computed(() => props.selectedReferenceImages.length)
const referenceDisabled = computed(() => props.submitting || props.referenceUploading)
const referenceAtLimit = computed(() => selectedReferenceCount.value >= props.sourceImageLimit)
const canOpenReferencePicker = computed(() => (
  !referenceDisabled.value
  && (!referenceAtLimit.value || props.sourceImageLimit === 1)
))
const referenceAddLabel = computed(() => {
  if (props.sourceImageLimit === 1 && selectedReferenceCount.value > 0) {
    return '替换图片'
  }
  if (referenceAtLimit.value) {
    return `最多 ${props.sourceImageLimit} 张`
  }
  return selectedReferenceCount.value > 0 ? '继续添加' : props.referenceUploadTitle
})

function validReferenceFiles(files) {
  return Array.from(files).filter((file) => (
    file.type === 'image/jpeg' || file.type === 'image/png'
  ))
}

function referenceImageName(image, index) {
  return image?.original_filename || image?.title || `参考图 ${index + 1}`
}

function emitReferenceFiles(files) {
  if (referenceDisabled.value) {
    return
  }
  if (props.requiresAuth) {
    emit('require-auth')
    return
  }

  const validFiles = validReferenceFiles(files)
  if (validFiles.length === 0) {
    return
  }

  const uploadLimit = props.sourceImageLimit === 1
    ? 1
    : Math.max(props.sourceImageLimit - selectedReferenceCount.value, 0)

  validFiles.slice(0, uploadLimit).forEach((file) => {
    emit('upload-reference', file)
  })
}

function openReferenceFileDialog() {
  if (referenceDisabled.value) {
    return
  }
  if (props.requiresAuth) {
    emit('require-auth')
    return
  }
  if (!canOpenReferencePicker.value) {
    return
  }
  referenceFileInputRef.value?.click?.()
}

function handleReferenceFileChange(event) {
  emitReferenceFiles(event.target.files || [])
  event.target.value = ''
}

function handleReferenceDragOver(event) {
  event.preventDefault()
  if (canOpenReferencePicker.value && !props.requiresAuth) {
    referenceDragging.value = true
  }
}

function handleReferenceDragLeave() {
  referenceDragging.value = false
}

function handleReferenceDrop(event) {
  event.preventDefault()
  referenceDragging.value = false
  emitReferenceFiles(event.dataTransfer?.files || [])
}

function removeReferenceImage(image) {
  if (referenceDisabled.value) {
    return
  }
  emit('remove-reference', image)
}

function toggleReferenceExpanded() {
  if (selectedReferenceCount.value === 0) {
    return
  }
  referenceExpanded.value = !referenceExpanded.value
}

function isMobileAdvancedPanel() {
  if (typeof window === 'undefined') {
    return false
  }

  return window.matchMedia?.('(max-width: 720px)').matches ?? window.innerWidth <= 720
}

function updateHomeAdvancedTone() {
  homeAdvancedTone.value = homeAdvancedToggleRef.value?.closest('.user-dark-shell') ? 'dark' : 'light'
}

function updateHomeAdvancedPosition() {
  if (!homeAdvancedToggleRef.value || typeof window === 'undefined') {
    return
  }

  updateHomeAdvancedTone()

  if (isMobileAdvancedPanel()) {
    homeAdvancedPanelStyle.value = {}
    return
  }

  const rect = homeAdvancedToggleRef.value.getBoundingClientRect()
  const viewportWidth = window.innerWidth || document.documentElement.clientWidth || 1024
  const viewportHeight = window.innerHeight || document.documentElement.clientHeight || 768
  const panelWidth = Math.min(420, Math.max(300, viewportWidth - 24))
  const left = Math.min(Math.max(rect.left, 12), Math.max(12, viewportWidth - panelWidth - 12))
  const top = Math.min(Math.max(rect.bottom + 8, 12), Math.max(12, viewportHeight - 120))

  homeAdvancedPanelStyle.value = {
    left: `${Math.round(left)}px`,
    top: `${Math.round(top)}px`,
    width: `${Math.round(panelWidth)}px`,
    maxHeight: `${Math.max(260, Math.round(viewportHeight - top - 12))}px`
  }
}

function closeHomeAdvancedPanel({ restoreFocus = false } = {}) {
  homeAdvancedOpen.value = false
  if (typeof window !== 'undefined') {
    window.removeEventListener('resize', updateHomeAdvancedPosition)
    window.removeEventListener('scroll', updateHomeAdvancedPosition, true)
    document.removeEventListener('pointerdown', handleHomeAdvancedOutsidePointer)
    document.removeEventListener('keydown', handleHomeAdvancedKeydown)
  }

  if (restoreFocus) {
    nextTick(() => homeAdvancedToggleRef.value?.focus?.())
  }
}

function openHomeAdvancedPanel() {
  if (props.submitting || !isHomeLayout.value) {
    return
  }

  homeAdvancedOpen.value = true
  nextTick(() => {
    if (!homeAdvancedOpen.value || !homeAdvancedToggleRef.value || typeof window === 'undefined') {
      return
    }

    updateHomeAdvancedPosition()
    window.addEventListener('resize', updateHomeAdvancedPosition)
    window.addEventListener('scroll', updateHomeAdvancedPosition, true)
    document.addEventListener('pointerdown', handleHomeAdvancedOutsidePointer)
    document.addEventListener('keydown', handleHomeAdvancedKeydown)
  })
}

function toggleHomeAdvancedPanel() {
  if (homeAdvancedOpen.value) {
    closeHomeAdvancedPanel()
    return
  }

  openHomeAdvancedPanel()
}

function handleHomeAdvancedOutsidePointer(event) {
  if (
    homeAdvancedToggleRef.value?.contains(event.target)
    || homeAdvancedPanelRef.value?.contains(event.target)
  ) {
    return
  }

  closeHomeAdvancedPanel()
}

function handleHomeAdvancedKeydown(event) {
  if (event.key !== 'Escape') {
    return
  }

  event.preventDefault()
  closeHomeAdvancedPanel({ restoreFocus: true })
}

onBeforeUnmount(() => {
  closeHomeAdvancedPanel()
})

defineExpose({
  focusPrompt() {
    promptInputRef.value?.focus?.()
  }
})
</script>

<template>
  <aside class="workspace-composer-area imini-composer-area">
    <form
      class="imini-composer-card"
      :class="{ 'imini-composer-card--home': isHomeLayout }"
      data-testid="workspace-composer-form"
      @submit.prevent="$emit('submit')"
    >
      <header class="imini-composer-header">
        <h2>图片生成</h2>
        <span>{{ me?.available_credits ?? 0 }} 点</span>
      </header>

      <label v-if="!isHomeLayout" class="imini-model-card">
        <span class="imini-model-icon">白</span>
        <span>
          <small>模型</small>
          <strong>{{ displayedModelName }}</strong>
        </span>
        <ClickSelect
          v-if="workspaceModels.length > 0"
          v-model="selectedModelId"
          :options="workspaceModels.map((model) => ({ value: model.id, label: model.name }))"
          data-testid="workspace-model-select"
          :disabled="submitting"
          aria-label="模型"
        />
      </label>

      <div
        v-if="taskError"
        class="workspace-generation-failure-notice"
        data-testid="workspace-generation-failure-notice"
        role="alert"
      >
        <div class="workspace-generation-failure-copy">
          <strong>{{ isCancelledTask ? '已取消生成' : '生成失败' }}</strong>
          <span>{{ taskError }}</span>
        </div>
        <button
          v-if="canRetryGeneration"
          class="workspace-generation-failure-action"
          type="button"
          data-testid="workspace-failure-retry-generation"
          @click="$emit('retry-generation')"
        >
          点击重试
        </button>
      </div>

      <div
        class="imini-reference-block"
        :class="{ 'imini-home-reference-pane': isHomeLayout }"
        data-testid="workspace-reference-upload"
      >
        <div
          class="workspace-reference-attachments"
          :class="{
            'workspace-reference-attachments--empty': selectedReferenceCount === 0,
            'workspace-reference-attachments--expanded': referenceExpanded,
            'workspace-reference-attachments--dragging': referenceDragging,
            'workspace-reference-attachments--uploading': referenceUploading,
            'workspace-reference-attachments--disabled': referenceDisabled
          }"
          data-testid="workspace-reference-dropzone"
          :aria-busy="referenceUploading ? 'true' : 'false'"
          :aria-disabled="referenceDisabled ? 'true' : 'false'"
          @dragover="handleReferenceDragOver"
          @dragleave="handleReferenceDragLeave"
          @drop="handleReferenceDrop"
        >
          <input
            ref="referenceFileInputRef"
            data-testid="workspace-reference-file-input"
            type="file"
            accept="image/jpeg,image/png"
            :multiple="sourceImageLimit > 1"
            :disabled="referenceDisabled"
            class="workspace-reference-file-input"
            @change="handleReferenceFileChange"
          />

          <div v-if="selectedReferenceCount === 0" class="workspace-reference-empty">
            <button
              type="button"
              class="workspace-reference-empty-button"
              data-testid="workspace-reference-add"
              :disabled="referenceDisabled"
              @click="openReferenceFileDialog"
            >
              <ImagePlus :size="20" aria-hidden="true" />
              <span>{{ referenceUploadTitle }}</span>
            </button>
            <span class="workspace-reference-empty-hint">{{ referenceUploadHint }}</span>
            <span class="workspace-reference-empty-drop"><Upload :size="14" aria-hidden="true" />拖拽图片到这里</span>
          </div>

          <template v-else>
            <div class="workspace-reference-compact">
              <button
                type="button"
                class="workspace-reference-stack"
                :aria-expanded="referenceExpanded ? 'true' : 'false'"
                @click="toggleReferenceExpanded"
              >
                <span
                  v-for="(image, index) in selectedReferenceImages.slice(0, 3)"
                  :key="image.id || image.work_id || index"
                  class="workspace-reference-stack-thumb"
                  data-testid="workspace-reference-stack-thumb"
                  :style="{ '--stack-index': index }"
                >
                  <img
                    :src="image.preview_url || image.url"
                    :alt="referenceImageName(image, index)"
                  />
                </span>
              </button>
              <div class="workspace-reference-summary">
                <span class="workspace-reference-count" data-testid="workspace-reference-count">
                  已选 {{ selectedReferenceCount }}/{{ sourceImageLimit }}
                </span>
                <strong>{{ referenceImageName(selectedReferenceImages[0], 0) }}</strong>
              </div>
              <button
                type="button"
                class="workspace-reference-icon-button"
                data-testid="workspace-reference-add"
                :disabled="referenceDisabled || (referenceAtLimit && sourceImageLimit !== 1)"
                :title="referenceAddLabel"
                @click="openReferenceFileDialog"
              >
                <Plus v-if="!referenceAtLimit || sourceImageLimit === 1" :size="16" aria-hidden="true" />
                <span>{{ referenceAddLabel }}</span>
              </button>
              <button
                type="button"
                class="workspace-reference-icon-button workspace-reference-toggle"
                data-testid="workspace-reference-toggle"
                :aria-expanded="referenceExpanded ? 'true' : 'false'"
                @click="toggleReferenceExpanded"
              >
                <ChevronUp v-if="referenceExpanded" :size="16" aria-hidden="true" />
                <ChevronDown v-else :size="16" aria-hidden="true" />
                <span>{{ referenceExpanded ? '收起' : '展开' }}</span>
              </button>
            </div>

            <Transition name="workspace-reference-expand">
              <div
                v-if="referenceExpanded"
                class="workspace-reference-grid"
                data-testid="workspace-reference-grid"
              >
                <div
                  v-for="(image, index) in selectedReferenceImages"
                  :key="image.id || image.work_id || index"
                  class="workspace-reference-grid-item"
                  data-testid="workspace-reference-grid-item"
                >
                  <img
                    :src="image.preview_url || image.url"
                    :alt="referenceImageName(image, index)"
                  />
                  <span>{{ referenceImageName(image, index) }}</span>
                  <button
                    type="button"
                    data-testid="workspace-reference-remove"
                    :disabled="referenceDisabled"
                    :aria-label="`移除 ${referenceImageName(image, index)}`"
                    @click="removeReferenceImage(image)"
                  >
                    <X :size="15" aria-hidden="true" />
                  </button>
                </div>
                <button
                  type="button"
                  class="workspace-reference-more"
                  data-testid="workspace-reference-more"
                  :disabled="referenceDisabled || (referenceAtLimit && sourceImageLimit !== 1)"
                  @click="openReferenceFileDialog"
                >
                  <Plus v-if="!referenceAtLimit || sourceImageLimit === 1" :size="18" aria-hidden="true" />
                  <span>{{ referenceAddLabel }}</span>
                </button>
              </div>
            </Transition>
          </template>

          <p
            v-if="referenceUploading"
            class="workspace-reference-upload-status"
            data-testid="workspace-reference-upload-status"
            role="status"
            aria-live="polite"
          >
            上传中...
          </p>
        </div>
        <div v-if="referenceError" class="workspace-inline-error" role="alert">
          <p class="error-message">{{ referenceError }}</p>
          <button type="button" class="secondary-button" @click="$emit('retry-reference-assets')">重试</button>
        </div>
      </div>

      <div v-if="isExpandTool" class="imini-expand-preview" data-testid="workspace-expand-preview">
        <div class="imini-expand-preview-head">
          <strong>目标画布</strong>
          <span>上 {{ expandEdges.top }}% · 下 {{ expandEdges.bottom }}% · 左 {{ expandEdges.left }}% · 右 {{ expandEdges.right }}%</span>
        </div>
        <div class="imini-expand-canvas" :style="expandPreviewStyle">
          <div class="imini-expand-source" :style="expandOriginalStyle">
            <img v-if="expandSourcePreview?.preview_url" :src="expandSourcePreview.preview_url" alt="" />
            <span v-else>源图区域</span>
          </div>
        </div>
      </div>

      <div
        v-if="isMaskSelectionTool && eraseSourcePreview"
        class="imini-erase-mask-panel"
        :data-testid="isPrecisionEditTool ? 'workspace-precision-mask-panel' : 'workspace-erase-mask-panel'"
      >
        <div class="imini-erase-mask-head">
          <strong>圈选区域</strong>
          <span>{{ hasEraseMask ? `${eraseMaskRegions.length} 个区域` : (isPrecisionEditTool ? '必选' : '可选') }}</span>
        </div>
        <div v-if="isPrecisionEditTool" class="imini-mask-mode-toggle" aria-label="圈选方式">
          <button
            type="button"
            :class="{ active: maskSelectionMode === 'brush' }"
            :disabled="submitting"
            data-testid="workspace-mask-mode-brush"
            @click="$emit('select-mask-mode', 'brush')"
          >
            画笔
          </button>
          <button
            type="button"
            :class="{ active: maskSelectionMode === 'lasso' }"
            :disabled="submitting"
            data-testid="workspace-mask-mode-lasso"
            @click="$emit('select-mask-mode', 'lasso')"
          >
            套索
          </button>
        </div>
        <div class="imini-erase-canvas-wrap">
          <img :src="eraseSourcePreview.preview_url" alt="" />
          <canvas
            width="720"
            height="480"
            :data-testid="isPrecisionEditTool ? 'workspace-precision-mask-canvas' : 'workspace-erase-mask-canvas'"
            @pointerdown.prevent="$emit('mask-pointer-down', $event)"
            @pointermove.prevent="$emit('mask-pointer-move', $event)"
            @pointerup.prevent="$emit('mask-pointer-up', $event)"
            @pointerleave.prevent="$emit('mask-pointer-up', $event)"
            @pointercancel.prevent="$emit('mask-pointer-up', $event)"
          ></canvas>
        </div>
        <div class="imini-erase-mask-controls">
          <label v-if="!isPrecisionEditTool || maskSelectionMode === 'brush'">
            <span>画笔</span>
            <input
              v-model.number="eraseBrushSize"
              type="range"
              min="12"
              max="72"
              step="2"
              :disabled="submitting"
              data-testid="workspace-erase-brush-size"
            />
          </label>
          <button
            type="button"
            class="secondary-button"
            :disabled="submitting || !hasEraseMask"
            :data-testid="isPrecisionEditTool ? 'workspace-precision-mask-undo' : 'workspace-erase-mask-undo'"
            @click="$emit('undo-mask')"
          >
            撤销
          </button>
          <button
            type="button"
            class="secondary-button"
            :disabled="submitting || !hasEraseMask"
            :data-testid="isPrecisionEditTool ? 'workspace-precision-mask-clear' : 'workspace-erase-mask-clear'"
            @click="$emit('clear-mask')"
          >
            清空
          </button>
        </div>
      </div>

      <div class="imini-prompt-card" :class="{ 'imini-home-prompt-pane': isHomeLayout }">
        <label class="imini-prompt-label" for="workspace-prompt-textarea">{{ promptLabel }}</label>
        <textarea
          id="workspace-prompt-textarea"
          ref="promptInputRef"
          v-model="prompt"
          data-testid="workspace-prompt-input"
          class="text-area imini-prompt-input"
          :class="{ 'imini-home-prompt-input': isHomeLayout }"
          :placeholder="promptPlaceholder"
          maxlength="6000"
          rows="7"
          :disabled="submitting"
        />
        <div class="prompt-footer imini-prompt-footer">
          <button
            class="prompt-optimizer-button"
            type="button"
            data-testid="workspace-open-prompt-optimizer"
            :disabled="submitting"
            @click="requiresAuth ? $emit('require-auth') : $emit('open-prompt-optimizer', $event)"
          >
            AI 优化
          </button>
          <span
            v-if="creditEstimateNotice"
            class="credit-estimate-notice"
            data-testid="workspace-credit-estimate-notice"
            :title="creditEstimateNotice"
            :aria-label="creditEstimateNotice"
          >
            <CircleHelp :size="16" aria-hidden="true" />
          </span>
          <span class="char-count">{{ prompt.length }}/6000</span>
        </div>
      </div>

      <div class="imini-feature-card">
        <span class="imini-feature-icon"><Edit3 :size="18" /></span>
        <span>
          <strong>精细编辑</strong>
          <small>参考图可配合提示词定向改图</small>
        </span>
        <PillTag tone="accent">NEW</PillTag>
      </div>

      <label class="imini-toggle-row">
        <span class="imini-feature-icon"><Languages :size="18" /></span>
        <span>
          <strong>启用自动翻译</strong>
          <small>翻译成英文以获得更好的结果</small>
        </span>
        <input
          v-model="autoTranslate"
          data-testid="workspace-auto-translate"
          type="checkbox"
          :disabled="submitting"
        />
      </label>

      <details
        v-if="!isHomeLayout"
        class="advanced-options imini-advanced-options"
      >
        <summary>高级选项</summary>
        <div class="prompt-section">
          <label class="section-label">反向提示词</label>
          <textarea
            v-model="negativePrompt"
            class="text-area"
            data-testid="workspace-negative-prompt"
            placeholder="描述你不想要的元素..."
            maxlength="500"
            rows="3"
            :disabled="submitting"
          />
        </div>
        <div class="style-section">
          <label class="section-label">风格预设</label>
          <div class="style-chips">
            <button
              type="button"
              class="style-chip"
              :class="{ active: !stylePreset }"
              :disabled="submitting"
              @click="stylePreset = ''"
            >
              无风格
            </button>
            <button
              v-for="style in stylePresets"
              :key="style"
              type="button"
              class="style-chip"
              :class="{ active: stylePreset === style }"
              :disabled="submitting"
              @click="$emit('select-style-preset', style)"
            >
              {{ style }}
            </button>
          </div>
        </div>
        <label v-if="showReferenceStrength" class="imini-reference-strength">
          <span class="section-label">参考强度</span>
          <strong>{{ referenceWeight }}</strong>
          <input
            v-model.number="referenceWeight"
            type="range"
            min="0"
            max="100"
            step="1"
            data-testid="workspace-reference-strength"
            :disabled="submitting"
          />
        </label>
      </details>

      <div
        class="imini-bottom-controls"
        :class="{ 'imini-home-control-bar': isHomeLayout }"
        :data-testid="isHomeLayout ? 'workspace-home-control-bar' : undefined"
      >
        <label v-if="isHomeLayout" class="imini-model-card imini-model-card--home">
          <span class="imini-model-icon">白</span>
          <span>
            <small>模型</small>
            <strong>{{ displayedModelName }}</strong>
          </span>
          <ClickSelect
            v-if="workspaceModels.length > 0"
            v-model="selectedModelId"
            :options="workspaceModels.map((model) => ({ value: model.id, label: model.name }))"
            data-testid="workspace-model-select"
            :disabled="submitting"
            trigger-label="模型"
            aria-label="模型"
          />
        </label>
        <AspectRatioSelector v-model="aspectRatio" :disabled="submitting" :trigger-label="isHomeLayout ? '比例' : ''" />
        <ClickSelect
          v-if="isHomeLayout"
          v-model="quality"
          :options="qualitySelectOptions"
          data-testid="workspace-quality-select"
          class="imini-quality-select imini-quality-select--dropdown"
          :disabled="submitting"
          trigger-label="分辨率"
          aria-label="分辨率"
        />
        <button
          v-if="isHomeLayout"
          :id="`${homeAdvancedPanelId}-toggle`"
          ref="homeAdvancedToggleRef"
          type="button"
          class="workspace-home-advanced-toggle"
          data-testid="workspace-home-advanced-toggle"
          aria-label="高级选项"
          :aria-expanded="homeAdvancedOpen ? 'true' : 'false'"
          :aria-controls="homeAdvancedPanelId"
          :disabled="submitting"
          @click="toggleHomeAdvancedPanel"
        >
          <SlidersHorizontal :size="16" aria-hidden="true" />
          <span>高级</span>
        </button>
        <div v-else class="imini-quality-group" aria-label="质量">
          <button
            v-for="item in qualityOptions"
            :key="item.key"
            type="button"
            class="imini-quality-select"
            :class="{ active: quality === item.key }"
            :data-testid="`workspace-quality-${item.key}`"
            :disabled="submitting"
            @click="quality = item.key"
          >
            {{ item.label }}
          </button>
        </div>
      </div>

      <div v-if="renderedToolFields.length > 0" class="imini-tool-options">
        <div v-if="isExpandTool" class="imini-expand-presets">
          <button
            v-for="preset in expandShortcutPresets"
            :key="preset.key"
            type="button"
            :disabled="submitting"
            @click="$emit('apply-expand-preset', preset.values)"
          >
            {{ preset.label }}
          </button>
        </div>
        <label
          v-for="field in renderedToolFields"
          :key="field.key"
          class="imini-tool-option"
          :class="{ 'imini-tool-option--wide': field.type === 'textarea' }"
        >
          <span>{{ field.label }}</span>
          <ClickSelect
            v-if="field.type === 'select'"
            :model-value="toolOptions[field.key] ?? field.default ?? field.options?.[0]"
            :options="field.options?.map((option) => ({ value: option, label: option })) ?? []"
            :data-testid="`workspace-tool-option-${field.key}`"
            :disabled="submitting"
            :aria-label="field.label"
            @update:model-value="$emit('set-tool-option', field.key, $event)"
          />
          <template v-else-if="field.type === 'number'">
            <input
              type="number"
              :value="toolOptions[field.key] ?? field.default ?? field.min ?? 0"
              :min="field.min"
              :max="field.max"
              :step="field.step || 1"
              :data-testid="`workspace-tool-option-${field.key}`"
              :disabled="submitting"
              @input="$emit('set-tool-option', field.key, Number($event.target.value))"
            />
            <input
              v-if="isExpandTool"
              class="imini-tool-range"
              type="range"
              :value="toolOptions[field.key] ?? field.default ?? field.min ?? 0"
              :min="field.min"
              :max="field.max"
              :step="field.step || 1"
              :disabled="submitting"
              @input="$emit('set-tool-option', field.key, Number($event.target.value))"
            />
          </template>
          <textarea
            v-else-if="field.type === 'textarea'"
            v-model="editInstruction"
            :data-testid="`workspace-tool-option-${field.key}`"
            rows="2"
            :disabled="submitting"
          />
          <span v-else class="imini-tool-option-hint">上传图片后可使用</span>
        </label>
      </div>

      <button
        v-if="canCancelGeneration"
        class="secondary-button workspace-cancel-generation-button"
        data-testid="workspace-cancel-generation"
        type="button"
        :disabled="cancelGenerationLoading"
        @click="$emit('cancel-generation')"
      >
        {{ cancelGenerationLoading ? '取消中...' : '取消生成' }}
      </button>

      <button
        class="create-button primary-button imini-create-button"
        :class="{ 'imini-create-button--round': isHomeLayout }"
        data-testid="workspace-create-button"
        type="submit"
        :aria-label="isHomeLayout ? `创建图片，预计消耗 ${currentEstimatedCredits} 点` : undefined"
        :disabled="requiresAuth ? false : !canSubmit"
      >
        <Sparkles :size="18" />
        <span>{{ submitting ? '提交中...' : '创建' }}</span>
        <span v-if="!submitting" class="imini-create-cost">
          <strong>{{ currentEstimatedCredits }} 点</strong>
        </span>
      </button>

      <div v-if="creditEstimateError" class="error-message">{{ creditEstimateError }}</div>
      <div v-if="isEditTool && !hasEditSourceImage && effectivePrompt" class="error-message">
        请先上传图片或选择作品作为编辑来源
      </div>
      <div v-if="successMessage" class="success-message">{{ successMessage }}</div>
    </form>
    <Teleport to="body">
      <div
        v-if="isHomeLayout && homeAdvancedOpen"
        :id="homeAdvancedPanelId"
        ref="homeAdvancedPanelRef"
        class="advanced-options imini-advanced-options imini-advanced-options--home workspace-home-advanced-panel"
        :class="`workspace-home-advanced-panel--${homeAdvancedTone}`"
        :style="homeAdvancedPanelStyle"
        data-testid="workspace-home-advanced-panel"
        role="dialog"
        aria-label="高级选项"
      >
        <div class="prompt-section">
          <label class="section-label">反向提示词</label>
          <textarea
            v-model="negativePrompt"
            class="text-area"
            data-testid="workspace-negative-prompt"
            placeholder="描述你不想要的元素..."
            maxlength="500"
            rows="3"
            :disabled="submitting"
          />
        </div>
        <div class="style-section">
          <label class="section-label">风格预设</label>
          <div class="style-chips">
            <button
              type="button"
              class="style-chip"
              :class="{ active: !stylePreset }"
              :disabled="submitting"
              @click="stylePreset = ''"
            >
              无风格
            </button>
            <button
              v-for="style in stylePresets"
              :key="style"
              type="button"
              class="style-chip"
              :class="{ active: stylePreset === style }"
              :disabled="submitting"
              @click="$emit('select-style-preset', style)"
            >
              {{ style }}
            </button>
          </div>
        </div>
        <label v-if="showReferenceStrength" class="imini-reference-strength">
          <span class="section-label">参考强度</span>
          <strong>{{ referenceWeight }}</strong>
          <input
            v-model.number="referenceWeight"
            type="range"
            min="0"
            max="100"
            step="1"
            data-testid="workspace-reference-strength"
            :disabled="submitting"
          />
        </label>
      </div>
    </Teleport>
  </aside>
</template>
