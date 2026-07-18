<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import {
  Banknote,
  ChevronLeft,
  ChevronRight,
  Download,
  Eye,
  FileCheck2,
  ReceiptText,
  RefreshCw,
  RotateCcw,
  Search,
  WalletCards
} from 'lucide-vue-next'

import { api } from '../api/client.js'
import ClickSelect from '../components/ClickSelect.vue'

const loading = ref(false)
const detailLoading = ref(false)
const syncingPaymentID = ref(null)
const errorMessage = ref('')
const selectedOrder = ref(null)
const orders = ref([])
const kpis = ref({})
const trend = ref([])
const refundOverview = ref({ items: [] })
const invoiceOverview = ref({ items: [] })
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)

const filters = reactive({
  q: '',
  type: '',
  payment_status: ''
})
const orderTypeOptions = [
  { value: '', label: '全部类型' },
  { value: 'package', label: '套餐订单' }
]
const paymentStatusOptions = [
  { value: '', label: '全部支付状态' },
  { value: 'pending', label: '待支付' },
  { value: 'paid', label: '已支付' },
  { value: 'refunded', label: '已退款' },
  { value: 'failed', label: '支付失败' },
  { value: 'expired', label: '已过期' }
]

const kpiCards = computed(() => [
  { key: 'today', label: '今日收入', value: formatCurrency(kpis.value.today_revenue_cents), icon: Banknote },
  { key: 'month', label: '本月收入', value: formatCurrency(kpis.value.month_revenue_cents), icon: WalletCards },
  { key: 'pending', label: '待处理订单', value: formatNumber(kpis.value.pending_orders), icon: ReceiptText },
  { key: 'refund', label: '退款中', value: formatNumber(kpis.value.refunding_count), icon: RotateCcw }
])

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))
const rangeStart = computed(() => (total.value === 0 ? 0 : (page.value - 1) * pageSize.value + 1))
const rangeEnd = computed(() => Math.min(total.value, page.value * pageSize.value))

const trendPolyline = computed(() => {
  const points = trend.value ?? []
  if (points.length === 0) return ''
  const maxRevenue = Math.max(1, ...points.map((point) => Number(point.revenue_cents ?? 0)))
  return points.map((point, index) => {
    const x = points.length === 1 ? 50 : (index / (points.length - 1)) * 100
    const y = 92 - (Number(point.revenue_cents ?? 0) / maxRevenue) * 78
    return `${x.toFixed(2)},${y.toFixed(2)}`
  }).join(' ')
})

const trendBars = computed(() => {
  const points = trend.value ?? []
  const maxOrders = Math.max(1, ...points.map((point) => Number(point.order_count ?? 0)))
  return points.slice(-12).map((point) => ({
    date: point.date,
    height: `${Math.max(10, (Number(point.order_count ?? 0) / maxOrders) * 64)}px`
  }))
})

function requestParams() {
  return {
    type: filters.type,
    payment_status: filters.payment_status,
    q: filters.q.trim(),
    page: page.value,
    page_size: pageSize.value
  }
}

async function load() {
  loading.value = true
  errorMessage.value = ''
  try {
    const payload = await api.listFinanceOrders(requestParams())
    orders.value = payload.items ?? []
    kpis.value = payload.kpis ?? {}
    trend.value = payload.trend ?? []
    refundOverview.value = payload.refund_overview ?? { items: [] }
    invoiceOverview.value = payload.invoice_overview ?? { items: [] }
    total.value = payload.total ?? 0
    page.value = payload.page ?? page.value
    pageSize.value = payload.page_size ?? pageSize.value
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    loading.value = false
  }
}

function applyFilters() {
  page.value = 1
  load()
}

function goToPage(nextPage) {
  page.value = Math.min(Math.max(1, nextPage), totalPages.value)
  load()
}

function exportOrders() {
  const url = api.financeOrdersExportURL(requestParams())
  globalThis.open?.(url, 'finance-orders-export')
}

