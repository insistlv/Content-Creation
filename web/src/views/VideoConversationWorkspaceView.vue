<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Archive, Clock3, Download, FolderOpen, Heart, LoaderCircle, MessageSquarePlus, Music2, Play, RefreshCw, Search, Send, Sparkles, Video, X } from 'lucide-vue-next'
import { api } from '../api/client.js'
import { useUserTheme } from '../composables/useUserTheme.js'
import { currentUser, refreshCurrentUser } from '../stores/session.js'
import '../video-conversation-workspace.css'

const route = useRoute(); const router = useRouter(); const me = currentUser
const { theme } = useUserTheme()
const conversations = ref([]); const timeline = ref([]); const activeConversation = ref(null)
const loading = ref(true); const conversationsLoading = ref(false); const sending = ref(false); const generating = ref(false); const error = ref(''); const listError = ref('')
const prompt = ref(''); const mode = ref('video'); const search = ref(''); const range = ref('all'); const status = ref('all'); const favoriteOnly = ref(false)
const model = ref('doubao-seed-2-0-mini-260428'); const aspectRatio = ref('16:9'); const duration = ref(''); const resolution = ref('720p')
const selectedAssets = ref([]); const assetModalOpen = ref(false); const assetLoading = ref(false); const assetError = ref(''); const assetLimitMessage = ref(''); const assets = ref([]); const models = ref([]); const estimate = ref(null); const scrollRoot = ref(null)
const railOpen = ref(false); const compareTarget = ref(null); const soundtracks = ref({}); const soundtrackBusy = ref({}); const soundtrackErrors = ref({}); const soundtrackUploadInput = ref(null); const soundtrackUploadTarget = ref(null)
let pollTimer = null; let loadSerial = 0; let listSerial = 0; let estimateTimer = null; let filterTimer; let modalReturnFocus = null

const conversationId = computed(() => Number(route.query.conversation || 0))
const selectedModel = computed(() => models.value.find((item) => item.runtime_model === model.value))
const durationOptions = computed(() => (selectedModel.value?.durations || []).map(value => String(value)))
const canSubmit = computed(() => prompt.value.trim() && !sending.value && !generating.value)
const credits = computed(() => me.value?.available_credits ?? 0)
const successfulGenerations = computed(() => timeline.value.filter(item => item.type === 'generation' && item.generation.status === 'succeeded' && item.generation.preview_url).map(item => item.generation))
const latestSuccessfulGeneration = computed(() => successfulGenerations.value.at(-1) || null)

function uid(prefix) { return `${prefix}-${Date.now()}-${Math.random().toString(16).slice(2)}` }
function statusText(value) { return ({ queued: '排队中', running: '生成中', saving: '保存中', succeeded: '已完成', failed: '失败' })[value] || '准备中' }
function progressOf(item) { if (item.progress) return item.progress; return item.status === 'succeeded' ? 100 : item.status === 'running' ? 35 : item.status === 'queued' ? 5 : 0 }
function titleOf(item) { return item.title || '新对话' }
function dateText(value) { if (!value) return ''; const date = new Date(value); return `${String(date.getMonth()+1).padStart(2,'0')}-${String(date.getDate()).padStart(2,'0')} ${String(date.getHours()).padStart(2,'0')}:${String(date.getMinutes()).padStart(2,'0')}` }
function selectedModelDefaultDuration() { const values = durationOptions.value; const preferred = String(selectedModel.value?.default_duration || ''); return values.includes(preferred) ? preferred : (values[0] || '') }
function syncSupportedDuration(showHistoryNotice = false) { const values = durationOptions.value; if (!values.length || values.includes(duration.value)) return; duration.value = selectedModelDefaultDuration(); if (showHistoryNotice) error.value = '历史任务时长已不再受当前模型支持，已切换为模型默认时长' }

