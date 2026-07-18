<script setup>
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { onLoad, onShow } from '@dcloudio/uni-app'

import { api } from '../../api/client.js'
import AnnouncementPopup from '../../components/AnnouncementPopup.vue'
import AppTabbar from '../../components/AppTabbar.vue'
import { navigateTo, redirectToAuth, requireAuth, routes } from '../../utils/routes.js'

const staticAssetBaseURL = `${import.meta.env.VITE_STATIC_ASSET_BASE_URL || ''}`.replace(/\/+$/, '')
const sourceCodeURL = `${import.meta.env.VITE_SOURCE_CODE_URL || 'https://github.com/your-org/image-agent'}`

function staticAsset(path) {
  const normalizedPath = `${path || ''}`.trim().replace(/^\/+/, '').replace(/^static\/+/i, '')
  if (!normalizedPath) return staticAssetBaseURL
  if (staticAssetBaseURL) return `${staticAssetBaseURL}/${normalizedPath}`
  return `/${['static', normalizedPath].join('/')}`
}

function staticIcon(name) {
  const normalizedName = `${name || ''}`.trim().replace(/\.png$/i, '')
  return staticAsset(`icons/${normalizedName}.png`)
}

const icon = staticIcon

const me = ref(null)
const credits = ref({ available_credits: 0 })
const transactions = ref([])
const loading = ref(false)
const saving = ref('')
const errorMessage = ref('')
const activeEditor = ref('')
const showPhoneBindModal = ref(false)
const showSMSPhoneBinder = ref(false)
const bindPhoneError = ref('')
const bindPhoneCountdown = ref(0)
const bindPhoneInputPhone = ref('')
const bindPhoneInputCode = ref('')
let bindPhoneTimer = null

const profileForm = reactive({ display_name: '' })
const emailForm = reactive({ email: '' })
const passwordForm = reactive({ current_password: '', new_password: '' })
const paymentForm = reactive({ current_password: '', payment_password: '' })

const username = computed(() => me.value?.username || '')
const displayName = computed(() => me.value?.display_name || username.value || '我的账号')
const roleText = computed(() => (me.value?.status === 'active' ? '普通用户' : '账号状态异常'))
const availableCredits = computed(() => credits.value?.available_credits ?? me.value?.available_credits ?? 0)
const paymentPasswordText = computed(() => (me.value?.payment_password_enabled ? '已设置' : '未设置'))
const emailText = computed(() => me.value?.email || '未绑定')
const phoneText = computed(() => me.value?.phone || '未绑定')
const needsPhoneBinding = computed(() => Boolean(me.value && !me.value.phone))
const bindPhoneValid = computed(() => /^1[3-9]\d{9}$/.test(bindPhoneInputPhone.value.trim()))
const bindPhoneCodeValid = computed(() => /^\d{6}$/.test(bindPhoneInputCode.value.trim()))

const recentTransactions = computed(() => transactions.value.slice(0, 4))

const monthConsumption = computed(() => {
  const now = new Date()
  return transactions.value
    .filter((item) => transactionKind(item) === 'consume' && sameMonth(item.created_at, now))
    .reduce((sum, item) => sum + Math.abs(Number(item.amount) || 0), 0)
})

const totalRecharge = computed(() =>
  transactions.value
    .filter((item) => transactionKind(item) === 'recharge')
    .reduce((sum, item) => sum + Math.abs(Number(item.amount) || 0), 0)
)

const lastUpdated = computed(() => {
  const times = [
    credits.value?.updated_at,
    me.value?.updated_at,
    transactions.value[0]?.created_at,
    transactions.value[0]?.updated_at
  ].filter(Boolean)
  if (times.length === 0) return '-'
  return formatDate(times[0], 'date')
})

function goPricing() {
  navigateTo(routes.pricing)
}

function openSourceCode() {
  if (typeof window !== 'undefined' && typeof window.open === 'function') {
    window.open(sourceCodeURL, '_blank', 'noopener,noreferrer')
    return
  }
  uni.setClipboardData({
    data: sourceCodeURL,
    success: () => showToast('源码地址已复制')
  })
}

function openCreditTransactions() {
  navigateTo(routes.accountTransactions)
}

function showToast(title) {
  uni.showToast({ title, icon: 'none' })
}

function resetForms() {
  profileForm.display_name = me.value?.display_name || ''
  emailForm.email = me.value?.email || ''
  bindPhoneInputPhone.value = me.value?.phone || ''
  bindPhoneInputCode.value = ''
  passwordForm.current_password = ''
  passwordForm.new_password = ''
  paymentForm.current_password = ''
  paymentForm.payment_password = ''
}

function inputText(event) {
  return `${event?.detail?.value ?? ''}`
}

function updateBindPhoneInputPhone(event) {
  bindPhoneInputPhone.value = inputText(event).trim().slice(0, 11)
}

