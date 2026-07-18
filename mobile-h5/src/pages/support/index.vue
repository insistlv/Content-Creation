<script setup>
import { computed, onMounted, ref } from 'vue'

import { api } from '../../api/client.js'
import AppTabbar from '../../components/AppTabbar.vue'
import { enableMiniProgramShare, navigateTo, routes } from '../../utils/routes.js'

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

const icon = staticIcon

enableMiniProgramShare({
  title: '白霖共享客服支持',
  path: routes.support
})

const fallbackSupport = {
  title: '联系客服',
  eyebrow: 'CUSTOMER SERVICE',
  subtitle: '微信 / QQ 快速联系，移动端支持长按二维码添加微信',
  description: '如您在使用过程中遇到账户问题、充值相关、生成异常或合作咨询等需求，请随时联系我们的客服团队，我们将竭诚为您提供帮助。',
  wechat: { label: '微信客服', account: 'CCTV-DL88888', qr_url: '' },
  qq: { label: 'QQ客服', account: '874122661', qr_url: '' },
  service_tags: ['账号问题', '充值咨询', '作品下载', '模型使用', '合作咨询', '售后支持'],
  stats: [
    { label: '在线时间', value: '09:00 - 22:00' },
    { label: '平均响应', value: '5 分钟内' },
    { label: '服务范围', value: '账号 / 充值 / 作品 / 合作' }
  ],
  features: [
    { title: '快速响应', text: '客服团队在线服务，协助处理移动端创作问题' },
    { title: '多渠道联系', text: '微信 / QQ 双渠道支持，按当前场景选择即可' },
    { title: '人工充值支持', text: '套餐购买意向提交后，可由客服继续确认到账' }
  ],
  faqs: [
    { title: '充值未到账怎么办？', url: '/pricing' },
    { title: '作品无法下载怎么办？', url: '/works' },
    { title: '如何联系人工客服？', url: '' }
  ]
}

const support = ref(fallbackSupport)
const loading = ref(false)
const errorMessage = ref('')

const wechatChannel = computed(() => ({
  ...fallbackSupport.wechat,
  ...(support.value.wechat || {})
}))
const qqChannel = computed(() => ({
  ...fallbackSupport.qq,
  ...(support.value.qq || {})
}))
const channels = computed(() => [
  { key: 'wechat', ...wechatChannel.value },
  { key: 'qq', ...qqChannel.value }
])
const serviceTags = computed(() =>
  Array.isArray(support.value.service_tags) && support.value.service_tags.length > 0
    ? support.value.service_tags
    : fallbackSupport.service_tags
)
const stats = computed(() =>
  Array.isArray(support.value.stats) && support.value.stats.length > 0 ? support.value.stats : fallbackSupport.stats
)
const faqs = computed(() =>
  Array.isArray(support.value.faqs) && support.value.faqs.length > 0 ? support.value.faqs : fallbackSupport.faqs
)

function showToast(title) {
  uni.showToast({ title, icon: 'none' })
}

function qr(channel) {
  return channel?.qr_url || channel?.qrcode_url || ''
}

function copyServiceAccount(account, title = '客服账号已复制') {
  const text = `${account || ''}`.trim()
  if (!text) {
    showToast('暂无客服账号')
    return
  }
  uni.setClipboardData({
    data: text,
    success() {
      showToast(title)
    }
  })
}

function serviceTagIcon(tag) {
  const text = `${tag}`
  if (text.includes('账号')) return '♟'
  if (text.includes('充值')) return '⊕'
  if (text.includes('下载') || text.includes('作品')) return '⇩'
  if (text.includes('模型')) return '□'
  if (text.includes('合作')) return '◇'
  return '♧'
}

function faqURL(faq) {
  return `${faq?.url || faq?.URL || ''}`.trim()
}

function openFAQ(faq) {
  const url = faqURL(faq)
  if (!url) return
  if (/^https?:\/\//i.test(url)) {
    if (typeof window !== 'undefined') {
      window.open(url, '_blank')
    } else {
      copyServiceAccount(url, '链接已复制')
    }
    return
  }
  const normalized = url.startsWith('/') ? url : `/${url}`
  const localRoutes = {
    '/': routes.home,
    '/home': routes.home,
    '/pricing': routes.pricing,
    '/packages': routes.pricing,
    '/works': routes.works,
    '/account': routes.account,
    '/contact': routes.support,
    '/support': routes.support
  }
  navigateTo(localRoutes[normalized] || routes.support)
}

