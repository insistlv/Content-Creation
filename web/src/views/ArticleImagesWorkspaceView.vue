<script setup>
import { computed, onBeforeUnmount, ref } from 'vue'
import { ArrowLeft, Download, ImagePlus, Loader2, RefreshCw, Sparkles, Upload, Wand2, X } from 'lucide-vue-next'
import { RouterLink } from 'vue-router'
import { api } from '../api/client.js'

const referenceAssetUploadMaxBytes = 50 * 1024 * 1024
const maxReferenceAssets = 4

const title = ref('')
const body = ref('')
const articleType = ref('知识科普')
const audience = ref('公众号读者')
const visualStyle = ref('清爽专业')
const imageCount = ref(4)
const includeCover = ref(true)
const referenceAssets = ref([])
const uploadingReference = ref(false)
const submitting = ref(false)
const plan = ref(null)
const generatedTasks = ref([])
const pageError = ref('')
const successMessage = ref('')
const pollTimers = new Map()

const articleTypes = ['知识科普', '品牌故事', '活动预告', '教程攻略', '产品种草', '行业观点']
const visualStyles = ['清爽专业', '杂志封面', '温暖治愈', '商务科技', '国风雅致', '极简插画']

const trimmedBody = computed(() => body.value.trim())
const selectedReferenceIds = computed(() => referenceAssets.value.map((asset) => asset.id).filter(Boolean))
const normalizedImageCount = computed(() => {
  const value = Number(imageCount.value)
  if (!Number.isFinite(value)) return 4
  return Math.min(9, Math.max(1, Math.trunc(value)))
})
const canSubmit = computed(() => trimmedBody.value.length > 0 && !submitting.value && !uploadingReference.value)
const creditHint = computed(() => `预计生成 ${normalizedImageCount.value} 张，实际点数按图片生成规则结算。`)

onBeforeUnmount(() => {
  pollTimers.forEach((timer) => clearTimeout(timer))
  pollTimers.clear()
})

function buildPlanPayload() {
  return {
    title: title.value.trim(),
    body: trimmedBody.value,
    article_type: articleType.value,
    audience: audience.value.trim() || '公众号读者',
    style: visualStyle.value,
    image_count: normalizedImageCount.value,
    include_cover: includeCover.value,
    reference_asset_ids: selectedReferenceIds.value
  }
}

async function submitPlan() {
  if (!canSubmit.value) return
  pageError.value = ''
  successMessage.value = ''
  submitting.value = true
  try {
    const nextPlan = await api.planArticleImages(buildPlanPayload())
    plan.value = normalizePlan(nextPlan)
    generatedTasks.value = await createBatchTasks(plan.value.image_cards)
    successMessage.value = '已创建配图生成任务，可单张编辑后重试。'
  } catch (error) {
    pageError.value = error?.message || '公众号配图方案生成失败，请稍后重试'
  } finally {
    submitting.value = false
  }
}

function normalizePlan(nextPlan) {
  return {
    article_summary: nextPlan?.article_summary || '',
    safety_notes: Array.isArray(nextPlan?.safety_notes) ? nextPlan.safety_notes : [],
    image_cards: Array.isArray(nextPlan?.image_cards)
      ? nextPlan.image_cards.map((card, index) => ({
        slot: Number(card.slot) || index + 1,
        role: card.role || (index === 0 ? '封面图' : '段落配图'),
        placement: card.placement || (index === 0 ? '文章开头' : `第 ${index + 1} 个小标题后`),
        caption: card.caption || card.overlay_title || '公众号配图',
        visual_prompt: card.visual_prompt || '',
        aspect_ratio: normalizeAspectRatio(card.aspect_ratio),
        overlay_title: card.overlay_title || card.caption || title.value.trim() || '公众号配图',
        layout: card.layout || 'clean_overlay'
      }))
      : []
  }
}

async function createBatchTasks(cards) {
  const batch = {
    id: `article-images-${Date.now()}`,
    total: cards.length
  }
  const tasks = await Promise.all(cards.map((card, index) => createOneImageTask(card, {
    batchId: batch.id,
    batchIndex: index + 1,
    batchTotal: batch.total
  })))
  return tasks
}

