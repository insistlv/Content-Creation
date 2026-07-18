<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { CheckCircle2, Copy, Download, ImagePlus, Loader2, Megaphone, RefreshCw, Send, X } from 'lucide-vue-next'
import { RouterLink } from 'vue-router'
import { api } from '../api/client.js'
import { useUserTheme } from '../composables/useUserTheme.js'

const outputTypes = [
  { key: 'copy_image_separate', label: '文案图分离' },
  { key: 'poster_overlay', label: '图上排版文案' },
  { key: 'nine_grid_campaign', label: '九宫格 campaign' }
]

const inputMode = ref('text')
const outputType = ref('copy_image_separate')
const imageCount = ref(3)
const brief = ref('')
const productName = ref('')
const sellingPoints = ref('')
const targetAudience = ref('')
const promotion = ref('')
const tone = ref('自然亲切')
const cta = ref('')
const referenceAssets = ref([])
const uploading = ref(false)
const submitting = ref(false)
const pageError = ref('')
const copyState = ref('')
const plan = ref(null)
const tasks = ref([])

const activeTaskStatuses = new Set(['queued', 'running'])
const storageKey = 'image-agent.moments-marketing.active-tasks'
let pollTimer = null

const { theme } = useUserTheme()
const momentsThemeClass = computed(() => (theme.value === 'light' ? 'moments-workspace-light' : 'moments-workspace-dark'))
const selectedOutput = computed(() => outputTypes.find((item) => item.key === outputType.value) || outputTypes[0])
const effectiveImageCount = computed(() => clampImageCount(imageCount.value))
const canSubmit = computed(() => !submitting.value && !uploading.value && (brief.value.trim() || productName.value.trim()) && (inputMode.value !== 'photo' || referenceAssets.value.length > 0))
const activeTasks = computed(() => tasks.value.filter((task) => activeTaskStatuses.has(task.status)))
const imageCards = computed(() => Array.isArray(plan.value?.image_cards) ? plan.value.image_cards : [])

function clampImageCount(value) {
  const next = Math.round(Number(value) || 0)
  return Math.min(9, Math.max(1, next || (outputType.value === 'nine_grid_campaign' ? 9 : 3)))
}

function setOutputType(type) {
  outputType.value = type
  if (type === 'nine_grid_campaign') {
    imageCount.value = 9
  } else if (Number(imageCount.value) === 9) {
    imageCount.value = 3
  }
}

function removeReferenceAsset(id) {
  referenceAssets.value = referenceAssets.value.filter((item) => item.id !== id)
}

async function handleReferenceChange(event) {
  const files = Array.from(event?.target?.files || []).slice(0, Math.max(0, 4 - referenceAssets.value.length))
  if (files.length === 0 || uploading.value) return
  uploading.value = true
  pageError.value = ''
  try {
    for (const file of files) {
      const uploaded = await api.uploadReferenceAsset(file)
      referenceAssets.value = [
        ...referenceAssets.value,
        uploaded
      ].slice(0, 4)
    }
  } catch (error) {
    pageError.value = error.message || '实图上传失败'
  } finally {
    uploading.value = false
    if (event?.target) {
      event.target.value = ''
    }
  }
}

async function submitPlan() {
  if (!canSubmit.value) return
  submitting.value = true
  pageError.value = ''
  copyState.value = ''
  plan.value = null
  tasks.value = []
  try {
    const requestPayload = buildPlanPayload()
    const nextPlan = await api.planMomentsMarketing(requestPayload)
    plan.value = nextPlan
    await createCampaignTasks(nextPlan)
  } catch (error) {
    pageError.value = error.message || '朋友圈营销方案生成失败'
  } finally {
    submitting.value = false
  }
}

