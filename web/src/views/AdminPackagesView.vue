<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import {
  BadgePercent,
  CalendarClock,
  ChevronLeft,
  ChevronRight,
  Copy,
  MoreHorizontal,
  PackagePlus,
  PencilLine,
  Power,
  RefreshCw,
  ShoppingBag,
  Trash2,
  WalletCards
} from 'lucide-vue-next'

import { api } from '../api/client.js'

const items = ref([])
const loading = ref(false)
const saving = ref(false)
const errorMessage = ref('')
const successMessage = ref('')
const mode = ref('create')
const selectedId = ref(null)
const editorOpen = ref(false)
const bulkMode = ref(false)
const selectedPackageIds = ref([])
const page = ref(1)
const pageSize = 6

const summary = reactive({
  active_packages: 0,
  active_packages_delta_percent: 0,
  active_packages_sparkline: [],
  revenue_share_percent: 0,
  revenue_share_delta_percent: 0,
  revenue_share_sparkline: [],
  average_order_cents: 0,
  average_order_delta_percent: 0,
  average_order_sparkline: [],
  monthly_orders: 0,
  monthly_orders_delta_percent: 0,
  monthly_orders_sparkline: []
})

const form = reactive(defaultForm())
const themePresets = ['blue', 'green', 'orange', 'violet', 'rose', 'gold']

const totalPages = computed(() => Math.max(1, Math.ceil(items.value.length / pageSize)))
const rangeStart = computed(() => (items.value.length === 0 ? 0 : (page.value - 1) * pageSize + 1))
const rangeEnd = computed(() => Math.min(items.value.length, page.value * pageSize))
const pagedItems = computed(() => items.value.slice((page.value - 1) * pageSize, page.value * pageSize))
const selectedPackage = computed(() => items.value.find((item) => item.id === selectedId.value))
const selectedBulkPackages = computed(() => items.value.filter((item) => selectedPackageIds.value.includes(item.id)))
const allPagedItemsSelected = computed(() => pagedItems.value.length > 0 && pagedItems.value.every((item) => selectedPackageIds.value.includes(item.id)))
const editorTitle = computed(() => (mode.value === 'edit' ? '编辑套餐' : '新增套餐'))
const submitText = computed(() => {
  if (saving.value) return '保存中...'
  return mode.value === 'edit' ? '保存套餐' : '新增套餐'
})
const descriptionCount = computed(() => form.description.trim().length)

const kpiCards = computed(() => [
  {
    key: 'active',
    label: '在售套餐',
    value: formatNumber(summary.active_packages),
    delta: summary.active_packages_delta_percent,
    sparkline: summary.active_packages_sparkline,
    icon: ShoppingBag
  },
  {
    key: 'revenue',
    label: '套餐收入占比',
    value: `${formatNumber(summary.revenue_share_percent)}%`,
    delta: summary.revenue_share_delta_percent,
    sparkline: summary.revenue_share_sparkline,
    icon: BadgePercent
  },
  {
    key: 'average',
    label: '平均客单价',
    value: formatYuan(summary.average_order_cents),
    delta: summary.average_order_delta_percent,
    sparkline: summary.average_order_sparkline,
    icon: WalletCards
  },
  {
    key: 'orders',
    label: '本月订单',
    value: formatNumber(summary.monthly_orders),
    delta: summary.monthly_orders_delta_percent,
    sparkline: summary.monthly_orders_sparkline,
    icon: CalendarClock
  }
])

function defaultForm() {
  const defaults = defaultPackagePresentation({ credits: 20, valid_days: 30, audience: '轻度用户 / 新手体验' }, 0)
  return {
    name: '',
    priceYuan: '',
    credits: 20,
    validDays: 30,
    audience: '',
    tagsText: '',
    icon: '',
    theme: 'blue',
    badge: '',
    sortOrder: 0,
    recommended: false,
    featuresText: defaults.features.join('\n'),
    benefitsText: benefitsToText(defaults.benefits),
    wechatVirtualProductId: '',
    description: '',
    isActive: true
  }
}

function resetForm() {
  Object.assign(form, defaultForm())
  selectedId.value = null
  mode.value = 'create'
  errorMessage.value = ''
}

function openCreatePackage() {
  resetForm()
  successMessage.value = ''
  editorOpen.value = true
}

function closeEditor() {
  editorOpen.value = false
  resetForm()
}

async function load() {
  loading.value = true
  errorMessage.value = ''
  try {
    const payload = await api.listAdminPackages()
    items.value = payload.items ?? []
    Object.assign(summary, payload.summary ?? {})
    if (page.value > totalPages.value) {
      page.value = totalPages.value
    }
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    loading.value = false
  }
}

