<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  Copy,
  Download,
  RefreshCw,
  Search,
  ShieldAlert,
  UserRound,
  Wrench,
  X,
  XCircle
} from 'lucide-vue-next'

import { api } from '../api/client.js'
import ClickSelect from '../components/ClickSelect.vue'

const categories = [
  {
    key: 'user_login',
    label: '用户登录日志',
    hint: '失败登录只展示 IP、UA、状态和错误码',
    icon: UserRound,
    emptyText: '暂无用户登录日志',
    detailTitle: '登录详情',
    keywordPlaceholder: '用户 / IP / UA / 错误码'
  },
  {
    key: 'user_operation',
    label: '用户操作日志',
    hint: '创作、支付、点数和模型调用请求链路',
    icon: Activity,
    emptyText: '暂无用户操作日志',
    detailTitle: '操作详情',
    keywordPlaceholder: '路径 / 用户 / 模型 / 支付'
  },
  {
    key: 'system_operation',
    label: '系统操作日志',
    hint: '管理员后台审计动作和目标对象',
    icon: Wrench,
    emptyText: '暂无系统操作日志',
    detailTitle: '后台操作详情',
    keywordPlaceholder: '管理员 / 动作 / 目标 / JSON'
  }
]

const logs = ref([])
const summary = ref({})
const total = ref(0)
const page = ref(1)
const pageSize = ref(30)
const loading = ref(false)
const errorMessage = ref('')
const selectedLog = ref(null)
const showDetailModal = ref(false)
const activeCategory = ref('user_login')

const filters = reactive({
  level: '',
  method: '',
  status: '',
  keyword: '',
  date_from: '',
  date_to: ''
})
const levelOptions = [
  { value: '', label: '全部级别' },
  { value: 'info', label: '信息' },
  { value: 'warn', label: '警告' },
  { value: 'error', label: '错误' }
]
const methodOptions = [
  { value: '', label: '全部方法' },
  { value: 'POST', label: 'POST' },
  { value: 'PATCH', label: 'PATCH' },
  { value: 'PUT', label: 'PUT' },
  { value: 'DELETE', label: 'DELETE' }
]

const activeConfig = computed(() => categories.find((item) => item.key === activeCategory.value) ?? categories[0])
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value || 1)))
const rangeStart = computed(() => {
  if (total.value === 0 || logs.value.length === 0) return 0
  return (page.value - 1) * pageSize.value + 1
})
const rangeEnd = computed(() => {
  if (rangeStart.value === 0) return 0
  return rangeStart.value + logs.value.length - 1
})

const summaryCards = computed(() => {
  if (activeCategory.value === 'user_login') {
    return [
      { label: '全部登录', value: formatNumber(summary.value?.total ?? 0), icon: UserRound },
      { label: '成功登录', value: formatNumber(summary.value?.success_total ?? 0), icon: CheckCircle2 },
      { label: '失败登录', value: formatNumber(summary.value?.failed_total ?? 0), icon: XCircle },
      { label: '最近登录', value: formatDateTime(summary.value?.last_event_at), icon: RefreshCw }
    ]
  }
  if (activeCategory.value === 'system_operation') {
    return [
      { label: '后台操作', value: formatNumber(summary.value?.total ?? 0), icon: Wrench },
      { label: '已记录', value: formatNumber(summary.value?.success_total ?? summary.value?.total ?? 0), icon: CheckCircle2 },
      { label: '近 24 小时', value: formatNumber(summary.value?.recent_total ?? 0), icon: AlertTriangle },
      { label: '最近操作', value: formatDateTime(summary.value?.last_event_at), icon: RefreshCw }
    ]
  }
  return [
    { label: '用户操作', value: formatNumber(summary.value?.total ?? 0), icon: Activity },
    { label: '成功操作', value: formatNumber(summary.value?.success_total ?? 0), icon: CheckCircle2 },
    { label: '异常操作', value: formatNumber(summary.value?.failed_total ?? 0), icon: ShieldAlert },
    { label: '最近操作', value: formatDateTime(summary.value?.last_event_at), icon: RefreshCw }
  ]
})

const showMethodFilter = computed(() => activeCategory.value === 'user_operation')
const showLevelFilter = computed(() => activeCategory.value !== 'system_operation')
const showStatusFilter = computed(() => activeCategory.value !== 'system_operation')

function requestParams(targetPage = page.value) {
  return {
    category: activeCategory.value,
    page: targetPage,
    page_size: pageSize.value,
    level: showLevelFilter.value ? filters.level : '',
    method: showMethodFilter.value ? filters.method : '',
    status: showStatusFilter.value && filters.status !== '' ? `${filters.status}` : '',
    keyword: filters.keyword,
    date_from: filters.date_from,
    date_to: filters.date_to
  }
}

