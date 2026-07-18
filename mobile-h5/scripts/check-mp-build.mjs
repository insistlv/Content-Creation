import { existsSync, readFileSync, readdirSync, statSync } from 'node:fs'
import { extname, join, relative, resolve } from 'node:path'
import assert from 'node:assert/strict'
import { assertRequiredPageArtifacts } from './mp-build-contract.mjs'

const root = process.cwd()
const buildRoot = resolve(root, 'dist/build/mp-weixin')
const requiredLibVersion = '3.15.1'

assert(existsSync(buildRoot), 'mp-weixin build output must exist at dist/build/mp-weixin')

function filesUnder(dir) {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name)
    if (statSync(path).isDirectory()) return filesUnder(path)
    return [path]
  })
}

const jsFiles = filesUnder(buildRoot).filter((path) => path.endsWith('.js'))
assert(jsFiles.length > 0, 'mp-weixin build output must contain JavaScript files')
const outputFiles = filesUnder(buildRoot)

const read = (path) => readFileSync(path, 'utf8')
const combinedJS = jsFiles.map((path) => read(path)).join('\n')
const outputRelativePath = (path) => relative(buildRoot, path).replace(/\\/g, '/')
const findOutputPath = (matcher) => outputFiles.find((path) => matcher(outputRelativePath(path)))

const appConfig = JSON.parse(read(resolve(buildRoot, 'app.json')))
assert(Array.isArray(appConfig.pages), 'mp-weixin app.json must declare pages')

const parentProjectConfig = JSON.parse(read(resolve(root, 'project.config.json')))
assert.equal(
  parentProjectConfig.libVersion,
  requiredLibVersion,
  `source root project.config.json must pin libVersion ${requiredLibVersion}`
)
assert.equal(
  parentProjectConfig.setting?.urlCheck,
  false,
  'source root project.config.json must disable urlCheck for local debugging'
)
assert.equal(
  parentProjectConfig.setting?.minified,
  false,
  'source root project.config.json must disable JS minification for local DevTools debugging'
)
assert.equal(
  parentProjectConfig.setting?.minifyWXML,
  false,
  'source root project.config.json must disable WXML minification for local DevTools debugging'
)

const parentPrivateProjectConfig = JSON.parse(read(resolve(root, 'project.private.config.json')))
assert.equal(
  parentPrivateProjectConfig.libVersion,
  requiredLibVersion,
  `source root project.private.config.json must pin libVersion ${requiredLibVersion}`
)
assert.equal(
  parentPrivateProjectConfig.setting?.urlCheck,
  false,
  'source root project.private.config.json must disable urlCheck for local debugging'
)
assert.equal(
  parentPrivateProjectConfig.setting?.minified,
  false,
  'source root project.private.config.json must disable JS minification for local DevTools debugging'
)
assert.equal(
  parentPrivateProjectConfig.setting?.minifyWXML,
  false,
  'source root project.private.config.json must disable WXML minification for local DevTools debugging'
)

const distProjectConfigPath = resolve(buildRoot, 'project.config.json')
assert(
  existsSync(distProjectConfigPath),
  'mp-weixin dist root must include project.config.json for direct WeChat DevTools import'
)
const distProjectConfig = JSON.parse(read(distProjectConfigPath))
assert.equal(
  distProjectConfig.compileType,
  'miniprogram',
  'mp-weixin dist project.config.json must be importable as a miniprogram'
)
assert.equal(
  distProjectConfig.libVersion,
  requiredLibVersion,
  `mp-weixin dist project.config.json must pin libVersion ${requiredLibVersion}`
)
assert.equal(
  distProjectConfig.setting?.urlCheck,
  false,
  'mp-weixin dist project.config.json must disable urlCheck for local debugging'
)
assert.equal(
  distProjectConfig.setting?.minified,
  false,
  'mp-weixin dist project.config.json must disable JS minification for local DevTools debugging'
)
assert.equal(
  distProjectConfig.setting?.minifyWXML,
  false,
  'mp-weixin dist project.config.json must disable WXML minification for local DevTools debugging'
)
assert.equal(
  distProjectConfig.appid,
  parentProjectConfig.appid,
  'mp-weixin dist project.config.json must use the parent project appid'
)
assert(
  !('miniprogramRoot' in distProjectConfig),
  'mp-weixin dist project.config.json must not set miniprogramRoot when imported directly'
)

