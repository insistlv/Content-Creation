<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { BadgeCheck, Crown, Image, Leaf, Library, Rocket, ShieldCheck, Sparkles, Video, Zap } from 'lucide-vue-next'

import { api } from '../api/client.js'
import { setCurrentUser } from '../stores/session.js'

const router = useRouter()
const route = useRoute()
const packages = ref([])
const me = ref(null)
const loading = ref(false)
const errorMessage = ref('')
const checkoutLoadingPackageId = ref(null)
const phoneBindModalOpen = ref(false)
const selectedPackage = ref(null)
const sendingBindPhoneCode = ref(false)
const bindingPhone = ref(false)
const bindPhoneCountdown = ref(0)
const bindPhoneMessage = ref('')
const bindPhoneError = ref('')
let bindPhoneTimer = null

const bindPhoneForm = reactive({
  phone: '',
  code: ''
})

const capabilityItems = [
  { icon: Image, title: '图片生成', text: '文生图 / 图生图，风格多样，高清输出' },
  { icon: Video, title: '视频生成', text: '支持文生视频、参考图与图生视频相关创作能力' },
  { icon: Library, title: '作品入库', text: '私有作品库，图片与视频作品统一存储管理' },
  { icon: ShieldCheck, title: '失败保护', text: '失败任务不扣点，以生成页实时提示为准' }
]

const defaultComparisonLabels = ['点数', '图片生成', '视频生成', '图生视频 / 参考图能力', '高清下载', '私有作品库', '队列优先级', '商用授权', '适合人群']
const requiredPackageFeatures = [
  { text: '支持视频生成', matches: (feature) => feature.includes('视频生成') },
  { text: '支持参考图 / 图生视频', matches: (feature) => feature.includes('参考图') || feature.includes('图生视频') },
  {
    text: '失败任务不扣点，以生成页实时提示为准',
    matches: (feature) => feature.includes('失败任务不扣点') || feature.includes('失败不扣点') || feature.includes('生成失败不扣点')
  }
]
const requiredComparisonRows = [
  { label: '视频生成', value: '✓' },
  { label: '图生视频 / 参考图能力', value: '✓' }
]

const faqItems = [
  { title: '点数可以做什么？', text: '点数为平台创作资源，可用于图片生成、视频生成、作品管理等创作功能，不支持提现。' },
  {
    title: '支付后多久到账？',
    text: '支付宝支付成功并完成验签后会自动到账，如遇通知延迟可在支付页刷新状态。',
    anchorId: 'recharge-guide'
  },
  {
    title: '视频如何扣点？',
    text: '提交前工作台会实时预估消耗，最终以生成页显示为准。',
    anchorId: 'points-rules'
  },
  {
    title: '失败会扣点吗？',
    text: '生成失败不扣点，任务成功保存到作品库后才扣点。'
  }
]

const coupleAlbumPackageID = computed(() => route.query?.source === 'couple_album' ? numericQueryID(route.query?.package_id) : 0)
const videoGenerationPackageID = computed(() => route.query?.source === 'video_generation' ? numericQueryID(route.query?.package_id) : 0)
const coupleAlbumRecommendedPackage = computed(() => packages.value.find((item) => Number(item.id) === coupleAlbumPackageID.value) || null)
const videoGenerationRecommendedPackage = computed(() => packages.value.find((item) => Number(item.id) === videoGenerationPackageID.value) || null)
const coupleAlbumRechargeContext = computed(() => buildRechargeContext('couple_album', coupleAlbumRecommendedPackage.value))
const videoGenerationRechargeContext = computed(() => buildRechargeContext('video_generation', videoGenerationRecommendedPackage.value))
const displayPackages = computed(() => packages.value)
const bindPhoneValid = computed(() => /^1[3-9]\d{9}$/.test(bindPhoneForm.phone.trim()))
const bindPhoneCodeValid = computed(() => /^\d{6}$/.test(bindPhoneForm.code.trim()))
const bindPhoneCodeButtonText = computed(() => {
  if (bindPhoneCountdown.value > 0) return `${bindPhoneCountdown.value}s`
  if (sendingBindPhoneCode.value) return '发送中...'
  return '获取验证码'
})
const comparisonRows = computed(() => {
  const configuredLabels = []
  displayPackages.value.forEach((item) => {
    ;(item.benefits ?? []).forEach((benefit) => {
      if (benefit?.label && !configuredLabels.includes(benefit.label)) {
        configuredLabels.push(benefit.label)
      }
    })
  })
  const labels = configuredLabels.length > 0 ? withRequiredComparisonLabels(configuredLabels) : defaultComparisonLabels
  return labels.map((label) => ({
    label,
    values: displayPackages.value.map((item, index) => {
      const benefit = (item.benefits ?? []).find((entry) => entry.label === label)
      return benefit?.value || requiredComparisonRows.find((row) => row.label === label)?.value || fallbackComparisonValue(item, index, label)
    })
  }))
})

