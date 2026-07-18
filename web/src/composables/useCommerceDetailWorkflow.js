import { computed, getCurrentInstance, onBeforeUnmount, ref } from 'vue'
import { api } from '../api/client.js'
import { useCommerceAssets } from './useCommerceAssets.js'
import { useCommerceBatches } from './useCommerceBatches.js'
import { commerceUserMessage } from '../components/ecommerce/commerceUserMessages.js'

const itemsOf = (value) => Array.isArray(value) ? value : (value?.items || [])
const key = (scope) => `${scope}-${globalThis.crypto?.randomUUID?.() || `${Date.now()}-${Math.random().toString(36).slice(2)}`}`
function normalizeRecipe(item = {}) {
  const source = item.definition || item.Definition || item
  const definition = {
    sections: source.sections || source.Sections || source.detail_sections || source.DetailSections || [],
    aspect_ratios: source.aspect_ratios || source.AspectRatios || [],
    quality_tiers: source.quality_tiers || source.QualityTiers || [],
    layout_templates: source.layout_templates || source.LayoutTemplates || [],
    section_options: source.section_options || source.SectionOptions || [],
    section_scopes: source.section_scopes || source.SectionScopes || {},
    quality_options: source.quality_options || source.QualityOptions || [],
    layout_template_options: source.layout_template_options || source.LayoutTemplateOptions || [],
    allowed_output_counts: source.allowed_output_counts || source.AllowedOutputCounts || [],
    default_parameters: source.parameters || source.default_parameters || source.DefaultParameters || {},
    required_assets: source.required_assets || source.RequiredAssets || [],
    optional_assets: source.optional_assets || source.OptionalAssets || []
  }
  return { ...item, key: item.key || item.Key || source.key || source.Key, title: item.title || item.Title || source.title || source.Title,
    version: item.version || item.Version || source.version || source.Version, definition }
}

