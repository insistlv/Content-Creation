<script setup>
import { computed, onMounted, ref } from 'vue'
import { onReachBottom } from '@dcloudio/uni-app'

import { api } from '../../../api/client.js'
import { navigateTo, redirectToAuth, requireAuth, routes } from '../../../utils/routes.js'

const pageSize = 20

const filterTabs = [
  { key: 'all', label: '全部' },
  { key: 'recharge', label: '收入' },
  { key: 'consume', label: '支出' }
]

const activeKind = ref('all')
const transactions = ref([])
const currentPage = ref(1)
const total = ref(0)
const hasMore = ref(false)
const loading = ref(false)
const loadingMore = ref(false)
const errorMessage = ref('')

const visibleTotal = computed(() => transactions.value.length)
const emptyText = computed(() => {
  if (activeKind.value === 'recharge') return '暂无流水记录'
  if (activeKind.value === 'consume') return '暂无流水记录'
  return '暂无流水记录'
})

function goBack() {
  const pages = getCurrentPages()
  if (pages.length > 1) {
    uni.navigateBack()
    return
  }
  navigateTo(routes.account)
}

function showToast(title) {
  uni.showToast({ title, icon: 'none' })
}

function transactionKind(item) {
  const amount = Number(item.amount) || 0
  const type = `${item.type || ''}`
  if (amount > 0 || type.includes('topup') || type.includes('recharge')) return 'recharge'
  return 'consume'
}

function transactionTitle(item) {
  const type = `${item.type || ''}`
  if (item.reason) return item.reason
  if (type.includes('generation')) return '生成消耗'
  if (type.includes('deduct')) return '点数扣减'
  if (type.includes('topup')) return '点数充值'
  return '点数变动'
}

function formatDate(value) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  const year = date.getFullYear()
  const month = `${date.getMonth() + 1}`.padStart(2, '0')
  const day = `${date.getDate()}`.padStart(2, '0')
  const hour = `${date.getHours()}`.padStart(2, '0')
  const minute = `${date.getMinutes()}`.padStart(2, '0')
  return `${year}/${month}/${day} ${hour}:${minute}`
}

function balanceText(item) {
  const value = Number(item.balance_after)
  if (!Number.isFinite(value)) return '-'
  return `${value}点`
}

function amountText(item) {
  const value = Number(item.amount) || 0
  return `${value > 0 ? '+' : ''}${value}点`
}

function requestParams(page) {
  return {
    page,
    page_size: pageSize,
    kind: activeKind.value
  }
}

function normalizeItems(payload) {
  return Array.isArray(payload?.items) ? payload.items : []
}

async function loadTransactions(reset = false) {
  if (loading.value || loadingMore.value) return
  const nextPage = reset ? 1 : currentPage.value + 1
  if (!reset && !hasMore.value) return

  if (reset) {
    loading.value = true
    transactions.value = []
  } else {
    loadingMore.value = true
  }
  errorMessage.value = ''

  try {
    const user = await requireAuth()
    if (!user) return
    const payload = await api.getCreditTransactions(requestParams(nextPage))
    const nextItems = normalizeItems(payload)
    transactions.value = reset ? nextItems : [...transactions.value, ...nextItems]
    currentPage.value = Number(payload?.page) || nextPage
    total.value = Number(payload?.total) || transactions.value.length
    hasMore.value = Boolean(payload?.has_more)
  } catch (error) {
    if (error?.status === 401) {
      redirectToAuth({ redirect: routes.accountTransactions })
      return
    }
    errorMessage.value = error.message || '流水读取失败'
    if (!reset) showToast(errorMessage.value)
  } finally {
    loading.value = false
    loadingMore.value = false
  }
}

function changeKind(kind) {
  if (activeKind.value === kind) return
  activeKind.value = kind
  currentPage.value = 1
  total.value = 0
  hasMore.value = false
  loadTransactions(true)
}

function retryLoad() {
  loadTransactions(transactions.value.length === 0)
}

function loadMore() {
  loadTransactions(false)
}

onMounted(() => {
  loadTransactions(true)
})

onReachBottom(() => {
  loadMore()
})
</script>

