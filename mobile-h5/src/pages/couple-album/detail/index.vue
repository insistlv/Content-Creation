<script setup>
import { computed, getCurrentInstance, onBeforeUnmount, ref } from 'vue'
import { onLoad } from '@dcloudio/uni-app'

import { api } from '../../../api/client.js'
import AppTabbar from '../../../components/AppTabbar.vue'
import {
  albumPosterCanvasID,
  albumPosterCanvasStyle,
  collectAlbumDownloadPages,
  openAlbumImageLinksOnH5,
  saveAlbumImagesIndividually,
  saveAlbumPoster
} from '../../../utils/couple-album-download.js'
import { enableMiniProgramShare, navigateTo, requireAuth, routes } from '../../../utils/routes.js'

const albumID = ref('')
const album = ref(null)
const loading = ref(false)
const errorMessage = ref('')
const sharing = ref(false)
const sharePanelVisible = ref(false)
const activeShareToken = ref('')
const retryingPageID = ref(0)
const previewIndex = ref(0)
const downloadPanelVisible = ref(false)
const albumDownloadBusy = ref(false)
const albumDownloadMode = ref('')
const posterCanvasHeight = ref(1180)

let pollTimer = null
const canvasOwner = getCurrentInstance()?.proxy

const pages = computed(() => album.value?.pages || [])
const previewPages = computed(() => pages.value)
const completedPages = computed(() => pages.value.filter((page) => page.status === 'succeeded').length)
const coverPage = computed(() => pages.value.find((page) => page.id === album.value?.cover_page_id) || pages.value.find((page) => page.preview_url) || null)
const currentPreviewPageNumber = computed(() => previewPages.value[previewIndex.value]?.page_number || previewIndex.value + 1)
const isGenerating = computed(() =>
  album.value?.status === 'generating' || pages.value.some((page) => page.status === 'queued' || page.status === 'running')
)
const isChildhoodDreamAlbum = computed(() => album.value?.story_template === 'childhood_career_dream')
const albumProductName = computed(() => isChildhoodDreamAlbum.value ? '童年梦想相册' : '情侣相册')
const albumBrandName = computed(() => isChildhoodDreamAlbum.value ? '白霖共享童年梦想相册' : '白霖共享情侣相册')
const albumMetaLabel = computed(() => isChildhoodDreamAlbum.value ? '六一职业梦想' : album.value?.location)
const shareTitle = computed(() => {
  const title = `${album.value?.title || ''}`.trim()
  return title ? `${title}｜${albumBrandName.value}` : albumBrandName.value
})
const shareQuery = computed(() => {
  const token = encodeShareValue(activeShareToken.value)
  return token ? `token=${token}` : ''
})
const sharePath = computed(() => {
  const query = shareQuery.value
  return query ? `${routes.coupleAlbumShare}?${query}` : routes.works
})
const coverShareImage = computed(() => pagePreview(pages.value.find((page) => pagePreview(page)) || coverPage.value))
const posterCanvasInlineStyle = computed(() => albumPosterCanvasStyle(posterCanvasHeight.value))
const albumDownloadPlatform = {
  canvasOwner,
  canvasToTempFilePath: uni.canvasToTempFilePath,
  saveImageToPhotosAlbum: uni.saveImageToPhotosAlbum
}

enableMiniProgramShare(({ event } = {}) => albumSharePayload(event))

onLoad((options = {}) => {
  albumID.value = `${options.id || ''}`
  void requireAuth()
  void loadAlbum()
})

onBeforeUnmount(() => {
  stopPolling()
})

function showToast(title) {
  uni.showToast({ title, icon: 'none' })
}

function encodeShareValue(value) {
  return encodeURIComponent(`${value || ''}`.trim())
}

function shareEventDataset(event) {
  return event?.target?.dataset || event?.currentTarget?.dataset || event?.buttonTarget?.dataset || {}
}

