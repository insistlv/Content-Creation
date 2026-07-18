<script setup>
import { computed, markRaw, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import {
  AlertTriangle,
  CheckCircle2,
  Clock3,
  Download,
  Eye,
  Image as ImageIcon,
  LoaderCircle,
  RefreshCw,
  Search,
  TimerReset,
  X,
  XCircle
} from 'lucide-vue-next'

import { api } from '../api/client.js'
import ClickSelect from '../components/ClickSelect.vue'

const records = ref([])
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const loading = ref(false)
const detailLoading = ref(false)
const errorMessage = ref('')
const detailError = ref('')
const selectedId = ref(null)
const selectedDetail = ref(null)
const detailModalOpen = ref(false)
const mediaPreview = ref(null)
const summary = ref({})
let detailRequestToken = 0

const filters = reactive({
  q: '',
  model: '',
  user_keyword: '',
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

const kpiDefinitions = [
  { key: 'today_generations', delta: 'today_generations_delta_percent', label: '今日生成', icon: markRaw(ImageIcon), format: formatNumber },
  { key: 'success_rate', delta: 'success_rate_delta_percent', label: '成功率', icon: markRaw(CheckCircle2), format: formatPercent },
  { key: 'average_latency_ms', delta: 'average_latency_delta_percent', label: '平均耗时', icon: markRaw(Clock3), format: formatLatency },
  { key: 'failed_tasks', delta: 'failed_tasks_delta_percent', label: '失败任务', icon: markRaw(AlertTriangle), format: formatNumber }
]

const modelOptions = computed(() => {
  const options = []
  const seen = new Set()
  const addOption = (value, label) => {
    if (!value || seen.has(value)) return
    seen.add(value)
    options.push({ value, label: label || value })
  }
  addOption('gpt-image-2', 'gpt-image-2')
  addOption('gpt-image-2-2026-04-21', 'gpt-image-2-2026-04-21')
  records.value.forEach((item) => {
    if (item.channel_id) {
      addOption(`channel:${item.channel_id}`, channelOptionLabel(item))
    }
    if (item.model_id) {
      addOption(`model:${item.model_id}`, modelDisplayName(item))
      return
    }
    if (item.model_config_id) {
      addOption(`id:${item.model_config_id}`, modelOptionLabel(item))
      return
    }
    if (item.model) addOption(item.model, item.model)
  })
  if (filters.model && !String(filters.model).startsWith('id:')) addOption(filters.model, filters.model)
  return options
})

const kpiCards = computed(() => kpiDefinitions.map((card) => {
  const delta = Number(summary.value?.[card.delta] ?? 0)
  return {
    ...card,
    value: card.format(summary.value?.[card.key] ?? 0),
    delta,
    deltaText: formatDelta(delta)
  }
}))

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value || 1)))
const rangeStart = computed(() => {
  if (total.value === 0 || records.value.length === 0) return 0
  return (page.value - 1) * pageSize.value + 1
})
const rangeEnd = computed(() => {
  if (rangeStart.value === 0) return 0
  return rangeStart.value + records.value.length - 1
})
const selectedResultLinks = computed(() => {
  const images = selectedDetail.value?.result_images ?? []
  return images
    .map((image) => image.download_url || image.preview_url)
    .filter(Boolean)
})

function applyDefaultDateRange() {
  const end = new Date()
  const start = new Date(end)
  start.setDate(start.getDate() - 6)
  filters.date_from = formatDateInput(start)
  filters.date_to = formatDateInput(end)
}

function requestParams(targetPage = page.value) {
  const modelFilter = modelFilterParams()
  return {
    q: filters.q,
    model: modelFilter.model,
    model_id: modelFilter.model_id,
    channel_id: modelFilter.channel_id,
    model_config_id: modelFilter.model_config_id,
    user_keyword: filters.user_keyword,
    status: filters.status,
    date_from: filters.date_from,
    date_to: filters.date_to,
    page: targetPage,
    page_size: pageSize.value
  }
}

function exportParams() {
  const modelFilter = modelFilterParams()
  return {
    q: filters.q,
    model: modelFilter.model,
    model_id: modelFilter.model_id,
    channel_id: modelFilter.channel_id,
    model_config_id: modelFilter.model_config_id,
    user_keyword: filters.user_keyword,
    status: filters.status,
    date_from: filters.date_from,
    date_to: filters.date_to
  }
}

function modelFilterParams() {
  const value = String(filters.model || '')
  if (value.startsWith('id:')) {
    return {
      model: '',
      model_id: '',
      channel_id: '',
      model_config_id: value.slice(3)
    }
  }
  if (value.startsWith('model:')) {
    return {
      model: '',
      model_id: value.slice(6),
      channel_id: '',
      model_config_id: ''
    }
  }
  if (value.startsWith('channel:')) {
    return {
      model: '',
      model_id: '',
      channel_id: value.slice(8),
      model_config_id: ''
    }
  }
  return {
    model: value,
    model_id: '',
    channel_id: '',
    model_config_id: ''
  }
}

