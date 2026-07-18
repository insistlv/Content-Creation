<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  Bell,
  BookOpen,
  CircleHelp,
  Coins,
  CreditCard,
  Headphones,
  LockKeyhole,
  LogOut,
  Mail,
  PenLine,
  Phone,
  X
} from 'lucide-vue-next'

import { api } from '../api/client.js'
import { applyAvailableCredits, clearCurrentUser, setCurrentUser } from '../stores/session.js'

const router = useRouter()
const route = useRoute()

const me = ref(null)
const credits = ref(null)
const transactions = ref([])
const ledgerFilter = ref('all')
const ledgerPage = ref(1)
const ledgerPageSize = ref(10)
const ledgerTotal = ref(0)
const ledgerLoading = ref(false)
const profileModalOpen = ref(false)
const passwordModalOpen = ref(false)
const phoneModalOpen = ref(false)
const historyModalOpen = ref(false)
const phoneBindMode = ref('bind')
const loading = ref(false)
const savingProfileFields = ref(false)
const sendingBindPhoneCode = ref(false)
const unbindingPhone = ref(false)
const bindPhoneCountdown = ref(0)
const message = ref('')
const errorMessage = ref('')
let bindPhoneTimer = null

const profileForm = reactive({
  display_name: ''
})

const emailForm = reactive({
  email: ''
})

const bindPhoneForm = reactive({
  phone: '',
  code: ''
})

const unbindPhoneForm = reactive({
  current_password: ''
})

const preferencesForm = reactive({
  login_notification_enabled: true,
  risk_notification_enabled: true
})

const passwordForm = reactive({
  current_password: '',
  new_password: '',
  confirm_password: ''
})

const availableCredits = computed(() => credits.value?.available_credits ?? me.value?.available_credits ?? 0)
const displayName = computed(() => me.value?.display_name || me.value?.username || '正在加载...')
const accountIDLabel = computed(() => (me.value?.user_id ? `#${me.value.user_id}` : me.value?.username || '未登录'))
const recentTransactions = computed(() => transactions.value.slice(0, 8))

const transactionTypeLabels = {
  manual_topup: '人工充值',
  manual_deduct: '人工扣减',
  generation_charge: '图片生成',
  payment_topup: '套餐充值',
  prompt_template_use: '模板使用',
  signup_bonus: '注册赠送'
}

function transactionTypeLabel(item) {
  return transactionTypeLabels[item?.type] || item?.type || '账户变动'
}

function transactionAmountLabel(item) {
  const amount = Number(item?.amount ?? 0)
  return amount > 0 ? `+${amount}` : `${amount}`
}

const avatarText = computed(() => {
  const name = `${me.value?.display_name || me.value?.username || ''}`.trim()
  return name ? name.slice(0, 1).toUpperCase() : '·'
})

const avatarStyle = computed(() => {
  const seed = `${me.value?.username || me.value?.display_name || 'user'}`
  let hash = 0
  for (let i = 0; i < seed.length; i += 1) {
    hash = (hash * 31 + seed.charCodeAt(i)) >>> 0
  }
  const hue = hash % 360
  return {
    background: `linear-gradient(135deg, hsl(${hue} 72% 56%), hsl(${(hue + 42) % 360} 70% 44%))`
  }
})

const monthlyConsumption = computed(() => {
  if (credits.value?.monthly_consumption !== undefined) {
    return Number(credits.value.monthly_consumption) || 0
  }
  const now = new Date()
  return transactions.value
    .filter((item) => {
      const date = new Date(item.created_at)
      return item.amount < 0 && date.getFullYear() === now.getFullYear() && date.getMonth() === now.getMonth()
    })
    .reduce((sum, item) => sum + Math.abs(item.amount), 0)
})

const totalRecharge = computed(() => {
  if (credits.value?.total_recharged !== undefined) {
    return Number(credits.value.total_recharged) || 0
  }
  return transactions.value
    .filter((item) => item.amount > 0 || item.type === 'manual_topup')
    .reduce((sum, item) => sum + Math.max(item.amount, 0), 0)
})

