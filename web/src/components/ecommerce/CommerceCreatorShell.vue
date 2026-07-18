<script setup>
import { computed } from 'vue'
import { CircleHelp, Coins, PanelRight } from 'lucide-vue-next'
import { useUserTheme } from '../../composables/useUserTheme.js'
import CommerceProjectSwitcher from './CommerceProjectSwitcher.vue'
import CommerceProductInput from './CommerceProductInput.vue'
import CommerceSKUManager from './CommerceSKUManager.vue'
import CommerceReportEditor from './CommerceReportEditor.vue'
import CommerceGenerationConfigurator from './CommerceGenerationConfigurator.vue'
import CommerceResultPanel from './CommerceResultPanel.vue'
import { commerceUserMessage } from './commerceUserMessages.js'
const props = defineProps({ workflow: { type: Object, required: true } })
const w = props.workflow
const { theme } = useUserTheme()
const confirmed = computed(() => w.creativeSpec.value?.status === 'confirmed')
async function upload(file, binding, form) { await run(async () => { await w.ensureProject(form); await w.upload(w.currentProject.value.id, file, binding); w.invalidateEstimate() }) }
async function remove(asset) { await run(async () => { await w.remove(w.currentProject.value.id, asset.id); w.invalidateEstimate() }) }
async function run(action) { w.error.value = ''; try { await action() } catch (reason) { w.error.value = commerceUserMessage(reason) } }
</script>
<template>
  <main class="commerce-creator-shell" data-testid="commerce-creator-shell" :data-theme="theme">
    <header class="creator-topbar" data-testid="commerce-creator-topbar"><div class="brand-mark"><span><PanelRight :size="20"/></span><div><small>AI 电商</small><h1>AI 商品详情页</h1></div></div><div class="topbar-center" data-testid="commerce-project-toolbar"><CommerceProjectSwitcher :projects="w.projects.value" :current-project="w.currentProject.value" @select="project => run(() => w.selectProject(project))" @new="w.newCreation"/><p data-testid="commerce-project-title">{{ w.currentProject.value?.title || '上传商品素材，自动生成完整详情页' }}</p></div><div class="topbar-meta"><button type="button" title="功能说明" @click="w.notice.value = '上传商品图后先核对 AI 报告，再选择章节、画幅与质量，确认点数后一次提交。'"><CircleHelp :size="18"/><span>功能说明</span></button><div class="credits"><Coins :size="18"/><span>点数</span><b>{{ w.credits.value ?? '—' }}</b></div></div></header>
    <nav class="mobile-workspace-tabs" role="tablist" aria-label="工作台视图"><button data-testid="commerce-mobile-tab-create" role="tab" :aria-selected="w.mobileTab.value === 'create'" @click="w.mobileTab.value = 'create'">创作</button><button data-testid="commerce-mobile-tab-results" role="tab" :aria-selected="w.mobileTab.value === 'results'" @click="w.mobileTab.value = 'results'">结果</button></nav>
    <p v-if="w.error.value" class="global-error" role="alert">{{ w.error.value }}</p>
    <section v-if="w.capabilities.value && !w.capabilities.value.enabled" class="commerce-disabled" role="status"><PanelRight :size="34"/><h2>AI 电商未开启</h2><p>该功能当前未开放，请联系管理员开启后再使用。</p></section>
    <template v-else>
    <p v-if="w.capabilities.value && !w.capabilities.value.worker_enabled" class="global-warning" role="status">生成服务暂不可用。您仍可上传商品并完善报告，服务恢复后再提交。</p>
    <div class="creator-columns">
      <section class="create-pane" data-testid="commerce-create-pane" :class="{ 'mobile-hidden': w.mobileTab.value !== 'create' }">
		<CommerceProductInput :assets="w.assets.value" :skus="w.skus.value" :product="w.currentProduct.value" :category-catalog="w.categoryCatalog.value" :category-selection="w.categorySelection.value" :analyzing="w.analyzing.value" :uploading="w.assetsLoading.value" :disabled="w.loading.value" @upload="upload" @remove="remove" @select-category="value => run(() => w.selectCategory(value))" @create-category="input => run(() => w.createCustomCategory(input))" @patch-category="input => run(() => w.patchCustomCategory(input))" @analyze="input => run(() => w.analyze(input))"/>
        <CommerceSKUManager v-if="w.currentProject.value" :skus="w.skus.value" :default-sku-id="w.currentProject.value.default_sku_id" :config="w.skuConfig.value" :preview="w.skuPreview.value" :busy="w.skuBusy.value" @preview="input => run(() => w.previewSKUMatrix(input))" @edit="w.clearSKUPreview" @apply="run(w.applySKUMatrix)" @patch="input => run(() => w.patchSKU(input))" @set-default="id => run(() => w.setDefaultSKU(id))"/>
        <CommerceReportEditor :spec="w.creativeSpec.value" :analyzing="w.analyzing.value" :saving="w.saving.value" :notice="w.notice.value" @save="overrides => run(() => w.saveReport(overrides))" @confirm="run(w.confirmReport)" @save-confirm="overrides => run(async () => { await w.saveReport(overrides); await w.confirmReport() })" @manual="run(w.createManualReport)"/>
        <CommerceGenerationConfigurator :recipe="w.recipe.value" :definition="w.definition.value" :spec-confirmed="confirmed && w.capabilities.value?.worker_enabled" :selected-sections="w.selectedSections.value" :section-scopes="w.sectionScopes.value" :aspect-ratio="w.aspectRatio.value" :quality-tier="w.qualityTier.value" :layout-template="w.layoutTemplate.value" :skus="w.skus.value" :selected-sku-ids="w.selectedSkuIds.value" :primary-sku-id="w.primarySkuId.value" :estimate="w.estimateResult.value" :credits="w.credits.value" :submitting="w.submitting.value" @sections="w.setSections" @section-scopes="w.setSectionScopes" @aspect="w.setAspectRatio" @quality="w.setQualityTier" @layout="w.setLayoutTemplate" @skus="w.setSelectedSKUs" @primary-sku="w.setPrimarySKU" @estimate="run(w.estimate)" @submit="run(w.submit)"/>
      </section>
      <aside class="result-pane" data-testid="commerce-result-pane" :class="{ 'mobile-hidden': w.mobileTab.value !== 'results' }"><CommerceResultPanel :theme="theme" :mode="w.resultMode.value" :batches="w.batches.batches.value" :events="w.batches.events.value" :assets="w.assets.value" :creative-spec="w.creativeSpec.value" :selected-sections="w.selectedSections.value" :aspect-ratio="w.aspectRatio.value" :quality-tier="w.qualityTier.value" :layout-template="w.layoutTemplate.value" :estimate="w.estimateResult.value" :current-project="w.currentProject.value" :loading="w.batches.loading.value" :error="w.batches.error.value" :definition="w.definition.value" @mode="value => w.resultMode.value = value" @cancel-batch="batch => run(() => w.cancelBatch(batch))" @cancel-item="item => run(() => w.cancelItem(item))" @retry-item="item => run(() => w.retryItem(item))"/></aside>
    </div>
    </template>
  </main>
