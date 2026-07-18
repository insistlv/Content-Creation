import { getCurrentInstance, onBeforeUnmount, ref } from 'vue'
import { api } from '../api/client.js'
import { commerceUserMessage } from '../components/ecommerce/commerceUserMessages.js'

const active = (status) => ['queued', 'retrying', 'running', 'canceling'].includes(status)
const snapshotStatus = (status) => active(status) || ['canceled', 'partial_succeeded', 'succeeded', 'failed'].includes(status)
const itemsOf = (value) => Array.isArray(value) ? value : (value?.items || [])

export function useCommerceBatches() {
  const batches = ref([])
  const events = ref([])
  const loading = ref(false)
  const error = ref('')
  const afterIds = new Map()
  let timer = null
  let projectId = null
  let generation = 0

  function clearPoll() { if (timer) { window.clearTimeout(timer); timer = null } }
  function stop() { generation += 1; clearPoll() }
  function reset() {
    stop()
    projectId = null
    batches.value = []
    events.value = []
    loading.value = false
    error.value = ''
    afterIds.clear()
  }
  async function poll(expectedGeneration = generation) {
    clearPoll()
    const currentProject = projectId
    if (!currentProject) return
    loading.value = true
    try {
      const listed = itemsOf(await api.listCommerceBatches(currentProject))
      if (expectedGeneration !== generation || currentProject !== projectId) return
      const snapshots = await Promise.all(listed.map(async (batch) => {
        if (!snapshotStatus(batch.status)) return batch
        const snapshot = await api.getCommerceBatch(batch.id)
        return { ...batch, ...(snapshot?.batch || {}), items: snapshot?.items || batch.items || [] }
      }))
      if (expectedGeneration !== generation || currentProject !== projectId) return
      batches.value = snapshots
      for (const batch of snapshots.filter((item) => active(item.status))) {
        const response = await api.listCommerceBatchEvents(batch.id, { after_id: afterIds.get(batch.id) || 0 })
        if (expectedGeneration !== generation || currentProject !== projectId) return
        const next = itemsOf(response)
        events.value.push(...next)
        if (next.length) afterIds.set(batch.id, next.at(-1).id)
      }
      error.value = ''
    } catch (reason) {
      if (expectedGeneration === generation) error.value = commerceUserMessage(reason, '批次加载失败，请稍后重试')
    } finally {
      if (expectedGeneration === generation) {
        loading.value = false
        timer = window.setTimeout(() => poll(expectedGeneration), document.visibilityState === 'hidden' ? 10000 : 2000)
      }
    }
  }
  function start(id) {
    stop()
    projectId = id
    batches.value = []
    events.value = []
    afterIds.clear()
    const current = generation
    return poll(current)
  }
  if (getCurrentInstance()) onBeforeUnmount(stop)
  return { batches, events, loading, error, start, stop, reset, poll, clearPoll }
}
