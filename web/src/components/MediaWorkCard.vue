<script setup>
import { computed } from 'vue'

const props = defineProps({
  item: {
    type: Object,
    required: true
  },
  showActions: {
    type: Boolean,
    default: true
  },
  compact: {
    type: Boolean,
    default: false
  }
})

defineEmits(['reuse', 'remove'])

const timestamp = computed(() => {
  if (!props.item?.created_at) {
    return '刚刚生成'
  }
  return new Date(props.item.created_at).toLocaleString()
})
</script>

<template>
  <article :class="['media-work-card', { 'media-work-card-compact': compact }]">
    <div class="media-work-frame">
      <img v-if="item.preview_url" :src="item.preview_url" alt="" />
      <div v-else class="media-work-placeholder">等待作品</div>
      <span class="ai-content-badge">AI生成</span>

      <div v-if="showActions" class="media-work-overlay">
        <a v-if="item.download_url" class="ghost-button" :href="item.download_url">下载</a>
        <button class="ghost-button" type="button" @click="$emit('reuse', item.work_id)">复用</button>
        <button class="ghost-button destructive-button" type="button" @click="$emit('remove', item.work_id)">删除</button>
      </div>
    </div>

    <div class="media-work-meta">
      <strong>{{ item.prompt || '未命名作品' }}</strong>
      <span>{{ timestamp }}</span>
    </div>
  </article>
</template>
