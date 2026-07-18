<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import {
  CheckCircle2,
  Image,
  Pencil,
  Plus,
  RefreshCw,
  Save,
  Search,
  Sparkles,
  Trash2,
  X
} from 'lucide-vue-next'

import { api } from '../api/client.js'
import ClickSelect from '../components/ClickSelect.vue'

const templates = ref([])
const loading = ref(false)
const saving = ref(false)
const generatingId = ref(null)
const generatingMissing = ref(false)
const message = ref('')
const errorMessage = ref('')
const dialogOpen = ref(false)
const editingTemplateId = ref(null)
const titleInput = ref(null)
let previewPollTimer = null
let previewPollAttempts = 0

const filters = reactive({
  q: '',
  active: '',
  preview: ''
})
const pagination = reactive({
  page: 1,
  page_size: 12,
  total: 0
})
const form = reactive({
  slug: '',
  title: '',
  category: '',
  description: '',
  prompt: '',
  aspect_ratio: '1:1',
  style_preset: '',
  theme: '',
  workspace_section: '',
  workspace_tool_mode: 'generate',
  workspace_sort: 0,
  sort_order: 0,
  is_active: true
})
const activeFilterOptions = [
  { value: '', label: '全部状态' },
  { value: 'true', label: '启用' },
  { value: 'false', label: '停用' }
]
const previewFilterOptions = [
  { value: '', label: '全部预览' },
  { value: 'generated', label: '已生成' },
  { value: 'missing', label: '缺失预览' }
]
const workspaceSectionOptions = [
  { value: '', label: '不展示' },
  { value: 'hot', label: '热门' },
  { value: 'inspiration', label: '灵感' }
]
const workspaceToolModeOptions = [
  { value: 'generate', label: '文本生成' },
  { value: 'expand', label: '智能扩图' },
  { value: 'erase', label: '移除物体' },
  { value: 'upscale', label: '高清放大' },
  { value: 'remove_background', label: '移除背景' }
]

const totalPages = computed(() => Math.max(1, Math.ceil(pagination.total / pagination.page_size)))
const generatedCount = computed(() => templates.value.filter((item) => item.preview_url).length)
const missingCount = computed(() => templates.value.filter((item) => !item.preview_url).length)
const editingTemplate = computed(() => templates.value.find((item) => item.id === editingTemplateId.value) ?? null)
const dialogTitle = computed(() => (editingTemplateId.value ? '编辑提示词模板' : '新增提示词模板'))
const previewMetaRows = computed(() => [
  { label: '模板标识', value: form.slug.trim() || '-' },
  { label: '分类', value: form.category.trim() || '-' },
  { label: '图片比例', value: form.aspect_ratio.trim() || '1:1' },
  { label: '风格', value: form.style_preset.trim() || '无风格' },
  { label: '主题', value: form.theme.trim() || '-' },
  { label: '工作台', value: workspaceSectionLabel(form.workspace_section) },
  { label: '工具模式', value: form.workspace_tool_mode.trim() || 'generate' },
  { label: '排序', value: `${Number(form.sort_order || 0)}` },
  { label: '状态', value: form.is_active ? '启用' : '停用' }
])
const hasPreviewTasks = computed(() =>
  templates.value.some((item) => ['queued', 'running'].includes(String(item.preview_status || '').trim()))
)

function listParams() {
  return {
    ...(filters.q ? { q: filters.q } : {}),
    ...(filters.active ? { active: filters.active } : {}),
    ...(filters.preview ? { preview: filters.preview } : {}),
    page: pagination.page,
    page_size: pagination.page_size
  }
}

async function loadTemplates(options = {}) {
  if (!options.silent) loading.value = true
  errorMessage.value = ''
  try {
    const payload = await api.listAdminPromptTemplates(listParams())
    templates.value = payload.items ?? []
    pagination.total = payload.total ?? 0
    pagination.page = payload.page ?? pagination.page
    pagination.page_size = payload.page_size ?? pagination.page_size
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    if (!options.silent) loading.value = false
  }
}