function updateBindPhoneInputCode(event) {
  bindPhoneInputCode.value = inputText(event).trim().slice(0, 6)
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

function openEditor(name) {
  activeEditor.value = activeEditor.value === name ? '' : name
  resetForms()
}

function openPhoneBinder() {
  if (me.value?.phone) {
    showToast('手机号已绑定')
    return
  }
  activeEditor.value = ''
  bindPhoneError.value = ''
  showSMSPhoneBinder.value = false
  showPhoneBindModal.value = true
  resetForms()
}

async function refreshMe() {
  me.value = await api.getMe()
  if (me.value?.phone) closePhoneBinder()
  resetForms()
}

function closePhoneBinder() {
  showPhoneBindModal.value = false
  showSMSPhoneBinder.value = false
  bindPhoneError.value = ''
  bindPhoneInputPhone.value = ''
  bindPhoneInputCode.value = ''
}

function toggleSMSPhoneBinder() {
  showSMSPhoneBinder.value = !showSMSPhoneBinder.value
  bindPhoneError.value = ''
}

function uniWeixinLogin() {
  return new Promise((resolve, reject) => {
    uni.login({
      provider: 'weixin',
      success(result) {
        if (result?.code) {
          resolve(result.code)
          return
        }
        reject(createBindPhoneError('wechat_login_failed', '登录失败，请稍后重试'))
      },
      fail(error) {
        reject(createBindPhoneError('wechat_login_failed', error?.errMsg || '登录失败，请稍后重试'))
      }
    })
  })
}

function createBindPhoneError(code, message) {
  const error = new Error(message)
  error.code = code
  return error
}

const wechatPhoneBackendErrorCodes = new Set([
  'wechat_phone_code_invalid',
  'wechat_phone_capability_unavailable',
  'wechat_phone_token_failed',
  'wechat_phone_failed'
])

function phoneAuthorizationErrorCode(event) {
  const rawMessage = `${event?.detail?.errMsg || ''}`.toLowerCase()
  if (rawMessage.includes('deny') || rawMessage.includes('cancel')) return 'phone_auth_cancelled'
  return 'phone_auth_required'
}

function bindPhoneErrorCode(error, fallbackCode = 'bind_failed') {
  const code = `${error?.code || ''}`
  if (code === 'network_error') return 'network_error'
  if (code === 'phone_exists') return 'phone_exists'
  if (code === 'phone_already_bound') return 'phone_already_bound'
  if (code === 'wechat_openid_conflict') return 'wechat_openid_conflict'
  if (code === 'verification_code_invalid') return 'verification_code_invalid'
  if (code === 'verification_attempts_exceeded') return 'verification_attempts_exceeded'
  if (code === 'wechat_login_failed') return 'wechat_login_failed'
  if (wechatPhoneBackendErrorCodes.has(code)) return code
  if (code === 'invalid_phone') return 'invalid_phone'
  if (code === 'invalid_sms_code') return 'invalid_sms_code'
  return fallbackCode
}

async function loadAccount() {
  loading.value = true
  errorMessage.value = ''
  try {
    const user = await requireAuth()
    if (!user) return
    me.value = user
    const [freshMe, creditPayload, transactionPayload] = await Promise.all([
      api.getMe(),
      api.getCredits(),
      api.getCreditTransactions()
    ])
    me.value = freshMe
    credits.value = creditPayload || { available_credits: freshMe.available_credits || 0 }
    transactions.value = Array.isArray(transactionPayload?.items) ? transactionPayload.items : []
    if (freshMe?.phone) closePhoneBinder()
    resetForms()
  } catch (error) {
    if (error?.status === 401) {
      redirectToAuth()
      return
    }
    errorMessage.value = error.message || '账号信息读取失败'
  } finally {
    loading.value = false
  }
}

async function saveProfile() {
  const nextName = profileForm.display_name.trim()
  if (!nextName) {
    showToast('昵称不能为空')
    return
  }
  saving.value = 'profile'
  try {
    me.value = await api.updateProfile({ display_name: nextName })
    activeEditor.value = ''
    showToast('资料已更新')
  } catch (error) {
    showToast(error.message || '资料保存失败')
  } finally {
    saving.value = ''
  }
}

async function saveEmail() {
  saving.value = 'email'
  try {
    me.value = await api.updateAccountEmail({ email: emailForm.email.trim() })
    activeEditor.value = ''
    showToast(me.value.email ? '邮箱已更新' : '邮箱已清空')
  } catch (error) {
    showToast(error.message || '邮箱保存失败')
  } finally {
    saving.value = ''
  }
}

async function sendBindPhoneCode() {
  if (saving.value || bindPhoneCountdown.value > 0) return
  bindPhoneError.value = ''
  if (!bindPhoneValid.value) {
    bindPhoneError.value = 'invalid_phone'
    return
  }
  saving.value = 'phoneCode'
  try {
    await api.sendSMSCode({ phone: bindPhoneInputPhone.value.trim(), purpose: 'bind_phone' })
    showToast('验证码已发送')
    startBindPhoneCountdown()
  } catch (error) {
    bindPhoneError.value = bindPhoneErrorCode(error, 'code_send_failed')
  } finally {
    saving.value = ''
  }
}

async function bindWechatPhone(event) {
  if (saving.value) return
  const phoneCode = event?.detail?.code
  if (!phoneCode) {
    bindPhoneError.value = phoneAuthorizationErrorCode(event)
    return
  }
  bindPhoneError.value = ''
  saving.value = 'wechatPhone'
  try {
    const code = await uniWeixinLogin()
    me.value = await api.bindWechatPhone({ code, phone_code: phoneCode })
    await refreshMe()
    showToast('手机号已绑定')
  } catch (error) {
    bindPhoneError.value = bindPhoneErrorCode(error, 'wechat_bind_failed')
  } finally {
    saving.value = ''
  }
}

async function bindPhone() {
  bindPhoneError.value = ''
  if (!bindPhoneValid.value) {
    bindPhoneError.value = 'invalid_phone'
    return
  }
  if (!bindPhoneCodeValid.value) {
    bindPhoneError.value = 'invalid_sms_code'
    return
  }
  saving.value = 'phone'
  try {
    me.value = await api.bindAccountPhone({
      phone: bindPhoneInputPhone.value.trim(),
      verification_code: bindPhoneInputCode.value.trim()
    })
    await refreshMe()
    showToast('手机号已绑定')
  } catch (error) {
    bindPhoneError.value = bindPhoneErrorCode(error, 'sms_bind_failed')
  } finally {
    saving.value = ''
  }
}

async function savePreferences(nextPatch) {
  if (!me.value) return
  const next = {
    login_notification_enabled: me.value.login_notification_enabled,
    risk_notification_enabled: me.value.risk_notification_enabled,
    ...nextPatch
  }
  saving.value = 'preferences'
  try {
    me.value = await api.updateAccountPreferences(next)
    showToast('偏好已更新')
  } catch (error) {
    showToast(error.message || '偏好保存失败')
  } finally {
    saving.value = ''
  }
}

async function savePassword() {
  saving.value = 'password'
  try {
    await api.changePassword({
      current_password: passwordForm.current_password,
      new_password: passwordForm.new_password
    })
    activeEditor.value = ''
    resetForms()
    showToast('登录密码已更新')
  } catch (error) {
    showToast(error.message || '密码更新失败')
  } finally {
    saving.value = ''
  }
}

async function savePaymentPassword() {
  saving.value = 'payment'
  try {
    await api.setPaymentPassword({
      current_password: paymentForm.current_password,
      payment_password: paymentForm.payment_password
    })
    await refreshMe()
    activeEditor.value = ''
    showToast('支付密码已更新')
  } catch (error) {
    showToast(error.message || '支付密码保存失败')
  } finally {
    saving.value = ''
  }
}

async function clearPaymentPassword() {
  if (!paymentForm.current_password) {
    showToast('请输入登录密码')
    return
  }
  saving.value = 'payment'
  try {
    await api.clearPaymentPassword({ current_password: paymentForm.current_password })
    await refreshMe()
    activeEditor.value = ''
    showToast('支付密码已清除')
  } catch (error) {
    showToast(error.message || '支付密码清除失败')
  } finally {
    saving.value = ''
  }
}

async function logout() {
  uni.showModal({
    title: '退出账号',
    content: '确认退出当前账号？',
    success: async (result) => {
      if (!result.confirm) return
      try {
        await api.logout()
      } finally {
        redirectToAuth({ redirect: routes.account })
      }
    }
  })
}

function transactionKind(item) {
  const amount = Number(item.amount) || 0
  const type = `${item.type || ''}`
  if (amount > 0 || type.includes('topup') || type.includes('recharge')) return 'recharge'
  return 'consume'
}

function transactionTitle(item) {
  const type = `${item.type || ''}`
  if (item.reason) return item.reason
  if (type.includes('generation')) return '生成消耗'
  if (type.includes('deduct')) return '点数扣减'
  if (type.includes('topup')) return '点数充值'
  return '点数变动'
}

function sameMonth(value, now) {
  const date = new Date(value)
  return date.getFullYear() === now.getFullYear() && date.getMonth() === now.getMonth()
}

function formatDate(value, mode = 'minute') {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  const year = date.getFullYear()
  const month = `${date.getMonth() + 1}`.padStart(2, '0')
  const day = `${date.getDate()}`.padStart(2, '0')
  if (mode === 'date') return `${year}/${month}/${day}`
  const hour = `${date.getHours()}`.padStart(2, '0')
  const minute = `${date.getMinutes()}`.padStart(2, '0')
  return `${year}/${month}/${day} ${hour}:${minute}`
}

onLoad((query = {}) => {
  if (`${query.bindPhone || ''}` === '1') openPhoneBinder()
})

onMounted(loadAccount)

onShow(() => {
  if (!me.value) return
  refreshMe().catch(() => {})
})

onUnmounted(() => {
  if (bindPhoneTimer) {
    clearInterval(bindPhoneTimer)
    bindPhoneTimer = null
  }
})
</script>

<template>
  <view class="account-page">
    <view class="app-shell">
      <view class="profile-hero">
        <view class="avatar-frame">
          <image :src="icon('logo-star')" mode="aspectFit" />
          <text class="avatar-badge">★</text>
        </view>
        <view class="profile-copy">
          <view class="profile-name-row">
            <text class="profile-name">{{ displayName }}</text>
            <button type="button" class="edit-profile-button" @click="openEditor('profile')">✎</button>
          </view>
          <text class="profile-meta">{{ username || '未设置账号名' }}</text>
          <text class="profile-status">{{ roleText }}</text>
        </view>
      </view>

      <view v-if="errorMessage" class="error-strip">{{ errorMessage }}</view>
      <view v-else-if="loading" class="sync-strip">正在同步账户信息</view>
      <view v-if="needsPhoneBinding" class="phone-bind-strip" @tap="openPhoneBinder">
        <view>
          <text>当前账号未绑定手机号</text>
          <text>绑定后可用手机号登录，也能通过手机号匹配账号。</text>
        </view>
        <button type="button" class="phone-bind-cta" @tap.stop="openPhoneBinder">去绑定</button>
      </view>

      <view v-if="activeEditor === 'profile'" class="inline-editor">
        <input v-model="profileForm.display_name" placeholder="昵称" />
        <button type="button" :disabled="saving === 'profile'" @click="saveProfile">保存</button>
      </view>

      <view class="credit-card">
        <view class="credit-main">
          <view>
            <text class="card-title">我的点数</text>
            <view class="credit-value">
              <text>{{ availableCredits }}</text>
              <text>点</text>
            </view>
          </view>
          <button type="button" class="recharge-button" @click="goPricing">充值</button>
        </view>
        <view class="credit-divider"></view>
        <view class="credit-stats">
          <view>
            <text>本月消耗</text>
            <text>{{ monthConsumption }}</text>
          </view>
          <view>
            <text>累计充值</text>
            <text>{{ totalRecharge }}</text>
          </view>
          <view>
            <text>最近更新</text>
            <text>{{ lastUpdated }}</text>
          </view>
        </view>
      </view>

      <view class="transaction-panel">
        <view class="panel-head">
          <view class="panel-title">
            <text class="panel-glyph">⌁</text>
            <text>点数流水</text>
          </view>
          <button type="button" class="panel-link" @tap="openCreditTransactions">查看全部</button>
        </view>

        <view v-if="recentTransactions.length > 0" class="flow-list">
          <view v-for="item in recentTransactions" :key="item.id" class="flow-row">
            <text class="flow-icon" :class="transactionKind(item) === 'recharge' ? 'up' : 'down'">
              {{ transactionKind(item) === 'recharge' ? '+' : '-' }}
            </text>
            <view class="flow-copy">
              <text>{{ transactionTitle(item) }}</text>
              <text>{{ formatDate(item.created_at) }}</text>
            </view>
            <text class="flow-amount" :class="transactionKind(item) === 'recharge' ? 'up' : 'down'">
              {{ Number(item.amount) > 0 ? '+' : '' }}{{ item.amount }}点
            </text>
          </view>
        </view>

        <view v-else class="transaction-empty">
          <view class="empty-illustration">
            <text></text>
            <text></text>
            <text></text>
          </view>
          <text>暂无流水记录</text>
        </view>
      </view>

      <view class="security-panel">
        <view class="panel-head security-head">
          <view class="panel-title">
            <text class="panel-glyph shield-glyph">◇</text>
            <text>安全设置</text>
          </view>
        </view>

        <button type="button" class="setting-row" @click="openEditor('password')">
          <text class="setting-icon lock"></text>
          <view>
            <text>登录密码</text>
            <text>用于账号登录与敏感操作确认</text>
          </view>
          <text class="setting-value">已设置</text>
          <text class="chevron">›</text>
        </button>

        <view v-if="activeEditor === 'password'" class="inline-editor stacked">
          <input v-model="passwordForm.current_password" type="password" placeholder="当前登录密码" />
          <input v-model="passwordForm.new_password" type="password" placeholder="新登录密码" />
          <button type="button" :disabled="saving === 'password'" @click="savePassword">更新登录密码</button>
        </view>

        <button type="button" class="setting-row" @click="openEditor('payment')">
          <text class="setting-icon key"></text>
          <view>
            <text>支付密码</text>
            <text>6 位数字，当前不参与点数扣减</text>
          </view>
          <text class="setting-value">{{ paymentPasswordText }}</text>
          <text class="chevron">›</text>
        </button>

        <view v-if="activeEditor === 'payment'" class="inline-editor stacked">
          <input v-model="paymentForm.current_password" type="password" placeholder="登录密码" />
          <input v-model="paymentForm.payment_password" type="number" maxlength="6" placeholder="6 位支付密码" />
          <view class="editor-actions">
            <button type="button" :disabled="saving === 'payment'" @click="savePaymentPassword">保存支付密码</button>
            <button type="button" class="ghost" :disabled="saving === 'payment'" @click="clearPaymentPassword">清除</button>
          </view>
        </view>

        <button type="button" class="setting-row" @click="openEditor('email')">
          <text class="setting-icon mail"></text>
          <view>
            <text>绑定邮箱</text>
            <text>{{ emailText }}</text>
          </view>
          <text class="chevron">›</text>
        </button>

        <view v-if="activeEditor === 'email'" class="inline-editor">
          <input v-model="emailForm.email" placeholder="邮箱，留空可清除" />
          <button type="button" :disabled="saving === 'email'" @click="saveEmail">保存</button>
        </view>

        <button type="button" class="setting-row" @tap="openPhoneBinder">
          <text class="setting-icon phone"></text>
          <view>
            <text>绑定手机号</text>
            <text>{{ phoneText }}</text>
          </view>
          <text class="chevron">›</text>
        </button>

        <view class="setting-row switch-row">
          <text class="setting-icon bell"></text>
          <view>
            <text>登录保护</text>
            <text>登录提醒开关</text>
          </view>
          <switch
            :checked="Boolean(me?.login_notification_enabled)"
            color="#7c3aed"
            @change="savePreferences({ login_notification_enabled: $event.detail.value })"
          />
        </view>

        <view class="setting-row switch-row last">
          <text class="setting-icon shield"></text>
          <view>
            <text>异常通知</text>
            <text>异常登录与风险提醒</text>
          </view>
          <switch
            :checked="Boolean(me?.risk_notification_enabled)"
            color="#7c3aed"
            @change="savePreferences({ risk_notification_enabled: $event.detail.value })"
          />
        </view>
      </view>

      <button type="button" class="logout-button" @click="logout">
        <text>退出账号</text>
      </button>

      <button type="button" class="source-code-button" @click="openSourceCode">
        <text>源码 · AGPL-3.0</text>
      </button>

    </view>

    <view v-if="showPhoneBindModal" class="phone-bind-modal-backdrop" @tap="closePhoneBinder">
      <view class="phone-bind-modal" @tap.stop>
        <view class="phone-bind-modal-head">
          <view>
            <text>绑定手机号</text>
            <text>优先使用手机号快捷验证绑定当前账号，也可改用短信验证码。</text>
          </view>
          <button type="button" class="modal-close-button" @tap="closePhoneBinder">×</button>
        </view>

        <button
          type="button"
          class="phone-quick-bind-button"
          open-type="getPhoneNumber"
          :disabled="saving !== ''"
          @getphonenumber="bindWechatPhone"
        >
          <text v-if="saving === 'wechatPhone'">手机号验证中...</text>
          <text v-else>手机号快捷绑定</text>
        </button>

        <view v-if="bindPhoneError" class="phone-bind-error">
          <text v-if="bindPhoneError === 'phone_auth_cancelled'">已取消手机号授权</text>
          <text v-else-if="bindPhoneError === 'phone_auth_required'">请完成手机号授权</text>
          <text v-else-if="bindPhoneError === 'wechat_login_failed'">登录失败，请稍后重试</text>
          <text v-else-if="bindPhoneError === 'wechat_phone_code_invalid'">授权已失效，请重新授权手机号</text>
          <text v-else-if="bindPhoneError === 'wechat_phone_capability_unavailable'">手机号快捷绑定暂不可用，请改用短信验证码</text>
          <text v-else-if="bindPhoneError === 'wechat_phone_token_failed'">手机号快捷绑定暂不可用，请改用短信验证码</text>
          <text v-else-if="bindPhoneError === 'wechat_phone_failed'">手机号快捷绑定暂不可用，请改用短信验证码</text>
          <text v-else-if="bindPhoneError === 'phone_exists'">手机号已被其他账号绑定</text>
          <text v-else-if="bindPhoneError === 'phone_already_bound'">当前账号已绑定手机号</text>
          <text v-else-if="bindPhoneError === 'wechat_openid_conflict'">账号绑定冲突，请联系客服或换号重试</text>
          <text v-else-if="bindPhoneError === 'verification_attempts_exceeded'">验证码错误次数过多，请重新获取</text>
          <text v-else-if="bindPhoneError === 'verification_code_invalid'">短信验证码错误或已过期</text>
          <text v-else-if="bindPhoneError === 'network_error'">网络连接失败，请稍后重试</text>
          <text v-else-if="bindPhoneError === 'invalid_phone'">请输入正确的大陆手机号</text>
          <text v-else-if="bindPhoneError === 'invalid_sms_code'">请输入 6 位短信验证码</text>
          <text v-else-if="bindPhoneError === 'code_send_failed'">验证码发送失败，请稍后重试</text>
          <text v-else-if="bindPhoneError === 'wechat_bind_failed'">手机号快捷绑定失败，请稍后重试</text>
          <text v-else>手机号绑定失败，请稍后重试</text>
        </view>

        <button type="button" class="sms-phone-bind-toggle" @tap="toggleSMSPhoneBinder">
          <text v-if="showSMSPhoneBinder">收起短信验证码绑定</text>
          <text v-else>使用短信验证码绑定</text>
        </button>

        <view v-if="showSMSPhoneBinder" class="sms-phone-bind-form">
          <input
            :value="bindPhoneInputPhone"
            type="number"
            maxlength="11"
            placeholder="大陆手机号"
            @input="updateBindPhoneInputPhone"
          />
          <view class="phone-code-row">
            <input
              :value="bindPhoneInputCode"
              type="number"
              maxlength="6"
              placeholder="短信验证码"
              @input="updateBindPhoneInputCode"
            />
            <button
              type="button"
              :disabled="saving !== '' || bindPhoneCountdown > 0 || !bindPhoneValid"
              @tap="sendBindPhoneCode"
            >
              <text v-if="saving === 'phoneCode'">发送中</text>
              <text v-else>获取验证码</text>
            </button>
          </view>
          <button
            type="button"
            class="sms-phone-bind-submit"
            :disabled="saving === 'phone' || !bindPhoneValid || !bindPhoneCodeValid"
            @tap="bindPhone"
          >
            绑定手机号
          </button>
        </view>
      </view>
    </view>

    <AppTabbar active-key="account" />
    <AnnouncementPopup />
  </view>
</template>

<style lang="scss" scoped>
@use '../../styles/tokens.scss' as *;

.account-page {
  min-height: 100vh;
  background: linear-gradient(180deg, #f7fbff 0%, #eef7f2 54%, #f8fafc 100%);
  color: #111827;
}

.account-page,
.account-page view,
.account-page button,
.account-page input,
.account-page image,
.account-page text {
  box-sizing: border-box;
}

.account-page button {
  margin: 0;
  padding: 0;
  border: 0;
  line-height: 1.2;
}

.account-page button::after {
  border: 0;
}

.app-shell {
  min-height: 100vh;
  padding: calc(18rpx + $phone-float-clearance-top + env(safe-area-inset-top)) 16rpx 0;
}

.profile-card,
.credit-card,
.transaction-panel,
.security-panel,
.device-panel,
.help-card,
.faq-panel,
.qr-card,
.logout-button,
.inline-editor,
.error-strip,
.phone-bind-strip {
  width: 100%;
  min-width: 0;
  border: 1rpx solid rgba(148, 163, 184, 0.18);
  border-radius: 18rpx;
  background: rgba(255, 255, 255, 0.92);
  box-shadow: 0 14rpx 34rpx rgba(31, 45, 82, 0.05);
}

.profile-card {
  display: flex;
  gap: 20rpx;
  padding: 24rpx;
}

.avatar-wrap {
  width: 104rpx;
  height: 104rpx;
  border-radius: 30rpx;
  background: #e0f2fe;
  display: flex;
  align-items: center;
  justify-content: center;

  image {
    width: 60rpx;
    height: 60rpx;
  }
}

.profile-copy {
  min-width: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 10rpx;
}

.name-line {
  display: flex;
  align-items: center;
  gap: 12rpx;
}

.username {
  min-width: 0;
  font-size: 36rpx;
  font-weight: 900;
  color: #0f172a;
  word-break: break-all;
}

.vip-badge {
  flex: 0 0 auto;
  padding: 6rpx 12rpx;
  border-radius: 999rpx;
  background: #ecfdf5;
  color: #047857;
  font-size: 20rpx;
  font-weight: 800;
}

.role {
  font-size: 24rpx;
  color: #64748b;
}

.edit-button {
  align-self: flex-start;
  height: 56rpx;
  padding: 0 18rpx;
  border-radius: 999rpx;
  background: #111827;
  color: #fff;
  font-size: 23rpx;
  font-weight: 800;
}

.error-strip {
  margin-top: 16rpx;
  padding: 18rpx 22rpx;
  color: #b91c1c;
  font-size: 24rpx;
  background: #fef2f2;
}

.phone-bind-strip {
  margin-top: 16rpx;
  padding: 22rpx 18rpx 22rpx 22rpx;
  display: grid;
  grid-template-columns: minmax(0, 1fr) 150rpx;
  gap: 16rpx;
  align-items: center;
  background: linear-gradient(135deg, #eff6ff 0%, #eef2ff 100%);
  border-color: rgba(37, 99, 235, 0.14);

  view {
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 8rpx;
  }

  text:first-child {
    color: #172554;
    font-size: 27rpx;
    font-weight: 900;
    line-height: 1.25;
  }

  text:last-child {
    color: #4b5563;
    font-size: 21rpx;
    line-height: 1.45;
  }

  .phone-bind-cta {
    width: 150rpx;
    height: 68rpx;
    min-height: 68rpx;
    padding: 0;
    border-radius: 18rpx;
    display: flex;
    align-items: center;
    justify-content: center;
    background: linear-gradient(135deg, #2563eb 0%, #1d4ed8 100%);
    color: #ffffff;
    font-size: 24rpx;
    font-weight: 900;
    line-height: 1;
    white-space: nowrap;
    box-shadow: 0 10rpx 22rpx rgba(37, 99, 235, 0.22);
  }
}

.inline-editor {
  margin-top: 16rpx;
  padding: 18rpx;
  display: flex;
  gap: 12rpx;

  input {
    flex: 1;
    min-width: 0;
    height: 68rpx;
    border-radius: 14rpx;
    background: #f8fafc;
    border: 1rpx solid rgba(148, 163, 184, 0.22);
    padding: 0 18rpx;
    font-size: 24rpx;
  }

  button {
    height: 68rpx;
    padding: 0 20rpx;
    border-radius: 14rpx;
    background: #2563eb;
    color: #fff;
    font-size: 24rpx;
    font-weight: 800;
  }
}

.inline-editor.compact {
  flex-direction: column;
}

.phone-code-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 154rpx;
  gap: 12rpx;

  button[disabled] {
    opacity: 0.56;
  }
}

.phone-bind-modal-backdrop {
  position: fixed;
  inset: 0;
  z-index: 50;
  padding: 0 24rpx calc(24rpx + env(safe-area-inset-bottom));
  background: rgba(15, 23, 42, 0.42);
  display: flex;
  align-items: flex-end;
  justify-content: center;
}

.phone-bind-modal {
  width: 100%;
  max-width: 520px;
  max-height: calc(100vh - 96rpx - env(safe-area-inset-bottom));
  min-width: 0;
  padding: 30rpx 28rpx 28rpx;
  border-radius: 30rpx 30rpx 24rpx 24rpx;
  background: #ffffff;
  box-shadow: 0 -18rpx 48rpx rgba(15, 23, 42, 0.16);
  display: flex;
  flex-direction: column;
  gap: 18rpx;
  overflow-y: auto;
}

.phone-bind-modal-head {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 88rpx;
  gap: 18rpx;
  align-items: flex-start;

  view {
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 8rpx;
  }

  text:first-child {
    color: #0f172a;
    font-size: 32rpx;
    font-weight: 900;
    line-height: 1.2;
  }

  text:last-child {
    color: #64748b;
    font-size: 23rpx;
    line-height: 1.45;
  }
}

.modal-close-button {
  width: 88rpx;
  height: 88rpx;
  min-height: 88rpx;
  border-radius: 50%;
  background: #f1f5f9;
  color: #475569;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 34rpx;
  font-weight: 700;
}

.phone-quick-bind-button,
.sms-phone-bind-submit {
  width: 100%;
  min-height: 88rpx;
  border-radius: 18rpx;
  background: linear-gradient(135deg, #2563eb 0%, #1d4ed8 100%);
  color: #ffffff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 26rpx;
  font-weight: 900;
  box-shadow: 0 12rpx 28rpx rgba(37, 99, 235, 0.22);
}

.phone-quick-bind-button[disabled],
.sms-phone-bind-submit[disabled] {
  opacity: 0.56;
}

.phone-bind-error {
  padding: 16rpx 18rpx;
  border-radius: 16rpx;
  background: #fef2f2;
  color: #b91c1c;
  font-size: 23rpx;
  line-height: 1.45;
}

.sms-phone-bind-toggle {
  min-height: 88rpx;
  border-radius: 16rpx;
  background: #f8fafc;
  color: #2563eb;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 24rpx;
  font-weight: 850;
}

.sms-phone-bind-form {
  display: flex;
  flex-direction: column;
  gap: 12rpx;

  input {
    width: 100%;
    height: 88rpx;
    border-radius: 16rpx;
    background: #f8fafc;
    border: 1rpx solid rgba(148, 163, 184, 0.22);
    padding: 0 18rpx;
    font-size: 24rpx;
  }

  .phone-code-row button {
    min-height: 88rpx;
    border-radius: 16rpx;
    background: #e0f2fe;
    color: #0369a1;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 23rpx;
    font-weight: 850;
  }
}

.editor-actions {
  display: flex;
  gap: 12rpx;

  button {
    flex: 1;
  }

  .ghost {
    background: #e2e8f0;
    color: #334155;
  }
}

.credit-card,
.transaction-panel,
.security-panel,
.device-panel,
.help-card,
.faq-panel,
.qr-card,
.logout-button {
  margin-top: 18rpx;
}

.credit-card {
  padding: 26rpx;
}

.credit-head,
.panel-head,
.panel-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16rpx;
}

.section-label {
  font-size: 23rpx;
  color: #64748b;
  font-weight: 700;
}

.credit-value {
  display: flex;
  align-items: baseline;
  gap: 10rpx;
  margin-top: 10rpx;

  text:first-child {
    font-size: 58rpx;
    font-weight: 900;
    color: #0f172a;
  }

  text:last-child {
    font-size: 24rpx;
    color: #475569;
  }
}

.coin-mark {
  width: 92rpx;
  height: 92rpx;
  border-radius: 50%;
  background: #fef3c7;
  color: #b45309;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 34rpx;
  font-weight: 900;
}

.credit-stats {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12rpx;
  margin-top: 22rpx;

  view {
    min-width: 0;
    padding: 16rpx 10rpx;
    border-radius: 14rpx;
    background: #f8fafc;
    display: flex;
    flex-direction: column;
    gap: 8rpx;
  }

  text {
    font-size: 22rpx;
    color: #64748b;
  }

  view > text:last-child {
    color: #0f172a;
    font-weight: 800;
  }
}

.recharge-button {
  width: 100%;
  height: 76rpx;
  margin-top: 22rpx;
  border-radius: 18rpx;
  background: #111827;
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10rpx;
  font-size: 25rpx;
  font-weight: 800;
}

.transaction-panel,
.security-panel,
.device-panel,
.faq-panel {
  padding: 22rpx;
}

.panel-head > view:first-child,
.panel-title {
  font-size: 28rpx;
  font-weight: 900;
  color: #0f172a;
}

.flow-tabs {
  display: flex;
  gap: 8rpx;

  text {
    padding: 8rpx 14rpx;
    border-radius: 999rpx;
    color: #64748b;
    background: #f1f5f9;
    font-size: 22rpx;
    font-weight: 700;
  }

  .active {
    color: #fff;
    background: #2563eb;
  }
}

.flow-list {
  margin-top: 18rpx;
  display: flex;
  flex-direction: column;
  gap: 12rpx;
}

.flow-row {
  display: grid;
  grid-template-columns: 44rpx minmax(0, 1fr) auto;
  gap: 12rpx;
  align-items: center;
  padding: 14rpx 0;
  border-bottom: 1rpx solid rgba(148, 163, 184, 0.14);
}

.flow-icon {
  width: 40rpx;
  height: 40rpx;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 22rpx;
  font-weight: 900;
}

.flow-icon.up {
  background: #dcfce7;
  color: #15803d;
}

.flow-icon.down {
  background: #fee2e2;
  color: #b91c1c;
}

.flow-title {
  min-width: 0;
  font-size: 25rpx;
  font-weight: 800;
  color: #0f172a;
}

.flow-amount {
  font-size: 24rpx;
  font-weight: 900;
}

.flow-amount.up {
  color: #15803d;
}

.flow-amount.down {
  color: #b91c1c;
}

.flow-date {
  grid-column: 2 / 4;
  font-size: 21rpx;
  color: #94a3b8;
}

.empty-row {
  padding: 28rpx 0;
  text-align: center;
  color: #94a3b8;
  font-size: 24rpx;
}

.setting-row {
  width: 100%;
  min-height: 86rpx;
  display: grid;
  grid-template-columns: 44rpx minmax(0, 1fr) auto auto;
  gap: 14rpx;
  align-items: center;
  padding: 18rpx 0;
  background: transparent;
  border-bottom: 1rpx solid rgba(148, 163, 184, 0.14);
  text-align: left;

  view {
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 6rpx;
  }

  view text:first-child {
    font-size: 26rpx;
    font-weight: 850;
    color: #0f172a;
  }

  view text:last-child {
    font-size: 22rpx;
    color: #64748b;
    word-break: break-all;
  }
}

.switch-row {
  grid-template-columns: 44rpx minmax(0, 1fr) auto;
}

.setting-icon {
  width: 38rpx;
  height: 38rpx;
  border-radius: 12rpx;
  background: #dbeafe;
}

.setting-value {
  color: #64748b;
  font-size: 22rpx;
}

.device-panel .panel-title button {
  height: 52rpx;
  padding: 0 16rpx;
  border-radius: 999rpx;
  background: #eff6ff;
  color: #1d4ed8;
  font-size: 22rpx;
  font-weight: 800;
}

.device-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 92rpx;
  gap: 12rpx;
  padding: 18rpx 0;
  border-bottom: 1rpx solid rgba(148, 163, 184, 0.14);

  view {
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 6rpx;
  }

  text:first-child {
    font-size: 25rpx;
    font-weight: 850;
    color: #0f172a;
  }

  text:not(:first-child) {
    font-size: 21rpx;
    color: #64748b;
    word-break: break-all;
  }

  button {
    height: 54rpx;
    border-radius: 14rpx;
    background: #fee2e2;
    color: #b91c1c;
    font-size: 22rpx;
    font-weight: 800;
  }

  button[disabled] {
    background: #f1f5f9;
    color: #94a3b8;
  }
}

.help-card {
  padding: 22rpx;
  display: grid;
  grid-template-columns: minmax(0, 1fr) 116rpx 116rpx;
  gap: 12rpx;
  align-items: center;

  view {
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 6rpx;
  }

  view text:first-child {
    width: 42rpx;
    height: 42rpx;
    border-radius: 50%;
    background: #111827;
    color: #fff;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 24rpx;
    font-weight: 900;
  }

  view text:nth-child(2) {
    font-size: 28rpx;
    font-weight: 900;
    color: #0f172a;
  }

  view text:last-child {
    font-size: 22rpx;
    color: #64748b;
  }

  button {
    height: 60rpx;
    border-radius: 14rpx;
    background: #111827;
    color: #fff;
    font-size: 20rpx;
    font-weight: 800;
  }
}

.qr-card {
  padding: 20rpx;
  display: flex;
  justify-content: center;

  image {
    width: 220rpx;
    height: 220rpx;
  }
}

.faq-panel {
  display: flex;
  flex-direction: column;
  gap: 12rpx;

  > text {
    font-size: 26rpx;
    font-weight: 900;
    color: #0f172a;
  }

  view {
    padding: 14rpx 0;
    border-bottom: 1rpx solid rgba(148, 163, 184, 0.14);
    font-size: 23rpx;
    color: #475569;
  }
}

.logout-button {
  height: 78rpx;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #fff1f2;
  color: #be123c;
  font-size: 26rpx;
  font-weight: 900;
}

/* Screenshot-matched account surface. */
.account-page {
  background:
    radial-gradient(circle at 50% 0%, rgba(255, 255, 255, 0.96) 0, rgba(255, 255, 255, 0) 260rpx),
    linear-gradient(180deg, #ecebff 0%, #f5f3ff 34%, #f7f8ff 100%);
  color: #16111f;
}

.profile-hero,
.credit-card,
.transaction-panel,
.security-panel,
.logout-button,
.inline-editor,
.error-strip,
.sync-strip {
  width: 100%;
  min-width: 0;
  border: 0;
  border-radius: 34rpx;
  background: rgba(255, 255, 255, 0.94);
  box-shadow: 0 18rpx 48rpx rgba(80, 63, 139, 0.1);
}

.profile-hero {
  margin-top: 16rpx;
  padding: 30rpx 28rpx;
  display: flex;
  gap: 24rpx;
  align-items: center;
}

.avatar-frame {
  position: relative;
  width: 116rpx;
  height: 116rpx;
  border-radius: 50%;
  background: linear-gradient(145deg, #fff6b7 0%, #ffd36e 48%, #f59e0b 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: inset 0 -8rpx 16rpx rgba(146, 64, 14, 0.16);
}

.avatar-frame image {
  width: 62rpx;
  height: 62rpx;
}

.avatar-badge {
  position: absolute;
  right: -4rpx;
  bottom: 6rpx;
  width: 34rpx;
  height: 34rpx;
  border: 4rpx solid #fff;
  border-radius: 50%;
  background: #8b5cf6;
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18rpx;
  font-weight: 900;
}

.profile-name-row {
  display: flex;
  align-items: center;
  gap: 12rpx;
}

.profile-name {
  min-width: 0;
  max-width: 100%;
  color: #201832;
  font-size: 36rpx;
  font-weight: 900;
  line-height: 1.15;
  word-break: break-all;
}

.edit-profile-button {
  flex: 0 0 auto;
  width: 42rpx;
  height: 42rpx;
  border-radius: 50%;
  background: #f0eaff;
  color: #7c3aed;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 24rpx;
  font-weight: 900;
}

.profile-meta,
.profile-status {
  font-size: 24rpx;
  color: #7a728e;
  line-height: 1.35;
  word-break: break-all;
}

.sync-strip {
  margin-top: 18rpx;
  padding: 18rpx 24rpx;
  color: #6d5f85;
  font-size: 24rpx;
}

.inline-editor button {
  display: flex;
  align-items: center;
  justify-content: center;
  background: #7c3aed;
  white-space: nowrap;
}

.inline-editor.stacked {
  flex-direction: column;
}

.credit-card {
  padding: 30rpx 28rpx;
}

.credit-main {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16rpx;
}

.card-title {
  display: block;
  color: #5f5575;
  font-size: 25rpx;
  font-weight: 800;
}

.credit-value text:first-child {
  color: #6d28d9;
  font-size: 62rpx;
}

.credit-value text:last-child {
  color: #6d5f85;
  font-weight: 800;
}

.recharge-button {
  width: 118rpx;
  height: 62rpx;
  margin-top: 0;
  border-radius: 999rpx;
  background: linear-gradient(135deg, #8b5cf6 0%, #6d28d9 100%);
  box-shadow: 0 12rpx 24rpx rgba(109, 40, 217, 0.22);
}

.credit-divider {
  height: 1rpx;
  margin: 24rpx 0 18rpx;
  background: linear-gradient(90deg, rgba(124, 58, 237, 0), rgba(124, 58, 237, 0.18), rgba(124, 58, 237, 0));
}

.credit-stats {
  gap: 12rpx;
}

.credit-stats view {
  padding: 0;
  background: transparent;
}

.credit-stats text:first-child {
  color: #9a92ad;
  font-size: 21rpx;
}

.credit-stats view > text:last-child {
  color: #302640;
  font-size: 24rpx;
  overflow-wrap: anywhere;
}

.transaction-panel,
.security-panel {
  padding: 26rpx 28rpx;
}

.panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16rpx;
}

.panel-title {
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 12rpx;
  color: #201832;
  font-size: 29rpx;
  font-weight: 900;
}

.panel-glyph {
  width: 38rpx;
  height: 38rpx;
  border-radius: 12rpx;
  background: #efe9ff;
  color: #7c3aed;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 25rpx;
  font-weight: 900;
}

.shield-glyph {
  background: #edf7ff;
  color: #2563eb;
}

.panel-link {
  flex: 0 0 auto;
  color: #8b5cf6;
  font-size: 24rpx;
  font-weight: 800;
}

.flow-row {
  min-height: 82rpx;
  grid-template-columns: 46rpx minmax(0, 1fr) auto;
  border-bottom: 0;
  border-top: 1rpx solid rgba(124, 58, 237, 0.1);
}

.flow-copy {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 6rpx;
}

.flow-copy text:first-child {
  color: #201832;
  font-size: 25rpx;
  font-weight: 850;
  overflow-wrap: anywhere;
}

.flow-copy text:last-child {
  color: #a19aaf;
  font-size: 21rpx;
}

.transaction-empty {
  min-height: 216rpx;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 18rpx;
  color: #aaa3ba;
  font-size: 25rpx;
  font-weight: 700;
}

.empty-illustration {
  position: relative;
  width: 126rpx;
  height: 96rpx;
}

.empty-illustration text:first-child {
  position: absolute;
  left: 22rpx;
  top: 10rpx;
  width: 82rpx;
  height: 62rpx;
  border-radius: 18rpx;
  background: #efe9ff;
  transform: rotate(-8deg);
}

.empty-illustration text:nth-child(2) {
  position: absolute;
  left: 34rpx;
  top: 24rpx;
  width: 82rpx;
  height: 62rpx;
  border-radius: 18rpx;
  background: #fff;
  border: 2rpx solid rgba(124, 58, 237, 0.2);
}

.empty-illustration text:last-child {
  position: absolute;
  left: 54rpx;
  top: 45rpx;
  width: 40rpx;
  height: 8rpx;
  border-radius: 999rpx;
  background: #cabffd;
}

.security-head {
  margin-bottom: 6rpx;
}

.setting-row {
  min-height: 94rpx;
  grid-template-columns: 46rpx minmax(0, 1fr) auto auto;
  border-bottom: 1rpx solid rgba(124, 58, 237, 0.1);
}

.setting-row.last {
  border-bottom: 0;
}

.switch-row {
  grid-template-columns: 46rpx minmax(0, 1fr) auto;
}

.setting-icon {
  width: 42rpx;
  height: 42rpx;
  border-radius: 14rpx;
  background: #efe9ff;
  position: relative;
}

.setting-icon::after {
  position: absolute;
  left: 50%;
  top: 50%;
  width: 16rpx;
  height: 16rpx;
  border: 3rpx solid #7c3aed;
  border-radius: 4rpx;
  transform: translate(-50%, -50%);
  content: '';
}

.setting-icon.key::after {
  width: 18rpx;
  height: 8rpx;
  border-radius: 999rpx;
}

.setting-icon.mail::after {
  width: 20rpx;
  height: 14rpx;
  border-radius: 3rpx;
}

.setting-icon.phone::after {
  width: 12rpx;
  height: 20rpx;
  border-radius: 5rpx;
}

.setting-icon.bell::after,
.setting-icon.shield::after {
  border-radius: 50%;
}

.setting-value {
  white-space: nowrap;
}

.chevron {
  color: #bbb4c8;
  font-size: 34rpx;
  line-height: 1;
}

.logout-button {
  height: 86rpx;
  background: #fff;
  color: #ef4444;
}

.source-code-button {
  margin: 20rpx auto 0;
  padding: 16rpx 24rpx;
  border: 0;
  background: transparent;
  color: #6d5f7d;
  font-size: 22rpx;
  text-decoration: underline;
}
</style>
