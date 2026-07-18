import { openAuthModal } from './stores/auth-modal.js'

export function resolveSafeRedirect(value, fallback = '/workspace') {
  const redirect = typeof value === 'string' ? value.trim() : ''
  if (!redirect || !redirect.startsWith('/') || redirect.startsWith('//') || redirect.includes('\\')) {
    return fallback
  }
  return redirect
}

export function loginRouteWithRedirect(redirect = '/workspace') {
  return {
    path: '/login',
    query: {
      redirect: resolveSafeRedirect(redirect)
    }
  }
}

export function authModalRouteWithRedirect(redirect = '/workspace', mode = 'login') {
  return {
    path: '/workspace',
    query: {
      auth: mode === 'register' ? 'register' : 'login',
      redirect: resolveSafeRedirect(redirect)
    }
  }
}

export function confirmLoginBeforeUse(router, redirect = '/workspace') {
  openAuthModal({
    mode: 'login',
    redirect,
    navigateOnSuccess: false,
    message: '需要登录才能使用该功能'
  })
  return true
}
