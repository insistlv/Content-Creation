import { onBeforeUnmount, onMounted, watch } from 'vue'

import { api } from '../api/client.js'

export const PRESENCE_HEARTBEAT_INTERVAL_MS = 60_000

const heartbeatSources = new Set()
let heartbeatTimer = null
let visibilityListenerInstalled = false

function pageIsVisible() {
  return typeof document === 'undefined' || document.visibilityState !== 'hidden'
}

function anySourceEnabled() {
  for (const source of heartbeatSources) {
    if (source.value) return true
  }
  return false
}

function stopHeartbeat() {
  if (heartbeatTimer) {
    window.clearInterval(heartbeatTimer)
    heartbeatTimer = null
  }
}

async function pingHeartbeat() {
  if (!anySourceEnabled() || !pageIsVisible()) return
  try {
    await api.pingPresence()
  } catch {
    // Heartbeats are best-effort; auth flows and page rendering should continue.
  }
}

function startHeartbeat() {
  if (!anySourceEnabled() || !pageIsVisible() || heartbeatTimer) return
  void pingHeartbeat()
  heartbeatTimer = window.setInterval(() => {
    void pingHeartbeat()
  }, PRESENCE_HEARTBEAT_INTERVAL_MS)
}

function syncHeartbeat() {
  if (anySourceEnabled() && pageIsVisible()) {
    startHeartbeat()
    return
  }
  stopHeartbeat()
}

function handleVisibilityChange() {
  if (pageIsVisible()) {
    startHeartbeat()
    return
  }
  stopHeartbeat()
}

function installVisibilityListener() {
  if (visibilityListenerInstalled || typeof document === 'undefined') return
  document.addEventListener('visibilitychange', handleVisibilityChange)
  visibilityListenerInstalled = true
}

function removeVisibilityListenerIfUnused() {
  if (!visibilityListenerInstalled || heartbeatSources.size > 0 || typeof document === 'undefined') return
  document.removeEventListener('visibilitychange', handleVisibilityChange)
  visibilityListenerInstalled = false
}

export function usePresenceHeartbeat(enabled) {
  onMounted(() => {
    heartbeatSources.add(enabled)
    installVisibilityListener()
    syncHeartbeat()
  })

  onBeforeUnmount(() => {
    heartbeatSources.delete(enabled)
    syncHeartbeat()
    removeVisibilityListenerIfUnused()
  })

  watch(
    () => Boolean(enabled.value),
    () => {
      syncHeartbeat()
    }
  )
}