function goBack() {
  const pages = getCurrentPages()
  if (pages.length > 1) {
    uni.navigateBack()
    return
  }
  navigateTo(routes.home)
}

async function loadSupport() {
  loading.value = true
  errorMessage.value = ''
  try {
    const payload = await api.getCustomerService()
    support.value = {
      ...fallbackSupport,
      ...payload,
      wechat: { ...fallbackSupport.wechat, ...(payload?.wechat || {}) },
      qq: { ...fallbackSupport.qq, ...(payload?.qq || {}) }
    }
  } catch (error) {
    errorMessage.value = error.message || '客服信息读取失败'
  } finally {
    loading.value = false
  }
}

onMounted(loadSupport)
</script>

<template>
  <view class="support-page">
    <view class="app-shell">
      <view class="hero-copy">
        <text class="eyebrow">✦ {{ support.eyebrow }}</text>
        <text class="page-title">{{ support.title }}</text>
        <text class="page-subtitle">{{ support.subtitle }}</text>
        <text class="page-desc">{{ support.description }}</text>
      </view>

      <view v-if="loading" class="state-strip">客服信息读取中...</view>
      <view v-else-if="errorMessage" class="state-strip error">{{ errorMessage }}</view>

      <view class="quick-contact-grid">
        <button type="button" class="wechat-action" @click="copyServiceAccount(wechatChannel.account, '微信号已复制')">
          <text>●●</text>
          <text>复制微信号</text>
        </button>
        <button type="button" class="qq-action" @click="copyServiceAccount(qqChannel.account, 'QQ号已复制')">
          <text>♟</text>
          <text>复制QQ号</text>
        </button>
      </view>

      <view class="service-tag-grid">
        <button v-for="tag in serviceTags" :key="tag" type="button" @click="copyServiceAccount(wechatChannel.account, '微信号已复制')">
          <text>{{ serviceTagIcon(tag) }}</text>
          <text>{{ tag }}</text>
        </button>
      </view>

      <view class="support-channel-grid">
        <view v-for="channel in channels" :key="channel.key" class="channel-card">
          <view class="channel-title">
            <text>{{ channel.key === 'wechat' ? '○' : '♙' }}</text>
            <text>{{ channel.label || (channel.key === 'wechat' ? '微信客服' : 'QQ客服') }}</text>
          </view>

          <view v-if="qr(channel)" class="qr-card" :class="channel.key">
            <image :src="qr(channel)" mode="aspectFit" />
          </view>
          <view v-else class="qr-card qr-placeholder" :class="channel.key">
            <image :src="icon('support')" mode="aspectFit" />
            <text>暂无二维码</text>
          </view>

          <text class="qr-hint">{{ channel.key === 'wechat' ? '移动端可长按识别二维码添加微信' : '扫码添加 QQ 客服' }}</text>
          <text class="account-line">
            {{ channel.key === 'wechat' ? '微信号：' : 'QQ：' }}{{ channel.account || '暂未配置' }}
          </text>
          <button type="button" class="copy-button" @click="copyServiceAccount(channel.account, channel.key === 'wechat' ? '微信号已复制' : 'QQ号已复制')">
            <text>▢</text>
            <text>{{ channel.key === 'wechat' ? '复制微信号' : '复制QQ号' }}</text>
          </button>
        </view>
      </view>

      <view class="stats-strip">
        <view v-for="item in stats" :key="item.label">
          <text>{{ item.label === '在线时间' ? '♧' : item.label === '平均响应' ? 'ϟ' : '♢' }}</text>
          <view>
            <text>{{ item.label }}</text>
            <text>{{ item.value }}</text>
          </view>
        </view>
      </view>

      <view class="faq-panel">
        <text class="section-title">常见问题</text>
        <button v-for="faq in faqs" :key="faq.title" type="button" @click="openFAQ(faq)">
          <text>{{ faq.title }}</text>
          <text>{{ faqURL(faq) ? '›' : '—' }}</text>
        </button>
      </view>

    </view>

    <AppTabbar active-key="" />
  </view>
</template>

<style lang="scss" scoped>
@use '../../styles/tokens.scss' as *;

