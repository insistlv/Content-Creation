<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { onReachBottom } from '@dcloudio/uni-app'

import { api, getStoredAuthToken } from '../../api/client.js'
import AnnouncementPopup from '../../components/AnnouncementPopup.vue'
import AppTabbar from '../../components/AppTabbar.vue'
import {
  loadPendingGenerations,
  removePendingGenerations,
  savePendingGenerations,
  stageProgress
} from '../../utils/generation-tasks.js'
import { enableMiniProgramShare, navigateTo, requireAuth, routes } from '../../utils/routes.js'

const staticAssetBaseURL = `${import.meta.env.VITE_STATIC_ASSET_BASE_URL || ''}`.replace(/\/+$/, '')

function staticAsset(path) {
  const normalizedPath = `${path || ''}`.trim().replace(/^\/+/, '').replace(/^static\/+/i, '')
  if (!normalizedPath) return staticAssetBaseURL
  if (staticAssetBaseURL) return `${staticAssetBaseURL}/${normalizedPath}`
  return `/${['static', normalizedPath].join('/')}`
}

function staticIcon(name) {
  const normalizedName = `${name || ''}`.trim().replace(/\.png$/i, '')
  return staticAsset(`icons/${normalizedName}.png`)
}

const icon = staticIcon

const tabs = [
  { key: 'all', label: '全部' },
  { key: 'image', label: '图片' },
  { key: 'video', label: '视频' },
  { key: 'poster_kv', label: '海报KV' },
  { key: 'product_main', label: '商品主图' },
  { key: 'cover', label: '封面' },
  { key: 'favorites', label: '收藏' }
]

const statusOptions = [
  { value: 'all', label: '全部状态' },
  { value: 'succeeded', label: '已完成' },
  { value: 'running', label: '生成中' },
  { value: 'failed', label: '失败' }
]

const timeOptions = [
  { value: 'all', label: '全部时间' },
  { value: 'today', label: '今天' },
  { value: 'week', label: '近7天' },
  { value: 'month', label: '近30天' }
]

const sortOptions = [
  { value: 'latest', label: '排序' },
  { value: 'oldest', label: '最早创建' }
]

const layoutStorageKey = 'image-agent-works-layout'
const missingPublicShareSupportMessage = '当前服务版本不支持公开分享，请重启后端服务后再试'
const downloadAPIBaseURL = `${import.meta.env.VITE_API_BASE_URL || 'https://example.com'}`.replace(/\/+$/, '')
const downloadTimeoutMS = 20000
const worksPageSize = 50

const activeTab = ref('all')
const timeFilter = ref('all')
const statusFilter = ref('all')
const sortFilter = ref('latest')
const searchVisible = ref(false)
const searchInput = ref('')
const searchQuery = ref('')
const layoutMode = ref('list')
const works = ref([])
const coupleAlbums = ref([])
const summary = ref(null)
const pendingGenerations = ref([])
const loading = ref(false)
const loadingMore = ref(false)
const currentPage = ref(1)
const totalWorks = ref(0)
const hasMore = ref(false)
const errorMessage = ref('')
const previewOverlayVisible = ref(false)
const previewImages = ref([])
const previewIndex = ref(0)
const previewTitle = ref('')
const sharePanelVisible = ref(false)
const sharePanelWork = ref(null)
const albumSharingID = ref('')

let pendingPollTimer = null

const pendingWorkItems = computed(() =>
  pendingGenerations.value.map((task) => {
    const inputMode = pendingGenerationInputMode(task)
    return {
      ...task,
      id: `pending-${task.generation_id}`,
      generation_status: task.status,
      status: task.status,
      mode: inputMode,
      tool_mode: inputMode,
      reference_preview_urls: inputMode === 'image' ? task.reference_preview_urls || [] : [],
      image_count: task.image_count || 1,
      progress: task.progress ?? stageProgress(task.stage, task.status)
    }
  })
)

const displayWorks = computed(() => [...pendingWorkItems.value, ...works.value])
const groupedDisplayWorks = computed(() => groupWorksByBatch(displayWorks.value))
const albumCards = computed(() => coupleAlbums.value.map(buildAlbumCard))
const visibleAlbumCards = computed(() => (activeTab.value === 'all' ? albumCards.value : []))

const filteredWorks = computed(() => {
  const filtered = [...visibleAlbumCards.value, ...groupedDisplayWorks.value].filter((work) => matchesLocalFilters(work))
  return [...filtered].sort((left, right) => {
    const leftTime = Date.parse(left.created_at || left.updated_at || left.completed_at || '') || 0
    const rightTime = Date.parse(right.created_at || right.updated_at || right.completed_at || '') || 0
    return sortFilter.value === 'oldest' ? leftTime - rightTime : rightTime - leftTime
  })
})

enableMiniProgramShare(({ event } = {}) => workSharePayload(event))

function normalizeWorksPayload(payload) {
  if (Array.isArray(payload)) return payload
  if (Array.isArray(payload?.items)) return payload.items
  if (Array.isArray(payload?.works)) return payload.works
  if (Array.isArray(payload?.data)) return payload.data
  return []
}

function normalizeCoupleAlbumsPayload(payload) {
  if (Array.isArray(payload)) return payload
  if (Array.isArray(payload?.albums)) return payload.albums
  if (Array.isArray(payload?.items)) return payload.items
  if (Array.isArray(payload?.data)) return payload.data
  return []
}

function listQueryParams(page = 1) {
  const params = {
    page,
    page_size: worksPageSize,
    q: searchQuery.value,
    sort: sortFilter.value,
    exclude_album_pages: true
  }
  if (statusFilter.value !== 'all') {
    params.status = statusFilter.value
  }
  if (timeFilter.value !== 'all') {
    params.time_range = timeFilter.value
  }
  if (activeTab.value === 'favorites') {
    params.favorite = true
  } else if (activeTab.value !== 'all') {
    params.category = activeTab.value
  }
  return params
}

