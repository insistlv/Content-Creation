import { authModalRouteWithRedirect } from './auth-navigation.js'

export function buildGuard({ ensureUser, ensureAdmin }) {
  return async (to) => {
    if (to.meta?.auth === 'user') {
      return (await ensureUser()) ? true : authModalRouteWithRedirect(to.fullPath || to.path || '/workspace')
    }

    if (to.meta?.auth === 'admin') {
      const result = await ensureAdmin(to.meta?.adminPermission)
      if (result === true) {
        return true
      }
      if (result?.authenticated && result?.authorized) {
        return true
      }
      if (result?.authenticated && !result?.authorized) {
        return { path: '/admin/forbidden' }
      }
      return { path: '/admin/login' }
    }

    return true
  }
}
