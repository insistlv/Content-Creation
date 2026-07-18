<script setup>
import { computed, ref } from 'vue'
import { onLoad } from '@dcloudio/uni-app'

import { api } from '../../../api/client.js'
import AppTabbar from '../../../components/AppTabbar.vue'
import { navigateTo, requireAuth, routes } from '../../../utils/routes.js'

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

const couplePhotoRoles = [
  { key: 'male', title: '男方照片', hint: '上传第一张', tag: '第一位主角' },
  { key: 'female', title: '女方照片', hint: '上传第二张', tag: '第二位主角' }
]

const childhoodPhotoRoles = [
  { key: 'male', title: '孩子照片', hint: '清晰正脸或半身照', tag: '唯一主角' },
  { key: 'female', title: '补充全身照', hint: '可选，服装体态参考', tag: '可选参考' }
]

const defaultLocationCards = [
  {
    value: '大理',
    label: '大理洱海',
    desc: '风吹洱海的蓝色午后',
    image: staticAsset('couple-album/dali-erhai.png')
  },
  {
    value: '京都',
    label: '京都樱花',
    desc: '樱花雨里的慢镜头',
    image: staticAsset('couple-album/kyoto-sakura.png')
  },
  {
    value: '巴黎',
    label: '巴黎街角',
    desc: '转角遇见电影感',
    image: staticAsset('couple-album/paris-corner.png')
  },
  {
    value: '厦门',
    label: '厦门海岸',
    desc: '海风、落日和并肩',
    image: staticAsset('couple-album/xiamen-coast.png')
  },
  {
    value: '上海',
    label: '上海夜景',
    desc: '灯光把故事点亮',
    image: staticAsset('couple-album/shanghai-night.png')
  }
]

const defaultChildhoodLocationCards = [
  {
    value: 'childhood_dream_stage',
    label: '童年梦想舞台',
    desc: '一次生成 8 页职业梦想故事',
    image: staticAsset('home-replica/couple-album-book.png')
  }
]

const defaultStoryTemplates = [
  { value: 'city_walk', label: '城市漫游', desc: '街角、咖啡和夜色', icon: icon('works') },
  { value: 'first_trip', label: '初次旅行', desc: '出发那天的心动', icon: icon('image') },
  { value: 'anniversary', label: '纪念日', desc: '520 与每个重要日子', icon: icon('favorite') },
  { value: 'proposal', label: '求婚时刻', desc: '黄昏、灯光和仪式感', icon: icon('generate') }
]

const defaultChildhoodStoryTemplates = [
  { value: 'childhood_career_dream', label: '童年职业梦想', desc: '宇航员、医生、画家等 8 页连续故事', icon: icon('logo-star') }
]

const defaultStyles = [
  { value: 'film', label: '旅行胶片', icon: icon('photo') },
  { value: 'cinematic', label: '电影旅拍', icon: icon('image-image') },
  { value: 'watercolor', label: '清透水彩', icon: icon('illustration') },
  { value: 'storybook', label: '绘本相册', icon: icon('prompt') }
]

const defaultChildhoodStyles = [
  { value: 'children_storybook', label: '童话绘本', icon: icon('illustration') },
  { value: 'dreamy_watercolor', label: '梦幻水彩', icon: icon('guofeng') },
  { value: 'animation_3d', label: '3D 动画电影', icon: icon('image-image') },
  { value: 'children_photo_poster', label: '儿童写真海报', icon: icon('photo') }
]

const dreamRoleCards = [
  { title: '小小宇航员', desc: '月球基地与蓝色地球' },
  { title: '小小医生', desc: '温暖诊室照顾玩偶' },
  { title: '小小画家', desc: '阳光画室画出彩虹' },
  { title: '小小科学家', desc: '安全实验桌探索星球' },
  { title: '小小厨师', desc: '制作六一节日蛋糕' },
  { title: '小小运动员', desc: '阳光操场自信领奖' },
  { title: '梦想纪念照', desc: '职业元素汇聚成海报' },
  { title: '封面舞台', desc: '火箭、画笔、书本与星星' }
]

const allLocationCards = ref([...defaultLocationCards, ...defaultChildhoodLocationCards])
const allStoryTemplates = ref([...defaultStoryTemplates, ...defaultChildhoodStoryTemplates])
const allStyles = ref([...defaultStyles, ...defaultChildhoodStyles])
const title = ref('我们的 520 旅行相册')
const albumMode = ref('couple')
const selectedLocationIndex = ref(0)
const selectedTemplateIndex = ref(0)
const selectedStyleIndex = ref(0)
const malePhoto = ref(null)
const femalePhoto = ref(null)
const submitting = ref(false)
const errorMessage = ref('')
const creditShortfall = ref(null)
const showModeSheet = ref(false)

const legacyLocationValues = new Set(defaultLocationCards.map((item) => item.value))
const legacyStoryTemplateValues = new Set(defaultStoryTemplates.map((item) => item.value))
const legacyStyleValues = new Set(defaultStyles.map((item) => item.value))
const childhoodLocationValues = new Set(defaultChildhoodLocationCards.map((item) => item.value))
const childhoodStoryTemplateValues = new Set(defaultChildhoodStoryTemplates.map((item) => item.value))
const childhoodStyleValues = new Set(defaultChildhoodStyles.map((item) => item.value))

