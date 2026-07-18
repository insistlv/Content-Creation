<script setup>
import { computed, nextTick, onMounted, reactive, ref } from 'vue'
import {
  CheckCircle2,
  Image,
  Pencil,
  Plus,
  Search,
  Sparkles,
  Trash2,
  X
} from 'lucide-vue-next'

import { api } from '../api/client.js'

const recommendations = ref([])
const loading = ref(false)
const saving = ref(false)
const message = ref('')
const errorMessage = ref('')
const dialogOpen = ref(false)
const editingRecommendationId = ref(null)
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
  heat_tags: '',
  preview_asset_key: '',
  preview_url: '',
  prompt: '',
  negative_prompt: '',
  aspect_ratio: '1:1',
  style_preset: '',
  theme: '',
  tool_mode: 'generate',
  model_id: '',
  params: '{}',
  sort_order: 0,
  is_active: true
})

const totalPages = computed(() => Math.max(1, Math.ceil(pagination.total / pagination.page_size)))
const activeCount = computed(() => recommendations.value.filter((item) => item.is_active).length)
const currentRecommendation = computed(() => recommendations.value.find((item) => item.id === editingRecommendationId.value) ?? null)
const dialogTitle = computed(() => (editingRecommendationId.value ? '编辑灵感推荐' : '新增灵感推荐'))

function listParams() {
  return {
    ...(filters.q ? { q: filters.q } : {}),
    ...(filters.active ? { active: filters.active } : {}),
    page: pagination.page,
    page_size: pagination.page_size
  }
}

async function loadRecommendations() {
  loading.value = true
  errorMessage.value = ''
  try {
    const payload = await api.listAdminInspirationRecommendations(listParams())
    recommendations.value = payload.items ?? []
    pagination.total = payload.total ?? 0
    pagination.page = payload.page ?? pagination.page
    pagination.page_size = payload.page_size ?? pagination.page_size
  } catch (error) {
    errorMessage.value = error.message || '灵感推荐读取失败'
  } finally {
    loading.value = false
  }
}

function applyFilters() {
  pagination.page = 1
  loadRecommendations()
}

function resetFilters() {
  filters.q = ''
  filters.active = ''
  pagination.page = 1
  loadRecommendations()
}

function formatHeatTags(tags) {
  return Array.isArray(tags) ? tags.join(', ') : ''
}

function parseHeatTags(value) {
  return String(value || '')
    .split(/[,，\n]/)
    .map((item) => item.trim())
    .filter(Boolean)
}

function normalizedParams() {
  const raw = String(form.params || '').trim()
  if (!raw) return {}
  const parsed = JSON.parse(raw)
  if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') {
    throw new Error('扩展参数必须是 JSON 对象')
  }
  return parsed
}

function resetForm(recommendation = null) {
  Object.assign(form, {
    slug: recommendation?.slug ?? '',
    title: recommendation?.title ?? '',
    category: recommendation?.category ?? '',
    description: recommendation?.description ?? '',
    heat_tags: formatHeatTags(recommendation?.heat_tags),
    preview_asset_key: recommendation?.preview_asset_key ?? '',
    preview_url: recommendation?.preview_url ?? '',
    prompt: recommendation?.prompt ?? '',
    negative_prompt: recommendation?.negative_prompt ?? '',
    aspect_ratio: recommendation?.aspect_ratio ?? '1:1',
    style_preset: recommendation?.style_preset ?? '',
    theme: recommendation?.theme ?? '',
    tool_mode: recommendation?.tool_mode ?? 'generate',
    model_id: recommendation?.model_id ? String(recommendation.model_id) : '',
    params: JSON.stringify(recommendation?.params ?? {}, null, 2),
    sort_order: Number(recommendation?.sort_order ?? 0),
    is_active: recommendation?.is_active ?? true
  })
}

function focusTitleInput() {
  nextTick(() => {
    titleInput.value?.focus({ preventScroll: true })
  })
}

function openCreateRecommendation() {
  editingRecommendationId.value = null
  resetForm()
  dialogOpen.value = true
  focusTitleInput()
}

function openEditRecommendation(recommendation) {
  editingRecommendationId.value = recommendation.id
  resetForm(recommendation)
  dialogOpen.value = true
  focusTitleInput()
}

function closeDialog() {
  if (saving.value) return
  dialogOpen.value = false
  editingRecommendationId.value = null
}