const distPrivateProjectConfigPath = resolve(buildRoot, 'project.private.config.json')
assert(
  existsSync(distPrivateProjectConfigPath),
  'mp-weixin dist root must include project.private.config.json for direct WeChat DevTools import'
)
const distPrivateProjectConfig = JSON.parse(read(distPrivateProjectConfigPath))
assert.equal(
  distPrivateProjectConfig.libVersion,
  requiredLibVersion,
  `mp-weixin dist project.private.config.json must pin libVersion ${requiredLibVersion}`
)
assert.equal(
  distPrivateProjectConfig.setting?.urlCheck,
  false,
  'mp-weixin dist project.private.config.json must disable urlCheck for local debugging'
)
assert.equal(
  distPrivateProjectConfig.setting?.compileHotReLoad,
  true,
  'mp-weixin dist project.private.config.json must enable compileHotReLoad for local debugging'
)
assert.equal(
  distPrivateProjectConfig.setting?.showShadowRootInWxmlPanel,
  true,
  'mp-weixin dist project.private.config.json must show shadow root in the WXML panel for local debugging'
)

const requiredPageArtifacts = ['.js', '.json', '.wxml', '.wxss']
const requiredCoupleAlbumPages = [
  'pages/couple-album/create/index',
  'pages/couple-album/detail/index',
  'pages/couple-album/share/index',
  'pages/works/share/index'
]
assertRequiredPageArtifacts({
  appConfig,
  buildRoot,
  pages: requiredCoupleAlbumPages,
  extensions: requiredPageArtifacts
})
for (const page of appConfig.pages) {
  for (const extension of requiredPageArtifacts) {
    assert(
      existsSync(resolve(buildRoot, `${page}${extension}`)),
      `mp-weixin page artifact missing: ${page}${extension}`
    )
  }
}

assert(
  appConfig.pages.includes('pages/support/index'),
  'mp-weixin app.json must register pages/support/index'
)
assert(
  existsSync(resolve(buildRoot, 'pages/support/index.wxml')),
  'mp-weixin support page WXML must exist at pages/support/index.wxml'
)

const appTabbarWXMLPath = findOutputPath((path) => /(^|\/)(app-tabbar|apptabbar)\.wxml$/i.test(path))
assert(appTabbarWXMLPath, 'mp-weixin output must include shared AppTabbar WXML')
const appTabbarJSPath = appTabbarWXMLPath.replace(/\.wxml$/, '.js')
const appTabbarWXSSPath = appTabbarWXMLPath.replace(/\.wxml$/, '.wxss')
assert(existsSync(appTabbarJSPath), 'mp-weixin output must include shared AppTabbar JS')
assert(existsSync(appTabbarWXSSPath), 'mp-weixin output must include shared AppTabbar WXSS')
const appTabbarWXML = read(appTabbarWXMLPath)
const appTabbarJS = read(appTabbarJSPath)
const appTabbarWXSS = read(appTabbarWXSSPath)
for (const label of ['工作台', '作品库', '套餐', '我的']) {
  assert(appTabbarWXML.includes(label), `mp-weixin AppTabbar WXML must render ${label}`)
}
assert((appTabbarWXML.match(/<image/g) || []).length === 4, 'mp-weixin AppTabbar WXML must render four icon images')
for (const marker of [
  'routes.imageToImage',
  'routes.works',
  'routes.pricing',
  'routes.account'
]) {
  assert(appTabbarJS.includes(marker), `mp-weixin AppTabbar JS must navigate through ${marker}`)
}
for (const marker of ['"home"', '"workspace"', '"pricing"', '"account"']) {
  assert(appTabbarJS.includes(marker), `mp-weixin AppTabbar JS must resolve icon key ${marker}`)
}
for (const style of [
  'position:fixed',
  'left:0',
  'right:0',
  'bottom:0',
  'safe-area-inset-bottom',
  'grid-template-columns:repeat(4,minmax(0,1fr))',
  'min-height:88rpx',
  '.app-tabbar__item.active'
]) {
  assert(appTabbarWXSS.includes(style), `mp-weixin AppTabbar WXSS must include ${style}`)
}

const homeWXSS = read(resolve(buildRoot, 'pages/home/index.wxss'))
assert(
  /\.campaign-link[^{]*\{[^}]*display:flex[^}]*align-items:center[^}]*justify-content:center/.test(homeWXSS),
  'mp-weixin home campaign CTA pills must keep centered flex layout'
)