.support-page {
  min-height: 100vh;
  background:
    radial-gradient(circle at 0 0, rgba(219, 234, 254, 0.86), transparent 34%),
    radial-gradient(circle at 100% 0, rgba(209, 250, 229, 0.76), transparent 32%),
    linear-gradient(180deg, #f8fbff 0%, #eef8f3 56%, #f8fafc 100%);
  color: #111827;
}

.support-page,
.support-page view,
.support-page button,
.support-page image,
.support-page text {
  box-sizing: border-box;
}

.support-page button {
  margin: 0;
  padding: 0;
  border: 0;
  line-height: 1.2;
}

.support-page button::after {
  border: 0;
}

.app-shell {
  min-height: 100vh;
  padding: calc(26rpx + $phone-float-clearance-top + env(safe-area-inset-top)) 28rpx 54rpx;
}

.topbar,
.brand,
.channel-main,
.tag-row,
.stats-grid,
.faq-panel button {
  display: flex;
  align-items: center;
}

.topbar {
  gap: 18rpx;
}

.back-button {
  display: grid;
  place-items: center;
  width: 58rpx;
  height: 58rpx;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.9);
  color: #111827;
  font-size: 44rpx;
  font-weight: 900;
  box-shadow: 0 12rpx 28rpx rgba(31, 45, 82, 0.08);
}

.brand {
  gap: 14rpx;
}

.brand-icon {
  width: 54rpx;
  height: 54rpx;
}

.brand-name,
.brand-subtitle,
.eyebrow,
.page-title,
.page-subtitle,
.page-desc,
.channel-label,
.channel-account,
.qr-card text,
.feature-list text,
.section-title {
  display: block;
}

.brand-name {
  color: #10182d;
  font-size: 30rpx;
  font-weight: 950;
  line-height: 1.08;
}

.brand-subtitle {
  margin-top: 6rpx;
  color: #64748b;
  font-size: 22rpx;
  font-weight: 750;
}

.hero-panel,
.channel-card,
.feature-list,
.faq-panel,
.state-strip {
  margin-top: 22rpx;
  border: 1rpx solid rgba(148, 163, 184, 0.18);
  border-radius: 20rpx;
  background: rgba(255, 255, 255, 0.9);
  box-shadow: 0 14rpx 34rpx rgba(31, 45, 82, 0.05);
}

.hero-panel {
  padding: 30rpx 28rpx;
}

.eyebrow {
  color: #2563eb;
  font-size: 20rpx;
  font-weight: 950;
}

.page-title {
  margin-top: 10rpx;
  color: #0f172a;
  font-size: 46rpx;
  font-weight: 950;
  line-height: 1.1;
}

.page-subtitle,
.page-desc {
  color: #526077;
  font-size: 24rpx;
  font-weight: 750;
  line-height: 1.5;
}

.page-subtitle {
  margin-top: 16rpx;
}

.page-desc {
  margin-top: 10rpx;
}

.state-strip {
  padding: 18rpx 22rpx;
  color: #2563eb;
  font-size: 24rpx;
  font-weight: 800;
}

.state-strip.error {
  color: #b91c1c;
  background: #fef2f2;
}

.tag-row {
  flex-wrap: wrap;
  gap: 12rpx;
  margin-top: 20rpx;
}

.tag-row text {
  padding: 10rpx 16rpx;
  border-radius: 999rpx;
  background: rgba(37, 99, 235, 0.09);
  color: #1d4ed8;
  font-size: 22rpx;
  font-weight: 850;
}

.stats-grid {
  gap: 12rpx;
  margin-top: 18rpx;
}

.stats-grid view {
  flex: 1;
  min-width: 0;
  padding: 18rpx 14rpx;
  border-radius: 16rpx;
  background: rgba(255, 255, 255, 0.84);
}

.stats-grid text {
  display: block;
  text-align: center;
}

.stats-grid text:first-child {
  color: #111827;
  font-size: 22rpx;
  font-weight: 950;
}

.stats-grid text:last-child {
  margin-top: 6rpx;
  color: #64748b;
  font-size: 19rpx;
  font-weight: 800;
}

.channel-list {
  display: grid;
  gap: 18rpx;
  margin-top: 20rpx;
}

.channel-card {
  padding: 22rpx;
}

.channel-main {
  gap: 16rpx;
}

.channel-icon {
  display: grid;
  place-items: center;
  width: 58rpx;
  height: 58rpx;
  border-radius: 18rpx;
  background: #eff6ff;
  color: #2563eb;
  font-size: 28rpx;
  font-weight: 950;
}

.channel-label {
  color: #0f172a;
  font-size: 27rpx;
  font-weight: 950;
}

.channel-account {
  margin-top: 6rpx;
  color: #64748b;
  font-size: 23rpx;
  font-weight: 800;
}

