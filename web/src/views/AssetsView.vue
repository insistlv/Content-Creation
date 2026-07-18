<script setup>
import { computed, onMounted, ref } from 'vue'
import { Grid2X2, List, Pencil, Send, Trash2, X } from 'lucide-vue-next'
import { useRouter } from 'vue-router'

import { api } from '../api/client.js'

const router = useRouter()
const assets = ref([])
const loading = ref(false)
const uploading = ref(false)
const errorMessage = ref('')
const message = ref('')
const typeFilter = ref('all')
const viewMode = ref('grid')
const selectedIds = ref([])
const editingId = ref(null)
const editingName = ref('')
const savingRenameId = ref(null)
const deletingIds = ref([])

const typeFilters = [
  { key: 'all', label: '全部' },
  { key: 'jpg', label: 'JPG' },
  { key: 'png', label: 'PNG' },
  { key: 'webp', label: 'WEBP' }
]

function responseItems(payload) {
  return Array.isArray(payload) ? payload : (payload?.items ?? [])
}

const filteredAssets = computed(() => assets.value.filter((asset) => {
  if (typeFilter.value === 'all') return true
  const mimeType = `${asset.mime_type || ''}`.toLowerCase()
  if (typeFilter.value === 'jpg') return mimeType === 'image/jpeg' || mimeType === 'image/jpg'
  if (typeFilter.value === 'png') return mimeType === 'image/png'
  if (typeFilter.value === 'webp') return mimeType === 'image/webp'
  return true
}))

const selectedAssets = computed(() => {
  const selected = new Set(selectedIds.value)
  return assets.value.filter((asset) => selected.has(asset.id))
})
const selectedCount = computed(() => selectedAssets.value.length)
const canBulkUse = computed(() => selectedCount.value > 0 && selectedCount.value <= 4)
const selectionLimitMessage = computed(() => selectedCount.value > 4 ? '最多送入 4 张参考图' : '')

async function loadAssets() {
  loading.value = true
  errorMessage.value = ''
  try {
    assets.value = responseItems(await api.listReferenceAssets())
    selectedIds.value = selectedIds.value.filter((id) => assets.value.some((asset) => asset.id === id))
  } catch (error) {
    errorMessage.value = error.message || '素材读取失败'
  } finally {
    loading.value = false
  }
}

async function handleUpload(event) {
  const file = event?.target?.files?.[0]
  if (!file || uploading.value) return
  uploading.value = true
  errorMessage.value = ''
  message.value = ''
  try {
    const uploaded = await api.uploadReferenceAsset(file)
    assets.value = [
      uploaded,
      ...assets.value.filter((item) => item.id !== uploaded.id)
    ]
    message.value = '素材已上传。'
  } catch (error) {
    errorMessage.value = error.message || '素材上传失败'
  } finally {
    uploading.value = false
    if (event?.target) event.target.value = ''
  }
}

async function removeAsset(asset) {
  errorMessage.value = ''
  message.value = ''
  deletingIds.value = [...deletingIds.value, asset.id]
  try {
    await api.deleteReferenceAsset(asset.id)
    removeAssetsFromState([asset.id])
    message.value = '素材已删除。'
  } catch (error) {
    errorMessage.value = error.message || '素材删除失败'
  } finally {
    deletingIds.value = deletingIds.value.filter((id) => id !== asset.id)
  }
}

async function bulkDeleteAssets() {
  if (selectedCount.value === 0) return
  if (typeof window !== 'undefined' && window.confirm && !window.confirm(`删除选中的 ${selectedCount.value} 个素材？`)) {
    return
  }

  errorMessage.value = ''
  message.value = ''
  const targets = [...selectedAssets.value]
  deletingIds.value = [...new Set([...deletingIds.value, ...targets.map((asset) => asset.id)])]
  const results = await Promise.allSettled(targets.map((asset) => api.deleteReferenceAsset(asset.id)))
  const deletedIds = targets
    .filter((_, index) => results[index].status === 'fulfilled')
    .map((asset) => asset.id)
  removeAssetsFromState(deletedIds)
  deletingIds.value = deletingIds.value.filter((id) => !targets.some((asset) => asset.id === id))

  if (deletedIds.length === targets.length) {
    message.value = `已删除 ${deletedIds.length} 个素材。`
    return
  }
  errorMessage.value = '部分素材删除失败'
}

function removeAssetsFromState(ids) {
  const deleted = new Set(ids)
  assets.value = assets.value.filter((item) => !deleted.has(item.id))
  selectedIds.value = selectedIds.value.filter((id) => !deleted.has(id))
}

function useInWorkspace(asset) {
  writeWorkspacePrefill([asset.id])
}

function bulkUseInWorkspace() {
  if (!canBulkUse.value) return
  writeWorkspacePrefill(selectedAssets.value.map((asset) => asset.id))
}

function writeWorkspacePrefill(referenceAssetIds) {
  window.sessionStorage?.setItem('image_agent_workspace_prefill:v1', JSON.stringify({
    reference_asset_ids: referenceAssetIds
  }))
  router.push('/workspace')
}

