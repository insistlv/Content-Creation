<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import {
  CheckCircle2,
  Image,
  MapPin,
  Pencil,
  Plus,
  Save,
  Search,
  Sparkles,
  Trash2,
  Upload,
  X
} from 'lucide-vue-next'

import { api } from '../api/client.js'
import ClickSelect from '../components/ClickSelect.vue'

const optionTypes = [
  { value: 'location', label: '旅游地点', icon: MapPin },
  { value: 'story_template', label: '故事模板', icon: Sparkles },
  { value: 'style', label: '画面风格', icon: Image }
]
const activeFilterOptions = [
  { value: '', label: '全部状态' },
  { value: 'true', label: '启用' },
  { value: 'false', label: '停用' }
]

const items = ref([])
const loading = ref(false)
const saving = ref(false)
const uploading = ref(false)
const message = ref('')
const errorMessage = ref('')
const activeTab = ref('location')
const editingId = ref(null)
const editorOpen = ref(false)

const filters = reactive({
  q: '',
  active: ''
})

const form = reactive(blankForm())

const enabledCount = computed(() => items.value.filter((item) => item.is_active).length)
const disabledCount = computed(() => items.value.filter((item) => !item.is_active).length)
const activeTypeLabel = computed(() => optionTypes.find((item) => item.value === activeTab.value)?.label || '配置')
const groupedItems = computed(() => optionTypes.reduce((acc, type) => {
  acc[type.value] = items.value.filter((item) => item.type === type.value)
  return acc
}, {}))
const visibleItems = computed(() => groupedItems.value[activeTab.value]
  .filter((item) => {
    if (filters.active === 'true' && !item.is_active) return false
    if (filters.active === 'false' && item.is_active) return false
    const query = filters.q.trim().toLowerCase()
    if (!query) return true
    return [item.value, item.label, item.description, item.prompt_label]
      .some((value) => `${value || ''}`.toLowerCase().includes(query))
  }))
const editorTitle = computed(() => editingId.value ? `编辑${activeTypeLabel.value}` : `新增${activeTypeLabel.value}`)
const mediaFieldLabel = computed(() => activeTab.value === 'location' ? '封面图 URL' : '图标 URL')

function blankForm() {
  return {
    type: 'location',
    value: '',
    label: '',
    description: '',
    image_url: '',
    icon_url: '',
    prompt_label: '',
    sort_order: 0,
    is_active: true
  }
}

async function loadOptions() {
  loading.value = true
  errorMessage.value = ''
  try {
    const payload = await api.listAdminCoupleAlbumOptions({ page: 1, page_size: 100 })
    items.value = payload.items ?? []
  } catch (error) {
    errorMessage.value = error.message || '相册配置读取失败'
  } finally {
    loading.value = false
  }
}

function switchTab(type) {
  activeTab.value = type
  filters.q = ''
  filters.active = ''
  if (editorOpen.value && !editingId.value) {
    resetForm()
  }
}

function resetForm(option = null) {
  Object.assign(form, blankForm(), option || {
    type: activeTab.value,
    sort_order: nextSortOrder(activeTab.value),
    is_active: true
  })
}

function nextSortOrder(type) {
  const values = groupedItems.value[type] || []
  if (values.length === 0) return 10
  return Math.max(...values.map((item) => Number(item.sort_order || 0))) + 10
}

function openCreate() {
  editingId.value = null
  resetForm()
  editorOpen.value = true
}

function openEdit(option) {
  activeTab.value = option.type
  editingId.value = option.id
  resetForm(option)
  editorOpen.value = true
}

function closeEditor() {
  if (saving.value || uploading.value) return
  editorOpen.value = false
  editingId.value = null
}

function optionPayload() {
  return {
    type: activeTab.value,
    value: form.value.trim(),
    label: form.label.trim(),
    description: form.description.trim(),
    image_url: form.image_url.trim(),
    icon_url: form.icon_url.trim(),
    prompt_label: form.prompt_label.trim(),
    sort_order: Number(form.sort_order || 0),
    is_active: Boolean(form.is_active)
  }
}