async function viewOrder(order) {
  detailLoading.value = true
  try {
    selectedOrder.value = await api.getFinanceOrder(order.id)
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    detailLoading.value = false
  }
}

async function syncPayment(order) {
  if (!canSyncPayment(order) || syncingPaymentID.value) return
  syncingPaymentID.value = order.id
  errorMessage.value = ''
  try {
    await api.syncFinanceOrderPayment(order.id)
    await load()
    if (selectedOrder.value?.id === order.id) {
      selectedOrder.value = await api.getFinanceOrder(order.id)
    }
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    syncingPaymentID.value = null
  }
}

async function completeRefund(refund) {
  await api.updateFinanceRefund(refund.id, { status: 'completed' })
  await load()
}

async function issueInvoice(invoice) {
  await api.updateFinanceInvoice(invoice.id, { status: 'issued' })
  await load()
}

function displayUser(order) {
  return order.user?.display_name || order.user?.username || `用户 #${order.user_id || '-'}`
}

function formatCurrency(cents = 0) {
  return `¥${(Number(cents || 0) / 100).toLocaleString('zh-CN', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  })}`
}

function formatNumber(value = 0) {
  return Number(value || 0).toLocaleString('zh-CN')
}

function formatDate(value) {
  if (!value) return '-'
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  }).format(new Date(value))
}

function paymentStatusText(status) {
  const labels = {
    pending: '待支付',
    paid: '已支付',
    refunded: '已退款',
    failed: '支付失败',
    expired: '已过期'
  }
  return labels[status] ?? status ?? '-'
}

function invoiceStatusText(status) {
  const labels = {
    pending: '待开票',
    issued: '已开票',
    rejected: '已驳回',
    voided: '已作废'
  }
  return labels[status] ?? status ?? '-'
}

function refundStatusText(status) {
  const labels = {
    pending: '待处理',
    processing: '处理中',
    approved: '已通过',
    rejected: '已拒绝',
    completed: '已退款'
  }
  return labels[status] ?? status ?? '-'
}

function orderTypeText(type) {
  return type === 'package' ? '套餐订单' : (type || '-')
}

function paymentMethodText(method) {
  const labels = {
    offline_transfer: '线下转账',
    alipay_page: '支付宝电脑网站支付',
    wechat_jsapi: '微信 JSAPI 支付',
    wechat_virtual_goods: '微信虚拟支付'
  }
  return labels[method] ?? method ?? '-'
}

function paymentRecordStatusText(status) {
  const labels = {
    created: '已创建',
    requested: '已请求',
    paid: '已支付',
    failed: '失败',
    closed: '已关闭'
  }
  return labels[status] ?? status ?? '-'
}

function paymentRecordCounts(record) {
  if (!record) return '请求 0 / 通知 0 / 查询 0'
  return `请求 ${record.request_count || 0} / 通知 ${record.notify_count || 0} / 查询 ${record.query_count || 0}`
}

function canSyncPayment(order) {
  return order?.payment_method === 'alipay_page' && order?.payment_status === 'pending'
}

onMounted(load)
</script>