const accountWXML = read(resolve(buildRoot, 'pages/account/index.wxml'))
for (const marker of ['profile-hero', 'transaction-empty']) {
  assert(
    accountWXML.includes(marker),
    `mp-weixin account page WXML must contain the new account page marker: ${marker}`
  )
}
for (const marker of ['phone-bind-strip', 'phone-bind-cta', '绑定手机号', '当前账号未绑定手机号']) {
  assert(accountWXML.includes(marker), `mp-weixin account page WXML must contain phone binding marker: ${marker}`)
}
for (const marker of ['phone-bind-modal', 'phone-quick-bind-button', 'sms-phone-bind-toggle', 'sms-phone-bind-form', 'getPhoneNumber', 'bindgetphonenumber']) {
  assert(accountWXML.includes(marker), `mp-weixin account page WXML must contain phone binding modal marker: ${marker}`)
}
for (const marker of ['placeholder="大陆手机号"', 'placeholder="短信验证码"', 'bindinput']) {
  assert(accountWXML.includes(marker), `mp-weixin account page WXML must contain SMS fallback input marker: ${marker}`)
}
for (const marker of ['手机号快捷绑定', '手机号验证中...', '使用短信验证码绑定', '收起短信验证码绑定', '获取验证码', '发送中']) {
  assert(accountWXML.includes(marker), `mp-weixin account page WXML must render static phone binding modal text: ${marker}`)
}
const accountJS = read(resolve(buildRoot, 'pages/account/index.js'))
for (const marker of ['bindAccountPhone({phone:', 'bindWechatPhone({code:', 'phone_code:', 'purpose:"bind_phone"', '手机号已绑定']) {
  assert(accountJS.includes(marker), `mp-weixin account page JS must contain phone binding behavior: ${marker}`)
}
const apiClientJS = read(resolve(buildRoot, 'api/client.js'))
assert(apiClientJS.includes('/api/account/wechat-phone'), 'mp-weixin API client must call current-account WeChat phone binding endpoint')
for (const staleMarker of ['profile-card', 'device-panel', 'help-card']) {
  assert(
    !accountWXML.includes(staleMarker),
    `mp-weixin account page WXML must not contain stale account page marker: ${staleMarker}`
  )
}

const authWXML = read(resolve(buildRoot, 'pages/auth/index.wxml'))
const authJS = read(resolve(buildRoot, 'pages/auth/index.js'))
for (const marker of ['phone-quick-auth-button', 'phone-quick-auth-divider', '其他方式']) {
  assert(authWXML.includes(marker), `mp-weixin auth page WXML must contain phone quick login marker: ${marker}`)
}
for (const marker of ['getPhoneNumber', 'bindgetphonenumber']) {
  assert(authWXML.includes(marker), `mp-weixin auth page WXML must contain phone quick login marker: ${marker}`)
}
assert(!authWXML.includes('wechat-phone-auth-button'), 'mp-weixin auth page WXML must not render a second WeChat phone login button')
assert((authWXML.match(/phone-quick-auth-button/g) || []).length === 1, 'mp-weixin auth page WXML must render one phone quick auth button')
assert(authJS.includes('login({provider:"weixin"'), 'mp-weixin auth page JS must still call wx login before phone login')
assert(authJS.includes('bindPhone=1'), 'mp-weixin auth page JS must redirect legacy no-phone accounts to phone binding')
assert(!authJS.includes('wechatLogin({code:'), 'mp-weixin auth page JS must not keep separate ordinary WeChat login behavior')
for (const marker of ['wechatPhoneLogin({code:', 'phone_code:', '登录成功', '手机号快捷登录']) {
  assert(authJS.includes(marker), `mp-weixin auth page JS must contain phone quick login behavior: ${marker}`)
}

for (const [outputName, output] of [
  ['auth WXML', authWXML],
  ['auth JS', authJS],
  ['account WXML', accountWXML],
  ['account JS', accountJS]
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
    assert(!output.includes(forbidden), `mp-weixin ${outputName} must not expose old WeChat-branded phone auth copy/class: ${forbidden}`)
  }
}

