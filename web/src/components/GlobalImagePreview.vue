<script setup>
import { onBeforeUnmount, onMounted, ref } from 'vue'

const preview = ref(null)

function closePreview() {
  preview.value = null
}

function handleImageDoubleClick(event) {
  const image = event.target?.closest?.('img')
  if (!image) return
  if (image.closest('[data-global-image-preview-modal]')) return
  if (image.closest('[data-skip-global-image-preview]')) return

  const src = image.getAttribute('src') || image.currentSrc || image.src
  if (!src) return

  event.preventDefault()
  event.stopPropagation()

  preview.value = {
    src,
    alt: image.getAttribute('alt') || '图片预览'
  }
}

function handleKeydown(event) {
  if (event.key === 'Escape') {
    closePreview()
  }
}

onMounted(() => {
  document.addEventListener('dblclick', handleImageDoubleClick, true)
  window.addEventListener('keydown', handleKeydown)
})

onBeforeUnmount(() => {
  document.removeEventListener('dblclick', handleImageDoubleClick, true)
  window.removeEventListener('keydown', handleKeydown)
})
</script>

<template>
  <Teleport to="body">
    <div
      v-if="preview"
      class="global-image-preview-modal"
      data-testid="global-image-preview-modal"
      data-global-image-preview-modal
      role="dialog"
      aria-modal="true"
      aria-label="图片放大预览"
      @click="closePreview"
    >
      <div class="global-image-preview-dialog" @click.stop>
        <button
          class="global-image-preview-close"
          data-testid="global-image-preview-close"
          type="button"
          aria-label="关闭图片预览"
          @click="closePreview"
        >
          ×
        </button>
        <img :src="preview.src" :alt="preview.alt" />
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.global-image-preview-modal {
  position: fixed;
  inset: 0;
  z-index: 2200;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 32px;
  background: rgba(8, 13, 24, 0.82);
  backdrop-filter: blur(12px);
}

.global-image-preview-dialog {
  position: relative;
  display: flex;
  max-width: min(1120px, 96vw);
  max-height: min(760px, 92vh);
}

.global-image-preview-dialog img {
  max-width: 100%;
  max-height: min(760px, 92vh);
  object-fit: contain;
  border-radius: 8px;
  box-shadow: 0 24px 70px rgba(0, 0, 0, 0.38);
}

.global-image-preview-close {
  position: absolute;
  top: -14px;
  right: -14px;
  width: 36px;
  height: 36px;
  border: 0;
  border-radius: 999px;
  background: #ffffff;
  color: #111827;
  font-size: 22px;
  line-height: 1;
  cursor: pointer;
  box-shadow: 0 10px 28px rgba(0, 0, 0, 0.24);
}

@media (max-width: 640px) {
  .global-image-preview-modal {
    padding: 18px;
  }

  .global-image-preview-close {
    top: 8px;
    right: 8px;
  }
}
</style>
