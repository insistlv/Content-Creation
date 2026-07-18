<script setup>
import { computed } from 'vue'
import ClickSelect from './ClickSelect.vue'

const props = defineProps({
  modelValue: {
    type: String,
    default: '1:1'
  },
  disabled: {
    type: Boolean,
    default: false
  },
  triggerLabel: {
    type: String,
    default: ''
  }
})

const emit = defineEmits(['update:modelValue'])

const ratios = [
  { value: '21:9', label: '超宽屏', use: '横幅 / 影院感', size: '1536x1024', shape: 'ultrawide' },
  { value: '16:9', label: '横屏', use: '封面 / 桌面', size: '1536x1024', shape: 'wide' },
  { value: '4:3', label: '横图', use: '图文 / 展示', size: '1536x1024', shape: 'landscape' },
  { value: '3:2', label: '相机横幅', use: '摄影 / 海报', size: '1536x1024', shape: 'photo' },
  { value: '1:1', label: '方图', use: '头像 / 商品', size: '1024x1024', shape: 'square' },
  { value: '2:3', label: '竖图', use: '海报 / 人像', size: '1024x1536', shape: 'portrait' },
  { value: '3:4', label: '竖版', use: '小红书 / 封面', size: '1024x1536', shape: 'vertical' },
  { value: '9:16', label: '手机竖屏', use: '短视频 / 壁纸', size: '1024x1536', shape: 'phone' },
  { value: '9:21', label: '长屏', use: '长图 / 故事', size: '1024x1536', shape: 'tall' }
]

const selectedRatio = computed(() => ratios.find((ratio) => ratio.value === props.modelValue) || ratios.find((ratio) => ratio.value === '1:1') || ratios[0])
const ratioOptions = computed(() => ratios.map((ratio) => ({
  value: ratio.value,
  label: `${ratio.value} ${ratio.label} · ${ratio.use} · 推荐输出尺寸 ${ratio.size}`
})))

function selectRatio(value) {
  if (!props.disabled) {
    emit('update:modelValue', value)
  }
}
</script>

<template>
  <div class="aspect-ratio-selector" data-testid="workspace-size-selector">
    <ClickSelect
      :model-value="modelValue"
      class="aspect-ratio-select"
      data-testid="workspace-size-select"
      :options="ratioOptions"
      :disabled="disabled"
      :trigger-label="triggerLabel"
      aria-label="尺寸"
      @update:model-value="selectRatio"
    />

    <div class="selected-ratio-summary" data-testid="workspace-size-selected">
      <span class="ratio-preview" aria-hidden="true">
        <span :class="['ratio-preview-frame', `ratio-preview-${selectedRatio.shape}`]" />
      </span>
      <span class="ratio-copy">
        <span class="ratio-main">
          <strong class="ratio-value">{{ selectedRatio.value }}</strong>
          <span>{{ selectedRatio.label }}</span>
        </span>
        <span class="ratio-use">{{ selectedRatio.use }}</span>
        <span class="ratio-size">推荐输出尺寸 {{ selectedRatio.size }}</span>
      </span>
    </div>
  </div>
</template>

<style scoped>
.aspect-ratio-selector {
  display: grid;
  gap: 10px;
}

.aspect-ratio-select {
  width: 100%;
  min-height: 44px;
  padding: 0 42px 0 14px;
  border: 1px solid rgba(140, 151, 170, 0.38);
  border-radius: 8px;
  background:
    linear-gradient(45deg, transparent 50%, #94a3b8 50%) right 18px center / 6px 6px no-repeat,
    linear-gradient(135deg, #94a3b8 50%, transparent 50%) right 12px center / 6px 6px no-repeat,
    #111827;
  color: #f9fafb;
  font: inherit;
  font-weight: 650;
  appearance: none;
  cursor: pointer;
  transition: border-color 0.16s ease, box-shadow 0.16s ease, background-color 0.16s ease;
}

.aspect-ratio-select:hover:not(:disabled),
.aspect-ratio-select:focus {
  border-color: rgba(53, 116, 255, 0.62);
  box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.28);
  outline: none;
}

.aspect-ratio-select:disabled {
  opacity: 0.66;
  cursor: not-allowed;
}

.selected-ratio-summary {
  min-width: 0;
  display: grid;
  grid-template-columns: 46px minmax(0, 1fr);
  align-items: center;
  gap: 12px;
  padding: 12px 14px;
  border: 1px solid rgba(140, 151, 170, 0.28);
  border-radius: 8px;
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.96), rgba(246, 248, 252, 0.92));
  color: var(--text);
  text-align: left;
}

.ratio-preview {
  width: 46px;
  height: 46px;
  display: grid;
  place-items: center;
  border-radius: 8px;
  background: rgba(15, 23, 42, 0.045);
}

.ratio-preview-frame {
  display: block;
  border-radius: 5px;
  border: 2px solid rgba(53, 116, 255, 0.78);
  background:
    linear-gradient(135deg, rgba(53, 116, 255, 0.16), rgba(20, 184, 166, 0.16)),
    #ffffff;
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.85);
}

.ratio-preview-ultrawide {
  width: 38px;
  height: 16px;
}

.ratio-preview-wide {
  width: 36px;
  height: 20px;
}

.ratio-preview-landscape {
  width: 34px;
  height: 24px;
}

.ratio-preview-photo {
  width: 32px;
  height: 22px;
}

.ratio-preview-square {
  width: 28px;
  height: 28px;
}

.ratio-preview-portrait {
  width: 24px;
  height: 34px;
}

.ratio-preview-vertical {
  width: 24px;
  height: 36px;
}

.ratio-preview-phone {
  width: 20px;
  height: 38px;
}

.ratio-preview-tall {
  width: 18px;
  height: 40px;
}

.ratio-copy,
.ratio-main {
  min-width: 0;
  display: flex;
}

.ratio-copy {
  flex-direction: column;
  gap: 4px;
}

.ratio-main {
  align-items: baseline;
  gap: 7px;
  color: var(--text);
}

.ratio-value {
  font-size: 0.98rem;
  line-height: 1.1;
  color: #111827;
}

.ratio-main span,
.ratio-use,
.ratio-size {
  overflow-wrap: anywhere;
}

.ratio-main span {
  font-size: 0.82rem;
  color: var(--text-muted);
}

.ratio-use {
  font-size: 0.78rem;
  line-height: 1.25;
  color: #4b5563;
}

.ratio-size {
  font-size: 0.74rem;
  line-height: 1.25;
  color: #64748b;
}

@media (max-width: 640px) {
  .selected-ratio-summary {
    grid-template-columns: 42px minmax(0, 1fr);
  }
}
</style>
