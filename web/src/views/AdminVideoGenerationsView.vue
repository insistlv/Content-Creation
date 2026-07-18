<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import {
  AlertTriangle,
  CheckCircle2,
  Clock3,
  Download,
  Eye,
  LoaderCircle,
  RefreshCw,
  Search,
  TimerReset,
  Video,
  X,
  XCircle
} from 'lucide-vue-next'

import { api } from '../api/client.js'

const records = ref([])
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const summary = ref({})
const loading = ref(false)
const errorMessage = ref('')
const detailLoading = ref(false)
const detailError = ref('')
const detailModalOpen = ref(false)
const selectedDetail = ref(null)
let detailRequestToken = 0

const filters = reactive({
  q: '',
  source: '',
  provider: '',
  runtime_model: '',
  status: '',
  date_from: '',
  date_to: ''
})

const statusOptions = [
  { value: '', label: '全部状态' },
  { value: 'succeeded', label: '成功' },
  { value: 'failed', label: '失败' },
  { value: 'running', label: '进行中' },
  { value: 'queued', label: '排队中' }
]

const sourceOptions = [
  { value: '', label: '全部来源' },
  { value: 'workspace', label: '普通视频' },
  { value: 'novel_shot', label: '小说分镜' }
]

const kpiCards = computed(() => [
  {
    key: 'today_videos',
    label: '今日视频',
    icon: Video,
    value: formatNumber(summary.value?.today_videos ?? 0),
    delta: summary.value?.today_videos_delta_percent ?? 0
  },
  {
    key: 'success_rate',
    label: '成功率',
    icon: CheckCircle2,
    value: formatPercent(summary.value?.success_rate ?? 0),
    delta: summary.value?.success_rate_delta_percent ?? 0
  },
  {
    key: 'average_latency_ms',
    label: '平均耗时',
    icon: Clock3,
    value: formatLatency(summary.value?.average_latency_ms ?? 0),
    delta: summary.value?.average_latency_delta_percent ?? 0
  },
  {
    key: 'failed_tasks',
    label: '失败任务',
    icon: AlertTriangle,
    value: formatNumber(summary.value?.failed_tasks ?? 0),
    delta: summary.value?.failed_tasks_delta_percent ?? 0
  }
])

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value || 1)))
const selectedVideo = computed(() => selectedDetail.value?.result_video ?? null)

function applyDefaultDateRange() {
  const end = new Date()
  const start = new Date(end)
  start.setDate(start.getDate() - 6)
  filters.date_from = formatDateInput(start)
  filters.date_to = formatDateInput(end)
}

function requestParams(targetPage = page.value) {
  return {
    q: filters.q,
    source: filters.source,
    provider: filters.provider,
    runtime_model: filters.runtime_model,
    status: filters.status,
    date_from: filters.date_from,
    date_to: filters.date_to,
    page: targetPage,
    page_size: pageSize.value
  }
}

function exportParams() {
  return {
    q: filters.q,
    source: filters.source,
    provider: filters.provider,
    runtime_model: filters.runtime_model,
    status: filters.status,
    date_from: filters.date_from,
    date_to: filters.date_to
  }
}

async function load(targetPage = page.value) {
  clearDetailState()
  loading.value = true
  errorMessage.value = ''
  try {
    const data = await api.listVideoGenerations(requestParams(targetPage))
    records.value = data.items ?? []
    summary.value = data.summary ?? {}
    total.value = data.total ?? 0
    page.value = data.page ?? targetPage
    pageSize.value = data.page_size ?? pageSize.value
  } catch (error) {
    errorMessage.value = error.message || '视频记录读取失败'
  } finally {
    loading.value = false
  }
}

function clearDetailState() {
  detailRequestToken += 1
  detailLoading.value = false
  detailError.value = ''
  detailModalOpen.value = false
  selectedDetail.value = null
}