function numericQueryID(value) {
  const id = Number(value ?? 0)
  return Number.isFinite(id) ? id : 0
}

function buildRechargeContext(source, recommendedPackage) {
  if (route.query?.source !== source) return null
  const missingCredits = Number(route.query?.missing_credits ?? 0)
  const requiredCredits = Number(route.query?.required_credits ?? 0)
  return {
    missing_credits: Number.isFinite(missingCredits) ? missingCredits : 0,
    required_credits: Number.isFinite(requiredCredits) ? requiredCredits : 0,
    recommended_package: recommendedPackage
  }
}

function normalizePrice(priceLabel) {
  return `${priceLabel ?? ''}`
}

function clearBindPhoneTimer() {
  if (bindPhoneTimer) {
    clearInterval(bindPhoneTimer)
    bindPhoneTimer = null
  }
}

function startBindPhoneCountdown(seconds = 60) {
  bindPhoneCountdown.value = seconds
  clearBindPhoneTimer()
  bindPhoneTimer = setInterval(() => {
    bindPhoneCountdown.value -= 1
    if (bindPhoneCountdown.value <= 0) {
      bindPhoneCountdown.value = 0
      clearBindPhoneTimer()
    }
  }, 1000)
}

function openBindPhoneModal(item) {
  selectedPackage.value = item
  phoneBindModalOpen.value = true
  errorMessage.value = ''
  bindPhoneError.value = ''
  bindPhoneMessage.value = ''
  bindPhoneForm.phone = bindPhoneForm.phone || me.value?.phone || ''
  bindPhoneForm.code = ''
}

function closeBindPhoneModal() {
  phoneBindModalOpen.value = false
  selectedPackage.value = null
  bindPhoneError.value = ''
  bindPhoneMessage.value = ''
  bindPhoneForm.code = ''
}

function withRequiredFeatures(features) {
  return requiredPackageFeatures.reduce((result, item) => {
    if (result.some((feature) => item.matches(feature))) return result
    return [...result, item.text]
  }, features)
}

function withRequiredComparisonLabels(labels) {
  const result = [...labels]
  const imageIndex = result.indexOf('图片生成')
  let insertAt = imageIndex >= 0 ? imageIndex + 1 : result.length
  requiredComparisonRows.forEach((row) => {
    const existingIndex = result.indexOf(row.label)
    if (existingIndex >= 0) {
      insertAt = Math.max(insertAt, existingIndex + 1)
      return
    }
    result.splice(insertAt, 0, row.label)
    insertAt += 1
  })
  return result
}

function fallbackComparisonValue(item, index, label) {
  const meta = planMeta(item, index)
  if (label === '点数') return `${Number(item?.credits ?? 0)} 点`
  if (label === '队列优先级') return defaultQueuePriority(item, index)
  if (label === '商用授权') return meta.features.some((feature) => `${feature}`.includes('商用授权')) ? '✓' : '—'
  if (label === '适合人群') return item?.audience || defaultAudience(item, index)
  return '✓'
}

