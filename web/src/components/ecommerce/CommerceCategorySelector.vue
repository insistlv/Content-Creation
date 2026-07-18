<script setup>
import { computed, nextTick, ref, watch } from 'vue'
import { ChevronDown, Plus, Search, Settings2, X } from 'lucide-vue-next'

const props = defineProps({ modelValue: { type: Object, default: null }, catalog: { type: Object, default: () => ({}) }, disabled: Boolean, loading: Boolean })
const emit = defineEmits(['select', 'create-custom', 'patch-custom'])
const open = ref(false), query = ref(''), activeParentId = ref(0), managing = ref(false), searchInput = ref(null), highlightedIndex = ref(-1)
const customNames = ref({})
const roots = computed(() => props.catalog?.system_categories || [])
const custom = computed(() => props.catalog?.custom_categories || [])
const activeRoot = computed(() => roots.value.find(item => item.id === activeParentId.value) || roots.value[0] || null)
const normalizedQuery = computed(() => query.value.trim().toLowerCase())
const allSelectable = computed(() => [
  ...roots.value.flatMap(root => (root.children || []).map(item => ({ ...item, parent_id: root.id, source: 'system', path: item.path || `${root.name} / ${item.name}` }))),
  ...custom.value.filter(item => item.status !== 'inactive')
])
const searchResults = computed(() => {
  const keyword = normalizedQuery.value
  if (!keyword) return []
  return allSelectable.value.filter(item => [item.name, item.path, ...(item.aliases || [])].some(value => `${value || ''}`.toLowerCase().includes(keyword)))
})
const visibleChildren = computed(() => {
  if (!activeRoot.value) return []
  return [...(activeRoot.value.children || []).map(item => ({ ...item, source: 'system', path: item.path || `${activeRoot.value.name} / ${item.name}` })), ...custom.value.filter(item => item.parent_id === activeRoot.value.id && item.status !== 'inactive')]
})
const canCreate = computed(() => normalizedQuery.value && searchResults.value.length === 0 && activeRoot.value)
watch(roots, value => { if (!activeParentId.value && value.length) activeParentId.value = value[0].id }, { immediate: true })
watch(custom, values => { customNames.value = Object.fromEntries(values.map(item => [item.id, item.name])) }, { immediate: true })
watch(query, () => { highlightedIndex.value = -1 })
watch(() => props.modelValue, value => { if (value?.id) close() })

async function toggle() { if (props.disabled) return; open.value = !open.value; managing.value = false; if (open.value) { await nextTick(); searchInput.value?.focus() } }
function choose(item) { emit('select', { id: item.id, parent_id: item.parent_id, source: item.source || 'system', name: item.name, path: item.path }); open.value = false; query.value = '' }
function createCustom() { emit('create-custom', { parent_id: activeRoot.value.id, name: query.value.trim() }) }
function close() { open.value = false; query.value = ''; managing.value = false }
function onSearchKeydown(event) {
  if (!searchResults.value.length) return
  if (event.key === 'ArrowDown') { event.preventDefault(); highlightedIndex.value = (highlightedIndex.value + 1) % searchResults.value.length }
  if (event.key === 'ArrowUp') { event.preventDefault(); highlightedIndex.value = (highlightedIndex.value - 1 + searchResults.value.length) % searchResults.value.length }
  if (event.key === 'Enter') { event.preventDefault(); choose(searchResults.value[Math.max(0, highlightedIndex.value)]) }
}
</script>