<template>
  <view class="transactions-page">
    <view class="app-shell">
      <view class="topbar">
        <button type="button" class="back-button" aria-label="返回" @tap="goBack">‹</button>
        <view class="title-copy">
          <text>点数流水</text>
          <text>{{ visibleTotal }} / {{ total }} 条记录</text>
        </view>
      </view>

      <view class="summary-band">
        <view>
          <text>当前筛选</text>
          <text>{{ filterTabs.find((item) => item.key === activeKind)?.label || '全部' }}</text>
        </view>
        <view>
          <text>每页</text>
          <text>{{ pageSize }} 条</text>
        </view>
      </view>

      <view class="filter-tabs" role="tablist" aria-label="点数流水筛选">
        <button
          v-for="tab in filterTabs"
          :key="tab.key"
          type="button"
          :class="{ active: activeKind === tab.key }"
          @tap="changeKind(tab.key)"
        >
          {{ tab.label }}
        </button>
      </view>

      <view v-if="errorMessage && transactions.length === 0" class="state-panel error">
        <text>{{ errorMessage }}</text>
        <button type="button" @tap="retryLoad">重试</button>
      </view>

      <view v-else-if="loading" class="state-panel">
        <text>流水读取中</text>
      </view>

      <view v-else-if="transactions.length === 0" class="state-panel empty">
        <view class="empty-illustration">
          <text></text>
          <text></text>
          <text></text>
        </view>
        <text>{{ emptyText }}</text>
      </view>

      <view v-else class="transaction-list">
        <view v-for="item in transactions" :key="item.id" class="transaction-row">
          <text class="flow-icon" :class="transactionKind(item) === 'recharge' ? 'up' : 'down'">
            {{ transactionKind(item) === 'recharge' ? '+' : '-' }}
          </text>
          <view class="flow-copy">
            <text>{{ transactionTitle(item) }}</text>
            <text>{{ formatDate(item.created_at) }}</text>
          </view>
          <view class="flow-numbers">
            <text class="flow-amount" :class="transactionKind(item) === 'recharge' ? 'up' : 'down'">
              {{ amountText(item) }}
            </text>
            <text>余额 {{ balanceText(item) }}</text>
          </view>
        </view>
      </view>

      <view v-if="transactions.length > 0" class="load-more-area">
        <button v-if="hasMore" type="button" :disabled="loadingMore" @tap="loadMore">
          {{ loadingMore ? '加载中' : '加载更多' }}
        </button>
        <text v-else>没有更多了</text>
      </view>

      <view v-if="errorMessage && transactions.length > 0" class="inline-error">
        <text>{{ errorMessage }}</text>
        <button type="button" @tap="retryLoad">重试</button>
      </view>
    </view>
  </view>
</template>

<style lang="scss" scoped>
@use '../../../styles/tokens.scss' as *;

