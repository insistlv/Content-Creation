<script setup>
import { computed, reactive, ref, watch } from 'vue'
const props = defineProps({ skus: { type: Array, default: () => [] }, defaultSkuId: [Number, String], config: { type: Object, default: () => ({ version: 0, dimensions: [] }) }, preview: { type: Object, default: null }, busy: Boolean })
const emit = defineEmits(['preview', 'apply', 'edit', 'patch', 'set-default'])
const mode = ref('single'), dimensions = ref([]), error = ref(''), codes = reactive({}), previewReady = ref(false)
let pendingSnapshot = null, frozenSnapshot = null
watch(() => props.skus, list => list.forEach(item => { codes[item.id] = item.code }), { immediate: true })
watch(() => props.config, config => {
  const active = config?.dimensions?.filter(d => d.status !== 'disabled') || []
  mode.value = active.length ? 'multiple' : 'single'
  dimensions.value = active.map(d => ({ name: d.name, values: (config.values || []).filter(v => v.dimension_id === d.id && v.status !== 'disabled').map(v => v.name).join(', ') }))
  previewReady.value = false; pendingSnapshot = null; frozenSnapshot = null
}, { immediate: true })
watch(() => props.preview, preview => {
  if (preview && pendingSnapshot) { frozenSnapshot = JSON.parse(JSON.stringify(pendingSnapshot)); previewReady.value = true }
  else if (!preview) { previewReady.value = false; frozenSnapshot = null }
})
const suggestions = ['颜色', '尺寸', '容量', '款式', '包装', '型号', '适用对象']
const displayCode = sku => sku.code === 'DEFAULT' ? '默认规格' : sku.code
const matrixInput = () => ({ expected_version: props.config?.version || 0, dimensions: dimensions.value.map((dimension, index) => ({ name: dimension.name.trim(), values: dimension.values.split(/[,，\n]/).map(v => v.trim()).filter(Boolean).map(name => ({ name })), sort_order: index })) })
function invalidatePreview() { previewReady.value = false; frozenSnapshot = null; emit('edit') }
function addDimension() { if (dimensions.value.length < 3) { dimensions.value.push({ name: suggestions[dimensions.value.length] || '', values: '' }); invalidatePreview() } }
function removeDimension(index) { dimensions.value.splice(index, 1); invalidatePreview() }
function validate(input) { error.value = ''; if (!input.dimensions.length || input.dimensions.some(d => !d.name || !d.values.length)) return (error.value = '请填写规格维度和值'); if (input.dimensions.some(d => d.values.length > 20)) return (error.value = '每个维度最多 20 个值'); if (input.dimensions.reduce((n, d) => n * d.values.length, 1) > 100) return (error.value = '规格组合最多 100 个'); return true }
function requestPreview(input = matrixInput()) { if (input.dimensions.length && validate(input) !== true) return; pendingSnapshot = JSON.parse(JSON.stringify(input)); previewReady.value = false; emit('preview', pendingSnapshot) }
function switchSingle() { mode.value = 'single'; dimensions.value = []; error.value = ''; requestPreview({ expected_version: props.config?.version || 0, dimensions: [] }) }
function switchMultiple() { mode.value = 'multiple'; invalidatePreview() }
function applyFrozen() { if (previewReady.value && frozenSnapshot) emit('apply', JSON.parse(JSON.stringify(frozenSnapshot))) }
function patchStatus(sku) { if (sku.id === Number(props.defaultSkuId) && sku.status === 'active') { error.value = '主规格不能停用，请先切换主规格'; return } emit('patch', { id: sku.id, input: { status: sku.status === 'active' ? 'disabled' : 'active' } }) }
const counts = computed(() => ({ add: props.preview?.add?.length || 0, keep: props.preview?.keep?.length || 0, disable: props.preview?.disable?.length || 0 }))
const previewCount = computed(() => counts.value.add + counts.value.keep + counts.value.disable)
</script>
<template><section class="creator-step sku-manager">
  <header><span class="step-number">02</span><div><h2>规格管理</h2><p>管理单规格或多规格组合，并指定生成任务使用的主规格。</p></div></header>
  <div class="sku-mode"><button data-testid="sku-mode-single" type="button" :class="{ active: mode === 'single' }" @click="switchSingle">单规格</button><button data-testid="sku-mode-multiple" type="button" :class="{ active: mode === 'multiple' }" @click="switchMultiple">多规格</button></div>
  <template v-if="mode === 'multiple'"><p class="sku-suggestions">常用维度：{{ suggestions.join('、') }}</p><div v-for="(dimension, index) in dimensions" :key="index" class="sku-dimension"><input v-model="dimension.name" data-testid="sku-dimension-name" placeholder="维度名称" @input="invalidatePreview"/><textarea v-model="dimension.values" data-testid="sku-dimension-values" rows="2" placeholder="用逗号分隔规格值" @input="invalidatePreview"></textarea><button type="button" @click="removeDimension(index)">删除维度</button></div><button data-testid="sku-add-dimension" type="button" :disabled="dimensions.length >= 3" @click="addDimension">添加维度</button><button data-testid="sku-preview" type="button" :disabled="busy" @click="requestPreview()">预览组合</button></template>
  <p v-else class="sku-suggestions">切换到单规格需预览影响并确认应用，已有多规格组合将被停用。</p>
  <div v-if="previewReady && preview" class="sku-preview"><p>预览共 {{ previewCount }} 个组合：新增 {{ counts.add }}、保留 {{ counts.keep }}、停用 {{ counts.disable }}</p><p v-if="preview.conflicts?.length" role="alert">存在 {{ preview.conflicts.length }} 个编码冲突，请修改后重试。</p><button data-testid="sku-apply" type="button" :disabled="busy || preview.conflicts?.length" @click="applyFrozen">确认应用</button></div>
  <p v-if="error" class="credit-warning" role="alert">{{ error }}</p>
  <div class="sku-list"><article v-for="sku in skus" :key="sku.id"><div><b>{{ displayCode(sku) }}</b><small>{{ sku.specification || '单规格' }} · {{ sku.status === 'active' ? '启用' : '停用' }}</small></div><input v-model="codes[sku.id]" :data-testid="`sku-code-${sku.id}`" aria-label="规格编码"/><button :data-testid="`sku-save-${sku.id}`" type="button" @click="emit('patch', { id: sku.id, input: { code: codes[sku.id] } })">保存编码</button><button :data-testid="`sku-default-${sku.id}`" type="button" :disabled="sku.id === Number(defaultSkuId) || sku.status !== 'active'" @click="emit('set-default', sku.id)">{{ sku.id === Number(defaultSkuId) ? '当前主规格' : '设为主规格' }}</button><button :data-testid="`sku-status-${sku.id}`" type="button" @click="patchStatus(sku)">{{ sku.status === 'active' ? '停用' : '恢复' }}</button></article></div>
</section></template>
<style scoped>.sku-mode{display:flex;gap:8px}.sku-mode .active{border-color:var(--commerce-accent);color:var(--commerce-accent)}.sku-suggestions{color:var(--commerce-muted);font-size:12px}.sku-dimension{display:grid;grid-template-columns:140px 1fr auto;gap:8px;margin:8px 0}.sku-list{display:grid;gap:8px;margin-top:14px}.sku-list article{display:grid;grid-template-columns:minmax(110px,1fr) minmax(120px,1fr) auto auto auto;gap:8px;align-items:center;padding:9px;border:1px solid var(--commerce-border);border-radius:10px}.sku-list article div{display:flex;flex-direction:column}.sku-list small{color:var(--commerce-muted)}@media(max-width:900px){.sku-dimension,.sku-list article{grid-template-columns:1fr}.sku-manager{overflow:visible}}</style>
