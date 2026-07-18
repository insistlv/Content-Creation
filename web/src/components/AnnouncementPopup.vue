<script setup>
import { computed, ref, watch } from 'vue'
import { Bell, ChevronRight, X } from 'lucide-vue-next'

import { api } from '../api/client.js'

const props = defineProps({
  enabled: {
    type: Boolean,
    default: false
  },
  client: {
    type: String,
    default: 'web'
  }
})

const items = ref([])
const activeIndex = ref(0)
const visible = ref(false)
const loading = ref(false)

const current = computed(() => items.value[activeIndex.value] ?? null)
const hasNext = computed(() => activeIndex.value < items.value.length - 1)
const positionText = computed(() => `${activeIndex.value + 1} / ${items.value.length}`)
const toneClass = computed(() => `announcement-popup-${current.value?.level || 'info'}`)

async function loadAnnouncements() {
  if (!props.enabled || loading.value) return
  loading.value = true
  try {
    const payload = await api.listPopupAnnouncements(props.client)
    items.value = payload.items ?? []
    activeIndex.value = 0
    visible.value = items.value.length > 0
  } catch {
    items.value = []
    visible.value = false
  } finally {
    loading.value = false
  }
}

function showNext() {
  if (hasNext.value) {
    activeIndex.value += 1
  }
}

async function dismissCurrent() {
  const target = current.value
  visible.value = false
  if (!target?.id) return
  try {
    await api.dismissAnnouncement(target.id, props.client)
  } catch {
    // 关闭操作不阻塞用户继续工作。
  }
}

function formatDate(value) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false
  })
}

watch(
  () => [props.enabled, props.client],
  () => {
    if (props.enabled) {
      loadAnnouncements()
    } else {
      visible.value = false
    }
  },
  { immediate: true }
)
</script>

<template>
  <div v-if="visible && current" class="announcement-popup-backdrop">
    <section class="announcement-popup" :class="toneClass" data-testid="announcement-popup" role="dialog" aria-modal="true" aria-labelledby="announcement-popup-title">
      <button class="announcement-popup-close" data-testid="announcement-popup-close" type="button" aria-label="关闭公告" @click="dismissCurrent">
        <X :size="18" />
      </button>

      <div class="announcement-popup-head">
        <span class="announcement-popup-icon" aria-hidden="true">
          <Bell :size="18" />
        </span>
        <div>
          <p>公告通知</p>
          <h2 id="announcement-popup-title">{{ current.title }}</h2>
        </div>
      </div>

      <p class="announcement-popup-content">{{ current.content }}</p>

      <div class="announcement-popup-meta">
        <span>{{ formatDate(current.published_at || current.created_at) }}</span>
        <span v-if="items.length > 1">{{ positionText }}</span>
      </div>

      <div class="announcement-popup-actions">
        <a
          v-if="current.action_text && current.action_url"
          class="primary-button announcement-popup-action"
          data-testid="announcement-popup-action"
          :href="current.action_url"
        >
          {{ current.action_text }}
        </a>
        <button v-if="hasNext" class="secondary-button announcement-popup-next" data-testid="announcement-popup-next" type="button" @click="showNext">
          下一条
          <ChevronRight :size="16" />
        </button>
      </div>
    </section>
  </div>
</template>
