<script setup>
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import {
  Activity,
  CheckCircle2,
  ClipboardList,
  Database,
  Eye,
  KeyRound,
  Network,
  Pencil,
  Plus,
  RefreshCw,
  Route,
  Save,
  Search,
  Server,
  SlidersHorizontal,
  Trash2,
  X
} from 'lucide-vue-next'

import { api } from '../api/client.js'
import ClickSelect from '../components/ClickSelect.vue'

const tabs = [
  { key: 'models', label: '模型目录', icon: Database },
  { key: 'channels', label: '渠道线路', icon: Network },
  { key: 'routing', label: '路由策略', icon: Route },
  { key: 'monitoring', label: '调用监控', icon: Activity },
  { key: 'audit', label: '变更审计', icon: ClipboardList }
]
const modalityOptions = [
  { value: 'image', label: '图片' },
  { value: 'video', label: '视频' },
  { value: 'chat', label: '文本与视觉理解' }
]
const onlineStatusOptions = [
  { value: 'online', label: '在线' },
  { value: 'offline', label: '离线' }
]
const visibilityOptions = [
  { value: 'public', label: '公开' },
  { value: 'internal', label: '内部' }
]
const healthStatusOptions = [
  { value: 'healthy', label: '健康' },
  { value: 'degraded', label: '降级' },
  { value: 'down', label: '不可用' }
]
const routingStrategyOptions = [
  { value: 'default', label: '默认渠道' },
  { value: 'weighted', label: '权重' },
  { value: 'speed_first', label: '速度优先' },
  { value: 'round_robin', label: '轮询' }
]
const callAttemptStatusOptions = [
  { value: '', label: '全部' },
  { value: 'succeeded', label: '成功' },
  { value: 'failed', label: '失败' }
]
const commonVideoDurations = ['1', '3', '4', '5', '6', '8', '10', '12', '15', '25', '-1']

const activeTab = ref('models')
const loading = ref(false)
const saving = ref(false)
const message = ref('')
const errorMessage = ref('')
const overview = reactive({
  summary: { models: 0, providers: 0, channels: 0 },
  models: [],
  providers: [],
  channels: [],
  routing: [],
  monitoring: []
})
const auditLogs = ref([])
const filters = reactive({ model: '', channel: '' })
const dialog = reactive({ type: '', mode: 'create', id: null })
const modelForm = reactive({
  name: '',
  modality: 'image',
  status: 'online',
  visibility: 'public',
  default_credits_cost: 1,
  capability_tags_text: '',
  video_durations: [],
  default_video_duration: '',
  custom_video_duration: '',
  sort_order: 0
})
const providerForm = reactive({
  name: '',
  provider: '',
  base_url: '',
  api_key: '',
  api_key_set: false,
  clear_api_key: false,
  default_timeout_seconds: 600,
  concurrency_limit: 0,
  status: 'online'
})
const channelForm = reactive({
  model_id: '',
  provider_id: '',
  name: '',
  runtime_model: '',
  video_durations: [],
  custom_video_duration: '',
  endpoint: '',
  weight: 100,
  priority: 1,
  status: 'online',
  health_status: 'healthy'
})
const routingDrafts = ref([])
const channelCallAttemptFilters = reactive({
  status: '',
  date_from: '',
  date_to: ''
})
const channelCallAttemptModal = reactive({
  open: false,
  loading: false,
  error: '',
  channel: null,
  model_id: '',
  items: [],
  page: 1,
  page_size: 20,
  total: 0,
  requestToken: 0
})

const modelById = computed(() => Object.fromEntries(overview.models.map((model) => [model.id, model])))
const providerById = computed(() => Object.fromEntries(overview.providers.map((provider) => [provider.id, provider])))
const selectedChannelModel = computed(() => modelById.value[Number(channelForm.model_id)] || null)
const channelDurationMissing = computed(() => {
  if (selectedChannelModel.value?.modality !== 'video') return []
  return (selectedChannelModel.value.video_durations || []).filter((value) => !channelForm.video_durations.includes(String(value)))
})
const imageModels = computed(() => overview.models.filter((model) => model.modality === 'image'))
const videoModels = computed(() => overview.models.filter((model) => model.modality === 'video'))
const chatModels = computed(() => overview.models.filter((model) => model.modality === 'chat'))
const filteredModels = computed(() => {
  const q = filters.model.trim().toLowerCase()
  if (!q) return overview.models
  return overview.models.filter((model) => `${model.name} ${model.modality} ${model.status}`.toLowerCase().includes(q))
})
const filteredChannels = computed(() => {
  const q = filters.channel.trim().toLowerCase()
  if (!q) return overview.channels
  return overview.channels.filter((channel) => `${channel.name} ${channel.runtime_model} ${channel.provider_name} ${channel.model_name}`.toLowerCase().includes(q))
})
const modelMonitoring = computed(() => {
  const byModel = new Map()
  overview.monitoring.forEach((item) => {
    const current = byModel.get(item.model_id) ?? { model_id: item.model_id, total_calls: 0, succeeded_calls: 0, failed_calls: 0, average_latency_ms: 0 }
    current.total_calls += Number(item.total_calls || 0)
    current.succeeded_calls += Number(item.succeeded_calls || 0)
    current.failed_calls += Number(item.failed_calls || 0)
    current.average_latency_ms = Math.max(current.average_latency_ms, Number(item.average_latency_ms || 0))
    byModel.set(item.model_id, current)
  })
  return [...byModel.values()]
})
const channelCallAttemptRange = computed(() => {
  const total = Number(channelCallAttemptModal.total || 0)
  if (!total) return '0 / 0'
  const page = Number(channelCallAttemptModal.page || 1)
  const pageSize = Number(channelCallAttemptModal.page_size || 20)
  const start = (page - 1) * pageSize + 1
  const end = Math.min(page * pageSize, total)
  return `${start}-${end} / ${total}`
})
const channelCallAttemptCanPrev = computed(() => Number(channelCallAttemptModal.page || 1) > 1)
const channelCallAttemptCanNext = computed(() => Number(channelCallAttemptModal.page || 1) * Number(channelCallAttemptModal.page_size || 20) < Number(channelCallAttemptModal.total || 0))

function resetMessages() {
  message.value = ''
  errorMessage.value = ''
}

async function loadOverview(options = {}) {
  const shouldResetMessages = options.resetMessages !== false
  loading.value = true
  if (shouldResetMessages) resetMessages()
  try {
    const data = await api.getModelCenterOverview()
    overview.summary = data.summary ?? { models: 0, providers: 0, channels: 0 }
    overview.models = data.models ?? []
    overview.providers = data.providers ?? []
    overview.channels = data.channels ?? []
    overview.routing = data.routing ?? []
    overview.monitoring = data.monitoring ?? []
    routingDrafts.value = buildRoutingDrafts(overview.routing)
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    loading.value = false
  }
}