function defaultAlbumSharePayload() {
  return {
    title: shareTitle.value,
    path: sharePath.value,
    query: shareQuery.value,
    imageUrl: coverShareImage.value
  }
}

function albumSharePayload(event) {
  const dataset = shareEventDataset(event)
  const fallback = defaultAlbumSharePayload()
  const token = `${dataset.shareToken || dataset.sharetoken || dataset['share-token'] || activeShareToken.value || ''}`.trim()
  if (!token) return fallback
  const query = `token=${encodeShareValue(token)}`
  return {
    ...fallback,
    title: dataset.shareTitle || dataset.sharetitle || fallback.title,
    path: `${routes.coupleAlbumShare}${query ? `?${query}` : ''}`,
    query,
    imageUrl: dataset.shareImage || dataset.shareimage || fallback.imageUrl
  }
}

function normalizeImageSource(value) {
  const source = `${value || ''}`.trim()
  if (!source) return ''
  if (/^(https?:|wxfile:|cloud:|blob:|data:image\/)/i.test(source)) return source
  if (/^\/(api|static|tmp|usr|store_|wxfile)/i.test(source)) return api.assetURL(source)
  return ''
}

function pagePreview(page) {
  return normalizeImageSource(page?.preview_url)
}

function collectDownloadInfo() {
  const preparedPages = pages.value.map((page) => ({
    ...page,
    download_url: page?.download_url || page?.preview_url
  }))
  return collectAlbumDownloadPages(preparedPages, normalizeImageSource)
}

function handlePreviewChange(event) {
  const nextIndex = Number(event?.detail?.current ?? 0)
  previewIndex.value = Number.isFinite(nextIndex) ? nextIndex : 0
}

function statusText(status) {
  switch (status) {
    case 'queued':
      return '排队中'
    case 'running':
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
    clearInterval(pollTimer)
    pollTimer = null
  }
}

