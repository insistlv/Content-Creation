<script setup>
defineProps({
  as: {
    type: String,
    default: 'section'
  },
  tone: {
    type: String,
    default: 'default'
  },
  roomy: {
    type: Boolean,
    default: false
  }
})
</script>

<template>
  <component :is="as" :class="['soft-panel', `soft-panel-${tone}`, { 'soft-panel-roomy': roomy }]">
    <slot />
  </component>
</template>
