const jsonHeaders = {
  'Content-Type': 'application/json'
}

const localAuthEntrypoint = 'http://localhost:8888'
const localPreviewEntrypoint = 'http://localhost:5174'
const apiBaseURL = `${import.meta.env.VITE_API_BASE_URL || ''}`.trim().replace(/\/+$/, '')
const authEntrypoint = apiBaseURL || localAuthEntrypoint
let adminSessionCache = null
let csrfTokenCache = ''
const recentNetworkErrors = []
const networkErrorBufferLimit = 20

const csrfCookieName = 'csrf_token'
const csrfHeaderName = 'X-CSRF-Token'
const networkErrorCode = 'network_unreachable'
const tooManyRequestsCode = 'too_many_requests'
const tooManyRequestsMessage = '请求过于频繁，请稍后再试'
const onlineNetworkErrorMessage = '网络连接不稳定，暂时无法连接服务器，请稍后重试'
const offlineNetworkErrorMessage = '当前网络已断开，请检查网络后重试'

export class ApiError extends Error {
  constructor(code, message, status, details = {}) {
    super(message)
    this.code = code
    this.status = status
    Object.assign(this, details)
  }
}

function getApiErrorMessage(code, fallbackMessage) {
  if (code === 'cross_origin_blocked') {
    return `当前页面不是服务端同源地址。请从服务端托管的同源入口重新打开应用后再试。当前入口请使用 ${authEntrypoint}；本地前端预览 ${localPreviewEntrypoint} 不支持登录或其他写操作。`
  }
  if (code === 'alipay_not_configured') {
    return '支付通道维护中，请联系客服'
  }
  if (code === tooManyRequestsCode) {
    return fallbackMessage ?? tooManyRequestsMessage
  }
  return fallbackMessage ?? '请求失败'
}

function getNavigatorOnline() {
  if (typeof navigator === 'undefined' || typeof navigator.onLine !== 'boolean') {
    return true
  }
  return navigator.onLine
}

function isAbortError(error) {
  return error?.name === 'AbortError'
}

function isFetchNetworkError(error) {
  if (isAbortError(error)) return false
  const message = `${error?.message || ''}`.toLowerCase()
  return error instanceof TypeError ||
    message.includes('failed to fetch') ||
    message.includes('networkerror') ||
    message.includes('network request failed') ||
    message.includes('load failed') ||
    message.includes('fetch failed')
}

function recordNetworkError(details) {
  const diagnostic = {
    code: networkErrorCode,
    method: details.method,
    path: details.path,
    online: details.online,
    timestamp: details.timestamp
  }
  recentNetworkErrors.push(diagnostic)
  if (recentNetworkErrors.length > networkErrorBufferLimit) {
    recentNetworkErrors.splice(0, recentNetworkErrors.length - networkErrorBufferLimit)
  }

  if (import.meta.env.DEV) {
    console.warn('[api] network request failed', diagnostic)
  }

  if (typeof window !== 'undefined' && typeof window.dispatchEvent === 'function') {
    window.dispatchEvent(new CustomEvent('image-agent:network-error', {
      detail: {
        ...diagnostic,
        message: details.message,
        status: 0,
        retryable: true
      }
    }))
  }
}

function toNetworkError(error, context) {
  const method = `${context.method || 'GET'}`.toUpperCase()
  const online = getNavigatorOnline()
  const timestamp = new Date().toISOString()
  const message = online ? onlineNetworkErrorMessage : offlineNetworkErrorMessage
  const details = {
    method,
    path: context.path,
    online,
    timestamp,
    retryable: true
  }
  recordNetworkError({ ...details, message })
  return new ApiError(networkErrorCode, message, 0, {
    ...details,
    cause: error
  })
}

async function safeFetch(path, options = {}) {
  try {
    return await fetch(apiURL(path), options)
  } catch (error) {
    if (isFetchNetworkError(error)) {
      throw toNetworkError(error, {
        method: options.method || 'GET',
        path
      })
    }
    throw error
  }
}

async function safeAbsoluteFetch(url, options = {}) {
  try {
    return await fetch(url, options)
  } catch (error) {
    if (isFetchNetworkError(error)) {
      throw toNetworkError(error, {
        method: options.method || 'GET',
        path: url
      })
    }
    throw error
  }
}

export function getRecentNetworkErrors() {
  return recentNetworkErrors.map((entry) => ({ ...entry }))
}

export function clearRecentNetworkErrors() {
  recentNetworkErrors.splice(0, recentNetworkErrors.length)
}

async function request(path, options = {}) {
  const method = `${options.method || 'GET'}`.toUpperCase()
  const hasFormDataBody = typeof FormData !== 'undefined' && options.body instanceof FormData
  const headers = hasFormDataBody
    ? { ...(options.headers ?? {}) }
    : { ...jsonHeaders, ...(options.headers ?? {}) }
  if (isMutatingMethod(method) && !headers[csrfHeaderName]) {
    const token = await getCSRFTokenForRequest()
    if (token) {
      headers[csrfHeaderName] = token
    }
  }

  const response = await safeFetch(path, {
    ...options,
    credentials: 'include',
    headers
  })

  const payload = await response.json().catch(() => ({}))
  if (!response.ok) {
    const error = payload?.error ?? {}
    const code = error.code ?? (response.status === 429 ? tooManyRequestsCode : 'request_failed')
    throw new ApiError(code, getApiErrorMessage(code, error.message), response.status, errorDetails(payload, error, response))
  }
  return payload
}

function errorDetails(payload = {}, error = {}, response = null) {
  const details = {}
  const preservedKeys = [
    'required_credits',
    'available_credits',
    'missing_credits',
    'enough',
    'recommended_package',
    'validation_errors'
  ]
  preservedKeys.forEach((key) => {
    if (error?.[key] !== undefined) {
      details[key] = error[key]
      return
    }
    if (payload?.[key] !== undefined) {
      details[key] = payload[key]
    }
  })
  const retryAfterSeconds = readRetryAfterSeconds(response?.headers)
  if (retryAfterSeconds !== null) {
    details.retry_after_seconds = retryAfterSeconds
  }
  return details
}

function readRetryAfterSeconds(headers) {
  if (!headers || typeof headers.get !== 'function') {
    return null
  }
  const raw = headers.get('Retry-After')
  if (!raw) {
    return null
  }
  const seconds = Number.parseInt(raw, 10)
  if (!Number.isFinite(seconds) || seconds <= 0) {
    return null
  }
  return seconds
}

