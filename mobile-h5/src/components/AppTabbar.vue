<script setup>
import { computed } from 'vue'

import { navigateTo, routes } from '../utils/routes.js'

const props = defineProps({
  activeKey: {
    type: String,
    default: ''
  },
  extraSpace: {
    type: String,
    default: '0rpx'
  }
})

const staticAssetBaseURL = `${import.meta.env.VITE_STATIC_ASSET_BASE_URL || ''}`.replace(/\/+$/, '')

function staticAsset(path) {
  const normalizedPath = `${path || ''}`.trim().replace(/^\/+/, '').replace(/^static\/+/i, '')
  if (!normalizedPath) return staticAssetBaseURL
  if (staticAssetBaseURL) return `${staticAssetBaseURL}/${normalizedPath}`
  return `/${['static', normalizedPath].join('/')}`
}

function staticIcon(name) {
  const normalizedName = `${name || ''}`.trim().replace(/\.png$/i, '')
  return staticAsset(`icons/${normalizedName}.png`)
}

const icons = {
  workspace: staticIcon('home'),
  works: staticIcon('workspace'),
  pricing: staticIcon('pricing'),
  account: staticIcon('account')
}

const spacerStyle = computed(() => ({
  height: `calc(112rpx + env(safe-area-inset-bottom) + ${props.extraSpace || '0rpx'})`
}))

function openWorkspace() {
  navigateTo(routes.imageToImage)
}

function openWorks() {
  navigateTo(routes.works)
}

function openPricing() {
  navigateTo(routes.pricing)
}

function openAccount() {
  navigateTo(routes.account)
}
</script>

<template>
  <view class="app-tabbar-spacer" :style="spacerStyle"></view>
  <view class="app-tabbar" data-component="AppTabbar">
    <view class="app-tabbar__items">
      <button
        type="button"
        class="app-tabbar__item"
        :class="{ active: activeKey === 'workspace' }"
        @click="openWorkspace"
      >
        <image :src="icons.workspace" mode="aspectFit" />
        <text>工作台</text>
      </button>
      <button
        type="button"
        class="app-tabbar__item"
        :class="{ active: activeKey === 'works' }"
        @click="openWorks"
      >
        <image :src="icons.works" mode="aspectFit" />
        <text>作品库</text>
      </button>
      <button
        type="button"
        class="app-tabbar__item"
        :class="{ active: activeKey === 'pricing' }"
        @click="openPricing"
      >
        <image :src="icons.pricing" mode="aspectFit" />
        <text>套餐</text>
      </button>
      <button
        type="button"
        class="app-tabbar__item"
        :class="{ active: activeKey === 'account' }"
        @click="openAccount"
      >
        <image :src="icons.account" mode="aspectFit" />
        <text>我的</text>
      </button>
    </view>
  </view>
</template>

<style lang="scss" scoped>
.app-tabbar-spacer {
  width: 100%;
  pointer-events: none;
}

.app-tabbar {
  position: fixed;
  left: 0;
  right: 0;
  bottom: 0;
  z-index: 20;
  padding: 8rpx 0 calc(8rpx + env(safe-area-inset-bottom));
  border-top: 1rpx solid rgba(148, 163, 184, 0.18);
  background: rgba(255, 255, 255, 0.96);
  box-shadow: 0 -16rpx 42rpx rgba(31, 45, 82, 0.08);
  backdrop-filter: blur(18rpx);
}

.app-tabbar__items {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  min-height: 88rpx;
}

.app-tabbar__item {
  position: relative;
  display: grid;
  place-items: center;
  align-content: center;
  gap: 4rpx;
  min-width: 0;
  min-height: 88rpx;
  padding: 0;
  border-radius: 0;
  background: transparent;
  box-shadow: none;
  color: #70798c;
  font-size: 20rpx;
  font-weight: 900;
  line-height: 1;
  transition: color 0.18s ease, background 0.18s ease;
}

.app-tabbar__item::before {
  content: '';
  position: absolute;
  top: 6rpx;
  left: 50%;
  width: 34rpx;
  height: 5rpx;
  border-radius: 999rpx;
  background: linear-gradient(90deg, #8c4dff 0%, #236cff 100%);
  opacity: 0;
  transform: translateX(-50%) scaleX(0.55);
  transition: opacity 0.18s ease, transform 0.18s ease;
}

.app-tabbar__item::after {
  border: 0;
}

.app-tabbar__item image {
  width: 30rpx;
  height: 30rpx;
  transition: transform 0.18s ease;
}

.app-tabbar__item text {
  display: block;
  line-height: 1.08;
}

.app-tabbar__item.active {
  color: #315cff;
  background: rgba(121, 86, 255, 0.08);
}

.app-tabbar__item.active::before {
  opacity: 1;
  transform: translateX(-50%) scaleX(1);
}

.app-tabbar__item.active image {
  transform: translateY(-1rpx);
}
</style>