async function createOneImageTask(card, batch) {
  const payload = {
    prompt: buildGenerationPrompt(card),
    aspect_ratio: normalizeAspectRatio(card.aspect_ratio),
    tool_mode: 'generate',
    batch_id: batch.batchId,
    batch_index: batch.batchIndex,
    batch_total: batch.batchTotal
  }
  const referenceIds = selectedReferenceIds.value
  if (referenceIds.length > 0) {
    payload.reference_asset_ids = referenceIds
    payload.reference_weight = 80
    payload.reference_intent = 'compose'
  }
  const response = await api.createImageGeneration(payload)
  const task = normalizeTask(response, card, payload)
  scheduleTaskPoll(task)
  return task
}

function normalizeTask(response, card, payload) {
  return {
    id: response?.generation_id || `${card.slot}-${Date.now()}`,
    card,
    status: response?.status || response?.stage || 'queued',
    stage: response?.stage || response?.status || 'queued',
    prompt: response?.prompt || payload.prompt,
    preview_url: response?.preview_url || response?.previewUrl || '',
    download_url: response?.download_url || response?.downloadUrl || '',
    parameters: response?.parameters || payload
  }
}

function buildGenerationPrompt(card) {
  const parts = [
    card.visual_prompt,
    `公众号文章配图，文章类型：${articleType.value}`,
    `目标读者：${audience.value || '公众号读者'}`,
    `视觉风格：${visualStyle.value}`,
    '画面干净，可用于前端后期叠加标题',
    '不要生成中文文字、标题、二维码、水印或品牌 logo'
  ]
  if (title.value.trim()) {
    parts.push(`文章主题：${title.value.trim()}`)
  }
  return parts.filter(Boolean).join('，')
}

function normalizeAspectRatio(value) {
  return ['16:9', '3:4', '1:1'].includes(value) ? value : '16:9'
}

function scheduleTaskPoll(task) {
  if (!task?.id || !['queued', 'running', 'processing', 'pending'].includes(task.status)) return
  const timer = window.setTimeout(async () => {
    pollTimers.delete(task.id)
    try {
      const next = await api.getImageGeneration(task.id)
      updateTask(task.id, normalizeTask(next, task.card, task.parameters))
    } catch {
      scheduleTaskPoll(task)
    }
  }, 3500)
  pollTimers.set(task.id, timer)
}

function updateTask(id, nextTask) {
  generatedTasks.value = generatedTasks.value.map((task) => (task.id === id ? nextTask : task))
  scheduleTaskPoll(nextTask)
}

async function retryCard(card) {
  if (!card || submitting.value) return
  pageError.value = ''
  submitting.value = true
  try {
    const nextTask = await createOneImageTask(card, {
      batchId: `article-images-retry-${Date.now()}`,
      batchIndex: 1,
      batchTotal: 1
    })
    generatedTasks.value = generatedTasks.value.map((task) => (task.card.slot === card.slot ? nextTask : task))
    successMessage.value = `已重试第 ${card.slot} 张配图。`
  } catch (error) {
    pageError.value = error?.message || '重试失败，请稍后再试'
  } finally {
    submitting.value = false
  }
}

async function handleReferenceFiles(event) {
  const files = Array.from(event.target?.files || [])
  event.target.value = ''
  if (files.length === 0 || uploadingReference.value) return
  pageError.value = ''
  const available = maxReferenceAssets - referenceAssets.value.length
  if (available <= 0) {
    pageError.value = `参考图最多上传 ${maxReferenceAssets} 张`
    return
  }
  uploadingReference.value = true
  try {
    for (const file of files.slice(0, available)) {
      if (file.size > referenceAssetUploadMaxBytes) {
        throw new Error('单张参考图不能超过 50MB')
      }
      const uploaded = await api.uploadReferenceAsset(file)
      referenceAssets.value = [...referenceAssets.value, uploaded]
    }
  } catch (error) {
    pageError.value = error?.message || '参考图上传失败'
  } finally {
    uploadingReference.value = false
  }
}

function removeReference(asset) {
  referenceAssets.value = referenceAssets.value.filter((item) => item.id !== asset.id)
}

function taskStatusText(task) {
  if (task.status === 'succeeded') return '已完成'
  if (task.status === 'failed') return '失败'
  return '生成中'
}

function aspectSize(aspectRatio) {
  switch (aspectRatio) {
    case '1:1':
      return { width: 1080, height: 1080 }
    case '3:4':
      return { width: 900, height: 1200 }
    default:
      return { width: 1200, height: 675 }
  }
}

