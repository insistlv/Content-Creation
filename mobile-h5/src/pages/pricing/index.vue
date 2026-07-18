<script setup>
import { computed, onMounted, ref } from 'vue'
import { onLoad } from '@dcloudio/uni-app'

import { api } from '../../api/client.js'
import AnnouncementPopup from '../../components/AnnouncementPopup.vue'
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
  title: '白霖共享 AI 图片套餐',
  path: routes.pricing
})

const billingMode = ref('once')
const packages = ref([])
const customerService = ref(null)
const me = ref(null)
const loading = ref(false)
const errorMessage = ref('')
const payingPlanId = ref(null)
const paymentMessage = ref('')
const rechargeGuide = ref({
  missing_credits: 0,
  required_credits: 0,
  package_id: '',
  source: ''
})

const fallbackPlans = [
  {
    id: 'starter',
    name: '体验包',
    tag: '体验',
    points: '50 点',
    price: '10',
    theme: 'blue',
    action: '立即支付',
    features: ['支持图片生成', '支持高清下载', '私有作品库', '优先队列']
  },
  {
    id: 'creator',
    name: '入门包',
    tag: '入门',
    points: '188 点',
    price: '30',
    theme: 'green',
    action: '立即支付',
    features: ['支持图片生成', '支持高清下载', '私有作品库', '优先队列']
  },
  {
    id: 'common',
    name: '常用包',
    tag: '常用',
    points: '688 点',
    price: '100',
    theme: 'orange',
    action: '立即支付',
    features: ['支持图片生成', '支持高清下载', '私有作品库', '优先队列 / 更高并发', '商用授权']
  },
  {
    id: 'advanced',
    name: '进阶包',
    tag: '进阶',
    points: '1488 点',
    price: '198',
    theme: 'violet',
    action: '立即支付',
    features: ['支持图片生成', '支持高清下载', '私有作品库', '批量生成', '商用授权']
  },
  {
    id: 'professional',
    name: '专业包',
    tag: '推荐',
    points: '2588 点',
    price: '298',
    theme: 'rose',
    action: '立即支付',
    recommended: true,
    features: ['支持图片生成', '支持高清下载', '私有作品库', '更高优先', '商用授权']
  },
  {
    id: 'flagship',
    name: '旗舰包',
    tag: '最划算',
    points: '6188 点',
    price: '648',
    theme: 'gold',
    action: '立即支付',
    features: ['支持图片生成', '支持高清下载', '私有作品库', '最高优先', '商用授权']
  }
]

const benefits = [
  { icon: '▧', title: '图片生成', desc: '文生图 / 图生图\n风格多样，高清输出' },
  { icon: '▣', title: '作品入库', desc: '私有作品库\n安全存储，便捷管理' },
  { icon: '◇', title: '商用创作支持', desc: '部分套餐含商用授权\n助力商业落地' }
]

const defaultComparisonLabels = ['点数', '图片生成', '高清下载', '私有作品库', '并发 / 优先级', '商用授权', '支付到账']
const fallbackFaqs = [
  { title: '点数可以退现吗？', url: '' },
  { title: '支付后多久到账？', url: '' },
  { title: '图片生成如何扣点？', url: '' }
]

const sourcePackages = computed(() => (packages.value.length > 0 ? packages.value : fallbackPlans))
const plans = computed(() => {
  const normalized = sourcePackages.value.map(packagePlan)
  const sorted =
    billingMode.value === 'enterprise'
      ? [...normalized].sort((left, right) => enterpriseRank(right) - enterpriseRank(left))
      : normalized
  return sorted.map(markRechargeRecommendedPlan)
})
const comparisonRows = computed(() =>
  defaultComparisonLabels.map((label) => [
    label,
    ...plans.value.map((plan) => benefitValue(plan, label))
  ])
)
const faqs = computed(() => customerService.value?.faqs?.length ? customerService.value.faqs : fallbackFaqs)
const rechargeRecommendedPackageID = computed(() => `${rechargeGuide.value.package_id || ''}`)
const rechargeGuidePlan = computed(() =>
  sourcePackages.value
    .map(packagePlan)
    .find((plan) => `${plan.id}` === rechargeRecommendedPackageID.value || `${plan.key}` === rechargeRecommendedPackageID.value) || null
)
const rechargeGuideText = computed(() => {
  const missingCredits = Number(rechargeGuide.value.missing_credits) || 0
  if (missingCredits <= 0) return ''
  const packageName = rechargeGuidePlan.value?.name || '推荐套餐'
  return `本次还差 ${missingCredits} 点，推荐购买「${packageName}」`
})

function showToast(title) {
  uni.showToast({ title, icon: 'none' })
}

