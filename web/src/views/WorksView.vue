<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { api } from '../api/client.js'
import ClickSelect from '../components/ClickSelect.vue'
import { usePointerZoom } from '../composables/usePointerZoom.js'

const router = useRouter()
const route = useRoute()

const categoryOptions = [
  { value: 'all', label: '全部' },
  { value: 'image', label: '图片' },
  { value: 'video', label: '视频' },
  { value: 'poster_kv', label: '海报KV' },
  { value: 'product_main', label: '商品主图' },
  { value: 'cover', label: '封面图' }
]

const timeRangeOptions = [
  { value: 'all', label: '全部时间' },
  { value: 'today', label: '今天' },
  { value: 'week', label: '本周' },
  { value: 'month', label: '本月' }
]

const sortOptions = [
  { value: 'recent', label: '按最近时间' },
  { value: 'oldest', label: '按最早时间' }
]

const defaultSummary = {
  total: 0,
  week_new: 0,
  stored_percent: 0,
  private_count: 0,
  category_counts: {}
}

const query = ref('')
const activeCategory = ref('all')
const timeRange = ref('all')
const sortMode = ref('recent')
const favoriteOnly = ref(false)
const viewMode = ref('grid')
const worksPageSize = 30
const works = ref([])
const albums = ref([])
const total = ref(0)
const summary = ref({ ...defaultSummary })
const loading = ref(false)
const loadingMore = ref(false)
const currentPage = ref(1)
const hasMore = ref(false)
const message = ref('')
const errorMessage = ref('')
const openMenuId = ref(null)
const previewOpen = ref(false)
const previewWork = ref(null)
const previewIndex = ref(0)
const loadMoreSentinel = ref(null)

let loadMoreObserver = null

const {
  imageRef: previewImageRef,
  zoomStyle: previewZoomStyle,
  handleWheel: handlePreviewWheel,
  resetZoom: resetPreviewZoom
} = usePointerZoom()

const currentCategoryLabel = computed(() => {
  return categoryOptions.find((item) => item.value === activeCategory.value)?.label ?? '全部'
})

const visibleAlbumCards = computed(() => {
  if (activeCategory.value !== 'all' || favoriteOnly.value) return []
  const keyword = query.value.trim().toLowerCase()
  return albums.value
    .map((album) => buildAlbumCard(album))
    .filter((album) => album && albumMatchesQuery(album, keyword))
})

const visibleTotal = computed(() => total.value + visibleAlbumCards.value.length)

const hasLibraryItems = computed(() => visibleAlbumCards.value.length > 0 || displayWorks.value.length > 0)

const statCards = computed(() => [
  {
    tone: 'purple',
    icon: '▱',
    label: '总作品',
    value: summary.value.total + visibleAlbumCards.value.length,
    suffix: '个'
  },
  {
    tone: 'blue',
    icon: '↗',
    label: '本周新增',
    value: summary.value.week_new,
    suffix: '个'
  },
  {
    tone: 'green',
    icon: '✓',
    label: '已入库',
    value: `${summary.value.stored_percent}%`,
    suffix: '作品安全保存'
  },
  {
    tone: 'orange',
    icon: '▢',
    label: '默认私有',
    value: summary.value.private_count ? '仅你可见' : '未公开',
    suffix: ''
  }
])

const displayWorks = computed(() => groupWorksForDisplay(works.value))
const previewItems = computed(() => workBatchItems(previewWork.value).filter((item) => item.preview_url || item.download_url))
const previewCurrent = computed(() => previewItems.value[previewIndex.value] ?? previewItems.value[0] ?? previewWork.value)
const shareableWorkIds = computed(() => flattenBatchItems(displayWorks.value)
  .map((item) => workID(item))
  .filter(Boolean)
  .slice(0, 16)
)

function categoryLabel(category) {
  const normalized = category || 'image'
  return categoryOptions.find((item) => item.value === normalized)?.label ?? '图片'
}

function isVideoWork(item) {
  if (isBatchWork(item)) return isVideoWork(workBatchItems(item)[0] ?? {})
  return item.category === 'video' || String(item.mime_type || '').startsWith('video/')
}

function mediaTypeLabel(item) {
  if (isVideoWork(item)) return 'MP4'
  return item.mime_type ? item.mime_type.replace('image/', '').toUpperCase() : '图片'
}