export function useCommerceDetailWorkflow(options = {}) {
  const capabilities = ref(null), recipes = ref([]), projects = ref([]), currentProject = ref(null)
  const currentProduct = ref(null), currentSKU = ref(null), creativeSpec = ref(null), credits = ref(null)
	const skus = ref([]), skuConfig = ref({ version: 0, dimensions: [], values: [], skus: [] }), skuPreview = ref(null)
	const selectedSkuIds = ref([]), primarySkuId = ref(null), skuBusy = ref(false)
	const skuContextDirty = ref(false), needsReconfirmation = ref(false)
	const categoryCatalog = ref({ system_categories: [], custom_categories: [], recent_categories: [] }), categorySelection = ref(null)
  const loading = ref(false), analyzing = ref(false), saving = ref(false), submitting = ref(false), error = ref(''), notice = ref('')
  const mobileTab = ref('create'), resultMode = ref('cases'), estimateResult = ref(null)
  const selectedSections = ref([]), aspectRatio = ref(''), qualityTier = ref(''), layoutTemplate = ref('')
  const sectionScopes = ref({})
  const { assets, loading: assetsLoading, refresh: refreshCommerceAssets, upload: uploadCommerceAsset, remove: removeCommerceAsset } = useCommerceAssets()
  const batches = useCommerceBatches()
  let stopped = false, bootstrapKey = '', analysisKey = '', submitKey = '', estimatedSubmission = null, frozenSKUPreviewRequest = null, skuPreviewToken = 0
  const recipe = computed(() => recipes.value.find((item) => item.key === 'product_detail_set') || null)
  const definition = computed(() => recipe.value?.definition || {})

  function applyDefinition() {
    const d = definition.value
    if (!selectedSections.value.length) selectedSections.value = (d.sections || []).map((item) => typeof item === 'string' ? item : item.key)
    if (!aspectRatio.value) aspectRatio.value = d.aspect_ratios?.[0] || ''
    if (!qualityTier.value) qualityTier.value = (d.quality_tiers?.[0]?.key || d.quality_tiers?.[0] || '')
    if (!layoutTemplate.value) layoutTemplate.value = (d.layout_templates?.[0]?.key || d.layout_templates?.[0] || d.default_parameters?.layout_template || '')
  }
  async function restoreProject(project) {
    currentProject.value = project || null; currentSKU.value = null; skus.value = []; selectedSkuIds.value = []; primarySkuId.value = null; sectionScopes.value = {}; skuPreview.value = null; frozenSKUPreviewRequest = null; skuContextDirty.value = false; needsReconfirmation.value = false; creativeSpec.value = null; estimateResult.value = null
    if (!project) { batches.reset(); assets.value = []; return }
	currentProduct.value = project.product_id ? await api.getCommerceProduct(project.product_id).catch(() => ({ name: project.title })) : { name: project.title }
	if (project.product_id) {
	  const [listedSKUs, config] = await Promise.all([api.listCommerceSKUs(project.product_id), api.getCommerceSKUConfig(project.product_id)])
	  skus.value = itemsOf(listedSKUs); skuConfig.value = config
	  currentSKU.value = skus.value.find(item => item.id === project.default_sku_id) || null
	  primarySkuId.value = currentSKU.value?.id || null; selectedSkuIds.value = primarySkuId.value ? [primarySkuId.value] : []
	}
	categorySelection.value = currentProduct.value?.category_id ? { id: currentProduct.value.category_id, source: currentProduct.value.category_source || 'system', path: currentProduct.value.category_path || currentProduct.value.category, name: (currentProduct.value.category_path || currentProduct.value.category || '').split(' / ').at(-1) } : (currentProduct.value?.category ? { source: 'legacy', path: currentProduct.value.category, name: currentProduct.value.category } : null)
    await refreshCommerceAssets(project.id); await batches.start(project.id)
    if (batches.batches.value.length) resultMode.value = 'history'
    try { creativeSpec.value = await api.getLatestCommerceCreativeSpec(project.id) } catch (reason) {
      if (reason?.status !== 404) throw reason
      if (project.active_creative_spec_id) creativeSpec.value = await api.getCommerceCreativeSpec(project.active_creative_spec_id)
    }
    if (creativeSpec.value?.status === 'analyzing') { analyzing.value = true; try { await pollSpec(creativeSpec.value.id) } finally { analyzing.value = false } }
  }
  async function initialize() {
    loading.value = true; error.value = ''
    try {
      const caps = await api.getCommerceCapabilities()
      capabilities.value = caps
      if (!caps?.enabled) { projects.value = []; recipes.value = []; currentProject.value = null; creativeSpec.value = null; credits.value = null; batches.stop(); return }
	  const [listed, recipeResponse, balance, categories] = await Promise.all([api.listCommerceProjects(), api.listCommerceRecipes({ pipeline: 'general' }), api.getCredits().catch(() => null), api.listCommerceCategories()])
      projects.value = itemsOf(listed)
      recipes.value = itemsOf(recipeResponse).map(normalizeRecipe).filter((item) => item.key === 'product_detail_set')
      credits.value = balance?.available_credits ?? null
	  categoryCatalog.value = categories
      applyDefinition(); await restoreProject(projects.value[0])
    } catch (reason) { error.value = commerceUserMessage(reason, 'AI 电商工作台加载失败') } finally { loading.value = false }
  }
  function requireEnabled() { if (capabilities.value?.enabled === false) throw new Error('AI 电商未开启') }
  async function ensureProject(input) {
    requireEnabled()
    if (currentProject.value) return currentProject.value
    bootstrapKey ||= key('commerce-bootstrap')
	const selection = input.category_selection || categorySelection.value
	const result = await api.bootstrapCommerceProject({ title: input.title?.trim(), category: selection?.path || input.category?.trim(), category_id: selection?.id || 0, category_source: selection?.source || '', category_path: selection?.path || '', sku_code: input.sku_code?.trim() || '', pipeline: 'general' }, bootstrapKey)
    currentProduct.value = result.product; currentSKU.value = result.sku; currentProject.value = result.project
	 skus.value = result.sku ? [result.sku] : []; skuConfig.value = { version: 0, dimensions: [], values: [], skus: skus.value }
	 primarySkuId.value = result.project.default_sku_id || result.sku?.id || null; selectedSkuIds.value = primarySkuId.value ? [primarySkuId.value] : []
    projects.value = [result.project, ...projects.value.filter((item) => item.id !== result.project.id)]
    await refreshCommerceAssets(result.project.id); await batches.start(result.project.id)
    return result.project
  }
	async function selectCategory(selection) {
	  if (currentProduct.value?.id && selection?.id) currentProduct.value = await api.patchCommerceProduct(currentProduct.value.id, { category_id: selection.id, category_source: selection.source, category_path: selection.path })
	  categorySelection.value = selection; bootstrapKey = ''
	}
	async function refreshCategories() { categoryCatalog.value = await api.listCommerceCategories(); return categoryCatalog.value }
	async function createCustomCategory(input) { const created = await api.createCommerceCustomCategory(input); await refreshCategories(); await selectCategory(created); return created }
	async function patchCustomCategory({ id, input }) { await api.patchCommerceCustomCategory(id, input); return refreshCategories() }
  async function analyze(input) {
    requireEnabled()
    const project = await ensureProject(input)
    const sourceIds = assets.value.map((asset) => asset.id)
    if (!sourceIds.length) throw new Error('请至少上传一张商品图片')
    analyzing.value = true; error.value = ''; analysisKey ||= key('commerce-analysis')
    try {
      const response = await api.analyzeCommerceCreativeSpec(project.id, { source_asset_ids: sourceIds, user_requirements: input.user_requirements || '' }, analysisKey)
      creativeSpec.value = response.creative_spec
      await pollSpec(response.creative_spec.id)
      skuContextDirty.value = false
      if (needsReconfirmation.value) notice.value = '规格相关内容已重新分析，请确认商品报告后继续。'
      analysisKey = ''
      return creativeSpec.value
    } catch (reason) {
      if (reason?.code === 'commerce_vision_not_configured' || reason?.status === 503) notice.value = '自动识别未配置，请手工补录商品事实后继续。'
      error.value = commerceUserMessage(reason, '商品分析失败，请稍后重试'); throw reason
    } finally { analyzing.value = false }
  }
  async function createManualReport() {
    requireEnabled()
    if (!currentProject.value) throw new Error('请先填写商品信息并上传图片')
    creativeSpec.value = await api.createManualCommerceCreativeSpec(currentProject.value.id, {
      product_facts: {}, selling_points: [], forbidden_changes: [], brand_tone: {}, copy_blocks: [], risk_notices: []
    })
    notice.value = '已创建手工报告，请补齐关键信息后确认。'
    return creativeSpec.value
  }
  async function pollSpec(id) {
    const delay = options.pollDelay ?? 1200
    for (let attempt = 0; attempt < 120 && !stopped; attempt += 1) {
      const spec = await api.getCommerceCreativeSpec(id); creativeSpec.value = spec
      if (!['analyzing', 'queued', 'running'].includes(spec.status)) return spec
      await new Promise((resolve) => setTimeout(resolve, delay))
    }
    return creativeSpec.value
  }
  async function saveReport(overrides) {
    requireEnabled()
    saving.value = true
    try { creativeSpec.value = await api.patchCommerceCreativeSpec(creativeSpec.value.id, { expected_version: creativeSpec.value.version, ...overrides }); invalidateEstimate(); return creativeSpec.value }
    catch (reason) {
      if (reason?.code === 'version_conflict' || reason?.status === 409) { creativeSpec.value = await api.getCommerceCreativeSpec(creativeSpec.value.id); notice.value = '报告已被更新，请合并最新版本后重试。' }
      throw reason
    } finally { saving.value = false }
  }
  async function confirmReport() { requireEnabled(); if (skuContextDirty.value) throw new Error('请先重新分析规格相关内容'); creativeSpec.value = await api.confirmCommerceCreativeSpec(creativeSpec.value.id); needsReconfirmation.value = false; invalidateEstimate(); return creativeSpec.value }
  function invalidateEstimate() { estimateResult.value = null; estimatedSubmission = null; submitKey = '' }
	function invalidateSKUContext() { invalidateEstimate(); skuContextDirty.value = true; needsReconfirmation.value = true; notice.value = '规格已变化，请重新分析并确认商品报告。' }
	function setSelectedSKUs(ids) { const active = new Set(skus.value.filter(s => s.status === 'active').map(s => s.id)); const next = [...new Set(ids.map(Number))].filter(id => active.has(id)); if (!next.length) throw new Error('至少选择一个规格'); selectedSkuIds.value = next; if (!next.includes(primarySkuId.value)) primarySkuId.value = next[0]; invalidateSKUContext() }
	function setPrimarySKU(id) { id = Number(id); if (!selectedSkuIds.value.includes(id)) throw new Error('主规格必须位于已选规格中'); primarySkuId.value = id; invalidateSKUContext() }
	function reconcileSelection() { const active = skus.value.filter(s => s.status === 'active'); const activeIDs = new Set(active.map(s => s.id)); const defaultSKU = active.find(s => s.id === currentProject.value?.default_sku_id); selectedSkuIds.value = [...new Set(selectedSkuIds.value.map(Number))].filter(id => activeIDs.has(id)); if (!selectedSkuIds.value.length && active.length) selectedSkuIds.value = [(defaultSKU || active[0]).id]; if (!selectedSkuIds.value.includes(primarySkuId.value)) primarySkuId.value = (defaultSKU && selectedSkuIds.value.includes(defaultSKU.id) ? defaultSKU.id : selectedSkuIds.value[0]) || null; if (currentProject.value?.default_sku_id) currentSKU.value = skus.value.find(s => s.id === currentProject.value.default_sku_id) || null; else currentSKU.value = active.find(s => s.id === primarySkuId.value) || null }
	async function previewSKUMatrix(input) { const frozen = JSON.parse(JSON.stringify(input)); const token = ++skuPreviewToken; skuBusy.value = true; try { const preview = await api.previewCommerceSKUMatrix(currentProduct.value.id, frozen); if (token !== skuPreviewToken) return preview; skuPreview.value = preview; frozenSKUPreviewRequest = frozen; return preview } finally { if (token === skuPreviewToken) skuBusy.value = false } }
	function clearSKUPreview() { skuPreviewToken += 1; skuPreview.value = null; frozenSKUPreviewRequest = null; skuBusy.value = false }
	async function applySKUMatrix() { if (!frozenSKUPreviewRequest || !skuPreview.value) throw new Error('请先预览规格组合'); const frozen = JSON.parse(JSON.stringify(frozenSKUPreviewRequest)); skuBusy.value = true; try { skuConfig.value = await api.applyCommerceSKUMatrix(currentProduct.value.id, frozen, key('sku-matrix')); skus.value = skuConfig.value.skus || []; if (Object.prototype.hasOwnProperty.call(skuConfig.value, 'default_sku_id') && skuConfig.value.default_sku_id != null && currentProject.value) currentProject.value = { ...currentProject.value, default_sku_id: skuConfig.value.default_sku_id }; reconcileSelection(); clearSKUPreview(); invalidateSKUContext(); return skuConfig.value } catch (reason) { if (reason?.code === 'sku_version_conflict' || reason?.status === 409) { skuConfig.value = await api.getCommerceSKUConfig(currentProduct.value.id); clearSKUPreview(); notice.value = '规格版本已更新，请核对最新配置后重试。' } throw reason } finally { skuBusy.value = false } }
	async function patchSKU({ id, input }) { const updated = await api.patchCommerceSKU(id, input); skus.value = skus.value.map(s => s.id === id ? updated : s); reconcileSelection(); invalidateSKUContext(); return updated }
	async function setDefaultSKU(id) { id = Number(id); const project = await api.patchCommerceProject(currentProject.value.id, { default_sku_id: id }); currentProject.value = project; currentSKU.value = skus.value.find(s => s.id === (project.default_sku_id || id)) || null; if (!selectedSkuIds.value.includes(id)) selectedSkuIds.value = [...selectedSkuIds.value, id]; setPrimarySKU(id); return project }
  function setAspectRatio(value) { aspectRatio.value = value; invalidateEstimate() }
  function setQualityTier(value) { qualityTier.value = value; invalidateEstimate() }
  function setLayoutTemplate(value) { layoutTemplate.value = value; invalidateEstimate() }
  function setSections(value) { selectedSections.value = [...value]; invalidateEstimate() }
  function setSectionScopes(value) { sectionScopes.value = { ...value }; invalidateEstimate() }
  function requestPayload() {
    const supportedRoles = new Set([...(definition.value.required_assets || []), ...(definition.value.optional_assets || [])].map(item => item.role).filter(Boolean))
    const assetBindings = {}
    for (const asset of assets.value) if (supportedRoles.has(asset.role)) (assetBindings[asset.role] ||= []).push(asset.id)
    return { recipe_key: recipe.value.key, recipe_version: recipe.value.version, quality_tier: qualityTier.value,
      output_count: selectedSections.value.length, creative_spec_id: creativeSpec.value.id,
      primary_sku_id: primarySkuId.value || currentProject.value.default_sku_id || currentSKU.value?.id,
      selected_sku_ids: selectedSkuIds.value.length ? selectedSkuIds.value : [currentProject.value.default_sku_id || currentSKU.value?.id].filter(Boolean), aspect_ratio: aspectRatio.value,
      asset_bindings: assetBindings,
      parameters: { detail_sections: selectedSections.value, section_scopes: { ...sectionScopes.value }, ...(layoutTemplate.value ? { layout_template: layoutTemplate.value } : {}) } }
  }
  async function estimate() {
    requireEnabled()
    if (skuContextDirty.value || needsReconfirmation.value) throw new Error('规格已变化，请重新分析并确认商品报告')
    if (!capabilities.value?.worker_enabled) throw new Error('生成服务暂不可用')
    if (creativeSpec.value?.status !== 'confirmed') throw new Error('请先确认商品报告')
    if (!recipe.value) throw new Error('商品详情页生成方案不可用')
    if (!assets.value.some((asset) => asset.role === 'product_front')) throw new Error('请先上传并标记商品主图')
    if (definition.value.allowed_output_counts?.length && !definition.value.allowed_output_counts.includes(selectedSections.value.length)) throw new Error('当前章节数量不符合生成方案规则')
    const frozenRequest = JSON.parse(JSON.stringify(requestPayload()))
    const response = await api.estimateCommerceBatch(currentProject.value.id, frozenRequest)
    const estimateItems = itemsOf(response)
    const sharedItems = estimateItems.filter(item => item.scope === 'shared').length
    estimateResult.value = { ...response, shared_items: sharedItems, sku_items: Math.max(0, Number(response.total_items || estimateItems.length) - sharedItems), enough: Number(credits.value || 0) >= Number(response.estimated_credits || 0) }
    estimatedSubmission = {
      request: frozenRequest,
      pricingSnapshotId: response.pricing_snapshot_id,
      requestDigest: response.request_digest || '',
      expiresAt: response.pricing_expires_at || '',
      estimatedCredits: Number(response.estimated_credits || 0)
    }
    submitKey = ''
    return estimateResult.value
  }
  async function submit() {
    requireEnabled()
    if (skuContextDirty.value || needsReconfirmation.value) throw new Error('规格已变化，请重新分析并确认商品报告')
    if (!estimateResult.value || !estimatedSubmission) throw new Error('请先完成点数估价')
    if (estimateResult.value.enough === false) throw new Error('点数余额不足')
    const expiresAt = Date.parse(estimatedSubmission.expiresAt)
    if (Number.isFinite(expiresAt) && expiresAt <= Date.now()) {
      invalidateEstimate(); notice.value = '估价已失效，请重新估价并确认。'
      throw new Error('估价已失效，请重新估价并确认')
    }
    submitting.value = true; submitKey ||= key('commerce-batch')
    try {
      const payload = { ...JSON.parse(JSON.stringify(estimatedSubmission.request)), pricing_snapshot_id: estimatedSubmission.pricingSnapshotId }
      const batch = await api.createCommerceBatch(currentProject.value.id, payload, submitKey)
      invalidateEstimate()
      resultMode.value = 'history'; mobileTab.value = 'results'; batches.start(currentProject.value.id); return batch
    } catch (reason) {
      if (['pricing_snapshot_stale', 'pricing_snapshot_expired', 'pricing_stale', 'estimate_expired'].includes(reason?.code)) { invalidateEstimate(); notice.value = '估价已失效，请重新估价并确认。' }
      throw reason
    } finally { submitting.value = false }
  }
  async function refreshAssets(projectId) { invalidateEstimate(); return refreshCommerceAssets(projectId) }
  async function upload(projectId, file, binding) { invalidateEstimate(); const result = await uploadCommerceAsset(projectId, file, binding); if (binding?.sku_id) invalidateSKUContext(); return result }
  async function remove(projectId, assetId) { const skuSpecific = assets.value.find(asset => asset.id === assetId)?.sku_id; invalidateEstimate(); const result = await removeCommerceAsset(projectId, assetId); if (skuSpecific) invalidateSKUContext(); return result }
  const retryKeys = new Map()
  async function refreshBatches() { await batches.poll() }
  async function cancelBatch(batch) { requireEnabled(); await api.cancelCommerceBatch(batch.id); await refreshBatches() }
  async function cancelItem(item) { requireEnabled(); await api.cancelCommerceItem(item.id); await refreshBatches() }
  async function retryItem(item) { requireEnabled(); if (!retryKeys.has(item.id)) retryKeys.set(item.id, key(`commerce-retry-${item.id}`)); await api.retryCommerceItem(item.id, retryKeys.get(item.id)); retryKeys.delete(item.id); await refreshBatches() }
  async function selectProject(project) { await restoreProject(project) }
	function newCreation() { batches.reset(); currentProject.value = null; currentProduct.value = null; currentSKU.value = null; skus.value = []; skuConfig.value = { version: 0, dimensions: [], values: [], skus: [] }; clearSKUPreview(); selectedSkuIds.value = []; primarySkuId.value = null; sectionScopes.value = {}; skuContextDirty.value = false; needsReconfirmation.value = false; creativeSpec.value = null; categorySelection.value = null; assets.value = []; invalidateEstimate(); bootstrapKey = ''; analysisKey = ''; resultMode.value = 'cases'; mobileTab.value = 'create' }
  if (getCurrentInstance()) onBeforeUnmount(() => { stopped = true })
	return { capabilities, recipes, recipe, definition, projects, currentProject, currentProduct, currentSKU, skus, skuConfig, skuPreview, selectedSkuIds, primarySkuId, skuBusy, skuContextDirty, needsReconfirmation, creativeSpec, credits, categoryCatalog, categorySelection,
    assets, assetsLoading, batches, loading, analyzing, saving, submitting, error, notice, mobileTab, resultMode,
    selectedSections, sectionScopes, aspectRatio, qualityTier, layoutTemplate, estimateResult, initialize, ensureProject, analyze,
    saveReport, confirmReport, estimate, submit, requestPayload, setAspectRatio, setQualityTier, setLayoutTemplate, setSections, setSectionScopes, invalidateEstimate, invalidateSKUContext, setSelectedSKUs, setPrimarySKU,
	previewSKUMatrix, clearSKUPreview, applySKUMatrix, patchSKU, setDefaultSKU, selectProject, newCreation, refreshAssets, upload, remove, createManualReport, cancelBatch, cancelItem, retryItem, selectCategory, refreshCategories, createCustomCategory, patchCustomCategory }
}
