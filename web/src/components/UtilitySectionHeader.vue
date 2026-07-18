<script setup>
defineProps({
  eyebrow: {
    type: String,
    default: ''
  },
  title: {
    type: String,
    default: ''
  },
  description: {
    type: String,
    default: ''
  },
  compact: {
    type: Boolean,
    default: false
  }
})
</script>

<template>
  <header :class="['utility-section-header', { 'utility-section-header-compact': compact }]">
    <div class="utility-section-copy">
      <p v-if="eyebrow" class="eyebrow">{{ eyebrow }}</p>
      <h1 v-if="!compact">{{ title }}</h1>
      <h2 v-else>{{ title }}</h2>
      <p v-if="description" class="section-copy">{{ description }}</p>
    </div>
    <div v-if="$slots.actions" class="utility-section-actions">
      <slot name="actions" />
    </div>
  </header>
</template>
