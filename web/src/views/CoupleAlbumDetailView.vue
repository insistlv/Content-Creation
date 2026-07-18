<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { Copy, Heart, RefreshCcw, Share2 } from 'lucide-vue-next'

import { api } from '../api/client.js'
import SoftPanel from '../components/SoftPanel.vue'

const route = useRoute()
const albumID = computed(() => `${route.params.id || ''}`.trim())
const album = ref(null)
const loading = ref(false)
const errorMessage = ref('')
const sharing = ref(false)
const retryingPageID = ref(0)
const sharePanelOpen = ref(false)
const shareLink = ref('')
const copyMessage = ref('')

let pollTimer = null

const pages = computed(() => album.value?.pages || [])
const completedPages = computed(() => pages.value.filter((page) => page.status === 'succeeded').length)
const totalPages = computed(() => Math.max(8, pages.value.length || 0))
const progressPercent = computed(() => Math.round((completedPages.value / totalPages.value) * 100))
const coverPage = computed(() =>
  pages.value.find((page) => page.id === album.value?.cover_page_id && page.preview_url) ||
  pages.value.find((page) => page.preview_url) ||
  null
)
const isGenerating = computed(() =>
  album.value?.status === 'generating' ||
  pages.value.some((page) => page.status === 'queued' || page.status === 'running')
)

function statusText(status) {
  switch (status) {
    case 'queued':
      return '排队中'
    case 'running':
    case 'generating':
      return '生成中'
    case 'succeeded':
      return '已完成'
    case 'failed':
      return '失败'
    case 'partial_failed':
      return '部分失败'
    default:
      return '草稿'
  }
}

function stopPolling() {
  if (pollTimer !== null) {
    window.clearInterval(pollTimer)
    pollTimer = null
  }
}

function startPolling() {
  if (pollTimer !== null) return
  pollTimer = window.setInterval(() => {
    void loadAlbum({ silent: true })
  }, 1000)
}

async function loadAlbum(options = {}) {
  if (!albumID.value) {
    errorMessage.value = '相册不存在'
    return
  }
  if (!options.silent) {
    loading.value = true
  }
  errorMessage.value = ''
  try {
    const payload = await api.getCoupleAlbum(albumID.value)
    album.value = payload?.album || null
    if (album.value?.share_enabled && album.value?.share_token) {
      shareLink.value = buildPublicShareLink(album.value.share_token)
    }
    if (isGenerating.value) {
      startPolling()
    } else {
      stopPolling()
    }
  } catch (error) {
    errorMessage.value = error.message || '相册读取失败'
    stopPolling()
  } finally {
    loading.value = false
  }
}

async function retryPage(page) {
  if (!page?.id || retryingPageID.value) return
  retryingPageID.value = page.id
  errorMessage.value = ''
  try {
    const payload = await api.retryCoupleAlbumPage(albumID.value, page.id)
    album.value = payload?.album || album.value
    startPolling()
  } catch (error) {
    errorMessage.value = error.message || '重试失败'
  } finally {
    retryingPageID.value = 0
  }
}

function buildPublicShareLink(token) {
  const encodedToken = encodeURIComponent(`${token || ''}`.trim())
  if (!encodedToken) return ''
  return `${window.location.origin}/couple-albums/share/${encodedToken}`
}

async function shareAlbum() {
  if (!albumID.value || sharing.value) return
  sharing.value = true
  copyMessage.value = ''
  try {
    const payload = await api.shareCoupleAlbum(albumID.value)
    album.value = payload?.album || album.value
    const token = `${payload?.share_token || payload?.album?.share_token || ''}`.trim()
    shareLink.value = buildPublicShareLink(token)
    sharePanelOpen.value = Boolean(shareLink.value)
  } catch (error) {
    errorMessage.value = error.message || '分享开启失败'
  } finally {
    sharing.value = false
  }
}

async function copyShareLink() {
  if (!shareLink.value) return
  await navigator?.clipboard?.writeText(shareLink.value)
  copyMessage.value = '链接已复制'
}

onMounted(() => {
  void loadAlbum()
})