function itemTime(item) {
  return Date.parse(item?.created_at || item?.updated_at || '') || 0
}

function isBatchWork(item) {
  return Boolean(item?.is_batch && Array.isArray(item.batch_items))
}

function workBatchItems(item) {
  if (!item) return []
  return isBatchWork(item) ? item.batch_items : [item]
}

function primaryWork(item) {
  return workBatchItems(item).find((work) => work?.preview_url || work?.download_url) ?? workBatchItems(item)[0] ?? item
}

function workCount(item) {
  const items = workBatchItems(item)
  const declaredTotal = Math.max(...items.map((work) => Number(work.batch_total || work.image_count || 0)), 0)
  return Math.max(declaredTotal, items.length, 1)
}

function groupWorksForDisplay(items) {
  const batchGroups = new Map()
  const legacyItems = []
  items.forEach((item) => {
    const batchID = `${item?.batch_id || ''}`.trim()
    if (!batchID) {
      legacyItems.push(item)
      return
    }
    if (!batchGroups.has(batchID)) batchGroups.set(batchID, [])
    batchGroups.get(batchID).push(item)
  })

  const grouped = [...batchGroups.entries()].map(([batchID, batchItems]) => buildBatchWork(batchID, batchItems))
  return mergeLegacyBatchGroups(grouped, legacyItems).sort((left, right) => itemTime(primaryWork(right)) - itemTime(primaryWork(left)))
}

function buildBatchWork(batchID, batchItems, expectedTotal = 0) {
  const sortedItems = [...batchItems].sort((left, right) => {
    const leftIndex = Number(left.batch_index)
    const rightIndex = Number(right.batch_index)
    if (Number.isFinite(leftIndex) && Number.isFinite(rightIndex) && leftIndex !== rightIndex) {
      return leftIndex - rightIndex
    }
    return itemTime(left) - itemTime(right)
  })
  const cover = sortedItems.find((item) => item.preview_url || item.download_url) ?? sortedItems[0] ?? {}
  return {
    ...cover,
    batch_id: batchID,
    batch_items: sortedItems,
    is_batch: true,
    image_count: Math.max(expectedTotal, ...sortedItems.map((item) => Number(item.batch_total || item.image_count || 0)), sortedItems.length)
  }
}

function fallbackGroupKey(item) {
  const promptText = `${item?.prompt || ''}`.trim()
  if (!promptText) return ''
  return `${promptText}|${item?.aspect_ratio || '1:1'}|${item?.category || 'image'}`.toLowerCase()
}

function areWorksNearInTime(left, right) {
  const leftTime = itemTime(primaryWork(left))
  const rightTime = itemTime(primaryWork(right))
  if (!leftTime || !rightTime) return true
  return Math.abs(leftTime - rightTime) <= 5 * 60 * 1000
}

function flattenBatchItems(items) {
  return items.flatMap((item) => workBatchItems(item))
}

function mergeLegacyBatchGroups(groupedItems, legacyItems) {
  const buckets = new Map()
  const passthrough = []
  ;[...groupedItems, ...legacyItems].forEach((item) => {
    if (item?.batch_id && !`${item.batch_id}`.startsWith('fallback-')) {
      passthrough.push(item)
      return
    }
    const key = fallbackGroupKey(primaryWork(item))
    if (!key) {
      passthrough.push(item)
      return
    }
    if (!buckets.has(key)) buckets.set(key, [])
    buckets.get(key).push(item)
  })

  const merged = []
  buckets.forEach((bucketItems, key) => {
    const ordered = [...bucketItems].sort((left, right) => itemTime(primaryWork(left)) - itemTime(primaryWork(right)))
    let cluster = []
    ordered.forEach((item) => {
      if (cluster.length > 0 && (!areWorksNearInTime(cluster[cluster.length - 1], item) || flattenBatchItems(cluster).length >= 4)) {
        merged.push(cluster.length > 1 ? buildBatchWork(`fallback-${key}-${itemTime(primaryWork(cluster[0]))}`, flattenBatchItems(cluster)) : cluster[0])
        cluster = []
      }
      cluster.push(item)
    })
    if (cluster.length > 0) {
      merged.push(cluster.length > 1 ? buildBatchWork(`fallback-${key}-${itemTime(primaryWork(cluster[0]))}`, flattenBatchItems(cluster)) : cluster[0])
    }
  })

  return [...passthrough, ...merged]
}

