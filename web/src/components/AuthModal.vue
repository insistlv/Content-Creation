<script setup>
import { onBeforeUnmount, onMounted } from 'vue'
import { X } from 'lucide-vue-next'
import { useRoute, useRouter } from 'vue-router'

import AuthForm from './AuthForm.vue'
import { authModalState, closeAuthModal } from '../stores/auth-modal.js'
import { loadCurrentUser, setCurrentUser } from '../stores/session.js'

const route = useRoute()
const router = useRouter()

function setMode(mode) {
  authModalState.mode = mode === 'register' ? 'register' : 'login'
}

async function handleAuthenticated(payload) {
  const redirect = authModalState.redirect
  const navigateOnSuccess = authModalState.navigateOnSuccess
  setCurrentUser(payload)
  try {
    await loadCurrentUser({ force: true, clearOnError: false })
  } catch {
    // 登录接口已经成功，刷新用户信息失败不阻塞用户继续操作。
  }
  closeAuthModal()
  clearAuthQuery()
  if (navigateOnSuccess) {
    router.push(redirect)
  }
}

function handleClose() {
  closeAuthModal()
  clearAuthQuery()
}

function clearAuthQuery() {
  if (!route.query?.auth) return
  const nextQuery = { ...route.query }
  delete nextQuery.auth
  delete nextQuery.redirect
  if (typeof router.replace === 'function') {
    router.replace({ path: route.path || '/workspace', query: nextQuery })
  }
}

function handleKeydown(event) {
  if (event.key === 'Escape' && authModalState.open) {
    handleClose()
  }
}

onMounted(() => {
  window.addEventListener('keydown', handleKeydown)
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', handleKeydown)
})
</script>

<template>
  <Teleport to="body">
    <div
      v-if="authModalState.open"
      class="auth-modal-backdrop"
      data-testid="auth-modal-backdrop"
      @click="handleClose"
    >
      <section
        class="auth-modal"
        data-testid="auth-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="auth-modal-title"
        @click.stop
      >
        <button
          class="auth-modal-close"
          type="button"
          data-testid="auth-modal-close"
          aria-label="关闭登录注册弹窗"
          @click="handleClose"
        >
          <X :size="20" />
        </button>
        <div class="auth-modal-copy">
          <p>登录提示</p>
          <h2 id="auth-modal-title">{{ authModalState.message }}</h2>
          <span>登录或注册后即可继续使用创作、上传、作品管理等功能。</span>
        </div>
        <AuthForm
          :mode="authModalState.mode"
          @mode-change="setMode"
          @authenticated="handleAuthenticated"
        />
      </section>
    </div>
  </Teleport>
</template>