.channel-card button {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 64rpx;
  margin-top: 18rpx;
  border-radius: 14rpx;
  background: #111827;
  color: #fff;
  font-size: 24rpx;
  font-weight: 900;
}

.qr-card {
  margin-top: 18rpx;
  padding: 18rpx;
  border-radius: 16rpx;
  background: #f8fafc;
}

.qr-card image {
  width: 240rpx;
  height: 240rpx;
  margin: 0 auto;
  display: block;
}

.qr-card text {
  margin-top: 12rpx;
  color: #64748b;
  font-size: 21rpx;
  font-weight: 800;
  text-align: center;
}

.feature-list {
  display: grid;
  gap: 14rpx;
  padding: 22rpx;
}

.feature-list view {
  padding-bottom: 14rpx;
  border-bottom: 1rpx solid rgba(148, 163, 184, 0.14);
}

.feature-list view:last-child {
  padding-bottom: 0;
  border-bottom: 0;
}

.feature-list text:first-child {
  color: #111827;
  font-size: 25rpx;
  font-weight: 950;
}

.feature-list text:last-child {
  margin-top: 6rpx;
  color: #64748b;
  font-size: 22rpx;
  font-weight: 750;
  line-height: 1.45;
}

.faq-panel {
  padding: 22rpx;
}

.section-title {
  margin-bottom: 10rpx;
  color: #111827;
  font-size: 27rpx;
  font-weight: 950;
}

.faq-panel button {
  justify-content: space-between;
  width: 100%;
  min-height: 58rpx;
  border-bottom: 1rpx solid rgba(148, 163, 184, 0.14);
  color: #111827;
  font-size: 24rpx;
  font-weight: 850;
}

.faq-panel button:last-child {
  border-bottom: 0;
}

/* Screenshot-matched customer-service surface. */
.support-page {
  background:
    radial-gradient(circle at 8% 0, rgba(221, 231, 255, 0.86), transparent 35%),
    radial-gradient(circle at 96% 6%, rgba(224, 245, 255, 0.7), transparent 34%),
    linear-gradient(180deg, #f7faff 0%, #eef6ff 54%, #f8fafc 100%);
  color: #0f172a;
}

.app-shell {
  position: relative;
  padding: calc(20rpx + $phone-float-clearance-top + env(safe-area-inset-top)) 22rpx 34rpx;
}

.hero-copy {
  max-width: 650rpx;
  padding-top: 4rpx;
}

.eyebrow,
.page-title,
.page-subtitle,
.page-desc,
.qr-hint,
.account-line,
.section-title {
  display: block;
}

.eyebrow {
  color: #4f6bff;
  font-size: 20rpx;
  font-weight: 950;
  letter-spacing: 0;
}

.page-title {
  margin-top: 20rpx;
  color: #0f172a;
  font-size: 52rpx;
  font-weight: 950;
  line-height: 1.05;
}

.page-subtitle {
  margin-top: 22rpx;
  color: #3f63ff;
  font-size: 23rpx;
  font-weight: 950;
  line-height: 1.35;
}

.page-desc {
  margin-top: 24rpx;
  color: #475569;
  font-size: 22rpx;
  font-weight: 800;
  line-height: 1.7;
}

.quick-contact-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 20rpx;
  margin-top: 28rpx;
}

.quick-contact-grid button,
.service-tag-grid button,
.copy-button {
  display: flex;
  align-items: center;
  justify-content: center;
}

.quick-contact-grid button {
  gap: 14rpx;
  min-width: 0;
  height: 74rpx;
  border-radius: 10rpx;
  color: #fff;
  font-size: 23rpx;
  font-weight: 950;
  box-shadow: 0 16rpx 34rpx rgba(37, 99, 235, 0.18);
}

.quick-contact-grid button text:first-child {
  font-size: 26rpx;
}

.wechat-action {
  background: linear-gradient(135deg, #55d98a 0%, #24b45e 100%);
}

.qq-action {
  background: linear-gradient(135deg, #5868ff 0%, #0b79ff 100%);
}

.service-tag-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 14rpx 18rpx;
  margin-top: 24rpx;
}

.service-tag-grid button {
  gap: 10rpx;
  height: 52rpx;
  border-radius: 12rpx;
  background: rgba(255, 255, 255, 0.9);
  color: #475569;
  font-size: 19rpx;
  font-weight: 900;
  box-shadow: 0 10rpx 24rpx rgba(31, 45, 82, 0.06);
}

.service-tag-grid button text:first-child {
  color: #526bff;
  font-size: 22rpx;
}

.support-channel-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14rpx;
  margin-top: 24rpx;
}