function categoryCount(category) {
  if (category === 'all') {
    return summary.value.total + visibleAlbumCards.value.length
  }
  return summary.value.category_counts?.[category] ?? 0
}

function albumStatusLabel(status) {
  switch (status) {
    case 'succeeded':
      return '已完成'
    case 'generating':
      return '生成中'
    case 'failed':
      return '生成失败'
    default:
      return '待生成'
  }
}

function buildAlbumCard(album) {
  if (!album?.id) return null
  const pages = Array.isArray(album.pages) ? album.pages : []
  const cover = pages.find((page) => Number(page.id) === Number(album.cover_page_id) && page.preview_url) ??
    pages.find((page) => page.status === 'succeeded' && page.preview_url) ??
    pages.find((page) => page.preview_url) ??
    null
  const doneCount = pages.filter((page) => page.status === 'succeeded' && page.preview_url).length
  return {
    id: album.id,
    title: album.title || '未命名相册',
    location: album.location || '',
    status: album.status || 'pending',
    status_label: albumStatusLabel(album.status),
    page_count: pages.length,
    done_count: doneCount,
    preview_url: cover?.preview_url || '',
    cover_title: cover?.page_title || album.title || '相册封面',
    created_at: album.created_at,
    updated_at: album.updated_at,
    searchable: [
      album.title,
      album.location,
      ...pages.flatMap((page) => [page.page_title, page.caption])
    ].filter(Boolean).join(' ').toLowerCase()
  }
}

function albumMatchesQuery(album, keyword) {
  if (!keyword) return true
  return album.searchable.includes(keyword)
}

function albumMeta(album) {
  const location = album.location ? `${album.location} · ` : ''
  return `${location}${album.done_count}/${album.page_count || 0} 页 · ${album.status_label}`
}

function openAlbum(albumID) {
  router.push(`/workspace/couple-album/${albumID}`)
}

function formatTimestamp(value) {
  if (!value) {
    return '刚刚生成'
  }
  return new Date(value).toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  })
}

function frameAspectStyle(item) {
  const ratio = String(primaryWork(item)?.aspect_ratio || '').trim()
  const match = ratio.match(/^(\d+(?:\.\d+)?)\s*[:/]\s*(\d+(?:\.\d+)?)$/)
  if (!match) return { aspectRatio: '1 / 1' }
  const width = Number(match[1])
  const height = Number(match[2])
  if (!width || !height) return { aspectRatio: '1 / 1' }
  // 限制极端比例（如 21:9 / 9:21），避免卡片过扁或过长
  const clamped = Math.min(Math.max(width / height, 0.6), 1.91)
  return { aspectRatio: `${clamped}` }
}

function workTitle(item) {
  return primaryWork(item)?.prompt || '未命名作品'
}

function shortWorkTitle(item, maxLength = 46) {
  const title = workTitle(item).replace(/\s+/g, ' ').trim()
  if (title.length <= maxLength) return title
  return `${title.slice(0, maxLength).trimEnd()}...`
}

function buildListParams(page = 1) {
  return {
    q: query.value.trim(),
    category: activeCategory.value,
    time_range: timeRange.value,
    sort: sortMode.value,
    favorite: favoriteOnly.value ? true : undefined,
    exclude_album_pages: true,
    page,
    page_size: worksPageSize
  }
}

function workKey(item) {
  if (isBatchWork(item)) return `batch-${item.batch_id || primaryWork(item)?.work_id || primaryWork(item)?.id || ''}`
  return String(item?.work_id ?? item?.id ?? '')
}

function workID(item) {
  return primaryWork(item)?.work_id ?? primaryWork(item)?.id
}