function buildPlanPayload() {
  return {
    input_mode: inputMode.value,
    output_type: outputType.value,
    image_count: effectiveImageCount.value,
    brief: brief.value.trim(),
    product_name: productName.value.trim(),
    selling_points: sellingPoints.value.trim(),
    target_audience: targetAudience.value.trim(),
    promotion: promotion.value.trim(),
    tone: tone.value.trim(),
    cta: cta.value.trim(),
    reference_asset_ids: inputMode.value === 'photo' ? referenceAssets.value.map((item) => item.id).filter(Boolean) : []
  }
}

async function createCampaignTasks(nextPlan) {
  const cards = Array.isArray(nextPlan?.image_cards) ? nextPlan.image_cards : []
  if (cards.length === 0) return
  const batchID = `moments-${Date.now()}-${Math.random().toString(16).slice(2, 8)}`
  const createdTasks = []
  for (const [index, card] of cards.entries()) {
    const task = await createOneImageTask(card, {
      batchID,
      batchIndex: index + 1,
      batchTotal: cards.length
    })
    createdTasks.push(task)
  }
  tasks.value = createdTasks
  persistTasks()
  startPolling()
}

async function createOneImageTask(card, batch) {
  const payload = {
    prompt: buildImagePrompt(card),
    aspect_ratio: '1:1',
    batch_id: batch.batchID,
    batch_index: batch.batchIndex,
    batch_total: batch.batchTotal,
    tool_mode: 'generate'
  }
  if (inputMode.value === 'photo') {
    payload.reference_asset_ids = referenceAssets.value.map((item) => item.id).filter(Boolean)
    payload.reference_weight = 80
    payload.reference_intent = 'compose'
  }
  const response = await api.createImageGeneration(payload)
  return normalizeTask({
    ...response,
    card,
    prompt: response.prompt || payload.prompt,
    parameters: response.parameters || payload
  })
}

function buildImagePrompt(card) {
  const base = (card?.visual_prompt || '').trim()
  const referenceRule = inputMode.value === 'photo'
    ? '保留参考图中真实产品、门店或服务的关键特征，构图升级为朋友圈商业宣传图。'
    : ''
  return [base, referenceRule, '画面不要出现中文广告字、二维码、水印、品牌 Logo 乱入。'].filter(Boolean).join('\n')
}

function normalizeTask(value) {
  return {
    generation_id: Number(value?.generation_id),
    status: value?.status || 'queued',
    stage: value?.stage || value?.status || 'queued',
    prompt: value?.prompt || '',
    parameters: value?.parameters || null,
    preview_url: value?.preview_url || '',
    download_url: value?.download_url || '',
    work_id: value?.work_id || null,
    error: value?.error || null,
    card: value?.card || null
  }
}

function upsertTask(nextTask) {
  const task = normalizeTask(nextTask)
  if (!task.generation_id) return
  const exists = tasks.value.some((item) => Number(item.generation_id) === task.generation_id)
  tasks.value = exists
    ? tasks.value.map((item) => Number(item.generation_id) === task.generation_id ? { ...item, ...task, card: task.card || item.card } : item)
    : [...tasks.value, task]
}

async function pollOneTask(task) {
  try {
    const payload = await api.getImageGeneration(task.generation_id)
    upsertTask({
      ...task,
      ...payload,
      card: task.card
    })
  } catch (error) {
    // 轮询失败不改任务状态，下一轮继续连接。
  }
}

async function pollTasks() {
  const pending = activeTasks.value
  if (pending.length === 0) {
    stopPolling()
    persistTasks()
    return
  }
  await Promise.all(pending.map((task) => pollOneTask(task)))
  persistTasks()
}

function startPolling() {
  if (pollTimer !== null) return
  pollTimer = window.setInterval(() => {
    void pollTasks()
  }, 1500)
}

function stopPolling() {
  if (pollTimer !== null) {
    window.clearInterval(pollTimer)
    pollTimer = null
  }
}