function editPackage(item) {
  const itemIndex = items.value.findIndex((entry) => entry.id === item.id)
  const defaults = defaultPackagePresentation(item, itemIndex < 0 ? 0 : itemIndex)
  selectedId.value = item.id
  mode.value = 'edit'
  form.name = item.name ?? ''
  form.priceYuan = centsToYuan(item.price_cents, item.price_label)
  form.credits = Number(item.credits ?? 20)
  form.validDays = Number(item.valid_days ?? 30)
  form.audience = item.audience ?? ''
  form.tagsText = (item.tags ?? []).join(', ')
  form.icon = item.icon ?? ''
  form.theme = item.theme ?? 'blue'
  form.badge = item.badge ?? ''
  form.sortOrder = Number(item.sort_order ?? 0)
  form.recommended = Boolean(item.recommended)
  form.featuresText = (item.features?.length ? item.features : defaults.features).join('\n')
  form.benefitsText = benefitsToText(item.benefits?.length ? item.benefits : defaults.benefits)
  form.wechatVirtualProductId = item.wechat_virtual_product_id ?? ''
  form.description = item.description ?? ''
  form.isActive = Boolean(item.is_active)
  errorMessage.value = ''
  successMessage.value = ''
  editorOpen.value = true
}

function packagePayload() {
  const priceCents = priceYuanToCents(form.priceYuan)
  const credits = Number(form.credits)
  const validDays = Number(form.validDays)
  if (!form.name.trim() || priceCents <= 0 || credits <= 0 || validDays <= 0) {
    throw new Error('请填写套餐名称、有效售价、点数和有效期')
  }
  return {
    name: form.name.trim(),
    price_cents: priceCents,
    credits,
    valid_days: validDays,
    audience: form.audience.trim(),
    tags: splitTags(form.tagsText),
    icon: form.icon.trim(),
    theme: form.theme.trim(),
    badge: form.badge.trim(),
    sort_order: Number(form.sortOrder || 0),
    recommended: Boolean(form.recommended),
    features: splitLines(form.featuresText),
    benefits: splitBenefits(form.benefitsText),
    wechat_virtual_product_id: form.wechatVirtualProductId.trim(),
    description: form.description.trim(),
    is_active: form.isActive
  }
}

async function savePackage() {
  saving.value = true
  errorMessage.value = ''
  successMessage.value = ''
  try {
    const payload = packagePayload()
    if (mode.value === 'edit' && selectedId.value) {
      await api.updateAdminPackage(selectedId.value, payload)
      successMessage.value = '套餐已保存'
    } else {
      await api.createAdminPackage(payload)
      successMessage.value = '套餐已新增'
    }
    editorOpen.value = false
    resetForm()
    await load()
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    saving.value = false
  }
}

async function copyPackage(item) {
  errorMessage.value = ''
  successMessage.value = ''
  try {
    await api.createAdminPackage({
      name: `${item.name} 副本`,
      price_cents: Number(item.price_cents ?? priceYuanToCents(item.price_label)),
      credits: Number(item.credits ?? 1),
      valid_days: Number(item.valid_days ?? 30),
      audience: item.audience ?? '',
      tags: [...(item.tags ?? [])],
      description: item.description ?? '',
      is_active: false,
      icon: item.icon ?? '',
      theme: item.theme ?? '',
      badge: item.badge ?? '',
      recommended: Boolean(item.recommended),
      features: [...(item.features ?? [])],
      benefits: [...(item.benefits ?? [])],
      wechat_virtual_product_id: item.wechat_virtual_product_id ?? '',
      sort_order: Number(item.sort_order ?? 0)
    })
    successMessage.value = '套餐副本已创建'
    await load()
  } catch (error) {
    errorMessage.value = error.message
  }
}

async function togglePackage(item) {
  errorMessage.value = ''
  successMessage.value = ''
  try {
    await api.updateAdminPackage(item.id, { is_active: !item.is_active })
    successMessage.value = item.is_active ? '套餐已停用' : '套餐已启用'
    await load()
  } catch (error) {
    errorMessage.value = error.message
  }
}

async function deletePackage(item) {
  if (typeof window !== 'undefined' && window.confirm && !window.confirm(`删除套餐「${item.name}」？`)) {
    return
  }
  errorMessage.value = ''
  successMessage.value = ''
  try {
    await api.deleteAdminPackage(item.id)
    if (selectedId.value === item.id) {
      resetForm()
    }
    successMessage.value = '套餐已删除'
    await load()
  } catch (error) {
    errorMessage.value = error.message
  }
}