function numericQueryValue(value) {
  const parsed = Number(value)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 0
}

function applyRechargeGuide(query = {}) {
  rechargeGuide.value = {
    missing_credits: numericQueryValue(query.missing_credits),
    required_credits: numericQueryValue(query.required_credits),
    package_id: `${query.package_id || ''}`.trim(),
    source: `${query.source || ''}`.trim()
  }
}

function themeForPackage(item, index) {
  const theme = `${item.theme || ''}`.toLowerCase()
  if (['blue', 'green', 'orange', 'violet', 'rose', 'gold'].includes(theme)) return theme
  if (index >= 5) return 'gold'
  if (index === 4 || item.recommended) return 'rose'
  if (index === 3) return 'violet'
  if (index === 2) return 'orange'
  if (index === 1) return 'green'
  return 'blue'
}

function iconForPackage(item, index) {
  const name = `${item.name || ''}`
  if (name.includes('旗舰') || index >= 5) return 'pricing'
  if (name.includes('专业') || index === 4) return 'favorite'
  if (name.includes('进阶') || index === 3) return 'custom'
  if (name.includes('常用') || index === 2) return 'generate'
  if (name.includes('入门') || index === 1) return 'image'
  return 'logo-star'
}

function packagePlan(item, index) {
  const credits = Number(item.credits) || 0
  const name = item.name || '创作套餐'
  const features = Array.isArray(item.features) && item.features.length > 0 ? item.features : ['支持图片生成', '支持高清下载', '私有作品库', '支付成功自动到账']
  const priceLabel = item.price_label || (item.price ? `¥${item.price}` : item.price || '联系购买')
  const points = item.points || `${credits} 点`
  return {
    ...item,
    id: item.id || item.key || name,
    key: item.key || `${item.id || name}`,
    iconSrc: staticIcon(iconForPackage(item, index)),
    name,
    tag: item.badge || (item.recommended ? '推荐' : '套餐'),
    points,
    priceLabel,
    theme: themeForPackage(item, index),
    action: '立即支付',
    description: item.description || '适合移动端创作、作品入库与自动到账处理',
    recommended: Boolean(item.recommended),
    features,
    benefits: Array.isArray(item.benefits) ? item.benefits : []
  }
}

function isRechargeRecommendedPlan(plan) {
  const packageID = rechargeRecommendedPackageID.value
  if (!packageID) return false
  return `${plan.id}` === packageID || `${plan.key}` === packageID
}

function markRechargeRecommendedPlan(plan) {
  if (!isRechargeRecommendedPlan(plan)) return plan
  return {
    ...plan,
    recommended: true,
    rechargeRecommended: true
  }
}

function enterpriseRank(plan) {
  const credits = Number(plan.credits) || 0
  const name = `${plan.name || ''}`
  if (name.includes('团队') || name.includes('企业')) return credits + 10000
  if (credits >= 300) return credits + 5000
  return credits
}

function benefitValue(plan, label) {
  if (label === '点数') return plan.points
  const configured = plan.benefits.find((item) => item?.label === label)
  if (configured?.value) return configured.value
  if (label === '商用授权') return plan.features.some((feature) => `${feature}`.includes('商用')) ? '√' : '－'
  if (label.includes('并发')) return plan.features.some((feature) => `${feature}`.includes('更高')) ? '更高优先' : '优先'
  if (label === '支付到账') return '自动到账'
  return plan.features.some((feature) => `${feature}`.includes(label.replace('私有', ''))) ? '√' : '√'
}

async function selectPlan(plan) {
  if (payingPlanId.value) return
  payingPlanId.value = plan.id
  paymentMessage.value = ''
  errorMessage.value = ''
  try {
    const user = await ensureWechatAccount()
    if (!user) return
    await payWechatVirtualPlan(plan, user, 1)
  } catch (error) {
    if (isPaymentCancel(error)) {
      paymentMessage.value = '支付已取消'
      showToast('支付已取消')
    } else {
      errorMessage.value = paymentErrorText(error)
    }
  } finally {
    payingPlanId.value = null
  }
}