const workspaceWXML = read(resolve(buildRoot, 'pages/workspace/image-to-image/index.wxml'))
const workspaceJS = read(resolve(buildRoot, 'pages/workspace/image-to-image/index.js'))
const workspaceWXSS = read(resolve(buildRoot, 'pages/workspace/image-to-image/index.wxss'))
for (const marker of ['白霖共享', '工作模式', '文生图', '图生图', '情侣相册', '上传图片', '提示词', '反向提示词', '风格偏好', '图片尺寸', '画质', '创意程度']) {
  assert(workspaceWXML.includes(marker), `mp-weixin workspace WXML must contain text-to-image UI marker: ${marker}`)
}
assert(workspaceWXML.includes('上传双人照片，生成可分享的旅行相册'), 'mp-weixin workspace WXML must contain the couple album mode card description')
assert(workspaceJS.includes('routes.coupleAlbumCreate'), 'mp-weixin workspace JS must navigate the couple album mode card through the route helper')
assert(workspaceJS.includes('navigateTo'), 'mp-weixin workspace JS must keep shared navigation for the couple album mode card')
assert(workspaceJS.includes('"favorite"'), 'mp-weixin workspace JS must resolve the favorite icon for the couple album mode card')
assert(!workspaceWXML.includes('生成数量'), 'mp-weixin workspace WXML must not render the generation quantity title')
assert(!workspaceWXML.includes('{{item}}张'), 'mp-weixin workspace WXML must not render dynamic generation quantity buttons')
for (const marker of ['topbar', 'prompt-card', 'reference-panel', 'size-card', 'generation-settings-grid']) {
  assert(workspaceWXML.includes(marker), `mp-weixin workspace WXML must contain screenshot layout marker: ${marker}`)
}
for (const marker of ['prompt-mode-picker', 'prompt-optimize-button']) {
  assert(workspaceWXML.includes(marker), `mp-weixin workspace WXML must contain inline prompt optimizer marker: ${marker}`)
}
for (const marker of ['AI优化', '人脸高清', '提示词已优化', 'api.optimizePrompt']) {
  assert(workspaceJS.includes(marker), `mp-weixin workspace JS must contain inline prompt optimizer behavior: ${marker}`)
}
for (const marker of ['提示词模板库', 'template-card', 'template-use-button']) {
  assert(workspaceWXML.includes(marker), `mp-weixin workspace WXML must contain prompt template library marker: ${marker}`)
}
const promptTemplateWXMLBlock = workspaceWXML.match(/class="template-card[\s\S]*?template-use-button/)?.[0] || ''
assert(workspaceJS.includes('previewImage'), 'mp-weixin workspace JS must use the native template preview API')
assert(
  /<image[^>]+catchtap="\{\{item\.[^"]+\}\}"/.test(promptTemplateWXMLBlock),
  'mp-weixin workspace template preview image must expose a stop-tap preview binding'
)
assert(
  /template-ratio-badge[^>]+catchtap="\{\{item\.[^"]+\}\}"/.test(promptTemplateWXMLBlock),
  'mp-weixin workspace template ratio badge must expose a stop-tap preview binding'
)
assert(!workspaceWXML.includes('template-category-row'), 'mp-weixin workspace WXML must not render the cramped prompt template category row')
assert(!workspaceJS.includes('selectedTemplateCategory'), 'mp-weixin workspace JS must not keep prompt template category state')
assert(!workspaceJS.includes('visibleTemplateItems'), 'mp-weixin workspace JS must render backend template list directly')
assert(!workspaceJS.includes('selectTemplateCategory'), 'mp-weixin workspace JS must not keep prompt template category handlers')
for (const marker of ['api.listPromptTemplates', 'api.usePromptTemplate', '使用 1 点', '已使用模板，扣除 1 点', '点数不足，无法使用模板']) {
  assert(workspaceJS.includes(marker), `mp-weixin workspace JS must contain prompt template usage behavior: ${marker}`)
}
assert(!workspaceWXML.includes('prompt-optimizer-backdrop'), 'mp-weixin workspace WXML must not render prompt optimizer bottom overlay')
assert(!workspaceJS.includes('showPromptOptimizer'), 'mp-weixin workspace JS must not keep prompt optimizer bottom overlay state')
for (const marker of ['朋友圈/电商图', '1024×1024', '768×1024', '576×1024', 'quality', 'high']) {
  assert(workspaceJS.includes(marker), `mp-weixin workspace JS must contain size preset: ${marker}`)
}
for (const marker of ['参考图片', 'chooseImage', 'reference_asset_ids', '请至少上传1张参考图']) {
  assert(workspaceJS.includes(marker), `mp-weixin workspace JS must contain image-to-image behavior: ${marker}`)
}
assert(apiClientJS.includes('uploadFile('), 'mp-weixin API client must keep reference image upload behavior')
assert(apiClientJS.includes('/api/reference-assets/upload-policy'), 'mp-weixin API client must request an OSS upload policy before uploading')
assert(apiClientJS.includes('/api/reference-assets/complete-upload'), 'mp-weixin API client must complete reference uploads after OSS upload')
assert(/url:\w+\.upload_url/.test(apiClientJS), 'mp-weixin upload API must send selected images directly to OSS')
assert(apiClientJS.includes('.filePath='), 'mp-weixin upload API must pass filePath to wx.uploadFile')
assert(!apiClientJS.includes('.file='), 'mp-weixin upload API must not pass H5 File objects to wx.uploadFile')
assert(apiClientJS.includes('upload_file_missing'), 'mp-weixin upload API must reject missing selected image paths before upload')
assert(workspaceWXML.includes('wx:if="{{') && workspaceWXML.includes('reference-panel'), 'mp-weixin workspace WXML must keep the conditional reference panel')
assert(!workspaceWXML.includes('ratio-card'), 'mp-weixin workspace WXML must not render old ratio cards')
assert(!workspaceWXSS.includes('ratio-card'), 'mp-weixin workspace WXSS must not include old ratio-card styles')
assert(!workspaceWXML.includes('size-picker-value'), 'mp-weixin workspace WXML must not render the old collapsed size picker')
const workspaceSizePickerWXMLBlock = workspaceWXML.match(/<picker[^>]*mode="selector"[\s\S]*?size-picker-trigger[\s\S]*?<\/picker>/)?.[0] || ''
assert(workspaceSizePickerWXMLBlock.includes('range="{{'), 'mp-weixin workspace size picker must receive a selector range')
assert(workspaceSizePickerWXMLBlock.includes('value="{{'), 'mp-weixin workspace size picker must bind the selected preset index')
assert(workspaceSizePickerWXMLBlock.includes('disabled="{{'), 'mp-weixin workspace size picker must disable while submitting')
assert(workspaceSizePickerWXMLBlock.includes('bindchange="{{'), 'mp-weixin workspace size picker must handle picker changes')
assert(workspaceSizePickerWXMLBlock.includes('size-head'), 'mp-weixin workspace size picker must wrap the size header row')
assert(workspaceSizePickerWXMLBlock.includes('图片尺寸'), 'mp-weixin workspace size picker must expose the image size title')
assert(workspaceSizePickerWXMLBlock.includes('size-picker-arrow'), 'mp-weixin workspace size picker must include the dropdown arrow')
for (const marker of ['创意程度', 'variation-row']) {
  assert(workspaceWXML.includes(marker), `mp-weixin workspace WXML must contain creativity mode marker: ${marker}`)
}
for (const marker of ['任务已提交，正在生成', 'addPendingGenerations', 'variation_mode', 'variation_prompt', '均衡变化', '正面构图，主体居中，干净商业摄影风格', 'setInterval']) {
  assert(workspaceJS.includes(marker), `mp-weixin workspace JS must contain single generation behavior: ${marker}`)
}
for (const marker of ['batch-', 'var-', '.map(e=>e.generation_id)']) {
  assert(!workspaceJS.includes(marker), `mp-weixin workspace JS must not contain batch generation marker: ${marker}`)
}
assert(
  workspaceJS.includes('"image"!==') && workspaceJS.includes('reference_preview_urls:'),
  'mp-weixin workspace JS must keep reference previews out of text-to-image pending tasks'
)
assert(
  !workspaceJS.includes('waitForGenerationTask') && !workspaceJS.includes('张已完成'),
  'mp-weixin workspace JS must not wait for each selected image before creating the next task'
)

