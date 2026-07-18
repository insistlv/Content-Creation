<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import {
  Eye,
  EyeOff,
  Hash,
  KeyRound,
  LockKeyhole,
  Phone,
  RefreshCcw,
  ShieldCheck,
  Ticket,
  UserRound
} from 'lucide-vue-next'

import { api } from '../api/client.js'
import { setCurrentUser } from '../stores/session.js'

const MIN_REGISTER_PASSWORD_LENGTH = 8

const props = defineProps({
  mode: {
    type: String,
    default: 'login'
  },
  initialReset: {
    type: Boolean,
    default: false
  },
  initialResetPhone: {
    type: String,
    default: ''
  }
})

const emit = defineEmits(['authenticated', 'mode-change'])
const activeMode = ref(props.mode)
const loginUsername = ref('')
const loginPassword = ref('')
const loginCaptchaId = ref('')
const loginCaptchaCode = ref('')
const loginCaptchaImage = ref('')
const loginCaptchaLoading = ref(false)
const registerUsername = ref('')
const registerPhone = ref('')
const registerPhoneRemoteHint = ref('')
const registerPhoneBlockedValue = ref('')
const registerCode = ref('')
const registerCodeRemoteHint = ref('')
const registerPassword = ref('')
const registerConfirmPassword = ref('')
const registerInviteCode = ref('')
const resetPasswordMode = ref(false)
const resetPhone = ref('')
const resetPhoneRemoteHint = ref('')
const resetCode = ref('')
const resetCodeRemoteHint = ref('')
const resetNewPassword = ref('')
const resetPasswordRemoteHint = ref('')
const resetConfirmPassword = ref('')
const rememberLogin = ref(false)
const acceptedTerms = ref(false)
const showPassword = ref(false)
const loading = ref(false)
const sendingCode = ref(false)
const codeCountdown = ref(0)
const message = ref('')
const errorMessage = ref('')
let countdownTimer = null

