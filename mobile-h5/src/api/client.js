const authTokenStorageKey = 'image_agent_auth_token'
const authExpiresAtStorageKey = 'image_agent_auth_expires_at'

const clientHeaders = {}

// Add client identifier for WeChat Mini Program
// #ifdef MP-WEIXIN
clientHeaders['X-Image-Agent-Client'] = 'mp-weixin'
// #endif

const jsonHeaders = {
  ...clientHeaders,
  'Content-Type': 'application/json'
}

const requestTimeoutMS = Number(import.meta.env.VITE_API_TIMEOUT_MS || 10000)
const uploadTimeoutMS = Number(import.meta.env.VITE_API_UPLOAD_TIMEOUT_MS || 60000)

export class ApiError extends Error {
  constructor(code, message, status, details = {}) {
    super(message)
    this.code = code
    this.status = status
    Object.assign(this, details)
  }
}

function parsePayload(payload) {
  if (!payload) return {}
  if (typeof payload === 'object') return payload ?? {}
  try {
    return JSON.parse(payload)
  } catch {
    return {}
  }
}

function normalizeError(payload, status) {
  const error = payload?.error ?? {}
  const details = { ...(payload || {}), ...(error || {}) }
  delete details.error
  return new ApiError(error.code ?? 'request_failed', error.message ?? '请求失败', status, details)
}

function buildQuery(params = {}) {
  const text = Object.entries(params)
    .filter(([, value]) => value !== undefined && value !== null && value !== '')
    .map(([key, value]) => `${encodeURIComponent(key)}=${encodeURIComponent(`${value}`)}`)
    .join('&')
  return text ? `?${text}` : ''
}

const mpAPIBaseURL = import.meta.env.VITE_API_BASE_URL || 'https://example.com'
let networkHelpText = `请确认后端服务可访问：${mpAPIBaseURL}`

// #ifdef MP-WEIXIN
networkHelpText = `请确认后端服务可访问，微信开发者工具模拟器当前访问 ${mpAPIBaseURL}`
// #endif

function joinURL(baseURL, path) {
  return `${baseURL.replace(/\/+$/, '')}/${path.replace(/^\/+/, '')}`
}