async function saveOption() {
  saving.value = true
  message.value = ''
  errorMessage.value = ''
  try {
    const payload = optionPayload()
    if (editingId.value) {
      await api.updateAdminCoupleAlbumOption(editingId.value, payload)
      message.value = '相册配置已更新'
    } else {
      await api.createAdminCoupleAlbumOption(payload)
      message.value = '相册配置已新增'
    }
    editorOpen.value = false
    editingId.value = null
    await loadOptions()
  } catch (error) {
    errorMessage.value = error.message || '相册配置保存失败'
  } finally {
    saving.value = false
  }
}

async function deleteOption(option) {
  if (typeof window !== 'undefined' && window.confirm && !window.confirm(`删除「${option.label}」？`)) {
    return
  }
  message.value = ''
  errorMessage.value = ''
  try {
    await api.deleteAdminCoupleAlbumOption(option.id)
    message.value = '相册配置已删除'
    await loadOptions()
  } catch (error) {
    errorMessage.value = error.message || '相册配置删除失败'
  }
}

async function uploadAsset(event) {
  const file = event.target.files?.[0]
  if (!file) return
  uploading.value = true
  message.value = ''
  errorMessage.value = ''
  try {
    const uploaded = await api.uploadCoupleAlbumOptionAsset(file)
    if (activeTab.value === 'location') {
      form.image_url = uploaded.url || ''
    } else {
      form.icon_url = uploaded.url || ''
    }
    message.value = '图片已上传'
  } catch (error) {
    errorMessage.value = error.message || '图片上传失败'
  } finally {
    uploading.value = false
    event.target.value = ''
  }
}

function mediaURL(option) {
  return option.type === 'location' ? option.image_url : option.icon_url
}

onMounted(loadOptions)
</script>

