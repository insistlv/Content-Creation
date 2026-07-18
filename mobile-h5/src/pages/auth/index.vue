<script setup>
import { computed, ref } from 'vue'
import { onLoad, onUnload } from '@dcloudio/uni-app'

import { api } from '../../api/client.js'
import { navigateTo, routes } from '../../utils/routes.js'

const staticAssetBaseURL = `${import.meta.env.VITE_STATIC_ASSET_BASE_URL || ''}`.replace(/\/+$/, '')

function staticAsset(path) {
  const normalizedPath = `${path || ''}`.trim().replace(/^\/+/, '').replace(/^static\/+/i, '')
  if (!normalizedPath) return staticAssetBaseURL
  if (staticAssetBaseURL) return `${staticAssetBaseURL}/${normalizedPath}`
  return `/${['static', normalizedPath].join('/')}`
}

const mode = ref('login')
const heroImage = staticAsset('auth-hero-crystal.png')
const redirect = ref(routes.account)
const submitting = ref(false)
const wechatPhoneSubmitting = ref(false)
const sendingCode = ref(false)
const errorMessage = ref('')
const rememberLogin = ref(true)
const codeCountdown = ref(0)
let countdownTimer = null

const loginForm = ref({
  identifier: '',
  password: ''
})

const registerForm = ref({
  phone: '',
  code: '',
  username: '',
  password: '',
  confirmPassword: '',
  inviteCode: ''
})

const resetForm = ref({
  phone: '',
  code: '',
  password: '',
  confirmPassword: ''
})

const submitText = computed(() => {
  if (mode.value === 'register') return '注册并进入'
  if (mode.value === 'reset') return '重置密码'
  return '登录'
})
const phoneQuickButtonText = computed(() => (
  wechatPhoneSubmitting.value ? '手机号验证中...' : '手机号快捷登录'
))
const codeButtonText = computed(() => {
  if (codeCountdown.value > 0) return `${codeCountdown.value}s`
  if (sendingCode.value) return '发送中'
  return '获取验证码'
})

function decodeQueryValue(value) {
  if (typeof value !== 'string') return ''
  try {
    return decodeURIComponent(value)
  } catch {
    return value
  }
}

function normalizeRedirect(value) {
  const decoded = decodeQueryValue(value).trim()
  if (!decoded || !decoded.startsWith('/pages/')) return routes.account
  if (decoded.startsWith(routes.auth)) return routes.account
  return decoded
}