function exportParams() {
  const params = requestParams(1)
  delete params.page
  delete params.page_size
  return params
}

async function load(targetPage = page.value) {
  loading.value = true
  errorMessage.value = ''
  try {
    const data = await api.listSystemLogs(requestParams(targetPage))
    logs.value = data.items ?? []
    summary.value = data.summary ?? {}
    total.value = data.total ?? 0
    page.value = data.page ?? targetPage
    pageSize.value = data.page_size ?? pageSize.value
    selectedLog.value = null
    showDetailModal.value = false
  } catch (error) {
    logs.value = []
    total.value = 0
    selectedLog.value = null
    showDetailModal.value = false
    errorMessage.value = error.message
  } finally {
    loading.value = false
  }
}

function switchCategory(category) {
  if (activeCategory.value === category || loading.value) return
  activeCategory.value = category
  resetFilterValues()
  load(1)
}

function queryLogs() {
  load(1)
}

function resetFilterValues() {
  filters.level = ''
  filters.method = ''
  filters.status = ''
  filters.keyword = ''
  filters.date_from = ''
  filters.date_to = ''
}

function resetFilters() {
  resetFilterValues()
  load(1)
}

function selectLog(item) {
  selectedLog.value = item
}

function openDetailModal(item) {
  selectLog(item)
  showDetailModal.value = true
}

function closeDetailModal() {
  showDetailModal.value = false
}

function handleGlobalKeydown(event) {
  if (event.key === 'Escape' && showDetailModal.value) closeDetailModal()
}

function loadPreviousPage() {
  if (loading.value || page.value <= 1) return
  load(page.value - 1)
}

function loadNextPage() {
  if (loading.value || page.value >= totalPages.value) return
  load(page.value + 1)
}

function exportLogs() {
  globalThis.open?.(api.systemLogsExportURL(exportParams()), 'system-logs-export')
}

async function copySelectedLog() {
  if (!selectedLog.value) return
  await globalThis.navigator?.clipboard?.writeText(JSON.stringify(selectedLog.value, null, 2))
}

function resultLabel(item) {
  if (Number(item?.status_code ?? 0) >= 400) return '失败'
  return '成功'
}

function resultClass(item) {
  return Number(item?.status_code ?? 0) >= 400 ? 'is-error' : 'is-info'
}

function levelLabel(level) {
  if (level === 'error') return '错误'
  if (level === 'warn') return '警告'
  return '信息'
}

function levelClass(level) {
  return `is-${level || 'info'}`
}

function actorName(item) {
  return item?.admin_username || item?.user_username || '-'
}

function targetName(item) {
  if (!item?.target_type) return '-'
  return item.target_id ? `${item.target_type} #${item.target_id}` : item.target_type
}

function primaryTitle(item) {
  if (!item) return ''
  if (activeCategory.value === 'system_operation') return `${item.action || '-'} · ${targetName(item)}`
  return `${item.method || '-'} ${item.path || '-'}`
}

function diagnosticText(item) {
  return item?.error_detail || item?.detail || item?.error_message || item?.error_code || '-'
}

function formatNumber(value) {
  return new Intl.NumberFormat('zh-CN').format(Number(value ?? 0))
}

function formatLatency(value) {
  const ms = Number(value ?? 0)
  if (ms >= 1000) return `${(ms / 1000).toFixed(1).replace(/\.0$/, '')}s`
  return `${ms}ms`
}

function formatDateTime(value) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return date.toLocaleString('zh-CN', { hour12: false })
}

onMounted(() => {
  load()
  globalThis.addEventListener?.('keydown', handleGlobalKeydown)
})

onBeforeUnmount(() => {
  globalThis.removeEventListener?.('keydown', handleGlobalKeydown)
})
</script>