const totalLedgerPages = computed(() => Math.max(Math.ceil(ledgerTotal.value / ledgerPageSize.value), 1))
const ledgerRangeStart = computed(() => {
  if (ledgerTotal.value <= 0) return 0
  return (ledgerPage.value - 1) * ledgerPageSize.value + 1
})
const ledgerRangeEnd = computed(() => {
  if (ledgerTotal.value <= 0) return 0
  return Math.min(ledgerPage.value * ledgerPageSize.value, ledgerTotal.value)
})

const accountStatusLabel = computed(() => {
  return !me.value?.status || me.value.status === 'active' ? '正常' : '待处理'
})

const emailStatusLabel = computed(() => {
  return me.value?.email ? '已绑定' : '未绑定'
})

const phoneBound = computed(() => Boolean(`${me.value?.phone || ''}`.trim()))
const maskedPhone = computed(() => (phoneBound.value ? maskPhone(me.value?.phone) : '未绑定'))
const bindPhoneValid = computed(() => /^1[3-9]\d{9}$/.test(bindPhoneForm.phone.trim()))
const bindPhoneCodeValid = computed(() => /^\d{6}$/.test(bindPhoneForm.code.trim()))
const bindPhoneCodeButtonText = computed(() => {
  if (bindPhoneCountdown.value > 0) return `${bindPhoneCountdown.value}s`
  if (sendingBindPhoneCode.value) return '发送中...'
  return '获取验证码'
})

const profileDisplayNameChanged = computed(() => {
  const currentName = me.value?.display_name ?? me.value?.username ?? ''
  return profileForm.display_name.trim() !== `${currentName}`.trim()
})

const profileEmailChanged = computed(() => {
  const currentEmail = me.value?.email ?? ''
  return emailForm.email.trim() !== `${currentEmail}`.trim()
})

const profileFieldsChanged = computed(() => profileDisplayNameChanged.value || profileEmailChanged.value)

function maskPhone(value) {
  const phone = `${value || ''}`.trim()
  if (!phone) return '未绑定'
  if (phone.length <= 7) return '已绑定'
  return `${phone.slice(0, 3)}****${phone.slice(-4)}`
}

function syncForms(payload) {
  profileForm.display_name = payload?.display_name ?? payload?.username ?? ''
  emailForm.email = payload?.email ?? ''
  if (Object.prototype.hasOwnProperty.call(payload ?? {}, 'phone')) {
    bindPhoneForm.phone = payload?.phone ?? ''
  }
  preferencesForm.login_notification_enabled = payload?.login_notification_enabled ?? true
  preferencesForm.risk_notification_enabled = payload?.risk_notification_enabled ?? true
}

function startBindPhoneCountdown(seconds = 60) {
  bindPhoneCountdown.value = seconds
  if (bindPhoneTimer) clearInterval(bindPhoneTimer)
  bindPhoneTimer = setInterval(() => {
    bindPhoneCountdown.value -= 1
    if (bindPhoneCountdown.value <= 0) {
      clearInterval(bindPhoneTimer)
      bindPhoneTimer = null
      bindPhoneCountdown.value = 0
    }
  }, 1000)
}

function formatDate(value) {
  if (!value) {
    return '未记录'
  }
  const date = new Date(value)
  const year = date.getFullYear()
  const month = `${date.getMonth() + 1}`.padStart(2, '0')
  const day = `${date.getDate()}`.padStart(2, '0')
  return `${year}/${month}/${day}`
}

function formatDateTime(value) {
  if (!value) {
    return '未记录'
  }
  const date = new Date(value)
  const hours = `${date.getHours()}`.padStart(2, '0')
  const minutes = `${date.getMinutes()}`.padStart(2, '0')
  return `${formatDate(value)} ${hours}:${minutes}`
}

function setMessage(text) {
  message.value = text
  errorMessage.value = ''
}

function setError(text) {
  errorMessage.value = text
  message.value = ''
}

function ledgerKindForFilter(filter = ledgerFilter.value) {
  if (filter === 'charge') return 'consume'
  if (filter === 'topup') return 'recharge'
  return undefined
}

function ledgerParams(page = ledgerPage.value, filter = ledgerFilter.value) {
  const params = {
    page,
    page_size: ledgerPageSize.value
  }
  const kind = ledgerKindForFilter(filter)
  if (kind) {
    params.kind = kind
  }
  return params
}

