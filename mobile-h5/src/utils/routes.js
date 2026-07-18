import { onShareAppMessage, onShareTimeline } from '@dcloudio/uni-app'

import { api } from '../api/client.js'

export const routes = {
  home: '/pages/home/index',
  auth: '/pages/auth/index',
  workspace: '/pages/workspace/image-to-image/index',
  imageToImage: '/pages/workspace/image-to-image/index',
  pricing: '/pages/pricing/index',
  account: '/pages/account/index',
  accountTransactions: '/pages/account/transactions/index',
  support: '/pages/support/index',
  works: '/pages/works/index',
  workShare: '/pages/works/share/index',
  coupleAlbumCreate: '/pages/couple-album/create/index',
  coupleAlbumDetail: '/pages/couple-album/detail/index',
  coupleAlbumShare: '/pages/couple-album/share/index'
}

const registeredRoutes = [
  routes.home,
  routes.auth,
  routes.workspace,
  routes.imageToImage,
  routes.works,
  routes.workShare,
  routes.pricing,
  routes.support,
  routes.account,
  routes.accountTransactions,
  routes.coupleAlbumCreate,
  routes.coupleAlbumDetail,
  routes.coupleAlbumShare
]

const mainStackRoutes = [
  routes.home,
  routes.workspace,
  routes.imageToImage,
  routes.works,
  routes.pricing,
  routes.support,
  routes.account
]

const staticAssetBaseURL = `${import.meta.env.VITE_STATIC_ASSET_BASE_URL || ''}`.replace(/\/+$/, '')

function normalizeAssetPath(path) {
  return `${path || ''}`
    .trim()
    .replace(/^\/+/, '')
    .replace(/^static\/+/i, '')
}

function staticAsset(path) {
  const normalizedPath = normalizeAssetPath(path)
  if (!normalizedPath) return staticAssetBaseURL
  if (staticAssetBaseURL) return `${staticAssetBaseURL}/${normalizedPath}`
  return `/${['static', normalizedPath].join('/')}`
}

const defaultMiniProgramShareConfig = {
  title: '白霖共享AI图片生成',
  path: '/pages/home/index',
  imageUrl: staticAsset('home-replica/mountain-hero.png')
}

function pathQuery(path) {
  const queryStart = `${path || ''}`.indexOf('?')
  if (queryStart < 0) return ''
  return `${path}`.slice(queryStart + 1)
}

function resolveShareConfig(config, channel, event) {
  const resolved = typeof config === 'function' ? config({ channel, event }) : config
  return {
    ...defaultMiniProgramShareConfig,
    ...(resolved || {})
  }
}

function appMessagePayload(config, event) {
  const share = resolveShareConfig(config, 'appMessage', event)
  return {
    title: share.title || defaultMiniProgramShareConfig.title,
    path: share.path || defaultMiniProgramShareConfig.path,
    imageUrl: share.imageUrl || defaultMiniProgramShareConfig.imageUrl
  }
}

function timelinePayload(config) {
  const share = resolveShareConfig(config, 'timeline')
  return {
    title: share.title || defaultMiniProgramShareConfig.title,
    query: share.query || pathQuery(share.path),
    imageUrl: share.imageUrl || defaultMiniProgramShareConfig.imageUrl
  }
}

export function enableMiniProgramShare(config = {}) {
  onShareAppMessage((event) => appMessagePayload(config, event))
  onShareTimeline(() => timelinePayload(config))

  // #ifdef MP-WEIXIN
  uni.showShareMenu({
    menus: ['shareAppMessage', 'shareTimeline']
  })
  // #endif
}

function currentFullPath() {
  const pages = getCurrentPages()
  const current = pages[pages.length - 1]
  if (!current?.route) return routes.account
  const fullPath = current.$page?.fullPath || current.$page?.path
  if (fullPath) return fullPath.startsWith('/') ? fullPath : `/${fullPath}`
  const query = buildRouteQuery(current.options || {})
  const suffix = query ? `?${query}` : ''
  return `/${current.route}${suffix}`
}

function buildRouteQuery(params = {}) {
  return Object.entries(params)
    .filter(([, value]) => value !== undefined && value !== null && value !== '')
    .map(([key, value]) => `${encodeURIComponent(key)}=${encodeURIComponent(`${value}`)}`)
    .join('&')
}

function routeURL(path, params = {}) {
  const query = buildRouteQuery(params)
  return `${path}${query ? `?${query}` : ''}`
}

export function redirectToAuth(params = {}) {
  const url = routeURL(routes.auth, {
    mode: params.mode || 'login',
    redirect: params.redirect || currentFullPath()
  })
  setTimeout(() => {
    const pages = getCurrentPages()
    const current = pages[pages.length - 1]?.route
    if (`/${current}` === routes.auth) return
    uni.redirectTo({
      url,
      fail() {
        uni.reLaunch({ url })
      }
    })
  }, 0)
}

export async function requireAuth(params = {}) {
  try {
    return await api.getMe()
  } catch (error) {
    if (error?.status === 401) {
      redirectToAuth(params)
      return null
    }
    throw error
  }
}

export function navigateTo(path, params = {}) {
  if (!registeredRoutes.includes(path)) {
    uni.showToast({
      title: '模块建设中',
      icon: 'none'
    })
    return
  }

  const pages = getCurrentPages()
  const current = pages[pages.length - 1]?.route
  const url = routeURL(path, params)
  const hasQuery = Boolean(buildRouteQuery(params))
  if (`/${current}` === path && !hasQuery) return

  if (mainStackRoutes.includes(path) && !hasQuery) {
    uni.redirectTo({
      url,
      fail() {
        uni.reLaunch({ url })
      }
    })
    return
  }

  uni.navigateTo({ url })
}
