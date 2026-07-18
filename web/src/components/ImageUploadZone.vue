<script setup>
import { ref } from 'vue'
import { Upload, X } from 'lucide-vue-next'

const props = defineProps({
  images: {
    type: Array,
    default: () => []
  },
  maxImages: {
    type: Number,
    default: 4
  },
  uploading: {
    type: Boolean,
    default: false
  },
  disabled: {
    type: Boolean,
    default: false
  },
  requiresAuth: {
    type: Boolean,
    default: false
  },
  emptyTitle: {
    type: String,
    default: '上传参考图'
  },
  emptyHint: {
    type: String,
    default: ''
  },
  emptyHintSecondary: {
    type: String,
    default: '点击或拖拽图片到此区域'
  },
  libraryActionLabel: {
    type: String,
    default: ''
  },
  libraryActionTestid: {
    type: String,
    default: ''
  }
})

const emit = defineEmits(['upload', 'remove', 'require-auth', 'select-library'])

const isDragging = ref(false)
const fileInput = ref(null)
const validationError = ref('')
const maxFileSize = 20 * 1024 * 1024

function handleDragOver(event) {
  event.preventDefault()
  if (props.requiresAuth) {
    return
  }
  if (!props.disabled && !props.uploading) {
    isDragging.value = true
  }
}

function handleDragLeave() {
  isDragging.value = false
}

function handleDrop(event) {
  event.preventDefault()
  isDragging.value = false

  if (props.disabled || props.uploading) return
  if (props.requiresAuth) {
    emit('require-auth')
    return
  }

  const files = Array.from(event.dataTransfer.files)
  handleFiles(files)
}

function handleFileSelect(event) {
  if (props.disabled || props.uploading) {
    event.target.value = ''
    return
  }
  if (props.requiresAuth) {
    emit('require-auth')
    event.target.value = ''
    return
  }
  const files = Array.from(event.target.files)
  handleFiles(files)
  event.target.value = ''
}

function handleFiles(files) {
  if (props.disabled || props.uploading) return
  validationError.value = ''
  const validFiles = files.filter((file) => {
    const isValidFormat = ['image/jpeg', 'image/png', 'image/webp'].includes(file.type)
    if (!isValidFormat) validationError.value = '图片格式不支持，请上传 JPG、PNG 或 WEBP。'
    else if (file.size > maxFileSize) validationError.value = '图片大小超过 20MB，请压缩后重试。'
    return isValidFormat && file.size <= maxFileSize
  })

  if (validFiles.length === 0) return

  const remainingSlots = props.maxImages - props.images.length
  const filesToUpload = validFiles.slice(0, remainingSlots)

  filesToUpload.forEach((file) => {
    emit('upload', file)
  })
}

function handleKeydown(event) {
  if (event.target !== event.currentTarget) return
  if (event.key === 'Enter' || event.key === ' ') {
    event.preventDefault()
    openFileDialog()
  }
}

function handleRemove(image) {
  if (props.disabled || props.uploading) return
  emit('remove', image)
}

function openFileDialog() {
  if (props.disabled || props.uploading) return
  if (props.requiresAuth) {
    emit('require-auth')
    return
  }
  fileInput.value?.click()
}

function handleSelectLibrary() {
  if (props.disabled || props.uploading) return
  if (props.requiresAuth) {
    emit('require-auth')
    return
  }
  emit('select-library')
}
</script>

<template>
  <div class="image-upload-container">
    <div
      class="image-upload-zone"
      :class="{ dragging: isDragging, disabled, uploading }"
      :aria-busy="uploading ? 'true' : 'false'"
      :aria-disabled="disabled || uploading ? 'true' : 'false'"
      :tabindex="disabled || uploading ? -1 : 0"
      role="button"
      @dragover="handleDragOver"
      @dragleave="handleDragLeave"
      @drop="handleDrop"
      @click="openFileDialog"
      @keydown="handleKeydown"
    >
      <input
        ref="fileInput"
        type="file"
        accept="image/jpeg,image/png,image/webp"
        multiple
        style="display: none"
        :disabled="disabled || uploading"
        @change="handleFileSelect"
      />

      <div v-if="images.length === 0" class="upload-prompt">
        <Upload :size="32" class="upload-icon" />
        <p class="upload-title">{{ emptyTitle }}</p>
        <p class="upload-hint">{{ emptyHint || `支持 JPG/PNG/WEBP 格式，单张不超过 20MB，最多 ${maxImages} 张` }}</p>
        <p v-if="emptyHintSecondary" class="upload-hint-secondary">{{ emptyHintSecondary }}</p>
        <button
          v-if="libraryActionLabel"
          class="upload-library-button"
          :data-testid="libraryActionTestid || undefined"
          type="button"
          :disabled="disabled || uploading"
          @click.stop="handleSelectLibrary"
        >
          {{ libraryActionLabel }}
        </button>
      </div>

      <div v-else class="image-preview-grid">
        <div
          v-for="(image, index) in images"
          :key="image.id || index"
          class="image-preview-item"
        >
          <img
            :src="image.preview_url || image.url"
            :alt="image.original_filename || `图片 ${index + 1}`"
            class="preview-image"
          />
          <span v-if="image.original_filename" class="image-preview-name">{{ image.original_filename }}</span>
          <button
            class="remove-button"
            @click.stop="handleRemove(image)"
            :disabled="disabled || uploading"
          >
            <X :size="16" />
          </button>
        </div>

        <div
          v-if="images.length < maxImages"
          class="add-more-button"
          @click.stop="openFileDialog"
        >
          <Upload :size="24" />
          <span>添加更多</span>
        </div>
      </div>

      <p
        v-if="validationError"
        class="upload-error"
        role="alert"
        data-testid="image-upload-error"
      >{{ validationError }}</p>

      <p
        v-if="uploading"
        class="upload-status"
        data-testid="image-upload-status"
        role="status"
        aria-live="polite"
      >
        <span class="upload-status-indicator" aria-hidden="true">
          <span></span>
          <span></span>
          <span></span>
        </span>
        <span>上传中...</span>
      </p>
    </div>
  </div>
</template>

<style scoped>
.image-upload-zone:focus-visible {
  outline: 3px solid var(--accent-color, #7c5cff);
  outline-offset: 3px;
}
.upload-error { color: var(--danger-color, #ff6b7a); }
</style>