function toggleBulkMode() {
  bulkMode.value = !bulkMode.value
  selectedPackageIds.value = []
  errorMessage.value = ''
  successMessage.value = ''
}

function setPackageSelected(id, checked) {
  if (checked) {
    if (!selectedPackageIds.value.includes(id)) {
      selectedPackageIds.value = [...selectedPackageIds.value, id]
    }
    return
  }
  selectedPackageIds.value = selectedPackageIds.value.filter((itemId) => itemId !== id)
}

function setPagedItemsSelected(checked) {
  const pageIds = pagedItems.value.map((item) => item.id)
  if (checked) {
    selectedPackageIds.value = Array.from(new Set([...selectedPackageIds.value, ...pageIds]))
    return
  }
  selectedPackageIds.value = selectedPackageIds.value.filter((id) => !pageIds.includes(id))
}

async function bulkSetActive(isActive) {
  if (selectedBulkPackages.value.length === 0) {
    errorMessage.value = '请先选择套餐'
    return
  }
  saving.value = true
  errorMessage.value = ''
  successMessage.value = ''
  try {
    await Promise.all(selectedBulkPackages.value.map((item) => api.updateAdminPackage(item.id, { is_active: isActive })))
    successMessage.value = isActive ? '已批量启用套餐' : '已批量停用套餐'
    selectedPackageIds.value = []
    await load()
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    saving.value = false
  }
}

async function bulkDeletePackages() {
  if (selectedBulkPackages.value.length === 0) {
    errorMessage.value = '请先选择套餐'
    return
  }
  if (typeof window !== 'undefined' && window.confirm && !window.confirm(`删除选中的 ${selectedBulkPackages.value.length} 个套餐？`)) {
    return
  }
  saving.value = true
  errorMessage.value = ''
  successMessage.value = ''
  try {
    const deletingIds = selectedBulkPackages.value.map((item) => item.id)
    await Promise.all(deletingIds.map((id) => api.deleteAdminPackage(id)))
    if (selectedId.value && deletingIds.includes(selectedId.value)) {
      resetForm()
    }
    successMessage.value = '已批量删除套餐'
    selectedPackageIds.value = []
    await load()
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    saving.value = false
  }
}

function goToPage(nextPage) {
  page.value = Math.min(Math.max(nextPage, 1), totalPages.value)
}

function formatNumber(value) {
  return new Intl.NumberFormat('zh-CN').format(Number(value ?? 0))
}

function formatYuan(cents) {
  const value = Number(cents ?? 0)
  return `￥${Math.floor(value / 100)}.${`${Math.abs(value % 100)}`.padStart(2, '0')}`
}

function centsToYuan(cents, fallbackLabel = '') {
  const value = Number(cents ?? 0)
  if (value > 0) {
    return (value / 100).toFixed(2).replace(/\.00$/, '').replace(/(\.\d)0$/, '$1')
  }
  const match = `${fallbackLabel}`.match(/\d+(?:\.\d+)?/)
  return match?.[0] ?? ''
}

function priceYuanToCents(value) {
  const match = `${value}`.match(/\d+(?:\.\d+)?/)
  if (!match) return 0
  return Math.round(Number(match[0]) * 100)
}

function splitTags(value) {
  const seen = new Set()
  return `${value}`
    .split(/[,，\s]+/)
    .map((tag) => tag.trim())
    .filter((tag) => {
      if (!tag || seen.has(tag)) return false
      seen.add(tag)
      return true
    })
}

function splitLines(value) {
  const seen = new Set()
  return `${value}`
    .split(/\n+/)
    .map((line) => line.trim())
    .filter((line) => {
      if (!line || seen.has(line)) return false
      seen.add(line)
      return true
    })
}

function splitBenefits(value) {
  const seen = new Set()
  return `${value}`
    .split(/\n+/)
    .map((line) => {
      const [label = '', benefitValue = ''] = line.split('|')
      return {
        label: label.trim(),
        value: benefitValue.trim()
      }
    })
    .filter((benefit) => {
      if (!benefit.label || !benefit.value || seen.has(benefit.label)) return false
      seen.add(benefit.label)
      return true
    })
}

function benefitsToText(benefits = []) {
  return benefits.map((benefit) => `${benefit.label}|${benefit.value}`).join('\n')
}

