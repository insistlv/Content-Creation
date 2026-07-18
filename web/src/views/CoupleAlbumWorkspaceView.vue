<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Camera, Heart, ImagePlus, MapPin, Palette, Rocket, ShieldCheck, Sparkles, X } from 'lucide-vue-next'

import { api } from '../api/client.js'
import ImageUploadZone from '../components/ImageUploadZone.vue'
import SoftPanel from '../components/SoftPanel.vue'
import rocketChildHero from '../image/childhood-dream-album/rocket-child-hero.png'
import themeStageImage from '../image/childhood-dream-album/theme-stage.png'
import themeSpaceImage from '../image/childhood-dream-album/theme-space.png'
import themeFairyTaleImage from '../image/childhood-dream-album/theme-fairy-tale.png'
import themeNatureImage from '../image/childhood-dream-album/theme-nature.png'
import styleStorybookImage from '../image/childhood-dream-album/style-storybook.png'
import styleWatercolorImage from '../image/childhood-dream-album/style-watercolor.png'
import style3dImage from '../image/childhood-dream-album/style-3d.png'
import stylePhotoPosterImage from '../image/childhood-dream-album/style-photo-poster.png'
import sideRocketChildImage from '../image/childhood-dream-album/side-rocket-child.png'
import recentFallbackImage from '../image/childhood-dream-album/recent-fallback.png'

const route = useRoute()
const router = useRouter()

const childhoodDreamPath = '/workspace/childhood-dream-album'
const childhoodLocationValues = new Set([
  'childhood_dream_stage',
  'childhood_space_adventure',
  'childhood_fairy_tale',
  'childhood_nature_explorer'
])
const childhoodStoryTemplateValues = new Set(['childhood_career_dream'])
const childhoodStyleValues = new Set([
  'children_storybook',
  'dreamy_watercolor',
  'animation_3d',
  'children_photo_poster'
])

const defaultLocations = [
  { value: '大理', label: '大理洱海', description: '风吹洱海的蓝色午后', image_url: '/static/couple-album/dali-erhai.png' },
  { value: '京都', label: '京都樱花', description: '樱花雨里的慢镜头', image_url: '/static/couple-album/kyoto-sakura.png' },
  { value: '巴黎', label: '巴黎街角', description: '转角遇见电影感', image_url: '/static/couple-album/paris-corner.png' },
  { value: '厦门', label: '厦门海岸', description: '海风、落日和并肩', image_url: '/static/couple-album/xiamen-coast.png' }
]

const defaultTemplates = [
  { value: 'city_walk', label: '城市漫游', description: '街角、咖啡和夜色' },
  { value: 'first_trip', label: '初次旅行', description: '出发那天的心动' },
  { value: 'anniversary', label: '纪念日', description: '重要日子的慢镜头' },
  { value: 'proposal', label: '求婚时刻', description: '黄昏、灯光和仪式感' }
]

const defaultStyles = [
  { value: 'film', label: '旅行胶片' },
  { value: 'cinematic', label: '电影旅拍' },
  { value: 'watercolor', label: '清透水彩' },
  { value: 'storybook', label: '绘本相册' }
]

const defaultChildhoodLocations = [
  { value: 'childhood_dream_stage', label: '童年梦想舞台', description: '节日舞台里的职业梦想秀', image_url: themeStageImage },
  { value: 'childhood_space_adventure', label: '星际探索之旅', description: '火箭、星球和探索冒险', image_url: themeSpaceImage },
  { value: 'childhood_fairy_tale', label: '童话奇遇记', description: '城堡、森林和奇妙伙伴', image_url: themeFairyTaleImage },
  { value: 'childhood_nature_explorer', label: '自然小达人', description: '森林、昆虫和自然观察', image_url: themeNatureImage }
]

const defaultChildhoodTemplates = [
  { value: 'childhood_career_dream', label: '童年职业梦想', description: '宇航员、医生、画家等 8 页连续故事' }
]

const defaultChildhoodStyles = [
  { value: 'children_storybook', label: '童话绘本', description: '柔和线条与绘本色彩', image_url: styleStorybookImage },
  { value: 'dreamy_watercolor', label: '梦幻水彩', description: '轻盈水彩和透明光感', image_url: styleWatercolorImage },
  { value: 'animation_3d', label: '3D 动画电影', description: '圆润立体的动画质感', image_url: style3dImage },
  { value: 'children_photo_poster', label: '儿童写真海报', description: '明亮干净的写真海报', image_url: stylePhotoPosterImage }
]

