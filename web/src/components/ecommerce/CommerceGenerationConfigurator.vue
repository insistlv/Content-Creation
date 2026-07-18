<script setup>
import { computed } from 'vue'
import { Coins, Sparkles } from 'lucide-vue-next'
import { displayLabel } from './commerceDisplayLabels.js'
const props = defineProps({ recipe: { type: Object, default: null }, definition: { type: Object, default: () => ({}) }, specConfirmed: Boolean, selectedSections: Array, sectionScopes: { type: Object, default: () => ({}) }, aspectRatio: String, qualityTier: String, layoutTemplate: String, skus: { type: Array, default: () => [] }, selectedSkuIds: { type: Array, default: () => [] }, primarySkuId: [Number, String], estimate: Object, credits: [Number, String, Object], submitting: Boolean })
const emit = defineEmits(['sections', 'section-scopes', 'aspect', 'quality', 'layout', 'skus', 'primary-sku', 'estimate', 'submit'])
const optionRows = (values, options, fallback) => (values || []).map(item => {
  const value = typeof item === 'string' ? item : (item.value ?? item.key)
  return { value, label: displayLabel(options, value, fallback) }
})
const sections = computed(() => optionRows(props.definition.sections, props.definition.section_options, '未知章节'))
const qualityOptions = computed(() => optionRows(props.definition.quality_tiers, props.definition.quality_options, '未知质量档'))
const layoutOptions = computed(() => optionRows(props.definition.layout_templates, props.definition.layout_template_options, '未知排版'))
const validOutputCount = computed(() => !props.definition.allowed_output_counts?.length || props.definition.allowed_output_counts.includes(props.selectedSections?.length || 0))
const checked = (key) => props.selectedSections?.includes(key)
function toggle(key) { emit('sections', checked(key) ? props.selectedSections.filter(item => item !== key) : [...props.selectedSections, key]) }
const activeSKUs = computed(() => props.skus.filter(item => item.status === 'active'))
const skuSelected = id => props.selectedSkuIds.map(Number).includes(Number(id))
function toggleSKU(id) { const next = skuSelected(id) ? props.selectedSkuIds.filter(item => Number(item) !== Number(id)) : [...props.selectedSkuIds, Number(id)]; if (next.length) emit('skus', next) }
const valueOf = (item) => typeof item === 'string' ? item : (item.value ?? item.key)
const labelOf = (item) => typeof item === 'string' ? item : (item.label || '未知选项')
const scopeMeta = key => props.definition.section_scopes?.[key] || { scope: 'sku' }
const scopeOf = key => props.sectionScopes[key] || scopeMeta(key).scope
const scopeLabel = key => scopeOf(key) === 'shared' ? '公共内容' : '按规格生成'
function setScope(section, scope) { emit('section-scopes', { ...props.sectionScopes, [section]: scope }) }
</script>
<template>
  <section class="creator-step generation-config" :class="{ locked: !specConfirmed }">
    <header><span class="step-number">03</span><div><h2>配置商品详情页</h2><p>{{ specConfirmed ? '选项由当前生成方案实时提供。' : '请先确认商品报告以解锁。' }}</p></div></header>
    <p v-if="!recipe" class="empty-state">商品详情页能力暂不可用，请稍后重试。</p>
    <fieldset v-if="activeSKUs.length" :disabled="!specConfirmed"><legend>生成规格（至少选择一个）</legend><div class="option-grid"><label v-for="sku in activeSKUs" :key="sku.id" :class="{ selected: skuSelected(sku.id) }"><input type="checkbox" :data-testid="`generation-sku-${sku.id}`" :checked="skuSelected(sku.id)" @change="toggleSKU(sku.id)"/><span>{{ sku.code === 'DEFAULT' ? '默认规格' : sku.code }}</span></label></div><p v-if="!selectedSkuIds.length" class="credit-warning">至少选择一个规格</p><label>主规格<select data-testid="generation-primary-sku" :value="primarySkuId" @change="emit('primary-sku', Number($event.target.value))"><option v-for="sku in activeSKUs.filter(item => skuSelected(item.id))" :key="sku.id" :value="sku.id">{{ sku.code === 'DEFAULT' ? '默认规格' : sku.code }}</option></select></label></fieldset>
    <fieldset v-if="recipe" :disabled="!specConfirmed"><legend>选择详情页章节</legend><div class="section-scope-list"><div v-for="item in sections" :key="valueOf(item)" :data-testid="`section-scope-${valueOf(item)}`" class="section-scope-row"><label :class="{ selected: checked(valueOf(item)) }"><input type="checkbox" :checked="checked(valueOf(item))" @change="toggle(valueOf(item))"/><span>{{ labelOf(item) }}</span></label><select v-if="scopeMeta(valueOf(item)).configurable" :value="scopeOf(valueOf(item))" :aria-label="`${labelOf(item)}作用域`" @change="setScope(valueOf(item), $event.target.value)"><option value="shared">公共内容</option><option value="sku">按规格生成</option></select><b v-else>{{ scopeLabel(valueOf(item)) }}</b></div></div></fieldset>
    <fieldset :disabled="!specConfirmed"><legend>画幅比例</legend><div class="option-grid"><label v-for="item in definition.aspect_ratios || []" :key="valueOf(item)" :class="{ selected: aspectRatio === valueOf(item) }"><input type="radio" name="aspect" :value="valueOf(item)" :checked="aspectRatio === valueOf(item)" @change="emit('aspect', valueOf(item))"/><span>{{ labelOf(item) }}</span></label></div></fieldset>
    <fieldset :disabled="!specConfirmed"><legend>质量档</legend><div class="option-grid"><label v-for="item in qualityOptions" :key="valueOf(item)" :class="{ selected: qualityTier === valueOf(item) }"><input type="radio" name="quality" :checked="qualityTier === valueOf(item)" @change="emit('quality', valueOf(item))"/><span>{{ labelOf(item) }}</span></label></div></fieldset>
    <fieldset v-if="layoutOptions.length" :disabled="!specConfirmed"><legend>排版模板</legend><div class="option-grid"><label v-for="item in layoutOptions" :key="valueOf(item)" :class="{ selected: layoutTemplate === valueOf(item) }"><input type="radio" name="layout" :checked="layoutTemplate === valueOf(item)" @change="emit('layout', valueOf(item))"/><span>{{ labelOf(item) }}</span></label></div></fieldset>
    <div class="estimate-card"><div><Coins :size="19"/><span>当前余额</span><b>{{ credits ?? '—' }}</b></div><template v-if="estimate"><div><span>公共任务</span><b>公共任务 {{ estimate.shared_items ?? estimate.shared_item_count ?? 0 }}</b></div><div><span>规格任务</span><b>规格任务 {{ estimate.sku_items ?? estimate.sku_item_count ?? Math.max(0, (estimate.total_items || 0) - (estimate.shared_items || 0)) }}</b></div><div><span>总图片</span><b>总图片 {{ estimate.total_items }}</b></div><div><span>总点数</span><b>总点数 {{ estimate.estimated_credits }}</b></div><div><span>预计用时</span><b>{{ estimate.eta_seconds ? `${estimate.eta_seconds} 秒` : '服务端计算中' }}</b></div><small v-if="estimate.pricing_expires_at">估价有效至 {{ new Date(estimate.pricing_expires_at).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' }) }}</small><p v-if="estimate.enough === false" class="credit-warning" role="alert">点数余额不足，请充值后再生成。</p></template></div>
    <p v-if="specConfirmed && !validOutputCount" class="credit-warning" role="alert">当前章节数量不符合生成规则，请调整选择。</p><div class="config-actions"><button type="button" :disabled="!specConfirmed || !validOutputCount" @click="emit('estimate')">预估点数与时间</button><button class="primary-action" type="button" :disabled="!estimate || estimate.enough === false || submitting" @click="emit('submit')"><Sparkles :size="18"/>{{ submitting ? '正在提交…' : '确认并开始生成' }}</button></div>
  </section>
</template>