const isChildhoodDreamMode = computed(() => albumMode.value === 'childhood-dream')
const locationCards = computed(() => filteredOptions(allLocationCards.value, isChildhoodDreamMode.value ? childhoodLocationValues : legacyLocationValues, isChildhoodDreamMode.value ? defaultChildhoodLocationCards : defaultLocationCards))
const storyTemplates = computed(() => filteredOptions(allStoryTemplates.value, isChildhoodDreamMode.value ? childhoodStoryTemplateValues : legacyStoryTemplateValues, isChildhoodDreamMode.value ? defaultChildhoodStoryTemplates : defaultStoryTemplates))
const styles = computed(() => filteredOptions(allStyles.value, isChildhoodDreamMode.value ? childhoodStyleValues : legacyStyleValues, isChildhoodDreamMode.value ? defaultChildhoodStyles : defaultStyles))
const selectedLocation = computed(() => locationCards.value[selectedLocationIndex.value] || locationCards.value[0] || (isChildhoodDreamMode.value ? defaultChildhoodLocationCards[0] : defaultLocationCards[0]))
const selectedTemplate = computed(() => storyTemplates.value[selectedTemplateIndex.value] || storyTemplates.value[0] || (isChildhoodDreamMode.value ? defaultChildhoodStoryTemplates[0] : defaultStoryTemplates[0]))
const selectedStyle = computed(() => styles.value[selectedStyleIndex.value] || styles.value[0] || (isChildhoodDreamMode.value ? defaultChildhoodStyles[0] : defaultStyles[0]))
const uploadedPhotoCount = computed(() => [malePhoto.value, femalePhoto.value].filter(Boolean).length)
const requiredPhotoTotal = computed(() => isChildhoodDreamMode.value ? 1 : 2)
const photoRoles = computed(() => isChildhoodDreamMode.value ? childhoodPhotoRoles : couplePhotoRoles)
const missingRequiredPhoto = computed(() => {
  if (!malePhoto.value?.serverId) return true
  if (!isChildhoodDreamMode.value && !femalePhoto.value?.serverId) return true
  return false
})
const photoUploading = computed(() => Boolean(malePhoto.value?.uploading || femalePhoto.value?.uploading))
const canSubmit = computed(() => Boolean(!missingRequiredPhoto.value && title.value.trim() && !submitting.value))
const currentModeLabel = computed(() => isChildhoodDreamMode.value ? '童年梦想相册' : '情侣相册')
const pageTitle = computed(() => isChildhoodDreamMode.value ? '童年职业梦想相册' : '520 情侣旅行相册')
const pageSubtitle = computed(() => isChildhoodDreamMode.value ? '上传孩子照片，选择梦想主题和画面风格，生成 8 页六一职业梦想故事。' : '上传双人参考照，选择地点、故事和画面风格，生成可分享的 AI 相册。')
const uploadTitle = computed(() => isChildhoodDreamMode.value ? '上传孩子照片' : '上传情侣照片')
const uploadHint = computed(() => isChildhoodDreamMode.value ? '建议使用 1 张清晰正脸或半身照，可补充 1 张全身照作为服装体态参考。' : '建议使用正面清晰合照或半身照，生成效果会更自然。')
const locationSectionTitle = computed(() => isChildhoodDreamMode.value ? '选择梦想主题' : '选择旅游地点')
const templateSectionTitle = computed(() => isChildhoodDreamMode.value ? '相册内容' : '故事模板')
const privacyText = computed(() => isChildhoodDreamMode.value ? '孩子照片仅用于生成梦想相册，结果默认保存至私有作品库。' : '照片仅用于生成相册，结果默认保存至私有作品库。')
const missingPhotoToastText = computed(() => isChildhoodDreamMode.value ? '请先上传孩子照片' : '请先上传双方照片')
const disabledSubmitText = computed(() => isChildhoodDreamMode.value ? '请先上传孩子照片' : '请先上传两张照片')

onLoad((options = {}) => {
  setAlbumMode(options.mode === 'childhood-dream' ? 'childhood-dream' : 'couple')
  void initializePage()
})

async function initializePage() {
  const me = await requireAuth()
  if (me) {
    await loadCoupleAlbumOptions()
  }
}

function setAlbumMode(mode) {
  albumMode.value = mode === 'childhood-dream' ? 'childhood-dream' : 'couple'
  selectedLocationIndex.value = 0
  selectedTemplateIndex.value = 0
  selectedStyleIndex.value = 0
  malePhoto.value = null
  femalePhoto.value = null
  errorMessage.value = ''
  creditShortfall.value = null
  title.value = isChildhoodDreamMode.value ? '我的六一梦想相册' : '我们的 520 旅行相册'
}

function showToast(titleText) {
  uni.showToast({ title: titleText, icon: 'none' })
}

function normalizeCreditEstimatePayload(payload = {}) {
  const source = payload?.error && !payload.required_credits ? { ...payload, ...payload.error } : payload
  const requiredCredits = Number(source?.required_credits ?? 0)
  const availableCredits = Number(source?.available_credits ?? 0)
  const missingCredits = Number(source?.missing_credits ?? Math.max(requiredCredits - availableCredits, 0))
  return {
    required_credits: Number.isFinite(requiredCredits) ? requiredCredits : 0,
    available_credits: Number.isFinite(availableCredits) ? availableCredits : 0,
    missing_credits: Number.isFinite(missingCredits) ? missingCredits : 0,
    enough: source?.enough === undefined ? missingCredits <= 0 : Boolean(source.enough),
    recommended_package: source?.recommended_package || null
  }
}

function insufficientCreditsDetailText(estimate) {
  const packageName = `${estimate?.recommended_package?.name || ''}`.trim()
  const suffix = packageName ? `，推荐套餐「${packageName}」` : ''
  return `点数不足，本次预计消耗 ${estimate.required_credits} 点，当前余额 ${estimate.available_credits} 点，还差 ${estimate.missing_credits} 点${suffix}`
}

function applyInsufficientCreditsEstimate(payload) {
  const estimate = normalizeCreditEstimatePayload(payload)
  creditShortfall.value = estimate
  errorMessage.value = insufficientCreditsDetailText(estimate)
  showToast('点数不足，请先充值')
}

function fileNameFromPath(path, fallback) {
  if (!path) return fallback
  return decodeURIComponent(`${path}`.split('/').pop() || fallback)
}