function applyLedgerPayload(payload, fallbackPage = ledgerPage.value) {
  transactions.value = payload?.items ?? []
  ledgerTotal.value = Number(payload?.total ?? transactions.value.length) || 0
  ledgerPage.value = Number(payload?.page ?? fallbackPage) || 1
  ledgerPageSize.value = Number(payload?.page_size ?? ledgerPageSize.value) || 10
}

async function loadCreditTransactions(page = ledgerPage.value, filter = ledgerFilter.value) {
  if (ledgerLoading.value) {
    return
  }
  ledgerLoading.value = true
  try {
    const payload = await api.getCreditTransactions(ledgerParams(page, filter))
    applyLedgerPayload(payload, page)
  } catch (error) {
    setError(error.message)
  } finally {
    ledgerLoading.value = false
  }
}

async function setLedgerFilter(filter) {
  if (ledgerLoading.value || filter === ledgerFilter.value) {
    return
  }
  ledgerFilter.value = filter
  await loadCreditTransactions(1, filter)
}

async function goLedgerPage(page) {
  if (ledgerLoading.value || page < 1 || page > totalLedgerPages.value || page === ledgerPage.value) {
    return
  }
  await loadCreditTransactions(page)
}

async function load() {
  loading.value = true
  ledgerLoading.value = true
  errorMessage.value = ''
  try {
    const [mePayload, creditsPayload, transactionPayload] = await Promise.all([
      api.getMe(),
      api.getCredits(),
      api.getCreditTransactions(ledgerParams(1, 'all'))
    ])
    credits.value = creditsPayload
    setCurrentUser(mePayload)
    if (creditsPayload?.available_credits !== undefined) {
      applyAvailableCredits(creditsPayload.available_credits)
      me.value = { ...(mePayload ?? {}), available_credits: creditsPayload.available_credits }
    } else {
      me.value = mePayload
    }
    applyLedgerPayload(transactionPayload, 1)
    syncForms(mePayload)
    if (!mePayload?.phone && route.query?.bindPhone === '1') {
      setMessage('当前账号未绑定手机号，请先完成手机号绑定。')
      openPhoneModal()
    }
  } catch (error) {
    setError(error.message)
  } finally {
    loading.value = false
    ledgerLoading.value = false
  }
}

function openProfileModal() {
  if (me.value) syncForms(me.value)
  profileModalOpen.value = true
}

function closeProfileModal() {
  if (savingProfileFields.value) return
  if (me.value) syncForms(me.value)
  profileModalOpen.value = false
}

function openPasswordModal() {
  passwordForm.current_password = ''
  passwordForm.new_password = ''
  passwordForm.confirm_password = ''
  passwordModalOpen.value = true
}

function closePasswordModal() {
  passwordModalOpen.value = false
}

function openPhoneModal() {
  phoneBindMode.value = phoneBound.value ? 'manage' : 'bind'
  bindPhoneForm.code = ''
  if (!phoneBound.value) bindPhoneForm.phone = ''
  unbindPhoneForm.current_password = ''
  phoneModalOpen.value = true
}

function closePhoneModal() {
  phoneModalOpen.value = false
  bindPhoneForm.code = ''
  unbindPhoneForm.current_password = ''
}

function openHistoryModal() {
  historyModalOpen.value = true
}

function closeHistoryModal() {
  historyModalOpen.value = false
}

async function sendBindPhoneCode() {
  if (sendingBindPhoneCode.value || bindPhoneCountdown.value > 0) {
    return
  }
  if (!bindPhoneValid.value) {
    setError('请输入有效手机号。')
    return
  }
  sendingBindPhoneCode.value = true
  try {
    await api.sendSMSCode({
      phone: bindPhoneForm.phone.trim(),
      purpose: 'bind_phone'
    })
    setMessage('验证码已发送，请注意查收。')
    startBindPhoneCountdown()
  } catch (error) {
    setError(error.message)
  } finally {
    sendingBindPhoneCode.value = false
  }
}

async function bindPhone() {
  if (!bindPhoneValid.value) {
    setError('请输入有效手机号。')
    return
  }
  if (!bindPhoneCodeValid.value) {
    setError('请输入 6 位短信验证码。')
    return
  }
  try {
    const payload = await api.bindAccountPhone({
      phone: bindPhoneForm.phone.trim(),
      verification_code: bindPhoneForm.code.trim()
    })
    me.value = payload
    setCurrentUser(payload)
    syncForms(payload)
    bindPhoneForm.code = ''
    phoneModalOpen.value = false
    setMessage('手机号已绑定。')
  } catch (error) {
    setError(error.message)
  }
}