async function retryTask(task) {
  if (!task?.card || submitting.value) return
  pageError.value = ''
  try {
    const replacement = await createOneImageTask(task.card, {
      batchID: task.parameters?.batch_id || `moments-${Date.now()}`,
      batchIndex: task.parameters?.batch_index || task.card.slot || 1,
      batchTotal: task.parameters?.batch_total || imageCards.value.length || 1
    })
    tasks.value = tasks.value.map((item) => Number(item.generation_id) === Number(task.generation_id) ? replacement : item)
    persistTasks()
    startPolling()
  } catch (error) {
    pageError.value = error.message || '重试失败'
  }
}

function persistTasks() {
  try {
    const snapshots = tasks.value.filter((task) => activeTaskStatuses.has(task.status))
    if (snapshots.length === 0) {
      window.localStorage.removeItem(storageKey)
      return
    }
    window.localStorage.setItem(storageKey, JSON.stringify(snapshots))
  } catch (error) {
    console.warn('Failed to persist moments marketing tasks:', error)
  }
}

function restoreTasks() {
  try {
    const raw = window.localStorage.getItem(storageKey)
    if (!raw) return
    const snapshots = JSON.parse(raw)
    tasks.value = (Array.isArray(snapshots) ? snapshots : []).map(normalizeTask).filter((task) => task.generation_id)
    if (activeTasks.value.length > 0) {
      startPolling()
    }
  } catch (error) {
    window.localStorage.removeItem(storageKey)
  }
}

async function copyMomentsText() {
  const text = plan.value?.moments_text || ''
  if (!text) return
  try {
    await navigator.clipboard?.writeText(text)
    copyState.value = '已复制'
  } catch (error) {
    copyState.value = '复制失败'
  }
}

function downloadTask(task) {
  const url = task?.download_url || task?.preview_url
  if (!url) return
  window.open(url, '_blank')
}

function downloadAll() {
  tasks.value.filter((task) => task.download_url || task.preview_url).forEach((task) => {
    downloadTask(task)
  })
}

async function downloadPoster(task) {
  if (!task?.preview_url || !task?.card) {
    downloadTask(task)
    return
  }
  const canvas = document.createElement('canvas')
  canvas.width = 1080
  canvas.height = 1080
  const context = canvas.getContext('2d')
  if (!context) return
  await drawPosterImage(context, canvas, task.preview_url)
  drawPosterOverlay(context, canvas, task.card)
  const link = document.createElement('a')
  link.download = `moments-poster-${task.card.slot || task.generation_id}.png`
  link.href = canvas.toDataURL('image/png')
  link.click()
}

function drawPosterImage(context, canvas, url) {
  return new Promise((resolve) => {
    const image = new Image()
    image.crossOrigin = 'anonymous'
    image.onload = () => {
      const scale = Math.max(canvas.width / image.width, canvas.height / image.height)
      const width = image.width * scale
      const height = image.height * scale
      context.drawImage(image, (canvas.width - width) / 2, (canvas.height - height) / 2, width, height)
      resolve()
    }
    image.onerror = () => {
      context.fillStyle = '#263238'
      context.fillRect(0, 0, canvas.width, canvas.height)
      resolve()
    }
    image.src = url
  })
}

function drawPosterOverlay(context, canvas, card) {
  const gradient = context.createLinearGradient(0, canvas.height * 0.42, 0, canvas.height)
  gradient.addColorStop(0, 'rgba(0,0,0,0)')
  gradient.addColorStop(1, 'rgba(0,0,0,0.72)')
  context.fillStyle = gradient
  context.fillRect(0, 0, canvas.width, canvas.height)

  context.fillStyle = '#ffffff'
  context.textBaseline = 'top'
  context.font = '700 78px sans-serif'
  wrapCanvasText(context, card.overlay_title || card.role || '朋友圈宣传', 72, 710, 936, 88, 2)
  context.font = '400 42px sans-serif'
  wrapCanvasText(context, card.overlay_subtitle || card.caption || '', 72, 890, 820, 54, 2)
  if (card.overlay_badge) {
    context.fillStyle = '#f7d56f'
    roundRect(context, 72, 620, Math.min(520, 84 + card.overlay_badge.length * 42), 72, 36)
    context.fill()
    context.fillStyle = '#202124'
    context.font = '700 34px sans-serif'
    context.fillText(card.overlay_badge, 112, 638)
  }
  if (card.cta) {
    context.fillStyle = '#ffffff'
    context.font = '700 32px sans-serif'
    context.fillText(card.cta, 72, 1016)
  }
}

