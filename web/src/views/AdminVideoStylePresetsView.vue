<script setup>
import { computed, nextTick, onMounted, reactive, ref } from 'vue'
import { CheckCircle2, Image, Pencil, Plus, Search, Trash2, Video, X } from 'lucide-vue-next'

import { api } from '../api/client.js'

const presets = ref([])
const loading = ref(false)
const saving = ref(false)
const message = ref('')
const errorMessage = ref('')
const dialogOpen = ref(false)
const editingPresetId = ref(null)
const titleInput = ref(null)

const filters = reactive({
  q: '',
  active: ''
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
  tags: '',
  preview_asset_key: '',
  preview_url: '',
  style_prompt: '',
  sort_order: 0,
  is_active: true
})

const totalPages = computed(() => Math.max(1, Math.ceil(pagination.total / pagination.page_size)))
const activeCount = computed(() => presets.value.filter((item) => item.is_active).length)
const dialogTitle = computed(() => (editingPresetId.value ? '编辑视频风格' : '新增视频风格'))

function listParams() {
  return {
    ...(filters.q ? { q: filters.q } : {}),
    ...(filters.active ? { active: filters.active } : {}),
    page: pagination.page,
    page_size: pagination.page_size
  }
}

async function loadPresets() {
  loading.value = true
  errorMessage.value = ''
  try {
    const payload = await api.listAdminVideoStylePresets(listParams())
    presets.value = payload.items ?? []
    pagination.total = payload.total ?? 0
    pagination.page = payload.page ?? pagination.page
    pagination.page_size = payload.page_size ?? pagination.page_size
  } catch (error) {
    errorMessage.value = error.message || '视频风格预设读取失败'
  } finally {
    loading.value = false
  }
}

function applyFilters() {
  pagination.page = 1
  loadPresets()
}

function resetFilters() {
  filters.q = ''
  filters.active = ''
  pagination.page = 1
  loadPresets()
}

function formatTags(tags) {
  return Array.isArray(tags) ? tags.join(', ') : ''
}

function parseTags(value) {
  return String(value || '')
    .split(/[,，\n]/)
    .map((item) => item.trim())
    .filter(Boolean)
}

function resetForm(preset = null) {
  Object.assign(form, {
    slug: preset?.slug ?? '',
    title: preset?.title ?? '',
    category: preset?.category ?? '',
    description: preset?.description ?? '',
    tags: formatTags(preset?.tags),
    preview_asset_key: preset?.preview_asset_key ?? '',
    preview_url: preset?.preview_url ?? '',
    style_prompt: preset?.style_prompt ?? '',
    sort_order: Number(preset?.sort_order ?? 0),
    is_active: preset?.is_active ?? true
  })
}

function focusTitleInput() {
  nextTick(() => {
    titleInput.value?.focus({ preventScroll: true })
  })
}

function openCreatePreset() {
  editingPresetId.value = null
  resetForm()
  dialogOpen.value = true
  focusTitleInput()
}

function openEditPreset(preset) {
  editingPresetId.value = preset.id
  resetForm(preset)
  dialogOpen.value = true
  focusTitleInput()
}

function closeDialog() {
  if (saving.value) return
  dialogOpen.value = false
  editingPresetId.value = null
}

function presetPayload() {
  return {
    slug: form.slug.trim(),
    title: form.title.trim(),
    category: form.category.trim(),
    description: form.description.trim(),
    tags: parseTags(form.tags),
    preview_asset_key: form.preview_asset_key.trim(),
    preview_url: form.preview_url.trim(),
    style_prompt: form.style_prompt.trim(),
    sort_order: Number(form.sort_order || 0),
    is_active: Boolean(form.is_active)
  }
}

