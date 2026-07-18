<script setup>
import { ChevronDown } from 'lucide-vue-next'
import { computed, nextTick, onBeforeUnmount, ref, useAttrs, watch } from 'vue'

defineOptions({
  inheritAttrs: false
})

const props = defineProps({
  modelValue: {
    type: [String, Number, Boolean, null],
    default: ''
  },
  options: {
    type: Array,
    default: () => []
  },
  disabled: {
    type: Boolean,
    default: false
  },
  ariaLabel: {
    type: String,
    default: ''
  },
  dataTestid: {
    type: String,
    default: ''
  },
  triggerLabel: {
    type: String,
    default: ''
  },
  compact: {
    type: Boolean,
    default: false
  }
})

const emit = defineEmits(['update:modelValue', 'change'])
const attrs = useAttrs()

const triggerRef = ref(null)
const menuRef = ref(null)
const isOpen = ref(false)
const activeIndex = ref(-1)
const menuStyle = ref({})
let themeObserver = null

const normalizedOptions = computed(() => props.options.map((option) => {
  if (option && typeof option === 'object') {
    return {
      value: option.value,
      label: option.label ?? option.text ?? String(option.value ?? ''),
      disabled: Boolean(option.disabled)
    }
  }

  return {
    value: option,
    label: String(option),
    disabled: false
  }
}))

const selectedIndex = computed(() => normalizedOptions.value.findIndex((option) => option.value === props.modelValue))
const selectedOption = computed(() => normalizedOptions.value[selectedIndex.value] || normalizedOptions.value[0] || null)
const triggerText = computed(() => props.triggerLabel || selectedOption.value?.label || '请选择')
const triggerTestid = computed(() => props.dataTestid || attrs['data-testid'] || undefined)
const listboxId = computed(() => triggerTestid.value ? `${triggerTestid.value}-menu` : undefined)

const triggerClass = computed(() => [
  'click-select-trigger',
  {
    'click-select-trigger-open': isOpen.value,
    'click-select-trigger-compact': props.compact
  },
  attrs.class
])

function firstEnabledIndex() {
  return normalizedOptions.value.findIndex((option) => !option.disabled)
}

function nextEnabledIndex(fromIndex, direction) {
  if (!normalizedOptions.value.length) {
    return -1
  }

  let index = fromIndex
  for (let step = 0; step < normalizedOptions.value.length; step += 1) {
    index = (index + direction + normalizedOptions.value.length) % normalizedOptions.value.length
    if (!normalizedOptions.value[index]?.disabled) {
      return index
    }
  }

  return -1
}

function copyThemeVariables(...sourceStyles) {
  const variableNames = [
    '--bg',
    '--bg-soft',
    '--panel',
    '--panel-strong',
    '--panel-highlight',
    '--ink',
    '--text',
    '--text-muted',
    '--muted',
    '--line',
    '--line-strong',
    '--border',
    '--border-muted',
    '--accent',
    '--accent-hover',
    '--accent-subtle',
    '--radius-md',
    '--click-select-trigger-bg',
    '--click-select-trigger-border',
    '--click-select-trigger-text',
    '--click-select-trigger-hover-bg',
    '--click-select-trigger-focus-border',
    '--click-select-focus-ring',
    '--click-select-menu-bg',
    '--click-select-menu-text',
    '--click-select-menu-border',
    '--click-select-menu-shadow',
    '--click-select-option-text',
    '--click-select-option-active-bg',
    '--click-select-option-active-text',
    '--click-select-option-selected-bg',
    '--click-select-option-selected-text'
  ]

  return Object.fromEntries(variableNames.flatMap((name) => {
    const value = sourceStyles
      .map((sourceStyle) => sourceStyle?.getPropertyValue(name))
      .find(Boolean)

    return value ? [[name, value]] : []
  }))
}

function closestThemeContainer() {
  return triggerRef.value?.closest('.user-dark-shell, .user-light-shell, [data-theme]') || null
}

function disconnectThemeObserver() {
  themeObserver?.disconnect()
  themeObserver = null
}

function observeThemeContainer() {
  disconnectThemeObserver()

  const themeContainer = closestThemeContainer()
  if (!themeContainer || typeof MutationObserver === 'undefined') {
    return
  }

  themeObserver = new MutationObserver(() => {
    if (isOpen.value) {
      updateMenuPosition()
    }
  })
  themeObserver.observe(themeContainer, {
    attributes: true,
    attributeFilter: ['class', 'data-theme', 'style']
  })
}

function updateMenuPosition() {
  if (!triggerRef.value) {
    return
  }

  const rect = triggerRef.value.getBoundingClientRect()
  const sourceStyle = window.getComputedStyle(triggerRef.value)
  const themeStyle = closestThemeContainer()
    ? window.getComputedStyle(closestThemeContainer())
    : null
  menuStyle.value = {
    ...copyThemeVariables(sourceStyle, themeStyle),
    position: 'fixed',
    top: `${Math.round(rect.bottom + 8)}px`,
    left: `${Math.round(rect.left)}px`,
    width: `${Math.round(rect.width)}px`
  }
}

