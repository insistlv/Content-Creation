<script setup>
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { X } from 'lucide-vue-next'

const visibleAlerts = ref([])
const cooldownByKey = new Map()
const timersById = new Map()
const cooldownMs = 8000
const autoDismissMs = 6000
let nextAlertId = 1

function alertKey(detail = {}) {
  return `${detail.method || 'GET'} ${detail.path || ''}`
}

function removeAlert(id) {
  const timer = timersById.get(id)
  if (timer) {
    clearTimeout(timer)
    timersById.delete(id)
  }
  visibleAlerts.value = visibleAlerts.value.filter((alert) => alert.id !== id)
}

function pushAlert(detail = {}) {
  if (detail.code !== 'network_unreachable') return

  const key = alertKey(detail)
  const now = Date.now()
  const lastShownAt = cooldownByKey.get(key) || 0
  if (now - lastShownAt < cooldownMs) return
  cooldownByKey.set(key, now)

  const id = nextAlertId
  nextAlertId += 1
  const nextAlerts = [
    {
      id,
      message: detail.message || '网络连接不稳定，暂时无法连接服务器，请稍后重试',
      method: detail.method || 'GET',
      path: detail.path || '',
      online: detail.online !== false
    },
    ...visibleAlerts.value
  ]
  const keptAlerts = nextAlerts.slice(0, 3)
  nextAlerts.slice(3).forEach((alert) => {
    const timer = timersById.get(alert.id)
    if (timer) {
      clearTimeout(timer)
      timersById.delete(alert.id)
    }
  })
  visibleAlerts.value = keptAlerts

  const timer = setTimeout(() => removeAlert(id), autoDismissMs)
  timersById.set(id, timer)
}

function handleNetworkError(event) {
  pushAlert(event.detail || {})
}

onMounted(() => {
  window.addEventListener('image-agent:network-error', handleNetworkError)
})

onBeforeUnmount(() => {
  window.removeEventListener('image-agent:network-error', handleNetworkError)
  timersById.forEach((timer) => clearTimeout(timer))
  timersById.clear()
  cooldownByKey.clear()
})
</script>

<template>
  <Teleport to="body">
    <div v-if="visibleAlerts.length" class="global-network-alerts" aria-live="assertive">
      <div
        v-for="alert in visibleAlerts"
        :key="alert.id"
        class="global-network-alert"
        role="alert"
        aria-live="assertive"
        data-testid="global-network-alert"
      >
        <div class="global-network-alert-copy">
          <strong>{{ alert.online ? '服务器连接失败' : '当前网络已断开' }}</strong>
          <p>{{ alert.message }}</p>
          <small v-if="alert.path">{{ alert.method }} {{ alert.path }}</small>
        </div>
        <button
          class="global-network-alert-close"
          type="button"
          aria-label="关闭网络提示"
          data-testid="global-network-alert-close"
          @click="removeAlert(alert.id)"
        >
          <X :size="16" aria-hidden="true" />
        </button>
      </div>
    </div>
  </Teleport>
</template>