async function payWechatVirtualPlan(plan, user, attempt, staleOrderNumber = '') {
  const paymentCode = await uniLogin()
  const input = {
    package_id: plan.id,
    code: paymentCode
  }
  if (attempt > 1 && staleOrderNumber) {
    Object.assign(input, {
      force_new: true,
      stale_order_number: staleOrderNumber,
      stale_reason: 'ORDER_CLOSED'
    })
  }

  const payload = await api.createWechatVirtualPayOrder(input)
  if (payload?.payment_state === 'already_paid') {
    handleWechatVirtualPaymentResult(payload, user)
    return
  }

  try {
    await requestWechatVirtualPayment(payload?.payment_params || {})
  } catch (error) {
    if (isPaymentCancel(error)) throw error
    if (isWechatOrderClosed(error) && attempt < 2) {
      return payWechatVirtualPlan(plan, user, attempt + 1, payload?.order?.order_number)
    }
    if (isWechatOrderClosed(error)) {
      throw new Error('订单状态已刷新失败，请重新点击支付或联系客服')
    }
    throw error
  }

  const result = await api.confirmWechatVirtualPayOrder(payload?.order?.order_number)
  handleWechatVirtualPaymentResult(result, user)
}

function handleWechatVirtualPaymentResult(result, user) {
  me.value = {
    ...(me.value || user),
    available_credits: result?.available_credits ?? me.value?.available_credits
  }
  if (result?.code === 'wechat_virtual_pay_pending') {
    paymentMessage.value = '支付处理中，请稍后刷新'
    showToast('支付处理中')
  } else if (result?.order?.payment_status === 'paid' || result?.payment_state === 'already_paid') {
    paymentMessage.value = '支付成功，点数已到账'
    showToast('支付成功，点数已到账')
  } else {
    paymentMessage.value = '支付处理中，请稍后刷新'
    showToast('支付处理中')
  }
}

function uniLogin() {
  return new Promise((resolve, reject) => {
    uni.login({
      provider: 'weixin',
      success(result) {
        if (result?.code) {
          resolve(result.code)
          return
        }
        reject(new Error('微信登录失败'))
      },
      fail(error) {
        reject(error)
      }
    })
  })
}

async function ensureWechatAccount() {
  let user = me.value
  if (!user) {
    const code = await uniLogin()
    user = await api.wechatLogin({ code })
    me.value = user
    return user
  }
  if (!user.wechat_openid_bound) {
    const code = await uniLogin()
    user = await api.wechatBind({ code })
    me.value = user
  }
  return user
}

function requestWechatVirtualPayment(params) {
  return new Promise((resolve, reject) => {
    wx.requestVirtualPayment({
      mode: params?.mode || 'short_series_goods',
      signData: `${params?.signData || ''}`,
      paySig: `${params?.paySig || ''}`,
      signature: `${params?.signature || ''}`,
      success: resolve,
      fail: reject
    })
  })
}

function isPaymentCancel(error) {
  return `${error?.errMsg || error?.message || ''}`.includes('cancel')
}

function isWechatOrderClosed(error) {
  const message = `${error?.errMsg || error?.message || error?.code || ''}`.toUpperCase()
  return message.includes('ORDER_CLOSED') || message.includes('ORDER CLOSED')
}

function paymentErrorText(error) {
  const wechatError = `${error?.errMsg || ''}`.trim()
  if (error?.code === 'wechat_virtual_pay_not_configured') return '微信虚拟支付暂未开通，请联系客服'
  if (error?.code === 'wechat_virtual_product_required') return '套餐未配置微信虚拟支付道具，请联系客服'
  if (error?.code === 'wechat_virtual_query_failed') return '支付结果查询失败，请稍后刷新或联系客服'
  if (error?.code === 'wechat_virtual_order_closed') return '微信订单已关闭，请重新下单'
  if (error?.code === 'wechat_virtual_order_refunded') return '微信订单已退款，请联系客服'
  if (error?.code === 'wechat_amount_mismatch') return '微信订单金额不一致，请联系客服'
  if (error?.code === 'wechat_openid_required') return '微信登录状态异常，请重试'
  if (error?.code === 'wechat_openid_mismatch') return '当前账号绑定的微信与支付微信不一致，请重新登录'
  if (wechatError) return `微信支付失败：${wechatError}`
  return error?.message || '支付失败，请重试或联系客服'
}

function contactService() {
  navigateTo(routes.support)
}

function copyServiceAccount() {
  const account = customerService.value?.wechat?.account || customerService.value?.qq?.account || ''
  if (!account) {
    showToast('暂无客服账号')
    return
  }
  uni.setClipboardData({
    data: account,
    success() {
      showToast('客服账号已复制')
    }
  })
}

function openFaq(faq) {
  const url = `${faq?.url || ''}`.trim()
  if (!url) return
  if (url.includes('pricing')) return
  if (url.includes('works')) {
    navigateTo(routes.works)
    return
  }
  if (url.includes('contact') || url.includes('support')) {
    contactService()
  }
}