function toggleSelected(asset, checked) {
  const current = new Set(selectedIds.value)
  if (checked) {
    current.add(asset.id)
  } else {
    current.delete(asset.id)
  }
  selectedIds.value = [...current]
}

function isSelected(asset) {
  return selectedIds.value.includes(asset.id)
}

function startRename(asset) {
  editingId.value = asset.id
  editingName.value = asset.display_name || ''
  errorMessage.value = ''
  message.value = ''
}

function cancelRename() {
  editingId.value = null
  editingName.value = ''
}

async function saveRename(asset) {
  const displayName = editingName.value.trim()
  savingRenameId.value = asset.id
  errorMessage.value = ''
  message.value = ''
  try {
    const updated = await api.updateReferenceAsset(asset.id, { display_name: displayName })
    assets.value = assets.value.map((item) => item.id === asset.id
      ? { ...item, ...updated, display_name: updated.display_name ?? displayName }
      : item)
    cancelRename()
    message.value = displayName ? '素材名称已更新。' : '素材名称已清除。'
  } catch (error) {
    errorMessage.value = error.message || '素材名称保存失败'
  } finally {
    savingRenameId.value = null
  }
}

function displayName(asset) {
  const customName = `${asset.display_name || ''}`.trim()
  return customName || asset.original_filename || `素材 ${asset.id}`
}

function originalTitle(asset) {
  return asset.original_filename || displayName(asset)
}

function formatAssetDate(value) {
  if (!value) return '刚刚上传'
  return new Date(value).toLocaleDateString('zh-CN')
}

function typeLabel(asset) {
  const mimeType = `${asset.mime_type || ''}`.toLowerCase()
  if (mimeType === 'image/jpeg' || mimeType === 'image/jpg') return 'JPG'
  if (mimeType === 'image/png') return 'PNG'
  if (mimeType === 'image/webp') return 'WEBP'
  return '图片'
}

function isBusy(asset) {
  return savingRenameId.value === asset.id || deletingIds.value.includes(asset.id)
}

onMounted(loadAssets)
</script>