function startPolling() {
  stopPolling()
  pollTimer = setInterval(() => {
    void loadAlbum({ silent: true })
  }, 1600)
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
    if (album.value?.share_enabled) {
      activeShareToken.value = `${album.value?.share_token || ''}`.trim()
    }
    if (previewIndex.value >= pages.value.length) {
      previewIndex.value = 0
    }
    if (isGenerating.value) {
      if (pollTimer === null) startPolling()
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
  try {
    const payload = await api.retryCoupleAlbumPage(albumID.value, page.id)
    album.value = payload?.album || album.value
    showToast('已提交重试')
    startPolling()
  } catch (error) {
    showToast(error.message || '重试失败')
  } finally {
    retryingPageID.value = 0
  }
}

async function shareAlbum() {
  if (!albumID.value || sharing.value) return
  sharing.value = true
  try {
    const payload = await api.shareCoupleAlbum(albumID.value)
    album.value = payload?.album || album.value
    activeShareToken.value = `${payload?.share_token || payload?.album?.share_token || album.value?.share_token || ''}`.trim()
    if (!activeShareToken.value) {
      showToast('分享已开启，请稍后重试')
      return
    }
    sharePanelVisible.value = true
  } catch (error) {
    showToast(error.message || '分享失败')
  } finally {
    sharing.value = false
  }
}

function closeNativeSharePanel() {
  sharePanelVisible.value = false
}

function openDownloadPanel() {
  const { items } = collectDownloadInfo()
  if (items.length === 0) {
    showToast('暂无可下载图片')
    return
  }
  downloadPanelVisible.value = true
}

function closeDownloadPanel() {
  if (albumDownloadBusy.value) return
  downloadPanelVisible.value = false
}

async function saveSingleAlbumImages() {
  if (albumDownloadBusy.value) return
  const { items, skippedCount } = collectDownloadInfo()
  if (items.length === 0) {
    showToast('暂无可下载图片')
    return
  }
  if (openAlbumImageLinksOnH5(items)) {
    downloadPanelVisible.value = false
    return
  }
  albumDownloadBusy.value = true
  albumDownloadMode.value = 'images'
  try {
    await saveAlbumImagesIndividually(items, {
      skippedCount,
      platform: albumDownloadPlatform
    })
    downloadPanelVisible.value = false
  } finally {
    albumDownloadBusy.value = false
    albumDownloadMode.value = ''
  }
}

async function saveAlbumLongPoster() {
  if (albumDownloadBusy.value) return
  const { items } = collectDownloadInfo()
  if (items.length === 0) {
    showToast('暂无可下载图片')
    return
  }
  albumDownloadBusy.value = true
  albumDownloadMode.value = 'poster'
  try {
    await saveAlbumPoster(items, {
      album: album.value,
      canvasID: albumPosterCanvasID,
      platform: albumDownloadPlatform,
      setCanvasHeight(height) {
        posterCanvasHeight.value = height
      },
      statusText,
      totalPageCount: pages.value.length
    })
    downloadPanelVisible.value = false
  } finally {
    albumDownloadBusy.value = false
    albumDownloadMode.value = ''
  }
}

function openWorks() {
  navigateTo(routes.works)
}

function backCreate() {
  if (isChildhoodDreamAlbum.value) {
    navigateTo(routes.coupleAlbumCreate, { mode: 'childhood-dream' })
    return
  }
  navigateTo(routes.coupleAlbumCreate)
}
</script>

<template>
  <view class="safe-page album-detail-page">
    <view class="topbar">
      <button type="button" class="ghost-button" @click="backCreate">‹</button>
      <text>{{ albumProductName }}</text>
      <button type="button" class="ghost-button" @click="openWorks">作品库</button>
    </view>

    <view v-if="loading && !album" class="state-strip">相册读取中...</view>
    <view v-else-if="errorMessage" class="state-strip error">{{ errorMessage }}</view>

    <template v-if="album">
      <view class="cover-panel">
        <image v-if="pagePreview(coverPage)" :src="pagePreview(coverPage)" mode="aspectFill" />
        <view v-else class="cover-placeholder">
          <text>相册生成中</text>
          <text>{{ completedPages }}/8 页完成</text>
        </view>
        <view class="cover-shade"></view>
        <view class="cover-copy">
          <text>{{ album.title }}</text>
          <text>{{ albumMetaLabel }} · {{ statusText(album.status) }}</text>
        </view>
      </view>

      <view class="summary-row">
        <view>
          <text>{{ completedPages }}/8</text>
          <text>页面完成</text>
        </view>
        <view>
          <text>{{ statusText(album.status) }}</text>
          <text>生成状态</text>
        </view>
        <view>
          <text>{{ album.share_enabled ? '已开启' : '私密' }}</text>
          <text>分享状态</text>
        </view>
      </view>

      <view class="action-row">
        <button type="button" class="share-button" :disabled="sharing" @click="shareAlbum">
          {{ sharing ? '开启中...' : '分享相册' }}
        </button>
        <button type="button" class="download-button" :disabled="albumDownloadBusy" @click="openDownloadPanel">
          下载相册
        </button>
        <button type="button" class="works-button" @click="openWorks">进入作品库</button>
      </view>

      <view class="album-effect-section">
        <view class="section-head">
          <text>相册效果</text>
          <text>{{ currentPreviewPageNumber }} / {{ previewPages.length || 8 }}</text>
        </view>
        <swiper
          v-if="previewPages.length"
          class="album-preview-swiper"
          :current="previewIndex"
          @change="handlePreviewChange"
        >
          <swiper-item v-for="page in previewPages" :key="page.id">
            <view class="preview-slide" :class="page.status">
              <image v-if="pagePreview(page)" :src="pagePreview(page)" mode="aspectFill" />
              <view v-else class="preview-slide-state">
                <text>{{ statusText(page.status) }}</text>
                <text>{{ page.error_message || '相册页正在准备中' }}</text>
              </view>
              <view class="preview-slide-copy">
                <text>{{ page.page_number }}. {{ page.page_title }}</text>
                <text>{{ page.caption }}</text>
              </view>
            </view>
          </swiper-item>
        </swiper>
        <view v-else class="album-preview-empty">
          <text>相册生成中</text>
          <text>完成后可左右滑动预览 8 页故事</text>
        </view>
      </view>

      <view class="page-section">
        <view class="section-head">
          <text>8 页故事缩略图</text>
          <text>失败页重试</text>
        </view>
        <view class="page-grid">
          <view v-for="page in pages" :key="page.id" class="page-card" :class="page.status">
            <view class="thumb">
              <image v-if="pagePreview(page)" :src="pagePreview(page)" mode="aspectFill" />
              <view v-else class="thumb-state">{{ statusText(page.status) }}</view>
            </view>
            <view class="page-copy">
              <text>{{ page.page_number }}. {{ page.page_title }}</text>
              <text>{{ page.caption }}</text>
              <text v-if="page.error_message" class="page-error">{{ page.error_message }}</text>
            </view>
            <button
              v-if="page.status === 'failed'"
              type="button"
              class="retry-button"
              :disabled="retryingPageID === page.id"
              @click="retryPage(page)"
            >
              {{ retryingPageID === page.id ? '提交中...' : '重试' }}
            </button>
          </view>
        </view>
      </view>
    </template>

    <view v-if="downloadPanelVisible" class="download-overlay" @click="closeDownloadPanel">
      <view class="download-panel" @click.stop>
        <view class="download-panel-header">
          <view>
            <text>下载相册</text>
            <text>仅保存已完成并有图片的页面</text>
          </view>
          <button type="button" :disabled="albumDownloadBusy" @click="closeDownloadPanel">×</button>
        </view>
        <button
          type="button"
          class="download-option"
          :disabled="albumDownloadBusy"
          @click="saveSingleAlbumImages"
        >
          <text>保存单张图片</text>
          <text>{{ albumDownloadMode === 'images' ? '正在保存到系统相册' : '逐张保存成功页面，失败项会继续跳过' }}</text>
        </button>
        <button
          type="button"
          class="download-option"
          :disabled="albumDownloadBusy"
          @click="saveAlbumLongPoster"
        >
          <text>保存长图</text>
          <text>{{ albumDownloadMode === 'poster' ? '正在生成相册长图' : '生成 1080px 竖版相册海报并保存到相册' }}</text>
        </button>
      </view>
    </view>

    <canvas
      canvas-id="album-poster-canvas"
      id="album-poster-canvas"
      class="album-poster-canvas"
      :style="posterCanvasInlineStyle"
    ></canvas>

    <view v-if="sharePanelVisible" class="native-share-overlay" @click="closeNativeSharePanel">
      <view class="native-share-panel" @click.stop>
        <view class="native-share-header">
          <text>微信分享</text>
          <button type="button" @click="closeNativeSharePanel">×</button>
        </view>
        <view class="native-share-summary">
          <image v-if="coverShareImage" :src="coverShareImage" mode="aspectFill" />
          <view>
            <text>{{ shareTitle }}</text>
            <text>{{ completedPages }}/8 页 · {{ statusText(album?.status) }}</text>
          </view>
        </view>
        <button
          type="button"
          class="native-share-button"
          open-type="share"
          data-share-kind="album"
          :data-share-token="activeShareToken"
          :data-share-title="shareTitle"
          :data-share-image="coverShareImage"
          @click.stop="closeNativeSharePanel"
        >
          发送给好友
        </button>
      </view>
    </view>

    <AppTabbar active-key="" />
  </view>
</template>

<style lang="scss" scoped>
@use '../../../styles/tokens.scss' as *;

.album-detail-page {
  padding-left: 28rpx;
  padding-right: 28rpx;
  background:
    radial-gradient(circle at 16% 10%, rgba(255, 214, 225, 0.68), transparent 30%),
    radial-gradient(circle at 90% 8%, rgba(209, 232, 255, 0.78), transparent 30%),
    linear-gradient(180deg, #fff8fb 0%, #f7fbff 58%, #eef6ff 100%);
}

.topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 74rpx;
  color: $ink;
  font-size: 28rpx;
  font-weight: 950;
}

.ghost-button {
  display: grid;
  place-items: center;
  min-width: 64rpx;
  height: 58rpx;
  padding: 0 18rpx;
  border-radius: 18rpx;
  background: rgba(255, 255, 255, 0.78);
  color: #9f1239;
  font-size: 23rpx;
  font-weight: 950;
  box-shadow: $shadow-card;
}

.state-strip {
  margin-top: 28rpx;
  padding: 24rpx;
  border-radius: 24rpx;
  background: rgba(255, 255, 255, 0.8);
  color: $muted;
  font-size: 24rpx;
  font-weight: 850;
  box-shadow: $shadow-card;
}

.state-strip.error {
  color: #b91c1c;
}

.cover-panel {
  position: relative;
  height: 520rpx;
  margin-top: 28rpx;
  overflow: hidden;
  border-radius: 34rpx;
  background:
    linear-gradient(135deg, rgba(255, 228, 235, 0.92), rgba(230, 242, 255, 0.92)),
    radial-gradient(circle at 22% 20%, rgba(225, 29, 72, 0.22), transparent 28%);
  box-shadow: 0 28rpx 74rpx rgba(154, 54, 88, 0.14);
}

.cover-panel image {
  width: 100%;
  height: 100%;
}

.cover-placeholder {
  display: grid;
  place-items: center;
  align-content: center;
  gap: 14rpx;
  height: 100%;
  color: #9f1239;
  font-weight: 950;
}

.cover-placeholder text:first-child {
  font-size: 36rpx;
}

.cover-placeholder text:last-child {
  font-size: 24rpx;
}

.cover-shade {
  position: absolute;
  inset: 0;
  background: linear-gradient(180deg, transparent 34%, rgba(17, 24, 39, 0.62));
}

.cover-copy {
  position: absolute;
  left: 30rpx;
  right: 30rpx;
  bottom: 30rpx;
  display: grid;
  gap: 10rpx;
  color: #fff;
}

.cover-copy text:first-child {
  font-size: 42rpx;
  font-weight: 950;
  line-height: 1.16;
}

.cover-copy text:last-child {
  font-size: 23rpx;
  font-weight: 850;
}

.summary-row {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 14rpx;
  margin-top: 22rpx;
}

.summary-row view {
  display: grid;
  gap: 6rpx;
  justify-items: center;
  min-height: 104rpx;
  padding: 18rpx 8rpx;
  border-radius: 22rpx;
  background: rgba(255, 255, 255, 0.8);
  box-shadow: $shadow-card;
}

.summary-row text:first-child {
  color: #8f1238;
  font-size: 27rpx;
  font-weight: 950;
}

.summary-row text:last-child {
  color: $muted;
  font-size: 20rpx;
  font-weight: 800;
}

.action-row {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16rpx;
  margin-top: 22rpx;
}

.share-button,
.download-button,
.works-button,
.retry-button {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 76rpx;
  border-radius: 999rpx;
  font-size: 24rpx;
  font-weight: 950;
}

.share-button {
  background: #e11d48;
  color: #fff;
  box-shadow: 0 16rpx 36rpx rgba(225, 29, 72, 0.22);
}

.download-button {
  background: #121b33;
  color: #fff;
  box-shadow: 0 16rpx 36rpx rgba(18, 27, 51, 0.16);
}

.works-button {
  background: rgba(255, 255, 255, 0.82);
  color: #8f1238;
  box-shadow: $shadow-card;
}

.album-effect-section {
  margin-top: 30rpx;
}

.album-preview-swiper {
  width: 100%;
  height: 830rpx;
  margin-top: 18rpx;
  overflow: hidden;
  border-radius: 30rpx;
  background: #151827;
  box-shadow: 0 26rpx 68rpx rgba(87, 38, 65, 0.18);
}

.preview-slide {
  position: relative;
  width: 100%;
  height: 100%;
  overflow: hidden;
  background:
    linear-gradient(135deg, rgba(255, 230, 238, 0.98), rgba(229, 242, 255, 0.96)),
    #f7edf3;
}

.preview-slide image {
  width: 100%;
  height: 100%;
}

.preview-slide::after {
  content: '';
  position: absolute;
  inset: 0;
  background: linear-gradient(180deg, transparent 48%, rgba(12, 16, 28, 0.72));
}

.preview-slide-state {
  display: grid;
  place-items: center;
  align-content: center;
  gap: 14rpx;
  height: 100%;
  padding: 44rpx;
  color: #9f1239;
  text-align: center;
}

.preview-slide-state text:first-child {
  font-size: 36rpx;
  font-weight: 950;
}

.preview-slide-state text:last-child {
  color: #7a5361;
  font-size: 23rpx;
  font-weight: 800;
  line-height: 1.45;
}

.preview-slide-copy {
  position: absolute;
  left: 30rpx;
  right: 30rpx;
  bottom: 32rpx;
  z-index: 1;
  display: grid;
  gap: 12rpx;
  color: #fff;
}

.preview-slide-copy text:first-child {
  font-size: 34rpx;
  font-weight: 950;
  line-height: 1.18;
}

.preview-slide-copy text:last-child {
  color: rgba(255, 255, 255, 0.88);
  font-size: 23rpx;
  font-weight: 800;
  line-height: 1.45;
}

.album-preview-empty {
  display: grid;
  place-items: center;
  align-content: center;
  gap: 14rpx;
  height: 430rpx;
  margin-top: 18rpx;
  border-radius: 28rpx;
  background: rgba(255, 255, 255, 0.78);
  color: #9f1239;
  box-shadow: $shadow-card;
}

.album-preview-empty text:first-child {
  font-size: 32rpx;
  font-weight: 950;
}

.album-preview-empty text:last-child {
  color: $muted;
  font-size: 22rpx;
  font-weight: 800;
}

.page-section {
  margin-top: 28rpx;
}

.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  color: $ink;
}