async function unbindPhone() {
  if (!unbindPhoneForm.current_password.trim()) {
    setError('请输入当前密码。')
    return
  }
  unbindingPhone.value = true
  try {
    const payload = await api.unbindAccountPhone({
      current_password: unbindPhoneForm.current_password
    })
    me.value = payload
    setCurrentUser(payload)
    syncForms(payload)
    bindPhoneForm.code = ''
    unbindPhoneForm.current_password = ''
    phoneBindMode.value = 'bind'
    setMessage('手机号已解绑。')
  } catch (error) {
    setError(error.message)
  } finally {
    unbindingPhone.value = false
  }
}

async function saveAccountProfileFields() {
  if (savingProfileFields.value) {
    return
  }
  if (!profileFieldsChanged.value) {
    setMessage('没有需要保存的修改。')
    return
  }

  const shouldUpdateDisplayName = profileDisplayNameChanged.value
  const shouldUpdateEmail = profileEmailChanged.value
  const displayNameDraft = profileForm.display_name.trim()
  const emailDraft = emailForm.email.trim()

  savingProfileFields.value = true
  try {
    if (shouldUpdateDisplayName) {
      const payload = await api.updateProfile({
        display_name: displayNameDraft
      })
      me.value = payload
      setCurrentUser(payload)
      profileForm.display_name = payload?.display_name ?? payload?.username ?? ''
    }

    if (shouldUpdateEmail) {
      const payload = await api.updateAccountEmail({
        email: emailDraft
      })
      me.value = payload
      setCurrentUser(payload)
      syncForms(payload)
    } else if (me.value) {
      syncForms(me.value)
    }

    profileModalOpen.value = false
    setMessage('资料已保存。')
  } catch (error) {
    setError(error.message)
  } finally {
    savingProfileFields.value = false
  }
}

function clearEmail() {
  emailForm.email = ''
}

async function savePreferences() {
  try {
    const payload = await api.updateAccountPreferences({
      login_notification_enabled: preferencesForm.login_notification_enabled,
      risk_notification_enabled: preferencesForm.risk_notification_enabled
    })
    me.value = payload
    setCurrentUser(payload)
    syncForms(payload)
    setMessage('偏好已保存。')
  } catch (error) {
    setError(error.message)
  }
}

async function changePassword() {
  if (passwordForm.new_password !== passwordForm.confirm_password) {
    setError('两次输入的新密码不一致。')
    return
  }

  try {
    await api.changePassword({
      current_password: passwordForm.current_password,
      new_password: passwordForm.new_password
    })
    passwordForm.current_password = ''
    passwordForm.new_password = ''
    passwordForm.confirm_password = ''
    passwordModalOpen.value = false
    setMessage('密码已更新。')
  } catch (error) {
    setError(error.message)
  }
}

function goPricing() {
  router.push('/pricing')
}

function goHelp(target) {
  router.push(target)
}

async function logout() {
  await api.logout()
  clearCurrentUser()
  router.push('/login')
}

onMounted(load)

onBeforeUnmount(() => {
  if (bindPhoneTimer) {
    clearInterval(bindPhoneTimer)
    bindPhoneTimer = null
  }
})
</script>

