<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import {
  Bell,
  CheckCircle2,
  Edit3,
  Eye,
  Megaphone,
  Plus,
  RefreshCw,
  Search,
  Send,
  X
} from 'lucide-vue-next'

import { api } from '../api/client.js'
import ClickSelect from '../components/ClickSelect.vue'

const pageSize = 12
const announcements = ref([])
const selected = ref(null)
const summary = ref({})
const total = ref(0)
const page = ref(1)
const loading = ref(false)
const saving = ref(false)
const modalOpen = ref(false)
const editingId = ref(null)
const errorMessage = ref('')
const actionMessage = ref('')

const filters = reactive({
  status: 'all',
  level: 'all',
  client: 'all',
  keyword: ''
})

const form = reactive({
  title: '',
  content: '',
  level: 'info',
  status: 'draft',
  target: 'all',
  popup_enabled: true,
  starts_at: '',
  ends_at: '',
  priority: 0,
  action_text: '',
  action_url: ''
})
const statusFilterOptions = [
  { value: 'all', label: '全部状态' },
  { value: 'draft', label: '草稿' },
  { value: 'published', label: '已发布' },
  { value: 'offline', label: '已下线' }
]
const levelFilterOptions = [
  { value: 'all', label: '全部级别' },
  { value: 'info', label: '信息' },
  { value: 'important', label: '重要' },
  { value: 'warning', label: '警告' }
]
const clientFilterOptions = [
  { value: 'all', label: '全部端' },
  { value: 'web', label: 'Web' },
  { value: 'mp-weixin', label: '小程序' }
]
const levelOptions = [
  { value: 'info', label: '信息' },
  { value: 'important', label: '重要' },
  { value: 'warning', label: '警告' }
]
const announcementStatusOptions = [
  { value: 'draft', label: '草稿' },
  { value: 'published', label: '发布' },
  { value: 'offline', label: '下线' }
]
const targetOptions = [
  { value: 'all', label: '全部' },
  { value: 'web', label: 'Web' },
  { value: 'mp-weixin', label: '小程序' },
  { value: 'both', label: 'Web / 小程序' }
]

const kpiCards = computed(() => [
  { label: '公告总数', value: summary.value.total ?? 0, icon: Megaphone },
  { label: '已发布', value: summary.value.published ?? 0, icon: CheckCircle2 },
  { label: '草稿', value: summary.value.draft ?? 0, icon: Edit3 },
  { label: '弹窗开启', value: summary.value.popup_enabled ?? 0, icon: Bell }
])

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize || 1)))

function requestParams(targetPage = page.value) {
  return {
    page: targetPage,
    page_size: pageSize,
    status: filters.status,
    level: filters.level,
    client: filters.client,
    keyword: filters.keyword.trim()
  }
}

async function load(targetPage = page.value) {
  loading.value = true
  errorMessage.value = ''
  try {
    const payload = await api.listAnnouncements(requestParams(targetPage))
    announcements.value = payload.items ?? []
    total.value = Number(payload.total ?? announcements.value.length)
    page.value = Number(payload.page ?? targetPage)
    summary.value = payload.summary ?? {}
    selected.value = announcements.value.find((item) => item.id === selected.value?.id) ?? announcements.value[0] ?? null
  } catch (error) {
    errorMessage.value = error.message || '公告读取失败'
  } finally {
    loading.value = false
  }
}

function applyFilters() {
  load(1)
}

function resetForm() {
  editingId.value = null
  form.title = ''
  form.content = ''
  form.level = 'info'
  form.status = 'draft'
  form.target = 'all'
  form.popup_enabled = true
  form.starts_at = ''
  form.ends_at = ''
  form.priority = 0
  form.action_text = ''
  form.action_url = ''
}

function openCreate() {
  resetForm()
  modalOpen.value = true
}

function openEdit(item) {
  editingId.value = item.id
  form.title = item.title ?? ''
  form.content = item.content ?? ''
  form.level = item.level ?? 'info'
  form.status = item.status ?? 'draft'
  form.target = targetValue(item.target_clients)
  form.popup_enabled = Boolean(item.popup_enabled)
  form.starts_at = toDatetimeLocal(item.starts_at)
  form.ends_at = toDatetimeLocal(item.ends_at)
  form.priority = Number(item.priority ?? 0)
  form.action_text = item.action_text ?? ''
  form.action_url = item.action_url ?? ''
  modalOpen.value = true
}

function closeModal() {
  if (saving.value) return
  modalOpen.value = false
}

function payloadFromForm() {
  return {
    title: form.title.trim(),
    content: form.content.trim(),
    level: form.level,
    status: form.status,
    target_clients: targetClientsFromValue(form.target),
    popup_enabled: Boolean(form.popup_enabled),
    starts_at: toRFC3339(form.starts_at),
    ends_at: toRFC3339(form.ends_at),
    priority: Number(form.priority || 0),
    action_text: form.action_text.trim(),
    action_url: form.action_url.trim()
  }
}