function defaultPackagePresentation(item = {}, index = 0) {
  const name = item.name ?? ''
  const credits = Number(item.credits ?? 20)
  const audience = item.audience || defaultAudienceForPackage(item, index)
  return {
    features: defaultFeaturesForPackage(item, index),
    benefits: [
      { label: '点数', value: `${credits} 点` },
      { label: '图片生成', value: '✓' },
      { label: '视频生成', value: '✓' },
      { label: '图生视频 / 参考图能力', value: '✓' },
      { label: '高清下载', value: '✓' },
      { label: '私有作品库', value: '✓' },
      { label: '队列优先级', value: defaultQueuePriority(name, credits, index) },
      { label: '商用授权', value: credits >= 120 || name.includes('团队') || name.includes('高频') ? '✓' : '—' },
      { label: '适合人群', value: audience }
    ]
  }
}

function defaultFeaturesForPackage(item = {}, index = 0) {
  const name = item.name ?? ''
  const credits = Number(item.credits ?? 20)
  if (name.includes('团队') || credits >= 300 || index === 3) {
    return ['支持图片生成', '支持视频生成', '支持参考图 / 图生视频', '作品入库与历史管理', '失败任务不扣点，以生成页实时提示为准', '商用授权', '团队协作']
  }
  if (name.includes('高频') || credits >= 120 || index === 2) {
    return ['支持图片生成', '支持视频生成', '支持参考图 / 图生视频', '作品入库与历史管理', '失败任务不扣点，以生成页实时提示为准', '商用授权']
  }
  if (name.includes('创作') || credits >= 50 || index === 1) {
    return ['支持图片生成', '支持视频生成', '支持参考图 / 图生视频', '作品入库与历史管理', '失败任务不扣点，以生成页实时提示为准', '优先队列']
  }
  return ['支持图片生成', '支持视频生成', '支持参考图 / 图生视频', '作品入库与历史管理', '失败任务不扣点，以生成页实时提示为准', '基础排队']
}

function defaultQueuePriority(name, credits, index) {
  if (name.includes('团队') || credits >= 300 || index === 3) return '最高优先'
  if (name.includes('高频') || credits >= 120 || index === 2) return '更高优先'
  if (name.includes('创作') || credits >= 50 || index === 1) return '优先'
  return '普通'
}

function defaultAudienceForPackage(item = {}, index = 0) {
  const name = item.name ?? ''
  const credits = Number(item.credits ?? 20)
  if (name.includes('团队') || credits >= 300 || index === 3) return '团队 / 工作室 / 企业'
  if (name.includes('高频') || credits >= 120 || index === 2) return '专业创作者 / 自媒体'
  if (name.includes('创作') || credits >= 50 || index === 1) return '个人创作者 / 频繁创作'
  return '轻度用户 / 新手体验'
}

function formatDelta(value) {
  const number = Number(value ?? 0)
  const text = Number.isInteger(number) ? number.toFixed(0) : number.toFixed(1)
  return `${number > 0 ? '+' : ''}${text}%`
}

function sparklineHeights(values = []) {
  const numbers = values.map((value) => Number(value ?? 0))
  const max = Math.max(...numbers, 1)
  return numbers.map((value) => Math.max(7, Math.round((value / max) * 30)))
}

function statusText(item) {
  return item.is_active ? '启用' : '停用'
}

function themeClass(item) {
  return `package-theme-${item.theme || 'blue'}`
}

function initials(item) {
  return (item.name || '套').slice(0, 1).toUpperCase()
}

onMounted(load)
</script>