async function downloadDesignedImage(task) {
  if (!task?.preview_url) return
  try {
    const size = aspectSize(task.card.aspect_ratio)
    const image = await loadImage(task.preview_url)
    const canvas = document.createElement('canvas')
    canvas.width = size.width
    canvas.height = size.height
    const context = canvas.getContext('2d')
    if (!context) return
    drawCoverImage(context, image, size.width, size.height)
    drawOverlay(context, task.card, size.width, size.height)
    const link = document.createElement('a')
    link.download = `article-image-${task.card.slot}.png`
    link.href = canvas.toDataURL('image/png')
    link.click()
  } catch {
    pageError.value = '排版图下载失败，请先下载原图'
  }
}

function loadImage(src) {
  return new Promise((resolve, reject) => {
    const image = new Image()
    image.crossOrigin = 'anonymous'
    image.onload = () => resolve(image)
    image.onerror = reject
    image.src = src
  })
}

function drawCoverImage(context, image, width, height) {
  const scale = Math.max(width / image.width, height / image.height)
  const drawWidth = image.width * scale
  const drawHeight = image.height * scale
  context.drawImage(image, (width - drawWidth) / 2, (height - drawHeight) / 2, drawWidth, drawHeight)
}

function drawOverlay(context, card, width, height) {
  const gradient = context.createLinearGradient(0, height * 0.48, 0, height)
  gradient.addColorStop(0, 'rgba(5, 18, 32, 0)')
  gradient.addColorStop(1, 'rgba(5, 18, 32, 0.72)')
  context.fillStyle = gradient
  context.fillRect(0, 0, width, height)
  const padding = Math.round(width * 0.07)
  context.fillStyle = '#ffffff'
  context.font = `700 ${Math.round(width * 0.052)}px system-ui, sans-serif`
  wrapText(context, card.overlay_title || card.caption || '公众号配图', padding, height - padding * 1.6, width - padding * 2, Math.round(width * 0.064), 2)
  context.font = `500 ${Math.round(width * 0.026)}px system-ui, sans-serif`
  context.fillStyle = 'rgba(255,255,255,.82)'
  context.fillText(card.placement || card.role, padding, height - padding * 0.72)
}

function wrapText(context, text, x, y, maxWidth, lineHeight, maxLines) {
  const chars = String(text || '').split('')
  let line = ''
  let lines = []
  for (const char of chars) {
    const testLine = line + char
    if (context.measureText(testLine).width > maxWidth && line) {
      lines.push(line)
      line = char
    } else {
      line = testLine
    }
  }
  if (line) lines.push(line)
  lines = lines.slice(0, maxLines)
  lines.forEach((item, index) => context.fillText(item, x, y - (lines.length - 1 - index) * lineHeight))
}
</script>