const isRegister = computed(() => activeMode.value === 'register')
const isResetPassword = computed(() => !isRegister.value && resetPasswordMode.value)
const passwordInputType = computed(() => (showPassword.value ? 'text' : 'password'))
const loginCaptchaImageSrc = computed(() => (
  loginCaptchaImage.value ? `data:image/png;base64,${loginCaptchaImage.value}` : ''
))
const registerPhoneValue = computed(() => normalizePhone(registerPhone.value))
const registerPhoneValid = computed(() => validPhone(registerPhoneValue.value))
const registerPhoneBlocked = computed(() => (
  registerPhoneBlockedValue.value !== '' &&
  registerPhoneValue.value === registerPhoneBlockedValue.value
))
const registerUsernameValue = computed(() => registerUsername.value.trim())
const registerUsernameValid = computed(() => validRegisterUsername(registerUsernameValue.value))
const registerCodeValue = computed(() => registerCode.value.trim())
const registerCodeValid = computed(() => /^\d{6}$/.test(registerCodeValue.value))
const registerPasswordMatched = computed(() => (
  registerPassword.value !== '' &&
  registerConfirmPassword.value !== '' &&
  registerPassword.value === registerConfirmPassword.value
))
const resetPhoneValue = computed(() => normalizePhone(resetPhone.value))
const resetPhoneValid = computed(() => validPhone(resetPhoneValue.value))
const resetCodeValue = computed(() => resetCode.value.trim())
const resetCodeValid = computed(() => /^\d{6}$/.test(resetCodeValue.value))
const resetPasswordMatched = computed(() => (
  resetNewPassword.value !== '' &&
  resetConfirmPassword.value !== '' &&
  resetNewPassword.value === resetConfirmPassword.value
))
const submitLabel = computed(() => {
  if (loading.value) {
    if (isResetPassword.value) return '重置中...'
    return isRegister.value ? '注册中...' : '登录中...'
  }
  if (isResetPassword.value) return '重置密码'
  return isRegister.value ? '注册并开始创作' : '登录并进入工作台'
})
const formTitle = computed(() => {
  if (isResetPassword.value) return '找回密码'
  return isRegister.value ? '立即创建账号' : '欢迎回来'
})
const formDescription = computed(() => (
  isResetPassword.value
    ? '使用绑定手机号接收验证码，并设置新的登录密码。'
    : isRegister.value
      ? '注册后即可开始图像生成、作品管理与点数使用。'
      : '登录后继续进行图像生成、作品管理与点数使用。'
))
const accountPrompt = computed(() => (isRegister.value ? '已有账号？' : '还没有账号？'))
const accountPromptLink = computed(() => (isRegister.value ? '立即登录' : '立即注册'))
const accountPromptTo = computed(() => (isRegister.value ? '/login' : '/register'))
const submitDisabled = computed(() => (
  isResetPassword.value
    ? !resetPhoneValid.value ||
      !resetCodeValid.value ||
      resetNewPassword.value.length < MIN_REGISTER_PASSWORD_LENGTH ||
      !resetPasswordMatched.value
    : isRegister.value
    ? !registerUsernameValid.value ||
      !registerPhoneValid.value ||
      registerPhoneBlocked.value ||
      !registerCodeValid.value ||
      registerPassword.value.length < MIN_REGISTER_PASSWORD_LENGTH ||
      !registerPasswordMatched.value ||
      !acceptedTerms.value
    : !loginUsername.value.trim() ||
      !loginPassword.value ||
      !loginCaptchaId.value ||
      !loginCaptchaCode.value.trim() ||
      loginCaptchaLoading.value
))
const registerPasswordHint = computed(() => {
  if (!isRegister.value) {
    return ''
  }
  if (!registerPassword.value) {
    return `密码至少 ${MIN_REGISTER_PASSWORD_LENGTH} 位`
  }
  const remaining = MIN_REGISTER_PASSWORD_LENGTH - registerPassword.value.length
  if (remaining > 0) {
    return `还差 ${remaining} 位`
  }
  return '密码长度符合要求'
})
const registerPasswordHintTone = computed(() => (
  registerPassword.value.length >= MIN_REGISTER_PASSWORD_LENGTH ? 'success' : 'error'
))
const registerPhoneHint = computed(() => (
  (registerPhoneBlocked.value ? '' : registerPhoneRemoteHint.value) ||
  (isRegister.value && registerPhone.value && !registerPhoneValid.value ? '请输入有效手机号' : '')
))
const registerUsernameHint = computed(() => (
  isRegister.value && registerUsername.value && !registerUsernameValid.value
    ? '账号只能使用 3-32 位字母、数字、下划线、点或横线'
    : ''
))
const registerCodeHint = computed(() => (
  registerCodeRemoteHint.value ||
  (isRegister.value && registerPhoneValid.value && !registerCodeValid.value ? '请输入 6 位短信验证码' : '')
))
const registerConfirmPasswordHint = computed(() => (
  registerConfirmPassword.value && registerPassword.value !== registerConfirmPassword.value
    ? '两次输入的密码不一致'
    : ''
))
const resetPhoneHint = computed(() => (
  resetPhoneRemoteHint.value ||
  (isResetPassword.value && resetPhone.value && !resetPhoneValid.value ? '请输入有效手机号' : '')
))
const resetCodeHint = computed(() => (
  resetCodeRemoteHint.value ||
  (isResetPassword.value && resetPhoneValid.value && !resetCodeValid.value ? '请输入 6 位短信验证码' : '')
))
const resetPasswordHint = computed(() => {
  if (!isResetPassword.value) {
    return ''
  }
  if (resetPasswordRemoteHint.value) {
    return resetPasswordRemoteHint.value
  }
  if (!resetNewPassword.value) {
    return `密码至少 ${MIN_REGISTER_PASSWORD_LENGTH} 位`
  }
  const remaining = MIN_REGISTER_PASSWORD_LENGTH - resetNewPassword.value.length
  if (remaining > 0) {
    return `还差 ${remaining} 位`
  }
  return '密码长度符合要求'
})
const resetPasswordHintTone = computed(() => (
  resetNewPassword.value.length >= MIN_REGISTER_PASSWORD_LENGTH && !resetPasswordRemoteHint.value ? 'success' : 'error'
))
const resetConfirmPasswordHint = computed(() => (
  resetConfirmPassword.value && resetNewPassword.value !== resetConfirmPassword.value
    ? '两次输入的密码不一致'
    : ''
))
const codeButtonText = computed(() => {
  if (codeCountdown.value > 0) return `${codeCountdown.value}s`
  if (sendingCode.value) return '发送中...'
  return '获取验证码'
})
const registerResetLinkTo = computed(() => ({
  path: '/login',
  query: {
    reset: '1',
    phone: registerPhoneValue.value
  }
}))