onBeforeUnmount(() => {
  stopPolling()
})
</script>

<template>
  <div v-if="loading && !album" class="workspace-loading">
    <p>相册读取中...</p>
  </div>

  <div v-else class="couple-album-detail-page">
    <p v-if="errorMessage" class="status-error couple-album-feedback" role="alert">{{ errorMessage }}</p>

    <template v-if="album">
      <section class="couple-album-detail-hero">
        <div class="couple-album-cover">
          <img v-if="coverPage?.preview_url" :src="coverPage.preview_url" :alt="album.title">
          <div v-else class="couple-album-cover-empty">
            <Heart :size="30" />
            <span>相册生成中</span>
          </div>
          <div class="couple-album-cover-copy">
            <p>{{ album.location }} · {{ statusText(album.status) }}</p>
            <h1 data-testid="couple-album-detail-title">{{ album.title }}</h1>
          </div>
        </div>

        <SoftPanel class="couple-album-detail-summary" tone="highlight" roomy>
          <div class="couple-album-section-head">
            <Heart :size="20" />
            <div>
              <h2>生成进度</h2>
              <p>{{ completedPages }}/{{ totalPages }} 页完成</p>
            </div>
          </div>
          <div class="couple-album-progress">
            <span :style="{ width: `${progressPercent}%` }"></span>
          </div>
          <div class="couple-album-summary-grid">
            <div>
              <strong>{{ completedPages }}/{{ totalPages }}</strong>
              <span>页面完成</span>
            </div>
            <div>
              <strong>{{ statusText(album.status) }}</strong>
              <span>生成状态</span>
            </div>
            <div>
              <strong>{{ album.share_enabled ? '已开启' : '私密' }}</strong>
              <span>分享状态</span>
            </div>
          </div>
          <button
            class="primary-button"
            data-testid="couple-album-share"
            type="button"
            :disabled="sharing"
            @click="shareAlbum"
          >
            <Share2 :size="17" />
            {{ sharing ? '开启中...' : '分享相册' }}
          </button>
        </SoftPanel>
      </section>

      <SoftPanel v-if="sharePanelOpen" class="couple-album-share-panel" tone="default">
        <div>
          <strong>公开网页链接</strong>
          <span>复制后可在 PC 或手机浏览器中打开。</span>
        </div>
        <input
          class="text-input"
          data-testid="couple-album-share-url"
          readonly
          :value="shareLink"
        />
        <button
          class="secondary-button"
          data-testid="couple-album-copy-share-url"
          type="button"
          @click="copyShareLink"
        >
          <Copy :size="16" />
          复制链接
        </button>
        <span v-if="copyMessage" class="status-success">{{ copyMessage }}</span>
      </SoftPanel>

      <section class="couple-album-pages-section">
        <div class="couple-album-section-head">
          <Heart :size="20" />
          <div>
            <h2>8 页故事效果</h2>
            <p>失败页面可单独重试，成功页面会保留现有结果。</p>
          </div>
        </div>

        <div class="couple-album-page-grid">
          <article
            v-for="page in pages"
            :key="page.id"
            class="couple-album-page-card"
            :class="`status-${page.status}`"
            :data-testid="`couple-album-page-${page.id}`"
          >
            <div class="couple-album-page-image">
              <img v-if="page.preview_url" :src="page.preview_url" :alt="page.page_title">
              <span v-else>{{ statusText(page.status) }}</span>
            </div>
            <div class="couple-album-page-copy">
              <strong>{{ page.page_number }}. {{ page.page_title }}</strong>
              <p>{{ page.caption }}</p>
              <span v-if="page.error_message" class="status-error">{{ page.error_message }}</span>
            </div>
            <button
              v-if="page.status === 'failed'"
              class="secondary-button"
              :data-testid="`retry-couple-album-page-${page.id}`"
              type="button"
              :disabled="retryingPageID === page.id"
              @click="retryPage(page)"
            >
              <RefreshCcw :size="16" />
              {{ retryingPageID === page.id ? '提交中...' : '重试' }}
            </button>
          </article>
        </div>
      </section>
    </template>
  </div>
</template>