<template>
  <section class="admin-packages-page">
    <div class="admin-page-heading packages-heading">
      <div>
        <p class="eyebrow">Packages</p>
        <h1>套餐配置</h1>
        <span>管理售卖套餐、点数有效期和购买入口状态</span>
      </div>
      <div class="packages-heading-actions">
        <button class="secondary-button icon-button-text" data-testid="toggle-bulk-packages" type="button" @click="toggleBulkMode">
          <MoreHorizontal :size="16" />
          {{ bulkMode ? '退出批量' : '批量管理' }}
        </button>
        <button class="secondary-button icon-button-text" type="button" @click="load">
          <RefreshCw :size="16" />
          刷新
        </button>
        <button class="primary-button icon-button-text" data-testid="new-package" type="button" @click="openCreatePackage">
          <PackagePlus :size="17" />
          新增套餐
        </button>
      </div>
    </div>

    <div class="packages-kpi-grid">
      <article v-for="card in kpiCards" :key="card.key" class="package-kpi-card">
        <div class="package-kpi-topline">
          <span><component :is="card.icon" :size="18" /> {{ card.label }}</span>
          <b class="users-kpi-delta" :class="{ negative: card.delta < 0 }">{{ formatDelta(card.delta) }}</b>
        </div>
        <strong>{{ card.value }}</strong>
        <div class="users-sparkline" aria-hidden="true">
          <i
            v-for="(height, index) in sparklineHeights(card.sparkline)"
            :key="`${card.key}-${index}`"
            :style="{ height: `${height}px` }"
          />
        </div>
      </article>
    </div>

    <div class="admin-packages-workspace">
      <div class="admin-panel packages-table-panel">
        <div class="panel-title-row">
          <div>
            <p class="eyebrow">Package list</p>
            <h2>套餐列表</h2>
          </div>
          <span class="packages-count">{{ items.length }} 个套餐</span>
        </div>

        <div v-if="bulkMode" class="packages-bulk-toolbar" data-testid="packages-bulk-toolbar">
          <span>已选择 {{ selectedPackageIds.length }} 个套餐</span>
          <div class="inline-actions">
            <button class="mini-button" data-testid="bulk-enable-packages" type="button" :disabled="saving || selectedPackageIds.length === 0" @click="bulkSetActive(true)">批量启用</button>
            <button class="mini-button" data-testid="bulk-disable-packages" type="button" :disabled="saving || selectedPackageIds.length === 0" @click="bulkSetActive(false)">批量停用</button>
            <button class="mini-button destructive-button" data-testid="bulk-delete-packages" type="button" :disabled="saving || selectedPackageIds.length === 0" @click="bulkDeletePackages">批量删除</button>
          </div>
        </div>

        <div class="admin-table-scroll packages-table-scroll">
          <table class="data-table admin-data-table packages-data-table">
            <thead>
              <tr>
                <th v-if="bulkMode" class="package-select-column">
                  <input
                    type="checkbox"
                    data-testid="select-page-packages"
                    :checked="allPagedItemsSelected"
                    aria-label="选择当前页套餐"
                    @change="setPagedItemsSelected($event.target.checked)"
                  />
                </th>
                <th>套餐名称</th>
                <th>价格</th>
                <th>点数</th>
                <th>有效期</th>
                <th>状态</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="item in pagedItems"
                :key="item.id"
                :class="{ selected: item.id === selectedId }"
              >
                <td v-if="bulkMode" class="package-select-column">
                  <input
                    type="checkbox"
                    :data-testid="`select-package-${item.id}`"
                    :checked="selectedPackageIds.includes(item.id)"
                    :aria-label="`选择套餐 ${item.name}`"
                    @change="setPackageSelected(item.id, $event.target.checked)"
                  />
                </td>
                <td>
                  <div class="package-name-cell">
                    <span class="package-icon-chip" :class="themeClass(item)">{{ initials(item) }}</span>
                    <div>
                      <strong>{{ item.name }}</strong>
                      <small>{{ item.audience || '全部用户' }}</small>
                    </div>
                  </div>
                </td>
                <td>
                  <strong>{{ item.price_label || formatYuan(item.price_cents) }}</strong>
                  <small>{{ formatYuan(item.price_cents) }}</small>
                </td>
                <td>{{ formatNumber(item.credits) }}</td>
                <td>{{ item.valid_days || 0 }} 天</td>
                <td>
                  <span class="status-pill" :class="item.is_active ? 'status-active' : 'status-disabled'">
                    {{ statusText(item) }}
                  </span>
                </td>
                <td>
                  <div class="table-icon-actions">
                    <button class="mini-button icon-only" :data-testid="`edit-package-${item.id}`" type="button" aria-label="编辑套餐" @click="editPackage(item)">
                      <PencilLine :size="15" />
                    </button>
                    <button class="mini-button icon-only" :data-testid="`copy-package-${item.id}`" type="button" aria-label="复制套餐" @click="copyPackage(item)">
                      <Copy :size="15" />
                    </button>
                    <button class="mini-button icon-only" :data-testid="`toggle-package-${item.id}`" type="button" :aria-label="item.is_active ? '停用套餐' : '启用套餐'" @click="togglePackage(item)">
                      <Power :size="15" />
                    </button>
                    <button class="mini-button icon-only destructive-button" :data-testid="`delete-package-${item.id}`" type="button" aria-label="删除套餐" @click="deletePackage(item)">
                      <Trash2 :size="15" />
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
          <p v-if="loading" class="page-status">加载中...</p>
          <p v-else-if="items.length === 0" class="page-status">暂无套餐</p>
        </div>

        <div class="admin-pagination" data-testid="packages-pagination">
          <span>第 {{ rangeStart }}-{{ rangeEnd }} 条 / 共 {{ items.length }} 条</span>
          <div class="inline-actions">
            <button class="mini-button" type="button" :disabled="page <= 1" @click="goToPage(page - 1)">
              <ChevronLeft :size="15" />
              上一页
            </button>
            <button class="mini-button" type="button" :disabled="page >= totalPages" @click="goToPage(page + 1)">
              下一页
              <ChevronRight :size="15" />
            </button>
          </div>
        </div>
      </div>

      <div v-if="editorOpen" class="package-editor-modal-backdrop" data-testid="package-editor-modal" @click.self="closeEditor">
        <form class="admin-panel package-editor-panel package-editor-modal" data-testid="package-editor-form" @submit.prevent="savePackage">
          <div class="panel-title-row">
            <div>
              <p class="eyebrow">Package editor</p>
              <h2>{{ editorTitle }}</h2>
            </div>
            <button class="mini-button" type="button" data-testid="close-package-editor" @click="closeEditor">关闭</button>
          </div>

          <label class="field-label" for="packageName">名称</label>
          <input id="packageName" v-model="form.name" data-testid="package-name" class="text-input" maxlength="32" />

          <div class="package-form-row">
            <label>
              <span class="field-label">售价</span>
              <input v-model="form.priceYuan" data-testid="package-price" class="text-input" inputmode="decimal" placeholder="99" />
            </label>
            <label>
              <span class="field-label">点数</span>
              <input v-model.number="form.credits" data-testid="package-credits" class="text-input" type="number" min="1" />
            </label>
          </div>

          <div class="package-form-row">
            <label>
              <span class="field-label">有效期</span>
              <input v-model.number="form.validDays" data-testid="package-valid-days" class="text-input" type="number" min="1" />
            </label>
            <label>
              <span class="field-label">适用人群</span>
              <input v-model="form.audience" data-testid="package-audience" class="text-input" maxlength="32" />
            </label>
          </div>

          <label class="field-label" for="packageTags">标签</label>
          <input id="packageTags" v-model="form.tagsText" data-testid="package-tags" class="text-input" placeholder="商用, 海报, 团队" />

          <div class="package-form-row">
            <label>
              <span class="field-label">卡片图标</span>
              <input v-model="form.icon" data-testid="package-icon" class="text-input" maxlength="16" placeholder="★" />
            </label>
            <label>
              <span class="field-label">主题色</span>
              <input v-model="form.theme" data-testid="package-theme" class="text-input" maxlength="32" placeholder="blue / teal / violet" />
            </label>
          </div>
          <div class="package-theme-picker" aria-label="主题色预设">
            <button
              v-for="theme in themePresets"
              :key="theme"
              class="package-theme-option"
              :class="[`package-theme-${theme}`, { active: form.theme === theme }]"
              :data-testid="`package-theme-option-${theme}`"
              type="button"
              @click="form.theme = theme"
            >
              <span></span>
              {{ theme }}
            </button>
          </div>

          <div class="package-form-row">
            <label>
              <span class="field-label">前台角标</span>
              <input v-model="form.badge" data-testid="package-badge" class="text-input" maxlength="32" placeholder="推荐 / 高性价比" />
            </label>
            <label>
              <span class="field-label">排序</span>
              <input v-model.number="form.sortOrder" data-testid="package-sort-order" class="text-input" type="number" />
            </label>
          </div>

          <label class="package-toggle-row">
            <input v-model="form.recommended" data-testid="package-recommended" type="checkbox" />
            <span>前台推荐高亮</span>
          </label>

          <label class="field-label" for="packageFeatures">套餐卡片权益（一行一个）</label>
          <textarea id="packageFeatures" v-model="form.featuresText" data-testid="package-features" class="text-input admin-textarea" rows="4" placeholder="支持图片生成&#10;商用授权&#10;加急排队" />

          <label class="field-label" for="packageBenefits">权益对比（权益名称|展示值）</label>
          <textarea id="packageBenefits" v-model="form.benefitsText" data-testid="package-benefits" class="text-input admin-textarea" rows="5" placeholder="点数|88 点&#10;商用授权|支持&#10;团队席位|3 人" />

          <label class="field-label" for="packageWechatVirtualProductId">微信虚拟支付道具 ID</label>
          <input id="packageWechatVirtualProductId" v-model="form.wechatVirtualProductId" data-testid="package-wechat-virtual-product-id" class="text-input" maxlength="128" placeholder="需与小程序虚拟支付后台道具 ID 完全一致" />

          <div class="field-label-row">
            <label class="field-label" for="packageDescription">描述</label>
            <span>{{ descriptionCount }}/120</span>
          </div>
          <textarea id="packageDescription" v-model="form.description" data-testid="package-description" class="text-input admin-textarea" rows="4" maxlength="120" />

          <label class="package-toggle-row">
            <input v-model="form.isActive" data-testid="package-active" type="checkbox" />
            <span>启用套餐</span>
          </label>

          <div class="package-preview">
            <span data-testid="package-preview-icon" class="package-icon-chip" :class="`package-theme-${form.theme || 'blue'}`">{{ form.icon || form.name.slice(0, 1) || '套' }}</span>
            <div>
              <strong>{{ form.name || '未命名套餐' }}</strong>
              <small>{{ form.audience || '全部用户' }} · {{ form.validDays || 0 }} 天</small>
              <p>{{ form.description || '套餐描述会显示在这里' }}</p>
              <b>{{ formatYuan(priceYuanToCents(form.priceYuan)) }} / {{ formatNumber(form.credits) }} 点</b>
            </div>
          </div>

          <button class="primary-button" type="submit" :disabled="saving">{{ submitText }}</button>
          <button
            v-if="mode === 'edit' && selectedPackage"
            class="secondary-button destructive-button"
            type="button"
            @click="deletePackage(selectedPackage)"
          >
            <Trash2 :size="15" />
            删除套餐
          </button>
        </form>
      </div>
    </div>

    <p v-if="successMessage" class="status-success">{{ successMessage }}</p>
    <p v-if="errorMessage" class="status-error">{{ errorMessage }}</p>
  </section>
