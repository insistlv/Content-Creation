<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import {
  CalendarPlus,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  Copy,
  Download,
  Eye,
  MoreHorizontal,
  RefreshCw,
  Search,
  Ticket,
  TicketCheck,
  UserPlus,
  Users
} from 'lucide-vue-next'

import { api } from '../api/client.js'
import ClickSelect from '../components/ClickSelect.vue'

const invites = ref([])
const redemptions = ref([])
const loadingInvites = ref(false)
const loadingRedemptions = ref(false)
const savingBatch = ref(false)
const errorMessage = ref('')
const actionMessage = ref('')
const invitePage = ref(1)
const invitePageSize = 10
const inviteTotal = ref(0)
const redemptionPage = ref(1)
const redemptionPageSize = 10
const redemptionTotal = ref(0)

const inviteFilters = reactive({
  q: '',
  status: 'all'
})

const batchForm = reactive({
  prefix: 'OPS',
  quantity: 5,
  expires_at: '',
  total_quota: 1
})

const redemptionFilters = reactive({
  start_date: '',
  end_date: '',
  result: 'all'
})

const summary = reactive({
  available_invites: 0,
  available_invites_delta_percent: 0,
  used_invites: 0,
  used_invites_delta_percent: 0,
  today_new_invite_users: 0,
  today_new_invite_users_delta_percent: 0,
  invite_conversion_rate: 0,
  invite_conversion_rate_delta_percent: 0
})

const inviteStatusOptions = [
  { value: 'all', label: '全部状态' },
  { value: 'available', label: '可用' },
  { value: 'partial', label: '部分使用' },
  { value: 'used', label: '已使用' },
  { value: 'expired', label: '已过期' },
  { value: 'disabled', label: '已停用' }
]

const redemptionResultOptions = [
  { value: 'all', label: '全部结果' },
  { value: 'converted', label: '已转化' },
  { value: 'unconverted', label: '未转化' }
]

const kpiCards = computed(() => [
  {
    key: 'available',
    label: '可用邀请码',
    value: formatNumber(summary.available_invites),
    delta: summary.available_invites_delta_percent,
    icon: Ticket
  },
  {
    key: 'used',
    label: '已使用',
    value: formatNumber(summary.used_invites),
    delta: summary.used_invites_delta_percent,
    icon: TicketCheck
  },
  {
    key: 'today',
    label: '今日新增邀请用户',
    value: formatNumber(summary.today_new_invite_users),
    delta: summary.today_new_invite_users_delta_percent,
    icon: UserPlus
  },
  {
    key: 'conversion',
    label: '邀请转化率',
    value: `${formatNumber(summary.invite_conversion_rate)}%`,
    delta: summary.invite_conversion_rate_delta_percent,
    icon: CheckCircle2
  }
])

const inviteTotalPages = computed(() => Math.max(1, Math.ceil(inviteTotal.value / invitePageSize)))
const inviteRangeStart = computed(() => (inviteTotal.value === 0 ? 0 : (invitePage.value - 1) * invitePageSize + 1))
const inviteRangeEnd = computed(() => Math.min(inviteTotal.value, invitePage.value * invitePageSize))
const redemptionTotalPages = computed(() => Math.max(1, Math.ceil(redemptionTotal.value / redemptionPageSize)))
const redemptionRangeStart = computed(() => (redemptionTotal.value === 0 ? 0 : (redemptionPage.value - 1) * redemptionPageSize + 1))
const redemptionRangeEnd = computed(() => Math.min(redemptionTotal.value, redemptionPage.value * redemptionPageSize))

function formatNumber(value) {
  return new Intl.NumberFormat('zh-CN').format(Number(value ?? 0))
}

function formatDate(value) {
  if (!value) return '-'
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  }).format(new Date(value))
}

function formatDateOnly(value) {
  if (!value) return '长期有效'
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit'
  }).format(new Date(value))
}

function formatDelta(value) {
  const number = Number(value ?? 0)
  const text = Number.isInteger(number) ? number.toFixed(0) : number.toFixed(1)
  return `${number > 0 ? '+' : ''}${text}%`
}

function inviteParams() {
  return {
    page: invitePage.value,
    page_size: invitePageSize,
    q: inviteFilters.q.trim(),
    status: inviteFilters.status === 'all' ? '' : inviteFilters.status
  }
}