function normalizePhone(value) {
  return String(value || '').trim()
}

function validPhone(value) {
  return /^1[3-9]\d{9}$/.test(normalizePhone(value))
}

function validRegisterUsername(value) {
  return /^[A-Za-z0-9][A-Za-z0-9_.-]{2,31}$/.test(value)
}

function startCountdown(seconds = 60) {
  codeCountdown.value = seconds
  if (countdownTimer) clearInterval(countdownTimer)
  countdownTimer = setInterval(() => {
    codeCountdown.value -= 1
    if (codeCountdown.value <= 0) {
      clearInterval(countdownTimer)
      countdownTimer = null
      codeCountdown.value = 0
    }
  }, 1000)
}

function rateLimitCooldownSeconds(error) {
  const seconds = Number.parseInt(error?.retry_after_seconds, 10)
  if (Number.isFinite(seconds) && seconds > 0) {
    return seconds
  }
  return 60
}

function isSMSRateLimitError(error) {
  return error?.status === 429 ||
    error?.code === 'sms_rate_limited' ||
    error?.code === 'too_many_requests'
}

function setSMSRateLimitError(error, hintRef) {
  hintRef.value = error?.message || '请求过于频繁，请稍后再试'
  errorMessage.value = ''
  startCountdown(rateLimitCooldownSeconds(error))
}

function setRegistrationError(error) {
  const text = error?.message || '请求失败'
  if (isSMSRateLimitError(error)) {
    setSMSRateLimitError(error, registerCodeRemoteHint)
    return
  }
  if (error?.code === 'phone_exists') {
    registerPhoneBlockedValue.value = registerPhoneValue.value
    registerPhoneRemoteHint.value = '手机号已注册'
    registerCode.value = ''
    errorMessage.value = ''
    return
  }
  errorMessage.value = text
}

function setResetPasswordError(error) {
  const text = error?.message || '请求失败'
  if (isSMSRateLimitError(error)) {
    setSMSRateLimitError(error, resetCodeRemoteHint)
    return
  }
  if (error?.code === 'phone_not_found') {
    resetPhoneRemoteHint.value = '手机号未注册'
    errorMessage.value = ''
    return
  }
  if (error?.code === 'invalid_phone') {
    resetPhoneRemoteHint.value = '请输入有效手机号'
    errorMessage.value = ''
    return
  }
  if (error?.code === 'verification_code_invalid' || error?.code === 'verification_attempts_exceeded') {
    resetCodeRemoteHint.value = text
    errorMessage.value = ''
    return
  }
  if (error?.code === 'password_too_short') {
    resetPasswordRemoteHint.value = `密码至少 ${MIN_REGISTER_PASSWORD_LENGTH} 位`
    errorMessage.value = ''
    return
  }
  errorMessage.value = text
}

async function sendRegisterCode() {
  if (sendingCode.value || codeCountdown.value > 0 || registerPhoneBlocked.value) {
    return
  }
  message.value = ''
  errorMessage.value = ''
  registerCodeRemoteHint.value = ''
  if (!registerPhoneValid.value) {
    errorMessage.value = '请输入有效手机号'
    return
  }
  sendingCode.value = true
  try {
    await api.sendSMSCode({ phone: registerPhoneValue.value, purpose: 'register' })
    message.value = '验证码已发送，请注意查收'
    startCountdown()
  } catch (error) {
    setRegistrationError(error)
  } finally {
    sendingCode.value = false
  }
}

