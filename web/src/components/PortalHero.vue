<script setup>
import FloatingFeatureCluster from './FloatingFeatureCluster.vue'
import PillTag from './PillTag.vue'

defineProps({
  eyebrow: {
    type: String,
    default: ''
  },
  title: {
    type: String,
    default: ''
  },
  lead: {
    type: String,
    default: ''
  },
  primaryTo: {
    type: [String, Object],
    default: '/'
  },
  primaryLabel: {
    type: String,
    default: ''
  },
  secondaryTo: {
    type: [String, Object],
    default: '/'
  },
  secondaryLabel: {
    type: String,
    default: ''
  },
  summaryItems: {
    type: Array,
    default: () => []
  },
  features: {
    type: Array,
    default: () => []
  },
  coreKicker: {
    type: String,
    default: ''
  },
  coreTitle: {
    type: String,
    default: ''
  },
  coreDescription: {
    type: String,
    default: ''
  }
})
</script>

<template>
  <section class="portal-hero">
    <div class="portal-hero-copy">
      <p class="eyebrow">{{ eyebrow }}</p>
      <h1>{{ title }}</h1>
      <p class="hero-lead">{{ lead }}</p>

      <div class="portal-actions">
        <RouterLink class="primary-button" :to="primaryTo">{{ primaryLabel }}</RouterLink>
        <RouterLink class="secondary-button" :to="secondaryTo">{{ secondaryLabel }}</RouterLink>
      </div>

      <div class="portal-summary-rail">
        <PillTag v-for="item in summaryItems" :key="item" tone="muted">{{ item }}</PillTag>
      </div>
    </div>

    <FloatingFeatureCluster :items="features">
      <div class="portal-hero-core">
        <span class="portal-hero-core-kicker">{{ coreKicker }}</span>
        <strong>{{ coreTitle }}</strong>
        <p>{{ coreDescription }}</p>
      </div>
    </FloatingFeatureCluster>
  </section>
</template>
