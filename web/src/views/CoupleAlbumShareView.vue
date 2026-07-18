<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { Heart } from 'lucide-vue-next'

import { api } from '../api/client.js'

const route = useRoute()
const token = computed(() => `${route.params.token || ''}`.trim())
const album = ref(null)
const loading = ref(false)
const errorMessage = ref('')

const pages = computed(() => album.value?.pages || [])
const completedPages = computed(() => pages.value.filter((page) => page.status === 'succeeded' && page.preview_url).length)
const coverPage = computed(() =>
  pages.value.find((page) => page.id === album.value?.cover_page_id && page.preview_url) ||
  pages.value.find((page) => page.preview_url) ||
  null
)

function statusText(status) {
  switch (status) {
    case 'succeeded':
      return '已完成'
    case 'partial_failed':
      return '部分失败'
    case 'failed':
      return '失败'
    case 'generating':
      return '生成中'
    default:
      return '草稿'
  }
}

function publicAlbumLoadErrorMessage(error) {
  if (error?.status === 404 || error?.code === 'album_not_found') {
    return '链接无效或分享已关闭'
  }
  return error?.message || '相册读取失败'
}

async function loadSharedAlbum() {
  if (!token.value) {
    errorMessage.value = '链接无效或分享已关闭'
    return
  }
  loading.value = true
  errorMessage.value = ''
  try {
    const payload = await api.getPublicCoupleAlbum(token.value)
    album.value = payload?.album || null
    if (!album.value) {
      errorMessage.value = '相册读取失败'
    }
  } catch (error) {
    errorMessage.value = publicAlbumLoadErrorMessage(error)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  void loadSharedAlbum()
})
</script>

<template>
  <section class="couple-album-share-page">
    <div v-if="loading" class="workspace-loading">
      <p>相册读取中...</p>
    </div>

    <div v-else-if="errorMessage" class="couple-album-public-error" role="alert">
      {{ errorMessage }}
    </div>

    <template v-else-if="album">
      <div class="couple-album-public-hero">
        <img v-if="coverPage?.preview_url" :src="coverPage.preview_url" :alt="album.title">
        <div v-else class="couple-album-cover-empty">
          <Heart :size="30" />
          <span>暂无封面</span>
        </div>
        <div class="couple-album-public-copy">
          <p>白霖共享情侣相册</p>
          <h1>{{ album.title }}</h1>
          <span>{{ album.location }} · {{ statusText(album.status) }} · {{ completedPages }}/8 页完成</span>
        </div>
      </div>

      <div class="couple-album-public-grid">
        <article
          v-for="page in pages"
          :key="page.id"
          class="couple-album-public-card"
          :data-testid="`public-couple-album-page-${page.id}`"
        >
          <img :src="page.preview_url" :alt="page.page_title">
          <div>
            <strong>{{ page.page_number }}. {{ page.page_title }}</strong>
            <p>{{ page.caption }}</p>
          </div>
        </article>
      </div>
    </template>
  </section>
</template>