async function sendResetCode() {
  if (sendingCode.value || codeCountdown.value > 0) {
    return
  }
  message.value = ''
  errorMessage.value = ''
  resetPhoneRemoteHint.value = ''
  resetCodeRemoteHint.value = ''
  if (!resetPhoneValid.value) {
    resetPhoneRemoteHint.value = '请输入有效手机号'
    return
  }
  sendingCode.value = true
  try {
    await api.sendSMSCode({ phone: resetPhoneValue.value, purpose: 'reset_password' })
    message.value = '验证码已发送，请注意查收'
    startCountdown()
  } catch (error) {
    setResetPasswordError(error)
  } finally {
    sendingCode.value = false
  }
}

async function submit() {
  if (loading.value || submitDisabled.value) {
    return
  }

  message.value = ''
  errorMessage.value = ''
  loading.value = true
  try {
    if (isResetPassword.value) {
      await api.resetPassword({
        phone: resetPhoneValue.value,
        verification_code: resetCodeValue.value,
        new_password: resetNewPassword.value
      })
      const phone = resetPhoneValue.value
      resetPasswordMode.value = false
      loginUsername.value = phone
      loginPassword.value = ''
      message.value = '密码已重置，请使用新密码登录'
      resetResetPasswordForm()
      return
    }
    let payload
    if (isRegister.value) {
      payload = await api.registerPhone({
        phone: registerPhoneValue.value,
        verification_code: registerCodeValue.value,
        username: registerUsername.value.trim(),
        password: registerPassword.value,
        invite_code: registerInviteCode.value.trim().toUpperCase()
      })
    } else {
      payload = await api.login(loginUsername.value.trim(), loginPassword.value, {
        captcha_id: loginCaptchaId.value,
        captcha_code: loginCaptchaCode.value.trim()
      }, { rememberLogin: rememberLogin.value })
    }
    setCurrentUser(payload)
    emit('authenticated', payload)
  } catch (error) {
    if (isResetPassword.value) {
      setResetPasswordError(error)
    } else if (isRegister.value) {
      setRegistrationError(error)
    } else {
      errorMessage.value = error?.message || '请求失败'
    }
    if (!isRegister.value && !isResetPassword.value) {
      await refreshLoginCaptcha()
    }
  } finally {
    loading.value = false
  }
}

async function refreshLoginCaptcha() {
  if (isRegister.value || loginCaptchaLoading.value) {
    return
  }
  loginCaptchaLoading.value = true
  try {
    const payload = await api.getCaptcha('user_login')
    loginCaptchaId.value = payload.captcha_id || ''
    loginCaptchaImage.value = payload.image_base64 || ''
    loginCaptchaCode.value = ''
  } catch (error) {
    loginCaptchaId.value = ''
    loginCaptchaImage.value = ''
    loginCaptchaCode.value = ''
    errorMessage.value = error?.message || '图形验证码加载失败'
  } finally {
    loginCaptchaLoading.value = false
  }
}

function resetResetPasswordForm() {
  resetPhone.value = ''
  resetPhoneRemoteHint.value = ''
  resetCode.value = ''
  resetCodeRemoteHint.value = ''
  resetNewPassword.value = ''
  resetPasswordRemoteHint.value = ''
  resetConfirmPassword.value = ''
}

function enterResetPasswordMode() {
  if (isRegister.value) {
    return
  }
  resetPasswordMode.value = true
  message.value = ''
  errorMessage.value = ''
  resetPhone.value = normalizePhone(loginUsername.value)
}

function exitResetPasswordMode() {
  resetPasswordMode.value = false
  resetResetPasswordForm()
  message.value = ''
  errorMessage.value = ''
}