async function openVideoGenerationDetail(item) {
  if (!item?.id) return
  const requestToken = detailRequestToken + 1
  detailRequestToken = requestToken
  detailModalOpen.value = true
  detailLoading.value = true
  detailError.value = ''
  selectedDetail.value = null
  try {
    const data = await api.getAdminVideoGeneration(item.id)
    if (requestToken !== detailRequestToken) return
    selectedDetail.value = data
  } catch (error) {
    if (requestToken !== detailRequestToken) return
    detailError.value = error.message || '视频记录详情读取失败'
  } finally {
    if (requestToken === detailRequestToken) detailLoading.value = false
  }
}

function closeDetailModal() {
  clearDetailState()
}

function queryVideoGenerations() {
  load(1)
}

function resetFilters() {
  filters.q = ''
  filters.source = ''
  filters.provider = ''
  filters.runtime_model = ''
  filters.status = ''
  applyDefaultDateRange()
  load(1)
}

function loadPreviousPage() {
  if (loading.value || page.value <= 1) return
  load(page.value - 1)
}

function loadNextPage() {
  if (loading.value || page.value * pageSize.value >= total.value) return
  load(page.value + 1)
}

function exportVideoGenerations() {
  window.open(api.videoGenerationExportURL(exportParams()), 'video-generations-export')
}

function downloadDetailVideo() {
  const url = selectedVideo.value?.download_url || selectedVideo.value?.preview_url
  if (url) window.open(url, '_blank')
}

function handleGlobalKeydown(event) {
  if (event.key === 'Escape' && detailModalOpen.value) closeDetailModal()
}

function statusMeta(status) {
  switch (status) {
    case 'succeeded':
      return { label: '成功', className: 'is-success', icon: CheckCircle2 }
    case 'failed':
      return { label: '失败', className: 'is-failed', icon: XCircle }
    case 'queued':
      return { label: '排队中', className: 'is-running', icon: LoaderCircle }
    case 'running':
      return { label: '进行中', className: 'is-running', icon: LoaderCircle }
    default:
      return { label: status || '未知', className: 'is-muted', icon: TimerReset }
  }
}

function sourceLabel(source) {
  if (source === 'novel_shot') return '小说分镜'
  if (source === 'workspace') return '普通视频'
  return source || '-'
}

function userName(user, fallbackID) {
  return user?.display_name || user?.username || (fallbackID ? `用户 ${fallbackID}` : '-')
}

function modelName(item) {
  return item?.model_name || item?.runtime_model || '-'
}

function formatNumber(value) {
  return new Intl.NumberFormat('zh-CN').format(Number(value ?? 0))
}

function formatPercent(value) {
  return `${Number(value ?? 0).toFixed(1).replace(/\.0$/, '')}%`
}

function formatLatency(value) {
  const latency = Number(value ?? 0)
  if (latency >= 1000) return `${(latency / 1000).toFixed(1).replace(/\.0$/, '')}s`
  return `${latency}ms`
}

function formatDelta(value) {
  const numeric = Number(value ?? 0)
  if (numeric === 0) return '0%'
  return `${numeric > 0 ? '+' : ''}${numeric.toFixed(1).replace(/\.0$/, '')}%`
}

function formatDate(value) {
  if (!value) return '-'
  return new Date(value).toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  })
}

function formatDateInput(value) {
  const year = value.getFullYear()
  const month = `${value.getMonth() + 1}`.padStart(2, '0')
  const day = `${value.getDate()}`.padStart(2, '0')
  return `${year}-${month}-${day}`
}

onMounted(() => {
  applyDefaultDateRange()
  load(1)
  document.addEventListener('keydown', handleGlobalKeydown)
})

onBeforeUnmount(() => {
  document.removeEventListener('keydown', handleGlobalKeydown)
})
</script>

