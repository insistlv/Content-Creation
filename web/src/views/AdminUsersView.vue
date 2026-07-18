<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import {
  Activity,
  ArrowDown,
  ArrowUp,
  CalendarPlus,
  ChevronLeft,
  ChevronRight,
  Coins,
  Eye,
  KeyRound,
  Link,
  Minus,
  Plus,
  ReceiptText,
  RefreshCw,
  Search,
  ShieldCheck,
  Trash2,
  Unlink,
  Users,
  WalletCards,
  X
} from 'lucide-vue-next'

import { api, getCurrentAdminSession } from '../api/client.js'
import ClickSelect from '../components/ClickSelect.vue'

const users = ref([])
const transactions = ref([])
const loadingUsers = ref(false)
const loadingTransactions = ref(false)
const savingCredits = ref(false)
const savingWechat = ref(false)
const savingPhone = ref(false)
const savingPassword = ref(false)
const deletingUsers = ref(false)
const selectedUserIds = ref([])
const errorMessage = ref('')
const transactionError = ref('')
const wechatMessage = ref('')
const wechatError = ref('')
const creditAdjustmentError = ref('')
const creditAdjustmentToast = reactive({
  open: false,
  message: ''
})
const passwordResetMessage = ref('')
const passwordResetError = ref('')
const successMessage = ref('')
const page = ref(1)
const pageSize = 10
const total = ref(0)
const transactionPageSize = 8
const creditMode = ref('add')
const detailUserId = ref('')
const adminPermissions = ref([])
const wechatBindingSection = ref(null)
const creditAdjustmentSection = ref(null)
const passwordResetSection = ref(null)

const filters = reactive({
  q: '',
  role: '',
  status: 'all'
})
const userSort = reactive({
  sortBy: '',
  sortDir: ''
})
const creditForm = reactive({
  userId: '',
  amount: 20,
  note: ''
})
const wechatForm = reactive({
  userId: '',
  openid: '',
  note: ''
})
const passwordResetForm = reactive({
  password: '',
  confirmPassword: ''
})
const summary = reactive({
  users_total: 0,
  active_users: 0,
  online_users: 0,
  today_new_users: 0,
  total_credits: 0,
  total_manual_topup: 0,
  users_total_delta_percent: 0,
  active_users_delta_percent: 0,
  today_new_users_delta_percent: 0,
  total_credits_delta_percent: 0,
  users_total_sparkline: [],
  active_users_sparkline: [],
  today_new_users_sparkline: [],
  total_credits_sparkline: []
})

const roleOptions = [
  { value: '', label: '全部角色' },
  { value: 'standard_user', label: '普通用户' },
  { value: 'standard_admin', label: '普通管理员' },
  { value: 'operations_admin', label: '运营管理员' },
  { value: 'content_reviewer', label: '内容审核' },
  { value: 'super_admin', label: '超级管理员' }
]

const statusOptions = [
  { value: 'all', label: '全部状态' },
  { value: 'active', label: '正常' },
  { value: 'disabled', label: '停用' },
  { value: 'online', label: '在线' },
  { value: 'offline', label: '离线' }
]

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))
const rangeStart = computed(() => (total.value === 0 ? 0 : (page.value - 1) * pageSize + 1))
const rangeEnd = computed(() => Math.min(total.value, page.value * pageSize))
const detailUser = computed(() => users.value.find((user) => `${user.user_id}` === `${detailUserId.value}`))
const selectedUser = computed(() => users.value.find((user) => `${user.user_id}` === `${creditForm.userId}`))
const selectedWechatUser = computed(() => users.value.find((user) => `${user.user_id}` === `${wechatForm.userId}`))
const allVisibleUsersSelected = computed(() => users.value.length > 0 && users.value.every((user) => selectedUserIds.value.includes(user.user_id)))
const canResetUserPassword = computed(() => adminPermissions.value.includes('users.password.reset'))
const creditActionText = computed(() => (creditMode.value === 'add' ? '确认加点' : '确认扣点'))
const creditPanelTitle = computed(() => (creditMode.value === 'add' ? '手动加点' : '手动扣点'))
const detailTransactions = computed(() => {
  if (!detailUser.value) return []
  return transactions.value.filter((item) => `${item.user_id}` === `${detailUser.value.user_id}`)
})
const kpiCards = computed(() => [
  {
    key: 'users_total',
    label: '用户总数',
    value: summary.users_total,
    delta: summary.users_total_delta_percent,
    chart: sparklineChart(summary.users_total_sparkline),
    icon: Users
  },
  {
    key: 'today_new_users',
    label: '今日新增',
    value: summary.today_new_users,
    delta: summary.today_new_users_delta_percent,
    chart: sparklineChart(summary.today_new_users_sparkline),
    icon: CalendarPlus
  },
  {
    key: 'online_users',
    label: '实时在线',
    value: summary.online_users,
    delta: 0,
    chart: sparklineChart(stableRealtimeSparkline(summary.online_users)),
    hint: '近 5 分钟',
    icon: Activity
  },
  {
    key: 'total_credits',
    label: '剩余总点数',
    value: summary.total_credits,
    delta: summary.total_credits_delta_percent,
    chart: sparklineChart(summary.total_credits_sparkline),
    icon: WalletCards
  }
])