function defaultQueuePriority(item, index) {
  const name = `${item?.name ?? ''}`
  const credits = Number(item?.credits ?? 0)
  if (name.includes('旗舰') || credits >= 5000 || index >= 5) return '最高优先'
  if (name.includes('专业') || name.includes('进阶') || credits >= 1400 || index >= 3) return '更高优先'
  if (name.includes('入门') || name.includes('常用') || credits >= 188 || index >= 1) return '优先'
  return '普通'
}

function defaultAudience(item, index) {
  const name = `${item?.name ?? ''}`
  const credits = Number(item?.credits ?? 0)
  if (name.includes('旗舰') || credits >= 5000 || index >= 5) return '团队 / 工作室'
  if (name.includes('专业') || credits >= 2500 || index >= 4) return '专业创作者'
  if (name.includes('进阶') || credits >= 1400 || index >= 3) return '商业创作者'
  if (name.includes('常用') || credits >= 600 || index >= 2) return '高频创作者'
  if (name.includes('入门') || credits >= 188 || index >= 1) return '个人创作者'
  return '新手体验'
}

function planMeta(item, index) {
  const name = item?.name ?? ''
  const credits = Number(item?.credits ?? 0)
  const configuredFeatures = Array.isArray(item?.features) && item.features.length > 0 ? withRequiredFeatures(item.features) : null
  const contextRecommended = isContextRecommendedPackage(item)
  if (name.includes('旗舰') || credits >= 5000 || index >= 5) {
    return {
      icon: Crown,
      badge: item.badge || '最划算',
      accent: item.theme || 'gold',
      recommended: Boolean(item.recommended) || contextRecommended,
      features: configuredFeatures || ['支持图片生成', '支持视频生成', '支持参考图 / 图生视频', '作品入库与历史管理', '失败任务不扣点，以生成页实时提示为准', '商用授权', '长期储备', '最高优先']
    }
  }
  if (name.includes('专业') || credits >= 2500 || index === 4) {
    return {
      icon: BadgeCheck,
      badge: item.badge || '推荐',
      accent: item.theme || 'rose',
      recommended: contextRecommended || (item.recommended ?? true),
      features: configuredFeatures || ['支持图片生成', '支持视频生成', '支持参考图 / 图生视频', '作品入库与历史管理', '失败任务不扣点，以生成页实时提示为准', '商用授权', '专业交付']
    }
  }
  if (name.includes('进阶') || credits >= 1400 || index === 3) {
    return {
      icon: Rocket,
      badge: item.badge || '进阶',
      accent: item.theme || 'violet',
      recommended: Boolean(item.recommended) || contextRecommended,
      features: configuredFeatures || ['支持图片生成', '支持视频生成', '支持参考图 / 图生视频', '作品入库与历史管理', '失败任务不扣点，以生成页实时提示为准', '商用授权', '批量生成']
    }
  }
  if (name.includes('常用') || credits >= 600 || index === 2) {
    return {
      icon: Zap,
      badge: item.badge || '常用',
      accent: item.theme || 'orange',
      recommended: Boolean(item.recommended) || contextRecommended,
      features: configuredFeatures || ['支持图片生成', '支持视频生成', '支持参考图 / 图生视频', '作品入库与历史管理', '失败任务不扣点，以生成页实时提示为准', '商用授权']
    }
  }
  if (name.includes('入门') || credits >= 188 || index === 1) {
    return {
      icon: Leaf,
      badge: item.badge || '入门',
      accent: item.theme || 'green',
      recommended: Boolean(item.recommended) || contextRecommended,
      features: configuredFeatures || ['支持图片生成', '支持视频生成', '支持参考图 / 图生视频', '作品入库与历史管理', '失败任务不扣点，以生成页实时提示为准', '优先队列']
    }
  }
  return {
    icon: Sparkles,
    badge: item.badge || '体验',
    accent: item.theme || 'blue',
    recommended: Boolean(item.recommended) || contextRecommended,
    features: configuredFeatures || ['支持图片生成', '支持视频生成', '支持参考图 / 图生视频', '作品入库与历史管理', '失败任务不扣点，以生成页实时提示为准', '基础排队']
  }
}

