<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { CreditCard, LayoutDashboard, RefreshCw, WalletCards } from 'lucide-vue-next'

import { api } from '../api/client.js'
import { applyAvailableCredits } from '../stores/session.js'

const route = useRoute()
const router = useRouter()
const order = ref(null)
const loading = ref(false)
const refreshing = ref(false)
const paying = ref(false)
const errorMessage = ref('')
const statusMessage = ref('')
const formHTML = ref('')
const alipayFormHost = ref(null)
const autoPolling = ref(false)
let autoPollTimer = null
let autoPollAttempts = 0
const maxAutoPollAttempts = 10
const autoPollIntervalMs = 3000

const orderNumber = computed(() => `${route.params?.order_number || route.query?.order_number || ''}`.trim())
const isReturnPage = computed(() => route.path === '/checkout/alipay/return' || Boolean(route.query?.order_number))
const snapshot = computed(() => order.value?.evidence_snapshot ?? {})
const statusText = computed(() => paymentStatusText(order.value?.payment_status))
const statusTone = computed(() => {
  if (order.value?.payment_status === 'paid') return 'paid'
  if (order.value?.payment_status === 'failed') return 'failed'
  return 'pending'
})
const canPay = computed(() => order.value?.payment_status === 'pending')

function unwrapOrder(payload) {
  return payload?.order ?? payload
}

async function loadOrder() {
  if (!orderNumber.value) {
    errorMessage.value = '订单号缺失'
    return
  }
  loading.value = true
  errorMessage.value = ''
  try {
    order.value = unwrapOrder(await api.getAlipayOrder(orderNumber.value))
    if (shouldAutoConfirmPayment()) {
      await confirmReturnPayment()
    }
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    loading.value = false
  }
}

async function startPay() {
  if (!orderNumber.value || paying.value) return
  paying.value = true
  errorMessage.value = ''
  formHTML.value = ''
  try {
    const payload = await api.payAlipayOrder(orderNumber.value)
    order.value = unwrapOrder(payload)
    formHTML.value = payload?.form_html || ''
    await nextTick()
    submitAlipayForm()
  } catch (error) {
    errorMessage.value = paymentErrorMessage(error)
  } finally {
    paying.value = false
  }
}

function submitAlipayForm() {
  const form = alipayFormHost.value?.querySelector('#auto-submit-alipay-form')
  if (!(form instanceof HTMLFormElement)) {
    throw new Error('支付宝支付页面打开失败，请稍后重试')
  }

  try {
    form.submit()
  } catch {
    throw new Error('支付宝支付页面打开失败，请稍后重试')
  }
}

async function refreshStatus() {
  if (!orderNumber.value || refreshing.value) return
  await queryPaymentStatus({ automatic: false })
}

async function queryPaymentStatus({ automatic }) {
  if (!orderNumber.value || refreshing.value) return
  refreshing.value = true
  if (!automatic) {
    errorMessage.value = ''
  }
  try {
    const payload = await api.queryAlipayOrder(orderNumber.value)
    order.value = unwrapOrder(payload)
    if (payload.available_credits !== undefined) {
      applyAvailableCredits(payload.available_credits)
    }
    if (order.value?.payment_status === 'paid') {
      statusMessage.value = '套餐购买成功，点数已到账'
      stopAutoPolling()
    } else if (automatic) {
      statusMessage.value = '支付已提交，系统正在确认到账'
      scheduleReturnPaymentPoll()
    }
  } catch (error) {
    if (automatic) {
      statusMessage.value = '支付已提交，系统正在确认到账；如长时间未到账请联系客服并提供订单号'
    } else {
      errorMessage.value = paymentErrorMessage(error)
    }
  } finally {
    refreshing.value = false
  }
}

async function confirmReturnPayment() {
  if (!orderNumber.value || order.value?.payment_status === 'paid') {
    if (order.value?.payment_status === 'paid') {
      statusMessage.value = '套餐购买成功，点数已到账'
    }
    return
  }
  statusMessage.value = '支付已提交，系统正在确认到账'
  autoPolling.value = true
  autoPollAttempts = 0
  await queryPaymentStatus({ automatic: true })
}

function scheduleReturnPaymentPoll() {
  if (!isReturnPage.value || order.value?.payment_status === 'paid') return
  if (autoPollAttempts >= maxAutoPollAttempts) {
    autoPolling.value = false
    statusMessage.value = '支付已提交，系统正在确认到账；如长时间未到账请联系客服并提供订单号'
    return
  }
  autoPolling.value = true
  autoPollAttempts += 1
  clearTimeout(autoPollTimer)
  autoPollTimer = window.setTimeout(() => {
    queryPaymentStatus({ automatic: true })
  }, autoPollIntervalMs)
}

