import { existsSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const root = process.cwd()
const requiredFiles = [
  'src/main.js',
  'src/App.vue',
  'src/pages.json',
  'src/manifest.json',
  'project.config.json',
  'src/pages/home/index.vue',
  'src/pages/workspace/image-to-image/index.vue',
  'src/pages/works/index.vue',
  'src/pages/pricing/index.vue',
  'src/pages/support/index.vue',
  'src/pages/account/index.vue',
  'src/api/client.js',
  'src/static/icons/logo-star.png',
  'src/styles/base.scss',
  'src/styles/tokens.scss'
]

const missing = requiredFiles.filter((file) => !existsSync(resolve(root, file)))
if (missing.length > 0) {
  console.error(`Missing files: ${missing.join(', ')}`)
  process.exit(1)
}

const pages = JSON.parse(readFileSync(resolve(root, 'src/pages.json'), 'utf8'))
if (pages.pages?.[0]?.path !== 'pages/home/index') {
  console.error('Home page must be the first registered page')
  process.exit(1)
}

const pagePaths = pages.pages.map((page) => page.path)
if (!pagePaths.includes('pages/workspace/image-to-image/index')) {
  console.error('Image-to-image workspace page must be registered')
  process.exit(1)
}

if (!pagePaths.includes('pages/works/index')) {
  console.error('Works history page must be registered')
  process.exit(1)
}

if (!pagePaths.includes('pages/pricing/index')) {
  console.error('Pricing page must be registered')
  process.exit(1)
}

if (!pagePaths.includes('pages/support/index')) {
  console.error('Support page must be registered')
  process.exit(1)
}

if (!pagePaths.includes('pages/account/index')) {
  console.error('Account profile page must be registered')
  process.exit(1)
}

const projectConfig = JSON.parse(readFileSync(resolve(root, 'project.config.json'), 'utf8'))
const expectedWeChatLibVersion = '3.15.1'
if (projectConfig.miniprogramRoot !== 'dist/build/mp-weixin') {
  console.error('WeChat devtools project must point miniprogramRoot to dist/build/mp-weixin')
  process.exit(1)
}

if (projectConfig.libVersion !== expectedWeChatLibVersion) {
  console.error(`WeChat devtools project.config.json must pin libVersion to ${expectedWeChatLibVersion}`)
  process.exit(1)
}

if (projectConfig.setting?.urlCheck !== false) {
  console.error('WeChat devtools project.config.json must disable setting.urlCheck for local development')
  process.exit(1)
}

const ignoredStaticFolder = projectConfig.packOptions?.ignore?.some(
  (item) => item.type === 'folder' && item.value === 'static'
)
if (!ignoredStaticFolder) {
  console.error('WeChat devtools packOptions must ignore the generated static folder')
  process.exit(1)
}

const privateProjectConfig = JSON.parse(readFileSync(resolve(root, 'project.private.config.json'), 'utf8'))
if (privateProjectConfig.libVersion !== expectedWeChatLibVersion) {
  console.error(`WeChat devtools project.private.config.json must pin libVersion to ${expectedWeChatLibVersion}`)
  process.exit(1)
}

if (privateProjectConfig.setting?.urlCheck !== false) {
  console.error('WeChat devtools project.private.config.json must disable setting.urlCheck for local development')
  process.exit(1)
}

const authPage = readFileSync(resolve(root, 'src/pages/auth/index.vue'), 'utf8')
const accountPage = readFileSync(resolve(root, 'src/pages/account/index.vue'), 'utf8')
const workspacePage = readFileSync(resolve(root, 'src/pages/workspace/image-to-image/index.vue'), 'utf8')
const styleTokens = readFileSync(resolve(root, 'src/styles/tokens.scss'), 'utf8')
const baseStyles = readFileSync(resolve(root, 'src/styles/base.scss'), 'utf8')

if (!styleTokens.includes('$phone-float-clearance-top: 160rpx;')) {
  console.error('Mobile styles must define the 160rpx phone float top clearance token')
  process.exit(1)
}

const safePageStyle = baseStyles.match(/\.safe-page\s*\{[\s\S]*?\n\}/)?.[0] || ''
if (!safePageStyle.includes('padding-top: calc($phone-float-clearance-top + env(safe-area-inset-top));')) {
  console.error('Shared safe-page padding must include phone float top clearance')
  process.exit(1)
}

const shellTopClearanceContracts = [
  ['src/pages/auth/index.vue', '.auth-shell', 'padding: calc(20rpx + $phone-float-clearance-top + env(safe-area-inset-top)) 28rpx 34rpx;'],
  ['src/pages/account/index.vue', '.app-shell', 'padding: calc(18rpx + $phone-float-clearance-top + env(safe-area-inset-top)) 16rpx 0;'],
  ['src/pages/pricing/index.vue', '.app-shell', 'padding: calc(34rpx + $phone-float-clearance-top + env(safe-area-inset-top)) 34rpx 0;'],
  ['src/pages/works/index.vue', '.app-shell', 'padding: calc(36rpx + $phone-float-clearance-top + env(safe-area-inset-top)) 34rpx 0;'],
  [
    'src/pages/workspace/image-to-image/index.vue',
    '.app-shell',
    'padding: calc(26rpx + $phone-float-clearance-top + env(safe-area-inset-top)) 28rpx 0;'
  ]
]

for (const [file, selector, expectedPadding] of shellTopClearanceContracts) {
  const source = readFileSync(resolve(root, file), 'utf8')
  const escapedSelector = selector.replace('.', '\\.')
  const shellStyle = source.match(new RegExp(`${escapedSelector}\\s*\\{[\\s\\S]*?\\n\\}`))?.[0] || ''
  if (!shellStyle.includes(expectedPadding)) {
    console.error(`${file} ${selector} padding must include phone float top clearance`)
    process.exit(1)
  }
}

const forbiddenAuthSnippets = [
  'class="auth-heading"',
  'const title = computed',
  'const subtitle = computed',
  '.auth-heading'
]
const presentForbiddenAuthSnippets = forbiddenAuthSnippets.filter((snippet) => authPage.includes(snippet))
if (presentForbiddenAuthSnippets.length > 0) {
  console.error(`Auth page must not render form heading/subtitle: ${presentForbiddenAuthSnippets.join(', ')}`)
  process.exit(1)
}

const requiredPhoneQuickAuthSnippets = [
  'function uniLogin',
  'uni.login({',
  "provider: 'weixin'",
  'function submitWechatPhoneLogin',
  'api.wechatPhoneLogin({ code, phone_code: phoneCode })',
  '手机号快捷登录',
  '手机号验证中...',
  'phone-quick-auth-button',
  'phone-quick-auth-divider'
]
const missingPhoneQuickAuthSnippets = requiredPhoneQuickAuthSnippets.filter((snippet) => !authPage.includes(snippet))
if (missingPhoneQuickAuthSnippets.length > 0) {
  console.error(`Auth page phone quick login missing: ${missingPhoneQuickAuthSnippets.join(', ')}`)
  process.exit(1)
}

const forbiddenPhoneQuickAuthSnippets = [
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
]
const phoneQuickSources = `${authPage}\n${accountPage}`
const presentForbiddenPhoneQuickSnippets = forbiddenPhoneQuickAuthSnippets.filter((snippet) =>
  phoneQuickSources.includes(snippet)
)
if (presentForbiddenPhoneQuickSnippets.length > 0) {
  console.error(`Phone quick auth visible copy/classes must be neutral: ${presentForbiddenPhoneQuickSnippets.join(', ')}`)
  process.exit(1)
}

const codeButtonUses = authPage.match(/class="code-button"/g) || []
if (codeButtonUses.length !== 2) {
  console.error('Auth register and reset SMS buttons must use the shared code-button class')
  process.exit(1)
}

const codeButtonStyle = authPage.match(/\.code-button\s*\{[\s\S]*?\n\}/)?.[0] || ''
const requiredCodeButtonStyle = [
  'display: flex;',
  'align-items: center;',
  'justify-content: center;',
  'line-height: 1;',
  'padding: 0;',
  'white-space: nowrap;',
  'background: linear-gradient'
]
const missingCodeButtonStyle = requiredCodeButtonStyle.filter((snippet) => !codeButtonStyle.includes(snippet))
if (missingCodeButtonStyle.length > 0) {
  console.error(`Auth SMS code button style missing: ${missingCodeButtonStyle.join(', ')}`)
  process.exit(1)
}

const primaryButtonStyle = authPage.match(/\.primary-button\s*\{[\s\S]*?\n\}/)?.[0] || ''
const requiredPrimaryButtonStyle = [
  'background: linear-gradient(100deg, #2563ff 0%, #7c3aed 100%);',
  'box-shadow: 0 16rpx 34rpx rgba(79, 70, 229, 0.28);'
]
const missingPrimaryButtonStyle = requiredPrimaryButtonStyle.filter((snippet) => !primaryButtonStyle.includes(snippet))
if (missingPrimaryButtonStyle.length > 0) {
  console.error(`Auth primary button style missing: ${missingPrimaryButtonStyle.join(', ')}`)
  process.exit(1)
}

const bottomEntryStyle = authPage.match(/\.bottom-entry\s*\{[\s\S]*?\n\}/)?.[0] || ''
const requiredBottomEntryStyle = [
  'justify-content: space-between;',
  'width: 100%;'
]
const missingBottomEntryStyle = requiredBottomEntryStyle.filter((snippet) => !bottomEntryStyle.includes(snippet))
if (missingBottomEntryStyle.length > 0) {
  console.error(`Auth bottom entry style missing: ${missingBottomEntryStyle.join(', ')}`)
  process.exit(1)
}

if (authPage.includes('.primary-button,\n.entry-link')) {
  console.error('Auth entry link must not share the primary-button full-width style')
  process.exit(1)
}

const requiredWorkspaceSizePickerSnippets = [
  'const sizePresets = [',
  'const selectedSizePresetIndex = ref(0)',
  'function selectSizePreset',
  'function setAspectRatioFromPresetValue',
  '图片尺寸',
  'size-card',
  'size-card-row',
  '朋友圈/电商图',
  '1024×1024',
  '576×1024'
]
const missingWorkspaceSizePickerSnippets = requiredWorkspaceSizePickerSnippets.filter(
  (snippet) => !workspacePage.includes(snippet)
)
if (missingWorkspaceSizePickerSnippets.length > 0) {
  console.error(`Workspace size picker missing: ${missingWorkspaceSizePickerSnippets.join(', ')}`)
  process.exit(1)
}

if (workspacePage.includes('class="ratio-card"') || workspacePage.includes('class="ratio-row"')) {
  console.error('Workspace picture size setting must use the direct text-to-image size cards instead of old ratio cards')
  process.exit(1)
}

const requiredWorkspaceDualModeSnippets = [
  'class="topbar"',
  '白霖共享',
  "label: '文生图'",
  "label: '图生图'",
  'function chooseReferences',
  'v-if="activeMode === \'image\'" class="reference-panel"',
  '上传图片',
  '请至少上传1张参考图',
  "activeMode.value === 'image'",
  'hasUploadedReference',
  'delete requestPayload.reference_asset_ids'
]
const missingWorkspaceDualModeSnippets = requiredWorkspaceDualModeSnippets.filter((snippet) =>
  !workspacePage.includes(snippet)
)
if (missingWorkspaceDualModeSnippets.length > 0) {
  console.error(`Workspace dual text/image mode missing: ${missingWorkspaceDualModeSnippets.join(', ')}`)
  process.exit(1)
}

const requiredWorkspaceSingleGenerationSnippets = [
  'async function createSingleGeneration',
  'const singleGenerationCreditCost = 1',
  'const estimatedGenerationCredits = computed(() => singleGenerationCreditCost + uploadedReferenceCreditCost.value)',
  'variation_mode: variationMode.value',
  'variation_prompt: variationPromptForIndex(0, variationMode.value)',
  'const pendingTask = buildPendingGenerationTask(created, requestPayload, new Date().toISOString())',
  'addPendingGenerations([pendingTask])',
  'taskMessage.value = \'任务已提交，正在生成\'',
  'startPolling([created.generation_id])'
]
const missingWorkspaceSingleGenerationSnippets = requiredWorkspaceSingleGenerationSnippets.filter(
  (snippet) => !workspacePage.includes(snippet)
)
if (missingWorkspaceSingleGenerationSnippets.length > 0) {
  console.error(`Workspace single generation missing: ${missingWorkspaceSingleGenerationSnippets.join(', ')}`)
  process.exit(1)
}

const forbiddenWorkspaceBatchGenerationSnippets = [
  'const generationCount',
  'function setCount',
  'function createGenerationBatchID',
  'function createVariationSeed',
  'const createdTasks = []',
  'createdTasks.push({',
  'for (let index = 0; index < total; index += 1)',
  'batch_id: batchID',
  'batch_index: index',
  'batch_total: total',
  'seed: createVariationSeed',
  '<text class="section-title">生成数量</text>',
  'generationCount === item',
  '@click="setCount(item)"',
  '生成 ${generationCount.value} 张',
  'async function runGenerationQueue',
  'async function waitForGenerationTask',
  '第 ${index + 1} / ${total} 张已完成'
]
const presentForbiddenWorkspaceBatchGenerationSnippets = forbiddenWorkspaceBatchGenerationSnippets.filter(
  (snippet) => workspacePage.includes(snippet)
)
if (presentForbiddenWorkspaceBatchGenerationSnippets.length > 0) {
  console.error(
    `Workspace must not expose mobile batch generation controls or submit multiple tasks: ${presentForbiddenWorkspaceBatchGenerationSnippets.join(', ')}`
  )
  process.exit(1)
}

const worksPage = readFileSync(resolve(root, 'src/pages/works/index.vue'), 'utf8')
const requiredWorksBatchPreviewSnippets = [
  'const groupedDisplayWorks = computed',
  'function groupWorksByBatch',
  'function mergeIncompleteBatchGroups',
  'function fallbackBatchGroupKey',
  'const fallbackLimit = expectedTotal > 1 ? expectedTotal : 4',
  'function canFallbackBatchGroup',
  'batch_items',
  'function isBatchWork',
  '@click="previewWork(work)"',
  'previewOverlayVisible',
  '<swiper',
  '下载当前',
  'saveImageToPhotosAlbum'
]
const missingWorksBatchPreviewSnippets = requiredWorksBatchPreviewSnippets.filter(
  (snippet) => !worksPage.includes(snippet)
)
if (missingWorksBatchPreviewSnippets.length > 0) {
  console.error(`Works batch preview missing: ${missingWorksBatchPreviewSnippets.join(', ')}`)
  process.exit(1)
}

const staticAssetConsumers = [
  'src/pages/home/index.vue',
  'src/pages/auth/index.vue',
  'src/pages/workspace/image-to-image/index.vue',
  'src/pages/works/index.vue',
  'src/pages/pricing/index.vue',
  'src/pages/support/index.vue',
  'src/pages/account/index.vue'
]

const localStaticReferences = []
for (const file of staticAssetConsumers) {
  const source = readFileSync(resolve(root, file), 'utf8')
  if (!source.includes('VITE_STATIC_ASSET_BASE_URL')) {
    console.error(`${file} must derive remote static asset URLs at page compile time`)
    process.exit(1)
  }
  if (source.includes('/static/')) {
    localStaticReferences.push(file)
  }
}

if (localStaticReferences.length > 0) {
  console.error(`Pages must not reference local /static assets: ${localStaticReferences.join(', ')}`)
  process.exit(1)
}

const staticHelperImports = staticAssetConsumers.filter((file) =>
  /utils\/(?:static-assets|asset-urls)\.js/.test(readFileSync(resolve(root, file), 'utf8'))
)
if (staticHelperImports.length > 0) {
  console.error(`Pages must not import a separate static URL helper module: ${staticHelperImports.join(', ')}`)
  process.exit(1)
}

console.log('mobile-h5 structure ok')