function applyFilters() {
  pagination.page = 1
  loadTemplates()
}

function resetFilters() {
  filters.q = ''
  filters.active = ''
  filters.preview = ''
  pagination.page = 1
  loadTemplates()
}

function resetForm(template = null) {
  Object.assign(form, {
    slug: template?.slug ?? '',
    title: template?.title ?? '',
    category: template?.category ?? '',
    description: template?.description ?? '',
    prompt: template?.prompt ?? '',
    aspect_ratio: template?.aspect_ratio ?? '1:1',
    style_preset: template?.style_preset ?? '',
    theme: template?.theme ?? '',
    workspace_section: template?.workspace_section ?? '',
    workspace_tool_mode: template?.workspace_tool_mode ?? 'generate',
    workspace_sort: Number(template?.workspace_sort ?? 0),
    sort_order: Number(template?.sort_order ?? 0),
    is_active: template?.is_active ?? true
  })
}

function openCreateTemplate() {
  editingTemplateId.value = null
  resetForm()
  dialogOpen.value = true
  focusTitleInput()
}

function openEditTemplate(template) {
  editingTemplateId.value = template.id
  resetForm(template)
  dialogOpen.value = true
  focusTitleInput()
}

function closeDialog() {
  if (saving.value) return
  dialogOpen.value = false
  editingTemplateId.value = null
}

function templatePayload() {
  return {
    slug: form.slug.trim(),
    title: form.title.trim(),
    category: form.category.trim(),
    description: form.description.trim(),
    prompt: form.prompt.trim(),
    aspect_ratio: form.aspect_ratio.trim() || '1:1',
    style_preset: form.style_preset.trim(),
    theme: form.theme.trim(),
    workspace_section: form.workspace_section.trim(),
    workspace_tool_mode: form.workspace_tool_mode.trim() || 'generate',
    workspace_sort: Number(form.workspace_sort || 0),
    sort_order: Number(form.sort_order || 0),
    is_active: Boolean(form.is_active)
  }
}

function workspaceSectionLabel(section) {
  if (section === 'hot') return '热门'
  if (section === 'inspiration') return '灵感'
  return '不展示'
}

function focusTitleInput() {
  nextTick(() => {
    titleInput.value?.focus({ preventScroll: true })
  })
}

async function saveTemplate(options = {}) {
  const generatePreviewAfterSave = Boolean(options.generatePreview)
  saving.value = true
  message.value = ''
  errorMessage.value = ''
  let shouldReload = false
  try {
    const payload = templatePayload()
    let savedTemplate
    if (editingTemplateId.value) {
      savedTemplate = await api.updateAdminPromptTemplate(editingTemplateId.value, payload)
      message.value = '模板已更新'
    } else {
      savedTemplate = await api.createAdminPromptTemplate(payload)
      message.value = '模板已新增'
    }
    shouldReload = true
    dialogOpen.value = false
    editingTemplateId.value = null
    if (generatePreviewAfterSave && savedTemplate?.id) {
      const report = await api.generateAdminPromptTemplatePreview(savedTemplate.id, { force: true })
      message.value = generationQueuedMessage(report)
      startPreviewPolling()
    }
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    saving.value = false
    if (shouldReload) await loadTemplates()
  }
}

async function deleteTemplate(template) {
  if (typeof window !== 'undefined' && window.confirm && !window.confirm(`删除模板「${template.title}」？`)) {
    return
  }
  message.value = ''
  errorMessage.value = ''
  try {
    await api.deleteAdminPromptTemplate(template.id)
    message.value = '模板已删除'
    await loadTemplates()
  } catch (error) {
    errorMessage.value = error.message
  }
}