<template>
  <section class="finance-orders-page">
    <div class="admin-page-heading finance-heading">
      <div>
        <p class="eyebrow">Finance orders</p>
        <h1>财务订单</h1>
        <span>管理订单、收入、退款与发票，掌握财务全局</span>
      </div>
      <div class="finance-heading-actions">
        <button class="secondary-button icon-button-text" type="button" :disabled="loading" @click="load">
          <RefreshCw :size="16" />
          刷新
        </button>
        <button class="primary-button icon-button-text" data-testid="finance-export" type="button" @click="exportOrders">
          <Download :size="16" />
          导出
        </button>
      </div>
    </div>

    <div class="finance-kpi-grid">
      <article v-for="card in kpiCards" :key="card.key" class="finance-kpi-card">
        <span class="finance-kpi-icon"><component :is="card.icon" :size="18" /></span>
        <div>
          <p>{{ card.label }}</p>
          <strong>{{ card.value }}</strong>
        </div>
      </article>
    </div>

    <div class="finance-main-grid">
      <article class="admin-panel finance-trend-panel">
        <div class="panel-title-row">
          <div>
            <p class="eyebrow">Revenue trend</p>
            <h2>近 30 天收入趋势</h2>
          </div>
          <span>{{ trend.length }} 天</span>
        </div>
        <svg class="finance-trend-chart" data-testid="finance-trend-chart" viewBox="0 0 100 100" preserveAspectRatio="none" role="img" aria-label="近 30 天收入趋势">
          <polyline v-if="trendPolyline" :points="trendPolyline" fill="none" stroke="#0f766e" stroke-width="3" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
        <div class="finance-trend-bars" aria-hidden="true">
          <span v-for="bar in trendBars" :key="bar.date" :style="{ height: bar.height }" />
        </div>
      </article>

      <article class="admin-panel finance-table-panel">
        <form class="admin-filter-bar finance-filter-bar" data-testid="finance-orders-filter" @submit.prevent="applyFilters">
          <label class="admin-search-field finance-search-field">
            <Search :size="17" />
            <input v-model="filters.q" data-testid="finance-order-search" type="search" placeholder="搜索订单号、支付记录号、支付宝交易号、用户或套餐" />
          </label>
          <ClickSelect v-model="filters.type" :options="orderTypeOptions" data-testid="finance-order-type" class="text-input compact-input" aria-label="订单类型" compact />
          <ClickSelect v-model="filters.payment_status" :options="paymentStatusOptions" data-testid="finance-payment-status" class="text-input compact-input" aria-label="支付状态" compact />
          <button class="primary-button compact-button" type="submit">筛选</button>
        </form>

        <div class="admin-table-scroll finance-table-scroll">
          <table class="data-table admin-data-table finance-data-table">
            <thead>
              <tr>
                <th>订单号</th>
                <th>用户</th>
                <th>类型</th>
                <th>金额</th>
                <th>支付方式</th>
                <th>支付状态</th>
                <th>开票状态</th>
                <th>创建时间</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="order in orders" :key="order.id">
                <td>
                  <strong>{{ order.order_number }}</strong>
                  <small>#{{ order.id }}</small>
                </td>
                <td>
                  <strong>{{ displayUser(order) }}</strong>
                  <small>{{ order.user?.email || order.user?.username || '-' }}</small>
                </td>
                <td>
                  <strong>{{ orderTypeText(order.order_type) }}</strong>
                  <small>{{ order.package_name || '-' }}</small>
                </td>
                <td><strong>{{ formatCurrency(order.amount_cents) }}</strong></td>
                <td>{{ paymentMethodText(order.payment_method) }}</td>
                <td><span class="status-pill" :class="`finance-pay-${order.payment_status}`">{{ paymentStatusText(order.payment_status) }}</span></td>
                <td><span class="status-pill" :class="`finance-invoice-${order.invoice_status}`">{{ invoiceStatusText(order.invoice_status) }}</span></td>
                <td>{{ formatDate(order.created_at) }}</td>
                <td>
                  <button
                    v-if="canSyncPayment(order)"
                    class="mini-button icon-only"
                    data-testid="finance-sync-payment"
                    type="button"
                    aria-label="同步支付状态"
                    :disabled="syncingPaymentID === order.id"
                    @click="syncPayment(order)"
                  >
                    <RefreshCw :size="15" />
                  </button>
                  <button class="mini-button icon-only" data-testid="finance-view-order" type="button" aria-label="查看订单" @click="viewOrder(order)">
                    <Eye :size="15" />
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
          <p v-if="loading" class="page-status">加载中...</p>
          <p v-else-if="orders.length === 0" class="page-status">暂无财务订单</p>
        </div>

        <div class="admin-pagination" data-testid="finance-orders-pagination">
          <span>第 {{ rangeStart }}-{{ rangeEnd }} 条 / 共 {{ total }} 条</span>
          <div class="inline-actions">
            <button class="mini-button" type="button" :disabled="loading || page <= 1" @click="goToPage(page - 1)">
              <ChevronLeft :size="15" />
              上一页
            </button>
            <button class="mini-button" type="button" :disabled="loading || page >= totalPages" @click="goToPage(page + 1)">
              下一页
              <ChevronRight :size="15" />
            </button>
          </div>
        </div>
      </article>
    </div>

    <div class="finance-bottom-grid">
      <article class="admin-panel finance-overview-panel">
        <div class="panel-title-row">
          <div>
            <p class="eyebrow">Refunds</p>
            <h2>退款 / 对账概览</h2>
          </div>
          <strong>{{ formatCurrency(refundOverview.total_refund_cents) }}</strong>
        </div>
        <div class="finance-overview-stats">
          <span>待处理 {{ formatNumber(refundOverview.pending_count) }}</span>
          <span>处理中 {{ formatNumber(refundOverview.processing_count) }}</span>
          <span>已完成 {{ formatNumber(refundOverview.completed_count) }}</span>
        </div>
        <div class="finance-list">
          <article v-for="refund in refundOverview.items" :key="refund.id" class="finance-list-item">
            <RotateCcw :size="16" />
            <div>
              <strong>{{ refund.refund_number }}</strong>
              <span>{{ refund.order_number }} · {{ formatCurrency(refund.amount_cents) }}</span>
              <small>{{ refund.reason || refundStatusText(refund.status) }}</small>
            </div>
            <button
              v-if="refund.status !== 'completed'"
              class="mini-button"
              data-testid="finance-refund-complete"
              type="button"
              @click="completeRefund(refund)"
            >
              处理
            </button>
          </article>
          <p v-if="(refundOverview.items ?? []).length === 0" class="page-status">暂无退款记录</p>
        </div>
      </article>

      <article class="admin-panel finance-overview-panel">
        <div class="panel-title-row">
          <div>
            <p class="eyebrow">Invoices</p>
            <h2>发票处理</h2>
          </div>
          <FileCheck2 :size="20" />
        </div>
        <div class="finance-overview-stats">
          <span>待开票 {{ formatNumber(invoiceOverview.pending_count) }}</span>
          <span>已开票 {{ formatNumber(invoiceOverview.issued_count) }}</span>
          <span>已驳回 {{ formatNumber(invoiceOverview.rejected_count) }}</span>
        </div>
        <div class="finance-list">
          <article v-for="invoice in invoiceOverview.items" :key="invoice.id" class="finance-list-item">
            <FileCheck2 :size="16" />
            <div>
              <strong>{{ invoice.invoice_number }}</strong>
              <span>{{ invoice.title }} · {{ formatCurrency(invoice.amount_cents) }}</span>
              <small>{{ invoice.order_number }} · {{ invoiceStatusText(invoice.status) }}</small>
            </div>
            <button
              v-if="invoice.status !== 'issued'"
              class="mini-button"
              data-testid="finance-invoice-issue"
              type="button"
              @click="issueInvoice(invoice)"
            >
              开票
            </button>
          </article>
          <p v-if="(invoiceOverview.items ?? []).length === 0" class="page-status">暂无发票记录</p>
        </div>
      </article>
    </div>

    <article v-if="selectedOrder" class="admin-panel finance-detail-panel" data-testid="finance-order-detail">
      <div class="panel-title-row">
        <div>
          <p class="eyebrow">Order detail</p>
          <h2>{{ selectedOrder.order_number }}</h2>
        </div>
        <span>{{ detailLoading ? '加载中' : paymentStatusText(selectedOrder.payment_status) }}</span>
      </div>
      <div class="finance-detail-grid">
        <div>
          <span>用户</span>
          <strong>{{ displayUser(selectedOrder) }}</strong>
        </div>
        <div>
          <span>套餐</span>
          <strong>{{ selectedOrder.package_name || '-' }}</strong>
        </div>
        <div>
          <span>金额</span>
          <strong>{{ formatCurrency(selectedOrder.amount_cents) }}</strong>
        </div>
        <div>
          <span>开票</span>
          <strong>{{ invoiceStatusText(selectedOrder.invoice_status || selectedOrder.invoice?.status) }}</strong>
        </div>
        <div>
          <span>支付方式</span>
          <strong>{{ paymentMethodText(selectedOrder.payment_method) }}</strong>
        </div>
        <div>
          <span>支付宝交易号</span>
          <strong>{{ selectedOrder.alipay_trade_no || '-' }}</strong>
        </div>
        <div>
          <span>支付记录号</span>
          <strong>{{ selectedOrder.payment_record?.payment_number || '-' }}</strong>
        </div>
        <div>
          <span>支付记录状态</span>
          <strong>{{ paymentRecordStatusText(selectedOrder.payment_record?.status) }}</strong>
        </div>
        <div>
          <span>渠道交易号</span>
          <strong>{{ selectedOrder.payment_record?.provider_trade_no || selectedOrder.alipay_trade_no || '-' }}</strong>
        </div>
        <div>
          <span>请求/通知/查询次数</span>
          <strong>{{ paymentRecordCounts(selectedOrder.payment_record) }}</strong>
        </div>
        <div class="finance-detail-wide">
          <span>最近错误</span>
          <strong>{{ selectedOrder.payment_record?.last_error_code || '-' }}{{ selectedOrder.payment_record?.last_error_message ? `：${selectedOrder.payment_record.last_error_message}` : '' }}</strong>
        </div>
        <div>
          <span>支付宝通知时间</span>
          <strong>{{ formatDate(selectedOrder.alipay_notify_at) }}</strong>
        </div>
        <div>
          <span>交易网址</span>
          <strong>{{ selectedOrder.evidence_snapshot?.transaction_url || selectedOrder.transaction_url || '-' }}</strong>
        </div>
        <div class="finance-detail-wide">
          <span>商品内容</span>
          <strong>{{ selectedOrder.evidence_snapshot?.product_content || '-' }}</strong>
        </div>
        <div class="finance-detail-wide">
          <span>收货地址</span>
          <strong>{{ selectedOrder.evidence_snapshot?.receipt_address || '-' }}</strong>
        </div>
      </div>
    </article>

    <p v-if="errorMessage" class="status-error">{{ errorMessage }}</p>
  </section>