function recommendationPayload() {
  return {
    slug: form.slug.trim(),
    title: form.title.trim(),
    category: form.category.trim(),
    description: form.description.trim(),
    heat_tags: parseHeatTags(form.heat_tags),
    preview_asset_key: form.preview_asset_key.trim(),
    preview_url: form.preview_url.trim(),
    prompt: form.prompt.trim(),
    negative_prompt: form.negative_prompt.trim(),
    aspect_ratio: form.aspect_ratio.trim() || '1:1',
    style_preset: form.style_preset.trim(),
    theme: form.theme.trim(),
    tool_mode: form.tool_mode.trim() || 'generate',
    model_id: Number(form.model_id || 0),
    params: normalizedParams(),
    sort_order: Number(form.sort_order || 0),
    is_active: Boolean(form.is_active)
  }
}

async function saveRecommendation() {
  saving.value = true
  message.value = ''
  errorMessage.value = ''
  let shouldReload = false
  try {
    const payload = recommendationPayload()
    if (editingRecommendationId.value) {
      await api.updateAdminInspirationRecommendation(editingRecommendationId.value, payload)
      message.value = '灵感推荐已更新'
    } else {
      await api.createAdminInspirationRecommendation(payload)
      message.value = '灵感推荐已新增'
    }
    shouldReload = true
    closeDialog()
  } catch (error) {
    errorMessage.value = error.message || '灵感推荐保存失败'
  } finally {
    saving.value = false
    if (shouldReload) await loadRecommendations()
  }
}

async function deleteRecommendation(recommendation) {
  if (typeof window !== 'undefined' && window.confirm && !window.confirm(`删除灵感推荐「${recommendation.title}」？`)) {
    return
  }
  message.value = ''
  errorMessage.value = ''
  try {
    await api.deleteAdminInspirationRecommendation(recommendation.id)
    message.value = '灵感推荐已删除'
    await loadRecommendations()
  } catch (error) {
    errorMessage.value = error.message || '灵感推荐删除失败'
  }
}

function nextPage() {
  if (pagination.page >= totalPages.value) return
  pagination.page += 1
  loadRecommendations()
}

function previousPage() {
  if (pagination.page <= 1) return
  pagination.page -= 1
  loadRecommendations()
}

onMounted(loadRecommendations)
</script>