function apiURL(path) {
  if (/^https?:\/\//i.test(path)) return path
  // #ifdef MP-WEIXIN
  return joinURL(mpAPIBaseURL, path)
  // #endif
  return path
}

export function getStoredAuthToken() {
  // #ifdef MP-WEIXIN
  return `${uni.getStorageSync(authTokenStorageKey) || ''}`.trim()
  // #endif
  return ''
}

export function saveAuthToken(payload) {
  const token = `${payload?.auth_token || ''}`.trim()
  if (!token) return
  // #ifdef MP-WEIXIN
  uni.setStorageSync(authTokenStorageKey, token)
  if (payload?.auth_expires_at) {
    uni.setStorageSync(authExpiresAtStorageKey, payload.auth_expires_at)
  }
  // #endif
}

export function clearAuthToken() {
  // #ifdef MP-WEIXIN
  uni.removeStorageSync(authTokenStorageKey)
  uni.removeStorageSync(authExpiresAtStorageKey)
  // #endif
}

function buildHeaders(headers = jsonHeaders) {
  const nextHeaders = { ...(headers ?? jsonHeaders) }
  const authToken = getStoredAuthToken()
  if (authToken) {
    nextHeaders.Authorization = `Bearer ${authToken}`
  }
  return nextHeaders
}

function formatNetworkError(error, fallback) {
  const errMsg = error?.errMsg || ''
  if (errMsg.toLowerCase().includes('timeout')) {
    return `${fallback}：请求超时，${networkHelpText}`
  }
  if (errMsg) {
    return `${fallback}：${errMsg}。${networkHelpText}`
  }
  return `${fallback}。${networkHelpText}`
}

export function request(path, options = {}) {
  return new Promise((resolve, reject) => {
    uni.request({
      url: apiURL(path),
      method: options.method ?? 'GET',
      data: options.data,
      header: buildHeaders(options.header),
      timeout: requestTimeoutMS,
      withCredentials: true,
      success(response) {
        const status = response.statusCode
        const payload = typeof response.data === 'string' ? parsePayload(response.data) : response.data
        if (status >= 200 && status < 300) {
          resolve(payload ?? {})
          return
        }
        if (status === 401) {
          clearAuthToken()
        }
        reject(normalizeError(payload, status))
      },
      fail(error) {
        reject(new ApiError('network_error', formatNetworkError(error, '网络请求失败'), 0))
      }
    })
  })
}

function fileNameFromPath(path) {
  const fallback = 'reference-image.png'
  if (!path) return fallback
  return decodeURIComponent(`${path}`.split('/').pop() || fallback)
}

function mimeTypeFromFilename(filename) {
  const lower = `${filename || ''}`.toLowerCase()
  if (lower.endsWith('.jpg') || lower.endsWith('.jpeg')) return 'image/jpeg'
  if (lower.endsWith('.webp')) return 'image/webp'
  return 'image/png'
}

function uploadInputDetails(input) {
  const isFileLike = typeof Blob !== 'undefined' && input instanceof Blob
  const file = isFileLike ? input : input?.file
  const filePath = typeof input === 'string' ? input : (input?.path ?? input?.filePath ?? file?.path ?? '')
  const filename = input?.filename ?? input?.name ?? file?.name ?? fileNameFromPath(filePath)
  const mimeType = input?.mime_type ?? input?.mimeType ?? file?.type ?? mimeTypeFromFilename(filename)
  const size = Number(input?.size ?? file?.size ?? 0)
  return {
    file,
    filePath,
    filename,
    mimeType,
    size
  }
}

function appendOSSFormFields(formData, fields = {}) {
  Object.entries(fields).forEach(([key, value]) => {
    if (value !== undefined && value !== null) {
      formData.append(key, value)
    }
  })
}

function uploadReferenceAssetWithFetch(policy, file) {
  const formData = new FormData()
  appendOSSFormFields(formData, policy.form_data)
  formData.append('file', file)
  return fetch(policy.upload_url, {
    method: 'POST',
    body: formData
  }).then((response) => {
    if (response.ok) return {}
    throw new ApiError('upload_failed', '上传失败：OSS 直传失败', response.status)
  }).catch((error) => {
    if (error instanceof ApiError) throw error
    throw new ApiError('upload_failed', formatNetworkError(error, '上传失败'), 0)
  })
}

function uploadReferenceAssetWithUni(policy, filePath) {
  return new Promise((resolve, reject) => {
    if (!filePath) {
      reject(new ApiError('upload_file_missing', '请选择图片文件后再上传', 0))
      return
    }

    const uploadOptions = {
      url: policy.upload_url,
      name: 'file',
      formData: { ...(policy.form_data ?? {}) },
      header: {},
      timeout: uploadTimeoutMS,
      success(response) {
        const status = response.statusCode
        if (status >= 200 && status < 300) {
          resolve({})
          return
        }
        reject(new ApiError('upload_failed', '上传失败：OSS 直传失败', status))
      },
      fail(error) {
        reject(new ApiError('upload_failed', formatNetworkError(error, '上传失败'), 0))
      }
    }
    uploadOptions.filePath = filePath
    uni.uploadFile(uploadOptions)
  })
}

function uploadReferenceAssetToOSS(policy, upload) {
  // #ifdef H5
  if (upload.file && typeof Blob !== 'undefined' && upload.file instanceof Blob && typeof FormData !== 'undefined' && typeof fetch !== 'undefined') {
    return uploadReferenceAssetWithFetch(policy, upload.file)
  }
  // #endif
  return uploadReferenceAssetWithUni(policy, upload.filePath)
}

export function uploadReferenceAsset(input) {
  const upload = uploadInputDetails(input)
  if (!upload.filePath && !upload.file) {
    return Promise.reject(new ApiError('upload_file_missing', '请选择图片文件后再上传', 0))
  }

  return request('/api/reference-assets/upload-policy', {
    method: 'POST',
    data: {
      filename: upload.filename,
      mime_type: upload.mimeType,
      size: upload.size
    }
  }).then((policy) => uploadReferenceAssetToOSS(policy, upload)
    .then(() => request('/api/reference-assets/complete-upload', {
      method: 'POST',
      data: {
        object_key: policy.object_key,
        upload_token: policy.upload_token
      }
    })))
}

export const api = {
  register(input) {
    return request('/api/auth/register', {
      method: 'POST',
      data: input
    }).then((payload) => {
      saveAuthToken(payload)
      return payload
    })
  },
  sendSMSCode(input) {
    return request('/api/auth/sms-code', {
      method: 'POST',
      data: input
    })
  },
  registerPhone(input) {
    return request('/api/auth/register-phone', {
      method: 'POST',
      data: input
    }).then((payload) => {
      saveAuthToken(payload)
      return payload
    })
  },
  resetPassword(input) {
    return request('/api/auth/reset-password', {
      method: 'POST',
      data: input
    })
  },
  login(input) {
    return request('/api/auth/login', {
      method: 'POST',
      data: input
    }).then((payload) => {
      saveAuthToken(payload)
      return payload
    })
  },
  wechatLogin(input) {
    return request('/api/auth/wechat-login', {
      method: 'POST',
      data: input
    }).then((payload) => {
      saveAuthToken(payload)
      return payload
    })
  },
  wechatPhoneLogin(input) {
    return request('/api/auth/wechat-phone-login', {
      method: 'POST',
      data: input
    }).then((payload) => {
      saveAuthToken(payload)
      return payload
    })
  },
  wechatBind(input) {
    return request('/api/auth/wechat-bind', {
      method: 'POST',
      data: input
    })
  },
  logout() {
    return request('/api/auth/logout', {
      method: 'POST'
    }).then((payload) => {
      clearAuthToken()
      return payload
    })
  },
  pingPresence() {
    return request('/api/account/presence')
  },
  getMe() {
    return request('/api/me')
  },
  listPopupAnnouncements(client = 'mp-weixin') {
    return request(`/api/announcements/popup${buildQuery({ client })}`)
  },
  dismissAnnouncement(id, client = 'mp-weixin') {
    return request(`/api/announcements/${id}/dismiss`, {
      method: 'POST',
      data: { client }
    })
  },
  getCredits() {
    return request('/api/account/credits')
  },
  getCreditTransactions(params = {}) {
    return request(`/api/account/credit-transactions${buildQuery(params)}`)
  },
  bindAccountPhone(input) {
    return request('/api/account/phone', {
      method: 'POST',
      data: input
    })
  },
  bindWechatPhone(input) {
    return request('/api/account/wechat-phone', {
      method: 'POST',
      data: input
    })
  },
  updateProfile(input) {
    return request('/api/account/profile', {
      method: 'PATCH',
      data: input
    })
  },
  updateAccountEmail(input) {
    return request('/api/account/email', {
      method: 'PATCH',
      data: input
    })
  },
  updateAccountPreferences(input) {
    return request('/api/account/preferences', {
      method: 'PATCH',
      data: input
    })
  },
  changePassword(input) {
    return request('/api/account/password', {
      method: 'POST',
      data: input
    })
  },
  setPaymentPassword(input) {
    return request('/api/account/payment-password', {
      method: 'POST',
      data: input
    })
  },
  clearPaymentPassword(input) {
    return request('/api/account/payment-password', {
      method: 'DELETE',
      data: input
    })
  },
  getAccountSessions() {
    return request('/api/account/sessions')
  },
  deleteAccountSession(id) {
    return request(`/api/account/sessions/${id}`, {
      method: 'DELETE'
    })
  },
  deleteOtherAccountSessions() {
    return request('/api/account/sessions/others', {
      method: 'DELETE'
    })
  },
  getCustomerService() {
    return request('/api/customer-service')
  },
  getPackages() {
    return request('/api/packages')
  },
  createWechatVirtualPayOrder(input) {
    return request('/api/payments/wechat/virtual-orders', {
      method: 'POST',
      data: input
    })
  },
  confirmWechatVirtualPayOrder(orderNumber) {
    return request(`/api/payments/wechat/virtual-orders/${orderNumber}/confirm`, {
      method: 'POST'
    })
  },
  listWorks(params = {}) {
    return request(`/api/works${buildQuery(params)}`)
  },
  listPromptTemplates(params = {}) {
    return request(`/api/prompt-templates${buildQuery(params)}`)
  },
  usePromptTemplate(id) {
    return request(`/api/prompt-templates/${id}/use`, {
      method: 'POST'
    })
  },
  createCoupleAlbum(input) {
    return request('/api/couple-albums', {
      method: 'POST',
      data: input
    })
  },
  estimateCoupleAlbum(input) {
    return request('/api/couple-albums/estimate', {
      method: 'POST',
      data: input
    })
  },
  getCoupleAlbumOptions() {
    return request('/api/couple-album/options')
  },
  generateCoupleAlbum(id) {
    return request(`/api/couple-albums/${id}/generate`, {
      method: 'POST'
    })
  },
  getCoupleAlbum(id) {
    return request(`/api/couple-albums/${id}`)
  },
  listCoupleAlbums(params = {}) {
    return request(`/api/couple-albums${buildQuery(params)}`)
  },
  retryCoupleAlbumPage(albumID, pageID) {
    return request(`/api/couple-albums/${albumID}/pages/${pageID}/retry`, {
      method: 'POST'
    })
  },
  shareCoupleAlbum(id) {
    return request(`/api/couple-albums/${id}/share`, {
      method: 'POST'
    })
  },
  getPublicCoupleAlbum(token) {
    return request(`/api/public/couple-albums/${token}`)
  },
  getPublicWorks(params = {}) {
    return request(`/api/public/works${buildQuery(params)}`)
  },
  assetURL(path) {
    return apiURL(path)
  },
  getWork(id) {
    return request(`/api/works/${id}`)
  },
  updateWork(id, input) {
    return request(`/api/works/${id}`, {
      method: 'PATCH',
      data: input
    })
  },
  deleteWork(id) {
    return request(`/api/works/${id}`, {
      method: 'DELETE'
    })
  },
  reuseWork(id) {
    return request(`/api/works/${id}/reuse`, {
      method: 'POST'
    })
  },
  createImageGeneration(input) {
    return request('/api/images/generations/async', {
      method: 'POST',
      data: input
    })
  },
  estimateImageGeneration(input) {
    return request('/api/images/generations/estimate', {
      method: 'POST',
      data: input
    })
  },
  optimizePrompt(input) {
    return request('/api/prompts/optimize', {
      method: 'POST',
      data: input
    })
  },
  getImageGeneration(id) {
    return request(`/api/images/generations/${id}`)
  },
  uploadReferenceAsset
}
