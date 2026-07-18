<script setup>
import PillTag from './PillTag.vue'

defineProps({
  items: {
    type: Array,
    default: () => []
  }
})
</script>

<template>
  <div class="floating-cluster">
    <div class="floating-cluster-core">
      <slot />
    </div>

    <div
      v-for="(item, index) in items"
      :key="item.label"
      class="floating-cluster-item"
      :style="{
        '--floating-x': item.x,
        '--floating-y': item.y,
        '--floating-delay': `${index * 0.08}s`
      }"
    >
      <PillTag :tone="item.tone || 'default'">{{ item.label }}</PillTag>
    </div>
  </div>
</template>
