<script setup>
import AnnouncementPopup from '../../components/AnnouncementPopup.vue'
import { enableMiniProgramShare, navigateTo, routes } from '../../utils/routes.js'

const staticAssetBaseURL = `${import.meta.env.VITE_STATIC_ASSET_BASE_URL || ''}`.replace(/\/+$/, '')

function staticAsset(path) {
  const normalizedPath = `${path || ''}`.trim().replace(/^\/+/, '').replace(/^static\/+/i, '')
  if (!normalizedPath) return staticAssetBaseURL
  if (staticAssetBaseURL) return `${staticAssetBaseURL}/${normalizedPath}`
  return `/${['static', normalizedPath].join('/')}`
}

const homeReplicaAssets = {
  mountainHero: staticAsset('home-replica/mountain-hero.png'),
  portraitCard: staticAsset('home-replica/portrait-card.png'),
  productBottleSmall: staticAsset('home-replica/product-bottle-small.png'),
  cityStage: staticAsset('home-replica/city-stage.png'),
  albumBook: staticAsset('home-replica/couple-album-book.png'),
  productBottleLarge: staticAsset('home-replica/product-bottle-large.png')
}

const features = [
  {
    icon: '↯',
    title: '持续生成工作台',
    text: '提示词与模板连续生产'
  },
  {
    icon: '▣',
    title: '智能入库管理',
    text: '作品资产自动沉淀'
  },
  {
    icon: '▱',
    title: '素材复用交付',
    text: '同款结果快速复用'
  }
]

const workflowSteps = ['提示词', '模板', '生成', '入库', '复用']

function openHome() {
  navigateTo(routes.home)
}

function openWorkspace() {
  navigateTo(routes.workspace)
}

function openCoupleAlbum() {
  navigateTo(routes.coupleAlbumCreate)
}

function openSupport() {
  navigateTo(routes.support)
}

function openAccount() {
  navigateTo(routes.account)
}

enableMiniProgramShare()
</script>