.channel-card {
  min-width: 0;
  padding: 22rpx 14rpx 18rpx;
  border: 1rpx solid rgba(148, 163, 184, 0.18);
  border-radius: 14rpx;
  background: rgba(255, 255, 255, 0.86);
  box-shadow: 0 16rpx 38rpx rgba(31, 45, 82, 0.06);
}

.channel-title {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10rpx;
  color: #0f172a;
  font-size: 22rpx;
  font-weight: 950;
}

.channel-title text:first-child {
  color: #2563eb;
  font-size: 24rpx;
}

.qr-card {
  position: relative;
  display: grid;
  place-items: center;
  width: 100%;
  aspect-ratio: 1 / 1;
  margin-top: 22rpx;
  padding: 0;
  overflow: hidden;
  border-radius: 10rpx;
  background: #f8fafc;
}

.qr-card.wechat {
  background: linear-gradient(155deg, #0099ff 0%, #005ee8 72%);
}

.qr-card.qq {
  border: 1rpx solid rgba(148, 163, 184, 0.16);
  background: #fff;
}

.qr-card image {
  width: 82%;
  height: 82%;
}

.qr-placeholder image {
  width: 64rpx;
  height: 64rpx;
  opacity: 0.52;
}

.qr-placeholder text {
  position: absolute;
  bottom: 22rpx;
  color: #64748b;
  font-size: 18rpx;
  font-weight: 800;
}

.qr-hint {
  margin-top: 16rpx;
  color: #22c55e;
  font-size: 19rpx;
  font-weight: 900;
  line-height: 1.35;
  text-align: center;
}

.account-line {
  margin-top: 12rpx;
  color: #1e293b;
  font-size: 21rpx;
  font-weight: 950;
  text-align: center;
  word-break: break-all;
}

.copy-button {
  gap: 10rpx;
  width: 100%;
  height: 56rpx;
  margin-top: 22rpx;
  border: 1rpx solid rgba(79, 107, 255, 0.42);
  border-radius: 10rpx;
  background: #fff;
  color: #4f6bff;
  font-size: 20rpx;
  font-weight: 950;
}

.stats-strip {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  margin-top: 20rpx;
  overflow: hidden;
  border: 1rpx solid rgba(148, 163, 184, 0.16);
  border-radius: 14rpx;
  background: rgba(255, 255, 255, 0.78);
  box-shadow: 0 14rpx 34rpx rgba(31, 45, 82, 0.05);
}

.stats-strip > view {
  display: flex;
  align-items: center;
  gap: 12rpx;
  min-width: 0;
  padding: 18rpx 12rpx;
  border-right: 1rpx solid rgba(148, 163, 184, 0.16);
}

.stats-strip > view:last-child {
  border-right: 0;
}

.stats-strip > view > text {
  flex: 0 0 auto;
  color: #4f6bff;
  font-size: 30rpx;
  font-weight: 950;
}

.stats-strip view view {
  min-width: 0;
}

.stats-strip view view text {
  display: block;
}

.stats-strip view view text:first-child {
  color: #475569;
  font-size: 18rpx;
  font-weight: 850;
}

.stats-strip view view text:last-child {
  margin-top: 4rpx;
  color: #0f172a;
  font-size: 21rpx;
  font-weight: 950;
  line-height: 1.2;
}

.faq-panel {
  margin-top: 20rpx;
  padding: 20rpx 22rpx;
  border-radius: 14rpx;
  background: rgba(255, 255, 255, 0.82);
  box-shadow: 0 14rpx 34rpx rgba(31, 45, 82, 0.05);
}

.section-title {
  margin-bottom: 8rpx;
  color: #0f172a;
  font-size: 24rpx;
  font-weight: 950;
}

.faq-panel button {
  min-height: 52rpx;
  color: #1e293b;
  font-size: 22rpx;
}

@media (max-width: 390px) {
  .page-title {
    font-size: 46rpx;
  }

  .quick-contact-grid button {
    height: 68rpx;
    font-size: 21rpx;
  }

  .service-tag-grid {
    gap: 12rpx;
  }

  .channel-card {
    padding-left: 12rpx;
    padding-right: 12rpx;
  }

  .stats-strip > view {
    padding-left: 10rpx;
    padding-right: 10rpx;
  }
}
</style>