async function loadConversations() {
  const serial = ++listSerial; conversationsLoading.value = true; listError.value = ''
  try { const payload = await api.listVideoConversations({ q: search.value, range: range.value, status: status.value, favorite: favoriteOnly.value || undefined, page_size: 30 }); if (serial === listSerial) conversations.value = payload.items || [] }
  catch (cause) { if (serial === listSerial) listError.value = cause.message || '会话列表读取失败' }
  finally { if (serial === listSerial) conversationsLoading.value = false }
}
async function openConversation(id) {
  const serial = ++loadSerial; loading.value = true; error.value = ''
  try { const payload = await api.getVideoConversation(id); if (serial !== loadSerial) return; activeConversation.value = payload.conversation; timeline.value = payload.timeline || []; void loadTimelineSoundtracks(); await nextTick(); scrollRoot.value?.scrollTo({ top: scrollRoot.value.scrollHeight }) }
  catch (cause) { if (serial === loadSerial) error.value = cause.message || '会话读取失败' }
  finally { if (serial === loadSerial) loading.value = false }
}
async function newConversation() { prompt.value = ''; timeline.value = []; activeConversation.value = null; railOpen.value = false; if (route.query.conversation) await router.replace({ query: {} }); loading.value = false }
async function ensureConversation() { if (activeConversation.value) return activeConversation.value; const item = await api.createVideoConversation({ title: '新对话' }); activeConversation.value = item; await router.replace({ query: { conversation: item.id } }); await loadConversations(); return item }
async function selectConversation(item) { railOpen.value = false; await router.replace({ query: { conversation: item.id } }) }
async function toggleFavorite(item) { listError.value = ''; try { const updated = await api.patchVideoConversation(item.id, { is_favorite: !item.is_favorite }); Object.assign(item, updated); if (activeConversation.value?.id === item.id) Object.assign(activeConversation.value, updated) } catch (cause) { listError.value = cause.message || '收藏状态保存失败' } }

