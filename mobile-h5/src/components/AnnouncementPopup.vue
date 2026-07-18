<script setup>
import { computed, onMounted, ref } from 'vue'

import { api } from '../api/client.js'
import { navigateTo, routes } from '../utils/routes.js'

const props = defineProps({
  client: {
    type: String,
    default: 'mp-weixin'
  }
})

const items = ref([])
const activeIndex = ref(0)
const visible = ref(false)
const loading = ref(false)

const current = computed(() => items.value[activeIndex.value] ?? null)
const hasNext = computed(() => activeIndex.value < items.value.length - 1)
const positionText = computed(() => `${activeIndex.value + 1} / ${items.value.length}`)

async function loadAnnouncements() {
  if (loading.value) return
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

async function closePopup() {
  const target = current.value
  visible.value = false
  if (!target?.id) return
  try {
    await api.dismissAnnouncement(target.id, props.client)
  } catch {
    // 关闭公告不阻塞当前页面操作。
  }
}

function openAction() {
  const url = `${current.value?.action_url || ''}`.trim()
  if (!url) return
  visible.value = false
  if (url.startsWith('/pages/')) {
    navigateTo(url)
    return
  }
  if (url.startsWith('/workspace')) {
    navigateTo(routes.imageToImage)
    return
  }
  if (url.startsWith('/works')) {
    navigateTo(routes.works)
    return
  }
  if (url.startsWith('/pricing')) {
    navigateTo(routes.pricing)
    return
  }
  if (url.startsWith('/account')) {
    navigateTo(routes.account)
  }
}

function formatDate(value) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const month = `${date.getMonth() + 1}`.padStart(2, '0')
  const day = `${date.getDate()}`.padStart(2, '0')
  const hour = `${date.getHours()}`.padStart(2, '0')
  const minute = `${date.getMinutes()}`.padStart(2, '0')
  return `${month}-${day} ${hour}:${minute}`
}

onMounted(loadAnnouncements)
</script>

<template>
  <view v-if="visible && current" class="announcement-popup-mask">
    <view class="announcement-popup-card">
      <button class="announcement-popup-close" type="button" @click="closePopup">×</button>
      <view class="announcement-popup-head" :class="`level-${current.level || 'info'}`">
        <view class="announcement-popup-icon">!</view>
        <view>
          <text class="announcement-popup-kicker">公告通知</text>
          <text class="announcement-popup-title">{{ current.title }}</text>
        </view>
      </view>
      <text class="announcement-popup-content">{{ current.content }}</text>
      <view class="announcement-popup-meta">
        <text>{{ formatDate(current.published_at || current.created_at) }}</text>
        <text v-if="items.length > 1">{{ positionText }}</text>
      </view>
      <view class="announcement-popup-actions">
        <button
          v-if="current.action_text && current.action_url"
          class="announcement-popup-action primary"
          type="button"
          @click="openAction"
        >
          {{ current.action_text }}
        </button>
        <button v-if="hasNext" class="announcement-popup-next" type="button" @click="showNext">下一条</button>
      </view>
    </view>
  </view>
</template>

<style lang="scss" scoped>
@use '../styles/tokens.scss' as *;

.announcement-popup-mask {
  position: fixed;
  top: 0;
  right: 0;
  bottom: 0;
  left: 0;
  z-index: 999;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 36rpx;
  background: rgba(15, 23, 42, 0.34);
}

.announcement-popup-card {
  position: relative;
  display: grid;
  width: 100%;
  max-width: 650rpx;
  gap: 24rpx;
  padding: 34rpx;
  border-radius: 28rpx;
  background: rgba(255, 255, 255, 0.96);
  box-shadow: 0 36rpx 90rpx rgba(15, 23, 42, 0.22);
}

.announcement-popup-close {
  position: absolute;
  top: 18rpx;
  right: 18rpx;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 56rpx;
  height: 56rpx;
  border-radius: 18rpx;
  background: rgba(15, 23, 42, 0.06);
  color: #42526e;
  font-size: 36rpx;
  line-height: 1;
}

.announcement-popup-head,
.announcement-popup-actions,
.announcement-popup-meta {
  display: flex;
  align-items: center;
}

.announcement-popup-head {
  gap: 18rpx;
  padding-right: 58rpx;
}

.announcement-popup-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  width: 58rpx;
  height: 58rpx;
  border-radius: 20rpx;
  font-size: 30rpx;
  font-weight: 950;
}

.level-info .announcement-popup-icon {
  background: rgba(36, 92, 255, 0.12);
  color: $accent;
}

.level-important .announcement-popup-icon {
  background: rgba(124, 77, 255, 0.13);
  color: $violet;
}

.level-warning .announcement-popup-icon {
  background: rgba(217, 119, 6, 0.15);
  color: #b45309;
}

.announcement-popup-kicker,
.announcement-popup-title,
.announcement-popup-content,
.announcement-popup-meta text {
  display: block;
}

.announcement-popup-kicker {
  color: $subtle;
  font-size: 22rpx;
  font-weight: 900;
}

.announcement-popup-title {
  margin-top: 4rpx;
  color: $ink;
  font-size: 34rpx;
  font-weight: 950;
  line-height: 1.25;
}

.announcement-popup-content {
  color: #3b465d;
  font-size: 26rpx;
  font-weight: 700;
  line-height: 1.7;
  white-space: pre-wrap;
}

.announcement-popup-meta {
  justify-content: space-between;
  gap: 18rpx;
  color: $subtle;
  font-size: 22rpx;
  font-weight: 800;
}

.announcement-popup-actions {
  justify-content: flex-end;
  gap: 16rpx;
}

.announcement-popup-action,
.announcement-popup-next {
  display: flex;
  align-items: center;
  justify-content: center;
  min-width: 150rpx;
  height: 68rpx;
  padding: 0 22rpx;
  border-radius: 20rpx;
  font-size: 24rpx;
  font-weight: 950;
}

.announcement-popup-action.primary {
  background: linear-gradient(100deg, #2563ff 0%, #7c3aed 100%);
  color: #fff;
}

.announcement-popup-next {
  background: rgba(36, 92, 255, 0.08);
  color: $accent;
}
</style>