function redemptionParams() {
  return {
    page: redemptionPage.value,
    page_size: redemptionPageSize,
    start_date: redemptionFilters.start_date,
    end_date: redemptionFilters.end_date,
    result: redemptionFilters.result === 'all' ? '' : redemptionFilters.result
  }
}

async function loadInvites() {
  loadingInvites.value = true
  errorMessage.value = ''
  try {
    const payload = await api.listInvites(inviteParams())
    invites.value = payload.items ?? []
    inviteTotal.value = Number(payload.total ?? invites.value.length)
    invitePage.value = Number(payload.page ?? invitePage.value)
    Object.assign(summary, payload.summary ?? {})
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    loadingInvites.value = false
  }
}

async function loadRedemptions() {
  loadingRedemptions.value = true
  errorMessage.value = ''
  try {
    const payload = await api.listInviteRedemptions(redemptionParams())
    redemptions.value = payload.items ?? []
    redemptionTotal.value = Number(payload.total ?? redemptions.value.length)
    redemptionPage.value = Number(payload.page ?? redemptionPage.value)
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    loadingRedemptions.value = false
  }
}

async function load() {
  await Promise.all([loadInvites(), loadRedemptions()])
}

async function applyInviteFilters() {
  invitePage.value = 1
  await loadInvites()
}

async function applyRedemptionFilters() {
  redemptionPage.value = 1
  await loadRedemptions()
}

async function goToInvitePage(nextPage) {
  invitePage.value = Math.min(Math.max(nextPage, 1), inviteTotalPages.value)
  await loadInvites()
}

async function goToRedemptionPage(nextPage) {
  redemptionPage.value = Math.min(Math.max(nextPage, 1), redemptionTotalPages.value)
  await loadRedemptions()
}

async function batchCreate() {
  savingBatch.value = true
  errorMessage.value = ''
  actionMessage.value = ''
  try {
    const payload = {
      prefix: batchForm.prefix.trim(),
      quantity: Number(batchForm.quantity || 1),
      total_quota: Number(batchForm.total_quota || 1)
    }
    if (batchForm.expires_at) {
      payload.expires_at = `${batchForm.expires_at}T23:59:59+08:00`
    }
    await api.batchCreateInvites(payload)
    actionMessage.value = '邀请码已生成'
    await loadInvites()
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    savingBatch.value = false
  }
}

async function toggleInvite(invite) {
  const nextStatus = invite.status === 'active' ? 'disabled' : 'active'
  errorMessage.value = ''
  actionMessage.value = ''
  try {
    await api.updateInvite(invite.id, {
      status: nextStatus,
      total_quota: invite.total_quota,
      label: invite.label,
      notes: invite.notes,
      expires_at: invite.expires_at
    })
    actionMessage.value = nextStatus === 'active' ? '邀请码已启用' : '邀请码已停用'
    await loadInvites()
  } catch (error) {
    errorMessage.value = error.message
  }
}

async function copyInvite(invite) {
  if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(invite.code)
    actionMessage.value = '邀请码已复制'
  }
}

function showInvite(invite) {
  actionMessage.value = `查看 ${invite.code}`
}

function exportInvites() {
  openExport(`/api/admin/invites/export${toQuery(inviteParams())}`, 'invites-export')
}

function exportRedemptions() {
  openExport(`/api/admin/invite-redemptions/export${toQuery(redemptionParams())}`, 'invite-redemptions-export')
}

function openExport(url, testId) {
  if (typeof document === 'undefined') return
  const link = document.createElement('a')
  link.href = url
  link.dataset.testid = testId
  link.download = ''
  document.body.appendChild(link)
  link.click()
  link.remove()
}

function toQuery(params) {
  const query = new URLSearchParams()
  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== '') {
      query.set(key, `${value}`)
    }
  })
  const text = query.toString()
  return text ? `?${text}` : ''
}

function remainingQuota(invite) {
  return Math.max(Number(invite.total_quota ?? 0) - Number(invite.used_quota ?? 0), 0)
}

function inviteState(invite) {
  if (invite.expires_at && new Date(invite.expires_at).getTime() < Date.now()) return 'expired'
  if (Number(invite.total_quota ?? 0) > 0 && Number(invite.used_quota ?? 0) >= Number(invite.total_quota ?? 0)) return 'used'
  if (invite.status === 'disabled') return 'disabled'
  if (Number(invite.used_quota ?? 0) > 0) return 'partial'
  return 'available'
}