const worksWXML = read(resolve(buildRoot, 'pages/works/index.wxml'))
const worksJS = read(resolve(buildRoot, 'pages/works/index.js'))
const worksWXSS = read(resolve(buildRoot, 'pages/works/index.wxss'))
const homeJS = read(resolve(buildRoot, 'pages/home/index.js'))
const pricingJS = read(resolve(buildRoot, 'pages/pricing/index.js'))
const pricingWXML = read(resolve(buildRoot, 'pages/pricing/index.wxml'))
const supportJS = read(resolve(buildRoot, 'pages/support/index.js'))
const supportWXML = read(resolve(buildRoot, 'pages/support/index.wxml'))
const coupleAlbumCreateWXML = read(resolve(buildRoot, 'pages/couple-album/create/index.wxml'))
const coupleAlbumCreateJS = read(resolve(buildRoot, 'pages/couple-album/create/index.js'))
const coupleAlbumDetailWXML = read(resolve(buildRoot, 'pages/couple-album/detail/index.wxml'))
const coupleAlbumDetailJS = read(resolve(buildRoot, 'pages/couple-album/detail/index.js'))
const coupleAlbumShareWXML = read(resolve(buildRoot, 'pages/couple-album/share/index.wxml'))
const coupleAlbumShareJS = read(resolve(buildRoot, 'pages/couple-album/share/index.js'))
const workShareJS = read(resolve(buildRoot, 'pages/works/share/index.js'))
const appTabbarPageOutputs = [
  ['workspace', workspaceWXML, 'pages/workspace/image-to-image/index.json'],
  ['couple album create', coupleAlbumCreateWXML, 'pages/couple-album/create/index.json'],
  ['works', worksWXML, 'pages/works/index.json'],
  ['pricing', pricingWXML, 'pages/pricing/index.json'],
  ['account', accountWXML, 'pages/account/index.json'],
  ['support', supportWXML, 'pages/support/index.json']
]
for (const [name, wxml, jsonPath] of appTabbarPageOutputs) {
  const pageJSON = read(resolve(buildRoot, jsonPath))
  assert(wxml.includes('app-tabbar'), `mp-weixin ${name} page WXML must render the shared AppTabbar component`)
  assert(pageJSON.includes('app-tabbar'), `mp-weixin ${name} page JSON must register the shared AppTabbar component`)
}
for (const marker of ['工作模式', '情侣相册', '选择工作模式', '文生图', '图生图']) {
  assert(coupleAlbumCreateWXML.includes(marker), `mp-weixin couple album create WXML must contain work-mode marker: ${marker}`)
}
assert(coupleAlbumCreateJS.includes('开始生成相册'), 'mp-weixin couple album create JS must keep the generate button ready copy')
assert(coupleAlbumCreateJS.includes('routes.imageToImage'), 'mp-weixin couple album create JS must navigate text/image modes through the workspace route helper')
assert(coupleAlbumCreateJS.includes('mode:"text"') || coupleAlbumCreateJS.includes("mode:'text'"), 'mp-weixin couple album create JS must route text-to-image mode')
assert(coupleAlbumCreateJS.includes('mode:"image"') || coupleAlbumCreateJS.includes("mode:'image'"), 'mp-weixin couple album create JS must route image-to-image mode')
assert(!coupleAlbumCreateWXML.includes('app-tabbar__item') || (coupleAlbumCreateWXML.match(/app-tabbar__item/g) || []).length <= 4, 'mp-weixin couple album create page must not add a fifth bottom nav item')
for (const marker of ['batch-count-badge', 'work-preview-swiper', '下载当前']) {
  assert(worksWXML.includes(marker), `mp-weixin works WXML must contain batch preview marker: ${marker}`)
}
for (const marker of ['batch_id', 'batch_items', 'is_batch', 'fallback-', 'saveImageToPhotosAlbum']) {
  assert(worksJS.includes(marker), `mp-weixin works JS must contain batch grouping and download behavior: ${marker}`)
}
for (const marker of ['enableMiniProgramShare', '/api/public/works/', 'routes.workShare', '白霖共享 AI 作品库']) {
  assert(worksJS.includes(marker), `mp-weixin works JS must contain native work share behavior: ${marker}`)
}
assert(worksWXML.includes('open-type="share"'), 'mp-weixin works WXML must use native share buttons for work cards')
assert(worksWXML.includes('data-share-kind="work"'), 'mp-weixin works WXML must mark native work share targets')
assert(worksWXML.includes('data-share-kind="album"'), 'mp-weixin works WXML must mark native album share targets')
assert(worksWXML.includes('data-share-token='), 'mp-weixin works WXML must expose native album share tokens')
assert(worksWXML.includes('native-share-panel'), 'mp-weixin works WXML must include the post-publish native share panel')
for (const marker of ['share_token', 'routes.coupleAlbumShare', '白霖共享情侣相册']) {
  assert(worksJS.includes(marker), `mp-weixin works JS must contain native album share behavior: ${marker}`)
}
for (const marker of ['cover-placeholder', 'placeholder-orbit']) {
  assert(worksWXML.includes(marker), `mp-weixin works WXML must contain pending placeholder marker: ${marker}`)
}
for (const marker of ['reference_preview_urls:', 'tool_mode:', '文字转图片', '图片转图片', '生成中', '暂无预览']) {
  assert(worksJS.includes(marker), `mp-weixin works JS must preserve pending generation mode behavior: ${marker}`)
}
assert(
  worksJS.includes('reference_preview_urls:"image"===') || worksJS.includes('reference_preview_urls: "image"==='),
  'mp-weixin works JS must keep reference previews only on image-mode pending tasks'
)
assert(!worksJS.includes('mode:"image",tool_mode:"image"'), 'mp-weixin works JS must not force pending text-to-image tasks into image mode')
assert(worksWXSS.includes('cover-placeholder'), 'mp-weixin works WXSS must style the pending placeholder cover')