<template>
  <div class="commerce-category-selector">
    <button data-testid="category-trigger" class="category-trigger" type="button" :disabled="disabled || loading" aria-haspopup="dialog" :aria-expanded="open" @click="toggle">
      <span>{{ modelValue?.path || '请选择商品品类' }}</span><ChevronDown :size="16" />
    </button>
    <div v-if="open" class="category-popover" role="dialog" aria-label="选择商品品类" @keydown.esc="close">
	  <header><div class="category-search"><Search :size="16"/><input ref="searchInput" v-model="query" data-testid="category-search" placeholder="搜索水杯、保温杯、面霜等" @keydown="onSearchKeydown" /></div><button type="button" aria-label="关闭" @click="close"><X :size="16"/></button></header>
      <template v-if="managing">
        <div class="custom-manager"><div class="category-manager-title"><b>管理我的品类</b><button type="button" @click="managing = false">返回选择</button></div>
          <p v-if="!custom.length">还没有个人品类</p>
          <div v-for="item in custom" :key="item.id" class="custom-row"><input v-model="customNames[item.id]" :data-testid="`category-custom-name-${item.id}`"/><button type="button" :data-testid="`category-custom-save-${item.id}`" @click="emit('patch-custom',{ id:item.id,input:{name:customNames[item.id]} })">保存</button><button type="button" :data-testid="`category-custom-toggle-${item.id}`" @click="emit('patch-custom',{ id:item.id,input:{status:item.status === 'inactive' ? 'active' : 'inactive'} })">{{ item.status === 'inactive' ? '恢复' : '停用' }}</button></div>
        </div>
      </template>
      <template v-else-if="normalizedQuery">
		<div class="category-results"><p>搜索结果</p><button v-for="(item,index) in searchResults" :key="`${item.source}-${item.id}`" type="button" :class="{highlighted:index===highlightedIndex}" :data-category-id="`${item.source}-${item.id}`" @mouseenter="highlightedIndex=index" @click="choose(item)"><span>{{ item.path }}</span><small v-if="item.source === 'user'">我的品类</small></button><button v-if="canCreate" data-testid="category-create-custom" class="create-custom" type="button" @click="createCustom"><Plus :size="16"/>在“{{ activeRoot.name }}”下新增“{{ query.trim() }}”</button></div>
      </template>
      <template v-else>
        <section v-if="catalog?.recent_categories?.length" class="recent-categories"><b>最近使用</b><div><button v-for="item in catalog.recent_categories" :key="`${item.source}-${item.id}`" type="button" @click="choose(item)">{{ item.path }}</button></div></section>
        <div class="category-columns"><nav aria-label="一级商品品类"><button v-for="root in roots" :key="root.id" type="button" :class="{active:activeRoot?.id===root.id}" @click="activeParentId=root.id">{{ root.name }}</button></nav><section><b>{{ activeRoot?.name }}</b><button v-for="item in visibleChildren" :key="`${item.source}-${item.id}`" type="button" :data-category-id="`${item.source}-${item.id}`" @click="choose(item)"><span>{{ item.name }}</span><small v-if="item.source === 'user'">我的</small></button></section></div>
        <footer><button data-testid="category-manage-custom" type="button" @click="managing=true"><Settings2 :size="15"/>管理我的品类</button></footer>
      </template>
    </div>
  </div>
</template>

<style scoped>
.commerce-category-selector{position:relative}.category-trigger{width:100%;display:flex;justify-content:space-between!important;background:#090c0f!important;font-weight:400!important}.category-trigger span{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.category-popover{position:absolute;z-index:30;top:calc(100% + 8px);right:0;width:min(620px,calc(100vw - 32px));border:1px solid #343c45;border-radius:14px;background:#101317;box-shadow:0 22px 60px #000a;overflow:hidden}.category-popover>header{display:flex;gap:8px;padding:12px;border-bottom:1px solid #293039}.category-search{display:flex;align-items:center;gap:8px;flex:1;border:1px solid #343c45;border-radius:9px;padding:0 10px;background:#090c0f}.category-search input{border:0!important;padding-left:0!important}.category-columns{display:grid;grid-template-columns:180px 1fr;min-height:330px}.category-columns nav{padding:8px;border-right:1px solid #293039;overflow:auto}.category-columns nav button,.category-columns section>button,.category-results>button{width:100%;justify-content:flex-start!important;border:0!important;background:transparent!important}.category-columns nav .active{color:#b7ff2a;background:#1b2410!important}.category-columns section{padding:14px;display:grid;align-content:start;grid-template-columns:1fr 1fr;gap:6px}.category-columns section>b{grid-column:1/-1;margin-bottom:5px}.category-columns small,.category-results small{margin-left:auto;color:#b7ff2a}.recent-categories{padding:12px;border-bottom:1px solid #293039}.recent-categories div{display:flex;gap:6px;flex-wrap:wrap;margin-top:8px}.recent-categories button{min-height:32px!important;font-size:11px}.category-results,.custom-manager{padding:14px;min-height:360px}.category-results>p{color:#8d969f}.category-results .create-custom{margin-top:10px;color:#b7ff2a;border:1px dashed #638b20!important}.category-popover footer{padding:10px 12px;border-top:1px solid #293039}.category-manager-title,.custom-row{display:flex;align-items:center;gap:8px}.category-manager-title{justify-content:space-between;margin-bottom:12px}.custom-row{margin:8px 0}.custom-row input{flex:1}@media(max-width:767px){.category-popover{position:fixed;left:0;right:0;top:auto;bottom:0;width:100%;max-height:82vh;border-radius:18px 18px 0 0}.category-columns{grid-template-columns:130px 1fr;min-height:55vh}.category-columns section{grid-template-columns:1fr}}
</style>