function inviteStateText(invite) {
  const state = inviteState(invite)
  return inviteStatusOptions.find((item) => item.value === state)?.label ?? '-'
}

function inviteStateClass(invite) {
  return `invite-status-${inviteState(invite)}`
}

function redemptionResultText(result) {
  return result === 'converted' ? '已转化' : '未转化'
}

function redemptionResultClass(result) {
  return result === 'converted' ? 'invite-status-available' : 'invite-status-disabled'
}

onMounted(load)
</script>

<template>
  <section class="admin-invites-page">
    <div class="section-heading">
      <div>
        <p class="eyebrow">后台管理中心 / 邀请码管理</p>
        <h1>邀请码管理</h1>
        <span>批量生成、筛选导出并跟踪邀请注册后的转化结果。</span>
      </div>
      <div class="invite-heading-actions">
        <button class="secondary-button icon-button-text" type="button" :disabled="loadingInvites || loadingRedemptions" @click="load">
          <RefreshCw :size="16" />
          刷新
        </button>
      </div>
    </div>

    <div class="users-kpi-grid invite-kpi-grid">
      <article v-for="card in kpiCards" :key="card.key" class="users-kpi-card invite-kpi-card">
        <div class="users-kpi-topline">
          <span>
            <component :is="card.icon" :size="18" />
            {{ card.label }}
          </span>
          <b class="users-kpi-delta" :class="{ negative: card.delta < 0 }">{{ formatDelta(card.delta) }}</b>
        </div>
        <strong>{{ card.value }}</strong>
      </article>
    </div>

    <div class="invite-workspace-grid">
      <form class="admin-panel invite-generator-panel" data-testid="invite-batch-form" @submit.prevent="batchCreate">
        <div class="panel-title-row">
          <div>
            <p class="eyebrow">Generate</p>
            <h2>生成邀请码</h2>
          </div>
          <Ticket :size="20" />
        </div>
        <label>
          <span>前缀</span>
          <input v-model="batchForm.prefix" data-testid="invite-prefix" class="text-input" maxlength="12" placeholder="OPS" />
        </label>
        <label>
          <span>数量</span>
          <input v-model.number="batchForm.quantity" data-testid="invite-quantity" class="text-input" type="number" min="1" max="200" />
        </label>
        <label>
          <span>有效期</span>
          <input v-model="batchForm.expires_at" data-testid="invite-expires-at" class="text-input" type="date" />
        </label>
        <label>
          <span>可使用次数</span>
          <input v-model.number="batchForm.total_quota" data-testid="invite-total-quota" class="text-input" type="number" min="1" />
        </label>
        <button class="primary-button icon-button-text" type="submit" :disabled="savingBatch">
          <CalendarPlus :size="16" />
          批量生成
        </button>
      </form>

      <article class="admin-panel invite-list-panel">
        <div class="panel-title-row">
          <div>
            <p class="eyebrow">Invite list</p>
            <h2>邀请码列表</h2>
          </div>
          <span>{{ formatNumber(inviteTotal) }} 条记录</span>
        </div>

        <form class="admin-filter-bar invite-filter-bar" data-testid="invite-filter-form" @submit.prevent="applyInviteFilters">
          <label class="invite-search-wrap">
            <Search :size="16" />
            <input v-model="inviteFilters.q" data-testid="invite-search" class="text-input admin-search-field" placeholder="搜索邀请码、标签或备注" />
          </label>
          <ClickSelect v-model="inviteFilters.status" :options="inviteStatusOptions" data-testid="invite-status" class="text-input compact-input" aria-label="邀请码状态" compact />
          <button class="primary-button compact-button" type="submit">筛选</button>
          <button class="secondary-button compact-button icon-button-text" type="button" @click="exportInvites">
            <Download :size="16" />
            导出
          </button>
        </form>

        <div class="admin-table-scroll invite-table-scroll">
          <table class="data-table admin-data-table invite-data-table">
            <thead>
              <tr>
                <th>邀请码</th>
                <th>标签</th>
                <th>状态</th>
                <th>使用情况</th>
                <th>有效期</th>
                <th>备注</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="invite in invites" :key="invite.id">
                <td>
                  <strong class="invite-code">{{ invite.code }}</strong>
                  <small>#{{ invite.id }}</small>
                </td>
                <td>{{ invite.label || '运营后台' }}</td>
                <td>
                  <span class="status-pill" :class="inviteStateClass(invite)">{{ inviteStateText(invite) }}</span>
                </td>
                <td>
                  <strong>{{ formatNumber(invite.used_quota) }} / {{ formatNumber(invite.total_quota) }}</strong>
                  <small>剩余 {{ formatNumber(remainingQuota(invite)) }}</small>
                </td>
                <td>{{ formatDateOnly(invite.expires_at) }}</td>
                <td>{{ invite.notes || '-' }}</td>
                <td>
                  <div class="table-icon-actions">
                    <button :data-testid="`invite-copy-${invite.id}`" class="mini-button" type="button" title="复制" @click="copyInvite(invite)">
                      <Copy :size="15" />
                    </button>
                    <button class="mini-button" type="button" title="查看" @click="showInvite(invite)">
                      <Eye :size="15" />
                    </button>
                    <button :data-testid="`invite-toggle-${invite.id}`" class="mini-button" type="button" @click="toggleInvite(invite)">
                      {{ invite.status === 'active' ? '停用' : '启用' }}
                    </button>
                    <button class="mini-button" type="button" title="更多">
                      <MoreHorizontal :size="15" />
                    </button>
                  </div>
                </td>
              </tr>
              <tr v-if="!loadingInvites && invites.length === 0">
                <td colspan="7">暂无邀请码</td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-if="loadingInvites" class="page-status">加载中...</p>
        <div class="admin-pagination" data-testid="invites-pagination">
          <span>第 {{ inviteRangeStart }}-{{ inviteRangeEnd }} 条 / 共 {{ formatNumber(inviteTotal) }} 条</span>
          <div class="table-icon-actions">
            <button class="mini-button" type="button" :disabled="loadingInvites || invitePage <= 1" @click="goToInvitePage(invitePage - 1)">
              <ChevronLeft :size="15" />
              上一页
            </button>
            <button class="mini-button" type="button" :disabled="loadingInvites || invitePage >= inviteTotalPages" @click="goToInvitePage(invitePage + 1)">
              下一页
              <ChevronRight :size="15" />
            </button>
          </div>
        </div>
      </article>
    </div>

    <article class="admin-panel invite-redemptions-panel">
      <div class="panel-title-row">
        <div>
          <p class="eyebrow">Invite records</p>
          <h2>邀请记录</h2>
        </div>
        <span>{{ formatNumber(redemptionTotal) }} 条记录</span>
      </div>

      <form class="admin-filter-bar invite-filter-bar" data-testid="redemption-filter-form" @submit.prevent="applyRedemptionFilters">
        <input v-model="redemptionFilters.start_date" class="text-input compact-input" type="date" aria-label="开始日期" />
        <input v-model="redemptionFilters.end_date" class="text-input compact-input" type="date" aria-label="结束日期" />
        <ClickSelect v-model="redemptionFilters.result" :options="redemptionResultOptions" data-testid="redemption-result" class="text-input compact-input" aria-label="转化结果" compact />
        <button class="primary-button compact-button" type="submit">筛选</button>
        <button class="secondary-button compact-button icon-button-text" type="button" @click="exportRedemptions">
          <Download :size="16" />
          导出
        </button>
      </form>

      <div class="admin-table-scroll invite-table-scroll">
        <table class="data-table admin-data-table redemption-data-table">
          <thead>
            <tr>
              <th>邀请人</th>
              <th>新用户</th>
              <th>邮箱</th>
              <th>注册时间</th>
              <th>转化结果</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in redemptions" :key="item.id">
              <td>
                <strong>{{ item.inviter_name || '运营后台' }}</strong>
                <small>{{ item.invite_code }}</small>
              </td>
              <td>
                <div class="invite-user-cell">
                  <span><Users :size="15" /></span>
                  <div>
                    <strong>{{ item.display_name || item.username }}</strong>
                    <small>{{ item.username }} · 用户 {{ item.user_id }}</small>
                  </div>
                </div>
              </td>
              <td>{{ item.email || '-' }}</td>
              <td>{{ formatDate(item.registered_at) }}</td>
              <td>
                <span class="status-pill" :class="redemptionResultClass(item.conversion_result)">
                  {{ redemptionResultText(item.conversion_result) }}
                </span>
              </td>
            </tr>
            <tr v-if="!loadingRedemptions && redemptions.length === 0">
              <td colspan="5">暂无邀请记录</td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-if="loadingRedemptions" class="page-status">加载中...</p>
      <div class="admin-pagination" data-testid="redemptions-pagination">
        <span>第 {{ redemptionRangeStart }}-{{ redemptionRangeEnd }} 条 / 共 {{ formatNumber(redemptionTotal) }} 条</span>
        <div class="table-icon-actions">
          <button class="mini-button" type="button" :disabled="loadingRedemptions || redemptionPage <= 1" @click="goToRedemptionPage(redemptionPage - 1)">
            <ChevronLeft :size="15" />
            上一页
          </button>
          <button class="mini-button" type="button" :disabled="loadingRedemptions || redemptionPage >= redemptionTotalPages" @click="goToRedemptionPage(redemptionPage + 1)">
            下一页
            <ChevronRight :size="15" />
          </button>
        </div>
      </div>
    </article>

    <p v-if="actionMessage" class="status-success">{{ actionMessage }}</p>
    <p v-if="errorMessage" class="status-error">{{ errorMessage }}</p>
  </section>