function composerContext() { return { model: model.value, aspect_ratio: aspectRatio.value, duration: duration.value, resolution: resolution.value, reference_asset_ids: selectedAssets.value.filter(a=>a.kind!=='video'&&a.kind!=='audio').map(a=>a.id), reference_video_asset_ids: selectedAssets.value.filter(a=>a.kind==='video').map(a=>a.id), reference_audio_asset_ids: selectedAssets.value.filter(a=>a.kind==='audio').map(a=>a.id) } }
async function submit() {
  if (!canSubmit.value) return; error.value = ''; const conversation = await ensureConversation()
  if (mode.value === 'chat') {
    sending.value = true; const content = prompt.value.trim(); timeline.value.push({ type: 'message', message: { id: uid('local'), role: 'user', content, status: 'pending', created_at: new Date().toISOString() } }); prompt.value = ''
    try { const payload = await api.createVideoConversationMessage(conversation.id, { content, composer_context: composerContext() }, uid('chat')); timeline.value = timeline.value.filter(item => item.message?.status !== 'pending'); timeline.value.push({ type: 'message', message: payload.message }, { type: 'message', message: payload.reply }); await loadConversations() }
    catch (cause) { error.value = cause.message || '视频策划助手暂时不可用'; timeline.value = timeline.value.map(item => item.message?.status === 'pending' ? { ...item, message: { ...item.message, status: 'failed' } } : item) }
    finally { sending.value = false }
    return
  }
  generating.value = true
  try { const body = { prompt: prompt.value.trim(), conversation_id: conversation.id, reference_mode: 'omni', output_count: 1, ...composerContext() }; const created = await api.createVideoGeneration(body, { headers: { 'Idempotency-Key': uid('video') } }); timeline.value.push({ type: 'generation', generation: { ...body, generation_record_id: created.generation_id, status: created.status, stage: created.stage, progress: created.progress, created_at: new Date().toISOString() } }); prompt.value = ''; startPolling(); await loadConversations() }
  catch (cause) { error.value = cause.message || '视频任务提交失败' }
  finally { generating.value = false }
}
async function poll() { const active = timeline.value.filter(i=>i.type==='generation'&&['queued','running'].includes(i.generation.status)); if (!active.length) { stopPolling(); return }; await Promise.all(active.map(async item => { try { const latest = await api.getVideoGeneration(item.generation.generation_record_id || item.generation.generation_id); item.generation = { ...item.generation, ...latest }; if (latest.status === 'succeeded') await refreshCurrentUser() } catch {} })) }
function startPolling() { if (!pollTimer) pollTimer = window.setInterval(poll, 2500) }
function stopPolling() { if (pollTimer) window.clearInterval(pollTimer); pollTimer = null }
function useSuggested(message) { prompt.value = message.suggested_prompt; mode.value = 'video' }
async function regenerate(item) { prompt.value = item.prompt || ''; model.value = item.runtime_model || item.model || model.value; aspectRatio.value = item.aspect_ratio || '16:9'; resolution.value = item.resolution || item.metadata?.resolution || resolution.value; duration.value = String(item.duration_seconds || item.duration || ''); await nextTick(); syncSupportedDuration(true); const ids = item.reference_asset_ids || []; if (ids.length && assets.value.length === 0) { try { const payload = await api.listReferenceAssets({ page_size: 60 }); assets.value = payload.items || [] } catch {} } selectedAssets.value = ids.map(id => assets.value.find(asset => Number(asset.id) === Number(id))).filter(Boolean); if (selectedAssets.value.length < ids.length) error.value = '部分历史素材已删除，请补充后再生成'; mode.value = 'video' }
function closeAssets() { assetModalOpen.value = false; document.body.style.overflow = ''; nextTick(() => modalReturnFocus?.focus?.()) }
async function openAssets(event) { modalReturnFocus = event?.currentTarget || document.activeElement; assetModalOpen.value = true; assetLoading.value = true; assetError.value = ''; document.body.style.overflow = 'hidden'; try { const payload = await api.listReferenceAssets({ page_size: 60 }); assets.value = payload.items || [] } catch (cause) { assetError.value = cause.message || '素材读取失败' } finally { assetLoading.value = false } }
function toggleAsset(asset) { assetLimitMessage.value = ''; const index = selectedAssets.value.findIndex(item=>item.id===asset.id); if (index>=0) selectedAssets.value.splice(index,1); else if (selectedAssets.value.length<12) selectedAssets.value.push(asset); else assetLimitMessage.value = '最多选择 12 个参考素材' }
function generationResolution(item) { return item.resolution || item.metadata?.resolution || '720p' }
function canCompare(item) { return successfulGenerations.value.length > 1 && latestSuccessfulGeneration.value?.id !== item.id }
function openCompare(item) { if (!canCompare(item)) return; modalReturnFocus = document.activeElement; compareTarget.value = item; document.body.style.overflow = 'hidden' }
function closeCompare() { compareTarget.value = null; document.body.style.overflow = ''; nextTick(() => modalReturnFocus?.focus?.()) }
async function loadTimelineSoundtracks() { const items = timeline.value.filter(entry => entry.type === 'generation' && entry.generation.status === 'succeeded' && entry.generation.work_id); await Promise.all(items.map(async entry => { try { const payload = await api.listVideoSoundtracks(entry.generation.work_id); soundtracks.value = { ...soundtracks.value, [entry.generation.work_id]: payload.items?.[0] || null } } catch {} })) }
async function generateSoundtrack(item, variation = 'smart') { if (!item.work_id || soundtrackBusy.value[item.work_id]) return; soundtrackBusy.value = { ...soundtrackBusy.value, [item.work_id]: true }; soundtrackErrors.value = { ...soundtrackErrors.value, [item.work_id]: '' }; try { const result = await api.generateVideoSoundtrack(item.work_id, { variation }); soundtracks.value = { ...soundtracks.value, [item.work_id]: result }; await refreshCurrentUser() } catch (cause) { soundtrackErrors.value = { ...soundtrackErrors.value, [item.work_id]: cause.message || '智能配乐失败' } } finally { soundtrackBusy.value = { ...soundtrackBusy.value, [item.work_id]: false } } }
function openSoundtrackUpload(item) { soundtrackUploadTarget.value = item; soundtrackUploadInput.value?.click() }
async function handleSoundtrackUpload(event) { const file = event.target.files?.[0]; const item = soundtrackUploadTarget.value; event.target.value = ''; if (!file || !item?.work_id) return; soundtrackBusy.value = { ...soundtrackBusy.value, [item.work_id]: true }; try { const result = await api.uploadVideoSoundtrack(item.work_id, file); soundtracks.value = { ...soundtracks.value, [item.work_id]: result } } catch (cause) { soundtrackErrors.value = { ...soundtrackErrors.value, [item.work_id]: cause.message || '音乐上传失败' } } finally { soundtrackBusy.value = { ...soundtrackBusy.value, [item.work_id]: false } } }
function handleEscape(event) { if (event.key !== 'Escape') return; if (assetModalOpen.value) closeAssets(); else if (compareTarget.value) closeCompare(); else railOpen.value = false }
function scheduleEstimate() { clearTimeout(estimateTimer); if (mode.value !== 'video' || !prompt.value.trim()) { estimate.value=null; return }; estimateTimer=setTimeout(async()=>{ try { estimate.value=await api.estimateVideoGeneration({ prompt: prompt.value.trim(), output_count:1, reference_mode:'omni', ...composerContext() }) } catch { estimate.value=null } },350) }

