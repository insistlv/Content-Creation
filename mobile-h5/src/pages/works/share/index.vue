<script setup>
import { computed, ref } from 'vue'
import { onLoad } from '@dcloudio/uni-app'

import { api } from '../../../api/client.js'
import { enableMiniProgramShare, routes } from '../../../utils/routes.js'

const shareIds = ref([])
const works = ref([])
const loading = ref(false)
const errorMessage = ref('')
const workShareRoute = routes.workShare || '/pages/works/share/index'

const shareQuery = computed(() => {
  const ids = shareIds.value.map((id) => encodeURIComponent(id)).join(',')
  return ids ? `ids=${ids}` : ''
})
const sharePath = computed(() => {
  const query = shareQuery.value
  return `${workShareRoute}${query ? `?${query}` : ''}`
})
const shareTitle = computed(() => {
  if (works.value.length > 1) return `白霖共享 AI 作品 · 共 ${works.value.length} 张`
  const text = `${works.value[0]?.title || works.value[0]?.prompt || ''}`.trim()
  if (!text) return '白霖共享 AI 作品'
  return text.length > 24 ? `${text.slice(0, 24)}...` : text
})
const coverShareImage = computed(() => normalizeImageSource(works.value[0]?.preview_url))

enableMiniProgramShare(() => ({
  title: shareTitle.value,
  path: sharePath.value,
  query: shareQuery.value,
  imageUrl: coverShareImage.value
}))

onLoad((options = {}) => {
  shareIds.value = parseShareIds(options.ids || options.id || '')
  void loadSharedWorks()
})

function parseShareIds(value) {
  return `${value || ''}`
    .split(',')
    .map((id) => id.trim())
    .filter(Boolean)
    .slice(0, 16)
}

function normalizeImageSource(value) {
  const source = `${value || ''}`.trim()
  if (!source) return ''
  if (/^(https?:|wxfile:|cloud:|blob:|data:image\/)/i.test(source)) return source
  if (/^\/(api|static|tmp|usr|store_|wxfile)/i.test(source)) return api.assetURL(source)
  return ''
}

function previewURL(work) {
  return normalizeImageSource(work?.preview_url)
}

function workTitle(work, index) {
  const text = `${work?.title || work?.prompt || ''}`.trim()
  if (!text) return `公开作品 ${index + 1}`
  return text.length > 30 ? `${text.slice(0, 30)}...` : text
}

function workMeta(work) {
  return [work?.aspect_ratio || '1:1', work?.category || 'image'].filter(Boolean).join(' · ')
}

async function loadSharedWorks() {
  if (shareIds.value.length === 0) {
    errorMessage.value = '作品暂不可访问'
    return
  }
  loading.value = true
  errorMessage.value = ''
  try {
    const payload = await api.getPublicWorks({ ids: shareIds.value.join(',') })
    works.value = Array.isArray(payload?.items) ? payload.items : []
    if (works.value.length === 0) {
      errorMessage.value = '作品暂不可访问'
    }
  } catch (error) {
    errorMessage.value = error.message || '作品暂不可访问'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <view class="safe-page work-share-page">
    <view v-if="loading" class="state-strip">作品读取中...</view>
    <view v-else-if="errorMessage" class="state-strip error">{{ errorMessage }}</view>

    <template v-if="works.length > 0">
      <view class="share-hero">
        <text class="eyebrow">AI WORKS</text>
        <text class="share-title">{{ shareTitle }}</text>
        <text class="share-count">共 {{ works.length }} 张公开作品</text>
      </view>

      <view class="shared-work-grid">
        <view v-for="(work, index) in works" :key="work.work_id || index" class="shared-work">
          <image v-if="previewURL(work)" :src="previewURL(work)" mode="aspectFill" />
          <view v-else class="shared-work-placeholder">暂无预览</view>
          <view class="shared-copy">
            <text>{{ workTitle(work, index) }}</text>
            <text>{{ workMeta(work) }}</text>
          </view>
        </view>
      </view>
    </template>
  </view>
</template>

<style lang="scss" scoped>
@use '../../../styles/tokens.scss' as *;

.work-share-page {
  min-height: 100vh;
  padding: 34rpx 28rpx 52rpx;
  background:
    radial-gradient(circle at 18% 0, rgba(255, 215, 230, 0.72), transparent 32%),
    radial-gradient(circle at 88% 8%, rgba(202, 231, 255, 0.72), transparent 30%),
    linear-gradient(180deg, #fff8fb 0%, #f7fbff 54%, #eef6ff 100%);
  color: #121b33;
}

.state-strip {
  margin-top: 40rpx;
  padding: 26rpx;
  border-radius: 18rpx;
  background: rgba(255, 255, 255, 0.86);
  color: $muted;
  font-size: 24rpx;
  font-weight: 850;
  box-shadow: $shadow-card;
}

.state-strip.error {
  color: #b91c1c;
}

.share-hero {
  display: flex;
  min-height: 250rpx;
  flex-direction: column;
  justify-content: flex-end;
  gap: 14rpx;
  padding: 26rpx 0 30rpx;
}

.eyebrow {
  color: #315cff;
  font-size: 20rpx;
  font-weight: 950;
  letter-spacing: 0;
}

.share-title {
  color: #121b33;
  font-size: 44rpx;
  font-weight: 950;
  line-height: 1.16;
}

.share-count {
  color: $muted;
  font-size: 24rpx;
  font-weight: 820;
}

.shared-work-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 18rpx;
}

.shared-work {
  overflow: hidden;
  border-radius: 18rpx;
  background: rgba(255, 255, 255, 0.9);
  box-shadow: $shadow-card;
}

.shared-work image,
.shared-work-placeholder {
  width: 100%;
  height: 330rpx;
  background: #eef3ff;
}

.shared-work-placeholder {
  display: flex;
  align-items: center;
  justify-content: center;
  color: $muted;
  font-size: 24rpx;
  font-weight: 850;
}

.shared-copy {
  display: flex;
  min-height: 112rpx;
  flex-direction: column;
  justify-content: center;
  gap: 8rpx;
  padding: 18rpx;
}

.shared-copy text {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.shared-copy text:first-child {
  color: #121b33;
  font-size: 24rpx;
  font-weight: 920;
}

.shared-copy text:last-child {
  color: $muted;
  font-size: 21rpx;
  font-weight: 780;
}
</style>