.section-head text:first-child {
  font-size: 28rpx;
  font-weight: 950;
}

.section-head text:last-child {
  color: $muted;
  font-size: 21rpx;
  font-weight: 850;
}

.page-grid {
  display: grid;
  gap: 18rpx;
  margin-top: 18rpx;
}

.page-card {
  display: grid;
  grid-template-columns: 168rpx minmax(0, 1fr);
  gap: 18rpx;
  align-items: center;
  padding: 16rpx;
  border-radius: 24rpx;
  background: rgba(255, 255, 255, 0.82);
  box-shadow: $shadow-card;
}

.thumb {
  display: grid;
  place-items: center;
  width: 168rpx;
  height: 212rpx;
  overflow: hidden;
  border-radius: 18rpx;
  background: #f7e9ef;
}

.thumb image {
  width: 100%;
  height: 100%;
}

.thumb-state {
  color: #9f1239;
  font-size: 22rpx;
  font-weight: 950;
}

.page-copy {
  display: grid;
  gap: 8rpx;
  min-width: 0;
}

.page-copy text:first-child {
  color: $ink;
  font-size: 25rpx;
  font-weight: 950;
}

.page-copy text:nth-child(2) {
  color: #6f5360;
  font-size: 21rpx;
  font-weight: 750;
  line-height: 1.46;
}