<template>
  <section class="assets-page">
    <header class="assets-header">
      <div>
        <p class="eyebrow">ASSETS</p>
        <h1>素材库</h1>
        <p>管理可复用参考图，直接带入工作台继续创作。</p>
      </div>
      <label class="secondary-button assets-upload-button">
        <span>{{ uploading ? '上传中...' : '上传素材' }}</span>
        <input
          data-testid="asset-upload-input"
          type="file"
          accept="image/jpeg,image/png,image/webp"
          :disabled="uploading"
          @change="handleUpload"
        />
      </label>
    </header>

    <div v-if="assets.length" class="assets-toolbar">
      <div class="assets-filter-group" aria-label="文件类型筛选">
        <button
          v-for="item in typeFilters"
          :key="item.key"
          :data-testid="`asset-filter-${item.key}`"
          type="button"
          :class="{ active: typeFilter === item.key }"
          @click="typeFilter = item.key"
        >
          {{ item.label }}
        </button>
      </div>
      <div class="assets-toolbar-actions">
        <span data-testid="selected-count">已选 {{ selectedCount }} 项</span>
        <button
          class="secondary-button compact-button"
          data-testid="asset-bulk-use"
          type="button"
          :disabled="!canBulkUse"
          @click="bulkUseInWorkspace"
        >
          <Send :size="15" />
          送入工作台
        </button>
        <button
          class="secondary-button compact-button destructive-button"
          data-testid="asset-bulk-delete"
          type="button"
          :disabled="selectedCount === 0"
          @click="bulkDeleteAssets"
        >
          <Trash2 :size="15" />
          批量删除
        </button>
        <div class="assets-view-toggle" aria-label="视图切换">
          <button
            data-testid="asset-view-grid"
            type="button"
            :class="{ active: viewMode === 'grid' }"
            title="网格视图"
            @click="viewMode = 'grid'"
          >
            <Grid2X2 :size="16" />
          </button>
          <button
            data-testid="asset-view-list"
            type="button"
            :class="{ active: viewMode === 'list' }"
            title="列表视图"
            @click="viewMode = 'list'"
          >
            <List :size="16" />
          </button>
        </div>
      </div>
    </div>
    <p v-if="selectionLimitMessage" class="asset-limit-hint" data-testid="asset-selection-limit">{{ selectionLimitMessage }}</p>

    <p v-if="loading" class="page-status">加载中...</p>
    <p v-if="message" class="status-success">{{ message }}</p>
    <p v-if="errorMessage" class="status-error">{{ errorMessage }}</p>

    <div v-if="assets.length && viewMode === 'grid'" class="assets-grid">
      <article
        v-for="asset in filteredAssets"
        :key="asset.id"
        class="asset-card"
        :class="{ selected: isSelected(asset) }"
        :data-testid="`asset-card-${asset.id}`"
      >
        <label class="asset-select">
          <input
            :data-testid="`asset-select-${asset.id}`"
            type="checkbox"
            :checked="isSelected(asset)"
            @change="toggleSelected(asset, $event.target.checked)"
          />
          <span>选择</span>
        </label>
        <img v-if="asset.preview_url" :src="asset.preview_url" :alt="displayName(asset)" />
        <div v-else class="works-card-placeholder">素材</div>
        <div class="asset-card-body">
          <template v-if="editingId === asset.id">
            <input
              v-model="editingName"
              class="asset-rename-input"
              :data-testid="`asset-rename-input-${asset.id}`"
              maxlength="80"
              type="text"
              :disabled="savingRenameId === asset.id"
              @keyup.enter="saveRename(asset)"
              @keyup.esc="cancelRename"
            />
            <div class="asset-rename-actions">
              <button :data-testid="`asset-rename-save-${asset.id}`" type="button" :disabled="savingRenameId === asset.id" @click="saveRename(asset)">保存</button>
              <button type="button" :disabled="savingRenameId === asset.id" @click="cancelRename">取消</button>
            </div>
          </template>
          <template v-else>
            <strong class="asset-name" :data-testid="`asset-name-${asset.id}`" :title="originalTitle(asset)">{{ displayName(asset) }}</strong>
            <span :data-testid="`asset-date-${asset.id}`">{{ formatAssetDate(asset.created_at) }} · {{ typeLabel(asset) }}</span>
          </template>
        </div>
        <div class="asset-card-actions" :data-testid="`asset-card-actions-${asset.id}`">
          <button :data-testid="`asset-use-${asset.id}`" type="button" :disabled="isBusy(asset)" @click="useInWorkspace(asset)">
            <Send :size="14" />
            送入
          </button>
          <button :data-testid="`asset-rename-${asset.id}`" type="button" :disabled="isBusy(asset)" @click="startRename(asset)">
            <Pencil :size="14" />
            重命名
          </button>
          <button :data-testid="`asset-delete-${asset.id}`" type="button" :disabled="isBusy(asset)" @click="removeAsset(asset)">
            <Trash2 :size="14" />
            删除
          </button>
        </div>
      </article>
    </div>

    <div v-else-if="assets.length" class="asset-list" data-testid="asset-list">
      <article
        v-for="asset in filteredAssets"
        :key="asset.id"
        class="asset-list-row"
        :class="{ selected: isSelected(asset) }"
        :data-testid="`asset-row-${asset.id}`"
      >
        <label class="asset-row-check">
          <input
            :data-testid="`asset-select-${asset.id}`"
            type="checkbox"
            :checked="isSelected(asset)"
            @change="toggleSelected(asset, $event.target.checked)"
          />
          <span>选择</span>
        </label>
        <img v-if="asset.preview_url" :src="asset.preview_url" :alt="displayName(asset)" />
        <div class="asset-row-main">
          <template v-if="editingId === asset.id">
            <input
              v-model="editingName"
              class="asset-rename-input"
              :data-testid="`asset-rename-input-${asset.id}`"
              maxlength="80"
              type="text"
              :disabled="savingRenameId === asset.id"
              @keyup.enter="saveRename(asset)"
              @keyup.esc="cancelRename"
            />
            <div class="asset-rename-actions">
              <button :data-testid="`asset-rename-save-${asset.id}`" type="button" :disabled="savingRenameId === asset.id" @click="saveRename(asset)">保存</button>
              <button type="button" :disabled="savingRenameId === asset.id" @click="cancelRename">取消</button>
            </div>
          </template>
          <template v-else>
            <strong class="asset-name" :data-testid="`asset-name-${asset.id}`" :title="originalTitle(asset)">{{ displayName(asset) }}</strong>
            <span>{{ originalTitle(asset) }}</span>
          </template>
        </div>
        <span class="asset-row-type">{{ typeLabel(asset) }}</span>
        <span class="asset-row-date" :data-testid="`asset-date-${asset.id}`">{{ formatAssetDate(asset.created_at) }}</span>
        <div class="asset-row-actions">
          <button :data-testid="`asset-use-${asset.id}`" type="button" :disabled="isBusy(asset)" title="送入工作台" @click="useInWorkspace(asset)">
            <Send :size="15" />
          </button>
          <button :data-testid="`asset-rename-${asset.id}`" type="button" :disabled="isBusy(asset)" title="重命名" @click="startRename(asset)">
            <Pencil :size="15" />
          </button>
          <button :data-testid="`asset-delete-${asset.id}`" type="button" :disabled="isBusy(asset)" title="删除" @click="removeAsset(asset)">
            <X :size="15" />
          </button>
        </div>
      </article>
    </div>

    <section v-if="assets.length && filteredAssets.length === 0" class="works-empty-panel">
      <p class="eyebrow">ASSETS</p>
      <h2>没有匹配的素材。</h2>
      <p>切换文件类型筛选后继续查看。</p>
    </section>

    <section v-else-if="!assets.length && !loading" class="works-empty-panel">
      <p class="eyebrow">ASSETS</p>
      <h2>还没有参考素材。</h2>
      <p>上传 JPG/PNG/WEBP 后，可在工作台作为参考图使用。</p>
    </section>
  </section>
</template>