</template>

<style>
html:has(.commerce-creator-shell),body:has(.commerce-creator-shell){min-width:0}
.commerce-creator-shell{--commerce-accent:#b7ff2a;--commerce-bg:#07090c;--commerce-panel:#101317;--commerce-panel-2:#15191e;--commerce-border:#293039;--commerce-muted:#8d969f;min-height:calc(100vh - 64px);background:var(--commerce-bg);color:#f5f7f8;padding:18px 22px 24px}.commerce-creator-shell *{box-sizing:border-box}.creator-topbar{display:grid;grid-template-columns:auto minmax(320px,1fr) auto;gap:24px;align-items:center;max-width:1600px;margin:0 auto 16px;border:1px solid var(--commerce-border);border-radius:16px;background:#0d1013;padding:13px 16px}.brand-mark,.brand-mark>span,.topbar-meta,.topbar-meta button,.credits,.project-switcher,.project-switcher label,.project-switcher button,.select-wrap,.creator-step>header,.status-line,.report-actions,.config-actions,.result-panel nav,.result-panel nav button,.estimate-card>div,.result-batch header,.result-items>div>div{display:flex;align-items:center}.brand-mark{gap:11px}.brand-mark>span{justify-content:center;width:38px;height:38px;border-radius:11px;background:var(--commerce-accent);color:#071000}.brand-mark small,.eyebrow{color:var(--commerce-accent);font-size:10px;letter-spacing:.14em}.brand-mark h1{font-size:16px;margin:2px 0 0}.topbar-center{display:flex;align-items:center;gap:18px;min-width:0}.topbar-center>p{color:var(--commerce-muted);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.project-switcher{gap:8px}.project-switcher label{gap:7px;font-size:12px;color:var(--commerce-muted)}.select-wrap{position:relative}.select-wrap svg{position:absolute;right:9px;pointer-events:none}.project-switcher select{appearance:none;padding:8px 30px 8px 10px;min-width:150px}.project-switcher button,.topbar-meta button,.result-panel nav button{height:36px}.topbar-meta{gap:9px}.topbar-meta button,.credits{gap:7px;padding:0 11px;border:1px solid var(--commerce-border);border-radius:10px;background:transparent;color:inherit}.credits{height:38px}.credits svg,.credits b{color:var(--commerce-accent)}.mobile-workspace-tabs{display:none}.global-error,.global-warning{max-width:1600px;margin:0 auto 12px;padding:10px 14px;border-radius:10px}.global-error{color:#ff9eaa;background:#32151b}.global-warning{color:#ffd97a;background:#312813}.creator-columns{display:grid;grid-template-columns:minmax(0,46fr) minmax(0,54fr);gap:16px;max-width:1600px;margin:auto;align-items:start}.create-pane{display:flex;flex-direction:column;gap:14px;min-width:0}.result-pane{min-width:0;position:sticky;top:16px;height:calc(100vh - 120px);border:1px solid var(--commerce-border);border-radius:18px;background:var(--commerce-panel);overflow:auto}.creator-step{border:1px solid var(--commerce-border);border-radius:16px;background:var(--commerce-panel);padding:18px}.creator-step>header{align-items:flex-start;gap:12px;margin-bottom:16px}.creator-step h2{font-size:16px;margin:0 0 4px}.creator-step header p,.empty-state{color:var(--commerce-muted);font-size:13px;margin:0}.step-number{display:grid;place-items:center;flex:0 0 32px;height:32px;border:1px solid #46505b;border-radius:9px;font-size:11px;color:var(--commerce-accent)}.commerce-creator-shell label{display:flex;flex-direction:column;gap:7px;color:#c7ccd1;font-size:12px}.commerce-creator-shell input,.commerce-creator-shell textarea,.commerce-creator-shell select{width:100%;border:1px solid #323941;border-radius:9px;background:#090c0f;color:#f5f7f8;padding:10px 11px;font:inherit}.commerce-creator-shell input:focus-visible,.commerce-creator-shell textarea:focus-visible,.commerce-creator-shell select:focus-visible,.commerce-creator-shell button:focus-visible,.commerce-creator-shell a:focus-visible{outline:2px solid var(--commerce-accent);outline-offset:2px}.field-grid,.report-form{display:grid;grid-template-columns:1fr 1fr;gap:12px;margin-bottom:12px}.role-uploads{display:grid;grid-template-columns:repeat(3,1fr);gap:9px;margin:14px 0}.role-uploads article{min-width:0;border:1px solid #242b32;border-radius:11px;padding:9px;background:#0b0e11}.role-uploads article>div{display:flex;flex-direction:column;margin-bottom:7px}.role-uploads small{color:var(--commerce-muted);font-size:10px}.role-uploads .image-upload-zone{min-height:118px;border:1px dashed #3b454f;border-radius:9px;display:grid;place-items:center;padding:8px;text-align:center}.role-uploads .upload-icon{width:20px}.role-uploads .upload-title{font-size:11px}.role-uploads .upload-hint{display:none}.role-uploads .image-preview-grid{width:100%}.role-uploads .preview-image{width:100%;height:95px;object-fit:cover;border-radius:7px}.role-uploads .remove-button{position:absolute;right:3px;top:3px}.image-preview-item{position:relative}.commerce-creator-shell button,.commerce-creator-shell a{display:inline-flex;align-items:center;justify-content:center;gap:7px;min-height:40px;border:1px solid #38414a;border-radius:9px;background:#151a20;color:#edf0f2;padding:8px 12px;cursor:pointer;text-decoration:none}.commerce-creator-shell button:disabled{opacity:.42;cursor:not-allowed}.commerce-creator-shell .primary-action{border-color:var(--commerce-accent);background:var(--commerce-accent);color:#081000;font-weight:750}.product-input>.primary-action{width:100%;margin-top:14px}.report-skeleton span{display:block;height:11px;margin:10px 0;border-radius:6px;background:linear-gradient(90deg,#171b20,#272d33,#171b20);background-size:200%;animation:commerce-shimmer 1.4s infinite}.report-skeleton span:nth-child(2){width:75%}.report-skeleton p{color:var(--commerce-muted)}@keyframes commerce-shimmer{to{background-position:-200%}}.notice,.missing,.risk{border-radius:9px;padding:10px 12px;background:#232716;font-size:12px}.status-line{justify-content:space-between;margin-bottom:12px}.status-line>span{padding:5px 9px;border-radius:99px;background:#242a30}.status-line .status-confirmed{color:var(--commerce-accent);background:#1c2911}.facts ul{list-style:none;padding:0;display:grid;gap:7px}.facts li{display:grid;grid-template-columns:100px 1fr auto;gap:9px;padding:9px;background:#0b0e11;border-radius:8px}.facts small{color:var(--commerce-muted)}.missing span{display:inline-block;margin-left:7px;color:#ffd97a}.report-form{margin-top:13px}.report-form .wide{grid-column:1/-1}.report-actions,.config-actions{justify-content:flex-end;gap:9px;margin-top:14px}.generation-config fieldset{border:0;padding:0;margin:15px 0}.generation-config legend{font-size:12px;color:#c7ccd1;margin-bottom:8px}.option-grid{display:flex;flex-wrap:wrap;gap:7px}.option-grid label{display:block;cursor:pointer}.option-grid input{position:absolute;opacity:0;pointer-events:none}.option-grid span{display:block;padding:8px 12px;border:1px solid #343c45;border-radius:8px;background:#0b0e11}.option-grid .selected span{border-color:var(--commerce-accent);color:var(--commerce-accent);background:#18210e}.estimate-card{display:grid;grid-template-columns:repeat(4,1fr);gap:8px;padding:12px;border:1px solid #2b333b;border-radius:10px;background:#0b0e11}.estimate-card>div{gap:6px;flex-wrap:wrap}.estimate-card span{color:var(--commerce-muted);font-size:11px}.estimate-card b{width:100%}.credit-warning{grid-column:1/-1;color:#ff8795}.result-panel nav{position:sticky;top:0;z-index:2;gap:5px;padding:12px;border-bottom:1px solid var(--commerce-border);background:#101317e8;backdrop-filter:blur(10px)}.result-panel nav button{border:0;background:transparent;color:var(--commerce-muted)}.result-panel nav .active{background:#20270f;color:var(--commerce-accent)}.case-library{padding:40px}.case-library header{max-width:530px}.case-library h2{font-size:clamp(28px,4vw,54px);line-height:1.04;margin:14px 0}.case-library header>p:last-child{color:var(--commerce-muted)}.case-library figure{margin:30px 0 18px;border:1px solid #333b43;border-radius:16px;overflow:hidden;background:#080a0c}.case-library img{display:block;width:100%;height:clamp(260px,42vh,510px);object-fit:cover}.case-library figcaption{display:flex;justify-content:space-between;padding:12px 14px}.case-library figcaption b{color:var(--commerce-accent)}.case-stats{display:grid;grid-template-columns:repeat(3,1fr);gap:9px}.case-stats span{display:flex;flex-direction:column;padding:13px;background:#0b0e11;border-radius:10px;color:var(--commerce-muted);font-size:11px}.case-stats b{font-size:23px;color:#fff}.history-list{padding:18px}.history-list>.empty-state{min-height:360px;display:flex;align-items:center;justify-content:center;flex-direction:column;gap:9px}.result-batch{padding:15px;border:1px solid var(--commerce-border);border-radius:12px;margin-bottom:12px;background:#0b0e11}.result-batch header{justify-content:space-between}.result-batch header div{display:flex;flex-direction:column}.progress{height:5px;border-radius:5px;background:#252c32;overflow:hidden;margin:13px 0}.progress i{display:block;height:100%;background:var(--commerce-accent)}.result-items>div{padding:10px 0;border-top:1px solid #242a30}.result-items>div>div{justify-content:space-between}.sr-only{position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0,0,0,0)}
.topbar-center{white-space:nowrap}.topbar-center>p{min-width:0;margin:0;line-height:40px}.project-switcher{flex:0 0 auto;white-space:nowrap}.commerce-creator-shell .project-switcher label{flex-direction:row;line-height:40px}.project-switcher select{height:40px}.project-switcher button{height:40px}
.commerce-creator-shell{--commerce-topbar-offset:82px;flex-shrink:0}.creator-topbar{position:sticky;top:0;z-index:10;background:#0d1013}.result-pane{top:var(--commerce-topbar-offset);height:calc(100vh - var(--commerce-topbar-offset) - 56px)}
@media(max-width:1199px){.commerce-creator-shell{--commerce-topbar-offset:142px}.creator-topbar{grid-template-columns:auto 1fr}.topbar-meta{grid-column:1/-1;justify-content:flex-end}.creator-columns{grid-template-columns:minmax(0,1fr) minmax(0,1.08fr)}.role-uploads{grid-template-columns:1fr}.role-uploads article{display:grid;grid-template-columns:150px 1fr}.estimate-card{grid-template-columns:1fr 1fr}}
@media(max-width:767px){.commerce-creator-shell{padding:10px}.creator-topbar{position:static;grid-template-columns:1fr;margin-bottom:8px}.topbar-center{align-items:stretch;flex-direction:column;gap:7px}.topbar-center>p{display:none}.topbar-meta{justify-content:space-between}.project-switcher{flex-wrap:wrap;justify-content:flex-start}.project-switcher label{flex:0 0 auto}.select-wrap{flex:1;min-width:140px}.select-wrap select{min-width:0}.mobile-workspace-tabs{display:grid;grid-template-columns:1fr 1fr;position:sticky;top:0;z-index:5;background:#07090c;padding:6px 0}.mobile-workspace-tabs button{border-radius:0;border-color:transparent;background:transparent}.mobile-workspace-tabs button[aria-selected="true"]{color:var(--commerce-accent);border-bottom-color:var(--commerce-accent)}.creator-columns{display:block}.mobile-hidden{display:none}.result-pane{position:static;height:auto;min-height:65vh}.creator-step{padding:14px}.field-grid,.report-form{grid-template-columns:1fr}.role-uploads article{display:block}.estimate-card{grid-template-columns:1fr 1fr}.case-library{padding:22px}.case-library h2{font-size:32px}.case-library img{height:300px}.case-stats{grid-template-columns:1fr 1fr 1fr}}
@media(min-width:1200px){.commerce-creator-shell{--commerce-topbar-offset:96px}}
@media(min-width:768px) and (max-width:1199px){.commerce-creator-shell{--commerce-topbar-offset:216px}}
.section-scope-list{display:grid;gap:7px}.section-scope-row{display:grid;grid-template-columns:minmax(0,1fr) 150px;align-items:center;gap:10px;padding:8px;border:1px solid #293039;border-radius:10px;min-width:0}.section-scope-row label{display:block}.section-scope-row input{position:absolute;opacity:0}.section-scope-row b{font-size:12px;color:var(--commerce-muted);text-align:right}.result-group{margin-top:12px;padding:12px;border:1px solid #293039;border-radius:10px;min-width:0}.result-group h3{margin:0 0 4px;color:var(--commerce-accent)}.result-group>p,.result-group>code{display:block;overflow-wrap:anywhere;color:var(--commerce-muted)}
@media(max-width:767px){.section-scope-row{grid-template-columns:minmax(0,1fr)}.section-scope-row b{text-align:left}.result-items>div>div{align-items:flex-start;gap:8px;flex-direction:column}}

/* Semantic palette keeps the commerce workspace aligned with the global theme. */
.commerce-creator-shell {
  color-scheme: dark;
  --commerce-accent-fill: #b7ff2a;
  --commerce-surface: #101317;
  --commerce-surface-raised: #151a20;
  --commerce-surface-subtle: #0b0e11;
  --commerce-input: #090c0f;
  --commerce-border-strong: #38414a;
  --commerce-text: #f5f7f8;
  --commerce-text-secondary: #c7ccd1;
  --commerce-on-accent: #081000;
  --commerce-accent-soft: #18210e;
  --commerce-notice-bg: #232716;
  --commerce-status-bg: #242a30;
  --commerce-success-bg: #1c2911;
  --commerce-danger-bg: #32151b;
  --commerce-danger-text: #ff9eaa;
  --commerce-warning-bg: #312813;
  --commerce-warning-text: #ffd97a;
  --commerce-progress-bg: #252c32;
  --commerce-overlay-shadow: 0 22px 60px rgba(0, 0, 0, .67);
}
.commerce-creator-shell[data-theme="light"] {
  color-scheme: light;
  --commerce-accent: #4d7c0f;
  --commerce-accent-fill: #84cc16;
  --commerce-bg: #f4f7fb;
  --commerce-panel: #ffffff;
  --commerce-panel-2: #f7f9fc;
  --commerce-surface: #ffffff;
  --commerce-surface-raised: #f5f7fa;
  --commerce-surface-subtle: #f8fafc;
  --commerce-input: #ffffff;
  --commerce-border: #d6dde7;
  --commerce-border-strong: #bdc8d5;
  --commerce-text: #17202c;
  --commerce-text-secondary: #475467;
  --commerce-muted: #667085;
  --commerce-on-accent: #1a2e05;
  --commerce-accent-soft: #eff8dc;
  --commerce-notice-bg: #f4f7e9;
  --commerce-status-bg: #eef2f6;
  --commerce-success-bg: #ecf7d8;
  --commerce-danger-bg: #fff0f1;
  --commerce-danger-text: #b42336;
  --commerce-warning-bg: #fff7df;
  --commerce-warning-text: #92610a;
  --commerce-progress-bg: #e4e9ef;
  --commerce-overlay-shadow: 0 22px 55px rgba(15, 23, 42, .16);
}
.commerce-creator-shell { color: var(--commerce-text); }
.creator-topbar,
.result-panel nav { background: color-mix(in srgb, var(--commerce-surface) 94%, transparent); }
.result-pane,
.creator-step { background: var(--commerce-surface); }
.brand-mark > span,
.commerce-creator-shell .primary-action { background: var(--commerce-accent-fill); color: var(--commerce-on-accent); }
.commerce-creator-shell label,
.generation-config legend { color: var(--commerce-text-secondary); }
.step-number,
.commerce-creator-shell input,
.commerce-creator-shell textarea,
.commerce-creator-shell select,
.role-uploads article,
.role-uploads .image-upload-zone,
.commerce-creator-shell button,
.commerce-creator-shell a,
.option-grid span,
.estimate-card,
.case-library figure,
.section-scope-row,
.result-group { border-color: var(--commerce-border-strong); }
.commerce-creator-shell input,
.commerce-creator-shell textarea,
.commerce-creator-shell select { background: var(--commerce-input); color: var(--commerce-text); }
.role-uploads article,
.facts li,
.option-grid span,
.estimate-card,
.case-stats span,
.result-batch { background: var(--commerce-surface-subtle); }
.commerce-creator-shell button,
.commerce-creator-shell a { background: var(--commerce-surface-raised); color: var(--commerce-text); }
.notice,
.missing,
.risk { background: var(--commerce-notice-bg); }
.status-line > span { background: var(--commerce-status-bg); }
.status-line .status-confirmed,
.option-grid .selected span,
.result-panel nav .active { background: var(--commerce-accent-soft); }
.global-error { background: var(--commerce-danger-bg); color: var(--commerce-danger-text); }
.global-warning { background: var(--commerce-warning-bg); color: var(--commerce-warning-text); }
.missing span { color: var(--commerce-warning-text); }
.credit-warning { color: var(--commerce-danger-text); }
.case-library figure { background: var(--commerce-media-bg, var(--commerce-surface-subtle)); }
.case-stats b { color: var(--commerce-text); }
.progress { background: var(--commerce-progress-bg); }
.result-items > div,
.compact-list > div,
.fullscreen-grid dl > div { border-color: var(--commerce-border); }
.mobile-workspace-tabs { background: var(--commerce-bg); }
.report-skeleton span { background: linear-gradient(90deg, var(--commerce-surface-raised), var(--commerce-status-bg), var(--commerce-surface-raised)); background-size: 200%; }

/* The category popover is rendered by a child component but inherits this palette. */
.commerce-creator-shell .category-trigger,
.commerce-creator-shell .category-search { background: var(--commerce-input) !important; }
.commerce-creator-shell .category-popover { background: var(--commerce-surface); border-color: var(--commerce-border-strong); box-shadow: var(--commerce-overlay-shadow); color: var(--commerce-text); }
.commerce-creator-shell .category-popover > header,
.commerce-creator-shell .category-columns nav,
.commerce-creator-shell .recent-categories,
.commerce-creator-shell .category-popover footer { border-color: var(--commerce-border); }
.commerce-creator-shell .category-columns nav .active { color: var(--commerce-accent); background: var(--commerce-accent-soft) !important; }
.commerce-creator-shell .category-columns small,
.commerce-creator-shell .category-results small,
.commerce-creator-shell .category-results .create-custom { color: var(--commerce-accent); }
.commerce-creator-shell .category-results > p { color: var(--commerce-muted); }
.commerce-creator-shell .category-results .create-custom { border-color: var(--commerce-accent) !important; }
</style>