</template>

<style scoped>
.finance-orders-page {
  display: grid;
  gap: 16px;
}

.finance-heading span {
  display: block;
  margin-top: 4px;
  color: #667085;
  font-size: 0.95rem;
}

.finance-heading-actions,
.finance-kpi-card,
.finance-overview-stats,
.finance-list-item {
  display: flex;
  align-items: center;
}

.finance-heading-actions {
  gap: 10px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.finance-kpi-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(160px, 1fr));
  gap: 12px;
}

.finance-kpi-card {
  min-width: 0;
  gap: 12px;
  min-height: 112px;
  padding: 16px;
  border: 1px solid rgba(112, 124, 156, 0.13);
  border-radius: 16px;
  background: rgba(255, 255, 255, 0.84);
  box-shadow: 0 16px 34px rgba(82, 92, 126, 0.08);
}

.finance-kpi-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  width: 40px;
  height: 40px;
  border-radius: 13px;
  background: #0f766e;
  color: #fff;
}

.finance-kpi-card p {
  margin: 0 0 8px;
  color: #667085;
  font-size: 0.84rem;
  font-weight: 800;
}

.finance-kpi-card strong {
  display: block;
  min-width: 0;
  overflow-wrap: anywhere;
  color: #101828;
  font-size: 1.55rem;
  line-height: 1.08;
}