function wrapCanvasText(context, text, x, y, maxWidth, lineHeight, maxLines) {
  const chars = String(text || '').split('')
  let line = ''
  let lines = 0
  chars.forEach((char, index) => {
    const next = line + char
    if (context.measureText(next).width > maxWidth && line) {
      context.fillText(line, x, y + lines * lineHeight)
      lines += 1
      line = char
    } else {
      line = next
    }
    if (index === chars.length - 1 && line && lines < maxLines) {
      context.fillText(line, x, y + lines * lineHeight)
    }
  })
}

function roundRect(context, x, y, width, height, radius) {
  context.beginPath()
  context.moveTo(x + radius, y)
  context.lineTo(x + width - radius, y)
  context.quadraticCurveTo(x + width, y, x + width, y + radius)
  context.lineTo(x + width, y + height - radius)
  context.quadraticCurveTo(x + width, y + height, x + width - radius, y + height)
  context.lineTo(x + radius, y + height)
  context.quadraticCurveTo(x, y + height, x, y + height - radius)
  context.lineTo(x, y + radius)
  context.quadraticCurveTo(x, y, x + radius, y)
  context.closePath()
}

onMounted(() => {
  restoreTasks()
})

onBeforeUnmount(() => {
  stopPolling()
})
</script>

<template>
  <main class="moments-workspace" :class="momentsThemeClass" :data-theme="theme">
    <header class="moments-toolbar">
      <RouterLink class="moments-back-link" to="/workspace">返回图像工坊</RouterLink>
      <div>
        <p>创作乐园</p>
        <h1>朋友圈广告营销</h1>
      </div>
    </header>

    <div class="moments-shell">
      <form class="moments-form" data-testid="moments-plan-form" @submit.prevent="submitPlan">
        <section class="moments-section">
          <div class="moments-section-title">
            <Megaphone :size="18" aria-hidden="true" />
            <h2>营销信息</h2>
          </div>

          <div class="moments-segmented" aria-label="输入模式">
            <label :class="{ active: inputMode === 'text' }">
              <input v-model="inputMode" data-testid="moments-input-text" type="radio" value="text">
              <span>文字描述</span>
            </label>
            <label :class="{ active: inputMode === 'photo' }">
              <input v-model="inputMode" data-testid="moments-input-photo" type="radio" value="photo">
              <span>实图宣传</span>
            </label>
          </div>

          <label class="moments-field">
            <span>店铺 / 产品</span>
            <input v-model="productName" data-testid="moments-product-name" type="text" placeholder="例如：巷口咖啡">
          </label>

          <label class="moments-field">
            <span>原始描述</span>
            <textarea v-model="brief" rows="4" placeholder="补充门店位置、产品背景、想发朋友圈的目的"></textarea>
          </label>

          <label class="moments-field">
            <span>核心卖点</span>
            <textarea v-model="sellingPoints" data-testid="moments-selling-points" rows="3" placeholder="例如：现磨咖啡、低糖甜点、步行 5 分钟可到"></textarea>
          </label>

          <div class="moments-two-columns">
            <label class="moments-field">
              <span>目标人群</span>
              <input v-model="targetAudience" data-testid="moments-target-audience" type="text" placeholder="附近上班族">
            </label>
            <label class="moments-field">
              <span>优惠活动</span>
              <input v-model="promotion" data-testid="moments-promotion" type="text" placeholder="第二杯半价">
            </label>
          </div>

          <div class="moments-two-columns">
            <label class="moments-field">
              <span>语气风格</span>
              <input v-model="tone" type="text" placeholder="自然亲切">
            </label>
            <label class="moments-field">
              <span>行动引导</span>
              <input v-model="cta" type="text" placeholder="私信预约 / 到店领取">
            </label>
          </div>
        </section>

        <section v-if="inputMode === 'photo'" class="moments-section">
          <div class="moments-section-title">
            <ImagePlus :size="18" aria-hidden="true" />
            <h2>宣传实图</h2>
          </div>
          <label class="moments-upload">
            <input data-testid="moments-reference-input" type="file" accept="image/*" multiple @change="handleReferenceChange">
            <span>{{ uploading ? '正在上传...' : '上传 1-4 张产品、门店或服务实图' }}</span>
          </label>
          <div v-if="referenceAssets.length" class="moments-reference-list">
            <article v-for="asset in referenceAssets" :key="asset.id" class="moments-reference-item">
              <img :src="asset.preview_url" :alt="asset.original_filename || '参考图'">
              <button type="button" aria-label="移除参考图" @click="removeReferenceAsset(asset.id)">
                <X :size="14" />
              </button>
            </article>
          </div>
        </section>

        <section class="moments-section">
          <div class="moments-section-title">
            <Send :size="18" aria-hidden="true" />
            <h2>输出方案</h2>
          </div>
          <div class="moments-output-grid">
            <button
              v-for="type in outputTypes"
              :key="type.key"
              type="button"
              :class="{ active: outputType === type.key }"
              @click="setOutputType(type.key)"
            >
              {{ type.label }}
            </button>
          </div>
          <label class="moments-field">
            <span>图片数量：{{ effectiveImageCount }}</span>
            <input v-model.number="imageCount" data-testid="moments-image-count" type="number" min="1" max="9">
          </label>
        </section>

        <p v-if="pageError" class="moments-error" role="alert">{{ pageError }}</p>
        <button class="moments-submit" data-testid="moments-submit" type="submit" :disabled="!canSubmit">
          <Loader2 v-if="submitting" :size="18" class="spin" />
          <Send v-else :size="18" />
          <span>{{ submitting ? '正在生成方案' : '生成朋友圈方案' }}</span>
        </button>
      </form>

      <section class="moments-preview">
        <div class="moments-preview-head">
          <div>
            <p>{{ selectedOutput.label }}</p>
            <h2>方案预览</h2>
          </div>
          <button type="button" :disabled="!plan?.moments_text" @click="copyMomentsText">
            <Copy :size="16" />
            <span>{{ copyState || '复制正文' }}</span>
          </button>
        </div>

        <article class="moments-copy-box">
          <p v-if="plan?.moments_text">{{ plan.moments_text }}</p>
          <p v-else>填写左侧信息后，这里会展示可直接复制的朋友圈正文。</p>
          <div v-if="plan?.hashtags?.length" class="moments-tags">
            <span v-for="tag in plan.hashtags" :key="tag">#{{ tag }}</span>
          </div>
        </article>

        <div v-if="plan?.safety_notes?.length" class="moments-notes">
          <CheckCircle2 :size="16" aria-hidden="true" />
          <span>{{ plan.safety_notes.join('；') }}</span>
        </div>

        <div class="moments-task-toolbar">
          <h3>图片任务</h3>
          <button type="button" :disabled="tasks.length === 0" @click="downloadAll">
            <Download :size="16" />
            <span>批量下载</span>
          </button>
        </div>

        <div class="moments-task-list">
          <article
            v-for="task in tasks"
            :key="task.generation_id"
            class="moments-task"
            :data-testid="`moments-task-${task.generation_id}`"
          >
            <div class="moments-task-media">
              <img v-if="task.preview_url" :src="task.preview_url" :alt="task.card?.caption || '宣传图'">
              <Loader2 v-else-if="task.status !== 'failed'" :size="22" class="spin" />
              <span v-else>失败</span>
            </div>
            <div class="moments-task-body">
              <strong>{{ task.card?.role || '宣传图' }} · {{ task.card?.caption || task.status }}</strong>
              <p>{{ task.card?.overlay_title || task.prompt }}</p>
              <small>{{ task.status === 'succeeded' ? '已完成' : task.status === 'failed' ? '生成失败' : '生成中' }}</small>
            </div>
            <div class="moments-task-actions">
              <button v-if="task.status === 'failed'" type="button" @click="retryTask(task)">
                <RefreshCw :size="15" />
                <span>重试</span>
              </button>
              <button v-if="task.preview_url || task.download_url" type="button" @click="downloadTask(task)">
                <Download :size="15" />
                <span>原图</span>
              </button>
              <button v-if="(outputType === 'poster_overlay' || outputType === 'nine_grid_campaign') && task.preview_url" type="button" @click="downloadPoster(task)">
                <Download :size="15" />
                <span>海报</span>
              </button>
            </div>
          </article>
          <p v-if="tasks.length === 0" class="moments-empty">图片生成任务会在方案返回后自动创建。</p>
        </div>
      </section>
    </div>
  </main>