<template>
  <section class="admin-video-generations-view">
    <header class="admin-page-header">
      <div>
        <p class="eyebrow">后台中心 / 视频记录</p>
        <h1>视频记录</h1>
      </div>
      <button class="admin-secondary-button" data-testid="video-generation-export" type="button" @click="exportVideoGenerations">
        <Download :size="16" />
        <span>导出</span>
      </button>
    </header>

    <div class="video-kpi-grid">
      <article v-for="card in kpiCards" :key="card.key" class="video-kpi-card">
        <component :is="card.icon" :size="20" />
        <div>
          <span>{{ card.label }}</span>
          <strong>{{ card.value }}</strong>
          <small>{{ formatDelta(card.delta) }}</small>
        </div>
      </article>
    </div>

    <form class="video-filter-bar" @submit.prevent="queryVideoGenerations">
      <label>
        <span>关键词</span>
        <input v-model.trim="filters.q" type="search" placeholder="提示词 / provider task id" />
      </label>
      <label>
        <span>来源</span>
        <select v-model="filters.source">
          <option v-for="option in sourceOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
        </select>
      </label>
      <label>
        <span>Provider</span>
        <input v-model.trim="filters.provider" type="text" placeholder="Wuyin" />
      </label>
      <label>
        <span>状态</span>
        <select v-model="filters.status">
          <option v-for="option in statusOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
        </select>
      </label>
      <label>
        <span>开始日期</span>
        <input v-model="filters.date_from" type="date" />
      </label>
      <label>
        <span>结束日期</span>
        <input v-model="filters.date_to" type="date" />
      </label>
      <div class="video-filter-actions">
        <button class="admin-primary-button" type="submit">
          <Search :size="16" />
          <span>查询</span>
        </button>
        <button class="admin-secondary-button" type="button" @click="resetFilters">
          <RefreshCw :size="16" />
          <span>重置</span>
        </button>
      </div>
    </form>

    <p v-if="errorMessage" class="admin-error">{{ errorMessage }}</p>

    <div class="video-table-wrap">
      <table>
        <thead>
          <tr>
            <th>预览</th>
            <th>用户</th>
            <th>提示词</th>
            <th>来源</th>
            <th>模型</th>
            <th>Provider</th>
            <th>参数</th>
            <th>状态</th>
            <th>耗时</th>
            <th>创建时间</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="loading">
            <td colspan="11">加载中...</td>
          </tr>
          <tr v-else-if="records.length === 0">
            <td colspan="11">暂无视频记录</td>
          </tr>
          <tr v-for="item in records" v-else :key="item.id">
            <td>
              <video v-if="item.preview_url" class="video-thumb" :src="item.preview_url" muted playsinline />
              <div v-else class="video-thumb is-empty"><Video :size="18" /></div>
            </td>
            <td>
              <strong>{{ userName(item.user, item.user_id) }}</strong>
              <small>{{ item.user?.username || '-' }}</small>
            </td>
            <td class="prompt-cell">{{ item.prompt_summary || '-' }}</td>
            <td>{{ sourceLabel(item.source) }}</td>
            <td>
              <strong>{{ modelName(item) }}</strong>
              <small>{{ item.runtime_model }}</small>
            </td>
            <td>
              <strong>{{ item.provider || '-' }}</strong>
              <small>{{ item.provider_request_id || '-' }}</small>
            </td>
            <td>{{ item.duration_seconds || '-' }}s / {{ item.aspect_ratio || '-' }}</td>
            <td>
              <span class="status-pill" :class="statusMeta(item.status).className">
                <component :is="statusMeta(item.status).icon" :size="14" />
                {{ statusMeta(item.status).label }}
              </span>
            </td>
            <td>{{ formatLatency(item.latency_ms) }}</td>
            <td>{{ formatDate(item.created_at) }}</td>
            <td>
              <button class="icon-action" :data-testid="`video-generation-view-${item.id}`" type="button" aria-label="查看视频记录" @click="openVideoGenerationDetail(item)">
                <Eye :size="16" />
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <footer class="video-pagination">
      <span>第 {{ page }} / {{ totalPages }} 页，共 {{ formatNumber(total) }} 条</span>
      <div>
        <button class="admin-secondary-button" type="button" :disabled="page <= 1 || loading" @click="loadPreviousPage">上一页</button>
        <button class="admin-secondary-button" type="button" :disabled="page >= totalPages || loading" @click="loadNextPage">下一页</button>
      </div>
    </footer>

    <div v-if="detailModalOpen" class="modal-backdrop">
      <section class="video-detail-modal" data-testid="video-generation-detail-modal">
        <header>
          <div>
            <p class="eyebrow">视频记录详情</p>
            <h2>{{ selectedDetail ? `VID-${selectedDetail.id}` : '加载中' }}</h2>
          </div>
          <button class="icon-action" type="button" aria-label="关闭详情" @click="closeDetailModal">
            <X :size="18" />
          </button>
        </header>
        <p v-if="detailLoading">加载中...</p>
        <p v-else-if="detailError" class="admin-error">{{ detailError }}</p>
        <div v-else-if="selectedDetail" class="video-detail-body">
          <video v-if="selectedVideo?.preview_url" class="detail-video" :src="selectedVideo.preview_url" controls playsinline />
          <div v-else class="detail-video is-empty"><Video :size="28" /></div>
          <div class="detail-actions">
            <button class="admin-secondary-button" type="button" @click="downloadDetailVideo">
              <Download :size="16" />
              <span>下载</span>
            </button>
          </div>
          <dl>
            <div>
              <dt>Provider Task ID</dt>
              <dd>{{ selectedDetail.provider_request_id || '-' }}</dd>
            </div>
            <div>
              <dt>模型</dt>
              <dd>{{ modelName(selectedDetail) }}</dd>
            </div>
            <div>
              <dt>来源</dt>
              <dd>{{ sourceLabel(selectedDetail.source) }}</dd>
            </div>
            <div>
              <dt>参数</dt>
              <dd>{{ selectedDetail.duration_seconds || '-' }}s / {{ selectedDetail.aspect_ratio || '-' }}</dd>
            </div>
            <div>
              <dt>提示词</dt>
              <dd>{{ selectedDetail.prompt || '-' }}</dd>
            </div>
            <div v-if="selectedDetail.error">
              <dt>失败原因</dt>
              <dd>{{ selectedDetail.error.message || selectedDetail.error.code }}</dd>
            </div>
          </dl>
          <section v-if="selectedDetail.reference_images?.length" class="reference-strip">
            <h3>参考图</h3>
            <img v-for="image in selectedDetail.reference_images" :key="image.reference_asset_id" :src="image.preview_url" :alt="image.original_filename || 'reference image'" />
          </section>
          <section v-if="selectedDetail.novel_video" class="detail-section">
            <h3>小说分镜</h3>
            <p>{{ selectedDetail.novel_video.title || '-' }} / {{ selectedDetail.novel_video.shot_title || '-' }}</p>
          </section>
        </div>
      </section>
    </div>
  </section>