async function loadPricing() {
  loading.value = true
  errorMessage.value = ''
  try {
    const [packagePayload, supportPayload, user] = await Promise.all([
      api.getPackages(),
      api.getCustomerService(),
      api.getMe().catch(() => null)
    ])
    packages.value = Array.isArray(packagePayload?.items) ? packagePayload.items : []
    customerService.value = supportPayload || null
    me.value = user
  } catch (error) {
    errorMessage.value = error.message || '套餐信息读取失败'
  } finally {
    loading.value = false
  }
}

onLoad((query) => {
  applyRechargeGuide(query)
})

onMounted(loadPricing)
</script>

<template>
  <view class="pricing-page">
    <view class="app-shell">
      <view class="topbar">
        <view class="brand">
          <image class="brand-icon" :src="icon('logo-star')" mode="aspectFit" />
          <view>
            <text class="brand-name">白霖共享</text>
            <text class="brand-subtitle">创作者 AI 图片平台</text>
          </view>
        </view>
        <button class="notification-button" type="button" @click="contactService">
          <view class="bell-icon">
            <view></view>
          </view>
        </button>
      </view>

      <view class="hero-copy">
        <text class="page-title">选择适合你的<text>创作套餐</text></text>
        <text class="page-subtitle">适用于图片生成、内容创作与商业创意的全场景工作流</text>
      </view>

      <view class="billing-toggle">
        <button type="button" :class="{ active: billingMode === 'once' }" @click="billingMode = 'once'">一次购买</button>
        <button type="button" :class="{ active: billingMode === 'enterprise' }" @click="billingMode = 'enterprise'">企业方案</button>
      </view>

      <view v-if="loading" class="state-strip">套餐信息读取中...</view>
      <view v-else-if="errorMessage" class="state-strip error">{{ errorMessage }}</view>
      <view v-else-if="paymentMessage" class="state-strip">{{ paymentMessage }}</view>
      <view v-if="rechargeGuideText" class="state-strip recharge-guide">{{ rechargeGuideText }}</view>

      <view class="pricing-list">
        <view
          v-for="plan in plans"
          :key="plan.id"
          class="pricing-card"
          :class="[plan.theme, { recommended: plan.recommended, 'recharge-recommended': plan.rechargeRecommended }]"
        >
          <view v-if="plan.recommended" class="recommend-ribbon">{{ plan.rechargeRecommended ? '本次推荐' : '✦ 推荐' }}</view>
          <view class="plan-main">
            <view class="plan-badge">
              <image class="plan-badge-icon" :src="plan.iconSrc" mode="aspectFit" />
            </view>
            <view class="plan-copy">
              <view class="plan-title-line">
                <text class="plan-name">{{ plan.name }}</text>
                <text class="plan-tag">{{ plan.tag }}</text>
              </view>
              <text class="plan-points">{{ plan.points }}</text>
            </view>
            <view class="price-block">
              <text>{{ plan.priceLabel }}</text>
            </view>
            <button type="button" class="select-button" :disabled="Boolean(payingPlanId)" @click="selectPlan(plan)">
              {{ payingPlanId === plan.id ? '支付中...' : '立即支付' }}
            </button>
          </view>
          <view class="feature-row">
            <view v-for="feature in plan.features" :key="feature" class="feature-item">
              <text>✓</text>
              <text>{{ feature }}</text>
            </view>
          </view>
        </view>
      </view>

      <view class="benefit-grid">
        <view v-for="item in benefits" :key="item.title" class="benefit-card">
          <view class="benefit-icon">{{ item.icon }}</view>
          <view>
            <text class="benefit-title">{{ item.title }}</text>
            <text class="benefit-desc">{{ item.desc }}</text>
          </view>
        </view>
      </view>

      <view class="comparison-panel">
        <view class="section-head">
          <text>权益对比</text>
          <button type="button" @click="contactService">查看完整对比表 ›</button>
        </view>
        <view class="comparison-table">
          <view class="comparison-row header">
            <text>权益</text>
            <text v-for="plan in plans" :key="`head-${plan.id}`">{{ plan.name }}</text>
          </view>
          <view v-for="row in comparisonRows" :key="row[0]" class="comparison-row">
            <text>{{ row[0] }}</text>
            <text v-for="(value, index) in row.slice(1)" :key="`${row[0]}-${index}`">{{ value }}</text>
          </view>
        </view>
      </view>

      <view class="faq-panel">
        <text class="faq-title">常见问题</text>
        <button v-for="item in faqs" :key="item.title" type="button" @click="openFaq(item)">
          <text>{{ item.title }}</text>
          <text>{{ item.url ? '›' : '⌄' }}</text>
        </button>
      </view>

      <view class="help-strip">
        <view class="service-icon">☏</view>
        <view>
          <text>需要帮助？</text>
          <text>{{ customerService?.subtitle || '如需帮助，可查看帮助中心或联系客服解决。' }}</text>
        </view>
        <button type="button" @click="copyServiceAccount">复制客服</button>
        <button type="button" @click="contactService">联系客服</button>
      </view>

    </view>

    <AppTabbar active-key="pricing" />
    <AnnouncementPopup />

  </view>