</template>

<style scoped>
.moments-workspace {
  --moments-bg: #111a17;
  --moments-panel: #17221e;
  --moments-panel-muted: #111c18;
  --moments-input: #0d1613;
  --moments-border: rgba(148, 163, 184, 0.22);
  --moments-divider: rgba(148, 163, 184, 0.16);
  --moments-text: #e5eee9;
  --moments-heading: #f2f8f5;
  --moments-muted: #9aaba4;
  --moments-subtle: #74867f;
  --moments-accent: #28a779;
  --moments-accent-strong: #39c08e;
  --moments-accent-soft: rgba(40, 167, 121, 0.16);
  --moments-accent-soft-text: #a7f3d0;
  --moments-action-bg: #23966c;
  --moments-action-text: #ffffff;
  --moments-secondary-action-bg: rgba(40, 167, 121, 0.12);
  --moments-secondary-action-text: #a7f3d0;
  --moments-media-bg: #0f1a16;
  --moments-upload-border: rgba(148, 163, 184, 0.34);
  --moments-shadow: 0 18px 44px rgba(0, 0, 0, 0.2);
  --moments-error: #f87171;
  --moments-success-bg: rgba(40, 167, 121, 0.13);
  --moments-success-text: #b7f7d5;
  min-height: 100%;
  padding: 28px;
  background: var(--moments-bg);
  color: var(--moments-text);
}