function isMutatingMethod(method) {
  return method !== 'GET' && method !== 'HEAD' && method !== 'OPTIONS'
}

function readCookie(name) {
  if (typeof document === 'undefined' || !document.cookie) return ''
  const prefix = `${name}=`
  const parts = document.cookie.split(';')
  for (const part of parts) {
    const text = part.trim()
    if (text.startsWith(prefix)) {
      return decodeURIComponent(text.slice(prefix.length))
    }
  }
  return ''
}

async function getCSRFTokenForRequest() {
  const cookieToken = readCookie(csrfCookieName)
  if (cookieToken) {
    csrfTokenCache = cookieToken
    return cookieToken
  }

  const response = await safeFetch('/api/auth/csrf-token', {
    credentials: 'include'
  })
  const payload = await response.json().catch(() => ({}))
  if (!response.ok) {
    const error = payload?.error ?? {}
    const code = error.code ?? 'csrf_token_failed'
    throw new ApiError(code, getApiErrorMessage(code, error.message), response.status)
  }
  csrfTokenCache = payload?.csrf_token || readCookie(csrfCookieName)
  return csrfTokenCache
}

function apiURL(path) {
  if (/^https?:\/\//i.test(path)) return path
  if (!apiBaseURL) return path
  return `${apiBaseURL}/${path.replace(/^\/+/, '')}`
}

function appendOSSFormFields(formData, fields = {}) {
  Object.entries(fields).forEach(([key, value]) => {
    if (value !== undefined && value !== null) {
      formData.append(key, value)
    }
  })
}

async function uploadFileToOSS(policy, file) {
  const formData = new FormData()
  appendOSSFormFields(formData, policy.form_data)
  formData.append('file', file)
  const response = await safeAbsoluteFetch(policy.upload_url, {
    method: 'POST',
    body: formData
  })
  if (!response.ok) {
    throw new ApiError('reference_asset_oss_upload_failed', '参考图片上传失败', response.status)
  }
}

function toQuery(params = {}) {
  const query = new URLSearchParams()
  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== '') {
      query.set(key, `${value}`)
    }
  })
  const text = query.toString()
  return text ? `?${text}` : ''
}