async function loadAuditLogs() {
  resetMessages()
  try {
    const data = await api.listModelCenterAuditLogs()
    auditLogs.value = data.items ?? []
  } catch (error) {
    errorMessage.value = error.message
  }
}

async function refreshAll() {
  await loadOverview()
  if (activeTab.value === 'audit') {
    await loadAuditLogs()
  }
}

function switchTab(tab) {
  activeTab.value = tab
  if (tab === 'audit' && auditLogs.value.length === 0) {
    loadAuditLogs()
  }
}

function monitoringChannel(item) {
  return overview.channels.find((channel) => channel.id === item.channel_id) ?? null
}

function monitoringChannelName(item) {
  return monitoringChannel(item)?.name || `#${item.channel_id}`
}

function monitoringModelName(item) {
  return modelById.value[item.model_id]?.name || `#${item.model_id}`
}

function resetChannelCallAttemptState() {
  Object.assign(channelCallAttemptFilters, {
    status: '',
    date_from: '',
    date_to: ''
  })
  Object.assign(channelCallAttemptModal, {
    loading: false,
    error: '',
    channel: null,
    model_id: '',
    items: [],
    page: 1,
    page_size: 20,
    total: 0
  })
}

function openChannelCallAttempts(item) {
  const channel = monitoringChannel(item) ?? {
    id: item.channel_id,
    model_id: item.model_id,
    model_name: monitoringModelName(item),
    name: `#${item.channel_id}`
  }
  resetChannelCallAttemptState()
  channelCallAttemptModal.open = true
  channelCallAttemptModal.channel = channel
  channelCallAttemptModal.model_id = item.model_id || channel.model_id || ''
  loadChannelCallAttempts(1)
}

function closeChannelCallAttemptModal() {
  channelCallAttemptModal.requestToken += 1
  channelCallAttemptModal.open = false
  resetChannelCallAttemptState()
}

async function loadChannelCallAttempts(page = channelCallAttemptModal.page) {
  const channel = channelCallAttemptModal.channel
  if (!channel?.id) return
  const token = channelCallAttemptModal.requestToken + 1
  channelCallAttemptModal.requestToken = token
  channelCallAttemptModal.loading = true
  channelCallAttemptModal.error = ''
  try {
    const data = await api.listModelCenterChannelCallAttempts(channel.id, {
      model_id: channelCallAttemptModal.model_id,
      page,
      page_size: channelCallAttemptModal.page_size,
      status: channelCallAttemptFilters.status,
      date_from: channelCallAttemptFilters.date_from,
      date_to: channelCallAttemptFilters.date_to
    })
    if (token !== channelCallAttemptModal.requestToken || !channelCallAttemptModal.open) return
    channelCallAttemptModal.channel = data.channel ?? channel
    channelCallAttemptModal.model_id = data.model_id || channelCallAttemptModal.model_id
    channelCallAttemptModal.items = data.items ?? []
    channelCallAttemptModal.page = Number(data.page || page)
    channelCallAttemptModal.page_size = Number(data.page_size || channelCallAttemptModal.page_size)
    channelCallAttemptModal.total = Number(data.total || 0)
  } catch (error) {
    if (token !== channelCallAttemptModal.requestToken || !channelCallAttemptModal.open) return
    channelCallAttemptModal.items = []
    channelCallAttemptModal.total = 0
    channelCallAttemptModal.error = error.message
  } finally {
    if (token === channelCallAttemptModal.requestToken) {
      channelCallAttemptModal.loading = false
    }
  }
}

function queryChannelCallAttempts() {
  channelCallAttemptModal.page = 1
  loadChannelCallAttempts(1)
}

function resetChannelCallAttemptFilters() {
  Object.assign(channelCallAttemptFilters, {
    status: '',
    date_from: '',
    date_to: ''
  })
  channelCallAttemptModal.page = 1
  loadChannelCallAttempts(1)
}

function previousChannelCallAttempts() {
  if (!channelCallAttemptCanPrev.value) return
  loadChannelCallAttempts(Number(channelCallAttemptModal.page || 1) - 1)
}

function nextChannelCallAttempts() {
  if (!channelCallAttemptCanNext.value) return
  loadChannelCallAttempts(Number(channelCallAttemptModal.page || 1) + 1)
}

function handleModelCenterKeydown(event) {
  if (event.key === 'Escape' && channelCallAttemptModal.open) {
    closeChannelCallAttemptModal()
  }
}

function buildRoutingDrafts(routes = []) {
  const byModality = Object.fromEntries(routes.map((route) => [route.modality, route]))
  const modalities = ['image', 'video']
  if (chatModels.value.length > 0) modalities.push('chat')
  return modalities.map((modality) => {
    const route = byModality[modality] ?? {}
    const channels = overview.channels.filter((channel) => modelById.value[channel.model_id]?.modality === modality)
    const entryByChannel = Object.fromEntries((route.entries ?? []).map((entry) => [entry.channel_id, entry]))
    return {
      modality,
      default_model_id: route.default_model_id || channels[0]?.model_id || '',
      fallback_model_id: route.fallback_model_id || route.default_model_id || channels[0]?.model_id || '',
      routing_enabled: route.routing_enabled ?? true,
      routing_strategy: route.routing_strategy || 'default',
      entries: channels.map((channel) => ({
        model_id: channel.model_id,
        channel_id: channel.id,
        enabled: entryByChannel[channel.id]?.enabled ?? channel.status === 'online',
        weight: Number(entryByChannel[channel.id]?.weight ?? channel.weight ?? 0),
        priority: Number(entryByChannel[channel.id]?.priority ?? channel.priority ?? 1),
        channel_name: channel.name,
        model_name: channel.model_name,
        runtime_model: channel.runtime_model
      }))
    }
  })
}

function openModelDialog(model = null) {
  dialog.type = 'model'
  dialog.mode = model ? 'edit' : 'create'
  dialog.id = model?.id ?? null
  Object.assign(modelForm, {
    name: model?.name ?? '',
    modality: model?.modality ?? 'image',
    status: model?.status ?? 'online',
    visibility: model?.visibility ?? 'public',
    default_credits_cost: model?.default_credits_cost ?? 1,
    capability_tags_text: (model?.capability_tags ?? []).join(', '),
    video_durations: (model?.video_durations ?? []).map(String),
    default_video_duration: model?.default_video_duration ? String(model.default_video_duration) : '',
    custom_video_duration: '',
    sort_order: model?.sort_order ?? 0
  })
}

function openProviderDialog(provider = null) {
  dialog.type = 'provider'
  dialog.mode = provider ? 'edit' : 'create'
  dialog.id = provider?.id ?? null
  Object.assign(providerForm, {
    name: provider?.name ?? '',
    provider: provider?.provider ?? '',
    base_url: provider?.base_url ?? '',
    api_key: '',
    api_key_set: Boolean(provider?.api_key_set),
    clear_api_key: false,
    default_timeout_seconds: provider?.default_timeout_seconds ?? 600,
    concurrency_limit: provider?.concurrency_limit ?? 0,
    status: provider?.status ?? 'online'
  })
}

