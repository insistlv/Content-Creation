<script setup>
import { ArrowUpRight } from 'lucide-vue-next'

defineProps({
  tool: {
    type: Object,
    required: true
  },
  active: {
    type: Boolean,
    default: false
  },
  disabled: {
    type: Boolean,
    default: false
  }
})

defineEmits(['select'])
</script>

<template>
  <button
    type="button"
    class="imini-tool-card"
    :class="{ active }"
    :data-testid="`workspace-tool-${tool.mode}`"
    :disabled="disabled"
    @click="$emit('select', tool)"
  >
    <span class="imini-tool-card-content">
      <span class="imini-card-copy-head">
        <span class="imini-tool-icon" aria-hidden="true">
          <component :is="tool.icon" :size="18" />
        </span>
        <span class="imini-card-enter" aria-hidden="true">
          <ArrowUpRight :size="16" />
        </span>
      </span>
      <strong>{{ tool.title }}</strong>
      <span>{{ tool.description }}</span>
    </span>
    <span class="imini-tool-card-media">
      <img :src="tool.image" :alt="tool.title" />
    </span>
  </button>
</template>
