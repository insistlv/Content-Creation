<script setup>
defineProps({
  tone: {
    type: String,
    default: 'default'
  }
})
</script>

<template>
  <span :class="['pill-tag', `pill-tag-${tone}`]">
    <slot />
  </span>
</template>
