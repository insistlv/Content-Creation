import { ref } from 'vue'

import { api } from '../api/client.js'

export const currentUser = ref(null)
export const currentUserLoading = ref(false)

let currentUserPromise = null

function isObjectPayload(payload) {
  return payload && typeof payload === 'object' && !Array.isArray(payload)
}

function isSessionExpiredError(error) {
  return error?.status === 401 || error?.code === 'session_expired'
}

export function setCurrentUser(payload) {
  currentUser.value = isObjectPayload(payload) ? payload : null
  return currentUser.value
}

export function clearCurrentUser() {
  currentUser.value = null
  currentUserLoading.value = false
  currentUserPromise = null
}

export function applyAvailableCredits(value) {
  if (value === undefined || value === null) {
    return currentUser.value
  }

  const numericValue = Number(value)
  const availableCredits = Number.isFinite(numericValue) ? numericValue : value
  currentUser.value = {
    ...(currentUser.value ?? {}),
    available_credits: availableCredits
  }
  return currentUser.value
}

export async function loadCurrentUser(options = {}) {
  const { force = false, clearOnError = true } = options
  if (currentUserPromise && !force) {
    return currentUserPromise
  }

  currentUserLoading.value = true
  currentUserPromise = api.getMe()
    .then((payload) => {
      if (isObjectPayload(payload)) {
        setCurrentUser(payload)
      }
      return currentUser.value
    })
    .catch((error) => {
      if (clearOnError) {
        clearCurrentUser()
      }
      throw error
    })
    .finally(() => {
      currentUserLoading.value = false
      currentUserPromise = null
    })

  return currentUserPromise
}

export async function refreshCurrentUser() {
  try {
    return await loadCurrentUser({ force: true, clearOnError: false })
  } catch {
    return currentUser.value
  }
}

export async function ensureUserSession() {
  try {
    const payload = await loadCurrentUser({ force: true, clearOnError: false })
    return Boolean(payload)
  } catch (error) {
    if (isSessionExpiredError(error)) {
      clearCurrentUser()
      return false
    }
    return Boolean(currentUser.value)
  }
}
