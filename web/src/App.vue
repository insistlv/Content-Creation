<script setup>
import GlobalImagePreview from './components/GlobalImagePreview.vue'
import GlobalNetworkAlert from './components/GlobalNetworkAlert.vue'

const sourceCodeURL = import.meta.env.VITE_SOURCE_CODE_URL || 'https://github.com/your-org/image-agent'
</script>

<template>
  <RouterView />
  <GlobalImagePreview />
  <GlobalNetworkAlert />
  <a class="source-code-link" :href="sourceCodeURL" target="_blank" rel="noopener noreferrer">
    源码 · AGPL-3.0
  </a>
</template>

<style scoped>
.source-code-link {
  position: fixed;
  right: 14px;
  bottom: 12px;
  z-index: 20;
  padding: 6px 10px;
  border: 1px solid rgb(148 163 184 / 35%);
  border-radius: 999px;
  background: rgb(15 23 42 / 78%);
  color: #e2e8f0;
  font-size: 12px;
  text-decoration: none;
  backdrop-filter: blur(8px);
}
</style>