async function load(targetPage = page.value) {
  clearDetailState()
  loading.value = true
  errorMessage.value = ''
  try {
    const data = await api.listGenerations(requestParams(targetPage))
    records.value = data.items ?? []
    summary.value = data.summary ?? {}
    total.value = data.total ?? 0
    page.value = data.page ?? targetPage
    pageSize.value = data.page_size ?? pageSize.value
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    loading.value = false
  }
}

function clearDetailState() {
  detailRequestToken += 1
  selectedId.value = null
  selectedDetail.value = null
  detailError.value = ''
  detailLoading.value = false
  detailModalOpen.value = false
  mediaPreview.value = null
}

async function openGenerationDetail(item) {
  if (!item?.id) return
  const requestToken = detailRequestToken + 1
  detailRequestToken = requestToken
  selectedId.value = item.id
  selectedDetail.value = null
  detailLoading.value = true
  detailError.value = ''
  detailModalOpen.value = true
  try {
    const data = await api.getGeneration(item.id)
    if (requestToken !== detailRequestToken) return
    selectedDetail.value = data
  } catch (error) {
    if (requestToken !== detailRequestToken) return
    detailError.value = error.message
    selectedDetail.value = null
  } finally {
    if (requestToken === detailRequestToken) detailLoading.value = false
  }
}

function closeDetailModal() {
  clearDetailState()
}

function handleGlobalKeydown(event) {
  if (event.key !== 'Escape') return
  if (mediaPreview.value) {
    closeMediaPreview()
    return
  }
  if (detailModalOpen.value) closeDetailModal()
}

function queryGenerations() {
  load(1)
}