function selectAuthMode(mode) {
  activeMode.value = mode === 'register' ? 'register' : 'login'
  if (activeMode.value === 'login') {
    exitResetPasswordMode()
  }
  emit('mode-change', activeMode.value)
}

function enterRegisterPhoneReset() {
  activeMode.value = 'login'
  resetPasswordMode.value = true
  resetPhone.value = registerPhoneValue.value
  message.value = ''
  errorMessage.value = ''
  emit('mode-change', 'login')
}

function firstQueryValue(value) {
  return Array.isArray(value) ? value[0] : value
}

function applyResetPasswordQuery() {
  if (isRegister.value) {
    return
  }
  if (!props.initialReset) {
    return
  }
  resetPasswordMode.value = true
  message.value = ''
  errorMessage.value = ''
  const phone = normalizePhone(firstQueryValue(props.initialResetPhone))
  resetPhone.value = validPhone(phone) ? phone : ''
}

onBeforeUnmount(() => {
  if (countdownTimer) {
    clearInterval(countdownTimer)
    countdownTimer = null
  }
})

onMounted(() => {
  if (!isRegister.value) {
    applyResetPasswordQuery()
    refreshLoginCaptcha()
  }
})

watch(
  () => props.mode,
  (mode) => {
    activeMode.value = mode === 'register' ? 'register' : 'login'
  }
)

watch(isRegister, (value) => {
  if (value) {
    loginCaptchaId.value = ''
    loginCaptchaImage.value = ''
    loginCaptchaCode.value = ''
    resetPasswordMode.value = false
    resetResetPasswordForm()
    return
  }
  applyResetPasswordQuery()
  refreshLoginCaptcha()
})

watch(registerPhone, () => {
  registerPhoneBlockedValue.value = ''
  registerCodeRemoteHint.value = ''
  if (registerPhoneRemoteHint.value) {
    registerPhoneRemoteHint.value = ''
  }
  if (errorMessage.value.includes('手机号已注册')) {
    errorMessage.value = ''
  }
})

watch(registerCode, () => {
  registerCodeRemoteHint.value = ''
})

watch(resetPhone, () => {
  resetPhoneRemoteHint.value = ''
})

watch(resetCode, () => {
  resetCodeRemoteHint.value = ''
})

watch(resetNewPassword, () => {
  resetPasswordRemoteHint.value = ''
})
</script>