function openMenu(preferredIndex = selectedIndex.value) {
  if (props.disabled || !normalizedOptions.value.length) {
    return
  }

  const fallbackIndex = firstEnabledIndex()
  activeIndex.value = normalizedOptions.value[preferredIndex]?.disabled ? fallbackIndex : preferredIndex
  if (activeIndex.value < 0) {
    activeIndex.value = fallbackIndex
  }
  updateMenuPosition()
  isOpen.value = true
  observeThemeContainer()
  window.addEventListener('resize', updateMenuPosition)
  window.addEventListener('scroll', updateMenuPosition, true)
  document.addEventListener('pointerdown', handleOutsidePointerDown)
}

function closeMenu() {
  isOpen.value = false
  disconnectThemeObserver()
  window.removeEventListener('resize', updateMenuPosition)
  window.removeEventListener('scroll', updateMenuPosition, true)
  document.removeEventListener('pointerdown', handleOutsidePointerDown)
}

function toggleMenu() {
  if (isOpen.value) {
    closeMenu()
    return
  }
  openMenu()
}

function selectOption(option) {
  if (props.disabled || option?.disabled) {
    return
  }

  emit('update:modelValue', option.value)
  emit('change', option.value)
  closeMenu()
  nextTick(() => triggerRef.value?.focus())
}

function handleOutsidePointerDown(event) {
  if (
    triggerRef.value?.contains(event.target)
    || menuRef.value?.contains(event.target)
  ) {
    return
  }
  closeMenu()
}

function moveActive(direction) {
  const fromIndex = activeIndex.value >= 0 ? activeIndex.value : selectedIndex.value
  activeIndex.value = nextEnabledIndex(fromIndex, direction)
}

function handleKeydown(event) {
  if (props.disabled) {
    return
  }

  if (event.key === 'Escape') {
    if (isOpen.value) {
      event.preventDefault()
      closeMenu()
    }
    return
  }

  if (event.key === 'ArrowDown') {
    event.preventDefault()
    if (!isOpen.value) {
      openMenu(selectedIndex.value)
      return
    }
    moveActive(1)
    return
  }

  if (event.key === 'ArrowUp') {
    event.preventDefault()
    if (!isOpen.value) {
      openMenu(selectedIndex.value)
      return
    }
    moveActive(-1)
    return
  }

  if (event.key === 'Home') {
    event.preventDefault()
    if (!isOpen.value) {
      openMenu(firstEnabledIndex())
    }
    activeIndex.value = firstEnabledIndex()
    return
  }

  if (event.key === 'End') {
    event.preventDefault()
    const lastEnabledIndex = [...normalizedOptions.value].reverse().findIndex((option) => !option.disabled)
    const index = lastEnabledIndex < 0 ? -1 : normalizedOptions.value.length - 1 - lastEnabledIndex
    if (!isOpen.value) {
      openMenu(index)
    }
    activeIndex.value = index
    return
  }

  if (event.key === 'Enter' || event.key === ' ') {
    event.preventDefault()
    if (!isOpen.value) {
      openMenu()
      return
    }
    selectOption(normalizedOptions.value[activeIndex.value])
  }
}

function optionTestid(option) {
  if (!triggerTestid.value) {
    return undefined
  }
  return `${triggerTestid.value}-option-${String(option.value)}`
}

watch(() => props.disabled, (disabled) => {
  if (disabled) {
    closeMenu()
  }
})

onBeforeUnmount(() => {
  closeMenu()
})
</script>

<template>
  <button
    v-bind="{ ...attrs, class: undefined }"
    ref="triggerRef"
    type="button"
    :class="triggerClass"
    :data-testid="triggerTestid"
    :value="modelValue"
    :disabled="disabled"
    :aria-label="ariaLabel || undefined"
    :aria-expanded="isOpen ? 'true' : 'false'"
    :aria-controls="listboxId"
    aria-haspopup="listbox"
    @click="toggleMenu"
    @keydown="handleKeydown"
  >
    <span class="click-select-value">{{ triggerText }}</span>
    <ChevronDown class="click-select-chevron" aria-hidden="true" />
  </button>

  <Teleport to="body">
    <div
      v-if="isOpen"
      :id="listboxId"
      ref="menuRef"
      class="click-select-menu"
      :style="menuStyle"
      role="listbox"
      :aria-label="ariaLabel || undefined"
      :data-testid="triggerTestid ? `${triggerTestid}-menu` : undefined"
    >
      <button
        v-for="(option, index) in normalizedOptions"
        :key="`${String(option.value)}-${index}`"
        type="button"
        role="option"
        class="click-select-option"
        :class="{
          'click-select-option-active': index === activeIndex,
          'click-select-option-selected': option.value === modelValue
        }"
        :aria-selected="option.value === modelValue ? 'true' : 'false'"
        :disabled="option.disabled"
        :data-testid="optionTestid(option)"
        @mouseenter="activeIndex = index"
        @click="selectOption(option)"
      >
        {{ option.label }}
      </button>
    </div>
  </Teleport>
</template>