.page-error {
  color: #b91c1c;
  font-size: 20rpx;
  line-height: 1.42;
}

.retry-button {
  grid-column: 2;
  width: 150rpx;
  min-height: 58rpx;
  background: rgba(225, 29, 72, 0.1);
  color: #be123c;
}

.download-overlay {
  position: fixed;
  inset: 0;
  z-index: 72;
  display: flex;
  justify-content: flex-end;
  align-items: stretch;
  flex-direction: column;
  padding: 28rpx 24rpx calc(28rpx + env(safe-area-inset-bottom));
  background: rgba(14, 20, 36, 0.56);
}

.download-panel {
  width: 100%;
  padding: 0 26rpx 28rpx;
  border-radius: 20rpx;
  background: #fff;
  box-shadow: 0 28rpx 72rpx rgba(16, 24, 40, 0.24);
}

.download-panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 96rpx;
  border-bottom: 1rpx solid rgba(18, 27, 51, 0.08);
}

.download-panel-header view {
  display: grid;
  gap: 6rpx;
}

.download-panel-header text:first-child {
  color: #121b33;
  font-size: 27rpx;
  font-weight: 950;
}

.download-panel-header text:last-child {
  color: $muted;
  font-size: 21rpx;
  font-weight: 800;
}

.download-panel-header button {
  width: 56rpx;
  height: 56rpx;
  border-radius: 50%;
  background: rgba(18, 27, 51, 0.08);
  color: #4d5874;
  font-size: 32rpx;
  font-weight: 850;
  line-height: 56rpx;
}