<template>
  <section class="admin-page system-logs-page">
    <div class="admin-page-header system-logs-header">
      <div>
        <p class="admin-breadcrumb">后台中心 / 日志管理</p>
        <h1>日志管理</h1>
      </div>
      <div class="header-actions">
        <button class="secondary-button icon-button-text" type="button" :disabled="loading" @click="load(page)">
          <RefreshCw :size="16" />
          刷新
        </button>
        <button data-testid="system-logs-export" class="primary-button icon-button-text" type="button" :disabled="loading" @click="exportLogs">
          <Download :size="16" />
          导出 CSV
        </button>
      </div>
    </div>

    <div class="system-log-tabs" role="tablist" aria-label="日志分类">
      <button
        v-for="item in categories"
        :key="item.key"
        class="system-log-tab"
        :class="{ active: activeCategory === item.key }"
        type="button"
        role="tab"
        :aria-selected="activeCategory === item.key"
        :data-testid="`system-logs-tab-${item.key}`"
        :disabled="loading"
        @click="switchCategory(item.key)"
      >
        <component :is="item.icon" :size="17" />
        <span>{{ item.label }}</span>
      </button>
    </div>

    <div class="admin-kpi-grid log-kpi-grid">
      <div v-for="card in summaryCards" :key="card.label" class="admin-kpi-card">
        <component :is="card.icon" :size="18" />
        <span>{{ card.label }}</span>
        <strong>{{ card.value }}</strong>
      </div>
    </div>

    <div class="admin-filter-bar system-log-filter-bar">
      <ClickSelect v-if="showLevelFilter" v-model="filters.level" :options="levelOptions" data-testid="system-logs-level" class="text-input" aria-label="日志级别" />
      <ClickSelect v-if="showMethodFilter" v-model="filters.method" :options="methodOptions" data-testid="system-logs-method" class="text-input" aria-label="请求方法" />
      <input v-if="showStatusFilter" v-model="filters.status" data-testid="system-logs-status" class="text-input status-input" type="number" placeholder="状态码" />
      <input v-model="filters.keyword" data-testid="system-logs-keyword" class="text-input keyword-input" type="search" :placeholder="activeConfig.keywordPlaceholder" />
      <input v-model="filters.date_from" data-testid="system-logs-date-from" class="text-input date-input" type="date" />
      <input v-model="filters.date_to" data-testid="system-logs-date-to" class="text-input date-input" type="date" />
      <button data-testid="system-logs-query" class="primary-button icon-button-text" type="button" :disabled="loading" @click="queryLogs">
        <Search :size="16" />
        查询
      </button>
      <button data-testid="system-logs-reset" class="secondary-button" type="button" :disabled="loading" @click="resetFilters">重置</button>
    </div>

    <p v-if="errorMessage" class="form-error">{{ errorMessage }}</p>

    <div class="table-panel log-table-panel">
      <div v-if="loading" class="empty-state">加载中...</div>
      <div v-else-if="logs.length === 0" class="empty-state">
        <strong>{{ activeConfig.emptyText }}</strong>
        <span>调整筛选条件后重试</span>
      </div>
      <div v-else class="admin-table-scroll">
        <table class="admin-data-table system-logs-table" :class="`table-${activeCategory}`">
          <thead>
            <tr v-if="activeCategory === 'user_login'">
              <th>时间</th>
              <th>登录结果</th>
              <th>用户</th>
              <th>状态码</th>
              <th>IP</th>
              <th>User-Agent</th>
              <th>错误码</th>
              <th>请求 ID</th>
              <th>操作</th>
            </tr>
            <tr v-else-if="activeCategory === 'user_operation'">
              <th>时间</th>
              <th>级别</th>
              <th>方法</th>
              <th>请求路径</th>
              <th>用户</th>
              <th>状态码</th>
              <th>耗时</th>
              <th>模型/支付诊断</th>
              <th>操作</th>
            </tr>
            <tr v-else>
              <th>时间</th>
              <th>管理员</th>
              <th>动作</th>
              <th>目标对象</th>
              <th>IP</th>
              <th>详情</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in logs" :key="`${item.category}-${item.id}`" :class="{ selected: selectedLog?.id === item.id }">
              <template v-if="activeCategory === 'user_login'">
                <td>{{ formatDateTime(item.created_at) }}</td>
                <td><span class="status-pill" :class="resultClass(item)">{{ resultLabel(item) }}</span></td>
                <td>{{ item.user_username || '-' }}</td>
                <td>{{ item.status_code || '-' }}</td>
                <td>{{ item.ip_address || '-' }}</td>
                <td class="path-cell">{{ item.user_agent || '-' }}</td>
                <td>{{ item.error_code || '-' }}</td>
                <td class="mono-cell">{{ item.request_id || '-' }}</td>
              </template>
              <template v-else-if="activeCategory === 'user_operation'">
                <td>{{ formatDateTime(item.created_at) }}</td>
                <td><span class="status-pill" :class="levelClass(item.level)">{{ levelLabel(item.level) }}</span></td>
                <td>{{ item.method || '-' }}</td>
                <td class="path-cell">{{ item.path || '-' }}</td>
                <td>{{ item.user_username || '-' }}</td>
                <td>{{ item.status_code || '-' }}</td>
                <td>{{ formatLatency(item.duration_ms) }}</td>
                <td class="path-cell">{{ diagnosticText(item) }}</td>
              </template>
              <template v-else>
                <td>{{ formatDateTime(item.created_at) }}</td>
                <td>{{ item.admin_username || '-' }}</td>
                <td>{{ item.action || '-' }}</td>
                <td>{{ targetName(item) }}</td>
                <td>{{ item.ip_address || '-' }}</td>
                <td class="path-cell">{{ item.detail || '-' }}</td>
              </template>
              <td class="system-logs-action-cell">
                <button data-testid="system-logs-view" class="secondary-button log-view-button" type="button" @click="openDetailModal(item)">查看</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="pagination-row">
        <span>第 {{ rangeStart }}-{{ rangeEnd }} 条 / 共 {{ total }} 条</span>
        <div>
          <button data-testid="system-logs-prev" class="secondary-button" type="button" :disabled="loading || page <= 1" @click="loadPreviousPage">上一页</button>
          <button data-testid="system-logs-next" class="secondary-button" type="button" :disabled="loading || page >= totalPages" @click="loadNextPage">下一页</button>
        </div>
      </div>
    </div>

    <div
      v-if="showDetailModal && selectedLog"
      data-testid="system-logs-modal-backdrop"
      class="system-log-modal-backdrop"
      @click.self="closeDetailModal"
    >
      <section
        data-testid="system-logs-detail-modal"
        class="system-log-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="system-log-modal-title"
      >
        <div class="detail-heading">
          <div>
            <p class="section-eyebrow">{{ activeConfig.detailTitle }}</p>
            <h2 id="system-log-modal-title">{{ primaryTitle(selectedLog) }}</h2>
          </div>
          <div class="modal-heading-actions">
            <button data-testid="system-logs-copy" class="secondary-button icon-only-button" type="button" title="复制关键信息" @click="copySelectedLog">
              <Copy :size="16" />
            </button>
            <button data-testid="system-logs-modal-close" class="modal-close-button" type="button" title="关闭" @click="closeDetailModal">
              <X :size="18" />
            </button>
          </div>
        </div>
        <p class="detail-hint">{{ activeConfig.hint }}</p>
        <dl class="detail-list">
          <dt>时间</dt>
          <dd>{{ formatDateTime(selectedLog.created_at) }}</dd>
          <template v-if="activeCategory === 'system_operation'">
            <dt>管理员</dt>
            <dd>{{ selectedLog.admin_username || '-' }}</dd>
            <dt>动作</dt>
            <dd>{{ selectedLog.action || '-' }}</dd>
            <dt>目标对象</dt>
            <dd>{{ targetName(selectedLog) }}</dd>
            <dt>详情 JSON</dt>
            <dd>{{ selectedLog.detail || '-' }}</dd>
          </template>
          <template v-else>
            <dt>请求 ID</dt>
            <dd>{{ selectedLog.request_id || '-' }}</dd>
            <dt>用户</dt>
            <dd>{{ selectedLog.user_username || '-' }}</dd>
            <dt>请求路径</dt>
            <dd>{{ selectedLog.path || '-' }}</dd>
            <dt>User-Agent</dt>
            <dd>{{ selectedLog.user_agent || '-' }}</dd>
            <dt>错误码</dt>
            <dd>{{ selectedLog.error_code || '-' }}</dd>
            <dt>错误消息</dt>
            <dd>{{ selectedLog.error_message || '-' }}</dd>
            <dt>诊断信息</dt>
            <dd>{{ selectedLog.error_detail || '-' }}</dd>
          </template>
          <dt>客户端 IP</dt>
          <dd>{{ selectedLog.ip_address || '-' }}</dd>
        </dl>
      </section>
    </div>
  </section>
