<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'

import { api } from '../api/client.js'

const route = useRoute()
const loading = ref(false)
const errorMessage = ref('')
const works = ref([])

const shareIds = computed(() => String(route.query.ids || '')
  .split(',')
  .map((item) => item.trim())
  .filter(Boolean)
  .slice(0, 16)
  .join(',')
)

async function load() {
  if (!shareIds.value) {
    errorMessage.value = '分享链接无效'
    return
  }
  loading.value = true
  errorMessage.value = ''
  try {
    const payload = await api.getPublicWorks({ ids: shareIds.value })
    works.value = Array.isArray(payload) ? payload : (payload?.items ?? [])
    if (works.value.length === 0) {
      errorMessage.value = '公开作品不可见'
    }
  } catch (error) {
    errorMessage.value = error.message || '公开作品加载失败'
  } finally {
    loading.value = false
  }
}

function workId(work) {
  return work?.work_id ?? work?.id
}

onMounted(load)
</script>

<template>
  <section class="works-share-page">
    <header class="works-share-header">
      <p class="eyebrow">SHARE</p>
      <h1>公开作品分享</h1>
      <p>最多展示 16 个已公开作品。</p>
    </header>

    <p v-if="loading" class="page-status">加载中...</p>
    <p v-else-if="errorMessage" class="status-error">{{ errorMessage }}</p>

    <div v-else class="works-share-grid">
      <article
        v-for="work in works"
        :key="workId(work)"
        class="works-share-card"
        :data-testid="`works-share-card-${workId(work)}`"
      >
        <img v-if="work.preview_url" :src="work.preview_url" :alt="work.prompt || '公开作品'" />
        <div v-else class="works-card-placeholder">公开作品</div>
        <span class="ai-content-badge">AI生成</span>
        <div>
          <h2>{{ work.prompt || '未命名作品' }}</h2>
          <p>{{ work.aspect_ratio || '默认画幅' }}</p>
        </div>
      </article>
    </div>
  </section>
</template>
