import { existsSync, mkdirSync, readFileSync, readdirSync, rmSync, statSync, writeFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { spawnSync } from 'node:child_process'

const root = process.cwd()
const outputDir = resolve(root, 'dist/build/mp-weixin')
const projectConfigPath = resolve(root, 'project.config.json')
const privateProjectConfigPath = resolve(root, 'project.private.config.json')
const distProjectConfigPath = resolve(outputDir, 'project.config.json')
const distPrivateProjectConfigPath = resolve(outputDir, 'project.private.config.json')
const staticOutputDir = resolve(outputDir, 'static')
const commonOutputDir = resolve(outputDir, 'common')
const commonAssetsPath = resolve(commonOutputDir, 'assets.js')
const utilsOutputDir = resolve(outputDir, 'utils')
const shareCompatPath = resolve(utilsOutputDir, 'share.js')
const requiredLibVersion = '3.15.1'

function parseEnvLine(line) {
  const trimmed = line.trim()
  if (!trimmed || trimmed.startsWith('#')) return null
  const normalized = trimmed.startsWith('export ') ? trimmed.slice(7).trim() : trimmed
  const separator = normalized.indexOf('=')
  if (separator < 0) return null
  const key = normalized.slice(0, separator).trim()
  let value = normalized.slice(separator + 1).trim()
  if (!key) return null
  if (
    (value.startsWith('"') && value.endsWith('"')) ||
    (value.startsWith("'") && value.endsWith("'"))
  ) {
    value = value.slice(1, -1)
  }
  return [key, value]
}

function loadEnvFile(path) {
  if (!existsSync(path)) return
  const lines = readFileSync(path, 'utf8').split(/\r?\n/)
  for (const line of lines) {
    const entry = parseEnvLine(line)
    if (!entry) continue
    const [key, value] = entry
    if (process.env[key] === undefined) {
      process.env[key] = value
    }
  }
}

function deriveStaticAssetBaseURL() {
  const explicit = `${process.env.VITE_STATIC_ASSET_BASE_URL || ''}`.trim()
  if (explicit) return explicit
  const publicBaseURL = `${process.env.OSS_PUBLIC_BASE_URL || ''}`.trim().replace(/\/+$/, '')
  if (!publicBaseURL) return ''
  return `${publicBaseURL}/mobile-h5/static`
}

function assertStaticAssetBaseURL() {
  const baseURL = deriveStaticAssetBaseURL().replace(/\/+$/, '')
  if (!baseURL) {
    console.error('VITE_STATIC_ASSET_BASE_URL is required for mp-weixin builds. Run go run ./cmd/upload-static-assets or set an HTTPS OSS static asset base URL.')
    process.exit(1)
  }
  let parsed
  try {
    parsed = new URL(baseURL)
  } catch {
    console.error('VITE_STATIC_ASSET_BASE_URL must be a valid HTTPS URL')
    process.exit(1)
  }
  if (parsed.protocol !== 'https:') {
    console.error('VITE_STATIC_ASSET_BASE_URL must use HTTPS for WeChat Mini Program assets')
    process.exit(1)
  }
  process.env.VITE_STATIC_ASSET_BASE_URL = baseURL
  console.log(`using VITE_STATIC_ASSET_BASE_URL=${baseURL}`)
}

function staticAssetCompatModule(baseURL) {
  return `"use strict";
const staticAssetBaseURL = "${baseURL}";
function staticAsset(path) {
  const normalizedPath = \`\${path || ""}\`.trim().replace(/^\\/+/, "").replace(/^static\\/+/i, "");
  if (!normalizedPath) return staticAssetBaseURL;
  return \`\${staticAssetBaseURL}/\${normalizedPath}\`;
}
function staticIcon(name) {
  const normalizedName = \`\${name || ""}\`.trim().replace(/\\.png$/i, "");
  return staticAsset(\`icons/\${normalizedName}.png\`);
}
exports.staticAsset = staticAsset;
exports.staticIcon = staticIcon;
`
}

function shareCompatModule() {
  return `"use strict";
module.exports = require("./routes.js");
`
}

function filesUnder(dir) {
  if (!existsSync(dir)) return []
  return readdirSync(dir).flatMap((name) => {
    const path = resolve(dir, name)
    if (statSync(path).isDirectory()) return filesUnder(path)
    return [path]
  })
}

function readJSON(path) {
  return JSON.parse(readFileSync(path, 'utf8'))
}

function writeJSON(path, value) {
  writeFileSync(path, `${JSON.stringify(value, null, 2)}\n`, 'utf8')
}

function writeDistProjectConfigs() {
  const parentProjectConfig = readJSON(projectConfigPath)
  const parentPrivateProjectConfig = readJSON(privateProjectConfigPath)

  const distProjectConfig = {
    setting: {
      ...(parentProjectConfig.setting || {}),
      urlCheck: false,
      minified: false,
      minifyWXML: false
    },
    compileType: 'miniprogram',
    libVersion: requiredLibVersion,
    simulatorPluginLibVersion: parentProjectConfig.simulatorPluginLibVersion || {},
    packOptions: parentProjectConfig.packOptions || {
      ignore: [],
      include: []
    },
    appid: parentProjectConfig.appid,
    projectname: parentProjectConfig.projectname || '白霖共享',
    editorSetting: parentProjectConfig.editorSetting || {}
  }

  const distPrivateProjectConfig = {
    libVersion: requiredLibVersion,
    projectname: parentPrivateProjectConfig.projectname || distProjectConfig.projectname,
    setting: {
      ...(parentPrivateProjectConfig.setting || {}),
      urlCheck: false,
      compileHotReLoad: true,
      showShadowRootInWxmlPanel: true
    }
  }

  writeJSON(distProjectConfigPath, distProjectConfig)
  console.log(`created direct WeChat DevTools project config ${distProjectConfigPath}`)
  writeJSON(distPrivateProjectConfigPath, distPrivateProjectConfig)
  console.log(`created direct WeChat DevTools private project config ${distPrivateProjectConfigPath}`)
}

loadEnvFile(resolve(root, '../.env'))
loadEnvFile(resolve(root, '.env'))
loadEnvFile(resolve(root, '.env.static-assets'))
assertStaticAssetBaseURL()

rmSync(outputDir, { recursive: true, force: true })
console.log(`removed ${outputDir}`)

const result = spawnSync('uni', ['build', '-p', 'mp-weixin'], {
  cwd: root,
  env: process.env,
  stdio: 'inherit',
  shell: process.platform === 'win32'
})

if (result.error) {
  console.error(result.error.message)
  process.exit(1)
}

if ((result.status ?? 1) !== 0) {
  process.exit(result.status ?? 1)
}

writeDistProjectConfigs()

rmSync(staticOutputDir, { recursive: true, force: true })
console.log(`removed remote static asset copies from ${staticOutputDir}`)

mkdirSync(commonOutputDir, { recursive: true })
if (!existsSync(commonAssetsPath)) {
  writeFileSync(commonAssetsPath, '"use strict";\n', 'utf8')
  console.log(`created WeChat DevTools compatibility stub ${commonAssetsPath}`)
}

mkdirSync(utilsOutputDir, { recursive: true })
const staticCompatSource = staticAssetCompatModule(process.env.VITE_STATIC_ASSET_BASE_URL)
for (const path of [resolve(utilsOutputDir, 'static-assets.js'), resolve(utilsOutputDir, 'asset-urls.js')]) {
  writeFileSync(path, staticCompatSource, 'utf8')
  console.log(`created WeChat DevTools compatibility module ${path}`)
}
writeFileSync(shareCompatPath, shareCompatModule(), 'utf8')
console.log(`created WeChat DevTools compatibility module ${shareCompatPath}`)

const packageBytes = filesUnder(outputDir).reduce((sum, path) => sum + statSync(path).size, 0)
console.log(`mp-weixin package size after cleanup: ${Math.ceil(packageBytes / 1024)}KB`)