.finance-main-grid {
  display: grid;
  grid-template-columns: minmax(300px, 0.42fr) minmax(0, 1fr);
  gap: 16px;
  align-items: start;
}

.finance-trend-panel,
.finance-table-panel,
.finance-overview-panel,
.finance-detail-panel {
  min-width: 0;
  overflow: hidden;
}

.finance-trend-chart {
  display: block;
  width: 100%;
  height: 220px;
  margin-top: 8px;
  border-radius: 14px;
  background:
    linear-gradient(rgba(118, 129, 166, 0.11) 1px, transparent 1px),
    linear-gradient(90deg, rgba(118, 129, 166, 0.11) 1px, transparent 1px);
  background-size: 100% 25%, 12.5% 100%;
}

.finance-trend-bars {
  display: grid;
  grid-template-columns: repeat(12, minmax(0, 1fr));
  align-items: end;
  gap: 7px;
  height: 64px;
  margin-top: 16px;
}

.finance-trend-bars span {
  display: block;
  min-height: 8px;
  border-radius: 999px 999px 3px 3px;
  background: #2563eb;
}

.finance-filter-bar {
  margin-bottom: 12px;
}

.finance-search-field {
  flex: 1;
  min-width: min(100%, 260px);
}

.finance-table-scroll {
  overflow-x: auto;
}

