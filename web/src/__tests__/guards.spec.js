import { describe, expect, it, vi } from 'vitest'

import { buildGuard } from '../guards.js'

describe('buildGuard', () => {
  it('redirects unauthenticated user routes to the workspace auth modal host with the original target', async () => {
    const guard = buildGuard({
      ensureUser: vi.fn().mockResolvedValue(false),
      ensureAdmin: vi.fn().mockResolvedValue(true)
    })

    const result = await guard({ meta: { auth: 'user' }, path: '/workspace/video', fullPath: '/workspace/video?seed=1' })
    expect(result).toEqual({
      path: '/workspace',
      query: {
        auth: 'login',
        redirect: '/workspace/video?seed=1'
      }
    })
  })

  it('allows admin routes when admin session is valid', async () => {
    const guard = buildGuard({
      ensureUser: vi.fn().mockResolvedValue(false),
      ensureAdmin: vi.fn().mockResolvedValue({ authenticated: true, authorized: true })
    })

    const result = await guard({ meta: { auth: 'admin', adminPermission: 'settings.image.read' }, path: '/admin/settings' })
    expect(result).toBe(true)
  })

  it('redirects unauthenticated admin routes to admin login', async () => {
    const guard = buildGuard({
      ensureUser: vi.fn().mockResolvedValue(false),
      ensureAdmin: vi.fn().mockResolvedValue({ authenticated: false, authorized: false })
    })

    const result = await guard({ meta: { auth: 'admin', adminPermission: 'users.read' }, path: '/admin/users' })
    expect(result).toEqual({ path: '/admin/login' })
  })

  it('sends authenticated admin users without permission to forbidden page', async () => {
    const ensureAdmin = vi.fn().mockResolvedValue({ authenticated: true, authorized: false })
    const guard = buildGuard({
      ensureUser: vi.fn().mockResolvedValue(false),
      ensureAdmin
    })

    const result = await guard({ meta: { auth: 'admin', adminPermission: 'users.read' }, path: '/admin/users' })
    expect(ensureAdmin).toHaveBeenCalledWith('users.read')
    expect(result).toEqual({ path: '/admin/forbidden' })
  })
})