<template>
  <main class="article-images-workspace">
    <section class="article-images-shell">
      <header class="article-images-header">
        <RouterLink class="article-back-link" to="/workspace">
          <ArrowLeft :size="16" />
          返回图像工坊
        </RouterLink>
        <div>
          <span class="article-kicker">WECHAT ARTICLE IMAGES</span>
          <h1>公众号文章配图</h1>
          <p>粘贴标题和正文，自动拆解封面图、段落配图、金句卡片与流程图，批量创建图片生成任务。</p>
        </div>
      </header>

      <div class="article-images-grid">
        <form class="article-plan-panel" data-testid="article-images-plan-form" @submit.prevent="submitPlan">
          <label class="article-field">
            <span>文章标题</span>
            <input v-model="title" data-testid="article-images-title" type="text" placeholder="例如：活动增长方法论" maxlength="80">
          </label>

          <label class="article-field">
            <span>文章正文</span>
            <textarea
              v-model="body"
              data-testid="article-images-body"
              rows="10"
              maxlength="12000"
              placeholder="粘贴公众号正文，MVP 暂不解析链接。"
            />
            <small>{{ body.length }}/12000</small>
          </label>

          <div class="article-form-row">
            <label class="article-field">
              <span>文章类型</span>
              <select v-model="articleType">
                <option v-for="item in articleTypes" :key="item" :value="item">{{ item }}</option>
              </select>
            </label>
            <label class="article-field">
              <span>视觉风格</span>
              <select v-model="visualStyle">
                <option v-for="item in visualStyles" :key="item" :value="item">{{ item }}</option>
              </select>
            </label>
          </div>

          <label class="article-field">
            <span>目标读者</span>
            <input v-model="audience" type="text" placeholder="例如：品牌运营、新手妈妈、门店老板">
          </label>

          <div class="article-form-row article-form-row--compact">
            <label class="article-field">
              <span>图片数量</span>
              <input v-model.number="imageCount" data-testid="article-images-image-count" type="number" min="1" max="9">
            </label>
            <label class="article-checkbox">
              <input v-model="includeCover" type="checkbox">
              <span>生成封面图</span>
            </label>
          </div>

          <section class="article-reference-panel">
            <div class="article-section-title">
              <div>
                <strong>参考图</strong>
                <small>可上传品牌、产品或人物参考图，最多 {{ maxReferenceAssets }} 张</small>
              </div>
              <span>{{ referenceAssets.length }}/{{ maxReferenceAssets }}</span>
            </div>
            <label class="article-reference-upload">
              <input
                data-testid="article-images-reference-input"
                type="file"
                accept="image/jpeg,image/png,image/webp"
                multiple
                @change="handleReferenceFiles"
              >
              <Upload v-if="!uploadingReference" :size="20" />
              <Loader2 v-else :size="20" class="spin" />
              <span>{{ uploadingReference ? '上传中' : '点击上传参考图' }}</span>
              <small>JPG/PNG/WEBP，单张小于 50MB</small>
            </label>
            <div v-if="referenceAssets.length" class="article-reference-list">
              <span v-for="asset in referenceAssets" :key="asset.id" class="article-reference-chip">
                <img :src="asset.preview_url" :alt="asset.original_filename || '参考图'">
                <em>{{ asset.original_filename || asset.display_name || '参考图' }}</em>
                <button type="button" aria-label="移除参考图" @click="removeReference(asset)">
                  <X :size="14" />
                </button>
              </span>
            </div>
          </section>

          <div class="article-cost-bar">
            <span>{{ creditHint }}</span>
            <small>生成前会先展示 AI 拆出的配图清单。</small>
          </div>

          <button class="article-submit" data-testid="article-images-submit" type="submit" :disabled="!canSubmit">
            <Loader2 v-if="submitting" :size="18" class="spin" />
            <Sparkles v-else :size="18" />
            {{ submitting ? '正在规划并创建任务' : '生成配图方案' }}
          </button>
          <p v-if="pageError" class="article-error">{{ pageError }}</p>
          <p v-if="successMessage" class="article-success">{{ successMessage }}</p>
        </form>

        <section class="article-result-panel">
          <div v-if="!plan" class="article-empty-state">
            <ImagePlus :size="42" />
            <strong>等待文章内容</strong>
            <span>提交后这里会显示配图清单、生成进度和单张操作。</span>
          </div>

          <template v-else>
            <div class="article-summary">
              <span>AI 拆解摘要</span>
              <p>{{ plan.article_summary }}</p>
            </div>
            <div v-if="plan.safety_notes.length" class="article-safety">
              <strong>安全提示</strong>
              <span v-for="note in plan.safety_notes" :key="note">{{ note }}</span>
            </div>

            <article
              v-for="task in generatedTasks"
              :key="task.id"
              class="article-task-card"
              :data-testid="`article-images-task-${task.id}`"
            >
              <div class="article-task-preview">
                <img v-if="task.preview_url" :src="task.preview_url" :alt="task.card.caption">
                <div v-else>
                  <Wand2 :size="26" />
                  <span>{{ taskStatusText(task) }}</span>
                </div>
              </div>
              <div class="article-task-body" :data-testid="`article-images-card-${task.card.slot}`">
                <div class="article-task-head">
                  <span>{{ task.card.role }}</span>
                  <strong>{{ task.card.caption }}</strong>
                  <em>{{ task.card.placement }} · {{ task.card.aspect_ratio }}</em>
                </div>
                <label class="article-field article-field--compact">
                  <span>visual prompt</span>
                  <textarea
                    v-model="task.card.visual_prompt"
                    :data-testid="`article-images-card-prompt-${task.card.slot}`"
                    rows="3"
                  />
                </label>
                <div class="article-task-actions">
                  <button
                    type="button"
                    :data-testid="`article-images-retry-${task.card.slot}`"
                    @click="retryCard(task.card)"
                  >
                    <RefreshCw :size="15" />
                    重试
                  </button>
                  <a
                    v-if="task.download_url"
                    :href="task.download_url"
                    :data-testid="`article-images-download-original-${task.card.slot}`"
                    download
                  >
                    <Download :size="15" />
                    下载原图
                  </a>
                  <button
                    v-if="task.preview_url"
                    type="button"
                    :data-testid="`article-images-download-designed-${task.card.slot}`"
                    @click="downloadDesignedImage(task)"
                  >
                    <Download :size="15" />
                    下载排版图
                  </button>
                </div>
              </div>
            </article>
          </template>
        </section>
      </div>
    </section>
  </main>