</template>


<style scoped>
.packages-heading > div span {
  display: block;
  margin-top: 6px;
  color: #6b7280;
  font-size: 0.92rem;
}

.packages-heading-actions,
.package-kpi-topline,
.package-name-cell,
.field-label-row,
.package-toggle-row {
  display: flex;
  align-items: center;
}

.packages-heading-actions {
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 10px;
}

.packages-kpi-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(170px, 1fr));
  gap: 12px;
}

.package-kpi-card {
  display: grid;
  gap: 12px;
  min-height: 126px;
  padding: 16px;
  border: 1px solid rgba(112, 126, 168, 0.16);
  border-radius: 16px;
  background: rgba(255, 255, 255, 0.78);
  box-shadow: 0 16px 34px rgba(82, 92, 126, 0.08);
  overflow: hidden;
}

.package-kpi-topline {
  justify-content: space-between;
  gap: 10px;
}

.package-kpi-topline span {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  color: #68758b;
  font-size: 0.84rem;
  font-weight: 800;
}

.package-kpi-card strong {
  color: #111827;
  font-size: 1.75rem;
  line-height: 1;
}

.admin-packages-workspace {
  display: block;
}

.packages-table-panel,
.packages-editor-stack {
  min-width: 0;
}

.packages-table-scroll {
  overflow-x: auto;
}