function resetFilters() {
  filters.q = ''
  filters.model = ''
  filters.user_keyword = ''
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

function exportGenerations() {
  window.open(api.generationExportURL(exportParams()), 'generations-export')
}

function downloadAllResults() {
  selectedResultLinks.value.forEach((url) => window.open(url, '_blank'))
}

function isVideoResult(item) {
  return String(item?.mime_type || '').startsWith('video/')
}

function mediaLabel(item) {
  return isVideoResult(item) ? '视频' : '图片'
}

function mediaSource(item) {
  return item?.preview_url || item?.download_url || ''
}

function referenceMediaLabel(item, index = 0) {
  return item?.original_filename || `参考图 ${Number(item?.sort_order ?? index) + 1}`
}

function openMediaPreview(item, title = mediaLabel(item)) {
  const src = mediaSource(item)
  if (!src) return
  mediaPreview.value = {
    src,
    downloadURL: item?.download_url || '',
    isVideo: isVideoResult(item),
    title
  }
}

function closeMediaPreview() {
  mediaPreview.value = null
}

function modelDisplayName(item) {
  return item?.model_name || item?.model || '-'
}

function modelRuntimeText(item) {
  const runtimeModel = item?.runtime_model || (item?.model_name ? item?.model : '')
  if (!runtimeModel || runtimeModel === modelDisplayName(item)) return ''
  return runtimeModel
}

function channelDisplayText(item) {
  return item?.channel_name || (item?.channel_id ? `渠道 #${item.channel_id}` : '')
}

function modelOptionLabel(item) {
  const runtimeModel = modelRuntimeText(item)
  return runtimeModel ? `${modelDisplayName(item)} · ${runtimeModel}` : modelDisplayName(item)
}

function channelOptionLabel(item) {
  const channel = channelDisplayText(item)
  const model = modelOptionLabel(item)
  return channel ? `${model} / ${channel}` : model
}

function providerDiagnostics(detail = selectedDetail.value) {
  return detail?.provider_diagnostics ?? {}
}

function hasProviderDiagnostics(detail = selectedDetail.value) {
  const diagnostics = providerDiagnostics(detail)
  return Boolean(
    Number(diagnostics.provider_http_status ?? 0) > 0 ||
    diagnostics.provider_error_code ||
    diagnostics.provider_error_message ||
    diagnostics.provider_failure_stage ||
    Number(diagnostics.provider_attempt_count ?? 0) > 0
  )
}

function formatHTTPStatus(value) {
  const status = Number(value ?? 0)
  return status > 0 ? `HTTP ${status}` : '-'
}

function formatEventMetadata(metadata) {
  if (!metadata || typeof metadata !== 'object') return ''
  return Object.entries(metadata)
    .filter(([, value]) => value !== undefined && value !== null && value !== '')
    .map(([key, value]) => `${key}: ${formatMetadataValue(value)}`)
    .join(' · ')
}

function formatMetadataValue(value) {
  if (Array.isArray(value)) return value.join(', ')
  if (value && typeof value === 'object') return JSON.stringify(value)
  return `${value}`
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

function userName(user, fallbackID) {
  return user?.display_name || user?.username || (fallbackID ? `用户 ${fallbackID}` : '-')
}

function userInitial(user, fallbackID) {
  const text = userName(user, fallbackID)
  return text.slice(0, 1).toUpperCase()
}

function formatNumber(value) {
  return new Intl.NumberFormat('zh-CN').format(Number(value ?? 0))
}

function formatPercent(value) {
  return `${Number(value ?? 0).toFixed(1).replace(/\.0$/, '')}%`
}

function formatLatency(value) {
  const latency = Number(value ?? 0)
  if (latency >= 1000) {
    return `${(latency / 1000).toFixed(1).replace(/\.0$/, '')}s`
  }
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

applyDefaultDateRange()
onMounted(() => {
  load(1)
  globalThis.addEventListener?.('keydown', handleGlobalKeydown)
})

onBeforeUnmount(() => {
  globalThis.removeEventListener?.('keydown', handleGlobalKeydown)
})
</script>

<template>
  <section class="admin-generations-page">
    <div class="generation-page-heading">
      <div>
        <p class="admin-breadcrumb">后台中心 / 生成记录</p>
        <h1>生成记录</h1>
      </div>
      <button class="secondary-button compact-button generation-export-button" type="button" data-testid="generations-export" @click="exportGenerations">
        <Download :size="16" />
        <span>导出 CSV</span>
      </button>
    </div>

    <div class="generation-kpi-grid">
      <article v-for="card in kpiCards" :key="card.key" class="generation-kpi-card">
        <div class="generation-kpi-label">
          <component :is="card.icon" :size="18" />
          <span>{{ card.label }}</span>
        </div>
        <strong>{{ card.value }}</strong>
        <small :class="{ negative: card.delta < 0 }">较昨日 {{ card.deltaText }}</small>
      </article>
    </div>

    <div class="generations-filter-panel">
      <label class="generation-field generation-field-wide">
        <span>提示词关键词</span>
        <div class="generation-input-shell">
          <Search :size="16" />
          <input v-model="filters.q" data-testid="generations-q" type="search" placeholder="搜索提示词" @keyup.enter="queryGenerations" />
        </div>
      </label>
      <label class="generation-field">
        <span>模型</span>
        <ClickSelect v-model="filters.model" :options="[{ value: '', label: '全部模型' }, ...modelOptions]" data-testid="generations-model" aria-label="模型" />
      </label>
      <label class="generation-field">
        <span>用户</span>
        <input v-model="filters.user_keyword" data-testid="generations-user-keyword" placeholder="用户名或完整手机号" />
      </label>
      <label class="generation-field">
        <span>任务状态</span>
        <ClickSelect v-model="filters.status" :options="statusOptions" data-testid="generations-status" aria-label="任务状态" />
      </label>
      <label class="generation-field">
        <span>开始日期</span>
        <input v-model="filters.date_from" data-testid="generations-date-from" type="date" />
      </label>
      <label class="generation-field">
        <span>结束日期</span>
        <input v-model="filters.date_to" data-testid="generations-date-to" type="date" />
      </label>
      <div class="generation-filter-actions">
        <button class="mini-button" type="button" data-testid="generations-reset" :disabled="loading" @click="resetFilters">
          <RefreshCw :size="15" />
          <span>重置</span>
        </button>
        <button class="primary-button compact-button" type="button" data-testid="generations-query" :disabled="loading" @click="queryGenerations">
          <Search :size="16" />
          <span>查询</span>
        </button>
      </div>
    </div>

    <div class="generations-workspace">
      <div class="generations-list-panel">
        <div class="generations-table-scroll">
          <table class="generation-data-table">
            <thead>
              <tr>
                <th>用户</th>
                <th>内容预览</th>
                <th>提示词摘要</th>
                <th>模型</th>
                <th>任务状态</th>
                <th>耗时</th>
                <th>点数消耗</th>
                <th>时间</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="item in records"
                :key="item.id"
                :class="{ selected: selectedId === item.id }"
              >
                <td>
                  <div class="generation-user-cell">
                    <span>{{ userInitial(item.user, item.user_id) }}</span>
                    <div>
                      <strong>{{ userName(item.user, item.user_id) }}</strong>
                      <small>#{{ item.user_id }}</small>
                    </div>
                  </div>
                </td>
                <td>
                  <div v-if="item.preview_images?.length" class="generation-preview-strip">
                    <template v-for="image in item.preview_images" :key="image.preview_url || image.download_url">
                      <video
                        v-if="isVideoResult(image)"
                        class="generation-preview-thumb"
                        :src="image.preview_url || image.download_url"
                        muted
                        playsinline
                      />
                      <img
                        v-else
                        class="generation-preview-thumb"
                        :src="image.preview_url || image.download_url"
                        alt=""
                      />
                    </template>
                  </div>
                  <span v-else class="generation-no-image">无结果</span>
                </td>
                <td>
                  <p class="generation-prompt-summary">{{ item.prompt_summary || '-' }}</p>
                </td>
                <td>
                  <span class="generation-model">
                    <strong>{{ modelDisplayName(item) }}</strong>
                    <small v-if="channelDisplayText(item)">{{ channelDisplayText(item) }}</small>
                    <small v-if="modelRuntimeText(item)">{{ modelRuntimeText(item) }}</small>
                  </span>
                </td>
                <td>
                  <span class="generation-status-pill" :class="statusMeta(item.status).className">
                    <component :is="statusMeta(item.status).icon" :size="14" />
                    {{ statusMeta(item.status).label }}
                  </span>
                </td>
                <td>{{ formatLatency(item.latency_ms) }}</td>
                <td>{{ item.credits_cost ?? 0 }}</td>
                <td>{{ formatDate(item.created_at) }}</td>
                <td>
                  <button class="mini-button icon-button-text" type="button" :data-testid="`generation-view-${item.id}`" @click="openGenerationDetail(item)">
                    <Eye :size="15" />
                    <span>查看</span>
                  </button>
                </td>
              </tr>
              <tr v-if="!loading && records.length === 0">
                <td colspan="9">
                  <div class="generation-empty-state">暂无生成记录</div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div class="admin-generations-footer">
          <p class="helper-text">第 {{ rangeStart }}-{{ rangeEnd }} 条 / 共 {{ total }} 条</p>
          <div class="admin-generations-pagination">
            <button
              data-testid="generations-prev"
              class="mini-button"
              type="button"
              :disabled="loading || page === 1"
              @click="loadPreviousPage"
            >
              上一页
            </button>
            <span class="helper-text">第 {{ page }} / {{ totalPages }} 页</span>
            <button
              data-testid="generations-next"
              class="mini-button"
              type="button"
              :disabled="loading || page * pageSize >= total"
              @click="loadNextPage"
            >
              下一页
            </button>
          </div>
        </div>
        <p v-if="loading" class="page-status">加载中...</p>
      </div>

    </div>

    <div
      v-if="detailModalOpen"
      class="generation-detail-modal-backdrop"
      data-testid="generation-detail-modal-backdrop"
      @click.self="closeDetailModal"
    >
      <section
        class="generation-detail-modal"
        data-testid="generation-detail-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="generation-detail-modal-title"
      >
        <div class="generation-detail-head">
          <div>
            <p class="admin-breadcrumb">任务详情</p>
            <h2 id="generation-detail-modal-title">{{ selectedDetail?.task_id || (selectedId ? `GEN-${selectedId}` : '-') }}</h2>
          </div>
          <div class="generation-detail-actions">
            <button
              class="mini-button icon-button-text"
              type="button"
              data-testid="generation-download-all"
              :disabled="selectedResultLinks.length === 0"
              @click="downloadAllResults"
            >
              <Download :size="15" />
              <span>下载全部结果</span>
            </button>
            <button
              class="generation-detail-close"
              type="button"
              data-testid="generation-detail-modal-close"
              title="关闭"
              aria-label="关闭生成记录详情"
              @click="closeDetailModal"
            >
              <X :size="18" />
            </button>
          </div>
        </div>

        <div class="generation-detail-content">
          <p v-if="detailLoading" class="page-status">详情加载中...</p>
          <p v-else-if="detailError" class="status-error">{{ detailError }}</p>
          <div v-else-if="selectedDetail" class="generation-detail-body">
            <div class="generation-detail-meta">
              <span>用户<strong>{{ userName(selectedDetail.user, selectedDetail.user_id) }}</strong></span>
              <span>状态<strong>{{ statusMeta(selectedDetail.status).label }}</strong></span>
              <span>
                模型
                <strong>{{ modelDisplayName(selectedDetail) }}</strong>
                <small v-if="channelDisplayText(selectedDetail)">{{ channelDisplayText(selectedDetail) }}</small>
                <small v-if="modelRuntimeText(selectedDetail)">{{ modelRuntimeText(selectedDetail) }}</small>
              </span>
              <span>耗时<strong>{{ formatLatency(selectedDetail.latency_ms) }}</strong></span>
              <span>点数<strong>{{ selectedDetail.credits_cost ?? 0 }}</strong></span>
              <span>时间<strong>{{ formatDate(selectedDetail.created_at) }}</strong></span>
            </div>

            <section class="generation-detail-section">
              <h3>原始提示词</h3>
              <p>{{ selectedDetail.prompt || '-' }}</p>
            </section>

            <section class="generation-detail-section">
              <h3>参数</h3>
              <div class="generation-param-grid">
                <span>比例<strong>{{ selectedDetail.params?.aspect_ratio || '-' }}</strong></span>
                <span>质量<strong>{{ selectedDetail.params?.quality || '-' }}</strong></span>
                <span>模式<strong>{{ selectedDetail.params?.tool_mode || '-' }}</strong></span>
                <span>风格<strong>{{ selectedDetail.params?.style_preset || '-' }}</strong></span>
                <span>风格强度<strong>{{ selectedDetail.params?.style_strength ?? '-' }}</strong></span>
                <span>参考权重<strong>{{ selectedDetail.params?.reference_weight ?? '-' }}</strong></span>
                <span class="generation-param-wide">Seed<strong>{{ selectedDetail.params?.seed || '-' }}</strong></span>
                <span class="generation-param-wide">负向提示词<strong>{{ selectedDetail.params?.negative_prompt || '-' }}</strong></span>
              </div>
            </section>

            <section class="generation-detail-section">
              <h3>结果文件</h3>
              <div v-if="selectedDetail.result_images?.length" class="generation-result-grid">
                <button
                  v-for="(image, index) in selectedDetail.result_images"
                  :key="image.preview_url || image.download_url"
                  class="generation-media-thumb-button"
                  type="button"
                  :data-testid="`generation-result-media-${index}`"
                  :aria-label="`预览结果${index + 1}`"
                  @click="openMediaPreview(image, mediaLabel(image))"
                >
                  <video v-if="isVideoResult(image)" :src="mediaSource(image)" playsinline data-skip-global-image-preview />
                  <img v-else :src="mediaSource(image)" alt="" data-skip-global-image-preview />
                  <span>{{ mediaLabel(image) }}</span>
                </button>
              </div>
              <p v-else class="generation-muted">当前任务没有可下载结果</p>
            </section>

            <section v-if="selectedDetail.reference_images?.length" class="generation-detail-section">
              <h3>参考图片</h3>
              <div class="generation-reference-grid">
                <button
                  v-for="(image, index) in selectedDetail.reference_images"
                  :key="image.reference_asset_id || image.preview_url"
                  class="generation-media-thumb-button generation-reference-thumb-button"
                  type="button"
                  :data-testid="`generation-reference-media-${index}`"
                  :aria-label="`预览${referenceMediaLabel(image, index)}`"
                  @click="openMediaPreview(image, referenceMediaLabel(image, index))"
                >
                  <img :src="mediaSource(image)" alt="" data-skip-global-image-preview />
                  <span>{{ referenceMediaLabel(image, index) }}</span>
                </button>
              </div>
            </section>

            <section v-if="selectedDetail.error" class="generation-detail-section generation-error-box">
              <h3>错误信息</h3>
              <p>{{ selectedDetail.error.code || '-' }}</p>
              <p>{{ selectedDetail.error.message || '-' }}</p>
            </section>

            <section v-if="hasProviderDiagnostics(selectedDetail)" class="generation-detail-section generation-diagnostics-box">
              <h3>供应商诊断</h3>
              <div class="generation-param-grid">
                <span>HTTP状态<strong>{{ formatHTTPStatus(providerDiagnostics(selectedDetail).provider_http_status) }}</strong></span>
                <span>错误码<strong>{{ providerDiagnostics(selectedDetail).provider_error_code || '-' }}</strong></span>
                <span>失败阶段<strong>{{ providerDiagnostics(selectedDetail).provider_failure_stage || '-' }}</strong></span>
                <span>尝试次数<strong>{{ providerDiagnostics(selectedDetail).provider_attempt_count || '-' }}</strong></span>
                <span class="generation-param-wide">原始错误<strong>{{ providerDiagnostics(selectedDetail).provider_error_message || '-' }}</strong></span>
              </div>
            </section>

            <section v-if="selectedDetail.events?.length" class="generation-detail-section generation-events-box">
              <h3>调用流程日志</h3>
              <ol class="generation-event-list">
                <li v-for="event in selectedDetail.events" :key="event.id || `${event.trace_id}-${event.event}-${event.created_at}`">
                  <div class="generation-event-head">
                    <span class="generation-event-level" :class="`is-${event.level || 'info'}`">{{ event.level || 'info' }}</span>
                    <strong>{{ event.event || '-' }}</strong>
                    <small>{{ formatDate(event.created_at) }}</small>
                  </div>
                  <p>{{ event.message || '-' }}</p>
                  <small>{{ event.stage || '-' }} · {{ event.trace_id || '-' }}</small>
                  <code v-if="formatEventMetadata(event.metadata)">{{ formatEventMetadata(event.metadata) }}</code>
                </li>
              </ol>
            </section>
          </div>
          <div v-else class="generation-empty-state">暂无详情</div>
        </div>
      </section>
    </div>

    <p v-if="errorMessage" class="status-error">{{ errorMessage }}</p>
  </section>

  <Teleport to="body">
    <div
      v-if="mediaPreview"
      class="generation-media-preview-backdrop"
      data-testid="generation-media-preview-modal"
      role="dialog"
      aria-modal="true"
      aria-labelledby="generation-media-preview-title"
      @click.self="closeMediaPreview"
    >
      <section class="generation-media-preview-panel">
        <header class="generation-media-preview-head">
          <h2 id="generation-media-preview-title">{{ mediaPreview.title }}</h2>
          <div class="generation-media-preview-actions">
            <a
              v-if="mediaPreview.downloadURL"
              class="mini-button icon-button-text generation-media-preview-download"
              data-testid="generation-media-preview-download"
              :href="mediaPreview.downloadURL"
              target="_blank"
              rel="noreferrer"
            >
              <Download :size="15" />
              <span>下载当前文件</span>
            </a>
            <button
              class="generation-detail-close"
              type="button"
              data-testid="generation-media-preview-close"
              title="关闭"
              aria-label="关闭媒体预览"
              @click="closeMediaPreview"
            >
              <X :size="18" />
            </button>
          </div>
        </header>
        <div class="generation-media-preview-stage">
          <video v-if="mediaPreview.isVideo" :src="mediaPreview.src" controls playsinline />
          <img v-else :src="mediaPreview.src" :alt="mediaPreview.title" />
        </div>
      </section>
    </div>
  </Teleport>
</template>

<style scoped>
.admin-generations-page {
  display: grid;
  gap: 16px;
}

.generation-page-heading,
.generation-detail-head,
.generation-detail-actions,
.generation-filter-actions,
.generation-kpi-label,
.generation-status-pill,
.generation-preview-strip,
.generation-user-cell {
  display: flex;
  align-items: center;
}

.generation-page-heading {
  justify-content: space-between;
  gap: 14px;
}

.generation-page-heading h1,
.generation-detail-head h2 {
  margin: 0;
  color: #101828;
  line-height: 1.05;
}

.generation-page-heading h1 {
  font-size: clamp(1.6rem, 2vw, 2.25rem);
}

.admin-breadcrumb {
  margin: 0 0 6px;
  color: #667085;
  font-size: 0.84rem;
  font-weight: 800;
}

.generation-export-button,
.generation-filter-actions .mini-button,
.generation-filter-actions .primary-button {
  gap: 8px;
  white-space: nowrap;
}

.generation-kpi-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(150px, 1fr));
  gap: 12px;
}

.generation-kpi-card,
.generations-filter-panel,
.generations-list-panel {
  border: 1px solid rgba(120, 132, 166, 0.16);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.8);
  box-shadow: 0 16px 38px rgba(82, 92, 126, 0.08);
}