for (const [name, output] of [
  ['home', homeJS],
  ['workspace', workspaceJS],
  ['works', worksJS],
  ['pricing', pricingJS],
  ['support', supportJS],
  ['couple album detail', coupleAlbumDetailJS],
  ['couple album public share', coupleAlbumShareJS],
  ['public works share', workShareJS]
]) {
  assert(output.includes('enableMiniProgramShare'), `mp-weixin ${name} page JS must enable native sharing`)
  assert(!output.includes('utils/share.js'), `mp-weixin ${name} page JS must not require utils/share.js`)
}
const routesHelperJS = read(resolve(buildRoot, 'utils/routes.js'))
for (const marker of ['onShareAppMessage', 'onShareTimeline', 'showShareMenu', 'shareAppMessage', 'shareTimeline']) {
  assert(routesHelperJS.includes(marker), `mp-weixin routes helper JS must contain ${marker}`)
}
const legacyShareHelperPath = resolve(buildRoot, 'utils/share.js')
assert(existsSync(legacyShareHelperPath), 'mp-weixin output must include a utils/share.js compatibility shim for stale DevTools caches')
const legacyShareHelperJS = read(legacyShareHelperPath)
assert(
  legacyShareHelperJS.includes('require("./routes.js")'),
  'mp-weixin utils/share.js compatibility shim must forward to utils/routes.js'
)
assert(routesHelperJS.includes('/pages/home/index'), 'mp-weixin default share helper JS must target the home page')
assert(workspaceJS.includes('白霖共享 AI 生图工作台'), 'mp-weixin workspace share JS must keep the workspace share title')
assert(workspaceJS.includes('routes.imageToImage'), 'mp-weixin workspace share JS must target the workspace page')
assert(pricingJS.includes('白霖共享 AI 图片套餐'), 'mp-weixin pricing share JS must keep the pricing share title')
assert(pricingJS.includes('routes.pricing'), 'mp-weixin pricing share JS must target the pricing page')
assert(supportJS.includes('白霖共享客服支持'), 'mp-weixin support share JS must keep the support share title')
assert(supportJS.includes('routes.support'), 'mp-weixin support share JS must target the support page')
for (const marker of ['routes.coupleAlbumShare', 'token=', 'share_token', 'query', 'imageUrl']) {
  assert(coupleAlbumShareJS.includes(marker), `mp-weixin couple album public share JS must keep token share marker: ${marker}`)
}
for (const marker of ['routes.coupleAlbumShare', 'token=', 'share_token', 'query', 'imageUrl']) {
  assert(coupleAlbumDetailJS.includes(marker), `mp-weixin couple album detail JS must keep token share marker: ${marker}`)
}
for (const marker of ['open-type="share"', 'data-share-kind="album"', 'data-share-token=', 'native-share-panel']) {
  assert(coupleAlbumDetailWXML.includes(marker), `mp-weixin couple album detail WXML must contain native album share marker: ${marker}`)
}
for (const marker of ['下载相册', '保存单张图片', '保存长图', 'album-poster-canvas']) {
  assert(coupleAlbumDetailWXML.includes(marker), `mp-weixin couple album detail WXML must contain album download marker: ${marker}`)
}
for (const marker of ['saveImageToPhotosAlbum', 'canvasToTempFilePath', 'download_url', 'preview_url']) {
  assert(coupleAlbumDetailJS.includes(marker), `mp-weixin couple album detail JS must contain album download behavior: ${marker}`)
}
for (const marker of ['cover-panel', 'summary-row', 'album-preview-swiper', 'page-grid']) {
  assert(coupleAlbumShareWXML.includes(marker), `mp-weixin couple album public share WXML must contain detail layout marker: ${marker}`)
}
for (const marker of ['下载相册', '保存单张图片', '保存长图', 'album-poster-canvas']) {
  assert(coupleAlbumShareWXML.includes(marker), `mp-weixin couple album public share WXML must contain album download marker: ${marker}`)
}
for (const marker of ['saveImageToPhotosAlbum', 'canvasToTempFilePath', 'download_url', 'preview_url']) {
  assert(coupleAlbumShareJS.includes(marker), `mp-weixin couple album public share JS must contain album download behavior: ${marker}`)
}
for (const marker of ['routes.workShare', 'ids=', 'getPublicWorks', 'query', 'imageUrl']) {
  assert(workShareJS.includes(marker), `mp-weixin public works share JS must keep ids share marker: ${marker}`)
}
assert(routesHelperJS.includes('/pages/works/share/index'), 'mp-weixin routes helper JS must register the public works share page')
for (const [name, output] of [
  ['auth', authJS],
  ['account', accountJS],
  ['couple album create', coupleAlbumCreateJS]
]) {
  assert(!output.includes('enableMiniProgramShare'), `mp-weixin ${name} page JS must not enable native sharing`)
  assert(!output.includes('showShareMenu'), `mp-weixin ${name} page JS must not open the native share menu`)
}

