import { describe, expect, it } from 'vitest'

import router from '../router.js'

describe('router', () => {
  it('redirects the root path to the public workspace', () => {
    const route = router.getRoutes().find((item) => item.path === '/')

    expect(route).toBeTruthy()
    expect(route.redirect).toBe('/workspace')
  })

  it('keeps the workspace landing page public while protecting production workspace routes', () => {
    expect(router.resolve('/workspace').meta.auth).toBeUndefined()
    expect(router.resolve('/workspace/video').meta.auth).toBe('user')
    expect(router.resolve('/workspace/video').meta.workspaceChrome).toBeUndefined()
    expect(router.resolve('/workspace/old-photo-restoration').meta.auth).toBe('user')
    expect(router.resolve('/workspace/moments-marketing').meta.auth).toBe('user')
    expect(router.resolve('/workspace/article-images').meta.auth).toBe('user')
    expect(router.resolve('/workspace/virtual-try-on').meta.auth).toBe('user')
    expect(router.resolve('/workspace/couple-album').meta.auth).toBe('user')
    expect(router.resolve('/workspace/couple-album/8').meta.auth).toBe('user')
  })

  it('protects every admin page route except login with the admin guard', () => {
    const adminPaths = [
      '/admin',
      '/admin/settings',
      '/admin/settings/models/3',
      '/admin/prompt-templates',
      '/admin/inspiration-recommendations',
      '/admin/video-style-presets',
      '/admin/couple-album-options',
      '/admin/system-settings',
      '/admin/system-logs',
      '/admin/system-resources',
      '/admin/announcements',
      '/admin/invites',
      '/admin/generations',
      '/admin/users',
      '/admin/packages',
      '/admin/customer-service',
      '/admin/video-generations',
      '/admin/content-reviews',
      '/admin/content-reports',
      '/admin/algorithm-compliance',
      '/admin/incidents',
      '/admin/finance-orders',
      '/admin/permissions',
      '/admin/forbidden'
    ]

    adminPaths.forEach((path) => {
      expect(router.resolve(path).meta.auth).toBe('admin')
    })

    expect(router.resolve('/admin/login').meta.auth).toBeUndefined()
    expect(router.getRoutes().some((route) => route.path === '/admin/purchase-intents')).toBe(false)
  })

  it('registers the system settings admin route with the correct permission', () => {
    const route = router.getRoutes().find((item) => item.path === '/admin/system-settings')

    expect(route).toBeTruthy()
    expect(route.meta.adminPermission).toBe('system_settings.read')
  })

  it('registers the video generation records admin route with generation permissions', () => {
    const route = router.getRoutes().find((item) => item.path === '/admin/video-generations')

    expect(route).toBeTruthy()
    expect(route.meta.adminPermission).toBe('generations.read')
  })

  it('registers the system logs admin route with the correct permission', () => {
    const route = router.getRoutes().find((item) => item.path === '/admin/system-logs')

    expect(route).toBeTruthy()
    expect(route.meta.adminPermission).toBe('system_logs.read')
  })

  it('registers the system resources admin route with the correct permission', () => {
    const route = router.getRoutes().find((item) => item.path === '/admin/system-resources')

    expect(route).toBeTruthy()
    expect(route.meta.adminPermission).toBe('system_resources.read')
  })

  it('registers the admin model detail route with the settings permission', () => {
    const route = router.getRoutes().find((item) => item.path === '/admin/settings/models/:id')

    expect(route).toBeTruthy()
    expect(route.meta.adminPermission).toBe('settings.image.read')
  })

  it('registers the prompt template admin route with the prompt template permission', () => {
    const route = router.getRoutes().find((item) => item.path === '/admin/prompt-templates')

    expect(route).toBeTruthy()
    expect(route.meta.adminPermission).toBe('prompt_templates.read')
  })

  it('registers the inspiration recommendation admin route with the recommendation permission', () => {
    const route = router.getRoutes().find((item) => item.path === '/admin/inspiration-recommendations')

    expect(route).toBeTruthy()
    expect(route.meta.adminPermission).toBe('inspiration_recommendations.read')
  })

  it('registers the video style preset admin route with the video style permission', () => {
    const route = router.getRoutes().find((item) => item.path === '/admin/video-style-presets')

    expect(route).toBeTruthy()
    expect(route.meta.adminPermission).toBe('video_style_presets.read')
  })

  it('registers the couple album options admin route with the album config permission', () => {
    const route = router.getRoutes().find((item) => item.path === '/admin/couple-album-options')

    expect(route).toBeTruthy()
    expect(route.meta.adminPermission).toBe('couple_album_options.read')
  })

  it('registers the announcements admin route with the announcements permission', () => {
    const route = router.getRoutes().find((item) => item.path === '/admin/announcements')

    expect(route).toBeTruthy()
    expect(route.meta.adminPermission).toBe('announcements.read')
  })

  it('registers contact and customer service admin routes', () => {
    expect(router.resolve('/contact').matched.length).toBeGreaterThan(0)

    const route = router.getRoutes().find((item) => item.path === '/admin/customer-service')

    expect(route).toBeTruthy()
    expect(route.meta.adminPermission).toBe('customer_service.read')
  })

  it('registers user-facing compliance and report routes', () => {
    const publicPaths = ['/terms', '/privacy', '/algorithm-disclosure', '/content-report']

    publicPaths.forEach((path) => {
      const resolved = router.resolve(path)
      expect(resolved.matched.length).toBeGreaterThan(0)
      expect(resolved.meta.auth).toBeUndefined()
    })
  })

  it('registers algorithm compliance admin routes with permissions', () => {
    const routes = {
      '/admin/content-reviews': 'content_reviews.read',
      '/admin/content-reports': 'content_reports.read',
      '/admin/algorithm-compliance': 'algorithm_compliance.read',
      '/admin/incidents': 'algorithm_incidents.read'
    }

    Object.entries(routes).forEach(([path, permission]) => {
      const route = router.getRoutes().find((item) => item.path === path)
      expect(route).toBeTruthy()
      expect(route.meta.adminPermission).toBe(permission)
    })
  })

  it('registers public works sharing and authenticated assets routes', () => {
    expect(router.resolve('/works/share?ids=1,2').matched.length).toBeGreaterThan(0)
    expect(router.resolve('/works/share?ids=1,2').meta.auth).toBeUndefined()
    expect(router.resolve('/assets').matched.length).toBeGreaterThan(0)
    expect(router.resolve('/assets').meta.auth).toBe('user')
  })

  it('registers the old photo restoration workspace route', () => {
    const resolved = router.resolve('/workspace/old-photo-restoration')

    expect(resolved.matched.length).toBeGreaterThan(0)
    expect(resolved.meta.auth).toBe('user')
  })

  it('registers the moments marketing workspace route', () => {
    const resolved = router.resolve('/workspace/moments-marketing')

    expect(resolved.matched.length).toBeGreaterThan(0)
    expect(resolved.meta.auth).toBe('user')
  })

  it('registers the article images workspace route', () => {
    const resolved = router.resolve('/workspace/article-images')

    expect(resolved.matched.length).toBeGreaterThan(0)
    expect(resolved.meta.auth).toBe('user')
  })

  it('registers the virtual try-on workspace route', () => {
    const resolved = router.resolve('/workspace/virtual-try-on')

    expect(resolved.matched.length).toBeGreaterThan(0)
    expect(resolved.meta.auth).toBe('user')
  })

  it('registers the AI commerce workspace route with user auth', () => {
    const resolved = router.resolve('/workspace/ai-commerce')
    expect(resolved.matched.length).toBeGreaterThan(0)
    expect(resolved.meta.auth).toBe('user')
  })

  it('registers the childhood dream album workspace route with user auth', () => {
    const resolved = router.resolve('/workspace/childhood-dream-album')

    expect(resolved.matched.length).toBeGreaterThan(0)
    expect(resolved.meta.auth).toBe('user')
  })

  it('redirects the legacy image-to-image workspace route to the unified workspace', () => {
    const route = router.getRoutes().find((item) => item.path === '/workspace/image-to-image')

    expect(route).toBeTruthy()
    expect(route.redirect).toBe('/workspace')
  })

  it('scrolls to hash anchors when present and top otherwise', () => {
    expect(router.options.scrollBehavior({ hash: '#recharge-guide' }, {}, null)).toEqual({
      el: '#recharge-guide',
      behavior: 'smooth'
    })
    expect(router.options.scrollBehavior({ hash: '' }, {}, null)).toEqual({ top: 0 })
  })
})