</template>

<style scoped>
.admin-video-generations-view {
  display: grid;
  gap: 18px;
}

.admin-page-header,
.video-filter-bar,
.video-table-wrap,
.video-pagination,
.video-detail-modal {
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  box-shadow: 0 16px 42px rgba(15, 23, 42, 0.06);
}

.admin-page-header,
.video-pagination {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 18px;
}

.admin-page-header h1,
.video-detail-modal h2 {
  margin: 0;
  color: #111827;
  font-size: 24px;
}

.eyebrow {
  margin: 0 0 4px;
  color: #64748b;
  font-size: 13px;
}

.video-kpi-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.video-kpi-card {
  display: flex;
  gap: 12px;
  align-items: center;
  min-height: 92px;
  padding: 16px;
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
}

.video-kpi-card svg {
  color: #4f46e5;
}

.video-kpi-card span,
.video-kpi-card small,
td small {
  display: block;
  color: #64748b;
  font-size: 12px;
}

.video-kpi-card strong {
  display: block;
  margin: 4px 0;
  color: #111827;
  font-size: 24px;
}

.video-filter-bar {
  display: grid;
  grid-template-columns: 1.8fr repeat(5, minmax(128px, 1fr)) auto;
  gap: 12px;
  align-items: end;
  padding: 16px;
}