.moments-workspace[data-theme="dark"] {
  --moments-bg: #111a17;
  --moments-panel: #17221e;
  --moments-panel-muted: #111c18;
  --moments-input: #0d1613;
  --moments-border: rgba(148, 163, 184, 0.22);
  --moments-divider: rgba(148, 163, 184, 0.16);
  --moments-text: #e5eee9;
  --moments-heading: #f2f8f5;
  --moments-muted: #9aaba4;
  --moments-subtle: #74867f;
  --moments-accent: #28a779;
  --moments-accent-strong: #39c08e;
  --moments-accent-soft: rgba(40, 167, 121, 0.16);
  --moments-accent-soft-text: #a7f3d0;
  --moments-action-bg: #23966c;
  --moments-action-text: #ffffff;
  --moments-secondary-action-bg: rgba(40, 167, 121, 0.12);
  --moments-secondary-action-text: #a7f3d0;
  --moments-media-bg: #0f1a16;
  --moments-upload-border: rgba(148, 163, 184, 0.34);
  --moments-shadow: 0 18px 44px rgba(0, 0, 0, 0.2);
  --moments-error: #f87171;
  --moments-success-bg: rgba(40, 167, 121, 0.13);
  --moments-success-text: #b7f7d5;
}

.moments-workspace[data-theme="light"] {
  --moments-bg: #f5f7f6;
  --moments-panel: #fff;
  --moments-panel-muted: #f7f9f8;
  --moments-input: #fff;
  --moments-border: #dde5e1;
  --moments-divider: #edf1ef;
  --moments-text: #17201c;
  --moments-heading: #20352d;
  --moments-muted: #66736d;
  --moments-subtle: #718078;
  --moments-accent: #1f8f6a;
  --moments-accent-strong: #116246;
  --moments-accent-soft: #e7f6ef;
  --moments-accent-soft-text: #116246;
  --moments-action-bg: #1f8f6a;
  --moments-action-text: #fff;
  --moments-secondary-action-bg: #eef5f2;
  --moments-secondary-action-text: #1e6d51;
  --moments-media-bg: #eef3f1;
  --moments-upload-border: #9fb5ac;
  --moments-shadow: 0 18px 44px rgba(22, 36, 30, 0.07);
  --moments-error: #a4382b;
  --moments-success-bg: #eff8f3;
  --moments-success-text: #315447;
}