.download-option {
  display: grid;
  justify-items: start;
  width: 100%;
  min-height: 110rpx;
  margin-top: 18rpx;
  padding: 22rpx 24rpx;
  border-radius: 18rpx;
  background: #fff7fa;
  color: #121b33;
  text-align: left;
}

.download-option text:first-child {
  font-size: 26rpx;
  font-weight: 950;
}

.download-option text:last-child {
  margin-top: 8rpx;
  color: $muted;
  font-size: 21rpx;
  font-weight: 780;
  line-height: 1.36;
}

.album-poster-canvas {
  position: fixed;
  left: -12000px;
  top: 0;
  opacity: 0;
  pointer-events: none;
}

.native-share-overlay {
  position: fixed;
  inset: 0;
  z-index: 70;
  display: flex;
  justify-content: flex-end;
  align-items: stretch;
  flex-direction: column;
  padding: 28rpx 24rpx calc(28rpx + env(safe-area-inset-bottom));
  background: rgba(14, 20, 36, 0.56);
}

.native-share-panel {
  width: 100%;
  border-radius: 20rpx;
  background: #fff;
  box-shadow: 0 28rpx 72rpx rgba(16, 24, 40, 0.24);
}

.native-share-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 88rpx;
  padding: 0 26rpx;
  border-bottom: 1rpx solid rgba(18, 27, 51, 0.08);
}

