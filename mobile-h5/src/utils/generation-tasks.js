const pendingGenerationStorageKey = 'image-agent:pending-generations'

function readStorage() {
  try {
    const value = uni.getStorageSync(pendingGenerationStorageKey)
    return Array.isArray(value) ? value : []
  } catch {
    return []
  }
}

function writeStorage(items) {
  try {
    uni.setStorageSync(pendingGenerationStorageKey, items)
  } catch {
    // Storage failure should not block generation.
  }
}

export function loadPendingGenerations() {
  return readStorage()
}

export function savePendingGenerations(items) {
  writeStorage(items)
}

export function addPendingGenerations(items) {
  const current = readStorage()
  const byID = new Map(current.map((item) => [item.generation_id, item]))
  items.forEach((item) => {
    if (item?.generation_id) {
      byID.set(item.generation_id, { ...byID.get(item.generation_id), ...item })
    }
  })
  writeStorage(Array.from(byID.values()))
}

export function updatePendingGeneration(id, patch) {
  const next = readStorage().map((item) => (item.generation_id === id ? { ...item, ...patch } : item))
  writeStorage(next)
  return next
}

export function removePendingGenerations(ids) {
  const idSet = new Set(ids)
  const next = readStorage().filter((item) => !idSet.has(item.generation_id))
  writeStorage(next)
  return next
}

export function stageProgress(stage, status) {
  if (status === 'succeeded') return 100
  if (status === 'failed') return 100
  if (stage === 'persisting_result') return 86
  if (stage === 'requesting_provider') return 58
  if (stage === 'queued') return 16
  return 36
}