export const api = {
  register(payload) {
    return request('/api/auth/register', {
      method: 'POST',
      body: JSON.stringify(payload)
    })
  },
  sendSMSCode(payload) {
    return request('/api/auth/sms-code', {
      method: 'POST',
      body: JSON.stringify(payload)
    })
  },
  resetPassword(payload) {
    return request('/api/auth/reset-password', {
      method: 'POST',
      body: JSON.stringify(payload)
    })
  },
  getCaptcha(purpose) {
    return request(`/api/auth/captcha${toQuery({ purpose })}`)
  },
  registerPhone(payload) {
    return request('/api/auth/register-phone', {
      method: 'POST',
      body: JSON.stringify(payload)
    })
  },
  login(username, password, captcha = {}, options = {}) {
    return request('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({
        username,
        password,
        ...captcha,
        remember_login: Boolean(options.rememberLogin)
      })
    })
  },
  logout() {
    return request('/api/auth/logout', {
      method: 'POST',
      body: JSON.stringify({})
    })
  },
  getMe() {
    return request('/api/me')
  },
  pingPresence() {
    return request('/api/account/presence')
  },
  getPackages() {
    return request('/api/packages')
  },
  getCustomerService() {
    return request('/api/customer-service')
  },
  createContentReport(payload) {
    return request('/api/content-reports', {
      method: 'POST',
      body: JSON.stringify(payload)
    })
  },
  createAlipayOrder(payload) {
    return request('/api/payments/alipay/orders', {
      method: 'POST',
      body: JSON.stringify(payload)
    })
  },
  getAlipayOrder(orderNumber) {
    return request(`/api/payments/alipay/orders/${encodeURIComponent(orderNumber)}`)
  },
  payAlipayOrder(orderNumber) {
    return request(`/api/payments/alipay/orders/${encodeURIComponent(orderNumber)}/pay`, {
      method: 'POST',
      body: JSON.stringify({})
    })
  },
  queryAlipayOrder(orderNumber) {
    return request(`/api/payments/alipay/orders/${encodeURIComponent(orderNumber)}/query`, {
      method: 'POST',
      body: JSON.stringify({})
    })
  },
  getCredits() {
    return request('/api/account/credits')
  },
  getCreditTransactions(params = {}) {
    return request(`/api/account/credit-transactions${toQuery(params)}`)
  },
  bindAccountPhone(payload) {
    return request('/api/account/phone', {
      method: 'POST',
      body: JSON.stringify(payload)
    })
  },
  unbindAccountPhone(payload) {
    return request('/api/account/phone', {
      method: 'DELETE',
      body: JSON.stringify(payload)
    })
  },
  updateProfile(payload) {
    return request('/api/account/profile', {
      method: 'PATCH',
      body: JSON.stringify(payload)
    })
  },
  updateAccountEmail(payload) {
    return request('/api/account/email', {
      method: 'PATCH',
      body: JSON.stringify(payload)
    })
  },
  updateAccountPreferences(payload) {
    return request('/api/account/preferences', {
      method: 'PATCH',
      body: JSON.stringify(payload)
    })
  },
  changePassword(payload) {
    return request('/api/account/password', {
      method: 'POST',
      body: JSON.stringify(payload)
    })
  },
  listWorks(params = {}) {
    return request(`/api/works${toQuery(params)}`)
  },
  getPublicWorks(params = {}) {
    return request(`/api/public/works${toQuery(params)}`)
  },
  listPromptTemplates(params = {}) {
    return request(`/api/prompt-templates${toQuery(params)}`)
  },
  getWorkspaceDiscovery() {
    return request('/api/workspace/discovery')
  },
  useInspirationRecommendation(id) {
    return request(`/api/workspace/inspiration-recommendations/${encodeURIComponent(id)}/use`, {
      method: 'POST',
      body: JSON.stringify({})
    })
  },
  usePromptTemplate(id) {
    return request(`/api/prompt-templates/${id}/use`, {
      method: 'POST',
      body: JSON.stringify({})
    })
  },
  getCoupleAlbumOptions() {
    return request('/api/couple-album/options')
  },
  estimateCoupleAlbum(input) {
    return request('/api/couple-albums/estimate', {
      method: 'POST',
      body: JSON.stringify(input)
    })
  },
  createCoupleAlbum(input) {
    return request('/api/couple-albums', {
      method: 'POST',
      body: JSON.stringify(input)
    })
  },
  generateCoupleAlbum(id) {
    return request(`/api/couple-albums/${encodeURIComponent(id)}/generate`, {
      method: 'POST',
      body: JSON.stringify({})
    })
  },
  getCoupleAlbum(id) {
    return request(`/api/couple-albums/${encodeURIComponent(id)}`)
  },
  listCoupleAlbums() {
    return request('/api/couple-albums')
  },
  retryCoupleAlbumPage(id, pageID) {
    return request(`/api/couple-albums/${encodeURIComponent(id)}/pages/${encodeURIComponent(pageID)}/retry`, {
      method: 'POST',
      body: JSON.stringify({})
    })
  },
  shareCoupleAlbum(id) {
    return request(`/api/couple-albums/${encodeURIComponent(id)}/share`, {
      method: 'POST',
      body: JSON.stringify({})
    })
  },
  getPublicCoupleAlbum(token) {
    return request(`/api/public/couple-albums/${encodeURIComponent(token)}`)
  },
  getCommerceCapabilities() { return request('/api/ecommerce/capabilities') },
  getCommerceProduct(id) { return request(`/api/ecommerce/products/${encodeURIComponent(id)}`) },
	patchCommerceProduct(id, input) { return request(`/api/ecommerce/products/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(input) }) },
  listCommerceSKUs(productId) { return request(`/api/ecommerce/products/${encodeURIComponent(productId)}/skus`) },
  createCommerceSKU(productId, input) { return request(`/api/ecommerce/products/${encodeURIComponent(productId)}/skus`, { method: 'POST', body: JSON.stringify(input) }) },
  patchCommerceSKU(id, input) { return request(`/api/ecommerce/skus/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(input) }) },
  getCommerceSKUConfig(productId) { return request(`/api/ecommerce/products/${encodeURIComponent(productId)}/sku-config`) },
  previewCommerceSKUMatrix(productId, input) { return request(`/api/ecommerce/products/${encodeURIComponent(productId)}/sku-matrix/preview`, { method: 'POST', body: JSON.stringify(input) }) },
  applyCommerceSKUMatrix(productId, input, idempotencyKey) { return request(`/api/ecommerce/products/${encodeURIComponent(productId)}/sku-matrix`, { method: 'PUT', headers: { 'Idempotency-Key': idempotencyKey }, body: JSON.stringify(input) }) },
  bootstrapCommerceProject(input, idempotencyKey) {
    if (!`${idempotencyKey || ''}`.trim()) return Promise.reject(new Error('bootstrapCommerceProject requires Idempotency-Key'))
    return request('/api/ecommerce/projects/bootstrap', { method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: JSON.stringify(input) })
  },
	listCommerceCategories() { return request('/api/ecommerce/categories') },
	createCommerceCustomCategory(input) { return request('/api/ecommerce/categories/custom', { method: 'POST', body: JSON.stringify(input) }) },
	patchCommerceCustomCategory(id, input) { return request(`/api/ecommerce/categories/custom/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(input) }) },
	listAdminCommerceCategories() { return request('/api/admin/ecommerce/categories') },
	createAdminCommerceCategory(input) { return request('/api/admin/ecommerce/categories', { method: 'POST', body: JSON.stringify(input) }) },
	patchAdminCommerceCategory(id, input) { return request(`/api/admin/ecommerce/categories/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(input) }) },
  analyzeCommerceCreativeSpec(projectId, input, idempotencyKey) {
    if (!`${idempotencyKey || ''}`.trim()) return Promise.reject(new Error('analyzeCommerceCreativeSpec requires Idempotency-Key'))
    return request(`/api/ecommerce/projects/${encodeURIComponent(projectId)}/creative-specs/analyze`, { method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: JSON.stringify(input) })
  },
  createManualCommerceCreativeSpec(projectId, input) { return request(`/api/ecommerce/projects/${encodeURIComponent(projectId)}/creative-specs`, { method: 'POST', body: JSON.stringify(input) }) },
  getCommerceCreativeSpec(id) { return request(`/api/ecommerce/creative-specs/${encodeURIComponent(id)}`) },
  getLatestCommerceCreativeSpec(projectId) { return request(`/api/ecommerce/projects/${encodeURIComponent(projectId)}/creative-specs/latest`) },
  patchCommerceCreativeSpec(id, input) { return request(`/api/ecommerce/creative-specs/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(input) }) },
  confirmCommerceCreativeSpec(id) { return request(`/api/ecommerce/creative-specs/${encodeURIComponent(id)}/confirm`, { method: 'POST', body: JSON.stringify({}) }) },
  listCommerceProjects() { return request('/api/ecommerce/projects') },
  createCommerceProject(input) { return request('/api/ecommerce/projects', { method: 'POST', body: JSON.stringify(input) }) },
  getCommerceProject(id) { return request(`/api/ecommerce/projects/${encodeURIComponent(id)}`) },
  patchCommerceProject(id, input) { return request(`/api/ecommerce/projects/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(input) }) },
  deleteCommerceProject(id) { return request(`/api/ecommerce/projects/${encodeURIComponent(id)}`, { method: 'DELETE', body: JSON.stringify({}) }) },
  listCommerceAssets(projectId) { return request(`/api/ecommerce/projects/${encodeURIComponent(projectId)}/assets`) },
  createCommerceAssetUploadPolicy(projectId, input) { return request(`/api/ecommerce/projects/${encodeURIComponent(projectId)}/assets/upload-policy`, { method: 'POST', body: JSON.stringify(input) }) },
  uploadCommerceAssetBinary(policy, file) { return uploadFileToOSS(policy, file) },
  completeCommerceAssetUpload(projectId, input) { return request(`/api/ecommerce/projects/${encodeURIComponent(projectId)}/assets/complete-upload`, { method: 'POST', body: JSON.stringify(input) }) },
  deleteCommerceAsset(projectId, assetId) { return request(`/api/ecommerce/projects/${encodeURIComponent(projectId)}/assets/${encodeURIComponent(assetId)}`, { method: 'DELETE', body: JSON.stringify({}) }) },
  listCommerceRecipes(params = {}) { return request(`/api/ecommerce/recipes${toQuery(params)}`) },
  estimateCommerceBatch(projectId, input) { return request(`/api/ecommerce/projects/${encodeURIComponent(projectId)}/batches/estimate`, { method: 'POST', body: JSON.stringify(input) }) },
  createCommerceBatch(projectId, input, idempotencyKey) { return request(`/api/ecommerce/projects/${encodeURIComponent(projectId)}/batches`, { method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: JSON.stringify(input) }) },
  getCommerceBatch(id) { return request(`/api/ecommerce/batches/${encodeURIComponent(id)}`) },
  listCommerceBatches(projectId) { return request(`/api/ecommerce/projects/${encodeURIComponent(projectId)}/batches`) },
  listCommerceBatchEvents(id, params = {}) { return request(`/api/ecommerce/batches/${encodeURIComponent(id)}/events${toQuery(params)}`) },
  cancelCommerceBatch(id) { return request(`/api/ecommerce/batches/${encodeURIComponent(id)}/cancel`, { method: 'POST', body: JSON.stringify({}) }) },
  cancelCommerceItem(id) { return request(`/api/ecommerce/items/${encodeURIComponent(id)}/cancel`, { method: 'POST', body: JSON.stringify({}) }) },
  retryCommerceItem(id, idempotencyKey) {
    if (!`${idempotencyKey || ''}`.trim()) return Promise.reject(new Error('retryCommerceItem requires Idempotency-Key'))
    return request(`/api/ecommerce/items/${encodeURIComponent(id)}/retry`, { method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: JSON.stringify({}) })
  },
  listReferenceAssets(params = {}) {
    return request(`/api/reference-assets${toQuery(params)}`)
  },
  uploadReferenceAsset(file) {
    return request('/api/reference-assets/upload-policy', {
      method: 'POST',
      body: JSON.stringify({
        filename: file?.name || 'reference-image',
        mime_type: file?.type || '',
        size: file?.size || 0
      })
    }).then(async (policy) => {
      await uploadFileToOSS(policy, file)
      return request('/api/reference-assets/complete-upload', {
        method: 'POST',
        body: JSON.stringify({
          object_key: policy.object_key,
          upload_token: policy.upload_token
        })
      })
    })
  },
  deleteReferenceAsset(id) {
    return request(`/api/reference-assets/${id}`, {
      method: 'DELETE',
      body: JSON.stringify({})
    })
  },
  updateReferenceAsset(id, payload) {
    return request(`/api/reference-assets/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(payload)
    })
  },
  getWork(id) {
    return request(`/api/works/${id}`)
  },
  updateWork(id, payload) {
    return request(`/api/works/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(payload)
    })
  },
  deleteWork(id) {
    return request(`/api/works/${id}`, {
      method: 'DELETE',
      body: JSON.stringify({})
    })
  },
  reuseWork(id) {
    return request(`/api/works/${id}/reuse`, {
      method: 'POST',
      body: JSON.stringify({})
    })
  },
  generateImage(input) {
    return request('/api/images/generations', {
      method: 'POST',
      body: JSON.stringify(input)
    })
  },
  estimateImageGeneration(input, options = {}) {
    return request('/api/images/generations/estimate', {
      ...options,
      method: 'POST',
      body: JSON.stringify(input)
    })
  },
  optimizePrompt(input, options = {}) {
    return request('/api/prompts/optimize', {
      ...options,
      method: 'POST',
      body: JSON.stringify(input)
    })
  },
  planImageAgent(input) {
    return request('/api/agent/image-plan', {
      method: 'POST',
      body: JSON.stringify(input)
    })
  },
  createImageGeneration(input) {
    return request('/api/images/generations/async', {
      method: 'POST',
      body: JSON.stringify(input)
    })
  },
  estimateVirtualTryOn(input, options = {}) {
    return request('/api/virtual-try-on/generations/estimate', {
      ...options,
      method: 'POST',
      body: JSON.stringify(input)
    })
  },
  createVirtualTryOn(input) {
    return request('/api/virtual-try-on/generations/async', {
      method: 'POST',
      body: JSON.stringify(input)
    })
  },
  planMomentsMarketing(input) {
    return request('/api/marketing/moments/plan', {
      method: 'POST',
      body: JSON.stringify(input)
    })
  },
  planArticleImages(input) {
    return request('/api/marketing/article-images/plan', {
      method: 'POST',
      body: JSON.stringify(input)
    })
  },
  getImageGeneration(id) {
    return request(`/api/images/generations/${id}`)
  },
  cancelImageGeneration(id) {
    return request(`/api/images/generations/${id}/cancel`, {
      method: 'POST',
      body: JSON.stringify({})
    })
  },
  createVideoGeneration(input, options = {}) {
    return request('/api/videos/generations/async', {
	  ...options,
      method: 'POST',
      body: JSON.stringify(input)
    })
  },
	createVideoConversation(input = {}) {
	  return request('/api/videos/conversations', { method: 'POST', body: JSON.stringify(input) })
	},
	listVideoConversations(params = {}) {
	  return request(`/api/videos/conversations${toQuery(params)}`)
	},
	getVideoConversation(id) {
	  return request(`/api/videos/conversations/${encodeURIComponent(id)}`)
	},
	patchVideoConversation(id, input) {
	  return request(`/api/videos/conversations/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(input) })
	},
	createVideoConversationMessage(id, input, idempotencyKey) {
	  return request(`/api/videos/conversations/${encodeURIComponent(id)}/messages`, { method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, body: JSON.stringify(input) })
	},
  estimateVideoGeneration(input, options = {}) {
    return request('/api/videos/generations/estimate', {
      ...options,
      method: 'POST',
      body: JSON.stringify(input)
    })
  },
  listUserVideoGenerations(params = {}) {
    return request(`/api/videos/generations${toQuery(params)}`)
  },
  listVideoModels() {
    return request('/api/videos/models')
  },
  getVideoGeneration(id) {
    return request(`/api/videos/generations/${id}`)
  },
  listVideoStylePresets() {
    return request('/api/videos/style-presets')
  },
  listVideoStyleTemplates() {
    return request('/api/videos/style-templates')
  },
  createVideoStyleTemplate(payload) {
    return request('/api/videos/style-templates', {
      method: 'POST',
      body: JSON.stringify(payload)
    })
  },
  deleteVideoStyleTemplate(id) {
    return request(`/api/videos/style-templates/${encodeURIComponent(id)}`, {
      method: 'DELETE',
      body: JSON.stringify({})
    })
  },
  listVideoSoundtracks(workID) {
    return request(`/api/videos/${encodeURIComponent(workID)}/soundtracks`, {
      method: 'GET'
    })
  },
  generateVideoSoundtrack(workID, input) {
    return request(`/api/videos/${encodeURIComponent(workID)}/soundtracks/generate`, {
      method: 'POST',
      body: JSON.stringify(input)
    })
  },
  uploadVideoSoundtrack(workID, file) {
    const formData = new FormData()
    formData.append('file', file)
    return request(`/api/videos/${encodeURIComponent(workID)}/soundtracks/upload`, {
      method: 'POST',
      body: formData
    })
  },
  createNovelVideoProject(input) {
    return request('/api/novel-video-projects', {
      method: 'POST',
      body: JSON.stringify(input)
    })
  },
  updateNovelVideoProject(id, payload) {
    return request(`/api/novel-video-projects/${encodeURIComponent(id)}`, {
      method: 'PATCH',
      body: JSON.stringify(payload)
    })
  },
  listNovelVideoProjects() {
    return request('/api/novel-video-projects')
  },
  getNovelVideoProject(id) {
    return request(`/api/novel-video-projects/${encodeURIComponent(id)}`)
  },
  analyzeNovelVideoProject(id) {
    return request(`/api/novel-video-projects/${encodeURIComponent(id)}/analyze`, {
      method: 'POST',
      body: JSON.stringify({})
    })
  },
  generateNovelVideoImagePlan(id, payload = {}) {
    return request(`/api/novel-video-projects/${encodeURIComponent(id)}/image-plan`, {
      method: 'POST',
      body: JSON.stringify(payload)
    })
  },
  updateNovelVideoCreature(projectID, creatureID, payload) {
    return request(`/api/novel-video-projects/${encodeURIComponent(projectID)}/creatures/${encodeURIComponent(creatureID)}`, {
      method: 'PATCH',
      body: JSON.stringify(payload)
    })
  },
  updateNovelVideoActor(projectID, actorID, payload) {
    return request(`/api/novel-video-projects/${encodeURIComponent(projectID)}/actors/${encodeURIComponent(actorID)}`, {
      method: 'PATCH',
      body: JSON.stringify(payload)
    })
  },
  generateNovelVideoActorLockSheet(projectID, actorID) {
    return request(`/api/novel-video-projects/${encodeURIComponent(projectID)}/actors/${encodeURIComponent(actorID)}/generate-lock-sheet`, {
      method: 'POST',
      body: JSON.stringify({})
    })
  },
  generateNovelVideoCreatureImage(projectID, creatureID) {
    return request(`/api/novel-video-projects/${encodeURIComponent(projectID)}/creatures/${encodeURIComponent(creatureID)}/generate-image`, {
      method: 'POST',
      body: JSON.stringify({})
    })
  },
  generateNovelVideoAssets(projectID, payload = {}) {
    return request(`/api/novel-video-projects/${encodeURIComponent(projectID)}/assets/generate`, {
      method: 'POST',
      body: JSON.stringify(payload)
    })
  },
  dedupeNovelVideoAssets(projectID) {
    return request(`/api/novel-video-projects/${encodeURIComponent(projectID)}/assets/dedupe`, {
      method: 'POST',
      body: JSON.stringify({})
    })
  },
  deleteNovelVideoAsset(projectID, assetID) {
    return request(`/api/novel-video-projects/${encodeURIComponent(projectID)}/assets/${encodeURIComponent(assetID)}`, {
      method: 'DELETE',
      body: JSON.stringify({})
    })
  },
  updateNovelVideoAsset(projectID, assetID, payload) {
    return request(`/api/novel-video-projects/${encodeURIComponent(projectID)}/assets/${encodeURIComponent(assetID)}`, {
      method: 'PATCH',
      body: JSON.stringify(payload)
    })
  },
  planNovelVideoEpisodes(id) {
    return request(`/api/novel-video-projects/${encodeURIComponent(id)}/episodes/plan`, {
      method: 'POST',
      body: JSON.stringify({})
    })
  },
  updateNovelVideoShot(projectID, shotID, payload) {
    return request(`/api/novel-video-projects/${encodeURIComponent(projectID)}/shots/${encodeURIComponent(shotID)}`, {
      method: 'PATCH',
      body: JSON.stringify(payload)
    })
  },
  renderNovelVideoApprovedShots(id) {
    return request(`/api/novel-video-projects/${encodeURIComponent(id)}/render-approved-shots`, {
      method: 'POST',
      body: JSON.stringify({})
    })
  },
  renderNovelVideoPreflight(id) {
    return request(`/api/novel-video-projects/${encodeURIComponent(id)}/render-preflight`, {
      method: 'POST',
      body: JSON.stringify({})
    })
  },
  queueNovelVideoRender(id) {
    return request(`/api/novel-video-projects/${encodeURIComponent(id)}/render`, {
      method: 'POST',
      body: JSON.stringify({})
    })
  },
  generateNovelVideoStoryboard(projectID, shotID) {
    return request(`/api/novel-video-projects/${encodeURIComponent(projectID)}/shots/${encodeURIComponent(shotID)}/storyboard`, {
      method: 'POST',
      body: JSON.stringify({})
    })
  },
  generateNovelVideoGrids(projectID, payload = {}) {
    return request(`/api/novel-video-projects/${encodeURIComponent(projectID)}/grids/generate`, {
      method: 'POST',
      body: JSON.stringify(payload)
    })
  },
  generateNovelVideoShotImages(projectID, payload = {}) {
    return request(`/api/novel-video-projects/${encodeURIComponent(projectID)}/images/generate`, {
      method: 'POST',
      body: JSON.stringify(payload)
    })
  },
  listNovelVideoShotImages(projectID, params = {}) {
    return request(`/api/novel-video-projects/${encodeURIComponent(projectID)}/images${toQuery(params)}`)
  },
  updateNovelVideoShotImage(projectID, imageID, payload = {}) {
    return request(`/api/novel-video-projects/${encodeURIComponent(projectID)}/images/${encodeURIComponent(imageID)}`, {
      method: 'PATCH',
      body: JSON.stringify(payload)
    })
  },
  getNovelVideoCostEstimate(id) {
    return request(`/api/novel-video-projects/${encodeURIComponent(id)}/cost-estimate`)
  },
  composeNovelVideoProject(id) {
    return request(`/api/novel-video-projects/${encodeURIComponent(id)}/compose`, {
      method: 'POST',
      body: JSON.stringify({})
    })
  },
  listNovelVideoCompositions(id) {
    return request(`/api/novel-video-projects/${encodeURIComponent(id)}/compositions`)
  },
  listNovelVideoEvents(id) {
    return request(`/api/novel-video-projects/${encodeURIComponent(id)}/events`)
  },
  restoreNovelVideoVersion(projectID, versionID) {
    return request(`/api/novel-video-projects/${encodeURIComponent(projectID)}/versions/${encodeURIComponent(versionID)}/restore`, {
      method: 'POST',
      body: JSON.stringify({})
    })
  },
  async exportNovelVideoProject(id) {
    const response = await safeFetch(`/api/novel-video-projects/${encodeURIComponent(id)}/export`, {
      credentials: 'include'
    })
    if (!response.ok) {
      const payload = await response.json().catch(() => ({}))
      const error = payload?.error ?? {}
      throw new ApiError(error.code ?? 'request_failed', getApiErrorMessage(error.code, error.message), response.status)
    }
    return response.text()
  },
  exportNovelVideoProjectJSON(id) {
    return request(`/api/novel-video-projects/${encodeURIComponent(id)}/export?format=json`)
  },
  async exportNovelVideoProjectPackage(id, format = 'zip') {
    const response = await safeFetch(`/api/novel-video-projects/${encodeURIComponent(id)}/export?format=${encodeURIComponent(format)}`, {
      credentials: 'include'
    })
    if (!response.ok) {
      const payload = await response.json().catch(() => ({}))
      const error = payload?.error ?? {}
      throw new ApiError(error.code ?? 'request_failed', getApiErrorMessage(error.code, error.message), response.status)
    }
    return response.blob()
  },
  adminLogin(username, password, captcha = {}, options = {}) {
    return request('/api/admin/login', {
      method: 'POST',
      body: JSON.stringify({
        username,
        password,
        ...captcha,
        remember_login: Boolean(options.rememberLogin)
      })
    }).then((payload) => {
      adminSessionCache = payload
      return payload
    })
  },
  adminLogout() {
    return request('/api/admin/logout', {
      method: 'POST',
      body: JSON.stringify({})
    }).finally(() => {
      adminSessionCache = null
    })
  },
  changeAdminPassword(payload) {
    return request('/api/admin/password', {
      method: 'POST',
      body: JSON.stringify(payload)
    }).then((responsePayload) => {
      adminSessionCache = null
      return responsePayload
    })
  },
  getAdminMe() {
    return request('/api/admin/me').then((payload) => {
      adminSessionCache = payload
      return payload
    })
  },
  searchAdmin(params = {}) {
    return request(`/api/admin/search${toQuery(params)}`)
  },
  getDashboard() {
    return request('/api/admin/dashboard')
  },
  listPopupAnnouncements(client = 'web') {
    return request(`/api/announcements/popup${toQuery({ client })}`)
  },
  dismissAnnouncement(id, client = 'web') {
    return request(`/api/announcements/${id}/dismiss`, {
      method: 'POST',
      body: JSON.stringify({ client })
    })
  },
  getImageSettings() {
    return request('/api/admin/settings/image')
  },
  updateImageSettings(payload) {
    return request('/api/admin/settings/image', {
      method: 'PUT',
      body: JSON.stringify(payload)
    })
  },
  getSystemSettings() {
    return request('/api/admin/system-settings')
  },
  updateSystemSettings(payload) {
    return request('/api/admin/system-settings', {
      method: 'PATCH',
      body: JSON.stringify(payload)
    })
  },
  systemSettingsExportURL() {
    return apiURL('/api/admin/system-settings/export')
  },
  listSystemLogs(params = {}) {
    return request(`/api/admin/system-logs${toQuery(params)}`)
  },
  getSystemResources() {
    return request('/api/admin/system-resources')
  },
  systemLogsExportURL(params = {}) {
    return apiURL(`/api/admin/system-logs/export${toQuery(params)}`)
  },
  getAdminCustomerService() {
    return request('/api/admin/customer-service')
  },
  updateAdminCustomerService(payload) {
    return request('/api/admin/customer-service', {
      method: 'PATCH',
      body: JSON.stringify(payload)
    })
  },
  uploadCustomerServiceQRCode(file) {
    const formData = new FormData()
    formData.set('file', file)
    return request('/api/admin/customer-service/qrcode', {
      method: 'POST',
      body: formData
    })
  },
  listAdminModels(params = {}) {
    return request(`/api/admin/models${toQuery(params)}`)
  },
  createAdminModel(payload) {
    return request('/api/admin/models', {
      method: 'POST',
      body: JSON.stringify(payload)
    })
  },
  updateAdminModel(id, payload) {
    return request(`/api/admin/models/${id}`, {
      method: 'PUT',
      body: JSON.stringify(payload)
    })
  },
  deleteAdminModel(id, options = {}) {
    const query = options.force ? '?force=true' : ''
    return request(`/api/admin/models/${id}${query}`, {
      method: 'DELETE',
      body: JSON.stringify({})
    })
  },
  getAdminModel(id) {
    return request(`/api/admin/models/${id}`)
  },
  getModelCenterOverview() {
    return request('/api/admin/model-center/overview')
  },
  listModelCenterModels(params = {}) {
    return request(`/api/admin/model-center/models${toQuery(params)}`)
  },
  createModelCenterModel(payload) {
    return request('/api/admin/model-center/models', {
      method: 'POST',
      body: JSON.stringify(payload)
    })
  },
  updateModelCenterModel(id, payload) {
    return request(`/api/admin/model-center/models/${id}`, {
      method: 'PUT',
      body: JSON.stringify(payload)
    })
  },
  deleteModelCenterModel(id) {
    return request(`/api/admin/model-center/models/${id}`, {
      method: 'DELETE',
      body: JSON.stringify({})
    })
  },
  listModelCenterProviders(params = {}) {
    return request(`/api/admin/model-center/providers${toQuery(params)}`)
  },
  createModelCenterProvider(payload) {
    return request('/api/admin/model-center/providers', {
      method: 'POST',
      body: JSON.stringify(payload)
    })
  },
  updateModelCenterProvider(id, payload) {
    return request(`/api/admin/model-center/providers/${id}`, {
      method: 'PUT',
      body: JSON.stringify(payload)
    })
  },
  deleteModelCenterProvider(id) {
    return request(`/api/admin/model-center/providers/${id}`, {
      method: 'DELETE',
      body: JSON.stringify({})
    })
  },
  listModelCenterChannels(params = {}) {
    return request(`/api/admin/model-center/channels${toQuery(params)}`)
  },
  listModelCenterChannelCallAttempts(channelId, params = {}) {
    return request(`/api/admin/model-center/channels/${encodeURIComponent(channelId)}/call-attempts${toQuery(params)}`)
  },
  createModelCenterChannel(payload) {
    return request('/api/admin/model-center/channels', {
      method: 'POST',
      body: JSON.stringify(payload)
    })
  },
  updateModelCenterChannel(id, payload) {
    return request(`/api/admin/model-center/channels/${id}`, {
      method: 'PUT',
      body: JSON.stringify(payload)
    })
  },
  deleteModelCenterChannel(id) {
    return request(`/api/admin/model-center/channels/${id}`, {
      method: 'DELETE',
      body: JSON.stringify({})
    })
  },
  getModelCenterRouting() {
    return request('/api/admin/model-center/routing')
  },
  updateModelCenterRouting(payload) {
    return request('/api/admin/model-center/routing', {
      method: 'PUT',
      body: JSON.stringify(payload)
    })
  },
  listModelCenterAuditLogs(params = {}) {
    return request(`/api/admin/model-center/audit-logs${toQuery(params)}`)
  },
  getModelRouting() {
    return request('/api/admin/model-routing')
  },
  updateModelRouting(payload) {
    return request('/api/admin/model-routing', {
      method: 'PUT',
      body: JSON.stringify(payload)
    })
  },
  listAdminPromptTemplates(params = {}) {
    return request(`/api/admin/prompt-templates${toQuery(params)}`)
  },
  listAdminInspirationRecommendations(params = {}) {
    return request(`/api/admin/inspiration-recommendations${toQuery(params)}`)
  },
  listAdminVideoStylePresets(params = {}) {
    return request(`/api/admin/video-style-presets${toQuery(params)}`)
  },
  createAdminVideoStylePreset(payload) {
    return request('/api/admin/video-style-presets', {
      method: 'POST',
      body: JSON.stringify(payload)
    })
  },
  updateAdminVideoStylePreset(id, payload) {
    return request(`/api/admin/video-style-presets/${id}`, {
      method: 'PUT',
      body: JSON.stringify(payload)
    })
  },
  deleteAdminVideoStylePreset(id) {
    return request(`/api/admin/video-style-presets/${id}`, {
      method: 'DELETE',
      body: JSON.stringify({})
    })
  },
  createAdminInspirationRecommendation(payload) {
    return request('/api/admin/inspiration-recommendations', {
      method: 'POST',
      body: JSON.stringify(payload)
    })
  },
  updateAdminInspirationRecommendation(id, payload) {
    return request(`/api/admin/inspiration-recommendations/${id}`, {
      method: 'PUT',
      body: JSON.stringify(payload)
    })
  },
  deleteAdminInspirationRecommendation(id) {
    return request(`/api/admin/inspiration-recommendations/${id}`, {
      method: 'DELETE',
      body: JSON.stringify({})
    })
  },
  createAdminPromptTemplate(payload) {
    return request('/api/admin/prompt-templates', {
      method: 'POST',
      body: JSON.stringify(payload)
    })
  },
  updateAdminPromptTemplate(id, payload) {
    return request(`/api/admin/prompt-templates/${id}`, {
      method: 'PUT',
      body: JSON.stringify(payload)
    })
  },
  deleteAdminPromptTemplate(id) {
    return request(`/api/admin/prompt-templates/${id}`, {
      method: 'DELETE',
      body: JSON.stringify({})
    })
  },
  generateAdminPromptTemplatePreview(id, payload = {}) {
    return request(`/api/admin/prompt-templates/${id}/preview`, {
      method: 'POST',
      body: JSON.stringify(payload)
    })
  },
  batchGenerateAdminPromptTemplatePreviews(payload = {}) {
    return request('/api/admin/prompt-templates/previews/generate', {
      method: 'POST',
      body: JSON.stringify(payload)
    })
  },
  listAdminCoupleAlbumOptions(params = {}) {
    return request(`/api/admin/couple-album-options${toQuery(params)}`)
  },
  createAdminCoupleAlbumOption(payload) {
    return request('/api/admin/couple-album-options', {
      method: 'POST',
      body: JSON.stringify(payload)
    })
  },
  updateAdminCoupleAlbumOption(id, payload) {
    return request(`/api/admin/couple-album-options/${id}`, {
      method: 'PUT',
      body: JSON.stringify(payload)
    })
  },
  deleteAdminCoupleAlbumOption(id) {
    return request(`/api/admin/couple-album-options/${id}`, {
      method: 'DELETE',
      body: JSON.stringify({})
    })
  },
  uploadCoupleAlbumOptionAsset(file) {
    const formData = new FormData()
    formData.set('file', file)
    return request('/api/admin/couple-album-options/assets', {
      method: 'POST',
      body: formData
    })
  },
  listInvites(params = {}) {
    return request(`/api/admin/invites${toQuery(params)}`)
  },
  createInvite(payload) {
    return request('/api/admin/invites', {
      method: 'POST',
      body: JSON.stringify(payload)
    })
  },
  batchCreateInvites(payload) {
    return request('/api/admin/invites/batch', {
      method: 'POST',
      body: JSON.stringify(payload)
    })
  },
  updateInvite(id, payload) {
    return request(`/api/admin/invites/${id}`, {
      method: 'PUT',
      body: JSON.stringify(payload)
    })
  },
  listInviteRedemptions(params = {}) {
    return request(`/api/admin/invite-redemptions${toQuery(params)}`)
  },
  listGenerations(params = {}) {
    return request(`/api/admin/generations${toQuery(params)}`)
  },
  getGeneration(id) {
    return request(`/api/admin/generations/${id}`)
  },
  generationExportURL(params = {}) {
    return apiURL(`/api/admin/generations/export${toQuery(params)}`)
  },
  listVideoGenerations(params = {}) {
    return request(`/api/admin/video-generations${toQuery(params)}`)
  },
  getAdminVideoGeneration(id) {
    return request(`/api/admin/video-generations/${id}`)
  },
  videoGenerationExportURL(params = {}) {
    return apiURL(`/api/admin/video-generations/export${toQuery(params)}`)
  },
  listContentReviews(params = {}) {
    return request(`/api/admin/content-reviews${toQuery(params)}`)
  },
  updateContentReview(id, payload) {
    return request(`/api/admin/content-reviews/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(payload)
    })
  },
  listContentReports(params = {}) {
    return request(`/api/admin/content-reports${toQuery(params)}`)
  },
  getAlgorithmDisclosure() {
    return request('/api/admin/algorithm-disclosure')
  },
  updateAlgorithmDisclosure(payload) {
    return request('/api/admin/algorithm-disclosure', {
      method: 'PATCH',
      body: JSON.stringify(payload)
    })
  },
  algorithmComplianceExportURL() {
    return apiURL('/api/admin/algorithm-compliance/export')
  },
  listAlgorithmIncidents(params = {}) {
    return request(`/api/admin/incidents${toQuery(params)}`)
  },
  createAlgorithmIncident(payload) {
    return request('/api/admin/incidents', {
      method: 'POST',
      body: JSON.stringify(payload)
    })
  },
  listAdminUsers(params = {}) {
    return request(`/api/admin/users${toQuery(params)}`)
  },
  listAdminCreditTransactions(params = {}) {
    return request(`/api/admin/credit-transactions${toQuery(params)}`)
  },
  addAdminCredits(id, payload) {
    return request(`/api/admin/users/${id}/credits`, {
      method: 'POST',
      body: JSON.stringify(payload)
    })
  },
  adjustAdminCredits(id, payload) {
    return request(`/api/admin/users/${id}/credit-adjustments`, {
      method: 'POST',
      body: JSON.stringify(payload)
    })
  },
  updateAdminUserWechatBinding(id, payload) {
    return request(`/api/admin/users/${id}/wechat-binding`, {
      method: 'PATCH',
      body: JSON.stringify(payload)
    })
  },
  deleteAdminUserWechatBinding(id, payload = {}) {
    return request(`/api/admin/users/${id}/wechat-binding`, {
      method: 'DELETE',
      body: JSON.stringify(payload)
    })
  },
  deleteAdminUserPhoneBinding(id, payload = {}) {
    return request(`/api/admin/users/${id}/phone-binding`, {
      method: 'DELETE',
      body: JSON.stringify(payload)
    })
  },
  resetAdminUserPassword(id, payload) {
    return request(`/api/admin/users/${id}/reset-password`, {
      method: 'POST',
      body: JSON.stringify(payload)
    })
  },
  deleteAdminUser(id) {
    return request(`/api/admin/users/${id}`, {
      method: 'DELETE',
      body: JSON.stringify({})
    })
  },
  batchDeleteAdminUsers(userIds) {
    return request('/api/admin/users/batch-delete', {
      method: 'POST',
      body: JSON.stringify({ user_ids: userIds })
    })
  },
  listAdminPackages() {
    return request('/api/admin/packages')
  },
  createAdminPackage(payload) {
    return request('/api/admin/packages', {
      method: 'POST',
      body: JSON.stringify(payload)
    })
  },
  updateAdminPackage(id, payload) {
    return request(`/api/admin/packages/${id}`, {
      method: 'PUT',
      body: JSON.stringify(payload)
    })
  },
  deleteAdminPackage(id) {
    return request(`/api/admin/packages/${id}`, {
      method: 'DELETE',
      body: JSON.stringify({})
    })
  },
  listFinanceOrders(params = {}) {
    return request(`/api/admin/finance-orders${toQuery(params)}`)
  },
  getFinanceOrder(id) {
    return request(`/api/admin/finance-orders/${id}`)
  },
  syncFinanceOrderPayment(id) {
    return request(`/api/admin/finance-orders/${id}/sync-payment`, {
      method: 'POST',
      body: JSON.stringify({})
    })
  },
  financeOrdersExportURL(params = {}) {
    return apiURL(`/api/admin/finance-orders/export${toQuery(params)}`)
  },
  updateFinanceRefund(id, payload) {
    return request(`/api/admin/finance-refunds/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(payload)
    })
  },
  updateFinanceInvoice(id, payload) {
    return request(`/api/admin/finance-invoices/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(payload)
    })
  },
  createAnnouncement(payload) {
    return request('/api/admin/announcements', {
      method: 'POST',
      body: JSON.stringify(payload)
    })
  },
  listAnnouncements(params = {}) {
    return request(`/api/admin/announcements${toQuery(params)}`)
  },
  updateAnnouncement(id, payload) {
    return request(`/api/admin/announcements/${id}`, {
      method: 'PUT',
      body: JSON.stringify(payload)
    })
  },
  updateAnnouncementStatus(id, status) {
    return request(`/api/admin/announcements/${id}/status`, {
      method: 'PATCH',
      body: JSON.stringify({ status })
    })
  },
  listAdminAccounts() {
    return request('/api/admin/admin-users')
  },
  createAdminAccount(payload) {
    return request('/api/admin/admin-users', {
      method: 'POST',
      body: JSON.stringify(payload)
    }).finally(() => {
      adminSessionCache = null
    })
  },
  updateAdminAccount(id, payload) {
    return request(`/api/admin/admin-users/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(payload)
    }).finally(() => {
      adminSessionCache = null
    })
  },
  updateAdminAccountRoles(id, payload) {
    return request(`/api/admin/admin-users/${id}/roles`, {
      method: 'PUT',
      body: JSON.stringify(payload)
    }).finally(() => {
      adminSessionCache = null
    })
  },
  resetAdminAccountPassword(id, payload) {
    return request(`/api/admin/admin-users/${id}/reset-password`, {
      method: 'POST',
      body: JSON.stringify(payload)
    }).finally(() => {
      adminSessionCache = null
    })
  },
  listAdminRoles() {
    return request('/api/admin/roles')
  },
  createAdminRole(payload) {
    return request('/api/admin/roles', {
      method: 'POST',
      body: JSON.stringify(payload)
    }).finally(() => {
      adminSessionCache = null
    })
  },
  updateAdminRole(id, payload) {
    return request(`/api/admin/roles/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(payload)
    }).finally(() => {
      adminSessionCache = null
    })
  },
  updateAdminRolePermissions(id, payload) {
    return request(`/api/admin/roles/${id}/permissions`, {
      method: 'PUT',
      body: JSON.stringify(payload)
    }).finally(() => {
      adminSessionCache = null
    })
  }
}

export async function ensureUserSession() {
  try {
    await api.getMe()
    return true
  } catch {
    return false
  }
}

export async function getCurrentAdminSession(force = false) {
  if (adminSessionCache && !force) {
    return adminSessionCache
  }
  return api.getAdminMe()
}

export async function ensureAdminSession(permission) {
  try {
    const payload = await getCurrentAdminSession()
    const permissions = payload.permissions ?? []
    if (permission && !permissions.includes(permission) && adminSessionCache) {
      const refreshedPayload = await getCurrentAdminSession(true)
      const refreshedPermissions = refreshedPayload.permissions ?? []
      return {
        authenticated: true,
        authorized: refreshedPermissions.includes(permission)
      }
    }
    return {
      authenticated: true,
      authorized: !permission || permissions.includes(permission)
    }
  } catch {
    adminSessionCache = null
    return { authenticated: false, authorized: false }
  }
}