<template>
  <section class="admin-couple-options-page">
    <div class="admin-page-heading couple-options-heading">
      <div>
        <p class="eyebrow">管理用户、套餐、模型与生成业务 / 情侣相册</p>
        <h1>情侣相册配置</h1>
        <span>管理旅游地点、故事模板与画面风格，移动端创建页会读取启用项。</span>
      </div>
      <button class="primary-button icon-button-text" type="button" data-testid="new-album-option" @click="openCreate">
        <Plus :size="16" />
        新增选项
      </button>
    </div>

    <div class="couple-option-kpis">
      <article class="admin-panel couple-option-kpi">
        <CheckCircle2 :size="18" />
        <span>启用选项 {{ enabledCount }}</span>
      </article>
      <article class="admin-panel couple-option-kpi">
        <X :size="18" />
        <span>停用 {{ disabledCount }}</span>
      </article>
      <article class="admin-panel couple-option-kpi">
        <Save :size="18" />
        <span>当前分组</span>
        <strong>{{ activeTypeLabel }}</strong>
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

    <div class="couple-option-workspace">
      <section class="admin-panel couple-option-main-panel">
        <div class="couple-option-tabs" role="tablist" aria-label="情侣相册配置分组">
          <button
            v-for="type in optionTypes"
            :key="type.value"
            type="button"
            :data-testid="`album-options-tab-${type.value}`"
            :class="{ active: activeTab === type.value }"
            @click="switchTab(type.value)"
          >
            <component :is="type.icon" :size="16" />
            <span>{{ type.label }}</span>
            <b>{{ groupedItems[type.value].length }}</b>
          </button>
        </div>

        <div class="admin-filter-bar couple-option-toolbar">
          <label class="admin-search-field">
            <Search :size="16" />
            <input v-model.trim="filters.q" type="search" placeholder="搜索名称、value 或提示词标签" />
          </label>
          <ClickSelect v-model="filters.active" :options="activeFilterOptions" class="compact-input" aria-label="启用状态" compact />
          <span class="couple-option-type-hint">当前分组：{{ activeTypeLabel }}</span>
        </div>

        <div v-if="loading" class="admin-empty-state">加载中...</div>
        <div v-else-if="visibleItems.length === 0" class="admin-empty-state">暂无配置</div>
        <div v-else class="couple-option-list">
          <article v-for="option in visibleItems" :key="option.id" class="couple-option-card">
            <div class="couple-option-media" :class="{ icon: option.type !== 'location' }">
              <img v-if="mediaURL(option)" :src="mediaURL(option)" :alt="option.label" />
              <Image v-else :size="24" />
            </div>
            <div class="couple-option-card-body">
              <div class="couple-option-title-row">
                <div>
                  <h2>{{ option.label }}</h2>
                  <p>{{ option.description || option.prompt_label || '未填写描述' }}</p>
                </div>
                <span :class="['status-pill', option.is_active ? 'success' : 'muted']">{{ option.is_active ? '启用' : '停用' }}</span>
              </div>
              <div class="couple-option-meta">
                <span>value: {{ option.value }}</span>
                <span>提示词：{{ option.prompt_label || option.label }}</span>
                <span>排序 {{ option.sort_order || 0 }}</span>
              </div>
              <div class="couple-option-actions">
                <button class="mini-button" type="button" :data-testid="`edit-album-option-${option.id}`" @click="openEdit(option)">
                  <Pencil :size="14" />
                  编辑
                </button>
                <button class="mini-button danger" type="button" :data-testid="`delete-album-option-${option.id}`" @click="deleteOption(option)">
                  <Trash2 :size="14" />
                  删除
                </button>
              </div>
            </div>
          </article>
        </div>
      </section>

      <aside v-if="editorOpen" class="admin-panel couple-option-editor">
        <div class="panel-title-row">
          <div>
            <p class="eyebrow">{{ editingId ? 'Edit option' : 'New option' }}</p>
            <h2>{{ editorTitle }}</h2>
          </div>
          <button class="mini-button icon-only" type="button" aria-label="关闭编辑面板" @click="closeEditor">
            <X :size="16" />
          </button>
        </div>

        <form class="couple-option-form" @submit.prevent="saveOption">
          <label>
            <span class="field-label">选项值</span>
            <input v-model="form.value" data-testid="album-option-value" class="text-input" required />
          </label>
          <label>
            <span class="field-label">显示名称</span>
            <input v-model="form.label" data-testid="album-option-label" class="text-input" required />
          </label>
          <label>
            <span class="field-label">描述文案</span>
            <textarea v-model="form.description" data-testid="album-option-description" class="text-input admin-textarea" rows="3" />
          </label>
          <label>
            <span class="field-label">提示词标签</span>
            <input v-model="form.prompt_label" data-testid="album-option-prompt-label" class="text-input" />
          </label>
          <label>
            <span class="field-label">{{ mediaFieldLabel }}</span>
            <input
              v-if="activeTab === 'location'"
              v-model="form.image_url"
              data-testid="album-option-image-url"
              class="text-input"
              required
            />
            <input
              v-else
              v-model="form.icon_url"
              data-testid="album-option-icon-url"
              class="text-input"
              required
            />
          </label>
          <label class="secondary-button icon-button-text couple-option-upload">
            <Upload :size="16" />
            {{ uploading ? '上传中...' : '上传图片' }}
            <input type="file" accept="image/png,image/jpeg,image/webp" :disabled="uploading" @change="uploadAsset" />
          </label>
          <div class="couple-option-form-row">
            <label>
              <span class="field-label">排序</span>
              <input v-model.number="form.sort_order" class="text-input" type="number" />
            </label>
            <label class="couple-option-toggle">
              <span class="field-label">启用</span>
              <input v-model="form.is_active" type="checkbox" />
            </label>
          </div>
          <div class="couple-option-editor-actions">
            <button class="mini-button" type="button" @click="closeEditor">取消</button>
            <button class="primary-button icon-button-text" type="submit" data-testid="album-option-save" :disabled="saving" @click.prevent="saveOption">
              <Save :size="16" />
              {{ saving ? '保存中...' : '保存配置' }}
            </button>
          </div>
        </form>
      </aside>
    </div>
  </section>
</template>

<style scoped>
.admin-couple-options-page {
  display: grid;
  gap: 18px;
}