function openChannelDialog(channel = null) {
  dialog.type = 'channel'
  dialog.mode = channel ? 'edit' : 'create'
  dialog.id = channel?.id ?? null
  Object.assign(channelForm, {
    model_id: channel?.model_id ?? overview.models[0]?.id ?? '',
    provider_id: channel?.provider_id ?? overview.providers[0]?.id ?? '',
    name: channel?.name ?? '',
    runtime_model: channel?.runtime_model ?? '',
    video_durations: (channel?.video_durations ?? []).map(String),
    custom_video_duration: '',
    endpoint: channel?.endpoint ?? '',
    weight: channel?.weight ?? 100,
    priority: channel?.priority ?? 1,
    status: channel?.status ?? 'online',
    health_status: channel?.health_status ?? 'healthy'
  })
}

function closeDialog() {
  dialog.type = ''
  dialog.mode = 'create'
  dialog.id = null
}

function modelPayload() {
  return {
    name: modelForm.name.trim(),
    modality: modelForm.modality,
    status: modelForm.status,
    visibility: modelForm.visibility,
    default_credits_cost: Number(modelForm.default_credits_cost ?? 1),
    capability_tags: modelForm.capability_tags_text.split(',').map((tag) => tag.trim()).filter(Boolean),
    video_durations: modelForm.modality === 'video' ? normalizeDurationValues(modelForm.video_durations) : [],
    default_video_duration: modelForm.modality === 'video' ? modelForm.default_video_duration : '',
    sort_order: Number(modelForm.sort_order || 0)
  }
}

function providerPayload() {
  const payload = {
    name: providerForm.name.trim(),
    provider: providerForm.provider.trim(),
    base_url: providerForm.base_url.trim(),
    default_timeout_seconds: Number(providerForm.default_timeout_seconds || 0),
    concurrency_limit: Number(providerForm.concurrency_limit || 0),
    status: providerForm.status
  }
  if (providerForm.api_key.trim()) payload.api_key = providerForm.api_key.trim()
  if (providerForm.clear_api_key) payload.clear_api_key = true
  return payload
}

function channelPayload() {
  return {
    model_id: Number(channelForm.model_id || 0),
    provider_id: Number(channelForm.provider_id || 0),
    name: channelForm.name.trim(),
    runtime_model: channelForm.runtime_model.trim(),
    video_durations: selectedChannelModel.value?.modality === 'video' ? normalizeDurationValues(channelForm.video_durations) : [],
    endpoint: channelForm.endpoint.trim(),
    weight: Number(channelForm.weight || 0),
    priority: Number(channelForm.priority || 1),
    status: channelForm.status,
    health_status: channelForm.health_status
  }
}

async function submitDialog() {
  saving.value = true
  resetMessages()
  try {
    if (dialog.type === 'model') {
      if (dialog.mode === 'edit') await api.updateModelCenterModel(dialog.id, modelPayload())
      else await api.createModelCenterModel(modelPayload())
      message.value = '模型目录已保存'
    }
    if (dialog.type === 'provider') {
      if (dialog.mode === 'edit') await api.updateModelCenterProvider(dialog.id, providerPayload())
      else await api.createModelCenterProvider(providerPayload())
      message.value = '供应商账号已保存'
    }
    if (dialog.type === 'channel') {
      if (dialog.mode === 'edit') await api.updateModelCenterChannel(dialog.id, channelPayload())
      else await api.createModelCenterChannel(channelPayload())
      message.value = '渠道线路已保存'
    }
    closeDialog()
    await loadOverview({ resetMessages: false })
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    saving.value = false
  }
}

async function deleteModel(model) {
  if (window.confirm && !window.confirm(`删除业务模型「${model.name}」？`)) return
  await mutate(() => api.deleteModelCenterModel(model.id), '业务模型已删除')
}

async function deleteProvider(provider) {
  if (window.confirm && !window.confirm(`删除供应商「${provider.name}」？`)) return
  await mutate(() => api.deleteModelCenterProvider(provider.id), '供应商已删除')
}

async function deleteChannel(channel) {
  if (window.confirm && !window.confirm(`删除渠道「${channel.name}」？`)) return
  await mutate(() => api.deleteModelCenterChannel(channel.id), '渠道已删除')
}

async function mutate(action, successText) {
  saving.value = true
  resetMessages()
  try {
    await action()
    message.value = successText
    await loadOverview({ resetMessages: false })
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    saving.value = false
  }
}

async function saveRouting() {
  saving.value = true
  resetMessages()
  try {
    await api.updateModelCenterRouting({
      routes: routingDrafts.value.map((route) => ({
        modality: route.modality,
        default_model_id: Number(route.default_model_id || 0),
        fallback_model_id: Number(route.fallback_model_id || 0),
        routing_enabled: Boolean(route.routing_enabled),
        routing_strategy: route.routing_strategy,
        entries: route.entries.map((entry) => ({
          model_id: Number(entry.model_id || 0),
          channel_id: Number(entry.channel_id || 0),
          enabled: Boolean(entry.enabled),
          weight: Number(entry.weight || 0),
          priority: Number(entry.priority || 1)
        }))
      }))
    })
    message.value = '路由策略已保存'
    await loadOverview({ resetMessages: false })
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    saving.value = false
  }
}

function modelsForModality(modality) {
  if (modality === 'video') return videoModels.value
  if (modality === 'chat') return chatModels.value
  return imageModels.value
}

function normalizeDurationValues(values = []) {
  return [...new Set(values.map(String).map((value) => value.trim()).filter(Boolean))]
    .sort((left, right) => left === '-1' ? 1 : right === '-1' ? -1 : Number(left) - Number(right))
}

function toggleDuration(form, value) {
  const normalized = String(value)
  form.video_durations = form.video_durations.includes(normalized)
    ? form.video_durations.filter((item) => item !== normalized)
    : normalizeDurationValues([...form.video_durations, normalized])
  if (form === modelForm && !form.video_durations.includes(form.default_video_duration)) {
    form.default_video_duration = form.video_durations[0] || ''
  }
}

function addCustomDuration(form) {
  const value = String(form.custom_video_duration || '').trim()
  const seconds = Number(value)
  if (!Number.isInteger(seconds) || seconds < 1 || seconds > 60) {
    errorMessage.value = '视频时长必须是 1–60 的整数秒'
    return
  }
  if (!form.video_durations.includes(value)) toggleDuration(form, value)
  form.custom_video_duration = ''
}