.moments-toolbar {
  display: flex;
  align-items: center;
  gap: 20px;
  max-width: 1360px;
  margin: 0 auto 22px;
}

.moments-toolbar p,
.moments-preview-head p {
  margin: 0 0 4px;
  color: var(--moments-muted);
  font-size: 13px;
}

.moments-toolbar h1,
.moments-preview-head h2 {
  margin: 0;
  color: var(--moments-heading);
  font-size: 28px;
  letter-spacing: 0;
}

.moments-back-link {
  color: var(--moments-accent-strong);
  font-weight: 700;
  text-decoration: none;
}

.moments-shell {
  display: grid;
  grid-template-columns: minmax(340px, 460px) minmax(0, 1fr);
  gap: 22px;
  max-width: 1360px;
  margin: 0 auto;
  align-items: start;
}

.moments-form,
.moments-preview {
  background: var(--moments-panel);
  border: 1px solid var(--moments-border);
  border-radius: 8px;
  box-shadow: var(--moments-shadow);
}

.moments-form {
  padding: 20px;
}

.moments-section + .moments-section {
  margin-top: 22px;
  padding-top: 20px;
  border-top: 1px solid var(--moments-divider);
}

.moments-section-title,
.moments-preview-head,
.moments-task-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.moments-section-title {
  justify-content: flex-start;
  margin-bottom: 14px;
  color: var(--moments-heading);
}

.moments-section-title h2,
.moments-task-toolbar h3 {
  margin: 0;
  font-size: 16px;
}

.moments-segmented,
.moments-output-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  margin-bottom: 14px;
}