.couple-options-heading {
  align-items: flex-start;
}

.couple-option-kpis {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

.couple-option-kpi {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 74px;
  padding: 16px;
}

.couple-option-kpi svg {
  color: #245cff;
}

.couple-option-kpi span {
  color: #64748b;
  font-size: 13px;
  font-weight: 700;
}

.couple-option-kpi strong {
  margin-left: auto;
  color: #0f172a;
  font-size: 24px;
}

.couple-option-workspace {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(320px, 380px);
  gap: 16px;
  align-items: start;
}

.couple-option-main-panel,
.couple-option-editor {
  min-width: 0;
}

.couple-option-tabs {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
  margin-bottom: 14px;
}

.couple-option-tabs button {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  min-height: 44px;
  border: 1px solid rgba(148, 163, 184, 0.28);
  border-radius: 8px;
  background: #f8fafc;
  color: #334155;
  font-weight: 850;
  cursor: pointer;
}

.couple-option-tabs button.active {
  border-color: rgba(36, 92, 255, 0.36);
  background: linear-gradient(135deg, #a94af3 0%, #1767ff 100%);
  color: #fff;
  box-shadow: 0 14px 28px rgba(82, 90, 235, 0.24);
}

.couple-option-tabs b {
  display: grid;
  place-items: center;
  min-width: 22px;
  height: 22px;
  border-radius: 999px;
  background: rgba(15, 23, 42, 0.08);
  font-size: 12px;
}

.couple-option-tabs button.active b {
  background: rgba(255, 255, 255, 0.2);
}

.couple-option-toolbar {
  margin-bottom: 16px;
}

.couple-option-type-hint {
  color: #64748b;
  font-size: 13px;
  font-weight: 800;
}

.couple-option-list {
  display: grid;
  gap: 12px;
}

.couple-option-card {
  display: grid;
  grid-template-columns: 150px minmax(0, 1fr);
  gap: 14px;
  padding: 12px;
  border: 1px solid rgba(148, 163, 184, 0.2);
  border-radius: 8px;
  background: #fff;
}

.couple-option-media {
  display: grid;
  place-items: center;
  width: 150px;
  aspect-ratio: 16 / 9;
  overflow: hidden;
  border-radius: 8px;
  background: #eef2ff;
  color: #64748b;
}

.couple-option-media.icon {
  width: 72px;
  aspect-ratio: 1;
  justify-self: center;
  align-self: center;
}

.couple-option-media img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.couple-option-card-body {
  min-width: 0;
}

.couple-option-title-row,
.couple-option-actions,
.couple-option-editor-actions,
.couple-option-form-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.couple-option-title-row {
  justify-content: space-between;
}

.couple-option-title-row h2 {
  margin: 0;
  color: #0f172a;
  font-size: 16px;
}

.couple-option-title-row p {
  margin: 5px 0 0;
  color: #64748b;
  font-size: 13px;
  font-weight: 650;
}

.couple-option-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 12px;
}

.couple-option-meta span {
  padding: 5px 8px;
  border-radius: 999px;
  background: #f1f5f9;
  color: #475569;
  font-size: 12px;
  font-weight: 750;
}

.couple-option-actions {
  justify-content: flex-end;
  margin-top: 12px;
}

.couple-option-form {
  display: grid;
  gap: 13px;
}

.couple-option-upload {
  justify-content: center;
  min-height: 42px;
}

.couple-option-upload input {
  display: none;
}

.couple-option-form-row > label {
  flex: 1;
}

.couple-option-toggle {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 42px;
  padding: 0 12px;
  border: 1px solid rgba(148, 163, 184, 0.28);
  border-radius: 8px;
  background: #f8fafc;
}

.couple-option-editor-actions {
  justify-content: flex-end;
  padding-top: 4px;
}

@media (max-width: 1040px) {
  .couple-option-workspace {
    grid-template-columns: 1fr;
  }

  .couple-option-kpis,
  .couple-option-tabs {
    grid-template-columns: 1fr;
  }
}
</style>