watch(() => route.query.conversation, id => { if (id) openConversation(id); else newConversation() })
watch([model, models], () => syncSupportedDuration(), { deep: true })
watch([prompt, mode, model, aspectRatio, duration, resolution, selectedAssets], scheduleEstimate, { deep: true })
watch([search, range, status, favoriteOnly], () => { clearTimeout(filterTimer); filterTimer=setTimeout(loadConversations,250) })
onMounted(async()=>{ window.addEventListener('keydown', handleEscape); try { const modelPayload = await api.listVideoModels(); models.value = modelPayload.items || modelPayload || []; if (!selectedModel.value && models.value[0]) model.value = models.value[0].runtime_model; syncSupportedDuration() } catch (cause) { error.value = cause.message || '视频模型读取失败' } await loadConversations(); if (conversationId.value) await openConversation(conversationId.value); else loading.value=false })
onBeforeUnmount(()=>{ stopPolling(); clearTimeout(estimateTimer); clearTimeout(filterTimer); window.removeEventListener('keydown', handleEscape); document.body.style.overflow = '' })
</script>

<template>
  <div class="video-chat-workspace" data-testid="video-conversation-workspace" :data-theme="theme">
    <button v-if="railOpen" class="video-rail-backdrop" type="button" aria-label="关闭会话列表" @click="railOpen=false" />
    <aside :class="['video-conversation-rail', { open: railOpen }]" aria-label="视频会话列表">
      <button class="video-new-chat" type="button" @click="newConversation"><MessageSquarePlus :size="18" /> 新对话</button>
      <div class="video-rail-tabs"><button type="button" :class="{active:!favoriteOnly}" :aria-pressed="!favoriteOnly" @click="favoriteOnly=false">最近对话</button><button type="button" :class="{active:favoriteOnly}" :aria-pressed="favoriteOnly" @click="favoriteOnly=true">收藏</button></div>
      <label class="video-rail-search"><Search :size="15"/><span class="sr-only">搜索会话</span><input v-model="search" placeholder="搜索会话" /></label>
      <p v-if="listError" class="video-rail-error">{{ listError }} <button type="button" @click="loadConversations">重试</button></p>
      <p v-else-if="conversationsLoading" class="video-rail-state">正在读取会话...</p>
      <div class="video-conversation-list">
        <div v-for="item in conversations" :key="item.id" class="video-conversation-row" :class="{active:item.id===activeConversation?.id}"><button type="button" class="video-conversation-item" @click="selectConversation(item)"><div class="video-conversation-thumb"><Video :size="22" /></div><div><strong :title="titleOf(item)">{{ titleOf(item) }}</strong><span>{{ dateText(item.last_activity_at) }}</span></div></button><button type="button" class="video-favorite-action" :aria-label="item.is_favorite?'取消收藏':'收藏会话'" @click="toggleFavorite(item)"><Heart :size="15" :fill="item.is_favorite?'currentColor':'none'" /></button></div>
      </div>
    </aside>

    <main class="video-chat-main">
      <header class="video-chat-toolbar"><button class="video-mobile-conversations" type="button" :aria-expanded="railOpen" @click="railOpen=true"><Clock3 :size="17"/>会话</button><label class="video-toolbar-search"><Search :size="17"/><span class="sr-only">搜索对话内容</span><input v-model="search" placeholder="搜索对话内容" /></label><select v-model="range" aria-label="时间范围"><option value="all">时间：全部</option><option value="today">今天</option><option value="7d">近 7 天</option><option value="30d">近 30 天</option></select><select v-model="status" aria-label="生成状态"><option value="all">状态：全部</option><option value="running">生成中</option><option value="succeeded">已完成</option><option value="failed">失败</option></select><button type="button" @click="openAssets"><FolderOpen :size="17"/>素材库</button></header>
      <section ref="scrollRoot" class="video-chat-timeline">
        <div v-if="loading" class="video-empty"><LoaderCircle class="spin"/>正在读取会话...</div>
        <div v-else-if="timeline.length===0" class="video-empty"><Sparkles :size="34"/><h2>今天想创作什么视频？</h2><p>先和策划助手聊想法，或直接切换到视频生成。</p></div>
        <template v-for="(entry,index) in timeline" :key="entry.message?.id || entry.generation?.id || index">
          <article v-if="entry.type==='message'" :class="['video-message', entry.message.role]">
            <div class="video-message-avatar"><Sparkles v-if="entry.message.role==='assistant'" :size="18"/><span v-else>我</span></div><div class="video-message-body"><small>{{ entry.message.role==='assistant'?'白霖 AI':'我' }} · {{ dateText(entry.message.created_at) }}</small><p>{{ entry.message.content }}</p><div v-if="entry.message.suggested_prompt" class="video-suggested"><strong>可直接生成的视频提示词</strong><p>{{ entry.message.suggested_prompt }}</p><button @click="useSuggested(entry.message)">使用此提示词</button></div><div class="video-quick-replies"><button v-for="reply in entry.message.quick_replies" :key="reply" @click="prompt=reply">{{ reply }}</button></div></div>
          </article>
          <article v-else class="video-generation-card">
            <div class="video-result-media"><video v-if="entry.generation.preview_url" :src="entry.generation.preview_url" controls/><div v-else class="video-result-placeholder"><LoaderCircle v-if="['queued','running'].includes(entry.generation.status)" class="spin"/><Play v-else :size="34"/><span>{{ statusText(entry.generation.status) }} {{ progressOf(entry.generation) }}%</span></div></div>
            <div class="video-result-summary"><span class="video-status-pill">{{ statusText(entry.generation.status) }}</span><h3>生成摘要</h3><p>{{ entry.generation.prompt }}</p><dl><dt>模型</dt><dd>{{ entry.generation.model_name || selectedModel?.name || model }}</dd><dt>分辨率</dt><dd>{{ entry.generation.aspect_ratio }} · {{ generationResolution(entry.generation) }}</dd><dt>时长</dt><dd>{{ entry.generation.duration_seconds || duration }} 秒</dd></dl></div>
            <div class="video-result-actions"><a v-if="entry.generation.download_url" :href="entry.generation.download_url" download><Download :size="16"/>下载视频</a><button v-if="entry.generation.status==='succeeded'&&entry.generation.work_id" type="button" :disabled="soundtrackBusy[entry.generation.work_id]" @click="generateSoundtrack(entry.generation, soundtracks[entry.generation.work_id]?'replace':'smart')"><Music2 :size="16"/>{{ soundtrackBusy[entry.generation.work_id]?'处理中...':soundtracks[entry.generation.work_id]?'换一首':'智能配乐' }}</button><button v-if="entry.generation.status==='succeeded'&&entry.generation.work_id" type="button" :disabled="soundtrackBusy[entry.generation.work_id]" @click="openSoundtrackUpload(entry.generation)">上传音乐</button><button type="button" @click="regenerate(entry.generation)"><RefreshCw :size="16"/>再次生成</button><button type="button" :disabled="!canCompare(entry.generation)" :title="canCompare(entry.generation)?'与最新成功版本对比':'至少需要两个成功版本'" @click="openCompare(entry.generation)"><Archive :size="16"/>版本对比</button></div>
            <div v-if="soundtracks[entry.generation.work_id]" class="video-soundtrack-inline"><audio :src="soundtracks[entry.generation.work_id].audio_url" controls/><a v-if="soundtracks[entry.generation.work_id].download_url" :href="soundtracks[entry.generation.work_id].download_url" download>下载音乐</a></div><p v-if="soundtrackErrors[entry.generation.work_id]" class="video-action-error">{{ soundtrackErrors[entry.generation.work_id] }}</p>
          </article>
        </template>
      </section>

      <section class="video-unified-composer">
        <div class="video-composer-assets"><span>参考素材（可选）</span><button v-for="asset in selectedAssets" :key="asset.id" class="video-asset-chip" type="button" @click="toggleAsset(asset)">{{ asset.original_filename || asset.display_name }} <X :size="13"/></button><button class="video-add-asset" type="button" @click="openAssets">+ 添加</button></div><p v-if="assetLimitMessage" class="video-action-error">{{ assetLimitMessage }}</p>
        <textarea v-model="prompt" aria-label="视频创意描述" maxlength="2000" :placeholder="mode==='chat'?'和视频策划助手聊聊你的创意...':'描述镜头、主体、光线、运动与氛围...'" @keydown.ctrl.enter.prevent="submit" />
        <div class="video-chat-composer-controls">
          <div class="video-chat-composer-primary">
            <div class="video-mode-switch"><button type="button" :class="{active:mode==='chat'}" :aria-pressed="mode==='chat'" @click="mode='chat'">创意对话</button><button type="button" :class="{active:mode==='video'}" :aria-pressed="mode==='video'" @click="mode='video'">视频生成</button></div>
            <div class="video-chat-composer-actions"><span class="video-cost">{{ mode==='chat'?'免费助手':`预计 ${estimate?.required_credits || '—'} 点 · 余额 ${credits}` }}</span><button class="video-send" type="button" aria-label="发送" :disabled="!canSubmit" @click="submit"><Send :size="18"/></button></div>
          </div>
          <div v-if="mode==='video'" class="video-chat-composer-params">
            <label class="video-chat-composer-field"><span>模型</span><select v-model="model" aria-label="视频模型"><option v-for="item in models" :key="item.runtime_model" :value="item.runtime_model">{{ item.name }}</option></select></label>
            <label class="video-chat-composer-field"><span>画面比例</span><select v-model="aspectRatio" aria-label="画面比例"><option>16:9</option><option>9:16</option></select></label>
            <label class="video-chat-composer-field"><span>分辨率</span><select v-model="resolution" aria-label="分辨率"><option>720p</option><option>1080p</option></select></label>
            <label class="video-chat-composer-field"><span>时长</span><select v-model="duration" aria-label="视频时长"><option v-for="value in durationOptions" :key="value" :value="value">{{ value === '-1' ? '自动时长' : `${value} 秒` }}</option></select></label>
          </div>
        </div>
        <p v-if="error" class="video-composer-error">{{ error }}</p>
      </section>
    </main>

    <input ref="soundtrackUploadInput" class="video-hidden-input" type="file" accept="audio/mpeg,audio/wav,audio/x-wav,audio/mp4,audio/aac,audio/ogg" @change="handleSoundtrackUpload" />
    <Teleport to="body"><div v-if="assetModalOpen" class="video-asset-modal" :data-theme="theme" role="dialog" aria-modal="true" aria-labelledby="video-asset-title" @click.self="closeAssets"><div><header><h3 id="video-asset-title">选择参考素材</h3><button type="button" aria-label="关闭素材库" @click="closeAssets"><X/></button></header><section><p v-if="assetLoading">正在读取素材...</p><p v-else-if="assetError" class="video-action-error">{{ assetError }} <button type="button" @click="openAssets">重试</button></p><p v-else-if="assets.length===0">暂无可用素材</p><template v-else><button v-for="asset in assets" :key="asset.id" type="button" :class="{selected:selectedAssets.some(item=>item.id===asset.id)}" @click="toggleAsset(asset)"><img v-if="asset.preview_url && !asset.mime_type?.startsWith('audio/')" :src="asset.preview_url" :alt="asset.original_filename || asset.display_name || '参考素材'"/><Music2 v-else/><span>{{ asset.original_filename || asset.display_name }}</span></button></template></section></div></div></Teleport>
    <Teleport to="body"><div v-if="compareTarget" class="video-asset-modal" :data-theme="theme" role="dialog" aria-modal="true" aria-labelledby="video-compare-title" @click.self="closeCompare"><div class="video-compare-dialog"><header><h3 id="video-compare-title">版本对比</h3><button type="button" aria-label="关闭版本对比" @click="closeCompare"><X/></button></header><section class="video-compare-grid"><article><strong>最新版本</strong><video :src="latestSuccessfulGeneration.preview_url" controls/></article><article><strong>所选版本</strong><video :src="compareTarget.preview_url" controls/></article></section></div></div></Teleport>
  </div>
</template>