const forbiddenCapsuleMarkers = ['wechat-capsule', 'mini-program-capsule']
const capsuleOutputFiles = outputFiles.filter((path) => ['.js', '.wxml', '.wxss'].includes(extname(path)))
for (const file of capsuleOutputFiles) {
  const content = read(file)
  for (const marker of forbiddenCapsuleMarkers) {
    assert(
      !content.includes(marker),
      `mp-weixin output must not contain fake mini-program capsule marker ${marker}: ${relative(buildRoot, file)}`
    )
  }
}

assert(!combinedJS.includes('URLSearchParams'), 'mp-weixin build output must not contain URLSearchParams')
assert(combinedJS.includes('IMAGE_AGENT_MP_BUILD'), 'mp-weixin app output must log the build marker')
assert(combinedJS.includes('no-urlsearchparams-v2'), 'mp-weixin app output must identify the URLSearchParams-free build')

const routesOutput = read(resolve(buildRoot, 'utils/routes.js'))
assert(routesOutput.includes('redirectTo'), 'mp-weixin route output must use redirectTo for auth redirects')
assert(routesOutput.includes('reLaunch'), 'mp-weixin route output must fall back to reLaunch for auth redirects')
assert(routesOutput.includes('/pages/auth/index'), 'mp-weixin route output must include the auth page path')
assert(routesOutput.includes('navigateTo'), 'mp-weixin route output must still support parameterized navigation')
assert(routesOutput.indexOf('redirectTo') < routesOutput.lastIndexOf('navigateTo') || routesOutput.includes('redirectTo'), 'mp-weixin main shell route output must include redirectTo to avoid page stack growth')