async function savePreset() {
  saving.value = true
  message.value = ''
  errorMessage.value = ''
  let shouldReload = false
  try {
    const payload = presetPayload()
    if (editingPresetId.value) {
      await api.updateAdminVideoStylePreset(editingPresetId.value, payload)
      message.value = '视频风格预设已更新'
    } else {
      await api.createAdminVideoStylePreset(payload)
      message.value = '视频风格预设已新增'
    }
    shouldReload = true
    closeDialog()
  } catch (error) {
    errorMessage.value = error.message || '视频风格预设保存失败'
  } finally {
    saving.value = false
    if (shouldReload) await loadPresets()
  }
}

async function deletePreset(preset) {
  if (typeof window !== 'undefined' && window.confirm && !window.confirm(`删除视频风格预设「${preset.title}」？`)) {
    return
  }
  message.value = ''
  errorMessage.value = ''
  try {
    await api.deleteAdminVideoStylePreset(preset.id)
    message.value = '视频风格预设已删除'
    await loadPresets()
  } catch (error) {
    errorMessage.value = error.message || '视频风格预设删除失败'
  }
}

function nextPage() {
  if (pagination.page >= totalPages.value) return
  pagination.page += 1
  loadPresets()
}

function previousPage() {
  if (pagination.page <= 1) return
  pagination.page -= 1
  loadPresets()
}

onMounted(loadPresets)
</script>

