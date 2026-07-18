<script setup>
import { ChevronDown, Plus } from 'lucide-vue-next'
defineProps({ projects: { type: Array, default: () => [] }, currentProject: { type: Object, default: null } })
const emit = defineEmits(['select', 'new'])
</script>
<template>
  <div class="project-switcher">
    <label class="project-switcher-label" for="commerce-project-select" data-testid="commerce-project-label">最近创作</label>
    <span class="select-wrap"><select id="commerce-project-select" data-testid="commerce-project-select" :value="currentProject?.id || ''" @change="emit('select', projects.find(item => `${item.id}` === $event.target.value))"><option value="">新的商品</option><option v-for="item in projects" :key="item.id" :value="item.id">{{ item.title }}</option></select><ChevronDown :size="15" aria-hidden="true"/></span>
    <button type="button" data-testid="commerce-project-new" @click="emit('new')"><Plus :size="16"/>新建创作</button>
  </div>
</template>