const apiOutput = read(resolve(buildRoot, 'api/client.js'))
assert(apiOutput.includes('timeout'), 'mp-weixin API output must include request timeout settings')
assert(apiOutput.includes('https://example.com'), 'mp-weixin API output must mention the production domain backend default in diagnostics')
assert(apiOutput.includes('Authorization'), 'mp-weixin API output must inject Authorization for bearer auth')
assert(apiOutput.includes('auth_token'), 'mp-weixin API output must handle auth_token from login responses')
assert(apiOutput.includes('getStorageSync'), 'mp-weixin API output must read auth token from uni storage')
assert(apiOutput.includes('setStorageSync'), 'mp-weixin API output must persist auth token to uni storage')
assert(apiOutput.includes('removeStorageSync'), 'mp-weixin API output must clear auth token from uni storage')

const commonAssetsPath = resolve(buildRoot, 'common/assets.js')
assert(existsSync(commonAssetsPath), 'mp-weixin output must include common/assets.js for WeChat DevTools cache compatibility')
assert(existsSync(resolve(buildRoot, 'utils/static-assets.js')), 'mp-weixin output must include utils/static-assets.js for stale WeChat DevTools cache compatibility')
assert(existsSync(resolve(buildRoot, 'utils/asset-urls.js')), 'mp-weixin output must include utils/asset-urls.js for stale WeChat DevTools cache compatibility')

const maxMainPackageBytes = 1900 * 1024
const packageBytes = outputFiles.reduce((sum, path) => sum + statSync(path).size, 0)
assert(
  packageBytes < maxMainPackageBytes,
  `mp-weixin main package must stay below 1900KB, got ${Math.ceil(packageBytes / 1024)}KB`
)

const packagedStaticPNGs = outputFiles
  .filter((path) => extname(path).toLowerCase() === '.png')
  .filter((path) => relative(buildRoot, path).split(/[\\/]/)[0] === 'static')
assert.deepEqual(packagedStaticPNGs, [], 'mp-weixin build output must not package local static PNG assets')

assert(!combinedJS.includes('"/static/'), 'mp-weixin output must not reference local /static assets')
assert(!combinedJS.includes("'/static/"), 'mp-weixin output must not reference local /static assets')
assert(!combinedJS.includes('static-assets.js'), 'mp-weixin output must not require static-assets.js')
assert(!combinedJS.includes('asset-urls.js'), 'mp-weixin output must not require asset-urls.js')

console.log('mp-weixin build contract ok')