const SPARKLINE_WIDTH = 120
const SPARKLINE_HEIGHT = 38
const SPARKLINE_PADDING_X = 5
const SPARKLINE_PADDING_TOP = 5
const SPARKLINE_PADDING_BOTTOM = 6
const SPARKLINE_FALLBACK_COUNT = 7
const sparklineViewBox = `0 0 ${SPARKLINE_WIDTH} ${SPARKLINE_HEIGHT}`
const USERS_AUTO_REFRESH_MS = 60_000
const CREDIT_ADJUSTMENT_TOAST_MS = 2_500
let usersRefreshTimer = null
let creditAdjustmentToastTimer = null

function formatNumber(value) {
  return new Intl.NumberFormat('zh-CN').format(Number(value ?? 0))
}

function formatDate(value) {
  if (!value) return '-'
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  }).format(new Date(value))
}

function formatDelta(value) {
  const number = Number(value ?? 0)
  const text = Number.isInteger(number) ? number.toFixed(0) : number.toFixed(1)
  return `${number > 0 ? '+' : ''}${text}%`
}

function roundSparklineCoordinate(value) {
  return Number(value.toFixed(2))
}

function normalizeSparklineValues(values = []) {
  const source = Array.isArray(values) ? values : []
  const numbers = source.map((value) => {
    const number = Number(value ?? 0)
    return Number.isFinite(number) ? number : 0
  })

  return numbers.length > 0 ? numbers : Array.from({ length: SPARKLINE_FALLBACK_COUNT }, () => 0)
}

function sparklineChart(values = []) {
  const numbers = normalizeSparklineValues(values)
  const min = Math.min(...numbers)
  const max = Math.max(...numbers)
  const range = max - min
  const baseline = SPARKLINE_HEIGHT - SPARKLINE_PADDING_BOTTOM
  const chartHeight = SPARKLINE_HEIGHT - SPARKLINE_PADDING_TOP - SPARKLINE_PADDING_BOTTOM
  const xStep = numbers.length > 1 ? (SPARKLINE_WIDTH - SPARKLINE_PADDING_X * 2) / (numbers.length - 1) : 0
  const points = numbers.map((value, index) => {
    const x = numbers.length > 1 ? SPARKLINE_PADDING_X + xStep * index : SPARKLINE_WIDTH / 2
    const ratio = range === 0 ? 0.5 : (value - min) / range
    const y = baseline - ratio * chartHeight

    return {
      index,
      value,
      x: roundSparklineCoordinate(x),
      y: roundSparklineCoordinate(y)
    }
  })
  const linePoints = points.map((point) => `${point.x},${point.y}`).join(' ')
  const areaPath = [
    `M ${points[0].x} ${baseline}`,
    `L ${points.map((point) => `${point.x} ${point.y}`).join(' L ')}`,
    `L ${points[points.length - 1].x} ${baseline}`,
    'Z'
  ].join(' ')

  return {
    points,
    linePoints,
    areaPath
  }
}

function stableRealtimeSparkline(value) {
  return Array.from({ length: SPARKLINE_FALLBACK_COUNT }, () => Number(value ?? 0))
}

function sparklinePointTitle(label, point) {
  return `${label} 第 ${point.index + 1} 天：${formatNumber(point.value)}`
}

function statusText(status) {
  return status === 'active' ? '正常' : '停用'
}

function presenceText(user) {
  return user.online ? '在线' : '离线'
}

function wechatOpenID(user) {
  return `${user?.wechat_open_id || user?.wechat_binding?.openid || ''}`.trim()
}

function wechatBound(user) {
  return Boolean(user?.wechat_bound || user?.wechat_binding?.bound || wechatOpenID(user))
}

function phoneBound(user) {
  return Boolean(`${user?.phone || ''}`.trim())
}

function roleClass(role) {
  return `role-${role?.color || 'blue'}`
}

function roleText(role) {
  return role?.name || '普通用户'
}

function isUserSortActive(sortBy) {
  return userSort.sortBy === sortBy
}

function userSortAria(sortBy) {
  if (!isUserSortActive(sortBy)) return 'none'
  return userSort.sortDir === 'asc' ? 'ascending' : 'descending'
}

function userSortButtonLabel(sortBy, label) {
  if (!isUserSortActive(sortBy)) return `按${label}降序排序`
  return userSort.sortDir === 'desc' ? `按${label}升序排序` : `按${label}降序排序`
}

async function toggleUserSort(sortBy) {
  if (isUserSortActive(sortBy)) {
    userSort.sortDir = userSort.sortDir === 'desc' ? 'asc' : 'desc'
  } else {
    userSort.sortBy = sortBy
    userSort.sortDir = 'desc'
  }
  page.value = 1
  await loadUsers()
}

function avatarInitial(user) {
  const text = user.display_name || user.username || user.account || 'U'
  return text.slice(0, 1).toUpperCase()
}

function displayName(user) {
  return user.display_name || user.username || `用户 #${user.user_id}`
}

function transactionTypeText(type) {
  if (type === 'manual_topup') return '人工加点'
  if (type === 'manual_deduct') return '人工扣点'
  return '生成扣点'
}

function transactionNote(item) {
  return item.admin_note || item.reason || '-'
}

function userParams() {
  const params = {
    page: page.value,
    page_size: pageSize,
    q: filters.q.trim(),
    role: filters.role,
    status: filters.status
  }
  if (userSort.sortBy && userSort.sortDir) {
    params.sort_by = userSort.sortBy
    params.sort_dir = userSort.sortDir
  }
  return params
}

