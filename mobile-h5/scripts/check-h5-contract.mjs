import { existsSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import assert from 'node:assert/strict'

const root = process.cwd()
const read = (path) => readFileSync(resolve(root, path), 'utf8')
const readBuffer = (path) => readFileSync(resolve(root, path))

function cssRule(source, selector) {
  const start = source.indexOf(`${selector} {`)
  assert(start >= 0, `missing CSS rule: ${selector}`)
  const open = source.indexOf('{', start)
  const close = source.indexOf('\n}', open)
  assert(close > open, `invalid CSS rule: ${selector}`)
  return source.slice(open + 1, close)
}

function assertPNGDimensions(path, expectedWidth, expectedHeight) {
  const buffer = readBuffer(path)
  const signature = buffer.subarray(0, 8).toString('hex')
  assert.equal(signature, '89504e470d0a1a0a', `${path} must be a PNG image`)
  assert.equal(buffer.readUInt32BE(16), expectedWidth, `${path} must be ${expectedWidth}px wide`)
  assert.equal(buffer.readUInt32BE(20), expectedHeight, `${path} must be ${expectedHeight}px tall`)
}

const pages = JSON.parse(read('src/pages.json'))
const pagePaths = pages.pages.map((page) => page.path)

assert(pagePaths.includes('pages/workspace/image-to-image/index'), 'workspace page must stay registered')
assert(pagePaths.includes('pages/works/index'), 'works history page must be registered')
assert(pagePaths.includes('pages/pricing/index'), 'pricing package page must be registered')
assert(pagePaths.includes('pages/support/index'), 'support help page must be registered')
assert(pagePaths.includes('pages/account/index'), 'account profile page must be registered')
assert(pagePaths.includes('pages/account/transactions/index'), 'account credit transactions page must be registered')
assert(pagePaths.includes('pages/auth/index'), 'single mobile auth page must be registered')
assert(pagePaths.includes('pages/couple-album/create/index'), 'couple album create page must be registered')
assert(pagePaths.includes('pages/couple-album/detail/index'), 'couple album detail page must be registered')
assert(pagePaths.includes('pages/couple-album/share/index'), 'couple album share page must be registered')
assert(pagePaths.includes('pages/works/share/index'), 'public works share page must be registered')

const routes = read('src/utils/routes.js')
assert(routes.includes("routes.works"), 'works route must be reachable through route helper')
assert(routes.includes("routes.imageToImage"), 'workspace route must be reachable through route helper')
assert(routes.includes("routes.pricing"), 'pricing route must be reachable through route helper')
assert(routes.includes('routes.pricing'), 'pricing route must be included in registered route navigation')
assert(routes.includes("routes.support"), 'support route must be reachable through route helper')
assert(routes.includes('routes.support'), 'support route must be included in registered route navigation')
assert(routes.includes("routes.account"), 'account route must be reachable through route helper')
assert(routes.includes('routes.account'), 'account route must be included in registered route navigation')
assert(routes.includes("accountTransactions: '/pages/account/transactions/index'"), 'account transactions route must be reachable through route helper')
assert(routes.includes('routes.accountTransactions'), 'account transactions route must be included in registered route navigation')
assert(routes.includes("auth: '/pages/auth/index'"), 'auth route must be reachable through route helper')
assert(routes.includes("coupleAlbumCreate: '/pages/couple-album/create/index'"), 'couple album create route must be reachable through route helper')
assert(routes.includes("coupleAlbumDetail: '/pages/couple-album/detail/index'"), 'couple album detail route must be reachable through route helper')
assert(routes.includes("coupleAlbumShare: '/pages/couple-album/share/index'"), 'couple album share route must be reachable through route helper')
assert(routes.includes("workShare: '/pages/works/share/index'"), 'public works share route must be reachable through route helper')
assert(routes.includes('routes.coupleAlbumCreate'), 'couple album create route must be included in registered route navigation')
assert(routes.includes('routes.coupleAlbumDetail'), 'couple album detail route must be included in registered route navigation')
assert(routes.includes('routes.coupleAlbumShare'), 'couple album share route must be included in registered route navigation')
assert(routes.includes('routes.workShare'), 'public works share route must be included in registered route navigation')
assert(routes.includes('redirectToAuth'), 'protected pages must be able to redirect unauthenticated users')
assert(routes.includes('requireAuth'), 'protected pages must share auth guard helper')
assert(routes.includes('setTimeout'), 'auth redirects must be deferred until mini-program launch routing settles')
assert(routes.includes('uni.redirectTo'), 'auth redirects must replace protected pages instead of pushing onto the launch stack')
assert(routes.includes('uni.reLaunch'), 'auth redirects must fall back to reLaunch if redirectTo cannot run')
assert(routes.includes('mainStackRoutes'), 'route helper must classify main shell pages')
assert(routes.includes('uni.redirectTo({'), 'main shell navigation must replace pages instead of pushing onto the webview stack')
assert(routes.includes('uni.navigateTo({ url })'), 'route helper must still support parameterized detail/reuse navigation')

const appTabbarPath = 'src/components/AppTabbar.vue'
assert(existsSync(resolve(root, appTabbarPath)), 'shared AppTabbar component file must exist')
const appTabbar = read(appTabbarPath)
assert(appTabbar.includes('defineProps'), 'AppTabbar must expose props for active tab state')
assert(appTabbar.includes('activeKey'), 'AppTabbar must accept an activeKey prop')
assert(appTabbar.includes('extraSpace'), 'AppTabbar must accept extraSpace for fixed action clearance')
assert(appTabbar.includes("import { navigateTo, routes }"), 'AppTabbar must use the shared route helper')
assert(appTabbar.includes('function staticIcon'), 'AppTabbar must resolve icons through a staticIcon helper')
for (const marker of [
  "navigateTo(routes.imageToImage)",
  "navigateTo(routes.works)",
  "navigateTo(routes.pricing)",
  "navigateTo(routes.account)"
]) {
  assert(appTabbar.includes(marker), `AppTabbar must navigate through shared route helper: ${marker}`)
}
for (const label of ['工作台', '作品库', '套餐', '我的']) {
  assert(appTabbar.includes(label), `AppTabbar must render ${label}`)
}
for (const icon of ["staticIcon('home')", "staticIcon('workspace')", "staticIcon('pricing')", "staticIcon('account')"]) {
  assert(appTabbar.includes(icon), `AppTabbar must resolve icon through staticIcon: ${icon}`)
}
const appTabbarStyle = appTabbar.match(/<style[\s\S]*<\/style>/)?.[0] || ''
for (const style of [
  'position: fixed;',
  'left: 0;',
  'right: 0;',
  'bottom: 0;',
  'env(safe-area-inset-bottom)',
  'grid-template-columns: repeat(4, minmax(0, 1fr));',
  'min-height: 88rpx;',
  '.app-tabbar__item.active'
]) {
  assert(appTabbarStyle.includes(style), `AppTabbar style must include ${style}`)
}

const appTabbarPageContracts = [
  ['src/pages/workspace/image-to-image/index.vue', 'workspace', true],
  ['src/pages/couple-album/create/index.vue', 'workspace', true],
  ['src/pages/works/index.vue', 'works', false],
  ['src/pages/pricing/index.vue', 'pricing', false],
  ['src/pages/account/index.vue', 'account', false],
  ['src/pages/support/index.vue', '', false]
]
for (const [pagePath, activeKey, needsExtraSpace] of appTabbarPageContracts) {
  const page = read(pagePath)
  assert(page.includes(`active-key="${activeKey}"`), `${pagePath} must pass active-key="${activeKey}" to AppTabbar`)
  assert(page.includes('<AppTabbar'), `${pagePath} must render shared AppTabbar`)
  assert(!page.includes('const navItems = ['), `${pagePath} must not define private bottom nav items`)
  assert(!page.includes('function goNav(item)'), `${pagePath} must not define private bottom nav navigation`)
  assert(!page.includes('class="tabbar"'), `${pagePath} must not render a private tabbar`)
  assert(!page.includes('.tabbar'), `${pagePath} must not keep private tabbar styles`)
  assert(!page.includes('class="bottom-space"'), `${pagePath} must use AppTabbar spacer instead of a page bottom-space node`)
  assert(!page.includes('.bottom-space'), `${pagePath} must use AppTabbar spacer instead of page bottom-space styles`)
  if (needsExtraSpace) {
    assert(page.includes('extra-space='), `${pagePath} must reserve AppTabbar extra space for the fixed generate button`)
  }
}

const announcementPopupPath = 'src/components/AnnouncementPopup.vue'
assert(existsSync(resolve(root, announcementPopupPath)), 'shared AnnouncementPopup component file must exist')
const announcementPopup = read(announcementPopupPath)
assert(announcementPopup.includes('api.listPopupAnnouncements'), 'AnnouncementPopup must load popup announcements from the backend')
assert(announcementPopup.includes('api.dismissAnnouncement'), 'AnnouncementPopup must dismiss announcements through the backend')
assert(announcementPopup.includes("default: 'mp-weixin'"), 'AnnouncementPopup must default to the mp-weixin client')
assert(announcementPopup.includes('announcement-popup-next'), 'AnnouncementPopup must allow users to move to the next unread announcement')
assert(announcementPopup.includes('announcement-popup-close'), 'AnnouncementPopup must expose a close button')

for (const pagePath of [
  'src/pages/home/index.vue',
  'src/pages/workspace/image-to-image/index.vue',
  'src/pages/works/index.vue',
  'src/pages/pricing/index.vue',
  'src/pages/account/index.vue'
]) {
  const page = read(pagePath)
  assert(page.includes("import AnnouncementPopup from"), `${pagePath} must import the shared announcement popup`)
  assert(page.includes('<AnnouncementPopup'), `${pagePath} must mount the shared announcement popup`)
}

const pendingTasks = read('src/utils/generation-tasks.js')
assert(pendingTasks.includes('pendingGenerationStorageKey'), 'pending generation tasks must persist across page switches')
assert(pendingTasks.includes('addPendingGenerations'), 'workspace must be able to save pending generation tasks')
assert(pendingTasks.includes('loadPendingGenerations'), 'works page must be able to load pending generation tasks')
assert(pendingTasks.includes('removePendingGenerations'), 'works page must clear completed pending generation tasks')

const main = read('src/main.js')
assert(main.includes('IMAGE_AGENT_MP_BUILD'), 'mp-weixin app startup must log a fixed build marker')
assert(main.includes('no-urlsearchparams-v2'), 'mp-weixin build marker must identify the URLSearchParams-free build')

const homePage = read('src/pages/home/index.vue')
assert(homePage.includes('白霖共享'), 'home page must render the 白霖共享 brand')
assert(homePage.includes('创作者 AI 图片平台'), 'home page must render the creator AI image platform subtitle')
assert(homePage.includes('CREATOR PORTAL'), 'home page must render the creator portal eyebrow')
assert(homePage.includes('一站式 AI 生图工作台'), 'home page must render the replica hero title')
assert(homePage.includes('提示词、模板、生成、入库、复用，一站完成'), 'home page must render the replica hero summary')
assert(homePage.includes('进入工作台') && homePage.includes('openWorkspace'), 'home page must keep the image generation workspace CTA')
assert(homePage.includes('持续生成工作台'), 'home page must render the first feature column')
assert(homePage.includes('智能入库管理'), 'home page must render the second feature column')
assert(homePage.includes('素材复用交付'), 'home page must render the third feature column')
assert(homePage.includes('520 情侣相册'), 'home page must render the 520 couple album campaign entry')
assert(homePage.includes('粉色相册书') || homePage.includes('albumBook'), 'home page must use the generated pink couple album book asset')
assert(homePage.includes('创建相册') && homePage.includes('openCoupleAlbum'), 'home page must expose the couple album CTA')
assert(homePage.includes('routes.coupleAlbumCreate'), 'home couple album CTA must navigate to the create page')
assert(homePage.includes('@click="openSupport"'), 'home notification action must jump to customer support')
assert(homePage.includes('@click="openAccount"'), 'home avatar action must jump to account')
assert(!homePage.includes('wechat-capsule'), 'home page must not draw a fake mini-program capsule')
assert(!homePage.includes('mini-program-capsule'), 'home page must not draw a fake mini-program capsule')
assert(homePage.includes('class="hero-visual"'), 'home page must render the right-side stacked hero visual')
assert(homePage.includes('class="feature-grid"'), 'home page must render the three-column feature grid')
assert(homePage.includes('class="campaign-grid"'), 'home page must render the two-column 520 and product cards')
assert(homePage.includes('商品主图') && homePage.includes('生成同款'), 'home product card must route users to the workspace')
const campaignLinkRuleStart = homePage.lastIndexOf('.campaign-link {')
assert(campaignLinkRuleStart >= 0, 'home page must define a dedicated campaign CTA pill rule')
const campaignLinkRuleOpen = homePage.indexOf('{', campaignLinkRuleStart)
const campaignLinkRuleClose = homePage.indexOf('\n}', campaignLinkRuleOpen)
const campaignLinkStyle = homePage.slice(campaignLinkRuleOpen + 1, campaignLinkRuleClose)
assert(campaignLinkStyle.includes('display: flex;'), 'home campaign CTA pills must use flex layout for centered text')
assert(campaignLinkStyle.includes('align-items: center;'), 'home campaign CTA pills must vertically center text')
assert(campaignLinkStyle.includes('justify-content: center;'), 'home campaign CTA pills must horizontally center text')
assert(homePage.includes('class="workflow-card"'), 'home page must render the generation, storage, reuse explainer card')
assert(homePage.includes('生成入库复用'), 'home workflow card must render the reuse explainer title')
assert(homePage.includes('homeReplicaAssets'), 'home page must reference the generated home replica assets')
for (const assetName of [
  'home-replica/mountain-hero.png',
  'home-replica/portrait-card.png',
  'home-replica/product-bottle-small.png',
  'home-replica/city-stage.png',
  'home-replica/couple-album-book.png',
  'home-replica/product-bottle-large.png'
]) {
  assert(homePage.includes(`staticAsset('${assetName}')`), `home page must reference generated asset ${assetName}`)
  assert(existsSync(resolve(root, `src/static/${assetName}`)), `missing generated home asset ${assetName}`)
}
assert(homePage.includes('class="home-local-tabbar"'), 'home page must render the homepage-only three-item tabbar')
assert(homePage.includes('首页') && homePage.includes('工作台') && homePage.includes('我的'), 'home local tabbar must render 首页 工作台 我的')
assert(homePage.includes('@click="openHome"'), 'home local tabbar active home item must keep a stable handler')
assert(homePage.includes('@click="openWorkspace"'), 'home local tabbar workspace item must navigate to workspace')
assert(homePage.includes('@click="openAccount"'), 'home local tabbar account item must navigate to account')
assert(!homePage.includes('import AppTabbar'), 'home page must not use the shared AppTabbar for the replica bottom nav')
assert(!homePage.includes('<AppTabbar'), 'home page must not render the shared AppTabbar for the replica bottom nav')
assert(!homePage.includes('class="modules-section"'), 'home page must remove the old modules section')
assert(!homePage.includes('class="value-panel"'), 'home page must remove the old value panel')
assert(!homePage.includes('class="home-footer"'), 'home page must remove the old footer')
assert(!homePage.includes('观看平台演示'), 'home page must remove the old demo button')

const apiClient = read('src/api/client.js')
assert(apiClient.includes('export function uploadReferenceAsset(input)'), 'mobile upload API must accept the selected file input')
assert(apiClient.includes('/api/reference-assets/upload-policy'), 'mobile upload API must request an OSS upload policy before sending file bytes')
assert(apiClient.includes('/api/reference-assets/complete-upload'), 'mobile upload API must complete the backend reference asset after OSS upload')
assert(apiClient.includes('appendOSSFormFields'), 'mobile upload API must build OSS form fields for H5 FormData uploads')
assert(apiClient.includes('uploadReferenceAssetWithFetch'), 'mobile H5 upload API must use fetch and FormData for OSS direct uploads')
assert(apiClient.includes('url: policy.upload_url'), 'mobile upload API must send mini-program uploads directly to the OSS policy URL')
assert(apiClient.includes('const uploadOptions = {'), 'mobile upload API must build runtime-specific upload options')
assert(apiClient.includes("reject(new ApiError('upload_file_missing'"), 'mobile upload API must reject missing upload files before calling uni.uploadFile')
assert(apiClient.includes('// #ifdef H5') && apiClient.includes('uploadReferenceAssetWithFetch(policy, upload.file)'), 'mobile upload API must keep H5 FormData uploads behind H5-only compilation')
assert(apiClient.includes("if (typeof payload === 'object') return payload ?? {}"), 'mobile upload API must preserve object JSON responses')
assert(apiClient.includes('function buildQuery'), 'mobile API must centralize query string building')
assert(!apiClient.includes('new URLSearchParams'), 'mobile API query building must not use browser-only URLSearchParams')
assert(apiClient.includes('function apiURL'), 'mobile API must normalize request URLs per runtime')
assert(apiClient.includes('VITE_API_BASE_URL'), 'mp-weixin API requests must support an absolute backend base URL')
assert(apiClient.includes('url: apiURL(path)'), 'uni.request must use normalized API URLs')
assert(!apiClient.includes("url: apiURL('/api/reference-assets')"), 'uni.uploadFile must not send reference images through the backend multipart endpoint')
assert(apiClient.includes('requestTimeoutMS'), 'mobile API requests must define an explicit timeout')
assert(apiClient.includes('timeout: requestTimeoutMS'), 'uni.request must pass the explicit timeout to mini-program requests')
assert(apiClient.includes('timeout: uploadTimeoutMS'), 'uni.uploadFile must pass the explicit timeout to uploads')
assert(apiClient.includes('formatNetworkError'), 'network errors must be normalized into readable diagnostics')
assert(apiClient.includes('https://example.com'), 'mp-weixin diagnostics must mention the production domain backend default')
assert(apiClient.includes('getWork(id)'), 'mobile API must expose work detail lookup')
assert(apiClient.includes('deleteWork(id)'), 'mobile API must expose work deletion')
assert(apiClient.includes('reuseWork(id)'), 'mobile API must expose work reuse')
assert(apiClient.includes('updateWork(id, input)'), 'mobile API must expose work updates')
assert(apiClient.includes("method: 'PATCH'"), 'mobile API must support PATCH work updates')
assert(apiClient.includes('listPromptTemplates(params = {})'), 'mobile API must expose prompt template listing')
assert(apiClient.includes("request(`/api/prompt-templates${buildQuery(params)}`)"), 'mobile API prompt template listing must call GET /api/prompt-templates')
assert(apiClient.includes('usePromptTemplate(id)'), 'mobile API must expose prompt template usage')
assert(apiClient.includes('/api/prompt-templates/${id}/use'), 'mobile API prompt template usage must call POST /api/prompt-templates/:id/use')
assert(apiClient.includes('createCoupleAlbum(input)'), 'mobile API must expose couple album creation')
assert(apiClient.includes('getCoupleAlbumOptions()'), 'mobile API must expose couple album option config lookup')
assert(apiClient.includes("request('/api/couple-album/options')"), 'mobile API couple album options must call GET /api/couple-album/options')
assert(apiClient.includes("request('/api/couple-albums'"), 'mobile API couple album creation must call POST /api/couple-albums')
assert(apiClient.includes('generateCoupleAlbum(id)'), 'mobile API must expose couple album generation')
assert(apiClient.includes('/api/couple-albums/${id}/generate'), 'mobile API couple album generation must call POST /api/couple-albums/:id/generate')
assert(apiClient.includes('getCoupleAlbum(id)'), 'mobile API must expose couple album detail')
assert(apiClient.includes('listCoupleAlbums(params = {})'), 'mobile API must expose couple album listing')
assert(apiClient.includes('retryCoupleAlbumPage(albumID, pageID)'), 'mobile API must expose couple album failed page retry')
assert(apiClient.includes('/api/couple-albums/${albumID}/pages/${pageID}/retry'), 'mobile API retry must call POST /api/couple-albums/:id/pages/:page_id/retry')
assert(apiClient.includes('shareCoupleAlbum(id)'), 'mobile API must expose couple album private sharing')
assert(apiClient.includes('/api/couple-albums/${id}/share'), 'mobile API sharing must call POST /api/couple-albums/:id/share')
assert(apiClient.includes('getPublicCoupleAlbum(token)'), 'mobile API must expose public couple album lookup')
assert(apiClient.includes('/api/public/couple-albums/${token}'), 'mobile API public lookup must call GET /api/public/couple-albums/:share_token')
assert(apiClient.includes('getPublicWorks(params = {})'), 'mobile API must expose public works lookup')
assert(apiClient.includes('request(`/api/public/works${buildQuery(params)}`)'), 'mobile API public works lookup must call GET /api/public/works')
assert(apiClient.includes('assetURL(path)'), 'mobile API must expose absolute asset URL helper for mp-weixin images')
assert(apiClient.includes('register(input)'), 'mobile API must expose user registration')
assert(apiClient.includes('login(input)'), 'mobile API must expose user login')
assert(apiClient.includes('logout()'), 'mobile API must expose logout')
assert(apiClient.includes('pingPresence()'), 'mobile API must expose account presence heartbeat')
assert(apiClient.includes("request('/api/account/presence')"), 'mobile API presence heartbeat must call GET /api/account/presence')
assert(apiClient.includes('getCredits()'), 'mobile API must expose account credits')
assert(apiClient.includes('getCreditTransactions(params = {})'), 'mobile API must expose account credit transactions with query params')
assert(apiClient.includes("request(`/api/account/credit-transactions${buildQuery(params)}`)"), 'mobile API credit transactions must pass query params')
assert(apiClient.includes('bindAccountPhone(input)'), 'mobile API must expose account phone binding')
assert(apiClient.includes("request('/api/account/phone'"), 'mobile API phone binding must call POST /api/account/phone')
assert(apiClient.includes('bindWechatPhone(input)'), 'mobile API must expose current-account WeChat phone binding')
assert(apiClient.includes("request('/api/account/wechat-phone'"), 'mobile API WeChat phone binding must call POST /api/account/wechat-phone')
assert(apiClient.includes('updateProfile(input)'), 'mobile API must expose account profile updates')
assert(apiClient.includes('updateAccountEmail(input)'), 'mobile API must expose account email updates')
assert(apiClient.includes('updateAccountPreferences(input)'), 'mobile API must expose notification preference updates')
assert(apiClient.includes('changePassword(input)'), 'mobile API must expose login password changes')
assert(apiClient.includes('setPaymentPassword(input)'), 'mobile API must expose payment password setup')
assert(apiClient.includes('clearPaymentPassword(input)'), 'mobile API must expose payment password clearing')
assert(apiClient.includes('getAccountSessions()'), 'mobile API must expose device session listing')
assert(apiClient.includes('deleteAccountSession(id)'), 'mobile API must expose single device logout')
assert(apiClient.includes('deleteOtherAccountSessions()'), 'mobile API must expose other-device logout')
assert(apiClient.includes('getCustomerService()'), 'mobile API must expose customer service config')
assert(apiClient.includes('listPopupAnnouncements(client ='), 'mobile API must expose popup announcement listing')
assert(apiClient.includes('/api/announcements/popup'), 'mobile API must call GET /api/announcements/popup')
assert(apiClient.includes('dismissAnnouncement(id, client ='), 'mobile API must expose announcement dismissal')
assert(apiClient.includes('/api/announcements/${id}/dismiss'), 'mobile API must call POST /api/announcements/:id/dismiss')
assert(apiClient.includes('getPackages()'), 'mobile API must expose package listing')
assert(apiClient.includes("request('/api/packages')"), 'mobile API package listing must call GET /api/packages')
assert(apiClient.includes('estimateImageGeneration(input)'), 'mobile API must expose image generation credit estimation')
assert(apiClient.includes("request('/api/images/generations/estimate'"), 'mobile API image estimation must call POST /api/images/generations/estimate')
assert(apiClient.includes('estimateCoupleAlbum(input)'), 'mobile API must expose couple album credit estimation')
assert(apiClient.includes("request('/api/couple-albums/estimate'"), 'mobile API couple album estimation must call POST /api/couple-albums/estimate')
assert(apiClient.includes('Object.assign(this, details)'), 'mobile API errors must preserve structured backend error fields')
assert(apiClient.includes('wechatLogin(input)'), 'mobile API must expose WeChat login')
assert(apiClient.includes("request('/api/auth/wechat-login'"), 'mobile API WeChat login must call POST /api/auth/wechat-login')
assert(apiClient.includes('wechatPhoneLogin(input)'), 'mobile API must expose WeChat phone login')
assert(apiClient.includes("request('/api/auth/wechat-phone-login'"), 'mobile API WeChat phone login must call POST /api/auth/wechat-phone-login')
assert(apiClient.includes('wechatBind(input)'), 'mobile API must expose WeChat openid binding')
assert(apiClient.includes("request('/api/auth/wechat-bind'"), 'mobile API WeChat binding must call POST /api/auth/wechat-bind')
assert(apiClient.includes('createWechatVirtualPayOrder(input)'), 'mobile API must expose WeChat virtual payment order creation')
assert(apiClient.includes("request('/api/payments/wechat/virtual-orders'"), 'mobile API WeChat virtual payment order creation must call POST /api/payments/wechat/virtual-orders')
assert(apiClient.includes('confirmWechatVirtualPayOrder(orderNumber)'), 'mobile API must expose WeChat virtual payment confirmation')
assert(apiClient.includes('/api/payments/wechat/virtual-orders/${orderNumber}/confirm'), 'mobile API WeChat virtual payment confirmation must call backend confirm endpoint')
assert(!apiClient.includes('createPurchaseIntent(input)'), 'mobile API must not expose purchase intent creation for package checkout')
assert(!apiClient.includes("request('/api/purchase-intents'"), 'mobile API must not call POST /api/purchase-intents from package checkout')

const appSource = read('src/App.vue')
assert(appSource.includes("import { api, getStoredAuthToken } from './api/client.js'"), 'App must import presence API and auth token lookup')
assert(appSource.includes('const presenceHeartbeatIntervalMS = 60_000'), 'App presence heartbeat interval must be 60 seconds')
assert(appSource.includes('let presenceHeartbeatTimer'), 'App must keep a global presence heartbeat timer')
assert(appSource.includes('function pingPresence'), 'App must define a presence ping helper')
assert(appSource.includes('if (!getStoredAuthToken()) return'), 'App presence heartbeat must no-op when there is no user token')
assert(appSource.includes('api.pingPresence()'), 'App presence heartbeat must call api.pingPresence')
assert(appSource.includes('function startPresenceHeartbeat'), 'App must define heartbeat startup')
assert(appSource.includes('function stopPresenceHeartbeat'), 'App must define heartbeat stop')
assert(appSource.includes('setInterval'), 'App must poll presence on an interval')
assert(appSource.includes('onShow()'), 'App must ping presence immediately when shown')
assert(appSource.includes('onHide()'), 'App must stop the presence heartbeat when hidden')

const authPageSource = read('src/pages/auth/index.vue')
assert(authPageSource.includes('open-type="getPhoneNumber"'), 'auth page must request bound phone authorization')
assert(authPageSource.includes('@getphonenumber="submitWechatPhoneLogin"'), 'auth page must handle phone authorization')
assert(authPageSource.includes('api.wechatPhoneLogin'), 'auth page must call phone quick login API after authorization')
assert(authPageSource.includes('phone_code: phoneCode'), 'auth page must send the phone authorization code to the backend')
assert(authPageSource.includes('wechatPhoneLoginErrorMessage'), 'auth page must map phone backend failures to user-facing messages')
assert(authPageSource.includes('wechat_phone_code_invalid'), 'auth page must show an expired authorization message for invalid phone codes')
assert(authPageSource.includes('wechat_phone_capability_unavailable'), 'auth page must show an SMS fallback when phone capability is unavailable')
assert(authPageSource.includes('wechat_phone_token_failed'), 'auth page must show an SMS fallback when phone token exchange fails')
assert(authPageSource.includes('授权已失效，请重新授权手机号'), 'auth page must not describe backend invalid-code failures as missing authorization')
assert(authPageSource.includes('手机号快捷登录暂不可用，请改用短信验证码'), 'auth page must suggest SMS fallback for phone quick login failures')
assert(authPageSource.includes('手机号快捷登录'), 'auth page must render neutral phone quick login copy')
assert(authPageSource.includes('手机号验证中...'), 'auth page must render neutral phone quick loading copy')
assert(authPageSource.includes('phone-quick-auth-button'), 'auth page must use neutral phone quick auth button class')
assert(authPageSource.includes('phone-quick-auth-divider'), 'auth page must use neutral phone quick auth divider class')
assert(authPageSource.includes('bindPhone=1'), 'auth page must redirect legacy accounts without a phone to the binding prompt')
assert(!authPageSource.includes('submitWechatLogin'), 'auth page must not keep a separate ordinary WeChat login button')
assert(!authPageSource.includes('wechat-phone-auth-button'), 'auth page must not keep a second WeChat phone login button')

const workspace = read('src/pages/workspace/image-to-image/index.vue')
assert(workspace.includes('api.uploadReferenceAsset'), 'workspace must keep reference upload API for image-to-image mode')
assert(workspace.includes('api.createImageGeneration'), 'workspace must keep async generation API')
assert(workspace.includes('async function createSingleGeneration'), 'workspace must create one async generation task before polling')
assert(!workspace.includes('async function createGenerationBatch'), 'workspace must not keep the selected-count batch generation helper')
const workspaceSizePickerBlock = workspace.match(/<picker[\s\S]*?class="size-picker-trigger"[\s\S]*?<\/picker>/)?.[0] || ''
assert(workspaceSizePickerBlock.includes('mode="selector"'), 'workspace size header must use a native selector picker')
assert(workspaceSizePickerBlock.includes(':range="sizePresetLabels"'), 'workspace size picker must use sizePresetLabels')
assert(workspaceSizePickerBlock.includes(':value="selectedSizePresetIndex"'), 'workspace size picker must bind the selected preset index')
assert(workspaceSizePickerBlock.includes(':disabled="submitting"'), 'workspace size picker must be disabled while submitting')
assert(workspaceSizePickerBlock.includes('@change="selectSizePreset"'), 'workspace size picker must reuse selectSizePreset')
assert(workspaceSizePickerBlock.includes('class="size-head"'), 'workspace size picker trigger must wrap the size header row')
assert(workspaceSizePickerBlock.includes('class="size-picker-arrow"'), 'workspace size picker trigger must include the dropdown arrow')

const accountPageSource = read('src/pages/account/index.vue')
assert(accountPageSource.includes('onShow(() => {'), 'account page must refresh current user when shown')
assert(accountPageSource.includes('当前账号未绑定手机号'), 'account page must label the unbound phone state neutrally')
assert(accountPageSource.includes('绑定后可用手机号登录，也能通过手机号匹配账号。'), 'account page must explain phone login and account matching neutrally')
assert(accountPageSource.includes('@tap="openPhoneBinder"'), 'account phone bind strip must be tappable in mp-weixin')
assert(accountPageSource.includes('class="phone-bind-cta"'), 'account phone bind CTA must use the centered CTA style')
assert(accountPageSource.includes('showPhoneBindModal'), 'account phone bind CTA must open an in-page binding modal')
assert(accountPageSource.includes('class="phone-bind-modal"'), 'account page must render a phone binding modal')
assert(accountPageSource.includes('优先使用手机号快捷验证绑定当前账号，也可改用短信验证码。'), 'account phone binding modal must use neutral quick verification copy')
assert(accountPageSource.includes('class="phone-quick-bind-button"'), 'account phone binding modal must use neutral phone quick bind class')
assert(accountPageSource.includes('手机号快捷绑定'), 'account phone binding modal must render neutral phone quick bind copy')
assert(accountPageSource.includes('手机号验证中...'), 'account phone binding modal must render neutral phone quick loading copy')
assert(accountPageSource.includes('open-type="getPhoneNumber"'), 'account phone binding modal must request bound phone authorization')
assert(accountPageSource.includes('@getphonenumber="bindWechatPhone"'), 'account phone binding modal must handle phone authorization')
assert(accountPageSource.includes('api.bindWechatPhone'), 'account page must bind phone to the current account through the quick auth endpoint')
assert(accountPageSource.includes('api.bindWechatPhone({ code, phone_code: phoneCode })'), 'account phone binding modal must send login code and phone authorization code')
assert(accountPageSource.includes('bindPhoneError'), 'account phone binding modal must show inline binding errors')
assert(!accountPageSource.includes("if (code === 'wechat_phone_failed') return 'phone_auth_required'"), 'account page must not turn backend phone failures into local authorization-required errors')
assert(accountPageSource.includes('wechat_phone_code_invalid'), 'account page must keep invalid phone code as a backend exchange failure')
assert(accountPageSource.includes('wechat_phone_capability_unavailable'), 'account page must show an SMS fallback when phone capability is unavailable')
assert(accountPageSource.includes('wechat_phone_token_failed'), 'account page must show an SMS fallback when phone token exchange fails')
assert(accountPageSource.includes('授权已失效，请重新授权手机号'), 'account page must not describe backend invalid-code failures as missing authorization')
assert(accountPageSource.includes('手机号快捷绑定暂不可用，请改用短信验证码'), 'account page must suggest SMS fallback for phone quick bind failures')
assert(accountPageSource.includes('账号绑定冲突，请联系客服或换号重试'), 'account page must show a neutral conflict message')
assert(accountPageSource.includes('showSMSPhoneBinder'), 'account phone binding modal must keep an SMS fallback')
assert(accountPageSource.includes('api.bindAccountPhone'), 'account phone binding modal must keep SMS fallback binding')
assert(accountPageSource.includes('手机号已绑定'), 'account phone binding modal must show a success toast')
assert(!accountPageSource.includes('wechatPhoneBindText'), 'account phone binding modal must not render WeChat button text through a computed expression')
assert(!accountPageSource.includes('bindPhoneCodeText'), 'account phone binding modal must not render SMS code button text through a computed expression')
assert(!accountPageSource.includes('v-model="bindPhoneForm.phone"'), 'account phone binding modal must not bind SMS phone through nested reactive v-model')
assert(!accountPageSource.includes('v-model="bindPhoneForm.code"'), 'account phone binding modal must not bind SMS code through nested reactive v-model')
assert(!accountPageSource.includes("showSMSPhoneBinder ?"), 'account phone binding modal must not render SMS toggle text through an inline ternary')

for (const [sourceName, source] of [
  ['auth page', authPageSource],
  ['account page', accountPageSource]
]) {
  for (const forbidden of [
    '微信手机号一键注册/登录',
    '微信手机号一键绑定',
    '当前微信账号未绑定手机号',
    '请授权微信绑定手机号',
    '微信手机号服务暂不可用',
    '微信手机号登录成功',
    '微信手机号登录失败',
    '微信登录失败',
    '微信账号绑定冲突',
    '微</text>',
    'wechat-auth-button',
    'wechat-auth-divider',
    'wechat-auth-icon',
    'wechat-phone-bind-button'
  ]) {
    assert(!source.includes(forbidden), `${sourceName} must not expose old WeChat-branded phone auth copy/class: ${forbidden}`)
  }
}
assert(accountPageSource.includes(':value="bindPhoneInputPhone"'), 'account phone binding modal must bind SMS phone input through an explicit value')
assert(accountPageSource.includes('@input="updateBindPhoneInputPhone"'), 'account phone binding modal must update SMS phone through an explicit input handler')
assert(accountPageSource.includes(':value="bindPhoneInputCode"'), 'account phone binding modal must bind SMS code input through an explicit value')
assert(accountPageSource.includes('@input="updateBindPhoneInputCode"'), 'account phone binding modal must update SMS code through an explicit input handler')
assert(!workspace.includes('const generationCount'), 'workspace must not keep selected generation count state')
assert(!workspace.includes('function setCount'), 'workspace must not expose generation count mutation')
assert(!workspace.includes('function createGenerationBatchID'), 'workspace must not create a frontend batch id for one-click generation')
assert(!workspace.includes('function createVariationSeed'), 'workspace must not create random per-image seeds for batch generation')
assert(!workspace.includes('batch_id: batchID'), 'workspace must not send batch_id from the mobile workspace submit path')
assert(!workspace.includes('batch_index: index'), 'workspace must not send batch_index from the mobile workspace submit path')
assert(!workspace.includes('batch_total: total'), 'workspace must not send batch_total from the mobile workspace submit path')
assert(!workspace.includes('const createdTasks = []'), 'workspace must not collect multiple generated tasks from one click')
assert(!workspace.includes('createdTasks.push({'), 'workspace must not push multiple generated tasks from one click')
assert(!workspace.includes('seed: createVariationSeed'), 'workspace must not send random batch variation seeds')
assert(!workspace.includes('<text class="section-title">生成数量</text>'), 'workspace must not render the generation quantity title')
assert(!workspace.includes('v-for="item in [1, 2, 4]"'), 'workspace must not render 1/2/4 generation quantity options')
assert(!workspace.includes('@click="setCount(item)"'), 'workspace must not handle generation quantity clicks')
assert(workspace.includes("const variationMode = ref('balanced')"), 'workspace must default single-image creativity to balanced')
assert(workspace.includes('function variationPromptForIndex'), 'workspace must keep a creativity prompt helper for the single generated image')
assert(workspace.includes('variation_mode: variationMode.value'), 'workspace must send the selected creativity mode with the generated image')
assert(workspace.includes('variation_prompt: variationPromptForIndex(0, variationMode.value)'), 'workspace must send the first creativity prompt for the single generated image')
assert(workspace.includes('创意程度'), 'workspace must expose a creativity control near quality')
assert(workspace.includes('const estimatedGenerationCredits = computed'), 'workspace must calculate single-generation estimated credit cost')
assert(workspace.includes('const uploadedReferenceCount = computed'), 'workspace must count uploaded asset and reused work references together')
assert(workspace.includes('const uploadedReferenceCreditCost = computed'), 'workspace must calculate uploaded-reference credit cost for image-to-image mode')
assert(workspace.includes('const singleGenerationCreditCost = 1'), 'workspace must treat each submit as one generated image')
assert(workspace.includes('singleGenerationCreditCost + uploadedReferenceCreditCost.value'), 'workspace image-to-image cost must add one generated image and uploaded references once')
assert(workspace.includes('生成 1 张 + 参考图 ${uploadedReferenceCount.value} 张'), 'workspace must explain single-image plus reference-image credit cost')
assert(workspace.includes('预计消耗 ${estimatedGenerationCredits.value} 点'), 'workspace must explain text-to-image credit cost')
assert(workspace.includes('点数不足，本次预计消耗'), 'workspace must block submit when credits do not cover the estimated cost')
assert(workspace.includes('credit-cost-hint'), 'workspace must show the estimated credit cost near generation count')
assert(workspace.includes('const pendingTask = buildPendingGenerationTask(created, requestPayload, new Date().toISOString())'), 'workspace must retain the created async task with normalized pending metadata')
assert(workspace.includes('addPendingGenerations([pendingTask])'), 'workspace must save the created task id before polling')
assert(workspace.includes('startPolling([created.generation_id])'), 'workspace must poll only the single created task')
assert(!workspace.includes('async function waitForGenerationTask'), 'workspace must not wait for one image to finish before creating the next selected task')
assert(workspace.includes("taskMessage.value = '任务已提交，正在生成'"), 'workspace must show single task progress copy')
assert(workspace.includes('addPendingGenerations'), 'workspace must save async generation ids before users leave the page')
assert(workspace.includes("if (activeMode.value !== 'image') return []"), 'workspace must not attach reference previews to text-to-image pending tasks')
assert(workspace.includes('reference_preview_urls: requestPayload.reference_asset_ids?.length || requestPayload.reference_work_ids?.length ? currentReferencePreviewUrls() : []'), 'workspace pending tasks must include reference previews only for image-to-image progress cards')
assert(workspace.includes("const activeMode = ref('text')"), 'workspace must default to text-to-image mode')
assert(workspace.includes('const modeOptions = ['), 'workspace must expose the screenshot work-mode control model')
assert(workspace.includes("label: '文生图'"), 'workspace work-mode control must label the current mode as 文生图')
assert(workspace.includes("label: '图生图'"), 'workspace must preserve the image-to-image work mode')
assert(workspace.includes('class="topbar"'), 'workspace must preserve the top brand/history navigation')
assert(workspace.includes('class="reference-panel"'), 'workspace must preserve the reference upload panel for image-to-image mode')
assert(workspace.includes('v-if="activeMode === \'image\'" class="reference-panel"'), 'reference upload panel must only show in image-to-image mode')
assert(workspace.includes('class="size-card"'), 'workspace must render direct size cards instead of an upload-first flow')
assert(workspace.includes('class="generation-settings-grid"'), 'workspace must render quality and creativity as grouped controls')
assert(workspace.includes("const quality = ref('high')"), 'workspace must default image quality to high')
assert(workspace.includes('quality: quality.value'), 'workspace submit payload must send the selected quality')
assert(workspace.includes('function chooseReferences'), 'image-to-image workspace must expose image upload selection')
assert(workspace.includes('上传图片'), 'image-to-image workspace must render upload image copy')
assert(workspace.includes('请至少上传1张参考图'), 'image-to-image workspace must require a reference image only in image mode')
assert(workspace.includes('function normalizeImageSource'), 'workspace must normalize dynamic image sources before rendering them')
assert(workspace.includes("const blockedPlainTextImageSources = ['开始生成']"), 'workspace image-source guard must reject plain button text so mp-weixin does not request it as a local image')
assert(workspace.includes('return normalizeImageSource('), 'workspace history covers must return only renderable image sources')
assert(workspace.includes('prompt-mode-picker'), 'workspace must expose prompt optimization direction picker inside the prompt box')
assert(workspace.includes('function selectPromptOptimizerMode'), 'workspace must handle prompt optimization direction changes inline')
assert(workspace.includes("key: 'portrait_detail'"), 'workspace prompt optimizer must expose portrait detail mode')
assert(workspace.includes('人脸高清'), 'workspace prompt optimizer must label portrait detail mode')
assert(workspace.includes('prompt.value = optimized'), 'prompt optimization must write the optimized prompt back into the prompt textarea')
assert(!workspace.includes('showPromptOptimizer'), 'prompt optimization must not open a bottom overlay')
assert(!workspace.includes('prompt-optimizer-backdrop'), 'prompt optimization must not render a bottom overlay')
assert(workspace.includes('showTemplateSheet'), 'workspace must render an inline prompt template library sheet')
assert(workspace.includes('function openPromptTemplates'), 'workspace prompt template button must open the template library')
assert(workspace.includes('@click="openPromptTemplates"'), 'workspace prompt template button must not open history')
assert(workspace.includes('api.listPromptTemplates'), 'workspace template library must load prompt templates from backend')
assert(workspace.includes('api.usePromptTemplate'), 'workspace template library must use backend template usage endpoint')
assert(workspace.includes('使用 1 点'), 'workspace template library must show the one-credit usage cost')
assert(workspace.includes('prompt.value = nextPrompt'), 'workspace template usage must write the template prompt into the prompt textarea')
assert(workspace.includes('availableCredits.value = payload.available_credits'), 'workspace template usage must update the remaining credit count')
assert(workspace.includes('templatePreviewURL(item)'), 'workspace template library must render template effect previews')
assert(workspace.includes('function previewPromptTemplate'), 'workspace template library must expose a template preview handler')
assert(workspace.includes('uni.previewImage'), 'workspace template preview handler must use the native image preview API')
assert(
  (workspace.match(/@click\.stop="previewPromptTemplate\(item\)"/g) || []).length >= 2,
  'workspace template thumbnail and ratio badge must both open the native image preview'
)
assert(workspace.includes('template-ratio-badge'), 'workspace template ratio badge must keep a dedicated preview tap target class')
assert(!workspace.includes('template-category-row'), 'workspace template library must not render the cramped horizontal category row')
assert(!workspace.includes('selectedTemplateCategory'), 'workspace template library must not keep category selection state')
assert(!workspace.includes('visibleTemplateItems'), 'workspace template library must render the backend template list directly')
assert(!workspace.includes('selectTemplateCategory'), 'workspace template library must not keep category chip handlers')
assert(workspace.includes('showModeSheet'), 'workspace must expose the mode selection modal')
assert(workspace.includes('工作模式'), 'workspace mode switch must label the control as 工作模式')
assert(workspace.includes('class="mode-label"'), 'workspace mode switch must render the left label as static text')
assert(!workspace.includes('<button type="button" @click="chooseTextMode">文字转图片</button>'), 'workspace must not expose a top-level text-to-image button')
assert(workspace.includes('<text class="modal-title">选择工作模式</text>'), 'workspace mode sheet title must remain visible')
assert(workspace.includes('<text>文生图</text>'), 'workspace mode sheet must list 文生图')
assert(workspace.includes('<text>图生图</text>'), 'workspace mode sheet must list 图生图')
assert(workspace.includes('<text>情侣相册</text>'), 'workspace mode sheet must list 情侣相册')
assert(workspace.includes('上传双人照片，生成可分享的旅行相册'), 'workspace couple album mode card must describe the travel album flow')
assert(workspace.includes('function openCoupleAlbumMode'), 'workspace must expose a couple album mode navigation handler')
assert(workspace.includes('navigateTo(routes.coupleAlbumCreate)'), 'workspace couple album mode must navigate to the couple album create page')
assert(workspace.includes(":src=\"icon('favorite')\""), 'workspace couple album mode card must reuse the favorite icon')
assert(!workspace.includes('文字转图片建设中'), 'workspace text-to-image mode must not show construction toast')
const activeModeDeclaration = workspace.match(/const activeMode = ref\('text'\)/)?.[0] || ''
assert.equal(activeModeDeclaration, "const activeMode = ref('text')", 'workspace activeMode must remain scoped to text/image generation modes')
assert(!workspace.includes("activeMode.value = 'couple"), 'workspace couple album entry must not mutate activeMode')
const modeModalStyle = cssRule(workspace, '.mode-modal')
assert(modeModalStyle.includes('max-height:'), 'workspace mode modal must constrain height for three mode cards')
assert(modeModalStyle.includes('overflow-y: auto;'), 'workspace mode modal must scroll vertically on small screens')
assert(workspace.includes("const stylePreset = ref('')"), 'workspace style preset must default to no style')
assert(workspace.includes("value: ''") && workspace.includes('无风格'), 'workspace style presets must include a no-style option')
assert(workspace.includes("stylePreset.value = `${payload.style_preset || ''}`.trim()"), 'history import must preserve empty style instead of forcing 写实')
assert(workspace.includes('function applyOptionalStylePayload'), 'workspace must centralize optional style payload handling')
assert(workspace.includes('if (stylePreset.value)'), 'workspace submit must add style fields only when a style is selected')
assert(!workspace.includes("|| '写实'"), 'workspace must not fallback missing style presets to 写实')

const promptInputBlock = workspace.match(/<textarea[\s\S]*?v-model="prompt"[\s\S]*?\/>/)?.[0] || ''
const negativePromptInputBlock = workspace.match(/<textarea[\s\S]*?v-model="negativePrompt"[\s\S]*?\/>/)?.[0] || ''
assert(promptInputBlock.includes('auto-height'), 'prompt textarea must grow with typed content')
assert(negativePromptInputBlock.includes('auto-height'), 'negative prompt textarea must grow with typed content')
assert(negativePromptInputBlock.includes('class="negative-input"'), 'negative prompt must remain styled as the negative input field')

const submitStart = workspace.indexOf('async function submitGeneration')
const submitEnd = workspace.indexOf('function retryLastGeneration')
const submitBlock = submitStart >= 0 && submitEnd > submitStart ? workspace.slice(submitStart, submitEnd) : ''
assert(submitBlock.includes("activeMode.value === 'image'"), 'submit must branch into image upload validation only for image-to-image mode')
assert(submitBlock.includes('!hasUploadedReference.value'), 'submit must reject image-to-image requests without uploaded references')
assert(submitBlock.includes('reference_asset_ids: uploadedReferenceIds.value'), 'image-to-image payloads must include uploaded reference ids')
assert(submitBlock.includes('delete requestPayload.reference_asset_ids'), 'text-to-image payloads must omit reference_asset_ids')
assert(submitBlock.includes('uploadedReferenceCount.value >= 2'), 'multi-reference image-to-image submit must detect precise compose mode')
assert(submitBlock.includes("requestPayload.reference_intent = 'compose'"), 'multi-reference image-to-image payloads must submit compose intent')
assert(!submitBlock.includes('background_reference_index'), 'multi-reference image-to-image payloads must leave background selection to backend planning unless explicitly chosen')
assert(submitBlock.includes('delete requestPayload.variation_prompt'), 'compose image-to-image payloads must omit the creativity variation prompt')
assert(submitBlock.includes('createSingleGeneration(requestPayload)'), 'submit must delegate async generation creation to a single task after validation')
assert(
  submitBlock.indexOf("activeMode.value === 'image'") >= 0 &&
    submitBlock.indexOf('createSingleGeneration(requestPayload)') > submitBlock.indexOf("activeMode.value === 'image'"),
  'reference image validation must happen before creating the single generation task'
)
assert(workspace.indexOf('api.createImageGeneration') > workspace.indexOf('async function createSingleGeneration'), 'single generation helper must create the async task after submit validation')
assert(workspace.includes('retryLastGeneration'), 'workspace must expose a retry action for failed generations')
assert(workspace.includes('failed?.error?.message'), 'workspace must display backend failure reasons for failed generations')
assert(workspace.includes('data-testid="mobile-generation-retry"'), 'workspace must render a retry control for retryable failed generations')
assert(workspace.includes('credits_insufficient'), 'workspace must handle insufficient-credit generation failures explicitly')
assert(workspace.includes('点数不足，请先充值后再生成'), 'workspace must show a clear insufficient-credit message')
assert(workspace.includes('api.estimateImageGeneration'), 'workspace submit must call the backend credit estimate before creating generation tasks')
assert(workspace.includes('applyInsufficientCreditsEstimate'), 'workspace must render structured insufficient-credit estimate details')
assert(workspace.includes('预计消耗') && workspace.includes('当前余额') && workspace.includes('还差') && workspace.includes('推荐套餐'), 'workspace must show required, available, missing credits and package recommendation')
assert(workspace.includes("source: activeMode.value === 'image' ? 'image_to_image' : 'text_to_image'"), 'workspace pricing CTA must include the generation source')
assert(workspace.includes('package_id: estimate?.recommended_package?.id'), 'workspace pricing CTA must pass the recommended package id')
assert(workspace.includes('missing_credits: estimate?.missing_credits'), 'workspace pricing CTA must pass missing credits')
assert(workspace.includes('required_credits: estimate?.required_credits'), 'workspace pricing CTA must pass required credits')
assert(workspace.includes('goPricing'), 'workspace must expose a pricing CTA for insufficient credits')
assert(workspace.includes('去充值'), 'workspace insufficient-credit state must render a pricing CTA')
assert(
  workspace.includes("generationErrorCode !== 'credits_insufficient'"),
  'workspace must hide retry for insufficient-credit failures'
)

const countButtonBlocks = [
  ...workspace.matchAll(/\.count-row button,\r?\n\.count-row uni-button \{[\s\S]*?\r?\n\}/g),
  ...workspace.matchAll(/\.count-row button \{[\s\S]*?\n\}/g)
].map((match) => match[0])
const countButtonBlock = countButtonBlocks.find((block) => block.includes('display: flex;')) || ''
assert(workspace.includes('.count-row uni-button'), 'generation count styles must target H5 uni-button output')
assert(countButtonBlock.includes('display: flex;'), 'generation count buttons must use flex centering')
assert(countButtonBlock.includes('align-items: center;'), 'generation count button text must be vertically centered')
assert(countButtonBlock.includes('justify-content: center;'), 'generation count button text must be horizontally centered')
assert(countButtonBlock.includes('line-height: 1;'), 'generation count button text must avoid default line-height drift')

const worksPagePath = 'src/pages/works/index.vue'
assert(existsSync(resolve(root, worksPagePath)), 'works history page file must exist')
const worksPage = read(worksPagePath)
assert(worksPage.includes('api.listWorks'), 'works page must read real works from /api/works')
assert(worksPage.includes('api.listCoupleAlbums'), 'works page must read couple albums for album cards')
assert(
  worksPage.includes('exclude_album_pages: true'),
  'works page must ask /api/works to hide works already bound to couple album pages'
)
assert(worksPage.includes('function normalizeCoupleAlbumsPayload'), 'works page must normalize couple album list payloads')
assert(worksPage.includes('function isAlbumCard'), 'works page must distinguish couple album cards from ordinary works')
assert(worksPage.includes('routes.coupleAlbumDetail'), 'works page album cards must navigate to the couple album detail route')
assert(worksPage.includes('api.shareCoupleAlbum'), 'works page album cards must share through the couple album API')
assert(worksPage.includes('album-card'), 'works page must render a distinct couple album card')
assert(worksPage.includes('查看相册'), 'works page album cards must expose a view-album action')
assert(worksPage.includes('分享相册'), 'works page album cards must expose a share-album action')
assert(worksPage.includes('const worksPageSize = 50'), 'works page must keep a fixed lazy-load page size')
assert(worksPage.includes('const currentPage = ref(1)'), 'works page must track the current works page')
assert(worksPage.includes('const hasMore = ref(false)'), 'works page must track whether more works can be loaded')
assert(worksPage.includes('const loadingMore = ref(false)'), 'works page must track append-page loading separately')
assert(worksPage.includes('function resetAndLoadWorks'), 'works page filters must reset pagination before loading')
assert(worksPage.includes('function loadNextWorksPage'), 'works page must expose next-page loading')
assert(worksPage.includes('onReachBottom'), 'works page must lazy-load more works when the user reaches the bottom')
assert(worksPage.includes('append: true'), 'works page next-page loading must append instead of replacing existing works')
assert(worksPage.includes('is_favorite'), 'favorites tab must filter by is_favorite')
assert(worksPage.includes("poster_kv"), 'works categories must include poster KV')
assert(worksPage.includes("product_main"), 'works categories must include product main image')
assert(worksPage.includes("cover"), 'works categories must include cover')
assert(!worksPage.includes("{ key: 'audio'"), 'works categories must not include unsupported audio tab')
assert(worksPage.includes('function submitSearch'), 'works page must submit keyword search')
assert(worksPage.includes('function toggleLayout'), 'works page must toggle list/grid layout')
assert(worksPage.includes('function toggleFavorite'), 'works page must update favorite state')
assert(worksPage.includes('function toggleVisibility'), 'works page must update public/private visibility')
assert(worksPage.includes('enableMiniProgramShare'), 'works page must enable native mini-program sharing for work share buttons')
assert(worksPage.includes('function shareWork'), 'works page must prepare native public work sharing')
assert(worksPage.includes('open-type="share"'), 'works page share button must use the native mini-program share open type')
assert(worksPage.includes('data-share-kind="work"'), 'works page share button must mark work share targets')
assert(worksPage.includes(':data-share-ids="workShareIDs(work)"'), 'works page share button must expose work ids through dataset')
assert(worksPage.includes('data-share-kind="album"'), 'works page album share button must mark album share targets')
assert(worksPage.includes('@click.stop="shareAlbumCard(work)"'), 'works page album share action must enable sharing through the couple album API before native sharing')
assert(worksPage.includes(':data-share-token="sharePanelToken()"'), 'works page native share panel must expose the API-returned album share token through dataset')
assert(worksPage.includes('function workSharePayload'), 'works page must resolve native share payloads from button datasets')
assert(worksPage.includes('function albumSharePayload'), 'works page must resolve native album share payloads from button datasets')
assert(worksPage.includes('function openNativeSharePanel'), 'works page must open the native share panel after publishing private works')
assert(worksPage.includes('function isMissingWorkUpdateRoute'), 'works page must classify missing PATCH work-update routes')
assert(
  worksPage.includes("error?.status === 404 && error?.code === 'not_found'"),
  'works page must only show the restart-backend hint for 404 not_found work-update failures'
)
assert(
  worksPage.includes('当前服务版本不支持公开分享，请重启后端服务后再试'),
  'works page must show a specific restart-backend hint when PATCH /api/works/:id is missing'
)
assert(worksPage.includes('function moreActions'), 'works page must expose real more actions')
assert(
  worksPage.includes("import { api, getStoredAuthToken }"),
  'works page downloads must read the mini-program auth token for protected work files'
)
assert(
  worksPage.includes('function resolveDownloadURL'),
  'works page downloads must normalize relative backend download URLs before calling uni.downloadFile'
)
assert(
  worksPage.includes('header: buildDownloadHeaders()'),
  'works page downloads must pass auth headers into uni.downloadFile instead of downloading anonymously'
)
assert(
  worksPage.includes('function saveDownloadedFileToPhone'),
  'works page downloads must save the downloaded file to the phone instead of exposing a link'
)
assert(
  worksPage.includes('const downloadTimeoutMS'),
  'works page downloads must define an explicit mini-program download timeout'
)
assert(
  worksPage.includes('downloadTask.abort()'),
  'works page downloads must abort stalled mini-program download tasks instead of leaving the loading spinner forever'
)
assert(
  worksPage.includes('clearDownloadTimeout'),
  'works page downloads must clear the timeout in every download callback path'
)
assert(
  worksPage.includes("uni.showLoading({ title: '正在保存'"),
  'works page downloads must show a distinct saving state after the file has downloaded'
)
assert(
  worksPage.includes('hidePhoneSaveLoading'),
  'works page phone-save callbacks must always close the saving loading state'
)
assert(
  !worksPage.includes("copyText(url, '下载链接已复制')"),
  'works page download failures must not fall back to copying the download link'
)
assert(worksPage.includes('function deleteWork'), 'works page must delete works after confirmation')
assert(worksPage.includes('api.updateWork'), 'works page must call PATCH /api/works/:id')
assert(worksPage.includes('api.deleteWork'), 'works page must call DELETE /api/works/:id')
assert(worksPage.includes('api.reuseWork'), 'works page must call POST /api/works/:id/reuse')
assert(worksPage.includes('/api/public/works/'), 'works page must copy public share links')
assert(worksPage.includes('loadPendingGenerations'), 'works page must show locally pending async generation tasks')
assert(worksPage.includes('api.getImageGeneration'), 'works page must poll pending generation status')
assert(
  worksPage.includes('function pendingGenerationInputMode'),
  'works page must preserve text/image mode for locally pending generation tasks'
)
assert(
  !worksPage.includes("mode: 'image',\n    tool_mode: 'image'"),
  'works page must not force all pending generation tasks into image-to-image mode'
)
assert(
  worksPage.includes('v-if="hasWorkThumbnail(work)"'),
  'works page must not render a stale default image when a pending work has no real thumbnail'
)
assert(
  worksPage.includes('class="cover-placeholder"'),
  'works page must render an explicit placeholder for generation tasks without result images'
)
assert(worksPage.includes('const groupedDisplayWorks = computed'), 'works page must group works that share a batch_id into one card')
assert(worksPage.includes('function groupWorksByBatch'), 'works page must centralize batch grouping behavior')
assert(worksPage.includes('function mergeIncompleteBatchGroups'), 'works page must merge incomplete batch groups when backend batch ids differ or are missing')
assert(worksPage.includes('function fallbackBatchGroupKey'), 'works page must have a stable fallback grouping key for same-click image batches')
assert(worksPage.includes('const fallbackLimit = expectedTotal > 1 ? expectedTotal : 4'), 'works page fallback grouping must merge same-prompt near-time works even when batch_total is missing')
assert(worksPage.includes('function canFallbackBatchGroup'), 'works page must allow fallback grouping for same-prompt near-time works')
assert(!worksPage.includes('if (!promptText || total <= 1) return'), 'works page fallback grouping must not depend on batch_total because stale backends may return 1张')
assert(worksPage.includes('batch_items'), 'works page grouped cards must keep their child image items')
assert(worksPage.includes('function isBatchWork'), 'works page must distinguish grouped batch cards from single works')
assert(worksPage.includes('@click="previewWork(work)"'), 'clicking a work card must open the multi-image preview')
assert(worksPage.includes('previewOverlayVisible'), 'works page must manage a custom preview overlay state')
assert(worksPage.includes('<swiper'), 'works page preview must use a swiper for left/right image switching')
assert(worksPage.includes('下载当前'), 'works page preview must expose a download-current-image action')
assert(worksPage.includes('saveImageToPhotosAlbum'), 'works page mini-program downloads must save the current image to the album when possible')
assert(worksPage.includes('pendingFailureText'), 'works page must render pending failed task error reasons')
assert(
  worksPage.includes('retryPendingGeneration') && worksPage.includes("!workID(work)") && worksPage.includes('navigateTo(routes.imageToImage'),
  'works page must retry pending failed generations without requiring a persisted work_id'
)
assert(worksPage.includes('requireAuth'), 'works page must require login before loading private works')
assert(worksPage.includes('startPendingPolling'), 'works page must keep progress updating while visible')
assert(worksPage.includes('class="page-title"'), 'works page must render the large 作品库 title from the approved design')
assert(worksPage.includes('class="category-tabs"'), 'works page must render category tabs like 全部/图片/视频/音效/收藏')
assert(worksPage.includes('class="utility-actions"'), 'works page must render search/list utility actions beside filters')
assert(worksPage.includes('class="cover-ratio-badge"'), 'work cards must overlay the ratio badge on the cover image')
assert(worksPage.includes('class="visibility-actions"'), 'work cards must render eye/favorite/more actions in the top-right')
assert(worksPage.includes('class="card-toolbar"'), 'work cards must render the bottom reuse/regenerate/share/more toolbar')
assert(worksPage.includes('复用提示词'), 'work card toolbar must include reuse prompt action')
assert(worksPage.includes('转图生图'), 'work card toolbar must include image-to-image transform action')
assert(worksPage.includes('transformWorkToImage'), 'works page must route the transform action through image-to-image reuse')
assert(workspace.includes('reference_work_ids'), 'workspace must submit reused work images as image-to-image references')

assert(routes.includes('params = {}'), 'navigateTo must accept query params')
assert(routes.includes('function buildRouteQuery'), 'navigateTo must centralize mini-program compatible query encoding')
assert(!routes.includes('new URLSearchParams'), 'navigateTo must not use browser-only URLSearchParams')
assert(workspace.includes('onLoad'), 'workspace must receive route query parameters')
assert(workspace.includes('prefillFromQuery'), 'workspace must prefill prompt and aspect from query')
assert(workspace.includes('requireAuth'), 'workspace page must require login before loading protected API state')

const recordTabBlocks = [
  ...worksPage.matchAll(/\.category-tabs button,\r?\n\.category-tabs uni-button \{[\s\S]*?\r?\n\}/g),
  ...worksPage.matchAll(/\.category-tabs button \{[\s\S]*?\r?\n\}/g)
].map((match) => match[0])
const recordTabBlock = recordTabBlocks.find((block) => block.includes('display: flex;')) || ''
assert(worksPage.includes('.category-tabs uni-button'), 'works category tabs must target H5 uni-button output')
assert(recordTabBlock.includes('display: flex;'), 'works record tab text must use flex centering')
assert(recordTabBlock.includes('align-items: center;'), 'works record tab text must be vertically centered')
assert(recordTabBlock.includes('justify-content: center;'), 'works record tab text must be horizontally centered')
assert(recordTabBlock.includes('line-height: 1;'), 'works record tab text must avoid default line-height drift')

const pricingPagePath = 'src/pages/pricing/index.vue'
assert(existsSync(resolve(root, pricingPagePath)), 'pricing page file must exist')
const pricingPage = read(pricingPagePath)
assert(
  pricingPage.includes('选择适合你的') && pricingPage.includes('创作套餐'),
  'pricing page must render the screenshot hero title'
)
assert(pricingPage.includes('class="billing-toggle"'), 'pricing page must render purchase/company segmented control')
for (const packageName of ['体验包', '入门包', '常用包', '进阶包', '专业包', '旗舰包']) {
  assert(pricingPage.includes(packageName), `pricing page fallback packages must include ${packageName}`)
}
assert(
  pricingPage.includes('recommended') && pricingPage.includes('专业包') && pricingPage.includes('最划算'),
  'pricing page must highlight the recommended and best-value packages'
)
assert(pricingPage.includes('class="benefit-grid"'), 'pricing page must render the three benefit summary cards')
assert(pricingPage.includes('class="comparison-table"'), 'pricing page must render the benefits comparison table')
assert(pricingPage.includes('class="faq-panel"'), 'pricing page must render FAQ rows')
assert(pricingPage.includes('class="help-strip"'), 'pricing page must render the customer service help strip')
assert(pricingPage.includes('api.getPackages'), 'pricing page must load real packages from /api/packages')
assert(pricingPage.includes('api.getCustomerService'), 'pricing page must load customer service config')
assert(pricingPage.includes('onLoad'), 'pricing page must read recharge guide query parameters')
assert(pricingPage.includes('rechargeGuide'), 'pricing page must store the generation shortfall guide')
assert(pricingPage.includes('本次还差'), 'pricing page must show the missing-credit guide copy')
assert(pricingPage.includes('推荐购买'), 'pricing page must show the recommended package guide copy')
assert(pricingPage.includes('recharge-recommended'), 'pricing page must highlight the recommended package card')
assert(pricingPage.includes('plans.value.map') || pricingPage.includes('sourcePackages.value.map'), 'pricing page must render every loaded package card')
assert(!pricingPage.includes('slice(0, 3)'), 'pricing page must not cap package cards at three')
assert(pricingPage.includes('uni.login'), 'pricing page must use WeChat login before mini-program payment')
assert(pricingPage.includes('api.wechatLogin'), 'pricing page must auto login with WeChat when needed')
assert(pricingPage.includes('api.wechatBind'), 'pricing page must bind openid for existing accounts before payment')
assert(pricingPage.includes('api.createWechatVirtualPayOrder'), 'pricing page must create a WeChat virtual payment order')
assert(pricingPage.includes('wx.requestVirtualPayment'), 'pricing page must invoke the WeChat virtual payment cashier')
assert(pricingPage.includes('api.confirmWechatVirtualPayOrder'), 'pricing page must confirm virtual payment with the backend after requestVirtualPayment')
assert(pricingPage.includes('payWechatVirtualPlan(plan, user, 1)'), 'pricing page must route payment through a retry-aware helper')
assert(pricingPage.includes('isWechatOrderClosed(error)'), 'pricing page must detect ORDER_CLOSED virtual payment failures')
assert(pricingPage.includes('attempt < 2'), 'pricing page must retry ORDER_CLOSED at most once')
assert(pricingPage.includes('force_new: true'), 'pricing page must request a forced replacement order after ORDER_CLOSED')
assert(pricingPage.includes('stale_order_number'), 'pricing page must pass the stale order number when replacing a closed order')
assert(pricingPage.includes("stale_reason: 'ORDER_CLOSED'"), 'pricing page must identify ORDER_CLOSED as the stale replacement reason')
assert(pricingPage.includes("payload?.payment_state === 'already_paid'"), 'pricing page must skip cashier when backend reports an already-paid virtual order')
assert(pricingPage.includes('订单状态已刷新失败，请重新点击支付或联系客服'), 'pricing page must show friendly feedback when the automatic ORDER_CLOSED retry also fails')
assert(pricingPage.includes("result?.code === 'wechat_virtual_pay_pending'"), 'pricing page must handle backend-confirmed virtual payment pending results')
assert(!pricingPage.includes('uni.requestPayment'), 'pricing page must not call ordinary JSAPI uni.requestPayment for package checkout')
assert(pricingPage.includes('支付成功，点数已到账'), 'pricing page must show paid-and-credited success feedback')
assert(pricingPage.includes('支付处理中，请稍后刷新'), 'pricing page must show pending feedback when backend query_order has not confirmed payment')
assert(pricingPage.includes('支付已取消'), 'pricing page must show cancellation feedback')
assert(pricingPage.includes('error?.errMsg'), 'pricing page must surface WeChat virtual payment fail errMsg')
assert(pricingPage.includes('立即支付'), 'pricing package CTA must be direct payment')
assert(!pricingPage.includes('api.createPurchaseIntent'), 'pricing page must not submit purchase intents')
assert(!pricingPage.includes(`source: 'mobile-h5'`), 'pricing page must not keep purchase-intent source payload')
assert(!pricingPage.includes('showPurchaseForm'), 'pricing page must remove purchase intent modal state')
assert(!pricingPage.includes('submitPurchaseIntent'), 'pricing page must remove purchase intent submission')
assert(!pricingPage.includes('提交购买意向'), 'pricing page must not show purchase intent copy')
assert(!pricingPage.includes('已选择${plan.name}'), 'pricing package buttons must not stop at local selection toast')
assert(!pricingPage.includes('客服功能建设中'), 'pricing page customer service controls must not show construction toast')
assert(pricingPage.includes('active-key="pricing"'), 'pricing page must highlight 套餐 through AppTabbar')
const pricingSelectButtonBlock = pricingPage.match(/\.select-button \{[\s\S]*?\n\}/)?.[0] || ''
const billingToggleButtonBlock = pricingPage.match(/\.billing-toggle button \{[\s\S]*?\n\}/)?.[0] || ''
const pricingPlanTagBlock = pricingPage.match(/\.plan-tag \{[\s\S]*?\n\}/)?.[0] || ''
assert(pricingSelectButtonBlock.includes('display: flex;'), 'pricing select buttons must use flex centering')
assert(pricingSelectButtonBlock.includes('align-items: center;'), 'pricing select button text must be vertically centered')
assert(pricingSelectButtonBlock.includes('justify-content: center;'), 'pricing select button text must be horizontally centered')
assert(billingToggleButtonBlock.includes('display: flex;'), 'pricing billing toggle buttons must use flex centering')
assert(billingToggleButtonBlock.includes('align-items: center;'), 'pricing billing toggle text must be vertically centered')
assert(billingToggleButtonBlock.includes('justify-content: center;'), 'pricing billing toggle text must be horizontally centered')
assert(pricingPlanTagBlock.includes('display: inline-flex;'), 'pricing plan tags must use inline-flex centering')

const accountPagePath = 'src/pages/account/index.vue'
assert(existsSync(resolve(root, accountPagePath)), 'account page file must exist')
const accountPage = read(accountPagePath)
assert(accountPage.includes('api.getMe'), 'account page must read current user profile')
assert(accountPage.includes('api.getCredits'), 'account page must read account credits')
assert(accountPage.includes('api.getCreditTransactions'), 'account page must read account credit transactions')
assert(accountPage.includes('api.updateProfile'), 'account page must update profile through the backend')
assert(accountPage.includes('api.sendSMSCode'), 'account page must request phone-binding SMS codes')
assert(accountPage.includes('api.bindAccountPhone'), 'account page must bind legacy account phones through the backend')
assert(accountPage.includes('api.bindWechatPhone'), 'account page must bind WeChat authorized phones through the current-account backend')
assert(accountPage.includes('bind_phone'), 'account page SMS code purpose must distinguish phone binding from registration')
assert(accountPage.includes('api.updateAccountEmail'), 'account page must update email through the backend')
assert(accountPage.includes('api.updateAccountPreferences'), 'account page must update notification preferences through the backend')
assert(accountPage.includes('api.changePassword'), 'account page must change login password through the backend')
assert(accountPage.includes('api.setPaymentPassword'), 'account page must set payment password through the backend')
assert(accountPage.includes('api.clearPaymentPassword'), 'account page must clear payment password through the backend')
assert(accountPage.includes('api.logout'), 'account page must logout through the backend')
assert(accountPage.includes('requireAuth'), 'account page must require login before loading account data')
assert(!accountPage.includes('建设中'), 'account page must not keep construction toasts')
assert(!accountPage.includes('?? 29'), 'account page must not hardcode a fallback credit balance')
assert(!accountPage.includes('wechat-capsule'), 'account page must not draw a fake mini-program capsule')
assert(!accountPage.includes('mini-program-capsule'), 'account page must not draw a fake mini-program capsule')
assert(accountPage.includes('class="profile-hero"'), 'account page must render the screenshot profile hero')
assert(accountPage.includes('class="credit-card"'), 'account page must render the available credit card')
assert(accountPage.includes('class="transaction-panel"'), 'account page must render credit transaction panel')
assert(accountPage.includes('class="transaction-empty"'), 'account page must render the screenshot empty transaction state')
assert(accountPage.includes('查看全部'), 'account page transaction panel must keep the 查看全部 entry')
assert(accountPage.includes('openCreditTransactions'), 'account page must expose a point transaction detail entry handler')
assert(accountPage.includes('@tap="openCreditTransactions"'), 'account page 查看全部 entry must be tappable')
assert(accountPage.includes('class="security-panel"'), 'account page must render security settings panel')
assert(accountPage.includes('登录保护'), 'account page must include login protection toggle row')
assert(!accountPage.includes('class="device-panel"'), 'account page must remove the old device management panel')
assert(!accountPage.includes('class="help-card"'), 'account page must remove the old help center row')
assert(!accountPage.includes('class="qr-card"'), 'account page must remove account-page customer service QR card')
assert(!accountPage.includes('class="faq-panel"'), 'account page must remove account-page FAQ panel')
assert(accountPage.includes('class="logout-button"'), 'account page must render logout button')
assert(accountPage.includes('active-key="account"'), 'account page must highlight 我的 through AppTabbar')

const accountTransactionsPagePath = 'src/pages/account/transactions/index.vue'
assert(existsSync(resolve(root, accountTransactionsPagePath)), 'account transactions page file must exist')
const accountTransactionsPage = read(accountTransactionsPagePath)
assert(accountTransactionsPage.includes('点数流水'), 'account transactions page must render the credit transaction title')
assert(accountTransactionsPage.includes('全部') && accountTransactionsPage.includes('收入') && accountTransactionsPage.includes('支出'), 'account transactions page must render all transaction filters')
assert(accountTransactionsPage.includes('api.getCreditTransactions'), 'account transactions page must load credit transactions through the backend API')
assert(accountTransactionsPage.includes('page_size'), 'account transactions page must request paginated transactions')
assert(accountTransactionsPage.includes('has_more'), 'account transactions page must honor backend pagination metadata')
assert(accountTransactionsPage.includes('加载更多'), 'account transactions page must render a load-more control')
assert(accountTransactionsPage.includes('没有更多了'), 'account transactions page must render an exhausted state')
assert(accountTransactionsPage.includes('暂无流水记录'), 'account transactions page must render an empty state')
assert(accountTransactionsPage.includes('重试'), 'account transactions page must render a retry control for load failures')
assert(accountTransactionsPage.includes('transactionTitle'), 'account transactions page must reuse transaction title presentation logic')
assert(accountTransactionsPage.includes('transactionKind'), 'account transactions page must classify income and expense rows')

const authPagePath = 'src/pages/auth/index.vue'
assert(existsSync(resolve(root, authPagePath)), 'auth page file must exist')
const authPage = read(authPagePath)
assert(authPage.includes('api.login'), 'auth page must call login API')
assert(authPage.includes('api.register'), 'auth page must call register API')
assert(authPage.includes("mode.value = 'login'"), 'auth page must support login mode')
assert(authPage.includes("mode.value = 'register'"), 'auth page must support register mode')
assert(authPage.includes('redirect'), 'auth page must preserve redirect target')

const supportPagePath = 'src/pages/support/index.vue'
assert(existsSync(resolve(root, supportPagePath)), 'support page file must exist')
const supportPage = read(supportPagePath)
assert(supportPage.includes('api.getCustomerService'), 'support page must load customer service config')
assert(supportPage.includes('copyServiceAccount'), 'support page must copy WeChat/QQ customer service accounts')
assert(supportPage.includes('faq'), 'support page must render FAQ content')
assert(supportPage.includes('qr'), 'support page must render customer service QR code when provided')
assert(!supportPage.includes('wechat-capsule'), 'support page must not draw a fake mini-program capsule')
assert(!supportPage.includes('mini-program-capsule'), 'support page must not draw a fake mini-program capsule')
assert(supportPage.includes('class="quick-contact-grid"'), 'support page must render top WeChat/QQ quick contact buttons')
assert(supportPage.includes('复制微信号'), 'support page must expose a direct copy WeChat button')
assert(supportPage.includes('复制QQ号'), 'support page must expose a direct copy QQ button')
assert(supportPage.includes('class="service-tag-grid"'), 'support page must render service tags as compact pills')
assert(supportPage.includes('class="support-channel-grid"'), 'support page must render WeChat and QQ channel cards side by side')
assert(supportPage.includes('微信号：'), 'support page must label the WeChat account')
assert(supportPage.includes('QQ：'), 'support page must label the QQ account')
assert(supportPage.includes('active-key=""'), 'support page must render AppTabbar without highlighting a tab')
assert(!supportPage.includes(":class=\"{ active:"), 'support page must not keep a page-level active tab binding')
assert(!supportPage.includes('建设中'), 'support page must not be a construction placeholder')

const coupleAlbumCreatePath = 'src/pages/couple-album/create/index.vue'
assert(existsSync(resolve(root, coupleAlbumCreatePath)), 'couple album create page file must exist')
const coupleAlbumCreatePage = read(coupleAlbumCreatePath)
const coupleAlbumCreateStyle = coupleAlbumCreatePage.match(/<style[\s\S]*<\/style>/)?.[0] || ''
for (const marker of [
  '520 情侣旅行相册',
  '白霖共享',
  '创作者 AI 图片平台',
  '工作模式',
  '情侣相册',
  '选择工作模式',
  '文生图',
  '图生图',
  '上传情侣照片',
  '{{ uploadedPhotoCount }}/{{ requiredPhotoTotal }}',
  '建议使用正面清晰合照或半身照',
  '男方照片',
  '女方照片',
  '大理洱海',
  '京都樱花',
  '巴黎街角',
  '厦门海岸',
  '上海夜景',
  '故事模板',
  '画面风格',
  '相册标题',
  '开始生成相册',
  '请先上传两张照片',
  'missingPhotoToastText',
  '照片仅用于生成相册，结果默认保存至私有作品库。',
  'api.uploadReferenceAsset',
  'api.createCoupleAlbum',
  'api.generateCoupleAlbum',
  'routes.coupleAlbumDetail',
  'routes.pricing'
]) {
  assert(coupleAlbumCreatePage.includes(marker), `couple album create page missing marker: ${marker}`)
}
for (const marker of [
  'locationCards',
  'function staticIcon',
  "icon('logo-star')",
  "icon('add-image')",
  '#a94af3',
  '#1767ff',
  "staticAsset('couple-album/dali-erhai.png')",
  "staticAsset('couple-album/kyoto-sakura.png')",
  "staticAsset('couple-album/paris-corner.png')",
  "staticAsset('couple-album/xiamen-coast.png')",
  "staticAsset('couple-album/shanghai-night.png')",
  'class="creator-card upload-card"',
  'class="creator-card title-card"',
  'class="photo-slot"',
  'class="location-card"',
  'class="template-grid"',
  'class="style-grid"',
  'class="floating-generate-bar"',
  '<AppTabbar active-key="workspace"',
  'extra-space=',
  'showModeSheet',
  'function chooseTextMode',
  'function chooseImageMode',
  'function chooseCoupleAlbumMode',
  "navigateTo(routes.imageToImage, { mode: 'text' })",
  "navigateTo(routes.imageToImage, { mode: 'image' })",
  '@click="selectLocation(index)"'
]) {
  assert(coupleAlbumCreatePage.includes(marker), `couple album create page missing refreshed maker UI marker: ${marker}`)
}
for (const marker of [
  "value: 'city_walk'",
  "value: 'first_trip'",
  "value: 'anniversary'",
  "value: 'proposal'",
  "value: 'film'",
  "value: 'cinematic'",
  "value: 'watercolor'",
  "value: 'storybook'",
  'title: title.value.trim()',
  'location: selectedLocation.value.value',
  'story_template: selectedTemplate.value.value',
  'style: selectedStyle.value.value',
  'male_reference_asset_id: malePhoto.value.serverId',
  'female_reference_asset_id: femalePhoto.value?.serverId || 0'
]) {
  assert(coupleAlbumCreatePage.includes(marker), `couple album create page must preserve payload/value marker: ${marker}`)
}
for (const name of [
  'dali-erhai.png',
  'kyoto-sakura.png',
  'paris-corner.png',
  'xiamen-coast.png',
  'shanghai-night.png'
]) {
  assert(existsSync(resolve(root, `src/static/couple-album/${name}`)), `missing couple album location asset ${name}`)
  assertPNGDimensions(`src/static/couple-album/${name}`, 720, 420)
}
assert(!coupleAlbumCreatePage.includes('<picker'), 'couple album create page must use image location cards instead of a picker')
assert(!coupleAlbumCreatePage.includes('LOVE JOURNAL'), 'couple album create page must use product workspace chrome instead of campaign page chrome')
assert(!coupleAlbumCreatePage.includes('照片授权'), 'couple album create page must use a compact privacy hint instead of a large photo authorization block')
assert(!coupleAlbumCreatePage.includes('520 情侣故事相册'), 'couple album create page must use the refreshed travel album title')
assert(!coupleAlbumCreatePage.includes('用旅行，记录我们的故事'), 'couple album create page must use the refreshed product-page hero copy')
assert(!coupleAlbumCreatePage.includes('<text>{{ selectedLocation.label }}</text>'), 'couple album create page must not duplicate the selected location in the section heading')
assert(!coupleAlbumCreatePage.includes('<text>{{ selectedTemplate.label }}</text>'), 'couple album create page must not duplicate the selected template in the section heading')
assert(!coupleAlbumCreatePage.includes('<text>{{ selectedStyle.label }}</text>'), 'couple album create page must not duplicate the selected style in the section heading')
assert(!coupleAlbumCreatePage.includes('已选地点'), 'couple album create page must not render a separate selected-location summary')
assert(!coupleAlbumCreatePage.includes('相册类型'), 'couple album create page must label the top switch as 工作模式')
assert(!coupleAlbumCreatePage.includes('<text>情侣旅行</text>'), 'couple album create page must show 情侣相册 as the current work mode')
assert(coupleAlbumCreatePage.includes('api.getCoupleAlbumOptions'), 'couple album create page must load album options from backend config')
assert(coupleAlbumCreatePage.includes('defaultLocationCards'), 'couple album create page must keep fallback location defaults')
assert(coupleAlbumCreatePage.includes('defaultStoryTemplates'), 'couple album create page must keep fallback story template defaults')
assert(coupleAlbumCreatePage.includes('defaultStyles'), 'couple album create page must keep fallback style defaults')
assert(coupleAlbumCreatePage.includes('童年职业梦想相册'), 'couple album create page must support the childhood career dream album mode')
assert(coupleAlbumCreatePage.includes('childhood-dream'), 'couple album create page must accept the childhood-dream mode query')
assert(coupleAlbumCreatePage.includes('childhood_career_dream'), 'couple album create page must submit the childhood career dream story template')
assert(coupleAlbumCreatePage.includes('childhood_dream_stage'), 'couple album create page must submit the childhood dream stage theme')
assert(coupleAlbumCreatePage.includes('children_storybook'), 'couple album create page must offer childhood album style presets')
assert(coupleAlbumCreatePage.includes('补充全身照'), 'couple album create page must make the second child reference optional')
assert(coupleAlbumCreatePage.includes('请先上传孩子照片'), 'couple album create page must use child-specific missing-photo copy')
assert(coupleAlbumCreatePage.includes('dreamRoleCards'), 'couple album create page must show the eight career dream pages')
assert(coupleAlbumCreatePage.includes('function applyCoupleAlbumOptions'), 'couple album create page must normalize backend option config before rendering')
assert(!coupleAlbumCreatePage.includes('const locationCards = ['), 'couple album create page must not use hard-coded locationCards as the only source')
assert(!coupleAlbumCreatePage.includes('const storyTemplates = ['), 'couple album create page must not use hard-coded storyTemplates as the only source')
assert(!coupleAlbumCreatePage.includes('const styles = ['), 'couple album create page must not use hard-coded styles as the only source')
const coupleAlbumTemplateCardStyle = cssRule(coupleAlbumCreateStyle, '.template-card')
for (const marker of [
  'grid-template-columns: minmax(0, 1fr);',
  'justify-content: center;',
  'align-items: center;',
  'justify-items: center;',
  'text-align: center;'
]) {
  assert(coupleAlbumTemplateCardStyle.includes(marker), `couple album template cards must center content with CSS marker: ${marker}`)
}
assert(
  cssRule(coupleAlbumCreateStyle, '.template-card text:last-child').includes('text-align: center;'),
  'couple album template card descriptions must be centered'
)
const coupleAlbumStyleCardStyle = cssRule(coupleAlbumCreateStyle, '.style-card')
for (const marker of ['justify-content: center;', 'align-items: center;', 'text-align: center;']) {
  assert(coupleAlbumStyleCardStyle.includes(marker), `couple album style cards must center content with CSS marker: ${marker}`)
}
const coupleAlbumSubmitStart = coupleAlbumCreatePage.indexOf('async function submitAlbum')
const coupleAlbumSubmitEnd = coupleAlbumCreatePage.indexOf('</script>', coupleAlbumSubmitStart)
const coupleAlbumSubmitBlock =
  coupleAlbumSubmitStart >= 0 && coupleAlbumSubmitEnd > coupleAlbumSubmitStart
    ? coupleAlbumCreatePage.slice(coupleAlbumSubmitStart, coupleAlbumSubmitEnd)
    : ''
assert(coupleAlbumCreatePage.includes('api.estimateCoupleAlbum'), 'couple album submit must estimate credits before creating an album')
assert(coupleAlbumCreatePage.indexOf('api.estimateCoupleAlbum') < coupleAlbumCreatePage.indexOf('api.createCoupleAlbum'), 'couple album estimate must run before album creation')
assert(coupleAlbumCreatePage.includes('applyInsufficientCreditsEstimate'), 'couple album page must render structured insufficient-credit estimate details')
assert(coupleAlbumCreatePage.includes('预计消耗') && coupleAlbumCreatePage.includes('当前余额') && coupleAlbumCreatePage.includes('还差') && coupleAlbumCreatePage.includes('推荐套餐'), 'couple album page must show required, available, missing credits and package recommendation')
assert(coupleAlbumCreatePage.includes("source: 'couple_album'"), 'couple album pricing CTA must include source=couple_album')
assert(coupleAlbumCreatePage.includes('package_id: estimate?.recommended_package?.id'), 'couple album pricing CTA must pass the recommended package id')
assert(coupleAlbumCreatePage.includes('missing_credits: estimate?.missing_credits'), 'couple album pricing CTA must pass missing credits')
assert(coupleAlbumCreatePage.includes('required_credits: estimate?.required_credits'), 'couple album pricing CTA must pass required credits')
assert(!coupleAlbumCreatePage.includes('setTimeout(goPricing'), 'couple album insufficient-credit handling must not auto redirect to pricing')
assert(
  coupleAlbumSubmitBlock.indexOf('missingRequiredPhoto.value') >= 0 &&
    coupleAlbumSubmitBlock.indexOf('const me = await requireAuth()') >
      coupleAlbumSubmitBlock.indexOf('missingRequiredPhoto.value'),
  'couple album submit must show the missing-photo toast before invoking auth redirects'
)

const coupleAlbumDetailPath = 'src/pages/couple-album/detail/index.vue'
assert(existsSync(resolve(root, coupleAlbumDetailPath)), 'couple album detail page file must exist')
const coupleAlbumDetailPage = read(coupleAlbumDetailPath)
for (const marker of [
  'enableMiniProgramShare',
  'api.getCoupleAlbum',
  'api.retryCoupleAlbumPage',
  'api.shareCoupleAlbum',
  'sharePath',
  'shareQuery',
  'coverShareImage',
  "path: sharePath.value",
  "query: shareQuery.value",
  "imageUrl: coverShareImage.value",
  'open-type="share"',
  'data-share-kind="album"',
  ':data-share-token="activeShareToken"',
  'routes.works',
  'album-preview-swiper',
  'previewPages',
  'currentPreviewPageNumber',
  '相册生成中',
  '相册效果',
  '失败页重试',
  '分享相册',
  '下载相册',
  '保存单张图片',
  '保存长图',
  'album-poster-canvas',
  'saveImageToPhotosAlbum',
  'canvasToTempFilePath',
  'download_url || page?.preview_url',
  '进入作品库',
  'page-grid',
  '<AppTabbar'
]) {
  assert(coupleAlbumDetailPage.includes(marker), `couple album detail page missing marker: ${marker}`)
}
assert(!coupleAlbumDetailPage.includes('active-key="couple"'), 'couple album detail page must not add a bottom-nav couple tab')

const coupleAlbumSharePath = 'src/pages/couple-album/share/index.vue'
assert(existsSync(resolve(root, coupleAlbumSharePath)), 'couple album share page file must exist')
const coupleAlbumSharePage = read(coupleAlbumSharePath)
for (const marker of [
  'api.getPublicCoupleAlbum',
  'sharePath',
  'shareQuery',
  'coverShareImage',
  "path: sharePath.value",
  "query: shareQuery.value",
  "imageUrl: coverShareImage.value",
  'share_token',
  'cover-panel',
  'summary-row',
  'album-preview-swiper',
  'page-grid',
  '下载相册',
  '保存单张图片',
  '保存长图',
  'album-poster-canvas',
  'saveImageToPhotosAlbum',
  'canvasToTempFilePath',
  'download_url || page?.preview_url',
  'ensureDownloadAuth',
  'requireAuth({ redirect: sharePath.value })',
  '链接无效或分享已关闭',
  '相册读取失败',
  'location',
  'caption'
]) {
  assert(coupleAlbumSharePage.includes(marker), `couple album share page missing marker: ${marker}`)
}
const coupleAlbumShareLoadStart = coupleAlbumSharePage.indexOf('async function loadSharedAlbum')
const coupleAlbumShareLoadEnd = coupleAlbumSharePage.indexOf('</script>', coupleAlbumShareLoadStart)
const coupleAlbumShareLoadBlock =
  coupleAlbumShareLoadStart >= 0 && coupleAlbumShareLoadEnd > coupleAlbumShareLoadStart
    ? coupleAlbumSharePage.slice(coupleAlbumShareLoadStart, coupleAlbumShareLoadEnd)
    : ''
assert(!coupleAlbumShareLoadBlock.includes('requireAuth'), 'couple album share page load must not require login')
assert(!coupleAlbumShareLoadBlock.includes('api.getMe'), 'couple album share page load must not load current user information')
const coupleAlbumShareSingleDownloadStart = coupleAlbumSharePage.indexOf('async function saveSingleAlbumImages')
const coupleAlbumSharePosterDownloadStart = coupleAlbumSharePage.indexOf('async function saveAlbumLongPoster')
const coupleAlbumShareOpenDownloadStart = coupleAlbumSharePage.indexOf('async function openDownloadPanel')
assert(
  coupleAlbumSharePage.indexOf('await ensureDownloadAuth()') > coupleAlbumShareOpenDownloadStart &&
    coupleAlbumSharePage.indexOf('await ensureDownloadAuth()', coupleAlbumShareSingleDownloadStart) > coupleAlbumShareSingleDownloadStart &&
    coupleAlbumSharePage.indexOf('await ensureDownloadAuth()', coupleAlbumSharePosterDownloadStart) > coupleAlbumSharePosterDownloadStart,
  'couple album share page downloads must require login only when the user starts a download action'
)
assert(!coupleAlbumSharePage.includes('api.retryCoupleAlbumPage'), 'couple album share page must not expose private failed-page retry API')
assert(!coupleAlbumSharePage.includes('function retryPage'), 'couple album share page must not expose private failed-page retry action')

const workSharePath = 'src/pages/works/share/index.vue'
assert(existsSync(resolve(root, workSharePath)), 'public works share page file must exist')
const workSharePage = read(workSharePath)
for (const marker of [
  'api.getPublicWorks',
  'shareIds',
  'sharePath',
  'shareQuery',
  'coverShareImage',
  "path: sharePath.value",
  "query: shareQuery.value",
  "imageUrl: coverShareImage.value",
  'shared-work-grid',
  '作品暂不可访问'
]) {
  assert(workSharePage.includes(marker), `public works share page missing marker: ${marker}`)
}
assert(!workSharePage.includes('requireAuth'), 'public works share page must not require login')
assert(!workSharePage.includes('api.getMe'), 'public works share page must not load current user information')

const legacyMiniProgramSharePath = 'src/utils/share.js'
assert(
  !existsSync(resolve(root, legacyMiniProgramSharePath)),
  'mini-program share helper must live in src/utils/routes.js, not src/utils/share.js'
)
const miniProgramShare = read('src/utils/routes.js')
for (const marker of [
  'enableMiniProgramShare',
  'onShareAppMessage',
  'onShareTimeline',
  'showShareMenu',
  "menus: ['shareAppMessage', 'shareTimeline']",
  "title: '白霖共享AI图片生成'",
  "path: '/pages/home/index'",
  "staticAsset('home-replica/mountain-hero.png')"
]) {
  assert(miniProgramShare.includes(marker), `shared mini-program share helper missing marker: ${marker}`)
}

const shareEnabledPageContracts = [
  ['src/pages/home/index.vue', homePage],
  ['src/pages/workspace/image-to-image/index.vue', workspace],
  [worksPagePath, worksPage],
  ['src/pages/pricing/index.vue', pricingPage],
  ['src/pages/support/index.vue', supportPage],
  [coupleAlbumDetailPath, coupleAlbumDetailPage],
  [coupleAlbumSharePath, coupleAlbumSharePage],
  [workSharePath, workSharePage]
]
for (const [pagePath, source] of shareEnabledPageContracts) {
  assert(
    source.includes('enableMiniProgramShare'),
    `${pagePath} must enable the native mini-program share menu`
  )
}
assert(homePage.includes('enableMiniProgramShare()'), 'home page must use the default share card')
assert(workspace.includes("title: '白霖共享 AI 生图工作台'"), 'workspace share title must describe the AI image workspace')
assert(workspace.includes('path: routes.imageToImage'), 'workspace share path must target the workspace page')
assert(pricingPage.includes("title: '白霖共享 AI 图片套餐'"), 'pricing share title must describe AI image packages')
assert(pricingPage.includes('path: routes.pricing'), 'pricing share path must target the pricing page')
assert(supportPage.includes("title: '白霖共享客服支持'"), 'support share title must describe help and customer service')
assert(supportPage.includes('path: routes.support'), 'support share path must target the support page')
for (const [pagePath, source] of shareEnabledPageContracts) {
  assert(!source.includes('utils/share.js'), `${pagePath} must import mini-program sharing from routes.js`)
}
for (const marker of [
  'sharePath',
  'shareQuery',
  'coverShareImage',
  "path: sharePath.value",
  "query: shareQuery.value",
  "imageUrl: coverShareImage.value"
]) {
  assert(coupleAlbumSharePage.includes(marker), `couple album public share page must preserve dynamic share marker: ${marker}`)
}
for (const marker of ['/pages/works/share/index', 'ids=', 'shareIds', 'query', 'imageUrl']) {
  assert(workSharePage.includes(marker), `public works share page must preserve ids share marker: ${marker}`)
}

for (const privatePagePath of [
  'src/pages/auth/index.vue',
  'src/pages/account/index.vue',
  'src/pages/account/transactions/index.vue',
  'src/pages/couple-album/create/index.vue'
]) {
  const privatePage = read(privatePagePath)
  assert(!privatePage.includes('enableMiniProgramShare'), `${privatePagePath} must not enable native mini-program sharing`)
}

const iconNames = [
  'logo-star.png',
  'history.png',
  'upload.png',
  'image.png',
  'prompt.png',
  'ratio.png',
  'style.png',
  'home.png',
  'workspace.png',
  'works.png',
  'pricing.png',
  'account.png',
  'more.png',
  'delete.png',
  'generate.png'
]

iconNames.forEach((name) => {
  assert(existsSync(resolve(root, `src/static/icons/${name}`)), `missing icon asset ${name}`)
})

console.log('mobile-h5 contract ok')
