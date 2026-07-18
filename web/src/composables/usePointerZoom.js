import { computed, ref } from 'vue'

function clampNumber(value, min, max) {
  return Math.min(max, Math.max(min, value))
}

function roundNumber(value) {
  const rounded = Number(value.toFixed(4))
  return Object.is(rounded, -0) ? 0 : rounded
}

export function usePointerZoom(options = {}) {
  const minScale = options.minScale ?? 1
  const maxScale = options.maxScale ?? 4
  const step = options.step ?? 0.2

  const imageRef = ref(null)
  const scale = ref(minScale)
  const offset = ref({ x: 0, y: 0 })

  const zoomStyle = computed(() => ({
    transform: `translate(${offset.value.x}px, ${offset.value.y}px) scale(${scale.value})`,
    transformOrigin: '0px 0px'
  }))

  function resetZoom() {
    scale.value = minScale
    offset.value = { x: 0, y: 0 }
  }

  function handleWheel(event) {
    event.preventDefault()

    const image = imageRef.value
    if (!image) return

    const currentScale = scale.value
    const zoomStep = event.deltaY < 0 ? step : -step
    const nextScale = Number(clampNumber(currentScale + zoomStep, minScale, maxScale).toFixed(2))

    if (nextScale === minScale) {
      scale.value = nextScale
      offset.value = { x: 0, y: 0 }
      return
    }

    const rect = image.getBoundingClientRect()
    if (rect.width <= 0 || rect.height <= 0 || currentScale <= 0) {
      scale.value = nextScale
      return
    }

    const anchorX = clampNumber(event.clientX, rect.left, rect.right) - rect.left
    const anchorY = clampNumber(event.clientY, rect.top, rect.bottom) - rect.top
    const scaleRatio = nextScale / currentScale

    offset.value = {
      x: roundNumber(offset.value.x + anchorX * (1 - scaleRatio)),
      y: roundNumber(offset.value.y + anchorY * (1 - scaleRatio))
    }
    scale.value = nextScale
  }

  return {
    imageRef,
    zoomStyle,
    handleWheel,
    resetZoom
  }
}