.video-filter-bar label {
  display: grid;
  gap: 6px;
  color: #475569;
  font-size: 12px;
}

.video-filter-bar input,
.video-filter-bar select {
  height: 38px;
  padding: 0 10px;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  background: #fff;
  color: #111827;
}

.video-filter-actions {
  display: flex;
  gap: 8px;
}

.admin-primary-button,
.admin-secondary-button,
.icon-action {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-height: 36px;
  border-radius: 6px;
  border: 1px solid transparent;
  cursor: pointer;
  font-weight: 700;
}

.admin-primary-button {
  padding: 0 14px;
  background: #4f46e5;
  color: #fff;
}

.admin-secondary-button {
  padding: 0 12px;
  background: #f8fafc;
  color: #334155;
  border-color: #d1d5db;
}

.icon-action {
  width: 36px;
  background: #f8fafc;
  color: #334155;
  border-color: #d1d5db;
}

.admin-secondary-button:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

.admin-error {
  margin: 0;
  padding: 12px 14px;
  border: 1px solid #fecaca;
  border-radius: 8px;
  background: #fef2f2;
  color: #b91c1c;
}

.video-table-wrap {
  overflow: auto;
}

table {
  width: 100%;
  border-collapse: collapse;
}

th,
td {
  padding: 12px;
  border-bottom: 1px solid #e5e7eb;
  text-align: left;
  color: #334155;
  vertical-align: middle;
  white-space: nowrap;
}

th {
  color: #64748b;
  font-size: 12px;
  font-weight: 800;
}

.prompt-cell {
  max-width: 260px;
  white-space: normal;
}

.video-thumb {
  width: 96px;
  height: 54px;
  object-fit: cover;
  border-radius: 6px;
  background: #111827;
}

.video-thumb.is-empty,
.detail-video.is-empty {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: #64748b;
  background: #f1f5f9;
}

.status-pill {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 5px 8px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 800;
}

.status-pill.is-success {
  color: #047857;
  background: #d1fae5;
}

.status-pill.is-failed {
  color: #b91c1c;
  background: #fee2e2;
}

.status-pill.is-running {
  color: #1d4ed8;
  background: #dbeafe;
}

.status-pill.is-muted {
  color: #475569;
  background: #e2e8f0;
}

.modal-backdrop {
  position: fixed;
  inset: 0;
  z-index: 60;
  display: grid;
  place-items: center;
  padding: 24px;
  background: rgba(15, 23, 42, 0.42);
}

.video-detail-modal {
  width: min(900px, 100%);
  max-height: calc(100vh - 48px);
  overflow: auto;
}

.video-detail-modal > header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px;
  border-bottom: 1px solid #e5e7eb;
}

.video-detail-body {
  display: grid;
  gap: 14px;
  padding: 16px;
}

.detail-video {
  width: 100%;
  aspect-ratio: 16 / 9;
  max-height: 460px;
  object-fit: contain;
  border-radius: 8px;
  background: #111827;
}

.detail-actions {
  display: flex;
  justify-content: flex-end;
}

dl {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  margin: 0;
}

dt {
  color: #64748b;
  font-size: 12px;
  font-weight: 800;
}

dd {
  margin: 4px 0 0;
  color: #111827;
  word-break: break-word;
}

.reference-strip,
.detail-section {
  display: grid;
  gap: 8px;
}

.reference-strip h3,
.detail-section h3 {
  margin: 0;
  color: #111827;
  font-size: 15px;
}

.reference-strip img {
  width: 96px;
  height: 96px;
  object-fit: cover;
  border-radius: 6px;
  border: 1px solid #e5e7eb;
}

@media (max-width: 960px) {
  .video-kpi-grid,
  .video-filter-bar,
  dl {
    grid-template-columns: 1fr;
  }

  .admin-page-header,
  .video-pagination {
    align-items: flex-start;
    flex-direction: column;
    gap: 12px;
  }
}
</style>
