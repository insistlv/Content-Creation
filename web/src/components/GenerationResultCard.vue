<script setup>
import { computed } from 'vue'
import {
  RefreshCw,
  Shuffle,
  Eraser,
  Maximize2,
  Heart,
  Download,
  Clock,
  Image as ImageIcon
} from 'lucide-vue-next'
import PillTag from './PillTag.vue'

const props = defineProps({
  work: {
    type: Object,
    required: true
  },
  showActions: {
    type: Boolean,
    default: true
  }
})

const emit = defineEmits([
  'regenerate',
  'randomRedraw',
  'smartErase',
  'expand',
  'toggleFavorite',
  'download',
  'preview'
])

const statusMap = {
  succeeded: { label: '已完成', tone: 'success' },
  queued: { label: '排队中', tone: 'default' },
  requesting_provider: { label: '生成中', tone: 'highlight' },
  persisting_result: { label: '保存中', tone: 'highlight' },
  failed: { label: '失败', tone: 'danger' }
}

const statusInfo = computed(() => {
  const status = props.work.status || 'succeeded'
  return statusMap[status] || statusMap.succeeded
})

const formattedTime = computed(() => {
  if (!props.work.created_at) return ''
  const date = new Date(props.work.created_at)
  const now = new Date()
  const diff = now - date
  const minutes = Math.floor(diff / 60000)
  const hours = Math.floor(diff / 3600000)
  const days = Math.floor(diff / 86400000)

  if (minutes < 1) return '刚刚'
  if (minutes < 60) return `${minutes}分钟前`
  if (hours < 24) return `${hours}小时前`
  return `${days}天前`
})

const truncatedPrompt = computed(() => {
  const prompt = props.work.prompt || ''
  return prompt.length > 100 ? prompt.slice(0, 100) + '...' : prompt
})
</script>

<template>
  <div class="generation-result-card">
    <div class="card-header">
      <PillTag :tone="statusInfo.tone">{{ statusInfo.label }}</PillTag>
      <div class="card-time">
        <Clock :size="14" />
        <span>{{ formattedTime }}</span>
      </div>
    </div>

    <div class="card-preview" @click="emit('preview', work)">
      <img
        v-if="work.preview_url"
        :src="work.preview_url"
        :alt="work.prompt"
        class="generation-card-preview-image"
        title="双击放大查看"
        data-skip-global-image-preview
        @dblclick.stop.prevent="emit('preview', work)"
      />
      <div v-else class="preview-placeholder">
        <ImageIcon :size="48" />
      </div>
      <span class="ai-content-badge">AI生成</span>
    </div>

    <div class="card-content">
      <div class="card-info">
        <p class="card-prompt">{{ truncatedPrompt }}</p>
        <div v-if="work.aspect_ratio" class="card-meta">
          <span>{{ work.aspect_ratio }}</span>
          <span v-if="work.model_name">· {{ work.model_name }}</span>
        </div>
      </div>

      <div v-if="work.reference_assets?.length" class="card-references">
        <div class="reference-thumbnails">
          <img
            v-for="(ref, index) in work.reference_assets.slice(0, 3)"
            :key="index"
            :src="ref.preview_url"
            :alt="ref.original_filename"
            class="reference-thumb"
          />
        </div>
      </div>
    </div>

    <div v-if="showActions" class="result-card-actions">
      <button
        class="action-button"
        title="再次生成"
        @click="emit('regenerate', work)"
      >
        <RefreshCw :size="16" />
      </button>
      <button
        class="action-button"
        title="随机重绘"
        @click="emit('randomRedraw', work)"
      >
        <Shuffle :size="16" />
      </button>
      <button
        class="action-button"
        title="智能擦除"
        @click="emit('smartErase', work)"
      >
        <Eraser :size="16" />
      </button>
      <button
        class="action-button"
        title="扩图"
        @click="emit('expand', work)"
      >
        <Maximize2 :size="16" />
      </button>
      <button
        class="action-button"
        :class="{ active: work.is_favorite }"
        title="收藏"
        @click="emit('toggleFavorite', work)"
      >
        <Heart :size="16" :fill="work.is_favorite ? 'currentColor' : 'none'" />
      </button>
      <button
        class="action-button"
        title="下载"
        @click="emit('download', work)"
      >
        <Download :size="16" />
      </button>
    </div>
  </div>
</template>