function clearCreditAdjustmentFeedback() {
  creditAdjustmentError.value = ''
}

function clearCreditAdjustmentToastTimer() {
  if (creditAdjustmentToastTimer) {
    window.clearTimeout(creditAdjustmentToastTimer)
    creditAdjustmentToastTimer = null
  }
}

function hideCreditAdjustmentToast() {
  clearCreditAdjustmentToastTimer()
  creditAdjustmentToast.open = false
  creditAdjustmentToast.message = ''
}

function showCreditAdjustmentToast(message) {
  clearCreditAdjustmentToastTimer()
  creditAdjustmentToast.message = message
  creditAdjustmentToast.open = true
  creditAdjustmentToastTimer = window.setTimeout(() => {
    hideCreditAdjustmentToast()
  }, CREDIT_ADJUSTMENT_TOAST_MS)
}

async function loadAdminPermissions() {
  try {
    const payload = await getCurrentAdminSession()
    adminPermissions.value = payload?.permissions ?? []
  } catch {
    adminPermissions.value = []
  }
}

async function loadUsers() {
  loadingUsers.value = true
  errorMessage.value = ''
  try {
    const payload = await api.listAdminUsers(userParams())
    users.value = payload.items ?? []
    total.value = payload.total ?? users.value.length
    page.value = payload.page ?? page.value
    Object.assign(summary, payload.summary ?? {})
    selectedUserIds.value = selectedUserIds.value.filter((id) => users.value.some((user) => user.user_id === id))
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    loadingUsers.value = false
  }
}

async function loadTransactions() {
  loadingTransactions.value = true
  transactionError.value = ''
  try {
    const payload = await api.listAdminCreditTransactions({ page: 1, page_size: transactionPageSize })
    transactions.value = payload.items ?? []
  } catch (error) {
    transactionError.value = error.message
  } finally {
    loadingTransactions.value = false
  }
}

async function load() {
  await Promise.all([loadUsers(), loadTransactions()])
}

function pageIsVisible() {
  return typeof document === 'undefined' || document.visibilityState !== 'hidden'
}

function stopUsersAutoRefresh() {
  if (usersRefreshTimer) {
    window.clearInterval(usersRefreshTimer)
    usersRefreshTimer = null
  }
}

function startUsersAutoRefresh() {
  stopUsersAutoRefresh()
  if (!pageIsVisible()) return
  usersRefreshTimer = window.setInterval(() => {
    if (pageIsVisible()) {
      void loadUsers()
    }
  }, USERS_AUTO_REFRESH_MS)
}

function handleVisibilityChange() {
  if (pageIsVisible()) {
    void loadUsers()
    startUsersAutoRefresh()
    return
  }
  stopUsersAutoRefresh()
}

async function applyFilters() {
  page.value = 1
  selectedUserIds.value = []
  await loadUsers()
}

async function goToPage(nextPage) {
  page.value = Math.min(Math.max(nextPage, 1), totalPages.value)
  await loadUsers()
}

function prepareCreditForm(user, mode = 'add') {
  creditMode.value = mode
  creditForm.userId = `${user.user_id}`
  creditForm.note = mode === 'add' ? `为 ${user.username} 手动加点` : `为 ${user.username} 手动扣点`
  clearCreditAdjustmentFeedback()
}

function setCreditMode(mode) {
  creditMode.value = mode
  clearCreditAdjustmentFeedback()
}

function prepareWechatForm(user) {
  wechatError.value = ''
  wechatMessage.value = ''
  wechatForm.userId = `${user.user_id}`
  wechatForm.openid = wechatOpenID(user)
  wechatForm.note = wechatBound(user) ? `核验 ${user.username} 微信绑定` : `为 ${user.username} 绑定微信 OpenID`
}

function resetPasswordResetForm() {
  passwordResetForm.password = ''
  passwordResetForm.confirmPassword = ''
  passwordResetMessage.value = ''
  passwordResetError.value = ''
}

function focusDetailSection(section) {
  nextTick(() => {
    const target =
      section === 'credit'
        ? creditAdjustmentSection.value
        : section === 'wechat'
          ? wechatBindingSection.value
          : section === 'password'
            ? passwordResetSection.value
            : null
    target?.scrollIntoView?.({ block: 'nearest', behavior: 'smooth' })
    if (section === 'wechat' && typeof document !== 'undefined') {
      document.getElementById('wechatOpenID')?.focus()
    }
    if (section === 'password' && typeof document !== 'undefined') {
      document.getElementById('userResetPasswordNew')?.focus()
    }
  })
}

function openUserDetail(user, section = 'overview') {
  detailUserId.value = `${user.user_id}`
  prepareWechatForm(user)
  prepareCreditForm(user)
  resetPasswordResetForm()
  errorMessage.value = ''
  focusDetailSection(section)
}

function closeUserDetail() {
  detailUserId.value = ''
}

function openWechatBinding(user) {
  openUserDetail(user, 'wechat')
}

function openPasswordReset(user) {
  openUserDetail(user, 'password')
}

function setUserSelected(id, checked) {
  if (checked) {
    if (!selectedUserIds.value.includes(id)) {
      selectedUserIds.value = [...selectedUserIds.value, id]
    }
    return
  }
  selectedUserIds.value = selectedUserIds.value.filter((itemId) => itemId !== id)
}