function applyChannelDurationIntersection() {
  const channels = overview.channels.filter((item) => item.model_id === dialog.id && item.status === 'online' && (item.video_durations || []).length)
  if (!channels.length) return
  modelForm.video_durations = channels.slice(1).reduce(
    (values, channel) => values.filter((value) => channel.video_durations.map(String).includes(value)),
    channels[0].video_durations.map(String)
  )
  if (!modelForm.video_durations.includes(modelForm.default_video_duration)) modelForm.default_video_duration = modelForm.video_durations[0] || ''
}

function recommendedChannelDurations() {
  const runtime = channelForm.runtime_model.toLowerCase()
  let values = []
  if (runtime.includes('grok') || runtime.includes('imagine')) values = ['1', '3', '6', '10', '15']
  else if (runtime.includes('seedance-2') || runtime.includes('seedance_2')) values = ['4', '5', '6', '7', '8', '9', '10', '11', '12', '13', '14', '15', '-1']
  else if (runtime.includes('seedance')) values = ['4', '5', '6', '8', '10', '12', '15', '-1']
  else if (runtime.includes('sora')) values = ['10', '15', '25']
  if (values.length) channelForm.video_durations = values
}

function durationSummary(item) {
  const values = (item?.video_durations || []).map((value) => value === '-1' ? '自动' : value)
  return values.length ? `${values.join('/')}s` : '-'
}

function channelHasDurationConflict(channel) {
  if (modelById.value[channel.model_id]?.modality !== 'video' || channel.status !== 'online') return false
  if (channel.duration_compatible === false) return true
  const required = modelById.value[channel.model_id]?.video_durations || []
  return required.some((value) => !(channel.video_durations || []).map(String).includes(String(value)))
}

function modelHasDurationConflict(model) {
  return model.modality === 'video' && overview.channels.some((channel) => channel.model_id === model.id && channelHasDurationConflict(channel))
}

function modalityLabel(value) {
  if (value === 'video') return '视频'
  if (value === 'chat') return '文本与视觉理解'
  return '图片'
}

function statusLabel(value) {
  return value === 'online' ? '在线' : '离线'
}

function healthLabel(value) {
  if (value === 'down') return '不可用'
  if (value === 'degraded') return '降级'
  return '健康'
}

function callAttemptStatusLabel(value) {
  if (value === 'succeeded') return '成功'
  if (value === 'failed') return '失败'
  return value || '-'
}

function callAttemptStatusClass(value) {
  return {
    offline: value === 'failed'
  }
}

function formatNumber(value) {
  return Number(value || 0).toLocaleString()
}

function formatHTTPStatus(value) {
  const status = Number(value || 0)
  return status > 0 ? status : '-'
}

function formatLatency(value) {
  const latency = Number(value || 0)
  return latency > 0 ? `${latency}ms` : '-'
}

function successRate(item) {
  const total = Number(item?.total_calls || 0)
  if (!total) return '0%'
  return `${Math.round((Number(item.succeeded_calls || 0) / total) * 100)}%`
}

function formatTime(value) {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

onMounted(() => {
  refreshAll()
  window.addEventListener('keydown', handleModelCenterKeydown)
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleModelCenterKeydown)
})
</script>