</template>

<style scoped>
.admin-invites-page {
  display: grid;
  gap: 16px;
}

.section-heading span,
.panel-title-row span {
  color: #667085;
  font-size: 0.9rem;
}

.invite-heading-actions,
.invite-search-wrap,
.invite-user-cell,
.invite-filter-bar {
  display: flex;
  align-items: center;
}

.invite-heading-actions {
  justify-content: flex-end;
}

.invite-kpi-card {
  min-height: 116px;
}

.invite-workspace-grid {
  display: grid;
  grid-template-columns: minmax(280px, 340px) minmax(0, 1fr);
  gap: 16px;
  align-items: start;
}

.invite-generator-panel {
  display: grid;
  gap: 14px;
}

.invite-generator-panel label {
  display: grid;
  gap: 7px;
  color: #475467;
  font-size: 0.86rem;
  font-weight: 850;
}

.invite-generator-panel .text-input {
  min-height: 42px;
}

.invite-list-panel,
.invite-redemptions-panel {
  overflow: hidden;
}

.invite-filter-bar {
  flex-wrap: wrap;
  gap: 10px;
  margin-bottom: 12px;
}

.invite-search-wrap {
  flex: 1;
  min-width: min(100%, 280px);
  gap: 8px;
  min-height: 42px;
  padding: 0 13px;
  border: 1px solid rgba(118, 129, 166, 0.14);
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.78);
  color: #7a8497;
}