function setVisibleUsersSelected(checked) {
  const visibleIds = users.value.map((user) => user.user_id)
  if (checked) {
    selectedUserIds.value = Array.from(new Set([...selectedUserIds.value, ...visibleIds]))
    return
  }
  selectedUserIds.value = selectedUserIds.value.filter((id) => !visibleIds.includes(id))
}

async function deleteUser(user) {
  const label = displayName(user)
  if (typeof window !== 'undefined' && window.confirm && !window.confirm(`删除用户「${label}」？删除后该用户将无法登录。`)) {
    return
  }
  deletingUsers.value = true
  errorMessage.value = ''
  successMessage.value = ''
  try {
    await api.deleteAdminUser(user.user_id)
    selectedUserIds.value = selectedUserIds.value.filter((id) => id !== user.user_id)
    successMessage.value = '用户已删除'
    await loadUsers()
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    deletingUsers.value = false
  }
}

async function batchDeleteUsers() {
  if (selectedUserIds.value.length === 0) {
    errorMessage.value = '请先选择用户'
    return
  }
  const deletingIds = [...selectedUserIds.value]
  if (typeof window !== 'undefined' && window.confirm && !window.confirm(`删除选中的 ${deletingIds.length} 个用户？删除后这些用户将无法登录。`)) {
    return
  }
  deletingUsers.value = true
  errorMessage.value = ''
  successMessage.value = ''
  try {
    await api.batchDeleteAdminUsers(deletingIds)
    selectedUserIds.value = []
    successMessage.value = '已批量删除用户'
    await loadUsers()
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    deletingUsers.value = false
  }
}

async function submitAdjustment() {
  const userId = Number(creditForm.userId)
  const amount = Number(creditForm.amount)
  clearCreditAdjustmentFeedback()
  errorMessage.value = ''
  if (!userId || amount <= 0) {
    creditAdjustmentError.value = '请选择用户并输入有效点数'
    return
  }
  savingCredits.value = true
  const userLabel = selectedUser.value ? displayName(selectedUser.value) : `用户 #${userId}`
  const actionText = creditMode.value === 'add' ? '加点' : '扣点'
  try {
    const payload = await api.adjustAdminCredits(userId, {
      type: creditMode.value,
      amount,
      note: creditForm.note.trim() || (creditMode.value === 'add' ? '后台手动加点' : '后台手动扣点')
    })
    const successMessage =
      creditMode.value === 'add'
        ? `${actionText}成功！${userLabel} 当前剩余 ${formatNumber(payload?.available_credits)} 点`
        : `${actionText}成功，${userLabel} 当前剩余 ${formatNumber(payload?.available_credits)} 点`
    showCreditAdjustmentToast(successMessage)
    closeUserDetail()
    await load()
  } catch (error) {
    creditAdjustmentError.value = error.message
  } finally {
    savingCredits.value = false
  }
}

async function submitWechatBinding() {
  const userId = Number(wechatForm.userId)
  const openid = wechatForm.openid.trim()
  if (!userId || !openid) {
    wechatError.value = '请选择用户并填写微信 OpenID'
    return
  }
  savingWechat.value = true
  wechatError.value = ''
  wechatMessage.value = ''
  try {
    await api.updateAdminUserWechatBinding(userId, {
      openid,
      note: wechatForm.note.trim()
    })
    wechatMessage.value = '微信绑定已更新'
    await loadUsers()
  } catch (error) {
    wechatError.value = error.message
  } finally {
    savingWechat.value = false
  }
}

async function unbindWechat() {
  const userId = Number(wechatForm.userId)
  if (!userId) {
    wechatError.value = '请选择用户'
    return
  }
  savingWechat.value = true
  wechatError.value = ''
  wechatMessage.value = ''
  try {
    await api.deleteAdminUserWechatBinding(userId, {
      note: wechatForm.note.trim()
    })
    wechatForm.openid = ''
    wechatMessage.value = '微信绑定已解绑'
    await loadUsers()
  } catch (error) {
    wechatError.value = error.message
  } finally {
    savingWechat.value = false
  }
}

async function unbindPhone(user) {
  if (!user || !phoneBound(user)) {
    return
  }
  const phone = `${user.phone}`.trim()
  if (typeof window !== 'undefined' && window.confirm && !window.confirm(`解绑用户「${displayName(user)}」的手机号 ${phone}？解绑后该用户将无法使用手机号登录。`)) {
    return
  }
  savingPhone.value = true
  errorMessage.value = ''
  successMessage.value = ''
  try {
    await api.deleteAdminUserPhoneBinding(user.user_id, {
      note: '后台解绑手机号'
    })
    successMessage.value = '手机号已解绑'
    await loadUsers()
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    savingPhone.value = false
  }
}

async function submitUserPasswordReset() {
  if (!canResetUserPassword.value || !detailUser.value) {
    return
  }
  passwordResetError.value = ''
  passwordResetMessage.value = ''
  if (passwordResetForm.password.trim().length < 8) {
    passwordResetError.value = '新密码至少 8 位'
    return
  }
  if (passwordResetForm.password !== passwordResetForm.confirmPassword) {
    passwordResetError.value = '两次输入的新密码不一致'
    return
  }

  savingPassword.value = true
  try {
    await api.resetAdminUserPassword(detailUser.value.user_id, {
      password: passwordResetForm.password
    })
    passwordResetForm.password = ''
    passwordResetForm.confirmPassword = ''
    passwordResetMessage.value = '密码已重置，用户需要重新登录'
  } catch (error) {
    passwordResetError.value = error.message || '密码重置失败'
  } finally {
    savingPassword.value = false
  }
}

