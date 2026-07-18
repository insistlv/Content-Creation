<script setup>
import { reactive, watch } from 'vue'
import { ScanSearch } from 'lucide-vue-next'
import ImageUploadZone from '../ImageUploadZone.vue'
import CommerceCategorySelector from './CommerceCategorySelector.vue'
const props = defineProps({ assets: { type: Array, default: () => [] }, skus: { type: Array, default: () => [] }, product: { type: Object, default: null }, categoryCatalog: { type: Object, default: () => ({}) }, categorySelection: { type: Object, default: null }, analyzing: Boolean, uploading: Boolean, disabled: Boolean })
const emit = defineEmits(['upload', 'remove', 'analyze', 'select-category', 'create-category', 'patch-category'])
const form = reactive({ title: '', category: '', sku_code: '', user_requirements: '' }), assetSKUs = reactive({})
watch(() => props.product, product => { form.title = product?.name || ''; form.category = product?.category || ''; form.sku_code = product?.sku_code || ''; form.user_requirements = product?.user_requirements || ''; Object.keys(assetSKUs).forEach(key => delete assetSKUs[key]) }, { immediate: true })
const roles = [{ key: 'product_front', title: '商品主图', hint: '必填 · 正面清晰完整' }, { key: 'product_back', title: '背面图', hint: '可选 · 包装与标签' }, { key: 'product_detail', title: '细节图', hint: '可选 · 材质与工艺' }]
const roleAssets = role => props.assets.filter(item => item.role === role)
const formPayload = () => ({ ...form, category_selection: props.categorySelection })
const binding = role => ({ role, lifecycle: 'project', ...(Number(assetSKUs[role]) ? { sku_id: Number(assetSKUs[role]) } : {}) })
function analyze() { if (form.title.trim()) emit('analyze', formPayload()) }
</script>
<template><section class="creator-step product-input" aria-labelledby="product-input-heading">
  <header><span class="step-number">01</span><div><h2 id="product-input-heading">上传商品，生成分析报告</h2><p>AI 只提取图片中可验证的事实，不猜测价格、材质或功效。</p></div></header>
  <div class="field-grid"><label>商品名称<input v-model="form.title" data-field="title" required placeholder="例如：便携保温杯"/></label><label>商品品类<CommerceCategorySelector :model-value="categorySelection" :catalog="categoryCatalog" :disabled="disabled" @select="value => emit('select-category', value)" @create-custom="value => emit('create-category', value)" @patch-custom="value => emit('patch-category', value)"/><input v-model="form.category" data-field="category" type="hidden"/></label></div>
  <label>商品规格编码（SKU，可选）<input v-model="form.sku_code" data-field="sku_code" placeholder="用于内部识别"/><small>仅用于内部识别；不填将自动生成，创建后仍可修改。</small></label>
  <div class="role-uploads"><article v-for="role in roles" :key="role.key"><div><b>{{ role.title }}</b><small>{{ role.hint }}</small><label>适用规格<select v-model="assetSKUs[role.key]" :data-testid="`asset-sku-${role.key}`"><option value="">全部规格共用</option><option v-for="sku in skus.filter(item => item.status === 'active')" :key="sku.id" :value="sku.id">{{ sku.code === 'DEFAULT' ? '默认规格' : sku.code }}</option></select></label></div><ImageUploadZone :images="roleAssets(role.key)" :max-images="role.key === 'product_front' ? 1 : 2" :uploading="uploading" :disabled="disabled || !form.title.trim() || !categorySelection" :empty-title="`上传${role.title}`" empty-hint-secondary="" @upload="file => emit('upload', file, binding(role.key), formPayload())" @remove="asset => emit('remove', asset)"/></article></div>
  <p v-if="skus.some(sku => sku.status === 'active' && !assets.some(asset => asset.sku_id === sku.id))" class="notice">部分规格没有专属图片，将使用全部规格共用素材。</p>
  <label>补充要求<textarea v-model="form.user_requirements" data-field="user_requirements" rows="3"></textarea></label><button class="primary-action" type="button" :disabled="disabled || analyzing || !form.title.trim() || !categorySelection || !assets.length" @click="analyze"><ScanSearch :size="18"/>{{ analyzing ? '正在分析商品…' : '生成商品分析报告' }}</button>
</section></template>