<template>
  <view class="safe-page mobile-home">
    <view class="topbar">
      <view class="brand">
        <view class="brand-mark">
          <text>✦</text>
        </view>
        <view class="brand-copy">
          <text class="brand-name">白霖共享</text>
          <text class="brand-subtitle">创作者 AI 图片平台</text>
        </view>
      </view>
      <view class="top-actions">
        <button class="icon-button notice-button" type="button" @click="openSupport">
          <text>⌁</text>
        </button>
        <button class="account-button" type="button" @click="openAccount">A</button>
      </view>
    </view>

    <view class="hero-panel">
      <view class="hero-copy">
        <text class="eyebrow">CREATOR PORTAL</text>
        <text class="hero-title">一站式 AI 生图工作台</text>
        <text class="hero-summary">提示词、模板、生成、入库、复用，一站完成</text>
        <button class="primary-action" type="button" @click="openWorkspace">
          <text>进入工作台</text>
          <text class="action-arrow">→</text>
        </button>
      </view>

      <view class="hero-visual" aria-hidden="true">
        <view class="visual-card visual-card-main">
          <image :src="homeReplicaAssets.mountainHero" mode="aspectFill" />
        </view>
        <view class="visual-card visual-card-portrait">
          <image :src="homeReplicaAssets.portraitCard" mode="aspectFill" />
        </view>
        <view class="visual-card visual-card-product">
          <image :src="homeReplicaAssets.productBottleSmall" mode="aspectFill" />
        </view>
        <view class="visual-card visual-card-stage">
          <image :src="homeReplicaAssets.cityStage" mode="aspectFill" />
        </view>
        <view class="ai-badge">AI</view>
      </view>
    </view>

    <view class="feature-grid">
      <view v-for="item in features" :key="item.title" class="feature-item">
        <view class="feature-icon">
          <text>{{ item.icon }}</text>
        </view>
        <text class="feature-title">{{ item.title }}</text>
        <text class="feature-text">{{ item.text }}</text>
      </view>
    </view>

    <view class="campaign-grid">
      <button class="campaign-card album-card" type="button" @click="openCoupleAlbum">
        <image class="campaign-image" :src="homeReplicaAssets.albumBook" mode="aspectFill" />
        <view class="campaign-copy">
          <text class="campaign-kicker">520 限定</text>
          <text class="campaign-title">520 情侣相册</text>
          <text class="campaign-text">粉色相册书，双人故事一键生成</text>
          <text class="campaign-link">创建相册</text>
        </view>
      </button>

      <button class="campaign-card product-card" type="button" @click="openWorkspace">
        <image class="campaign-image product-image" :src="homeReplicaAssets.productBottleLarge" mode="aspectFill" />
        <view class="campaign-copy">
          <text class="campaign-kicker">商品图卡</text>
          <text class="campaign-title">商品主图</text>
          <text class="campaign-text">上传参考图，生成同款电商视觉</text>
          <text class="campaign-link">生成同款</text>
        </view>
      </button>
    </view>

    <view class="workflow-card">
      <view class="workflow-copy">
        <text class="workflow-kicker">生成入库复用</text>
        <text class="workflow-title">生成入库复用说明</text>
        <text class="workflow-text">创作结果默认私有留存，可在工作台继续套用提示词、参考图和成品资产。</text>
      </view>
      <view class="workflow-steps">
        <view v-for="step in workflowSteps" :key="step" class="workflow-step">
          <text>{{ step }}</text>
        </view>
      </view>
    </view>

    <view class="home-tabbar-spacer"></view>
    <view class="home-local-tabbar">
      <button class="home-tabbar-item active" type="button" @click="openHome">
        <text class="tabbar-icon">◆</text>
        <text>首页</text>
      </button>
      <button class="home-tabbar-item" type="button" @click="openWorkspace">
        <text class="tabbar-icon">▣</text>
        <text>工作台</text>
      </button>
      <button class="home-tabbar-item" type="button" @click="openAccount">
        <text class="tabbar-icon">○</text>
        <text>我的</text>
      </button>
    </view>

    <AnnouncementPopup />
  </view>
</template>

<style lang="scss" scoped>
@use '../../styles/tokens.scss' as *;

.mobile-home {
  position: relative;
  overflow: hidden;
  padding-right: 24rpx;
  padding-left: 24rpx;
  background:
    linear-gradient(132deg, rgba(255, 248, 253, 0.96) 0%, rgba(244, 249, 255, 0.94) 43%, rgba(231, 242, 255, 0.96) 100%),
    linear-gradient(180deg, #fbfdff 0%, #eef6ff 100%);
}

.mobile-home,
.mobile-home button,
.mobile-home text {
  letter-spacing: 0;
}

.mobile-home::before {
  content: '';
  position: absolute;
  inset: 132rpx 0 auto;
  height: 1rpx;
  background: rgba(117, 137, 186, 0.16);
}

.topbar,
.hero-panel,
.feature-grid,
.campaign-grid,
.workflow-card {
  position: relative;
  z-index: 1;
}

.topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18rpx;
  min-height: 92rpx;
  padding-bottom: 26rpx;
}

.brand,
.top-actions,
.primary-action,
.campaign-link,
.workflow-steps,
.home-local-tabbar,
.home-tabbar-item {
  display: flex;
  align-items: center;
}

.brand {
  min-width: 0;
  gap: 16rpx;
}