</template>

<style scoped>
.system-logs-page {
  display: grid;
  gap: 16px;
}

.system-logs-header {
  gap: 16px;
}

.header-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  justify-content: flex-end;
}

.system-log-tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  border-bottom: 1px solid var(--border);
}

.system-log-tab {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-height: 40px;
  border: 0;
  border-bottom: 2px solid transparent;
  background: transparent;
  color: var(--muted);
  cursor: pointer;
  font-weight: 800;
}

.system-log-tab.active {
  border-color: #2155d6;
  color: #1f2937;
}

.system-log-tab:disabled {
  cursor: not-allowed;
  opacity: 0.62;
}

.log-kpi-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(150px, 1fr));
  gap: 12px;
}

.log-kpi-grid .admin-kpi-card {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 6px 10px;
  align-items: center;
  min-height: 78px;
  padding: 14px 16px;
  border: 1px solid rgba(118, 129, 166, 0.16);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.76);
}

.log-kpi-grid .admin-kpi-card svg {
  color: #2155d6;
}

.log-kpi-grid .admin-kpi-card span {
  color: var(--muted);
  font-size: 0.82rem;
  font-weight: 800;
}

.log-kpi-grid .admin-kpi-card strong {
  grid-column: 1 / -1;
  color: #111827;
  font-size: 1.25rem;
  line-height: 1.1;
}