.finance-data-table {
  min-width: 980px;
}

.finance-data-table td strong,
.finance-data-table td small {
  display: block;
}

.finance-data-table td small {
  margin-top: 3px;
  color: #8a94a6;
}

.finance-pay-paid,
.finance-invoice-issued {
  background: rgba(34, 197, 94, 0.12);
  color: #14804a;
}

.finance-pay-pending,
.finance-invoice-pending {
  background: rgba(245, 158, 11, 0.16);
  color: #92400e;
}

.finance-pay-refunded,
.finance-invoice-voided {
  background: rgba(107, 114, 128, 0.1);
  color: #667085;
}

.finance-pay-failed,
.finance-pay-expired,
.finance-invoice-rejected {
  background: rgba(239, 68, 68, 0.12);
  color: #b42318;
}

.finance-bottom-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.finance-overview-panel {
  display: grid;
  gap: 14px;
}

.finance-overview-stats {
  gap: 8px;
  flex-wrap: wrap;
}

.finance-overview-stats span {
  padding: 7px 10px;
  border-radius: 999px;
  background: rgba(248, 250, 253, 0.94);
  color: #667085;
  font-size: 0.82rem;
  font-weight: 850;
}

.finance-list {
  display: grid;
  gap: 10px;
}

.finance-list-item {
  gap: 10px;
  min-width: 0;
  padding: 12px;
  border: 1px solid rgba(118, 129, 166, 0.12);
  border-radius: 14px;
  background: rgba(248, 250, 253, 0.84);
}

.finance-list-item > svg {
  flex: 0 0 auto;
  color: #0f766e;
}

.finance-list-item div {
  display: grid;
  min-width: 0;
  gap: 3px;
  flex: 1;
}

.finance-list-item strong,
.finance-list-item span,
.finance-list-item small {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.finance-list-item span,
.finance-list-item small {
  color: #667085;
  font-size: 0.82rem;
}

.finance-detail-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.finance-detail-grid div {
  display: grid;
  min-width: 0;
  gap: 5px;
  padding: 12px;
  border-radius: 14px;
  background: rgba(248, 250, 253, 0.9);
}

.finance-detail-grid span {
  color: #667085;
  font-size: 0.8rem;
  font-weight: 800;
}

.finance-detail-grid strong {
  min-width: 0;
  overflow-wrap: anywhere;
  color: #111827;
}

.finance-detail-wide {
  grid-column: span 2;
}

@media (max-width: 1180px) {
  .finance-main-grid,
  .finance-bottom-grid {
    grid-template-columns: 1fr;
  }

  .finance-kpi-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 720px) {
  .finance-heading,
  .finance-heading-actions {
    align-items: flex-start;
  }

  .finance-heading-actions,
  .finance-kpi-grid,
  .finance-detail-grid {
    display: grid;
    grid-template-columns: 1fr;
    width: 100%;
  }

  .finance-kpi-card {
    min-height: 104px;
  }
}
</style>