function stopAutoPolling() {
  autoPolling.value = false
  clearTimeout(autoPollTimer)
  autoPollTimer = null
}

function paymentErrorMessage(error) {
  if (error?.code === 'alipay_not_configured') {
    return '支付通道维护中，请联系客服'
  }
  return error?.message ?? '请求失败'
}

function goAccount() {
  router.push('/account')
}

function goWorkspace() {
  router.push('/workspace')
}

function formatCurrency(cents = 0) {
  return `¥${(Number(cents || 0) / 100).toLocaleString('zh-CN', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  })}`
}

function formatDate(value) {
  if (!value) return '-'
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  }).format(new Date(value))
}

function shouldAutoConfirmPayment() {
  return order.value?.payment_status === 'pending' && (isReturnPage.value || Boolean(order.value?.payment_request_at))
}

function paymentStatusText(status) {
  const labels = {
    pending: '待支付',
    paid: '已到账',
    failed: '支付失败',
    refunded: '已退款',
    expired: '已过期'
  }
  return labels[status] ?? '确认中'
}

onMounted(loadOrder)
onUnmounted(stopAutoPolling)
</script>

<template>
  <section class="checkout-alipay-page">
    <div class="checkout-receipt-shell">
      <header class="checkout-receipt-header">
        <div>
          <p class="checkout-eyebrow">Alipay checkout</p>
          <h1>支付宝支付确认</h1>
          <span>订单 {{ order?.order_number || orderNumber || '-' }}</span>
        </div>
        <div class="checkout-status-stack">
          <span :class="['checkout-status-pill', `checkout-status-${statusTone}`]">{{ statusText }}</span>
          <strong>{{ formatCurrency(order?.amount_cents) }}</strong>
        </div>
      </header>

      <div v-if="loading" class="page-status">加载中...</div>
      <p v-if="statusMessage" class="status-info">{{ statusMessage }}</p>
      <p v-if="errorMessage" class="status-error">{{ errorMessage }}</p>

      <template v-if="order">
        <section class="checkout-topline">
          <div>
            <span>订单号</span>
            <strong>{{ order.order_number }}</strong>
          </div>
          <div>
            <span>支付方式</span>
            <strong>支付宝</strong>
          </div>
          <div>
            <span>套餐金额</span>
            <strong>{{ formatCurrency(order.amount_cents) }}</strong>
          </div>
          <button
            v-if="canPay"
            class="checkout-pay-button"
            data-testid="checkout-alipay-pay"
            type="button"
            :disabled="paying"
            @click="startPay"
          >
            <CreditCard :size="18" />
            {{ paying ? '正在打开支付宝...' : '去支付宝支付' }}
          </button>
        </section>

        <section class="checkout-section">
          <div class="checkout-section-title">
            <h2>交易信息</h2>
            <span>{{ formatDate(snapshot.ordered_at || order.created_at) }}</span>
          </div>
          <dl class="checkout-detail-grid">
            <div>
              <dt>交易网址</dt>
              <dd>{{ snapshot.transaction_url || order.transaction_url || '-' }}</dd>
            </div>
            <div>
              <dt>商品内容</dt>
              <dd>{{ snapshot.product_content || order.package_name || '-' }}</dd>
            </div>
            <div>
              <dt>套餐点数</dt>
              <dd>{{ snapshot.package_credits || order.package_credits }} 点</dd>
            </div>
            <div>
              <dt>有效期</dt>
              <dd>{{ snapshot.valid_days || '-' }} 天</dd>
            </div>
          </dl>
        </section>

        <section class="checkout-section">
          <div class="checkout-section-title">
            <h2>举证信息</h2>
          </div>
          <dl class="checkout-detail-grid">
            <div>
              <dt>收货人</dt>
              <dd>{{ snapshot.receipt_name || '-' }}</dd>
            </div>
            <div class="checkout-detail-wide">
              <dt>收货地址</dt>
              <dd>{{ snapshot.receipt_address || '虚拟商品线上交付，支付成功后点数发放至当前账户，无需物流' }}</dd>
            </div>
          </dl>
        </section>

        <section class="checkout-payment-row">
          <div class="checkout-payment-method" aria-label="支付方式">
            <span>AliPay</span>
            <strong>支付宝电脑网站支付</strong>
            <small>默认支付方式</small>
          </div>
          <div class="checkout-actions">
            <button class="secondary-button icon-button-text" data-testid="checkout-alipay-refresh" type="button" :disabled="refreshing" @click="refreshStatus">
              <RefreshCw :size="16" />
              刷新支付状态
            </button>
            <button class="secondary-button icon-button-text" type="button" @click="goAccount">
              <WalletCards :size="16" />
              查看余额
            </button>
            <button class="primary-button icon-button-text" type="button" @click="goWorkspace">
              <LayoutDashboard :size="16" />
              去工作台
            </button>
          </div>
        </section>
      </template>
    </div>

    <div ref="alipayFormHost" data-testid="alipay-form-host" class="alipay-form-host" v-html="formHTML" />
  </section>