.moments-output-grid {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.moments-segmented label,
.moments-output-grid button {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 40px;
  border: 1px solid var(--moments-border);
  border-radius: 8px;
  background: var(--moments-panel-muted);
  color: var(--moments-muted);
  font-weight: 700;
  cursor: pointer;
}

.moments-segmented input {
  position: absolute;
  opacity: 0;
  pointer-events: none;
}

.moments-segmented label.active,
.moments-output-grid button.active {
  border-color: var(--moments-accent);
  background: var(--moments-accent-soft);
  color: var(--moments-accent-soft-text);
}

.moments-field {
  display: grid;
  gap: 7px;
  margin-top: 12px;
  color: var(--moments-muted);
  font-size: 13px;
  font-weight: 700;
}

.moments-field input,
.moments-field textarea {
  width: 100%;
  border: 1px solid var(--moments-border);
  border-radius: 8px;
  padding: 11px 12px;
  color: var(--moments-text);
  font: inherit;
  resize: vertical;
  background: var(--moments-input);
  outline: none;
}

.moments-field input::placeholder,
.moments-field textarea::placeholder {
  color: var(--moments-subtle);
}

.moments-two-columns {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.moments-upload {
  display: grid;
  place-items: center;
  min-height: 94px;
  border: 1px dashed var(--moments-upload-border);
  border-radius: 8px;
  color: var(--moments-muted);
  background: var(--moments-panel-muted);
  cursor: pointer;
}

.moments-upload input {
  position: absolute;
  opacity: 0;
  pointer-events: none;
}

.moments-reference-list,
.moments-task-list {
  display: grid;
  gap: 10px;
  margin-top: 12px;
}

.moments-reference-list {
  grid-template-columns: repeat(4, minmax(0, 1fr));
}

.moments-reference-item {
  position: relative;
  aspect-ratio: 1;
  overflow: hidden;
  border-radius: 8px;
  background: var(--moments-media-bg);
}

.moments-reference-item img,
.moments-task-media img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.moments-reference-item button {
  position: absolute;
  top: 6px;
  right: 6px;
  display: grid;
  place-items: center;
  width: 24px;
  height: 24px;
  border: 0;
  border-radius: 999px;
  background: rgba(0, 0, 0, 0.62);
  color: var(--moments-action-text);
}

.moments-submit,
.moments-preview-head button,
.moments-task-toolbar button,
.moments-task-actions button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  border: 0;
  border-radius: 8px;
  padding: 10px 14px;
  background: var(--moments-action-bg);
  color: var(--moments-action-text);
  font-weight: 800;
  cursor: pointer;
}

.moments-submit {
  width: 100%;
  margin-top: 18px;
  min-height: 48px;
}

button:disabled,
.moments-submit:disabled {
  opacity: 0.52;
  cursor: not-allowed;
}

.moments-error {
  margin: 14px 0 0;
  color: var(--moments-error);
  font-weight: 700;
}

.moments-preview {
  padding: 22px;
}

.moments-copy-box {
  min-height: 152px;
  margin-top: 18px;
  padding: 18px;
  border-radius: 8px;
  background: var(--moments-panel-muted);
  color: var(--moments-text);
  line-height: 1.75;
  white-space: pre-wrap;
}

.moments-copy-box p {
  margin: 0;
}

.moments-tags,
.moments-notes {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 14px;
}

.moments-tags span {
  color: var(--moments-accent-strong);
  font-weight: 700;
}

.moments-notes {
  align-items: center;
  padding: 12px 14px;
  border-radius: 8px;
  background: var(--moments-success-bg);
  color: var(--moments-success-text);
}

.moments-task-toolbar {
  margin-top: 22px;
}

.moments-task {
  display: grid;
  grid-template-columns: 92px minmax(0, 1fr) auto;
  gap: 14px;
  align-items: center;
  padding: 12px;
  border: 1px solid var(--moments-border);
  border-radius: 8px;
}

.moments-task-media {
  display: grid;
  place-items: center;
  width: 92px;
  aspect-ratio: 1;
  overflow: hidden;
  border-radius: 8px;
  background: var(--moments-media-bg);
  color: var(--moments-muted);
}

.moments-task-body {
  min-width: 0;
}

.moments-task-body strong,
.moments-task-body p,
.moments-task-body small {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.moments-task-body p {
  margin: 5px 0;
  color: var(--moments-muted);
}

.moments-task-body small {
  color: var(--moments-subtle);
}

.moments-task-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  justify-content: flex-end;
}

.moments-task-actions button,
.moments-task-toolbar button,
.moments-preview-head button {
  min-height: 36px;
  padding: 8px 11px;
  background: var(--moments-secondary-action-bg);
  color: var(--moments-secondary-action-text);
}

.moments-empty {
  margin: 8px 0 0;
  color: var(--moments-subtle);
}

.spin {
  animation: moments-spin 0.9s linear infinite;
}

@keyframes moments-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 980px) {
  .moments-workspace {
    padding: 18px;
  }

  .moments-shell {
    grid-template-columns: 1fr;
  }

  .moments-two-columns,
  .moments-output-grid,
  .moments-task {
    grid-template-columns: 1fr;
  }

  .moments-task-media {
    width: 100%;
  }
}
</style>