onMounted(() => {
  load()
  loadAdminPermissions()
  startUsersAutoRefresh()
  if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', handleVisibilityChange)
  }
})

onBeforeUnmount(() => {
  stopUsersAutoRefresh()
  clearCreditAdjustmentToastTimer()
  if (typeof document !== 'undefined') {
    document.removeEventListener('visibilitychange', handleVisibilityChange)
  }
})
</script>

<template>
  <section class="admin-users-page">
    <div
      v-if="creditAdjustmentToast.open"
      class="admin-credit-adjustment-toast"
      data-testid="credit-adjustment-toast"
      role="status"
      aria-live="polite"
    >
      {{ creditAdjustmentToast.message }}
    </div>

    <div class="admin-page-heading">
      <div>
        <p class="eyebrow">Users & credits</p>
        <h1>用户与点数</h1>
      </div>
      <div class="users-heading-actions">
        <button class="secondary-button icon-button-text" type="button" :disabled="deletingUsers" @click="load">
          <RefreshCw :size="16" />
          刷新
        </button>
      </div>
    </div>

    <div class="users-kpi-grid">
      <article v-for="card in kpiCards" :key="card.key" class="users-kpi-card">
        <div class="users-kpi-topline">
          <span><component :is="card.icon" :size="18" /> {{ card.label }}</span>
          <b class="users-kpi-delta" :class="{ negative: card.delta < 0 }">{{ formatDelta(card.delta) }}</b>
        </div>
        <strong>{{ formatNumber(card.value) }}</strong>
        <small v-if="card.hint" class="users-kpi-hint">{{ card.hint }}</small>
        <div class="users-sparkline">
          <svg
            class="users-sparkline-chart"
            :viewBox="sparklineViewBox"
            preserveAspectRatio="none"
            role="img"
            :aria-label="`${card.label} 最近 7 日趋势`"
          >
            <path class="users-sparkline-area" :d="card.chart.areaPath" />
            <polyline class="users-sparkline-line" :points="card.chart.linePoints" />
            <circle
              v-for="point in card.chart.points"
              :key="`${card.key}-${point.index}`"
              class="users-sparkline-point"
              :cx="point.x"
              :cy="point.y"
              r="2.5"
              tabindex="0"
              :aria-label="sparklinePointTitle(card.label, point)"
            >
              <title>{{ sparklinePointTitle(card.label, point) }}</title>
            </circle>
          </svg>
        </div>
      </article>
    </div>

    <div class="admin-users-workspace">
      <div class="admin-panel users-table-panel">
        <form class="admin-filter-bar users-filter-bar" data-testid="admin-users-filter" @submit.prevent="applyFilters">
          <label class="admin-search-field users-search-field">
            <Search :size="17" />
            <input
              v-model="filters.q"
              data-testid="admin-users-search"
              type="search"
              placeholder="搜索用户、账号、手机号或邮箱"
            />
          </label>
          <ClickSelect v-model="filters.role" :options="roleOptions" data-testid="admin-users-role" class="text-input compact-input" aria-label="角色" compact />
          <ClickSelect v-model="filters.status" :options="statusOptions" data-testid="admin-users-status" class="text-input compact-input" aria-label="状态" compact />
          <button class="primary-button compact-button" type="submit">筛选</button>
        </form>

        <div v-if="selectedUserIds.length > 0" class="users-bulk-toolbar" data-testid="users-bulk-toolbar">
          <span>已选择 {{ selectedUserIds.length }} 个用户</span>
          <div class="inline-actions">
            <button class="mini-button destructive-button" data-testid="bulk-delete-users" type="button" :disabled="deletingUsers || selectedUserIds.length === 0" @click="batchDeleteUsers">
              {{ deletingUsers ? '删除中...' : '批量删除' }}
            </button>
          </div>
        </div>

        <div class="admin-table-scroll users-table-scroll">
          <table class="data-table admin-data-table users-data-table">
            <thead>
              <tr>
                <th class="user-select-column">
                  <input
                    type="checkbox"
                    data-testid="select-visible-users"
                    :checked="allVisibleUsersSelected"
                    aria-label="选择当前页用户"
                    @change="setVisibleUsersSelected($event.target.checked)"
                  />
                </th>
                <th>用户</th>
                <th>账号</th>
                <th>角色</th>
                <th>
                  <button
                    class="table-sort-button"
                    :class="{ active: isUserSortActive('available_credits') }"
                    data-testid="admin-users-sort-available-credits"
                    type="button"
                    :aria-sort="userSortAria('available_credits')"
                    :aria-label="userSortButtonLabel('available_credits', '剩余点数')"
                    @click="toggleUserSort('available_credits')"
                  >
                    <span>剩余点数</span>
                    <span class="table-sort-icon-slot" aria-hidden="true">
                      <ArrowDown v-if="isUserSortActive('available_credits') && userSort.sortDir === 'desc'" :size="13" />
                      <ArrowUp v-else-if="isUserSortActive('available_credits')" :size="13" />
                    </span>
                  </button>
                </th>
                <th>
                  <button
                    class="table-sort-button"
                    :class="{ active: isUserSortActive('total_recharged') }"
                    data-testid="admin-users-sort-total-recharged"
                    type="button"
                    :aria-sort="userSortAria('total_recharged')"
                    :aria-label="userSortButtonLabel('total_recharged', '累计充值')"
                    @click="toggleUserSort('total_recharged')"
                  >
                    <span>累计充值</span>
                    <span class="table-sort-icon-slot" aria-hidden="true">
                      <ArrowDown v-if="isUserSortActive('total_recharged') && userSort.sortDir === 'desc'" :size="13" />
                      <ArrowUp v-else-if="isUserSortActive('total_recharged')" :size="13" />
                    </span>
                  </button>
                </th>
                <th>
                  <button
                    class="table-sort-button"
                    :class="{ active: isUserSortActive('last_login_at') }"
                    data-testid="admin-users-sort-last-login-at"
                    type="button"
                    :aria-sort="userSortAria('last_login_at')"
                    :aria-label="userSortButtonLabel('last_login_at', '最近登录')"
                    @click="toggleUserSort('last_login_at')"
                  >
                    <span>最近登录</span>
                    <span class="table-sort-icon-slot" aria-hidden="true">
                      <ArrowDown v-if="isUserSortActive('last_login_at') && userSort.sortDir === 'desc'" :size="13" />
                      <ArrowUp v-else-if="isUserSortActive('last_login_at')" :size="13" />
                    </span>
                  </button>
                </th>
                <th>
                  <button
                    class="table-sort-button"
                    :class="{ active: isUserSortActive('presence') }"
                    data-testid="admin-users-sort-presence"
                    type="button"
                    :aria-sort="userSortAria('presence')"
                    :aria-label="userSortButtonLabel('presence', '状态')"
                    @click="toggleUserSort('presence')"
                  >
                    <span>状态</span>
                    <span class="table-sort-icon-slot" aria-hidden="true">
                      <ArrowDown v-if="isUserSortActive('presence') && userSort.sortDir === 'desc'" :size="13" />
                      <ArrowUp v-else-if="isUserSortActive('presence')" :size="13" />
                    </span>
                  </button>
                </th>
                <th>微信绑定</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in users" :key="item.user_id">
                <td class="user-select-column">
                  <input
                    type="checkbox"
                    :data-testid="`select-user-${item.user_id}`"
                    :checked="selectedUserIds.includes(item.user_id)"
                    :aria-label="`选择用户 ${displayName(item)}`"
                    @change="setUserSelected(item.user_id, $event.target.checked)"
                  />
                </td>
                <td>
                  <div class="user-cell">
                    <img v-if="item.avatar_url" :src="item.avatar_url" :alt="displayName(item)" />
                    <span v-else>{{ avatarInitial(item) }}</span>
                    <div>
                      <strong>{{ displayName(item) }}</strong>
                      <small>#{{ item.user_id }}</small>
                    </div>
                  </div>
                </td>
                <td>
                  <strong>{{ item.account || item.username }}</strong>
                  <small>{{ item.phone || '未绑定手机号' }}</small>
                  <small>{{ item.email || '未绑定邮箱' }}</small>
                </td>
                <td>
                  <span class="role-pill" :class="roleClass(item.role)">
                    <ShieldCheck :size="13" />
                    {{ roleText(item.role) }}
                  </span>
                </td>
                <td><strong>{{ formatNumber(item.available_credits) }}</strong></td>
                <td>{{ formatNumber(item.total_recharged) }}</td>
                <td>{{ formatDate(item.last_login_at) }}</td>
                <td>
                  <div class="status-stack">
                    <span class="status-pill" :class="`status-${item.status}`">{{ statusText(item.status) }}</span>
                    <span class="presence-pill" :class="{ offline: !item.online }">{{ presenceText(item) }}</span>
                  </div>
                </td>
                <td>
                  <button
                    class="wechat-binding-button"
                    :class="{ bound: wechatBound(item) }"
                    type="button"
                    :data-testid="`open-wechat-binding-${item.user_id}`"
                    @click="openWechatBinding(item)"
                  >
                    <Link v-if="wechatBound(item)" :size="14" />
                    <Unlink v-else :size="14" />
                    <span>{{ wechatBound(item) ? '已绑定' : '未绑定' }}</span>
                  </button>
                  <small class="wechat-openid-preview">{{ wechatOpenID(item) || '暂无 OpenID' }}</small>
                </td>
                <td>
                  <div class="table-icon-actions">
                    <button
                      class="mini-button icon-only"
                      type="button"
                      :data-testid="`view-user-${item.user_id}`"
                      aria-label="查看用户"
                      @click="openUserDetail(item)"
                    >
                      <Eye :size="15" />
                    </button>
                    <button
                      v-if="canResetUserPassword"
                      class="mini-button icon-only"
                      type="button"
                      :data-testid="`reset-user-password-${item.user_id}`"
                      aria-label="重置登录密码"
                      @click="openPasswordReset(item)"
                    >
                      <KeyRound :size="15" />
                    </button>
                    <button
                      class="mini-button icon-only destructive-button"
                      type="button"
                      :data-testid="`delete-user-${item.user_id}`"
                      aria-label="删除用户"
                      :disabled="deletingUsers"
                      @click="deleteUser(item)"
                    >
                      <Trash2 :size="15" />
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
          <p v-if="loadingUsers" class="page-status">加载中...</p>
          <p v-else-if="users.length === 0" class="page-status">暂无匹配用户</p>
        </div>

        <div class="admin-pagination">
          <span>第 {{ rangeStart }}-{{ rangeEnd }} 条 / 共 {{ total }} 条</span>
          <div class="inline-actions">
            <button class="mini-button" type="button" :disabled="loadingUsers || page <= 1" @click="goToPage(page - 1)">
              <ChevronLeft :size="15" />
              上一页
            </button>
            <button class="mini-button" type="button" :disabled="loadingUsers || page >= totalPages" @click="goToPage(page + 1)">
              下一页
              <ChevronRight :size="15" />
            </button>
          </div>
        </div>
      </div>

    </div>

    <div
      v-if="detailUser"
      class="admin-user-detail-backdrop"
      data-testid="admin-user-detail-modal"
      role="dialog"
      aria-modal="true"
      aria-labelledby="adminUserDetailTitle"
      @click.self="closeUserDetail"
    >
      <div class="admin-panel admin-user-detail-modal">
        <header class="user-detail-header">
          <div class="user-detail-identity">
            <img v-if="detailUser.avatar_url" :src="detailUser.avatar_url" :alt="displayName(detailUser)" />
            <span v-else>{{ avatarInitial(detailUser) }}</span>
            <div>
              <p class="eyebrow">User detail</p>
              <h2 id="adminUserDetailTitle">{{ displayName(detailUser) }}</h2>
              <small>#{{ detailUser.user_id }} · {{ detailUser.username }}</small>
            </div>
          </div>
          <button
            class="mini-button icon-only"
            data-testid="close-user-detail-modal"
            type="button"
            aria-label="关闭用户详情"
            @click="closeUserDetail"
          >
            <X :size="15" />
          </button>
        </header>

        <div class="user-detail-summary">
          <div>
            <span>账号</span>
            <strong>{{ detailUser.account || detailUser.username || '-' }}</strong>
          </div>
          <div>
            <span>手机号</span>
            <strong>{{ detailUser.phone || '未绑定手机号' }}</strong>
            <button
              v-if="phoneBound(detailUser)"
              class="mini-button destructive-button"
              data-testid="phone-binding-unbind"
              type="button"
              :disabled="savingPhone"
              @click="unbindPhone(detailUser)"
            >
              {{ savingPhone ? '解绑中...' : '解绑手机号' }}
            </button>
          </div>
          <div>
            <span>邮箱</span>
            <strong>{{ detailUser.email || '未绑定邮箱' }}</strong>
          </div>
          <div>
            <span>角色</span>
            <strong>{{ roleText(detailUser.role) }}</strong>
          </div>
          <div>
            <span>状态</span>
            <strong>{{ statusText(detailUser.status) }} · {{ presenceText(detailUser) }}</strong>
          </div>
          <div>
            <span>剩余点数</span>
            <strong>{{ formatNumber(detailUser.available_credits) }}</strong>
          </div>
          <div>
            <span>累计充值</span>
            <strong>{{ formatNumber(detailUser.total_recharged) }}</strong>
          </div>
          <div>
            <span>微信绑定</span>
            <strong>{{ wechatBound(detailUser) ? '已绑定' : '未绑定' }}</strong>
            <small>{{ wechatOpenID(detailUser) || '暂无 OpenID' }}</small>
          </div>
        </div>

        <div class="user-detail-grid">
          <form
            v-if="canResetUserPassword"
            ref="passwordResetSection"
            class="user-detail-section password-reset-panel"
            data-testid="user-password-reset-form"
            @submit.prevent="submitUserPasswordReset"
          >
            <div class="panel-title-row">
              <div>
                <p class="eyebrow">Login password</p>
                <h2>重置登录密码</h2>
              </div>
              <KeyRound :size="20" />
            </div>

            <label class="field-label" for="userResetPasswordNew">新密码</label>
            <input
              id="userResetPasswordNew"
              v-model="passwordResetForm.password"
              data-testid="user-reset-password-new"
              class="text-input"
              type="password"
              autocomplete="new-password"
              placeholder="至少 8 位"
            />

            <label class="field-label" for="userResetPasswordConfirm">确认新密码</label>
            <input
              id="userResetPasswordConfirm"
              v-model="passwordResetForm.confirmPassword"
              data-testid="user-reset-password-confirm"
              class="text-input"
              type="password"
              autocomplete="new-password"
              placeholder="再次输入新密码"
            />

            <button class="primary-button" type="submit" :disabled="savingPassword">
              {{ savingPassword ? '提交中...' : '重置密码' }}
            </button>
            <p v-if="passwordResetMessage" class="status-success">{{ passwordResetMessage }}</p>
            <p v-if="passwordResetError" class="status-error">{{ passwordResetError }}</p>
          </form>

          <form
            ref="wechatBindingSection"
            class="user-detail-section wechat-binding-panel"
            data-testid="wechat-binding-form"
            @submit.prevent="submitWechatBinding"
          >
            <div class="panel-title-row">
              <div>
                <p class="eyebrow">WeChat binding</p>
                <h2>微信绑定</h2>
              </div>
              <Link :size="20" />
            </div>

            <div v-if="selectedWechatUser" class="selected-credit-user wechat-selected-user">
              <span>{{ avatarInitial(selectedWechatUser) }}</span>
              <div>
                <strong>{{ displayName(selectedWechatUser) }}</strong>
                <small>{{ selectedWechatUser.username }} · {{ wechatBound(selectedWechatUser) ? '已绑定微信' : '未绑定微信' }}</small>
              </div>
            </div>

            <label class="field-label" for="wechatOpenID">微信 OpenID</label>
            <input
              id="wechatOpenID"
              v-model="wechatForm.openid"
              data-testid="wechat-binding-openid"
              class="text-input"
              maxlength="128"
              placeholder="填写微信 OpenID"
            />

            <label class="field-label" for="wechatBindingNote">操作备注</label>
            <textarea
              id="wechatBindingNote"
              v-model="wechatForm.note"
              data-testid="wechat-binding-note"
              class="text-input admin-textarea"
              rows="3"
              placeholder="记录修改原因，便于审计"
            />

            <div class="inline-actions">
              <button class="primary-button" type="submit" :disabled="savingWechat">
                {{ savingWechat ? '保存中...' : '保存绑定' }}
              </button>
              <button
                class="secondary-button"
                data-testid="wechat-binding-unbind"
                type="button"
                :disabled="savingWechat || !wechatForm.userId"
                @click="unbindWechat"
              >
                解绑
              </button>
            </div>
            <p v-if="wechatMessage" class="status-success">{{ wechatMessage }}</p>
            <p v-if="wechatError" class="status-error">{{ wechatError }}</p>
          </form>

          <form
            ref="creditAdjustmentSection"
            class="user-detail-section credit-form-panel credit-adjust-panel"
            data-testid="credit-adjustment-form"
            @submit.prevent="submitAdjustment"
          >
            <div class="panel-title-row">
              <div>
                <p class="eyebrow">Credit operation</p>
                <h2>{{ creditPanelTitle }}</h2>
              </div>
              <Coins :size="20" />
            </div>

            <div class="credit-mode-tabs" role="tablist" aria-label="点数操作类型">
              <button
                class="credit-mode-tab"
                :class="{ active: creditMode === 'add' }"
                data-testid="credit-tab-add"
                type="button"
                @click="setCreditMode('add')"
              >
                <Plus :size="15" />
                手动加点
              </button>
              <button
                class="credit-mode-tab"
                :class="{ active: creditMode === 'deduct' }"
                data-testid="credit-tab-deduct"
                type="button"
                @click="setCreditMode('deduct')"
              >
                <Minus :size="15" />
                手动扣点
              </button>
            </div>

            <div v-if="selectedUser" class="selected-credit-user">
              <span>{{ avatarInitial(selectedUser) }}</span>
              <div>
                <strong>{{ displayName(selectedUser) }}</strong>
                <small>{{ selectedUser.username }} · {{ formatNumber(selectedUser.available_credits) }} 点</small>
              </div>
            </div>

            <label class="field-label" for="creditAmount">点数</label>
            <input id="creditAmount" v-model.number="creditForm.amount" data-testid="credit-amount" class="text-input" type="number" min="1" />
            <label class="field-label" for="creditNote">备注</label>
            <textarea id="creditNote" v-model="creditForm.note" data-testid="credit-note" class="text-input admin-textarea" rows="3" />
            <button
              class="primary-button"
              data-testid="credit-adjustment-submit"
              type="submit"
              :disabled="savingCredits"
              :aria-busy="savingCredits ? 'true' : 'false'"
            >
              {{ savingCredits ? '处理中...' : creditActionText }}
            </button>
            <p
              v-if="creditAdjustmentError"
              class="status-error"
              data-testid="credit-adjustment-error"
              role="alert"
            >
              {{ creditAdjustmentError }}
            </p>
          </form>

          <section class="user-detail-section credit-log-panel">
            <div class="panel-title-row">
              <div>
                <p class="eyebrow">Credit log</p>
                <h2>最近点数变动</h2>
              </div>
              <button class="mini-button icon-only" type="button" aria-label="刷新点数记录" @click="loadTransactions">
                <RefreshCw :size="15" />
              </button>
            </div>
            <div class="credit-log-list">
              <article v-for="item in detailTransactions" :key="item.id" class="credit-log-item">
                <ReceiptText :size="16" />
                <div>
                  <strong>{{ item.display_name || item.username || `用户 #${item.user_id}` }}</strong>
                  <span>{{ transactionTypeText(item.type) }} · {{ formatDate(item.created_at) }}</span>
                  <small>{{ transactionNote(item) }}</small>
                </div>
                <b :class="{ negative: item.amount < 0 }">{{ item.amount > 0 ? '+' : '' }}{{ item.amount }}</b>
              </article>
              <p v-if="loadingTransactions" class="page-status">加载中...</p>
              <p v-else-if="detailTransactions.length === 0" class="page-status">暂无该用户点数记录</p>
            </div>
            <p v-if="transactionError" class="status-error">{{ transactionError }}</p>
          </section>
        </div>
      </div>
    </div>

    <p v-if="errorMessage" class="status-error">{{ errorMessage }}</p>
    <p v-if="successMessage" class="status-success">{{ successMessage }}</p>
  </section>
</template>