function normalizeImageSource(value) {
  const source = `${value || ''}`.trim()
  if (!source) return ''
  if (/^(https?:|wxfile:|cloud:|blob:|data:image\/)/i.test(source)) return source
  if (/^\/(api|static|tmp|usr|store_|wxfile)/i.test(source)) return source
  return ''
}

function optionText(value) {
  return `${value || ''}`.trim()
}

function optionAssetURL(value, fallback) {
  const source = optionText(value)
  if (!source) return fallback
  if (/^(https?:|wxfile:|cloud:|blob:|data:image\/)/i.test(source)) return source
  if (/^\/static\//i.test(source)) return staticAsset(source.replace(/^\/static\/+/i, ''))
  if (/^static\//i.test(source)) return staticAsset(source.replace(/^static\/+/i, ''))
  if (/^\//.test(source)) return api.assetURL(source)
  return staticAsset(source)
}

function defaultOptionFor(defaults, value, index) {
  return defaults.find((item) => item.value === value) || defaults[index] || defaults[0] || {}
}

function normalizeBackendOptions(list, defaults, mapper) {
  const source = Array.isArray(list) ? list : []
  const normalized = source
    .map((item, index) => {
      const value = optionText(item?.value)
      const label = optionText(item?.label)
      if (!value || !label) return null
      const fallback = defaultOptionFor(defaults, value, index)
      return mapper(item, fallback, value, label)
    })
    .filter(Boolean)
  return normalized.length > 0 ? normalized : defaults
}

function filteredOptions(list, allowedValues, defaults) {
  const filtered = (Array.isArray(list) ? list : []).filter((item) => allowedValues.has(item.value))
  return filtered.length > 0 ? filtered : defaults
}

function indexForValue(list, value) {
  const index = list.findIndex((item) => item.value === value)
  return index >= 0 ? index : 0
}

function applyCoupleAlbumOptions(payload = {}) {
  const nextLocations = normalizeBackendOptions(payload.locations, [...defaultLocationCards, ...defaultChildhoodLocationCards], (item, fallback, value, label) => ({
    value,
    label,
    desc: optionText(item.description) || fallback.desc || label,
    image: optionAssetURL(item.image_url, fallback.image)
  }))
  const nextTemplates = normalizeBackendOptions(payload.story_templates, [...defaultStoryTemplates, ...defaultChildhoodStoryTemplates], (item, fallback, value, label) => ({
    value,
    label,
    desc: optionText(item.description) || fallback.desc || label,
    icon: optionAssetURL(item.icon_url, fallback.icon)
  }))
  const nextStyles = normalizeBackendOptions(payload.styles, [...defaultStyles, ...defaultChildhoodStyles], (item, fallback, value, label) => ({
    value,
    label,
    icon: optionAssetURL(item.icon_url, fallback.icon)
  }))

  allLocationCards.value = nextLocations
  allStoryTemplates.value = nextTemplates
  allStyles.value = nextStyles
  selectedLocationIndex.value = indexForValue(locationCards.value, selectedLocation.value?.value)
  selectedTemplateIndex.value = indexForValue(storyTemplates.value, selectedTemplate.value?.value)
  selectedStyleIndex.value = indexForValue(styles.value, selectedStyle.value?.value)
}

async function loadCoupleAlbumOptions() {
  try {
    const payload = await api.getCoupleAlbumOptions()
    applyCoupleAlbumOptions(payload)
  } catch {
    applyCoupleAlbumOptions({})
  }
}

function photoFor(role) {
  return role === 'male' ? malePhoto.value : femalePhoto.value
}

function setPhoto(role, value) {
  if (role === 'male') {
    malePhoto.value = value
    return
  }
  femalePhoto.value = value
}

function choosePhoto(role) {
  if (submitting.value) return
  uni.chooseImage({
    count: 1,
    sizeType: ['compressed', 'original'],
    sourceType: ['album', 'camera'],
    success(result) {
      const path = result.tempFilePaths?.[0] || ''
      const file = result.tempFiles?.[0] || {}
      if (!path) return
      const next = {
        path,
        file,
        displayPreviewUrl: path,
        name: file.name || fileNameFromPath(path, role === 'male' ? '男方照片.png' : '女方照片.png'),
        uploading: true,
        serverId: 0,
        error: ''
      }
      setPhoto(role, next)
      void uploadPhoto(role, next)
    }
  })
}

async function uploadPhoto(role, item) {
  try {
    const uploaded = await api.uploadReferenceAsset({
      path: item.path,
      file: item.file,
      name: item.name
    })
    if (!uploaded?.id) {
      throw new Error('上传响应无效')
    }
    setPhoto(role, {
      ...item,
      uploading: false,
      serverId: uploaded.id,
      displayPreviewUrl: normalizeImageSource(uploaded.preview_url) || item.path,
      name: uploaded.original_filename || item.name,
      error: ''
    })
  } catch (error) {
    const message = error.message || '照片上传失败'
    setPhoto(role, { ...item, uploading: false, error: message })
    showToast(message)
  }
}

function removePhoto(role) {
  if (submitting.value) return
  setPhoto(role, null)
}

function selectLocation(index) {
  if (submitting.value) return
  selectedLocationIndex.value = Number(index) || 0
}

function selectTemplate(index) {
  if (submitting.value) return
  selectedTemplateIndex.value = index
}

function selectStyle(index) {
  if (submitting.value) return
  selectedStyleIndex.value = index
}

function goPricing() {
  const estimate = creditShortfall.value
  navigateTo(routes.pricing, {
    missing_credits: estimate?.missing_credits,
    required_credits: estimate?.required_credits,
    package_id: estimate?.recommended_package?.id,
    source: 'couple_album'
  })
}

function goHome() {
  navigateTo(routes.home)
}

function chooseTextMode() {
  showModeSheet.value = false
  navigateTo(routes.imageToImage, { mode: 'text' })
}

function chooseImageMode() {
  showModeSheet.value = false
  navigateTo(routes.imageToImage, { mode: 'image' })
}

function chooseCoupleAlbumMode() {
  showModeSheet.value = false
  setAlbumMode('couple')
}

function chooseChildhoodDreamMode() {
  showModeSheet.value = false
  setAlbumMode('childhood-dream')
}

async function submitAlbum() {
  errorMessage.value = ''
  creditShortfall.value = null
  if (submitting.value) return
  if (missingRequiredPhoto.value) {
    showToast(missingPhotoToastText.value)
    return
  }
  if (photoUploading.value) {
    showToast('照片正在上传，请稍候')
    return
  }
  if (!title.value.trim()) {
    showToast('请填写相册标题')
    return
  }
  const me = await requireAuth()
  if (!me) return

  submitting.value = true
  try {
    const requestPayload = {
      title: title.value.trim(),
      location: selectedLocation.value.value,
      story_template: selectedTemplate.value.value,
      style: selectedStyle.value.value,
      male_reference_asset_id: malePhoto.value.serverId,
      female_reference_asset_id: femalePhoto.value?.serverId || 0
    }
    const estimate = normalizeCreditEstimatePayload(await api.estimateCoupleAlbum(requestPayload))
    if (!estimate.enough) {
      applyInsufficientCreditsEstimate(estimate)
      return
    }
    const created = await api.createCoupleAlbum(requestPayload)
    const albumID = created?.album?.id
    if (!albumID) {
      throw new Error('相册创建失败')
    }
    await api.generateCoupleAlbum(albumID)
    navigateTo(routes.coupleAlbumDetail, { id: albumID })
  } catch (error) {
    if (error?.code === 'credits_insufficient') {
      applyInsufficientCreditsEstimate(error)
      return
    }
    const message = error.message || '相册生成失败'
    errorMessage.value = message
    showToast(message)
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <view class="album-create-page">
    <view class="app-shell">
      <view class="topbar">
        <button type="button" class="topbar-icon" @click="goHome">‹</button>
        <view class="brand">
          <image class="brand-icon" :src="icon('logo-star')" mode="aspectFit" />
          <view>
            <text class="brand-name">白霖共享</text>
            <text class="brand-subtitle">创作者 AI 图片平台</text>
          </view>
        </view>
        <button type="button" class="credits-button" @click="goPricing">点数</button>
      </view>

      <view class="mode-tabs">
        <view class="mode-label">
          <text>工作模式</text>
        </view>
        <button class="mode-current active" type="button" @click="showModeSheet = true">
          <text>{{ currentModeLabel }}</text>
          <text class="chevron">⌄</text>
        </button>
      </view>

      <view class="page-intro">
        <text class="page-title">{{ pageTitle }}</text>
        <text class="page-subtitle">{{ pageSubtitle }}</text>
      </view>

      <view class="creator-card upload-card">
        <view class="section-head">
          <view>
            <text class="section-title">{{ uploadTitle }}</text>
            <text class="section-hint">{{ uploadHint }}</text>
          </view>
          <text class="counter">{{ uploadedPhotoCount }}/{{ requiredPhotoTotal }}</text>
        </view>
        <view class="photo-section">
          <view
            v-for="role in photoRoles"
            :key="role.key"
            class="photo-slot"
            :class="{ filled: photoFor(role.key)?.displayPreviewUrl, uploading: photoFor(role.key)?.uploading }"
            @click="choosePhoto(role.key)"
          >
            <template v-if="photoFor(role.key)?.displayPreviewUrl">
              <image class="photo-preview" :src="photoFor(role.key).displayPreviewUrl" mode="aspectFill" />
              <view class="photo-replace-badge">更换</view>
              <view v-if="photoFor(role.key)?.uploading" class="photo-mask">上传中</view>
              <view v-if="photoFor(role.key)?.error" class="photo-mask error">上传失败</view>
              <button type="button" class="remove-photo" @click.stop="removePhoto(role.key)">×</button>
            </template>
            <template v-else>
              <view class="photo-placeholder">
                <image class="slot-icon" :src="icon('add-image')" mode="aspectFit" />
                <text>{{ role.title }}</text>
                <text>{{ role.hint }}</text>
              </view>
            </template>
            <view class="slot-tag">{{ role.tag }}</view>
          </view>
        </view>
      </view>

      <view class="creator-card title-card">
        <view class="section-head compact">
          <text class="section-title">相册标题</text>
          <text class="counter">{{ title.length }}/40</text>
        </view>
        <view class="prompt-panel title-input-shell">
          <input v-model="title" maxlength="40" placeholder="给这本相册起个名字" :disabled="submitting" />
        </view>
      </view>

      <view class="creator-card location-section">
        <view class="section-head compact">
          <text class="section-title">{{ locationSectionTitle }}</text>
        </view>
        <scroll-view class="location-scroll" scroll-x :show-scrollbar="false">
          <view class="location-track">
            <button
              v-for="(item, index) in locationCards"
              :key="item.value"
              type="button"
              class="location-card"
              :class="{ active: selectedLocationIndex === index }"
              @click="selectLocation(index)"
            >
              <image :src="item.image" mode="aspectFill" />
              <view class="location-shade"></view>
              <view class="location-copy">
                <text>{{ item.label }}</text>
                <text>{{ item.desc }}</text>
              </view>
              <view class="location-check">✓</view>
            </button>
          </view>
        </scroll-view>
      </view>

      <view class="creator-card template-section">
        <view class="section-head compact">
          <text class="section-title">{{ templateSectionTitle }}</text>
        </view>
        <view class="template-grid">
          <button
            v-for="(item, index) in storyTemplates"
            :key="item.value"
            type="button"
            class="template-card"
            :class="{ active: selectedTemplateIndex === index }"
            @click="selectTemplate(index)"
          >
            <image :src="item.icon" mode="aspectFit" />
            <text>{{ item.label }}</text>
            <text>{{ item.desc }}</text>
          </button>
        </view>
        <view v-if="isChildhoodDreamMode" class="dream-role-grid">
          <view v-for="item in dreamRoleCards" :key="item.title" class="dream-role-card">
            <text>{{ item.title }}</text>
            <text>{{ item.desc }}</text>
          </view>
        </view>
      </view>

      <view class="creator-card style-section">
        <view class="section-head compact">
          <text class="section-title">画面风格</text>
        </view>
        <view class="style-grid">
          <button
            v-for="(item, index) in styles"
            :key="item.value"
            type="button"
            class="style-card"
            :class="{ active: selectedStyleIndex === index }"
            @click="selectStyle(index)"
          >
            <image :src="item.icon" mode="aspectFit" />
            <text>{{ item.label }}</text>
          </button>
        </view>
      </view>

      <view class="privacy-note">
        <text>{{ privacyText }}</text>
      </view>

      <view v-if="errorMessage" class="error-strip">
        <view>
          <text>{{ errorMessage }}</text>
          <view v-if="creditShortfall" class="credit-shortfall-detail">
            <text>预计消耗 {{ creditShortfall.required_credits }} 点</text>
            <text>当前余额 {{ creditShortfall.available_credits }} 点</text>
            <text>还差 {{ creditShortfall.missing_credits }} 点</text>
            <text v-if="creditShortfall.recommended_package">推荐套餐 {{ creditShortfall.recommended_package.name }}</text>
          </view>
        </view>
        <button v-if="creditShortfall" type="button" class="shortfall-pricing-button" @click="goPricing">去充值</button>
      </view>
    </view>

    <view class="floating-generate-bar">
      <button type="button" class="generate-album-button" :class="{ disabled: !canSubmit }" @click="submitAlbum">
        {{ submitting ? '生成相册中...' : canSubmit ? '开始生成相册' : disabledSubmitText }}
      </button>
    </view>
    <AppTabbar active-key="workspace" extra-space="126rpx" />

    <view v-if="showModeSheet" class="modal-backdrop" @click="showModeSheet = false">
      <view class="mode-modal" @click.stop>
        <view class="drag-handle"></view>
        <text class="modal-title">选择工作模式</text>
        <text class="modal-subtitle">选择适合你的创作方式</text>

        <button type="button" class="mode-card" @click="chooseTextMode">
          <image :src="icon('text-image')" mode="aspectFit" />
          <view>
            <text>文生图</text>
            <text>通过文字描述生成全新图片</text>
            <text>适合创意构思、概念设计、海报和电商图</text>
          </view>
          <text class="arrow">›</text>
        </button>

        <button type="button" class="mode-card" @click="chooseImageMode">
          <image :src="icon('image-image')" mode="aspectFit" />
          <view>
            <text>图生图</text>
            <text>基于参考图进行风格转换或内容重绘</text>
            <text>适合风格迁移、细节调整、延展创作</text>
          </view>
          <text class="arrow">›</text>
        </button>

        <button type="button" class="mode-card mode-card-link selected" @click="chooseCoupleAlbumMode">
          <image :src="icon('favorite')" mode="aspectFit" />
          <view>
            <text>情侣相册</text>
            <text>上传双人照片，生成可分享的旅行相册</text>
            <text>适合纪念日、旅拍记录和 520 分享卡片</text>
          </view>
          <text :class="isChildhoodDreamMode ? 'arrow' : 'selected-dot'">{{ isChildhoodDreamMode ? '›' : '✓' }}</text>
        </button>

        <button type="button" class="mode-card mode-card-link" :class="{ selected: isChildhoodDreamMode }" @click="chooseChildhoodDreamMode">
          <image :src="icon('logo-star')" mode="aspectFit" />
          <view>
            <text>童年梦想相册</text>
            <text>上传孩子照片，生成六一职业梦想相册</text>
            <text>8 页连续故事，适合儿童节分享保存</text>
          </view>
          <text :class="isChildhoodDreamMode ? 'selected-dot' : 'arrow'">{{ isChildhoodDreamMode ? '✓' : '›' }}</text>
        </button>

        <view class="mode-tips">
          <text>{{ isChildhoodDreamMode ? '童年梦想相册功能亮点' : '情侣相册功能亮点' }}</text>
          <text>{{ isChildhoodDreamMode ? '单张孩子照片即可生成连续故事页' : '双人参考照生成连续相册页面' }}</text>
          <text>{{ isChildhoodDreamMode ? '封面、职业内页和梦想纪念照共 8 页' : '地点、故事模板和画面风格可组合选择' }}</text>
          <text>生成后可进入详情页预览与分享</text>
        </view>
      </view>
    </view>
  </view>
</template>

<style lang="scss" scoped>
@use '../../../styles/tokens.scss' as *;

.album-create-page {
  min-height: 100vh;
  background:
    radial-gradient(circle at 2% 0, rgba(255, 219, 233, 0.9), transparent 33%),
    radial-gradient(circle at 100% 0, rgba(220, 238, 255, 0.92), transparent 35%),
    linear-gradient(180deg, #fff9fd 0%, #f8fbff 52%, #eef6ff 100%);
  color: #111827;
}

.album-create-page button {
  margin: 0;
  padding: 0;
  border: 0;
  line-height: 1.2;
  overflow: visible;
}

.album-create-page button::after {
  border: 0;
}

.app-shell {
  min-height: 100vh;
  padding: calc(26rpx + $phone-float-clearance-top + env(safe-area-inset-top)) 28rpx 0;
}

.topbar {
  display: grid;
  grid-template-columns: 64rpx minmax(0, 1fr) 76rpx;
  align-items: center;
  gap: 14rpx;
  min-height: 64rpx;
}

.topbar-icon,
.credits-button {
  display: grid;
  place-items: center;
  height: 58rpx;
  border-radius: 999rpx;
  background: rgba(255, 255, 255, 0.92);
  color: #34405b;
  font-weight: 900;
  box-shadow: 0 10rpx 24rpx rgba(41, 57, 94, 0.12);
}

.topbar-icon {
  width: 58rpx;
  font-size: 40rpx;
  line-height: 1;
}

.credits-button {
  width: 76rpx;
  color: #245cff;
  font-size: 23rpx;
}

.brand,
.mode-tabs,
.mode-label,
.mode-current,
.mode-card,
.mode-tips text,
.section-head,
.section-head > view {
  display: flex;
  align-items: center;
}

.brand {
  justify-content: center;
  gap: 14rpx;
  min-width: 0;
}

.brand-icon {
  flex: 0 0 auto;
  width: 54rpx;
  height: 54rpx;
}

.brand-name,
.brand-subtitle,
.page-title,
.page-subtitle,
.section-title,
.section-hint,
.counter,
.modal-title,
.modal-subtitle,
.mode-card text,
.mode-tips text,
.photo-placeholder text,
.template-card text,
.style-card text,
.location-copy text,
.privacy-note text {
  display: block;
}

.brand-name {
  overflow: hidden;
  color: #10182d;
  font-size: 30rpx;
  font-weight: 950;
  line-height: 1.08;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.brand-subtitle {
  margin-top: 6rpx;
  overflow: hidden;
  color: #6f7890;
  font-size: 22rpx;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mode-tabs {
  gap: 8rpx;
  height: 86rpx;
  margin-top: 36rpx;
  padding: 6rpx;
  border: 1rpx solid rgba(140, 151, 177, 0.16);
  border-radius: 18rpx;
  background: rgba(255, 255, 255, 0.82);
  box-shadow: 0 10rpx 26rpx rgba(36, 47, 82, 0.06);
}

.mode-label,
.mode-current {
  flex: 1;
  justify-content: center;
  min-width: 0;
  height: 74rpx;
  border-radius: 15rpx;
  white-space: nowrap;
}

.mode-label {
  background: rgba(244, 247, 252, 0.74);
  color: #6f7d96;
  font-size: 25rpx;
  font-weight: 900;
  box-shadow: inset 0 0 0 1rpx rgba(142, 153, 177, 0.08);
}

.mode-current {
  flex: 1.12;
  gap: 10rpx;
  background: linear-gradient(135deg, #a94af3 0%, #1767ff 100%);
  color: #fff;
  font-size: 25rpx;
  font-weight: 900;
  box-shadow: 0 14rpx 30rpx rgba(82, 90, 235, 0.3);
}

.mode-label text,
.mode-current text {
  display: block;
  min-width: 0;
  white-space: nowrap;
}

.chevron {
  flex: 0 0 auto;
  font-size: 30rpx;
  line-height: 1;
}

.page-intro {
  margin-top: 28rpx;
}

.page-title {
  color: #111827;
  font-size: 42rpx;
  font-weight: 950;
  line-height: 1.18;
}

.page-subtitle {
  margin-top: 10rpx;
  color: #68758c;
  font-size: 24rpx;
  font-weight: 700;
  line-height: 1.5;
}

.creator-card {
  margin-top: 24rpx;
  padding: 22rpx;
  border: 1rpx solid rgba(143, 154, 177, 0.13);
  border-radius: 24rpx;
  background: rgba(255, 255, 255, 0.88);
  box-shadow: 0 16rpx 38rpx rgba(31, 45, 82, 0.045);
}

.section-head {
  justify-content: space-between;
  gap: 16rpx;
  min-width: 0;
}

.section-head > view {
  flex: 1 1 auto;
  flex-direction: column;
  align-items: flex-start;
  min-width: 0;
}

.section-head.compact {
  align-items: baseline;
}

.section-title {
  color: #172033;
  font-size: 27rpx;
  font-weight: 950;
}

.section-hint {
  margin-top: 8rpx;
  color: #7d8799;
  font-size: 21rpx;
  font-weight: 700;
  line-height: 1.42;
}

.counter {
  flex: 0 0 auto;
  color: #8a94a8;
  font-size: 22rpx;
  font-weight: 900;
}

.photo-section {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16rpx;
  margin-top: 18rpx;
}

.photo-slot {
  position: relative;
  display: grid;
  place-items: center;
  min-width: 0;
  height: 236rpx;
  overflow: hidden;
  border: 1rpx dashed rgba(133, 143, 166, 0.3);
  border-radius: 18rpx;
  background: rgba(247, 249, 253, 0.86);
  color: #66718a;
}

.photo-slot.filled {
  border-style: solid;
  border-color: rgba(36, 92, 255, 0.2);
  background: #eef3ff;
}

.photo-preview {
  width: 100%;
  height: 100%;
}

.photo-placeholder {
  display: grid;
  gap: 9rpx;
  justify-items: center;
  padding: 0 18rpx;
  text-align: center;
}

.slot-icon {
  width: 42rpx;
  height: 42rpx;
}

.photo-placeholder text:first-child {
  color: #172033;
  font-size: 24rpx;
  font-weight: 900;
}

.photo-placeholder text:last-child {
  color: #7d8799;
  font-size: 21rpx;
  font-weight: 750;
}

.slot-tag {
  position: absolute;
  left: 12rpx;
  bottom: 12rpx;
  padding: 7rpx 12rpx;
  border-radius: 999rpx;
  background: rgba(255, 255, 255, 0.82);
  color: #66718a;
  font-size: 19rpx;
  font-weight: 900;
  line-height: 1;
}

.photo-replace-badge {
  position: absolute;
  top: 14rpx;
  left: 14rpx;
  padding: 7rpx 14rpx;
  border-radius: 999rpx;
  background: rgba(20, 29, 48, 0.62);
  color: #fff;
  font-size: 20rpx;
  font-weight: 800;
  line-height: 1;
}

.photo-mask {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  background: rgba(255, 255, 255, 0.72);
  color: #245cff;
  font-size: 24rpx;
  font-weight: 850;
}

.photo-mask.error {
  color: #d33c3c;
}

.remove-photo {
  position: absolute;
  top: 12rpx;
  right: 12rpx;
  display: grid;
  place-items: center;
  width: 44rpx;
  height: 44rpx;
  border-radius: 999rpx;
  background: rgba(20, 29, 48, 0.7);
  color: #fff;
  font-size: 28rpx;
  line-height: 1;
}

.prompt-panel {
  margin-top: 14rpx;
  border: 1rpx solid rgba(143, 154, 177, 0.16);
  border-radius: 17rpx;
  background: rgba(255, 255, 255, 0.82);
}

.title-input-shell {
  display: flex;
  align-items: center;
  min-height: 76rpx;
  padding: 0 20rpx;
}

.title-input-shell input {
  width: 100%;
  min-height: 60rpx;
  color: #172033;
  font-size: 28rpx;
  font-weight: 650;
}

.location-scroll {
  width: 100%;
  margin-top: 16rpx;
  overflow: hidden;
  white-space: nowrap;
}

.location-track {
  display: inline-flex;
  gap: 14rpx;
  padding-right: 20rpx;
}

.location-card {
  position: relative;
  flex: 0 0 auto;
  width: 218rpx;
  height: 138rpx;
  overflow: hidden;
  border: 3rpx solid rgba(255, 255, 255, 0.74);
  border-radius: 18rpx;
  background: #e9eef8;
  text-align: left;
  box-shadow: 0 10rpx 24rpx rgba(31, 45, 82, 0.08);
  transition: transform 160ms ease, border-color 160ms ease, box-shadow 160ms ease;
}

.location-card.active {
  border-color: #245cff;
  box-shadow: 0 14rpx 30rpx rgba(82, 90, 235, 0.28);
  transform: translateY(-3rpx);
}

.location-card image,
.location-shade {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
}

.location-shade {
  background: linear-gradient(180deg, rgba(17, 24, 39, 0.02) 0%, rgba(17, 24, 39, 0.66) 100%);
}

.location-copy {
  position: absolute;
  left: 14rpx;
  right: 42rpx;
  bottom: 12rpx;
  min-width: 0;
  color: #fff;
}

.location-copy text:first-child {
  overflow: hidden;
  font-size: 22rpx;
  font-weight: 900;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.location-copy text:last-child {
  margin-top: 4rpx;
  overflow: hidden;
  color: rgba(255, 255, 255, 0.86);
  font-size: 17rpx;
  font-weight: 650;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.location-check {
  position: absolute;
  right: 10rpx;
  bottom: 10rpx;
  display: grid;
  place-items: center;
  width: 34rpx;
  height: 34rpx;
  border-radius: 999rpx;
  background: rgba(255, 255, 255, 0.28);
  color: transparent;
  font-size: 22rpx;
  font-weight: 900;
}

.location-card.active .location-check {
  background: linear-gradient(135deg, #a94af3 0%, #1767ff 100%);
  color: #fff;
}

.template-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14rpx;
  margin-top: 16rpx;
}

.template-card {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 8rpx;
  justify-content: center;
  align-content: center;
  justify-items: center;
  align-items: center;
  min-height: 136rpx;
  padding: 18rpx;
  border: 1rpx solid rgba(143, 154, 177, 0.15);
  border-radius: 18rpx;
  background: rgba(248, 250, 255, 0.82);
  text-align: center;
  transition: transform 160ms ease, border-color 160ms ease, background 160ms ease;
}

.template-card.active {
  border-color: rgba(36, 92, 255, 0.42);
  background: linear-gradient(135deg, rgba(169, 74, 243, 0.09), rgba(23, 103, 255, 0.08));
  transform: translateY(-2rpx);
}

.template-card image {
  width: 34rpx;
  height: 34rpx;
}

.template-card text:nth-of-type(1) {
  overflow: hidden;
  color: #172033;
  font-size: 25rpx;
  font-weight: 900;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.template-card text:last-child {
  color: #748097;
  font-size: 21rpx;
  font-weight: 700;
  line-height: 1.42;
  text-align: center;
}

.dream-role-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12rpx;
  margin-top: 14rpx;
}

.dream-role-card {
  display: grid;
  gap: 6rpx;
  min-height: 92rpx;
  padding: 14rpx 16rpx;
  border: 1rpx solid rgba(86, 106, 151, 0.12);
  border-radius: 16rpx;
  background: rgba(255, 251, 239, 0.68);
}

.dream-role-card text:first-child {
  color: #172033;
  font-size: 23rpx;
  font-weight: 950;
}

.dream-role-card text:last-child {
  color: #748097;
  font-size: 20rpx;
  font-weight: 700;
  line-height: 1.35;
}

.style-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14rpx;
  margin-top: 16rpx;
}

.style-card {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12rpx;
  min-width: 0;
  min-height: 76rpx;
  padding: 0 18rpx;
  border: 1rpx solid rgba(143, 154, 177, 0.15);
  border-radius: 18rpx;
  background: rgba(248, 250, 255, 0.82);
  color: #31405e;
  font-size: 23rpx;
  font-weight: 900;
  text-align: center;
}

.style-card image {
  flex: 0 0 auto;
  width: 34rpx;
  height: 34rpx;
}

.style-card text {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.style-card.active {
  border-color: rgba(36, 92, 255, 0.42);
  background: linear-gradient(135deg, #a94af3 0%, #1767ff 100%);
  color: #fff;
  box-shadow: 0 14rpx 30rpx rgba(82, 90, 235, 0.28);
}

.style-card.active image {
  filter: brightness(0) invert(1);
}

.privacy-note {
  margin-top: 18rpx;
  padding: 0 6rpx 6rpx;
  color: #7d8799;
  font-size: 21rpx;
  font-weight: 700;
  line-height: 1.5;
}

.error-strip {
  font-size: 21rpx;
  font-weight: 750;
  line-height: 1.48;
}

.error-strip {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16rpx;
  margin-top: 18rpx;
  padding: 18rpx 20rpx;
  border-radius: 22rpx;
  background: rgba(255, 240, 240, 0.82);
  color: #d33c3c;
}

.error-strip > view {
  display: grid;
  gap: 8rpx;
  min-width: 0;
}

.error-strip text {
  display: block;
}

.credit-shortfall-detail {
  display: flex;
  flex-wrap: wrap;
  gap: 8rpx 14rpx;
  color: #8f3131;
  font-size: 20rpx;
  font-weight: 850;
  line-height: 1.25;
}

.shortfall-pricing-button {
  flex: 0 0 auto;
  min-width: 124rpx;
  height: 52rpx;
  border-radius: 999rpx;
  background: #245cff;
  color: #fff;
  font-size: 22rpx;
  font-weight: 900;
  line-height: 52rpx;
}

.floating-generate-bar {
  position: fixed;
  right: 0;
  bottom: calc(112rpx + env(safe-area-inset-bottom));
  left: 0;
  z-index: 21;
  display: grid;
  padding: 14rpx 24rpx 10rpx;
  background: linear-gradient(180deg, rgba(248, 251, 255, 0), rgba(248, 251, 255, 0.94) 28%, #f3f8ff 100%);
}

.generate-album-button {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 94rpx;
  border-radius: 999rpx;
  background: linear-gradient(135deg, #a94af3 0%, #1767ff 100%);
  color: #fff;
  font-size: 30rpx;
  font-weight: 900;
  box-shadow: 0 22rpx 48rpx rgba(82, 90, 235, 0.3);
}

.generate-album-button.disabled {
  background: #dfe7f6;
  color: rgba(100, 113, 137, 0.84);
  box-shadow: 0 16rpx 36rpx rgba(82, 99, 145, 0.14);
}

.modal-backdrop {
  position: fixed;
  inset: 0;
  z-index: 40;
  display: grid;
  place-items: center;
  padding: 40rpx;
  background: rgba(23, 31, 49, 0.08);
  backdrop-filter: blur(4rpx);
}

.mode-modal {
  width: min(100%, 560rpx);
  max-height: calc(100vh - 80rpx);
  padding: 24rpx;
  overflow-y: auto;
  border-radius: 28rpx;
  background: rgba(255, 255, 255, 0.96);
  box-shadow: 0 24rpx 70rpx rgba(37, 49, 83, 0.16);
}

.drag-handle {
  width: 72rpx;
  height: 6rpx;
  margin: 0 auto 40rpx;
  border-radius: 999rpx;
  background: #d5d9e2;
}

.modal-title {
  color: #111827;
  text-align: center;
  font-size: 30rpx;
  font-weight: 950;
}

.modal-subtitle {
  margin-top: 13rpx;
  color: #7c8699;
  text-align: center;
  font-size: 22rpx;
  font-weight: 800;
}

.mode-card {
  position: relative;
  width: 100%;
  min-height: 126rpx;
  margin-top: 28rpx;
  gap: 18rpx;
  padding: 20rpx 18rpx;
  border: 1rpx solid rgba(142, 153, 177, 0.18);
  border-radius: 15rpx;
  background: rgba(255, 255, 255, 0.92);
  text-align: left;
}

.mode-card image {
  flex: 0 0 auto;
  width: 58rpx;
  height: 58rpx;
}

.mode-card view {
  flex: 1;
  min-width: 0;
}

.mode-card view text:first-child {
  color: #172033;
  font-size: 26rpx;
  font-weight: 950;
}

.mode-card view text:nth-child(2) {
  margin-top: 10rpx;
  color: #4f5b73;
  font-size: 21rpx;
  font-weight: 800;
}

.mode-card view text:nth-child(3) {
  margin-top: 9rpx;
  color: #8a94a8;
  font-size: 20rpx;
  font-weight: 700;
}

.mode-card.selected {
  border-color: #7b4fff;
  background: rgba(250, 249, 255, 0.98);
}

.mode-card.selected view text:first-child {
  color: #4e55f5;
}

.mode-card-link {
  background: linear-gradient(180deg, rgba(255, 250, 253, 0.98), rgba(255, 255, 255, 0.96));
}

.arrow {
  color: #718096;
  font-size: 42rpx;
}

.selected-dot {
  display: grid;
  place-items: center;
  width: 32rpx;
  height: 32rpx;
  border-radius: 50%;
  background: #4f62ff;
  color: #fff;
  font-size: 22rpx;
  font-weight: 950;
}

.mode-tips {
  display: grid;
  gap: 15rpx;
  margin-top: 28rpx;
  padding: 22rpx;
  border-radius: 16rpx;
  background: linear-gradient(180deg, rgba(248, 247, 255, 0.96), rgba(244, 244, 252, 0.94));
}

.mode-tips text {
  gap: 10rpx;
  color: #687389;
  font-size: 21rpx;
  font-weight: 800;
}

.mode-tips text:first-child {
  color: #6958ff;
  font-size: 22rpx;
  font-weight: 950;
}

.mode-tips text:not(:first-child)::before {
  content: '';
  width: 10rpx;
  height: 10rpx;
  border: 2rpx solid #6958ff;
  border-radius: 3rpx;
  transform: rotate(45deg);
}
</style>