.invite-search-wrap .text-input {
  min-height: 38px;
  padding: 0;
  border: 0;
  background: transparent;
  box-shadow: none;
}

.invite-data-table {
  min-width: 1040px;
}

.redemption-data-table {
  min-width: 920px;
}

.invite-code {
  color: #1d2939;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  letter-spacing: 0;
}

.invite-data-table small,
.redemption-data-table small {
  display: block;
  margin-top: 3px;
  color: #8a94a6;
  font-size: 0.75rem;
}

.invite-status-available {
  background: rgba(34, 197, 94, 0.12);
  color: #14804a;
}

.invite-status-partial {
  background: rgba(59, 130, 246, 0.13);
  color: #1d4ed8;
}

.invite-status-used {
  background: rgba(99, 102, 241, 0.13);
  color: #4338ca;
}

.invite-status-expired {
  background: rgba(245, 158, 11, 0.16);
  color: #92400e;
}

.invite-status-disabled {
  background: rgba(107, 114, 128, 0.1);
  color: #667085;
}

.invite-user-cell {
  gap: 10px;
  min-width: 0;
}

.invite-user-cell > span {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 10px;
  background: rgba(37, 99, 235, 0.1);
  color: #1d4ed8;
}

.invite-user-cell div {
  min-width: 0;
}

.invite-user-cell strong,
.invite-user-cell small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 1180px) {
  .invite-workspace-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 860px) {
  .invite-heading-actions,
  .invite-heading-actions button,
  .invite-filter-bar .compact-input,
  .invite-filter-bar .compact-button,
  .invite-generator-panel button {
    width: 100%;
  }

  .invite-filter-bar {
    align-items: stretch;
  }

  .invite-data-table {
    min-width: 980px;
  }

  .redemption-data-table {
    min-width: 820px;
  }
}
</style>
