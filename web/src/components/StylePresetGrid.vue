<script setup>
import { Ban, Palette, Image, Sparkles, ShoppingBag, Zap, Flower2, Settings } from 'lucide-vue-next'

const props = defineProps({
  modelValue: {
    type: String,
    default: ''
  },
  disabled: {
    type: Boolean,
    default: false
  }
})

const emit = defineEmits(['update:modelValue'])

const presets = [
  { value: '', label: '无风格', icon: Ban },
  { value: '写实', label: '写实', icon: Image },
  { value: '插画', label: '插画', icon: Palette },
  { value: '漫画', label: '漫画', icon: Sparkles },
  { value: '电商', label: '电商', icon: ShoppingBag },
  { value: '转幻', label: '转幻', icon: Zap },
  { value: '国风', label: '国风', icon: Flower2 },
  { value: '自定义', label: '自定义', icon: Settings }
]

function selectPreset(value) {
  if (!props.disabled) {
    emit('update:modelValue', props.modelValue === value ? '' : value)
  }
}
</script>

<template>
  <div class="style-preset-grid">
    <button
      v-for="preset in presets"
      :key="preset.value"
      class="style-preset-card"
      :class="{ active: modelValue === preset.value, disabled }"
      :disabled="disabled"
      @click="selectPreset(preset.value)"
    >
      <component :is="preset.icon" :size="24" class="preset-icon" />
      <span class="preset-label">{{ preset.label }}</span>
    </button>
  </div>
</template>