const childhoodContentPills = ['8 页连续故事', '6 个职业梦想', '可分享相册', '生成中自动刷新']
const childhoodWhyItems = [
  { icon: ShieldCheck, title: '孩子是唯一主角', text: '参考照片只用于保持孩子身份和年龄感。' },
  { icon: Sparkles, title: '主题真实参与生成', text: '选中的梦想主题会写入 8 页 prompt。' },
  { icon: ImagePlus, title: '补充全身照可选', text: '只上传孩子照片也能创建相册。' }
]

const isChildhoodDreamMode = computed(() => route.path === childhoodDreamPath)
const modeText = computed(() => isChildhoodDreamMode.value
  ? {
      defaultTitle: '我的六一梦想相册',
      loading: '童年梦想相册读取中...',
      loadError: '童年梦想相册读取失败',
      kicker: 'Childhood Dream Album',
      title: '童年梦想相册',
      subtitle: '上传孩子照片，选择梦想主题和画面风格，生成 8 页六一职业梦想故事。',
      composerTitle: '创建梦想相册',
      composerHint: '建议使用 1 张清晰正脸或半身照，可补充 1 张全身照作为服装体态参考。',
      primaryPhoto: '孩子照片',
      secondaryPhoto: '补充全身照',
      locationSection: '梦想主题',
      templateSection: '相册内容',
      submitMissing: '请先填写标题并上传孩子照片',
      recentTitle: '最近梦想相册',
      recentHint: '生成中的梦想相册会在详情页持续刷新进度。',
      emptyTitle: '还没有童年梦想相册',
      emptyHint: '上传孩子照片完成创建后，最近记录会出现在这里。'
    }
  : {
      defaultTitle: '我们的 520 旅行相册',
      loading: '情侣相册读取中...',
      loadError: '情侣相册读取失败',
      kicker: 'Couple Album',
      title: '情侣相册',
      subtitle: '上传双方参考图，选择地点、故事模板和画面风格，生成 8 页可分享旅行相册。',
      composerTitle: '创建相册',
      composerHint: '建议使用清晰正面照或半身照，人物识别会更稳定。',
      primaryPhoto: '第一位主角',
      secondaryPhoto: '第二位主角',
      locationSection: '旅游地点',
      templateSection: '故事模板',
      submitMissing: '请先填写标题并上传双方参考图',
      recentTitle: '最近相册',
      recentHint: '生成中的相册会在详情页持续刷新进度。',
      emptyTitle: '还没有情侣相册',
      emptyHint: '完成创建后，最近记录会出现在这里。'
    })

const loading = ref(true)
const pageError = ref('')
const submitError = ref('')
const submitHint = ref('')
const submitting = ref(false)
const uploadingRole = ref('')
const title = ref(modeText.value.defaultTitle)
const locations = ref(defaultLocations)
const templates = ref(defaultTemplates)
const styles = ref(defaultStyles)
const selectedLocation = ref(defaultLocations[0].value)
const selectedTemplate = ref(defaultTemplates[0].value)
const selectedStyle = ref(defaultStyles[0].value)
const maleAsset = ref(null)
const femaleAsset = ref(null)
const recentAlbums = ref([])
const creditShortfall = ref(null)
const optionPayload = ref({})
const assetPickerOpen = ref(false)
const assetPickerRole = ref('')
const assetPickerAssets = ref([])
const assetPickerLoading = ref(false)
const assetPickerError = ref('')
const selectedAssetPickerId = ref('')

const maleImages = computed(() => maleAsset.value ? [maleAsset.value] : [])
const femaleImages = computed(() => femaleAsset.value ? [femaleAsset.value] : [])
const uploadedCount = computed(() => [maleAsset.value, femaleAsset.value].filter(Boolean).length)
const requiredUploadCount = computed(() => isChildhoodDreamMode.value ? 1 : 2)
const canSubmit = computed(() =>
  !submitting.value &&
  !uploadingRole.value &&
  Boolean(title.value.trim()) &&
  Boolean(maleAsset.value?.id) &&
  (isChildhoodDreamMode.value || Boolean(femaleAsset.value?.id))
)
const locationLabelMap = computed(() => new Map(locations.value.map((item) => [item.value, item.label])))
const selectedAssetPickerItem = computed(() => assetPickerAssets.value.find((asset) => `${asset.id}` === `${selectedAssetPickerId.value}`))
const displayRecentAlbums = computed(() => {
  const albums = Array.isArray(recentAlbums.value) ? recentAlbums.value : []
  return isChildhoodDreamMode.value
    ? albums.filter((album) => album?.story_template === 'childhood_career_dream')
    : albums.filter((album) => album?.story_template !== 'childhood_career_dream')
})

