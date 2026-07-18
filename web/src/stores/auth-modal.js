import { reactive } from 'vue'

export const authModalState = reactive({
  open: false,
  mode: 'login',
  redirect: '/workspace',
  navigateOnSuccess: false,
  message: '需要登录才能使用该功能'
})

export function openAuthModal(options = {}) {
  authModalState.open = true
  authModalState.mode = options.mode === 'register' ? 'register' : 'login'
  authModalState.redirect = resolveModalRedirect(options.redirect, '/workspace')
  authModalState.navigateOnSuccess = Boolean(options.navigateOnSuccess)
  authModalState.message = options.message || '需要登录才能使用该功能'
}

export function closeAuthModal() {
  authModalState.open = false
  authModalState.navigateOnSuccess = false
}

function resolveModalRedirect(value, fallback) {
  const redirect = typeof value === 'string' ? value.trim() : ''
  if (!redirect || !redirect.startsWith('/') || redirect.startsWith('//') || redirect.includes('\\')) {
    return fallback
  }
  return redirect
}