.brand-mark {
  display: grid;
  place-items: center;
  flex: 0 0 auto;
  width: 52rpx;
  height: 52rpx;
  border-radius: 16rpx;
  color: #fff;
  font-size: 32rpx;
  line-height: 1;
  background: linear-gradient(135deg, #8b6fff 0%, #245cff 100%);
  box-shadow: 0 18rpx 36rpx rgba(68, 84, 230, 0.2);
}

.brand-copy,
.brand-name,
.brand-subtitle,
.eyebrow,
.hero-title,
.hero-summary,
.feature-title,
.feature-text,
.campaign-kicker,
.campaign-title,
.campaign-text,
.campaign-link,
.workflow-kicker,
.workflow-title,
.workflow-text {
  display: block;
}

.brand-copy {
  min-width: 0;
}

.brand-name {
  overflow: hidden;
  color: $ink;
  font-size: 29rpx;
  font-weight: 950;
  line-height: 1.18;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.brand-subtitle {
  overflow: hidden;
  margin-top: 4rpx;
  color: #6e7894;
  font-size: 20rpx;
  font-weight: 800;
  line-height: 1.2;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.top-actions {
  flex: 0 0 auto;
  gap: 12rpx;
}

.icon-button,
.account-button {
  display: grid;
  place-items: center;
  width: 58rpx;
  height: 58rpx;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.88);
  box-shadow: 0 14rpx 32rpx rgba(49, 72, 128, 0.12);
}

.notice-button {
  color: #5b6a8b;
  font-size: 30rpx;
  font-weight: 950;
}

.account-button {
  color: #fff;
  font-size: 24rpx;
  font-weight: 950;
  background: linear-gradient(135deg, #8a63ff 0%, #245cff 100%);
  box-shadow: 0 18rpx 36rpx rgba(48, 89, 238, 0.26);
}

.hero-panel {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 286rpx;
  gap: 14rpx;
  min-height: 430rpx;
  padding: 34rpx 24rpx 30rpx;
  border: 1rpx solid rgba(255, 255, 255, 0.88);
  border-radius: 36rpx;
  background:
    linear-gradient(128deg, rgba(255, 255, 255, 0.94) 0%, rgba(248, 251, 255, 0.76) 52%, rgba(231, 241, 255, 0.74) 100%),
    linear-gradient(180deg, rgba(255, 255, 255, 0.78), rgba(238, 246, 255, 0.82));
  box-shadow: $shadow-soft;
}

.hero-copy {
  position: relative;
  z-index: 2;
  min-width: 0;
  padding-top: 10rpx;
}

.eyebrow {
  color: #6b77a0;
  font-size: 19rpx;
  font-weight: 950;
  line-height: 1.2;
}

.hero-title {
  width: 352rpx;
  max-width: 100%;
  margin-top: 20rpx;
  color: #101827;
  font-size: 54rpx;
  font-weight: 950;
  line-height: 1.12;
}

.hero-summary {
  width: 348rpx;
  max-width: 100%;
  margin-top: 18rpx;
  color: #65718d;
  font-size: 23rpx;
  font-weight: 750;
  line-height: 1.5;
}

.primary-action {
  justify-content: center;
  gap: 12rpx;
  width: 264rpx;
  max-width: 100%;
  min-height: 72rpx;
  margin-top: 26rpx;
  border-radius: 20rpx;
  color: #fff;
  font-size: 24rpx;
  font-weight: 950;
  white-space: nowrap;
  background: linear-gradient(135deg, #8056ff 0%, #225fff 100%);
  box-shadow: 0 20rpx 40rpx rgba(45, 92, 240, 0.28);
}

.action-arrow {
  font-size: 25rpx;
}

.hero-visual {
  position: relative;
  min-width: 0;
  height: 366rpx;
}

.visual-card {
  position: absolute;
  overflow: hidden;
  border: 7rpx solid rgba(255, 255, 255, 0.94);
  border-radius: 22rpx;
  background: #edf4ff;
  box-shadow: 0 18rpx 44rpx rgba(60, 78, 134, 0.18);
}

.visual-card image,
.campaign-image {
  width: 100%;
  height: 100%;
}

.visual-card-main {
  top: 22rpx;
  left: 28rpx;
  width: 158rpx;
  height: 220rpx;
  transform: rotate(-5deg);
}

.visual-card-portrait {
  top: 42rpx;
  right: 6rpx;
  width: 122rpx;
  height: 122rpx;
  transform: rotate(8deg);
}

.visual-card-product {
  right: 22rpx;
  bottom: 74rpx;
  width: 116rpx;
  height: 120rpx;
  transform: rotate(5deg);
}

.visual-card-stage {
  left: 0;
  bottom: 34rpx;
  width: 168rpx;
  height: 106rpx;
  transform: rotate(-4deg);
}

.ai-badge {
  position: absolute;
  top: 0;
  right: 20rpx;
  display: grid;
  place-items: center;
  width: 58rpx;
  height: 58rpx;
  border-radius: 18rpx;
  color: $violet;
  font-size: 23rpx;
  font-weight: 950;
  background: rgba(255, 255, 255, 0.96);
  box-shadow: 0 16rpx 34rpx rgba(67, 83, 140, 0.14);
}

.feature-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 14rpx;
  margin-top: 20rpx;
}

.feature-item {
  min-width: 0;
  min-height: 184rpx;
  padding: 18rpx 14rpx 16rpx;
  border: 1rpx solid rgba(255, 255, 255, 0.88);
  border-radius: 26rpx;
  background: rgba(255, 255, 255, 0.72);
  box-shadow: $shadow-card;
}

.feature-icon {
  display: grid;
  place-items: center;
  width: 44rpx;
  height: 44rpx;
  border-radius: 15rpx;
  color: $violet;
  font-size: 24rpx;
  font-weight: 950;
  background: rgba(238, 242, 255, 0.96);
}

.feature-title {
  margin-top: 16rpx;
  color: #141c2e;
  font-size: 22rpx;
  font-weight: 950;
  line-height: 1.25;
}

.feature-text {
  margin-top: 8rpx;
  color: #71809f;
  font-size: 19rpx;
  font-weight: 750;
  line-height: 1.42;
}

.campaign-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16rpx;
  margin-top: 20rpx;
}

.campaign-card {
  position: relative;
  min-width: 0;
  overflow: hidden;
  min-height: 398rpx;
  padding: 14rpx 14rpx 18rpx;
  border-radius: 30rpx;
  text-align: left;
  background: rgba(255, 255, 255, 0.78);
  box-shadow: $shadow-card;
}

.album-card {
  border: 1rpx solid rgba(255, 228, 236, 0.96);
  background: linear-gradient(180deg, rgba(255, 247, 250, 0.94), rgba(255, 255, 255, 0.78));
}

.product-card {
  border: 1rpx solid rgba(224, 235, 255, 0.96);
  background: linear-gradient(180deg, rgba(246, 250, 255, 0.94), rgba(255, 255, 255, 0.78));
}

.campaign-image {
  display: block;
  height: 188rpx;
  border-radius: 24rpx;
  background: #edf4ff;
}

.product-image {
  height: 204rpx;
}

.campaign-copy {
  min-width: 0;
  padding: 16rpx 4rpx 0;
}

.campaign-kicker {
  color: #7784a0;
  font-size: 18rpx;
  font-weight: 900;
  line-height: 1.2;
}

.album-card .campaign-kicker,
.album-card .campaign-link {
  color: #e11d48;
}

.campaign-title {
  margin-top: 9rpx;
  color: #121a2b;
  font-size: 28rpx;
  font-weight: 950;
  line-height: 1.2;
}

.campaign-text {
  min-height: 58rpx;
  margin-top: 8rpx;
  color: #6b7895;
  font-size: 20rpx;
  font-weight: 750;
  line-height: 1.45;
}

.campaign-link {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 142rpx;
  height: 50rpx;
  margin-top: 14rpx;
  border-radius: 999rpx;
  color: $accent;
  text-align: center;
  font-size: 20rpx;
  font-weight: 950;
  background: rgba(255, 255, 255, 0.86);
  box-shadow: 0 10rpx 24rpx rgba(60, 75, 124, 0.1);
}

.workflow-card {
  overflow: hidden;
  margin-top: 360rpx;
  padding: 26rpx;
  border: 1rpx solid rgba(255, 255, 255, 0.88);
  border-radius: 32rpx;
  background:
    linear-gradient(126deg, rgba(255, 255, 255, 0.94) 0%, rgba(247, 251, 255, 0.76) 44%, rgba(229, 240, 255, 0.84) 100%),
    linear-gradient(180deg, rgba(255, 255, 255, 0.78), rgba(238, 246, 255, 0.82));
  box-shadow: $shadow-soft;
}

.workflow-card::after {
  content: '';
  position: absolute;
  right: 12rpx;
  bottom: 16rpx;
  width: 228rpx;
  height: 156rpx;
  border-radius: 999rpx 999rpx 32rpx 32rpx;
  border: 1rpx solid rgba(126, 93, 255, 0.14);
  transform: rotate(-8deg);
}

.workflow-copy {
  position: relative;
  z-index: 1;
  width: 455rpx;
  max-width: 100%;
}

.workflow-kicker {
  width: max-content;
  padding: 8rpx 16rpx;
  border-radius: 999rpx;
  color: #5d6ea4;
  font-size: 19rpx;
  font-weight: 950;
  background: rgba(238, 242, 255, 0.92);
}

.workflow-title {
  margin-top: 18rpx;
  color: #121a2b;
  font-size: 34rpx;
  font-weight: 950;
  line-height: 1.22;
}

.workflow-text {
  margin-top: 12rpx;
  color: #65718d;
  font-size: 22rpx;
  font-weight: 750;
  line-height: 1.55;
}

.workflow-steps {
  position: relative;
  z-index: 1;
  flex-wrap: wrap;
  gap: 10rpx;
  margin-top: 22rpx;
}

.workflow-step {
  display: grid;
  place-items: center;
  min-width: 82rpx;
  height: 48rpx;
  padding: 0 14rpx;
  border-radius: 999rpx;
  color: #52617d;
  font-size: 20rpx;
  font-weight: 900;
  background: rgba(255, 255, 255, 0.82);
  box-shadow: 0 10rpx 24rpx rgba(58, 73, 118, 0.08);
}

.home-tabbar-spacer {
  position: relative;
  z-index: 1;
  height: calc(132rpx + env(safe-area-inset-bottom));
}

.home-local-tabbar {
  position: fixed;
  right: 24rpx;
  bottom: calc(14rpx + env(safe-area-inset-bottom));
  left: 24rpx;
  z-index: 20;
  justify-content: space-around;
  min-height: 96rpx;
  padding: 10rpx 12rpx;
  border: 1rpx solid rgba(255, 255, 255, 0.92);
  border-radius: 30rpx;
  background: rgba(255, 255, 255, 0.94);
  box-shadow: 0 -18rpx 46rpx rgba(42, 60, 107, 0.12);
  backdrop-filter: blur(20rpx);
}

.home-tabbar-item {
  position: relative;
  flex: 1;
  flex-direction: column;
  justify-content: center;
  gap: 4rpx;
  min-width: 0;
  min-height: 76rpx;
  border-radius: 22rpx;
  color: #7a86a2;
  font-size: 20rpx;
  font-weight: 900;
}

.home-tabbar-item.active {
  color: #245cff;
  background: linear-gradient(180deg, rgba(238, 243, 255, 0.96), rgba(255, 255, 255, 0.72));
}

.tabbar-icon {
  font-size: 26rpx;
  line-height: 1;
}

@media screen and (max-width: 360px) {
  .mobile-home {
    padding-right: 20rpx;
    padding-left: 20rpx;
  }

  .hero-panel {
    grid-template-columns: minmax(0, 1fr) 256rpx;
    padding-right: 18rpx;
    padding-left: 18rpx;
  }

  .hero-title {
    font-size: 48rpx;
  }

  .hero-summary {
    font-size: 21rpx;
  }

  .campaign-card {
    min-height: 384rpx;
  }

  .campaign-title {
    font-size: 25rpx;
  }
}
</style>