function responseAlbums(payload) {
  if (Array.isArray(payload)) return payload
  if (Array.isArray(payload?.albums)) return payload.albums
  return payload?.items ?? []
}

function responseItems(payload) {
  return Array.isArray(payload) ? payload : (payload?.items ?? [])
}

function normalizeOptions(items, fallbackItems) {
  const source = Array.isArray(items) ? items : []
  const normalized = source
    .map((item) => ({
      value: `${item?.value || ''}`.trim(),
      label: `${item?.label || ''}`.trim(),
      description: `${item?.description || ''}`.trim(),
      image_url: `${item?.image_url || ''}`.trim()
    }))
    .filter((item) => item.value && item.label)
  return normalized.length ? normalized : fallbackItems
}

function keepSelected(currentValue, nextOptions) {
  return nextOptions.some((item) => item.value === currentValue)
    ? currentValue
    : nextOptions[0]?.value || ''
}

function onlyOptions(options, allowedValues, fallbackItems) {
  const filtered = options.filter((item) => allowedValues.has(item.value))
  return filtered.length ? filtered : fallbackItems
}

function withoutOptions(options, blockedValues, fallbackItems) {
  const filtered = options.filter((item) => !blockedValues.has(item.value))
  return filtered.length ? filtered : fallbackItems
}

function applyFallbackMetadata(options, fallbackItems) {
  const fallbackByValue = new Map(fallbackItems.map((item) => [item.value, item]))
  return options.map((item) => {
    const fallback = fallbackByValue.get(item.value)
    if (!fallback) return item
    return {
      ...fallback,
      ...item,
      description: item.description || fallback.description || '',
      image_url: fallback.image_url || item.image_url || ''
    }
  })
}

function applyOptions(payload = {}) {
  const normalizedLocations = normalizeOptions(payload.locations, isChildhoodDreamMode.value ? defaultChildhoodLocations : defaultLocations)
  const normalizedTemplates = normalizeOptions(payload.story_templates, isChildhoodDreamMode.value ? defaultChildhoodTemplates : defaultTemplates)
  const normalizedStyles = normalizeOptions(payload.styles, isChildhoodDreamMode.value ? defaultChildhoodStyles : defaultStyles)

  const nextLocations = isChildhoodDreamMode.value
    ? onlyOptions(normalizedLocations, childhoodLocationValues, defaultChildhoodLocations)
    : withoutOptions(normalizedLocations, childhoodLocationValues, defaultLocations)
  const nextTemplates = isChildhoodDreamMode.value
    ? onlyOptions(normalizedTemplates, childhoodStoryTemplateValues, defaultChildhoodTemplates)
    : withoutOptions(normalizedTemplates, childhoodStoryTemplateValues, defaultTemplates)
  const nextStyles = isChildhoodDreamMode.value
    ? onlyOptions(normalizedStyles, childhoodStyleValues, defaultChildhoodStyles)
    : withoutOptions(normalizedStyles, childhoodStyleValues, defaultStyles)

  locations.value = isChildhoodDreamMode.value ? applyFallbackMetadata(nextLocations, defaultChildhoodLocations) : nextLocations
  templates.value = nextTemplates
  styles.value = isChildhoodDreamMode.value ? applyFallbackMetadata(nextStyles, defaultChildhoodStyles) : nextStyles
  selectedLocation.value = keepSelected(selectedLocation.value, nextLocations)
  selectedTemplate.value = keepSelected(selectedTemplate.value, nextTemplates)
  selectedStyle.value = keepSelected(selectedStyle.value, nextStyles)
}

async function load() {
  loading.value = true
  pageError.value = ''
  try {
    const [optionsPayload, albumsPayload] = await Promise.all([
      api.getCoupleAlbumOptions().catch(() => ({})),
      api.listCoupleAlbums().catch(() => ({ albums: [] }))
    ])
    optionPayload.value = optionsPayload
    applyOptions(optionsPayload)
    recentAlbums.value = responseAlbums(albumsPayload)
  } catch (error) {
    pageError.value = error.message || modeText.value.loadError
  } finally {
    loading.value = false
  }
}

function setRoleAsset(role, asset) {
  if (role === 'male') {
    maleAsset.value = asset
    return
  }
  femaleAsset.value = asset
}

function roleAsset(role) {
  return role === 'male' ? maleAsset.value : femaleAsset.value
}