.generation-kpi-card {
  display: grid;
  gap: 10px;
  min-height: 108px;
  padding: 16px;
}

.generation-kpi-label {
  justify-content: space-between;
  gap: 10px;
  color: #667085;
  font-size: 0.84rem;
  font-weight: 850;
}

.generation-kpi-card strong {
  color: #111827;
  font-size: 1.85rem;
  line-height: 1;
}

.generation-kpi-card small {
  color: #047857;
  font-weight: 800;
}

.generation-kpi-card small.negative {
  color: #b42318;
}

.generations-filter-panel {
  display: grid;
  grid-template-columns: minmax(240px, 1.25fr) repeat(5, minmax(120px, 0.7fr)) auto;
  gap: 10px;
  align-items: end;
  padding: 14px;
}

.generation-field {
  display: grid;
  gap: 7px;
  min-width: 0;
}

.generation-field span {
  color: #667085;
  font-size: 0.78rem;
  font-weight: 850;
}

.generation-field input,
.generation-field select,
.generation-input-shell {
  width: 100%;
  min-width: 0;
  min-height: 40px;
  border: 1px solid rgba(120, 132, 166, 0.2);
  border-radius: 8px;
  background: rgba(247, 249, 253, 0.92);
  color: #1d2435;
}

.generation-field input,
.generation-field select {
  padding: 0 11px;
  outline: none;
}