</template>

<style lang="scss" scoped>
@use '../../styles/tokens.scss' as *;

.pricing-page {
  min-height: 100vh;
  background:
    radial-gradient(circle at 0 0, rgba(255, 203, 225, 0.78), transparent 33%),
    radial-gradient(circle at 100% 0, rgba(219, 231, 255, 0.92), transparent 36%),
    linear-gradient(180deg, #fff7fb 0%, #f8fbff 52%, #eff6ff 100%);
  color: #111b34;
}

.pricing-page,
.pricing-page view,
.pricing-page uni-view,
.pricing-page button,
.pricing-page uni-button,
.pricing-page image,
.pricing-page uni-image,
.pricing-page text {
  box-sizing: border-box;
}

.pricing-page button {
  margin: 0;
  padding: 0;
  border: 0;
  line-height: 1.2;
  overflow: visible;
}

.pricing-page button::after {
  border: 0;
}

.app-shell {
  min-height: 100vh;
  padding: calc(34rpx + $phone-float-clearance-top + env(safe-area-inset-top)) 34rpx 0;
}

.topbar,
.brand,
.billing-toggle,
.plan-main,
.plan-title-line,
.feature-row,
.feature-item,
.benefit-card,
.section-head,
.comparison-row,
.faq-panel button,
.help-strip {
  display: flex;
  align-items: center;
}

.topbar,
.section-head {
  justify-content: space-between;
}

.brand {
  gap: 16rpx;
}

.brand-icon {
  width: 58rpx;
  height: 58rpx;
}

.brand-name,
.brand-subtitle,
.page-title,
.page-subtitle,
.plan-name,
.plan-points,
.benefit-title,
.benefit-desc,
.faq-title,
.help-strip text {
  display: block;
}

.brand-name {
  color: #10182d;
  font-size: 31rpx;
  font-weight: 950;
  line-height: 1.08;
}

.brand-subtitle {
  margin-top: 8rpx;
  color: #6f7890;
  font-size: 23rpx;
  font-weight: 700;
}

.notification-button {
  display: grid;
  place-items: center;
  width: 74rpx;
  height: 74rpx;
  border-radius: 20rpx;
  background: rgba(255, 255, 255, 0.9);
  box-shadow: 0 18rpx 36rpx rgba(41, 57, 94, 0.1);
}

.bell-icon {
  position: relative;
  width: 30rpx;
  height: 34rpx;
}

.bell-icon::before {
  content: '';
  position: absolute;
  left: 5rpx;
  top: 3rpx;
  width: 20rpx;
  height: 22rpx;
  border: 3rpx solid #111b34;
  border-bottom: 0;
  border-radius: 15rpx 15rpx 8rpx 8rpx;
}

.bell-icon::after {
  content: '';
  position: absolute;
  left: 2rpx;
  right: 2rpx;
  bottom: 6rpx;
  height: 3rpx;
  border-radius: 999rpx;
  background: #111b34;
}

.bell-icon view {
  position: absolute;
  left: 12rpx;
  bottom: 0;
  width: 6rpx;
  height: 6rpx;
  border-radius: 50%;
  background: #111b34;
}

.hero-copy {
  margin-top: 42rpx;
}

.page-title {
  color: #0f1b34;
  font-size: 40rpx;
  font-weight: 950;
  line-height: 1.16;
}

.page-title text {
  display: inline;
  color: #5d64ff;
}

.page-subtitle {
  margin-top: 18rpx;
  color: #68738d;
  font-size: 22rpx;
  font-weight: 750;
  line-height: 1.45;
}

.billing-toggle {
  width: 380rpx;
  max-width: 100%;
  height: 56rpx;
  margin: 28rpx auto 0;
  padding: 5rpx;
  border-radius: 999rpx;
  background: rgba(255, 255, 255, 0.92);
  box-shadow: inset 0 0 0 1rpx rgba(143, 154, 177, 0.1);
}

.billing-toggle button {
  display: flex;
  align-items: center;
  justify-content: center;
  flex: 1;
  height: 46rpx;
  border-radius: 999rpx;
  background: transparent;
  color: #172033;
  font-size: 21rpx;
  font-weight: 950;
}

.billing-toggle button.active {
  background: linear-gradient(135deg, #8c4dff 0%, #236cff 100%);
  color: #fff;
  box-shadow: 0 12rpx 26rpx rgba(75, 90, 235, 0.3);
}

.state-strip {
  margin-top: 18rpx;
  padding: 16rpx 20rpx;
  border-radius: 16rpx;
  background: rgba(255, 255, 255, 0.88);
  color: #315cff;
  font-size: 22rpx;
  font-weight: 850;
}

.state-strip.error {
  color: #b91c1c;
  background: #fef2f2;
}

.state-strip.recharge-guide {
  color: #2443b8;
  background: rgba(235, 240, 255, 0.92);
  box-shadow: inset 0 0 0 1rpx rgba(75, 100, 235, 0.12);
}

.pricing-list {
  display: grid;
  gap: 18rpx;
  margin-top: 20rpx;
}

.pricing-card {
  position: relative;
  width: 100%;
  min-width: 0;
  padding: 26rpx 22rpx 22rpx;
  border-radius: 22rpx;
  background: rgba(255, 255, 255, 0.9);
  box-shadow: 0 18rpx 46rpx rgba(31, 45, 82, 0.06);
}

.pricing-card.recommended {
  border: 2rpx solid #755cff;
}

.pricing-card.recharge-recommended {
  border-color: #245cff;
  background: rgba(248, 250, 255, 0.98);
  box-shadow:
    0 18rpx 46rpx rgba(31, 45, 82, 0.07),
    0 0 0 4rpx rgba(36, 92, 255, 0.08);
}

.recommend-ribbon {
  position: absolute;
  left: 24rpx;
  top: -2rpx;
  height: 36rpx;
  padding: 0 18rpx;
  border-radius: 0 0 8rpx 8rpx;
  background: linear-gradient(135deg, #8b4cff, #5d6bff);
  color: #fff;
  font-size: 20rpx;
  font-weight: 950;
  line-height: 36rpx;
}

.plan-main {
  gap: 18rpx;
}

.plan-badge {
  display: grid;
  place-items: center;
  flex: 0 0 auto;
  width: 64rpx;
  height: 64rpx;
  border-radius: 50%;
  font-size: 30rpx;
  font-weight: 950;
}

.plan-badge-icon {
  width: 34rpx;
  height: 34rpx;
}

.pricing-card.blue .plan-badge {
  background: #e9edff;
  color: #315cff;
}

.pricing-card.green .plan-badge {
  background: #e0f7f2;
  color: #179b82;
}

.pricing-card.orange .plan-badge {
  background: #fff1d9;
  color: #e8800b;
}

.pricing-card.violet .plan-badge {
  background: #ede9fe;
  color: #7c3aed;
}

.pricing-card.rose .plan-badge {
  background: #ffe4ec;
  color: #e11d48;
}

.pricing-card.gold .plan-badge {
  background: #fef3c7;
  color: #b7791f;
}

.plan-copy {
  flex: 1;
  min-width: 0;
}

.plan-title-line {
  gap: 10rpx;
}

.plan-name {
  color: #111b34;
  font-size: 30rpx;
  font-weight: 950;
}

.plan-tag {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  min-width: 54rpx;
  height: 36rpx;
  padding: 6rpx 10rpx;
  border-radius: 8rpx;
  background: rgba(121, 86, 255, 0.1);
  color: #7657ff;
  font-size: 19rpx;
  font-weight: 950;
}

.plan-points {
  margin-top: 7rpx;
  color: #59647e;
  font-size: 22rpx;
  font-weight: 850;
}

.price-block {
  display: flex;
  align-items: baseline;
  flex: 0 0 auto;
  color: #111b34;
  font-size: 32rpx;
  font-weight: 950;
  line-height: 1;
}

.price-block .currency {
  margin-right: 4rpx;
  font-size: 24rpx;
}

.select-button {
  display: flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  width: 172rpx;
  height: 54rpx;
  border-radius: 999rpx;
  background: linear-gradient(135deg, #8c4dff 0%, #236cff 100%);
  color: #fff;
  font-size: 20rpx;
  font-weight: 950;
  box-shadow: 0 14rpx 28rpx rgba(75, 90, 235, 0.28);
  line-height: 1;
  text-align: center;
  white-space: nowrap;
}

.feature-row {
  flex-wrap: wrap;
  gap: 18rpx 26rpx;
  margin-top: 24rpx;
}

.feature-item {
  gap: 8rpx;
  min-width: 154rpx;
  color: #3f4962;
  font-size: 19rpx;
  font-weight: 850;
}

.feature-item text:first-child {
  display: grid;
  place-items: center;
  width: 22rpx;
  height: 22rpx;
  border-radius: 50%;
  background: #6d63ff;
  color: #fff;
  font-size: 15rpx;
}

.pricing-card.green .feature-item text:first-child {
  background: #209a87;
}

.pricing-card.orange .feature-item text:first-child {
  background: #e88410;
}

.pricing-card.violet .feature-item text:first-child {
  background: #7c3aed;
}

.pricing-card.rose .feature-item text:first-child {
  background: #e11d48;
}

.pricing-card.gold .feature-item text:first-child {
  background: #b7791f;
}

.benefit-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 14rpx;
  margin-top: 20rpx;
}

.benefit-card {
  gap: 14rpx;
  min-width: 0;
  padding: 20rpx 16rpx;
  border-radius: 18rpx;
  background: rgba(255, 255, 255, 0.86);
  box-shadow: 0 14rpx 34rpx rgba(31, 45, 82, 0.05);
}

.benefit-icon {
  display: grid;
  place-items: center;
  flex: 0 0 auto;
  width: 50rpx;
  height: 50rpx;
  border-radius: 14rpx;
  background: rgba(121, 86, 255, 0.1);
  color: #6d57ff;
  font-size: 28rpx;
  font-weight: 950;
}

.benefit-title {
  color: #121b33;
  font-size: 21rpx;
  font-weight: 950;
}

.benefit-desc {
  margin-top: 5rpx;
  color: #65718c;
  font-size: 17rpx;
  font-weight: 750;
  line-height: 1.35;
  white-space: pre-line;
}

.comparison-panel,
.faq-panel,
.help-strip {
  margin-top: 18rpx;
  border-radius: 22rpx;
  background: rgba(255, 255, 255, 0.88);
  box-shadow: 0 16rpx 38rpx rgba(31, 45, 82, 0.05);
}

.comparison-panel {
  padding: 22rpx;
}

.section-head text,
.faq-title {
  color: #111b34;
  font-size: 26rpx;
  font-weight: 950;
}

.section-head button {
  display: flex;
  align-items: center;
  justify-content: center;
  color: #68738d;
  font-size: 18rpx;
  font-weight: 850;
}

.comparison-table {
  margin-top: 16rpx;
  overflow: hidden;
  border: 1rpx solid rgba(143, 154, 177, 0.16);
  border-radius: 14rpx;
}

.comparison-row {
  min-height: 43rpx;
  border-bottom: 1rpx solid rgba(143, 154, 177, 0.12);
}

.comparison-row:last-child {
  border-bottom: 0;
}

.comparison-row text {
  flex: 1;
  min-width: 0;
  padding: 0 12rpx;
  color: #4b5873;
  font-size: 17rpx;
  font-weight: 850;
  text-align: center;
}

.comparison-row text:first-child {
  text-align: left;
  color: #172033;
}

.comparison-row text:last-child {
  align-self: stretch;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(116, 92, 255, 0.08);
  color: #6257ff;
  font-weight: 950;
}

.comparison-row.header text {
  color: #5b6680;
  font-weight: 950;
}

.faq-panel {
  padding: 20rpx 22rpx;
}

.faq-panel button {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  min-height: 48rpx;
  border-bottom: 1rpx solid rgba(143, 154, 177, 0.12);
  color: #172033;
  font-size: 21rpx;
  font-weight: 900;
}

.faq-panel button:last-child {
  border-bottom: 0;
}

.help-strip {
  gap: 18rpx;
  padding: 20rpx 22rpx;
}

.service-icon {
  display: grid;
  place-items: center;
  flex: 0 0 auto;
  width: 54rpx;
  height: 54rpx;
  border-radius: 50%;
  background: rgba(98, 87, 255, 0.1);
  color: #6257ff;
  font-size: 28rpx;
  font-weight: 950;
}

.help-strip view:nth-child(2) {
  flex: 1;
  min-width: 0;
}

.help-strip text:first-child {
  color: #111b34;
  font-size: 22rpx;
  font-weight: 950;
}

.help-strip text:last-child {
  margin-top: 6rpx;
  color: #68738d;
  font-size: 18rpx;
  font-weight: 750;
}

.help-strip button {
  display: flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  width: 122rpx;
  height: 46rpx;
  border: 2rpx solid #755cff;
  border-radius: 14rpx;
  background: transparent;
  color: #6257ff;
  font-size: 20rpx;
  font-weight: 950;
  line-height: 1;
  text-align: center;
  white-space: nowrap;
}

.modal-backdrop {
  position: fixed;
  z-index: 50;
  left: 0;
  right: 0;
  top: 0;
  bottom: 0;
  display: flex;
  align-items: flex-end;
  padding: 28rpx;
  background: rgba(15, 23, 42, 0.42);
}

.purchase-modal {
  width: 100%;
  max-height: 86vh;
  padding: 18rpx 22rpx 26rpx;
  overflow-y: auto;
  border-radius: 28rpx 28rpx 22rpx 22rpx;
  background: #fff;
  box-shadow: 0 -20rpx 60rpx rgba(15, 23, 42, 0.18);
}

.drag-handle {
  width: 72rpx;
  height: 8rpx;
  margin: 0 auto 20rpx;
  border-radius: 999rpx;
  background: #dbe3ef;
}

.modal-head,
.success-panel view {
  display: flex;
  align-items: center;
}

.modal-head {
  justify-content: space-between;
  gap: 18rpx;
}

.modal-title,
.modal-subtitle,
.success-panel text {
  display: block;
}

.modal-title {
  color: #111b34;
  font-size: 30rpx;
  font-weight: 950;
}

.modal-subtitle {
  margin-top: 7rpx;
  color: #667085;
  font-size: 22rpx;
  font-weight: 800;
}

.modal-head button {
  display: grid;
  place-items: center;
  width: 52rpx;
  height: 52rpx;
  border-radius: 50%;
  background: #f1f5f9;
  color: #334155;
  font-size: 34rpx;
  font-weight: 900;
}

.purchase-form {
  display: grid;
  gap: 14rpx;
  margin-top: 20rpx;
}

.purchase-form input,
.purchase-form textarea {
  width: 100%;
  min-width: 0;
  border: 1rpx solid rgba(148, 163, 184, 0.22);
  border-radius: 16rpx;
  background: #f8fafc;
  color: #111827;
  font-size: 24rpx;
  font-weight: 750;
}

.purchase-form input {
  height: 68rpx;
  padding: 0 18rpx;
}

.purchase-form textarea {
  min-height: 112rpx;
  padding: 18rpx;
  line-height: 1.45;
}

.contact-type-row {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10rpx;
}

.contact-type-row button,
.submit-intent-button,
.success-panel button {
  display: flex;
  align-items: center;
  justify-content: center;
}

.contact-type-row button {
  height: 54rpx;
  border-radius: 14rpx;
  background: #eef2ff;
  color: #475569;
  font-size: 20rpx;
  font-weight: 900;
}

.contact-type-row button.active {
  background: #111827;
  color: #fff;
}

.submit-intent-button {
  height: 76rpx;
  border-radius: 18rpx;
  background: linear-gradient(135deg, #8c4dff 0%, #236cff 100%);
  color: #fff;
  font-size: 25rpx;
  font-weight: 950;
  box-shadow: 0 16rpx 34rpx rgba(75, 90, 235, 0.26);
}

.success-panel {
  margin-top: 20rpx;
  padding: 22rpx;
  border-radius: 18rpx;
  background: #f0fdf4;
}

.success-panel text:first-child {
  color: #166534;
  font-size: 28rpx;
  font-weight: 950;
}

.success-panel text:nth-child(2) {
  margin-top: 8rpx;
  color: #3f6212;
  font-size: 22rpx;
  font-weight: 750;
  line-height: 1.45;
}

.success-panel view {
  gap: 12rpx;
  margin-top: 18rpx;
}

.success-panel button {
  flex: 1;
  height: 62rpx;
  border-radius: 14rpx;
  background: #166534;
  color: #fff;
  font-size: 22rpx;
  font-weight: 900;
}

.success-panel button:first-child {
  background: #dcfce7;
  color: #166534;
}

@media (max-width: 390px) {
  .app-shell {
    padding-left: 24rpx;
    padding-right: 24rpx;
  }

  .page-title {
    font-size: 36rpx;
  }

  .plan-main {
    gap: 12rpx;
  }

  .plan-badge {
    width: 54rpx;
    height: 54rpx;
  }

  .plan-name {
    font-size: 26rpx;
  }

  .price-block {
    font-size: 27rpx;
  }

  .select-button {
    width: 136rpx;
    font-size: 18rpx;
  }

  .feature-item {
    min-width: 130rpx;
    font-size: 18rpx;
  }

  .benefit-grid {
    gap: 10rpx;
  }

  .benefit-card {
    padding: 16rpx 10rpx;
    gap: 10rpx;
  }

  .benefit-icon {
    width: 42rpx;
    height: 42rpx;
  }

  .benefit-title {
    font-size: 18rpx;
  }

  .benefit-desc {
    font-size: 15rpx;
  }

  .comparison-row text {
    padding: 0 7rpx;
    font-size: 15rpx;
  }

  .help-strip button {
    width: 110rpx;
    font-size: 18rpx;
  }

}
</style>