async function openAssetPicker(role) {
  if (submitting.value || uploadingRole.value) return
  assetPickerOpen.value = true
  assetPickerRole.value = role
  selectedAssetPickerId.value = roleAsset(role)?.id || ''
  assetPickerError.value = ''
  assetPickerLoading.value = true
  try {
    assetPickerAssets.value = responseItems(await api.listReferenceAssets())
  } catch (error) {
    assetPickerAssets.value = []
    assetPickerError.value = error.message || '素材读取失败'
  } finally {
    assetPickerLoading.value = false
  }
}

function closeAssetPicker() {
  assetPickerOpen.value = false
  assetPickerRole.value = ''
  selectedAssetPickerId.value = ''
}

function selectAssetPickerItem(asset) {
  selectedAssetPickerId.value = asset?.id || ''
}

function confirmAssetPicker() {
  const asset = selectedAssetPickerItem.value
  if (!asset || !assetPickerRole.value) return
  setRoleAsset(assetPickerRole.value, {
    ...asset,
    _selectedFromLibrary: true
  })
  submitError.value = ''
  creditShortfall.value = null
  closeAssetPicker()
}

async function uploadReference(role, file) {
  if (submitting.value) return
  submitError.value = ''
  creditShortfall.value = null
  uploadingRole.value = role
  try {
    const uploaded = await api.uploadReferenceAsset(file)
    setRoleAsset(role, uploaded)
  } catch (error) {
    submitError.value = error.message || '参考图上传失败'
  } finally {
    uploadingRole.value = ''
  }
}

async function removeReference(role, image) {
  if (submitting.value) return
  setRoleAsset(role, null)
  if (image?.id && !image?._selectedFromLibrary) {
    await api.deleteReferenceAsset(image.id).catch(() => {})
  }
}

function normalizeCreditEstimate(payload = {}) {
  const source = payload?.error && !payload.required_credits ? { ...payload, ...payload.error } : payload
  const requiredCredits = Number(source.required_credits ?? 0)
  const availableCredits = Number(source.available_credits ?? 0)
  const missingCredits = Number(source.missing_credits ?? Math.max(requiredCredits - availableCredits, 0))
  return {
    required_credits: Number.isFinite(requiredCredits) ? requiredCredits : 0,
    available_credits: Number.isFinite(availableCredits) ? availableCredits : 0,
    missing_credits: Number.isFinite(missingCredits) ? missingCredits : 0,
    enough: source.enough === undefined ? missingCredits <= 0 : Boolean(source.enough),
    recommended_package: source.recommended_package || null
  }
}

function buildPayload() {
  return {
    title: title.value.trim(),
    location: selectedLocation.value,
    story_template: selectedTemplate.value,
    style: selectedStyle.value,
    male_reference_asset_id: maleAsset.value?.id,
    female_reference_asset_id: isChildhoodDreamMode.value ? (femaleAsset.value?.id || 0) : femaleAsset.value?.id
  }
}

function shortfallText(estimate) {
  const packageName = `${estimate?.recommended_package?.name || ''}`.trim()
  const suffix = packageName ? `，推荐套餐「${packageName}」` : ''
  return `点数不足，本次预计消耗 ${estimate.required_credits} 点，当前余额 ${estimate.available_credits} 点，还差 ${estimate.missing_credits} 点${suffix}`
}

function applyShortfall(payload) {
  const estimate = normalizeCreditEstimate(payload)
  creditShortfall.value = estimate
  submitError.value = shortfallText(estimate)
}

function goPricing() {
  const estimate = creditShortfall.value
  router.push({
    path: '/pricing',
    query: {
      source: 'couple_album',
      missing_credits: estimate?.missing_credits,
      required_credits: estimate?.required_credits,
      package_id: estimate?.recommended_package?.id
    }
  })
}

async function submitAlbum() {
  submitError.value = ''
  submitHint.value = ''
  creditShortfall.value = null
  if (!canSubmit.value) {
    submitError.value = modeText.value.submitMissing
    return
  }

  submitting.value = true
  const payload = buildPayload()
  try {
    const estimate = normalizeCreditEstimate(await api.estimateCoupleAlbum(payload))
    if (!estimate.enough) {
      applyShortfall(estimate)
      return
    }
    const created = await api.createCoupleAlbum(payload)
    const albumID = created?.album?.id
    if (!albumID) {
      throw new Error('相册创建失败')
    }
    await api.generateCoupleAlbum(albumID)
    submitHint.value = '相册已开始生成'
    router.push(`/workspace/couple-album/${albumID}`)
  } catch (error) {
    if (error?.code === 'credits_insufficient') {
      applyShortfall(error)
      return
    }
    submitError.value = error.message || '相册生成失败'
  } finally {
    submitting.value = false
  }
}