.packages-data-table {
  min-width: 880px;
}

.packages-data-table:has(.package-select-column) {
  min-width: 940px;
}

.package-select-column {
  width: 48px;
  text-align: center;
}

.package-select-column input {
  width: 18px;
  height: 18px;
  accent-color: #2563eb;
}

.packages-bulk-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin: 14px 0;
  padding: 10px 12px;
  border: 1px solid rgba(37, 99, 235, 0.16);
  border-radius: 14px;
  background: rgba(239, 246, 255, 0.72);
}

.packages-bulk-toolbar > span {
  color: #1f3a8a;
  font-size: 0.88rem;
  font-weight: 850;
}

.packages-data-table tbody tr.selected {
  background: rgba(37, 99, 235, 0.07);
}

.packages-count {
  flex: 0 0 auto;
  color: #667085;
  font-size: 0.86rem;
  font-weight: 800;
}

.package-name-cell {
  min-width: 0;
  gap: 10px;
}

.package-name-cell div {
  min-width: 0;
}

.package-name-cell strong {
  display: block;
  max-width: 180px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.package-icon-chip {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  width: 36px;
  height: 36px;
  border-radius: 12px;
  background: linear-gradient(135deg, #2563eb 0%, #14b8a6 100%);
  color: #fff;
  font-size: 0.86rem;
  font-weight: 900;
  box-shadow: 0 12px 22px rgba(37, 99, 235, 0.18);
}

.package-theme-blue {
  background: linear-gradient(135deg, #2563eb 0%, #14b8a6 100%);
}

.package-theme-green {
  background: linear-gradient(135deg, #059669 0%, #22c55e 100%);
}

.package-theme-orange {
  background: linear-gradient(135deg, #d97706 0%, #f59e0b 100%);
}

.package-theme-violet {
  background: linear-gradient(135deg, #7c3aed 0%, #2563eb 100%);
}

.package-theme-rose {
  background: linear-gradient(135deg, #e11d48 0%, #fb7185 100%);
}

.package-theme-gold {
  background: linear-gradient(135deg, #b7791f 0%, #facc15 100%);
}

.package-theme-picker {
  display: grid;
  grid-template-columns: repeat(6, minmax(0, 1fr));
  gap: 8px;
}

.package-theme-option {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-width: 0;
  min-height: 34px;
  border: 1px solid rgba(118, 129, 166, 0.16);
  border-radius: 10px;
  background: rgba(248, 250, 253, 0.86);
  color: #344054;
  font-size: 0.72rem;
  font-weight: 850;
  cursor: pointer;
}

.package-theme-option span {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  background: currentColor;
}

.package-theme-option.active {
  border-color: rgba(37, 99, 235, 0.42);
  box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.1);
}
.package-editor-panel {
  display: grid;
  gap: 14px;
  align-content: start;
}

.package-editor-panel .field-label,
.package-editor-panel .field-label-row {
  min-height: 24px;
  margin-top: 2px;
}

.package-editor-panel .text-input {
  width: 100%;
  box-sizing: border-box;
}

.package-editor-panel textarea.text-input {
  min-height: 132px;
  padding-top: 14px;
  padding-bottom: 14px;
  line-height: 1.5;
  resize: vertical;
  overflow: auto;
}

.package-editor-modal-backdrop {
  position: fixed;
  inset: 0;
  z-index: 1100;
  display: flex;
  justify-content: flex-end;
  padding: 18px;
  background: rgba(15, 23, 42, 0.36);
  backdrop-filter: blur(8px);
}

.package-editor-modal {
  width: min(560px, 100%);
  max-height: calc(100vh - 36px);
  overflow-y: auto;
  align-self: stretch;
  border-radius: 20px;
  box-shadow: 0 24px 80px rgba(15, 23, 42, 0.28);
}

.package-form-row {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.package-form-row label {
  display: grid;
  gap: 7px;
  min-width: 0;
}

.field-label-row {
  justify-content: space-between;
  gap: 12px;
}

.field-label-row span {
  color: #8a94a6;
  font-size: 0.78rem;
  font-weight: 800;
}

.package-toggle-row {
  justify-content: space-between;
  min-height: 44px;
  padding: 10px 12px;
  border: 1px solid rgba(118, 129, 166, 0.14);
  border-radius: 14px;
  background: rgba(247, 250, 253, 0.86);
  color: #25314f;
  font-size: 0.9rem;
  font-weight: 850;
  cursor: pointer;
}

.package-toggle-row input {
  width: 18px;
  height: 18px;
  accent-color: #2563eb;
}

.package-preview {
  display: grid;
  grid-template-columns: 42px minmax(0, 1fr);
  gap: 12px;
  padding: 13px;
  border: 1px solid rgba(118, 129, 166, 0.14);
  border-radius: 16px;
  background: rgba(248, 250, 253, 0.9);
}

.package-preview div {
  display: grid;
  min-width: 0;
  gap: 3px;
}

.package-preview strong,
.package-preview small,
.package-preview p,
.package-preview b {
  overflow: hidden;
  text-overflow: ellipsis;
}

.package-preview strong {
  color: #111827;
  white-space: nowrap;
}

.package-preview small,
.package-preview p {
  margin: 0;
  color: #7a8497;
  font-size: 0.8rem;
}

.package-preview p {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

.package-preview b {
  color: #1f3a8a;
  font-size: 0.9rem;
  white-space: nowrap;
}

@media (max-width: 1180px) {
  .packages-kpi-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 720px) {
  .packages-heading,
  .packages-heading-actions {
    align-items: flex-start;
  }

  .packages-heading-actions,
  .packages-kpi-grid,
  .package-form-row {
    grid-template-columns: 1fr;
    width: 100%;
  }

  .packages-heading-actions {
    display: grid;
  }

  .package-kpi-card {
    min-height: 112px;
  }

  .packages-bulk-toolbar {
    align-items: stretch;
    flex-direction: column;
  }

  .packages-bulk-toolbar .inline-actions {
    display: grid;
    grid-template-columns: 1fr;
  }

  .package-editor-modal-backdrop {
    padding: 10px;
  }

  .package-editor-modal {
    width: 100%;
    max-height: calc(100vh - 20px);
  }

  .package-editor-panel {
    gap: 16px;
  }

  .package-editor-panel textarea.text-input {
    min-height: 146px;
  }
}
</style>