<template>
  <section class="account-page account-center-page">
    <section id="profile" class="account-glass-card account-profile-card" data-testid="account-profile-section">
      <div class="account-profile-avatar-wrap">
        <div class="account-avatar account-avatar-initial account-profile-avatar" :style="avatarStyle" aria-hidden="true">
          {{ avatarText }}
        </div>
      </div>
      <div class="account-profile-copy">
        <div class="account-profile-title-row">
          <div>
            <span class="account-profile-eyebrow">个人资料</span>
            <div class="account-profile-name-line">
              <h1>{{ displayName }}</h1>
              <span class="account-chip">创作者账户</span>
            </div>
            <div class="account-profile-meta" aria-label="账号信息">
              <span>账号ID {{ accountIDLabel }}</span>
              <span>账号状态 {{ accountStatusLabel }}</span>
              <span>注册时间 {{ formatDateTime(me?.created_at) }}</span>
            </div>
          </div>
          <div class="account-profile-actions">
            <button class="account-primary-action" data-testid="account-edit-profile" type="button" @click="openProfileModal">
              <PenLine :size="16" aria-hidden="true" />
              编辑资料
            </button>
            <button class="account-logout-button" data-testid="account-logout" type="button" @click="logout">
              <LogOut :size="16" aria-hidden="true" />
              退出登录
            </button>
          </div>
        </div>
      </div>
    </section>

    <section id="security" class="account-glass-card account-section-card" data-testid="account-security-section">
      <div class="account-section-head">
        <div>
          <span class="account-chip">Security</span>
          <h2>安全与绑定中心</h2>
        </div>
      </div>

      <div class="account-settings-list">
        <article class="account-setting-row">
          <span class="account-setting-icon"><Phone :size="18" aria-hidden="true" /></span>
          <div class="account-setting-main">
            <strong>手机号</strong>
            <small v-if="phoneBound" data-testid="account-bound-phone">{{ maskedPhone }}</small>
            <small v-else class="muted">未绑定</small>
          </div>
          <span class="account-status-pill" :class="{ muted: !phoneBound }" data-testid="account-phone-status">
            {{ phoneBound ? '已绑定' : '未绑定' }}
          </span>
          <button
            class="account-row-action"
            :class="{ highlight: !phoneBound }"
            data-testid="account-manage-phone"
            type="button"
            @click="openPhoneModal"
          >
            {{ phoneBound ? '更换/解绑' : '立即绑定' }}
          </button>
        </article>

        <article class="account-setting-row">
          <span class="account-setting-icon"><Mail :size="18" aria-hidden="true" /></span>
          <div class="account-setting-main">
            <strong>邮箱</strong>
            <small :class="{ muted: !me?.email }">{{ me?.email || '未绑定' }}</small>
          </div>
          <span class="account-status-pill" :class="{ muted: !me?.email }">{{ emailStatusLabel }}</span>
          <button
            class="account-row-action"
            :class="{ highlight: !me?.email }"
            data-testid="account-bind-email"
            type="button"
            @click="openProfileModal"
          >
            {{ me?.email ? '更换邮箱' : '立即绑定' }}
          </button>
        </article>

        <article class="account-setting-row">
          <span class="account-setting-icon"><LockKeyhole :size="18" aria-hidden="true" /></span>
          <div class="account-setting-main">
            <strong>密码</strong>
            <small>******</small>
          </div>
          <span class="account-status-pill">已设置</span>
          <button class="account-row-action" data-testid="account-change-password" type="button" @click="openPasswordModal">
            修改密码
          </button>
        </article>

        <article class="account-setting-row account-setting-row-toggle">
          <span class="account-setting-icon"><Bell :size="18" aria-hidden="true" /></span>
          <div class="account-setting-main">
            <strong>登录保护</strong>
            <small>异地登录提醒</small>
          </div>
          <span class="account-status-pill">{{ preferencesForm.login_notification_enabled ? '已开启' : '已关闭' }}</span>
          <label class="account-switch">
            <input
              v-model="preferencesForm.login_notification_enabled"
              data-testid="account-login-notification"
              type="checkbox"
              @change="savePreferences"
            />
            <span aria-hidden="true"></span>
          </label>
        </article>
      </div>
    </section>

    <section id="credits" class="account-glass-card account-section-card" data-testid="account-credits-section">
      <div class="account-section-head account-section-head-actions">
        <div>
          <span class="account-chip">Credits</span>
          <h2>点数明细与套餐</h2>
        </div>
        <button class="account-primary-action" data-testid="account-go-pricing" type="button" @click="goPricing">
          <CreditCard :size="16" aria-hidden="true" />
          去套餐页充值
        </button>
      </div>

      <div class="account-credit-stats">
        <article>
          <span>本月消耗</span>
          <strong>{{ monthlyConsumption }}<small> 点</small></strong>
        </article>
        <article>
          <span>累计充值</span>
          <strong>{{ totalRecharge }}<small> 点</small></strong>
        </article>
        <article>
          <span>当前余额</span>
          <strong>{{ availableCredits }}<small> 点</small></strong>
        </article>
      </div>

      <div class="account-recent-ledger">
        <div class="account-ledger-title-row">
          <h3>近期记录</h3>
          <span>仅展示最近 {{ recentTransactions.length }} 条</span>
        </div>
        <div class="account-table-wrap">
          <table class="account-ledger-table account-ledger-table-compact">
            <thead>
              <tr>
                <th>时间</th>
                <th>类型</th>
                <th>变动数值</th>
                <th>说明</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="item in recentTransactions"
                :key="item.id"
                :data-testid="`account-recent-transaction-${item.id}`"
              >
                <td>{{ formatDateTime(item.created_at) }}</td>
                <td>
                  <span class="account-type-badge" :class="item.amount > 0 ? 'positive' : 'negative'">
                    {{ transactionTypeLabel(item) }}
                  </span>
                </td>
                <td class="account-amount-cell" :class="item.amount > 0 ? 'positive' : 'negative'">
                  {{ transactionAmountLabel(item) }}
                </td>
                <td>{{ item.reason || '账户变动' }}</td>
              </tr>
              <tr v-if="recentTransactions.length === 0">
                <td colspan="4">{{ ledgerLoading ? '加载流水中...' : '暂无流水记录' }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div class="account-ledger-footer">
          <span>共 {{ ledgerTotal }} 条流水</span>
          <button class="account-link-button" data-testid="account-open-history" type="button" @click="openHistoryModal">
            查看全部历史记录
          </button>
        </div>
      </div>
    </section>

    <section class="account-glass-card account-section-card" data-testid="account-support-section">
      <div class="account-section-head">
        <div>
          <span class="account-chip">Support</span>
          <h2>帮助与支持</h2>
        </div>
      </div>

      <div class="account-help-grid">
        <button
          class="account-help-card"
          data-testid="account-help-recharge"
          type="button"
          @click="goHelp({ path: '/pricing', hash: '#recharge-guide' })"
        >
          <span><BookOpen :size="22" aria-hidden="true" /></span>
          <strong>充值说明</strong>
          <small>支付、到账与订单确认</small>
        </button>
        <button
          class="account-help-card"
          data-testid="account-help-points"
          type="button"
          @click="goHelp({ path: '/pricing', hash: '#points-rules' })"
        >
          <span><Coins :size="22" aria-hidden="true" /></span>
          <strong>点数规则</strong>
          <small>生成消耗与余额说明</small>
        </button>
        <button
          class="account-help-card account-help-card-featured"
          data-testid="account-help-contact"
          type="button"
          @click="goHelp('/contact')"
        >
          <span><Headphones :size="22" aria-hidden="true" /></span>
          <strong>联系客服</strong>
          <small>账号、支付和创作问题</small>
        </button>
        <button
          class="account-help-card"
          data-testid="account-help-faq"
          type="button"
          @click="goHelp('/contact')"
        >
          <span><CircleHelp :size="22" aria-hidden="true" /></span>
          <strong>常见问题 FAQ</strong>
          <small>查看常见账户问题</small>
        </button>
      </div>
    </section>

    <div
      v-if="profileModalOpen"
      class="account-modal-backdrop"
      data-testid="account-profile-modal"
      role="dialog"
      aria-modal="true"
      aria-labelledby="account-profile-modal-title"
      @click.self="closeProfileModal"
    >
      <form class="account-modal account-profile-modal" @submit.prevent="saveAccountProfileFields">
        <div class="account-modal-head">
          <div>
            <span class="account-chip">Profile</span>
            <h2 id="account-profile-modal-title">编辑资料</h2>
          </div>
          <button class="account-icon-button" type="button" aria-label="关闭" @click="closeProfileModal">
            <X :size="18" aria-hidden="true" />
          </button>
        </div>
        <label class="account-field">
          <span>昵称</span>
          <input
            v-model="profileForm.display_name"
            data-testid="account-display-name-input"
            type="text"
            autocomplete="nickname"
          />
        </label>
        <label class="account-field">
          <span>邮箱</span>
          <input
            v-model="emailForm.email"
            data-testid="account-email-input"
            type="text"
            inputmode="email"
            name="image-agent-account-email"
            autocomplete="new-password"
            placeholder="请输入邮箱"
          />
        </label>
        <button class="account-row-button ghost" data-testid="account-clear-email" type="button" @click="clearEmail">
          清空邮箱草稿
        </button>
        <div class="account-modal-actions">
          <button class="account-secondary-action" type="button" @click="closeProfileModal">取消</button>
          <button
            class="account-primary-action"
            data-testid="account-save-profile-fields"
            type="button"
            :disabled="savingProfileFields || !profileFieldsChanged"
            @click="saveAccountProfileFields"
          >
            {{ savingProfileFields ? '保存中...' : '保存资料' }}
          </button>
        </div>
      </form>
    </div>

    <div
      v-if="phoneModalOpen"
      class="account-modal-backdrop"
      data-testid="account-phone-modal"
      role="dialog"
      aria-modal="true"
      aria-labelledby="account-phone-modal-title"
      @click.self="closePhoneModal"
    >
      <form class="account-modal account-phone-modal" @submit.prevent="phoneBound && phoneBindMode === 'manage' ? unbindPhone() : bindPhone()">
        <div class="account-modal-head">
          <div>
            <span class="account-chip">Phone</span>
            <h2 id="account-phone-modal-title">{{ phoneBound && phoneBindMode === 'manage' ? '管理手机号' : '绑定手机号' }}</h2>
          </div>
          <button class="account-icon-button" type="button" aria-label="关闭" @click="closePhoneModal">
            <X :size="18" aria-hidden="true" />
          </button>
        </div>

        <template v-if="phoneBound && phoneBindMode === 'manage'">
          <p class="account-modal-note">当前手机号 {{ maskedPhone }}。如需更换，请先解绑当前手机号，再绑定新号码。</p>
          <label class="account-field">
            <span>当前密码</span>
            <input
              v-model="unbindPhoneForm.current_password"
              data-testid="account-unbind-phone-password"
              type="password"
              name="image-agent-unbind-phone-password"
              autocomplete="new-password"
              placeholder="请输入当前密码"
            />
          </label>
          <div class="account-modal-actions">
            <button class="account-secondary-action" type="button" @click="closePhoneModal">取消</button>
            <button
              class="account-danger-action"
              data-testid="account-unbind-phone-submit"
              type="button"
              :disabled="unbindingPhone || !unbindPhoneForm.current_password.trim()"
              @click="unbindPhone"
            >
              {{ unbindingPhone ? '解绑中...' : '解绑手机号' }}
            </button>
          </div>
        </template>

        <template v-else>
          <label class="account-field">
            <span>手机号</span>
            <input
              v-model="bindPhoneForm.phone"
              data-testid="account-bind-phone-input"
              inputmode="tel"
              maxlength="11"
              type="text"
              placeholder="请输入大陆手机号"
            />
          </label>
          <div class="account-phone-code-row">
            <label class="account-field">
              <span>短信验证码</span>
              <input
                v-model="bindPhoneForm.code"
                data-testid="account-bind-phone-code"
                inputmode="numeric"
                maxlength="6"
                type="text"
                placeholder="6 位验证码"
              />
            </label>
            <button
              data-testid="account-send-bind-phone-code"
              type="button"
              :disabled="sendingBindPhoneCode || bindPhoneCountdown > 0 || !bindPhoneValid"
              @click="sendBindPhoneCode"
            >
              {{ bindPhoneCodeButtonText }}
            </button>
          </div>
          <div class="account-modal-actions">
            <button class="account-secondary-action" type="button" @click="closePhoneModal">取消</button>
            <button
              class="account-primary-action"
              data-testid="account-bind-phone-submit"
              type="button"
              :disabled="!bindPhoneValid || !bindPhoneCodeValid"
              @click="bindPhone"
            >
              绑定手机号
            </button>
          </div>
        </template>
      </form>
    </div>

    <div
      v-if="passwordModalOpen"
      class="account-modal-backdrop"
      data-testid="account-password-modal"
      role="dialog"
      aria-modal="true"
      aria-labelledby="account-password-modal-title"
      @click.self="closePasswordModal"
    >
      <form class="account-modal account-password-modal" @submit.prevent="changePassword">
        <div class="account-modal-head">
          <div>
            <span class="account-chip">Password</span>
            <h2 id="account-password-modal-title">修改密码</h2>
          </div>
          <button class="account-icon-button" type="button" aria-label="关闭" @click="closePasswordModal">
            <X :size="18" aria-hidden="true" />
          </button>
        </div>
        <label class="account-field">
          <span>当前密码</span>
          <input
            v-model="passwordForm.current_password"
            data-testid="account-current-password"
            type="password"
            name="image-agent-current-password"
            autocomplete="new-password"
            placeholder="请输入当前密码"
          />
        </label>
        <label class="account-field">
          <span>新密码</span>
          <input
            v-model="passwordForm.new_password"
            data-testid="account-new-password"
            type="password"
            name="image-agent-new-password"
            autocomplete="new-password"
            placeholder="请输入新密码"
          />
        </label>
        <label class="account-field">
          <span>确认新密码</span>
          <input
            v-model="passwordForm.confirm_password"
            data-testid="account-confirm-password"
            type="password"
            name="image-agent-confirm-password"
            autocomplete="new-password"
            placeholder="请再次输入新密码"
          />
        </label>
        <div class="account-modal-actions">
          <button class="account-secondary-action" type="button" @click="closePasswordModal">取消</button>
          <button class="account-primary-action" data-testid="account-update-password" type="button" @click="changePassword">
            更新密码
          </button>
        </div>
      </form>
    </div>

    <div
      v-if="historyModalOpen"
      class="account-modal-backdrop"
      data-testid="account-history-modal"
      role="dialog"
      aria-modal="true"
      aria-labelledby="account-history-modal-title"
      @click.self="closeHistoryModal"
    >
      <section class="account-modal account-history-modal">
        <div class="account-modal-head">
          <div>
            <span class="account-chip">Ledger</span>
            <h2 id="account-history-modal-title">全部历史记录</h2>
          </div>
          <button class="account-icon-button" type="button" aria-label="关闭" @click="closeHistoryModal">
            <X :size="18" aria-hidden="true" />
          </button>
        </div>

        <div class="account-filter-pills">
          <button
            :class="{ active: ledgerFilter === 'all' }"
            data-testid="account-ledger-filter-all"
            type="button"
            :disabled="loading || ledgerLoading"
            @click="setLedgerFilter('all')"
          >
            全部
          </button>
          <button
            :class="{ active: ledgerFilter === 'charge' }"
            data-testid="account-ledger-filter-charge"
            type="button"
            :disabled="loading || ledgerLoading"
            @click="setLedgerFilter('charge')"
          >
            消费
          </button>
          <button
            :class="{ active: ledgerFilter === 'topup' }"
            data-testid="account-ledger-filter-topup"
            type="button"
            :disabled="loading || ledgerLoading"
            @click="setLedgerFilter('topup')"
          >
            充值
          </button>
        </div>

        <div class="account-table-wrap">
          <table class="account-ledger-table">
            <thead>
              <tr>
                <th>时间</th>
                <th>类型</th>
                <th>变动数值</th>
                <th>说明</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in transactions" :key="item.id">
                <td>{{ formatDateTime(item.created_at) }}</td>
                <td>
                  <span class="account-type-badge" :class="item.amount > 0 ? 'positive' : 'negative'">
                    {{ transactionTypeLabel(item) }}
                  </span>
                </td>
                <td class="account-amount-cell" :class="item.amount > 0 ? 'positive' : 'negative'">
                  {{ transactionAmountLabel(item) }}
                </td>
                <td>{{ item.reason || '账户变动' }}</td>
              </tr>
              <tr v-if="transactions.length === 0">
                <td colspan="4">{{ ledgerLoading ? '加载流水中...' : '暂无流水记录' }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div class="account-ledger-pagination" data-testid="account-ledger-pagination">
          <span data-testid="account-ledger-range">第 {{ ledgerRangeStart }}-{{ ledgerRangeEnd }} 条 / 共 {{ ledgerTotal }} 条</span>
          <div>
            <button
              class="account-text-button"
              data-testid="account-ledger-prev"
              type="button"
              :disabled="loading || ledgerLoading || ledgerPage <= 1"
              @click="goLedgerPage(ledgerPage - 1)"
            >
              上一页
            </button>
            <button
              class="account-text-button"
              data-testid="account-ledger-next"
              type="button"
              :disabled="loading || ledgerLoading || ledgerPage >= totalLedgerPages"
              @click="goLedgerPage(ledgerPage + 1)"
            >
              下一页
            </button>
          </div>
        </div>
      </section>
    </div>

    <p v-if="message" class="account-toast success">{{ message }}</p>
    <p v-if="errorMessage" class="account-toast error">{{ errorMessage }}</p>
    <p v-if="loading" class="page-status">加载中...</p>
  </section>
</template>