async function submitForm() {
  saving.value = true
  errorMessage.value = ''
  actionMessage.value = ''
  try {
    const payload = payloadFromForm()
    if (editingId.value) {
      await api.updateAnnouncement(editingId.value, payload)
      actionMessage.value = '公告已更新'
    } else {
      await api.createAnnouncement(payload)
      actionMessage.value = '公告已保存'
    }
    modalOpen.value = false
    await load()
  } catch (error) {
    errorMessage.value = error.message || '公告保存失败'
  } finally {
    saving.value = false
  }
}

async function setStatus(item, status) {
  errorMessage.value = ''
  actionMessage.value = ''
  try {
    await api.updateAnnouncementStatus(item.id, status)
    actionMessage.value = status === 'published' ? '公告已发布' : '公告已下线'
    await load()
  } catch (error) {
    errorMessage.value = error.message || '状态更新失败'
  }
}

function selectAnnouncement(item) {
  selected.value = item
}

function targetClientsFromValue(value) {
  if (value === 'both') return ['web', 'mp-weixin']
  if (value === 'web') return ['web']
  if (value === 'mp-weixin') return ['mp-weixin']
  return ['all']
}

function targetValue(clients = []) {
  const values = clients.length > 0 ? clients : ['all']
  if (values.includes('all')) return 'all'
  if (values.includes('web') && values.includes('mp-weixin')) return 'both'
  if (values.includes('mp-weixin')) return 'mp-weixin'
  return 'web'
}

function formatTargets(clients = []) {
  const value = targetValue(clients)
  if (value === 'all') return '全部'
  if (value === 'both') return 'Web / 小程序'
  if (value === 'mp-weixin') return '小程序'
  return 'Web'
}

function levelText(level) {
  const map = { info: '信息', important: '重要', warning: '警告' }
  return map[level] ?? level ?? '-'
}

function statusText(status) {
  const map = { draft: '草稿', published: '已发布', offline: '已下线' }
  return map[status] ?? status ?? '-'
}

function formatDate(value) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return date.toLocaleString('zh-CN', { hour12: false })
}

function toDatetimeLocal(value) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const offset = date.getTimezoneOffset()
  const local = new Date(date.getTime() - offset * 60000)
  return local.toISOString().slice(0, 16)
}

function toRFC3339(value) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return date.toISOString()
}

onMounted(() => {
  load()
})
</script>