<template>
  <section class="prompt-template-admin-page">
    <div class="admin-page-heading">
      <div>
        <p class="eyebrow">运营配置 / 灵感推荐</p>
        <h1>灵感推荐</h1>
      </div>
      <button class="primary-button" type="button" data-testid="new-recommendation" @click="openCreateRecommendation">
        <Plus :size="16" />
        新增推荐
      </button>
    </div>

    <div class="template-admin-stats">
      <article class="admin-panel template-stat-card">
        <Sparkles :size="18" />
        <span>当前页启用</span>
        <strong>{{ activeCount }}</strong>
      </article>
      <article class="admin-panel template-stat-card">
        <Image :size="18" />
        <span>当前页推荐</span>
        <strong>{{ recommendations.length }}</strong>
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
          <input v-model.trim="filters.q" type="search" placeholder="搜索标题、标识或分类" @keyup.enter="applyFilters" />
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
        <div v-else-if="recommendations.length === 0" class="admin-empty-state">暂无灵感推荐</div>
        <article
          v-for="recommendation in recommendations"
          v-else
          :key="recommendation.id"
          class="template-admin-card"
          :data-testid="`recommendation-row-${recommendation.id}`"
        >
          <img v-if="recommendation.preview_url" :src="recommendation.preview_url" :alt="recommendation.title" />
          <div v-else class="template-admin-placeholder">
            <Image :size="28" />
          </div>
          <div class="template-admin-main">
            <div class="template-admin-title">
              <div>
                <h2>{{ recommendation.title }}</h2>
                <p>{{ recommendation.slug }} · {{ recommendation.category || '未分类' }}</p>
              </div>
              <span :class="['status-pill', recommendation.is_active ? 'online' : 'offline']">
                {{ recommendation.is_active ? '启用' : '停用' }}
              </span>
            </div>
            <p class="template-admin-desc">{{ recommendation.description }}</p>
            <p class="template-admin-prompt">{{ recommendation.prompt }}</p>
            <div class="template-admin-meta">
              <span>{{ recommendation.aspect_ratio || '1:1' }}</span>
              <span>{{ recommendation.style_preset || '无风格' }}</span>
              <span>{{ recommendation.tool_mode || 'generate' }}</span>
              <span v-for="tag in recommendation.heat_tags || []" :key="tag">{{ tag }}</span>
              <span>套用 {{ recommendation.use_count || 0 }}</span>
              <span>排序 {{ recommendation.sort_order || 0 }}</span>
            </div>
            <div class="template-admin-actions">
              <button class="mini-button" type="button" :data-testid="`edit-recommendation-${recommendation.id}`" @click="openEditRecommendation(recommendation)">
                <Pencil :size="14" />
                编辑
              </button>
              <button class="mini-button danger" type="button" :data-testid="`delete-recommendation-${recommendation.id}`" @click="deleteRecommendation(recommendation)">
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
      <form class="admin-modal prompt-template-modal" data-testid="inspiration-recommendation-modal" @submit.prevent="saveRecommendation">
        <div class="prompt-template-modal-head">
          <div>
            <h2>{{ dialogTitle }}</h2>
            <p>维护发现页热门样图、提示词和一键同款参数。</p>
          </div>
          <button class="icon-button" type="button" aria-label="关闭弹窗" @click="closeDialog">
            <X :size="16" />
          </button>
        </div>

        <div class="prompt-template-modal-body">
          <aside class="prompt-template-preview-panel">
            <div class="prompt-template-panel-title">
              <strong>预览 / 参数</strong>
              <span>{{ form.aspect_ratio || '1:1' }}</span>
            </div>
            <div class="prompt-template-preview-frame">
              <img v-if="form.preview_url" :src="form.preview_url" :alt="form.title || '推荐预览图'" />
              <div v-else class="prompt-template-preview-empty">
                <Image :size="30" />
                <strong>暂无预览图</strong>
                <span>填写 OSS 或公网图片 URL 后前台可展示</span>
              </div>
            </div>
            <dl class="prompt-template-spec-list">
              <div>
                <dt>标识</dt>
                <dd>{{ form.slug || '-' }}</dd>
              </div>
              <div>
                <dt>分类</dt>
                <dd>{{ form.category || '-' }}</dd>
              </div>
              <div>
                <dt>热度标签</dt>
                <dd>{{ form.heat_tags || '-' }}</dd>
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
                <input ref="titleInput" v-model="form.title" data-testid="recommendation-title" required />
              </label>
              <label>
                <span>标识 <b>*</b></span>
                <input v-model="form.slug" data-testid="recommendation-slug" required />
              </label>
              <label>
                <span>分类</span>
                <input v-model="form.category" data-testid="recommendation-category" />
              </label>
              <label>
                <span>热度标签</span>
                <input v-model="form.heat_tags" data-testid="recommendation-heat-tags" placeholder="本周最热, 新手推荐" />
              </label>
              <label>
                <span>预览图 URL <b>*</b></span>
                <input v-model="form.preview_url" data-testid="recommendation-preview-url" />
              </label>
              <label>
                <span>OSS key</span>
                <input v-model="form.preview_asset_key" data-testid="recommendation-preview-asset-key" />
              </label>
              <label>
                <span>比例</span>
                <input v-model="form.aspect_ratio" data-testid="recommendation-aspect-ratio" />
              </label>
              <label>
                <span>风格</span>
                <input v-model="form.style_preset" data-testid="recommendation-style-preset" />
              </label>
              <label>
                <span>主题</span>
                <input v-model="form.theme" data-testid="recommendation-theme" />
              </label>
              <label>
                <span>工具模式</span>
                <select v-model="form.tool_mode" data-testid="recommendation-tool-mode">
                  <option value="generate">generate</option>
                  <option value="expand">expand</option>
                  <option value="erase">erase</option>
                  <option value="upscale">upscale</option>
                  <option value="remove_background">remove_background</option>
                  <option value="precision_edit">precision_edit</option>
                </select>
              </label>
              <label>
                <span>模型 ID</span>
                <input v-model="form.model_id" data-testid="recommendation-model-id" inputmode="numeric" />
              </label>
              <label>
                <span>排序</span>
                <input v-model.number="form.sort_order" data-testid="recommendation-sort-order" type="number" />
              </label>
              <label class="template-form-checkbox">
                <input v-model="form.is_active" data-testid="recommendation-active" type="checkbox" />
                <span>启用推荐</span>
              </label>
            </div>

            <label>
              <span>描述</span>
              <textarea v-model="form.description" rows="2" data-testid="recommendation-description"></textarea>
            </label>
            <label>
              <span>提示词 <b>*</b></span>
              <textarea v-model="form.prompt" rows="5" data-testid="recommendation-prompt" required></textarea>
            </label>
            <label>
              <span>负向提示词</span>
              <textarea v-model="form.negative_prompt" rows="2" data-testid="recommendation-negative-prompt"></textarea>
            </label>
            <label>
              <span>扩展参数 JSON</span>
              <textarea v-model="form.params" rows="5" data-testid="recommendation-params" spellcheck="false"></textarea>
            </label>
          </div>
        </div>

        <div class="prompt-template-modal-actions">
          <button class="secondary-button" type="button" @click="closeDialog">取消</button>
          <button class="primary-button" type="button" data-testid="save-recommendation" :disabled="saving" @click="saveRecommendation">
            {{ saving ? '保存中...' : '保存推荐' }}
          </button>
        </div>
      </form>
    </div>
  </section>
</template>