function mergeWorks(previousItems, nextItems) {
  const seen = new Set()
  return [...previousItems, ...nextItems].filter((item) => {
    const key = workKey(item)
    if (!key) return true
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
}

function albumPageWorkIDs() {
  return new Set(albums.value.flatMap((album) => Array.isArray(album.pages)
    ? album.pages.map((page) => Number(page.work_id || 0)).filter(Boolean)
    : []
  ))
}

function excludeAlbumPageWorks(items) {
  const excludedIDs = albumPageWorkIDs()
  if (excludedIDs.size === 0) return items
  return items.filter((item) => !excludedIDs.has(Number(workID(item) || 0)))
}

function applyListPayload(payload, page, append) {
  const nextItems = excludeAlbumPageWorks(payload.items ?? [])
  works.value = append ? mergeWorks(works.value, nextItems) : nextItems
  total.value = payload.total ?? works.value.length
  summary.value = {
    ...defaultSummary,
    ...(payload.summary ?? {})
  }
  currentPage.value = payload.page ?? page
  hasMore.value = works.value.length < total.value
}

async function load(page = 1, { append = false } = {}) {
  if (append) {
    loadingMore.value = true
  } else {
    loading.value = true
  }
  errorMessage.value = ''
  try {
    const [payload, albumsPayload] = append
      ? [await api.listWorks(buildListParams(page)), null]
      : await Promise.all([
          api.listWorks(buildListParams(page)),
          api.listCoupleAlbums()
        ])
    if (albumsPayload) {
      albums.value = albumsPayload.albums ?? []
    }
    applyListPayload(payload, page, append)
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    if (append) {
      loadingMore.value = false
    } else {
      loading.value = false
    }
  }
}

async function resetAndLoad() {
  currentPage.value = 1
  hasMore.value = false
  await load(1)
}

async function loadNextPage() {
  if (loading.value || loadingMore.value || !hasMore.value) return
  await load(currentPage.value + 1, { append: true })
}

function setupLoadMoreObserver() {
  if (!loadMoreSentinel.value || typeof window === 'undefined' || !window.IntersectionObserver) return
  loadMoreObserver = new window.IntersectionObserver((entries) => {
    if (entries.some((entry) => entry.isIntersecting)) {
      void loadNextPage()
    }
  }, {
    rootMargin: '240px 0px'
  })
  loadMoreObserver.observe(loadMoreSentinel.value)
}

async function selectCategory(category) {
  activeCategory.value = category
  await resetAndLoad()
}

function openPreview(item) {
  if (!primaryWork(item)?.preview_url) return
  previewWork.value = item
  previewIndex.value = 0
  previewOpen.value = true
  openMenuId.value = null
  resetPreviewZoom()
}

function closePreview() {
  previewOpen.value = false
  previewWork.value = null
  previewIndex.value = 0
  resetPreviewZoom()
}

function showPreviousPreview() {
  if (previewItems.value.length <= 1) return
  previewIndex.value = (previewIndex.value - 1 + previewItems.value.length) % previewItems.value.length
  resetPreviewZoom()
}

function showNextPreview() {
  if (previewItems.value.length <= 1) return
  previewIndex.value = (previewIndex.value + 1) % previewItems.value.length
  resetPreviewZoom()
}

function handlePreviewKeydown(event) {
  if (event.key === 'Escape' && previewOpen.value) {
    closePreview()
  }
}

async function reuse(workId) {
  try {
    const payload = await api.reuseWork(workId)
    window.sessionStorage?.setItem('image_agent_workspace_prefill:v1', JSON.stringify(payload))
    router.push('/workspace')
  } catch (error) {
    errorMessage.value = error.message
  }
}

function patchWorkInList(workId, patch) {
  works.value = works.value.map((item) => {
    if (workID(item) === workId) {
      return { ...item, ...patch }
    }
    if (isBatchWork(item)) {
      return {
        ...item,
        batch_items: item.batch_items.map((batchItem) => workID(batchItem) === workId ? { ...batchItem, ...patch } : batchItem)
      }
    }
    return item
  })
  if (previewWork.value) {
    previewWork.value = works.value.find((item) => workKey(item) === workKey(previewWork.value)) || previewWork.value
  }
}

async function toggleFavorite(workId) {
  const item = flattenBatchItems(works.value).find((work) => workID(work) === workId)
  const nextValue = !item?.is_favorite
  try {
    await api.updateWork(workId, { is_favorite: nextValue })
    patchWorkInList(workId, { is_favorite: nextValue })
    message.value = nextValue ? '已收藏。' : '已取消收藏。'
    openMenuId.value = null
  } catch (error) {
    errorMessage.value = error.message
  }
}

async function toggleFavoriteFilter() {
  favoriteOnly.value = !favoriteOnly.value
  await resetAndLoad()
}

async function shareWorks() {
  const ids = shareableWorkIds.value
  if (ids.length === 0) return
  const selectedWorks = flattenBatchItems(displayWorks.value)
    .filter((item) => ids.includes(workID(item)))
  const privateWorks = selectedWorks.filter((item) => item.visibility === 'private')
  if (privateWorks.length > 0 && !window.confirm('分享前需要将私有作品转为公开，是否继续？')) return
  try {
    await Promise.all(privateWorks.map((item) => api.updateWork(workID(item), { visibility: 'public' })))
    privateWorks.forEach((item) => patchWorkInList(workID(item), { visibility: 'public' }))
    router.push(`/works/share?ids=${ids.join(',')}`)
  } catch (error) {
    errorMessage.value = error.message
  }
}

function toggleMenu(workId) {
  openMenuId.value = openMenuId.value === workId ? null : workId
}

async function remove(workId) {
  try {
    await api.deleteWork(workId)
    message.value = '作品已删除。'
    openMenuId.value = null
    if (previewWork.value?.work_id === workId) {
      closePreview()
    }
    await resetAndLoad()
  } catch (error) {
    errorMessage.value = error.message
  }
}

onMounted(() => {
  const routeCategory = String(route.query.category || '')
  if (categoryOptions.some((item) => item.value === routeCategory)) {
    activeCategory.value = routeCategory
  }
  window.addEventListener('keydown', handlePreviewKeydown)
  setupLoadMoreObserver()
  resetAndLoad()
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', handlePreviewKeydown)
  loadMoreObserver?.disconnect()
})
</script>

<template>
  <section class="works-page works-library-page">
    <div class="works-library-hero">
      <div class="works-library-copy">
        <p class="eyebrow">LIBRARY</p>
        <h1>你的私有作品库</h1>
        <p>这里是你的专属素材空间，搜索、浏览、复用、下载与管理你生成的图片和视频。</p>
      </div>

      <div class="works-library-tools">
        <form class="works-search-form" data-testid="works-search-form" @submit.prevent="resetAndLoad">
          <label class="works-search-box">
            <span aria-hidden="true">⌕</span>
            <input
              v-model="query"
              data-testid="works-search-input"
              type="search"
              placeholder="搜索作品标题、关键词或描述"
            />
          </label>
          <button class="works-filter-button" type="submit">
            <span aria-hidden="true">▽</span>
            筛选
          </button>
        </form>

        <div class="works-select-row">
          <button
            class="works-filter-button"
            :class="{ active: favoriteOnly }"
            data-testid="works-favorite-filter"
            type="button"
            @click="toggleFavoriteFilter"
          >
            {{ favoriteOnly ? '全部作品' : '收藏' }}
          </button>
          <ClickSelect v-model="timeRange" :options="timeRangeOptions" data-testid="works-time-range" aria-label="时间范围" @change="resetAndLoad" />
          <ClickSelect
            v-model="activeCategory"
            :options="categoryOptions.map((item) => ({ value: item.value, label: item.label === '全部' ? '全部类型' : item.label }))"
            aria-label="作品类型"
            @change="resetAndLoad"
          />
          <ClickSelect v-model="sortMode" :options="sortOptions" data-testid="works-sort" aria-label="排序" @change="resetAndLoad" />
        </div>
      </div>
    </div>

    <div class="works-stat-grid">
      <article
        v-for="item in statCards"
        :key="item.label"
        :class="['works-stat-card', `works-stat-card-${item.tone}`]"
      >
        <span class="works-stat-icon">{{ item.icon }}</span>
        <div>
          <span>{{ item.label }}</span>
          <strong>{{ item.value }} <small v-if="item.suffix && item.label !== '已入库'">{{ item.suffix }}</small></strong>
          <small v-if="item.label === '已入库'">{{ item.suffix }}</small>
        </div>
      </article>
    </div>

    <div class="works-filter-shell" data-testid="works-filter-bar">
      <div class="works-category-tabs">
        <button
          v-for="item in categoryOptions"
          :key="item.value"
          :class="['works-category-tab', { 'works-category-tab-active': activeCategory === item.value }]"
          :data-testid="`works-category-${item.value}`"
          type="button"
          @click="selectCategory(item.value)"
        >
          <span aria-hidden="true">{{ item.value === 'all' ? '▦' : '▧' }}</span>
          {{ item.label }}
          <small>{{ categoryCount(item.value) }}</small>
        </button>
      </div>

      <div class="works-view-toggle" aria-label="视图切换">
        <button
          type="button"
          data-testid="works-share-selected"
          aria-label="分享当前作品"
          @click="shareWorks"
        >
          分享
        </button>
        <button
          :class="{ active: viewMode === 'grid' }"
          type="button"
          aria-label="网格视图"
          @click="viewMode = 'grid'"
        >
          ▦
        </button>
        <button
          :class="{ active: viewMode === 'list' }"
          type="button"
          aria-label="列表视图"
          @click="viewMode = 'list'"
        >
          ☰
        </button>
      </div>
    </div>

    <div class="works-section-head">
      <strong>{{ currentCategoryLabel }}作品</strong>
      <span>共 {{ visibleTotal }} 个</span>
    </div>

    <div v-if="hasLibraryItems" :class="['works-library-grid', { 'works-library-list': viewMode === 'list' }]">
      <article
        v-for="album in visibleAlbumCards"
        :key="`album-${album.id}`"
        class="works-library-card works-album-card"
        :data-testid="`works-album-card-${album.id}`"
        @click="openAlbum(album.id)"
      >
        <div class="works-card-frame">
          <img
            v-if="album.preview_url"
            :src="album.preview_url"
            :alt="album.cover_title"
            :title="album.title"
          />
          <div v-else class="works-card-placeholder">等待相册</div>
          <span class="ai-content-badge">AI生成</span>
          <span class="works-card-badge">相册</span>
        </div>

        <div class="works-card-body">
          <div>
            <h2 :title="album.title">{{ album.title }}</h2>
            <p>{{ albumMeta(album) }}</p>
          </div>
          <div class="works-card-tags">
            <span>相册</span>
            <span>{{ album.status_label }}</span>
            <span>{{ album.done_count }}/{{ album.page_count || 0 }} 页</span>
          </div>
        </div>

        <div class="works-card-actions works-album-actions">
          <button :data-testid="`works-album-view-${album.id}`" type="button" @click.stop="openAlbum(album.id)">查看相册</button>
        </div>
      </article>

      <article
        v-for="item in displayWorks"
        :key="workKey(item)"
        class="works-library-card"
        :data-testid="`works-card-${workID(item)}`"
      >
        <div class="works-card-frame" :style="frameAspectStyle(item)">
          <video
            v-if="isVideoWork(item) && item.preview_url"
            :src="primaryWork(item).preview_url"
            controls
            muted
            playsinline
          />
          <img
            v-else-if="primaryWork(item).preview_url"
            :src="primaryWork(item).preview_url"
            :alt="shortWorkTitle(item)"
            :title="workTitle(item)"
          />
          <div v-else class="works-card-placeholder">等待作品</div>
          <span class="ai-content-badge">AI生成</span>
          <span class="works-card-badge">{{ item.visibility === 'private' ? '私有' : '已入库' }}</span>
          <span v-if="isBatchWork(item)" class="works-card-badge works-card-count-badge">{{ workCount(item) }}张</span>
        </div>

        <div class="works-card-body">
          <div>
            <h2 :title="workTitle(item)">{{ shortWorkTitle(item) }}</h2>
            <p>{{ formatTimestamp(primaryWork(item).created_at) }} · {{ primaryWork(item).aspect_ratio || '默认画幅' }}</p>
          </div>
          <div class="works-card-tags">
            <span>{{ categoryLabel(primaryWork(item).category) }}</span>
            <span>{{ mediaTypeLabel(item) }}</span>
            <span>{{ workCount(item) }}张</span>
          </div>
        </div>

        <div class="works-card-actions">
          <button :data-testid="`works-view-${workID(item)}`" type="button" @click="openPreview(item)">查看</button>
          <a
            v-if="primaryWork(item).download_url"
            :data-testid="`works-download-${workID(item)}`"
            :href="primaryWork(item).download_url"
          >
            下载
          </a>
          <button :data-testid="`works-reuse-${workID(item)}`" type="button" @click="reuse(workID(item))">复用</button>
          <button
            :data-testid="`works-favorite-${workID(item)}`"
            type="button"
            @click="toggleFavorite(workID(item))"
          >
            {{ primaryWork(item).is_favorite ? '取消收藏' : '收藏' }}
          </button>
          <div class="works-more-menu">
            <button :data-testid="`works-more-${workID(item)}`" type="button" @click="toggleMenu(workID(item))">...</button>
            <div v-if="openMenuId === workID(item)" class="works-more-popover">
              <button
                :data-testid="`works-delete-${workID(item)}`"
                type="button"
                @click="remove(workID(item))"
              >
                删除作品
              </button>
            </div>
          </div>
        </div>
      </article>
    </div>

    <section v-else-if="!loading" class="works-empty-panel">
      <p class="eyebrow">LIBRARY</p>
      <h2>还没有作品，先去工作台完成第一轮生成。</h2>
      <p>作品生成成功后会自动进入这里，随后可以继续搜索、下载、复用或删除。</p>
    </section>

    <p v-if="loading" class="page-status">加载中...</p>
    <p v-else-if="loadingMore" class="page-status">加载更多作品中...</p>
    <p v-else-if="hasLibraryItems && hasMore" class="page-status">向下滚动加载更多作品</p>
    <p v-else-if="hasLibraryItems" class="page-status">已显示全部作品</p>
    <div ref="loadMoreSentinel" class="works-load-more-sentinel" data-testid="works-load-more-sentinel" aria-hidden="true"></div>
    <p v-if="message" class="status-success">{{ message }}</p>
    <p v-if="errorMessage" class="status-error">{{ errorMessage }}</p>

    <div
      v-if="previewOpen && previewWork"
      class="works-preview-modal"
      data-testid="works-preview-modal"
      role="dialog"
      aria-modal="true"
      aria-label="作品放大预览"
      @click="closePreview"
    >
      <div class="works-preview-dialog" @click.stop>
        <div class="works-preview-header">
          <div class="works-preview-title">
            <strong>放大查看</strong>
            <span data-testid="works-preview-title-text" :title="workTitle(previewCurrent)">{{ shortWorkTitle(previewCurrent, 64) }}</span>
          </div>
          <div class="works-preview-actions">
            <a
              v-if="previewCurrent?.download_url"
              class="works-preview-action works-preview-download"
              data-testid="works-preview-download"
              :href="previewCurrent.download_url"
              target="_blank"
              rel="noopener noreferrer"
            >
              下载当前
            </a>
            <button
              class="works-preview-action"
              data-testid="works-preview-reuse"
              type="button"
              @click="reuse(workID(previewCurrent))"
            >
              复用
            </button>
            <button
              class="works-preview-action"
              data-testid="works-preview-collect"
              type="button"
              @click="toggleFavorite(workID(previewCurrent))"
            >
              {{ previewCurrent?.is_favorite ? '取消收藏' : '收藏' }}
            </button>
            <button
              class="works-preview-action works-preview-danger"
              data-testid="works-preview-delete"
              type="button"
              @click="remove(workID(previewCurrent))"
            >
              删除作品
            </button>
            <button
              class="works-preview-close"
              data-testid="works-preview-close"
              type="button"
              aria-label="关闭预览"
              @click="closePreview"
            >
              ×
            </button>
          </div>
        </div>

        <div v-if="isVideoWork(previewCurrent)" class="works-preview-video-wrap">
          <video
            data-testid="works-preview-video"
            :src="previewCurrent.preview_url"
            controls
            playsinline
          />
        </div>
        <div
          v-else
          class="works-preview-image-wrap"
          data-testid="works-preview-zoom-surface"
          @wheel="handlePreviewWheel"
        >
          <img
            ref="previewImageRef"
            data-testid="works-preview-zoom-image"
            :src="previewCurrent.preview_url"
            :alt="shortWorkTitle(previewCurrent, 64)"
            :title="workTitle(previewCurrent)"
            :style="previewZoomStyle"
          />
          <span class="ai-content-badge works-preview-ai-badge">AI生成 / 白霖共享生成</span>
        </div>
        <div v-if="previewItems.length > 1" class="works-preview-batch-nav">
          <button data-testid="works-preview-prev" type="button" @click="showPreviousPreview">上一张</button>
          <span>{{ previewIndex + 1 }} / {{ previewItems.length }}</span>
          <button data-testid="works-preview-next" type="button" @click="showNextPreview">下一张</button>
        </div>
      </div>
    </div>
  </section>
</template>