.transactions-page {
  min-height: 100vh;
  background:
    radial-gradient(circle at 50% 0%, rgba(255, 255, 255, 0.96) 0, rgba(255, 255, 255, 0) 260rpx),
    linear-gradient(180deg, #ecebff 0%, #f5f3ff 34%, #f7f8ff 100%);
  color: #16111f;
}

.transactions-page,
.transactions-page view,
.transactions-page button,
.transactions-page text {
  box-sizing: border-box;
}

.transactions-page button {
  margin: 0;
  padding: 0;
  border: 0;
  line-height: 1.2;
}

.transactions-page button::after {
  border: 0;
}

.app-shell {
  min-height: 100vh;
  padding: calc(18rpx + $phone-float-clearance-top + env(safe-area-inset-top)) 16rpx calc(38rpx + env(safe-area-inset-bottom));
}

.topbar {
  display: grid;
  grid-template-columns: 76rpx minmax(0, 1fr);
  gap: 16rpx;
  align-items: center;
}

.back-button {
  width: 76rpx;
  height: 76rpx;
  border-radius: 24rpx;
  background: rgba(255, 255, 255, 0.94);
  color: #6d28d9;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 54rpx;
  font-weight: 900;
  box-shadow: 0 14rpx 34rpx rgba(80, 63, 139, 0.1);
}

.title-copy {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 8rpx;
}

.title-copy text:first-child {
  color: #201832;
  font-size: 40rpx;
  font-weight: 950;
  line-height: 1.15;
}

.title-copy text:last-child {
  color: #7a728e;
  font-size: 23rpx;
  font-weight: 700;
}

.summary-band,
.filter-tabs,
.state-panel,
.transaction-list,
.load-more-area,
.inline-error {
  width: 100%;
  min-width: 0;
  border-radius: 32rpx;
  background: rgba(255, 255, 255, 0.94);
  box-shadow: 0 18rpx 48rpx rgba(80, 63, 139, 0.1);
}

.summary-band {
  margin-top: 22rpx;
  padding: 24rpx 26rpx;
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12rpx;
}

.summary-band view {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 8rpx;
}

.summary-band text:first-child {
  color: #9a92ad;
  font-size: 22rpx;
  font-weight: 800;
}

.summary-band text:last-child {
  color: #302640;
  font-size: 28rpx;
  font-weight: 900;
}

.filter-tabs {
  margin-top: 18rpx;
  padding: 8rpx;
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8rpx;
}

.filter-tabs button {
  min-width: 0;
  height: 68rpx;
  border-radius: 24rpx;
  color: #7a728e;
  background: transparent;
  font-size: 25rpx;
  font-weight: 850;
  display: flex;
  align-items: center;
  justify-content: center;
}

.filter-tabs button.active {
  color: #fff;
  background: linear-gradient(180deg, #8b5cf6 0%, #7c3aed 100%);
  box-shadow: 0 12rpx 24rpx rgba(124, 58, 237, 0.2);
}

.state-panel {
  min-height: 360rpx;
  margin-top: 18rpx;
  padding: 34rpx;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 22rpx;
  color: #aaa3ba;
  font-size: 26rpx;
  font-weight: 800;
}

.state-panel.error {
  color: #b91c1c;
  background: #fff7f7;
}

.state-panel button,
.inline-error button {
  height: 62rpx;
  padding: 0 28rpx;
  border-radius: 999rpx;
  background: #7c3aed;
  color: #fff;
  font-size: 24rpx;
  font-weight: 900;
  display: flex;
  align-items: center;
  justify-content: center;
}

.empty-illustration {
  position: relative;
  width: 126rpx;
  height: 96rpx;
}

.empty-illustration text:first-child {
  position: absolute;
  left: 22rpx;
  top: 10rpx;
  width: 82rpx;
  height: 62rpx;
  border-radius: 18rpx;
  background: #efe9ff;
  transform: rotate(-8deg);
}

.empty-illustration text:nth-child(2) {
  position: absolute;
  left: 34rpx;
  top: 24rpx;
  width: 82rpx;
  height: 62rpx;
  border-radius: 18rpx;
  background: #fff;
  border: 2rpx solid rgba(124, 58, 237, 0.2);
}

.empty-illustration text:last-child {
  position: absolute;
  left: 54rpx;
  top: 45rpx;
  width: 40rpx;
  height: 8rpx;
  border-radius: 999rpx;
  background: #cabffd;
}

.transaction-list {
  margin-top: 18rpx;
  padding: 10rpx 28rpx;
}

.transaction-row {
  min-height: 104rpx;
  display: grid;
  grid-template-columns: 46rpx minmax(0, 1fr) auto;
  gap: 14rpx;
  align-items: center;
  padding: 18rpx 0;
  border-top: 1rpx solid rgba(124, 58, 237, 0.1);
}

.transaction-row:first-child {
  border-top: 0;
}

.flow-icon {
  width: 42rpx;
  height: 42rpx;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 22rpx;
  font-weight: 900;
}

.flow-icon.up {
  background: #dcfce7;
  color: #15803d;
}

.flow-icon.down {
  background: #fee2e2;
  color: #b91c1c;
}

.flow-copy,
.flow-numbers {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 8rpx;
}

.flow-copy text:first-child {
  color: #201832;
  font-size: 26rpx;
  font-weight: 850;
  overflow-wrap: anywhere;
}

.flow-copy text:last-child,
.flow-numbers text:last-child {
  color: #a19aaf;
  font-size: 21rpx;
  font-weight: 700;
}

.flow-numbers {
  align-items: flex-end;
}

.flow-amount {
  font-size: 25rpx;
  font-weight: 950;
  white-space: nowrap;
}

.flow-amount.up {
  color: #15803d;
}

.flow-amount.down {
  color: #b91c1c;
}

.load-more-area {
  min-height: 82rpx;
  margin-top: 18rpx;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #9a92ad;
  font-size: 24rpx;
  font-weight: 850;
}

.load-more-area button {
  width: 100%;
  height: 82rpx;
  border-radius: 32rpx;
  color: #7c3aed;
  font-size: 25rpx;
  font-weight: 900;
  display: flex;
  align-items: center;
  justify-content: center;
}

.load-more-area button[disabled] {
  color: #aaa3ba;
}

.inline-error {
  margin-top: 18rpx;
  padding: 18rpx 22rpx;
  display: grid;
  grid-template-columns: minmax(0, 1fr) 116rpx;
  gap: 12rpx;
  align-items: center;
  color: #b91c1c;
  font-size: 23rpx;
  font-weight: 800;
  background: #fff7f7;
}
</style>