<template>
  <section class="model-center-page">
    <div class="admin-page-heading model-center-heading">
      <div>
        <p class="eyebrow">模型配置中心</p>
        <h1>模型、供应商、渠道与路由</h1>
      </div>
      <button class="mini-button compact-button" type="button" data-testid="refresh-model-center" :disabled="loading" @click="refreshAll">
        <RefreshCw :size="15" />
        刷新
      </button>
    </div>

    <div class="model-center-summary">
      <div><span>业务模型</span><strong>{{ formatNumber(overview.summary.models) }}</strong></div>
      <div><span>供应商账号</span><strong>{{ formatNumber(overview.summary.providers) }}</strong></div>
      <div><span>调用渠道</span><strong>{{ formatNumber(overview.summary.channels) }}</strong></div>
    </div>

    <div v-if="message" class="settings-alert success"><CheckCircle2 :size="16" /><span>{{ message }}</span></div>
    <div v-if="errorMessage" class="settings-alert error"><X :size="16" /><span>{{ errorMessage }}</span></div>

    <nav class="model-center-tabs" aria-label="模型配置中心页签">
      <button v-for="tab in tabs" :key="tab.key" type="button" :data-testid="`tab-${tab.key}`" :class="{ active: activeTab === tab.key }" @click="switchTab(tab.key)">
        <component :is="tab.icon" :size="16" />
        <span>{{ tab.label }}</span>
      </button>
    </nav>

    <section v-if="activeTab === 'models'" class="admin-panel model-center-panel">
      <div class="panel-title-row">
        <div><p class="panel-kicker">Catalog</p><h2>业务模型</h2></div>
        <button class="primary-button compact-button" type="button" data-testid="open-model-create" @click="openModelDialog()"><Plus :size="16" />新增模型</button>
      </div>
      <div class="admin-filter-bar">
        <label class="admin-search-field"><Search :size="16" /><input v-model.trim="filters.model" data-testid="model-search" type="search" placeholder="搜索模型" /></label>
      </div>
      <div class="admin-table-scroll">
        <table class="admin-data-table">
          <thead><tr><th>名称</th><th>类型</th><th>状态</th><th>可见性</th><th>扣点</th><th>能力</th><th>时长</th><th>排序</th><th>操作</th></tr></thead>
          <tbody>
            <tr v-if="loading"><td colspan="9" class="empty-cell">加载中...</td></tr>
            <tr v-else-if="filteredModels.length === 0"><td colspan="9" class="empty-cell">暂无模型</td></tr>
            <tr v-for="model in filteredModels" v-else :key="model.id">
              <td><strong>{{ model.name }}</strong><small>#{{ model.id }}</small></td>
              <td>{{ modalityLabel(model.modality) }}</td>
              <td><span class="status-pill" :class="{ offline: model.status !== 'online' }">{{ statusLabel(model.status) }}</span></td>
              <td>{{ model.visibility === 'internal' ? '内部' : '公开' }}</td>
              <td>{{ model.default_credits_cost }} 点</td>
              <td>{{ (model.capability_tags || []).join(' / ') || '-' }}</td>
              <td :class="{ 'duration-warning': (model.modality === 'video' && !(model.video_durations || []).length) || modelHasDurationConflict(model) }">{{ durationSummary(model) }}<small v-if="modelHasDurationConflict(model)">渠道冲突</small></td>
              <td>{{ model.sort_order }}</td>
              <td class="row-actions">
                <button class="mini-button icon-only" type="button" :data-testid="`edit-model-${model.id}`" @click="openModelDialog(model)"><Pencil :size="15" /></button>
                <button class="mini-button icon-only destructive-button" type="button" :data-testid="`delete-model-${model.id}`" @click="deleteModel(model)"><Trash2 :size="15" /></button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <section v-if="activeTab === 'channels'" class="model-center-stack">
      <div class="admin-panel model-center-panel">
        <div class="panel-title-row">
          <div><p class="panel-kicker">Providers</p><h2>供应商账号</h2></div>
          <button class="primary-button compact-button" type="button" data-testid="open-provider-create" @click="openProviderDialog()"><KeyRound :size="16" />新增供应商</button>
        </div>
        <div class="admin-table-scroll">
          <table class="admin-data-table">
            <thead><tr><th>名称</th><th>Provider</th><th>Base URL</th><th>Key</th><th>超时</th><th>并发</th><th>状态</th><th>操作</th></tr></thead>
            <tbody>
              <tr v-for="provider in overview.providers" :key="provider.id">
                <td><strong>{{ provider.name }}</strong></td>
                <td>{{ provider.provider }}</td>
                <td>{{ provider.base_url || '-' }}</td>
                <td>{{ provider.api_key_set ? '已设置' : '未设置' }}</td>
                <td>{{ provider.default_timeout_seconds || '-' }}s</td>
                <td>{{ provider.concurrency_limit || '-' }}</td>
                <td><span class="status-pill" :class="{ offline: provider.status !== 'online' }">{{ statusLabel(provider.status) }}</span></td>
                <td class="row-actions">
                  <button class="mini-button icon-only" type="button" :data-testid="`edit-provider-${provider.id}`" @click="openProviderDialog(provider)"><Pencil :size="15" /></button>
                  <button class="mini-button icon-only destructive-button" type="button" :data-testid="`delete-provider-${provider.id}`" @click="deleteProvider(provider)"><Trash2 :size="15" /></button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div class="admin-panel model-center-panel">
        <div class="panel-title-row">
          <div><p class="panel-kicker">Channels</p><h2>渠道线路</h2></div>
          <button class="primary-button compact-button" type="button" data-testid="open-channel-create" @click="openChannelDialog()"><Server :size="16" />新增渠道</button>
        </div>
        <div class="admin-filter-bar">
          <label class="admin-search-field"><Search :size="16" /><input v-model.trim="filters.channel" type="search" placeholder="搜索渠道、runtime 或供应商" /></label>
        </div>
        <div class="admin-table-scroll">
          <table class="admin-data-table">
            <thead><tr><th>渠道</th><th>业务模型</th><th>供应商</th><th>Runtime</th><th>时长</th><th>Endpoint</th><th>权重</th><th>优先级</th><th>健康</th><th>操作</th></tr></thead>
            <tbody>
              <tr v-for="channel in filteredChannels" :key="channel.id">
                <td><strong>{{ channel.name }}</strong><small>#{{ channel.id }}</small></td>
                <td>{{ channel.model_name || modelById[channel.model_id]?.name || '-' }}</td>
                <td>{{ channel.provider_name || providerById[channel.provider_id]?.name || '-' }}</td>
                <td>{{ channel.runtime_model || '-' }}</td>
                <td :class="{ 'duration-warning': (modelById[channel.model_id]?.modality === 'video' && !(channel.video_durations || []).length) || channelHasDurationConflict(channel) }">{{ durationSummary(channel) }}<small v-if="channelHasDurationConflict(channel)">能力冲突</small></td>
                <td>{{ channel.endpoint || '-' }}</td>
                <td>{{ channel.weight }}</td>
                <td>{{ channel.priority }}</td>
                <td><span class="status-pill" :class="{ offline: channel.health_status === 'down', warn: channel.health_status === 'degraded' }">{{ healthLabel(channel.health_status) }}</span></td>
                <td class="row-actions">
                  <button class="mini-button icon-only" type="button" :data-testid="`edit-channel-${channel.id}`" @click="openChannelDialog(channel)"><Pencil :size="15" /></button>
                  <button class="mini-button icon-only destructive-button" type="button" :data-testid="`delete-channel-${channel.id}`" @click="deleteChannel(channel)"><Trash2 :size="15" /></button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </section>

    <section v-if="activeTab === 'routing'" class="admin-panel model-center-panel">
      <div class="panel-title-row">
        <div><p class="panel-kicker">Routing</p><h2>路由策略</h2></div>
        <button class="primary-button compact-button" type="button" data-testid="save-routing" :disabled="saving" @click="saveRouting"><Save :size="16" />保存路由</button>
      </div>
      <div class="routing-grid">
        <article v-for="route in routingDrafts" :key="route.modality" class="routing-block">
          <div class="routing-head">
            <strong>{{ modalityLabel(route.modality) }}默认策略</strong>
            <label><input v-model="route.routing_enabled" :data-testid="`routing-${route.modality}-enabled`" type="checkbox" />启用</label>
          </div>
          <div class="routing-fields">
            <label><span>默认模型</span><ClickSelect v-model="route.default_model_id" :options="modelsForModality(route.modality).map((model) => ({ value: model.id, label: model.name }))" :data-testid="`routing-${route.modality}-default-model`" class="select-input" aria-label="默认模型" /></label>
            <label><span>回退模型</span><ClickSelect v-model="route.fallback_model_id" :options="modelsForModality(route.modality).map((model) => ({ value: model.id, label: model.name }))" :data-testid="`routing-${route.modality}-fallback-model`" class="select-input" aria-label="回退模型" /></label>
            <label><span>策略</span><ClickSelect v-model="route.routing_strategy" :options="routingStrategyOptions" :data-testid="`routing-${route.modality}-strategy`" class="select-input" aria-label="路由策略" /></label>
          </div>
          <div class="routing-entry-list">
            <label v-for="entry in route.entries" :key="entry.channel_id" class="routing-entry">
              <input v-model="entry.enabled" :data-testid="`routing-entry-${entry.channel_id}-enabled`" type="checkbox" />
              <span><strong>{{ entry.channel_name }}</strong><small>{{ entry.model_name }} · {{ entry.runtime_model || '-' }}</small></span>
              <input v-model.number="entry.weight" :data-testid="`routing-entry-${entry.channel_id}-weight`" type="number" min="0" max="100" />
              <input v-model.number="entry.priority" :data-testid="`routing-entry-${entry.channel_id}-priority`" type="number" min="1" />
            </label>
          </div>
        </article>
      </div>
    </section>

    <section v-if="activeTab === 'monitoring'" class="model-center-stack">
      <div class="admin-panel model-center-panel">
        <div class="panel-title-row"><div><p class="panel-kicker">Models</p><h2>按业务模型汇总</h2></div></div>
        <div class="admin-table-scroll">
          <table class="admin-data-table">
            <thead><tr><th>业务模型</th><th>调用</th><th>成功率</th><th>失败</th><th>平均耗时</th></tr></thead>
            <tbody>
              <tr v-for="item in modelMonitoring" :key="item.model_id">
                <td>{{ modelById[item.model_id]?.name || `#${item.model_id}` }}</td>
                <td>{{ formatNumber(item.total_calls) }}</td>
                <td>{{ successRate(item) }}</td>
                <td>{{ formatNumber(item.failed_calls) }}</td>
                <td>{{ item.average_latency_ms }}ms</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
      <div class="admin-panel model-center-panel">
        <div class="panel-title-row"><div><p class="panel-kicker">Channels</p><h2>按渠道钻取</h2></div></div>
        <div class="admin-table-scroll">
          <table class="admin-data-table">
            <thead><tr><th>渠道</th><th>业务模型</th><th>调用</th><th>成功率</th><th>失败</th><th>平均耗时</th><th>操作</th></tr></thead>
            <tbody>
              <tr v-for="item in overview.monitoring" :key="`${item.model_id}-${item.channel_id}`">
                <td>{{ monitoringChannelName(item) }}</td>
                <td>{{ monitoringModelName(item) }}</td>
                <td>{{ formatNumber(item.total_calls) }}</td>
                <td>{{ successRate(item) }}</td>
                <td>{{ formatNumber(item.failed_calls) }}</td>
                <td>{{ item.average_latency_ms }}ms</td>
                <td class="row-actions">
                  <button class="mini-button compact-button" type="button" :data-testid="`monitoring-channel-calls-${item.channel_id}`" @click="openChannelCallAttempts(item)">
                    <Eye :size="15" />
                    查看
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </section>

    <section v-if="activeTab === 'audit'" class="admin-panel model-center-panel">
      <div class="panel-title-row">
        <div><p class="panel-kicker">Audit</p><h2>变更审计</h2></div>
        <button class="mini-button compact-button" type="button" @click="loadAuditLogs"><SlidersHorizontal :size="15" />刷新审计</button>
      </div>
      <div class="admin-table-scroll">
        <table class="admin-data-table">
          <thead><tr><th>时间</th><th>操作</th><th>对象</th><th>操作人</th><th>详情</th></tr></thead>
          <tbody>
            <tr v-for="log in auditLogs" :key="log.id">
              <td>{{ formatTime(log.created_at) }}</td>
              <td>{{ log.action }}</td>
              <td>{{ log.target_type }} #{{ log.target_id || '-' }}</td>
              <td>{{ log.admin_user_id || '-' }}</td>
              <td><code>{{ log.detail }}</code></td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <div v-if="dialog.type" class="model-center-dialog-backdrop">
      <form class="admin-panel model-center-dialog" data-testid="model-center-form" @submit.prevent="submitDialog">
        <div class="panel-title-row">
          <div><p class="panel-kicker">{{ dialog.mode === 'edit' ? 'Edit' : 'Create' }}</p><h2>{{ dialog.type === 'model' ? '业务模型' : dialog.type === 'provider' ? '供应商账号' : '渠道线路' }}</h2></div>
          <button class="mini-button icon-only" type="button" @click="closeDialog"><X :size="16" /></button>
        </div>

        <template v-if="dialog.type === 'model'">
          <label><span>名称</span><input v-model="modelForm.name" class="text-input" data-testid="model-name-input" required /></label>
          <div class="dialog-grid">
            <label><span>类型</span><ClickSelect v-model="modelForm.modality" :options="modalityOptions" class="select-input" data-testid="model-modality-input" aria-label="类型" /></label>
            <label><span>状态</span><ClickSelect v-model="modelForm.status" :options="onlineStatusOptions" class="select-input" data-testid="model-status-input" aria-label="状态" /></label>
            <label><span>可见性</span><ClickSelect v-model="modelForm.visibility" :options="visibilityOptions" class="select-input" data-testid="model-visibility-input" aria-label="可见性" /></label>
            <label><span>扣点</span><input v-model.number="modelForm.default_credits_cost" class="text-input" data-testid="model-credits-input" type="number" :min="modelForm.modality === 'chat' && modelForm.visibility === 'internal' ? 0 : 1" /></label>
          </div>
          <label><span>能力标签</span><input v-model="modelForm.capability_tags_text" class="text-input" data-testid="model-tags-input" placeholder="image, reference" /></label>
          <section v-if="modelForm.modality === 'video'" class="video-duration-editor" data-testid="model-video-duration-editor">
            <div class="duration-editor-title"><strong>视频时长能力</strong><button v-if="dialog.mode === 'edit'" class="mini-button" type="button" @click="applyChannelDurationIntersection">使用启用渠道交集</button></div>
            <div class="duration-quick-options">
              <button v-for="value in commonVideoDurations" :key="value" type="button" :class="{ active: modelForm.video_durations.includes(value) }" @click="toggleDuration(modelForm, value)">{{ value === '-1' ? '自动时长' : `${value} 秒` }}</button>
            </div>
            <div class="duration-custom-row"><input v-model="modelForm.custom_video_duration" class="text-input" type="number" min="1" max="60" placeholder="其他秒数" @keydown.enter.prevent="addCustomDuration(modelForm)" /><button class="mini-button" type="button" @click="addCustomDuration(modelForm)">添加</button></div>
            <div class="duration-selected"><button v-for="value in modelForm.video_durations" :key="value" type="button" @click="toggleDuration(modelForm, value)">{{ value === '-1' ? '自动时长' : `${value}s` }} ×</button></div>
            <label><span>默认时长</span><ClickSelect v-model="modelForm.default_video_duration" :options="modelForm.video_durations.map((value) => ({ value, label: value === '-1' ? '自动时长' : `${value} 秒` }))" class="select-input" data-testid="model-default-duration-input" aria-label="默认时长" /></label>
          </section>
          <label><span>排序</span><input v-model.number="modelForm.sort_order" class="text-input" data-testid="model-sort-input" type="number" min="0" /></label>
        </template>

        <template v-if="dialog.type === 'provider'">
          <label><span>名称</span><input v-model="providerForm.name" class="text-input" data-testid="provider-name-input" required /></label>
          <div class="dialog-grid">
            <label><span>Provider</span><input v-model="providerForm.provider" class="text-input" data-testid="provider-code-input" /></label>
            <label><span>状态</span><ClickSelect v-model="providerForm.status" :options="onlineStatusOptions" class="select-input" data-testid="provider-status-input" aria-label="状态" /></label>
            <label><span>默认超时</span><input v-model.number="providerForm.default_timeout_seconds" class="text-input" data-testid="provider-timeout-input" type="number" min="0" /></label>
            <label><span>并发限制</span><input v-model.number="providerForm.concurrency_limit" class="text-input" data-testid="provider-concurrency-input" type="number" min="0" /></label>
          </div>
          <label><span>Base URL</span><input v-model="providerForm.base_url" class="text-input" data-testid="provider-base-url-input" placeholder="https://api.openai.com" /></label>
          <label><span>API Key</span><input v-model="providerForm.api_key" class="text-input" data-testid="provider-api-key-input" type="password" :placeholder="providerForm.api_key_set ? '已设置，留空不变' : ''" autocomplete="new-password" /></label>
          <label v-if="providerForm.api_key_set" class="inline-check"><input v-model="providerForm.clear_api_key" data-testid="provider-clear-key-input" type="checkbox" />清空 API Key</label>
        </template>

        <template v-if="dialog.type === 'channel'">
          <label><span>名称</span><input v-model="channelForm.name" class="text-input" data-testid="channel-name-input" required /></label>
          <div class="dialog-grid">
            <label><span>业务模型</span><ClickSelect v-model="channelForm.model_id" :options="overview.models.map((model) => ({ value: model.id, label: model.name }))" class="select-input" data-testid="channel-model-input" aria-label="业务模型" /></label>
            <label><span>供应商</span><ClickSelect v-model="channelForm.provider_id" :options="overview.providers.map((provider) => ({ value: provider.id, label: provider.name }))" class="select-input" data-testid="channel-provider-input" aria-label="供应商" /></label>
            <label><span>状态</span><ClickSelect v-model="channelForm.status" :options="onlineStatusOptions" class="select-input" data-testid="channel-status-input" aria-label="状态" /></label>
            <label><span>健康</span><ClickSelect v-model="channelForm.health_status" :options="healthStatusOptions" class="select-input" data-testid="channel-health-input" aria-label="健康" /></label>
            <label><span>权重</span><input v-model.number="channelForm.weight" class="text-input" data-testid="channel-weight-input" type="number" min="0" max="100" /></label>
            <label><span>优先级</span><input v-model.number="channelForm.priority" class="text-input" data-testid="channel-priority-input" type="number" min="1" /></label>
          </div>
          <label><span>Runtime Model</span><input v-model="channelForm.runtime_model" class="text-input" data-testid="channel-runtime-input" placeholder="gpt-image-2" /></label>
          <section v-if="selectedChannelModel?.modality === 'video'" class="video-duration-editor" data-testid="channel-video-duration-editor">
            <div class="duration-editor-title"><strong>供应商支持时长</strong><button class="mini-button" type="button" @click="recommendedChannelDurations">应用系统推荐能力</button></div>
            <div class="duration-quick-options">
              <button v-for="value in commonVideoDurations" :key="value" type="button" :class="{ active: channelForm.video_durations.includes(value) }" @click="toggleDuration(channelForm, value)">{{ value === '-1' ? '自动时长' : `${value} 秒` }}</button>
            </div>
            <div class="duration-custom-row"><input v-model="channelForm.custom_video_duration" class="text-input" type="number" min="1" max="60" placeholder="其他秒数" @keydown.enter.prevent="addCustomDuration(channelForm)" /><button class="mini-button" type="button" @click="addCustomDuration(channelForm)">添加</button></div>
            <div class="duration-selected"><button v-for="value in channelForm.video_durations" :key="value" type="button" @click="toggleDuration(channelForm, value)">{{ value === '-1' ? '自动时长' : `${value}s` }} ×</button></div>
            <p v-if="channelDurationMissing.length" class="duration-conflict">与业务模型冲突，缺少：{{ channelDurationMissing.join(' / ') }}s</p>
            <p v-else class="duration-compatible">与业务模型时长兼容</p>
          </section>
          <label><span>Endpoint</span><input v-model="channelForm.endpoint" class="text-input" data-testid="channel-endpoint-input" placeholder="/v1/images/generations" /></label>
        </template>

        <button class="primary-button" type="submit" :disabled="saving">{{ saving ? '保存中...' : '保存' }}</button>
      </form>
    </div>

    <div
      v-if="channelCallAttemptModal.open"
      class="model-center-dialog-backdrop"
      data-testid="channel-call-attempt-modal-backdrop"
      @click.self="closeChannelCallAttemptModal"
    >
      <section class="admin-panel channel-call-attempt-modal" data-testid="channel-call-attempt-modal" role="dialog" aria-modal="true">
        <div class="panel-title-row">
          <div>
            <p class="panel-kicker">Call Attempts</p>
            <h2>{{ channelCallAttemptModal.channel?.name || '渠道调用明细' }}</h2>
            <small>{{ channelCallAttemptModal.channel?.model_name || monitoringModelName({ model_id: channelCallAttemptModal.model_id }) }} · {{ channelCallAttemptModal.channel?.runtime_model || '-' }}</small>
          </div>
          <button class="mini-button icon-only" type="button" data-testid="channel-call-attempt-modal-close" @click="closeChannelCallAttemptModal"><X :size="16" /></button>
        </div>

        <div class="channel-call-attempt-filters">
          <label>
            <span>状态</span>
            <ClickSelect v-model="channelCallAttemptFilters.status" :options="callAttemptStatusOptions" class="select-input" data-testid="channel-call-attempt-status" aria-label="状态" />
          </label>
          <label>
            <span>开始日期</span>
            <input v-model="channelCallAttemptFilters.date_from" class="text-input" data-testid="channel-call-attempt-date-from" type="date" />
          </label>
          <label>
            <span>结束日期</span>
            <input v-model="channelCallAttemptFilters.date_to" class="text-input" data-testid="channel-call-attempt-date-to" type="date" />
          </label>
          <div class="channel-call-attempt-filter-actions">
            <button class="primary-button compact-button" type="button" data-testid="channel-call-attempt-query" :disabled="channelCallAttemptModal.loading" @click="queryChannelCallAttempts">
              <Search :size="15" />
              查询
            </button>
            <button class="mini-button compact-button" type="button" data-testid="channel-call-attempt-reset" :disabled="channelCallAttemptModal.loading" @click="resetChannelCallAttemptFilters">
              重置
            </button>
          </div>
        </div>

        <div class="admin-table-scroll channel-call-attempt-table-scroll">
          <table class="admin-data-table channel-call-attempt-table">
            <thead>
              <tr>
                <th>时间</th>
                <th>任务ID</th>
                <th>尝试次数</th>
                <th>状态</th>
                <th>耗时</th>
                <th>HTTP</th>
                <th>错误码</th>
                <th>失败阶段</th>
                <th>Request ID</th>
                <th>错误消息</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="channelCallAttemptModal.loading">
                <td colspan="10" class="empty-cell">调用尝试加载中...</td>
              </tr>
              <tr v-else-if="channelCallAttemptModal.error">
                <td colspan="10" class="empty-cell">{{ channelCallAttemptModal.error }}</td>
              </tr>
              <tr v-else-if="channelCallAttemptModal.items.length === 0">
                <td colspan="10" class="empty-cell">暂无调用尝试</td>
              </tr>
              <tr v-for="item in channelCallAttemptModal.items" v-else :key="item.id">
                <td>{{ formatTime(item.started_at) }}</td>
                <td>#{{ item.generation_record_id || '-' }}</td>
                <td>{{ item.attempt_index || '-' }}</td>
                <td><span class="status-pill" :class="callAttemptStatusClass(item.status)">{{ callAttemptStatusLabel(item.status) }}</span></td>
                <td>{{ formatLatency(item.latency_ms) }}</td>
                <td>{{ formatHTTPStatus(item.http_status) }}</td>
                <td>{{ item.error_code || '-' }}</td>
                <td>{{ item.failure_stage || '-' }}</td>
                <td>{{ item.provider_request_id || '-' }}</td>
                <td class="call-attempt-error-message">{{ item.error_message || '-' }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <div class="channel-call-attempt-pagination">
          <span>{{ channelCallAttemptRange }}</span>
          <div>
            <button class="mini-button compact-button" type="button" data-testid="channel-call-attempt-prev" :disabled="channelCallAttemptModal.loading || !channelCallAttemptCanPrev" @click="previousChannelCallAttempts">上一页</button>
            <button class="mini-button compact-button" type="button" data-testid="channel-call-attempt-next" :disabled="channelCallAttemptModal.loading || !channelCallAttemptCanNext" @click="nextChannelCallAttempts">下一页</button>
          </div>
        </div>
      </section>
    </div>
  </section>
</template>

<style scoped>
.model-center-page {
  display: grid;
  gap: 16px;
}

.model-center-heading {
  align-items: center;
}

.model-center-heading .eyebrow {
  color: #667085;
  letter-spacing: 0;
  text-transform: none;
}

.model-center-summary {
  display: grid;
  grid-template-columns: repeat(3, minmax(160px, 1fr));
  gap: 10px;
}

.model-center-summary > div,
.model-center-tabs,
.model-center-panel {
  border: 1px solid rgba(117, 130, 156, 0.16);
  border-radius: 10px;
  background: rgba(255, 255, 255, 0.88);
}

.model-center-summary > div {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px;
}

.model-center-summary span,
.admin-data-table small,
.routing-entry small {
  color: #667085;
}

.model-center-summary strong {
  color: #111827;
  font-size: 24px;
}

.model-center-tabs {
  display: flex;
  gap: 4px;
  padding: 6px;
  overflow-x: auto;
}

.model-center-tabs button {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  min-height: 36px;
  padding: 0 12px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: #475467;
  cursor: pointer;
  white-space: nowrap;
}

.model-center-tabs button.active {
  background: #111827;
  color: #fff;
}

.model-center-panel {
  padding: 16px;
}

.model-center-stack {
  display: grid;
  gap: 16px;
}

.empty-cell {
  padding: 28px;
  text-align: center;
  color: #667085;
}

.status-pill {
  display: inline-flex;
  align-items: center;
  min-height: 24px;
  padding: 0 8px;
  border-radius: 999px;
  background: #ecfdf3;
  color: #067647;
  font-size: 12px;
  font-weight: 700;
}

.status-pill.offline {
  background: #f2f4f7;
  color: #667085;
}

.status-pill.warn {
  background: #fffaeb;
  color: #b54708;
}

.row-actions {
  display: flex;
  gap: 6px;
}

.routing-grid {
  display: grid;
  gap: 16px;
}

.routing-block {
  display: grid;
  gap: 12px;
  padding-top: 4px;
  border-top: 1px solid rgba(117, 130, 156, 0.14);
}

.routing-head,
.routing-entry {
  display: flex;
  align-items: center;
  gap: 10px;
}

.routing-head {
  justify-content: space-between;
}

.routing-fields,
.dialog-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

.routing-entry-list {
  display: grid;
  gap: 8px;
}

.routing-entry {
  grid-template-columns: auto minmax(180px, 1fr) 80px 80px;
}

.routing-entry span,
.admin-data-table td:first-child {
  display: grid;
  gap: 2px;
}

.routing-entry input[type='number'] {
  width: 100%;
  min-height: 34px;
  border: 1px solid rgba(117, 130, 156, 0.22);
  border-radius: 8px;
  padding: 0 8px;
}

.model-center-dialog-backdrop {
  position: fixed;
  inset: 0;
  z-index: 50;
  display: grid;
  place-items: center;
  padding: 20px;
  background: rgba(15, 23, 42, 0.38);
}

.model-center-dialog {
  display: grid;
  gap: 12px;
  width: min(720px, 100%);
  max-height: calc(100vh - 40px);
  overflow: auto;
  padding: 18px;
}

.model-center-dialog label,
.routing-fields label {
  display: grid;
  gap: 6px;
  color: #344054;
  font-size: 13px;
  font-weight: 700;
}

.video-duration-editor {
  display: grid;
  gap: 10px;
  padding: 12px;
  border: 1px solid rgba(117, 130, 156, 0.22);
  border-radius: 10px;
  background: rgba(248, 250, 252, 0.72);
}

.duration-editor-title,
.duration-custom-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.duration-custom-row .text-input {
  flex: 1;
}

.duration-quick-options,
.duration-selected {
  display: flex;
  flex-wrap: wrap;
  gap: 7px;
}

.duration-quick-options button,
.duration-selected button {
  min-height: 30px;
  padding: 0 10px;
  border: 1px solid rgba(117, 130, 156, 0.24);
  border-radius: 999px;
  background: #fff;
  color: #344054;
  cursor: pointer;
}

.duration-quick-options button.active {
  border-color: #0ea5e9;
  background: #e0f2fe;
  color: #0369a1;
}

.duration-conflict,
.duration-warning {
  color: #b54708;
}

.duration-compatible {
  color: #067647;
}

.inline-check {
  display: flex !important;
  align-items: center;
  gap: 8px;
}

.channel-call-attempt-modal {
  display: grid;
  gap: 14px;
  width: min(1180px, 100%);
  max-height: calc(100vh - 40px);
  overflow: hidden;
  padding: 18px;
}

.channel-call-attempt-modal .panel-title-row small {
  display: block;
  margin-top: 4px;
  color: #667085;
}

.channel-call-attempt-filters {
  display: grid;
  grid-template-columns: 150px 180px 180px auto;
  align-items: end;
  gap: 12px;
}

.channel-call-attempt-filters label {
  display: grid;
  gap: 6px;
  color: #344054;
  font-size: 13px;
  font-weight: 700;
}

.channel-call-attempt-filter-actions,
.channel-call-attempt-pagination,
.channel-call-attempt-pagination > div {
  display: flex;
  align-items: center;
  gap: 8px;
}

.channel-call-attempt-table-scroll {
  max-height: min(58vh, 560px);
  overflow: auto;
}

.channel-call-attempt-table {
  min-width: 1120px;
}

.call-attempt-error-message {
  max-width: 260px;
  white-space: normal;
}

.channel-call-attempt-pagination {
  justify-content: space-between;
  color: #667085;
  font-size: 13px;
  font-weight: 700;
}

@media (max-width: 900px) {
  .model-center-summary,
  .routing-fields,
  .dialog-grid,
  .channel-call-attempt-filters {
    grid-template-columns: 1fr;
  }

  .routing-entry {
    grid-template-columns: auto minmax(0, 1fr);
  }

  .routing-entry input[type='number'] {
    grid-column: 2;
  }
}
</style>