<template>
  <section class="prompt-template-admin-page">
    <div class="admin-page-heading">
      <div>
        <p class="eyebrow">运营配置 / 视频风格预设</p>
        <h1>视频风格预设</h1>
      </div>
      <button class="primary-button" type="button" data-testid="new-video-style-preset" @click="openCreatePreset">
        <Plus :size="16" />
        新增风格
      </button>
    </div>

    <div class="template-admin-stats">
      <article class="admin-panel template-stat-card">
        <Video :size="18" />
        <span>当前页启用</span>
        <strong>{{ activeCount }}</strong>
      </article>
      <article class="admin-panel template-stat-card">
        <Image :size="18" />
        <span>当前页预设</span>
        <strong>{{ presets.length }}</strong>
      </article>
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
          <input v-model.trim="filters.q" type="search" placeholder="搜索标题、标识、分类或提示词" @keyup.enter="applyFilters" />
        </label>
        <select v-model="filters.active" class="compact-input" aria-label="启用状态">
          <option value="">全部状态</option>
          <option value="true">启用</option>
          <option value="false">停用</option>
        </select>
        <button class="mini-button compact-button" type="button" @click="applyFilters">筛选</button>
        <button class="mini-button compact-button" type="button" @click="resetFilters">重置</button>
      </div>

      <div class="template-admin-list">
        <div v-if="loading" class="admin-empty-state">加载中...</div>
        <div v-else-if="presets.length === 0" class="admin-empty-state">暂无视频风格预设</div>
        <article
          v-for="preset in presets"
          v-else
          :key="preset.id"
          class="template-admin-card"
          :data-testid="`video-style-preset-row-${preset.id}`"
        >
          <img v-if="preset.preview_url" :src="preset.preview_url" :alt="preset.title" />
          <div v-else class="template-admin-placeholder">
            <Image :size="28" />
          </div>
          <div class="template-admin-main">
            <div class="template-admin-title">
              <div>
                <h2>{{ preset.title }}</h2>
                <p>{{ preset.slug }} · {{ preset.category || '未分类' }}</p>
              </div>
              <span :class="['status-pill', preset.is_active ? 'online' : 'offline']">
                {{ preset.is_active ? '启用' : '停用' }}
              </span>
            </div>
            <p class="template-admin-desc">{{ preset.description }}</p>
            <p class="template-admin-prompt">{{ preset.style_prompt }}</p>
            <div class="template-admin-meta">
              <span v-for="tag in preset.tags || []" :key="tag">{{ tag }}</span>
              <span>套用 {{ preset.use_count || 0 }}</span>
              <span>排序 {{ preset.sort_order || 0 }}</span>
            </div>
            <div class="template-admin-actions">
              <button class="mini-button" type="button" :data-testid="`edit-video-style-preset-${preset.id}`" @click="openEditPreset(preset)">
                <Pencil :size="14" />
                编辑
              </button>
              <button class="mini-button danger" type="button" :data-testid="`delete-video-style-preset-${preset.id}`" @click="deletePreset(preset)">
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
      <form class="admin-modal prompt-template-modal" data-testid="video-style-preset-modal" @submit.prevent="savePreset">
        <div class="prompt-template-modal-head">
          <div>
            <h2>{{ dialogTitle }}</h2>
            <p>维护视频生成的官方视觉风格、预览图、排序和上线状态。</p>
          </div>
          <button class="icon-button" type="button" aria-label="关闭弹窗" @click="closeDialog">
            <X :size="16" />
          </button>
        </div>

        <div class="prompt-template-modal-body">
          <aside class="prompt-template-preview-panel">
            <div class="prompt-template-panel-title">
              <strong>预览 / 风格</strong>
              <span>{{ form.category || '未分类' }}</span>
            </div>
            <div class="prompt-template-preview-frame">
              <img v-if="form.preview_url" :src="form.preview_url" :alt="form.title || '视频风格预览图'" />
              <div v-else class="prompt-template-preview-empty">
                <Image :size="30" />
                <strong>暂无预览图</strong>
                <span>填写 OSS 或公网图片 URL 后展示</span>
              </div>
            </div>
            <dl class="prompt-template-spec-list">
              <div>
                <dt>标识</dt>
                <dd>{{ form.slug || '-' }}</dd>
              </div>
              <div>
                <dt>标签</dt>
                <dd>{{ form.tags || '-' }}</dd>
              </div>
              <div>
                <dt>状态</dt>
                <dd>{{ form.is_active ? '启用' : '停用' }}</dd>
              </div>
            </dl>
          </aside>

          <div class="prompt-template-form-panel">
            <div class="template-form-grid">
              <label>
                <span>标题 <b>*</b></span>
                <input ref="titleInput" v-model="form.title" data-testid="video-style-preset-title" required />
              </label>
              <label>
                <span>标识 <b>*</b></span>
                <input v-model="form.slug" data-testid="video-style-preset-slug" required />
              </label>
              <label>
                <span>分类</span>
                <input v-model="form.category" data-testid="video-style-preset-category" />
              </label>
              <label>
                <span>标签</span>
                <input v-model="form.tags" data-testid="video-style-preset-tags" placeholder="本周热门, 新手推荐" />
              </label>
              <label>
                <span>预览图 URL <b>*</b></span>
                <input v-model="form.preview_url" data-testid="video-style-preset-preview-url" />
              </label>
              <label>
                <span>OSS key</span>
                <input v-model="form.preview_asset_key" data-testid="video-style-preset-preview-asset-key" />
              </label>
              <label>
                <span>排序</span>
                <input v-model.number="form.sort_order" data-testid="video-style-preset-sort-order" type="number" />
              </label>
              <label class="template-form-checkbox">
                <input v-model="form.is_active" data-testid="video-style-preset-active" type="checkbox" />
                <span>启用预设</span>
              </label>
            </div>

            <label>
              <span>描述</span>
              <textarea v-model="form.description" rows="2" data-testid="video-style-preset-description"></textarea>
            </label>
            <label>
              <span>风格提示词 <b>*</b></span>
              <textarea v-model="form.style_prompt" rows="6" data-testid="video-style-preset-style-prompt" required></textarea>
            </label>
          </div>
        </div>

        <div class="prompt-template-modal-actions">
          <button class="secondary-button" type="button" @click="closeDialog">取消</button>
          <button class="primary-button" type="button" data-testid="save-video-style-preset" :disabled="saving" @click="savePreset">
            {{ saving ? '保存中...' : '保存风格' }}
          </button>
        </div>
      </form>
    </div>
  </section>
</template>
