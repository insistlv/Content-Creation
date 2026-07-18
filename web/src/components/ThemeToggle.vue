<script setup>
import { computed } from 'vue'
import { Moon, Sun } from 'lucide-vue-next'

import { useUserTheme } from '../composables/useUserTheme.js'

const { isDarkTheme, toggleTheme } = useUserTheme()

const ariaLabel = computed(() => (isDarkTheme.value ? '切换到亮色模式' : '切换到暗色模式'))
const iconComponent = computed(() => (isDarkTheme.value ? Moon : Sun))
</script>

<template>
  <button
    :class="['theme-toggle', { 'theme-toggle-light': !isDarkTheme, 'theme-toggle-dark': isDarkTheme }]"
    type="button"
    data-testid="site-theme-toggle"
    :aria-label="ariaLabel"
    @click="toggleTheme"
  >
    <span class="theme-toggle-thumb" aria-hidden="true">
      <component :is="iconComponent" :size="15" stroke-width="2.4" />
    </span>
  </button>
</template>