function isContextRecommendedPackage(item) {
  return (coupleAlbumPackageID.value > 0 && Number(item?.id) === coupleAlbumPackageID.value) ||
    (videoGenerationPackageID.value > 0 && Number(item?.id) === videoGenerationPackageID.value)
}

function cardClasses(item, index) {
  const meta = planMeta(item, index)
  return [
    'pricing-plan-card',
    `pricing-plan-${meta.accent}`,
    {
      'pricing-plan-featured': meta.recommended,
      'pricing-plan-context-recommended': isContextRecommendedPackage(item)
    }
  ]
}

async function load() {
  loading.value = true
  errorMessage.value = ''
  try {
    const [packageData, meResult] = await Promise.all([
      api.getPackages(),
      api.getMe().catch(() => null)
    ])
    packages.value = packageData.items ?? []
    me.value = meResult
  } catch (error) {
    errorMessage.value = error.message
  } finally {
    loading.value = false
  }
}

async function submitIntent(item) {
  errorMessage.value = ''
  if (!me.value?.user_id) {
    router.push('/login')
    return
  }
  if (!me.value?.phone) {
    openBindPhoneModal(item)
    return
  }
  await createAlipayOrder(item)
}

async function createAlipayOrder(item) {
  checkoutLoadingPackageId.value = item.id
  try {
    const order = await api.createAlipayOrder({ package_id: item.id })
    router.push(`/checkout/alipay/${order.order_number}`)
  } catch (error) {
    if (error?.code === 'phone_binding_required') {
      openBindPhoneModal(item)
    } else {
      errorMessage.value = error.message
    }
  } finally {
    checkoutLoadingPackageId.value = null
  }
}

async function sendBindPhoneCode() {
  if (sendingBindPhoneCode.value || bindPhoneCountdown.value > 0) return
  if (!bindPhoneValid.value) {
    bindPhoneError.value = '请输入有效手机号。'
    bindPhoneMessage.value = ''
    return
  }
  sendingBindPhoneCode.value = true
  bindPhoneError.value = ''
  bindPhoneMessage.value = ''
  try {
    await api.sendSMSCode({
      phone: bindPhoneForm.phone.trim(),
      purpose: 'bind_phone'
    })
    bindPhoneMessage.value = '验证码已发送，请注意查收。'
    startBindPhoneCountdown()
  } catch (error) {
    bindPhoneError.value = error.message
  } finally {
    sendingBindPhoneCode.value = false
  }
}

async function bindPhoneAndContinue() {
  if (!bindPhoneValid.value) {
    bindPhoneError.value = '请输入有效手机号。'
    bindPhoneMessage.value = ''
    return
  }
  if (!bindPhoneCodeValid.value) {
    bindPhoneError.value = '请输入 6 位短信验证码。'
    bindPhoneMessage.value = ''
    return
  }
  bindingPhone.value = true
  bindPhoneError.value = ''
  bindPhoneMessage.value = ''
  try {
    const payload = await api.bindAccountPhone({
      phone: bindPhoneForm.phone.trim(),
      verification_code: bindPhoneForm.code.trim()
    })
    const updatedUser = {
      ...(me.value ?? {}),
      ...(payload ?? {}),
      phone: payload?.phone ?? bindPhoneForm.phone.trim()
    }
    const packageToCheckout = selectedPackage.value
    me.value = updatedUser
    setCurrentUser(updatedUser)
    phoneBindModalOpen.value = false
    selectedPackage.value = null
    bindPhoneCountdown.value = 0
    clearBindPhoneTimer()
    bindPhoneForm.phone = updatedUser.phone ?? ''
    bindPhoneForm.code = ''
    bindPhoneMessage.value = ''
    if (packageToCheckout) await createAlipayOrder(packageToCheckout)
  } catch (error) {
    bindPhoneError.value = error.message
  } finally {
    bindingPhone.value = false
  }
}