<template>
  <section class="admin-page admin-announcements-page">
    <div class="admin-page-heading announcements-heading">
      <div>
        <p class="eyebrow">Announcements</p>
        <h1>公告通知</h1>
      </div>
      <div class="dashboard-heading-actions">
        <button class="secondary-button icon-button-text" type="button" @click="load()">
          <RefreshCw :size="16" />
          刷新
        </button>
        <button class="primary-button icon-button-text" data-testid="open-announcement-create" type="button" @click="openCreate">
          <Plus :size="16" />
          新建公告
        </button>
      </div>
    </div>

    <p v-if="errorMessage" class="status-error">{{ errorMessage }}</p>
    <p v-if="actionMessage" class="status-success">{{ actionMessage }}</p>

    <div class="announcements-kpis">
      <article v-for="item in kpiCards" :key="item.label" class="admin-kpi-card">
        <span>
          <component :is="item.icon" :size="18" />
        </span>
        <div>
          <p>{{ item.label }}</p>
          <strong>{{ item.value }}</strong>
        </div>
      </article>
    </div>

    <div class="admin-toolbar announcements-toolbar">
      <label class="admin-search-field">
        <Search :size="16" />
        <input v-model="filters.keyword" type="search" placeholder="搜索标题或内容" @keyup.enter="applyFilters" />
      </label>
      <ClickSelect v-model="filters.status" :options="statusFilterOptions" class="text-input" aria-label="公告状态" @change="applyFilters" />
      <ClickSelect v-model="filters.level" :options="levelFilterOptions" class="text-input" aria-label="公告级别" @change="applyFilters" />
      <ClickSelect v-model="filters.client" :options="clientFilterOptions" class="text-input" aria-label="投放端" @change="applyFilters" />
    </div>

    <div class="announcements-grid">
      <div class="admin-panel announcements-table-panel">
        <table class="admin-table announcements-table">
          <thead>
            <tr>
              <th>公告</th>
              <th>级别</th>
              <th>投放端</th>
              <th>状态</th>
              <th>优先级</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in announcements" :key="item.id" :class="{ active: selected?.id === item.id }" @click="selectAnnouncement(item)">
              <td>
                <strong>{{ item.title }}</strong>
                <small>{{ item.content }}</small>
              </td>
              <td><span :class="['status-pill', `is-${item.level}`]">{{ levelText(item.level) }}</span></td>
              <td>{{ formatTargets(item.target_clients) }}</td>
              <td><span :class="['status-pill', `is-${item.status}`]">{{ statusText(item.status) }}</span></td>
              <td>{{ item.priority ?? 0 }}</td>
              <td>
                <div class="table-actions">
                  <button class="mini-button icon-only" :data-testid="`edit-announcement-${item.id}`" type="button" aria-label="编辑公告" @click.stop="openEdit(item)">
                    <Edit3 :size="15" />
                  </button>
                  <button
                    v-if="item.status === 'published'"
                    class="secondary-button compact-button"
                    :data-testid="`offline-announcement-${item.id}`"
                    type="button"
                    @click.stop="setStatus(item, 'offline')"
                  >
                    下线
                  </button>
                  <button
                    v-else
                    class="primary-button compact-button"
                    :data-testid="`publish-announcement-${item.id}`"
                    type="button"
                    @click.stop="setStatus(item, 'published')"
                  >
                    发布
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
        <p v-if="!loading && announcements.length === 0" class="empty-state">暂无公告</p>
        <div class="admin-pagination">
          <span>第 {{ page }} / {{ totalPages }} 页</span>
          <button class="secondary-button compact-button" type="button" :disabled="page <= 1" @click="load(page - 1)">上一页</button>
          <button class="secondary-button compact-button" type="button" :disabled="page >= totalPages" @click="load(page + 1)">下一页</button>
        </div>
      </div>

      <aside class="admin-panel announcement-preview-panel" data-testid="announcement-preview">
        <template v-if="selected">
          <div class="announcement-preview-head">
            <span :class="['announcement-preview-icon', `is-${selected.level}`]">
              <Eye :size="18" />
            </span>
            <div>
              <p>{{ statusText(selected.status) }} · {{ formatTargets(selected.target_clients) }}</p>
              <h2>{{ selected.title }}</h2>
            </div>
          </div>
          <p class="announcement-preview-content">{{ selected.content }}</p>
          <dl class="announcement-preview-meta">
            <div>
              <dt>展示时间</dt>
              <dd>{{ formatDate(selected.starts_at) }} - {{ formatDate(selected.ends_at) }}</dd>
            </div>
            <div>
              <dt>发布时间</dt>
              <dd>{{ formatDate(selected.published_at) }}</dd>
            </div>
            <div>
              <dt>CTA</dt>
              <dd>{{ selected.action_text || '-' }} {{ selected.action_url || '' }}</dd>
            </div>
          </dl>
        </template>
        <p v-else class="empty-state">选择公告查看预览</p>
      </aside>
    </div>

    <div v-if="modalOpen" class="dashboard-modal-backdrop">
      <form class="dashboard-modal admin-panel announcement-modal" data-testid="announcement-form" @submit.prevent="submitForm">
        <div class="modal-head">
          <h2>{{ editingId ? '编辑公告' : '新建公告' }}</h2>
          <button class="mini-button icon-only" type="button" aria-label="关闭公告弹层" @click="closeModal">
            <X :size="17" />
          </button>
        </div>

        <label>
          <span>标题</span>
          <input v-model="form.title" class="text-input" data-testid="announcement-title" required maxlength="160" />
        </label>
        <label>
          <span>内容</span>
          <textarea v-model="form.content" class="text-input admin-textarea" data-testid="announcement-content" required rows="4"></textarea>
        </label>

        <div class="form-grid">
          <label>
            <span>级别</span>
            <ClickSelect v-model="form.level" :options="levelOptions" class="text-input" data-testid="announcement-level" aria-label="级别" />
          </label>
          <label>
            <span>状态</span>
            <ClickSelect v-model="form.status" :options="announcementStatusOptions" class="text-input" data-testid="announcement-status" aria-label="状态" />
          </label>
          <label>
            <span>投放端</span>
            <ClickSelect v-model="form.target" :options="targetOptions" class="text-input" data-testid="announcement-target" aria-label="投放端" />
          </label>
          <label>
            <span>优先级</span>
            <input v-model="form.priority" class="text-input" data-testid="announcement-priority" type="number" />
          </label>
          <label>
            <span>开始时间</span>
            <input v-model="form.starts_at" class="text-input" data-testid="announcement-starts-at" type="datetime-local" />
          </label>
          <label>
            <span>结束时间</span>
            <input v-model="form.ends_at" class="text-input" data-testid="announcement-ends-at" type="datetime-local" />
          </label>
        </div>

        <label class="checkbox-row">
          <input v-model="form.popup_enabled" data-testid="announcement-popup" type="checkbox" />
          <span>作为登录后弹窗展示</span>
        </label>

        <div class="form-grid">
          <label>
            <span>CTA 文案</span>
            <input v-model="form.action_text" class="text-input" data-testid="announcement-action-text" maxlength="80" />
          </label>
          <label>
            <span>CTA 链接</span>
            <input v-model="form.action_url" class="text-input" data-testid="announcement-action-url" />
          </label>
        </div>

        <button class="primary-button icon-button-text" type="submit" :disabled="saving">
          <Send :size="16" />
          {{ saving ? '保存中...' : '保存公告' }}
        </button>
      </form>
    </div>
  </section>
</template>