</template>

<style scoped>
.article-images-workspace {
  min-height: 100vh;
  background:
    radial-gradient(circle at 8% 5%, rgba(34, 211, 238, .14), transparent 32%),
    linear-gradient(135deg, #f7fbff 0%, #ffffff 48%, #f2f7fb 100%);
  color: #102033;
  padding: 32px;
}

.article-images-shell {
  width: min(1480px, 100%);
  margin: 0 auto;
}

.article-images-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 24px;
  margin-bottom: 24px;
}

.article-back-link {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 10px 14px;
  border: 1px solid rgba(135, 160, 184, .35);
  border-radius: 999px;
  color: #216184;
  text-decoration: none;
  background: rgba(255, 255, 255, .82);
  box-shadow: 0 12px 32px rgba(39, 72, 104, .08);
}

.article-kicker {
  display: block;
  font-size: 12px;
  letter-spacing: .08em;
  color: #62809b;
  margin-bottom: 6px;
}

.article-images-header h1 {
  margin: 0;
  font-size: 34px;
  line-height: 1.12;
}

.article-images-header p {
  margin: 10px 0 0;
  color: #5f7186;
}

.article-images-grid {
  display: grid;
  grid-template-columns: minmax(380px, 480px) minmax(0, 1fr);
  gap: 24px;
  align-items: start;
}

.article-plan-panel,
.article-result-panel {
  border: 1px solid rgba(144, 163, 184, .26);
  border-radius: 24px;
  background: rgba(255, 255, 255, .86);
  box-shadow: 0 24px 70px rgba(39, 72, 104, .12);
}

.article-plan-panel {
  padding: 24px;
  display: grid;
  gap: 16px;
}

.article-result-panel {
  min-height: 720px;
  padding: 24px;
}

.article-field {
  display: grid;
  gap: 8px;
  font-size: 14px;
  font-weight: 700;
  color: #29384a;
}

.article-field input,
.article-field textarea,
.article-field select {
  width: 100%;
  border: 1px solid rgba(135, 160, 184, .34);
  border-radius: 16px;
  background: rgba(248, 251, 255, .94);
  color: #122033;
  padding: 13px 14px;
  font: inherit;
  resize: vertical;
  outline: none;
}

.article-field input:focus,
.article-field textarea:focus,
.article-field select:focus {
  border-color: rgba(14, 165, 233, .64);
  box-shadow: 0 0 0 4px rgba(14, 165, 233, .12);
}

.article-field small,
.article-section-title small,
.article-cost-bar small {
  color: #7890a8;
  font-weight: 500;
}

.article-form-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
}

.article-form-row--compact {
  grid-template-columns: 1fr auto;
  align-items: end;
}

.article-checkbox {
  min-height: 50px;
  display: inline-flex;
  align-items: center;
  gap: 10px;
  padding: 0 16px;
  border-radius: 16px;
  border: 1px solid rgba(135, 160, 184, .34);
  background: rgba(248, 251, 255, .94);
  font-weight: 700;
}

.article-reference-panel {
  display: grid;
  gap: 12px;
  padding: 16px;
  border-radius: 20px;
  border: 1px solid rgba(14, 165, 233, .18);
  background: rgba(239, 249, 255, .62);
}

.article-section-title {
  display: flex;
  justify-content: space-between;
  gap: 16px;
}

.article-section-title div {
  display: grid;
  gap: 4px;
}

.article-section-title span {
  color: #0e83b5;
  font-weight: 800;
}

.article-reference-upload {
  min-height: 126px;
  display: grid;
  place-items: center;
  gap: 6px;
  border: 1px dashed rgba(14, 165, 233, .45);
  border-radius: 18px;
  background: rgba(255, 255, 255, .74);
  cursor: pointer;
  color: #236486;
  text-align: center;
}

.article-reference-upload input {
  position: absolute;
  width: 1px;
  height: 1px;
  opacity: 0;
  pointer-events: none;
}

.article-reference-upload span {
  font-weight: 800;
}

.article-reference-upload small {
  color: #7990a6;
}