onMounted(load)

onBeforeUnmount(() => {
  clearBindPhoneTimer()
})
</script>

<template>
  <section class="pricing-page pricing-agent-page">
    <header class="pricing-hero">
      <span class="pricing-hero-pill">✦ AI Image + Video</span>
      <h1>白霖共享 AI 价目表</h1>
      <p>点数可用于图片生成、视频生成、作品管理等创作能力，适配日常内容制作与商业创意工作流</p>
      <div class="pricing-hero-notes" aria-label="套餐说明">
        <span>✓ 支付宝自动到账</span>
        <span>✓ 虚拟商品线上交付</span>
      </div>
    </header>

    <div v-if="coupleAlbumRechargeContext" class="pricing-context-banner" data-testid="pricing-couple-album-shortfall">
      <strong>情侣相册点数不足</strong>
      <span>
        本次还差 {{ coupleAlbumRechargeContext.missing_credits }} 点，
        本次预计消耗 {{ coupleAlbumRechargeContext.required_credits }} 点
        <template v-if="coupleAlbumRechargeContext.recommended_package">
          ，推荐购买「{{ coupleAlbumRechargeContext.recommended_package.name }}」
        </template>
      </span>
    </div>

    <div v-if="videoGenerationRechargeContext" class="pricing-context-banner" data-testid="pricing-video-generation-shortfall">
      <strong>视频生成点数不足</strong>
      <span>
        本次还差 {{ videoGenerationRechargeContext.missing_credits }} 点，
        本次预计消耗 {{ videoGenerationRechargeContext.required_credits }} 点
        <template v-if="videoGenerationRechargeContext.recommended_package">
          ，推荐购买「{{ videoGenerationRechargeContext.recommended_package.name }}」
        </template>
      </span>
    </div>

    <div v-if="displayPackages.length" class="pricing-plan-grid">
      <article
        v-for="(item, index) in displayPackages"
        :key="item.id"
        :class="cardClasses(item, index)"
        :data-testid="`pricing-package-card-${item.id}`"
      >
        <span v-if="planMeta(item, index).recommended" class="pricing-recommend-ribbon">推荐</span>
        <div class="pricing-plan-head">
          <span class="pricing-plan-icon">
            <component :is="planMeta(item, index).icon" :size="24" stroke-width="2.2" />
          </span>
          <div>
            <div class="pricing-plan-title-row">
              <h2>{{ item.name }}</h2>
              <span>{{ planMeta(item, index).badge }}</span>
            </div>
            <p>{{ item.credits }} 点</p>
          </div>
        </div>

        <div class="pricing-plan-price">
          <strong>{{ normalizePrice(item.price_label) }}</strong>
        </div>
        <p class="pricing-plan-copy">{{ item.description }}</p>

        <ul class="pricing-feature-list">
          <li v-for="feature in planMeta(item, index).features" :key="feature">
            <span aria-hidden="true">✓</span>
            {{ feature }}
          </li>
        </ul>

        <button
          class="pricing-plan-button"
          type="button"
          :data-package-id="item.id"
          :disabled="checkoutLoadingPackageId === item.id"
          @click="submitIntent(item)"
        >
          {{ checkoutLoadingPackageId === item.id ? '创建订单中...' : `选择${item.name}` }}
        </button>
      </article>
    </div>

    <div class="pricing-capability-rail">
      <article v-for="item in capabilityItems" :key="item.title">
        <span><component :is="item.icon" :size="21" stroke-width="2.2" /></span>
        <div>
          <strong>{{ item.title }}</strong>
          <p>{{ item.text }}</p>
        </div>
      </article>
    </div>

    <div class="pricing-bottom-grid">
      <section class="pricing-compare-panel">
        <h2>套餐权益对比</h2>
        <div
          class="pricing-compare-table"
          data-testid="pricing-comparison-table"
          :style="{ '--pricing-plan-count': displayPackages.length }"
        >
          <div class="pricing-compare-row pricing-compare-head">
            <span>权益 / 套餐</span>
            <strong v-for="item in displayPackages" :key="item.id">{{ item.name }}</strong>
          </div>
          <div v-for="row in comparisonRows" :key="row.label" class="pricing-compare-row">
            <span>{{ row.label }}</span>
            <strong v-for="(value, index) in row.values" :key="`${row.label}-${index}`">{{ value }}</strong>
          </div>
        </div>
      </section>

      <section class="pricing-faq-panel">
        <h2>常见问题</h2>
        <details v-for="item in faqItems" :id="item.anchorId" :key="item.title" open>
          <summary>{{ item.title }}</summary>
          <p>{{ item.text }}</p>
        </details>
        <p class="pricing-faq-help">如需帮助，可查看帮助中心或联系客服。</p>
      </section>
    </div>

    <div
      v-if="phoneBindModalOpen"
      class="pricing-contact-modal-backdrop pricing-phone-bind-backdrop"
      data-testid="pricing-phone-bind-modal"
      role="dialog"
      aria-modal="true"
      aria-labelledby="pricing-phone-bind-title"
      @click.self="closeBindPhoneModal"
    >
      <form class="pricing-contact-modal pricing-phone-bind-modal" @submit.prevent="bindPhoneAndContinue">
        <span class="pricing-contact-eyebrow">Phone</span>
        <h2 id="pricing-phone-bind-title">绑定手机号后继续支付</h2>
        <p class="pricing-phone-bind-summary">
          当前选择
          <strong>{{ selectedPackage?.name || '套餐' }}</strong>
          <span>{{ normalizePrice(selectedPackage?.price_label) }}</span>
        </p>

        <label class="pricing-bind-field">
          <span>手机号</span>
          <input
            v-model="bindPhoneForm.phone"
            data-testid="pricing-bind-phone-input"
            inputmode="tel"
            maxlength="11"
            type="text"
            placeholder="请输入大陆手机号"
          />
        </label>

        <div class="pricing-bind-code-row">
          <label class="pricing-bind-field">
            <span>短信验证码</span>
            <input
              v-model="bindPhoneForm.code"
              data-testid="pricing-bind-phone-code"
              inputmode="numeric"
              maxlength="6"
              type="text"
              placeholder="6 位验证码"
            />
          </label>
          <button
            data-testid="pricing-send-bind-phone-code"
            type="button"
            :disabled="sendingBindPhoneCode || bindPhoneCountdown > 0 || !bindPhoneValid"
            @click="sendBindPhoneCode"
          >
            {{ bindPhoneCodeButtonText }}
          </button>
        </div>

        <p v-if="bindPhoneMessage" class="status-success pricing-bind-feedback">{{ bindPhoneMessage }}</p>
        <p v-if="bindPhoneError" class="status-error pricing-bind-feedback" role="alert">{{ bindPhoneError }}</p>

        <div class="pricing-contact-actions pricing-phone-bind-actions">
          <button
            data-testid="pricing-bind-phone-submit"
            type="button"
            :disabled="bindingPhone || !bindPhoneValid || !bindPhoneCodeValid"
            @click="bindPhoneAndContinue"
          >
            {{ bindingPhone ? '绑定中...' : '确认绑定并继续支付' }}
          </button>
          <button class="secondary" data-testid="pricing-bind-phone-cancel" type="button" @click="closeBindPhoneModal">
            取消
          </button>
        </div>
      </form>
    </div>

    <p v-if="errorMessage" class="status-error">{{ errorMessage }}</p>
    <p v-if="loading" class="page-status">加载中...</p>
  </section>
</template>