function statusText(status) {
  switch (status) {
    case 'generating':
      return '生成中'
    case 'succeeded':
      return '已完成'
    case 'failed':
      return '失败'
    case 'partial_failed':
      return '部分失败'
    default:
      return '草稿'
  }
}

function albumCover(album) {
  return (album?.pages || []).find((page) => page.preview_url)?.preview_url || ''
}

function completedChildhoodPageCount(album) {
  return (album?.pages || []).filter((page) => page?.status === 'succeeded' || page?.preview_url || page?.work_id).length
}

function recentAlbumLine(album) {
  if (isChildhoodDreamMode.value) {
    return `${completedChildhoodPageCount(album)}/8 · ${statusText(album.status)}`
  }
  return `${locationLabelMap.value.get(album?.location) || album?.location || '未选择地点'} · ${statusText(album.status)}`
}

function resetFormForCurrentMode() {
  title.value = modeText.value.defaultTitle
  maleAsset.value = null
  femaleAsset.value = null
  submitError.value = ''
  submitHint.value = ''
  creditShortfall.value = null
  applyOptions(optionPayload.value)
}

onMounted(() => {
  void load()
  if (typeof window !== 'undefined') {
    window.addEventListener('keydown', handleAssetPickerKeydown)
  }
})

onBeforeUnmount(() => {
  if (typeof window !== 'undefined') {
    window.removeEventListener('keydown', handleAssetPickerKeydown)
  }
})

watch(isChildhoodDreamMode, () => {
  resetFormForCurrentMode()
})

function handleAssetPickerKeydown(event) {
  if (event.key === 'Escape' && assetPickerOpen.value) {
    closeAssetPicker()
  }
}
</script>