async function generatePreview(template) {
  generatingId.value = template.id
  message.value = ''
  errorMessage.value = ''
  try {
    const report = await api.generateAdminPromptTemplatePreview(template.id, { force: true })
    message.value = generationQueuedMessage(report)
    await loadTemplates()
    startPreviewPolling()
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    generatingId.value = null
  }
}

async function generateMissingPreviews() {
  generatingMissing.value = true
  message.value = ''
  errorMessage.value = ''
  try {
    const report = await api.batchGenerateAdminPromptTemplatePreviews({ limit: 12 })
    message.value = generationQueuedMessage(report)
    await loadTemplates()
    startPreviewPolling()
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    generatingMissing.value = false
  }
}

function generationQueuedMessage(report = {}) {
  const queued = Number(report.queued || 0)
  if (queued <= 0) return '没有需要生成的预览'
  return `已开始生成 ${queued} 张预览`
}

function previewStatus(template) {
  const status = String(template.preview_status || '').trim()
  if (status === 'queued' || status === 'running') return '生成中'
  if (status === 'failed') return '生成失败'
  if (template.preview_url || status === 'generated') return '已生成'
  return '缺失预览'
}

function previewStatusClass(template) {
  const status = String(template.preview_status || '').trim()
  if (status === 'queued' || status === 'running') return 'pending'
  if (status === 'failed') return 'danger'
  if (template.preview_url || status === 'generated') return 'online'
  return 'offline'
}

function stopPreviewPolling() {
  if (previewPollTimer) {
    clearInterval(previewPollTimer)
    previewPollTimer = null
  }
}

function startPreviewPolling() {
  if (previewPollTimer) return
  previewPollAttempts = 0
  previewPollTimer = setInterval(async () => {
    previewPollAttempts += 1
    await loadTemplates({ silent: true })
    if (!hasPreviewTasks.value || previewPollAttempts >= 80) {
      stopPreviewPolling()
    }
  }, 3000)
}

function nextPage() {
  if (pagination.page >= totalPages.value) return
  pagination.page += 1
  loadTemplates()
}

function previousPage() {
  if (pagination.page <= 1) return
  pagination.page -= 1
  loadTemplates()
}

onMounted(async () => {
  await loadTemplates()
  if (hasPreviewTasks.value) startPreviewPolling()
})

onBeforeUnmount(stopPreviewPolling)
</script>