</template>

<style scoped>
.checkout-alipay-page {
  min-height: calc(100svh - 80px);
  padding: 24px 0 42px;
  color: #111827;
}

.checkout-receipt-shell {
  display: grid;
  gap: 16px;
  width: min(1040px, calc(100vw - 32px));
  margin: 0 auto;
}

.checkout-receipt-header,
.checkout-topline,
.checkout-section,
.checkout-payment-row {
  border: 1px solid rgba(17, 24, 39, 0.1);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.92);
}

.checkout-receipt-header {
  display: flex;
  justify-content: space-between;
  gap: 18px;
  padding: 22px;
}

.checkout-eyebrow,
.checkout-receipt-header span,
.checkout-topline span,
.checkout-section-title span,
.checkout-detail-grid dt,
.checkout-payment-method small {
  color: #667085;
  font-size: 0.82rem;
  font-weight: 800;
}

.checkout-eyebrow {
  margin: 0 0 6px;
  text-transform: uppercase;
  letter-spacing: 0;
}

.checkout-receipt-header h1 {
  margin: 0 0 6px;
  font-size: 1.9rem;
}

.checkout-status-stack {
  display: grid;
  justify-items: end;
  align-content: center;
  gap: 8px;
}

.checkout-status-stack strong {
  font-size: 1.8rem;
}

.checkout-status-pill {
  display: inline-flex;
  align-items: center;
  min-height: 30px;
  padding: 0 10px;
  border-radius: 999px;
  font-weight: 900;
}

.checkout-status-pending {
  background: rgba(245, 158, 11, 0.16);
  color: #92400e;
}

.checkout-status-paid {
  background: rgba(34, 197, 94, 0.14);
  color: #15803d;
}

.checkout-status-failed {
  background: rgba(239, 68, 68, 0.12);
  color: #b42318;
}

.checkout-topline {
  display: grid;
  grid-template-columns: 1.25fr 0.85fr 0.75fr auto;
  align-items: center;
  gap: 14px;
  padding: 16px;
}

.checkout-topline div,
.checkout-detail-grid div {
  min-width: 0;
}

.checkout-topline strong,
.checkout-detail-grid dd {
  overflow-wrap: anywhere;
}

.checkout-pay-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  min-height: 44px;
  padding: 0 16px;
  border: 0;
  border-radius: 8px;
  background: #1677ff;
  color: #fff;
  font-weight: 900;
  cursor: pointer;
}

.checkout-section {
  padding: 18px;
}

.checkout-section-title {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 14px;
}

.checkout-section-title h2 {
  margin: 0;
  font-size: 1.08rem;
}

.checkout-detail-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px 18px;
  margin: 0;
}

.checkout-detail-grid dt {
  margin-bottom: 5px;
}

.checkout-detail-grid dd {
  margin: 0;
  font-weight: 800;
}

.checkout-detail-wide {
  grid-column: 1 / -1;
}

.checkout-payment-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 16px;
}

.checkout-payment-method {
  display: grid;
  gap: 4px;
}

.checkout-payment-method span {
  width: fit-content;
  padding: 4px 9px;
  border-radius: 999px;
  background: #1677ff;
  color: #fff;
  font-size: 0.78rem;
  font-weight: 900;
}

.checkout-actions {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.alipay-form-host {
  display: none;
}

@media (max-width: 760px) {
  .checkout-receipt-header,
  .checkout-payment-row {
    display: grid;
    justify-items: start;
  }

  .checkout-status-stack {
    justify-items: start;
  }

  .checkout-topline,
  .checkout-detail-grid {
    grid-template-columns: 1fr;
  }

  .checkout-actions {
    justify-content: flex-start;
  }
}
</style>
