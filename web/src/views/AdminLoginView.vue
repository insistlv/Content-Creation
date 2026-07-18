<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  Activity,
  ArrowRight,
  BarChart3,
  Eye,
  EyeOff,
  LockKeyhole,
  RefreshCw,
  ShieldCheck,
  Sparkles,
  UserRound
} from 'lucide-vue-next'

import { api } from '../api/client.js'
import adminHeroImage from '../image/admin-login-hero.png'

const router = useRouter()
const username = ref('')
const password = ref('')
const captchaId = ref('')
const captchaCode = ref('')
const captchaImage = ref('')
const captchaLoading = ref(false)
const rememberLogin = ref(false)
const showPassword = ref(false)
const loading = ref(false)
const infoMessage = ref('')
const errorMessage = ref('')
const passwordInputType = computed(() => (showPassword.value ? 'text' : 'password'))
const captchaImageSrc = computed(() => (
  captchaImage.value ? `data:image/png;base64,${captchaImage.value}` : ''
))
const submitDisabled = computed(() => (
  !username.value.trim() ||
  !password.value ||
  !captchaId.value ||
  !captchaCode.value.trim() ||
  captchaLoading.value
))

const capabilities = [
  {
    icon: BarChart3,
    title: '数据可视化',
    text: '实时掌握订单、用户与生成任务。'
  },
  {
    icon: ShieldCheck,
    title: '权限安全',
    text: '管理员角色与敏感操作分级管控。'
  },
  {
    icon: Activity,
    title: '系统监控',
    text: '配置、模型与运行状态集中维护。'
  }
]

async function submit() {
  if (loading.value || submitDisabled.value) {
    return
  }
  infoMessage.value = ''
  errorMessage.value = ''
  loading.value = true
  try {
    await api.adminLogin(username.value.trim(), password.value, {
      captcha_id: captchaId.value,
      captcha_code: captchaCode.value.trim()
    }, { rememberLogin: rememberLogin.value })
    router.push('/admin')
  } catch (error) {
    errorMessage.value = error.message
    await refreshCaptcha()
  } finally {
    loading.value = false
  }
}

async function refreshCaptcha() {
  if (captchaLoading.value) {
    return
  }
  captchaLoading.value = true
  try {
    const payload = await api.getCaptcha('admin_login')
    captchaId.value = payload.captcha_id || ''
    captchaImage.value = payload.image_base64 || ''
    captchaCode.value = ''
  } catch (error) {
    captchaId.value = ''
    captchaImage.value = ''
    captchaCode.value = ''
    errorMessage.value = error?.message || '图形验证码加载失败'
  } finally {
    captchaLoading.value = false
  }
}

function showForgotPasswordHint() {
  infoMessage.value = '请联系系统管理员重置密码。'
  errorMessage.value = ''
}

onMounted(() => {
  refreshCaptcha()
})
</script>

<template>
  <section class="admin-login-shell">
    <div class="admin-login-canvas">
      <header class="admin-login-topline">
        <div class="admin-login-brand">
          <div class="admin-brand-mark">IA</div>
          <div>
            <p class="eyebrow">白霖共享 Admin</p>
            <strong>潜核绘影管理端</strong>
          </div>
        </div>
        <div class="admin-login-secure-pill">
          <ShieldCheck :size="17" />
          <span>Secure Console</span>
        </div>
      </header>

      <div class="admin-login-layout">
        <aside class="admin-login-copy">
          <div class="admin-login-kicker">
            <Sparkles :size="16" />
            <span>Operations Control</span>
          </div>
          <h1>后台控制中心</h1>
          <p>
            面向平台运营、财务订单与系统配置的统一入口，登录后继续处理关键后台任务。
          </p>

          <div class="admin-login-capabilities" aria-label="后台能力">
            <article v-for="item in capabilities" :key="item.title" class="admin-login-capability">
              <component :is="item.icon" :size="19" />
              <div>
                <strong>{{ item.title }}</strong>
                <span>{{ item.text }}</span>
              </div>
            </article>
          </div>
        </aside>

        <div class="admin-login-hero" aria-label="后台登录视觉">
          <img
            class="admin-login-hero-image"
            :src="adminHeroImage"
            alt="透明玻璃方块组成的后台登录视觉"
          />
          <div class="admin-login-hero-badge">
            <ShieldCheck :size="18" />
            <span>Admin Access</span>
          </div>
        </div>

        <form class="admin-login-card" @submit.prevent="submit">
          <div class="admin-login-card-head">
            <p class="eyebrow">Sign in</p>
            <h2>管理员登录</h2>
            <span>使用管理员账号进入后台工作台。</span>
          </div>

          <label class="admin-login-field" for="adminUser">
            <span>用户名</span>
            <div class="admin-login-input">
              <UserRound :size="18" />
              <input id="adminUser" v-model="username" autocomplete="username" placeholder="请输入管理员账号" />
            </div>
          </label>

          <label class="admin-login-field" for="adminPass">
            <span>密码</span>
            <div class="admin-login-input">
              <LockKeyhole :size="18" />
              <input
                id="adminPass"
                v-model="password"
                :type="passwordInputType"
                autocomplete="current-password"
                placeholder="请输入登录密码"
              />
              <button
                class="admin-login-icon-button"
                data-testid="admin-toggle-password"
                type="button"
                :aria-label="showPassword ? '隐藏密码' : '显示密码'"
                @click="showPassword = !showPassword"
              >
                <EyeOff v-if="showPassword" :size="18" />
                <Eye v-else :size="18" />
              </button>
            </div>
          </label>

          <label class="admin-login-field" for="adminCaptcha">
            <span>图形验证码</span>
            <div class="admin-login-input admin-login-captcha-input">
              <ShieldCheck :size="18" />
              <input
                id="adminCaptcha"
                v-model="captchaCode"
                autocomplete="off"
                maxlength="5"
                placeholder="输入验证码"
              />
              <button
                class="admin-login-captcha-button"
                data-testid="admin-refresh-captcha"
                type="button"
                :disabled="captchaLoading"
                aria-label="刷新图形验证码"
                @click="refreshCaptcha"
              >
                <img
                  v-if="captchaImageSrc"
                  data-testid="admin-captcha-image"
                  :src="captchaImageSrc"
                  alt="图形验证码"
                />
                <RefreshCw v-else :size="18" />
              </button>
            </div>
          </label>

          <div class="admin-login-options">
            <label class="admin-login-check">
              <input v-model="rememberLogin" data-testid="admin-remember-login" type="checkbox" />
              <span>记住登录状态</span>
            </label>
            <button
              class="admin-login-text-button"
              data-testid="admin-forgot-password"
              type="button"
              @click="showForgotPasswordHint"
            >
              忘记密码？
            </button>
          </div>

          <button class="admin-login-submit primary-button" type="submit" :disabled="loading || submitDisabled">
            <span>{{ loading ? '登录中...' : '登录后台' }}</span>
            <ArrowRight :size="18" />
          </button>

          <p v-if="infoMessage" class="status-success admin-login-message" role="status">
            {{ infoMessage }}
          </p>
          <p v-if="errorMessage" class="status-error admin-login-message" role="alert">
            {{ errorMessage }}
          </p>
        </form>
      </div>

      <footer class="admin-login-footer">
        <span>受保护的后台入口</span>
        <span>所有登录行为将记录审计日志</span>
      </footer>
    </div>
  </section>
</template>