<template>
  <section class="prompt-template-admin-page">
    <div class="admin-page-heading">
      <div>
        <p class="eyebrow">管理用户、套餐、模型与生成业务 / 提示词模板</p>
        <h1>提示词模板</h1>
      </div>
      <button class="primary-button" type="button" data-testid="new-template" @click="openCreateTemplate">
        <Plus :size="16" />
        新增模板
      </button>
    </div>

    <div class="template-admin-stats">
      <article class="admin-panel template-stat-card">
        <Image :size="18" />
        <span>当前页已生成</span>
        <strong>{{ generatedCount }}</strong>
      </article>
      <article class="admin-panel template-stat-card">
        <Sparkles :size="18" />
        <span>当前页缺失</span>
        <strong>{{ missingCount }}</strong>
      </article>
      <button class="primary-button" type="button" data-testid="generate-missing-previews" :disabled="generatingMissing" @click="generateMissingPreviews">
        <RefreshCw :size="16" />
        {{ generatingMissing ? '生成中...' : '生成缺失预览' }}
      </button>
    </div>

    <div v-if="message" class="settings-alert success">
      <CheckCircle2 :size="16" />
      <span>{{ message }}</span>
    </div>
    <div v-if="errorMessage" class="settings-alert error">
      <X :size="16" />
      <span>{{ errorMessage }}</span>
    </div>

    <section class="admin-panel">
      <div class="admin-filter-bar">
        <label class="admin-search-field">
          <Search :size="16" />
          <input v-model.trim="filters.q" type="search" placeholder="搜索标题、标识或分类" @keyup.enter="applyFilters" />
        </label>
        <ClickSelect v-model="filters.active" :options="activeFilterOptions" class="compact-input" aria-label="启用状态" compact />
        <ClickSelect v-model="filters.preview" :options="previewFilterOptions" class="compact-input" aria-label="预览状态" compact />
        <button class="mini-button compact-button" type="button" @click="applyFilters">筛选</button>
        <button class="mini-button compact-button" type="button" @click="resetFilters">重置</button>
      </div>

      <div class="template-admin-list">
        <div v-if="loading" class="admin-empty-state">加载中...</div>
        <div v-else-if="templates.length === 0" class="admin-empty-state">暂无模板</div>
        <article v-for="template in templates" v-else :key="template.id" class="template-admin-card">
          <img v-if="template.preview_url" :src="template.preview_url" :alt="template.title" :data-testid="`template-preview-${template.id}`" />
          <div v-else class="template-admin-placeholder">
            <Image :size="28" />
          </div>
          <div class="template-admin-main">
            <div class="template-admin-title">
              <div>
                <h2>{{ template.title }}</h2>
                <p>{{ template.slug }} · {{ template.category || '未分类' }}</p>
              </div>
              <span :class="['status-pill', previewStatusClass(template)]">{{ previewStatus(template) }}</span>
            </div>
            <p class="template-admin-desc">{{ template.description }}</p>
            <p v-if="template.preview_status === 'failed' && template.preview_error_message" class="template-preview-error">
              {{ template.preview_error_message }}
            </p>
            <p class="template-admin-prompt">{{ template.prompt }}</p>
            <div class="template-admin-meta">
              <span>{{ template.aspect_ratio || '1:1' }}</span>
              <span>{{ template.style_preset || '无风格' }}</span>
              <span>工作台 {{ workspaceSectionLabel(template.workspace_section) }}</span>
              <span>{{ template.workspace_tool_mode || 'generate' }}</span>
              <span>{{ template.is_active ? '启用' : '停用' }}</span>
              <span>排序 {{ template.sort_order || 0 }}</span>
            </div>
            <div class="template-admin-actions">
              <button class="mini-button" type="button" :data-testid="`generate-template-preview-${template.id}`" :disabled="Boolean(generatingId)" @click="generatePreview(template)">
                <Sparkles :size="14" />
                {{ generatingId === template.id ? '生成中' : '生成预览' }}
              </button>
              <button class="mini-button" type="button" :data-testid="`edit-template-${template.id}`" @click="openEditTemplate(template)">
                <Pencil :size="14" />
                编辑
              </button>
              <button class="mini-button danger" type="button" :data-testid="`delete-template-${template.id}`" @click="deleteTemplate(template)">
                <Trash2 :size="14" />
                删除
              </button>
            </div>
          </div>
        </article>
      </div>

      <div class="admin-pagination">
        <button class="mini-button" type="button" :disabled="pagination.page <= 1" @click="previousPage">上一页</button>
        <span>第 {{ pagination.page }} / {{ totalPages }} 页 · 共 {{ pagination.total }} 条</span>
        <button class="mini-button" type="button" :disabled="pagination.page >= totalPages" @click="nextPage">下一页</button>
      </div>
    </section>

    <div v-if="dialogOpen" class="admin-modal-backdrop prompt-template-modal-backdrop">
      <form class="admin-modal prompt-template-modal" data-testid="prompt-template-modal" @submit.prevent="saveTemplate">
        <div class="prompt-template-modal-head">
          <div>
            <h2>{{ dialogTitle }}</h2>
            <p>填写模板信息，保存后可立即生成预览图。</p>
          </div>
          <button class="icon-button" type="button" aria-label="关闭弹窗" @click="closeDialog">
            <X :size="16" />
          </button>
        </div>

        <div class="prompt-template-modal-body">
          <aside class="prompt-template-preview-panel">
            <div class="prompt-template-panel-title">
              <strong>预览 / 规格</strong>
              <span>{{ form.aspect_ratio || '1:1' }}</span>
            </div>
            <div class="prompt-template-preview-frame">
              <img v-if="editingTemplate?.preview_url" :src="editingTemplate.preview_url" :alt="form.title || '模板预览图'" />
              <div v-else class="prompt-template-preview-empty">
                <Image :size="30" />
                <strong>暂无预览图</strong>
                <span>保存并生成预览后会显示在这里</span>
              </div>
            </div>
            <dl class="prompt-template-spec-list">
              <div v-for="row in previewMetaRows" :key="row.label">
                <dt>{{ row.label }}</dt>
                <dd>{{ row.value }}</dd>
              </div>
            </dl>
          </aside>

          <div class="prompt-template-form-panel">
            <div class="template-form-grid">
              <label>
                <span>模板标题 <b>*</b></span>
                <input ref="titleInput" v-model="form.title" data-testid="template-title" placeholder="例如：城市失眠指数海报" required />
              </label>
              <label>
                <span>模板标识 <b>*</b></span>
                <input v-model="form.slug" data-testid="template-slug" placeholder="city-sleepless-index" required />
              </label>
              <label>
                <span>分类</span>
                <input v-model="form.category" data-testid="template-category" placeholder="数据海报" />
              </label>
              <label>
                <span>图片比例</span>
                <input v-model="form.aspect_ratio" data-testid="template-aspect-ratio" placeholder="1:1 / 4:3 / 16:9" />
              </label>
              <label>
                <span>风格</span>
                <input v-model="form.style_preset" data-testid="template-style-preset" placeholder="海报 / 写实 / 插画" />
              </label>
              <label>
                <span>主题</span>
                <input v-model="form.theme" data-testid="template-theme" placeholder="city-data" />
              </label>
              <label>
                <span>工作台分区</span>
                <ClickSelect v-model="form.workspace_section" :options="workspaceSectionOptions" data-testid="template-workspace-section" aria-label="工作台分区" />
              </label>
              <label>
                <span>工作台工具</span>
                <ClickSelect v-model="form.workspace_tool_mode" :options="workspaceToolModeOptions" data-testid="template-workspace-tool-mode" aria-label="工作台工具" />
              </label>
              <label>
                <span>工作台排序</span>
                <input v-model.number="form.workspace_sort" data-testid="template-workspace-sort" type="number" />
              </label>
              <label>
                <span>排序</span>
                <input v-model.number="form.sort_order" data-testid="template-sort-order" type="number" />
              </label>
              <label class="template-toggle">
                <input v-model="form.is_active" data-testid="template-is-active" type="checkbox" />
                <span>启用模板</span>
              </label>
            </div>
            <label class="template-wide-field">
              <span>描述</span>
              <textarea v-model="form.description" data-testid="template-description" rows="2" placeholder="一句话说明用户会得到什么结果"></textarea>
            </label>
            <label class="template-wide-field">
              <span>提示词 <b>*</b></span>
              <textarea v-model="form.prompt" data-testid="template-prompt" rows="7" placeholder="输入完整提示词，支持变量占位" required></textarea>
            </label>
          </div>
        </div>

        <div class="prompt-template-modal-actions">
          <button class="mini-button" type="button" @click="closeDialog">取消</button>
          <button class="secondary-button compact-button" type="button" data-testid="save-template-and-generate-preview" :disabled="saving" @click="saveTemplate({ generatePreview: true })">
            <Sparkles :size="16" />
            {{ saving ? '保存中...' : '保存并生成预览' }}
          </button>
          <button class="primary-button" type="button" data-testid="save-template" :disabled="saving" @click="saveTemplate()">
            <Save :size="16" />
            {{ saving ? '保存中...' : '保存模板' }}
          </button>
        </div>
      </form>
    </div>
  </section>
</template>