.system-log-filter-bar {
  display: grid;
  grid-template-columns: minmax(120px, 0.8fr) minmax(120px, 0.8fr) minmax(110px, 0.7fr) minmax(240px, 1.6fr) minmax(140px, 0.9fr) minmax(140px, 0.9fr) auto auto;
  align-items: center;
  gap: 10px;
  margin-bottom: 0;
}

.system-log-filter-bar .keyword-input {
  min-width: 220px;
}

.system-log-filter-bar .status-input,
.system-log-filter-bar .date-input {
  min-width: 0;
}

.log-table-panel {
  min-width: 0;
  width: 100%;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.82);
}

.system-logs-table {
  min-width: 1120px;
}

.table-user_operation {
  min-width: 1200px;
}

.table-system_operation {
  min-width: 980px;
}

.system-logs-table tr.selected {
  background: rgba(33, 85, 214, 0.08);
}

.system-logs-action-cell {
  width: 92px;
  text-align: right;
  white-space: nowrap;
}

.log-view-button {
  min-height: 32px;
  padding: 0 12px;
  font-size: 0.82rem;
}

.path-cell {
  max-width: 360px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mono-cell {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.system-log-modal-backdrop {
  position: fixed;
  inset: 0;
  z-index: 1000;
  display: grid;
  place-items: center;
  padding: 16px;
  background: rgba(17, 24, 39, 0.38);
}

.system-log-modal {
  width: min(720px, calc(100vw - 32px));
  max-height: calc(100vh - 32px);
  overflow: auto;
  padding: 18px;
  border: 1px solid rgba(118, 129, 166, 0.2);
  border-radius: 8px;
  background: #fff;
  box-shadow: 0 24px 70px rgba(15, 23, 42, 0.24);
}

.modal-heading-actions {
  display: flex;
  flex-shrink: 0;
  gap: 8px;
  align-items: center;
}

.modal-close-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  min-width: 36px;
  height: 36px;
  padding: 0;
  border: 1px solid rgba(118, 129, 166, 0.22);
  border-radius: 8px;
  background: #fff;
  color: #374151;
  cursor: pointer;
}

.modal-close-button:hover {
  border-color: rgba(33, 85, 214, 0.34);
  color: #1d4ed8;
}

.detail-heading {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  justify-content: space-between;
}

.detail-heading h2 {
  margin: 4px 0 0;
  overflow-wrap: anywhere;
  font-size: 1rem;
}

.detail-hint {
  margin: 12px 0 16px;
  color: var(--muted);
  font-size: 0.84rem;
  line-height: 1.5;
}

.detail-list {
  display: grid;
  grid-template-columns: minmax(96px, 0.28fr) minmax(0, 1fr);
  gap: 10px 14px;
  margin: 0;
}

.detail-list dt {
  color: var(--muted);
  font-size: 0.78rem;
  font-weight: 800;
}

.detail-list dd {
  margin: 0;
  overflow-wrap: anywhere;
  color: #1f2937;
}

.icon-only-button {
  width: 36px;
  min-width: 36px;
  height: 36px;
  padding: 0;
  justify-content: center;
}

.empty-state {
  display: grid;
  gap: 6px;
}

.empty-state span {
  color: var(--muted);
}

.is-info {
  background: rgba(33, 85, 214, 0.1);
  color: #1d4ed8;
}

.is-warn {
  background: rgba(245, 158, 11, 0.14);
  color: #a15c07;
}

.is-error {
  background: rgba(239, 68, 68, 0.12);
  color: #b42318;
}

@media (max-width: 760px) {
  .header-actions {
    justify-content: flex-start;
  }

  .log-kpi-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .system-log-filter-bar {
    grid-template-columns: 1fr;
  }

  .system-log-filter-bar .keyword-input {
    min-width: 0;
  }

  .system-log-modal {
    padding: 16px;
  }

  .system-log-modal .detail-list {
    grid-template-columns: 1fr;
    gap: 6px;
  }
}
</style>