function setMode(nextMode) {
  errorMessage.value = ''
  if (nextMode === 'login') {
    mode.value = 'login'
    return
  }
  if (nextMode === 'register') {
    mode.value = 'register'
    return
  }
  mode.value = nextMode
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

function normalizePhone(value) {
  return String(value || '').trim()
}

function validPhone(value) {
  return /^1[3-9]\d{9}$/.test(normalizePhone(value))
}

function validRegisterUsername(value) {
  return /^[A-Za-z0-9][A-Za-z0-9_.-]{2,31}$/.test(String(value || '').trim())
}

function currentCodePhone() {
  return mode.value === 'reset' ? resetForm.value.phone : registerForm.value.phone
}

function currentPurpose() {
  return mode.value === 'reset' ? 'reset_password' : 'register'
}

async function sendCode() {
  if (sendingCode.value || codeCountdown.value > 0) return
  errorMessage.value = ''
  const phone = normalizePhone(currentCodePhone())
  if (!validPhone(phone)) {
    errorMessage.value = '请输入正确的大陆手机号'
    return
  }
  sendingCode.value = true
  try {
    await api.sendSMSCode({ phone, purpose: currentPurpose() })
    showToast('验证码已发送，请注意查收')
    startCountdown()
  } catch (error) {
    const message = error.message || '验证码发送失败，请稍后重试'
    errorMessage.value = message
    showToast(message)
  } finally {
    sendingCode.value = false
  }
}

function goHome() {
  navigateTo(routes.home)
}

function showToast(titleText) {
  uni.showToast({ title: titleText, icon: 'none' })
}

function uniLogin() {
  return new Promise((resolve, reject) => {
    uni.login({
      provider: 'weixin',
      success(result) {
        if (result?.code) {
          resolve(result.code)
          return
        }
        reject(new Error('登录失败，请稍后重试'))
      },
      fail(error) {
        reject(error)
      }
    })
  })
}

function wechatPhoneLoginErrorMessage(error) {
  const code = `${error?.code || ''}`
  if (code === 'wechat_phone_code_invalid') return '授权已失效，请重新授权手机号'
  if (
    code === 'wechat_phone_capability_unavailable' ||
    code === 'wechat_phone_token_failed' ||
    code === 'wechat_phone_failed'
  ) {
    return '手机号快捷登录暂不可用，请改用短信验证码'
  }
  if (code === 'wechat_login_failed') return '登录失败，请稍后重试'
  return '登录失败，请稍后重试'
}

async function submitWechatPhoneLogin(event) {
  if (submitting.value || wechatPhoneSubmitting.value) return
  const phoneCode = event?.detail?.code
  if (!phoneCode) {
    const rawMessage = `${event?.detail?.errMsg || ''}`
    const message = rawMessage.includes('deny') || rawMessage.includes('cancel')
      ? '已取消手机号授权'
      : '请完成手机号授权'
    errorMessage.value = message
    showToast(message)
    return
  }
  errorMessage.value = ''
  wechatPhoneSubmitting.value = true
  try {
    const code = await uniLogin()
    await api.wechatPhoneLogin({ code, phone_code: phoneCode })
    showToast('登录成功')
    uni.redirectTo({ url: redirect.value })
  } catch (error) {
    const message = wechatPhoneLoginErrorMessage(error)
    errorMessage.value = message
    showToast(message)
  } finally {
    wechatPhoneSubmitting.value = false
  }
}

async function submitAuth() {
  if (submitting.value || wechatPhoneSubmitting.value) return
  errorMessage.value = ''

  submitting.value = true
  try {
    if (mode.value === 'login') {
      await submitLogin()
    } else if (mode.value === 'register') {
      await submitRegister()
    } else {
      await submitReset()
    }
  } catch (error) {
    errorMessage.value = error.message || '认证失败'
    showToast(errorMessage.value)
  } finally {
    submitting.value = false
  }
}

async function submitLogin() {
  const identifier = loginForm.value.identifier.trim()
  const password = loginForm.value.password.trim()
  if (!identifier || !password) {
    throw new Error('请输入手机号/账号和密码')
  }
  if (password.length < 8) {
    throw new Error('密码至少 8 位')
  }
  const payload = await api.login({ username: identifier, password })
  if (rememberLogin.value) {
    uni.setStorageSync('auth:last_identifier', identifier)
  } else {
    uni.removeStorageSync('auth:last_identifier')
  }
  if (!payload?.phone) {
    showToast('当前账号还未绑定手机号')
    uni.redirectTo({ url: `${routes.account}?bindPhone=1` })
    return
  }
  uni.redirectTo({ url: redirect.value })
}

async function submitRegister() {
  const phone = normalizePhone(registerForm.value.phone)
  const username = registerForm.value.username.trim()
  const password = registerForm.value.password.trim()
  const confirmPassword = registerForm.value.confirmPassword.trim()
  const code = registerForm.value.code.trim()
  if (!validPhone(phone)) throw new Error('请输入正确的大陆手机号')
  if (!/^\d{6}$/.test(code)) throw new Error('请输入 6 位短信验证码')
  if (!username || !password) throw new Error('请填写账号和密码')
  if (!validRegisterUsername(username)) throw new Error('账号只能使用 3-32 位字母、数字、下划线、点或横线')
  if (password.length < 8) throw new Error('密码至少 8 位')
  if (password !== confirmPassword) throw new Error('两次输入的密码不一致')
  await api.registerPhone({
    phone,
    verification_code: code,
    username,
    password,
    invite_code: registerForm.value.inviteCode.trim()
  })
  uni.setStorageSync('auth:last_identifier', phone)
  uni.redirectTo({ url: redirect.value })
}

async function submitReset() {
  const phone = normalizePhone(resetForm.value.phone)
  const password = resetForm.value.password.trim()
  const confirmPassword = resetForm.value.confirmPassword.trim()
  const code = resetForm.value.code.trim()
  if (!validPhone(phone)) throw new Error('请输入正确的大陆手机号')
  if (!/^\d{6}$/.test(code)) throw new Error('请输入 6 位短信验证码')
  if (password.length < 8) throw new Error('密码至少 8 位')
  if (password !== confirmPassword) throw new Error('两次输入的密码不一致')
  await api.resetPassword({
    phone,
    verification_code: code,
    new_password: password
  })
  loginForm.value.identifier = phone
  loginForm.value.password = ''
  setMode('login')
  showToast('密码已重置，请重新登录')
}

onLoad((query = {}) => {
  const lastIdentifier = uni.getStorageSync('auth:last_identifier')
  if (lastIdentifier) loginForm.value.identifier = lastIdentifier
  mode.value = query.mode === 'register' ? 'register' : 'login'
  redirect.value = normalizeRedirect(query.redirect)
})

onUnload(() => {
  if (countdownTimer) clearInterval(countdownTimer)
})
</script>

<template>
  <view class="auth-page">
    <view class="auth-shell">
      <view class="auth-topbar">
        <button type="button" class="ghost-icon-button" @click="goHome">‹</button>
        <view class="brand-lockup">
          <text class="brand-mark">IA</text>
          <text>白霖共享</text>
        </view>
      </view>

      <view class="hero-stage">
        <image class="hero-image" :src="heroImage" mode="aspectFill" />
        <view class="hero-copy">
          <text>白霖共享</text>
          <text>让每一次灵感快速成片</text>
        </view>
      </view>

      <view class="auth-panel">
        <button
          type="button"
          class="phone-quick-auth-button"
          open-type="getPhoneNumber"
          :disabled="submitting || wechatPhoneSubmitting"
          @getphonenumber="submitWechatPhoneLogin"
        >
          <text>{{ phoneQuickButtonText }}</text>
        </button>

        <view class="phone-quick-auth-divider">
          <text>其他方式</text>
        </view>

        <view class="mode-tabs">
          <view :class="{ active: mode === 'login' }" @click="setMode('login')" @tap="setMode('login')">登录</view>
          <view :class="{ active: mode === 'register' }" @click="setMode('register')" @tap="setMode('register')">注册</view>
        </view>

        <view v-if="mode === 'login'" class="auth-form">
          <label>
            <text>手机号 / 账号</text>
            <input v-model="loginForm.identifier" type="text" placeholder="输入手机号或账号" confirm-type="next" />
          </label>
          <label>
            <text>密码</text>
            <input v-model="loginForm.password" type="password" placeholder="至少 8 位" confirm-type="done" />
          </label>
          <view class="form-row">
            <label class="remember-row">
              <checkbox :checked="rememberLogin" color="#111827" @click="rememberLogin = !rememberLogin" />
              <text>记住登录</text>
            </label>
            <view class="link-button" @click="setMode('reset')" @tap="setMode('reset')">忘记密码</view>
          </view>
        </view>

        <view v-else-if="mode === 'register'" class="auth-form">
          <label>
            <text>手机号</text>
            <input v-model="registerForm.phone" type="number" maxlength="11" placeholder="输入大陆手机号" confirm-type="next" />
          </label>
          <label>
            <text>短信验证码</text>
            <view class="code-field">
              <input v-model="registerForm.code" type="number" maxlength="6" placeholder="6 位验证码" confirm-type="next" />
              <button type="button" class="code-button" :disabled="sendingCode || codeCountdown > 0" @click="sendCode">{{ codeButtonText }}</button>
            </view>
          </label>
          <label>
            <text>账号</text>
            <input v-model="registerForm.username" type="text" placeholder="设置登录账号" confirm-type="next" />
          </label>
          <label>
            <text>密码</text>
            <input v-model="registerForm.password" type="password" placeholder="至少 8 位" confirm-type="next" />
          </label>
          <label>
            <text>确认密码</text>
            <input v-model="registerForm.confirmPassword" type="password" placeholder="再次输入密码" confirm-type="next" />
          </label>
          <label>
            <text>邀请码</text>
            <input v-model="registerForm.inviteCode" type="text" placeholder="可选" confirm-type="done" />
          </label>
        </view>

        <view v-else class="auth-form">
          <label>
            <text>手机号</text>
            <input v-model="resetForm.phone" type="number" maxlength="11" placeholder="输入已注册手机号" confirm-type="next" />
          </label>
          <label>
            <text>短信验证码</text>
            <view class="code-field">
              <input v-model="resetForm.code" type="number" maxlength="6" placeholder="6 位验证码" confirm-type="next" />
              <button type="button" class="code-button" :disabled="sendingCode || codeCountdown > 0" @click="sendCode">{{ codeButtonText }}</button>
            </view>
          </label>
          <label>
            <text>新密码</text>
            <input v-model="resetForm.password" type="password" placeholder="至少 8 位" confirm-type="next" />
          </label>
          <label>
            <text>确认新密码</text>
            <input v-model="resetForm.confirmPassword" type="password" placeholder="再次输入新密码" confirm-type="done" />
          </label>
        </view>

        <text v-if="errorMessage" class="error-message">{{ errorMessage }}</text>

        <button type="button" class="primary-button" :disabled="submitting" @click="submitAuth">
          <text>{{ submitting ? '处理中...' : submitText }}</text>
        </button>

        <view class="bottom-entry">
          <template v-if="mode === 'login'">
            <text>还没有账号？</text>
            <view class="entry-link" @click="setMode('register')" @tap="setMode('register')">立即注册</view>
          </template>
          <template v-else>
            <text>已有账号？</text>
            <view class="entry-link" @click="setMode('login')" @tap="setMode('login')">返回登录</view>
          </template>
        </view>
      </view>
    </view>
  </view>
</template>

<style lang="scss" scoped>
@use '../../styles/tokens.scss' as *;

.auth-page {
  min-height: 100vh;
  background:
    radial-gradient(circle at 8% 2%, rgba(187, 223, 255, 0.7), transparent 35%),
    radial-gradient(circle at 94% 9%, rgba(255, 224, 182, 0.62), transparent 31%),
    linear-gradient(180deg, #f5fbff 0%, #edf5f0 48%, #f8faf7 100%);
  color: #0f172a;
}

.auth-page,
.auth-page view,
.auth-page button,
.auth-page input,
.auth-page text {
  box-sizing: border-box;
}

.auth-page button {
  margin: 0;
  padding: 0;
  border: 0;
  line-height: 1.2;
}

.auth-page button::after {
  border: 0;
}

.auth-shell {
  min-height: 100vh;
  padding: calc(20rpx + $phone-float-clearance-top + env(safe-area-inset-top)) 28rpx 34rpx;
  display: flex;
  flex-direction: column;
}

.auth-topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 70rpx;
  z-index: 2;

  .ghost-icon-button {
    width: 62rpx;
    height: 64rpx;
    border-radius: 50%;
    background: rgba(255, 255, 255, 0.7);
    color: #0f172a;
    font-size: 48rpx;
    font-weight: 500;
    box-shadow: 0 12rpx 30rpx rgba(15, 23, 42, 0.08);
  }
}

.brand-lockup {
  display: flex;
  align-items: center;
  gap: 12rpx;

  text {
    font-size: 22rpx;
    font-weight: 800;
    color: rgba(15, 23, 42, 0.8);
  }

  .brand-mark {
    width: 44rpx;
    height: 44rpx;
    border-radius: 14rpx;
    display: flex;
    align-items: center;
    justify-content: center;
    background: #111827;
    color: #ffffff;
    font-size: 18rpx;
  }
}

.hero-stage {
  position: relative;
  height: 360rpx;
  margin: 12rpx -28rpx 0;
  overflow: hidden;
}

.hero-image {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  opacity: 0.98;
}

.hero-stage::after {
  content: '';
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  height: 160rpx;
  background: linear-gradient(180deg, rgba(248, 250, 247, 0) 0%, rgba(248, 250, 247, 0.92) 100%);
}

.hero-copy {
  position: absolute;
  left: 56rpx;
  right: 56rpx;
  bottom: 42rpx;
  z-index: 1;
  display: flex;
  flex-direction: column;
  gap: 10rpx;

  text:first-child {
    font-size: 52rpx;
    line-height: 1.05;
    font-weight: 900;
    color: #0b1220;
  }

  text:last-child {
    font-size: 24rpx;
    color: rgba(51, 65, 85, 0.78);
  }
}

.auth-panel {
  margin-top: -10rpx;
  width: 100%;
  border: 1rpx solid rgba(148, 163, 184, 0.18);
  border-radius: 30rpx;
  background: rgba(255, 255, 255, 0.88);
  box-shadow: 0 26rpx 72rpx rgba(15, 23, 42, 0.1);
  padding: 24rpx 28rpx 30rpx;
  backdrop-filter: blur(22rpx);
}

.phone-quick-auth-button {
  width: 100%;
  height: 88rpx;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 20rpx;
  background: linear-gradient(100deg, #2563ff 0%, #7c3aed 100%);
  color: #ffffff;
  font-size: 28rpx;
  font-weight: 900;
  box-shadow: 0 16rpx 34rpx rgba(79, 70, 229, 0.28);
}

.phone-quick-auth-button[disabled] {
  opacity: 0.62;
}

.phone-quick-auth-divider {
  display: flex;
  align-items: center;
  gap: 16rpx;
  margin: 22rpx 0 18rpx;
  color: #94a3b8;
  font-size: 22rpx;
  font-weight: 800;

  text {
    flex: none;
  }
}

.phone-quick-auth-divider::before,
.phone-quick-auth-divider::after {
  content: '';
  flex: 1;
  height: 1rpx;
  background: rgba(148, 163, 184, 0.32);
}

.mode-tabs {
  display: grid;
  grid-template-columns: 1fr 1fr;
  height: 68rpx;
  padding: 6rpx;
  margin-bottom: 26rpx;
  border-radius: 18rpx;
  background: #eef3f7;

  > view {
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 14rpx;
    background: transparent;
    color: #64748b;
    font-size: 24rpx;
    font-weight: 800;
  }

  > view.active {
    background: #ffffff;
    color: #101828;
    box-shadow: 0 8rpx 18rpx rgba(15, 23, 42, 0.08);
  }
}

.auth-form {
  display: flex;
  flex-direction: column;
  gap: 16rpx;

  label {
    display: flex;
    flex-direction: column;
    gap: 10rpx;
  }

  label > text {
    font-size: 24rpx;
    font-weight: 700;
    color: #334155;
  }

  input {
    width: 100%;
    height: 86rpx;
    border-radius: 18rpx;
    border: 1rpx solid rgba(148, 163, 184, 0.26);
    background: #f8fafc;
    padding: 0 24rpx;
    font-size: 28rpx;
    color: #0f172a;
  }
}

.code-field {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 192rpx;
  gap: 12rpx;
}

.code-button {
  width: 192rpx;
  height: 86rpx;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border-radius: 18rpx;
  background: linear-gradient(135deg, #0f766e 0%, #14b8a6 100%);
  color: #ffffff;
  font-size: 23rpx;
  font-weight: 800;
  line-height: 1;
  white-space: nowrap;
}

.code-button[disabled] {
  opacity: 0.58;
}

.form-row {
  height: 58rpx;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.remember-row {
  flex-direction: row !important;
  align-items: center;
  gap: 8rpx !important;

  checkbox {
    transform: scale(0.72);
    transform-origin: left center;
  }

  text {
    color: #475569;
    font-size: 23rpx;
    font-weight: 700;
  }
}

.link-button {
  min-width: 128rpx;
  height: 58rpx;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  background: transparent;
  color: #0f766e;
  font-size: 23rpx;
  font-weight: 800;
}

.error-message {
  display: block;
  margin-top: 18rpx;
  color: #dc2626;
  font-size: 24rpx;
}

.primary-button {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  height: 90rpx;
  margin-top: 26rpx;
  border-radius: 20rpx;
  background: linear-gradient(100deg, #2563ff 0%, #7c3aed 100%);
  color: #fff;
  font-size: 28rpx;
  font-weight: 800;
  box-shadow: 0 16rpx 34rpx rgba(79, 70, 229, 0.28);
}

.primary-button[disabled] {
  opacity: 0.62;
}

.bottom-entry {
  margin-top: 22rpx;
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20rpx;

  text {
    flex: 1;
    min-width: 0;
    color: #64748b;
    font-size: 24rpx;
  }

  .entry-link {
    flex: none;
    min-width: 112rpx;
    height: 48rpx;
    display: flex;
    align-items: center;
    justify-content: flex-end;
    color: #0f766e;
    font-size: 24rpx;
    font-weight: 900;
  }
}
</style>
