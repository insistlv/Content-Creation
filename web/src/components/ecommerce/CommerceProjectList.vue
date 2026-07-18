<script setup>
defineProps({ projects: { type: Array, default: () => [] }, currentId: { default: null } })
defineEmits(['select', 'create', 'delete'])
</script>
<template>
  <section class="commerce-card project-list" aria-label="电商项目">
    <header><h2>商品项目</h2><button type="button" @click="$emit('create')">新建项目</button></header>
    <div v-if="!projects.length" data-testid="commerce-project-empty" class="empty">尚无商品项目，创建第一个项目开始建档。</div>
    <button v-for="project in projects" :key="project.id" type="button" class="project-row" :class="{ active: project.id === currentId }" @click="$emit('select', project)">
      <span>{{ project.name || project.title || `项目 ${project.id}` }}</span><small>{{ project.pipeline || '通用商品' }}</small>
    </button>
  </section>
</template>