<template>
  <section
    :class="[
      'auth-page',
      'auth-agent-page',
      isRegister ? 'auth-agent-page-register' : 'auth-agent-page-login'
    ]"
  >
    <form :class="['auth-card', isRegister ? 'auth-card-register' : 'auth-card-login']" @submit.prevent="submit">
      <div class="auth-mode-switch" aria-label="登录或注册切换">
        <a
          :class="['auth-mode-link', { 'auth-mode-link-active': !isRegister }]"
          href="/login"
          @click.prevent="selectAuthMode('login')"
        >
          登录
        </a>
        <a
          :class="['auth-mode-link', { 'auth-mode-link-active': isRegister }]"
          href="/register"
          @click.prevent="selectAuthMode('register')"
        >
          注册
        </a>
      </div>

      <div class="auth-card-head">
        <h1>{{ formTitle }}</h1>
        <p>{{ formDescription }}</p>
      </div>

      <template v-if="isRegister">
        <label class="auth-field" for="register-user">
          <span>账号</span>
          <div class="auth-input-shell">
            <b aria-hidden="true"><UserRound :size="20" :stroke-width="1.9" /></b>
            <input
              id="register-user"
              v-model="registerUsername"
              autocomplete="username"
              placeholder="请输入账号"
            />
          </div>
        </label>
        <p v-if="registerUsernameHint" class="field-hint field-hint-error" aria-live="polite">
          {{ registerUsernameHint }}
        </p>

        <div data-testid="auth-register-phone-field">
          <label class="auth-field" for="register-phone">
            <span>手机号</span>
            <div class="auth-input-shell">
              <b aria-hidden="true"><Phone :size="20" :stroke-width="1.9" /></b>
              <input
                id="register-phone"
                v-model="registerPhone"
                autocomplete="tel"
                inputmode="tel"
                maxlength="11"
                placeholder="请输入大陆手机号"
              />
            </div>
          </label>
          <p v-if="registerPhoneBlocked" class="field-hint field-hint-error" aria-live="polite">
            手机号已注册，请
            <a data-testid="auth-register-login-link" href="/login" @click.prevent="selectAuthMode('login')">立即登录</a>
            或
            <a data-testid="auth-register-reset-link" :href="`${registerResetLinkTo.path}?reset=1&phone=${registerPhoneValue}`" @click.prevent="enterRegisterPhoneReset">找回密码</a>
          </p>
          <p v-else-if="registerPhoneHint" class="field-hint field-hint-error" aria-live="polite">
            {{ registerPhoneHint }}
          </p>
        </div>

        <div data-testid="auth-register-code-field">
          <label class="auth-field" for="register-code">
            <span>短信验证码</span>
            <div class="auth-input-shell auth-code-shell">
              <b aria-hidden="true"><Hash :size="20" :stroke-width="1.9" /></b>
              <input
                id="register-code"
                v-model="registerCode"
                autocomplete="one-time-code"
                inputmode="numeric"
                maxlength="6"
                placeholder="请输入 6 位验证码"
              />
              <button
                class="auth-code-button"
                data-testid="auth-send-register-code"
                type="button"
                :disabled="sendingCode || codeCountdown > 0 || !registerPhoneValid || registerPhoneBlocked"
                @click="sendRegisterCode"
              >
                {{ codeButtonText }}
              </button>
            </div>
          </label>
          <p v-if="registerCodeHint" class="field-hint field-hint-error" aria-live="polite">
            {{ registerCodeHint }}
          </p>
        </div>

        <label class="auth-field" for="register-password">
          <span>密码</span>
          <div class="auth-input-shell">
            <b aria-hidden="true"><LockKeyhole :size="20" :stroke-width="1.9" /></b>
            <input
              id="register-password"
              v-model="registerPassword"
              :type="passwordInputType"
              autocomplete="new-password"
              placeholder="请输入密码"
              aria-describedby="register-password-hint"
            />
            <button
              class="auth-icon-button"
              data-testid="auth-toggle-password"
              type="button"
              aria-label="切换密码可见性"
              @click="showPassword = !showPassword"
            >
              <EyeOff v-if="showPassword" :size="20" :stroke-width="1.9" aria-hidden="true" />
              <Eye v-else :size="20" :stroke-width="1.9" aria-hidden="true" />
            </button>
          </div>
        </label>
        <p
          id="register-password-hint"
          :class="['field-hint', `field-hint-${registerPasswordHintTone}`]"
          aria-live="polite"
        >
          {{ registerPasswordHint }}
        </p>

        <label class="auth-field" for="register-confirm-password">
          <span>确认密码</span>
          <div class="auth-input-shell">
            <b aria-hidden="true"><KeyRound :size="20" :stroke-width="1.9" /></b>
            <input
              id="register-confirm-password"
              v-model="registerConfirmPassword"
              :type="passwordInputType"
              autocomplete="new-password"
              placeholder="请再次输入密码"
            />
            <button
              class="auth-icon-button"
              type="button"
              aria-label="切换确认密码可见性"
              @click="showPassword = !showPassword"
            >
              <EyeOff v-if="showPassword" :size="20" :stroke-width="1.9" aria-hidden="true" />
              <Eye v-else :size="20" :stroke-width="1.9" aria-hidden="true" />
            </button>
          </div>
        </label>
        <p v-if="registerConfirmPasswordHint" class="field-hint field-hint-error" aria-live="polite">
          {{ registerConfirmPasswordHint }}
        </p>

        <label class="auth-field" for="register-invite-code">
          <span>邀请码（可选）</span>
          <div class="auth-input-shell">
            <b aria-hidden="true"><Ticket :size="20" :stroke-width="1.9" /></b>
            <input
              id="register-invite-code"
              v-model="registerInviteCode"
              autocomplete="off"
              placeholder="有邀请码可填写"
            />
          </div>
        </label>
      </template>

      <template v-else-if="isResetPassword">
        <div data-testid="auth-reset-phone-field">
          <label class="auth-field" for="reset-phone">
            <span>手机号</span>
            <div class="auth-input-shell">
              <b aria-hidden="true"><Phone :size="20" :stroke-width="1.9" /></b>
              <input
                id="reset-phone"
                v-model="resetPhone"
                autocomplete="tel"
                inputmode="tel"
                maxlength="11"
                placeholder="请输入绑定手机号"
              />
            </div>
          </label>
          <p v-if="resetPhoneHint" class="field-hint field-hint-error" aria-live="polite">
            {{ resetPhoneHint }}
          </p>
        </div>

        <label class="auth-field" for="reset-code">
          <span>短信验证码</span>
          <div class="auth-input-shell auth-code-shell">
            <b aria-hidden="true"><Hash :size="20" :stroke-width="1.9" /></b>
            <input
              id="reset-code"
              v-model="resetCode"
              autocomplete="one-time-code"
              inputmode="numeric"
              maxlength="6"
              placeholder="请输入 6 位验证码"
            />
            <button
              class="auth-code-button"
              data-testid="auth-send-reset-code"
              type="button"
              :disabled="sendingCode || codeCountdown > 0 || !resetPhoneValid"
              @click="sendResetCode"
            >
              {{ codeButtonText }}
            </button>
          </div>
        </label>
        <p v-if="resetCodeHint" class="field-hint field-hint-error" aria-live="polite">
          {{ resetCodeHint }}
        </p>

        <label class="auth-field" for="reset-password">
          <span>新密码</span>
          <div class="auth-input-shell">
            <b aria-hidden="true"><LockKeyhole :size="20" :stroke-width="1.9" /></b>
            <input
              id="reset-password"
              v-model="resetNewPassword"
              :type="passwordInputType"
              autocomplete="new-password"
              placeholder="请输入新密码"
              aria-describedby="reset-password-hint"
            />
            <button
              class="auth-icon-button"
              data-testid="auth-toggle-password"
              type="button"
              aria-label="切换密码可见性"
              @click="showPassword = !showPassword"
            >
              <EyeOff v-if="showPassword" :size="20" :stroke-width="1.9" aria-hidden="true" />
              <Eye v-else :size="20" :stroke-width="1.9" aria-hidden="true" />
            </button>
          </div>
        </label>
        <p
          id="reset-password-hint"
          :class="['field-hint', `field-hint-${resetPasswordHintTone}`]"
          aria-live="polite"
        >
          {{ resetPasswordHint }}
        </p>

        <label class="auth-field" for="reset-confirm-password">
          <span>确认密码</span>
          <div class="auth-input-shell">
            <b aria-hidden="true"><KeyRound :size="20" :stroke-width="1.9" /></b>
            <input
              id="reset-confirm-password"
              v-model="resetConfirmPassword"
              :type="passwordInputType"
              autocomplete="new-password"
              placeholder="请再次输入新密码"
            />
            <button
              class="auth-icon-button"
              type="button"
              aria-label="切换确认密码可见性"
              @click="showPassword = !showPassword"
            >
              <EyeOff v-if="showPassword" :size="20" :stroke-width="1.9" aria-hidden="true" />
              <Eye v-else :size="20" :stroke-width="1.9" aria-hidden="true" />
            </button>
          </div>
        </label>
        <p v-if="resetConfirmPasswordHint" class="field-hint field-hint-error" aria-live="polite">
          {{ resetConfirmPasswordHint }}
        </p>
      </template>

      <template v-else>
        <label class="auth-field" for="login-user">
          <span>账号</span>
          <div class="auth-input-shell">
            <b aria-hidden="true"><UserRound :size="20" :stroke-width="1.9" /></b>
            <input id="login-user" v-model="loginUsername" autocomplete="username" placeholder="请输入账号" />
          </div>
        </label>

        <label class="auth-field" for="login-password">
          <span>密码</span>
          <div class="auth-input-shell">
            <b aria-hidden="true"><LockKeyhole :size="20" :stroke-width="1.9" /></b>
            <input
              id="login-password"
              v-model="loginPassword"
              :type="passwordInputType"
              autocomplete="current-password"
              placeholder="请输入密码"
            />
            <button
              class="auth-icon-button"
              data-testid="auth-toggle-password"
              type="button"
              aria-label="切换密码可见性"
              @click="showPassword = !showPassword"
            >
              <EyeOff v-if="showPassword" :size="20" :stroke-width="1.9" aria-hidden="true" />
              <Eye v-else :size="20" :stroke-width="1.9" aria-hidden="true" />
            </button>
          </div>
        </label>

        <label class="auth-field" for="login-captcha">
          <span>图形验证码</span>
          <div class="auth-input-shell auth-captcha-shell">
            <b aria-hidden="true"><ShieldCheck :size="20" :stroke-width="1.9" /></b>
            <input
              id="login-captcha"
              v-model="loginCaptchaCode"
              autocomplete="off"
              inputmode="text"
              maxlength="5"
              placeholder="输入验证码"
            />
            <button
              class="auth-captcha-image-button"
              data-testid="auth-refresh-captcha"
              type="button"
              :disabled="loginCaptchaLoading"
              aria-label="刷新图形验证码"
              @click="refreshLoginCaptcha"
            >
              <img
                v-if="loginCaptchaImageSrc"
                data-testid="auth-captcha-image"
                :src="loginCaptchaImageSrc"
                alt="图形验证码"
              />
              <span v-else>
                {{ loginCaptchaLoading ? '...' : '' }}
                <RefreshCcw
                  v-if="!loginCaptchaLoading"
                  :size="18"
                  :stroke-width="2"
                  aria-hidden="true"
                />
              </span>
            </button>
          </div>
        </label>
      </template>

      <div v-if="isRegister" class="auth-register-consent-row">
        <label class="auth-check auth-check-wrap">
          <input v-model="acceptedTerms" data-testid="auth-accept-terms" type="checkbox" />
          <span>
            我已阅读并同意
            <a data-testid="auth-terms-link" href="/terms">《用户协议》</a>
            与
            <a data-testid="auth-privacy-link" href="/privacy">《隐私政策》</a>
            及
            <a data-testid="auth-algorithm-link" href="/algorithm-disclosure">《算法公示》</a>
          </span>
        </label>
      </div>

      <div v-else-if="isResetPassword" class="auth-form-options">
        <button
          class="auth-text-link"
          data-testid="auth-back-login"
          type="button"
          @click="exitResetPasswordMode"
        >
          返回登录
        </button>
      </div>

      <div v-else class="auth-form-options">
        <label class="auth-check">
          <input v-model="rememberLogin" data-testid="auth-remember-login" type="checkbox" />
          <span>记住登录</span>
        </label>
        <button
          v-if="!isRegister"
          class="auth-text-link"
          data-testid="auth-forgot-password"
          type="button"
          @click="enterResetPasswordMode"
        >
          忘记密码？
        </button>
      </div>

      <button class="auth-submit-button primary-button" type="submit" :disabled="loading || submitDisabled">
        {{ submitLabel }}
      </button>

      <div class="auth-divider">
        <span></span>
        <p>
          {{ accountPrompt }}
          <a :href="accountPromptTo" @click.prevent="selectAuthMode(isRegister ? 'login' : 'register')">{{ accountPromptLink }}</a>
        </p>
        <span></span>
      </div>

      <p class="auth-safe-note">登录与注册可在同一页面完成，安全便捷。</p>
      <p v-if="message" class="status-success">{{ message }}</p>
      <p v-if="errorMessage" class="status-error">{{ errorMessage }}</p>
    </form>
  </section>
</template>