.article-reference-list {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.article-reference-chip {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  max-width: 100%;
  padding: 6px 8px;
  border-radius: 14px;
  background: #ffffff;
  border: 1px solid rgba(135, 160, 184, .24);
}

.article-reference-chip img {
  width: 34px;
  height: 34px;
  border-radius: 10px;
  object-fit: cover;
}

.article-reference-chip em {
  max-width: 210px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-style: normal;
  color: #40536a;
}

.article-reference-chip button,
.article-task-actions button,
.article-task-actions a {
  border: 0;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  text-decoration: none;
  color: #135b7d;
  background: rgba(227, 245, 255, .9);
  border-radius: 999px;
  padding: 9px 12px;
  font-weight: 800;
}

.article-cost-bar {
  display: grid;
  gap: 3px;
  padding: 13px 14px;
  border-radius: 16px;
  background: rgba(246, 250, 255, .95);
  border: 1px solid rgba(135, 160, 184, .22);
}

.article-cost-bar span {
  font-weight: 800;
}

.article-submit {
  min-height: 54px;
  border: 0;
  border-radius: 18px;
  background: linear-gradient(135deg, #0ea5e9, #22c7d8);
  color: #ffffff;
  font-weight: 900;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 9px;
  cursor: pointer;
  box-shadow: 0 18px 34px rgba(14, 165, 233, .24);
}

.article-submit:disabled {
  cursor: not-allowed;
  background: #dbe6ef;
  color: #8ba0b4;
  box-shadow: none;
}

.article-error,
.article-success {
  margin: 0;
  padding: 12px 14px;
  border-radius: 14px;
  font-weight: 700;
}

.article-error {
  background: #fff0f1;
  color: #c03945;
}

.article-success {
  background: #ecfdf5;
  color: #047857;
}

.article-empty-state {
  min-height: 640px;
  display: grid;
  place-items: center;
  align-content: center;
  gap: 12px;
  color: #7690a8;
  text-align: center;
}

.article-empty-state strong {
  color: #18283a;
  font-size: 18px;
}

.article-summary,
.article-safety {
  border-radius: 18px;
  padding: 16px;
  background: rgba(248, 251, 255, .94);
  border: 1px solid rgba(135, 160, 184, .22);
  margin-bottom: 16px;
}

.article-summary span,
.article-safety strong {
  display: block;
  color: #0e83b5;
  font-weight: 900;
  margin-bottom: 7px;
}

.article-summary p {
  margin: 0;
  color: #40536a;
  line-height: 1.7;
}

.article-safety {
  display: grid;
  gap: 8px;
}

.article-safety span {
  color: #6b7d91;
}

.article-task-card {
  display: grid;
  grid-template-columns: minmax(180px, 260px) minmax(0, 1fr);
  gap: 18px;
  padding: 16px;
  margin-bottom: 16px;
  border-radius: 20px;
  border: 1px solid rgba(135, 160, 184, .22);
  background: rgba(255, 255, 255, .88);
}

.article-task-preview {
  min-height: 180px;
  border-radius: 18px;
  overflow: hidden;
  background: linear-gradient(135deg, rgba(14, 165, 233, .08), rgba(34, 197, 94, .08));
  display: grid;
  place-items: center;
  color: #5f7893;
}

.article-task-preview img {
  width: 100%;
  height: 100%;
  min-height: 180px;
  object-fit: cover;
}

.article-task-preview div {
  display: grid;
  place-items: center;
  gap: 8px;
}

.article-task-body {
  display: grid;
  gap: 12px;
}

.article-task-head {
  display: grid;
  gap: 4px;
}

.article-task-head span {
  color: #0e83b5;
  font-weight: 900;
  font-size: 13px;
}

.article-task-head strong {
  font-size: 18px;
}

.article-task-head em {
  color: #70879f;
  font-style: normal;
}

.article-field--compact textarea {
  min-height: 82px;
}

.article-task-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.spin {
  animation: article-spin 1s linear infinite;
}

@keyframes article-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 1080px) {
  .article-images-workspace {
    padding: 20px;
  }

  .article-images-grid,
  .article-task-card {
    grid-template-columns: 1fr;
  }

  .article-result-panel {
    min-height: 420px;
  }
}

@media (max-width: 720px) {
  .article-images-header {
    display: grid;
  }

  .article-images-header h1 {
    font-size: 28px;
  }

  .article-form-row,
  .article-form-row--compact {
    grid-template-columns: 1fr;
  }

  .article-checkbox {
    justify-content: flex-start;
  }

  .article-plan-panel,
  .article-result-panel {
    padding: 18px;
    border-radius: 20px;
  }
}
</style>
