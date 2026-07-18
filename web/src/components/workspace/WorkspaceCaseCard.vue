<script setup>
import { ArrowUpRight } from 'lucide-vue-next'

defineProps({
  item: {
    type: Object,
    required: true
  }
})

defineEmits(['select'])
</script>

<template>
  <button
    type="button"
    class="imini-template-card imini-case-card"
    :data-testid="`workspace-template-${item.id}`"
    @click="$emit('select', item)"
  >
    <span class="imini-case-card-media">
      <img :src="item.preview_url" :alt="item.title" />
    </span>
    <span class="imini-case-card-content">
      <span class="imini-card-enter" aria-hidden="true">
        <ArrowUpRight :size="16" />
      </span>
      <strong>{{ item.title }}</strong>
      <span>{{ item.description }}</span>
    </span>
  </button>
</template>