.native-share-header text {
  color: #121b33;
  font-size: 27rpx;
  font-weight: 950;
}

.native-share-header button {
  width: 56rpx;
  height: 56rpx;
  border-radius: 50%;
  background: rgba(18, 27, 51, 0.08);
  color: #4d5874;
  font-size: 32rpx;
  font-weight: 850;
  line-height: 56rpx;
}

.native-share-summary {
  display: flex;
  gap: 20rpx;
  padding: 24rpx 26rpx 8rpx;
}

.native-share-summary image {
  width: 104rpx;
  height: 104rpx;
  flex: 0 0 104rpx;
  border-radius: 16rpx;
  background: #f7e9ef;
}

.native-share-summary view {
  display: grid;
  align-content: center;
  min-width: 0;
  gap: 8rpx;
}

.native-share-summary text {
  display: block;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.native-share-summary text:first-child {
  color: #121b33;
  font-size: 26rpx;
  font-weight: 950;
}

.native-share-summary text:last-child {
  color: $muted;
  font-size: 22rpx;
  font-weight: 820;
}

.native-share-button {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 82rpx;
  margin: 22rpx 26rpx 28rpx;
  border-radius: 16rpx;
  background: #e11d48;
  color: #fff;
  font-size: 25rpx;
  font-weight: 950;
  line-height: 82rpx;
}
</style>