function mergeWorks(previousItems, nextItems) {
  const seen = new Set()
  return [...previousItems, ...nextItems].filter((item) => {
    const key = `${workID(item) || item.generation_id || item.created_at || ''}`
    if (!key) return true
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
}

function groupWorksByBatch(items) {
  const groups = new Map()
  const singles = []
  items.forEach((item) => {
    const batchID = `${item?.batch_id || ''}`.trim()
    if (!batchID) {
      singles.push(item)
      return
    }
    if (!groups.has(batchID)) groups.set(batchID, [])
    groups.get(batchID).push(item)
  })

  const grouped = [...groups.entries()].map(([batchID, batchItems]) => buildBatchWork(batchID, batchItems))

  return mergeIncompleteBatchGroups(grouped, singles)
}

function buildBatchWork(batchID, batchItems, expectedTotal = 0) {
  const sortedItems = [...batchItems].sort((left, right) => {
    const leftIndex = Number(left.batch_index)
    const rightIndex = Number(right.batch_index)
    if (Number.isFinite(leftIndex) && Number.isFinite(rightIndex) && leftIndex !== rightIndex) {
      return leftIndex - rightIndex
    }
    const leftTime = itemTime(left)
    const rightTime = itemTime(right)
    return leftTime - rightTime
  })
  const coverItem = sortedItems.find((item) => workThumbnails(item).length > 0) || sortedItems[0] || {}
  const latestTime = sortedItems.reduce((latest, item) => Math.max(latest, itemTime(item)), 0)
  const declaredTotal = Math.max(
    expectedTotal,
    ...sortedItems.map((item) => Number(item.batch_total || item.image_count || item.count) || 0),
    sortedItems.length
  )
  const status = batchStatus(sortedItems)
  return {
    ...coverItem,
    id: `batch-${batchID}`,
    batch_id: batchID,
    batch_items: sortedItems,
    is_batch: true,
    image_count: declaredTotal,
    status,
    generation_status: status,
    created_at: latestTime ? new Date(latestTime).toISOString() : coverItem.created_at,
    updated_at: latestTime ? new Date(latestTime).toISOString() : coverItem.updated_at
  }
}

function mergeIncompleteBatchGroups(groupedItems, singleItems) {
  const buckets = new Map()
  const passthrough = []
  ;[...groupedItems, ...singleItems].forEach((item) => {
    if (!canFallbackBatchGroup(item)) {
      passthrough.push(item)
      return
    }
    const key = fallbackBatchGroupKey(item)
    if (!key) {
      passthrough.push(item)
      return
    }
    if (!buckets.has(key)) buckets.set(key, [])
    buckets.get(key).push(item)
  })

  const merged = []
  buckets.forEach((bucketItems, key) => {
    const ordered = [...bucketItems].sort((left, right) => itemTime(left) - itemTime(right))
    let cluster = []
    ordered.forEach((item) => {
      const expectedTotal = expectedBatchTotal(item)
      const fallbackLimit = expectedTotal > 1 ? expectedTotal : 4
      const shouldStartNewCluster =
        cluster.length > 0 &&
        (!areWorksNearInTime(cluster[cluster.length - 1], item) || flattenBatchItems(cluster).length >= fallbackLimit)
      if (shouldStartNewCluster) {
        merged.push(buildFallbackBatchWork(key, cluster))
        cluster = []
      }
      cluster.push(item)
    })
    if (cluster.length > 0) {
      merged.push(buildFallbackBatchWork(key, cluster))
    }
  })

  return [...merged, ...passthrough]
}

function buildFallbackBatchWork(key, items) {
  if (items.length === 1) return items[0]
  const children = flattenBatchItems(items)
  const latestTime = Math.max(...items.map(itemTime), 0)
  const expectedTotal = Math.max(...items.map(expectedBatchTotal), children.length)
  const timePart = latestTime ? Math.floor(latestTime / 300000) : Date.now()
  return buildBatchWork(`fallback-${key}-${timePart}`, children, expectedTotal)
}

function flattenBatchItems(items) {
  return items.flatMap((item) => (isBatchWork(item) ? item.batch_items : [item]))
}

function fallbackBatchGroupKey(work) {
  const promptText = `${work.prompt || work.input_prompt || work.title || ''}`.trim()
  const aspectText = `${work.aspect_ratio || work.ratio || '1:1'}`.trim()
  const modeText = `${work.tool_mode || work.mode || normalizedMode(work) || ''}`.trim()
  if (!promptText) return ''
  return `${promptText}|${aspectText}|${modeText}`.toLowerCase()
}

function canFallbackBatchGroup(work) {
  return Boolean(fallbackBatchGroupKey(work))
}

function expectedBatchTotal(work) {
  const children = flattenBatchItems([work])
  const values = [
    Number(work.batch_total),
    Number(work.image_count),
    Number(work.count),
    ...children.map((item) => Number(item.batch_total || item.image_count || item.count))
  ].filter((value) => Number.isFinite(value) && value > 0)
  return Math.max(...values, children.length)
}

function itemTime(work) {
  return Date.parse(work.updated_at || work.created_at || work.completed_at || '') || 0
}

function areWorksNearInTime(left, right) {
  const leftTime = itemTime(left)
  const rightTime = itemTime(right)
  if (!leftTime || !rightTime) return true
  return Math.abs(leftTime - rightTime) <= 5 * 60 * 1000
}

function batchStatus(items) {
  if (items.some((item) => normalizedStatus(item) === 'running')) return 'running'
  if (items.length > 0 && items.every((item) => normalizedStatus(item) === 'failed')) return 'failed'
  return 'succeeded'
}

function applyWorksPayload(payload, page, append) {
  const nextItems = normalizeWorksPayload(payload)
  works.value = append ? mergeWorks(works.value, nextItems) : nextItems
  summary.value = payload?.summary || null
  totalWorks.value = payload?.total ?? works.value.length
  currentPage.value = payload?.page ?? page
  hasMore.value = works.value.length < totalWorks.value
}

async function loadWorks({ page = 1, append = false } = {}) {
  if (append) {
    loadingMore.value = true
  } else {
    loading.value = true
  }
  errorMessage.value = ''
  try {
    if (append) {
      const payload = await api.listWorks(listQueryParams(page))
      applyWorksPayload(payload, page, append)
      return
    }
    const [worksPayload, albumsPayload] = await Promise.all([
      api.listWorks(listQueryParams(page)),
      api.listCoupleAlbums()
    ])
    coupleAlbums.value = normalizeCoupleAlbumsPayload(albumsPayload)
    applyWorksPayload(worksPayload, page, append)
  } catch (error) {
    errorMessage.value = error.message || '作品列表读取失败'
  } finally {
    if (append) {
      loadingMore.value = false
    } else {
      loading.value = false
    }
  }
}

function resetAndLoadWorks() {
  currentPage.value = 1
  hasMore.value = false
  void loadWorks({ page: 1 })
}

function loadNextWorksPage() {
  if (loading.value || loadingMore.value || !hasMore.value) return
  void loadWorks({ page: currentPage.value + 1, append: true })
}

function isActivePending(task) {
  return normalizedStatus(task) === 'running'
}

function loadPendingTasks() {
  pendingGenerations.value = loadPendingGenerations().filter((task) => task.status !== 'succeeded')
  savePendingGenerations(pendingGenerations.value)
}

function stopPendingPolling() {
  if (pendingPollTimer !== null) {
    clearInterval(pendingPollTimer)
    pendingPollTimer = null
  }
}

async function pollPendingGenerations() {
  const activeTasks = pendingGenerations.value.filter(isActivePending)
  if (activeTasks.length === 0) {
    stopPendingPolling()
    return
  }

  const results = await Promise.all(
    activeTasks.map(async (task) => {
      try {
        return {
          generationID: task.generation_id,
          payload: await api.getImageGeneration(task.generation_id)
        }
      } catch (error) {
        return {
          generationID: task.generation_id,
          error
        }
      }
    })
  )
  const byID = new Map(results.map((result) => [result.generationID, result]))
  const completedIDs = []
  const nextPending = []

  pendingGenerations.value.forEach((task) => {
    const result = byID.get(task.generation_id)
    if (!result || result.error) {
      nextPending.push(task)
      return
    }

    const payload = result.payload || {}
    const status = payload.status || task.status
    if (status === 'succeeded') {
      completedIDs.push(task.generation_id)
      return
    }

    nextPending.push({
      ...task,
      ...payload,
      batch_id: payload.batch_id || task.batch_id || '',
      batch_index: Number.isFinite(Number(payload.batch_index)) ? Number(payload.batch_index) : task.batch_index,
      batch_total:
        Number.isFinite(Number(payload.batch_total)) && Number(payload.batch_total) > 0
          ? Number(payload.batch_total)
          : task.batch_total,
      status,
      stage: payload.stage || task.stage,
      progress: stageProgress(payload.stage || task.stage, status),
      error: payload?.error?.message || task.error || ''
    })
  })

  pendingGenerations.value = nextPending
  if (completedIDs.length > 0) {
    removePendingGenerations(completedIDs)
    savePendingGenerations(nextPending)
    resetAndLoadWorks()
  } else {
    savePendingGenerations(nextPending)
  }

  if (!pendingGenerations.value.some(isActivePending)) {
    stopPendingPolling()
  }
}

function startPendingPolling() {
  if (pendingPollTimer !== null || !pendingGenerations.value.some(isActivePending)) return
  pendingPollTimer = setInterval(() => {
    void pollPendingGenerations()
  }, 1600)
}

function openHistory() {
  activeTab.value = 'all'
  resetAndLoadWorks()
}

function openSupport() {
  navigateTo(routes.support)
}

function setActiveTab(key) {
  if (activeTab.value === key) return
  activeTab.value = key
  resetAndLoadWorks()
}

function selectTimeFilter() {
  const labels = timeOptions.map((item) => item.label)
  uni.showActionSheet({
    itemList: labels,
    success(result) {
      timeFilter.value = timeOptions[result.tapIndex]?.value || 'all'
      resetAndLoadWorks()
    }
  })
}

function selectStatusFilter() {
  const labels = statusOptions.map((item) => item.label)
  uni.showActionSheet({
    itemList: labels,
    success(result) {
      statusFilter.value = statusOptions[result.tapIndex]?.value || 'all'
      resetAndLoadWorks()
    }
  })
}

function selectSortFilter() {
  const labels = sortOptions.map((item) => item.label)
  uni.showActionSheet({
    itemList: labels,
    success(result) {
      sortFilter.value = sortOptions[result.tapIndex]?.value || 'latest'
      resetAndLoadWorks()
    }
  })
}

function currentTimeLabel() {
  return timeOptions.find((item) => item.value === timeFilter.value)?.label || '全部时间'
}

function currentStatusLabel() {
  return statusOptions.find((item) => item.value === statusFilter.value)?.label || '全部状态'
}

function currentSortLabel() {
  return sortOptions.find((item) => item.value === sortFilter.value)?.label || '排序'
}

function submitSearch() {
  searchQuery.value = searchInput.value.trim()
  resetAndLoadWorks()
}

function toggleSearch() {
  searchVisible.value = !searchVisible.value
  if (searchVisible.value) return
  if (searchInput.value || searchQuery.value) {
    searchInput.value = ''
    searchQuery.value = ''
    resetAndLoadWorks()
  }
}

function toggleLayout() {
  layoutMode.value = layoutMode.value === 'grid' ? 'list' : 'grid'
  try {
    uni.setStorageSync(layoutStorageKey, layoutMode.value)
  } catch {
    // Ignore storage failures; the current session still updates.
  }
}

function workID(work) {
  if (isAlbumCard(work)) {
    return work?.album_id || work?.id
  }
  if (isBatchWork(work)) {
    return concreteWorkItems(work)[0]?.work_id || concreteWorkItems(work)[0]?.id
  }
  return work?.work_id || work?.id
}

function isAlbumCard(work) {
  return Boolean(work?.is_album_card)
}

function isBatchWork(work) {
  return Boolean(work?.is_batch && Array.isArray(work.batch_items))
}

function concreteWorkItems(work) {
  if (isAlbumCard(work)) return []
  const items = isBatchWork(work) ? work.batch_items : [work]
  return items.filter((item) => {
    const id = item?.work_id || item?.id
    return id && !`${id}`.startsWith('pending-')
  })
}

function workShareItems(work) {
  return concreteWorkItems(work).filter((item) => normalizedStatus(item) === 'succeeded')
}

function workShareIDs(work) {
  return workShareItems(work)
    .map((item) => item.work_id || item.id)
    .filter(Boolean)
    .join(',')
}

function canNativeShareWork(work) {
  const targets = workShareItems(work)
  return targets.length > 0 && targets.every((target) => !isPrivate(target))
}

function normalizeShareIDs(value) {
  return `${value || ''}`
    .split(',')
    .map((id) => id.trim())
    .filter(Boolean)
    .slice(0, 16)
}

function shareEventDataset(event) {
  return event?.target?.dataset || event?.currentTarget?.dataset || event?.buttonTarget?.dataset || {}
}

function allShareableWorks() {
  return [...works.value, ...pendingWorkItems.value].flatMap((work) => (isBatchWork(work) ? work.batch_items : [work]))
}

function findShareWorksByIDs(ids) {
  const byID = new Map(allShareableWorks().map((work) => [`${work.work_id || work.id}`, work]))
  return ids.map((id) => byID.get(`${id}`)).filter(Boolean)
}

function workShareTitle(work, count = 1) {
  if (count > 1) return `白霖共享 AI 作品 · 共 ${count} 张`
  const text = `${work?.title || work?.prompt || work?.input_prompt || ''}`.trim()
  if (!text) return '白霖共享 AI 作品'
  return text.length > 24 ? `${text.slice(0, 24)}...` : text
}

function workSharePath(ids) {
  return `${routes.workShare}?ids=${ids.map((id) => encodeURIComponent(`${id}`)).join(',')}`
}

function publicWorkPreviewPath(id) {
  return `/api/public/works/${id}/file`
}

function workSharePayload(event) {
  const dataset = shareEventDataset(event)
  const rawIDs = dataset.shareIds || dataset.shareids || dataset['share-ids']
  const ids = normalizeShareIDs(rawIDs)
  const shareKind = dataset.shareKind || dataset.sharekind || dataset['share-kind']
  if (shareKind === 'album') {
    const token = `${dataset.shareToken || dataset.sharetoken || dataset['share-token'] || ''}`.trim()
    if (token) {
      return albumSharePayload({
        token,
        title: dataset.shareTitle || dataset.sharetitle || '',
        imageUrl: dataset.shareImage || dataset.shareimage || ''
      })
    }
  }
  if (shareKind === 'work' && ids.length > 0) {
    const sharedWorks = findShareWorksByIDs(ids)
    return {
      title: workShareTitle(sharedWorks[0], ids.length),
      path: workSharePath(ids),
      imageUrl: api.assetURL(publicWorkPreviewPath(ids[0]))
    }
  }
  return {
    title: '白霖共享 AI 作品库',
    path: routes.works
  }
}

function encodeShareValue(value) {
  return encodeURIComponent(`${value || ''}`.trim())
}

function albumShareToken(work) {
  return `${work?.share_token || ''}`.trim()
}

function isChildhoodDreamAlbum(work) {
  return work?.story_template === 'childhood_career_dream'
}

function albumProductName(work) {
  return isChildhoodDreamAlbum(work) ? '童年梦想相册' : '情侣相册'
}

function albumBrandName(work) {
  return isChildhoodDreamAlbum(work) ? '白霖共享童年梦想相册' : '白霖共享情侣相册'
}

function albumShareTitle(work) {
  const title = `${work?.title || ''}`.trim()
  return title ? `${title}｜${albumBrandName(work)}` : albumBrandName(work)
}

function albumSharePath(token) {
  const encodedToken = encodeShareValue(token)
  return `${routes.coupleAlbumShare}${encodedToken ? `?token=${encodedToken}` : ''}`
}

function albumShareImage(work) {
  return coverUrl(work)
}

function albumSharePayload(input) {
  const token = `${input?.token || albumShareToken(input)}`.trim()
  const query = token ? `token=${encodeShareValue(token)}` : ''
  return {
    title: input?.title || albumShareTitle(input),
    path: albumSharePath(token),
    query,
    imageUrl: input?.imageUrl || albumShareImage(input)
  }
}

function buildPublishedShareWork(original, updatedItems) {
  const updatedByID = new Map(updatedItems.map((item) => [`${workID(item)}`, item]))
  if (isBatchWork(original)) {
    return {
      ...original,
      batch_items: original.batch_items.map((item) => updatedByID.get(`${workID(item)}`) || item),
      visibility: 'public'
    }
  }
  return updatedItems[0] || { ...original, visibility: 'public' }
}

function isPendingWork(work) {
  if (isAlbumCard(work)) return false
  if (isBatchWork(work)) {
    return concreteWorkItems(work).length === 0 || normalizedStatus(work) === 'running'
  }
  return `${work?.id || ''}`.startsWith('pending-') || !workID(work)
}

function replaceWork(updated) {
  const id = workID(updated)
  if (!id) return
  const index = works.value.findIndex((item) => workID(item) === id)
  if (index >= 0) {
    works.value.splice(index, 1, updated)
  }
}

async function transformWorkToImage(work) {
  const id = workID(work)
  if (!id) {
    uni.showToast({ title: '生成完成后可转图生图', icon: 'none' })
    return
  }
  try {
    await api.reuseWork(id)
    navigateTo(routes.imageToImage, { reuse_work_id: id })
  } catch (error) {
    uni.showToast({ title: error.message || '导入图生图失败', icon: 'none' })
  }
}

function retryWork(work) {
  if (!workID(work)) {
    retryPendingGeneration(work)
    return
  }
  void transformWorkToImage(work)
}

function retryPendingGeneration(work) {
  const text = work.prompt || work.input_prompt || ''
  if (!text) {
    uni.showToast({ title: '暂无提示词', icon: 'none' })
    return
  }
  navigateTo(routes.imageToImage, {
    prompt: text,
    aspect: work.aspect_ratio || work.ratio || '1:1',
    mode: normalizedMode(work)
  })
}

function reusePrompt(work) {
  const text = work.prompt || work.input_prompt || ''
  if (!text) {
    uni.showToast({ title: '暂无提示词', icon: 'none' })
    return
  }
  navigateTo(routes.imageToImage, {
    prompt: text,
    aspect: work.aspect_ratio || work.ratio || '1:1',
    mode: 'text'
  })
}

async function toggleFavorite(work) {
  const targets = concreteWorkItems(work)
  if (targets.length === 0) {
    uni.showToast({ title: '生成完成后可收藏', icon: 'none' })
    return
  }
  const nextFavorite = !isFavorite(work)
  targets.forEach((target) => replaceWork({ ...target, is_favorite: nextFavorite }))
  try {
    const updatedItems = await Promise.all(
      targets.map((target) => api.updateWork(target.work_id || target.id, { is_favorite: nextFavorite }))
    )
    updatedItems.forEach(replaceWork)
    if (activeTab.value === 'favorites' && !nextFavorite) {
      const removedIDs = new Set(updatedItems.map((item) => `${workID(item)}`))
      works.value = works.value.filter((item) => !removedIDs.has(`${workID(item)}`))
    }
  } catch (error) {
    targets.forEach(replaceWork)
    uni.showToast({ title: error.message || '收藏更新失败', icon: 'none' })
  }
}

function confirmModal(content, title = '确认操作') {
  return new Promise((resolve) => {
    uni.showModal({
      title,
      content,
      confirmText: '确认',
      cancelText: '取消',
      success(result) {
        resolve(Boolean(result.confirm))
      },
      fail() {
        resolve(false)
      }
    })
  })
}

async function ensurePublic(work) {
  const id = workID(work)
  if (!id) return null
  if (!isPrivate(work)) return work
  const confirmed = await confirmModal('公开后可通过链接访问该作品文件，是否继续？', '公开作品')
  if (!confirmed) return null
  const updated = await api.updateWork(id, { visibility: 'public' })
  replaceWork(updated)
  return updated
}

async function toggleVisibility(work) {
  const targets = concreteWorkItems(work)
  if (targets.length === 0) {
    uni.showToast({ title: '生成完成后可设置可见性', icon: 'none' })
    return
  }
  try {
    if (isPrivate(work)) {
      const confirmed = await confirmModal(
        isBatchWork(work) ? '公开后该任务下的作品文件可通过链接访问，是否继续？' : '公开后可通过链接访问该作品文件，是否继续？',
        '公开作品'
      )
      if (!confirmed) return
      const updatedItems = await Promise.all(
        targets.map((target) => api.updateWork(target.work_id || target.id, { visibility: 'public' }))
      )
      updatedItems.forEach(replaceWork)
    } else {
      const updatedItems = await Promise.all(
        targets.map((target) => api.updateWork(target.work_id || target.id, { visibility: 'private' }))
      )
      updatedItems.forEach(replaceWork)
    }
  } catch (error) {
    uni.showToast({ title: error.message || '可见性更新失败', icon: 'none' })
  }
}

function publicShareLink(work) {
  const origin = typeof window !== 'undefined' ? window.location.origin : ''
  return `${origin}/api/public/works/${workID(work)}/file`
}

function copyText(text, title = '已复制') {
  uni.setClipboardData({
    data: text,
    success() {
      uni.showToast({ title, icon: 'none' })
    }
  })
}

function isMissingWorkUpdateRoute(error) {
  return error?.status === 404 && error?.code === 'not_found'
}

function shareErrorTitle(error, fallback) {
  if (isMissingWorkUpdateRoute(error)) return missingPublicShareSupportMessage
  return error.message || fallback
}

function openNativeSharePanel(work) {
  sharePanelWork.value = work
  sharePanelVisible.value = true
}

function closeNativeSharePanel() {
  sharePanelVisible.value = false
}

async function shareWork(work) {
  if (isAlbumCard(work)) {
    await shareAlbumCard(work)
    return
  }
  const targets = workShareItems(work)
  if (targets.length === 0) {
    uni.showToast({ title: '生成完成后可分享', icon: 'none' })
    return
  }
  if (canNativeShareWork(work)) return
  try {
    const confirmed = await confirmModal(
      isBatchWork(work) ? '公开后该任务下的作品可通过微信卡片分享，是否继续？' : '公开后该作品可通过微信卡片分享，是否继续？',
      '公开并分享'
    )
    if (!confirmed) return
    const privateTargets = targets.filter(isPrivate)
    const updatedItems = privateTargets.length
      ? await Promise.all(privateTargets.map((target) => api.updateWork(target.work_id || target.id, { visibility: 'public' })))
      : []
    updatedItems.forEach(replaceWork)
    openNativeSharePanel(buildPublishedShareWork(work, updatedItems))
  } catch (error) {
    uni.showToast({ title: shareErrorTitle(error, '分享失败'), icon: 'none' })
  }
}

function previewWork(work) {
  if (isAlbumCard(work)) {
    openAlbumCard(work)
    return
  }
  const urls = workThumbnails(work)
  if (urls.length === 0) {
    uni.showToast({ title: '暂无预览图', icon: 'none' })
    return
  }
  previewImages.value = urls
  previewIndex.value = 0
  previewTitle.value = isBatchWork(work) ? `共 ${workCount(work)} 张` : workTitle(work)
  previewOverlayVisible.value = true
}

function closePreviewOverlay() {
  previewOverlayVisible.value = false
}

function handlePreviewChange(event) {
  const nextIndex = Number(event?.detail?.current ?? 0)
  previewIndex.value = Number.isFinite(nextIndex) ? nextIndex : 0
}

function currentPreviewURL() {
  return previewImages.value[previewIndex.value] || previewImages.value[0] || ''
}

function resolveDownloadURL(url) {
  const value = `${url || ''}`.trim()
  if (!value) return ''
  if (/^https?:\/\//i.test(value)) return value
  if (value.startsWith('//')) return `https:${value}`
  return `${downloadAPIBaseURL}/${value.replace(/^\/+/, '')}`
}

function buildDownloadHeaders() {
  const headers = {}
  // #ifdef MP-WEIXIN
  headers['X-Image-Agent-Client'] = 'mp-weixin'
  // #endif
  const token = getStoredAuthToken()
  if (token) {
    headers.Authorization = `Bearer ${token}`
  }
  return headers
}

function isVideoDownloadURL(url, filePath = '') {
  const text = `${url || ''} ${filePath || ''}`.toLowerCase()
  return /\.(mp4|mov|m4v|webm)(\?|#|\s|$)/.test(text)
}

function handlePhoneSaveFailure(error, fallbackMessage = '保存失败，请稍后重试') {
  const errMsg = `${error?.errMsg || error?.message || ''}`
  if (/auth deny|authorize|permission|scope\.writePhotosAlbum/i.test(errMsg)) {
    if (typeof uni.openSetting === 'function') {
      uni.showModal({
        title: '需要相册权限',
        content: '请允许保存到相册，开启后再重新下载。',
        confirmText: '去设置',
        success(result) {
          if (result.confirm) {
            uni.openSetting()
          }
        }
      })
      return
    }
    uni.showToast({ title: '请在系统设置中允许保存到相册', icon: 'none' })
    return
  }
  uni.showToast({ title: fallbackMessage, icon: 'none' })
}

function hidePhoneSaveLoading() {
  uni.hideLoading()
}

function saveDownloadedFileToPhone(filePath, url) {
  if (!filePath) {
    uni.showToast({ title: '下载文件缺失，请重试', icon: 'none' })
    return
  }

  uni.showLoading({ title: '正在保存', mask: true })

  if (isVideoDownloadURL(url, filePath) && typeof uni.saveVideoToPhotosAlbum === 'function') {
    uni.saveVideoToPhotosAlbum({
      filePath,
      success() {
        hidePhoneSaveLoading()
        uni.showToast({ title: '已保存到相册', icon: 'none' })
      },
      fail(error) {
        hidePhoneSaveLoading()
        handlePhoneSaveFailure(error, '视频保存失败，请稍后重试')
      }
    })
    return
  }

  if (typeof uni.saveImageToPhotosAlbum === 'function') {
    uni.saveImageToPhotosAlbum({
      filePath,
      success() {
        hidePhoneSaveLoading()
        uni.showToast({ title: '已保存到相册', icon: 'none' })
      },
      fail(error) {
        hidePhoneSaveLoading()
        handlePhoneSaveFailure(error, '图片保存失败，请稍后重试')
      }
    })
    return
  }

  if (typeof uni.saveFile === 'function') {
    uni.saveFile({
      tempFilePath: filePath,
      success() {
        hidePhoneSaveLoading()
        uni.showToast({ title: '已保存到本机文件', icon: 'none' })
      },
      fail(error) {
        hidePhoneSaveLoading()
        handlePhoneSaveFailure(error)
      }
    })
    return
  }

  hidePhoneSaveLoading()
  uni.showToast({ title: '当前客户端不支持直接保存文件', icon: 'none' })
}

function downloadURL(rawURL) {
  const url = resolveDownloadURL(rawURL)
  if (!url) {
    uni.showToast({ title: '暂无可下载文件', icon: 'none' })
    return
  }
  if (typeof window !== 'undefined') {
    window.open(url, '_blank')
    return
  }
  uni.showLoading({ title: '正在下载', mask: true })
  let timedOut = false
  let completed = false
  let timeoutID = null
  let downloadTask = null
  const clearDownloadTimeout = () => {
    if (timeoutID !== null) {
      clearTimeout(timeoutID)
      timeoutID = null
    }
  }
  const finishDownload = () => {
    completed = true
    clearDownloadTimeout()
    uni.hideLoading()
  }
  downloadTask = uni.downloadFile({
    url,
    header: buildDownloadHeaders(),
    success(result) {
      if (timedOut) return
      if (result.statusCode && result.statusCode >= 400) {
        finishDownload()
        uni.showToast({ title: '下载失败，请重新登录后再试', icon: 'none' })
        return
      }
      const filePath = result.tempFilePath
      finishDownload()
      saveDownloadedFileToPhone(filePath, url)
    },
    fail(error) {
      if (timedOut) return
      finishDownload()
      handlePhoneSaveFailure(error, '下载失败，请稍后重试')
    },
    complete() {
      if (!timedOut && !completed) {
        finishDownload()
      }
    }
  })
  timeoutID = setTimeout(() => {
    if (completed) return
    timedOut = true
    clearDownloadTimeout()
    if (downloadTask && typeof downloadTask.abort === 'function') {
      downloadTask.abort()
    }
    uni.hideLoading()
    uni.showToast({ title: '下载超时，请稍后重试', icon: 'none' })
  }, downloadTimeoutMS)
}

function downloadPreviewCurrent() {
  downloadURL(currentPreviewURL())
}

function downloadWork(work) {
  if (isAlbumCard(work)) {
    openAlbumCard(work)
    return
  }
  downloadURL(work.download_url || coverUrl(work))
}

async function copyWorkLink(work) {
  const id = workID(work)
  if (!id) {
    uni.showToast({ title: '暂无链接', icon: 'none' })
    return
  }
  try {
    const publicWork = await ensurePublic(work)
    if (!publicWork) return
    copyText(publicShareLink(publicWork), '链接已复制')
  } catch (error) {
    uni.showToast({ title: shareErrorTitle(error, '复制失败'), icon: 'none' })
  }
}

async function deleteWork(work) {
  const targets = concreteWorkItems(work)
  if (targets.length === 0) {
    uni.showToast({ title: '生成中作品暂不可删除', icon: 'none' })
    return
  }
  const confirmed = await confirmModal(
    isBatchWork(work) ? '删除后该任务下的作品将不再显示，是否继续？' : '删除后作品库不再显示该作品，是否继续？',
    '删除作品'
  )
  if (!confirmed) return
  try {
    await Promise.all(targets.map((target) => api.deleteWork(target.work_id || target.id)))
    const removedIDs = new Set(targets.map((item) => `${item.work_id || item.id}`))
    works.value = works.value.filter((item) => !removedIDs.has(`${item.work_id || item.id}`))
    resetAndLoadWorks()
    uni.showToast({ title: '作品已删除', icon: 'none' })
  } catch (error) {
    uni.showToast({ title: error.message || '删除失败', icon: 'none' })
  }
}

function moreActions(work) {
  if (isAlbumCard(work)) {
    const labels = ['查看相册', '分享相册']
    uni.showActionSheet({
      itemList: labels,
      success(result) {
        if (result.tapIndex === 0) openAlbumCard(work)
        if (result.tapIndex === 1) void shareAlbumCard(work)
      }
    })
    return
  }
  if (isPendingWork(work)) {
    uni.showToast({ title: '生成完成后可操作', icon: 'none' })
    return
  }
  const labels = ['查看大图', '下载', '复制链接', '删除作品']
  uni.showActionSheet({
    itemList: labels,
    success(result) {
      if (result.tapIndex === 0) previewWork(work)
      if (result.tapIndex === 1) downloadWork(work)
      if (result.tapIndex === 2) void copyWorkLink(work)
      if (result.tapIndex === 3) void deleteWork(work)
    }
  })
}

function normalizedCategory(work) {
  if (isAlbumCard(work)) return 'album'
  const raw = `${work.category || work.media_type || work.type || ''}`.toLowerCase()
  if (raw.includes('video') || raw.includes('视频')) return 'video'
  if (raw.includes('poster_kv') || raw.includes('海报') || raw.includes('kv')) return 'poster_kv'
  if (raw.includes('product_main') || raw.includes('商品') || raw.includes('主图')) return 'product_main'
  if (raw.includes('cover') || raw.includes('封面')) return 'cover'
  return 'image'
}

function matchesLocalFilters(work) {
  if (isAlbumCard(work)) {
    if (activeTab.value !== 'all') return false
    if (statusFilter.value !== 'all' && normalizedStatus(work) !== statusFilter.value) return false
    if (timeFilter.value !== 'all' && !matchesTimeFilter(work)) return false
    if (searchQuery.value && !albumSearchText(work).includes(searchQuery.value)) return false
    return true
  }
  if (isPendingWork(work)) {
    if (activeTab.value === 'favorites') return false
    if (activeTab.value !== 'all' && normalizedCategory(work) !== activeTab.value) return false
    if (statusFilter.value !== 'all' && normalizedStatus(work) !== statusFilter.value) return false
    if (searchQuery.value && !`${work.prompt || work.input_prompt || ''}`.includes(searchQuery.value)) return false
    return true
  }
  return true
}

function matchesTimeFilter(work) {
  const timestamp = itemTime(work)
  if (!timestamp) return true
  const now = new Date()
  let start = 0
  if (timeFilter.value === 'today') {
    start = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime()
  } else if (timeFilter.value === 'week') {
    start = now.getTime() - 7 * 24 * 60 * 60 * 1000
  } else if (timeFilter.value === 'month') {
    start = new Date(now.getFullYear(), now.getMonth() - 1, now.getDate()).getTime()
  }
  return start === 0 || timestamp >= start
}

function albumSearchText(albumCard) {
  return [
    albumCard.title,
    albumCard.location,
    ...(albumCard.pages || []).flatMap((page) => [page.page_title, page.caption])
  ]
    .filter(Boolean)
    .join(' ')
}

function pendingGenerationInputMode(task) {
  const rawMode = `${task?.mode || ''}`.toLowerCase()
  if (rawMode === 'text' || rawMode.includes('text') || rawMode.includes('文')) return 'text'
  if (rawMode === 'image' || rawMode.includes('image') || rawMode.includes('图')) return 'image'
  if (task?.reference_asset_ids?.length || task?.reference_preview_urls?.length) return 'image'
  return 'text'
}

function normalizedMode(work) {
  const rawMode = `${work.mode || ''}`.toLowerCase()
  const rawTool = `${work.tool_mode || work.type || ''}`.toLowerCase()
  if (rawMode === 'text' || rawMode.includes('text') || rawMode.includes('文')) return 'text'
  if (rawMode === 'image' || rawMode.includes('image') || rawMode.includes('图')) return 'image'
  if (rawTool.includes('image') || rawTool.includes('图')) return 'image'
  if (work.reference_asset_ids?.length || work.reference_assets?.length || work.reference_preview_urls?.length) return 'image'
  return 'text'
}

function modeLabel(work) {
  if (isAlbumCard(work)) return albumProductName(work)
  return normalizedMode(work) === 'image' ? '图片转图片' : '文字转图片'
}

function isFavorite(work) {
  return Boolean(work.is_favorite || work.favorite || work.favorited)
}

function isPrivate(work) {
  const visibility = `${work.visibility || work.privacy || ''}`.toLowerCase()
  return visibility === 'private' || visibility === '私有'
}

function visibilityLabel(work) {
  if (isAlbumCard(work)) return albumStatusText(work.status)
  if (normalizedStatus(work) === 'running') return '生成中'
  if (isFavorite(work)) return '已收藏'
  return isPrivate(work) ? '私有' : '已公开'
}

function visibilityClass(work) {
  if (isAlbumCard(work)) return normalizedStatus(work)
  if (normalizedStatus(work) === 'running') return 'running'
  if (isFavorite(work)) return 'favorite'
  return isPrivate(work) ? 'private' : 'public'
}

function normalizedStatus(work) {
  if (isAlbumCard(work)) {
    const albumStatus = `${work.status || ''}`.toLowerCase()
    if (albumStatus === 'succeeded') return 'succeeded'
    if (albumStatus === 'failed' || albumStatus === 'partial_failed') return 'failed'
    return 'running'
  }
  const status = `${work.status || work.generation_status || ''}`.toLowerCase()
  if (['succeeded', 'completed', 'success', 'done'].includes(status)) return 'succeeded'
  if (['failed', 'error', 'cancelled', 'canceled'].includes(status)) return 'failed'
  return 'running'
}

function statusText(work) {
  const status = normalizedStatus(work)
  if (status === 'succeeded') return '已完成'
  if (status === 'failed') return '失败'
  return '生成中'
}

function pendingFailureText(work) {
  if (isAlbumCard(work)) return ''
  if (normalizedStatus(work) !== 'failed') return ''
  return work?.error?.message || work?.error || '生成失败，请稍后重新生成'
}

function workTitle(work) {
  if (isAlbumCard(work)) return work.title || albumProductName(work)
  const text = work.title || work.prompt || work.input_prompt || '未命名作品'
  return text.length > 15 ? `${text.slice(0, 15)}...` : text
}

function workExcerpt(work) {
  if (isAlbumCard(work)) {
    return `${work.location || '旅行相册'} · ${work.completed_page_count || 0}/${work.page_count || 8} 页完成`
  }
  const text = work.prompt || work.input_prompt || work.title || ''
  if (!text) return ''
  return text.length > 42 ? `"${text.slice(0, 42)}..."` : `"${text}"`
}

function workRatio(work) {
  if (isAlbumCard(work)) return '相册'
  return work.aspect_ratio || work.ratio || '1:1'
}

function workCount(work) {
  if (isAlbumCard(work)) return work.completed_page_count || 0
  const thumbnails = workThumbnails(work)
  return work.image_count || work.count || Math.max(thumbnails.length, 1)
}

function workTime(work) {
  const raw = work.created_at || work.updated_at || work.completed_at
  if (!raw) return '刚刚'
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) return `${raw}`
  const diff = Date.now() - date.getTime()
  if (diff < 60_000) return '刚刚'
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)}分钟前`
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)}小时前`
  return `${date.getMonth() + 1}/${date.getDate()} ${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`
}

function progressValue(work) {
  const value = Number(work.progress ?? work.percent ?? 60)
  if (!Number.isFinite(value)) return 60
  return Math.max(6, Math.min(100, value))
}

function progressPercent(work) {
  if (isAlbumCard(work)) {
    const total = Math.max(Number(work.page_count) || 8, 1)
    return Math.max(6, Math.min(100, Math.round(((work.completed_page_count || 0) / total) * 100)))
  }
  return progressValue(work)
}

function progressLabel(work) {
  if (isAlbumCard(work)) {
    return `已完成 ${work.completed_page_count || 0}/${work.page_count || 8} 页`
  }
  return `进度 ${progressValue(work)}%`
}

function uniqueURLs(urls) {
  const seen = new Set()
  return urls.filter((url) => {
    if (!url || seen.has(url)) return false
    seen.add(url)
    return true
  })
}

function collectWorkThumbnails(work) {
  if (isAlbumCard(work)) {
    return [work.preview_url, work.cover_url, ...(work.pages || []).map((page) => page.preview_url)].filter(Boolean)
  }
  const images = work.images || work.outputs || work.results || []
  const referencePreviews =
    normalizedMode(work) === 'image' && Array.isArray(work.reference_preview_urls) ? work.reference_preview_urls : []
  const fromList = Array.isArray(images)
    ? images
        .map((item) => (typeof item === 'string' ? item : item.url || item.image_url || item.thumbnail_url))
        .filter(Boolean)
    : []
  return [
    work.preview_url,
    work.download_url,
    work.thumbnail_url,
    work.cover_url,
    work.image_url,
    work.url,
    ...referencePreviews,
    ...fromList
  ].filter(Boolean)
}

function workThumbnails(work) {
  if (isBatchWork(work)) {
    return uniqueURLs(work.batch_items.flatMap((item) => collectWorkThumbnails(item)))
  }
  return uniqueURLs(collectWorkThumbnails(work))
}

function hasWorkThumbnail(work) {
  return workThumbnails(work).length > 0
}

function coverUrl(work) {
  return workThumbnails(work)[0] || ''
}

function albumPages(album) {
  return Array.isArray(album?.pages) ? album.pages : []
}

function albumCompletedPages(album) {
  return albumPages(album).filter((page) => page.status === 'succeeded' && page.preview_url).length
}

function albumCoverPage(album) {
  const pages = albumPages(album)
  return pages.find((page) => page.id === album?.cover_page_id && page.preview_url) || pages.find((page) => page.preview_url) || null
}

function buildAlbumCard(album) {
  const pages = albumPages(album)
  const cover = albumCoverPage(album)
  return {
    ...album,
    id: `album-${album.id}`,
    album_id: album.id,
    is_album_card: true,
    pages,
    page_count: Math.max(pages.length, 8),
    completed_page_count: albumCompletedPages(album),
    preview_url: cover?.preview_url || '',
    cover_url: cover?.preview_url || ''
  }
}

function albumStatusText(status) {
  switch (`${status || ''}`) {
    case 'succeeded':
      return '已完成'
    case 'partial_failed':
      return '部分失败'
    case 'failed':
      return '失败'
    case 'generating':
      return '生成中'
    default:
      return '草稿'
  }
}

function sharePanelTitle() {
  if (isAlbumCard(sharePanelWork.value)) return albumShareTitle(sharePanelWork.value)
  return sharePanelWork.value ? workShareTitle(sharePanelWork.value, workShareItems(sharePanelWork.value).length) : '白霖共享 AI 作品'
}

function sharePanelSubtitle() {
  if (isAlbumCard(sharePanelWork.value)) {
    return `${sharePanelWork.value.completed_page_count || 0}/${sharePanelWork.value.page_count || 8} 页 · 微信卡片`
  }
  return sharePanelWork.value ? `共 ${workShareItems(sharePanelWork.value).length || 1} 张` : '共 1 张'
}

function sharePanelImage() {
  return sharePanelWork.value ? coverUrl(sharePanelWork.value) : ''
}

function sharePanelKind() {
  return isAlbumCard(sharePanelWork.value) ? 'album' : 'work'
}

function sharePanelToken() {
  return isAlbumCard(sharePanelWork.value) ? albumShareToken(sharePanelWork.value) : ''
}

function sharePanelIDs() {
  return isAlbumCard(sharePanelWork.value) ? '' : workShareIDs(sharePanelWork.value)
}

function openAlbumCard(work) {
  const id = work?.album_id || work?.id
  if (!id) return
  navigateTo(routes.coupleAlbumDetail, { id })
}

async function shareAlbumCard(work) {
  const id = work?.album_id || work?.id
  if (!id || albumSharingID.value) return
  albumSharingID.value = `${id}`
  try {
    const payload = await api.shareCoupleAlbum(id)
    const token = `${payload?.share_token || payload?.album?.share_token || work?.share_token || ''}`.trim()
    let panelAlbum = { ...work, share_token: token, share_enabled: true }
    if (payload?.album) {
      const index = coupleAlbums.value.findIndex((album) => `${album.id}` === `${id}`)
      if (index >= 0) {
        coupleAlbums.value.splice(index, 1, payload.album)
      }
      panelAlbum = { ...buildAlbumCard(payload.album), share_token: token }
    }
    if (!albumShareToken(panelAlbum)) {
      uni.showToast({ title: '分享已开启，请稍后重试', icon: 'none' })
      return
    }
    openNativeSharePanel(panelAlbum)
  } catch (error) {
    uni.showToast({ title: error.message || '分享失败', icon: 'none' })
  } finally {
    albumSharingID.value = ''
  }
}

onMounted(async () => {
  const user = await requireAuth()
  if (!user) return
  try {
    const savedLayout = uni.getStorageSync(layoutStorageKey)
    if (savedLayout === 'grid' || savedLayout === 'list') {
      layoutMode.value = savedLayout
    }
  } catch {
    layoutMode.value = 'list'
  }
  loadPendingTasks()
  void pollPendingGenerations()
  startPendingPolling()
  resetAndLoadWorks()
})

onReachBottom(() => {
  loadNextWorksPage()
})

onBeforeUnmount(stopPendingPolling)
</script>

<template>
  <view class="works-page">
    <view class="app-shell">
      <view class="topbar">
        <view class="brand">
          <image class="brand-icon" :src="icon('logo-star')" mode="aspectFit" />
          <view>
            <text class="brand-name">白霖共享</text>
            <text class="brand-subtitle">创作者 AI 图片平台</text>
          </view>
        </view>
        <button class="notification-button" type="button" @click="openSupport">
          <view class="bell-icon">
            <view></view>
          </view>
        </button>
      </view>

      <text class="page-title">作品库</text>

      <view class="category-tabs">
        <button
          v-for="tab in tabs"
          :key="tab.key"
          type="button"
          :class="{ active: activeTab === tab.key }"
          @click="setActiveTab(tab.key)"
        >
          {{ tab.label }}
        </button>
      </view>

      <view class="filter-bar">
        <view class="filter-row">
          <button type="button" @click="selectTimeFilter">
            <text>{{ currentTimeLabel() }}</text>
            <text>⌄</text>
          </button>
          <button type="button" @click="selectStatusFilter">
            <text>{{ currentStatusLabel() }}</text>
            <text>⌄</text>
          </button>
          <button type="button" @click="selectSortFilter">
            <text>{{ currentSortLabel() }}</text>
            <text>⌄</text>
          </button>
        </view>

        <view class="utility-actions">
          <button type="button" :class="{ active: searchVisible }" @click="toggleSearch">
            <text>⌕</text>
          </button>
          <button type="button" :class="{ active: layoutMode === 'grid' }" @click="toggleLayout">
            <text>{{ layoutMode === 'grid' ? '☰' : '☷' }}</text>
          </button>
        </view>
      </view>

      <view v-if="searchVisible" class="search-row">
        <input
          v-model="searchInput"
          confirm-type="search"
          placeholder="搜索提示词或作品名称"
          @confirm="submitSearch"
        />
        <button type="button" @click="submitSearch">搜索</button>
      </view>

      <view v-if="loading && filteredWorks.length === 0" class="empty-state">作品读取中...</view>
      <view v-else-if="errorMessage" class="empty-state error">{{ errorMessage }}</view>
      <view v-else-if="filteredWorks.length === 0" class="empty-state">
        {{ activeTab === 'favorites' ? '暂无收藏作品' : '暂无生成记录' }}
      </view>

      <view v-else class="work-list" :class="layoutMode">
        <view
          v-for="work in filteredWorks"
          :key="work.album_id ? `album-${work.album_id}` : work.batch_id || work.work_id || work.id || work.generation_id || work.created_at"
          class="work-card"
          :class="{ 'album-card': isAlbumCard(work) }"
          @click="previewWork(work)"
        >
          <view class="work-main">
            <view class="cover-wrap">
              <image v-if="hasWorkThumbnail(work)" class="cover" :src="coverUrl(work)" mode="aspectFill" />
              <view v-else class="cover-placeholder" :class="{ running: normalizedStatus(work) === 'running' }">
                <view class="placeholder-orbit"></view>
                <text>{{ normalizedStatus(work) === 'running' ? '生成中' : '暂无预览' }}</text>
              </view>
              <view class="cover-ratio-badge">
                <text>▣</text>
                <text>{{ workRatio(work) }}</text>
              </view>
              <view v-if="isAlbumCard(work)" class="batch-count-badge album-count-badge">
                {{ work.completed_page_count }}/{{ work.page_count }} 页
              </view>
              <view v-else-if="isBatchWork(work)" class="batch-count-badge">共 {{ workCount(work) }} 张</view>
            </view>
            <view class="work-info">
              <view class="title-line">
                <text class="work-title">{{ workTitle(work) }}</text>
                <view v-if="!isAlbumCard(work)" class="visibility-actions">
                  <button type="button" class="visibility-button" :class="visibilityClass(work)" @click.stop="toggleVisibility(work)">
                    <text>{{ isPrivate(work) ? '▢' : '◉' }}</text>
                  </button>
                  <button type="button" class="visibility-button favorite" :class="{ active: isFavorite(work) }" @click.stop="toggleFavorite(work)">
                    <image :src="icon('favorite')" mode="aspectFit" />
                  </button>
                  <button type="button" class="visibility-button" @click.stop="moreActions(work)">
                    <image :src="icon('more')" mode="aspectFit" />
                  </button>
                </view>
              </view>

              <text v-if="workExcerpt(work)" class="work-excerpt">{{ workExcerpt(work) }}</text>
              <text class="mode-pill">{{ modeLabel(work) }}</text>

              <view v-if="isAlbumCard(work)" class="meta-line album-meta-line">
                <text>⌖</text>
                <text>{{ work.location || '旅行相册' }}</text>
                <text>▧</text>
                <text>{{ work.completed_page_count }}/{{ work.page_count }}页</text>
                <text class="visibility-dot" :class="visibilityClass(work)">•</text>
                <text class="status" :class="visibilityClass(work)">{{ visibilityLabel(work) }}</text>
              </view>
              <view v-else class="meta-line">
                <text>▦</text>
                <text>{{ workRatio(work) }}</text>
                <text>▧</text>
                <text>{{ workCount(work) }}张</text>
                <text>◷</text>
                <text>{{ workTime(work) }}</text>
                <text class="visibility-dot" :class="visibilityClass(work)">•</text>
                <text class="status" :class="visibilityClass(work)">{{ visibilityLabel(work) }}</text>
              </view>

              <view v-if="normalizedStatus(work) === 'running'" class="progress-block">
                <text>{{ progressLabel(work) }}</text>
                <view>
                  <view :style="{ width: `${progressPercent(work)}%` }"></view>
                </view>
              </view>
              <text v-if="pendingFailureText(work)" class="pending-failure">{{ pendingFailureText(work) }}</text>

            </view>
          </view>

          <view class="card-toolbar">
            <template v-if="isAlbumCard(work)">
              <button type="button" class="primary" @click.stop="openAlbumCard(work)">
                <image :src="icon('works')" mode="aspectFit" />
                <text>查看相册</text>
              </button>
              <button
                type="button"
                data-share-kind="album"
                :disabled="albumSharingID === `${work.album_id || work.id}`"
                @click.stop="shareAlbumCard(work)"
              >
                <image :src="icon('upload')" mode="aspectFit" />
                <text>{{ albumSharingID === `${work.album_id || work.id}` ? '开启中' : '分享相册' }}</text>
              </button>
            </template>
            <template v-else>
            <button type="button" class="primary" @click.stop="reusePrompt(work)">
              <image :src="icon('prompt')" mode="aspectFit" />
              <text>复用提示词</text>
            </button>
            <button type="button" @click.stop="normalizedStatus(work) === 'failed' ? retryWork(work) : transformWorkToImage(work)">
              <image :src="icon(normalizedStatus(work) === 'failed' ? 'refresh' : 'image-image')" mode="aspectFit" />
              <text>{{ normalizedStatus(work) === 'failed' ? '重新生成' : '转图生图' }}</text>
            </button>
            <button
              v-if="canNativeShareWork(work)"
              type="button"
              open-type="share"
              data-share-kind="work"
              :data-share-ids="workShareIDs(work)"
              @click.stop="shareWork(work)"
            >
              <image :src="icon('upload')" mode="aspectFit" />
              <text>分享</text>
            </button>
            <button v-else type="button" @click.stop="shareWork(work)">
              <image :src="icon('upload')" mode="aspectFit" />
              <text>分享</text>
            </button>
            <button type="button" @click.stop="moreActions(work)">
              <text>更多</text>
              <text>⌄</text>
            </button>
            </template>
          </view>
        </view>
      </view>

      <view v-if="filteredWorks.length > 0" class="load-more-state">
        <text v-if="loadingMore">正在加载更多作品...</text>
        <text v-else-if="hasMore">上拉加载更多作品</text>
        <text v-else>已显示全部作品</text>
      </view>

    </view>

    <view v-if="previewOverlayVisible" class="preview-overlay" @click="closePreviewOverlay">
      <view class="preview-panel" @click.stop>
        <view class="preview-header">
          <text>{{ previewTitle }}</text>
          <button type="button" @click="closePreviewOverlay">×</button>
        </view>
        <swiper class="work-preview-swiper" :current="previewIndex" @change="handlePreviewChange">
          <swiper-item v-for="(url, index) in previewImages" :key="`${url}-${index}`">
            <image :src="url" mode="aspectFit" />
          </swiper-item>
        </swiper>
        <view class="preview-footer">
          <text>{{ previewIndex + 1 }} / {{ previewImages.length }}</text>
          <button type="button" @click="downloadPreviewCurrent">下载当前</button>
        </view>
      </view>
    </view>

    <view v-if="sharePanelVisible" class="native-share-overlay" @click="closeNativeSharePanel">
      <view class="native-share-panel" @click.stop>
        <view class="native-share-header">
          <text>微信分享</text>
          <button type="button" @click="closeNativeSharePanel">×</button>
        </view>
        <view class="native-share-summary">
          <image v-if="sharePanelImage()" :src="sharePanelImage()" mode="aspectFill" />
          <view>
            <text>{{ sharePanelTitle() }}</text>
            <text>{{ sharePanelSubtitle() }}</text>
          </view>
        </view>
        <button
          type="button"
          class="native-share-button"
          open-type="share"
          :data-share-kind="sharePanelKind()"
          :data-share-ids="sharePanelIDs()"
          :data-share-token="sharePanelToken()"
          :data-share-title="sharePanelTitle()"
          :data-share-image="sharePanelImage()"
          @click.stop="closeNativeSharePanel"
        >
          <image :src="icon('upload')" mode="aspectFit" />
          <text>发送给好友</text>
        </button>
      </view>
    </view>

    <AppTabbar active-key="works" />
    <AnnouncementPopup />
  </view>
</template>

<style lang="scss" scoped>
@use '../../styles/tokens.scss' as *;

.works-page {
  min-height: 100vh;
  background:
    radial-gradient(circle at 3% 0, rgba(255, 197, 225, 0.78), transparent 32%),
    radial-gradient(circle at 96% 2%, rgba(218, 230, 255, 0.92), transparent 36%),
    linear-gradient(180deg, #fff6fb 0%, #f7faff 48%, #f2f7ff 100%);
  color: #121b33;
}

.works-page,
.works-page view,
.works-page uni-view,
.works-page button,
.works-page uni-button,
.works-page image,
.works-page uni-image,
.works-page text {
  box-sizing: border-box;
}

.works-page button {
  margin: 0;
  padding: 0;
  border: 0;
  line-height: 1.2;
  overflow: visible;
}

.works-page button::after {
  border: 0;
}

.app-shell {
  min-height: 100vh;
  padding: calc(36rpx + $phone-float-clearance-top + env(safe-area-inset-top)) 34rpx 0;
}

.topbar,
.brand,
.category-tabs,
.filter-bar,
.filter-row,
.filter-row button,
.search-row,
.search-row button,
.utility-actions,
.utility-actions button,
.work-main,
.title-line,
.meta-line,
.thumb-row,
.visibility-actions,
.visibility-button,
.card-toolbar,
.card-toolbar button,
.preview-overlay,
.preview-header,
.preview-footer,
.native-share-overlay,
.native-share-header,
.native-share-summary,
.native-share-button {
  display: flex;
  align-items: center;
}

.topbar {
  justify-content: space-between;
}

.brand {
  gap: 16rpx;
}

.brand-icon {
  width: 58rpx;
  height: 58rpx;
}

.brand-name,
.brand-subtitle,
.page-title,
.work-title,
.work-excerpt,
.mode-pill,
.progress-block text {
  display: block;
}

.brand-name {
  color: #10182d;
  font-size: 31rpx;
  font-weight: 950;
  line-height: 1.08;
  letter-spacing: 0;
}

.brand-subtitle {
  margin-top: 8rpx;
  color: #6f7890;
  font-size: 23rpx;
  font-weight: 700;
}

.notification-button {
  display: grid;
  place-items: center;
  width: 74rpx;
  height: 74rpx;
  border-radius: 20rpx;
  background: rgba(255, 255, 255, 0.9);
  color: #121b33;
  font-size: 40rpx;
  font-weight: 900;
  box-shadow: 0 18rpx 36rpx rgba(41, 57, 94, 0.1);
}

.bell-icon {
  position: relative;
  width: 30rpx;
  height: 34rpx;
}

.bell-icon::before {
  content: '';
  position: absolute;
  left: 5rpx;
  top: 3rpx;
  width: 20rpx;
  height: 22rpx;
  border: 3rpx solid #111b34;
  border-bottom: 0;
  border-radius: 15rpx 15rpx 8rpx 8rpx;
}

.bell-icon::after {
  content: '';
  position: absolute;
  left: 2rpx;
  right: 2rpx;
  bottom: 6rpx;
  height: 3rpx;
  border-radius: 999rpx;
  background: #111b34;
}

.bell-icon view {
  position: absolute;
  left: 12rpx;
  bottom: 0;
  width: 6rpx;
  height: 6rpx;
  border-radius: 50%;
  background: #111b34;
}

.page-title {
  margin-top: 42rpx;
  color: #0f1b34;
  font-size: 42rpx;
  font-weight: 950;
  line-height: 1.1;
}

.visibility-button image,
.card-toolbar image {
  width: 28rpx;
  height: 28rpx;
}

.category-tabs {
  gap: 16rpx;
  margin-top: 28rpx;
  overflow-x: auto;
  padding-bottom: 4rpx;
}

.category-tabs button,
.category-tabs uni-button {
  display: flex;
  align-items: center;
  justify-content: center;
  min-width: 0;
  height: 58rpx;
  padding: 0 24rpx;
  border-radius: 999rpx;
  background: transparent;
  color: #69738d;
  font-size: 25rpx;
  font-weight: 950;
  line-height: 1;
  white-space: nowrap;
}

.category-tabs button.active,
.category-tabs uni-button.active {
  background: linear-gradient(135deg, #9452ff 0%, #226bff 100%);
  color: #fff;
  box-shadow: 0 16rpx 32rpx rgba(85, 92, 235, 0.3);
}

.filter-bar {
  gap: 16rpx;
  justify-content: space-between;
  margin-top: 30rpx;
}

.filter-row {
  flex: 1;
  min-width: 0;
  gap: 14rpx;
}

.filter-row button {
  justify-content: center;
  flex: 1;
  min-width: 0;
  gap: 10rpx;
  height: 70rpx;
  padding: 0 18rpx;
  border-radius: 16rpx;
  background: rgba(255, 255, 255, 0.84);
  color: #26304a;
  font-size: 23rpx;
  font-weight: 950;
  box-shadow: 0 12rpx 28rpx rgba(31, 45, 82, 0.05);
}

.filter-row button text:first-child {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.utility-actions {
  flex: 0 0 auto;
  gap: 12rpx;
}

.utility-actions button {
  justify-content: center;
  width: 70rpx;
  height: 70rpx;
  border-radius: 16rpx;
  background: rgba(255, 255, 255, 0.86);
  color: #101b34;
  font-size: 42rpx;
  font-weight: 900;
  box-shadow: 0 12rpx 28rpx rgba(31, 45, 82, 0.05);
}

.utility-actions button.active {
  color: #315cff;
  background: rgba(121, 86, 255, 0.12);
}

.search-row {
  gap: 14rpx;
  margin-top: 18rpx;
}

.search-row input {
  flex: 1;
  min-width: 0;
  height: 72rpx;
  padding: 0 22rpx;
  border-radius: 16rpx;
  background: rgba(255, 255, 255, 0.9);
  color: #172033;
  font-size: 24rpx;
  font-weight: 800;
  box-shadow: 0 12rpx 28rpx rgba(31, 45, 82, 0.05);
}

.search-row button {
  justify-content: center;
  width: 112rpx;
  height: 72rpx;
  border-radius: 16rpx;
  background: #172033;
  color: #fff;
  font-size: 23rpx;
  font-weight: 950;
}

.empty-state {
  margin-top: 26rpx;
  padding: 64rpx 24rpx;
  border-radius: 24rpx;
  background: rgba(255, 255, 255, 0.88);
  color: #7b8496;
  text-align: center;
  font-size: 24rpx;
  font-weight: 900;
}

.empty-state.error {
  color: #df3d3d;
}

.load-more-state {
  display: flex;
  justify-content: center;
  padding: 26rpx 0 8rpx;
  color: #8a93a6;
  font-size: 22rpx;
  font-weight: 850;
}

.work-list {
  display: grid;
  gap: 28rpx;
  margin-top: 26rpx;
}

.work-list.grid {
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 20rpx;
}

.work-card {
  width: 100%;
  min-width: 0;
  overflow: hidden;
  padding: 26rpx;
  border-radius: 26rpx;
  background: rgba(255, 255, 255, 0.9);
  box-shadow: 0 18rpx 46rpx rgba(31, 45, 82, 0.06);
}

.work-card.album-card {
  background:
    linear-gradient(135deg, rgba(255, 250, 252, 0.95), rgba(246, 251, 255, 0.94)),
    #fff;
  box-shadow: 0 20rpx 54rpx rgba(154, 54, 88, 0.1);
}

.album-card .cover-wrap {
  height: 292rpx;
}

.album-card .cover-placeholder {
  color: #9f1239;
  background:
    linear-gradient(135deg, rgba(255, 235, 242, 0.98), rgba(236, 246, 255, 0.96)),
    #f8edf3;
}

.album-card .mode-pill {
  background: rgba(225, 29, 72, 0.1);
  color: #be123c;
}

.album-count-badge {
  background: rgba(225, 29, 72, 0.9);
}

.album-meta-line {
  color: #7b5362;
}

.album-card .card-toolbar button {
  flex-basis: 50%;
}

.work-list.grid .work-card {
  padding: 18rpx;
}

.work-list.grid .work-main {
  display: block;
}

.work-list.grid .cover-wrap {
  width: 100%;
  height: auto;
  aspect-ratio: 1 / 0.78;
}

.work-list.grid .work-info {
  margin-top: 16rpx;
}

.work-list.grid .title-line {
  align-items: center;
}

.work-list.grid .visibility-actions {
  gap: 6rpx;
}

.work-list.grid .visibility-button {
  width: 42rpx;
  height: 42rpx;
}

.work-list.grid .work-excerpt,
.work-list.grid .meta-line text:nth-child(-n + 6) {
  display: none;
}

.work-list.grid .mode-pill {
  margin-top: 12rpx;
}

.work-list.grid .card-toolbar {
  height: auto;
  flex-wrap: wrap;
}

.work-list.grid .card-toolbar button {
  flex: 0 0 50%;
  height: 58rpx;
  font-size: 19rpx;
}

.work-main {
  min-width: 0;
  align-items: flex-start;
  gap: 22rpx;
}

.cover-wrap {
  position: relative;
  flex: 0 0 auto;
  width: 232rpx;
  height: 180rpx;
  overflow: hidden;
  border-radius: 12rpx;
  background: #edf2fb;
}

.cover {
  display: block;
  width: 100%;
  height: 100%;
}

.cover-placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 14rpx;
  width: 100%;
  height: 100%;
  background:
    linear-gradient(135deg, rgba(247, 250, 255, 0.98), rgba(234, 240, 252, 0.96)),
    #edf2fb;
  color: #7c879d;
  font-size: 21rpx;
  font-weight: 950;
}

.placeholder-orbit {
  position: relative;
  width: 48rpx;
  height: 48rpx;
  border-radius: 50%;
  background: rgba(112, 98, 255, 0.12);
}

.placeholder-orbit::before,
.placeholder-orbit::after {
  content: '';
  position: absolute;
  border-radius: 999rpx;
}

.placeholder-orbit::before {
  inset: 10rpx;
  background: linear-gradient(135deg, #9954ff, #246cff);
}

.placeholder-orbit::after {
  left: 7rpx;
  top: 7rpx;
  width: 34rpx;
  height: 34rpx;
  border: 3rpx solid rgba(49, 92, 255, 0.26);
  border-top-color: rgba(49, 92, 255, 0.78);
}

.cover-placeholder.running .placeholder-orbit::after {
  animation: placeholder-spin 1s linear infinite;
}

@keyframes placeholder-spin {
  to {
    transform: rotate(360deg);
  }
}

.cover-ratio-badge {
  position: absolute;
  left: 12rpx;
  bottom: 10rpx;
  display: flex;
  align-items: center;
  gap: 6rpx;
  height: 34rpx;
  padding: 0 10rpx;
  border-radius: 8rpx;
  background: rgba(17, 24, 39, 0.72);
  color: #fff;
  font-size: 18rpx;
  font-weight: 900;
}

.batch-count-badge {
  position: absolute;
  right: 12rpx;
  top: 10rpx;
  height: 34rpx;
  padding: 0 12rpx;
  border-radius: 8rpx;
  background: rgba(49, 92, 255, 0.86);
  color: #fff;
  font-size: 18rpx;
  font-weight: 950;
  line-height: 34rpx;
}

.work-info {
  flex: 1;
  min-width: 0;
}

.title-line {
  min-width: 0;
  align-items: flex-start;
  gap: 16rpx;
}

.work-title {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  color: #172033;
  font-size: 27rpx;
  font-weight: 950;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.visibility-actions {
  flex: 0 0 auto;
  gap: 10rpx;
}

.visibility-button {
  justify-content: center;
  width: 50rpx;
  height: 50rpx;
  border-radius: 50%;
  background: rgba(244, 247, 252, 0.92);
  color: #111b34;
  font-size: 24rpx;
  font-weight: 950;
}

.visibility-button.public,
.visibility-button.running {
  background: rgba(37, 214, 134, 0.12);
  color: #23c982;
}

.visibility-button.private {
  background: rgba(135, 89, 255, 0.12);
  color: #8655ff;
}

.visibility-button.favorite.active {
  background: rgba(255, 183, 0, 0.16);
}

.work-excerpt {
  margin-top: 16rpx;
  color: #8390aa;
  font-size: 22rpx;
  font-weight: 800;
  line-height: 1.5;
}

.mode-pill {
  width: fit-content;
  margin-top: 10rpx;
  padding: 8rpx 12rpx;
  border-radius: 8rpx;
  background: rgba(121, 86, 255, 0.1);
  color: #7457ff;
  font-size: 20rpx;
  font-weight: 950;
}

.meta-line {
  flex-wrap: wrap;
  gap: 9rpx;
  margin-top: 18rpx;
  color: #8390aa;
  font-size: 21rpx;
  font-weight: 850;
}

.status {
  font-weight: 950;
}

.status.public,
.visibility-dot.public {
  color: #22c77a;
}

.status.running,
.visibility-dot.running {
  color: #f59e0b;
}

.status.private,
.visibility-dot.private {
  color: #8655ff;
}

.status.favorite,
.visibility-dot.favorite {
  color: #ffb000;
}

.thumb-row {
  gap: 10rpx;
  margin-top: 18rpx;
}

.thumb-row image {
  width: 76rpx;
  height: 64rpx;
  border-radius: 9rpx;
  background: #eef3ff;
}

.progress-block {
  margin-top: 16rpx;
}

.progress-block text {
  color: #7d8799;
  font-size: 20rpx;
  font-weight: 800;
}

.progress-block > view {
  height: 7rpx;
  margin-top: 10rpx;
  overflow: hidden;
  border-radius: 999rpx;
  background: #e2e7ef;
}

.progress-block > view > view {
  height: 100%;
  border-radius: inherit;
  background: linear-gradient(90deg, #a94af3, #5572ff);
}

.pending-failure {
  display: block;
  margin-top: 14rpx;
  color: #d33c3c;
  font-size: 21rpx;
  font-weight: 800;
  line-height: 1.45;
}

.card-toolbar {
  margin-top: 24rpx;
  height: 66rpx;
  overflow: hidden;
  border: 1rpx solid rgba(143, 154, 177, 0.14);
  border-radius: 14rpx;
  background: rgba(255, 255, 255, 0.72);
}

.card-toolbar button {
  position: relative;
  flex: 1;
  justify-content: center;
  gap: 10rpx;
  min-width: 0;
  height: 100%;
  color: #26304a;
  font-size: 22rpx;
  font-weight: 900;
  background: transparent;
}

.card-toolbar button + button::before {
  content: '';
  position: absolute;
  left: 0;
  top: 14rpx;
  bottom: 14rpx;
  width: 1rpx;
  background: rgba(143, 154, 177, 0.18);
}

.card-toolbar button.primary {
  color: #7657ff;
}

.card-toolbar button text {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.preview-overlay {
  position: fixed;
  inset: 0;
  z-index: 60;
  justify-content: center;
  padding: calc(36rpx + env(safe-area-inset-top)) 28rpx calc(36rpx + env(safe-area-inset-bottom));
  background: rgba(14, 20, 36, 0.78);
}

.preview-panel {
  width: 100%;
  max-width: 720rpx;
  overflow: hidden;
  border-radius: 18rpx;
  background: #0f172a;
  box-shadow: 0 28rpx 80rpx rgba(0, 0, 0, 0.32);
}

.preview-header,
.preview-footer {
  justify-content: space-between;
  min-height: 88rpx;
  padding: 0 24rpx;
  color: #fff;
}

.preview-header text {
  min-width: 0;
  overflow: hidden;
  font-size: 25rpx;
  font-weight: 950;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.preview-header button {
  width: 58rpx;
  height: 58rpx;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.12);
  color: #fff;
  font-size: 34rpx;
  font-weight: 800;
  line-height: 58rpx;
}

.work-preview-swiper {
  width: 100%;
  height: 820rpx;
  background: #050816;
}

.work-preview-swiper image {
  width: 100%;
  height: 100%;
}

.preview-footer {
  border-top: 1rpx solid rgba(255, 255, 255, 0.08);
}

.preview-footer text {
  color: rgba(255, 255, 255, 0.72);
  font-size: 22rpx;
  font-weight: 850;
}

.preview-footer button {
  height: 58rpx;
  padding: 0 22rpx;
  border-radius: 12rpx;
  background: #315cff;
  color: #fff;
  font-size: 22rpx;
  font-weight: 950;
  line-height: 58rpx;
}

.native-share-overlay {
  position: fixed;
  inset: 0;
  z-index: 70;
  justify-content: flex-end;
  padding: 28rpx 24rpx calc(28rpx + env(safe-area-inset-bottom));
  background: rgba(14, 20, 36, 0.56);
}

.native-share-panel {
  width: 100%;
  border-radius: 18rpx;
  background: #fff;
  box-shadow: 0 28rpx 72rpx rgba(16, 24, 40, 0.24);
}

.native-share-header {
  justify-content: space-between;
  min-height: 88rpx;
  padding: 0 26rpx;
  border-bottom: 1rpx solid rgba(18, 27, 51, 0.08);
}

.native-share-header text {
  color: #121b33;
  font-size: 27rpx;
  font-weight: 950;
}

.native-share-header button {
  width: 56rpx;
  height: 56rpx;
  border-radius: 50%;
  background: rgba(18, 27, 51, 0.08);
  color: #4d5874;
  font-size: 32rpx;
  font-weight: 850;
  line-height: 56rpx;
}

.native-share-summary {
  gap: 20rpx;
  padding: 24rpx 26rpx 8rpx;
}

.native-share-summary image {
  width: 104rpx;
  height: 104rpx;
  flex: 0 0 104rpx;
  border-radius: 14rpx;
  background: #eef3ff;
}

.native-share-summary view {
  min-width: 0;
  gap: 8rpx;
}

.native-share-summary text {
  display: block;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.native-share-summary text:first-child {
  color: #121b33;
  font-size: 26rpx;
  font-weight: 950;
}

.native-share-summary text:last-child {
  color: $muted;
  font-size: 22rpx;
  font-weight: 820;
}

.native-share-button {
  justify-content: center;
  gap: 12rpx;
  height: 82rpx;
  margin: 22rpx 26rpx 28rpx;
  border-radius: 16rpx;
  background: #315cff;
  color: #fff;
  font-size: 25rpx;
  font-weight: 950;
  line-height: 82rpx;
}

.native-share-button image {
  width: 30rpx;
  height: 30rpx;
  filter: brightness(0) invert(1);
}

@media (max-width: 360px) {
  .app-shell {
    padding-left: 24rpx;
    padding-right: 24rpx;
  }

  .category-tabs {
    gap: 8rpx;
  }

  .category-tabs button,
  .category-tabs uni-button {
    padding: 0 14rpx;
    font-size: 22rpx;
  }

  .filter-bar {
    align-items: stretch;
  }

  .filter-row {
    gap: 8rpx;
  }

  .filter-row button {
    height: 64rpx;
    padding: 0 10rpx;
    font-size: 20rpx;
  }

  .utility-actions {
    gap: 8rpx;
  }

  .utility-actions button {
    width: 64rpx;
    height: 64rpx;
  }

  .work-main {
    gap: 16rpx;
  }

  .cover-wrap {
    width: 190rpx;
    height: 154rpx;
  }

  .work-title {
    font-size: 24rpx;
  }

  .visibility-actions {
    gap: 6rpx;
  }

  .visibility-button {
    width: 42rpx;
    height: 42rpx;
  }

  .card-toolbar button {
    gap: 6rpx;
    font-size: 19rpx;
  }

}
</style>