.generation-input-shell {
  gap: 8px;
  padding: 0 11px;
  color: #667085;
}

.generation-input-shell input {
  min-height: 38px;
  border: 0;
  padding: 0;
  background: transparent;
}

.generation-filter-actions {
  gap: 8px;
}

.generations-workspace {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 16px;
  align-items: start;
}

.generations-list-panel {
  min-width: 0;
  padding: 14px;
}

.generations-table-scroll {
  overflow-x: auto;
}

.generation-data-table {
  width: 100%;
  min-width: 1040px;
  border-collapse: collapse;
}

.generation-data-table th,
.generation-data-table td {
  padding: 12px 10px;
  border-bottom: 1px solid rgba(118, 129, 166, 0.12);
  text-align: left;
  vertical-align: middle;
}

.generation-data-table th {
  background: rgba(247, 249, 253, 0.86);
  color: #7a8497;
  font-size: 0.76rem;
  font-weight: 900;
}

.generation-data-table tbody tr:hover,
.generation-data-table tbody tr.selected {
  background: rgba(239, 246, 255, 0.72);
}

.generation-user-cell {
  gap: 9px;
  min-width: 148px;
}

.generation-user-cell > span {
  justify-content: center;
  flex: 0 0 auto;
  width: 34px;
  height: 34px;
  border-radius: 50%;
  background: linear-gradient(135deg, #2563eb 0%, #14b8a6 100%);
  color: #fff;
  font-weight: 900;
}

.generation-user-cell div {
  min-width: 0;
}

.generation-user-cell strong,
.generation-user-cell small {
  display: block;
  max-width: 150px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.generation-user-cell small,
.generation-muted,
.generation-no-image {
  color: #8a94a6;
  font-size: 0.78rem;
  font-weight: 750;
}

.generation-preview-strip {
  gap: 6px;
  min-width: 74px;
}

.generation-preview-thumb {
  width: 46px;
  height: 46px;
  border-radius: 8px;
  object-fit: cover;
  background: #eef2f7;
}

.generation-prompt-summary {
  display: -webkit-box;
  min-width: 230px;
  max-width: 310px;
  margin: 0;
  overflow: hidden;
  color: #263246;
  font-size: 0.88rem;
  font-weight: 760;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

.generation-model {
  display: grid;
  gap: 2px;
  max-width: 156px;
}

.generation-model strong,
.generation-model small {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.generation-model strong {
  color: #344054;
  font-size: 0.86rem;
}

.generation-model small,
.generation-detail-meta small {
  color: #8a94a6;
  font-size: 0.74rem;
  font-weight: 760;
}

.generation-status-pill {
  gap: 6px;
  min-height: 28px;
  padding: 0 9px;
  border-radius: 999px;
  background: rgba(107, 114, 128, 0.1);
  color: #4b5563;
  font-size: 0.78rem;
  font-weight: 900;
  white-space: nowrap;
}

.generation-status-pill.is-success {
  background: rgba(16, 185, 129, 0.12);
  color: #047857;
}

.generation-status-pill.is-failed {
  background: rgba(244, 63, 94, 0.12);
  color: #be123c;
}

.generation-status-pill.is-running {
  background: rgba(245, 158, 11, 0.16);
  color: #92400e;
}

.generation-empty-state {
  padding: 24px 12px;
  color: #7a8497;
  font-weight: 800;
  text-align: center;
}

.generation-detail-modal-backdrop {
  position: fixed;
  inset: 0;
  z-index: 1000;
  display: grid;
  place-items: center;
  padding: 16px;
  background: rgba(17, 24, 39, 0.38);
}

.generation-detail-modal {
  display: grid;
  grid-template-rows: auto minmax(0, 1fr);
  gap: 14px;
  width: min(940px, calc(100vw - 32px));
  max-height: calc(100vh - 32px);
  min-width: 0;
  padding: 18px;
  border: 1px solid rgba(118, 129, 166, 0.2);
  border-radius: 8px;
  background: #fff;
  box-shadow: 0 24px 70px rgba(15, 23, 42, 0.24);
}

.generation-detail-head {
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.generation-detail-head h2 {
  margin-top: 4px;
  font-size: 1.18rem;
  overflow-wrap: anywhere;
}

.generation-detail-actions {
  flex-shrink: 0;
  gap: 8px;
}

.generation-detail-close {
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

.generation-detail-close:hover {
  border-color: rgba(33, 85, 214, 0.34);
  color: #1d4ed8;
}

.generation-detail-content {
  min-height: 0;
  overflow: auto;
  padding-right: 4px;
}

.generation-detail-body,
.generation-detail-section {
  display: grid;
  gap: 12px;
}

.generation-detail-meta,
.generation-param-grid {
  display: grid;
  gap: 8px;
}

.generation-detail-meta {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.generation-detail-meta span,
.generation-param-grid span {
  display: grid;
  gap: 4px;
  min-width: 0;
  padding: 9px;
  border-radius: 8px;
  background: rgba(247, 249, 253, 0.86);
  color: #7a8497;
  font-size: 0.74rem;
  font-weight: 850;
}

.generation-detail-meta strong,
.generation-param-grid strong {
  min-width: 0;
  overflow: hidden;
  color: #1d2939;
  font-size: 0.86rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.generation-detail-section h3 {
  margin: 0;
  color: #101828;
  font-size: 0.92rem;
}

.generation-detail-section p {
  margin: 0;
  color: #344054;
  font-size: 0.9rem;
  font-weight: 700;
  overflow-wrap: anywhere;
}

.generation-param-grid {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.generation-param-wide {
  grid-column: 1 / -1;
}

.generation-result-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(138px, 1fr));
  gap: 8px;
}

.generation-reference-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(138px, 1fr));
  gap: 8px;
}

.generation-result-grid img,
.generation-result-grid video,
.generation-reference-grid img {
  width: 100%;
  aspect-ratio: 1;
  border-radius: 8px;
  object-fit: cover;
  background: #eef2f7;
}

.generation-media-thumb-button {
  position: relative;
  min-width: 0;
  padding: 0;
  border: 0;
  background: transparent;
  color: inherit;
  cursor: pointer;
  text-align: left;
}

.generation-media-thumb-button:hover img,
.generation-media-thumb-button:hover video {
  filter: brightness(0.96);
}

.generation-media-thumb-button:focus-visible {
  outline: 2px solid rgba(37, 99, 235, 0.56);
  outline-offset: 3px;
}

.generation-result-grid .generation-media-thumb-button span {
  position: absolute;
  right: 8px;
  bottom: 8px;
  padding: 3px 7px;
  border-radius: 999px;
  background: rgba(10, 18, 34, 0.72);
  color: #fff;
  font-size: 0.7rem;
  font-weight: 850;
}

.generation-reference-thumb-button {
  display: grid;
  gap: 6px;
  color: #344054;
}

.generation-reference-thumb-button span {
  overflow: hidden;
  font-size: 0.74rem;
  font-weight: 850;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.generation-media-preview-backdrop {
  position: fixed;
  inset: 0;
  z-index: 1200;
  display: grid;
  place-items: center;
  padding: 24px;
  background: rgba(7, 12, 24, 0.76);
}

.generation-media-preview-panel {
  display: grid;
  grid-template-rows: auto minmax(0, 1fr);
  gap: 14px;
  width: min(1120px, 96vw);
  max-height: 94vh;
  padding: 14px;
  border: 1px solid rgba(255, 255, 255, 0.18);
  border-radius: 8px;
  background: #fff;
  box-shadow: 0 28px 80px rgba(0, 0, 0, 0.34);
}

.generation-media-preview-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.generation-media-preview-head h2 {
  min-width: 0;
  margin: 0;
  color: #101828;
  font-size: 1rem;
  line-height: 1.3;
  overflow-wrap: anywhere;
}

.generation-media-preview-actions {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  gap: 8px;
}

.generation-media-preview-download {
  color: #1d4ed8;
  text-decoration: none;
}

.generation-media-preview-stage {
  display: grid;
  min-height: 0;
  place-items: center;
  overflow: hidden;
  border-radius: 8px;
  background: #0b1020;
}

.generation-media-preview-stage img,
.generation-media-preview-stage video {
  display: block;
  max-width: 100%;
  max-height: calc(94vh - 104px);
  object-fit: contain;
}

.generation-error-box {
  padding: 12px;
  border-radius: 8px;
  background: rgba(244, 63, 94, 0.08);
}

.generation-diagnostics-box {
  padding: 12px;
  border-radius: 8px;
  background: rgba(245, 158, 11, 0.1);
}

.generation-diagnostics-box .generation-param-wide strong {
  white-space: normal;
}

.generation-events-box {
  padding: 12px;
  border-radius: 8px;
  background: rgba(15, 118, 110, 0.08);
}

.generation-event-list {
  display: grid;
  gap: 10px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.generation-event-list li {
  display: grid;
  gap: 7px;
  min-width: 0;
  padding: 10px;
  border: 1px solid rgba(120, 132, 166, 0.16);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.72);
}

.generation-event-head {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.generation-event-head strong {
  min-width: 0;
  overflow: hidden;
  color: #172033;
  font-size: 0.82rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.generation-event-head small,
.generation-event-list li > small {
  color: #7a8497;
  font-size: 0.72rem;
  font-weight: 800;
}

.generation-event-level {
  flex: 0 0 auto;
  padding: 2px 6px;
  border-radius: 999px;
  background: rgba(37, 99, 235, 0.12);
  color: #1d4ed8;
  font-size: 0.68rem;
  font-weight: 900;
}

.generation-event-level.is-error {
  background: rgba(244, 63, 94, 0.12);
  color: #be123c;
}

.generation-event-list code {
  display: block;
  overflow-wrap: anywhere;
  white-space: normal;
}

@media (max-width: 1280px) {
  .generations-filter-panel {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }

  .generation-field-wide {
    grid-column: span 2;
  }
}

@media (max-width: 980px) {
  .generation-kpi-grid,
  .generations-workspace {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 720px) {
  .generation-page-heading {
    align-items: stretch;
    flex-direction: column;
  }

  .generation-kpi-grid,
  .generations-filter-panel,
  .generation-detail-meta,
  .generation-param-grid {
    grid-template-columns: 1fr;
  }

  .generation-field-wide,
  .generation-param-wide {
    grid-column: auto;
  }

  .generation-filter-actions {
    justify-content: stretch;
  }

  .generation-filter-actions > button,
  .generation-export-button {
    flex: 1 1 0;
    justify-content: center;
  }

  .generation-detail-modal-backdrop {
    align-items: stretch;
    padding: 10px;
  }

  .generation-detail-modal {
    width: 100%;
    max-height: calc(100vh - 20px);
    padding: 14px;
  }

  .generation-detail-head {
    flex-direction: column;
  }

  .generation-detail-actions {
    width: 100%;
    justify-content: space-between;
  }

  .generation-media-preview-backdrop {
    padding: 10px;
  }

  .generation-media-preview-panel {
    width: 100%;
    max-height: calc(100vh - 20px);
    padding: 10px;
  }

  .generation-media-preview-head {
    flex-direction: column;
  }

  .generation-media-preview-actions {
    width: 100%;
    justify-content: space-between;
  }

  .generation-media-preview-stage img,
  .generation-media-preview-stage video {
    max-height: calc(100vh - 138px);
  }
}
</style>