<template>
  <div v-if="loading" class="workspace-loading">
    <p>{{ modeText.loading }}</p>
  </div>

  <div v-else-if="pageError" class="workspace-error">
    <p>{{ pageError }}</p>
  </div>

  <div
    v-else-if="isChildhoodDreamMode"
    class="childhood-dream-workspace"
    data-testid="childhood-dream-workspace"
  >
    <section class="childhood-dream-shell">
      <main class="childhood-dream-main">
        <section class="childhood-dream-hero">
          <div class="childhood-dream-hero-copy">
            <p class="childhood-dream-kicker">{{ modeText.kicker }}</p>
            <h1>创建梦想相册</h1>
            <p>{{ modeText.subtitle }}</p>
          </div>
          <img :src="rocketChildHero" alt="" aria-hidden="true">
        </section>

        <div class="childhood-dream-steps" aria-label="创建步骤">
          <div
            v-for="(step, index) in ['上传孩子照片', '选择梦想主题', '生成 8 页相册']"
            :key="step"
            class="childhood-dream-step"
            :data-testid="`childhood-dream-step-${index + 1}`"
          >
            <span>{{ index + 1 }}</span>
            <strong>{{ step }}</strong>
          </div>
        </div>

        <section class="childhood-dream-card childhood-dream-form">
          <label class="childhood-dream-field">
            <span>相册标题</span>
            <input
              v-model="title"
              class="text-input"
              data-testid="couple-album-title"
              type="text"
              maxlength="80"
              :disabled="submitting"
            />
          </label>

          <div class="childhood-dream-photo-grid">
            <div class="childhood-dream-photo-slot" data-testid="couple-album-male-upload">
              <span>{{ modeText.primaryPhoto }}</span>
              <ImageUploadZone
                :images="maleImages"
                :max-images="1"
                :uploading="uploadingRole === 'male'"
                :disabled="submitting"
                empty-hint-secondary="点击上传、拖拽图片或从素材库选择"
                library-action-label="从素材库选择"
                library-action-testid="couple-album-open-asset-picker-male"
                @upload="uploadReference('male', $event)"
                @remove="removeReference('male', $event)"
                @select-library="openAssetPicker('male')"
              />
            </div>
            <div class="childhood-dream-photo-slot" data-testid="couple-album-female-upload">
              <span>{{ modeText.secondaryPhoto }}</span>
              <ImageUploadZone
                :images="femaleImages"
                :max-images="1"
                :uploading="uploadingRole === 'female'"
                :disabled="submitting"
                empty-hint-secondary="点击上传、拖拽图片或从素材库选择"
                library-action-label="从素材库选择"
                library-action-testid="couple-album-open-asset-picker-female"
                @upload="uploadReference('female', $event)"
                @remove="removeReference('female', $event)"
                @select-library="openAssetPicker('female')"
              />
            </div>
          </div>

          <div class="childhood-dream-section">
            <div class="childhood-dream-section-title">
              <Rocket :size="18" />
              <span>{{ modeText.locationSection }}</span>
            </div>
            <div class="childhood-dream-theme-grid">
              <button
                v-for="item in locations"
                :key="item.value"
                class="childhood-dream-theme-card"
                :class="{ active: selectedLocation === item.value }"
                type="button"
                :disabled="submitting"
                :data-testid="`childhood-dream-theme-${item.value}`"
                @click="selectedLocation = item.value"
              >
                <img :src="item.image_url" :alt="item.label">
                <strong>{{ item.label }}</strong>
                <span>{{ item.description || item.value }}</span>
              </button>
            </div>
          </div>

          <div class="childhood-dream-section">
            <div class="childhood-dream-section-title">
              <Sparkles :size="18" />
              <span>{{ modeText.templateSection }}</span>
            </div>
            <div class="childhood-dream-content-pills">
              <button
                v-for="item in templates"
                :key="item.value"
                class="childhood-dream-content-pill active"
                type="button"
                :disabled="submitting"
                @click="selectedTemplate = item.value"
              >
                <strong>{{ item.label }}</strong>
                <span>{{ item.description || item.value }}</span>
              </button>
              <span v-for="pill in childhoodContentPills" :key="pill">{{ pill }}</span>
            </div>
          </div>

          <div class="childhood-dream-section">
            <div class="childhood-dream-section-title">
              <Palette :size="18" />
              <span>画面风格</span>
            </div>
            <div class="childhood-dream-style-grid">
              <button
                v-for="item in styles"
                :key="item.value"
                class="childhood-dream-style-card"
                :class="{ active: selectedStyle === item.value }"
                type="button"
                :disabled="submitting"
                :data-testid="`childhood-dream-style-${item.value}`"
                @click="selectedStyle = item.value"
              >
                <img :src="item.image_url" :alt="item.label">
                <strong>{{ item.label }}</strong>
                <span>{{ item.description || item.value }}</span>
              </button>
            </div>
          </div>

          <div class="childhood-dream-actions">
            <button
              class="primary-button"
              data-testid="couple-album-submit"
              type="button"
              :disabled="!canSubmit"
              @click="submitAlbum"
            >
              {{ submitting ? '提交中...' : '预估点数并生成' }}
            </button>
            <span>预计生成 8 页，点数会在提交前确认</span>
            <button
              v-if="creditShortfall"
              class="secondary-button"
              data-testid="couple-album-recharge"
              type="button"
              @click="goPricing"
            >
              去充值
            </button>
          </div>

          <p v-if="submitError" class="status-error couple-album-feedback" role="alert">{{ submitError }}</p>
          <p v-if="submitHint" class="status-success couple-album-feedback">{{ submitHint }}</p>
        </section>
      </main>

      <aside class="childhood-dream-side">
        <section class="childhood-dream-card childhood-dream-recent-panel">
          <div class="childhood-dream-side-title">
            <Heart :size="18" />
            <h2>{{ modeText.recentTitle }}</h2>
          </div>
          <p>{{ modeText.recentHint }}</p>

          <div v-if="displayRecentAlbums.length" class="childhood-dream-recent-list">
            <RouterLink
              v-for="album in displayRecentAlbums"
              :key="album.id"
              class="childhood-dream-recent-item"
              :data-testid="`recent-couple-album-${album.id}`"
              :to="`/workspace/couple-album/${album.id}`"
            >
              <img :src="albumCover(album) || recentFallbackImage" :alt="album.title">
              <div>
                <strong>{{ album.title }}</strong>
                <span>{{ recentAlbumLine(album) }}</span>
              </div>
            </RouterLink>
          </div>

          <div v-else class="couple-album-empty-state">
            <strong>{{ modeText.emptyTitle }}</strong>
            <span>{{ modeText.emptyHint }}</span>
          </div>
        </section>

        <section class="childhood-dream-card childhood-dream-why-panel">
          <img :src="sideRocketChildImage" alt="" aria-hidden="true">
          <div class="childhood-dream-side-title">
            <Sparkles :size="18" />
            <h2>为什么选择童年梦想相册</h2>
          </div>
          <div class="childhood-dream-why-list">
            <div v-for="item in childhoodWhyItems" :key="item.title">
              <component :is="item.icon" :size="18" />
              <div>
                <strong>{{ item.title }}</strong>
                <span>{{ item.text }}</span>
              </div>
            </div>
          </div>
        </section>
      </aside>
    </section>
  </div>

  <div v-else class="couple-album-workspace-page">
    <section class="couple-album-header">
      <div>
        <p class="couple-album-kicker">{{ modeText.kicker }}</p>
        <h1>{{ modeText.title }}</h1>
        <p>{{ modeText.subtitle }}</p>
      </div>
      <div class="couple-album-upload-count">
        <Heart :size="18" />
        <span>{{ uploadedCount }}/{{ requiredUploadCount }} 参考图</span>
      </div>
    </section>

    <div class="couple-album-workspace-grid">
      <SoftPanel class="couple-album-composer" tone="default" roomy>
        <div class="couple-album-section-head">
          <Camera :size="20" />
          <div>
            <h2>{{ modeText.composerTitle }}</h2>
            <p>{{ modeText.composerHint }}</p>
          </div>
        </div>

        <label class="couple-album-field">
          <span>相册标题</span>
          <input
            v-model="title"
            class="text-input"
            data-testid="couple-album-title"
            type="text"
            maxlength="80"
            :disabled="submitting"
          />
        </label>

        <div class="couple-album-photo-grid">
          <div class="couple-album-photo-slot" data-testid="couple-album-male-upload">
            <span>{{ modeText.primaryPhoto }}</span>
            <ImageUploadZone
              :images="maleImages"
              :max-images="1"
              :uploading="uploadingRole === 'male'"
              :disabled="submitting"
              empty-hint-secondary="点击上传、拖拽图片或从素材库选择"
              library-action-label="从素材库选择"
              library-action-testid="couple-album-open-asset-picker-male"
              @upload="uploadReference('male', $event)"
              @remove="removeReference('male', $event)"
              @select-library="openAssetPicker('male')"
            />
          </div>
          <div class="couple-album-photo-slot" data-testid="couple-album-female-upload">
            <span>{{ modeText.secondaryPhoto }}</span>
            <ImageUploadZone
              :images="femaleImages"
              :max-images="1"
              :uploading="uploadingRole === 'female'"
              :disabled="submitting"
              empty-hint-secondary="点击上传、拖拽图片或从素材库选择"
              library-action-label="从素材库选择"
              library-action-testid="couple-album-open-asset-picker-female"
              @upload="uploadReference('female', $event)"
              @remove="removeReference('female', $event)"
              @select-library="openAssetPicker('female')"
            />
          </div>
        </div>

        <div class="couple-album-option-section">
          <div class="couple-album-section-title">
            <MapPin :size="18" />
            <span>{{ modeText.locationSection }}</span>
          </div>
          <div class="couple-album-location-grid">
            <button
              v-for="item in locations"
              :key="item.value"
              class="couple-album-option-card"
              :class="{ active: selectedLocation === item.value }"
              type="button"
              :disabled="submitting"
              @click="selectedLocation = item.value"
            >
              <img v-if="item.image_url" :src="item.image_url" :alt="item.label">
              <strong>{{ item.label }}</strong>
              <span>{{ item.description || item.value }}</span>
            </button>
          </div>
        </div>

        <div class="couple-album-two-column">
          <div class="couple-album-option-section">
            <div class="couple-album-section-title">
              <Sparkles :size="18" />
              <span>{{ modeText.templateSection }}</span>
            </div>
            <div class="couple-album-chip-grid">
              <button
                v-for="item in templates"
                :key="item.value"
                class="couple-album-chip"
                :class="{ active: selectedTemplate === item.value }"
                type="button"
                :disabled="submitting"
                @click="selectedTemplate = item.value"
              >
                <strong>{{ item.label }}</strong>
                <span>{{ item.description || item.value }}</span>
              </button>
            </div>
          </div>

          <div class="couple-album-option-section">
            <div class="couple-album-section-title">
              <Sparkles :size="18" />
              <span>画面风格</span>
            </div>
            <div class="couple-album-chip-grid">
              <button
                v-for="item in styles"
                :key="item.value"
                class="couple-album-chip"
                :class="{ active: selectedStyle === item.value }"
                type="button"
                :disabled="submitting"
                @click="selectedStyle = item.value"
              >
                <strong>{{ item.label }}</strong>
              </button>
            </div>
          </div>
        </div>

        <div class="couple-album-actions">
          <button
            class="primary-button"
            data-testid="couple-album-submit"
            type="button"
            :disabled="!canSubmit"
            @click="submitAlbum"
          >
            {{ submitting ? '提交中...' : '预估点数并生成' }}
          </button>
          <button
            v-if="creditShortfall"
            class="secondary-button"
            data-testid="couple-album-recharge"
            type="button"
            @click="goPricing"
          >
            去充值
          </button>
        </div>

        <p v-if="submitError" class="status-error couple-album-feedback" role="alert">{{ submitError }}</p>
        <p v-if="submitHint" class="status-success couple-album-feedback">{{ submitHint }}</p>
      </SoftPanel>

      <aside class="couple-album-side">
        <SoftPanel class="couple-album-recent-panel" tone="highlight" roomy>
          <div class="couple-album-section-head">
            <Heart :size="20" />
            <div>
              <h2>{{ modeText.recentTitle }}</h2>
              <p>{{ modeText.recentHint }}</p>
            </div>
          </div>

          <div v-if="displayRecentAlbums.length" class="couple-album-recent-list">
            <RouterLink
              v-for="album in displayRecentAlbums"
              :key="album.id"
              class="couple-album-recent-item"
              :data-testid="`recent-couple-album-${album.id}`"
              :to="`/workspace/couple-album/${album.id}`"
            >
              <img v-if="albumCover(album)" :src="albumCover(album)" :alt="album.title">
              <span v-else class="couple-album-recent-placeholder">8P</span>
              <div>
                <strong>{{ album.title }}</strong>
                <span>{{ recentAlbumLine(album) }}</span>
              </div>
            </RouterLink>
          </div>

          <div v-else class="couple-album-empty-state">
            <strong>{{ modeText.emptyTitle }}</strong>
            <span>{{ modeText.emptyHint }}</span>
          </div>
        </SoftPanel>
      </aside>
    </div>
  </div>

  <div
    v-if="assetPickerOpen"
    class="couple-album-asset-picker-backdrop"
    data-testid="couple-album-asset-picker"
    role="dialog"
    aria-modal="true"
    aria-labelledby="coupleAlbumAssetPickerTitle"
    @click.self="closeAssetPicker"
  >
    <section class="couple-album-asset-picker-panel">
      <header class="couple-album-asset-picker-head">
        <div>
          <p class="eyebrow">Assets</p>
          <h2 id="coupleAlbumAssetPickerTitle">从素材库选择</h2>
          <span>选择 1 张已有图片作为{{ assetPickerRole === 'male' ? modeText.primaryPhoto : modeText.secondaryPhoto }}。</span>
        </div>
        <button
          class="mini-button icon-only"
          data-testid="couple-album-asset-picker-close"
          type="button"
          aria-label="关闭素材选择"
          @click="closeAssetPicker"
        >
          <X :size="16" />
        </button>
      </header>

      <p v-if="assetPickerLoading" class="page-status">素材读取中...</p>
      <p v-else-if="assetPickerError" class="status-error" role="alert">{{ assetPickerError }}</p>
      <div v-else-if="assetPickerAssets.length" class="couple-album-asset-picker-grid">
        <button
          v-for="asset in assetPickerAssets"
          :key="asset.id"
          class="couple-album-asset-option"
          :class="{ active: `${selectedAssetPickerId}` === `${asset.id}` }"
          :data-testid="`couple-album-asset-option-${asset.id}`"
          type="button"
          @click="selectAssetPickerItem(asset)"
        >
          <img v-if="asset.preview_url" :src="asset.preview_url" :alt="asset.original_filename || '素材图片'">
          <span v-else class="couple-album-asset-placeholder">
            <ImagePlus :size="22" />
          </span>
          <strong>{{ asset.original_filename || `素材 ${asset.id}` }}</strong>
        </button>
      </div>
      <div v-else class="couple-album-asset-empty">
        <ImagePlus :size="26" />
        <strong>素材库暂无图片</strong>
        <span>可以先使用本地上传，或前往资产页上传素材。</span>
      </div>

      <footer class="couple-album-asset-picker-actions">
        <button class="secondary-button" type="button" @click="closeAssetPicker">取消</button>
        <button
          class="primary-button"
          data-testid="couple-album-asset-picker-confirm"
          type="button"
          :disabled="!selectedAssetPickerItem"
          @click="confirmAssetPicker"
        >
          确认选择
        </button>
      </footer>
    </section>
  </div>
</template>
